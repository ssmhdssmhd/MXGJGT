// Package model 定义全局数据类型
package model

// Response 统一 API 返回格式
type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

// VideoSite 官方视频网站识别结果
type VideoSite struct {
	Name     string   `json:"name"`      // 网站名称
	Domain   string   `json:"domain"`    // 主域名
	Aliases  []string `json:"aliases"`   // 别名域名
	Title    string   `json:"title"`     // 从链接提取的影视名
	Episode  string   `json:"episode"`   // 集数
	Original string   `json:"original"`  // 原始链接
}

// ResourceSite 资源站配置
type ResourceSite struct {
	Name      string `yaml:"name"`                // 资源站名称
	Type      string `yaml:"type"`                // 类型: html / applecms
	URL       string `yaml:"url"`                 // 搜索 URL，{keyword} 为关键字占位符
	DetailURL string `yaml:"detail_url"`          // 详情 URL（applecms 类型用 {id} 占位符）
	Encode    string `yaml:"encode"`              // 编码方式: url / gb2312 / utf8
	Selector  string `yaml:"selector"`            // html 类型: goquery 选择器
	TitleAttr string `yaml:"title_attr"`          // html 类型: 标题属性
	LinkAttr  string `yaml:"link_attr"`           // html 类型: 链接属性
	Enabled   bool   `yaml:"enabled"`             // 是否启用
}

// SearchResult 资源站搜索结果
type SearchResult struct {
	Title        string `json:"title"`
	URL          string `json:"url"`             // 详情页 URL 或播放地址
	Site         string `json:"site"`
	ResourceID   string `json:"resource_id,omitempty"`
	Type         string `json:"type,omitempty"`         // 影片类型（国产剧/动作片...）
	Remark       string `json:"remark,omitempty"`       // 更新备注（已完结/更新至第XX集）
	PlayFrom     string `json:"play_from,omitempty"`    // 播放源
	PlayURL      string `json:"play_url,omitempty"`     // 原始播放列表字符串
	Matched      bool   `json:"matched,omitempty"`      // 是否精确匹配
}

// EpisodePlay 单集播放信息
type EpisodePlay struct {
	Episode string `json:"episode"`
	URL     string `json:"url"`
}

// CleanResult 清洗后的最终结果
type CleanResult struct {
	Original    string        `json:"original"`
	Title       string        `json:"title"`
	Episode     string        `json:"episode"`
	Site        string        `json:"site"`
	Replacement string        `json:"replacement"`
	Results     []SearchResult `json:"results"`
	BestMatch   *SearchResult  `json:"best_match,omitempty"`
	Episodes    []EpisodePlay  `json:"episodes,omitempty"` // 最佳匹配的全集播放列表
}

// AppConfig 应用配置
type AppConfig struct {
	Server   ServerConfig   `yaml:"server"`
	Updater  UpdaterConfig  `yaml:"updater"`
	Security SecurityConfig `yaml:"security"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port         int    `yaml:"port"`
	Mode         string `yaml:"mode"`
	NotFoundHTML string `yaml:"not_found_html"`
}

// UpdaterConfig 自动更新配置
type UpdaterConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Interval string   `yaml:"interval"`
	Branch   string   `yaml:"branch"`
	RepoURL  string   `yaml:"repo_url"`
	Mirrors  []string `yaml:"mirrors"`
	UseProxy bool     `yaml:"use_proxy"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	APIKey string `yaml:"api_key"`
}

// ======== 苹果CMS 接口专用结构 ========

// AcmsListResponse 苹果CMS列表接口返回
type AcmsListResponse struct {
	Code  int       `json:"code"`
	Msg   string    `json:"msg"`
	Total int       `json:"total"`
	List  []AcmsVideo `json:"list"`
}

// AcmsVideo 苹果CMS视频条目
type AcmsVideo struct {
	VodID       int    `json:"vod_id"`
	VodName     string `json:"vod_name"`
	TypeID      int    `json:"type_id"`
	TypeName    string `json:"type_name"`
	VodRemark   string `json:"vod_remarks"`
	VodPic      string `json:"vod_pic"`
	VodPlayFrom string `json:"vod_play_from"`
	VodPlayURL  string `json:"vod_play_url"`
	VodArea     string `json:"vod_area"`
	VodYear     string `json:"vod_year"`
}
