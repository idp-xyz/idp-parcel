# PBC-08 行为面收口：补 30 方法真缺证据，53 行归属定案，审计脚本门禁化

Category: enhancement
Status: ready-for-agent

来源：[票 01](./01-reeval-verdict-bento-gate-stays-blocked.md)「差什么」第 1、2 条，
2026-08-21 MCP-3 受用户委托裁断开票。证据基线与三分类清单以票 01 正文为准，原件以
`git show 2399ecd:.scratch/bento-gate-reeval/<file>` 调取。

## 交付物

1. **补真缺**：票 01 列名的 30 方法 / 22 类型（customscompliance 12、visibilityexception 12、
   parcelpricing 1、settlementaccounting 特名 5）逐个补「引用 `ErrTransactionRequired` 且调用
   该方法」的负向证据块。简报明文该性质「随适配器逐个成立，没有静态门禁能替它把关」。
2. **定待归属**：按 `evidence-blocks.txt`（`2399ecd`）完成 53 行人工归属，把审计输出的
   「84 缺」修成真实清单；已实证的假 MISSING 形状（捆绑夹具 `new*Stores` + 包内非唯一
   `Save`/`Replace`）见票 01 第二节。
3. **脚本门禁化**：改进 `audit-pbc08.ps1` 的归属逻辑（识别捆绑夹具），使其可复跑、零假阳后，
   评估并入 `internal/architecture` 常驻门禁的可行性；并不进去也要把可复跑版脚本与口径
   落盘（不再是一次性脚本）。

## 边界

- 只补测试与脚本，不改任何生产写方法的行为。
- `outboxintent.EnqueueOnce` 自包证明要不要求，属口径问题，本票按票 01 的记载与行动 2 一并
  定口径并写明理由，不默认。
- 本票完成不解除 Bento 闸门：它只收 PBC-08 行为面一项；闸门解除按票 01 裁定走九项全过 +
  `B-06` 登记。

## 参照

票 01（三分类清单与证据索引）；ADR-0026（先证后闸）；`docs/design/` 持久化简报的
「每新增一个持久化适配器都要自带这条证明」。

## Comments

- 2026-08-21 MCP-3：随票 01 收口裁定开票。
