# 15 CR 指令、事实、差异事项没有「关联原记录」，差异事项没有结论格，汇付记账不核归集截点

Category: enhancement
Status: ready-for-agent——R3（不急）：CR 今天只有受控 CLI，没有生产来源；缺的是更正链与结论这几格的结构，CLI 操作员靠记账标识与备注对上。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无（与 [14](./14-collection-principal-posts-more-than-once-and-shortfall-moves-received-funds.md) 改同一组文件，宜排在 14 之后）
出处：[audit/sa-cr.md](../audit/sa-cr.md) L6（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象与证据

- 文档：CR CONTEXT Lifecycles——「服务要求在来源侧变化 → 登记新的代收指令并关联原指令」；「来源侧更正或撤销时追加新的事实并关联原事实」；「差异事项被更正 → 登记新的事项并关联原事项」；「差异事项在册 → 处置结论形成 → ……」。Language「回汇批次」：「在归集截点前已处于`应付客户`的本金」。
- 代码：
  - `internal/collectionremittance` 的 `CollectionInstructionSpec`、`CollectionFactSpec`、`DiscrepancyItemSpec` 都没有回指原记录的字段；差异事项没有结论格。
  - `CollectionFact` 的注释写「追加新事实并关联原事实」，结构上做不到。
  - 汇付记账不核记账时刻是否早于批次的 `CollectedThrough`。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：三类记录各加可选的「关联原记录」并核同租户同类；差异事项加结论格与结论形成的记账依据；汇付记账核归集截点。

## 最小的诚实修法

各记录可选回指原记录（原记录不改写，读出「已被某条关联更正」）；差异事项结论形成之后才允许它作依据的差额落账（与 14 的短款裁定对齐）；汇付批次只收归集截点前已处于`应付客户`的本金。

## 要裁的

无（CONTEXT 已定）。若 09 改写差异事项的生命周期，结论格随之。
