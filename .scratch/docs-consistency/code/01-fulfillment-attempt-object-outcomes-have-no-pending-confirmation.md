# 01 TF 履约尝试的逐对象结果没有「待确认」一格

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code01（基于 main `061156bcc`）。此前 ready-for-agent——R2（上线后尽快）：场外揽收与派送尝试已在生产命令面上，「只有车辆到场和司机扫描」「签收与地址来源冲突」这类来源第一周就会遇到；后果是执行方只能在四个已定结果里硬选一个，或者不登这次尝试，结果被说成确定，或这次尝试不留痕。可靠绕法：来源不足时先不登尝试，补齐证据后再登。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/tf.md](../audit/tf.md) M16（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

TF CONTEXT 要求履约尝试逐对象保存「成功、失败、拒收、待确认或其他适用结果」，UC-TF-002 与 UC-TF-006 都有「待确认」的结果行与 AT。代码里两套逐对象结果都是封闭四格，没有待确认。揽收一侧也没有独立的「执行方拒收」，拒收混在失败一族里。

## 证据

- 文档：
  - TF CONTEXT Rules「揽收、派送与交付」：「每个对象必须分别保存成功、失败、拒收、待确认或其他适用结果，任务汇总只能由对象结果派生」。
  - UC-TF-002 结果契约「揽收结果待确认」：「已有来源但不足以证明接收、控制或对象范围」；`AT-TF-018`：「只有车辆到场和司机扫描 | 保存来源，结果待确认，不推导实际接收」；`AT-TF-017`：「包装不合格且执行方明确拒收 | 保存对象级拒收和证据，控制不转移」。
  - UC-TF-006 `AT-TF-070`：「本人签收和地址来源冲突 | 形成冲突或待确认，不按最后消息覆盖」。
- 代码：
  - `internal/transportfulfillment/domain` 的 `AttemptObjectOutcome`：`ObjectPickedUp`、`CustomerAbsent`、`GoodsNotReady`、`PackagingUnacceptable` 四格。
  - 同包的 `DeliveryObjectOutcome`：`ObjectDelivered`、`DeliveryRefused`、`NoOneToReceive`、`WrongAddress` 四格。
  - 命令面 `adapters/http/perform_offsite_pickup.go` 的 `attemptObjectOutcomeFromWord` 只认这四个词，生产装配在 `cmd/parcel-api/assemble_control_facts.go`。

## 三类

机制：结果集合加一格「待确认」（带已知事实与缺口），汇总派生把它算作未结束。产品策略：无。租户取值：无（哪些来源够得上确定结果仍由既有规则判，不在本票设默认）。

## 规模

S–M：两个领域枚举、汇总派生、命令面词表、库表 CHECK（若有）与读回，各加一格；补 `AT-TF-018`、`AT-TF-070` 的用例。

## 最小的诚实修法

1. `AttemptObjectOutcome` 与 `DeliveryObjectOutcome` 各加「待确认」：不转移控制、不形成有效交付或揽收到手，任务汇总把它读作未结束。
2. 待确认那一格必须带来源引用；以后同一对象的确定结果作为新尝试或更正形成，不改写这一格。
3. 揽收要不要另立「执行方拒收」，见下「要裁的」；不裁之前不加。

## 要裁的

- 揽收「执行方拒收」是独立一格，还是并在 `PackagingUnacceptable` 一族里（UC-TF-002 结果契约把「本次对象被拒收」列为单独一行）。归 TF owner；本票按「待确认」先做，拒收一格随裁定。
- 审计附注：owner 若认定「待确认」不落在尝试上、而是不登尝试，这条改归 B，改 CONTEXT 与两份 UC，本票作废。

## 完成记录（2026-10-25，分支 claude-code01）

用户授权原话「你作为业务和系统专家自决」；下列「代决」由作者会话代 TF owner 作出，待 TF owner 复核。从 main `9ea060682` 起做，收尾前重放到 main `061156bcc`（AT 盘点文末追记与 code/19 那段冲突，两段都留、main 那段在前）。

### 做了什么（照「最小的诚实修法」逐条）

