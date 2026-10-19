# 34 演示租户的路由策略修订声明冻结形态：收寄后的路由复核在演示上越过冻结一格

Category: enhancement
Status: 完工，待评审与进 main——2026-10-18 Claude Code 会话立并实施（用户「你作为业务和系统专家自决……解决所有的问题」授权），分支 `claude-psb34`。出自演示动线重走（[05](./05-demo-journey-criterion-evidence.md)）。
Blocked by: 无（实走要 [routing-first-cut/21](../../routing-first-cut/issues/21-reassessment-after-network-intake-carries-no-geo-projection.md) 先越过 `PLAN_REVIEW_INCONCLUSIVE`；种子本身不依赖它）
地盘：`scripts/demo-seeds/data/network/` 新增一份行文件；`scripts/demo-seeds/seed.sh` 只加调用行；`scripts/demo-seeds/README.md` 两句；`cmd/parcel-dispatch/reference_network_wiring_test.go` 一条断言。
出处：network-routing CONTEXT「路由冻结边界」；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)；先例：上海枢纽修订 2（`05-node-sha-hub-v2.json`）。

## 现状（实测，证据 `S`）

- 带上 routing-first-cut/21 之后，收寄后的路由复核越过适用性那一格，计划仍适用，接着答 `FREEZE_FORM_UNCONFIGURED`。采用信封三次后落 `ABANDONED`。
- 成因：演示路由策略 `SYN-RS-CN-SG-01@1` 是参考配置 `SYN-CN-SG@1` 的采用行，只带适用范围与排序形态，没有冻结形态。复核按「策略版本未声明冻结形态不能当成未冻结」停下（CONTEXT 规则）。

## 三类归法

- **机制**与**产品策略**：冻结判断与内置形态「剩余段数」都已在 main，本票不改代码。
- **租户取值（演示租户）**：选哪种冻结形态、剩余几段即冻结。参考配置装不下这一格，演示租户另登一版修订。

## 做什么

- `23-route-strategy-cn-sg-v2.json`：`SYN-RS-CN-SG-01` 修订 2，直接登记行。
  - 照抄采用行的适用范围与排序形态。
  - 声明冻结形态 `REMAINING_SEGMENT_COUNT`，剩余不超过一段即冻结。
  - 生效自 2026-06-01，依据换成租户自己的。登记修订 2 会接续闭合修订 1。
- `seed.sh` 在段计价区域之后调一行登记口。

## 完成判据

- 一次性库干净灌种子退 0。
- 带上 routing-first-cut/21 实走：复核答 `STILL_APPLICABLE`，采用信封 `PUBLISHED`。证据只记 `S`，走完删库。

## 完成记录（2026-10-18，Claude Code 会话，分支 `claude-psb34`）

- 种子与调用行如上。演示自动改路声明仍未登，复核在计划仍适用时不做改善改路评估，照常答仍适用。
- 实走（`S`）：一次性库 `c8_walk5`，代码取 main 加 routing-first-cut/21 与本票。灌种子退 0。提交委托，初始路由 `ROUTE_FORMED`。经收寄口收寄、PS 采用之后，复核落一行 `STILL_APPLICABLE`，采用信封 `PUBLISHED`。

## 评审记录（2026-10-18，隔离子代理一次过两轴，判 PASS）

非阻断已修：README 组成表与动线第 3 步补上路由策略修订 2；接线用例补一条冻结形态断言，原先只断言排序形态；完成记录的基点不再写死。票 21 的链接随票 21 进 main 已能打开。
