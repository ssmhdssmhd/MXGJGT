// Package service —— 官方站 cover_id → 剧名 字典
//
// 用法:
//
//	service.InitCoverIDDict(config.DataDir())   // main.go 启动时调用一次
//	title, platform, hit := service.LookupCoverID(rawURL)
package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============== 数据结构 ==============

// CoverIDPlatform 单个平台的字典 + 提取 pattern
type CoverIDPlatform struct {
	// 域名别名用于 host 匹配（优先）
	Hosts []string `json:"hosts"`
	// 从 URL path / query 里抠 cover_id 的正则，按顺序尝试
	Patterns []string `json:"patterns"`
	// cover_id → 剧名
	IDs map[string]string `json:"ids"`
}

// CoverIDDict 整个字典文件的根结构
type CoverIDDict map[string]*CoverIDPlatform // key = 平台中文名

// CoverIDEntry 对外展示用的条目（带元数据）
type CoverIDEntry struct {
	CoverID  string `json:"cover_id"`
	Title    string `json:"title"`
	Platform string `json:"platform,omitempty"`
}

// CoverIDStat 平台统计
type CoverIDStat struct {
	Platform string `json:"platform"`
	Count    int    `json:"count"`
	Hosts    []string `json:"hosts"`
}

// ============== 全局状态 ==============

var (
	coverIDMu       sync.RWMutex
	coverIDDict     CoverIDDict
	coverIDPath     string
	coverIDCompiled map[string][]*regexp.Regexp // pattern 编译缓存
)

// InitCoverIDDict 从磁盘加载字典，dataDir 通常传 config.DataDir()
func InitCoverIDDict(dataDir string) error {
	coverIDMu.Lock()
	defer coverIDMu.Unlock()

	if dataDir == "" {
		dataDir = "data"
	}
	coverIDPath = filepath.Join(dataDir, "cover_ids.json")

	// 不存在就自动生成空模板
	if _, err := os.Stat(coverIDPath); os.IsNotExist(err) {
		if err := saveCoverIDDictUnlocked(defaultCoverIDDict()); err != nil {
			return fmt.Errorf("初始化 cover_ids.json 失败: %w", err)
		}
	}

	raw, err := os.ReadFile(coverIDPath)
	if err != nil {
		return fmt.Errorf("读取 cover_ids.json 失败: %w", err)
	}

	var d CoverIDDict
	if err := json.Unmarshal(raw, &d); err != nil {
		return fmt.Errorf("解析 cover_ids.json 失败: %w", err)
	}

	for k, v := range d {
		if v == nil {
			d[k] = &CoverIDPlatform{IDs: map[string]string{}}
		}
		if v.IDs == nil {
			v.IDs = map[string]string{}
		}
	}

	coverIDDict = d
	coverIDCompiled = compilePatterns(d)
	return nil
}

// ReloadCoverIDDict 热重载
func ReloadCoverIDDict() error {
	return InitCoverIDDict(filepath.Dir(coverIDPath))
}

// ============== 查询 ==============

// LookupCoverID 从官方 URL 里抽出 cover_id，到字典里查剧名
// 返回: 剧名, 平台中文名, 是否命中
func LookupCoverID(rawURL string) (title, platform string, hit bool) {
	coverIDMu.RLock()
	defer coverIDMu.RUnlock()

	if coverIDDict == nil {
		return "", "", false
	}

	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "https://" + rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", false
	}

	host := strings.ToLower(u.Host)
	path := u.Path

	// 1. 先按 host 精确匹配（每个平台都有 hosts 列表）
	for name, plat := range coverIDDict {
		if plat == nil {
			continue
		}
		for _, h := range plat.Hosts {
			h = strings.ToLower(h)
			if h != "" && (host == h || strings.HasSuffix(host, "."+h) || strings.Contains(host, h)) {
				if cid := extractCoverID(plat, u); cid != "" {
					if title, ok := plat.IDs[cid]; ok && title != "" {
						return title, name, true
					}
				}
				return "", name, false
			}
		}
	}

	// 2. host 没匹配上 → 全局兜底：遍历所有平台 patterns，从 path 里抠
	for name, plat := range coverIDDict {
		if plat == nil {
			continue
		}
		if cid := extractCoverID(plat, u); cid != "" {
			if title, ok := plat.IDs[cid]; ok && title != "" {
				return title, name, true
			}
		}
	}
	_ = path
	return "", "", false
}

