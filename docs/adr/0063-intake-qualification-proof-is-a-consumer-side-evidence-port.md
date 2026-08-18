# ADR-0063: 收寄硬资格证明由消费侧窄口取证，商业上下文只声明开放引用

Status: Accepted
Date: 2026-08-18

## Context

`JudgeIntakeEligibility` 在 `PAR-COM-16` 资格清单非空时一律答 `NOT_ESTABLISHED`，依据为 `INTAKE_QUALIFICATION_UNPROVEN/` 加头一项引用。注释把取证缝写成实例半边，但适配器并不向任何权威方提问。插入关务或实例行也不会被读到。这是机制缺口。

[ADR-0058](./0058-stage-content-owned-by-rule-objects.md) 管的是阶段内容声明按规则对象归属；[ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md) 管的是采用版本怎么从已接受解析标识回指。两份都不回答「声明列出的开放引用，由谁证明」。

[party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md) 写明：规则包只装配各权威上下文的规则引用，不得替具体委托选择判断值，也不得把正式关务判断前移到商业上下文。`RuleReference` 是引用，正文在执行该规则的那个上下文。

夹具字符串 `INTAKE-QUAL/customs-precheck` 出现在 PC/PS 测试与 SYN-PC-SEED 里，不是 `customs-compliance` 的类型、事件或端口名。`ReadinessView` 与 `GateConditionView` 属于申报就绪与放行门禁（[UC-CC-003](../application/customs-compliance/UC-CC-003-ASSESS-DECLARATION-READINESS.md) / [UC-CC-009](../application/customs-compliance/UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md)），不是收寄硬资格。[UC-PS-003](../application/parcel-shipment/UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md) 写明正式承诺不等待关务提交。[UC-NR-002](../application/network-routing/UC-NR-002-ASSESS-PARCEL-REACHABILITY.md) 的禁限运与关务资格是路由候选筛选，不能顶收寄阶段硬资格。

空清单会让资格直接 `ESTABLISHED` 并形成正式承诺。不得用清空清单或默认成立来「越过」未证明。

## Decision

**一、商业上下文只声明开放引用，不发明证明。** `IntakeQualificationContent` 的资格项仍是开放 `RuleReference`。证明不在 party-commercial 形成，也不在消费侧把引用拆成假的关务身份。

**二、消费侧自有窄口，按（身份 + 来源/包裹 + 引用 + 收寄业务时点）问证明。** 端口用 parcel-shipment 的语言定义（[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)）。时点取自被采用来源的实际发生时间，不用处理时间或时钟。适配器把提供方引用转写成消费方 `QualificationRuleReference`；权威方实现落在日后的消费侧适配器，本记录不接 `ReadinessView`、`GateConditionView` 或路由硬约束口。

**三、封闭格按恢复动作划分（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)）。**

| 格 | 资格判断 | 恢复 |
|---|---|---|
| 已证明 | 该项通过，看下一项；全部通过且来源在允许集合内则 `ESTABLISHED` | 无 |
| 未证明 | `NOT_ESTABLISHED`，依据保持 `INTAKE_QUALIFICATION_UNPROVEN/<ref>` | 等该项证据到达后重试同一拍（`AT-PS-047`） |
| 依赖不可用 | `JudgeIntakeEligibility` 返回 error，不折成资格目录未配置 | 等依赖恢复后重试 |

资格目录未配置（`configured=false`）与硬资格未证明是两格：前者等 `PAR-COM-16` 声明，后者声明已在、证明未到。两者都可以被编排译成 `consumer_undecided`，但适配器 outcome 必须可分。未知前缀答未证明，不得已证明。

**四、空清单仍是显式「无硬资格」：`ESTABLISHED`。** 清单非空时，`nil` 证据口不得变成 `ESTABLISHED`；生产装配必须给出显式未配置实现，该实现答未证明。不得清空清单来形成承诺。

**五、正式关务判断仍由 customs-compliance 拥有。** 本口不问申报就绪、提交授权或放行门禁，也不把「关务提交尚未发生」当成收寄硬资格失败以外的第二套停点。

## Consequences

- `ServiceStageRulesAdapter` 在清单非空时逐项（先碰到的未证明项点名依据）问证据口；未配置实现让 SYN-PC-SEED 两条真库链停点保持 `NOT_ESTABLISHED`。
- 将来按引用前缀接权威方时，未登记前缀仍答未证明。不得为了变绿把 `INTAKE-QUAL/customs-precheck` 拆成申报单元或案件号。
- ADR-0058 第三条与 ADR-0062 回指路径不变：本记录只补「声明列出之后如何取证」。

## Alternatives considered

- **继续在适配器里对非空清单一律未证明，等实例行。** 否决：没有查找缝，实例来了也接不上。
- **空清单或默认 `ESTABLISHED`。** 否决：会形成正式承诺；SYN-PC-SEED 已禁止。
- **用 `ReadinessView` / `GateConditionView` / UC-NR-002 硬约束顶收寄硬资格。** 否决：那些口问的不是 UC-PS-003 第 5 条；正式承诺不等待关务提交。
- **把证明口放进 party-commercial。** 否决：PC 不得替委托选择判断值，也不得前移正式关务判断。
- **未知前缀当依赖不可用。** 否决：没有登记过的前缀不是暂时故障，重试依赖不会让它出现；答未证明才能续办或改声明。

## Links

- [ADR-0025：跨上下文适配器落在消费方](./0025-cross-context-adapters-live-on-the-consumer-side.md)
- [ADR-0029：取回失败按恢复动作分格](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)
- [ADR-0058：阶段内容声明按拥有规则对象归属](./0058-stage-content-owned-by-rule-objects.md)：本记录只管声明之后的证明，不改声明归属
- [ADR-0062：采用规则版本从已接受解析标识回指](./0062-adopted-stage-owner-from-accepted-resolution.md)：采用版本与证明是两段
- [UC-PS-003：有效网络收寄与正式承诺](../application/parcel-shipment/UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md)
- [UC-NR-002：可达性评估](../application/network-routing/UC-NR-002-ASSESS-PARCEL-REACHABILITY.md)：禁限运不是收寄硬资格口
- [UC-CC-003：申报就绪](../application/customs-compliance/UC-CC-003-ASSESS-DECLARATION-READINESS.md)：不得冒充收寄硬资格
