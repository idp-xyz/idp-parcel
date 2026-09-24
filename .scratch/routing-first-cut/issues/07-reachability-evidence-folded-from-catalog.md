# 07 可达性证据从版本化网络目录折出：视图修订、服务区域、候选与可执行性

Category: enhancement
Status: resolved · 已进 main——2026-09-24 通道 5 自行代行推送方以 merge commit `f2645d86` 合入 main（合并基 `537db8c1`；分支 `mcp5-rfc07` tip `e776cbd1` 原样进 main，SHA 不换）。评审只有作者自审、没有非作者评审（用户答「没有其他人帮你了，你自决吧」，隔离评审子代理不可用），非作者评审仍欠着；见 Comments「进 main 记录」。续做完成于单 task-67538d38（接封存笔 `f125ea1c`），代码 tip `0a33cb16`、清点 `8feddbc9`；完成记录见文末。此前：in-progress——2026-09-24 通道 5 认领（单 task-2c47b04e-3fa8-4f06-8bc2-4d9ba6d00936），分支 `mcp5-rfc07` 基 `f9fffabe`；迁移号预留 network_routing `0011`。再此前：ready-for-agent
Blocked by: 02
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「接路由证据取数侧」那一步（可达性一侧）
地盘：network-routing 目录的内容列（服务区域覆盖、节点对区域的覆盖等，新迁移，号开工时在频道预留）、目录登记口、postgres 取数侧、证据视图端口；[ADR-0068](../../../docs/adr/0068-versioned-network-catalog-structure-precedes-rule-content.md) 状态行与两处护栏注释（`0008` 迁移头注、目录适配器 `NetworkCatalog` 的类型注释）。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七；02 的 ADR；[ADR-0075](../../../docs/adr/0075-customer-address-is-carried-with-the-routing-request.md)；[ADR-0052](../../../docs/adr/0052-network-evidence-catalogue-has-an-unconfigured-grade.md) 与 [ADR-0053](../../../docs/adr/0053-network-fact-families-are-derived-not-registrable.md)（三格、不得退成空证据）；[`first-tenant-runway/03`](../../first-tenant-runway/issues/03-network-resolution-layer.md) 的 Answer。

## 做什么

1. **第一步**：视图修订改由目录修订锚派生（first-tenant-runway/03 的 Answer「`0007` 与 `0008` 不合流」一节；漏掉的症状是静默错判，不是报错）。
2. 按 02 的决定给目录补内容列的**形状**（服务区域覆盖文法、节点对区域的覆盖角色等），登记口经领域构造门收；迁移不种任何默认行。
3. 取数侧按判断键的 `asOf` 选版，折出服务区域解析、候选（02 定的首版生成形态）、路径可执行性（含临时网络可用性调整）与目录内可得的硬约束。关务资格按 02 定的来源；来源未接时如实答状态未知，不答满足。
4. 证据视图端口按 ADR-0075 收地理解析投影——导出签名改动，开工前在频道报窗口。本票用合成投影测，PS 侧携带归 08。
5. 目录为空，或判断时点没有适用的路由策略版本 → `未配置`；登记了而解不出 → 照旧响亮上抛，不退成`未配置`，也不退成空证据。
6. 同笔：02 的 ADR 里 ADR-0068 决定六的部分停用在此生效，两处护栏注释一并改。

## 不做

- 不做时间投影、段链与成本（归 09、10）；不改 PS。

## 完成判据

- [x] 真库用例：合成目录上可达性判断得出可达、不可达、资料不足各一；空目录与无适用策略各答`未配置`；目录改一笔后视图修订随之变。
- [x] 应用层可达性用例经真取数侧跑通（证据层级 `S`）。
- [x] ADR-0068 状态行、两处护栏与代码同笔（`0008` 头注一格改由 `0011` 头注承载，理由见完成记录判断项一）。

## 开工设计（2026-09-24 通道 5，钉 `f9fffabe`；下一任接手照此）

