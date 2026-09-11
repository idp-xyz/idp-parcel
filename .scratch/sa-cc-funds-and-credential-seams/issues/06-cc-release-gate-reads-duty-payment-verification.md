# 放行门禁核对不读税费付款核对：`VerifyReleaseGate` 的依赖里没有 `DutyVerificationStore`，「税费付款」那一道门禁今天只能由调用方口头交进来

Category: enhancement
Status: resolved——2026-09-11 18:4x **进 main**（通道 1 推送方重放：main 上代码 `612e9afb`、票面 `a89cd823`、推送方清点 `9c0b7223`；非作者评审 ← 通道 1 推送方自跑两轴 0 阻断，Standards 3 / Spec 3 非阻断随票记，见 Comments）；此前 resolved——2026-09-11 17:1x 通道 4 完工，等非作者评审进 main（分支 `mcp4-sacc06`，基 main `620f7fed`，代码 tip `455cacbd`；迁移钉 `0019_duty_payment_gate_rule_and_reading.sql`；完成记录见文末）；此前 in-progress——2026-09-11 16:4x 通道 4 接单 task-18848394，树 `D:/tops/idp-parcel-mcp4-sacc06`；此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁，CC owner 口径）写入裁决：折法是登记进来的规则（三态各自接受集合，无默认；[ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定三）、门禁记录加一列核对版本引用（见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无

## 缺口（取证于 `3f485e97`）

- `internal/customscompliance/application/verify_release_gate.go` 四格结果 `GateVerificationRecorded / Existing / NotAccepted / Undecided` 在；`git grep -n DutyVerificationStore -- internal/customscompliance/application/verify_release_gate.go` **零**——门禁核对不读付款核对。
- 付款核对已有登记面：`ports.DutyVerificationStore`（mech/07 CC-c，`616646d`），`VerifyPayment` 写它。
- mech/07「没做」第 3 条后半：「步 10 门禁核对读付款核对——各一张」。

## 语言从哪里来

- CC `CONTEXT.md`：「放行门禁核对必须绑定当前有效的监管程序、明确申报范围、拟执行动作、适用监管边界、**税费付款核对**、限制、处置和其他前置条件判断。门禁满足不生成放行，也不能复用于其他动作或监管边界；门禁未满足也不能删除已经接收的放行结果。」
- UC-CC-009 范围节第 6 条：「按申报范围、拟执行动作和监管边界形成税费付款、限制、处置和其他前置条件的放行门禁核对，以及 `UC-CC-006` 外部放行回接。」

## 做法

1. `VerifyReleaseGateDeps` 加 `DutyVerifications ports.DutyVerificationStore`（读半边）；门禁核对形成时按（租户、申报范围、监管程序）取**当前**付款核对版本，把覆盖 / 差额 / 有效性三态折成「税费付款」那一道门禁的满足与否，并把核对版本引用记进门禁记录。
2. 没有付款核对 → 那一道门禁**未决**并指名等谁（不是「未满足」也不是「满足」）；三态里任一为「待确认 / 冲突」 → 同样未决。
3. 折法（哪些三态组合算满足）**不在代码里写死**——见「要裁的」第 1 条；裁前只做「有核对 → 记引用，无核对 → 未决」。

## 红线

- 门禁满足不生成放行（CONTEXT）；本票不碰 `receive_external_result.go`。
- 三态不得压成一组互斥总状态（CONTEXT「覆盖状态、差额状态和有效性状态分别表达」）——门禁记录带三态原值 + 引用，不带一个合成布尔。
- 真实程序的付款条件（何种差额可放行）属实例半边 `PAR-CUS-0x`，不写默认。

## 完成判据

1. `VerifyReleaseGate` 有读付款核对的路径：有核对 → 门禁记录带核对版本引用；无核对 → `GateVerificationUndecided` 并指名。
2. 应用层用例覆盖：有 / 无 / 待确认三条。
3. 真库：门禁记录往返带引用列（若要加列，新迁移序号在票面写明）。

## 地盘

`internal/customscompliance/application/verify_release_gate.go`、`internal/customscompliance/domain/`（`ReleaseGateVerification` 若要多一格引用）、`internal/customscompliance/adapters/postgres/`、`migrations/customs_compliance/`（若加列）。

## 要裁的

1. **三态怎么折成一道门禁**：（已覆盖 · 无差额 · 有效）才算满足，还是「超额」也算、「部分覆盖」按真实程序定——CONTEXT 只说分别表达，没说门禁怎么读；真实规则是实例半边，机制半边要裁的是「折法是登记进来的规则（门禁目录一行）还是编排常量」。归 CC owner。
2. **门禁记录要不要多一列核对版本引用**：ADR 层面是「引用还是快照」；本票倾向引用（CC 自己的表，同上下文内引用不违反 ADR-0013）。归 CC owner。

### 裁决

（1 由用户 17:0x 授权、通道 5 按 CC owner 口径代裁并落 [ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定三；2 是 A 类由通道 1 推送方裁；通道 5 写入，2026-09-10 17:2x；task-b941ce87 分类、task-9a2ff746 落笔。拿不准的在 ADR「越权风险点」3 / 4。）

- **1 → 折法是登记进来的规则，不是编排常量；规则挂门禁目录既有登记册（`GateConditionRegistry` 那一族，范围 / 动作 / 边界三维键），这一道的目录行登规则而不登结论性认定。** 规则正文两种形之一：「税费付款不构成本动作在本边界的前置条件」，或三个接受集合——覆盖 ⊆ {无覆盖, 部分覆盖, 已覆盖}、差额 ⊆ {无差额, 不足, 超额}、有效性 ⊆ {有效, 失效}，三态各落在自己的接受集合内才满足。`待确认` / `冲突` 不可登记为接受，三态任一为它们时该道门禁未决并指名；没有付款核对版本同样未决；目录里没有这一道的规则行 → 答「规则未配置」诚实停点，不取任何默认折法。理由：CC CONTEXT「税费支付是否是放行前置条件，取决于当前监管程序的适用规则；本上下文不得统一假设『先税后放』或『先放后税』」与 AGENTS 红线「未确认参数保持可配置或显式未决」同时排除常量；「三态分别表达」→ 门禁记录带三态原值不带合成布尔。规则取值属实例半边 `PAR-CUS-0x`。做法 3「裁前只做有核对记引用 / 无核对未决」由此改为「有规则且有核对 → 按规则折；无规则 → 规则未配置；无核对 → 未决」。
- **2 → 加一列核对版本引用（引用，不快照）。** 同上下文内引用，[ADR-0013](../../../docs/adr/0013-pricing-owns-versioned-external-reference-series.md) 的「引用 vs 快照」判据只约束跨上下文的外部数值序列；本票红线本已写「门禁记录带三态原值 + 引用」。新迁移序号开工时重取、票面写明。

## 参照

[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md)「没做」第 3 条；UC-CC-009；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-8 ④；`customs-gate-conditions` 读面（`cmd/parcel-api/endpoints.go`，门禁目录已有读口）。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
- 2026-09-11 16:4x · 通道 4：接单 task-18848394 开工，基 main `620f7fed`，按「裁决」1 / 2 实现（规则登目录行、门禁记录带三态原值 + 核对版本引用、迁移钉 0019）；本笔只改 Status。
- 2026-09-11 17:1x · 通道 4：完工，Status → resolved，完成记录见下。两轴评审子代理本会话仍是「Authentication error」（同 04），作者按 `/code-review` 正文串行自跑两遍、结果随记；**不顶替非作者评审**。
- 2026-09-11 18:2x · **评审 ← 通道 1**（推送方自跑：通道 3 那单 17:48 报未开工，三路评审子代理都是「Authentication error」，按 `/code-review` 正文串行两轴、互不见结论）· 钉 `2747ee94`（基线 `620f7fed`）。
  - **Standards——阻断 0，非阻断 3**。① 两处 CONTEXT 引文不是原句：`domain/duty_payment_gate_rule.go` 包头注与 `0019_*.sql` 头注引「……不得统一假设『先税后放』或『先放后税』」，CONTEXT 原文内层是“先税后放”（弯双引号），整句搜不中；`DutyPaymentGateReading` 头注与 0019 头注引「覆盖状态、差额状态和有效性状态分别表达，不能实现为一组互斥总状态」，原句三个状态各带括注取值、去掉括注又无省略号，是转述——sa-cc/15 评审同族「引文要能被搜到」，归 CC owner 随 15 后继一并收。② `adapters/postgres/case_restriction_gate.go` `attachTo` 头注「把七列折回读数」数的是 0019 的列（同文件 `dutyReadingColumns` 恰好也七个字段，判断题）；作者 `455cacbd` 收了其余「七列」漏这一处。③ `gate_verification.findings_digest` 列从此存的是 `GateVersionDigest`（逐项判断 + 税费付款读数），列名不再描述内容——Mysterious Name 判断题，改列名要迁移，不值当下改，`Save` 头注点一句即可。另记不算违反：`NewRegisterCaseConfigurationHandler` 沿旧形不拒 nil，新增 `DutyRules` / `DutyRuleView` 两口同样不拒——派单红线只约束新增构造器，且两处组合根都已接上。核过：领域包 import 只有 `errors` / `sort`；注释中文；夹具全 `SYN-`；新增行无行号引用、无跨包计数。
  - **Spec——阻断 0，非阻断 3**。① 做法 1「按（租户、申报范围、监管程序）取当前付款核对版本」实做按（租户、范围）——核对册身份无程序维，`CurrentDutyVerificationView` 头注如实说明；票面那第三维是立票时的误写，不是漏做。② `RegisterGateFinding` 对 `DutyPaymentPrecondition` 关门是票面未点名的行为变化；ADR-0137 决定三「这一道的目录行不再由人登结论性的认定」直接支持，接受；但它与空字段同答 `ConfigurationNotAccepted`、无独立理由格，登记面日后接上时调用方分不出「拒的是这一道」——可另立小票。③ 编排现在对每一次门禁核对都先取这一道的规则行，目录在而规则缺一律停在 `DutyPaymentGateRuleNotConfigured`，而规则今天没有任何登记面（无 CLI / 端点 / 管理台）——与 ADR「没登时门禁答规则未配置停下，不猜」一致，且 `VerifyReleaseGate` 今天无生产装配点，无生产影响；作者判断项已如实记，登记面归 07 家族。完成判据 1–3 逐项对上：有核对记 `DutyVerificationReference`（`Version` = 核对指纹）、无核对 `DutyVerificationAbsent`、待确认 / 冲突 `DutyVerificationPending`；用例三条在且多七个停点表；迁移 0019 往返用例在。红线三条（不碰 `receive_external_result.go`、三态原值不压成布尔、不写真实付款条件）零违反；越界零（`apps/` 零改动、`RegisterReadiness` 形未动）。
- 2026-09-11 18:4x · 通道 1 推送方 · **进 main 记录**。重放在 `%TEMP%\idp-replay-w5c` detached `45b9c468`（原通道 1 17:1x 做、本会话接管后核）：`810fbd09→ac6471d4` / `23b7f671→92161d31` / `da4d8369→4136b6e1` / `664116c9→22e99fee` / `99608e4b→76a7fdd0` / `455cacbd→612e9afb` / `2747ee94→a89cd823`；作者清点 `d4f1a1e6` 在旧底座 `620f7fed` 生成、跳过不 pick，推送方清点 `9c0b7223` 兑齐。sa-cc/15 九笔叠在其上（`f061b834`…`af40abf7`），带 DSN 全量在含 15 的 tip `af40abf7` 跑一次（110 ok / 0 FAIL / 0 cached；探针 `ReleaseGate|DutyPaymentGateRule|CredentialGate` PASS 27 / SKIP 0）；main 快进到簿记笔，SHA 在推后广播。

## 完成记录（通道 4，2026-09-11）

### 逐笔 SHA（分支 `mcp4-sacc06`，基 main `620f7fed`）

| 笔 | SHA | 内容 |
|---|---|---|
| 1 | `810fbd09` | 票面 Status → in-progress |
| 2 | `23b7f671` | **顺手 · 04 评审 S①**：`ports.CredentialGateDigest` 结论以 `String()` 封闭词入指纹，不再用枚举整数 |
| 3 | `da4d8369` | domain `duty_payment_gate_rule.go`（`DutyPaymentGateRule` 两形、`Judge`、`DutyPaymentGateReading`、`DutyVerificationReference`、`DutyPaymentPrecondition`、`ReleaseGateVerification.WithDutyPayment` / `DutyPayment`）+ ports（`DutyPaymentGateRuleRegistry` / `DutyPaymentGateRuleView`、`CurrentDutyVerificationView`、`GateVersionDigest`、`GateConditionCatalogueEntry.DutyPaymentRule`）+ application（`VerifyReleaseGate` 改读规则与当前核对、`VerifyGateUndecidedReason`、构造门拒 nil；`RegisterDutyPaymentGateRule`；`RegisterGateFinding` 对这一道关门）与三份用例 |
| 4 | `664116c9` | 迁移 `0019_duty_payment_gate_rule_and_reading.sql` + postgres `duty_payment_gate_rule.go`（规则写读、当前核对读）、`case_restriction_gate.go`（门禁记录读数各列）、`gate_condition_catalogue.go`（上列带规则）+ 真库用例 |
| 5 | `99608e4b` | `cmd/parcel-api/assemble_customs_registration.go`、`cmd/parcel-customs-register/main.go`：组合根填 `DutyRules` / `DutyRuleView`（不加端点、不加子命令、不写默认规则） |
| 6 | `d4f1a1e6` | 机制清点在 `99608e4b` 干净检出重生成（CC 生产 88→90 / 测试 88→91、postgres 适配器 39→40、迁移 165→166、端口声明 397→400；缺口两口径不变）；`455cacbd` 上复跑零差 |
| 7 | `455cacbd` | 两轴自评修：规则集合编解码抽泛型、门禁指纹两函数共用行、注释去跨文件计数（零行为） |

代码 tip = 分支 tip = `455cacbd`（本票面笔在其后）。**没动**：`receive_external_result.go`、`RegisterReadiness` 的形、`parcel-api` 端点表、`apps/`、`internal/architecture/*baseline.txt`。

### 完成判据逐项（按裁决改读）

1. **`VerifyReleaseGate` 有读付款核对的路径：有核对 → 门禁记录带核对版本引用；无核对 → 未决并指名**——✓。`VerifyReleaseGateDeps` 多 `DutyRules`（规则读半边）与 `DutyVerifications`（`CurrentDutyVerificationView`，按租户 + 范围取当前版）；编排：取规则 → 规则要读核对则取当前版 → `rule.Judge` 折三态 → 这一道以 `DutyPaymentPrecondition` 进清单 → 领域折叠 → 记录 `WithDutyPayment`（三态原值 + `DutyVerificationReference{Duty, Funds, Version=核对指纹}`）。停点各有名（`VerifyGateUndecidedReason`）：`DutyPaymentGateRuleNotConfigured`（规则未配置）/ `DutyVerificationAbsent`（无核对）/ `DutyVerificationPending`（三态待确认或冲突）/ `DutyPaymentFindingRegisteredBesideRule`（认定与规则并存）；都不是「未满足」，都不入册。
2. **应用层用例覆盖有 / 无 / 待确认三条**——✓ 并多几条：`TestTheDutyPaymentGateIsJudgedByTheRegisteredRuleAndRecordedByReference`（有核对、满足、带引用；核对换版门禁另成一版）、`TestTheDutyPaymentGateAnswersUnmetByRuleAndIsSkippedWhenNotAPrecondition`（不在集内 → 未满足；不构成前置条件 → 不读核对不挂读数）、`TestTheDutyPaymentGateStopsHonestlyInsteadOfAnsweringUnmet`（规则未配置 / 规则读口故障 / 无核对 / 核对读口故障 / 差额待确认 / 有效性冲突 / 认定与规则并存七个停点）、构造门表。规则登记面 `register_duty_payment_gate_rule_test.go`：两形登记、重放 / 冲突、受理门（两形互斥、空集、待确认、冲突、集外）、依赖故障、这一道拒认定。
3. **真库：门禁记录往返带引用列**——✓ `TestAGateVerificationRoundTripsItsDutyPaymentReadingByReference`（读数各列往返、核对换版另行首版不改、无读数版本各列空）；迁移 **`migrations/customs_compliance/0019_duty_payment_gate_rule_and_reading.sql`**：`gate_condition_duty_payment_rule` 新表（与目录同键、外键到目录行）+ `gate_verification` 加 `duty_state / duty_coverage / duty_delta / duty_validity / duty_ref / funds_ref / duty_version_digest`（同生同灭 CHECK、封闭词 CHECK；PENDING / CONFLICTING 进不来）。另七条真库用例见笔 4。

### 判断项（供评审）

- **「当前版本」怎么定**：`CurrentDutyVerificationView.LoadCurrentDutyVerification(tenant, scope)` 取 `verified_at DESC, version_digest ASC` 第一行——核对时刻最新那版，不按到达顺序、不按指纹；同刻并存按指纹字典序取定，让「当前」在同一份数据上只有一个答案。**不按监管边界过滤**：付款核对的身份是税费版本 / 资金事实 / 范围三维，表上没有程序列，边界在门禁自己的键上（做法 1 写的「按（租户、申报范围、监管程序）」里那第三维在核对册上不存在，如实记）。
- **规则行落在目录哪一格**：新表 `gate_condition_duty_payment_rule`，主键 = 门禁目录三维键、外键到 `gate_condition_catalog`——同一册的第二张表（0008 已是「目录表 + 认定明细表」两张，这是第三张），一个目录行至多一条规则；不改 `gate_condition_finding` 的形（既有认定行一字不变，ADR-0137 Consequences 那句）。规则正文：`not_a_precondition` + 三个 jsonb 数组接受集合，CHECK 两形互斥、各轴词封闭（`accept_delta <@ '["NO_DELTA","SHORT","EXCESS"]'` 等）。
- **这一道的引用是机制常量 `DutyPaymentPrecondition = "DUTY_PAYMENT"`**：规则行按目录键落，没有登记方给的前置条件引用；这一道进清单要一个名字，取机制侧固定名（CONTEXT 把「税费付款核对」点名为门禁绑定的前置条件之一），不是实例参数。由此 `RegisterGateFinding` 对这个引用关门（ADR-0137 决定三「不再由人登结论性的认定」），编排遇到册上认定与规则并存停下（`DutyPaymentFindingRegisteredBesideRule`）而不选边。
- **读数上带 `State`（MET / UNMET）算不算「合成布尔」**：它是规则折出的这一道认定（ADR 说的「编排拿当前付款核对版本对着规则折出认定」），与三态原值并列而不是替代——三态原值三列各自在册（CONTEXT「分别表达」），`State` 是 `FoldGateConclusion` 要吃的那一格。若 owner 认为不该落库，去掉 `duty_state` 一列即可，编排不变。
- **读数进门禁指纹**（`GateVersionDigest`）：核对换版而折出的判断不变时，门禁另成一版指向新引用——否则记录里的引用会永远指向首版核对。没挂读数时与 `FindingsDigest` 逐字节相同，旧行照旧能撞上。`FindingsDigest` 自身仍以枚举整数入行（本册既有指纹，改会让已入库门禁版本全部换指纹），与 04 S① 的改法不一致，如实记；要改另起一票同步。
- **「不构成前置条件」那一形**：不读核对、这一道不进清单、不挂读数；只登目录 + 这一形且无其它认定 → 空清单 → `不适用`（此动作在此边界本就不受门禁），与既有折叠一致。
- **登记面**：规则行今天只有 application 口 `RegisterDutyPaymentGateRule` 与组合根接线，**没有 CLI 子命令、没有端点、没有管理台**——「规则未配置」停点在生产里今天没有解法，与 04 的装配缺口同类，如实记；登记面归 07 家族另票（票面「地盘」未列 CLI / 端点；派单写「若要多收这一格写清不写默认」，本票没收）。
- `NewVerifyReleaseGateHandler` 改为 `(*Handler, error)` 构造期拒 nil（派单红线）；全仓生产无调用方（与 04 同：本编排今天无装配点），只改用例。

### 两轴自评（`/code-review`，基线 `620f7fed`，作者串行自跑，不顶替非作者评审）

**Standards**：① `dutyRuleSetsJSON` / `rebuildDutyPaymentGateRule` 三轴各写一遍编解码循环、`GateVersionDigest` 与 `FindingsDigest` 各拼一遍行——Duplicated Code，已修（`455cacbd`：泛型 `closedWords` / `closedSet`、共用 `findingDigestLines`）；② postgres 适配器与用例注释「七列」数的是迁移里的列——AGENTS「不数别处的东西」，已修（同笔）。其余：注释中文；`verify_release_gate.go` / `register_case_configuration.go` 原有的「硬句 216」两处顺手换成引文、未新添；`VerifyGateUndecidedReason.String()` 九格齐（enum 门禁绿）；PBC08 负向证据 `TestDutyPaymentGateRuleWritesRefuseToRunOutsideATransaction` 在；`WithinTransaction` 闭包内无 `t.Fatal`；夹具 `SYN-` / `syn-`（顺手把 `verify_release_gate_test.go` 里 `US-IMPORT/TYPE-86` 换成 `SYN-PROC-IMPORT`）。0 未修。

**Spec**：① 做法 1「按（租户、申报范围、监管程序）取当前版」——核对册无程序维，按（租户、范围）取，非阻断如实记（判断项首条）；② 规则登记面缺席（判断项末二条），非阻断；③ `RegisterGateFinding` 对 `DUTY_PAYMENT` 关门是票面没点名的行为变化（ADR-0137 决定三的直接推论），非阻断供评。缺失 0、越界 0（`apps/` 零改动、`receive_external_result.go` 零改动、PP / SA 零改动）。

### 验证（tip `455cacbd`）

- `gofmt -l internal/customscompliance cmd` 空；`go build ./...`、`go vet ./internal/customscompliance/... ./cmd/parcel-api/ ./cmd/parcel-customs-register/` 退 0。
- 带 DSN（55432 占号 / 释号各两轮）：`go test -count=1 -p 1 ./internal/customscompliance/... ./internal/architecture/... ./migrations/... ./cmd/...` 全绿 0 FAIL；新 postgres 用例 `-v` 7 PASS 0 SKIP；迁移 0019 首施加成功。不跑全量（派单如此）。
- 新 `.go` 全部 `gofmt -w` 后入库、`git ls-files --eol` 皆 `i/lf w/lf`；`0019_*.sql` CR 数 0、无 BOM，`migrations` EOL 守卫绿。
