# 90 履约尝试逐对象结果的「待确认」没有更正口：同一次上门的后补来源确定不了它

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code90（基于 main `a5544faff`，评审修订轮后重放到 `ae21dab5c`）。此前 needs-triage——建议 R2：code/01 落地后，揽收与派送尝试都收得下待确认，可同一次上门的后补来源（伙伴迟到的签收、复核后认定的地址）没有地方落成确定结果；派送一侧那件对象因此登不出有效交付，只能等新的上门。可靠绕法：真的再上门一次，登新尝试。2026-10-25 票 [docs-consistency/code/01](./01-fulfillment-attempt-object-outcomes-have-no-pending-confirmation.md) 作者会话立（用户「你作为业务和系统专家自决」授权），分支 claude-code01。
Blocked by: 无
上下文：TF（运输履约），揽收与派送两侧的履约尝试。
出处：code/01 代决（完成记录「代决 2」）。

## 现象

TF CONTEXT 要求待确认之后，同一对象凭同一次上门的后补来源以更正形成新判断版本；代码里没有这条口。待确认那一格照规则不改写，可它也就停在待确认：派送那件对象登不出有效交付，失败尝试费也等不到它确定。

## 证据（读码，只记 `S`）

- 规则：
  - TF CONTEXT「揽收、派送与交付」：之后同一对象的确定结果，凭同一次上门的后补来源以更正形成新判断版本，凭新的上门形成新尝试；待确认那一版留在册上不改写。
  - UC-TF-002「一致性、幂等与并发」：同一对象在同一尝试中只能有一个当前有效结果；更正或来源撤销形成新判断版本。外部伙伴状态迟到时按业务发生时间采用。
  - UC-TF-002 结果契约「揽收结果待确认」与 UC-TF-006「尝试待确认」要保存查询或复核入口。
- 代码：
  - `domain.FormPendingAttemptObjectResult`、`domain.FormPendingDeliveryAttemptResult` 形成待确认；尝试登记按来源身份（揽收）或尝试身份（派送）只收一次，同身份换说法答冲突，待确认那一格不改写。
  - 没有任何入口能对（尝试，对象）落一版回指待确认的确定结果，两张结果表都是一个对象一行、没有版本列：`transport_fulfillment.pickup_attempt_result` 主键 `(tenant_id, source_id, object_ref)`——按尝试提交的来源身份，不按尝试身份；`transport_fulfillment.delivery_attempt_result` 主键 `(tenant_id, attempt_ref, object_ref)`。
  - 派送一侧：`application.RegisterEffectiveDeliveryHandler` 只认那件对象在那次尝试里的结果是妥投，待确认答`派送结果待确认`（`DELIVERY_RESULT_PENDING_CONFIRMATION`）；同一次上门后来证实交付成立，也登不出有效交付。
  - 揽收一侧：对象级登记口（`/transport-fulfillment/offsite-pickups`，按（对象，尝试）幂等）登得进同一次尝试的揽收到手、不改写待确认那一格，但它不回指那一格；尝试登记那条链上那件对象仍读作待确认。失败或拒收的确定结果没有这样一条口。
  - 失败尝试费对待确认答`未受理`（`ATTEMPT_RESULT_PENDING_CONFIRMATION`），续办是「等确定之后再登」，而今天同一次上门确定不了。
  - 交付门禁读数（`application.DeliveryGateReader.ReadRegistration`）只为妥投问关务，待确认交回零值读数。
  - 没有运营查阅面列出还在待确认的对象（尝试登记两口都不在 TF 查阅页的登记册里）。

## 三类

机制：更正口与版本链、读当前有效版本的各消费方（有效交付、失败尝试费、交付门禁读数）。产品策略：待确认复核清单的排序与时限提醒若要，属产品策略，另判。租户取值：哪些后补来源够得上确定结果仍由既有证据规则判，不设默认。

## 规模

M–L：两张结果表各加版本链或另立更正表与迁移；领域更正方法；更正口的接入面；各消费方改读当前有效版本；揽收一侧更正成揽收到手时形成场外揽收、进段并交 parcel-shipment。

## 最小的诚实修法

