# 136 段续办补进段之后没有东西唤醒补派生，那次出发丢出追踪

Category: bug
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg135` tip 为写本完成记录的那一笔（代码与文档最后一笔 `d1f8308da`，本票自己的最后一笔 `7fa90b1b6`；见文末「完成记录」）。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，与 135 同批，分支 `claude-dcg135`（worktree `idp-parcel-dcg135`）。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M19；ADR-0224 越权风险点 1

## 现象与修法

现象、证据与修法的完整写法以复核报告 M19 一条为准（单一权威），此处只记要点：

二选一：续办进段成功时交一封唤醒信封；或补派生改按对象全扫。并在 ADR-0224 风险点 1 末尾与票 53 完成记录补本票号。

## 完成记录（2026-10-26，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcg135`）

起点 main `449a25508`。与票 135 同批同分支（135 的提交见那一票）。

### 二选一：取「补派生改按对象全扫」

理由（详见 [ADR-0302](../../../docs/adr/0302-the-movement-fact-backfill-scans-every-segment-of-the-object-so-a-later-control-fact-heals-a-segment-entered-without-an-envelope.md)）：

1. **唤醒信封无处可挂。** 读码（钉 `449a25508`）：运输履约今天没有「续办进段」那一路。同一份控制事实重放时，交接、对象级揽收登记与揽收执行三口都答已有版本、不碰段册（`existingResult`；票 tf-unwired-seven/08 完成记录早已写明「重放不重进段」）；更正只替代凭前版入场的参与，对象从没进过段时答 `NO_PARTICIPATION_TO_REDERIVE`。「续办进段成功」那一刻不存在，造一条续办路（重放要不要重进段、谁来续、续不上答什么）是运输履约的产品裁决，还要加一类信封、分区登记与生产方合同，ADR-0224 决定一当初正是为此没造「段已成立」信封；揽收执行那一口也要动。
2. **全扫不挑进段的路。** 对象不经任何一封控制事实信封进了段——日后若立的续办路、存量迁移、直写段册——都由它的下一份转出控制的交接或对象级揽收补上；ADR-0224 越权风险点 6 的大半与「上线前已丢的出发」里此后还有控制事实的对象顺带有了去处。
3. **判据与幂等键不变**：在控取 `ObjectsUnderControlAt`，派生走同一处 `deriveMovementFactOnto`，早已派生过的答已有结果；代价是读量（越权风险点 2）。

### 提交

| SHA | 内容 |
|---|---|
| `24d870d4c` | 代码：`BackfillMovementFactsAdapter` 按对象扫它有参与的每一段（`backfillObject`、`segmentOf` 取代按入场依据认段的 `backfillEntered`、`segmentEnteredBy`）；替身用例改写（AT-VE-180 去掉「段不是这一次进的」两个子用例），新立 `TestTheNextControlFactBackfillsASegmentTheObjectEnteredOtherwise`；派发真库用例 `TestTheNextHandoverBackfillsADepartureOnASegmentTheObjectEnteredWithoutAnEnvelope` |
| `7fa90b1b6` | 文档：ADR-0302（新）；ADR-0224 Status 注记（部分停用决定三一句、越权风险点 1 已答）、Links、文末 10-20 复核记录（M19 → 本票）；VE CONTEXT 两句；UC-VE-002 `AT-VE-180` 改、`AT-VE-204` 新立；AT 盘点；票 53 完成记录越权风险点 1 补票号 |
| 本笔 | 两票完成记录 |

### 改了什么

- **VE 适配器**（`internal/visibilityexception/adapters/transportfulfillment/backfill_movement_facts.go`）：交接（只认转出控制的那一版）与对象级揽收两封信上的补派生，扫对象有参与的每一段，每段上按段列事实、事实那一刻对象在控的就补派生。触发、只补这一件、只补投影、失败分格、路由与哨兵名单都不变；`cmd/parcel-dispatch` 装配与路由文件没动。
- **运输履约一侧没动**（没碰 code/01 名单内的任何文件，也没碰别的 TF 文件）。`EnteredByPickup` 从此没有生产调用方，原样留着（ADR-0302 越权风险点 5）。
- **ADR**：新立 ADR-0302（代 VE owner 决）；ADR-0224 只动 Status 行注记、Links 与文末新加的复核记录，正文不动。ADR 索引没重生成（试跑生成器确认能生成，已复原）。
- **VE CONTEXT**（「追踪投影与里程碑」入账不派生那一条下，逐句）：
  - 原「段不在册的那一条，对象日后凭`已交接`或对象级场外揽收进了那一段时，由那一份控制事实补派生（ADR-0224）。」→ 现「段不在册的那一条，对象日后进了那一段，由它进段那一份或此后任一份`已交接`、对象级场外揽收补派生——补派生扫对象有参与的每一段，不问它凭什么进的段（ADR-0224、ADR-0302）。」
  - 原「补派生时没有可补的移动事实：交接是拒收或待确认，没让对象进段；对象不在任何段，或它参与的段不是这一次交接或收寄进的那一段；那一段上没有声明它的移动事实；对象进段晚于事实发生（ADR-0224 决定三、四）。」→ 现「补派生时没有可补的移动事实：交接是拒收或待确认，没让对象进段；对象不在任何段；它参与的段上没有声明那一段的移动事实；对象进段晚于事实发生（ADR-0224 决定三、四，ADR-0302）。」
  - 生命周期「全程追踪投影」那一句「段日后登记、对象凭控制事实进段时，由那一份控制事实补派生」没改：它说的那一格照旧成立，不经信封进段那一格由规则句说。
