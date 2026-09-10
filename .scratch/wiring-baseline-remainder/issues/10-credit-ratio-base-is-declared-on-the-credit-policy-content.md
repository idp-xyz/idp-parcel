# 比例额度的基数由信用政策正文自己声明：封闭集、不给默认、缺席在构造门拒

Category: enhancement
Status: resolved——2026-09-09 23:2x 通道 6 接续收口（作者通道 3 五笔 + 接续三笔，分支 `mcp3-wbr10` 基 `90c025ca`，已推 origin；完成记录见文末，进 main 记录归推送方）。此前：in-progress——2026-09-09 22:2x 通道 3 认领，分支 `mcp3-wbr10` 基 `90c025ca`，单 task-519de030-3eec-46c4-9468-f190e8926899。此前：2026-09-09 通道 1 代裁立票（用户经 IDP 队列授权「你自决，目标是全部解决」）：wbr/03「未落三件」之③、ADR-0127 决定四「比例额度的基数今天未裁……
那是 `BD-*` 一类，等它自己的裁决」。裁决方向见下；封闭集的成员表交本票 `/domain-modeling` 一格定、落 ADR-0129（号由通道 1 给）
Blocked by: 无（PC 半边与 SA 半边同票；SA 半边的 contract 段是 [09](./09-sa-credit-basis-contract-and-exposure-ledger-policy-reference.md)，本票不依赖它——`CREDIT_RATIO_BASE_UNDECIDED` 那格今天就在，本票是让它不再被走到）

## 缺口

`CreditLimit` 两格封闭（金额 / 比例），比例格的注释写「比例相对于什么基数由消费方的业务判断给出」；SA `exposeCredit` 遇比例额度停在 `CREDIT_RATIO_BASE_UNDECIDED`，
不折成金额、不默认。今天没有任何一处能声明「比例相对于什么」，所以**比例额度在产品里发布得出来、永远用不上**——与「发布得出来、管理台看不见」同一类缺口。

## 裁决方向（通道 1 代裁；owner 授权自决口径）

1. **基数是信用政策正文的一格，不是消费方的判断。** CONTEXT 说信用政策「按责任法人、业务角色、费用类型、金额或比例形成版本」且「只提供业务判断依据」——
   一个「30%」没有分母不是依据；分母属于同一条政策声明，登记它的人就是能说出「30% 的什么」的人。让 SA 猜是把租户的商业判断搬进消费方，ADR-0127 决定四
   拒绝这么做是对的，本票把那一格补到它该在的地方。
2. **封闭集、不给默认、缺席在构造门拒。** `CreditLimit` 比例格必须带 `CreditRatioBase`（封闭集）；比例在场而基数缺席 → `NewCreditLimit` 拒（`ErrInvalidCreditPolicy`
   一族），不再让它流到 SA 才停在`待判断`。金额格不带基数、带了拒（「含则必填、不含则必缺」，ADR-0044 同形）。
3. **封闭集成员由 `/domain-modeling` 一格定，判据：SA 凭自己的账本与状况登记能不能算出来。** 候选：`DEPOSIT_BALANCE`（该结算账户当前预付 / 保证金余额——
   SA 运营结算余额有它）、`PRIOR_PERIOD_CONFIRMED_CHARGES`（上一结算周期已确认费用合计——SA 确认费用记录按周期可汇）。**不收**「客户合同声明的基数金额」
   这类要 PC 再长一格的候选——那是第二张票，且今天没有任何 CONTEXT 句说合同有这一格。成员表与每格 SA 的取数路径写进 ADR-0129 Decision；拿不准的成员
   单列越权风险点，不写进集合。
4. **SA 折算**：`exposeCredit` 对比例额度按声明的基数取 SA 自己的数、乘比例、进 `CreditStanding.WithAuthorizedLimit`；基数取不到（账户无余额记录 / 无上一周期）
   落 `NotFormedReason` 既有的不可用格或新加一格（作者定、写理由），**不折成 0 也不折成无限**。`CREDIT_RATIO_BASE_UNDECIDED` 保留给重建门读到的存量正文
   （构造门之前登进去的、无基数的比例额度）——无租户故理论上为零，但重建门按 ADR-0028 只校验不重算，那一格是它的合法答案。