1. 结果表加版本链，或另立一张更正表：原行（待确认那一版）不改写，更正行回指它、带新的确定结果、所凭来源与更正时刻。版本链挂在哪个键上要随之定：揽收一侧结果行按（租户，来源身份，对象）落，更正口若按尝试身份指名，就要先从尝试身份认回那一份提交（同一尝试身份是否只对一份提交，今天没有约束）；派送一侧按（租户，尝试身份，对象）落，直接挂。
2. 领域更正只许回指待确认那一版：确定结果不再更正成待确认，也不更正另一版确定结果（那是另一种更正，不在本票）；更正出来的那一版走既有确定结果的构造门（揽收到手不带依据、失败与拒收必带依据、妥投不带依据）。
3. 有效交付登记、失败尝试费登记与交付门禁读数改读（尝试，对象）的当前有效版本：更正成妥投之后同一份交付登记走得过去，更正成失败之后失败尝试费照常登。
4. 揽收一侧更正成揽收到手：形成场外揽收、进段、交 parcel-shipment 采认，与尝试登记里那条路同一套。
5. 更正口挂哪一族接入面待裁（见「要裁的」），照 ADR-0222 落特性文件。
6. 交付登记对待确认那一格的答复语义不变：没有更正之前，那件对象照旧答`派送结果待确认`，续办仍是先把那一格确定下来；更正落地只是让「确定下来」有了同一次上门的那条路，不把这一格并回`未生效`，也不改它的词。

## 要裁的

- 更正挂作业事实（执行方后补）还是运营决定（复核人认定），或两者都开。
- 揽收一侧更正成揽收到手，与对象级登记口同（对象，尝试）的那一版谁是当前有效、要不要互斥。
- 待确认复核清单要不要随本票一并开（运营查阅页一格），还是另票。

## 相关

- 票 code/01（待确认与执行方拒收两格）；ADR-0112（更正形成替代版本）；ADR-0029（按恢复动作分格）；UC-TF-002、UC-TF-006「给开发的交接」。

## 完成记录（2026-10-26，分支 claude-code90）

用户授权原话「你作为业务和系统专家自决」；下列「代决」由作者会话代 TF owner 作出，待 TF owner 复核。分诊：票面所述现状核实仍在（`a5544faff` 上没有任何入口对（尝试，对象）落回指待确认的确定结果；两张结果表仍是一个对象一行、没有版本列；对象级登记口不回指待确认那一格），定 R2，规模 L，本票一次做完更正口、版本读法、各消费方与复核清单。出处决定记录：[ADR-0297](../../../docs/adr/0297-a-pending-attempt-object-result-is-corrected-to-a-determinate-result-by-appending-a-correction-pointing-back-at-it-submitted-by-the-executor-or-decided-by-a-reviewer.md)（部分停用 ADR-0151 决定四一句）。

### 做了什么（照「最小的诚实修法」逐条）

