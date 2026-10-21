# 文档自洽审计 · PC（party-commercial）+ PP（parcel-pricing）

- 取证树：只读 worktree `/home/tops/workspace/idp-parcel-audit`，detached @ `6b0074d0`；在途分支未读。
- 口径：`.scratch/docs-consistency/README.md`「审计口径」；引用一律用文件 + 引文 / 代码符号，不用行号。
- 范围：PC、PP 两份 `CONTEXT.md`；`UC-PC-001..003`、`UC-PP-001`；PC/PP 相关 ADR（重点 0014、0016/0146、0032、0034、0038、0044、0056、0057、0104、0107、0110、0116、0126、0127、0151、0176–0179、0202、0204、0209、0229、0233、0234）；`CONTEXT-MAP.md`、`GLOSSARY.md` 的 PC、PP 条目；`internal/partycommercial/**`、`internal/parcelpricing/**` 与 `cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-pricing-register` 的装配。
- 共 **24** 条：高 5、中 8、低 11。

---

## 高

### H1 供应商协议版本收尾之后，在途运输委托的后续发生项被挡；CONTEXT 仍写「既有委托继续引用原版本」

- **类别**：1（CONTEXT 现行规则）
- **位置**：
  - `docs/domain/party-commercial/CONTEXT.md`「商业规则与政策版本」生命周期：「已生效 → 已到期、已退役或已替代：停止用于新的解析和判断，既有委托、交易和结算继续引用原版本。」
  - 同文件「供应商商业协议版本」生命周期：「协议版本到期、终止或被替代后，不改变已经形成的运输委托、履约事实、供应商账单主张或审核应付依据。」
  - 同文件 Rules（收尾那一条）：「收尾之后，按范围解析与按版本点读、用于新决定的口（供应商协议、内部成本政策）同一口径；为既有引用点读的口（按回指取已固定的解析、按已固定的快照取正文）不折收尾」——这句没有说清一份**已提交**运输委托之后的发生项登记算「新决定」还是「既有引用」。
- **证据**：
  - `docs/adr/0233-…md` 越权风险点 1 自认这是矛盾：「(c) 这与 PC CONTEXT「已退役……既有委托、交易和结算继续引用原版本」相抵：委托快照照旧引用旧版，而按旧版判的新登记被收尾挡住」；同记录「对已接受记录的作用」又写「不停用任何条款」。
  - `docs/adr/0204-…md` 背景里「服务方、协议快照与采购责任法人的权威仍是委托那一行」（ADR-0233 把它记作 ADR-0204 决定一「委托快照是采购的权威」）。
  - 代码：`transportfulfillment/application.registerChargeOccurrence` 在 PC 点读折进收尾之后答 `ChargeOccurrenceAgreementNotEffective` / `ChargeOccurrenceAgreementTerminated`；`form_actual_fulfillment_on_participation_end.go` 的 `registrationDisposition` 把这两格归 `FormationSettled`，也就是入账、不重投、不留痕，BUY 请求就此缺席。
  - ADR-0233 的「运营约束：票 88 落地前，不得截止或退役仍被在途运输委托指名的供应商协议版本」只写在 ADR 的 Status 行和越权风险点里，PC CONTEXT 与 `UC-PC-001` 的 `AT-PC-058` 都没有提。
- **归类**：**B 要裁**。PC CONTEXT 两条生命周期句、ADR-0204 与 ADR-0233 决定五都是已接受记录，互相相抵，谁也没停用谁；代码只照 ADR-0233 一头。承接口是在途票 88（README 依赖列已列）。
- **严重度**：高。照 CONTEXT 写 TF 或 SA 的人会以为在途委托不受收尾影响；实际上这些委托此后的成本会静默不形成。
- **建议修法**：立代决 ADR（或并入票 88）定一句：已提交委托上的后续发生项登记属于「为既有引用点读」，不折收尾；或者明写它属于新决定，并把「收尾前须确认无在途委托」写进 PC CONTEXT 的收尾规则与 `AT-PC-058`。之后改 PC CONTEXT 那两条生命周期句，并在 ADR-0233 Status 行挂停用指针。

### H2 授权规则或客户合同的版本收尾之后，授权授予与合同委派照旧生效；CONTEXT 说「停止用于新的判断」

- **类别**：1
- **位置**：`docs/domain/party-commercial/CONTEXT.md`「商业规则与政策版本」生命周期：「已生效 → 已到期、已退役或已替代：停止用于新的解析和判断……」「本生命周期共同适用于接单规则包、接受前财务控制策略、商业价格政策、定价方案绑定、结算政策、信用政策和授权规则」。
- **证据**：
  - `docs/adr/0233-…md` 决定六（不在本记录）：「合同委派与授权授予随合同、授权规则版本收尾：两者按自己的区间判，今天不看版本壳的生命周期」；越权风险点 6：「同起点退役一版合同之后，挂在旧版上的委派照它自己的区间适用」。
  - 代码：收尾口对任何 `CommercialObjectKind` 都收，`AuthorizationRuleObject` 也在内（`registrationjson/version_closure.go` 经 `CommercialKindFrom` 取种类）。但 `postgres.AuthorityGrants.LoadEffectiveGrants` 只按授予自身区间取行（`effective_starts_at <= $3 AND (effective_ends_at IS NULL OR effective_ends_at > $3)`），不连退役册，也不连区间更正册；`postgres.ContractDelegations.LoadEffectiveDelegations` 同样只看委派自己的区间。
  - 后果更重的一格：`LoadEffectiveGrants` 按 `ORDER BY object_id, version_label` 排序，`domain.Authorize` 取首个命中。同起点重发 v2、退役 v1 之后，命中的是已退役的 v1 授予，消费方保存的「所采用的授权规则版本」也就是一版已退役的规则。这一格又落在 `UC-PC-003` 未决五上（见 M4）。
