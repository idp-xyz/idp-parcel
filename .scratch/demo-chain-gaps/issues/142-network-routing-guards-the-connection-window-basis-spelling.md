# 142 NR 侧没有用例守住计划段窗口依据的 CONNECTION 拼写

Category: enhancement
Status: in-progress——2026-10-26 复评无阻断，待进 main，分支 `claude-dcg142` tip `4c45d65e2`（复评收尾末笔；本票面一笔随后）。此前 in-progress——2026-10-25 评审阻断已修、修订轮完，待评审复核与进 main，分支 `claude-dcg142` tip `7e78cb155`（修订轮末笔；本票面一笔随后）。此前 in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 `claude-dcg142` tip `ff13558ed`（代码末笔；本票与 143 的完成记录随后一笔）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，与 143 同批，分支 `claude-dcg142`（worktree `idp-parcel-dcg142`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R4；ADR-0201 决定二

## 现象与修法

现象、证据与修法的完整写法以复核报告 R4 一条为准（单一权威），此处只记要点：

代决「是」：两条认作 NR 对 TF 的契约；NR 侧加一条用例守 `CONNECTION/<代码>@<版本>` 恰一格与「同代码跨版本只改起讫」。

## 完成记录（2026-10-25，Claude Code 作者子代理，协调方 `tops-20`；分支 `claude-dcg142`，worktree `idp-parcel-dcg142`，起点 `origin/main` `f8b25837b`）

授权：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」。裁定的单一权威是复核报告 R4，本节只记落地与落地时的代决。

### 提交

| SHA | 内容 |
|---|---|
| `dfc94ecb5` | test(network-routing)：守计划段窗口依据 CONNECTION 拼写（本票全部代码改动，只加测试） |
| 随后一笔 | docs(scratch)：142、143 完成记录 |

同分支另有 143 的 `9c2e171df`（ADR-0301）与 `ff13558ed`（代码），与本票无交集。

### 找到了什么（取证于 `f8b25837b`）

- **NR 产出点**：目录证据视图 `CatalogInitialRouteEvidence` 折候选路径时，`projectCandidateTime` 经 `plannedLeg` 给每段节点间段拼窗口依据 `"CONNECTION/" + versionReference(代码, 版本)`；交付段由 `deliveryLeg` 拼 `NODE/…`。初始路由与复核改路都原样取被选候选的 `PlanLegs()` 成计划段链，计划段的窗口依据只在这里拼；计划落库按原串存取。
- 候选级的投影窗口另拼一串（各节点日历的 `NODE/…` 与衔接缓冲的 `CONNECTION/…`，排序去重后以 `+` 连），TF 不读它。
- **TF 解析点**：`internal/transportfulfillment/adapters/networkrouting` 的 `connectionOf`——按 `+` 切格、认 `CONNECTION/` 前缀、要求恰一格、在最后一个 `@` 处切掉版本。

### 做了什么

- 新用例文件 `internal/networkrouting/application/connection_window_basis_contract_test.go`，经真目录证据视图加目录读口替身折出候选路径，按 TF 的读法在 NR 产出上读一遍：
  - `TestEachLegBetweenNodesNamesExactlyOneVersionedConnection`
  - `TestANewVersionOfAConnectionKeepsItsCodeAndMovesOnlyTheVersionAndEnds`
- 文件头注释写明这是 NR 对 TF 的契约，出处 ADR-0201 决定二、复核记录 R4、越权风险点 6 与 25，以及 TF 那一侧的读法符号 `connectionOf`。
- 生产代码零改动。

### 代决

1. **第二条只守了落在 NR 产出上的那一半。**「同一连接代码跨版本只改起讫」在产出上可测的是：同一代码换版后，计划段那一格代码一字不变、只有版本号跟着走，段起讫取当版起讫——用例二守这一半。「含义变就换代码」守不了：目录连接行只有代码、版本、起讫、业务时区、生效区间与登记依据，除起讫外没有承载含义的格；登记口拿新旧两版比也判不出含义变没变——时区、区间与依据各版本来就可以不同。这一半是登记方（租户或参考配置）的纪律，没加守卫。
2. **读法在 NR 测试里按 TF `connectionOf` 的规则另写一遍，不导入 TF 包**：NR 的测试依赖 TF 会倒转上下文方向。代价见越权风险点 2。
3. **用例放在应用层、走真证据视图加目录读口替身，不走真库**：窗口依据只在证据视图里拼，落库按原串存取，真库再走一遍不多证什么。
4. 用例里一条连接代码故意带 `@` 与 `/`（`SYN@CONN/B-C`），证 TF 按最后一个 `@` 切版本、按前缀认格时这种代码也读回原样。

### 越权风险点

