# 14 准入范围读口与「不在准入范围」一格：ADR-0055 决定五第二项

Category: enhancement
Status: ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）
Blocked by: 无（读口与答复格）；接进各族铸造随 10、11
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：pilot-governance 生产权威区间的只读口（按租户、对象范围、能力、事实类型问当前区间）、`internal/accessidentity` 铸造前的准入判断与新答复格。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四第二条；PN-08 的生产权威区间记录能力。

## 做什么

1. pilot-governance 出只读口：给定（租户、对象范围、能力、事实类型）与时点，答当前生产权威区间覆盖与否；读失败走依赖故障，不折成「不覆盖」。
2. 铸造前判：区间不覆盖答新格「不在准入范围」（`403`，恢复动作：阶段治理登记区间），不进业务编排。隔离形态（ADR-0091）不走这一判。

## 完成判据

- 覆盖、不覆盖、读失败三格各有用例；生产形态下无区间时一律答「不在准入范围」；隔离形态行为不变。
