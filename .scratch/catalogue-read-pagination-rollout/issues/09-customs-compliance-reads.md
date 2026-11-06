# 09 customs-compliance 十六册迁到 ADR-0144

Category: enhancement
Status: resolved——2026-11-06 进 main（`098767b1e`），记录见文末「进 main 记录」
Blocked by: 01
地盘：`internal/customscompliance/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/customs_compliance` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/customs-case-records` | `CustomsCaseRecordRead.ListCustomsCases` | `c.case_id`（另有按案件的窄口参数） |
| `/customs-submission-records` | `SubmissionRecordRead.ListSubmissionVersions` | `unit_id, procedure_ref, fixed_at, version_id` |
| `/customs-external-result-records` | `ExternalResultViewRead.ListExternalResults` | `recorded_at, source_id` |
| `/customs-action-admission-judgements` | `ActionAdmissionJudgementRead.ListActionAdmissionJudgements` | `judged_at DESC, judgement_id` |
| `/customs-unadmitted-objects` | `UnadmittedObjectRead.ListUnadmittedObjects` | `action_at, object_ref`，带计数与 `truncated` |
| `/customs-compliance-rules`（两种） | `RuleCatalogueRead.ListCaseRequirementRules` / `ListInterpretationRules` | `jurisdiction_ref, direction, procedure_ref` / `result_layer, jurisdiction_ref, applies_from DESC` |
| `/customs-case-registers`（三种） | `CaseRegisterCatalogueRead.ListReadinessJudgments` / `ListSubmissionAuthorities` / `ListClosureObligations` | `unit_id` / `unit_id` / `case_ref` |
| `/customs-gate-conditions` | `GateConditionCatalogueRead.ListGateConditions` | `scope_ref, action, boundary_ref` |
| `/customs-ports-paths`（两种） | `PortsPathsCatalogueRead.ListCandidatePorts` / `ListDeclarationPaths` | `port_ref, applies_from DESC` / `path_ref, applies_from DESC` |
| `/customs-credentials` | `CredentialCatalogueRead.ListCredentials` | `credential_id` |
| `/customs-duty-collaborations` | `DutyCollaborationCatalogueRead.ListDutyCollaborations` | `scope_ref, duty_ref` |
| `/customs-duty-verifications` | `DutyVerificationCatalogueRead.ListDutyVerifications` | `duty_ref, funds_ref, scope_ref, verified_at, version_digest` |

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- 今天首列与决胜键方向不一致的（`applies_from DESC` 那几册）照 spec「与排序同向作决胜键」改为同向，判断项点名。
- 按案件、按版本的窄口参数登为筛选维或选择器，不留成未声明的键（决定四「未知参数键即拒」）。
- 未准入对象带计数，照 spec「已带计数与 `truncated` 的待办队列保留这两格」。

## 完成记录（2026-11-06，作者会话，分支 `cursor/crpr-09-customs`，基 `d48c5b130`）

**落点**

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `93bd03a1c` | `ports/catalogue_pages.go`（新）、`ports/operations_records.go`、`ports/ports.go`；`adapters/http/catalogue_page.go`（新）、各列表端点、`receive_external_result.go`；`adapters/postgres/catalogue_page.go`（新）与各读面；`cmd/parcel-api` 的占位与替身；各包旧用例 | 各册声明；读端口改收查询对象、答统一结果类型（卡关对象外加积压数）；读面经落列声明取页，total 与本页同一组条件；端点经本册声明解码，答复加 `page`，问题体加 `detail` 一格。这一笔的端点仍是先解码后 Intake（提交说明也这样写），由 `9f7371418` 改正 |
| `fd1a04490` | `adapters/http/catalogue_pages_test.go`、`adapters/http/catalogue_page_helpers_test.go`、`adapters/postgres/catalogue_pages_test.go`（均新） | 表驱动的端点用例与真库翻页用例，各册各登一次 |
| `1bba36e9c` | `migrations/customs_compliance/0033_catalogue_read_pagination_indexes.sql` | 新建与缺省序同形的索引；已有同形主键或唯一约束的册在头注点名、不重建 |
| `9f7371418` | 各列表端点、`adapters/http/catalogue_page.go`、`adapters/postgres/operations_record_views.go`、两份端口声明、端点与真库用例 | 端点改先 Intake 后解码，按 registry 分派的路由连分派校验一并挪到 Intake 之后；窄口参数空值照旧落整册；`q` 在包裹号数组上只比元素；注释改引规则原句；补对应用例 |
| `a14c8d1e6` | 端口声明、几处读面与用例的注释 | 注释不数别处的东西，改为点名 |
| 本笔 | 本票面；`ports/catalogue_pages.go` | 完成记录；「做法与判据」两句改引规则原句；就绪判断声明头注里数答复体字段的那句改为点名 |

`cmd/parcel-api` 里：`unwired_orchestration.go` 只改关务各读口占位方法的签名（规则库、案件登记、门禁、口岸路径、凭证、税费协作与核对），邻行未动；`endpoints_customs_records_test.go` 里记录各口与卡关对象的占位、`assemble_customs_registration_test.go` 的真库装配用例（读回前先解首页查询）同样只跟签名。共用件 `internal/platform/cataloguepage` 未改。

**各册声明**（声明在 `ports/catalogue_pages.go`，落列在各读面头部；缺省序都取迁移前 `ORDER BY` 的首列与方向，声明文件的头注写明为什么都不是 `-registeredAt`）

| 册（路由 · 选择器） | 缺省序 | 可排维 | 筛选维 | q 覆盖的列 | 索引 |
|---|---|---|---|---|---|
| 关务案件 `/customs-case-records` | `case` | case、establishedAt | case（自由取值）、direction | `case_id`、`document_number`、`jurisdiction_ref`、`procedure_ref`、`obligation_ref`、`parcels` 里逐个包裹号 | 已有 `customs_case_id_unique` |
| 提交版本 `/customs-submission-records` | `unit` | unit、fixedAt | version（自由取值）、isCurrent | `version_id`、`unit_id`、`procedure_ref`、`members` 里逐个包裹号 | 新 `declaration_submission_by_tenant_unit` |
| 外部监管结果 `/customs-external-result-records` · `version` | `recordedAt` | recordedAt | layer | `source_id`、`submission_version`、`claimed_version` | 新 `external_result_by_tenant_recorded` |
| 动作准入判断册 `/customs-action-admission-judgements` | `-judgedAt` | judgedAt | object（自由取值）、answer | `judgement_id`、`object_ref`、`node_ref`、`onward_node_ref` | 新 `action_admission_judgement_by_tenant_judged`（标识同向倒序） |
| 卡关对象 `/customs-unadmitted-objects` | `actionAt` | actionAt | answer（只收不过门的词） | `object_ref`、`judgement_id`、`node_ref`、`onward_node_ref` | 派生集合，派生那一步走已有 `action_admission_judgement_by_object` |
| 建案要求规则 `/customs-compliance-rules` · `registry=case-requirement` | `jurisdiction` | jurisdiction | direction | `jurisdiction_ref`、`procedure_ref`、`basis` | 已有 `case_requirement_rule_pkey` |
| 解释规则 `/customs-compliance-rules` · `registry=interpretation` | `layer` | layer、appliesFrom | layer | `jurisdiction_ref`、`rule_ref` | 已有 `interpretation_rule_pkey`（生效起点改升序后同形） |
| 就绪判断 `/customs-case-registers` · `registry=readiness` | `unit` | unit、judgedAt | — | `unit_id`、`basis_ref`、`revoked_by` | 已有 `readiness_judgment_pkey` |
| 提交授权 `/customs-case-registers` · `registry=submission-authority` | `unit` | unit、grantedAt | — | `unit_id`、`authority_ref`、`revoked_by` | 已有 `submission_authority_pkey` |
| 关闭义务目录 `/customs-case-registers` · `registry=closure-obligation` | `case` | case、registeredAt | — | `case_ref` | 已有 `closure_obligation_catalog_pkey` |
| 门禁条件册 `/customs-gate-conditions` | `scope` | scope、registeredAt | action | `scope_ref`、`boundary_ref` | 已有 `gate_condition_catalog_pkey` |
| 口岸目录 `/customs-ports-paths` · `registry=candidate-port` | `port` | port、appliesFrom | — | `port_ref`、`display_name` | 已有 `candidate_port_pkey`（生效起点改升序后同形） |
| 申报路径目录 `/customs-ports-paths` · `registry=declaration-path` | `path` | path、appliesFrom | direction | `path_ref`、`port_ref`、`declaration_mode` | 已有 `declaration_path_pkey`（同上） |
| 监管凭证册 `/customs-credentials` | `credential` | credential、registeredAt | — | `credential_id`、`issuer_ref`、`holder_ref`、`procedure_ref` | 已有 `regulatory_credential_pkey` |
| 税费付款协作事项册 `/customs-duty-collaborations` | `scope` | scope、formedAt | kind | `scope_ref`、`duty_ref`、`obligor_ref` | 已有 `duty_payment_collaboration_pkey` |
| 税费付款核对册 `/customs-duty-verifications` | `duty` | duty、verifiedAt | coverage、delta、validity | `duty_ref`、`funds_ref`、`scope_ref` | 新 `duty_payment_verification_by_tenant_duty` |

没有筛选维的册（就绪判断、提交授权、关闭义务目录、口岸目录、监管凭证册）各在声明头注写了为什么不放：答复体没有状态一格，或状态在嵌套项上。

**完成判据**（代码钉 `a14c8d1e6`；本笔在代码上只改一句注释）

- ✅ 真库用例（`SYN-` 夹具，带 DSN；`adapters/postgres/catalogue_pages_test.go`，各册在 `pagedRegisters` 里各登一次，下列每条用例对每册各过一遍）：
  - `TestCustomsCataloguesPageByCursorWithoutRepeatsOrGapsWhileRowsArrive`：本租户播四行、他租户播一行，两行一页翻完第一页后插两行，一行落在已翻过的那头、一行落在未翻的那头；第二页不重出第一页的行，未翻那头的新行照样翻到，total 由 4 到 6，末页 next 为空，他租户的行不进来。
  - `TestCustomsCatalogueTotalsFollowTheFiltersAndTheKeyword`：q 只命中一行时 total 为 1，`%` 按字面零命中；有筛选维的册另证一个取值、维内为或、维与 q 为与，total 都与本页出自同一组条件；空租户答空页且 total 为 0。
  - `TestTheUnadmittedBacklogStaysWholeUnderFiltersAndTheKeyword`：卡关对象的积压数不随筛选与 q 变；本页一件也没有时，积压数与 total 照样答出。
  - `TestTheKeywordOnParcelArraysMatchesElementsOnly`：案件的 `parcels` 与提交版本的 `members` 各过一遍——查 `"`、`,`、`[` 与两个包裹号首尾相接拼出的串都零命中，按第二个包裹号查到那一行且只算一行。
