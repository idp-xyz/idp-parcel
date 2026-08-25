# ADR-0077: 主数据登记目录查阅沿运营读口通例——独立查询端点、每上下文自立租户级作用域、未配置即拒;空目录如实答空

Status: Accepted  
Date: 2026-08-25

## Context

管理台主数据区的一批页面是运营查阅面,读的对象是各上下文的登记目录与策略登记册:价卡目录与计价参考序列(parcel-pricing)、网络目录与服务区域(network-routing)、合规规则登记册(customs-compliance)、服务产品与商业策略(party-commercial)。这批对象的存储表与受控登记口(`cmd/parcel-pricing-register`、`cmd/parcel-network-register`、`cmd/parcel-customs-register`、`cmd/parcel-commercial`)已在,缺的是在线列表读面——登记口是治理动作的进程入口,不监听端口,「是登记口不是后台 CRUD」的分界句写在 `cmd/parcel-network-register` 的包注释里,拿它兼任在线读面就是推翻那句分界。

[ADR-0076](./0076-operations-tracking-read-is-a-separate-endpoint-on-the-projection-store.md) 为运营追踪查阅逐条裁过:独立读口、租户级无客户维的作用域、Intake 沿 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 未配置即拒、outcome 按运营语义分格、读面租户维在签名上。主数据区这批页面每页都要过同类裁决;逐页重裁会把同一套理由复制若干份,违反单一权威。本记录把通例立在「主数据登记目录查阅」这一类上,一处定义,各实现票引用。

范围裁剪的取证记录(渠道产品目录在 `CommercialObjectKind` 封闭集合里没有格、口岸与申报路径不在案件配置登记册覆盖内、0007 网络定义登记册无写入方)见 [master-data-wiring 规格](../../.scratch/master-data-wiring/spec.md);方向经用户 2026-08-25 上午经队列放行,授权来源句记于同一规格。

## Decision

**一、每个主数据目录查阅走独立查询端点,消费所属上下文的存储读面,不接编排。** 读端口是既有登记写口的伴生列表读口,分界句沿 `/shipment-request-views` 先例:「查阅不触发判断、决定或披露——所以这里接存储读面,不接应用编排」。不复用登记 CLI,不跨上下文合库:一个「主数据查询服务」把四个上下文的表合进一个读面,穿的是 CONTEXT-MAP 的所有权边界。

**二、每上下文自立租户级运营查阅作用域,形状同 ADR-0076 Decision 二,各自成形互不参数化。** (作用域引用,租户)两维非零,无客户维。不从 `visibilityexception` 导入共享作用域类型:作用域是各上下文语言的一部分,共享类型让边界在类型上互相依赖,与「两个作用域各自成形,谁也不参数化谁」同一条理由。

**三、Intake 沿 ADR-0055 机制:装配「未配置即拒」,一律 403 `ACCESS_CHANNEL_NOT_CONFIGURED`。** 运营接入认证属接入渠道实例半边、未登记(所等参数以[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)为准);禁落任何「开发用」采信实现;作用域只在真 Intake 之后由认证结果铸造。

**四、空目录如实答空列表,走 2xx 成格,不折成未配置。** 「未配置」属接入渠道,住在 Intake 缝里;目录内容为空是租户内已授权查阅的一个如实答案。两格的恢复动作不同([ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 判据):空目录的续办是操作员去登记口登记,未配置的续办是接入方去配置渠道。此格与 [ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)/[ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md)/[ADR-0054](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md) 的「未配置」格不冲突:那三条立在**判断与证据端口**上——读空册形成不了判断,答未配置是拒绝用空册冒充判断依据;本记录立在**目录上列**上——空表本身就是内容,上列不形成任何判断。同一张表,证据端口与目录读口各答各的格。

**五、列表读面的键形状:租户维在方法签名上;limit 非正拒。** 与 ADR-0076 Decision 五同派(仓储派,不受 [ADR-0071](./0071-catalogue-views-carry-tenant-in-the-method-signature.md) Proposed 去留影响)。列表口不拓宽既有写口接口——扩写侧接口会拆全部写侧测试替身,伴生读端口另立。

**六、本记录只覆盖已有存储的目录查阅。** 集团法人、业务参与方、客户合同、供应商协议(本体机制未开工)、渠道产品目录(无存储格)、口岸与申报路径(不在登记册覆盖内)不在本记录,不因本通例获得任何读面;它们等各自的建模票。上列哪几本册子、透哪些字段,属各上下文 CONTEXT 词汇对照,归各实现票,不归本记录。

## Consequences

- 四个上下文各得伴生列表读端口、postgres 读适配器与查询端点;`cmd/parcel-api` 增对应装配行;管理台七页从骨架转「发请求、如实渲染 403 未配置」一档,与已接线页同档。
- 接入认证参数未登记前,生产路径没有任何选项能让页面显示真数据——深度上限与 ADR-0076 一致,不因本记录改变。隔离合成 S 环境里,种子经登记 CLI 灌入后页面可见合成数据,证据层级记 S。
- 为这批端点立 UC 时引本记录与各 CONTEXT 词条,不另造第二套口径。
- 后续若某目录长出「按内容维过滤」的查询语义(过滤器是查询条件不是授权边界,ADR-0076 Decision 二同句),在各上下文票内扩,不回改本记录。

## Alternatives considered

- **逐上下文各写一篇 ADR。** 否决:四篇的 Context 与 Decision 会是同一套理由的四份复写,单一权威红线;上下文特有的判断(上列范围、字段词汇)本就留给各实现票,不需要 ADR 级裁决。
- **拓宽既有登记写口接口加列表方法。** 否决:写口接口的全部测试替身要跟着扩;读写混在一个接口上,下一个写侧替身还得再实现一遍列表。伴生读端口零成本分开。
- **建跨上下文「主数据查询服务」合库上列。** 否决:限界上下文表达数据所有权,合库读面让四个上下文的表结构变成一个共享消费者的隐式契约,任何一侧改列都拆它。
- **前端 mock 演示数据。** 否决:UnwiredModule 的设计注释明确拒绝模拟列表(「看起来能用」误当「已交付」);证据红线要求合成 S 只记为 S——mock 连 S 都不是,它没有经过任何真机制。演示走登记 CLI 灌合成种子。
- **等 PAR-INT-01 一起做。** 否决:ADR-0055 同款理由——租户数为零时那是一件不会到来的事,等待期间机制半边的欠账不会自己变小。

## Links

- [ADR-0076](./0076-operations-tracking-read-is-a-separate-endpoint-on-the-projection-store.md):作用域形状、读面键形状、独立读口裁决的通例来源
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md):未配置即拒的机制与 403 语义
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md):空目录与未配置分格的判据
- [ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)、[ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md)、[ADR-0054](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md):证据端口「未配置」格的原始范围;本记录 Decision 四划出目录上列与它们的分界
- [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md):0007 登记册合流判给解析层的出处(服务区域页边界)
- [ADR-0070](./0070-customs-rule-registries-split-recording-from-selection.md):关务规则登记册记录侧与选择侧的分界(合规规则库票的对照依据)
- [master-data-wiring 规格](../../.scratch/master-data-wiring/spec.md):范围取证、授权来源句与票序
