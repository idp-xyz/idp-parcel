# 17 容量池登记（01·二）

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg17（此前 ready-for-agent——2026-10-19 Claude Code 会话立，用户「你作为业务和系统专家自决……解决所有的问题」授权；代决见 [tf-chain-plan](../tf-chain-plan.md)）。
Blocked by: [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md)
地盘：TF：`domain` 池与模板容量声明；`application/prepare_transport_opportunity.go` 的 `EstablishPool`；`postgres/capacity_pool.go`、`review_catalogue.go`；迁移 TF 0027；tfhttp 新 `register_capacity_pool.go`；CLI 加一个 kind；装配与 `endpoints.go` 行；种子。
出处：[01](./01-transport-opportunity-has-no-production-entry.md)；方案「01 运输机会与班次」D01-3、D01-9、D01-11，「03 装载分配与容量」D03-6，「需要的 ADR」ADR-0201。

## 做什么

- 容量池走登记册配置写，池登记口与 CLI 同一用例（方案 D01-3）。
- `EstablishPool` 改为核对班次在册（方案 D03-6）。
- 周期安排上可选的逐维容量声明随物化建池（方案 D01-9）。
- 演示种子登物化班次上的 SYN 池。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 池登记口与 CLI。
- `EstablishPool` 对不在册班次答 `SCHEDULE_NOT_FOUND`。
- 模板的逐维容量随物化建池且幂等。
- 多维分存（AT-TF-030）。
- 真库测试。
- 演示：查阅口 `capacity-pool` 列出物化班次上的 SYN 池。

## 规模

