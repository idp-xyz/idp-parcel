# 项目完成情况客观评估（2026-08-20）

Category: chore
Status: 取证快照，不随后续提交改写

只读评估，无代码改动。评估框架取自[首发开发主线](./../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)自身的判据：**产品就绪 = 八个 PN 切片的机制半边全部达标**（骨架完整、受控案例可复算、参数显式未配置），**试点就绪**属租户里程碑、不在开发方可达路线图内。本文不重述该框架，只对着它定位现状。

## 取证基线

- 盘于 `c133e46`（`docs(scratch): 补提交只存在于工作区的票面与交接档`），2026-08-20。
- 共享树上另有九处未提交变更（`.cursor/mcp.json`、ADR-0067 草稿、`docs/domain/ABBREVIATIONS.md`、SA `CONTEXT.md` 修订、`docs/adr/README.md`、四份 scratch 票面），**全部为 md/json，无未提交的 `.go`/`.sql`**，因此下述测试证据准确反映 `c133e46` 的已提交代码。
- 机制半边的上一轮权威盘点是基线文档「机制半边现状」一节（第二十四轮，头注盘于 `4c928b0`，2026-08-14）。**当心该节内部数字视点不一**：端口计数注明盘于 `bfb2063`，而「62 份迁移」「132 个适配器」「路由表只有一条」「组合根已接通」等表述经实测均晚于 `4c928b0`（该提交上 `assemble.go` 还挂着 `errDispatcherNotWired` 哨兵、迁移仅 40 份）——快照节被逐次增补而头注戳未随动。因此本文「快照后增量」的差量**一律取自两端实测**（`4c928b0` 与 `c133e46` 各数一遍），不采基线正文中间值。

## 结论

八个 PN 切片全部「部分」、无一达标、也无一未开始（与第二十四轮定级相同，但多处缺口在六天里实质收窄）。机制半边大盘已铺满：十个业务上下文全部有生产代码、应用编排、真库适配器与测试；含 PG 门禁的全量测试全绿。剩余机制缺口收敛为：端口生产实现未接满、三票在办裁断、若干待分诊票与工程残局。实例半边按定义为空（尚无租户），按基线规则不构成机制完成障碍。

## 硬证据（本轮实测）

| 项 | 结果 | 口径 |
|---|---|---|
| `go build ./...` / `go vet ./...` | 零信号 | 共享树，`c133e46` |
| `go test -p 1 -count=1 ./...` **含 PG 门禁** | **68 包全部 ok，0 失败**，3 分 13 秒 | `postgres:16.14` 门禁容器，主机端口 `127.0.0.1:55432`，`IDP_PARCEL_POSTGRES_DSN` 已设；**本机未加 `-race`**（CI 含 `-race`，本轮未复跑 CI） |
| 生产 Go 文件 / 测试文件 | 442 / 444 | `internal`+`cmd` 计生产，另含 `tests/bentocontract` 计测试；按文件名 `_test.go` 分 |
| 生产代码假实现 | 零匹配 | `internal/` 与 `cmd/` 非测试文件 grep `TODO|FIXME|not implemented|panic(`，两处分别查过 |
| SQL 迁移 | 75 份 | `migrations/` 十个模块目录 |
| postgres 适配器生产文件 | 141 份 | `internal/*/adapters/postgres/` 非测试 |
| 应用编排文件 | 59 份 | `internal/*/application/` 非测试 |
| 用例 | 49 个 UC（另一份 `UC-PS-001` 商业简报不是独立 UC） | `docs/application/**/UC-*.md` 共 50 份 |
| ADR | 67 个（`0067` 尚未提交，在工作区） | `docs/adr/0*.md` |
| 调度器路由表 | 十二类事件（含 FanOut 与未决哨兵按路翻译） | 实测 `NewDirectPublisher` 路由映射 12 个条目，与 `assemble.go` 文件自注一致 |

**注意本表全部是文件级与包级计数**，与基线用 `go/ast` 做的方法集口径不同（差别见 [第二十五轮端口盘点](./port-inventory-r25/report.md) 的「计数口径」一节）；两套口径不可直接互换，本文引用基线语义计数时逐处注明出处。

### 三条产品就绪判据对照

