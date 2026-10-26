# 01 TF 履约尝试的逐对象结果没有「待确认」一格

Category: bug
Status: resolved——2026-10-26 进 main（见文末「进 main 记录」）。此前 in-progress——收尾轮完工待第三轮评审与进 main，分支 claude-code01（基于 main `b685046d3`）。此前 ready-for-agent——R2（上线后尽快）：场外揽收与派送尝试已在生产命令面上，「只有车辆到场和司机扫描」「签收与地址来源冲突」这类来源第一周就会遇到；后果是执行方只能在四个已定结果里硬选一个，或者不登这次尝试，结果被说成确定，或这次尝试不留痕。可靠绕法：来源不足时先不登尝试，补齐证据后再登。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
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

1. 两套逐对象结果各加「待确认」：揽收 `domain.PickupPendingConfirmation`、派送 `domain.DeliveryPendingConfirmation`，词都是 `PENDING_CONFIRMATION`。待确认只经 `domain.FormPendingAttemptObjectResult` / `domain.FormPendingDeliveryAttemptResult` 形成，既有 `FormAttemptObjectResult` / `FormDeliveryAttemptResult` 拒收待确认（签名不变）。`Failed()` 不含待确认，新增 `PendingConfirmation()`；`Succeeded()` 照旧只认揽收到手与妥投。于是待确认：不形成场外揽收、不签揽收版本、不进段、不交 parcel-shipment 意图（`PerformOffsitePickupHandler` 照旧只为揽收到手走那条路）；交付登记单答 `DELIVERY_RESULT_PENDING_CONFIRMATION`、不结束参与（`FormEffectiveDelivery` 只收妥投，对待确认交回 `domain.ErrDeliveryResultPendingConfirmation`，修订轮 N2）；失败尝试费答`未受理`带新细格 `ATTEMPT_RESULT_PENDING_CONFIRMATION`（`domain.ErrAttemptResultPendingConfirmation`）。
2. 待确认必带来源引用（新值对象 `domain.AttemptResultSourceReference`）与缺口或冲突（沿用依据一格）；同一来源身份（揽收）或尝试身份（派送）换说法再交照旧答 `SOURCE_CONFLICT`，待确认那一格不改写；新的上门照常登新尝试。同一次上门以更正确定那一条没有口，立票 [code/90](./90-a-pending-attempt-object-result-has-no-correction-entry.md)。
3. 揽收另立「执行方拒收」`domain.ExecutorRefused`（`EXECUTOR_REFUSED`），见代决 1。
- 编排：`formPickupObjectResult`、`formDeliveryObjectResult` 按结果分走两条构造器；确定结果带来源、待确认或拒收带控制依据都答`来源未受理`。
- 传输面：`/transport-fulfillment/offsite-pickup-attempts` 与 `/transport-fulfillment/delivery-attempts` 逐对象加可缺的 `source`，词表 `attemptObjectOutcomeFromWord`、`deliveryObjectOutcomeFromWord` 各加新词（仍取常量自己的 `String()`），答复逐对象透出 `source`。没给 `source` 的旧载荷与旧答复形状不变。
- 摘要（修订轮改写，评审 B1）：`CanonicalizeOffsitePickupPayload` 按内容取最低一版——至少一件对象带来源时写新号 `TFC-3`（常量 `pickupAttemptSourceCanonicalizationVersion`），每个对象行都写 `source`、没有的写 `null`；一件都不带时仍写 `TFC-1`、没有 `source` 键，与来源进文法之前逐字节同形。揽收执行口的比对改走新的 `domain.CompareStoredPickupAttemptDigest`：在 `CompareStoredDigest` 之上认 `TFC-3`；确定结果带来源的提交在编排入口、内容比对之前就答`来源未受理`（第二轮评审 B2）。派送尝试的内容比对锚 `deliveryAttemptContent` 加来源一格（两侧现算，无已存摘要）。`unversionedPickupContentDigest` 函数体不动，头注写明来源为什么不在旧形里。
- 库：迁移 `transport_fulfillment/0050_attempt_object_result_pending_confirmation_and_executor_refusal.sql`——两张结果表加 `source_ref`、放开封闭集、加「来源只属待确认」CHECK，揽收 shape CHECK 改写为「揽收到手不带依据，其余必带」，派送的依据 CHECK 本就是这一形、不动。读回 `rebuildPickupResult`、`rebuildDeliveryResult` 按结果走两条构造器，确定结果读回来源即报错（库面不一致暴露，不悄悄带出去）。存量行共存，直接校验、不 `NOT VALID`。
- 文档：TF CONTEXT「揽收、派送与交付」补两条规则句（待确认的内容、去向与不改写；揽收拒收与包装不合格的分界），「运输收费发生项与成本来源」里失败尝试的尝试费补「（含拒收）」；UC-TF-002、UC-TF-006 状态行与「给开发的交接」改为同一次上门以更正确定那一条指 code/90，UC-TF-002 另加一行「拒收的责任归属不另存」待 TF owner 裁（修订轮 N3），UC-TF-006 结果契约「尝试待确认」写明交付登记单答的那一格（修订轮 N2）。
- 没动：`cmd/parcel-api` 的 `endpoints.go`、`main.go`、`endpoints_test.go`、`routes.go`、各 `assemble_*.go`，`cmd/parcel-dispatch` 全部（ADR-0222）；admin-web（不读尝试结果）。无新写者（两只写口 `PickupAttempts.Save`、`DeliveryAttempts.Save` 是既有的，事务外拒跑的负面证据照旧在 `TestPickupWritesRefuseToRunOutsideATransaction`、`TestDeliveryAttemptWritesRefuseToRunOutsideATransaction`），无新锁，无新路由，无已接受 ADR 正文或 Status 改动。

