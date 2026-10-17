// Package model 定义全局数据类型
package model

// Response 统一 JSON 返回格式
// 简洁成功返回: {"code":200,"jm":"庆余年","jj":"第12集","url":"https://...","msg":"https://...","time":"...","kfz":"射手沫蝴蝶"}
type Response struct {
	Code int    `json:"code"`
	JM   string `json:"jm,omitempty"`    // 剧名
	JJ   string `json:"jj,omitempty"`    // 剧集集数
	URL  string `json:"url,omitempty"`   // 替换后的播放地址
	Msg  string `json:"msg,omitempty"`   // 同 url（或错误消息）
	Time string `json:"time,omitempty"`  // 响应时间
	KFZ  string `json:"kfz,omitempty"`   // 开发商/制作方
	Data any    `json:"data,omitempty"`  // 后台管理扩展数据
}

// OK 构造成功响应
func OK(jm, jj, url string) Response {
	return Response{
		Code: 200,
		JM:   jm,
		JJ:   jj,
		URL:  url,
		Msg:  url,
	}
}

// Fail 构造失败响应
func Fail(code int, msg string) Response {
	return Response{Code: code, Msg: msg}
}

// VideoSite 官方视频网站识别结果
type VideoSite struct {
	Name     string   `json:"name"`
	Domain   string   `json:"domain"`
	Aliases  []string `json:"aliases"`
	Title    string   `json:"title"`
	Episode  string   `json:"episode"`
	Original string   `json:"original"`
}

// ResourceSite 资源站配置
type ResourceSite struct {
	ID        string `yaml:"id" json:"id"`                   // 唯一 ID（后台管理用）
	Name      string `yaml:"name" json:"name"`
	Type      string `yaml:"type" json:"type"`                 // applecms / html
	URL       string `yaml:"url" json:"url"`                  // 搜索 URL，{keyword} 占位符
	DetailURL string `yaml:"detail_url" json:"detail_url"`     // 详情 URL，{id} 占位符
	Encode    string `yaml:"encode" json:"encode"`
	Selector  string `yaml:"selector" json:"selector"`
	TitleAttr string `yaml:"title_attr" json:"title_attr"`
	LinkAttr  string `yaml:"link_attr" json:"link_attr"`
	CID       string `yaml:"cid" json:"cid"`                  // 苹果CMS分类ID映射（逗号分隔）
	Enabled   bool   `yaml:"enabled" json:"enabled"`
}

// SiteMapping 官方视频站 → 资源站 映射
type SiteMapping struct {
	Official string   `yaml:"official" json:"official"` // 官方站点中文名
	Sites    []string `yaml:"sites" json:"sites"`       // 优先使用的资源站 ID 列表
}

// SearchResult 资源站搜索结果
type SearchResult struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	Site       string `json:"site"`
	ResourceID string `json:"resource_id"`
	Type       string `json:"type"`
	Remark     string `json:"remark"`
	PlayFrom   string `json:"play_from"`
	PlayURL    string `json:"play_url"`
	Matched    bool   `json:"matched"`
}

// EpisodePlay 单集播放信息
type EpisodePlay struct {
	Episode string `json:"episode"`
	URL     string `json:"url"`
}

// AppConfig 主配置
type AppConfig struct {
	Server   ServerConfig   `yaml:"server"`
	Updater  UpdaterConfig  `yaml:"updater"`
	Security SecurityConfig `yaml:"security"`
	Browser  BrowserConfig  `yaml:"browser"`
	Admin    AdminConfig    `yaml:"admin"`
}

type ServerConfig struct {
	Port         int    `yaml:"port"`
	Mode         string `yaml:"mode"`
	NotFoundHTML string `yaml:"not_found_html"`
}

type UpdaterConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Interval string   `yaml:"interval"`
	Branch   string   `yaml:"branch"`
	RepoURL  string   `yaml:"repo_url"`
	Mirrors  []string `yaml:"mirrors"`
}

type SecurityConfig struct {
	APIKey string `yaml:"api_key"`
}

// BrowserConfig 无头浏览器渲染配置（方案1 主方案）
// 从官方页面渲染抓取剧名+集数，覆盖爱奇艺/优酷等强反爬站
type BrowserConfig struct {
	Enabled    bool   `yaml:"enabled"`     // 总开关；未配置时默认启用
	ChromePath string `yaml:"chrome_path"` // Chrome/Chromium 可执行文件路径，留空自动探测
	TimeoutSec int    `yaml:"timeout_sec"` // 单次渲染超时秒数，默认 20
}

// AdminConfig 网页后台（/mx/）配置
type AdminConfig struct {
	Enabled      bool   `yaml:"enabled"`       // 是否启用网页后台；关闭后 /mx/ 与 /api/admin/* 走伪装 404
	Path         string `yaml:"path"`          // 后台入口路径，默认 /mx
	Username     string `yaml:"username"`      // 登录用户名
	Password     string `yaml:"password"`      // 登录密码（明文，请务必修改默认值）
	SessionHours int    `yaml:"session_hours"` // 登录态有效期（小时），默认 24
}

// ======== 苹果CMS 接口专用 ========

type AcmsListResponse struct {
	Code  int          `json:"code"`
	Msg   string       `json:"msg"`
	Total int          `json:"total"`
	List  []AcmsVideo  `json:"list"`
}

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

// CIDMapResponse 苹果CMS分类接口返回
type CIDMapResponse struct {
	List []struct {
		ID   int    `json:"type_id"`
		Name string `json:"type_name"`
	} `json:"list"`
}

// ======== 后台管理请求 ========

// AutoAnalyzeCMSRequest 自动分析苹果CMS接口请求
type AutoAnalyzeCMSRequest struct {
	URL  string `json:"url" binding:"required"`
	Name string `json:"name"`
}

// AddSiteRequest 添加资源站请求
type AddSiteRequest struct {
	Name      string `json:"name" binding:"required"`
	Type      string `json:"type"`
	URL       string `json:"url" binding:"required"`
	DetailURL string `json:"detail_url"`
	Encode    string `json:"encode"`
	CID       string `json:"cid"`
	Enabled   *bool  `json:"enabled"`
}

// UpdateSiteRequest 更新资源站请求
type UpdateSiteRequest struct {
	Name      *string `json:"name"`
	URL       *string `json:"url"`
	DetailURL *string `json:"detail_url"`
	CID       *string `json:"cid"`
	Enabled   *bool   `json:"enabled"`
}

// UpdateMappingRequest 更新官方站→资源站映射
type UpdateMappingRequest struct {
	Site   string   `json:"site" binding:"required"`
	Sites []string `json:"sites" binding:"required"`
}
