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

// 后台侧边栏圆形徽标：MXLOGO 仓库 app/app-icon-mark-512.png（纯蝴蝶图标，透明底）
//
//go:embed assets/logo-mark.png
var adminLogoMarkPNG []byte

//go:embed assets/favicon.png
var adminFaviconPNG []byte

// 内嵌 hls.js（Apache-2.0，https://github.com/video-dev/hls.js）：
// 后台「详情」弹窗内嵌播放直链 m3u8 用，避免依赖 CDN。
//
//go:embed assets/hls.min.js
var adminHlsJS []byte

func handleAdminLogo(c *gin.Context) {
	c.Data(http.StatusOK, "image/png", adminLogoPNG)
}

func handleAdminLogoMark(c *gin.Context) {
	c.Data(http.StatusOK, "image/png", adminLogoMarkPNG)
}

func handleAdminFavicon(c *gin.Context) {
	c.Data(http.StatusOK, "image/png", adminFaviconPNG)
}

func handleAdminHlsJS(c *gin.Context) {
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", adminHlsJS)
}