1. **版本链**：另立两张只追加的更正册（TF 迁移 `0055`），不在结果表加版本列。更正行回指首次提交的待确认行：外键带 `corrects_outcome`（CHECK 钉死为 `PENDING_CONFIRMATION`）连主键三列指向结果行的 `（…，outcome）` 唯一键，库面就接不上一格已是确定结果的行；主键同结果行，一格待确认至多一条更正；首次提交的结果行、尝试行、揽收尝试摘要一个字不改。揽收一侧按（租户，来源身份，对象）落，同 `pickup_attempt_result` 的主键；派送一侧按（租户，尝试身份，对象）。
2. **领域更正只许回指待确认**：`domain.CorrectPendingAttemptObjectResult`、`domain.CorrectPendingDeliveryAttemptResult`。确定结果走既有构造门（失败与拒收必带依据、揽收到手与妥投不带依据、不早于到场、对象在尝试范围内），必带所凭的后补来源，更正成待确认与对已确定判断的更正都不接（后者答`并非待确认`）。业务发生时刻可以早于待确认那一版，更正这一判断不早于它所更正的待确认与它说的那件事。
3. **各消费方改读当前有效结果**：`DeliveryAttempts.LoadDeliveryResult` 外连更正册读「首次提交，被更正则取更正」，交付登记、交付门禁读数与失败尝试费（派送一侧）三个读方一处改；揽收一侧失败尝试费经 `FailedAttemptSource` 的新读法 `FindEffectiveByKey`（原 `FindByKey` 改名，按当前有效结果答；适配器同一只，两处装配不用动）。尝试登记的重放与冲突比对仍读首次提交（`FindByKey`）——同一次提交的重放不因更正而变成冲突。
4. **更正成揽收到手**：经对象级揽收登记（首登口那同一条编排，同一组缝）形成场外揽收、进声明的段、铸判断首版、入队 `offsite-pickup.registered` 交 parcel-shipment；同（对象，尝试）已登过同内容的揽收（此前的绕法）收口、不登第二份、不再进一次段，登过的是另一回事则答冲突、不写更正。登记写在更正追加之前、同一笔事务，装配事务壳对「成立」以外的任一格整笔回滚。更正晚到、对象此后已有更晚的控制事实时，登记不带段声明、答 `LATER_CONTROL_FACTS`（评审修订轮，见文末）。
5. **接入面**：两个面同一份编排——执行方在作业事实面后补（`/transport-fulfillment/offsite-pickup-attempt-result-corrections`、`/transport-fulfillment/delivery-attempt-result-corrections`），复核人在运营决定面认定（`/transport-fulfillment-pickup-attempt-result-confirmations`、`/transport-fulfillment-delivery-attempt-result-confirmations`，决定种类 `PICKUP_ATTEMPT_RESULT_CONFIRMATION`、`DELIVERY_ATTEMPT_RESULT_CONFIRMATION`，access_identity 迁移 `0027`）。照 ADR-0222：新特性文件 `cmd/parcel-api/endpoints_attempt_result_corrections.go` 加 assemble 文件加特性测试里的未接线占位；未动 endpoints.go、main.go、endpoints_test.go、routes.go 与 `cmd/parcel-dispatch/routes.go`。查阅册的未接线占位（`unwired_orchestration.go`）多一个读方法，既有 assemble 文件一个没动。
6. **待确认那一格的答复语义不变**：交付登记对仍是待确认的那一格照旧答 `DELIVERY_RESULT_PENDING_CONFIRMATION`、失败尝试费照旧答 `ATTEMPT_RESULT_PENDING_CONFIRMATION`，不并回`未生效`、不改词；更正落地只是让「先把那一格确定下来」有了同一次上门的路。
7. **复核清单**（票面「要裁的」第三条）：运输履约查阅页加 `pending-attempt-result` 册，列还在待确认的对象（两侧合并、最早的先看、带来源与缺口和更正口要指名的键），已更正的靠反连接不再列。
8. 文档：TF CONTEXT「揽收、派送与交付」补两条规则句；UC-TF-002、UC-TF-006 删去指向本票的差距行并改状态行，新立 `AT-TF-357`、`358`、`360`、`361`、`362`（UC-TF-002）、`AT-TF-359`（UC-TF-006）；演示种子加授予、准入对照与权威区间（步 `481`）；ADR-0151 Status 与 Links 加前向指针。

### 代决

