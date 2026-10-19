# 95 同一（对象，范围）下另起一次交接：TF 当独立首登，NO 当前一次的更正，在身控制在错的那次交接转出

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-dcg95（基于 `origin/main` `5f0aa66d`），见文末「完成记录」；隔离评审 0 阻断，非阻断 4 项全修，见文末「评审记录」。此前 needs-triage——2026-10-19 Claude Code 子代理立（演示动线重走，[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「重走（钉 `8197fa2f`，2026-10-19）」停点一）。
Blocked by: 无
地盘：二选一，见「要裁的」——TF `application/register_transport_handover.go` 首登那一步；或 NO `adapters/postgres/physical_control_transition.go` 的 `FindAdvancedInHandoverChain` 与 `application/transfer_control_on_handover.go`。文档 ADR-0214 补记、UC-NO-005。
出处：[ADR-0214](../../../docs/adr/0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md) 越权风险点「同链按（对象，范围）认」；[UC-NO-005](../../../docs/application/node-operations/UC-NO-005-ADVANCE-NODE-CONTROL-ON-AN-AUTHORITATIVE-TRANSPORT-HANDOVER.md)。

## 现象（实测，钉 `8197fa2f`，只记 `S`）

演示包裹 `DPCL-FRVC5MAXIPKBQHYKMXQGOATJUE`（一次性库，走完已删）在深圳口岸先后登两次`已交接`，`scope` 都写 `SZX`：

- `H2/v1`：自营干线交进深圳口岸，只声明 `receivingNode`。TF 答 `201 HANDOVER_REGISTERED`；NO 在深圳口岸立 `HANDOVER_IN` 控制（`IN`·`ADVANCED`）。
- `H3/v1`：深圳口岸交给干线承运商，声明 `releasingNode` 为 `SYN-NODE-SZX-GATE`、`onwardNode` 为 `SYN-NODE-SIN-HUB`。TF 照样答 `201 HANDOVER_REGISTERED`，是首登不是更正。NO 却把它读成 `H2/v1` 的同链后版：转出那一半剔掉同链转入立起的那一份，落 `OUT`·`NOT_ADVANCED`·`NO_ACTIVE_CONTROL`。
- 再下一次交接 `H4/v1`（承运商交进新加坡枢纽，`scope` 为 `SIN`，不声明交出节点）把深圳口岸那一份转出：`OUT`·`ADVANCED`，节点 `SYN-NODE-SZX-GATE`。控制在错的那次交接转出；那次交接没声明交出节点，`mismatchedReleasingNode` 也落不下，册上看不出错。

对照：第二件包裹 `DPCL-7VJZ4RRIS66IV6FQW4RYKPANDI` 走同一段，三次交接各用自己的范围（`W19B-1`、`W19B-2`、`W19B-3`），深圳口岸交给干线承运商那一次落 `OUT`·`ADVANCED`·`SYN-NODE-SZX-GATE`。

## 成因（读码）

- TF 交接的身份是（租户，对象，范围，版本）。更正经更正口登记、回指前身，`TransportHandovers.FindSuccessor` 按这条回指认后版；首登口不问同（对象，范围）下是否已有别的版本，换一个版本号就是另一次首登。
- NO 认同链按（对象，范围）：`PhysicalControlTransitions.FindAdvancedInHandoverChain` 只按对象与范围取，不看 TF 的更正回指；`TransferControlOnHandoverHandler.transferOut` 据它剔掉「这条链转入立起的控制」（`withoutControlOf`）。
- 两边对「哪几版是一条链」各认各的。ADR-0214 越权风险点「同链按（对象，范围）认」预先写明了这一格，此前没有承接票。

## 三类

- 机制：TF 与 NO 对交接链只能有一种认法。两条路：
  - TF 首登口遇到同（对象，范围）已有版本时答一个指名的结果（例如要求走更正口），不落册、不发信封；
  - NO 认链改读 TF 的更正回指，不再按（对象，范围）认。
- 产品策略：无。
- 租户取值：无。

## 完成判据

- 同（对象，范围）下另起一次首登：要么 TF 拒（指名结果，不落册，不发信封），要么 NO 把它当独立交接，前一次立起的控制被它正常转出。
- 更正链的既有各格照旧：后继版仍`已交接`时零写入、撤回控制的更正入账「控制来源链待重算」、一次交接不转出它自己立起的控制。
- 真库用例；证据记 `S`。

## 要裁的

- 走哪一条：「范围」在 TF 语言里是不是交接链的身份。是，就由首登口拒；不是，就改 NO 的认法，ADR-0214 决定五与那条越权风险点随之补记。
- 已按（对象，范围）认错的存量转移行要不要入账「控制来源链待重算」，归 [81](./81-control-source-chain-recompute-has-no-disposition-face.md) 的处置面。
- 演示动线文档要补一句「交接的 `scope` 每次交接各起一个，`version` 只给更正用」，不论本票走哪条路都要补；句子列在 psb/05 那一节的停点建议里。

## 规模

S–M

## 完成记录

2026-10-19，作者子代理，分支 `claude-dcg95`（基于 `origin/main` `5f0aa66d`）。授权：用户原话「你作为业务和系统专家自决……解决所有的问题」；协调方指示「先核 TF 的语义……NO 应照 TF 的更正链判，不自己按 scope 推」。方案（[handover-dispatch-plan](../handover-dispatch-plan.md)）C2–C7 没裁认链这一格，下面逐条记「代决」，已写进新立的 [ADR-0220](../../../docs/adr/0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md)。证据只记 `S`。

### 做了什么

- **核 TF 语义**（读码）：
  - 交接身份是（租户，对象，范围，版本），范围是「一次交接的共同范围」。
  - 更正只经更正口，新版回指前版、保留对象与范围；链线性（票 tf-face-algebra/07）。
  - 首登口不问同一（对象，范围）下有没有别的版本。
  - 结论：同一范围下另起的首登在 TF 是另一次交接；错在 NO 按（对象，范围）认链。
- **领域**：
  - 新文件 `domain/handover_correction_chain.go`：`HandoverCorrectionChain`，即这一版与它沿 TF 更正回指上溯的各前版；立链时核对象、范围相同，版本不重复。
  - `AuthoritativeHandoverReference.SameChain` 退役。
  - `PendingControlSourceRecompute` 改收更正链，只认链上前版推进过的那一行。
- **端口** `ports/physical_control.go`：
  - 交接来源答复加 `Earlier`，即各前版，近者在前。
  - `FindAdvancedInHandoverChain` 改收更正链。
- **消费侧适配器** `adapters/transportfulfillment/control_transfer.go`：读回那一版后沿 `Corrects` 逐版按键上溯。前版读不动包进既有的未决哨兵；前版读不回、与键不符、回指成环答 `ErrHandoverRecordInconsistent`。键一致性核对抽成 `consistentWith`，这一版与前版共用。
- **转移册** `adapters/postgres/physical_control_transition.go`：认链只取链上各版（`handover_version = ANY(...)`）。（对象，范围）只用来缩小读的范围。
- **编排**：`Advance` 立链，转出、转入两半共用；来源给的前版连不成链时响亮失败。转出一半剔除「同链转入立起的那一份」照旧，只是同链改按回指认。
- **派发**：路由、inbox 名、哨兵名单、装配都不改。
- **文档**：
  - ADR-0220 新立。ADR-0214 的 Status 行与 Links 加前向指针。`docs/adr/README.md` 的 ADR-0214 一行写部分停用（被停条款与适用场景），并追加 ADR-0220 一行。
  - UC-NO-005：状态、权威依据 `ADR-CORR`、参与者、输入契约、流程「按四维键读回那一版」那一步、一致性里认链那一条，新增 `AT-NO-126`–`128`。
  - NO CONTEXT「节点控制」更正那一条末尾加一句；CONTEXT-MAP `node-operations ↔ transport-fulfillment` 加一个分句；`docs/application/README.md` UC-NO-005 一行。
  - 演示动线加补记。
  - AT 盘点追记；方案「后继」加一行；票 [81](./81-control-source-chain-recompute-has-no-disposition-face.md) 文末补记存量形状。
- **新立票**：[99](./99-handover-scope-summary-counts-one-object-twice-under-the-same-scope.md)。TF 交接范围汇总把同一范围下同一对象的两次首登各算一次，TF 地盘，归 TF owner 裁。

### 代决（逐条已写进 ADR-0220）

1. **走「NO 改认法」，不改 TF 首登口**（ADR-0220 决定一、四）。TF 语言里范围是一次交接的共同范围，不是更正链的身份。首登口加拒绝会挡掉真实交接，例如离线补登、把节点代码当范围填。
2. **立 ADR-0220，不在 ADR-0214 文末补记**。改的是 ADR-0214 正文决定五已经裁定的一句（「同链认法：同一（对象，范围）的各版」）；补记只补正文没裁的格。按 README 顶部「部分停用」规则办。
3. **前版由消费侧适配器沿回指逐版按键上溯**（决定二）。不加 TF 读口，不改 TF，不在转移行上存链首，不加迁移。迁移号 `node_operations/0019` 未用。
4. **上溯失败分格**（决定二）：
   - 前版读不动：未决重投，与这一版读不动同一个哨兵；
   - 前版读不回、与键不符、回指成环：缺陷，响亮失败。前版先于后版提交，读不回不是可见性滞后；
   - 编排立链时再核一次形状。
5. **链值进领域**：`SameChain` 退役，`PendingControlSourceRecompute` 收链。这样「哪几版是一条链」只在一处定义。
6. **转移册认链按链上版本取**（决定三）。既有部分索引 `physical_control_transition_advanced_by_chain` 留作读路；同一范围下多行推进不再是错误。
7. **存量不改写、不重判，交票 81**（决定五）。本票之前零写入的版本被补拍时，按新规则重判。
8. **演示动线那句改写**。psb/05 停点一建议的句子说「同一 `scope` 换个版本号另登，节点作业会当它是前一次的更正」，修后不再成立。改写为：`scope` 是一次交接的共同范围、每次各起一个，`version` 只给更正用；仍要各起一个，理由换成 TF 范围汇总不按包裹去重（票 99）。
9. **立票 99**：TF 范围汇总对同一对象的两次首登各算一次。不在本票修：它是 TF 地盘，而且怎么修（首登口拒，还是汇总按对象取一个）要 TF owner 裁。

### 越权风险点

逐条见 ADR-0220 文末，待 NO owner 复核、TF owner 会签。按标题：

- TF 不拒同一（对象，范围）下另起的首登（范围汇总算两次，票 99）；
- 存量不重判（票 81）；
- 补拍旧信封答复会变；
- 每封信多读 TF；
- 前版读不回按缺陷处置；
- 链上各版同对象与范围是立链的前提。

### 用例

`AT-NO-126`–`128` 新立于 UC-NO-005。号段 126–135 由协调方预分，本票用 126–128，129–135 未用。

- 领域：
  - `handover_correction_chain_test.go`：126，以及立链拒绝各种坏形状；
  - `control_transition_test.go`：127 的领域面，即同一范围下另起的首登不能入账待重算。
- 编排 `control_on_handover_correction_chain_test.go`：
  - 126：同一范围下先交进、再从同一节点交出，交出那一次转出交进那一次立起的控制；另起的首登两个方向各自判；
  - 127：同一范围下另起的拒收两个方向零写入、不入账待重算；
  - 128：来源给的前版连不成链时响亮失败。
  - 既有 61、62 的编排用例改为显式回指前版，断言不变。
- 消费侧适配器：128。沿回指上溯、近者在前；另起的首登前版为空；前版读不动答未决；读不回、与键不符、成环答缺陷。
- 真库存储：126 的存储面。同一范围下两次首登各推进一行，各自的链只认得出自己那一行；回指的更正认得出前版那一行。
- 派发生产路由真库 `cmd/parcel-dispatch/node_control_same_scope_on_handover_test.go`：
  - 126：照停点那件包裹，交进与交出深圳口岸的范围同为 `SZX`；
  - 108 的派发面：交出那一次经 TF 更正口更正、仍`已交接`，两个方向都零写入。
  - 交出门禁读数助手从 `registerDeclared` 抽成 `declaredGateReading`，与更正共用。

### 验证

- `go build ./...` 干净；`gofmt -l internal cmd migrations` 干净；`go vet` 跑过 `./internal/nodeoperations/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./internal/architecture/`，干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1` 全 ok：`./internal/nodeoperations/...`、`./internal/architecture/`、`./cmd/parcel-dispatch/`。没跑全量，没重生成机制清点（按指示进 main 时统一做）。
- **反证**：
  - 把本票的生产代码临时退回 `5f0aa66d`，派发真库用例照原样复现停点：交出那一次落 `OUT`·`NOT_ADVANCED`·`NO_ACTIVE_CONTROL`。
  - 把转移册查询的版本筛临时短路，存储用例因「advanced more than once」失败。
  - 两处都已复原。
- **门禁**：没有新信封、新分区键、新标识前缀常量，没有新的生产接线；接线棘轮与类型可达性棘轮照过。
- **演示**：没有在一次性库上实走。按协调方「一次性库实走或真库装配用例」取后者，即上面那条派发生产路由真库用例。演示种子未改，没跑 `seed.sh`。

### 偏离与待裁

- **地盘**：
  - 代码只动 61、62 那批 NO 控制转移文件，外加新文件；`reception.go` 收寄部分、`cmd/parcel-dispatch/assemble*.go` 一字未改。
  - 派发测试 `node_control_in_on_handover_test.go`（62 立）抽出一个助手。
  - 文档热点：`docs/adr/README.md` 只在 ADR-0214 那一行末尾追加部分停用，另追加 ADR-0220 一行。NO CONTEXT 只在「节点控制」更正那一条末尾加一句，idp-parcel-9b 在改送站交接单段落，冲突面小。
- **迁移注释过时**：迁移 0017 给索引 `physical_control_transition_advanced_by_chain` 写的注释「同一（对象，范围）下各版是一条更正链」，自本票起不再成立。迁移不改，适配器注释写明这本索引只是读路。要不要另起迁移删掉或改名这本索引，留给 NO owner；它仍是这一问最窄的读路。
- **号段**：
  - 全部照协调方预分，没有自取；
  - 用了 `AT-NO-126`–`128`、新立票号 99、ADR-0220；
  - 没用迁移 `node_operations/0019`、新立票号 100。
- **待裁**：ADR-0220 越权风险点与上面代决 1–9，待 NO owner 复核、TF owner 会签。票 99 的修法归 TF owner。存量行的处置归票 81。

## 评审记录

2026-10-19，隔离评审（作者子代理按评审 JSON 修订，分支 `claude-dcg95`）。阻断 0 项；非阻断 4 项全修，其中 1 项评审建议进 main 前修。

- **转出一半头注把前版也算进「读不回答未决」**（建议进 main 前修，已修）：`transferOut` 头注步骤 1 原写「读回那一版与它的各前版；读不动或读不回答未决」，与实现、ADR-0220 决定二、UC-NO-005 都对不上。改为分格写：这一版或前版读不动、这一版还读不回答未决；前版读不回、读回的与键不符、回指成环或连不成一条链响亮失败，并写明为什么不能挪进未决（前版先于后版提交，读不回不是可见性滞后）。同一口径的两处旧注一并收紧到「还读不回这一版」：`AdvanceControlOnHandoverResult` 头注、消费侧适配器 `ErrControlTransferUndecided` 的注释。评审没列这两处，是复核时顺查出来的：措辞来自票 61、62，那时还没有前版要读，本票之后同样会被读成「前版读不回也重投」。
- **派发哨兵名单注没列立链失败**（已修）：`nodeControlOnHandoverUndecidedSentinels` 头注「不在名单里」那一串在「提供方记录坏了」那一组后补上节点作业领域的 `ErrInvalidHandoverCorrectionChain`（来源给的前版连不成一条更正链，ADR-0220 决定二）。按符号名写，不计数。名单本身不变。
- **迁移 0017 的索引注释已过时**（已修，落文字）：ADR-0220 Consequences 加一条，引迁移原句「同一（对象，范围）下各版是一条更正链。」，写明自 ADR-0220 起这本索引只是认链那一问的读路，不定义链；已发布的迁移不改，要不要另起迁移改名或改注释仍留给 NO owner（同「偏离与待裁」那一条）。没另立票：这不是缺陷，只是命名与注释的去留，由 owner 裁。
- **README ADR-0214 一行两处「停用」方向不明**（已修）：原摘要末尾的「无条款停用。」改为「不停用别的记录的条款。」，说的是 ADR-0214 自己没有停用别的记录；后面追加的「｜**部分停用**」照全表惯例不动，说的是 ADR-0214 被 ADR-0220 停了一句。没改追加段的标题：全表凡部分停用的行都用同一个「｜**部分停用**」写法，只在这一行换写法会破坏惯例。

没有不采纳的项，也没有另立新票。

地盘说明：协调方的协作边界把 `internal/nodeoperations/**` 列为 idp-parcel-9b 的在途地盘。本轮只改本票已经改过的那几个 NO 文件里的注释，没有新增文件，也没有改代码行；`cmd/parcel-dispatch/assemble_node_control_on_handover.go` 不在热点名单内，本票此前没有改过它，这次只改了注释。

修后验证：`go build ./...` 干净；`gofmt -l internal cmd migrations` 干净；`go vet ./internal/nodeoperations/... ./cmd/parcel-dispatch/ ./internal/architecture/` 干净；`IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1 ./internal/nodeoperations/... ./internal/architecture/ ./cmd/parcel-dispatch/` 全 ok。改动只在注释与文档，演示种子未动，未复跑演示，没重生成机制清点。
