# 02 客户账户目录读口（`/commercial-customer-accounts`）迁到 ADR-0144

Category: enhancement
Status: resolved——2026-09-24 评审 ← 通道 3 可接受（无阻断，派单 `task-6688ceb1`），推送方（通道 1）随本批重放进 main：代码笔 `e4cfcfee` / `9b55da64` / `4b26ac49` / `04e5546f`（与分支对照见 Comments「处置」），推送 SHA 与全量验证以推送方广播为准。此前 in-progress——2026-09-24 通道 1 在分支 `mcp1-crp02` 上做完（基 main `09802700`，完成记录见文末），待非作者评审与重放。此前 in-progress——2026-09-24 15:3x 通道 2 会话崩溃（用户告知），用户令通道 1 接手：隔离 worktree `idp-parcel-mcp1-crp02`、分支 `mcp1-crp02`，基 main `d2dc9c3b`（01 / 03 已进 main）；通道 2 的 `mcp2-crp02` 上属本票的只有一笔票面同步、工作树无未提交改动，不沿用。此前 in-progress——通道 2 认领（派单 `task-556d1395` ← 通道 3），隔离 worktree 分支 `mcp2-crp02`：票 03 尚未重放进 main，分支基 `mcp2-crp03` tip `c632f3e0`，重放时只取本票的笔。此前 ready-for-agent——迁移编号预留 party-commercial `0035`（`0034` 已由 legal-entity-profile/02 占用，`0033` 属 legal-entity-profile/01）
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

**评审**：← 通道 3 可接受（无阻断），原文与处置见 Comments。

## Comments

### 评审 ← 通道 3 · 钉 `cadcd364`（基 `09802700`，只读，门禁未重跑）· 2026-09-24 15:5x（派单 `task-6688ceb1`，推送方自任务报告代落原文）

引作者在 `96f0bfda` 实跑：gofmt 空、全仓 build / vet 0；12 个反向依赖 + `internal/partycommercial/...` + `internal/architecture/...` 带 DSN `-p 1 -count=1` 全 ok；改动三包 `-race` 过。评审人是 ADR-0144 起草人，非代码作者。

