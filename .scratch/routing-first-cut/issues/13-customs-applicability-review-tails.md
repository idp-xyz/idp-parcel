# 13 关务适用性判断的评审尾巴：词条改引规则节、两处旧口径注释、NR 侧同国用例

Category: enhancement
Status: resolved——2026-10-10 22:3x 通道 1 重放进 main：`f30dc502`…`d32b5e51`，清点重生成无差；评审与进 main 记录见文末。此前 in-progress——**完工，待评审与重放**（2026-10-10 21:4x 通道 2；分支 `mcp2-rfc13`，代码 tip `ce2e3454`，基 `1f8b0cea`）；完成记录见文末。此前 in-progress——2026-10-10 通道 2 认领，分支 `mcp2-rfc13`、基 `1f8b0cea`。此前 ready-for-agent——2026-10-10 通道 1 立（用户授权自决），出自 [12](12-cc-customs-applicability-judgment-for-route-candidates.md) 阻断修复两份非作者补评审（通道 3、通道 4，均钉 `18be66e8`）的非阻断项
Blocked by: [11](11-demo-network-adopted-as-reference-configuration.md)（已解：11 已进 main，通道 1 派单时核过 `internal/networkrouting` 与 `internal/customscompliance` 上无他人在途）——不是逻辑依赖，是地盘：11 正在 `internal/networkrouting` 写，等它进 main 再动，免得一个目录两个写入方
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为出自 12 的评审。
地盘：`docs/domain/customs-compliance/CONTEXT.md`「关务适用性判断」词条；`internal/networkrouting/adapters/customscompliance/` 的注释与用例；`cmd/parcel-dispatch/assemble.go` 里 `acceptanceReachability` 的头注。
出处：12 票面 Comments「补评审 ← 通道 3」「补评审 ← 通道 4」；[ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定三；AGENTS.md 红线「单一权威」。

## 做什么

1. CC CONTEXT 词条「关务适用性判断」里状态未知的三格不再自列，改为引规则节，与 `e9e1bbf7` 对不可用理由的处理同形。现状两处各列一遍，且词条那份不带「两端异国」限定：字面上同国＋依赖读不到也落状态未知，与规则三相抵。
2. 注释跟上 12 的修复：
   - `ErrCustomsCatalogUnreadable` 的注释补上经初始路由视图形成的`未决`；今天只写了可达性一侧的`未形成判断`。
   - `acceptanceReachability` 头注里「目录读不到时 CC 如实答状态未知，不冒充满足」改成现行答法：这条装配上目录读不到形成的是`未形成判断`。
3. NR 侧补一例「两端同国＋目录读不到」照常作答。今天只有异国两例，同国这一格只靠 `unknownGap` 只认 `CatalogUnreadable` 守着，改坏了没有 NR 用例会红。用例要能红：让 `unknownGap` 连同国一起上抛，或让 CC 读不到时整批答缺口，它都应失败。

## 不做

- 不改答案代数，不改 12 的分诊裁定三；不改 NR 四条约定（`EvidenceGap`、`HardConstraintOutcome`、`NetworkEvidenceView`、`CustomsApplicabilitySource`）的注释与 network-routing CONTEXT。
- 不处理「日后候选带逐候选端点后混批成真」那条残余风险（通道 4 评审所记）——那是改投影形状时的事，记在这里供那张票引用。

## 完成判据

- [x] 词条与规则节对状态未知只剩一处定义，「两端异国」限定在。——词条状态未知一格改为「带成因，成因各格同以该规则为准」（`ce2e3454`）；
  成因只在规则节「关务适用性判断」里「任一端国家/地区缺码时作答状态未知」那一条列一处，「两端异国」限定在那里。邻词条未动，改后重读过。
- [x] 两处注释与现行答法一致。——`ErrCustomsCatalogUnreadable` 补上经初始路由视图形成的`未决`；`acceptanceReachability` 头注改为两端异国的件
  在这条装配上形成`未形成判断`（`b7b75a2d`）。
- [x] 同国用例在现状上绿、在上面两种改坏之一上红，写明怎么证的。——`TestASameCountryCandidateIsStillAnsweredWhenTheCatalogIsUnreadable`
  （`41bb6348`）现状 PASS，两种改坏都红，见完成记录「判别力」。

## 完成记录（通道 2 · 2026-10-10 21:4x · 代码 tip `ce2e3454`，基 `1f8b0cea`）

派单 `task-5b077857`；证据层级 `S`（合成替身与 `SYN-` 夹具）。

**各笔**：`2c1d9936` 认领；`41bb6348` 第 3 条同国用例；`b7b75a2d` 第 2 条两处注释；`ce2e3454` 第 1 条词条；本记录随其后一笔。

**判断项**

1. 同国用例的判断服务接真的 CC 处理器（`ccapplication.NewCustomsApplicabilityHandler` 配一份读不动的目录），不用本文件的答卷替身：替身交的是
   预先折好的作答，CC 侧「读不到时整批答状态未知」若坏在处理器里，替身照样交出可用，用例拦不住。该适配器包是 ADR-0025 定的跨上下文位置，
   `TestBusinessModulesDoNotReachIntoEachOther` 对它整包豁免，用例导入 CC 的 application 不越界。
2. 用例同时钉出处：判断标识等于 `FoldCustomsApplicabilityUnreadable` 对同一候选铸的那一个、不带目录版本——证它走的确是读不到那条路，没被当成
   目录为空（CC 规则节要求两条路的判断标识不得相同）。
3. 词条只换状态未知那一格的括号，形状照 `e9e1bbf7`；后面那句「状态未知不折成可用，也不从「查无记录」推导可用」留着——它说的是这一格怎么对待，
   不是哪几格落进它，与规则节同向、不相抵。
4. `acceptanceReachability` 头注写「两端异国的件」：同国与缺码不靠目录，目录读不到时照常作答，不是整条装配一律`未形成判断`。机制只在
   `nrcustoms.ErrCustomsCatalogUnreadable` 的注释里讲，头注指过去。
5. **提交信更正**：`41bb6348` 的提交信末句写「机制清点在本笔的检出上重生成，随笔提」，实际重生成无差，该笔没带清点文件。分支已推、本仓不
   force-push，在此更正。

**判别力**（临时变异，证完 `git checkout` 还原，未提交；钉 `41bb6348`）

- M1 本桥连同国一起上抛：`translate` 的可用一格加「出处不带目录版本即交 `ErrCustomsCatalogUnreadable`」——新用例红，
  `err = network routing: customs applicability: the customs port and path catalog is unreadable: candidate cand-domestic`。
- M2 CC 读不到时整批答状态未知：`FoldCustomsApplicabilityUnreadable` 去掉先过 `foldEndpoints` 那一段——新用例红，同一句。
- 现状：新用例 PASS，所在包全绿。

**验证**（钉 `ce2e3454`，隔离工作树无未提交，含 PG）

- `go build ./...`、`go vet ./...` 退 0；改动的 `.go` 文件 `gofmt -l` 零行，CR 与 BOM 均无。
- `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable go test -count=1 -p 1` 退 0：包取 `go list -test` 反查依赖
  `internal/networkrouting/adapters/customscompliance` 或 `cmd/parcel-dispatch` 的全部包（即该适配器包与 `cmd/parcel-dispatch`），加 `./internal/architecture/...`。
- `-v` 单跑 `cmd/parcel-dispatch` 的 `TestAnUnreadableCustomsCatalogReachesBothEvidenceViewsAsADependencyFailure`：PASS，非 SKIP。
- `tools/mechanism-inventory` 在 `ce2e3454` 的干净检出上重生成，`docs/product/MECHANISM-INVENTORY.md` 无差。

**未验**：全仓 `go test ./...`，留推送方重放后那一跑。

## Comments

**评审 ← 通道 3 · 钉 `f65447c1` · 22:2x（`task-3d797e04`；卡在开工后被撤回、已终态，报告经频道交）**：两轴无阻断。

- Standards 非阻断（低）：`cmd/parcel-dispatch/assemble.go` 的 `acceptanceReachability` 头注后半句重述了 `nrcustoms.ErrCustomsCatalogUnreadable`
  名下的上抛机制（当依赖调不通上抛、不记成`资料不足`），与判断项 4「机制只在那里讲」不符；留「两端异国的件在这条装配上形成`未形成判断`
  （见 `nrcustoms.ErrCustomsCatalogUnreadable`）」即可。
- Spec：无非阻断。判别力独立复现：M1、M2 各自让新用例红在 err 断言；M1、M2 配 `1f8b0cea` 的旧测试文件全绿，票面前提属实；
  加做 M3（处理器把读不到当目录为空）红在出处判断标识，判断项 2 成立。
- 验证：`f65447c1` 上 build / vet / gofmt 净，CC 与 NR 适配器包 ok；带 DSN `-v` 的 `TestAnUnreadableCustomsCatalogReachesBothEvidenceViewsAsADependencyFailure`
  PASS，非 SKIP。

**评审 ← 通道 4 · 钉 `f65447c1` · 22:2x（`task-1cbc90cd`）**：两轴无阻断。

- Standards 非阻断（低）：`ErrCustomsCatalogUnreadable` 的注释补了初始路由一侧的`未决`，但「为什么不能译成缺口」仍只写可达性的`资料不足`；
  初始路由一侧译成缺口同样是`未决`（`RouteCandidateEvidenceIncomplete`），差在未决原因（`RouteEvidenceUnavailable`）。旁注（旧有、不在本票地盘）：
  `assemble.go` 的 `brokenCustomsSource` 头注仍只写`未形成判断`，而它同时接进初始路由那条。
- Spec：无非阻断。票面两种改坏之外另加两种（坏在处理器、处理器把读不到折成目录为空），四种下新用例都是该包唯一变红的用例。
- 验证：同上，带 DSN 用例 PASS，非 SKIP。

## 进 main 记录（通道 1 · 2026-10-10 22:3x）

- 远端 main `e266a876` → `d32b5e51`（本记录随其后一笔）：本票五笔 cherry-pick 重放到 tip，SHA 换了——分支 `mcp2-rfc13` 的 `2c1d9936`、`41bb6348`、
  `b7b75a2d`、`ce2e3454`、`f65447c1` 在 main 上依次是 `f30dc502`、`ac06188f`、`6b53f51c`、`4d936e55`、`d32b5e51`。重放前核过 `1f8b0cea..e266a876`
  与本票文件无重叠，重放后本票文件与分支 tip 逐字节一致；清点在 `d32b5e51` 上重生成无差，不另成笔。
- 评审：通道 3 与通道 4 各自独立评完（通道 3 那张卡撤回未及），两轴均无阻断。两份各一条低优先非阻断见 Comments，随票记、未改，
  日后碰这两处注释的票顺手处理。
- 验证（推送方，钉合入候选 `d32b5e51`）：`go build ./...`、`go vet ./...` 退出 0；带 DSN 全仓 `go test -count=1 -p 1 ./...` 138 个包 ok、0 FAIL；
  `-v` 单跑 `TestASameCountryCandidateIsStillAnsweredWhenTheCatalogIsUnreadable` PASS，带 DSN `-v` 单跑
  `TestAnUnreadableCustomsCatalogReachesBothEvidenceViewsAsADependencyFailure` PASS（0.48s，非 SKIP）。
- 解锁：[17](17-initial-route-pricing-input-from-customer-declaration.md) 的「Blocked by 13」（地盘）已解。
