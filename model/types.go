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
	Name      string `json:"name"`       // 资源站名称
	URL       string `json:"url"`        // 搜索 URL，{keyword} 为关键字占位符
	Encode    string `json:"encode"`     // 编码方式: url / gb2312 / utf8
	Selector  string `json:"selector"`   // goquery 选择器提取搜索结果列表
	TitleAttr string `json:"title_attr"` // 标题属性名
	LinkAttr  string `json:"link_attr"`  // 链接属性名
	Enabled   bool   `json:"enabled"`    // 是否启用
}

// SearchResult 资源站搜索结果
type SearchResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Site  string `json:"site"`
}

// CleanResult 清洗后的最终结果
type CleanResult struct {
	Original    string         `json:"original"`
	Title       string         `json:"title"`
	Episode     string         `json:"episode"`
	Site        string         `json:"site"`
	Replacement string         `json:"replacement"`
	Results     []SearchResult `json:"results"`
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
	Mirrors  []string `yaml:"mirrors"` // 镜像列表，含 {origin} 占位符
	UseProxy bool     `yaml:"use_proxy"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	APIKey string `yaml:"api_key"`
}
