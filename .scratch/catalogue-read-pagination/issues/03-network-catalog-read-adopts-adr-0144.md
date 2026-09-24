# 03 网络目录读口（`/network-catalog`）迁到 ADR-0144

Category: enhancement
Status: in-progress——2026-09-24 通道 2 认领（派单 `task-374025f0` ← 通道 3），隔离 worktree 分支 `mcp2-crp03`：票 01 尚未重放进 main，分支基 `mcp2-crp01` tip `db96d561` 并拣入本笔，重放时只取本票的笔
Blocked by: 01
地盘：`internal/networkrouting/adapters/http/query_network_catalog.go` 与其测试、它消费的读端口与 postgres 读面、`migrations/` 下 network-routing 模块的
一条新迁移（索引）。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定三至七；首批之一（规模先到）。

## 做什么

同票 02 的四步，另有两处本册特有：

1. `?family=` 是册子选择器，保持原义，不兼作筛选维（ADR-0144 决定四）；各 `family` 各自声明可排维与筛选维。
2. **缺省序要本票裁一次**：网络版本册常按对象看版本，`-registeredAt` 未必顺手（ADR-0144 越权风险点 1）。若某个 `family` 另点缺省序，在读面头注写明
   理由，并在本票完成记录里列出，供 NR owner 复核。

## 不做

- 不改登记写面；不改作用域与授权；不动网络解析层。

## 完成判据

- 同票 02 的真库与端点用例，按每个 `family` 各跑一遍翻页不重不漏。
- 完成记录列出各 `family` 的缺省序与理由。

## 完成记录（2026-09-24，通道 2，分支 `mcp2-crp03`）

**基线与重放**：开工时票 01 尚未重放进 main，分支基 `mcp2-crp01` 的 tip `db96d561`，并拣入 main 上的认领笔 `fb2ff8bb`（分支上为 `983a5429`，只含票面
Status 一行，重放时为空、跳过）。重放只取本票的笔：`2da83db0`、`00b13dd9`、`a703dee5` 与本笔。

**落点**

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `2da83db0` | `internal/platform/cataloguepage/page.go`、`page_test.go` | `Trim`（通道 3 评审票 01 的非阻断 1） |
| `00b13dd9` | `internal/networkrouting/ports/catalog_query.go`、`catalog_query_test.go` | 七族的查询声明与 `CatalogPage[Row]`（纯新增） |
| `a703dee5` | `ports/catalog_read.go`；postgres `network_catalog_list.go` 与两份测试（新 `network_catalog_page_test.go`）；http `query_network_catalog.go` 与测试；`cmd/parcel-api/unwired_orchestration.go`、`assemble_catalog_registration_test.go` | 第 1、2 条与同票 02 的四步：端口签名、读面 keyset 取页、端点解码与 `page`；`cmd` 两处是签名跟改（已于频道预告） |
| 本笔 | 票面 | 完成记录 |

**`Trim` 的签名**：`func Trim[T any](rows []T, size int) (page []T, more bool)`——读面照 `size+1` 取行，交回本页与「还有下一页」，票 02 照用。

**各 `family` 的缺省序与理由**（供 NR owner 复核，ADR-0144 越权风险点 1）

- 七张版本表都没有登记时间一格，决定三的缺省序 `-registeredAt` 无从取，各族在自己的封闭集里另点。
- `node`、`connection`、`line`、`service-area`、`availability-adjustment`、`route-strategy`：`code` 升序；`service-calendar`：`targetKind` 升序（其身份是
  「对象种类 + 代码 + 版本号」，以种类打头）。理由：网络目录是按对象看版本的册，读面此前就是「身份升序」；缺省序与各表主键同形，决定七要的那条索引就是
  主键，所以**没有新增迁移**。
- 代价：决胜键与排序同向（决定三），同一对象内的版本由「最新版在前」变为按版本号升序。管理台只请求 `?family=`、按服务端次序展示，可见的就是这一点。
- 各族的可排维、筛选维见 `ports/catalog_query.go` 的声明；`q` 覆盖的列见 postgres 读面 `network_catalog_list.go` 的头注。

**完成判据**（钉 `a703dee5`，WSL，go1.26.8，真库为 Parcel 自带的 55432 门禁库）

- ✅ 每个 `family` 各跑一遍翻页不重不漏：`TestEachFamilyPagesWithoutRepeatsOrGapsWhileRowsAreRegistered` 七族各一子例，两行一页地翻，翻完第一页后登两行——
  落在已翻区间的那行不重出，落在未翻区间的那行在后面的页里出现，total 随登记变化，末页 `next` 为空。
- 同票 02 的其余真库与端点用例：
  - ✅ `total` 随筛选与 `q` 变：`TestFiltersAndKeywordNarrowTheRowsAndTheTotal`（维内或、维间与、`q` 不分大小写、覆盖端点列、下划线按字面）。
  - ✅ 空册答空且 `total` 为 0：`TestEmptyCatalogueListsAnswerEmptyLists`（七族）；端点 `TestAnEmptyFamilyAnswersAnEmptyArray` 钉 `page` 为
    `{"size":25,"next":null,"total":0}`。
  - ✅ 换条件拿旧游标答 400：`TestMalformedQueryParametersAreRefusedWithAReason`（换条件后的旧游标、别族的游标）。
  - ✅ 未知键、集外排序维、词表外筛选值各答 400 带理由散文：同上一条与 `TestSelfReportedScopeParametersAreRefusedBeforeTheIntake`。
  - ◑ 迁移无 CR、无 BOM：不适用——没有新增迁移（理由见上节）。
