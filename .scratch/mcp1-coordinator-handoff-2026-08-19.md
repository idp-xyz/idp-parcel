# 交接：MCP-1 协调会话（PN-06 物理源投影管线）

Status: superseded

> 2026-08-25 MCP-1 注:本档是 2026-08-19 的接棒快照,其待办与禁令已被后续轮次消化
> (物理源管线闭合、VE-008 解禁并接线、MAP-KIND 落地、死树清册执行完毕、UC-VE-008
> 客户视图链与 ADR-0076 运营读口先后落地),存活工人名单等断言亦已过期(MCP-2 现役)。
> 现行轨道以 development-plan-2026-08-20.md 及其复核记录为准;本档仅追溯,不再更新。

工作区副本（下一任 Cursor 会话应 @ 此文件）：`.scratch/mcp1-coordinator-handoff-2026-08-19.md`  
TEMP 副本（技能默认落点，新会话不会扫）：`C:\Users\topsx\AppData\Local\Temp\idp-parcel-mcp1-handoff-2026-08-19.md`

新对话**不会**自动读上一场、不会扫 TEMP、也不会读 AGENTS.md 里没有的指针。接手方式只有人交指针：开场 `@.scratch/mcp1-coordinator-handoff-2026-08-19.md` 并写「接 MCP-1」。若仍绑本 MCP-1 通道，再 `load_progress`。

- 写成：2026-08-19
- 上一会话：[MCP-1 投影管线](251181c5-dc9e-490c-ae24-241e71877bc4)
- 下一会话用途：**接 MCP-1 协调岗**（派工人、隔离 worktree 集成、验绿后把确切 SHA 推 `origin/main`）。不是直接写业务代码。用户说「继续」再派下一票；未点头不要开新实现。

## 立刻要做的第一件事

1. `git fetch origin main`，确认 tip **至少**是 `7d7e138`（`feat(dispatch): 交接登记只投 VE 投影不 FanOut PS`）。共享树 `D:\tops\idp-parcel` 经常落后，**以 `origin/main` 为准**，不要在共享树上编码或 `git add -A`。
2. 确认 MCP-2 / MCP-4 空闲。HANDOVER-B 已合入；曾通知 MCP-2 自行拆 `C:\Users\topsx\AppData\Local\Temp\idp-parcel-cons-proj-handover-b`。若该 worktree 还在，让工人拆，协调员不要动工人目录。
3. 等用户点头再派票。上一会话已合完物理源四路投影，**没有未推的已验证 SHA**。

## Suggested skills

下一会话按意图调用（本仓技能装了但部分 `disable-model-invocation`，agent 列表里看不见不等于没装；读 `~/.cursor/skills/<name>/SKILL.md`）：

| 意图 | Skill |
|---|---|
| 不知下一步 | `/which-skill` |
| 用户点头后按票实现（工人） | `/implement` → 内嵌 `/tdd` |
| 合入前双轴评审 | `/code-review`（Spec 轴对照 `UC-*`，不是工单口吻） |
| 映射键 / 更正 / 反查口这类领域裁断 | `/grill-with-docs` → `/ubiquitous-language` → `/domain-modeling` |
| CONTEXT-MAP 缩写对照表（人已点头才写） | `/writing-for-agents` |
| 会话将满再交 | `/handoff`（本文就是） |
| 外来堆、非 W 包 | `/triage`（已是 PN 工作包的不要 triage） |