M

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg17`）

### 接票时核实

票面成立：池只有单位、没有维度（迁移 0004），`EstablishPool` 收一个自由池名、不核班次，在 cmd 下零调用，池表无生产行；周期安排文法里没有容量声明；查阅口 `capacity-pool` 已接 postgres，只是空册。票 16 已摘掉 `PrepareTransportOpportunityDeps.Schedules`，本票按它留的话接回一个只读班次口。

### 代决（用户授权「你作为业务和系统专家自决，解决所有的问题」）

计划与 ADR-0201 正文没裁、本票必须定的几格，耐久的那几条写进 [ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 文末「补记：票 17 落地时的补充裁定」（正文不改），这里只列名与票内细节：

1. **池身份**：由（班次、维度）派生，写作 `<班次身份>/<维度词>`，一班一维一池；`EstablishPoolCommand` 不再收池名。见补记「池身份」。
2. **容量维度封闭词**：`WEIGHT`、`VOLUME`、`PIECES`、`PALLET_POSITIONS`、`BAG_POSITIONS`、`CARGO_SLOTS`，大小写不折。见补记「容量维度封闭词」。
3. **单位只存不判**：照旧是不透明引用，不按维度限定拼写，同维换算留给票 18（ADR-0201 决定八）。
4. **登记口不改池**（评审后由「池不改版」收窄，见下文评审记录）：登记口上同身份同内容答`已存在`，异内容答`池冲突`并交回库里那一池。原并发分支在撞键后不比内容一律答`已存在`，一并改成按赢家内容分格。见补记「登记口不改池」。
5. **建池次序**：先按池身份答重放与冲突，再核班次在册（照方案 D03-2 给装载分配定的次序）；班次只经准备评估物化而没有显式登记，不算在册。
6. **物化登记逐池作答**：班次那一格照旧，旁边逐池列`已建立` / `已存在` / `池冲突`；一维冲突不挡班次。`ScheduleRegistrationResult.Persisted()` 取「班次落了一版或任一池新建」，端点据此给 201。见补记「物化登记逐池作答」。
7. **容量声明的文法与摘要**：可选；按维度次序归一，同一维度两次、硬上限不为正、缺单位都答`未受理`。不声明时摘要整键缺席，与此前登下的周期安排摘要逐字相同（黄金摘要用例原样通过即证）。
8. **池摘要加维度不换形状版本**：迁移 0027 在池表有行时拒绝施加，库里没有不带维度的池摘要；`unversionedOpportunityDigest` 只剩建池一个用处，随之删去，`ESTABLISH_POOL` 的无版本用例一并删。
9. **迁移号 0027**：取计划占的号（0025、0026 归同波票 23、32），没取「现有最大号的下一号」。
10. **建池装配不接消耗交接**：`buildCapacityPoolRegistration` 与 CLI 的建池编排不给 `Downstream`，交出去的 `tfhttp.CapacityPoolRegistrar` 只有 `EstablishPool` 一法。
11. **端点与响应**：路径取读面册名前缀加册名 `/transport-fulfillment-capacity-pool-registrations`（票 16 的取法）；只有新建了池答 201。响应里的池不带预占三量，现有多少预占归查阅口那一册。
12. **未决原因**：建池时班次册读不动答 `TRANSPORT_SCHEDULE_REGISTRY_UNAVAILABLE`（与班次登记口同名），物化登记时池册读不动答 `CAPACITY_POOL_REGISTRY_UNAVAILABLE`，各自进封闭集合。
13. **CLI 退出码**：加 2 = 池冲突（parcel-settlement-register 的先例）；`capacity-pool` 命令的`班次不在册`退 1。`schedule` 命令物化登记时只要有一个池冲突也退 2，班次照登。
14. **演示种子**：三份周期安排各声明重量（KG）与件数（PCS）两维（`SHA-SZX` 20000 / 4000、`SZX-SIN` 5000 / 1000、`SIN-LM` 1500 / 300，合成值）。只声明这两维，是因为准备评估只认重量、体积、件数三维有来源（决定八），而体积要看客户报没报外廓。种子按固定合成日期 2026-10-21 显式物化登记干线一班，池随之建立；日期不取「今天」，因为班次与池的身份都由它派生，重灌要答`已存在`。
15. **AT 号**：`AT-TF-105`（建池核班次在册）、`AT-TF-106`（随物化建池且幂等、换版不改写已建的池），认领时没有占号。

### 越权风险点

ADR-0201 补记接正文编号续写的四条：17 池身份绑死班次与维度、18 维度只封闭这六种、19 池容量更正没有入口、20 单位只存不判（都归 TF owner）。本票另有：

1. **一维冲突不挡班次**（TF owner）：只看顶层 outcome 的调用方会漏看池冲突。在线口逐池给答案，CLI 用退出码 2 补。
2. **种子日期固定**（演示 owner）：2026-10-21 那一班会随日期过去而变成过去的班次。准备评估的候选照旧按日期现算，不靠它；票 20、21 的演示要预占或分配，就在当天显式登一班。
3. **查阅口两处加键**（前端 owner）：池行加 `dimension`，周期安排行加 `capacities`。管理台池区眼下不显示维度，周期安排区还没有，见票 [38](./38-admin-web-transport-schedule-section-reads-connection-and-nodes.md)。
4. **登记写授予无逐册维度**（access-identity owner）：照方案 X2，持登记写授予的配置员也能登池。

### 做了什么

- 迁移 `migrations/transport_fulfillment/0027_capacity_pool_dimension_and_template_capacity.sql`：池表有行即拒绝施加；池表加 `dimension`（封闭词 CHECK）与「池身份 = 班次/维度」CHECK；新立 `schedule_template_capacity` 子表，外键挂周期安排那一版，维度封闭、硬上限为正、单位非空。
- 领域：新 `domain/capacity_declaration.go`（`CapacityDimension`、`ParseCapacityDimension`、`CapacityDeclaration`、归一）；`transport_schedule_capacity.go` 的池加维度、身份派生，`CapacityPoolSpec` 去掉池名换成维度；`schedule_template.go` 的文法加 `Capacities`，加 `ScheduleTemplate.MaterializeCapacityPools`；`payload_canonicalization.go` 周期安排摘要加可缺的 `capacities`，池摘要改从池本体算并加维度。
- 端口：`catalogue_read.go` 池行加 `Dimension`，周期安排行加 `Capacities`（`ScheduleTemplateCapacityRow`）。`ports.go` 未动。
- postgres：`capacity_pool.go` 读写维度并核读回的池身份；`schedule_template.go` 随那一版落、读容量声明（相关子查询成 jsonb）；`review_catalogue.go` 两册各加一格。
- 应用：`prepare_transport_opportunity.go` 的 `EstablishPool` 改为先答重放 / 冲突、再核班次在册（新格 `SCHEDULE_NOT_FOUND`），抽出建池三段供物化路径复用；`register_transport_schedule.go` 的物化路径随班次逐维建池，结果加 `Pools()`、`Persisted()`，命令加 `Capacities`；`unversioned_payload_digest.go` 删去建池那一函数。
- tfhttp：新 `register_capacity_pool.go`（载荷、Intake、端点、响应）；`register_transport_schedule.go` 载荷收 `capacities`，响应带 `pools`；`query_transport_fulfillment_records.go` 两册加键；`unconfigured_intake.go` 补一法；`operator_registry_intake.go` 头注。
- cmd：新 `cmd/parcel-api/assemble_capacity_pool.go`；`assemble_transport_schedule.go` 接池册；`endpoints.go`、`main.go` 各追加一行；`unwired_orchestration.go` 补占位；`cmd/parcel-transport-register` 加 `capacity-pool` 命令与退出码 2。
- 种子：`data/transport/` 三份周期安排加容量声明，新 `04-materialized-schedule-szx-sin.json`；`seed.sh` 追加一节；README 组成表追加一行、数据故事第 8 条追加两行。
- 文档：ADR-0201 文末补记（正文不改）；UC-TF-003「容量与班次规则」一句指针与 `AT-TF-105`、`AT-TF-106`；演示动线第 3 步追加补记。

### 用例

- 领域 `capacity_declaration_test.go`：维度封闭词逐词认回、变体与列举外的词拒；同一班次两维各成一池、身份派生、一维预占不动另一维（`AT-TF-030`）；容量声明归一、重复维度 / 零上限 / 缺单位 / 缺维度拒、正文拷贝隔离、列举顺序不影响摘要、声不声明摘要不同；按声明在物化出的班次上立池且确定，不声明不立，临时班次与别的周期安排拒。`payload_canonicalization_test.go` 钉带容量的周期安排与新形池摘要两份黄金值，原「不带容量」那份黄金值不动。
- 应用：`prepare_transport_opportunity_test.go` 加班次不在册（含他租户）答 `SCHEDULE_NOT_FOUND` 零写入、班次册读不动未决、同身份重放 / 冲突先于班次核对且冲突交回库里那一池、同班另一维建池、文法外零写入、并发撞键按赢家内容分格；原预占 / 释放 / 消耗用例改用派生池身份。`register_transport_schedule_test.go` 加物化登记逐维建池、再登已存在、换版后冲突 / 已存在 / 新建三格并存且不改写（`AT-TF-106`），容量文法外未受理、不声明与临时班次不带池、池册读不动未决。
- 真库 `adapters/postgres/capacity_pool_registration_test.go`：建池核班次在册（没登过、只经评估物化、他租户三形）、登了之后建 / 已存在 / 冲突不改写、两维两行各带维度（`AT-TF-105`、`AT-TF-030`）；容量声明在当前版、候选来源、查阅册三处读回同一组，物化登记建池、再登不多落、换版后冲突与新建（`AT-TF-106`）；迁移 0027 有行即拒；库面 CHECK 挡坏行（维度不在封闭词、身份不是派生的、缺维度、声明维度 / 上限 / 单位 / 挂在不在册的一版上）。`journey_schedule_capacity_test.go`、`review_catalogue_test.go`、`capacity_consumption_handoff_test.go` 换新构造。
- tfhttp `register_capacity_pool_test.go`：各格状态码与回显、物化登记响应逐池列出、载荷逐字段翻译、自报租户 / 自起池名 / 容量不是整数答坏报文、未出示令牌 401 一族、未配置 403。`query_transport_fulfillment_records_test.go` 补 `capacities` 与 `dimension`。
- `cmd/parcel-api/assemble_capacity_pool_test.go`（真库）：经操作者渠道登记写面，班次不在册 200、带容量的周期安排物化登记一班 201 且两池已建立、另登一维 201、再登 200。端点探针表与登记写面列表各加一口。
- `cmd/parcel-transport-register`：演示种子灌真库后，物化登记的干线一班两池在册、重跑已存在；`capacity-pool` 命令建立 / 已存在 / 冲突退 2 / 班次不在册退 1；三份周期安排都只声明重量与件数两维。

### 验证

- `go build ./...`；`go vet` 覆盖 `./internal/transportfulfillment/...`、`./cmd/parcel-api/`、`./cmd/parcel-transport-register/`、`./internal/architecture/`、`./migrations/`；`gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN=…55432… go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./cmd/parcel-api/`、`./cmd/parcel-transport-register/`、`./internal/architecture/`（接线棘轮、类型可达性、前缀门禁）、`./migrations/`、`./scripts/demo-seeds/...` 全 ok。没跑全量。
- 一次性库 `dcg17_seed` 上 `seed.sh` 灌入退 0，末段一行 `SCHEDULE_REGISTERED（SYN-SCHED-SZX-SIN-DAILY/2026-10-21 v1）；…/WEIGHT POOL_ESTABLISHED；…/PIECES POOL_ESTABLISHED`。在其上起 `parcel-api`（`127.0.0.1:19171`，操作者渠道接演示 Dex），配置员令牌：`GET /transport-fulfillment-records?registry=capacity-pool` 答 `CAPACITY_POOLS_LISTED`，列出 `SYN-SCHED-SZX-SIN-DAILY/2026-10-21/WEIGHT`（5000 KG）与 `…/PIECES`（1000 PCS）；`registry=transport-schedule-template` 三份都带 `capacities`；`POST /transport-fulfillment-capacity-pool-registrations` 指向 `…/2026-10-22`（没登过）答 200 `SCHEDULE_NOT_FOUND`，在 `…/2026-10-21` 上登 `BAG_POSITIONS` 答 201、再登 200 `EXISTING_POOL`；物化登记 `SYN-SCHED-SIN-LM-DAILY/2026-10-22` 答 201 且两池 `POOL_ESTABLISHED`；查阅员令牌建池答 403 `OPERATOR_NOT_GRANTED`。证据只记 `S`。走完已停进程、`DROP DATABASE … WITH (FORCE)`。
- 与本地 `main`（`d7283946`）`merge-tree` 无冲突。机制清点未重生成（进 main 时统一做）。

### 偏离与待裁

- **AT 号**：`AT-TF-105`、`AT-TF-106` 没占号，同波票若也在 TF 用例里加 AT，进 main 时可能要按号重编。
- **迁移次序**：0027 先于同波的 0025、0026 进 main 时，已施加过 0027 的库后补 0025、0026 的施加次序归协调方按号合并时定。
- **池容量更正**（ADR-0201 补记风险点 19）：UC-TF-003 写容量事实更正形成新版本，本票只答冲突。建议另立票；为免同波撞号没在本票立，交协调方编号。
- **给票 18、20、21 的接缝**：评估时物化的候选可调 `ScheduleTemplate.MaterializeCapacityPools` 算池（只算不存，满容量）；已登记班次上的池要按班次列，本票没加这一问（`postgres/capacity_pool.go` 在票 21 的地盘里，D03-3 的「班次挂没挂池」也要这一问）。预占与释放的命令仍按池身份串寻址，派生身份即 `<班次>/<维度>`。
- **消耗交接的信封 ID**：`capacityConsumptionEventID` 用 `/` 拼租户、池与预占，池身份现在自带 `/`。这一口没接线，方案 D03-7 已把它并入 envelope-id-bounds/01，指纹化时一并消掉。本票只改了它的测试夹具。
- **地盘外动到的文件**（都是追加或随签名必须跟的）：`application/unversioned_payload_digest.go`(+test)、tfhttp `operator_registry_intake.go`（头注）与 `unconfigured_intake.go`、`postgres/capacity_consumption_handoff_test.go`、`cmd/parcel-api/assemble_transport_schedule.go`、`unwired_orchestration.go`、`endpoints_test.go`、`assemble_operator_decisions_test.go`、`cmd/parcel-transport-register/main_test.go`、`docs/design/synthetic-demo-journey-script.md`、UC-TF-003。没碰 peer 地盘与 `apps/admin-web`。

## 评审记录（2026-10-19，隔离评审一轮，作者子代理修订）

阻断 0 项。非阻断 9 项：标「进 main 前修」的 3 项全修；其余 6 项修 3 项、登记去向 3 项。修订两笔：代码 `0fe11a37`，文档 `b0aa2e46`。

### 已修

1. **`PAR-NET-12` 与 ADR-0201 补记口径不一**（进 main 前修）：登记册这一行照 `PAR-NET-08` 补〔ADR-0146 拆分〕。「容量池身份」由（具体班次、维度）派生，划为机制；维度封闭词与周期安排上逐维容量声明的文法划为产品策略，都指向 ADR-0201 补记。租户提供各班次（或其周期安排）启用哪几维、各维单位与硬上限，以及按资源粒度分池等需求。原有「缩容与超配缺口处置 → psb/09」一句不动。补记里加一条「参数登记册」作对照。
2. **补记「池不改版」与 TF CONTEXT 缩容规则相抵**（进 main 前修）：改名「登记口不改池」，范围收窄到登记写面，即同（班次、维度）异内容答`池冲突`，不经再登记改上限。删掉「换上限会让已占的量失去守恒依据」那半句，改写为：登记口立池、不更正池；容量变了是容量事实更正，UC-TF-003 要它形成新版本，TF CONTEXT 要缩容不改写既有预占、形成超配缺口，这些归另立的更正入口。风险点 19 跟着改，指向新立的票 [40](./40-capacity-pool-capacity-correction-has-no-entry.md)，并写明登记口的`池冲突`不能读成「池容量不可更正」，票 20、21 与 psb/09 不以它为拒绝缩容的依据。代码里三处引这个补记名的注释（`poolReplayOrConflict` 头注、`establishPools` 头注、建池幂等用例的 Covers）同步改名，`poolReplayOrConflict` 头注里同一个理由也一并换掉。CLI 退出码 2 的说明从「池不改版」改为「登记口不改池」。
3. **跨文件计数与按位置数邻行**（进 main 前修）：`cmd/parcel-transport-register/main.go` 包头注改成「与 parcel-api 上对应的登记口调同一个编排」；`cmd/parcel-api/endpoints.go` 新行注改成「族与周期安排、具体班次登记口同」，按名指，不按位置数。
4. **TFC-1 不换号没登记**（可选）：没有照票 16 的先例不处理。ADR-0014 的部分停用范围在 README 那一行写的是「仅限 PC 客户合同一册……此后任何一册再改规范化形状照本记录换号」，只在补记里补半句会与这句相抵。因此照 README 顶部「部分停用」规则补全登记：ADR-0201 补记「迁移 0027」一条补上「`TFC-1` 不换号，与 ADR-0177 决定三同一前提」，同时部分停用 ADR-0014 那一句，适用场景仅限容量池一册加维度这一次；ADR-0014 的 `Status` 行与 Links 各加一条前向指针（正文不改）；`docs/adr/README.md` 里 ADR-0014 那一行在「此后任何一册……」之前插入这一次的适用场景（没有新增行，也没有重排）。另加风险点 21：若有库绕过了 0027 的拒绝、留下旧池，这些池的摘要会全部失配，只能答`池冲突`。票 16 改班次摘要时 `REGISTER_TRANSPORT_SCHEDULE` 是新起的面名，不是同一面改形状，所以不算进这一条。
5. **查阅册与登记答复的容量次序不一**：领域加 `domain.CompareCapacityDimensions`，按封闭词次序比较，声明归一也改用它。`postgres/review_catalogue.go` 的 `catalogueCapacities` 按它稳定排序。SQL 里仍按维度词排序，只为结果稳定。列面照实转写、不当重建门（文件头注的纪律），认不出的维度词照原样列出并排在最前。真库用例与 tfhttp 用例都改成 WEIGHT 在 PIECES 之前。
6. **vertical 用例 Covers 替票 18 定件数来源**：改成「准备评估只认重量、体积、件数三维有申报来源（ADR-0201 决定八）」，不再写「件数即对象数」。

### 登记去向，不在本票改

7. **AT 号撞号**（`AT-TF-105`、`106`）：本票保留。按方案「并行车道」波 2 的落地顺序 17 → 23 → 27 → 32，本票先进；dcg23（UC-TF-004、ADR-0202 占 105 至 107）与 dcg32（UC-TF-005、ADR-0205 占 105 至 111）由协调方在各自进 main 前顺延重编，ADR 里的引用一并改。进 main 记录照此写明。
8. **`capacityConsumptionEventID` 遇上含 `/` 的池身份**：不在本票改。已在 [envelope-id-bounds/01](../../envelope-id-bounds/issues/01-remaining-envelope-ids-still-concatenate-instance-references.md) 末尾追加一条补记，写明前提：池身份写作 `<班次>/<维度>`，物化班次身份本身也带 `/`；运输履约半边改指纹形时一并消掉。
9. **与同波分支合并**：修订后重跑 merge-tree。与当前 `main`（`08254c8c`）、`claude-dcg32` 无冲突。与 `claude-dcg27` 冲突在 `scripts/demo-seeds/README.md`。与 `claude-dcg23` 冲突在 `cmd/parcel-api/endpoints.go`、`endpoints_test.go`、`main.go`、`unwired_orchestration.go` 与 `docs/design/synthetic-demo-journey-script.md`，比评审时多出后三处，都是只追加的装配与动线令牌。协调方按令牌次序合并（17 先），后进的分支追加在本票所加行之后。

### 修订后验证

- `go build ./...`；`go vet`（`./internal/transportfulfillment/...`、`./cmd/parcel-api/`、`./cmd/parcel-transport-register/`、`./internal/architecture/`、`./migrations/`）；`gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN=…55432… go test -p 1 -count=1`：`./internal/transportfulfillment/...`、`./cmd/parcel-transport-register/`、`./migrations/`、`./cmd/parcel-api/`、`./internal/architecture/`、`./scripts/demo-seeds/...` 全部 ok。用 `-v` 确认查阅册次序那条真库用例（`TestTemplateCapacitiesMaterializePoolsOnTheRegistry`）确实执行了。种子数据没改，`seed.sh` 没有重灌。

