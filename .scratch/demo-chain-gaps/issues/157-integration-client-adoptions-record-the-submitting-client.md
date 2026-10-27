# 157 集成客户端族采用资金事实与登记凭证时记下提交的客户端

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 由票 140 立（ADR-0236 决定七、越权风险点 3、5）
Blocked by: 无
出处：票 [demo-chain-gaps/140](./140-operators-adopt-external-funds-facts-and-regulatory-credentials.md) 完成记录「没做与去向」；[ADR-0236](../../../docs/adr/0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 越权风险点 3、5

## 现象

票 140 给外部资金事实版本行（`settlement_accounting.external_funds_fact_version.operation_evidence`）与凭证行（`customs_compliance.regulatory_credential.operation_evidence`）加了操作证据一列：经管理台手工采用或登记时记 `OPERATOR/<发行方>#<sub>`。集成客户端族那三口（`/settlement-external-funds-fact-registrations`、`/settlement-external-funds-fact-correction-registrations`、`/customs-regulatory-credential-registrations`）与受控登记 CLI 不记，列为 NULL。结果是两族留痕不对称：册上看得出「这一版是不是人工录的」，看不出是哪台客户端送的；ADR-0236 决定四「更正不绑原采用族」若被 owner 推翻，也没有可读的依据判链头是哪一族采用的。

## 修法（待分诊）

- 集成客户端认证方把信封上的客户端主体交给 Intake，Intake 记成 `INTEGRATION_CLIENT/<发行方>#<sub>`（渠道前缀与手工口同一写法），落同一列；待遇同 ADR-0236 决定三（只进审计、不进比对与任何门）。
- 受控 CLI 一路照旧不记，或记一个表明「运维登记」的渠道前缀——分诊时定。
- 判据：真库用例证集成客户端采用的那一版记下客户端、重放不顶替；读面原样透出；手工口不受影响。