- **归类**：**C 代码错**。CONTEXT 是现行规则，ADR-0233 只记了「今天不看」、没有立相反的规则。修代码之前须先定截止对授予与委派怎么作用；owner 若认定授予应按自身区间走，就改归 **B**，CONTEXT 照改。
- **严重度**：高。运营退役一版授权规则以收回某个等级的拒绝、关闭或重开权，这些权限实际上照旧生效。
- **建议修法**：立代码票：授予与委派的装载连上版本壳的退役与最新区间更正，或者收尾口拒收 `AuthorizationRuleObject` 与带委派的合同版本，直到这件事接上。在此之前，PC CONTEXT 生命周期里给授权规则与合同委派写明现状。

### H3 PC 服务产品版本生命周期仍写「自退役生效时间起」；ADR-0233 的退役对任何锚点都不再可选

- **类别**：1
- **位置**：
  - `docs/domain/party-commercial/CONTEXT.md`「服务产品版本」生命周期：「已发布 → 已退役：自退役生效时间起不再供新的商业选择，但已经接受的委托继续引用原版本。」
  - 同文件 Rules（收尾那一条）只写「同起点重发的，旧版按决定引用退役」，没有写退役时刻取登记时刻，也没有写退役之后任何锚点都不再选中。
- **证据**：
  - `docs/adr/0233-…md` 决定三：「旧版退役……自此不再参与任何新的选择」「退役时刻取登记时刻，不由登记方给」；越权风险点 2：「CONTEXT『自退役生效时间起不再供新的商业选择』本记录读作『退役登记之后作出的新决定』，业务时点早于退役时刻的新决定同样不再可选」。
  - `UC-PC-001` `AT-PC-055`：「此后任何锚点只选新版」。
  - 代码：`domain.CommercialVersion.AppliesAt` 只认 `CommercialVersionEffective` 加区间包含，退役之后对任何时点都答 false。
- **归类**：**A 文档错**。代码与已接受的 ADR-0233 一致，CONTEXT 那句的文义是按锚点切分。
- **严重度**：高。读者会以为锚点早于退役时刻的委托仍能选中旧版，照此写的解析或测试会与代码相反。
- **建议修法**：把「自退役生效时间起」改成「自退役登记之后，任何锚点的新选择都不再选中它」，并在收尾规则里补一句「退役时刻取登记时刻」，引 ADR-0233 决定三。

### H4 选卡的权威有两套：PC 价格政策的定价方案绑定与 PP 按范围选卡并存于现行文档，生产只走后者

- **类别**：1（兼 4、6）
- **位置**：
  - `docs/domain/party-commercial/CONTEXT.md` Rules：「商业价格政策版本必须显式绑定价格方向和可执行定价方案版本」；「已解析定价依据」词条：「它是 `parcel-pricing` 和 `settlement-accounting` 的输入」。
  - `docs/domain/CONTEXT-MAP.md`「`party-commercial → parcel-pricing`：……商业价格政策、价格方向授权、定价方案绑定、计价基准时点……；计价校验引用闭包并形成可执行评价」。
  - `docs/domain/GLOSSARY.md`「可执行价卡版本」：「依据已解析商业价格政策和定价方案绑定形成」；「价格规则版本」：「`parcel-pricing` 依据该政策形成可执行价卡和纯评价」。
  - `docs/domain/parcel-pricing/CONTEXT.md`「计价输入快照」把「商业依据」列为快照成员。
- **证据**：
  - `docs/adr/0179-…md` Context：「正式 SELL·客户费用评价按（租户、计价范围、方向、目的、时点）经 `PriceCardInForceResolver` 解析在用价卡，不走 PC 价格政策的定价方案绑定（ADR-0034）」；决定三：「正式路径将来若改走商业价格政策的绑定，两处同一笔改」。
  - ADR-0234 的收尾只作用于 PP 按范围选卡（`ResolveInForce`、`LoadApplicable`），不回报 PC。PC 的 `PricingPlanStandingLookup` 在生产里没有任何提供方：`application.ResolveCommercialBasisHandler.Handle` 向 `domain.ResolveCommercialClosure` 传 `nil`；PC 的 `PricingPurpose` 在 PC 领域包以外无人使用。`docs/adr/0032-…md` Consequences 也写着：「`AT-PC-036` 的真适配器不存在」。
  - `parcelpricing/domain.PricingInputSnapshot` 没有商业依据引用这一格，只有 `settlement` 币种。
  - 结果：一份价格政策绑定的方案版本被 PP 截止或退役之后，PC 那份绑定照旧指着它，也没有任何一方会读这份绑定。
