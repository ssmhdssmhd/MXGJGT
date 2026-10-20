// Package service — 线路测速与智能选线
//
// 对应「智能选线」能力：给定多条线路，实测可用性 / 首屏耗时 / 清晰度 /
// 下载速度 / 广告片段占比，再按策略（综合推荐、清晰优先、低广告优先、速度优先）打分排序。
package service

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"mxgjgt/model"
)

var resolutionRe = regexp.MustCompile(`(?i)RESOLUTION=(\d+)x(\d+)`)

// lineVariant 主索引中的一条清晰度子列表
type lineVariant struct {
	URI       string
	Bandwidth int
	Width     int
	Height    int
}

// parseLineVariants 解析主索引的 #EXT-X-STREAM-INF 及其子列表地址
func parseLineVariants(text string) []lineVariant {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var out []lineVariant
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(strings.ToUpper(line), "#EXT-X-STREAM-INF") {
			continue
		}
		v := lineVariant{}
		if m := bandwidthRe.FindStringSubmatch(line); len(m) == 2 {
			fmt.Sscanf(m[1], "%d", &v.Bandwidth)
		}
		if m := resolutionRe.FindStringSubmatch(line); len(m) == 3 {
			fmt.Sscanf(m[1], "%d", &v.Width)
			fmt.Sscanf(m[2], "%d", &v.Height)
		}
		for j := i + 1; j < len(lines); j++ {
			nxt := strings.TrimSpace(lines[j])
			if nxt == "" || strings.HasPrefix(nxt, "#") {
				continue
			}
			v.URI = nxt
			i = j
			break
		}
		if v.URI != "" {
			out = append(out, v)
		}
	}
	return out
}

// pickLineVariant 按策略选择清晰度子列表
func pickLineVariant(vs []lineVariant, strategy string) lineVariant {
	best := vs[0]
	score := func(v lineVariant) float64 {
		switch strategy {
		case "quality":
			// 分辨率优先，同分辨率看码率
			return float64(v.Width*v.Height)*1000 + float64(v.Bandwidth)
		case "speed", "lowad":
			// 速度 / 低广告策略优先小码率，起播更快
			return -float64(v.Bandwidth)
		default:
			return float64(v.Bandwidth)
		}
	}
	for _, v := range vs[1:] {
		if score(v) > score(best) {
			best = v
		}
	}
	return best
}

// ProbeLine 对一条线路做测速
// rawURL 可以是 m3u8 直链，也可以是播放页（自动解析出直链）
func ProbeLine(rawURL string, site *model.FilterSite, strategy string, timeout time.Duration) *model.LineProbeResult {
	res := &model.LineProbeResult{URL: strings.TrimSpace(rawURL)}
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		res.Error = "地址为空"
		return res
	}
	base, err := url.Parse(raw)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		res.Error = "地址无效"
		return res
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	client := httpClientFor(site)
	client.Timeout = timeout

	// 1. 拉取索引（播放页则先解析出 m3u8）
	start := time.Now()
	text, finalURL, kind, err := fetchPlaylistText(client, raw)
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	res.Kind = kind
	if b, e := url.Parse(finalURL); e == nil {
		base = b
	}

	// 2. 主索引 → 按策略选一条子列表
	if vs := parseLineVariants(text); len(vs) > 0 {
		res.Kind = "master"
		pick := pickLineVariant(vs, strategy)
		res.Width, res.Height = pick.Width, pick.Height
		sub := absolutize(base, pick.URI)
		subText, subURL, _, e := fetchPlaylistText(client, sub)
		if e != nil {
			res.Error = "子索引拉取失败：" + e.Error()
			return res
		}
		text = subText
		if b, e2 := url.Parse(subURL); e2 == nil {
			base = b
		}
	}

	// 3. 统计片段 / 时长 / 广告占比
	_, segs, _, _ := parseM3U8(text)
	res.Segments = len(segs)
	for _, s := range segs {
		res.DurationSec += s.Duration
	}
	if res.Segments == 0 {
		res.Error = "索引中没有可用片段（可能为空列表或已失效）"
		return res
	}
	res.AdSegments = countAdSegments(segs, site)
	res.AdRatio = float64(res.AdSegments) / float64(res.Segments)

	// 4. 首个分片测速（最多读 256KB，避免拖慢测速）
	if segs[0] != nil {
		if kb, e := sampleSegmentSpeed(client, absolutize(base, segs[0].URI), 256<<10); e == nil {
			res.SpeedKBps = math.Round(kb*10) / 10
		}
	}

	res.Score = lineScore(res, strategy)
	return res
}

