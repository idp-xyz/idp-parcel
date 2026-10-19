# 50 TF 装载分配与终止参与两口没核重放代数就接上了真渠道：逐口裁现有格够不够

Category: task
Status: in-progress——2026-10-19 完工，待推送方自审进 main（纯 `.md`，分支 `claude-oc5039`），见「裁定」与「完成记录」。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（50 与 39 同一分支 `claude-oc5039`，先 50 后 39）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 15 之四补核）
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判簇 C6 第五张草案；逐口核研判
地盘：
- internal/transportfulfillment/application/form_load_assignment.go 及其测试
- internal/transportfulfillment/application/end_fulfillment_participation.go 及其测试
- internal/transportfulfillment/adapters/http/form_load_assignment.go
- internal/transportfulfillment/adapters/http/terminate_fulfillment_participation.go
- internal/transportfulfillment/domain/payload_canonicalization.go（裁为补形状时）
- 票 15 补记下的回写行（只加行）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 说明

逐口核研判（workflow review26-face-algebra-sweep）已对这两口实测，缺陷分立在 tf-face-algebra：装载分配见 tf-face-algebra/02，终止参与见 tf-face-algebra/03、04、05。本票只做逐口裁定「现有格够不够」并把结论回写票 15 补记（随 operator-channel/39）。附录是全部决定口的逐口结论原稿。

## 为什么

之四同一笔把 `/transport-fulfillment-load-assignment-registrations` 和 `/transport-fulfillment-participation-terminations` 由字面量 `UnconfiguredIntake{}` 换成了操作者 Intake（`dc439417` 的说明原句：「……五行，由字面量 `UnconfiguredIntake{}` 换成操作者渠道……的 Intake」）。ADR-0151 决定三要求「未核完的口不开真渠道」。票 15 的逐口记录（10-13「发现二」）不含这两口，票 20 又把它们排除在外。也就是说，问题有记录（列在未定形状里），但没有处置，到现在也没人认领。

## 逐口（取证钉 `8d22729f`，只读了编排和响应形状，没有实测）

### 装载分配 `/transport-fulfillment-load-assignment-registrations`

- 现有代数：`FormLoadAssignmentHandler.Form` 按（租户 + 分配 + 版本）认重放。同一版本再投，答 `LOAD_ASSIGNMENT_VERSION_EXISTS` 并交回在册的版本——`loadAssignmentResponse` 回显分配、班期、成员、版本、分配时刻——但不比较内容。编排注释写的是「同一版本重投：交回原版本，不顶替。换对象范围要换版本号，那是变化不是重投」。
- 要裁的问题：同一版本、内容不同时，是加一个冲突格，还是现有的格就够用。
- 建议（未裁）：现有格够用，只核对。这与票 20 对建派送任务的裁定同形：版本就是身份，变化走新版本；靠回显，调用方自己就能比出差异。

### 终止参与 `/transport-fulfillment-participation-terminations`

- 现有代数：`EndFulfillmentParticipationHandler.End` 看到对象在本段已离场，就答 `PARTICIPATION_ALREADY_ENDED`，不比较命令带来的结束依据和结束时刻；`participationTerminationResponse` 只带 `outcome`、`segment`、`object` 和续办引用，不回显在册的终点。编排注释写的是「已离场的不重复结束也不改写——更正走新的判断版本，不在这里覆盖」。
- 要裁的问题：同一对象、依据不同或结束时刻不同时，是加一个冲突格，还是现有的格就够用。
- 建议（未裁）：与票 20 对关段的裁定同形——现有格够用，但答复补上回显在册的结束依据和时刻，让调用方比得出差异。

判据可以照 ADR-0151 的复核记录（<日期>，随 <T0> 落）。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决）

代 TF owner 裁，依据见上方「代决依据」。判据照 ADR-0151 复核记录（2026-10-19，随 operator-channel/39 落）的读法：「内容冲突代数」看答法，不看有没有一个叫「冲突」的格。两口的取舍都属机制，不涉租户取值。

### 装载分配 `/transport-fulfillment-load-assignment-registrations`

- **同版本重放：现有格够用，只核对。**
  - 凭 `FormLoadAssignmentHandler.Form` 按（租户、分配、版本）认重放：同一版本再投答 `LOAD_ASSIGNMENT_VERSION_EXISTS`，经 `loadAssignmentResponse` 回显在册那一版（分配、班期、成员、版本、分配时刻），不落库、不顶替。
  - 版本即身份。同一版本异内容不是本口该收的「变化」：变化与撤回要回指前版，走领域 `ReviseMembers` / `Withdraw` 的入口，不经本口重投表达。回显让调用方自己比得出差异，与票 20「裁定记录」对建派送任务的裁法同形。
  - 不补 TFC-1 形状，不加冲突格，`payload_canonicalization.go` 不动。
