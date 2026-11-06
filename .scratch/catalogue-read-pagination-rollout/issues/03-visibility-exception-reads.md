# 03 visibility-exception 十四册迁到 ADR-0144

Category: enhancement
Status: in-progress——非作者评审（钉 `3dd97c5b1`）无阻断，四条非阻断已由派单方处理，待进 main（2026-11-06，分支 `cursor/crpr-03-visibility`，基 `d48c5b130`；完成记录、修订记录与评审记录见文末）。迁移编号 visibility-exception `0034`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/visibilityexception/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/visibility_exception` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/tracking-projections` | `OperationsProjectionRead.ListCurrent` | `derived_at DESC, parcel_ref DESC` |
| `/exception-case-records` | `CaseReviewRead.ListExceptionCases` | `established_at DESC, case_id` |
| `/exception-triage-records`（两种） | `TriageReviewRead.ListSignalEpisodes` / `ListDispositionRequests` | `started_at DESC, seq DESC` / `sent_at DESC, request_id` |
| `/claims-recovery-records`（三种） | `ClaimsRecoveryReviewRead.ListCustomerNotifications` / `ListClaimItems` / `ListRecoveryMatters` | `decided_at DESC, notification_id` / `submitted_at DESC, batch_ref, item_id` / `opened_at DESC, matter_id` |
| `/visibility-catalogues`（八种） | `CatalogueListRead.ListMilestoneMappings` … `ListConflictSignalRules` | 版本册按 `effective_from DESC, <版本>`；声明册按各自的键 |

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- 选择器分派的每种各是一册（声明各有册名），选择器键在声明里登为选择器，不兼作筛选维（决定四）。
- 追踪投影按包裹计量，是 spec「`total` 一律给精确值」点名的那类大册。

## 完成记录（2026-11-06，作者会话，分支 `cursor/crpr-03-visibility`，基 `d48c5b130`）

**落点**

| 笔 | 做了什么 |
|---|---|
| `ba550c620` | 各册声明（`ports/catalogue_pages.go`）；读端口收查询对象、答统一结果类型；postgres 读面改用落列取页（`adapters/postgres/catalogue_page.go` 的 `readPage`）；列表端点先 Intake 后解码、答复加 `page`（`adapters/http/catalogue_page.go`）；`cmd/parcel-api` 占位改签名；旧用例接统一结果，改同刻次序与自报租户那一格 |
| `8a661d835` | 真库翻页契约（`adapters/postgres/catalogue_pages_test.go`）、端点传输形状（`adapters/http/catalogue_pages_test.go`）；迁移 `0034_catalogue_read_pagination_indexes.sql` |
| `8e6c4379e` | 修订轮：追踪投影的定位参数、分诊与理赔追偿的 `registry`、可见性目录的 `kind` 挪到 Intake 之后读；端点用例补选择器与定位参数一表（配置渠道上的旧答、未配置时同答 403）；真库用例里按键登记的目录改把新行插在末尾，各册种的行改用自备构造器、一律 `SYN-`；注释里引 spec 的序数改成原句 |

**各册声明**（声明在 `ports/catalogue_pages.go`，落列在各读面头部；行标识取登记表主键去掉租户段，冲突信号规则除外，见判断项「冲突信号规则 ◑」）

| 册（路由 · 选择器） | 缺省序 | 可排维 | 筛选维 | q 覆盖 | 索引（`0034`） |
|---|---|---|---|---|---|
| 追踪投影（`/tracking-projections` 列表那一支） | `-derivedAt` | derivedAt、parcel | — | 包裹、版本 | 新 `tracking_projection_version_by_tenant_derived` |
| 异常案件（`/exception-case-records`） | `-establishedAt` | establishedAt、rootParcel | phase | 案件、单据号、根包裹、责任团队 | 新 `exception_case_by_tenant_established` |
| 信号发作期（`registry=signal-episode`） | `-startedAt` | startedAt、lastHitAt、parcel | outcome | 发作期、包裹、信号种类 | 新 `signal_episode_by_tenant_started` |
| 处置请求（`registry=disposition-request`） | `-sentAt` | sentAt | judgment | 请求、案件、案件单据号、动作 | 新 `disposition_request_by_tenant_sent` |
| 客户通知（`registry=customer-notification`） | `-decidedAt` | decidedAt | — | 通知、客户、发作期、渠道 | 新 `customer_notification_by_tenant_decided` |
| 索赔项（`registry=claim-item`） | `-submittedAt` | submittedAt | screen、conclusion | 批次、索赔项、单据号、客户、目标、索赔类型 | 新 `claim_item_by_tenant_submitted` |
| 追偿事项（`registry=recovery-matter`） | `-openedAt` | openedAt | — | 事项、案件、案件单据号、相对方 | 新 `recovery_matter_by_tenant_opened` |
| 区间型目录（`kind=MILESTONE_MAPPING` / `TRIAGE_RULE` / `DISCLOSURE_POLICY` / `EXCEPTION_DISCLOSURE_RULE`，各是一册） | `-effectiveFrom` | effectiveFrom | — | 版本、批准人 | 新 `<版本表>_by_tenant_effective` 各一条 |
| 按键登记的目录（`kind=NOTIFICATION_POLICY` / `CLAIM_ELIGIBILITY` / `CLAIM_AUTHORIZATION`，各是一册） | 键正序（policy / contract / customer） | 同缺省维 | — | 键与渠道 / 键与版本 | 主键（租户，键）同形，不重建 |
| 冲突信号规则（`kind=CONFLICT_SIGNAL_RULE`） | `signalKind` | signalKind | — | 信号种类、版本 | 主键（租户）：一租户至多一行 |