1. 两套逐对象结果各加「待确认」：揽收 `domain.PickupPendingConfirmation`、派送 `domain.DeliveryPendingConfirmation`，词都是 `PENDING_CONFIRMATION`。待确认只经 `domain.FormPendingAttemptObjectResult` / `domain.FormPendingDeliveryAttemptResult` 形成，既有 `FormAttemptObjectResult` / `FormDeliveryAttemptResult` 拒收待确认（签名不变）。`Failed()` 不含待确认，新增 `PendingConfirmation()`；`Succeeded()` 照旧只认揽收到手与妥投。于是待确认：不形成场外揽收、不签揽收版本、不进段、不交 parcel-shipment 意图（`PerformOffsitePickupHandler` 照旧只为揽收到手走那条路）；交付登记答 `NOT_EFFECTIVE`、不结束参与（`FormEffectiveDelivery` 只收妥投）；失败尝试费答`未受理`带新细格 `ATTEMPT_RESULT_PENDING_CONFIRMATION`（`domain.ErrAttemptResultPendingConfirmation`）。
2. 待确认必带来源引用（新值对象 `domain.AttemptResultSourceReference`）与缺口或冲突（沿用依据一格）；同一来源身份（揽收）或尝试身份（派送）换说法再交照旧答 `SOURCE_CONFLICT`，待确认那一格不改写；新的上门照常登新尝试。同一次上门以更正确定那一条没有口，立票 [code/90](./90-a-pending-attempt-object-result-has-no-correction-entry.md)。
3. 揽收另立「执行方拒收」`domain.ExecutorRefused`（`EXECUTOR_REFUSED`），见代决 1。
- 编排：`formPickupObjectResult`、`formDeliveryObjectResult` 按结果分走两条构造器；确定结果带来源、待确认或拒收带控制依据都答`来源未受理`。
- 传输面：`/transport-fulfillment/offsite-pickup-attempts` 与 `/transport-fulfillment/delivery-attempts` 逐对象加可缺的 `source`，词表 `attemptObjectOutcomeFromWord`、`deliveryObjectOutcomeFromWord` 各加新词（仍取常量自己的 `String()`），答复逐对象透出 `source`。没给 `source` 的旧载荷与旧答复形状不变。
- 摘要：`CanonicalizeOffsitePickupPayload` 对象行加 `source` 键、缺席不写，没有来源的行逐字节同形，TFC-1 不换版（ADR-0014 不改写已存摘要）；派送尝试的内容比对锚 `deliveryAttemptContent` 加来源一格（两侧现算，无已存摘要）。`unversionedPickupContentDigest` 函数体不动，头注写明来源为什么不在旧形里。
- 库：迁移 `transport_fulfillment/0050_attempt_object_result_pending_confirmation_and_executor_refusal.sql`——两张结果表加 `source_ref`、放开封闭集、加「来源只属待确认」CHECK，揽收 shape CHECK 改写为「揽收到手不带依据，其余必带」，派送的依据 CHECK 本就是这一形、不动。读回 `rebuildPickupResult`、`rebuildDeliveryResult` 按结果走两条构造器，确定结果读回来源即报错（库面不一致暴露，不悄悄带出去）。存量行共存，直接校验、不 `NOT VALID`。
- 文档：TF CONTEXT「揽收、派送与交付」补两条规则句（待确认的内容、去向与不改写；揽收拒收与包装不合格的分界）；UC-TF-002、UC-TF-006 状态行与「给开发的交接」改为只剩同一次上门以更正确定那一条，指 code/90。
- 没动：`cmd/parcel-api` 的 `endpoints.go`、`main.go`、`endpoints_test.go`、`routes.go`、各 `assemble_*.go`，`cmd/parcel-dispatch` 全部（ADR-0222）；admin-web（不读尝试结果）。无新写者（两只写口 `PickupAttempts.Save`、`DeliveryAttempts.Save` 是既有的，事务外拒跑的负面证据照旧在 `TestPickupWritesRefuseToRunOutsideATransaction`、`TestDeliveryAttemptWritesRefuseToRunOutsideATransaction`），无新锁，无新路由，无已接受 ADR 正文或 Status 改动。

### 新格在各消费方的落点（ADR-0025 全函数）

