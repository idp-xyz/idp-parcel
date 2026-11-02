# 01 网点（物流节点）与口岸各加一个可选的显示名称，随版本登记

Category: enhancement
Status: resolved——2026-11-01 进 main（全量 164 ok / 0 FAIL，种子退 0），见文末「评审记录」「进 main 记录」。此前 in-progress——2026-11-01 作者完工，待隔离评审与进 main，分支 claude-placenames（最后一笔见「完成记录」）
Blocked by: 无
地盘：`internal/networkrouting`（domain、ports、registrationjson、http、postgres 与各自用例）、`internal/customscompliance`（同上，加登记编排）、两处测试替身的签名（`cmd/parcel-customs-register`、`cmd/parcel-dispatch`）、迁移 `network_routing/0033`、`customs_compliance/0031`、`scripts/demo-seeds`（网络与口岸种子、README 两处）、`apps/admin-web` 的两份 `api.ts` 类型与网络目录读口契约用例、ADR-0353、两份 CONTEXT、GLOSSARY「物流节点」、UC-NR-001（`AT-NR-111`、`AT-NR-112` 与交接一行）、UC-CC-003（`AT-CC-457`、`AT-CC-458` 与新立「给开发的交接」一节，修订轮）、AT 盘点、本票。
出处：用户 2026-11-01 裁定，原话选项「网点、口岸各加一个可选的显示名称，随版本登记；没登名称的照旧显示编码。演示种子补上名称，页面行首显示名称、编码进技术详情」；协调方派单。页面显示名称不在本票，协调方另派。

## 要做的

1. 定设计并落 ADR：名称属哪一层、可选、为空时的读法、长度与字符约束、与 ADR-0267 的关系、口岸与网点是否同一写法；同笔改 CONTEXT、GLOSSARY 与引用它的 UC。
2. 实现：领域、端口、应用、HTTP 读口与登记口、postgres（新列可空，迁移能在已有数据的库上跑）、受控 CLI；测试含旧数据迁移后读法不变、旧版本重放与冲突判定不变。
3. 前端只改 `api.ts` 类型与契约用例。
4. 演示种子给 SYN- 网点、口岸补中文名称，新库灌通。

## 完成记录

**起点** `origin/main` `b2c1505c7`。分支 `claude-placenames`，只推分支不推 main。ADR 号 0353：核过 origin 各 `claude-*` 分支（最高 0352）与主工作树未提交的 0350、0351，两边都没占。

### 怎么做的，为什么

设计全在 [ADR-0353](../../../docs/adr/0353-logistics-nodes-and-customs-ports-may-carry-an-optional-display-name-registered-with-the-version-and-it-never-enters-a-judgment.md)，这里只记落点与取舍。

- **随版本登记**：名称是节点版本行、口岸那一版上的一格内容，不另立名册；改名登新版本，前版照留旧名（两册原有的换版写法）。
- **可选与读法**：登记输入缺席或 `null` 即没登；给了就过领域构造门（`domain.NewNodeDisplayName`、`domain.NewCustomsPortDisplayName`），不合格在翻译那一步拒——CLI 非零、在线口 400 `MALFORMED_REQUEST`，与依据格写坏同一格，不新增受理门理由。读口没登的行不带 `displayName`（`omitempty`），不答空串、不拿编码顶。
- **约束（产品策略）**：非空白、首尾无空白、至多 100 个码点、不含控制 / 格式（Cf）/ 行段分隔字符；不规范化、不唯一。库上 CHECK 是第二道网。两个上下文各写一份检查，同一组边界各自钉住（领域包互不依赖）。
- **租户取值**：参考配置不带名称；节点的采用行许带 `display_name`（`nodeAdoptionKeys`），别的族采用行带它答 `ErrAdoptedContentGiven`，直接行按未知键拒。演示种子四个节点修订 1 都是采用行，靠这一格带名称。
- **不进判断**：网络目录选版读口与关务适用性快照都不取名称；真库用例对拍「只差名称的两个租户」选版快照逐格相同、适用性作答与目录版本引用相同。节点族上列的检索词多匹配名称一列。
- **重放与冲突**：口岸册把名称并入同一起点的逐字段比对（两边都没登算相同）；网络目录同一修订号照旧由主键挡，与名称无关。
- **ADR-0267**：两册不存摘要、没有规范化版本，换号判据不适用；旧行字节与身份不变、旧构建对带名称的登记响亮拒（两侧 `DisallowUnknownFields`）、滚动发布期间口岸重放的新旧答法差一格，记在决定七与越权风险点 4。
- **迁移**：`network_routing/0033_logistics_node_version_display_name.sql`、`customs_compliance/0031_candidate_port_display_name.sql`，各加一个可空列与 CHECK，不回填。真库用例按「施加到前一份 → 落旧行 → 施加本份」核过旧行一格不动、名称为 NULL。
- **种子**：首版就带名称（上海枢纽、深圳口岸站、新加坡枢纽、新加坡末端站；深圳出境口岸、新加坡入境口岸），标识不变；存量演示库要 `--reset` 重灌（旧库上重跑节点撞修订号、口岸答内容冲突）。`cmd/parcel-dispatch` 的参考网络接线用例逐行读这批种子，跑过。

