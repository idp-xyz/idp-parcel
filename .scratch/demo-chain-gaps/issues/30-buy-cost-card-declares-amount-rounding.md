# 30 BUY 成本卡声明金额取整（09 余半）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无
地盘：`scripts/demo-seeds/data/pricing/` 成本卡新版本、`seed.sh` 一行。
出处：[09](./09-demo-price-cards-declare-no-amount-rounding.md)；方案「BUY 发起（接 08，收 10）」D10-1，「已不成立或已修」09。

## 做什么

- 演示 BUY 成本卡补声明 `amountRounding`，以新版本重发，`seed.sh` 加一行（方案 D10-1）。
- 进 main 的种子令牌次序见方案「并行车道」。

## 完成判据

- 成本卡以新版本重发并声明取整。
- `seed.sh` 退出码 0。
- 走 ADR-0200 路径时不再落「金额未声明」格。

## 规模

S
