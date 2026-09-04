# 索赔期限与最低材料两维改经消费侧适配器读 `party-commercial` 客户服务规则正文——今天恒答未登记

Category: enhancement
Status: resolved（2026-09-04 MCP-4，分支 `mcp4-ve03`，已验 tip `91967a6b`，基线 main `9e4e90bb`；见 Comments 完成记录）
Blocked by: [party-commercial-context-gaps/05](../../party-commercial-context-gaps/issues/05-customer-service-rule-version-has-no-consuming-seam-into-visibility-exception.md)（已入 main：九笔重放，`SaveCustomerServiceRule` / `CustomerServiceRuleContentView` 在 `05992ce2`，票面转 resolved 在 `469bb9ba`；本票开工时阻塞已解除）

由 [ADR-0104](../../../docs/adr/0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)
Consequences「VE 侧另立一票」与票 pc-gaps/05「另立而不在本票」一句立票。本票是 VE 地盘，
pc-gaps/05 落地前 **VE 行为一字不变**（ADR-0104 Consequences 原句）。

## 代码事实（钉在 pc-gaps/05 分支 `9210b745`，VE 侧与 main 同）

- `internal/visibilityexception/adapters/postgres/claim_eligibility.go` 的 `claimRulesForTenant`
  交回 `ports.EligibilityRules` 时，`FilingDeadline` 与 `Materials` 两格填的是零值——
  `FilingDeadlineRule{}` / `MinimumMaterialsRule{}`，`Registered` 恒为假。头注写明这是有意的：
  「索赔时限要起算事件与业务日历、最低材料要一份清单，两样仍属 `PAR-VIS-08` 待登记实例参数且
  没有登记面。**凑一份就是发明实例参数**，所以那两维一律如实答未登记，由编排停在指名到维的未决」。
- 编排据此停在指名到维的格：装配测试钉过 `ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED`
  （见[本目录票 01](./01-eligibility-rule-view-needs-a-tenant-dimensioned-read.md) Comments），
  那是本票落地前唯一走得到的真实分支。
- VE 另两维（合同责任范围承不承担某索赔类型 0011、申请人授权目录 0018）**不在本票范围**：
  ADR-0104 Decision 一裁它们是 VE 自己的判断参数，不迁移、不改键。
- 提供侧从 pc-gaps/05 起有了：
  - `pcports.CustomerServiceRuleContentView.LoadCustomerServiceRule(ctx, tenant, version)`，
    交回 `pcdomain.CustomerServiceRuleVersion`——`ClaimDeadline(kind)` 三种期限各至多一行
    （起算事件引用 × 整数天 × 日历引用），`MinimumMaterialsFor(claimKind)` 一份材料条目引用清单；
    `found=false` 即正文未登记，坏数据（含壳与正文所挂对象分歧）走 error。
  - 解析走既有闭包：`CustomerServiceRuleObject` 在封闭集内，`ResolveCommercialClosure` 一视同仁
    （ADR-0104 Decision 四，解析不改）。VE 要自己带 `RequiredBases` 含该类别去解析，再按选中的
    版本壳点读。
  - 起算事件与材料条目在 PC 是**引用**不是封闭集：`DeadlineStartEventReference`、
    `MaterialRequirementReference`、`ClaimKindReference` 的解释权都在 VE（PC 类型注释明写）。

## 要做什么（形状，不在本票之外裁新决定）

按 ADR-0025 在 `internal/visibilityexception/adapters/partycommercial/` 建消费侧适配器，让
`EligibilityRuleView` 的两维从 PC 读：

1. **解析**：按（客户合同版本 / 服务产品版本 + 业务时点）经 PC 既有闭包选中客户服务规则版本壳。
   `EligibilityQuery` 今天带的是 `Contract domain.ContractScopeReference`（VE 自己的合同范围引用），
   与 PC 解析键的对应关系要在本票摆清——这是 VE 词到 PC 键的翻译，归消费侧适配器。
2. **点读**：按选中的版本壳调 `LoadCustomerServiceRule`；`found=false` 照旧答两维未登记（恢复动作
   不变：去 PC 登记正文）；error 按端口合同「依赖调不通作为错误返回」。
