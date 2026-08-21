# 治理登记没有进程级入口，PAR-GOV-03..07 的实例登记无路可走

Category: enhancement
Status: needs-triage

来源：票 02「缺的最小机制件」第 2 条，按 MCP-1 指示自票 02 拆出独立成票
（`.scratch/syn-wall-door-audit/issues/02-production-ownership-authority-has-no-adapter.md`）。
票 02 已交第 1 条（PS←PG 桥接适配器）并收口，第 2 条从头到尾未做，理由是地盘而非判断——
它按定义要落 `cmd`，而票 02 的派单明令不碰 `cmd/parcel-dispatch/assemble.go` 与
`cmd/parcel-api/endpoints.go`。

## 缺什么

治理侧的登记机制半边是齐的：`authority_interval`、暂停/恢复/接管四张表在
（`migrations/pilot_governance/0001_governance_records.sql` 与
`0003_suspension_resumption_takeover.sql`），INSERT 写入方在（`governance_records.go`、
`incident_records.go`），登记用例在（`record_stage_review.go`、`govern_incident.go`）。

**缺的只是「人怎么把一条登记送进去」。** 复核于 `d5e5d20`：`cmd` 全树 `pilotgovernance`
引用为零。于是 PAR-GOV-03..07 的实例登记今天没有任何进程级路径——权威区间、阶段评审、
暂停、恢复、接管五类记录都只能由测试写入。

## 阻断理由分辨（按 [ADR-0017](../../../docs/adr/0017-admission-gates-judged-by-blocking-cause.md)）

ADR-0017 的方法是**先分辨阻断理由属机制半边、实例半边还是外部依赖，再决定它约束什么**，
并点名了两种失误方向，其中一种正是「把机制半边误判为实例半边而继续停滞」。本票三样理由
此前被「端点路堵死」一句话掩在同一格里，逐条分开如下。

| 阻断理由 | 性质 | 约束什么 |
|---|---|---|
| 入口本身的形状：命令、调用哪个登记用例、执行者身份怎么被断言 | **机制半边** | **不阻断**，现在就能做 |
| 真实授权角色名、阶段证据清单、接入渠道参数（PAR-GOV-*、PAR-INT-01） | **实例半边** | 阻断，留空不给默认值，不预设岗位名称 |
| 票 01 在等的那张新 ADR（接入渠道登记册的替代方案已被 ADR-0055 明文否决） | **一项未作的决定**，既非机制亦非实例 | 只阻断**端点路**，不阻断受控 CLI 路 |

按这个分辨，**本票不是整体阻塞在票 01 之后**——票 02 收口时那句「第二件只剩受控 CLI 或
另拆票两条路」说的是端点路，不是全部。分路看：

- **端点路（被第三行阻断）**：`cmd/parcel-api/endpoints.go` 里业务端点全部装
  `UnconfiguredIntake{}`——按 [ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)
  它「不读业务内容、不采信任何自报身份、不构造命令，一律如实答『接入渠道未配置』」。
  无接入渠道即无可认证入口，而票 01 已转 `ready-for-human` 等新 ADR。
- **受控 CLI 路（第三行不适用）**：不经 HTTP 接入面。它的机制半边按上表第一行现在就放行；
  卡住它的只有下面「要裁的」第 2 条那个身份问题，而那是一次裁决不是一份参数。

## 要裁的

1. **治理登记算不算「业务端点面」。** ADR-0055 治的是客户委托与外部结果那一类接入；治理登记
   是运营方对自己试点的登记，不是客户送进来的委托。两者若属不同面，端点路那一行也不适用，
   本票整条都不必等票 01。**这一格该由领域/架构拍，不该由实现票默认选一个**——选错的后果
   不对称：要么白等一张 ADR，要么在没有认证方案的地方开一个口子。
2. **若走受控 CLI：执行者身份从哪里来。** 这是**一次裁决，不是一份待提供的参数**——按上表
   它不落在实例半边那一行，所以不能拿「等真实运营方案」把它推掉。CLI 的执行者身份不能由参数
   自报：[ADR-0022](../../../docs/adr/0022-http-status-carries-answer-formed-not-business-verdict.md)
   写明「采信客户自报的租户号会穿透 [ADR-0003](../../../docs/adr/0003-group-tenant-legal-entity-customer-account.md)
   的最高数据隔离边界」，同一条理由对治理登记同样成立：`SuspensionDecision.ExecutedBy` 与
   `ResumptionDecision.DecidedBy` 记的是「谁决定的」，自报即等于让登记册自证。
   要裁的是身份的**来源形状**（宿主机操作者凭证、独立的运维身份登记、还是别的），
   角色的**取值**才是实例半边、才留空。
3. 五类记录是否都要入口，还是先只开其中一两类（如暂停/恢复这一对最可能被真实运营用到）。

## 红线

- **不得自造采信自报身份的口子。** 见上，理由是 ADR-0022→ADR-0003 那条链。
- 治理记录不可覆盖——入口只能追加，不得提供任何改写既有登记的路径（既有票已钉，
  `Save` 一律 `ON CONFLICT DO NOTHING`，入口不得绕过它）。
- 恢复准入必须四件齐备（原因解除证据、一致性核对、在途盘点、试点业务责任角色决定），
  入口不得提供「只填标识就恢复」的简化路径。
- 实例半边照旧留空：入口是机制半边，现在就能做；真实角色名、授权方案与证据清单等待
  真实运营方案确认，不预设岗位名称。

## 参照

`docs/product/PILOT-PARAMETER-REGISTER.md` PAR-GOV-03..07；ADR-0055、ADR-0022、ADR-0003；
票 01（接入渠道登记册，`ready-for-human`）；票 02（第 1 条已交付并收口）。

## Comments

- 2026-08-21 MCP-5：按 MCP-1 指示自票 02 拆出，未做任何实现。拆出时复核于 `d5e5d20`：
  `cmd` 全树 `pilotgovernance` 引用仍为零，票 02 收口时的那句预言成立。本票不阻塞票 02。

- 2026-08-21 MCP-5（按 MCP-1 要求补 ADR-0017 分辨，**结论与初稿不同，记下差别**）：初稿把
  三样阻断理由并成「端点路堵死 → 本票阻塞在票 01 之后」一句。按 ADR-0017 逐条分性质之后
  结论变了：入口的形状属机制半边、现在就放行，票 01 那张未作的决定只挡端点路，**受控 CLI
  路今天并不被票 01 阻塞**。初稿那句正是 ADR-0017 在 Consequences 里点名的失误之一——
  「把机制半边误判为实例半边而继续停滞」。分辨表已补进正文，`Blocked by` 因此不写票 01。
