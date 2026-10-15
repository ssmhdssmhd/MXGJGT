package config

import (
	"os"
	"path/filepath"
	"sync"

	"mxgjgt/model"

	"gopkg.in/yaml.v3"
)

var (
	appConfig *model.AppConfig
	sites     []model.ResourceSite
	mu        sync.RWMutex
)

// GetConfig 获取应用配置
func GetConfig() *model.AppConfig {
	mu.RLock()
	defer mu.RUnlock()
	return appConfig
}

// GetSites 获取资源站列表
func GetSites() []model.ResourceSite {
	mu.RLock()
	defer mu.RUnlock()
	return sites
}

// LoadConfig 从指定目录加载配置
func LoadConfig(dataDir string) error {
	mu.Lock()
	defer mu.Unlock()

	// 加载 config.yaml
	cfgPath := filepath.Join(dataDir, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		appConfig = defaultConfig()
		return nil
	}

	var cfg model.AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	applyDefaults(&cfg)
	appConfig = &cfg

	// 加载资源站
	sitesPath := filepath.Join(dataDir, "sites.yaml")
	sitesData, err := os.ReadFile(sitesPath)
	if err == nil {
		var loadedSites []model.ResourceSite
		if yaml.Unmarshal(sitesData, &loadedSites) == nil {
			sites = loadedSites
		}
	}
	return nil
}

// Reload 热重载配置
func Reload(dataDir string) error {
	return LoadConfig(dataDir)
}

func defaultConfig() *model.AppConfig {
	return &model.AppConfig{
		Server: model.ServerConfig{
			Port:         8080,
			Mode:         "release",
			NotFoundHTML: "",
		},
		Updater: model.UpdaterConfig{
			Enabled:  false,
			Interval: "0 0 3 * * *",
			Branch:   "main",
			RepoURL:  "https://github.com/example/mxgjgt.git",
			Mirrors: []string{
				"https://ghproxy.com/{origin}",
				"https://mirror.ghproxy.com/{origin}",
				"https://gh-proxy.com/{origin}",
			},
			UseProxy: true,
		},
		Security: model.SecurityConfig{
			APIKey: "",
		},
	}
}

func applyDefaults(cfg *model.AppConfig) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
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
}
