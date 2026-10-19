# 67 实际履约发生项随交接与有效交付信封自动形成

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `9d1dff03`：153 ok / 0 FAIL；见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg67（2026-10-19 作者子代理，见文末「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: [25](./25-segment-adopts-transport-commission-and-cancellation-faces.md)、[28](./28-booking-and-cancellation-occurrences-and-procurement-context-from-adoption.md)
地盘：TF 新 `application/form_actual_fulfillment_on_participation_end.go`、新 inbox 消费者两只（交接、有效交付，inbox 名各自分家）；`cmd/parcel-dispatch/assemble.go`（交接 FanOut 与 `deliveryFan` 各加一路）+ 新装配文件 + 新真库用例；文档 ADR-0218（ADR-0207 Links 加后继指针、README 状态格注明部分停用）、UC-TF-005「应用流程」步 9 与 AT、演示脚本（干线成本那一步改为自动）。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」TF 自消费交接 / 有效交付信封；方案「复核」等 peer 各项现状，「代决」C15，「需要的 ADR」ADR-0218。

## 做什么

- 只对已采用委托的段，在参与结束时经既有登记用例形成实际履约发生项；采购上下文、旅程与各格答法照方案 C15。
- 演示约定的旅程拼写随之改为派生拼写。
- 文档：起草 ADR-0218。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 已采用委托的段参与结束：发生项 `REGISTERED`，与已形成信封同笔。
- 无采用连线：零写入，入账。
- 协议未声明收费发生条件：`未配置`，入账。
- 人工同身份重登：`EXISTING`。
- PC 读不动：登哨兵重投。
- 真库测试。
- 演示：干线段采用委托后在 SIN-HUB 交接 → 派发两拍 → BUY 请求 `FIRED`，无需人工登记发生项。

## 规模

M

## 完成记录（2026-10-19，作者子代理，分支 claude-dcg67）

基线 `8e5125c2`（origin/main，25、27、28、59 都已在内）。授权：用户原话「你作为业务和系统专家自决……解决所有的问题」；方案代决 C15 照 [handover-dispatch-plan](../handover-dispatch-plan.md) 落，方案没裁的细节按下文「代决」自决，统一写进 [ADR-0218](../../../docs/adr/0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)。

### 提交

- `7ecc0dc9`：领域（参与由哪件事实结束、自动形成的旅程、交付终点依据一处给出）与旅程册按成员读。
- `003a19c8`：自动形成编排、处置三格与两扇消费门（含 `transportHandoverKeyOf` 共用一处的小重构）。
- `cc6478de`：派发装配、交接与有效交付两张扇出各加一路、真库用例。
- `fd088f3b`：ADR-0218；ADR-0207、ADR-0209 的 Status 行与 Links 前向指针；`docs/adr/README.md` 0207、0209 行末追加部分停用，0217 行后追加 0218 行。
- `30fced43`：TF CONTEXT 成本来源一条；UC-TF-005 目标与边界、步 9、`AT-TF-246`–`255`；AT 覆盖盘点点名与追记。
- `60c6ff02`：立票 [88](./88-actual-fulfillment-not-formed-when-adoption-or-agreement-arrives-late.md)。
- `9105ab04`：演示动线第 3 步补记（一次性库实走）、种子 README 第 8 步一段。

### 落点