5. **发布路**：`creditPolicyBodyDocument`（受控批文）与 `CreditPolicyBodyPayload`（表单载荷，票 awf/16）各加 `ratioBase` 一格（可缺，比例在场时服务端点名）；
   PCC-1 不换号（加一键，ADR-0126 决定一同法，`canonicalCreditPolicyBody` 加 `ratioBase,omitempty`）；词表读口（awf/20）加 `CREDIT_POLICY` 的 `ratioBase` 一集；
   表单下拉不内置、不预选。0020 正文表加一列（PC 迁移，号顺延）。

**越权风险点（单列）**：① 「基数属政策正文」是从 CONTEXT「只提供业务判断依据」推的，CONTEXT 没有逐字写基数；② 封闭集候选两格是按「SA 能自算」选的，
业务上可能还要「合同声明基数」——本票明确不收，留给将来有租户提出时另立。

## 完成判据

比例额度不带基数在构造门拒；带基数的比例额度经 ADR-0127 那条路到 SA 后折成金额进授信额度（真库用例一正一反）；受控批文 / 表单 / 词表 / 读面各一格；
ADR-0129 落文、PC CONTEXT 信用政策那句补「比例额度声明其基数」半句、README 一行；含 DSN 跑 PC 四包 + SA + `cmd/parcel-commercial` + `cmd/parcel-dispatch`；
admin-web tsc / run-tests 绿。

## 边界

不动 ADR-0127 正文（回指即可）；不动结算政策；不给 PC 合同加任何格。

## 完成记录（2026-09-09 23:2x，通道 3 五笔 + 通道 6 接续三笔；分支 `mcp3-wbr10` 基 `90c025ca`，每笔已推 origin 同 SHA——推送方重放进 main）

| 笔 | SHA | 作者 | 内容 |
|---|---|---|---|
| ① | `23d1f33c` | 通道 3 | 票面转 in-progress |
| ② | `b9f5a2b1` | 通道 3 | ADR-0129（封闭集 `CreditRatioBase` 两格 `POSTED_BALANCE` / `PRIOR_PERIOD_CONFIRMED_CHARGES`、每格 SA 取数路径、不给默认、缺席构造门拒、SA 折算与两格新 `NotFormedReason`、越权风险点五条）；PC CONTEXT 信用政策那句补「比例额度在政策正文里声明其基数」半句；`docs/adr/README.md` 加一行 |
| ③ | `5eb22c3e` | 通道 3 | PC：`NewCreditRatioLimit` 收基数、缺席或集外拒，`RehydrateCreditRatioLimit` 只为存量读回「未声明」；PCC-1 加键 `ratioBase` 不换号；`ViewRevision` 比例段带基数；迁移 `0029` 给 `0020` 加 `ratio_base`（可空只为存量行，CHECK 金额行必空、非空必在集合内）；postgres 写读往返、整册装载与闭包快照各加一键、重建门对 NULL 如实读回；发布路 `CreditPolicyBodyPayload` / 受控批文 `creditPolicyBodyDocument` 各加 `ratioBase`（缺席 / 集外 / 金额带基数点名 `creditPolicy.ratioBase`）；词表读口 `CREDIT_POLICY` 答 `ratioBase` 一集；读面信用政策册比例行带 `ratioBase`；机制清点随迁移重生成 |
| ④ | `43362615` | 通道 3 | SA：领域 `CreditRatioBase` 镜像封闭集、`CreditBasis` 比例格带基数（`NewCreditRatioBasis` 收基数、`NewUndeclaredCreditRatioBasis` 只为存量）、`LimitOnBase` 向下取整且负基数折 0；新端口 `CreditRatioBaseView` 三格照 ADR-0054；postgres 适配器 `CreditRatioBases` 读 `operational_balance.posted_minor` / 最近一张已发布对账单费用行之和（作废无替代答尚无事实）；`exposeCredit` 经 `authorizedMinorOf` 折算，`NotFormedReason` 加 `CREDIT_RATIO_BASE_UNAVAILABLE` / `CREDIT_RATIO_BASE_NOT_ESTABLISHED`，`CREDIT_RATIO_BASE_UNDECIDED` 只留给未声明基数的存量比例；`Deps` 新增 `RatioBases` mandatory；SA→PC 适配器逐格译基数、存量无基数译「未声明」；`cmd/parcel-dispatch` 接 `CreditRatioBases`；PS 夹具补 `ratioBaseDouble`；真库一正一反 |
| ⑤ | `f733374b` | 通道 3 | admin-web：信用政策发布表单加「比例的基数」一格（`VocabularySelect` 只从词表读口取码、不内置不预选），草稿 / 载荷 / 认领路径 / `*RenderedPaths` 各加 `creditPolicy.ratioBase`；读面比例行带基数（存量无基数示「基数未声明」）；`publication-draft-api.ts` / `api.ts` / `presentation.ts` 各加一格；node:test 197/197 |
| ⑥ | `a92526a0` | 通道 6 | 票面 Comments 记接续 |
| ⑦ | `a2e02e52` | 通道 6 | 机制清点在 `a92526a0` 干净检出重生成：settlementaccounting 生产 80→81 / 测试 60→61 / 端口 37→38（④ 的新端口与适配器一正一反），合计与端口声明数各 +1——③ 那次重生成早于 ④，所以 tip 上有差，单独一笔 |
| ⑧ | 本笔 | 通道 6 | 票面 → resolved + 本记录 |

