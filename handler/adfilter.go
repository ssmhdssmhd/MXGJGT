// Package handler — 广告过滤 · 画面遮罩（内嵌广告 / 贴片）后台 API
//
// 与 M3U8 去插播（filter.go）互补：插播广告剔除片段，内嵌广告靠播放器叠加遮罩。
package handler

import (
	"net/http"
	"strings"

	"mxgjgt/config"
	"mxgjgt/model"

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

// resolveAdOverlay 为播放地址匹配画面遮罩规则；未命中返回 nil
// 命中后由 /api/clean 的 JSON 以 ad 字段下发，播放器据此叠加遮罩
func resolveAdOverlay(playURL string) *model.AdOverlayClean {
	rule := config.MatchOverlayRule(playURL)
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
