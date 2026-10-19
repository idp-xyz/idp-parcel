# 64 路由计划带交付段，运输准备跳过它

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 @@PIN@@；交接与派送 A 波同批，见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg64。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无（种子令牌）
地盘：NR `domain/initial_route_plan.go`（段种类与交付终点）、`application/calendar_projection.go`、`application/catalog_initial_route_evidence.go`（带出所用服务区域版本）、`application/reassess_route.go`（已执行前缀只取节点间段）、`ports/catalog_registration.go`（服务日历节点行派送时长）、`adapters/registrationjson/translate.go`、`adapters/postgres/network_catalog*.go`、`adapters/postgres/initial_route.go`（段 json）、NR 迁移（认领时现取）、`adapters/http/query_route_plans.go`；TF `adapters/networkrouting/planned_leg_source.go`（交付段不形成准备腿）；种子 `scripts/demo-seeds/data/network/` 加 SIN-LM 日历新版本（带派送时长）+ `seed.sh` 一行；文档 ADR-0217、NR CONTEXT「路由形成与选择」一句、GLOSSARY「计划履约段」一行、UC-NR-001、`PAR-NET-14`、演示脚本（末端交接声明交付段）。
出处：[06](./06-delivery-dispatch-has-no-in-process-trigger-and-no-recipient-leg.md)；方案「复核」06「升级」，「代决」C11，「需要的 ADR」ADR-0217。

## 做什么

- NR 计划在交付节点之后带一个交付段；终点、窗口、未登记时的答法照方案 C11。
- 段带种类；已执行前缀与 TF 运输准备只取节点间段，避免准备消费者把交付段判为译不出。
- 演示种子给 SIN-LM 登派送时长（SYN 值，证据 `S`）。
- 文档：起草 ADR-0217。
- 进 main 的令牌次序见方案「波次与令牌」；种子落地后把 SHA 通知 idp-parcel-9b。

## 完成判据

- 登了派送时长：计划多出交付段，段种类为交付，窗口 = 离开交付节点的时刻 + 时长。
- 没登：计划段链止于交付节点，与今天相同。
- TF 窗口缝：交付段引用答 `RESOLVED`；不存在的序位答 `MISSING`。
- TF 运输准备只备节点间段，不成毒丸。
- 复核的已执行前缀不变；旧计划行读回不变。
- 真库测试。
- 演示：提交一单 → `/route-plans?registry=initial-route` 末段为交付段；末端交接声明该段引用 → 派送任务窗口取交付段窗口（配合 [60](./60-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope.md)）。

## 规模