- **归类**：**B 要裁**。PC CONTEXT、CONTEXT-MAP、GLOSSARY（含 ADR-0034 的闭包形状）与 ADR-0179 及代码各执一词，也没有停用关系。
- **严重度**：高。新消费方（如线路成本、结算）的作者照文档会去解析 PC 价格闭包选卡；租户照 PC CONTEXT 登绑定，以为它决定计价用哪张卡。
- **建议修法**：立代决 ADR 定首发选卡的唯一权威（按范围选卡，PC 绑定只作商业授权留痕；或反过来）。随后改 PC CONTEXT「已解析定价依据」、绑定规则、CONTEXT-MAP 那一条边、GLOSSARY 两条，以及 PP「计价输入快照」里的「商业依据」，并给 ADR-0034 挂指针。

### H5 SELL 方向的体积系数与金额取整：文档归商业价格政策，代码与演示种子写在价卡上

- **类别**：4
- **位置**：
  - `docs/domain/parcel-pricing/CONTEXT.md`「体积系数」：「采购方向的系数属价卡内容，销售方向的系数属商业价格政策」；「金额取整策略」：「采购方向的策略属价卡内容，销售方向的属商业价格政策」；Rules：「系数是价卡或商业价格政策的内容」。
  - `docs/domain/party-commercial/CONTEXT.md`「商业价格政策版本」：「……以及销售方向的体积系数……`parcel-pricing` 按声明的口径解析取值」。
  - `docs/adr/0107-…md` 决定一：「采购方向的策略属价卡内容，销售方向的属商业价格政策」。
- **证据**：
  - PC 代码 `domain.NewVolumetricCaliber`：SELL 必须声明系数引用，BUY / INTERNAL 不得声明。它的注释写明危害：「采购方向再声明一份，同一个包裹就有两个系数各自都合法，而计价选中哪一个取决于取数顺序」。
  - PP 代码：`domain.PricingPlanVersion` 不分方向，一律携带卡自己的 `VolumetricFactor` 与金额取整；`deriveVolumetricWeight` 只读 `policy.VolumetricFactor()`；`internal/parcelpricing` 里没有任何地方读 PC 价格政策口径。
  - 演示种子 `scripts/demo-seeds/data/pricing/price-card-cn-sg.json`（`direction: SELL`）自带 `weightPolicy.volumetricFactor` 除数 5000 与 `amountRounding` `HALF_UP`。
  - PC 价格政策口径 `domain.PricePolicyCaliber` 只有 tax、volumetric、fx 三格，没有金额取整一格。PC CONTEXT 也没有把金额取整列为价格政策口径，与 PP CONTEXT、ADR-0107 不对应。
- **归类**：**C 代码错**。CONTEXT 两份与 ADR-0107 一致，PP 代码与它相反。若 owner 认定 SELL 卡自带系数与取整就是首发形态，改归 **B**：立 ADR 停用 ADR-0107 决定一与两份 CONTEXT 的相应句，并删去 PC 的 SELL 系数口径。
- **严重度**：高。SELL 的系数或取整会有两处各自合法的声明，计价只用卡上那一处，合同政策上声明的那一个静默失效。
- **建议修法**：立代码票，让 PP 的 SELL 评价从商业价格政策口径取系数与取整，并在 SELL 卡上拒收这两格；或者走上面的 B 路。不论哪条路，PC CONTEXT 都补上金额取整一格的归属。

---

## 中

### M1 `AT-PC-036` 一行写了两种结果，其中「无适用依据」被 ADR 明禁

- **类别**：2
- **位置**：`docs/application/party-commercial/UC-PC-002-…md` `AT-PC-036`：「合同和价格政策唯一，但绑定价卡版本已退役且无替代 | 返回解析未决或无适用依据，不选择最新价卡兜底」。
- **证据**：`docs/adr/0032-…md` 决定五：「不可用按恢复动作分两格，且都不是`无适用依据`」；`docs/adr/0034-…md` 决定三：「已退役 → 原因 `BoundPlanWithdrawn`。两者都**不是** `无适用依据`」。代码 `domain.resolvePriceRuleBasis` 遇 `ErrPricingPlanWithdrawn` 答 `pending(…, BoundPlanWithdrawn, …)`。
- **归类**：A
- **严重度**：中
- **建议修法**：改成「返回解析未决（原因：绑定方案已退役），不选择最新价卡兜底」。

### M2 `UC-PC-002` 把「锚点身份无法建立」归`输入未受理`，同一用例的 AT 与代码答`解析未决`