### 新格在各消费方的落点（ADR-0025 全函数）

读码 grep 两个枚举、各常量与 `Succeeded`/`Failed`/`Outcome()` 的全部非测试引用（`cmd`、`internal`、`tools`；VE、SA、PS、NO、NR、PP、CC 的跨上下文适配器与 `cmd/parcel-dispatch` 都不读尝试结果——派发只接 `offsite-pickup.registered` 与 `effective-delivery.registered`，尝试级 `offsite-pickup.formed` 只带租户与来源身份；admin-web 不展示尝试结果）：

| 消费方 | 待确认 | 执行方拒收 |
|---|---|---|
| `domain` 两条构造器、`valid`、`String` | 只从待确认构造器进，必带来源与缺口 | 同失败：必带依据 |
| `domain.FormEffectiveDelivery` | `ErrDeliveryResultPendingConfirmation`（包着 `ErrNotAnEffectiveDelivery`） | 不适用（派送侧无此格） |
| `domain.FailedPickupAttemptBasis` / `FailedDeliveryAttemptBasis` | `ErrAttemptResultPendingConfirmation` | 照常形成依据 |
| `application.PerformOffsitePickupHandler` | 记结果，不形成揽收、不进段、不交意图；带控制依据未受理 | 同 |
| `application.RecordDeliveryAttemptHandler` | 记结果；内容比对含来源 | — |
| `application.RegisterEffectiveDeliveryHandler`、`DeliveryGateReader.ReadRegistration` | `DELIVERY_RESULT_PENDING_CONFIRMATION`（200）；读数零值、不问关务 | — |
| `domain.CanonicalizeOffsitePickupPayload`、`CompareStoredPickupAttemptDigest` | 带来源写 `TFC-3`，比对两号都认 | 词是取值不是键，照写 `TFC-1` |
| `application` 失败尝试费 | `ATTEMPT_RESULT_PENDING_CONFIRMATION` 未受理 | 照常登记 |
| `adapters/http` 两口词表与答复 | 收词、收来源、答来源 | 收词 |
| `adapters/postgres` 两只库的写、读与 `attemptOutcomeFrom` / `deliveryOutcomeFrom` | 写读 `source_ref` | 写读 |

### 代决

