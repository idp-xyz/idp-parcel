# 03 visibility-exception 十四册迁到 ADR-0144

Category: enhancement
Status: in-progress——作者完成，待非作者评审（2026-11-06 作者会话，分支 `cursor/crpr-03-visibility`，基 `d48c5b130`；完成记录见文末）。迁移编号 visibility-exception `0034`（进 main 时按 tip 重核）
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
- 追踪投影按包裹计量，是取舍 4 点名的大册。

## 完成记录（2026-11-06，作者会话，分支 `cursor/crpr-03-visibility`，基 `d48c5b130`）

**落点**

| 笔 | 做了什么 |
|---|---|
| `ba550c620` | 各册声明（`ports/catalogue_pages.go`）；读端口收查询对象、答统一结果类型；postgres 读面改用落列取页（`adapters/postgres/catalogue_page.go` 的 `readPage`）；列表端点先 Intake 后解码、答复加 `page`（`adapters/http/catalogue_page.go`）；`cmd/parcel-api` 占位改签名；旧用例接统一结果，改同刻次序与自报租户那一格 |
| `8a661d835` | 真库翻页契约（`adapters/postgres/catalogue_pages_test.go`）、端点传输形状（`adapters/http/catalogue_pages_test.go`）；迁移 `0034_catalogue_read_pagination_indexes.sql` |

**各册声明**（声明在 `ports/catalogue_pages.go`，落列在各读面头部；行标识一律是主键去掉租户段，取舍 2）

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

排序列与标识列在各建表迁移里都是 `NOT NULL`（取舍 6）。异常案件的单据号可空，管理台表格虽能按它排，不放出作可排维。

**完成判据**

- ✅ 真库用例（`SYN-` 数据，带 DSN，钉 `8a661d835`）：`adapters/postgres/catalogue_pages_test.go` 每册落五行，按小页翻过第一页后插入一条排在最前的行，后续各页 `total` 由 5 变 6，拼起来恰是原五行按缺省序各一次（不重出、不漏）；筛选维与 `q` 各收窄出预期的 `total`（含维内为或、维间与、`q` 与筛选相与），`q` 里的 `%` 与 `_` 按字面；放出的排序维各走一遍；另一个租户答空页、`total` 为 0、无 `next`。冲突信号规则一租户至多一行，见判断项「冲突信号规则 ◑」。
- ✅ 端点用例（钉 `8a661d835`）：`adapters/http/catalogue_pages_test.go` 各册表驱动过一遍——合规的一问带 `page`，`size` 取 Intake 页大小，`next` 与 `total` 原样带出，查询对象原样交给读口、选择器不进筛选；未知键、集外排序维、词表外筛选值、换条件拿旧游标各答 400 带 `detail`，读口不被调用（同一游标在原条件下照收）；渠道未配置时参数写对写错同答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`、答复逐字相同、读口不被调用；追踪投影按包裹、按版本取单件的两支带着集外参数照答 200、不带 `page`。
- ✅ 迁移无 CR、无 BOM（提交脚本按字节查过；`migrations` 行尾守卫过）。
- 门（作者自验，钉 `8a661d835`，WSL go1.26.8）：`gofmt -l` 空；全仓 `go build`、`go vet` 退 0；`ba550c620` 单独在临时 worktree 上 `go vet` 本上下文与 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-ve-register` 退 0。带 DSN `-p 1 -count=1`：`internal/visibilityexception/adapters/postgres` 259 PASS，`adapters/http` 180 PASS，`application` 253 PASS，`domain` 103 PASS，其余各 adapters 包全过；`cmd/parcel-api` 654 PASS，`cmd/parcel-dispatch` 506 PASS，`cmd/parcel-ve-register` 35 PASS，`internal/architecture` 174 PASS，`internal/platform/cataloguepage` 130 PASS，`internal/platform/migrate` 8 PASS，`migrations` 过；以上 SKIP 全为 0。计数取证：`grep -c -- '--- PASS'` 与 `grep -c -- '--- SKIP'` 打在各包 `-v` 输出上；包集为本上下文全部包、`ports` / `adapters/http` / `adapters/postgres` 的直接引用方（含测试引用）与迁移两包。

**判断项**

1. **先 Intake 后解码**：spec 取舍 8 在本票基 `d48c5b130` 之后才进 spec（推送方分支 `3a20b2bcc`），本票照它做。
2. **选择器校验仍在 Intake 之前**：`registry` / `kind` 缺席或集外，迁移前就在 Intake 之前答不带理由的 400（端点头注称它是传输形状，与方法检查同级），本票不动；挪到 Intake 之后的只有 ADR-0144 的解码。于是未配置渠道上 `registry` 集外仍答 400——与迁移前同答，但与取舍 8「渠道未配置时对一切请求同答 403」字面不齐，请评审定是否一并挪到 Intake 之后（管理台只带合法的选择器值，挪了也不受影响）。
3. **同刻收尾的方向**：异常案件、处置请求、客户通知、索赔项、追偿事项、区间型目录迁移前同刻按标识正序，现按取舍 2 与排序维同向（倒序）；信号发作期的同刻收尾从库内到达序 `seq` 换成发作期标识。旧用例里索赔项、追偿事项的夹具同刻提交，次序断言随之改成标识倒序（`case_review_test.go`）。
4. **会前移的排序列**：追踪投影的 `derivedAt` 与发作期的 `lastHitAt` 随重推导、再命中前移；按它翻页时途中前移的行挪到已翻过的那头，本轮可能不再见到，但不重出（声明头注写明）。
5. **大册**：追踪投影按包裹计量（取舍 4 点名），`total` 照给精确值；它的索引建在版本表上，按它走会跨过被替代的历史版，由读面按当前标记联回时滤掉——历史版积压多时这条走法的代价请评审看一眼。
6. **管理台与脚本带的参数**：管理台只带 `registry`、`kind`（登为选择器）与追踪投影单件的 `parcel` / `version`（先分派、不解码）；`scripts/demo-seeds/demo-run/chain.sh` 只取 `/tracking-projections`、不带参数。答复只多一格 `page`，管理台不改照旧能用。
7. **自报参数从「被忽略」变「答 400」**：`operator_read_intake_test.go` 里经端点、查询串自报租户那一格，原断言被忽略照答，改为答 400 且不带行、读口不被调用，另补不带参数照答本租户（ADR-0144 决定四）。直接测 Intake 的那条（自报租户与页大小被忽略）是 Intake 自身的行为，不动。
8. **冲突信号规则 ◑**：一租户至多一行，「翻页期间插入新行」在这一册上无从发生；真库用例只证单行一页（`limit` 1 时 `total` 1、无 `next`）、`q`（含 `%` 与 `_` 按字面）与空租户。
9. **册数**：票题写「十四册」，本票册表列的是追踪投影、异常案件、分诊两种、理赔追偿三种、目录八种，合十五册（派单也点名八种目录）。按册表做了十五册，票题未改。

**未验**

- 管理台未接（归 14）。
- 新索引只按形状对过缺省序，未在有量的库上取 `EXPLAIN` 证明被用上。
- 全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。