读码 grep 两个枚举、各常量与 `Succeeded`/`Failed`/`Outcome()` 的全部非测试引用（`cmd`、`internal`、`tools`；VE、SA、PS、NO、NR、PP、CC 的跨上下文适配器与 `cmd/parcel-dispatch` 都不读尝试结果——派发只接 `offsite-pickup.registered` 与 `effective-delivery.registered`，尝试级 `offsite-pickup.formed` 只带租户与来源身份；admin-web 不展示尝试结果）：

| 消费方 | 待确认 | 执行方拒收 |
|---|---|---|
| `domain` 两条构造器、`valid`、`String` | 只从待确认构造器进，必带来源与缺口 | 同失败：必带依据 |
| `domain.FormEffectiveDelivery` | `ErrNotAnEffectiveDelivery` | 不适用（派送侧无此格） |
| `domain.FailedPickupAttemptBasis` / `FailedDeliveryAttemptBasis` | `ErrAttemptResultPendingConfirmation` | 照常形成依据 |
| `application.PerformOffsitePickupHandler` | 记结果，不形成揽收、不进段、不交意图；带控制依据未受理 | 同 |
| `application.RecordDeliveryAttemptHandler` | 记结果；内容比对含来源 | — |
| `application.RegisterEffectiveDeliveryHandler`、`DeliveryGateReader.ReadRegistration` | `NOT_EFFECTIVE`；读数零值、不问关务 | — |
| `application` 失败尝试费 | `ATTEMPT_RESULT_PENDING_CONFIRMATION` 未受理 | 照常登记 |
| `adapters/http` 两口词表与答复 | 收词、收来源、答来源 | 收词 |
| `adapters/postgres` 两只库的写、读与 `attemptOutcomeFrom` / `deliveryOutcomeFrom` | 写读 `source_ref` | 写读 |

### 代决

