# 57 段采用委托与委托取消都没有更正面，首个参与被更正后委托的开始也不重派生

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg57。此前 in-progress——2026-10-21 认领（57、15 同批）；此前 ready-for-agent——2026-10-20 上线就绪评估归 R1，与 88 同批；「解除采用」的裁定见 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」。此前 needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [25](./25-segment-adopts-transport-commission-and-cancellation-faces.md) 完成记录与 [ADR-0204](../../../docs/adr/0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 越权风险点 1、2、8（风险点 8 由票 25 隔离评审补入）。
Blocked by: 无——用户决定一格已于 2026-10-20 代决，见 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」（此前 用户决定（TF owner 复核 ADR-0204 风险点 1、2、8：采用错了怎么改，「已开始」能不能回退，采用登记之前成立的取消怎么更正））

## 现象

- 采用册只有插入（迁移 TF 0035）。指错了段或委托，同段再交答 `ADOPTION_CONFLICT`，没有任何一口改得了；委托一经采用就标了开始，也就取消不了。
- 采用把委托的开始依据与时刻取自已采用各段首个当前参与里最早的那一条（后采用、开始更早的段把它前移），此后那条参与被来源更正替代或改判失效（ADR-0112），委托上的开始与连线上的开始都不随之重派生，也不后移。极端情形：段上的控制事实全部撤回，委托仍记着「已开始」。
- 采用登记之前的取消窗口（ADR-0204 决定三「采用与取消的先后」）：段已凭交接开始、采用还没登记时，委托照常取消得了（201）；之后的采用答 `COMMISSION_CANCELLED_AFTER_START`，并排回显取消时刻与段的首个参与，不落连线。续办写的是「更正取消或更正控制事实」，但取消没有更正面，委托停在已取消、段挂不上它。
- 取消时刻早于段的开始时，结果随采用与取消的登记先后而异（先采用则已开始、取消被拒；先取消则不可采用），缺的是「取消是否送达服务方」这一事实。

## 要裁的

- 解除或更正采用是不是一种运营决定（新种类），还是由控制事实的更正驱动。
- 「已开始」能不能回退：TF CONTEXT 只说已成立的实际履约段不存在「取消回未开始」，委托的开始没有同样明文；回退后此前「开始之前提出」的订舱判据随之变化。
- 首个参与被更正时，委托的开始按新的首个参与重派生，还是只在连线上记新版本、委托上保留首次。
- 取消的更正：取消晚于开始那一格由谁、凭什么更正——撤回取消让委托回到已提交再被采用，还是直接改标已开始；撤回之前已据取消停掉的订舱、预占怎么办。
- 要不要登记「取消送达服务方」这一事实，用它裁取消时刻早于开始的那一处先后。

## 三类

机制；解除的授权粒度属产品角色模型（psb/07）。

## 代决复核追加（2026-10-20）

出处：[owner-review-20261020](../../owner-review-20261020/report.md) 的 R10 与 M9。

- **R10 代决「是」**：解除采用后，委托再无别的已采用段时，撤回「已开始」回到「已提交」，以更正事实记录，原开始与解除都留历史；还有别的已采用段时，开始改按其余段最早的首个参与重新派生。
- **须新 ADR**（M9 问题一）：部分停用 ADR-0204 决定一末两句（「采用之后不改、不解除」）与决定二「只前移不后移」。
- **发生项失效的生产入口纳入本批**（M9 问题三）：「已形成的先走发生项失效」那条路今天不存在（TF `ReviseValidity` 无生产调用方）。本票连同 88 一批做时，一并给出发生项失效的生产入口；做不到就在完工记录里写明成本纠错暂缺时的绕法。88 的门铃消费者形成发生项之前，先复核采用是否仍在册。票 15 随之升为上线前必修（分诊表本就注明「那个入口一落地即升 A」）。
- **其余三问拆出**（M9 问题二）：首个参与被更正后是否重新派生、取消的更正、「取消送达服务方」事实，三问不在本票，由协调方另立 needs-triage 子票；本票只留解除采用。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 claude-dcg57，变基在 main `7b598e47` 之上）

与票 [15](./15-occurrence-revision-does-not-drive-a-cost-correction.md) 同批，共用一份新 ADR [ADR-0232](../../../docs/adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md)。裁定的单一权威是分诊「上线就绪评估（2026-10-20）」关于本票那一条与「代决复核追加的上线前必修」、代决复核 R10 与 M9；正文没写的细节代决，逐条记在 ADR-0232 决定一至九，越权风险点见其文末（第 1 至 9、14 点归本票）。

**做了什么**

- 运营决定「解除履约段采用」（`POST /transport-fulfillment-segment-commission-adoption-withdrawals`，载荷段与点名的委托）：段上还有按这份委托形成（服务方与协议快照同委托）、仍有效的实际履约发生项答 `OCCURRENCES_FORMED` 并回显，续办是先失效；段上采用的不是点名那一份答 `ADOPTION_MISMATCH`；没有当前采用答 `ADOPTION_NOT_FOUND`（最近一次解的正是这一份答 `EXISTING_WITHDRAWAL`）。解除另记一行（之前的开始、变法、之后的开始、解除人），采用那一行不改；采用册改为按代，解除之后同段再采用是下一代。
- R10：委托再无别的已采用段时开始撤回、回到已提交（之后又取消得了）；还有时按其余各段采用上记的开始取最早的那一条（可以后移）；不是开始所依的那一段时不动。部分停用 ADR-0204 决定一末两句与决定二「只前移不后移」（限解除采用），ADR-0204 Status 行与 Links 加前向指针、正文不动。
- M9 问题三：发生项失效的生产入口（`POST /transport-fulfillment-charge-occurrence-invalidations`，运营决定「运输收费发生项失效」）：点名链头那一版与一条依据，取范围锁、长出 V(n+1) 失效版本、记修订者，已形成信封同笔交出；票 15 由此升为上线前必修并在同一分支做完。登记用例改读链头（ADR-0207「已知限制」兑现）：失效之后同一件事实再登从失效版本长出替代版本，所以错采用的那一段改采用对的委托之后，成本能在对的委托下重新形成。
- 并发：解除先取这一段实际履约范围的发生项锁（与登记查覆盖同一把）再读发生项；登记拿到这把锁之后重读采用、变了答未决 `PROCUREMENT_SOURCE_CHANGED`（88 的门铃消费者经同一个登记用例，「形成之前先复核采用是否仍在册」由这一步做到）；采用与解除共用委托的开始锁（新空间 `transport_fulfillment.transport_commission.start`）。三条真库并发用例（解除对登记两个方向、采用对解除），各在拆掉对应那一步时变红（实测）。
- M9 问题二：其余三问拆成 needs-triage 子票 [162](./162-commission-start-does-not-rederive-when-the-first-participation-is-corrected.md)、[163](./163-a-cancellation-dated-after-the-start-has-no-correction-face.md)、[164](./164-whether-a-cancellation-reached-the-provider-is-not-recorded.md)。
- 文档：TF `CONTEXT` 段采用委托一条、委托生命周期、发生项生命周期；UC-TF-004 补 `AT-TF-306`–`310`、`315` 与一致性一条，UC-TF-005 补 `AT-TF-311`–`314`；AT 盘点①加行。演示种子另起 `steps/457`（配置员持两种决定，`data/access/11`、`12`，`data/governance/48`、`49`），一次性库上实灌两遍零报错。

**迁移**：access_identity `0021`（决定种类 CHECK 以 main 上的 0020 为底取并集加两词）、transport_fulfillment `0048`（main 上 0047 已被票 145 占用；修订者一列、一版至多被修订一次、采用册代次与解除册）。

**提交**（变基之后）：`000dbe15` 决定种类；`f535839a` 发生项失效入口与再登；`ca01be22` 解除采用；`5ace0366` 种子；`20a2427f`、`57720489` ADR；`ea00cc0b` CONTEXT 与 UC；`38884472` 子票；`e37d1765` AT 盘点。票 15 那一笔见其完成记录。

**验证**：每一笔提交 `go build ./... && go vet ./...`（`git rebase --exec` 逐笔跑过）；`gofmt -l` 干净；`scripts/test-full.sh` 跑 TF、SA、access-identity 与它们的全部反向依赖（含 cmd/parcel-api、parcel-dispatch、parcel-access-register、parcel-settlement-register、parcel-transport-register）、`./internal/architecture/...`、`./migrations/...`，48 包 ok、0 FAIL；清单在会话 scratchpad 的 `dcg57/test-packages.txt`。未跑全量，未重生成机制清点、ADR 索引与 AT 计数。

**未决与知会**：越权风险点见 ADR-0232 文末。票 88 会改 `adopt_segment_commission.go`、`assemble_segment_commission_adoption.go` 及其测试、`tfhttp/register_charge_occurrence.go`、`operator_authenticator_test.go`、`cmd/parcel-dispatch/actual_fulfillment_on_participation_end_test.go`——本票在这几处只加（一口必备依赖 `CommissionHold` 与取锁一行、响应里修订五格、决定映射两行、测试构造一格），后进 main 的一方合并；88 的 access_identity 迁移若也取 0021、TF 若取 0048，后进的一方改号。管理台预期成本页还没显示 `withdrawn`（admin-web 另做）。

## 评审记录（一轮，变基前；2026-10-21，Claude Code 作者子代理，分支 claude-dcg57，未变基）

隔离评审的各条逐条去向。票 15 那几条（4、5、7、8、9）记在 [15](./15-occurrence-revision-does-not-drive-a-cost-correction.md) 的同名一节。

- **B1（阻断）解除与采用的重放产生新效果——已修，`3db95e8a`。** 采用回显带 `generation`（采用口与解除口回显的采用都带）；解除载荷加必填 `generation`，按（段、委托、代次）认：点名那一代已解除且委托相同答 `EXISTING_WITHDRAWAL`，委托不同答 `ADOPTION_MISMATCH` 回显那一次，当前那一代不是点名那一代答 `GENERATION_NOT_CURRENT` 回显当前那一代；端口改为 `FindWithdrawal(key, generation)`（`FindLatestWithdrawal` 留给采用那一侧判有没有历史）。采用一侧：段上一代已解除时请求必须带 `afterGeneration`，缺了或不符答 `ADOPTION_ALREADY_WITHDRAWN` 并回显那一次解除，不落新一代；从没解除过的段不受影响。PG 写口在 `INSERT … SELECT` 里再守一次代次，并发两笔再采用不会各落一代。评审那份真库复现收为 `cmd/parcel-api/fulfillment_corrections_test.go::TestReplayingAnOldAdoptionOrWithdrawalHasNoNewEffect`（采用、解除；旧采用请求重交答已被解除、不落新一代；点名第 1 代的再采用落第 2 代；第 1 代那一次解除重交答重放、不解除第 2 代；点名第 3 代答代次不是当前并回显第 2 代；册上采用 2 行、解除 1 行）。ADR-0232 决定一与越权风险点 5、UC-TF-004 `AT-TF-306`、`310`、TF `CONTEXT` 与种子 README 的更正次序随之改口（`c7cdd183`、`35b7da92`、`d9167fb5e`）。
- **3 同刻并列时解除不相干的段也改开始——已修，`b95a5046`。** `WithdrawSegmentCommissionAdoption` 在取最早一条之前先看其余各段里有没有一条恰是开始的（依据、时刻），有就记 `KEPT`、开始不动；评审复现收为 `TestWithdrawingAnUnrelatedSegmentUnderATieKeepsTheStart`。ADR-0232 决定二与 `AT-TF-308` 补这一格。
- **6 注释（TF 与 cmd 那几处）——已修，`ae29b24b`。** `endpoints_fulfillment_corrections.go`「上一口」改为点名「运输收费发生项失效口」；`invalidate_charge_occurrence.go` 头注与 `register_charge_occurrence.go` 改成直接陈述；顺手扫本分支新增注释，`segment_commission_adoption.go` 里「迁移 0048 的两道约束」（计数且带将变的迁移号）改点约束名，`charge_occurrence_rehydration.go` 一处「此前」去掉。SA 那一处见票 15。新用例 Covers 里的评审出处去掉（`083f710c`）。

**未采纳**：无。

**验证**：本轮每一笔代码提交单独 `go build ./... && go vet ./...`（各自 `git archive` 出来跑）；`gofmt -l` 干净；`scripts/test-full.sh` 跑 TF、SA、access-identity 的包与全部反向依赖、`./internal/architecture/...`、`./migrations/...`，以及两边的 domain 包：除 `cmd/parcel-dispatch` 的 `TestATransportPreparationUndecidedRedeliveryDoesNotReprojectVisibility` 在包并发下红过一次之外全绿；那一条与本票无关（运输准备与可见性投影），单独跑三遍与整包重跑都绿，记作并发负载下的偶发。

**变基时做**（88 进 main 之后，协调方通知）：

- `formedUnder` 与 88 的委托协议换版：88 换版之后按业务时间形成的项带后继版本的协议快照，只比 `commission.Agreement()` 认不出、解除会被放行——改为与委托上全部协议快照（提交版加 `AgreementRevisions()`）比，或让发生项记下委托标识；补用例。
- 88 的补形成与失效：委托一换版，门铃补形成会以系统身份把人工失效过的项长回 V(n+1)——只有操作者登记才从失效版本长出替代版本；系统身份遇已失效链头答已了结（除非失效之后采用换了代），或补形成跳过已失效链头；补用例。
- 迁移与种子号：TF 取 main 下一个空号（88 占 0048 则本票 0049），access_identity 以 main 最新为底只加本票两个词（88 占 0021 则本票 0022，CHECK 并入 88 的 `TRANSPORT_COMMISSION_AGREEMENT_REVISION`）；种子 `data/access/11`、`12` 与 `data/governance/48`、`49` 改空号；`access_identity 0021` 等迁移号在注释与 ADR-0232 Consequences 里的引用随之改。
- `ListByCommission`（88）与 `FindCurrentByCommission`（本票）同一问两份口径，变基后统一（票 160 管的那一半写清）。
- AT 编号：本轮新用 `AT-SA-211`（UC-SA-004），变基时核一次 main 上有没有别的票先占了。