排序列与标识列在各建表迁移里都是 `NOT NULL`（spec「可排维与行标识的列必须非空」）。异常案件的单据号可空，管理台表格虽能按它排，不放出作可排维。

**完成判据**（钉 `8e6c4379e`）

- ✅ 真库用例（`SYN-` 夹具，带 DSN）：`adapters/postgres/catalogue_pages_test.go` 每册各过一遍——
  - 先种五行，按小页翻过第一页后插入一行，后面各页 `total` 由 5 变 6，拼起来不重出、不漏行：倒序册（按键登记的目录与冲突信号规则以外的各册）新行排在已翻过的那头、不进后面的页；正序册（按键登记的目录）新行落在末尾、在末页出现，原有五行各出现恰好一次。
  - 每册至少一条筛选或 `q` 的收窄断言 `total`（含维内为或、维间为与、`q` 与筛选相与）；`q` 里的 `%` 与 `_` 按字面，`total` 为 0。
  - 放出的排序维各按小页走一遍，次序如期、`total` 为 6。
  - 另一个租户答空页、`total` 为 0、无 `next`。
  - 冲突信号规则一租户至多一行：证那一行一页即末页（`total` 1、无 `next`），`q` 收窄断言 `total`（命中 1、不中 0），`%` 与 `_` 按字面，空租户 `total` 为 0；翻页途中插入无从演示，见判断项「冲突信号规则 ◑」。
