# 02 PC 闭包：客户服务规则合同优先解析，与结算政策同排第二段

Category: enhancement
Status: resolved——2026-10-10 23:1x 通道 1 重放进 main：`41c66134`…`a751d73b`，清点 `2243e878`；复评与进 main 记录见文末。此前 阻断已修，待 Spec 轴复评与重放——2026-10-10 评审 ← 通道 3（钉 `e2c65894`）Spec 轴阻断一条，通道 5 在同一分支修完并推 origin：代码 `2c386838`，注释 `ae6a87df`，票面是其后一笔；评审与修复见文末 Comments。进 main 时取本票的笔 `a041499b`、`8e859548`、`e03a58fd`、`2c386838`、`ae6a87df` 与两笔票面（`e2c65894` 与修复后这一笔）；认领笔 `9dc2997f` 只改本行。此前：完工，待评审与重放——2026-10-10 通道 5 在分支 `mcp5-csr02`（基 `1d67e27c`）上做完，分支已推 origin：代码 tip `8e859548`，含清点 tip `e03a58fd`。完成记录见文末。此前：in-progress——2026-10-10 通道 5 认领（单 task-b39b287a-4dfa-4c14-9786-0d43264c2aab）。此前：ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: 无
父票：[spec](../spec.md)
地盘：`internal/partycommercial/domain` 的闭包解析；碰 Go，走[并行会话](../../../docs/agents/parallel-sessions.md)那条路。
出处：[ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定一；spec「Implementation Decisions」解析段、「Testing Decisions」缝一。

## 做什么

1. 预重构先做：`resolutionOrder` 由「只挪结算政策」改为「依合同的第二段」一处声明；「同范围挂服务产品、锚点生效」的选法抽成一处，供 [03](03-pc-layered-read-port-returns-contract-and-product-base-bodies.md) 复用。
2. 闭包对客户服务规则：有壳 `references` 指名闭包已解出的客户合同的版本，就采纳它；没有就采纳挂服务产品的版本。同层多候选答`适用冲突`，两层皆零答`无适用依据`。闭包仍唯一采纳一版。
3. 只有挂产品一版的既有登记，行为不变；解析键登记面不加键。
4. 用例镜像 `settlement_basis_resolution_test` 一族。

## 判断项（拆票时裁定）

- **合同不在必需依据里时（Q1）**：合同被请求但冲突或无依据 → 前提未解；合同根本没被请求 → 只看产品层。理由：合同没解出时无从知道有没有挂合同版，回落产品版可能静默套上更宽的条款。
- **指名到哪一级（Q2）**：按对象一级对上，合同换版后同一份挂合同规则照旧适用；条款要随合同改，就发新一版规则。不给壳加版本格（ADR-0176 决定五「规范化零变化」）。
- **同一合同两版（Q4）**：只在闭包答`适用冲突`，发布面不加门——ADR-0176 没裁发布门，Alternatives 否的是保存面禁止产品版与合同版并存。spec 用户故事 4 的「并拒绝」读作解析时不采纳，不是发布时拒收。**越权风险点 · 待 PC owner 复核。**

## 完成判据

- [x] 合同优先、产品回落、同层多候选、两层皆零、合同被请求但未解出、合同未被请求六格各有用例。
- [x] 只有挂产品一版的既有登记，解析结果与改动前相同，有用例钉住。
- [x] 解析键登记面与 PCC-1 规范化无改动。

## 完成记录（2026-10-10，通道 5，分支 `mcp5-csr02`）

**落点**

| 笔 | 段 | 做了什么 |
|---|---|---|
| `9dc2997f` | 票面 | 认领：Status 转 in-progress |
| `a041499b` | 领域（预重构） | 闭包第二段改由 `resolvesAfterContract` 一处声明，`resolutionOrder` 与解析循环里「前提未解」那一格共读；`adoptedContractLabel` 改为 `adoptedContractVersion`，交回已采用的合同版本本身；成员按类别分派单依据入口。此笔仍只含结算政策，行为零变化，既有用例一条未改 |
| `8e859548` | 领域 + 用例 | `resolvesAfterContract` 加入客户服务规则；`resolveCustomerServiceRuleBasis`（合同层优先，零候选回落产品层）；单依据入口 `ResolveCommercialBasis` 对客户服务规则同一答法；选法一处 `customerServiceRuleTiers` + `soleCandidate`，导出 `CommercialRegistry.CustomerServiceRuleProductBase` 供 03 选底座；新用例文件 `customer_service_rule_basis_resolution_test.go` |
| `e03a58fd` | 清点 | 在 `8e859548` 干净检出重生成：`partycommercial` 测试文件 162 → 163，合计 1086 → 1087，其余各格零变化 |

**完成判据**

- ✅ 六格各有用例，都在 `internal/partycommercial/domain/customer_service_rule_basis_resolution_test.go`：
  - 合同优先：`TestTheClosureAdoptsTheServiceRuleNamingTheResolvedContract`，合同先声明、规则先声明两种次序都走。
  - 产品回落：`TestTheClosureFallsBackToTheProductTierWhenNoVersionNamesTheResolvedContract`，同范围另有挂别的合同的一版，它不被当合同层。
  - 同层多候选：合同层 `TestTwoVersionsNamingTheResolvedContractConflictWithoutFallingBack`（同一合同两版，冲突且不回落）；产品层 `TestTwoProductTierVersionsConflictWhenNoVersionNamesTheContract`（指名产品的一版加什么都没指名的一版）。
  - 两层皆零：`TestNeitherTierHoldingAVersionIsNoApplicableBasis`，报在 unresolved，不在前提未解。
  - 合同被请求但未解出：`TestAnUnresolvedContractLeavesTheServiceRuleUnasked`，合同无依据、合同冲突两格，客户服务规则都记前提未解。
  - 合同未被请求：`TestWithoutTheContractInTheClosureOnlyTheProductTierIsAsked`，产品版在场采纳产品版；只有合同版答`无适用依据`。
  - 另两条：单依据入口 `TestASingleBasisServiceRuleResolutionOnlyAsksTheProductTier`；03 要复用的选法 `TestTheProductBaseIsTheTierTheClosureFallsBackTo`（挂合同的不算底座、零版与两版、声明的服务产品收窄、别的范围 / 别的租户 / 锚点不在区间内、立不住的问法）。
- ✅ 既有挂产品一版行为不变：`TestAnExistingProductOnlyRegistrationResolvesAsBefore`，壳指名服务产品、壳什么都没指名两种在册形态 × 两种声明次序，均唯一采纳，解析标识都是 `CLO-d89b23824ea8f1bd`。这个值是在改动前的代码上（钉 `9dc2997f`，代码与基 `1d67e27c` 相同）对同一夹具实算的。真库侧：`cmd/parcel-api` 的 `TestTheWiredClaimsReadTheRuleAdoptedAtAcceptanceThroughParcelShipment`（壳什么都没指名的 `SYN-CSR-1` 经 PC 真解析器解出并固定闭包）带 DSN、`-v` 下为 PASS。
- ✅ 解析键登记面与 PCC-1 规范化无改动：`git diff --stat 1d67e27c 8e859548` 除认领笔改的本票面 Status 行之外，只有 `internal/partycommercial/domain/` 下的 `reference_closure.go`、`commercial_resolution.go` 与新用例文件。PS 解析键登记面（`internal/parcelshipment/adapters/partycommercial`）、`publication_canonicalization*.go`、ports、postgres、http、迁移都没动。

**red 与判别力**

- red（钉 `9dc2997f`，即改动前的代码，新用例文件未跟踪）：六条红（原写五条，漏计单依据入口那条；评审 ← 通道 3 把新用例 overlay 到 `1d67e27c` 上量得 6 红 3 绿）。合同优先两种次序都答 `APPLICABILITY_CONFLICT`；产品回落答 `APPLICABILITY_CONFLICT`；两层皆零答 `RESOLUTION_PENDING`（`NAMED_REFERENCE_NOT_CONFIRMED`：挂别的合同的那一版先被采纳，再在指名核对处停住）；合同未解出两格的前提未解都为空；合同未请求两格分别答冲突与唯一解析；单依据入口 `TestASingleBasisServiceRuleResolutionOnlyAsksTheProductTier` 红。三条绿：两条同层冲突（改动前本来就答冲突，它们拦的是「冲突时回落」那种写法），以及既有登记的标识钉。
- 临时变异八处，证完都从备份还原并核过哈希，未提交。每一处都被拿住：
  - M1 合同层收任何挂合同的版本：产品回落、两层皆零红。
  - M2 合同层冲突时回落：合同层同层冲突红。
  - M3 客户服务规则不走前提：合同优先两种次序、合同层冲突、合同未解出两格红。
  - M4 排序时不把客户服务规则挪进第二段：规则先声明那次、既有登记标识钉的反序那次红。
  - M5 挂别的合同的版本进产品层：产品回落、两层皆零、合同未请求两格、单依据入口、底座两格红。
  - M6 什么都没指名的壳移出产品层：产品层同层冲突、既有登记「壳什么都没指名」、底座「两版是冲突」红。
  - M7 单依据入口不分层：单依据用例红。
  - M8 底座丢掉声明的服务产品：底座「声明的服务产品收窄底座」红。

**门**（钉 `8e859548` 的干净分离检出，WSL，go1.26.8，DSN 为门禁库 55432）：`go build ./...`、`go vet ./...` 退 0；改动的 `.go` `gofmt -l` 无输出，无 CR、无 BOM。`go list` 反查 `partycommercial/domain` 的反向依赖（生产依赖 ∪ 测试二进制依赖）共 21 个包，含 `cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-dispatch`；连同 `./internal/architecture/...` 以 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable go test -count=1 -p 1` 跑，退 0，各包都是 ok。`-v` 单跑上面那条 `cmd/parcel-api` 真库用例，是 PASS 不是 SKIP。全量由推送方跑。

**判断项**

1. **产品层按「壳上没指名任何客户合同」认，什么都没指名的壳归产品层。** ADR-0176 只写「挂服务产品的候选」，没说壳上什么都没指名的版本归哪层。解析只看壳，壳空的版本正文挂产品还是挂合同它看不见；而 `ConsistentCustomerServiceRuleApplicability` 对没指名的壳放行，这一形态在册。改按「壳指名服务产品」认，壳空而正文挂产品的既有登记会从唯一解析变成`无适用依据`，与本票「只有挂产品一版的既有登记，行为不变」相违；`cmd/parcel-api` 的真库用例（`SYN-CSR-1` 壳空、正文挂合同 `SYN-CONTRACT-1`、期望被采纳）也会红。代价：壳空而正文挂合同的版本会被当成产品层，可能回落给别的合同，也可能被 03 选作底座。前者由 04 的「正文挂合同 = 查询携带的合同」点核拦下，但拦在冻结闭包的索赔那一刻：该委托的索赔会一直报错，而按「壳指名服务产品」认的另一选法在受理时就答`无适用依据`。后者要不要在 03 / 04 核「底座正文挂的是产品」，留给那两张票。另一条代价：壳空而正文挂合同的一版与产品版同范围并存时，两版同落产品层，仍答`适用冲突`——ADR-0176 要消掉的正是这一形态的冲突，对它不达；`SYN-CSR-1` 即此形态，06 留意；解法是壳指名合同。（两条代价由评审 ← 通道 3 Spec ① 补记，选法不变。）**越权风险点 · 待 PC owner 复核。**
2. **挂别的合同的版本两层都不进。** 范围里只有它时答`无适用依据`；改动前它会被采纳，再在指名核对处停成`解析未决`（`NAMED_REFERENCE_NOT_CONFIRMED`）。对这一户来说，`无适用依据`对应的恢复动作（去登一份）才对。
3. **合同按对象认、不按版本**（拆票裁定「指名到哪一级」）：壳 `references` 本来就只到对象一级，没有加版本格。
4. **同一合同两版只在闭包答`适用冲突`**（拆票裁定）：发布面未动。
5. **合同请求了却没解出记前提未解，没请求只看产品层**（拆票裁定）：由 `resolvesAfterContract(kind) && key.requires(CustomerContractObject)` 一处判。结算政策那一侧行为不变，它的键本来就必须请求合同。
6. **合同作参数交给客户服务规则那一支，不进解析键。** 闭包直接调 `resolveCustomerServiceRuleBasis`，不经 `ResolveCommercialBasis`：闭包入口已经判过锚点与权威可读，单依据键的最小身份由闭包键的最小身份推得出。
7. **单依据入口 `ResolveCommercialBasis` 对客户服务规则改为只看产品层。** 单依据键上没有合同那一维，与闭包不请求合同时同一答法；不分层就是第二套口径。它的非测试调用点只有闭包（实测于 `1d67e27c`，本票未新增调用点）。
8. **`Adopted()` 的成员次序**：规则先于合同声明的闭包，现在规则排在合同之后（第二段）。解析标识不随次序变，已由既有登记用例的反序那次钉住。——**评审阻断，已修**：快照按成员次序写下，这一换序让改动前已固定的解析重解即撞`内容冲突`；`2c386838` 把成员次序改回声明次序，见文末「阻断修复记录」。
9. **留给 03 的接口**：`CommercialRegistry.CustomerServiceRuleProductBase(tenant, scope, anchor, declaredProduct)` 交回 `(CommercialVersion, ResolutionOutcome)`，与闭包回落层共用 `customerServiceRuleTiers` 与 `soleCandidate`。做成方法、只用既有类型：本笔它还没有生产调用点（由 03 接），函数名棘轮只扫顶层函数，类型可达性棘轮只看导出类型，本笔没有新类型，`internal/architecture` 绿。`declaredProduct` 可缺席；闭包键带声明的服务产品时（PS `requested_service_product_keys.go` 会带），回落层照旧经 `admitsDeclaredServiceProduct` 收窄，03 若要与闭包同口径，应传同一个产品。
10. **`CandidateCount()` 报作答那一层的候选数**，不是两层之和。

**未做 / 风险**

- 03–06 一件未做（各自另派）；演示种子未动。
- 判断项 1 的代价要不要在 03 / 04 加一道「底座正文挂产品」的核，待 PC owner 复核时一并定。

## Comments

### 评审 ← 通道 3 · 钉 `e2c65894` · 22:0x

（task-1b6c424e，非作者评审；通道 1 派修复单 task-0f877692 时转来，作者照录。）Standards 0 阻断 / 1 非阻断；Spec 1 阻断 / 2 非阻断。

- **阻断（Spec）**：`reference_closure.go` `resolvesAfterContract` 把规则挪进第二段后，规则先于其他成员声明时 `Adopted()` 换序而解析标识不变（探针：基线与 tip 同为 `CLO-d89b23824ea8f1bd`，adopted 由 `[CUSTOMER_SERVICE_RULE, CUSTOMER_CONTRACT]` 变 `[CUSTOMER_CONTRACT, CUSTOMER_SERVICE_RULE]`）。`postgres/commercial_resolution.go` `documentOfClosure` 按此序写快照，`CommercialResolutions.Save` 撞键时比 `content_digest`；应用层 `ResolveCommercialBasisHandler.fix` 每次唯一解析都写，故改动前已固定的解析重解一次就答 `ResolutionContentConflict`，PS `commercial_basis.go` 落 `PC-RESOLUTION_CONTENT_CONFLICT` 无法判定。判断项 8 只核了标识，「既有登记行为不变」在落库层不成立。评审给两条路：保旧成员次序，或写明取舍并实测无此类登记。**通道 1 裁定走保旧成员次序**：「既有登记行为不变」是本票完成判据；本仓卖的是产品，「实测无此类登记」只测得到演示租户，测不到升级后各租户的库。
- **非阻断（Standards）**：`commercial_resolution.go` `CustomerServiceRuleProductBase` 头注「两处读同一份分层」是计数，改成「闭包回落与层次读口读同一份分层」。
- **非阻断（Spec ①）**：判断项 1（空壳归产品层）的代价补两条——04 的点核要到冻结闭包的索赔时才拦，该委托的索赔会一直报错（另一选法在受理时就答`无适用依据`）；空壳挂合同版与产品版同范围仍`适用冲突`，ADR 目标对这一形态不达（`SYN-CSR-1` 即此形态，06 留意），解法是壳指名合同。只补代价，不改选法，越权风险点标记照旧待 PC owner。
- **非阻断（Spec ②，供 03 引用）**：03 复用 `CommercialRegistry.CustomerServiceRuleProductBase` 时，`declaredProduct` 须取自冻结闭包键（快照里有 `serviceProduct`；spec 读口签名缺这一维，不传则同范围两个产品版答冲突）；未决不带原因（锚点缺与权威读不到压成一格）。只记在这里，不改 03 票面、不改 spec。
- **无发现**：注释中文且只写取舍、无跨文件行号、领域包无新 import、术语用原词、无租户取值写死；六格与既有登记均有用例，overlay 把新用例放到 `1d67e27c` 上得 6 红 3 绿（完成记录「五条红」漏计单依据那条），②属票面允许的变化，`a041499b` 逐行等价，未做票面外的事。

### 阻断修复记录（2026-10-10 22:3x，通道 5，task-0f877692，同一分支 `mcp5-csr02`，基不动、未 rebase）

| 笔 | 做了什么 |
|---|---|
| `2c386838` | `reference_closure.go`：`memberOrder`（成员次序：声明次序，只把结算政策排最后）与 `inMemberOrder`；唯一解析时 `closure.adopted` 排回成员次序，`resolutionOrder` 只管解析先后。用例：postgres 适配器新增 `TestAResolutionFixedWithTheServiceRuleDeclaredFirstReplaysAsRecorded`；领域 `TestAnExistingProductOnlyRegistrationResolvesAsBefore` 加成员次序断言 |
| `ae6a87df` | Standards 非阻断：`CustomerServiceRuleProductBase` 头注去计数 |
| 本笔 | 票面：评审记录、本修复记录、判断项 1 补代价、判断项 8 指向本节、「五条红」更正、Status |

**red → green**

- 新用例的做法：夹具是规则先于合同声明、壳什么都没指名的产品层一版。「改动前已固定的那一行」经重建门按声明次序造出并固定，它的 `content_digest` 钉成在 `1d67e27c` 上实算的 `05d151b57978ad040ea5389042ba648c7ca3ca3a521f800bf28b72ed2769ccbe`；再把当前代码重解出的闭包固定一次，要求答 `ALREADY_RECORDED`。同一用例在 `1d67e27c` 上 PASS（带 DSN，非 SKIP）。
- red（钉 `e2c65894`，代码未改、只加用例）：postgres 用例报 `重解后再固定 = CONTENT_CONFLICT, want ALREADY_RECORDED（重解的成员次序 [CUSTOMER_CONTRACT CUSTOMER_SERVICE_RULE]）`；领域用例两格各报 `bases [CUSTOMER_SERVICE_RULE CUSTOMER_CONTRACT]: 成员次序 = [CUSTOMER_CONTRACT CUSTOMER_SERVICE_RULE], want [CUSTOMER_SERVICE_RULE CUSTOMER_CONTRACT]（改动前按声明次序）`。钉住的摘要那一处断言在 tip 上也过，红在重解那一步。
- green（`2c386838`）：两条 `-v` 都是 PASS，postgres 那条非 SKIP。

**判断项**

1. **成员次序取「声明次序、结算政策排最后」**，即 `1d67e27c` 上 `resolutionOrder` 的输出：既有快照都是按它写下的。`memberOrder` 刻意不读 `resolvesAfterContract`：第二段再加一类时成员次序若跟着变，同一个阻断会再来一次。
2. **只排 `Adopted()`，未决三张清单的次序不动。** 非唯一解析不落库（`ResolveCommercialBasisHandler.fix` 只写有标识的结局）；`Adopted()` 的非测试读者只有 postgres 适配器的 `documentOfClosure`（实测于 `e2c65894`，`internal/` 与 `cmd/` 的非测试代码）。
3. **快照形状、解析标识算法与已接受 ADR 的决定都没动。** `closureIdentity` 本就先排序再散列；`ValidateClosureBeforeDecision` 只比标识，不受次序影响。
4. **用例落在 `internal/partycommercial/adapters/postgres` 的测试文件**，越出本票地盘所列的 `domain`：派单点名「能带 DSN 走 postgres 适配器更好」。生产代码仍只改 `domain`。

**门**（WSL，go1.26.8，DSN 为门禁库 55432；在提交前的工作副本上跑，内容即 `ae6a87df`，提交后 `git status` 为空）：改动的 `.go` `gofmt -l` 无输出，无 CR、无 BOM；`go build ./...` 退 0；`go vet ./internal/partycommercial/...` 退 0。`go list` 反查 `internal/partycommercial/...` 的反向依赖（生产依赖 ∪ 测试二进制依赖，去掉本上下文自己的包）得 14 个，连同 `./internal/partycommercial/...` 与 `./internal/architecture/...` 以 `go test -count=1 -p 1` 带 DSN 跑：退 0，21 ok / 0 FAIL / 1 无测试，55 s。`-v` 单跑 `cmd/parcel-api` 的 `TestTheWiredClaimsReadTheRuleAdoptedAtAcceptanceThroughParcelShipment`：PASS，非 SKIP。机制清点在 `ae6a87df` 的干净树上重生成，与已提交文件逐字节相同（只加了用例函数，没加文件）。全量留给推送方重放后那一跑。

**非阻断处置**：Standards 那条已改（`ae6a87df`）；Spec ① 已补进判断项 1；Spec ② 记在上一条评审里供 03 引用，未改 03 票面与 spec。

### 复评 ← 通道 2 · 钉 `76120b55` · 23:0x（`task-9de81782`；Spec 轴，非作者）

阻断无，非阻断一条，其余无发现。

- 非阻断（Spec）：`reference_closure.go` 的 `memberOrder` 里「结算政策排最后」那半没有用例守——`/tmp` 副本变异（令结算政策留在声明位置）后，
  结算政策不在最后的几种声明次序下 `Adopted()` 换序而标识不变，正是阻断那一类快照摘要漂移，而 `./internal/partycommercial/...` 带 DSN 全绿；
  两条新用例都不带结算政策。生产代码现在是对的，缺的是护栏：可在 `TestAnExistingProductOnlyRegistrationResolvesAsBefore` 的成员次序断言里
  补一格「结算政策先于合同声明」。
- 无发现：阻断真解——overlay 探针用两版共有的夹具，覆盖规则先 / 后于合同、结算政策在首 / 中 / 末、带受理规则包、只挂产品、只有规则等声明次序，
  `1d67e27c` 与 `76120b55` 的结局、解析标识、`Adopted()` 成员与次序逐行相同；判别力——两条 red 用例在 `e2c65894` 的生产代码上都红、在
  `76120b55` 上都 PASS（postgres 那条非 SKIP），postgres 那条在 `1d67e27c` 上也 PASS（钉住的摘要确是旧代码写下的值）；ADR-0176 的解析语义未动
  （`resolutionOrder` 与解析循环未改，修复只在唯一解析时排 `closure.adopted`）；未越出裁定与票面；red 用例落在 adapters/postgres 测试文件可接受
  （阻断就在落库层，生产代码只改 domain）；Standards 简核无发现。

## 进 main 记录（通道 1 · 2026-10-10 23:1x）

- 远端 main `5e6cd4b6` → `2243e878`（本记录随其后一笔）：分支 `mcp5-csr02` 的 `9dc2997f`、`a041499b`、`8e859548`、`e2c65894`、`2c386838`、
  `ae6a87df`、`76120b55` cherry-pick 重放到 tip，SHA 换了，在 main 上依次是 `41c66134`、`5361436f`、`171cda24`、`83905878`、`78bc40f5`、`36f3d3c4`、
  `a751d73b`。清点笔 `e03a58fd` 不重放——main 上的清点自基 `1d67e27c` 起已被别的票改过——在重放 tip 上重生成为 `2243e878`。重放前核过
  `1d67e27c..5e6cd4b6` 与本票文件只在清点上重叠；重放后本票其余文件与分支 tip 逐字节一致。
- 评审：通道 3 `task-1b6c424e`（钉 `e2c65894`）Spec 阻断一条、非阻断三条；通道 5 在同一分支修复（`task-0f877692`）；通道 2 复评 `task-9de81782`
  （钉 `76120b55`）无阻断，新提的非阻断一条（「结算政策排最后」那半缺用例）随票记、未改，宜随 03 或另立小票补。
- 验证（推送方，钉合入候选 `2243e878`）：`go build ./...`、`go vet ./...` 退出 0；带 DSN 全仓 `go test -count=1 -p 1 ./...` 138 个包 ok、0 FAIL；
  带 DSN `-v` 单跑 `TestAResolutionFixedWithTheServiceRuleDeclaredFirstReplaysAsRecorded` 与 `cmd/parcel-api` 的
  `TestTheWiredClaimsReadTheRuleAdoptedAtAcceptanceThroughParcelShipment` 均 PASS（非 SKIP）。
- 解锁：[03](03-pc-layered-read-port-returns-contract-and-product-base-bodies.md) 的「Blocked by 02」已解；评审记下供 03 引用的两条
  （`CustomerServiceRuleProductBase` 的 declaredProduct 须取自冻结闭包键、未决不带原因）见 Comments 里通道 3 那条。
