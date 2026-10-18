// Package config 配置管理 — 支持首次启动自动写模板 / 热加载 / 持久化
package config

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"mxgjgt/model"

	"gopkg.in/yaml.v3"
)

// defaultCleanPath 默认调用路径（与线上既有调用方式保持一致）
const defaultCleanPath = "/api/clean"

var (
	appConfig   *model.AppConfig
	sites       []model.ResourceSite
	mappings    []model.SiteMapping
	cleanPaths  []model.CleanPath
	filterSites []model.FilterSite

	dataDir  string
	cacheDir string
	logsDir  string

	mu sync.RWMutex
)

// ===== 路径初始化 =====

func InitPaths(data, cache, logs string) {
	dataDir = data
	cacheDir = cache
	logsDir = logs
}

func CacheDir() string { return cacheDir }
func LogsDir() string  { return logsDir }
func DataDir() string  { return dataDir }

// ===== Getter =====

func GetConfig() *model.AppConfig {
	mu.RLock()
	defer mu.RUnlock()
	return appConfig
}

func GetSites() []model.ResourceSite {
	mu.RLock()
	defer mu.RUnlock()
	return sites
}

func GetMappings() []model.SiteMapping {
	mu.RLock()
	defer mu.RUnlock()
	return mappings
}

// GetCleanPaths 获取用户自定义的 /api/clean 调用路径
func GetCleanPaths() []model.CleanPath {
	mu.RLock()
	defer mu.RUnlock()
	return cleanPaths
}

// GetFilterSites 获取 M3U8 广告过滤站点规则
func GetFilterSites() []model.FilterSite {
	mu.RLock()
	defer mu.RUnlock()
	return filterSites
}

// ===== 首次启动写模板（仅当文件不存在时） =====

func EnsureDefaults() {
	ensureFile(filepath.Join(dataDir, "config.yaml"), defaultConfigYAML())
	ensureFile(filepath.Join(dataDir, "sites.yaml"), defaultSitesYAML())
	ensureFile(filepath.Join(dataDir, "mappings.yaml"), defaultMappingsYAML())
	ensureFile(filepath.Join(dataDir, "cleanpaths.yaml"), defaultCleanPathsYAML())
	ensureFile(filepath.Join(dataDir, "filter_sites.yaml"), defaultFilterSitesYAML())
}

func ensureFile(path, content string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.WriteFile(path, []byte(content), 0644)
	}
}

// ===== 加载 =====

func LoadConfig(dir string) error {
	mu.Lock()
	defer mu.Unlock()
	dataDir = dir

	// config.yaml
	cfgPath := filepath.Join(dir, "config.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		var c model.AppConfig
		if yaml.Unmarshal(data, &c) == nil {
			appConfig = &c
			applyDefaults(appConfig)
		}
	}
	if appConfig == nil {
		appConfig = defaultConfig()
	}

	// sites.yaml
	sitesPath := filepath.Join(dir, "sites.yaml")
	if data, err := os.ReadFile(sitesPath); err == nil {
		var loaded []model.ResourceSite
		if yaml.Unmarshal(data, &loaded) == nil {
			for i := range loaded {
				if loaded[i].ID == "" {
					loaded[i].ID = randomID()
				}
			}
			sites = loaded
		}
	}

	// mappings.yaml
	mapPath := filepath.Join(dir, "mappings.yaml")
	if data, err := os.ReadFile(mapPath); err == nil {
		var m []model.SiteMapping
		if yaml.Unmarshal(data, &m) == nil {
			mappings = m
		}
	}
	if mappings == nil {
		mappings = defaultMappingsStruct()
	}

	// cleanpaths.yaml —— 自定义 /api/clean 调用路径
	cleanPathFile := filepath.Join(dir, "cleanpaths.yaml")
	if data, err := os.ReadFile(cleanPathFile); err == nil {
		var cps []model.CleanPath
		if yaml.Unmarshal(data, &cps) == nil {
			cleanPaths = cps
		}
	}
	cleanPaths = normalizeCleanPaths(cleanPaths)

	// filter_sites.yaml —— M3U8 广告过滤站点规则
	filterFile := filepath.Join(dir, "filter_sites.yaml")
	if data, err := os.ReadFile(filterFile); err == nil {
		var fs []model.FilterSite
		if yaml.Unmarshal(data, &fs) == nil {
			normalizeFilterSites(fs)
			filterSites = fs
		}
	}

	return nil
}

