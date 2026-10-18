// Package config 配置管理 — 支持首次启动自动写模板 / 热加载 / 持久化
package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"

	"mxgjgt/model"

	"gopkg.in/yaml.v3"
)

var (
	appConfig *model.AppConfig
	sites     []model.ResourceSite
	mappings  []model.SiteMapping

	dataDir   string
	cacheDir  string
	logsDir   string

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

// ===== 首次启动写模板（仅当文件不存在时） =====

func EnsureDefaults() {
	ensureFile(filepath.Join(dataDir, "config.yaml"), defaultConfigYAML())
	ensureFile(filepath.Join(dataDir, "sites.yaml"), defaultSitesYAML())
	ensureFile(filepath.Join(dataDir, "mappings.yaml"), defaultMappingsYAML())
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

	return nil
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
#  MXGJGT v0.0.10 — 主配置
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
