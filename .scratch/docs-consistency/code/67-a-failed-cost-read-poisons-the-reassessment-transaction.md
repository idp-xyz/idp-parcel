# 67 NR 复核里候选成本读失败会让收件箱事务中毒：失效与「仍适用」一并回滚，与「候选评估未决不拦失效落库」相抵

Category: bug
Status: ready-for-agent——建议 R2（上线后尽快）：失败是响亮的（整封回滚重投，最终 `ABANDONED`），错在把一次可降级的读失败扩成整笔失败，并误报成写库故障。2026-10-21 协调方裁定立（票 [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md) 隔离评审建议 2；用户「你作为业务和系统专家自决……解决所有的问题」授权），115 作者子代理执笔，分支 claude-dcg115。
Blocked by: 无
出处：demo-chain-gaps/115 隔离评审建议 2。

## 现象

复核编排在三条线上对候选成本「读不回」各有处置：无路由线答未决、回滚重投；失效线记候选评估未决、失效照落；仍适用线不试改善、保留原计划。后两格只在读失败不碰事务时成立。

复核跑在收件箱消费门的那一笔事务里，取数侧经 parcel-pricing 与 NR 目录的仓储读库，读口在事务里时走同一个事务（`bentopg.DB.ReadExecutor` 有事务绑定即返回事务执行器）。PostgreSQL 上一条语句失败，整笔事务即进入中止态，此后的语句都被拒。于是：

- 失效线：适用性已写、成本读失败，随后落复核册被拒，编排答 `REASSESS_STORE_UNAVAILABLE` 未决，整封回滚重投；失效没有落库，与 CONTEXT「失效即失效：候选评估未决拦不住失效落库」相抵。
- 仍适用线：同样在落复核册时失败，原计划仍适用这个已判的结论也一并丢掉。
- 两格的未决原因都报成写库故障，运维会去查复核册而不是 parcel-pricing。

只有不碰事务的读不回（取数侧的翻译类错误，如认不出的评价形状）才真落进「候选评估未决、入账」与「保留原计划」。

## 证据（读码，钉 `378b9b2dc`，证据 `S`）

- `internal/networkrouting/application/reassess_route.go`：`reviewCandidates`、`tryImprovementReroute` 遇 `candidateCosts` 答读不回时分别记候选评估未决、交回原记录，随后 `commit` 落册；`commit` 遇写库错误答 `ReassessStoreUnavailable`。
- `cmd/parcel-dispatch/assemble.go` 的 `routeCosts`：目录、价卡、内部政策、包裹事实各口都是 `bentopg.DB` 上的仓储。
- `go.idp.xyz/idp-bento-go/postgres` 的 `DB.ReadExecutor`：上下文带事务绑定时返回事务执行器。

## 三类

机制。

## 规模

S–M：

- 在复核编排里给取成本包一层保存点（savepoint），读失败时回到保存点，事务照常可用。保存点要由平台的事务口提供，不在 NR 里直接调 `pgx`。
- 平台若还没有保存点口，先在 `internal/platform` 或 bento 侧加；领域与应用层不依赖 `pgx`。
- 改完之后，读失败照原先写定的分格落：失效线候选评估未决、失效照落；仍适用线保留原计划。

## 最小的诚实修法

1. 事务口加「在保存点里执行」的能力，失败即回到保存点、把错误交回。
2. 复核编排的 `candidateCosts` 经这一口调取数侧。
3. 真库用例：失效线上让取数侧的一次库读失败（例如在用例独占的库上撤掉一张价卡表的读权限），失效照落、候选评估未决、信封入账。

## 相关

- 初始路由取成本也在同一笔事务里读库。那里读不回本来就答未决；一份交接带几件包裹时，一件读失败会不会让同一笔里别的包裹的落库一并失败，本票没核，修时一并看。
- 同族的确定性错误落进重投桶，见 demo-chain-gaps/155 的 2026-10-21 备注。
