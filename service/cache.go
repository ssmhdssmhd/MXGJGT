// Package service — 简单内存缓存（可选磁盘持久化）
package service

import (
	"encoding/json"
	"hash/fnv"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mxgjgt/config"
)

type cacheEntry struct {
	Value     []byte    `json:"v"`
	ExpiresAt time.Time `json:"e"`
}

type memCache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
	ttl   time.Duration
}

var globalCache = &memCache{
	items: make(map[string]cacheEntry),
	ttl:   5 * time.Minute, // 默认 5 分钟
}

func hashKey(s string) string {
	h := fnv.New64a()
	h.Write([]byte(s))
	return "c" + string(h.Sum(nil))
}

func cacheSet(key string, value any, ttl time.Duration) {
	if ttl <= 0 {
		ttl = globalCache.ttl
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}

	globalCache.mu.Lock()
	globalCache.items[key] = cacheEntry{Value: data, ExpiresAt: time.Now().Add(ttl)}
	globalCache.mu.Unlock()

	// 同时写磁盘（可选，cache 目录存在就写）
	if config.CacheDir() != "" {
		os.MkdirAll(config.CacheDir(), 0755)
		os.WriteFile(filepath.Join(config.CacheDir(), hashKey(key)+".json"), data, 0644)
	}
}

func cacheGet(key string) ([]byte, bool) {
	globalCache.mu.RLock()
	e, ok := globalCache.items[key]
	globalCache.mu.RUnlock()
	if ok && time.Now().Before(e.ExpiresAt) {
		return e.Value, true
	}

	// 磁盘兜底
	if config.CacheDir() != "" {
		if b, err := os.ReadFile(filepath.Join(config.CacheDir(), hashKey(key)+".json")); err == nil {
			// 回填内存
			globalCache.mu.Lock()
			globalCache.items[key] = cacheEntry{Value: b, ExpiresAt: time.Now().Add(globalCache.ttl)}
			globalCache.mu.Unlock()
			return b, true
		}
	}
	return nil, false
}

func cachePurge() {
	globalCache.mu.Lock()
	defer globalCache.mu.Unlock()
	now := time.Now()
	for k, v := range globalCache.items {
		if now.After(v.ExpiresAt) {
			delete(globalCache.items, k)
		}
	}
}

// CacheSetTTL 运行时动态调整 TTL（秒）
func CacheSetTTL(seconds int) {
	globalCache.ttl = time.Duration(seconds) * time.Second
}

// CacheStats 返回内存缓存统计
func CacheStats() map[string]any {
	globalCache.mu.RLock()
	count := len(globalCache.items)
	expired := 0
	now := time.Now()
	for _, v := range globalCache.items {
		if now.After(v.ExpiresAt) {
			expired++
		}
	}
	globalCache.mu.RUnlock()
	return map[string]any{
		"memory_items": count,
		"expired":      expired,
		"ttl_seconds":  int(globalCache.ttl.Seconds()),
	}
}