- ✅ 端点用例（`adapters/http/catalogue_pages_test.go`，各册在 `pagedCatalogues` 里各登一次）：
  - `TestCustomsCataloguesHandTheDecodedQueryToTheReader`：排序、筛选、q 与游标齐给的合规一问原样交给读口，租户与页大小取自接入面；答复带 `page`，`size` 是接入面给的页大小。
  - `TestCustomsCataloguesRefuseQueriesOutsideTheirDeclaration`：自报 `tenant`、自报 `limit`（都是集外键）、集外排序维、解不开的游标、换了排序的旧游标、换了 q 的旧游标，有封闭词表维的册另加词表外筛选值，各答 400 `MALFORMED_REQUEST` 带 `detail`，读口不被调用。
  - `TestUnconfiguredChannelsAnswerEveryCatalogueQueryIdentically`：渠道未配置时，写对的一问与集外排序维、集外键、解不开的游标、词表外筛选值，按 registry 分派的路由另加缺 registry 与集外 registry，全答 403、答复逐字相同，读口不被调用。
  - `TestARegistryCursorDoesNotCarryOverToAnotherRegistry`：同一路由上一册的游标换个 registry 去翻答 400，回原册照样接着翻。
  - 窄口参数与卡关对象另有用例，见判断项「窄口参数」「卡关对象」。
