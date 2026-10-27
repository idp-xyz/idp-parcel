# 156 管理台手工采用资金事实与登记监管凭证改逐字段表单

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 由票 140 立（ADR-0236 决定七、越权风险点 6）
Blocked by: 无
出处：票 [demo-chain-gaps/140](./140-operators-adopt-external-funds-facts-and-regulatory-credentials.md) 完成记录「没做与去向」；[ADR-0236](../../../docs/adr/0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 越权风险点 6

## 现象

票 140 起，收付款核销页「采用资金事实」签（首版、更正两册）与关务案件页的凭证登记签经手工口（运营决定能力面）提交，持对应授予的财务、关务人员已经能在管理台采用外部资金事实、登记监管凭证。可三张签仍是 JSON 录入：登记方要照形状提示逐个写键名（`factRef`、`sourceRef`、`kind` 封闭三词、`amountMinor` 最小单位整数、RFC 3339 时刻……）。这一形态是给受控 CLI 的批文设计的，那时这几张签没有真渠道、用不了；如今它们是财务、关务人员照银行回单或凭证原件录入的日常入口，JSON 录入出错率高、要人懂格式。

## 修法（待分诊）

- 三张签各换一份逐字段表单：封闭集给选择、金额按币种小数位录入并换算成最小单位、时刻用日期时间控件并带时区；更正那一册从当前链头带出被更正版本，只让改金额与更正时刻。
- 载荷形状不变（服务端 `registrationjson` 那一份仍是唯一译装），表单只是生成同一份 JSON；答案代数与面板的答复格不变。
- 判据：表单生成的载荷与现有形状提示逐键一致；封闭集外的值在表单里选不出；admin-web 三道门绿。

分诊要定的：是否上线前必修（看首个试点租户的手工录入量），以及表单落在页面里还是抽进共享的登记面板。
