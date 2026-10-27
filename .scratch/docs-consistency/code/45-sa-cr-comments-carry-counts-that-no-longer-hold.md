# 45 SA、CR 代码注释里的计数与实际不符，一处数的是 CONTEXT 的条目

Category: chore
Status: resolved——2026-10-27 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-cmtB
Blocked by: 无（`collectionremittance` 那两处与 [14](./14-collection-principal-posts-more-than-once-and-shortfall-moves-received-funds.md) 改同一组文件，可并进 14 一起改）
出处：[audit/sa-cr.md](../audit/sa-cr.md) L9（归 A，但改的是 Go 注释，修订票不改代码，故立本票）。取证于 main `899a4555`，下列各处原样仍在。

## 现象

- `cmd/parcel-collection-register/main.go` 包注释「七个命令覆盖 CONTEXT 的五个词形加分户账两面」：数的是另一份文件的条目（AGENTS.md「改文档」明禁跨文件计数），collection-remittance CONTEXT 的 Language 实际立了九个词条。
- `internal/collectionremittance/domain/subledger.go` 的 `PostingBasisKind` 注释「封闭四值」：实际五值（`COLLECTION_FACT`、`ALLOCATION`、`REMITTANCE_BATCH`、`DISCREPANCY`、`CORRECTION`）。
- `internal/settlementaccounting/domain/evaluation_resumption.go` 的 `StopPendingJudgmentCauseUnrecorded` 注释「所以三类登记都解得开它」：`ResumedBy` 对它列出价卡、序列、目录、映射四类登记，与 ADR-0229 一致，注释少数了一类。
- `internal/settlementaccounting/application/audit_supplier_bill.go` 的 `AuditUndecidedReason` 注释「两格未配置都是实例半边」：实际有 `AUDIT_AUTHORITY_UNCONFIGURED`、`PAYABLE_ACCOUNT_UNCONFIGURED`、`AUDIT_ESCALATION_UNCONFIGURED`，最后一格是 ADR-0163 后加的。
- `internal/settlementaccounting/application/receive_supplier_bill.go` 的 `BillUndecidedReason` 头注「跨币种匹配保持待判断是等配置，不是改单（AT-SA-096）。」与 `Handle` 里 `ErrCrossCurrencyMatch` 分支的注释「币种不同且无换算依据：保持待判断等配置，不用当前汇率猜测（AT-SA-096）。」：UC-SA-004 `AT-SA-096` 已改为答未配置（`BILL_UNDECIDED`，原因 `CONVERSION_UNCONFIGURED`），注释里的「保持待判断」要改为未配置的说法（2026-10-21 文档自洽 09 评审轮补）。
- 迁移 `migrations/collection_remittance/0001_collection_subledger_and_remittance.sql` 头注「五张表对应 CONTEXT 的五个词形」实建六张表：迁移已施加、受 checksum 固定，**不改**，只记在此。

## 证据

- AGENTS.md「改文档」：「引另一个文件的某句或某段 → 优先用符号名，无符号可指时用引文，一律**不用行号，也不用计数**」，并明说「**Go 注释里的跨文件引用同受此约束**」。
- 上列各符号的定义处（`PostingBasisKind` 的常量、`EvaluationStopCause.ResumedBy`、`AuditUndecidedReason` 的常量）。

## 三类

机制（注释）。无产品策略、无租户取值。

## 规模

S。

## 最小的诚实修法

1. `cmd/parcel-collection-register/main.go`：去掉「CONTEXT 的五个词形」，改为点名各命令所登的对象（代收指令、分户账、代收事实、记账、差异事项、回汇批次、汇付主张），不数 CONTEXT 的条目。
2. `PostingBasisKind`：去掉「四值」，写「封闭词表」并列出成员。
3. `StopPendingJudgmentCauseUnrecorded`：改为「价卡、序列、目录、映射四类登记都解得开它」，或去掉数字只列成员（与 `ResumedBy` 的分支同一份列表）。
4. `AuditUndecidedReason`：去掉「两格」，列出各未配置格。
5. `receive_supplier_bill.go` 那两句注释：「保持待判断」改为「答未配置（原因 `CONVERSION_UNCONFIGURED`）」，`AT-SA-096` 引用保留。

## 要裁的

无。

## 完成记录（2026-10-27，作者会话，分支 claude-cmtB，基 origin/main `f6a4a5298`）

只改注释，去掉注释行后 `git diff -U0` 无变动。迁移 `0001_collection_subledger_and_remittance.sql` 头注按票面**不改**。

### 逐处结果

