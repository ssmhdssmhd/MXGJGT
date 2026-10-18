// Package service — 播放地址解析（后台内嵌播放用）
//
// 采集站给的 vod_play_url 是「播放页」地址（HTML），不能直接喂给 video 标签。
// 好在这些播放页（苹果CMS 通用模板）会把直链 m3u8 写在页面里，例如：
//
//	<script>
//	  const url = "/20251029/2162_86e78499/index.m3u8?sign=xxx";
//	</script>
//
// 本文件负责把播放页翻成直链 m3u8，供后台用 hls.js 内嵌播放。
package service

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"mxgjgt/utils"
)

// 播放页里出现直链的几种常见写法（按优先级取第一个命中 .m3u8 的）
var playURLPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:const|let|var)\s+url\s*=\s*["']([^"']+\.m3u8[^"']*)["']`),
	regexp.MustCompile(`(?i)["']url["']\s*:\s*["']([^"']+\.m3u8[^"']*)["']`),
	regexp.MustCompile(`(?i)["']((?:https?:)?//[^"']+\.m3u8[^"']*)["']`),
	regexp.MustCompile(`(?i)["'](/[^"']+\.m3u8[^"']*)["']`),
}

// ExtractM3U8 从播放页 HTML 里提取直链 m3u8
func ExtractM3U8(pageHTML string) string {
	for _, re := range playURLPatterns {
		if m := re.FindStringSubmatch(pageHTML); len(m) >= 2 {
			if u := strings.TrimSpace(m[1]); u != "" {
				return u
			}
		}
	}
	return ""
}

// ResolvePlayStream 抓取播放页并解析出可直连播放的 m3u8 地址
// 返回的地址已转成绝对地址（补全 host / 协议）
func ResolvePlayStream(pageURL string) (string, error) {
	page, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil || (page.Scheme != "http" && page.Scheme != "https") || page.Host == "" {
		return "", fmt.Errorf("播放页地址无效")
	}

	html, err := utils.GetHTML(page.String(), map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36",
		"Referer":    page.Scheme + "://" + page.Host + "/",
	})
	if err != nil {
		return "", err
	}

	raw := ExtractM3U8(html)
	if raw == "" {
		return "", fmt.Errorf("未能从播放页解析出 m3u8（可改用「新窗口播放」）")
	}

	// 相对地址补全为绝对地址（保留 query 里的 sign 等参数）
	if strings.HasPrefix(raw, "//") {
		return page.Scheme + ":" + raw, nil
	}
	if strings.HasPrefix(raw, "/") || !strings.Contains(raw, "://") {
		ref, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		return page.ResolveReference(ref).String(), nil
	}
	return raw, nil
}