3. **翻译到 VE 的规则形状**：
   - `FilingDeadlineRule`：`Registered=true`、`RuleVersion` 冻版本引用（租户 + 对象 + 版本号，
     ADR-0104 Decision 五，不冻正文）、`StartEvent` / `Calendar` 照 PC 引用转写、`Scope` 取索赔自己
     固定的目标范围。**`Deadline`（截止时刻）不是 PC 交的**——它是 VE 拿起算事实的实际时刻 + 天数 +
     日历派生出来的事实（PC 类型注释「截止时刻本身不在这里算，那是 VE 拿起算事实与这条规则派生
     出来的事实，归 VE」）。派生要一个起算事实源与一份按引用取日历的能力，两样今天 VE 都没有，
     见「未决」。
   - `MinimumMaterialsRule`：`Registered=true`、`RuleVersion` 同上、`Required` 照 PC 材料条目引用
     转写；`SupplementDeadline` 可由 PC 的 `MaterialSupplementDeadline` 那一行派生（同样要起算事实
     与日历）；**`Notice`（补充通知依据）PC 首发没有来源**——通知义务是 ADR-0104 不进首发的四项之一，
     见「未决」。
4. **装配**：`cmd/parcel-api/assemble_claims.go` 那一格把消费侧适配器接进资格缝；受控登记口
   （`parcel-ve-register`）不接——它不该拿到跨上下文读。

## 未决（本票开工前要有人答，或在本票里显式留格）

- **起算事实源**：三种期限的起算事件（交付、资料索取、结论通知/送达/可获取）在 VE 哪些事实上落？
  今天 `EligibilityQuery` 不带任何时刻。没有它就算不出 `Deadline`，`Registered=true` 而
  `Deadline` 零值是一个残缺的第三态，端口注释明禁（「四件凑不齐就停在未决，不记一个残缺的第三态」）。
- **业务日历**：PC 只带 `BusinessCalendarReference`，日历内容（工作日、节假日、时区）属实例半边
  且今天无任何上下文持有。按引用算日的能力放哪、由谁登记，要裁。
- **`Notice` 的来源**：通知义务不在 PC 首发（ADR-0104 Decision 二）。资料不足那条路四件落点里的
  「通知依据」因此仍缺一件——是等 PC 放宽项类（ADR-0104 Consequences 的重启条件）还是 VE 通知策略册
  （`notification_policy.go`，键是披露策略引用）自己答，要裁；后者是 VE 自己的判断参数，不违 ADR-0104。
- **`Registered` 的粒度**：PC 一版规则可以只登期限不登材料（两项合起来至少一项）。两维各自按
  「PC 那一项有没有行」答 `Registered`，还是版本在场即两维都算登记？前者与今天「各维自己交代」的
  纪律一致，本票倾向前者，但要在票面钉死。

## 裁决（2026-09-04 MCP-4，细则受托代裁；用户可 supersede）

开工前按 `/grill-with-docs` 对 VE CONTEXT 索赔一节、ADR-0104 Decision 二/四/五、PC
`customer_service_rule.go` 类型注释与 VE `handle_claim.go` 逐维核对过一遍。判据只有一条：能在不动
不变式、不造默认值的前提下答的就答；答不了的在实施里**显式留格**，不拿零值冒充第三态。

- **`Registered` 的粒度——自裁：各维按「PC 那一项有没有行」答。** `FilingDeadline.Registered` 看
  这一版有没有 `FIRST_CLAIM` 那一行（PC `ClaimDeadline(kind)` found=false 即「这一版对该种期限无
  客户差异，不是零值期限」）；`Materials.Registered` 看这一版对**本索赔类型**有没有材料清单行
  （`MinimumMaterialsFor(claimKind)`）。版本壳在场不等于两维都登记：ADR-0104 Decision 三允许一版只
  登其中一项，而 VE 端口注释要求「某一维规则尚未登记，由各维自己的 Registered 如实交代」——两处
  合起来只剩这一种读法。恢复动作因此指得准：缺哪一项去 PC 补哪一项，不是重登整版。
- **起算事实源——留格。** PC 交的是不透明的 `DeadlineStartEventReference`，解释权在 VE；要把它落到
  VE 的某个已接受事实（交付、资料索取、结论通知/送达/可获取）上，需要一份「起算事件引用 → VE 事实
  种类」的对应，那是实例半边且今天无任何上下文持有；`EligibilityQuery` 也不带任何时刻。适配器因此
  只填规则半边（版本引用、起算事件引用、日历引用、范围），`Deadline` 留零值。编排侧
  `judgeFilingDeadline` 既有守卫把「`Registered` 为真而 `Deadline` 为零」判成核不了、停在指名到维
  的未决、索赔项一字不动——第三态没有被记下，但那一格的名字 `ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED`
  对这一状态已经不准（规则登了，缺的是起算事实），改名归 `application`，不在本票地盘，见完成记录。
