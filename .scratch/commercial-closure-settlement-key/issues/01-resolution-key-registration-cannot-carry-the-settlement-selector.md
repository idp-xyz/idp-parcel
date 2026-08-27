# 解析键登记面装不下结算选择器——接受前控制链因此整条走不通，且合同维有一处循环要先裁

Category: enhancement
Status: blocked（裁决已出＝乙案，落 [ADR-0080](../../../docs/adr/0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)；PC 解析顺序、PS 登记面与种子三段已落。阻断一已由 [02](./02-settlement-policy-body-has-no-publication-channel.md) 清除，收口只剩阻断二，见末节）
Blocked by: 03

发现于 [sa-preacceptance-policy-view/01](../../sa-preacceptance-policy-view/issues/01-sa-preacceptance-control-policy-view-has-no-production-adapter.md) 收口时留的那条遗留（锚 `0fb4040`）。那一票把 SA 这一侧全部做完了——控制策略视图有了生产适配器，判据 B 该口从缺转有——但适配器**暂不装进 `cmd/parcel-api`**，因为上游形不成它要读的那份闭包。本票就是那一道。

## 事实链（实读代码取证，锚 `0fb4040`）

1. **登记面明拒两类依据**。`internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go` 的 `validate()` 对 `SettlementPolicyObject` 与 `PriceRuleObject` 直接报错，原话：「需要键携带额外选择维度，本登记面不承载」。库内 `migrations/parcel_shipment/0007_commercial_resolution_key.sql` 的 `..._bases_closed` CHECK 是同一判据的第二道镜像——封闭名集里没有 `SETTLEMENT_POLICY` 与 `PRICE_RULE`。该迁移的注释已经把今天这件事写好了：「日后接通结算作用域缝时随新决定扩列，不在这里预留。」

2. **缺了它，接受前控制链整条断在第一步**。`ResolveCommercialClosure` 按 `RequiredBases` 逐项解析；没有 `SettlementPolicyObject`，闭包里就没有已采用结算政策。于是：
   - PS 侧 `CommercialBasisSnapshot.SettlementTerms()` 缺席（PS→PC 适配器正是从 `closure.AdoptedFor(SettlementPolicyObject)` 取它）；
   - `PolicyBackedControlScopeSource.FormControlScope` 据此交回 `formed=false`，编排停在 `CONTROL_SCOPE_NOT_CONFIGURED`；
   - SA→PC 控制策略适配器的`要求`格取不到方式与采用政策。

   **注意这道停摆与「账户映射未配置」同码不同因**。账户映射确属实例半边（没有租户就没有目录），停在那里是对的；而闭包形不成是**机制半边的欠账**，两者今天挤在同一个 `CONTROL_SCOPE_NOT_CONFIGURED` 里。补齐本票后那一格才真的只剩实例半边那一半——这本身就是本票的一项收益：让那个码只说一件事。

3. **持久化那一半刚补齐**。采用了结算政策的闭包原先写得进读不回（解析键上的选择器与政策正文都没落库）；已于 `5911d3b` 修复，配对用例在 `internal/partycommercial/adapters/postgres/commercial_resolution_test.go`。所以本票不必再顾虑「扩了键也存不住」。

4. **合同维有一处循环**（本票要先裁的那件事，见下节）。`SettlementSelector.Contract` 是 `CommercialVersionLabel`，`resolveSettlementPolicyBasis` 拿它去做 `NewSettlementQuery` 的精确六维匹配，而登记夹具里它的取值形如 `contract-1/v1`——**它指名一个客户合同版本**。可同一个闭包的另一项必需依据 `CustomerContractObject` 要解析的正是「哪一版合同适用」。

   两者撞在同一条纪律上：`psports.CommercialBasisQuery` 的注释写着「它只携带引用：本上下文说明需要哪种依据，**绝不指定应当选中哪个商业版本**」。把合同版本标签登记进 PS 的键登记面，就是消费方在指定该选中哪个版本。

## 三案（裁决未出）

- **甲：登记面照收四维，含合同版本标签。** 最省事，也最直接撞上第 4 条那句纪律。若采纳，至少要补一道交叉核对：闭包解析出的客户合同版本必须与选择器里的那一个相同，否则整份闭包判`输入未受理`——不许出现「按 A 版合同接单、按 B 版合同的结算政策控制」。**代价**：租户每换一版合同就得改一行登记，而换版本本是商业侧的正常动作；漏改的后果不是报错而是解析不出结果，看起来像「没登记过结算政策」。

