# 23 渠道择优决定的运营查阅面：留痕落库了，运营今天没有地方看它

Category: enhancement
Status: resolved——MCP-2 2026-09-04 落地于分支 `mcp2-lc23`（起于 `eba019a8`，完工前 rebase 到 main `9e4e90bb`；待 MCP-1 重放入 main，共享接线五件的整合行见文末「完成记录」与同目录 `lc23-integration-lines.patch`）：读端口二口 + postgres 读口（登记册适配器兼两个契约）+ 查阅端点 `GET /channel-selection-decisions`（列表 / 单份两分支）+ cmd 读口装配与真库装配用例 + 管理台「渠道择优决定」页；**端点表行、探针、隔离读放行行归 MCP-1 落**，落行前 `internal/architecture` 的管理台路径门禁在本分支如实红。认领记录：MCP-2 2026-09-04 认领（基线 main `eba019a8`，task-7f3d6450）；MCP-1 同日代裁 draft → ready-for-agent：票面「做什么」四条与「先答再开工」两条的本票倾向即裁决（读端口另立二口、不拓宽登记册端口；postgres 读适配器只读列面、读回过重建门；人工裁决动作不进本票；不按时间隐式截断；端点命名照 PS 既有读面）。立票原句：随票 14 收口立票，只写票面未动代码
Blocked by: 无（14 已 resolved，八笔随 `19cf2ce5` 进 main）

## 缺口

票 [14](./14-rejected-candidate-trace-object.md) 把「渠道择优决定」记成只追加的记录（领域对象、
`ports.ChannelSelectionDecisionRegistry`、`parcel_shipment.channel_selection_decision` 头行 +
`channel_selection_candidate` 子行、择优编排落定后同事务写入）。它今天只有**写**与一个按对象列
历史的读口（`ListBySubject`，给同一对象的择优历史用）——**没有任何运营面能看见它**：哪些择优停在
并列冲突等人工裁决（`PAR-NET-16`「交人工裁决」那一格）、某个候选最近为何总是出局、某笔映射下
这一天做过几次择优。

票 14 的裁决原句：「谁读它：运营查阅面，沿 ADR-0077 读面通例另立读口与票；本票只落登记。」
本票就是那张票。

## 做什么

按 [ADR-0077](../../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)
读面通例：

1. **读端口另立**（`ports` 下伴生读端口，不拓宽 `ChannelSelectionDecisionRegistry`——理由同
   `internal/parcelpricing/ports/evaluation_read.go` 头注：扩写既有接口会拆全部替身）。至少两口：
   - 按租户列**并列冲突待人工**的决定（`conclusion = TIED`），按决定时刻倒序、可按对象收窄；
   - 按（租户 + 决定标识）取一条决定的逐候选结果（含出局因由与所用评价引用）。
2. **postgres 读适配器**只读列面（头行 + 子行），不在 SQL 里解释四格之外的任何语义；读回照旧过
   `RehydrateChannelSelectionDecision`。
3. **端点**进 `cmd/parcel-api/endpoints.go`（共享接线文件，先在频道占号）+ 隔离读放行表按
   ADR-0078 判是否入格。
4. **管理台**页：落点先取证——面单交易/包裹侧读面今天有哪几页、择优对象引用（商业范围 + 产品—渠道
   映射）在哪一页显得出来，登在哪里看得见就摆哪里（票 admin-write-faces/02 「写签跟着读签走」同一条
   判据反过来用）。

## 先答再开工

- **并列冲突的人工裁决动作要不要同票**：裁决人与裁决规则属实例半边（`PAR-NET-16` 待提供列「并列时的
  裁决规则和授权」），机制侧「人工裁决」怎么落——是形成一条新的决定记录（规则引用换成人工裁决那一格）
  还是别的形状——**要先 `/domain-modeling`**，不在读面票里顺手定。本票倾向：读面只列冲突，不给动作。
- **保留期与读面**：`PAR-NET-16` 留痕要求待提供，读面不做任何按时间的隐式截断；「近期」之类窄口由
  查询参数显式给。

## 红线

- 不拷候选内容与金额进读面：金额要看去 `parcel-pricing` 的评价（读面只透评价引用）。
- 不填任何实例取值；SYN 夹具只记 `S`。
- 不开任何行级 UPDATE/DELETE；读面对登记册零写入。

