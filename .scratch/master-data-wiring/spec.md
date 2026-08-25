# 主数据区 7 页接线(B 批)+ 合成 S 种子包

Category: feature
Status: in-progress

授权来源:用户经 IDP 队列(通道 2)于 2026-08-25 上午放行——10:44 左右「开干」批准
[可接线前沿调研](../ve-operations-tracking-read/next-wiring-frontier.md)的 B 批主数据方案,
10:52 左右指示「并行下发任务给 mcp-3/4/5/6」。分派人 MCP-2。

## 范围

接线 7 页,按上下文分四票后端 + 三票收口:

| 页面(moduleId) | 主责上下文 | 存储 | 票 |
|---|---|---|---|
| price-card-catalog | parcel-pricing | price_card_catalog(0002) | 02 |
| reference-series | parcel-pricing | reference_series_register(0003) | 02 |
| network-catalog | network-routing | 0008 七张版本表 | 03 |
| service-areas | network-routing | 0008 的 service-area 族 | 03 |
| compliance-rules | customs-compliance | 案件配置五登记册 + 案件要求规则 | 04 |
| service-products | party-commercial | service_product_form(0008) | 05 |
| commercial-policies | party-commercial | 五策略表(0005/0007/0010/0011/0014) | 05 |

**明确不做**(前沿调研三断点已于 2026-08-25 复核,证据见下):

- group-legal-entities / business-parties / party-contracts / supplier-agreements:PC 本体
  机制未开工,建模先行,保持诚实骨架。
- channel-product-catalog:`CommercialObjectKind` 封闭集合(commercial_version.go)里
  没有渠道产品目录这一格,无存储,维持阻断。
- customs-ports-paths:案件配置五登记册(就绪/提交授权/解释规则/关闭义务/门禁条件,
  register_case_configuration.go)不含口岸与申报路径,维持阻断。
- service-areas 的 0007 network_definition 登记册:无写入方(两表合流判给解析层,
  ADR-0068 Decision 六,见 cmd/parcel-network-register/main.go 头注),本轮只读 0008 的
  service-area 版本骨架,页面如实说明地理覆盖列尚不存在(PAR-NET-14)。

## 模式权威

- 四件套先例:提交 `267cb44`(票 ve-operations-tracking-read/02)——ports 伴生读端口 +
  postgres 读适配器 + adapters/http 查询端点 + cmd/parcel-api 装配。
- 裁决通例:[ADR-0077](../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)
  (本 feature 票 01 落库)——独立查询端点、每上下文自立租户级作用域、未配置即拒、
  空目录如实答空、租户维在方法签名上。
- Intake 机制:[ADR-0055](../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md);
  作用域形状:[ADR-0076](../../docs/adr/0076-operations-tracking-read-is-a-separate-endpoint-on-the-projection-store.md) Decision 二。

## 深度上限(全部票共同)

PAR-INT-01 接入认证未登记,一切新端点装配 UnconfiguredIntake,生产路径一律
403 `ACCESS_CHANNEL_NOT_CONFIGURED`。页面接线到「发请求、如实渲染未配置态」一档,
与已接线页同档。真数据展示只发生在隔离合成 S 环境:种子经登记 CLI 灌入本机库,
证据层级记 S,不写任何生产默认值。

## 全部票共同纪律

- 注释一律中文;领域术语用文档原词;跨文件引用不用行号不用计数(AGENTS.md)。
- 领域包不依赖 HTTP/`pgx`;读面接存储侧,不接编排(/shipment-request-views 分界句)。
- 不建新表、不写迁移;只读已有表。不实现写端点。
- 共享树纪律(docs/agents/parallel-sessions.md):逐文件 `git add`,绝不 `-A`;
  不跑 `go mod tidy`/全树格式化;不碰树上他人未提交改动
  (governance/ReconciliationPage.tsx、.cursor/mcp.json、.scratch/agent-docs-local-env、
  MCP-1 在途的 pages/visibility/** 与 .scratch/ve-operations-tracking-read/issues/03)。
- Windows 暗礁(docs/agents/workflow.md 本机环境):不用 `Set-Content` 改源文件;
  gofmt 判输出不判退出码。
- 真库测试:容器已跑在 127.0.0.1:55432,自己 shell 设
  `$env:IDP_PARCEL_POSTGRES_DSN = "postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable"`,
  用 `-count=1`,以 `-v` 下 PASS(非 SKIP)为准并在收工报告里写明是哪种绿。
- 完工:自己提交自己的文件(提交信中文,引票号),票面记 `Status: resolved` + SHA +
  验证结果,回频道 2 报「已验 SHA + 自上一已验 SHA 以来动没动 .go/.sql」。
- **不推远端**;集成推送统一由人另行决定。

## 子票

- 01 ADR-0077 主数据目录查阅通例 —— MCP-2,已落
- 02 parcel-pricing 读面(价卡目录+参考序列) —— resolved(f4dad52)
- 03 network-routing 读面(网络目录+服务区域) —— resolved(c1e10ce)
- 04 customs-compliance 读面(合规规则库) —— resolved(2d5c8ff)
- 05 party-commercial 读面(服务产品+商业策略) —— resolved(ecf268e)
- 06 cmd/parcel-api 装配(七端点登记) —— resolved(820c5a4,补验 3b6c03d)
- 07 admin-web 七页接线 —— 2026-08-25 18:02 改派 WSL 队列频道 3,证据见票面 Comments
- 08 合成 S 主数据种子包 —— 2026-08-25 18:02 改派 WSL 队列频道 5,证据见票面 Comments

## 调度轮注记

- 2026-08-25 18:02 WSL 队列频道 1:用户经该队列指示「启动 idp-parcel 实施」并授权调度
  频道 3/5/7/9。02–06 的 resolved 与 SHA 按 git log 与各票面对账回填;07/08 改派证据记
  在各票面 Comments。演示动线(product-story-and-demo/04)的筹备半边(只读)同轮派频道 7,
  执行半边仍候 07/08。本轮完工报告一律改报 WSL 队列频道 1;不推远端照旧。
