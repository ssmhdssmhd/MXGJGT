// Package handler HTTP 路由处理
package handler

import (
	"net/http"
	"os"
	"strings"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/service"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
func RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/clean/", handleClean)
		api.GET("/search/", handleSearch) // 纯搜索接口，输入关键词
		api.GET("/version", handleVersion)
		api.POST("/update", handleUpdate)
	}

	// 其他所有路径 -> 伪装 404
	r.NoRoute(handleNotFound)
}

// handleClean 核心清洗 API
// GET /api/clean/?url=<待解析链接>
func handleClean(c *gin.Context) {
	targetURL := strings.TrimSpace(c.Query("url"))

	if targetURL == "" {
		c.JSON(http.StatusOK, model.Response{
			Code: 404,
			Msg:  "无url参数",
		})
		return
	}

	// 1. 识别官方网站 + 提取影视信息
	offSite, err := service.ParseOfficial(targetURL)
	if err != nil || offSite == nil {
		c.JSON(http.StatusOK, model.Response{
			Code: 400,
			Msg:  "无法识别的链接格式: " + errMsg(err),
			Data: gin.H{"url": targetURL},
		})
		return
	}

	// 2. 从资源站搜索
	results := []model.SearchResult{}
	var bestMatch *model.SearchResult
	var episodes []model.EpisodePlay

	if offSite.Title != "" {
		results = service.SearchResource(offSite.Title, offSite.Episode)
		bestMatch = pickBestMatch(results, offSite.Title)
		if bestMatch != nil && bestMatch.ResourceID != "" {
			for _, rs := range config.GetSites() {
				if rs.Enabled && strings.EqualFold(rs.Type, "applecms") && bestMatch.Site == rs.Name {
					var detail *model.SearchResult
					detail, episodes, _ = service.FetchDetail(rs, bestMatch.ResourceID)
					episodes = pickTargetEpisode(detail, episodes, offSite.Episode)
					break
				}
			}
		}
	}

	// 3. 确定 replacement
	replacement := ""
	if bestMatch != nil {
		if len(episodes) > 0 {
			// 如果指定了集数并找到了对应播放链接
			replacement = episodes[0].URL
		} else if bestMatch.URL != "" {
			replacement = bestMatch.URL
		} else {
			replacement = bestMatch.PlayURL
		}
	}

	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "success",
		Data: model.CleanResult{
			Original:    targetURL,
			Title:       offSite.Title,
			Episode:     offSite.Episode,
			Site:        offSite.Name,
			Replacement: replacement,
			Results:     results,
			BestMatch:   bestMatch,
			Episodes:    episodes,
		},
	})
}

// handleSearch 纯关键词搜索
// GET /api/search/?wd=庆余年
func handleSearch(c *gin.Context) {
	wd := strings.TrimSpace(c.Query("wd"))
	episode := strings.TrimSpace(c.Query("episode"))

	if wd == "" {
		c.JSON(http.StatusOK, model.Response{
			Code: 400,
			Msg:  "缺少 wd 关键词参数",
		})
		return
	}

	results := service.SearchResource(wd, episode)
	bestMatch := pickBestMatch(results, wd)

	episodes := []model.EpisodePlay{}
	if bestMatch != nil && bestMatch.ResourceID != "" {
		for _, rs := range config.GetSites() {
			if rs.Enabled && strings.EqualFold(rs.Type, "applecms") && bestMatch.Site == rs.Name {
				var detail *model.SearchResult
				detail, episodes, _ = service.FetchDetail(rs, bestMatch.ResourceID)
				episodes = pickTargetEpisode(detail, episodes, episode)
				break
			}
		}
	}

	replacement := ""
	if bestMatch != nil {
		if len(episodes) > 0 {
			replacement = episodes[0].URL
		} else if bestMatch.URL != "" {
			replacement = bestMatch.URL
		}
	}

	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "success",
		Data: gin.H{
			"keyword":     wd,
			"episode":     episode,
			"replacement": replacement,
			"best_match":  bestMatch,
			"episodes":    episodes,
			"results":     results,
		},
	})
}

// pickBestMatch 从搜索结果中选最佳匹配
func pickBestMatch(results []model.SearchResult, keyword string) *model.SearchResult {
	if len(results) == 0 {
		return nil
	}
	// results 已按匹配度排序，第一条就是最优
	// 如果第一条匹配度较高(>=70)，直接返回
	if len(results) > 0 && (results[0].Matched || keyword == "" || len(results) <= 3) {
		return &results[0]
	}
	// 否则仍然返回第一条（按 scoreMatch 排序的）
	return &results[0]
}

// pickTargetEpisode 在 episodes 中挑选用户指定的那一集
func pickTargetEpisode(res *model.SearchResult, eps []model.EpisodePlay, targetEpisode string) []model.EpisodePlay {
	_ = res // 暂时忽略 res
	if len(eps) == 0 {
		return nil
	}
	if targetEpisode == "" {
		return eps // 返回全集
	}
	// 找到匹配集数
	for _, ep := range eps {
		if strings.Contains(ep.Episode, targetEpisode) {
			return []model.EpisodePlay{ep}
		}
	}
	return eps
}

// handleVersion 返回版本信息
func handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "success",
		Data: gin.H{
			"version":  Version,
			"build":    BuildTime,
			"repo_url": config.GetConfig().Updater.RepoURL,
			"branch":   config.GetConfig().Updater.Branch,
			"updater":  config.GetConfig().Updater.Enabled,
		},
	})
}

// handleUpdate 手动触发更新
func handleUpdate(c *gin.Context) {
	cfg := config.GetConfig()
	if cfg.Security.APIKey != "" {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("key")
		}
		if apiKey != cfg.Security.APIKey {
			c.JSON(http.StatusOK, model.Response{
				Code: 403,
				Msg:  "API Key 无效",
			})
			return
		}
	}

	msg := service.ManualUpdate()
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  msg,
	})
}

// handleNotFound 伪装 404
func handleNotFound(c *gin.Context) {
	cfg := config.GetConfig()
	if cfg.Server.NotFoundHTML != "" {
		if data, err := os.ReadFile(cfg.Server.NotFoundHTML); err == nil {
			c.Data(http.StatusNotFound, "text/html; charset=utf-8", data)
			return
		}
	}
	c.JSON(http.StatusNotFound, model.Response{
		Code: 404,
		Msg:  "页面未找到",
	})
}

func errMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
