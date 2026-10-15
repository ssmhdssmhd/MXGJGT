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
	site, err := service.ParseOfficial(targetURL)
	if err != nil || site == nil {
		c.JSON(http.StatusOK, model.Response{
			Code: 400,
			Msg:  "无法识别的链接格式: " + errMsg(err),
			Data: gin.H{"url": targetURL},
		})
		return
	}

	// 2. 从资源站搜索替换
	var results []model.SearchResult
	if site.Title != "" {
		results = service.SearchResource(site.Title, site.Episode)
	}

	// 3. 整理返回
	replacement := ""
	if len(results) > 0 {
		replacement = results[0].URL
	}

	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "success",
		Data: model.CleanResult{
			Original:    targetURL,
			Title:       site.Title,
			Episode:     site.Episode,
			Site:        site.Name,
			Replacement: replacement,
			Results:     results,
		},
	})
}

// handleVersion 返回版本信息
func handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "success",
		Data: gin.H{
			"version":   Version,
			"build":     BuildTime,
			"repo_url":  config.GetConfig().Updater.RepoURL,
			"branch":    config.GetConfig().Updater.Branch,
			"updater":   config.GetConfig().Updater.Enabled,
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

// handleNotFound 伪装 404：返回静态 404 页面或简单 JSON
func handleNotFound(c *gin.Context) {
	cfg := config.GetConfig()
	if cfg.Server.NotFoundHTML != "" {
		if data, err := os.ReadFile(cfg.Server.NotFoundHTML); err == nil {
			c.Data(http.StatusNotFound, "text/html; charset=utf-8", data)
			return
		}
	}

	// 默认伪装 JSON 返回
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
