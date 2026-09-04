# `.scratch` 未 resolved 部分对照当前实现的只读审查（2026-09-04）

Category: chore
Status: done——A 组（MCP-4）、B 组（MCP-5，12:5x 交回并入）已写；C 组（MCP-6）未交（会话重启，截至 13:07），票目留在 C 组节待补

## 这份报告是什么、不是什么

用户令 MCP-4/5/6 并行「结合当前实现全面审查 `.scratch` 没有 resolved 的部分」。三会话全程**只读**：不改任何票面 `Status:`、不动代码、不 add 不 commit 别人的东西；本目录是唯一写入点，只 MCP-4 写。

**取证基线**：扫全 `.scratch`（276 份 `.md`）钉在 `08e62ec`（2026-09-04 11:59）。审查期间主线又推进了若干笔（`16fc63e`、`9e3c1a5`、`b99b9e2`、`9a4069e`、`fb3c89f`、`1af59c8`、`04b69b8`，以及 MCP-1 12:5x 推到远端的 `ec995e0`——其广播自述只多一件机制清点 `.md`），其中几张票在本报告写作期间**已由 owner 自行转 resolved**——那是并行工作的正常结果，本报告对这类票只记「取证时如此、现已收口」，不重复评。各条断言各自带自己的取证 SHA。

**不做的事**：不替任何 owner 改票、不裁任何领域问题、不给别的通道派活。建议一律写成「可答的选项」交给 owner 或用户。

**建议用词的封闭集**：`保持` / `可转 resolved` / `可转 superseded` / `需人裁` / `可立即开工` / `应拆票`。

## 全景：按 `Status:` 值分组（钉 `08e62ec`）

扫描方法：逐文件取第一条 `Status:` 行，按首个词分组。数字本身是论点，故锚 SHA，**只作此刻取证，不作别人的基准**。

| 值 | 份数 | 说明 |
|---|---|---|
| `resolved` / `done` | 188 | 不在本报告范围 |
| `superseded` / `handed-off` | 6 | 不在范围 |
| `in-progress` | 22 | 含 spec 与票 |
| `draft` | 12 | |
| `ready-for-agent` | 5 | |
| `needs-info` | 5 | |
| `blocked` | 1 | |
| 无 `Status:` 行 | 24 | 多为 report / census / 简报，见末节 |
| 非标准状态词（「取证完成」「提案存档」「取证快照」…） | 13 | 见末节 |

---

## A 组（MCP-4）：label-channel-service-first-release 余票 · admin-write-faces 02/05/06/07 · auto-reroute-demo-reachability/01

### A 组总表

| 票 | 票面 Status（取证时） | 核实结果一句 | 建议 |
|---|---|---|---|
| label-channel/spec | in-progress | 状态行准；**子票表三行过期**（`13`、`15` 仍写 `ready-for-agent`，`14` 的阻塞栏仍写 `12`）；票 `12` 记下的「整条链组合根与调用入口」**无票承接** | `应拆票`（新子票：面单渠道链的组合根与调用入口）+ 表格对齐 |
| label-channel/11 | draft（Blocked by 06） | `06` 已 resolved，阻塞解除；票问的「与既有终局是不是同一个」**PS `CONTEXT.md` 已正面答**（同一「终局服务结果」，面单渠道服务按自己那组规则形成） | `可立即开工`（票面「做什么」先按 CONTEXT 改写为 ready-for-agent） |
| label-channel/13 | in-progress（待评审） | 三道缝已落 `54ae107`，评审补刀 `5e9d688`；两条棘轮基线条目已随票 `12` 剪掉（有生产调用方）；三包 `go test -count=1` 绿（无 DSN，纯单元） | `可转 resolved` |
| label-channel/14 | draft（Blocked by 01, 12） | 两个阻塞均已 resolved；`channel_candidate_cost.go` 已带四格出局因由（`ChannelCostUnavailability`），留痕对象仍无 | `需人裁`（留痕是事实还是判断过程记录——PAR-NET-16 与 CONTEXT-MAP 已给一半答案） |
| label-channel/18 | ready-for-agent | 前提成立：`ports.ExternalCarrierCredentialResolver` 无生产实现，`MECHANISM-INVENTORY.md`「缺」名单在列；TF 迁移最新 `0011`，票写 `0012` 准 | `保持`（MCP-2 在途：上一段会话 12:22 中断于共享树，留 domain 三件 + ports + `0012` 迁移未跟踪，postgres 适配器未开写；新会话 12:5x 自报从中断点续做，提交前先报） |
| label-channel/19 | ready-for-agent | 前提成立：`ports.EffectiveTimeRules` 无生产实现，清点在列 | `保持`（MCP-2 12:5x 释号，此刻无人占；可直接占，占了广播一声） |
| label-channel/20 | draft（Blocked by 18） | 按票面「随第一家真源立」，形状记录完整；`ports/tracking_source.go` 在 | `保持` |
| label-channel/21 | ready-for-agent | 前提成立：`application/judge_external_tracking_effective_time.go` 在，无端点无页 | `保持`（MCP-2 12:5x 释号，此刻无人占） |
| label-channel/capability-shape-inventory | draft（待 MCP-1 消费） | 已被消费：spec 2026-09-02 MCP-4 Comment 记「两份盘点的建议一并落成 `02`..`17`」 | `可转 resolved`（状态行改「已消费为子票 02..17」） |
| label-channel/tracking-source-seam-inventory | draft（待立票） | 同上，已消费 | `可转 resolved` |
| admin-write-faces/02 | in-progress | 四片实现全落主线；`publication`/`customer-account` 两缺口已由票 `03`/`04` resolved 收走；余项只剩 `pnpm build` 按 MCP-3 定的措辞记法 | `可转 resolved`（余一条见下「A 组顺带发现」第 3 点） |
| admin-write-faces/05 | resolved（未提交） | CLI 第八族已提交 `6e40312`；「未提交」三字过期 | `可转 resolved`（去掉「未提交」） |
| admin-write-faces/06 | draft（先取证） | 票要的取证**本报告已代做**：`migrations/party_commercial/` 无该策略正文表，只有合同级声明 `0007_pre_acceptance_control_declaration` 与版本壳；SA 读路径 `settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go` 读的是闭合 + 声明，不读策略正文 | `需人裁`（二选一：按票面判据第二条记「只有壳」收口；或升级为 PC 正文表票，形同 `party-commercial-context-gaps/03`） |
| admin-write-faces/07 | draft（伞票） | 无阻断；票 `03` 的 JSON 镜像签已在 `pages/party/CommercialPoliciesPage.tsx`；九册一册未拆 | `保持`（拆票归 owner；建议先拆四本「低频 · 逐字段表单候选」） |
| auto-reroute-demo-reachability/01 | draft（依赖链未复核） | 票要求的复核**本报告已代做**：`cmd/parcel-dispatch/assemble.go` 已接 `NewIntakeAdoptions`、`NewReassessOnNetworkIntakeAdapter`、`NewNetworkIntakeConsumer` 与 `effective-delivery.registered` 消费链，即 `synthetic-vertical-closure/design.md` 表里 `CONS-INTAKE` / `CONS-DELIVERY` / `CONS-INTAKE-REASSESS` 三行的依赖已落 | `需人裁`（缺的变成一个具体问题：真实 `InitialRouteJudgmentKey` 六维从哪个读面取出来喂给 `parcel-network-register -kind auto-reroute-facts`；定了即可转 ready-for-agent） |

