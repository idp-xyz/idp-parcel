# `cmd/parcel-api` 对 TF 新编排的装配与端点

Category: enhancement
Status: draft——等 owner 裁「哪些编排要端点、走哪条 UC」，裁前不转 ready-for-agent
Blocked by: 无

## 现状（钉 `e3dbf3f`）

`cmd/parcel-api` 今天对 TF 装配的只有 `RegisterEffectiveDelivery`（首登与更正）与
`SummarizeHandoverScope`（读面）。**TF 其余全部应用层处理器的构造函数在生产代码里零调用点**
（`git grep -n 'New<Handler>' -- ':!*_test.go'` 只命中各自的声明行），包括：

- 三条早就存在的控制事实入口：`RegisterTransportHandover`、`RegisterOffsitePickup`、
  `PerformOffsitePickup`——进段那道门（`enterFulfillmentSegment`）由它们内部调用，**所以进段
  在端点层同样没有路走到**，不是「随既有端点自然走到」。
- `tf-unwired-seven` 新落的：`EndFulfillmentParticipation`（交付 / 下一次交接 / 明确终止三路）、
  `RecordMovementFact`、`OpenDispatchTask`、`FormLoadAssignment`、`RegisterFailedAttemptCharge`。
- 本 spec 票 01 的 `CloseFulfillmentSegment`。

**这不是 TF 独有的形状。** `.scratch/admin-remainder-mechanism-batch/` 的 T2 普查（二族「应用层
处理器 `New*Handler`」，`7d68475` 上 68 个声明、35 个零非测试调用点）已把 TF 那三条老入口连同
全仓另三十来条一并数在里面。所以**本票的范围只该是 TF 这一组的产品判断**——哪些该有外部端点、
哪些内部触发、哪些暂不接——不是替 T2 做全仓接线；接线机制（端点表、Intake、装配点）沿
`parcel-api-remaining-endpoint-wiring` 的先例，不另发明。

## 为什么它不是 tf-unwired-seven 的遗漏

那批 spec 定的口径是「每条一次四层建设」（表 + 端口 + 适配器 + 编排），完工判据量的是棘轮名单
清空；spec 自己在完工判据一节写明「名单清空只判名单清空，判不了每个入口都接上了」。端点层从
一开始就不在那批范围内——这一票是把那句话里的缺口立出来，不是追责。

## 分两类，处置不同

**一、控制事实入口**：交接登记、场外揽收登记、揽收执行。它们是 CONTEXT 成立边界的来源事实，
接上它们进段才有路走到。接的时候**命令要带段引用**（`RegisterTransportHandoverCommand.Segment`
等字段应用层已有）——HTTP 适配器若不收那一格，就是「编排接上了、端点没让它接上」，与票 08 那个
缝同形，端点测试要专钉一条。

**二、结果与派生事实**：结束参与、关段、移动事实、派送任务、装载分配、失败尝试费。每一条都要答：
- 对应哪条 UC（`UC-TF-003..007`）、哪个执行者（承运方回传？运营人员？系统内部由别的事实触发？）
- Intake 认证走哪一格（ADR-0003 来源信封；`PAR-INT-01` 属实例半边，机制半边照 ADR-0063 显式
  未配置形状先立）
- 结果代数如何映射 ADR-0022 的响应格（`未决` → 5xx `NO_ANSWER_FORMED` 那一格已有先例）

一条从票 01 带过来的：段已关闭后新对象凭正当控制事实来到同段，今天的可观察结果只有「交接在册、
段里没它」——`enterFulfillmentSegment` 对领域拒绝交回空串不留续办，**没有任何一格告诉调用方它该
另立新段**。接端点时要不要为`段已关闭`单开一格答给调用方，在这里裁。

**其中有几条很可能不该有外部端点**：结束参与的交付一路本可由 `RegisterEffectiveDelivery` 落库后
在同一编排里触发；关段是运营决定，更像 admin 写面而不是承运方回传口。这些是产品判断，本票不裁。

## 要 owner 答的

1. 上面第二类六条，哪些开外部端点、哪些做成内部触发、哪些暂不接。
2. 若开端点，按 UC 还是按事实分端点表。
3. 端点接线走 `.scratch/parcel-api-remaining-endpoint-wiring` 那套票的形状（已 resolved，有先例
   可抄）还是另立。

答完再拆票；本票届时转 ready-for-agent 或按答案拆成多张。

## 边界

本票在裁定前不写代码。`cmd/parcel-api/` 是共享接线文件最凶的一处（`endpoints.go`、
`unwired_orchestration.go`、`main.go`），开工前按 `docs/agents/parallel-sessions.md` 占号。