- ✅ 迁移按字节无 CR、无 BOM：`0033` 的 CR 行数为 0、无 BOM、以 LF 结尾；`migrations` 包的守卫过。
- 门（作者自验，WSL go1.26.8）：改过的 go 文件 `gofmt -l` 为空；全仓 `go build ./...`、`go vet ./...` 退 0（钉 `9f7371418`；其后 `a14c8d1e6` 只改注释，`go vet ./internal/customscompliance/...` 退 0）。带 DSN（`IDP_PARCEL_POSTGRES_DSN`）跑 `go test -count=1 -p 1 -v`，全部 0 SKIP、0 FAIL：
  - `internal/customscompliance/...` 1329 PASS（钉 `a14c8d1e6`；其中 `adapters/postgres` 444、`adapters/http` 366、`application` 266、`domain` 187）；
  - 反向依赖（`go list -deps` 反查，钉 `9f7371418`）：`cmd/parcel-api` 654、`cmd/parcel-dispatch` 506、`cmd/parcel-customs-register` 42；`internal/networkrouting/adapters/customscompliance` 12、`internal/parcelshipment/adapters/customscompliance` 4、`internal/settlementaccounting/adapters/customscompliance` 12、`internal/transportfulfillment/adapters/customscompliance` 11、`internal/visibilityexception/adapters/customscompliance` 74；
  - `internal/architecture/...` 174、`migrations` 1、`internal/platform/migrate` 8。
  - 计数取证：各包 `-v` 输出上 `grep -c -- '--- PASS'` 与 `grep -c -- '--- SKIP'`，子测试一并计入。

