# 客户服务规则「挂客户合同」的那一格缺选择与回落机制——同一范围挂产品与挂合同的版本无法共存解析，消费侧也不核适用声明

Category: enhancement
Status: in-progress（2026-10-08 开工：按票面「建议路径」进入 `/grill-with-docs`，设计树逐轮采访中）
Blocked by: 无。本票是建模诉求本身，先于一切实现；机制侧证据见正文「代码事实」。

## 来处

`scripts/demo-seeds` 第二客户对比面工作（合成演示租户 SYN-TENANT-01）走到「为账户 02 补具体索赔
资格规则」时暴露的缺口。索赔资格四维里，「保哪些类型」挂合同（VE 0011）、「谁能代提」挂账户
（VE 0018），「首次索赔时限」与「最低材料要求」两维的正文在 `party-commercial` 客户服务规则册
（[ADR-0104](../../../docs/adr/0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)）。
领域的适用声明**已经是两格封闭恰一**——`CustomerServiceRuleApplicability` 支持「随服务产品适用」
或「随客户合同适用」（CONTEXT 原句「按服务产品和客户合同明确适用范围」）。合同维在模型里存在，
`SYN-SERVICE-RULE-01` 只是恰好挂在产品上。真正的缺口在**选择与回落**：想给合同 02 发一版挂合同的
定制规则，今天发不出去也读不对。

## 代码事实

- 提供侧适用声明：`CustomerServiceRuleApplicability` 两格封闭恰一（
  `CustomerServiceRuleAppliesToServiceProduct` / `CustomerServiceRuleAppliesToCustomerContract`），
  正文规范化文档双键并列（`serviceProduct,omitempty` / `customerContract,omitempty`）。
- 闭包解析：`ResolveCommercialClosure` 对 `CustomerServiceRuleObject` 一视同仁，单依据解析的
  `registry.applicable` 按（租户、类别、范围、生效区间）收窄。领域里另有一道产品过滤
  （`admitsDeclaredServiceProduct`，只在解析键携带 `ServiceProduct` 维时生效，按版本壳上
  指名的产品过滤），而解析键登记面（`resolutionKeyDocument`）**没有**该键，这条登记路径上
  产品维恒空、过滤不生效；合同维则全领域没有任何对应的过滤。因此同一范围一旦并存
  「挂产品的一版」与「挂合同的一版」，解析答`适用冲突`——这是 CONTEXT「同一解析键和商业
  选择锚点下，每种必需商业依据必须唯一适用；零个候选形成`无适用依据`，多个候选形成`适用冲突`」
  这一句的直出结果，也意味着合同定制版现有机制下没有可被选中的路。
- 消费侧（`internal/visibilityexception/adapters/partycommercial/claim_service_rules.go`）：
  `translateRule` 按选中的版本点读正文，**不核适用声明是否对上手查询携带的合同 / 产品**；
  `LoadCustomerServiceRule` 里的核只有 `ConsistentCustomerServiceRuleApplicability`（壳与正文
  指向一致）这一道，ADR-0104 Decision 四「这一版挂的是不是我手上这份产品 / 合同」那一层
  没有落地。选择问题一旦解开，挂合同 01 的版本会被合同 02 的索赔照读。
- 回落语义未定：合同版对某一种期限 / 某一种索赔类型无行时，是回落同范围的产品版，还是如实
  「这一版对该格无客户差异」——ADR-0104 Alternatives 否「显式空版本」时依据的是「产品没有
  默认期限」，没有裁合同版 ↔ 产品版的层叠。
- 现实业务的合同定制（同一产品、不同客户不同索赔条款）因此只能走歪路：给每户伪造一条产品链
  （新规则包 → 新产品），把「客户差异」写成「产品差异」——服务产品目录从此混入客户层，是
  本票点名要避免的做法。

## 诉求（已磨：设计树终态）

六格经 `/grill-with-docs` 逐格裁毕，全部落进 ADR 草稿的六条 Decision，逐格归宿：

