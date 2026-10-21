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

## 评审记录（变基；2026-10-21，Claude Code 作者子代理，分支 claude-dcg57）

票 88 进 main 之后变基到 main，其后 146+94 又进 main，再变基一次，最终基在 main `67438a0f3`（da12d4c2b 之后只进了 .scratch 文档）。上文各节里的 SHA 是变基之前的；变基后提交说明不变，按说明对得上。逐条去向：

- **迁移与种子号——已改，从引入那一笔起就带最终号。** transport_fulfillment `0049`、access_identity `0022`、settlement_accounting `0044`（未撞）；种子 `data/access/13`、`14` 与 `data/governance/49`、`50`，步 `457` 未撞。access_identity 迁移在变基解冲突时改名，CHECK 以 main 上 0021 为底只加本票两个词（核过：0022 的词集 = 0021 的词集 + 2，含 88 的 `TRANSPORT_COMMISSION_AGREEMENT_REVISION`）；`decisionKinds` 取并集。TF 迁移同样在引入那一笔的冲突里改名，后续改它的那一笔随改名合上。种子四个数据文件与 `steps/457` 里的路径用只改这几处的 `git filter-branch --tree-filter` 在全部本票提交上改号（前后两树只差这几处）；逐笔扫过，任何一笔里迁移目录、种子数据目录与步骤目录都没有同号两份。代码注释、ADR-0232 Consequences 与种子 README 里的号单独一笔（`d4a360fec`）。种子在一次性库上实灌一遍：迁移 TF 0049、access_identity 0022 施加，457 一节两段区间、两条授予、两条对照全部 RECORDED，88 的 475 一节照常。
- **`ListByCommission` 与 `FindCurrentByCommission` 统一——已做，`2ea72320c`。** 取 88 的端口与名字（`SegmentCommissionAdoptionsByCommission.ListByCommission`），实现改按 `currentAdoptionCondition` 只交此刻仍采用的段、各段当前那一代；解除编排改调它，本票那一口删去。票 160 的列段那一半随之做完（票 160 记一笔）；未形成条目作罢那一半留给 160。
- **`formedUnder` 与委托换版——已修，`b0040ac4d`。** 选「与委托上全部协议快照比」：委托多一问 `BoundAgreementSnapshot`（提交那一版或任一次换版的后继）。不让发生项记下委托标识：那要动发生项册与已形成信封，换版又只在同一协议对象内换，服务方加这一集快照已分得开。用例：编排面 `TestAnOccurrenceFormedUnderARevisedAgreementBlocksTheWithdrawal`，真库 `cmd/parcel-dispatch/actual_fulfillment_after_invalidation_test.go::TestAnOccurrenceFormedUnderTheRevisedAgreementBlocksTheWithdrawal`（换版在 SIN 交接之前、自动形成按 v2、解除答 `OCCURRENCES_FORMED`）；改回只比提交那一版，两条都红。
- **88 的补形成复活失效项——已修，`d12d034d6`。** 登记命令多一格 `Automatic`，只有自动形成那一处（`formEnded`，交接、交付与补形成三路共用）置真。链头已失效且这一段的当前采用落册不晚于那一次失效：答已存在、细格 `OCCURRENCE_INVALIDATED_BY_DECISION`、回显失效版本，不长出替代版本；采用晚于失效（解除之后再采用，或失效之后才采用）照常长出——指错了委托的更正因此是「失效 → 解除 → 再采用」三步，再采用的门铃自己把那一项按新的连线补回来。操作者登记不受这一条管。
  - **与 `formationDefectOf` 的关系**：新加的是一格答复，不是错误，`formationDefectOf` 不必认它。已存在本就由 `registrationDisposition` 判入账、由 `keepUnformedRegister` 照「已形成」了结册上那一条（指名失效版本）——那一件不是待补的缺口，是操作者的决定，不该当数据缺陷进册、更不该当编程缺陷整封失败。读采用读不动答未决 `PROCUREMENT_SOURCE_UNAVAILABLE`，照既有那一格重投。
  - 用例：编排面 `register_charge_occurrence_automatic_test.go::TestAnAutomaticRegistrationDoesNotRegrowAnInvalidatedOccurrence`；真库 `TestAnInvalidatedOccurrenceIsNotRegrownByCatchUpUntilTheSegmentIsAdoptedAgain`（SIN 交接自动形成 → 人工失效 → 换版 → 换版门铃 PUBLISHED、仍只有 V1 与失效版本 V2 → 解除 → 接第 1 代再采用 → 采用门铃补形成长出 V3、按 v2）。拆掉这一条修法，真库那条在「换版补形成之后各版」处红，读到 V3。