// extractCoverID 用平台的 patterns 从 URL 抠 cover_id
func extractCoverID(plat *CoverIDPlatform, u *url.URL) string {
	if plat == nil {
		return ""
	}

	// 编译缓存
	patterns, ok := coverIDCompiled[plat.hostKey()]
	if !ok {
		patterns = compilePatternStrings(plat.Patterns)
	}

	haystacks := []string{u.Path, u.RawQuery, u.String()}
	for _, re := range patterns {
		for _, hay := range haystacks {
			if m := re.FindStringSubmatch(hay); len(m) >= 2 {
				return strings.TrimSpace(m[1])
			}
		}
	}
	return ""
}

// hostKey 拿平台第一个 host 当 map key
func (p *CoverIDPlatform) hostKey() string {
	if len(p.Hosts) > 0 {
		return p.Hosts[0]
	}
	return ""
}

func compilePatterns(d CoverIDDict) map[string][]*regexp.Regexp {
	m := map[string][]*regexp.Regexp{}
	for _, v := range d {
		if v == nil {
			continue
		}
		m[v.hostKey()] = compilePatternStrings(v.Patterns)
	}
	return m
}

func compilePatternStrings(list []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range list {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if re, err := regexp.Compile(p); err == nil {
			out = append(out, re)
		}
	}
	return out
}

// ============== 管理：增删改查 ==============

// ListCoverIDStats 所有平台统计
func ListCoverIDStats() []CoverIDStat {
	coverIDMu.RLock()
	defer coverIDMu.RUnlock()

	var out []CoverIDStat
	for name, plat := range coverIDDict {
		if plat == nil {
			continue
		}
		out = append(out, CoverIDStat{
			Platform: name,
			Count:    len(plat.IDs),
			Hosts:    plat.Hosts,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Platform < out[j].Platform })
	return out
}

// ListCoverIDs 列出某平台所有条目（按 cover_id 排序）
func ListCoverIDs(platform string) ([]CoverIDEntry, error) {
	coverIDMu.RLock()
	defer coverIDMu.RUnlock()

	plat, ok := coverIDDict[platform]
	if !ok || plat == nil {
		return nil, fmt.Errorf("平台 %s 不存在", platform)
	}

	var out []CoverIDEntry
	for cid, t := range plat.IDs {
		out = append(out, CoverIDEntry{CoverID: cid, Title: t, Platform: platform})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CoverID < out[j].CoverID })
	return out, nil
}

// SaveCoverID 新增 / 更新 一条。platform 不存在会自动创建
func SaveCoverID(platform, coverID, title string) error {
	platform = strings.TrimSpace(platform)
	coverID = strings.TrimSpace(coverID)
	title = strings.TrimSpace(title)
	if platform == "" || coverID == "" || title == "" {
		return fmt.Errorf("platform / cover_id / title 均不能为空")
	}

	coverIDMu.Lock()
	defer coverIDMu.Unlock()

	if coverIDDict == nil {
		coverIDDict = defaultCoverIDDict()
	}
	if coverIDDict[platform] == nil {
		coverIDDict[platform] = &CoverIDPlatform{IDs: map[string]string{}}
	}
	coverIDDict[platform].IDs[coverID] = title
	return saveCoverIDDictUnlocked(coverIDDict)
}