// normalizeCleanPaths 规范化：去空白/去重；确保至少有一条启用路径可用
// （用户可自由启停各路径，但全部停用时自动回退到默认 /api/clean）
func normalizeCleanPaths(cps []model.CleanPath) []model.CleanPath {
	out := make([]model.CleanPath, 0, len(cps)+1)
	seen := map[string]bool{}
	enabled := false
	for _, cp := range cps {
		p := normalizeCleanPath(cp.Path)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		cp.Path = p
		if cp.Enabled {
			enabled = true
		}
		out = append(out, cp)
	}
	if !enabled {
		if seen[defaultCleanPath] {
			for i := range out {
				if out[i].Path == defaultCleanPath {
					out[i].Enabled = true
				}
			}
		} else {
			out = append([]model.CleanPath{{Path: defaultCleanPath, Enabled: true, Remark: "默认调用路径"}}, out...)
		}
	}
	return out
}

// normalizeCleanPath 规范化调用路径：必须 / 开头、不带结尾斜杠
func normalizeCleanPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return ""
	}
	return p
}

// normalizeFilterSites 补齐站点规则的默认值并生成缺失的 ID
func normalizeFilterSites(fs []model.FilterSite) {
	for i := range fs {
		if fs[i].ID == "" {
			fs[i].ID = randomID()
		}
		if fs[i].ParseType == "" {
			fs[i].ParseType = "both"
		}
		if fs[i].DetectMode == "" {
			fs[i].DetectMode = "auto"
		}
		if fs[i].IndexMode == "" {
			fs[i].IndexMode = "auto"
		}
		if fs[i].StreamPick == "" {
			fs[i].StreamPick = "first"
		}
	}
}

// PrimaryCleanPath 返回对外展示/生成 qcb 时使用的主调用路径
// 优先默认 /api/clean（若已启用），否则取第一条启用路径
func PrimaryCleanPath() string {
	mu.RLock()
	defer mu.RUnlock()
	for _, cp := range cleanPaths {
		if cp.Enabled && cp.Path == defaultCleanPath {
			return cp.Path
		}
	}
	for _, cp := range cleanPaths {
		if cp.Enabled {
			return cp.Path
		}
	}
	return defaultCleanPath
}

// IsCleanPath 判断某个请求路径是否为用户配置的 clean 调用路径
func IsCleanPath(p string) bool {
	norm := normalizeCleanPath(p)
	if norm == "" {
		return false
	}
	mu.RLock()
	defer mu.RUnlock()
	for _, cp := range cleanPaths {
		if cp.Enabled && cp.Path == norm {
			return true
		}
	}
	return false
}

// MatchFilterSite 按 URL 前缀 / 正则匹配广告过滤站点规则
func MatchFilterSite(u string) *model.FilterSite {
	host := ""
	if pu, err := url.Parse(u); err == nil {
		host = pu.Hostname()
	}
	mu.RLock()
	defer mu.RUnlock()
	var best *model.FilterSite
	for i := range filterSites {
		s := &filterSites[i]
		if !s.Enabled {
			continue
		}
		hit := false
		for _, p := range s.URLPrefixes {
			p = strings.TrimSpace(p)
			if p != "" && host != "" && strings.HasPrefix(host, p) {
				hit = true
				break
			}
		}
		if !hit {
			for _, re := range s.URLRegexes {
				re = strings.TrimSpace(re)
				if re == "" {
					continue
				}
				// 默认只对主机名匹配；正则以 ^ 开头或含 :// 时对整条 URL 匹配
				target := host
				if strings.HasPrefix(re, "^") || strings.Contains(re, "://") {
					target = u
				}
				if ok, err := regexp.MatchString(re, target); err == nil && ok {
					hit = true
					break
				}
			}
		}
		if hit {
			best = s
			break
		}
	}
	return best
}

// Reload 重新从磁盘加载
func Reload() error { return LoadConfig(dataDir) }

// ===== 持久化 =====

