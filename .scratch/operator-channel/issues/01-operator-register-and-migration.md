# 01 操作者册：领域、迁移首个模块、受控登记口与参数登记册一行

Category: enhancement
Status: ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可（「参考专业头部软件的做法，你来帮我自决吧」）；ADR-0149 另让本册多一格能力面「作业事实登记」，那一格归 10
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md)「操作者渠道落地」甲轨第一步
地盘：`internal/accessidentity`（操作者册的领域、端口与 postgres 适配器）、`migrations/access_identity/` 首个模块（共享接线文件 `migrations/migrations.go` 与计划装配按 parallel-sessions「占号、同笔、逐块核」办）、受控登记 CLI 一个子命令、参数登记册增一行。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二第三条、决定六与 Consequences。

## 做什么

1. 操作者册：操作者主体（发行方 + `sub`）绑定唯一租户；授予按能力面显式登记（登记册配置写、主数据与运营查阅读；治理登记那一格只预留，按 ADR-0085 决定四另裁），可撤销、带生效区间；不存在跨租户主体。列由 ADR-0100 定死，行是租户取值。
2. 迁移 `access_identity` 首个模块：结构与 CHECK 守上面的不变量，不种任何行。
3. 受控 CLI 登记口（登记主体、授予、撤销），定位是 ADR-0085 决定一的「受控批量口」；演示租户的合成操作者经它登记，只记 `S`。
4. 参数登记册增「运营操作者账户与授予」一行（租户取值：某租户的操作者主体、绑定与授予），写法照登记册既有行，`PAR-INT-01` 不动。

## 不做

- 凭据校验与信封铸造（02、03）；任何端点换线。

## 完成判据

- 真库用例：登记、重放、撤销、区间外不生效、跨租户主体拒收；CLI 端到端一条；干净检出迁移计划施加通过；登记册那一行已增。
