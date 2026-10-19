# 41 TF 更正口的重放代数不全：POD 更正原样重发会追加新版本，交接更正同一新版本换内容答已有版本

Category: bug
Status: in-progress——2026-10-18 会话 `idp-parcel-9b` 认领（分支 `claude-oc41tfa07`，tf-face-algebra/07 随后在同一分支）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 序 5 研判；逐口核实测补强）
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判簇 C4 第四张草案；逐口核研判 POD 更正、交接更正两口
地盘：
- internal/transportfulfillment/application/register_effective_delivery.go
- internal/transportfulfillment/application/register_transport_handover.go
- internal/transportfulfillment/domain/payload_canonicalization.go（视裁定）
- internal/transportfulfillment/adapters/postgres 的有效交付登记册与迁移（视裁定）
- internal/transportfulfillment/adapters/http/work_fact_intake.go（仅当裁定改 POD 更正线格式时）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

取 (a)。POD 更正载荷指名前版（`predecessorVersion`），与揽收更正同形：链线性、一版至多被更正一次；指名已被更正过的版本时，同内容答已有版本、异内容答冲突。交接更正撞键后按已存 TFC-1 摘要（`CompareStoredDigest`，旧摘要照票 13 与无版本那一版比）区分 `EXISTING_VERSION` 与 `SOURCE_CONFLICT`；交接首登并发输家同版本异内容同样答冲突。POD 更正签版本号之前先过完备性检查。改线格式是端侧契约、难逆转，在 ADR-0149 后追加复核记录（决定原文不动）；今天没有调用方，现在改成本最低。附录是逐口核实测原稿（tf-face-algebra 研判的 POD 更正与交接更正两份），修时一并覆盖其中的迟到旧更正改回当前版、版本链分叉两格。

## 现象（钉 `8d22729f`）

- **POD 更正**（`/transport-fulfillment/delivery-proof-corrections`）：
  - `RegisterEffectiveDeliveryHandler.Correct` 每次先 `NextDeliveryResultVersion`，再 `CorrectProof`；`EffectiveDeliveries.Supersede` 把当前行翻成历史，再插一行新的。整个过程不比前版、不比摘要，载荷里也不指名前版。
  - 真库实测（生产装配的编排 + 作业事实渠道）：同一份 POD 更正原样重发，两次都答 201 `DELIVERY_CORRECTED`。第一次签出 `DRV-000000000002`，第二次又签出回指它的 `DRV-000000000003`，下游多收一份意图。
- **交接更正**（`/transport-fulfillment/handover-corrections`）：
  - `commit` 遇到 `HandoverAlreadyRegistered` 就读回赢家作答，不比 `CanonicalizeHandoverCorrectionPayload` 算出的摘要。首登那条路径是用 `CompareStoredDigest` 区分重放与冲突的。
  - 编排替身实测：同一个 `newVersion` 换了结论再提交，答 `EXISTING_VERSION`，册上仍是原结论——内容冲突没有被认出来。
- **一并看**：
  - 被 `CorrectProof` 拒掉的 POD 更正（缺更正时刻、更正早于交付发生时刻等）也已经签出了版本号。揽收那一侧的做法见 `pickupCorrectionFrom` 的注释「一份形成不了的更正不该烧掉一个版本号」。
  - 票 10 换上作业事实渠道的其余各口（移动事实、派送发起、派送尝试等，票 13 也把其中几口列在「未定形状」里）各凭哪条代数认重放，票面没有逐口记录。

## 要裁的一问

**POD 更正按什么认出「同一次更正」的重放。** 这是机制取舍，属于端侧契约，难逆转，要进 ADR，或在 ADR-0149 后追加复核记录（决定原文不动）。候选：

