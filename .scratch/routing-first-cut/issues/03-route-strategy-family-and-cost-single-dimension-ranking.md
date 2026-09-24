# 03 路由策略版本声明排序形态；首个内置形态「满足硬约束后按成本单维择优，并列交人工」

Category: enhancement
Status: ready-for-agent
Blocked by: 无
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「路由策略族」那一步的排序部分，与「首个内置排序策略」那一步
地盘：network-routing 领域与应用（排序与初始路由、复核两处择优出口）；目录路由策略版本的内容列与登记口（新迁移，号开工时在频道预留）；network-routing [`CONTEXT.md`](../../../docs/domain/network-routing/CONTEXT.md)、[UC-NR-001](../../../docs/application/network-routing/UC-NR-001-CREATE-INITIAL-ROUTE.md)、[UC-NR-003](../../../docs/application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md) 的相关句。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二、七；[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-NET-16` 已确认的机制句；[`label-channel-service-first-release/01`](../../label-channel-service-first-release/issues/01-channel-candidate-tie-break-authority.md) 的裁决。

## 做什么

1. **路由策略族**：路由策略版本声明它采用哪一种内置排序形态；形态的判断逻辑归产品，租户只选形态、填取值（ADR-0146 决定二）。首版族里只有一种形态。形态集合与校验落在领域，目录登记口经领域构造门收，未知形态拒登。
2. **首个内置形态**按 `PAR-NET-16` 已确认的那几句判：只在通过硬约束与时间可行性的合格候选里比；缺成本事实（待判断或不可计价）的候选出局，不以零或其他候选的金额顶替；币种不齐停下不比；最低成本并列且选不出唯一一条时交冲突，不按候选标识或任何无业务依据的次序收尾。
3. **并列的去处**：初始路由既不形成计划也不形成`无当前有效路由`，留痕全部候选与并列理由，等授权角色裁——人工选择入口不在本票（UC-NR-001「人工选择即使后续引入……」）；复核里并列只形成改路建议。两份 UC 的失败边界补这一格。
4. **候选成本事实的形状**（金额与币种，或待判断 / 不可计价）在本票定；它从哪里来归 02 与 10。

**要写明的一处前提变化。** label-channel/01 裁决时判路由侧「按标识升序收尾」仍然正确，前提是「那里是多维准则序，几乎不会全维打平」；ADR-0146 把首个内置形态定为成本单维，而那张票自己写过「单维恰恰最容易打平」。前提不再成立，本票据此改路由侧的并列出口，理由写进 CONTEXT 规则句，不改那张票的历史正文。

## 不做

- 不预选任何租户用哪种形态；不启用时效、可靠性等其他维度（`PAR-NET-16`：保持未配置）；不做人工裁决入口。

## 完成判据

- [ ] 领域用例：缺成本出局、币种不齐停下、唯一最低者选中、最低并列交冲突，各一格。
- [ ] 应用用例（内存替身）：初始路由并列时不落计划也不落无路由、结果可续办；复核并列只成建议。
- [ ] 真库用例：路由策略版本带形态登记并读回；未知形态拒登。
- [ ] CONTEXT 规则句与两份 UC 的失败边界同笔更新。
