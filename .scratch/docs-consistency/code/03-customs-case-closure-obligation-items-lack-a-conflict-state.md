# 03 CC 关闭依据项只有三值，缺「冲突」

Category: bug
Status: ready-for-agent——R3（不急）：关案编排 `CloseCustomsCaseHandler` 在 `cmd/` 下没有装配，今天没有生产路径；接线之前必须先修，否则冲突项会被迫读成未解决或已终结。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/cc.md](../audit/cc.md) M8（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

文档里关闭依据项是四值（已终结、已移交、未解决、冲突），任一未解决或冲突项都阻止关闭；代码只有三值，没有「冲突」，头注却引了含「冲突」的那句 CONTEXT。

## 证据

- 文档：
  - CC CONTEXT Language「关闭依据项」：「并分别表达已终结、已移交、未解决或冲突」；Rules：「任一未解决或冲突项都阻止关闭」。
  - GLOSSARY「关闭依据项」同义；UC-CC-010 输入表「已终结、已移交、未解决或冲突结论」。
- 代码：
  - `internal/customscompliance/domain/case_closure.go` 的 `ObligationItemState` 注释「关闭依据项的封闭三值：已终结、已被有权接收方有效承接、未解决」，取值 `CONCLUDED` / `HANDED_OVER` / `UNRESOLVED`。
  - `application.NewCloseCustomsCaseHandler` 在 `cmd/` 下无调用方。

## 三类

机制。无产品策略、无租户取值。

## 规模

S：领域加一格并带冲突依据引用，关闭核对把它当作阻止项；补用例。

## 最小的诚实修法

`ObligationItemState` 加「冲突」：阻止关闭，必须带冲突依据（与哪几份事实矛盾）。冲突依据的形状与票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md) 要立的冲突关系对齐，124 先落就直接引用它。

## 要裁的

无（CONTEXT 已定四值）。审计给的另一条路——owner 裁定冲突并入未解决、改 CONTEXT 与 GLOSSARY——属 B，归修订票 docs-consistency/03；那张票若这样裁，本票作废。
