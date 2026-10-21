# ADR-0260：评价按范围选卡，计价条款只从价卡读——体积系数与金额取整不分方向都是卡的内容，换算用卡绑定的汇率序列；按范围选卡不读商业价格政策的定价方案绑定，政策不声明体积系数

Status: Accepted（2026-10-21，Claude Code 会话代 PP owner 与 PC owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/07，出处审计 [audit/pc-pp.md](../../.scratch/docs-consistency/audit/pc-pp.md) H4、H5 与 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) B2-02、B2-03。部分停用 [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md) 决定一一句，部分停用 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四「比较币种与换算」一条里的一句、按口解同条下一句的「该口径」并撤掉其 Consequences 的一项，回答 ADR-0148 越权风险点 1，部分停用（按口解）[ADR-0157](./0157-built-in-as-of-forms-are-adopted-as-reference-citations.md) Consequences 一句并回答其越权风险点 3；被停原句与适用场景见「对已接受记录的作用」，三份正文不改，只在其 Status 行与 Links 加前向指针。代码随票 [docs-consistency/code/25](../../.scratch/docs-consistency/code/25-sell-volumetric-factor-and-amount-rounding-are-read-from-the-price-card.md)。越权风险点见文末，待 PP owner 复核；第 1、7 点同时待 PC owner，第 2、5 点待 PC owner，第 6 点待 NR owner。）<!-- 索引注记 -->｜**选卡**：正式评价、接受前估价与试算按范围从 `parcel-pricing` 自己的价卡登记册选卡（折法照 ADR-0179、ADR-0234），不读商业价格政策的定价方案绑定、不与选出的卡核对；绑定只在路由自营段的内部成本上指名方案（ADR-0148 决定四），PC 计价目的的解析无人使用。**计价条款**：体积系数与金额取整是价卡内容，`SELL` 卡与 `BUY` 卡同一归属；商业价格政策口径的体积一格三个方向一律不收。**换算**：用卡绑定的汇率序列，期次按计价基准时点、版本按形成时刻的在用版本；口径的声明仍在商业价格政策上，由序列取值登记的口径引用指名，评价不读政策正文、不执行政策上的取值时点规则、不加点；路由比价同此，路由策略版本所引的价格政策不参与换算。**重启**：选卡改走绑定时，三项随之另立记录
Date: 2026-10-21

## Context

