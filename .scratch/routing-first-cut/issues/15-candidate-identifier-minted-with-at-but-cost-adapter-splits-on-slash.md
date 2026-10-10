# 15 候选标识「线路@版本」与成本适配器按「/」切不一致：计价输入一接上，初始路由就落成本来源不可用

Category: bug
Status: in-progress——**评审无阻断，非阻断两条已改，待增量复评与重放**（2026-10-10 21:1x 通道 1；代码 tip `7cab7369`；评审与处置见 Comments）。此前：in-progress——**完工，待评审与重放**（2026-10-10 20:5x 通道 1，新会话接续；分支 `mcp1-rfc15`，代码 tip `2301cd5d`，基 `1d67e27c`）；完成记录见文末。此前：in-progress——2026-10-10 20:3x 通道 1 认领（认领笔 `8bccdd5c` 提交于 20:31，原写 21:0x 是笔误；用户令通道 1 自己完成：通道 3 在派单前已 crash，`task-33e8ef5a` 未执行）；分支 `mcp1-rfc15`，基 `1d67e27c`，隔离工作树 `/home/tops/workspace/idp-parcel-mcp1-rfc15`。此前：ready-for-agent——2026-10-10 通道 1 立（用户授权自决），出自 [11](11-demo-network-adopted-as-reference-configuration.md) 完工报里通道 2 的探针实测（探针未入库）
Blocked by: 无逻辑依赖——铸标识的一侧随 [09](09-initial-route-evidence-folded-from-catalog.md)、切标识的一侧随 [10](10-candidate-cost-from-leg-buy-evaluations.md)，两侧在 main 上都已存在
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为它是 09 与 10 之间的缝。
地盘：`internal/networkrouting` 里候选标识的铸造与解析两侧及其用例。

## 现象

- 目录折叠给候选铸的标识形如 `线路@版本`（`catalog_network_evidence.go` 的 `versionReference`）。
- `RouteCandidateCostAdapter.resolveLegs` 按 `/` 切这个标识找线路。
- 通道 2 用探针把计价输入接上后实测：`untranslatable pricing evaluation: candidate "SYN-LINE-CN-SG-01@1" carries no line reference`，初始路由落 `COST_SOURCE_UNAVAILABLE`。
- 今天碰不到，是因为 dispatch 的计价输入仍是 `unconfiguredRoutePricingInput{}`，初始路由先停在 `COST_SOURCE_NOT_CONFIGURED`；10 的用例手造 `线路/1`，没经过真正的铸造路径。能编过、语义不对，测试全绿。

## 做什么

1. 候选标识的格式只在一处定义：铸与解共用同一个类型或同一对函数，不再各自约定分隔符。改哪一侧、用字符串还是结构化引用，按 NR 现有约定定，写进判断项。
2. 补一条经真实铸造路径的用例：目录折叠铸出的候选直接交成本适配器，能解出线路。放回旧写法（一侧 `@`、一侧 `/`）时它必须红。

## 不做

- 不接计价输入本身，那是 [17](17-initial-route-pricing-input-from-customer-declaration.md)。

## 完成判据

- [x] 格式一处定义，两侧共用。——`domain.NewLineCandidateID` 铸、`domain.CandidateID.LineVersion` 解；目录折叠经前者、`RouteCandidateCostAdapter.resolveLegs`
  经后者；非测试代码里没有别的读者按分隔符切候选标识（`git grep`，钉 `2301cd5d`）。
- [x] 经真实铸造路径的用例绿；放回旧写法红，写明怎么证的。——`cmd/parcel-dispatch` 的
  `TestTheCostSideResolvesTheLineOfACandidateMintedByCatalogFolding` 真库 PASS；放回旧写法即变异 M1，见完成记录。

## 完成记录（通道 1 · 2026-10-10 20:5x · 代码 tip `2301cd5d`，基 `1d67e27c`）

**各笔**：`8bccdd5c` 认领；`2301cd5d` 修复与用例。

**判断项**

1. 格式留在领域、`CandidateID` 旁一对函数：`NewLineCandidateID(lineCode, version)` 铸、`CandidateID.LineVersion()` 解，分隔符只在
   `reachability.go` 一处。没改成结构化引用：`CandidateID` 是进判断留痕、进库的不透明值，改形状要动留痕与迁移，超出本票。分隔符取 `@`，
   因为铸造侧本来就这么铸，已落库的留痕不变——改的是读的那一侧。
2. `LineVersion` 取最后一个 `@` 之后为版本，线路编码里即便含 `@` 也解得回；铸造时不拒含 `@` 的线路编码，与原先 `versionReference`
   一致。不是首版形态铸的标识答 ok 为假，取数侧照原来的失败格答 `carries no line reference`，答复格不变。
