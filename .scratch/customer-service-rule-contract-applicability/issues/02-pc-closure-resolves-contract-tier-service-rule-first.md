# 02 PC 闭包：客户服务规则合同优先解析，与结算政策同排第二段

Category: enhancement
Status: in-progress——2026-10-10 通道 5 认领（单 task-b39b287a-4dfa-4c14-9786-0d43264c2aab），分支 `mcp5-csr02` 基 `1d67e27c`。此前：ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: 无
父票：[spec](../spec.md)
地盘：`internal/partycommercial/domain` 的闭包解析；碰 Go，走[并行会话](../../../docs/agents/parallel-sessions.md)那条路。
出处：[ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定一；spec「Implementation Decisions」解析段、「Testing Decisions」缝一。

## 做什么

1. 预重构先做：`resolutionOrder` 由「只挪结算政策」改为「依合同的第二段」一处声明；「同范围挂服务产品、锚点生效」的选法抽成一处，供 [03](03-pc-layered-read-port-returns-contract-and-product-base-bodies.md) 复用。
2. 闭包对客户服务规则：有壳 `references` 指名闭包已解出的客户合同的版本，就采纳它；没有就采纳挂服务产品的版本。同层多候选答`适用冲突`，两层皆零答`无适用依据`。闭包仍唯一采纳一版。
3. 只有挂产品一版的既有登记，行为不变；解析键登记面不加键。
4. 用例镜像 `settlement_basis_resolution_test` 一族。

## 判断项（拆票时裁定）

- **合同不在必需依据里时（Q1）**：合同被请求但冲突或无依据 → 前提未解；合同根本没被请求 → 只看产品层。理由：合同没解出时无从知道有没有挂合同版，回落产品版可能静默套上更宽的条款。
- **指名到哪一级（Q2）**：按对象一级对上，合同换版后同一份挂合同规则照旧适用；条款要随合同改，就发新一版规则。不给壳加版本格（ADR-0176 决定五「规范化零变化」）。
- **同一合同两版（Q4）**：只在闭包答`适用冲突`，发布面不加门——ADR-0176 没裁发布门，Alternatives 否的是保存面禁止产品版与合同版并存。spec 用户故事 4 的「并拒绝」读作解析时不采纳，不是发布时拒收。**越权风险点 · 待 PC owner 复核。**

## 完成判据

- [ ] 合同优先、产品回落、同层多候选、两层皆零、合同被请求但未解出、合同未被请求六格各有用例。
- [ ] 只有挂产品一版的既有登记，解析结果与改动前相同，有用例钉住。
- [ ] 解析键登记面与 PCC-1 规范化无改动。