1. **揽收「执行方拒收」立独立一格，同一笔做。** 按文档字面：TF CONTEXT 把成功、失败、拒收、待确认列为四种结果；UC-TF-002 结果契约「本次尝试失败」与「本次对象被拒收」分两行，要保存的东西不同（失败要失败原因，拒收要原因、责任方与此前控制方）；`AT-TF-016`（客户不在或货物未备好 → 失败）与 `AT-TF-017`（包装不合格且执行方明确拒收 → 对象级拒收）分写。并进 `PackagingUnacceptable` 不成立的另一条理由：禁运、超限等别的原因的明确拒收今天没有格可落。拒收一格只带原因与证据，**拒收的责任归属不由它推导、也不另存**（修订轮 N3：执行方作出拒收不等于责任归执行方，UC-TF-002 交接表留一行待 TF owner 裁）。分界是**执行方有没有作出明确拒收**：明确拒收不论原因记 `EXECUTOR_REFUSED`，原因与证据进依据；包装不合格而执行方没有拒收（如执行方指出包装不合格、客户决定这次不交运）记 `PACKAGING_UNACCEPTABLE` 失败——同一场景只落一格，写进 CONTEXT 规则句与领域常量注释。拒收与失败同属 `Failed()`（本次尝试的确定未成功）：可作失败尝试费事实依据，与派送一侧收件人拒收 `REFUSED` 同族（ADR-0207 决定一只把揽收到手与妥投排除在失败之外）。
2. **以后的确定结果：新的上门形成新尝试，同一次上门的后补来源以更正形成新判断版本；待确认那一版不改写。** 照 CONTEXT「每次实际上门形成新的履约尝试」与「不按最后消息覆盖」、UC-TF-002「同一对象在同一尝试中只能有一个当前有效结果；更正或来源撤销形成新判断版本」「外部伙伴状态迟到时按业务发生时间采用」。不选「一律新尝试」：伙伴迟到的签收、复核后认定的地址不是一次新的到场，登成新尝试就是虚构一次上门。不选「改写这一格」：那正是按最后消息覆盖。今天走得通的是新尝试；同一次上门的更正口没有，立 code/90（为什么不在本票做见越权风险点 4）。写成 CONTEXT 规则句，差距写在两份 UC 的交接表。
3. **待确认带两样：已有来源（`source_ref`）与缺口或冲突（依据）。** UC-TF-002 要「已知事实、冲突、当前控制、缺口和查询入口」，UC-TF-006 要「全部来源、当前控制和复核入口」。来源与缺口分两格：一格说已经知道什么，一格说还差什么或哪里冲突，合成一格复核的人分不出证据与待补。「当前控制」不复制进结果格——控制关系由控制事实形成，结果结构上没有控制字段，「保持此前已确认的控制关系」由形状保证；复制一份会在控制事实追加版本时静默过期（与有效交付不复制实际承运商同一理由，ADR-0103 决定二、八）。「查询/复核入口」= 尝试身份（揽收按来源身份、派送按尝试身份）+ 对象 + 来源引用；运营查阅清单没有，记进 code/90。
4. **任务汇总读作未结束，落在结果分类上，不另立汇总类型。** 代码里没有揽派任务的汇总派生：任务完成与终止是显式带依据的动作（`DispatchTask.Complete`、`Terminate`，生产无调用方），`AT-TF-015`、`AT-TF-064` 今天也是逐对象钉的。汇总能读的判据是 `Succeeded()`（完成）、`Failed()`（本次确定未成功）、`PendingConfirmation()`（未结束），待确认不落进前两样；读成已结束（`Failed()` 收进待确认）由变异 M1、M6 钉住。立一个没有消费方的汇总类型是另一份口径。
5. **失败尝试费对待确认答`未受理`另带细格 `ATTEMPT_RESULT_PENDING_CONFIRMATION`，不并进 `ATTEMPT_DID_NOT_FAIL`。** 后者断言「这次没有失败」，对待确认就是替缺口默认了成功；续办也不同——那一格放弃，这一格等确定之后再登（ADR-0029 按恢复动作分格）。仍是`未受理`（ADR-0207 决定一「不是失败，答未受理」照用），不进任何未决格（ADR-0029：确定性拒绝不是读不动）。
6. **（修订轮改写，评审 N2，协调方裁）交付登记对待确认单立一格 `DELIVERY_RESULT_PENDING_CONFIRMATION`（200），不并进 `NOT_EFFECTIVE`。** 两格都登不出有效交付（CONTEXT「证据不足……不构成有效交付」），但续办不同：`未生效`说这次没交成、改约再派；待确认说交没交成还不知道——签收与地址冲突时那件对象可能已在收件人手里，改约再派可能是错的动作，续办是先把那一格确定下来（ADR-0029 恢复动作不同必须分）。与失败尝试费单立细格同一把尺。现在就分，是因为 code/90 之后再拆就是改已发布答复的取值。领域 `ErrDeliveryResultPendingConfirmation` 包着 `ErrNotAnEffectiveDelivery`：「不是有效交付」照旧成立，编排先认待确认那一种。初稿并进 `NOT_EFFECTIVE`，理由是今天两者续办相同，评审指出那一读漏了对象可能已交到的情形。
7. **构造器分两条，不改既有签名。** 待确认的构造器签名就要来源与缺口，没有来源的待确认构造不出来；既有构造器拒待确认。既有签名的调用点（含 PS、VE 的测试夹具）一处不动。
8. **（修订轮改写，评审 B1）线格式只加可缺键；揽收摘要带来源换号 `TFC-3`，不带来源的照旧 `TFC-1`。** 线格式：`source` 可缺、答复里 omitempty，旧客户端照旧。摘要：揽收执行口存摘要（`pickup_attempt.content_digest`），在已接 `TFC-1` 的这一面加键（可缺键也算）是形状变化，照换号（ADR-0267 决定一）；按内容取最低一版（ADR-0209 决定三）——一件都不带来源的文档仍写 `TFC-1`、逐字节不变，已存摘要照旧对得上；带来源的写 `TFC-3`，不认它的构建对新文档答「认不出的形状」而不是丢掉来源算错摘要。号取 TF 前缀下一个：`TFC-2` 已是交接节点那一形（`handoverNodeCanonicalizationVersion`），各面的比对只认自己那几形，揽收执行口不认 `TFC-2`。比对：已存 `TFC-3` 与本次逐串比；本次带来源而已存是 `TFC-1` 或无版本那一形直接答冲突——这与 ADR-0014「按已存摘要的形状重算本次命令再比」同答：带来源的命令到得了比对口就必含待确认（确定结果带来源在编排入口、比对之前已答未受理，第二轮评审 B2），按旧形重算时待确认那一行的结果取值也在里面，而旧形写下的记录里不可能有待确认，重算出的串不可能与已存的相同。TFC-3 的出处写在本条与常量 `pickupAttemptSourceCanonicalizationVersion` 的头注。初稿同号加键、只在代码头注写理由，违反 ADR-0267 决定三（同号改形要另立记录写明前提）。
9. **（修订轮改准，评审 N5）揽收结果枚举的新值只往末尾加**：取值整数进了揽收的无版本旧摘要（`unversionedPickupContentDigest`），挪位会让旧记录的重放认不出自己。派送结果没有这一层——库与内容比对都按词，整数不出进程，新值放在末尾只为与揽收那一侧同形。
10. **迁移号 0050**：开工时 `ls migrations/transport_fulfillment/` 最大 0049，在途分支（`git branch -a`，去 `merged/`、`salvage/`）没有占 005x 的 TF 迁移；重放到 `061156bcc` 后仍是 0049 之后的下一个。

