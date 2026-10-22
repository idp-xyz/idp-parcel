# 84 VE 跟随 CC 外部结果更正关系

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code84（见文末「完成记录」）。此前 ready-for-agent——R2（上线后尽快）：CC 已形成层内更正关系（[code/04](./04-customs-external-results-have-no-in-layer-correction-relation.md)），当前判断按更正取；VE 运营投影仍把生效的更正当作一条独立条目派生，被更正的原条目照旧当前有效。客户视图今天不列里程碑明细，演示映射下改前改后客户面输出相同，所以不列 R1；接真实关务程序、或客户视图开始按条目披露之前升 R1。2026-10-21 协调方据 code/04 隔离评审立（作者子代理代笔；用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: [code/04](./04-customs-external-results-have-no-in-layer-correction-relation.md)（进 main 之后开工）
出处：code/04 完成记录的代决「VE 不跟随更正关系」与越权风险点「CC 当前判断与 VE 运营投影分叉」；code/04 隔离评审 N1。

## 现象

CC 收到来源明确指名所更正那份的同层结果时，记下更正关系（`ExternalResultRecord.CorrectionInForce`、`Result.Corrects()`），放行读口按时点取：更正发生之后以更正后的结果为准。VE 的外部结果适配器按 [ADR-0197](../../../docs/adr/0197-customs-external-results-enter-tracking-projection-per-layer.md) 决定三的取法，事实引用是 `external-result/<成员>/<来源标识>`、版本取提交版本、不登前身。更正有它自己的来源标识，于是落在另一个事实引用下；VE 的替代关系只在同一事实引用的版本之间成立（VE CONTEXT「来源事实替代关系」），登不出前身。

结果：放行收窄更正之后，CC 放行读口只交出部分放行，VE 运营投影里原全部放行那条仍当前有效、仍按映射归入 `CUSTOMS_CLEARED`，另多一条部分放行条目；核定税费、处置决定的更正同样两条并存。新出的是 CC 当前判断与 VE 运营投影分叉。

## 证据

- 现状钉：`internal/visibilityexception/adapters/customscompliance/derive_on_external_result_correction_test.go::TestAnInForceCorrectionIsDerivedAsAnIndependentEntryWithoutAPredecessor`——生效更正作为独立条目派生、不带 Supersedes，原条目仍归入清关。本票落地时翻这一条。
- ADR-0197 决定三：「事实版本取结果关联的提交版本；接收记录只建立不改写，来源更正在 CC 是另一次接收、另一个来源标识，所以不登前身。」
- ADR-0226 决定三：「VE 一侧不必改。投影把每一份未记冲突的接收各派生成本层条目（ADR-0197 决定三）」。
- VE CONTEXT：「源上下文随更正给出替代关系 → 登记该关系，被指名的前身条目转为已被替代并继续保留」。

## 三类

机制：替代关系怎样从 CC 更正关系译成 VE 的事实引用与版本。产品策略：无。租户取值：无（里程碑映射照旧是租户取值，不随本票变）。

## 规模

M：一篇新 ADR 加适配器改动。新 ADR 部分停用 ADR-0197 决定三中事实引用、事实版本与前身的取法，视裁法而定；「要裁的」第 4 件按推荐裁时还触及 ADR-0226 决定三「投影把每一份未记冲突的接收各派生成本层条目」。代码只改 VE 消费 CC 外部结果的适配器与它的用例，CC 一侧已给出需要的全部字段。

## 做法

1. 起草新 ADR，裁下「要裁的」各件，并照 [adr/README](../../../docs/adr/README.md) 顶部写法在被部分停用的 ADR 上挂前向指针。ADR 号向编号分配方取，不自取。起草与接受由协调方代决（用户已授权自决），越权风险点照常列出。
2. ADR 接受后改适配器，把 `TestAnInForceCorrectionIsDerivedAsAnIndependentEntryWithoutAPredecessor` 翻成新口径，并补真库派发用例。

## 要裁的（新 ADR）

