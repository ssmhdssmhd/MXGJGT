package service

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"sync"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/utils"

	"github.com/PuerkitoBio/goquery"
)

// SearchResource 在所有启用的资源站中搜索影视（并发）
func SearchResource(title, episode string) []model.SearchResult {
	sites := config.GetSites()
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []model.SearchResult
	)

	for _, site := range sites {
		if !site.Enabled {
			continue
		}
		wg.Add(1)
		go func(site model.ResourceSite) {
			defer wg.Done()
			kw := strings.TrimSpace(title)
			got, err := searchOneSite(site, kw)
			if err != nil || len(got) == 0 {
				return
			}
			mu.Lock()
			results = append(results, got...)
			mu.Unlock()
		}(site)
	}
	wg.Wait()

	return results
}

// searchOneSite 根据资源站类型分发
func searchOneSite(site model.ResourceSite, keyword string) ([]model.SearchResult, error) {
	if IsAppleCMSType(site.Type) {
		return searchAppleCMS(site, keyword)
	}
	return searchHTML(site, keyword)
}

// SearchOneSite 在单个资源站内搜索（供后台「测试 / 搜索」使用）
func SearchOneSite(site model.ResourceSite, keyword string) ([]model.SearchResult, error) {
	return searchOneSite(site, keyword)
}

// KeywordVariants 生成关键词降级候选（由严到松）：
// 原名 → 去空格 → 去「第X季/第X部/粤语/国语」等后缀 → 取主名
//
// 资源站关键词搜索多为精确分词，官方剧名常带「空格 + 季/语言」后缀
// （如腾讯返回「开心锤锤 第一季」，资源站只有「开心锤锤」），
// 直接用原剧名会 0 命中，需要逐级放宽后再搜。
func KeywordVariants(name string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}

	norm := strings.Join(strings.Fields(name), " ") // 压缩连续空白
	add(norm)

	noSpace := strings.ReplaceAll(norm, " ", "")
	add(noSpace)

	stripped := stripTitleSuffix(noSpace)
	add(stripped)

	// 主名：第一个空格 / 分隔符之前的片段
	for _, s := range []string{norm, stripped} {
		if idx := strings.IndexAny(s, " /|\\-_·:：,，"); idx > 0 {
			add(s[:idx])
		}
	}
	return out
}

// IsAppleCMSType 判断资源站类型是否为苹果CMS 采集接口
// 兼容 applecms / applemcms / acms / cms 几种写法
func IsAppleCMSType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "applecms", "applemcms", "acms", "cms":
		return true
	}
	return false
}

// ========== 苹果CMS 接口搜索 ==========

func searchAppleCMS(site model.ResourceSite, keyword string) ([]model.SearchResult, error) {
	encoded := url.QueryEscape(keyword)
	target := strings.ReplaceAll(site.URL, "{keyword}", encoded)

	body, err := utils.GetHTML(target, nil)
	if err != nil {
		return nil, err
	}

	var resp model.AcmsListResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 1 || len(resp.List) == 0 {
		return nil, nil
	}

	var results []model.SearchResult
	for _, v := range resp.List {
		r := model.SearchResult{
			Title:      v.VodName,
			Site:       site.Name,
			ResourceID: itoa(v.VodID),
			Type:       v.TypeName,
			Remark:     v.VodRemark,
			PlayFrom:   v.VodPlayFrom,
			PlayURL:    v.VodPlayURL,
		}

		// 构造详情 URL
		if site.DetailURL != "" {
			r.URL = strings.ReplaceAll(site.DetailURL, "{id}", itoa(v.VodID))
		}

		results = append(results, r)
	}

	// 按标题匹配度排序（精确匹配优先）
	sort.SliceStable(results, func(i, j int) bool {
		return scoreMatch(keyword, results[i].Title) > scoreMatch(keyword, results[j].Title)
	})

	// 标记精确匹配
	if len(results) > 0 {
		best := results[0]
		best.Matched = scoreMatch(keyword, best.Title) >= 80
		results[0] = best
	}

	return results, nil
}

// ========== HTML 资源站搜索 ==========

