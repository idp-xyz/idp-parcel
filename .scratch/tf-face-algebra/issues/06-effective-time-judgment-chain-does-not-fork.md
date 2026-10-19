# 06 有效时间判断：同一事实并发判断时版本链分叉且不自愈——回指列缺「一版至多被回指一次」，撞键一律答未决

Category: bug
Status: in-progress——2026-10-19 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（分支 `claude-tfa06`）。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-effective-time-judgments
地盘：
- migrations/transport_fulfillment/（新迁移，序号取落地时的下一个）
- internal/transportfulfillment/application/judge_external_tracking_effective_time.go 及其测试
- internal/transportfulfillment/application/adopt_tracking_material.go 及其测试（`resolveCorrection` 撞键读回）
- internal/transportfulfillment/adapters/postgres/（新增真库回归测试文件）
- cmd/parcel-api/（新增真库并发回归测试文件）
- .scratch/label-channel-service-first-release/issues/20-tracking-pull-beat.md（只加 Blocked by 一行）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决）

照「修复方向（机制）」四条做，与 tf-face-algebra/02、07 同形；迁移取 `migrations/transport_fulfillment/` 下一个空号。属机制。

## 现象
- 同一载荷经生产装配并发投递（默认池），全答 201 `EFFECTIVE_TIME_JUDGED`，判断版本全都回指同一待判断版，每份各发一封判断意图。管理台当前版读面上，同一事实列出与投递份数相同的当前版。多轮复跑，分叉每轮都有。
- 两份不同有效时间的判断并发（读后屏障）也分叉。
- 分叉之后再改判，新版本只接在 `FindCurrent` 取到的那一支上，另一支永远留作当前版，不会自愈。
- VE 侧（经生产适配器 `DeriveOnExternalCarrierTrackingAdapter` 译成已接受事实）：
  - 刚分叉时，`SupersessionForks` 报出这一组分叉，`ResolveByBusinessTime` 不择一。
  - 改判之后，`CurrentlyEffective` 里同时有两份，而 `SupersessionForks` 为空：冲突信号不再成立，旧那一支被静默当作有效。
- 只在单份测试库里给回指列加上照 0014 写法的部分唯一索引：分叉没了，但输家答 200 `JUDGMENT_UNDECIDED`（`TRACKING_FACT_REGISTRY_UNAVAILABLE`）。

