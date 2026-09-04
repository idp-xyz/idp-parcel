# 追踪页运营查阅作用域裁决简报

Status: superseded——本简报要等的那次裁决已发生并落地：运营查阅面的追踪读口走 [`ve-operations-tracking-read`](../ve-operations-tracking-read/spec.md) 全批（四票全 resolved，票 01 落 `c6c7914` + ADR-0076），本文只作 2026-08-24 那次取证的出处，不再是待裁项。状态由通道 2 于 2026-09-04 代簿记（原状态词「取证完成,待裁决(本简报不拍板;裁决归 owner/用户)」）
出处:票 03「三、追踪视图接真」与票 04 第三点记下的同一裁决闸;取证时点 2026-08-24。

## 问题

管理台全程追踪页(`TrackingProjectionPage`)接真被一个未裁决问题挡住:parcel-api 已挂载的读口 `GET /customer-tracking-view` 是**客户隔离作用域**,本页是**运营查阅面**——直接复用会把客户隔离视图当运营全景。取证结论先行一句:**漏报不是猜测,是结构性的**——客户视图库本身就不含全景(证据见「漏报的结构性根源」),因此「复用加参数」改的是门,根子在库。

## 一、现有读口的授权与隔离语义(代码取证,引符号名)

装配面(`cmd/parcel-api`):

- `assembleBusinessEndpoints` 把 `GET /customer-tracking-view` 交给 `visibilityhttp.NewQueryCustomerTrackingViewEndpoint(visibilityhttp.UnconfiguredIntake{}, trackingViews)`;`main.go` 中 `trackingViews` 由 `vepostgres.NewCustomerViews(db)` 构造——真库**客户视图**读适配器,不是投影读适配器。
- `endpoints.go` 注释已立读口分界:「requestViews 与 trackingViews 是两个查阅端点的读口:读面不是编排(查阅不触发判断、派生或披露)」。

读适配器(`internal/visibilityexception/adapters/http` 的 `query_customer_tracking_view.go`):

- **查询键**:`TrackingViewQuery` 三段——`Tenant`、`Customer`、`Parcel`;租户与货主客户账户「来自认证结果」,注释原话:「隔离在键上——查询只能问『我名下这个包裹』,问不出别人的;租户是最高数据隔离边界(ADR-0003)」。
- **授权入口**:`QueryIntake` 是接口且**本包不带任何实现**——「认证方式属 `PAR-INT-01` 待提供;采信客户自报的账户号会穿透账户隔离……HTTP 面不得另开口子。未决期间本包不带任何实现,包括『开发用』的采信头部版本」。装配点交的是 `UnconfiguredIntake`:不读业务内容,一律 403 + `ACCESS_CHANNEL_NOT_CONFIGURED`(ADR-0055)。
- **读面形状**:`TrackingViewReader.FindCurrent(ctx, tenant, customer, parcel)`——按三元组**点查单包裹**,无列表面;编译期锁缝 `var _ TrackingViewReader = ports.CustomerViewStore(nil)` 钉死端点只消费客户视图库。
- **业务结果**:「封闭两格」`CURRENT_VIEW` / `VIEW_NOT_FOUND`;后者承担 ADR-0029 探针纪律——「包裹不存在、不属于请求账户、或视图尚未形成,一律这一格——区分它们就是把对象存在性泄给跨账户探针」(UC-VE-008:「对未授权对象的外部响应不得泄露对象是否存在或属于其他客户」)。
- **内容形状**:`trackingViewBody`「四维三态如实转写」(milestones/eta/final/note)——是经披露规则过滤后的**客户视图**版本,不是内部投影的并行维度。

端口层(`internal/visibilityexception/ports` 的 `ports.go`):

- `CustomerViewStore`:键含租户与客户账户——「账户隔离是字段不是约定,按包裹一个键会让两个客户的授权范围共用一份视图」。
- `ProjectionStore`(内部投影库):`FindCurrent(ctx, tenant, parcel)` 与 `FindByVersion`——**键无客户维**(投影是租户内部对象),但只有点查与按版本读,无列表面,且**当前没有任何 HTTP 端点消费它**。
- `ParcelCustomerAccountView`:回答包裹当前归属账户;答 false 时「不派生视图、不发明账户,投影照旧存在」。

## 二、CONTEXT.md 的所有权表述(`docs/domain/visibility-exception/CONTEXT.md`)

- VE 同时拥有**两个不同对象**:Boundaries 首条把「全程追踪投影」与「客户全程追踪视图」并列为本上下文所有;Language 里「客户全程追踪视图」定义句明文:「它**不同于内部全程追踪投影**」。
- 客户视图的隔离规则(「客户可见性与通知」节):「每次普通追踪查询必须同时核对请求方身份、货主客户账户和目标对象授权」「客户全程追踪视图只展示适用服务与授权允许的公开里程碑、地点粒度、ETA、终局和说明,不暴露其他客户、内部节点控制、未公开线路、供应商商业信息、内部调查或敏感监管信息」。
- UC-VE-008 同向加钉:「客户视图**不是内部全程追踪投影的完整副本**。内部控制位置、非公开节点、路由策略、合作伙伴商业角色、调查记录、证据和未经确认责任默认不展示」;「货主客户账户是本用例的首要隔离边界」。
- 客户归属缺口:「归属不可确定期间不形成视图,也不得发明账户或把它表达为无轨迹」(CONTEXT 生命周期与 UC-VE-008 各有一句)。
- **CONTEXT.md 没有「运营查阅」的语言**:客户查询的授权规则有明文词条,内部运营人员读投影的授权语义(谁、什么范围、要不要按客户分区)在 Language/Rules 里没有对应表述。「追踪摘要」词条只说「用于阅读和检索」。若裁决走新读口,这半边语言需先补。

