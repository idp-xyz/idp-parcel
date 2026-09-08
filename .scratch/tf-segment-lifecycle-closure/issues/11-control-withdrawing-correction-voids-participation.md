# 撤回控制转移的更正让参与关系失效：链上的失效版本怎么落、对关段与在场判据的影响

Category: enhancement
Status: resolved——由票 10 裁决 4 / ADR-0112 决定四拆出（2026-09-04，通道 5，task-82fd973a）；2026-09-08 通道 4（task-c2003926）在分支 `mcp4-tf11`（基 `d1e6c094`）逐条裁定「要定的」（见「裁决」）后会话 crash；同日通道 4 新会话（task-a8e6a834）接管，从封存现场重切后按裁决实施完（见「完成记录」），分支 `mcp4-tf11`；进 main 的 SHA 由推送方重放后另记
Blocked by: 无（10 已 resolved，替代链在 main）

## 缺口

交接更正可以把`已交接`改成`已拒收`或`待确认`（`TransportHandover.Correct` 收 `Verdict`）。改完之后新版本不再转出控制（`TransferOutBasis` 不给），段里凭前版入场的参与关系没有了入场依据——CONTEXT 说这时要「形成失效……关系并重新派生当前有效控制」。票 10 落的替代链只覆盖「更正后仍转出控制」的替代格；撤回控制那一格今天在重派生门如实答 `CORRECTION_WITHDRAWS_CONTROL`，参与照旧站着，与事实不符。

## 已裁（ADR-0112 决定四）

失效属替代链的一格：一条回指前版、标失效的版本；链尾失效即该对象在本段当前无有效参与；原参与一字不动。

## 要定的

- 失效版本的列：入场种类照前版还是另立一格？入场依据取撤回控制的那一版引用；入场时刻取什么（它没有控制起点）。
- 对关段不变量的影响：链尾失效的对象算不算「在场」（应不算），`CloseSegment` 与重建门怎么数。
- 对已离场参与的失效：原参与已按有效交付离场、随后更正撤回入场控制——交付事实还在，失效版本要不要继承离场三件（ADR-0112 决定三对替代格是继承；失效格未必）。
- 对 PS 采用口的影响：PS 那侧（ADR-0117）对撤回控制的更正怎么反应，是否要另一封信封。

## 裁决（2026-09-08，通道 4，task-c2003926；owner 授权自决，全部在 ADR-0112 决定四的字面内，不立 ADR-0121）

先说全局：ADR-0112 决定四已经把形状定死——「链上一条回指前版、标失效的版本；链尾失效即该对象在本段当前无有效
参与；原参与一字不动」。下面每条都是把这句话落到列与读法上，没有一条改它的方向，所以不立新 ADR；0121 释回。

1. **失效版本是链上的一版，不是原参与上的一格状态。** 与替代版本走同一条链、同一个窄写口 `Supersede`（只插不改）：
   `supersedes` 回指被失效那一版的入场依据，行上多一列 `voided`。「一格状态」要 UPDATE 原参与，违只插不改；「另立一种
   对象」会让 `ParticipationHistory` 断成两截。**入场种类照前版**（`TRANSPORT_HANDOVER`）——它是同一条来源谱系的下一
   版，`ParticipationEntryKind` 仍是封闭二值；只有交接更正走得到这一格（揽收更正构造期就拒「更正成失败到访」）。
   **入场依据取撤回控制那一版的引用** `TRANSPORT-HANDOVER/<新版本>`——`TransferOutBasis` 对拒收/待确认不给，但版本引用
   本身是有的，它是「这一版判断说了什么」的身份，与替代版本的入场依据同一种写法。**入场时刻沿用被失效那一版的起点**：
   失效版本没有控制起点，但它记的是「起于 T 的那条参与失效了」，T 是它回指的事实；对交接来源这本就是裁决业务时间
   （`Correct` 不改 `JudgedAt`），沿用不引入第二个时刻；列保持 NOT NULL，链按入场时刻排序不乱。