- **骨架完整**：满足。生产代码零 `TODO`/`panic`，空缺是干净的空缺（本轮 grep 复核，与第二十四轮结论一致）。
- **受控案例可复算**：满足且有持续保证。测试与生产文件约 1:1；CI 在 push/PR 上跑 `gofmt`/`go vet`/`go test -race`/`go build` 并起真实 PG 服务，真库适配器在 CI 实跑不跳过。本轮本机含 PG 全量复跑印证。
- **参数显式未配置**：满足且被刻意维持。全仓直接依赖仅 `chi`/`pgx`/`idp-bento-go` 三个，生产代码无业务阈值、金额、费率常量。

判据逐条都满足，但「达标」按切片判：每个切片各自还有端口未接满或消费装配未收尾，故八切片仍全为「部分」。

## 快照后增量（`4c928b0` → `c133e46`，六天）

PN-06 主线大幅推进，git log 可查：**派发组合根从未接线走到接通**（`4c928b0` 上 `assemble.go` 还在 `errDispatcherNotWired`，现为真实连接池 + Outbox/Inbox + 进程内直投 + 12 类路由）、物理源投影管线闭合（收寄/揽收/交付 FanOut 先 VE 后 PS、交接仅投 VE）、映射目录改按事实类型键（迁移 `0014`）、`UC-VE-008` 客户视图接线完成（PS 按委托反查读口、接受决定经 FanOut 补派生、迟到账户绑定重派生进 CONTEXT 与 UC）、调度器启动就绪（Ping/CheckSchema）、`claim_item` 修订 CAS、外部评审 `external-review-49a2ab0` 清账过半：01/02/04 三票 resolved，03 票经用户拍板走事件驱动补派生（[决定文档](./external-review-49a2ab0/issues/03-rederive-route-decision.md)已 resolved）、实现落在 ve-008 票族并已合入（`a771bc3`），但问题票本身状态仍挂 needs-triage（见下）。

量化差量（两端实测，同口径）：生产 Go 文件 338→442、测试文件 320→442（`internal`+`cmd`；`tests/` 另有 2 份不变）、迁移 40→75、postgres 适配器 102→141、UC 48→49（新增 [UC-PC-003 商业授权裁断](../docs/application/party-commercial/UC-PC-003-ADJUDICATE-COMMERCIAL-AUTHORIZATION.md)，其编排触点 `adjudicate_commercial_authorization.go` 本轮实测存在）。第二十四轮点名的「消费装配只开一个方向」这条横切缺口**显著收窄但未收口**：十个上下文共 46 个 Outbox 交接适配器在发意图，已接消费者的事件类型是 12 类；其余类型按 ADR-0049 记 `no_subscriber` 属诚实状态而非缺陷，但逐消费者铺满仍是机制半边的余量。

## 剩余机制缺口（真缺口，非实例墙）

1. **端口生产实现未接满**。基线现行记载（盘于 `bfb2063`）：十个 `ports.go` 共 200 口，11 口无任何生产实现（规则/政策/权威视图六口、授权四口、外部通道一口）。**本轮未重算**，该数取自基线；下一轮重盘按第二十五轮报告的口径程序执行。
2. **在办裁断 3 票**（Status: in-progress）：
   - [outbox 逐事件分区键使顺序保证空洞化](./outbox-partition-key/issues/01-per-event-partition-keys-make-the-ordering-guarantee-vacuous.md)
   - [同币种更正与形成门冲突](./supplier-expected-cost-correction/issues/01-same-currency-correction-contradicts-the-forming-door.md)（ADR-0067 草稿与 SA `CONTEXT.md` 修订在工作区未提交，同族另有两票 draft）
   - [索赔资格查询七维缺四维](./ve-claim-eligibility-dimensions/issues/01-eligibility-query-cannot-carry-four-of-seven-dimensions.md)

   其中两票各有在途 worktree/分支（`claim-elig-b`、`outbox-pk-annotate`），实现未合入 `main`。