- **并列首版：不成立，已由 tf-face-algebra/02 修。** 本口只形成一次分配的首版。该分配已有任何版本时，对新版本号答新格 `LOAD_ASSIGNMENT_ALREADY_FORMED`（200，回显链头），不落库、不另起根；库面一分配一个首版（迁移 `transport_fulfillment/0021`）。
- **零时刻之后不足一微秒的分配时刻、版本或成员带 NUL：不成立，已由 tf-face-algebra/01 修。** 构造门按登记册精度判零，`newRequiredValue` 拒 NUL，都答 `INPUT_NOT_ACCEPTED`，不落库。
- 余项（潜伏，不挡本口）：链头是撤回版时，「分配已形成」的答复看不出分配已撤回 → tf-face-algebra/13（ready-for-agent）。撤回入口（tf-unwired-seven/05）接上之前走不到。

### 终止参与 `/transport-fulfillment-participation-terminations`

- **现有格够用，补回显；已由 tf-face-algebra/03 落。**
  - 凭 `EndFulfillmentParticipationHandler.End`：终点是链尾参与上的单向槽，已离场答 `PARTICIPATION_ALREADY_ENDED`，不落库、不发意图、不签标识。
  - 已离场之后没有第二个终点可落，冲突格给不出可续办的去处。这与票 20 对关段的理由同形，代价也同形：答复要让调用方比得出差异。
  - 现在已离场的答复带在册终点的种类、依据、时刻（`endKind` / `endBasis` / `endedAt`）。原样重放、异依据或异时刻、被下一次交接结束，这几种情形调用方都能自己分出来。
  - tf-face-algebra/03 里补 TFC-1 形状 `END_FULFILLMENT_PARTICIPATION` 的备选不取。
  - 同一张票还收了两件：并发输家先读回再作答；坏命令不论参与状态，一律答 `INPUT_NOT_ACCEPTED`。
- **与交接更正并发时终点丢失：已由 tf-face-algebra/05 修**（替代口与终止窄口都先锁住链尾行）。
- **凭实际承运商首次有效收寄入场的段按键读不回：已由 tf-face-algebra/04 修。**
- **链尾被交接更正成失效版本后，同一终止重放答 `OBJECT_NOT_IN_SEGMENT`：维持，不加格。** tf-face-algebra/03「不做」把它的去留留给了 TF owner，这里一并裁。此刻链尾的入场依据已失效，对象确实不在本段；答复如实反映在册状态，不会让调用方把没被采纳的内容读成成功，不属于 ADR-0151 复核记录说的「不算的情况」。
- 读码所见、未实测：`FulfillmentParticipation.End` 的时刻非零门仍按纳秒判，属 tf-face-algebra/01 清点出的履约段同病 → tf-face-algebra/14（in-progress，本票落时未进 main）。

### 回归用例

本票不另写代码。完成判据要的「领域、编排、真库装配三层」，逐格指到修复票已有的用例；用例名都在本分支基 `b57531cc` 上用 `grep -rn 'func <名>' --include=*_test.go` 找到了定义。修复没动的那一层，照实写无用例。「真库装配」含真库适配器用例与 `cmd/parcel-api` 的真渠道用例（生产装配加真库）。

- 装载分配 · 同版本重放（只核对，代码未动）：
  - 领域：领域层未改，故无领域用例。
  - 编排：`TestASecondFormOfTheSameVersionKeepsTheOriginal`（既有，tf-face-algebra/02 修后照过）。
  - 真库装配：真渠道 `TestTheLoadAssignmentFaceAnswersAlreadyFormedAfterAWithdrawnChain` 末段（撤回链之后同版本重放仍答 `LOAD_ASSIGNMENT_VERSION_EXISTS`，回显原版）；真库适配器 `TestALoadAssignmentAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry` 里「零点后 1µs 原样重放」那一格。
- 装载分配 · 并列首版（tf-face-algebra/02）：
  - 领域：领域层未改，故无领域用例。
  - 编排：`TestAnotherVersionOfAFormedAssignmentAnswersAlreadyFormed`、`TestAWithdrawnAssignmentAnswersAlreadyFormedWithTheWithdrawal`、`TestALosingConcurrentFirstVersionAnswersAlreadyFormedWithTheWinner`、`TestAChainHeadReadFailureIsUndecided`。
  - 真库装配：真库适配器 `TestTheChainHeadIsTheVersionNoOtherCorrects`、`TestASecondFirstVersionOfOneAssignmentIsAlreadyRegistered`；真渠道 `TestTheLoadAssignmentFaceAnswersAlreadyFormedForASecondVersion`、`TestTheLoadAssignmentFaceAnswersAlreadyFormedAfterAWithdrawnChain`、`TestALosingConcurrentFirstVersionAnswersAlreadyFormedWithTheWinner`（与编排那例同名，定义在 `cmd/parcel-api`）、`TestConcurrentFirstVersionsThroughTheFaceFormExactlyOne`。
  - 另有 HTTP 适配器 `TestFormLoadAssignmentAnswersAlreadyFormedWithTheChainHead`。
