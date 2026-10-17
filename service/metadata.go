// Package service — 第三方元数据 API
package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"mxgjgt/utils"
)

// ========== 哔哩哔哩 ==========

// BilibiliVideo B站视频结构（精简）
type BilibiliVideo struct {
	Title  string `json:"title"`
	BVID   string `json:"bvid"`
	AID    int    `json:"aid"`
	CID    int    `json:"cid"`
	TID    int    `json:"tid"`
	Pages  int    `json:"videos"`
	Desc   string `json:"desc"`
}

// FetchBilibiliInfo 从 BV 号拉取 B站视频标题
func FetchBilibiliInfo(bvid string) (*BilibiliVideo, error) {
	target := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?bvid=%s", bvid)
	body, err := utils.GetHTML(target, map[string]string{
		"User-Agent": "Mozilla/5.0",
		"Referer":    "https://www.bilibili.com/",
	})
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Code int           `json:"code"`
		Data BilibiliVideo `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &wrapper); err != nil {
		return nil, err
	}
	if wrapper.Code != 0 {
		return nil, fmt.Errorf("bilibili api code=%d", wrapper.Code)
	}
	return &wrapper.Data, nil
}

var bvRe = regexp.MustCompile(`BV[a-zA-Z0-9]{10}`)

func extractBVID(u *url.URL) string {
	if m := bvRe.FindString(u.Path); m != "" {
		return m
	}
	if m := bvRe.FindString(u.String()); m != "" {
		return m
	}
	return ""
}

// ========== 豆瓣 ==========

// DoubanSuggest 豆瓣搜索建议
type DoubanSuggest struct {
	Title    string `json:"title"`
	SubTitle string `json:"sub_title"`
	Type     string `json:"type"` // movie / tv ...
	Year     string `json:"year"`
	Episode  string `json:"episode"`
	URL      string `json:"url"`
	ID       string `json:"id"`
}

// SearchDouban 豆瓣 j/subject_suggest 搜索（公开免签）
func SearchDouban(keyword string) []DoubanSuggest {
	target := "https://movie.douban.com/j/subject_suggest?q=" + url.QueryEscape(keyword)
	body, err := utils.GetHTML(target, map[string]string{
		"User-Agent": "Mozilla/5.0",
		"Referer":    "https://movie.douban.com/",
	})
	if err != nil {
		return nil
	}
	var list []DoubanSuggest
	json.Unmarshal([]byte(body), &list)
	return list
}

// PickBestDouban 从豆瓣候选里挑最佳剧名
// 优先精确匹配 keyword 的，然后挑最热门的
func PickBestDouban(keyword string, candidates []DoubanSuggest) string {
	if len(candidates) == 0 {
		return ""
	}

	// 1. 精确匹配 title
	for _, c := range candidates {
		if c.Title == keyword {
			return stripDoubanSuffix(c.Title)
		}
	}
	// 2. title 开头就是 keyword
	for _, c := range candidates {
		if strings.HasPrefix(c.Title, keyword) {
			return stripDoubanSuffix(c.Title)
		}
	}
	// 3. keyword 在 title 里
	for _, c := range candidates {
		if strings.Contains(c.Title, keyword) {
			return stripDoubanSuffix(c.Title)
		}
	}
	// 4. 都不行返回第一条
	return stripDoubanSuffix(candidates[0].Title)
}

func stripDoubanSuffix(t string) string {
	// "庆余年 第一季" → "庆余年"
	for _, sep := range []string{" 第一季", " 第二季", " 第三季", " 第四季", " 第五季",
		" 第1季", " 第2季", " 第3季", " 第一部", " 第二部", " 电影", " (原版)"} {
		t = strings.ReplaceAll(t, sep, "")
	}
	return strings.TrimSpace(t)
}

// ========== 官方链接兜底解析 ==========

// ResolveOfficialTitle 多层降级从官方链接拿到剧名
// 返回: 剧名, 集数 (可能为空), 来源说明
func ResolveOfficialTitle(u *url.URL) (title, episode, source string) {
	host := u.Host
	path := u.Path

	// 1. B站 — 直接调 BV 号
	if strings.Contains(host, "bilibili.com") || strings.Contains(host, "b23.tv") {
		if bvid := extractBVID(u); bvid != "" {
			if info, err := FetchBilibiliInfo(bvid); err == nil && info.Title != "" {
				return info.Title, "", "bilibili-api"
			}
		}
	}

	// 2. 尝试 HTML og:title / meta (对非反爬站有效)
	if htmlTitle, htmlEp := fetchTitleFromPage(u.String()); isGoodTitle(htmlTitle) {
		return htmlTitle, htmlEp, "html-meta"
	}

	// 3. 从 URL path 里找所有可能的 segment，去豆瓣逐个搜
	segments := extractChineseSegments(path)
	for _, seg := range segments {
		if isSuspiciousTitle(seg) {
			continue
		}
		candidates := SearchDouban(seg)
		if best := PickBestDouban(seg, candidates); best != "" {
			return best, "", "douban-search"
		}
	}

	// 4. URL query 里所有非空参数值都试一遍
	q := u.Query()
	for _, k := range []string{"wd", "keyword", "q", "name", "title", "series", "cover", "coverName", "series_name"} {
		if v := q.Get(k); v != "" && !isSuspiciousTitle(v) {
			candidates := SearchDouban(v)
			if best := PickBestDouban(v, candidates); best != "" {
				return best, "", "douban-search-query"
			}
		}
	}

	// 5. 实在不行，把所有非 ID 的 path segment 拼起来当关键词试
	var meaningful []string
	for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
		if !isIDLike(seg) && len(seg) >= 2 {
			meaningful = append(meaningful, seg)
		}
	}
	if len(meaningful) > 0 {
		combined := strings.Join(meaningful, " ")
		if !isSuspiciousTitle(combined) {
			candidates := SearchDouban(combined)
			if best := PickBestDouban(combined, candidates); best != "" {
				return best, "", "douban-search-combined"
			}
		}
	}

	return "", "", ""
}

// extractChineseSegments 从 path 里提取包含中文字符的 segment
func extractChineseSegments(path string) []string {
	var result []string
	for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
		if containsChinese(seg) && !isSuspiciousTitle(seg) {
			result = append(result, seg)
		}
	}
	return result
}

func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// 让 caller 也能访问 (parser.go 里已定义同名函数，但它是小写)
func init() {}