- ✅ 端点用例：
  - `adapters/http/catalogue_pages_test.go` 按册表驱动：合规的一问带 `page`，`size` 取 Intake 页大小（Intake 给 13，答复即 13），`next` 与 `total` 原样带出，查询对象原样交给读口、选择器不进筛选；未知键、集外排序维、解不开的游标、词表外筛选值、换条件拿旧游标各答 400 带 `detail`，读口不被调用（换条件拿旧游标每册都证，同一游标在原条件下照收）；渠道未配置时合规与不合规的一问同答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`、答复逐字相同、读口不被调用。
  - 按选择器或定位参数分派的端点（追踪投影、分诊、理赔追偿、可见性目录）另有一表：配置了的渠道上，选择器缺席、取空、集外，定位参数取空或同时在场，照旧答不带 `detail` 的 400、读口不被调用（`TestSelectorEndpointsKeepTheirOwnRefusals`）；渠道未配置时这些坏请求与合规的几问（含按包裹、按版本取单件）同答一个 403、读口不被调用（`TestSelectorEndpointsAnswerTheUnconfiguredChannelBeforeReadingSelectors`）。把这几个端点临时换回 `5cecfde33` 的写法跑一遍，未配置那一表每个端点都红、`TestOperationsQueryRejectsAmbiguousLocators` 也红；换回本笔即绿。
  - 追踪投影按包裹、按版本取单件的两支带着集外参数照答 200、不带 `page`。
- ✅ 迁移无 CR、无 BOM（提交脚本按字节查过；`migrations` 行尾守卫过，钉 `8a661d835`）；修订轮未动迁移。
- 门（作者自验，钉 `8e6c4379e`，WSL go1.26.8）：`gofmt -l` 空；全仓 `go build`、`go vet` 退 0。带 DSN `-p 1 -count=1`：`internal/visibilityexception/adapters/postgres` 259 PASS（翻页契约各条用例在 `-v` 输出里都是 `--- PASS`，不是 SKIP），`adapters/http` 190 PASS，`application` 253 PASS，`domain` 103 PASS，其余各 adapters 包全过；`cmd/parcel-api` 654 PASS，`cmd/parcel-dispatch` 506 PASS，`cmd/parcel-ve-register` 35 PASS，`internal/architecture` 174 PASS，`internal/platform/cataloguepage` 130 PASS，`internal/platform/migrate` 8 PASS，`migrations` 1 PASS；以上 SKIP 全为 0。计数取证：`grep -c -- '--- PASS'` 与 `grep -c -- '--- SKIP'` 打在各包 `-v` 输出上；包集为本上下文全部包、`ports` / `adapters/http` / `adapters/postgres` 的直接引用方（含测试引用）与迁移两包。首轮各笔的门见 `5cecfde33` 的本节。

**判断项**

1. **先 Intake、后读一切查询参数**（spec「端点先 Intake、后解码查询参数」）：这一条在本票基 `d48c5b130` 之后才进 spec（推送方分支 `3a20b2bcc`）。首轮只把 ADR-0144 的解码挪到 Intake 之后，`registry` / `kind` 与追踪投影两个定位参数的校验留在 Intake 之前；修订轮照派单方要求一并挪后。于是未配置渠道上 `registry` 集外、`kind` 缺席、`parcel` 与 `version` 同时在场也答 403，与合规请求逐字相同（ADR-0055 决定三）；配置了的渠道上这些坏请求的答复不变。
2. **取单件的两支**：追踪投影按包裹、按版本取单件仍在解码之前分派，只是分派挪到了 Intake 之后；查找与答复形状没动，集外参数照旧不看。
3. **同刻收尾的方向**：异常案件、处置请求、客户通知、索赔项、追偿事项、区间型目录迁移前同刻按标识正序，现按 spec「行标识取登记表主键去掉租户那几段」与排序维同向（倒序）；信号发作期的同刻收尾从库内到达序 `seq` 换成发作期标识。旧用例里索赔项、追偿事项的夹具同刻提交，次序断言随之改成标识倒序（`case_review_test.go`）。
4. **会前移的排序列**：追踪投影的 `derivedAt` 与发作期的 `lastHitAt` 随重推导、再命中前移。倒序翻页时途中前移的行挪到已翻过的那头，本轮可能不再见到；两册都放出了正序，正序翻页时它挪到游标之后，可能在后面的页再出现一次（声明头注写明；评审指出首轮只写了倒序那一半）。
5. **大册**：追踪投影按包裹计量，信号发作期与客户通知按事件计量，都是 spec「`total` 一律给精确值」点名的那类册，`total` 照给精确值。追踪投影的索引建在版本表上，按它走会跨过被替代的历史版，由读面按当前标记联回时滤掉——历史版积压多时这条走法的代价请评审看一眼。另：它的 `q` 落在版本表的列上，每页数 `total` 都要联版本表；改落当前标记表上同义的列，数总数就只扫当前标记表。没有改，留作有量数据上看过执行计划之后的优化。
6. **管理台与脚本带的参数**：管理台只带 `registry`、`kind`（登为选择器）与追踪投影单件的 `parcel` / `version`（先分派、不解码）；`scripts/demo-seeds/demo-run/chain.sh` 只取 `/tracking-projections`、不带参数。答复只多一格 `page`，管理台不改照旧能用。选择器与定位参数传空值（`?registry=`、`?kind=`、`?parcel=`、`?version=`）迁移前就答不带理由的 400，现仍如此：它们不经本册声明解码，不会变成集外键的 400（`TestSelectorEndpointsKeepTheirOwnRefusals`）。
7. **自报参数从「被忽略」变「答 400」**：`operator_read_intake_test.go` 里经端点、查询串自报租户那一格，原断言被忽略照答，改为答 400 且不带行、读口不被调用，另补不带参数照答本租户（ADR-0144 决定四）。直接测 Intake 的那条（自报租户与页大小被忽略）是 Intake 自身的行为，不动。
8. **冲突信号规则 ◑**：登记表主键只有租户一段，去掉租户后不剩可作标识的列（偏离 spec「行标识取登记表主键去掉租户那几段」），行标识取行上的信号种类——一租户一行之下它在租户内唯一，任何次序排出来都是同一页。同一原因，「翻页途中插入新行」在这一册上无从发生；真库用例只证单行一页（`limit` 1 时 `total` 1、无 `next`）、`q` 收窄改 `total`（含 `%` 与 `_` 按字面）与空租户。信号发作期的登记表主键本就只有发作期标识一段、不含租户，行标识即取它，不算偏离。
9. **没有带计数的队列**：本票各册的读端口都答统一结果类型 `cataloguepage.Result`，没有自带 `Next` / `Total` 的计数队列，spec「已带计数与 `truncated` 的待办队列保留这两格」在本票无适用。
10. **册数**：票题写「十四册」，本票册表（钉 `d48c5b130`）列的是追踪投影、异常案件、分诊两种、理赔追偿三种、目录八种，合十五册（派单也点名八种目录）。按册表做了十五册，票题未改。
11. **`cmd/parcel-api/read_target_guard.go` 头注一句变得不实，不在本票地盘**：那里举例「/tracking-projections 两个定位参数同在即 400」，本票之后渠道未配置时答 403。交推送方在进 main 时改那一句。

**未验**

- 管理台未接（归 14）。
- 新索引只按形状对过缺省序，未在有量的库上取 `EXPLAIN` 证明被用上。
- 全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。

## 修订记录

**修订轮 ← 派单方（通道 1）· 本票钉 `5cecfde33`**：照派单说明「02 的非作者评审拦下过的」逐条自查，不 rebase，留在基 `d48c5b130`。

| 要求 | 处理（`8e6c4379e`） |
|---|---|
| 每个端点先 Intake，迁移前就有的选择器或参数校验也挪到其后；未配置用例补坏参数、坏选择器 | 对调追踪投影（`parcel` / `version`）、分诊与理赔追偿（`registry`）、可见性目录（`kind`）；异常案件迁移前就没有先于 Intake 的读取，不动。补 `TestSelectorEndpointsAnswerTheUnconfiguredChannelBeforeReadingSelectors`、`TestSelectorEndpointsKeepTheirOwnRefusals`，改写 `TestOperationsQueryRejectsAmbiguousLocators`（判断项 1、2） |
| 每册真库用例要有翻页途中插入新行、空册 `total` 为 0、一条筛选或 `q` 改变 `total` | 首轮各册已有；按键登记的目录缺省序是升序，新行原插在已翻过的那头，改插在末尾、在末页出现 |
| 端点表驱动：合规一问带 `page` 且 `size` 取 Intake 页大小，集外参数 400 带 `detail`，未配置时合规与不合规都 403；至少一个端点证换条件拿旧游标 400 | 首轮已按册表驱动，换条件拿旧游标每册都证；合规一问的 Intake 页大小改用不像缺省值的 13 |
| 注释与票面不引 spec 的取舍序号，不数别处的东西 | `ports/catalogue_pages.go` 与本票改引原句；HTTP 用例注释里会过时的册数改为不数 |
| 新夹具一律 `SYN-` | 真库翻页用例改用本文件自备的构造器，所种的行（含登记的目录条目与夹带的引用值）一律 `SYN-`；旧用例的夹具不回改 |
| JSON 数组上的 `q` 不用 `::text` | 本票各册 `q` 覆盖的列都是普通文本列，无此情形 |
| 既有参数传空值的旧行为保住 | 选择器与定位参数传空值迁移前答不带理由的 400，现仍如此，端点用例钉住（判断项 6） |
| 计数队列自带 `Next` / `Total`、行标识不取主键去租户的，写进判断项 | 本票没有计数队列（判断项 9）；行标识只有冲突信号规则不取主键去租户（判断项 8） |

## 评审记录

**评审 ← 子代理 · 钉 `3dd97c5b1`**：两轴无阻断。

| 评审意见 | 处理 |
|---|---|
| 非阻断：`TrackingProjectionPages`、`SignalEpisodePages` 头注与判断项 4 称「不会重出」，只对倒序成立；两册都放出了正序，正序时前移的行会跳到游标之后、重出一次 | 两处头注与判断项 4 改为「倒序可能漏、正序可能重出」 |
| 非阻断：`read_target_guard.go` 头注「/tracking-projections 两个定位参数同在即 400」因本票变假，文件不在本票地盘 | 判断项 11，交推送方进 main 时改 |
| 非阻断：追踪投影的 `q` 落在版本表上，每页数 `total` 都联版本表；改落当前标记表即可只扫一张表 | 判断项 5 记下，留作看过执行计划之后的优化 |
| 非阻断：判断项 5 只点名追踪投影，信号发作期、客户通知也按事件计量 | 判断项 5 一并点名 |

修订由派单方在本分支上做（作者会话已收工），只动两处头注与本票面；评审取证用的探针照旧有效，未另起复核。