1. **更正挂作业事实还是运营决定：两者都开，同一份编排。** 票面两个例子来源不同：伙伴迟到的签收是执行方（或其伙伴）后补，随原事实的族挂作业事实面（ADR-0151 决定四原样适用，认证与授予就是尝试登记那一份）；复核后认定的地址是复核人核对来源作出的判断，没有设备，挂运营决定面（ADR-0151 定族判据：提交方是谁、授予维度是什么）。只开前者，第二个例子无人能做；只开后者，执行方为改自己登的东西要找复核人，是 ADR-0151 决定四自己担心的「能登不能改」。揽收与派送各成一种决定，与建派送任务、终止参与、承运商首次有效收寄判断都分开授予。提交操作者作为操作证据落在更正行上，待遇照 ADR-0303（只进审计、不进重放比对与任何门、回执不回显）；因提交人不再是登记尝试的设备，部分停用（收窄）ADR-0151 决定四「更正与首登的提交方是同一方，认证与授予也应是同一份」一句，用预留号 0297，写法照 ADR-0236 对外部资金事实更正的做法。
2. **更正什么：只更正待确认那一版，更正出来的确定结果过与首次提交同一道构造门。** 不选「更正可以改任何一版」：对已确定判断的更正是另一种更正（要裁的是谁有权推翻一个确定判断），不是本票的待确认收尾。不选「更正放宽构造门」：放宽就会放进首次提交进不来的形状。
3. **形状：追加表，不加版本列。** 所有读结果表的口（重放比对、读回的接力核对、失败尝试费、交付登记）若都改认版本，漏改的一口会无声读到过期的一版；追加表让「原版不动」靠形状、读哪一版靠两个具名的读口。
4. **揽收一侧更正按尝试提交的来源身份指名，不按尝试身份。** 结果行按来源身份落，同一尝试身份是否只对一份提交今天没有约束，按尝试身份指名会指不准。
5. **揽收一侧更正成揽收到手与对象级登记口同（对象，尝试）的那一版谁是当前有效、要不要互斥：对象级登记就是那一份揽收事实，更正是把待确认那一格绑到它；因此互斥靠构造，不靠再造一道检查。** 更正成揽收到手不另在尝试链上形成 `pickup_attempt_pickup`（尝试级 `offsite-pickup.formed` 信封没有消费者，那样形成的揽收既不被 parcel-shipment 采认、又与对象级登记各说一样），而是转交首登口那条编排，同（对象，尝试）已登记的同内容重放读回在册那份，不同内容答冲突。
6. **更正命令带段声明（缺席不立段），不沿用首登时的声明。** 待确认那一格从没带对象进过段，这是对象的第一次揽收，不是对已有参与的重派生，ADR-0112 决定二「更正命令不带段号」说的是后一种。沿用首登声明要给尝试与结果行加列并改尝试登记口（ADR-0307 决定二为交接做了），记为越权风险点 3。
7. **各消费方读当前有效结果、重放读首次提交。** 同一次提交的重放不因更正而变成冲突；失败尝试费、交付登记与交付门禁读数读更正之后的结果。
8. **复核清单随本票开，一册；排序的时限提醒、超期告警不做。** 票 code/01 越权风险点 3 已把「查阅清单」托给本票，没有清单，运营认定面无处下手；时限与提醒是产品策略，另判。
9. **更正成妥投不带 POD、不转移控制。** 有效交付仍由交付登记带着 POD 另登，门禁读数与参与结束一字不改。

三类：机制本票做完（更正册与读法、各消费方、两个接入面、复核清单）；产品策略：待确认复核的排序与时限提醒不做（另判）；租户取值：哪些后补来源够得上确定结果仍由既有证据规则判、不设默认，谁持哪一种认定授予是租户取值。证据层级 `S`。

### 越权风险点

1. **ADR-0151 决定四一句的收窄**（TF owner、PC owner）：认定另挂运营决定面使「更正与首登同一方」对这两口不成立。若 owner 认为复核后认定也该走设备（作业事实面），撤两个运营决定口与两种决定种类的授予，更正册与作业事实面不动。
2. **复核人能认定的范围**：两种决定只约束谁能认定，不约束认定时要有什么证据；所凭来源是一个引用，产品不核它的内容。若要第二人复核或来源格式门，是产品策略，加在认定口上。
3. **段声明由命令带，复核人未必知道**：运营决定面认定揽收到手时若不知道当初声明的段，只能留空，对象于是登了揽收而不在任何段里（段那一半另说，归票 demo-chain-gaps/179）。沿用首登声明要改尝试登记口的存储形状，未做。
4. **管理台没有认定页**：清单与认定口都在后端，`apps/admin-web` 没有这一册的页签和认定动作（共享件与运营页另一会话在改，本票未碰）；上线前认定只能经 API。
5. **晚到更正的段那一半不重算；对象第二次揽收的责任起点归采用先后；只拦「更晚」，不拦「已在别的段里在场」**：更正成揽收到手前先问 ADR-0307 决定三的那一问（评审修订轮补的，N1），命中时场外揽收照登、不带段声明、答 `LATER_CONTROL_FACTS`，该对象在声明的段里没有参与（不立段、无实际承运商判断首版）。对象于是可能有两个不同尝试的场外揽收、发两封 `offsite-pickup.registered`：parcel-shipment 的采用按责任起点唯一处理，先到的采用为责任起点，后到的（不声明更正关系的新一次尝试）落一份 `SOURCE_NOT_ADOPTED`、依据 `RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/<先到的版本>`，确定性、入账收工，不报错、不写第二份采用（核查记录见文末）；责任起点因此取先到的那封而不是业务时间更早的那份，是 parcel-shipment 既有口径（`AT-PS-049`）。拦截只覆盖「此后已有更晚的入场或离场」：对象在更正所说的发生时刻之前已在别的段里在场、尚未离场时，更正成揽收到手仍会让它进声明的段——对象级揽收首登对这种对象同样不拦，这里不另补。
6. **更正册里存的控制依据是「更正当时所报」，不是当前控制**：只为重放比对留；对象级登记之后又经揽收更正口改了控制依据，重放更正拿旧控制依据比，可能答冲突。
7. **尝试级「揽收到手」结果与对象级登记对同（对象，尝试）今天不互斥**（更早就有）：本票只保证更正路径不制造第二份。
8. **更正成揽收到手的重放不重发登记意图**：重放答已有更正、不再调登记编排，登记时交意图失败留下的续办引用只能经对象级登记口原样重放来重发。
9. **access_identity 迁移 `0027` 的决定种类 CHECK 是 `0026` 那一份的超集**（`0026` 在在途分支 `claude-code76`，含 `CANCELLATION_ORDER_CONFLICT_RESOLUTION`，尚未进 main）。若它被改号或落在本迁移之后施加，要核对并集；重建这条 CHECK 的迁移都取已有各份的并集。
10. **演示种子编号**：`data/access/22`、`23`、`data/governance/59`、`60`、`steps/481` 取开工时 main 的下一个空号；进 main 时若另有分支占同号，改号即可。