- 装载分配 · 零时刻与 NUL（tf-face-algebra/01）：
  - 领域：`TestALoadAssignmentAtTheZeroInstantOnTheRegistryIsNotFormed`、`TestLoadAssignmentRevisionAndWithdrawalAtTheZeroInstantOnTheRegistryAreRefused`、`TestRequiredValuesRefuseNUL`。
  - 编排：编排层未改，故无编排用例。
  - 真库装配：真库适配器 `TestALoadAssignmentAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry`、`TestValuesWithNULAreNotAcceptedAgainstARealRegistry`；真渠道 `TestLoadAssignmentUnstorableValuesAreNotAcceptedOnTheProductionChannel`。
- 终止参与 · 回显、并发输家读回、坏命令（tf-face-algebra/03）：
  - 领域：领域层未改，故无领域用例。
  - 编排：`TestAnAlreadyEndedAnswerEchoesTheRecordedEnd`、`TestAConcurrentLoserReadsBackBeforeAnswering`、`TestAMalformedTerminationIsNotAcceptedWhateverTheParticipationState`。
  - 真库装配：真渠道 `TestATerminationReplayEchoesTheRecordedEndAndWritesNothing`、`TestATerminationAfterTheNextHandoverEndedItEchoesThatHandover`、`TestAConcurrentTerminationLoserBehindASupersedingCorrectionDoesNotClaimTheEnd`。
  - 另有 HTTP 适配器 `TestParticipationTerminationAlreadyEndedEchoesTheRecordedEnd`。
- 终止参与 · 与交接更正并发（tf-face-algebra/05）：
  - 领域、编排：都未改，故无领域用例，也无编排用例。
  - 真库装配：真库适配器 `TestSupersedeInheritsAnEndCommittedAfterTheCallerRead`、`TestSupersedeRefusesWhenTheInheritedEndPrecedesTheCorrectedStart`；真渠道 `TestAHandoverCorrectionArrivingInsideAnOpenTerminationKeepsTheEnd`、`TestATerminationCommittedBetweenACorrectionsReadAndItsSupersedeIsKept`、`TestATerminationArrivingWhileACorrectionHoldsTheTailKeepsTheEnd`。
- 终止参与 · 第三入场词读回（tf-face-algebra/04）：
  - 领域、编排：都未改，故无领域用例，也无编排用例。
  - 真库装配：真库适配器 `TestACarrierPickupEntryRoundTripsBesideOtherMembers`。没有真渠道用例，tf-face-algebra/04 完成记录「未做」如实记着。
- 终止参与 · 链尾失效后答 `OBJECT_NOT_IN_SEGMENT`（维持）：代码未动，没有专门用例。

### 越权风险点

代 TF owner 裁了这两口的代数够用（装载分配同版本重放只核对；终止参与补回显即够，链尾失效后答 `OBJECT_NOT_IN_SEGMENT` 维持），owner 复核时看今天有没有调用方靠同一版本号重投表达变化，或者非要冲突格而不是回显来分辨异依据的终止。

## 完成判据（按裁定）

- [x] 两口的逐口裁定记在本票里（由 TF owner 裁）；裁为补形状或补回显的口，补上领域、编排、真库装配三层的回归用例。
- [x] 在票 15 补记里这两口那一行下追加回写，只加行。
- [x] 改代码的部分走 Go 那条路（并行会话）。

## 完成记录（2026-10-19，会话 idp-parcel-9b 起的子代理，分支 `claude-oc5039`）

- 基线 `b57531cc`。本笔只改本票票面；「地盘」里列的 `.go` 一份没动——要改代码的格都在修复票里走完了 Go 那条路，并已进 main。
- 逐口裁定：见「裁定」，由会话代 TF owner 裁，越权风险点在该节末。终止参与裁为补回显，三层回归用例见「回归用例」：领域层两口都没改，照实写无领域用例；编排与真库装配各格都指到了已有用例。
- 票 15 补记的回写：随 operator-channel/39 落，在同分支紧随本笔的下一笔里（票 15 的补记节由 39 统一追加，本票这两口的行与回写都在那一节）。
- Go 那条路：tf-face-algebra/01、02、03、04、05 的 Status 都是 resolved，各票「进 main 记录」有推送方全量。
- 附录里的〈占位名〉逐个在 `.scratch/` 下按票题与 Status 核过（核于 `b57531cc`），对应如下。附录原稿不改，票 15 补记里已换成票号。
  - 〈装载分配并列首版〉→ tf-face-algebra/02（resolved）
  - 〈TF 构造门存得下〉→ tf-face-algebra/01（resolved）。附录里点名它的装载分配、移动事实、派送尝试，都在 01 里收；01 清点出的地盘外同病另立 tf-face-algebra/14（in-progress，未进 main），附录各行没有点名它。
  - 〈终止参与回显〉→ tf-face-algebra/03（resolved）
  - 〈终止与更正并发丢终点〉→ tf-face-algebra/05（resolved）
  - 〈第三入场词读回〉→ tf-face-algebra/04（resolved）
  - 〈有效时间链线性〉→ tf-face-algebra/06（resolved）
  - 〈交接更正冲突〉→ operator-channel/41（resolved；该票附录 F-7，tf-face-algebra/07 的裁定也说比对「用 operator-channel/41 交接更正那一半立起来的那套」）
  - 〈交接更正链线性〉→ tf-face-algebra/07（resolved）
  - 〈POD 更正代数〉→ operator-channel/41（resolved；该票附录 F-6）
  - 〈移动事实冲突格〉→ tf-face-algebra/08（resolved）
  - 〈派送发起任务引用〉→ tf-face-algebra/09（needs-triage，未修）
  - 〈建派送任务回显条件〉→ tf-face-algebra/10（needs-triage，未修）
  - 〈监管凭证存得下〉→ tf-face-algebra/11（resolved）
  - 〈集成客户端租户键〉→ operator-channel/52（resolved）
  - 附录里写成票号的 operator-channel/30、31、33、34、36 也都是 resolved。