1. 选择机制 = 镜像结算政策两段 + 合同优先（Decision 一）——机制。
2. 消费侧适用核 = 核合同不核产品，失败落 `ErrUntranslatableAnswer`（Decision 四）——机制。
3. 回落语义 = 行级继承 + 版本级回落（Decision 二），只收紧不删减（Decision 三）——机制 + 产品策略。
4. 规范化 / 摘要 = 零变化，壳 `references` 与既有双键承载全部（Decision 五）。
5. 消费侧翻译分格 = 全部落进既有格（适用冲突 / 无适用依据 / 未登记 / `ErrUntranslatableAnswer`），零新词。
6. 管理台表单 = 实现面，随 `/to-tickets`。

三分类终判：选择、继承、核 = 机制 + 产品策略；「哪个租户给哪个合同挂哪一版、配哪组值」=
租户取值（留空、如实答未配置）。演示证据记 S。

## 建议路径

多会话：`/grill-with-docs` → `/to-spec` → `/to-tickets`；涉及难逆转取舍时出新 ADR 或 supersede
ADR-0104 的相应格，不改写已接受 ADR 历史。票收口判据：上述六格每格有归属（机制 / 产品策略 /
租户取值），机制半边可形成交接任务包。

## Comments

- 2026-10-08 开工：进入 `/grill-with-docs`（`/grilling` 设计树逐轮采访 + `/domain-modeling` 记录）。
  前置事实补齐一处：VE `CONTEXT.md` 索赔节「收到客户索赔……再按申请人授权、客户账户、
  **合同版本**、索赔时限、目标范围、重复关系和最低材料要求判断资格」——消费侧按合同版本
  判断资格是 VE 自己的硬句，Q3（消费侧适用核）的 CONTEXT 依据。

- 2026-10-08 设计树终态（四轮采访，Q1–Q7 逐轮裁定）：
  - Q1 选择机制 = 镜像结算政策两段 + 合同优先；闭包仍唯一采纳。
  - Q2 回落语义 = 行级继承（业务审后重裁：差异条款「未尽事项以产品标准为准」是行业惯例，
    整体替换让「只改一处」的合同形态失真）。
  - Q3 消费侧核 = 核合同不核产品。
  - Q4 指认方式 = 壳 `references`（既有 Consistent 核兜底）——由 Q1 隐含，未单裁。
  - Q5 失败格 = 沿用 `ErrUntranslatableAnswer`。
  - Q6 底座来源 = PC 新增层次读口（与 `LoadCustomerServiceRule` 同族）。
  - Q7 删减表达 = 不支持删减、只继承或收紧（与既有「合同仅可收紧」纪律同源）。
  - 产物：ADR 草稿 [adr-draft-customer-service-rule-contract-tier.md](../adr-draft-customer-service-rule-contract-tier.md)
    （Proposed；接受后移入 `docs/adr` 编 0176，并按 README 制度在 ADR-0104 Status 行与 Links 节
    加前向指针，正文不改写）。
  - grilling 结束条件：frontier 已空、用户已确认共同理解。下一段：接受 ADR 草稿 → `/to-spec` →
    `/to-tickets`。

- 2026-10-08 ADR 已接受：草稿移入 `docs/adr` 编
  [ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md)
  （六条 Decision + 六项 Alternatives）；按 README「部分停用」制度回写 ADR-0104 的 Status 行与
  Links 节前向指针（其 Alternatives「允许显式空版本」条目中「产品没有默认期限」一句的前提由
  Decision 二消解），正文不改写。

- 2026-10-08 `/to-spec` 完成：[spec.md](../spec.md) 已发布（`Category: enhancement` +
  `Status: ready-for-agent`），含 Problem / Solution / 18 条 User Stories / Implementation
  Decisions / Testing Decisions（三缝：PC 域解析缝、VE 适配器装配缝、新增层次读口契约缝——
  经用户确认）/ Out of Scope。下一段：`/to-tickets` 拆实施票。