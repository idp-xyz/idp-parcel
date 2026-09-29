# 01 接管记录写侧：ADR 草案，待用户裁

Category: enhancement
Status: decided——2026-09-29 用户授权自决，口径在 ADR-0154 Decision。本票仍不实现 CLI。
Blocked by: 无。实现另派，不在本票。
地盘：`docs/adr/0154-takeover-record-write-side-is-not-yet-decided.md` 与本票。不改 `parcel-governance-register`，不改迁移，不灌种子。

## 为什么现在不写代码

表 `pilot_governance.takeover_record` 和领域 `RecordTakeover` 已经在。读面按权威区间取这一行，没有行就不答「其他权威」。`parcel-governance-register` 没有接管子命令；不在已开四类里的种类，报错括注「接管未开」。`Takeovers.Save` 的插入在 `incident_records.go`，生产进程没有调用方，应用层 `TakeOver` 只在测试里被调用。ADR-0128 决定五把写侧和 `PAR-GOV-05..07` 的实例值留在外面。这三格在参数登记册都是「待提供」，证据责任方是运营、技术、合规与试点业务责任角色，不是开发方。

## 已定

口径只在 ADR-0154 Decision，这里不复述第二套。选定是：先开 CLI、HTTP 另票；不加构造函数以外的前置；不与暂停、恢复、阶段评审互引；种子不造行。实现另派。实现前 CLI 仍括注「接管未开」。
