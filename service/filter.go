// Package service — M3U8 广告过滤（去插播）核心引擎
//
// 目标：把采集站/播放站返回的 m3u8 索引里的「插播广告」片段剔除，
// 输出一份仍可直接播放的 m3u8（片段地址保持指向源站 CDN 的绝对地址，
// 因此播放器只会向我们取一次索引，不需要代理转发 TS，零额外带宽）。
//
// 检测方式（与参考站字段体系对齐，映射到下列可离线复现的原语）：
//
//	fingerprint  时长指纹：广告片段的时长序列特征（站点规则里配置多组）
//	hash         内容哈希：逐段采样比对内容，找出反复出现的填充/广告片段
//	sequence     TS 序列连贯性：片段文件名数字序列出现断裂 → 判定插播
//	regex        片段级正则剔除
//	blockcut / slicebatch / shortblock
//	             块分析：按 #EXT-X-DISCONTINUITY 切块后，明显偏短的孤立块判为广告
//	direct       直链播放：不做过滤，原样输出
//	auto         自动：上述方式按可用性组合
package service

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mxgjgt/model"
)

// ======== 数据结构 ========

// M3U8Segment 单个媒体片段
type M3U8Segment struct {
	Index    int      // 序号（0 起）
	Duration float64  // #EXTINF 时长（秒）
	URI      string   // 原始 URI（可能是相对地址）
	AbsURL   string   // 绝对地址
	Ad       bool     // 是否被判定为广告
	Reason   string   // 判定原因
	Tags     []string // 片段的前导标签（原样保留）
	Break    bool     // 该片段前是否有 #EXT-X-DISCONTINUITY
}

// FilterResult 过滤结果
type FilterResult struct {
	Playlist string // 过滤后的 m3u8 文本
	IndexURL string // 实际解析到的媒体索引地址
	IsMaster bool   // 原始地址是否为主索引
	Variants []string
	Total    int
	Kept     int
	Removed  int
	AdRanges []string
	Mode     string
	Segments []*M3U8Segment
	Cached   bool
}

// ======== 缓存（内存 TTL） ========

type filterCacheItem struct {
	text   string
	result *FilterResult
	expire time.Time
}

var (
	filterCacheMu sync.RWMutex
	filterCache   = map[string]filterCacheItem{}
	filterCacheTTL = 30 * time.Minute
)

func filterCacheKey(siteID, u string) string {
	return siteID + "|" + u
}

func filterCacheGet(key string) (string, *FilterResult, bool) {
	filterCacheMu.RLock()
	it, ok := filterCache[key]
	filterCacheMu.RUnlock()
	if !ok || time.Now().After(it.expire) {
		return "", nil, false
	}
	return it.text, it.result, true
}

func filterCachePut(key, text string, r *FilterResult) {
	filterCacheMu.Lock()
	filterCache[key] = filterCacheItem{text: text, result: r, expire: time.Now().Add(filterCacheTTL)}
	filterCacheMu.Unlock()
}

// ======== 调用日志（内存，最近 N 条） ========

const filterLogMax = 200

var (
	filterLogMu sync.Mutex
	filterLogs  []model.FilterRecord
)

// AddFilterRecord 记录一次过滤调用（滚动保留最近 filterLogMax 条）
func AddFilterRecord(r model.FilterRecord) {
	filterLogMu.Lock()
	filterLogs = append(filterLogs, r)
	if len(filterLogs) > filterLogMax {
		filterLogs = filterLogs[len(filterLogs)-filterLogMax:]
	}
	filterLogMu.Unlock()
}

// GetFilterRecords 返回最近的调用记录（最新在前）
func GetFilterRecords() []model.FilterRecord {
	filterLogMu.Lock()
	defer filterLogMu.Unlock()
	out := make([]model.FilterRecord, 0, len(filterLogs))
	for i := len(filterLogs) - 1; i >= 0; i-- {
		out = append(out, filterLogs[i])
	}
	return out
}

// ======== HTTP 拉取 ========

