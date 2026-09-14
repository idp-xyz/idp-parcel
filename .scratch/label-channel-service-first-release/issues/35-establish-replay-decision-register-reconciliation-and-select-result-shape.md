# 35 择优决定册与面单交易对不上账：建立重放在决定册留下第二条 SELECTED；`Select` 把并列 / 无人参选当错误交回，三处各分类一遍

Category: enhancement
Status: ready-for-agent——2026-09-14 10:2x 通道 1 按用户 10:1x「授权代裁」（PS owner 口径）裁「要裁的」1：**取甲**（建立重放先查交易已存在则跳择优），乙不做、留作候选后继；做法二「先做」与三的五条「归本票」照旧；裁决全文见文末「裁决」。取证锚仍是票面的 `9ddbafcf`——作者开工先在 main 重量「缺口」两段点名的文件是否被 lc/36 / lc/37 / lc/38 改过（lc/38 只改了 `assemble.go` 注释与 PS 收寄路，与本票地盘无交集，但要核）。此前 draft——2026-09-10 22:0x 通道 4 立票（按通道 1 派单 task-d00c5556；lc/28 非作者评审 Spec 非阻断 ③ / 判断题 (c)(d) 与 Standards 非阻断 ① 的后继，顺带收 lc/26 / lc/29 两票评审里仍未修的非阻断）。要裁的一条（重放对账二选一）归 PS owner。只写票面未动代码；取证锚 main `9ddbafcf`
Blocked by: 无（[`28`](./28-channel-selection-composition-root-and-call-entry.md) / [`29`](./29-channel-selection-result-to-label-transaction-basis-translation.md) 已进 main）

## 缺口（取证于 `9ddbafcf`，逐符号名）

**一、建立重放与决定册对不上账（lc/28 Spec ③ / 判断题 (d)）。**

- `EstablishSelectedLabelTransactionHandler.Establish`（`internal/parcelshipment/application/establish_selected_label_transaction.go`）每次都先调 `Selector.Select`；择优步在 `recordDecision` 里**每次比较都追加一条**决定（`ports.ChannelSelectionDecisionRegistry.Append`，头注「只追加」）；随后 `LabelTransactionHandler.Establish` 撞键即重放——`Insert` 答 `LabelTransactionAlreadyExists` → `labelTransactionAlreadyApplied(existing)`，不覆盖。
- 于是一次「建立版本冲突后重放」、或调用方任何一次重试 `Flow.Establish`：交易只有一笔，决定册却多一条 `SELECTED`。`ChannelSelectionDecision` 的对象是 `ChannelSelectionSubject`（scope + mapping），决定不带交易标识；`SelectedChannelBasis` 七格 + `Evaluation`，不带决定标识；`LabelTransaction` 同样不带——**决定册与交易之间今天没有任何一条引用**。若重放时成本已变（价卡版本换了），第二条 SELECTED 的赢家 / 费率与已建交易的 `Rate` 可能不同，且无法对账：票 [`23`](./23-channel-selection-decision-operations-read-face.md) 的读面按对象列历史，看不出哪一条决定成了哪一笔交易。
- 两段分两笔事务是有意的（`cmd/parcel-api/assemble_label_channel.go` `transactionalChannelSelection` 头注：择优留痕在择优那一步已写完，不随翻译 / 建立失败回滚）——所以这不是把两段合成一笔能解的。lc/28 两条真库用例没有钉重放行为（票面无重放判据，评审 ③ 原话）。

**二、`Select` 把两个业务答案当错误交回，分类在三处各写一遍（lc/28 Standards ① / 判断题 (c)）。**

