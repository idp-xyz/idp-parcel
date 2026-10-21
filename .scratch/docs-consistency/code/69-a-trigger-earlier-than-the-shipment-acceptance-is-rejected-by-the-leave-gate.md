# 69 业务时间早于委托接受的复核触发撞离场门，整封信封响亮失败

Category: bug
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code69。R2（上线后尽快；若某租户首发动线里「先收后受理」加触发晚处理是常态则升 R1）。2026-10-21 协调方据 code/60 越权风险点 1 与其隔离评审建议立，同日给裁向。
Blocked by: 无（裁向已给；实现与 ADR 由同一张票落）
出处：code/60 完成记录越权风险点 1；code/60 隔离评审 risk1_recommendation。

## 现象

code/60 之后初始路由从委托接受时刻起生效。触发的业务时间早于接受、信封却在初始路由形成之后才处理（积压、离线补登、来源时间写早了），而复核又判出失效或替代时，离场门按「离场不早于生效」拒，整封信封响亮失败，重投到 ABANDONED，无绕法。

接受之前就处理到的收寄（PS 答 `REQUEST_NOT_ACCEPTED`）与交接（NR 按 ADR-0215 入账）不撞；只有「早于接受、晚于初始路由形成才处理」这一格撞。

## 证据

- `internal/networkrouting/application/create_initial_route.go`：`planRecord` 的生效取 `RouteHandoff.AcceptedAt()`（code/60）。
- `internal/networkrouting/application/reassess_route.go`：失效线 `Lapse`、改善替代 `Supersede` 都以触发业务时间离场（ADR-0272）；领域 `leaveCurrent` 头注「离场必须带依据与不早于生效的时刻」。

## 三类

机制。

## 要裁的（协调方裁向，2026-10-21）

取「入账为`未判`、不判」，否决「离场夹到生效时刻」（ADR-0272 已否决同形候选，`AT-NR-064` 钉住失效时刻取触发业务时间）与「PS 采用时拒」（所有权错、且盖不到 TF 交接那一路）。依据：ADR-0235 决定四（早于当前位置的触发入账不判，「业务时间不会变，重投永远推不动」）、ADR-0174、ADR-0250 先例。

落法：新立未判原因（例如「触发早于委托接受」，与 `TRIGGER_PRECEDES_CURRENT_POSITION` 分开）；下限取这一判断键的接受时刻（不取 `plan.EffectiveFrom()`，免得「原先无当前有效路由」那条线形成一份早于接受就生效的首个计划）；接受时刻由收寄与交接两路适配器从 PS 委托聚合的 `AcceptanceBaseline().FixedAt()` 带进来。新 ADR 写明对 ADR-0235 决定一（未判原因封闭集）与决定四的作用，扩成也算部分停用，照 `docs/adr/README.md` 规则挂指针；越权风险点记「那次触发的硬约束检查被跳过，要等下一次触发再判」。

## 规模

M：一份 ADR、一个未判原因、两路适配器带接受时刻、编排在比较当前位置前先比接受时刻、编排与派发真库用例。

## 完成记录（2026-10-21，作者会话，分支 claude-code69，基于 main `cbcaaa873`）

### 做了什么

- 新立 [ADR-0279](../../../docs/adr/0279-a-reassessment-trigger-earlier-than-the-shipment-acceptance-is-settled-as-not-judged.md)，照协调方裁向：复核的业务时间下限取这一判断键的委托接受时刻（接受基线固定的那一刻）；触发严格早于它时答`未判`，原因 `TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE`，未判登记另记比较所用的接受时刻；不读路由历史、不判。同一时刻照判。
- 编排 `ReassessRouteHandler.Handle`：已有结果分界之后、比当前位置之前先比接受。`notJudged` 改收一份 `ports.NotJudgedTrigger`，比较所用的时刻由调用方按原因填，入账时刻仍在那里取。
- 领域 `ReassessmentTriggerSpec.AcceptedAt`（缺即触发不受理）与 `ReassessmentTrigger.PrecedesAcceptance`。接受时刻不进触发指纹。
- 端口 `NotJudgedTriggerPrecedesShipmentAcceptance`、`NotJudgedTrigger.ShipmentAcceptedAt`；postgres `RecordNotJudged` 按同一条 iff 先拒一次。
- 迁移 `migrations/network_routing/0024_route_reassessment_not_judged_before_acceptance.sql`：原因封闭集加一格，加 `shipment_accepted_at` 列与 CHECK（只在这一格原因下有值，触发 `<=` 它）。
- 两路适配器带接受时刻，取值都是 PS 委托聚合 `AcceptanceBaseline().FixedAt()`，只经 NR 已有的 PS 读口，没改 PS 代码：
  - `AcceptedCarriedGeo` 加 `AcceptedAt`，`AcceptedCarriedGeoSource` 读回时一并取，基线没有固定时刻答 `ErrAcceptedRequestInconsistent`。
  - 交接一路 `triggerSpecFor` 取它。
  - 收寄采用一路 `ReassessOnNetworkIntakeAdapter` 多一个 `AcceptedParcelLookup`：`采用`时按（租户，包裹）读回，核对委托与基线就是采用记录指着的那一份；读不动交 `ErrAcceptedParcelUnreadable`，读不着或对不上交 `ErrAcceptedRequestInconsistent`；`不采用`不去读。`ReassessOnIntakeAdapter.TriggerReassessment` 多一个接受时刻参数。