1. **事实引用落链根**：生效的更正改落在它所更正那条链的链根的事实引用下（`external-result/<成员>/<链根来源标识>`），还是另立一种引用并在 VE 侧放开跨引用替代（后者要改 VE CONTEXT，推荐前者）。
2. **链根怎么取**：适配器沿 `Corrects()` 逐份回读到第一份不带生效更正的记录；还是由 CC 在记录上另存链根，省掉回读。回读的终止与坏链（读不回、成环）归不变量已破还是续办。
3. **事实版本**：链根照旧取提交版本（存量不动）；链上各份怎样取一个在同一引用下唯一、稳定的版本（例如提交版本加来源标识），前身指名哪个版本。
4. **哪些行登替代**：只有 `CorrectionInForce` 的更正登前身；范围不等（交集为空）、记了冲突、指向已被更正过的那份（链尾冲突）这几种更正各怎样派生——照独立条目、不派生，还是另立冲突关系。
5. **时点**：替代在 VE 上从更正的业务发生时间起生效，还是派生即替代；被替代条目在更正之前的投影版本里怎样读回（与 CC 放行读口按时点取对齐）。
6. **客户视图换代**：原条目转为已被替代时，客户全程追踪视图是否形成新版本、标明更正关系（VE CONTEXT「来源更正……必须重新派生客户视图并形成追加更正或明确替代关系」），已发布的清关怎样以追加更正表达。
7. **存量**：本票之前已按独立条目派生的更正怎样处理——重投重派生、写迁移回填前身，还是首发前随演示库重灌（ADR-0226 决定五同一取舍）。
8. **过渡期**：本票落地前接收的更正，投影上原条目与更正并存；是否要一个运营可见的提示，还是只在完成记录里记风险。
9. **与冲突关系的合并**：票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md) 要给 VE 立「关务进展待确认」信号；分叉的更正（链尾冲突那份）与同层冲突是否走同一条冲突关系与信号。

## 完成记录

2026-10-21，分支 `claude-code84`（基于 main `673b827a1`），用户授权「你作为业务和系统专家自决……解决所有的问题」，协调方代决、作者子代理成文。

### 做了什么

- 新 ADR [ADR-0281](../../../docs/adr/0281-an-in-force-customs-correction-lands-on-its-chain-root-reference-and-supersedes-the-corrected-version.md)（Accepted，代 VE owner 裁定、代 CC owner 会签），裁下「要裁的」九件，各件对应其决定一至九。部分停用（收窄）ADR-0197 决定三引用与版本两句，部分停用（按口解、收窄）ADR-0226 决定三两句；两份只在 Status 行索引注记与 Links 各加一条前向指针，正文不动。
- VE CONTEXT「追踪投影与里程碑」加一条现行规则：源上下文的更正以另一份事实身份给出时，生效的更正落在被更正那条链的同一事实引用下、指名前身，不生效的照独立事实派生。CC CONTEXT 没有被停的句子，未改。UC-CC-006 `AT-CC-156` 首发注里「VE 投影今天不随更正关系替代原条目」改为现行口径。
- 代码：`internal/visibilityexception/adapters/customscompliance/derive_on_external_result.go` 加 `correctionLineage`，沿 `Corrects()` 经既有 `FindByKey` 回读到链根；派生命令的引用取链根来源标识，版本取提交版本（链根）或「提交版本/来源标识」（生效更正），`Supersedes` 指名所更正那份的版本。读不动落 `ErrExternalResultNotVisible`（已在未决哨兵名单），坏链落 `ErrExternalResultRecordInconsistent`（不在名单，ADR-0029）。不改 CC、不加迁移、不动 `cmd/` 下装配与路由文件。
- 测试：现状钉 `TestAnInForceCorrectionIsDerivedAsAnIndependentEntryWithoutAPredecessor` 翻成 `TestAnInForceCorrectionLandsOnItsChainRootAndSupersedesTheCorrectedResult`；同文件补链上两次更正、更正先到、不生效的更正、链根记冲突、前身读不动与坏链各格；新文件 `cmd/parcel-dispatch/customs_result_correction_projection_test.go::TestAnInForceCustomsCorrectionSupersedesTheCorrectedEntryEndToEnd` 走真库派发（全部放行 → 收窄 → 改回，每步成员投影只剩链尾、清关随之出入，事实库前身列逐版核对）。
- AT 盘点：`AT-CC-156` 一行去掉「VE 投影不随更正关系替代原条目」缺口、补 VE 两个半边的用例名；`AT-VE-044` 一行补用例名。没手算总览计数（`-check` 退 0，计数不变）。
- ADR 索引在本分支单独一笔重生成。