- **类别**：2
- **位置**：`UC-PC-002` 解析结果语义表：「输入未受理 | 最小租户、客户、范围或锚点身份无法建立 | 不查询或泄露候选商业对象」。
- **证据**：同文件 `AT-PC-018`：「商业选择锚点策略未配置 | 解析未决，不选系统当前时间默认」。代码 `domain.ClosureResolutionKey.minimumIdentityEstablished` 不含锚点；锚点无效时 `ResolveCommercialClosure` 答 `closurePending(…, AnchorPolicyNotConfigured, …)`，注释写着「锚点策略缺失是`解析未决`而非失败」。
- **归类**：A
- **严重度**：中。照语义表写消费方的人，会把锚点未配置当作改输入，而不是等租户登记。
- **建议修法**：「输入未受理」一行删去「或锚点」，另加一句「锚点策略未配置是`解析未决`」。

### M3 `UC-PC-001` 发布的结果语义与流程，和 ADR-0233 及代码不符

- **类别**：2（兼 4）
- **位置**：`docs/application/party-commercial/UC-PC-001-…md`
  - 应用流程第 4 步：「会产生歧义的版本不得激活」。
  - 结果语义表把「适用冲突 | 同一解析键与时点会出现多个必需候选」列为发布结果；`AT-PC-006`：「两个合同版本在同一解析键和期间重叠 | 形成适用冲突」。
  - 结果语义表没有`未受理`，`AT-PC-052`、`AT-PC-056`、`AT-PC-057` 却都答未受理；`AT-PC-033` 答「发布冲突」，这个词也不在表里。
- **证据**：
  - `docs/adr/0233-…md` Context：「PC 只增：同一对象另发一版，旧版留在册上照旧`已生效`。按范围解析的册……在两版区间重叠的锚点上答`适用冲突`」；决定一：「新版发布本身不收尾旧版」。同用例 `AT-PC-054` 也写「同一对象新版已生效、晚于旧版起点，区间与旧版重叠（同范围答`适用冲突`）」。
  - 代码：`application.PublishCommercialAuthorityHandler` 不做区间重叠判断，结果集是 `PUBLISHED_EFFECTIVE`、`PLANNED_EFFECTIVE`、`REPLAYED`、`CONTENT_CONFLICT`、`PENDING`、`NOT_ACCEPTED`。跨向绑定 `ErrPriceDirectionBindingConflict` 经 `declarationWrites` 出错，落 `CommercialPublicationNotAccepted`。
- **归类**：A
- **严重度**：中。作者可能照第 4 步去造一道发布时的重叠门，那与 ADR-0233「换版时两版并存、显式收尾」相反。
- **建议修法**：第 4 步改成「重叠不阻发布，由解析答`适用冲突`，经收尾消解」；把「适用冲突」从发布结果表移到 `UC-PC-002`；表里补`未受理`；`AT-PC-033` 改答`未受理`；把`来源冲突`与代码 `CONTENT_CONFLICT`、PC CONTEXT `内容冲突` 对齐成一个词。

### M4 `UC-PC-003` 对代码现状的描述整体过时

- **类别**：4
- **位置**：`docs/application/party-commercial/UC-PC-003-…md`
  - 首切范围：「`AuthorizedAction` 今天是封闭二值（人工复核、主动拒绝）」。
  - 未决二：「今天 `Authorization` 交回动作、所命中的授权规则版本、原因、证据与时点，不交回决定方」。
  - 未决三：「撤回与资料修订都不在 `AuthorizedAction` 内」。
  - 未决五：「命中顺序即装载顺序、装载顺序即数据库顺序」。
  - 给开发的交接：「领域件已在位：……`Authorize(grants, request)`……缺的是能装载 grants 的端口、调用 `Authorize` 的应用编排，以及授权治理册的持久化面（`migrations/party_commercial/` 从 `0003` 起号）」；「`PublicationRegistry` 的注释自己写着「另票补……」，照办」。
- **证据**：
  - 动作集与决定方：`domain.AuthorizedAction` 现有 `MANUAL_REVIEW`、`ACTIVE_REJECTION`、`SOURCE_DATA_AMENDMENT`、`CONTROLLED_CLOSURE`、`REOPENING` 五格（迁移 `0025`、`0032`）。`domain.Authorization.Decider()` 已在。`docs/adr/0116-…md` 决定一、三已答资料修订那一支的未决二、三。
  - 签名：`domain.Authorize(grants, delegations, request)`，同一用例的裁决段已经写成三参。
  - 端口、编排与持久化：`ports.AuthorityGrantStore.LoadEffectiveGrants`、`ports.EffectiveContractDelegationView`、`application.AdjudicateCommercialAuthorizationHandler`、`postgres.AuthorityGrants`、迁移 `0003_authorization_grant.sql` 都已在。
  - 注释：全仓 `internal/partycommercial` 找不到「另票补」。
  - 排序：`LoadEffectiveGrants` 是 `ORDER BY object_id, version_label`，按版本标签字典序取首个，正是 `UC-PC-001`「不得依赖……最高版本号自动选择」要拦的那一类（与 H2 叠加）。
- **归类**：A
- **严重度**：中。照交接段开工会重造端口与编排；未决二、三看上去仍然挡着资料修订。
- **建议修法**：重写首切范围、未决一至五的现状句与交接段：标出哪些项已由 ADR-0116 与迁移 `0032` 解决，哪些仍未决（撤回、多条命中的采用规则）；把未决五的现状改成「按对象与版本标签排序取首个」。