### 判据对用例

| AT / 规则 | 判据 | 用例 |
|---|---|---|
| `AT-NR-111` | 名称给了就要成形、恰在上限照收、零值即没登 | `domain::TestANodeDisplayNameIsTakenAsWrittenOrRefused` |
| `AT-NR-111` | 直接行缺席 / null 即没登、带名称原样登、不合格拒 | `registrationjson::TestADirectNodeRowCarriesAnOptionalDisplayName` |
| `AT-NR-111` | 节点采用行可给名称、内容照旧取参考配置；别的族带它被拒 | `registrationjson::TestAnAdoptedNodeMayBeGivenItsDisplayNameByTheAdoptionRow` |
| `AT-NR-111` | 受理门不看名称、原样到写入口 | `application::TestCatalogRegistrationCarriesTheNodeDisplayNameUntouched` |
| `AT-NR-111` | 在线口带名称成命令，不合格答 400 | `http::TestTheOnlineNodeRegistrationCarriesTheDisplayNameAndRefusesAMalformedOne` |
| `AT-NR-111`、`112` | 读口登了带、没登不带，其余键集不变 | `http::TestNodeVersionsCarryTheirDisplayNameOnlyWhenRegistered` |
| `AT-NR-111`、`112` | 真库：逐版带名、前版接续闭合照留、检索匹配名称、同修订号主键挡、选版快照不含名称 | `postgres::TestNodeDisplayNamesAreRegisteredPerVersionAndStayOutOfTheSnapshot` |
| `AT-NR-112` | 迁移在有数据的库上不改旧行、CHECK 第二道网 | `postgres::TestMigration0033LeavesExistingNodeVersionsUnnamedAndUnchanged` |
| `AT-CC-457`、`AT-CC-458`（修订轮前引关务 CONTEXT 口岸名称句） | 领域、翻译、编排比对、读口、真库往返与比对、适用性不受影响、迁移 | `domain::TestACustomsPortDisplayNameIsTakenAsWrittenOrRefused`、`registrationjson::TestACandidatePortRowCarriesAnOptionalDisplayName`、`application::TestTheCandidatePortDisplayNameIsPartOfTheReplayComparison`、`http::TestCandidatePortListCarriesTheDisplayNameOnlyWhenRegistered`、`postgres::TestCandidatePortDisplayNamesRoundTripAndJoinTheReplayComparison`、`::TestCandidatePortDisplayNamesStayOutOfTheApplicabilityJudgment`、`::TestMigration0031LeavesExistingCandidatePortsUnnamedAndUnchanged` |
| 前端读口契约 | 名称原样交出，没登不带 | `apps/admin-web/src/pages/network/network-catalog-read.test.ts` 新增一例 |

AT 号 111、112 核过 origin 各分支与主工作树无撞；盘点①桶在 `AT-NR-110` 之后加两行、文末加追记。口岸那一半没有 UC 承载（见「留给协调方」），用例 `Covers:` 引关务 CONTEXT 规则句，照 CC 的既有口径。

### 自验

`HTTPS_PROXY=http://127.0.0.1:7897`、DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`（含 PG）：`go build ./...`；`go vet ./internal/networkrouting/... ./internal/customscompliance/... ./cmd/...`；改过与新增的 `.go` 跑 `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/networkrouting/... ./internal/customscompliance/... ./internal/platform/... ./internal/architecture/... ./cmd/... ./migrations/...` 55 个包全 ok（含 PG）；文档改完后 `./internal/architecture/... ./tools/...` 再跑一遍 ok。前端 `npx tsc -b` 无输出，`pnpm test` 1007 / 0 失败。种子：一次性库 `seed_placenames` 上 `seed.sh` 退 0，库里五个节点版本与三个口岸版本都带名称，用后删库。`go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 只报生成物过期（作者不重生成）。

### 留给协调方

- ~~**口岸目录没有 UC。**~~ 修订轮已按协调方裁定处理，见下。 口岸与申报路径两册是票 admin-remainder-mechanism-batch/03 落的，`docs/application/customs-compliance` 下没有哪份 UC 承载它的 AT 与「给开发的交接」；关务 CONTEXT 新加的「运营查阅以名称称呼口岸」那半句在代码里还没到（管理台口岸页），差距无处可记，暂记在这里。要不要给两册立 UC 或挂到哪份既有 UC，请裁。
- **页面显示**：网点页、口岸页行首显示名称、编码进技术详情另派；连接、线路、服务区域等引用节点编码的地方要不要也换成名称（读口今天只在节点族带名称，别处要按编码对照），一并裁。
- **`demo_live`**：存量演示库没有名称，要 `seed.sh --reset` 重灌才有（派单禁碰，我没动）。
- 生成物（ADR 索引、AT 盘点计数）没重生成，`-check` 只报过期。

### 修订轮（2026-11-01 作者，应协调方评审，评审钉 `738b37a89`、两轴阻断 0 条、四条要修）