## 完成判据

两口读端口有 postgres 实现与真库用例（含租户隔离与「从未择优过」答空）、端点进表且未配置态按
ADR-0055 作答、管理台页面能列并列冲突并展开逐候选结果；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

票 [14](./14-rejected-candidate-trace-object.md) 裁决节；`internal/parcelshipment/ports/channel_selection_decision.go`；
`migrations/parcel_shipment/0015_channel_selection_decision.sql`；[ADR-0077](../../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)；
参数登记册 `PAR-NET-16`。

## Comments

- 2026-09-04 · MCP-2：立票。起因是票 14 裁决把读面划出登记票之外，收口时按裁决原句另立。
  **只写票面，未动代码。**
- 2026-09-04 · MCP-2：**管理台落点取证**（「做什么」第 4 条，取证于 main `eba019a8` 的 `navigation.ts`）。面单交易 /
  包裹侧今天在管理台「委托受理」区的页面：提交与撤回（`shipment-request`）、委托查阅（`shipment-request-inquiry`）、
  接受前人工复核（`acceptance-review`）、面单交易（`label-transactions`）、取消与收寄后处置（`cancel-parcel`），
  全部主责 parcel-shipment。择优对象引用的
  两半：**产品—渠道映射引用**在主数据区「渠道产品目录」页（`channel-product-catalog`，party-commercial，行对象是
  映射修订、列 `mappingId`）显得出来；**商业范围引用**今天没有任何页面显它——它是择优编排入参里的不透明引用
  （`domain.CommercialScopeReference` 注释：范围取值属实例半边，本上下文只要求它被指名）。裁：**页面落「委托受理」
  区、挨着面单交易**——决定是 parcel-shipment 形成并登记的判断记录，读面随所有权走；挂到渠道产品目录页会把 PS 的
  记录摆进 PC 的目录页，一页两主责。对象引用两列以等宽文本原样透出，页头一句指明映射本体在渠道产品目录页；
  收窄入口收两个引用的字面（两个都给才收窄，服务端只给一半答 400）。导航 id `channel-selection-decisions`，
  `liveIds` 登记为已接线（页面对真实端点发请求；接入渠道未配置时如实渲染 403）。
- 2026-09-04 · MCP-2：**双轴评审**（子代理认证失败，本会话串行隔离自评）。Standards 轴修三处（票面取证句去掉
  别处计数并锚 SHA、装配注释去掉「零命中」计数、前端 `DecisionDetailState` 删永不产生的 `idle` 变体），两处留痕不改：
  ① postgres 读口的头行扫描与子行装载与既有 `ListBySubject` 同形——那份文件归票 14 收口且本票「只加不改」，折回一套
  helper 是后继小票；② 前端 `wordOf` 与 `operations/presentation.ts` 的 `labelOf`、面单交易页的 `wordOr` 同形——
  各目录自持一份是既有先例，不在本票抬成共享模块。Spec 轴：「做什么」四条与「先答再开工」两条逐条对上，无越界；
  `selectedCandidate` 与列表行携带逐候选结果是「整条决定」的一部分，不算加功能。

## 完成记录（MCP-2，2026-09-04）

**落点（分支 `mcp2-lc23`，起于 `eba019a8`、完工前 rebase 到 main `9e4e90bb`；SHA 为 rebase 后分支上的，重放入 main 后由
MCP-1 对照新旧 SHA）**

