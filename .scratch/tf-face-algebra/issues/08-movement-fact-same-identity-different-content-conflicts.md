# 08 移动事实同身份异内容答「已有版本」：作业事实口缺内容冲突格，另一台设备或另一作业范围的真实作业被静默吞掉

Category: bug
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 代决（见「裁定」）。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment/movement-facts
地盘：
- internal/transportfulfillment/domain/payload_canonicalization.go 及其测试
- internal/transportfulfillment/application/record_movement_fact.go
- internal/transportfulfillment/application/record_movement_fact_test.go
- internal/transportfulfillment/adapters/http/record_movement_fact.go 及其测试
- cmd/parcel-api/（新增真渠道真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

照「修复方向（机制）」做，「应当」里留给 TF owner 的两处张力由本会话代决：

1. **同身份异内容答 `SOURCE_CONFLICT`（200，不回显、不落库、不另起版本）。** 依据 ADR-0149 决定四「摘要作同身份重放与内容冲突的判据」与同族各口；ADR-0023 决定「同一身份重复到达返回原结果」说的是重复到达，不是异内容。TF CONTEXT「同一源事件再次到达 → 重复投递：返回原结果」是外部承运轨迹的规则，不适用于设备作业事实。
2. **业务时间不进摘要，照 ADR-0023 字面**（「发生时间与接收时间……两者都不进入内容摘要」）：只差发生时间的同身份重投读作重放、交回原结果。同族的揽收登记形状 `CanonicalizeOffsitePickupRegistrationPayload` 把 occurred_at 放进了摘要，与 ADR-0023 字面不一致——那一处不在本票，留给 TF owner 另裁（追加复核记录或改形状），本票不跟着照抄。
3. 身份在册时先比内容、后判门禁，免得重放被支去取放行。

越权风险点：第 2 条与研判推荐相反（研判推荐照同族把业务时间放进摘要），本会话按权威次序取已接受 ADR 的字面；TF owner 复核时看设备侧的发生时间是否真的不会在同一身份下变化。

## 现象
- 同一（租户、事实、版本）带着不同内容再到时，一律答 200 `MOVEMENT_FACT_VERSION_EXISTS`，回显在册那一份，不落库、不留痕。不同内容包括：种类与地点、只差亚秒、只差整秒、只差班次、带放行的受门禁出发、换放行引用、去掉门禁与放行。
- 另一台在册设备带同一（事实、版本）来，答已有版本。实测两种情形：同作业范围；另一作业范围上的另一名操作者。后者还会被回显出另一作业范围的事实内容。
- 竞态：并发异内容的输家都答已有版本，且回显赢家。自然竞态多轮实测，`Save` 撞键后走「读回赢家」那一支，不比内容。
- 次序：只多 `gateRequired=true` 而不带放行时答 `DEPARTURE_GATE_BLOCKED`。原因是门禁先判，查册在后。
- 去处：处理器注释给的去处是「换版本重投」，可那样登出来的是互不回指的并列版本（v2 的 `corrects_version` 为 NULL）。本口载荷带前版格（`corrects`、`correctsVersion`、`predecessorVersion`）一律答 400，领域 `TransportMovementFact.Correct` 在生产上也没有调用方。
- 回显：首登回显取内存那一份（纳秒），重放回显取库面那一份（微秒）；业务时间回显只到秒。

