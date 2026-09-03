# 实际履约段的生命周期收口：关段声明口、承运商判断、端点接线

Category: enhancement
Status: in-progress——01 resolved（`2b4f6d7` + `722e846`）；03 resolved 作索引（MCP-3 2026-09-03 裁决，拆出 04–07 全部 ready-for-agent）；02 draft，需一次 `/domain-modeling`

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

01 的编排（`CloseFulfillmentSegment`）随 07 挂上，正是上面那条先后偏好说的便宜路。

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