### 代决（裁定摘要，理由在 ADR-0281 决定与后果）

1. 事实引用落链根 `external-result/<成员>/<链根来源标识>`，不放开跨引用替代。
2. 链根由 VE 适配器沿 `Corrects()` 回读，CC 不另存。读不动是续办；所指读不回、成环、跨层/提交版本/范围、归属不上、指着另一个键、生效却无所指、链中段记冲突是不变量已破。链根本身记冲突时照停在它上面。
3. 链根版本取提交版本（存量不动）；链上各份取「提交版本/来源标识」；前身指名所更正那份在 VE 上的版本。
4. 只有 `CorrectionInForce` 登替代；范围不等的更正照独立条目；记了冲突的（含链尾冲突）入账不派生。
5. 派生即替代，不按业务时间；被替代之前的投影版本按版本读回，按时点问的是 CC 放行读口。
6. 客户视图沿投影换代既有机制（`Reaffirm`/`Supersede`），不另立；按条目披露时的追加更正归那张票。规模未超 M，第 6、7 件不拆票。
7. 存量不回填、不重投，首发前 `--reset`。
8. 过渡期不设运营提示，只记风险。
9. 链尾冲突与同层冲突同走票 demo-chain-gaps/124 的冲突关系与信号；VE 替代链分叉从本路走不到。

### 越权风险点（待 VE owner 复核；第 1 点同时待 CC owner）

照 ADR-0281「越权风险点」：
1. 代 CC owner 把一跳更正关系读成链；CC 若放开跨版本、跨层或跨范围的更正，要重裁。
2. 版本拼法进了 VE 事实键，改拼法要迁事实库。
3. 派生即替代与 CC 按时点读之间的可见差（更正的业务时间晚于派生时刻、或真实程序另有适用时间）。
4. 链根记冲突时前身不在场，运营查阅找不到被指名那一版。
5. 存量不自愈，今天没有按事实库重派生的运维路径。
6. 过渡期无提示。
7. 收窄更正把已归入的清关拉回；按条目披露落地前客户读不到撤回的是哪一条。
8. 范围不等的更正仍两条并存，单元以下范围落地后回来重裁。

### 未决

- 无新立票：规模在 M 之内，第 6、7 件已在本票裁完并落地（客户视图沿既有换代、存量不回填），无未做的半边。
- 冲突关系与信号仍归票 demo-chain-gaps/124。

### 被权限拦下的改动

无。

### 测试

- 先红后绿：新用例与翻过的现状钉在实现前全红（不生效的更正那一格本就绿，钉的是不变的口径）。
- 变异六处全杀：引用不落链根（适配器四格与真库用例红）、前身恒取提交版本（链上两次更正红）、不核范围（坏链「前身在另一范围」红）、链中段冲突放过（坏链红）、读不动当坏链（续办格红）、不登前身（适配器四格与真库用例红）。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh -count=1 ./internal/visibilityexception/... ./internal/customscompliance/... ./cmd/parcel-dispatch/ ./cmd/parcel-api/ ./internal/architecture/... ./migrations/...`：29 包 ok，无失败。改动包的反向依赖（`go list -test` 算）只有 `./cmd/parcel-dispatch/`，已含。
- 新用例 `-count=3`（适配器七个、派发真库一个）全 ok。
- `go build ./...`、`go vet ./...` 干净，`gofmt -l` 无输出；`go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 退 0（前者在索引重生成那笔之后）。
- 无迁移，种子未受影响（演示种子不写外部结果），未灌一次性库。