2. **失效后当前有效控制 = 无，不回退到前一仍转出控制的版本。** 链尾是失效版本时 `ParticipationFor` 仍答链尾（它是当前
   版本），`Active()` 为否、新增 `Voided()` 为是；`ActiveParticipations` 不数它。不回退：被撤回控制的那一版已被回指，
   回到它就是把已被更正的判断重新当成当前——CONTEXT「不能简单回填为早期原控制方」。**再次进入本段只有一条路**：更正那一版
   撤回控制的判断（v3 更正 v2、裁决回到`已交接`）——链尾从失效版本长出替代版本，参与重新在场；凭新的控制事实再进本段
   `join` 照旧答`已在段内`——与「已结束的参与不重开，再次进入是新的段」同一条规矩（ADR-0103 主体判据：控制边界变了才是
   另一段）。
3. **失效版本继承原参与的离场三件**，与替代版本同一条规则（ADR-0112 决定三：对象的控制终点是它自己的事实，更正入场不改
   它）。理由有二：一是链的继承要能传下去——失效版本若丢掉终点，随后一版再更正回`已交接`就会继承出一条「已交付却又在场」
   的参与；二是交付事实并没有被更正撤销（CONTEXT「后续事实不得被删除或倒退」），失效版本如实记着「这条参与的对象已按 D 离场」，
   `Voided()` 说明入场被撤回，两件事各说各的。起点沿用前版，「起点晚于继承的终点」那一格在失效版本上不可能发生。
   **段已关闭照样长失效版本**，段不重开——同一条例外格。
4. **在场判据与关段判据都不数失效版本。** 在场 = 未离场 ∧ 无人回指 ∧ 未失效——领域 `Active()`、`FindActiveSegments` 与
   `EndParticipation` 的 SQL 谓词三处同一条；被失效的对象不会被结束（`EndParticipation` 对失效链尾答`已离场`那一格，
   编排在指名段那一路答 `OBJECT_NOT_IN_SEGMENT`——对象在本段当前无有效参与，说的就是这件事），按对象反查在场的两路
   （交付 / 下一次交接）答 `NO_ACTIVE_PARTICIPATION`。`CloseSegment` 与重建门按 `Active()` 数：段里唯一对象被失效后段可以
   关（没有人在控制中），关段声明仍是人的决定，不自动。`closesBeforeAParticipationEnded` 照旧比全部已离场版本的终点——失效
   版本继承的终点与原参与同刻，不引入新约束。「曾在场」留在 `ParticipationHistory` 里读：失效版本的 `EnteredAt` 是被撤回
   的那个起点，读它的人先看 `Voided()`。段仍算 `Established`——它由当时的控制事实成立过，不能回写为未发生。
5. **`CORRECTION_WITHDRAWS_CONTROL` 那一格退场。** 它是决定四落地前「不猜也不静默」的临时答格；失效版本落地后撤回控制不再是
   段那一半的拒绝而是链上的一版，成功时两格都空，与替代版本同形。`ErrCorrectionWithdrawsControl` 同退。ADR-0112 正文不改
   （它写的是「本记录的实施对这一格如实答」，是历史）。
6. **PS 采用口不在本票。** 交接更正的意图已经经既有 `HandOffTransportHandover` 重发一份（裁决 `REFUSED`/`PENDING_CONFIRMATION`
   带 `corrects`），消费方按 ADR-0117 自己的链反应；TF 不另铸一封「参与已失效」的信封——ADR-0112 明写 `parcel-shipment` /
   `network-routing` 对更正后参与的消费不在其内，那是消费方的票。

**迁移 TF 0019**：`fulfillment_participation` 加 `voided boolean NOT NULL DEFAULT false` 与一条 CHECK——失效版本必回指前版
（首登不能失效：对象从未进段就没有东西可失效）且入场种类为 `TRANSPORT_HANDOVER`。领域重建门守同一形。