本仓开工顺序与红线：[AGENTS.md](file:///D:/tops/idp-parcel/AGENTS.md)。技能如何落到 PN 切片：[docs/agents/workflow.md](file:///D:/tops/idp-parcel/docs/agents/workflow.md)。多会话地盘：[docs/agents/parallel-sessions.md](file:///D:/tops/idp-parcel/docs/agents/parallel-sessions.md)。

## 切片与主责

- 切片：**PN-06 / PN06-W01**
- 主责上下文：**visibility-exception（VE）**
- 主用例：**UC-VE-002**（内部追踪投影）。~~**不要开 UC-VE-008**（客户视图）：投影按包裹立键，不带货主客户账户；派生事件登记会把 ADR-0049 的诚实 `no_subscriber` 变成假接线。~~
  **2026-08-19 晚已解禁**（用户点头）：禁令理由被结构性消解——PS 按包裹反查同行投影列（`0f40744`，ADR-0060）与 VE 账户 ACL 读口（`73cd97d`，`ParcelCustomerAccountLookup`）落地后，账户维取得回，登记不再是假接线。接线工作包 VE-008-WIRE-A/B 已派 MCP-3。
- 权威：
  - [docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md](file:///D:/tops/idp-parcel/docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)
  - [docs/design/pn-06-visibility-and-exception-development-handoff.md](file:///D:/tops/idp-parcel/docs/design/pn-06-visibility-and-exception-development-handoff.md)
  - [docs/domain/visibility-exception/CONTEXT.md](file:///D:/tops/idp-parcel/docs/domain/visibility-exception/CONTEXT.md)
  - [docs/application/visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md](file:///D:/tops/idp-parcel/docs/application/visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md)
  - [docs/adr/0017-admission-gates-judged-by-blocking-cause.md](file:///D:/tops/idp-parcel/docs/adr/0017-admission-gates-judged-by-blocking-cause.md)
  - [docs/adr/0049-…](file:///D:/tops/idp-parcel/docs/adr)（未登记消费者 = `no_subscriber`，不要为了测试绿去登记接不住的类型）

## 当前 `origin/main`（已合入，勿重做）

Tip（交接时）：**`7d7e138`** `feat(dispatch): 交接登记只投 VE 投影不 FanOut PS`

这一段（旧 → 新），细节以各 commit 为准，不要从本表抄实现：

| SHA | 票 | 做什么 |
|---|---|---|
| `a6c6cc6` | FanOut 原语 | 平台 `dispatch.FanOut` |
| `d0de019` | CONS-PROJ-A | VE 消费 `node-intake.formed` |
| `138d361` | CONS-PROJ-B | assemble：收寄 **先 VE 再 PS** |
| `f388c50` | CONS-PROJ-DELIVERY-A | VE 消费有效交付 |
| `5e6db75` + `dc38ec8` | CONS-PROJ-PICKUP-A | VE 消费场外揽收（测试名冲突修过） |
| `0139279` | CONS-PROJ-TF-B | 揽收/交付 FanOut VE→PS |
| `1709872` + `afb57bf` | MAP-KIND | 映射目录按**事实类型**键，不按实例事实引用 |
| `917cb4a` | MAP-KIND-SYN-TF | SYN 可插入隔离映射行证明归类；**生产 assemble 不种 PAR-VIS-01** |
| `47cd536` | CONS-PROJ-HANDOVER-A | VE 消费 `transport-handover.registered` |
| `7d7e138` | CONS-PROJ-HANDOVER-B | assemble 第六路：**只投 VE，不 FanOut PS** |

弃掉的重复：MCP-2 的 `f3a3c2b`（TF-B 兄弟提交，缺 intake-then-delivery SYN）。不要捡。

MAP-KIND 票：`.scratch/ve-milestone-mapping-key/issues/01-mapping-catalog-keyed-on-fact-reference-is-unfillable.md`。共享树上 Status 可能仍是 `in-progress`；**代码已在 `afb57bf` 落地**。共享树落后时以 origin 为准。

## 调度器六路（`cmd/parcel-dispatch/assemble.go`）

1. `parcel-shipment.acceptance-decision.formed` → NR 初路由
2. `parcel-shipment.network-intake.recorded` → NR 复核
3. `node-operations.node-intake.formed` → **FanOut(VE, PS 采认)**
4. `transport-fulfillment.offsite-pickup.registered` → **FanOut(VE, PS 采认)**
5. `transport-fulfillment.effective-delivery.registered` → **FanOut(VE, PS 终局)**
6. `transport-fulfillment.transport-handover.registered` → **仅 VE**（不 FanOut PS。终局只认有效交付。NO/NR 控制转移不是这票）

FanOut 顺序一律 **先 VE 后 PS**，避免投影卡在采认资格 / 终局规则实例墙上。

~~**不要登记** `visibility-exception.tracking-projection.derived`。~~ **2026-08-19 晚已解禁**（用户点头，见上「主用例」条）：VE-008-WIRE-B 将登记该类型接客户视图消费者。派生信封与交接/交付可能同 `(tenant+parcel)` 分区的提示仍有效，登记后既有 assemble 级测试的 no_subscriber/published 计数与 beat 口径要逐个如实更新。

## 诚实实例墙（未变，不要用默认值拆）

- 接受 → NR：`ROUTE_EVIDENCE_NOT_CONFIGURED`（PAR-NET-14）
- 节点收寄 / 场外揽收（PS）：`INTAKE_QUALIFICATION_UNPROVEN`（PAR-COM-16）
- 有效交付 → 终局（PS）：`FINAL_RULE_UNCONFIGURED`（PAR-COM-17）
- 空映射目录 → **`PROJECTION_DERIVED` + 未归类（`MAPPING_NOT_CONFIGURED`）**。不要在生产 assemble 种 PAR-VIS-01。

交接 SYN 未归类成功路径的断言是 **`published==1`**（只投 VE）。毒丸 `{}` 入账后也是 `published==1`。不要改成 FanOut-with-PS 的 `published==0`。

## MAP-KIND 已裁定（写进 CONTEXT，勿重开）

- 事实类型是已接受源事实的一部分（UC-VE-002 输入表），不是 VE 发明。
- `AcceptedSourceFact` 必填 `SourceFactKind`（不封闭枚举）。幂等键仍是（租户+源+事实引用+版本）。Kind 进内容摘要。
- 映射条目键：（租户, 映射版本, 源上下文, 事实类型）。迁移 `0014`（不改写 `0009`）。空库 `ADD COLUMN NOT NULL` 无默认；**永不填 `UNTYPED`**。
- 来源代码本轮可空；映射键不含它。
- 适配器类型字面量：
  - 收寄 `node-intake`
  - 揽收 `offsite-pickup`
  - 交付 `effective-delivery`
  - 交接裁决（HANDOVER-A 实装，**不是**早期票面上的 `transport-handover-*`）：`handover-handed-over` / `handover-refused` / `handover-pending-confirmation`
- 事实引用要加前缀，避免揽收/交付/交接抢同一 `(source, fact, version)`：交接用 `transport-handover/` + object + `/` + scope。
- 同一 Go 包 `visibilityexception/adapters/transportfulfillment`：测试名唯一，哨兵加前缀（`ErrHandover*` vs pickup/delivery）。

## 硬禁止

- 不发明生产默认；隔离 `S` 只记 `S`。
- 不写 PAR-NET-14 解析器、空 `network_definition`、假 CC、SYN `final_rule` 种子、默认 ESTABLISHED、默认生产里程碑、生产 assemble 种 PAR-VIS-01。
- 领域包不依赖 HTTP/`pgx`。中文注释。不要用 `Set-Content` 改中文源文件。
- 跨文件引用用符号名或引文，**不用行号**。
- **永远不要 `git add -A`。** 只 `git add` 本票文件。推的是**已验证 SHA**，不是会动的分支名：`git push origin <verified_SHA>:main`。
- `assemble.go` 是共享接线。B 类票占用它时不要再派第二张碰它的票。A 类（inbox+adapter）可与别的 A 并行，不要两个人同时改同一 adapter 文件。
- 开发方不是运营企业；机制现在做，实例留空。

## 协调协议（工人 MCP-2 / MCP-4）

- 本岗是 **MCP-1**（`project-0-idp-parcel-idp-mcp-1`）。每轮文字回复之后**最后一步**必须 `check_messages({ blockUntilMessage: true, reply })`。Cursor 传输超时则加 `maxBlockMs: 600000`；空队列返回 `[continuation]` 时用相同参数立刻再调。
- 工人：~~MCP-2、MCP-4~~ → **2026-08-20 用户告知：只剩 MCP-4 / MCP-5 存活，MCP-2 与 MCP-3 已死。** MCP-5 是新面孔，本档原先没有它，已置空闲待命。派活用 `send_to_session`。失败则 `set_channel_lock({target:N, locked:false})` 再发。
- **会话死亡会留下无人认领的残局。** MCP-3 死后清点出一条从未有人裁断的未合入平行实现（分支 `mcp3-ve008-wire`）。工人死了它的分支和树不会自己消失，也不会有人替它说话——**换人之后先清点，别默认「没人提就是没有」**。
- **`send_to_session` 看起来没送到**：工人若马上 `check_messages` 且 `reply` 仍是上一轮摘要，面板会像没收到。要求工人**先回一句票名**确认。
- 工人 ack「已开工 / 空闲」之后不要再 ping。
- **验测按包路径跑，不要 `-run "关键字"` 跑子集。** 测试名未必含所属领域词——`internal/visibilityexception` 索赔那一片就有 `TestAMatchingRevisionStillCannotTruncateHistory`、`TestAStaleClaimSnapshotCannotEraseAnApprovedExtension` 这类漏掉 `Claim` 关键字的用例，`-run "Claim"` 会静默少跑而全绿。集成口的全量 `go test ./...` 不受影响，这条是给工人自验用的。改名收益只是可发现性，已裁定不开票。
- 工人在**隔离 git worktree**（从 `origin/main` 长出），不写共享树。集成：另开 detached verify worktree → `gofmt` / `go build` / `go vet` / `git diff --check` / 设 DSN 后 `go test -p 1 -count=1 ./...` → `git push origin <SHA>:main` → `git worktree remove --force` 验证树 → 通知工人自行拆他们的树。
- DSN 与门禁容器见 [workflow.md 本机环境](file:///D:/tops/idp-parcel/docs/agents/workflow.md) 与仓库根 `compose.yaml`。主机端口 **`127.0.0.1:55432`**。报测试状态必须写明**含不含 PG**。

## Docker 暗礁（本会话踩过）

容器名约 `idp-parcel-postgres-gate`，镜像 `postgres:16.14`。

- `healthy` **不够**。必须 `docker compose ps` 看到 **`127.0.0.1:55432->5432`**。若 PORTS 只有 `5432/tcp`（无主机映射），host DSN 会 connection refused；`docker compose up -d --force-recreate`。
- Docker Desktop 引擎 500 / named pipe 丢失时，先救引擎再测，不要把测试红当代码红。
- 全量套件约 2.5–8 分钟（TF/VE postgres 慢时偏长）。中途 Docker 死后不要用中断前的绿当证据。

## 等人点头（未实施）

用户问过仓库要不要一份缩写 MD。协调意见（**未获点头，不要擅自改文档**）：

- 要**索引**，不要第二套术语表。
- 短表放 [docs/domain/CONTEXT-MAP.md](file:///D:/tops/idp-parcel/docs/domain/CONTEXT-MAP.md)：代码 → 中文名 → 该上下文 `CONTEXT.md`。mermaid 已用 PS/VE。
- **不**重定义委托/投影；[GLOSSARY.md](file:///D:/tops/idp-parcel/docs/domain/GLOSSARY.md) 仍是领域词权威。
- 票名 / `FanOut` / `CONS-PROJ-*` 不进领域文档（过期快；真要留放 `docs/agents/`）。
- `UC-*` 读法已在 [docs/application/README.md](file:///D:/tops/idp-parcel/docs/application/README.md)。

## 建议下一票（机制；等人说「继续」再拆）

物理源投影管线（收寄/揽收/交付 FanOut VE+PS，交接仅 VE）已闭合。实例墙仍在。候选按杠杆，**不要并行两张 assemble 票**：

1. **AT-VE-044 更正/迟到走调度器**：UC-VE-002 要求追加投影版本、保留原版本。现有消费门接的是「形成/登记」，更正链是否经 outbox 进同一消费者要先对照源上下文与 CONTEXT，可能要 `/grill-with-docs`。
2. **更多 NO 已接受事实**（扫描/封志等）：先确认源上下文已发布、事实类型字面量、事实引用前缀不撞车，再走 A（inbox+adapter）然后 B（assemble）。B 占用 `assemble.go`。
3. **按包裹反查当事人**（高杠杆、大）：`.scratch/outbox-handoff-consumption-map/next-consumer-survey.md` 写明 PS 采认/终局与 VE-008 都卡「事实取得回、账户/委托取不回」。`shipment_request` 成员包裹埋在 jsonb，要迁移，属 PS owner。做完才能诚实接 UC-VE-008。勘察取证于更早 SHA，派之前对 `origin/main` 重取证。
4. **交接 → NO 控制转出**：领域函数在，缺 UC/编排。不是「再接一条 assemble」。
5. CONTEXT-MAP 缩写表：文档小票，**与 assemble 不冲突**，但必须等人点头。

## 共享树与残留 worktree

`git worktree list` 里有大量旧票残留（`cons-proj-*`、`integrate-*`、`product-version-closure-*` 等）。**不要批量 `worktree remove`**，可能有别人还在用。只拆**自己**开的 verify 树。工人树让工人拆。

**2026-08-20 已按用户裁定清过一轮**（口径：逐棵先证 HEAD 是 `origin/main` 祖先再拆）：

- 已拆六棵（已合入且工作区干净）：`b4`、`dispatch-db-ready`、`integrate-delivery-a`、`integrate-pickup-a`、`integrate-proj-a`、`integrate-proj-b`。
- **`git worktree remove` 不加 `--force`**，脏树会如实拒绝——这一条救了三棵：`adr-0065-storage`（12 改 + 新迁移 `0016`）、`syn-wall-door-audit`（11 个已暂存文件，一份墙/门审计报告加 10 张票）、`bento-gate-reeval`（未跟踪目录）。**加 `--force` 会无声抹掉**。三棵去留已立票 `.scratch/dead-session-salvage/issues/02-*.md`，未获点头前不要动。
- `inspect-03`（detached `a097d7f`）故意留着：名字指向外部评审 03 票，MCP-4 正在办该票，归属未确认。
- 大批 `cons-*` / `ps-*` / `syn-*` 按 SHA 判 UNMERGED，多因集成时另起 `integrate-*` 重做提交——**内容可能已在 main 而 SHA 不是祖先**，不可按上述口径拆。

共享树 `D:\tops\idp-parcel` 上可能有未提交的 `.scratch/`、`.cursor/mcp.json`。那不是本岗的提交范围。

## 通道工具

MCP-1 工具描述在 `C:\Users\topsx\.cursor\projects\d-tops-idp-parcel\mcps\project-0-idp-parcel-idp-mcp-1\tools\`。跨通道：`send_to_session` / `broadcast` / `set_channel_lock`。进度：`save_progress` / `load_progress`。阻塞提问：`ask_question`。

## 本交接不包含

- 密钥、生产参数、真实价卡/合同（仓库里也不该有）。
- 把勘察文档里的「第 3 消费者」清单当现行待办——那份写于 `e1b985a`，此后物理源管线已接上；**结论「反查口仍缺」仍可能成立，清单行号要重读代码。**
