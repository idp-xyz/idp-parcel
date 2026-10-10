# 04 VE 索赔资格：采纳合同版时按合同点核、按行继承产品底座

Category: enhancement
Status: ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: [03](03-pc-layered-read-port-returns-contract-and-product-base-bodies.md)
父票：[spec](../spec.md)
地盘：`internal/visibilityexception/adapters/partycommercial` 与 `cmd/parcel-api` 的索赔装配；碰 Go，走并行会话那条路。
出处：[ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定二、四；spec「Testing Decisions」缝二。

## 做什么

1. 闭包采纳挂合同版时，VE 适配器经 03 取两份正文：正文挂的合同 ≠ 查询携带的合同 → `ErrUntranslatableAnswer`。
2. 按行拼：合同版有行用合同版，无行取底座版该行，两层皆无答既有「未登记」。
3. 闭包采纳挂产品版时照旧走单版点读，不核。
4. `cmd/parcel-api` 的索赔装配接新读口；`claim_service_rules_resolution_test` 加三族。

## 判断项（拆票时裁定）

- **读时底座多候选落哪一格（Q5）**：落与「闭包对客户服务规则答`适用冲突`」同一既有格——恢复动作相同，都是运营更正登记（ADR-0029 看恢复动作），不新立格。实现时核一次，写进完成记录。
- **底座不随闭包冻结（Q5）**：底座是读时按锚点选的，已接受委托的底座只靠「锚点固定 + 版本不回溯生效」保持不变；有效性更正（ADR-0038）改了底座区间时会改口。这是 ADR-0176 决定二的已知后果，不当缺陷。

## 完成判据

- [ ] 点核：挂合同 ≠ 查询合同答 `ErrUntranslatableAnswer`，有用例。
- [ ] 按行继承三格（合同有行、无行取底座、两层皆无答未登记）各有用例；挂产品版行为不变。
- [ ] `cmd/parcel-api` 装配有带 DSN 的用例。
