# 12 同层先后到达的关务结果一律判冲突：先部分放行、后全部放行时，全部放行被吞

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-dcg12（2026-10-21，见文末「完成记录」）。此前 2026-10-20 升 A 并认领（product-strategy-boundary/18 同期开工）；needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [11](./11-external-result-received-has-no-subscriber.md) 的隔离评审。
Blocked by: [product-strategy-boundary/18](../../product-strategy-boundary/issues/18-customs-case-and-declaration-submission-entry.md)（关务立案与申报提交入口；那张开工时本票须同期或先修，见 [triage-2026-10-19](../triage-2026-10-19.md)）
出处：[ADR-0197](../../../docs/adr/0197-customs-external-results-enter-tracking-projection-per-layer.md) 越权风险点 5。

## 现象

CC 收外部结果时，同层、同提交版本、同范围下只要原始语义不同就判同层冲突。按 ADR-0197，冲突的那份不进 VE 投影。由此：

- 先部分放行、后全部放行（剩余范围后来也放了）：后到的全部放行被判冲突，成员永远到不了 `CUSTOMS_CLEARED`。
- 同一层先后几次不同的监管过程决定（例如先查验、后补资料）：第二次起都被吞掉。

## 证据（读码）

`internal/customscompliance/domain/external_result.go` 的 `CheckLayerConsistency` 逐一比对同层既有事实；`internal/customscompliance/application/receive_external_result.go` 遇 `domain.ErrLayerConflict` 把新到那份记为冲突。VE 的 `internal/visibilityexception/adapters/customscompliance/derive_on_external_result.go` 对冲突记录入账、不派生。

## 三类

机制（同层后续事件与矛盾怎样分）。规模 M。

## 最小的诚实修法

CC 区分「同层后续事件」与「同层矛盾」：例如放行层按范围单调推进（部分 → 全部）算后续，不算冲突；监管过程决定按发生时间成序列。真正的矛盾（同一时刻同范围两种结论）照判冲突。VE 侧的冲突信号（UC-VE-001 `CC-RESULT`）要 CC 先给出冲突关系，可并在本票或另拆。

## 要裁的

放行层的单调推进怎样判（范围包含关系、放行种类的偏序）；监管过程决定是否天然是序列。归 CC owner。CC CONTEXT「与同层现有事实冲突时，不得据此猜测提交、补造缺失层次或按最后到达直接改变当前判断」那句的边界要一起说清。

## 既有票

none

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 `claude-dcg12`）

### 接票时核实

现象与证据逐条复核成立。另核得一处票面没写的后果：CC 的放行读口同样排掉记了冲突的那份，动作准入与放行门禁核对读的就是它。所以先部分后全部时，不只 VE 到不了清关，动作准入也一直答「只有部分放行」、停在未决。VE 投影一侧不用改：每份未记冲突的接收各成本层条目，引用含来源标识；全部放行那条按映射归清关，部分放行那条未归类。结果范围今天只有申报单元一级（接收编排要求范围就是在册单元标识）。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」，代 CC owner）

权威落在 [ADR-0226](../../../docs/adr/0226-successive-same-layer-customs-results-are-successors-unless-they-contradict.md) 与 CC CONTEXT「同层后续事件不是同层冲突」一条。这里只记对「要裁的」的答案：

1. **放行层的单调推进**：只比同一提交版本、同一范围。放行种类的偏序：全部不窄于任何一种，部分与附条件各只不窄于自己、互不可比。按业务发生时间，后一份不窄于前一份为后续事件；同一时刻种类不同、全部之后回退、部分与附条件之间改换为矛盾。放行层的原始语义与条件说明不进判断。范围包含关系今天退化为相等，因为范围只有单元一级。说不出种类的放行层存量退回按原始语义判。
2. **监管过程决定天然是序列**：它是审核过程中先后下达的指令，每一项都是新发生的监管事件，后一项不否定前一项。只有同一时刻异义才是矛盾。
3. **其余四层**（监管接收、业务受理、监管核定税费、监管处置决定）一版提交一个结论，异义即冲突，不看业务时间：后一份是更正还是追加，要等来源指明更正关系。
4. **CONTEXT「不得……按最后到达直接改变当前判断」的边界**：这句只管矛盾。后续事件按业务发生时间进入当前判断：乱序时后到的那份业务时间更早，排进序列早处，当前判断不变。矛盾时冲突标在后到那份，判断以它到达时在册的事实为准；被标的是哪一方取决于到达顺序，这是那句话的直接后果，对给定顺序答案确定。
5. **判断对称**：两两不矛盾的一组按任何到达顺序都不记冲突。比对含已记冲突的在册事实（保守的一边）。
6. **偏离原行为一处**：在册事实的放行事实与层次配不上时整笔报错。原先这一格答未受理，可那是仓储不变量已破，不是来源的错。
7. **存量不自愈**：已记的冲突标记不重判。接收记录只建立不改写；重判要形成新判断及关系，CC 没有这条路。首发前只有演示租户，`--reset` 重灌即可。同一来源以新来源标识再到时，按新判法形成。
8. **VE 冲突信号另拆 [124](./124-customs-layer-conflict-relation-and-tracking-signal.md)**：冲突关系（与谁矛盾）的形状要随它的消费方、存量回填与迁移一起定。本票不改冲突那份「入账不派生」（ADR-0197 决定五）。

