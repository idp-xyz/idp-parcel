# 30 TF 登记册上的时刻按微秒认：带纳秒的原样重放被判成冲突或多签一版，同一微秒内的时间窗与恰在起点的作废答 500

Category: bug
Status: in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 序 3 评审遗留；用户授权代决）；分支 `claude-oc30`
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)（评审欠账）。被评的是 [20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md)，代码 `857453d8`。
出处：
- 票 20 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `857453d8`」，Standards 非阻断 (1) 前半与 (2)。
- 票 26「评审结果」序 3：「子代理建议另立：给 TF 定一处在册时刻精度规则」。
- 取证钉 main `8d22729f`；`8d22729f..c68e4342` 在 `internal/transportfulfillment` 下无差异。证据层级 `S`。
出处：票 20 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `857453d8`」，Standards 非阻断 (1) 后半、(3)、(4)。取证钉 main `8d22729f`，`c68e4342` 上相同。证据层级 `S`。
地盘：
- internal/transportfulfillment/domain/external_carrier_credential.go
- internal/transportfulfillment/domain/master_document.go
- internal/transportfulfillment/domain/payload_canonicalization.go
- internal/transportfulfillment/domain/dispatch_task.go
- internal/transportfulfillment/domain/fulfillment_attempt.go
- internal/transportfulfillment/domain/（新增 AtRegistryPrecision 所在文件及其测试；Equal 与严格门的单测）
- internal/transportfulfillment/application/register_external_carrier_credential.go
- internal/transportfulfillment/application/judge_external_tracking_effective_time.go
- internal/transportfulfillment/application/record_delivery_attempt.go
- internal/transportfulfillment/application/（上列三口的无 DSN 单测，宜新增测试文件）
- internal/transportfulfillment/adapters/postgres/（新增真库回归测试文件）
- internal/transportfulfillment/application/judge_carrier_first_effective_pickup.go
- internal/transportfulfillment/application/judge_carrier_first_effective_pickup_test.go

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

本票并入了同簇的小票「承运商首次有效收寄判断：同依据重放头注去掉 CCC-1 先例，冲突断言改成拿得住误落一版」，见末节。

## 现象

TF 各登记册的时刻都落在 `timestamptz` 列上，写入时截到微秒（pgx 二进制编码按微秒向下取整）。HTTP 各入口按 `time.Parse(time.RFC3339, …)` 解析，小数秒照收到纳秒。于是，凡是拿「命令带来的时刻」和「读回的在册时刻」按纳秒比较的地方，原样重放都会判错。

真库探针实测条件：门禁库 postgres:16.14，pgtest 每例自建物理库；命令时刻取 `…09:00:00.123456789Z`，对照组取 `…09:00:00.123456Z`。结果如下：

| 口 | 按纳秒比较的那一处 | 带纳秒原样重放 | 对照组 |
|---|---|---|---|
| 凭证首登 | `ExternalCarrierCredential.Equal`（经 `replayOrConflict`） | `CONTENT_CONFLICT` | `EXISTING_VERSION` |
| 凭证首登·并发输家 | `commit` 读回赢家后，同上 | `CONTENT_CONFLICT` | `EXISTING_VERSION` |
| 凭证改变适用关系 | `RegisterExternalCarrierCredentialHandler.replayAgainstCurrent` 的「`changedAt.Equal(command.At.UTC())`」 | `CONTENT_CONFLICT` | `EXISTING_VERSION` |
| 总单形成新版本 | `MasterDocument.Equal` 的 `changedAt`（经 `masterDocumentReplayOrConflict`） | `CONTENT_CONFLICT` | `EXISTING_VERSION` |
| 有效时间显式判断 | `JudgeEffectiveTimeHandler.Judge` 的「`at.Equal(command.EffectiveAt.UTC())`」 | 同一命令连发，依次签出 `EXTV-N1`、`EXTV-N2`、`EXTV-N3` | 后两次答 `ALREADY_JUDGED_AS_GIVEN`，仍指 `EXTV-N1` |
| 派送尝试 | `deliveryAttemptContent`（计划窗两端、到场时刻、逐对象发生时刻都按 `RFC3339Nano` 进串） | `SOURCE_CONFLICT` | `EXISTING_RESULT` |