func searchHTML(site model.ResourceSite, keyword string) ([]model.SearchResult, error) {
	encoded := url.QueryEscape(keyword)
	target := strings.ReplaceAll(site.URL, "{keyword}", encoded)

	html, err := utils.GetHTML(target, nil)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var results []model.SearchResult
	doc.Find(site.Selector).Each(func(i int, s *goquery.Selection) {
		var title, link string
		if site.TitleAttr != "" {
			if strings.Contains(site.TitleAttr, "@") {
				parts := strings.SplitN(site.TitleAttr, "@", 2)
				title = s.Find(strings.TrimSpace(parts[0])).AttrOr(strings.TrimSpace(parts[1]), "")
			} else {
				title = s.Find(site.TitleAttr).Text()
			}
		} else {
			title = s.Text()
		}

		if site.LinkAttr != "" {
			if strings.Contains(site.LinkAttr, "@") {
				parts := strings.SplitN(site.LinkAttr, "@", 2)
				link = s.Find(strings.TrimSpace(parts[0])).AttrOr(strings.TrimSpace(parts[1]), "")
			} else {
				link = s.Find(site.LinkAttr).AttrOr("href", "")
			}
		}

		title = strings.TrimSpace(title)
		link = strings.TrimSpace(link)
		if title == "" || link == "" {
			return
		}

		if strings.HasPrefix(link, "/") {
			if u, err := url.Parse(target); err == nil {
				link = u.Scheme + "://" + u.Host + link
			}
		}

		results = append(results, model.SearchResult{
			Title: title,
			URL:   link,
			Site:  site.Name,
		})
	})

	return results, nil
}

// ========== 详情获取 + 播放列表解析 ==========

// FetchDetail 获取资源站详情（含播放列表）
func FetchDetail(site model.ResourceSite, resourceID string) (*model.SearchResult, []model.EpisodePlay, error) {
	if !IsAppleCMSType(site.Type) || site.DetailURL == "" || resourceID == "" {
		return nil, nil, nil
	}

	target := strings.ReplaceAll(site.DetailURL, "{id}", resourceID)
	body, err := utils.GetHTML(target, nil)
	if err != nil {
		return nil, nil, err
	}

	var resp struct {
		Code int          `json:"code"`
		List []model.AcmsVideo `json:"list"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, nil, err
	}
	if resp.Code != 1 || len(resp.List) == 0 {
		return nil, nil, nil
	}

	v := resp.List[0]
	res := &model.SearchResult{
		Title:      v.VodName,
		Site:       site.Name,
		ResourceID: itoa(v.VodID),
		Type:       v.TypeName,
		Remark:     v.VodRemark,
		PlayFrom:   v.VodPlayFrom,
		PlayURL:    v.VodPlayURL,
	}

	// 解析播放列表
	episodes := parsePlayList(v.VodPlayURL)

	return res, episodes, nil
}

// parsePlayList 解析苹果CMS播放列表
// 格式: 源名1$$$播放1#播放2#播放3$$$源名2$$$播放...
// 每项: 集数$URL
func parsePlayList(raw string) []model.EpisodePlay {
	if raw == "" {
		return nil
	}

	var episodes []model.EpisodePlay

	// 按 $$$ 分割多个播放源
	sources := strings.Split(raw, "$$$")
	for _, src := range sources {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}

		// 按 # 分割每集
		parts := strings.Split(src, "#")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			kv := strings.SplitN(part, "$", 2)
			if len(kv) != 2 {
				continue
			}
			episodes = append(episodes, model.EpisodePlay{
				Episode: strings.TrimSpace(kv[0]),
				URL:     strings.TrimSpace(kv[1]),
			})
		}

		// 只取第一个源（最清晰）
		break
	}

	return episodes
}

// ========== 工具函数 ==========

// scoreMatch 计算关键字与标题的匹配度 (0-100)
func scoreMatch(keyword, title string) int {
	if keyword == "" || title == "" {
		return 0
	}
	k := strings.ToLower(strings.ReplaceAll(keyword, " ", ""))
	t := strings.ToLower(strings.ReplaceAll(title, " ", ""))

	// 完全相等
	if k == t {
		return 100
	}
	// 标题包含关键字
	if strings.Contains(t, k) {
		return 90
	}
	// 关键字包含标题
	if strings.Contains(k, t) {
		return 85
	}
	// 包含主要关键词
	kwords := splitKeywords(keyword)
	if len(kwords) > 0 {
		hit := 0
		for _, kw := range kwords {
			if kw != "" && strings.Contains(t, kw) {
				hit++
			}
		}
		if hit == len(kwords) && len(kwords) > 0 {
			return 75
		}
		return hit * 15
	}
	return 0
}

func splitKeywords(s string) []string {
	// 按常见分隔符 + 单字切分，提取 2+ 字词
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '/' || r == '\\' || r == '_' || r == '-' || r == ':' || r == '|'
	})
	if len(fields) == 0 {
		fields = []string{s}
	}
	return fields
}

// itoa int -> string
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