// httpClientFor 按站点规则构造 HTTP 客户端
// proxy_url 留空=跟随环境变量（便于容器/代理环境）；填 - 表示强制直连；其它按 url 解析
func httpClientFor(site *model.FilterSite) *http.Client {
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     60 * time.Second,
	}
	if site != nil {
		p := strings.TrimSpace(site.ProxyURL)
		if p == "-" {
			tr.Proxy = nil
		} else if p != "" {
			if pu, err := url.Parse(p); err == nil {
				tr.Proxy = http.ProxyURL(pu)
			}
		}
	}
	return &http.Client{Transport: tr, Timeout: 20 * time.Second}
}

// FetchText 拉取文本（m3u8 索引），自动带上 UA 与 Referer
func FetchText(rawURL string, site *model.FilterSite) (string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	if pu, err := url.Parse(rawURL); err == nil {
		req.Header.Set("Referer", pu.Scheme+"://"+pu.Host+"/")
	}
	resp, err := httpClientFor(site).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("索引请求失败 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ======== 解析 ========

var uriAttrRe = regexp.MustCompile(`URI="([^"]+)"`)
var streamInfRe = regexp.MustCompile(`(?i)#EXT-X-STREAM-INF:[^\r\n]*`)
var bandwidthRe = regexp.MustCompile(`(?i)BANDWIDTH=(\d+)`)

// parseM3U8 解析 m3u8 文本
// 返回：头部标签、片段列表、是否主索引、子列表 URI
//
// 标签分为两类：
//   - 播放列表级（#EXTM3U / #EXT-X-VERSION / #EXT-X-TARGETDURATION …）：只进 headTags
//   - 片段级（#EXTINF / #EXT-X-KEY / #EXT-X-MAP / #EXT-X-DISCONTINUITY …）：只进所属片段 Tags
//
// 这样重写时不会把播放列表头重复写进片段、也不会把片段的 #EXTINF 混进头部。
func parseM3U8(text string) (headTags []string, segs []*M3U8Segment, isMaster bool, variants []string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var pending []string
	seenSegment := false
	pendingBreak := false
	for i := 0; i < len(lines); i++ {
		raw := strings.TrimSpace(lines[i])
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "#") {
			if strings.HasPrefix(raw, "#EXT-X-STREAM-INF") {
				isMaster = true
				// 紧随其后的非注释行是子列表地址
				for j := i + 1; j < len(lines); j++ {
					nxt := strings.TrimSpace(lines[j])
					if nxt == "" || strings.HasPrefix(nxt, "#") {
						continue
					}
					variants = append(variants, nxt)
					i = j
					break
				}
				continue
			}
			if !seenSegment && !isSegmentTag(raw) {
				headTags = append(headTags, raw)
			}
			// #EXT-X-DISCONTINUITY 表示「其后的片段」与前面不连续 → 记到下一个片段上
			if strings.HasPrefix(raw, "#EXT-X-DISCONTINUITY") {
				pendingBreak = true
			}
			pending = append(pending, raw)
			continue
		}
		// 片段行
		seenSegment = true
		s := &M3U8Segment{Index: len(segs), URI: raw, Break: pendingBreak}
		pendingBreak = false
		for _, t := range pending {
			if !isSegmentTag(t) {
				continue
			}
			s.Tags = append(s.Tags, t)
			if strings.HasPrefix(t, "#EXTINF:") {
				if v := extinfDuration(t); v > 0 {
					s.Duration = v
				}
			}
		}
		pending = nil
		segs = append(segs, s)
	}
	return
}

// isSegmentTag 判断是否为「片段级」标签（作用于其后的单个片段）
func isSegmentTag(t string) bool {
	up := strings.ToUpper(strings.TrimSpace(t))
	prefixes := []string{
		"#EXTINF",
		"#EXT-X-BYTERANGE",
		"#EXT-X-DISCONTINUITY",
		"#EXT-X-KEY",
		"#EXT-X-MAP",
		"#EXT-X-PROGRAM-DATE-TIME",
		"#EXT-X-DATERANGE",
		"#EXT-X-CUE",
		"#EXT-X-ASSET",
		"#EXT-X-SCTE35",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(up, p) {
			return true
		}
	}
	return false
}

// extinfDuration 取 #EXTINF 的时长
func extinfDuration(tag string) float64 {
	v := strings.TrimSpace(strings.TrimPrefix(tag, "#EXTINF:"))
	if i := strings.IndexAny(v, ","); i >= 0 {
		v = v[:i]
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0
	}
	return f
}

// absolutize 把相对 URI 补全为绝对地址（保留 query 里的 sign 等参数）
func absolutize(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return base.Scheme + ":" + raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

// ======== 广告识别原语 ========

// matchFingerprint 时长指纹匹配：返回被判定为广告的序号集合
func matchFingerprint(segs []*M3U8Segment, fingerprint string) map[int]bool {
	ad := map[int]bool{}
	groups := parseFingerprint(fingerprint)
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		for i := 0; i+len(g) <= len(segs); i++ {
			hit := true
			for j, want := range g {
				d := segs[i+j].Duration
				// 容差 0.15s，兼顾采集站四舍五入
				if d <= 0 || absF(d-want) > 0.15 {
					hit = false
					break
				}
			}
			if hit {
				for j := range g {
					ad[i+j] = true
				}
			}
		}
	}
	return ad
}

// parseFingerprint 解析指纹配置：一行一组，逗号分隔秒数
func parseFingerprint(s string) [][]float64 {
	var out [][]float64
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var g []float64
		for _, part := range strings.Split(line, ",") {
			if f, err := strconv.ParseFloat(strings.TrimSpace(part), 64); err == nil && f > 0 {
				g = append(g, f)
			}
		}
		if len(g) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// matchSequence TS 序列连贯性：文件名数字序列出现断裂 → 断裂点所在片段判为插播
//
// 关键约束：仅当整条索引确实采用「连续数字」命名（如 0.ts / 1.ts / … / 123.ts）时，
// 序列断裂才是插播信号。若文件名是 hash（如 a1b2c3d4.ts，恰好以数字结尾），
// 相邻片段末尾数字毫无规律，序列比对会产生大量误删 —— 故先做命名一致性校验，
// 不满足则直接放弃该检测方式。
func matchSequence(segs []*M3U8Segment) map[int]bool {
	ad := map[int]bool{}
	if len(segs) < 4 {
		return ad
	}
	nums := make([]int, len(segs))
	ok := make([]bool, len(segs))
	valid, sequential := 0, 0
	for i, s := range segs {
		n, has := segNumber(s.AbsURL)
		nums[i], ok[i] = n, has
		if i > 0 && ok[i] && ok[i-1] && !s.Break {
			valid++
			if n == nums[i-1]+1 {
				sequential++
			}
		}
	}
	// 绝大多数相邻片段都应为 +1 递进，才认为命名方案是「连续数字」
	if valid < 3 || float64(sequential) < 0.6*float64(valid) {
		return ad
	}

	// 找出所有「断裂点」：nums[i] != nums[i-1]+1（块边界除外）
	var cuts []int
	for i := 1; i < len(segs); i++ {
		if !ok[i] || !ok[i-1] || segs[i].Break {
			continue
		}
		if nums[i] != nums[i-1]+1 {
			cuts = append(cuts, i)
		}
	}
	// 两两配对：正向大跳（插入广告块）→ 随后的「回落到插入前应有的值」，
	// 二者之间的片段即为插入广告；不成对的断裂点只标记其自身。
	for k := 0; k < len(cuts); k++ {
		i := cuts[k]
		if k+1 < len(cuts) {
			j := cuts[k+1]
			// i 为插入起点（明显前跳），j 为恢复点（回到 nums[i-1]+1）
			if nums[i] > nums[i-1]+1 && nums[j] == nums[i-1]+1 {
				for x := i; x < j; x++ {
					ad[x] = true
				}
				k++ // j 是恢复后的正片片段，保留
				continue
			}
		}
		ad[i] = true
	}
	return ad
}

// segNumber 取片段文件名末尾的数字（index123.ts → 123）
func segNumber(u string) (int, bool) {
	p := u
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	base := path.Base(p)
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	digits := ""
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] >= '0' && base[i] <= '9' {
			digits = string(base[i]) + digits
		} else {
			break
		}
	}
	if digits == "" {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return n, true
}

// matchRegex 片段级正则剔除
func matchRegex(segs []*M3U8Segment, list []string) map[int]bool {
	ad := map[int]bool{}
	for _, re := range list {
		re = strings.TrimSpace(re)
		if re == "" {
			continue
		}
		rgx, err := regexp.Compile(re)
		if err != nil {
			continue
		}
		for i, s := range segs {
			if rgx.MatchString(s.AbsURL) {
				ad[i] = true
			}
		}
	}
	return ad
}

// matchBlock 块分析：按 #EXT-X-DISCONTINUITY 切块，明显偏短的孤立块判为广告
// 覆盖 blockcut（广告块尾部截断）/ shortblock（孤立短块）/ slicebatch（批次特征退化版）
func matchBlock(segs []*M3U8Segment) map[int]bool {
	ad := map[int]bool{}
	if len(segs) < 3 {
		return ad
	}
	type block struct {
		start, end int
		dur        float64
	}
	var blocks []block
	cur := block{start: 0}
	for i, s := range segs {
		if s.Break && i > cur.start {
			cur.end = i
			blocks = append(blocks, cur)
			cur = block{start: i}
		}
		cur.dur += s.Duration
	}
	cur.end = len(segs)
	blocks = append(blocks, cur)

	if len(blocks) < 2 {
		return ad
	}
	durs := make([]float64, 0, len(blocks))
	for _, b := range blocks {
		durs = append(durs, b.dur)
	}
	sort.Float64s(durs)
	maxDur := durs[len(durs)-1]
	if maxDur <= 0 {
		return ad
	}
	// 明显短于最长正片块（<25%）的块判为广告插播
	for _, b := range blocks {
		if b.dur > 0 && b.dur < maxDur*0.25 {
			for i := b.start; i < b.end; i++ {
				ad[i] = true
			}
		}
	}
	return ad
}

// matchHash 内容哈希：逐段采样，反复出现的同一内容判为填充/广告
// 采样有带宽成本，故限制段数与单段字节上限
func matchHash(segs []*M3U8Segment, site *model.FilterSite, limit int, maxBytes int64) map[int]bool {
	ad := map[int]bool{}
	if len(segs) == 0 {
		return ad
	}
	if limit <= 0 || limit > len(segs) {
		limit = len(segs)
	}
	hashes := map[string]int{}
	segHash := make([]string, limit)
	for i := 0; i < limit; i++ {
		h, err := sampleHash(segs[i].AbsURL, site, maxBytes)
		if err != nil || h == "" {
			continue
		}
		segHash[i] = h
		hashes[h]++
	}
	// 众数哈希：出现 3 次以上且明显重复的内容，判为广告/填充
	for i := 0; i < limit; i++ {
		if segHash[i] != "" && hashes[segHash[i]] >= 3 {
			ad[i] = true
		}
	}
	return ad
}

// sampleHash 采样单个片段的内容哈希（取前 maxBytes 字节）
func sampleHash(segURL string, site *model.FilterSite, maxBytes int64) (string, error) {
	req, err := http.NewRequest("GET", segURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MXGJGT/1.0)")
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxBytes-1))
	if pu, err := url.Parse(segURL); err == nil {
		req.Header.Set("Referer", pu.Scheme+"://"+pu.Host+"/")
	}
	resp, err := httpClientFor(site).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "", fmt.Errorf("空片段")
	}
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:]), nil
}

