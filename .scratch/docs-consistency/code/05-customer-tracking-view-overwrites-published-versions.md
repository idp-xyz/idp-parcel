# 05 VE 客户全程追踪视图的已发布版本被原地覆盖，且每个投影新版本都换一代视图

Category: bug
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code05（基于 main `35d181d1c`）。原分级 R1（上线前必修）不变。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-01（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在，VE 代码在此期间未改）。

## 现象

1. **原地覆盖**：客户视图替代时 `UPDATE` 当前那一行，表主键是（租户，客户账户，包裹），前一版被改写，已发布的历史留不下。
2. **按投影版本换代**：只要投影版本变了就替代客户视图，四维内容没变也换一代，与「不与内部投影版本一一对应」相反。

## 证据

- 文档：
  - VE CONTEXT「客户可见性与通知」：「来源更正……必须重新派生客户视图并形成追加更正或明确替代关系。原已发布版本保留，但不再有效的内容不得继续显示为当前事实。」「客户全程追踪视图的版本由披露更正与替代关系驱动，不与内部投影版本一一对应；投影重新派生本身不要求形成新的客户视图版本」「已经向客户发布的里程碑、ETA、异常说明或通知出现更正时，必须以追加更新或明确替代关系表达，不得静默删除或改写历史披露。」
  - UC-VE-008 `AT-VE-161`：「形成追加更正/替代视图，原版本保留且不再作为当前结果」。
  - ADR-0065 Context 否决过同一形状（「指针在，靶子不在」），决定二：「原版本的留存以存下来为准，不以可重放为准」；其 Consequences 写「客户视图那一层不变：CONTEXT 既有硬句……原样适用」。
- 代码：
  - `internal/visibilityexception/adapters/postgres/customer_view.go` 的 `CustomerViews.Save`：替代分支 `UPDATE visibility_exception.customer_view SET version = …`，乐观锁按旧版本号。
  - `migrations/visibility_exception/0001_customer_view.sql`：`PRIMARY KEY (tenant_id, customer_account_id, parcel_id)`。
  - `ports.CustomerViewStore` 注释「原版本由替代关系承担历史，库只管当前」；`domain.RehydrateCustomerView` 注释「库里只有当前版本」。
  - `application.DeriveCustomerViewHandler.Handle`：`current.BasedOn() == command.Projection.Version()` 才跳过，否则 `current.Supersede(...)`。
  - 测试：内存替身用例 `TestANewerProjectionSupersedesTheCurrentViewKeepingHistory` 绿；真库用例 `TestSupersedeAdvancesTheVersionWithOptimisticLock` 只验指回、不验留存。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：迁移另取号，把 `customer_view` 改成版本表加当前标记（照 `0016_tracking_projection_versions.sql` 的投影形状）；`Save`、读回、`FindCurrent` 随改；派生编排加「内容或披露依据变了才换代」；存量行迁成各自的首版。

## 最小的诚实修法

1. 客户视图版本只增不改写：每一版一行，当前版另有标记或由最大版本派生；替代写新行并回指前一版。
2. 只在四维内容或披露依据变化时形成新版本；投影换版本、四维不变，只更新「所依据的投影版本」，不换代——这一格怎么存见「要裁的」。
3. 真库用例补「替代后前一版仍读得回」。

## 要裁的

- 四维未变、投影换版本时，「所依据的投影版本」记在哪：当前版上原地更新这一列（它不是对客发布的内容），还是不记、读时现取。推荐前者，作者可代决并写越权风险点。
- 票 [ve-customer-view-per-projection-version/01](../../ve-customer-view-per-projection-version/issues/01-does-customer-view-keep-a-generation-per-projection-version.md) 裁「不需要按投影版本留代」时引的前提「CONTEXT『原发布历史保留』已保证历史不丢」在代码里不成立；那张票的结论（不按投影版本留代）本票照守，前提已在那张票上补注。
- 产品文档 `docs/product/IDP-PARCEL-BUSINESS-PROCESS-AND-MODULE-GUIDE.md`「投影和客户视图只增版本」一句与代码不符，本票落地后即成真，不必改文档。

