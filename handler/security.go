// Package handler - 安全设置（ST 加密 + 失效）
package handler

import (
	"net/http"
	"strings"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/service"

	"github.com/gin-gonic/gin"
)

// ========= 安全设置 · ST 加密 + 失效 =========

// handleAdminSecurityGet GET /api/admin/security
// 返回安全配置（ST 密钥脱敏展示，仅本机后台可见）
func handleAdminSecurityGet(c *gin.Context) {
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: securityView(config.GetSecurity()),
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleAdminSecuritySave POST /api/admin/security
// 保存安全设置；reset_secret=true 时重新生成 ST 密钥（旧令牌立即失效）
func handleAdminSecuritySave(c *gin.Context) {
	var req struct {
		APIKey      *string `json:"api_key"`
		STEnabled   *bool   `json:"st_enabled"`
		STExpire    *int    `json:"st_expire"`
		STSecret    *string `json:"st_secret"`
		ResetSecret bool    `json:"reset_secret"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}

	sec := config.GetSecurity()
	if req.APIKey != nil {
		sec.APIKey = strings.TrimSpace(*req.APIKey)
	}
	if req.STEnabled != nil {
		sec.STEnabled = *req.STEnabled
	}
	if req.STExpire != nil {
		sec.STExpire = *req.STExpire
	}
	if req.STSecret != nil && strings.TrimSpace(*req.STSecret) != "" {
		sec.STSecret = strings.TrimSpace(*req.STSecret)
	}
	// 密钥缺失或主动重置时自动生成
	if req.ResetSecret || strings.TrimSpace(sec.STSecret) == "" {
		sec.STSecret = service.GenerateSTSecret()
	}
	// 未显式指定有效期时启用默认 5 分钟，避免「加密了却永不过期」
	if req.STExpire == nil && sec.STEnabled && sec.STExpire <= 0 {
		sec.STExpire = 300
	}
	if err := config.SaveSecurity(sec); err != nil {
		c.JSON(200, model.Fail(500, "保存失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "安全设置已保存",
		Data: securityView(config.GetSecurity()),
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleAdminSecuritySTToken POST /api/admin/security/st-token
// 用当前 ST 密钥为指定地址生成加密令牌（供后台自测 / 对接验证）
func handleAdminSecuritySTToken(c *gin.Context) {
	var req struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	u := strings.TrimSpace(req.URL)
	if u == "" {
		c.JSON(200, model.Fail(400, "请填写要加密的播放地址或 m3u8 地址"))
		return
	}
	sec := config.GetSecurity()
	if !sec.STEnabled {
		c.JSON(200, model.Fail(400, "尚未启用 ST 加密，请先开启并保存"))
		return
	}
	typ := strings.ToLower(strings.TrimSpace(req.Type))
	if typ == "" {
		if strings.Contains(strings.ToLower(u), ".m3u8") {
			typ = "m3u8"
		} else {
			typ = "json"
		}
	}
	tk, err := service.EncodeST(u, typ, sec.STSecret, sec.STExpire)
	if err != nil {
		c.JSON(200, model.Fail(500, "生成失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "令牌生成成功",
		Data: map[string]any{
			"token":     tk,
			"clean_url": buildCleanURL(c, u, typ),
			"type":      typ,
			"expire":    sec.STExpire,
			"expire_at": service.STExpireAt(sec.STExpire),
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// securityView 后台展示用视图（密钥脱敏）
func securityView(sec model.SecurityConfig) map[string]any {
	return map[string]any{
		"api_key":        sec.APIKey,
		"st_enabled":     sec.STEnabled,
		"st_expire":      sec.STExpire,
		"st_secret_set":  strings.TrimSpace(sec.STSecret) != "",
		"st_secret_mask": maskSecret(sec.STSecret),
	}
}

// maskSecret 密钥脱敏：仅保留首尾 4 位
func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "********"
	}
	return s[:4] + "********" + s[len(s)-4:]
}
