package service

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/utils"

	"github.com/PuerkitoBio/goquery"
)

// SearchResource 在所有启用的资源站中搜索影视
func SearchResource(title, episode string) []model.SearchResult {
	sites := config.GetSites()
	var results []model.SearchResult

	for _, site := range sites {
		if !site.Enabled {
			continue
		}
		kw := title
		if episode != "" {
			kw = title + " 第" + episode + "集"
		}
		kw = strings.TrimSpace(kw)

		result, err := searchOneSite(site, kw)
		if err != nil || len(result) == 0 {
			continue
		}
		results = append(results, result...)
	}

	return results
}

// searchOneSite 从单个资源站搜索
func searchOneSite(site model.ResourceSite, keyword string) ([]model.SearchResult, error) {
	var encoded string
	switch strings.ToLower(site.Encode) {
	case "gb2312", "gbk":
		encoded = url.QueryEscape(keyword)
	default:
		encoded = url.QueryEscape(keyword)
	}

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
			if strings.Contains(site.TitleAttr, " @") {
				// 复合选择器: 选择器@属性
				parts := strings.SplitN(site.TitleAttr, "@", 2)
				title = s.Find(parts[0]).AttrOr(parts[1], "")
			} else {
				title = s.Find(site.TitleAttr).Text()
			}
		} else {
			title = s.Text()
		}

		if site.LinkAttr != "" {
			if strings.Contains(site.LinkAttr, " @") {
				parts := strings.SplitN(site.LinkAttr, "@", 2)
				link = s.Find(parts[0]).AttrOr(parts[1], "")
			} else if strings.Contains(site.LinkAttr, "@") {
				parts := strings.SplitN(site.LinkAttr, "@", 2)
				link = s.AttrOr(parts[1], "")
			} else {
				link = s.Find(site.LinkAttr).AttrOr("href", "")
			}
		}

		title = strings.TrimSpace(title)
		link = strings.TrimSpace(link)
		if title == "" || link == "" {
			return
		}

		// 相对链接转绝对
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

// 备用的通用 HTTP 辅助（避免循环依赖）
func init() {
	http.DefaultClient.Timeout = 10 * time.Second
	_ = io.Discard
}
