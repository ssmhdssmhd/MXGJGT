// Package handler - 后台管理 API
package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/service"

	"github.com/gin-gonic/gin"
)

// ========= 播放 / 版本检查 =========

// handleAdminPlayStream GET /api/admin/stream?page=<播放页地址>
// 把采集站的播放页解析成直链 m3u8，供后台内嵌播放（hls.js 直连，站点已开 CORS）
func handleAdminPlayStream(c *gin.Context) {
	page := strings.TrimSpace(c.Query("page"))
	if page == "" {
		c.JSON(200, model.Fail(400, "缺少 page 参数（播放页地址）"))
		return
	}
	stream, err := service.ResolvePlayStream(page)
	if err != nil {
		c.JSON(200, model.Fail(404, "解析播放地址失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "ok",
		Data: map[string]any{
			"page":   page,
			"stream": stream,
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleAdminUpdateCheck GET /api/admin/update/check
// 只查版本不更新：返回当前版本 / 远程最新版本 / 是否有更新
func handleAdminUpdateCheck(c *gin.Context) {
	info := service.CheckUpdateInfo()
	msg := "已是最新版本 v" + info.Current
	if info.Error != "" {
		msg = "检查失败: " + info.Error
	} else if info.HasUpdate {
		msg = "发现新版本 v" + info.Latest
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  msg,
		Data: info,
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// ========= 资源站 CRUD =========

func handleAdminListSites(c *gin.Context) {
	sites := config.GetSites()
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: map[string]any{
			"total": len(sites),
			"sites": sites,
		},
	})
}

func handleAdminAddSite(c *gin.Context) {
	var req model.AddSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	if strings.EqualFold(req.Type, "") {
		req.Type = "applemcms"
	}
	if req.Encode == "" {
		req.Encode = "utf8"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	sites := config.GetSites()
	id := strings.ToLower(strings.ReplaceAll(req.Name, " ", ""))
	newSite := model.ResourceSite{
		ID:        id,
		Name:      req.Name,
		Type:      req.Type,
		URL:       req.URL,
		DetailURL: req.DetailURL,
		Encode:    req.Encode,
		CID:       req.CID,
		Enabled:   enabled,
	}
	sites = append(sites, newSite)
	if err := config.SaveSites(sites); err != nil {
		c.JSON(200, model.Fail(500, "保存失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.OK("", "", fmt.Sprintf("添加成功 id=%s", id)))
}

func handleAdminUpdateSite(c *gin.Context) {
	id := c.Param("id")
	var req model.UpdateSiteRequest
	_ = c.ShouldBindJSON(&req)

	sites := config.GetSites()
	for i, s := range sites {
		if s.ID == id || s.Name == id {
			if req.Name != nil { s.Name = *req.Name }
			if req.URL != nil { s.URL = *req.URL }
			if req.DetailURL != nil { s.DetailURL = *req.DetailURL }
			if req.CID != nil { s.CID = *req.CID }
			if req.Enabled != nil { s.Enabled = *req.Enabled }
			sites[i] = s
			if err := config.SaveSites(sites); err != nil {
				c.JSON(200, model.Fail(500, err.Error()))
				return
			}
			c.JSON(http.StatusOK, model.OK("", "", "更新成功"))
			return
		}
	}
	c.JSON(http.StatusOK, model.Fail(404, "未找到资源站"))
}

func handleAdminDelSite(c *gin.Context) {
	id := c.Param("id")
	sites := config.GetSites()
	var kept []model.ResourceSite
	for _, s := range sites {
		if s.ID != id && s.Name != id {
			kept = append(kept, s)
		}
	}
	if len(kept) == len(sites) {
		c.JSON(200, model.Fail(404, "未找到资源站"))
		return
	}
	if err := config.SaveSites(kept); err != nil {
		c.JSON(200, model.Fail(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.OK("", "", "删除成功"))
}

// ========= 苹果CMS 自动分析 =========

func handleAdminAutoCMS(c *gin.Context) {
	var req model.AutoAnalyzeCMSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	analyzed, cids, err := service.AutoAnalyzeAppleCMS(req.URL)
	if err != nil {
		c.JSON(200, model.Fail(400, "接口分析失败: "+err.Error()))
		return
	}

	// 生成候选 ID
	id := strings.ToLower(strings.ReplaceAll(analyzed.Name, " ", ""))
	if req.Name != "" {
		analyzed.Name = req.Name
		id = strings.ToLower(strings.ReplaceAll(req.Name, " ", ""))
	}
	analyzed.ID = id

	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "分析成功，请确认后保存",
		Data: map[string]any{
			"site":  analyzed,
			"cids":  cids,
			"hint":  "确认无误后调用 POST /api/admin/site 保存",
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// ========= 映射管理 =========

func handleAdminListMap(c *gin.Context) {
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: map[string]any{
			"mappings": config.GetMappings(),
			"sites":    config.GetSites(),
		},
	})
}

func handleAdminSetMap(c *gin.Context) {
	var req model.UpdateMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	maps := config.GetMappings()
	found := false
	for i := range maps {
		if maps[i].Official == req.Site {
			maps[i].Sites = req.Sites
			found = true
			break
		}
	}
	if !found {
		maps = append(maps, model.SiteMapping{Official: req.Site, Sites: req.Sites})
	}
	if err := config.SaveMappings(maps); err != nil {
		c.JSON(200, model.Fail(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.OK("", "", "映射保存成功"))
}

func handleAdminReload(c *gin.Context) {
	if err := config.Reload(); err != nil {
		c.JSON(200, model.Fail(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.OK("", "", "配置已重新加载"))
}

// ========= 资源站 测试 / 搜索 / 同步 =========

// findSite 按 ID 或名称查找资源站
func findSite(id string) (model.ResourceSite, bool) {
	for _, s := range config.GetSites() {
		if s.ID == id || s.Name == id {
			return s, true
		}
	}
	return model.ResourceSite{}, false
}

// siteIDFromQuery 从 query 取资源站 ID/名称
func siteIDFromQuery(c *gin.Context) string {
	return strings.TrimSpace(c.Query("id"))
}

// handleAdminTestSite GET /api/admin/site/test?id=&wd=
// 连通性测试：用关键词（默认「庆余年」）向该资源站发一次真实请求
func handleAdminTestSite(c *gin.Context) {
	site, ok := findSite(siteIDFromQuery(c))
	if !ok {
		c.JSON(200, model.Fail(404, "未找到资源站"))
		return
	}
	res := service.ProbeSite(site, c.Query("wd"))
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "测试完成",
		Data: map[string]any{
			"site":   site.Name,
			"result": res,
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleAdminSearchSite GET /api/admin/site/search?id=&wd=
// 在指定资源站内搜索关键词，返回结果列表（后台人工核对用）
func handleAdminSearchSite(c *gin.Context) {
	site, ok := findSite(siteIDFromQuery(c))
	if !ok {
		c.JSON(200, model.Fail(404, "未找到资源站"))
		return
	}
	wd := strings.TrimSpace(c.Query("wd"))
	if wd == "" {
		c.JSON(200, model.Fail(400, "缺少 wd 参数"))
		return
	}
	results, err := service.SearchOneSite(site, wd)
	if err != nil {
		c.JSON(200, model.Fail(400, "搜索失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  fmt.Sprintf("命中 %d 条", len(results)),
		Data: map[string]any{
			"site":    site.Name,
			"wd":      wd,
			"count":   len(results),
			"results": results,
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleAdminSiteDetail GET /api/admin/site/detail?id=&rid=
// 查看某部资源的详情与剧集播放列表（后台可直接跳转播放 / 复制地址）
func handleAdminSiteDetail(c *gin.Context) {
	site, ok := findSite(siteIDFromQuery(c))
	if !ok {
		c.JSON(200, model.Fail(404, "未找到资源站"))
		return
	}
	rid := strings.TrimSpace(c.Query("rid"))
	if rid == "" {
		c.JSON(200, model.Fail(400, "缺少 rid 参数（资源ID）"))
		return
	}
	if !service.IsAppleCMSType(site.Type) || site.DetailURL == "" {
		c.JSON(200, model.Fail(400, "该资源站不支持详情（需要苹果CMS 类型且已配置详情地址）"))
		return
	}

	res, eps, err := service.FetchDetail(site, rid)
	if err != nil {
		c.JSON(200, model.Fail(400, "获取详情失败: "+err.Error()))
		return
	}
	if res == nil {
		c.JSON(200, model.Fail(404, "未获取到详情，请检查详情地址模板是否正确"))
		return
	}

	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  fmt.Sprintf("共 %d 集", len(eps)),
		Data: map[string]any{
			"site":     site.Name,
			"detail":   res,
			"episodes": eps,
			"count":    len(eps),
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleAdminSyncSite POST /api/admin/site/sync?id=
// 同步资源站分类：重新拉取苹果CMS分类列表并写回 cid
func handleAdminSyncSite(c *gin.Context) {
	id := siteIDFromQuery(c)
	sites := config.GetSites()
	for i := range sites {
		if sites[i].ID != id && sites[i].Name != id {
			continue
		}
		cids, err := service.FetchSiteCIDs(sites[i])
		if err != nil {
			c.JSON(200, model.Fail(400, "同步失败: "+err.Error()))
			return
		}
		oldCID := sites[i].CID
		parts := make([]string, 0, len(cids))
		for _, it := range cids {
			parts = append(parts, strconv.Itoa(it.ID))
		}
		sites[i].CID = strings.Join(parts, ",")
		if err := config.SaveSites(sites); err != nil {
			c.JSON(200, model.Fail(500, "保存失败: "+err.Error()))
			return
		}
		c.JSON(http.StatusOK, model.Response{
			Code: 200,
			Msg:  fmt.Sprintf("同步完成，分类 %d 个", len(cids)),
			Data: map[string]any{
				"site":    sites[i].Name,
				"old_cid": oldCID,
				"cid":     sites[i].CID,
				"count":   len(cids),
				"cids":    cids,
			},
			Time: time.Now().Format("2006-01-02 15:04:05"),
		})
		return
	}
	c.JSON(200, model.Fail(404, "未找到资源站"))
}