**能力边界**：读了 ADR-0112 全文、票 10 裁决与完成记录、TF CONTEXT 词条与生命周期句、`actual_fulfillment_segment.go` /
`segment_rehydration.go` / `transport_handover.go` / `rederive_fulfillment_participation.go` / `register_transport_handover.go` /
`end_fulfillment_participation.go` / `trigger_delivery_dispatch.go` 的读法、PG 登记册与迁移 0006 / 0016、两份替身与三份
既有用例；**未读** `ActualCarrierJudgment` 的重派生口、NR / PS / NO 对交接更正意图的消费、`close_fulfillment_segment.go` 全文
（只确认它经领域 `CloseSegment`）。**越权风险点**：① 失效版本继承离场三件——ADR-0112 决定三写的是替代格，本票把同一条规则
用到失效格（裁决 3）；② 凭新控制事实再进已失效的段答`已在段内`（裁决 2 后半）是本票按「已结束不重开」类推的，CONTEXT 没有
逐字写失效那一格；③ 退掉一个已在 HTTP 面透出过的答格 `CORRECTION_WITHDRAWS_CONTROL`（裁决 5）。

## 红线

- 原参与、原段只插不改；不自动关段；不改承运主体判断现有版本。
- 不从计划推事实。

## 完成记录

分支 `mcp4-tf11`，基 `d1e6c094`（main 此后只多 `.md`，TF 地盘 `git diff --stat d1e6c094 main -- internal/transportfulfillment migrations/transport_fulfillment` 为空，未 rebase）。
封存笔 `6e5c8a0c`（推送方代封存的 red 中途现场）按 parallel-sessions「未提交现场」节重切：其改口并入领域笔，原指针留作本地 `salvage/mcp4-tf11-red`，不进 main。分支上的 SHA 作封存出处；进 main 的 SHA 由推送方重放后广播，届时并列补记。

| 分支 SHA | 内容 |
|---|---|
| `503dfcc6` | docs(scratch)：本票「裁决」六条（已由通道 1 重放进 main `bd5ccb9e`） |
| `3559f5d5` | feat(tf/domain)：`FulfillmentParticipation.Voided()`；`Active()` 加「未失效」；`RederiveParticipationWithHandover` 对撤回控制的更正经共用 `rederive` 门插失效版本（回指前版、入场依据 `TRANSPORT-HANDOVER/<新版本>`、种类照前版、起点沿用前版、继承离场三件）；`ErrCorrectionWithdrawsControl` 退场；重建门 `RehydrateParticipationSpec.Voided` + 守「失效必回指前版 ∧ 种类为 TRANSPORT_HANDOVER」；用例四个新增、一个改口 |
| `59b0bb1e` | feat(tf/application)：`rederiveFulfillmentParticipation` 退撤回控制那一格，失效版本经 `Supersede` 落库、两格都空；`SegmentEntryRefusal` 退 `CORRECTION_WITHDRAWS_CONTROL`；`EndFulfillmentParticipationHandler` 与 `TriggerDeliveryDispatchHandler` 对失效链尾答 `OBJECT_NOT_IN_SEGMENT`；两份段登记册替身学 `Voided`、「在场」收成 `active()` 一处；用例改口一个、新增一个 |
| `244ad85d` | feat(tf/postgres)：迁移 `0019_fulfillment_participation_voided_version.sql`（`voided boolean NOT NULL DEFAULT false` + CHECK 回指前版 ∧ `TRANSPORT_HANDOVER`）；登记册写读 `voided`；`activeParticipationPredicate`（未离场 ∧ 未失效 ∧ 链尾）供 `FindActiveSegments` 与 `EndParticipation` 共用；真库用例走通拒收→失效版本→反查/结束→更正回`已交接`重新在场 |
| （本笔） | docs：本票 Status/完成记录；tf spec 票一览 11 行；TF CONTEXT「履约参与关系」词条补失效参与版本一句 |
| （随后） | chore(inventory)：机制清点在干净检出重生成 |