**分层**：折叠是判断方法（ADR-0146 产品策略），不落 postgres 适配器（「适配器只翻译不判断」）。
- `ports`：`NetworkCatalogSnapshot` 从 postgres 适配器挪进 ports，加读端口 `NetworkCatalogRead`（`LoadDefinitionsAt`，目录适配器已实现同名方法）；加关务事实来源端口（逐候选交 `domain.HardConstraintFinding`）；`NetworkEvidenceView.LoadNetworkEvidence` 多收一个随请求携带的内容结构（首版只含地理解析投影，08 / 09 再加服务要求与承诺上界）。
- `application`：目录取数侧（实现 `NetworkEvidenceView`）= 读快照 → 判未配置 → 折服务区域解析、候选、可执行性 → 并入关务事实 → 交 `NetworkEvidence`；生产装配的关务来源是「未配置」实现，逐候选答状态未知（ADR-0148 决定三），测试用替身答满足以证「可达」（证据层级 `S`）。
- `domain`：地理解析投影（寄件段、收件段，各国家 / 地区码与邮编，原样；国家码不成形按资料不足）；服务区域覆盖的匹配（整国家 / 地区，或国家加邮编前缀逐字比）；服务区域解析补一格「起点侧不在覆盖内」——UC-NR-002 层次 1 写的是起止服务区域，现有三格只有终点侧。

**目录内容（迁移 `0011`）**：`service_area_version` 加覆盖国家、邮编前缀数组、始发节点数组、交付节点数组四列，全可空（存量行没有，按「缺哪一格候选在那一格如实不可用」处置），CHECK 钉形状；CONTEXT Language「服务区域」本就是「把客户地址解析为候选收寄节点、交付节点或尾程注入节点」，节点角色随区域版本走。

**折叠规则（ADR-0148 决定二、五、六）**：
- 未配置：目录修订锚不存在，或 asOf 没有 `applicable_scope` 等于判断键服务目的的路由策略版本。
- 候选：asOf 适用的每条线路，首节点至少服务一个区域的始发角色、末节点至少服务一个区域的交付角色，才成候选。
- 服务区域解析（逐候选）：寄件侧或收件侧缺国家码 → 资料不足（缺口点名哪一侧）；首节点所服务的区域都不覆盖寄件地址 → 起点侧排除；末节点所服务的区域都不覆盖收件地址 → 终点侧排除；都覆盖 → 覆盖（引所用收件侧区域版本）。
- 可执行性：线路各连接与节点在 asOf 都有适用版本，且无生效中的临时调整（停运 / 关闭类）作用于该线路、其连接或节点 → 可执行（引线路版本）；否则不可执行（引那条调整或缺版本的连接）。
- 视图修订 = 目录修订锚。

**同笔**：ADR-0068、ADR-0053 Status 行前向指针；`0008` 头注与 `NetworkCatalog` 类型注释两处护栏；`NetworkDefinitions` 退出两个证据视图（初始路由证据视图在 09 之前照旧响亮上抛「解不出」，但`未配置`改由目录与策略判，`0007` 不再被读）。

## 完成记录（2026-09-24 通道 5，单 task-67538d38；代码 tip `0a33cb16`，清点 `8feddbc9`）

**分支上的笔**（`mcp5-rfc07`，均已推 origin）：认领 `38001415`、开工设计 `85609870`、之一 `25e47541`（领域：投影、覆盖匹配、起点侧排除）、封存 `f125ea1c`（前一会话现场原样入库）；本单续做四笔——`26e237a5` 合入 main `443a472e`（含 rfc03；merge 不 rebase，已推 SHA 不改写，唯一冲突在登记拒收原因枚举，两边都留）、之二 `0b003474`（覆盖四列的登记门、读回、运营上列、HTTP 查阅体、CLI 载荷）、之三 `0a33cb16`（证据视图折叠 + 同笔项）、`8feddbc9`（干净检出重生成清点）。

