# 交接范围汇总读面——`SummarizeHandovers` 至今没有调用方

Category: enhancement
Status: in-progress——MCP-3
Blocked by: 无

## CONTEXT 要求什么

`docs/domain/transport-fulfillment/CONTEXT.md` 对权威交接的纪律里，与汇总直接相关的是**逐对象
分立**：交接结果按载运对象各自成立，不能由整批结果覆盖成员差异。汇总因此只能是**派生量**
——它报出一个交接范围里各裁决各有多少，而不新增任何事实。

领域侧 `SummarizeHandovers` 已经实现了这条：空集不成立汇总；跨范围或跨租户混入即拒绝
（「那不是『这一次交接』的成员」）；产出 `HandoverScopeSummary` 只带 `scope` 与三个计数
（已交接 / 拒收 / 待确认）。

## 现状（锚 `9d6063c`）

- 领域：`SummarizeHandovers` 与 `HandoverScopeSummary` 在 `domain/transport_handover.go`，有测试。
- 编排：`application/register_transport_handover.go` 调 `FormTransportHandover` 登记单条，
  **从不派生汇总**。
- 端口：`TransportHandoverRegistry` 只有 `FindByKey` 与 `Save`，**没有按范围列出**。
- 库：`migrations/transport_fulfillment/0005_...` 已有交接册。

所以这一票**不建表**：事实已经在库里，缺的是「按范围取回并派生」这条读路。这是七票里最小的
一刀，选它先走是为了验证形状。

## 要做的四层

1. **端口**：`TransportHandoverRegistry` 增 `ListByScope(ctx, tenant, scope) ([]TransportHandoverRecord, error)`。
   空范围答空列表不报错——「这个范围还没有交接」与「读失败」是两件事。
2. **适配器**：postgres 侧按 `(tenant_id, scope)` 取回，逐行走 `FormTransportHandover` 重建，
   不在 SQL 里做计数——计数是领域的派生，下沉到 SQL 会立第二个口径。
3. **应用**：新增一个读用例，取回后调 `SummarizeHandovers`，空集按领域的答案交回
   「该范围不成立汇总」而不是一份三个零的汇总。**零与不成立是两件事**。
4. **HTTP**：目录读面照本上下文既有形状（租户显式入参、limit 非正拒、空册答空）。

## 陷阱

- **不要在 SQL 里 `COUNT(*) GROUP BY 裁决`。** 那会绕过 `SummarizeHandovers` 的跨范围/跨租户
  校验，并且立下第二个计数口径——领域改一次判据，SQL 那份会悄悄漂移。本仓已有同形教训
  （`evaluation_snapshot.go` 头注：「在适配器里复刻整张图等于为同一形状立第二个口径」）。
- **汇总不得入库。** 它是派生量，存下来就有了「与源事实不一致」的可能。CONTEXT 说汇总
  「只派生」。
- 重建每一行都要过 `FormTransportHandover` 的构造门，不要按列直接拼结构体。

## 完工判据

`SummarizeHandovers` 出现在非测试的生产调用路径上，棘轮基线里
`internal/transportfulfillment/domain SummarizeHandovers` 那一行可以剪掉；剪之前按基线要求
核一遍全仓是否只有一处同名声明。

## Comments

- 2026-09-02 · MCP-3：**四层里落了两层（端口 + 应用），棘轮那一行已剪。余下适配器与 HTTP。**

  **端口**：新开 `ports.HandoverScopeView`，**没有**给 `TransportHandoverRegistry` 加方法
  ——那会打断所有实现者，属「会让旧调用点对不上」那一类。分开还有个契约上的理由：那个口是
  写侧的幂等存取，这里要的是按范围的只读列举，实现者可以是同一个类型但契约不同。

  **应用**：`SummarizeHandoverScopeHandler`。四格结果（`已汇总` / `不成立汇总` / `未决` /
  `输入未受理`），其中`不成立汇总`独立成格而不复用「三个零的汇总」——零说的是「这个范围有
  交接，只是这一格没有」，不成立说的是「这个范围还没有交接」，调用方要做的事不同。读失败
  形成本上下文自己的未决并带续办引用，不上抛技术错误、也不冒充空范围。

  **计数没有下沉到 SQL**，也没在编排里自己数：`SummarizeHandovers` 自带跨范围与跨租户的成员
  校验，绕过它去数就等于为同一形状立第二个口径。

  **剪基线前按门禁要求分过三种成因**：全仓 `SummarizeHandovers` 只有一处声明
  （`domain/transport_handover.go`），不存在「别处同名声明造成误判」那一种，因此是第二种
  ——真的接上了包外非测试引用。剪后按基线要求**重数实测 32 条**（不是拿 33 减 1 算的），
  并把这一笔记进了它的流水账。

  **未做且要说清**：postgres 适配器与 HTTP 读面都没做，所以**「出名单」等于「有了应用层
  调用方」，不等于「生产可达」**——这与基线里 `label-channel/06` 那条注记的是同一件事。
  `HandoverScopeView` 目前无生产实现。

  **验证**：`go build ./...` 退 0、`go vet ./internal/transportfulfillment/...` 退 0、
  `go test -count=1 ./...` 全仓零 FAIL，架构十一道门禁（含棘轮）全绿。**这是「绿（未设 DSN，
  PG 用例跳过）」不是含真库的绿**——本笔没有 postgres 适配器，那一层也无从验。`-race` 未跑：
  本机无 C 工具链（MCP-6 当日实测 `cgo: C compiler "gcc" not found`）。
