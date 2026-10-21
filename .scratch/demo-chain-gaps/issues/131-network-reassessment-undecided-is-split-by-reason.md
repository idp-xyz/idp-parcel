# 131 NR 复核未决不分原因一律重投，不会自愈的几格堵住交接分区直到 ABANDONED

Category: bug
Status: resolved——2026-10-21 进 main（全量钉 `77ea36fa`：`scripts/test-full.sh` 158 ok / 0 FAIL，见文末「进 main 记录」）；未判三格入账之后的续办转票 [154](./154-network-reassessment-not-judged-triggers-resume-after-registration.md)，初始路由同病转票 [155](./155-initial-route-unconfigured-undecided-burns-the-acceptance-partition.md)。此前 in-progress——隔离评审无阻断项、评审修订完，待进 main，分支 claude-dcg131（2026-10-21，见文末「完成记录」「评审记录」）。此前完工待评审；此前 2026-10-21 认领（131、132 同批）；此前 ready-for-agent——2026-10-20 代决复核立（上线前必修），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M10；ADR-0215 越权风险点 9、10

## 现象与修法

现象、证据与修法的完整写法以复核报告 M10 一条为准（单一权威），此处只记要点：

按 `UndecidedReason` 分格：没有路由历史、证据未配置、冻结形态未配置三格入账并留原因，不登哨兵；读不回、看不清、取号依赖故障照旧重投。日后可并入票 14 的同族续办。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 `claude-dcg131`，与 132 同批）

### 接票时核实

M10 的现象逐条成立：两路适配器的 `reassessmentToConsumption` 把复核未决一律折成本路哨兵；三格未配置重投到派发预算耗尽后落 ABANDONED（派发真库用例改前核对时实测到 `ABANDONED` + `dispatch.consumer_undecided`）。另核得两处票面没写的：

- 收寄采用一路同病：它停在小包托运的分区上，同一封信上排在后面的计费那一路被拖住。
- 光把折法改成入账还不够：复核编排在读路由历史之前就把触发指纹落了 `route_handoff_log`，入账会把指纹一起提交。原先「指纹在册、复核不在」答登记册读不回，同一触发换一封信封再来就会卡回重投、照样烧到 ABANDONED。

### 代决（用户授权「要我裁定的部分，你作为业务和系统专家，请你自决吧」，代 NR owner）