三类：机制本票做完（两格结果、来源与缺口、不转移控制、分类判据、库面与线格式）；产品策略无；租户取值无（哪些来源够得上确定结果仍由既有证据规则判，不设默认）。证据层级 `S`。

已接受 ADR：未改任何口径。ADR-0263（交接待确认保持此前控制）与本票同向、管的是交接不是尝试结果；ADR-0264、ADR-0265 与本票无关。揽收摘要照 ADR-0267 决定一换号、照 ADR-0209 决定三按内容取最低一版（修订轮，评审 B1），没有同号改形，不需要另立记录。ADR-0014 照用、不构成部分停用（第二轮评审 B2 改写本句）：已存摘要不改写；比对口对「本次带来源、已存是旧形」答冲突，与 ADR-0014 决定的「按已存摘要的形状重算本次命令再比」同答，理由见代决 8——带来源必含待确认、旧形记录里不可能有待确认；也不借用 ADR-0213 只为交接首登一口登记的那一次部分停用。初稿（修订轮）的比对口理由写成「同交接」，第二轮评审指出那是借用另一口的停用，且确定结果带来源的畸形命令能走到比对口、被答成冲突——ADR-0014 说版本不同不是冲突；入口前移之后那条路不再存在。ADR-0207 决定一照用（见代决 5），ADR-0029（代决 5、6）、ADR-0022（新落一次尝试 201、其余 200；交付登记新格 200）、ADR-0222（未动装配文件）照用。