- 证据层级：本票没有新取证；所引用例与实测都出自修复票，层级 `S`。
- 评审：纯 `.md`，推送方自审。


## 附录：逐口结论原稿

- /shipment-requests/continued-attempt-closures：只核对 · 成立（票 28 修后）。生产装配的授权映射为 nil，请求停在 UNDECIDED / AUTHORITY_UNAVAILABLE，所以重放分支今天在生产上走不到。这是潜伏锁，不算缺陷。｜- 受控关闭 ✅ **只核对**（不在本节重记）：凭 `domain.ContinuedAttemptRegister.RecordedRequest`，见上方「补记（2026-10-16，票 operator-channel/28）」与 ADR-0183「复核记录二」。本轮钉 `c68e4342` 带 DSN 复跑 `TestClosureAndReopeningThroughTheOperatorChannelAgainstARealDatabase`，照过。生产上授权映射为空，停在 `UNDECIDED / AUTHORITY_UNAVAILABLE`。
- /shipment-requests/continued-attempt-reopenings：只核对 · 成立（票 28 修后）。与受控关闭一样受授权映射为 nil 所限，生产上停在 UNDECIDED / AUTHORITY_UNAVAILABLE。｜- 受控重开 ✅ **只核对**（不在本节重记）：与受控关闭同凭 `RecordedRequest`，比对到所指关闭为止；在货主重授权门和签发标识之前就认出重放。钉 `c68e4342` 复跑既有用例，含「重开重投答已有结果」那一格，照过。
- /shipment-requests/manual-review-completions：缺陷：提交版本被服务端当前版顶替，新版本形成后重投，会把旧版本的复核签到新版本的任务上。另一路已实测，修复票为 operator-channel/34（in-progress），本路未重做。｜- 复核完成 ❌ → operator-channel/34（不在本节重记，行文见票 39 的补记草稿）。
- /shipment-requests/rejections：缺陷：主动拒绝的留痕不落库，读不回已拒绝；撞上已有决定时仍答 FORMED（201），不回显，重放还会耗掉决定标识。另一路已实测，修复票为 operator-channel/33（in-progress）与 operator-channel/36（ready-for-agent），本路未重做。｜- 主动拒绝 ❌ → operator-channel/33、operator-channel/36（不在本节重记，行文见票 39 的补记草稿）。
- /shipment-requests/authorized-dispositions：按去向分开判：
- 「交客户补充」：只核对 · 成立。
- 「拒绝」：缺陷，重放答 500。另一路已实测，修复票为 operator-channel/33、operator-channel/36。｜- 授权处置：「交客户补充」去向 ✅ **只核对**；「拒绝」去向 ❌ → operator-channel/33、operator-channel/36（不在本节重记，行文见票 39 的补记草稿）。
- /transport-fulfillment-segment-closures：只核对 · 成立，依据票 20 裁定。

残留问题：回显的 closedAt 只到秒，亚秒差异调用方比不出。由 operator-channel/31（ready-for-agent，Blocked by 30）处理。｜- 关段 ✅ **只核对**（「发现二」回写已记）：凭 `CloseFulfillmentSegmentHandler.Close`，已关答 `SEGMENT_ALREADY_CLOSED` 并回显在册关闭时刻，并发输家读回赢家（票 20「裁定记录」）。回显只到秒 → operator-channel/31。
- /transport-fulfillment-dispatch-task-registrations：只核对 · 成立（依据票 20 裁定），本次补核修正（amend）三处：
1. 答复 `dispatchTaskResponse` 不带 conditions，「回显让调用方自己比」缺这一格。真库实测：手工口 201 的答复没有 conditions，派送发起口对同一任务的回显却带 conditions。→ 〈建派送任务回显条件〉
2. 时刻回显只到秒 → operator-channel/31。
3. 本口能用派送发起的确定性引用建任何任务，占住键 → 〈派送发起任务引用〉。｜- 建派送任务 ✅ **只核对**（「发现二」回写已记），补一格 ❌ → 〈建派送任务回显条件〉。凭 `OpenDispatchTaskHandler.Open`：同一任务交回原任务，「差异调用方自己比」（票 20「裁定记录」）。但 `dispatchTaskResponse` 不带 `conditions`，这一格比不出。回显只到秒 → operator-channel/31。手工口能占用派送发起的确定性引用 → 〈派送发起任务引用〉。
- /transport-fulfillment-load-assignment-registrations：缺陷 · 已可触发。本次核验修正（amend）了原研判「代数按票 20 判据够用」的结论。