L

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg64`，基于 `3f7b2b48`）

### 接票时核实

票面成立：NR `calendar_projection.go` 只按连接成段，演示计划三段止于 `SYN-NODE-SIN-LM`；TF `planned_leg_source.go` 对不指名连接的段答 `ErrUntranslatableAnswer`（不在哨兵名单），交付段一进计划信封就成毒丸。票面没提、实现时要一并管的两处：候选成本取数侧（NR `adapters/parcelpricing`）按线路段号逐段取成本依据，交付段若混进候选路径的 `Legs`，候选会停在待判断、演示停在 `CANDIDATE_COSTS_PENDING`；路由计划查阅口此前只上检索列面，看不到段。

号段（`origin/main 3f7b2b48` 上核过）：UC-NR 现有最大 `AT-NR-053`，本票用 `AT-NR-054` 至 `AT-NR-063`；`AT-TF-201`；NR 迁移现有最大 `0019`，本票 `0020`；ADR-0217；新立票 72，评审后立 73。

### 做了什么

- 迁移 `migrations/network_routing/0020_service_calendar_delivery_minutes.sql`：日历加可空的 `delivery_minutes`，CHECK 不为负。
- NR 领域 `domain/initial_route_plan.go`：`PlannedLegKind`（`BetweenNodesLeg` 零值、`DeliveryLeg`）、`PlannedLegKindFrom`、`NewDeliveryLeg`、`PlannedLeg.Kind/DeliveryScope`、`InitialRoutePlan.NodeLegs`；`FormInitialRoutePlan` 立交付段的不变量（只在末尾、至多一段、起于末个节点；首段必须是节点间段）；`Nodes()` 只由节点间段推。`domain/route_freeze.go` 的 `PlanNodes` 遇交付段答不出。
- NR 端口：`ServiceCalendarDefinitionVersion.DeliveryMinutes`；`CandidatePath.Delivery/HasDelivery` 与 `PlanLegs()`；`RoutePlanLegCatalogueRow` 与 `InitialRouteCatalogueRow.Legs`。
- NR 应用：`calendar_projection.go` 在末个节点登了派送时长且候选有交付范围时成交付段、投影延到交付段终点（`deliveryLeg`）；`catalog_initial_route_evidence.go` 按区域解析取交付范围（`deliveryScopes`）；初始路由与复核形成计划取 `PlanLegs()`；复核的已执行前缀改取 `NodeLegs()`；登记门派送时长为负答日历内容越界。
- NR 适配器：`registrationjson` 日历载荷加 `delivery_minutes`；postgres 目录日历读写带派送时长；计划 jsonb 段行加可缺的 `kind`（只给交付段写，交付段的 `to` 写交付范围），读回无 `kind` 即节点间段、带 opaque 的交付段判坏；`route_plan_catalogue.go` 成计划行重建整份计划后逐段列出；HTTP `query_route_plans.go` 成计划行加 `legs`。
- TF：`planned_leg_source.go` 只译 `NodeLegs()`；`delivery_window_source.go` 注明不按段种类分支。
- 种子：`scripts/demo-seeds/data/network/24-calendar-node-sin-lm-v2.json`（处理 240、派送 240 分钟，2026-06-01 起，依据 `SYN-NET-OPS/CHANGE-SIN-LM-DELIVERY-2606`），`seed.sh` 追加一行，README 组成表追加一行。
- 文档：ADR-0217；ADR-0131 后继指针（只在 Links 与 adr/README 该行，Status 行不动），ADR-0175 决定一、决定四各一句与 ADR-0201 决定二末句部分停用（Status 行、Links、adr/README 各该行），adr/README 追加 0217 一行；NR CONTEXT「服务日历」与「路由形成与选择」；GLOSSARY「路由计划」一行；UC-NR-001 `AT-NR-054`–`058`、`060`–`063`，UC-NR-003 `AT-NR-059`，UC-TF-003 `AT-TF-201`；`PAR-NET-14` 一句；演示脚本第 5 步补记。
- 新立票 [72](./72-admin-web-route-plans-shows-planned-legs.md)：管理台路由计划页列段，契约夹具补 `legs`。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

耐久的写进 ADR-0217，这里只记落地时的细节。

1. **交付段另放候选路径一格**，不混进与线路段号逐一对应的 `Legs`；计划形成时由 `PlanLegs()` 接上。成本取数侧一行不改，交付段不进比价（`AT-NR-058`）。
2. **交付范围取区域解析的那一格版本引用**：覆盖收件地址的区域版本，多版并列照解析依据的「+」连写；责任引用同串。
3. **交付段窗口依据写 `NODE/<节点>@<日历修订>`**，与节点日历在候选投影依据里的写法同形；TF 不据依据认交付段，按种类跳过。
4. **候选投影延到交付段终点**（方案 C11 越权风险点「总窗口变长」）：交付段成段才延，没成段时投影与此前相同。
5. **`PlanNodes` 遇交付段答不出而不是跳过**：调用方该交 `NodeLegs()`；跳过会把误用藏起来。复核经 `planNodesOf` 取节点序列，答不出就返回 error，不折成「冻结形态未配置」（评审后改，见评审记录）。
6. **种类缺席即节点间段**：计划 jsonb 只给交付段写 `kind`，节点间段与旧行同形，不迁移、不回填。
7. **查阅口逐段列出要重建整份计划**：与段窗口窄读口同一读法；坏行让整页答不出，不跳过。为此把 `route_plan_catalogue_test.go` 的成计划夹具行换成能过构造门的单段计划（无 `kind`，即旧行形状）。
8. **`legs` 只在有段时出现，契约夹具暂不含它**：放进夹具，管理台契约测试当场红，而 `apps/admin-web` 是别的会话的地盘；票 72 收口。
9. **派送时长不按日历适用对象种类拒登**：与截单、处理时长、衔接缓冲同一受理门，登在连接或线路日历上不被读。
10. **ADR-0175 与 ADR-0201 的停用条款由本票自判**：方案只点了 ADR-0131 与 ADR-0201「只取节点间段」；ADR-0175 决定一「三格」、决定四「线路的连接序列是候选段链」也被改了，照 adr/README 顶部规则一并写成部分停用。ADR-0114 决定一被本记录重申、没改，不加指针。
11. **`AT-NR-059` 放 UC-NR-003**：已执行前缀属复核用例；票面只写了 UC-NR-001。
12. **演示取值**：派送时长 240 分钟（合成值，证据 `S`）；修订 2 照修订 1 重登处理时长 240、不登截单。

### 越权风险点

ADR-0217 文末各条原样成立：总窗口变长、责任暂指服务区域版本、派送时长挂节点不挂区域、交付段不进比价、交付段没有不透明形、查阅口重建整份计划、管理台契约夹具不含 `legs`、段种类不推派送段。另加落地时的几条：

1. **答不出的节点序列让复核这一拍报错**（NR owner）：只有绕过构造门的计划才走得到，复核返回 error 而不是落未决；这一支经构造门走不到，没有用例钉它，生产调用取 `NodeLegs()` 由真库与应用用例钉住。
2. **派送时长登错对象静默无效**（NR owner）：登在连接或线路日历上不拒、也不被读。
3. **停用范围自判**（NR、TF owner）：代决 10 的停用不在方案里，owner 若认为 ADR-0175 应整份由新记录取代或不算停用，改 adr/README 两行即可，代码不动。

### 用例

- 领域 `domain/delivery_leg_test.go`：交付段在末尾、种类与交付范围、节点序列不含范围、`LegAt` 越过交付段答没有、`PlanNodes` 遇交付段答不出、剩余段数不含交付段（`AT-NR-054`、`AT-NR-059` 领域半边）；错位的交付段不成计划（`AT-NR-056`）；缺交付范围拒、0 分钟零长窗口、种类词封闭（`AT-NR-063` 领域半边）。
- 应用 `application/delivery_leg_test.go`：选中候选的交付段进计划；带交付段的计划在起点节点复核、冻结段数 1 即越过边界不改路（`AT-NR-059`）；改路新计划带上候选的交付段。`register_network_catalog_test.go` 派送时长为负拒（`AT-NR-063`）；`translate_test.go` 派送时长缺席为 nil、登 0 为 0。
- 真库 `adapters/postgres/delivery_leg_test.go`：修订 2 登派送时长、接续闭合修订 1，证据带交付段（`node-lax → area-us@1`，12:30Z 起 180 分钟，依据 `NODE/node-lax@2`），投影延到 15:30Z（`AT-NR-054`、`AT-NR-057`）；没登不成段（`AT-NR-055`）；负值被 CHECK 挡（`AT-NR-063`）；带交付段的计划落库读回、`#3` 答交付段窗口、`#4` 未找到、jsonb 只有交付段带 `kind`（`AT-NR-060`）；直插旧形状行读回全是节点间段（`AT-NR-061`）；查阅口逐段列出、末段 `DELIVERY` 终点 `area-sg@1`（`AT-NR-062`）。`route_plan_catalogue_test.go` 成计划行带段、无路可走行无段。
- HTTP `query_route_plans_test.go`：`legs` 逐键转写、无路可走行没有 `legs`；契约夹具字节不变。
- TF `planned_leg_source_test.go`：交付段不译、两段节点间段序位不变（`AT-TF-201` 翻译半边）；真库 `delivery_window_source_db_test.go`：NR 落带交付段的计划，窗口缝 `#2`（交付段）`RESOLVED` 带交付段窗口、`#3` `MISSING`、`#1` 照答（`AT-NR-060` TF 一侧）。
- 派发真库：`transport_preparation_skips_delivery_leg_test.go` 末段为交付段的初始路由经生产路由表只备 `#1`、信封定稿、TF 与 VE inbox 各一行（`AT-TF-201`）；`reference_network_wiring_test.go` 演示种子经生产装配的证据带交付段（`SYN-NODE-SIN-LM → SYN-AREA-SG@1`、240 分钟、`NODE/SYN-NODE-SIN-LM@2`），带交付段时出价照旧三段 25500（`AT-NR-058`）。

