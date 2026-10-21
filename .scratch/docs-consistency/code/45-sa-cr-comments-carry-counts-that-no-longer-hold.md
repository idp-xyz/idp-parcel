# 45 SA、CR 代码注释里的计数与实际不符，一处数的是 CONTEXT 的条目

Category: chore
Status: ready-for-agent——R3（不急）：只是注释，不改行为；读注释的作者会被少数、错数与跨文件计数误导。2026-10-21 文档自洽 09 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc09。
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