## 复现（S）
在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`。

用到的探针与日志：
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/effective-time-judgments/zz_probe_algebra_test.go 的 `TestProbeDecide2ETConcurrentReplay`（overlay_algebra.json）。
- 同目录 adv-impl/zz_probe_adv_impl_test.go 的 `TestAdvETProductionDefaultPoolBurst`、`TestAdvETForkAfterwards`（日志 adv_impl_run1.log、adv_impl_burst3.log）。
- 同目录 adv-authority/zz_probe_adv_authority_test.go 的 `TestProbeAdvAuthorityForkShapes`（日志 adv_fork_shapes.log）。
- 同目录 zz_probe_verify_test.go 的 `TestPVBurstRepeated`、`TestPVBarrierDifferentContent`、`TestPVForkThenRejudgeAndVE`、`TestPVUniqueIndexLoser`（overlay_verify.json；日志 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/effective-time-judgments/synth/run-verify.log）。

## 应当
- 版本链的形状由库面守，不靠编排里那次「先读当前版再写」。0014 头注原话如此；0017、0020 在同一上下文里也这样做（`effective_time_rule_single_successor`、`carrier_master_document_supersedes_once`、`carrier_first_effective_pickup_superseded_once`）。
- 并发输家读回赢家后再判：同值答 `ALREADY_JUDGED_AS_GIVEN` 并回显赢家；异值在赢家之上改判。

## 根因
- `0011_external_carrier_tracking_fact.sql` 的回指列上没有部分唯一索引。
- `transactionalEffectiveTimeJudge` 运行在读已提交事务里。
- `JudgeEffectiveTimeHandler.Judge` 在 `Save` 未答已保存时一律答未决，注释为「版本是刚签的，撞键说明登记册出了本上下文任何路径都造不出的状态」，没有读回赢家的那一支。

## 修复方向（机制）
1. 新迁移给 `transport_fulfillment.external_carrier_tracking_fact` 加部分唯一索引 `(tenant_id, fact_ref, supersedes_version) WHERE supersedes_version IS NOT NULL`。
   - 建索引前先查存量有没有分叉；有就如实报出，不静默删。
   - `Save` 的 `ON CONFLICT DO NOTHING` 未指冲突目标，所以会把这条索引上的冲突也答成「已登记」。
2. `JudgeEffectiveTimeHandler.Judge` 在 `Save` 答「已登记」时：
   - 重读 `FindCurrent`，再走同一套判定。
   - 同值答 `ALREADY_JUDGED_AS_GIVEN` 并回显赢家；异值在赢家之上改判。
   - 有限重试，仍撞则答未决并带续办。
   - 先例是 `OpenDispatchTaskHandler.Open` 的「并发输家读回赢家」。
3. 收编更正路径 `AdoptTrackingMaterialHandler.resolveCorrection` 回指当前版，会撞同一条索引，它的撞键读回一并改。
4. 在拉取节拍票 label-channel-service-first-release/20 上加一行 Blocked by 指向本票（只加行）。

## 是否潜伏
潜伏。锁在事实来源：生产装配里没有外部承运轨迹事实的写入方。
- `AdoptTrackingMaterialHandler` 没有生产入口。`buildExternalCarrierCredentialRegistration` 头注原话：「收编执行器（AdoptTrackingMaterialHandler）的生产入口随第一家真源的拉取节拍票立」。
- lc/20 仍是 draft；种子也不灌轨迹事实。
- 本口在空库上答 200 `INPUT_NOT_ACCEPTED`。

拉取节拍或任何收编入口接上的那一刻就会发作。

## 完成判据
- [ ] 迁移：干净库上 `migrate` 顺序跑通；存量分叉检查语句与结果写进完成记录。
- [ ] 编排单测（替身模拟「已登记」）：同值读回答 `ALREADY_JUDGED_AS_GIVEN`；异值在赢家之上改判；读回不通答未决。
- [ ] 真库并发回归（`cmd/parcel-api` 新测试文件，生产装配 `buildEffectiveTimeJudgment`，默认池）：
  - 同一载荷并发多份，恰一份 201，其余答 `ALREADY_JUDGED_AS_GIVEN` 并回显赢家；当前版读面只有一条；判断意图只有一封。
  - 读后屏障两路异值，最终链线性，当前版读面只有一条。
- [ ] 收编更正与判断撞同一前版的真库用例：后到者读回，不分叉。
- [ ] VE 侧：经 `DeriveOnExternalCarrierTrackingAdapter` 译后，`SupersessionForks` 为空，`CurrentlyEffective` 只有一份。
- [ ] lc/20 的 Blocked by 行已加。
- [ ] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./internal/visibilityexception/... ./cmd/parcel-api/... ./internal/architecture/...` 全部 ok。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验、非作者评审（两轴分开）、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa06）

基线 `32a9d03f`。提交：`3ff44878`（迁移与编排修复、编排单测）、`d36c2fd7`（真库回归）、`35263c35`（lc/20 加 Blocked by 行）、`2a83c0c3`（机制清点重生成）、本条。

**落点**
- 迁移 `migrations/transport_fulfillment/0023_external_carrier_tracking_fact_superseded_once.sql`：部分唯一索引 `external_carrier_tracking_fact_superseded_once` (tenant_id, fact_ref, supersedes_version) WHERE supersedes_version IS NOT NULL；建索引前的 DO 块按（租户，事实，前版）分组数后继，>1 即 RAISE 列出各前版与后继，不代删。
- `JudgeEffectiveTimeHandler.Judge`：读当前版→判定→落版本走成有界循环（`judgmentAttempts`），Save 答`已登记`即重读 `FindCurrent` 再判；同值答 `ALREADY_JUDGED_AS_GIVEN` 回显赢家并重发其意图（`EnqueueOnce` 按版本去重，不多出一封），异值在赢家之上改判；仍撞答未决 `TRACKING_FACT_REGISTRY_UNAVAILABLE` 带续办。
- `AdoptTrackingMaterialHandler.Adopt`：撞键先按（源，源事件）读回（读得到即重复投递，原行为）；读不到而本版回指前版，即调 `resolveCorrection` 重新解析到此刻的当前版再落，有界（`trackingCorrectionAttempts`），本次签的版本未落行，沿用。
- `ExternalTrackingFacts.Save` 头注补上新索引这一种撞键（越出地盘的唯一一处，只改注释）。