**逐条对裁决**：1 失效版本的列 → 领域 `rederive(voided=true)` + 迁移 0019 + 重建门；2 当前有效控制为无、不回退、再进本段只经更正回`已交接` → `Active()` / `ParticipationFor` 答链尾 / `join` 照旧答`已在段内` / 领域与真库用例各证一遍 v3 更正 v2 重新在场；3 继承离场三件、段已关闭照长 → 共用 `rederive` 的继承分支 + `TestAVoidedParticipationInheritsTheEndAndLeavesAClosedSegmentClosed`；4 在场与关段判据三处同一条 → 领域 `Active()`、SQL `activeParticipationPredicate`、两份替身 `active()`；结束参与与派送触发对失效链尾答 `OBJECT_NOT_IN_SEGMENT`，按对象反查两路经 `FindActiveSegments` 答 `NO_ACTIVE_PARTICIPATION`；5 `CORRECTION_WITHDRAWS_CONTROL` 退场 → 编排枚举与领域错误同退，ADR-0112 正文不改；6 PS 采用口不在本票 → 未碰 `internal/parcelshipment/**`。

**验证**：见完工报（干净 detached 检出 tip 上 gofmt / build / vet / 含 DSN `go test -p 1 -count=1 ./...` 计数、探针一正一反）。

**红线自查**：原参与与原段无任何 UPDATE（`EndParticipation` 仍只填离场三列且只作用于在场链尾；失效版本只经 `Supersede` INSERT）；不自动关段（`CloseSegment` 仍是声明）；`ActualCarrierJudgment` 一字未动；`internal/parcelshipment/**` 未碰；领域包只导 std；夹具全为合成（S）。

**能力边界**（接管会话）：读了本票裁决与票 10 完成记录、ADR-0112、TF CONTEXT 词条与生命周期句、领域替代链与重建门、`rederive_fulfillment_participation.go` / `enter_fulfillment_segment.go` / `end_fulfillment_participation.go` / `trigger_delivery_dispatch.go`、PG 段登记册与迁移 0016、两份替身、既有编排与真库用例；**未读** `ActualCarrierJudgment` 的重派生口、NR / PS / NO 对交接更正意图的消费、HTTP 端点对 `SegmentEntryRefusal` 的透出位置（grep 无 `CORRECTION_WITHDRAWS_CONTROL` 引用，未逐文件读）。**越权风险点**在「裁决」末段三条之外多一条：④ 派送触发对失效链尾改答 `OBJECT_NOT_IN_SEGMENT`（票 09 / ADR-0114 决定二写的是「被替代或已离场的参与不触发」，本票把失效格并入「对象不在段内」而非「参与已不在场」）。

## Comments

- 2026-09-04 · 通道 5：由票 10 拆出立票，只写票面，未动代码。
- 2026-09-08 · 通道 4（task-c2003926）：接票。先裁「要定的」四条（全部落在 ADR-0112 决定四的字面内，不立 ADR-0121），再按裁决实施；本笔只动票面。
- 2026-09-08 16:4x · 通道 1：全通道 crash 后清点，裁决笔只在分支上、main 上本票仍是 draft——为防重裁，把 `503dfcc6` 重放进 main（`bd5ccb9e`）并改 Status 点明现场位置；封存笔 `6e5c8a0c` 按规矩不进 main。ADR-0121 号已释回（`docs/adr/` 无 0121，README 无行）。
- 2026-09-08 17:15 · 通道 4（task-a8e6a834）：接管。先读封存 diff 报现场分析（改口对、用；辅助与专用用例未写完，`domain_test` 包红），再按 /tdd 三层各红一次绿一次：领域 → 编排 → 迁移 0019 + PG。封存笔重切进领域笔，原指针留 `salvage/mcp4-tf11-red`。完成记录见上。