### 验证

- `go build ./...`、`go vet`（NR、TF、`cmd/parcel-dispatch`、`cmd/parcel-api`、`scripts/demo-seeds`）、`gofmt -l` 干净。
- 真库（`IDP_PARCEL_POSTGRES_DSN=…55432…`，`-p 1 -count=1`）：`./internal/architecture/`、`./internal/networkrouting/...`、`./internal/transportfulfillment/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./cmd/parcel-network-register/`、`./migrations/...`、`./scripts/demo-seeds/...` 全绿。事务闭包门禁抓到窗口缝真库用例在闭包里 `t.Fatalf`，已搬出（`75adb9db`）。
- 种子：一次性库 `dcg64_seed` 上 `seed.sh` 退 0；`SYN-NODE-SIN-LM` 日历修订 1 `effective_to = 2026-06-01`，修订 2 `processing 240 / delivery 240`、无终点。
- 演示（同一库，实测只记 `S`）：首方客户发行方另起一只同配置的 Dex（宿主端口 5568，`iss` 不变；本机常驻那只早于首方客户端入册），起 `parcel-api`（`127.0.0.1:19464`）与 `parcel-dispatch`（`NETWORK_SERVICE`、2s）。16:32:21Z 提交 `SHR-HKZPKV7ZLSE2K6VIVUVOUDN2PU`，派发推到初始路由 `ROUTE_FORMED`、计划 `RPV-000000000001` 四段，`network-routing.initial-route.formed` `PUBLISHED`，运输准备只有 `#1`–`#3` 三行。配置员令牌 `GET /route-plans?register=initial-route` 答 200 `INITIAL_ROUTES_LISTED`，`legs` 四段，`#4` `DELIVERY`，`SYN-NODE-SIN-LM → SYN-AREA-SG@1`，2026-10-21T10:32:23Z–14:32:23Z，依据 `NODE/SYN-NODE-SIN-LM@2`。新加坡尾程场外设备 `SYN-DEVICE-OFFSITE-SG-LM-01` 登末端交接（`segmentServiceAction=FINAL_DELIVERY`，`plannedSegment=RPV-000000000001#4`）答 201 `HANDOVER_REGISTERED`；手打派送发起答 201 `DISPATCH_TASK_FORMED`，`windowFrom/windowTo` 即交付段窗口（微秒截断）。走完已停两进程、删容器、`DROP DATABASE … WITH (FORCE)`。