- **乙：闭包解析分两段——先解合同，再拿解出的合同版本去解结算政策。** 登记面只收三维（相对方、费用范围、币种），合同维由解析器填。语义上最正：结算政策本就是「这一版合同下的结算约定」，合同是它的前提而不是它的输入。**代价**：`ResolveCommercialClosure` 今天对每一项必需依据独立调 `singleBasisKey(kind)`，是无序的；引入依赖顺序是 PC 领域层的形状改动，够一份 ADR。还要定一格：合同解不出时结算政策记`无适用依据`还是另立一格「前提未解析」——两者恢复动作不同（ADR-0029 判据）。

- **丙：费用范围与币种不进登记面，随请求过来。** 理由是这两维听起来像逐票事实而非租户配置。**取证不支持**：`CommercialBasisQuery` 今天只有身份与两个单据标识，没有金额也没有币种；给它加维是另一个决定，且首发没有真实报价，逐票币种从哪来说不出。本案宜否决，但要写进记录——否则下一个人会重新想一遍。

**倾向（非裁决）**：乙。它把「合同是结算约定的前提」这件领域事实落进解析顺序里，而甲是把同一件事外包给租户每次手工对齐，且失败形状是静默的。但乙动 PC 领域层，代价要由裁决者认下。

## 实现范围（定案后）

- PC：按选定方案调整 `ClosureResolutionKey` / `ResolveCommercialClosure`（乙案还需定「前提未解析」那一格）；快照持久化已支持结算选择器与政策正文，只需跟随。
- PS：`ResolutionKeyRegistration` 与 `ResolutionKeyRow` 扩维；`validate()` 与迁移 CHECK 两道镜像同步放行 `SETTLEMENT_POLICY`（`PRICE_RULE` 是否一并放行属另一决定——价格方向那一维与本票的结算四维不是同一件事，且价格政策的快照重建仍缺席，见 `RehydrateAdoptedBasisSpec` 注释）。
- 迁移：新增列，不改 0007（已发布迁移不回改）。
- `cmd/parcel-commercial` 的 `resolutionKeyDocument` 与 `keyRegistrationFromJSON` 跟随；`scripts/demo-seeds` 补一行含结算依据的登记。
- 装上 SA→PC 控制策略适配器到 `cmd/parcel-api` 的接受链——这是本票的收口动作，也是 sa-preacceptance-policy-view/01 留下的那条遗留。

## 完成标准

- 种子租户下接受前控制走通到`要求-预付`分支，且**仍停在账户映射未配置**（那一格属实例半边，不得为验它而造账户映射）——判据是停摆原因从「闭包形不成」变成「账户目录未配置」，两者今天同码，届时要能分辨。
- `CONTROL_SCOPE_NOT_CONFIGURED` 只剩实例半边一个成因（第 2 条）。
- 甲案若获采纳，须有一例证「解出的合同与选择器不符时整份闭包判`输入未受理`」；乙案若获采纳，须有一例证「合同解不出时结算政策不落成`无适用依据`」。

## 地盘

`internal/parcelshipment/adapters/{partycommercial,postgres}`、`internal/partycommercial/domain`、`migrations/parcel_shipment/`、`cmd/parcel-commercial`、`cmd/parcel-api` 接受链装配。MCP-1 于 `0fb4040` 时在 `internal/customscompliance/**`（前沿票 05/06），无重叠；`cmd/parcel-api` 那一处本票只加装配行、不增删端点。

## 进展与两条阻断（2026-08-27 · MCP-1，锚 `9c95d7c`）

裁决取乙案，另落 [ADR-0080](../../../docs/adr/0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)。「实现范围」六条里已落四条：

- **PC 领域层的两段解析** — `resolutionOrder` 把结算政策排到最后，`adoptedContractLabel` 取解出的合同补第四维，`premiseUnresolved` 单独成格。
- **PS 登记面扩三维、放行 `SETTLEMENT_POLICY`** — `8dfe2e4`；`validateSettlement` 与迁移 `0008` 的 `..._settlement_paired` 两道镜像。
- **`cmd/parcel-commercial` 跟随** — 同笔，`settlementSelectorDocument` 三维、给了节却少一维就响亮失败。
- **种子补一行含结算依据的登记** — `9c95d7c`。

### 阻断一：结算政策没有任何发布路径，因此种子里发不出一份可被采用的结算政策

`ports.PublicationRegistry.SaveSettlementPolicy` 有端口、有真库适配器
（`internal/partycommercial/adapters/postgres/settlement_policy.go`）、有配对用例，但
**没有任何生产调用方**——`PublishCommercialAuthorityHandler` 的 `declarationWrites` 里没有
结算政策这一路，`cmd/parcel-commercial` 的发布批文档也没有承载它的字段。种子 README 的
「已知边界」早已记着这条（与价格政策、服务产品形态同处），写它时那还只是「商业策略页
两列为空」；本票把它顶成了主径上的墙。

