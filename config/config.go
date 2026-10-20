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

// 更新渠道：stable=正式版（跟随 main 正式发布）/ prerelease=先行版（跟随先行版 dev 预发布）
const (
	ChannelStable     = "stable"
	ChannelPrerelease = "prerelease"
)

var (
	appConfig     *model.AppConfig
	sites         []model.ResourceSite
	mappings      []model.SiteMapping
	cleanPaths    []model.CleanPath
	filterSites   []model.FilterSite
	overlayRules  []model.AdOverlayRule
	playerConfig  model.PlayerConfig
	updateChannel string

	dataDir  string
	cacheDir string
	logsDir  string

	mu sync.RWMutex
)

// GetUpdateChannel 返回当前更新渠道：stable（正式版）/ prerelease（先行版）
func GetUpdateChannel() string {
	mu.RLock()
	defer mu.RUnlock()
	if updateChannel == "" {
		return ChannelStable
	}
	return updateChannel
}

// SaveUpdateChannel 保存更新渠道到 data/updater.yaml
func SaveUpdateChannel(ch string) error {
	ch = normalizeChannel(ch)
	mu.Lock()
	updateChannel = ch
	mu.Unlock()
	data, err := yaml.Marshal(map[string]string{"channel": ch})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "updater.yaml"), data, 0644)
}

// normalizeChannel 仅允许 stable / prerelease，其它值一律回退 stable
func normalizeChannel(ch string) string {
	if strings.EqualFold(strings.TrimSpace(ch), ChannelPrerelease) {
		return ChannelPrerelease
	}
	return ChannelStable
}

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

// GetOverlayRules 获取画面遮罩（内嵌广告）规则
func GetOverlayRules() []model.AdOverlayRule {
	mu.RLock()
	defer mu.RUnlock()
	return overlayRules
}

// GetSecurity 获取安全配置（ST 加密 / API Key）
func GetSecurity() model.SecurityConfig {
	mu.RLock()
	defer mu.RUnlock()
	if appConfig == nil {
		return model.SecurityConfig{}
	}
	return appConfig.Security
}

// SaveSecurity 保存安全配置到 data/security.yaml
// （独立文件存储，避免重写 config.yaml 丢失注释）
func SaveSecurity(sec model.SecurityConfig) error {
	sec.APIKey = strings.TrimSpace(sec.APIKey)
	sec.STSecret = strings.TrimSpace(sec.STSecret)
	if sec.STExpire < 0 {
		sec.STExpire = 0
	}
	mu.Lock()
	if appConfig != nil {
		appConfig.Security = sec
	}
	mu.Unlock()
	data, err := yaml.Marshal(sec)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "security.yaml"), data, 0644)
}

// ===== 播放器控制面板 =====

// GetPlayer 获取播放器控制面板配置
func GetPlayer() model.PlayerConfig {
	mu.RLock()
	defer mu.RUnlock()
	return playerConfig
}

// SavePlayer 保存播放器配置到 data/player.yaml
func SavePlayer(p model.PlayerConfig) error {
	normalizePlayer(&p)
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	mu.Lock()
	playerConfig = p
	mu.Unlock()
	return os.WriteFile(filepath.Join(dataDir, "player.yaml"), data, 0644)
}

// normalizePlayer 规范化播放器配置：补默认值、裁剪取值范围
func normalizePlayer(p *model.PlayerConfig) {
	switch strings.ToLower(strings.TrimSpace(p.AdMode)) {
	case "manual":
		p.AdMode = "manual"
	default:
		p.AdMode = "smart"
	}
	switch strings.ToLower(strings.TrimSpace(p.MaskStyle)) {
	case "blur":
		p.MaskStyle = "blur"
	case "mosaic":
		p.MaskStyle = "mosaic"
	default:
		p.MaskStyle = "cover"
	}
	if strings.TrimSpace(p.MaskColor) == "" {
		p.MaskColor = "#000000"
	}
	if p.MaskOpacity < 0 {
		p.MaskOpacity = 0
	}
	if p.MaskOpacity > 1 {
		p.MaskOpacity = 1
	}
	if p.MaskOpacity == 0 && p.MaskStyle == "cover" {
		p.MaskOpacity = 1 // cover 全透明等于没遮，回退默认
	}
	if p.MaskStrength <= 0 {
		p.MaskStrength = 8
	}
	if p.MaskStrength > 20 {
		p.MaskStrength = 20
	}
	switch strings.ToLower(strings.TrimSpace(p.LineStrategy)) {
	case "quality":
		p.LineStrategy = "quality"
	case "lowad":
		p.LineStrategy = "lowad"
	case "speed":
		p.LineStrategy = "speed"
	default:
		p.LineStrategy = "smart"
	}
	if p.LineTimeoutMS <= 0 {
		p.LineTimeoutMS = 8000
	}
	if p.LineTimeoutMS < 1000 {
		p.LineTimeoutMS = 1000
	}
	if p.LineTimeoutMS > 30000 {
		p.LineTimeoutMS = 30000
	}
	if p.LineConcurrency <= 0 {
		p.LineConcurrency = 4
	}
	if p.LineConcurrency > 16 {
		p.LineConcurrency = 16
	}
	mirrors := make([]string, 0, len(p.Mirrors))
	seen := map[string]bool{}
	for _, m := range p.Mirrors {
		m = strings.TrimRight(strings.TrimSpace(m), "/")
		if m == "" {
			continue
		}
		if !strings.Contains(m, "://") {
			m = "https://" + m
		}
		if seen[m] {
			continue
		}
		seen[m] = true
		mirrors = append(mirrors, m)
	}
	p.Mirrors = mirrors
}