权威落在 [ADR-0235](../../../docs/adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定一至三、六。这里只记要点：

1. **判据在复核编排一处，两路同改**：`ReassessUndecidedReason` 只留重投推得动的格；没有路由历史、证据未配置、冻结形态未配置改答`未判`（`ReassessTriggerNotJudged`），原因归 `ports.NotJudgedReason`。两路适配器只按结论折，不持原因字面量。M10 字面只点名交接适配器，收寄采用一路同改的理由见 ADR 候选第一条。
2. **原因落库，不只落日志**：新表 `network_routing.route_reassessment_not_judged`（迁移 0021），一行记关联、判断键各维、原因、触发业务时间与入账时刻。原因没落库不得答`未判`，写不进改答登记册那一格的未决、回滚重投。M10 写「日志与 inbox 账」，落成登记的理由见 ADR 决定二。
3. **携带同一触发的另一封信封再来时重新判**：指纹在册、复核不在时往下重新判，不再答登记册读不回；未判登记只留先到那一行。同一封信封入账后由消费门按 inbox 跳过，到不了这里，所以这不是续办手段。
4. **续办不在本票**：另立 [154](./154-network-reassessment-not-judged-triggers-resume-after-registration.md)，Blocked by 票 14（同族续办形状）。初始路由一路同病，另立 [155](./155-initial-route-unconfigured-undecided-burns-the-acceptance-partition.md)。
5. **ADR-0215 部分停用**：决定四「重投」一条里「复核停在未决（复核登记册或路由历史读不回、证据未配置、适用性看不清、取号依赖故障）」一句的未决范围，适用场景限 NR 路由复核两路。只在 ADR-0215 的 Status 行与 Links 加前向指针，正文不改。

### 越权风险点（全文见 ADR-0235「越权风险点」，本票相关的几条）

1. 三格入账之后没有续办触发：配置补齐或初始路由形成之后，这几次复核不自动重判。今天没有任何续办手段（同一封信封入账后由消费门跳过，生产上也没有工具发出携带同一触发的另一封），全靠票 154。owner 若坚持 ADR-0094 决定四「续办触发同笔落地」，撤回的是`未判`一格，代价回到 ADR-0215 越权风险点 10。
2. 「没有路由历史」可能自愈：初始路由停在依赖类未决、事后才形成时，早到的触发原本重投到它形成就能判。
3. 原因落 NR 登记，不落进程日志；owner 要日志行另加即可。
4. 未判登记只留第一次的原因。
5. 未判登记没有读面，今天只能在库里查（随票 154 定）。

### 做了什么

- `internal/networkrouting/ports/ports.go`：`NotJudgedReason`、`NotJudgedTrigger`；`ReassessmentStore.RecordNotJudged`。
- `internal/networkrouting/application/reassess_route.go`：`ReassessTriggerNotJudged`、`ReassessRouteResult.NotJudgedReason`；`ReassessUndecidedReason` 去掉三格；`notJudged` 先落原因再答；指纹在册、复核不在时重新判。
- `internal/networkrouting/adapters/postgres/route_reassessment.go`：`RecordNotJudged`（撞键不写、要事务）。
- 两路适配器 `adapters/transportfulfillment/reassess_on_handover.go`、`adapters/parcelshipment/reassess_on_network_intake.go`：`未判`入账；哨兵注释改为「只在重投推得动时交出」。
- `cmd/parcel-dispatch/assemble_reassess_on_handover.go` 只改注释；哨兵名单、路由文件、接线棘轮基线都不变。
- 迁移 network_routing 0021。
- 文档：ADR-0235（两票共用）；ADR-0215 Status 行与 Links 加前向指针；UC-NR-003 结果表补`触发未判`、一致性节补一条、`AT-NR-067` 收窄到依赖类未决、新立 `AT-NR-074`–`078`；AT 盘点加五行与追记（计数未动）；立票 154、155。

### 用例

- 编排 `reassess_route_test.go`：三格各答`未判`且原因落登记、不落复核、不动适用性（AT-NR-074/075/076）；证据读不动仍答未决、不落未判（AT-NR-067）；另一封信封带来同一触发时重新判、前提补齐后判成、原因落不了库答未决（AT-NR-078）。
- 交接适配器：三格入账、零复核、未判一行，业务时间取交接；证据读不动仍回滚重投、不落未判。收寄适配器：没有路由历史入账留原因；证据读不动仍回滚重投。
- 未判登记真库 `route_reassessment_not_judged_test.go`：一行往返、撞键保留先到那一行、原因在库里封闭、无事务拒、回滚无痕。
- 派发真库 `cmd/parcel-dispatch/reassess_not_judged_test.go`：
  - 三格经生产同形的复核路入账、处理完毕、未判一行，另一封信封带来同一触发仍一行（AT-NR-074–076、078）。
  - 前提补齐后另一封信封判成一条仍适用（AT-NR-078）。
  - 生产路由表上没有路由历史的已接受包裹先后两次交接，两封信封都定稿 `PUBLISHED`，各落一行未判（AT-NR-077）。
- `reassess_on_handover_test.go` 的 AT-NR-067 用例改用依赖故障（证据读不动），回滚重投、不落未判。
- 改前核对：把交接适配器的`未判`临时折回本路哨兵，AT-NR-077 那条红，信封落 `ABANDONED` + `dispatch.consumer_undecided`；改回后绿。

### 验证

于 `7e7dc058`（含 132）：`go build ./...`、`go vet ./...`、`gofmt -l cmd internal migrations tools` 干净；两笔代码提交 `5fb721e5`、`dc428623` 各自单独 `go build ./... && go vet ./...` 通过。`scripts/test-full.sh` 跑了改动包及其反向依赖 44 个包（含 `./cmd/...` 下全部反向依赖的 cmd 包），加 `./internal/architecture/...`、`./migrations/...`，全绿。未跑全量；未重生成机制清点、ADR 索引与 AT 计数（`go run ./tools/adr-index -check` 如预期答 README 未重生成）。

## 评审记录（2026-10-21，协调方隔离评审：无阻断项；作者子代理同分支修订）

逐条去向（与票 132 同一份评审，两票共用的条目两边都记）：

1. **「同一信封再投会重判」不成立**——已修。消费门按（消费者名 + 来源 + 事件 ID）跳过重复投递，未判入账后同一封再投到不了复核编排，只有携带同一触发的另一封信封才会重判；生产上今天没有工具发出这样一封。ADR-0235 决定三改写、删去「手工续办的口子」，越权风险点 1 写明今天没有续办手段、全靠票 154；票 154「现象」同改；UC-NR-003 一致性节与 AT-NR-078 用词改为「另一封信封」；复核编排、端口、postgres 适配器与迁移 0021 的注释同改（`cf0ca77e`、`a79164fa`、`d0e888b8`）。
2. **ADR-0215 决定三被收窄却写「不受影响」**——已修，主要落在票 132，见那边的评审记录。
3. **派发真库用例里的变更叙述**——已修：`TestAHandoverWithoutRoutingHistoryNoLongerHoldsItsPartitionThroughTheProductionRoute` 的拍数注释改为「这一路若答未决，预算耗尽之前同一分区的第二封认领不到，第一封落 ABANDONED」（`cf0ca77e`）。
4. **`ReassessUndecidedReason` 注释枚举 ports.NotJudgedReason 的成员**——已修，改为「重投推不动的归 ports.NotJudgedReason」（`cf0ca77e`）。
5. **`freezeOf` 把三种答不出并成「冻结形态未配置」**——另分一格。只有真的未声明才答未配置、落 `FREEZE_FORM_UNCONFIGURED`。另两种都是编程错误，响亮报错，不落未判：前缀立不起来今天不可达；冻结事实不成立（形态在封闭族外、段数为负）被策略版本册的 CHECK 拒写，只能来自坏了的证据视图。理由同 `planNodesOf`：折成未配置会把装配缺陷伪装成租户配置缺口，在未判登记里留一条点名租户没登记的假原因。补编排用例 `TestInvalidFreezeFactsAreLoudRatherThanUnconfigured`；ADR-0235 决定一补一条，越权风险点加第 12 条「装配缺陷响亮失败，照样占着分区」（`cf0ca77e`、`a79164fa`、`3ff125ca`）。

**未采纳**：无。

**修订后验证**：于 `d0e888b8`，`go build ./...`、`go vet ./...`、`gofmt -l cmd internal migrations tools` 干净；修订那笔代码提交 `cf0ca77e` 单独 build、vet 通过。`scripts/test-full.sh` 跑了 `./internal/networkrouting/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./cmd/parcel-network-register/`，PS、TF、VE 各自的 `adapters/networkrouting`，`./internal/platform/inboxconsume/`、`./internal/architecture/...`、`./migrations/...`，22 个包全绿，0 FAIL。之后的提交只动 ADR 与票面。

## 进 main 记录

2026-10-21 协调方合入，131、132 同批。分支基点 `cb087e5a`，其后 main 多了票 50（PC 版本收尾），与本分支只在 `.scratch/at-coverage-inventory.md` 重叠；cherry-pick 14 笔按原序到 `c1828dd1` 无冲突，其余各文件与分支逐字相同；SHA 随之换新（分支 `3c2ffc58`…`6980e1aa` 作封存出处，main 上为 `c49803e7`…`f09cc9de`）。其上按 parallel-sessions.md 重生成 ADR 索引、重算 AT 盘点计数、重生成机制清点（`77ea36fa`），各自单独成笔；`adr-index -check` 与 `at-coverage-counts -check` 退 0。在干净的隔离树上对 `77ea36fa` 跑 `go build ./...`、`go vet ./...`、`gofmt -l` 干净，`scripts/test-full.sh` 全量 158 ok、0 FAIL（`-count=1`，无缓存）；推 `77ea36fa:main`。迁移落号 `network_routing/0021`、`0022`（连号）。
