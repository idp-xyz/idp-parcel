# ADR-0176：客户服务规则挂合同的一格按「合同优先解析 + 行级继承产品版」接入——同一范围产品版与合同版并存可解，消费侧按合同点核

Status: Accepted（2026-10-08，经 `/grill-with-docs` 设计树四轮采访、用户逐问裁决；由票 party-commercial-customer-service-rule-contract-applicability/01「申请领域建模」引出。接受后对 ADR-0104 的承接线见本记录 Consequences 与 ADR-0104 的 Links 节前向指针）
Date: 2026-10-08

## Context

领域适用声明**已经是**两格封闭恰一——`CustomerServiceRuleApplicability` 支持「随服务产品适用」或
「随客户合同适用」（PC `CONTEXT.md` 客户服务规则词条原句「按服务产品和客户合同明确适用范围」），
正文规范化文档双键并列（`serviceProduct,omitempty` / `customerContract,omitempty`）。缺的是**接入**：

- 解析段不看正文：`registry.applicable` 按（租户、类别、范围、生效区间）收窄，同一范围并存
  「挂产品的一版」与「挂合同的一版」即答`适用冲突`（PC `CONTEXT.md`「每种必需商业依据必须唯一
  适用；多个候选形成`适用冲突`」的直出结果）——合同定制版没有可被选中的路。
- 消费侧（VE `claim_service_rules.go`）点读正文后**不核**适用声明是否对上手查询携带的合同；
  读口 `LoadCustomerServiceRule` 只有「壳与正文一致」一道核（`ConsistentCustomerServiceRuleApplicability`）。
- 回落语义未裁：合同版某行无行时的行为，`ADR-0104` Alternatives 只否了「显式空版本」
  （「产品没有默认期限」），没有裁合同版 ↔ 产品版的层叠。
- VE `CONTEXT.md` 索赔节硬句：资格按「申请人授权、客户账户、**合同版本**、索赔时限、目标范围、
  重复关系和最低材料要求」判断——按合同版本判断是 VE 自己的事。
- 业务现实（演示租户 S 证据暴露、行业惯例佐证）：小包大客户协议通行**差异条款**形态
  「未尽事项以产品标准为准」，少数条款另定；「把产品条款整本抄一遍再改两处」不是常态。

## Decision

**一、解析段镜像结算政策的两段纪律（ADR-0080），合同优先。** `resolutionOrder` 把
`CustomerServiceRuleObject` 与结算政策同排第二段（合同解出之后）。新增解析分支
`resolveCustomerServiceRuleBasis`：候选先取**壳 `references` 恰指名闭包解出的客户合同版本**的
规则版本；没有时取「挂服务产品」的候选；同层多候选=`适用冲突`，两层皆零=`无适用依据`，其余
`解析未决` 照旧。闭包仍**唯一采纳**一版——「唯一适用」硬句不破：有合同版采纳合同版，没有则
采纳产品版（等同今天的单版行为）。

**二、行级继承 + 版本级回落。** 闭包采纳合同版时，行级读取经 PC 内容面**新的层次读口**完成：
一次调用按（租户、范围、合同版本、锚点）取回（合同版正文、产品底座版正文、各自在场标志），
底座版在层次读口内选「同范围挂服务产品、锚点生效」的版本，多候选照`适用冲突`纪律答。
VE 按行拼接：合同版有行用合同版，无行取底座版该行。拼接口在 VE 适配器，两份正文与在场标志
都交回，VE 不自行解析（解析位在 PC，不造第二套口径）。合同版那一行在底座版也不在时，
照既有「未登记」一维如实答（正是「该格无客户差异且无继承物」）。

**三、只收紧不删减。** 合同版行只能是「继承（无行）」或「收紧（有行且清单更短、期限更严）」；
不提供「删去某类期限 / 材料」的显式空行。判据与既有「合同仅可收紧不得放宽」纪律（保价条件、
交付条件）同源——继承位与删减位落在同一个「无行」上时两者同形，宁可少一个表达也不造歧义。