### M5 GLOSSARY「可执行价卡版本」写「退役或更正形成新版本」，与 ADR-0234 相反；词也与 PP CONTEXT 不同

- **类别**：6
- **位置**：`docs/domain/GLOSSARY.md`「可执行价卡版本」：「发布后正文不可覆盖；变化、退役或更正形成新版本并保留历史引用」。
- **证据**：
  - `docs/adr/0234-…md` 索引注记：「截止与退役各另册只增，版本行一字不动」。
  - `docs/domain/parcel-pricing/CONTEXT.md` 收尾规则：「都是册上追加的事实，不改写版本与既有评价」。
  - PP CONTEXT 没有「可执行价卡版本」一词，用的是「定价方案版本」「价表版本」。
  - 词条前半句「依据已解析商业价格政策和定价方案绑定形成」见 H4。
- **归类**：A
- **严重度**：中
- **建议修法**：GLOSSARY 改为「变化形成新版本；截止与退役是册上追加的收尾事实，不形成新版本」；词头对齐 PP CONTEXT 的「定价方案版本」，或在 PP CONTEXT 里定义「价卡」是哪两者的合称。

### M6 计价参考序列的实例：GLOSSARY 与 CONTEXT-MAP 写两种，PP CONTEXT 写三种

- **类别**：6
- **位置**：`docs/domain/GLOSSARY.md`「计价参考序列」：「燃油费率与汇率是它的两个实例」；`docs/domain/CONTEXT-MAP.md`「`parcel-pricing ↔ 外部数值来源`：承运商公布燃油费率，财务侧提供汇率牌价，两者都是计价之外的事实」。
- **证据**：`docs/domain/parcel-pricing/CONTEXT.md`「计价参考序列」：「燃油费率、汇率与按期公布的定额附加金额（高峰 / 需求附加费一类）是它的三个实例」（ADR-0110）；代码 `ChargeMethodSeriesAmount`。
- **归类**：A
- **严重度**：中。读 GLOSSARY 的人会把金额序列当成价卡常量写死。
- **建议修法**：GLOSSARY 与 CONTEXT-MAP 补第三个实例，引 ADR-0110。

### M7 ADR-0202 决定四「区间更正没有折进核对」已被 ADR-0233 决定五推翻，停用链两头都没有

- **类别**：3
- **位置**：`docs/adr/0202-…md` 决定四：「ADR-0038 的区间更正没有折进核对：判据只用正文区间与终止时刻，同 PS `agreementOf` 的先例」。
- **证据**：`docs/adr/0233-…md` 决定五：「它与正文点读口都把壳上最新一条区间更正的终点折进正文的判用区间」；同记录「对已接受记录的作用」写「不停用任何条款……ADR-0202、ADR-0209 正文不改，它们的越权风险点 5 由决定五回答」。ADR-0202 的 Status 行没有指向 ADR-0233 的指针。
- **归类**：A
- **严重度**：中。读 ADR-0202 的 TF 作者会以为截止对委托提交的核对不起作用。
- **建议修法**：ADR-0233 的「对已接受记录的作用」改为部分停用 ADR-0202 决定四那一句（适用场景：PC 点读口），ADR-0202 Status 行补前向指针。若认为 ADR-0202 原句「若 PC owner 裁定要折入，由 PC 在点读口一处交出」已经预留了这条路，至少补前向指针。ADR-0202 属 TF，与 TF 组或横切组协调。

### M8 三份 UC 的状态行与交接段仍按「机制待实现、只许夹具」写

- **类别**：2
- **位置**：
  - `UC-PC-001` 状态「稳定业务骨架待实现」；交接：「首个生产持久化仍受 `PN02-W01/W03` 真实证据和技术门槛阻断；可以先实现纯领域版本、不变量、冲突分类和命名夹具」。
  - `UC-PC-002` 状态同上；首发试点叠加条件：「真实证据未完成前，只允许实现解析类型、两阶段端口、命名夹具和负向测试；不得返回生产“唯一已解析”」；交接：「当前实现闸门未解除前不得建立伪造的生产商业数据适配器」。
  - `UC-PP-001` 状态「机制待实现」；交接：「`parcel-api` 端点表一行、以 `UnconfiguredIntake{}` 起步」。
- **证据**：
  - `docs/adr/0146-…md` 决定一：「机制与产品策略同属产品交付轨道，开发方现在就做」；ADR-0150：演示租户按真实租户对待。
  - 代码：PC 有生产 postgres 解析库与发布册，`cmd/parcel-api` 已装配，演示租户解析答 `UNIQUELY_RESOLVED`；`cmd/parcel-api/endpoints_pricing_estimate.go` 的 `/pricing-estimates` 挂的是 `channels.operatorRegistries.pricingOperationsRead`，不是 `UnconfiguredIntake`。
- **归类**：A
- **严重度**：中。「不得返回生产唯一已解析」与现行开发方式正面相反。
- **建议修法**：三份状态行按 ADR-0146 三分改写（机制已落、租户取值待提供）；删去或改写「只许夹具」「不得返回生产唯一已解析」两句；`UC-PP-001` 交接改成现装配。

