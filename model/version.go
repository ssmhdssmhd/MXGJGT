package model

// Version 应用版本 — 百进位: v0.0.99 -> v0.1.0 -> v0.99.99 -> v1.0.0
// 正式版（main 分支）不带后缀；先行版（dev 分支）带 -dev.N 预发布后缀
// 构建时通过 -ldflags "-X mxgjgt/model.Version=x.y.z" 注入
var (
	Version   = "0.0.17"
	BuildTime = "2026-10-19T00:00:00Z"
)