① 同一分配可以经本口另起无前版的并列首版，版本链变成森林。
- 这些首版会抢占领域「变化」要用的版本号。
- 撤回之后，本口还能起一个活跃的新根（这一条潜伏，因为撤回入口未接）。

② 分配时刻落在零时刻之后不足一微秒时，写得进、读不回：该身份此后永远答 `LOAD_ASSIGNMENT_UNDECIDED`；版本或成员带 NUL 时答 500。

同版本重放这一半的代数本身成立。｜- 装载分配 ❌ → 〈装载分配并列首版〉、〈TF 构造门存得下〉。凭 `FormLoadAssignmentHandler.Form`「同一版本重投：交回原版本」，这一半照票 20 判据够用，待 TF owner 并裁。不成立：同一分配换版本号，会另起一个无前版的首版，撤回之后仍能起活跃新根；零时刻之后不足一微秒的分配时刻首投落库，读回时被重建门拒，此后永远答 `LOAD_ASSIGNMENT_UNDECIDED`。本口在 `dc439417` 开真渠道时没有核对记录。
- /transport-fulfillment-participation-terminations：缺陷 · 已可触发：
- ① 已离场时，原样重放与同身份异内容（异依据、异时刻、被下一次交接结束）同答，且不回显在册终点，票 20「回显让调用方自己比」不成立。
- ② 并发输家在链尾被交接更正替代时不读回，直接答「已离场」，而参与其实仍在场。核验补测。
- ③ 与交接更正并发时终点丢失：终止答 201，新链尾却没有终点。核验补测。
- ④ 凭实际承运商首次有效收寄入场的段，按键读不回；段里只要有一名这样的成员，整段都停在未决。
- ⑤ 坏命令的答复随参与状态而变。

此外，链尾被更正成失效版本后，重放改答 `OBJECT_NOT_IN_SEGMENT`，已实测。｜- 终止参与 ❌ → 〈终止参与回显〉、〈终止与更正并发丢终点〉、〈第三入场词读回〉。
  - 凭 `EndFulfillmentParticipationHandler.End`：终点是链尾参与上的单向槽，`!current.Active()` 即答 `PARTICIPATION_ALREADY_ENDED`，不落库、不发意图、不签标识。
  - 不成立：
    - 已离场与异依据、异时刻、被下一次交接结束同答，且不回显在册终点（票 20 对关段的同类裁法以补回显为代价）；
    - 并发输家在链尾被替代时不读回就答已离场；
    - 与交接更正并发时终点丢失；
    - 凭实际承运商首次有效收寄入场的段读不回。
  - 本口在 `dc439417` 开真渠道时没有核对记录。
- /transport-fulfillment-effective-time-judgments：代数成立：同值重放、异值改判，秒到微秒都实测过。

缺陷 · 潜伏，有两处：
- ① 原样重放带亚微秒时，每次多签一版。operator-channel/30（in-progress）已覆盖，不另立票。
- ② 同一事实被并发判断时，版本链分叉，且不会自愈；其后再改判，只接在 `FindCurrent` 取的那一支上，另一支永远留成当前版。VE 在改判之后看到两份同时有效，却不再立冲突信号。→ 〈有效时间链线性〉

锁在事实来源：生产上没有外部承运轨迹事实的写入方。｜- 有效时间判断 ✅ 代数照「发现二」原记录成立（秒到微秒实测）；❌ → operator-channel/30（亚微秒原样重放多签一版）、〈有效时间链线性〉（同一事实并发判断时版本链分叉，其后改判不收敛，VE 不再立冲突信号）。两格都潜伏：生产上没有外部承运轨迹事实的写入方；拉取节拍票 label-channel-service-first-release/20 接上之前须修。
- /transport-fulfillment-carrier-first-effective-pickup-judgments：只核对 · 成立（票 20 补 TFC-1，票 27 修失效重放）。