| 层 | 文件 | SHA |
|---|---|---|
| ports | `internal/parcelshipment/ports/channel_selection_decision_read.go`：`ChannelSelectionDecisionRead`（`ListTiedChannelSelectionDecisions` + `FindChannelSelectionDecision`）、`TiedChannelSelectionFilter`（`EveryTiedChannelSelection` / `TiedChannelSelectionsOf`）；不动 `ChannelSelectionDecisionRegistry` | `1fc3d859` |
| postgres | `adapters/postgres/channel_selection_decision_read.go`：方法加在既有 `ChannelSelectionDecisions` 上（只加不改既有文件），`conclusion = 'TIED'` 字面取自领域封闭集 `String()`，倒序 + `LIMIT`，子行 `ANY($2)` 一次取回，读回照旧过 `rehydrateChannelSelectionDecision`；真库用例四条（倒序只列 TIED 且跨租户不可见、按对象收窄与截页、从未择优答空切片 + limit 非正拒、按标识只在本租户内取得到） | `1fc3d859` |
| http | `adapters/http/query_channel_selection_decisions.go`：`GET /channel-selection-decisions`，`decisionId` 在场走单份分支（404 `CHANNEL_SELECTION_DECISION_NOT_VISIBLE`，不带 outcome），否则列表分支 `view=tied` 必备、`scope`/`mapping` 成对收窄（只给一半 400）；`ChannelSelectionDecisionQuery{Tenant, Limit}` 与 `ChannelSelectionDecisionQueryIntake` 另立（只有租户维，理由同面单交易）；`UnconfiguredIntake` 与 `IsolatedOperationsReadIntake` 的对应方法随端点放在本文件；传输层用例六条（含无金额断言、未配置两分支同答 403、隔离读只交注入租户） | `7283ce3d` |
| cmd | `cmd/parcel-api/assemble_channel_selection_decisions.go`：`buildChannelSelectionDecisionRead(db)` 交回登记册本尊作读口；真库装配用例落一条 TIED 一条 SELECTED，经隔离读 Intake 列表只列 TIED 且逐候选四格与因由带回、按标识取回选中者、不存在 404、同一读口挂 `UnconfiguredIntake{}` 仍 403 | `687be091` |
| admin-web | `apps/admin-web/src/pages/channel-selection/{api.ts, channel-selection-decisions.ts, channel-selection-decisions.test.ts, ChannelSelectionDecisionsPage.tsx, index.ts}`；`navigation.ts` 加导航项 / 图标 / `moduleInfoById` 三处、`page-registry.tsx` 加 `pageById` 与 `liveIds` 两行；落点取证见 Comments | `5c2d5d74` |
| 评审修补 | 票面 / 装配注释 / `DecisionDetailState` 三处 | `96a220eb` |
| 清点 | `docs/product/MECHANISM-INVENTORY.md` 在 `96a220eb` 干净检出上重生成（PS 生产 +3 / 测试 +2、端口文件 +1、http +1；cmd 生产 / 测试各 +1；端口声明 335→336，缺口两栏不变） | `08635c74` |

**两端点路径与 outcome 词**：同一路径 `GET /channel-selection-decisions`——列表分支 `?view=tied[&scope=&mapping=]` →
`TIED_CHANNEL_SELECTION_DECISIONS_LISTED` + `decisions[]`；单份分支 `?decisionId=` → `CHANNEL_SELECTION_DECISION` +
`decision`；查不到 404 `CHANNEL_SELECTION_DECISION_NOT_VISIBLE`。决定体：`decisionId / scope / mapping / assembledAsOf /
rule / decidedAt / conclusion / selectedCandidate?（只在有选中者时在场）/ candidates[]{candidate, evaluation?, outcome, exclusion?}`。
**无金额、无评价内容列**（传输层用例断言体里不含 amount / currency / price）。

**隔离读放行（ADR-0078）判入格，理由**：消费本上下文自己的存储读面且不触发判断、派生或披露（列并列冲突不裁决，人工
裁决不在这一口）；零持久化；作用域是运营侧授权结果（只有租户维，由注入给定，`IsolatedOperationsReadIntake` 的对应方法
不读请求任何授权输入）。与面单交易同款：同一个注入值的第三半接口。