**完成判据对照**（真库用例都在 `internal/networkrouting/adapters/postgres/catalog_reachability_test.go`，合成目录经登记用例受理门落库）：
- 三值：`TestReachabilityOverARealCatalogFormsEachOfTheThreeValues`——收件地址落在交付区域前缀内得`可达`，他国得`不可达`（`SERVICE_AREA_EXCLUDES_DESTINATION/SYN-AREA-XB-10@1`），缺国家码得`资料不足`；三份判断经真判断库落库，视图修订 = 目录修订锚 9。
- `未配置`两格：`TestReachabilityOverARealCatalogAnswersUnconfigured`——空目录、目录在而无适用于该服务目的的策略，各答 `NETWORK_EVIDENCE_NOT_CONFIGURED`，不落库。
- 改一笔修订变：`TestAOneRowCatalogChangeSupersedesTheJudgment`——修订 9 → 10，提交前重校由`仍然当前`转`已换代`。
- 应用层经真取数侧：同一文件里 `AssessParcelReachabilityHandler` 跑在真目录（选版读口）+ `CatalogNetworkEvidence` + 真判断库上，关务来源用答满足的替身（证据层级 `S`）。
- 同笔：`0a33cb16` 一笔内含 ADR-0068 / ADR-0053 的 Status 行与 Links 前向指针、ADR 索引三行、`NetworkCatalog` 类型注释、`0011` 头注护栏段、ADR-0148 Status 行补记，与证据视图接线同笔。

**做什么逐条**：1 视图修订取目录修订锚（选版读口单语句）；2 覆盖四列（`0011`）经领域构造门登记、读回、列出，迁移不种默认行；3 折叠在 `application.CatalogNetworkEvidence`（候选、服务区域解析、含临时调整的可执行性、关务状态未知）；4 `NetworkEvidenceView.LoadNetworkEvidence` 收 `ports.RequestCarriedContent`（开工前已报窗口），合成投影测；5 空目录 / 无适用策略答`未配置`，登记了解不出响亮上抛；6 部分停用自 `0a33cb16` 起生效，`NetworkDefinitions` 删除，`cmd/parcel-dispatch` 三处装配改接两个目录视图。

**自验**（钉 `0a33cb16`，树干净；DSN 已设，先单跑 `TestReachabilityOverARealCatalogAnswersUnconfigured` 为 PASS 非 SKIP）：`go build ./...` 0、`go vet ./...` 0、`gofmt -l` 无输出；改动包 ∪ `go list` 反查的生产反向依赖 ∪ 测试里用 `pgtest` 或实现 NR 端口的包（`0011` 改了每个真库用例跑的迁移计划，而 `.Deps` 看不见测试导入）共 50 包，含 `cmd/*` 12 个与 `internal/architecture`，`-p 1 -count=1`：49 ok / 0 FAIL / 1 无测试文件（`scripts/demo-seeds/migrate`），耗时 1 分 41 秒。

**自 main `443a472e` 以来动过的 .go / .sql**（`git diff --name-status 443a472e 0a33cb16`）：
- `migrations/network_routing/0011_service_area_coverage.sql`（新）
- `internal/networkrouting/domain`：`geo_resolution.go`（新）、`geo_resolution_test.go`（新）、`service_area.go`
- `internal/networkrouting/ports`：`ports.go`、`catalog_read.go`、`catalog_registration.go`、`customs_applicability.go`（新）
- `internal/networkrouting/application`：`catalog_network_evidence.go`（新）、`catalog_initial_route_evidence.go`（新）、`assess_parcel_reachability.go`、`validate_reachability_judgment.go`、`create_initial_route.go`（注释）、`register_network_catalog.go`；测试 `catalog_network_evidence_test.go`、`catalog_initial_route_evidence_test.go`、`register_service_area_coverage_test.go`（均新）、`assess_parcel_reachability_test.go`、`create_initial_route_test.go`
- `internal/networkrouting/adapters/postgres`：`network_catalog.go`、`network_catalog_list.go`；删 `network_definition.go` 与其测试；测试 `catalog_reachability_test.go`、`network_catalog_coverage_test.go`（均新）
- `internal/networkrouting/adapters/http`：`query_network_catalog.go` 与测试
- `internal/parcelshipment/adapters/networkrouting/reachability_test.go`（只改证据替身签名）
- `cmd/parcel-dispatch`：`assemble.go`、`syn_pc_seed_test.go`、`synthetic_v0_test.go`（注释）；`cmd/parcel-network-register`：`main.go` 与测试

