# 03 `OperatorEnvelope` 与铸造、三格答复代数

Category: enhancement
Status: draft
Blocked by: 01、02
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`internal/accessidentity`（信封类型与铸造）、`internal/platform/httpapi` 若需新答复格；`internal/accessidentity/doc.go`；ADR-0072 / 0085 的前向指针核对。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定三、决定四答复代数与 Consequences。

## 做什么

1. `OperatorEnvelope`：租户、操作者主体、授予集、来源固定为管理台；字段不导出、包外无构造函数，拿到即经过铸造；不复用 `SourceEnvelope`，编译期拿它铸不出客户委托。
2. 铸造：校验令牌（02）→ 查操作者册（01）→ 按请求的租户与能力面核授予 → 铸信封。
3. 答复代数三格不合并（ADR-0029）：发行方未配置 → `403 ACCESS_CHANNEL_NOT_CONFIGURED`；令牌缺失、过期或校验不过 → `401`；令牌有效但不在册、不绑该租户或无此能力面授予 → `403` 新格，不披露哪一半不对（ADR-0055 决定四）。
4. `doc.go` 那段「本轮既没有登记册的表，也没有凭据形态」改为「客户渠道那一半仍等 `PAR-INT-01`；操作者那一半已按 ADR-0100 立」；ADR-0072 与 ADR-0085 的部分停用前向指针，缺则补。

## 完成判据

- 三格各有用例且互不顶替；册读失败答依赖故障而不是未授予；零值信封不可用。
