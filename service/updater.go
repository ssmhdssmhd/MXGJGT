package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"mxgjgt/config"

	"github.com/robfig/cron/v3"
)

// StartAutoUpdater 启动自动更新 cron 任务
func StartAutoUpdater() (*cron.Cron, error) {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Updater.Enabled {
		return nil, nil
	}

	c := cron.New()
	_, err := c.AddFunc(cfg.Updater.Interval, func() {
		doUpdate()
	})
	if err != nil {
		return nil, err
	}
	c.Start()
	return c, nil
}

// doUpdate 执行一次更新
func doUpdate() {
	cfg := config.GetConfig()
	if cfg == nil {
		return
	}

	fmt.Println("[Updater] 开始自动更新检查...")

	// 1. 先测速挑选最优镜像
	bestMirror := pickBestMirror(cfg.Updater.RepoURL, cfg.Updater.Mirrors)
	repoURL := cfg.Updater.RepoURL
	if bestMirror != "" {
		repoURL = bestMirror
	}

	// 2. git pull 更新
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "pull", repoURL, cfg.Updater.Branch)
	cmd.Dir, _ = os.Getwd()
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[Updater] git pull 失败: %v, 输出: %s\n", err, string(out))
		return
	}
	fmt.Printf("[Updater] 更新成功: %s\n", strings.TrimSpace(string(out)))

	// 3. 重新编译（如果是二进制部署）
	if binaryPath := os.Args[0]; binaryPath != "" {
		goBuild(ctx)
		fmt.Println("[Updater] 编译完成，等待下次自动重启...")
	}
}

// pickBestMirror 测速挑选最快的镜像
func pickBestMirror(origin string, mirrors []string) string {
	if len(mirrors) == 0 {
		return ""
	}

	type result struct {
		url    string
		delay  time.Duration
		ok     bool
	}

	ch := make(chan result, len(mirrors))
	for _, m := range mirrors {
		mirrorURL := strings.ReplaceAll(m, "{origin}", origin)
		go func(u string) {
			start := time.Now()
			cmd := exec.Command("git", "ls-remote", "--exit-code", u)
			cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
			err := cmd.Run()
			ch <- result{url: u, delay: time.Since(start), ok: err == nil}
		}(mirrorURL)
	}

	var best result
	best.delay = time.Hour * 24
	timeout := time.After(10 * time.Second)

	for i := 0; i < len(mirrors); i++ {
		select {
		case r := <-ch:
			if r.ok && r.delay < best.delay {
				best = r
			}
		case <-timeout:
			if best.url != "" {
				return best.url
			}
			return ""
		}
	}

	if best.ok {
		return best.url
	}
	return ""
}

// goBuild 重新编译
func goBuild(ctx context.Context) {
	cmd := exec.CommandContext(ctx, "go", "build", "-o", "mxgjgt", ".")
	cmd.Dir, _ = os.Getwd()
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[Updater] go build 失败: %v, 输出: %s\n", err, string(out))
	}
}

// ManualUpdate 手动触发更新
func ManualUpdate() string {
	before := time.Now()
	doUpdate()
	return fmt.Sprintf("更新完成，耗时: %v", time.Since(before))
}
