# 176 连接代码含窗口依据分隔符 `+` 时登记口照收，TF 准备那一路每封初始路由信封都响亮失败

Category: bug
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg176` tip `08759bf96`（代码末笔；本票完成记录随后一笔）。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，分支 `claude-dcg176`（worktree `idp-parcel-dcg176`）。此前 ready-for-agent——R3（不急）：要租户把连接代码登成带 `+` 才撞，撞了是响亮失败（准备那一路每封信都失败、可见），不静默答错；补法清楚。2026-10-26 会话 `tops-20`（Claude Code）据票 [142](./142-network-routing-guards-the-connection-window-basis-spelling.md) 完成记录越权风险点 1 立（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权）。
Blocked by: 无
地盘（建议）：NR 连接登记口（`RegisterConnectionVersion` 所在的 `internal/networkrouting` 应用层与领域构造门）及其 HTTP、CLI 拒因标签；admin-web 若有该拒因的中文标签一并补。
出处：票 142 完成记录「越权风险点」1；[ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定二与越权风险点 25；代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) R4。

## 现象（读码，钉票 142 分支 tip `6d1363429`）

- NR 目录证据视图 `CatalogInitialRouteEvidence` 折候选路径时，`plannedLeg` 给节点间段拼窗口依据 `"CONNECTION/" + versionReference(代码, 版本)`。
- TF 侧 `internal/transportfulfillment/adapters/networkrouting` 的 `connectionOf` 按 `+` 切格、认 `CONNECTION/` 前缀、要求恰一格、在最后一个 `@` 处切版本。
- 连接登记口 `RegisterConnectionVersion` 与库上 CHECK 只要求代码非空白。代码含 `+` 时，NR 拼出 `CONNECTION/A+B@1`，TF 切成两格、读不出带版本的连接，准备那一路在每封初始路由信封上响亮失败（ADR-0201 越权风险点 25 那一种）。
- 票 142 的用例守的是「恰一格 `CONNECTION/<代码>@<版本>`」这条契约在合法代码上成立，不守代码本身带分隔符。

## 修法（建议）

- 连接登记口加一个确定性拒因：代码含窗口依据分隔符 `+` 时答`未受理`并带原因；库上 CHECK 同步收紧（新迁移，取号时核在途分支）。
- 拒因是登记答案集的改动：HTTP 载荷、CLI 与 admin-web 的拒因标签都要认这一格。
- 存量：迁移前核演示种子与各在途分支的连接代码没有 `+`（今天的 `SYN-` 种子没有）；若真有，迁移不能静默失败，要先列出。

## 完成判据

- [ ] 领域/应用用例：带 `+` 的连接代码登记答未受理，原因指名分隔符；不带的照旧。
- [ ] 真库：CHECK 拒带 `+` 的代码；迁移在演示种子上施加不失败。
- [ ] HTTP、CLI、admin-web 拒因标签认这一格。
- [ ] Go 门禁；全量用 `scripts/test-full.sh`；走 Go 那条路。

## 完成记录（2026-10-26，Claude Code 作者子代理，协调方 `tops-20`；分支 `claude-dcg176`，worktree `idp-parcel-dcg176`，起点 `origin/main` `f1fece40d`）

授权：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」。

### 提交

| SHA | 内容 |
|---|---|
| `08759bf96` | fix(network-routing)：连接登记口拒含窗口依据分隔符 `+` 的连接代码，库上 CHECK 同步收紧（本票全部代码、迁移、UC 与盘点改动） |
| 随后一笔 | docs(scratch)：本票完成记录 |

### 做了什么

- **应用受理门**（`internal/networkrouting/application/register_network_catalog.go`）：`CatalogRefusalReason` 新增 `CatalogConnectionCodeCarriesBasisSeparator`，原名 `CONNECTION_CODE_CARRIES_BASIS_SEPARATOR`；`RegisterConnectionVersion` 在抬头五件（`checkVersionRow`）之后、端点之前判：代码含 `+` 答`未受理`并带这一格，写入口零调用。分隔符以 `windowBasisSeparator` 命名，注释指向产出侧 `joinSortedUnique` 与 TF 读法 `connectionOf`。结果代数照旧两格（`REGISTERED` / `REFUSED`），只多一格拒因。
- **迁移** `migrations/network_routing/0028_connection_code_without_window_basis_separator.sql`：先查存量，已有连接代码含 `+` 的行就 `RAISE EXCEPTION` 拒绝施加，报文按「租户/代码@版本」列出那几行并写明续办（先例 0027）；否则加 `network_connection_version_code_without_basis_separator`：`CHECK (strpos(connection_code, '+') = 0)`。
- **HTTP 载荷与 CLI**：两处都经既有逐名转写（`writeRegistrationAnswer`、`parcel-network-register` 的 `execute`）取 `RefusalReason().String()`，生产代码不用改，各补一条用例钉住这一格过线。
- **admin-web**：`apps/admin-web/src/pages/network/api.ts` 的 `registrationRefusalReasonLabels` 补这一格的中文标签。
- **UC**：`docs/application/network-routing/UC-NR-001-CREATE-INITIAL-ROUTE.md` 验收表立 `AT-NR-094`（紧随 `AT-NR-063`）；`.scratch/at-coverage-inventory.md` ① 桶补 `AT-NR-094` 一行。总览计数未动。
- 存量核对（取证于 `f1fece40d`，代码提交后又在 `origin/main` `47c589855` 上复核迁移号与 AT 号）：演示种子 `scripts/demo-seeds/steps/200-network.sh` 登的连接代码（`SYN-CONN-SHA-SZX`、`SYN-CONN-SZX-SIN`、`SYN-CONN-SIN-LM`）与参考配置 `referenceconfig/network-routing/network-catalog/SYN-CN-SG@1.json` 的连接都不带 `+`；全仓没有别处直写 `network_connection_version`；在途分支（`claude-*`）没有 NR 迁移，`AT-NR-094` 全仓与各在途分支无撞号。
- 未改 `docs/domain/**`，未立 ADR。

### 代决

1. **只拒 `+`，`@` 与 `/` 照收。** TF `connectionOf` 在最后一个 `@` 处切版本，NR 拼的版本是 `strconv` 写出的十进制整数、不含 `@`，所以最后一个 `@` 恒是 NR 自己的分隔，代码里带多少个 `@`（含结尾带 `@`）都读回原样；`CONNECTION/` 前缀每格只切一次，代码里带 `/`、甚至以 `CONNECTION/` 开头，切完剩下的仍是原代码。`+` 不同：它是格与格之间的分隔，带上它一格就被切成两半，前缀那一半读不出版本，后半若恰拼成 `CONNECTION/…` 还会多出一格。票 142 的 `SYN@CONN/B-C` 已证前一半；本票新用例把 `SYN-CONN-B-C@`、`CONNECTION/SYN-B-C` 与四种带 `+` 的写法（头、尾、中间、后半截拼成另一格）放在同一份读法下逐个对着受理门证。
2. **拒因落在应用受理门，不在领域构造门。** NR 没有连接定义的领域类型：登记行是 `ports.ConnectionDefinitionVersion`，各格的门都在 `NetworkCatalogRegistration` 里（先例 `ENDPOINTS_NOT_DISTINCT`），为这一格新立领域类型属重构，不在本票。
3. **单立一格，不并进 `IDENTITY_MISSING`。** 身份不缺，续办是换代码不是补身份；折进去会让登记方去补错东西（`CatalogRefusalReason` 头注的分格理由）。
4. **判在抬头五件之后、端点之前。** 一行同时差几格时先报抬头（租户、身份、版本、生效时间、区间），再报分隔符，再报端点与时区；顺序确定，与该口既有写法一致。
5. **只收连接代码。** 节点代码不进节点间段的窗口依据（候选投影那一串 TF 不读，交付段 TF 不译）；线路段链、日历与调整里的对象身份带 `+`，引的就是一条登不进来的连接，与引用别的没登的连接同一种落法，不另设门。
6. **存量带 `+` 时迁移拒绝施加并列出，不替登记方改名或删行**（先例 0027）：连接代码同时是 TF 班次方向的一格，改名要连同线路段、日历、调整与班次方向一起换，由登记方定。
7. **`windowBasisSeparator` 只在受理门用，`joinSortedUnique` 不改用它**：后者还拼服务区域版本引用与调整引用，不全是窗口依据，改用这个名字反而误导。
8. **AT 立在 UC-NR-001**：NR 没有目录登记的 UC；目录登记口的既有 AT（`AT-NR-063`，日历派送时长）也在这一份，它的「计划内容与责任边界」管计划段窗口与依据。

### 越权风险点

1. **两侧读法同步仍不受守**（NR、TF owner；票 142 越权风险点 2 同一类）。受理门与 0028 写死的是 ASCII `+`；TF 若改了 `connectionOf` 的分隔符或切法，这里不随之变。本票新用例把受理门与 NR 测试里那份读法放在一起，守的是「拒的恰是这份读法读不回的」，不守这份读法与 TF 的真读法一致。
2. **只挡读不回，不挡别的不整洁**（NR owner）。首尾空白、制表符之类不拒：TF 读回的仍是原样代码，班次方向按原样比；要不要另加整洁度门由 NR owner 定。
3. **admin-web 网络目录拒因标签本就缺若干格**（admin owner）：取证于 `f1fece40d`，`registrationRefusalReasonLabels` 没有 `RANKING_FORM_UNKNOWN`、`COVERAGE_MALFORMED`、`NODE_ROLE_BLANK`、`NODE_ROLE_DUPLICATED`、`NODE_ROLES_WITHOUT_COVERAGE`、`CALENDAR_CONTENT_OUT_OF_RANGE`、`COST_BASIS_MALFORMED`、`LEG_ZONE_MALFORMED`、`LEG_ZONE_WITHOUT_COST_BASIS`，页面按 `RegistrationAnswerNote` 的纪律示英文原名。本票只补本格，那几格不在票面，建议另立前端小票。
4. **存量带 `+` 的库上 0028 挡住整次迁移作业**（NR owner、部署 owner）：与 0027 同一取舍——宁可停下列出，不静默改数据。首个租户实例进生产前没有这种存量；演示库重灌即消。

### 完成判据 → 用例

| 判据 | 用例 |
|---|---|
| 领域/应用：带 `+` 的连接代码登记答未受理，原因指名分隔符；不带的照旧 | `internal/networkrouting/application/register_network_catalog_test.go::TestCatalogRegistrationRefusesIncompleteRowsBySlot`「连接代码带窗口依据分隔符」；`internal/networkrouting/application/connection_window_basis_contract_test.go::TestTheRegistrationGateRefusesExactlyTheConnectionCodesTheContractCannotReadBack`：逐个代码先经真证据视图折成计划段、按 TF 读法读，读不回的四种受理门指名拒且写入口零调用，读得回的四种（含 `@`、`/`、以 `CONNECTION/` 开头、结尾带 `@`）原样登记 |
| 真库：CHECK 拒带 `+` 的代码；迁移在演示种子上施加不失败 | `internal/networkrouting/adapters/postgres/connection_code_separator_test.go::TestMigration0028RefusesToApplyOverConnectionCodesCarryingTheSeparator`：停在 0028 之前落演示种子登的连接代码（从种子步脚本里认 `-kind connection` 那几份读出）、`SYN@CONN/B-C`、`CONNECTION/SYN-B-C` 与一行带 `+` 的；施加被拒、报文列出带 `+` 那一行且不列别的、行数不动；移走后施加成功；之后带 `+` 的行撞 `network_connection_version_code_without_basis_separator`。`::TestTheCatalogTableRefusesAConnectionCodeCarryingTheSeparator`：绕过受理门交目录写入口，撞同一约束、一行不落。`cmd/parcel-dispatch` 的演示网络装配用例在全迁移库上把演示种子经受理门照常登记（含 PG 跑过） |
| HTTP、CLI、admin-web 拒因标签认这一格 | `internal/networkrouting/adapters/http/register_network_catalog_test.go::TestNetworkCatalogRegistrationTranscribesTheUseCaseAnswersVerbatim`「连接代码带窗口依据分隔符」：200 + `REFUSED` + `refusalReason` 原名；`cmd/parcel-network-register/main_test.go::TestExecuteRefusesAConnectionCodeCarryingTheWindowBasisSeparator`：退出码 1、消息指名、写入口零调用，带 `@`、`/` 的照常登记；admin-web 标签补齐，三道门过 |
| Go 门禁；全量用 `scripts/test-full.sh` | 受影响范围见下；全量留给推送方 |

变异检查：受理门改成连 `@` 一起拒，新用例在 `SYN@CONN/B-C`、`SYN-CONN-B-C@` 两格红；受理门改成什么都不拒，新用例四格读不回的全红、门用例表那一格红；改回即过。

### 自验（实测于 `08759bf96`）

- `go build ./...`、`go vet ./...` 全仓过；改过与新写的 `.go` 已 `gofmt -w`，`gofmt -l internal cmd migrations tools` 无输出。
- 带 DSN（`127.0.0.1:55432`，含 PG）`go test -count=1 -p 4`：`./internal/networkrouting/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-network-register/`（三者是 `go list` 反查出依赖 NR 的 `cmd`）、`./internal/transportfulfillment/adapters/networkrouting/...`、`./internal/parcelshipment/adapters/networkrouting/...`、`./internal/architecture/...`、`./migrations/...`，20 个包全 ok、退 0（NR `adapters/postgres` 7.2 秒、`cmd/parcel-api` 23.0 秒、`cmd/parcel-dispatch` 31.2 秒）。新迁移落进嵌入清单，另把依赖 `migrations` 的其余 `cmd`（access、collection、commercial、customs、frontline-import、governance、node-operations、pricing-feed、pricing-register、settlement、transport、ve 各登记口）与 `./internal/platform/migrate/...` 带 DSN 跑了一遍，13 个包全 ok。新真库用例 `-v` 确认 PASS 非 SKIP。
- admin-web：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` fail 0；`pnpm exec vite build` 成功。
- `go run ./tools/at-coverage-counts -check` 只报「生成物与文件不一致」（总览计数待推送方重算），无缺行多行。
- 全量 `scripts/test-full.sh` 留给推送方。
