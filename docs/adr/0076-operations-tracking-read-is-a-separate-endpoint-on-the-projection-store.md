# ADR-0076: 运营追踪查阅走独立读口消费投影库，不复用客户视图端点；运营作用域是租户级、无客户维

Status: Accepted  
Date: 2026-08-24

## Context

管理台全程追踪页是运营查阅面，要读的对象是投影：页面各列（物流进展、位置或控制范围、关务进展、交付或退运进展、异常影响、可见性缺口、投影版本）取的全是 CONTEXT「全程追踪投影」的并行维度原词。而 `cmd/parcel-api` 唯一挂载的追踪读口是 `GET /customer-tracking-view`：`assembleBusinessEndpoints` 把它交给 `visibilityhttp.NewQueryCustomerTrackingViewEndpoint(visibilityhttp.UnconfiguredIntake{}, trackingViews)`，`main.go` 的 `trackingViews` 由 `vepostgres.NewCustomerViews(db)` 构造——读的是客户视图库，不是投影库。

[visibility-exception CONTEXT](../domain/visibility-exception/CONTEXT.md) 把「全程追踪投影」与「客户全程追踪视图」并列为本上下文拥有的两个不同对象，客户视图定义句明文「它不同于内部全程追踪投影」。把客户视图端点当运营全景用，不是加一个参数的事，是把一个对象当另一个对象读。漏报不是猜测，是结构性的，四层各自独立成立：

一、**覆盖**。客户归属不可确定的包裹没有客户视图行，投影照旧存在——`ports.ParcelCustomerAccountView` 的注释写明答 false 时「不派生视图、不发明账户，投影照旧存在」，CONTEXT 硬句同向：「归属不可确定期间不形成视图，也不得发明账户或把它表达为无轨迹」。拿客户视图库当全景，这些包裹结构性不可见；库里没有的行，任何查询参数都改不出来。

二、**键形状**。`TrackingViewQuery` 三段（`Tenant`、`Customer`、`Parcel`），端点读口 `TrackingViewReader.FindCurrent` 按三元组点查单包裹，无列表面；编译期锁缝 `var _ TrackingViewReader = ports.CustomerViewStore(nil)` 钉死端点只消费客户视图库。运营查阅是列表与全景语义，现有读口连枚举都做不到。

三、**内容删减**。`trackingViewBody` 是经披露规则过滤后的客户版本（UC-VE-008：「客户视图不是内部全程追踪投影的完整副本」；AT-VE-157 验收内部控制位置、非公开节点、供应商商业信息不进客户视图）；运营查阅恰恰要看投影的并行维度与冲突、待确认态，客户视图体里多数无对应字段。

四、**探针合并**。客户端点业务结果是封闭两格 `CURRENT_VIEW` 与 `VIEW_NOT_FOUND`，后者一格三义——注释原话：「包裹不存在、不属于请求账户、或视图尚未形成，一律这一格」。对外部客户这是 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 纪律与 UC-VE-008 验收（「对未授权对象的外部响应不得泄露对象是否存在或属于其他客户」，AT-VE-151、AT-VE-155）；对租户内运营它是信息损失——运营要分得开「投影未形成」与「引用打错」。

供给侧的对象已经在：`ports.ProjectionStore` 键为（租户，包裹）、无客户维，注释自带理由「租户是最高数据隔离边界（ADR-0003）：TrackedParcelReference 只是字符串引用，缺租户维两个租户的同名包裹就会共用一份投影」；有 `FindCurrent` 与 `FindByVersion`（[ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md) 的审计口），无列表面；当前没有任何 HTTP 端点消费它。

读口分界有先例：`/shipment-request-views` 的 `ShipmentRequestViewsReader` 注释已立「查阅不触发判断、决定或披露——所以这里接存储读面，不接应用编排」。作用域没有先例：本仓唯一的查询作用域 `parcelshipment/domain` 的 `AuthorizedQueryScope` 是客户账户作用域，构造器以「可见账户至少一个」为不变量、空作用域被拒；租户级全景作用域的形状本仓没有过，本记录钉住它。

方向经用户 2026-08-24 放行（方向 B：另建运营读口；授权来源句记于 [ve-operations-tracking-read 规格](../../.scratch/ve-operations-tracking-read/spec.md)）。CONTEXT 的「运营追踪查阅」词条、投影规则与生命周期句随本记录同笔落地。

## Decision

**一、运营追踪查阅走独立查阅端点，消费投影库。** 新端点（路径形如 `GET /tracking-projections`，最终路径归装配那笔）接投影库一侧的存储读面，不接编排——沿 `/shipment-request-views` 的分界句。`GET /customer-tracking-view`、`CustomerViewStore` 与其编译期锁缝原样不动。两个端点各答各的对象，与 CONTEXT 两对象并列的所有权表述同构。

**二、运营作用域是租户级、无客户维的独立形状。** 形如（作用域引用，租户）两维，两样必须非零；**没有**货主客户账户维——不是「客户维可选」：留一个可选客户维就是把两套披露语义装回同一个口，正是客户端点 `QueryIntake` 注释「HTTP 面不得另开口子」警告的方向。空租户拒绝构造，判据与 `AuthorizedQueryScope` 同款：授权能力答不出租户，该在接入处拒绝这次查询，而不是造一个空作用域让每个读口各自决定它是「全租户可见」还是「什么都不可见」。按客户过滤可以是查询条件，但那是过滤器不是授权边界——运营查阅的授权边界只有租户。

