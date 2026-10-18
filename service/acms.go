package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"mxgjgt/model"
	"mxgjgt/utils"
)

// AutoAnalyzeAppleCMS 给一个苹果CMS采集接口，自动探测 /list /detail /type 端点
// 返回: 资源站配置 + 分类列表
func AutoAnalyzeAppleCMS(givenURL string) (*model.ResourceSite, []model.CIDMapResponse, error) {
	base, name := normalizeCMSBase(givenURL)
	if base == "" {
		return nil, nil, fmt.Errorf("无法解析接口URL")
	}

	// 探测 list 端点
	listURL, err := probeCMSEndpoint(base, "list")
	if err != nil {
		return nil, nil, fmt.Errorf("list 探测失败: %v", err)
	}
	detailURL, err := probeCMSEndpoint(base, "detail")
	if err != nil {
		return nil, nil, fmt.Errorf("detail 探测失败: %v", err)
	}

	// 拉分类列表（cid -> 分类名）
	cids := fetchCMSCIDs(base)

	site := &model.ResourceSite{
		Type:      "applemcms",
		URL:       strings.ReplaceAll(listURL, "{keyword}", url.QueryEscape("庆余年")),
		DetailURL: strings.ReplaceAll(detailURL, "{id}", "35210"),
		Encode:    "utf8",
		Enabled:   true,
	}
	// 占位符恢复
	site.URL = listURL
	site.DetailURL = detailURL

	if name == "" {
		name = "自动探测资源站"
	}
	site.Name = name

	return site, cids, nil
}

// normalizeCMSBase 解析用户给的 URL 为 base endpoint
// 例如 http://caiji.jjzyapi.com/api.php/provide/vod/from/jjm3u8/at/xml/
// 返回 base=http://caiji.jjzyapi.com/api.php/provide/vod  name=自动
func normalizeCMSBase(given string) (base, name string) {
	given = strings.TrimRight(given, "/")

	// 去掉已知查询参数
	if idx := strings.Index(given, "?"); idx >= 0 {
		given = given[:idx]
	}

	// 尝试定位 /provide/vod
	for _, marker := range []string{"/provide/vod", "/api.php", "/index.php"} {
		if pos := strings.Index(given, marker); pos >= 0 {
			base = given[:pos+len(marker)]
			// 从 URL 路径提取 from 参数值（如 jjm3u8）
			rest := given[pos+len(marker):]
			if rest != "" {
				// URL 形如 .../provide/vod/from/jjm3u8/at/xml
				// 找 /from/xxx/
				if from := extractSlashField(rest, "from"); from != "" {
					name = from
				} else if from := extractSlashField(rest, "resource"); from != "" {
					name = from
				}
			}
			return
		}
	}

	// 没找到 marker，保留整个 host + path 前 2 段
	if u, err := url.Parse(given); err == nil {
		base = u.Scheme + "://" + u.Host
		path := strings.TrimRight(filepath.Dir(u.Path), "/")
		if path != "" && path != "/" {
			base += path
		}
	}
	return
}

func extractSlashField(path, key string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == key && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// probeCMSEndpoint 探测 ac=list 或 ac=detail
func probeCMSEndpoint(base, action string) (string, error) {
	var candidates []string
	switch action {
	case "list":
		candidates = []string{
			base + "/?ac=list&wd={keyword}",
			base + "?ac=list&wd={keyword}",
			base + "/?action=list&wd={keyword}",
			base + "?action=list&wd={keyword}",
		}
	case "detail":
		candidates = []string{
			base + "/?ac=detail&ids={id}",
			base + "?ac=detail&ids={id}",
			base + "/?action=detail&ids={id}",
			base + "?action=detail&ids={id}",
		}
	}

	for _, c := range candidates {
		testURL := strings.ReplaceAll(c, "{keyword}", "%E5%96%9C")
		testURL = strings.ReplaceAll(testURL, "{id}", "1")
		body, err := utils.GetHTML(testURL, nil)
		if err != nil {
			continue
		}
		var r struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if json.Unmarshal([]byte(body), &r) == nil && r.Code == 1 {
			return c, nil
		}
	}
	return "", fmt.Errorf("未探测到有效的 %s 端点", action)
}

// fetchCMSCIDs 拉分类列表
func fetchCMSCIDs(base string) []model.CIDMapResponse {
	for _, path := range []string{"/?ac=type", "?ac=type", "/?ac=class", "?ac=class"} {
		body, err := utils.GetHTML(base+path, nil)
		if err != nil {
			continue
		}
		var raw struct {
			Code int `json:"code"`
			List []struct {
				ID   int    `json:"type_id"`
				Name string `json:"type_name"`
			} `json:"list"`
		}
		if json.Unmarshal([]byte(body), &raw) != nil || raw.Code != 1 || len(raw.List) == 0 {
			continue
		}

		// 去重
		seen := map[int]bool{}
		var result []model.CIDMapResponse
		for _, item := range raw.List {
			if !seen[item.ID] {
				seen[item.ID] = true
				var one model.CIDMapResponse
				one.List = append(one.List, struct {
					ID   int    `json:"type_id"`
					Name string `json:"type_name"`
				}{ID: item.ID, Name: item.Name})
				result = append(result, one)
			}
		}
		return result
	}
	return nil
}

// ========= 资源站 测试 / 分类同步 =========

// FetchSiteCIDs 拉取指定资源站的苹果CMS分类列表（id -> name）
func FetchSiteCIDs(site model.ResourceSite) ([]model.CIDItem, error) {
	base, _ := normalizeCMSBase(site.URL)
	if base == "" {
		return nil, fmt.Errorf("无法解析资源站接口地址")
	}
	var out []model.CIDItem
	seen := map[int]bool{}
	for _, r := range fetchCMSCIDs(base) {
		for _, it := range r.List {
			if seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			out = append(out, model.CIDItem{ID: it.ID, Name: it.Name})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("接口未返回分类信息")
	}
	return out, nil
}

// ProbeSite 单资源站连通性测试：用关键词做一次真实搜索，返回耗时 / 条数 / 样例
func ProbeSite(site model.ResourceSite, keyword string) model.SiteTestResult {
	if strings.TrimSpace(keyword) == "" {
		keyword = "庆余年"
	}
	var res model.SiteTestResult

	start := time.Now()
	got, err := SearchOneSite(site, keyword)
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = err.Error()
		return res
	}

	res.OK = true
	res.Count = len(got)
	for i, r := range got {
		if i >= 5 {
			break
		}
		res.Samples = append(res.Samples, r.Title)
	}
	if cids, err := FetchSiteCIDs(site); err == nil {
		res.CIDCount = len(cids)
	}
	return res
}
