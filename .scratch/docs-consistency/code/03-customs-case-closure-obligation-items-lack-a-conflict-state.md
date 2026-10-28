# 03 CC 关闭依据项只有三值，缺「冲突」

Category: bug
Status: resolved——2026-10-28 进 main（`b59215776`，code/03 与 code/34 同批；全量 160 ok / 0 FAIL，种子退 0），见文末「进 main 记录」。此前 in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-code0334（代码最后一笔 ee74961e1）。此前 in-progress——2026-10-27 会话 tops-20（idp-parcel-91）接手 tops-26 断在中途的「上线前必须」一组（冻结清单「六」），分支 claude-code03。此前 ready-for-agent——R3（不急）：关案编排 `CloseCustomsCaseHandler` 在 `cmd/` 下没有装配，今天没有生产路径；接线之前必须先修，否则冲突项会被迫读成未解决或已终结。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/cc.md](../audit/cc.md) M8（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

文档里关闭依据项是四值（已终结、已移交、未解决、冲突），任一未解决或冲突项都阻止关闭；代码只有三值，没有「冲突」，头注却引了含「冲突」的那句 CONTEXT。

## 证据

- 文档：
  - CC CONTEXT Language「关闭依据项」：「并分别表达已终结、已移交、未解决或冲突」；Rules：「任一未解决或冲突项都阻止关闭」。
  - GLOSSARY「关闭依据项」同义；UC-CC-010 输入表「已终结、已移交、未解决或冲突结论」。
- 代码：
  - `internal/customscompliance/domain/case_closure.go` 的 `ObligationItemState` 注释「关闭依据项的封闭三值：已终结、已被有权接收方有效承接、未解决」，取值 `CONCLUDED` / `HANDED_OVER` / `UNRESOLVED`。
  - `application.NewCloseCustomsCaseHandler` 在 `cmd/` 下无调用方。

## 三类

机制。无产品策略、无租户取值。

## 规模

S：领域加一格并带冲突依据引用，关闭核对把它当作阻止项；补用例。

## 最小的诚实修法

`ObligationItemState` 加「冲突」：阻止关闭，必须带冲突依据（与哪几份事实矛盾）。冲突依据的形状与票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md) 要立的冲突关系对齐，124 先落就直接引用它。

## 要裁的

无（CONTEXT 已定四值）。审计给的另一条路——owner 裁定冲突并入未解决、改 CONTEXT 与 GLOSSARY——属 B，归修订票 docs-consistency/03；那张票若这样裁，本票作废。

文档自洽 03（2026-10-21）复核：不走「冲突并入未解决」那条路——CONTEXT、GLOSSARY 与 UC-CC-010 的四值维持，本票照旧有效。


## 完成记录（2026-10-28，作者子代理，分支 claude-code0334）

起点 origin/main `94ef6908b`。提交见 Status 行（代码一笔）。

### 改了什么
- 领域 `ObligationItemState` 加 `ObligationConflict`（`CONFLICT`）；`ClosureObligationItem` 加 `ConflictsWith []string`，`complete()` 双向核：冲突项至少两份互不相同的非空事实引用，非冲突项不带。新增 `BlockingItems()`（未解决加冲突）、`ConflictItems()`、`Equal`（含切片后结构体不能用 `==`）；`Closable()` 改认阻止项。`UnresolvedItems()` 仍只含未解决。
- 应用：`ClosureBlocked` 的清单改用 `BlockingItems()`，`Unresolved()` 注释写明含冲突项与冲突依据；`register_case_configuration.go` 的内容比对改用 `Equal`。
- 迁移 `customs_compliance/0029_closure_obligation_item_conflict.sql`：加 `conflicts_with text[]`、状态 CHECK 扩四值、不可变函数 `conflict_basis_valid` 加双向 CHECK（CHECK 不许子查询，故用函数）。
- 适配器：registrationjson 译装、postgres 登记写口、盘点读口、目录查阅、关闭记录快照 JSON、HTTP 查阅体都带上四值与 `conflictsWith`。
- 测试：domain、application、postgres（含 CHECK 拒绝各路）、`cmd/parcel-customs-register` 译装。

### 代决与越权风险点
1. 冲突依据形状：124（冲突关系）未落、仍冻结，故取「至少两份互不相同的事实引用字符串」；124 解冻时若形状变，迁移 0029 的 `conflicts_with` 是唯一要换的列。
2. 「至少两份」是代决的下限（一份事实不构成冲突）；CONTEXT 只说「冲突」未给数量。
3. 未新增 AT 号：UC-CC-010 的现有 AT 未单列冲突项，沿用 `Covers:` 引 CONTEXT 规则句。

### 判据对用例
UC-CC-010「每个关闭依据项只能采用已终结、已移交、未解决或冲突之一……未解决或冲突必然阻止当前关闭」：四值齐，冲突阻止关闭，清单带冲突依据。接线：票面未要求，`CloseCustomsCaseHandler` 仍未在 `cmd/` 装配，未接。

### 自验
`go build ./...`、`go vet` customscompliance 与 cmd、`gofmt -l` 无输出；带 PG DSN `go test -count=1 -p 1` 跑 `./internal/customscompliance/...`、`./internal/architecture/...`、`./migrations/...`、`./cmd/parcel-customs-register/...`、`./cmd/parcel-api/...` 全绿。

### 留给协调方
- 无新发现需立票。

## 评审记录（2026-10-28，隔离子代理，钉 `2da1e325b`）

两轴无阻断（与 code/34 同一评审）。Spec 轴：四值与「任一未解决或冲突项都阻止关闭」对上；迁移 0029 DROP/ADD 0008 的约束名成立，存量行无需回填；关闭记录快照带冲突依据不与已存摘要失配；没新增 AT、`Covers:` 引 CONTEXT 规则句合规；冲突依据「至少两份互不相同的事实引用」无 CONTEXT/ADR 出处，属机制层结构约束、不是租户取值，作者已标为代决（124 解冻时只换 `conflicts_with` 一列）。Standards 轴非阻断：领域层 `conflictBasisMatches` 不 trim，译装层与库 CHECK 会 trim，三处口径不一（进库时被 CHECK 拒，不静默写错）——记入冻结清单。已发布迁移无改动。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-code0334` tip `2da1e325b` 作出处；在隔离树上 cherry-pick 到 main `1e328bc17`，无冲突。
- 重生成 ADR 索引、AT 盘点计数与机制清点，有变化的各成一笔；两项 `-check` 退 0。
- 全量 `scripts/test-full.sh`：160 ok / 0 FAIL。种子灌一次性库退 0，用后删库。
- 推确切 SHA `b59215776` 到 main。编排在 `cmd/` 下的装配照票面不在本票。

