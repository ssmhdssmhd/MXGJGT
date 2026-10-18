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

		// 后台管理接口：需登录态
		admin := api.Group("/admin", adminAuth())
		{
			admin.GET("/sites",        handleAdminListSites)
			admin.POST("/site/auto",   handleAdminAutoCMS)
			admin.GET("/site/test",    handleAdminTestSite)
			admin.GET("/site/search",  handleAdminSearchSite)
			admin.GET("/site/detail",  handleAdminSiteDetail)
			admin.GET("/stream",       handleAdminPlayStream)
			admin.GET("/update/check", handleAdminUpdateCheck)
			admin.POST("/site/sync",   handleAdminSyncSite)
			admin.POST("/site",        handleAdminAddSite)
			admin.PUT("/site/:id",     handleAdminUpdateSite)
			admin.DELETE("/site/:id",  handleAdminDelSite)
			admin.GET("/mappings",     handleAdminListMap)
			admin.POST("/mapping",     handleAdminSetMap)
			admin.POST("/reload",      handleAdminReload)
		}

		api.POST("/update", handleUpdate)
	}

	// 网页后台登录入口（路径来自配置，默认 /mx）
	registerAdminUI(r)

	r.NoRoute(handleNotFound)
}

// registerAdminUI 注册网页后台页面与登录路由
func registerAdminUI(r *gin.Engine) {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Admin.Enabled {
		return
	}
	p := strings.TrimRight(strings.TrimSpace(cfg.Admin.Path), "/")
	if p == "" {
		p = "/mx"
	}
	r.GET(p, handleAdminPage)
	r.GET(p+"/", handleAdminPage)
	r.GET(p+"/assets/logo.png", handleAdminLogo)
	r.GET(p+"/assets/logo-mark.png", handleAdminLogoMark)
	r.GET(p+"/assets/favicon.png", handleAdminFavicon)
	r.GET(p+"/assets/hls.js", handleAdminHlsJS)
	r.POST(p+"/api/login", handleAdminLogin)
	r.POST(p+"/api/logout", handleAdminLogout)
	r.GET(p+"/api/me", handleAdminMe)
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
		resp.Msg = noMatchMsg(offSite.Title)
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
		resp.Msg = noMatchMsg(wd)
	}
	c.JSON(http.StatusOK, resp)
}

// noMatchMsg 未命中提示：附上已尝试过的搜索关键词，便于排查是「识别错剧名」还是「资源站没收录」
func noMatchMsg(title string) string {
	msg := "未在资源站中匹配到资源"
	if vs := service.KeywordVariants(title); len(vs) > 0 {
		msg += "（已尝试关键词：" + strings.Join(vs, " / ") + "）"
	}
	return msg
}

// containsStr 字符串切片是否包含目标值
func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// resolvePlay 解析剧名+集数 → 最佳替换播放地址
// 返回: 剧名, 集数, 替换URL, 最佳匹配搜索结果
func resolvePlay(title, episode, siteName string) (jm, jj, url string, best *model.SearchResult) {
	jm = title
	jj = episode

	if title == "" {
		return
	}

	// 1. 根据映射取「优先资源站」的站点名（映射里存的是站点 ID，需要换成名称才能和搜索结果对应）
	var preferredNames []string
	for _, m := range config.GetMappings() {
		if m.Official != siteName {
			continue
		}
		// 按映射里配置的优先级顺序转成站点名
		for _, id := range m.Sites {
			for _, s := range config.GetSites() {
				if (s.ID == id || s.Name == id) && !containsStr(preferredNames, s.Name) {
					preferredNames = append(preferredNames, s.Name)
					break
				}
			}
		}
		break
	}

	// 2. 搜索 + 选最佳
	// 关键词逐级降级重试：原名 → 去空格 → 去「第X季/部/语言」后缀 → 主名
	// （资源站关键词搜索多为精确分词，官方剧名带后缀时会 0 命中）
	var results []model.SearchResult
	for _, kw := range service.KeywordVariants(title) {
		if got := service.SearchResource(kw, episode); len(got) > 0 {
			results = got
			break
		}
	}
	var picked *model.SearchResult

	// 优先映射的资源站（按映射配置的先后顺序）
	for _, name := range preferredNames {
		for i := range results {
			if results[i].Site == name {
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
			// 按站点名定位来源资源站（ResourceID 是采集站的 vod_id，与站点 ID 不是一回事）
			if picked.Site == rs.Name && rs.Enabled && service.IsAppleCMSType(rs.Type) {
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
	cfg := config.GetConfig()
	// 已登录后台会话时放行；否则按 security.api_key 校验
	if cfg != nil && cfg.Security.APIKey != "" && !isAdminSession(c) {
		k := c.GetHeader("X-API-Key")
		if k == "" {
			k = c.Query("key")
		}
		if k != cfg.Security.APIKey {
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
