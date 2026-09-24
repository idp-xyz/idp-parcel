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
| [02](./issues/02-split-parameter-register-and-retriage-deferrals.md) | 参数登记册逐行拆分，以「实例半边」为由的暂缓逐份重新定性 | resolved · 拆分落登记册，划出的产品策略转 06–14 |
| [03](./issues/03-reference-configuration-adoption-pattern.md) | 参考配置的存放、版本与显式采用路径，在注册号类型目录上立样板 | resolved · 已进 main（ADR-0147 `27ca40f9`，代码 `c1446ebc`…`b215c11d`）· 评审 ← 通道 3 可接受 |
| [04](./issues/04-routing-product-strategy-first-cut.md) | 路由第一刀：拆 `PAR-NET-14`，路由策略族与首个内置策略 | in-progress · 跟踪容器，子票在 [`routing-first-cut/`](../routing-first-cut/issues/)（ready-for-agent，清单与阻塞边只记在 04 票面）· 通道 5（分支 `mcp5-psb04`） |
| [05](./issues/05-demo-journey-criterion-evidence.md) | 第四条判据的动线取证：逐步列出停在`未配置`的每一格 | ready-for-agent · 收口 Blocked by 02、03、04 |
| [06](./issues/06-ps-acceptance-and-label-selection-judgment-methods.md) | PS：受理链与面单择优链上被归进实例半边的判断方法 | needs-triage |
| [07](./issues/07-pc-authorization-coordinates-and-role-models.md) | PC：授权请求坐标的推导与各上下文的角色模型 | needs-triage |
| [08](./issues/08-no-current-valid-measurement-derivation.md) | NO：当前有效实测的派生方法 | needs-triage · Blocked by `pp-pricing-input-seams/04` |
| [09](./issues/09-tf-fulfillment-judgment-methods-and-connectors.md) | TF：履约判断方法与出入向连接器 | needs-triage |
| [10](./issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) | CC：申报发送通道与公开监管标准的参考配置 | needs-triage · 参考配置那半 Blocked by 03 |
| [11](./issues/11-ve-eta-triage-disclosure-methods-and-notification-connector.md) | VE：ETA、分诊、异常目录与披露的判断方法和通知出向连接器 | needs-triage |
| [12](./issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md) | SA：金额文法、分摊与周期费用形态、经营指标方法与账务连接器 | needs-triage |
| [13](./issues/13-pg-hard-risk-detection-and-evidence-storage-connector.md) | PG：硬风险检测形态、证据存储连接器与回放差异分类 | needs-triage |
| [14](./issues/14-pp-postal-prefix-granularity-and-public-unit-reference-configuration.md) | PP：邮编前缀匹配形态与公开标准的参考配置 | needs-triage · 参考配置那半 Blocked by 03 |
| [15](./issues/15-operator-channel-per-adr-0100.md) | 横切：ADR-0100 操作者渠道落地 | in-progress · 跟踪容器，子票在 [`operator-channel/`](../operator-channel/issues/) 01–09（draft，拆法待认可） |
| [16](./issues/16-mechanism-gaps-without-a-ticket.md) | 机制缺口：重定级表第一项里尚无票的几处 | needs-triage · 按上下文拆 |

走法：01、02、05 只动文档与票面，一人在共享树上顺序做；03、04 碰 Go / SQL，走[并行会话](../../docs/agents/parallel-sessions.md)那条路。

## 红线

- 租户取值照旧留空、拒绝默认；参考配置没有租户显式采用就不生效。
- 演示租户上的采用记录与演示数据只记 `S`，不进参数登记册。
- 以「实例半边」为由暂缓过的 ADR 只重新定性，不改写正文。
