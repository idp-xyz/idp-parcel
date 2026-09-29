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

## 进 main 记录（2026-09-29，通道 1 推送）

分支 `mcp2-takeover-adr@24bf90d4`（已推 origin）重放到 `7d12f3b0` 之上，零冲突，`git range-diff` 逐笔为 `=`：`9a40936e→b6791887`、`24bf90d4→9eddf1a8`。纯文档（ADR-0154 草案、ADR 索引一行、本票），没有代码变化，未重跑全量测试；上一次含 DSN 全量在 `ee4186ed`：134 ok / 0 FAIL。ADR 状态仍是 Proposed，五项待裁项等用户拍板，写入口未实现。

## Comments

**评审 ← 通道 4 · 钉 `9a40936e`，复审 `24bf90d4`**：首审一条阻断——待裁 2 丙与「领域今天已经拒绝」把 `RecordTakeover` 说成「三件齐就可以」，与代码不符（构造函数还拒绝已接受事实、外部未决、实际控制、责任、下一步为空白与生效时刻为零；表约束 `takeover_record_not_blank` 覆盖那几列文本）。作者以 `24bf90d4` 新增提交改为逐条列出拒绝条件，并改准「`Takeovers.Save` 在 `incident_records.go`、生产无调用方、CLI 无接管子命令」的措辞。复审：拒绝清单与构造函数和表约束一致（`AuthorityInterval.valid`；`inventory_present`、`bounds` 约束），无阻断、无非阻断。其余事实（ADR-0128 决定五、PAR-GOV-05..07 三格待提供、编号 0154 不与 main 上 0153 冲突、Status Proposed 无 Decision、不填规则版本/人/范围/阈值）首审即成立。