- 派发装配：`networkIntakeConsumer`（`assemble.go`）接上 PS 委托仓储与 `AcceptedCarriedGeoSource`；`routes_network_intake.go` 复核那一路的未决哨兵加 `ErrAcceptedParcelUnreadable`。交接那一路装配不变。
- 派发真库夹具 `reassessHandoverAt` 改为真实时钟之后四天的整点：夹具的接受决定按真实时钟形成，原先的固定时刻早于接受，改后那批用例的触发全落在接受之前、会入账不判。各用例相对它的偏移不变。
- 积压夹具抽成 `backloggedInitialRoute`（`initial_route_backlog_test.go`），`AT-NR-086` 那条用例行为不变。
- 文档：NR CONTEXT「路由稳定性与受控改路」复核触发排序那条补一句引 ADR-0279；UC-NR-003 结果表`触发未判`补这一格与另记的接受时刻、验收立 `AT-NR-087`；ADR-0235 只改 Status 索引注记与 Links；code/60 越权风险点 1 注「已补」；AT 盘点①加一行、文末加追记。ADR 索引、AT 计数与机制清点没重生成。

### 代决

1. **未判原因单立一格，码值 `TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE`**，与 `TRIGGER_PRECEDES_CURRENT_POSITION` 分开；登记册另立 `shipment_accepted_at` 一列，不与 `current_position_at` 合用。理由：两格比的是两样东西，合用就读不出早于谁。
2. **先比接受，再比当前位置**；两格都中时记接受那一格。
3. **对 ADR-0235 的作用写成部分停用（扩成）**：决定一「重投推不动的答`未判`……」一句（原因封闭集加一格）与决定四「复核在已有结果分界之后、读路由历史之前比较……」一句（比较之前先比接受）。两句均逐字引，适用场景是收寄采用与交接两路、触发严格早于判断键的委托接受时刻。ADR-0272、0250、0174 判为无条款停用。
4. **收寄采用一路读 PS 的不一致按不变量破处理。** 读不着、读到另一份委托或另一条基线，都不进未决哨兵：采用以当前已接受委托为前提（ADR-0060），已接受是终态。
5. **登记册 CHECK 用 `<=`。** 编排按纳秒比；PS 的接受时刻带纳秒，落进 timestamptz 只留到微秒。纳秒上早于、同一微秒里的那一次两格相等，用 `<` 就写不进，会回滚重投到 ABANDONED。
6. **派发夹具时刻改为相对真实时钟**，不改 PS 夹具的接受时钟：PS 判断的 asOf 也取真实时钟，动接受时钟会牵连 PS 一侧的校验。
7. **`AT-NR-087` 号自取。** 取号时查过全仓与全部本地分支，`AT-NR-087` 起无人用。

### 越权风险点

照 ADR-0279「越权风险点」：

1. 那次触发的硬约束检查被跳过，要等下一次触发再判（NR owner）；最后一个节点上再无触发，补法归票 154。
2. 先收后受理成常态的租户会整片跳过首个节点前的硬约束检查（NR、PS owner），那时本票升 R1。
3. 收寄采用一路多依赖一次 PS（NR、PS owner）。
4. 业务时间与接受时刻都取自来源（NR、PS、TF owner）。
5. 票 code/60 之前落库的初始计划不受这道下限保护（NR owner），演示库 `--reset` 重灌。

### AT

UC-NR-003 立 `AT-NR-087`。Covers 指它的用例：

