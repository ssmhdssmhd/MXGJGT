// Package handler - 后台管理 API
package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/service"

	"github.com/gin-gonic/gin"
)

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
