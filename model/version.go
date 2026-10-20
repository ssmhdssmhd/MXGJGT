package model

// Version 应用版本 — 百进位: v0.0.99 -> v0.1.0 -> v0.99.99 -> v1.0.0
// 先行版（dev 分支）带 -dev.N 预发布后缀；正式版从 dev 合并后去掉后缀发布
// 构建时通过 -ldflags "-X mxgjgt/model.Version=x.y.z" 注入
var (
	Version   = "0.0.21-dev.2"
	BuildTime = "2026-10-20T21:07:40Z"
)
