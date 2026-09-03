# ADR-0104：客户服务规则版本的正文归 `party-commercial`，首发只进「索赔期限」与「最低材料」两项且子行至少一项；VE 既有的通知策略册与索赔资格声明册是 VE 自己的判断参数，不迁移、不改键；VE 对规则的采用只冻版本引用，解析走既有闭包加一个点读口

Status: Accepted（2026-09-03，用户经 IDP 队列通道 3 授权本会话「有全部权限」自决并向各会话派工。裁决能力边界：读过票 [party-commercial-context-gaps/05](../../.scratch/party-commercial-context-gaps/issues/05-customer-service-rule-version-has-no-consuming-seam-into-visibility-exception.md) 全文（含 MCP-2 于 `877444a` 的取证与「待 owner 裁的三处」）、`party-commercial` `CONTEXT.md` 中「客户服务规则版本」词条、Rules 两句与 Boundaries 那一句、`visibility-exception` `CONTEXT.md` 索赔一节中期限与材料的硬句、`internal/partycommercial/domain/customer_service_rule.go`、`internal/visibilityexception/adapters/postgres` 的 `claim_eligibility.go` 与 `notification_policy.go` 头注、`migrations/party_commercial/0014` 头注；未重读 VE `CONTEXT.md` 披露与通知各节全文、`UC-VE-007` 与 `UC-VE-008` 全文、VE 编排代码——本记录因此只裁**正文归谁、首发进几项、VE 怎么采用**三件，不改任何一条 VE 不变量、不动 VE 的表与编排）
Date: 2026-09-03

## Context

`party-commercial` `CONTEXT.md` 把客户服务规则版本判给本上下文，并写明它可以承载客户差异的六项（追踪披露、异常响应、客户更新、通知义务、索赔期限、最低材料）；同一节要求 `visibility-exception` 「保存具体案件、通知和索赔实际采用的规则依据」，Boundaries 更写死「`party-commercial` 只拥有服务产品、客户合同及客户服务规则版本，不能直接形成或修改这些实际业务结果」。

代码今天是这样的（票 05 取证锚 `877444a`，本记录复核了下列几处）：

- PC 侧有领域骨架 `CustomerServiceRuleVersion`（两格封闭适用声明 + 责任方 + 范围），封闭集已拓宽（`0019`），**没有正文表、没有解析口、没有消费缝**。
- VE 侧 `claim_eligibility.go` 头注明写：索赔时限与最低材料两维「仍属 `PAR-VIS-08` 待登记实例参数且没有登记面。**凑一份就是发明实例参数**」，两维 `Registered` 恒为假，编排据此停在指名到维的未决。
- VE 侧另有两本自己建的册：通知策略（`notification_policy.go`，`PAR-VIS-07`，键是**披露策略引用**）与索赔资格声明（合同责任范围承不承担某索赔类型，`PAR-VIS-08` 合同角，键是**客户合同**）。

票 05 把这一处摆成「不是接一条缝，是两侧各有一半册子，先裁谁拥有」。它摆对了，但有一个前提要先纠正：VE 那两本册子今天承载的**是不是**客户服务规则的正文。

看键。通知策略按披露策略引用取行，答的是「这一条披露该走什么渠道、限时多少、按哪条判据算满足义务」——那是 VE「客户披露及通知决定」这项它自己拥有的业务结果的**判断参数**，键上没有客户；索赔资格声明按客户合同取行，答的是「这份合同的责任范围承不承担这个索赔类型」——那是 VE 「客户索赔项」资格判断的参数，它引用的是**客户合同**（PC 拥有）而不是客户服务规则版本。两本册的键都不是（服务产品版本 / 客户合同版本 + 业务时点 → 规则版本）这一形。**它们不是长错了地方的客户服务规则正文，是 VE 自己的判断参数**——只是其中通知策略这一本，将来若租户要按客户差异化通知义务，会需要从 PC 的规则版本里读一项输入。那是消费关系，不是所有权错位。

再看 VE 缺什么。索赔期限与最低材料两维，VE 的注释写得很干脆：连登记面都没有，凑一份就是发明实例参数。这两维恰好是 PC CONTEXT 点名归客户服务规则的六项里、**唯一没有任何一侧承载**的两项。VE 不能凑（那是替租户定规则），PC 是 CONTEXT 指名的所有者——正文该落在 PC，VE 按 CONTEXT 只保存「实际采用的规则依据」。

