# 09 派送发起的确定性任务引用不是忠实身份：键可被手工建任务口占用、拼接不单射，对象首拍就答「已形成」而它并没有派送任务

Category: bug
Status: needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-delivery-dispatch-triggers, /transport-fulfillment-dispatch-task-registrations
地盘：
- internal/transportfulfillment/application/trigger_delivery_dispatch.go 及其测试
- internal/transportfulfillment/adapters/http/trigger_delivery_dispatch.go 及其测试
- cmd/parcel-api/assemble_delivery_dispatch.go（执行器先查在册时接任务登记册读口；同笔清本文件过期头注）
- cmd/parcel-api/（新增真渠道真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象
- **手工口占用。** 先经运营决定渠道的手工建任务口，用执行器的确定性引用建一项任务（kind=PICKUP，对象是别的）。该对象首拍即答 200 `DISPATCH_TASK_ALREADY_FORMED`，回显别人的种类与对象集。
- **拼接撞键。** 段 `SYN-SEG/A` 加对象 `CU-77`，与段 `SYN-SEG` 加对象 `A/CU-77`（交接版本同为 v1），铸出同一引用，后到的对象首拍即答「已形成」。
- **核验补测一：对象取生产形状也能撞。** 对象取不含斜杠的生产形状（`DPCL-` 开头）时，只靠精心构造的段引用与交接版本也能撞键。
- **核验补测二：核种类与对象集不够。** 同一对象换到第二个派送段，段与交接版本经构造后撞上第一段那项任务；这时在册任务的种类与对象集都对得上，所以只核这两样拦不住。
- **入场依据不变就形成不了。** 上述被撞的对象，在入场依据不变期间一直形成不了派送任务；经交接更正换了入场依据后可以脱出（核验补测）。原研判写的「永远」应修正为「入场依据不变期间」。
- **生产缝下的可达条件。** 地点、时间窗、条件三个所有者都给出答复，才走得到撞键；空库上答 `REQUIREMENT_MISSING`。
- **在册之后仍重拉所有者。** 在册要等重拉所有者之后才认。所有者改答或参与离场后，原样重放分别答 `REQUIREMENT_MISSING`、`DISPATCH_UNDECIDED`、`INPUT_NOT_ACCEPTED` 或 `NOT_A_DELIVERY_TRIGGER`，都不回显在册任务；每次重放还要白读一遍所有者。

## 复现（S）
在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行：
`IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`

- 原探针：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-dispatch-triggers/probe_ddt_test.go 的 `TestProbeDeliveryDispatchTriggerKeyTakenByTheManualPortAgainstARealDatabase`、`TestProbeDeliveryDispatchTriggerKeyConcatenationIsNotInjective`、`TestProbeDeliveryDispatchTriggerReplayAlgebraAgainstARealDatabase`（同目录 overlay.json）。
- 对抗与可达核验：同目录 adv_probe_ddt_test.go 的 `TestAdvDDT…`，verify/verify_ddt_test.go 的 `TestVerifyJUnderProductionSeams`、`TestVerifyCollisionWithProductionShapedObjectIDs`、`TestVerifySameObjectCollisionAcrossSegmentsPassesKindAndObjectCheck`。合挂 overlay 为 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-dispatch-triggers/synth/overlay-all.json，日志为 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/delivery-dispatch-triggers/synth/run-adv-verify.log。

## 应当
- 「已在册」认的是本拍要的那一项任务。答复不能对一个没有派送任务的对象声称「已形成」。
- 同一拍重跑，不随所有者的后续改答漂移，也不白读所有者。

## 根因
- `deliveryDispatchTaskReference` 把段、对象、入场依据以斜杠直拼，即「"DELIVERY-DISPATCH/" + 段 + "/" + 对象 + "/" + 入场依据」。三者都是只拒空白的不透明串，交接版本串跨对象还可以相同。
- 手工口 `dispatchTaskSpecFrom` 只要求引用非空，能指名任意引用。
- `TriggerDeliveryDispatchHandler.Trigger` 对 `DispatchTaskAlreadyRegistered` 不看记录内容。
- `pullRequirements` 排在 `Dispatch.Open` 之前。

## 修复方向（机制）
1. 引用改成单射铸法：逐段转义、加长度前缀，或用带形状版本的规范化摘要，由实施方定。
   - 必须兼认旧拼法下已在册的任务：先查新键，查不到再按旧拼法查；命中且下面第 2 条核得过，就算已在册。
   - 存量不回写，与 ADR-0014、ADR-0150 守同一纪律。
2. 在册任务核对作第二道防线：种类须为 `DeliveryDispatch`，对象集须恰为该对象。对不上时答新格（例如「任务引用已被另一项任务占用」，200，带回显），不写、不另铸、不发意图。单靠这一条拦不住同对象跨段撞键，所以第 1 条是主修。
3. 先查在册再取件：定出键、读回段并确认链尾凭已交接之后，先按键查在册，在册就不再向所有者取件。查在册放在 Active 判断之前还是之后，决定参与离场后的重拍答已在册还是 `PARTICIPATION_NOT_ACTIVE`，归 TF owner；ADR-0114 决定二「已离场的参与不触发」两种放法都守得住。
4. 可选，归 TF owner：手工建任务口拒执行器的保留前缀。
5. 同笔清 `buildDeliveryDispatchTrigger` 头注里已过期的「端点挂 UnconfiguredIntake{} 起步」。

## 不做（观察）
- 任务登记册读不动时，`DISPATCH_TASK_UNDECIDED` 的续办引用跨租户相同（`TestAdvDDTDispatchTaskUndecidedContinuationOmitsTheTenant`），出自 `OpenDispatchTaskHandler.Open` 的未决摘要。要不要进租户，归 TF owner。
- 入场依据进键（票 tf-segment-lifecycle-closure/09 越权风险点）：交接更正后会另立任务、前一项不动，按 ADR-0114 字面是新身份。

## 是否潜伏
已可触发，但属边角。生产装配上 `deliveryDispatchTriggerIntake = workFactTransport`，三条缝都接真，没有授权映射，作业事实铸造也不判准入。触发还需要两样之一：手工口占了那串引用；或者段引用、交接版本里带斜杠，同时三个所有者都给出答复。

## 完成判据
- [ ] 编排单测：
  - 新铸法单射，对拼接撞键那组输入铸出不同引用；
  - 旧拼法在册任务兼认；
  - 被手工口占用答新格；
  - 在册后不再取件（所有者替身计数为零）。
- [ ] 真渠道真库回归（`cmd/parcel-api` 新测试文件，作业事实 Intake + 生产 `buildDeliveryDispatchTrigger` 或三缝替身 + 运营决定渠道手工口）。下列各组各成其格：
  - 手工口占用：答新格并回显占用者；
  - 拼接撞键：两个对象各自形成任务；
  - 生产形状对象撞键：两个对象各自形成任务；
  - 同对象跨段：在第二段形成自己的任务；
  - 所有者改答后原样重放：答已在册并回显。
- [ ] 并发首拍真库回归照过：一笔 201，另一笔读回赢家答已在册。
- [ ] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...`，全部 ok。
- [ ] 新增生产文件时，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验；非作者评审，两轴分开；推送方在干净检出上带 DSN 跑全量。