- **业务日历——留格。** PC 只带 `BusinessCalendarReference`；按引用算日的能力与日历内容（工作日、
  节假日、时区）属实例半边，今天没有持有者。适配器照引用转写 `Calendar`，不做任何日期运算。
- **`Notice` 的来源——留格，报 owner 裁。** 通知义务不在 PC 首发（ADR-0104 Decision 二）；VE 通知
  策略册的键是披露策略引用，索赔项手上没有那个引用，从它派生要先有一份新的对应（实例半边）。两条
  出路（等 PC 放宽项类 / VE 自建索赔通知依据册）都不是本票能拍的，`Notice` 留零值。
- **`SupplementDeadline`——留格，与起算事实源同因。** 它由 PC 的 `MATERIAL_SUPPLEMENT` 那一行 +
  资料索取的实际时刻 + 日历派生，前一件 PC 有、后两件今天没有。留零值的编排后果要写明：材料维
  `Registered` 为真且差材料时，`applyScreen` 会先撞 `!SupplementDeadline.After(now)` 那一格、停在
  `ELIGIBILITY_SUPPLEMENT_WINDOW_CLOSED`——同样是停在未决、不记第三态、不拒赔，但名字说的是「窗口
  已关」而实情是「截止算不出」。同归 `application` 的改名票。
- **VE 词到 PC 键的翻译——立缝，不代拟。** PC 闭包键要租户、客户账户、责任法人候选、商业范围、
  目的、锚点（时刻 + 锚点策略版本）与必需依据；`EligibilityQuery` 只有前两项。缺的三项与锚点全是
  实例半边（判据同 PS 的 `ResolutionKeySource`：「没有租户时谁也说不出这份委托该在哪个商业范围下
  解析」）。适配器包内立 `RuleResolutionKeySource` 接口（ADR-0025：实例半边协作者的接口留在适配器
  包内，不进 `ports`），nil 或 formed=false 即显式未配置，两维答未登记——端口没有逐维的未决格，
  两个可用的答案里「未登记」的恢复方向（去登记）对得上，「报错」的（重试依赖）对不上。必需依据
  由登记方给，但必须含 `CustomerServiceRuleObject`，缺了是配置缺陷走 error 不折成未登记；要不要一并
  要 `CustomerContractObject` 让闭包核指名引用，归登记方。生产装配今天 Keys 留 nil（没有任何租户
  登记过这份映射，也还没有登记面），装配测试经同一装配函数注入一份键来源钉「已登记」态。登记面
  另立票。
- **`RuleVersion` 冻什么——按 ADR-0104 Decision 五。** 三段：租户 / 对象 / 版本号，`/` 连接；
  不冻正文，`ContentDigest` 也不冻（ADR 说「可」不说「须」，VE 端口没有那一格）。
- **`Scope`——取索赔自己固定的目标范围**（`EligibilityQuery.Target`），票面「要做什么」第 3 条原句。
- **不核 `query.Contract` 与规则适用声明的合同是不是同一个标识。** VE 的合同责任范围引用与 PC 的
  客户合同对象标识是不是同一个标识空间，今天没有任何文档立过；替它们假定相等就是在适配器里判断。
  壳与正文的一致性由 PC 读口核（ADR-0104 Decision 四），规则与合同的对应由闭包的指名引用核（登记方
  把合同列进必需依据时）。
- **闭包各结局的落点（全函数，不留兜底）：** 唯一解析 → 点读；`无适用依据` → 两维未登记（权威说
  这个范围里没有生效的规则版本，去发布一版）；`适用冲突` / `解析未决` / `输入未受理` → error（前者要
  商业责任方修重叠，中者要重试或等实例参数，后者是键配错——三种都不是「去登记」，而 VE 端口只有
  未登记与 error 两个格，error 至少让人来看）；`已失效` / `依据未解析` 第一阶段不产，出现即哨兵错误。

## 边界

- 不动 `party-commercial`：点读口与解析今天已够用；要 PC 多交什么（如通知义务）走 ADR-0104 的重启条件，
  不在本票顺手加。
- 不迁 VE 两本册（0011 / 0018）、不改键（ADR-0104 Decision 一）。
- 不在 VE 造任何默认期限、默认日历或默认材料——读不到就是未登记。

## 完成标准

- 已登记客户服务规则正文的租户：两维 `Registered` 为真，`RuleVersion` 是 PC 版本引用，`Required` 与
  PC 材料条目逐项相等；未登记的租户两维照旧未登记，编排停的格不变。
- 装配测试对真库钉两态（未登记 / 已登记），VE 全包与 `cmd/parcel-api` 真库套件绿。
- `claim_eligibility.go` 头注里「两样连登记面都还没有」那一句随之改口——登记面从此在 PC 的客户服务
  规则册（ADR-0104 Consequences 对 `PAR-VIS-08` 时限与材料半边的改口）。