// defaultPlayerConfig 播放器控制面板默认值
func defaultPlayerConfig() model.PlayerConfig {
	return model.PlayerConfig{
		AdBlock:         true,
		AdMode:          "smart",
		FilterSegments:  true,
		MaskStyle:       "cover",
		MaskColor:       "#000000",
		MaskOpacity:     1,
		MaskStrength:    8,
		LineStrategy:    "smart",
		LineTimeoutMS:   8000,
		LineConcurrency: 4,
	}
}

// ===== 首次启动写模板（仅当文件不存在时） =====

func EnsureDefaults() {
	ensureFile(filepath.Join(dataDir, "config.yaml"), defaultConfigYAML())
	ensureFile(filepath.Join(dataDir, "sites.yaml"), defaultSitesYAML())
	ensureFile(filepath.Join(dataDir, "mappings.yaml"), defaultMappingsYAML())
	ensureFile(filepath.Join(dataDir, "cleanpaths.yaml"), defaultCleanPathsYAML())
	ensureFile(filepath.Join(dataDir, "filter_sites.yaml"), defaultFilterSitesYAML())
	ensureFile(filepath.Join(dataDir, "ad_overlay.yaml"), defaultOverlayRulesYAML())
	ensureFile(filepath.Join(dataDir, "updater.yaml"), defaultUpdaterYAML())
	ensureFile(filepath.Join(dataDir, "player.yaml"), defaultPlayerYAML())
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

	// security.yaml —— 安全设置（ST 加密 / 失效 / API Key）
	// 独立文件优先于 config.yaml 的 security 段，便于后台可视化保存
	securityFile := filepath.Join(dir, "security.yaml")
	if data, err := os.ReadFile(securityFile); err == nil {
		var s model.SecurityConfig
		if yaml.Unmarshal(data, &s) == nil {
			appConfig.Security = s
		}
	}
	if appConfig.Security.STExpire < 0 {
		appConfig.Security.STExpire = 0
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

	// ad_overlay.yaml —— 画面遮罩（内嵌广告）规则
	overlayFile := filepath.Join(dir, "ad_overlay.yaml")
	if data, err := os.ReadFile(overlayFile); err == nil {
		var rs []model.AdOverlayRule
		if yaml.Unmarshal(data, &rs) == nil {
			normalizeOverlayRules(rs)
			overlayRules = rs
		}
	}

	// player.yaml —— 播放器控制面板（广告屏蔽 / 智能选线 / 备用入口）
	playerFile := filepath.Join(dir, "player.yaml")
	playerConfig = defaultPlayerConfig()
	if data, err := os.ReadFile(playerFile); err == nil {
		var p model.PlayerConfig
		if yaml.Unmarshal(data, &p) == nil {
			playerConfig = p
		}
	}
	normalizePlayer(&playerConfig)

	// updater.yaml —— 更新渠道（正式版 stable / 先行版 prerelease）
	updateFile := filepath.Join(dir, "updater.yaml")
	if data, err := os.ReadFile(updateFile); err == nil {
		var u struct {
			Channel string `yaml:"channel"`
		}
		if yaml.Unmarshal(data, &u) == nil {
			updateChannel = normalizeChannel(u.Channel)
		}
	}
	if updateChannel == "" {
		updateChannel = ChannelStable
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

// normalizeOverlayRules 补齐画面遮罩规则的默认值并生成缺失的 ID / 区域默认样式
func normalizeOverlayRules(rs []model.AdOverlayRule) {
	for i := range rs {
		if rs[i].ID == "" {
			rs[i].ID = randomID()
		}
		rs[i].SiteIDs = cleanStrings(rs[i].SiteIDs)
		rs[i].SourceURL = strings.TrimSpace(rs[i].SourceURL)
		for j := range rs[i].Regions {
			r := &rs[i].Regions[j]
			if r.Mode == "" {
				r.Mode = "cover"
			}
			if r.Color == "" {
				r.Color = "#000000"
			}
			if r.Opacity <= 0 {
				r.Opacity = 1
			}
		}
	}
}

// cleanStrings 去空白、去空项、去重（保持原有顺序）
func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// overlayBoundToSite 判断画面遮罩规则是否绑定了某个资源站
// siteName 为运行时的资源站名称；ids 为规则里配置的绑定项（可能是站点 ID 也可能是名称）
func overlayBoundToSite(ids []string, siteName string) bool {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" || len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if id == siteName {
			return true
		}
	}
	// 绑定项里既可能写的是站点 ID，也可能写的是名称：按名称反查 ID 再比对
	for _, s := range sites {
		if s.Name != siteName {
			continue
		}
		for _, id := range ids {
			if id == s.ID {
				return true
			}
		}
	}
	return false
}

// matchURLRules 按前缀 / 正则匹配 URL：
// 前缀匹配主机名；正则默认也匹配主机名，以 ^ 开头或含 :// 时匹配整条 URL
func matchURLRules(u, host string, prefixes, regexes []string) bool {
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		if p != "" && host != "" && strings.HasPrefix(host, p) {
			return true
		}
	}
	for _, re := range regexes {
		re = strings.TrimSpace(re)
		if re == "" {
			continue
		}
		target := host
		if strings.HasPrefix(re, "^") || strings.Contains(re, "://") {
			target = u
		}
		if ok, err := regexp.MatchString(re, target); err == nil && ok {
			return true
		}
	}
	return false
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
	host := hostOf(u)
	mu.RLock()
	defer mu.RUnlock()
	for i := range filterSites {
		s := &filterSites[i]
		if !s.Enabled {
			continue
		}
		if matchURLRules(u, host, s.URLPrefixes, s.URLRegexes) {
			return s
		}
	}
	return nil
}

