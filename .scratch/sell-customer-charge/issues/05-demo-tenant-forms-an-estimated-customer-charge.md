# 05 演示租户：登 SELL 计费时点采用、费用项目映射与价卡取整，演示包裹形成预估客户费用

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立。
Blocked by: [02](./02-request-sell-evaluation-on-parcel-moment.md)、[03](./03-pricing-consumes-sell-evaluation-requests.md)、[04](./04-sell-evaluation-recorded-forms-estimated-customer-charge.md)
地盘：`scripts/demo-seeds`（seedgen 的售价卡、`data/settlement/` 新增两份、`seed.sh` 调用行、README）；演示动线脚本补记。
出处：只收 demo-chain-gaps/09 的 SELL 那一半与 10 的 SELL 那一半；[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)；[demo-chain-gaps/09](../../demo-chain-gaps/issues/09-demo-price-cards-declare-no-amount-rounding.md)、[10](../../demo-chain-gaps/issues/10-demo-tenant-buy-and-sell-trigger-registers-are-empty.md)。

## 做什么

- 售价卡声明金额取整（总额、0.01 CNY、四舍五入），就地改 v1 后用 `seed.sh --reset` 重灌；另登 v2 会撞适用冲突。
- `SYN-SETTLEMENT-ACCOUNT-01` 采用 `NETWORK_INTAKE_ADOPTED`，计价范围与受理前估价采用册登的那一个相同（ADR-0190 越权风险点 9）。
- `BASE_FREIGHT` 与 `REMOTE_AREA_SURCHARGE` 都映到同一个合成费用项目（`SYN` 简化，ADR-0190 越权风险点 2）。

## 完成判据

- 一次性库实走（`S`）：提交、受理、收寄、采用之后，演示包裹形成一笔预估客户费用，金额与受理前冻结同额（今天没有实测读口，两边都用申报，故同额）。走完删库。
