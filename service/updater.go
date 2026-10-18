// Package service — 真·二进制热更新
// 更新流程: 查 GitHub Releases 最新 tag → 下载对应平台/架构二进制 → 原子替换 → exec 自重启
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"

	"github.com/robfig/cron/v3"
)

// ===== GitHub Releases API =====

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Assets      []ghAsset `json:"assets"`
	Body        string    `json:"body"`
	PublishedAt string    `json:"published_at"`
}

// UpdateInfo 版本检查结果（供后台「系统 / 更新」展示当前版本与远程版本）
type UpdateInfo struct {
	Current   string `json:"current"`    // 当前版本
	Latest    string `json:"latest"`     // 远程最新版本（查询失败为空）
	HasUpdate bool   `json:"has_update"` // 是否有新版本
	Release   string `json:"release"`    // 远程 release 名称
	Published string `json:"published"`  // 远程发布时间
	Repo      string `json:"repo"`       // 更新仓库 owner/repo
	Enabled   bool   `json:"enabled"`    // 定时更新是否开启
	Error     string `json:"error"`      // 查询失败原因（有值时 latest 为空）
}

// CheckUpdateInfo 查询远程最新版本（只读，不下载不重启）
func CheckUpdateInfo() UpdateInfo {
	info := UpdateInfo{Current: model.Version}
	cfg := config.GetConfig()
	if cfg == nil {
		info.Error = "配置为空"
		return info
	}
	info.Enabled = cfg.Updater.Enabled
	repo := extractRepo(cfg.Updater.RepoURL)
	info.Repo = repo
	if repo == "" {
		info.Error = "无法从 repo_url 提取 owner/repo"
		return info
	}
	rel, err := fetchLatestRelease(repo)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Latest = strings.TrimPrefix(rel.TagName, "v")
	info.Release = rel.Name
	info.Published = rel.PublishedAt
	// 仅当远程版本「严格高于」当前版本才算有新版本（避免版本号相等/更旧时误报）
	info.HasUpdate = info.Latest != "" && compareVersion(info.Latest, info.Current) > 0
	return info
}

// compareVersion 比较两个点分数字版本，返回 -1(小于) / 0(相等) / 1(大于)
// 兼容前导 v 与 -beta 之类后缀；非数字段按 0 处理
func compareVersion(a, b string) int {
	pa, pb := parseVerParts(a), parseVerParts(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parseVerParts 把版本号拆成数字段
func parseVerParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, s := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		out = append(out, n)
	}
	return out
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// StartAutoUpdater 启动自动更新 cron
func StartAutoUpdater() (*cron.Cron, error) {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Updater.Enabled {
		return nil, nil
	}

	c := cron.New()
	_, err := c.AddFunc(cfg.Updater.Interval, func() {
		logUpdate("开始自动更新检查...")
		if err := DoUpdate(); err != nil {
			logUpdate("更新失败: %v", err)
		}
	})
	if err != nil {
		return nil, err
	}
	c.Start()
	return c, nil
}

// ManualUpdate 手动触发更新（供 API 调用）
func ManualUpdate() string {
	start := time.Now()
	if err := DoUpdate(); err != nil {
		return fmt.Sprintf("更新失败: %v", err)
	}
	return fmt.Sprintf("更新完成，耗时 %v，正在重启...", time.Since(start))
}

// DoUpdate 执行一次完整更新
func DoUpdate() error {
	cfg := config.GetConfig()
	if cfg == nil {
		return fmt.Errorf("配置为空")
	}

	repo := extractRepo(cfg.Updater.RepoURL)
	if repo == "" {
		return fmt.Errorf("无法从 repo_url 提取 owner/repo")
	}

	logUpdate("检查更新: %s (当前 v%s)", repo, model.Version)

	// 1. 获取最新 release
	latest, err := fetchLatestRelease(repo)
	if err != nil {
		return err
	}
	logUpdate("最新版本: %s", latest.TagName)

	tag := strings.TrimPrefix(latest.TagName, "v")
	if cmp := compareVersion(tag, model.Version); cmp <= 0 {
		if cmp == 0 {
			logUpdate("已是最新版本 v%s，无需更新", model.Version)
		} else {
			logUpdate("远程版本 v%s 不高于当前 v%s，跳过更新", tag, model.Version)
		}
		return nil
	}

	// 2. 找匹配当前平台/架构的 asset
	asset := pickAsset(latest.Assets, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		return fmt.Errorf("未找到 %s/%s 对应的发布包", runtime.GOOS, runtime.GOARCH)
	}

	downloadURL := asset.BrowserDownloadURL
	// 应用镜像
	if len(cfg.Updater.Mirrors) > 0 {
		if m := pickMirror(cfg.Updater.Mirrors, downloadURL); m != "" {
			downloadURL = m
			logUpdate("使用镜像: %s", m)
		}
	}

	// 3. 下载到临时文件
	tmpPath, err := downloadToTemp(downloadURL, asset.Size)
	if err != nil {
		return fmt.Errorf("下载失败: %v", err)
	}
	logUpdate("下载完成 -> %s (%.1f MB)", tmpPath, float64(asset.Size)/1024/1024)

	// 4. 原子替换
	newExe, err := replaceBinary(tmpPath)
	if err != nil {
		return fmt.Errorf("替换失败: %v", err)
	}

	// 5. 自重启
	logUpdate("准备重启 -> %s", newExe)
	go func() {
		time.Sleep(1 * time.Second)
		execSelf(newExe)
	}()

	return nil
}

// ===== 内部函数 =====

func extractRepo(url string) string {
	// 支持 https://github.com/owner/repo 或带 .git 后缀
	url = strings.TrimSuffix(url, ".git")
	if idx := strings.Index(url, "github.com/"); idx >= 0 {
		rest := url[idx+len("github.com/"):]
		if parts := strings.SplitN(rest, "/", 2); len(parts) == 2 {
			return parts[0] + "/" + parts[1]
		}
	}
	return ""
}

func fetchLatestRelease(repo string) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mxgjgt-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("仓库或 release 不存在 (404)")
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, string(b))
	}

	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// pickAsset 根据 GOOS/GOARCH 找 asset
