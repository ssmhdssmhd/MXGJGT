// Package main 入口
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

// dataDir 存放配置文件目录
var dataDir = "data"

func main() {
	// 1. 定位 data 目录
	if v := os.Getenv("MXGJGT_DATA_DIR"); v != "" {
		dataDir = v
	} else if wd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(wd, "data")); err == nil {
			dataDir = filepath.Join(wd, "data")
		}
	}

	// 2. 加载配置
	if err := config.LoadConfig(dataDir); err != nil {
		log.Printf("[WARN] 配置加载失败: %v，使用默认配置", err)
	}
	cfg := config.GetConfig()

	// 3. 启动自动更新（后台）
	cronObj, err := service.StartAutoUpdater()
	if err != nil {
		log.Printf("[WARN] 自动更新启动失败: %v", err)
	}
	if cronObj != nil {
		defer cronObj.Stop()
	}

	// 4. 启动 HTTP 服务
	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	r.Use(gin.Recovery())

	handler.RegisterRoutes(r)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("[MXGJGT] v%s 启动成功，监听 %s", handler.Version, addr)
	log.Printf("[MXGJGT] 伪装模式：/api/clean/?url= 为真实 API，其他路径返回 404")

	if err := r.Run(addr); err != nil {
		log.Fatalf("[MXGJGT] 启动失败: %v", err)
	}
}
