# ADR-0080: 引用闭包先解合同再据以解结算政策——合同维是结论不是输入，前提未解析自成一格

Status: Accepted
Date: 2026-08-26

## Context

[ADR-0044](./0044-settlement-basis-adopts-via-settlement-policy.md) 让结算依据经结算政策采用：解析键上带一个 `SettlementSelector`（相对方、**客户合同版本**、费用范围、币种），`ResolveSettlementPolicy` 拿它与法人、时点合成六维精确匹配。四维中的合同那一维，取值形如 `contract-1/v1`——**它指名一个客户合同版本**。

[ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md) 把「回指换闭包 → 闭包取合同 → 合同读声明」定成生产路径之后，接受前控制链上游那一段到期了：`parcel-shipment` 要能登记一个含 `SETTLEMENT_POLICY` 的解析键，闭包才形得成，`PolicyBackedControlScopeSource.FormControlScope` 才不再一律交回 `formed=false`。

这一段撞上一处循环：

- PS 的 `ResolutionKeyRegistration.validate()` 对 `SettlementPolicyObject` 与 `PriceRuleObject` 直接报错，原话「需要键携带额外选择维度，本登记面不承载」；`migrations/parcel_shipment/0007_commercial_resolution_key.sql` 的 CHECK 是同一判据的第二道镜像。
- 要放行就得给登记面加维。可加上合同版本标签这一维，就撞上 `psports.CommercialBasisQuery` 的头一句纪律：「它只携带引用：本上下文说明需要哪种依据，**绝不指定应当选中哪个商业版本**」。
- 而同一个闭包的另一项必需依据 `CustomerContractObject` 要解析的，正是「哪一版合同适用」。

换句话说：结算政策要按合同选，合同要由这次闭包解出来。三案对比与事实链取证记于 [commercial-closure-settlement-key/01](../../.scratch/commercial-closure-settlement-key/issues/01-resolution-key-registration-cannot-carry-the-settlement-selector.md)；裁决经用户 2026-08-26 队列答复（「按你倾向的吧」）。

## Decision

**一、闭包解析键上的结算选择器只带三维，合同维必须缺席。**

`ClosureResolutionKey.Settlement` 的齐备判据是 `declaredWithoutContract()`：相对方、费用范围、币种三维在场即算给全；带上合同维则整份键**不受理**。

单依据键（`ResolutionKey`）仍要四维，判据不变。两者的差别有据可循：单依据那条路径上没有任何东西在解合同，调用方不给就没人给。

**二、`ResolveCommercialClosure` 分两段解：先解其余成员，再拿解出的合同版本去解结算政策。**

顺序由 `resolutionOrder` 一处表达——把结算政策排到最后，其余保持声明次序。合同解出后由 `SettlementSelector.WithContract` 补上第四维，形成结算那一项的单依据键。

写成一处排序而不是散在解析里的几个分支，是因为「谁在谁之后」本身就是要被读到的规则；顺序稳定还让 `Adopted()` 与快照里的成员次序不随调用方的声明次序摆动——必需依据集合按什么次序写下来不构成不同的请求（闭包指纹先排序正是这个意思）。

**不做通用拓扑排序。** 正文指名引用之间的相互约束由 `namedReferencesConfirmed` 事后核对，那条路径不需要顺序。真出现第二条依赖时再谈通用解法：现在写一个只有一条边的图算法，读的人得先证明它没有环才敢信。

**三、要结算依据就必须在同一个闭包里要客户合同。** 合同维要由本闭包解出的合同来填；不请求合同就永远没人填得上。这样的键在最小身份处就被拒，不许它走到后面变成一句像是「没有适用结算政策」的话。

**四、合同解不出时，结算政策落进新的一格「前提未解析」，不落`无适用依据`。**

闭包因此报三个清单：`UnresolvedBases`、`ConflictingBases`、`PremiseUnresolvedBases`。分格判据是 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)——恢复动作不同：

| 格 | 这句话的意思 | 去做什么 |
|---|---|---|
| `无适用依据` | 权威说了这个范围里没有这类对象 | 让商业责任方登一份 |
| `前提未解析` | 连问都没问过 | 先把前提（那一版合同）解开；登结算政策没有用 |

压成同一格，运维会照着一句假话去修：补一份结算政策，闭包照样解不开。

**五、结局仍是一个。** 没问过的那一项一并算进「解不出来」，闭包结局照旧只有`适用冲突` / `无适用依据` / `唯一解析`那几种。分格只在报出的清单上。按本裁决的解析顺序，前提塌了必然已经落在前两格之一，「前提未解析」不会单独成立；判定里仍留着这一项，因为它守的是「全有或全无」——少一项就不是唯一解析，这条不该依赖排序恰好把前提排在前面。

