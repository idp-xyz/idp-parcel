# 来源更正 → 参与关系重派生：交接与揽收两种来源的更正今天都不进段

Category: enhancement
Status: resolved——MCP-5（2026-09-07，task-edb0a8c9 接管 task-854580ed / task-82fd973a；隔离分支 `mcp5-tf-batch`，merge-base `2efef58e`，已验 tip `968b0fa1`）。三问已裁（见「裁决」，落 [ADR-0112](../../../docs/adr/0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md)），实施见「完成记录」；进 main 的 SHA 由推送方重放后另记
Blocked by: 无

## 缺口

CONTEXT 生命周期一句：「来源证据被更正或事件有效性变化 → 保留原段、原参与关系和原判断，形成失效或替代关系并重新
派生当前有效控制与履约结果。」这句话的**前半**（保留原判断、形成替代版本）今天两种来源都有——`TransportHandover.Correct`
与 `OffsitePickup.Correct` 都形成回指前版的新版本；**后半**（参与关系跟着重派生）两种来源都没有：

- `RegisterTransportHandoverHandler.Correct` 只落新版本并重交意图，不碰段登记册。
- `RegisterOffsitePickupHandler.Correct`（票 [08](08-offsite-pickup-correction-model.md)）照交接那一侧的现状，同样不进段。

而参与关系今天以来源版本为 `entryBasis`、以来源业务时间为 `enteredAt`（`JoinWithPickup` 取 `OFFSITE-PICKUP/<版本>` 与
`OccurredAt`；`JoinWithHandover` 同形），更正一旦改了控制证据或发生时刻，段里那条参与关系指着的就是一个已被回指的版本、
一个已被更正的起点。

## 票 08 已裁的那一半

> 附问：更正改了控制证据或发生时刻，参与起点是同段新起点还是新段？——**同段**，但这半边不在本票。段是共同控制责任
> 范围，「承运责任变了才是另一段」（ADR-0103 主体判据；CONTEXT「可验证的实际承运责任或运输控制边界发生变化时，结束原
> 参与并形成下一段」）——更正证据或时刻不改变谁在控制，所以不是新段。

所以本票不必再裁「新段还是同段」，要裁的是**同段内怎么表达「替代参与」**：原参与关系不能被取消、删除或回写为未发生
（CONTEXT 硬句），新起点又要按新版本重派生——是参与关系上长出版本链（形照来源那一侧的 `Corrects`），还是段上另立一条
参与关系回指原参与并把原参与标为被替代，两条路对 `ActualFulfillmentSegment.ParticipationFor`、`Active()`、
`SummarizeHandovers` 式的折叠读法影响不同。

## 为什么一票覆盖两种来源

只在揽收一侧补它会让两种来源的更正在段上行为不一致——票 08 裁决原句。触发点是两条：`RegisterTransportHandoverHandler.Correct`
与 `RegisterOffsitePickupHandler.Correct`；进段那道门今天两侧共用 `enterFulfillmentSegment`，重派生那道门也该共用一处。

## 先答再开工

- 参与关系的「替代」在领域上长什么样（见上）——`/domain-modeling`，落 CONTEXT「履约参与关系」词条。
- 重派生与更正编排是否同事务：票 06 把「结束前一段参与」放进交接编排同事务的理由是「输入全在 TF」；重派生的输入
  （新版本、原参与）也全在 TF，但票 09 裁决③给过反例的判据，要对着再走一遍。
- 段已关闭（`ActualFulfillmentSegment.CloseSegment` 之后）时更正来源的参与怎么办：CONTEXT「实际履约段结束 → 判断历史封存：不再接受
  新版本，除来源事实更正引起的重新派生」——这一句正是本票的例外格。

## 裁决（2026-09-04，通道 5，task-82fd973a；owner 授权自决，理由与被否替代在 ADR-0112）

