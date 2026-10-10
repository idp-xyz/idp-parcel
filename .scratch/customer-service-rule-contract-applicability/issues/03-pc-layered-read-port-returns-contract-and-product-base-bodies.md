# 03 PC 层次读口：一次取回合同版正文、产品底座版正文与各自在场标志

Category: enhancement
Status: resolved——2026-10-11 00:5x 通道 4 代推送方重放进 main（用户 00:4x 裁定：其余通道都已崩溃，由通道 4 独自收口）：`835252bd`…`16f27492`，清点 `547908ad`；评审是作者自审（隔离评审子代理两次报认证错误、起不来），不是非作者评审，见文末 Comments 与进 main 记录。此前：完工，待评审与重放——2026-10-11 00:0x 通道 4 在分支 `mcp4-csr03`（基 `70f32c2a`）上做完，分支已推 origin：本票代码 `4d19cc2c`、`3031a1d2`、`943f1cd7`，另一笔 02 复评补测 `65fdcd20`，清点 tip `ae2a949e`；完成记录见文末。此前：in-progress——2026-10-10 23:4x 通道 4 认领（单 task-628d9b90，改派自通道 5 `task-96bf9634`），分支 `mcp4-csr03`，基 `70f32c2a`。此前：ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: [02](02-pc-closure-resolves-contract-tier-service-rule-first.md)——底座必须用 02 抽出的那一处选法，不造第二套口径
父票：[spec](../spec.md)
地盘：`internal/partycommercial` 的 ports、application、adapters/postgres；碰 Go / SQL，走并行会话那条路。
出处：[ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定二；spec「Testing Decisions」缝三。

## 做什么

1. 新读口按（租户、范围、合同版本、锚点）一次取回：合同版正文、产品底座版正文、各自在场标志。与 `LoadCustomerServiceRule` 同族，ports + application + postgres。
2. 底座在读口内按 02 那一处选法选「同范围挂服务产品、锚点生效」的版本；底座多候选照`适用冲突`纪律答。
3. 既有单版点读口不动。
4. 用例镜像 postgres `customer_service_rule_test` 一族：在场标志、底座多候选、租户隔离。

## 完成判据

- [x] 合同版与底座版在场 / 不在场四种组合各有真库用例（带 DSN，`-v` 下 PASS 非 SKIP）。
- [x] 底座多候选答`适用冲突`；租户隔离有用例。
- [x] 单版点读口答复不变。

## 完成记录（2026-10-11，通道 4，分支 `mcp4-csr03`）

**落点**

| 笔 | 段 | 做了什么 |
|---|---|---|
| `7e750bbd` | 票面 | 认领：Status 转 in-progress |
| `4d19cc2c` | ports | 新文件 `ports/customer_service_rule_layers.go`：层次读口 `CustomerServiceRuleLayerView.LoadCustomerServiceRuleLayers(ctx, tenant, scope, contractRule, anchor, declaredProduct)` 与答复形 `CustomerServiceRuleLayers`——合同版正文与在场、底座选法的答案 `ProductBaseOutcome`、底座正文与在场 |
| `3031a1d2` | application | 新文件 `application/customer_service_rule_layers.go`：`CustomerServiceRuleLayerReader` 实现该口。底座经 `CommercialAuthorityView` 与 `CommercialRegistry.CustomerServiceRuleProductBase` 选，两层正文都经 `CustomerServiceRuleContentView.LoadCustomerServiceRule` 取 |
| `943f1cd7` | adapters/postgres | 新用例文件 `customer_service_rule_layers_test.go`（缝三，真库）；读口经 postgres 的 `CommercialAuthority` 与 `CustomerServiceRuleContents` 装配。postgres 生产代码未加 |
| `65fdcd20` | 02 复评补测 | 见下「另一笔」 |
| `ae2a949e` | 清点 | 在 `65fdcd20` 的干净树（`git status --short --ignored` 为空）重生成：partycommercial 生产 154 → 156、测试 163 → 164、应用编排 12 → 13，端口声明 457 → 458；新口记在基线口径缺，标虚低，精确口径实现者 `application.CustomerServiceRuleLayerReader`。其余各格零变化 |

**完成判据**

- ✅ 四种组合各有真库用例：`TestTheLayeredReadAnswersEachTierPresenceOnItsOwn`，合同版（在场 / 壳在而没登正文）× 底座（在场 / 范围里没有产品版 / 选出那一版但没登正文），表驱动各一格。底座「不在场」的两种成因分开配：前者选法答`无适用依据`，后者答`唯一解析`而在场为假。没有产品版的那几格同时证范围里那一版合同版本身不被选作底座。
- ✅ 底座多候选答`适用冲突`：`TestTwoProductBaseCandidatesAreAnApplicabilityConflictNotAPick`，指名服务产品的一版加什么都没指名的一版，不挑、不交底座正文，合同版照常交回。租户隔离：`TestTheLayeredReadIsBoundToItsTenant`，拿他租户身份读本租户合同版是 error 且两层都不交；他租户在同名范围里登的产品版不是本租户的候选（`无适用依据`）。
- ✅ 单版点读口答复不变：`git diff --stat 70f32c2a ae2a949e -- internal/partycommercial/adapters/postgres/customer_service_rule.go internal/partycommercial/ports/ports.go` 为空；`customer_service_rule_test.go` 各真库用例 `-v` 全 PASS。层次读口两层都经它取正文，没有另写读法。
- 另有四条：`TestTheDeclaredServiceProductNarrowsTheProductBaseAsTheClosureDoes`（判断项 2）、`TestAnUnreadableAuthorityAndAMissingAnchorBothLeaveTheProductBasePending`（判断项 5）、`TestTheLayeredReadRefusesAnAskItCannotAnswer`（判断项 7）、`TestABadProductBaseBodyIsAnErrorNotAnAbsentBase`（底座正文壳与正文分歧走 error，不折成底座不在场）。

**red 与判别力**

- red：ports 与用例就位、实现先写成交回零值的空桩，同一组用例带 DSN 跑全红，都是断言失败、无 SKIP——两层在场全为假、选法答案为空串、该报 error 的三处答 nil。
- 临时变异七处，经 `go test -overlay` 换掉实现文件，工作副本未动；每一处都被拿住：
  - M1 底座不按声明的服务产品收窄：声明产品收窄那条红。
  - M2 底座冲突改答`无适用依据`：冲突、声明产品收窄两条红。
  - M3 不核范围：问法立不住那条红。
  - M4 底座正文读错折成不在场：底座坏数据那条红。
  - M5 权威读不到上抛 error：未决那条红。
  - M6 不核类别：问法立不住那条红。
  - M7 不读合同版正文：四组合里合同版在场的格、冲突、租户、未决都红。
- 没变异的一处是读口自己的租户核：去掉它，点读口那道同名核照样答 error，用例不红。两道是同一条纪律的两层，读口这一道让「不读任何一层」不依赖点读口排在前面。

**门**（钉 `65fdcd20` 的工作树，`git status --short --ignored` 为空；`ae2a949e` 只改清点 `.md`，代码相同。WSL，go1.26.8，DSN 为门禁库 55432）

- 改动的 `.go` 都 `gofmt -l` 无输出，无 CR、无 BOM。
- `go build ./...`、`go vet ./...` 退 0。
- `go list -test` 反查改动的四个包（ports、application、domain、adapters/postgres）的反向依赖（生产依赖 ∪ 测试二进制依赖），连同自身得 21 个包，含 `cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-dispatch`；连同 `./internal/architecture/...` 以 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable go test -count=1 -p 1` 跑：退 0，21 ok / 0 FAIL / 1 无测试文件（ports），62 s。
- `-v` 单跑层次读口的四组合、冲突、租户三条与既有单版点读口各条：PASS，非 SKIP。
- 全量留给推送方重放后那一跑。证据层级 S。

**判断项**

1. **读口键里的「合同版本」读作闭包采纳的那一版客户服务规则（合同版），不是客户合同版本。** 受理时冻结的是规则版本，按它点读才不因后续发布改口（spec 用户故事 12）；04 要核「正文挂合同 = 查询携带的合同」，读口若按客户合同重选合同版，那道核就恒真；解析位在 PC，合同版闭包已选过，读口再选一次就是第二套口径。读口不核传入那一版的壳是否指名合同：04 若按正文认「挂合同版」，壳空而正文挂合同的那一形态（02 判断项 1，`SYN-CSR-1`）也会走到这里，拒它会让 04 的一种合理接法直接报错。
2. **读口多带 `declaredProduct`（可缺席），取自冻结闭包键 `ResolutionKey().ServiceProduct`**（02 评审 ← 通道 3 Spec ②）。ADR-0176 决定二、spec 与本票写的读口键是（租户、范围、合同版本、锚点）。不带这一维，同范围两个产品各有一版时底座答`适用冲突`，而闭包回落在同一处按声明的产品收窄、答唯一——底座与闭包回落层成了两套口径，正是决定二「VE 不自行解析、不造第二套口径」要防的。带了也不改决定二：底座仍是「同范围挂服务产品、锚点生效」那一层，多候选仍答`适用冲突`，只是与闭包同样按声明的产品收窄；缺席时与闭包键不带产品时同一答法。`TestTheDeclaredServiceProductNarrowsTheProductBaseAsTheClosureDoes` 钉住。**越权风险点 · 待 PC owner 复核。**
3. **实现落在 application，postgres 不加生产代码。** 票面与 ADR 写「ports + application + postgres，与 LoadCustomerServiceRule 同族」。底座必须与闭包读同一份权威视图、经同一处选法，正文必须过点读口里「壳与正文一致」与坏数据两道判据；组合两只既有端口与一处领域选法是应用层的活，先例是 networkrouting 的 `CatalogInitialRouteEvidence` 实现 ports 读口。在 postgres 另写一条取底座或正文的查询就是第二处口径。postgres 一层因此只有缝三的真库契约用例。04 在 `cmd/parcel-api` 装配时用 `application.NewCustomerServiceRuleLayerReader(authority, contents)`。
4. **底座的各格以既有 `ResolutionOutcome` 交出，不是 error，不新立格。** 底座在场 = 选法答`唯一解析`且那一版正文已登记。选出了而没登正文与「范围里没有产品版」分得开，恢复动作不同（补正文 / 登一版）。
5. **未决不带原因**（02 评审 ← 通道 3 Spec ②）：锚点立不住与权威读不到同答`解析未决`，沿用 `CustomerServiceRuleProductBase` 的既有答法。权威读不到不上抛 error，与第一阶段 `ResolveCommercialBasisHandler` 同一条分界；正文读不到仍走 error，判据随点读口。本票不扩这一格，要分原因留给 owner。
6. **本口不核底座正文挂的是不是服务产品**（02 判断项 1 的遗留）。选法只看壳：壳上什么都没指名而正文挂合同 X 的那一版，会被选作合同 02 那一户的底座；没有人核的话，消费方会把 X 的条款补进 02 的行（spec 用户故事 9）。本口交回的每份正文都带适用声明，ADR-0176 决定四「正文挂合同 = 查询携带的合同」若在 04 对底座正文同样施行，这一形态就答 `ErrUntranslatableAnswer`，与它在闭包回落那条路上的答法一致；端口头注已点名。另一条路是在本口把它答成某一格，那要新立格或改 02 的分层。**越权风险点 · 待 PC owner 复核（与 02 判断项 1 一并）；04 接手时须定在哪一侧核。**
7. **问法立不住走 error，不读任何一层**：租户、范围或合同版身份缺席，类别不是客户服务规则，租户与合同版不同身份，范围不是合同版自己的范围（别的范围的产品版对这一户不可见，拿它补行就是跨范围套条款）。都是调用方违约，不是任一格业务答案。
8. **两层各自答**：合同版没登正文时照常选底座、交底座，怎么用归 04。

**未做 / 风险**

- 04–06 未做；演示种子未动；`cmd/` 未接线——层次读口今天没有生产消费方，由 04 接。
- 判断项 2、6 待 PC owner 复核。

**另一笔：02 复评非阻断的补测（`65fdcd20`）**

- 出处：复评 ← 通道 2（`task-9de81782`，钉 `76120b55`）记下的非阻断——`memberOrder`「结算政策排最后」那半没有用例守。
- `TestAnExistingProductOnlyRegistrationResolvesAsBefore` 加「结算政策先于合同声明」：两种在册壳各一个子测试，各用一份登记册单跑（原有那几格先红时它照样跑到）；夹具补 `registerSettlementPolicyIn`，`closureKey` 带三维结算选择器；声明次序为结算政策、客户合同、客户服务规则，期望成员次序为客户合同、客户服务规则、结算政策。
- 判别力（`go test -overlay`，工作副本未动；`reference_closure.go` 自 `78bc40f5` 起无改动）：R1 把它取成 `78bc40f5^` 那一版、即改回修复前，新格两子测试都红，成员次序答 `[CUSTOMER_CONTRACT SETTLEMENT_POLICY CUSTOMER_SERVICE_RULE]`，原有「规则先声明」那格同红；R2 令 `memberOrder` 不再挪结算政策、即复评那次变异，只有新格红，原有两格照绿。现码四个子测试 PASS。
- 判断项：新格不钉解析标识——键带结算选择器，标识与原有格不同，派单只要成员次序。生产代码未改。

## Comments

### 评审 ← 通道 4（作者自审，不是非作者评审）· 钉 `ec2133b0` · 00:4x

**这一格没守住规矩，如实记下。** [parallel-sessions](../../../docs/agents/parallel-sessions.md)「合入前独立评审」要求代码票进 main 前有一份非作者评审，没有空闲通道时由推送方用 `/code-review` 的隔离子代理跑，不由作者自评。00:4x 用户裁定其余通道都已崩溃、由通道 4 独自决定并收口；通道 4 照那条退路派了两次隔离评审子代理，都以 `Authentication error` 起不来，于是改为作者自审后重放。自审看不到作者自己的盲点，**宜由 PC owner 或下一个空闲会话事后补一份非作者评审**，重点是判断项 2、6 两条越权风险点。

- **Standards**：阻断无。非阻断两条：① `CustomerServiceRuleLayerReader` 自己的租户核与点读口那道同名核重叠，变异时去掉它用例不红，留着是为了「不读任何一层」不依赖点读口排在前面（完成记录已写）；② `ports.CustomerServiceRuleLayers` 是导出字段的结构，「在场为真而选法不是唯一解析」这种不一致的组合在类型上拦不住，与本包各目录行类型同一先例，消费方只能信提供方。无发现：机械核过新增注释——中文、无行号引用、无跨文件计数；ports 只引 domain，application 不引 adapters；迁移、`cmd/` 与共享接线文件零改动；生产文件里没有写死的租户取值。
- **Spec**：阻断无。非阻断：判断项 6 的风险仍开着——壳空而正文挂合同的那一版会被选作底座，04 接手时须定在哪一侧核。无发现：完成判据三条逐条对过用例；底座只经 `CustomerServiceRuleProductBase` 选，没有第二套口径；单版点读口与 `ports.go` 自基起零改动。

## 进 main 记录（通道 4 代推送方 · 2026-10-11 00:5x）

- 远端 main `4b767047` → `547908ad`（本记录随其后一笔）：分支 `mcp4-csr03` 的 `7e750bbd`、`4d19cc2c`、`3031a1d2`、`943f1cd7`、`65fdcd20`、`ec2133b0` cherry-pick 重放到 tip，SHA 换了，在 main 上依次是 `835252bd`、`abc3cf39`、`03ffad10`、`857cc89c`、`982a0886`、`16f27492`。清点笔 `ae2a949e` 不重放——main 上的清点自基 `70f32c2a` 起已被别的票改过——在重放 tip 上重生成为 `547908ad`。重放前核过 `70f32c2a..4b767047` 与本票文件只在清点上重叠；重放后本票其余文件与分支 tip 逐字节一致。
- 评审：作者自审（见上条 Comments），非作者评审这道门本次没有过。
- 验证（推送方，钉合入候选 `547908ad` 的隔离 worktree，WSL，go1.26.8，DSN 为门禁库 55432）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退出 0；带 DSN 全仓 `go test -count=1 -p 1 ./...` 退 0，138 个包 ok、0 FAIL、0 cached，167 s；带 DSN `-v` 单跑 `TestTheLayeredReadAnswersEachTierPresenceOnItsOwn`、`TestTheLayeredReadIsBoundToItsTenant` 与 `cmd/parcel-api` 的 `TestTheWiredClaimsReadTheRuleAdoptedAtAcceptanceThroughParcelShipment` 均 PASS（非 SKIP）。00:53:45 推前查 `origin/main..547908ad`，只有本票这几笔，其下无他人提交。
- 解锁：[04](04-ve-claim-eligibility-checks-contract-and-inherits-rows-from-product-base.md) 的「Blocked by 03」已解；照 02 进 main 时的先例记在这里，04 票面未改。
- 分支 `mcp4-csr03` 本地改名 `merged/mcp4-csr03`、作者树已拆；远端分支照 `mcp5-csr02` 的先例留着。
