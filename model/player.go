// Package model — 播放器控制面板（广告屏蔽 / 智能选线 / 备用入口 / 嵌入代码）
//
// 参考巨量播放器的公开能力整理而来，只保留「服务端能真正落地」的部分：
//   - 广告屏蔽：总开关、智能/仅手动、遮挡样式与强度、插播片段过滤开关
//   - 智能选线：综合推荐 / 清晰优先 / 低广告优先 / 速度优先 + 线路测速
//   - 备用入口：镜像域名，生成调用地址时优先使用
//   - 嵌入代码：iframe / 苹果CMS，自动带上 ST 加密令牌
package model

// PlayerConfig 播放器控制面板配置（持久化到 data/player.yaml）
type PlayerConfig struct {
	AdBlock         bool     `yaml:"ad_block" json:"ad_block"`                   // 广告屏蔽总开关
	AdMode          string   `yaml:"ad_mode" json:"ad_mode"`                     // smart=智能屏蔽 / manual=仅手动
	FilterSegments  bool     `yaml:"filter_segments" json:"filter_segments"`     // 插播片段过滤（去插播）
	MaskStyle       string   `yaml:"mask_style" json:"mask_style"`               // cover=纯色 / blur=模糊 / mosaic=马赛克
	MaskColor       string   `yaml:"mask_color" json:"mask_color"`               // cover 填充色
	MaskOpacity     float64  `yaml:"mask_opacity" json:"mask_opacity"`           // 不透明度 0~1（cover 生效）
	MaskStrength    int      `yaml:"mask_strength" json:"mask_strength"`         // 模糊/马赛克强度 1~20
	LineStrategy    string   `yaml:"line_strategy" json:"line_strategy"`         // smart / quality / lowad / speed
	LineTimeoutMS   int      `yaml:"line_timeout_ms" json:"line_timeout_ms"`     // 单线路测速超时（毫秒）
	LineConcurrency int      `yaml:"line_concurrency" json:"line_concurrency"`   // 并发测速数
	Mirrors         []string `yaml:"mirrors" json:"mirrors"`                     // 备用入口域名（生成调用地址时优先）
}

// PlayerConfigRequest 保存播放器配置（nil 字段保持原值）
type PlayerConfigRequest struct {
	AdBlock         *bool     `json:"ad_block"`
	AdMode          *string   `json:"ad_mode"`
	FilterSegments  *bool     `json:"filter_segments"`
	MaskStyle       *string   `json:"mask_style"`
	MaskColor       *string   `json:"mask_color"`
	MaskOpacity     *float64  `json:"mask_opacity"`
	MaskStrength    *int      `json:"mask_strength"`
	LineStrategy    *string   `json:"line_strategy"`
	LineTimeoutMS   *int      `json:"line_timeout_ms"`
	LineConcurrency *int      `json:"line_concurrency"`
	Mirrors         *[]string `json:"mirrors"`
}

// AdMaskStyle 下发给播放器的遮罩样式（全局默认，区域未单独指定时使用）
type AdMaskStyle struct {
	Style    string  `json:"style"`    // cover / blur / mosaic
	Color    string  `json:"color"`    // cover 填充色
	Opacity  float64 `json:"opacity"`  // 0~1
	Strength int     `json:"strength"` // 模糊/马赛克强度
	Mode     string  `json:"mode"`     // smart=智能屏蔽 / manual=仅手动
}

// ======== 线路测速 / 智能选线 ========

// LineProbeRequest 线路测速请求
type LineProbeRequest struct {
	URLs     []string `json:"urls"`     // 待测线路（m3u8 直链或播放页）
	Strategy string   `json:"strategy"` // 选线策略，留空用配置值
}

// LineProbeResult 单条线路的测速结果
type LineProbeResult struct {
	URL         string  `json:"url"`
	OK          bool    `json:"ok"`
	Kind        string  `json:"kind"` // master=主索引（含多清晰度）/ media=媒体索引 / page=播放页
	LatencyMS   int64   `json:"latency_ms"`
	SpeedKBps   float64 `json:"speed_kbps"`   // 首个分片下载速度（KB/s）
	Width       int     `json:"width"`        // 分辨率宽（主索引解析）
	Height      int     `json:"height"`       // 分辨率高
	Segments    int     `json:"segments"`     // 片段数
	DurationSec float64 `json:"duration_sec"` // 总时长（秒）
	AdSegments  int     `json:"ad_segments"`  // 命中广告规则的片段数
	AdRatio     float64 `json:"ad_ratio"`     // 广告片段占比 0~1
	Score       float64 `json:"score"`        // 综合得分 0~100（按策略计算）
	Rank        int     `json:"rank"`         // 排名（1=最优）
	Error       string  `json:"error,omitempty"`
}

// ======== 嵌入代码 ========

// EmbedRequest 生成嵌入代码
type EmbedRequest struct {
	URL  string `json:"url"`  // 播放地址 / m3u8
	Type string `json:"type"` // iframe / acms / url，留空=iframe
	Base string `json:"base"` // 指定入口域名（留空=当前后台域名，或配置的备用入口）
}

// EmbedResult 嵌入代码结果
type EmbedResult struct {
	CleanURL string `json:"clean_url"` // 本服务清洗地址（可能含 st 令牌）
	Token    string `json:"token"`     // st 令牌（未启用 ST 时为空）
	Iframe   string `json:"iframe"`    // iframe 嵌入代码
	ACMS     string `json:"acms"`      // 苹果CMS 播放器代码
	Note     string `json:"note"`      // 使用说明
}