**判断项**（评审请重点看）：
1. **`0008` 头注没改。** 已施加迁移按 checksum 固定（`migrations.go` 对文件原始内容算 sha256，ADR-0068 Consequences「不得改写 0008」），改一个注释字符，已迁移过的库下次运行就以 `ErrChecksumDrift` 停下。改由 `0011` 头注写明 `0008` 那几句是立表时的状态、自本笔起按 ADR-0148 决定六停用，`NetworkCatalog` 类型注释同写；先例 ADR-0127 对 `0020` 头注的同一处置。ADR-0148 Status 行补记了这一格。
2. **候选线路的连接或节点没有适用版本、段链断开 → `ErrCatalogUnresolvable`（未形成判断），不是开工设计写的「不可执行」。** 依据 UC-NR-002 矩阵行 7「必需网络版本未发布、配置损坏 → 未形成判断」与 `logical_path.go` 可执行性「刻意没有未知格，读不到属技术可用性」；`不可达`要求必需权威证据完整，拿缺版本去淘汰候选会编出`不可达`。作用在候选上的适用范围调整同样上抛：目录里没有它的范围内容列，折不出它的效果。
3. **线路的 `applicable_scope` 按服务目的解释**（与路由策略同一解释；ADR-0148 决定六授权 07 定它在目录上怎样落形），另一个服务目的的线路不进候选空间。首段或末段连接缺版本时定不出两端、候选空间闭合不了，也上抛。
4. **「目录内可得的硬约束」**：目录今天没有限制类内容列（禁限运、节点 / 法人资格都不在目录里）；服务范围那一格按上一条落成候选空间成员资格，不落成限制事实；临时调整归可执行性。所以证据里的硬约束目前只来自关务来源。
5. **发起方没带投影 → `资料不足`，缺口单独点名 `GEO_PROJECTION_NOT_CARRIED`**（ADR-0148 决定二「缺席如实」），没有新增未形成原因——那会在 08 落地前打断 PS 适配器的现行路径。08 之前生产上已配置的租户会因此答资料不足；今天没有租户，演示租户的服务目的 `NETWORK_SERVICE` 与种子策略范围 `SYN-SCOPE-01` 不一致、种子区域也没登覆盖，仍答`未配置`。
6. **关务出处字段不预拟**：形状取决于 12 的第 4 问（归 CC owner），端口首版只交逐候选硬约束事实；漏答一条上抛 `ErrCustomsAnswerIncomplete`。生产接 `CustomsApplicabilityNotConnected`，已配置租户覆盖且可执行的候选因此答`资料不足`，与 ADR-0148 决定三一致。
7. **初始路由视图选版时点取时钟**：初始路由判断键没有 asOf，UC-NR-001「网络判断基线」把路由判断时点归本上下文。
8. 两份以上适用策略不报错：可达性不依赖选哪份，初始路由消费策略引用时由 09 定。重校不携带内容（视图修订不随所携内容变），折叠照跑、关务来源照问，生产上是未接实现、开销可忽略。
9. 引用写法：候选标识 = `线路码@版本`；多版区域或多条调整并列时按字典序以「+」连。

**未做 / 交出**：PS 携带投影与判断记录留投影摘要归 08；时间投影、段链、成本归 09、10；CC 判断口与出处归 12；HTTP 登记口仍是未配置 Intake（未动）。以下陈述已随本票部分过时、不在本票地盘，请簿记方按需改：开发主线 PN-02 / PN-03 行里的 `NetworkDefinitions` / `ErrNetworkDefinitionUnresolvable`，合成演示动线「墙三」对 `network_definition` 的描述。

## Comments