- **选卡有两套说法。** 本记录之前的 PC CONTEXT 把可执行价卡绑定写在商业价格政策上，并把「已解析定价依据」列为 `parcel-pricing` 的输入；CONTEXT-MAP 的 `party-commercial → parcel-pricing` 一边与 GLOSSARY「可执行价卡版本」同一说法。代码只有一套：正式 `SELL` 评价、接受前估价与试算都按（租户、计价范围、方向、目的、时点）经价卡登记册选卡（[ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定三，Context 原句「不走 PC 价格政策的定价方案绑定（ADR-0034）」），`internal/parcelpricing` 不 import 也不经端口读 PC 的价格政策。路由候选成本按段的成本依据引用指名价卡：外包段直接引 `BUY` 卡，自营段引一份 `INTERNAL` 方向价格政策，NR 侧适配器（`InternalPlanResolver`）点读它的定价方案绑定得出方案（ADR-0148 决定四）——那是绑定决定评价用哪份方案的唯一一处。PC 那一侧的计价目的解析（[ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)）没有调用方，生产装配给 `domain.ResolveCommercialClosure` 的方案存续答复是 `nil`，问到即答`解析未决`（`BOUND_PLAN_NOT_CONFIRMED`）。PP 的 `PricingInputSnapshot` 也没有商业依据一格，而 PP CONTEXT「计价输入快照」把「商业依据」列为成员。
- **体积系数与金额取整，文档归政策、代码在卡上。** PP CONTEXT「体积系数」「金额取整策略」与 [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md) 决定一都说销售方向的属商业价格政策；PC 的 `NewVolumetricCaliber` 要求 `SELL` 政策必须声明体积系数引用。PP 的 `PricingPlanVersion` 不分方向携带卡自己的系数与金额取整，`deriveVolumetricWeight` 只读卡上那一份；演示种子的 `SELL` 卡与 `SELL` 价格政策各登了一个 5000。[ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 把「价卡的金额取整声明」列为租户取值，[ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定二按「卡没声明金额取整」记停因，[ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 让取整未声明只由价卡登记续办——三份都站在卡这一边。PC 价格政策口径（`PricePolicyCaliber`）由税务、体积与可缺的汇率组成，从来没有金额取整一格。
- **汇率口径有两个来处。** [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md) 定汇率口径由商业价格政策声明；PP 的汇率序列取值登记必带口径引用（`NewQuotedReferenceSeriesValue` 的 `quoteBasis`，指一版商业价格政策），换算用的是卡绑定的序列。[ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四又要路由比价按「策略版本所引的一份价格政策」的口径换算；NR 登了 `ComparisonPricePolicy`、一路带到候选证据，此后无人读它；[ADR-0189](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 越权风险点 8 写的是「比价与正式评价只认 parcel-pricing 的在用序列」。PC 价格政策的汇率格可以选用「提交接收」一类时点形态，计价侧没有执行它的东西（[ADR-0157](./0157-built-in-as-of-forms-are-adopted-as-reference-citations.md) 越权风险点 3）。
- 结果：同一格有两处各自合法的声明，计价只用卡上那一处，政策上那一处静默不起作用；照文档写新消费方的作者会去解析 PC 价格闭包选卡。

## 候选与反方

- **认商业价格政策：选卡改走绑定，系数、取整、汇率口径从政策读，卡上这几格拒收。** 合同议定的抛比与取整本是合同条款，这条路在业务上更贴近签约。反方：要改正式评价、接受前估价与试算三条选卡路（ADR-0179 决定三「两处同一笔改」），PP 要有一口读 PC 价格政策的端口，PS 的解析键登记面要开计价目的（ADR-0179 Context），ADR-0190、0198、0229 的停因与续办要从卡改挂到政策；首发没有任何客户差异化 `SELL` 价的证据，换来的只是把同一个数挪个地方。否决，留作重启条件（决定四）。
- **两处都留，计价取卡上的，政策上的只作参考。** 一格两处合法，正是审计要拆掉的那种静默失效：租户在合同政策上调了系数而不改卡，计费重量就与预期不符，没有任何地方报错。否决。
- **两处都留，计价核两处相等，不等即冲突。** 形式上一致，实际是让租户永远登两遍同一个数，错的那一遍只换来一次停办。否决。
- **只撤体积系数，汇率口径照旧由路由策略版本所引的政策声明。** 比价与正式评价就用两套口径，同一张卡在两条路上的换算结果可以不同。否决，两条路同一条换算（决定三）。

## Decision

**一、按范围选卡不读商业价格政策的定价方案绑定。**（产品策略）

- 正式评价、接受前估价与试算按（租户、计价范围、方向、目的、时点）从 `parcel-pricing` 自己的价卡登记册选卡，收尾之后的折法照 [ADR-0234](./0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md)；这三条路上它是选卡的唯一权威。
- 路由候选成本照 ADR-0148 决定四按段指名：外包段引 `BUY` 卡，自营段引 `INTERNAL` 方向价格政策、经它的定价方案绑定得出方案。这是绑定决定评价用哪份方案的唯一一处，本记录不改它。
- 销售与采购方向的商业价格政策照旧必须绑定价格方向与定价方案，PC 照 ADR-0034 解析并带回它；在按范围选卡的三条路上，它是这份政策授权哪份方案的商业留痕，不决定计价选哪张卡，也不与选出的卡核对。
- 计价目的的 PC 解析今天没有消费方；生产装配不传方案存续答复（[ADR-0032](./0032-resolution-confirms-references-named-in-the-adopted-content.md) 的入参），问到即答`解析未决`、原因 `BOUND_PLAN_NOT_CONFIRMED`（ADR-0034 决定三）。本记录不补那个提供方。
- 计价输入快照不带商业依据：评价不引用任何 PC 解析。

**二、体积系数与金额取整是价卡内容，不分方向。**（产品策略）

- `SELL` 卡与 `BUY`、`INTERNAL` 卡同法声明系数与金额取整，同法进内容摘要；ADR-0107 决定二至四照旧（应用点、进摘要、未声明即不取整并记问题项）。
- 商业价格政策不声明体积系数：PC 价格政策口径的体积一格三个方向一律不收（销售方向不再必填，带了即`未受理`，与采购、法人间方向今天的判法相同）。已发布的口径行留在册上作历史，不参与评价。税务口径照旧必声明。
- 金额取整从来不在 PC 上，本记录不新增那一格。

**三、换算用价卡绑定的汇率序列；口径的声明在商业价格政策上，评价不读它。**（机制）

- 换算所用的汇率是卡绑定的那串汇率序列：版本取评价形成时刻的在用版本（[ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)），期次按计价基准时点取（ADR-0013 Decision 原句「并在评价时按计价基准时点解析」）。
- 汇率口径仍由商业价格政策版本化声明（ADR-0013）：汇率序列的取值登记以口径引用指名声明它的那一版政策，「不接受未声明口径的裸汇率」在登记门上成立。计价不读那一版政策的正文，也不核它是否真的声明了这一牌价类型。
- 政策上声明的取值时点规则，评价不执行：期次一律按计价基准时点取，政策选了别的时点形态也不改变取哪一期。加点规则 PC 本就没有立（`FxCaliber` 的注释：口径里没有加点那一格）；评价按序列牌价换算、不加点。`FxCaliber` 注释里「消费方据以停在未决」那半句从此不对任何评价成立，随 code/25 改。
- 路由比价同此：路由候选成本换到比较币种，所用汇率与口径就是那张卡绑定的序列；比较币种仍是路由策略版本上的租户取值，路由策略版本所引的价格政策不参与换算。

**四、重启条件。**（产品策略）

正式评价若日后改走商业价格政策的绑定选卡（ADR-0179 决定三「正式路径将来若改走商业价格政策的绑定，两处同一笔改」），体积系数、金额取整与换算序列从哪一份工件读随之另立记录一并裁，不在本记录之下零散回挪。

三类：一、二、三里从哪一份工件读是**产品策略**，读法本身是**机制**；每张卡上的系数、取整、绑哪串序列，每串汇率序列引哪一版政策作口径，是**租户取值**。演示租户：种子卡 `scripts/demo-seeds/data/pricing/price-card-cn-sg.json` 已带系数与取整；种子价格政策口径里的 `volumetricFactor` 随 code/25 撤掉。证据只记 `S`。

## Consequences

- 代码随票 docs-consistency/code/25：PC 价格政策口径的体积一格三个方向一律不收、`SELL` 不再必填，种子与受控批文同步；NR 路由策略版本的比较价格政策一格不再作换算依据。PP 不改。
- PC CONTEXT「商业价格政策版本」「定价方案绑定」「已解析定价依据」词条与 Rules 绑定一条、两阶段解析一条；PP CONTEXT「计价输入快照」「金额取整策略」「体积系数」词条与 Rules 体积、序列登记责任、换算三条；UC-PC-001（新增 `AT-PC-062`）、UC-PC-002（`AT-PC-036` 改写）、UC-PP-001（新增 `AT-PP-013`）随之改。
- ADR-0107、ADR-0148、ADR-0157（按口解）的 Status 行与 Links 挂前向指针。
- GLOSSARY「可执行价卡版本」「价格规则版本」「计价参考序列」、CONTEXT-MAP `party-commercial → parcel-pricing` 一边与外部数值来源一边、network-routing CONTEXT 首发排序形态那一条的换算一句、UC-SA-002 参数表 `PAR-SET-11` 一行、参数登记册与计价设计文档里的旧口径，交文档自洽 10 改。

## 对已接受记录的作用

- **部分停用** ADR-0107 决定一「采购方向的策略属价卡内容，销售方向的属商业价格政策——与体积系数的两侧归属逐字同构（CONTEXT「体积系数」词条）。」一句。**适用场景**：全部方向的评价金额取整——两个方向的策略都是价卡内容。决定一其余（金额取整是价卡内容、不内置任何金额精度常量、不按币种查表）与决定二至四不变。
- **部分停用** ADR-0148 决定四「比较币种与换算」一条里「换算所用的汇率口径（牌价类型、取值时点规则、加点规则）由策略版本所引的一份价格政策声明——PP 不接受未声明口径的裸汇率，本记录不另开口子。」一句；同条下一句「换算由 PP 按该口径、取判断时点在用的汇率序列完成」里的「该口径」按卡绑定的汇率序列所带的口径解；其 Consequences 里「路由策略版本加「比较币种」与所引价格政策两项」的所引价格政策一项不再立。**适用场景**：路由候选成本的比较币种换算。比较币种是路由策略版本上的租户取值、逐段留痕、NR 不持汇率也不自己换算，与决定四其余各条不变。
- **部分停用**（按口解）ADR-0157 Consequences「计价侧没有汇率时点执行器，那一格仍答`未配置`，不在本记录里补执行器。」一句：评价不执行价格政策汇率格上的时点形态，期次一律按计价基准时点取、照常换算，那一格不答`未配置`。**适用场景**：评价换算的期次。同一 Consequences 的「价格政策汇率格可以选用同一引用。」与 ADR-0157 其余各条不变；越权风险点 3「汇率格可以选用，计价侧没有执行器。」随之回答。
- ADR-0148 越权风险点 1「比较币种换算是 PP 换算的延伸」由决定三回答：口径经卡绑定序列的口径引用落地，PP 不读策略版本所引的政策。
- ADR-0013、ADR-0034、ADR-0179、ADR-0189：无条款停用。ADR-0013 的口径声明经序列登记的口径引用落地；ADR-0034 的解析照旧，只是无人用它选卡；ADR-0179 决定三与 ADR-0189 越权风险点 8 正是本记录认的那一边。ADR-0148 决定四自营段经 `INTERNAL` 价格政策的绑定得出方案那一条不变。

## 越权风险点

1. **客户差异化的销售价首发表达不了**（PP、PC owner）。按范围选卡不认客户：同一计价范围只有一张在用 `SELL` 卡，合同议定的不同抛比或取整只能另立计价范围，或等选卡改走绑定（决定四）。首发没有客户差异化 `SELL` 价的证据，这条限制先如实写在 PC CONTEXT 与 UC-PC-002 里。
2. **销售与采购政策的绑定照旧可登、照旧解析，却不决定选卡**（PC owner）。只有 `INTERNAL` 方向政策的绑定经路由自营段决定评价用哪份方案；租户照 PC 管理台给销售或采购政策登了绑定，可能以为它决定计价用哪张卡；绑定的方案被 PP 截止或退役后，PC 那份绑定照旧指着它。本记录不加两侧一致性核对：那要 PC 读 PP 的册或反过来，换来的只是一道没人依赖的门。
3. **政策上的取值时点规则不执行**（PP owner）。政策选了「提交接收」，评价按计价基准时点取期次，两者不同时以计价基准时点为准、不报。要执行它，得先有计价侧的时点执行器，并裁它与计价基准时点谁优先。
4. **口径引用不核**（PP owner）。汇率序列登记时引的那一版政策是不是真声明了这一牌价类型，计价不核；转录与登记错误由登记责任方承担（PP CONTEXT 序列登记责任一条）。
5. **撤掉 PC 销售体积格改了 PC 的登记面**（PC owner）。带 `volumetricFactor` 的 `SELL` 价格政策批文此后答未受理，种子与受控批文工具须同步；已发布的行不动。今天没有真实租户登过这一格。
6. **NR 比价口径**（NR owner）。路由策略版本上登的比较价格政策此后不起作用；租户若要选路按另一套汇率比，照 ADR-0189 越权风险点 8 另立机制。
7. **不加点**（PP、PC owner）。合同约定牌价加若干点的租户今天表达不了：评价按序列牌价换算。加点放在哪份工件（政策、卡还是序列）另立记录裁；在那之前不得把加点后的数登成一串序列——那串数就不再是外部牌价，取值凭证对不上。

## Links

- [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md)：部分停用决定一「采购方向的策略属价卡内容，销售方向的属商业价格政策——与体积系数的两侧归属逐字同构（CONTEXT「体积系数」词条）。」一句
- [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)：部分停用决定四「比较币种与换算」一条里汇率口径那一句，按口解下一句的「该口径」，撤掉 Consequences 的所引价格政策一项，回答越权风险点 1；适用场景限路由候选成本的比较币种换算
- [ADR-0157](./0157-built-in-as-of-forms-are-adopted-as-reference-citations.md)：部分停用（按口解）Consequences「计价侧没有汇率时点执行器，那一格仍答`未配置`，不在本记录里补执行器。」一句，适用场景限评价换算的期次；越权风险点 3 随之回答
- [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md)：汇率口径由商业价格政策声明、期次按计价基准时点解析
- [ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)、[ADR-0032](./0032-resolution-confirms-references-named-in-the-adopted-content.md)：计价目的的 PC 解析与方案存续入参
- [ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)：决定三按范围选卡与「两处同一笔改」
- [ADR-0234](./0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md)：选卡口的收尾折法
- [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)：卡绑序列标识、在用版本由复核派生
- [ADR-0189](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md)：越权风险点 8「比价与正式评价只认 parcel-pricing 的在用序列」
- [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)、[ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md)、[ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md)：金额取整按卡声明与续办
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)、[parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)、[UC-PC-002](../application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md)、[UC-PP-001](../application/parcel-pricing/UC-PP-001-FORM-ESTIMATE-EVALUATIONS.md)
- 票 `.scratch/docs-consistency/issues/07-pc-pp-docs.md`；代码票 `.scratch/docs-consistency/code/25-sell-volumetric-factor-and-amount-rounding-are-read-from-the-price-card.md`