---

## 低

### L1 PP CONTEXT「价卡发布必须通过……受控案例校验」，同文件又说治理案例没有执行器

- **类别**：1
- **位置**：`docs/domain/parcel-pricing/CONTEXT.md` Rules：「价卡发布必须通过依赖闭合、价格方向隔离、区间/单位/币种检查和受控案例校验」。
- **证据**：同文件「首发实现取舍」：「现有治理案例仍按上文源完整性门禁隔离为 `S` 且没有执行器」；代码的登记与草稿发布路径里没有案例校验门（`internal/parcelpricing` 中只有 `registered_price_card.go` 的一句注释提到治理案例）。
- **归类**：A
- **严重度**：低
- **建议修法**：给这条加限定：「受控案例校验门在案例执行器落地时启用，首发不设」；或者改归 B，另裁首发是否要这道门。

### L2 生命周期里的「已替代」「已生效」与代码、与彼此不一致

- **类别**：1
- **位置**：
  - PC CONTEXT「商业规则与政策版本」「客户合同版本」等生命周期以现行口吻写「已替代」。
  - PP CONTEXT「定价方案与价表版本」：「已发布 → 已退役或已替代」，没有提截止。
  - PC CONTEXT「客户服务规则版本」：「已发布 → 已生效：由服务产品或客户合同在明确期间引用」，与同文件「商业规则与政策版本」「已发布 → 已生效：到达明确生效边界并且引用依赖有效」是两种定义。
  - PC CONTEXT 生命周期把「定价方案绑定」列为「各对象仍保持独立身份」之一；Rules「服务产品、客户合同、……和授权规则都遵循商业版本共同不变量」这份清单漏了客户服务规则。
- **证据**：
  - ADR-0233 决定六：完整替代写口归后继票 148，生产不可达。PC 的 `CommercialVersion.SupersededBy` 没有生产调用方；PP 只有 `CutForSuccessor` 与 `RetireForCoveringSuccessor`，没有「替代」。
  - `CommercialVersion.TakeEffect` 按生效边界取效。
  - `CommercialObjectKind` 含 `CustomerServiceRuleObject`、不含绑定；绑定只是 `CommercialPricePolicy.plan` 一格。
- **归类**：A
- **严重度**：低
- **建议修法**：「已替代」注明「写口未开（票 148）」；PP 生命周期补「截止只收窄选用、不是状态迁移」；客户服务规则那一行改成与共同生命周期同一定义；两份对象清单与 `CommercialObjectKind` 对齐。

### L3 `UC-PC-002` 的「计算目的」，在代码里是 PC 自己的解析目的，不是 PP 的计算目的

- **类别**：6
- **位置**：`UC-PC-002` 两阶段契约第一阶段输入列有「计算目的」；一致性一节：「请求计价依据时，解析键还必须包含计算目的和价格方向」。`GLOSSARY.md`「计算目的」：「`party-commercial` 在解析商业依据时引用它……双方都不定义其取值」。
- **证据**：代码 `partycommercial/domain.ResolutionPurpose` 取值是 `ACCEPTANCE_CONTROL`、`PRICING`，由 PC 自己定义；PP 的 `PricingPurpose` 是 `CUSTOMER_CHARGE` 等，PC 不引用。
- **归类**：A
- **严重度**：低
- **建议修法**：UC 改称「解析目的（接受控制 / 计价）」；GLOSSARY「计算目的」删去「`party-commercial` 在解析商业依据时引用它」，或者注明 PC 用的是另一个轴。

### L4 同一次停用在 Status 行写了两遍（infra-speed/07 的形态）

- **类别**：3
- **位置与证据**：
  - `docs/adr/0209-…md` Status：「**部分停用**（2026-10-19）：决定五「登记者取自操作者信封的主体」一句已由 ADR-0218 停用……」，`<!-- 索引注记 -->` 之后又是「｜**部分停用**（ADR-0218）：决定五「登记者取自操作者信封的主体」……」。
  - `docs/adr/0104-…md` Status：ADR-0176 消解「客户服务规则没有对应的回退物」一句，标记前一次、索引注记里一次。
  - `docs/adr/0207-…md` 里 ADR-0209、ADR-0218 两次停用各写两遍（TF ADR）。
  - `docs/adr/0151-…md` 里 ADR-0187、ADR-0230、ADR-0236 三次停用各写两遍（横切）。
- **归类**：A
- **严重度**：低
- **建议修法**：按 infra-speed/07 合成一份、留在索引注记，不丢事实；0207 与 0151 交 TF 组或横切组，避免重复修。

### L5 ADR-0146 部分取代 ADR-0016，ADR-0016 没有前向指针；「第三条判据」是位置引用

- **类别**：3（兼 5）
- **位置**：`docs/adr/0016-…md` Status 只有「Accepted」；`docs/adr/0146-…md` Status：「本记录**部分取代** ADR-0016 的机制半边定义与第三条判据」。
- **证据**：ADR-0016 决定段「判据三项：骨架完整、受控案例可复算、参数显式未配置」——「第三条」靠计数指称其中一项。
- **归类**：A
- **严重度**：低（与横切组重叠）
- **建议修法**：ADR-0016 Status 行补部分停用注记与适用场景；ADR-0146 的「第三条判据」改为引文「参数显式未配置」。

