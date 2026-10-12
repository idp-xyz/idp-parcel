# 10 设备登记与「作业事实登记」能力面：一线作业事实走操作者渠道族

Category: enhancement
Status: in-progress——2026-10-12 14:5x 通道 1 接手认领：通道 1 派单 task-caef2c2f 给通道 4，其新会话列完计划即 crash（用户报；14:52 实测工作树 `/home/tops/workspace/idp-parcel-mcp4-oc10b` 与分支 `mcp4-oc10b` 已建、停在 `ff3b01fa`、零提交零改动），用户令通道 1 接手。分支 `mcp4-oc10b`、基 `ff3b01fa`；从 `mcp4-oc10@382c4efb` 接着做（通道 4 前一会话的未提交现场原样封存，非集成候选），先写自己第一片 red 再对照取用。本轮只做机制半边——做什么第 1–3 条与完成判据第一条；第 4 条更正口与完成判据第二条「换口同笔撤隔离放行」与 [04](./04-registration-write-faces-take-operator-intake.md) 第 1 件同一前提（演示环境接上发行方与合成操作者授予），等 [07](./07-admin-web-login-gate.md)。此前 in-progress——2026-10-11 01:00 通道 4 认领（`2d90f548`，只在分支 `mcp4-oc10` 上，基 `2603f9ad`）。此前 ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）
Blocked by: 01、03
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：`internal/accessidentity`（设备册、能力面授予格、铸造里「操作者主体 + 设备」两件）与其迁移、受控登记 CLI、参数登记册增「作业设备」一行；作业事实命令口的装配留给 13、14 齐了之后逐口换。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定二；[ADR-0023](../../../docs/adr/0023-work-fact-identity-and-time-are-minted-by-the-device.md)。

## 做什么

1. 操作者册加能力面「作业事实登记」，授予按租户 × 作业范围（节点、场外作业范围）。
2. 设备册：设备标识、所属租户与作业范围、状态（登记 / 停用），受控 CLI 登记；参数登记册增「作业设备」一行（租户取值）。
3. 铸造：令牌有效 + 在册有授予 + 设备已登记且在用，才铸带「操作者主体 + 设备」的信封；答复另加「设备未登记或已停用」一格。
4. 主链事实的更正口随原事实归本能力面（2026-09-24 随 [ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定四补）：`/transport-fulfillment/handover-corrections`、`/transport-fulfillment/offsite-pickup-corrections`、`/transport-fulfillment/delivery-proof-corrections` 与首登口同一份认证与授予。
5. 关段 `/transport-fulfillment-segment-closures`、手工建派送任务 `/transport-fulfillment-dispatch-task-registrations`、有效时间判断 `/transport-fulfillment-effective-time-judgments` 与承运商首次有效收寄判断 `/transport-fulfillment-carrier-first-effective-pickup-judgments` 不归本能力面（2026-09-25 随 ADR-0151 决定六改，两个判断口同日补）：它们是管理台上的运营写决定，归 [15](./15-operation-decision-faces-take-operator-intake.md) 的「运营决定」能力面；派送发起口照旧在本能力面的范围内。

## 完成判据

- 四格答复各有用例；设备停用后同一操作者在该设备上被拒、换已登记设备照常；事实身份与发生时间照旧由请求携带、服务端不重签。
- 随 [ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三（2026-09-24 补）：换上作业事实能力面的各口（今天经写开关放行的节点收寄、揽收、交接、移动、派送、交付、派送尝试登记等）同一笔撤下隔离放行，隔离放行用例改写为真渠道答复格用例。