- **AT-SA-211**：main 上没被占（main 的 AT-SA 到 201）。AT-TF 本票的 306–315 与 146+94 的 316–328 不撞。
- **变基中顺手补的**：88 让采用编排的 `Doorbell`、本票让 `CommissionHold`、`Withdrawals` 成为必备，两边各自新写的测试装配只带自己那几口，合到一起之后缺口处空指针；补齐（`f5fc13d9a`）。88 把发生项身份的派生收进 `occurrenceIdentity` 之后，登记用例里本票读链头那两处改用 `key.Occurrence`（解冲突时改，在引入那一笔里）。
- 文档：ADR-0232 决定一（委托各版快照）、决定五（自动形成不复活）、越权风险点 4、6 改写、新增 16（按落册时刻比「失效之后才采用」），Links 加 ADR-0231；ADR-0231 Links 加后继一行（正文不改）；TF CONTEXT 发生项生命周期一句；`AT-TF-309`、`AT-TF-312` 两行补格；AT 盘点只在这两行加用例。

**未采纳**：无。

**验证**：变基过程中每一笔都跑过 `go build ./... && go vet ./...`（`git rebase` 的 exec），filter-branch 只改种子文件名与步骤里的路径；`gofmt -l` 干净。`scripts/test-full.sh` 覆盖 TF、SA、access-identity 的包与全部反向依赖（含 cmd/parcel-api、parcel-dispatch、parcel-access-register、parcel-settlement-register、parcel-transport-register）、`./internal/architecture/...`、`./migrations/...`：51 个包 ok、0 FAIL。变基后没走 exec 的前几笔另用 `git archive` 单独 build、vet 过。

## 评审记录（二轮；2026-10-21，Claude Code 作者子代理，分支 claude-dcg57）

二轮评审无阻断。逐条去向：

- **1 未形成册把失效版本记成「已形成」——已修，`8749495c2`。** 选「另立一个状态并了结那一条」：未形成册多一格 `INVALIDATED_BY_DECISION`（记下失效版本，迁移 TF 0049 放开状态与了结两条约束），端口多一口 `SettleInvalidatedByDecision`；`keepUnformedRegister` 遇已存在、细格 `OCCURRENCE_INVALIDATED_BY_DECISION` 单独走这一口，不再走已形成那一支。查阅面只列 `UNFORMED`，新状态不上列；端口与 TF CONTEXT 同步。不选「不写」：册上已有的那一条（例如先答过未配置、后来形成又被失效）就会停在 FORMED 或 UNFORMED 上，两种都说错话。
  - 自动形成的失效链头判定提到协议核对之前（锁外先判，锁下照旧再判，同一个 `keptInvalidated`），失效链头 + 委托换到一版没声明条件的协议不再答未配置、不立那一条。
  - 同族一处一并改：补形成问「盖住」按身份只读首版（`coveringOccurrence`），失效之后也当成已盖住、了结为已形成指向 V1——改读链头、链头已失效不算盖住。这部分停用 ADR-0231 决定六「状态三格」与「按身份读只读首版」两句（ADR-0232「对已接受记录的作用」、ADR-0231 Status 与 Links 前向指针），回答 ADR-0231 越权风险点 10。
  - 真库用例 `TestCatchUpOnAnInvalidatedOccurrenceSettlesTheUnformedRegisterAsInvalidatedByDecision` 两格：册上已有那一条 → 了结为 `INVALIDATED_BY_DECISION`、指名 V2、查阅页不列；册上没有 → 失效链头换到无条件协议不立未配置那一条。拆掉了结那一支，前一格红（读到 FORMED）；拆掉提前那一判，后一格红（立了 NOT_CONFIGURED）——后一格只有在「盖住」改读链头之后才测得出，读首版时那一判拆了也被首版「盖住」掩过去。库面用例 `TestTheUnformedRegisterSettlesAnInvalidatedOccurrenceAsInvalidatedByDecision`；无事务负例加进 88 的 `TestTheUnformedRegisterRefusesWhatItCannotHold`（证据门）。
