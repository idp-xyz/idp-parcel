# CC 入向登记没有版本维：SA 合法的更正版本到 CC 侧落成「内容冲突」，只留 inbox 痕，到不了 UC-CC-009 的重新核对

Category: enhancement
Status: resolved——**已进 main，2026-09-14 14:0x**（通道 1 推送方：分支基 main 当时 tip `ace35ac3`、main 未动，直接 ff——四笔 SHA 不变 `c90d8606` / `e833355d` / `bab22328` / `8dbd49e2` + 清点 `0bd86d42` + 本簿记笔；`0bd86d42` 带 DSN 全仓 `-p 1 -count=1` 110 ok / 0 FAIL / 16 无测试 / 0 cached；清点 CC 生产 92→93、迁移 169→170。**非作者评审缺席**，处置同 12：放行 + 补评审（见 Comments「进 main 记录」）。后继两票已立：[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md)（触发重核对，draft，要裁三轴从哪来）、[20](20-sa-external-funds-fact-holds-one-row-per-fact-and-cannot-store-a-correction.md)（SA 存不下第二版，draft，归 SA owner））；此前 resolved——2026-09-14 13:5x 通道 1 自办完工（分支 `mcp1-sacc13` 基远端 main `ace35ac3`：认领 `c90d8606`、之一 `e833355d`、之二 `bab22328`、本笔）；完成判据 1–4 全部落地，验证、判断项与后继票的形见「完成记录」。**非作者评审缺席**：隔离子代理仍「Authentication error」、无在线通道，作者两轴自查见「完成记录」，处置同 sa-cc/12（放行 + 补评审）。进 main 的 SHA 由推送方重放后另记。此前 in-progress——2026-09-14 13:2x 通道 1 自办（用户「请你自决」两次；无在线通道可派；`/implement` › `/tdd`；分支 `mcp1-sacc13` 基远端 main `ace35ac3`（sa-cc/12 刚进 main，`0020` 已在），树 `D:/tops/idp-parcel-mcp1-sacc13`；迁移序号钉 `customs_compliance/0021`；形取**版本子表**——`duty_payment_verification` 外键钉在 `external_funds_fact (tenant_id, fact_ref)` 上，主键加版本会拆掉它，子表让事实身份行不动、版本各占一行）；此前 ready-for-agent——2026-09-14 10:2x 通道 1 按用户 10:1x「授权代裁」（CC owner 口径）裁「要裁的」1 / 2：**要版本维**（一版本一行，形由作者定）；**触发重核对不在本票**，本票只到「登记册看得见新版本与回指」，触发归核对那一族另立票；全文见文末「裁决」。「做法」与「完成判据」按裁决写实，取证锚仍是票面的 `9ddbafcf`，作者开工先在 main 重量。此前 draft——2026-09-10 22:0x 通道 4 立票（按通道 1 派单 task-d00c5556；sa-cc/03 非作者评审 Spec 非阻断 ① 的后继）。只写票面未动代码；取证锚 main `9ddbafcf`
Blocked by: 无（[03](03-cc-inbox-consumer-receives-external-funds-fact.md) 已进 main；本票要裁的归 CC owner；与 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 并列、互不阻塞）

## 缺口（取证于 `9ddbafcf`，逐符号名）