1. **连接代码里带 `+` 会破第一条**（NR owner；TF owner 知会）。登记口（`RegisterConnectionVersion`）与库上 CHECK 都只要求代码非空白。代码含 `+` 时，NR 拼出的 `CONNECTION/A+B@1` 在 TF 那里被切成两格、读不出带版本的连接，准备那一路在每封初始路由信封上响亮失败，即 ADR-0201 越权风险点 25 那一种。本票没加守卫：加一格登记拒因是登记答案集的改动（HTTP、CLI 与 admin-web 的拒因标签都读它），不属「加一条用例」。建议另立票：连接代码含窗口依据分隔符 `+` 时登记口拒。
2. **TF 读法在 NR 测试里另写一份**（NR、TF owner）。TF 若改了 `connectionOf`（例如换分隔符），这条用例不会红；它守的是 NR 不改拼写，不守两侧读法同步。
3. **「含义变就换代码」只靠登记方纪律**（NR owner），见代决 1。

### 完成判据 → 用例

| 判据（R4） | 用例 |
|---|---|
| 计划段窗口依据里恰有一格 `CONNECTION/<代码>@<版本>` | `TestEachLegBetweenNodesNamesExactlyOneVersionedConnection`：两段节点间段各恰一格，拼写逐字等于当版连接；按 TF 读法读回原代码与版本，含带 `@`、`/` 的代码；段起讫为当版连接的起讫；衔接缓冲不给计划段添格；交付段零格（ADR-0217） |
| 同一连接代码跨版本只改起讫、含义变就换代码 | `TestANewVersionOfAConnectionKeepsItsCodeAndMovesOnlyTheVersionAndEnds`：两段各换一版、其中一段改终点，代码不变、版本跟着走、终点取当版；「含义变就换代码」守不了，见代决 1 |

变异检查：把 `plannedLeg` 的依据改成 `NODE/x@1+CONNECTION/<代码>@<版本>+CONNECTION/y@1`，用例一红（「指了 2 格连接」）；改回即过。

### 自验（实测于 `ff13558ed`，即本票与 143 代码合在一起的分支 tip）

- `go build ./...`、`go vet ./...` 全仓过；改动的 `.go` 都已 `gofmt -w`，`gofmt -l` 无输出。
- 带 DSN（`127.0.0.1:55432`，含 PG）`go test -count=1 -p 4`：`./internal/networkrouting/...`、`./internal/parcelshipment/...`、`./cmd/parcel-api/`、`./cmd/parcel-commercial/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-frontline-import/`、`./cmd/parcel-network-register/`、`./cmd/parcel-settlement-register/`、`./internal/architecture/...`、`./migrations/...`，40 个包全 ok、退 0。真库包确实跑了：NR `adapters/postgres` 7.5 秒，PS `adapters/postgres` 15.5 秒，`cmd/parcel-dispatch` 33.5 秒。
- 全量留给推送方。

## 评审记录

以下照抄协调方转来的 `review-142-143.md`（二级标题降为三级），142、143 两票同一份。

评审 ← tops-20 隔离子代理 · 钉 `6fc41e9ad` · 23:48

结论：有阻断 1 条（Spec 轴，143 新读口）；142 无发现。

### Standards
阻断：无。
非阻断：
1. PS CONTEXT「地址要素」末句：不改也不与 ADR-0301 相抵（按租户+包裹、取基线那一版、同一份投影仍成立），但 AGENTS「改文档」要求「同一笔把 CONTEXT 里被停的句子改成现行规则」；ADR-0301 Consequences 转述成「适配器…取…」，漏了 PS CONTEXT 原有的「反查当前已接受委托」。若改，在本分支落，并同改 Consequences 里「CONTEXT…不改」那句（合入后 ADR 正文冻结）。
2. ADR-0279 候选「在 PS 委托仓储或 PS 读口上加一法取接受时刻」的否决前提「NR 已有的 PS 读口…就交得出它」已被 ADR-0301 换掉；ADR-0301 正文交代了但不是逐字引。可照 ADR-0008 先例在 ADR-0279 注记挂「理由限定」；不挂不算相抵。
无发现：红线（NR 只经 `psports.AcceptanceBaselineGeoProjectionView` 读，SQL 在 PS adapters/postgres，architecture 探针绿）；ADR-0215 正文未动、三句引文逐字在决定二；去向/来向分写合 adr/README；adr-index -check 只报生成物；注释中文无行号。

### Spec
阻断：
1. `ShipmentRequests.LoadAcceptanceBaselineGeoProjection` 把重建门会拒的委托当好答案交出。下面三形 `FindBySourceIdentity` 都拒：
   - 基线挂在历史版本：found=true，交出历史版的投影。
   - 基线时刻 ≠ 决定时刻：found=true，`AcceptedAt` 是错值，ADR-0279 的业务时间下限随之错。
   - 成员收窄：被剔的成员答 found=false。
   旧路先撞重建门，落 `ErrAcceptedParcelUnreadable` 重投至放弃，看得见；新路静默作答。PS CONTEXT「已提交 → 已接受」把基线固定在当前提交版本，第一形自相矛盾，UC-NR-003 `AT-NR-067` 要求响亮失败；`TestTheBaselineVersionIsReadEvenWhenItIsNotTheCurrentVersion` 反把这一形钉成正确答案；ADR-0301 越权风险点 1「本口守它读的两段上」的自相矛盾与事实不符。
   修法：基线版本 ≠ `currentVersion.versionId` 时答 `ErrAcceptanceBaselineInconsistent`，翻转那个用例；时刻、成员两条至少写进越权风险点 1。
