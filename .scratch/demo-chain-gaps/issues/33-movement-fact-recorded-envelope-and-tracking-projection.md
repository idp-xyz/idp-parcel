# 33 移动事实已形成信封与 VE 投影

Category: enhancement
Status: in-progress——隔离评审已过（0 阻断），修补完，待进 main，分支 claude-dcg33。2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [32](./32-movement-facts-declare-segment-and-record-departure-gate-reading.md)
地盘：TF 新 `postgres/movement_fact_handoff.go` 与端口、`record_movement_fact.go`；VE 新 inbox 消费者与派生；`cmd/parcel-dispatch/assemble.go`(+_test)；`cmd/parcel-api/assemble_movement_fact.go`；种子 visibility 映射（参考配置 + SYN 采用）。
出处：[05](./05-movement-facts-are-unlinked-and-departure-gate-is-self-reported.md)；方案「05 移动事实与出发门禁」D05-1、D05-10，「跨票通则」X3，「需要的 ADR」ADR-0205。

## 做什么

- `transport-fulfillment.movement-fact.recorded` 与 VE 消费者、路由行、里程碑映射参考配置在同一个变更上线（方案 D05-10、X3）。
- 阻断读数随信封交 VE，派生异常信号（方案 D05-1）。
- 演示种子：visibility 映射的参考配置与 SYN 采用。
- NR 路由偏离不在本票，归 [35](./35-network-routing-route-deviation-consumes-movement-facts.md)。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 生产方与消费者在同一个变更。
- 哨兵登列。
- 阻断读数派生异常。
- 真库测试。
- 演示：出发后 `/tracking-projections` 出现 DEPARTED。

## 规模