承运商首次有效收寄判断是对照：票 20 已在 `CanonicalizeCarrierFirstEffectivePickupPayload` 里内联截到微秒，同法实测答 `ALREADY_RECORDED`。

同一条规则还有另一半：严格先后的构造门。领域按纳秒判「严格晚于」，库面 CHECK 按截后的微秒判 `>`。当窗口或区间整段落在同一微秒内时（例如起点 `.123456100`、终点 `.123456900`），领域门放行，写入撞上 CHECK，事务回滚；编排答`未决`并带续办，重试永远是同一个答复：

| 领域门 | 库面约束 | 真库实测 |
|---|---|---|
| `OpenDispatchTask` 的「`!spec.WindowTo.After(spec.WindowFrom)`」（`DispatchTask.Reschedule` 同形） | `dispatch_task_window_ordered` | `DISPATCH_TASK_UNDECIDED`，事务回滚 |
| `FormFulfillmentAttempt` 的「`!spec.PlannedTo.After(spec.PlannedFrom)`」（派送、揽收共用） | `delivery_attempt_window_ordered`、`pickup_attempt_window_ordered` | 派送尝试答 `DELIVERY_ATTEMPT_UNDECIDED`，事务回滚 |
| `NewCredentialApplicability` 的「`!until.After(from)`」；`CredentialApplicability.closedAt` 的「`at.Before(applicability.from)`」 | `external_carrier_credential_range_ordered` | 首登与作废都答 `REGISTRATION_UNDECIDED`（`CREDENTIAL_REGISTRY_UNAVAILABLE`），事务回滚 |

经真装配 `buildExternalCarrierCredentialRegistration`（事务包装加真登记册）走 HTTP，同一微秒内的适用区间首登与重试都答 500 `NO_ANSWER_FORMED`。原因是事务包装把提交错误原样交回，而 `commandEndpoint` 按「编排交回 error 是没形成答案」写 500。派送任务、派送尝试与揽收尝试的装配（`transactionalDispatchTaskOpener`、`transactionalDeliveryAttempt`、`transactionalPickupAttempt`）形态相同，按此推得答复相同，未逐一实测。

`closedAt` 还有一格与精度无关，但长在同一道门上：作废点恰好等于起点（整秒）时，会落出一个空区间，同样撞上 `external_carrier_credential_range_ordered`，HTTP 同样答 500。

单测为什么没拦住：应用层的登记册替身原样存下纳秒（`credentialRegistryDouble.Save` 直接追加）；`adapters/postgres` 的真库用例只测往返，不跑编排的重放。

暴露面与后果：派送尝试经作业事实渠道进来，设备与集成客户端常发 9 位小数，暴露面最大。有效时间判断挂在操作者真渠道 `operatorDecisions.transport` 上，每重试一次就多一代事实、再交一份意图，后果最重。

## 裁定（2026-10-16，会话 `idp-parcel-9b` 代决；归机制类，不涉租户取值）

1. **精度规则只在 TF 领域定义一处。**
   - 在 `internal/transportfulfillment/domain` 新增导出函数 `AtRegistryPrecision(at time.Time) time.Time`，实现为 `at.UTC().Truncate(time.Microsecond)`。
   - 它的中文头注是这条规则在 TF 的唯一陈述，写成本上下文的决定，而不是转述驱动的行为。规则原文：本上下文按微秒认登记册上的时刻；凡结论要在入册之后仍然成立的时刻比较，即同键内容相等（重放与冲突）和严格先后的构造门，两侧都先折到这一精度再判；只改判定，不改存值。
   - 头注另写一句理由：登记册的时刻列是 `timestamptz`，存不下微秒以下。
   - 再写一句前提：本上下文登记册的时刻一律落在 `timestamptz` 列上；哪一册改存 jsonb 快照或改了列型，这条要重看（PS 受控关闭册的 jsonb 快照就保留纳秒）。
   - 同族先例用符号名引 accessidentity 的 `sameInstant`。
   - 函数不叫 `RecordedInstant`：本上下文到处都有 `RecordedAt`（登记落库时刻），同形的名字套在业务时间上，容易被读成登记时刻。