### A 组逐票

#### label-channel-service-first-release/spec.md

- **(a) 票面 vs 事实**：状态行「`01`..`10`、`12`、`15`、`16`、`17` 已 resolved」与各票文件一致（`b99b9e2` 已把 `10` 对齐）。子票表内三处与票文件不一致：`13` 行写 `ready-for-agent`（票文件 in-progress、实为已落地）、`15` 行写 `ready-for-agent`（票文件 resolved，`7904003`）、`14` 行阻塞栏写 `12`（`12` 已 resolved）。
- **(b) 实现缺口（spec 级）**：票 `12` 收口 Comment 明写「生产可达仍差两步：组合根与调用入口」并建议另立票；`cmd/` 下 `SelectChannelCandidate` / `AssembleChannelCandidates` / `ChannelCandidateCostSource` 零命中（核于 `08e62ec`），即票 `06` 编排、`07` 出向端口、`12` 装配与择优整条面单渠道链**没有装配点**。spec 子票清单 `18`–`21` 全是轨迹侧，没有一张承接这两步。
- **(c) 阻断**：无。
- **(d) 冲突**：无。
- **(e) 建议**：`应拆票`——新子票「面单渠道链的组合根与调用入口」，内容照票 `12` Comment 末条（择优是运营端点还是面单交易编排的前置步，是产品流程决定；票 `14` 留痕会改择优结果形状）。同笔把表格三行对齐。
- **(f) 归属**：spec 由立票方（MCP-4 前会话）与 MCP-1 交替维护；MCP-1 12:20 已释号 label-channel/10 相关文件。

#### label-channel/11 包裹终局的跨交易判断

- **(a)**：`Blocked by: 06`，`06` 已 resolved → 阻塞解除，票面未更新。`internal/parcelshipment/domain/label_transaction.go` 聚合注释仍明写包裹终局不在聚合内；无执行器承接（`application/` 下只有 `form_parcel_final.go` 的 UC-PS-004 来源采纳路径）。
- **(b)**：缺一个跨交易判断的执行器。输入侧三样今天都已存在：该包裹全部面单交易（票 `06`/`10` 的登记册）、包裹级继续尝试判断（票 `10` 的 `Judge`）、TF 首次有效收寄事实（`transportfulfillment` 收寄/外部承运轨迹事实，票 `16`）。
- **(c)**：票要求「与既有终局是不是同一个，要正面答」——**PS `CONTEXT.md` 已答**：词条「终局服务结果」（「网络服务和面单渠道服务可以具有不同终局结果」）、规则「首个面单渠道服务产品默认跨同一包裹的全部相关面单交易判断终局。除已经形成的有效取消结果外，实际承运商首次有效收寄即形成终局；否则…」以及生命周期节「`transport-fulfillment` 提供实际承运商首次有效收寄事件 → 面单渠道服务非取消终局结果」。即：**同一个概念（终局服务结果），不同的形成规则集**。
- **(d)**：无冲突；反而是票面把一个 CONTEXT 已答的问题当成待裁——与票 `04` 当初「零命中只证明代码里没有，不证明没人裁过」同一形。
- **(e)**：`可立即开工`。开工前把「做什么」按 CONTEXT 那三处改写；剩下的工程选择（复用 `FormParcelFinalHandler` 加一种来源 vs 并列执行器）不需要新 ADR，但要在票面写理由——`FinalRuleView` 今天的规则形状是否装得下「全部相关面单交易均已定案且无有效/待确认面单结果」这类跨聚合谓词，是那一步的关键。
- **(f)**：无人占。

#### label-channel/13 `BUY` 批量评价与成本分值桥

- **(a)**：三道缝落 `54ae107`（`parcelpricing/domain/batch_evaluation.go`、`parcelshipment/adapters/parcelpricing/cost_bridge.go`、`parcelshipment/domain/channel_candidate_cost.go`，各带 `_test.go`）；`5e9d688`「成本分值桥注释去掉跨包计数与『必须红』的过强措辞（票 label-channel/13 收口评审）」即票面说的「待评审」已发生。
- **(b)**：无缺口。票面末条 Comment「未接生产调用路径…等票 `12`」已被 `12` 兑现：`production_wiring_baseline.txt` 注释记 `SelectChannelCandidateByCost` 与 `EvaluatePricingAcrossPlans` 两条**均已剪掉**，各有非测试调用方（`application/select_channel_candidate.go`、`adapters/parcelpricing/cost_source.go`）。
- **(c)**：无。
- **(d)**：无。验证：`go build ./...` 退 0；`go test -count=1` 于上述三包 `ok`（本机未设 DSN；三包无真库用例，此处「绿」是纯单元绿）。
- **(e)**：`可转 resolved`。完成判据四条（批量口、分值桥、出局不以零金额顶替用例、四门禁）票面自证已满足，只差状态行。
- **(f)**：票面持有 MCP-6（接手方）；MCP-6 12:0x 自报「已在 `54ae107` 落地待评审、无在途件」。

#### label-channel/14 落选留痕对象

- **(a)**：`Blocked by: 01, 12` 两者均 resolved，票面未更新。
- **(b)**：留痕对象与读面均无。已有的一半：`channel_candidate_cost.go` 的 `ChannelCostUnavailability` 四格（对应 parcel-pricing 四种非完成结果）与 `UnpriceableChannelCandidate` 带因由——票 `13` Comment 明说「票 `14` 的落选留痕要答的正是这个」。
- **(c)**：无阻断。
- **(d)**：票问「留痕是不是事实」——权威文档给了一半：`PAR-NET-16`「日常择优可在已确认候选集合内自动进行并**逐次留痕**」「择优决定的留痕要求与逐票样本」列在**待提供**栏；CONTEXT-MAP「未被选中的候选评价继续有效并留痕」说的是**评价**（parcel-pricing 拥有、不删）。即：评价那半已由 PP 不删保证；缺的是**择优决定本体**（引用各候选的成本/出局格）的登记。
- **(e)**：`需人裁`，问题可收成一问：择优决定是否作为一条只追加的决定记录落 PS 侧（引用候选与四格因由，不拷内容）——是则票转 ready-for-agent；否（认为评价留痕已够）则转 wontfix 并在 spec 写明。
- **(f)**：无人占。

#### label-channel/18、19、21（MCP-2 11:55 起占号顺序 18→19→21；12:5x MCP-2 新会话只续认 18，19/21 释号）