### 偏离与待裁

- 票面写的 `/route-plans?registry=initial-route`，端点的查询参数名是 `register`，照端点。
- 演示第二半的派送发起仍是手打触发口；进程内随交接信封发起是票 60，60 落地后演示改由派发一拍发起，声明同一段 `#4`。
- 种子令牌：本票进 main 后由协调方把种子那一笔的 SHA 通知 idp-parcel-9b（方案「波次与令牌」）。
- 机制清点未重生成（进 main 时统一做）。
- ADR-0217 越权风险点待 NR owner 复核、TF owner 会签。

## 评审记录（2026-10-19，隔离评审一轮，作者子代理修订）

阻断 0 项；非阻断 7 项，其中 3 项标了进 main 前修。7 项都已处理：5 项改了，2 项按评审建议的去向登记，没有不采纳的。

1. **ADR-0131 改了 Status 行**（进 main 前修）——已修。Status 行恢复 `Accepted`。ADR-0131 没有被停条款，照 adr/README 顶部规则不改 Status 行，只保留 Links 节的后继指针与 adr/README 该行的「后继」注（`2d2a7903`）。
2. **演示取值写了多处**（进 main 前修）——已修。ADR-0217 决定六、`seed.sh` 注释、演示脚本补记改指种子 README 组成表 `24-calendar-node-sin-lm-v2.json` 那一行，不写数值。README 那一行照 transport 行的先例补「不复述，都指到这里」，行内「加 240 分钟」改「加派送时长」。演示脚本里实测的窗口时刻是取证，保留。本票完成记录里的取值是落地记录，不改（`2d2a7903`、`9d482b8e`）。
3. **计数与变更叙述**（进 main 前修）——已修。
   - ADR-0217 Consequences 删掉「三条」，改为直接写不变量本身。
   - `route_plan_read.go` 与 `query_route_plans.go` 的种类词改指 `domain.PlannedLegKind`。
   - `register_network_catalog_test.go` 直接写出截单、处理时长、衔接缓冲。
   - freezeOf 的理由改成一般表述（交付段计入剩余段数会让冻结边界整体提前一个节点），不再引用演示取值，挪进 `planNodesOf` 的注释。
   - `calendar_projection.go` 删去「与这一格出现之前相同」。
   - `route_plan_read.go` 超长的那一行重新折行，适用性一段另起。
   - 评审没点、属同一类的几处一并改了：domain 测试里的「封闭两格」，迁移 0020 注释里的「0015 的三格」（改为写出格名），UC-NR-001 `AT-NR-055` 的「与没有这一格之前相同」，票 72 的「封闭两格原词」（`5b374b77`、`2d2a7903`、`e7bf1b1e`）。