1. **替代参与的形状**：参与关系上长链——新参与回指被替代参与的入场依据（来源版本引用），原参与一字不动；当前参与 = 链尾，`ParticipationFor` 答链尾，`ActiveParticipations` 只数链尾，被回指的版本在聚合内标已被替代（派生态不落列），`Active()` 为否。库面：主键换（租户+段+对象+入场依据），根唯一 + 每入场依据至多被替代一次两条部分唯一索引 + 自引用外键（迁移 0016）。与 ADR-0117 同形，理由同族（只插不改、当前派生）；不采「另立参与并标原参与被替代」（要 UPDATE 原行）。CONTEXT「履约参与关系」词条补一句替代参与版本。
2. **同事务，派生一侧**：输入全在 TF（新版本 + 段上原参与），票 09 ③的「读三个外部上下文」判据不成立，票 06 的判据成立。失败不回滚更正：登记册故障留续办引用；领域正当拒绝单开答格（`NO_PARTICIPATION_TO_REDERIVE` / `CORRECTION_WITHDRAWS_CONTROL`）。两触点共用 `rederiveFulfillmentParticipation`；段由登记册按对象反查（新读口，含已离场的当前参与），更正命令不带段号。
3. **段已关闭仍重派生**（CONTEXT 封存例外格）：替代版本照插，段不重开不再关；替代版本继承原参与的离场三件，更正后起点晚于继承终点即拒。
4. **失效格**（更正撤回控制转移）：属同一条链的一格，形状为回指前版、标失效的版本；本票如实答 `CORRECTION_WITHDRAWS_CONTROL` 不实施，拆到 [11](./11-control-withdrawing-correction-voids-participation.md)。
5. **「更正后的起点晚于继承的终点」答格**（2026-09-07 补裁，通道 5，task-edb0a8c9）：ADR-0112 决定三把它定为领域正当拒绝，决定二给正当拒绝的处方是「不留引用但单开答格」，而决定二点名的只有两格；接管前的实现把它落进「其余错误不出声」那一路——来源更正已落、段那一半一声不吭，与重派生成功在调用方眼里同形。补裁：按决定二同一条规则给第三格 `CORRECTED_START_AFTER_INHERITED_END`，领域具名 `ErrCorrectedStartAfterInheritedEnd`（包着 `ErrInvalidFulfillmentSegment`），链尾不动、不欠账。**不改 ADR-0112 正文**：它是同一条规则下多出来的一格，不是取舍变更；ADR 的「加两格」读作当时点名的两格。

**能力边界**：读了 `actual_fulfillment_segment.go`、`segment_rehydration.go`、`fulfillment_segment.go`（端口）、`fulfillment_segment_registry.go`、`enter_fulfillment_segment.go`、两处 `Correct`、`transport_handover.go` 的 `Correct` / `TransferOutBasis`、迁移 0006、CONTEXT 相关句、ADR-0097/0103/0117；**未读** `end_fulfillment_participation.go` 全文（只确认它用 `ParticipationFor`）、`ActualCarrierJudgment` 的重派生口、NR/PS 对参与变化的消费。**越权风险点**：① 参与表主键换四元（0006 头注写「不需要版本维」，本裁决推翻它——依据是 CONTEXT 生命周期句的后半）；② `EndParticipation` / `FindActiveSegments` 的「在场」判据从 `ended_at IS NULL` 改为「且无人回指」；③ 段已关闭仍插替代版本；④（接管会话补）`SegmentEntryRefusal` 多出 ADR-0112 未点名的第三格 `CORRECTED_START_AFTER_INHERITED_END`（裁决 5），ADR 正文未改。

**接管会话的能力边界**（2026-09-07）：读了本分支上全部 TF 改动（领域、重建门、端口、PG 登记册、迁移 0016、重派生门、两处 `Correct`、HTTP 替身与三份新用例）与 ADR-0112 / 本票；**未读** `ActualCarrierJudgment` 的重派生口、NR/PS 对更正后参与的消费、`perform_offsite_pickup.go` 全文（只改了一句注释）。按 parallel-sessions「先写自己第一片 red 再读对方代码」：真库经编排用例的断言全部取自 ADR 原句，首跑即绿，作对接管前三笔的独立印证。

## 红线

- 原参与关系与原段一字不动（只插不改，与两本登记册同一条纪律）。
- 不自动关段，不改承运主体判断的现有版本——判断按更正关系重新派生是 `ActualCarrierJudgment` 自己的下一版，走它自己的口。
- 实例值留空拒默认；SYN 夹具只记 `S`。

## 参照

票 [08](08-offsite-pickup-correction-model.md) 裁决附问；`internal/transportfulfillment/domain/actual_fulfillment_segment.go`
的 `JoinWithPickup` / `JoinWithHandover`；`internal/transportfulfillment/application/enter_fulfillment_segment.go`；
`docs/domain/transport-fulfillment/CONTEXT.md` 生命周期节；ADR-0103。

## 完成记录

分支 `mcp5-tf-batch`，merge-base `2efef58e`（main 此后只多 `.md` 与 PS 代码，与 TF 唯一重叠是 `cmd/parcel-api/unwired_orchestration.go` 非 TF hunk，未 rebase）。分支上的 SHA 作封存出处；进 main 的 SHA 由推送方重放后广播，届时并列补记。