| 位置 | 结果 |
|---|---|
| `cmd/parcel-collection-register/main.go` 包注释 | 位置已漂移：「七个命令」「五个词形」早已不在，只剩「覆盖 CONTEXT 的词形加分户账两面」；改为「各命令所登的对象」后接各命令名，不再指 CONTEXT |
| `PostingBasisKind` | 已漂移：代码注释现在只写「词表封闭」，不带数；六个成员就是下面的常量（含 `AGREED_OFFSET`），没有再列一遍 |
| `StopPendingJudgmentCauseUnrecorded` | 已改：价卡、参考序列、参考目录、费用项映射这几类登记，成员以 `ResumedBy` 的该分支为准 |
| `AuditUndecidedReason` | 已改：「各 `*Unconfigured` 格都是实例半边」，列审核授权、应付结算账户、审核越权升级上限（ADR-0163）；同句里 `AT-SA-088` 名下的引文「无授权不得人工接受或拒绝」其实出自 UC-SA-004「争议/审核授权」一行，改为点名 UC 再带 AT 号 |
| `BillUndecidedReason` 头注、`Handle` 的 `ErrCrossCurrencyMatch` 分支 | 已改：答未配置（原因 `CONVERSION_UNCONFIGURED`），`AT-SA-096` 保留 |

### 同类残留（SA、CR）

- `AT-SA-096` 的旧说法还在：`domain/supplier_bill.go` 的 `ErrCrossCurrencyMatch` 与 `MatchBillLine` 注释、`receive_supplier_bill_test.go` 与 `supplier_bill_test.go` 的 `Covers:`（后者还把 `AT-SA-082`、`AT-SA-083`、`AT-SA-086` 的引文写得与原文不同，改成原文）。其余仍说「保持待判断」的 AT（`AT-SA-110`、`AT-SA-144`、`AT-SA-177`）核对过，原文仍是保持待判断，没动。
- UC-SA-001「商业责任」一行同样改成了答未配置（`CONTRACT_UNCONFIGURED`）：`adapters/partycommercial/contract_responsibility.go` 引的「未配置时保持待判断」已不是原文，改为转述现行说法，并去掉「今天没有租户」一类时间叙述。
- 非原文引文：`form_requested_supplier_expected_cost.go` 的 ADR-0094（原文「失败预算从此只花在……」）；`apply_pre_acceptance_control.go` 的 CONTEXT「用虚假冻结冒充控制」（CONTEXT 里没有这句，改成不带出处的陈述）；`domain/credit_exposure.go` 的 CONTEXT「每项信用暴露保存实际采用的政策」（改引词条「接受前财务控制结果」）；`adopt_duty_payment_verification.go` 的 UC-SA-001 步 2 引文拼接了两栏并用省略号，改为单栏原文，并去掉「几格今天没有采用口」这句已无法核实的时间叙述。
- 变更叙述：`evaluation_resumption.go`、`adopt_duty_payment_verification.go` 各一处「今天」，CR 的 `handler_test.go` 一处「今天唯一」。
- CR 包的 ADR、UC、CONTEXT 引文机器核对没有不匹配；SA 其余不匹配多是省略号缩写或概念名，没动。
- 核了五个包注释里的 `ADR-NNNN`、`AT-XX-NNN` 编号，全部存在。

### 代决

1. `PostingBasisKind` 注释里「六个成员」不再单独列：成员就是紧跟其后的常量，再列一遍就是第二处定义。
2. SA 注释里的「UC-SA-004 步 5」之类 UC 步骤标号按 UC 自己的名字对待，没有清理。

### 越权风险点

- `assess_advance_recovery` 一路上「合同责任没登记」的实际答案是 `CONTRACT_UNCONFIGURED` 未决，与 UC-SA-001 现行说法一致；`adopt_duty_payment_verification.go` 那一句里「付款方、外部资金事实、合同责任几格不在本步采用」是我按 `AdoptDutyPaymentVerification` 的实际职责写的，没有去核其它几格现在各由哪个用例采用。

## 进 main 记录（2026-10-27，协调方）

- 分支 `claude-cmtB` 与 code/38、40、45、55 同批，由协调方快进，tip `c6f983ea2`；隔离评审（Sonnet）可合入，无阻断：每个改动的 `.go` 文件在去掉注释后语法树逐个比对（程序放评审私有目录，对有代码变动的老提交试过会报差异），38、40、45 三个提交 0 差异；新增的 38 条带引号或「」的引文机械核了 36 条逐字相同，另 2 条是带省略号的前缀引文与一句假想读者的话；新增的 ADR、UC、AT 指针逐条核过。协调方单独提交重算的 AT 计数、重生成的 ADR 索引与机制清点（本批无变动）；全量 `scripts/test-full.sh` 退 0、159 个包 ok；`go build ./...`、`go vet ./...` 干净；管理台 `tsc -b --noEmit`、`node scripts/run-tests.mjs`（803 过）、`vite build` 与 `./internal/architecture/...` 全过。无迁移、无新 ADR、无 AT 号变化。
- 本票：`StopPendingJudgmentCauseUnrecorded`、`AuditUndecidedReason`、`receive_supplier_bill.go` 两句按票改了；`cmd/parcel-collection-register/main.go` 的「七个命令、五个词形」改成「各命令所登的对象」；`AT-SA-096` 的旧说法「保持待判断」在 `domain/supplier_bill.go` 与两个 `Covers:` 里改成「答未配置」，评审核实与 UC-SA-004 原文及 `receive_supplier_bill.go` 实际结果（`BILL_UNDECIDED`，原因 `CONVERSION_UNCONFIGURED`）一致。`PostingBasisKind` 注释已不带数字，不用动。
- 去向：无新票。