### 越权风险点

1. **拒收与包装不合格的分界靠执行方报对**（TF owner）。两格都是「执行方没拿走」，分界是有没有作出拒收决定；一线设备若一律报 `PACKAGING_UNACCEPTABLE`，明确拒收会被读成失败，反之亦然。两格的计费面相同（都能作失败尝试费依据），差别在审计与复核语义。设备端怎么让执行方选归一线客户端。存量若有执行方明确拒收却记成 `PACKAGING_UNACCEPTABLE` 的行，今后读作未拒收；上线前只有演示数据与夹具。
2. **UC 结果契约「必须保存 当前控制 / 此前控制方」按「保持不变、可从控制事实读回」解，不复制进结果格**（TF owner，见代决 3）。若 owner 要结果格自带一份控制快照，要改两张表与读回，且要回答快照与控制事实不一致时信谁。
3. **「查询入口 / 复核入口」按「尝试身份 + 对象 + 来源引用」解，没有运营查阅清单**（TF owner）。今天要找出还在待确认的对象，只能按尝试身份逐件读；查阅清单归 code/90。长期待确认也没有信号（与 ADR-0263 越权风险点 3 同形），本票不立信号规则。
4. **同一次上门的待确认今天确定不了，更正口另票 code/90**（TF owner）。为什么不在本票做：要给两张结果表加版本链、定更正挂作业事实还是运营决定、让有效交付与失败尝试费与交付读数改读当前有效版本、并裁揽收一侧更正成揽收到手与对象级登记口那条链的关系——是另一张 M–L 的票，票面「规模」S–M 也没有把它算进来。期间：派送那件对象要形成有效交付只能等新的上门；揽收一侧对象级登记口（`/transport-fulfillment/offsite-pickups`，按（对象，尝试）幂等）登得进同一次尝试的揽收到手、不改写待确认那一格，但不回指它，两条链对同（对象，尝试）各说一样。
5. **失败尝试费多一个未受理细格**（TF owner；SA 不消费这一格）。登记方见到 `ATTEMPT_RESULT_PENDING_CONFIRMATION` 要等确定之后再登，而今天同一次上门确定不了（风险点 4）——实际续办是等新尝试，新尝试是另一个依据。
6. **揽收拒收计入失败尝试费事实依据**（TF owner，结算会签为宜）。协议若只对客户原因的失败计费，拒收原因归谁由结算按依据判（ADR-0098 的边界）；本票不在 TF 侧替它分。
7. **（修订轮）交付登记答复多一个词 `DELIVERY_RESULT_PENDING_CONFIRMATION`**（TF owner；一线客户端）。不认识它的设备照 ADR-0022 读作已形成的答案、不自动重发，交人；首发前没有真实一线设备。
8. **（修订轮）拒收的责任归属不另存**（TF owner，评审 N3）。UC-TF-002 结果契约「本次对象被拒收」要保存责任方；今天只存执行方（随尝试）与原因证据，责任归谁没有一格。交接表留一行，去处是票 [code/91](./91-an-executor-refusal-records-no-responsibility-attribution.md)（第二轮评审 N-a：初稿写「待 TF owner 裁、不立票」，全仓独此一处不指票不指 ADR，进 main 之后无人追）。

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

## 评审记录与修订轮