1. **揽收「执行方拒收」立独立一格，同一笔做。** 按文档字面：TF CONTEXT 把成功、失败、拒收、待确认列为四种结果；UC-TF-002 结果契约「本次尝试失败」与「本次对象被拒收」分两行，要保存的东西不同（失败要失败原因，拒收要原因、责任方与此前控制方）；`AT-TF-016`（客户不在或货物未备好 → 失败）与 `AT-TF-017`（包装不合格且执行方明确拒收 → 对象级拒收）分写。并进 `PackagingUnacceptable` 不成立的另一条理由：禁运、超限等别的原因的明确拒收今天没有格可落。分界是**执行方有没有作出明确拒收**：明确拒收不论原因记 `EXECUTOR_REFUSED`，原因与证据进依据；包装不合格而执行方没有拒收（如执行方指出包装不合格、客户决定这次不交运）记 `PACKAGING_UNACCEPTABLE` 失败——同一场景只落一格，写进 CONTEXT 规则句与领域常量注释。拒收与失败同属 `Failed()`（本次尝试的确定未成功）：可作失败尝试费事实依据，与派送一侧收件人拒收 `REFUSED` 同族（ADR-0207 决定一只把揽收到手与妥投排除在失败之外）。
2. **以后的确定结果：新的上门形成新尝试，同一次上门的后补来源以更正形成新判断版本；待确认那一版不改写。** 照 CONTEXT「每次实际上门形成新的履约尝试」与「不按最后消息覆盖」、UC-TF-002「同一对象在同一尝试中只能有一个当前有效结果；更正或来源撤销形成新判断版本」「外部伙伴状态迟到时按业务发生时间采用」。不选「一律新尝试」：伙伴迟到的签收、复核后认定的地址不是一次新的到场，登成新尝试就是虚构一次上门。不选「改写这一格」：那正是按最后消息覆盖。今天走得通的是新尝试；同一次上门的更正口没有，立 code/90（为什么不在本票做见越权风险点 4）。写成 CONTEXT 规则句，差距写在两份 UC 的交接表。
3. **待确认带两样：已有来源（`source_ref`）与缺口或冲突（依据）。** UC-TF-002 要「已知事实、冲突、当前控制、缺口和查询入口」，UC-TF-006 要「全部来源、当前控制和复核入口」。来源与缺口分两格：一格说已经知道什么，一格说还差什么或哪里冲突，合成一格复核的人分不出证据与待补。「当前控制」不复制进结果格——控制关系由控制事实形成，结果结构上没有控制字段，「保持此前已确认的控制关系」由形状保证；复制一份会在控制事实追加版本时静默过期（与有效交付不复制实际承运商同一理由，ADR-0103 决定二、八）。「查询/复核入口」= 尝试身份（揽收按来源身份、派送按尝试身份）+ 对象 + 来源引用；运营查阅清单没有，记进 code/90。
4. **任务汇总读作未结束，落在结果分类上，不另立汇总类型。** 代码里没有揽派任务的汇总派生：任务完成与终止是显式带依据的动作（`DispatchTask.Complete`、`Terminate`，生产无调用方），`AT-TF-015`、`AT-TF-064` 今天也是逐对象钉的。汇总能读的判据是 `Succeeded()`（完成）、`Failed()`（本次确定未成功）、`PendingConfirmation()`（未结束），待确认不落进前两样；读成已结束（`Failed()` 收进待确认）由变异 M1、M6 钉住。立一个没有消费方的汇总类型是另一份口径。
5. **失败尝试费对待确认答`未受理`另带细格 `ATTEMPT_RESULT_PENDING_CONFIRMATION`，不并进 `ATTEMPT_DID_NOT_FAIL`。** 后者断言「这次没有失败」，对待确认就是替缺口默认了成功；续办也不同——那一格放弃，这一格等确定之后再登（ADR-0029 按恢复动作分格）。仍是`未受理`（ADR-0207 决定一「不是失败，答未受理」照用），不进任何未决格（ADR-0029：确定性拒绝不是读不动）。
6. **交付登记对待确认答 `NOT_EFFECTIVE`，不另立一格。** CONTEXT「证据不足……不构成有效交付」；今天没有更正口，待确认那件对象的续办与失败相同（改约再派）。更正口落地后两种续办才分开，要不要分格交 code/90 判（写进那张票的「要裁的」）。
7. **构造器分两条，不改既有签名。** 待确认的构造器签名就要来源与缺口，没有来源的待确认构造不出来；既有构造器拒待确认。既有签名的调用点（含 PS、VE 的测试夹具）一处不动。
8. **线格式与摘要只加不改。** `source` 可缺、答复里 omitempty；揽收摘要缺席不写键。旧客户端、旧已存摘要、旧库行都照旧。
9. **枚举新值只往末尾加**：取值整数进了无版本旧摘要（`unversionedPickupContentDigest`），挪位会让旧记录的重放认不出自己。
10. **迁移号 0050**：开工时 `ls migrations/transport_fulfillment/` 最大 0049，在途分支（`git branch -a`，去 `merged/`、`salvage/`）没有占 005x 的 TF 迁移；重放到 `061156bcc` 后仍是 0049 之后的下一个。

三类：机制本票做完（两格结果、来源与缺口、不转移控制、分类判据、库面与线格式）；产品策略无；租户取值无（哪些来源够得上确定结果仍由既有证据规则判，不设默认）。证据层级 `S`。

已接受 ADR：未改任何口径。ADR-0263（交接待确认保持此前控制）与本票同向、管的是交接不是尝试结果；ADR-0264、ADR-0265 与本票无关。ADR-0207 决定一照用（见代决 5），ADR-0029、ADR-0022（新落一次尝试 201、其余 200）、ADR-0014（已存摘要不改写）、ADR-0222（未动装配文件）照用。

### 越权风险点

