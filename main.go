// Package main 入口 — 自动定位可执行文件目录，首次启动自动创建 data/cache/logs
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"mxgjgt/config"
	"mxgjgt/handler"
	"mxgjgt/service"

	"github.com/gin-gonic/gin"
)

func main() {
	// ===== 1. 定位可执行文件所在目录 =====
	exeDir := resolveExeDir()
	log.Printf("[MXGJGT] 可执行文件目录: %s", exeDir)

	// ===== 2. 首次启动自动创建目录 + 写入默认模板 =====
	dataDir := filepath.Join(exeDir, "data")
	cacheDir := filepath.Join(exeDir, "cache")
	logsDir := filepath.Join(exeDir, "logs")

	for _, d := range []string{dataDir, cacheDir, logsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			log.Fatalf("[MXGJGT] 创建目录失败 %s: %v", d, err)
		}
	}

	config.InitPaths(dataDir, cacheDir, logsDir)
	config.EnsureDefaults()

	// ===== 3. 加载配置 =====
	if err := config.LoadConfig(dataDir); err != nil {
		log.Printf("[WARN] 配置加载失败: %v，使用默认配置", err)
	}
	cfg := config.GetConfig()

	// ===== 4. 启动自动更新（后台 cron） =====
	if cronObj, err := service.StartAutoUpdater(); err != nil {
		log.Printf("[WARN] 自动更新启动失败: %v", err)
	} else {
		defer cronObj.Stop()
	}

	// ===== 5. 启动 HTTP 服务 =====
	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	r.Use(gin.Recovery())

	handler.RegisterRoutes(r)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("==============================================")
	log.Printf(" MXGJGT v%s  启动成功", handler.Version)
	log.Printf("----------------------------------------------")
	log.Printf(" 监听地址      :  http://127.0.0.1%s", addr)
	log.Printf(" 数据目录      :  %s", dataDir)
	log.Printf(" 缓存目录      :  %s", cacheDir)
	log.Printf(" 日志目录      :  %s", logsDir)
	log.Printf(" 伪装模式      :  仅 clean 调用路径 / /api/search/ 响应，其余 404 伪装")
	log.Printf("==============================================")

	if err := r.Run(addr); err != nil {
		log.Fatalf("[MXGJGT] 启动失败: %v", err)
	}
}

// resolveExeDir 优先使用可执行文件所在目录，可被环境变量覆盖
func resolveExeDir() string {
	// 环境变量覆盖（容器化 / systemd 用）
	if v := os.Getenv("MXGJGT_HOME"); v != "" {
		return v
	}

	// 可执行文件目录
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "." {
			return dir
		}
	}

	// 兜底：当前工作目录
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}
