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

// ========== 腾讯视频 ==========

// TencentInfo 腾讯视频 cover 信息
type TencentInfo struct {
	Title string `json:"title"`
	Year  string `json:"year"`
}

var tencentCIDRe = regexp.MustCompile(`/(?:cover|page)/([A-Za-z0-9]{8,})(?:\.html|/|$)`)

// tencentCIDValueRe query 参数形式的 cid 校验（纯字母数字，长度合理）
var tencentCIDValueRe = regexp.MustCompile(`^[A-Za-z0-9]{6,}$`)

// extractTencentCID 从 v.qq.com 链接里提取 cover id
// 支持两类常见形式：
//  1. 路径形式： https://v.qq.com/x/cover/{cid}/{vid}.html 、 https://v.qq.com/x/page/{cid}.html
//  2. 参数形式： https://m.v.qq.com/x/m/play?cid={cid}&vid={vid} 、 /play.html?cid={cid}&vid={vid}
func extractTencentCID(u *url.URL) string {
	// 先看 query 参数（移动端播放页 / 分享链接的 cid 都在这里）
	q := u.Query()
	for _, k := range []string{"cid", "cover_id", "coverId", "cId"} {
		if v := strings.TrimSpace(q.Get(k)); v != "" && tencentCIDValueRe.MatchString(v) {
			return v
		}
	}
	// 再看路径（含完整 URL 串兜底）
	if m := tencentCIDRe.FindStringSubmatch(u.Path); len(m) >= 2 {
		return m[1]
	}
	if m := tencentCIDRe.FindStringSubmatch(u.String()); len(m) >= 2 {
		return m[1]
	}
	return ""
}

// FetchTencentInfo 调用腾讯公开接口按 cid 拉剧名
func FetchTencentInfo(cid string) (*TencentInfo, error) {
	target := "https://node.video.qq.com/x/api/float_vinfo2?cid=" + cid
	body, err := utils.GetHTML(target, map[string]string{
		"User-Agent": "Mozilla/5.0",
		"Referer":    "https://v.qq.com/",
	})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		C TencentInfo `json:"c"`
	}
	if err := json.Unmarshal([]byte(body), &wrapper); err != nil {
		return nil, err
	}
	if strings.TrimSpace(wrapper.C.Title) == "" {
		return nil, fmt.Errorf("tencent api empty title")
	}
	return &wrapper.C, nil
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

// titleSuffixes 剧名常见的季 / 部 / 语言后缀（不含空格写法，便于同时覆盖两种形式）
var titleSuffixes = []string{
	"第一季", "第二季", "第三季", "第四季", "第五季", "第六季",
	"第1季", "第2季", "第3季", "第4季", "第5季", "第6季",
	"第一部", "第二部", "第三部", "第四部", "第五部",
	"粤语版", "国语版", "普通话版", "原声版", "电影版",
	"粤语", "国语", "普通话",
	// 合集 / 特辑类后缀（腾讯「斗罗大陆合集篇」→ 资源站「斗罗大陆」）
	"合集篇", "合集版", "合集", "完整版", "纯享版", "精编版", "特辑版",
}

// stripTitleSuffix 去掉剧名末尾的「第X季 / 第X部 / 语言」等后缀
// 用于资源站搜索关键词降级（腾讯返回「开心锤锤 第一季」，资源站只有「开心锤锤」）
func stripTitleSuffix(t string) string {
	t = strings.TrimSpace(t)
	for changed := true; changed; {
		changed = false
		for _, sep := range titleSuffixes {
			for _, form := range []string{sep, " " + sep, "·" + sep, "-" + sep} {
				if strings.HasSuffix(t, form) && len(t) > len(form) {
					t = strings.TrimSpace(strings.TrimSuffix(t, form))
					changed = true
				}
			}
		}
	}
	return t
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

	// 1.5 腾讯视频 — cover cid API
	if strings.Contains(host, "v.qq.com") || strings.Contains(host, "film.qq.com") {
		if cid := extractTencentCID(u); cid != "" {
			if info, err := FetchTencentInfo(cid); err == nil && info.Title != "" {
				return info.Title, "", "tencent-api"
			}
		}
	}

	// 2. 尝试 HTML og:title / meta (对非反爬站有效)
	if htmlTitle, htmlEp := fetchTitleFromPage(u.String()); isGoodTitle(htmlTitle) {
		return htmlTitle, htmlEp, "html-meta"
	}

	// 2.5 方案1：无头浏览器渲染（覆盖爱奇艺/优酷/芒果等强反爬站）
	if BrowserAvailable() {
		if rt, re, err := RenderOfficialTitle(u.String()); err == nil && isGoodTitle(rt) {
			return rt, re, "headless-render"
		}
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