- **(a)**：三票前提逐条成立（核于 `08e62ec`）：`internal/transportfulfillment/ports/external_tracking_fact.go` 有 `ExternalCarrierCredentialResolver`（含 `CredentialRegistryUnconfigured` 格）与 `EffectiveTimeRules`（含 `EffectiveTimeRuleAbsent` 格）两接口，`application/judge_external_tracking_effective_time.go` 在；`docs/product/MECHANISM-INVENTORY.md` 把这两个端口列在「缺」名单，且把 `TrackingSource` 标「虚高：名字出现过，但无人实现」。
- **(b)**：缺口即票面所写；`migrations/transport_fulfillment/` 最新 `0011`，`18` 预留 `0012` 准确，`19` 若也落表须取 `0013`。
- **(c)**：`Blocked by: 无`，`16` 已 resolved（`2bb9300` 落文 + 三笔实现）。ADR-0100/0101/0102 均在 `docs/adr/`。
- **(d)**：无。
- **(e)**：`保持`。就绪度足够。两处提醒给实施方：`21` 要在 `cmd/parcel-api/endpoints.go` 加行（共享接线文件）；`18` 的 `0012` 已在共享树上占号（未跟踪），`19` 若落表与 tf/02 都从 `0013` 起按开工那刻重取——`18`/`19` 如今不再是同一会话顺序做，撞号风险从「无」变成「要看树」。
- **(f)**：`18` MCP-2（新会话续做，共享树，提交前先报；`git worktree list` 无 mcp2 worktree，A 组原记的「12:40 隔离 worktree」未落地）；`19`、`21` 已释号，此刻无人占。

#### label-channel/20 轨迹拉取节拍

- **(a)–(d)**：票面自述「随第一家真源的票一并给出；本票在此之前保持 draft」，与现状一致；`ports/tracking_source.go` 的 `TrackingSource` 接口在，无实现（清点标「虚高」）。
- **(e)**：`保持`。
- **(f)**：无人占。

#### label-channel 两份盘点（capability-shape-inventory.md、tracking-source-seam-inventory.md）

- **(a)**：状态行各写「待 MCP-1 消费」「待立票」；spec 2026-09-02 MCP-4 Comment 记「轨迹源盘点已交，两份盘点的建议一并落成 `02`..`17` 共十六张子票，清单至此完整」。已消费。
- **(e)**：`可转 resolved`（或按只读产物惯例写「已消费为子票 02..17（`9e6d53a`）」）。

#### admin-write-faces/02 其余登记册逐个接在线登记口

- **(a)**：状态行「四片实现已全部落主线（`1bd9e9d`），余共享件一格在途」——`1bd9e9d` 实为 35 行票面注记，不是实现提交；四片的实现 SHA 在票面「已落地实况」表与 MCP-1 接手表里（`4776670`…`81b05d3`），`git grep -l 'components/registration' HEAD -- apps/admin-web/src/pages` 22 份页面在册。「余共享件一格」票面未指名，按上下文只剩 `pnpm build`（共享工具链）那一格，且 MCP-3 已定记法「本机跑不了，以 `tsc --noEmit` 代替」。
- **(b)**：02a–02d 各片完成判据票面逐条自证；02e 用户裁排除；两缺口 `publication`（票 `03` resolved）、`customer-account`（票 `04` resolved，`0d4eb0d`）已收。
- **(c)**：无。
- **(d)**：无。票面**一处事实错**（不影响结论）：MCP-5 Comment 写「网络片把译装下沉成 `internal/networkrouting/adapters/registrationjson`」——该目录不存在（核于 `1af59c8`）；实际存在的是 `customscompliance/adapters/registrationjson` 与 `visibilityexception/adapters/registrationjson`。
- **(e)**：`可转 resolved`，完成记录里把 `pnpm build` 一格按已定措辞写明。
- **(f)**：最后一位整合者 MCP-1；无人在途。

#### admin-write-faces/05 自动改路四条件事实登记入口

- **(a)**：状态「resolved（未提交）」；`git log -- cmd/parcel-network-register/` 最新 `6e40312`「自动改路四条件事实增第八族登记入口（票 admin-write-faces/05）」，`HEAD:cmd/parcel-network-register/main.go` 含 `auto-reroute-facts`。已提交（且随 MCP-1 12:20 那次推送在远端）。
- **(e)**：`可转 resolved`（去掉「未提交」）。
- **(f)**：MCP-4 前会话；本会话只读不改。

#### admin-write-faces/06 接受前财务控制策略版本无读面

- **(a)**：draft，票面要求「先取证这一类版本今天带什么正文」。
- **(b) 取证（本报告代做，核于 `08e62ec`）**：`migrations/party_commercial/` 里与之相关的只有 `0007_pre_acceptance_control_declaration.sql`（合同级声明，答「这份合同要不要控制」）、`0005`/`0006` 的判断类型枚举；**无策略正文表**。SA 侧 `settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go` 的 `LoadControlPolicy` 由 `closures` + `declarations` 两个只读半边装配，即结算解析读的是**商业闭合 + 合同声明**，不读任何策略版本正文。结论：该对象类别今天**只有 `commercial_version` 壳**。
- **(c)**：无阻断。
- **(d)**：与 `party-commercial-context-gaps/03`（信用政策与供应商协议补正文表，resolved `877444a`）同族——那张票补了两类正文，没补这一类。
- **(e)**：`需人裁`，二选一：① 按票面完成判据第二条收口——票面写明「只有壳、列壳无信息量」并把这一句记进票 `03` 的提示句（纯票面/前端提示句改动）；② 认为策略版本该有正文（「控制怎么做」），另立 PC 正文表票，形同 `party-commercial-context-gaps/03`。选 ② 要先回 PC `CONTEXT.md` 看「接受前财务控制策略」词条要求带什么。
- **(f)**：无人占。

#### admin-write-faces/07 商业发布九类的运营主路径（伞票）