**判断项**

1. **窄口参数**（管理台与脚本带的参数怎么声明的）。管理台 `apps/admin-web/src/pages/customs/api.ts` 实际带的只有：按 registry 分派的路由带 `registry`，动作准入判断册在选了对象时带 `object`（没选不带）；其余各册不带参数。`scripts/` 不调这些读口；`cmd/parcel-api` 的用例带非空 `case`，路由探针带 `registry`。
   - `registry` 登为选择器：每个取值各是一册、各有册名，取值进游标摘要，一册的游标换个 registry 去翻答 400。
   - 案件的 `case`、提交版本的 `version`、判断册的 `object` 登为自由取值的筛选维。迁移前是单值精确收窄，迁移后可给多值、维内为或。迁移前空值即不收窄，端点在解码前去掉这几个键上的空值，`?case=` 照旧落整册，不答「筛选维的取值为空」；与有值的同给时只按有值的收窄（`TestBlankNarrowingStillListsTheWholeCatalogue`）。
   - 外部监管结果的 `version` 登为选择器：原样交给读口、空即不收窄，取值进摘要（`TestTheExternalResultVersionStaysASelector`）。答复体里那一格叫 `submissionVersion`，筛选维名只取答复体的字段名（决定四），所以不当筛选维。
   - 答复只多一格 `page`，管理台不改也照旧能用。
2. **决胜键改同向**。解释规则、口岸目录、申报路径目录迁移前是首列升序、`applies_from DESC`，迁移后 `applies_from` 随首列升序：同一键下有多版时旧版排在前，迁移前是新版在前。管理台规则页与口岸路径页按答复次序上列、不在前端重排，这几行的上下次序在页面上看得见。动作准入判断册迁移前是 `judged_at DESC, judgement_id`（标识升序），迁移后标识随首列倒序，只在判断时刻完全相同时次序不同。
3. **行标识不是「主键去掉租户」的**。关务案件取唯一约束 `customs_case_id_unique` 里的案件标识，不取主键（辖区、方向、程序、义务）：主键是「同一法律行为一案」的幂等键，案件标识在租户内同样唯一，又是迁移前 `ORDER BY` 的唯一列和答复里引用案件的那一格。提交版本与税费付款核对在主键各段之外插一段固定时刻、核对时刻：版本号随机签发、版本指纹是内容摘要，都不带先后，插上时刻才保住迁移前的次序；这两册因此各新建一条索引，主键不动。
4. **按包裹、事件计量的大册**。动作准入判断册（每次判断一行）、外部监管结果（每条回执一行）、提交版本（每个申报单元逐版一行）照给精确 total，超出租户级规模时回 ADR-0144 改成 `null`。卡关对象每次取页都从全部判断派生一遍，积压大时取页与计数的代价随判断条数涨（见未验）。
5. **卡关对象**。答复体嵌统一结果类型：`ports.UnadmittedObjects` 是 `cataloguepage.Result` 外加 `Count`，Next 与 Total 都来自 `Result`。`unadmittedCount` 是整个队列的积压数，不随筛选与 q 变，与本页同一条语句求，本页为空时另问一句只数件数的；`page.total` 是这一组条件下的件数；`truncated` 即 `page.next` 不为 `null`（`TestTheUnadmittedCountIsTheWholeBacklogWhilePageTotalFollowsTheConditions`）。对象的排序值是它最近一次判断的动作时点：翻页期间同一对象再被判断会挪到后面，可能在后面的页重出一次；这一点也写进了 `UnadmittedObjectPages` 的声明头注。
6. **从「被忽略」变「答 400」的参数**。迁移前各口只读 `registry` 与窄口参数，其余查询参数一概静默忽略；迁移后集外键答 400（ADR-0144 决定四）。受影响的有请求自报的 `tenant`、`limit`——旧用例原在查询串里带它们并断言被忽略，改为不带、断言租户与页大小取自接入面，拒绝由 `TestCustomsCataloguesRefuseQueriesOutsideTheirDeclaration` 逐册证；还有案件登记口的 `cutoff` 与口岸路径口的时点参数，两口头注早裁定不收，迁移前同样被静默忽略。
7. **registry 校验挪到 Intake 之后**。未配置的渠道上，缺 registry 或集外 registry 迁移前答 400，迁移后与合规一问同答 403；配置了的渠道上照旧答 400。
8. **total 与本页是两条语句**（卡关对象除外）。没开只读快照，两次读之间有写入时 total 与本页可能差一；卡关对象的两个件数与本页同一条语句求。
9. **`93bd03a1c` 的提交说明**写的是先解码后 Intake，与现行口径相反；`9f7371418` 改正并在提交说明里写明。
10. **就绪判断与提交授权没放状态维（偏离 ADR-0144 决定四）**：两册有「失效与否」这一状态，答复体却没有状态一格，筛选维无名可取。今天没有调用方按状态筛这两册，不是回归；
   连同 04、05 同类的两处收进 16 号票统一处理（能在 SQL 里由本册的行导出就补上，导不出就回 ADR-0144 改决定四的适用范围），不在本票豁免。