第一轮隔离评审（协调方转述）：暂不可合入，一条阻断 B1，另 N1–N5 与协调方加的两处。修订轮先重放到 `origin/main` `5759e853a`（本地 `main` 引用那时落在 `061156bcc`，真 main 以 `origin/main` 为准；AT 盘点文末追记与 demo-chain-gaps/147 那段冲突，两段都留、main 那段在前），再逐条改，新提交、不改写旧提交；收尾前 main 又前移到 `fb8ee7096`（demo-chain-gaps/142、143 与两笔 admin-web），重放无冲突。

- **B1 揽收摘要同号加键（阻断，协调方裁走评审的 (a)）。** 照 ADR-0267 决定一换号、ADR-0209 决定三按内容取最低一版：`domain.CanonicalizeOffsitePickupPayload` 至少一件对象带来源时写 `TFC-3`（新常量 `pickupAttemptSourceCanonicalizationVersion`；`TFC-2` 已是交接节点那一形，取下一个号），每个对象行都写 `source`、没有的写 `null`；一件都不带时仍写 `TFC-1`、没有 `source` 键、逐字节不变。比对：新 `domain.CompareStoredPickupAttemptDigest`，仿 `CompareStoredHandoverDigest`——已存 `TFC-3` 与本次逐串比；其余交 `CompareStoredDigest`，本次是 `TFC-3` 而旧形答成同串时改答冲突；`TFC-2` 与认不出的前缀答认不出。`PerformOffsitePickupHandler` 改走它。金样：带来源那一格改钉 `TFC-3`，另加「pickup without any source stays TFC-1」证不带来源的仍是 `TFC-1` 原字节（新结果词 `EXECUTOR_REFUSED` 是取值不是键）；圆转断言改为文档自报版本等于摘要前缀。比对口有九格的表测 `TestStoredPickupAttemptDigestComparesThePendingSourceShape`；编排用例补「存下的是 TFC-3」「同内容重投答已有结果」「换来源答冲突」。头注里「而待确认是同一笔才进的文法」那句变更叙述随改写删去。票面代决 8 与「已接受 ADR」那句改写。
- **N1 交付读数对待确认无用例。** `read_delivery_gate_test.go` 在「a failed result gets no reading」旁加「a pending result gets no reading」：读数零值、`customs.queries` 为 0。评审的变异「待确认也问关务」（`!Succeeded()` 改 `Failed()`）对新用例跑，红。
- **N2 交付一侧单立一格（协调方裁）。** `DeliveryRegistrationOutcome` 加 `DeliveryResultPendingConfirmation`（`DELIVERY_RESULT_PENDING_CONFIRMATION`，200）；领域 `ErrDeliveryResultPendingConfirmation` 包着 `ErrNotAnEffectiveDelivery`，`FormEffectiveDelivery` 对待确认交回它，编排先认它。HTTP 状态码映射本就按 `String()` 透出、新落一版才 201，只改了 200 那一列的注释；全仓再 grep 一遍 `DeliveryNotEffective`、`NOT_EFFECTIVE` 与 `DeliveryRegistrationOutcome`，没有别的消费方（装配、派发、admin-web、演示动线都不分支在这一格上）。`DeliveryRegistrationOutcome` 头注里「那条更正口另票（code/90），落地时再看…」删去。UC-TF-006 结果契约「尝试待确认」与交接行按新格写；代决 6 改写；越权风险点加 7。
- **N3 拒收的责任方是未申报的代决。** TF CONTEXT 拒收那句与 `domain.ExecutorRefused` 头注删去「责任方是这次尝试的执行方」，头注改写为责任归属不由这一格推导、也不另存；UC-TF-002 交接表加一行「本次对象被拒收要保存责任方 → 拒收的责任归属不另存 → 待 TF owner 裁（不立票）」；代决 1 补一句，越权风险点加 8。
- **N4 code/90 票面。** Category 改 bug；「证据」单立一节归拢规则与代码；加「最小的诚实修法」（版本链或更正表、更正只回指待确认那一版、三个消费方改读当前有效版本、揽收更正成揽收到手走同一套、接入面待裁），并写明交付待确认那一格在更正落地后语义不变、不并回`未生效`；「要裁的」里「要不要从未生效分出来」那条随 N2 删去，补「复核清单随本票开还是另票」。
- **N5 注释。** `DeliveryObjectOutcome` 头注改为「库与内容比对都按词存、按词比，取值的整数不出本进程」；代决 9 改准为只对揽收成立。`formPickupObjectResult`、`formDeliveryObjectResult`、`rebuildPickupResult` 头注点名两个构造器符号。`AttemptResultPendingConfirmation` 从「未受理：改请求」那组挪到常量块末尾自己一组，注明续办是等确定结果再登。
- **协调方另加。** TF CONTEXT 收费原因句「失败揽收或派送尝试的尝试费」补「（含拒收）」；越权风险点 1 补存量 `PACKAGING_UNACCEPTABLE` 那一句。