- (a) 载荷指名前版（`predecessorVersion`），与揽收更正同形。链是线性的，一版最多被更正一次；指名一个已被更正过的版本时，同内容答已有版本，异内容答冲突——即票 tf-segment-lifecycle-closure/08 给揽收更正定的那一套。需要改线格式。
- (a′) 载荷带设备签发的新版本标识，与交接更正的 `newVersion` 同形。同键同内容是重放，同键异内容是冲突（TFC-1 那一类）。需要改线格式，版本号也要从服务端签改为设备签。
- (b) 不改线格式：当前版如果已是内容相同的更正，就答已有版本。缺点：只比当前版，一条旧更正若迟于新更正到达，会被当成又一次更正；而且这等于从内容派生身份，与 ADR-0023 相抵。

研判建议取 (a)。理由：更正各口里，揽收更正已经是这一套；指名前版让「同一次更正」有一个不从内容派生的锚；今天没有调用方，现在改线格式成本最低。

交接更正那一格不用另裁：形状已经有了（`CanonicalizeHandoverCorrectionPayload`），照首登的做法补 `CompareStoredDigest` 区分重放与冲突即可（ADR-0149 决定四）。

## 做什么（裁定之后）

1. POD 更正照裁定补上重放代数，形状进 TFC-1 并带形状版本，摘要进册行。
2. 交接更正：`commit` 的 `HandoverAlreadyRegistered` 分支按已存摘要区分 `EXISTING_VERSION` 与 `SOURCE_CONFLICT`。比较用 `CompareStoredDigest`；旧摘要照票 13 的做法，与无版本那一版比。
3. POD 更正在签版本号之前先过完备性检查，至少要求「更正时刻在场」；被拒的更正不签版本号，照 `pickupCorrectionFrom` 的做法。
4. 票 10 换上作业事实渠道的其余各口，逐口记下凭哪条代数认重放（只核对，记在票 10 或本票）；缺代数的另立票。

## 不做

- 不改更正口的时刻分格：已由「TF 更正载荷缺时刻」票定。
- 不改揽收更正：它已经有线性链和摘要比对。

## 完成判据

- [ ] 「要裁的一问」已裁，写进本票；难逆转的部分进 ADR 或复核记录。
- [ ] POD 更正原样重发不再追加版本，答已有版本并交回原版本；异内容按裁定答冲突或新更正。领域、编排、真库各有回归用例。
- [ ] 交接更正：同一新版本换内容答 `SOURCE_CONFLICT`，同内容答 `EXISTING_VERSION`；有编排用例和真库用例。
- [ ] 被拒的 POD 更正不再签版本号，用例断言签发次数不变。
- [ ] 票 10 其余作业事实口的代数已逐口记下。
- [ ] Go 门禁与 Go 那条路（parallel-sessions）；机制清点按需重生成。证据只记 `S`。


## 附录：逐口核实测原稿（F-6.md）

### POD 更正口没有重放与冲突代数：原样重投每次再签一版、再交一份意图，迟到的旧更正把当前 POD 改回去，读旧当前版时版本链分叉

namespace: operator-channel
faces: /transport-fulfillment/delivery-proof-corrections
地盘:
- internal/transportfulfillment/application/register_effective_delivery.go 及其测试
- internal/transportfulfillment/adapters/postgres/effective_delivery.go 及其测试
- internal/transportfulfillment/adapters/http/work_fact_intake.go（`DeliveryProofCorrectionPayload`）及其测试
- internal/transportfulfillment/adapters/http/register_effective_delivery.go（`deliveryResponse`）及其测试
- internal/transportfulfillment/domain/payload_canonicalization.go（新增 `CORRECT_DELIVERY_PROOF` 形状）及其测试
- cmd/parcel-api/（新增真渠道真库回归测试文件）

Category: bug
Status: draft
Blocked by:
- operator-channel/30 进 main（`AtRegistryPrecision`；同改 `payload_canonicalization.go`）
- operator-channel/32 进 main（同改 `work_fact_intake.go`、`register_effective_delivery.go`）
- 〈移动事实冲突格〉进 main（同改 `payload_canonicalization.go`）
- 线格式加 `predecessorVersion`，待 TF owner（= 用户）裁

父票：[15](./15-operation-decision-faces-take-operator-intake.md)；票 10 换口的那一笔。

