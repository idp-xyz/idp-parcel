# 03 PC 层次读口：一次取回合同版正文、产品底座版正文与各自在场标志

Category: enhancement
Status: in-progress——2026-10-10 23:4x 通道 4 认领（单 task-628d9b90，改派自通道 5 `task-96bf9634`），分支 `mcp4-csr03`，基 `70f32c2a`。此前：ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: [02](02-pc-closure-resolves-contract-tier-service-rule-first.md)——底座必须用 02 抽出的那一处选法，不造第二套口径
父票：[spec](../spec.md)
地盘：`internal/partycommercial` 的 ports、application、adapters/postgres；碰 Go / SQL，走并行会话那条路。
出处：[ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定二；spec「Testing Decisions」缝三。

## 做什么

1. 新读口按（租户、范围、合同版本、锚点）一次取回：合同版正文、产品底座版正文、各自在场标志。与 `LoadCustomerServiceRule` 同族，ports + application + postgres。
2. 底座在读口内按 02 那一处选法选「同范围挂服务产品、锚点生效」的版本；底座多候选照`适用冲突`纪律答。
3. 既有单版点读口不动。
4. 用例镜像 postgres `customer_service_rule_test` 一族：在场标志、底座多候选、租户隔离。

## 完成判据

- [ ] 合同版与底座版在场 / 不在场四种组合各有真库用例（带 DSN，`-v` 下 PASS 非 SKIP）。
- [ ] 底座多候选答`适用冲突`；租户隔离有用例。
- [ ] 单版点读口答复不变。
