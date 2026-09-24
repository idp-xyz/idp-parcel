# 04 路由第一刀：拆 `PAR-NET-14`，路由策略族与首个内置策略

Category: enhancement
Status: in-progress——跟踪容器：2026-09-24 通道 5 按 /to-tickets 拆为 [`.scratch/routing-first-cut/`](../../routing-first-cut/issues/) 下子票 01–11（全部 `draft`，待通道 1 认可拆法后转 ready-for-agent），切片计划见文末；本票自身不再有可执行工作。此前：in-progress——2026-09-24 通道 5 认领（单 task-d12bb120-aac3-40b4-b1e9-a018b77afddc），分支 `mcp5-psb04` 基 `64b37f27`。此前：ready-for-agent（演示网络参考配置那半 Blocked by 03）
Blocked by: 03（只挡演示网络参考配置那半）
地盘：network-routing 的领域、应用与取数侧适配器；参数登记册 `PAR-NET-14` 一行的拆分结论。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七；票 `nr-route-evidence-views/01`（needs-info，其重启条件由本票替换）。

## 做什么

1. 把 `PAR-NET-14` 拆成产品策略与租户取值，拆分结论写回登记册那一行（与 02 同口径）。
2. 路由策略族：候选生成、硬约束过滤、排序、冻结边界与已执行前缀的判定、并发裁决、自动 / 人工改路的判断结构，落在 network-routing。
3. 首个内置排序策略沿 `PAR-NET-16` 已确认的「满足硬约束后按成本单维择优」；并列照旧交人工。
4. 把版本化网络目录折成逐候选事实的取数侧接上 `NetworkEvidenceView` 与 `InitialRouteEvidenceView`；目录为空或未采用策略时照旧答`未配置`。
5. 演示网络作为参考配置，经 03 的采用路径进入演示租户（这半等 03）。

## 不做

- 不定任何租户的阈值、日历、截单或权限分派；不预选某个租户用哪种排序策略。

## 完成判据

- 演示租户上一票已接受的委托能形成初始路由，不再停在路由证据未配置；`nr-route-evidence-views/01` 与被它挡住的两张 blocked 票各有去处。

## 切片计划（2026-09-24 通道 5，取证钉 `64b37f27`）

**结论：拆。** 「路由策略族」那一步要的判断函数大半已在：区域 → 承诺分界 → 可执行性 → 硬约束 → 时间可行性 → 择优一条管线，自动改路三态也有。缺的是：路由策略版本上没有任何内容载体；首个内置形态要的并列出口与现行收尾相反；冻结边界、已执行前缀与并发裁决没有判断结构；目录里没有可折的内容列。「接路由证据取数侧」那一步里，地理投影、关务资格、候选成本这几族不出自目录，每一族都是一条没做过的跨上下文缝。一张票装不下，也不该在一张实现票里顺手替这几条缝做边界决定。

### 取证（只作此刻取证，不作别人的基准）

- **取数侧**：生产上两个证据视图的唯一实现 `NetworkDefinitions` 只读定义登记册，登记了即上抛 `ErrNetworkDefinitionUnresolvable`。目录（迁移 `0008`）只有结构：服务区域无覆盖内容、服务日历无营业日与截单、路由策略版本无规则正文，头注写明等形态定了再以新迁移扩列。
- **排序**：`SelectRouteCandidate` 全部声明准则打平时按候选标识升序收尾。PS 渠道择优的比较器在同一处交 `ErrChannelCandidateCostTied`、决定记 TIED 等人裁——本票「并列照旧交人工」的「照旧」指的是这里。[`label-channel-service-first-release/01`](../../label-channel-service-first-release/issues/01-channel-candidate-tie-break-authority.md) 当时判路由侧的收尾「仍然正确」，前提是多维准则几乎不会全维打平；成本单维下这个前提不成立（归子票 03）。
- **改路**：`EvaluateAutoRerouteConditions` 收的是已折好的布尔，折法注为实例半边；复核编排里没有冻结判断；「改路与装载或交接并发」只有 CONTEXT 句，没有判断结构，NR 也没有消费装载事实。
- **跨上下文**：NR 的端口与适配器里没有读关务资格的口，也没有取 BUY 评价的口。[ADR-0075](../../../docs/adr/0075-customer-address-is-carried-with-the-routing-request.md) 定了地址随请求携带、投影形状留给取数侧实现票，而证据视图端口今天只收判断键。
- **旧票**：[`first-tenant-runway/03`](../../first-tenant-runway/issues/03-network-resolution-layer.md)（网络解析层，Blocked by `PAR-NET-14`）与「接路由证据取数侧」是同一件事。其 Answer 里「视图修订改由目录修订锚派生」「ADR-0068 决定六随解析层同笔部分停用」两条结论仍成立，由子票 07 承接；[`auto-reroute-demo-reachability/02`](../../auto-reroute-demo-reachability/issues/02-syn-vertical-run-reaches-reroute-after-lapse.md) 在等它。