// ======== 主流程 ========

// FilterM3U8 拉取并过滤 m3u8（核心入口）
// site 为 nil 时使用自动模式（无站点规则：仅做块分析 + 序列连贯性）
func FilterM3U8(rawURL string, site *model.FilterSite) (*FilterResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	base, err := url.Parse(rawURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("m3u8 地址无效")
	}

	siteID := ""
	if site != nil {
		siteID = site.ID
	}
	key := filterCacheKey(siteID, rawURL)
	if _, res, ok := filterCacheGet(key); ok {
		cp := *res
		cp.Cached = true
		return &cp, nil
	}

	res := &FilterResult{IndexURL: rawURL}

	// 1. 拉索引（主索引则按码率选一条子列表）
	text, err := FetchText(base.String(), site)
	if err != nil {
		return nil, err
	}
	headTags, segs, isMaster, variants := parseM3U8(text)
	res.IsMaster = isMaster
	res.Variants = variants

	if isMaster {
		if len(variants) == 0 {
			return nil, fmt.Errorf("主索引中未找到子列表")
		}
		pick := "first"
		if site != nil && site.StreamPick != "" {
			pick = site.StreamPick
		}
		sub := pickVariant(text, variants, pick)
		subAbs := absolutize(base, sub)
		if site != nil && site.IndexMode == "fixed" && strings.TrimSpace(site.FixedSub) != "" {
			if fix := strings.TrimSpace(site.FixedSub); strings.Contains(fix, "://") {
				subAbs = fix
			} else if b2, err := url.Parse(subAbs); err == nil {
				subAbs = b2.Scheme + "://" + b2.Host + "/" + strings.TrimLeft(fix, "/")
			}
		}
		res.IndexURL = subAbs
		nBase, _ := url.Parse(subAbs)
		nText, err := FetchText(subAbs, site)
		if err != nil {
			return nil, err
		}
		headTags, segs, _, _ = parseM3U8(nText)
		base = nBase
		if base == nil {
			base, _ = url.Parse(rawURL)
		}
	}

	// 2. 补全绝对地址 + 重写标签里的 URI 属性
	for _, s := range segs {
		s.AbsURL = absolutize(base, s.URI)
		for i, t := range s.Tags {
			s.Tags[i] = uriAttrRe.ReplaceAllString(t, `URI="`+"$1"+`"`)
			s.Tags[i] = absolutizeURIAttr(base, s.Tags[i])
		}
	}

	res.Total = len(segs)
	res.Mode = detectModeOf(site)

	// 3. 判定广告
	if res.Mode != "direct" {
		ad := map[int]bool{}
		mark := func(m map[int]bool, reason string) {
			for i := range m {
				if !ad[i] {
					ad[i] = true
					if segs[i].Reason == "" {
						segs[i].Reason = reason
					}
				}
			}
		}
		switch res.Mode {
		case "fingerprint":
			mark(matchFingerprint(segs, fpOf(site)), "fingerprint")
		case "hash":
			mark(matchHash(segs, site, 60, 64<<10), "hash")
		case "sequence":
			mark(matchSequence(segs), "sequence")
		case "regex":
			mark(matchRegex(segs, regexListOf(site)), "regex")
		case "blockcut", "slicebatch", "shortblock":
			mark(matchBlock(segs), "block")
		case "both":
			mark(matchFingerprint(segs, fpOf(site)), "fingerprint")
			mark(matchHash(segs, site, 60, 64<<10), "hash")
			mark(matchRegex(segs, regexListOf(site)), "regex")
		default: // auto：指纹（已配置时）→ 块分析 → 序列 → 正则
			if fpOf(site) != "" {
				mark(matchFingerprint(segs, fpOf(site)), "fingerprint")
			}
			mark(matchBlock(segs), "block")
			if len(ad) == 0 {
				mark(matchSequence(segs), "sequence")
			}
			mark(matchRegex(segs, regexListOf(site)), "regex")
		}
		for i := range segs {
			segs[i].Ad = ad[i]
		}
	}

	// 4. 重写：剔除广告片段，补 DISCONTINUITY 避免画面跳变
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	// 头部保留必要的版本/类型标签，剔除与片段数量相关的旧值
	for _, t := range headTags {
		if strings.HasPrefix(t, "#EXTM3U") ||
			strings.HasPrefix(t, "#EXT-X-MEDIA-SEQUENCE") ||
			strings.HasPrefix(t, "#EXT-X-DISCONTINUITY-SEQUENCE") ||
			strings.HasPrefix(t, "#EXT-X-STREAM-INF") {
			continue
		}
		b.WriteString(t)
		b.WriteString("\n")
	}
	kept := 0
	lastKept := -1
	for i, s := range segs {
		if s.Ad {
			continue
		}
		// 与上一保留片段不相邻 → 插入不连续标记
		if lastKept >= 0 && i != lastKept+1 {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		for _, t := range s.Tags {
			switch {
			case strings.HasPrefix(t, "#EXT-X-DISCONTINUITY"):
				// 重新按需生成，避免重复
				continue
			case strings.HasPrefix(t, "#EXTINF:"), strings.HasPrefix(t, "#EXT-X-BYTERANGE"):
				b.WriteString(t)
				b.WriteString("\n")
			default:
				b.WriteString(t)
				b.WriteString("\n")
			}
		}
		b.WriteString(s.AbsURL)
		b.WriteString("\n")
		lastKept = i
		kept++
	}
	if !strings.Contains(strings.ToUpper(text), "#EXT-X-ENDLIST") {
		b.WriteString("#EXT-X-ENDLIST\n")
	}

	res.Kept = kept
	res.Removed = res.Total - kept
	res.Segments = segs
	res.AdRanges = adRanges(segs)
	res.Playlist = b.String()

	// 仅对 VOD（含 ENDLIST）做缓存，直播索引不缓存
	if strings.Contains(strings.ToUpper(text), "#EXT-X-ENDLIST") {
		filterCachePut(key, res.Playlist, res)
	}
	return res, nil
}

// absolutizeURIAttr 把标签里 URI="..." 的相对地址补全为绝对地址
func absolutizeURIAttr(base *url.URL, tag string) string {
	if !strings.Contains(tag, `URI="`) {
		return tag
	}
	return uriAttrRe.ReplaceAllStringFunc(tag, func(m string) string {
		sub := uriAttrRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		return `URI="` + absolutize(base, sub[1]) + `"`
	})
}

// pickVariant 按码率策略选子列表
func pickVariant(masterText string, variants []string, pick string) string {
	if pick != "highest" && pick != "lowest" {
		return variants[0]
	}
	type cand struct {
		uri string
		bw  int
	}
	var cands []cand
	lines := strings.Split(strings.ReplaceAll(masterText, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !streamInfRe.MatchString(line) {
			continue
		}
		bw := 0
		if m := bandwidthRe.FindStringSubmatch(line); len(m) > 1 {
			bw, _ = strconv.Atoi(m[1])
		}
		for j := i + 1; j < len(lines); j++ {
			nxt := strings.TrimSpace(lines[j])
			if nxt == "" || strings.HasPrefix(nxt, "#") {
				continue
			}
			cands = append(cands, cand{uri: nxt, bw: bw})
			break
		}
	}
	if len(cands) == 0 {
		return variants[0]
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].bw < cands[b].bw })
	if pick == "highest" {
		return cands[len(cands)-1].uri
	}
	return cands[0].uri
}

