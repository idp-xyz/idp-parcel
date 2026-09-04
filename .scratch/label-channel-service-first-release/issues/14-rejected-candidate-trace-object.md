# 14 落选留痕挂在哪个对象上，不清楚

Category: enhancement
Status: resolved——MCP-2 2026-09-04 按「裁决」节落地于分支 `mcp2-lc14`（基线 `27ced913`，待 MCP-1 重放入 main；分支 SHA 见文末「完成记录」）：领域对象 + 重建门 + 只追加登记册端口 + postgres 头行/子行两表（迁移 `parcel_shipment/0015`）+ 择优编排落定后同事务写入 + 标识签发器；运营查阅面另立票 [22](./22-channel-selection-decision-operations-read-face.md)（draft）。留痕对象已裁（MCP-3 2026-09-04，owner 授权自决）：**是**，择优决定作为只追加的决定记录落 PS 侧，引用候选与出局因由、不拷内容
Blocked by: 无（01、12 均已 resolved）

## 裁决（MCP-3，2026-09-04）

**留痕是什么：一条只追加的「渠道择优决定」记录，归 `parcel-shipment`，有所有者、不可覆盖。** 它不是来源事实（ADR-0005 意义上
的外部发生），是本上下文自己形成的一次**判断记录**——与实际承运商判断（ADR-0103）、权威交接判断同属「本上下文形成、带
版本、只追加」那一族。票面那一问「是事实还是一次判断的过程记录」两个选项都不准：它是**判断结果的记录**，随每次择优各成一条，
不随「判断版本」原地演进，也不是可丢弃的过程日志。

**为什么要有它、且今天就能立**：`PAR-NET-16` 已确认的机制部分原句「日常择优可在已确认候选集合内自动进行并**逐次留痕**」；
CONTEXT-MAP 说的「未被选中的候选**评价**继续有效并留痕」由 `parcel-pricing` 不删评价保证，那是评价那半——**择优决定本体**
（这一次、对这一票、在这些候选里、按这条规则、选了谁、谁出局及为何）今天没有任何登记，`SelectChannelCandidate` 算完即散。
留痕要求与逐票样本（留多久、给谁看）在登记册里是**待提供**，那是实例半边；决定记录的**形状**由机制句推得出，现在就做。

**形状（引用，不拷贝）**：租户；被择优的对象引用（面单交易/包裹侧的引用，按票 12 择优编排的入参取）；决定时刻；采用的择优
规则引用（首发只有成本单维，规则引用仍要写——它是日后多维时的版本锚）；候选集合引用（每候选：渠道候选引用 + 所用 `BUY`
评价引用，**不拷金额、不拷评价内容**）；逐候选结果：选中 / 出局（因由取 `ChannelCostUnavailability` 四格，一一对应
`parcel-pricing` 四种非完成结果，票 13 已把四格分开正为此处）/ 落选（可计价但不最优）/ 并列冲突（`PAR-NET-16`「并列且无法
选出唯一一条时为冲突，交人工裁决」——冲突也是一次决定的结果，同样留痕，不是不留）。整条记录只追加：重跑择优形成新记录，
不改旧记录；同一票多条记录按决定时刻构成择优历史。

**不复用 NR `RouteCandidate`**：票 01 已裁，且合格判据不同（网络可达 vs 渠道约束与成本）；形状像不构成理由。**不落 PP**：
评价归 PP，择优归 PS（CONTEXT-MAP「具体渠道选择」那一条），决定记录跟着择优走。

**谁读它**：运营查阅面，沿 ADR-0077 读面通例另立读口与票；本票只落登记（领域对象 + 端口 + postgres 追加表 + 择优编排落库后
同事务写入）。**留多久**：实例半边（`PAR-NET-16` 留痕要求待提供），机制不设保留期默认值，不做清理口。

**红线沿票面**：不拷候选内容与金额；不填任何实例取值；SYN 夹具只记 `S`。

**能力边界**：读过本票、`PAR-NET-16` 全行、CONTEXT-MAP 两处「留痕」句、票 13 的 Comments（四格出局因由的来历）、票 01
裁决摘要（spec 表）；**没读** `select_channel_candidate.go` 的入参形状与 `channel_candidate_cost.go` 全文——形状节里「按
票 12 择优编排的入参取」即为此留的口，实施时以代码为准。裁的是**留痕是不是要立、立在哪、引用什么**，不裁表结构细节。

## 缺口

`PAR-NET-16` 要求给落选者留痕。今天有一个形似的东西但**不是它**：
`internal/networkrouting/domain` 的 `RouteCandidate` 带 `CandidateOutcome` 与
`CandidateReason`（`visibilityexception/adapters/networkrouting/derive_on_initial_route.go`
消费它）——那是**路由**候选的留痕。

渠道候选是否复用它、还是归 `parcel-shipment` 面单交易侧，**没有代码可指**。

## 为什么被两票阻塞

- `01` 裁定择优住哪个上下文——留痕对象大概率跟着择优走。
- `12` 产出候选集合的形状——留痕挂在候选上，候选长什么样先得有。

## 做什么

定落选留痕的对象与生命周期：挂在哪、留多久、谁读它。一件要正面回答的事：**留痕是不是
事实**——若是，它就有所有者与不可覆盖的要求；若只是一次判断的过程记录，随判断版本走即可。
两者的存储与读面形状不同。

