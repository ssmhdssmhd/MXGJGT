// Package handler - 后台静态资源（logo/favicon）
package handler

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 内嵌 logo 与 favicon：后台页面不再依赖外部图床（raw.githubusercontent.com 常被墙）
//
//go:embed assets/logo.png
var adminLogoPNG []byte

//go:embed assets/favicon.png
var adminFaviconPNG []byte

func handleAdminLogo(c *gin.Context) {
	c.Data(http.StatusOK, "image/png", adminLogoPNG)
}

func handleAdminFavicon(c *gin.Context) {
	c.Data(http.StatusOK, "image/png", adminFaviconPNG)
}