### 修订轮验证

- 先红后绿：B1 的金样、比对口表测与编排用例先改先跑——领域编译不过（新符号不在），编排答 `PICKUP_UNDECIDED`（旧比对口不认 `TFC-3`，落认不出的形状）；N2 的领域、编排与生产装配用例先改，领域编译不过（新错误值不在）。实现后转绿。
- 变异（每条改后跑、跑完从 `scratchpad/c01/` 的备份复原、`cmp` 核过）：RB1 揽收执行口改回 `CompareStoredDigest`——编排红，生产装配真库红（同内容重投不再答 `EXISTING_RESULT`）；RB1b 带来源的文档仍写 `TFC-1`（同号加键）——比对口表测多格红、金样红；RB1c 比对口去掉「本次带来源、旧形同串答冲突」那道门——表测「unversioned row, this one carries a source」红；RN1（评审变异）`ReadRegistration` 的 `!Succeeded()` 改 `Failed()`，待确认也问关务——新用例红；RN2 编排去掉待确认那一格、并回 `NOT_EFFECTIVE`——编排红，生产装配真库红。五处全被杀。
- `go build ./...`、`go vet ./...` 干净，`gofmt -l internal cmd` 无输出。
- `TMPDIR=… scripts/test-full.sh ./internal/transportfulfillment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-transport-register/... ./internal/architecture/... ./migrations/...` 加 `go list -test` 算出的 TF 反向依赖（任一 `internal/transportfulfillment/` 包或 `migrations` 的反向依赖），对修订轮 tip 跑：重放到 `fb8ee7096` 前后各一次，都是 65 个包 ok、2 个无测试文件，退出码 0。
- 新改用例（本轮加的比对口表测，与 `TestTheOtherDeliveryNodeStopsAreNamedAndRegister` 一起）共二十二个顶层测试 `-count=3`（真库，`-v` 核过无 SKIP、无 FAIL），重放前后各一次：都是 66 次 PASS，退出码 0。
- 迁移 0050 本轮未动，未重灌种子。

### 第二轮评审与收尾轮

第二轮评审（协调方转述）：B1、N1–N5 都已落实；余一条阻断 B2、两条 N 与一条可选。收尾轮先 `git fetch -q origin` 再重放到 `origin/main` `4f29f861e`（AT 盘点文末追记与 demo-chain-gaps/134 那段冲突，两段都留、main 那段在前），新提交、不改写旧提交。