**三、授权入口沿 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 机制：Intake 接口加装配「未配置即拒」。** 运营接入面的认证方式属接入渠道实例半边、未登记（所等参数以[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)为准，错误码命名状态不命名参数——ADR-0055 同款），未登记前装配 UnconfiguredIntake，一律 403 `ACCESS_CHANNEL_NOT_CONFIGURED`；禁落任何「开发用」采信实现，与客户面同一条红线。作用域只在真 Intake 之后由认证结果铸造。角色与权限细分不在本记录：现在替租户拟运营授权策略，就是替租户拟实例半边。

**四、outcome 按运营语义分格，不承袭 `VIEW_NOT_FOUND` 的三义合并。** 「尚无投影」对租户内已授权的运营查阅如实作答。ADR-0029 的探针同答保护对外部账户的存在性作答（ADR-0055 已立「保护租户域资源的存在性，不保护产品能力目录」的同款分界），对租户内已认证的运营查阅如实作答不触碰它；对外部客户面的合并义务也不因本记录松动。具体 outcome 词表随实现那笔按 ADR-0029 的恢复动作判据立。

**五、投影库的列表读面随实现那笔补，键形状本记录钉住：租户维在签名上，无客户维。** 落在 `ProjectionStore` 扩展还是伴生读端口归实现那笔；无论哪种，与 `ProjectionStore` 现有方法同派——租户在签名上看得见，不受 [ADR-0071](./0071-catalogue-views-carry-tenant-in-the-method-signature.md) 去留影响（其分歧在目录与策略视图的构造期绑定，仓储派两头一致）。

## Consequences

- 客户面零改动：`GET /customer-tracking-view`、客户视图库与 UC-VE-008 验收口径原样。
- 新代码落 `internal/visibilityexception`（读面、作用域、Intake、HTTP 端点）与 `cmd/parcel-api`（装配行），属机制半边，随本轮实现票；前端追踪页接线后到「发请求、如实渲染 403 未配置」一档，与已接线的委托查阅页同档。接入渠道认证参数未登记前，没有任何选项能让页面显示真轨迹——本记录不改变该深度上限，真轨迹属实例半边。
- 为该端点立 UC 时引本记录与 CONTEXT「运营追踪查阅」词条，不另造第二套口径。
- 若用户收回方向 B，本记录按 [README](./README.md) 的取代机制处理，不改写历史。

## Alternatives considered

- **复用 `GET /customer-tracking-view`，加作用域参数（被否决的选项 A）。** 否决，三条各自足够：(i) 覆盖缺口无解——读的仍是客户视图库，归属不可确定的包裹在库里没有行，参数改的是门，根子在库；(ii) 一个端点两套披露语义——客户调用要探针合并与内容删减，运营调用要解开合并、看全维度，封闭两格与 `trackingViewBody` 的删减形状都得按调用方分叉，正是「HTTP 面不得另开口子」警告的方向；(iii) UC-VE-008 的隔离验收（AT-VE-151、AT-VE-155、AT-VE-157）按客户隔离写成，复用后对同一端点只对一半请求成立，验收口径被撕。
- **给客户视图库补全景行，让归属不可确定的包裹也有行。** 否决：CONTEXT 硬句明禁「归属不可确定期间不形成视图，也不得发明账户或把它表达为无轨迹」；为了读全景改写客户视图的形成条件，是让读需求倒灌进披露对象。
- **继续搁置。** 否决：覆盖、键形状、内容、合并四层没有一层随时间消失，裁决债每轮 UI 都要向后传一次；且用户已放行方向 B，搁置不再是现状。
- **运营作用域复用 `AuthorizedQueryScope`，把客户账户集放空表示全租户。** 否决：该构造器以「可见账户至少一个」为不变量，空集被拒是它的正确性来源；其注释拒绝空作用域的理由——「让每个读口各自决定空集合是『全都可见』还是『全都不可见』」——一字不差地适用于此。两个作用域各自成形，谁也不参数化谁。

## Links

- [ADR-0003：采用集团租户、法人责任与货主客户账户三级边界](./0003-group-tenant-legal-entity-customer-account.md)：租户级作用域与投影键租户维的共同出处
- [ADR-0029：按标识取回原状态失败时，结果代数按消费方的恢复动作分格](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：探针同答纪律的原始范围；本记录立其保护对象（对外部账户的作答）与租户内运营查阅的分界
- [ADR-0055：业务端点 Intake 增设「未配置」格](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：运营 Intake 沿用的机制先例；「保护存在性不保护能力目录」分界句出处
- [ADR-0065：追踪投影版本只增不改写](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md)：`FindByVersion` 审计口——运营查阅按版本读回的依据
- [ADR-0071：visibility-exception 的目录与策略视图把租户放进方法签名](./0071-catalogue-views-carry-tenant-in-the-method-signature.md)：Proposed 草案；本记录的读面属仓储派，两头一致，不依赖其结论
- [visibility-exception CONTEXT](../domain/visibility-exception/CONTEXT.md)：两对象并列所有权与「运营追踪查阅」词条
- [UC-VE-008](../application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)：客户面验收口径，本记录不触碰
- [追踪页运营查阅作用域裁决简报](../../.scratch/admin-web-uiux-20260824/tracking-scope-decision-brief.md)：四层取证的原始记录
- [ve-operations-tracking-read 规格](../../.scratch/ve-operations-tracking-read/spec.md)：方向 B 的授权来源句与本轮票序