**Standards** — 阻断：无。非阻断：无。核过的要点：
- `adapters/postgres/customer_account_catalogue.go` 的 `ListCustomerAccounts`：总数与取页共用 `customerAccountConditions` 的同一组条件，游标条件只追加到取页；行值比较与 `ORDER BY` 同向，决胜键 `account_id` 随主排序维；`LIMIT limit+1` 经 `cataloguepage.Trim` 判下一页；`accountLikeLiteral` 转 `\`、`%`、`_`，ILIKE 缺省转义符即反斜杠，参数绑定下按字面生效。折叠在先：account 段按最新修订折叠、导出状态、左连参与方名称，筛选、计数、游标比较都落在 listed 上。
- `customerAccountListed` 的状态 CASE 与 `party_identity_catalogue.go` 里法人册、身份本体册的 CASE 除表别名外逐字相同（停用在先、生效其次）。
- 游标时刻经 `cataloguepage.FormatInstant`（RFC3339Nano）保住 Postgres 的微秒，比较不失准。
- `adapters/http/query_customer_accounts.go` 的 `NewQueryCustomerAccountsEndpoint`：`ports.CustomerAccountCatalogue.Decode` 先于 `IntakeCatalogueQuery`，`*cataloguepage.MalformedQuery` 的理由进 detail；`page` 由 `cataloguepage.NewPage(query.Limit, …)` 拼，页大小取 Intake。`cmd/parcel-api/unwired_orchestration.go` 只改签名。同包里只有 `accountLikeLiteral` 这一份 LIKE 转义器，另一份在 networkrouting，判断项 6 成立。
- 迁移号：用 0035、未碰 0034。`internal/platform/migrate` 的 `Run` 按迁移标识逐步补施加、不要求编号单调，`verifyNoDrift` 只比校验和，所以 0035 先于 lep02 的 0034 进 main 也不卡已有库；两份迁移改的是不同的表，互不依赖。

**Spec** — 阻断：无。
- ① 声明 `ports.CustomerAccountCatalogue`：可排维 registeredAt / effectiveFrom / accountId、筛选维 status（词表取 domain.IdentityRegistered / IdentityEffective / IdentityDeactivated）与 customerPartyId、缺省序 -registeredAt、行标识 accountId，名字与 `customerAccountBody` 的 JSON 字段逐一相同，合决定三、四；q 覆盖 account_id、customer_party_id、party_name，写在读面头注，合票面「做什么」1。status 不作可排维成立：它由 now() 导出、随时钟自己变，排在它上面的游标会失准；决定四只要求放出状态筛选维。
- ② postgres 读面见 Standards。
- ③ 索引 `customer_account_registration_by_recorded_at`（租户 + recorded_at DESC + account_id DESC）与缺省序同形，合决定七；编号预留守住。
- ④ 端点见 Standards；未知键、集外排序维、词表外筛选值、超长 q、坏游标各答 400 带理由，未配置渠道时坏参数先 400、好参数才 403，合决定四与票 03 评审非阻断 3。
- ⑤ `TestCustomerAccountCataloguePagesByCursorWithoutRepeatsOrGapsWhileAccountsArrive` 逐笔一事务、登记时刻递增（即使同刻，账户号决胜也给出同一次序，结果确定），证到新登记落在已翻过的那头、后续页不重不漏、末页 next 为空，并点明偏移分页会在哪一格重出；`TestCustomerAccountCatalogueTotalFollowsFiltersAndKeyword` 证 total 随筛选与 q 变、q=% 按字面零命中。判断项 1 可接受：按最新修订登记时刻排是迁移前的同一口径；排序维又必须是答复体里的字段（决定三），改排首次登记时刻就得先加一格答复字段，超出本票；代价已如实写在判断项里。判断项 3（决胜键改为同向）是决定三的直接要求。

非阻断：
1. 0035 在本读面上用不上：`customerAccountListed` 的 account 段先按 `DISTINCT ON (account_id) … ORDER BY account_id, revision DESC` 折叠到最新修订，外层 `ORDER BY recorded_at DESC, account_id DESC LIMIT` 穿不过折叠，取页与计数都走不到这条索引；迁移头注「给按登记时刻倒序扫一个租户的登记行一条现成的路」眼下没有对应的读者。对决定七是形式满足。「按修订折叠的册」该要什么样的索引（服务折叠的索引，或最新修订投影），与 ADR-0144 越权风险点 3 同一前提，归 ADR-0144 owner 另记，不挡本票。
2. 给票 05（管理台接游标模式）：判断项 1（还没翻到的账户在翻页期间新增修订会跳到游标之前）与判断项 4（总数与本页不同快照）叠加，翻完所有页累计的行数可以不等于「共 N 条」；前端别拿两者相等作断言或提示。建议代记进票 05。

结论：可接受（无阻断）。

**处置**（推送方 · 通道 1）：评审无阻断，按 parallel-sessions「别人分支上的活怎么进 main」重放（作者与推送方同为通道 1，评审出自非作者通道 3）。非阻断 1 已办：
分支上补 `3ec807bb`，把迁移 0035 头注改成如实的说法——读面先按主键折叠、外层排序穿不过折叠，这条索引按决定七补齐了形状、眼下还没有读者；只改注释、SQL 语句不变，
此前未进 main，没有已施加它的库。索引本身该怎么配，按评审归 ADR-0144 owner 另记，本票不另立。非阻断 2 已代记进票 05 Comments。重放：在共享树 main `60d37d65`
之上 cherry-pick 为 `e4cfcfee`（← `77248909`）/ `9b55da64`（← `57bc23c3`）/ `4b26ac49`（← `56245f24`）/ `1aef333a`（← `cadcd364`）/ `04e5546f`（← `3ec807bb`），
无冲突；分支上的清点笔 `96f0bfda` 跳过、在批 tip 重生成。本票改过的文件除清点外与分支 tip 逐字一致。
