# 02 外部标识关联册落地，客户包裹件号随委托接受形成，节点身份核对接真

Category: enhancement
Status: ready-for-agent——2026-10-16 Claude Code 会话立（[ADR-0184](../../../docs/adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 决定一至六）
Blocked by: 无
父票：[01](./01-external-mark-relations-have-no-model-in-parcel-shipment.md)

## 做什么

1. **PS 领域**：外部标识关联的值类型——号码、标识类型（本票只有客户包裹件号一类有来源；渠道包裹件号随 03 进）、分配方、被标识对象、适用范围、形成依据——与关联本身；委托上的（声明包裹，客户包裹件号）配对，每件至多一个号，指着成员集合外即立不起来；已接受委托交出它该形成的关联（形成依据为接受决定）。
2. **提交路径**：`SubmitShipmentRequestCommand` 带配对；`IsolatedSubmissionIntake` 按逐件 `customerParcelReference` 与现签内部标识一一配上。配对立不起来答`输入未受理`。快照存、重建带回；新提交版本不重报。
3. **PS 持久化**：迁移建关联表与适用变化表、按（租户，号码）查的索引；`ShipmentRequests.Save` 保存已接受委托时与快照同一条事务写入它的关联，按键幂等。
4. **核对读口**：PS 端口按（租户，号码）答此刻适用、标识包裹的关联所指的包裹，逐字比对、去重；真库实现。
5. **NO 防腐适配器**：`internal/nodeoperations/adapters/parcelshipment` 实现 NO 的 `ParcelIdentityView`，包裹译成 `ParcelAssociationReference`；空号码答零个候选。
6. **装配**：`cmd/parcel-api` 与 `cmd/parcel-frontline-import` 换掉 `unconfiguredParcelIdentityView`，两处未配置替身与前线导入开工前那句提示一并退场。

## 不做

- 渠道包裹件号、作废与替代（03）。
- 人工登记、更正、作废口；查阅读面；客户参考号与渠道交易级号码两类（ADR-0184 决定二）。
- 「身份后来被确认」的补关联（`AT-NO-026`）。

## 完成判据

- 委托接受后，客户包裹件号逐字在节点识别出那一件包裹：收寄形成、控制建立，票 operator-channel/25 的服务结果读口随之被问。
- 接受前、已拒绝、已撤回的委托，其号码在节点答零个候选。
- 两件已接受包裹用了同一个号（同客户或跨客户）→ 两个候选 → 身份冲突。
- 已接受委托反复保存不重复形成关联；关联表只插不改。
- 提交时配对指着成员集合外、或一件两个号 → `输入未受理`。
- 真库用例：经生产仓储走「提交 → 接受 → 按号码核对」一条；`cmd/parcel-api` 装配的收寄编排对一件已接受包裹答`节点收寄已形成`。
- 架构门禁绿；机制清点重生成。
