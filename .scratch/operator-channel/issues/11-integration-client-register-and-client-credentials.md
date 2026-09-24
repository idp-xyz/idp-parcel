# 11 集成客户端册与客户端凭据校验：外部结果与资金事实的信任入口

Category: enhancement
Status: ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）
Blocked by: 02
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：`internal/accessidentity`（集成客户端册、客户端凭据令牌校验、集成客户端信封）与其迁移、受控登记 CLI、参数登记册增「集成客户端与凭据」一行。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三前两条。

## 做什么

1. 集成客户端册：客户端标识绑定唯一租户与一个来源身份，授予按事实类型登记、可撤销、带生效区间；凭据本体不入库，只存凭据引用。
2. OAuth 2.0 client credentials 令牌校验，方式同 02；可选 mTLS 绑定。
3. 集成客户端信封：租户、客户端、来源身份、授予集；与操作者信封、客户来源信封分型，编译期不可互换。

## 完成判据

- 客户端未登记、授予不含该事实类型、区间外、令牌无效各答其格（带测试）；参数登记册那一行已增。
- 随 [ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三（2026-09-24 补）：换上集成客户端 Intake 的各口（今天经写开关放行的外部结果、监管凭证登记、外部资金事实等）同一笔撤下隔离放行，隔离放行用例改写为真渠道答复格用例。