- 领域 `domain/actual_fulfillment_formation.go`（新）：`ActualFulfillmentJourney`（参与入场时已开始、开始最晚的那一趟替代 / 退运旅程，否则派生 `JRN/<对象>`；同刻两趟 `ErrAmbiguousJourney`）、`EffectiveDeliveryEndBasis`、`ActualFulfillmentSegment.ParticipationEndedBy`。`EndParticipationWithDelivery` 改用 `EffectiveDeliveryEndBasis`，结束参与与事后找参与是同一出处。前缀常量 `derivedJourneyPrefix = "JRN"` 进前缀门禁。
- 端口 `ports/alternate_journey_member_view.go`（新）；postgres `AlternateJourneys.FindByMember`（`members @> jsonb_build_array($2)`，按整串认成员），`FindByKey` 的重建抽成共用函数。无迁移。
- 应用 `application/form_actual_fulfillment_on_participation_end.go`（新）：`OnHandover` / `OnDelivery` 两个入口共用 `formFor`；结果代数、`Disposition`、`ActualFulfillmentFormationRegistrant`、`ErrParticipationEndedInSeveralSegments`。登记仍交 `RegisterChargeOccurrenceHandler`，本编排只答「结束了哪一条参与、段采没采用委托、旅程是哪一趟」。
- 消费门 `adapters/inbox/actual_fulfillment_formation_consumers.go`（新）：两个消费者名 `transport-fulfillment/form-actual-fulfillment-occurrence-from-transport-handover`、`…-from-effective-delivery`，共用一对哨兵 `ErrActualFulfillmentFormationUndecided` / `ErrActualFulfillmentFormationDefect`；交付信封译码要版本（缺了是毒丸）；`EffectiveDeliveryRegisteredEventType` 在本包写出，与 PS、VE 两路相等由真库用例钉。
- 派发 `cmd/parcel-dispatch/assemble_actual_fulfillment_on_participation_end.go`（新）：两份未决哨兵名单；`actualFulfillmentFormerOver` 是协议核对口以外唯一的接线处，登记用例照 parcel-api 的 `chargeOccurrenceDeps` 另接一份。`assemble.go` 只在两张扇出前各追加一路、FanOut 参数末尾各加一个，路由表头注两行补一句、「各投两个」改「各投各的」，没重排既有行。
- 不加迁移、不加信封、不改登记用例与已形成信封；未用 TF 迁移号 0042，未用新立票号 89。

### 代决与越权风险点

方案 C15 之外，本票自决（都写进 ADR-0218）：

1. **找参与按终点依据，不按「此刻在场」或「最近一段」**：在对象有过参与的各段里找链尾（非失效）终点种类与依据恰是这件事实的那一条；零条入账，多条上抛。交付那一路的依据取信封所指那一版结果，不读交付登记册的当前版。
   - 越权风险点：交接更正撤回控制之后已形成的项不回退（ADR-0218 越权风险点 2，去处是发生项修订入口）。
2. **没有采用连线时不交登记用例**（方案只说「不形成、不推断自营」）：交过去登记用例会问承运商判断、可能答不适用，那就是推断。
3. **成员只有这一个对象**，按段计的协议由覆盖一问让第一件盖住整段。
   - 越权风险点：段费那一项的成员名单只有第一件（ADR-0218 越权风险点 5）。
4. **旅程按参与入场时刻判**（方案原话「当前旅程」）：按处理此刻判，信封晚到会把原段成本挂到新旅程上。同刻两趟响亮失败。
   - 越权风险点：对象是集运单元时派生拼写指的是单元；替代旅程册今天没有生产写口，生产上一律是派生拼写（ADR-0218 越权风险点 4）。
5. **登记者记本上下文的系统身份** `idp-parcel/transport-fulfillment#form-actual-fulfillment-on-participation-end`。这一句与 ADR-0209 决定五「登记者取自操作者信封的主体」相抵，按 README 规则记成 ADR-0209 部分停用（限自动形成那一路），派单只点名了 ADR-0207，这是本票多出来的一处。
   - 越权风险点：审计在这些项上看到的是系统；人工后登回显系统身份（ADR-0218 越权风险点 3）。
6. **ADR-0207 决定四末句的演示约定一并记成停用**：派单只点名决定七后半句与风险点 3；演示拼写改了，决定四末句那句租户取值随之失效，照实标出。
7. **处置分格**：已登记、已存在、冲突（采购上下文冲突除外）、协议未生效与已终止、未配置、不计费、缺采购上下文入账；登记用例的未决与本编排读不动重投；不适用、其余未受理、协议不在册、供应商不符、采购上下文冲突、集外格、上抛响亮失败（后三格本路走不到，评审后改判，见「评审记录」）。缺采购上下文入账的理由：只出现在迁移 0038 之前的委托上，重投报不出协议版本。
8. **自动形成没赶上的参与不补**（采用晚于参与结束、协议后补条件）：另立票 88（ADR-0218 越权风险点 1、6）。

ADR-0218 的 Status 写明待 TF owner 复核，第 2、3 点同时待 SA owner，第 5 点同时待 PC owner。

### 各判据对应用例

