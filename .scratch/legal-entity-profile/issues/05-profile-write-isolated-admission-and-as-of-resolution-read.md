# 05 服务端：资料登记写口进隔离写准入、法人资料按时点解析读口

Category: enhancement
Status: ready-for-agent——2026-09-24 通道 3 立（通道 1 就派单 `task-a5cd823d` 裁定「拆」：04 的「法人资料」区缺的两处服务端单立本票）
Blocked by: 无
地盘：party-commercial http 适配器（隔离写 Intake 加资料登记一口、新增按时点解析读口端点）；`cmd/parcel-api` 端点表两行（资料登记写行换 Intake 变量、解析读口新行）、装配入参与路由 / 隔离准入测试表。不碰领域与应用判断。
出处：[ADR-0091](../../../docs/adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)（写路径逐口放行、可分辨物由 `SYN-` 前缀承担）；[ADR-0077](../../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)（目录查阅通例）；[ADR-0145](../../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定五、六；[票 03](./03-legal-entity-profile-revisions-and-as-of-resolution.md) 完成记录「未做 / 风险」里「按时点解析只到应用用例与端口」与「写口未进隔离写准入」；[票 04](./04-admin-web-identity-fields-and-profile-face.md)「法人详情加『法人资料』区」。

## 做什么

1. **资料登记写口按 ADR-0091 逐口放进隔离写准入**：`IsolatedPartyIdentityIntake` 加 `IntakeLegalEntityProfileRegistration`，载荷形状与受控 CLI `register-legal-entity-profiles` 的一项同形、不收 `tenantId`（租户取自准入作用域），可分辨性照身份族五口的先例；`cmd/parcel-api/endpoints.go` 资料登记那一行把字面量 `UnconfiguredIntake{}` 换成隔离写 Intake 变量。
2. **法人资料按时点解析读口**：如 `GET /commercial-group-legal-entities/{legalEntityId}/profile-resolution?at=…`（路径以既有商业目录读口的命名为准），按 ADR-0077 挂商业目录 Intake、进隔离读准入。答复把 `ResolveLegalEntityProfileHandler` 的各格如实译出：已解析（修订引用与内容）、资料不全两成因（缺开票资料时带出有效的那一笔）、法人未登记 / 未生效 / 已停用；不在读口里另算。`at` 由调用方给，缺或形状不对答 400，不拿服务端时钟代填——同一问两次要得同一答。

## 不做

- 不动领域判断与应用用例（`ResolveLegalEntityProfile`、资料门）。
- 不开生产操作者渠道（归 operator-channel 票族）。
- 不改管理台（归票 04）。

## 完成判据

- 隔离写三态：未启用答 403、启用对合规载荷答 201 `REGISTERED`、带 `tenantId` 答 400；真库一条经隔离写 Intake 与生产装配登下一笔资料修订。
- 解析读口端点用例：各格逐一转写，读失败答没形成答案；隔离读放行表加一行，启用态经真路由答 500（未接线读口）而不是 400。
- 端点表与路由探针表同步；作者自验 `cmd/*` 带 DSN。
