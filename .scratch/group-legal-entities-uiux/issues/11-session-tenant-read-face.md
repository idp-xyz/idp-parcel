# 11 — 本会话租户读口：顶栏作用域与本页页头接上

**Parent:** [spec](../spec.md)

**What to build:** 「集团与法人」这一页上看不到集团。集团是 ADR-0003 三级边界里的第一级，也就是租户。

- 在页面上，它只在概要签末尾以 `SYN-TENANT-01` 出现一次。
- 顶栏的作用域 chip 永远显示「作用域：由服务端按会话判定」。这是票 [admin-web-ux-alignment/01](../../admin-web-ux-alignment/issues/01-shell-top-bar-theme-density.md) 留下的兜底句，它等的正是一个「本会话绑哪个租户」的读口。
- 前端拿不到这个信息：`id_token` 里没有租户声明，操作者与租户的绑定在服务端的操作者册里（ADR-0100 决定二）。

本票新增这个读口，并把两处接上：顶栏作用域 chip 显示本会话的租户，集团与法人页的页头显示当前集团。

**边界：**

- 读口回答操作者册上该主体绑定的那一个租户。归属是操作者册的拥有者 `internal/accessidentity`。读口能不能顺带答出授予集，不在本票的判据里。
- 产品现在没有登记租户显示名的地方，所以只答标识，不编名称。如果要显示名称，那是另一项决定，要先按开发主线的三类把它归好：机制、产品策略，还是租户取值。
- 本票碰 Go 和端点表，**不走前端切片**，走[并行会话](../../../docs/agents/parallel-sessions.md)那条路。新端点要进端点表，也要进 admin-web 的端点消费者门禁。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** in-progress

- [ ] 带有效操作者令牌请求读口，答出该主体绑定的租户；拒绝各态与其他操作者口同口径（ADR-0100 决定四）
- [ ] 顶栏作用域 chip 显示租户标识；读口未配置或拒绝时，退回现有的兜底句
- [ ] 集团与法人页的页头显示当前集团
- [ ] Go 测试和三道门都过；演示环境截图，证据记 `S`

## 裁定（2026-10-16，作者会话认领；机制，没有租户取值）

1. **路径与归属：** `GET /operator-binding`，归 accessidentity（操作者册的拥有者）。实现放在 `internal/accessidentity/adapters/http`，与作业设备册查阅同一个包、同一个认证件。
2. **用哪一面守口：** 「主数据与运营查阅读」，复用 `AuthenticateCatalogueRead`：以这一面铸操作者信封，交回的是信封上的绑定租户。这样不越过 ADR-0100 决定四的同答纪律：没有这一面授予的人，与不在册、不绑这个租户的人同答 403 `OPERATOR_NOT_GRANTED`，看不出自己绑在哪；有这一面授予的人，本来就能从任一册的行上读到 `tenantId`，把它回给他不多泄露任何东西。
3. **答什么：** `{"outcome":"OPERATOR_BINDING_RESOLVED","tenantId":"…"}`。只答标识；产品没有登记租户显示名的地方，不编名称。
4. **走哪条路：** 改 Go 与端点表，按[并行会话](../../../docs/agents/parallel-sessions.md)办：在 worktree 分支 `claude-glu11` 上做，作者自验受影响的包，进 main 前在干净检出上带 DSN 跑一次全量，按 SHA 推。本仓不用子代理，本机也没有空闲通道可派评审，所以评审记为作者自审，不算非作者评审。