- **2 中段提交测试红——已重写。** 用 `git filter-branch --tree-filter`（不用 rebase -i）把夹具补丁按各夹具引入的那一笔折进去：解除那一笔（`1804e65ac`）补 Doorbell 与 88 补形成用例的 CommissionHold，B1 那一笔（`0e3d71b9f`）补 Withdrawals 与编排面用例的 Doorbell，parcel-api 并发用例在引入它的那一笔补 outbox 门铃；原补丁那一笔重写后成空、已删。重写前后最终树逐字节相同（`git diff` 0 字节）。抽验原红的三笔（解除那一笔、B1、评审 6 那一笔）在 `./internal/transportfulfillment/...` 与 `./cmd/parcel-api/` 上真库全绿。
- **3 首次采用的 201 多了 `generation`——写明，不改。** ADR-0232 决定一与 UC-TF-004 `AT-TF-310` 写明它是新增回显字段、只加不改；段此刻已有当前采用时同一份委托再交答 `EXISTING_ADOPTION`、不看 `afterGeneration`，写明是有意的宽松接受（那一次什么都不落）。探针重跑：首次采用 201 COMMISSION_ADOPTED、原样再交 200 EXISTING_ADOPTION、带 `afterGeneration:1` 再交 200 EXISTING_ADOPTION，三份响应去掉时刻后与评审留下的分支探针逐字相同；前两份去掉 `generation` 与 main 探针相同（main 上第三份答 MALFORMED_REQUEST，是 main 不认这一格）。
- **4 `adoptedAfter` 的注释——已修，`8749495c2`。** 判法收进 `keptInvalidated`（锁外、锁下两处共用），被顶上去的 `errProcurementSourceUnreadable` 注释移回原变量；锁序改为「解除在发生项范围锁下排过先后；再采用经门铃，在它那一笔提交之后才触发补形成」。
- **5 采用与失效同一刻——已补，`1a824e954`。** `TestAnAdoptionRecordedAtTheSameMomentAsTheInvalidationDoesNotReopenIt`：同一刻答已存在、不长出，晚一微秒才长；变异成 `!Before` 即红。ADR-0232 决定五与越权风险点 16 写明「同一刻算不晚于」。
- **6 种子 README 更正次序——已改。** 结尾改为「再采用的门铃补形成自动从失效版本长出替代版本；人工重登也只会答已存在」。

**未采纳**：无。

**验证**：变基到 main `8e9014e87`（67438a0f 之后只进了 `.scratch` 文档，冲突只在本票与票 15 的票面，按并集合）。`go build ./... && go vet ./...`、`gofmt -l` 干净；`scripts/test-full.sh` 覆盖 TF、SA、access-identity 的包与全部反向依赖（含各 cmd 包）、`./internal/architecture/...`、`./migrations/...`：52 个包 ok、0 FAIL。种子在一次性库上实灌：TF 0049、access_identity 0022 施加，457 一节全部 RECORDED/REGISTERED，库已删。