真库取证（演示库灌完种子后跑「折键 → 解闭包」，锚 `9c95d7c`）：

    formed=true purpose=ACCEPTANCE_CONTROL
    bases=[SERVICE_PRODUCT ACCEPTANCE_RULE_PACKAGE CUSTOMER_CONTRACT SETTLEMENT_POLICY]
    settlement="SYN-ACCOUNT-01"/"SYN-CHARGE-PREPAID"/"CNY" contract=""
    outcome=NO_APPLICABLE_BASIS unresolved=[SETTLEMENT_POLICY]
    conflicting=[] premiseUnresolved=[]

`premiseUnresolved` 空说明合同解出来了、这一项是真的问过；剩下的唯一成因是本范围里没有
已发布的结算政策版本。补它要在 `internal/partycommercial/{application,ports}` 加一路声明
通道（`SaveSettlementPolicy` 交回的是 `SettlementPolicySaveOutcome` 而不是
`DeclarationSaveOutcome`，`declarationWrite` 的形状要跟着改一格），并决定价格政策的同处
缺席要不要一并补——那是另一道决定，且不在本票地盘内（本票只写 `internal/partycommercial/domain`）。

**已由票 [02](./02-settlement-policy-body-has-no-publication-channel.md) 清除**（`ae966e6` /
`41250b1`）。端口未改：`SettlementPolicySaveOutcome` 在应用层逐值折成 `DeclarationSaveOutcome`；
价格政策的同处缺席显式不补（理由记在 02 的收口一节）。同一支探针在同一个演示库上现在答：

    outcome=UNIQUELY_RESOLVED unresolved=[] conflicting=[] premiseUnresolved=[] adopted=4
      adopted SETTLEMENT_POLICY -> SYN-SETTLEMENT-PREPAID-01/v1
        method=PREPAID contract=SYN-CONTRACT-01/v1 chargeScope=SYN-CHARGE-PREPAID currency=CNY

### 阻断二：`cmd/parcel-api` 根本没有接受链

票面「实现范围」最后一条与 sa-preacceptance-policy-view/01 的占号核对都假定
`cmd/parcel-api` 有一条接受链等着装配适配器。实读代码不成立：

- `NewAdvanceAcceptanceJudgmentHandler`、`NewAdvanceFinancialControlJudgmentHandler`、
  `NewFormAcceptanceDecisionHandler` 三个编排的构造函数在 `cmd/` 下**只有**
  `cmd/parcel-dispatch/synthetic_v0_test.go` 一个调用点，那是测试夹具。
- `saapplication.NewApplyPreAcceptanceControlHandler` 同样只在测试里被调；
  `cmd/parcel-api/assemble_withdrawal.go` 装的是同一个适配器的**释放**半边，`Apply` 显式留空。
- SA→PC 控制策略适配器 `partycommercial.NewPreAcceptanceControlPolicy` 零生产调用方。
- `assembleBusinessEndpoints` 的端点清单里没有任何接受判断入口。

所以「装上适配器」不是加一行装配，而是先给接受链一个装配点与进程入口——那是增端点，
票面自己写着本票不增删端点（占号纪律）。这一段应另开票，并先定它是 HTTP 端点还是像
派发那样由信封驱动。

### 完成标准逐条

1. **种子租户下走通到`要求-预付`并停在账户映射未配置** — 未达成，被阻断二挡住（阻断一已清）。
   闭包现在解得开、结算依据带得出方式与六维范围，但没有任何进程会去走那条接受前控制链，
   所以「停在账户目录未配置」这一格今天仍无处可观察。
2. **`CONTROL_SCOPE_NOT_CONFIGURED` 只剩实例半边一个成因** — 机制半边的两处欠账都已清
   （登记面装得下结算选择器、权威册里有结算政策），`PolicyBackedControlScopeSource` 不再走
   `SettlementTerms()` 缺席那一支。仍不可端到端举证，理由同上条：没有装配点就没有调用方。
3. **乙案的那一例证** — 已达成：
   `internal/partycommercial/domain/settlement_basis_resolution_test.go` 的
   `TestAnUnresolvedContractLeavesTheSettlementBasisUnasked` 证「合同解不出时结算政策落
   `前提未解析`、不落`无适用依据`」；配对的 `TestASettlementPolicyNamingAnotherContractIsNotAdopted`
   证反面（合同解出来了就真去问，且不采用指名另一版合同的结算约定）。