### 验证

- 先红后绿：领域用例先写、编译不过（符号不在）再实现；编排、真库与传输面的用例随实现一层层写，生产装配用例的红由下列变异补证（实现先于测试的层都以变异证红）。
- 变异（每条改后跑相关用例、跑完从 `scratchpad/c90/mut/` 的备份复原、`cmp` 核过）：M1 去掉结果行 `FOR UPDATE`（锁）——并发排队用例红；M2 去掉「只回指待确认」的门——领域用例红；M3 `LoadDeliveryResult` 不叠更正——派送真库用例红；M4 事务壳对非成立格不回滚——「更正册写不进则登记一并撤回」用例红；M5 重放比对不比控制依据——换控制依据的重放用例红；M6 `FindEffectiveByKey` 不叠更正——当前有效结果用例红；M7 更正成揽收到手不转交登记——编排四条红；M8 迁移去掉回指待确认的外键——库面 CHECK/外键用例红；M9 复核清单去掉租户条件——租户隔离用例红。九处全被杀。
- `go build ./... && go vet ./...` 干净，`gofmt -l` 无输出。
- `TMPDIR=… scripts/test-full.sh`：范围 = 改动包 + `go list -test` 算出的反向依赖 + `./cmd/parcel-api/` + `./cmd/parcel-dispatch/` + `./internal/architecture/...` + `./migrations/...`，共 92 个包，重放到 `06649ed18` 之后跑：88 个包 ok、4 个无测试文件，退出码 0。
- 新用例 `-count=3`（真库，`-v` 核过无 SKIP、无 FAIL）：138 次 PASS；并发排队用例 `-count=5` 通过。
- 种子：一次性库上 `scripts/demo-seeds/seed.sh` 退出码 0，四笔（两授予、两对照）落 RECORDED，用后删库。
- `go run ./tools/at-coverage-counts -check` 退 1（只是总览计数过期，未退 2：新立六个 AT 号在分桶表都有行）；`go run ./tools/adr-index -check` 退 1（只是索引生成物过期），生成一次核过 ADR-0297 与 ADR-0151 两行解析正常、README 已从备份复原。两个生成物不提交，由协调方统一重生成。

### 迁移号 / ADR 号 / AT 号

TF 迁移 `0055_attempt_result_corrections.sql`；access_identity 迁移 `0027_attempt_result_confirmation_decisions.sql`；ADR-0297；AT-TF-357 至 363（`363` 为评审修订轮所立）。

### 评审记录与修订轮（2026-10-27）

独立评审判**可合入**，无阻断；合入前要补的一条（N1）、一批测试缺口（N2）与规范小项（N3），本轮在同一分支上加提交补齐。