- **B2 揽收比对口借了交接那一口的停用（阻断，协调方裁走 (a)、不立记录）。** 评审的探针：同 `sourceId` 先登 `CUSTOMER_ABSENT`（存 TFC-1），再原样重交、只给这一行带 `source`，答 `SOURCE_CONFLICT`；换新 `sourceId` 答 `SOURCE_NOT_ACCEPTED`——同一种畸形命令两种答法，前一种是按版本不同判了冲突，ADR-0014 不许。改法：`PerformOffsitePickupHandler.Handle` 入口先判 `determinateResultCarriesSource`，确定结果带来源在内容比对之前答 `SOURCE_NOT_ACCEPTED`（与新键同答，ADR-0029 恢复动作是改请求）；`formPickupObjectResult` 里那道判挪走，头注写明入口已拒。于是到得了 `CompareStoredPickupAttemptDigest` 的带来源命令必含待确认，它对旧形答冲突与按旧形重算同答，头注改写为这条等价理由，删去「理由同 CompareStoredHandoverDigest」与那句不成立的「同一来源身份换成带来源的待确认就会被认作重放」。派送一侧不用动：`RecordDeliveryAttemptHandler` 本就在读册比对之前形成逐对象结果，确定结果带来源先答未受理。补编排用例 `TestADeterminateResultCarryingASourceIsNotAcceptedBeforeTheContentComparison`（在册 TFC-1、同来源重交确定结果带来源 → 未受理、原记录不动、不签版本不交意图），先跑红（答 `SOURCE_CONFLICT`，即评审探针）后绿；比对口表测「TFC-2」那一格的 current 改成揽收口产得出的 TFC-1。票面代决 8、「做了什么」摘要一条与「已接受 ADR」那句照实改写：ADR-0014 照用、不构成部分停用，也不借 ADR-0213 那一次停用；TFC-3 的出处写在代决 8 与常量头注。
- **N-a 拒收责任归属的去处不指票。** 立票 [code/91](./91-an-executor-refusal-records-no-responsibility-attribution.md)（needs-triage，立号前 ls 核过 91 空），UC-TF-002 交接表那一行去处改链到它，越权风险点 8 同改。
- **N-b code/90 两表主键写不准。** 证据里分开写：`pickup_attempt_result` 主键 `(tenant_id, source_id, object_ref)`、`delivery_attempt_result` 主键 `(tenant_id, attempt_ref, object_ref)`；修法第 1 条点出版本链挂哪个键随之定（揽收按来源身份落，同一尝试身份对几份提交今天没有约束）。
- **可选（做了）** UC-TF-006 结果契约「尝试待确认」：单答一格与续办挪到业务含义一列，禁止行为一列只留「不按外部最后状态码裁决；不形成有效交付，不并进`未生效`」。

收尾轮验证：

- 变异 RB2「拒收挪回比对之后」（入口那道判删去、放回 `formPickupObjectResult`）——新编排用例红；从 `scratchpad/c01/` 备份复原、`cmp` 核过。
- `go build ./...`、`go vet ./...` 干净，`gofmt -l internal cmd` 无输出。
- `TMPDIR=… scripts/test-full.sh ./internal/transportfulfillment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/...`：15 个包 ok、1 个无测试文件，退出码 0。
- 新改用例六个顶层测试 `-count=3`（真库，`-v` 核过无 SKIP、无 FAIL）：18 次 PASS，退出码 0。
- 收尾前 `origin/main` 又前移到 `b685046d3`（只有一笔 party-commercial-uiux 的票面），重放无冲突；那一笔不动代码，上面几跑不重跑。

## 进 main 记录（2026-10-26，协调方）

- 分支 `claude-code01` 由作者 rebase 到 origin/main `b685046d3` 后快进，tip `7f3e4a770`；迁移 TF 0050；揽收带来源的规范化形取新号 TFC-3（不带来源仍 TFC-1、逐字节不变）。
- 第一轮隔离评审一条阻断：B1 揽收摘要同号（TFC-1）加 `source` 键，违 ADR-0267 决定一。协调方裁按内容取最低一版（ADR-0267 决定一、ADR-0209 决定三）换号 TFC-3，另裁交付登记对待确认单立 `DELIVERY_RESULT_PENDING_CONFIRMATION`（与失败尝试费一侧同一把尺，ADR-0029）；N1–N5 与协调方另加两处在修订轮收。
- 第二轮评审一条阻断：B2 揽收比对口照抄交接口的做法，借用了 ADR-0213 只为交接首登一口登记的 ADR-0014 部分停用。协调方裁不立记录：确定结果带来源在内容比对之前答未受理，于是到得了比对口的带来源命令必含待确认，比对口答冲突与 ADR-0014 按旧形重算同答。N-a 立 code/91，N-b 改准 code/90 证据。收尾轮 diff 与处方一致，协调方核过后未再起第三轮评审。
- 协调方单独提交重算的 AT 计数与重生成的机制清点；ADR 索引无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净；演示种子在一次性库上灌通（施加 0050），用后删库。
- 去向：code/90（待确认那一格没有同一次上门的更正口），code/91（拒收的责任归属要不要另立一格、由谁判）。