2. **相等类：两侧先折再比。** 涉及以下各处：
   - `ExternalCarrierCredential.Equal` 的适用范围起止与 `changedAt`；
   - `MasterDocument.Equal` 的 `changedAt`；
   - `RegisterExternalCarrierCredentialHandler.replayAgainstCurrent` 中 `changedAt` 对 `command.At` 的比较；
   - `JudgeEffectiveTimeHandler.Judge` 中在册有效时间对 `command.EffectiveAt` 的比较；
   - `deliveryAttemptContent` 的计划窗两端、到场时刻与逐对象发生时刻；
   - `CanonicalizeCarrierFirstEffectivePickupPayload`：把内联的 `Truncate(time.Microsecond)` 换成调用 `AtRegistryPrecision`，摘要字节不变。
3. **严格先后类：两侧先折再判。** 涉及以下各处：
   - `NewCredentialApplicability` 与 `CredentialApplicability.valid` 的「终点晚于起点」，两处共用同一个判定；
   - `CredentialApplicability.closedAt` 的「收掉的终点须严格晚于起点」与「既定终点之后不能再收」；
   - `OpenDispatchTask` 与 `DispatchTask.Reschedule` 的时间窗；
   - `FormFulfillmentAttempt` 的计划窗。

   改后，同一微秒内的窗口或区间答`未受理`（派送尝试答 `SOURCE_NOT_ACCEPTED`），事务不回滚。
4. **作废点恰在起点，答`未受理`。** `CredentialApplicability` 自己的不变式（`valid`，以及 `NewCredentialApplicability`「非零时必须晚于起点」）与库面 `external_carrier_credential_range_ordered` 早已一致要求终点严格晚于起点，`closedAt` 是唯一漏网的路径。这一格今天从未成功过，只会答 500，所以改成`未受理`不会拿走任何已有能力。如果业务需要「自始作废」，那是要改类型不变式与迁移的新建模，由 TF owner 另立票。
5. **不动的部分。**
   - `canonicalInstant` 与所有经 `CompareStoredDigest` 比对的口：摘要是落库前由命令原值算出的，原样重放必然同串；已存摘要不在原处改写（ADR-0014）。
   - 非严格的先后比较：向下截断保得住「不早于」。
   - 只认身份的重放。

   由此留下一处有意为之的不对称：存摘要的口按纳秒比，两侧现算的口按微秒比。原样重放在两类口上都认作重放，只有「亚微秒不同的两份命令」答法不同：存摘要的口答冲突，现算的口答重放。这条写进 `canonicalInstant` 的头注作唯一陈述：它不能原地折到微秒，要折就得换形状版本。
6. **不放 PG 适配器，不做 platform 共用，不在入口截，不在构造时归一。**
   - 不放 PG 适配器：TF 适配器只答「已登记」，内容比对发生在编排与领域里，`ExternalCarrierCredential.Equal`、`MasterDocument.Equal` 本身就在领域。规则放在比较发生的那一层。accessidentity 的「撞键后读回再比」写在 PG 适配器里，所以它的 `sameInstant` 在那里，只截给定的一侧；同一条规则按比较所在各放一处，并不矛盾。TF 取两侧对称，是因为并发输家那条路比的是「读回的赢家」对「本次命令」，`Equal` 哪一侧来自库并不固定。
   - 不做 platform 共用：精度随持久化形态而变，PS 的 jsonb 快照保留纳秒，所以这不是全仓事实；而且领域类型「只用标准库和框架的领域事件合同表达」（`internal/architecture` 的 `infrastructureImports` 头注）。
   - 不在入口截：入口截会让存摘要口同一线载荷算出的摘要变化，与已存摘要失配（ADR-0014）；精度也不属于传输层。
   - 不在构造时归一：那要动所有带时刻的构造门，而直接拿命令字段比的地方仍要过函数，本票不取。
