# 01 `PN09-W01` 前半：部署开关与依赖方向门禁

Category: enhancement
Status: resolved（2026-11-18 进 main，见「进 main 记录」）
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

## 评审记录

作者是 Sonnet 5.5 子代理，评审是主会话，钉 `9eef5267e`。一轮，没有阻断。

- 对照票面逐条读了全部改动。生产代码只动 `cmd/parcel-api` 与 `cmd/parcel-dispatch` 的装配和新包 `cmd/internal/warehouseswitch`。`internal/` 下只加了一份门禁测试。
- 评审自己重跑：`gofmt -l .` 为空，`go vet ./...`、`go build ./...`、分片核对退 0，`cmd/internal` 与 `internal/architecture` 的测试通过。
- 作者报告做过变异检查：把门反着写、把值接成常量、放宽读法、删掉启动日志，对应用例都变红。评审没有重做这一步。

不挡的三处，照收：

- `cmd/parcel-api/warehouse_fulfillment_switch_test.go` 断言的是源文件结构，不是行为。它对 `run`、`openDatabase`、`startup` 这几个名字敏感，改名要同改。第一张落仓配端点的票有了开与关两种装配的用例之后，可以删掉它。
- 目录门禁比票面严：任何层级的 `adapters/warehousefulfillment` 不在点名位置即红，含仓配自己下面的同名目录。
- `dispatchSettings` 头注「全部必填」对这一格不成立，字段注释里写了例外。头注是原有的，没有改。

## 进 main 记录

- main 上的内容是 `9eef5267e`，与分支 `cursor/wf-w01-switch` 的 tip 同一个对象，快进，没有重放。收口是随后这一笔。
- 全量真库测试：`scripts/test-full.sh --detach` 对 `9eef5267e` 跑，`FULL-EXIT=0`。
- 演示种子没有灌：改动不碰迁移、登记 CLI 与种子，开关默认关。
- 生成物不重算。