- ✅ 完成记录列出各 `family` 的缺省序与理由（上节）。
- 另：非缺省维也翻过一遍——`TestPagingByAnInstantDimensionDescendingBreaksTiesByIdentity`（`-effectiveFrom`，同一时刻两行按标识倒序决胜，时刻带微秒往返）。
- 门禁：改动包与 `go list` 反查出的反向依赖共 13 个包（含 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-network-register`）带 DSN `-p 1 -count=1`
  全 ok，`cmd/parcel-api` 的真库装配用例 `-v` 下为 PASS；postgres 适配器 68 个顶层用例、含子用例 85 PASS / 0 FAIL / 0 SKIP，http 26 / 37，ports 3 / 10，
  cataloguepage 30 / 97；`-race`（cataloguepage、http、ports）过；`internal/architecture` 过；`go build ./...`、`go vet ./...` 全仓退 0；gofmt 按入库字节无输出。
  变异核对（已还原）：`q` 不转义、游标比较带等号、决胜键不与排序同向、total 不带筛选，各红对应用例。

**判断项**

1. **其余参数的解码先于 Intake**，与 `family` 校验同属传输形状，沿端点原有分层；未配置 Intake 时坏参数答 400 而不是 403，参数形状不泄露业务内容。
2. **自报的 `tenant` / `limit` 由「忽略」变为「集外键即拒」**（ADR-0144 越权风险点 2 预告过的收紧），原用例拆成两条。管理台只带 `family`，不受影响。
3. **声明放在 ports**：端点解码与读面落列照的是同一份，放在任一个适配器里另一边都得复制。读面的列映射里重复了一份值形状——cataloguepage 按派单只加
   `Trim`、不开读取声明的接口；七族翻页用例覆盖每族的缺省序，时刻维另有一例。
4. **`total` 与本页是两条语句**：同一组筛选与 `q` 条件，但不在同一快照，并发登记时两者可能差一行；决定五要的是「同一组筛选条件」。
5. **可排维与筛选维的取舍**：各族都放出 `code` 与生效时刻；连接加起止节点，日历与调整加对象与种类维；这几族没有状态字段，所以没有状态维；线路与策略的
   `applicableScope` 不开放——其解释属折叠层（0008 迁移头注）。
6. **`family` 仍只取第一个值**，给多个 `family` 时的行为与迁移前相同；游标摘要覆盖全部 `family` 取值。
7. **未做**：管理台接游标模式（票 05）；非缺省维的专用索引（租户级规模下不必，ADR 只要求缺省序那一条）。

## Comments

### 评审 ← 通道 3 · 钉 `a703dee5`（基 `db96d561`，只读，门禁未重跑）· 2026-09-24 15:2x（推送方自通道 3 来信代落原文）

引通道 2 实跑：13 包带 DSN -p 1 全 ok、-race 过、architecture 过、全仓 build / vet 0。

**Spec** — 阻断：无。无发现（实核）：`cataloguepage.Trim` 按 size+1 切页对；七族声明在 `ports/catalog_query.go`，family 走 `Selectors`、服务日历 targetKind 带封闭词表；版本表无登记时间列→缺省序点名 code（服务日历 targetKind）并在头注写明理由，恰是 ADR-0144 决定三「没有登记时间一格的册在自己的封闭集里点名缺省序、写明为什么」，缺省序与主键同形故不新增索引合决定七；postgres 读面游标比较是行值 `(排序列, 标识…) > (…)`、ORDER BY 同向、LIMIT limit+1；total 与取页共用 `conditions`（租户 + 筛选 `= ANY` + q），不带游标谓词；q 经 `likeLiteral` 转义 `\ % _` 只做字面包含；处理器按 family 选声明再解码，答复加 page。② 自报 tenant / limit 由忽略变拒即 ADR-0144 越权风险点 2 所述，接受。⑤ 值形状在读面列映射里重复一份，接受。

非阻断：
1. **total 与本页不在同一快照**（④，`network_catalog_list.go` 的泛型取页）：并发登记时「共 N 条」可能差一两行。ADR 只要求同一组条件，已满足；若读执行器能取只读 REPEATABLE READ 事务，两条包进去即对齐，否则接受。
2. **缺省序改变了页面上的版本次序**：同一对象最新版不再在前。合 ADR 同向决胜规则、已交 NR owner；请在 catalogue-read-pagination/05 票面补一句提示——要「最新版在前」可用 `sort=-code`（决胜键同向即版本降序），代价是对象按代码倒序。
3. **解码在 Intake 之前**（③）：参数写错时未配置渠道也先答 400 而非 403。沿既有 family 校验次序，说得通；但 catalogue-read-pagination/02（客户账户）要照同一次序，免得两册答法不一——派 02 时我会写进卡面。

结论：**可接受**（Spec 0 / 3）。

**处置**（推送方 · 通道 1）：评审无阻断，按 parallel-sessions「别人分支上的活怎么进 main」重放，与 legal-entity-profile/01 同批、同一次全量，排在它之后。只取作者点名的四笔
（`983a5429` 是拣入的认领笔、内容已在 main，跳过），在 lep01 重放 tip 之上 cherry-pick 为 `aaac1233`（← `2da83db0`）/ `9b827ba1`（← `00b13dd9`）/
`462c34bf`（← `a703dee5`）/ `d5e7a79d`（← `c632f3e0`），无冲突。本票改过的十二个文件与分支 tip 逐字一致；与 lep01 同改的 `cmd/parcel-api/unwired_orchestration.go`
相对本分支 tip 只多 lep01 的 24 行纯增、相对 lep01 tip 只多本票的 +22 / -14，两块不重叠。非阻断 2 已在票 05 票面补提示（同笔）；非阻断 1 接受；非阻断 3 由通道 3 写进票 02 卡面。
