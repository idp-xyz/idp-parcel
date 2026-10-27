# 07 VE ETA 存储整行覆盖，键里没有来源维，两种来源的预测互相覆盖

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code07（基 origin/main `efab73e87`）。此前 ready-for-agent——R3（不急）：`FormETAHandler` 在 `cmd/` 下没有调用方，今天不上生产路径；但谁来接线都会继承这个错，接线之前必须先修（接线即升 R1：历史预测丢失、来源混淆）。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-11（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

ETA 按（租户，包裹，里程碑）一行，刷新整行改写，历史预测留不下；键里没有来源这一维，承运商预测与运营企业派生预测会互相覆盖。存储形状与 [05](./05-customer-tracking-view-overwrites-published-versions.md) 同属 ADR-0065 否决过的「库只管当前」，但多一层来源维缺失，且未接线、分级不同，单独成票。

## 证据

- 文档：
  - VE CONTEXT「ETA 与可见性缺口」：「承运商提供的预测与运营企业派生的预测分别保留来源和口径。新的 ETA 形成新版本，不覆盖历史预测」。
  - GLOSSARY「预计到达时间（ETA）」：「新的 ETA 不覆盖历史」。
  - UC-VE-003 `AT-VE-051`：「两个来源都提供 ETA | 来源和口径分别保留」；`AT-VE-057`：「原判断保留」。
- 代码：
  - `internal/visibilityexception/adapters/postgres/eta_gap_notification.go` 的 `ETAs.Save`：注释「同（租户+包裹+里程碑）整行更新——刷新不是第二行」，SQL `ON CONFLICT (tenant_id, parcel_ref, milestone_ref) DO UPDATE`。
  - `ports.ETAStore` 注释：「历史版本由 Refresh 的指回关系承担，库只管当前」。
  - `application.FormETAHandler.FormETA` 按（租户，包裹，里程碑）`FindCurrent`，取到的可能是另一种来源的预测，再 `Refresh` 盖掉它；同函数头注「历史预测不覆盖」与存储行为相反。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：迁移另取号，ETA 改版本表、键加来源维；`FindCurrent` 按来源取；改正两处注释。

## 最小的诚实修法

照 `0016_tracking_projection_versions.sql` 的形状：每一版一行、只增不改写，当前版按（租户，包裹，里程碑，来源）各一条；刷新写新行并回指同来源的前一版。

## 要裁的

无（CONTEXT 已定）。对客可见判断从哪一来源取由 UC-VE-003 既有规则判，不在本票。

## 完成记录（2026-10-27，分支 claude-code07）

用户授权原话「你作为业务和系统专家自决」；下列「代决」由作者会话代 VE owner 作出，待 VE owner 复核。

### 分诊

票面描述核实仍属实：`eta_prediction` 键是（租户，包裹，里程碑），`Save` 整行 `DO UPDATE`，被指回的前一版随即被覆盖；键里没有来源，承运商与运营企业两路预测互相盖；`FormETAHandler` 在 `cmd/` 下仍无生产调用方（只有测试与 `cmd/parcel-dispatch` 的判断那一路消费它入队的信封）。code/51（已进 main）的判断链读「当前那一版」，两路来源互盖时判断引用的可能是后写的那一路——本票按任务单的连带要求一并重定候选选取。

范围裁定：存储（版本表加当前标记，键含来源）、写路径的并发纪律、判断链的候选选取、存量迁移、用例与文档。不做：接线（见「接线还差什么」）、规则册登记口等 code/51 清单里与存储无关的项。

### 做了什么