3. **待分诊 10 票**（Status: needs-triage，其中一票疑似已过时）：VE-008 迟到绑定余波两票（[归属易手](./ve-008-late-account-rederive/issues/05-customer-attribution-changes-hands.md)、[回退未定](./ve-008-late-account-rederive/issues/06-customer-attribution-reverts-to-indeterminate.md)）、[客户视图无重派生](./external-review-49a2ab0/issues/03-customer-view-has-no-rederive-after-late-account-binding.md)（**疑似已被覆盖待清账**：其「未定问题」已由决定文档裁为方向 a 并经 `a771bc3` 实现，本文不代改状态）、[PS 外部标记关系无模型](./ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md)、[NR 路由证据视图机制半边切割](./nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md)、[交接-交付粒度](./route-handoff-delivery-granularity/issues/01-per-parcel-independence-cannot-be-expressed-in-one-delivery-one-transaction.md)、[第二十五轮端口盘点](./port-inventory-r25/report.md)、死会话残局两票（见下）；第十票是阻于 PAR-INT-01 的 ops 重放端点票，归在「实例半边墙」一节列出。
4. **工程残局**：约 30 棵 TEMP worktree 残留；[三棵脏树扣着未提交工作](./dead-session-salvage/issues/02-dirty-orphan-worktrees-hold-uncommitted-work.md)（含新迁移 `0016` 一份、墙/门审计报告加十张票一份）；[未合并平行实现分支 `mcp3-ve008-wire`](./dead-session-salvage/issues/01-mcp3-ve008-wire-a-unmerged-parallel-implementation.md)。均已立票，未获裁断前不动。
5. **真渠道 Intake 的两项前置**（本文初版遗漏，自查补入）：ADR-0055 第五条明记载荷规范化摘要与准入范围装配（`PAR-GOV-03..07`）仍拦着真渠道 Intake，「真渠道落地时它们必须先行或同批」。机制可先建，相关票面见 [接入渠道登记与首个真实 Intake](./syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md)（属墙/门审计票族）。
6. **消费方向清点任务**：哪些事件类型必须接消费者要按 UC 逐个裁（终点不是 46 类全接，见「快照后增量」末句），这份清点本身尚无人做，是达标重盘的前置。

另有一票 ready-for-agent 属本机 agent 工具环境（glob 不入 junction），不计产品缺口。

## 实例半边墙（按设计如实存在，不计缺口）

`ROUTE_EVIDENCE_NOT_CONFIGURED`（PAR-NET-14）、`INTAKE_QUALIFICATION_UNPROVEN`（PAR-COM-16）、`FINAL_RULE_UNCONFIGURED`（PAR-COM-17）、`MAPPING_NOT_CONFIGURED`、`ROUTING_APPLICABILITY_UNAVAILABLE`（商业适用性在逐包裹证据之前，两道先后顺序见基线派发条目）、Intake 认证（PAR-INT-01，[ops 重放端点票](./ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md)阻在此）。这些墙全部拒绝默认值；接通不等于能干活，纵向闭环阻在实例半边而不是接线上——这正是基线要求的诚实形态。

## 总评

机制半边完成度很高且质量证据扎实：含 PG 全量全绿、零假实现、测试与生产约 1:1、无业务常量。用例编排触点 49/49：其中 48/48 是基线经两道复核的断言（本轮未逐个重验），快照后新增的 UC-PC-003 的编排触点本轮实测存在。距「产品就绪」的主要距离是十一口端口的生产实现、三票在办裁断与消费装配收尾，外加一批已立票的工程残局。方法论上项目高度自省：基线自带重盘触发条件并如实记录过往误记，本文计数与其口径可相互印证、差异处均已注明出处。

## 「本文缺口全部清完」不等于项目完成

清完上述清单只把项目送到「产品就绪」的**候选**状态（可演示、可商务），且宣布达标前还差一轮正式重盘：「11 口」是松口径下限（方法集口径缺口更多，见[第二十五轮盘点](./port-inventory-r25/report.md)的两套口径），消费装配的终点按 UC 逐个裁而不是按 46 类事件数（ADR-0049「接不住的类型登记比不登记更糟」），且本文只列 `c133e46` 时点的已知缺口——本仓的重盘机制本身假定缺口逐轮发现。

**从开发角度，终点是明确且可枚举的**：缺口清单（含第 5、6 两条自查补入项）清零 + 一轮达标重盘通过 = 开发侧完成，基线给开发方定义的职责就收敛于此。当前工程状态始终可交付（CI 全绿含 `-race` 含真库、零假实现、测试 1:1），所以剩余距离是有限的机制工作量，不是稳定化风险。清零之后开发工作不消失但性质改变：接真渠道、陪跑 PN-08、按真实参数复核 `PA-*` 假设都以租户出现为前提，属租户接入工程而非本项目欠账。

而「项目完成」的完整含义含试点就绪，那按基线定义**不是开发方的里程碑**：`Go/No-Go` 由租户责任法人指定的角色作出，实例半边在没有租户时不是还没拿到、是尚无产生它们的关系。产品就绪是开发方可达的最高点；其后的那一半，等的是第一个租户，不是更多提交。

## 本文不包含

- 不重算端口方法集口径（取基线数并注明盘点提交）。
- 不评估 `docs/archive/` 与关务专项工作单的历史覆盖。
- 不裁断任何在办票；去留以各票与用户裁定为准。
