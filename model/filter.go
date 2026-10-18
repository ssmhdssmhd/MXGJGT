// Package model — M3U8 广告过滤（去插播）相关数据结构
package model

// ======== 站点规则 ========

// FilterSite M3U8 广告过滤站点规则
// 与参考站（mxqcb）「资源站规则」字段保持一致，便于后续对齐全部检测方式
type FilterSite struct {
	ID          string   `yaml:"id" json:"id"`                     // 唯一 ID
	Name        string   `yaml:"name" json:"name"`                 // 站点名
	Enabled     bool     `yaml:"enabled" json:"enabled"`           // 启用该站点
	Sort        int      `yaml:"sort" json:"sort"`                 // 排序码
	ParseType   string   `yaml:"parse_type" json:"parse_type"`     // both / duration / hash / direct
	DetectMode  string   `yaml:"detect_mode" json:"detect_mode"`   // auto / sequence / blockcut / slicebatch / shortblock / fingerprint / hash / both
	IndexMode   string   `yaml:"index_mode" json:"index_mode"`     // auto / single / fixed
	StreamPick  string   `yaml:"stream_pick" json:"stream_pick"`   // first / highest / lowest
	FixedSub    string   `yaml:"fixed_sub" json:"fixed_sub"`       // 索引模式=fixed 时的固定子路径
	ProxyURL    string   `yaml:"proxy_url" json:"proxy_url"`       // 站点级出网代理（留空=直连，填 - 强制直连）
	URLPrefixes []string `yaml:"url_prefixes" json:"url_prefixes"` // URL 匹配前缀（主机名）
	URLRegexes  []string `yaml:"url_regexes" json:"url_regexes"`   // URL 匹配正则（整条 URL）
	SubDir      string   `yaml:"sub_dir" json:"sub_dir"`           // 缓存子目录
	Fingerprint string   `yaml:"fingerprint" json:"fingerprint"`   // 广告时长指纹，一行一组
	RegexList   []string `yaml:"regex_list" json:"regex_list"`     // 片段级广告过滤正则
	Remark      string   `yaml:"remark" json:"remark"`             // 备注
}

// ======== 调用路径（系统设置 · 路径设置） ========

// CleanPath 用户自定义的 /api/clean 调用路径
type CleanPath struct {
	Path    string `yaml:"path" json:"path"`       // 如 /api/clean
	Enabled bool   `yaml:"enabled" json:"enabled"` // 是否启用
	Remark  string `yaml:"remark" json:"remark"`   // 备注
}

// ======== 调用日志 ========

// FilterRecord 一次过滤调用记录
type FilterRecord struct {
	Time    string `json:"time"`    // 时间
	Site    string `json:"site"`    // 命中的站点名（未命中为空）
	SiteID  string `json:"site_id"` // 命中的站点 ID
	URL     string `json:"url"`     // 请求的 m3u8 地址
	Origin  int    `json:"origin"`  // 原始片段数
	Kept    int    `json:"kept"`    // 保留片段数
	Removed int    `json:"removed"` // 剔除片段数
	Mode    string `json:"mode"`    // 实际使用的检测方式
	CostMS  int64  `json:"cost_ms"` // 耗时（毫秒）
	Cached  bool   `json:"cached"`  // 是否命中缓存
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// ======== 自动探测 ========

// ProbeSegment 探测出的单个片段
type ProbeSegment struct {
	Index    int     `json:"index"`
	Duration float64 `json:"duration"`
	URI      string  `json:"uri"`
	Ad       bool    `json:"ad"` // 探测判定为广告
}

// ProbeResult m3u8 自动探测结果
type ProbeResult struct {
	OK          bool           `json:"ok"`
	IndexURL    string         `json:"index_url"`   // 实际解析到的媒体索引地址
	IsMaster    bool           `json:"is_master"`   // 原始地址是否为主索引
	Variants    []string       `json:"variants"`    // 主索引里的子列表
	Segments    []ProbeSegment `json:"segments"`    // 解析出的片段（最多返回前 N 条）
	Total       int            `json:"total"`       // 片段总数
	AdRanges    []string       `json:"ad_ranges"`   // 探测到的广告区间（如 0-3）
	Fingerprint string         `json:"fingerprint"` // 广告时长指纹（可直接粘贴到站点规则）
	SuggestMode string         `json:"suggest_mode"`
	Error       string         `json:"error,omitempty"`
}

// ======== M3U8 测试 ========

// FilterTestResult M3U8 过滤测试结果
type FilterTestResult struct {
	OK       bool           `json:"ok"`
	SiteID   string         `json:"site_id"`
	SiteName string         `json:"site_name"`
	Mode     string         `json:"mode"`
	IndexURL string         `json:"index_url"`
	Total    int            `json:"total"`
	Kept     int            `json:"kept"`
	Removed  int            `json:"removed"`
	AdRanges []string       `json:"ad_ranges"`
	Segments []ProbeSegment `json:"segments"`
	Playlist string         `json:"playlist"` // 过滤后的 m3u8（前若干行，供预览）
	CostMS   int64          `json:"cost_ms"`
	Error    string         `json:"error,omitempty"`
}

// ======== 请求体 ========

// FilterSiteRequest 新增 / 更新站点规则
type FilterSiteRequest struct {
	Name        *string   `json:"name"`
	Enabled     *bool     `json:"enabled"`
	Sort        *int      `json:"sort"`
	ParseType   *string   `json:"parse_type"`
	DetectMode  *string   `json:"detect_mode"`
	IndexMode   *string   `json:"index_mode"`
	StreamPick  *string   `json:"stream_pick"`
	FixedSub    *string   `json:"fixed_sub"`
	ProxyURL    *string   `json:"proxy_url"`
	URLPrefixes *[]string `json:"url_prefixes"`
	URLRegexes  *[]string `json:"url_regexes"`
	SubDir      *string   `json:"sub_dir"`
	Fingerprint *string   `json:"fingerprint"`
	RegexList   *[]string `json:"regex_list"`
	Remark      *string   `json:"remark"`
}

// CleanPathRequest 新增 / 更新调用路径
type CleanPathRequest struct {
	Path    string `json:"path"`
	Enabled *bool  `json:"enabled"`
	Remark  string `json:"remark"`
}