### 子票

子票放在独立目录 [`.scratch/routing-first-cut/issues/`](../../routing-first-cut/issues/)，从 01 编号：本目录 06 起的编号由 02 的新票先占（通道 4），两族分目录，此后不会撞号。下表与正文里的两位数编号都指那个目录；指本目录的票一律写 `psb/NN`。

| 票 | 做什么 | Blocked by |
|---|---|---|
| [01](../../routing-first-cut/issues/01-par-net-14-split-and-rehome-old-tickets.md) | 登记册 `PAR-NET-14` 写回拆分结论，被它挡住的旧票改去处 | psb/02 登记册那一笔 |
| [02](../../routing-first-cut/issues/02-route-evidence-sourcing-and-candidate-cost-adr.md) | 取数侧逐族定来源、路由候选成本口径、首版候选生成形态——ADR | 无 |
| [03](../../routing-first-cut/issues/03-route-strategy-family-and-cost-single-dimension-ranking.md) | 路由策略版本声明排序形态；首个内置形态成本单维、并列交冲突 | 无 |
| [04](../../routing-first-cut/issues/04-freeze-boundary-and-executed-prefix.md) | 冻结边界与已执行前缀的判定 | 03 |
| [05](../../routing-first-cut/issues/05-auto-reroute-conditions-folded-from-strategy.md) | 自动改路条件由策略与计划事实折出，改善阈值 | 04 |
| [06](../../routing-first-cut/issues/06-reroute-loading-concurrency-arbitration.md) | 改路与装载 / 交接的并发裁决 | 04 |
| [07](../../routing-first-cut/issues/07-reachability-evidence-folded-from-catalog.md) | 可达性证据从目录折出（视图修订、服务区域、候选、可执行性） | 02 |
| [08](../../routing-first-cut/issues/08-shipment-carries-geo-projection-to-routing.md) | PS 随路由判断携带地理解析投影（ADR-0075 的 PS 半边） | 02、07 |
| [09](../../routing-first-cut/issues/09-initial-route-evidence-folded-from-catalog.md) | 初始路由证据从目录折出（时间投影、段链、策略引用） | 07 |
| [10](../../routing-first-cut/issues/10-candidate-cost-from-leg-buy-evaluations.md) | 候选成本：段的 BUY 评价经 PP 合成 | 02、03、09 |
| [11](../../routing-first-cut/issues/11-demo-network-adopted-as-reference-configuration.md) | 演示网络经 psb/03 的采用路径进入演示租户 | psb/03、08、10 |

可以立刻开工的前沿是 02、03，彼此不相碰（一份 ADR / NR 领域与策略载体）；01 等 psb/02 的登记册那一笔落地后即可做。

### 完成判据对照

- **「演示租户上一票已接受的委托能形成初始路由」**：关键路径 02 → 07 → 09 → 10（并 03）→ 11（并 psb/03、08）。**另有一条可能挡在路上的缝不在本票族里**：候选的关务资格。02 若判定 CC 侧今天没有按候选作答的判断方法，那是 ADR-0146 意义上又一处缺执行器，登进 psb/05 的动线取证并另立票；它补上之前，含关务段的候选只能答状态未知 → 路由判断未决，11 的判据到不了。
- **「`nr-route-evidence-views/01` 与被它挡住的两张 blocked 票各有去处」**：01。

### 做法约定

- 碰 Go / SQL 的子票照[并行会话](../../../docs/agents/parallel-sessions.md)走隔离 worktree。迁移号各票开工时在频道预留，这里不预分——落地次序未定，预分会逼着按号序落。
- 证据视图端口的导出签名在子票 07 改，开工前在频道报窗口。
- 子票全部 resolved 后，本票按 [issue-tracker](../../../docs/agents/issue-tracker.md) 约定收口。
