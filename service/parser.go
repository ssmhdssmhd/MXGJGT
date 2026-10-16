// Package service 业务逻辑层
package service

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"mxgjgt/model"
	"mxgjgt/utils"

	"github.com/PuerkitoBio/goquery"
)

// officialSiteBlacklist HTML 反爬标题黑名单 —— 命中就丢弃
var titleBlacklist = []string{
	"腾讯视频", "爱奇艺", "优酷", "芒果TV", "搜狐视频", "B站", "哔哩哔哩",
	"矩形备份", "热搜榜", "那条视频不见了", "页面不存在", "404",
	"首页", "登录", "注册", "下载客户端",
}

// episodePatterns 集数正则
var officialSites = []model.VideoSite{
	{
		Name:    "腾讯视频",
		Domain:  "v.qq.com",
		Aliases: []string{"v.qq.com", "film.qq.com", "video.qq.com", "m.v.qq.com"},
	},
	{
		Name:    "爱奇艺",
		Domain:  "iqiyi.com",
		Aliases: []string{"iqiyi.com", "www.iqiyi.com", "m.iqiyi.com", "v.iqiyi.com"},
	},
	{
		Name:    "优酷",
		Domain:  "youku.com",
		Aliases: []string{"youku.com", "www.youku.com", "v.youku.com", "m.youku.com"},
	},
	{
		Name:    "芒果TV",
		Domain:  "mgtv.com",
		Aliases: []string{"mgtv.com", "www.mgtv.com", "v.mgtv.com", "m.mgtv.com"},
	},
	{
		Name:    "搜狐视频",
		Domain:  "tv.sohu.com",
		Aliases: []string{"tv.sohu.com", "sohu.com"},
	},
	{
		Name:    "乐视视频",
		Domain:  "letv.com",
		Aliases: []string{"letv.com", "www.letv.com", "video.le.com"},
	},
	{
		Name:    "哔哩哔哩",
		Domain:  "bilibili.com",
		Aliases: []string{"bilibili.com", "www.bilibili.com", "m.bilibili.com", "b23.tv"},
	},
}

// episodePatterns 集数正则
var episodePatterns = []*regexp.Regexp{
	regexp.MustCompile(`[第T](\d+)[集话部]`),   // 第XX集 / TXX
	regexp.MustCompile(`[Ee][Pp]?(\d+)`),      // EPXX / eXX
	regexp.MustCompile(`(\d+)[集话部]`),        // XX集
	regexp.MustCompile(`第\s*(\d+)\s*[集话部期]`), // 带空格
	regexp.MustCompile(`_(\d{2,4})(?:[_/])`),  // _01_ / _12/
}

// 页面 HTML 中提取标题的 meta 选择器
var titleMetaSelectors = []string{
	`meta[property="og:title"]`,
	`meta[name="og:title"]`,
	`meta[name="description"]`,
	`meta[property="og:description"]`,
}

// ParseOfficial 识别官方视频网站并提取影视信息
func ParseOfficial(rawURL string) (*model.VideoSite, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, nil
	}

	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	host := parsed.Host
	path := parsed.Path

	// 用户手动覆盖（最高优先级）
	userWD := parsed.Query().Get("wd")
	if userWD == "" {
		userWD = parsed.Query().Get("keyword")
	}

	// 匹配官方网站
	for _, site := range officialSites {
		for _, alias := range site.Aliases {
			if strings.Contains(host, alias) {
				result := &model.VideoSite{
					Name:     site.Name,
					Domain:   site.Domain,
					Aliases:  site.Aliases,
					Original: rawURL,
				}

				// 1. 用户手动 wd= 直接用（跳过一切自动解析）
				if userWD != "" {
					result.Title = cleanTitle(userWD)
					result.Episode = extractEpisode(path, rawURL)
					return result, nil
				}

				// 2. 先从 URL 路径提取
				result.Title = extractTitleFromURL(parsed)
				result.Episode = extractEpisode(path, rawURL)

				// 3. 抓 HTML 里的 meta 标题 —— 但很多官方站已反爬，需要质量判断
				htmlTitle, htmlEpisode := fetchTitleFromPage(rawURL)
				if isGoodTitle(htmlTitle) {
					result.Title = htmlTitle
					if htmlEpisode != "" {
						result.Episode = htmlEpisode
					}
				}

				// 4. 如果啥都没拿到，返回 error 让用户传 wd=
				if result.Title == "" || isSuspiciousTitle(result.Title) {
					return result, fmt.Errorf("无法从该链接自动提取剧名，请附加参数 ?wd=剧名 例如 /api/clean/?wd=庆余年&url=%s", rawURL)
				}

				return result, nil
			}
		}
	}

	// 未匹配官方站点
	episode := extractEpisode(path, rawURL)
	title := extractTitleFromURL(parsed)

	if userWD != "" {
		title = cleanTitle(userWD)
	}

	// 通用：也尝试抓取页面标题（也需要质量判断）
	if title == "" || isSuspiciousTitle(title) {
		if hTitle, hEp := fetchTitleFromPage(rawURL); isGoodTitle(hTitle) {
			title = hTitle
			if episode == "" {
				episode = hEp
			}
		}
	}

	return &model.VideoSite{
		Name:     "未知来源",
		Domain:   host,
		Original: rawURL,
		Title:    title,
		Episode:  episode,
	}, nil
}

