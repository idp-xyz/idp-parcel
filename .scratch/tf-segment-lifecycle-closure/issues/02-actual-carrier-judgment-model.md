# 实际承运商判断：有语言无形状

Category: enhancement
Status: resolved——MCP-2（2026-09-04，基线 `a17bfac`，分支 `mcp2-tf02`，已验 tip `6a3f190`；主线 SHA 待 MCP-1 重放后由集成广播给出，本行不代填）。形状已裁（2026-09-03，MCP-3 过 `/domain-modeling`，owner 授权自决），落 [ADR-0103](../../../docs/adr/0103-actual-carrier-judgment-is-a-versioned-record-per-segment-with-pending-as-a-value.md) 与 TF `CONTEXT.md` 三处追加；四问答案与要做的见文末；完成记录见文末
Blocked by: 无

## CONTEXT 要求什么

> **实际承运商**：在明确实际履约段中接收并运输载运对象的业务参与方。实际承运商依据收寄、交接和
> 履约事实确定；证据不足时保持待确认，不能从渠道服务方、签约服务商、底层承运商、品牌或单号推断。

> 实际承运商可以在实际履约段成立时仍处于待确认。后续获得有效证据时形成带业务时间和来源的身份
> 判断，**不追溯覆盖原来的未知期间和判断历史**。

> 渠道账号持有人、渠道服务方、签约服务商、合同与结算相对方、底层承运商、责任承担方和实际承运商
> 必须分别保存，任何一个角色都不能自动推导其他角色。

所有权句：`transport-fulfillment` 拥有「实际承运商判断」。

## 现状（钉 `e3dbf3f`）

领域里没有「实际承运商判断」这个对象；`FulfillmentParticipation` 与 `ActualFulfillmentSegment`
上都没有承运商字段。`grep -rn "实际承运商" internal/transportfulfillment` 只命中注释里的引语。

票 tf-unwired-seven/07 把它明确列为「不在本票，且不是遗漏」：**它要的是一个新模型而不是参与
关系上的一格**，硬塞进段或参与关系就是把两个身份合成一个——CONTEXT 恰恰要求它们分别保存。

## 这一票属 mechanism-executor-triage 的「第四格」

[triage spec](../../mechanism-executor-triage/spec.md) 末节记了一类「CONTEXT 有语言、代码无
形状」的缺口——没有类型可数，棘轮与探针在构造上都看不见。本条同族。处置也同族：**先裁形状，
可能裁成「留空」**（ADR-0098 对揽收↔委托连线就是这样裁的）。

## 建模前要答的

- 判断的主体是什么：按段一条？按对象一条？按对象×段一条？CONTEXT 说「在明确实际履约段中」，
  又说「首次有效收寄必须明确关联载运对象、实际承运商、业务发生时间」——两句指向不同粒度。
- 「待确认」是判断的一个值还是判断缺席？「不追溯覆盖未知期间」要求未知期间本身有起止，那它
  就得是一条记录不是一个空位。
- 证据来源封闭集合有哪些格（承运商直接收寄扫描、承运商收货凭证、权威交接结果、保留原始承运
  来源的可信渠道回传——CONTEXT 列了四种，是否就是全部）。
- 与 `party-commercial` 的边界：承运商作为参与方身份归谁铸；本上下文只引用还是也登记。

## 边界

本票在形状裁定前**不写生产代码**。裁定的落点是 CONTEXT（必要时 GLOSSARY）与一篇 ADR；票面
转 ready-for-agent 时把上面四问的答案写进「要做的」。

## 四问的答案（2026-09-03 · MCP-3，理由全在 ADR-0103，此处只列结论）

1. **主体**：按**实际履约段**一条判断历史。段就是共同控制责任范围，承运责任变了就是另一段；
   逐对象的收寄/交接/履约事实是输入。同段证据指向不同主体 → 待确认（来源冲突），不分别判断。
2. **待确认是值**：段成立即有第一版；待确认原因封闭三格——无合格证据 / 来源冲突 / 承运主体身份
   未登记。版本只追加，未知期间有起止。
3. **证据来源封闭四格**：承运商直接收寄扫描、承运商收货凭证、接收方为承运主体的`已交接`权威交接
   结果、保留原始承运来源的可信渠道回传（外部承运轨迹事实同格）。
4. **边界**：承运主体身份只引用 PC 已登记的参与方（外部）或运营法人（自营），本上下文不铸；
   名称找不到身份 → 待确认（身份未登记），名称作素材随依据保留。

## 要做的

- `internal/transportfulfillment/domain`：新聚合「实际承运商判断」——版本（判断值、业务时间、
  判断形成时间、依据引用列表）、判断值两支（外部参与方引用 / 自营法人引用）+ 待确认三原因、
  来源四格封闭集、构造门（业务时间不早于段成立；来源不在四格内拒；名称素材只随依据）、追加不
  覆盖的转换（待确认→已识别、任一→来源冲突、身份未登记→已识别、更正重派生）、重建门。
  **不改 `actual_fulfillment_segment.go` 任何导出签名。**
