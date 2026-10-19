# 11 外部结果已收信封没有订阅者，每封都撞 no_subscriber

Category: enhancement
Status: 完工，待评审与进 main——分支 `claude-dcg11`
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

外部结果落库后，`customs-compliance.external-result.received` 没有任何订阅者，每封都撞 `dispatch.no_subscriber`。这是最响的装配错误格，直到失败预算耗尽落 ABANDONED。结果本身已入库，但演示日志持续报红，放行事实也到不了 VE 或下游

## 证据（读码）

internal/customscompliance/adapters/postgres/external_result_handoff.go `externalResultEventType`；cmd/parcel-dispatch/assemble.go `wireDispatcher` 路由表没有这个类型；internal/platform/dispatch/dispatcher.go `failureCodeFor` 把 `ErrNoSubscriber` 定为最响一格。演示 VE 映射种子 visibility/01-milestone-mapping-v1.json 已登 CUSTOMS_COMPLIANCE/external-result → CUSTOMS_CLEARED，但没有消费者能产出这种事实

## 三类

机制。规模 M。

## 最小的诚实修法

接一个真接得住的消费者：VE 投影按 scope→单元→成员逐成员展开（同 `deriveDeclarationSubmissionConsumer` 的 ADR-0066 做法），只派生已形成的层；或者由 CC 的放行门禁核对来消费。接不住之前维持 no_subscriber，不登记空消费者

## 要裁的

谁消费外部结果：VE、CC 门禁核对，还是两者都要？演示映射把 external-result 一律映成 CUSTOMS_CLEARED，与 CC CONTEXT「不得用一个清关成功状态覆盖各层」相冲，应改成只映放行层，归 VE owner 定

## 既有票

