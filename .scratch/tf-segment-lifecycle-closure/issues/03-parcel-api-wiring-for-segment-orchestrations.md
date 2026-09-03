# `cmd/parcel-api` 对 TF 新编排的装配与端点

Category: enhancement
Status: draft——决策简报已备（文中「决策简报」节，钉 `722e846`），owner 逐行答同意/改即可拆票；裁前不转 ready-for-agent
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

## 决策简报（2026-09-03 · MCP-1，钉 `722e846`，只读取证）

按各 UC 的「责任方」列与「触发」行逐条对表，给出建议处置。**建议不是裁决**：定级与端点取舍是
owner 的产品判断；下表把每条要裁的东西压到一格，owner 逐行答「同意 / 改为 X」即可。

| # | 编排 | UC · 步骤 / 触发 | UC 说谁给这件事实 | 建议处置 | 一句话理由 |
|---|---|---|---|---|---|
| 1 | `RegisterTransportHandover`（含 `Correct`） | UC-TF-005 步骤 1「运输接入：保全交接请求」 | 节点侧交出证据 / 运输方接收证据（伙伴接入） | **外部端点，第一优先** | 它是 CONTEXT 成立边界的主入口；进段随它走。命令必须带 `Segment`/`PlannedSegment`，端点测试专钉「不收那一格就是票 08 那个缝」 |
| 2 | `RegisterOffsitePickup` | UC-TF-002 单对象揽收登记 | 伙伴回传 | **外部端点** | 与 1 同为控制事实入口；进段随它走 |
| 3 | `PerformOffsitePickup` | UC-TF-002 一次到访多对象 | 自营实际执行方 | **外部端点**（与 2 同族，可同一端点表分组） | 同上；多对象逐个成败并存是它与 2 的全部差别 |
| 4 | `RecordMovementFact` | UC-TF-005 步骤 6「执行方/伙伴接入：接收出发、移动、到达、中断」 | 自营执行方 / 外部伙伴 | **外部端点（自营执行方）**；外部轨迹**不走它**，走 `TrackingSource` 入站口的采纳执行器（label-channel/15 已落端口、16 draft） | 两条来源两个口，别让 HTTP 端点顺手替轨迹采纳做判断 |
| 5 | `EndFulfillmentParticipation` | UC-TF-005 步骤 7「形成下一次交接或控制结束 → 结束相应对象参与关系」 | `transport-fulfillment` 自己 | **内部触发**：交付一路挂在 `RegisterEffectiveDelivery` 落库之后、下一次交接一路挂在 `RegisterTransportHandover` 落库之后；**终止一路走 admin 写面**（它指向本上下文之外的处置决定） | UC 把步骤 7 的责任方写成 TF 自己，不是接入方；不该有外部端点 |
| 6 | `CloseFulfillmentSegment` | CONTEXT 生命周期④（UC-TF-005 未单列步骤） | 运营决定 | **admin 写面**（沿 ADR-0085 登记写面进端点表的形状） | 「不再接受新对象」是决定，由人做；不是承运方回传口 |
| 7 | `OpenDispatchTask` | UC-TF-006 触发「尾程实际履约段到达派送范围，**或授权角色建立派送任务**」 | TF 自己 / 授权角色 | **内部触发**（到达事实）+ **admin 写面**（授权角色）；无外部端点 | UC 的两个触发源都不是外部接入 |
| 8 | `FormLoadAssignment` | UC-TF-003/004 运输准备 | 运输运营 | **admin 写面**（运营决定分配）；不是内部触发 | 核过：`PrepareTransportOpportunity.Consume` 收的是**调用方给的**分配引用、消耗后才把它交给装载分配链——分配本体在消耗之前、由人定，方向与「由 Prepare 触发分配」相反 |
| 9 | `RegisterFailedAttemptCharge` | UC-TF-002 失败尝试费（`AT-TF-094`） | — | **暂不接**，阻塞于 ADR-0098 决定四 | 触发链要的采购上下文（揽收↔委托连线）ADR 裁了今天不建；机制齐备、触发链未接通是 ADR 明记的后果，不是本票能补的 |

**读表的两点**：
- 外部端点只有 1–4 四条，且全是**控制事实与移动事实的来源入口**；5–7 是 TF 自己的派生与运营决定，
  不该长出承运方回传口。把 5–7 做成端点会让 UC 步骤 7 的责任方从 TF 变成接入方。
- 1–3 接上之前，进段那道门（`enterFulfillmentSegment`）在生产上没有任何路走到——**这三条是本表
  的关键路径**，与 tf-unwired-seven 里票 01 是关键路径同一个理由。

**接线机制不另发明**：HTTP 适配器进 `adapters/http/`、端点表 `cmd/parcel-api/endpoints.go` 加行、
装配点交入、Intake 按 ADR-0063 显式未配置形状先立——沿 `parcel-api-remaining-endpoint-wiring` 票 02
（TF 交付端点）那一笔的形状；admin 写面沿 ADR-0085。**结果代数 → ADR-0022 响应格**每条编排都要映一遍，
`未决` → 5xx `NO_ANSWER_FORMED` 有先例。

## 要 owner 答的

逐行答上表「建议处置」列：同意 / 改为 X。另加一问：1–4 按 UC 分端点表还是按事实分（建议按事实：
交接、揽收、移动三组，交接与揽收各自带更正口）。

答完拆票：预计 1–3 一张（控制事实入口，含进段带段引用的端点测试）、4 一张、5 一张（两处内部触发）、
6+7+8 一张（admin 写面三格：关段、建派送任务、装载分配）、9 不立票。本票届时转 resolved 作为索引。

## 边界

本票在裁定前不写代码。`cmd/parcel-api/` 是共享接线文件最凶的一处（`endpoints.go`、
`unwired_orchestration.go`、`main.go`），开工前按 `docs/agents/parallel-sessions.md` 占号。