### L6 Go 注释用计数引 CONTEXT，其中一处已经错了

- **类别**：5（兼 4）
- **位置**：`internal/parcelpricing/domain/value_objects.go` 的 `ChargeMethod` 注释：「CONTEXT 把取值集合闭合为四种」。
- **证据**：PP CONTEXT「计算方法」现列定额、查表、按基数百分比、取当期序列定额、两者取较大值五种；代码常量也是五个（含 `ChargeMethodSeriesAmount`）。同形但今天仍对的跨文件计数还有：`partycommercial/domain/credit_limit.go`「SA CONTEXT 运营结算余额五项之一」，`partycommercial/adapters/postgres/commercial_resolution.go`「ADR-0044 只留两格」。
- **归类**：A
- **严重度**：低
- **建议修法**：三处都改为引词条名或原词，不写数（AGENTS.md「改文档」：Go 注释同受此约束）。

### L7 文档里的跨文件计数与区间引用

- **类别**：5
- **位置**：
  - `UC-PC-001`：「`PAR-COM-01..17` 的真实状态仍只在参数登记册维护」。
  - PP CONTEXT 源完整性门禁：「七条中五条已裁决」「当前 136 个案例中 83 个 `NORMATIVE_SYNTHETIC` 和 12 个 `UPSTREAM_CONSISTENCY`……41 个 `SOURCE_RATE_CARD`」。
- **证据**：
  - 登记册已有 `PAR-COM-18`（商业发布审批职责规则），区间已经漏项。
  - PP 那几处数的是 `pp-golden-case-transcript.md` 与 `PP-S03-W01` 里的东西，没有锚定取证 SHA。顺带：转录本 `SOURCE_RATE_CARD` 一行仍写「`SRC-DISC-001` 至 `004` 取得业务裁决前不得用于金额验证」，与 PP CONTEXT「`001` 不成立……`003` 与 `004` 不成立」相抵。
- **归类**：A
- **严重度**：低
- **建议修法**：`UC-PC-001` 改成「`PAR-COM-*` 各行」；PP CONTEXT 的计数或者锚定取证 SHA（数本身就是论点时），或者改成指向转录本的分层表；转录本那一行另票改。

### L8 「未形成」一词两用；`UC-PP-001` 又叫它「失败」

- **类别**：6
- **位置**：PP CONTEXT「价格评价」生命周期「待形成 → 未形成：请求不合法或计算无法完成」（评价的一种状态，代码 `EvaluationFailed`）；同文件 Rules 与 GLOSSARY「评价未形成」（根本没有形成评价）；`UC-PP-001`「评价自己的状态（完成 / 待判断 / 不可计价 / 冲突 / 失败）」。
- **证据**：代码 `domain.EvaluationStatus` 的 `FAILED` 与 ADR-0229 的「评价未形成」信封是两件事；GLOSSARY「评价未形成」把「评价失败」列为避免用词。
- **归类**：A
- **严重度**：低
- **建议修法**：PP CONTEXT 给 `FAILED` 那一格另取一个词（如「评价失败」），或者明说两者的区别；`UC-PP-001` 用同一个词。

### L9 `UC-PP-001` 边界写「结算不得据试算形成任何金额」「不交任何下游」，漏了 CONTEXT 的唯一例外

- **类别**：2
- **位置**：`UC-PP-001` 目标与边界：「`settlement-accounting` 不得据试算评价形成任何金额（同上）」「试算评价不入评价册、不交任何下游」。
- **证据**：PP CONTEXT「评价对象」：「唯一的例外是接受前估价（ADR-0179）」；代码 `application.FormPreAcceptanceEstimatesHandler` 把逐件试算合计交给结算作控制金额。
- **归类**：A
- **严重度**：低（本用例本身只讲操作者试算）
- **建议修法**：在「同上」之后补「接受前估价例外见 ADR-0179，不经本用例」。

### L10 PC CONTEXT 用到「内部成本政策」，Language 与 GLOSSARY 都没有定义

- **类别**：6
- **位置**：PC CONTEXT 收尾规则：「按版本点读、用于新决定的口（供应商协议、内部成本政策）同一口径」。
- **证据**：PC CONTEXT Language 与 GLOSSARY 都没有这个词条；代码有 `postgres/internal_cost_policy.go`（INTERNAL 方向价格政策的点读），ADR-0233 越权风险点 7 也用了这个词。
- **归类**：A
- **严重度**：低
- **建议修法**：在 PC Language 里定义，或者改写成「`INTERNAL` 方向商业价格政策的版本点读」。

### L11 `UC-PP-001` 说正式评价「选卡与本用例同一套口径」，两条路的冲突判法其实不同

