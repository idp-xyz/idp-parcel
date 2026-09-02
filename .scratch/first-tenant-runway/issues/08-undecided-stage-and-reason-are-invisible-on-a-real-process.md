# 未决停在哪一站、原因是什么，在真进程上没有任何人读得到

Category: bug
Status: ready-for-human（要先定一句：错误正文该在哪一层出声——派发器自己打，还是把逐条失败交回装配方）

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
