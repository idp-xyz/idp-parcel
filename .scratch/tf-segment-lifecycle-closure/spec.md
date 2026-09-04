# 实际履约段的生命周期收口：关段声明口、承运商判断、端点接线

Category: enhancement
Status: in-progress——01、02、03、04、05、06、07 resolved（02 于 2026-09-04 入 main：代码 tip `56d2a437`、票面 `fe5b3a35`、`cmd/parcel-api` 装配行 `9e8a9202`；ADR-0103）；08 resolved（裁决 `bca02e5`；MCP-3 2026-09-04 于分支 `mcp3-tf08` 完工，MCP-1 重放入 main `91fab19d`..`27116c52`，TF 迁移 `0015` 换 `offsite_pickup` 主键纳入版本、更正版 handoff ID 加版本段、端点 `/transport-fulfillment/offsite-pickup-corrections`；PS 采用口把更正版当第二责任起点的缺口立 `label-channel/24`，段侧重派生立本目录 `10`，均 draft）；09 draft（裁决 `21ee4f0` 把它拆成派送段声明建模 + 三条输入缝 + 触发执行器，开工前置一次 `/domain-modeling`——票面为准，此前状态行写的 ready-for-agent 过期）；10 draft（来源更正 → 参与关系重派生，覆盖交接与揽收两种来源）；06 「补刀二」裁「采」（`3f3a675`，封存笔 `mcp4-tf03@44808f3`）已由 MCP-1 于 2026-09-04 在 main 上重放为 `4cbe5266`（与 tf/02 的 `enterFulfillmentSegment` 挂点自动合并无冲突）

## 从哪里分出来

[`tf-unwired-seven`](../tf-unwired-seven/spec.md) 八票 2026-09-03 全 resolved，棘轮
`transport-fulfillment` 组清空。**但该 spec 完工判据一节自己写了：名单清空只判名单清空，判不了
都接上了。** 收口审查（`0c8d65d` 后，MCP-1）把它没覆盖、票面也如实标为「未做 / 不在本票」的三件
收进这里，免得它们只活在一条已 resolved 票的评论里。

三件分量不同、要的裁决也不同，所以三票分开，不并成一张「TF 收尾」。

## 三票

| 票 | 内容 | 体量 | 要什么才能开工 |
|---|---|---|---|
| [01](issues/01-close-segment-declaration-entry.md) | 关段声明口：`CloseFulfillmentSegment` 编排 | 小 | 不要裁决——领域、端口、适配器三层都在（ADR-0097 第三个窄口），缺的只是编排 |
| [02](issues/02-actual-carrier-judgment-model.md) | 实际承运商判断（CONTEXT 词条 + 「不追溯覆盖未知期间」） | 大 | 先 `/domain-modeling`：它是新模型不是参与关系上的一格 |
| [03](issues/03-parcel-api-wiring-for-segment-orchestrations.md) | `cmd/parcel-api` 对本批 TF 新编排的装配与端点 | 大 | owner 裁：哪些编排要端点、走哪条 UC、Intake 要什么 |

无阻塞边：三票互不依赖。**01 与 03 之间有一条先后偏好不是阻塞**——03 若开，把 01 的编排一起
挂上比事后再补一格便宜。

### 03 裁后拆出的实施票（2026-09-03，MCP-3 裁、MCP-4 拆）

| 票 | 内容 | 开工前置 |
|---|---|---|
| [04](issues/04-control-fact-entry-endpoints.md) | 控制事实入口：交接（登记 + 更正）、揽收（登记 + 执行）四个端点、装配、进段带段引用 | 无；关键路径先做 |
| [05](issues/05-movement-fact-endpoint.md) | 移动事实端点，只收自营执行方 | 无 |
| [06](issues/06-participation-end-internal-triggers.md) | 结束参与的两处内部触发（交付后、交接后） | MCP-1 对动 TF application 的答复 |
| [07](issues/07-admin-write-faces-segment-closure-dispatch-task-load-assignment.md) | admin 写面四格：关段、建派送任务、装载分配、明确终止参与 | 无 |
| [08](issues/08-offsite-pickup-correction-model.md) | 揽收登记的更正：已裁取 A 新版本（`Corrects` 回指，登记册只插不改） | 已裁（`bca02e5`），ready-for-agent |
| [09](issues/09-arrival-triggers-dispatch-task.md) | 到达事实触发建立派送任务：四问已裁，触发事实改为对象凭`已交接`进入派送段 | 已裁（`21ee4f0`），ready-for-agent；05 已 resolved 不再阻塞 |

04、05、06、07 已 resolved（2026-09-03）；02 已 resolved（2026-09-04，MCP-2，ADR-0103）。01 的编排（`CloseFulfillmentSegment`）随 07 挂上，正是上面那条先后偏好说的便宜路。

## 红线（沿 tf-unwired-seven，不复述其正文）

- 不改现有编排的导出签名；不改已施加的迁移。
- 实例取值留空并拒绝默认值。
- CONTEXT 分立纪律不得合并；每票走 TDD。
- **不自动关段。** 票 tf-unwired-seven/07 的那条裁定在本 spec 仍然成立：CONTEXT 生命周期④的
  「不再接受新对象」是一个决定不是可推导状态，01 建的是让人做那个决定的口，不是替人做。

## 完工判据

01：段可经编排关闭，关闭后新对象经生产进段路径进不了该段，仍有在场参与的段关不上；三条各有
测试钉住。**本票没有棘轮条目要剪**——`CloseSegment` 是聚合方法，本就不在网内。

02、03：各自票面写；转 ready-for-agent 之前先把「要什么裁决」那一格填上答案。