- 迁移 `0032_eta_prediction_versions.sql`：`eta_prediction_version`（每个版本一行、写入后不改动，租户内版本号唯一，线内输入版本唯一）与 `eta_prediction_current`（当前标记，键含来源，唯一可变处，五维外键只能指本线存下来的版本）。存量行迁成各线的当前版，旧的前身指回如实带过来；旧表退场。预检：同租户下版本号重复时停下，报错指明租户与版本号，停下时旧表原样。
- 领域：`ETAPrediction.Refresh` 要求同来源（预测线的键是包裹、里程碑、来源）；新增 `CandidateETAFrom`（候选选取）。
- 端口：`ETACurrentRead` 改为 `FindCurrent`（带来源）与 `ListCurrent`（一条语句读出各来源线的当前版）；`ETAStore` 增 `LockCurrent`（线锁加读）、`FindVersion`（按版本读回留存的任一版）；新增哨兵 `ports.ErrETACurrentChanged`。头注写清锁序与事务要求。
- 适配器 `postgres.ETAs`：`Save` 先取线锁、核条件、核完才写版本行并移标记（首版 `ON CONFLICT DO NOTHING`、刷新 `UPDATE ... WHERE version_id = 前版` 留作最后一道），条件不成立什么都不写、交回 `ErrETACurrentChanged`；`LockCurrent` 用事务级咨询锁（锁键含四维，口径同 TF 的 `BookingAnswers.FindAnswersForUpdate`），走 `RequireExecutor`。
- 编排：`FormETAHandler` 以 `LockCurrent` 取线锁再决定重放、刷新或首版，重放认本线上曾经出现过的输入版本（`FindByInputs`，线内输入版本唯一）；并发输家落新增的 `ETACurrentChanged` 一格（与预测库读不动的 `ETAStoreUnavailable` 分开：恢复动作是重读再走）。`JudgeETAVisibilityHandler` 以 `ListCurrent` 读出各线当前版、先取规则再用 `CandidateETAFrom` 按来源优先序选第一个达标的当前版作候选；其余（判重放、替代指回、AT-VE-214 的目标里程碑）原样。
- 判断的 `candidate` 引用：判断版本本就存候选的版本与来源（0031 的 `candidate_eta_version`、`candidate_source`），本票补的是它「读得回」——预测版本表的键恰是（包裹，里程碑，来源，版本），派发真库用例里首版判断的候选引用按版本读得回预测的输入。事件载荷不变（信封只是触发与认领，候选由处理方读库后选）。
- 文档：CONTEXT「ETA 与可见性缺口」补预测线一句与候选选取一句；UC-VE-003 改 `AT-VE-051` 的结果、新立 `AT-VE-222`–`AT-VE-225`；AT 盘点 `AT-VE-051` 由④移入①、新号入①。

### 代决