- **UC-VE-002**：`AT-VE-180` 去掉「对象有参与的段不是这一次交接或收寄进的」；新立 `AT-VE-204`；段末验收说明加一句指向它。
- **票 53**：完成记录越权风险点 1 末尾补「承接票 136、ADR-0302」。

### 代决与越权风险点

代决：

1. **二选一取全扫**（理由见上）。读了 ADR-0224 全文、票 53 全文、TF 进段、重放与更正重派生各口；没读 TF owner 对续办入口的任何裁决（没有）。owner 若要唤醒信封：TF 先裁续办进段的入口，再由那一路交唤醒信封；全扫可留作兜底，或退回 ADR-0224 决定三。
2. **需要新 ADR**：全扫改掉了 ADR-0224 决定三「只看这一次控制事实让对象进的那一段……认到就停」一句，是部分停用，停用方必须是一份记录；号 0302 用在这里，票 135 改走 ADR-0205 注记「已答（落地）」。
3. **触发不扩**：拒收与待确认的交接照旧连段都不读；有效交付信封不挂补派生（ADR-0302 候选与反方、越权风险点 1）。
4. **ADR-0224 文末补一则 10-20 复核记录**写 M19 → 本票：派单要的是「越权风险点 1 末尾补票号」，正文冻结，照 ADR-0205 已有的同形复核记录办；ADR-0302 的来向只写在 Status 注记里，复核记录不重说。

越权风险点（详见 ADR-0302 文末）：

1. 补上要等对象的下一份转出控制的交接或对象级揽收；末段（此后只剩有效交付）照丢（VE、TF owner）。
2. 读量随对象走过的段数线性增长（运维）。
3. 坏行波及加宽：进过某段的对象此后每份控制事实都会停在那一段的坏行上（TF、VE owner、运维；去处票 89、114）。
4. 扫到的早先各段同样按消费那一刻段册算（VE owner）。
5. `EnteredByPickup` 没有生产调用方了（TF owner）。
6. 选全扫而不是唤醒信封是代决（VE、TF owner）。

**另报一格（不在本票修）**：运输履约「段那一半欠账」今天没有任何续办入口——HTTP 注释说「调用方就没法续办那一半」，可同一份控制事实重放不重进段，欠着的那一半只能等另一份控制事实或人工直写。这是 TF 侧的缺口，建议协调方另立票交 TF owner 裁续办入口的形状；若那一路日后要立刻补追踪，再按 ADR-0302 越权风险点 1 交唤醒信封。

### 修法 → 用例

| 修法（M19 / 票面） | 用例或落点 |
|---|---|
| 二选一：补派生改按对象全扫 | `AT-VE-204`：`backfill_movement_facts_test.go::TestTheNextControlFactBackfillsASegmentTheObjectEnteredOtherwise`（交接连同这一版进的段、这一版进段欠账、揽收、重扫只答已有结果）；派发真库 `movement_fact_backfill_test.go::TestTheNextHandoverBackfillsADepartureOnASegmentTheObjectEnteredWithoutAnEnvelope`（出发先登、直写段册进段、下一次交接经生产路由补出 DEPARTED） |
| 既有行为不回退 | `AT-VE-178`、`179`、`181`、`182` 原用例与派发真库用例照绿；`AT-VE-180` 余下各格照绿 |
| ADR-0224 越权风险点 1 末尾补票号 | ADR-0224 Status 注记与文末复核记录（`7fa90b1b6`） |
| 票 53 完成记录补票号 | `7fa90b1b6` |

提交前变异抽验：让补派生只扫最近那一段（退回只看一段），替身用例与派发真库用例都转红。

### 自验

- `go build ./...`、`go vet ./...` 全仓过；`gofmt` 干净。
- 带 DSN `go test -count=1`：`./internal/visibilityexception/...`、`./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-ve-register/...`、`./internal/architecture/...`、`./migrations/...`、`./internal/platform/migrate/...`，21 个包全 ok（含 PG）；新派发真库用例 `-v` PASS。迁移改号之后另跑 `./migrations/...`、`./internal/platform/migrate/...` 与 VE PG 适配器包，全 ok。
- `tools/adr-index -check`、`tools/at-coverage-counts -check` 只报生成物过期（预期）。没跑全量。