- SA 侧：更正 / 撤销是**同一事实的新版本**——`domain.ExternalFundsFact` 的 `Version()` / `Corrects()`，`CorrectAmount` 结构拷贝出新版本并回指前版；[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) 裁决 2 定更正复用同一事件类型 `settlement-accounting.external-funds-fact.adopted` 再发一封，ID `<租户>/funds-fact/<事实>/<版本>`，载荷带可缺席的 `corrects`。
- CC 侧：`ports.ExternalFundsFactRegistration` 只有 Fact / Source / Payer / Currency / AmountMinor / OccurredAt，**无版本、无回指**；`customs_compliance/0016` 的 `external_funds_fact` 主键 `(tenant_id, fact_ref)`——一事实一行。`ReceiveFundsFact` 按引用幂等：同引用同内容 → `FundsFactExisting`，同引用换内容 → `FundsFactContentConflict`。
- 于是 v2 更正版（同 `fact_ref`，金额或业务时间变了）到 CC：`ccinbox.ExternalFundsFactConsumer` 头注写「对首版与更正版一视同仁地交给处理方，重新核对是 UC-CC-009 的事，不在这里分路」；`ReceiveOnAdoptedFundsFactAdapter` 回查那一版、译六格交编排 → 编排答 `FundsFactContentConflict` → 消费门入账、不重投（`receiveConsumption`）。`receive_on_adopted_funds_fact_test.go` 的 `TestAnAdoptedFundsFactIsRegisteredOnceAndReplayOrConflictStillSettles` 第三段把「v2 更正到达」明确建模为内容冲突、CC 登记保留 v1 金额——这是 03 完成判据 2 与做法 3 的字面，代码没有错，是登记册的形装不下更正。
- 结果：SA 合法更正与真冲突（同引用两个来源各说一套）在 CC **不可区分**；更正在 CC 侧只留 inbox 已处理的痕迹，`VerifyPayment` 的调用方看不到有新版本，无处触发「新核对版本」。消费者头注那句「重新核对是 UC-CC-009 的事」在数据路径上到不了 UC-CC-009。
- 与 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 同根异题：12 是登记的一格（付款人）能不能缺席，本票是登记的键有没有版本维。03 评审判断题 ② ⑤ 点出「同根」：CC 读端口 `AdoptedFundsFact` 不带 Version / Corrects、消费者不译 `corrects`，都因为登记册没有那一维可放——不是在读端口上加字段能解，要登记册先有版本维。

## 语言从哪里来

- CC `CONTEXT.md`「税费付款核对」：「部分付款、超额付款、错误范围、错误币种、重复付款、资金退回和付款撤销都必须保留原事实并形成新的核对判断」；集成规则：「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态」。
- UC-CC-009 一致性节：「外部资金事实迟到、更正、资金退回或付款撤销时，形成新核对版本并保留原覆盖判断；不删除原付款、不按最后到达覆盖」；「保留全部版本和原事实」。
- 两句合起来要的是：更正版本**进得来**（原事实与新版本都保留）且**能成为重新核对的来源**。今天的 `内容冲突` 两样都没做到——原事实保住了，新版本被当作矛盾丢在 inbox。

## 做法（待裁后写实）

1. CC 入向登记加版本维：`ExternalFundsFactRegistration` 加 `Version`（与信封 / SA `FundsFactVersion` 同字面）与可缺席的 `Corrects`；`external_funds_fact` 改为一版本一行（主键加版本，或另起版本子表——形由作者定），新迁移序号重取，`0016` 不改；`ReceiveFundsFact` 幂等键随之变为（引用 + 版本）：同版本同内容 → `已存在`，同版本换内容 → `内容冲突`（真冲突），**新版本 → 新一行、回指前版**。
2. `ccports.AdoptedFundsFact` 读端口带出 Version / Corrects（03 评审 ② 所说「要用时再加」的那一刻到了）；`ReceiveOnAdoptedFundsFactAdapter` 把信封所指版本与回查到的回指译进登记；消费者仍只译不判。
3. 「新版本到达 → 重新核对」的触发落点：`VerifyPayment` 的三轴与关联依据由调用方交、调用方今天不在本进程（03 票面红线原句）。本票至少保证登记册上**看得见**新版本与回指（`LoadFundsFact` 或新读口按引用列全部版本）；触发重核对的编排是否随本票立，见「要裁的」2。
4. 03 判断题 ③ 的保留（`未受理` 在 CC 侧无落痕）不在本票，归 12。

## 红线

- 不改 SA 任何东西：版本与回指的权威在 SA，CC 只登引用 + 核对所需维度（03 票面红线）；不把 SA 的版本链复制成 CC 的第二份。
- 不按到达顺序覆盖：新版本追加，旧版本一行不动（UC-CC-009「不按最后到达覆盖」）。
- 不由消费者判「这是更正还是冲突」：消费者只译，判在编排（03 做法 3 原句）。
- 真实付款条件 / 关联规则属实例半边，一行不预填。

## 完成判据（待裁后写实）