**第 6 步 seed（无需改，理由）**：`scripts/demo-seeds/data/commercial/publish-batch.json` 的批文里没有 `CREDIT_POLICY` 项（八类册：`SERVICE_PRODUCT` / `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` / `ACCEPTANCE_RULE_PACKAGE` / `CUSTOMER_CONTRACT` / `SETTLEMENT_POLICY` / `SUPPLIER_AGREEMENT` / `PRICE_RULE` / `AUTHORIZATION_RULE`），整个 `scripts/demo-seeds` 也无信用政策——没有一处要加 `ratioBase`；PCC-1 加键 `omitempty` 之后金额正文的规范化文档一字不变（ADR-0129 决定一），既有 seed 摘要不受影响，`cmd/parcel-commercial` 带 DSN 的用例在本 tip 上 ok。给 seed 补一条信用政策不在本票判据内，属合成 `S` 证据另议。

**触及文件**（48 件，+1638/−150，`git diff --stat 90c025ca..a2e02e52`）：PC `domain/{credit_limit.go,commercial_registry.go,publication_canonicalization.go,publication_vocabulary.go}`、`ports/ports.go`、`adapters/postgres/{credit_policy.go,commercial_resolution.go,commercial_publication.go}`、`adapters/http/{publication_draft_payload.go,query_commercial_policies.go}`、`migrations/party_commercial/0029_credit_policy_ratio_base.sql`（新）；SA `domain/{credit_basis.go,credit_exposure.go}`、`ports/ports.go`、`application/apply_pre_acceptance_control.go`、`adapters/partycommercial/credit_basis.go`、`adapters/postgres/{credit_ratio_base.go（新）,pre_acceptance_control.go}`；`cmd/parcel-commercial/translate.go`、`cmd/parcel-dispatch/assemble.go`；PS `adapters/settlementaccounting/pre_acceptance_control_test.go`（只加替身，不改既有断言）；admin-web `party/` 八件；ADR-0129、README 一行、PC CONTEXT 半句、清点、本票。**未碰**：ADR-0127 正文；结算政策任何一格；PC 合同任何一格；SA 迁移（两格基数都读既有两表）。

**验收对照**（票面完成判据逐项）：比例额度不带基数在构造门拒 ✓（③ `NewCreditRatioLimit`，集外 / 缺席 `ErrInvalidCreditLimit`；金额带基数拒）；带基数的比例额度经 ADR-0127 那条路到 SA 后折成金额进授信额度 ✓（④ `LimitOnBase` → `WithAuthorizedLimit`；真库一正一反在 `credit_ratio_base_test.go`）；受控批文 / 表单 / 词表 / 读面各一格 ✓（③ 批文与载荷、词表一集、读面比例行；⑤ 表单）；ADR-0129 落文 ✓、PC CONTEXT 半句 ✓、README 一行 ✓（②）；含 DSN 跑 PC 四包 + SA + `cmd/parcel-commercial` + `cmd/parcel-dispatch` ✓（作者 ④ 前后跑过 PC postgres + migrations 与 SA postgres 带 DSN 各一轮并广播；接续在 tip 跑 `cmd/*` 三包带 DSN，见下）；admin-web tsc / run-tests 绿 ✓（⑤ 与接续各跑一次）。边界三条 ✓。

