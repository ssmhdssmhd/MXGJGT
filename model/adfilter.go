// Package model — 广告过滤 · 画面遮罩（内嵌广告 / 贴片）相关数据结构
//
// 与 M3U8 插播广告过滤（去插播，见 filter.go）互为补充：
//   - 插播片段级广告：剔除 m3u8 中的广告片段（filter.go）
//   - 画面内嵌级广告：广告文字/台标/二维码「烧」进画面像素，无法靠剔除片段解决，
//     改由播放器按本规则叠加遮罩（零转码、零带宽、秒级生效）
package model

// ======== 画面遮罩 ========

// AdOverlayRegion 播放器遮罩区域
// 坐标全部使用「相对比例 0~1」，播放器按视频实际尺寸等比换算，
// 因此同一规则可跨分辨率、跨剧集复用（无需为每个清晰度单独配置）。
type AdOverlayRegion struct {
	X       float64 `yaml:"x" json:"x"`             // 左上角横向比例 0~1
	Y       float64 `yaml:"y" json:"y"`             // 左上角纵向比例 0~1
	W       float64 `yaml:"w" json:"w"`             // 宽度比例 0~1
	H       float64 `yaml:"h" json:"h"`             // 高度比例 0~1
	Mode    string  `yaml:"mode" json:"mode"`       // cover=纯色遮挡（默认）/ blur=模糊
	Color   string  `yaml:"color" json:"color"`     // cover 填充色，默认 #000000
	Opacity float64 `yaml:"opacity" json:"opacity"` // 不透明度 0~1，默认 1（仅 cover 生效）
	Label   string  `yaml:"label" json:"label"`     // 备注，如「片头角标」「本片由…文字」
}

// AdOverlayRule 画面遮罩规则：命中播放域名或绑定的资源站后，向播放器下发遮罩区域列表
type AdOverlayRule struct {
	ID          string            `yaml:"id" json:"id"`                     // 唯一 ID
	Name        string            `yaml:"name" json:"name"`                 // 规则名
	Enabled     bool              `yaml:"enabled" json:"enabled"`           // 启用该规则
	Sort        int               `yaml:"sort" json:"sort"`                 // 排序码
	URLPrefixes []string          `yaml:"url_prefixes" json:"url_prefixes"` // 按播放域名主机名前缀匹配
	URLRegexes  []string          `yaml:"url_regexes" json:"url_regexes"`   // 正则匹配（^ 开头或含 :// 时匹配整条 URL）
	SiteIDs     []string          `yaml:"site_ids" json:"site_ids"`         // 绑定的资源站（ID 或名称，任一命中即套用，优先于 URL 匹配）
	SourceURL   string            `yaml:"source_url" json:"source_url"`     // 识别来源链接（自动识别时记录，便于回溯）
	Regions     []AdOverlayRegion `yaml:"regions" json:"regions"`           // 遮罩区域列表
	Remark      string            `yaml:"remark" json:"remark"`             // 备注
}

// AdOverlayClean /api/clean 返回给播放器的画面遮罩信息
// 播放器按 regions 在对应相对位置叠加遮罩，即完成「去内嵌广告」
type AdOverlayClean struct {
	RuleID   string            `json:"rule_id,omitempty"`
	RuleName string            `json:"rule_name,omitempty"`
	Regions  []AdOverlayRegion `json:"regions"`
}

// ======== 请求体 ========

// AdOverlayRuleRequest 新增 / 更新画面遮罩规则
type AdOverlayRuleRequest struct {
	Name        *string            `json:"name"`
	Enabled     *bool              `json:"enabled"`
	Sort        *int               `json:"sort"`
	URLPrefixes *[]string          `json:"url_prefixes"`
	URLRegexes  *[]string          `json:"url_regexes"`
	SiteIDs     *[]string          `json:"site_ids"`
	SourceURL   *string            `json:"source_url"`
	Regions     *[]AdOverlayRegion `json:"regions"`
	Remark      *string            `json:"remark"`
}

// AdOverlayIdentifyRequest 自动识别请求：给一条链接（播放页 / m3u8 / 图片 / 资源站），
// 服务端据此推断匹配域名、可绑定的资源站，并给出候选遮罩区域
type AdOverlayIdentifyRequest struct {
	URL string `json:"url"`
}

// AdOverlayIdentifyResult 自动识别结果
type AdOverlayIdentifyResult struct {
	Host        string            `json:"host"`          // 解析出的主机名
	Prefixes    []string          `json:"prefixes"`      // 建议的 URL 匹配前缀
	SiteID      string            `json:"site_id"`       // 命中的资源站 ID（可能为空）
	SiteName    string            `json:"site_name"`     // 命中的资源站名称（可能为空）
	Kind        string            `json:"kind"`          // 链接类型：page / m3u8 / image / unknown
	Method      string            `json:"method"`        // 识别方式：site-bind / frame-detect / preset / image-ratio
	Regions     []AdOverlayRegion `json:"regions"`       // 候选遮罩区域（前端可一键添加）
	ReuseRuleID string            `json:"reuse_rule_id"` // 若已有同域名规则则给出可复用的规则 ID
	Note        string            `json:"note"`          // 识别说明 / 提示
}
