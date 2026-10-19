# 03 计价：接 SELL 评价请求，按包裹建快照形成 SELL·客户费用评价

Category: enhancement
Status: 完工，待评审与进 main——分支 `claude-sell-03`（基于 `claude-sell-01`；2026-10-19 Claude Code 会话立，同日作者会话落地，见文末「完成记录」）。
Blocked by: [01](./01-sell-request-identity-and-registers.md)、[02](./02-request-sell-evaluation-on-parcel-moment.md)（同改派发路由表，串行）
地盘：`internal/parcelpricing` 的 ports、application（请求受理门）、adapters/settlementaccounting、adapters/inbox、按包裹的解析器（从 adapters/transportfulfillment 现有解析器里拆）；`cmd/parcel-dispatch` 路由与装配。
出处：[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定四；ADR-0171；ADR-0179 决定三。

## 做什么

1. 请求受理门从「必须点名发生项」放宽到「发生项与包裹二者有一」。
2. 把现有解析器里「按已受理包裹取实重尺寸（实测优先、缺席取申报，ADR-0171 决定二）与邮编路线」那一半拆成按包裹的解析器，原解析器组合它。快照加结算币种，取请求上快照的那一格。
3. SELL 请求翻译：方向 SELL、目的客户费用、计价范围与基准时点取自请求；选卡同受理前试算口径。复用现有的「由请求形成评价」编排（选卡、按回指幂等、入册、交付）。
4. inbox 消费者（名 `parcel-pricing/form-sell-evaluation-from-request`）与派发路由。

## 完成判据

- 计价侧用例：一份 SELL 请求形成一份评价并回指请求；未配置、适用冲突、输入不可得各一条。
- 真库装配：经生产路由表，一封 SELL 请求信封形成评价、发出评价已记录信封。

## 完成记录

2026-10-19 Claude Code 作者会话在分支 `claude-sell-03`（基于 `claude-sell-01`）落地。代码三笔，派发接线单独压在最后一笔：

- `90e0310b` 受理门收发生项或包裹，按包裹的解析器拆出并带结算币种（只动 `internal/parcelpricing`）。
- `453d3ef1` SELL 评价请求译成形成命令，收 SELL 请求信封的消费者（只动 `internal/parcelpricing`）。
- `0951fe0c` 派发路由接 SELL 评价请求消费者（只动 `cmd/parcel-dispatch`：`assemble.go` 与新用例文件）。票 02 同改路由表，合并冲突落在这一笔里。

另有 `b87dbd31` 机制清点重生成（含票 01 与本票的新增文件；票 01 那条分支没重生成）。证据级别：隔离合成，只记 `S`。

### 做了什么

1. **受理门**（做什么 1）：`FormEvaluationFromRequestCommand.accepted` 从「必须点名发生项」放宽到「发生项与包裹二者有一」，其余各格不动。`ports.EligibleSourceReferences` 加 `Parcel` 与 `ParcelReferenced`。
2. **按包裹的解析器**（做什么 2）：`pptransport.ParcelResolver`（`NewParcelResolver`/`ParcelResolverDeps`）只读 PS 的申报测量与地址要素，可选实测读口；键是请求点名的包裹，业务时点取请求的计价基准时点。实测优先、缺席取申报并标 `declaration`（ADR-0171 决定二）那一段从原解析器原样移过来。原 `Resolver` 经 TF 成员找到唯一包裹后整段交给它，`ResolverDeps` 不变。结算币种：`PricingInputQuery.Settlement`（零值即没带），带了就 `WithSettlementCurrency`；命令上同名一格由 SELL 翻译填，BUY 不填。
3. **SELL 翻译**（做什么 3）：`ppsettlement.TranslateSellEvaluationRequest` 把 SA `SellEvaluationRequestRecord` 译成 SELL + CUSTOMER_CHARGE 命令。计价范围、计价基准时点、包裹、结算币种取自请求，回指是 SELL 请求标识。来源事实引用与结算账户不进命令：计价不拿它们键事实。结算币种译不成计价的币种形（三位大写字母）就答 `ErrUntranslatableAnswer`，不拿卡币种顶。`FormOnSellEvaluationRequestSubmittedAdapter` 经 SA 现有的 `SellEvaluationRequests` 按标识回读（消费侧接口 `SellEvaluationRequestSource` 只声明 `FindByID`，SA 侧零改动），交同一只 `FormEvaluationFromRequestHandler`：选卡、按回指幂等、入册与交付都照旧。结果折法从 BUY 那一路抽成 `foldFormation`，两路共用，哨兵同一套。
4. **消费者与派发**（做什么 4）：`ppinbox.SellEvaluationRequestSubmittedConsumer`，名 `parcel-pricing/form-sell-evaluation-from-request`，只认 `settlement-accounting.sell-evaluation-request.submitted`，载荷（`tenantId`、`sellEvaluationRequestId`）缺一即毒丸。派发：路由表加一条，挂自己的未决名单 `sellEvaluationRequestSubmittedUndecidedSentinels`（成员与 BUY 那一路相同）。SELL 那只入口的 Inputs 装 `ParcelResolver`，不接 TF。入口的装法抽成 `formEvaluationFromRequest`，BUY、SELL 两路共用，序列与目录的在用解析因此只有一种装法；`preAcceptanceTrials` 头注里指向旧函数名的那一处跟着改了。

### 用例（先红后绿）

- 应用层 `application/form_sell_evaluation_from_request_test.go`：
  - 一份 SELL 请求形成一份评价并回指请求。选卡问的是 SELL/CUSTOMER_CHARGE、基准时点；输入查询带包裹与结算币种；评价 COMPLETED、CNY；同请求再来按回指答已存在。
  - 未配置、适用冲突、输入不可得各一格，都零写入、零铸造。
  - 受理门：只点名包裹的过；两者都没有、包裹带空白、发生项缺版本都不过。
  - 把受理门改回旧口径，上面三条全红（实测）。
- 解析器 `adapters/transportfulfillment/parcel_input_test.go`：
  - 按包裹从申报解出快照：业务时点取请求、结算币种 CNY、`declaration@baseline`。
  - 实测在场时顶替申报；包裹不在已接受委托上答不可得；只带发生项答不可得；读口故障上抛；缺口构造拒。
  - BUY 不带币种时快照不声明结算币种。
  - 原 `pricing_input_test.go` 一字未改，全绿，证 BUY 行为不变。
- 适配器 `adapters/settlementaccounting`：翻译逐格；币种、证据层级与零值记录拒。处理方：已形成与已存在入账；未配置、冲突、输入不可得、未决各是一枚哨兵；读不回是未决；引用坏了、记录译不出、未受理都原样上抛。BUY 那一路原有用例在 `foldFormation` 抽出后全绿。
- inbox 真库 `adapters/inbox/sell_evaluation_request_submitted_consumer_test.go`：恰处理一次；缺键与带 BUY 键名都是毒丸；失败回滚、重投成功；BUY 请求类型与评价已记录类型都响亮拒；nil 依赖拒。
- 派发真库 `cmd/parcel-dispatch/sell_evaluation_request_formation_test.go`，经生产路由表：
  - 准备：演示租户 SYN-PARCEL-01 申报 1.2 KG、30×20×10 CM、寄 200001 收 018956，经受理链接受；照演示种子登 SYN-PLAN-CN-SG-01、燃油序列 SYN-SERIES-FUEL-01、分区目录 SYN-CAT-ZONE-CN-SG 及两份复核；SA 真请求册与真交接口落一份 SELL 请求。
  - 正例：一拍之后信封 PUBLISHED，SELL 消费者 inbox 一行；按回指读到 SELL/CUSTOMER_CHARGE、COMPLETED、选到 SYN-PLAN-CN-SG-01、合计 CNY 的评价，快照结算币种 CNY、业务时点等于请求基准时点、申报标 `declaration@baseline`；以评价标识为 ID 的 `parcel-pricing.evaluation.recorded` 入队一封。
  - 反例：另一件包裹、范围下没卡的请求停在 `ErrPriceCardNotConfigured`、`dispatch.consumer_undecided`、inbox 零行、无评价。
  - 删掉路由条目时正例红在 `dispatch.no_subscriber`（实测）。

### 验证

在 `b87dbd31` 上，DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`：`go test -p 1 -count=1 ./internal/parcelpricing/... ./internal/settlementaccounting/... ./cmd/parcel-dispatch/ ./internal/architecture/` 全部 `ok`，本票新增真库用例逐条 `PASS`、无 `SKIP`。`go vet` 两处地盘退 0，`gofmt -l` 空，`go build ./...` 退 0。两笔中间提交各自检出后 build、vet 与包内用例也都过。

### 偏离与取舍

- **按包裹的解析器仍在 `adapters/transportfulfillment`**，没有挪去 `adapters/parcelshipment`。按发生项那一只要组合它；两只同包，就不必为组合导出「从包裹到快照」那一步，实测优先的取源规则也只有一个入口。目录名的复核本来就挂在 ADR-0171 越权风险点 1，归 PP owner。
- **结算币种由解析器加进快照**，不由入口加。它随查询交给输入读口，「快照由读口造」这一条因此不变。BUY 不带，零值，行为不变。
- **PP 侧停在形成之前的各格照 BUY 先例按未决重投**（未配置、适用冲突、输入不可得）。ADR-0190 决定五的「入账不重投」管的是 SA 收评价已记录那一侧，不管这一侧。这样补登价卡之后，在重投预算内仍能形成，算是对越权风险点 3「请求卡死」在计价这一段的缓和。要改成入账，只动派发的未决名单。
- **评价回指不分 BUY、SELL**：同一列 `request_reference` 装两种铸造标识，同租户下撞不上；读回指的一方（票 04）凭评价的方向与目的分辨该去哪本册。
- 入口没装 Inputs 时，「缺哪几只读口」那一组点名仍是 BUY 的。生产两路都装了，没为 SELL 另写一组。
- 没改 `docs/`。ADR-0190 Consequences 列的文档同步归协调方。另：PP `CONTEXT`「计价输入快照」里「某一计算目的是否拒用申报…不预设」那句，可补指 ADR-0190 决定四对 SELL·客户费用预估那一格的作答。

