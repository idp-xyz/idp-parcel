# PS←PG 生产归属桥已实现却零装配，且接它之前有一件机制半边的裁决没做

Category: enhancement
Status: needs-triage

由[票 02](./02-production-ownership-authority-has-no-adapter.md) 收口后分出。票 02 的「缺的最小
机制件」第 1 项（桥接适配器）已由 `57e0b1f` / `c0ea050` 交付并转 `resolved`——**本票不重开它**，
本票问的是那之后剩下的一格：**桥造好了，谁来接。**

发现路径值得记一句：它是[生产接线棘轮门禁票](../../production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)
的取证表在 MCP-5 离线后被回头复核时撞出来的——**票 02 转 `resolved` 那一刻，这一格就没人再看了**。
那张门禁票的第四族（ports 适配器）正是为自动盯住这一类而加的。

## 现状（取证于 `f6ed413`，只读）

- **桥在**：`internal/parcelshipment/adapters/pilotgovernance/production_ownership.go` 的
  `ProductionOwnershipAdapter` 已实现 `psports.ProductionOwnershipAuthority`（编译期断言
  `var _ psports.ProductionOwnershipAuthority = (*ProductionOwnershipAdapter)(nil)` 在）。
- **零装配**：`NewProductionOwnershipAdapter` 在全仓非测试代码里**唯一一次出现就是它自己的声明**；
  `cmd/` 下对 `pilotgovernance` **零引用**。
- **它唯一的消费者是提交编排**：`SubmitShipmentRequestHandler` 的第三个入参就是
  `ports.ProductionOwnershipAuthority`（`DecideProductionOwnership` 在 `Handle` 的第二步）。
- **提交编排本身也零装配**：`NewSubmitShipmentRequestHandler` 无任何生产构造点。

## 要答的第一问：今天到底有没有一个装配点可接

**有，而且只有一个**：`cmd/parcel-api/endpoints.go` 里 `/shipment-requests` 那一行的第二参。
它今天填 `unwiredSubmission{}`，那是一个**显式命名的诚实占位**，不是遗漏——
`unwired_orchestration.go` 的文件注释把理由写死了：

> 各端点构造函数的第二参是应用编排；**未配置 Intake 在它之前就拒了**，因此本文件这几个类型一个
> 都到不了。它们存在只为回答「到不了的那一格填什么」。……真渠道 Intake 就位那笔工作在装配点把
> 它们换成真编排，**与替换 Intake 同一处、同一行**。

## 按 ADR-0017 分阻断理由的性质——三件，性质各不相同

**不要写成「阻塞在票 01 之后」。** 逐件分完之后那句话不成立：

| 要件 | 性质 | 今天能不能动 |
|---|---|---|
| 装配点存在 | —— | **在**，被诚实占位占着 |
| `ProductionOwnershipAdapterDeps.AnswerValidity` 必须由装配方说出 | **机制半边的一次裁决** | **今天就能裁，不等租户** |
| `ProductionOwnershipAdapterDeps.SelfAuthority` | 实例半边 | 留空即诚实答`权威未确定`，**不阻塞** |
| 真渠道 Intake（票 01） | 混合（登记册属机制、凭据属实例、身份来源形状属裁决） | **不阻塞装配**，见下 |

**第二行是本票的实质内容，也是唯一真正没做的一件。** `NewProductionOwnershipAdapter` 对
`AnswerValidity <= 0` **构造期硬拒**（`ErrAnswerValidityNotStated`），而该字段的注释写明了为什么
不能随手补一个：

> 它由装配方说出，本包不挑一个数：`ProductionOwnershipDecision` 要求有效期间必须存在且包含判断
> 时点，而治理登记册的开放区间（`to_at` 为 NULL）根本没有终点，**随手补一个时长就是发明默认值**。

所以「一份归属答复可被信任多久」是**一次裁决**，不是一个待租户提供的参数——它问的是产品行为，
不是租户取值。**这一件今天就可以裁，且它不依赖票 01 的任何一半。**

**为什么真 Intake 不阻塞装配**：Intake 在运行期拒在编排之前（上引注释原话），所以「装不装真编排」
与「Intake 真不真」在**装配期互不影响**。今天把真编排接上去，运行期照样一封都进不来——但那一格
从此是真的，而不是一个占位。这与 MCP-4 票 10-B 的做法同形：**用一个显式命名的未配置来源接上去，
比不接强，因为它把恢复动作摆到了台面上。**

## 要答的第二问：接上去之后，答案会变吗

**不会，且这正是要的。** 无租户时治理登记册为空，桥答`权威未确定`（`AUTHORITY_UNRESOLVED`），
提交停在 `OWNERSHIP_UNRESOLVED`。**与今天的区别不在结果，在结果的来源**：今天是「这一格根本没接
东西」，接上之后是「接了真桥、真读了登记册、册里没有」。前者的恢复动作是「写代码」，后者是
「登记一条权威区间」——**ADR-0063 那一整套「显式未配置」讲的就是这个差别**。

## 不在本票内

- **不重开票 02。** 桥的实现已验收，本票只管接线与那一件裁决。
- **不裁 `AnswerValidity` 取什么值**——本票只指出它是一次待裁的机制半边裁决、且卡住了接线。
  取值归领域 owner；若该裁决被判为难逆转，按 AGENTS.md 走 ADR。
- **不改票 01 的范围**，也不主张本票必须等它。
- 不碰 `internal/pilotgovernance/**` 与 `internal/parcelshipment/adapters/pilotgovernance/**`
  的既有实现（MCP-5 离线后该地盘无主，本票只读它）。

## 参照

票 02；ADR-0017（按阻断理由的性质分别裁决）；ADR-0063（显式未配置）；
`cmd/parcel-api/unwired_orchestration.go` 的文件注释；
[棘轮门禁票](../../production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)第四族。