## Comments

- 2026-09-04 MCP-4：立票（draft）。前置 pc-gaps/05 的六件已在分支 `mcp4-pcgaps05` 上：迁移 0023、
  `SaveCustomerServiceRule`、`CustomerServiceRuleContentView`、发布通道、批文一节、目录读面一格；
  待 MCP-1 重放进 main 后本票的 Blocked by 才算解除。「未决」四条里前两条（起算事实源、业务日历）
  决定本票能不能一次做完——建议开工前先经 `/grill-with-docs` 对 VE CONTEXT 索赔一节过一遍。
- 2026-09-04 MCP-4：**resolved**。分支 `mcp4-ve03`（隔离 worktree，先基 `mcp4-pcgaps05@34d8ad90`，05 入
  main 后两次 rebase，最终基线 main `9e4e90bb`），**已验 tip `91967a6b`**，不推、交 MCP-1 重放。六笔：
  - `4baadba8` 票面转 in-progress，四条未决过 `/grill-with-docs` 写进「裁决」节；
  - `555461fc` 新包 `internal/visibilityexception/adapters/partycommercial`：`ClaimServiceRules`（装饰
    `EligibilityRuleView`）、`RuleResolutionKeySource`（实例半边协作者接口，留包内）、两个哨兵
    `ErrUntranslatableAnswer` / `ErrCustomerServiceRuleUnresolved`；测试对着真 PC 编排解闭包，只替身
    权威读口、解析库、正文读口与 VE 自己的册（七个用例，含全函数分派与构造期守卫）；
  - `99200610` `claim_eligibility.go` 头注改口（完成标准第三条）与两维交候处注释，只动注释；
  - `8d9cc4ee` `cmd/parcel-api/assemble_claims.go`：`buildClaimEligibilityRules` 把适配器叠在多租户读
    适配器上，PC 闭包编排 + 解析库 + 点读口接真，Keys 传 nil；`buildClaimsOrchestration` 经
    `buildClaimsOrchestrationWith(db, nil)`；装配测试 `TestTheWiredClaimsReadCustomerServiceRulesFromPartyCommercial`
    对真库钉两态 + 生产装配不变那一格；
  - `e44a2e0b` 立票 04（解析键登记面，draft）与 05（编排改名两格，ready-for-agent）；
  - `fd49465f` 装配测试的事务回调只做 IO（`TestNoTransactionClosureCarriesAGoexitAssertion` 点名的三处）；
  - `91967a6b` 机制清点在 `fd49465f` 干净树上重生成（VE 生产/测试各 +1、跨上下文消费缝 +1 组）。
  **四条未决各自的去向**：`Registered` 粒度——自裁（各维按 PC 那一项有没有行），已落代码与用例；
  起算事实源、业务日历——留格（`Deadline` 零值，编排既有守卫停在未决），能力归谁未裁，报 MCP-1；
  `Notice` 来源——留格（零值），PC 放宽 vs VE 自建索赔通知依据册要 owner 裁，报 MCP-1；顺带发现的
  `SupplementDeadline`——留格，与起算事实源同因。留格的编排后果（两格未决命名不准）已立票 05。
  **VE 词到 PC 键的翻译**立缝不代拟，生产装配 Keys=nil、行为与本票之前一字不变；登记面立票 04。
  **验证（`91967a6b` 干净树）**：`gofmt -l` 空；`go build` / `go vet` 退 0；无 DSN `go test -count=1 ./...`
  96 包 ok / 0 FAIL；带 DSN `-p 1 -count=1 -v` VE + parcel-api + architecture + migrations
  **1020 PASS / 0 SKIP / 0 FAIL**（17 包 ok），探针 `TestTheWiredClaimsReadCustomerServiceRulesFromPartyCommercial`
  带 DSN PASS、无 DSN SKIP；`tools/mechanism-inventory` vet/test 退 0。测试输入是隔离合成，只记 `S`。
  **完成标准逐条**：已登记正文的租户两维 `Registered` 为真、`RuleVersion` = `SYN-TENANT-1/SYN-CSR-1/v1`、
  `Required` 与 PC 条目逐项相等（装配测试钉）；未登记照旧未登记、编排停的格不变（同一测试态一 +
  生产装配那一格）；两态对真库钉住、VE 全包与 `cmd/parcel-api` 真库套件绿；头注改口已落。**无新端点、
  无迁移、无基线改动**，MCP-1 无需落装配行。
