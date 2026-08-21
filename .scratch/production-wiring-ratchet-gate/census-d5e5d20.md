# 三族普查数字（基线 `d5e5d20`）

> ## ⛔ 实现方：完成自己那一份普查之前不要打开本文件
>
> 本文件是[票 01](./issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md) 的普查结果，
> 由 MCP-2 一人得出。票 01 的第一条方向约束**不可逆**，初始清单错一行就会一直锁在里面——
> **一个把错误吸收进基线的门禁永远不会红**，而那正是票 01 要治的病本身。
>
> 所以它被从票面拆出来单独存放：**你可以整篇读票，不必记行号、不必自律**。
> 顺序见票 01 的「开工条件」一节。读本文件是第 4 步，不是第 1 步。
>
> 拆成两份文件而不是在票里写一句「读到这里请停」，理由与票 01 通篇一致：
> **能用结构守的就不要留给纪律守**——「先读判据后读数」做到与没做到的产物长得一模一样。

判据：**某个生产端口、命令处理器或领域工厂的构造函数，其全部调用点是否都在 `_test.go` 里。**

三族分开数，**不做加总解读**——它们的「接线」含义不同（交接口接的是 outbox 装配，处理器接的是
进程入口，工厂接的是处理器），混成一个数字会让下一个人按错的含义去核。

## 一族：outbox 交接口 `NewOutbox*Handoff`

`internal/` 下共 **46** 个构造函数。

`cmd/parcel-dispatch/assemble.go` 是全仓唯一装配处——`cmd` 下非测试代码里 `Handoff` 只出现在这
一个文件，`cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-pricing-register` 三个进程零处。
装配 5 口、6 个调用点：

- `nrpostgres.NewOutboxInitialRouteHandoff`
- `vepostgres.NewOutboxCustomerViewHandoff`（两处）
- `vepostgres.NewOutboxProjectionHandoff`
- `pspostgres.NewOutboxFinalOutcomeHandoff`
- `pspostgres.NewOutboxNetworkIntakeHandoff`

**41 个零生产调用点。**

## 二族：应用层命令处理器 `New*Handler`

`internal/*/application/` 下共 **62** 个构造函数。`internal/` 非测试代码里零调用点（全部命中都是
声明本身），生产调用点全在 `cmd/`，共 9 个：

- `cmd/parcel-pricing-register/main.go` 两个（价卡登记、参考序列登记）
- `cmd/parcel-commercial/main.go` 一个（商业授权发布）
- `cmd/parcel-dispatch/assemble.go` 六个（初始路由、改路重评、客户视图派生、投影派生、终局形成、
  收寄采用）

**53 个零生产调用点。**

## 三族：领域工厂

`internal/*/domain/` 下的 `Form*` / `Establish*` / `Fix*` / `Judge*` / `Grant*` / `Cut*` /
`Publish*` / `Accept*` / `Propose*` / `Verify*` / `Open*` / `Record*`，共 **89** 个构造函数。

**13 个零非测试调用点**：`EstablishCase`、`EstablishSegmentWithHandover`、
`EstablishSegmentWithPickup`、`FormAuditedPayable`、`FormChargeAdjustment`、`FormDutyCollaboration`、
`FormLoadAssignment`、`FormSupplierCreditNote`、`FormSupplierExpectedCost`、`OpenDispatchTask`、
`PublishChannelAccountUseAuthorization`、`RecordMovementFact`、`VerifyDutyPayment`。

## 汇总（只是表尾，不是结论）

| 族 | 构造函数 | 零非测试调用点 |
|---|---|---|
| outbox 交接口 `NewOutbox*Handoff` | 46 | 41 |
| 应用层命令处理器 `New*Handler` | 62 | 53 |
| 领域工厂 `Form*` 等 | 89 | 13 |
| 合计 | 197 | 107 |

**那个 197/107 被当成 KPI 就完了**，它没有业务含义。三族并排看，不相加。

## 这 107 个不是 107 个缺陷

绝大多数是 SYN-WALL-DOOR-AUDIT 十八墙里还没建门的口，属**缺席**（门还没建）而非**在场且错**。
这条界线要写进门禁注释，否则下一个人会把清单长度当成待修工量。

## 三族的数字为什么不能横向比——判据是逐跳的，不是传递的

领域工厂只有 13/89 落网，看起来这一族「基本都接上了」。**不是。** 领域工厂的调用方是应用层命令
处理器，而那一族有 53/62 自己没有进程入口。**一个被未接线处理器调用的领域工厂，在本判据下算
「已接线」**——它确实有非测试调用点，只是那个调用点自己到不了任何进程。

`FormChargeAdjustment` 之所以落网，是因为它连应用层调用方都没有（SA 应用层根本没有形成费用调整
的编排），断在更靠前的一跳。

## 取证方法（供重跑者对照口径，读到这里说明你已经跑完自己那份）

- 构造函数声明：按 `^func <前缀>\w+\(` 在 `internal/` 非测试文件上取，三族各自的前缀集见上。
- 调用点：在 `internal/` 与 `cmd/` 的**非测试**文件上找 `\b<名字>\(`，命中数 ≤ 1 即认定为零调用点
  （那一次命中是声明本身）。
- 三族的前缀集是**判断**不是穷举：领域工厂那一族的十二个前缀取自本仓现有命名，新前缀出现时要补。
  **口径差异优先怀疑这里**，其次才怀疑数数。