与 [06](./06-customer-view-shows-dimensions-without-a-visible-judgement.md) 改同一个派生编排，宜先后紧接。

## 完成记录（2026-10-21，分支 claude-code05）

用户授权原话「你作为业务和系统专家自决……解决所有的问题」；下列「代决」由作者会话代 VE owner 作出，待 VE owner 复核。

### 做了什么

- 迁移 `migrations/visibility_exception/0028_customer_view_versions.sql`：`customer_view` 换成版本表 `customer_view_version`（键：租户、客户账户、包裹、版本）加当前标记 `customer_view_current`（键：租户、客户账户、包裹；外键指名一行存下来的版本；另带「当前所依据的投影版本」一格），形状照 `0016_tracking_projection_versions.sql`。存量行各自迁成一条版本行并立为当前，旧表删去。
- `adapters/postgres` 的 `CustomerViews`：`Save` 先追加版本行、再立或移当前标记，乐观锁语义不变（首发撞已有当前、替代时当前不是前一版，都答 `ErrCurrentViewChanged`，同一事务回滚，不留从未成为当前的版本行）；新增 `FindVersion` 按版本读回（带账户与包裹，跨账户、跨租户与不存在同答）与 `Reaffirm`（只改标记上的依据，乐观锁看版本与原依据）。`FindCurrent` 走标记直取。
- `ports.CustomerViewStore` 加 `FindVersion`、`Reaffirm`，注释改为版本只增不改写；`domain.RehydrateCustomerView` 与快照注释里「库里只有当前版本」随改。
- 领域：`CustomerViewDimensions.Equal`；`CustomerTrackingView.Reaffirm(basedOn, dimensions)`——四维变了或投影没换都拒。
- 派生编排 `DeriveCustomerViewHandler.Handle`：四维与当前版相同时不签新身份、不换代，当前版 `Reaffirm` 跟上新投影，答 `EXISTING_RESULT`，意图照重放那样以同一视图版本再交一次；四维变了才 `Supersede`。
- 测试：真库 `TestASupersededVersionStaysReadableByVersion`（替代后前一版按版本原样读回、前版指回指着存下来的行、版本行两行、跨账户与跨租户按版本探针同不存在）、`TestReaffirmMovesOnlyTheCurrentBasis`（不多一行、不改版本行、过时依据被乐观锁拒）、`TestTheCustomerViewVersionMigrationCarriesExistingViewsAsTheirCurrentVersions`（存量迁移）、无事务负向证据并进 `TestWritesRefuseToRunOutsideATransaction`；编排 `TestANewerProjectionThatChangesTheDimensionsSupersedesTheCurrentViewKeepingHistory`、`TestANewerProjectionThatLeavesTheDimensionsUnchangedKeepsTheViewVersion`、`TestAFailedReaffirmIsUndecided`；领域 `TestReaffirmFollowsANewerProjectionOnlyWhileTheDimensionsStayTheSame`。`TestAPermittedFinalPendsUntilTheProjectionCarriesTheAdoptedFinal` 的第二拍随改为不换代。`cmd/parcel-dispatch` 里按表数行的用例改数 `customer_view_version`。
- 文档：UC-VE-008 `AT-VE-161` 的现状缺口注改为已补；票 ve-customer-view-per-projection-version/01 补 2026-10-21 补记。产品指南「投影和客户视图只增版本」一句本票落地后即成真，未改。

### 代决