**未验**

- 没在租户级数据量上跑 `EXPLAIN`：新索引与点名的主键、唯一约束是否被选中，卡关对象派生在大积压下的代价，都没有实测。
- 管理台没在浏览器里点过；管理台改造归 14。
- 并发写入下 total 与本页的一致性没有用例。
- 全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。

## 评审记录

**评审 ← 子代理 · 钉 `2f952629a`**：两轴无阻断。

| 评审意见 | 处理 |
|---|---|
| 非阻断：就绪判断、提交授权有「失效与否」状态却没放状态维，等于在代码里豁免 ADR-0144 决定四，应交 ADR 定 | 判断项 10；收进 16 号票，与 04、05 同类两处一并处理 |
| 非阻断：三处新写的 `grantedCatalogueIntake{tenant: "TENANT-1"}` 不带 `SYN-` | 三条 registry 拒绝用例里改为 `SYN-TENANT-1`；旧用例里的旧夹具不回改 |
| 非阻断：卡关对象翻页时被重判的对象可能在后面的页重出一次，应写进声明头注 | 补进 `UnadmittedObjectPages` 头注 |

修订由派单方在本分支上做（作者会话已收工），只动上面三处与本票面；评审取证用的探针照旧有效，未另起复核。

## 进 main 记录（2026-11-06，通道 1 推送）

**出处**：分支 `cursor/crpr-09-customs`，tip `00f390657`。**评审**：钉 `2f952629a` 两轴无阻断。

**main 上的 SHA**：`098767b1e`（重放到当时的 `origin/main` `0874218c9` 之上：各分支逐笔 cherry-pick（带 `-x`，出处在提交说明里），推送方补的几笔见下）。

| 步骤 | 结果 |
|---|---|
| 重放 | 01/02 分支与 03–13、15、14a 各分支共 106 笔，无冲突 |
| 推送方补的几笔 | `de1fb75d8` AT-TF-333 改指新用例名；`66a689cbc` `cmd/parcel-api` 三处注释改成现状；`29af46c24` 删掉 15 留的两个别名；`6aff3149a` 管理台路线计划契约用例认 `page`；`446252dc4` 重生成机制清点；`098767b1e` 01 与 05 叠合处的用例 |
| 生成物 | `go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 在 tip 上退 0；机制清点在 `446252dc4` 重生成，tip 上再生成无差异 |
| 全量真库测试 | `scripts/test-full.sh` 对 `098767b1e` 退 0（175 个包 ok、17 个包无测试文件）。前一跑对 `446252dc4` 红一条：01 的 NUL 补件与 05 的挡回册判据用例叠合，`098767b1e` 修后重跑全绿 |
| 种子灌一次性库 | 升级路径：`0874218c9` 的代码灌库 A，本批代码对库 A 迁移退 0（施加 11 条新迁移）；本批代码灌空库 B 退 0；两库用后删 |
| 管理台三道门 | tsc 退 0、run-tests 1236 条全过、vite build 退 0 |
| 推确切 SHA | `git push origin 098767b1e:main`，`origin/main` 等于 `098767b1e` |