出处：逐口代数核对中，POD 更正口的探针与对抗核验，钉 `c68e4342`，证据层级 `S`。本口在 `ec00b795` 由 `tfhttp.UnconfiguredIntake{}` 换上真渠道。票 13「未定形状」列着 `CorrectDeliveryProof`，票 10 完成记录里没有逐口代数。

地盘：见本票 territory。

#### 现象
1. 串行原样重投：同一份更正连投，每次都答 201 `DELIVERY_CORRECTED`，版本 `DRV-…2` → `…3` → `…4` 逐次回指上一版。交付行、交付意图、版本序列各加一，每份意图的事件标识都带新版本，`EnqueueOnce` 拦不住。
2. 与原请求重叠的重投：原请求停在提交前时，第二份答 200 `SOURCE_NOT_ACCEPTED`，同时耗掉一个版本号。
3. 读旧当前版：第二份更正在第一份提交前读当前版、在其后写入，形成 `DRV-3 corrects DRV-1`。`DRV-2` 被翻成历史且没有后继，两版同时指向同一前版，意图多出一封。同内容、异内容两种情形都实测过。
4. 续办承诺落空：意图交付失败一次后按续办原样重投，答复另签 `DRV-3`。在队意图只有 `DRV-1` 与 `DRV-3`，`DRV-2` 的意图永远交不出去。结果头注「DeliveryHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份」对本口不成立。
5. 改内容与乱序：
   - 同一更正时刻换 POD，或同一 POD 换更正时刻，都答 201 新版本，没有冲突格。
   - 迟到的旧更正把当前 POD 改回旧值。
   - 更正时刻早于当前版更正时刻的那一份也成了当前版。
6. 经 `assembleBusinessEndpoints` 装配好的端点表，加真库操作者册与设备册走，结果同上；只剩令牌核验是替身。
7. 首登口在多次更正之后原样重放，答 `EXISTING_VERSION`，带回当前版：这是首登口自己的代数，不能替更正口作数。

读码（未实测）：PS 的 `FormParcelFinalHandler.Handle` 对同源新版本走重派生，每一份多余的版本都会在下游另起一次派生。