3. `versionReference` 不动：它还给策略、区域等别的引用铸版本引用，那些不是候选标识。
4. 真实铸造路径的用例放在 `cmd/parcel-dispatch`：组合根本来就同时装配两侧；放进适配器包要让 `adapters` 的用例导入 `application`。
   计价输入用已配置的替身，读逐段依据那一步截住——本票不接计价输入（[17](17-initial-route-pricing-input-from-customer-declaration.md)），
   截住之后的评价也不在本票。适配器与 application 两侧各自的用例另经同一对函数取齐，适配器用例不再手造 `线路/1`。

**判别力**（临时变异，证完 `git checkout` 还原，未提交；钉 `2301cd5d`）

- M1 放回旧写法（铸造侧照旧 `线路@版本`，取数侧按 `/` 切）：`adapters/parcelpricing` 7 条用例红；
  `TestTheCostSideResolvesTheLineOfACandidateMintedByCatalogFolding` 红，答 `candidate "SYN-LINE-CN-SG-01@1" carries no line reference`。
- M2 铸造侧改按 `线路/版本` 铸：`application` 3 条红（含 `LineVersion` 解回断言）；`cmd/parcel-dispatch` 两条红（候选标识断言；真实铸造
  路径用例答 `"SYN-LINE-CN-SG-01/1" carries no line reference`）；适配器包绿——它的用例也经 `NewLineCandidateID` 铸，符合预期。

**验证**（钉 `2301cd5d`，隔离工作树无未提交）

- `go build ./...`、`go vet ./...` 退出 0；本票改动的 `.go` 文件 `gofmt -l` 零行。
- `go test -count=1 -p 1`，带 `IDP_PARCEL_POSTGRES_DSN`：`./internal/networkrouting/...`、`./cmd/...`、`./internal/architecture/...`，加 `go list`
  反查出的反向依赖 `internal/{parcelshipment,transportfulfillment,visibilityexception}/adapters/networkrouting/...`——30 个包 ok、0 FAIL；
  `-v` 单跑 `TestTheCostSideResolvesTheLineOfACandidateMintedByCatalogFolding` 为 PASS，不是 SKIP。
- 没动端口、端点、迁移，未重生成清点。

**未验**：全仓 `go test ./...`，留推送方重放后那一跑。

## Comments

### 评审 ← 通道 3 · 钉 `a44941d2` · 21:1x（`task-1213993e`）

两轴均无阻断，各一条非阻断（全文在任务台账 `task-1213993e`）：

- Standards：`NewLineCandidateID` 与 `CandidateID.LineVersion` 没照 NR 先例 `ParsePlannedLegReference` 守往返——`LineVersion` 收 `X@+1`、`X@01`，
  `NewLineCandidateID("", 1)` 铸出 `@1` 却解不回；生产走不到（登记口拒空编码与版本小于 1）。
- Spec：`foldPathExecutability` 以 `"LINE/"+candidate.id.String()` 拼班期版本引用，是第三个按内容用候选标识的地方，判据一的凭据没列；
  同族 `NODE/`、`CONNECTION/` 走 `versionReference`。

### 非阻断两条的处置 ← 通道 1 · 21:1x（代码 tip `7cab7369`）

两条都在本分支改，不另立票：

- `NewLineCandidateID` 线路编码空白或版本非正即拒（`ErrInvalidLineCandidate`）；`LineVersion` 的版本段只认 `NewLineCandidateID` 写得出的正整数，
  线路编码空白答 ok 为假——与 `ParsePlannedLegReference` 同一守法。用例：非首版形态清单补 `@01`、`@+1`、`@0`、`@-1` 与空白编码，另加
  `TestALineCandidateNeedsALineCodeAndAPositiveVersion`。判别力：分别去掉往返比较、解析侧正数门、铸造门，各自那条用例红（证完还原）。
- `foldPathExecutability` 的班期版本引用改为 `"LINE/"+versionReference(line.Code, line.Version)`，与同族一致，不再读候选标识；今天输出逐字相同，
  既有用例照绿。判据一的凭据随之补全：network-routing 非测试代码里按内容解候选标识的只剩 `LineVersion`，关务适配器只把它当不透明键。
- 验证（钉 `7cab7369`）：`go build ./...`、`go vet ./...` 退出 0，改动文件 `gofmt -l` 零行；带 DSN 的 `go test -count=1 -p 1` 范围同完成记录——30 个包
  ok、0 FAIL；`-v` 两条真库用例 PASS，非 SKIP。
- 增量复评：交通道 3，只评 `a44941d2..7cab7369`。