// MatchOverlayRule 为一次播放匹配画面遮罩规则（未命中返回 nil）
// 匹配顺序：① 绑定的资源站优先（siteName 命中即套用，无需再依赖播放域名）
//
//	② 其次按播放域名前缀 / 正则匹配
//
// siteName 为空时仅按 URL 规则匹配（兼容旧调用）
func MatchOverlayRule(u, siteName string) *model.AdOverlayRule {
	host := hostOf(u)
	mu.RLock()
	defer mu.RUnlock()
	// ① 资源站绑定优先：避免为每个播放域名手工配置遮罩
	if strings.TrimSpace(siteName) != "" {
		for i := range overlayRules {
			s := &overlayRules[i]
			if !s.Enabled || len(s.Regions) == 0 {
				continue
			}
			if overlayBoundToSite(s.SiteIDs, siteName) {
				return s
			}
		}
	}
	// ② URL 前缀 / 正则
	for i := range overlayRules {
		s := &overlayRules[i]
		if !s.Enabled || len(s.Regions) == 0 {
			continue
		}
		if matchURLRules(u, host, s.URLPrefixes, s.URLRegexes) {
			return s
		}
	}
	return nil
}

// hostOf 取 URL 主机名（解析失败返回空串）
func hostOf(u string) string {
	if pu, err := url.Parse(u); err == nil {
		return pu.Hostname()
	}
	return ""
}

// HostOf 导出 hostOf，供识别逻辑复用
func HostOf(u string) string { return hostOf(u) }

// FindSiteByHost 按主机名匹配已配置的资源站（用于「自动识别」推荐绑定站点）
// 采用包含关系做宽松匹配，返回命中的站点副本；未命中返回 nil
func FindSiteByHost(host string) *model.ResourceSite {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil
	}
	mu.RLock()
	defer mu.RUnlock()
	var best *model.ResourceSite
	bestLen := 0
	for i := range sites {
		sh := overlaySiteHost(sites[i].URL)
		if sh == "" {
			continue
		}
		if host == sh || strings.HasSuffix(host, "."+sh) || strings.HasSuffix(sh, "."+host) || strings.Contains(host, sh) {
			if len(sh) > bestLen {
				s := sites[i]
				best = &s
				bestLen = len(sh)
			}
		}
	}
	return best
}

// overlaySiteHost 取资源站配置 URL 的主机名（兼容缺少协议、带 {keyword} 占位符的写法）
func overlaySiteHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return strings.ToLower(hostOf(raw))
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