**N1 晚到更正不问「更晚的控制事实」（评审实测复现：登记 A1 里对象 X 为待确认，X 凭 A2 经对象级登记口进 SEG-A2，作业事实面把 A1 更正成揽收到手并声明 SEG-A1，更正答 201 且没有拒绝格，`FindActiveSegments(X)` 返回两段，之后交付登记按对象找段答 `ErrObjectActiveInSeveralSegments`、整笔回滚）。**
- 修法：更正成揽收到手、转交对象级揽收登记之前，`CorrectPickupAttemptResultHandler` 先问 ADR-0307 决定三的同一问。`laterControlFacts` 改成吃（租户、对象、发生时刻），交接首登与更正、揽收更正共用一份，不另写；命中时登记**不带命令的段声明**（场外揽收事实照落、意图照交 parcel-shipment，不立声明的段、对象不加入它），答复带 `segmentEntryRefusal: LATER_CONTROL_FACTS`、不带续办引用。取「不带段声明登记」而不取「拒绝这次更正」：揽收事实本身是真的，只是不该再造第二个段（ADR-0307 对交接首登取的是同一取舍：不拒登）。
- 答复格：`LATER_CONTROL_FACTS` 是 `SegmentEntryRefusal` 既有的一格，更正的 HTTP 译装沿用对象级揽收登记那个 `segmentEntryRefusal` 字段，没有新增格，无落到 default 的消费方；确定性拒绝，不进未决哨兵（ADR-0029）。命令没声明段、或同（对象，尝试）已登过同内容揽收（读回在册那份、没碰段）时答复不带这一格。登记册读不动不在更正里作答——转交的登记要读同一份册，读不动它自己会留进段的欠账。
- 装配：`newPickupAttemptResultCorrection` 把同一份段登记册也交给更正处理器（`Segments`，可缺席，缺席时不问）。
- 文档：ADR-0297 决定四加一条，越权风险点 5 重写；TF CONTEXT 更正规则句补一句；UC-TF-002 立 `AT-TF-363`；盘点追记同步。
- 测试：生产装配真库复现场景 `TestALateCorrectionToAPickupLeavesTheObjectInItsOneSegmentSoItsDeliveryStillRegisters`（同一次到访两件对象，parcel-1 晚到、parcel-2 没有更晚控制事实：更正后 `FindActiveSegments(parcel-1)` 仍只有 SEG-A2，SEG-A1 里只有 parcel-2，场外揽收一行、信封一封，之后交付登记 201 并结束参与）；编排四条（晚到、同一刻不算更晚照常进段、没声明段不带拒绝格、读回在册那份不带拒绝格）。
- PS 对同一对象第二封 `registered` 信封的核查（只读，没有改 PS）：`AdoptOnOffsitePickupAdapter` 按信封里的（对象，尝试，版本）取回那一代，转 `AdoptNetworkIntakeHandler`。采用键带来源版本，所以第二个尝试的版本不撞幂等；之后 `FindResponsibilityStart` 命中先到的那一份，第二份来源不声明更正关系，于是 `refuse` 落一份 `SOURCE_NOT_ADOPTED`，依据 `RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/<先到的版本>`；`adoptconsume.Consumption` 把 `SOURCE_NOT_ADOPTED` 当已消费入账。即：**确定性拒绝、不报错、不写第二份采用、消费门不重投**，既有用例 `TestACompetingSourceIsStillRefusedAfterACorrection` 钉的就是这一格。PS 侧不会因此坏掉。留意的口径：责任起点取先到的那封，不是业务时间更早的那份（这里 A2 的信封先入队，A1 晚到的更正登记反而被拒），是 PS 的既有规则（`AT-PS-049`），不是本票引入；越权风险点 5 已记。可见性（VE）侧两个消费者按（对象，尝试）各派生各的事实，键不同，不互相覆盖。

**N2 测试没盖住的存活变异（生产代码是对的，补用例，变异已验红）**
1. 重放比对：`TestAPickupCorrectionReplayWithAnotherBasisOrCorrectionTimeConflicts`、`TestADeliveryCorrectionReplayWithAnotherBasisOrCorrectionTimeConflicts`（换依据、换更正时刻再交答 `SOURCE_CONFLICT`，原样再交答已有更正）。
2. 派送一侧更正早于待确认那一版：应用层 `TestADeliveryCorrectionEarlierThanThePendingVersionIsNotAccepted`，领域层在 `TestADeliveryCorrectionGoesThroughTheDeterminateGatesAndPointsBackAtPending` 补两格（更正早于待确认那一版、更正早于它说的那件事）。
3. 派送一侧结果行锁：`TestConcurrentCorrectionsOfOnePendingDeliveryResultQueueOnItsRowLock`（仿揽收一侧，`-count=5`）。
4. 运营决定面自报租户：传输面 `TestTheAttemptResultConfirmationFacesTakeTheTenantOnlyFromTheEnvelope`，生产装配 `TestAnOperatorOfAnotherTenantCannotConfirmThisTenantsAttemptResultsByNamingItInTheRequest`（租户 B 的复核人持两种认定授予，带 `?tenant=` 与 `X-Reported-Tenant` 认定租户 A 的待确认：答 `来源未受理`、两张更正册零行；载荷夹带租户答 400；租户 A 自己的复核人对照成立）。
5. 跨租户：`TestTwoTenantsWithTheSameAttemptAndObjectNeverSeeOrLockEachOthersCorrections`（两租户同来源身份/尝试身份、同对象，A 更正后 B 的加载、当前有效读、交付读数仍是待确认；A 才有的尝试 B 指名不到；A 持结果行锁时 B 加载同键的两格不被堵住）。
- 变异结果见下「验证」。