- 已采用委托的段参与结束：发生项 `REGISTERED`，与已形成信封同笔：
  - 应用 `TestAHandoverEndingAnAdoptedSegmentsParticipationFormsTheOccurrence`、`TestAnEffectiveDeliveryEndingAnAdoptedSegmentsParticipationFormsTheOccurrence`；
  - 派发真库 `TestAHandoverAtTheEndOfAnAdoptedTrunkSegmentFormsTheOccurrenceAndFiresBuy`（生产路由表：交接信封 `PUBLISHED`、本路一行 inbox、发生项一项、再一拍 BUY 请求恰一份）、`TestAnEffectiveDeliveryEndingAnAdoptedSegmentsParticipationFormsTheOccurrence`。
- 无采用连线：零写入，入账：应用 `TestASegmentWithoutAnAdoptionFormsNothingAndInfersNoSelfOperation`（承运商判断是自营法人也不交登记用例）；真库 `TestAnUnadoptedSegmentSettlesWithoutAnOccurrence`。
- 协议未声明收费发生条件：`未配置`，入账：应用 `TestAnAgreementThatDoesNotChargeOrDeclareActualFulfillmentSettlesWithoutAnOccurrence`（另含不计费）。
- 人工同身份重登：`EXISTING`：应用 `TestManualAndAutomaticRegistrationOfTheSameIdentityAnswerExisting`（先自动后人工、先人工后自动、人工写别的旅程答冲突）；真库那一条的末段（先自动后人工）。
- PC 读不动：登哨兵重投：应用 `TestUnreadableDependenciesAreRetriedWithoutWriting`；真库 `TestAnUnreadableAgreementCheckIsRetriedAndLeavesNothingBehind`（`dispatch.ErrConsumerUndecided` 带本路哨兵、inbox 0 行、发生项 0 项，恢复后重投形成）。
- 其余格：`TestAFactThatEndsNoParticipationSettlesWithoutRegistration`、`TestTheOccurrenceJoinsTheAlternateJourneyStartedBeforeTheParticipation`、`TestRegistrationBusinessAnswersSettle`、`TestFormationDefectsAreNeitherSettledNorRetried`；领域 `actual_fulfillment_formation_test.go` 三个；tfinbox 两扇门各一个真库用例；postgres `TestAlternateJourneysAreFoundByMember`。AT 对照见 AT 覆盖盘点 `AT-TF-246`–`255` 各行。

### 验证

- `go build ./...` 退 0；`go vet ./cmd/... ./internal/transportfulfillment/... ./internal/architecture/...` 退 0；`gofmt -l cmd internal scripts` 无输出。
- 带 DSN `go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./cmd/parcel-dispatch/`、`./internal/architecture/`、`./scripts/demo-seeds/...` 全 ok。代码定在 `cc6478de`，此后只改了一处用例 Covers 注释与文档。
- 没跑全量，没重生成机制清点（按派单，进 main 时统一做）。

### 演示实走（证据 `S`）

一次性库 `dcg67_demo`，`seed.sh` 原样灌入退 0；`parcel-api`（操作者与集成客户端两族）与 `parcel-dispatch`（七个变量照动线「前置」）都用本分支构建。

1. 干线委托 `SYN-COMMISSION-TRUNK-DCG67`（`v3`，成员 `SYN-PARCEL-DCG67-01`）答 `201 COMMISSION_SUBMITTED`。
2. 华东场外设备在 SZX 交接，立段 `SYN-SEG-SZX-SIN-DCG67`（`LINEHAUL`），答 `201 HANDOVER_REGISTERED`。
3. 采用答 `201 COMMISSION_ADOPTED`，委托开始依据是那一次交接。
4. 新加坡尾程场外设备在 SIN-HUB 交接，答 `201`，`participationEnd` 为 `PARTICIPATION_ENDED`。
5. dispatch 随后两拍：两封交接信封与发生项已形成信封都 `PUBLISHED`；本路两封各一行 inbox（SZX 那封入账为没有参与被结束）；发生项一项，旅程 `JRN/SYN-PARCEL-DCG67-01`、登记者系统身份、服务方与协议 `…/v3` 取委托、1 件 `PIECE`、业务时间是 SIN 交接时刻；BUY 请求一份（费用项目 `SYN-FEE-ITEM-TRUNK-COST-01`），没有人工登记。
6. 人工按 `JRN/SYN-PARCEL-DCG67-01` 再登答 `200 EXISTING`，回显系统登记者；按旧约定 `SYN-JRN-…` 答 `200 CONFLICT`。
7. 再往下：计价那一拍停在计价输入不可得（合成包裹号不是申报包裹），与票 29 的记录同一停点；没有走提交真包裹那条路（要另起首方客户发行方），所以 `ZONE_UNRESOLVED` 那一停点（票 43）这次没有重走。