**验证强度**（接续，树 `D:/tops/idp-parcel-mcp3-wbr10` 钉 `a92526a0`，代码 tip `f733374b`，`status --untracked-files=all` 空）：`gofmt -l ./internal ./cmd ./migrations ./tools` 零输出；`go build ./...`、`go vet ./...` 全仓退 0；无 DSN `go test -count=1` PC 四包（domain / application / adapters/postgres / adapters/http）+ SA 四包（domain / application / adapters/postgres / adapters/partycommercial）+ `./internal/parcelshipment/adapters/settlementaccounting/` + `./internal/architecture/...`：10 ok；**含 DSN** `go test -p 1 -count=1 -v ./cmd/parcel-commercial/ ./cmd/parcel-dispatch/... ./cmd/parcel-api/...`：3 ok，`--- PASS` 292 / `--- SKIP` 0 / `--- FAIL` 0（占号 / 释号各广播一次）；admin-web `tsc -b --force` 0 错、`run-tests` 197/197；清点在 `a92526a0` 干净 detached 树重生成有差 → 落 ⑦，⑦ 之后再生成即零差（推送方在 tip 兑底）。未跑全量（作者范围口径）、未跑 `-race`。PC postgres + migrations、SA postgres 带 DSN 沿用作者 22:5x–23:0x 两轮广播的结果，接续未重跑。日志 `%TEMP%\wbr10-cmd-dsn.log`，仓内无残留。

**与 main（`dec37d78`）碰面**（`git merge-tree --write-tree origin/main mcp3-wbr10` 干跑，未动树、未合）：两侧都改的只有两件——`internal/partycommercial/adapters/http/publication_draft_payload.go` 自动合并干净（awf/23 加在 `Publication` 结算政策块、本票改 `CreditPolicyBodyPayload` 两块，不相邻）；`docs/adr/README.md` 内容冲突，是同点插入（main 已 0128 → 0130 → 0132，本票 0129 行）——按号序插进 0128 与 0130 之间即解，无语义冲突。PC CONTEXT 两侧不同期改动（awf/23 那句在 `90c025ca` 之前已进 main），无重叠。

**判断题**（给评审与推送方，都不阻断）：

1. **地盘外两处小改**（作者 ④，广播过、无人喉）：`cmd/parcel-dispatch/assemble.go` 接 `NewCreditRatioBases` 并给 `Deps.RatioBases` 一项；PS 夹具 `internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control_test.go` 补 `ratioBaseDouble` 接进三处构造，只加不改既有断言。理由：`Deps` 新增 mandatory 项（ADR-0127 决定五 contract 纪律，票 09 已收齐、本票不再开 expand 段），缺任何一件构造门就拒，两处不同笔接就编不过。
2. **票面候选 `DEPOSIT_BALANCE` 改名 `POSTED_BALANCE`**（ADR-0129 决定二）：SA 没有「保证金」格、「预付余额」不是运营结算余额五项之一；术语跟着 SA 算得出的那一项走。
3. **`PRIOR_PERIOD_CONFIRMED_CHARGES` 三处裁定**（ADR-0129 风险点 ③）：对账单快照作周期代理；对账单按（账户、币种）键入、责任法人由账户蕴含；费用行合计不含调整行（代码上 `lines` 列只存费用行、调整行在 `adjustment_lines` 另一列，求和口径与注释一致）。
4. **负入账余额折 0 进`业务限制`而不是停`待判断`**（ADR-0129 风险点 ④）。
5. **两格新 `NotFormedReason`**（`CREDIT_RATIO_BASE_UNAVAILABLE` / `CREDIT_RATIO_BASE_NOT_ESTABLISHED`）未经产品单独裁，与 ADR-0127 三格同性质。
6. **最近一张对账单已作废且尚无替代 → 答尚无事实**而不是退回更早一张（`priorPeriodConfirmedChargesBase` 头注）：退回会静默拿更早周期的数当分母。

