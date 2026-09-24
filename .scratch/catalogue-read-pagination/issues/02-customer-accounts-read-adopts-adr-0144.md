# 02 客户账户目录读口（`/commercial-customer-accounts`）迁到 ADR-0144

Category: enhancement
Status: in-progress——2026-09-24 通道 1 在分支 `mcp1-crp02` 上做完（基 main `09802700`，完成记录见文末），待非作者评审与重放。此前 in-progress——2026-09-24 15:3x 通道 2 会话崩溃（用户告知），用户令通道 1 接手：隔离 worktree `idp-parcel-mcp1-crp02`、分支 `mcp1-crp02`，基 main `d2dc9c3b`（01 / 03 已进 main）；通道 2 的 `mcp2-crp02` 上属本票的只有一笔票面同步、工作树无未提交改动，不沿用。此前 in-progress——通道 2 认领（派单 `task-556d1395` ← 通道 3），隔离 worktree 分支 `mcp2-crp02`：票 03 尚未重放进 main，分支基 `mcp2-crp03` tip `c632f3e0`，重放时只取本票的笔。此前 ready-for-agent——迁移编号预留 party-commercial `0035`（`0034` 已由 legal-entity-profile/02 占用，`0033` 属 legal-entity-profile/01）
Blocked by: 01
地盘：`internal/partycommercial/adapters/http/query_customer_accounts.go` 与其测试、它消费的读端口与 postgres 读面、`migrations/` 下 party-commercial 模块的
一条新迁移（索引）。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定三至七；首批之一（规模先到）。

## 做什么

1. 按答复体字段点名本册的可排维与筛选维（缺省序照决定三；有状态即放出状态维），`q` 覆盖的文本列（标识、名称这类）写在读面头注里。
2. 读端口改为收查询对象、答「本页行 + 下一游标 + 总数」；租户仍在签名上，`limit` 非正仍拒（ADR-0077 Decision 五）。
3. postgres 读面按「排序维值 + 标识」取游标之后的行，`total` 与本页同一组条件；补一条与缺省序同形的索引。
4. 端点经 01 的共用件解码，答复加 `page`；Intake 不动——隔离形态的页大小仍是 `isolatedReadLimit`。

## 不做

- 不改登记写面；不改作用域与授权。

## 完成判据

- 真库用例（`SYN-` 数据）：翻页期间插入新登记，已翻过的页不重出、未翻的页不漏行；`total` 随筛选与 `q` 变；空册答空且 `total` 为 0；换条件拿旧游标答 400。
- 端点用例：未知键、集外排序维、词表外筛选值各答 400 带理由散文。
- 迁移按字节无 CR、无 BOM（`migrations` 行尾守卫）。

## 完成记录（2026-09-24，通道 1，分支 `mcp1-crp02`，基 main `09802700`）

通道 2 会话崩溃时本票尚无代码（其分支 `mcp2-crp02` 上属本票的只有一笔票面同步、工作树无未提交改动），通道 1 从 main 起新分支重做，
照 catalogue-read-pagination/03（网络目录）已进 main 的写法迁本册。

**落点**

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `77248909` | `migrations/party_commercial/0035_customer_account_registration_by_recorded_at.sql` | 第 3 条的索引：与缺省序同形（租户 + 登记时刻倒序 + 账户号倒序，ADR-0144 决定七） |
| `57bc23c3` | `ports/customer_account_catalogue.go`、新 `ports/catalogue_page.go`、`adapters/postgres/customer_account_catalogue.go`、`adapters/http/query_customer_accounts.go`、`cmd/parcel-api/unwired_orchestration.go`、两份既有测试 | 第 1、2、4 条与第 3 条的读面：本册声明、读端口改签名、postgres 游标读面、端点解码与 `page`、占位只改签名 |
| `56245f24` | `adapters/postgres/customer_account_catalogue_test.go`、`adapters/http/query_customer_accounts_test.go` | 完成判据的用例 |
| `96f0bfda` | `docs/product/MECHANISM-INVENTORY.md` | 机制清点在分支干净检出上重生成 |