1. **口岸交接与 AT 挂处**：customs-compliance 下没有一份 UC 直接承载「关务适用性判断」（它的执行器只被 NR 消费）；在既有 CC UC 里唯一谈到口岸适用性、并以当前口岸作门禁的是 UC-CC-003，挂在那里。UC-CC-003 原无「给开发的交接」，照 UC-NR-001 的形新立一节，一行：运营查阅以名称称呼口岸，后端已带 `displayName`，口岸页由前端票补。口岸用例改挂新号 `AT-CC-457`（登记与读法）、`AT-CC-458`（重放比对、改名、旧版、不进适用性判断）——`AT-CC-456` 已被 `claude-hometodo` 占用，跳过；`Covers:` 改指新号，盘点①桶在 `AT-CC-455` 之后加两行、追记改写。
2. **CHECK 只守 ASCII 空白**：两份迁移头注与 ADR-0353 决定三如实写明「第二道网，只守 ASCII 空白与长度，宽严随库 ctype；完整校验以领域门为准」，点名 U+0085 / U+00A0 / U+2007 / U+202F。CHECK 本身不改（只改注释，迁移 SQL 语句一字未动）。
3. **别的族采用行带 `display_name`**：`adoptionOf` 对这一键单独答 `ErrDisplayNameOnlyOnNodeAdoption`「只有物流节点的采用行可以带显示名称，这一族没有名称这一格」，不再落进 `ErrAdoptedContentGiven`「由参考配置与采用路径给出」那一句；用例钉住连接与日历两族，断言新错误、新文案，且不是旧错误、不含「由参考配置」。ADR 决定四同步改写。
4. **UC-NR-001 `AT-NR-111`**：补上采用行那一路——别的族的采用行带名称拒登并答上面那句，直接行按未知键拒。

自验：`go build ./...`；`go vet ./internal/networkrouting/... ./internal/customscompliance/... ./cmd/...`；改过的 `.go` `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/networkrouting/... ./internal/customscompliance/... ./internal/architecture/... ./migrations/... ./cmd/parcel-dispatch/... ./cmd/parcel-customs-register/... ./cmd/parcel-network-register/... ./tools/...` 全 ok（含 PG）。种子未重灌：种子数据与迁移语句都没动。

### 提交

- `77febb747` 代码、迁移、用例、管理台类型与契约用例、演示种子
- `2a5091b48` 文档：ADR-0353、两份 CONTEXT、GLOSSARY、UC-NR-001、AT 盘点、本票
- `738b37a89` 本票提交清单
- `6c4c3f9bb` 修订轮四条
- 其后一笔只改本票提交清单

## 评审记录（2026-11-01，隔离子代理，钉 `738b37a89`）

两轴阻断为零。ADR-0267 判「不适用」站得住：节点版本与口岸都不进任何摘要、指纹或 outbox 正文，`LoadDefinitionsAt` 的 jsonb 选列与关务适用性快照 SQL 两树无差，两登记口的载荷摘要尚未落地，判断快照不序列化节点版本。没登名称的行两树读出逐字节相同（探针对拍 `nodeVersionBodyOf`、`serveCandidatePorts`，含闭区间与开区间）；全仓无 `SELECT *`，旧构建连迁移后的库照常可用。校验口径逐码点对拍（U+0001–U+10FFFF 四种位置加 100 / 101 字符边界）：库拒领域收 0 条，库收领域拒 691 条且都是 Cf、Zl/Zp 与非 ASCII 空白，两份领域门结论相同；恰 100 码点收、101 拒，null 与缺键为没登、空串拒。口岸重放比对与网点主键行为、滚动发布那一格如 ADR 所写。非阻断四条由协调方裁定、修订轮 `6c4c3f9bb` 处理：口岸那条 CONTEXT 规则的交接挂 UC-CC-003 并立 AT-CC-457/458（AT-CC-456 已被 home-today-todo 分支占）；迁移头注与 ADR-0353 决定三如实写明库上 CHECK 只守 ASCII 空白、完整校验以领域门为准；别的族采用行带名称单独答 `ErrDisplayNameOnlyOnNodeAdoption`；AT-NR-111 补采用行那一路。修订只多一个专门的拒答与文档，协调方核过 diff 后未再派复评。

## 进 main 记录（2026-11-01，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-placenames` tip `b0e4ca40a` 作出处；隔离树把 `b2c1505c7..b0e4ca40a` cherry-pick 到 main `4aade8e49`，无冲突；重生成 ADR 索引、AT 盘点计数与机制清点各成一笔，两项 `-check` 退 0；对 `3cfdb0cfa` 跑全量 164 ok / 0 FAIL（含 PG）；种子一次性库 `seed_placenames` 退 0 后删库；管理台 `tsc -b` 过。本笔票面簿记只动 `.md`，叠在验过的树上推确切 SHA。
- 留给后续（协调方已排）：管理台各处显示节点 / 口岸名称（含替换网络与网点、口岸与申报路径两页页头「只登编码，不登名称」的说明）另派前端票；共享演示库 `demo_live` 走领域本来的路补名称（节点登新版本、口岸登新一版），不 `--reset`。
