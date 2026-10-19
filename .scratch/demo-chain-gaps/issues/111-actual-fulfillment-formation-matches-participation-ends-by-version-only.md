# 111 实际履约发生项自动形成只按 `TRANSPORT-HANDOVER/<版本>` 找结束的参与：同一包裹两次交接用同一版本号，交接信封整封弃置

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-dcg111（2026-10-19 Claude Code 子代理立，演示动线重走，[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「重走（钉 `76686b7b`，2026-10-19）」停点一）。
Blocked by: 无
地盘：TF `application/form_actual_fulfillment_on_participation_end.go`（`formFor`）；若改依据拼法，另及 `domain/transport_handover.go`（`TransferOutBasis`）、`domain/actual_fulfillment_segment.go`（`ParticipationEndedBy`）与存量迁移。与 [89](./89-actual-fulfillment-formation-blocks-later-envelopes-in-the-handover-and-delivery-partitions.md) 相邻。
出处：[ADR-0218](../../../docs/adr/0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 找参与那一步「多于一条：库面不一致，响亮失败」；[ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末补记停点 `ENTRY_HANDOVER_AMBIGUOUS`（交付门禁那一侧已认「入场依据只带版本、不带范围」，先按业务时间再分）；演示动线票 95、98 补记给的交接载荷（每次交接各起 `scope`，`version` 写 `v1`）。

## 现象（实测，钉 `76686b7b`，只记 `S`）

演示包裹 `DPCL-6NWOLEGPYLZ5VDROK2UL2Z52BU`（一次性库，走完已删）照动线补记登五次`已交接`，范围与版本依次是：

- `SHA`/`v1`：上海交给自营干线，立自营段；
- `SZX`/`v1`：交进深圳口岸，结束自营段上的参与；
- `SZX`/`v2`：交给干线承运商，立干线段，段采用干线委托；
- `SIN`/`v1`：承运商交进新加坡枢纽，结束干线段上的参与；
- `SINLM`/`v1`：交给自营尾程，立派送段，没结束任何参与（`NO_ACTIVE_PARTICIPATION`）。

段登记册上，自营段那条参与的入场与终点依据都是 `TRANSPORT-HANDOVER/v1`，干线段那条入场 `TRANSPORT-HANDOVER/v2`、终点 `TRANSPORT-HANDOVER/v1`。

- `SIN`/`v1` 的交接信封：消费者 `transport-fulfillment/form-actual-fulfillment-occurrence-from-transport-handover` 三次都报 `ErrParticipationEndedInSeveralSegments`（`basis TRANSPORT-HANDOVER/v1` 对上两段），信封落 `ABANDONED` / `dispatch.publish_failed`。同一封信的另外几个消费者（NO、NR、VE、派送发起）各自 `PROCESSED`。干线段的 `ACTUAL_FULFILLMENT` 没形成，BUY 请求也没发。
- `SINLM`/`v1` 的交接信封：它没结束任何参与，照样对上那两段，同一错误，同样 `ABANDONED`。
- 绕法：配置员经 `POST /transport-fulfillment-charge-occurrence-registrations` 对干线段人工登 `ACTUAL_FULFILLMENT`，答 `201`；随后 BUY 请求、计价 `COMPLETED`、供应商预期成本一行依次形成。

对照：`8197fa2f` 那次重走每次交接的版本串各不相同（`H2/v1`、`H4/v1` 等），同一消费者形成正常，交接信封都 `PUBLISHED`。

## 成因（读码）

- 参与的入场与终点依据都是 `TransportHandover.TransferOutBasis` 拼的 `TRANSPORT-HANDOVER/<版本>`，不带范围。
- `FormActualFulfillmentOnParticipationEndHandler.formFor` 列出对象有过参与的各段，数终点种类与依据串都对上的参与；多于一条就报上面那个错，ADR-0218 把这一格当库面不一致。演示这组数据却是合法的：两次交接范围不同、版本串相同。
- 别处已经认了这种歧义：`FulfillmentParticipation.EnteredByHandover` 另比业务时间；交付门禁读入场交接时先按业务时间再分，仍多于一版才答 `ENTRY_HANDOVER_AMBIGUOUS`。`formFor` 没有这一步。
- 这个错误是确定性的，重投不会变，信封照重投预算烧完才弃置。

## 三类

- 机制：两条路——
  - `formFor` 照 `EnteredByHandover` 与交付门禁的做法，按这次交接的业务时间与参与的终点时刻再分；仍多于一条时答一个指名的结果（未决或入账），不整封失败；
  - 参与依据带上范围（例如 `TRANSPORT-HANDOVER/<范围>/<版本>` 一类拼法），连同存量迁移；NO 的转出引用用的是同一个串，要一并看。
- 产品策略：无。
- 租户取值：无。

## 完成判据

- 同一包裹两次结束参与的交接用同一版本号时，各自形成发生项或各自答不形成，交接信封 `PUBLISHED`；没结束任何参与的交接答「没结束参与」。
- 真库用例照上面那组范围与版本的次序复现；证据记 `S`。

## 要裁的

- 走哪条路。第二条牵动 NO 的转出引用（ADR-0214 越权风险点「转入依据由本上下文铸、不复用 TF 的转出引用」），规模更大。
- 本票落地之前，动线文档的交接载荷示例要不要先改成「同一包裹的各次交接版本串各不相同」。句子列在 psb/05 那一节的停点建议里。

## 规模

S–M

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg111`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。票面两条路走第二条，裁定写进新 [ADR-0221](../../../docs/adr/0221-a-handover-participation-basis-is-minted-from-the-handover-key.md)；本节只记落地与代决，裁定内容以 ADR 为准。

### 做了什么

- **领域**（TF `domain/handover_participation_basis.go`，新文件）：`已交接`作参与依据时由交接键铸——`TRANSPORT-HANDOVER-KEY/` 后接对象、范围、版本三节「字节长度:原串」（`TransportHandover.ParticipationBasis`）。进段、下一次交接结束、交接更正长出的替代与失效版本都记这一串。认法一处定义：逐字对上的是那一次；只带版本的旧拼法存量按版本加按它立下的时刻认（`ControlFactMatch` 两格）。`EntryHandoverVersion`、`EnteredByHandover` 从交付门禁读数那一份搬来并改认两种拼法；新增 `EndedByHandover` 与 `ActualFulfillmentSegment.ParticipationEndedByHandover`。`TransferOutBasis` 只给 NO，仍是旧串。
- **重派生**：被替代的那一条按被更正版本的键认，存量按版本加起点认；回指写在册入场依据原串。
- **自动形成**（`form_actual_fulfillment_on_participation_end.go`）：逐字对上的优先；逐字对上多于一条仍响亮失败；没有逐字的、旧拼法恰一条就取它；旧拼法多于一条答新格 `ENDED_PARTICIPATION_AMBIGUOUS`、入账。
- **交付门禁读数**：认法随领域那一处，新依据只认得出范围相同的一版；`ENTRY_HANDOVER_AMBIGUOUS` 自此只出自存量。注释随之改（端口、PG 读口、停点）。
- 不加迁移（TF 迁移号 0044 未用）、不加信封、不改派发路由与哨兵名单、不改 NO。
- **文档**：ADR-0221；ADR-0218 Status 行与 Links 加前向指针、README 该行写部分停用；ADR-0213 Links 加一条；ADR README 追加 ADR-0221 一行。TF CONTEXT「载运对象、计划与实际履约」一条；UC-TF-005 步 9 引 ADR-0221、`AT-TF-255` 措辞改为「逐字对上」、新立 `AT-TF-266`–`270`、`272`；UC-TF-006 新立 `AT-TF-271`、`AT-TF-262` 注明只出自存量。演示动线第 3 步补记一则。AT 盘点追记。
- **立票**：[112](./112-node-control-transfer-out-reference-names-only-the-handover-version.md)（NO 转出引用只带版本）、[113](./113-effective-delivery-end-basis-names-only-the-delivery-result-version.md)（有效交付终点依据只带版本）。

### 代决

1. **走「依据由完整交接键铸」，不只在自动形成里按业务时间再分**。后者留着同版本、同时刻、不同范围的歧义，委托与段采用的开始依据、派送任务引用仍指不回交接，重派生还要另修一遍。
2. **拼法另起起头 `TRANSPORT-HANDOVER-KEY/`，不在旧起头下加 `v2` 标记**。旧版本是任意串，旧起头下的新串可以与旧串逐字相同（票 tf-face-algebra/09 的已知局限）；另起起头新旧串集合不相交。对象进串，租户不进。
3. **起头常量不以 Prefix 结尾、不进标识前缀门禁**：它是依据串的起头（带横线与斜杠），与 `OFFSITE-PICKUP/`、`DELIVERY-DISPATCH/` 同类，不是签发器的类别前缀；命名照 `deliveryDispatchTaskReferenceRoot`、NO `handoverInControlBasisRoot` 取 `…Root`。
4. **存量照读不改写**；旧拼法单条对上也要时刻对上，这条从交付门禁读数推到结束与重派生两处。
5. **旧拼法多条对上答 `ENDED_PARTICIPATION_AMBIGUOUS`、入账**，不响亮失败：重投不会变，响亮只会把信封烧成 `ABANDONED`、挡同分区后面的信封（ADR-0218 越权风险点 8）。逐字对上多条仍响亮。
6. **NO 的转出引用不换**：那是 NO 的取舍，`internal/nodeoperations/**` 此刻在另一会话地盘里；换了 TF 的 `TransferOutBasis`，NO 的既有测试就要跟着改。立票 112。
7. **有效交付那一侧同族缺口不在本票修**，立票 113。
8. **新 ADR 而非补记**：依据串落在段登记册、委托与段采用的开始依据里，派送任务引用由它派生，两种拼法从此长期并存，属难逆转的身份取舍；部分停用 ADR-0218 决定一两句与决定六一条。ADR-0213 补记的停点不停用，只加 Links。
9. **票 tf-face-algebra/09 的撞键用例改造**：新依据里带着对象，原来「段 `seg/A` 加对象 `CU-77` 与段 `seg` 加对象 `A/CU-77`」那组在旧拼法任务引用下不再撞。`cmd/parcel-api` 三条撞键用例改由 `collidingEntries` 按新依据凑出撞键的段与版本，`application` 那条「旧拼法串本身合新形状」照新依据重算；编排单测那条拼接撞键只改注释、把前提写明是旧依据时代的。
10. **票面「要裁的」第二条**：动线文档不再补「各次交接版本各不相同」那句。本票落地之后不必守，补记里写明。

### 越权风险点

1. 存量歧义仍在：旧拼法里同版本、同时刻、不同范围的两次交接仍认不出；自动形成入账之外没有信号（与 ADR-0218 越权风险点 6、票 88 同一去处），交付门禁读数停在 `ENTRY_HANDOVER_AMBIGUOUS`，重派生在这一格可能对上不相干的参与。演示库与开发库按一次性库用，生产没有存量。
2. 旧拼法单条对上也要时刻对上：本票之前单条逐字就算。生产路径上时刻一律相等，据此判定没有存量会变；手工改过参与时刻的行会答没有参与被结束、入账。
3. 同一次交接两串：TF 参与上是新依据，NO 转出引用是旧串，查阅按交接键对（票 112）。
4. 派送任务引用变长，对象在引用里出现两次；存量参与的任务引用不变。
5. 有效交付终点依据仍只带版本（票 113）。
6. 改了票 tf-face-algebra/09 的证据用例（代决 9）：用例的意思保住了，但撞键输入换了，评审时要按新依据核一遍前提。

### 用例

- `AT-TF-266`（拼法、单射、读回只认一种写法、进段与结束各认自己那一次）、`267`（演示判据：编排与派发生产路由真库两半）、`268`（旧拼法按版本加时刻认）、`269`（旧拼法多条答认不出、入账）、`270`（逐字优先）、`271`（交付门禁：新依据认得出进段那一次）、`272`（存量参与的更正重派生）。号段 266–275 用到 272，273–275 未用。
- 新用例：`domain/handover_participation_basis_test.go`、`application/form_actual_fulfillment_handover_basis_test.go`、`application/read_delivery_gate_test.go::TestAKeyedEntryBasisTellsTwoScopesReusingTheVersionApart`、`cmd/parcel-dispatch/actual_fulfillment_handover_version_reuse_test.go`。`AT-TF-262` 的歧义格改用旧拼法存量参与摆。

### 验证（证据只记 `S`）

- `go build ./...`、`go vet`（TF 各包、`cmd/parcel-api`、`cmd/parcel-dispatch`）、`gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1`：`./internal/architecture/`、`./internal/transportfulfillment/...`、`./internal/nodeoperations/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/` 全 ok（NO 各包不改、照绿）。
- 演示判据：`TestHandoversThatAllWriteV1FormTheTrunkOccurrenceAndFireBuy` 照重走的次序登 SHA/v1、SZX/v1、SZX/v2（随后采用干线委托）、SIN/v1、SINLM/v1，派发生产路由表逐拍跑：五封交接信封都 `PUBLISHED`，自动形成那一路各一行 inbox，干线发生项恰一项、BUY 请求恰一份，两段终点依据各指回 SZX/v1 与 SIN/v1。把 `internal/` 退回改动之前再跑同一用例：第一封 SHA/v1 就停在 `PENDING` / `dispatch.publish_failed`。
- 变异核验：去掉旧拼法的时刻条件，旧拼法三条用例变红；铸法去掉范围，拼法、单射、演示判据与交付门禁用例变红。
- 种子未改，没有在一次性库上 `seed.sh` 实走；演示判据由上面的真库装配用例证。

### 偏离与待裁

- TF 迁移号 0044 未用：存量照读、不改写，没有库面变化。
- psb/05「重走（钉 `76686b7b`）」停点一的去处与动线建议句，进 main 时由协调方回填；本票没改 psb/05。
- 票 112、113 待分诊；112 的地盘此刻在另一会话手里。