1. **预测线的键是（租户，包裹，里程碑，来源）；来源是线的维，不是线上的属性。** 承运商一路刷新只接替承运商那一路的前一版，不碰运营企业那一路，反之亦然；`Refresh` 换来源直接拒。理由：CONTEXT「分别保留来源和口径」；票面「最小的诚实修法」。
2. **存储形状照 `0016`、`0028`、`0031`：版本表只增不改写，当前标记是唯一可变处。** 不沿用「一行带 prior」的形状（被指回的行会被同一句覆盖）。`prior_version` 留裸文本不设外键（存量前身读不回；与 0016 同）。
3. **候选选取（任务单连带要求，回答 code/51 越权风险点 6 的「按来源优先还是各自判断」；评审修订后的口径）：候选 = 先取适用规则，再在各来源线的当前版里按来源优先序（运营企业派生的在前、承运商提供的在后，`domain.candidateSourcePrecedence` 一处）取第一个满足规则质量门槛的那一版；没有一个满足，或规则缺失、冲突无从比对时，取来源优先序最靠前的那一版，判断相应答 ETA 不可见或待判断，候选照实记下。** 不看预测时点；结果只由各线当前版与规则决定，与到达先后无关。判断线仍是（租户，账户，包裹）一条，不按来源分两条线。理由：（a）客户视图只有一维 ETA，「各自判断」把选择推到视图，状态翻倍而选择问题还在；（b）`PAR-VIS-03` 的对客质量门槛面向运营企业自己有形成方法的预测，承运商预测是保留来源的外部输入，排在后面作后备；（c）租户在规则册里把承运商的依据登为达标，就是愿意对客展示它——运营企业那一版不达标而承运商那一版达标时，候选是承运商那一版，不让客户 ETA 变空（门槛含新鲜度时，运营企业版过期、承运商版新鲜也是这个转换）；（d）不按预测时点选，对客的候选不随到达先后在两路之间来回换手。
4. **承运商提供的预测可作候选（后备，也可在运营企业那一版不达标时顶上）。** 沿 code/51 现状（判断本就存候选来源）；租户靠规则册的达标集合决定承运商的可信度依据是否达标。
5. **来源进候选引用：** 判断版本存（版本，来源），候选换手（版本或来源变了）形成替代判断指回前版，前一版判断仍引用换手前的那一版且读得回；候选没变时别的来源出新版本，判断答已有结果、不换代，客户视图也不换代（`AT-VE-225`）。候选所在里程碑不是规则指名的目标里程碑时沿 `AT-VE-214` 不判。
6. **写路径并发：线锁，加 `Save` 自己先核后写。** 先 `LockCurrent` 再决定：同一线上并发的相同命令收敛成一个版本（输家落重放而不是未决）、不同输入排成不分叉的链。`Save` 自己也取同一把线锁（同一事务内重入）、先核条件再写：首版核线上无当前标记，刷新核标记恰指前版，这个输入版本在线上没出现过；不成立什么都不写、交回 `ErrETACurrentChanged`，所以绕过线锁的写者输了也不留版本行，不靠调用方回滚。写后的 `ON CONFLICT`/`UPDATE ... WHERE` 零行命中留作最后一道。`FormETA` 因此必须在环境事务里运行（线锁与写同一事务；无事务拒跑有负面证据）。
7. **幂等键：（线，输入版本），线内输入版本唯一（评审修订）。** 这条线上曾经出现过的输入版本，不论先后再到，都答当初形成的那一版：不形成新版本、不移当前版、不把线推回旧输入；当前版自己的输入版本重复到达另重发同一份意图（ADR-0043），被接替的旧版本不再交意图（下游读各线当前版）。库里（线，输入版本）有唯一约束。两路来源带同名输入版本各是自己线上的首版，不算重放。
8. **迁移不对判断（0031）的候选引用补外键。** 外键会让预测版本的保留期限与归档（属实例半边）被判断挡住；候选引用 = 预测版本表一行的键，已够读回。
9. **不立 ADR（预留号 0332 未用）：** 没有改动任何已接受 ADR 的口径；ADR-0065 的追加版本原则是引用，本票是把 ETA 存储带到这条原则上。候选选取的来源次序记在 CONTEXT 规则句与本节，owner 如要改口径，改 `domain.candidateSourcePrecedence` 一处。
10. **迁移号 0032；AT：`AT-VE-051` 由④移入①，新立 `AT-VE-222`–`AT-VE-225`。** 盘点总览计数未手算，由推送方重生成。
11. **事件载荷不加来源。** 判断读库选候选，信封宣告的版本不决定候选；加字段会变更事件契约而收益为零。

### 越权风险点

1. **候选口径是产品口径，代决（owner 需知情）。** 来源优先序（运营企业在前）、「第一个达标者」与「都不达标取最靠前的一版」都是代决；如果 VE owner 认为承运商预测应优先、或承运商预测不应直接对客，改 `domain.candidateSourcePrecedence` 或 `CandidateETAFrom` 一处；已形成的判断要靠下一份 ETA 触发重判才会变（code/51 风险点 2 的老问题）。CONTEXT 状态是 Confirmed，代决的口径落在其中，不另立 ADR。
2. **两路都不达标时客户看不到 ETA（答 ETA 不可见）；运营企业一路不达标而承运商一路达标时客户看到承运商那一版。** 后者依赖租户把承运商依据登为达标这一治理动作；若租户不愿对客展示承运商预测，就不要把承运商的依据登进达标集合（今天达标集合不分来源，规则册没有「只收运营企业」的表达，需要时是另一个规则维度，属 owner 裁）。
3. **重复到达已由线内输入版本唯一兜住，乱序送不同输入仍不兜**（评审修订）：同一条线上曾经出现过的输入版本再到，一律答当初形成的那一版。兜不住的是从未出现过、但数据更旧的输入版本晚到（X2 之后才到 X1）：它被当成新的一版，会把线推回旧数据，接线时上游若可能乱序送不同的输入，仍需顺序守卫。另一点语义：「输入集合真的回到先前某一版」（例如更正撤回了新事实）在同一条线上形成不了新版本——回到旧输入集合也要给新的输入版本标识。
4. **存量行的 `prior_version` 指向已被覆盖的前身，读不回。** 迁移如实带过来；「每个 `prior_version` 都读得回」只对迁移之后的链条成立。生产上 `FormETAHandler` 无调用方，存量行预期为零。
5. **判断读候选不加锁。** 判断与写者并发时可能读到写者提交前的快照，之后写者的信封到达再判一次，这是既有的「判断按当前版、后继信封只是重放」，没有新增窗口；`ListCurrent` 用一条语句，两路并存时不会读到一路新一路旧的拼接。
6. **`FindCurrent`（带来源）今天只有测试与运营查阅用，没有生产调用方。** 保留是因为按线读当前版是预测线的基本读口。
7. **code/51 清单其余项不变**：规则册归属与登记口、门槛或规则版本单独生效的重判、停止展示边界执行方、判断读到无投影的残留竞态仍在；本票只做了其中第 6 项（候选选取）。

