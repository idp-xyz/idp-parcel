# 未决停在哪一站、原因是什么，在真进程上没有任何人读得到

Category: bug
Status: in-progress——MCP-1；[ADR-0095](../../../docs/adr/0095-undecided-stage-and-reason-surface-in-two-layers.md) 第二层（自愈那格的进程侧观察口）已落 `20d21f4`；第一层随票 07 的 Decision 四/五 切片收口，见文末 Comments 末条

来源：2026-09-02 MCP-1 接手 MCP-5 崩溃后的现场时取证。锚 `9d6063c`（工作树的未提交改动只有 `.md` 与 `.scratch/**`，不含任何 `internal/`）。

## 观察到的事实

    10|`cmd/parcel-dispatch` 连跑 11 分钟，库里那一行累计到 `attempt = 24`、`failure_code = dispatch.consumer_undecided`，**进程输出零行**。

零行不是配置问题，是接线本来如此：

- `ShipmentRequestSubmittedConsumer` 未决时把停在哪一站与未决原因都写进错误正文（`advanceAcceptanceChainThrough` 里那句 `stage %s, reason %s`），注释写着「原错误原样留在链上供运维读」。
- `Dispatcher.DispatchOnce` 逐条发布失败时，只把 `failureCodeFor(err)` 交给 `RecordFailure` 写进库，随即 `continue`——`err` 本体就地丢弃，不返回也不记。
- `cmd/parcel-dispatch` 的 `Loop.report` 只在 `DispatchOnce` **整拍**返回错误时才打日志，而逐条失败按上一条根本不构成整拍错误。

所以那句注释在生产接线里不成立：**没有任何东西在读那条链。** 落到库里的只有一个 128 字节以内的失败码，而失败码按 `failureCodeFor` 的分格判据取值——它答的是「运维该做哪一类动作」，本来就不负责答「停在哪一站」。
    20|
## 缺口的代价，不只是少一行日志

**它把诊断逼成了猜。** 同一天验证隔离形态提交链路时，为了搞清链停在哪一站，只能反过来从库里推：先查各接受判断表（未决整笔回滚，全零，查不出）、再往 `network_routing.network_definition` 插一行探针试图让某一站变响亮。那条路本身也走不通（见票 07 的 Comments），但**它之所以被想出来，是因为进程这一侧什么都问不到**。

**它让票 07 的三条候选路都缺一块验收。** [07](./07-undecided-that-never-self-heals-burns-the-retry-budget.md) 要裁的是「等人去登记」算不算 `未决`；三条路无论走哪条，都预设了事后分得出这次未决属哪一格。今天分不出——连「链答的未决原因是哪一个」都落不到任何持久面上。

**而它在库里长着「一切正常」的脸。** 一封停在未决的信封与一封根本没被消费门碰过的信封，在 `bento.outbox` 上的区别只有 `attempt` 与那个失败码；进程日志两边都是空的。这是 [parallel-sessions.md](../../../docs/agents/parallel-sessions.md)「不同的『绿』在输出上长着同一张脸」那一族的又一个面。

## 不属本票

    30|**失败码分格本身**（`failureCodeFor` 取最响动作格）没有问题，本票不动它：码面答的是动作，不是位置，两件事本来就该分开承载。本票要的是把**位置与原因**补一条出路，不是把它们塞进码面。

**未决该不该烧重投预算**由票 07 裁，本票不重述也不预判。两票可以各自落地：本票补的是可观测性，07 改的是处置。

## 要定的那一句

错误正文在哪一层出声，有两条路：

1. **派发器自己打日志。** 最省事，代价是 `internal/platform/dispatch` 今天零日志是有意的——包注释明写循环、批量与重试节奏属装配职责，它对事件类型一无所知，十个上下文共用同一拍。给它塞一个 logger 等于把「怎么出声」这件事从装配方收回平台层。
2. **把逐条失败交回装配方**（`DispatchOnce` 交回一份逐条结果，或收一个观察口）。形状更正，代价是动 `Beat` 这个所有装配点共用的接口，且要定「交回什么」——整个 `error` 还是已分格的摘要。
    40|
我倾向 2，但它改的是平台层跨十个上下文的接口形状，按 AGENTS.md 属难逆转取舍，留给裁决不自行执行。

## 复现

复现步骤与票 07 完全相同（照那一票的「复现」一节走），差别只在看什么：**别看库，看 dispatch 那个窗口**。链在库里累计失败的同时，窗口里一行都不会出现。

## 参照

`internal/platform/dispatch/dispatcher.go`（`DispatchOnce` 丢弃 `err` 那一处、`failureCodeFor` 的分格判据）、`cmd/parcel-dispatch/loop.go`（`Loop.report`）、`internal/parcelshipment/adapters/inbox/shipment_request_submitted_consumer.go`（`advanceAcceptanceChainThrough` 写进错误正文的 stage 与 reason，以及 `ErrAcceptanceChainUndecided` 那句「供运维读」）、票 [07](./07-undecided-that-never-self-heals-burns-the-retry-budget.md)。

## Comments

