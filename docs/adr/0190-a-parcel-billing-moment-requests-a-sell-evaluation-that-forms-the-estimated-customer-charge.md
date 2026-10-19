# ADR-0190：包裹计费时点发起 SELL 评价，形成预估客户费用——独立的 SELL 请求身份，内置「有效网络收寄已采用」「有效交付已终局」两种时点，按结算账户登记采用

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，其后又令「继续」，按会话里报告过的次序先做客户费用。取代 [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) Consequences「形成编排不进 `cmd/parcel-dispatch`」与决定四「也不在本记录里把形成编排接进进程」，以及 [ADR-0170](./0170-settlement-orchestrations-enter-through-parcel-api.md) Context「SELL 评价形成客户费用的编排不进 `cmd/parcel-dispatch`」与「候选与反方」里否决「把 SELL 形成编排接进派发」那一条；为 [ADR-0171](./0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md) 决定五在 SELL·客户费用预估这一格作答。越权风险点见文末，待 SA、PP owner 复核。）

Date: 2026-10-19

## Context

- 一件已经交付的包裹，在 main 上形成不了客户费用（演示动线实测，[product-strategy-boundary/05](../../.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 重走一节）。
- ADR-0169 把 SELL 评价何时发起挂在运输收费发生项的原因上，内置时点只有「发生项形成」。可发生项的定义是「可能依据供应商协议形成外部运输成本的业务事实范围」，而 transport-fulfillment `CONTEXT` 说自营履约「不虚构外部供应商、供应商协议或外部运输委托」，[ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md) 决定三据此判「自营揽收失败不形成发生项」。两条合起来，自营包裹永远等不到发起 SELL 的那一拍。向客户收费不该以向供应商付费为前提。
- SELL 也没有自己的请求身份。BUY 的请求身份（`CalculationPurpose`、发生项 / 费用项目 / 供应商协议三件引用）装不下包裹计费，ADR-0169 已否决往里加 SELL；计价侧的受理门也要求请求点名发生项。
- 计价领域已有 `PricingDirectionSell` 与 `PricingPurposeCustomerCharge`。受理前估价（[ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)）已按（租户、计价范围、SELL、客户费用、时点）选卡，只算不存。
- 结算的事件驱动编排都住在 `cmd/parcel-dispatch`：全仓只有它装了 inbox 与派发器。ADR-0169、0170 却写明 SELL 形成编排不进派发，于是它没有任何进程可住。

## Decision

**一、SELL 评价请求是独立的请求身份（机制）。** 结算新立一种请求：铸造标识，自然键（租户、包裹、计费时点）只守幂等。请求带：租户、包裹、结算账户、结算币种（登记请求时从不可变的结算账户行快照，[ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md)）、计价范围、计费时点、来源事实引用（收寄采用键或终局键）、计价基准时点。不带金额、换算或卡。请求册记形成状态；已形成时同时记下那笔预估客户费用的标识，按包裹找当前有效的预估客户费用经由这一格。经 Outbox 交出只带（租户、请求标识）的信封，计价以 inbox 消费者接收、按标识回读请求。BUY 的请求身份与触发册一字不动。

**二、内置两种包裹计费时点（产品策略）。** 封闭集合：

- `有效网络收寄已采用`：小包托运采用了一份有效网络收寄（不采用不算）。计价基准时点取这件包裹的责任起点。
- `有效交付已终局`：小包托运把这件包裹采用为终局「有效交付」。计价基准时点取终局的生效时点。

不采用「委托已接受」：那时服务还没发生，而网络服务包裹只允许在有效网络收寄形成之前取消（parcel-shipment `CONTEXT`），在接受时计费就会让收寄前取消的包裹背上一笔没有失效路径的费用；它还与受理前估价重叠。一个结算账户只采用一种时点；同一件包裹在所采时点只发起一次。

**三、采用即登记，没登记答未配置（租户取值）。** 结算新开一册「SELL 计费时点采用」：键是（租户、结算账户），值是（计费时点、计价范围），不设修订，照 [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md) 与 ADR-0179 决定五的不可变元组先例。包裹到结算账户的链与受理前财务控制同一条：小包托运在接受时固定的商业解析 → 参与方与商业的闭包里的结算政策 → 结算账户册。册上没有这一户，或这一户采用的不是这次到来的时点，就不发起，消费入账、不重投。不预选时点，不登任何真实租户的行。

**四、选卡与输入（机制）。** 计价按（租户、计价范围、SELL、客户费用、计价基准时点）解析在用价卡，与受理前估价同一口径：唯一命中才形成，零命中是`未配置`，多于一版是`适用冲突`。输入快照按包裹建：

- 计费重量与尺寸照 ADR-0171 决定二取源：有仍有效的实测取实测；实测读口缺席或答没有这条实测，取已接受基线上的申报，事实引用标 `declaration`。对 ADR-0171 决定五在「SELL·客户费用、预估阶段」这一格的作答是：不拒用申报。确认阶段要不要拒用申报，不在本记录。
- 邮编路线取小包托运的寄件到收件，分区由卡绑定的目录解出（ADR-0109、ADR-0189 决定一）。
- 结算币种取请求上快照的那一格。

**五、形成（机制）。** SELL·客户费用评价已记录之后，结算的 SELL 消费门接住它：

- 经「费用项目映射」册（键：租户、计价费用代码；值：费用项目、登记依据）把评价的全部费用行映到同一个费用项目，才形成预估客户费用。任一费用行没有映射、或全部费用行映到多于一个项目，一律答待判断（`AT-SA-174`），不建占位费用，也不拿价格代码当费用项目。映射行是租户取值。
- 费用标识由 SELL 请求标识确定性派生，同一请求至多一笔预估客户费用。
- 评价待判断、不可计价或冲突时入账，不重投，状态记在请求册上。计价侧没形成评价（未配置、适用冲突、输入不可得）时不发评价已记录信封，请求停在「已提交」。

**六、两只编排都住在 `cmd/parcel-dispatch`（机制）。** 时点到来发起请求，评价已记录形成费用，这两只编排都由派发器驱动，各有自己的 inbox 与未决哨兵。取代 ADR-0169 Consequences「形成编排不进 `cmd/parcel-dispatch`」、决定四「也不在本记录里把形成编排接进进程」，以及 ADR-0170 Context「SELL 评价形成客户费用的编排不进 `cmd/parcel-dispatch`」与它否决「把 SELL 形成编排接进派发」那一条。ADR-0169 的消费门、形成编排与发生项原因触发册不动，与本记录的包裹时点册并存。

三类：

- **机制**：SELL 请求身份、请求册与信封；计价侧的 SELL 请求消费者与按包裹建快照；SELL 消费门、费用项目映射与形成；包裹到结算账户的链；派发接线。
- **产品策略**：内置的两种计费时点与各自的计价基准时点；一户一时点、一件一次；一份评价映一个费用项目。
- **租户取值**：每个结算账户采用哪种时点、哪个计价范围；费用代码映到哪个费用项目；价卡的金额取整声明。演示租户经种子登 `SYN-` 值，证据只记 `S`。

## Consequences

- 结算迁移新立三册：SELL 评价请求、SELL 计费时点采用、费用项目映射。`parcel-settlement-register` 增两条登记命令。
- 计价的请求受理门从「必须点名发生项」放宽到「发生项与包裹二者有一」；按包裹建快照的那一半从现有解析器里拆出来共用。
- 派发路由表上，`parcel-shipment.network-intake.recorded`、`parcel-shipment.final-outcome.formed` 与 `parcel-pricing.evaluation.recorded` 都改成扇出，各路记各自的 inbox。
- 要随本记录同步的文档：settlement-accounting `CONTEXT`「哪些发生项原因发起 SELL 评价另册登记」一句、UC-SA-002 步 2、`CONTEXT-MAP` 的 `settlement-accounting → parcel-pricing` 那一段、ADR 索引。
- 演示租户的金额链（[demo-chain-gaps/09](../../.scratch/demo-chain-gaps/issues/09-demo-price-cards-declare-no-amount-rounding.md)、[10](../../.scratch/demo-chain-gaps/issues/10-demo-tenant-buy-and-sell-trigger-registers-are-empty.md)）随实现票登种子。验收：同一件包裹的预估客户费用与受理前冻结同额。

## 越权风险点

1. **预估阶段没有实测时用申报**（SA、PP owner）。UC-SA-002 `AT-SA-037` 要求「不把声明直接当计费重量」。本记录只在预估阶段、且没有实测时用申报，并标明来源，确认阶段不放宽。owner 也可以裁预估同样要实测，那样演示要等实测口落地。
2. **一份评价只映一个费用项目**（SA owner）。按费用行拆成多个费用项目的形态没有内置。演示把附加费并进运费，是 `SYN` 简化。
3. **请求卡死**（SA owner）。计价侧没形成评价、或评价待判断、不可计价、冲突之后，同一自然键不再发起；之后补登了价卡或映射也不回补。续办路径（例如照 [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 发「参数已登记」类信封）另立票。
4. **与发生项原因那一路的关系**（SA owner）。ADR-0169 的发生项原因触发册今天仍没有请求面：本记录的 SELL 请求自然键里，计费时点只收两种包裹时点，不收发生项。后继若给那一路铸请求，同一件包裹会在两路各形成一笔预估客户费用；费用项目映射按计价费用代码登记、与路无关，区分不了两路。两册要不要互斥，留给那一路的后继记录裁。
5. **收寄之后的停止服务与更正**（SA owner）。收寄后的包裹不能再取消，停止服务走服务终止或退运；本记录不让已形成的预估客户费用随之失效。收寄更正、终局失效或重派生也不重算。
6. **进程归属改判**（SA owner）。本记录取代了两份已接受记录里「不进派发」的句子，理由见 Context「结算的事件驱动编排都住在 `cmd/parcel-dispatch`」那一条。
7. **形成的是预估，不建预估到暂估的转移**（SA owner）。两种时点都在责任起点或之后，照 settlement-accounting `CONTEXT` 的生命周期已够得上暂估。本记录沿 ADR-0169 决定二只形成预估客户费用，预估到暂估的转移不在本记录。
8. **不在本记录范围的几类包裹与形态**（SA、PS、PC owner）：
   - 面单渠道服务包裹：责任起点是面单交易，终局是面单服务终局，两种时点都不会触发。
   - 找不到接受时固定的商业解析的包裹（例如日后谱系里拆分、合并出来的包裹）：发起编排答「包裹无商业解析」并入账，不计费，也不留痕；恢复动作在小包托运那一侧。
   - 采用「有效交付已终局」的结算账户，退运、销毁、丢失的包裹不计费。
   - 按包裹建快照表达不了票级、多件计价形态（ADR-0111）。
   - 计价基准时点由内置时点给出；GLOSSARY 说它由商业价格政策和可执行价卡明确规定，归 PC owner 复核。
   - 卡币种与结算币种不同而卡没绑汇率序列时，评价答待判断，随后落进第 3 条那一格。
9. **计价范围登两处**（SA owner）。同一个结算账户在 ADR-0179 的估价采用册与本册各登一份计价范围，两册允许不同；不同时，受理前估价与随后的预估客户费用会用不同的卡，与 ADR-0179 决定三「同一条规则选卡」的意图有张力。演示租户两册登同一个范围。

## Links

- [ADR-0169：SELL 评价形成客户费用，何时发起另册登记](./0169-sell-evaluation-forms-a-customer-charge.md)：部分取代
- [ADR-0170：结算编排从 parcel-api 进入；确认与截单没登记就不做](./0170-settlement-orchestrations-enter-through-parcel-api.md)：部分取代
- [ADR-0171：计价输入在没有实测读口时用申报，并标明来源](./0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md)：决定五在本格作答
- [ADR-0179：受理前控制金额由结算按委托当前提交版本的逐件 SELL 试算合计形成](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)：选卡口径与采用册键形
- [ADR-0098：失败尝试费发生项不在揽收编排里形成](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md)：决定三
- [ADR-0164：BUY 评价请求的触发面是发生项形成，原因没登记就不发起](./0164-buy-evaluation-request-trigger-is-occurrence-formed.md)：BUY 那一半不动
- 票：[sell-customer-charge](../../.scratch/sell-customer-charge/issues/)
