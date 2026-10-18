// Package handler - M3U8 广告过滤（去插播）后台 API + 系统设置·路径设置
package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"
	"mxgjgt/service"

	"github.com/gin-gonic/gin"
)

// ========= 站点规则 =========

func handleAdminFilterSites(c *gin.Context) {
	sites := config.GetFilterSites()
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: map[string]any{
			"total": len(sites),
			"sites": sites,
		},
	})
}

func handleAdminFilterSiteAdd(c *gin.Context) {
	var req model.FilterSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	if strings.TrimSpace(derefStr(req.Name)) == "" {
		c.JSON(200, model.Fail(400, "请填写规则名称"))
		return
	}
	sites := config.GetFilterSites()
	site := model.FilterSite{ID: newFilterID(derefStr(req.Name))}
	applyFilterSiteReq(&site, &req)
	sites = append(sites, site)
	if err := config.SaveFilterSites(sites); err != nil {
		c.JSON(200, model.Fail(500, "保存失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.OK("", "", "新增成功 id="+site.ID))
}

func handleAdminFilterSiteUpdate(c *gin.Context) {
	id := c.Param("id")
	var req model.FilterSiteRequest
	_ = c.ShouldBindJSON(&req)

	sites := config.GetFilterSites()
	for i := range sites {
		if sites[i].ID == id || sites[i].Name == id {
			applyFilterSiteReq(&sites[i], &req)
			if err := config.SaveFilterSites(sites); err != nil {
				c.JSON(200, model.Fail(500, err.Error()))
				return
			}
			c.JSON(http.StatusOK, model.OK("", "", "更新成功"))
			return
		}
	}
	c.JSON(200, model.Fail(404, "未找到过滤规则"))
}

func handleAdminFilterSiteDel(c *gin.Context) {
	id := c.Param("id")
	sites := config.GetFilterSites()
	var kept []model.FilterSite
	for _, s := range sites {
		if s.ID != id && s.Name != id {
			kept = append(kept, s)
		}
	}
	if len(kept) == len(sites) {
		c.JSON(200, model.Fail(404, "未找到过滤规则"))
		return
	}
	if err := config.SaveFilterSites(kept); err != nil {
		c.JSON(200, model.Fail(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.OK("", "", "删除成功"))
}

// ========= 探测 / 测试 =========

func handleAdminFilterProbe(c *gin.Context) {
	var req struct {
		URL      string `json:"url"`
		SiteID   string `json:"site_id"`
		DeepHash bool   `json:"deep_hash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	u := strings.TrimSpace(req.URL)
	if u == "" {
		c.JSON(200, model.Fail(400, "请填写 m3u8 地址"))
		return
	}
	u = resolveFilterInput(u)
	site := resolveFilterSite(req.SiteID, u)
	res, err := service.ProbeM3U8(u, site, req.DeepHash)
	if err != nil {
		c.JSON(200, model.Fail(400, "探测失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "探测完成",
		Data: res,
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

func handleAdminFilterTest(c *gin.Context) {
	var req struct {
		URL    string `json:"url"`
		SiteID string `json:"site_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	u := strings.TrimSpace(req.URL)
	if u == "" {
		c.JSON(200, model.Fail(400, "请填写 m3u8 地址"))
		return
	}
	u = resolveFilterInput(u)
	site := resolveFilterSite(req.SiteID, u)
	start := time.Now()
	res, err := service.FilterM3U8(u, site)
	if err != nil {
		c.JSON(200, model.Fail(400, "过滤失败: "+err.Error()))
		return
	}
	out := model.FilterTestResult{
		OK:       true,
		Mode:     res.Mode,
		IndexURL: res.IndexURL,
		Total:    res.Total,
		Kept:     res.Kept,
		Removed:  res.Removed,
		AdRanges: res.AdRanges,
		Segments: toProbeSegments(res.Segments, 120),
		Playlist: headLines(res.Playlist, 40),
		CostMS:   time.Since(start).Milliseconds(),
	}
	if site != nil {
		out.SiteID = site.ID
		out.SiteName = site.Name
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "测试完成",
		Data: out,
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

func handleAdminFilterLogs(c *gin.Context) {
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: map[string]any{
			"records": service.GetFilterRecords(),
		},
	})
}

// ========= 系统设置 · 路径设置 =========

func handleAdminCleanPaths(c *gin.Context) {
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Data: map[string]any{
			"paths": config.GetCleanPaths(),
		},
	})
}

func handleAdminSaveCleanPaths(c *gin.Context) {
	var req struct {
		Paths []model.CleanPath `json:"paths"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, model.Fail(400, "参数错误"))
		return
	}
	for i := range req.Paths {
		req.Paths[i].Path = strings.TrimSpace(req.Paths[i].Path)
	}
	if err := config.SaveCleanPaths(req.Paths); err != nil {
		c.JSON(200, model.Fail(500, "保存失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.Response{
		Code: 200,
		Msg:  "保存成功，已立即生效",
		Data: map[string]any{
			"paths": config.GetCleanPaths(),
		},
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
}

// ========= 工具 =========

// resolveFilterInput 输入兼容 m3u8 直链与播放页：
// 非 m3u8 的地址尝试解析成直链（失败则原样返回，交由后续流程报错）
func resolveFilterInput(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.Contains(strings.ToLower(u), ".m3u8") {
		return u
	}
	if s, err := service.ResolvePlayStream(u); err == nil && s != "" {
		return s
	}
	return u
}

// resolveFilterSite 优先按 ID 指定；否则按 URL 自动匹配；都不命中返回 nil（走自动模式）
func resolveFilterSite(siteID, u string) *model.FilterSite {
	siteID = strings.TrimSpace(siteID)
	if siteID != "" {
		for i := range config.GetFilterSites() {
			s := config.GetFilterSites()[i]
			if s.ID == siteID || s.Name == siteID {
				return &s
			}
		}
	}
	return config.MatchFilterSite(u)
}

// applyFilterSiteReq 把请求里的字段写入站点规则（nil 字段保持原值）
func applyFilterSiteReq(s *model.FilterSite, req *model.FilterSiteRequest) {
	if req.Name != nil {
		s.Name = strings.TrimSpace(*req.Name)
	}
	if req.Enabled != nil {
		s.Enabled = *req.Enabled
	}
	if req.Sort != nil {
		s.Sort = *req.Sort
	}
	if req.ParseType != nil {
		s.ParseType = strings.TrimSpace(*req.ParseType)
	}
	if req.DetectMode != nil {
		s.DetectMode = strings.TrimSpace(*req.DetectMode)
	}
	if req.IndexMode != nil {
		s.IndexMode = strings.TrimSpace(*req.IndexMode)
	}
	if req.StreamPick != nil {
		s.StreamPick = strings.TrimSpace(*req.StreamPick)
	}
	if req.FixedSub != nil {
		s.FixedSub = strings.TrimSpace(*req.FixedSub)
	}
	if req.ProxyURL != nil {
		s.ProxyURL = strings.TrimSpace(*req.ProxyURL)
	}
	if req.URLPrefixes != nil {
		s.URLPrefixes = cleanList(*req.URLPrefixes)
	}
	if req.URLRegexes != nil {
		s.URLRegexes = cleanList(*req.URLRegexes)
	}
	if req.SubDir != nil {
		s.SubDir = strings.TrimSpace(*req.SubDir)
	}
	if req.Fingerprint != nil {
		s.Fingerprint = strings.TrimSpace(*req.Fingerprint)
	}
	if req.RegexList != nil {
		s.RegexList = cleanList(*req.RegexList)
	}
	if req.Remark != nil {
		s.Remark = strings.TrimSpace(*req.Remark)
	}
	// 补齐默认值
	if s.ParseType == "" {
		s.ParseType = "both"
	}
	if s.DetectMode == "" {
		s.DetectMode = "auto"
	}
	if s.IndexMode == "" {
		s.IndexMode = "auto"
	}
	if s.StreamPick == "" {
		s.StreamPick = "first"
	}
}

// cleanList 去掉空行/空白
func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// newFilterID 由名称生成站点规则 ID（小写去空格），再拼短随机串避免重名
func newFilterID(name string) string {
	id := strings.ToLower(strings.Join(strings.Fields(name), ""))
	if id == "" {
		id = "rule"
	}
	b := make([]byte, 2)
	rand.Read(b)
	return id + "-" + hex.EncodeToString(b)
}

// toProbeSegments 把内部片段结构转为探测输出（最多 n 条）
func toProbeSegments(segs []*service.M3U8Segment, n int) []model.ProbeSegment {
	out := make([]model.ProbeSegment, 0, n)
	for i, s := range segs {
		if i >= n {
			break
		}
		out = append(out, model.ProbeSegment{Index: s.Index, Duration: s.Duration, URI: s.AbsURL, Ad: s.Ad})
	}
	return out
}

// headLines 取文本前 n 行（供播放列表预览）
func headLines(text string, n int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