- 领域：`TestATriggerPrecedesTheAcceptanceOnlyWhenItsBusinessTimeIsStrictlyEarlier`；
- 编排：`TestATriggerEarlierThanTheShipmentAcceptanceIsNotJudged`（原计划本会失效、本会被替代、原先无当前有效路由不形成首个计划、同一时刻照判）、`TestATriggerEarlierThanTheAcceptanceIsComparedBeforeTheCurrentPosition`；
- 未判登记：`TestATriggerBeforeTheAcceptanceCarriesTheAcceptanceItWasComparedWith`；
- 两路适配器：`TestAnIntakeEarlierThanTheAcceptanceSettlesNotJudged`、`TestAHandoverEarlierThanTheAcceptanceSettlesNotJudged`；
- 两路真库：`TestAHandoverEarlierThanTheAcceptanceIsNotJudgedOnTheDatabase`、`TestAnIntakeEarlierThanTheAcceptanceIsNotJudgedOnTheDatabase`；
- 另有收寄采用一路读委托的分格 `TestTheAcceptanceReadOfAnAdoptedIntakeIsSplitByRecovery`，取接受时刻 `TestAnAcceptedParcelAnswersItsJudgmentKeyAndTheBaselineVersionsGeo` 加断言。

### 先跑红

- 编排：修前「本会失效」答 `lapse plan applicability: network routing: invalid plan applicability`，「本会被替代」答 `supersede plan applicability: …invalid plan applicability`，「原先无当前有效路由」答 `FIRST_PLAN_FORMED`，「先比接受」答 `TRIGGER_PRECEDES_CURRENT_POSITION`。
- 派发真库：去掉比接受那一道后，两路投递都答 `lapse plan applicability: network routing: invalid plan applicability`，就是本票的现象。

### 验证

- `go build ./...`、`go vet ./...` 干净；改动的 Go 文件 `gofmt -l` 为空。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/networkrouting/... ./cmd/parcel-dispatch ./cmd/parcel-api ./cmd/parcel-network-register ./internal/parcelshipment/adapters/networkrouting ./internal/transportfulfillment/adapters/networkrouting ./internal/visibilityexception/adapters/networkrouting ./internal/architecture/... ./migrations/... ./internal/platform/migrate/...`：退 0，22 个包 ok。
- 新用例与改过断言的用例 `-count=3`（同一脚本，六个包）：全过，36 次顶层 PASS。
- 迁移：一次性库 `code69seed` 随 `scripts/demo-seeds/seed.sh` 全量灌入，退 0。另在同一库上回退 0024、按旧封闭集插入四种既有原因各一行，再单独施加 0024，成功，四行 `shipment_accepted_at` 皆空。用完 `DROP DATABASE … WITH (FORCE)`。
- 收口自检：scratchpad 副本上 `go run ./tools/adr-index`、`-check` 均退 0；两句被停原句在 ADR-0235 正文里逐字存在；ADR-0235 除 Status 行（原行是新行的前缀）与 Links 新加一行外逐字节不变。
- 没跑全量；没重生成 ADR 索引、AT 计数与机制清点，留推送方。

### 评审后修订（2026-10-21，作者会话，rebase 到 main `3e3b08300`）

- 隔离评审无阻断；协调方核过 `assemble.go` 那一处装配，改的是已有的消费者构造，不违背 ADR-0222。
- ADR-0279「对已接受记录的作用」补「ADR-0249：无条款停用」，Links 补一行，0249 那一侧不挂指针。ADR-0235 注记改写为「已由 [ADR-0279](…) 停用（扩成）」。
- `networkIntakeConsumer` 头注的链条补「读回已接受委托取接受时刻」一步；派发测试注释去掉变更叙述。
- 迁移 0024 头注改写两边时刻的精度，约束名改为 `route_reassessment_not_judged_acceptance_iff_not_after`。协调方给的前提是「两边都从库里读回、只到微秒」，核下来只对了触发那一边：触发的业务时间读自交接册、收寄采用册的 timestamptz 列，只到微秒；接受时刻读自 PS 委托文档 jsonb 里的 `fixedAt` 时间串，带纳秒（`pspostgres` 委托文档的 `FixedAt` 字段，真库用例读回的值实测带纳秒）。所以 `<=` 不只是防御：触发恰在接受那一微秒里时，`<` 会写不进。
- AT 盘点 rebase 冲突取并集。
- 验证：同一组包重跑退 0，22 个包 ok；迁移改过，在一次性库上随种子重灌一遍，退 0，新约束名在库里，用完已删。scratchpad 副本上 `adr-index`、`-check` 退 0；ADR-0235 正文除 Status 行与 Links 外逐字节不变。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-code69` tip `764fd9b3d` 在 main `3e3b08300` 上快进；隔离评审无阻断，小改已收。
- 协调方在 tip 上单独提交重生成的 ADR 索引（新 ADR-0279、0235 注记）、AT 计数（AT-NR-087）与机制清点，检查均退 0。
- 全量 `scripts/test-full.sh` 退 0，158 个包 ok；`go build ./...`、`go vet ./...` 干净；演示种子在一次性库上灌通（迁移 `network_routing/0024` 一并施加），用后删库。
- 去向：ADR-0279 越权风险点 1（从接受时刻的位置补判一次）归 demo-chain-gaps/154。