// detectModeOf 取站点配置的检测方式（无站点规则时用 auto）
func detectModeOf(site *model.FilterSite) string {
	if site == nil {
		return "auto"
	}
	m := strings.ToLower(strings.TrimSpace(site.DetectMode))
	if m == "" {
		return "auto"
	}
	// parse_type=direct → 直链播放，不过滤
	if strings.EqualFold(strings.TrimSpace(site.ParseType), "direct") {
		return "direct"
	}
	return m
}

func fpOf(site *model.FilterSite) string {
	if site == nil {
		return ""
	}
	return site.Fingerprint
}

func regexListOf(site *model.FilterSite) []string {
	if site == nil {
		return nil
	}
	return site.RegexList
}

// adRanges 汇总广告区间（连续片段合并为 a-b）
func adRanges(segs []*M3U8Segment) []string {
	var out []string
	start := -1
	for i := 0; i <= len(segs); i++ {
		isAd := i < len(segs) && segs[i].Ad
		if isAd && start < 0 {
			start = i
		}
		if !isAd && start >= 0 {
			if i-1 == start {
				out = append(out, strconv.Itoa(start))
			} else {
				out = append(out, fmt.Sprintf("%d-%d", start, i-1))
			}
			start = -1
		}
	}
	return out
}

// ProbeM3U8 探测：解析索引结构并给出广告区间与时长指纹建议
func ProbeM3U8(rawURL string, site *model.FilterSite, deepHash bool) (*model.ProbeResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	base, err := url.Parse(rawURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("m3u8 地址无效")
	}
	text, err := FetchText(base.String(), site)
	if err != nil {
		return nil, err
	}
	_, segs, isMaster, variants := parseM3U8(text)
	out := &model.ProbeResult{OK: true, IsMaster: isMaster, Variants: variants, IndexURL: base.String()}

	if isMaster {
		if len(variants) == 0 {
			return nil, fmt.Errorf("主索引中未找到子列表")
		}
		sub := pickVariant(text, variants, "first")
		subAbs := absolutize(base, sub)
		out.IndexURL = subAbs
		if b2, err := url.Parse(subAbs); err == nil {
			base = b2
		}
		text, err = FetchText(subAbs, site)
		if err != nil {
			return nil, err
		}
		_, segs, _, _ = parseM3U8(text)
	}
	for _, s := range segs {
		s.AbsURL = absolutize(base, s.URI)
	}

	ad := matchBlock(segs)
	for i := range matchSequence(segs) {
		ad[i] = true
	}
	if deepHash {
		for i := range matchHash(segs, site, 40, 32<<10) {
			ad[i] = true
		}
	}
	for i, s := range segs {
		s.Ad = ad[i]
	}
	out.Total = len(segs)
	out.AdRanges = adRanges(segs)
	out.Segments = probeSegments(segs, 120)
	out.Fingerprint = suggestFingerprint(segs, ad)
	out.SuggestMode = "auto"
	if len(out.AdRanges) == 0 {
		out.SuggestMode = "direct"
	} else if out.Fingerprint != "" {
		out.SuggestMode = "fingerprint"
	}
	return out, nil
}

