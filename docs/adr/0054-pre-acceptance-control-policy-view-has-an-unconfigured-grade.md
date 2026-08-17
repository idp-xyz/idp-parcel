# ADR-0054: 接受前财务控制策略视图增设「未配置」格

Status: Accepted  
Date: 2026-08-17

## Context

`settlement-accounting` 的 `PreAcceptanceControlPolicyView` 是 SA 向商业侧要「这个范围要不要接受前财务控制」的那一口。它今天只有两格：

```go
LoadControlPolicy(ctx, tenant, scope) (domain.PreAcceptanceControlPolicy, error)
```

**这两格说不出「没人登记过」。** 端口自己的注释已经把危险点写明了——「把它读成`不要求控制`正是 CONTEXT 禁止的`默认信用通过`」——但它防的只是 error 那一格被误读。真正没有出口的是另一条路：适配器交回零值 `PreAcceptanceControlPolicy` 且 `err == nil` 时，`ControlRequired()` 为 false，编排随即落成 `ControlNotApplicable`，且 `Basis()` 是空的。那正是被禁的那一格，而两格代数里没有任何地方拦得住它。

领域侧有一条用例守着零值（`TestTheZeroControlPolicyDoesNotAnswerNotRequired`），但它只证了「零值不报告需要控制」。**零值报告不需要控制、且不带依据**——恰恰是它没管的那一半，也恰恰是危险的那一半。

今天撞不着，因为这一口**没有任何生产实现**：`PAR-COM-15`（接受前财务控制策略）在参数登记册里是`待提供`，`party-commercial` 侧连领域类型都还没有，`PreAcceptanceFinancialControlPolicyObject` 只作为封闭九类的一个枚举值存在。但缺口的形状已经定了：**一写适配器就撞得着**，而首发没有租户时，「未登记」是那个适配器唯一走得到的真实分支。

同样的病刚在 `network-routing` 治过。[ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md) 给两个网络证据端口补了第三格，理由一字不差地成立于此：空答复会被下游评成一个业务结论，error 则把未配置伪装成依赖故障。

## Decision

**一、`PreAcceptanceControlPolicyView` 增设「未配置」格。**

```go
LoadControlPolicy(ctx, tenant, scope) (domain.PreAcceptanceControlPolicy, bool, error)
```

三格各自的含义与消费方义务：

| 返回 | 含义 | 消费方必须 |
|---|---|---|
| `found=true` + policy | 商业侧登记过本范围的控制策略 | 按 policy 走：`要求`带方式与采用政策，`不要求`带商业不适用依据 |
| `found=false` | **未登记**（`PAR-COM-15` 待提供） | 停在自己的未决格，等登记 |
| `error` | 调不通 | 停在未决格，等重试 |

**二、`未配置`与`调不通`分成两个未决原因，不合并。** SA 编排新增 `ControlPolicyNotConfigured`（`CONTROL_POLICY_NOT_CONFIGURED`），与既有 `ControlPolicyUnavailable` 并列。恢复动作相反是分格的唯一判据（同 [ADR-0029](./0029-recovery-action-is-the-error-algebra.md)）：前者要去催商业侧登记，后者要去重试依赖。合成一格，运维读不出该找谁。

**三、`未配置`绝不落成`无控制`。** 这是本记录存在的理由。`无控制`是合同已经说过的终局答案，据它可以放行接受判断；没人说过话时放行，就是 CONTEXT 明禁的默认信用通过。两者在结果代数上必须离得足够远——`未配置`走`待判断`（带可续办引用），不走 `ControlNotApplicable`。

**四、未配置时不读余额与信用。** 顺序与既有的`无控制`分支同理：策略都还没有，去读这个客户的资金状况既是白做的，也已经读了。

## Consequences

- 这是一次跨文件签名改动：端口、SA 编排、两个测试替身与 `parcel-shipment` 消费侧适配器的测试同笔跟随。跟随部分只改签名不改语义。
- 未来那个 PC 消费适配器（`internal/settlementaccounting/adapters/partycommercial/`）有了诚实的表达方式：未登记即 `found=false`，不必在「编一个不适用依据」与「假装调不通」之间二选一。
- **本记录不解决提供方表面。** 控制策略声明族在 `party-commercial`（CONTEXT 与 CONTEXT-MAP 都已判给它），那是另一笔工作。本记录只保证：那份声明到位之前，缺它的事实说得出口。
- 另一半的分工同时钉住：`要求`那一格需要的结算方式与实际采用政策**不由控制策略声明重新发明**，走 [ADR-0044](./0044-settlement-basis-adopts-via-settlement-policy.md) 已有的结算政策解析。反过来也不成立——[`pn-02-w03`](../design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md) 明写「结算模式不等于接受前财务控制策略：账期不能推导无需信用校验」，因此不得拿解析出的结算政策倒推控制要不要做。

## Alternatives considered

- **归入 ADR-0052 的适用范围。** 否决：0052 的正文与标题都限定在网络证据端口上，事后把一份已接受记录的范围拉伸到另一个上下文，比新写一份小记录更糟——回溯的人分不清被接受的到底是哪一条。
- **让适配器在未登记时交回 error。** 否决：那是把一个已知的、正常的实例半边状态伪装成故障。运维会去重试一个永远不会自己好起来的东西，而真正要做的是登记 `PAR-COM-15`。
- **在领域侧再加一条用例，拦住「零值答不要求」。** 否决：它拦得住零值，拦不住一个**构造合法但内容为空**的答复，也说不出「未登记」这件事本身。问题在端口代数上，补用例是拿测试去顶一个类型该表达的东西。
- **让 SA 在未配置时按最严处置（一律要求控制）。** 否决：那是本上下文自行推导控制策略，而端口注释第一句就写着「本上下文只消费它，绝不自行推导」。最严的默认值也还是默认值。

## Links

- [ADR-0052：网络证据端口增设「未配置」格](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)：同病先例，本记录沿用它的三格形状与分格理由
- [ADR-0029：错误代数按恢复动作划分](./0029-recovery-action-is-the-error-algebra.md)：`未配置`与`调不通`分成两格的判据出处
- [ADR-0044：结算依据经结算政策采用](./0044-settlement-basis-adopts-via-settlement-policy.md)：`要求`那一格的方式与采用政策由它提供，不由控制策略声明重造
- [ADR-0047：账期控制形成信用暴露而非冻结](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)：`要求`携带方式与采用政策引用的出处
- [`pn-02-w03`](../design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md)：「结算模式不等于接受前财务控制策略」的禁推导句
- [参数登记册 `PAR-COM-15`](../product/PILOT-PARAMETER-REGISTER.md)：本记录所说的「未登记」指的就是它
