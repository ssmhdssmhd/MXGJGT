package model

// Version 应用版本 — 百进位: v0.0.99 -> v0.1.0 -> v0.99.99 -> v1.0.0
// 构建时通过 -ldflags "-X mxgjgt/model.Version=x.y.z" 注入
var (
	Version   = "0.0.4"
	BuildTime = "2026-10-18T00:00:00Z"
)
