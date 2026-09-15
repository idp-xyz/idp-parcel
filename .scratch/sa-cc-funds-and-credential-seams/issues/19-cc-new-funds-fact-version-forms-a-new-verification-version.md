# 资金事实新版本到达后没有编排接着做：登记册看得见 v2 回指 v1，UC-CC-009「形成新核对版本并保留原覆盖判断」在 CC 侧仍无入口

Category: enhancement
Status: resolved——**2026-09-15 12:1x 通道 2（接手）**（task-d230f095-32d6-4183-b102-2483fd689176；分支 `mcp2-sacc22-19` 基 `1e74aaaf`，代码 tip `a0cb6fef`；原作者会话已换，完成记录由接手方按 diff 与代码逐条对判据代写；全文见「完成记录」）。此前 in-progress——**2026-09-15 10:2x 通道 2 认领**（与 [22](22-cc-duty-verification-procedure-is-caller-asserted-and-not-recorded.md) 同一 task `f3329ba3`、同一作者、同一分支 `mcp2-sacc22-19` 基 `3a21dab7`；22 先落、本票在其上，不等 22 进 main；迁移序号本票 `0023`）；此前 ready-for-agent——**2026-09-14 22:1x 通道 1 按用户「你是业务和系统专家，自决」代裁（CC owner 口径），两条「要裁的」写入下方「裁决」节**：(a′) 形成新核对版本——覆盖轴按 UC-CC-009 原句「保留原覆盖判断」承前版、差额与有效性两轴显式 `PENDING`、依据与程序承前版、资金版本取新到的那一版；触发条件是「该事实已有既往核对版本」、与 `corrects` 在不在无关；资金版本**折进指纹 + 记录列**（照 [22](22-cc-duty-verification-procedure-is-caller-asserted-and-not-recorded.md) 裁决 2 之形）。**Blocked by 22（同一作者同一分支，22 先落）**。此前 draft——2026-09-14 14:0x 通道 1 立票（按 sa-cc/13 裁决 2「触发重核对不在本票……推送方据此立票」，形取 13 完成记录「后继票的形」）。只写票面未动代码；取证锚 main `0bd86d42`（sa-cc/13 重放 tip）
Blocked by: [22](22-cc-duty-verification-procedure-is-caller-asserted-and-not-recorded.md)（指纹与记录先加程序维，本票在其上加资金版本维；同一作者同一分支两笔即可，不必等 22 进 main）。此前：无（[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) 已进 main：`ExternalFundsFactRegister.ListFundsFactVersions` 列得出全部版本与回指）；要裁的已裁，见「裁决」

## 缺口（取证于 `0bd86d42`，逐符号名）

- 13 落地后：更正版本 v2 经 `settlement-accounting.external-funds-fact.adopted` 信封到 CC，`ReceiveFundsFact` 答`已接收`、`external_funds_fact_version` 落第二行回指 v1。到此为止——`external_funds_fact_consumer.go` 头注写的是「触发重核对归后继票，今天登记册上看得见、还没有编排接着做」。
- `VerifyPayment` 的三轴（覆盖 / 差额 / 有效性）与关联依据由调用方交（`VerifyDutyPaymentCommand`），调用方今天不在 `parcel-dispatch` 进程里（sa-cc/03 票面红线原句）；新版本到达时没有调用方在场，编排不算三轴。
- `DutyVerificationStore` 今天只有按幂等键的 `FindVerification`，没有「按（租户、资金事实）列全部核对版本」的读口——新版本到达时连「这条事实有没有过核对」都问不出来。
- `LoadFundsFact` 交回「最近接收的那一版」是 13 端口头注写明的临时口径：核对命令上没有版本，`duty_payment_verification` 也没有 `funds_version` 列——核对引用的是事实身份，不是事实的哪一版。

## 语言从哪里来

- CC `CONTEXT.md`「税费付款核对」：「部分付款、超额付款、错误范围、错误币种、重复付款、资金退回和付款撤销都必须保留原事实并形成新的核对判断」；集成规则「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态」。
- UC-CC-009 一致性节：「外部资金事实迟到、更正、资金退回或付款撤销时，形成新核对版本并保留原覆盖判断；不删除原付款、不按最后到达覆盖」。

## 做法（待裁后写实）