## 复现（S）
命令：在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`；各 overlay 与日志都在同一目录。
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/movement-facts/probe_movement_fact_test.go 的 `TestProbeMovementFactReplayAndSameIdentityDifferentContent`、`TestProbeMovementFactRaceLoserWithDifferentContent`
- 同目录 probe_movement_fact_prod_registers_test.go 的 `TestProbeMovementFactFormsDecisionsOnProductionRegisters`（生产登记口登授予与设备；日志 run1.log、run2.log）
- 同目录 verify/verify_movement_fact_test.go 的 `TestVerifyMFCrossDeviceSameIdentityIsSwallowed`、`TestVerifyMFNaturalRaceWithDifferentContent`、`TestVerifyMFGateFlagIsCallerSuppliedAndClearanceIsNotCompared`（run-verify1.log）
- 同目录 adv-impl/adv_movement_fact_test.go 的 `TestAdvMFIdentityKeyIgnoresDeviceAndWorkScope`、`TestAdvMFGateFlagOnlyDifferenceIsJudgedBeforeTheLookup`、`TestAdvMFFirstEchoAndReplayEchoComeFromDifferentPrecisions`（run1.log）
- 同目录 adv-authority-r2 的 `TestAdvAuthContinuationIsAParallelVersionNotACorrection`、`TestAdvAuthTrackingSameSourceEventWithDifferentContentIsADuplicate`（run.log）
- 同目录 verify-authority/probe_mf_authority_test.go 的 `TestVerifyMFAHasNoCorrectionPortOnThisFace`（run.log）

## 应当（推荐，待 TF owner 裁）
同身份同内容答 `MOVEMENT_FACT_VERSION_EXISTS`；同身份异内容答 `SOURCE_CONFLICT`（200），不回显、不落库、不另起版本。

依据：
- ADR-0149 决定四：摘要「作同身份重放与内容冲突的判据」。
- UC-TF-005 把「重复返回原结果」与「冲突保留并待复核」分成两格。
- ADR-0023 后果节：「判错的方向是把前者当后者，那会静默吞掉真实作业」。
- 同族各口都已答 `SOURCE_CONFLICT`：收寄、交接首登、交付首登、揽收首登、派送尝试。

票 20「回显让调用方自己比」的前提是控制台前的操作者。作业事实族的调用方是离线队列，按 ADR-0022 的表 2xx 即出队，这条前提不成立。

冲突答复不回显，正好关掉「另一作业范围的内容被回显」这一格。

要 TF owner 一并裁的两处张力：
- TF CONTEXT 对外部承运轨迹的规定是「同一源事件再次到达 → 重复投递：返回原结果」，实测异内容也答 `DUPLICATE_DELIVERY`。那是外部源的规则，本票认为不适用于设备作业事实，需 owner 认可。
- ADR-0023 写发生时间与接收时间「两者都不进入内容摘要」，而同族 TFC-1 形状 `CanonicalizeOffsitePickupRegistrationPayload` 把 occurred_at 放进了摘要。本票推荐照同族形状进摘要；若 owner 照 ADR-0023 字面裁，就去掉业务时间那一格。

## 根因
- `RecordMovementFactHandler.Record` 两处都只认身份、不比内容：
  - 「找到在册版本」那一支只按键找；注释是「同一版本重投：交回原版本，不顶替。改内容要换版本号，那是更正不是重投」。
  - 「并发撞键读回赢家」那一支同样不比。
- 本口没有 TFC-1 形状。`TestASecondRecordOfTheSameVersionKeepsTheOriginal` 把「异内容答已有版本」钉成了现行行为。

## 修复方向（机制）
1. 领域：在 `payload_canonicalization.go` 加 `CanonicalizeMovementFactPayload`，形状 TFC-1，face 取 `RECORD_MOVEMENT_FACT`。
   - 进摘要：班次、种类（取 `MovementFactKind.String()` 的词形）、地点、放行引用（缺席写 null），以及按本票推荐的业务时间（经 `AtRegistryPrecision`）。
   - 不进摘要：租户、事实、版本（这些是身份）；来源（由这一面注入，隔离写时期的存量行带合成来源）；`GateRequired`（`RehydrateMovementFactSpec` 注释说它是判断的输入、不是事实的属性）；RecordedAt。
2. 两侧都从在册本体现算后再比，不加摘要列、不动迁移，照票 13 凭证登记口与票 20 承运商判断口的做法。
3. 编排：「找到在册版本」与「并发撞键读回赢家」两支都比。同串照旧答已有版本；异串答新格 `SOURCE_CONFLICT`。领域门禁与内容比对的先后要定死：推荐身份在册时先比内容，免得把调用方支去取放行。
4. HTTP：`writeMovementFactOutcome` 给新格映射 200、不带记录；时刻回显随票 31。
5. 测试：改写 `TestASecondRecordOfTheSameVersionKeepsTheOriginal`，补 TFC-1 往返用例。

## 不做
- 身份键不含设备与作业范围。ADR-0023 的「由记录它的设备签发」服务端核不到，这一点留给 TF owner，本票不改键。
- 作业事实铸造不问准入范围（核验实测铸造请求不带准入要求），归票 10 越权风险点。
- 零时刻与 NUL，归〈TF 构造门存得下〉。

## 是否潜伏
已可触发。探针用生产 `buildWorkFactIntakes` 接真库操作者册与设备册：登上一笔作业事实授予和一台设备，就形成了决定，且同身份异种类答已有版本。

## 完成判据
- [ ] 领域单测：TFC-1 往返（并入 `TestCommandPayloadsRoundTripAsTFC1`）；纳秒与对应微秒判等；来源、`GateRequired` 不进摘要。
- [ ] 编排单测：同内容答已有版本；改种类、地点、班次、业务时间、放行各答 `SOURCE_CONFLICT`；并发输家读回赢家后比；身份在册时只多 `gateRequired` 不答门禁格。
- [ ] 真渠道真库回归（`cmd/parcel-api` 新测试文件，生产 `buildWorkFactIntakes` + 真库操作者册与设备册 + `buildMovementFactOrchestration`）：
  - 原样重放答 200 已有版本，库不变；
  - 同作业范围另一设备异内容、另一作业范围异内容，各答 200 `SOURCE_CONFLICT`，答复不含在册内容，库不变；
  - 自然竞态多轮，输家异内容答 `SOURCE_CONFLICT`。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...`，全部 ok。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验、非作者评审（两轴分开）、推送方在干净检出上带 DSN 跑全量。