// RankLines 按得分排序并写入排名（可用线路在前）
func RankLines(results []*model.LineProbeResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].OK != results[j].OK {
			return results[i].OK
		}
		return results[i].Score > results[j].Score
	})
	for i, r := range results {
		if !r.OK {
			r.Rank = 0
			continue
		}
		r.Rank = i + 1
	}
}

// lineScore 按策略计算 0~100 综合得分
func lineScore(r *model.LineProbeResult, strategy string) float64 {
	if !r.OK || r.Segments == 0 {
		return 0
	}
	speed := math.Min(r.SpeedKBps/2048, 1) * 100          // 2MB/s 记满分
	latency := math.Max(0, 1-float64(r.LatencyMS)/3000) * 100
	ads := (1 - r.AdRatio) * 100
	quality := 45.0 // 未知清晰度给中间分
	if r.Width > 0 && r.Height > 0 {
		quality = math.Min(float64(r.Width*r.Height)/float64(1920*1080), 1) * 100
	}
	switch strategy {
	case "quality":
		return quality*0.75 + speed*0.15 + latency*0.10
	case "lowad":
		return ads*0.75 + speed*0.15 + latency*0.10
	case "speed":
		return speed*0.75 + latency*0.25
	default: // smart 综合推荐
		return speed*0.35 + latency*0.20 + ads*0.20 + quality*0.25
	}
}

// countAdSegments 按站点规则统计广告片段数（无规则返回 0）
func countAdSegments(segs []*M3U8Segment, site *model.FilterSite) int {
	if site == nil || len(segs) == 0 {
		return 0
	}
	switch detectModeOf(site) {
	case "fingerprint":
		return len(matchFingerprint(segs, fpOf(site)))
	case "sequence":
		return len(matchSequence(segs))
	case "regex":
		return len(matchRegex(segs, regexListOf(site)))
	case "blockcut", "slicebatch", "shortblock":
		return len(matchBlock(segs))
	}
	return 0
}

// fetchPlaylistText 拉取 m3u8 文本；若为播放页则解析出直链后再拉一次
// 返回：文本、最终地址、类型（media / master / page）
func fetchPlaylistText(client *http.Client, raw string) (string, string, string, error) {
	text, finalURL, err := httpGetText(client, raw)
	if err != nil {
		return "", raw, "", err
	}
	if strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		return text, finalURL, "media", nil
	}
	// 播放页：从 HTML 中提取 m3u8
	rel := ExtractM3U8(text)
	if rel == "" {
		return "", finalURL, "", fmt.Errorf("不是有效的 m3u8（可能已失效或需要鉴权）")
	}
	u, err := url.Parse(finalURL)
	if err != nil {
		return "", finalURL, "", err
	}
	ref, err := url.Parse(rel)
	if err != nil {
		return "", finalURL, "", err
	}
	abs := u.ResolveReference(ref).String()
	t2, f2, err := httpGetText(client, abs)
	if err != nil {
		return "", abs, "page", err
	}
	return t2, f2, "page", nil
}

// httpGetText GET 文本（带 UA / Referer，限制 4MB）
func httpGetText(client *http.Client, raw string) (string, string, error) {
	req, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		return "", raw, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	if u, err := url.Parse(raw); err == nil {
		req.Header.Set("Referer", u.Scheme+"://"+u.Host+"/")
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", raw, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", raw, fmt.Errorf("请求失败 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", raw, err
	}
	return string(b), resp.Request.URL.String(), nil
}

// sampleSegmentSpeed 采样首个分片下载速度（KB/s）
func sampleSegmentSpeed(client *http.Client, segURL string, maxBytes int64) (float64, error) {
	req, err := http.NewRequest("GET", segURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	if u, err := url.Parse(segURL); err == nil {
		req.Header.Set("Referer", u.Scheme+"://"+u.Host+"/")
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("分片请求失败 HTTP %d", resp.StatusCode)
	}
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxBytes))
	if err != nil || n == 0 {
		return 0, fmt.Errorf("分片读取失败")
	}
	elapsed := time.Since(start).Seconds()
	if elapsed <= 0 {
		elapsed = 0.001
	}
	return float64(n) / 1024 / elapsed, nil
}
