// Package handler — 广告过滤 · 画面遮罩（内嵌广告 / 贴片）后台 API
//
// 与 M3U8 去插播（filter.go）互补：插播广告剔除片段，内嵌广告靠播放器叠加遮罩。
package handler

import (
	"io"
	"net/http"
	"strings"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/service"

	"github.com/gin-gonic/gin"
)

// ========= 画面遮罩规则 CRUD =========

func handleAdminAdOverlayRules(c *gin.Context) {
	rules := config.GetOverlayRules()
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: map[string]any{
			"total": len(rules),
			"rules": rules,
		},
	})
}

func handleAdminAdOverlayRuleAdd(c *gin.Context) {
	var req model.AdOverlayRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	if strings.TrimSpace(derefStr(req.Name)) == "" {
		c.JSON(200, model.Fail(400, "请填写规则名称"))
		return
	}
	rules := config.GetOverlayRules()
	rule := model.AdOverlayRule{ID: newRuleID(derefStr(req.Name))}
	applyOverlayReq(&rule, &req)
	rules = append(rules, rule)
	if err := config.SaveOverlayRules(rules); err != nil {
		c.JSON(200, model.Fail(500, "保存失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.OK("", "", "新增成功 id="+rule.ID))
}

func handleAdminAdOverlayRuleUpdate(c *gin.Context) {
	id := c.Param("id")
	var req model.AdOverlayRuleRequest
	_ = c.ShouldBindJSON(&req)

	rules := config.GetOverlayRules()
	for i := range rules {
		if rules[i].ID == id || rules[i].Name == id {
			applyOverlayReq(&rules[i], &req)
			if err := config.SaveOverlayRules(rules); err != nil {
				c.JSON(200, model.Fail(500, err.Error()))
				return
			}
			c.JSON(http.StatusOK, model.OK("", "", "更新成功"))
			return
		}
	}
	c.JSON(200, model.Fail(404, "未找到画面遮罩规则"))
}

func handleAdminAdOverlayRuleDel(c *gin.Context) {
	id := c.Param("id")
	rules := config.GetOverlayRules()
	var kept []model.AdOverlayRule
	for _, r := range rules {
		if r.ID != id && r.Name != id {
			kept = append(kept, r)
		}
	}
	if len(kept) == len(rules) {
		c.JSON(200, model.Fail(404, "未找到画面遮罩规则"))
		return
	}
	if err := config.SaveOverlayRules(kept); err != nil {
		c.JSON(200, model.Fail(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.OK("", "", "删除成功"))
}

// ========= clean 响应下发 =========

// resolveAdOverlay 为一次播放匹配画面遮罩规则；未命中返回 nil
// siteName 为该次播放实际命中的资源站名称：绑定到该资源站的规则会**优先自动套用**，
// 从而无需为每个播放域名手工配置遮罩（「绑定资源站、避免手动遮罩」）
func resolveAdOverlay(playURL, siteName string) *model.AdOverlayClean {
	rule := config.MatchOverlayRule(playURL, siteName)
	if rule == nil {
		return nil
	}
	return &model.AdOverlayClean{
		RuleID:   rule.ID,
		RuleName: rule.Name,
		Regions:  rule.Regions,
	}
}

// ========= 工具 =========

// applyOverlayReq 把请求里的字段写入画面遮罩规则（nil 字段保持原值）
func applyOverlayReq(r *model.AdOverlayRule, req *model.AdOverlayRuleRequest) {
	if req.Name != nil {
		r.Name = strings.TrimSpace(*req.Name)
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
	if req.Sort != nil {
		r.Sort = *req.Sort
	}
	if req.URLPrefixes != nil {
		r.URLPrefixes = cleanList(*req.URLPrefixes)
	}
	if req.URLRegexes != nil {
		r.URLRegexes = cleanList(*req.URLRegexes)
	}
	if req.SiteIDs != nil {
		r.SiteIDs = cleanList(*req.SiteIDs)
	}
	if req.SourceURL != nil {
		r.SourceURL = strings.TrimSpace(*req.SourceURL)
	}
	if req.Regions != nil {
		r.Regions = normalizeOverlayRegions(*req.Regions)
	}
	if req.Remark != nil {
		r.Remark = strings.TrimSpace(*req.Remark)
	}
}

// normalizeOverlayRegions 规范化遮罩区域：裁剪到 0~1 范围、补默认样式、丢弃无效区域
func normalizeOverlayRegions(in []model.AdOverlayRegion) []model.AdOverlayRegion {
	out := make([]model.AdOverlayRegion, 0, len(in))
	for _, r := range in {
		r.X = clamp01(r.X)
		r.Y = clamp01(r.Y)
		r.W = clamp01(r.W)
		r.H = clamp01(r.H)
		if r.W <= 0 || r.H <= 0 {
			continue // 宽或高为 0 的区域没有意义，丢弃
		}
		if r.X+r.W > 1 {
			r.X = 1 - r.W
		}
		if r.Y+r.H > 1 {
			r.Y = 1 - r.H
		}
		if r.Mode != "blur" {
			r.Mode = "cover"
		}
		if strings.TrimSpace(r.Color) == "" {
			r.Color = "#000000"
		}
		if r.Opacity <= 0 || r.Opacity > 1 {
			r.Opacity = 1
		}
		r.Label = strings.TrimSpace(r.Label)
		out = append(out, r)
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ========= 自动识别（链接 → 域名 / 资源站 / 候选遮罩区域） =========

// handleAdminAdOverlayIdentify POST /api/admin/adfilter/overlay/identify
// 给一条链接：自动解析匹配域名、推荐可绑定的资源站，并给出候选遮罩区域
func handleAdminAdOverlayIdentify(c *gin.Context) {
	var req model.AdOverlayIdentifyRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		c.JSON(200, model.Fail(400, "请填写识别链接"))
		return
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: identifyOverlay(strings.TrimSpace(req.URL)),
	})
}

// identifyOverlay 核心识别逻辑
func identifyOverlay(raw string) model.AdOverlayIdentifyResult {
	res := model.AdOverlayIdentifyResult{Kind: "unknown"}
	res.Kind = classifyLinkKind(raw)

	norm := raw
	if !strings.Contains(norm, "://") {
		norm = "http://" + norm
	}
	res.Host = config.HostOf(norm)
	if res.Host == "" {
		res.Note = "无法解析链接域名，请检查链接是否完整"
		return res
	}
	res.Prefixes = []string{res.Host}

	// ① 命中已配置的资源站 → 推荐绑定（绑定后无需再依赖播放域名）
	if site := config.FindSiteByHost(res.Host); site != nil {
		res.SiteID = site.ID
		res.SiteName = site.Name
	}
	// ② 若已有可复用规则（同域名 / 同资源站），提示直接复用
	rules := config.GetOverlayRules()
	for i := range rules {
		if overlayRuleCoversHost(&rules[i], res.Host) || (res.SiteName != "" && overlayRuleBindsSite(&rules[i], res.SiteName)) {
			res.ReuseRuleID = rules[i].ID
			break
		}
	}

	// ③ 候选遮罩区域
	switch res.Kind {
	case "image":
		if w, h, ok := fetchImageSize(raw); ok {
			res.Regions = imageRatioPresets(w, h)
			res.Method = "image-ratio"
			res.Note = "已按广告图片的宽高比推算遮罩大小；位置为常见广告位，请按实际画面确认。"
		} else {
			res.Regions = overlayPresets()
			res.Method = "preset"
			res.Note = "图片读取失败，已给出常见广告位候选。"
		}
	default:
		// 页面 / m3u8：尝试无头浏览器抓帧 + 启发式识别
		if shot, err := service.CapturePageShot(norm); err == nil && shot != nil {
			if regs := service.DetectCandidateRegions(shot.PNG); len(regs) > 0 {
				res.Regions = regs
				res.Method = "frame-detect"
				res.Note = "已抓帧做启发式识别，结果为候选（越小的框越可能是台标 / 二维码），请人工确认。"
				break
			}
		}
		res.Regions = overlayPresets()
		res.Method = "preset"
		res.Note = "未能抓帧（无头浏览器不可用或页面无播放画面），已给出常见广告位候选。"
	}

	if res.SiteName != "" {
		res.Note = "已识别资源站「" + res.SiteName + "」，绑定后调用该资源站将自动套用遮罩。" + res.Note
	}
	return res
}

// overlayRuleCoversHost 规则是否已覆盖该域名
func overlayRuleCoversHost(r *model.AdOverlayRule, host string) bool {
	for _, p := range r.URLPrefixes {
		if p != "" && strings.HasPrefix(host, p) {
			return true
		}
	}
	return false
}

// overlayRuleBindsSite 规则是否已绑定该资源站
func overlayRuleBindsSite(r *model.AdOverlayRule, siteName string) bool {
	for _, id := range r.SiteIDs {
		if id == siteName {
			return true
		}
	}
	return false
}

// classifyLinkKind 判断链接类型：image / m3u8 / page / unknown
func classifyLinkKind(raw string) string {
	low := strings.ToLower(raw)
	if strings.Contains(low, ".m3u8") || strings.Contains(low, "type=m3u8") {
		return "m3u8"
	}
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"} {
		if strings.Contains(low, ext) {
			return "image"
		}
	}
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return "page"
	}
	return "unknown"
}

// fetchImageSize 下载图片并解析其宽高（失败返回 ok=false）
func fetchImageSize(raw string) (int, int, bool) {
	client := &http.Client{Timeout: 12 * time.Second}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return 0, 0, false
	}
	req.Header.Set("User-Agent", service.BrowserUA)
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, false
	}
	// 限制读取体积，避免超大图拖垮内存
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, 0, false
	}
	return service.ImageConfig(body)
}

// overlayPresets 常见内嵌广告位（相对比例 0~1 的候选）
func overlayPresets() []model.AdOverlayRegion {
	return []model.AdOverlayRegion{
		{X: 0.02, Y: 0.03, W: 0.16, H: 0.10, Mode: "cover", Color: "#000000", Opacity: 1, Label: "左上角台标"},
		{X: 0.82, Y: 0.03, W: 0.16, H: 0.10, Mode: "cover", Color: "#000000", Opacity: 1, Label: "右上角水印"},
		{X: 0.82, Y: 0.78, W: 0.15, H: 0.20, Mode: "cover", Color: "#000000", Opacity: 1, Label: "右下角二维码"},
		{X: 0.03, Y: 0.78, W: 0.15, H: 0.20, Mode: "cover", Color: "#000000", Opacity: 1, Label: "左下角二维码"},
		{X: 0, Y: 0.86, W: 1, H: 0.14, Mode: "cover", Color: "#000000", Opacity: 1, Label: "底部滚动字幕条"},
		{X: 0, Y: 0, W: 1, H: 0.10, Mode: "cover", Color: "#000000", Opacity: 1, Label: "顶部横幅"},
	}
}

// imageRatioPresets 按广告图片宽高比推算遮罩尺寸，并放到几个常见位置
// 说明：16:9 画面里，相对高度 = 相对宽度 × (图片高/图片宽) × (16/9)
func imageRatioPresets(w, h int) []model.AdOverlayRegion {
	if w <= 0 || h <= 0 {
		return overlayPresets()
	}
	const frameRatio = 16.0 / 9.0
	wf := 0.28
	hf := wf * (float64(h) / float64(w)) * frameRatio
	if hf > 0.60 {
		hf = 0.60
	}
	if hf < 0.05 {
		hf = 0.05
	}
	return []model.AdOverlayRegion{
		{X: clamp01(1 - wf - 0.02), Y: clamp01(1 - hf - 0.04), W: wf, H: hf, Mode: "cover", Color: "#000000", Opacity: 1, Label: "右下角（按广告图比例）"},
		{X: 0.02, Y: 0.03, W: wf, H: hf, Mode: "cover", Color: "#000000", Opacity: 1, Label: "左上角（按广告图比例）"},
		{X: clamp01((1 - wf) / 2), Y: clamp01(1 - hf - 0.03), W: wf, H: hf, Mode: "cover", Color: "#000000", Opacity: 1, Label: "底部居中（按广告图比例）"},
	}
}
