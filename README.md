# Prometheus for DBX

Prometheus for DBX 是一个独立、只读的 Prometheus HTTP API 工作台。插件连接单个 Prometheus 服务，提供 PromQL 查询、采集目标、活动告警、规则和运行状态查看能力；不会连接 Alertmanager，也不会修改 Prometheus 配置。

## 版本说明

### v0.1.7（2026-09-21）

- 采集目标、活动告警和规则改为可搜索下拉选择，无需手动输入完整关键字。
- 支持使用鼠标或键盘选择具体项，并可随时切回“全部”。
- 修复选择具体采集目标或活动告警后无法切回全部的问题。
- 修复规则候选无法通过鼠标点击选中的问题。
- 单次最多展示 200 个候选项，超出时提示继续输入关键字缩小范围。

### v0.1.6

- 服务发现支持多选采集服务、关键字搜索和已选服务独立移除。
- 最多合并查询 20 个服务，并对活动目标或已丢弃目标进行 20、50、100 条分页展示。

## 连接配置

要求 DBX `0.6.14+`，Host API 版本为 `1`。

创建连接时填写主机、端口、HTTP/HTTPS 协议和可选的反向代理应用路径。Basic Auth 用户名和密码为可选项，但必须同时填写；密码由 DBX 密码字段管理，不会写入插件源码或日志。使用 Basic Auth 时建议启用 HTTPS。

如果 DBX 连接配置了 SSH、代理或 HTTP 隧道，插件会使用宿主提供的隧道地址。宿主未提供隧道地址时，插件会直接失败，不会绕过隧道连接原始地址。

连接测试通过 `/api/v1/status/buildinfo` 完成。

## 功能

- 多个相互独立的 PromQL 查询面板，支持表格/图表视图、单面板执行和删除。
- PromQL 编辑器支持语法高亮、指标/标签/标签值补全、Lint 诊断和 `Shift+Enter` 执行。
- 可搜索的指标浏览器，可将选中指标插入当前查询面板。
- 支持评估时间前后移动、图表时间范围与步长配置、本地时间/UTC 切换，以及查询历史持久化。
- 支持浅色、跟随系统和深色主题。
- 支持瞬时查询和范围查询；范围查询最多 11,000 个点，响应最大 4 MiB，最多展示 100 条序列。
- 查看活动采集目标、健康状态、最近采集时间和错误信息（`/api/v1/targets`）。
- 查看活动告警（`/api/v1/alerts`）以及记录规则、告警规则组（`/api/v1/rules`）。
- 查看运行时/构建信息、TSDB 基数、命令行参数和已加载配置（`/api/v1/status/*`）。
- 采集目标、活动告警和规则支持可搜索下拉选择、鼠标/键盘操作及“全部”切换。
- 服务发现支持多选：从 `/api/v1/scrape_pools` 加载服务，仅按当前输入关键字搜索，最多选择 20 个服务，并将活动目标或已丢弃目标合并为分页结果。后端以流式方式读取每个服务响应，统计合并后的目标总数，仅返回当前请求的 20、50 或 100 条数据。

所有请求均使用 GET 和固定 API 路径，并拒绝重定向。查询会发送到已配置的 Prometheus 服务，请根据实际环境限制连接权限和查询规模。插件不启动本地监听端口、不创建子进程，也不写入持久化插件文件。

## 本地预览

运行以下命令后，打开终端中显示地址的 `/preview.html`：

```sh
npm run dev
```

预览页面使用模拟数据并明确标记为预览环境，不会访问真实 Prometheus。模拟桥接代码不会进入最终打包的 `ui/index.html`，正式入口仍必须在 DBX 中运行。

## 构建与测试

需要 Node.js 22+、Go 1.22+ 和 `@dbx-app/plugin-cli@0.1.9`。

```sh
npm ci --ignore-scripts
npm test
npm run build
go -C backend test -race ./...
go -C backend vet ./...
npm run package:all
go run scripts/verify-package.go dist/io.github.caichangqing1120.prometheus-0.1.7-darwin-arm64.dbxp --handshake
```

打包脚本会生成 macOS、Windows、Linux 的 ARM64/x64 共六个平台候选包，同时生成 SHA256 校验文件、每个平台的元数据文件和 `dist/release-candidates.json`。

跨平台包会校验目标架构、入口文件和 SHA256，但未在每个操作系统的 DBX 中逐一运行。macOS ARM64 `0.1.7` 候选包会在本机 DBX 中验证真实采集目标读取和筛选交互；后端测试使用 `httptest` 固定数据，前端自动化测试使用模拟数据。源码/预览测试与安装包在 DBX 中的验证证据分别记录，不能相互替代。

## 插件市场发布

源码和未签名候选包通过候选 PR 提交到 `t8y2/dbx-store`。DBX 维护者会审核、签名并合并完全一致的包，之后插件才会出现在官方市场。公开仓库、Git Tag 或候选 PR 本身都不代表已正式上架。

签名密钥不得放入本仓库。本地构建包为未签名开发候选包，仅用于本地验证。

## 许可证与商标

插件源码使用 MIT License。第三方许可证和 SDK 来源见 `LICENSE`、`assets/THIRD_PARTY_NOTICES.txt` 与 `backend/sdk/PROVENANCE.md`。

Prometheus 是 Linux Foundation 的商标。本插件是独立集成项目，与 Prometheus 项目无隶属或官方合作关系。
