// Package handler HTTP 路由处理
package handler

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/service"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册路由
func RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/clean/",  handleClean)
		api.GET("/search/", handleSearch)
		api.GET("/version", handleVersion)

		admin := api.Group("/admin")
		{
			admin.GET("/sites",        handleAdminListSites)
			admin.POST("/site/auto",   handleAdminAutoCMS)
			admin.POST("/site",        handleAdminAddSite)
			admin.PUT("/site/:id",     handleAdminUpdateSite)
			admin.DELETE("/site/:id",  handleAdminDelSite)
			admin.GET("/mappings",     handleAdminListMap)
			admin.POST("/mapping",     handleAdminSetMap)
			admin.POST("/reload",      handleAdminReload)
		}

		api.POST("/update", handleUpdate)
	}
	r.NoRoute(handleNotFound)
}

// ========= 核心清洗接口 =========

// handleClean GET /api/clean/?url=[&wd=剧名&episode=第X集]
func handleClean(c *gin.Context) {
	targetURL := strings.TrimSpace(c.Query("url"))

	// 方案2（保底）：客户端可直接传剧名+集数，跳过官方解析
	userWD := strings.TrimSpace(c.Query("wd"))
	if userWD == "" {
		userWD = strings.TrimSpace(c.Query("title"))
	}
	userEp := strings.TrimSpace(c.Query("episode"))
	if userEp == "" {
		userEp = strings.TrimSpace(c.Query("jj"))
	}

	if targetURL == "" && userWD == "" {
		c.JSON(200, model.Fail(404, "无url参数"))
		return
	}

	// 有 URL：把用户剧名注入 query，让 ParseOfficial 识别来源站时直接采用
	if targetURL != "" && userWD != "" {
		if u, err := url.Parse(targetURL); err == nil {
			q := u.Query()
			q.Set("wd", userWD)
			u.RawQuery = q.Encode()
			targetURL = u.String()
		}
	}

	var offSite *model.VideoSite
	if targetURL != "" {
		site, err := service.ParseOfficial(targetURL)
		if err != nil {
			c.JSON(200, model.Fail(400, err.Error()))
			return
		}
		if site == nil {
			c.JSON(200, model.Fail(400, "无法识别的链接格式"))
			return
		}
		offSite = site
	} else {
		// 无 URL：纯客户端传参（方案2）
		offSite = &model.VideoSite{Name: "客户端传参", Original: targetURL, Title: userWD, Episode: userEp}
	}

	// 用户手动 episode 优先
	if userEp != "" {
		offSite.Episode = userEp
	}

	jim, jj, u, best := resolvePlay(offSite.Title, offSite.Episode, offSite.Name)
	resp := model.OK(jim, jj, u)
	resp.Time = time.Now().Format("2006-01-02 15:04:05")
	resp.KFZ = "射手沫蝴蝶"
	if best == nil {
		resp.Code = 404
		resp.Msg = "未在资源站中匹配到资源"
	}
	c.JSON(http.StatusOK, resp)
}

// handleSearch GET /api/search/?wd=&episode=
func handleSearch(c *gin.Context) {
	wd := strings.TrimSpace(c.Query("wd"))
	ep := strings.TrimSpace(c.Query("episode"))
	if wd == "" {
		c.JSON(200, model.Fail(400, "缺少 wd 参数"))
		return
	}
	jim, jj, url, best := resolvePlay(wd, ep, "")
	resp := model.OK(jim, jj, url)
	resp.Time = time.Now().Format("2006-01-02 15:04:05")
	resp.KFZ = "射手沫蝴蝶"
	if best == nil {
		resp.Code = 404
		resp.Msg = "未在资源站中匹配到资源"
	}
	c.JSON(http.StatusOK, resp)
}

// resolvePlay 解析剧名+集数 → 最佳替换播放地址
// 返回: 剧名, 集数, 替换URL, 最佳匹配搜索结果
func resolvePlay(title, episode, siteName string) (jm, jj, url string, best *model.SearchResult) {
	jm = title
	jj = episode

	if title == "" {
		return
	}

	// 1. 根据映射选资源站
	var preferred []string
	for _, m := range config.GetMappings() {
		if m.Official == siteName {
			preferred = m.Sites
			break
		}
	}

	// 2. 搜索 + 选最佳
	var results = service.SearchResource(title, episode)
	var picked *model.SearchResult

	// 优先映射的资源站 + 匹配度高的
	for _, pref := range preferred {
		for i := range results {
			if results[i].ResourceID == pref {
				picked = &results[i]
				break
			}
		}
		if picked != nil {
			break
		}
	}
	if picked == nil && len(results) > 0 {
		picked = &results[0]
	}
	best = picked

	if picked != nil && picked.ResourceID != "" {
		for _, rs := range config.GetSites() {
			if picked.ResourceID == rs.ID || (picked.Site == rs.Name && strings.EqualFold(rs.Type, "applecms")) {
				_, eps, _ := service.FetchDetail(rs, picked.ResourceID)
				target := episode
				// 找指定集数的 URL
				for _, ep := range eps {
					if target != "" && strings.Contains(ep.Episode, target) {
						url = ep.URL
						break
					}
				}
				// 没匹配到指定集数就回退第 1 集
				if url == "" && len(eps) > 0 {
					url = eps[0].URL
					jj = eps[0].Episode
				}
				break
			}
		}
	}
	if url == "" && picked != nil && picked.URL != "" {
		url = picked.URL
	}
	return
}

// ========= 其他接口 =========

func handleVersion(c *gin.Context) {
	c.JSON(200, model.Response{
		Code: 200,
		Data: map[string]any{
			"version":   Version,
			"build":     BuildTime,
			"repo_url":  config.GetConfig().Updater.RepoURL,
			"updater":   config.GetConfig().Updater.Enabled,
			"kfz":       "射手沫蝴蝶",
		},
	})
}

func handleUpdate(c *gin.Context) {
	if cfg := config.GetConfig(); cfg != nil && cfg.Security.APIKey != "" {
		if k := c.GetHeader("X-API-Key"); k == "" {
			k = c.Query("key")
		} else if k != cfg.Security.APIKey {
			c.JSON(200, model.Fail(403, "API Key 无效"))
			return
		}
	}
	c.JSON(200, model.OK("", "", service.ManualUpdate()))
}

func handleNotFound(c *gin.Context) {
	if cfg := config.GetConfig(); cfg != nil && cfg.Server.NotFoundHTML != "" {
		if b, err := os.ReadFile(cfg.Server.NotFoundHTML); err == nil {
			c.Data(http.StatusNotFound, "text/html; charset=utf-8", b)
			return
		}
	}
	c.JSON(http.StatusNotFound, model.Fail(404, "页面未找到"))
}

// errMsg 保留（用于兼容）
func errMsg(err error) string {
	if err == nil { return "" }
	return err.Error()
}