none（.scratch/outbox-handoff-consumption-map/report.md 记为「应有但未开」，没有立票）

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg11`）

### 接票时核实

现象与证据逐条复核成立：`wireDispatcher` 路由表没有 `customs-compliance.external-result.received`；VE 侧没有任何适配器产出 `external-result` 这个类型词，演示映射那一行永远命不中；若照它造一个单一类型词的消费者，每一层外部结果都会被这一行映成清关。另核得：结果范围在 CC 接收编排里就是申报单元标识（编排沿范围→单元→案件取辖区），可以照 ADR-0066 逐成员展开；映射目录按（源上下文，事实类型）建键，类型词由 VE 消费适配器决定，所以「按层区分」在键上做得到，不需要改映射键的形状。

### 代决（用户授权「你作为业务和系统专家自决，解决所有的问题」）

权威落在 [ADR-0197](../../../docs/adr/0197-customs-external-results-enter-tracking-projection-per-layer.md)，这里只记对「要裁的」两问的答案与偏离：

1. **谁消费**：VE 投影订阅，只投 VE、不 FanOut。CC 放行门禁核对不订阅：它绑定拟执行动作与监管边界，由动作的核对请求驱动；需要外部结果的执行方在动作时向 CC 问。
2. **演示映射**：关务那一行改为只登 `external-result-release-full` → `CUSTOMS_CLEARED`。类型词按层分开，放行层再按全部 / 部分 / 附条件分开（词表在 ADR-0197 决定二）。
3. **偏离票面倾向**：非放行层**也进投影**，形成本层类型词的未归类条目，而不是入账不派生。理由：它们是已接受关务事实（UC-CC-006 交接表把任一层交给 VE），运营追踪查阅要看得见受理、查验、税费各层；租户要为某层登里程碑只登映射、不改代码；「不制造假的可见事实」由「映射未登记即未归类」保证，不必靠丢事实。完成判据里「非放行层不形成」按「不形成清关里程碑」落用例。
4. **接不住的格**：同层冲突的那份入账不派生；归属不上的留存、放行三件与层次不配、范围单元答非所问为仓储不变量已破，响亮失败、不进未决名单。

### 越权风险点（全文见 ADR-0197「越权风险点」）

1. 代 VE owner 定非放行层也进投影；owner 若只要放行层，改适配器层次分支，词表不动。
2. 代 CC owner 定类型词表；词一经租户映射引用，改名要迁映射。
3. 部分放行逐成员展开：成员条目说的是「所在单元部分放行」，不是「本件已放行」。
4. 监管过程决定、处置决定只有层级粒度，查验与扣留分不开。
5. 同层冲突不进投影，VE 看不见「关务进展待确认」；冲突信号要 CC 先给出冲突关系。
6. 组成取申报单元而不取提交版本快照；单元今天只建立不更新，两者相同。
7. 有效时间取 CC 的「业务发生或适用时间」；CC 若把适用时间单独存出要改取它。
8. CC 放行门禁核对不订阅；要到达即重核挂起门禁时另立票，加一路 FanOut 即可。

### 做了什么

- `internal/visibilityexception/adapters/inbox/external_result_consumer.go`：消费门 `ExternalResultConsumer`，认 `customs-compliance.external-result.received`，载荷两维（租户、来源标识）缺一即毒丸。
- `internal/visibilityexception/adapters/customscompliance/derive_on_external_result.go`：处理方 `DeriveOnExternalResultAdapter`——按键重读接收记录 → 同层冲突入账不派生 → 层次（放行再按种类）译成类型词 → 结果范围→申报单元→组成 → 逐成员交派生编排。引用 `external-result/<成员>/<来源标识>`，版本取提交版本，发生 = 有效 = CC 的业务发生或适用时间，接收取 CC 落记录时刻。
- `cmd/parcel-dispatch/assemble.go`：`deriveExternalResultConsumer` 与路由登记；未决哨兵名单 `veExternalResultUndecidedSentinels` 只收接收记录读不回、范围单元读不回与派生编排未决。
- `scripts/demo-seeds/data/visibility/01-milestone-mapping-v1.json`：关务一行改登 `external-result-release-full`；`scripts/demo-seeds/README.md` 补已灌库要 `--reset`。
- `docs/adr/0197-…`、`docs/adr/README.md`。

### 用例

- 领域 / 应用层（替身 CC 读口 + 真 `DeriveProjectionHandler` + 按类型键的映射替身）`derive_on_external_result_test.go`：全部放行在两个成员上各形成本层事实并归入清关；其余各层与部分、附条件放行只形成本层未归类条目；重放不再签发投影版本；同层冲突入账不派生且不取范围单元；成员未决整封回滚；单成员冲突不拖垮其余；接收记录 / 范围单元读不动读不回落未决；不变量已破各格响亮失败；引用译不出；缺依赖拒绝构造。
- inbox 真库 `external_result_consumer_test.go`：恰一次、缺维毒丸、处理失败回滚重投、外类型响亮报错、与申报提交账本分家、缺依赖拒绝构造。
- `cmd/parcel-dispatch` 真库装配 `external_result_projection_test.go`：经生产路由表一拍 PUBLISHED，按**演示种子文件**登的映射，全部放行两成员归入 `CUSTOMS_CLEARED`；业务受理、部分放行定稿且未归类；同层冲突定稿不派生、无派生信封；失败分格（未决三格落 `consumer_undecided`，其余 `publish_failed`）。

### 验证

于 `c66afa0d`：`go build ./...`、`go vet ./cmd/parcel-dispatch/ ./internal/visibilityexception/...`、`gofmt -l cmd internal scripts` 干净；`go test ./internal/architecture/` 绿（两道棘轮基线不用动）；`IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1 ./internal/visibilityexception/... ./cmd/parcel-dispatch/ ./cmd/parcel-ve-register/ ./internal/customscompliance/adapters/postgres/` 全绿。种子改前先跑装配用例，全部放行那一格按旧种子红（成员未归类），改种子后绿——证的是种子行与适配器类型词真对得上。未跑全量，未重生成机制清点。

### 偏离与待裁

- 偏离票面倾向一处（见代决 3）。
- 顺带核得、**未在本票改**：演示映射另一行（`TRANSPORT_FULFILLMENT`，`transport-handover`）→ `IN_TRANSIT` 同样命不中——TF 交接适配器按裁决产出 `handover-handed-over` / `handover-refused` / `handover-pending-confirmation`，没有 `transport-handover`。演示动线上 `IN_TRANSIT` 因此永远不归类，建议另立票（同样只映 `handover-handed-over`）。
- 待立票：VE 侧关务冲突信号（UC-VE-001 `CC-RESULT`，要 CC 先给出冲突关系）；执行方在动作时向 CC 问放行（与本组 05「出发门禁不问关务」同一条缝）。
- 已灌过的演示库：`SYN-VE-MAP-V1` 版本号已占，重跑种子答已登记、库里仍是旧行，要 `--reset`。
- `.scratch/outbox-handoff-consumption-map/report.md` 第 23 行状态未刷新（该报告自带保质期声明，按 HEAD 重取证时自然更新）。
- ADR 编号进 main 时若改号，全仓搜 `ADR-0197` 与 `0197-` 一并替换。
