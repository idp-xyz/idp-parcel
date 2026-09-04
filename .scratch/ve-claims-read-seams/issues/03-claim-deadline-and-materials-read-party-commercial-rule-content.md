# 索赔期限与最低材料两维改经消费侧适配器读 `party-commercial` 客户服务规则正文——今天恒答未登记

Category: enhancement
Status: draft
Blocked by: [party-commercial-context-gaps/05](../../party-commercial-context-gaps/issues/05-customer-service-rule-version-has-no-consuming-seam-into-visibility-exception.md)（正文册、点读口与批文口，分支 `mcp4-pcgaps05`，待重放进 main）

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
