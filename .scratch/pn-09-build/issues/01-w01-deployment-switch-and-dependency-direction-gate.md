# 01 `PN09-W01` 前半：部署开关与依赖方向门禁

Category: enhancement
Status: in-progress（2026-11-18 认领，分支 `cursor/wf-w01-switch`）
Blocked by: 无

依据：[ADR-0375](../../../docs/adr/0375-warehouse-fulfillment-is-an-optional-capability-on-its-own-pn-09-slice-off-per-deployment-unless-switched-on-and-unconfigured-per-tenant-until-registered.md) 决定三、四；[PN-09 开发交接](../../../docs/design/pn-09-warehouse-fulfillment-development-handoff.md) 的 `PN09-W01`。

用户 2026-11-18 点名继续，原话「继续，以后你能够只是分析和计划和评估，实际的代码开发工作让sonnet 5.5来做吗」。分工：拆包、任务说明与验收由主会话做，代码与测试由 Sonnet 5.5 子代理写。

## 要做的

1. **开关的读法只有一处。** 新包放在 `cmd/internal/` 下，与 `cmd/internal/docnumbering` 同级。环境变量 `IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED`。去首尾空白后恰为 `true` 才开；缺省、空、`TRUE`、`True`、`1`、`yes`、`false` 都按关读。读法照 `cmd/parcel-api` 里 `pricingRegisterNewerCanonicalization` 的先例。同一个包给出启动日志：开与关都出声，带环境变量名。
2. **`parcel-dispatch`。** 启动时读一次，写启动日志，值放进 `dispatchSettings`。加一个仓配专用的登记函数：仓配的各路只经它登进 `routeRegistry`；开关关着时它交回零路，并且不调用构造函数。今天没有任何仓配的路，这个函数暂时没有生产调用方。
3. **`parcel-api`。** 启动时读一次，写启动日志，值放进 `endpointSources`。不加任何端点，不改任何端点的答复。
4. **依赖方向门禁**，放在 `internal/architecture`。其他上下文不得导入 `internal/warehousefulfillment` 及其子包。例外只有三个位置：`internal/settlementaccounting/adapters/warehousefulfillment`、`internal/parcelpricing/adapters/warehousefulfillment`、`internal/visibilityexception/adapters/warehousefulfillment`。这三个位置之外，任何上下文下出现 `adapters/warehousefulfillment` 目录即红。`internal/warehousefulfillment` 自己与 `cmd/` 不在此限。门禁带一条「确实抓得住违规」的自测，写法照同目录已有的门禁。

## 不在本票

- 「委托待受控补充」的收工路。小包托运今天不发这封信封（UC-PS-001「给开发的交接」写着未做）。先登一条没有发出方的路，是一条测不到的死路；先让小包托运发出而没有路接，每个部署都会显式失败。两样要同一笔落，而且改的是主链的接受判断。它单独成票，排在 `PN09-W15` 之前。这是对开发交接里 `PN09-W01` 一行的拆分，可以推翻。
- 开关关着时仓配端点怎么答。归第一张落仓配端点的票（ADR-0384 决定七）。
- 仓配的领域包、迁移、登记 CLI。

## 完成判据

- 开关读法的用例覆盖上面列的每一种取值，并钉住环境变量名。
- 派发：开关关着时仓配登记函数交回零路且不调用构造函数；开着时原样交回构造函数给的路与错误。既有路由表不变。
- 门禁在今天的树上是绿的，自测证明它对违规导入与违规目录各能变红。
- `gofmt -l .` 为空，`go vet ./...`、`go build ./...` 退 0，`bash scripts/ci/test-shards.sh check` 退 0。受影响包的测试通过。
- 其他上下文的代码一行不改。
