// Package utils 工具函数
package utils

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPClient 带超时的 http 客户端
var HTTPClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   true,
	},
}

// GetHTML 发起 GET 请求获取 HTML
func GetHTML(targetURL string, headers map[string]string) (string, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// URLDecode URL 解码
func URLDecode(s string) string {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// URLEncode URL 编码
func URLEncode(s string) string {
	return url.QueryEscape(s)
}

// TrimAll 去除所有空白字符
func TrimAll(s string) string {
	return strings.Join(strings.Fields(s), "")
}

// ContainsAny 判断字符串是否包含任一关键字
func ContainsAny(s string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
