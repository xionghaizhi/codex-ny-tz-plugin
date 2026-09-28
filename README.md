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

## 构建

```bash
go build -buildmode=c-shared -trimpath -o codex-ny-timezone.so main.go rewrite.go
# plugin.c is a separate C reference implementation, so list the Go files for tests too.
go test main.go rewrite.go rewrite_test.go main_test.go
```

必须显式列出 Go 文件。同目录 `plugin.c` 是纯 C 参考实现，用 `go build .` 会被 cgo 一起编译，
链接时 `cliproxy_plugin_init` 重复定义；当前带请求头改写能力的构建以 Go 实现为准。

## 发布

推送 `v*` tag 触发 `.github/workflows/release.yml`，产出
`codex-ny-timezone_<version>_<goos>_<goarch>.zip`（动态库位于 zip 根目录）与 `checksums.txt`，
符合 CPA 插件商店的资产约定。

`request.normalize` 和请求拦截都返回 CPA 当前请求链所需的原始 `Body`/`Headers` 字段，
因此动态库需要由支持 `request_interceptor` 的 CPA 版本加载。