## 红线

- 不复制第二套候选：留痕引用候选，不拷贝它的内容。
- 不填任何候选内容或金额（实例半边）。
- 若判定要复用 `RouteCandidate`，**要说明渠道候选与路由候选为什么是同一种东西**——它们的
  合格判据不同（一个看网络可达，一个看渠道约束），复用要有理由不能靠形状像。

## 完成判据

留痕对象有明确归属与形状并落地；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第三段；票 `01`、`12`。

## 完成记录（MCP-2，2026-09-04）

**落点（分支 `mcp2-lc14`，基线 `27ced913`；SHA 为分支上的，重放入 main 后由 MCP-1 在广播里对照新旧 SHA）**

| 层 | 文件 | SHA |
|---|---|---|
| domain | `channel_selection_decision.go`（`ChannelSelectionDecision`、`FormChannelSelectionDecision`、四格 `ChannelCandidateOutcome`、结论三格、`ChannelSelectionSubject`、规则引用 `ChannelSelectionByCostOnly`）；`channel_candidate_cost.go` 只加：`ChannelCandidateCost.WithEvaluation/Evaluation`，`SelectChannelCandidateByCost` 的排序抽成包内 `rankChannelCandidatesByCost` 供记录复用，对外签名与四种出口不变 | `6832320` |
| domain（重建门） | `channel_selection_decision_rehydration.go`（`RehydrateChannelSelectionDecision{,Spec}`，只校不重算）；`internal/architecture/rehydration_gate_test.go` 受限名单加两行 | `0187639` |
| ports + postgres + 迁移 | `ports/channel_selection_decision.go`（`ChannelSelectionDecisionRegistry`：`Append` + `ListBySubject`，无 Update/Delete；`ChannelSelectionDecisionIdentity`）；`adapters/postgres/channel_selection_decisions.go`；`migrations/parcel_shipment/0015_channel_selection_decision.sql` | `55ba728`（规则列译码函数补于收口笔） |
| 挂点 | `application/select_channel_candidate.go`：Deps 纯加法 `Decisions/DecisionIDs/Clock`，三件一起缺席行为不变、只到一半报 `ErrChannelSelectionRecordingMisconfigured`；选出/并列/无人参选各成一条，成本表对不上与币种不齐不成记录 | `44187b1` |
| 桥 | `adapters/parcelpricing/cost_bridge.go`：译完成本后带上 `evaluation.ID()` 作评价引用（已确立与出局两格都带；没登记价卡的候选无评价，如实缺席） | `e255602` |
| 标识 | `adapters/identity`：`ChannelSelectionDecisions`，前缀 `CSDN` | `fe46b5a` |
| 清点 | `docs/product/MECHANISM-INVENTORY.md` 在 `fe46b5a` 干净检出上重生成 | `9e9fcbc` |

**决定记录的字段**：租户；决定标识（签发口铸）；被择优对象引用 =（商业范围引用 + 产品—渠道映射引用，照择优编排入参
`ChannelSelectionQuery` 取——入参里今天没有面单交易/包裹引用，不凭空造）；候选装配时点（`query.At`）；规则引用
`COST_ONLY`；决定时刻（时钟）；结论三格 `SELECTED / TIED / NONE_QUALIFIED`；逐候选：候选引用、所用 `BUY` 评价引用（可缺）、
四格 `SELECTED / NOT_SELECTED / EXCLUDED / TIED`、出局因由（`PENDING_EVIDENCE / RATECARD_EXCLUSION / CONFLICT / NOT_FORMED`，
与 `ChannelCostUnavailability` 同一集合）。**只引用不拷贝**：无金额列、无评价内容列。

**裁决之外由代码定的两处，写明**：① 币种不齐不成记录——那一次没有比较发生（比较器拒绝比较），硬记会把一次没发生的择优
写成一条决定；② 被择优对象引用不含「装配时点」，时点单独成列——同一（范围 + 映射）在不同时点重跑是同一对象的择优历史，
不是不同对象。

**验证**（钉 `fe46b5a` 及其后两笔文档/译码笔）：`gofmt -l` 空；`go build ./...`、`go vet ./...` 退 0；无 DSN 全仓
`go test -count=1 ./...` 零 FAIL（PG 用例跳过）；**含真库** `go test -count=1 -p 1 -v ./internal/parcelshipment/...
./internal/architecture/... ./migrations/...`：1527 PASS / 0 SKIP / 0 FAIL，其中
`TestAChannelSelectionDecisionRoundTripsThroughPostgres` 与 `TestChannelSelectionDecisionsRefuseToRunOutsideATransaction`
均 `--- PASS`（非 SKIP），`TestEmbeddedMigrationAssetsCarryNoCarriageReturnOrBOM` PASS。`-race` 未跑。
两份棘轮基线零改动：新类型与两个工厂经 application / adapters 的选择器可达。

**无生产装配点**：`cmd/` 下 `SelectChannelCandidate` 零命中（票 12 收口 Comment「生产可达仍差两步」），留痕随择优
编排的接线票一起可达；装配时把 `adapters/postgres.NewChannelSelectionDecisions`、`adapters/identity.NewChannelSelectionDecisions`
与时钟一起交给 `SelectChannelCandidateDeps`，缺一即 `ErrChannelSelectionRecordingMisconfigured`。
