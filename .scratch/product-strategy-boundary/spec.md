# 产品策略边界：ADR-0146 落地

Category: enhancement
Status: in-progress——2026-09-24 通道 4 按用户授权自决立 ADR-0146 并拆票；子票待派
出处：用户 2026-09-24「我们是软件公司，不可能等到租户就绪、业务就绪再去补系统」→ [ADR-0146](../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)；
运行定义在[开发主线](../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「切片的机制半边与实例半边」一节。

规则只在 ADR-0146（取舍记录）与开发主线（运行定义）两处；本 spec 与子票只引，不复述。

## 子票

| 票 | 标题 | 状态 |
|---|---|---|
| [01](./issues/01-regrade-slices-under-four-criteria.md) | 按四项判据逐切片重定级 | resolved · 结论落开发主线 |
| [02](./issues/02-split-parameter-register-and-retriage-deferrals.md) | 参数登记册逐行拆分，以「实例半边」为由的暂缓逐份重新定性 | ready-for-agent |
| [03](./issues/03-reference-configuration-adoption-pattern.md) | 参考配置的存放、版本与显式采用路径，在注册号类型目录上立样板 | ready-for-agent |
| [04](./issues/04-routing-product-strategy-first-cut.md) | 路由第一刀：拆 `PAR-NET-14`，路由策略族与首个内置策略 | in-progress · 跟踪容器，子票在 [`routing-first-cut/`](../routing-first-cut/issues/)（draft，清单与阻塞边只记在 04 票面）· 通道 5（分支 `mcp5-psb04`） |
| [05](./issues/05-demo-journey-criterion-evidence.md) | 第四条判据的动线取证：逐步列出停在`未配置`的每一格 | ready-for-agent · 收口 Blocked by 02、03、04 |

走法：01、02、05 只动文档与票面，一人在共享树上顺序做；03、04 碰 Go / SQL，走[并行会话](../../docs/agents/parallel-sessions.md)那条路。

## 红线

- 租户取值照旧留空、拒绝默认；参考配置没有租户显式采用就不生效。
- 演示租户上的采用记录与演示数据只记 `S`，不进参数登记册。
- 以「实例半边」为由暂缓过的 ADR 只重新定性，不改写正文。
