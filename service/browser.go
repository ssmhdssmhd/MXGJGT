package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"mxgjgt/config"

	"github.com/chromedp/chromedp"
)

// 方案1：无头浏览器渲染解析官方页面
// 用于爱奇艺 / 优酷 / 芒果 等对后端强反爬、纯 HTTP 拿不到剧名的官方站

var (
	chromePathOnce sync.Once
	chromePath     string
)

// findChromePath 按 配置 → 环境变量 → 常见安装路径 → PATH 依次探测 Chrome/Chromium
func findChromePath() string {
	chromePathOnce.Do(func() {
		if cfg := config.GetConfig(); cfg != nil && cfg.Browser.ChromePath != "" {
			if _, err := os.Stat(cfg.Browser.ChromePath); err == nil {
				chromePath = cfg.Browser.ChromePath
				return
			}
		}
		if v := os.Getenv("MXGJGT_CHROME"); v != "" {
			if _, err := os.Stat(v); err == nil {
				chromePath = v
				return
			}
		}
		for _, c := range []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/opt/google/chrome/chrome",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		} {
			if _, err := os.Stat(c); err == nil {
				chromePath = c
				return
			}
		}
		for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome", "chrome-headless-shell"} {
			if p, err := exec.LookPath(name); err == nil {
				chromePath = p
				return
			}
		}
	})
	return chromePath
}

// BrowserAvailable 无头浏览器（方案1）当前是否可用
func BrowserAvailable() bool {
	if cfg := config.GetConfig(); cfg != nil && !cfg.Browser.Enabled {
		return false
	}
	return findChromePath() != ""
}

// ResetChromePath 配置热更新后重置浏览器路径探测缓存
func ResetChromePath() {
	chromePathOnce = sync.Once{}
	chromePath = ""
}

// BrowserUA 渲染用浏览器 UA
const BrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

// RenderOfficialTitle 无头渲染官方页面，返回 剧名, 集数
func RenderOfficialTitle(rawURL string) (title, episode string, err error) {
	path := findChromePath()
	if path == "" {
		return "", "", fmt.Errorf("未找到可用的 Chrome/Chromium")
	}

	timeout := 20 * time.Second
	if cfg := config.GetConfig(); cfg != nil && cfg.Browser.TimeoutSec > 0 {
		timeout = time.Duration(cfg.Browser.TimeoutSec) * time.Second
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(path),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("blink-settings", "imagesEnabled=false"),
		chromedp.UserAgent(BrowserUA),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	// 导航并等待 JS 渲染完成
	if _, err := chromedp.Run(ctx, chromedp.Steps(
		chromedp.Navigate(rawURL),
		chromedp.Sleep(3*time.Second),
	)); err != nil {
		return "", "", err
	}

	var domTitle, ogTitle, bodyText string
	if v, e := chromedp.Run(ctx, chromedp.Title()); e == nil {
		domTitle = v
	}
	if v, e := chromedp.Run(ctx, chromedp.Evaluate[string](
		`(function(){var m=document.querySelector('meta[property="og:title"],meta[name="og:title"],meta[name="title"],meta[property="twitter:title"]');return m&&m.content?m.content:"";})()`,
	)); e == nil {
		ogTitle = v
	}
	if v, e := chromedp.Run(ctx, chromedp.Evaluate[string](
		`document.body?document.body.innerText.slice(0,3000):""`,
	)); e == nil {
		bodyText = v
	}

	// 依次尝试：document.title → og:title → 页面文本，做剧名抽取
	for _, raw := range []string{domTitle, ogTitle, bodyText} {
		name := cleanTitle(extractDramaName(raw))
		if name == "" || isSuspiciousTitle(name) {
			continue
		}
		if len([]rune(name)) > 30 {
			continue
		}
		title = name
		break
	}
	if title == "" {
		return "", "", fmt.Errorf("渲染后仍未取到有效剧名")
	}

	// 集数：只从标题里找（页面文本里的"共39集"会误判成集数）
	for _, pat := range episodePatterns {
		if m := pat.FindStringSubmatch(title); len(m) >= 2 {
			episode = m[1]
			break
		}
	}
	title = stripEpisodeSuffix(title)
	return strings.TrimSpace(title), episode, nil
}

// dramaNameRe 匹配 《剧名》 形式
var dramaNameRe = regexp.MustCompile(`《([^《》]{1,40})》`)

// extractDramaName 从长标题/页面描述里抽取剧名
// 例：《狂飙》是由徐纪周导演… → 狂飙 ； 狂飙-电视剧-全集 → 狂飙
func extractDramaName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// 1. 形如 《剧名》
	if m := dramaNameRe.FindStringSubmatch(s); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	// 2. 取最早出现的分隔符之前的首段
	best := -1
	for _, sep := range []string{" - ", "|", "_", "，", ",", "：", ":", "（", "(", "-", " "} {
		if idx := strings.Index(s, sep); idx > 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	if best > 0 {
		s = s[:best]
	}
	return strings.TrimSpace(s)
}