1. **（评审后改写，协调方裁定）客户追踪响应体不带所依据的投影版本，客户的采用版本由 `version` 承担；形成时依据与确认依据在领域上分两格。** 版本行的 `based_on_projection` 是那一版形成时所依据的投影，一经写入不改，`CustomerTrackingView.BasedOn` 答它；当前标记上的那一格是当前版最近一次按之确认四维未变的投影，`ConfirmedOn` 答它，`Reaffirm` 只前移这一格，派生编排的幂等判断读它。`FindCurrent` 带出标记上的确认依据；`FindVersion` 读到的恰是当前版时同样带出，读到已被替代的版本时确认依据退回形成时依据。初稿把可变的那一格放在标记上、却仍由 `basedOnProjection` 对客答出，同一个 `version` 会对客户答出不同的投影版本——那就是改写已发布内容，只是位置挪到了标记上（评审 N1）。依据：VE CONTEXT「留存投影版本的按版本读回属运营追踪查阅，不属客户发布物」；docs 与 admin-web 都没有把 `basedOnProjection` 列为对客字段（2026-10-21 全仓 grep，只有 HTTP 适配器及其用例读写它）。
2. **换不换代只看四维（逐维三态加内容引用）是否相同。** 票面「内容或披露依据变化」里的「披露依据」，视图今天不存披露策略版本，可观察的只有策略逐维的答复；策略换版而四维答复不变，不换代。
3. **存量行的前版指回原样迁入，不抹成首版。** 指令里「迁成各自的首版」读作「各自成为版本表里存下来的第一行」。那一版确曾替代过别的版本、客户读到过 `priorVersion`；抹掉等于改写已发布的替代关系。它指的那一行读不回，是旧形状已经造成的事实，照 0016 对投影的处理如实留存。
4. **重新确认答 `EXISTING_RESULT`，不新增结果枚举。** 对客户而言当前版没变；意图仍由同一视图版本认领，`outboxintent.EnqueueOnce` 按信封 ID 去重，不多发一份。没有新信封，因此不涉及 `FingerprintEventID`，也没有新派发路由。
5. **`prior_version` 不设外键**，同 0016：存量行的前身读不回；新链条由「替代只在当前标记恰指前一版时成立、标记经外键只能指存下来的行」保证前身在库。

### 越权风险点

1. **对客 HTTP 契约删字段**：客户追踪响应体删去 `basedOnProjection`。首方 API 尚未发布兼容性基线（ADR-0140 Decision 五），今天删不破坏任何集成方；基线之后再删就是破坏性变更。客户再要「这一版依据哪份投影」，只能经运营追踪查阅，客户面答不出。客户面 HTTP 契约票 [code/22](./22-customer-face-http-contract-misses-accepted-adrs.md) 没有列这个字段（2026-10-21 查），不必在那张票上加说明。
2. 一版被替代之后，它曾经跟上过哪些投影（标记上的确认依据）随标记移动而不留；只留形成时那份。审计若要「每一版被哪些投影确认过」，要另加一张确认记录表。
3. 视图不存披露策略版本：策略换版、答复相同，不换代，事后也答不出某一版视图依据哪一版策略。若审计要这一格，要给版本行加列。
4. 存量行的悬空前版指针保留；按它 `FindVersion` 答查无。今天没有租户、没有生产数据，演示库重灌即无此类行。
5. 迁移删旧表 `customer_view`：仓内引用已全部改指新表（`cmd/parcel-dispatch` 里按表数行的用例）；仓外若有人直接读这张表（运维脚本、报表），会断。

### AT

- `AT-VE-161`：AT 盘点 ① 行改指新编排用例，并加真库持久化半边。总览计数未动，由协调方进 main 时重算。

### 验证（分支 claude-code05）

- `go build ./...`、`go vet ./...`、`gofmt -l ./cmd ./internal ./migrations` 干净。
- `scripts/test-full.sh ./internal/visibilityexception/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-ve-register/ ./internal/architecture/... ./migrations/... ./internal/platform/migrate/...`：21 个包 ok，退出码 0（`cmd/parcel-ve-register` 是改动包的反向依赖）。
- 一次性库 `code05seed` 上 `scripts/demo-seeds/seed.sh` 灌完（迁移链含 0028 施加通过），用完已删。演示种子数据未改。

## 评审记录

### 第 1 轮（2026-10-21，隔离评审；协调方转达，无阻断）