### 接线还差什么

存储与判断链修完后，接线不会再继承覆盖与混淆。还差的是接线本身（不在本票）：

1. **预测来源的生产调用方**：承运商预测的接入（外部承运商预测的受理与翻译为 `FormETACommand`，来源 `CarrierProvidedETA`）和运营企业派生预测的形成方（`PAR-VIS-03` 的形成方法属产品策略，待提供）。两者都经 `FormETAHandler`，每次调用必须在环境事务里，`HandOffETA` 与预测写同一事务。
2. **结果格的消费方落点（ADR-0025）**：`FormETAUndecided` 的 `ETACurrentChanged`（重读再走）与 `ETAStoreUnavailable`（等库）、`HandoffReference`（发布续办）、`FormETANotAccepted`。
3. **上游不需要单调来防重复，但乱序送不同输入仍要顺序守卫**（风险点 3）：真正不同的输入集合必须有不同的输入版本引用（回到旧输入集合也要新标识）；从未出现过的输入版本按到达序生效，上游若可能乱序送不同输入，接线时仍需顺序守卫。
4. 事件路由已在位（`routes_eta_visibility.go`）；对客放开 ETA 之前 code/51「上线前必做清单」的其余项仍要做。

### 验证（分支 claude-code07）

- `go build ./...`、`go vet ./...` 干净，`gofmt -l` 无输出。
- `scripts/test-full.sh`：改动包 + `go list` 算出的反向依赖 + `./cmd/parcel-api/` + `./cmd/parcel-dispatch/` + `./internal/architecture/...` + `./migrations/...` + `./internal/visibilityexception/...`，退 0（81 个包 ok）。新用例 `-count=3` 全绿；并发用例（`Concurrent`、`AStaleWriter`）`-count=5` 全绿。
- 变异 14 处，均被杀、逐一从备份还原并 `cmp` 一致：`Refresh` 不查来源；候选来源次序反过来；候选改为「预测时点最晚者」（应用层用例原先杀不死——两路预测时点相同——已改夹具让承运商那一版总是更晚，重跑被杀）；幂等键不带来源；刷新时删被接替的版本行；线锁去掉（并发用例 `-count=5` 每轮红）；乐观条件 `version_id = 前版` 去掉；首版标记改为 `DO UPDATE`；判断里候选来源写死；迁移预检去掉；写者改用不带锁的读；并发输家并入「库读不动」一格；`FindVersion` 不带来源；`ListCurrent` 不带租户。
- 迁移 0032：存量迁移用例（带前身指回的行原样、同名包裹跨租户各自成线、旧表退场、跨来源线的标记被外键拒）与预检用例（版本号重复给出可读报错、旧表原样）；一次性库灌 `scripts/demo-seeds/seed.sh` 成功（新两表在、旧表无），用后已删库。
- `go run ./tools/adr-index -check` 通过；`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，分桶表无缺行）。

### 评审记录与修订轮

独立评审（2026-10-27）：可合入，无阻断；下列三条建议与一条口径意见，协调方全部采纳，已在本分支修订。

- **N1（并发输家的版本行「随事务回滚」只在适配器层成立）：** `FormETA` 把 `ErrETACurrentChanged` 译成 `Undecided/ETACurrentChanged` 并返回 nil error，调用方事务照常提交，原先 `Save` 先 INSERT 版本行再试移标记，绕过线锁的写者输了会留下一个从未成为当前的版本。修（做法 a）：`Save` 先取线锁，核完条件才写，输了什么都不写。真库用例 `TestALoserThatBypassedTheLineLockLeavesNoVersionBehindEvenWhenTheTransactionCommits`：绕过线锁的写者（以为线上没有当前版 / 拿着已被接替的前版）经 `FormETA` 输了之后，事务照常提交，版本行数不变、当前版仍是赢家。
- **N2（幂等的文字承诺比实现宽，乱序重放）：** 现在就做线内输入版本唯一。`FormETA` 经新增的 `FindByInputs` 判重放（兜住重复到达，不兜从未出现过但数据更旧的输入版本晚到），晚到的旧输入版本答当初形成的那一版；迁移 0032 本身加唯一约束 `eta_prediction_version_inputs_unique`（0032 未进 main，直接改；存量每条线只有一行，不会违反，所以没有新增预检）。用例：应用层 A、B、A（第三个答已有结果、版本数不增、当前版仍是 B、不入队意图），真库同序列，并发重复输入版本（每个输入版本恰形成一版）、唯一约束与 `FindByInputs`（同名输入版本在两条来源线上各是各的）。CONTEXT 规则句、`AT-VE-222` 措辞、`FormETA` 头注、UC-VE-003 末段与「接线还差什么」第 3 项同步。
- **口径变更（候选 = 达标者中来源优先的那一版）与 N3：** 判断编排改为先取规则再选候选；`domain.CandidateETAFrom` 按来源优先序取第一个满足质量门槛的当前版，都不满足或规则缺失、冲突时取最靠前的一版。CONTEXT 规则句、`AT-VE-224`/`AT-VE-225` 与两个单测、派发真库用例按新口径改写。补用例 (a) 运营企业 `MEDIUM` 达标、承运商更新且 `HIGH` 达标 → 候选运营企业、可见；(b) 运营企业线刷成 `LOW/sparse`、承运商仍 `HIGH` → 候选换手到承运商那一版、仍可见、指回原判断（单测与派发真库各一）；(c) 两路都不达标 → 候选取运营企业、答不可见（派发真库里再接一份 `LOW/stale` 的承运商版）；(d) 只有承运商线 → 候选承运商，按门槛判（达标可见、未达不可见）；另补规则缺失与冲突时候选取最靠前的一版、答待判断。
- **变异（备份→改→跑→拷回→`cmp`，11 处新增，均被杀）：** 候选忽略规则（回到「只看来源」）→ (b) 与候选来源重判用例红；候选改成「预测时点最晚者」→ (a) 等六例红；先选候选再取规则（顺序对调）→ 单测与派发真库红；`Save` 去掉先核 → N1 用例与晚到旧输入用例红；重放只认当前版 → A、B、A 用例红；被接替版本的重放也交意图 → 该用例红；唯一约束去掉 → 约束用例红；线锁去掉（`lockLine` 改成不阻塞的 `pg_try`，`-count=5`）→ 并发用例红；只去掉 `LockCurrent` 里的取锁（`Save` 仍取）→ 并发用例红（输家落未决而非重放）；`Refresh` 不查来源、刷新时删被接替版本行 → 原用例红。前一轮对「乐观条件 `version_id = 前版`」与「首版标记 `DO UPDATE`」的两处变异不再能被杀：`Save` 持线锁先核后写，这两处只是留作最后一道、持锁时不会命中，属于有意保留的纵深。
- **验证：** 见下方「修订轮验证」。

### 修订轮验证

- `go build ./...`、`go vet ./...` 干净，`gofmt -l` 无输出。
- `scripts/test-full.sh`：改动包 + 反向依赖 + `./cmd/parcel-api/` + `./cmd/parcel-dispatch/` + `./internal/architecture/...` + `./migrations/...`，退 0。新用例 `-count=3` 全绿，并发用例 `-count=5` 全绿。
- 迁移 0032 加了唯一约束：一次性库灌 `scripts/demo-seeds/seed.sh` 成功，用后已删库。
- `go run ./tools/adr-index -check` 通过；`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，分桶表无缺行）。

