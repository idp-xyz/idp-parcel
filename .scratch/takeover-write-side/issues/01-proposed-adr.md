# 01 接管记录写侧：ADR 草案，待用户裁

Category: enhancement
Status: draft——2026-09-29 通道 2 只出文档。用户已要求开写入口；五项还没拍板，本票不实现。
Blocked by: 用户裁决（[ADR-0154](../../../docs/adr/0154-takeover-record-write-side-is-not-yet-decided.md) 的待裁 1–4）
地盘：`docs/adr/0154-takeover-record-write-side-is-not-yet-decided.md` 与本票。不改 `parcel-governance-register`，不改迁移，不灌种子。

## 为什么现在不写代码

表 `pilot_governance.takeover_record` 和领域 `RecordTakeover` 已经在。读面按权威区间取这一行，没有行就不答「其他权威」。`parcel-governance-register` 没有接管子命令；不在已开四类里的种类，报错括注「接管未开」。`Takeovers.Save` 的插入在 `incident_records.go`，生产进程没有调用方，应用层 `TakeOver` 只在测试里被调用。ADR-0128 决定五把写侧和 `PAR-GOV-05..07` 的实例值留在外面。这三格在参数登记册都是「待提供」，证据责任方是运营、技术、合规与试点业务责任角色，不是开发方。

## 待裁

选项正文在 ADR-0154，这里只列题，避免两套口径：

1. 谁可以写：只开 CLI，或只开管理台，或 CLI 先开、HTTP 另票。
2. 写之前要有什么：已有权威区间，或未恢复的暂停，或不加这两问、只守 `RecordTakeover` 今天的拒绝条件（停写证据、区间四维与时刻、已接受事实、外部未决、实际控制、责任、下一步、至少一条盘点、生效时刻；逐条见 ADR）。
3. 与暂停、恢复、阶段评审：互不引用，或只允许发生在未恢复的暂停之内，或登记时闭合旧区间但不顺带插入新区间。
4. 合成 S：写入口打开后给演示租户一行且只记 S，或写入口开但种子保持空，或写入口也先不开。

第 5 项是继续不开的后果，不是实现选项：归属不答「其他权威」，交接步不开始，页面如实空。
