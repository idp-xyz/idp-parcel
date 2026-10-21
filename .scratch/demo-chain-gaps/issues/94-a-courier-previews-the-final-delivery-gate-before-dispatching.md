# 94 派送出发之前问一次交付门禁

Category: enhancement
Status: in-progress——评审两条阻断已修，待复审与进 main，分支 claude-dcg146（2026-10-21，见文末「评审记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg146（2026-10-21 Claude Code 作者子代理，变基到 main `b290b777`；见文末「完成记录」）。此前 in-progress——2026-10-21 认领（146、94 同批），分支 claude-dcg146。此前 ready-for-agent——2026-10-20 代决复核升为上线前必修（R14），见 [owner-review-20261020](../../owner-review-20261020/report.md)。此前 needs-triage——2026-10-19 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [66](./66-effective-delivery-records-a-final-delivery-gate-reading.md) 的完成记录。
Blocked by: [66](./66-effective-delivery-records-a-final-delivery-gate-reading.md)（读数内核与 `DeliveryGateReader` 随它进 main）
地盘：TF 新 `application/preview_delivery_gate.go`（复用 `DeliveryGateReader` 取 at 的那一步）、新 tfhttp 问询口；新 `cmd/parcel-api/assemble_delivery_gate_preview.go` + `endpoints.go` 一行（装配令牌）；文档 UC-TF-006 AT、ADR（号认领时由协调方给）。
出处：[ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末「补记：票 66 落决定五时的补充裁定」越权风险点 20；先例 [ADR-0219](../../../docs/adr/0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md)（设备出发前门禁问询）。

## 现象

票 66 只在有效交付登记时问交付门禁、照记读数。读数是交付之后的记录：读到阻断时包裹已经在收件方手里。TF CONTEXT 写的是「挡下违规交付是派送出发之前那一问的事」，而派送这一侧今天没有那一问：派送任务形成、派送尝试出发都不问关务，尾程设备也没有读口。出发那一侧由 ADR-0219 补上了出发前问询，交付这一侧没有对应的口。

## 三类

- 机制：作业事实渠道开只读问询，按（对象，拟交付时点）交回与随后同参交付登记同一套读数（at 的取法同 ADR-0213 决定五与票 66 补记）；TF 不落记录，CC 照 ADR-0206 决定六记判断。授予口径照 ADR-0219（能登有效交付的就能问），不新增能力种类。
- 产品策略：问询挂在设备出发前，还是派送任务形成那一拍也问一次并把读数挂在任务上；参考做法是前者，与 ADR-0219 同形。
- 租户取值：无。

## 完成判据

- 尾程设备在派送出发之前按对象与拟交付时点问一次，答与随后同参交付登记逐字节相同的读数；阻断照答 200，挡不挡归设备与现场。
- 问询不落 TF 记录；之后的交付登记照旧按它自己那一刻重问。
- 真库装配用例；证据记 `S`。

## 规模

M

## 代决复核追加（2026-10-20）

出处：[owner-review-20261020](../../owner-review-20261020/report.md) 的 R14。代决「是」：列为上线前必修。不做则交付侧关务门禁只能事后记账，受限或被扣的包裹照样交到收件人手里。照 ADR-0219 同形，排在票 42 之后。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 `claude-dcg146`，与票 146 同批，变基到 main `b290b777`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」与 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」。裁定的单一权威是代决复核报告 R14（回给设备那一格照 R11 / ADR-0237）；落地时补的裁定写进 [ADR-0238](../../../docs/adr/0238-a-courier-previews-the-delivery-gate-before-dispatching-and-reads-what-the-delivery-will-record.md)，那里是唯一权威，本节只列去处。

### 做了什么

- **编排**：
  - `application/read_delivery_gate.go`：读数路径收成一条 `readIntent`（取 at、停点、问关务），有效交付登记的 `ReadRegistration` 读出派送结果的发生时刻后走它；问询的受理收成 `deliveryGateIntentOf`（租户、对象、时点），对象判据与交付登记的键共用新抽出的 `deliveredObjectFrom`（`register_effective_delivery.go`）。
  - 新 `application/preview_delivery_gate.go`：结果代数（`DELIVERY_GATE_PREVIEWED` / `INPUT_NOT_ACCEPTED`）与 `PreviewDeliveryGateHandler`，只依赖交付登记那只 `DeliveryGateReader`，结构上没有写口。问询不读派送尝试。
- **线格式**：新 `adapters/http/preview_delivery_gate.go`：`POST /transport-fulfillment/delivery-gate-previews`，载荷只收 `object` 与 `deliversAt`（封闭解码），答复一律 200，`gateReading` 与有效交付口同一个视图（逐对象那一格照 ADR-0237）；未配置答法同文件。`work_fact_intake.go` 加 `IntakeDeliveryGatePreview`（与有效交付口同一道 `tenantFrom`）。
- **装配**：照 ADR-0222 新建 `cmd/parcel-api/assemble_delivery_gate_preview.go`（读数编排取 `buildDeliveryGateReader`，与交付登记同一处构造）、`endpoints_delivery_gate_preview.go`（作业事实面 OFFSITE）与同名 `_test.go`（未接线占位与作业事实探测载荷）。票面「`endpoints.go` 一行」是 ADR-0222 之前的旧写法，没照它做；`endpoints.go`、`main.go`、`endpoints_test.go`、`assemble.go`、`routes.go` 都未动。
- **不动的**：端口、迁移、信封；关务代码。
- **文档**：
  - ADR-0238 新立；ADR-0213 的 Status 行与 Links、ADR-0206 的 Links 加前向指针，无条款停用。`docs/adr/README.md` 未手改。
  - TF CONTEXT「揽收、派送与交付」加一条；GLOSSARY 加「交付前门禁问询」词条、「动作准入判断」所有者一条补一句；CONTEXT-MAP `transport-fulfillment → customs-compliance` 补一句；UC-TF-006 规则一句，验收立 `AT-TF-322`–`325`。
  - 演示脚本补记一条；AT 盘点①补行与追记（只加行，总览未动）。
- **另立票**：[165](./165-customs-docs-name-the-delivery-gate-preview-as-a-request-timing.md)（CC CONTEXT「动作准入判断」词条与 UC-CC-013 的请求时机一句没补交付前问询；CC 地盘本票只读）。

### 代决

内容与理由只在 ADR-0238，这里只列编号与去处：

1. 问询挂在设备派送出发之前，不挂在派送任务形成那一拍（票面三类里的产品策略，取参考做法）：决定五。
2. 按件问，不按派送任务或一车批量问：决定一末条，越权风险点 5。
3. 拟交付时点对的是随后那次派送尝试里这件对象结果的发生时刻；缺席答未受理、不补服务端时钟：决定一。
4. 问询不读派送尝试，「派送尝试读面读不动」那一格只属登记：决定二。
5. 对象的受理判据与交付登记的键同一处：决定二。
6. 授予与有效交付口同一只作业事实 Intake，不新增能力种类：决定四。
7. CC 文档两处请求时机一句留给票 165，共用的 GLOSSARY 与 CONTEXT-MAP 本票补：Consequences「关务」，越权风险点 9。

### 越权风险点

编号与正文以 ADR-0238 为准，这里只列一行：

- 1：读到放行不等于交付那一刻仍放行（TF owner）。
- 2：用写授予开读口（TF owner、access-identity owner）。
- 3：问询不挡交付，客户端读到阻断怎么处置未定（TF owner、一线客户端 owner）。
- 4：问询在关务判断册上留行，没有限流与保留期（CC owner、运维）。
- 5：按件问，一车多少件就多少问（TF owner、一线客户端 owner）。
- 6：任一场外设备可零成本探知任一件是否被扣，不留痕（TF owner）。
- 7：不记谁问过（TF owner、VE owner）。
- 8：拟交付时点是预计、段按此刻取，同参条件很少严格成立（TF owner）。
- 9：CC 文档的请求时机一句没补，票 165（CC owner）。
- （评审修订补）4、5 指向票 172（判断册增长）；6、7 指向票 171（按作业范围或派送指派限对象并留痕）。

### 用例

`AT-TF-322`–`325` 全部入 AT 盘点①，半边覆盖在行内注明：

- 编排：`application/preview_delivery_gate_test.go`——关务五格、两种没形成答复、取不到 at 的各停点各一例，问询读数与同参交付登记读数 `reflect.DeepEqual`、问关务的问法逐问相同；零写入、不读派送尝试、读数不交给随后的交付（关务改答后交付读到改答那一格）；受理不了的三格（缺对象、对象立不起来、缺拟交付时点）不问任何一口；对象受理不了的那两格，同一件对象的交付同样受理不了。
- 线格式：`adapters/http/preview_delivery_gate_test.go`——阻断答 200、列出那一件、不带判断标识与依据；与把同一份读数登成交付时的 `gateReading` 逐字节相同；不受门禁时没有 `objects`；未受理、400、405、403 两种、未配置不读体、500。`work_fact_intake_test.go` 的逐行表加这一口。
- 生产装配真库：新 `cmd/parcel-api/delivery_gate_preview_test.go`——
  - `TestACourierAsksBeforeDispatchingAndTheDeliveryReadsTheSame`：新加坡枢纽交给尾程的两件，一件落了约束最终交付的限制。问询各答 200：受限那一件 `BLOCKED`、列出它，另一件 `NOT_GUARDED`、没有 `objects`，`at` 都是 `SYN-NODE-SIN-HUB`；两问之后整库指纹只有关务判断册变了。随后两件按问询的时点登有效交付，`gateReading` 与各自问询逐字节相同，关务判断册各仍一行（命中问询那一次的判断）。
  - `TestADeliveryPreviewWithoutTheGrantIsRefusedAndAsksNoOne`：作业范围上没有授予答 403 `OPERATOR_NOT_GRANTED`，整库不变。
- 装配表：新行自动进 `endpoints_test.go` 的各条登记处用例；把这一行的接入面改成登记写，`TestEachLineOpensOnlyOnItsDeclaredAccessFace` 当场变红（实测后复原）。

### 验证（实测，只记 `S`）

- `go build ./...`、`go vet ./...` 通过，`gofmt -l internal cmd migrations` 干净；代码提交在 `git archive` 出的干净目录里 `go build ./... && go vet ./...` 通过。
- `scripts/test-full.sh` 跑改动包、它们的反向依赖（含 `cmd` 三包）、`./internal/architecture/...`、`./migrations/...`，清单存 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/0d01979a-68c3-4b61-b295-1b04bc643b8b/scratchpad/dcg146/test-packages-final.txt`，变基到 `97841f2d` 后重跑全 ok；收口时再变基到 `b290b777`，相对 `97841f2d` 只多两笔 `.scratch` 文档提交，没有重跑（与票 146 同一跑）。
- `-v` 逐条确认两条新真库用例是 PASS 不是 SKIP。
- 没跑全量，没重生成机制清点、ADR 索引与 AT 计数；演示种子未改。

### 偏离与待裁

- **号**：ADR-0238；`AT-TF-322`–`325`；新立票 165（166 未用）；无迁移，暂定的 TF `0049` 没有用上。
- **演示没做一次性库实走**：取证是生产装配上的真库用例。
- **票面地盘之外**：`application/register_effective_delivery.go` 抽出 `deliveredObjectFrom`（受理同一处）、`ports/delivery_gate.go` 包头一句、`adapters/http/work_fact_intake.go` 加一个方法与 Intake 断言；共用文档 GLOSSARY、CONTEXT-MAP 各补一句。
- **待 owner 复核**：上列代决与越权风险点，尤其是代决 1（挂在设备出发前）、越权风险点 6（任一件是否被扣可零成本探知）与 8（同参条件很少严格成立）。
- **无被权限检查拦下的改动。**

## 评审记录（2026-10-21，隔离评审；作者子代理修订，分支 `claude-dcg146`，与票 146 同批，main 仍是 `b290b777`）

两条阻断都在票 146（见那张票面的「评审记录」）；本票只有顺手各条，全办。

- **3**：`application/preview_delivery_gate_test.go` 首条用例注释「问询不读派送尝试，见本文件末一条用例」已指错（末一条是 `TestADeliveryPreviewThatCannotBeAcceptedAsksNothing`），改用符号名 `TestADeliveryPreviewWritesNothingAndIsNotHandedToTheDelivery`（提交 `45fc6441`）。
- **4**：`adapters/http/work_fact_intake.go` 的 `IntakeDeliveryGatePreview` 注释「只收两格的 DeliveryGatePreviewPayload」数了别的文件的字段，改成「只收读数取决于的那几格」（同上）。
- **5**：`cmd/parcel-api/transport_fulfillment_command_lines_test.go` 补 `/transport-fulfillment/delivery-gate-previews` 一行，与出发前问询那一行对等：越过 Intake 答 200 `DELIVERY_GATE_PREVIEWED`（合成对象不在任何段里，读数停在本侧），自报 `tenantId` 答 400 `MALFORMED_REQUEST`、不带 outcome（同上；`-v` 确认子用例 PASS）。
- **6**：立票 [171](./171-gate-previews-are-scoped-to-the-asking-devices-work-and-leave-a-trace.md)——两种问询一起按作业范围或派送指派限对象，并在本上下文留痕（指向 ADR-0219 越权风险点 6、ADR-0237 越权风险点 2、ADR-0238 越权风险点 6、7）；ADR-0238 越权风险点 6、7 改写并指向它。
- **7**：立票 [172](./172-customs-judgement-register-grows-with-every-gate-preview.md)——关务判断册的保留期与限流（问询按件、时点按微秒入标识，恶意设备可无限增长）；ADR-0238 越权风险点 4、5 指向它（提交 `1962d6b1`）。
- **随票 146 的 B1 一并动到本票代码的**：交付读数取 at 时认进段交接的那一段收成共用的 `entryHandoverOf`（`application/entry_handover.go`），端口 `ports.DeliveryGateEntryHandoverView` 改名 `ports.EntryHandoverView`。交付读数的行为与停点不变，交付编排与线格式用例原样全过。
- **验证**：同票 146 评审记录「评审修订后的验证」，同一跑。
