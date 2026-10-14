# 21 作业设备册查阅页：只看已登记的行

Category: enhancement
Status: 2026-10-14 15:13 通道 2 认领（用户指定 ← 通道 1），分支 mcp2-oc21，基 a3187272
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：`internal/accessidentity` 的设备册查阅行与其 postgres 读口、`internal/accessidentity/adapters/http` 的只读查阅处理器、`apps/admin-web` 的作业设备册页。不改 `cmd/parcel-api/endpoints.go` 与 `assemble_*.go`。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定二与越权风险点 2；[10](./10-device-register-and-operation-fact-capability-face.md) 的设备册（机制半边已进 main）。

## 做什么

1. 设备册增一个按租户列出已登记设备的读口。每行只有设备标识、作业范围、登记还是停用；停用了的带停用时刻。不出型号、序列号、使用人、健康状态。
2. 一个只读的 HTTP 查阅端点，走操作者渠道。租户只取自信封，不采信请求。能力面在判断项里定。没配发行方答未配置；没有查阅读授予答未授予。空册答空列表，不答未配置。
3. 管理台一页，只读这些行。先例是价卡目录页。词用册上的原词（登记、停用、作业范围），不自造译法。

## 不做

不做设备管理，不扫描现场，不发设备标识，不校验设备健康，不改登记口的语义。不做 operator-channel/10 余下半边（作业事实换口、撤隔离放行）。不改 `internal/transportfulfillment` 与 `internal/nodeoperations` 的 `work_fact_intake.go`。

## 完成判据

- 读口按租户交回行，别的租户的设备不在其中；停用行带停用时刻，登记行不带；行上没有型号、序列号、使用人、健康状态，也没有依据。
- 查阅端点：发行方未配置答未配置；没有主数据与运营查阅读授予答未授予；空册答空列表；请求里的租户不被采信。
- 管理台一页只读这些行，用登记、停用、作业范围这三个词。

## 判断项

1. **能力面取「主数据与运营查阅读」，不取「作业事实登记」，不判准入范围。** 这是册的查阅，不形成作业事实。候选已由派单点名。越权风险点 · 待 owner 复核。
2. **查阅行不带依据。** 依据留在登记行上，不进查阅列。派单写明每行只有设备标识、作业范围、登记还是停用，以及停用时刻。越权风险点 · 待 owner 复核。
3. **一次交回该租户册上的全部行，不设页大小，按设备标识排序。** 页大小会把「册上没有」和「被截掉」混成同一张空脸。排序只为两次读回次序稳定，不表示业务优先级。越权风险点 · 待 owner 复核。
4. **线上状态词用 `REGISTERED` / `DEACTIVATED`，页面显示登记 / 停用；作业范围种类用 `NODE` / `OFFSITE`，页面显示节点 / 场外作业范围。** 页面不自造「启用」「在线」这类词。集外取值原样显示。越权风险点 · 待 owner 复核。

## Comments
