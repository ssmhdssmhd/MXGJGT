package handler

import "mxgjgt/model"

// 兼容导出 —— 实际版本常量在 model.Version
var (
	Version   = model.Version
	BuildTime = model.BuildTime
)
