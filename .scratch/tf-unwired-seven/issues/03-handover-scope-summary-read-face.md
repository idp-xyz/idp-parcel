# 交接范围汇总读面——`SummarizeHandovers` 至今没有调用方

Category: enhancement
Status: resolved——四层齐（端口 + 应用 + 真库适配器 + HTTP 读面），棘轮那条已剪；
接进 cmd/parcel-api 装配不在本票判据内，另票认领，理由见文末末条
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
  PG 用例跳过）」不是含真库的绿**——本笔没有 postgres 适配器，那一层也无从验。

  **`-race`：本笔未跑。最近一次实跑钉在 `a573528`（本笔的祖先，零 DATA RACE、零 FAIL、
  93 包 ok），那一跑同样不含 PG。** 本条初稿写的是「未跑：本机无 C 工具链」——**那句话是假的
  且要就地更正**：`-race` 在这台机器上跑得了，只是不在 Windows 侧（Windows 缺 gcc 属实，
  WSL 侧有）。它的来历是一个后来被作者撤回的全称结论，我照引了没有重新取证，正是本仓
  反复记的那一种「推测与取证在写下来之后长得一模一样」。

  **不为本笔单独补跑，是有裁定的**：`-race` 逐笔补会得到一堆各钉不同 SHA、拼不成一句完整
  断言的绿——今晚 HEAD 每几分钟往前走一笔。按频道约定它归收尾那一批，与 `gofmt -l` 为空、
  清点重生成比对三件一次跑齐并钉住同一个 SHA。

- 2026-09-03 · MCP-3：**余下两层已落 `0005897`，本票转 resolved。**

  **适配器**：`ListByScope` 落在 `TransportHandovers` 上，作 `ports.HandoverScopeView` 的真库
  实现。交回范围内全部已登记版本，逐行过 `rebuildHandover` 的构造门，空范围答空列表不报错。
  没在 SQL 里筛，也没在 SQL 里数——理由与票面「陷阱」那条同一条。

  **动笔时核出一处两层之前没说清的事**：更正是新版本新行、原行不删，于是按范围读回的是整条
  版本链，而 `SummarizeHandovers` 原先逐条计数——**一次更正会把一个对象数成两个**，两格裁决
  各多一。折叠落在领域而不在读口：「更正形成新版本使原结果失效或被替代」是 CONTEXT 的判断，
  读口若先筛一遍，就是为同一条规则立第二个口径。回指按对象配对，因为版本标识只在对象内唯一。

  **HTTP 读面** `GET /transport-fulfillment-handover-scope-summary?scope=…`：接的是读用例而不是
  存储读面——汇总是派生量，与 ADR-0077 目录上列的那些不同属，端点直读存储再自己数就是第二个
  口径。门次序照本包查阅面通例（方法 → `scope` 形状 → 准入），租户只取自准入结果。四格答案
  一律 200 进 `outcome`：成立带 `summary`（三格计数 + 总数 + 整批结论原样透出，派生只由领域做，
  不留给页面自己算），**不成立汇总没有 `summary` 键**（三个零与「还没有交接」是两种答案），
  未决带 `reason` 与续办引用，无名结果 500 `UNNAMED_OUTCOME`。

  **未接进 `cmd/parcel-api` 装配，且这不在本票判据内。** 那是共享接线文件（端点表、探针、
  放行表）且要改 `assembleBusinessEndpoints` 的签名，按 `docs/agents/parallel-sessions.md`
  要先占号再另起一笔。它现在有自己的票：`admin-web-audit-followups/06`。**因此本票 resolved
  说的是「四层齐、判据满足」，不等于「生产可达」**——`HandoverScopeView` 在装配点接上之前
  仍无生产实现，与上一条注记里 `label-channel/06` 那件事同形。

  **验证**：隔离 worktree（检出 `828dbfa`）——`go build`/`go vet` 退 0，`gofmt -l .` 空，
  `go test -count=1 -p 1 ./...` 全仓 93 包 ok 零 FAIL（DSN 已设，TF postgres 包实跑；
  `ListByScope` 两例 `-v` 下 PASS 非 SKIP）。共享树 `c808d01` 上应用后再跑：build/vet 退 0，
  架构门禁 ok，TF 四包 ok。棘轮名单未动——上一层剪过之后本笔不新增导出工厂。

  **上一条注记里那三件收尾验证仍未跑**（`-race`、`gofmt -l` 为空、清点重生成比对）。裁定不变：
  归收尾那一批，一次跑齐并钉同一个 SHA。本票 resolved 不把它们带走。
