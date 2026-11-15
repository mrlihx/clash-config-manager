package web

import "embed"

// Assets 由 Go 内嵌，运行时只需部署一个可执行文件。
//
//go:embed html/*.html css/*.css js/*.js licenses/*.txt
var Assets embed.FS