// 支持命名如: mxgjgt-linux-amd64, mxgjgt-linux-amd64.tar.gz, mxgjgt-windows-amd64.exe.zip
func pickAsset(assets []ghAsset, goos, goarch string) *ghAsset {
	var osTag, archTag, ext string
	switch goos {
	case "linux":
		osTag = "linux"
		ext = ""
	case "windows":
		osTag = "windows"
		ext = ".exe"
	case "darwin":
		osTag = "darwin"
	}
	switch goarch {
	case "amd64":
		archTag = "amd64"
	case "arm64":
		archTag = "arm64"
	case "arm":
		archTag = "arm"
	}

	// 优先精确匹配
	for i := range assets {
		name := strings.ToLower(assets[i].Name)
		if strings.Contains(name, osTag) && strings.Contains(name, archTag) {
			// 排除源码包 / 校验文件
			if strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
				return &assets[i]
			}
			if ext != "" && strings.Contains(name, ext) {
				return &assets[i]
			}
			// 无扩展名或 .exe 直接匹配
			return &assets[i]
		}
	}

	// 次优先：宽松匹配
	for i := range assets {
		name := strings.ToLower(assets[i].Name)
		if strings.Contains(name, osTag) && strings.Contains(name, archTag) {
			return &assets[i]
		}
	}
	return nil
}

func pickMirror(mirrors []string, originURL string) string {
	for _, m := range mirrors {
		if strings.Contains(m, "{origin}") {
			return strings.ReplaceAll(m, "{origin}", originURL)
		}
		// 直接代理前缀模式 https://ghproxy.com/https://github.com/xxx
		if strings.HasSuffix(m, "/") {
			return m + originURL
		}
	}
	return ""
}

func downloadToTemp(url string, expectedSize int64) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "mxgjgt-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 302 {
		return "", fmt.Errorf("下载 %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "mxgjgt-update-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()

	written, err := io.Copy(tmp, resp.Body)
	tmp.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	if expectedSize > 0 && written < expectedSize*80/100 {
		os.Remove(tmpPath)
		return "", fmt.Errorf("下载不完整 (%d/%d)", written, expectedSize)
	}

	return tmpPath, nil
}

func replaceBinary(tmpPath string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	self = filepath.Clean(self)

	// 重命名旧版本为 .bak
	bakPath := self + ".bak"
	os.Remove(bakPath)
	os.Rename(self, bakPath)

	// 新文件重命名为正式位置
	if err := os.Rename(tmpPath, self); err != nil {
		// 原子移动失败（跨设备等），回退用 copy
		in, _ := os.Open(tmpPath)
		out, _ := os.Create(self)
		io.Copy(out, in)
		in.Close()
		out.Close()
		os.Remove(tmpPath)
	}

	// 设为可执行
	if runtime.GOOS != "windows" {
		os.Chmod(self, 0755)
	}

	return self, nil
}

func execSelf(path string) {
	if runtime.GOOS == "windows" {
		// Windows 用 ShellExecute 自重启
		cmd := exec.Command(path)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Start()
		os.Exit(0)
		return
	}
	exec := exec.Command(path)
	exec.Stdout = os.Stdout
	exec.Stderr = os.Stderr
	exec.Stdin = os.Stdin
	exec.Env = os.Environ()
	if err := exec.Start(); err != nil {
		logUpdate("自重启失败: %v", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func logUpdate(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("[Updater] %s\n", msg)
	appendLog(msg)
}

func appendLog(msg string) {
	if config.LogsDir() == "" {
		return
	}
	path := filepath.Join(config.LogsDir(), "update.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
}