**各格对应用例**
- 编排单测（替身照库面索引拒同一前版的第二个后继，`beforeSave` 摆出「另一方先提交」）：同值读回 `TestALosingConcurrentJudgmentWithTheSameTimeEchoesTheWinner`；异值改判 `TestALosingConcurrentJudgmentWithAnotherTimeIsJudgedOverTheWinner`；读回不通 `TestAJudgmentWhoseReadbackFailsIsUndecided`；有限重试 `TestAJudgmentThatKeepsLosingRetriesBoundedlyThenIsUndecided`；更正输给判断 `TestACorrectionThatLosesToAConcurrentJudgmentLandsOnTheWinner`。
- 真库并发（`cmd/parcel-api/effective_time_judgment_single_chain_test.go`，生产装配、默认池）：赢家在事务里挂住、观察连接看到本口卡在锁上再放行——同值 `TestALosingConcurrentJudgmentWithTheSameTimeEchoesTheWinnerAgainstARealDatabase`、异值 `TestALosingConcurrentJudgmentWithAnotherTimeIsJudgedOverTheWinnerAgainstARealDatabase`（代替「读后屏障两路异值」，交错是确定的）；同一载荷六份并发四轮 `TestConcurrentIdenticalJudgmentsFormExactlyOneJudgedVersionAgainstARealDatabase`（恰一份已判断、其余回显赢家、当前版读面一条、每条事实判断意图一封）。
- 收编更正与判断撞同一前版（`internal/transportfulfillment/adapters/postgres/external_tracking_fact_single_chain_test.go`）：`TestACorrectionLosingToAHeldJudgmentLandsOnTheWinnerAgainstARealDatabase`；索引本身 `TestASecondSuccessorOfOneVersionIsRefusedByTheRegistry`。

**先红后绿**：用例先于实现写下并跑红，红都落在行为上——编排单测四例红（同值、异值答 `JUDGMENT_UNDECIDED`；更正答 `ADOPTION_UNDECIDED`；持续撞键「试了 1 次」），读回不通一例修复前后同答未决，作守门；真库五例红：同值输家答 `EFFECTIVE_TIME_JUDGED`、异值改判回指 `…/v1` 而非赢家、六份并发两份 `EFFECTIVE_TIME_JUDGED`、更正回指首版 `SYN-EXTV-TFA06-a`、第二个后继答 `SAVED`。票面现象（并发判断分叉）由此在新用例上复现，属 `S`。加迁移与修复后全绿，真库那几例连跑三遍皆绿。

**迁移号** `0023`（取号时 fetch 过 origin，origin/main 与在途分支上 transport_fulfillment 最高为 0022）。存量分叉检查：干净库上 `migrate` 顺序跑通（`./migrations/...`、全部带 DSN 的包都从模板库跑过整套迁移）；拒绝分支另用一次性用例核过（未提交）——测试库删索引、造 SYN-V-a 两个后继后执行该 DO 块，答 `P0001`「external_carrier_tracking_fact 存在被回指了不止一次的版本……SYN-T/SYN-F 的 SYN-V-a（后继 SYN-V-b、SYN-V-c）……」。

**门**：`gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api ./internal/architecture/... ./migrations/...` 全 ok，另跑 `./internal/visibilityexception/...` 全 ok。机制清点重生成单独提交。全仓全量未跑。

**未做或复现不了的格**
- VE 侧（经 `DeriveOnExternalCarrierTrackingAdapter` 译后 `SupersessionForks` 为空、`CurrentlyEffective` 一份）未单独立用例：TF 链线性后每个前版至多一个后继，译过去的替代关系同形；需要派生管线全装配，留给评审定是否补。
- 票 15 补记下追加回写行：不在本次授权地盘内，未动。
- 端点层 201/200 未断言：真库用例断在生产装配的判断编排结果上，HTTP 映射未改。
- 票面「复现」节列的 scratchpad 探针未重跑，复现以上述新用例为准。

**证据层级** `S`（隔离合成，SYN- 值）。

评审：待会话读 diff 审

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `b85efa9e`（基线 `32a9d03f`）· 2026-10-19**：代码出自本会话起的实施子代理，会话读 diff 审，不算非作者评审，如实记下。
  - **Spec**：阻断无。迁移 0023 给 `external_carrier_tracking_fact` 加「一版至多被回指一次」部分唯一索引，存量分叉整笔失败并列出、不删行；判断口 Save 答已登记时重读当前版再判（同值答 `ALREADY_JUDGED_AS_GIVEN` 回显赢家，异值在赢家之上改判，有界重试后答未决带续办）；收编更正撞键先按（源，源事件）读回、读不到再重新解析当前版。`ON CONFLICT DO NOTHING` 不让事务进中止态，同一笔事务里的有界重试安全。lc/20 的 Blocked by 只加一行。
  - **Standards**：阻断无，非阻断无。
  - **未做的格**：VE 侧派生管线的替代关系线性没单独写用例（TF 链线性后译过去即线性）；端点层 201/200 未断言（HTTP 映射未改）。
