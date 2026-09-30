# 01 接管记录写侧：ADR 草案，待用户裁

Category: enhancement
Status: decided——2026-09-29 用户授权自决，口径在 ADR-0154 Decision。本票仍不实现 CLI。
Blocked by: 无。实现另派，不在本票。
地盘：`docs/adr/0154-takeover-record-write-side-is-not-yet-decided.md` 与本票。不改 `parcel-governance-register`，不改迁移，不灌种子。

## 为什么现在不写代码

表 `pilot_governance.takeover_record` 和领域 `RecordTakeover` 已经在。读面按权威区间取这一行，没有行就不答「其他权威」。`parcel-governance-register` 没有接管子命令；不在已开四类里的种类，报错括注「接管未开」。`Takeovers.Save` 的插入在 `incident_records.go`，生产进程没有调用方，应用层 `TakeOver` 只在测试里被调用。ADR-0128 决定五把写侧和 `PAR-GOV-05..07` 的实例值留在外面。这三格在参数登记册都是「待提供」，证据责任方是运营、技术、合规与试点业务责任角色，不是开发方。

## 已定

口径只在 ADR-0154 Decision，这里不复述第二套。选定是：先开 CLI、HTTP 另票；不加构造函数以外的前置；不与暂停、恢复、阶段评审互引；种子不造行。登记命令 `takeover` 在分支 `mcp2-takeover-cli`。种子仍不造行。HTTP 写仍另票。查阅口另见 ADR-0155，不在本票。

## 进 main 记录（2026-09-29，通道 1 推送）

分支 `mcp2-takeover-adr@24bf90d4`（已推 origin）重放到 `7d12f3b0` 之上，零冲突，`git range-diff` 逐笔为 `=`：`9a40936e→b6791887`、`24bf90d4→9eddf1a8`。纯文档（ADR-0154 草案、ADR 索引一行、本票），没有代码变化，未重跑全量测试；上一次含 DSN 全量在 `ee4186ed`：134 ok / 0 FAIL。那次重放当时 ADR 状态仍是 Proposed，写入口未实现。用户随后授权自决，Decision 在后一笔。

## Comments

**评审 ← 通道 4 · 钉 `9a40936e`，复审 `24bf90d4`**：首审一条阻断——待裁 2 丙与「领域今天已经拒绝」把 `RecordTakeover` 说成「三件齐就可以」，与代码不符（构造函数还拒绝已接受事实、外部未决、实际控制、责任、下一步为空白与生效时刻为零；表约束 `takeover_record_not_blank` 覆盖那几列文本）。作者以 `24bf90d4` 新增提交改为逐条列出拒绝条件，并改准「`Takeovers.Save` 在 `incident_records.go`、生产无调用方、CLI 无接管子命令」的措辞。复审：拒绝清单与构造函数和表约束一致（`AuthorityInterval.valid`；`inventory_present`、`bounds` 约束），无阻断、无非阻断。其余事实（ADR-0128 决定五、PAR-GOV-05..07 三格待提供、编号 0154 不与 main 上 0153 冲突、当时 Status 仍是 Proposed、不填规则版本/人/范围/阈值）首审即成立。其后用户授权自决，Decision 在随后那一笔，不再等拍板。