**要 MCP-1 落的共享接线（逐字，已在干净检出上套用验证；同目录 `lc23-integration-lines.patch` 可 `git apply`）**：
① `cmd/parcel-api/endpoints.go`——`assembleBusinessEndpoints` 在 `labelTransactions shipmenthttp.LabelTransactionsReader,` 之后加参
`channelSelectionDecisions shipmenthttp.ChannelSelectionDecisionsReader,`；变量组加
`channelSelectionDecisionIntake := shipmenthttp.ChannelSelectionDecisionQueryIntake(shipmenthttp.UnconfiguredIntake{})`；
`if isolatedRead != nil` 块加 `channelSelectionDecisionIntake = isolatedRead.channelSelectionDecisions`；`/label-transactions` 行之后加
`{Pattern: "/channel-selection-decisions", Handler: shipmenthttp.NewQueryChannelSelectionDecisionsEndpoint(channelSelectionDecisionIntake, channelSelectionDecisions)},`。
② `cmd/parcel-api/assemble_isolated_read.go`——`isolatedReadIntakes` 加字段 `channelSelectionDecisions shipmenthttp.ChannelSelectionDecisionQueryIntake`，
字面量加 `channelSelectionDecisions: shipmentViews,`。③ `cmd/parcel-api/main.go`——`labelTransactions` 之后加
`channelSelectionDecisions, err := buildChannelSelectionDecisionRead(db)`（错误即 `return err`），`assembleBusinessEndpoints(...)` 实参在
`labelTransactions,` 之后加 `channelSelectionDecisions,`。④ `cmd/parcel-api/unwired_orchestration.go`——加
`type unwiredChannelSelectionDecisions struct{}` 及两方法（`ListTiedChannelSelectionDecisions` 回 `nil, errOrchestrationNotWired`；
`FindChannelSelectionDecision` 回零值、`false`、`errOrchestrationNotWired`）。⑤ `cmd/parcel-api/endpoints_test.go`——探针
`"/channel-selection-decisions": {method: http.MethodGet, target: "/channel-selection-decisions?view=tied"},`；
`assembleUnwiredBusinessEndpointsWith` 在 `unwiredLabelTransactions{},` 之后加 `unwiredChannelSelectionDecisions{},`。
⑥ `cmd/parcel-api/isolated_read_test.go`——`isolatedReadAdmittedPatterns` 加 `"/channel-selection-decisions": true,`（表尾）；
`TestBuildIsolatedReadIntakesGrantsAllContexts` 的 nil 检查加 `intakes.channelSelectionDecisions == nil`。

**验证**（钉 `08635c74`，干净 detached 检出）：
- 分支原样：`gofmt -l` 空；`go build ./...`、`go vet ./...` 退 0；无 DSN `go test -count=1 ./...` **94 包 ok、唯一红是
  `internal/architecture` 的 `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable`**——管理台发向 `/channel-selection-decisions` 而端点表
  无此行，红的正是等 MCP-1 落的那一行，这是门禁在守而不是缺陷。反向取证：无 DSN 下 PS postgres 四条新用例全 `--- SKIP`。
- 套上上述整合行（`git apply` 同目录 patch）后：`gofmt -l` 空；build / vet 退 0；**含真库**（门禁容器 55432）`go test -count=1 -p 1 ./...`
  退 0、95 包 ok / 0 FAIL；`-v ./internal/parcelshipment/... ./cmd/parcel-api/... ./internal/architecture/... ./migrations/...`
  = **1611 PASS / 0 SKIP / 0 FAIL**（不锚定行首计数），证据行 `--- PASS: TestTiedDecisionsAreListedNewestFirstWithinTheTenant (0.30s)`、
  `--- PASS: TestADecisionIsFoundByIdentityOnlyWithinItsTenant (0.29s)`、`--- PASS: TestTheWiredChannelSelectionDecisionReadAnswersHonestlyAgainstARealDatabase (0.29s)`、
  `--- PASS: TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable (0.03s)`、`--- PASS: TestIsolatedReadAdmissionSwitchesOnlyOperationsReadLines (0.00s)`、
  `--- PASS: TestEmbeddedMigrationAssetsCarryNoCarriageReturnOrBOM (0.00s)`。`-race` 未跑。
- 前端（借主树 `node_modules`）：`tsc --noEmit` 退 0；`node scripts/run-tests.mjs` 66/66 过（新增五条）。
- 两份棘轮基线零改动；无新迁移；`.go/.ts/.tsx/.md/.patch` 新文件逐份 CR=0 无 BOM。

**有意留待后续**：① 并列冲突的人工裁决动作——形状要先过 /domain-modeling（裁决人与裁决规则属 `PAR-NET-16` 待提供）；
② 「某个候选最近为何总是出局」「某笔映射下这一天做过几次择优」两种读法（票「缺口」节的动机句）——本票按「至少两口」
落了 TIED 列表与按标识取一条，按候选 / 按日汇总的读口等有人要看时另立；③ postgres 读口与 `ListBySubject` 的头行扫描 /
子行装载折成一套 helper（本票只加不改既有文件）。