**wbr/09 评审三条非阻断的顺手收落**：全部落在 ④ `43362615`——(a) `CreditPolicyReference.IsZero()` 已加，三处零值判法全改——`WithAuthorizedLimit` 用 `policy.IsZero()`、`Expose` 用 `standing.Policy().IsZero()`、postgres `Save` 用 `!exposure.Policy().IsZero()`；(b) `ApplyPreAcceptanceControlDeps` 头注重写为「每一件都是 mandatory…」，「它曾经允许为 nil」那句变更史已删；(c) `CreditStanding.Policy()` 自 `Expose` 改用它起有了生产调用。

**接续方看到的**（只记不改，归评审与下一张票）：读过 ④ 的 `credit_ratio_base.go` 全文与 `apply_pre_acceptance_control.go` 的 `Deps` 段，未读出语义问题；`priorPeriodConfirmedChargesBase` 的查询不带 `legal_entity`，与 ADR-0129 风险点 ③「责任法人由账户蕴含」一致——若 owner 判对账单该按作用域四维键入，改的是那一条与对账单表的键，不在本票。②③④⑤ 的其余文件接续方未逐行读，评审以代码为准。

**父 spec**：`wiring-baseline-remainder/spec.md` 状态行不由本票改。

## Comments

- 2026-09-09 23:1x · 通道 6 接续（作者会话通道 3 崩，`list_sessions` offline）：从 `f733374b` 接着做第 6–7 步（seed 核对 / 最后一轮反向依赖 `cmd/*` 带 DSN / 清点核零差 / 票面 resolved + 完成记录），单 task-431cc4a0-5f54-4d9f-865a-c5ca637fd301。只加笔不改作者五笔；接续方读出的语义问题写「接续方看到的」一节，不自改。
- 2026-09-10 10:49 · 评审 ← 通道 5 · 钉 `87e3cfc0`（隔离检出 `%TEMP%\idp-review-wbr10`，基 `90c025ca`，八笔 48 件；只读、未跑任何测试、未碰作者树；派单 task-fecd651a，10:46 派 → 10:49 交）。**Standards** — 阻断：无。非阻断：① `internal/settlementaccounting/adapters/postgres/credit_ratio_base.go` `priorPeriodConfirmedChargesBase`：`ORDER BY published_at DESC, statement_number DESC`——0002 的 CHECK 允许 `voided_at = published_at`，替代单与被作废单同一时刻发布时靠 `statement_number` 字串序决胜，与头注「替代单 published_at 更晚、自然成为最近一张」的前提不全同构；无租户、边缘。② `internal/partycommercial/domain/publication_canonicalization.go` `creditLimitOf`：`RehydratePublicationContent` 读回的是受控批文暂存载体（ADR-0126 pending carrier），走构造门不走重建门，0129 之前暂存的比例正文自此读不回；与该函数既有头注「每一格都过领域构造门」一致，但 ADR-0129 Consequences 只列了 0020 行与闭包快照两种存量。无发现：注释全中文；新增行无行号 / 计数引用；PC / SA domain 未引 HTTP / pgx；SA `CreditRatioBase` 为镜像不 import PC，`creditRatioBaseFrom` 全函数、集外 `ErrUntranslatableAnswer` 不吸收（ADR-0025）；0029 CHECK 叠 0020 `credit_policy_limit_exactly_one` 恰为「金额行必空、非空必在集合内」；词表 `closedCodes(CreditRatioBase.valid, …)` 从接受判据逐值列出；表单 `emptyCreditPolicyDraft().ratioBase=''`、码只从 `ratioBaseCodesOf(词表)` 来。**Spec** — 阻断：无。非阻断：① ④ `43362615` 顺手收落 wbr/09 三条不在本票完成判据内，行为不变、票面已如实登记。无发现：完成判据逐项 ✓（构造门缺席 / 集外 / 负比例同答 `ErrInvalidCreditLimit`；金额带基数在 http `CreditPolicyBodyPayload.body()`、`cmd/parcel-commercial` `creditLimitFrom`、canonical `creditLimitOf`、postgres `creditLimitFrom` 四口各自拒；`exposeCredit → authorizedMinorOf → LimitOnBase → WithAuthorizedLimit` 取基数在授信依据之后、信用状况之前；批文 / 载荷 / 词表 / 读面各一格；ADR-0129、PC CONTEXT 半句、README 一行在 diffstat）；边界三条 ✓；`CREDIT_RATIO_BASE_UNDECIDED` 只剩 `NewUndeclaredCreditRatioBasis` ← `creditBasisFrom` ← `RehydrateCreditRatioLimit` 一条来路 ✓；`RehydrateCreditRatioLimit` 对 NULL 如实读回、非空集外仍拒 ✓。**判断题六道**全部同意（1 contract 纪律不开 expand；2 `POSTED_BALANCE` 是 SA CONTEXT 运营结算余额原词；3 `lines` / `adjustment_lines` 分列、`customer_statement` 表本无 `legal_entity` 列；4 「不折 0 不折无限」修饰的是尚无事实，负基数是已成立的事实，`LimitOnBase` 只在 `established` 之后被调；5 与 ADR-0127 三格同性质；6 作废不退回更早周期，同刻决胜见 Standards ①）。结论：两轴无阻断，可重放。
- 2026-09-10 10:5x · 进 main 记录（通道 1 推送方）：八笔在 `%TEMP%\idp-replay-wbr10` 重放到 `160a1def` 之上（重放本身由前一任 09-09 23:36–23:38 做好、未 ff 未推、无评审记录；本任核过再用），分支 → main：`23d1f33c`→`2583e6d5` / `b9f5a2b1`→`ca597f57` / `5eb22c3e`→`f9d81b27` / `43362615`→`d2c51b3a` / `f733374b`→`d6a2dcae` / `a92526a0`→`5d5665ab` / `a2e02e52`→`cfcc49a2` / `87e3cfc0`→`26e64d1c`。`patch-id --stable` 七对相等，`b9f5a2b1` 那对不等只因 README / PC CONTEXT 上下文已含 main 的 0131 / 0133 行与「交付条件」词条；`git diff 87e3cfc0 26e64d1c` 全树只 3 件 +10/−0，全是 main 自己的（README 4、PC CONTEXT 5、`publication_draft_payload.go` awf/23 那一行——两侧都改、hunk 不相邻、自动合并）；README 0127→0133 号序对。验证（隔离树钉 `26e64d1c`）：`gofmt -l` 零、`go build` / `go vet` 全仓 0；机制清点在 tip 重生成零差（无需清点笔）；**含 DSN** `go test -p 1 -count=1 -v ./...`：102 ok / 0 FAIL，`--- PASS` 7839 / `--- SKIP` 1（仅 `pgtest.TestHelperTemplateOwnerProcess`，设计上只由子进程驱动）/ `--- FAIL` 0，122 s，探针 `TestFreezeScopesAreInvisibleToEachOther` PASS；admin-web（junction 主树 `node_modules`）`tsc -b --force` 0 错、`run-tests` 198/198（main 那侧 awf/23 多一条，故 197→198），产物已清。10:5x `merge --ff-only` 共享 main `160a1def`→`26e64d1c`，`git push origin 26e64d1c:main`，`ls-remote` 核 = `26e64d1c`。**非阻断处置**：Standards ①（同刻决胜）与 ②（ADR-0129 Consequences 漏列 pending carrier 存量）随票记，不改 ADR 正文、不另立票——无租户，两格都只在存量上成立，owner 若判要补由 SA / PC owner 各补一句；Spec ① 票外顺手活已在完成记录登记，不动。分支 `mcp3-wbr10` → `merged/mcp3-wbr10`（48 件对 main 只差 main 自己多出的加行），远端 `mcp3-wbr10` 删；树 `D:/tops/idp-parcel-mcp3-wbr10`（先 `rmdir` 掉 `node_modules` junction、清 `.tmp-test` / `tsbuildinfo`）与 `%TEMP%\idp-replay-wbr10` 拆，均未加 `--force`。