## Decision

**一、客户服务规则版本的正文归 `party-commercial`，这是重申 CONTEXT 不是新裁。** VE 不得为这六项建自己的规则正文册；VE 已有的两本册（通知策略、索赔资格声明）**不是**这六项的正文（理由见 Context「看键」一段），维持为 VE 自己的判断参数，键不改、表不动、`PAR-VIS-07` / `PAR-VIS-08` 的登记册行不变。将来 PC 规则版本承载通知义务那一项时，VE 通知策略从它读一项输入——那是一条消费缝，届时另立，不迁册。

**二、首发只进「索赔期限」与「最低材料」两项，且以正文形态进。** 六项里只有这两项两侧都无承载、且 VE 今天就因它们停在未决（`Registered` 恒假）。它们在 PC 是**正文**而不是引用——被引侧不存在，也不该存在（VE 的注释已经拒绝造它）。形状：索赔期限一项按 VE CONTEXT 期限硬句所要求的**规则侧**要素成行——期限种类（首次索赔 / 资料补充 / 结论复核，封闭三格，与 VE 「三个独立期限」逐字对应）× 起算事件（封闭集，取 VE 词）× 时长 × 业务日历或时区引用；最低材料一项按索赔类型成行，每行一份材料条目清单。**任何时长、日历、材料清单的取值一律不出现在仓库**——形状是产品的，值是租户登记的；红线两个方向都禁。另四项（追踪披露、异常响应、客户更新、通知义务）**不进首发**：VE 侧连消费形状都没有（票 05 取证），进了没人读，与 ADR-0098 「静默不发生」那一族同形；重启条件写在 Consequences。

**三、子行至少一项，不允许显式空版本。** 照 `0014` 先例（「空包等于无条件接受，因此零子行不是显式空约定」）。理由：一版客户服务规则存在的全部理由就是承载差异，「这一版对首发两项都无客户差异」不是一版规则，是不登记；允许显式空会造出「登记了但什么都没说」与「没登记」两种在 VE 那一侧同形的空。父行照 `0014` 形状：适用声明两列恰一非空（镜像 `CustomerServiceRuleApplicability`）、责任方、范围；子行按「项类 × 内容」，项类首发封闭两值，CHECK 钉死，后续四项进时放宽 CHECK 走新迁移。

**四、解析走既有闭包，不另开口；点读口新开一个。** `CustomerServiceRuleObject` 已在封闭集，`ResolveCommercialClosure` 对它一视同仁——版本壳层面今天就能被选中，零改动。选中之后按版本点读正文：新开 `CustomerServiceRuleContentView.LoadCustomerServiceRule(ctx, tenant, version)`，`found=false` 即正文未登记，坏数据与读失败走 error；与票 03 的 `CreditPolicyContentView` 同形，不进整册、不进 `ViewRevision`。消费方点读之后核「这一版挂的是不是我手上这份产品 / 合同」，这一核放在读口里做，不放在解析里（同 `ConsistentAcceptanceRulePackage` 那一道）。

**五、VE 采用时只冻版本引用（租户 + 对象 + 版本号），不冻正文。** 依据是本上下文正文册的纪律：同键异内容答`内容冲突`、绝不覆盖（ADR-0031），改规则必须发新版本，所以版本引用已唯一确定正文。VE 的期限记录按它自己的硬句保存「适用规则版本、起算事件、业务时区或日历、截止时间和适用范围」——其中规则版本是引用，其余是那一次判断**采用并派生出来**的事实，归 VE。版本壳上的 `ContentDigest` 可一并快照作交叉核对，但它是发布者声明的摘要不是系统算的，作核对不作唯一依据。VE 那一侧的列改不改归 VE 的票，本记录只答「存什么」。

**六、不做的，逐条写明。** 不迁移 VE 两册；不给 VE 建规则正文表；不在 PC 写任何期限天数、日历或材料清单的默认值；不把另四项塞进首发；不给客户服务规则开「编辑」口——改规则发新版本。

## Consequences

