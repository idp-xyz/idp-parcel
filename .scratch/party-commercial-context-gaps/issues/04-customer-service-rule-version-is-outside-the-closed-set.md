# 客户服务规则版本连九值封闭集都没进，而 VE 的响应目标与索赔期限本该以它为依据

Category: chore
Status: draft
Blocked by: 无

## CONTEXT 要求什么

`docs/domain/party-commercial/CONTEXT.md` 给了它 Language 词条、两条 Rules 硬句、一节独立
生命周期，以及 Boundaries 里的一句点名——**规格完整度与票 01 相当**。

Language 词条原文：

> **客户服务规则版本**
> 服务产品或客户合同在明确期间内采用的追踪披露、异常响应、客户更新、通知义务、索赔期限和最低
> 材料等服务规则。它表达可复用的商业责任和差异化条件，不拥有具体追踪投影、异常信号、案件、
> 通知或索赔结论。

Rules 两条：

> 客户服务规则必须按服务产品和客户合同明确适用范围、有效期间及责任方。追踪披露、异常响应、
> 通知义务、索赔期限和最低材料可以具有客户差异，但不得把内部异常检测阈值、事实有效性或最终
> 赔付金额写成客户可以直接覆盖的商业配置。

> 客户服务规则版本变化只影响其声明生效范围内的新判断。`visibility-exception` 保存具体案件、
> 通知和索赔实际采用的规则依据；后续商业变化不得覆盖已经形成的响应目标、披露决定、通知内容
> 或索赔资格历史。

「客户服务规则版本」生命周期小节：

> - 草稿 → 已发布：固定追踪披露、异常响应、客户更新、通知和索赔条件及其适用范围。
> - 已发布 → 已生效：由服务产品或客户合同在明确期间引用；具体业务保存实际采用依据。

Boundaries：

> `visibility-exception` 拥有追踪投影、异常信号和案件、响应目标、客户披露及通知决定、客户索赔
> 项和追偿事项；`party-commercial` 只拥有服务产品、客户合同及客户服务规则版本，不能直接形成或
> 修改这些实际业务结果。

## 代码里实际有什么（取证 `9d6063c`）

**什么都没有——比票 01 还少一层。** 票 01 至少有领域模型，这一件连类型都不存在。

`CommercialObjectKind` 是九值封闭集，由
`TestCommercialObjectKindsStayIndependentAndClosed` 钉住，九值是：

    ServiceProductObject
    CustomerContractObject
    SupplierAgreementObject
    AcceptanceRulePackageObject
    PreAcceptanceFinancialControlPolicyObject
    PriceRuleObject
    SettlementPolicyObject
    CreditPolicyObject
    AuthorizationRuleObject

**客户服务规则版本不在其中。** 后果是它连版本壳都入不了 `commercial_version`
——票 03 那两件至少能入册、能被解析选中，这一件在第一道门就进不去。

## 另一半：`visibility-exception` 那侧也没有对接口

这一条要说准，别只怪一头。

`docs/domain/visibility-exception/CONTEXT.md` 的 Boundaries 里对本上下文的表述是概括的：

> `party-commercial` 拥有货主客户账户、参与方、责任法人、服务产品、客户合同、供应商协议和商业
> 规则版本；本上下文引用这些依据形成响应、披露、索赔和责任判断快照，不修改商业版本。

**「商业规则版本」是个统称，VE 的 CONTEXT 全文没有出现「客户服务规则版本」这个词。** 而它的
索赔一节写了非常具体的期限纪律：

> 客户首次索赔期限、资料补充期限和结论复核期限是三个独立期限。每个期限必须保存适用规则版本、
> 起算事件、业务时区或日历、截止时间和适用范围；不能用其中一个期限代替或重置另一个期限。

> 复核期限的起算必须引用合同规定的结论通知、送达或可获取事实；无法确认起算事实时保持期限
> 待判断，不得默认从内部结论时间起算。

「必须保存适用规则版本」——那个版本本该就是客户服务规则版本。

