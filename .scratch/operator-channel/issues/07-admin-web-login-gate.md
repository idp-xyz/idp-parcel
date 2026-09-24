# 07 管理台登录门：authorization_code + PKCE、Bearer 与三态文案

Category: enhancement
Status: draft
Blocked by: 02、03
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`apps/admin-web/**`（前端切片，按 [workflow.md「前端切片」](../../../docs/agents/workflow.md) 一人在 main 上直接做）。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二第一条与 Consequences。

## 做什么

1. 外壳加登录门：authorization_code + PKCE 取令牌，请求带 Bearer；令牌只在内存，不落本地存储。
2. 各页「接入渠道未配置」的单一文案分成三态：渠道未配置、需要登录、没有授予。

## 完成判据

- 在接好发行方的隔离环境里，合成操作者能登进管理台并在 04、05 已换的口上走通；三态文案各有一条浏览器验收。