1. 应用层：v1 已登记，v2 同引用、回指 v1、金额变 → `已接收`（新一行），按引用列出两版且 v2 回指 v1；同版本重投 → `已存在`；同版本换内容 → `内容冲突`。
2. 真库：新迁移往返；`0016` 一字未动；`cmd/parcel-dispatch` 真库装配用例的正例扩一格（v2 到达落第二行）。
3. 与 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 的交叉：付款人可缺席与版本维互不依赖，任一先落另一不重做（各改各的列）。
4. 消费者头注「重新核对是 UC-CC-009 的事」那句改为指向实际落点。

## 地盘

`internal/customscompliance/{ports,application,adapters/postgres,adapters/settlementaccounting,adapters/inbox}`、`migrations/customs_compliance/`（新序号）、`cmd/parcel-dispatch/assemble_test.go` 正例一格（共享文件，动前占号）。SA 侧不动。

## 要裁的

1. **CC 登记要不要版本维**——归 CC owner。不要的话，更正版本在 CC 的正确归宿是什么：a) 照旧 `内容冲突` 入账、靠运营从 SA 读面比对（等于承认 UC-CC-009「形成新核对版本」在 CC 侧无入口）；b) 消费者对带 `corrects` 的信封分路（违 03 做法 3「消费者只译不判」）。要的话，形取「主键加版本」还是「版本子表」由作者定，本票不裁形。
2. **（已裁，见「裁决」）** 范围题：新版本到达要不要在本票内触发重新核对（一个 CC 内部的「资金事实新版本 → 待重核对」编排），还是本票只到「登记册看得见新版本」、触发归核对那一族（[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 邻接）——归 CC owner；裁前按后者写完成判据。

## 裁决（2026-09-14 10:2x，通道 1 推送方按用户「授权代裁」以 CC owner 口径裁）

1. **要版本维。** CC CONTEXT「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实」与 UC-CC-009「保留全部版本和原事实」两句要的是更正版本**进得来、原版本留得住**，今天的 `内容冲突` 两样都做不到——这是登记册的形装不下语言，不是消费者或读端口的事（03 评审 ② ⑤ 已点出同根）。**一版本一行**：主键加版本还是版本子表由作者定，判据是「同引用列全部版本、每版回指前版」在 SQL 上一问答得出、`0016` 不改；幂等键随之为（引用 + 版本）：同版本同内容 → `已存在`，同版本换内容 → `内容冲突`（真冲突——同一版本两个来源各说一套），新版本 → 新一行回指前版（做法 1 原句）。版本与回指的字面与 SA `FundsFactVersion` / 信封 `corrects` 同，CC 只登引用不复制 SA 的版本链（票面红线）。
2. **触发重核对不在本票。** 本票只到「登记册看得见新版本与回指」：`LoadFundsFact` 或新读口按引用列全部版本，`ccports.AdoptedFundsFact` 带出 Version / Corrects，消费者仍只译不判（做法 2 / 3 前半）。「新版本到达 → 形成新核对版本」的编排归核对那一族——`VerifyPayment` 的三轴与关联依据由调用方交、调用方今天不在本进程（03 票面红线），把触发塞进本票就是在登记票里长出第二只核对编排。**作者在完成记录里写清那张后继票该长什么样**（触发落点、要读哪几口、与 [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 的关系），推送方据此立票，本票不立不做。
3. **与 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 的交叉**照票面完成判据 3：互不依赖、各改各的列；两票若同期在途，迁移序号各自重取、动前占号。
4. **不改的**：不改 SA；不按到达顺序覆盖；不由消费者判更正还是冲突；真实付款条件 / 关联规则一行不预填（票面红线）。完成判据 4（消费者头注那句改指实际落点）照旧。

## 参照

[03](03-cc-inbox-consumer-receives-external-funds-fact.md)（评审 Spec ①、判断题 ② ⑤、进 main 记录「候选后继」）；[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) 裁决 2；[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)（同根异题）；`internal/customscompliance/application/reconcile_duty_payment.go`（`ReceiveFundsFact`）；`internal/customscompliance/ports/ports.go`（`ExternalFundsFactRegistration` / `AdoptedFundsFact` / `ExternalFundsFactRegister`）；`internal/customscompliance/adapters/inbox/external_funds_fact_consumer.go` 头注；`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact.go`；`internal/settlementaccounting/domain/external_funds.go`（`Version` / `Corrects` / `CorrectAmount`）；`migrations/customs_compliance/0016_duty_payment_reconciliation.sql`；CC `CONTEXT.md` 税费付款核对与集成规则两句；UC-CC-009 一致性节。

## 完成记录

分支 `mcp1-sacc13`，基 `ace35ac3`（sa-cc/12 刚进 main，`0020` 已在）：

| SHA | 内容 |
|---|---|
| `c90d8606` | docs：认领，形定版本子表（理由见 Status） |
| `e833355d` | feat（之一）：领域 `domain.FundsFactVersion`（只作引用，不比大小不复制链）；端口 `ExternalFundsFactRegistration.Version` / `Corrects`（零值即首版）、`ExternalFundsFactRegister.ListFundsFactVersions`、`LoadFundsFact` 头注写实为「最近接收的那一版」、`AdoptedFundsFact.Version` / `Corrects`；迁移 `0021_external_funds_fact_versions.sql`（版本子表 `external_funds_fact_version`，键（租户、事实、版本），外键到身份行，`corrects_version` 可空且不得指自己；存量非零 `RAISE` 停下交人；身份表退成身份列）；postgres `RegisterFundsFact` 身份行 + 版本行两次 DO NOTHING、`LoadFundsFact` / `ListFundsFactVersions` 共用 `scanFundsFactVersions`；编排 `ReceiveFundsFact` 幂等键（引用 + 版本）+ `sameFundsFactVersion`（回指也是内容的一维）；SA 消费侧 `SettlementAdoptedFundsFactSource` 译出版本 / 回指、`ReceiveOnAdoptedFundsFactAdapter` 版本取信封所指、回指取事实本体；替身随形。真库例 `TestFundsFactVersionsAccrueAsRowsThatPointBack` + CHECK 拒版本空白 / 回指自己 / 回指空白 / 版本不属于已登记事实 |
| `bab22328` | test（之二）：`cmd/parcel-dispatch` 正例扩一格（v2 经信封落第二行回指 v1）；消费者头注改指实际落点（判据 4） |
| （本笔） | docs：本票 Status → resolved + 本完成记录 |

**逐条对完成判据**：**1** 应用层——`TestACorrectionVersionIsReceivedAsANewRowThatPointsBackToTheVersionItCorrects`：v1 已登记，v2 同引用回指 v1 金额变 → `已接收`新一行；`ListFundsFactVersions` 列两版、v2 回指 v1、v1 一字不动；同版本重投`已存在`；同版本换金额 / 换回指`内容冲突`且册上不动；`TestFundsFactVersionsKeepTheirOwnRowsRegardlessOfArrivalOrder`：版本空白 / 回指自己`未受理`不落，先到 v2 再到 v1 各占一行。**2** 真库——`TestFundsFactVersionsAccrueAsRowsThatPointBack`（两版并存、身份 1 行版本 2 行直读、同版本重登不顶替、乱序各占一行）+ `TestTheReconciliationTablesRejectWhatTheDomainRejects` 新增四格；`0021` 在全套迁移计划上施加（`internal/platform/migrate` ok）；**`0016` 一字未动**（`git diff ace35ac3 --stat -- migrations/` 只有 `0021`）；`cmd/parcel-dispatch` `TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable` 正例扩一格——v2 经 `external-funds-fact.adopted` 信封 → 消费者 → SA 只读视图 → 入向登记落第二行回指 v1，带 DSN PASS。**3** 与 12 的交叉：12 先落（`0020` 放宽 `payer_ref`），本票把内容列连同 `payer_ref` 一起搬进版本子表——12 的机制（NULL 即「未提供」+ 空白拒）在子表上逐条保留（`external_funds_fact_version_payer_provided_or_null`），12 的真库例改读子表后仍 PASS；两票没有互相重做，只是 12 的那一列换了住处。**4** `external_funds_fact_consumer.go` 头注那句改为「处理方按（引用 + 版本）把更正版本登成新一行、回指前版；触发重核对归后继票，今天登记册上看得见、还没有编排接着做」。

**做法逐条**：1 ✓（形取版本子表，理由：`duty_payment_verification` 外键钉在身份行）；2 ✓；3 前半 ✓（`ListFundsFactVersions`）、后半按裁决 2 不做；4 不在本票。**裁决逐条**：1 ✓（「同引用列全部版本、每版回指前版」在 SQL 上一问答得出——`SELECT … FROM external_funds_fact_version WHERE tenant_id = $1 AND fact_ref = $2 ORDER BY received_at`）；2 ✓（下有后继票的形）；3 ✓（判据 3）；4 ✓（SA 零 diff；不覆盖；消费者不判；无预填）。**红线**：SA 目录 `git diff ace35ac3 --stat -- internal/settlementaccounting` 为空；夹具值全合成。

**后继票的形（裁决 2 要作者写清，推送方据此立票）——「资金事实新版本到达 → 形成新核对版本」**：
- **先要裁一条**：新核对版本的三轴（覆盖 / 差额 / 有效性）与关联依据从哪来。今天 `VerifyPayment` 的三轴由调用方交（真实关联规则属实例半边，03 票面红线原句），编排不算；新版本到达时没有调用方在场。两条路：(a) 复用前版三轴与依据、有效性轴标 `PENDING`，形成「待人判」的新核对版本；(b) 不形成核对，只登一条「待重核对」事项（新表或核对表一格）交人 / 交规则。裁前不动代码。
- **触发落点**：CC 内部，`ReceiveFundsFact` 答 `已接收` 且 `DutyVerificationStore` 按（租户、资金事实）能列出既有核对版本时——触发点在 `ReceiveOnAdoptedFundsFactAdapter` 之后一格（消费侧适配器仍只译不判，触发另起一只编排），不塞进 `ReceiveFundsFact`。
- **要读哪几口**：`ExternalFundsFactRegister.ListFundsFactVersions`（新版内容与回指）、`DutyVerificationStore`（既有核对版本——今天只有按幂等键 `FindVerification`，要加「按（租户、资金事实）列全部核对」读口）、`DutyCollaborationStore`（协作事项仍在）、`PayerRequirementRuleView`（付款人维照 sa-cc/12 三停格）。
- **核对按版本读**：`VerifyDutyPaymentCommand` 加 `FundsVersion`、`duty_payment_verification` 加 `funds_version` 列（回指哪一版事实）、`LoadFundsFact`「最近接收」这一临时口径随之退役——这是本票端口头注里写明的债。
- **与 05 的关系**：新核对版本形成后同样走 05 的 `DutyPaymentVerificationHandoff`（每版一封）交 SA；SA 侧的采用（sa-cc/09 那族）按版本收，不必改。

**判断项（归 owner）**：① **SA 侧存不下第二版**（归 SA owner）：`settlement_accounting` 0004 `external_funds_fact` 主键 `(tenant_id, fact_id)` 一事实一行，`ExternalFundsFacts.Save` DO NOTHING；`domain.ExternalFundsFact.CorrectAmount` 在 SA **无生产调用点**；`AdoptedFundsFactView` 头注自己写「库里将来一事实多行」。sa-cc/02 裁决 2「更正复用同一事件类型再发一封」在 SA 侧今天没有落地路径——本票只解 CC 半边，dispatch 正例因此另用一条事实、v1 经 CC 写口预铺。② `LoadFundsFact` 交回「最近接收的那一版」是临时口径（端口头注写明），核对按版本读随后继票落；同一事务内到达的两版 `received_at` 相同，再按版本字面定序只为确定性。③ 回指前版不校验是否已到（版本链权威在 SA）；乱序到达各占一行，链是否连续由 SA 说。④ 迁移 `0021` 的存量守卫用 `DO $$ … RAISE EXCEPTION`（先例 CC `0011`）：当前无租户存量零；若哪个库上不为零，迁移停下交人搬，不替存量编版本。⑤ HTTP 读面（`DutyReconciliationCatalogue`）今天不列资金事实，版本在读面上不可见——要不要给它一口归后继。

**验证**（树 `D:/tops/idp-parcel-mcp1-sacc13`，13:2x–13:5x）：每笔 `gofmt -l` 空、`go build ./...` / `go vet` 0；不带 DSN CC 七包 + `cmd/parcel-dispatch` + `cmd/parcel-customs-register` + `cmd/parcel-api` + `internal/architecture` 全 ok；**带 DSN** `-p 1 -count=1` CC postgres + `internal/platform/migrate` + cmd 三包 ok（`e833355d`），`TestFundsFactVersionsAccrueAsRowsThatPointBack` / `TestTheReconciliationTablesRejectWhatTheDomainRejects` / `TestAFundsFactWithoutAPayerRoundTripsAsExplicitlyNotProvided` `-v` PASS；dispatch 正例（`bab22328`）带 DSN PASS。全量与清点由推送方在重放 tip 上兑。

**作者两轴自查（子代理不可用的替代，非「非作者评审」）**：Standards——注释全中文、跨文件引用符号名无行号；新词只有 `FundsFactVersion`（与 SA 同字面）；无默认无预填；SA 零 diff。Spec——做法 / 裁决 / 判据逐条如上；`0016` 不在 diff；`0021` 无 INSERT；消费者只译不判（`receive_on_adopted_funds_fact.go` 无分路）。

## Comments

- 2026-09-14 13:2x · 通道 1：认领自办（`c90d8606`）。
- 2026-09-14 13:5x · 通道 1：之一 / 之二 + 本笔完成记录；Status resolved；SA 侧存不下第二版这一发现写进判断项 ①，等推送方立 SA 票与「新版本到达 → 形成新核对版本」票。等重放。
- **2026-09-14 14:0x · 进 main 记录（通道 1 推送方）**：分支 `mcp1-sacc13` 基 `ace35ac3` = 当时 main tip，`ls-remote` 核 main 未动 → 无需 cherry-pick，四笔 SHA 原样 `c90d8606` / `e833355d` / `bab22328` / `8dbd49e2`；清点在 `8dbd49e2` 干净检出重生成 → `0bd86d42`；`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；14:0x 先排队列再占号，带 DSN 全仓 `-p 1 -count=1` **110 ok / 0 FAIL / 16 无测试 / 0 cached**（130 s）；释号。簿记一笔在其上 → ff → `push <sha>:main`。分支 → `merged/`，远端删，树拆。
  - **非作者评审缺席，推送方按 12 那条同一裁法放行 + 补评审**（用户 13:0x 授权「自决」、13:2x 再授权；子代理 12:5x–14:0x 六次「Authentication error」）。权衡同 12，另加两点：本票的形（版本子表）在认领笔里先写了理由再动代码，`0016` 的外键是否保住有真库用例（`版本不属于已登记的事实` 被拒）钉着；SA 零 diff 由 `git diff --stat` 核过。**补评审**：子代理或任一通道恢复后对 main 上 `e833355d..8dbd49e2`（基线 `ace35ac3`，spec 指本票）跑两轴一次，结论回写本条之下；有阻断另立票修。
  - **立票两张**（推送方按裁决 2 与判断项 ①）：[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md) draft——「资金事实新版本到达 → 形成新核对版本」，要裁三轴与依据从哪来（归 CC owner）；[20](20-sa-external-funds-fact-holds-one-row-per-fact-and-cannot-store-a-correction.md) draft——SA `external_funds_fact` 一事实一行、`CorrectAmount` 无生产调用点（归 SA owner）。
- 2026-09-10 22:0x · 通道 4（task-d00c5556，取证锚 main `9ddbafcf`）：立票。**只写票面，未动代码。** 能力边界：读过 03 全文（含评审与进 main 记录）、`reconcile_duty_payment.go` 的 `ReceiveFundsFact`、`ports.go` 的三个类型、`ccinbox` 消费者头注、`receive_on_adopted_funds_fact.go` 头注与其测试第三段、`0016` 表结构、SA `external_funds.go` 的版本 / 回指访问器、CC CONTEXT 与 UC-CC-009 引到的几句；**没读** SA `AdoptedFundsFactView` 的 SQL 与 05 票面全文——「要裁的」2 把触发落点归到 05 邻接是按 spec 子票表那一行推的，作者开工时核。本票不复述 12 的付款人题，只在「缺口」末段写两者为何同根。