走完已停进程并删库。

### 偏离与待裁

- 派单号段：AT `246`–`255` 用满；新立票号 88、89 都用了（89 是评审后立的）；TF 迁移 0042 未用。
- ADR-0209 决定五部分停用是派单没点名的一处（代决 5）；ADR-0207 决定四末句停用同（代决 6）。进 main 时 README 这两行与别的票若同时改，按「只追加」合并。
- `cmd/parcel-dispatch/assemble.go` 两张扇出的参数列表与路由表头注改了几处既有行（追加参数、补一句、「两个」改「各的」）。票 61、63 也往交接扇出加路，合并时参数照追加顺序排，头注照两边各补的句子合。
- 待裁：票 88（自动形成没赶上的参与怎么补）；票 89（自动形成停住时挡同分区后面的信封）。

## 评审记录（2026-10-19，作者子代理，分支 claude-dcg67）

隔离评审：**0 阻断**，非阻断 7 项，其中 3 项要求进 main 前处理。下面逐项写处理结果：改了什么，没改的理由。

### 要求进 main 前处理的 3 项

1. **新这一路停住时挡同一分区后面的信封**（交接与交付信封按（租户、对象、口名段）分区，Outbox 只交出队头）。**改了文档，没改代码。**
   - ADR-0218 加越权风险点 8：写明代价（下一次交接的追踪投影与派送发起、POD 更正那一封的终局与投影都跟着等），写明放大效应（54、77 把确定性失败答成未决，在本路上会以未决之名挂在队头烧失败预算），写明为什么接受现状，去处是新立的票 [89](./89-actual-fulfillment-formation-blocks-later-envelopes-in-the-handover-and-delivery-partitions.md)。「Consequences」加「已知代价」一节，Links 补 ADR-0074。
   - 票 89 列出两种挪分区的形状：扇出那一路只转交一封自家分区的信封，或交接与交付编排同笔交门铃；还有一种是维持现状，三者归 TF owner 裁。
   - 票 77「证据」补进本票新增的读口：`OnHandover` 读交接；`formFor` 列段、读段、读采用连线、读 `AlternateJourneys.FindByMember`。77 与 54 各加一节「自动形成之后的分量」。
   - `assemble.go` 两张扇出旁本票自己加的注释，各补一句「殿后只保住同一封信上的各路，挡不住同分区后面的信封」，指向 ADR-0218。路由表头注是热点，不改。
   - 代码不改的理由：挪分区要新信封类型、新路由与新未决名单。门铃那一形还要动交接与交付编排，那是在途各票的地盘，超出本票。同一张交接扇出上派送发起那一路（ADR-0216）已是同一形状。未决只在依赖读不动时出现；缺陷本该响亮，修好即放行。
   - 越权风险点：接受现状是本票代决，待 TF owner 在票 89 复核。
2. **注释列举或计数别处扇出上的路**。**改了。**
   - 消费者名常量头注改成按类说：「交接与有效交付信封都扇出给多路独立消费者」。
   - `EffectiveDeliveryRegisteredEventType` 头注改成「与同一封信上别的消费者认的是同一个串」。
   - 交接那一路的哨兵名单头注改成「与同一封信上别的路的名单分开列」。
   - `assemble.go` 交付扇出旁本票自己加的「前两路」改「前面各路」。
   - 派发真库用例 `TestAnEffectiveDeliveryEndingAnAdoptedSegmentsParticipationFormsTheOccurrence` 头注里点名的 PS、VE，就是紧接着那段代码逐个比对的两个常量。注释描述的是这条用例自己的断言，不是别处扇出，照留。