- **类别**：2
- **位置**：`UC-PP-001`「按评价请求形成评价」一节：「选卡与本用例同一套口径（无价卡、适用冲突各成一格）」。
- **证据**：
  - 试算：`application.FormEstimateEvaluationsHandler` 经 `LoadApplicable` 允许不同方案并列，只在同一方案两版同时适用时答冲突。
  - 正式：`ports.PriceCardInForceResolver` 注释写明「多于一版同时适用（不同方案身份，或同一方案两版）」都答 `APPLICABILITY_CONFLICT`（ADR-0234 越权风险点 5 也写了跨方案退不了场）。
  - PP CONTEXT「多于一版同时适用」没有说清包不包括跨方案。
- **归类**：A
- **严重度**：低
- **建议修法**：`UC-PP-001` 改成「收尾折法同一套；冲突判法不同：正式路径跨方案并存也答适用冲突」；PP CONTEXT 那一句注明正式路径包括不同方案。

---

## 已核无问题的要点

| 要点 | 核对结果 |
|---|---|
| PC 解析结果格（`UC-PC-002` 七格 vs `domain.ResolutionOutcome`） | 一一对应：`UNIQUELY_RESOLVED`、`NO_APPLICABLE_BASIS`、`APPLICABILITY_CONFLICT`、`RESOLUTION_PENDING`、`INPUT_NOT_ACCEPTED`、`STALE`、`BASIS_NOT_RESOLVED` |
| 回指失败按恢复动作分格（PC CONTEXT、`UC-PC-002`、ADR-0029、ADR-0027 停用链） | 一致，ADR-0027 的停用注记在，所引原句逐字存在 |
| PC 版本状态（`CommercialVersionStatus`） | 草稿、已发布、已生效、已到期、已退役、已替代六格与 CONTEXT 词一致；批准不是独立状态，与注释、ADR-0035 一致 |
| PC 收尾（ADR-0233）落地 | 两口 `/commercial-version-successor-cuts`、`/commercial-version-retirements`、CLI `register-version-closures`，迁移 `0040`，领域与用例符号均在；`AT-PC-054..058` 判法与代码一致；`AGREEMENT_NOT_EFFECTIVE`、`AGREEMENT_TERMINATED` 在 TF 代码中存在 |
| PP 收尾（ADR-0234）落地 | 两口 `/pricing-price-card-successor-cuts`、`/pricing-price-card-retirements`、`parcel-pricing-register -kind price-card-cut/-retirement`，迁移 `0013`，`SelectablePriceCardPeriod`、`ClosePriceCardVersionHandler`、`PriceCardClosureRegistry` 均在；PP CONTEXT 收尾规则与 `AT-PP-007..012` 一致 |
| PP 评价结果四格不互替（CONTEXT vs `EvaluationStatus`） | `COMPLETED`、`PENDING`、`CONFLICT`、`FAILED`、`UNRATABLE` 与五种结果对应（用词问题见 L8） |
| 试算结果格（`UC-PP-001` vs `FormEstimateEvaluations`） | `FORMED`、`PRICE_CARD_NOT_CONFIGURED`、`PRICE_CARD_APPLICABILITY_CONFLICT`、`NOT_ACCEPTED`、`UNDECIDED`，逐卡 `EVALUATED`、`INPUT_INCOMPLETE` 一致 |
| PP 草稿四态（CONTEXT 生命周期 vs `PriceCardDraftStatus`） | 草稿、已校验、已批准、已发布一致 |
| 重量方法、特征、目录种类闭合集 | `ACTUAL_ONLY`、`MAX`；特征十项；目录两种，与 CONTEXT 一致（`ChargeMethod` 注释计数见 L6） |
| 在用序列版本选法（ADR-0099、0109） | 序列与目录共用 `selectInForceVersion` 一处 |
| 收费发生条件（ADR-0209、PC CONTEXT、GLOSSARY、`AT-PC-051..053`） | `PCC-1`/`PCC-2` 双形、`NOT_CONFIGURED`、`NOT_CHARGED`、`AGREEMENT_DECLARED`、`SCOPE_ALREADY_CHARGED` 均在代码；GLOSSARY 定义与 CONTEXT 一致 |
| 规范化与摘要停用链（ADR-0014 ↔ 0177、0201、0202、0213；ADR-0126 ↔ 0178；ADR-0104 ↔ 0176） | 两头都在，被停原句逐字存在（重复书写见 L4） |
| ADR-0229 对 SA 四份 ADR 的停用链（0190、0198、0200、0208） | 两头都在 |
| 授权裁定四格（`UC-PC-003` 结果语义 vs `domain.Authorize`） | `已授权`、`ErrNotAuthorized`、`ErrAuthorityRulesNotConfigured`、error 四格，按「范围加时刻」判未配置，与 UC 一致（过时描述见 M4） |
| `ManualReviewRequirementFor` 按 `UC-PC-003` 裁决删去 | 全仓已无此符号 |
| 法人资料、责任法人身份、货主客户账户（PC CONTEXT vs GLOSSARY） | 定义一致 |
| 声明服务产品身份收窄（ADR-0153、PC CONTEXT、`AT-PC-050`） | 一致 |
| 汇率口径进序列（PP CONTEXT「不接受未声明口径的裸汇率」） | `NewQuotedReferenceSeriesValue` 对汇率种类必带 `quoteBasis` 引用 |
