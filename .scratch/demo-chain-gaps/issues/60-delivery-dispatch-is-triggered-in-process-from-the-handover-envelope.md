# 60 派送发起随交接信封在进程内逐对象发起

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg60（2026-10-19 作者子代理，见「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无
地盘：TF 新 `application/trigger_dispatch_on_handover.go`、新 `adapters/inbox/transport_handover_registered_consumer.go`（inbox 名与 VE 那一路分家）；`cmd/parcel-dispatch/assemble.go`（交接一行改 FanOut，`veHandoverUndecidedSentinels` 注释）+ 新 `assemble_delivery_dispatch_on_handover.go`（照 parcel-api `buildDeliveryDispatchTrigger` 另写一份，不跨 main 包复用）+ 新真库用例；`cmd/parcel-api/assemble_delivery_dispatch.go` 头注「实例半边」一句改指 ADR-0216；文档 ADR-0216、UC-TF-006 触发行与 AT、TF CONTEXT「揽收、派送与交付」一句、`docs/product/PILOT-PARAMETER-REGISTER.md` 的 `PAR-NET-09` 拍频格、演示脚本。
出处：[06](./06-delivery-dispatch-has-no-in-process-trigger-and-no-recipient-leg.md)；[psb/09](../../product-strategy-boundary/issues/09-tf-fulfillment-judgment-methods-and-connectors.md)「派送发起策略」；[psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「格 13 · 派送发起」；方案「代决」C10，「需要的 ADR」ADR-0216。

## 做什么

- TF 自消费交接信封，按对象此刻在场的段调既有触发执行器；各答复的入账与重投照方案 C10。
- 新消费者认 `DISPATCH_TASK_REFERENCE_TAKEN`（[tf-face-algebra/09](../../tf-face-algebra/issues/09-dispatch-trigger-task-reference-is-a-faithful-identity.md)）。
- 端点 `/transport-fulfillment-delivery-dispatch-triggers` 保留作补拍。
- 文档：起草 ADR-0216；`PAR-NET-09` 拍频格退役；psb/09 那一项与 psb/05 格 13 指向本票。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 派送段且派送要求各缝齐：形成一项任务；重投不重建。
- 非派送段、已离场、引用被占：入账，无任务。
- 要求缺失：入账，不重投。
- 缝读不动：登哨兵重投。
- 真库测试。
- 演示：SIN-LM 场外设备登末端交接（`segmentServiceAction=FINAL_DELIVERY`，计划段照当前演示声明；[64](./64-route-plan-carries-a-delivery-leg.md) 落地后改声明交付段）→ 派发一拍 `PUBLISHED`；再手打触发口同（段、对象）答 200 `DISPATCH_TASK_ALREADY_FORMED`；派送尝试、有效交付、PS 终局照走。

## 规模

M

## 完成记录（2026-10-19，作者子代理，分支 claude-dcg60）

基线 `3f7b2b48`（origin/main）。授权：用户原话「你作为业务和系统专家自决……解决所有的问题」；方案代决 C10 照 [handover-dispatch-plan](../handover-dispatch-plan.md) 落，方案没裁的细节按下文「代决」自决。

### 提交

- `987adc1f`：TF 编排与消费门（含用例）。
- `137dd2de`：派发装配、路由表交接一行改 FanOut、真库用例。
- `9493388c`：用例 Covers 的 AT 号对齐。
- `68a2672a`：ADR-0216 与各处文档。
- `0c852282`：立票 70、71。
- `1b51f2f9`：演示动线补记。
- `8a78a025`：AT 覆盖盘点追记。

### 落点

- `internal/transportfulfillment/application/trigger_dispatch_on_handover.go`（新）：`TriggerDispatchOnHandoverHandler` 按信封键读回那一版交接，只认`已交接`；再按对象问段登记册此刻在场的段，恰一个就交既有 `TriggerDeliveryDispatchHandler`，任务成立时间取注入的时钟。结果代数带 `Disposition()`，把入口与执行器两层答复折成入账 / 重投 / 缺陷三格。执行器本身一行未改。
- `internal/transportfulfillment/adapters/inbox/transport_handover_registered_consumer.go`（新）：消费门，inbox 名 `transport-fulfillment/trigger-delivery-dispatch-from-transport-handover`，与 VE 那一路分家。三格译成 nil、带 `ErrDeliveryDispatchOnHandoverUndecided` 的错误、带 `ErrDeliveryDispatchOnHandoverDefect` 的错误；四维缺一是毒丸。`initial_route_formed_consumer.go` 只改了包头注一句（本包也收自家信封）。
- `cmd/parcel-dispatch/assemble_delivery_dispatch_on_handover.go`（新）：三条要求缝照 parcel-api 的 `buildDeliveryDispatchTrigger` 另接一份，`deliveryDispatchOnHandoverConsumerOver` 是缝以外唯一的接线处；本路的未决哨兵名单只列一个哨兵。
- `cmd/parcel-dispatch/assemble.go`：交接那一行改为 `handoverFan`（VE 在前、TF 在后），`veHandoverUndecidedSentinels` 与 `wireDispatcher` 头注随之改。只动这几处，没重排既有行。
- 文档：
  - ADR-0216（新）；
  - ADR-0114 的 Status 行加前向指针，并新起 Links 节；
  - `docs/adr/README.md` 的 0114 行末追加「生产入口已答」，并在 0212 行后追加 0216 行；
  - UC-TF-006 的触发行，以及 `AT-TF-191`–`200`；
  - TF CONTEXT「揽收、派送与交付」那一句；
  - 参数登记册 `PAR-NET-09`：拍频格退役；
  - 开发主线重定级表后补记 PN-04 那一格过时；
  - 演示动线脚本补记；
  - AT 覆盖盘点追记。
- psb/09 第 2 项、psb/05 格 13 立票时已指向本票，这次没再改。

### 代决与越权风险点

代决（方案 C10 之外，本票自决）：

1. **入口只认`已交接`**。拒收、待确认在入口入账，不查段、不问执行器。
   - 越权风险点：若 owner 要这两格也交执行器、由它答`不是触发事实`，改编排一处即可（ADR-0216 越权风险点 3）。
2. **段取「此刻在场的段」**（方案原话「对象此刻在场的段」），不比对链尾入场依据是否就是这一版交接；零个入账，多个上抛 `ErrObjectActiveInSeveralSegments`，与结束参与那一侧同一个错误。
   - 越权风险点：信封晚到时，可能替后一段形成任务（ADR-0216 越权风险点 2）。
3. **处置三格放在应用层的 `Disposition`**，消费门只做翻译。执行器各格逐一列出，不留默认：执行器日后多一格时会落进缺陷，而不是悄悄入账。
4. **缝没接线（`*_SOURCE_NOT_WIRED`）在本路按缺陷处置**：不登哨兵、落发布失败。ADR-0114 决定三把它答成未决，但在派发这一路重投接不上缝。
   - 越权风险点：ADR-0216 越权风险点 6。
5. **执行器不受理、答出封闭集合以外的格、自家键过不了构造门**，都按缺陷处置，不入账、不登哨兵。
6. **处理方跑在 inbox 那一笔事务里，不另包事务**：任务那一行与 inbox 的「已处理」同笔提交。重投时消费门先认出已处理，执行器不会再问所有者。
7. **事件类型串由 tfinbox 自己写出**：提供方那一份是 postgres 适配器的未导出常量。三处相等由真库用例钉住——生产交接口交出的信封经生产路由表到了两路。
8. **交接更正（仍是`已交接`）会自动另立一项任务**：沿用执行器按入场依据铸键的现状，本票不改执行器。原先要人手打，现在自动发生。
   - 越权风险点：ADR-0216 越权风险点 7。
9. **AT 号段 191–200 用满**：200 给「补拍答已在册」。

方案 C10 本身的越权风险点「要求缺失不会自愈，要人补拍」，记进 ADR-0216 越权风险点 1，并另立票 [70](./70-delivery-dispatch-left-unformed-has-no-trace.md)。ADR-0216 的 Status 写明待 TF owner 复核。

### 各判据对应用例

- 派送段且三件齐，形成一项任务；重投不重建：
  - `application/trigger_dispatch_on_handover_test.go` 的 `TestAHandoverIntoADeliverySegmentFormsTheTaskAtTheDispatchClock`、`TestRerunningTheHandoverBeatFindsTheTaskAlreadyFormed`；
  - `cmd/parcel-dispatch/delivery_dispatch_on_handover_test.go` 的 `TestTheHandoverLaneFormsOneTaskAndNeitherRedeliveryNorARetickRebuildsIt`：同一封投两次库里仍一项，所有者只被问一轮；补拍执行器答`已在册`、交回同一项。
- 非派送段、已离场、引用被占，入账无任务：`TestHandoversThatAreNotATriggerSettleWithoutATask` 的「段不是派送段」「已离场」「引用被占」三个子用例；拒收那一格在同一用例里，另有 `TestARefusedOrPendingHandoverIsNotATrigger`。
- 要求缺失入账、不重投：
  - 生产路由那一条用真缝：`TestAHandoverIntoADeliverySegmentReachesTrackingAndDispatchThroughTheProductionRoute`。空库里三个所有者都答没有；信封 `PUBLISHED`，两路各一行 inbox，任务 0 项。
  - 替身那一条：上一用例的「要求缺失」子用例。
- 缝读不动，登哨兵重投：`TestAnUnreadableRequirementSeamIsRetriedUntilItReads`。错误既是 `dispatch.ErrConsumerUndecided`，也带本路哨兵；inbox 0 行；缝恢复后重投形成任务。
- 缺陷与毒丸：`TestOwnDefectsAreNeitherSettledNorRetried`；tfinbox 的 `TestTheHandoverDispatchGateSettlesRetriesOrFailsAsTheOrchestrationDisposes`、`TestAHandoverEnvelopeMissingAKeyFieldIsPoison`。
- 路由表：`assemble_test.go` 的 `TestARegisteredTransportHandoverReachesTheConsumerThroughTheRouteTable` 改为核两路各一行 inbox。

### 验证

- `go build ./...` 退 0。
- `go vet ./cmd/parcel-dispatch/ ./cmd/parcel-api/ ./internal/transportfulfillment/...` 退 0。
- `gofmt -l cmd internal` 无输出。
- 带 DSN 跑 `go test -p 1 -count=1 ./internal/architecture/ ./internal/transportfulfillment/... ./cmd/parcel-dispatch/...`，全 ok。代码定在 `137dd2de`，此后只改了用例注释与文档；改完又重跑 architecture、TF application、tfinbox 三包，全 ok。
- 没跑全量，没重生成机制清点（按派单，进 main 时统一做）。

### 演示实走（证据 `S`）

用一次性库 `dcg60walk`，`seed.sh` 原样灌入退 0；`parcel-api` 与 `parcel-dispatch` 都用本分支构建，dispatch 七个变量照动线「前置」。

1. 经首方客户渠道提交 `SHR-WJPP3YXSGLGMI5HIQW4JJL7ET4`，成`已接受`；初始路由 `RPV-000000000001` 三段。
2. 上海枢纽设备收寄（标识用客户包裹号），答 `201 INTAKE_FORMED`，PS 采用落行。
3. 新加坡尾程场外设备（`SYN-DEVICE-OFFSITE-SG-LM-01`）登末端交接：段 `SYN-SEG-SIN-LM-60`，`plannedSegment=RPV-000000000001#3`，`segmentServiceAction=FINAL_DELIVERY`。答 `201 HANDOVER_REGISTERED`（16:38:28Z）。
4. 派发一拍：交接信封 attempt 1 即 `PUBLISHED`（16:38:30Z）。VE 与 TF 两路 inbox 各一行 `PROCESSED`。任务 `DELIVERY-DISPATCH/v2/17:SYN-SEG-SIN-LM-60/31:DPCL-QUMX2OTOBC6WJLA6PLIJI2B52Y/31:TRANSPORT-HANDOVER/SYN-HO-60/v1` 形成：
   - 地点 `DPR-1:…/DELIVERY_PLACE/baseline`；
   - 窗口取计划第三段；
   - 条件 `CLO-6d0045efddb09af2`；
   - `opened_at` 16:38:30.199Z，即派发时钟。
5. 手打补拍口，同一段与包裹答 `200 DISPATCH_TASK_ALREADY_FORMED`，回显同一项；库仍 1 项。
6. 派送尝试 `201 ATTEMPT_RECORDED`；有效交付 `201 DELIVERY_REGISTERED`，参与结束；PS 终局 `EFFECTIVE_DELIVERY`。
7. 除两类本就无订阅者的信封外，outbox 无一行非 `PUBLISHED`；dispatch 日志里没有 TF 那一路的失败。

走完已停进程、删库，也删了自起的发行方容器。

实走里踩到的两处，都不是本票引入的：

- **交接信封 ID 超长**（产品缺陷，已有票）。第一次实走，交接的 `scope` 与 `version` 写得长，信封 ID 是 129 字节，越过上限 128。交接照登 201，答复却带 `handoffReference`，outbox 里没有那封信，派送发起也就收不到。交接信封 ID 仍是四维直拼，已在 envelope-id-bounds/01 的 TF 例外清单里。第二次实走换了新库、写短标识；演示动线补记里写明了这一条。
- **共享发行方配置旧**（环境问题，不是产品）。共享的集成客户端发行方容器（5558）挂的是旧检出 `idp-parcel-psb31` 的 `integration-dex.yaml`，没有合成货主客户端，首方提交取不到令牌。实走时另起了一只私有 Dex：同一份当前配置、同一串 issuer，监听 5596。parcel-api 首方族的 JWKS 指向它，issuer 串不变，所以客户渠道册的行照样对得上。共享容器没碰。

### 偏离与待裁

- **没改 `cmd/parcel-api/assemble_delivery_dispatch.go` 的头注**（票面地盘列了这一项）：该文件此刻在会话 idp-parcel-9b 的在途地盘内。`TriggerDeliveryDispatchCommand` 注释里同样过期的那一句也在其地盘。两句一并立票 [71](./71-stale-cadence-comments-point-at-adr-0216.md)，ADR-0216 Consequences 已指过去。
- **越出地盘的改动**：
  - `adapters/inbox/initial_route_formed_consumer.go`：包头注一句；
  - `cmd/parcel-dispatch/assemble_test.go`：交接路由用例（派发令牌）；
  - 开发主线补记、AT 覆盖盘点追记。
- **进 main 时的令牌与热点**：
  - 派发令牌按方案是 60 → 61 → 63 → 67；
  - 文档令牌按方案是 59 → 60：本票改了 TF CONTEXT 与 UC-TF-006，59 若先进 main，rebase 时两处都是追加或单句替换，冲突应只在行内；
  - `docs/adr/README.md` 与 `.scratch/at-coverage-inventory.md` 是追加行，与同波票可能撞在同一位置，按「两段都留」合。
- **待 owner 复核**：ADR-0216 越权风险点 1–7（TF owner；第 5 条同时待产品）。
