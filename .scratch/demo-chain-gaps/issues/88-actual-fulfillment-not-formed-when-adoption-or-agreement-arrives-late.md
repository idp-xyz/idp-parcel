# 88 自动形成没赶上的参与不补：采用晚到、协议后补条件的段，实际履约发生项要人记得去登

Category: enhancement
Status: in-progress——评审已修，待复评与进 main，分支 claude-dcg88。此前 in-progress——完工待评审；2026-10-21 认领；ready-for-agent——2026-10-20 上线就绪评估归 R1（上线前必修），裁定与批次见 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」。此前 needs-triage——2026-10-19 票 [67](./67-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0218](../../../docs/adr/0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 越权风险点 1、6。
Blocked by: 无

## 现象

- 实际履约发生项随交接与有效交付信封自动形成（ADR-0218），判的是派发处理这封信的那一刻：段那时有没有采用连线，委托采用的那一版协议那时声明了什么。
- ADR-0204 允许参与结束之后才采用委托（采用只要求段有当前参与、首个参与不早于委托提交）。采用晚于派发处理时，那封信早已入账成「段没有采用连线」，没有任何东西回头再形成；采用若落在参与结束与派发处理之间，照样会形成。同样是参与结束之后的采用，形不形成随派发时序而定。
- 协议那时答未配置或不计费，租户随后发一版声明了条件的协议，同样没有东西回头重跑；何况委托记着提交时那一版、不改指，新的一版怎样作用到这份委托上本身还没裁。
- 结算那一侧看不到「该有而没有」：ADR-0207 越权风险点 3 说的那一格，在这两种情形下仍然成立，补登全凭人记。

## 要裁的

- **采用落下时回头形成**：采用编排同笔查这一段上已结束的参与，逐件交同一个登记用例。这会把发生项登记带进采用那一笔，要看与 ADR-0207 决定七前半句「不内联」的关系——采用是运营决定，不是物理编排，但同一笔失败会连累采用。另一种是采用之后交一封门铃，由派发进程逐件补形成。
- **协议后补条件**：委托不改指协议（ADR-0204 决定一：委托快照是采购的权威），协议的新一版要怎样作用到已结束的参与上，是产品策略，要先裁「补登的业务时间仍按参与终点核哪一版」。
- **只留痕不自动**：退一步，TF 立一张「已结束而未形成」的查阅面，由运营照面补登。
- 三类：回头形成的触发与留痕是机制；协议后补时按哪一版判是产品策略；哪几段要补是租户取值。

## 规模

M

## 出自票 50 评审（2026-10-21）

票 [50](./50-commercial-versions-have-no-closing-entry-so-agreement-versions-pile-up.md) 开了商业版本的两种收尾（[ADR-0233](../../../docs/adr/0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md)）：截止与同起点退役。收尾之后，运输履约按版本点读核协议：截止点之后答未生效，退役之后答已终止。自动形成实际履约发生项那一路（`registrationDisposition`）把这两格都判作已了结——不重投、不留痕，成本静默不形成（ADR-0233 越权风险点 1）。所以本票的**重跑与「已结束而未形成」查阅面必须覆盖收尾造成的 `AGREEMENT_TERMINATED` / `AGREEMENT_NOT_EFFECTIVE` 两格**：委托协议换版之后按新一版重跑，查阅面要能列出因旧版被收尾而未形成的那些参与。本票落地之前，ADR-0233 记了运营约束：不得截止或退役仍被在途委托指名的供应商协议版本。

## 完成记录（2026-10-21，分支 claude-dcg88，基于 main `b290b777`）

裁定的单一权威是 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」关于 88 的那一条；落成 [ADR-0231](../../../docs/adr/0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md)。

### 做了什么

- **采用晚到**：采用成立的那一笔交一封门铃（`transport-fulfillment.segment-commission-adoption.registered`，不内联形成，ADR-0207 决定七前半句不变）；门铃入不了队采用答未决、整笔回滚。派发进程收到门铃，复核采用仍在册，把段上以下一次交接或有效交付结束的参与逐件交回自动形成的同一个形成后半（`formEnded`，同一个登记用例、旅程取法与系统登记者），所以重投与重放答已存在、不多一项。明确终止结束的不补。
- **协议后补条件 / 旧版被收尾**：运营决定新种类「委托协议换版」（`TRANSPORT_COMMISSION_AGREEMENT_REVISION`，口 `POST /transport-fulfillment-transport-commission-agreement-revisions`）：指名同一协议对象的后继版本、起用时刻与依据；先取换版锁再读委托，后继在起用时刻按委托的服务方核，委托出新一版（第 2 版起，另册只插不改）、旧版留历史，同一笔交换版门铃；不随 PC 新版自动改指。换版门铃在消费那一刻按委托列出采用它的各段逐段补形成。
- **取版**：发生项登记从委托取采购上下文时（订舱、取消、已采用委托的实际履约），协议版本、快照与法人取委托在事实业务时间所凭的那一版（起用不晚于业务时间的换版里最后登记的那一版，没有就是提交那一版）；答复带回作答那一版的号与问 PC 的那一版，冲突回显那一版。
- **已结束而未形成**：TF 实际履约未形成册——登记答未配置、协议未生效、已终止、缺采购上下文时立或刷新一条（先问是否已有一项盖住，换版之后参与终点所凭的那一版可能已不是当初形成它的那一版），形成或认出已盖住了结，不计费只了结已在册的；只在形成后半一处写、与 inbox 同笔；查阅页 `GET /transport-fulfillment-records?registry=actual-fulfillment-unformed` 只列本租户未形成的。覆盖票 50 评审要的收尾两格。
- 文档：ADR-0231；ADR-0204、0207、0218、0233 Status 行与 Links 前向指针；TF CONTEXT「委托协议换版」「实际履约未形成」两词条与成本来源规则；UC-TF-004 `AT-TF-296`–`299`、UC-TF-005 `AT-TF-300`–`305`；演示动线补记；AT 盘点加行（总览未重算）。
- 迁移：transport_fulfillment `0048`（换版册与未形成册同一份）；access_identity `0021`（决定种类 CHECK 以 0020 为底取并集加一词）。种子：`steps/475-access-commission-agreement-revision.sh`，数据 `data/access/11`、`12`、`data/governance/48`；一次性库 `dcg88_seed` 上 `seed.sh` 实灌零报错，灌完删库。

### 代决与越权风险点（全文见 ADR-0231）

代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」。票面与裁定没写、本票自决的几格：
- 换版带**起用时刻**（操作者给），事实按业务时间取起用不晚于它的、最后登记的那一版——截止的后继从截止点才生效，整份替换会把截止点之前结束、此前还没形成的参与变成按后继答未生效的死格。
- 换版只认**同一协议对象**；起用不早于委托提交；后继在起用时刻核一次。同一份依据是重放键，决定人不进内容比对。已取消的委托也能换版。
- 补形成**不补明确终止**结束的参与；**没有采用连线的段**与**从没进过册的不计费**不进册。
- 记未形成之前**先问盖住**（按发生项登记册的范围锁与首版，与登记用例同一读法）。
- 换版门铃在**消费那一刻**列段，不在换版那一笔里扇出。
- 订舱与取消不随换版自动重跑。

越权风险点（ADR-0231 文末，逐条）：动了 ADR-0204 委托快照语义；起用时刻由人给、给晚了留缝；换版只认同一协议对象；后登记的覆盖先登记的、没有撤销换版；已按旧版形成的不按新版重判（去处票 15、57 的发生项修订）；一封门铃一笔事务、数据缺陷进册而不响（日志照进程形状另建一只；与编程缺陷的分界只在 `formationDefectOf`）；不补明确终止；没有采用连线的段不进册；不计费不进册（ADR-0209 越权风险点 8 不变）；「盖住」按首版认；册是台账；补形成不重跑订舱与取消；解除采用之后册上那一段不作罢、按委托列段不分已解除（票 160）；换版锁是双 int 事务级咨询锁；收尾不再被运营约束挡住、承接是事后的（票 161）；换版门铃上一段读不回整封失败、不逐段记。

### 用例

`AT-TF-296`–`305`，Covers 点名见 AT 盘点。真库：采用晚于参与结束 → 门铃 → 补形成、BUY（`cmd/parcel-dispatch/actual_fulfillment_catch_up_test.go::TestALateAdoptionFormsTheParticipationThatHadAlreadyEnded`）；协议后补条件 → 换版 → 补形成，换版之前 PC 已发布新版而委托仍按旧版答未配置、册上一条、查阅页按租户（`::TestACommissionRevisionFormsWhatTheAgreementLaterDeclared`）；旧版被同起点退役 / 截止 → 已终止 / 未生效进册 → 换版到后继 → 补形成（`::TestAClosedAgreementVersionIsTracedAndRecoveredByRevision`）；换版口成立、重放、冲突、各格、没有授予 403、并发换版等在换版锁上先 2 后 3（`cmd/parcel-api/commission_agreement_revision_test.go`，拆锁实测变红：后一笔撞号报错、观察不到等锁）；未形成册与门铃库面、无事务即拒、确定性拒写（`internal/transportfulfillment/adapters/postgres/actual_fulfillment_catch_up_test.go`）。

### 验证

- `go build ./...`、`go vet ./...` 干净，变更的 Go 文件 `gofmt -l` 干净。
- `scripts/test-full.sh` 跑变更包及其反向依赖（含 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-access-register`、`cmd/parcel-transport-register`）、`./internal/architecture/...`、`./migrations/...`，共 39 包：36 ok、3 无测试文件、0 FAIL。清单与日志在会话 scratchpad `dcg88/test-pkgs.txt`、`test-run.log`。
- 没跑全量；没重生成机制清点、ADR 索引与 AT 计数。

### 没做与去向

- 解除采用（票 57）之后未形成条目的作罢、按委托列段只列仍在采用的：票 [160](./160-unadoption-leaves-actual-fulfillment-unformed-entries-and-revision-catch-up-lists-unadopted-segments.md)，Blocked by 57。
- 收尾在途委托指名的版本时没有提醒：票 [161](./161-closing-an-agreement-version-named-by-in-flight-commissions-raises-no-prompt.md)。
- 已形成的发生项按新版重计：去处票 15、57 的发生项修订入口。
- 与票 57 同改的文件（进 main 时后到的一方合并）：`internal/transportfulfillment/application/adopt_segment_commission.go`（本票加门铃依赖与 `ringAdopted`）、`cmd/parcel-api/assemble_segment_commission_adoption.go` 与它的测试、`internal/transportfulfillment/adapters/http/register_charge_occurrence.go`、`internal/transportfulfillment/adapters/accessidentity/operator_authenticator_test.go` 的决定映射表、`cmd/parcel-dispatch/actual_fulfillment_on_participation_end_test.go`、`internal/accessidentity/operation_decision.go` 的 `decisionKinds`（取并集）与 access_identity 决定种类 CHECK（本票 0021，票 57 另取的那一份要以 0021 为底取并集）。票 57 若改了采用册的读法（解除另记），本票的 `SegmentCommissionAdoptions.ListByCommission` 要随之只列仍在采用的（票 160）。

## 评审记录

### 一轮（2026-10-21，隔离评审；变基到 main `7b598e47` 之后修）

变基：票 145 先进 main、占了 transport_fulfillment/0047，本票的 TF 迁移改为 `0048`，代码、ADR 与票面引用随改；本票先于票 57 进 main，`0048` 归本票，票 57 顺延。access_identity `0021` 照用。UC-TF-004 与 AT 盘点同 145 的新行冲突，两边并存。

必修逐条：

1. **没换过版时冲突回显多出 `"agreementRevision":1`**——已修。`writeChargeOccurrenceOutcome` 只在作答的那一版是某一次换版（号 2 起）时填这一键；没换过版的订舱、取消、实际履约三种冲突回显与换版之前逐字节相同。用例 `internal/transportfulfillment/adapters/http/register_charge_occurrence_revision_echo_test.go::TestTheConflictEchoNamesARevisionOnlyWhenTheCommissionWasRevised`（没换版不出这一键、键数与换版前相同；换版后等于 2）；UC-TF-004 `AT-TF-299` 写明。
2. **采用门铃的信封 ID 在解除后再采用同一份委托时撞上**——已修。指纹加这一次采用的登记时刻（折到登记册精度，从册上读回重交仍是同一个 ID）；票 57 让采用册按代记之后换成代次。用例 `internal/transportfulfillment/adapters/postgres/actual_fulfillment_catch_up_test.go::TestEachAdoptionOfTheSameSegmentAndCommissionRingsItsOwnDoorbell`；ADR-0231 决定一与票 160 补这一层交互。
3. **「盖住」一问另抄了一份身份派生**——已修。依据、范围、身份与覆盖一问收进 `application/charge_occurrence_identity.go`（`actualFulfillmentScope`、`occurrenceIdentity`），登记用例与补形成都调它；补形成派生出错上抛按缺陷处置，读不动才答 `OCCURRENCE_REGISTRY_UNAVAILABLE` 重投。
4. **注释计数与枚举**——已修。`formEnded` 头注改成「交接、交付与补形成都走这一处」；补形成哨兵注释改成「确定性错误都不进名单」加符号名，不枚举。
5. **一件缺陷烧完预算后其余各件静默缺失**——选了第一条，已修：补形成里一件撞上确定性缺陷（旅程平局、登记用例上抛的数据违约、答出本路不该见的一格），记成未形成册上的 `FORMATION_DEFECT`（记下停在哪里；记之前先问盖住），这一件入账，接着问下一件，整封入账；未形成册连缺陷都记不下（确定性拒写）时才整封响亮失败。迁移 0048 的未形成册加缺陷一格（号与问的那一版在缺陷格可缺），查阅页带 `defect`。自动形成那两扇门一封只管一件，缺陷照 ADR-0218 决定六响亮失败、不进册。用例：编排 `catch_up_actual_fulfillment_test.go::TestADefectiveParticipationIsRecordedWhileTheRestOfTheDoorbellForms`、库面 `::TestTheUnformedRegisterKeepsAFormationDefect`、派发真库端到端 `cmd/parcel-dispatch/actual_fulfillment_catch_up_test.go::TestOneDefectiveParticipationDoesNotSinkTheLateAdoptionCatchUp`、查阅线格式。ADR-0231 决定二、六与越权风险点 6 改写（代价：缺陷不再表现为发布失败，要看查阅页；数据违约只留错误原文前段；缺陷修好之后要再换版或重新采用才会再问）。

未采纳：无。

验证：`go build ./...`、`go vet ./...`、变更 Go 文件 `gofmt -l` 干净；`scripts/test-full.sh ./internal/transportfulfillment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-transport-register/ ./cmd/parcel-access-register/ ./internal/accessidentity/... ./internal/architecture/... ./migrations/...`：23 ok、1 无测试文件、0 FAIL；一次性库 `dcg88_seed` 上 `seed.sh` 实灌零报错，灌完删库。