#### 复现（S）
命令：在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`

探针：
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-proof-corrections/zz_probe_v2_test.go 的 `TestProbeDecide2PODCorrectionV2`（overlay-v2.json）。
- 同目录 zz_probe_race_test.go 的 `TestProbeDecide2PODCorrectionRace`（overlay-race.json）。
- 同目录 zz_adv_fork_test.go 的 `TestAdvPODCorrectionStaleReadFork`。
- 同目录 zz_adv_handoff_test.go 的 `TestAdvPODCorrectionHandoffContinuationReplay`。
- 同目录 zz_adv_router_test.go 的 `TestAdvPODCorrectionThroughTheAssembledRouterWithRealRegisters`。

后三条合挂在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-proof-corrections/synth/overlay-adv.json，日志为 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-proof-corrections/synth/run-adv.log。

#### 应当
- 同一身份重复到达返回原结果，不追加第二条事实（ADR-0023「服务端据以判重：同一身份重复到达返回原结果，不追加第二条事实」）。
- 不按接收顺序定序（ADR-0023「服务端按二者定序，不按接收顺序」）；不按最后消息覆盖（TF CONTEXT「揽收、派送与交付」）。
- 同身份异内容给冲突格，指向可续办的去处：对当前版提更正。
- 版本链线性，每份已提交版本的意图都交得出去。

#### 根因
- `CorrectDeliveryProofCommand` 只有 TenantID、Attempt、Object、NewProof、CorrectedAt：没有请求键，没有指名的前版，也没有设备签发的更正身份。
- `RegisterEffectiveDeliveryHandler.Correct` 每次都依次走 `NextDeliveryResultVersion` → `CorrectProof` → `Supersede` → `handOff`，中间没有任何比对。
- `EffectiveDeliveries.Supersede` 只按 `is_current` 翻旧行，不看前版是谁。
- 版本号在领域门之前就已签发。
- 新记录的 `ContentDigest` 沿用首登摘要，不能拿来比更正。

#### 修复方向（机制，同族先例 `RegisterOffsitePickupHandler.Correct`，票 tf-segment-lifecycle-closure/08 裁决 A）
1. 线格式与命令：`DeliveryProofCorrectionPayload` 与 `CorrectDeliveryProofCommand` 加必填的 `predecessorVersion`。设备从首登或上一次更正答复里的 `deliveryVersion` 取它。这一步归 TF owner 裁。
2. 编排 `Correct`：认重放放在签号之前，重放不耗版本号。
   - 当前版就是指名的前版：照现路径形成新更正。
   - 当前版不是指名的前版：用新加的 TFC-1 形状 `CanonicalizeDeliveryProofCorrectionPayload`（face 取 `CORRECT_DELIVERY_PROOF`，内容为 predecessor_version、new_proof、经 `AtRegistryPrecision` 的 corrected_at）两侧都从在册当前版本体现算，不加摘要列、不动迁移。同串答 `EXISTING_VERSION`，回显当前版并重发同一版本的意图（事件标识不变，`EnqueueOnce` 自然去重）；异串答 `SOURCE_CONFLICT`。比对锚在当前版，同揽收更正（「要改请对当前版提更正」）。
   - 领域门（更正早于交付）挪到签号之前。
3. 仓储 `Supersede`：翻旧行的 UPDATE 加上「`delivery_version` = 指名前版」的条件。并发输家照揽收更正处理 `OffsitePickupAlreadyRegistered` 的先例，读回赢家后走第 2 步的比对。
4. 答复 `deliveryResponse`：加一格回显在册 `correctedAt`，照票 20 关段回显 `closedAt` 的先例，时刻格式随票 31。

备选，不推荐：照 ADR-0183「复核记录二」(b)，不加键，当前版内容相同即答已有结果。不改线格式，但挡不住迟到的旧更正改回，只适合线格式裁定之前作过渡。

#### 是否潜伏
已可触发。
- 生产装配上 `deliveryCorrectionIntake = workFactTransport` 恒挂真渠道。`buildDeliveryOrchestration` 头注：「本笔没有「显式未配置」缝」。作业事实铸造不带准入要求。
- 只差租户取值：`WORK_FACT_REGISTRATION` 授予（`OFFSITE`）与在用设备，`parcel-access-register` 可登。演示种子今天没有这两样，答 `OPERATOR_NOT_GRANTED`。

#### 完成判据
- [ ] 编排单测：
  - 串行原样重投答 `EXISTING_VERSION`，不签号、不落库；
  - 同前版异内容答 `SOURCE_CONFLICT`；
  - 指名前版已不是当前版时与当前版比；
  - 更正早于交付答 `SOURCE_NOT_ACCEPTED` 且不签号。
- [ ] 领域单测：`CORRECT_DELIVERY_PROOF` 形状往返，并入 `TestCommandPayloadsRoundTripAsTFC1`；纳秒与对应微秒判等。
- [ ] 真渠道真库回归（`cmd/parcel-api` 新测试文件，`WorkFactCommandIntake` + `buildDeliveryOrchestration`）：
  - 原样重投：版本序列、交付行、交付意图都不变；
  - 迟到旧更正答 `SOURCE_CONFLICT`，当前 POD 不变；
  - 意图交付失败后重投，重发原版本的意图，不另签。
- [ ] 并发真库回归，照 `TestAdvPODCorrectionStaleReadFork` 与 `TestProbeDecide2PODCorrectionRace` 的钩子：
  - 读旧当前版的那一份不形成分叉，答冲突或已有版本；
  - 每个版本至多被回指一次；
  - 被翻成历史却无后继的版本为零。
- [ ] 首登口既有用例照过，首登摘要不动。
- [ ] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./internal/parcelshipment/... ./cmd/parcel-api/... ./internal/architecture/...`，全部 ok。
- [ ] 新增生产文件时，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验，非作者评审（两轴分开），推送方在干净检出上带 DSN 跑全量。


