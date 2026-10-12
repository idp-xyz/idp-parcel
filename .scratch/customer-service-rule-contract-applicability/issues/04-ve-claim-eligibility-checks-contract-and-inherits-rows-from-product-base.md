# 04 VE 索赔资格：采纳合同版时按合同点核、按行继承产品底座

Category: enhancement
Status: 完工，待评审与重放——2026-10-12 通道 1 在分支 `mcp1-csr04`（基 `2603f9ad`）上做完，分支已推 origin：代码 `f66e18da`、`c2b8084f`，清点 tip `fb4e8ce5`；完成记录见文末。此前：in-progress——2026-10-12 10:1x 通道 1 认领（用户经 IDP 队列令「继续后续的工作」；其余通道自 10-11 起无活动、不在等待态，未派单），分支 `mcp1-csr04`，基 `2603f9ad`。此前：ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: [03](03-pc-layered-read-port-returns-contract-and-product-base-bodies.md)
父票：[spec](../spec.md)
地盘：`internal/visibilityexception/adapters/partycommercial` 与 `cmd/parcel-api` 的索赔装配；碰 Go，走并行会话那条路。
出处：[ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定二、四；spec「Testing Decisions」缝二。

## 做什么

1. 闭包采纳挂合同版时，VE 适配器经 03 取两份正文：正文挂的合同 ≠ 查询携带的合同 → `ErrUntranslatableAnswer`。
2. 按行拼：合同版有行用合同版，无行取底座版该行，两层皆无答既有「未登记」。
3. 闭包采纳挂产品版时照旧走单版点读，不核。
4. `cmd/parcel-api` 的索赔装配接新读口；`claim_service_rules_resolution_test` 加三族。

## 判断项（拆票时裁定）

- **读时底座多候选落哪一格（Q5）**：落与「闭包对客户服务规则答`适用冲突`」同一既有格——恢复动作相同，都是运营更正登记（ADR-0029 看恢复动作），不新立格。实现时核一次，写进完成记录。
- **底座不随闭包冻结（Q5）**：底座是读时按锚点选的，已接受委托的底座只靠「锚点固定 + 版本不回溯生效」保持不变；有效性更正（ADR-0038）改了底座区间时会改口。这是 ADR-0176 决定二的已知后果，不当缺陷。

## 完成判据

- [x] 点核：挂合同 ≠ 查询合同答 `ErrUntranslatableAnswer`，有用例。
- [x] 按行继承三格（合同有行、无行取底座、两层皆无答未登记）各有用例；挂产品版行为不变。
- [x] `cmd/parcel-api` 装配有带 DSN 的用例。

## 完成记录（2026-10-12，通道 1，分支 `mcp1-csr04`）

**落点**

| 笔 | 段 | 做了什么 |
|---|---|---|
| `2416c8cd` | 票面 | 认领：Status 转 in-progress |
| `f66e18da` | VE 适配器 | `ClaimServiceRules` 第三段分两条路：闭包采纳那一版的壳指名客户合同 → `contractTierDimensions` 经 `CustomerServiceRuleLayerView` 取两层、`translateRule(合同版, 底座)` 按行拼；其余照旧单版点读。新 `tiedToAdoptedContract`（ADR-0176 决定四）、`productBase`（底座各格分派）、`adoptedRule`（第二段交第三段：规则版本 + 闭包采用的客户合同 + 闭包）。`ClaimServiceRulesDeps` 加 `Layers`，缺即 `ErrNilDependency`。新用例文件 `claim_service_rules_contract_tier_test.go`，既有两份夹具补 `Layers` 与缺件格 |
| `c2b8084f` | `cmd/parcel-api` | `buildClaimEligibilityRules` 接 `pcapplication.NewCustomerServiceRuleLayerReader(CommercialAuthority(CommercialPublications), CustomerServiceRuleContents)`；新真库用例文件 `assemble_claims_contract_tier_test.go` |
| `fb4e8ce5` | 清点 | 在 `c2b8084f` 的干净树（`git status --short --ignored` 为空）重生成：visibilityexception 测试 96 → 97、`cmd/` 测试 111 → 112、端口基线口径缺 31 → 30（层次读口有了生产消费方）；其余各格零变化 |

**完成判据**

- ✅ 点核：`TestABodyTiedToAnotherContractIsUntranslatable` 三格——合同版正文、底座正文、产品层那一版正文各挂 `contract-2`，闭包采用 `contract-1`，都答 `ErrUntranslatableAnswer`。
- ✅ 按行继承：`TestAContractTierRuleTakesItsOwnRowsAndInheritsTheRestFromTheProductBase`（合同版有期限行、无材料行 → 期限冻合同版、材料继承并冻底座；层次读口问法是查询租户、合同版自己的范围、闭包采纳的那一版、闭包锚点与闭包键上的产品，且不走单版点读）、`TestAContractTierRowWinsOverTheProductBaseRow`（两层都有 → 取合同版）、`TestARowAbsentFromBothTiersIsUnregistered`（两层都无、无底座、底座正文未登三格 → 未登记，合同版写了的一维照答）。挂产品版行为不变：`TestAProductTierRuleIsStillReadAloneAndOnlyAContractBodyIsChecked` 两格不问层次读口，既有两份用例文件全绿。
- ✅ 装配带 DSN：`TestTheWiredClaimsInheritTheRowsAContractTierRuleLeavesToTheProductBase`——同范围并存壳指名 `SYN-CONTRACT-1` 的合同版 `SYN-CSR-CONTRACT-1`（正文只登首次索赔期限、起算 `SYN-EVENT-HANDED-OVER`）与壳空、正文挂服务产品的 `SYN-CSR-1`（两行都登），PS 解析键真登记、PC 真解析器合同优先采纳合同版、委托真接受，读回期限冻 `SYN-TENANT-1/SYN-CSR-CONTRACT-1/v1`、材料冻 `SYN-TENANT-1/SYN-CSR-1/v1`。`-v` 下 PASS 非 SKIP，既有 `TestTheWiredClaimsReadTheRuleAdoptedAtAcceptanceThroughParcelShipment`（`SYN-CSR-1` 壳空、正文挂 `SYN-CONTRACT-1`）照绿。

**red 与判别力**

- red：用例与 `Layers` 依赖先就位、实现未动，新用例全部断言失败（无 SKIP、无编译错），既有用例全绿。
- 临时变异八处，经 `go test -overlay` 换掉 `claim_service_rules.go`，工作副本未动；每处都被拿住：M1 合同版不走层次读口（适配器六条 + 装配用例红）、M2 不继承底座（继承用例 + 装配用例红）、M3 点核恒过（点核三格红）、M4 底座冲突当没有底座（冲突格红）、M5 不带闭包锚点（问法用例 + 装配用例红）、M6 合同版正文未登时整版拿底座顶（该格红）、M7 底座未决当没有底座（未决格红；首次写法撞了 `duplicate case` 编不过，改写后重跑）、M8 继承来的一维冻合同版的引用（继承用例 + 装配用例红）。
- 没钉住的：层次读口收到的 `declaredProduct` 只在零值形态下核过（适配器夹具的闭包键不带服务产品）；带产品的收窄由 03 的 `TestTheDeclaredServiceProductNarrowsTheProductBaseAsTheClosureDoes` 守在 PC 一侧。

**门**（钉 `c2b8084f` 的工作树，`git status --short --ignored` 为空；`fb4e8ce5` 只改清点 `.md`，代码相同。WSL，go1.26.8，DSN 为门禁库 55432）

- 改动的 `.go` 都 `gofmt -l` 无输出，无 CR、无 BOM。
- `go build ./...`、`go vet ./...` 退 0。
- `go list -test` 反查 VE 适配器包的反向依赖只有 `cmd/parcel-api`；两包连同 `./internal/architecture/...` 带 DSN `go test -count=1 -p 1`：3 ok / 0 FAIL。
- 全量留给推送方重放后那一跑。证据层级 S。

**判断项**

1. **「查询携带的合同」读作闭包采用的客户合同版本的对象标识，不是 `EligibilityQuery.Contract`。** 后者是 VE 的合同责任范围引用（如 `contract-1/liability`），与 PC 合同对象不是一套词；ADR-0136 决定五写明两者不核一致，本票不改它。VE 手上与 PC 正文可比的合同只有冻结闭包采用的那一份。
2. **合同层按壳认，与 PC 闭包分层同一（02 判断项 1）。** 壳指名客户合同才走层次读口；壳空而正文挂合同的那一版（`SYN-CSR-1` 形态）在闭包里是产品层，这里也走单版点读。按正文分会让同一版在两处两种归法。
3. **点核施于每一份挂合同的正文，含产品层那一版，挂产品的正文不核。** 票面「做什么 3」写「挂产品版时照旧走单版点读，不核」，ADR-0176 决定四写「挂产品的版本不核（查询无产品维）」——两句说的都是正文挂产品；壳空而正文挂合同的那一版正文挂的是合同，02 判断项 1 已写明它回落给别的合同时「由 04 的点核拦下」。底座也核，是 03 判断项 6 留给本票的那一格：在消费侧核，PC 层次读口不改，答法与它在闭包回落路上一致（`ErrUntranslatableAnswer`）。**越权风险点 · 与 02 判断项 1、03 判断项 6 一并待 PC owner 复核。**
4. **合同版正文未登 → 两维未登记，不整版继承底座。** 03 判断项 8 把「怎么用」留给本票。合同版可能收紧的条款没登上来时拿产品标准代答，会把一次漏登变成一次放宽；与单版点读「正文未登」同格，恢复动作相同（去 PC 登合同版正文）。
5. **底座各格（Q5 实现时核一次）。** `适用冲突` → `ErrUntranslatableAnswer`：核过适配器第二段，闭包结局不是`唯一解析`时答的正是这一格，恢复动作同为运营更正登记，不新立格。`解析未决` → 不带哨兵的 error：冻结闭包的锚点立得住，剩下的成因只有权威读不到，是依赖调不通，不折成未登记。`无适用依据`与「选出了而正文未登」→ 没有底座，合同版没写的行未登记。底座的问题无论合同版两行写没写全都先报——底座坏了是登记坏了，不因这一次恰好用不上就不报；代价是合同版写全两行的户也会被一版坏底座挡住。
6. **继承来的一维 `RuleVersion` 冻底座版本引用。** 资格判断事后要查得到那一行出自哪一版；冻合同版会让继承来的条款看起来像合同定制。
7. **底座不随闭包冻结（Q5 第二条）照票面接受**，不在本票补。

**未做 / 风险**

- 05（管理台表单）、06（演示对比面）未做；演示种子未动。
- 判断项 3 待 PC owner 复核。
