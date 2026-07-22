//go:build !ios

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// modifyOptionsForIOS is a no-op on non-iOS platforms
func modifyOptionsForIOS(opts *application.Options) {
	// No modifications needed for non-iOS platforms
}

// main 在非 iOS 平台下作为占位入口存在。
// build/ios 是独立的 package main，仅在 iOS 构建时由 overlay 注入真正的 main()；
// 非 iOS 平台执行 `go build ./...` 时若缺少 main() 会报
// "function main is undeclared in the main package"，故在此提供空实现。
func main() {}
