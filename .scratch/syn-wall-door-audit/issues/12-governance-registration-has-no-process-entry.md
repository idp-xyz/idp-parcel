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

## 为什么这不只是接一根线

票 02 收口时点明了：端点那条路已经堵死，只剩受控 CLI，或者本票另裁第三条。

- **端点路（今天走不通）**：`cmd/parcel-api/endpoints.go` 里业务端点全部装
  `UnconfiguredIntake{}`——按 [ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)
  它「不读业务内容、不采信任何自报身份、不构造命令，一律如实答『接入渠道未配置』」。
  无接入渠道即无可认证入口；而接入渠道登记册（票 01）已撞上 ADR-0055 明文否决的替代方案，
  转 `ready-for-human` 等新 ADR。**本票因此阻塞在票 01 之后**，除非裁定治理登记不走业务端点面。
- **受控 CLI 路（今天开着）**：不经 HTTP 接入面，因而不受票 01 阻塞。

## 要裁的

1. **治理登记算不算「业务端点面」。** ADR-0055 治的是客户委托与外部结果那一类接入；治理登记
   是运营方对自己试点的登记，不是客户送进来的委托。两者若属不同面，本票就不必等票 01。
   **这一格该由领域/架构拍，不该由实现票默认选一个**——选错的后果是要么白等一张 ADR，
   要么在没有认证方案的地方开一个口子。
2. 若走受控 CLI：身份从哪里来。CLI 的执行者身份不能由参数自报——
   [ADR-0022](../../../docs/adr/0022-http-status-carries-answer-formed-not-business-verdict.md)
   写明「采信客户自报的租户号会穿透 [ADR-0003](../../../docs/adr/0003-group-tenant-legal-entity-customer-account.md)
   的最高数据隔离边界」，同一条理由对治理登记同样成立：`SuspensionDecision.ExecutedBy` 与
   `ResumptionDecision.DecidedBy` 记的是「谁决定的」，自报即等于让登记册自证。
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
