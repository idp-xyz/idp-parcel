# 解析键登记面装不下结算选择器——接受前控制链因此整条走不通，且合同维有一处循环要先裁

Category: enhancement
Status: ready-for-agent（裁决未出，见「三案」；实现前须先定案）

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