- 票 05 转 ready-for-agent，形状与票 03 同一条流水线：一份迁移（父子两表，照 `0014`）、`CustomerServiceRuleVersion` 补两项正文的领域类型与构造门（时长、日历引用、材料条目只校形状不校值）、`PublicationRegistry` 一个具名 Save、一个点读口、目录读面一格、CLI 批文一节；**不动 VE**。
- VE 侧另立一票（`visibility-exception` 地盘）：`ClaimEligibilityRules` 那两维从「恒答未登记」改为经消费侧适配器（ADR-0025）读 PC 点读口——读到则两维 `Registered` 为真并把版本引用冻进资格判断，读不到照旧未登记。**那一票落地前，VE 行为一字不变**。
- `PAR-VIS-08` 在参数登记册里的「时限与材料」半边，其登记面从此在 PC 的客户服务规则册；登记册行由其所有者按本记录改口径，本记录不代改。
- 另四项的重启条件：VE 侧出现读它们的消费形状（追踪披露 / 异常响应 / 客户更新进 VE 编排的规则依据，或通知策略要按客户差异化）时，放宽项类 CHECK 走新迁移，不必新 ADR；**若届时要把 VE 通知策略册并入 PC 规则版本**，那是所有权迁移，须新 ADR。
- `party-commercial` `CONTEXT.md` 不改：本记录每一条都在其原句之内。`visibility-exception` `CONTEXT.md` 不改。

## Alternatives considered

- **把 VE 两本册的正文迁到 PC 客户服务规则版本下。** 否决：两册的键（披露策略引用、客户合同）都不是客户服务规则的键，它们是 VE 自己业务结果的判断参数；迁移的代价是重做两本册与其编排、测试，收益为零——今天没有任何消费方要按客户服务规则读它们。
- **六项全进首发。** 否决：另四项 VE 侧无消费形状，登了没人读，与「静默不发生」同形；且四项的形状各自要一次与 VE 的对齐，一次做完等于把四次裁决压进一张实现票。
- **六项全作引用、数值留 VE。** 否决：索赔期限与最低材料的被引侧不存在，VE 注释明拒造它；「引用一个不存在的东西」比「正文放在所有者那里」多一层且什么都没解决。
- **允许显式空版本（照 `0012`）。** 否决：造出两种在消费侧同形的空；`0012` 允许显式空是因为客户合同正文有「本合同对此无约定、按产品」这一格真实语义，客户服务规则没有对应的回退物——产品没有默认期限。
- **VE 冻正文而不是冻引用。** 否决：正文册不覆盖（ADR-0031），引用已唯一确定正文；冻正文是第二份副本，两份之间的一致只能靠纪律。
- **另开一条客户服务规则专用解析口。** 否决：既有闭包对封闭集内类别一视同仁，零改动可用；另开一口就是第二套口径。

## Links

- 票 [party-commercial-context-gaps/05](../../.scratch/party-commercial-context-gaps/issues/05-customer-service-rule-version-has-no-consuming-seam-into-visibility-exception.md)：四问、前提纠正与「待 owner 裁的三处」的出处
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)：「客户服务规则版本」词条、Rules 两句、Boundaries 那一句——本记录 Decision 一的全部依据
- [visibility-exception CONTEXT](../domain/visibility-exception/CONTEXT.md)：索赔一节「三个独立期限」与「每个期限必须保存适用规则版本、起算事件、业务时区或日历、截止时间和适用范围」——Decision 二的期限形状与 Decision 五的引用/派生分界
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧适配器——VE 读 PC 点读口的形状
- [ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：登记册写入结果是封闭代数、同键异内容答内容冲突而不覆盖——Decision 五冻引用即够的依据
- [ADR-0093](./0093-channel-account-use-authorization-is-not-a-commercial-version.md)：归族判据——客户服务规则版本是版本演进，进封闭集是对的（票 04 第一笔）
- [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md)：「静默不发生」那一族——另四项不进首发的判据
- `internal/partycommercial/domain/customer_service_rule.go`：`CustomerServiceRuleVersion` 与 `CustomerServiceRuleApplicability`——本记录在其上加两项正文
- `internal/visibilityexception/adapters/postgres/claim_eligibility.go`、`notification_policy.go`：VE 两维「连登记面都没有」与两本册的键——Context 的取证点
- `migrations/party_commercial/0014_acceptance_rule_package.sql`：父子两表与「至少一条」先例
- 来源：IDP 队列通道 3 的授权（2026-09-03）
