# 现场作业事实 → 既有命令用例 映射表

Category: chore
Status: 取证于 `8acacfe`（2026-09-02，MCP-5）；[ADR-0089](../../docs/adr/0089-frontline-transition-controlled-import-with-structural-sunset.md) 已于 `32b78d7` 落文，决定以 ADR 为准，本表只出事实与缺口

本表回答一件事：过渡期内勤汇总的四类现场作业事实，各自能不能灌进**既有**命令用例、缺哪个输入。对不上的如实记缺口，不造用例、不改 domain。受控导入口是 [`cmd/parcel-frontline-import`](../../cmd/parcel-frontline-import/main.go)。

## 四类事实

| 现场事实 | 目标上下文 | 既有命令用例 | 判定 | 说明 |
|---|---|---|---|---|
| 收货 / 收寄（客户送站） | `node-operations` | `ReceiveDeliveredUnitHandler.Handle`（[UC-NO-002](../../docs/application/node-operations/UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md)） | **对上，运行期被挡** | 输入全部对得上（见下节）。但 `ports.ParcelIdentityView` 全仓无生产实现（[ps-external-mark-relations/01](../ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md)），`receive` 在身份核对报错时答 `RECEPTION_UNDECIDED` 且**不落库**——`RECEIVED` 行今天一律未决。`REFUSED` / `SCAN_ONLY` 两支不经身份核对，可落库 |
| 扫码换单（收货后二次取尾程单号、贴新标签） | `parcel-shipment`（面单交易）/ `node-operations`（标签观察） | 无 | **缺口** | 面单交易只有 domain 聚合（ADR-0084），`internal/parcelshipment/application` 无「收寄后换单」命令用例；`node-operations` 的「新旧标签观察」无模型无用例 |
| 装箱封签 | `node-operations` | `ConsolidateParcelsHandler` `Open` / `AddMember` / `Seal`（[UC-NO-003](../../docs/application/node-operations/UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md)） | **对上，来源标记无落点** | 输入对得上（单元实例、载具、成员、封签引用、作业依据、封装时间）。但三口**没有任何来源 / 记录方表达**——无来源身份、无证据引用、无执行方；UC-NO-003 结果契约要求「作业事实已形成」保存「执行方、来源和证据」，实现的 `ConsolidationUnit` 是纯状态机。要让导入封签与设备封签可区分，只剩「前缀塞进 `WorkBasisReference` / `ConsolidationUnitID`」（语义拉伸）或「给集运聚合加来源事实层」（domain 改动、ADR 级）两条。**已裁**：前者否决（读的人会把它当作业依据），本期**不建集运子命令**，缺口另立票 [no-consolidation-fact-provenance/01](../no-consolidation-fact-provenance/issues/01-consolidation-commands-carry-no-source-executor-evidence-or-business-time.md) |
| 称重实测 | `node-operations` | 无 | **缺口** | 「实际测量」在存储上没有登记册、无用例；`ports/catalogue_read.go` 注释明说「实际测量与节点侧交接证据两区在存储上还没有登记册」 |

票面四类之外有用例可接、但本票不导入的：交出给运输方 → `transport-fulfillment` `RegisterTransportHandoverHandler.Register`（UC-TF-005）。它的输入带 `ReleasingEvidence` / `ReceivingEvidence` 两个证据引用，来源标记有天然落点；要不要纳入过渡模板归 MCP-1 定，不在本票顺手加。

## 收寄：输入逐格

`ReceiveDeliveredUnitCommand` 的每一格从模板哪一列来、来源标记落在哪：

| 命令字段 | 模板列 | 备注 |
|---|---|---|
| `TenantID` | CLI `-tenant` | 租户是最高隔离边界（ADR-0003），由受控运维给出，不让内勤填 |
| `SourceID` | 由 `templateVersion` + `batchRef` + `factRef` 拼成 `FTI/INTAKE-1/<batchRef>/<factRef>` | **来源标记落点一**。身份与录入者无关：同一张纸单被两个内勤各录一次是重放不是两条事实。`FTI/` 前缀使「身份与时间非设备铸造」这一点在 `source_id` 上直接可辨（对照 ADR-0023） |
| `Node` | `node` | 节点标识属实例半边，模板不给默认 |
| `DeliveredBy` | `deliveredBy` | 客户或授权交付方 |
| `Unit` | `handlingUnit` | 现场能区分的作业实物号 |
| `Mark.Mark` | `mark` | 外部标识观察，只是线索 |
| `Claim` | `claim` ∈ `RECEIVED` / `REFUSED` / `SCAN_ONLY` | 三态是观察不是判断 |
| `Evidence` | 由 `templateVersion` + `operator` + `evidenceRef` 拼成 `FTI/INTAKE-1/<operator>/<evidenceRef>` | **来源标记落点二**。`ReceptionEvidenceReference` 的注释本就点名「受控接收记录」——过渡模板行就是那份记录，录入者是记录方。只有 `RECEIVED` 支消费此格 |
| `Refusal` | `refusalReason` | 只 `REFUSED` 支 |
| `ServiceMarkers` | 不填 | 那是 PS 服务结果引用，不是来源标记的地方 |
| `OccurredAt` | `occurredAt` | 内勤抄自纸单的业务时间；CLI 不用当前时刻顶替 |

**已知限制**：`REFUSED` / `SCAN_ONLY` 两类行的记录不带证据引用，录入操作者因此不在库内记录上，只在批次文件与 CLI 打印的导入清单里可追溯。这是既有事实形状决定的，不在本票补字段。

## 集运：输入逐格（已裁本期不建，留作解阻后的备料）

| 用例入口 | 模板将来的列 | 来源标记 |
|---|---|---|
| `Open(tenant, unitID, asset)` | `unitRef`、`assetRef` | 无落点 |
| `AddMember(tenant, unitID, member)` | `memberUnit` | 无落点 |
| `Seal(tenant, unitID, seal, basis)` | `sealRef`、`basisRef`、`sealedAt`（封装时间取 `Clock.Now()`，模板给的时间进不去） | `basis` 是作业依据不是记录方；无落点 |

`Seal` 的封装时间由 handler 取 `Clock.Now()`，模板抄的现场封装时间没有入口——这是第二处与导入相抵的形状，与来源标记那处同归 `no-consolidation-fact-provenance/01`（该票「要做的」第 1 条把业务时间与来源身份一并要求由调用方带入）。