3. **合并次序越过两条令牌**（派发令牌 60 → 61 → 63 → 67，文档令牌 59 → 60 → 65 → 66 → 67）。**未采纳，交协调方。**
   - 令牌在方案「波次与令牌」里，由协调方定。作者没有改写令牌的权限，也不知道 61、63、65、66 的排期。
   - 给协调方的事实：评审用 `git merge-tree` 与 origin/main（`129e9a38`）试合并，干净，合并树全绿。本票在 `assemble.go` 只往两张扇出末尾追加参数，并在本票自己加的注释行上改动；UC-TF-005 与 TF CONTEXT 只有本票自己的行与步 9。
   - 若改令牌、让 67 先进：61、63 在 67 之后 rebase，交接扇出参数照追加次序排；65、66 的 UC-TF-005 行只追加，与 67 的行不相交。
   - 若照旧令牌：本票等 61、63、65、66 进 main 后 rebase，冲突面同上。

### 其余 4 项

4. **「采用晚于参与结束……不回头形成」说法不准**：采用连线是在派发处理那封信时读的。**改了。**ADR-0218 决定七末条与越权风险点 1 改成「采用晚于派发处理那封信」，并写明形成与否随派发时序而定。票 88「现象」改成同一措辞。
5. **`registrationDisposition` 把本路走不到的几格判成入账**。**改了。**
   - 协议不在册、供应商不符，以及带 `PROCUREMENT_CONTEXT_CONFLICT` 细格的冲突，改判缺陷。本路不报服务方、协议与自营，段又已采用委托：委托记着版本时，前两格由登记用例上抛；没记着时先答缺采购上下文。三格都轮不到。
   - 协议未生效、已终止仍入账，那可能就是委托那一版在业务时间的如实答案。别的冲突（同身份异内容、成员已覆盖、范围已计过）仍入账。
   - 头注改写。ADR-0218 决定六与 UC-TF-005 `AT-TF-254`、`255` 随之补格。
   - 用例：`TestFormationDefectsAreNeitherSettledNorRetried` 加三格（经真登记用例造出答复）；`TestRegistrationBusinessAnswersSettle` 加「委托那一版在业务时间未生效」入账一格。
6. **UC-TF-005 步 9 复述了 ADR-0218 决定四、五**。**改了。**步 9 自动形成那半句只留「由派发进程随交接与有效交付信封经同一个登记用例自动形成」，找参与、旅程与登记者的取法指向 ADR-0218。
7. **每封信整段重建，O(N²)**。**未改代码，记了去处。**ADR-0218「Consequences · 已知代价」写明这项代价；去处是负载证据出现时加一个按（租户、对象、终点种类与依据）直查参与与所在段的窄读口，在此之前不另立票。理由：规模判断要有负载证据，现在加读口是没有消费方证据的优化。号段里也没有余号。

### 提交

- `0ed00c3e`：处置三格改判缺陷，及用例。
- `30201f5a`：注释不列举、不计数；两张扇出旁加队头阻塞的指针。
- `a8f75201`：ADR-0218（风险点 8、已知代价、风险点 1 与决定七措辞、决定六处置格、Links）；UC-TF-005 步 9 与 `AT-TF-254`、`255`。
- `59ac5a2f`：立票 89；77、54 补记；88 措辞。

### 验证

- `go build ./...` 退 0；`go vet ./cmd/parcel-dispatch/ ./internal/transportfulfillment/...` 退 0；`gofmt -l cmd internal scripts` 无输出。
- 带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-dispatch/ ./internal/architecture/`，全 ok。
- 没跑全量，没重生成机制清点，没动演示种子。

## 进 main 记录（2026-10-19）

- 单独一批，叠在 61、63、65、69 之后，快进推送，与本记录同一次推送。交接信封那一行的 FanOut 合成五路（VE 投影、TF 派送发起、NO 控制转出、NR 复核、实际履约发生项），头注改成各路不靠位置，另补一句：位置不影响记账，但发生项那一路停住时同样挡同一分区后面的信封（ADR-0218 越权风险点 8）。有效交付那一张扇出照本票原样追加。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `9d1dff03`：153 包 ok，0 FAIL，0 SKIP。机制清点在同一 tip 上重生成。