func SaveSites(newSites []model.ResourceSite) error {
	mu.Lock()
	defer mu.Unlock()
	sites = newSites
	data, err := yaml.Marshal(sites)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "sites.yaml"), data, 0644)
}

func SaveMappings(newMaps []model.SiteMapping) error {
	mu.Lock()
	defer mu.Unlock()
	mappings = newMaps
	data, err := yaml.Marshal(mappings)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "mappings.yaml"), data, 0644)
}

// SaveCleanPaths 保存自定义调用路径
func SaveCleanPaths(cps []model.CleanPath) error {
	norm := normalizeCleanPaths(cps)
	mu.Lock()
	defer mu.Unlock()
	cleanPaths = norm
	data, err := yaml.Marshal(norm)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "cleanpaths.yaml"), data, 0644)
}

// SaveFilterSites 保存 M3U8 广告过滤站点规则
func SaveFilterSites(fs []model.FilterSite) error {
	normalizeFilterSites(fs)
	data, err := yaml.Marshal(fs)
	if err != nil {
		return err
	}
	mu.Lock()
	filterSites = fs
	mu.Unlock()
	return os.WriteFile(filepath.Join(dataDir, "filter_sites.yaml"), data, 0644)
}

// ===== 默认值 =====

func applyDefaults(cfg *model.AppConfig) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 9991
	}
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "release"
	}
	if cfg.Updater.Interval == "" {
		cfg.Updater.Interval = "0 0 3 * * *"
	}
	if cfg.Updater.Branch == "" {
		cfg.Updater.Branch = "main"
	}
	// 无头浏览器渲染（方案1）：未显式配置 browser 段时默认启用
	if cfg.Browser.TimeoutSec == 0 {
		cfg.Browser.TimeoutSec = 20
	}
	if !cfg.Browser.Enabled && cfg.Browser.ChromePath == "" {
		cfg.Browser.Enabled = true
	}
	// 网页后台（/mx/）：配置段整体缺失时默认启用，并套用默认账号
	if cfg.Admin.Path == "" && cfg.Admin.Username == "" && cfg.Admin.Password == "" {
		cfg.Admin.Enabled = true
	}
	if cfg.Admin.Path == "" {
		cfg.Admin.Path = "/mx"
	}
	if cfg.Admin.Username == "" {
		cfg.Admin.Username = "admin"
	}
	if cfg.Admin.Password == "" {
		cfg.Admin.Password = "mxgjgt"
	}
	if cfg.Admin.SessionHours <= 0 {
		cfg.Admin.SessionHours = 24
	}
}

func defaultConfig() *model.AppConfig {
	return &model.AppConfig{
		Server:   model.ServerConfig{Port: 9991, Mode: "release"},
		Updater:  model.UpdaterConfig{Enabled: false, Interval: "0 0 3 * * *", Branch: "main"},
		Security: model.SecurityConfig{},
		Browser:  model.BrowserConfig{Enabled: true, TimeoutSec: 20},
		Admin:    model.AdminConfig{Enabled: true, Path: "/mx", Username: "admin", Password: "mxgjgt", SessionHours: 24},
	}
}

func defaultMappingsStruct() []model.SiteMapping {
	return []model.SiteMapping{
		{Official: "腾讯视频", Sites: []string{"jjm3u8"}},
		{Official: "爱奇艺", Sites: []string{"jjm3u8"}},
		{Official: "优酷", Sites: []string{"jjm3u8"}},
		{Official: "芒果TV", Sites: []string{"jjm3u8"}},
		{Official: "搜狐视频", Sites: []string{"jjm3u8"}},
		{Official: "乐视视频", Sites: []string{"jjm3u8"}},
		{Official: "哔哩哔哩", Sites: []string{"jjm3u8"}},
	}
}

func randomID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ===== 默认模板 YAML（首次启动写入） =====

