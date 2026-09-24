# ADR-0152: 运营试算是计价自有的应用用例——按假设包裹对每张适用价卡各形成一份试算评价，只算不存、结构上不交结算、不择优；入口是 `parcel-api` 命令行 `POST /pricing-estimates`，起步未配置、随操作者渠道换 Intake

Status: Accepted（2026-09-25，用户经 IDP 队列答通道 3「按你的理解和建议，你自己独立完成吧」「按你的建议，但是要理解我们的实现方式」授权自决；票 [operator-workspace-gaps/02](../../.scratch/operator-workspace-gaps/issues/02-estimate-evaluation-entry.md) 的产物。越权风险点见「裁决方的能力边界」，同日用户授权通道 3 代为复核、四条维持，见文末「owner 复核记录」）
Date: 2026-09-25

## Context

[PP CONTEXT](../domain/parcel-pricing/CONTEXT.md) 已把试算放进首发：「评价对象为试算对象时，评价不得进入结算，也不得形成客户承诺。产品已确认需要多供应商比价择优，因此试算能力进入首发；正式报价接受、锁价及 Quote 生命周期仍不进入首发」。评价对象词条同样写明试算对象「在包裹尚未存在时由发起试算的一方给出，只在该次试算内有身份」，针对它的评价可服务比价、择优和成本预测，`settlement-accounting` 不得据其形成任何费用、预估费用或应收应付。

运营却没有任何入口：客户问「这个重量、这个尺寸、寄到这个邮编多少钱」，产品答不了。另一处现行口径卡着它——[应用用例 README](../application/README.md) 写 `parcel-pricing`「当前作为 PN-07 的内部纯计价前置能力……不单独建立报价、合同或账务用例；后续只有在确认独立报价接受、锁价或外部计价服务责任后，才新增相应 `UC-PP-*` 用例」。

代码现状（取证钉 `2df79907`）：

- 领域已有试算对象：`domain.SubjectEstimate` 与 `domain.NewEstimateSubject`。输入快照三种给法齐备：调用方给分区（`NewPricingInputSnapshot`）、只给邮编路线由目录解析分区（`NewPostalPricingInputSnapshot`）、两者都给（`WithPostalRoute`）；结算币种可补（`WithSettlementCurrency`）。
- 产品内已有一处试算先例：小包托运的接受前估价（`internal/parcelshipment/adapters/parcelpricing` 的 `EstimationAmountSource`）用试算对象调纯函数 `domain.EvaluatePricing`，**只算不存**。
- 正式评价的编排 `application.EvaluatePricingHandler` 在入册后**无条件**经 `EvaluationHandoff` 交出发布意图；`internal/settlementaccounting` 里没有按评价对象种类过滤的代码（按 `SubjectEstimate` 与 `ESTIMATE` 检索无命中）。试算若走这条编排，就会被交给结算。
- 按价卡绑定补齐序列取值与目录读数（`completeSeriesReadings`、`completeCatalogueReadings`）只写在 `EvaluatePricingHandler` 的方法里；不走它，绑了分区目录或燃油、汇率序列的卡就会落待判断。
- 取价卡有两口：`ports.PriceCardInForceResolver` 答「一份评价该用哪一版」，多份适用是要人裁的冲突；`ports.PriceCardCatalog.LoadApplicable` 交回（方向 + 主要范围 + 计价基准时点）下的全部适用价卡，注释写明是给「多份供应商价卡各形成一份 `BUY` 评价、择优归 `network-routing`」那条路用的，今天没有生产调用方。
- 命令端点的先例是 [ADR-0124](./0124-evaluation-replay-is-triggered-through-a-parcel-api-command-endpoint-and-never-handed-to-settlement.md) 的回放门：`parcel-api` 端点表上一条命令行、以 `UnconfiguredIntake{}` 起步、结果不交结算且「不在结构上」。操作者渠道（[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)）的校验器与信封已进 main，各面换 Intake 的票在途（`.scratch/operator-channel/`，回放端点那一面是 06）。

## Decision

**一、运营试算是计价自有的应用用例 [`UC-PP-001`](../application/parcel-pricing/UC-PP-001-FORM-ESTIMATE-EVALUATIONS.md)，不是报价。** 应用用例 README 那句「不单独建立报价、合同或账务用例」的**适用场景收窄**：不再覆盖运营发起的试算。试算不形成客户承诺、不进结算、不锁价、没有接受动作，所以它不属于那句话要挡的「独立报价接受、锁价」；报价接受、锁价与 Quote 生命周期照旧不进首发，本记录一个字不改它们。

**二、只算不存，且不存、不交都在结构上。** 试算的编排依赖里没有评价库的写口，也没有 `EvaluationHandoff`，装配点装不进去——手法同 ADR-0124 决定五，比一句「不要调」的注释硬。理由四条：