- 残留：回显只到秒 → operator-channel/31。
- 关联缺陷：经本口立起的段，按键读不回。判断口实测答 201 `PICKUP_FORMED`，但带 carrierJudgmentContinuationReference，即实际承运商判断那一半读不回段 → 〈第三入场词读回〉。｜- 承运商首次有效收寄判断 ✅ **只核对**（「发现二」回写已记，票 27 修失效重放）。凭 `replayOrConflictOnTheSameBasis` 与 TFC-1 `CanonicalizeCarrierFirstEffectivePickupPayload`（业务时间按微秒）。回显只到秒 → operator-channel/31。经本口立起的段按键读不回 → 〈第三入场词读回〉。
- /node-operations/receptions：只核对 · 成立，依据为 NOC-1。｜- 收寄（NO）✅ **只核对**。凭 NOC-1：`ReceiveDeliveredUnitHandler.Handle` 按（租户、来源标识）幂等，`domain.CanonicalizeDeliveryPayload` 经 `CompareStoredDigest` 比对，同答 `EXISTING_RESULT`、异答 `SOURCE_CONFLICT`。摘要不含交付人、证据、拒收原因，沿用原判据。
- /transport-fulfillment/deliveries：只核对 · 成立，依据为 TFC-1。另有一处观察：并发输家读回赢家后不比摘要，是窄竞态，未实测，也未立票。｜- 交付首登 ✅ **只核对**。凭 TFC-1：`RegisterEffectiveDeliveryHandler.Register` 按（租户、对象、尝试）幂等，用 `CanonicalizeEffectiveDeliveryPayload` 比已存摘要，同答 `EXISTING_VERSION`、异答 `SOURCE_CONFLICT`。观察：并发输家 `DeliveryAlreadyRegistered` 读回赢家后不比摘要，未实测、未立票。
- /transport-fulfillment/delivery-proof-corrections：缺陷 · 已可触发：本口既没有重放代数，也没有冲突代数。

真库实测：
- 串行原样重投：每次都签新版、新落一行、再交一份意图。
- 与原请求重叠的重投：答 `SOURCE_NOT_ACCEPTED`，并耗掉一个版本号。
- 读在对方提交前、写在其后：版本链分叉，两版都回指同一前版。
- 意图交付失败后按续办重投：另签一版，原版本的意图永远交不出去。
- 迟到的旧更正会把当前 POD 改回去；更正时刻倒退的版本也能成为当前版。

经装配好的端点表与真库操作者册、设备册走，结果同样如此。｜- POD 更正 ❌ → 〈POD 更正代数〉。没有可凭的代数：`RegisterEffectiveDeliveryHandler.Correct` 每次现签结果版本、回指此刻的当前版，命令里既无前版也无请求键。
  - 原样重投：每次 201，再签一版、再交一份意图。
  - 与原请求重叠的重投：答 `SOURCE_NOT_ACCEPTED`，并耗掉一个版本号。
  - 读在对方提交前、写在其后：版本链分叉。
  - 意图交付失败后的重投：另签一版，原版本的意图永不交出。
  - 迟到的旧更正：把当前 POD 改回去。
  - 本口在 `ec00b795` 开真渠道，票 13「未定形状」列着 `CorrectDeliveryProof`。
- /transport-fulfillment/delivery-attempts：代数成立：秒、毫秒、微秒三种精度都实测过；任务关闭后原样重放仍答 EXISTING_RESULT。

缺陷 · 已可触发：
- 时刻带非零的亚微秒位时，原样重放与并发输家答 SOURCE_CONFLICT；计划窗两端落在同一微秒内时答 500。这两处已由 operator-channel/30（in-progress）覆盖，不另立票。
- 证据引用带 NUL 时答 500 → 〈TF 构造门存得下〉。｜- 派送尝试 ✅ 代数成立（秒、毫秒、微秒实测）。凭 `RecordDeliveryAttemptHandler.Handle` → `answerFromExisting` / `deliveryAttemptContent`：两侧都从领域记录现算，内容相同答 `EXISTING_RESULT` 并回显在册，不同答 `SOURCE_CONFLICT`，比对锚覆盖命令每一格。❌ → operator-channel/30（亚微秒原样重放答冲突、同一微秒计划窗答 500）；〈TF 构造门存得下〉（证据带 NUL 答 500）。本口在 `ec00b795` 开真渠道，票 13「未定形状」列着 `RecordDeliveryAttempt`。
- /transport-fulfillment/handovers：串行只核对 · 成立，依据为 TFC-1。

缺陷 · 已可触发（核验补测）：并发首登时，同版本异内容的输家答 EXISTING_VERSION，不答 SOURCE_CONFLICT。真库并发实测每一对都如此。→ 〈交接更正冲突〉随 `commit` 一并收住。

关联缺陷：对象凭实际承运商首次有效收寄入场后，下一次交接整笔回滚，答 500。→ 〈第三入场词读回〉｜- 交接首登 ✅ 串行 **只核对**。凭 TFC-1：`RegisterTransportHandoverHandler.Register` 先读后比 `CompareStoredDigest`，异内容答 `SOURCE_CONFLICT`。❌ → 〈交接更正冲突〉：并发输家经 `commit` 读回赢家后不比摘要，同版本异内容答 `EXISTING_VERSION`（真库并发实测，各对都如此）。
- /transport-fulfillment/handover-corrections：缺陷 · 已可触发，原判维持。

同一 newVersion 下，以下几种情况一律答 `EXISTING_VERSION`，答复与原样重放逐字相同：换证据、换规则、换更正时刻、改判拒收、换前版，以及撞上同对象同范围另一份首登的版本号。

候选修复已在 overlay 上试算：按新版本键先读后比，并让 `commit` 的并发输家也比，以上各格转为 `SOURCE_CONFLICT`，application 包既有用例照过。