func defaultConfigYAML() string {
	return `# =============================================
#  MXGJGT v0.0.14 — 主配置
#  所有配置修改后调用 POST /api/admin/reload 热加载
# =============================================

server:
  port: 9991
  mode: release
  not_found_html: ""

updater:
  enabled: false
  interval: "0 0 3 * * *"   # 秒 分 时 日 月 周
  repo_url: "https://github.com/ssmhdssmhd/MXGJGT"
  branch: "main"
  mirrors:
    - "https://ghproxy.com/https://github.com"

security:
  api_key: ""

# 网页后台（登录后可在浏览器管理资源站 / 映射 / 更新）
# 访问 http://<域名>:9991/mx/  → 登录页
# 关闭 enabled 后 /mx/ 与 /api/admin/* 均走伪装 404
admin:
  enabled: true
  path: "/mx"          # 后台入口路径
  username: "admin"    # 登录用户名
  password: "mxgjgt"   # 登录密码（务必修改）
  session_hours: 24    # 登录态有效期（小时）

# 无头浏览器渲染（方案1 主方案）
# 从官方页面渲染抓取剧名+集数，覆盖爱奇艺/优酷/芒果等对后端强反爬的站点
# 找不到 Chrome/Chromium 时，自动回退到方案2（客户端传参）/方案3（提示加 wd）
browser:
  enabled: true
  chrome_path: ""   # 留空自动探测；也可用环境变量 MXGJGT_CHROME 指定
  timeout_sec: 20
`
}

func defaultSitesYAML() string {
	return `# =============================================
#  资源站列表
#  type: applecms / html
#  url 中用 {keyword} 作为搜索词占位
#  detail_url 用 {id} 作为资源 ID 占位
#  cid: 苹果CMS 分类 ID 逗号分隔（可留空）
# =============================================

- id: "jjm3u8"
  name: "吉吉资源 M3U8"
  type: applecms
  url: "http://caiji.jjzyapi.com/api.php/provide/vod/?ac=list&wd={keyword}"
  detail_url: "http://caiji.jjzyapi.com/api.php/provide/vod/?ac=detail&ids={id}"
  encode: "utf8"
  cid: ""
  enabled: true
`
}

func defaultMappingsYAML() string {
	return `# =============================================
#  官方视频站 → 资源站优先映射
#  sites 列表按优先级排列
# =============================================

- official: "腾讯视频"
  sites: ["jjm3u8"]
- official: "爱奇艺"
  sites: ["jjm3u8"]
- official: "优酷"
  sites: ["jjm3u8"]
- official: "芒果TV"
  sites: ["jjm3u8"]
- official: "搜狐视频"
  sites: ["jjm3u8"]
- official: "乐视视频"
  sites: ["jjm3u8"]
- official: "哔哩哔哩"
  sites: ["jjm3u8"]
`
}

// defaultCleanPathsYAML /api/clean 调用路径（系统设置 · 路径设置）
func defaultCleanPathsYAML() string {
	return `# =============================================
#  调用路径设置（系统设置 · 路径设置）
#  客户端调用地址形如： http://<域名>{path}/?url=<官方链接>
#  可配置多个；默认路径 /api/clean 始终保留
# =============================================

- path: "/api/clean"
  enabled: true
  remark: "默认调用路径"
`
}

// defaultFilterSitesYAML M3U8 广告过滤站点规则
func defaultFilterSitesYAML() string {
	return `# =============================================
#  M3U8 广告过滤（去插播）站点规则
#  url_prefixes: 按主机名前缀匹配（任一命中即命中该站点）
#  url_regexes:  默认只匹配主机名；以 ^ 开头或含 :// 时匹配整条 URL
#  detect_mode:  auto / sequence / blockcut / slicebatch / shortblock / fingerprint / hash / both
#  parse_type:   both / duration / hash / direct
#  index_mode:   auto / single / fixed
#  stream_pick:  first / highest / lowest
#  fingerprint:  广告时长指纹，一行一组（逗号分隔的秒数）
# =============================================

- id: "jjm3u8"
  name: "吉吉资源 M3U8"
  enabled: true
  sort: 0
  parse_type: both
  detect_mode: auto
  index_mode: auto
  stream_pick: first
  fixed_sub: ""
  proxy_url: ""
  url_prefixes: ["svip.jijiplay", "jijiplay"]
  url_regexes: []
  sub_dir: "jjm3u8"
  fingerprint: ""
  regex_list: []
  remark: "示例规则：按主机名前缀匹配吉吉资源播放域名"
`
}