// AnalyzeFilterSource 分析资源站播放链接，自动探测广告并给出可直接填入
// 「新增过滤规则」表单的建议字段，避免逐项手动填写。
//
// 输入既可以是 m3u8 直链，也可以是资源站的播放页（自动解析出直链）。
func AnalyzeFilterSource(rawURL string, deepHash bool) (*model.FilterAnalyzeResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("请填写资源站播放链接")
	}

	// 播放页 → 直链 m3u8（本身就是 m3u8 则直接用）
	resolved := rawURL
	if !strings.Contains(strings.ToLower(resolved), ".m3u8") {
		s, err := ResolvePlayStream(rawURL)
		if err != nil || s == "" {
			if err == nil {
				err = fmt.Errorf("未解析出 m3u8")
			}
			return nil, fmt.Errorf("未能从播放链接解析出 m3u8：%v", err)
		}
		resolved = s
	}

	pr, err := ProbeM3U8(resolved, nil, deepHash)
	if err != nil {
		return nil, err
	}

	out := &model.FilterAnalyzeResult{
		OK:          true,
		SourceURL:   rawURL,
		IndexURL:    pr.IndexURL,
		IsMaster:    pr.IsMaster,
		Total:       pr.Total,
		AdRanges:    pr.AdRanges,
		HasAd:       len(pr.AdRanges) > 0,
		ParseType:   "both",
		IndexMode:   "auto",
		StreamPick:  "first",
		URLPrefixes: []string{},
		Segments:    pr.Segments,
		Remark:      "由播放链接自动分析生成",
	}

	// URL 匹配前缀 / 建议规则名：取实际 m3u8 的主机名
	host := ""
	if pu, err := url.Parse(resolved); err == nil {
		host = pu.Hostname()
	}
	if host != "" {
		out.URLPrefixes = append(out.URLPrefixes, host)
		out.Name = strings.TrimPrefix(host, "www.")
		out.Remark = "由播放链接自动分析生成（" + host + "）"
	}

	// 检测方式：探测到指纹优先「时长指纹」，否则自动
	if out.Fingerprint = pr.Fingerprint; out.Fingerprint != "" {
		out.DetectMode = "fingerprint"
	} else {
		out.DetectMode = "auto"
	}
	return out, nil
}