非阻断：无基线、无时刻、版本不在三格，端到端由「重投」变为「响亮失败」，方向合 `AT-NR-067`；但票面代决 4「各格落法与之前相同」只在适配器层成立，宜注一句。
无发现：判断顺序与旧适配器逐格相同；键名漂移不会静默答错；分格与两路哨兵名单不变；ADR-0301 如实写明初始路由一路仍读整份聚合；142 两条变异（多格连接、`@`→`:`）都让用例变红，代决 1 只守一半不与 R4 相抵；无票面外改动。

修订轮重跑探针：在检出根目录带 DSN 跑
`go test -count=1 -v -overlay <o.json> -run TestReviewProbeGateVersusView ./internal/parcelshipment/adapters/postgres/`
探针源：/tmp/claude-1000/-home-tops/916d5bf9-1a44-4ef9-92dc-171db91f1686/scratchpad/probe143/probe_gate_vs_view_test.go；overlay：同目录 o.json（Replace 键写死评审检出路径，换树要改）。

## 修订轮（2026-10-25，同一作者子代理，同分支）

本票评审无发现，无改动。分支上的修订轮各笔（`1a552ad1c`、`7e78cb155`）都属 143，见那张票的「修订轮」。

### 重跑

实测于修订后的 tip `7e78cb155`（含 PG，DSN `127.0.0.1:55432`）：

- **评审探针**：探针源原样不动，另起 overlay `o-dcg142.json`，Replace 键换成本树路径，`-overlay … -run TestReviewProbeGateVersusView ./internal/parcelshipment/adapters/postgres/`，ok。三形 × 两成员共六行，每行都是：`FindBySourceIdentity` 拒（`invalid rehydrated shipment request`），读口 `found=false`，错误包 `ErrAcceptanceBaselineInconsistent`，原因依次为
  - 基线在历史版本：「baseline is fixed on version "version-1", not on the current submission version」；
  - 成员收窄：「baseline members differ from the current submission version」；
  - 时刻不一：「baseline fixed time differs from the acceptance decision time」。
  
  没有一行交出投影；修订前三形分别答 found=true、found=true、parcel-2 found=false。
- `go build ./...` 与 `go vet ./...` 全仓通过；改动的 `.go` 都已 `gofmt -w`，`gofmt -l` 无输出。
- 带 DSN `go test -count=1 -p 4 ./internal/parcelshipment/... ./internal/networkrouting/... ./cmd/parcel-dispatch/... ./internal/architecture/...`：34 个包全 ok，退 0。真库包确实跑了：PS `adapters/postgres` 16.5 秒，NR `adapters/postgres` 7.9 秒，`cmd/parcel-dispatch` 39.4 秒，`architecture` 8.4 秒。
- 全量留给推送方。

## 复评

以下照抄协调方转来的 `rereview-142-143.md`，两票同一份。

复评 ← tops-20 隔离子代理 · 钉 `f4902c73b` · 00:01

结论：上轮阻断已清零，修订后无新阻断。

探针（重跑于 `f4902c73b`，overlay `probe143/o-142b.json`）：三形里 `FindBySourceIdentity` 都被重建门拒；读口对两个成员都答 found=false、err 包 `ErrAcceptanceBaselineInconsistent`、投影零值——基线挂历史版本「not on the current submission version」；成员收窄「members differ」；时刻不等「fixed time differs from the acceptance decision time」。另：相关包 go vet 无输出；读口真库用例全绿（`TestAnInconsistentAcceptanceBaselineIsNamed` 六子格）；`internal/architecture`、NR 适配器、PS 领域绿；adr-index -check 只报生成物。

Spec：阻断无（判断顺序与 `acceptedProductsValidForRehydration` 基线各条一致；先判后问；SQL 只多读 `currentVersion.versionId`、`declaredParcelIds`、`elements` 与 `decision.decidedAt`，`TestTheProjectionDoesNotReadTheRestOfTheAggregate` 作证；用例翻转到位）。非阻断无新增（上轮那条已在代决 4 补注）。无发现：NR 适配器分格与两路哨兵名单不变。

Standards（只看新增改动）：阻断无。非阻断 1：PS CONTEXT「地址要素」末句括注「（不指当前提交版本，成员或时刻与当前提交版本、接受决定对不上）」只列了 ADR-0301 决定一五格里的三格，缺「没有基线」「基线没有固定时刻」；CONTEXT 是现行规则唯一出处，括注易读成穷举——补上两格或改举例口吻。无发现：ADR-0279「理由限定」注记合 ADR-0008 先例、引文逐字、正文未动；ADR-0301 修订去向写法、决定一二与风险点 1、2 随实现改；PS CONTEXT 末句已改成现行规则；注释中文无行号计数。

本票复评无发现、无改动。