### 越权风险点（全文见 ADR-0226「越权风险点」）

1. 代 CC owner 定放行偏序：「部分之后剩余附条件放行」判冲突，门禁对该单元只读到部分放行而答未决——不会误放，但附条件那份看不见。要放开改 `ReleaseKind.atLeastAsComplete` 一处。
2. 范围包含关系退化为相等；单元以下的范围由真实程序给出时要补。
3. 「同一时刻」依赖来源业务时间的精度。
4. 过程决定成序列不看语义，撤销类决定也进序列。
5. 监管核定税费换金额、处置决定换种类照判冲突，要等层内更正关系。
6. 比对含已记冲突的在册事实，偏保守。
7. 真矛盾时被排除的是后到一方，即使它业务时间更早；按权威或更正裁决要等冲突处置入口。
8. 存量不自愈（同时待 VE owner）。
9. 在册事实配不上层次，HTTP 上从 2xx 业务答复变 5xx。

### 做了什么

- `internal/customscompliance/domain/external_result.go`：`LayerFact`（分层事实连同放行事实）；`CheckLayerConsistency` 按层判后续与矛盾，判断对称；`ReleaseKind.atLeastAsComplete` 是放行种类偏序。
- `internal/customscompliance/ports/ports.go`：`ExternalResultStore.LoadForSubmission` 交回 `[]domain.LayerFact`。
- `internal/customscompliance/adapters/postgres/external_result.go`：同层一致性读面左连放行事实；放行行重建抽成 `rebuildReleaseOutcome`。
- `internal/customscompliance/application/receive_external_result.go`：新到那份连同放行事实交进核对；在册事实配不上层次整笔报错。
- 测试替身（application、http）随端口改。
- 文档：CC CONTEXT 只加「同层后续事件不是同层冲突」一条；UC-CC-006 新立 `AT-CC-431`–`436`，「同层冲突裁决」待决项注明已定为机制的部分；UC-VE-002 新立 `AT-VE-186`；ADR-0226；ADR-0197 Status 行与 Links 加前向指针，不停用条款；AT 盘点加行与追记（计数未动）；立票 124。
- 无迁移，未动装配文件与接线棘轮基线。

### 用例

- 领域 `external_result_test.go`：放行按种类随业务时间推进的十三格（含乱序、同刻、回退、部分与附条件改换、说不出种类的存量）；过程决定成序列与其余四层一版一结论；全对称性检查与三份链的六种到达顺序；不同范围与不同层各归各位；放行事实与层次配不上即不成形。
- 编排 `receive_successive_results_test.go`（替身结果库把已存记录交回下一次接收）：先部分后全部与乱序都记下、都交出；先后两项过程决定都记下；同刻两种结论与回退标在后到那份、先到那份不动；重投答已有记录、存量冲突标记重放不改；在册事实配不上层次整笔报错、不落记录。
- 结果库真库 `release_outcome_test.go::TestLoadForSubmissionCarriesEachReleaseKind`：部分、全部各带种类，非放行层与说不出种类的存量不带。
- 派发真库 `cmd/parcel-dispatch/successive_customs_results_projection_test.go`：生产结果库、提交索引、解释规则读口、辖区链与 Outbox 交接组成的接收编排，加生产 `wireDispatcher` 与演示映射种子。覆盖四件：
  - 先部分后全部、全部先到两种顺序：两成员都归入 `CUSTOMS_CLEARED`，部分放行未归类，放行读口两份都交出。
  - 先后两项过程决定都进投影。
  - 同刻两种放行种类：后到那份判冲突、信封定稿不派生，放行读口只交先到那份。
  - 全部放行重投：答已有记录，Outbox 一行，VE 收件账一行，成员上仍两条。
- 改前核对：把 `contradicts` 临时改回旧判法，派发真库前两条用例按票面现象红（`LAYER_CONFLICT`），改回后绿。

### 验证

于 `32b8fec3`：`go build ./...`、`go vet ./...`、`gofmt -l cmd internal scripts tools` 干净。`scripts/test-full.sh` 跑了 CC 全部包、改动包的反向依赖（NR/PS/SA/TF/VE 各自的 customscompliance 适配器）、`./internal/visibilityexception/...`、`./internal/architecture/...`、`./migrations/...`、`./cmd/...`，46 个包全绿。未跑全量；未重生成机制清点、ADR 索引与 AT 计数（`go run ./tools/adr-index -check` 如预期答 README 未重生成）。

### 偏离与待裁

- 偏离原行为一处，见代决 6。
- [product-strategy-boundary/10](../../product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) 第 5 项「待核：……外部结果冲突、冲突前当前判断……的判断规则」：判断方法中的同层后续与冲突一半，现在有了执行器（ADR-0226）。本票没改那张票面与参数登记册，留给它核对时引用。
- 进 main 时：ADR 号、票号 124、AT 号若改号，全仓搜 `ADR-0226`、`0226-`、`124-customs-layer-conflict`、`AT-CC-43[1-6]`、`AT-VE-186` 一并替换。ADR 索引、AT 盘点计数、机制清点按批重生成。
- 已在演示库里手工走过「先部分后全部」的，旧冲突标记留着，要 `--reset`。种子本身不含外部结果。