- `ports/`：判断登记册端口（按段读当前版本与全部版本、追加版本）；PC 身份存在性读口（消费侧）。
- `application/`：「形成实际承运商判断」用例（收一条合格证据引用 + 承运主体引用或名称素材 →
  查身份 → 形成新版本）；段成立后铸第一版的挂点（挂在 `enterFulfillmentSegment` 落库之后，与
  票 03 的内部触发同形；06 号票已 resolved（`8165c84`），那一段此刻无人在动）。
- `adapters/postgres/`：追加式版本表迁移（编号取当时 TF 下一号）+ 登记册适配器 + 真库用例。
- `adapters/partycommercial/`：身份存在性读口的消费侧适配器（ADR-0025）。
- 验证：领域用例覆盖四问各一条场景（同段冲突、业务时间早于段成立被拒、身份未登记→登记后
  新版本、已识别后相反证据→冲突）；真库用例证版本只追加；`gofmt -l` 空、`go build`/`go vet`
  退 0、`go test -count=1 ./...` 绿并注明含不含真库。
- 完成后在 mechanism-executor-triage spec「第四格」表追一行（该目录所有者），记「已裁形状进
  实现票」。

## 完成记录（2026-09-04 · MCP-2 · 分支 `mcp2-tf02`，基线 `a17bfac`）

按 `/implement`（内驱 `/tdd`，收口 `/code-review` 双轴）逐片提交，每片一笔：

| 笔 | 内容 |
|---|---|
| `c2cc5ab` | 票面转 in-progress；ADR-0103 Consequences 迁移号句改为「实际落在 0013」 |
| `5ec0abe` | 领域：`ActualCarrierJudgment` 聚合首片——`OpenActualCarrierJudgment` 段成立即铸首版（待确认·无合格证据），业务时间取段成立时刻；值类型 `CarrierSubject` 两支 / `PendingCarrierReason` 三原因 / `CarrierEvidenceSource` 四格 / `CarrierEvidence`（在册身份与名称素材恰居其一） |
| `989ec48` | 领域：`Consider` 收合格依据追加版本；`deriveCarrierVerdict` 唯一算法；业务时间早于段成立即拒 |
| `982723f` | 领域：相反证据 → 来源冲突并保留全部依据；同一份依据不成第二版 |
| `dbe8630` | 领域：身份未登记留素材待确认；`RecogniseCarrierIdentity` 凭同一份证据补认成新版本，不追溯改写未登记期间 |
| `dbf5e93` | 领域：`WithdrawEvidence` 撤回依据重新派生（保留原版本、不倒填）；封闭集 String/Parse 往返 |
| `de28342` | 领域：`RehydrateActualCarrierJudgment` 重建门（ADR-0028：验形状与成对关系，不重走派生） |
| `dafb2c0` | ports：`ActualCarrierJudgmentRegistry`（FindByKey 整份历史 / Open / AppendVersion，只插不改）与 `CarrierIdentityDirectory` |
| `ff1832c` | 应用：`enterFulfillmentSegment` 段首登后同笔开判断首版；四条调用编排 Deps 各加可缺席的 `Judgments` |
| `62266d9` | 应用：`FormActualCarrierJudgmentHandler`——收一条合格证据 + 承运主体引用或名称素材 → 查 PC 身份 → 追加版本；结果代数按恢复动作分格 |
| `5076037` | 迁移 `0013_actual_carrier_judgment.sql`（头行 / 版本 / 依据三表，只插不改）+ `adapters/postgres.ActualCarrierJudgments` + 真库用例 |
| `ccaadeb` | `adapters/partycommercial.CarrierIdentityDirectory`（ADR-0025 消费侧；只依赖 PC 两个窄读口） |
| `b98d13b` | `effective_delivery.go` 挂点注释：实际承运商轴由判断按段作答，不复制到交付上 |
| `ec28d9a` | `/code-review` Standards 轴修补四条（派生算法用真集合、注释去计数、主体两列摊法抽一处、撤回改直白过滤） |
| `6a3f190` | 机制清点重生成（在本分支干净检出上跑） |

**验收场景对照**（票面「验证」条）：同段冲突 `TestEvidenceNamingADifferentSubjectTurnsTheJudgmentIntoASourceConflict`；业务时间早于段成立被拒 `TestEvidenceOccurringBeforeTheSegmentWasEstablishedIsRefused`；身份未登记→登记后新版本 `TestAnUnregisteredCarrierNameLeavesTheJudgmentPendingUntilTheIdentityIsRecognised`（领域）与 `TestAnUnregisteredSubjectStaysPendingUntilTheDirectoryKnowsItThenTheSameEvidenceRecognisesIt`（应用）；已识别后相反证据→冲突 同第一条；真库证版本只追加 `TestVersionsAppendWithTheirOwnBasesAndNeverOverwrite`；挂点在真库上落首版 `TestRegisteringAPickupIntoANewSegmentOpensTheJudgmentInTheDatabase`。