// isGoodTitle 判断 HTML 抓回来的标题质量
// 规则：非空 + 有意义（长度 ≥ 2 且不含黑名单词）
func isGoodTitle(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" || len(t) < 2 {
		return false
	}
	if isSuspiciousTitle(t) {
		return false
	}
	return true
}

// isSuspiciousTitle 判断标题是否像反爬垃圾
func isSuspiciousTitle(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" {
		return true
	}

	// 1. 黑名单关键词命中
	for _, kw := range titleBlacklist {
		if t == kw {
			return true
		}
		if strings.Contains(t, "腾讯视频") && strings.Contains(t, "矩形备份") {
			return true
		}
		if strings.Contains(t, "矩形备份") || strings.Contains(t, "热搜榜") {
			return true
		}
	}

	// 2. 全是英文/数字/下划线的短串 —— 大概率是 URL segment (vid / cover_id / hash)
	if isAllASCII(t) && len(t) < 20 {
		return true
	}

	// 3. 全小写 + 无空格 + 含数字 —— 也像 vid
	if isAllLower(t) && !strings.Contains(t, " ") && hasDigit(t) && len(t) < 25 {
		return true
	}

	return false
}

func isAllASCII(s string) bool {
	for _, c := range s {
		if c > 127 {
			return false
		}
	}
	return true
}

func isAllLower(s string) bool {
	return strings.ToLower(s) == s
}

func hasDigit(s string) bool {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

// extractTitleFromURL 从 URL 中初步提取标题
func extractTitleFromURL(u *url.URL) string {
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	var candidate string
	for _, s := range segments {
		if len(s) > 2 && !isIDLike(s) {
			candidate = s
		}
	}
	q := u.Query()
	for _, key := range []string{"name", "title", "wd", "keyword"} {
		if v := q.Get(key); v != "" {
			candidate = v
		}
	}
	if candidate == "" {
		return ""
	}
	return cleanTitle(candidate)
}

// fetchTitleFromPage 请求页面，从 HTML meta / title 中提取真实标题
func fetchTitleFromPage(targetURL string) (title, episode string) {
	html, err := utils.GetHTML(targetURL, nil)
	if err != nil || html == "" {
		return "", ""
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", ""
	}

	// 1. 优先从 og:title meta 提取
	for _, sel := range titleMetaSelectors {
		if meta := doc.Find(sel); meta.Length() > 0 {
			if t, ok := meta.Attr("content"); ok && t != "" {
				title = t
				break
			}
		}
	}

	// 2. 回退到 <title> 标签
	if title == "" {
		if t := strings.TrimSpace(doc.Find("title").Text()); t != "" {
			title = t
		}
	}

	if title == "" {
		return "", ""
	}

	title = cleanTitle(title)

	// 尝试从标题中提取集数
	for _, pat := range episodePatterns {
		match := pat.FindStringSubmatch(title)
		if len(match) >= 2 {
			episode = match[1]
			break
		}
	}

	// 从标题中剥离集数后缀
	title = stripEpisodeSuffix(title)

	return title, episode
}

// cleanTitle 清理标题
func cleanTitle(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ".html", "")
	s = strings.ReplaceAll(s, ".htm", "")
	// 移除常见站点后缀
	s = regexp.MustCompile(`[-_|].*?官网`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`[-_|].*?在线观看.*$`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`[-_|].*?高清.*$`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`[-_|].*?全集.*$`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`[-_|].*?免费.*$`).ReplaceAllString(s, "")
	// 尾部的站点名清理
	s = strings.TrimSpace(s)
	return s
}

// stripEpisodeSuffix 剥离标题末尾的集数信息
func stripEpisodeSuffix(s string) string {
	for _, pat := range episodePatterns {
		s = pat.ReplaceAllString(s, "")
	}
	// 清理尾部常见装饰
	s = regexp.MustCompile(`\s+(第\s*\d+\s*[集话部期])\s*$`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+(\d+\s*[集话部期])\s*$`).ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	// 去除尾部 - 分隔
	s = strings.TrimRight(s, "-|_ ")
	return s
}

// extractEpisode 从 URL 中提取集数
func extractEpisode(path, rawURL string) string {
	combined := path + "/" + rawURL
	for _, pat := range episodePatterns {
		match := pat.FindStringSubmatch(combined)
		if len(match) >= 2 {
			return match[1]
		}
	}
	return ""
}

func isIDLike(s string) bool {
	if len(s) < 4 {
		return true
	}
	digitCount := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digitCount++
		}
	}
	return digitCount >= len(s)-1
}

// BuildCleanMessage 生成人类可读的修复消息（便于前端展示）