**N3 规范小项**
- 迁移 `0027` 头注改成不带时态的说法（`0026` 的超集，重建 CHECK 的迁移取已有各份的并集），CHECK 内容未动。
- 本分支新增行里的变更叙述用词（「此前」「照旧」「不再」「改答」「今天」等）改成现行规则句：AT-TF-358、测试注释、ADR-0297 与 CONTEXT 里的相应句子；`git diff origin/main...HEAD` 在非票面文件的新增行里按上列词查无余项。
- ADR-0297 Status 与「对已接受记录的作用」「Links」统一写「四口」（作业事实面后补两口：`/transport-fulfillment/offsite-pickup-attempt-result-corrections`、`/transport-fulfillment/delivery-attempt-result-corrections`；运营决定面认定两口：`/transport-fulfillment-pickup-attempt-result-confirmations`、`/transport-fulfillment-delivery-attempt-result-confirmations`），与 ADR-0151 注记的「四口」一致（核实：两个更正处理器、两个接入面，端点四行）；「后四种与未决都是 200」改成「只有 `RESULT_CORRECTED` 答 201，其余五格都答 200」。入向指针仍只在 ADR-0151 的 Status 注记与 Links，ADR-0151 正文未动。

**验证（修订轮）**
- 变异（每条改后跑相关用例、跑完从 `scratchpad/c90b/` 的备份复原、`cmp` 核过；新用例都先对变异看红）：
  - N1：去掉晚到检查（`laterControlFactsForDeclaredSegment` 恒假）——生产装配复现用例红（答复没有 `LATER_CONTROL_FACTS`）、编排用例红；命中时不清空段声明——复现用例红（对象落进 SEG-A1 与 SEG-A2 两段）、编排用例红；装配不把段登记册交给更正处理器——复现用例红；拒绝格不限于「新登了揽收」（读回在册那份也带）——「读回在册那份不带拒绝格」用例红。
  - N2：`correctionContent` 去掉 `correctedAt`、去掉 `basis`——两侧重放用例红；派送领域去掉「更正不早于待确认那一版」、去掉「不早于它说的那件事」——领域用例各红，前者应用层用例也红；派送 `LoadForCorrection` 去掉 `FOR UPDATE OF r`——派送并发排队用例红（`-count=5` 逐次红）；运营决定面两口让 `?tenant=` 覆盖信封租户——传输面用例红、生产装配用例红（答 201，自报租户认定成了）；揽收与派送 `LoadForCorrection` 的 `a.tenant_id` 条件换恒真、揽收 `loadPickupCorrection` 与派送更正读取的 `tenant_id` 条件换恒真、`LoadDeliveryResult` 外连 `c.tenant_id = r.tenant_id` 去掉——两租户用例各红。
- `gofmt -l` 无输出；`go build ./... && go vet ./...` 干净。
- `TMPDIR=… scripts/test-full.sh ./internal/transportfulfillment/... ./internal/accessidentity/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/... ./migrations/... ./scripts/demo-seeds/...`（重放到 `ae21dab5c` 之后，前台跑到出结果）：退出码 0，无失败。
- 新用例 `-count=3`（`-v` 核过 39 次 PASS、无 FAIL 无 SKIP）；并发排队用例（揽收、派送各一）`-count=5` 共 10 次 PASS。
- `go run ./tools/at-coverage-counts -check` 退 1（只是总览计数过期，未退 2：`AT-TF-363` 在分桶表有行）；`go run ./tools/adr-index -check` 退 1（只是索引生成物过期）。
- 重放中 `internal/accessidentity/operation_decision.go` 与 `.scratch/at-coverage-inventory.md` 文末追记的冲突按两边并存处理（main 那段在前）。