**完成判据**

- ✅ 真库用例（`SYN-` 数据，带 DSN 实跑）：`TestCustomerAccountCataloguePagesByCursorWithoutRepeatsOrGapsWhileAccountsArrive`——五个账户按缺省序两行一页，
  翻完第一页后新登 `SYN-ACC-6`，第二页仍是 `SYN-ACC-3,SYN-ACC-2`、末页 `SYN-ACC-1` 且 next 为空，新登的落在已翻过的那头、总数随之 5 → 6（偏移分页会在第二页重出
  `SYN-ACC-4`）；`TestCustomerAccountCatalogueTotalFollowsFiltersAndKeyword`——状态维内为或、维间为与、客户参与方、`q` 命中参与方名称与账户号（不分大小写）、
  `q=%` 按字面零命中，各格 `total` 与本页同一组条件；空册答空页且 `total` 为 0 在既有用例的跨租户那一格（改签名时补了 `Total` 断言）；换条件拿旧游标答 400 在端点用例里。
- ✅ 端点用例：`TestCustomerAccountsEndpointRefusesQueriesOutsideTheDeclaration`——未知键（`tenant`）、集外排序维（`sort=status`）、词表外筛选值（`status=ARCHIVED`）、
  超长 `q`、解不开的游标各答 400 `MALFORMED_REQUEST` 且理由散文点到那一格；未配置渠道时坏参数先答 400、好参数才答 403。
  `TestCustomerAccountsEndpointCarriesThePageAndRefusesAStaleCursor`——答复带 `page`（`size` 取 Intake 页大小）、换条件拿旧游标答 400（决定一原句）、同条件照常翻页。
- ✅ 迁移按字节无 CR、无 BOM；`migrations` 包与带 DSN 的 `internal/platform/migrate` 实跑通过。
- 门（作者自验，钉 `96f0bfda`，WSL，go1.26.8）：`gofmt -l` 空、全仓 `go build` / `go vet` 退 0；`go list` 反查的 12 个反向依赖（含 `cmd/parcel-api`、`cmd/parcel-commercial`、
  `cmd/parcel-dispatch`）连同 `internal/partycommercial/...` 与 `internal/architecture/...` 带 DSN `-p 1 -count=1` 全 ok；改动三包 `-race` 过。

**判断项**

1. **`registeredAt` 是最新修订的登记时刻。** 一笔新修订会把账户排回前面——与迁移前「按最新修订登记时刻倒序」同一口径，没有另立「首次登记时刻」一格。代价：翻页期间
   若某个还没翻到的账户新增了修订，它会跳到游标之前、后面各页不再出现；判据点名的「插入新登记」不受影响。
2. **`status` 只作筛选维、不作可排维**：它是装载时点按 `now()` 导出的，随时钟自己变，排在它上面的游标翻着翻着会失准。
3. **决胜键账户号与排序同向**（决定三）：迁移前缺省序是 `recorded_at DESC, account_id ASC`，现为 `DESC, DESC`；只在登记时刻完全相同时次序不同。
4. **总数与本页是两条语句**，不在同一快照（同票 03 评审非阻断 1）：并发登记时「共 N 条」可能差一两行；ADR 只要求同一组条件，已满足。
5. **解码先于 Intake**，与 `/network-catalog` 同序（票 03 评审非阻断 3 点名本票照办）。
6. `accountLikeLiteral` 与网络目录读面的 `likeLiteral` 同形各一份：两个上下文的 postgres 适配器不互相引用，共用件不含 SQL 写法（决定七）。

**未做**：管理台接游标模式、README「列表页上列通则」改写（票 05）；非缺省维的专用索引（租户级规模下不必，ADR 只要求缺省序那一条）。

**评审**：待派非作者 Spec 轴评审；评审无阻断后由推送方（通道 1）重放进 main。