// probeSegments 转为探测输出结构（最多 n 条）
func probeSegments(segs []*M3U8Segment, n int) []model.ProbeSegment {
	out := make([]model.ProbeSegment, 0, n)
	for i, s := range segs {
		if i >= n {
			break
		}
		out = append(out, model.ProbeSegment{
			Index:    s.Index,
			Duration: s.Duration,
			URI:      s.AbsURL,
			Ad:       s.Ad,
		})
	}
	return out
}

// suggestFingerprint 从判定为广告的连续块里提取时长指纹建议
func suggestFingerprint(segs []*M3U8Segment, ad map[int]bool) string {
	var groups []string
	i := 0
	for i < len(segs) {
		if !ad[i] {
			i++
			continue
		}
		j := i
		var parts []string
		for j < len(segs) && ad[j] {
			if segs[j].Duration > 0 {
				parts = append(parts, trimZero(segs[j].Duration))
			}
			j++
		}
		if len(parts) > 0 && len(parts) <= 12 {
			groups = append(groups, strings.Join(parts, ","))
		}
		i = j
	}
	// 最多给 3 组建议，避免规则过长
	if len(groups) > 3 {
		groups = groups[:3]
	}
	return strings.Join(groups, "\n")
}

// trimZero 去掉浮点尾随 0（5.500 → 5.5）
func trimZero(f float64) string {
	s := strconv.FormatFloat(f, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

func absF(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// AnalyzeAdM3U8 统一广告分析：解析播放链接 → 探测广告 → 生成去广告播放列表 → 建议规则
// 供「M3U8 广告分析」页使用，返回分析明细 + 去广告 m3u8 + 建议规则字段。
// deepHash=true 时对前若干片段采样内容哈希，辅助识别重复填充广告（有带宽开销）。
func AnalyzeAdM3U8(rawURL string, deepHash bool) (*model.AdAnalyzeResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("请填写播放链接或 m3u8 地址")
	}

	// 播放页 → 直链 m3u8（本身就是 m3u8 则直接用）
	resolved := rawURL
	if !strings.Contains(strings.ToLower(resolved), ".m3u8") {
		s, err := ResolvePlayStream(rawURL)
		if err != nil || s == "" {
			if err == nil {
				err = fmt.Errorf("未解析出 m3u8")
			}
			return nil, fmt.Errorf("未能从播放链接解析出 m3u8：%v", err)
		}
		resolved = s
	}

	// 探测（解析 + 广告识别）
	pr, err := ProbeM3U8(resolved, nil, deepHash)
	if err != nil {
		return nil, err
	}

	// 过滤（生成去广告播放列表）
	fr, err := FilterM3U8(resolved, nil)
	if err != nil {
		return nil, err
	}

	out := &model.AdAnalyzeResult{
		OK:               true,
		SourceURL:        rawURL,
		IndexURL:         fr.IndexURL,
		IsMaster:         pr.IsMaster,
		Variants:         pr.Variants,
		Total:            fr.Total,
		Kept:             fr.Kept,
		Removed:          fr.Removed,
		AdRanges:         fr.AdRanges,
		HasAd:            len(fr.AdRanges) > 0,
		Mode:             fr.Mode,
		Fingerprint:      pr.Fingerprint,
		SuggestMode:      pr.SuggestMode,
		Segments:         pr.Segments,
		FilteredPlaylist: fr.Playlist,
		PlayURLDirect:    fr.IndexURL,
	}

	// 时长统计
	var totalDur, adDur float64
	for _, s := range fr.Segments {
		if s.Duration > 0 {
			totalDur += s.Duration
			if s.Ad {
				adDur += s.Duration
			}
		}
	}
	out.Duration = totalDur
	out.AdDuration = adDur

	// 原始 m3u8（前 30 行，供对比）
	if orig, err := FetchText(fr.IndexURL, nil); err == nil {
		out.OriginalPlaylist = headLinesStr(orig, 30)
	}

	// 建议规则字段：取主机名
	if pu, err := url.Parse(fr.IndexURL); err == nil {
		host := pu.Hostname()
		if host != "" {
			out.URLPrefixes = []string{host}
			out.Name = strings.TrimPrefix(host, "www.")
			out.Remark = "由 M3U8 广告分析自动生成（" + host + "）"
		}
	}
	if out.SuggestMode == "" {
		if out.HasAd {
			out.SuggestMode = "auto"
		} else {
			out.SuggestMode = "direct"
		}
	}
	return out, nil
}

// headLinesStr 取文本前 n 行
func headLinesStr(s string, n int) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
