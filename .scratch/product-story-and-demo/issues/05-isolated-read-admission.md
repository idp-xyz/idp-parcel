# 05 隔离读面准入:按 ADR-0078 落装配注入放行

Category: feature
Status: resolved
Owner: MCP-1
Blocked by: (无——ADR-0078 已接受,机制半边即刻可做)

实现 [ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md):
隔离环境里,运营查阅端点的 Intake 按装配注入放行,使登记 CLI 灌入的合成 S 种子
在管理台读页可见。范围、判据与全部取舍以 ADR 正文为准,本票只列工序,不复述理由。

## 工序

1. 六个上下文(`parcelshipment`、`visibilityexception`、`parcelpricing`、`networkrouting`、
   `customscompliance`、`partycommercial`)各在 `adapters/http` 立一个隔离运营查阅
   Intake 类型:作用域与页大小由构造参数注入,实现不读请求中的任何授权输入(无定位
   参数的查阅参数匿名,同 `UnconfiguredIntake` 纪律;PS 详情分支的 `shipmentRequestId`
   属传输形状,照既有分工在 Intake 内解析——ADR-0078 勘误后措辞);**只实现查阅
   Intake 接口**,命令 Intake 一个不碰;带编译期断言。传输层测试:作用域来自注入、
   答复与请求内容无关、命令端点装不进(编译期即无此可能,测试覆盖查阅口行为即可)。
2. `assembleBusinessEndpoints` 增隔离读面输入(零值 = 现状逐字节同形):只切
   ADR-0078 Decision 一枚举的八条查阅行;`/customer-tracking-view` 与全部命令行
   不动。装配测试覆盖未设/设置两态。
3. `cmd/parcel-api` main:解析 `IDP_PARCEL_ISOLATED_READ_TENANT`;非 `SYN-` 前缀
   启动即拒、报错退出(不静默回落);启用时启动日志写明隔离读面准入与所用合成租户。
4. 手验:`IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01` 起进程,灌种子
   (`scripts/demo-seeds/`),经 admin-web 代理打八个查阅端点见数据、打任一命令
   端点仍 403;未设变量时全端点 403 照旧。

## 完成标准

全仓 `go build`/`go vet`/`go test`(真库套件)绿;上述手验两态截然;自己提交,
票面记 resolved + SHA + 哪种绿。页面侧「如实标注演示态」不在本票(属票 04)。

## Comments

- 2026-08-26 立票:用户经队列通道 1 委托裁断,三路选项(简报)裁定 b 路,ADR-0078
  记录裁决与全部理由。
- 2026-08-26 收口(MCP-1):**resolved**。实现 5023279(六上下文隔离 Intake+装配切换+
  main 门禁与启动日志+两态测试),ADR 勘误 affc299(委托查阅作用域带可见账户维、
  定位标识属传输形状),门禁修正 766bd58(前缀常量去分隔符,标识前缀门禁所拦)。
  **绿的种类**:①全量真库套件 `go test -p 1 ./...` 退出码 0(79 包 ok / 0 FAIL,
  DSN 55432);②受影响七包单测绿;③架构门禁绿。**手验**(本机,库内为频道 5 已灌
  种子):未设变量六探针全 403;`SYN-TENANT-01` 启用后八条查阅端点全 200(价卡端点
  逐行可见 SYN-PLAN-* 种子),POST /shipment-requests 与 /customer-tracking-view
  维持 403;`TENANT-PROD-1` 启动即拒且报错点名 SYN- 前缀与 ADR-0078;启动日志出声
  (Isolated read admission enabled + tenant)。委托/追踪两读页答空列表属实——事实链
  种子归票 04,账户约定 SYN-ACCOUNT-01 已写进票 04 要求。