7. **定性：这是让实现回到已成文口径的机制修复，不是新定语义。** 依据如下：
   - [UC-TF-006](../../../docs/application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md)「同一任务、尝试、对象和来源版本只能形成一个结果；相同内容重复回传返回原结果」；
   - [UC-TF-004](../../../docs/application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md)「相同身份内容不同形成冲突」；
   - 票 15 进展记录对有效时间判断代数的核对：「异内容是改判，换一版而不是冲突」，而逐字节相同的命令不是异内容；
   - `ExternalCarrierCredential.Equal` 与 `MasterDocument.Equal` 头注所引 [ADR-0031](../../../docs/adr/0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md) 的结果代数；
   - 严格先后那一类，依 ADR-0031「`error` 留给『这次写入没能完成』」与「分格判据是消费方的恢复动作」：永远存不进去的输入，该答「改输入」，不该答「等恢复后重试」。

   **不立 ADR，不写复核记录：** 没有改任何已接受 ADR 的决定；ADR-0014 管的 `canonicalInstant` 不动；只改判定，不改存值与已存摘要；没有存量要迁移；可逆。**不改 CONTEXT：** 这是实现精度，不是领域语言。
8. **取代与收窄。**
   - 票 20「做什么」里「业务时间 `occurred_at`（照 `canonicalInstant`）」那一句，由本裁定取代为「先经 `AtRegistryPrecision`，再照 `canonicalInstant`」。
   - 票 20 完成记录里「别的口存的是落库前算好的摘要，不受影响」，只对存摘要的口成立。

## 做什么

1. 在 `internal/transportfulfillment/domain` 新增 `AtRegistryPrecision`（放在新文件里，文件名由实施方定），头注照裁定 1。
2. 相等类照裁定 2 改。`ExternalCarrierCredential.Equal`、`MasterDocument.Equal` 的头注各补半句：时刻按登记册精度比。`deliveryAttemptContent` 头注里「读回的记录经构造门重建后与新提交同形」改成「在登记册精度上同形」。
3. 严格先后类照裁定 3、4 改。
4. `canonicalInstant` 补头注，写明裁定 5 的不对称。
5. `CanonicalizeCarrierFirstEffectivePickupPayload` 的头注改三处：
   - 「进段三格同 CanonicalizeHandoverPayload 不进摘要」改为点名：命令上的进段引用（`JudgeCarrierFirstEffectivePickupCommand` 的 `Segment`、`PlannedSegment`、`SegmentServiceAction`）同 `CanonicalizeHandoverPayload` 不进摘要。
   - 删去「别的口存的是落库前算好的摘要，不受此影响」，这句对两侧现算的口不成立。
   - 讲业务时间截法的那段收成一句，指向 `AtRegistryPrecision`；「`canonicalInstant` 本身不能改」的理由挪到 `canonicalInstant` 自己的头注。