- **(a)–(d)**：无阻断（票 `03` 已落 JSON 镜像签，`apps/admin-web/src/pages/party/CommercialPoliciesPage.tsx` 与 `api.ts` 含「受控发布」）；九册一册未拆，无代码可核。
- **(e)**：`保持`。若要推进，四本「低频 · 逐字段表单候选」（`SERVICE_PRODUCT`、`CUSTOMER_CONTRACT`、`SUPPLIER_AGREEMENT`、`SETTLEMENT_POLICY`）可先各拆一张；`PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 那册依赖票 `06` 的裁决。
- **(f)**：无人占。

#### auto-reroute-demo-reachability/01 demo 里自动改路那条路走不到

- **(a)**：draft；票面把「先复核 `synthetic-vertical-closure/design.md` 推进表的依赖链」列为开工第一步，并指出该表 `CONS-INTAKE-REASSESS` 行已知过期。
- **(b) 复核（本报告代做，核于 `08e62ec`）**：`cmd/parcel-dispatch/assemble.go` 已装配 `pspostgres.NewIntakeAdoptions`、`nrparcelshipment.NewReassessOnNetworkIntakeAdapter`、`nrinbox.NewNetworkIntakeConsumer`，以及只接 `effective-delivery.registered` 的终局链；`nrpostgres.NewAutoRerouteFactsCatalog` 亦在。对照 design.md 表：`CONS-INTAKE`、`CONS-DELIVERY`、`CONS-INTAKE-REASSESS` 三行的「依赖」栏所指今天都在装配里；`PS-PARCEL-INDEX` 对应的读口由 `ve-008-late-account-rederive/02`（resolved）落。**依赖链已落，票面那句「未复核」可以收掉。**
- **(c)**：解除后剩下的不是依赖，是一个具体设计问题：真实 `InitialRouteJudgmentKey` 六维（后四维是运行时产物）**从哪个读面取出来**，才能喂给 `parcel-network-register -kind auto-reroute-facts`。候选：路由计划读面（`networkrouting/adapters/http/query_route_plans.go`）是否已透出判断键；若没有，是加读面还是让 CLI 接受「委托号 + 包裹号」由服务端解析成键。
- **(d)**：红线「不得在 `seed.sh` 塞事实」与「不改 `assemble.go`」仍成立。
- **(e)**：`需人裁`（上述一问），定了即可转 ready-for-agent；也可 `应拆票`——把「demo 运行时走通一条委托→接受→初始路由→复核」与「按真实键登记事实」分开。
- **(f)**：无人占。

### A 组顺带发现（不在任何票面上）

1. **面单渠道链无组合根**（见 spec 条）——这是 A 组最大的一处「没有票面在盯的空白」，与 `admin-write-faces/05` 立票时发现的那种「两头都指过却从未立」同形。
2. **label-channel spec 子票表与票文件的状态不同步**（`13`/`15`/`14` 三行）。表是第二套口径，AGENTS「单一权威」那条建议表格只留链接与一句内容、状态以票文件为准。
3. **商业（PC）与网络（NR）两口译装不同源**：`admin-write-faces/02` MCP-5 Comment 第六节记「商业片比网络与 VE 少一道锁」，实际网络也没有（`internal/networkrouting/adapters/registrationjson` 不存在，核于 `1af59c8`）；`customscompliance` 与 `visibilityexception` 有。这条今天没有票，只靠人工核对在守。

### A 组没核到的东西与原因

- 未跑全仓 `go test ./...`：树上有他人在途件（MCP-2 的类型棘轮此刻已入库 `16fc63e`，但审查期间曾为未跟踪件），且全仓验证归推送方；只对票 `13` 三包跑了 `-count=1`。
- 未设 DSN，所有「绿」都是无真库绿；A 组各票无一条结论依赖真库用例。
- 未打开 `docs/wooolink/`、`docs/reference/xls/` 客户文件。
- `admin-write-faces/02` 的「余共享件一格」是按上下文推断为 `pnpm build`，票面没指名——这是**推出来的**，不是量出来的。

---

## B 组（MCP-5）：tf-segment-lifecycle-closure · mechanism-executor-triage · first-tenant-runway

> 并入注（MCP-4）：以下为 MCP-5 `report_task` 交回内容，做了两处不改断言的整理：①标题层级对齐 A 组；②原文代码引用带的行号（`file.go:NNN`、`L359` 一类）一律去掉、只留文件与符号名——按 AGENTS「不用行号」那条，行号在下一笔提交后就指错且无人报警，符号名足够定位。B 组开工锚 `08e62ec`，收口锚 **`04b69b8`**（12:18:54）——凡 B 组写「HEAD」均指后者。B 组交回后 MCP-1 又推了 `ec995e0`（自述只多一件 `.md`），不影响 B 组任何结论。原文（含行号）仍在 MCP 任务记录 `task-cc197865` 里可查。

**B 组共享树在途件（只看不动、不评）**：`internal/transportfulfillment/domain/external_carrier_credential{,_rehydration,_test}.go`、`internal/transportfulfillment/ports/external_carrier_credential.go`、`migrations/transport_fulfillment/0012_external_carrier_credential.sql`（未跟踪，mtime 今晨；按频道广播属 MCP-2 的 label-channel/18–21）；`.scratch/unresolved-review-20260904/`（MCP-4）。工作树 `--ignore-cr-at-eol` 下**零真改**（74 个 M 全是 CRLF）。

**B 组构建信号**：
- 共享树：`go build ./...` 退 0；`go vet ./...` 在 `internal/transportfulfillment/domain_test` 红（`external_carrier_credential_test.go` 引用未定义的 `domain.ExternalCarrierCredentialSpec`）——**他人在途件，不报**。`go test -count=1 -v`（无 DSN，全部「无真库绿」）：`internal/parcelshipment/adapters/inbox` 11 PASS/21 SKIP；`internal/platform/dispatch` 18 PASS/6 SKIP；`internal/transportfulfillment/application` 118 PASS；`internal/parcelshipment/domain` 200 PASS；`internal/parcelshipment/application` 169 PASS；`internal/architecture` **因他人在途件未能验证**（两道棘轮均指名上述未跟踪 TF 类型——棘轮本身按设计在报）。
- **干净 detached worktree（`$TEMP`，钉 `04b69b8`，用后已删）**：`go vet ./...` 退 0；`internal/architecture` 43 PASS/0 FAIL；`internal/transportfulfillment/domain` 81 PASS。无 DSN，未跑真库。

### B 组总表

| 票 | 票面 Status（HEAD） | 核实结果一句 | 建议 |
|---|---|---|---|
| tf-segment-lifecycle-closure/spec | in-progress | 状态句与 9 张票面逐票一致（01/03–07 resolved SHA 全在 HEAD 祖先链）；02 ready、08/09 draft 属实 | `保持` |
| tf/02 actual-carrier-judgment | ready-for-agent（MCP-2 占） | ADR-0103 `37fcf2d` 已落、TF CONTEXT 三处+Boundaries 两句、GLOSSARY「实际承运商判断」词条均在；代码零形状（domain/ports/adapters/migration 一层都没有）；票面一处过时协调句 | `可立即开工`（已被 MCP-2 认领；两处票面小修见下） |
| tf/08 offsite-pickup-correction | draft，Blocked by 无 | 票面三条代码断言逐条为真：`RegisterOffsitePickupHandler` 只有 `Register`、`OffsitePickup` 无 `Corrects`、登记册只插不改、端点表无更正口 | `需人裁`（MCP-3；两选项已列） |
| tf/09 arrival-triggers-dispatch-task | draft，**Blocked by: 05** | 05 已 resolved（`4c48bac`），阻塞边已失效但票面未更新；「派送范围」全仓只在 UC-TF-006 一处；NR 服务区域无地理覆盖列 | `需人裁`（MCP-3）+ 票面改 Blocked by |
| mechanism-executor-triage/spec | in-progress | 处置裁决六条：1 未产出、2 **半做**（`NewDecimal` 已删 `9699b8e`，PP 三条仍在）、3 done、4 done（05 已 resolved）、5 **未做**（过期免责句仍在基线）、6 归 owner；基线 25 条（HEAD 上数） | `保持`；建议把 2 余项与 5 各落成可派小票 |
| mech/03 fourteen-only-tests-call | in-progress（MCP-3 取证中） | 十四条在 HEAD 基线里一条不少；票内无十四行产出；无 SA/CC/VE 实现票 | `保持`（归 MCP-3） |
| mech/05 type-reachability-ratchet | **resolved**（`16fc63e`，票面 `9e3c1a5`） | 派工时为 in-progress，审查窗口内已收口：第二道棘轮+基线 63 条落地、探针已删、README 留指针；干净树上棘轮 43 PASS | `可转 resolved`（已转） |
| first-tenant-runway/spec | in-progress | 子票表只列 01–06，**07/08/09 不在表内**；状态句仍写「01/03 ready-for-human…06 draft」且自注「未逐票重写」；实际：01/02/04/05/06 resolved、03 blocked、07/08 in-progress、09 needs-info | `保持`；需 owner 补表与状态句 |
| ftr/03 network-resolution-layer | blocked（无 `Blocked by:` 行） | `PAR-NET-14` 登记册仍「待提供」；`network_definition` 零生产写入方；0008 内容列未建（NR 迁移止于 0009）；ADR-0068 决定六在力；三哨兵在 | `保持` blocked；票面补 `Blocked by: PAR-NET-14` |
| ftr/06 declared-value | **resolved（形态半边）** `1af59c8` | 派工时 draft；两份 CONTEXT 词条 `fb3c89f`、GLOSSARY/UC-PS-001 同笔；**承诺的「落点另立实现票」尚未立**（`.scratch` 搜「保价」只命中 06 与 spec） | `可转 resolved`（已转）；缺一张后继实现票 |
| ftr/07 undecided-retry-budget | in-progress（MCP-1） | D1–D3 与 0011 属实；`undecidedDisposition` 过渡态在；D4/D5 缺口逐符号核实与 MCP-1 `04b69b8` 切法一致 | `保持`（MCP-1 在途，切法已定） |
| ftr/08 undecided-stage-and-reason | in-progress（MCP-1） | 第二层属实（`DeliveryFailureObserver` + `logDeliveryFailure` + 真图用例）；第一层挂 07 第二笔 | `保持`（随 07 收口） |
| ftr/09 customer-supplement-self-heal | needs-info | 三问中两问今天就能从代码答：受控补充编排**不铸信封且无生产调用方**；`waitingOn=CUSTOMER_SUPPLEMENT` 结构上永不落库 | `需人裁`——可从 needs-info 转 ready-for-human |

### B 组逐票

#### tf-segment-lifecycle-closure/spec.md

- **(a)**：Status 行列 01/03/04/05/06/07 resolved；票面 SHA `2b4f6d7 722e846 9ced5ae cf8a666 ae97d3e 4c48bac 275646e 14388c5 fad9d87 8165c84 3b6c480` 全部 `merge-base --is-ancestor` HEAD 成立。01 完工判据的 `close_fulfillment_segment.go` 在 `internal/transportfulfillment/application/`。
- **(b)**：无——spec 本身不含实现。
- **(c)**：无阻塞边。
- **(d)**：无冲突。
- **(e)**：`保持`。
- **(f)**：目录无单一 owner 广播；02 由 MCP-2 占。

#### tf/02 实际承运商判断

- **(a)**：ADR-0103 `37fcf2d`（2026-09-03 20:13）Status Accepted；TF CONTEXT 词条、Rules、Lifecycles 各一处与 Boundaries 两句；GLOSSARY「### 实际承运商判断」。`grep 实际承运商 internal/transportfulfillment` 仍只命中注释（`effective_delivery.go`「等实际承运商判断对象落地后再挂」等 5 处）。票面「三处追加」属实。
- **(b)**：缺口=票面「要做的」全部：domain 无判断聚合；`ports/` 无判断登记册端口（`ports.go`/`external_tracking_fact.go`/`tracking_source.go` 里只有 `CarrierAcceptance`、`ExternalCarrierCredentialResolver`、`CarrierReference string`，无 Judgment）；`adapters/` 只有 `http postgres trackingsource` 三目录，**无 `adapters/partycommercial/`**；TF 迁移已提交到 0011，**共享树上 `0012_external_carrier_credential.sql` 已被在途件占号**——ADR-0103 Consequences 写的「`0012` 起按当时下一号」实际要取 0013+。PC 侧可依托 `ports.PartyIdentityRegistry`（found=false=从未登记）与 `PartyIdentityCatalogueRead`，存在性读口的消费侧适配器要新建。挂点 `enter_fulfillment_segment.go` 在。
- **(c)**：Blocked by 无，属实。
- **(d)**：无冲突；mech spec「第四格」表已有本项一行（写「已裁形状，进实现票」），票面末条「完成后…追一行」所指已被提前做过，实现完只需把处置改为已落地。
- **(e)**：`可立即开工`（已被认领）。票面两处小修：①「要做的」里「票 03 拆出的 06 号票在动那一段，先在频道对一下顺序」——06 已 resolved（`8165c84`），协调句过时；②迁移号按开工那刻重取。
- **(f)**：MCP-2（11:55 广播占号）。同一人在 `internal/transportfulfillment/domain` 另有在途件，无跨会话冲突。

#### tf/08 揽收登记的更正

- **(a)**：三条代码断言在 HEAD 逐条为真：`register_offsite_pickup.go` 的 `Register` 是唯一导出方法（其余 `establishSegment/existingResult/handOff` 未导出）；`domain/offsite_pickup.go` 有 `PickupResultVersion` 而 `OffsitePickup` 无 `Corrects`（对照：`TransportHandover`、`EffectiveDelivery`、`TransportMovementFact`、`LoadAssignment`、`TransportChargeOccurrence` 五个聚合都有 `Corrects`+版本链）；`ports.go` 的 `OffsitePickupRegistry{FindByKey, Save}` 只插不改；`cmd/parcel-api/endpoints.go` 只挂 `/offsite-pickups` 与 `/offsite-pickup-attempts`，无 `/offsite-pickup-corrections`。
- **(b)**：裁前无实现，票面自述四层一次建设。
- **(c)**：Blocked by 无属实，但真正等的是 MCP-3 的领域裁决——Status 写 draft 而非 ready-for-human，tracker 上看不出「等人」。
- **(d)**：无冲突。
- **(e)**：`需人裁`，可答的选项：A「新版本」（照交接/交付/移动事实/装载分配/费发生项五处既有形状：`Corrects` 回指、新版本新登记、PS 采用按版本幂等）；B「失效+替代」（要给 `OffsitePickupRegistry` 开改写口，打破本上下文全部登记册「只插不改」的一致形状）。附问：更正改了控制证据/发生时刻时，对象参与起点是同段新起点还是新段（与 02 的「不追溯覆盖」同族）。证据倾向 A 只是形状一致性，不替裁。建议 Status 改 ready-for-human 以便派发。
- **(f)**：待 MCP-3 裁；无人占号。

#### tf/09 到达事实触发建立派送任务

- **(a)**：**`Blocked by: 05` 已失效**：05 resolved（`4c48bac`，2026-09-03 20:30），票面未更新。其余断言属实：`MovementFactKind` 封闭三值含 `ArrivalFact→"ARRIVAL"`（`load_assignment_movement.go`）；移动事实挂班次不挂对象/段（同文件注释）。
- **(b)**：`RecordMovementFactDeps{Facts, Clock}`（`record_movement_fact.go`）无任何派送任务钩子；`OpenDispatchTask` 生产唯一调用方是 admin 写面装配 `cmd/parcel-api/assemble_segment_operations.go`。「派送范围」全仓（排 archive）只命中 `UC-TF-006` 触发句，TF CONTEXT 无词条；NR 侧 `ports.ServiceAreaDefinitionVersion`（`catalog_registration.go`）自注「覆盖内容列未定」——若裁成读 NR 服务区域，今天没有地理内容可判，会**连上 first-tenant/03 的 `PAR-NET-14` 阻断**。
- **(c)**：05 已落，阻断解除；真正的等待是 MCP-3 裁两问。
- **(d)**：无冲突，但 NR 路会继承 ADR-0068 决定六护栏。
- **(e)**：`需人裁`：①「到达」=对象计划段终点还是班次终点、推导链（到达事实→装载分配成员→对象）归 TF 否；②「派送范围」是 TF 自有词（加 CONTEXT 词条）还是 NR `ServiceArea`（读判断结果还是收事件，且受 PAR-NET-14 阻断）；③触发落层：`RecordMovementFactHandler` 同事务调 `OpenDispatchTask`（06 形状）还是异步执行器；④ `OpenDispatchTaskCommand` 七件里时间窗从哪来。票面同时改 `Blocked by: 无（05 已 resolved 4c48bac）`。
- **(f)**：待 MCP-3 裁；无人占号。

#### mechanism-executor-triage/spec.md

- **(a)**：Status 仍写「另立类型侧名单（票 05）」——05 已 resolved（`16fc63e`/`9e3c1a5`），状态句可补一句。基线 `production_wiring_baseline.txt` 在 HEAD 上重数 **25 条**（PS 2、VE 6、TF 0、SA 4、CC 4、PC 5、PP 4；最后一笔改动 `220619a` 09-03 17:29）。
- **(b)**：处置裁决六条落地情况：①十四条取证——未产出（见 03）；②PP 死码——`NewDecimal` 已删（`9699b8e`，09-03 11:17；`decimal.go` 无声明），**但 `MarshalPricingPlanSnapshot`/`RehydratePricingPlanSnapshot`（`plan_snapshot.go`）与 `ParseCanonical`（`decimal.go`）仍在代码与基线，基线注释仍是 spec 已判「过期」的那句「同一条未接线路径的两端…一次修好会同时去掉两行」**；③TF 七条——done；④票 05——done；⑤基线过期免责句「按本仓机制先行实例后到的顺序，切片还没开工的能力本来就该是这样」**仍在基线头注**（`220619a` 之后没有人改过基线，「由下一个改基线的人删」尚未触发）；⑥归 owner。「第四格」两行（实际承运商判断→02、客户服务规则正文→pc-gaps/05）均已进实现票。
- **(c)**：无阻塞边。
- **(d)**：无冲突。
- **(e)**：`保持` in-progress；建议：把②余项（PP 两个平行写法删/`ParseCanonical` 二选一，parcelpricing 地盘）与⑤（删免责句，随下一次基线改动）各落成可派小票或写进 03 的收口清单，否则它们只活在 spec 末节。
- **(f)**：目录持有者 MCP-3（自决授权）。

#### mech/03 只有测试调得到的十四条

- **(a)**：十四条在 HEAD 基线里一条不少（SA `FormAuditedPayable FormSupplierCreditNote IncludeAdjustmentInSubsequentPeriod FormSupplierExpectedCost`；CC `FormDutyCollaboration ReceiveReleaseOutcome RegisterCredential VerifyDutyPayment`；VE `DecideDisclosure EstablishCase PrepareDisclosure RaiseConflictSignal ResolveByBusinessTime SubmitEvidence`）。票内无十四行产出；`.scratch` 搜 `FormAuditedPayable|RaiseConflictSignal` 只命中普查/棘轮票，**无 SA/CC/VE 实现票**。MCP-3 12:05 广播自证「十四行取证尚未产出」。
- **(b)**：本票不接线；产出物缺席。
- **(c)**：无阻塞。
- **(d)**：无。
- **(e)**：`保持`（MCP-3 12:15 已列为第三件）。
- **(f)**：MCP-3。

#### mech/05 类型可达性棘轮

- **(a)**：派工时 in-progress，审查窗口内 `16fc63e`（12:07）落代码：`production_type_reachability_ratchet_test.go`（+576）、`production_type_reachability_baseline.txt`（+152，首版 **63 条**，含「待改小写」候选一节）、删 `.scratch/domain-executor-audit/probe/main.go`（−405）、README +5；`9e3c1a5` 票面转 resolved。干净树 `internal/architecture` 43 PASS。共享树上该棘轮对未跟踪 TF 类型报红——按设计在工作，不算票的问题。
- **(b)–(d)**：无。
- **(e)**：已 resolved，同意。
- **(f)**：MCP-2。

#### first-tenant-runway/spec.md

- **(a)**：Status 「in-progress」无摘要；子票表只到 06，**07/08/09 三票不在表内**（它们是 09-02 追加的 bug 票）；状态句仍是立批时文字，`1af59c8` 只括注「06 已 resolved，其余以票面为准，本行未逐票重写」。逐票实况：01 resolved（ADR-0091）、02 resolved、03 blocked、04 resolved、05 resolved、06 resolved（形态半边）、07 in-progress、08 in-progress、09 needs-info。
- **(b)**：n/a。
- **(c)**：06 的优先级阻塞边已按票面规则解除。
- **(d)**：无。
- **(e)**：`保持` in-progress；需 owner 一次表面收口：补 07/08/09 入表、重写状态句为「以票面为准」的一行、Status 补摘要。
- **(f)**：MCP-6 立批；无 owner 广播。

#### ftr/03 网络解析层

- **(a)**：blocked 属实：`PILOT-PARAMETER-REGISTER.md` 的 `PAR-NET-14` 仍「待提供」，最低证据首项仍是「候选生成/过滤/排序规则、服务区域、业务时区、服务日历、截单…」整行；`network_routing.network_definition` 生产侧只有 SELECT（`network_definition.go`），INSERT 只在 `network_definition_test.go`；NR 迁移 0001–0009，无扩内容列的新迁移，`0008` 头注仍四处指 PAR-NET-14；三哨兵 `RouteEvidenceNotConfigured/NetworkEvidenceNotConfigured/ReassessEvidenceNotConfigured` 在 `create_initial_route.go/assess_parcel_reachability.go/reassess_route.go` 各 4–5 处；ADR-0068 Status Accepted、决定六「三个证据视图仍不读本目录」在力（README 只记 0053 被 0068 部分停用，0068 自身未被停用）。
- **(b)**：缺口=整个解析层，票面 Answer 第四节已把第一步钉为「view_revision 改由目录修订派生」（0007 已按校验和固定，改列走新迁移）。跨批：label-channel/01 已 resolved；13 in-progress（MCP-6）。
- **(c)**：阻断仍真且属实例半边，工程侧不可自解。
- **(d)**：无。
- **(e)**：`保持` blocked。票面用 Status 说 blocked 但**没有 `Blocked by:` 行**，建议补 `Blocked by: PAR-NET-14（实例半边）` 让 tracker 能按字段过滤。
- **(f)**：无人占号（MCP-5 09-02 答完三问后释）。

#### ftr/06 保价与声明价值

- **(a)**：派工时 draft；`fb3c89f`（12:14）落两份 CONTEXT 词条（parcel-shipment「保价要求」+3、party-commercial「保价条件」词条 +3 与不变量 +1），`1af59c8` 票面 Answer、GLOSSARY +16、UC-PS-001 第 5/9A 步各改一行、spec 阻塞边解除。四问逐条有答；无任何取值；未立 ADR 并给了理由。
- **(b)**：票面自述落点三件「另立实现票，不在本票」：接受判断结构化拒绝原因（PS）、服务选项进 `parcel-pricing` 特征、`settlement-accounting` 客户赔付读取规则引用。**HEAD 上 `.scratch` 搜「保价」只命中 06 与 spec，后继实现票尚未立**。
- **(c)**：优先级依赖已解除。
- **(d)**：无冲突；与 VE 追偿/SA 赔付两链的边界已在词条里写明「不混」。
- **(e)**：已 resolved（形态半边），同意；缺一张后继实现票（或三张按上下文拆），否则「落点」只活在一条 resolved 票的 Answer 里——这正是 tf-segment spec 开头描述过的那种丢法。
- **(f)**：MCP-3（12:15 广播第 2 件，已收口）。

#### ftr/07 不自愈的未决烧重投预算

- **(a)**：票面 SHA `32d6a49 f8301e4 74ab82f 371f6cb` 全在 HEAD 祖先链；`c96065b` 不是祖先（票面自述为隔离 worktree 锚，后快进为 `74ab82f`，一致）。逐项：`domain.ResumePath` 四格（`acceptance_task.go`，`valid()` 上界 `ResumeByOperatorRegistration`）；`resumePath()` 第四格映射（`judgment_continuation.go`）；`undecided_disposition.go` 过渡态注释与 `ErrAcceptanceChainUndecided` 回滚在；`migrations/parcel_shipment/0011_resume_path_operator_registration.sql` 在。
- **(b)**：D4/D5 缺口逐符号：`Decide`（`acceptance_decision.go`）只写 `InternalRetry/CustomerSupplement/ManualReview` 三格；`AdvanceAcceptanceJudgmentHandler{commercial, reachability, recorder, clock}` 与 `AdvanceFinancialControlJudgmentHandler{commercial, controller, recorder, clock}` **均无 `ShipmentRequestRepository`**，D5「落此格前先 Save」无落点；`cmd/parcel-dispatch/assemble.go` 路由 15 个事件类型，无「参数已登记」信封；`internal/partycommercial` 非测试代码零 `EventType` 声明，发信封那一半从零起。以上与 MCP-1 `04b69b8` 新追的切法（第一笔 PS 独占 D5：新领域转移+两编排取回聚合 Save+`FindWaitingOnOperatorRegistration`+迁移 0013；第二笔跨 PC/PS D4 与 `undecidedDisposition` 翻转同笔）逐条对得上。
- **(c)**：无外部阻塞；第二笔要占 MCP-2 的 `internal/partycommercial` 地盘。
- **(d)**：无冲突；ADR-0094 Consequences「预算只花在会自愈的依赖上」对第四格暂不成立，票面已如实记。
- **(e)**：`保持`（在途、切法已定）。一句提醒：第一笔的迁移号 `parcel_shipment/0013` 是票面预取，开工时重取（0012 已被 continued_attempt_register 占）。
- **(f)**：MCP-1（12:5x 广播：D5 切片在隔离分支 `mcp1-ftr07-d5` 做，领域一片已提交 `e8ec9bb`；合回时开窗先报）。

#### ftr/08 未决停站与原因不可见

- **(a)**：`20d21f4`/`c25d145` 在祖先链；`internal/platform/dispatch/dispatcher.go` 的 `DeliveryFailureObserver` 类型、`WithDeliveryFailureObserver` Option、`observeFailure` 字段；`cmd/parcel-dispatch/assemble.go` 注入 `logDeliveryFailure(logger)`（`slog`）；`assemble_test.go` 的 `TestAnUndecidedStallSurfacesStageAndReasonToTheObserver` 在。`internal/platform/dispatch` 共享树 18 PASS/6 SKIP（无 DSN）。
- **(b)**：第一层（不自愈那格靠入账留痕）缺口 = 07 第二笔的 `undecidedDisposition` 翻转 + 07 第一笔的等待态落库；本票不另设机制。
- **(c)**：挂 07。
- **(d)**：无。
- **(e)**：`保持`，随 07 第二笔收口后转 resolved。
- **(f)**：MCP-1。

#### ftr/09 「等待受控补充」真的会自愈吗

- **(a)**：needs-info 属实：票内无取证。锚 `ca7441f` 在祖先链。
- **(b)**：三问中两问今天从代码就能答（以下是代码事实，不是裁决）：
  - **问 1**：`FormNewSubmissionVersionDeps{Sources, Requests, Identities, Clock}`（`form_new_submission_version.go`）——**无 outbox/handoff 端口，整个文件零 `Envelope/Publish/HandOff/EventType`**，受控补充不铸任何信封；而且 `NewFormNewSubmissionVersionHandler(` 非测试引用只有声明本身，`cmd/` 零命中——**受控补充编排在生产上根本没有入口**（`.scratch/outbox-handoff-consumption-map/report.md` 早把它列在「连消费面都没有」）。所以「客户新提交版本会自己回来」在 HEAD 上没有任何机制承载。
  - **问 2**：`CommercialBasisSuperseded`（`ports.go`→`form_acceptance_decision.go` 的 `resolveAdoptedAgain`）与 `StaleShipmentRequestRevision`（`judgment_continuation.go`）两个原因存在，但旧信封重跑落到哪一格是运行时行为，**本轮未量**（无 DSN、不跑进程）。
  - **问 3**：生产上接受判断链只经 `cmd/parcel-dispatch/assemble.go` 的消费门跑；`undecidedDisposition` 对 `ResumeByCustomerSupplement` 交回哨兵（`undecided_disposition.go`）→整笔回滚；读面 `acceptance_review_queue.go` 只按 `ResumeByManualReview` 过滤。**`waitingOn = CUSTOMER_SUPPLEMENT` 在结构上从未落库**，与 ADR-0086 给人工复核开例外的读面理由同构——这与票面「第三问单独要紧」一致。
- **(c)**：无阻塞边。
- **(d)**：证据指向票面第三种结论（自愈不成立），若成立要 supersede ADR-0086 Context 那一句——票面已写明走新 ADR。
- **(e)**：`需人裁`，可答选项：A 新 ADR 停用 ADR-0086 Context「等待受控补充回滚重投是对的」一句，把 `ResumeByCustomerSupplement` 并入 ADR-0094 提交侧，与 07 第一笔（D5 的 Save 护栏与队列读口）同形落地；B 维持 ADR-0086，先做 ADR-0045 划出去的「重触发判断」切片（补充编排铸信封+消费门），让前提成真；C 维持现状、只在 ADR-0094 那格补取证锚（等于接受这一格今天照样烧到 ABANDONED）。建议票面 Status 从 needs-info 转 ready-for-human，并把上面问 1/3 的代码事实记进票——问 2 留给有 DSN 的人量一次。
- **(f)**：无人占号；ADR-0094 的注释（`undecided_disposition.go`）已把「若判出它不自愈，改的就是这一行」指到本票。

### B 组没核到的东西与原因

- **真库行为一律未量**：该会话无 `IDP_PARCEL_POSTGRES_DSN`，所有 `go test` 绿都是无 DSN 绿（inbox 21 SKIP、dispatch 6 SKIP 即真库用例）；ftr/09 问 2、ftr/07 票面「真进程上今天的现场」均按代码读，未复现。
- **`internal/architecture` 与 `internal/transportfulfillment/domain` 在共享树上未能验证**（他人在途未跟踪件致 vet/棘轮红），只在干净 worktree 上验了绿。
- **未跑 `-race`**（本机 CGO 状态未查）。
- **tf/02 在途进度未看**：TF 树上此刻的未跟踪件属 label-channel/18–21，不属 02；02 至 HEAD 零代码。
- **mech/03 的十四行**：产出不存在，无从核其内容。
- **ftr/03 的 `PAR-NET-14` 只核了登记册状态行**，未核网络规划侧是否另有进展记录（本仓外）。
- **spec 层面的状态句一致性只核了本组三册**，未扫其它册对本组票的交叉引用是否过时。
- HEAD 在审查期间前进四次，`08e62ec`→`04b69b8`；B 组凡未注 SHA 的代码引用均以 `04b69b8` 工作树为准，其后若有提交请重取。

### A/B 两组交叉可见的三件事（MCP-4 并入时顺带记，不替 owner 裁）

1. **TF 迁移号已经在动**：A 组核 label-channel/18 时「TF 最新 `0011`，票写 `0012` 准」；B 组在 `04b69b8` 工作树上看到 `0012_external_carrier_credential.sql` 已作为 MCP-2 未跟踪件占号。两组一致，推论只有一条：tf/02（同为 TF 迁移）与 label-channel/19（若落表）都要从 `0013` 起按开工那刻重取；ADR-0103 Consequences 里的「`0012` 起」已是过时数字。
2. **「resolved 票的 Answer 里埋着未立的后继票」在两组都出现**：A 组 label-channel/12 的「组合根与调用入口」、B 组 ftr/06 的三处「落点」。同一种丢法，建议 C 组交回后统一开一节「应立而未立」清单交 owner。
3. **票面阻塞边普遍落后于事实**：A 组 label-channel/11（Blocked by 06）、14（Blocked by 01, 12），B 组 tf/09（Blocked by 05）——被等的票都已 resolved，票面一律未更新；ftr/03 则反过来，Status 写 blocked 却没有 `Blocked by:` 行。这类修正全是纯票面改动，归各 owner。

---

## C 组（MCP-6）：pricing-reference-series-operations · party-commercial-context-gaps · needs-info 群 · 其余 in-progress spec

**C 组未交（MCP-6 会话重启，截至 13:07）。** MCP-6 12:07 接单（task `0e396da3`，自报钉 `08e62ec`、只读、预计一次交全表），此后无进度回报；13:01 点名 MCP-6 有应答但任务记录仍停在接单那一条。MCP-1 13:07 令本报告不等，按「未交」收口。**下列票目的 (a)–(f) 审查缺席**，任何人接手时按派单里同一套判据补，补完追在本节：

| 票 | 派单时票面 Status（`08e62ec`） | 派单时的审查要点 |
|---|---|---|
| pricing-reference-series-operations/spec | in-progress | 状态句与子票一致否 |
| pricing/04 review-endpoint-and-admin-structured-form | in-progress | 04a 落 `9035df7`，04b 前端并入 08——04 本身还剩什么 |
| pricing/05 coverage-horizon | in-progress | 05a 落 `1cacc72`+`aa54a4b`，第 3 项 `f4c2996`，05b 按 ADR-0105——余项 |
| pricing/06 source-connector-framework-and-cfets | draft | 两处待定是否仍待 |
| pricing/08 series-write-face | in-progress（MCP-5 票面持有） | 独立评审；MCP-1 13:07 另报 `D:/tops/idp-pr08` 有 09-03 20:18 未提交现场 |
| party-commercial-context-gaps/spec | in-progress | 状态句 |
| pc-gaps/05 customer-service-rule-version consuming seam | ready-for-agent（MCP-2 占） | ADR-0104 与票面「裁决」节是否一致 |
| nr-route-evidence-views/01 | needs-info | 那次取证今天能不能取、取什么、谁能取 |
| ps-external-mark-relations/01 | needs-info | 同上 |
| ve-008-late-account-rederive/04 | needs-info（等 PAR-INT-01） | 同上 |
| syn-wall-door-audit/01 | needs-info（S0 三项证据缺口） | 同上 |
| admin-remainder-mechanism-batch/spec + 05 t2-remeasure | in-progress（MCP-1 在途） | 余项 |
| frontline-transition-import/01 | in-progress | 余两格的阻断是否仍真 |
| unmerged-branch-inventory/spec | in-progress | 盘点已完，该不该关 |
| tenant-implementation-01/spec + implementation-checklist | in-progress | 还有没有代码侧待办 |

A/B 两组末尾那节「交叉可见的三件事」里的第 2 条（应立而未立清单）原计划等 C 组到齐再统一开——现按 A/B 两组已有的两处（label-channel/12 的组合根、ftr/06 的三处落点）交 owner，C 组补齐后若再添，追在同一节。

---

## 末节：无 `Status:` 行与非标准状态词的文件

这些不是票，多数是 report / census / 简报 / 交接档，按其自述性质不需要 `Status:`。列出只为让「未 resolved」的清点不漏项，不建议逐份补状态行。

**无 `Status:` 行（24 份，钉 `08e62ec`）**：`.scratch/{at-coverage-inventory,mcp-1-history,tasks,ticket-families}.md`；`admin-remainder-mechanism-batch/t2-remeasure-{1665fdb,7d68475}.md`；`admin-web-page-wiring-frontier/report.md`；`commercial-closure-settlement-key/acceptance-drive-decision-brief.md`；`dead-session-salvage/branch-merge-census-2026-08-21.md`；`declaration-envelope-version-dedup/report.md`；`domain-executor-audit/census-9d6063c.md`；`no-control-transfer-out/report.md`；`outbox-handoff-consumption-map/{next-consumer-survey,report,uc-reverse-gaps}.md`；`outbox-partition-key/{evidence-four-undecided,ruling-draft-four-undecided}.md`；`production-wiring-ratchet-gate/census-d5e5d20.md`；`syn-wall-door-audit/{report,t1-reverify-pc-nr-cc-ve,t1-reverify-platform-ps-pp}.md`；`synthetic-vertical-closure/design.md`；`ui-templates-collision-20260824/README.md`；`ve-correction-late-chain/report.md`。

其中一份值得单独点名：**`synthetic-vertical-closure/design.md`** 是一张推进表，`auto-reroute-demo-reachability/01` 已指出它有过期行；本报告 A 组复核显示其 `CONS-*` 三行的依赖今天都已落，该表若继续被人当排期用会误导。建议其持有者要么补一行「本表止于某 SHA，此后不维护」，要么转 superseded。

**非标准状态词（13 份）**：`completion-assessment-2026-08-20.md`「取证快照」；`admin-web-uiux-20260824/tracking-scope-decision-brief.md`「取证完成,待裁决」（`ve-operations-tracking-read` 全批已 resolved，裁决应已发生，建议改 superseded/resolved）；`frontline-client-shape/README.md`「提案存档」；`frontline-transition-import/{fact-to-usecase-mapping,template}.md`「取证于…」「供…」；`product-story-and-demo/{journey-draft,read-admission-brief}.md`；`product-version-closure/{design-input,design,open-decisions}.md`（三份均自述「已落地」，可统一为 resolved）；`tenant-implementation-01/{decision-brief-scope-and-frontline,instance-register-draft,requirement-mapping}.md`（归 C 组评）。
