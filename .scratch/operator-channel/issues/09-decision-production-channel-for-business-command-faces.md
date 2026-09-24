# 09 决策：主链业务命令面在生产上走哪一族渠道，ADR-0055 决定五两项未决怎么解

Category: enhancement
Status: draft
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨
地盘：一份新 ADR（编号开工时在频道预留）与本票票面；不写代码。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四末段与决定五（客户业务命令面「照旧被两项未决拦着」、「不给客户业务命令面开操作者渠道」）；[ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md) 决定五（载荷规范化摘要与准入范围装配）；[ADR-0021](../../../docs/adr/0021-frontline-operations-client-is-part-of-the-product.md)（一线作业端属产品）。

## 要裁的

1. 作业事实登记（收寄、交接、移动、派送、交付等）由谁提交：一线操作者族（ADR-0021 的作业端）、设备、还是伙伴来源——各自的信任锚与授权模型。按 ADR-0146，渠道族与认证形态归产品策略，只有凭证与人到角色的分派是租户取值。
2. 外部结果与外部资金事实的接收渠道（连接器形态与票 psb/09、10、12 的连接器项衔接）。
3. ADR-0055 决定五两项未决（载荷规范化摘要、准入范围装配）对这些口怎么解。

## 完成判据

- ADR 接受（归用户或其授权的 owner），裁决拆成后续实施票；08 的隔离放行不因本票而退场（ADR-0091 决定六）。