6. 测试照完成判据补。无 DSN 用例的替身若要模拟读回，用测试里独立写的 `Truncate(time.Microsecond)`，不要调被测函数，免得测试同义反复。
7. 新增生产文件后，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`，随这一笔提交（生成物不占号）。
8. 可加：在 `cmd/parcel-api` 用真装配补一例，验证凭证同一微秒内的区间经 HTTP 答 200 `INPUT_NOT_ACCEPTED`，而不是 500。

## 不做

- 存摘要的口与 `canonicalInstant`（裁定 5）；构造时归一（裁定 6）；「自始作废」（裁定 4）。
- TF HTTP 答复的时刻回显：归本簇另立的「TF 命令答复的时刻经登记册精度带小数秒回显」，那张票以本票进 main 为前置。
- `replayOrConflictOnTheSameBasis` 的头注与 `assertConflictLeavesTheChain`：归本票末节「并入：承运商首次有效收寄判断的头注与冲突断言」。
- 同族旁支，生产上走不到：`ruleReplayOrConflict` 比较 `EffectiveTimeRule` 的 `Content()`，其中 `Offset` 在库面按整秒存（`offset_seconds`）；但它唯一的生产者是 HTTP，而 HTTP 只收整数 `offsetSeconds`。
- 查过、不在此列：
  - `CapacityPool.Reserve` 的有效期门是严格比较，但预占时刻不入册，入册之后没有人再判这对先后。
  - 预占是否过期（`occupiedAt` 等）是拿命令时刻对微秒边界比，折与不折答案相同。
  - `CredentialApplicability.Covers` 同理。
- 别的上下文：不在本票裁，也不在本票改，抄送各上下文 owner。
  - customs-compliance 的 `sameCredentialContent` **已实测同病**。它两侧都经 `CanonicalizeCredentialRegistrationPayload` 用纳秒的 `canonicalInstant` 现算，而 `valid_from`、`valid_to` 是 `timestamptz`。实测：期限带纳秒的监管凭证首登后原样重放，答 `CONTENT_CONFLICT`；只到微秒时答 `EXISTING`（代决阶段真库探针，钉 `8d22729f`）。
  - 其余只是线索，各 owner 须先核持久化形态（列，还是 jsonb 快照）再定。取线索的办法：在本上下文 `application`、`domain`、`adapters/postgres` 的非测试文件里，用 `grep -rnF '.Equal('`（去掉 `bytes.Equal`、`slices.Equal`）与 `grep -rnF 'Format(time.RFC3339Nano)'` 找出比较点，逐处看是否一侧来自命令、一侧来自 `timestamptz` 读回。
  - 代决阶段已 grep 到的线索：customs-compliance `register_ports_paths.go` 的 `existing.AppliesFrom.Equal(command.AppliesFrom)`；collection-remittance `post_subledger.go` 的 `OpenedAt`、`PostedAt`，以及 `remittance.go` 的 `CollectedThrough`、`FormedAt`；party-commercial `register_party_identity.go` 与 `register_registration_number_type.go` 的 `at.Equal(command.At.UTC())`。

## 完成判据

- [ ] 领域单测 `AtRegistryPrecision`：去掉纳秒尾巴；跨一微秒仍判为不同；非 UTC 输入与对应的 UTC 时刻同值；零值仍 `IsZero`。
- [ ] 领域单测：`ExternalCarrierCredential.Equal`、`MasterDocument.Equal` 对「纳秒版 / 微秒版」判等，两个方向都要断言。
- [ ] 编排层无 DSN 单测：凭证首登、凭证改变适用关系、总单形成新版本、有效时间显式判断、派送尝试，各一组。替身里预置截到微秒的在册记录；命令带纳秒原样重放，答重放那一格；+1µs 时答冲突、签新版或 `SOURCE_CONFLICT`。
- [ ] 真库回归（`adapters/postgres`，编排加真登记册，同一事务），上列各口各一组。所有时刻格都带纳秒尾巴：凭证的起点、终点与 `changedAt`；派送尝试的计划窗两端、到场时刻与逐对象发生时刻。每组各配一个 +1µs 对照。
- [ ] 有效时间显式判断的重放：答 `ALREADY_JUDGED_AS_GIVEN`，不签新版，重发的意图仍指原版本。依据是 `Judge` 头注「交回原版本并重发意图」，去重发生在 `OutboxExternalTrackingFactHandoff`（`outboxintent.EnqueueOnce` 按含版本的事件 ID）。不要写成「不再发意图」。
- [ ] 凭证首登并发输家的真库用例：两个事务，READ COMMITTED。A 写入后不提交；B 排在唯一键上；A 提交后，B 读回赢家，答 `EXISTING_VERSION`。排队的取证照 party-commercial `TestSameVersionCorrectionsSerializeOnTheVersionRow` 的做法。
- [ ] 严格先后门：派送任务窗、派送尝试计划窗、凭证首登区间、凭证作废点各取一例落在同一微秒内，都答`未受理`且事务不回滚；+1µs 时照常受理；作废点恰在起点答`未受理`。
- [ ] 往返钉：本票涉及的各册，写入带纳秒的时刻，读回等于 `AtRegistryPrecision(写入)`。前提一变，这条就红。
- [ ] 变异自检：把任一调用点改回按纳秒比较，对应用例转红。完成记录写明对哪些调用点做过。
- [ ] `TestCommandPayloadsRoundTripAsTFC1`、`TestCarrierPickupJudgmentBusinessTimeIsComparedToTheMicrosecond` 照过（摘要字节不变）；票 20、票 27 的既有用例照过。
- [ ] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全部 ok，`-v` 下 TF `adapters/postgres` 无 `--- SKIP`。
- [ ] `docs/product/MECHANISM-INVENTORY.md` 在干净检出上重生成，并随笔提交。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：从当时的 `origin/main` 起分支；作者自验；非作者评审（空闲通道，或 `/code-review` 的隔离子代理，两轴分开）；推送方在干净检出上带 DSN 跑全量。

## 并入：承运商首次有效收寄判断的头注与冲突断言

### 现状

- **`replayOrConflictOnTheSameBasis` 的头注。** 现在写的是「同 customs-compliance 凭证登记口的 CCC-1 先例」。「CCC-1」是 customs-compliance 领域常量 `payloadCanonicalizationVersion` 的值，不是行号，也不是计数，所以改它属于「优先用符号名」的改良，不是纠正违规。但它所指的先例 `sameCredentialContent`，在簇 C3 代决阶段已实测有纳秒同病：期限带纳秒的监管凭证首登后原样重放，答 `CONTENT_CONFLICT`。把它当先例摆在旁边，容易被照抄。
- **`assertConflictLeavesTheChain` 的断言太弱。** 它在 `TestASameBasisWithDifferentContentAnswersContentConflict` 里，断言的是「`len(fixture.pickups.rows) != len(fixture.handoff.intents)`」。冲突路若误落一版、同时发出一份意图，两边同时加一，测试照样绿。代决阶段做过变异实测：让冲突路误落一版、误发意图、答复仍带回原链尾，各子例照绿；换成强断言即转红（行数与意图数都从 1 变 2）。更现实的错法，即冲突路落版并带回新版本，已经被「`record.Key != tail.Key`」拿住，所以这条属于补漏。
- **评审 Standards (3)「手抄 `resolveSubject` 前半段」已由票 27 解决。** `readCarrierNaming`（`application/carrier_naming.go`）现由 `replayOrConflictOnTheSameBasis`、`JudgeCarrierFirstEffectivePickupHandler.resolveSubject`、`FormActualCarrierJudgmentHandler.resolveSubject` 共用。两个 `resolveSubject` 读完指名之后的后半段，读码看是同形的，只有结果类型和依赖名不同（`Directory` / `Identities`）；但两者的结果代数不同（`CarrierPickupJudgmentOutcome` 与 `CarrierJudgmentOutcome`），不再抽出来合并。

### 裁定（2026-10-16，会话 `idp-parcel-9b` 代决；只涉及注释与测试，不涉业务取舍）

1. `replayOrConflictOnTheSameBasis` 的头注去掉先例指引，直接陈述性质：册上不存摘要，两侧都从本体现算，没有无版本的旧摘要要认。不改成指向 `sameCredentialContent`。
2. `assertConflictLeavesTheChain` 改成能拿住「冲突路误落一版」：在调用前快照行数与意图数，或者逐个子例写死期望的绝对数（同一用例的重放分支就是这么写的）。冲突后断言两者分别不变，并断言 `FindCurrentByObject` 仍是 `tail.Key`。
3. Standards (3) 记为票 27 已解决，不再跟进。

### 做什么

1. 照裁定 1 改头注。
2. 照裁定 2 改断言。这个 helper 今天是在 `fixture.judge` 调用之后才拿到 fixture，所以快照要挪到调用之前：可以改成由 helper 接收命令、先快照再调用，也可以由各子例自己快照。
3. 变异自检见完成判据。

### 不做

- `CanonicalizeCarrierFirstEffectivePickupPayload` 的头注（「进段三格」与业务时间截法那段）：随operator-channel/30一起改，所在文件属于那张票的地盘。
- customs-compliance 的 `sameCredentialContent` 本身：归 customs-compliance owner，已在那张精度票的「不做」里抄送。

### 完成判据

- [ ] 换成新断言后，`TestASameBasisWithDifferentContentAnswersContentConflict` 的各子例照绿。
- [ ] 变异自检：把冲突路改成误落一版、发出意图、答复仍带回原链尾，用例转红；复原后转绿。完成记录写明做法。
- [ ] 头注不计数、不写行号、不写变更经过（AGENTS「改文档」「写代码注释」）。
- [ ] 改动包的 `go vet`、`go test -count=1` 通过，`gofmt -l` 无输出。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：开分支、作者自验、非作者评审（空闲通道或 `/code-review` 的隔离子代理）、推送方在干净检出上带 DSN 跑全量。