1. **拒收与包装不合格的分界靠执行方报对**（TF owner）。两格都是「执行方没拿走」，分界是有没有作出拒收决定；一线设备若一律报 `PACKAGING_UNACCEPTABLE`，明确拒收会被读成失败，反之亦然。两格的计费面相同（都能作失败尝试费依据），差别在审计与复核语义。设备端怎么让执行方选归一线客户端。
2. **UC 结果契约「必须保存 当前控制 / 此前控制方」按「保持不变、可从控制事实读回」解，不复制进结果格**（TF owner，见代决 3）。若 owner 要结果格自带一份控制快照，要改两张表与读回，且要回答快照与控制事实不一致时信谁。
3. **「查询入口 / 复核入口」按「尝试身份 + 对象 + 来源引用」解，没有运营查阅清单**（TF owner）。今天要找出还在待确认的对象，只能按尝试身份逐件读；查阅清单归 code/90。长期待确认也没有信号（与 ADR-0263 越权风险点 3 同形），本票不立信号规则。
4. **同一次上门的待确认今天确定不了，更正口另票 code/90**（TF owner）。为什么不在本票做：要给两张结果表加版本链、定更正挂作业事实还是运营决定、让有效交付与失败尝试费与交付读数改读当前有效版本、并裁揽收一侧更正成揽收到手与对象级登记口那条链的关系——是另一张 M–L 的票，票面「规模」S–M 也没有把它算进来。期间：派送那件对象要形成有效交付只能等新的上门；揽收一侧对象级登记口（`/transport-fulfillment/offsite-pickups`，按（对象，尝试）幂等）登得进同一次尝试的揽收到手、不改写待确认那一格，但不回指它，两条链对同（对象，尝试）各说一样。
5. **失败尝试费多一个未受理细格**（TF owner；SA 不消费这一格）。登记方见到 `ATTEMPT_RESULT_PENDING_CONFIRMATION` 要等确定之后再登，而今天同一次上门确定不了（风险点 4）——实际续办是等新尝试，新尝试是另一个依据。
6. **揽收拒收计入失败尝试费事实依据**（TF owner，结算会签为宜）。协议若只对客户原因的失败计费，拒收原因归谁由结算按依据判（ADR-0098 的边界）；本票不在 TF 侧替它分。

### AT

- `AT-TF-018`、`AT-TF-070`：④ → ①；`AT-TF-017`：② → ①。三行各点领域、编排、真库、传输面与生产装配真库用例（`*/attempt_pending_confirmation_test.go`，`cmd/parcel-api/attempt_pending_confirmation_test.go`）。总览计数未改，由推送方重算。

### 验证

- 先红后绿：领域、编排、真库、传输面四层的新用例都先写、先跑红（领域与编排是编译不过——新符号不在；真库是 0050 已加而适配器未写 `source_ref`，三条红在 CHECK 与「unknown attempt object outcome」上；传输面是 `unknown field "source"` 与 400），再实现转绿。生产装配那两条是传输面实现之后写的，红由变异 M2（派送那条）与 M3（揽收那条）补证。
- 变异（每条改后跑、跑完从 scratchpad 备份复原、`cmp` 核过）：M1 揽收 `Failed()` 收进待确认（待确认被汇总读成已结束）——领域红；M2 `FormEffectiveDelivery` 收下待确认（待确认转移了控制）——领域、编排（参与被结束）、生产装配真库（不再答 `NOT_EFFECTIVE`）三层红；M3 编排把待确认送上揽收到手那条路（取得控制、进段）——编排三条、传输面一条红，重放之后另对生产装配真库那条跑，红（答未受理而不是 201）；M4 迁移去掉揽收表「来源只属待确认」——真库 CHECK 用例红；M5 派送内容比对不看来源——编排红；M6 失败尝试费把揽收待确认答成「没有失败」——领域、编排红；M7 揽收摘要漏掉来源——金样红。七处全被杀。
- 存量共存实测：用 main `9ea060682` 的迁移助手把一次性库 `c01coexist` 迁到 0049，按旧写入方形状插揽收（四种旧结果，含一条揽收行）与派送（四种旧结果）各一次尝试，再用本分支的助手施加 0050：施加成功，新 CHECK 全部 `convalidated`，旧行 `source_ref` 为空、原样在；用后 `DROP DATABASE … WITH (FORCE)`。
- `go build ./...`、`go vet ./...` 干净，`gofmt -l internal cmd` 无输出。
- `TMPDIR=… scripts/test-full.sh <改动包、go list -test 算出的反向依赖、./cmd/parcel-api/、./cmd/parcel-dispatch/、./internal/architecture/...、./migrations/...>`：重放前（基 `9ea060682`）与重放到 `061156bcc` 之后各跑一次，都是 65 个包 ok、2 个无测试文件，退出码 0。
- 新用例与扩出金样的 `TestCommandPayloadsRoundTripAsTFC1` 共二十个顶层测试 `-count=3`（真库，`-v` 核过无 SKIP、无 FAIL），重放前后各跑一次：都是 60 次 PASS，退出码 0。
- 种子：一次性库 `c01seed` 上 `scripts/demo-seeds/seed.sh` 退出码 0（日志里施加了 0050），用后删库。
