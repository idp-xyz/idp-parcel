# 47 CR 协议抵扣没有记账形：认可的抵扣额只能留在`渠道在途`

Category: enhancement
Status: needs-info——R2（上线后尽快）：COD 渠道按有效协议从回款里扣下的金额（例如服务费抵扣），经 settlement-accounting 认可后，在代收分户账上没有任何记账能把它了结，只能留在`渠道在途`；账面不丢不重、看得见是未了结，但`渠道在途`余额不再只等于渠道还欠的本金，汇付前要人工对。记账形待 CR owner 定（[ADR-0271](../../../docs/adr/0271-collection-principal-enters-the-ledger-only-on-the-channel-collection-report-and-shortfall-comes-only-from-in-transit.md) 越权风险点 6）。2026-10-21 文档自洽 09 评审轮立（用户「你作为业务和系统专家自决」授权），分支 claude-dc09。
Blocked by: [14](./14-collection-principal-posts-more-than-once-and-shortfall-moves-received-funds.md)；用户（CR owner）决定抵扣记账形（[ADR-0271](../../../docs/adr/0271-collection-principal-enters-the-ledger-only-on-the-channel-collection-report-and-shortfall-comes-only-from-in-transit.md) 越权风险点 6）
出处：文档自洽 09 第 2 轮评审（协调方裁定：短款公式扣除经 settlement-accounting 认可的协议抵扣，抵扣的记账形未裁、另立本票）；与 [SCENARIOS](../../../docs/domain/SCENARIOS.md) 场景十七（报告 100、到账 96、认可抵扣 3、短款 1）对照。

## 现象

- collection-remittance CONTEXT「清分、汇付与差异」写：代收本金不得与 COD 服务费、运费或其他运营应收在本上下文内相互净额，抵扣只能依据 `settlement-accounting` 拥有的、有效协议允许的抵扣金额，通过明确记账依据表达。
- 代码没有这一种依据：`domain.PostingBasisKind` 只有 `COLLECTION_FACT`、`ALLOCATION`、`REMITTANCE_BATCH`、`DISCREPANCY`、`CORRECTION`，`domain.FundPosition` 也没有抵扣那一格；认可的抵扣额除了留在`渠道在途`，没有地方可去。
- 照 ADR-0271 的短款公式，认可抵扣从短款里扣除、不落`短款`；场景十七里那 3 因此留在`渠道在途`。

## 证据

- `internal/collectionremittance/domain/subledger.go`：`PostingBasisKind` 只有 `COLLECTION_FACT`、`ALLOCATION`、`REMITTANCE_BATCH`、`DISCREPANCY`、`CORRECTION`，没有抵扣依据；`FundPosition` 只有 `EXTERNAL_SOURCE` 与账内的渠道在途、待清分、应付客户、已汇付、短款、溢款，没有抵扣位置。
- `migrations/collection_remittance/0001_collection_subledger_and_remittance.sql`：`subledger_posting` 的 `from_position`、`to_position`、`basis_kind` 三列各有 CHECK 封住同一份词表，再加依据门的 CHECK；新依据或新位置要另取迁移号放开。
- 文档：collection-remittance CONTEXT「清分、汇付与差异」里抵扣那一条与「给开发的交接」；ADR-0271 决定四与越权风险点 6。

## 三类

机制：抵扣的记账依据与去向（待 CR owner 定形）。产品策略：无。租户取值：哪些抵扣有效、金额多少，由 settlement-accounting 的协议与认可给出，CR 不预填。

## 规模

M（定形之后）：新依据种类（指向 settlement-accounting 的认可引用），新去向或出账形，迁移 CHECK、领域构造门与编排；CLI 与种子各补一份。

## 最小的诚实修法

形定下来之前代码不动：认可的抵扣额留在`渠道在途`。汇付与对账时这部分余额要看得出是未了结的认可抵扣，不能当作渠道还欠的本金去追收——读面与对账口径按 settlement-accounting 认可的抵扣额把它从「渠道欠款」里分出来列示；形定之后照定下的依据与去向补记账，存量按认可记录补记。

## 要裁的

- 抵扣落到哪里：另立一个账内位置（例如「已抵扣」，账内终局，守恒照旧），还是允许一种出账记账（与 ADR-0082 决定四「去向侧没有账外位置」相抵，要另立记录）。
- 依据是什么：settlement-accounting 认可的抵扣引用经什么口交到 CR，CR 是否核金额上限。
