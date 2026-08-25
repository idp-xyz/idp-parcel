# 06 cmd/parcel-api 装配:七端点登记

Category: feature
Status: resolved
Owner: MCP-4(2026-08-25 13:02 改派;原派 MCP-2 自 11:40 后无提交、截至 13:02 未响应,用户经通道 3 授权 MCP-3 调度本轮)
Blocked by: 02, 03, 04, 05(四票均已 resolved,不阻)

四张后端票落库后,把七个查询端点(计价二、网络一、关务一、商业二…以各票实际构造器
为准)登进 `cmd/parcel-api`:main 构造真库读适配器交入、endpoints.go 端点表登记、
endpoints_test.go 装配测试跟上、unwired_orchestration.go 若需桩位按 267cb44 先例处理。

共享文件纪律:改前在频道广播占号(migrations.go 同款三条:占号/同笔/`add` 前逐块核),
四件同一笔提交。

## 完成标准

全仓 build/vet 零告警,`go test -count=1 ./...` 全绿(写明含不含 PG),自己提交,
票面 resolved + SHA,回频道 2 并释号。

## Comments

- 2026-08-25 13:02 MCP-3(调度):本轮完工报告改回**频道 3**(原票面写频道 2,调度权
  已按用户授权变更);其余完成标准不变。开工后**尽早把七端点最终路径表广播全频道**
  ——票 07 的路径绑定等它。四票读面的 http 构造器见 f4dad52(计价二)、c1e10ce
  (网络二合一端点,以实际构造器为准)、2d5c8ff(关务)、ecf268e(商业二)。
- 2026-08-25 MCP-4:已认领并开工;先核对四票实际 HTTP 构造器与既有
  `cmd/parcel-api` 装配先例,路径表确认后立即广播。
- 2026-08-25 MCP-4:完成于 `820c5a4`。七个页面按实际构造器落为六个 GET 入口
  (网络目录与服务区域共用 `/network-catalog?family=`);`main` 构造四个上下文的
  真库读适配器,端点统一装配 `UnconfiguredIntake`。验证为**绿(含真库)**:
  `go build ./...`、`go vet ./...` 零告警,设置本机 DSN 后
  `go test -count=1 ./...` 全绿;`go test -count=1 -v ./cmd/parcel-api` 中真库
  装配用例与全部端点未配置用例均 `PASS`,无 `SKIP`。
- 2026-08-25 MCP-4:审查预告后补验于 `3b6c03d`:商业策略端点对封闭集内全部合法
  `kind` 均交回字节一致的 403,且未触发读口;网络七族与关务两册已有同款断言。
  补验后再次执行全仓 build/vet 与带 DSN 的 `go test -count=1 ./...`,结果全绿。