- 2026-09-02 · MCP-1（owner 授权自决，裁决落 [ADR-0095](../../../docs/adr/0095-undecided-stage-and-reason-surface-in-two-layers.md)，转 ready-for-agent）。

  **本票立起来之后又发现一层，它把票面缩小了一半。** 领域其实一直在记：`recordAttempt` 会把未决原因、恢复路径与续办引用写成一条处理尝试挂到接受判断任务上，`UC-PS-001` 要的「保存当前判断、失败位置和安全续办依据」正是它。**是消费门的整笔回滚把它擦掉了**——那个消费门自己的注释就记着这条代价。所以不自愈那一格的可观测性不需要新机制，[ADR-0094](../../../docs/adr/0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 把它改成提交入账之后自动补上。

  **剩下的是自愈那一格**，它仍该回滚（回滚是对的，那一轮不该在库里留业务事实），因此只能在进程侧出声。裁的是票面两条之外的第三条形状：**装配方注入的失败观察口**——`Dispatcher` 收一个可选回调，`cmd/parcel-dispatch` 用自己的 logger 实现。不改 `Beat`（所有装配点共用），也不给平台层塞 logger（该包对事件类型一无所知、十个上下文共用一拍，这条分工不是风格偏好）。票面候选一、二的否决理由逐条记在 ADR 的 Alternatives。

  两层的划分不是凑数：会自愈的进日志、不自愈的进库，正是 ADR-0029「按恢复动作分格」在可观测性这一面的投影。

  **一条明确不做的**：本票只补观察，不补告警。「未决持续多久算异常」是运营口径，属实例半边，不填。

- 2026-09-03 · MCP-1（第二层已合入主线 `20d21f4`；第一层的收口挂在票 07）。

  **第二层已落**（ADR-0095 Decision 二/三/四）：`dispatch.Dispatcher` 增可选的 `DeliveryFailureObserver`，在 `RecordFailure` 成功之后、`continue` 之前调用，交出这一条投递、已分格的失败码与**原始 `err`**；用变参 `Option` 给出，既有装配点一个没动，`nil` 时行为与之前逐字相同（有用例钉着）。`cmd/parcel-dispatch` 用自己的 `slog.Logger` 实现，`Warn` 不用 `Error`——这一条失败已入账且会按重投节奏再来，进程本身没坏，整拍失败那一格仍归 `Loop.report`。`assembleDispatcher` 收 logger 而不是收一个现成的观察口，「怎么出声」留在装配层定。日志会按重投节奏重复，那是真实的，不在平台层压噪。**票面观察到的「进程输出零行」从此不成立**：同一现场再跑，每一拍都会把 `stage … reason …` 那句原文打出来。

  **第一层**（不自愈那格靠入账留痕）**今天在库里已经成立，但成立的方式悬着**：`32d6a49` 让消费门对 `ResumeByOperatorRegistration` 入账，`recordAttempt` 写下的处理尝试（原因、恢复路径、续办引用）因此留在库里、委托读面可见——这就是 ADR-0095 Decision 一要的东西。但那次入账本身是否允许在续办触发之前落地，是票 07 末条记的那处要人裁的口子；若裁成暂时收回入账，这一层随之退回「随回滚蒸发」，等 Decision 四/五 切片一起回来。**本票不另设机制，收口跟着 07 走。**

  验证同票 07 末条（钉 `c96065b`，含真库，未跑 `-race`）。

  **完成判据的直接证据补在 `c25d145`**：`TestAnUndecidedStallSurfacesStageAndReasonToTheObserver` 在生产依赖图上接替身观察口，钉住消费门那句 `stage …, reason …` 穿过 `WithUndecidedSentinels` 的 `%w: %w` 包装后仍在。此前三条平台用例钉的是替身发布器上的 `errors.Is`，证不到这一层。建议来自 MCP-4 对 `20d21f4` 的对读。钉 `c25d145` 在临时 worktree 验：gofmt 空、build/vet 退 0、`cmd/parcel-dispatch` 49 条 PASS 非 SKIP（含真库）。

- 2026-09-03 · MCP-4（开工前在 `371f6cb`——本地 main HEAD，非票面旧锚——重取一遍证据；只取证不改代码）。

  票面与 ADR-0095 的结论都不过期：`Dispatcher.DispatchOnce` 逐条 `Publish` 失败仍只把 `failureCodeFor(err)` 交给 `RecordFailure` 随即 `continue`，`err` 本体丢弃；`Loop.report` 仍只在 `DispatchOnce` 整拍返错时打 `dispatch beat failed`；`internal/platform/dispatch` 包内零 logger、零 `slog` 引用。

  与实现直接相关的两条现场事实：`NewDispatcher` 五个位置参数，调用点共九处且全在本会话地盘（`cmd/parcel-dispatch/assemble.go` 一处、`assemble_test.go` 五处、`internal/platform/dispatch/dispatcher_test.go` 两处、`fanout_failure_code_test.go` 一处）——加一个可选参数属「会让旧调用点对不上」那一类，但调用点少且不跨地盘，按 parallel-sessions 走单独 worktree 一次性应用。`wireDispatcher(db, settings)` 今天拿不到 logger（logger 停在 `run` → `NewLoop`），装配方要实现观察口就得把 logger 或观察口穿过 `assembleDispatcher` → `wireDispatcher`，那两个签名同在本地盘。
