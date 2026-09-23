# codex-ny-timezone

CLIProxyAPI（CPA）原生动态库插件，ABI v1。把 Codex 请求体 `environment_context` 里的
`<timezone>` 与 `<current_date>` 改写成配置的 IANA 时区（默认 `America/New_York`）。

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
      timezone: "America/New_York"
```

重启后用 `GET /v0/management/plugins` 确认 `registered: true`。

## 构建

```bash
go build -buildmode=c-shared -trimpath -o codex-ny-timezone.so main.go rewrite.go
```

必须显式列出 Go 文件。同目录 `plugin.c` 是纯 C 参考实现，用 `go build .` 会被 cgo 一起编译，
链接时 `cliproxy_plugin_init` 重复定义。

## 发布

推送 `v*` tag 触发 `.github/workflows/release.yml`，产出
`codex-ny-timezone_<version>_<goos>_<goarch>.zip`（动态库位于 zip 根目录）与 `checksums.txt`，
符合 CPA 插件商店的资产约定。

## 已知限制

`rewrite.go` 的正则把 `America/New_York` 与 `2026-09-23` 写成字面量，只匹配当前基线；
基线日期变化后 `current_date` 替换会失效。