- `SelectChannelCandidateHandler.Select`（`select_channel_candidate.go`）对并列 / 无人参选：先 `recordDecision`，再以 `domain.ErrChannelCandidateCostTied` / `domain.ErrNoQualifiedChannelCandidate` 交回（头注「决定已记、交回的候选为零值、错误具名」）。
- 于是每个调用方都要把这两个错误从真错误里挑出来：`Select` 自身（决定要不要记）、`EstablishSelectedLabelTransactionHandler.Establish`（翻成 `ChannelSelectionTied` / `NoQualifiedChannelCandidate` 结果格）、`transactionalChannelSelection.Select`（这两格提交、其余回滚）。第三处最重：组合根里写着「哪些错误是业务答案」，贴着 lc/28 红线「组合根只接线不加规则」（评审 Standards ② 判可接受，根因在 ①）。
- 领域比较器 `SelectChannelCandidateByCost` 的这两个出口与决定册的结论三格 `ChannelSelectionConclusion` 一一对应——它们在领域里已经是「结果」，只在编排的交回值上长成了「错误」。

## 语言从哪里来

- 票 [`01`](./01-channel-candidate-tie-break-authority.md) 裁决 / `PAR-NET-16`：并列**且无法选出唯一一条**交冲突、由人裁——冲突是择优的一种正当结果，不是失败。
- 票 [`14`](./14-rejected-candidate-trace-object.md) 票面：决定记录「三种非错误出口各成一条」——票 14 自己就把并列 / 无人参选叫「非错误出口」。
- ADR-0029：结果代数按恢复动作分格。并列（等人裁）、无人参选（补价卡 / 放宽约束）、择优停下（修装配 / 等依赖）三者恢复动作各不相同，各该成一格，而不是两格藏在 `error` 里。
- 对账那半：票 23 读面存在的理由是「择优可独立回放与审计」（lc/28「两条路」甲的那半句）；决定与交易之间没有引用，审计到决定就断了。

## 做法

**二先做**（它让一的两条路都更好写）：

1. `Select` 的交回值改为一个结果对象（形照 `SelectedLabelTransactionResult`：结论格 + 选中候选可缺席），并列 / 无人参选成结果格，`error` 只留真错误；`ChannelSelector` 窄面随之改。三处分类归一：`Establish` 直接映射结果格；`transactionalChannelSelection.Select` 不再认任何领域错误——`Select` 不返 error 就提交、返 error 就回滚，组合根里那段业务分类连同头注一起删。
2. **调用点清单**（改 12 / 29 既有签名，每处写明）：`select_channel_candidate.go` 的 `Select` 与其窄面 `Handle`（票 29 expand 时保留的旧签名，今天无生产调用方——保留还是删随本票定，删则 `SelectChannelCandidateHandler` 只剩 `Select`）；`establish_selected_label_transaction.go` 的 `ChannelSelector` 接口与 `Establish`；`cmd/parcel-api/assemble_label_channel.go` 的 `transactionalChannelSelection`；测试 `select_channel_candidate_test.go` / `select_channel_candidate_decision_test.go` / `establish_selected_label_transaction_test.go` / `assemble_label_channel_test.go`。`domain.ErrChannelCandidateCostTied` / `ErrNoQualifiedChannelCandidate` 留在领域比较器（它们是比较器的出口），只是编排不再把它们透传出去。

**一按「要裁的」1 二选一**：

- 甲 · **建立重放先查交易已存在则跳择优**：`EstablishSelectedLabelTransactionHandler.Establish` 先按 `TransactionID` 问交易册（`LabelTransactionRepository.FindByID`——给 `Flow` 一个只读口，或让 `LabelTransactionEstablisher` 窄面多一问），已在即直接答重放、不择优不记决定。好处：一笔交易一条 SELECTED，不改任何对象形状、不加列；坏处：多一次读，且仍不能从决定册反查交易。
- 乙 · **决定记录带交易引用**：`ChannelSelectionDecisionSpec` 加可缺席的 `Transaction`（或 `SelectedChannelBasis` / `LabelTransaction` 带决定标识——三处择一，不三处都加），PS 迁移序号重取；票 23 读面按交易能查到那条决定。好处：可对账；坏处：择优发生在交易建立之前，决定写下时交易标识虽已在 `EstablishSelectedLabelTransactionCommand.TransactionID` 里但交易还没落——写的是「打算建的那笔」，建立失败它就指向一笔不存在的交易，读面上要说清。
- 两条都做也成立（甲堵重复、乙给对账），归 PS owner。

