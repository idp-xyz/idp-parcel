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

- [x] 带有效操作者令牌请求读口，答出该主体绑定的租户；拒绝各态与其他操作者口同口径（ADR-0100 决定四）
- [x] 顶栏作用域 chip 显示租户标识；读口未配置或拒绝时，退回现有的兜底句
- [x] 集团与法人页的页头显示当前集团
- [x] Go 测试和三道门都过；演示环境截图，证据记 `S`

## 裁定（2026-10-16，作者会话认领；机制，没有租户取值）

1. **路径与归属：** `GET /operator-binding`，归 accessidentity（操作者册的拥有者）。实现放在 `internal/accessidentity/adapters/http`，与作业设备册查阅同一个包、同一个认证件。
2. **用哪一面守口：** 「主数据与运营查阅读」，复用 `AuthenticateCatalogueRead`：以这一面铸操作者信封，交回的是信封上的绑定租户。这样不越过 ADR-0100 决定四的同答纪律：没有这一面授予的人，与不在册、不绑这个租户的人同答 403 `OPERATOR_NOT_GRANTED`，看不出自己绑在哪；有这一面授予的人，本来就能从任一册的行上读到 `tenantId`，把它回给他不多泄露任何东西。
3. **答什么：** `{"outcome":"OPERATOR_BINDING_RESOLVED","tenantId":"…"}`。只答标识；产品没有登记租户显示名的地方，不编名称。
4. **走哪条路：** 改 Go 与端点表，按[并行会话](../../../docs/agents/parallel-sessions.md)办：在 worktree 分支 `claude-glu11` 上做，作者自验受影响的包，进 main 前在干净检出上带 DSN 跑一次全量，按 SHA 推。本仓不用子代理，本机也没有空闲通道可派评审，所以评审记为作者自审，不算非作者评审。

## 完成记录（2026-10-16，作者会话，分支 `claude-glu11`）

- **落点：** 分支上代码一笔 `bf26ecf3`（Go 与管理台同一笔），之后是机制清点重生成一笔。进 main 的 SHA 见下面的「进 main 记录」。
- **改了什么：**
  - accessidentity 新增 `query_operator_binding.go`：`GET /operator-binding` 复用 `NewOperatorCatalogueAuthenticator`，答 `{"outcome":"OPERATOR_BINDING_RESOLVED","tenantId":…}`。没接铸造器时答未配置，这一件与作业设备册共用 `unconfiguredOperatorRead`。
  - `cmd/parcel-api` 新增 `buildOperatorBinding`，在 `main.go` 接线，端点表加一行字面量 `/operator-binding`；装配测试 `assemble_operator_binding_test.go`，端点表测试加期望行。
  - 管理台：`pages/operator-binding.ts` 判读答复，node:test 钉着；`pages/use-operator-binding.ts` 一页只问一次、答不出不记。`Layout` 把绑定交给 TopBar 作用域 chip，集团与法人页头显示「集团 <标识>」。`TopBar` 与 `Layout` 的过时注释已改。
- **判据：**
  - ✅ 读口。处理器测试与装配测试覆盖了这几种答复：未配置 403、没有令牌 401、只有写授予 403 `OPERATOR_NOT_GRANTED`、有读授予 200 且答出夹具绑定的 `SYN-TENANT-01`、查询串里自报的 `SYN-TENANT-02` 不算数、依赖读不动 503、POST 答 405。真实演示环境（分支构建的 parcel-api，接 `psb26_walk` 库与 Dex）：不带令牌 401；配置员、查阅员两个账号的真令牌都答 200 `SYN-TENANT-01`。证据记 S。
  - ✅ 顶栏作用域 chip 显示「作用域：SYN-TENANT-01」；读口答不出时退回兜底句，由 `boundTenantOf` 的 node:test 覆盖各种非成格。
  - ✅ 集团与法人页头显示「集团 SYN-TENANT-01 · …」。真浏览器里两处共用一次 `/api/operator-binding` 请求。
  - ✅ 作者自验（worktree，未 rebase 前）：
    - 全仓 `go build ./...`、`go vet ./...` 退 0，改动文件 `gofmt -l` 无输出；
    - 带 DSN 跑 `go test -p 1 -count=1`，范围是 accessidentity 全包、`cmd/parcel-api`（唯一的反向依赖）、`internal/architecture`（含管理台端点消费者门禁），全部 ok；
    - 管理台三道门：`tsc` 退 0，`run-tests` 534/534，`vite build` 退 0。
- **评审：** 作者自审，**不算非作者评审**（本仓不用子代理，本机也没有空闲通道）。Standards 与 Spec 两轴：
  - ADR-0100 决定四的同答纪律守住了；租户只取自操作者册；端点表与门禁都已补上；注释合规。
  - ADR-0140 是草案、尚未接受，不算依据；它本身也写着操作者 API 不带版本前缀，与 `/work-devices` 的先例一致。
  - 自审 0 阻断。