- 试算不形成承诺、不进结算（CONTEXT），入册换不来任何下游用途，只换来被误采用的面；
- 正式评价编排的交付是无条件的，结算侧不按对象种类过滤，入册即交出；
- 同一评价输入、方向、目的、业务时间与版本清单必然得同一结果（CONTEXT 确定性），响应带版本清单与解释，复核照此重算即可；
- 产品内先例（接受前估价）就是只算不存。

代价是试算没有留痕：事后不能按引用读回「那次试算说了多少」。需要留痕的是报价——将来立报价对象时由它自己留，不是把试算改成入册。

**三、补齐读数只有一处实现。** 按价卡绑定解析在用序列版本与期次、在用目录版本与读数的那两步，从 `EvaluatePricingHandler` 的方法抽成两个编排共用的一件，行为一格不改（复核门、说明文字、查不到时冻结「值缺席、版本在」都照旧）。试算与正式评价因此在同一形成时刻、同一版本清单下给同一结果；两处各写一份会在下一次改复核门时分叉。

**四、每张适用价卡各形成一份试算评价，不择优。** 按（租户、价格方向、主要范围、计价基准时点）经 `LoadApplicable` 取全部适用价卡，每张卡各形成一份试算评价，各带自己的版本清单与解释。零张答「价卡未配置」；同一方案身份两版同时适用答「价卡适用冲突」并交回候选（与 `LoadApplicable` 的现行语义一致）。评价之间没有优劣：编排不排序、不标首选，页面也不（择优归 `network-routing`，[CONTEXT-MAP](../domain/CONTEXT-MAP.md) `parcel-pricing → network-routing` 那条）。计算目的由领域配对表按方向成对取得，首发一一对应（CONTEXT）。

**五、输入由发起方声明，不给默认；标识由输入确定性派生；证据层级按定义是 `S`。**