## 三、漏报的结构性根源(复用为何等于漏报)

1. **覆盖**:客户归属不可确定的包裹**没有客户视图行**,投影照旧存在(`ParcelCustomerAccountView` 注释与 CONTEXT 生命周期同句)。拿客户视图当全景,这些包裹结构性不可见——这正是票面「漏报」的实体。
2. **键形状**:`CustomerViewStore.FindCurrent` 需要(租户,客户,包裹)三元组点查;运营查阅是列表/全景语义,现有读口**连枚举都做不到**。
3. **内容删减**:客户视图按披露规则过滤内部维度(UC-VE-008 验收 AT-VE-157:内部控制位置、非公开节点、供应商商业信息不进客户视图);运营查阅恰恰要看投影的并行维度与冲突/待确认态——前端 `TrackingProjectionRow` 的列(物流进展/位置或控制范围/关务进展/交付或退运进展/异常影响/可见性缺口/投影版本)取的全是**投影**原词,客户视图body 里多数无对应字段。
4. **探针合并**:`VIEW_NOT_FOUND` 一格合并「不存在/不属账户/未形成」对外部客户是纪律(ADR-0029),对内部运营是信息损失——运营需要区分「投影未形成」与「引用打错」。

## 四、可选项(不拿主意;后果与代价从上述取证推出)

### A. 复用 `GET /customer-tracking-view`,加作用域参数

- **改法轮廓**:`TrackingViewQuery.Customer` 从必填键变为按作用域可选/多值;Intake 按调用方铸不同作用域。
- **后果**:(i) 覆盖缺口无解——读的仍是 `CustomerViewStore`,归属不可确定的包裹在库里没有行,参数改不出不存在的数据;(ii) 一个端点两种披露语义——客户调用要探针合并与内容删减,运营调用要解开合并、看全维度,`queryResponse` 封闭两格与 `trackingViewBody` 删减形状都得按调用方分叉,正是 `QueryIntake` 注释「HTTP 面不得另开口子」警告的方向;(iii) UC-VE-008 的隔离验收(AT-VE-151/155/157)按客户隔离写成,复用后对同一端点只对一半请求成立,验收口径被撕开。
- **代价**:改动看似集中一处,但客户面与运营面此后每次演进互相牵制;做完也只解掉探针合并一层,覆盖与内容两层仍漏。

### B. 另建运营读口(例:`GET /tracking-projections`,消费 `ProjectionStore`)

- **改法轮廓**:新端点 + 运营授权 Intake(认证同属 PAR-INT-01 待提供,同用 `UnconfiguredIntake` 门禁)+ 为 `ProjectionStore` 补列表读面(或新端口);outcome 按运营语义设计,可如实区分「无投影」与「未授权」,不背客户探针合并义务。
- **与先例一致**:`/shipment-request-views` 的 `ShipmentRequestViewsReader` 已立「查阅读口接存储读面不接编排」先例;前端 `TrackingProjectionRow` 列形状与投影原词天然对齐,页面名字本来就叫「投影」。
- **需先补的半边**:CONTEXT.md 缺「运营查阅」授权语言(见第二节末条);且 parcelshipment 的 `AuthorizedQueryScope` 先例**也是客户账户作用域**(「可见账户至少一个」,空作用域被拒)——租户级运营全景作用域在本仓没有先例形状。语言补充归 CONTEXT.md;端点形状与作用域模型属难逆转取舍,按 AGENTS.md「改文档」应走 ADR。
- **代价**:新端点、新端口读面、CONTEXT 语言补充、可能一份 ADR;落点在 `cmd/parcel-api`(当前 MCP-4 占号)与 `internal/visibilityexception`(后端地盘)。是「建一条新路」而非「改一扇门」,但与所有权表述同构。

### C. 继续搁置

- **后果**:前端保持未配置态——现状文案已如实记录裁决闸(`TrackingProjectionPage` 的 `viewState.facts.unlock` 原句);不接就无漏报风险。
- **代价**:追踪页持续骨架;裁决债记在票 03/04,每轮 UI 轮都要向后传一次。

### 任选 A/B 都改变不了的深度上限

PAR-INT-01(认证方式)未登记前,装配点是 `UnconfiguredIntake`,任何请求一律 403——已接线的委托查阅页(`ShipmentRequestListPage` 渲染 `UnconfiguredState`)就是这个深度。追踪页裁决后「接真」同样只能到「前端发请求、如实渲染未配置态」这一档,真数据等渠道参数。**这意味着裁决可以从容做:眼下没有任何选项能让页面显示真轨迹。**

## 裁决归属

visibility-exception 所有权(CONTEXT.md 为语言权威)+ parcel-api 组合面共同裁决;选 A 或 B 涉及端点形状/作用域模型的难逆转取舍时走 ADR。本简报只取证列项,不替 owner 拍板。
