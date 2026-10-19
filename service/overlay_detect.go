package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"sort"
	"time"

	"mxgjgt/config"
	"mxgjgt/model"

	"github.com/chromedp/chromedp"
)

// 画面遮罩「自动识别」—— 链接抓帧 + 启发式文本/图标区域检测
//
// 说明：内嵌广告（文字 / 台标 / 二维码）被「烧」进画面像素，无法靠剔片段解决。
// 在无解码器（ffmpeg）的环境下，这里用无头浏览器对链接页面截图，
// 再用纯 Go 图像启发式（边缘密度聚类）找出「像文字 / 图标」的候选区域。
// 识别结果为**候选**，供后台一键预填，仍需人工确认。

// CapturedShot 抓帧结果：图像字节 + 实际尺寸
type CapturedShot struct {
	PNG  []byte
	W, H int
}

// CapturePageShot 用无头浏览器打开链接并截图（优先截 <video> 元素，退化为整页）
func CapturePageShot(rawURL string) (*CapturedShot, error) {
	path := findChromePath()
	if path == "" {
		return nil, fmt.Errorf("未找到可用的 Chrome/Chromium")
	}
	timeout := 20 * time.Second
	if cfg := config.GetConfig(); cfg != nil && cfg.Browser.TimeoutSec > 0 {
		timeout = time.Duration(cfg.Browser.TimeoutSec) * time.Second
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(path),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("mute-audio", true),
		chromedp.WindowSize(1280, 720),
		chromedp.UserAgent(BrowserUA),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	if _, err := chromedp.Run(ctx, chromedp.Steps(
		chromedp.Navigate(rawURL),
		chromedp.Sleep(4*time.Second),
	)); err != nil {
		return nil, err
	}

	var buf []byte
	// 优先截取播放器画面（<video>）；拿不到再整页截图
	if b, err := chromedp.Run(ctx, chromedp.Screenshot("video")); err == nil && len(b) > 0 {
		buf = b
	} else {
		b2, err2 := chromedp.Run(ctx, chromedp.CaptureScreenshot())
		if err2 != nil {
			return nil, err2
		}
		buf = b2
	}
	if len(buf) == 0 {
		return nil, fmt.Errorf("截图失败")
	}
	w, h := imageSize(buf)
	return &CapturedShot{PNG: buf, W: w, H: h}, nil
}

// DetectCandidateRegions 启发式检测图中的「文字 / 图标」候选区域（相对坐标 0~1）
// 思路：灰度 → 边缘梯度 → 阈值二值化 → 粗网格边缘密度 → 连通域合并 → 输出候选框
func DetectCandidateRegions(img []byte) []model.AdOverlayRegion {
	src, _, err := image.Decode(bytes.NewReader(img))
	if err != nil {
		return nil
	}
	b := src.Bounds()
	W, H := b.Dx(), b.Dy()
	if W < 16 || H < 16 {
		return nil
	}

	// 1) 灰度化 + 下采样（最长边压到 ~480，降低噪声与计算量）
	maxSide := 480
	scale := 1
	if W > maxSide || H > maxSide {
		if W >= H {
			scale = int(math.Ceil(float64(W) / float64(maxSide)))
		} else {
			scale = int(math.Ceil(float64(H) / float64(maxSide)))
		}
	}
	if scale < 1 {
		scale = 1
	}
	gw, gh := W/scale, H/scale
	if gw < 8 || gh < 8 {
		return nil
	}
	gray := make([]float64, gw*gh)
	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			r, gg, bb, _ := src.At(b.Min.X+x*scale, b.Min.Y+y*scale).RGBA()
			gray[y*gw+x] = 0.299*float64(r>>8) + 0.587*float64(gg>>8) + 0.114*float64(bb>>8)
		}
	}

	// 2) 梯度幅值（与右、下邻居的差分）
	grad := make([]float64, gw*gh)
	var sum, sum2, n float64
	for y := 0; y < gh-1; y++ {
		for x := 0; x < gw-1; x++ {
			c := gray[y*gw+x]
			gx := gray[y*gw+x+1] - c
			gy := gray[(y+1)*gw+x] - c
			g := math.Abs(gx) + math.Abs(gy)
			grad[y*gw+x] = g
			sum += g
			sum2 += g * g
			n++
		}
	}
	if n == 0 {
		return nil
	}
	mean := sum / n
	std := math.Sqrt(math.Max(0, sum2/n-mean*mean))
	thr := mean + 1.4*std
	if thr < 26 {
		thr = 26
	}

	// 3) 粗网格边缘密度（16 x 9）
	const cols, rows = 16, 9
	cellPix := make([]int, cols*rows)
	cellEdge := make([]int, cols*rows)
	for y := 0; y < gh-1; y++ {
		cy := y * rows / gh
		for x := 0; x < gw-1; x++ {
			cx := x * cols / gw
			idx := cy*cols + cx
			cellPix[idx]++
			if grad[y*gw+x] >= thr {
				cellEdge[idx]++
			}
		}
	}

	// 4) 标记「像文字」的格子：边缘像素占比超过阈值
	const dense = 0.07
	marked := make([]bool, cols*rows)
	for i := range cellPix {
		if cellPix[i] == 0 {
			continue
		}
		if float64(cellEdge[i])/float64(cellPix[i]) >= dense {
			marked[i] = true
		}
	}

	// 5) 连通域合并（4 邻域）
	seen := make([]bool, cols*rows)
	var boxes []model.AdOverlayRegion
	for i := range marked {
		if !marked[i] || seen[i] {
			continue
		}
		queue := []int{i}
		seen[i] = true
		minC, maxC, minR, maxR := i%cols, i%cols, i/cols, i/cols
		edgeTotal, cellTotal := 0, 0
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			c, rr := cur%cols, cur/cols
			if c < minC {
				minC = c
			}
			if c > maxC {
				maxC = c
			}
			if rr < minR {
				minR = rr
			}
			if rr > maxR {
				maxR = rr
			}
			edgeTotal += cellEdge[cur]
			cellTotal++
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nc, nr := c+d[0], rr+d[1]
				if nc < 0 || nc >= cols || nr < 0 || nr >= rows {
					continue
				}
				ni := nr*cols + nc
				if marked[ni] && !seen[ni] {
					seen[ni] = true
					queue = append(queue, ni)
				}
			}
		}
		x := float64(minC) / cols
		y := float64(minR) / rows
		w := float64(maxC-minC+1) / cols
		h := float64(maxR-minR+1) / rows
		// 过滤：太小（噪声）/ 太大（疑似整屏）
		if w < 0.03 || h < 0.03 || w*h > 0.86 {
			continue
		}
		density := 0
		if cellTotal > 0 {
			density = int(float64(edgeTotal) / float64(cellTotal))
		}
		boxes = append(boxes, model.AdOverlayRegion{
			X: x, Y: y, W: w, H: h,
			Mode: "cover", Color: "#000000", Opacity: 1,
			Label: fmt.Sprintf("候选%d（边缘密度%d）", len(boxes)+1, density),
		})
	}

	// 6) 取面积最小的 6 个候选（越小的框越可能是台标 / 二维码这类贴片）
	sort.SliceStable(boxes, func(a, b int) bool {
		return boxes[a].W*boxes[a].H < boxes[b].W*boxes[b].H
	})
	if len(boxes) > 6 {
		boxes = boxes[:6]
	}
	return boxes
}

// imageSize 解析图像尺寸（失败返回 0,0）
func imageSize(img []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// ImageConfig 导出图像尺寸解析（供 handler 判断图片比例）
func ImageConfig(img []byte) (w, h int, ok bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}
