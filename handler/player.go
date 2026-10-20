// Package handler — 播放器控制面板（广告屏蔽 / 智能选线 / 备用入口 / 嵌入代码）
package handler

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/service"

	"github.com/gin-gonic/gin"
)

// ========= 后台：播放器控制面板 =========

// handleAdminPlayerGet GET /api/admin/player
func handleAdminPlayerGet(c *gin.Context) {
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: config.GetPlayer(),
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleAdminPlayerSave POST /api/admin/player
func handleAdminPlayerSave(c *gin.Context) {
	var req model.PlayerConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	p := config.GetPlayer()
	if req.AdBlock != nil {
		p.AdBlock = *req.AdBlock
	}
	if req.AdMode != nil {
		p.AdMode = *req.AdMode
	}
	if req.FilterSegments != nil {
		p.FilterSegments = *req.FilterSegments
	}
	if req.MaskStyle != nil {
		p.MaskStyle = *req.MaskStyle
	}
	if req.MaskColor != nil {
		p.MaskColor = *req.MaskColor
	}
	if req.MaskOpacity != nil {
		p.MaskOpacity = *req.MaskOpacity
	}
	if req.MaskStrength != nil {
		p.MaskStrength = *req.MaskStrength
	}
	if req.LineStrategy != nil {
		p.LineStrategy = *req.LineStrategy
	}
	if req.LineTimeoutMS != nil {
		p.LineTimeoutMS = *req.LineTimeoutMS
	}
	if req.LineConcurrency != nil {
		p.LineConcurrency = *req.LineConcurrency
	}
	if req.Mirrors != nil {
		p.Mirrors = *req.Mirrors
	}
	if err := config.SavePlayer(p); err != nil {
		c.JSON(200, model.Fail(500, "保存失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "播放器配置已保存",
		Data: config.GetPlayer(),
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleAdminPlayerProbeLines POST /api/admin/player/lines
// 线路测速：并发实测每条线路，按策略打分排序，返回最优线路
func handleAdminPlayerProbeLines(c *gin.Context) {
	var req model.LineProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	urls := make([]string, 0, len(req.URLs))
	seen := map[string]bool{}
	for _, u := range req.URLs {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		urls = append(urls, u)
	}
	if len(urls) == 0 {
		c.JSON(200, model.Fail(400, "请填写至少一条线路地址"))
		return
	}
	if len(urls) > 12 {
		urls = urls[:12] // 限制并发规模，避免测速过慢
	}

	p := config.GetPlayer()
	strategy := strings.ToLower(strings.TrimSpace(req.Strategy))
	switch strategy {
	case "quality", "lowad", "speed", "smart":
	default:
		strategy = p.LineStrategy
	}
	timeout := time.Duration(p.LineTimeoutMS) * time.Millisecond

	results := make([]*model.LineProbeResult, len(urls))
	sem := make(chan struct{}, p.LineConcurrency)
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = service.ProbeLine(u, config.MatchFilterSite(u), strategy, timeout)
		}(i, u)
	}
	wg.Wait()

	service.RankLines(results)
	best := ""
	for _, r := range results {
		if r.OK {
			best = r.URL
			break
		}
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "测速完成",
		Data: map[string]any{
			"strategy": strategy,
			"total":    len(results),
			"best":     best,
			"results":  results,
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleAdminPlayerEmbed POST /api/admin/player/embed
// 生成 iframe / 苹果CMS 嵌入代码；启用 ST 时自动带上加密令牌
func handleAdminPlayerEmbed(c *gin.Context) {
	var req model.EmbedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	target := strings.TrimSpace(req.URL)
	if target == "" {
		c.JSON(200, model.Fail(400, "请填写播放地址或 m3u8 地址"))
		return
	}
	typ := strings.ToLower(strings.TrimSpace(req.Type))
	if typ == "" {
		typ = "iframe"
	}

	// 清洗地址类型：m3u8 走「返回过滤后的 m3u8 文本」，其余走 JSON 清洗
	cleanType := "json"
	if strings.Contains(strings.ToLower(target), ".m3u8") {
		cleanType = "m3u8"
	}

	base := strings.TrimRight(strings.TrimSpace(req.Base), "/")
	if base == "" {
		if p := config.GetPlayer(); len(p.Mirrors) > 0 {
			base = p.Mirrors[0]
		} else {
			base = publicBase(c)
		}
	}
	cleanURL := buildCleanURLBase(base, target, cleanType)
	token := ""
	if u, err := url.Parse(cleanURL); err == nil {
		token = u.Query().Get("st")
	}

	iframe := `<div style="position:relative;width:100%;aspect-ratio:16/9;">` + "\n" +
		` <iframe src="` + cleanURL + `" allow="autoplay; fullscreen; picture-in-picture" allowfullscreen ` +
		`style="position:absolute;inset:0;width:100%;height:100%;border:0;"></iframe>` + "\n" +
		`</div>`

	acms := "var frame = document.createElement('iframe');\n" +
		"frame.src = '" + cleanURL + "';\n" +
		"frame.style.cssText = 'width:100%;height:100%;border:0;';\n" +
		"frame.allow = 'autoplay; fullscreen; picture-in-picture';\n" +
		"frame.allowFullscreen = true;\n" +
		"MacPlayer.Html = frame.outerHTML;\n" +
		"MacPlayer.Show();"

	note := "把 iframe 代码放进页面即可；外层容器需固定比例或高度，且不要加 sandbox，否则全屏会失效。"
	if token != "" {
		note += " 已启用 ST 加密：地址中的 st 令牌到期自动失效，需重新生成嵌入代码。"
	}

	code := iframe
	if typ == "acms" {
		code = acms
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "生成成功",
		Data: map[string]any{
			"type":      typ,
			"clean_url": cleanURL,
			"token":     token,
			"code":      code,
			"iframe":    iframe,
			"acms":      acms,
			"note":      note,
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handlePlayerConfig GET /api/player/config
// 播放器运行时配置（公开）：广告屏蔽开关 / 遮罩样式 / 选线策略 / ST 状态
func handlePlayerConfig(c *gin.Context) {
	p := config.GetPlayer()
	sec := config.GetSecurity()
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: map[string]any{
			"ad_block":        p.AdBlock,
			"ad_mode":         p.AdMode,
			"filter_segments": p.FilterSegments,
			"mask": model.AdMaskStyle{
				Style:    p.MaskStyle,
				Color:    p.MaskColor,
				Opacity:  p.MaskOpacity,
				Strength: p.MaskStrength,
				Mode:     p.AdMode,
			},
			"line_strategy": p.LineStrategy,
			"clean_path":    config.PrimaryCleanPath(),
			"st_enabled":    sec.STEnabled,
			"st_expire":     sec.STExpire,
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}