- 声明项：主要范围、价格方向、计价基准时点、实重（值与单位）、尺寸（可缺）、分区与邮编路线、结算币种（可缺）。租户只从信封来，载荷里带即拒。
- 分区给法逐卡定：绑了分区目录的卡用邮编路线由目录解析分区，未绑的卡用发起方给的分区，绑了偏远档位目录的卡另需邮编路线。某张卡该给的没给，这一张单独答「输入不全」并点名缺哪一格，别的卡照算——不拿另一种给法顶替，也不给默认分区。
- 结算币种卡需要而没给，照纯函数的既有处置落待判断，编排不补。
- 试算对象引用与评价标识由（租户、声明项、价卡版本引用）确定性派生：同输入同标识，纯评价可比对可复算。引用由发起方的输入唯一决定，满足 CONTEXT「由发起试算的一方给出」。
- 证据层级：试算的输入是发起方假设的包裹，不是来自真实委托的事实，也不形成任何申报、履约、费用或资金事实——按[验收矩阵](../product/PILOT-ACCEPTANCE-MATRIX.md#证据层级)的定义属受控模拟，编排一律记 `S`，不收触发方声明。这与 ADR-0124 让回放「由触发方声明」不矛盾：回放的输入是历史事实，来源由执行环境决定；试算的输入按定义就是假设。

**六、入口是 `parcel-api` 端点表上的命令行 `POST /pricing-estimates`，以 `UnconfiguredIntake{}` 起步。** 路径叫 `-estimates`：本上下文这一格的动词是试算，答案代数说的也是试算。真 Intake 随操作者渠道族在装配点换，与计价回放同批（operator-channel/06 的地盘）；能力面取「主数据与运营查阅读」——试算不写任何册、不形成任何生产事实，ADR-0100 以「登记一个版本不形成任何生产事实」豁免登记写面，试算连版本都不登。不加隔离放行（[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：合成租户经真渠道进出）。在操作者 Intake 接上之前，演示环境里它如实答未配置。

**七、答案代数只说编排，不替评价说话。** 面上 `outcome` 五格：`FORMED`（带逐卡结果）/ `PRICE_CARD_NOT_CONFIGURED` / `PRICE_CARD_APPLICABILITY_CONFLICT`（带候选）/ `NOT_ACCEPTED` / `UNDECIDED`（带停在哪一口）。`FORMED` 下逐卡一格：`EVALUATED`（带那份评价，其状态完成 / 待判断 / 不可计价 / 冲突 / 失败原样透出）或 `INPUT_INCOMPLETE`（带缺项）。状态码照 [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：形成了答案即 `200`，畸形载荷 `400`，依赖故障没形成答案 `500`。

**八、不做的，逐条写明。**

- 报价、锁价、接受、试算留痕与试算列表。
- 一次请求多个主要范围的比价：页面可以对几个范围各问一次再并排。
- 从客户合同或服务产品解析主要范围：那是 `party-commercial` 的解析，今天由发起方选范围。
- 地址性质与服务选项：输入快照今天不带这两格，随特征来源另立。
- 给结算、路由或任何下游消费试算结果的口。

## Consequences

- `internal/parcelpricing/application` 增试算编排与补齐读数的共用件；`EvaluatePricingHandler` 改用共用件，行为与既有测试不变。
- `internal/parcelpricing/adapters/http` 增试算端点、严格解码的载荷与响应形状；`cmd/parcel-api` 端点表增一行，装配 `UnconfiguredIntake{}`；机制清点随之重生成。
- 管理台增试算页，在端点落地后另拆前端切片；操作者 Intake 接上之前页面如实呈现未配置。
- [应用用例 README](../application/README.md) 的范围句按决定一收窄，并登 `UC-PP-001`。

## Alternatives considered

- **入册但不交付（ADR-0124 形状）。** 得到留痕与按引用回读，代价是每次试算都写一行、评价册混进大量假设评价、查阅面要按对象种类分开，而且入口要挂写能力面。试算不是承诺，留痕的价值在报价那边；不取。
- **复用 `EvaluatePricingHandler`、装配时给一个空交付。** 交付不交付就变成装配约定，而不是结构；结算那侧今天也不按对象种类过滤，一次装错就是一笔被采用的费用。不取。
- **用 `PriceCardInForceResolver` 只算一张卡。** 它把多份适用答成冲突，而试算要回答的恰是「各家各多少」；CONTEXT 点名试算为比价择优进首发。不取。
- **挂 ADR-0077 的目录查阅口（GET）。** 那一族的包约定是「查阅不做评价、不择优、不解析期次，只消费存储读面、不接应用编排」，试算要跑编排、解析在用期次，不属那一族。不取。

## 裁决方的能力边界

本记录由通道 3 受用户经 IDP 队列授权自决。读过：PP CONTEXT 的评价对象、计价参考目录、Rules and invariants 与首发取舍诸句；CONTEXT-MAP 的 `parcel-pricing` 各条关系约束；应用用例 README 全文；验收矩阵证据层级一节；ADR-0100、0124、0150、0151 的 Decision；代码 `application/evaluate_pricing.go`、`application/form_evaluation_from_request.go`、`ports/price_card_in_force.go`、`ports/price_card_catalog.go`、`domain/input.go` 的构造函数、`adapters/http` 的回放门与目录查阅口、小包托运 `EstimationAmountSource`。没读：`settlement-accounting` 消费评价意图的适配器全文（「结算侧不按对象种类过滤」只凭检索无命中）、`network-routing` 的择优实现、操作者渠道 Intake 的实现细节。

因此本记录裁的是**试算归谁、存不存、交不交、取几张卡、入口走哪条缝**，没有裁管理台表单的形状，也没有裁报价。越权风险点单列供复核：

1. 决定一收窄了应用用例 README 的范围口径——那是产品范围取舍，本该由 owner 拍板。
2. 决定五把试算评价的证据层级定为 `S`：试算用的价卡是生产配置，按验收矩阵字面也可以读成靠近 `P`；本记录取「输入是假设」这一面。
3. 决定六把一个 `POST` 命令挂在「主数据与运营查阅读」授予上，与 ADR-0100 授予模型按读写分面的字面分类有张力；备选是新增一个「试算」能力面。
4. 决定二放弃留痕——若业务认为销售口径需要可追溯，应改走「入册不交付」，代价见 Alternatives。

## Links

- [PP CONTEXT](../domain/parcel-pricing/CONTEXT.md)
- [UC-PP-001 按假设包裹形成试算评价](../application/parcel-pricing/UC-PP-001-FORM-ESTIMATE-EVALUATIONS.md)
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0124](./0124-evaluation-replay-is-triggered-through-a-parcel-api-command-endpoint-and-never-handed-to-settlement.md)、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)
- 票 [operator-workspace-gaps/02](../../.scratch/operator-workspace-gaps/issues/02-estimate-evaluation-entry.md)

## owner 复核记录

- 2026-09-25：用户经 IDP 队列答通道 3「你是业务和系统专家，请你自决」，授权通道 3 代为复核四条越权风险点。**这是受托复核，不是 owner 本人认可**；四条逐条维持：
  1. 范围口径收窄：CONTEXT 已把试算能力放进首发，面向运营的入口是它最直接的消费方；试算不接受、不锁价、不形成承诺，那句要挡的是报价。
  2. 证据层级 `S`：证据层级描述的是本次执行的数据来源，假设包裹标 `P` 会把假设冒充成生产证据；价卡是生产配置这一面不改变输入的性质。
  3. 查阅读授予：能力面按语义分，不按 HTTP 方法分；试算不写任何册，用 `POST` 只是因为结构化声明放请求体更合适。另立「试算」能力面会给授予模型添一格没有独立授权理由的面。
  4. 放弃留痕：要追溯的是报价，将来随报价对象自己留；把试算改成入册只会多出一个被结算误采用的面。