1. **触发落点**：CC 内部，`ReceiveFundsFact` 答`已接收`且该事实已有既往核对版本时——触发在消费侧适配器 `ReceiveOnAdoptedFundsFactAdapter` 之后一格另起一只编排（消费侧适配器仍只译不判，sa-cc/03 做法 3），不塞进 `ReceiveFundsFact`。
2. **要读哪几口**：`ExternalFundsFactRegister.ListFundsFactVersions`（新版内容与回指）、`DutyVerificationStore` 新增「按（租户、资金事实）列全部核对版本」读口、`DutyCollaborationStore`（协作事项仍在）、`PayerRequirementRuleView`（付款人维照 sa-cc/12 三停格）。
3. **核对按版本读**：`VerifyDutyPaymentCommand` 加 `FundsVersion`，`duty_payment_verification` 加 `funds_version` 列（新迁移，`0016` 不改），`LoadFundsFact`「最近接收」口径退役。
4. **交接不变**：新核对版本形成后走 [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 的 `DutyPaymentVerificationHandoff`（每版一封）交 SA；SA 侧采用（sa-cc/09 那族）按版本收。

## 红线

- 不按到达顺序覆盖：原核对版本一行不动（UC-CC-009 原句）。
- 三轴与关联依据不由编排猜——真实关联规则属实例半边，一行不预填。
- 消费者与消费侧适配器只译不判。

## 完成判据（待裁后写实）

1. 应用层：v1 已核对，v2 到达 → 形成新核对版本（形随裁决 1），原核对版本仍在、内容不变；无既往核对的事实新版本到达 → 不形成、不报错。
2. 真库：新迁移往返；`0016` / `0021` 一字未动；`cmd/parcel-dispatch` 真库装配用例在 13 那一格之后再扩一格（v2 到达 → 新核对版本 / 待重核对事项落行）。
3. `LoadFundsFact`「最近接收」的临时口径退役，13 端口头注那句随之改口。

## 地盘

`internal/customscompliance/{ports,application,adapters/postgres,adapters/settlementaccounting}`、`migrations/customs_compliance/`（新序号）、`cmd/parcel-dispatch/assemble_test.go`（共享文件，动前占号）。SA 侧不动。

## 要裁的

1. **新核对版本的三轴与关联依据从哪来**——归 CC owner。(a) 复用前版三轴与依据、有效性轴标 `PENDING`，形成一版「待人判」的核对；(b) 不形成核对，只登一条「待重核对」事项（新表或核对表一格）交人 / 交规则。两条路都不让编排猜三轴；差别在「形成了一版核对」还是「登了一条待办」。
2. 触发要不要区分 `corrects` 在不在（首版到达从不触发；有回指才可能有既往核对）——可由做法 1「已有既往核对版本」一条覆盖，裁时确认。

## 裁决（2026-09-14 22:1x 通道 1 代裁，CC owner 口径；依据是通道 4 21:2x 取证条，钉 `bccb60a1`）

1. **要裁的 1——(a′) 形成新核对版本，轴按「承前 / 显式待判」分开取，不猜。** 新版本的七样：**覆盖轴承前版**（UC-CC-009 原句「形成新核对版本并保留原覆盖判断」——那句里被保留的正是覆盖判断，不是编排猜的）；**差额轴 `PENDING`**（金额可能变了，`DeltaPending` 是既有格）；**有效性轴 `PENDING`**（`FundsFactPending` 是既有格：前版核对所依的事实已被新版本取代，有效性待人重判）；**依据 `Basis` 承前版**（「凭什么把这笔资金关联到这版税费」说的是事实身份与税费的关联，事实换版本不换身份，关联不变）；**程序承前版**（22 落地后是记录列）；**资金版本取新到的那一版**（本票加的维）；`verifiedAt` 取库时钟。指纹因差额 / 有效性 / 资金版本变而不同 → 新行；前版一字不动（UC「不删除原付款、不按最后到达覆盖」）。**为什么不选 (b)**：(b) 让前版继续当「当前」，放行门禁会在一份**对着已被取代的事实**做出的核对上放行——与 CONTEXT 生命周期「资金退回、付款撤销或外部资金事实更正只作为**重新核对**的来源事实」和 UC「形成新核对版本」字面相抵；且 (b) 要在 `duty_payment_collaboration` 造第三种 `kind`、改 CHECK 与键，动的是另一个词条。**取证量到的后果照单接受**：`PENDING` 版成为 `CurrentDutyVerificationView` 的当前一版后，`VerifyReleaseGate` 那一道答**未决**（`gateUndecided(DutyVerificationPending)`）而不是未满足——这正是要的效果：事实变了、人没重核之前门禁不该放也不该判失败；人重核走既有 `VerifyPayment`（带新资金版本、断言三轴）再成一版，门禁随之。**红线「三轴不由编排猜」守住**：承前的那一轴是 UC 明令保留的，另两轴写的是「未判」不是「判了」。
2. **要裁的 2——触发条件只看「该事实已有既往核对版本」，不看 `corrects`。** 取证量得两者互不蕴含（不带回指的新版本可以在既往核对之后落地；带回指的首版也可能在任何核对之前到）。所以：`ReceiveFundsFact` 对一个**新**版本答 `FundsFactReceived` 之后（同版重放答`已登记`不触发），编排问新读口「这条事实有没有既往核对版本」，有 → 对**每一条**既往核对谱系（按（税费、范围、程序）分组，取各组最近一版）各形成一版 (a′)，无 → 什么都不做、不报错（完成判据 1 第二句）。`corrects` 照登、不校验前版已到（13 判断项 ③ 不变）。
3. **做法写实**（对照上面「待裁后写实」四条）：(1) 触发落点照做法 1——消费侧适配器 `ReceiveOnAdoptedFundsFactAdapter` 仍只译不判；它在 `ReceiveFundsFact` 答`已接收`后调一只**新的应用层编排**（名字作者定，如 `RederiveDutyVerificationsOnFundsFactVersion`），与接收**同一事务**（版本行、新核对版本、交接意图三者同生同灭）。(2) 读口：`ExternalFundsFactRegister` 加 `LoadFundsFactVersion(ctx, tenant, fact, version)`（按版本读，供 `VerifyPayment` 与编排用）；`DutyVerificationStore` 加「按（租户、资金事实）列全部核对版本」；`DutyCollaborationStore.FindCollaboration` 与 `PayerRequirementRuleView` 照 12 三停格读（付款人维按新版本的付款人 + 前版程序的规则重判：要求而新版本未提供 → 该谱系**不形成新版本、登一条未决 reason 交人**——这是 (a′) 唯一不形成的分支，完成记录写明）。(3) `VerifyDutyPaymentCommand` 加 `FundsVersion`（必填；`LoadFundsFactVersion` 找不到 → `未受理`）；`duty_payment_verification` 加 `funds_version` 列（新迁移 `customs_compliance/0023`，`0016` / `0021` / `0022` 不改；新增一条外键到 `external_funds_fact_version (tenant_id, fact_ref, version)`，既有那条到身份表的外键不动）；`verificationDigest` 在 22 的顺序之后追 `FundsVersion`；`LoadFundsFact`「最近接收」口径退役——端口方法删或改名为按版本读，13 端口头注那句与 26 反序格最后一条断言随之改口（22:0x Comments 已预告）。(4) 交接不变：每一新版本走 [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 的 `DutyPaymentVerificationHandoff`（信封 ID 含指纹，自然一版一封）；SA 侧 sa-cc/09 采用照收（`AdoptDutyPaymentVerification` 只存引用，不读三轴），SA 零改动。
4. **完成判据写实**：(1) 应用层：v1 已核对（谱系 A），v2 到达 → 谱系 A 形成新版本（覆盖承前、差额 / 有效性 `PENDING`、依据 / 程序承前、资金版本 = v2），前版内容零 diff、`FindVerification(前版键)` 仍命中；两条谱系各成一版；无既往核对的事实新版本到达 → 不形成、不报错；同版本重放 → 不触发；付款人维要求而 v2 未提供 → 该谱系不形成、未决 reason 点名。(2) 真库：`0023` 往返；`0016` / `0021` / `0022` 零 diff；`cmd/parcel-dispatch` 真库装配用例在 13 那一格之后扩一格（v2 到达 → 新核对版本一行 + 交接意图一封）。(3) `LoadFundsFact`「最近接收」退役：`git grep -n '最近接收' -- internal/customscompliance/` 只剩历史注释或零命中；26 反序格断言改为按版本读。(4) 放行门禁：`VerifyReleaseGate` 对 `PENDING` 当前版答未决——既有用例若断言别的，以本裁决改口并写进判断项。(5) SA 目录零 diff。
5. **能力边界**：裁的是结构（形成 vs 登待办、各轴从哪取、触发看什么、版本维怎么进键）；具体不变式（`DutyPaymentVerification` 构造门对 `PENDING` 组合的接受、`CurrentDutyVerificationView` 排序在同秒两版时的取舍、`handOffVerification` 的事务边界）归作者按代码定并写进判断项。读过：本票与 22 全文、通道 4 两份取证、13 完成记录「后继票的形」、UC-CC-009 两句与 CONTEXT 四句（经取证引文）；**没读**：`duty_release.go` / `reconcile_duty_payment.go` / `verify_release_gate.go` / `duty_payment_gate_rule.go` 正文、SA 采用编排本体、任何测试断言。作者量到与代码不符，以代码为准并写进判断项，不回头等我。

## 完成记录（2026-09-15 通道 2 接手代写，钉 `a0cb6fef`）

原作者（上一代通道 2 会话）两笔已推：`6ab8e41c` feat（按 (a′) 形成新核对版本、核对按版本读、资金版本进记录与指纹、迁移 `0023`）与 `a0cb6fef` test（`cmd/parcel-dispatch` 判据 (2) 那一格）；最后一句是「641 PASS 转入提交」，票面完成记录没写。本记录由接手方按 `git diff 1e74aaaf a0cb6fef`（30 件）与代码逐条对判据代写，**不改代码**；每条点到符号名与用例名，判断项写代码实际怎么做的。Blocked by [22](22-cc-duty-verification-procedure-is-caller-asserted-and-not-recorded.md) 已解除：22 在基 `1e74aaaf` 之前进 main，本分支 rebase 后只剩 19 的两笔。

### 逐条对裁决 4「完成判据写实」(1)–(5)

**(1) 应用层**——`internal/customscompliance/application/rederive_duty_verifications_test.go`（替身照真库代数）：

- v1 已核对（谱系 A），v2 到达 → 谱系 A 形成新版本：覆盖承前、差额 / 有效性 `PENDING`、依据 / 程序承前、资金版本 = v2、核对时刻取时钟；前版内容零 diff、`FindVerification(前版键)` 仍命中；新旧指纹不同；交接一版一封、意图认领新落册那一版 → `TestANewFundsFactVersionFormsAPendingVerificationVersionOnEachLineage`。
- 两条谱系各成一版，且各承**自己谱系最近一版**（谱系 A 在 v1 上改判过一次、覆盖 `NONE`、时刻晚一小时，新版本承的是改判后那版不是首版）→ `TestEveryLineageOfTheFactGetsItsOwnNewVersionCarryingItsLatestCoverage`。
- 无既往核对的事实新版本到达 → 零条谱系、`已重派`、册上不动、不交信封 → `TestAFactThatWasNeverVerifiedGetsNothingRederived`。
- 同版本重放 → 不触发：触发判在消费侧适配器 `ReceiveOnAdoptedFundsFactAdapter.HandleAdoptedExternalFundsFact`（`ReceiveFundsFact` 答非 `FundsFactReceived` 即返回，不调重派）；`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go` `TestANewVersionArrivingThroughTheAdapterRederivesTheLineageOnceAndReplayDoesNot`——同一封 v2 重投后核对行数与信封数不变。
- 付款人维要求而 v2 未提供 → 该谱系不形成、点名 `PayerRequiredNotProvided`、前版仍在、不交新封、整笔仍 `已重派` → `TestALineageWhoseNewVersionLacksARequiredPayerStaysPendingByName`；适配器侧 `TestRederivationUndecidedSplitsBusinessPendingFromDependencyFailure` 证业务未决入账不重投、依赖故障折成 `ErrDutyVerificationRederivationUndecided` 且与 `ErrFundsFactReceiveUndecided` 不混。
- 依赖故障（核对册 / 规则读口不可用）整笔未决并指名、不落行；命令缺格 `未受理` → `TestARederivationStopsOnDependencyFailureAndRefusesBlankCommands`。
- 核对按版本读（做法 3）：`FundsVersion` 空白 `未受理`；命令指未接收的 v9 → `FundsFactNotReceived`（别的版本在册不顶替）；同三轴同依据同程序只换资金版本 → 另一版、指纹不同；同版本重核 `已存在` → `reconcile_duty_payment_test.go` `TestAVerificationReadsThePrerequisiteByTheVersionItNames`。
- 领域构造门：资金版本必填、空白立不起核对 → `internal/customscompliance/domain/duty_release_test.go` `TestDutyVerificationRecordsWhichFundsFactVersionItJudged`；译装处必填 → `cmd/parcel-customs-register/translate_test.go` `TestCommandForRequiresTheFundsVersionOnAVerification`。

**(2) 真库**：

- `0023` 往返 → `internal/customscompliance/adapters/postgres/duty_payment_reconciliation_test.go` `TestVerificationsRoundTripTheFundsFactVersionTheyJudgedAndListByFundsFact`：写口落 `funds_version`、`FindVerification` 读回同一版；`ListVerificationsByFundsFact` 按（租户、资金事实）列全部版本、核对时刻升序、同一时刻按指纹字典序，别的事实与别的租户不可见、没核对过的事实答空、空引用报错；库内 `0023` 外键 `duty_payment_verification_funds_version_received` 拒「引用没接收过的那一版」、CHECK `duty_payment_verification_funds_version_not_blank` 拒空白（旁路 SQL 直插）。同文件 `TestExternalFundsFactsRoundTripByReference` 加「事实在册而版本不在 → found=false」「空白版本报错」两格。
- `0016` / `0021` / `0022` 零 diff → `git diff --stat 1e74aaaf a0cb6fef -- migrations/customs_compliance/0016* migrations/customs_compliance/0021* migrations/customs_compliance/0022*` 输出为空；`git diff --stat 1e74aaaf a0cb6fef -- migrations/` 只列新增的 `0023_duty_payment_verification_funds_version.sql`。
- `cmd/parcel-dispatch` 真库装配在 13 那一格之后扩一格 → `cmd/parcel-dispatch/assemble_test.go` `TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable` 正例第三格：先经真登记册铺协作事项 + 付款人规则「要求」，按 v1 走真 `VerifyPayment`（真核对册 + 真 Outbox 交接，断言 `DutyVerificationFormed` **且** `HandoffReference()` 为空），一拍发出 v1 那一封；v2 经 `external-funds-fact.adopted` 信封到达同一拍、同一事务 → `ListVerificationsByFundsFact` 两版：前版一字不动（资金版本 v1、`NO_DELTA`、`VALID`），新版是 (a′)（`COVERED` 承前、`PENDING` / `PENDING`、程序与依据承前、资金版本 v2、指纹不同）；下一拍发出新版本那一封，SA `DutyPaymentVerificationAdoptions.FindByKey` 按（范围、税费、资金、指纹）找到采用行——sa-cc/09 那族照收、SA 零改动。

**(3) `LoadFundsFact`「最近接收」退役**：端口方法改名为 `ExternalFundsFactRegister.LoadFundsFactVersion(ctx, tenant, fact, version)`，postgres 实现按 `(tenant_id, fact_ref, version)` 点读、不再 `ORDER BY received_at DESC … LIMIT 1`；`git grep -n '最近接收' -- internal/customscompliance/` 的命中（`ports/ports.go` 端口头注、`adapters/postgres/duty_payment_reconciliation.go` 与其测试、`application/reconcile_duty_payment.go` 与其测试、`adapters/registrationjson/translate.go`）全部是「自票 sa-cc/19 起退役」「不取『最近接收』顶替」一类的历史或反面注释，无一处是活语义。26 反序格改名 `TestFundsFactVersionsArrivingOutOfOrderListByReceiptAndLoadByVersion`，最后一条断言改为两版各按版本点读、不受到达顺序影响（22:0x Comments 预告的那一处）。

**(4) 放行门禁对 `PENDING` 当前版答未决** → `internal/customscompliance/application/verify_release_gate_test.go` `TestTheDutyPaymentGateStopsHonestlyInsteadOfAnsweringUnmet` 新增子格「新版本到达后待重核的那一版」（`COVERED` / `PENDING` / `PENDING` 为当前版）→ `DutyVerificationPending`，走 `verify_release_gate.go` 的 `gateUndecided(DutyVerificationPending)` 分支。既有子格无一条断言别的，不需改口。

**(5) SA 目录零 diff** → `git diff --stat 1e74aaaf a0cb6fef -- internal/settlementaccounting` 输出为空。

### 逐条对裁决 1–3

**裁决 1——七样各从哪取**（`internal/customscompliance/application/rederive_duty_verifications.go` `RederiveDutyVerificationsOnFundsFactVersion` 组的 `derived VerifyDutyPaymentCommand`）：覆盖轴 ← `latest.Verification.Coverage()`；差额轴 ← `domain.DeltaPending`；有效性轴 ← `domain.FundsFactPending`；依据 ← `latest.Basis`（`ports.DutyVerificationRecord.Basis`）；程序 ← `latest.Verification.Procedure()`；资金版本 ← `command.Version`（信封所指新版本）；核对时刻 ← 走 `VerifyPayment` 取 `handler.deps.Clock.Now()`。形成不另起一条路：`derived` 交既有 `VerifyPayment`，两道前置、付款人三停格、指纹、落册、交接与人核同一条路。指纹 `verificationDigest` 顺序 `Coverage、Delta、Validity、Basis、Procedure、FundsVersion`，两轴 `PENDING` 与资金版本任一变即换指纹；前版一字不动由 `SaveVerification` 的 `ON CONFLICT DO NOTHING` 与指纹不同共同保证。为什么不是猜：文件头注逐样写明来源（覆盖轴 UC 明令保留、两轴写的是「未判」、依据与程序是事实身份 / 记录依据维不随版本变）。

**裁决 2——触发只看「已有既往核对版本」**：`ReceiveOnAdoptedFundsFactAdapter.HandleAdoptedExternalFundsFact` 在 `receiveConsumption(result)` 之后 `result.Outcome() != FundsFactReceived → return nil`（`已存在` / `内容冲突` / `未受理` 都不触发），才调 `RederiveDutyVerificationsOnFundsFactVersion`；编排问 `DutyVerificationStore.ListVerificationsByFundsFact(tenant, funds)`，`latestVerificationPerLineage` 按 `lineageKey{duty, scope, procedure}` 分组、只在 `VerifiedAt().After()` 严格更晚时换人；空切片 → 零条谱系、`已重派`。`corrects` 不参与判断：`ReceiveFundsFact` 对回指仍只查「回指自己」，照登不校验前版已到（13 判断项 ③ 不变，diff 里该段只改注释）。

**裁决 3——做法 (1)–(4) 各落在哪**：

- (1) 触发落点：`adapters/settlementaccounting/receive_on_adopted_funds_fact.go` `FundsFactReceiver` 接口加 `RederiveDutyVerificationsOnFundsFactVersion`（真实装配仍是同一只 `ccapplication.DutyPaymentReconciliationHandler`）；适配器只译不判——三轴从哪来、形成几版全在编排。同一事务：`internal/platform/inboxconsume/consume.go` 的门在 `transactor.WithinTransaction` 里调处理方，版本行（`RegisterFundsFact`）、新核对版本（`SaveVerification`）、交接意图（`outboxintent.EnqueueOnce`）三处都走 `RequireExecutor`，同生同灭。结果落账：`rederivationConsumption` 对 `已重派` 入账、`未决` 折成新哨兵 `ErrDutyVerificationRederivationUndecided`（`cmd/parcel-dispatch/assemble.go` `externalFundsFactUndecidedSentinels` 已加）、集外响亮；`DutyReconciliationReason.dependencyFailure()` 把四个 `*Unavailable` 与业务未决分开——前者整笔重投，后者谱系级入账交人。
- (2) 读口：`ports.ExternalFundsFactRegister.LoadFundsFactVersion`（由 `LoadFundsFact` 改名而来）；`ports.DutyVerificationStore.ListVerificationsByFundsFact`（新增；postgres 实现 `ORDER BY verified_at ASC, version_digest ASC`，经 `rebuildVerification` 整门重验）；`FindCollaboration` 与 `LoadPayerRequirement` 经 `VerifyPayment` 复用，付款人维按新版本的付款人 + 前版程序的规则重判，要求而未提供 → 该谱系 `PayerRequiredNotProvided`、不形成——(a′) 唯一不形成的分支。
- (3) 版本维：`VerifyDutyPaymentCommand.FundsVersion` 必填（空白 `未受理`），前置 `LoadFundsFactVersion` 找不到 → `FundsFactNotReceived`；`domain.DutyPaymentVerification` 加 `fundsVersion` 字段与 `FundsVersion()` 读口，`VerifyDutyPayment` 构造门加 `fundsVersion.valid()`；`migrations/customs_compliance/0023_duty_payment_verification_funds_version.sql`：`funds_version text NOT NULL`、CHECK 非空白、外键到 `external_funds_fact_version (tenant_id, fact_ref, version)`，既有到身份表的外键 `duty_payment_verification_funds_fact_received` 原样保留，存量守卫 `RAISE EXCEPTION`（照 0021 / 0022 之形），主键不动；`verificationDigest` 末尾追 `FundsVersion`；`rebuildVerification` 加 `fundsVersion` 入参，四处读回（`FindVerification` / `ListVerificationsByFundsFact` / `LoadCurrentDutyVerification` / `DutyReconciliationCatalogue.ListDutyVerifications`）都带回；HTTP `dutyVerificationBody` 加 `fundsVersion`、`registrationjson.DutyPaymentVerificationFromJSON` 必填 `fundsVersion`、admin-web `DutyVerificationRecord.fundsVersion` 与 `duty-payment-verification` 快照提示随之。替身随形：`reconcile_duty_payment_test.go` `dutyStoreDouble`、`verify_release_gate_test.go`、`receive_on_adopted_funds_fact_test.go` `unreachedDutyStores` / 新 `rederiveStores`、`cmd/parcel-customs-register` `fakeDutyBook`、HTTP `dutyStoreStub`。
- (4) 交接不变：`handOffVerification` 未改；信封 ID 仍 `dutyPaymentVerificationEventID`（分区键 + 税费 + 资金 + 指纹），资金版本经指纹自然一版一封；SA 采用 `AdoptDutyPaymentVerification` 那族零改动（判据 (5)），第三格证 SA 按新指纹采用。

### 判断项（读代码写它实际怎么做的）

1. **`DutyPaymentVerification` 构造门对 `PENDING` 组合的接受**：`domain.VerifyDutyPayment` 对三轴各自只查 `valid()`——`DutyCoverage.valid()` 是 `CoverageNone..CoverageFull` 区间、`DutyDelta.valid()` 是 `DeltaNone..DeltaPending`、`DutyFactValidity.valid()` 是 `FundsFactValid..FundsFactPending`——没有跨轴约束，(`COVERED`, `PENDING`, `PENDING`) 与任何组合一样进门；库侧 `0016` 三列 CHECK 各自封闭、无跨列。所以 (a′) 没加新格也没加门，`TestDutyVerificationRecordsWhichFundsFactVersionItJudged` 正是用这一组合构造的。
2. **`CurrentDutyVerificationView` 同秒两版的取舍**：`LoadCurrentDutyVerification` `ORDER BY verified_at DESC, version_digest ASC LIMIT 1`——同一时刻取指纹字典序小者；`latestVerificationPerLineage` 在升序列上只在严格更晚时换人，同时刻留先出现（字典序小）者，两口一把尺。后果：(a′) 版与前版若同一 `verified_at`，谁是「当前」由指纹字典序定，**不是**「重派的那版自然成当前」。生产 `cmd/parcel-dispatch` 的 `systemClock{}` 取 `time.Now()`，同一事务里两版相隔纳秒、落库微秒级，同刻概率极低但非零；替身用例用固定时钟时两版确实同刻（`TestANewFundsFactVersionFormsAPendingVerificationVersionOnEachLineage` 断言 `VerifiedAt().Equal(dutyBaseAt)`），该用例不断言「当前」是哪版。
3. **`handOffVerification` 的事务边界**：交接在 `SaveVerification` 答 `已登记` 后、同一 ctx 调 `Handoff.HandOffDutyPaymentVerification`（真装配 `NewOutboxDutyPaymentVerificationHandoff` → `outboxintent.EnqueueOnce` 走 `RequireExecutor`），与核对行同一事务；但**交接失败不翻核对**——`handOffVerification` 吞错、返回 `CONT-DUTY-VERIFICATION/<范围>/<指纹前段>` 续办引用，outcome 仍 `DutyVerificationFormed`（既有头注「失败不翻核对，留续办引用」）。重派路上 `RederiveDutyVerificationsOnFundsFactVersion` 只看每条谱系的 `Outcome()`，`lineage.Result.HandoffReference()` 没有读者；`rederivationConsumption` 对 `已重派` 一律 nil → 消费门提交。于是交接失败时：核对行提交、信封没发、消费入账成功、续办引用只留在结果对象上无人看。这是既有取舍在新路上的延伸，接手方只记录不改。
4. **信封 ID 128 字节**（原作者量到、上一代通道 2 已广播）：`dutyPaymentVerificationEventID` = `租户/duty-payment-verification/范围/税费/资金/指纹`，指纹是 64 位十六进制，再加四个引用；框架 eventing 信封 ID 上限 128 字节，引用稍长（原作者实测 `tenant-a` + `SYN-UNIT-RD` + `SYN-DUTY-RD/v1` + `bank-fact-2` = 138 字节）`EnqueueOnce` 里的 `store.Enqueue` 就拒 → 正好落进上一条的路径：核对行提交、信封没发、续办引用无声。第三格因此把税费 / 范围取短（`SYN-D9` / `SYN-U9`）并显式断言 `HandoffReference()` 为空，不让续办引用滑过。既有用例全用短指纹（`digest-v1`）或替身，没碰到。同形还有 CC `gateVerificationEventID` / `verificationEventID`（都含 64 位指纹）。不在 19 范围——05 / ADR-0069 的 ID 形状，归 CC owner 立票（通道 1 已收到要立票的一条）。
5. **与裁决字面不符、以代码为准**：
   - 裁决 1「`verifiedAt` 取库时钟」→ 代码取 `handler.deps.Clock.Now()`（编排时钟，与 `VerifyPayment` 同源；生产是 `systemClock{}` 进程时钟），不是库 `now()`。理由是文件头注「核对时刻取编排的时钟——与 VerifyPayment 同源」；库时钟是 0021 版本行 `received_at` 的口径，核对行 `verified_at` 从来由应用给。
   - 裁决 3 (2)「要求而新版本未提供 → 该谱系不形成、**登**一条未决 reason 交人」→ 代码不登任何行：reason 只在 `DutyVerificationRederivationResult.Lineages()` 各条的 `Result.UndecidedReason()` 上，消费门按 `已重派` 入账后不落库、不另记；可查的痕迹只有「该谱系没有新版本行」。同理 `PayerRequirementNotConfigured`（程序没登规则）也走谱系级业务未决而不是整笔未决——`dependencyFailure()` 只认四个 `*Unavailable`，裁决没写，代码这样定。
   - 裁决 3 (2)「`ExternalFundsFactRegister` **加** `LoadFundsFactVersion`」→ 代码是**改名**（`LoadFundsFact` 删除），与同条 (3)「端口方法删或改名为按版本读」一致，取后者。

### 验证（接手方自己跑，树 `a0cb6fef`）

- `gofmt -l ./internal/ ./cmd/` 输出为空；`go build ./...`、`go vet ./...` 退出码 0、零输出。
- 真库（占 55432 广播后跑，跑完即释）：`$env:IDP_PARCEL_POSTGRES_DSN = "postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable"`；`go test -p 1 -count=1 -v ./internal/customscompliance/... ./internal/architecture/... ./cmd/parcel-dispatch/... ./cmd/parcel-api/... ./cmd/parcel-customs-register/... ./internal/settlementaccounting/...` → 退出码 0；`--- PASS` 顶层 1098 + 子测试 600 = 1698，`--- FAIL` 0，`--- SKIP` 0（DSN 生效）。全部包 `ok`（`registrationjson` / `ports` 两处 `[no test files]`）。本记录点名的十四个用例全部 `PASS` 在列。
- 取证命令：`git diff --stat 1e74aaaf a0cb6fef -- migrations/`（只 `0023`）；`… -- migrations/customs_compliance/0016* 0021* 0022*` 空；`… -- internal/settlementaccounting` 空；`git grep -n '最近接收' -- internal/customscompliance/` 命中处逐一读过，见判据 (3)。

### 能力边界

读了：本票全文与裁决 1–5、22:0x Comments；`git diff 1e74aaaf a0cb6fef` 全部 30 件逐 hunk（生产代码与测试）；`rederive_duty_verifications.go` 与其测试全文；`reconcile_duty_payment.go` 的 `VerifyPayment` / `handOffVerification` / `verificationDigest` / `dependencyFailure` 段；`duty_release.go` 三轴 `valid()` 与 `VerifyDutyPayment`；`verify_release_gate.go` 的 `DutyVerificationPending` 分支；`duty_payment_verification_handoff.go` 信封 ID 两函数；`outboxintent/enqueue_once.go` 全文；`inboxconsume/consume.go` 事务段；`cmd/parcel-dispatch/assemble.go` 的 `systemClock` 与哨兵段；`0023` 全文。**没读**：`0016` / `0019` / `0021` / `0022` 正文（零 diff 只按 `--stat` 认）；SA 采用编排本体；`duty_payment_gate_rule.go` 的 `Judge`；框架 eventing 的 128 字节校验源码（按原作者实测与第三格注释转记）；admin-web 除两文件外的引用处，未跑前端构建。没写代码、没改测试、没改 `spec.md` / `tasks.md`。

## 参照

[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) 完成记录「后继票的形」；[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md)；[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 三停格；`internal/customscompliance/application/reconcile_duty_payment.go`（`VerifyPayment` / `ReceiveFundsFact`）；`internal/customscompliance/ports/ports.go`（`ExternalFundsFactRegister` 头注「两个读口分工」、`DutyVerificationStore`）；CC `CONTEXT.md` 税费付款核对与集成规则两句；UC-CC-009 一致性节。

## Comments

- 2026-09-14 14:0x · 通道 1：立票（推送方按 13 裁决 2 立）。只写票面，未动代码。
- 2026-09-14 16:0x · 通道 1（接管会话）：sa-cc/13 补评审 ← 通道 4 Spec 非阻断 2 给本票的一句——`VerifyPayment` 在生产有调用方（`cmd/parcel-api/assemble_customs_registration.go` `transactionalDutyPaymentVerificationRegistration.VerifyPayment`、`cmd/parcel-customs-register/translate.go`）；13 之后同一事实 ≥ 2 版时付款人维读的是最近接收那一版，而 `duty_payment_verification` 键（租户、税费、资金事实、范围、指纹）里没有资金版本——v2 到达后调用方以同三轴同依据重核落`已存在`，形成不出「新核对版本」。**本票立字时把「核对身份缺资金版本」写成完成判据**（做法 3 已接，判据里点名）。同一张键的另一维（程序）在 [22](22-cc-duty-verification-procedure-is-caller-asserted-and-not-recorded.md)，两票若同期在途改键合一笔。
- **2026-09-14 21:2x · 取证 ← 通道 4 · 钉 `bccb60a1`**。只读代码、逐符号量「要裁的」两条所需的事实；不裁、不提方案。每条写「取数方法 → 结果」。

  **要裁的 1（新核对版本的三轴与关联依据从哪来）**

  - `internal/customscompliance/domain/duty_release.go` `VerifyDutyPayment` 入参（读源）：`duty AssessedDutyReference, funds ExternalFundsFactReference, scope DecisionScopeReference, coverage DutyCoverage, delta DutyDelta, validity DutyFactValidity, verifiedAt time.Time`——七个全部构造期必填：任一 `valid()` 为假或 `verifiedAt.IsZero()` 即 `ErrInvalidDutyVerification`。入参里**没有**程序、**没有**资金版本。
  - 三轴取值集（同文件 `String()` 词形）：`DutyCoverage` = `NONE / PARTIAL / COVERED`；`DutyDelta` = `NO_DELTA / SHORT / EXCESS / PENDING`；`DutyFactValidity` = `VALID / INVALIDATED / CONFLICTING / PENDING`。**有效性轴今天有 `PENDING` 一格**（`FundsFactPending`），差额轴也有（`DeltaPending`）；库列 CHECK `duty_payment_verification_validity_closed` 同词（`0016`）。另一侧事实：门禁读数 `domain.DutyPaymentGateReading.valid()`（`duty_payment_gate_rule.go`）明文拒 `DeltaPending` / `FundsFactConflicting` / `FundsFactPending`，`0019` `gate_condition_duty_payment_rule_validity_closed` 只放 `VALID / INVALIDATED`——一版有效性为 `PENDING` 的核对能入 `duty_payment_verification`，但按今天的门禁规则表登不进接受集合、读数上也过不了 `valid()`。
  - `DutyPaymentVerification` 结构体字段：`duty / funds / scope / coverage / delta / validity / verifiedAt`，读口各一；**没有** `Basis`——依据在 `ports.DutyVerificationRecord.Basis` 上，不在领域对象里（`ports.go` `DutyVerificationRecord` 头注原句「依据不在领域对象里」）。
  - `internal/customscompliance/ports/ports.go` `DutyVerificationKey` 组成：`TenantID / Duty / Funds / Scope / Digest`。`Digest` 由 `application.verificationDigest(command)` 算：`Coverage`、`Delta`、`Validity` 三个枚举**整数**加 `Basis`，以 `\x00` 拼接后 sha256——头注「三维身份在键上，不进指纹」。`Procedure` 与资金版本都不进指纹、也不在键上。
  - `ports.DutyVerificationStore` 方法集：`FindVerification(ctx, key DutyVerificationKey)` 与 `SaveVerification(ctx, record DutyVerificationRecord)`，共两个方法（钉 `bccb60a1`）。没有按（租户、资金事实）列全部版本的读口。**旁边两口**能按别的维读核对：`CurrentDutyVerificationView.LoadCurrentDutyVerification(ctx, tenant, scope)`（按范围取「当前」一版，头注「监管程序不是核对的维度……所以这里不按边界过滤」；postgres 实现 `duty_payment_gate_rule.go` `LoadCurrentDutyVerification` SQL `ORDER BY verified_at DESC, version_digest ASC LIMIT 1`）与 `DutyVerificationCatalogueRead.ListDutyVerifications(ctx, tenant, limit)`（按租户上列）。
  - `ports.ExternalFundsFactRegister` 头注「两个读口分工」原词：「LoadFundsFact 交回本上下文**最近接收**的那一版——核对（VerifyPayment）今天按引用读前置与付款人维、命令上没有版本，它读的就是这一版；『新版本到达 → 形成新核对版本』的编排归后继票，那张票落地时核对该按版本读。ListFundsFactVersions 按接收先后列全部版本、每版带回指前版——『登记册看得见新版本与回指』（裁决 2）就是这一口；空切片即一版都没接收。」
  - `ports.DutyCollaborationStore` 的形：`FindCollaboration(ctx, tenant, scope, duty)` / `SaveCollaboration(ctx, tenant, collaboration)`；协作事项本体 `domain.DutyCollaborationSpec` 字段 `Kind / Duty / NoPayBasis / Scope / Obligor / Requirement / Target / FormedAt`，库表 `duty_payment_collaboration` 主键 `(tenant_id, scope_ref, duty_ref)`（`0016`），`kind` 封闭二值 `ASSESSED_DUTY / EXPLICITLY_NOT_REQUIRED`，CHECK `duty_payment_collaboration_basis_matches_kind` 只放这两种形状。(b) 路若照它的形立「待重核对事项」，今天这张表上**没有**第三种 `kind`、没有资金事实列、没有「待办」状态列，一范围一税费引用至多一行。
  - [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 的 `DutyPaymentVerificationHandoff` 信封键（`adapters/postgres/duty_payment_verification_handoff.go`）：`dutyPaymentVerificationEventID(key)` = `dutyPaymentVerificationPartitionKey(key) + "/" + Duty + "/" + Funds + "/" + Digest`，其中分区键 = `TenantID + "/duty-payment-verification/" + Scope`。**信封 ID 带的「版本」是 `Digest`（内容指纹），不带资金事实版本**；载荷 `dutyPaymentVerificationPayload` 五维 `tenantId / scope / duty / funds / digest`。
  - SA 侧 sa-cc/09 采用登记册存的引用：`migrations/settlement_accounting/0020_duty_payment_verification_adoption.sql` 表 `duty_payment_verification_adoption` 主键 `(tenant_id, scope_ref, duty_ref, funds_ref, version_digest)`——**照抄 CC 核对主键五维**，无跨 schema 外键；`internal/settlementaccounting/adapters/customscompliance/duty_payment_verification_view.go` 回读时重铸 `ccports.DutyVerificationKey{TenantID, Duty, Funds, Scope, Digest: verification.Version().String()}`——SA 领域里叫 `Version()` 的就是 CC 的 `Digest`。
  - 原文引文（不转述）。UC-CC-009「一致性、幂等与并发」节：「外部资金事实迟到、更正、资金退回或付款撤销时，形成新核对版本并保留原覆盖判断；不删除原付款、不按最后到达覆盖。」同用例「税费付款与放行核对规则」节另一句：「税费、付款或放行事实迟到时，按业务发生时间、适用时间、当前有效性和更正关系形成新核对版本；不得按消息到达顺序覆盖。」CC `CONTEXT.md`「税费付款核对」词条：「`customs-compliance` 将监管核定税费与银行、支付或财务系统提供的外部资金事实，按明确申报范围、法定义务以及来源提供或真实程序要求的付款人、金额、币种、业务时间等维度进行的版本化比较判断。它分别判断覆盖状态、差额和事实有效性，不形成实际付款、客户回收或监管放行。」Rules「税费、放行与案件闭环」段：「部分付款、超额付款、错误范围、错误币种、重复付款、资金退回和付款撤销都必须保留原事实并形成新的核对判断。」Lifecycles「税费付款与放行门禁」段：「外部资金事实接入 → 税费付款核对：按真实程序逐范围分别形成覆盖状态、差额状态和有效性状态；资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态。」

  **要裁的 2（触发要不要区分 `corrects` 在不在）**

  - `application.ReceiveFundsFact`（`reconcile_duty_payment.go`）对回指的全部校验只有一条：`registration.Corrects == registration.Version` → `未受理`。`Corrects` 零值不拒、不查前版是否已到（头注「回指是提供方给的字面，照登不校验前版是否已到」）。
  - `adapters/postgres/duty_payment_reconciliation.go` `RegisterFundsFact`：同一条校验（`a version cannot correct itself`）；写口两次 `INSERT … ON CONFLICT DO NOTHING`——身份行 `external_funds_fact (tenant_id, fact_ref)`，版本行 `external_funds_fact_version (tenant_id, fact_ref, version)`；`correctsColumn` 把零值回指写成 `NULL`。
  - `migrations/customs_compliance/0021_external_funds_fact_versions.sql`：`corrects_version text NULL`，唯一 CHECK `external_funds_fact_version_corrects_another_version` = `corrects_version IS NULL OR (btrim(corrects_version) <> '' AND corrects_version <> version)`；主键 `(tenant_id, fact_ref, version)`。没有「非首版必须带回指」的约束，也没有到本表自身的外键（头注明说不设）。
  - 消费侧路径 `adapters/settlementaccounting/receive_on_adopted_funds_fact.go`：`Version` 取信封所指、`Corrects` 取 `content.Corrects`（SA 只读口交回的事实本体）原样递给 `ReceiveFundsFact`，不加校验。
  - **答案**：同一事实在已有核对版本之后到达一个**不带回指**的新版本（版本字面不同、`Corrects` 零值），代码路径 `ReceiveFundsFact → RegisterFundsFact → INSERT external_funds_fact_version` 全程放行，落成 `corrects_version IS NULL` 的第二行，`ReceiveFundsFact` 答 `已接收`（`FundsFactReceived`）。「有回指」与「已有既往核对版本」在代码上是两个互不蕴含的条件：前者看 `Corrects`，后者今天没有读口能问（`DutyVerificationStore` 无按资金事实列的方法）。

  **两路各自的实测后果（只列不选）**

  - 选 (a)「复用前版三轴与依据、有效性轴标 `PENDING`，形成一版核对」要动的符号：新编排（`application` 新文件，读 `ExternalFundsFactRegister.ListFundsFactVersions` + `DutyVerificationStore` 新增按（租户、资金事实）列全部核对的方法 + `DutyCollaborationStore.FindCollaboration` + `PayerRequirementRuleView.LoadPayerRequirement`）；`ports.DutyVerificationStore` 加方法 → 替身 `internal/customscompliance/application/reconcile_duty_payment_test.go`、`verify_release_gate_test.go`、`cmd/parcel-customs-register/duty_registers_test.go`（`fakeDutyBook`）、`cmd/parcel-dispatch/assemble_test.go` 随形；`adapters/postgres/duty_payment_reconciliation.go` 加实现与 SQL；`VerifyDutyPaymentCommand` 加 `FundsVersion`（做法 3）→ `verificationDigest` 或 `DutyVerificationKey` 之一要带它（进键则见 [22](22-cc-duty-verification-procedure-is-caller-asserted-and-not-recorded.md) 取证「改键会拆到谁」那段：`0016` 主键、`0019` 的 `gate_verification.duty_version_digest` 三列引用、SA `0020` 采用表主键、`dutyPaymentVerificationEventID`、`dutyPaymentVerificationPayload`、SA `decodeFormedDutyPaymentVerification`、`duty_payment_verification_view.go` 重铸键）；有效性 `PENDING` 的新版成为 `CurrentDutyVerificationView` 的「当前」后（按 `verified_at DESC` 它就是最新那版），`application.VerifyReleaseGate` 那一道读到它，`domain.DutyPaymentGateRule.Judge` 对 `DeltaPending / FundsFactConflicting / FundsFactPending` 答 `ErrDutyPaymentGateUndecided`，门禁编排折成 `gateUndecided(DutyVerificationPending)`——**该范围的放行门禁在此期间答未决，不是未满足**（`verify_release_gate.go` 该分支实测）。
  - 选 (b)「不形成核对，只登一条待重核对事项」要动的符号：新表或 `duty_payment_collaboration` 加格——后者今天 `kind` 封闭二值 + `basis_matches_kind` CHECK + 主键无资金事实维，加格即新迁移改 CHECK 与键；`ports` 新端口或 `DutyCollaborationStore` 加方法（同上替身随形）；新编排同 (a) 的读口集但不调 `VerifyDutyPayment`、不调 `DutyPaymentVerificationHandoff`（无新核对版本即无信封，[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 与 SA 侧零改动）；`DutyVerificationStore` 仍要加按资金事实列的读口（触发条件「已有既往核对」两路都要问）；`duty_payment_verification` 表与 `0016` 在这条路上不动，`LoadFundsFact`「最近接收」口径的退役与 `VerifyDutyPaymentCommand.FundsVersion` 是否仍做，随裁。

  **能力边界**：读了 `duty_release.go`、`reconcile_duty_payment.go`、`ports.go`、`duty_collaboration.go`（结构体段）、`duty_payment_gate_rule.go`（`DutyPaymentGateReading` / `DutyVerificationReference` 段）、`adapters/postgres/duty_payment_reconciliation.go`、`duty_payment_verification_handoff.go`、`duty_payment_gate_rule.go`（`LoadCurrentDutyVerification`）、`adapters/settlementaccounting/receive_on_adopted_funds_fact.go`（回指段）、`0016` / `0019` / `0020` / `0021`、SA `0020`、SA `duty_payment_verification_view.go`（键重铸段）、SA `duty_payment_verification_consumer.go`（译码段）、UC-CC-009 两节、CONTEXT 四句、13 裁决与「后继票的形」。`verify_release_gate.go`（税费付款那一道的分支）。**没读**：SA 采用编排 `AdoptOnDutyPaymentVerificationAdapter` 本体、任何测试的断言内容、真库（未跑、未占 55432）。
- 2026-09-14 22:0x · 通道 1 推送方（自 [26](26-cc-funds-fact-versions-test-registers-in-map-order-and-flakes-on-received-at.md) 评审 ← 通道 4 Standards ① 转记）：本票做法 3 让 `LoadFundsFact`「最近接收」口径退役时，CC postgres 反序格用例 `TestFundsFactVersionsArrivingOutOfOrderListByReceiptAndLoadTheLatestReceived` 最后一条断言（`LoadFundsFact` 交回最近接收的 v1）会先红——那是本票作者要一并改口的地方，不是 26 的遗漏；26 头注未把该口径写成永久规则。