## 附录：逐口核实测原稿（F-7.md）

### 交接更正口同一 newVersion 换内容仍答「已有版本」，交接首登并发输家同版本异内容也答「已有版本」：TFC-1 摘要算了、存了，撞键后从不比

namespace: operator-channel
faces: /transport-fulfillment/handover-corrections, /transport-fulfillment/handovers
地盘:
- internal/transportfulfillment/application/register_transport_handover.go 及其测试
- internal/transportfulfillment/application/unversioned_payload_digest.go
- internal/transportfulfillment/application/unversioned_payload_digest_test.go
- cmd/parcel-api/（新增真渠道真库回归测试文件）
- .scratch/operator-channel/issues/13-command-payload-canonicalization-per-face.md（只加一行更正）

Category: bug
Status: draft
Blocked by: 无
父票：[15](./15-operation-decision-faces-take-operator-intake.md)、[13](./13-command-payload-canonicalization-per-face.md)
出处：逐口代数核对交接更正口的探针、对抗核验与候选修复试算，钉 `c68e4342`，证据层级 `S`。
- 票 13「已定形」写着 TFC-1 覆盖「交接首登与更正」；同票阻断修复记录却写「交接更正口只走版本键、不比摘要，未动」，两处自相矛盾。
- `TestRecordsStoredBeforeTFC1AreComparedUnderTheirOwnShape` 的头注自承「交接更正口不比摘要（重放与撞键都走版本键），不在此列」。
- 本口自 `769372cb` 起只存摘要、不比摘要；`2e477cb4` 只把摘要换成了 TFC-1；`ec00b795` 把它从 `tfhttp.UnconfiguredIntake{}` 换上了真渠道。
地盘：见本票 territory。

#### 现象
- 更正：先首登 v1，再更正 v1→v2（内容 A）。之后在同一 newVersion v2 上投下列内容，一律答 200 `EXISTING_VERSION`，与原样重放的答复逐字相同，在册 v2 仍是内容 A：
  - 换放行证据；
  - 换规则；
  - 换更正时刻；
  - 改判拒收（带依据）；
  - 以别的版本作前版；
  - newVersion 撞上同对象同范围另一份首登的版本号——此时回显的是那份首登。
  回显不含证据、规则、依据、更正时刻，调用方比不出差异。经路由层、生产 `buildWorkFactIntakes`、真库操作者册与设备册走，结果相同。
- 首登：同版本、异放行证据的两份首登并发，各对答复都是 201 `HANDOVER_REGISTERED` 加 200 `EXISTING_VERSION`，输家不答 `SOURCE_CONFLICT`。
- 对照：首登口串行投同版本异内容，答 `SOURCE_CONFLICT`；首登口投一个已被更正占用的版本号，也答 `SOURCE_CONFLICT`。

