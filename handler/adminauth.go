// Package handler - 网页后台（/mx/）：登录鉴权 + 会话管理
package handler

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"

	"github.com/gin-gonic/gin"
)

//go:embed mxadmin.html
var adminPageHTML []byte

const adminCookieName = "mx_admin_sess"

type adminSession struct {
	Username string
	Expire   time.Time
}

var (
	sessMu   sync.RWMutex
	sessions = map[string]adminSession{}
)

func adminCfg() model.AdminConfig {
	if cfg := config.GetConfig(); cfg != nil {
		return cfg.Admin
	}
	return model.AdminConfig{}
}

// newSessionToken 生成随机会话令牌
func newSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// createSession 创建会话并返回令牌
func createSession(username string, ttl time.Duration) string {
	tok := newSessionToken()
	sessMu.Lock()
	sessions[tok] = adminSession{Username: username, Expire: time.Now().Add(ttl)}
	sessMu.Unlock()
	return tok
}

// lookupSession 校验令牌是否有效
func lookupSession(token string) (adminSession, bool) {
	if token == "" {
		return adminSession{}, false
	}
	sessMu.RLock()
	s, ok := sessions[token]
	sessMu.RUnlock()
	if !ok {
		return adminSession{}, false
	}
	if time.Now().After(s.Expire) {
		sessMu.Lock()
		delete(sessions, token)
		sessMu.Unlock()
		return adminSession{}, false
	}
	return s, true
}

func destroySession(token string) {
	sessMu.Lock()
	delete(sessions, token)
	sessMu.Unlock()
}

// isAdminSession 供其它 handler（如 /api/update）判断是否已登录后台
func isAdminSession(c *gin.Context) bool {
	tok, _ := c.Cookie(adminCookieName)
	_, ok := lookupSession(tok)
	return ok
}

// adminAuth 中间件：校验后台登录态（未登录直接返回 401 JSON）
func adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg := adminCfg(); !cfg.Enabled {
			c.AbortWithStatusJSON(http.StatusNotFound, model.Fail(404, "页面未找到"))
			return
		}
		tok, _ := c.Cookie(adminCookieName)
		if _, ok := lookupSession(tok); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.Fail(401, "未登录或登录已过期"))
			return
		}
		c.Next()
	}
}

// ===== 后台页面 =====

func handleAdminPage(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", adminPageHTML)
}

// ===== 登录 / 登出 / 状态 =====

func handleAdminLogin(c *gin.Context) {
	cfg := adminCfg()
	if !cfg.Enabled {
		c.JSON(200, model.Fail(404, "页面未找到"))
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	userOK := subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.Username)), []byte(cfg.Username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(cfg.Password)) == 1
	if !userOK || !passOK {
		time.Sleep(300 * time.Millisecond) // 轻微延迟，缓解暴力破解
		c.JSON(200, model.Fail(401, "用户名或密码错误"))
		return
	}
	ttl := time.Duration(cfg.SessionHours) * time.Hour
	tok := createSession(cfg.Username, ttl)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(adminCookieName, tok, int(ttl.Seconds()), "/", "", false, true)
	c.JSON(200, model.OK("", "", "登录成功"))
}

func handleAdminLogout(c *gin.Context) {
	if tok, _ := c.Cookie(adminCookieName); tok != "" {
		destroySession(tok)
	}
	c.SetCookie(adminCookieName, "", -1, "/", "", false, true)
	c.JSON(200, model.OK("", "", "已退出登录"))
}

func handleAdminMe(c *gin.Context) {
	tok, _ := c.Cookie(adminCookieName)
	s, ok := lookupSession(tok)
	c.JSON(200, model.Response{
		Code: 200,
		Data: map[string]any{
			"authenticated": ok,
			"username":      s.Username,
			"version":       Version,
		},
	})
}