邻接缺陷：同一前版可以被更正成两个新版本，`SummarizeHandovers` 会把一个对象数两次。｜- 交接更正 ❌ → 〈交接更正冲突〉、〈交接更正链线性〉。
  - 票 13「已定形」与同票阻断修复记录（「交接更正口只走版本键、不比摘要，未动」）互相矛盾，以后者为实：`RegisterTransportHandoverHandler.Correct` 算了 `CanonicalizeHandoverCorrectionPayload` 并落库，但撞键后从不比。
  - 同一 newVersion 下换证据、换规则、换更正时刻、改判、换前版或撞上另一份首登，都答 `EXISTING_VERSION`。
  - 同一前版还能被更正成两个新版本。
- /transport-fulfillment/offsite-pickups：只核对 · 成立，依据为 TFC-1。另有一处观察：并发输家读回赢家后不比摘要，属窄竞态，未实测，未立票。｜- 场外揽收首登 ✅ **只核对**。凭 TFC-1：用 `CanonicalizeOffsitePickupRegistrationPayload` 比已存摘要，同答 `EXISTING_VERSION`、异答 `SOURCE_CONFLICT`。观察：并发输家 `OffsitePickupAlreadyRegistered` 读回赢家后不比摘要，未实测，未立票。
- /transport-fulfillment/offsite-pickup-corrections：只核对 · 成立，依据为 TFC-1，首登与更正共用同一内容判据。｜- 场外揽收更正 ✅ **只核对**。凭 TFC-1，与首登用同一内容判据：指名的前版已不是当前版时，与当前版的已存摘要比，同答 `EXISTING_VERSION`、异答 `SOURCE_CONFLICT`；比对锚在当前版（「要改请对当前版提更正」，属设计）。
- /transport-fulfillment/offsite-pickup-attempts：只核对 · 成立，依据为 TFC-1。摘要的字段集沿用旧判据。｜- 场外揽收执行 ✅ **只核对**。凭 TFC-1：`PerformOffsitePickupHandler.Handle` 按（租户、来源标识）幂等，用 `CanonicalizeOffsitePickupPayload` 比已存摘要，同答已有结果，异答 `PickupSourceConflict`。摘要不含执行人、地点、计划窗、证据、改约来源，沿用原判据。
- /transport-fulfillment/movement-facts：缺陷 · 已可触发，判据待 TF owner 确认。

同身份异内容一律答 `MOVEMENT_FACT_VERSION_EXISTS`，新内容静默丢失。核验另外补测了以下几格，结果同样如此：
- 另一台在册设备、另一作业范围上的另一名操作者，带同一（事实、版本）来；
- 赢家之外的自然竞态输家（90 份）。

「冲突格有可续办的去处」这一说法修正（amend）：本口没有更正口，载荷带前版格答 400；换版本号登出的是互不回指的并列版本。缺冲突格的依据因此改为作业事实族的调用方性质，不再凭「有去处」。

另有两处缺陷：
- 零时刻之后不足一微秒的业务时间写得进、读不回（本路补测）；
- 地点带 NUL 时答 500。｜- 移动事实 ❌ → 〈移动事实冲突格〉、〈TF 构造门存得下〉。
  - 可凭的只有身份那一半：`RecordMovementFactHandler.Record` 按（租户、事实、版本）查到即答 `MOVEMENT_FACT_VERSION_EXISTS`，交回原版（ADR-0023「同一身份重复到达返回原结果」）。
  - 不成立：同身份异内容（种类、地点、班次、业务时间、放行）也答已有版本，新内容静默丢失；另一台设备、另一作业范围带同一（事实、版本）来，同样被吞（实测）。作业事实族调用方是离线队列，「回显自比」不适用。
  - 另：零时刻之后不足一微秒的业务时间写得进、读不回；地点带 NUL 答 500。
  - 本口在 `ec00b795` 开真渠道，票 13「未定形状」列着 `RecordMovementFact`。
- /transport-fulfillment-delivery-dispatch-triggers：主路径只核对 · 成立：原样重放、异 occurredAt、并发首拍都答「已在册」并回显在册任务。

缺陷 · 已可触发（边角）：已在册只按键认，但键不是忠实身份。
- 键可被手工建任务口占用。
- 段、对象、入场依据以斜杠直拼，拼接不单射。对象取生产形状（不含斜杠）时，只靠段引用与交接版本也能撞键；同一对象在第二个派送段上撞上第一段那项任务时，种类与对象集都对得上（核验补测）。
- 被撞的对象首拍即答「已形成」，实际却没有派送任务。

核验修正（amend）：
- 原判「永远形成不了」改为「入场依据不变期间形成不了」：交接更正换了入场依据后可以脱出。
- 只核对在册任务的种类与对象集不足以拦住撞键。

生产缝下，要三个所有者都给出答复才走得到撞键。