**六、快照落库时不写合同维，因为键上本来就没有。** `settlementSelectorDocument` 只有三维。已采用的那一版合同另有去处——它就在 `Adopted` 里，还随结算政策正文的适用范围一起写下；在选择器里再写一遍，等于给同一件事留两个可以互相打架的记录。

这一条与 [ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md) Decision 九同向：解析键必须原样读得回来。落库时顺手把解出的合同补进选择器，读回的键就不再是当初提问的那个键，而重校验正是照这个键重解——它会去问一个没人问过的问题。

**七、「对象/版本」两段式指称只在一处拼。** `CommercialVersion.QualifiedLabel()`。两边必须写出同一个串才对得上：结算政策适用范围里那一维记的是「本约定属于哪一版客户合同」，而闭包解出合同之后要拿这个串去命中它。两处各拼一遍，日后谁改了分隔符，命中就会静静失败——看起来像「这个范围没有结算政策」，其实是两串对不上。

## Consequences

- **PC 领域层从此有一条成员间的解析顺序。** 在此之前 `ResolveCommercialClosure` 对每一项必需依据独立提问，成员之间无序。这是本裁决的主要代价：读这段代码的人得知道有这么一条边。它只有一条、只朝一个方向（合同不引用结算政策，两段之间不会折回来），且集中在 `resolutionOrder` 一处。
- **PS 的登记面因此只需扩三维**（相对方、费用范围、币种），不必登记合同版本标签，`CommercialBasisQuery` 那句纪律保住。登记面与迁移的放行是本裁决的下游动作，同票跟随。
- **租户换合同版本时不必改登记。** 甲案（登记面照收四维）会让每次换版都要有人手工对齐一行登记，而换版本本是商业侧的正常动作；漏改的后果不是报错，是解析不出结果，看起来像「没登记过结算政策」。本裁决把这件事变成解析器的职责。
- **既有快照里那一维被忽略。** `5911d3b` 之后写下的闭包快照可能带 `settlement.contract`；本裁决之后读回时该字段不再解释，键回到三维。对这些快照重跑提交前校验会得到不同的解析身份，因而判`已失效`——那是诚实的：解析语义确实变了，重解一次即可。生产上没有这种行，闭包在生产里至今形不成（PS 登记面明拒）。
- **`PRICE_RULE` 不随本裁决放行。** 价格方向那一维与结算三维不是同一件事，且价格政策的快照重建仍缺席（见 `RehydrateAdoptedBasisSpec` 注释）。它是另一道决定。
- **`CONTROL_SCOPE_NOT_CONFIGURED` 因此将只剩实例半边一个成因。** 那一码今天挤着两件事：账户映射未配置（实例半边，没有租户就没有目录，停在那里是对的）与闭包形不成（机制半边的欠账）。本裁决落地并接通登记面之后，那个码只说一件事。

## Alternatives considered

- **甲：登记面照收四维，含合同版本标签。** 否决：直接撞上「消费方绝不指定应当选中哪个商业版本」。即便补一道交叉核对（解出的合同必须与选择器里的相同，否则整份闭包判`输入未受理`），代价仍是租户每换一版合同就得改一行登记，且漏改是静默的。
- **丙：费用范围与币种不进登记面，随请求过来。** 否决：取证不支持。`CommercialBasisQuery` 今天只有身份与两个单据标识，没有金额也没有币种；给它加维是另一个决定，且首发没有真实报价，逐票币种从哪来说不出。记在此处，免得下一个人重新想一遍。
- **合同解不出时把结算政策一并记进`无适用依据`。** 否决：见 Decision 四。那是拿一次「没问过」冒充一句权威说过的话。
- **为「前提未解析」另立一个闭包结局。** 否决：调用方的动作不因它改变——闭包整体不成立，全有或全无。多一个结局，每个消费方的分支表都要跟着长一格，而那一格与`无适用依据`的处置完全相同。分歧只在给人看的清单上，就只落在清单上。
- **通用依赖排序（拓扑）。** 否决：见 Decision 二。今天只有一条边。

## Links

- [ADR-0044：结算依据经结算政策采用](./0044-settlement-basis-adopts-via-settlement-policy.md)：六维精确匹配与选择器的出处，本记录改的是它在闭包键上的形状
- [ADR-0079：接受前控制策略视图凭商业解析回指提问](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)：把闭包变成生产路径的那次裁决，Decision 九与本记录 Decision 六同向
- [ADR-0029：结果代数按消费方的恢复动作分格](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：「前提未解析」单独成格的判据
- [ADR-0028：重建只校验不重算](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：快照读回解析键必须原样的判据
- [ADR-0027：跨上下文多步协议的中间状态由提供方按解析标识保留](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)：消费方只回显引用、不持有商业内容
- [commercial-closure-settlement-key/01](../../.scratch/commercial-closure-settlement-key/issues/01-resolution-key-registration-cannot-carry-the-settlement-selector.md)：三案对比、事实链取证与实现范围