| 分支 SHA | 内容 |
|---|---|
| `00cc28f4` | docs(adr)：ADR-0112 + README 行 + 本票「裁决」+ 立 11 draft + TF CONTEXT「履约参与关系」词条补替代参与版本一句 |
| `8b70c052` | feat(domain)：`FulfillmentParticipation.Supersedes/Superseded`、`ParticipationFor` 答链尾、`ParticipationHistory` 链序、`ActiveParticipations` 只数链尾、`RederiveParticipationWithPickup/WithHandover` + 共用 `rederive`（继承离场三件、段已关闭照长）、`ErrNoParticipationToRederive` / `ErrCorrectionWithdrawsControl`、重建门 `markSupersededByChain` 核链形（一个首登、回指不落空、不分叉、不自指） |
| `7e7f0e25` | chore(salvage)：端口 `Supersede` / `FindSegmentsForObject` + `ParticipationSupersedeOutcome`；PG 登记册 `Supersede`（只插不改，与 `Join` 同一段 INSERT）、`currentParticipationPredicate`（NOT EXISTS 回指）进 `FindActiveSegments` 与 `EndParticipation`、`FindSegmentsForObject`、读回带 `supersedes_entry_basis`；HTTP 替身同步；迁移 `0016_fulfillment_participation_supersession_chain.sql`（主键换四元、`supersedes_entry_basis` 列 + 非自指非空白 CHECK、自引用外键、`one_root_per_object` 与 `supersedes_once` 两道部分唯一索引） |
| `114d2b30` | chore(salvage)：应用层 `rederiveFulfillmentParticipation` 门（按对象反查、逐段问领域门、`Supersede` 落库、失败留续办引用不翻更正）、两处 `Correct` 各挂 `rederiveParticipation`、`SegmentEntryRefusal` 加 `NO_PARTICIPATION_TO_REDERIVE` / `CORRECTION_WITHDRAWS_CONTROL`、领域 `currentParticipationEnteredBy`（先判无可替代后判撤回控制）、编排替身用例与 PG 替代链用例 |
| `bcd58298` | test(postgres)：真库经编排两向——揽收更正替代（根行 SQL 复核不动）/ 未进段答无可替代；交接更正替代 / 撤回控制答格原参与不动；段已关闭仍重派生且继承终止三件 |
| `f22825ba` | feat：裁决 5——`ErrCorrectedStartAfterInheritedEnd` + 第三格 `CORRECTED_START_AFTER_INHERITED_END`，真库用例先红后绿 |
| `89c57904` | test(http)：揽收更正端点面对齐（未进段透出 `NO_PARTICIPATION_TO_REDERIVE`；带段首登再更正走到重派生门） |
| `4d89d703` | docs：双轴评审修补——过时 / 带计数注释四处 |
| `968b0fa1` | chore(inventory)：机制清点在 `4d89d703` 干净检出重生成（TF 生产 118→119、测试 106→109、迁移 15→16） |

**验证**（干净 detached 检出 `968b0fa1`，`$env:TEMP\idp-mcp5-tf10-verify`）：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0；无 DSN `go test -count=1 ./...` 99 包 ok / 0 FAIL（PG 用例跳过）；含 DSN `go test -p 1 -count=1 -v ./...` **6955 PASS / 0 FAIL / 0 SKIP，99 包 ok**（含 PS 侧消费 TF 端口的包与 `cmd/parcel-dispatch`、`cmd/parcel-api`）；探针 `TestAPickupCorrectionSupersedesTheParticipationInTheDatabaseWhileTheRootRowStays` 未设 DSN `--- SKIP`、设 DSN `--- PASS`；同一检出上重跑清点生成器零差。`-race` 本机无 cgo 未跑，由 CI 覆盖。

**双轴评审**（基 `2efef58e`，无独立 worker，串行自评）：Standards——四处注释过时或带计数，已在 `4d89d703` 修；`markSupersededByChain` 对悬空回指 / 分叉 / 双根一律答 `ErrObjectAlreadyParticipating` 属命名判断题，保留（都是「链外的第二条参与」，重建门用例已钉）。Spec——决定一至四逐项对上（迁移四件、`Supersede` 只插不改、`FindSegmentsForObject`、两处 `Correct` 共用一门、同事务靠 ctx 携带 `RequireExecutor` 与首登进段同形、段已关闭照长且继承终点、撤回控制答格、CONTEXT 词条、README 行）；范围外只有裁决 5 那一格，已记越权点 ④。

**红线自查**：原参与与原段无任何 UPDATE（`EndParticipation` 仍只填离场三列且只作用于链尾）；不自动关段；`ActualCarrierJudgment` 一字未动；`FindByKeyAndVersion` 签名未动；`internal/parcelshipment/**` 未碰；领域包只导 std；夹具全为合成（S）。

**未落 / 拆出**：失效格 → [11](./11-control-withdrawing-correction-voids-participation.md)（draft，阻塞边已清）；NR/PS 对更正后参与的消费不在本票（ADR-0112「不在本记录内」）。

## Comments

- 2026-09-04 · MCP-3：立票。起因是票 08 裁决把段侧重派生划出更正票之外并要求一票覆盖两种来源。**只写票面，未动代码。**
- 2026-09-07 · 通道 5（task-edb0a8c9）：接管两次封存现场（`7e7f0e25`、`114d2b30`），先写真库经编排两向 red 再对读对方用例，补裁决 5 与 HTTP 断言，双轴评审后收口。完成记录见上。