// DeleteCoverID 删除一条
func DeleteCoverID(platform, coverID string) error {
	coverIDMu.Lock()
	defer coverIDMu.Unlock()

	plat, ok := coverIDDict[platform]
	if !ok || plat == nil {
		return fmt.Errorf("平台 %s 不存在", platform)
	}
	if _, ok := plat.IDs[coverID]; !ok {
		return fmt.Errorf("cover_id %s 不存在于 %s", coverID, platform)
	}
	delete(plat.IDs, coverID)
	return saveCoverIDDictUnlocked(coverIDDict)
}

// GetPlatforms 返回所有已注册平台名
func GetPlatforms() []string {
	coverIDMu.RLock()
	defer coverIDMu.RUnlock()
	var out []string
	for k := range coverIDDict {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ============== 持久化 ==============

func saveCoverIDDictUnlocked(d CoverIDDict) error {
	if coverIDPath == "" {
		coverIDPath = "data/cover_ids.json"
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	// 原子写：先写 .tmp 再 rename
	tmp := coverIDPath + ".tmp." + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, coverIDPath)
}

// ============== 默认模板 ==============

// defaultCoverIDDict 返回预置的各平台 host + pattern 骨架（ids 为空）
// 便于首次启动就具备 host 匹配能力，只需往里填 cover_id → 剧名
func defaultCoverIDDict() CoverIDDict {
	return CoverIDDict{
		"腾讯视频": {
			Hosts: []string{"v.qq.com", "film.qq.com", "video.qq.com", "m.v.qq.com"},
			Patterns: []string{
				`/x/cover/([a-zA-Z0-9]+)`,          // /x/cover/abc123.html
				`/cover/([a-zA-Z0-9]+)`,              // 通用兜底
				`vid=([a-zA-Z0-9]+)`,                 // query 里的 vid
				`/([a-zA-Z0-9]{6,})\.html`,           // path 兜底长串
			},
			IDs: map[string]string{},
		},
		"爱奇艺": {
			Hosts: []string{"iqiyi.com", "www.iqiyi.com", "m.iqiyi.com", "v.iqiyi.com", "qy.com"},
			Patterns: []string{
				`/v_([a-zA-Z0-9]+)`,                  // /v_abc123.html
				`/play/([a-zA-Z0-9]+)`,
				`vid=([a-zA-Z0-9]+)`,
				`aid=(\d+)`,
			},
			IDs: map[string]string{},
		},
		"优酷": {
			Hosts: []string{"youku.com", "www.youku.com", "v.youku.com", "m.youku.com"},
			Patterns: []string{
				`id_([a-zA-Z0-9]+)`,                  // id_XQ==.html
				`/v_show/id_([a-zA-Z0-9]+)`,
				`vid=([a-zA-Z0-9]+)`,
			},
			IDs: map[string]string{},
		},
		"芒果TV": {
			Hosts: []string{"mgtv.com", "www.mgtv.com", "v.mgtv.com", "m.mgtv.com"},
			Patterns: []string{
				`/b/(\d+)`,                           // /b/12345678.html
				`/play/(\d+)`,
				`cid=(\d+)`,
			},
			IDs: map[string]string{},
		},
		"搜狐视频": {
			Hosts: []string{"tv.sohu.com", "sohu.com"},
			Patterns: []string{
				`/v/([a-zA-Z0-9]+)`,                  // /v/abc12345.html
				`vid=(\d+)`,
				`aid=(\d+)`,
			},
			IDs: map[string]string{},
		},
		"乐视视频": {
			Hosts: []string{"letv.com", "www.letv.com", "video.le.com"},
			Patterns: []string{
				`/v_([a-zA-Z0-9]+)`,
				`pid=(\d+)`,
			},
			IDs: map[string]string{},
		},
		"哔哩哔哩": {
			Hosts: []string{"bilibili.com", "www.bilibili.com", "m.bilibili.com", "b23.tv"},
			Patterns: []string{
				`(BV[a-zA-Z0-9]{10})`,                // BV号
				`/av(\d+)`,                           // av号
				`bvid=(BV[a-zA-Z0-9]{10})`,
			},
			IDs: map[string]string{},
		},
	}
}