**验证**（钉在 `6a3f190`，本机 Windows）：`gofmt -l` 对改过的全部 `.go` 为空；`go build ./...` 与 `go vet ./...` 退 0；`go test -count=1 ./...` 未设 DSN 退 0（95 个包 `ok`，PG 用例跳过）；真库对触及包 `go test -count=1 -v ./internal/transportfulfillment/... ./internal/architecture/... ./migrations/...`（DSN 指向门禁容器 55432）退 0，`--- PASS` 1210 / `--- SKIP` 0 / `--- FAIL` 0；反向取证 `./internal/transportfulfillment/adapters/postgres/` 未设 DSN 时 `--- SKIP` 159 / `--- PASS` 0。`internal/architecture` 两份棘轮基线未改一行：新增的领域类型全部可达、领域工厂全部有生产调用点。`go test -race` 未在本机跑（Windows 无 cgo，见 workflow.md），由 CI 覆盖。

**`/code-review` 双轴**（基线 `a17bfac`，两轴串行隔离——Task 子代理鉴权失败不可用）：Standards 轴 4 条判断题，均已在 `ec28d9a` 修掉，无硬违规；Spec 轴 0 条「实现看着不对」，下列三处为**有意留待后续**并在代码注释写明理由：

1. 挂点只铸待确认首版，不收「成立事实随带的合格证据」（CONTEXT 生命周期首条的已识别分支）：今天立段的收寄事实里执行方是运输方引用不是承运主体身份、`已交接`交接的接收方可以是节点也可以是承运方，替它们推一步就是 CONTEXT 明禁的推断。证据从「形成实际承运商判断」用例进来。要让立段事实自带承运主体声明，得先给收寄/交接登记加一个显式的承运主体输入——那是另一票。
2. 段结束后的「来源事实更正引起的重新派生」与「冲突由人裁为一次新的已识别版本」：领域门 `WithdrawEvidence` 已在；人裁没有领域门（票面「要做的」未列，且它要一个授权模型——谁能裁）。两者都没有应用入口：前者要从交接更正 / 轨迹源更正那两条链触发，后者要一个带授权的运营写面。各自另立票。
3. 端口不单开「读当前版本」口：`FindByKey` 交回整份历史，当前版由聚合 `Current()` 派生——两条读路只会分叉。第一个消费方（VE 投影）到来时按其形状再议。

**其余不在本票**：mechanism-executor-triage spec「第四格」表那一行归该目录所有者（ADR-0103 Consequences 原话「本记录不代改」）；`FormActualCarrierJudgmentHandler` 尚无 HTTP 面或内部触发，是否开在线面归 tf 系列下一票裁。

**要 MCP-1 落的装配行**（`cmd/parcel-api`，本波归 MCP-5 独占，逐字如下）：

- `assemble_control_facts.go` `buildControlFactOrchestrations`：在 `segments` 之后加
  `judgments, err := tfpostgres.NewActualCarrierJudgments(db)`（err 照其余构造处包成 `parcel-api: actual carrier judgments: %w`），并在 `RegisterTransportHandoverDeps`、`RegisterOffsitePickupDeps`、`PerformOffsitePickupDeps` 三处字面量各加一行 `Judgments: judgments,`。
- `buildParticipationEnder`（`EndFulfillmentParticipationDeps`）同样加 `Judgments: judgments,`（它现在在 `buildControlFactOrchestrations` 之前构造；要么把 `judgments` 的构造挪到它前面传进去，要么在它里面自己再 `NewActualCarrierJudgments(db)` 一次——两个实例无状态，都行）。
- 不接这几行时行为不变（`Judgments` 可缺席，同 `Segments` 缺席那条），`assemble_control_facts_test.go` / `assemble_segment_operations_test.go` 里 `SegmentContinuationReference() == ""` 的断言照旧成立；接上之后建议补一条真库装配用例断言段成立后 `actual_carrier_judgment` 有头行——本包最近的一格是 `TestRegisteringAPickupIntoANewSegmentOpensTheJudgmentInTheDatabase`。
- 「形成实际承运商判断」用例暂无装配点；届时身份读口装配为 `tfpartycommercial.NewCarrierIdentityDirectory(identities, identities)`，其中 `identities, _ := pcpostgres.NewPartyIdentityRegistrations(db)`（同一个登记册满足参与方册与法人册两个窄口，编译期已钉）。
