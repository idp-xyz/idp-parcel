# FanOut 合并错误里未决盖过硬失败，失败码误导运维

Category: bug
Status: resolved

来自外部评估（基线 `49a2ab0`），协调岗已在 detached 取证树核实机制成立。

## 现象

`internal/platform/dispatch` 的 `failureCodeFor` 用 `errors.Is` 按固定顺序分类：
`ErrNoSubscriber` → `ErrConsumerUndecided` → `eventing.ErrPublishUncertain` → 默认
`dispatch.publish_failed`。而 `fanOut.Consume` 用 `errors.Join` 合并各路错误（注释明确
「每一路都要调到，即使前面已经失败」）。

于是当一路经 `WithUndecidedSentinels` 翻成 `ErrConsumerUndecided`、另一路是硬失败
（如各适配器的 `*RecordInconsistent` 这类「重投不自愈」的不变量破坏）时，合并错误对
`errors.Is(err, ErrConsumerUndecided)` 为真，整封记 `dispatch.consumer_undecided`。
同理，合并里有 `ErrPublishUncertain` + 未决时也记未决，重复投递核对的提示被盖掉。

## 后果

失败码的设计约定是「按运维要做的动作取值」（`failureCodeFor` 注释）。未决盖过硬失败
时，运维按码面去「查消费方等的那个依赖」，而真正需要人动手的是另一路的数据/装配问题；
硬失败若持续，条目以 `consumer_undecided` 之名耗尽失败预算，事后排查也从错的入口进。

## 期望语义

仅当合并错误的**全部**分支都是未决时才记 `consumer_undecided`；出现任何非未决失败时
按该失败自身分格。非未决各格之间的相对优先级按「运维动作紧迫度」在实现里定下并用测试
钉住（`no_subscriber` 是装配错误应保持最响；`publish_uncertain` 与 `publish_failed`
的先后写清理由即可）。

## 边界

- 单消费者路径行为不变；`WithUndecidedSentinels` 的翻译职责不动（派发器仍只认一个哨兵）。
- 改动落 `internal/platform/dispatch`；平台层不得 import 任何上下文错误——判「全部分支
  未决」只能对 `ErrConsumerUndecided` 做结构判断（如走 `errors.Join` 的展开树），不认
  各上下文哨兵。
- 测试至少覆盖：未决+硬失败、未决+不确定、全未决、单路未决、单路硬失败。

## Comments

- 2026-08-20 MCP-1：外部评估四项可操作发现之一（其第 7a 条），核实属实后立票。
- 2026-08-20 MCP-1：MCP-5 完工于 `99f7328`（基线 `49a2ab0`）。该基线期间 `main` 已推进到
  `b4b6a64`，两者为兄弟提交，直推非快进；协调岗在 detached 验证树上 cherry-pick 到
  `b4b6a64`，无冲突（改动落 `internal/platform/dispatch`，`b4b6a64` 只碰 `assemble.go`
  与申报投影测试，文件不重叠），内容与原提交逐字相同。门禁 gofmt / go build / go vet /
  `git diff --check` 全过；全仓 `go test -p 1 -count=1 ./...` 绿，**含 PG**（DSN 已设，
  真库包 15–22s 可见）。新表测试七行经 `-v` 复核确为跑过而非跳过。已推 `a097d7f:main`。
