# 05 隔离读面准入:按 ADR-0078 落装配注入放行

Category: feature
Status: ready-for-agent
Owner: 待派
Blocked by: (无——ADR-0078 已接受,机制半边即刻可做)

实现 [ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md):
隔离环境里,运营查阅端点的 Intake 按装配注入放行,使登记 CLI 灌入的合成 S 种子
在管理台读页可见。范围、判据与全部取舍以 ADR 正文为准,本票只列工序,不复述理由。

## 工序

1. 六个上下文(`parcelshipment`、`visibilityexception`、`parcelpricing`、`networkrouting`、
   `customscompliance`、`partycommercial`)各在 `adapters/http` 立一个隔离运营查阅
   Intake 类型:作用域与页大小由构造参数注入,实现不读请求任何部分(参数匿名,
   同 `UnconfiguredIntake` 纪律);**只实现查阅 Intake 接口**,命令 Intake 一个不碰;
   带编译期断言。传输层测试:作用域来自注入、答复与请求内容无关、命令端点装不进
   (编译期即无此可能,测试覆盖查阅口行为即可)。
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