4. **freezeOf 把编程错误折成未配置**——已修。新增 `planNodesOf`：`PlanNodes(plan.NodeLegs())` 答不出时返回 error。`freezeOf` 与 `structuralAutoReroute` 都改走它，把 error 往上交，复核 `Handle` 随之返回 error。`structuralAutoReroute` 原先静默把 `onlyUnexecuted` 当作假，也一并改掉。本票代决 5、越权风险点 1 与 ADR-0217 Consequences 随之改写。没有补用例：经构造门成形的计划走不到这一支，零值计划在适用性比对的 `Nodes()` 里会先出错。
5. **契约夹具不含 `legs`**——去向票 [72](./72-admin-web-route-plans-shows-planned-legs.md)，票面已写明两侧同一笔补齐，本轮没改。
6. **复核册不列段**——立票 [73](./73-route-reassessments-register-lists-the-new-plan-legs.md)。ADR-0217 越权风险点 6 写明复核册暂不列段；adr/README 0217 行的风险摘要补了这一条。读码与评审描述有一处出入：复核册连新计划版本也不给（检索列只有被复核的原计划 `reviewed_plan`），不是「只给 new_plan 的版本」。风险点与票 73 都照读码写，并把复核时首次形成计划（`FIRST_PLAN_FORMED`）那一路也算在内。
7. **CONTEXT 不透明段写到「交付范围」**——已修。ADR-0217 越权风险点 5 点明：CONTEXT 词条里止于交付范围的不透明段当前没有实现形状，留给末端外包另行裁决，CONTEXT 不改。

修订后验证：`go build ./...`、`go vet`（`./internal/networkrouting/...`、`./internal/transportfulfillment/adapters/networkrouting/...`、`./cmd/parcel-dispatch/`）、`gofmt -l` 都干净。真库测试（`-p 1 -count=1`）全绿，跑了 `./internal/networkrouting/...`、`./internal/transportfulfillment/adapters/networkrouting/...`、`./internal/architecture/`、`./cmd/parcel-dispatch/`、`./migrations/...`、`./scripts/demo-seeds/...`。`seed.sh` 过了 `bash -n`；在一次性库 `rv64b_seed` 上灌入退 0，`SYN-NODE-SIN-LM` 修订 1 闭合于 2026-06-01，修订 2 处理与派送时长与种子文件一致、无终点，查完已 `DROP … WITH (FORCE)`。

## 进 main 记录（2026-10-19）

- 交接与派送 A 波 60、64、68 合成一批，按 68 → 64 → 60 叠到 main 上，整批快进推送，与本记录同一次推送。合批时协调方在合批分支上直接做了票 71（派送发起过期注释改指 ADR-0216）与票 74（ADR 索引 ADR-0205 两行合一）；另把 ADR 索引里 0206、0207、0209、0210、0217 各两行合成一行——那是协调方先前合批时的冲突解法把同一行的新旧两版都留下了，解法已改为按合并基点判新旧。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 @@PIN@@。机制清点在推送 tip 上重生成。
