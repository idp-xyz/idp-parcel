# 18 批文译装下沉后 `/commercial-publications` 换操作者 Intake

Category: enhancement
Status: in-progress——2026-10-13 22:06 通道 3 认领（派单本卡 ← 通道 1，`task-42392427`），分支 `mcp3-oc18`，基 `b3c8bd46`（开工前 `ls-remote` 核远端 main 仍是它）。此前 ready-for-agent——2026-10-12 20:2x 通道 1 立（派单方裁定），出自 [06](./06-commercial-publication-and-replay-take-operator-identity.md) 完成记录判断项 1：06 把发布路径四口、词表读口与计价回放口换上了操作者渠道，`/commercial-publications` 留在字面量 `UnconfiguredIntake{}` 上，作者报「另立下沉票，或扩本票地盘」待裁，派单方裁另立，即本票。
Blocked by: 06（本票接在 06 的 `OperatorRegistryIntake` 与认证适配器交回的 `OperatorIdentity` 上；06 进 main 之前不开工）
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`cmd/parcel-commercial` 里把受控批文译成 party-commercial 命令的那一段（下沉出去、受控 CLI 改调下沉后的那一份）；下沉的落点在 party-commercial 的适配器包里，照 [04](./04-registration-write-faces-take-operator-intake.md) 第三、五批「译装先下沉成 `registrationjson`、在线口不另写平行解码」的先例定；`cmd/parcel-api` 端点表里 `/commercial-publications` 那一行与它的装配测试。
出处：06 判断项 1；[ADR-0126](../../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 决定五；[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四。

## 做什么

1. 批文译装下沉：受控 CLI 与在线口共用同一份译装，不另写平行解码；受控 CLI 对同一份批文的答复不变。
2. `/commercial-publications` 换操作者 Intake（挂 `operatorRegistries.commercial`）。这一口收的是受控批文的在线镜像（声明摘要 + 批准三格 + 角色确认），批准照旧从批文取、不从信封来——操作者的批准路径是 ADR-0126 决定三的待批准载体，06 已换（06 判断项 1 原话，该判断项标着待 owner 复核，复核改判就照改判做）。
3. 三格测试同 06 的回放口：未带令牌 401、授予不足 403、操作者册读不动 503；授予齐备而载荷自报租户 400；发行方未配置仍答 403 未配置。

## 不做

- 发布路径四口与词表读口（06 已换）。
- 审批职责规则的取值与登记入口；角色模型里有哪些授予格（[psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)）。

## 完成判据

- 下沉前后受控 CLI 对同一份批文的答复相同（带测试）。
- `/commercial-publications` 三格各答其格，授予齐备时答业务结果（带测试，`cmd/parcel-api` 装配测试带 DSN）。
