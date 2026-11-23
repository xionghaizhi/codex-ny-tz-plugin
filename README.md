# codex-ny-timezone

CLIProxyAPI（CPA）原生动态库插件，ABI v1。把 Codex 请求体 `environment_context` 里的
`<timezone>` 与 `<current_date>` 改写成配置的 IANA 时区（默认 `Asia/Singapore`），并把已有的
`Accept-Language` 请求头统一改为 `en-US,en;q=0.9`。

## 使用

插件 ID 取自动态库文件名去掉扩展名，因此文件名必须是 `codex-ny-timezone.so`（Windows `.dll`，macOS `.dylib`）。

```
plugins/codex-ny-timezone.so        # 或 plugins/linux/amd64/codex-ny-timezone.so
```

```yaml
plugins:
  enabled: true          # 全局开关，默认关闭
  dir: "plugins"
  configs:
    codex-ny-timezone:
      enabled: true
      timezone: "Asia/Singapore"
```

重启后用 `GET /v0/management/plugins` 确认 `registered: true`。

请求头改写只在上游请求已经包含 `Accept-Language` 时执行；没有该请求头时保持原请求不变。

## 查看 CPA 并发

在 CPA 管理中心打开「并发查看」，或访问同一实例的 `/v0/resource/plugins/codex-ny-timezone/index.html`。
输入 CPA 管理密钥，点击「开始监控」。每 2 秒读取本插件的管理接口；密钥不写浏览器存储。
不依赖其他插件。宿主需要支持请求生命周期 schema v2；旧宿主仍保留原有时区与请求头改写。

- **当前在途**：已进入 CPA 前置请求拦截、尚未收到完成回调的模型请求数。同一请求重试及其间等待仍计一次。
  前置拦截后被认证、路由或其他步骤拒绝的请求也会计入，直到收到完成回调；完成回调是异步的，监控值可能短时高于实际并发，历史峰值也会保留该观测值。
- **本次运行峰值**：从插件加载起的最高在途数及发生时间；无需打开页面。重启 CPA 或重新加载插件动态库后重新统计。
- **并发趋势**：近 10 分钟内，每 5 秒的最高在途数。统计仅覆盖当前实例，不是集群汇总，不限流，不显示单独的排队数。

## 构建

```bash
go build -buildmode=c-shared -trimpath -o codex-ny-timezone.so main.go rewrite.go concurrency.go
# plugin.c is a separate C reference implementation, so list the Go files for tests too.
go test main.go rewrite.go concurrency.go rewrite_test.go main_test.go concurrency_test.go
```

必须显式列出 Go 文件。同目录 `plugin.c` 是纯 C 参考实现，用 `go build .` 会被 cgo 一起编译，
链接时 `cliproxy_plugin_init` 重复定义；当前带请求头改写能力的构建以 Go 实现为准。

## 发布

推送 `v*` tag 触发 `.github/workflows/release.yml`，产出
`codex-ny-timezone_<version>_<goos>_<goarch>.zip`（动态库位于 zip 根目录）与 `checksums.txt`，
符合 CPA 插件商店的资产约定。

`request.normalize` 和请求拦截都返回 CPA 当前请求链所需的原始 `Body`/`Headers` 字段，
因此动态库需要由支持 `request_interceptor` 的 CPA 版本加载。