另：在册要等重拉所有者之后才认，所有者改答或参与离场时，原样重放不回显在册任务。｜- 派送发起 ✅ 主路径 **只核对**：凭 ADR-0114 决定二的确定性任务引用，加 `OpenDispatchTaskHandler.Open` 的已在册支。原样重放与异 `occurredAt` 答 `DISPATCH_TASK_ALREADY_FORMED` 并回显在册；并发首拍读回赢家。
  ❌ → 〈派送发起任务引用〉：
  - 已在册只按键认，键既不独占（手工建任务口能占用），也不单射（段、对象、入场依据以斜杠直拼）。被撞的对象在入场依据不变期间答「已形成」，却没有派送任务。
  - 在册要等重拉所有者之后才认，所有者改答时原样重放不回显。
  - 本口在 `ec00b795` 开真渠道，票 13「未定形状」列着 `TriggerDeliveryDispatch`。
- /customs/external-results：只核对 · 成立，依据为 CCC-1。

租户前提成立：`externalResultDocument` 没有租户格，租户只取认证结果，所以大小写变体的租户键都是未知字段，答 400（读码）。｜- 外部结果（CC）✅ **只核对**。凭 CCC-1：`ReceiveExternalResultHandler.Handle` 按（租户、来源标识）幂等，用 `CanonicalizeExternalResultPayload` 比已存摘要，相同答 `EXISTING_RESULT`、不同答 `SOURCE_CONFLICT`。租户只取认证结果：`externalResultDocument` 没有租户格，大小写变体都按未知字段答 400（读码）。摘要不含角色，沿用原判据。
- /customs-regulatory-credential-registrations：缺陷 · 已可触发，原判维持，核验补充了下面几项修正：

① 触发重放误判的是「微秒以下有非零位」，与小数位数无关。七位小数而第七位为零（.NET 往返格式写日期），或九位小数而末三位为零，原样重放都答 EXISTING。

② 零时刻之后不足一微秒的有效期：首登 201，之后原样重投答 500，点读读不回，而且该租户整张凭证列面答 500。

③ 以下输入都答 500：额度超出 integer、引用带 NUL、有效期两端落在同一微秒内。

④ 租户键大小写变体：凡在字典序上排在 `tenantId` 之后的写法（如 `tenantid`、`tenantiD`）都能改写认证租户，导致：
- 跨租户写入；
- 跨租户探询在册内容：猜中答 EXISTING，猜错额度答 CONTENT_CONFLICT；
- 给他租户植入毒行，使其列面答 500；
- 抢注：被植入的租户日后自登同一身份，答 CONTENT_CONFLICT。｜- 监管凭证登记 ❌ → 〈监管凭证存得下〉、〈集成客户端租户键〉。
  - 凭 CCC-1 REGISTER_CREDENTIAL（`b4d28563`）：撞键后 `sameCredentialContent` 两侧现算，相同答 `EXISTING`、不同答 `CONTENT_CONFLICT`；整秒与微秒实测成立。
  - 不成立：
    - 有效期带非零亚微秒位时，原样重放答 `CONTENT_CONFLICT`；
    - 零时刻之后不足一微秒的有效期写得进、读不回，并让该租户凭证列面答 500；
    - 同一微秒内的有效期、越出 `integer` 的额度、引用带 NUL，都答 500；
    - 载荷带 `tenantid` 一类大小写变体，即改写认证租户。
  - 本口开真渠道时未定形，随票 13（`b4d28563`）补形。
- /settlement-external-funds-fact-registrations：代数部分：只核对 · 成立，依据为 SAC-1。

身份前提部分：缺陷 · 已可触发（本路补测，amend）。`snapshotWithAuthenticatedTenant` 与 CC 那一份同形，载荷带 `tenantid` 或 `tenantiD` 时，命令租户被改写成载荷自报的租户。已在 Intake 层实测；落库未实测，但 CC 同形一路已经真库实测跨租户落库。→ 〈集成客户端租户键〉｜- 资金事实采用（SA）✅ 代数 **只核对**：凭 SAC-1，`MapExternalFundsHandler.AdoptFact` 按（租户、事实、版本）比 `adoptDigest`，相同答 `EXISTING_FUNDS_FACT`，不同答 `FUNDS_FACT_CONFLICT`。❌ 身份前提 → 〈集成客户端租户键〉：`snapshotWithAuthenticatedTenant` 只拒精确键 `tenantId`，载荷带 `tenantid` 一类变体会改写命令租户（Intake 层实测）。
- /settlement-external-funds-fact-correction-registrations：代数：只核对 · 成立（SAC-1）。

身份前提：缺陷 · 已可触发（本路补测，amend）。更正口也经 `snapshotWithAuthenticatedTenant` 进来；载荷带 `tenantid` 时，命令租户被改写为 SYN-TENANT-OTHER（Intake 层实测）。→ 〈集成客户端租户键〉｜- 资金事实更正（SA）✅ 代数 **只核对**：凭 SAC-1，`MapExternalFundsHandler.CorrectFact` 先按新版本比 `correctDigest`，再查链头。❌ 身份前提 → 〈集成客户端租户键〉：同采用口。