// SaveOverlayRules 保存画面遮罩（内嵌广告）规则
func SaveOverlayRules(rs []model.AdOverlayRule) error {
	normalizeOverlayRules(rs)
	data, err := yaml.Marshal(rs)
	if err != nil {
		return err
	}
	mu.Lock()
	overlayRules = rs
	mu.Unlock()
	return os.WriteFile(filepath.Join(dataDir, "ad_overlay.yaml"), data, 0644)
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
#  MXGJGT v0.0.16 — 主配置
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
  # ST 加密 + 失效：建议在后台「安全设置」里可视化配置
  # 保存后会写入 data/security.yaml（优先级高于本段），密钥仅保存在服务端
  st_enabled: false
  st_secret: ""
  st_expire: 300

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
#  ads: true=广告资源站（走去插播 qcb 后返回干净地址）
#       false=纯净资源站（无广告，直接返回原始地址）
# =============================================

- id: "jjm3u8"
  name: "吉吉资源 M3U8"
  type: applecms
  url: "http://caiji.jjzyapi.com/api.php/provide/vod/?ac=list&wd={keyword}"
  detail_url: "http://caiji.jjzyapi.com/api.php/provide/vod/?ac=detail&ids={id}"
  encode: "utf8"
  cid: ""
  ads: true
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

// defaultOverlayRulesYAML 画面遮罩（内嵌广告 / 贴片）规则
func defaultOverlayRulesYAML() string {
	return `# =============================================
#  画面遮罩（内嵌广告 / 贴片）规则
#  解决「广告文字 / 台标 / 二维码烧进画面像素」的问题：
#  播放器按本规则在对应位置叠加遮罩（零转码、零带宽、秒级生效），
#  与 M3U8 去插播互补，二者可同时启用。
#
#  regions 区域使用「相对比例 0~1」，播放器按实际视频尺寸等比换算，
#  跨分辨率、跨剧集通用；例：左上角台标 → x=0.02 y=0.03 w=0.16 h=0.10
#
#  mode:    cover=纯色遮挡（默认）/ blur=模糊
#  color:   cover 填充色（默认 #000000）
#  opacity: 不透明度 0~1（默认 1）
#
#  url_prefixes: 按播放域名主机名前缀匹配（任一命中即命中该规则）
#  url_regexes:  默认匹配主机名；以 ^ 开头或含 :// 时匹配整条 URL
#  site_ids:     绑定的资源站（ID 或名称）；命中该资源站时**无需**再依赖播放域名，
#                直接套用本规则 —— 用于「绑定资源站、避免手动逐个配遮罩」
# =============================================

- id: "demo"
  name: "示例：左上角台标遮挡"
  enabled: false
  sort: 0
  url_prefixes: ["svip.jijiplay"]
  url_regexes: []
  site_ids: []
  regions:
    - x: 0.02
      y: 0.03
      w: 0.16
      h: 0.10
      mode: cover
      color: "#000000"
      opacity: 1
      label: "左上角台标"
  remark: "示例规则（默认停用）：启用后播放器将在相对坐标区域叠加遮罩"
`
}

// defaultUpdaterYAML 更新渠道（后台「系统 · 更新」可切换，无需手工改本文件）
func defaultUpdaterYAML() string {
	return `# =============================================
#  MXGJGT — 更新渠道
#  channel: stable     = 正式版（跟随 main 的正式发布）
#           prerelease = 先行版（跟随先行版 dev 的预发布）
#  后台「系统 · 更新」可切换，保存后写回本文件
# =============================================

channel: stable
`
}

// defaultPlayerYAML 播放器控制面板（后台「播放器控制面板」可保存，无需手工改本文件）
func defaultPlayerYAML() string {
	return `# =============================================
#  MXGJGT — 播放器控制面板
#  后台「播放器控制面板」保存后写回本文件
#
#  ad_block:         广告屏蔽总开关（含插播片段 + 画面遮罩）
#  ad_mode:          smart=智能屏蔽 / manual=仅手动圈选
#  filter_segments:  插播片段过滤（去插播）；关闭后仅保留画面遮罩
#  mask_style:       cover=纯色遮挡 / blur=模糊 / mosaic=马赛克
#  mask_color:       cover 填充色，如 #000000
#  mask_opacity:     不透明度 0~1（cover 生效）
#  mask_strength:    模糊 / 马赛克强度 1~20
#  line_strategy:    smart=综合推荐 / quality=清晰优先 / lowad=低广告优先 / speed=速度优先
#  line_timeout_ms:  单线路测速超时（毫秒，1000~30000）
#  line_concurrency: 并发测速数（1~16）
#  mirrors:          备用入口域名；生成调用地址时优先使用第一个
# =============================================

ad_block: true
ad_mode: smart
filter_segments: true
mask_style: cover
mask_color: "#000000"
mask_opacity: 1
mask_strength: 8
line_strategy: smart
line_timeout_ms: 8000
line_concurrency: 4
mirrors: []
`
}