清点上这条链是断的：`docs/product/MECHANISM-INVENTORY.md` 的跨上下文消费缝里，
`visibilityexception` 只连到 `customscompliance`、`networkrouting`、`nodeoperations`、
`parcelshipment`、`transportfulfillment` **五个提供方，没有 `partycommercial`**。而
`parcelshipment`、`networkrouting`、`settlementaccounting` 三个上下文都各自连着
`partycommercial`。**十二个上下文里，VE 是唯一一个 CONTEXT 明写要引用商业依据、清点上却零消费缝的。**

## 差在哪儿：责任链断在两头之间

VE 那侧不是没做期限——它把期限做得很细（三个独立期限、起算事实待判断不默认）。缺的是**期限
从哪来**。今天 VE 的响应目标与索赔期限只可能有两个来源：写死，或者由 VE 自己拥有。两者都与
CONTEXT 冲突——它明说这些是 `party-commercial` 拥有的可复用商业责任，且**可以具有客户差异**。

「可以具有客户差异」这半句是关键：没有客户服务规则版本，就没有地方表达"这个客户的索赔期限
是 60 天、那个是 30 天"。而差异化服务条件恰恰是合同谈判里最常动的东西。

## 补与不补，各自的连带

### 若补，这一票比另外三票都大

因为要动封闭集，而封闭集是本上下文最靠底的那个东西：

- **`CommercialObjectKind` 从九值变十值。** 连带 `commercial_version` 的 `object_kind` CHECK
  区间（`0004_commercial_version_kind_range.sql`）、所有按 `object_kind` 判别的正文表 CHECK、
  以及 `TestCommercialObjectKindsStayIndependentAndClosed` 的九值清单。这是一次**跨文件的
  封闭集拓宽**，按 `docs/agents/parallel-sessions.md` 属"会让旧调用点对不上"的那一类，动之前
  要在频道占号。
- **领域类型从零建**：追踪披露、异常响应、客户更新、通知义务、索赔期限、最低材料六类内容，
  加适用范围（服务产品 + 客户合同）、有效期间、责任方。
- **正文表 + 端口 + 读面**三层，形状可照票 03 说的那套。
- **VE 那侧要开一条消费缝**：按解析结果取回适用的服务规则版本，并在案件/通知/索赔上保存实际
  采用依据。CONTEXT 已经要求 VE「保存具体案件、通知和索赔实际采用的规则依据」，缝接上之后这
  句才落得了地。
- **VE 的 CONTEXT 可能要补词**：把「商业规则版本」这个统称在索赔与响应两处落实成
  「客户服务规则版本」，否则接线时没有共同的词可指。

### 若不补

- 需要有人明说：首发不支持客户差异化服务规则，响应目标与索赔期限**由 VE 按统一口径拥有**。
  这是一个正当的首发范围裁决，但它同时改的是**两个上下文的所有权划分**——`party-commercial`
  的 Boundaries 那句「只拥有服务产品、客户合同及客户服务规则版本」要改，VE 那句「引用这些
  依据」也要跟着改。**跨上下文所有权变更按 [AGENTS.md](../../AGENTS.md) 要走 ADR，不能只改
  两份 CONTEXT。**
- 代价是 CONTEXT 里一整节生命周期与两条硬句永久空转。

### 第三条路在这一票上尤其值得考虑

四票里这一票**改动最大、首发业务紧迫度最低**（无租户则无差异化合同可谈）。把它写成明认
「已知未建，重启条件是第一个租户提出差异化服务条件」，可能比现在补一整套更诚实——但**明认
必须写下来**，因为现在的沉默看起来像是漏了。

## 边界

本票**不动封闭集、不建类型、不写迁移、不接 VE 的缝**，只把差异摆到可裁的形状。

特别地：**本票不擅自拓宽 `CommercialObjectKind`。** 那是一次难逆转的跨文件改动，且按
`parallel-sessions.md` 会让共享树对所有人编不过，必须先裁后做、做前占号。
