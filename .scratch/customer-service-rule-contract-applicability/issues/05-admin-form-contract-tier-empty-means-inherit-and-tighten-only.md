# 05 管理台：客户服务规则表单的合同版——空表即继承、只收紧不删减

Category: enhancement
Status: ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: [04](04-ve-claim-eligibility-checks-contract-and-inherits-rows-from-product-base.md)——文案不赶在行为前面
父票：[spec](../spec.md)
地盘：`apps/admin-web` 的客户服务规则表单；前端切片，在 `main` 上做（[workflow.md「前端切片」](../../../docs/agents/workflow.md#前端切片一人在-main-上直接做)）。
出处：[ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定三、六；admin-write-faces/18（现表单已有合同格与壳引用 `CUSTOMER_CONTRACT`）。

## 做什么

1. 空表语义改写：挂产品时无行即未登记；挂合同时无行即继承同范围产品版该行。
2. 加「只收紧」提示。
3. 浏览器发一版挂合同规则，走通五步。

## 判断项（拆票时裁定）

- **只收紧怎么守（Q3）**：本拆法只落 ADR-0176 决定三「不提供删减表达」那一半，不设「合同版行比底座更严」的比对门——底座是读时按锚点选的，起算事件或日历不同的两条期限行也比不出严宽。spec 用户故事 3 据此读作「不能借空行放宽成删减」；收紧是商务纪律，由本票的提示承担。**越权风险点 · 待 PC owner 复核。**
- **预览键（Q6）**：只做前端；预览口不改成答继承后的整行。实现时发现非碰 Go 不可，就停下来报，这张票不再是前端切片。

## 完成判据

- [ ] 三道门退 0。
- [ ] 浏览器五步走通；做不到就如实写哪一步、为什么。
- [ ] 文案与 04 的行为一致。