#### 复现（S）
命令：在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`。
- 原探针：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/handover-corrections/zz_probe_v2_test.go 的 `TestProbeDecide2HandoverCorrectionV2`、`TestProbeDecide2HandoverCorrectionThroughRealRegistries`（overlay_v2.json）。
- 对抗探针：同目录 zz_probe_v3_adversarial_test.go 的 `TestAdvHCThroughRouterAndReverseCollision`、`TestAdvHCConcurrentSameIdentityDifferentContent`（overlay_v3.json；日志 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/handover-corrections/synth/run-v3.log）。
- 候选修复试算：用 overlay 把 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/handover-corrections/verify-amend/register_transport_handover.go 与同目录 unversioned_payload_digest.go 换进 application 包（verify-amend/overlay_fix_and_probe.json、overlay_fix_only.json）。结果：上列各变体都转成 `SOURCE_CONFLICT`，原样重投仍答 `EXISTING_VERSION`，application 包既有用例照过。日志在同目录 synth/run-fix-and-probe.log、synth/run-fix-only-app.log。

#### 应当
同键同内容是重放，同键异内容是冲突。依据：
- ADR-0149 决定四：「摘要进信封，作同身份重放与内容冲突的判据」；
- 本处理器首登口的注释：「同一判断版本携带不同裁决或证据：冲突保留原判断——改判走更正入口换新版，不按最后到达顶替」；
- UC-TF-005：「重复返回原结果；冲突保留并待复核」。

票 20 那条「现有格够用」判据在这里不成立：本口有演进写口，可以从链尾再更正；冲突格也有可续办的去处。

#### 根因
- `RegisterTransportHandoverHandler.Correct` 算出 TFC-1 摘要后直接调 `commit`，没有先按新版本键去读。
- `commit` 里 `case ports.HandoverAlreadyRegistered:` 读回赢家后直接走 `existingResult`，首登与更正两路都不比摘要。

#### 修复方向（机制，只改 application 层；domain 形状、HTTP、迁移都不动）
1. `Correct`：领域 `Correct` 成功、摘要算出之后，在 `commit` 之前先按新版本键调 `Handovers.FindByKey`。在册时用 `domain.CompareStoredDigest(existing.ContentDigest, correctionDigest, unversionedHandoverCorrectionDigest(command))` 分三格：
   - `SamePayload` → `existingResult`；
   - `DifferentPayload` → `HandoverRegistrationConflict`；
   - `UnknownPayloadShape` → `handoverRegistryUndecided`。
   跨口撞键时 face 不同，自然落进冲突格。
2. `commit` 的 `HandoverAlreadyRegistered` 分支读回赢家后，同样分这三格。`commit` 为此多收一个本次命令的无版本摘要参数。首登口的竞态窗口一并收住。
3. 在 `unversioned_payload_digest.go` 补 `unversionedHandoverCorrectionDigest`，函数体从基 `1121ba61` 的 `correctionDigest` 原样恢复（ADR-0014）。TFC-1 之前入库的更正行存的就是这个算法的摘要，不回写、不迁移。
4. 删掉 `TestRecordsStoredBeforeTFC1AreComparedUnderTheirOwnShape` 头注里「交接更正口不比摘要…不在此列」那一句；在 `unversionedDigestCases` 加 `CORRECT_TRANSPORT_HANDOVER` 一格，定值取基上旧代码的输出。
5. 在票 13 末尾追加一行更正，说明阻断修复记录的 TF 一行已由本票收口（只加行）。

#### 是否潜伏
已可触发。生产上这一行挂 `WorkFactCommandIntake`；`buildControlFactOrchestrations` 头注写「本笔没有「显式未配置」缝」；没有授权映射，铸造也不判准入。只差租户登记作业事实授予与设备，探针已用真库操作者册与设备册实证。

#### 不做
同一前版被更正成两个新版本（链不线性），归〈交接更正链线性〉。

#### 完成判据
- [ ] 编排单测：
  - 原样重放答 `EXISTING_VERSION`；
  - 异证据、异规则、异更正时刻、改判、换前版、跨口撞键，都答 `SOURCE_CONFLICT`；
  - 并发输家（替身让首读查无、`Save` 答已登记）读回后比摘要。
- [ ] 兼容旧摘要单测：`unversionedDigestCases` 加 `CORRECT_TRANSPORT_HANDOVER` 一格，定值取基上旧代码的输出；新记录断言带 TFC-1 前缀。
- [ ] 真渠道真库回归（`cmd/parcel-api` 新测试文件，`buildWorkFactIntakes` + 真库操作者册与设备册 + `buildControlFactOrchestrations`）：
  - 更正口：重投答 200 `EXISTING_VERSION`；同 v2 换内容答 200 `SOURCE_CONFLICT`，在册 v2 不变；
  - 首登口：同版本异内容两份并发，输家答 `SOURCE_CONFLICT`，用例带多轮。
- [ ] 变异自检：撤掉比对，上述用例转红。
- [ ] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...`，全部 ok。
- [ ] 票 13 更正行、票 15 补记回写行都已追加（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验，非作者评审（两轴分开），推送方在干净检出上带 DSN 跑全量。