**三、顺手：lc/26 评审 Standards 四条与 lc/29 评审非阻断五条，逐条核于 `9ddbafcf`**（`git diff --stat` 核过 lc/29 点名的三个文件自 lc/29 进 main 起零改动；lc/26 点名的四处注释原文仍在）：

| 出处 | 内容 | `9ddbafcf` 上的状态 | 处置 |
|---|---|---|---|
| lc/26 S(1) | 跨文件计数进注释：`cmd/parcel-dispatch/assemble.go` `judgeLabelFinalOnLabelTransactionConsumer` 头注「判断编排的六口」、`labelTransactionJudgmentUndecidedSentinels` 头注「四个读口之一」；`internal/parcelshipment/adapters/labelfinal/judge_parcel.go` `ErrJudgmentUndecided` 与 `Consumption` 注释「四个读口之一」 | 四处原文在 | **归本票**：去数字（「读口之一」/「判断编排的几口」）；`assemble.go` 是共享文件，只改那两行、动前占号 |
| lc/26 S(2) | `assemble.go` `parcelFinalAdoptionChain` 头注「各建一份 handler 会让……看见两处」，而两只消费者各调一次它、各得一份 handler；实际共用的是 `FinalOutcomeStore` 那张表 | 原句在 | **归本票**：改句为「两条链各建一份 handler，共用的是 `FinalOutcomeStore` 那一处当前有效终局」；不改代码结构 |
| lc/26 S(3) | `operate_label_transaction.go` `judgmentBeat` 用 `saved.Revision()+1`，把 postgres `Save`「写回恰好加一」抬进应用层；`LabelTransactionRepository` 头注未承诺 +1 | 原样在（lc/28 改了同文件的 `Basis`，没碰这一处） | **归本票**（设计题）：`Save` 交回持久化后的版本（`LabelTransactionSaveOutcome` 带版本或 `Save` 多一返回值），`judgmentBeat` 用它；改 `ports` 签名要连 `pspostgres.LabelTransactions` 与各测试替身一起 |
| lc/26 S(4) | `NewLabelTransactionHandler` 不校依赖，`Judgments == nil` 到首次 `RecordChannelResult` 才响；lc/28 装配处须断言非 nil | `assemble_label_channel.go` 已 `if transactions == nil \|\| judgments == nil { return … }` | **已由 lc/28 修**（装配处断言；构造器本身照 PS application 既有风格不改） |
| lc/29 S(1) | `NewChannelSelectionBasisTranslator` 头注只为三个源论证 nil = 显式未配置，两个 PC 读口 `Authorizations` / `Contents` 不是实例半边，nil 会在 `authorizationOf` 的 `LoadLatest` 处 panic 而非具名停 | 原样在（lc/28 在装配处真装了两口，构造器未改） | **归本票**：构造期拒这两口 nil、返 error（形照同包 `NewCommercialResolutionKeys`）；调用点 `assemble_label_channel.go` 那一处接错误 |
| lc/29 S(2) | `authorizationOf` / `agreementOf` 用 `!query.At.Before(revokedAt)` 自分「已撤销 / 不在有效期」，与 PC `AllowsUseAt` / `SupportsProcurementAt` 内部谓词是第二份 | 原样在 | **不归本票**：PC 侧出「为何不允许」访问器是 PC owner 的地盘，无票号；PS 这边等它出了再换。本票不为它立票 |
| lc/29 Spec(1) | `adapters/parcelpricing/cost_bridge.go` `withRateReference` 不在票面地盘四项之内 | 范围说明 | **无需修**（评审自判为范围说明） |
| lc/29 Spec(2) | `accountUseFor` / `agreementVersionFor` / `resolutionFor` 三处 `deps.X == nil` 分支无用例；「授权源未装」子用例用零值替身而非 nil | 原样在（`channel_selection_basis_test.go` 无任一源为 nil 的用例） | **归本票**：与 lc/29 S(1) 同一处的两面，三处各补一条 nil 用例断言到具名停格 |
| lc/29 Spec(3) | `Handle` → `Select` 窄面等价；`selectedCandidateOf` 赢家不在成本表按 `ErrChannelCostsIncomplete` 报，实际不可达 | 读法说明 | **无需修**；做法二若删 `Handle`，这条自然消失 |