| 条 | 意见 | 处理 |
|---|---|---|
| N1 | 同一 `version` 会对客户答出不同的 `basedOnProjection`，按代决 1 的前提就是改写已发布内容 | 采纳协调方裁定 (a)：响应体删去 `basedOnProjection`；领域分出 `ConfirmedOn`，`BasedOn` 只答形成时依据；幂等判断读 `ConfirmedOn`；代决 1 与越权风险点 1 改写。code/22 没有列这个字段，未加说明。admin-web 与演示种子都不读它（grep 确认） |
| N3 | 测试缺口 | 补：乐观锁输家之后版本行数不变、输家签出的版本 `FindVersion` 查无；同号重写以 `customer_view_version_pkey` 主键冲突报错（`TestRewritingAStoredVersionIsRefusedByItsKey`）；`cmd/parcel-dispatch` 真库端到端 `TestASecondProjectionWithUnchangedDimensionsKeepsTheCustomerViewVersion`（版本 1 行、确认依据前移到 P2、形成时依据仍是 P1、意图 1 份）与 `TestASecondProjectionThatChangesTheDimensionsSupersedesTheCustomerView`（版本 2 行、指回前版、意图 2 份）；迁移用例比全四维、内容与发布时间，并证标记外键拒绝悬空版本；内存替身注释写明真库用例才是凭据 |
| N4 | 旧投影迟到的竞态窗口 | 不在本票，另立 [code/50](./50-customer-view-derivation-can-be-carried-back-by-a-concurrent-stale-projection-envelope.md)，建议 R2 |

验证（修订后，同一分支）：`go build ./...`、`go vet ./...`、`gofmt -l` 干净；`scripts/test-full.sh ./internal/visibilityexception/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/... ./migrations/...` 19 个包 ok，退出码 0。

### 第 2 轮（2026-10-21，隔离评审；协调方转达，无阻断，变异测试 M1–M6 均被抓住）

| 条 | 意见 | 处理 |
|---|---|---|
| N-a | 「所依据的投影」有几处说法没跟上新语义 | `DeriveCustomerViewHandler.Handle` 头注、`TestANewerProjectionThatLeavesTheDimensionsUnchangedKeepsTheViewVersion` 与 `TestAFailedReaffirmIsUndecided` 头注、UC-VE-008 `AT-VE-161` 缺口注都改为「只前移确认依据（`ConfirmedOn`）」 |
| N-b | AT 盘点 ④ 不另开「现状」惯例 | `AT-VE-158` 那一行还原成 main 原文；现状只留在 UC-VE-008 缺口注与 code/06 票面 |
| N-c | 四维不变用例补重投 | `TestASecondProjectionWithUnchangedDimensionsKeepsTheCustomerViewVersion` 末尾以新信封 ID 把 P2 的派生信封交给生产的客户视图派生那一路，再直接问一次租户现绑的派生编排：答 `EXISTING_RESULT`、意图已交出，版本行仍 1 行、确认依据仍是 P2、意图仍 1 份 |
| N-d | 包数按实况写；补跑反向依赖与迁移执行器 | 第 1 轮那一跑的清单不含 `cmd/parcel-ve-register` 与 `internal/platform/migrate`，19 个包是那份清单的实况；本轮按全清单补跑，见下。code/50 的 Blocked by 改为「无（建在 code/05 的 `ConfirmedOn` 之上，随 code/05 进 main）」 |

验证（第 2 轮修订后）：`go build ./...`、`go vet ./...`、`gofmt -l cmd internal migrations tools` 干净；`scripts/test-full.sh ./internal/visibilityexception/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-ve-register/ ./internal/architecture/... ./migrations/... ./internal/platform/migrate/...` 21 个包 ok，退出码 0。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-code05`（code/05、06 同一分支）tip `318000920` 在 main `35d181d1c` 上快进；两轮隔离评审无阻断，第二轮附变异测试（M1–M6 均被用例抓住）。
- 协调方在 tip 上单独提交重生成的机制清点；ADR 索引与 AT 计数 `-check` 退 0。
- 全量 `scripts/test-full.sh` 退 0，158 个包 ok；`go build ./...`、`go vet ./...` 干净；演示种子在一次性库上灌通（迁移 `visibility_exception/0028` 一并施加），用后删库。
- 去向：code/50（并发旧投影信封带回旧依据，R2）、code/51（ETA 客户可见性判断，Blocked by code/07，R2）。