M

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg33`）

### 接票时核实

- 票 32 已进 main：出发一律落册、读数与事实同一行；`buildMovementFactOrchestration` 不向任何下游交意图，头注写明信封随本票上线。
- VE 的信号只有一条生产路径：投影派生里无法裁决的冲突，按租户登记的冲突信号规则（`PAR-VIS-04`，VE 0025）形成，未配置如实不形成。VE CONTEXT「异常识别」与 UC-VE-001 都要信号「依据版本化异常类型和识别规则」；`PAR-VIS-04` 的 ADR-0146 拆分把「通用异常类型目录与识别条件形态」归参考配置、「启用哪些类型」归租户取值。
- 移动事实本身不带对象：对象只在出发读数的逐对象那一格里（段停点、班次停点时没有），移动与到达根本没有。
- 演示脚本的出发（票 32 补记）经作业事实渠道带 `segment` 报；一线客户端不报 `segment`（ADR-0205 越权风险点 6）。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

权威落在 ADR-0205 文末补记（越权风险点续 14–23；22、23 由评审补登）。这里逐条记本票自决的几处；进了 ADR 的写明补记风险点几，只在票面的是装配与种子细节。

1. **出发、移动、到达都交信封，不只出发。** 方案只说「移动事实已形成信封」，追踪要的不只门禁。引用式三键、指纹 ID、分区「租户/移动事实」；同内容重投重交同一份；交出失败答未决，续办引用与登记册读不动分开。
   - 越权风险点：移动与到达也进投影、未归类（补记 19，VE owner）。
2. **类型词按种类三个**：`movement-departure` / `movement-in-transit` / `movement-arrival`；读数不进类型词，阻断由信号表达。
   - 越权风险点：词由本票代 TF owner 定，日后改名要迁映射（补记 18）。
3. **对象沿声明的段取事实发生那一刻在控的对象**，与决定四同一口径（`ObjectsUnderControlAt` 一处定义），三种事实同一条；按消费那一刻的段册算。没声明段、段不在册、那一刻没人：入账不派生；段册读不动：未决。
   - 越权风险点：段日后才登记的不回补（补记 14，另立票 53）；消费时段册可能与登记时不同（补记 15）；袋当包裹投（补记 20，等票 42）。
4. **阻断读数派生信号走租户登记的新规则册。** 识别条件（「出发读数里这一件阻断」）归产品；类型、规则版本、可信度与启不启用归租户——新立 VE 迁移 0027 `departure_gate_signal_rule`，形同冲突信号规则（一租户一条、不可覆盖），未配置如实不形成。不复用冲突信号规则册：两册语义不同，共一行就把两类异常绑成一种。不内置默认规则：ADR-0146 决定三否决 opt-out 默认。只看逐对象那一格；命中时刻取判断时刻（出发信封各占一个分区不保序，按业务时间记命中会让活跃发作期的命中时刻倒退、被领域拒）；更正版不再命中；信号与投影同一笔事务，搁置即整封回滚。
   - 越权风险点：只开了受控 CLI，在线登记口与目录查阅面缺（补记 16，另立票 52）；只有阻断形成信号，读不动的出发不报（补记 17）。
5. **路由只投 VE，不 FanOut**；未决名单只登四格（事实读不回、段册读不动、信号搁置、派生未决）。NR 路由偏离归票 35，接得住之前不挂。
6. **参考映射只把出发映成 `DEPARTED`**，演示映射 `SYN-VE-MAP-V1` 就地加一行（同票 11、13）。
   - 越权风险点：已灌库要 `--reset`（补记 21，演示租户 owner）。
7. **演示租户采用一条信号规则**：`DEPARTURE_GATE_BLOCKED` / `SYN-VE-DEPARTURE-GATE-V1` / `SOURCE_CONFIRMED`（`visibility/13`）；分诊规则 v1 不加这一类，信号进人工复核。
   - 越权风险点（演示租户 owner）：要演示自动建案得改分诊种子 v1（就地改要 `--reset`），本票没改；演示动线上 SHA→SZX 读数不受门禁，这条规则在动线上不命中。
8. **派发装配另立侧文件 `cmd/parcel-dispatch/assemble_movement_fact.go`**，`assemble.go` 只追加 6 行（构造一行组、路由一行）。阻断信号的租户现绑包装借投影派生那一份发作期库、身份签发与分诊意图交出，信号编排在那里另构造一次。
   - 越权风险点（运维）：分诊编排的构造在 `tenantBoundProjectionDerive.Handle` 与侧文件各写一份；为守热点文件只追加，没把既有那一份抽出来。
9. **迁移号**：VE 用 `0027`（main 上 VE 最大号 0026 的下一号，号表没给 VE 预分）；TF 预分的 `0033` 未用——信封走框架 Outbox，不需要 TF 迁移。

### 做了什么

- TF：新端口 `ports/movement_fact_handoff.go`；新适配器 `adapters/postgres/movement_fact_handoff.go`（`OutboxMovementFactHandoff`，导出 `MovementFactRecordedEventType` 供派发用例对照）；`application/record_movement_fact.go` 的 deps 加 `Handoff`，登记成立与同内容重放交意图、交出失败答未决；分区主体登记表加一行。
- VE：
  - 新消费门 `adapters/inbox/movement_fact_consumer.go`；
  - 新派生适配器 `adapters/transportfulfillment/derive_on_movement_fact.go`（六个哨兵）；
  - 新编排 `application/raise_departure_gate_signal.go`；
  - 新端口 `ports/departure_gate_signal.go`、postgres `departure_gate_signal_rule.go`（视图 + `CatalogRegistrar` 写口）、迁移 `migrations/visibility_exception/0027_departure_gate_signal_rule.sql`；
  - `CatalogRegistration.RegisterDepartureGateSignalRule`（`CatalogRegistries` 合成口加这一册），`registrationjson.DepartureGateSignalRuleFromJSON`，`parcel-ve-register departure-gate-signal-rule`。
- 装配：`cmd/parcel-api/assemble_movement_fact.go` 接 Outbox 交出（`main.go`、`endpoints.go` 未动）；`cmd/parcel-dispatch/assemble_movement_fact.go` 新文件（消费者、未决名单、租户现绑信号包装），`assemble.go` 追加构造与路由一行。
- 种子：`visibility/01` 加出发一行；新 `visibility/13`；`seed.sh` 追加一行；种子 README 三处。
- 文档：ADR-0205 文末补记；UC-TF-005 `AT-TF-156`–`161`；演示脚本补记一条；AT 覆盖盘点六行入①与追记；立票 [52](./52-departure-gate-signal-rule-has-no-online-registration-or-catalogue-read.md)、[53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)。

### 用例

- TF postgres `movement_fact_handoff_test.go`（真库）：事实与信封同提交同回滚、载荷恰三键、ID 定长且异于分区键、重发不出第二封；两版同分区；无事务与缺键拒。
- TF 应用 `record_movement_fact_test.go`：登记成立交出落册那一份、重投重交、冲突不交；交出失败答未决且续办引用与登记册读不动分得开，续办重投补上。
- VE 消费门（真库）：恰一次、三键毒丸、失败回滚重投、外来类型响亮、与交接账本分家。
- VE 适配器：出发投到段里那一刻在控的每件（出发后入段的不投）、只把阻断那件交信号；移动与到达各自类型词；没声明段（不读段册）、段不在册、那一刻没人都入账；各格哨兵；更正版带替代关系、不再交信号；缺件构造拒。
- VE 编排 `raise_departure_gate_signal_test.go`：登记规则即形成发作期（三样取登记、时刻取判断时刻），再命中同一发作期；未配置不问分诊；规则读不动与分诊未决都搁置。
- VE postgres（真库）：规则册登记前未配置、登记后原样读回、不可覆盖、跨租户与空租户不可见；空白列库拒；无事务登记拒。登记用例、翻译、CLI 表与 CLI 真库纵向用例各补一册。
- 派发 `movement_fact_projection_test.go`（真库）：生产方与消费方类型串相等；经生产登记编排登出发、生产路由表一拍定稿，两件按演示种子映射归 `DEPARTED`（把种子那一行删掉这条转红，实测）；演示种子的信号规则登上后阻断那件形成发作期、分诊进人工复核，另一件没有；没登规则不形成；失败分格取名单本身；路由表毒丸定稿。
- 装配 `cmd/parcel-api/movement_fact_handoff_test.go`（真库）：演示动线同一条线上，出发登记的那一笔同时入队信封，重投不出第二封。

### 验证

- 于 `f6d6beb7`：`go build ./...`、`go vet ./...`、`gofmt -l`（全部已跟踪 Go 文件）干净。
- 于 `f6d6beb7`：`IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1` 全绿：`./internal/architecture/`、`./internal/transportfulfillment/...`、`./internal/visibilityexception/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-ve-register/`、`./migrations/`、`./scripts/demo-seeds/...`。
- 种子：一次性库 `dcg33_seed` 上 `seed.sh` 干净灌，退 0，`departure-gate-signal-rule: REGISTERED`；用完删库。
- 演示判据「出发后 `/tracking-projections` 出现 `DEPARTED`」由生产登记编排与生产路由表上的真库用例取证（只记 `S`），没有起进程按演示脚本重走。
- 未跑全量，未重生成机制清点。

### 偏离与待裁

- **地盘外的改动**：
  - VE 登记一族：`application/register_catalog.go`（+test）、`adapters/http/register_catalog_test.go`（替身补一法）、`adapters/registrationjson`、`adapters/postgres/transaction_guard_test.go`、`cmd/parcel-ve-register`（+两份测试）——信号规则册要有登记入口，才是「租户取值可配置」而不是死表。
  - 构造 `RecordMovementFactDeps` 的几处测试跟着补 `Handoff`：TF `adapters/http/record_movement_fact_test.go`、`adapters/postgres/storable_values_regression_test.go`、`cmd/parcel-api/movement_fact_same_identity_test.go`（operator-channel/48 在改 `cmd/parcel-api` 测试注释，这一份若撞上，只是相邻行）。
  - `internal/architecture/partition_subject_registry_test.go` 一行；`.scratch/at-coverage-inventory.md`。
  - 没碰 peer 地盘（`internal/nodeoperations/**`、`work_fact_intake*.go`、交接 / 有效交付 / 派送各文件、`apps/admin-web`），没碰 `main.go`、`endpoints.go`、`docs/adr/README.md`。
- **号**：`AT-TF-156`–`161`（预分 156–165）；立票 52、53（预分 52–53）；VE 迁移 0027（非预分，按「其余上下文用 main 最大号的下一号」）；TF 迁移 0033 未用。
- **合并预检**：对 `origin/main`（`1734f2a0`）`git merge-tree` 只有 `.scratch/at-coverage-inventory.md` 一处冲突（追记与行，与别票并行），按惯例两边都留、计数重算；`cmd/parcel-dispatch/assemble.go` 与分区登记表自动合入。
- **派发令牌**：方案次序 dcg08 → dcg11 → 26 → 18 → 33；票 18 若同改路由表，本票那一行与它相邻时按两边都留合。波 3 落地顺序 20 → 24 → 18 → 33 → 29（方案「并行车道」），票 18（`claude-dcg18`）进 main 之前本票不落。
- **待裁**（都已登进补记风险点）：信号规则册的在线面（票 52）；段后登记的事实补投（票 53）；读不动的出发要不要另一类信号（补记 17）；演示要不要演示自动建案。

## 评审记录（2026-10-19，隔离评审一轮；作者子代理修补）

阻断 0 项。非阻断 9 项，其中建议进 main 前修的 4 项。

### 修了的

1. **`cmd/parcel-ve-register` 包头「后两册」按位置指代失配**（建议进 main 前修）。本票在列表末尾追加出发门禁阻断信号规则一行，「后两册」就落到了别的册上。改为点名 `exception-disclosure-rules` 与 `conflict-signal-rule`（连同「由票 ve-disclosure-policy-view/02 接入」那句一起点名，原先括号里的「接入后两册」也是位置指代）；`departure-gate-signal-rule` 另起一句，写明还没有目录读口的 kind（票 [52](./52-departure-gate-signal-rule-has-no-online-registration-or-catalogue-read.md)）。
2. **出发门禁阻断信号的发作期永不结束**（建议进 main 前修）。核实属实：生产代码里没有任何地方调 `SignalEpisode.End`，`RaiseSignalHandler` 对活跃发作期只记命中、不再分诊。ADR-0205 补记登**越权风险点 22**（VE owner）：规则册没有恢复边界，同一件后续的违规出发只记命中、不再分诊；是加恢复边界，还是让因果条件带上出发事实、每次违规各开一期，归 VE owner 裁。纯文档，不改代码——恢复边界的形状是 VE CONTEXT 层面的取舍（冲突信号同缺，属存量），不该由本票顺手定。
3. **`record_movement_fact.go` 两处注释与实情不符**（建议进 main 前修）。改成实情：续办引用按停在哪一口分开，同一口重投引用不变；引用不透明、结果不带原因码，单看引用分不出是哪一口。交出失败那一格写明答未决即 2xx 出队（ADR-0022），同一份再报才补上；真库上入队失败把整笔带进中止态，随装配点提交失败上抛、事实一并回滚。ADR-0205 补记「信封」一条与 `AT-TF-156` 同步这一句。没有照收费发生项那样在结果里带出原因码：那要在移动事实口的线格式加一格，HTTP 适配器归作业事实族（peer 在途地盘相邻），本票只把注释改对。
4. **信号不带事实依据**（不必进 main 前修，顺手登）。补记登**越权风险点 23**（VE owner）：`RaiseSignalCommand` 没有事实依据这一格（存量），日后加时这一路带上移动事实引用与版本、逐对象那一格的关务判断标识。
5. **AT-TF-157–161 挂在 TF 用例、VE owner 从自己的用例找不到**（不必改，顺手做）。UC-VE-002、UC-VE-004 验收表下各加一行指向 UC-TF-005 的对应 AT 号，UC-VE-004 那行另指向风险点 22、23；没有迁号——另编 `AT-VE` 号要等 VE owner 复核，号段也不在本票预分范围内。

### 没在本分支改的及去向

6. **进 main 流程**（建议进 main 前做，不属作者这一步）：按方案波 3 顺序等票 18 先落；合并时 `.scratch/at-coverage-inventory.md` 两边都留、计数重算；全量测试钉 SHA，重生成 `MECHANISM-INVENTORY`。已写进上文「偏离与待裁」的派发令牌一条。另：修补后复做预合并，对 `origin/main`（`a2ff42f6`）仍只有 at-coverage-inventory 一处冲突；与 `claude-dcg29`（`ed993ac8`）在 UC-TF-005 验收表有一处冲突——两边都在 `AT-TF-125` 之后追加行，按号序两边都留（156–161 在前、166–170 在后）即可，修补前的 `4e302b24` 就已如此，不是本轮引入；与 `claude-dcg18`/`20`/`24` 无冲突。
7. **`tenantBoundDepartureGateSignals` 与 `tenantBoundProjectionDerive` 的分诊编排构造逐字重复**：已在代决 8 登为运维风险点；去向是装配令牌（派发令牌）空出来后，在 `tenantBoundProjectionDerive` 上抽一个按租户构造信号编排的方法两处共用。本票守热点文件只追加，不在本分支动既有那一份。
8. **三个 VE 适配器哨兵包住了全部 error（含装不回的确定性缺陷）**：存量模式，与交接、交付各 VE 适配器及冲突信号规则视图同形；去向随 ADR-0205 越权风险点 2——TF 仓储把装不回与连不上分开时一起收窄。
9. **`docs/adr/README.md` 上 ADR-0205 两行并存**：票 32 合并时留下，不在本分支 diff；README 是跨票热点只追加行，删行不归本票。去向：进 main 时由协调方作文档小修删旧那一行。没有立票：本票预分的票号 52、53 已用完，自取号会与同波撞号。

### 修补后验证

- `go build ./...` 干净；`go vet`（`internal/transportfulfillment/application`、`cmd/parcel-ve-register`）与改动 Go 文件的 `gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1` 全绿：`./internal/architecture/`、`./internal/transportfulfillment/application/`、`./internal/transportfulfillment/adapters/postgres/`、`./internal/visibilityexception/...`、`./cmd/parcel-ve-register/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`。
- 演示种子本轮未改，未重灌。未跑全量，未重生成机制清点。