- 评审 ← 通道 5（**作者自审，不是非作者评审**）· 钉 `aada9abd` · 20:50。成因写清，免得被读成已过独立评审：完工报之后用户在 IDP 队列答「没有其他人帮你了，你自决吧」，频道里没有空闲的非作者通道；按 parallel-sessions「没有空闲通道时推送方自己用 `/code-review` 的隔离子代理跑」起了两个隔离评审子代理（Standards / Spec 各一），两次都报 `Authentication error`，没有产出。于是在隔离检出 `/tmp/idp-review-rfc07`（detached `aada9abd`，基线 `443a472e`）上按 skill 两轴各过一遍。非作者评审仍欠着，进 main 之后谁有空可补，发现照常回本票。
  - **Standards · 阻断**：无。**非阻断（已修）**：新写的几处注释数了别处的东西——「两个证据视图共用」「读侧两个端口在 catalog_read.go」「两处共用同一个来源」「覆盖四列（迁移 0011）」等，违反 AGENTS.md「改文档」一节「计数与行号同构」；改成点名（「可达性与初始路由」「覆盖各列」），计数只留本声明内的（`serviceAreaCoverageColumns` 的四个返回值、拒收原因枚举紧挨着的四格）与 ADR 自己的术语（三格、三路）。**非阻断（留）**：`joinSortedUnique` 用切片原地去重的惯用法，读的人要想一下别名，但正确；引用串（`线路码@版本`、`LINE/…`、`ADJUSTMENT/…`）是字符串拼写，与本上下文「引用而非自由文本」的既有写法一致，不另立类型。
  - **Spec · 阻断**：无。**非阻断（留，均已在完成记录判断项里写明）**：ADR-0148 决定一「端口取回的事实各自带出处」在关务一格未落（出处形状取决于 12 第 4 问）；所携投影的版本化摘要未进判断记录（ADR-0075 决定三，归 08 完成判据）；PS 适配器测试替身改了签名（「不改 PS」按行为读，签名随端口必改）；`0008` 头注改由 `0011` 头注承载（checksum）。**核过、站得住**：首版候选按「首节点服务某始发区域、末节点服务某交付区域」生成而不按所携地址筛——若按地址筛，区域都不覆盖目的地时候选空间为空只能停在未形成判断，而 UC-NR-002 矩阵行 5 / `AT-NR-023` 要的是`不可达`带排除依据；缺版本上抛与 logical_path「刻意没有未知格」、矩阵行 7 一致。
  - 修复与本条同笔提交，只动注释；`go build ./...`、NR 与 CLI 包 vet / test 复跑为绿。

### 进 main 记录（推送方 · 通道 5 自行代行）

- **门**：只有上一条作者自审（两轴无阻断）；非作者评审未取得，照实记，不当成已过。代行推送的依据是用户在 IDP 队列那句「没有其他人帮你了，你自决吧」。
- **合入方式**：merge commit，不 cherry-pick。分支上已有一笔 merge（`26e237a5` 合入 main `443a472e`），逐笔重放会让封存笔 `f125ea1c` 在登记拒收原因枚举处再撞一次；merge 让分支 SHA 原样进 main，`git merge-base --is-ancestor e776cbd1 main` 答得出「合了没」。先例：main 上 `b394adf4`、`f47f6983` 两笔合入。
- **合并**：隔离检出 `/tmp/idp-land-rfc07`，main `537db8c1` 之上 `git merge --no-ff e776cbd1`，得 `f2645d86`。唯一冲突是生成物 `docs/product/MECHANISM-INVENTORY.md`，在合并树上重生成解决，随合并笔落；分支清点笔 `8feddbc9` 的数字因此被覆盖（main 在 `443a472e` 之后多了 access_identity 模块，迁移总数 181 → 182）。其余文件（含 `docs/adr/README.md`、本票面）自动合上。
- **验证**（钉 `f2645d86`，DSN 已设，先单跑真库用例 `TestReachabilityOverARealCatalogAnswersUnconfigured` 为 PASS 非 SKIP）：`gofmt -l` 无输出、`go build ./...` 0、`go vet ./...` 0；`go test -p 1 -count=1 ./...` 全量 120 ok / 0 FAIL / 15 无测试文件，耗时 2 分 2 秒。
- **推送**：推前 `ls-remote origin main` = `537db8c1`（与合并基一致），`git push origin f2645d86:main`；这一推发布的是本票 10 笔（认领、开工设计、之一、封存、分支合入、之二、之三、清点、完成记录、自审修复）加合并笔，共 11 笔，没有别人的提交。共享树 `main` 随后快进到 `f2645d86`，树上用户本地的 `.cursor/` 改动原样留着。
