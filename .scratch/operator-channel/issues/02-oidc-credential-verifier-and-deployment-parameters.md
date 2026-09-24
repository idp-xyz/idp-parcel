# 02 操作者族的凭据校验：OIDC 令牌校验器与 parcel-api 部署参数

Category: enhancement
Status: ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`internal/accessidentity`（`CredentialVerifier` 在操作者族上的生产实现）、`cmd/parcel-api` 部署形态参数。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二前两条、决定五第二条。

## 做什么

1. `parcel-api` 自己校验令牌：签名（按 JWKS）、`iss`、`aud`、有效期；不采信任何自报头部与 SPA 登录态。凭据本体只在校验那一刻存在，不落库、不进日志。
2. 部署形态参数：发行方地址、JWKS 端点、受众。未设即操作者渠道整族未配置，缺省朝拦；与 ADR-0049 一样全部必填、不给默认值。
3. 测试用进程内签发与 JWKS 替身；演示环境要接一个真 OIDC 实现作为发行方（选型在本票定，写进完成记录），不做绕过 OIDC 的「开发用」本地账号。

## 不做

- 操作者册（01）、信封与答复代数（03）。

## 完成判据

- 用例覆盖：合格令牌通过；签名错、`iss` / `aud` 不符、过期、缺令牌各自拒；JWKS 取不回答依赖故障而不是「凭据不对」；参数缺席时整族答未配置。
