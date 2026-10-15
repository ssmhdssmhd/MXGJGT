// Package config 配置管理
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
	mu        sync.RWMutex
)

// InitDataDir 初始化数据目录
func InitDataDir(dir string) { dataDir = dir }

func GetConfig() *model.AppConfig {
	mu.RLock(); defer mu.RUnlock()
	return appConfig
}

func GetSites() []model.ResourceSite {
	mu.RLock(); defer mu.RUnlock()
	return sites
}

func GetMappings() []model.SiteMapping {
	mu.RLock(); defer mu.RUnlock()
	return mappings
}

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
		mappings = defaultMappings()
	}

	return nil
}

// Reload 重新从磁盘加载
func Reload() error { return LoadConfig(dataDir) }

// SaveSites 保存资源站到文件
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

// SaveMappings 保存映射到文件
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

func applyDefaults(cfg *model.AppConfig) {
	if cfg.Server.Port == 0 { cfg.Server.Port = 8080 }
	if cfg.Server.Mode == "" { cfg.Server.Mode = "release" }
	if cfg.Updater.Interval == "" { cfg.Updater.Interval = "0 0 3 * * *" }
	if cfg.Updater.Branch == "" { cfg.Updater.Branch = "main" }
}

func defaultConfig() *model.AppConfig {
	return &model.AppConfig{
		Server:   model.ServerConfig{Port: 8080, Mode: "release"},
		Updater:  model.UpdaterConfig{Enabled: false, Interval: "0 0 3 * * *", Branch: "main"},
		Security: model.SecurityConfig{},
	}
}

func defaultMappings() []model.SiteMapping {
	return []model.SiteMapping{
		{Official: "腾讯视频", Sites: []string{"jjm3u8"}},
		{Official: "爱奇艺",   Sites: []string{"jjm3u8"}},
		{Official: "优酷",     Sites: []string{"jjm3u8"}},
		{Official: "芒果TV",   Sites: []string{"jjm3u8"}},
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