**四、消费侧按合同点核，失败落既有格。** VE 适配器点读后核「正文挂合同必须等于查询携带的
合同」；不等答 `ErrUntranslatableAnswer`（提供方契约一族，不新立错误值）。挂产品的版本不核
（查询无产品维），由解析唯一性保证。「壳 ↔ 正文一致」仍由既有 `ConsistentCustomerServiceRuleApplicability`
把关（发布写入前与点读后各一次），与本节这一道分属两层、各守一边。

**五、规范化与摘要零变化。** 选择靠壳 `references`（既有键），正文双键既有——不新增规范化
文档键，PCC-1 一字不改（若要改则按加 omitempty 键不换号的先例另核）。

**六、不做的。** 不改 `CONTEXT.md` 语言（本次未产生新词）；不动闭包唯一采纳形态；管理台服务
规则表单的合同格录入与预览键归实现票，不在本记录裁。

## Consequences

- 落地位：`resolutionOrder` 与 `ResolveCommercialBasis` 新分支（PC 域）；层次读口（PC ports +
  postgres + application，与 `LoadCustomerServiceRule` 同族）；VE 适配器的核与按行拼接；管理台
  表单（随实现票）。
- VE 行为变化：合同版接进后，「合同版无行」不再永远是「未登记」——有底座版时答继承值；
  只有两层皆无才落「未登记」。已接受的委托按接受时固定的版本读取，不受后续发布影响
  （冻结引用纪律不变）。
- 对 ADR-0104 的承接关系：ADR-0104 Alternatives「允许显式空版本」条目中「客户服务规则没有
  对应的回退物——产品没有默认期限」这一句的前提由本记录 Decision 二消解（回退物＝产品版
  该行）；ADR-0104 Decision 三「子行至少一项、不允许显式空版本」对**版本级**继续成立
  （合同版仍至少一行）——被停的只是该 Alternatives 条目里那一个前提，其余各条不变。已按
  README「部分停用」制度在 ADR-0104 的 Status 行与 Links 节加前向指针，其正文不改写。
- 三分类界限：选择、继承、核全部是**机制 + 产品策略**；「哪个租户给哪个合同挂哪一版、
  配哪组时限与材料」是**租户取值**，留空如实答未配置。演示证据记 S。

## Alternatives considered

- **保存面禁止同范围并存（发布时拒第二挂法）。** 否决：直接杀死按户定制，本票目的落空。
- **消费侧自筛（VE 自己从候选里挑）。** 否决：解析位在 PC，VE 侧挑等于第二套口径，且破
  「唯一适用」与解析可续办机制。
- **闭包为服务规则开双槽（同时采纳合同版与产品版）。** 否决：与「每种必需商业依据必须唯一
  适用」正面冲突。
- **行级不回落（整体替换条款）。** 否决：与差异条款的商业惯例相悖——客户只改一处时限就得
  整本抄写产品条款，否则其余各行落入未登记；「未尽事项以产品标准为准」是更普遍的合同形态。
- **显式空行表达删减。** 否决：与 ADR-0104 否「显式空版本」同一条歧义——继承位与删减位
  同形，读的人答不出哪一个。
- **产品核也做在 VE（经 PS 回指把目标包裹的产品解出来）。** 否决：多一段跨上下文取数与
  失败面，而产品版正确性已由解析唯一性保证。

## Links

- 票 [01：挂合同的那一格缺选择与回落机制](../../.scratch/customer-service-rule-contract-applicability/issues/01-service-rule-keyed-by-product-cannot-express-per-customer-claim-terms.md)
- [ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)：正文归属、首发两项、解析走闭包与点读口——本记录承接其 Alternatives 一处前提
- [ADR-0080](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)：两段解析纪律
- [PC CONTEXT](../domain/party-commercial/CONTEXT.md)：客户服务规则词条与「唯一适用 / 适用冲突」硬句
- [VE CONTEXT](../domain/visibility-exception/CONTEXT.md)：索赔资格按合同版本判断的硬句