## 红线

- 决定记录只追加（票 14 / 23）：不删、不改既有 SELECTED；甲路是「不再多写」，不是「回去删」。
- 不改并列 / 无人参选的业务语义：并列仍交人裁、无人参选仍不建立（票 01 裁决、lc/28 判据 2）；本票改交回值的形状，不改判法。
- 领域比较器 `SelectChannelCandidateByCost` 不动；`domain/**` 只在乙路加一格。
- `cmd/parcel-dispatch/assemble.go` 与 `cmd/parcel-api/assemble_label_channel.go` 是共享接线文件，改注释也占号、只改自己那几行。
- 不种任何映射 / 价卡 / 约束（实例半边）；重放用例用替身与合成串。

## 完成判据

1. `Select` 交回结果对象，并列 / 无人参选为结果格且决定已记；`git grep -n 'ErrChannelCandidateCostTied\|ErrNoQualifiedChannelCandidate' -- internal/parcelshipment/application cmd/ ':!*_test.go'` 只剩 `select_channel_candidate.go` 一处（领域出口 → 结果格的翻译点）；`transactionalChannelSelection` 里无任何领域错误分类。
2. 重放：同 `TransactionID` 调 `Flow.Establish` 两次——甲路：决定册仍一条 SELECTED、第二次答重放；乙路：两条决定各带交易引用、读面按交易能取到。真库一条钉住（照 lc/28 两条真库用例的形，落 `assemble_label_channel_test.go`）。
3. 上表「归本票」五条各自有据：注释去数 / 改句（`git grep` 原句零命中）；`judgmentBeat` 不再 `+1`（既有真库用例仍钉版本相邻）；翻译器构造期拒两口 nil + 三处 nil 用例。
4. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` PS application / adapters/partycommercial / adapters/postgres / adapters/labelfinal + `cmd/parcel-api` + `cmd/parcel-dispatch`（带 DSN）+ `./internal/architecture/...` 绿；接线基线不响（不加导出领域工厂；乙路若加 `New*` 不在棘轮网内）。
5. 完成记录逐笔 SHA、逐项对上表、写明二选一取了哪条与理由。

## 地盘

`internal/parcelshipment/application/`（`select_channel_candidate.go` / `establish_selected_label_transaction.go` / `operate_label_transaction.go` 及测试）、`internal/parcelshipment/ports/`（`ChannelSelector` 所引类型；`LabelTransactionRepository.Save` 若改）、`internal/parcelshipment/adapters/postgres/`（`Save` 若改；乙路迁移与读回）、`internal/parcelshipment/adapters/partycommercial/channel_selection_basis.go` 及测试、`internal/parcelshipment/adapters/labelfinal/judge_parcel.go` 两处注释、`cmd/parcel-api/assemble_label_channel.go`、`cmd/parcel-dispatch/assemble.go` 三处注释（共享，占号）；乙路加 `internal/parcelshipment/domain/channel_selection_decision.go` 一格 + `migrations/parcel_shipment/` 新序号；本票面；lc spec 子票表一行。**不动** PC / PP、端点表 / 探针 / 放行表、`internal/architecture/*_baseline.txt`。

## 要裁的

1. **（已裁，见「裁决」）** 重放对账取甲（建立重放先查交易已存在则跳择优）还是乙（决定记录带交易引用），或两条都要——归 PS owner。本票倾向**先甲**：不加列、不改对象形状、当场堵住第二条 SELECTED；乙的「决定写下时交易还没落」那格要先想清读面怎么说。若 owner 取乙，「引用放在决定上 / 择优结果上 / 交易上」三处择一也在这一裁里定。

## 裁决（2026-09-14 10:2x，通道 1 推送方按用户「授权代裁」以 PS owner 口径裁）

1. **取甲，不做乙。** `EstablishSelectedLabelTransactionHandler.Establish` 先按 `TransactionID` 问交易册，已在即直接答重放（结果格用既有那格的现名，不新造）、不择优、不记决定；不在才走择优 → 建立。只读那一问的落点作者定——给 `Flow` 一只只读口，或让 `LabelTransactionEstablisher` 窄面多一问——判据是**不扩 `LabelTransactionRepository` 的写面、不给组合根加规则**；那一问与随后的建立在 `transactionalChannelSelection` 同一事务内，事务边界照旧。
2. **为什么甲不乙**：乙的「决定写下时交易还没落」不是实现细节而是读面语义题——决定册那条引用会指向一笔可能永远不存在的交易，票 23 读面要先说清它对运营意味着什么，那是 `/domain-modeling` 的活，不在尾巴票里顺手定；甲当场堵住第二条 SELECTED，不加列、不改 `ChannelSelectionDecisionSpec` / `SelectedChannelBasis` / `LabelTransaction` 任一形状、不动票 14 / 23「只追加」的语义。**从决定册反查交易**这半留作候选后继（归 PS owner），等票 23 读面有了真实消费者、知道它要按什么查时再裁乙——到那时若要加引用，「放在决定上 / 择优结果上 / 交易上」三处择一一并定。
3. **越权风险点**：多一次读（每次建立多一问交易册）——今天没有量化过这条路的读写比，本记录认下这个代价；若将来有证据说它是热点，乙路的引用可以反过来省掉这一问，届时另议。
4. **不改的**：并列 / 无人参选的业务语义（票 01 裁决）、领域比较器、决定记录只追加——与票面「红线」同。

## 参照

票 `28` Comments「评审 ← 通道 3」Standards ① ②、Spec ③、判断题 (c)(d)、「进 main 记录」；票 `26` Comments「评审 ← 通道 6」Standards (1)–(4)、「进 main 记录」；票 `29` Comments「评审 ← 通道 6」Standards (1)(2)、Spec (1)–(3)、「进 main 记录」；票 `14`（决定记录只追加、三种非错误出口）；票 `23`（读面）；票 `01` 裁决；ADR-0029；ADR-0134 决定三；`internal/parcelshipment/application/select_channel_candidate.go` / `establish_selected_label_transaction.go` / `operate_label_transaction.go`；`internal/parcelshipment/domain/channel_selection_decision.go`（`ChannelSelectionSubject` / `ChannelSelectionConclusion`）；`internal/parcelshipment/domain/selected_channel_basis.go`；`internal/parcelshipment/ports/channel_selection_decision.go`；`cmd/parcel-api/assemble_label_channel.go`。

## Comments

- 2026-09-10 22:0x · 通道 4（task-d00c5556，取证锚 main `9ddbafcf`）：立票，draft（要裁的一条）。**只写票面，未动代码。** 能力边界：读过 `select_channel_candidate.go` / `establish_selected_label_transaction.go` 全文、`operate_label_transaction.go` 的 `Establish` 与 `judgmentBeat` 一带、`assemble_label_channel.go` 全文、`channel_selection_decision.go` 的对象与结论类型、`selected_channel_basis.go` 的 Spec、`channel_selection_basis.go` 的构造器与三处 nil 分支、`assemble.go` 的四处注释与 `parcelFinalAdoptionChain` 头注、lc/26 / 28 / 29 三票评审与进 main 记录全文；`git diff --stat` 核过 lc/29 点名的文件自进 main 起零改动、lc/26 点名的四处注释在 `9ddbafcf` 上原文仍在。**没读** 票 23 读面的 SQL 与 `LabelTransactionSaveOutcome` 的全部调用点——lc/26 S(3) 改 `Save` 签名会波及多少替身，作者开工时量。上表「已由 lc/28 修」只认了 lc/26 S(4) 一条（装配处断言）；lc/28 评审「无发现」里那句「lc/29 两 PC 读口真装已做」指装配处真装，不等于构造器拒 nil，故 lc/29 S(1) 仍归本票。