### 评审记录与修订轮（第二轮）

窄复审（2026-10-27）：可合入，无阻断；三条建议为补测试与改措辞，不改业务逻辑，已在本分支合入前补齐。

- **N-a（`FindByInputs` 与 `Save` 的「输入版本已出现」查询缺跨包裹、跨里程碑的测试）：** 补真库用例 `TestTheSameInputVersionOnAnotherParcelOrMilestoneIsAFreshInputVersion`——同租户同来源，P1/DELIVERED 形成 X；P2/DELIVERED 形成 Y 再来 X；P1/PICKED_UP 形成 Z 再来 X；断言后两个 X 都是 `ETARefreshed`（形成新版本，不是已有结果，也不被预检误报成并发输家），版本行 5 条。
- **N-b（`Save` 自己取线锁、`inputsSeen` 预检没有测试钉住）：** 补并发用例 `TestWritersThatBypassedLockCurrentStillSerializeOnTheLineInsideSave`（8 个绕过 `LockCurrent` 的写者用不同输入在空线上形成首版：恰 1 个 `ETAFormed`、其余 `ETACurrentChanged`、版本行 1 条，`-count=5` 稳定）；补用例 `TestAnAlreadySeenInputVersionFromAWriterThatBypassedTheReplayCheckIsAConflictNotAnOutage`（绕过线锁与重放判断的写者拿着正确的当前版、用线上出现过的旧输入版本刷新，结果是 `Undecided/ETACurrentChanged` 而不是 `ETA_STORE_UNAVAILABLE`，ADR-0029）。`Save` 里「写后两处零行命中」的注释补一句：若命中，版本行已写下且 `FormETA` 不回滚，这是不变量被破坏的告警而非保护。
- **N-c（文档措辞）：** (1) UC-VE-003 末段与本票风险点 3、「接线还差什么」第 3 项改为：上游不需要单调来防重复；但真正不同的输入集合必须有不同的引用（回到旧输入集合也要新标识）；从未出现过的输入版本按到达序生效，上游若可能乱序送不同输入，接线时仍需顺序守卫（唯一约束只兜曾经出现过的输入版本）。(2) 变更叙述词：`FormETA` 头注与 `TestAnInputVersionThatAppearedBefore…` 的 Covers 注释改成直述（先用 `git diff origin/main...HEAD` 找全，只改本分支新增的代码与文档行；票面里的 Status 前史与评审记录属叙述，保留）。(3) `TestAStaleWriterIsRefusedAndLeavesNoVersionBehind` 的注释改成「输家什么都没写」，并指向绕锁并发用例证提交后也不留版本行。
- **变异（备份→改→跑→拷回→`cmp`，6 处，均被杀）：** `FindByInputs` 去 `parcel_ref`、去 `milestone_ref`，`Save` 的 `inputsSeen` 去 `parcel_ref`、去 `milestone_ref`（四处都由跨包裹、跨里程碑用例杀）；删 `Save` 里的 `lockLine`（并发绕锁用例 `-count=5` 每轮红）；删 `Save` 里的 `inputsSeen ||`（冲突不是库读不动的用例红）。
- **验证：** 见下方「第二轮验证」。

### 第二轮验证

- `go build ./...`、`go vet ./...` 干净，`gofmt -l` 无输出。
- `scripts/test-full.sh`：改动包 + 反向依赖 + `./cmd/parcel-api/` + `./cmd/parcel-dispatch/` + `./internal/architecture/...` + `./migrations/...`，退 0。新用例 `-count=3` 全绿，并发用例 `-count=5` 全绿。
- 本轮没有动迁移，不另灌种子。`go run ./tools/adr-index -check` 通过；`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，分桶表无缺行）。
