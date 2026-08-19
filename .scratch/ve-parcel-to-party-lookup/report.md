# VE-PARTY-LOOKUP-SURVEY：包裹 → 货主客户账户反查勘察

只读勘察。取证基准钉在 `7d7e138`（`feat(dispatch): 交接登记只投 VE 投影不 FanOut PS`），在
`$env:TEMP\idp-parcel-ve-party-lookup` 的 detached worktree 上取证，不在共享树 `D:\tops\idp-parcel`
上读写。切片 PN-06 / 主责上下文 `visibility-exception`。

本票不写实现代码、不改任何 `.go` / `.sql`，不推进 UC-VE-008，不登记
`visibility-exception.tracking-projection.derived`。

## 结论摘要

1. 追踪投影键是 `(tenant_id, parcel_ref)`，**不带**货主客户账户维；客户视图键是
   `(tenant_id, customer_account_id, parcel_id)`，**带**。差的正好是这一维。
2. 差的一维就是 `DeriveCustomerViewCommand.Customer`（`domain.CustomerAccountReference`）。唯一能
   唤醒该派生的上游 `ProjectionHandoffIntent` 只有 `{TenantID, Projection}`，自动接线时没有任何一
   处能填它——这就是「反查不成环」。
3. 「包裹 → 货主客户账户」**不归 VE 拥有**：PS 拥有「包裹 ∈ 哪份委托」，PC 拥有账户身份与查询授
   权，委托指向账户那一跳由 PS 的委托来源身份承担。VE 只能引用。
4. 这层关系是**机制半边**，而且**在 7d7e138 上已经做完了**——`CurrentAcceptedParcelTargetView`
   交回的 `SourceIdentity` 里就有 `CustomerAccountID`。派工票预设的「PS 已闭合但只闭合了包裹反查、
   账户那一跳还欠着」不成立，账户跟着同一次反查一起回来了。
5. 因此 Q4 的答案是**否**：VE 侧不是同形的一件事，**不需要新迁移**，要做的是一个消费方侧翻译口
   （ACL）把 PS 已有的读口引进来。VE 侧另建 parcel→account 表反而会撞「不拥有」与「单一权威」两
   条红线。

---

## Q1：追踪投影今天的主键/唯一键是什么？带不带货主客户账户维？

**主键 `(tenant_id, parcel_ref)`，没有第二个唯一键，不带货主客户账户维。**

库侧取证 —— `migrations/visibility_exception/0007_tracking_projection.sql`，约束
`tracking_projection_pkey` 声明为 `PRIMARY KEY (tenant_id, parcel_ref)`。该文件头一句原文：

> 全程追踪投影：键=租户+包裹，库只管当前版。重派生换版本改同一行，历史由 prior_version 指回承担。

全表列只有 `tenant_id`、`parcel_ref`、`version_id`、`derived_at`、`prior_version`、`entries`，没有任
何客户账户列，也没有额外 `CREATE UNIQUE INDEX`。

领域/端口侧取证 —— `vedomain` 与 `veports` 三处一致地只认两维：

- `ports.ProjectionStore.FindCurrent(ctx, tenant, parcel)`，其注释原文「租户是最高数据隔离边界
  （ADR-0003）：TrackedParcelReference 只是字符串引用，缺租户维两个租户的同名包裹就会共用一份投影」
  ——只提租户，不提账户。
- `ports.ProjectionHandoffIntent` 只有 `TenantID` 与 `Projection` 两个字段，注释原文「租户随意图到
  达（ADR-0003）：投影对象没有租户维，下游按（租户+包裹）查库」。
- `veapplication.DeriveCustomerViewCommand` 的注释把这件事说成设计意图而不是遗漏：「账户由调用方给
  出而不是从投影推导——**投影面向包裹，不认识账户**」。

作为对照，客户视图那一侧是带账户的：`migrations/visibility_exception/0001_customer_view.sql` 的
`customer_view_pkey` 声明为 `PRIMARY KEY (tenant_id, customer_account_id, parcel_id)`，该文件原文写明
「账户在键上不是过滤器而是身份的一部分」。

**这就是缝所在：上游按两维立键，下游按三维立键，中间那一维没有来处。**

## Q2：`CustomerViewStore.FindCurrent` 与 `DeriveCustomerViewCommand` 各要哪几维？差的是哪一维？

**`FindCurrent` 要四维（含 ctx 则五）；命令要三维；差的是货主客户账户 `Customer`。**

`ports.CustomerViewStore.FindCurrent` 签名要 `ctx`、`tenant domain.TenantID`、
`customer domain.CustomerAccountReference`、`parcel domain.TrackedParcelReference`。端口注释原文：

> 键含租户与客户账户——租户是最高数据隔离边界（ADR-0003），跨越它必须在签名上看得见；账户隔离是字
> 段不是约定，按包裹一个键会让两个客户的授权范围共用一份视图。

持久化侧同形：`vepostgres.CustomerViews.FindCurrent` 的 `SELECT` 走
`WHERE tenant_id = $1 AND customer_account_id = $2 AND parcel_id = $3`；`Save` 的 INSERT 与 UPDATE 也
都逐字带这三列。同文件的包注释原文「所有语句显式携带租户与客户账户条件：作用域不是过滤器而是身份的
一部分（ADR-0003）」。

`veapplication.DeriveCustomerViewCommand` 的字段是 `TenantID`、`Customer`、`Projection` 三个。
`Handle` 的第一道受理门逐个检查 `command.TenantID`、`command.Customer`、`command.Projection.Version()`、
`command.Projection.Parcel()`，四者任一为空即 `CustomerViewNotAccepted`。

**差的一维是 `Customer`，即 `domain.CustomerAccountReference`。** 更准确地说，差的不是这个字段本身
（它在结构体上已经存在），而是**它的来处**：

- `TenantID` 有来处：`ProjectionHandoffIntent.TenantID` 带着。
- `Projection` 有来处：`ProjectionStore.FindCurrent(tenant, parcel)` 能按信封载荷取回。
- `Customer` **没有来处**：`ProjectionHandoffIntent` 上没有这个字段，`TrackingProjection` 上也没有
  （Q1 已证），而 `CustomerViewStore.FindCurrent` 又必须先拿到它才查得动幂等。

今天这一维在两个地方是由**人**给的，不是自动接线给的：

- HTTP 读面 `visibilityhttp.TrackingViewQuery` 的 `Customer` 来自认证结果，端点注释原文「租户与货主
  客户账户来自认证结果，包裹引用来自请求定位」，且 `QueryIntake` 刻意不带任何实现（含「开发用」的
  采信头部版本），理由原文「采信客户自报的账户号会穿透账户隔离」。
- `cmd/parcel-api/unwired_orchestration.go` 的 `unwiredTrackingViews.FindCurrent` 顶着同一个四维签
  名，交回 `errOrchestrationNotWired`。

所以缺口的准确表述是：**缺一个「按追踪包裹取回当前货主客户账户」的只读口**，不是缺字段、不是缺表、
也不是缺 UC。

## Q3：这层关系按领域文档归谁拥有？机制半边还是实例半边？

### 归属：PS 拥有「包裹 ∈ 哪份委托」，PC 拥有账户身份与查询授权，VE 两样都不拥有

**VE 不拥有。** `docs/domain/CONTEXT-MAP.md` 的 `visibility-exception` **不拥有**栏第二条原文：

> 客户委托、包裹身份及谱系、客户承诺、终局服务结果、集运单元、路由计划、班次或装载分配。

`docs/domain/visibility-exception/CONTEXT.md` 的 Boundaries and ownership 一节把两边都写死了：

> `party-commercial` 拥有货主客户账户、参与方、责任法人、服务产品、客户合同、供应商协议和商业规则版
> 本；本上下文引用这些依据形成响应、披露、索赔和责任判断快照，不修改商业版本。

> `parcel-shipment` 拥有客户委托、包裹身份及谱系、外部标识关系、客户承诺和终局服务结果；本上下文按这
> 些身份和结果形成追踪投影，不决定委托取消、完成或包裹服务终局。

**PC 拥有账户对象本身。** `docs/domain/GLOSSARY.md` 的「货主客户账户」条目写「所有者：
`party-commercial`」；`docs/domain/party-commercial/CONTEXT.md` 的 Boundaries 首条原文「`party-commercial`
拥有业务参与方身份、参与方关系、运营集团与法人、货主客户账户以及客户和供应商商业关系」。

**PS 拥有包裹到委托那一跳。** `CONTEXT-MAP` 的 `parcel-shipment` **拥有**栏第一条原文：

> 提交批次的业务归组、客户委托、委托提交版本、接受判断任务、每次接受判断逐项解析的判断依据时点、接受
> 或拒绝决定、委托接受基线及**多件委托中的客户声明成员关系**。

「客户声明成员关系」正是「这件包裹属于哪份委托」。而「这份委托属于哪个账户」由委托的来源身份承担——
`psdomain.SourceIdentity` 的四个字段是 `tenantID`、`customerAccountID`、`source`、`requestKey`。

**关系边只允许 VE 引用，不允许 VE 自建。** `CONTEXT-MAP` 的
`party-commercial / parcel-shipment → visibility-exception` 一条原文：

> 参与方与商业提供货主客户账户、责任法人、服务产品、客户合同、客户查询授权及适用异常服务规则；小包托
> 运提供包裹身份谱系、客户承诺和终局服务结果。全程追踪保存实际采用的追踪公开、响应、披露和责任依据，
> 形成客户账户隔离的普通全程追踪视图，不修改商业版本、包裹身份、承诺或终局。

一句话：**这层关系是 PS 的「成员关系」与 PC 的「账户身份」的复合，VE 是纯消费方。**

### 半边判定：机制半边——而且它在 7d7e138 上**已经做完了**

这是本票最要紧的一条，也是与派工票预设不同的一条。

**判定：机制半边。** 三条取证：

1. **库里已经有了。** `parcel_shipment.shipment_request` 的 `shipment_request_pkey` 是
   `PRIMARY KEY (tenant_id, customer_account_id, source, source_request_key)`——`customer_account_id`
   本来就是主键列。迁移 `0006_current_accepted_parcel_projection.sql` 又加了 `declared_parcel_ids text[]`
   与部分 GIN 索引 `shipment_request_accepted_parcels_gin`（谓词 `WHERE state = 2`，即只认已接受）。
   两者在同一张表同一行上，**一次查询同时给出包裹归属与账户**。
2. **读口已经有了，而且交回的就是账户。** `psports.CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel(ctx, tenant, parcel)`
   交回 `psdomain.CurrentAcceptedParcelTarget`；该类型的 `Identity()` 交回 `SourceIdentity`，而
   `SourceIdentity.CustomerAccountID()` 就是货主客户账户。适配器
   `pspostgres.ShipmentRequests.FindCurrentAcceptedByParcel` 已实现，SQL 第一行 `SELECT` 列表就带着
   `customer_account_id`。
3. **不需要任何真实租户参数。** 上面两条全是结构，一条都不依赖真实客户合同、价卡或授权目录。

**所以派工票里那句「PS 半边已闭合、剩下 VE 半边」需要修正一个字**：PS 那一半闭合得比预期更多——旧勘
察 `next-consumer-survey.md` 把方向 37 的欠账记为「缺按包裹/载运对象反查**当事人**（委托或客户账户）这
一层读」，并预判「PS 一票、VE 一票」。实测 ADR-0060 那一票**顺带把账户也带回来了**，因为账户本来就在
委托的来源身份里。VE 侧剩下的不是「补一层反查」，而是「把已有的那层引进来」。

**实例半边是另一件事，不要与它混。** 取回账户之后「这个账户获准看到什么」才是实例半边：
`migrations/visibility_exception/0012_disclosure_policy.sql` 头部原文「内容属实例半边（真实披露范围、
地点粒度、各维内容引用待登记），表在首发是空的」；`ports.DisclosurePolicyView` 第二返回值为 false 即
「披露规则未配置」，`veapplication.dimensionsFrom` 据此把四维**全部**落成待确认，其注释原文「真实披露
范围属待登记实例参数，如实说等；不虚构可见性，也不把没人作过的披露决定说成『不展示』」。

两者不能互相顶：把**机制半边**（这件包裹属于哪个账户）也当实例半边压着，UC-VE-008 就永远接不上；把
**实例半边**（这个账户能看到什么）当机制半边发默认值，就是替商业责任方签字。

## Q4：VE 侧是同形的一件事吗？需要新迁移吗？形状建议

### 不是同形，不需要新迁移

**PS 那一票的形状是「提供方把自己文档里已有的关系升成查询投影列 + 索引」**，前提是 PS **拥有**那层关
系而当时读不出来（ADR-0060 的 Context 原文：「委托聚合今天整份进 `shipment_request.snapshot` jsonb…
没有任何包裹维度的列或索引」）。

**VE 不满足这个前提**：VE 不拥有这层关系（Q3 已证），且提供方读口已经存在（Q3 第 2 条）。VE 侧若同形
地建一张自己的 parcel→account 表，会同时撞两条红线——AGENTS.md 的「所有权清晰」（限界上下文表达数据所
有权）与「单一权威」（一决策一处定义，用例与交接只引用，不复制第二套口径）。

**所以：VE 侧不需要新迁移。** `visibility_exception` schema 下不新增表、不给 `tracking_projection` 加
账户列。给投影加账户列尤其要避免——那会把「投影面向包裹」这条已经写进 `DeriveCustomerViewCommand` 注
释的设计判断反转掉，并且一件包裹在集团委派下可能对多个账户可见，加列等于提前替 PC 的授权模型定形。

### 建议形状（只建议，不实现，PS owner 与 VE owner 各自复核自己那半）

**一、VE ports 新增一个窄只读口。** 位置 `internal/visibilityexception/ports/ports.go`，与
`DisclosurePolicyView` 同类（都是「本上下文不拥有、只消费答复」的口）。形如：

```go
// ParcelCustomerAccountView 回答「这件追踪对象当前属于哪个货主客户账户」。
// 关系归 parcel-shipment 与 party-commercial 拥有（CONTEXT-MAP），本上下文只消费答复，
// 绝不自行推导或缓存第二份映射。
type ParcelCustomerAccountView interface {
    FindCustomerAccount(
        ctx context.Context,
        tenant domain.TenantID,
        parcel domain.TrackedParcelReference,
    ) (domain.CustomerAccountReference, bool, error)
}
```

**二、答案代数要三格，不是两格。** 与 ADR-0060 的零/一/多对齐：

| PS 侧命中 | PS 交回 | VE 侧建议落法 |
|---|---|---|
| 0 行 | `found=false` | 不派生视图，不发明账户。**不是**「无轨迹」，投影照旧存在 |
| 1 行 | `SourceIdentity` 等三重指名 | 取 `CustomerAccountID()`，填 `DeriveCustomerViewCommand.Customer` |
| >1 行 | `psdomain.ErrAmbiguousParcelTarget` | **不得任选**。落未决/待确认，不落成「无视图」 |

第三格有对客语义支撑：UC-VE-008 的 `AT-VE-152`「同一外部标识存在多个有效候选 → 形成标识冲突/待确认，
不任选候选或列出其他客户对象」，与 ADR-0060 的「同包裹被两份已接受委托同时声明，是机制拒绝自动采认」
是同一条纪律的两个面。

**三、适配器落在 VE 侧，不落在 PS 侧。** 按 ADR-0025「跨上下文翻译留在消费方」，新建
`internal/visibilityexception/adapters/parcelshipment/`（今天不存在——VE 的 `adapters/` 下只有
`http`、`identity`、`inbox`、`nodeoperations`、`postgres`、`transportfulfillment`、`veconsume`），在其中
把 `psports.CurrentAcceptedParcelTargetView` 译成上面那个 VE 口，翻译 `TenantID` 与包裹引用的字符串、
把 `ErrAmbiguousParcelTarget` 译成 VE 侧的具名哨兵。参照现有同类：
`internal/visibilityexception/adapters/transportfulfillment/derive_on_effective_delivery.go`（包注释原文
「它只翻译不判断」）。PS 那侧一行都不用改。

### 做之前必须先裁的一件事：`TrackedParcelReference` 与 `DeclaredParcelID` 不是同一个身份空间

这是本票发现的、上面形状建议之外的一个真实风险，**建议在实现票开工前先裁**。

取证 —— `veadapters/transportfulfillment/derive_on_effective_delivery.go` 的 `DeriveOnEffectiveDeliveryAdapter`
注释原文：

> 载运对象引用按 transport-fulfillment 的领域定义可能指正式包裹身份，也可能指集运单元。今天不替它猜：
> 对象号照原样进追踪包裹引用。

代码上确实如此：`projectionCommand` 直接
`vedomain.NewTrackedParcelReference(record.Delivery.Object().String())`，不作任何身份种类判别。

后果：`tracking_projection.parcel_ref` 上可能坐着一个**集运单元号**。拿它去 PS 反查必然 0 行。这不是
bug，但它让 `found=false` 有两种成因——「这不是一件包裹」与「是包裹但当前没有已接受委托」——两者的续
办对象不同（前者问 TF/NO owner 要身份种类，后者等 PS 采认）。

建议：**对外仍不区分**（照 ADR-0029 的探针纪律与 UC-VE-008「不得泄露对象是否存在」），但 VE 侧记录未
派生原因时要能分得开。具体怎么记留给实现票，本票不定，也不替 TF owner 决定要不要在事实里带身份种类。

### 边界重申

即使上述读口做完，**是否登记 `visibility-exception.tracking-projection.derived` 仍是独立一票**。
`cmd/parcel-dispatch/assemble.go` 的 `wireDispatcher` 注释原文：

> 不登记 `visibility-exception.tracking-projection.derived`（UC-VE-008）：派生交接会入队，未登记是
> ADR-0049 认下的 no_subscriber，不是漏接。
> 登记的仍然只有本进程真接得住的类型——按 ADR-0049 第三条，登记一个接不住的比不登记更糟。

「接得住」的判据是 `Customer` 这一维真的填得上。本票只回答了「填得上需要什么」，没有让它填上。

---

## 能力边界

**逐条读过并有取证的：**

- 文档：`AGENTS.md`、`docs/domain/CONTEXT-MAP.md`（全文）、`docs/domain/visibility-exception/CONTEXT.md`
  （按「货主客户账户」检索 + Boundaries 一节）、`docs/domain/party-commercial/CONTEXT.md`（同上）、
  `docs/domain/GLOSSARY.md`（「货主客户账户」条目）、`UC-VE-002`（全文）、`UC-VE-008`（全文）、
  `ADR-0060`（全文）。
- 迁移：`visibility_exception/0007_tracking_projection.sql`、`visibility_exception/0001_customer_view.sql`、
  `visibility_exception/0012_disclosure_policy.sql`、`parcel_shipment/0002_shipment_request.sql`、
  `parcel_shipment/0006_current_accepted_parcel_projection.sql`。
- 代码：`veports/ports.go`（`ProjectionStore`、`ProjectionHandoffIntent`、`DisclosurePolicyView`、
  `CustomerViewStore`、`CustomerViewHandoffIntent`）、`veapplication/derive_customer_view.go`（全文）、
  `vepostgres/customer_view.go`（全文）、`visibilityhttp/query_customer_tracking_view.go`（全文）、
  `veadapters/transportfulfillment/derive_on_effective_delivery.go`（全文）、
  `psports/ports.go` 的 `CurrentAcceptedParcelTargetView` 与 `ShipmentRequestRepository`、
  `pspostgres/current_accepted_parcel_target.go`（全文）、`psdomain/parcel_target.go`（全文）、
  `psdomain/source_submission.go` 的 `SourceIdentity`、`cmd/parcel-api/unwired_orchestration.go`（全文）、
  `cmd/parcel-dispatch/assemble.go` 的 `wireDispatcher` 注释与 `networkIntakeAdoptionGraph`。
- 旧勘察：`.scratch/outbox-handoff-consumption-map/next-consumer-survey.md` 的②′挡与覆盖声明。

**没读的（本票结论不依赖，但列出以便复核者判断保质期）：**

- `UC-VE-003`（ETA 与可见性缺口）、`UC-VE-006`（客户披露与通知）、`UC-PS-003`/`UC-PS-004` 全文。四者都
  被上面的判断间接引用（例如 `AT-VE-158` 的 ETA 门槛、终局展示），但本票只回答账户维的来处，未展开。
- `ADR-0049` 全文（只读了 `assemble.go` 对它第三条的引述）、`ADR-0025`/`ADR-0029`/`ADR-0003` 全文（只
  读了代码注释里的引述）。
- `vedomain` 的 `TrackingProjection`、`CustomerTrackingView` 实现细节（只读了端口与应用层用到的访问
  器）。
- PS 侧 `adopt_on_node_intake.go` / `adopt_on_offsite_pickup.go` / `adopt_on_effective_delivery.go` 三个
  消费适配器的正文——只据 `grep` 确认它们调用 `FindCurrentAcceptedByParcel`，未逐行读。方向 8/16/18/19
  「已落地」这一句我按此二手确认，未独立复验其真库测试。
- `party-commercial` 的客户查询授权/合同委派在代码里的落点（`AT-VE-154` 的集团委派那一格）。我判断它属
  实例半边，但这一句是按 CONTEXT 归纳，未在 `internal/partycommercial/` 里取证。

**保质期：** 现状断言取证于 `7d7e138`。HEAD 前进后，「某个口在不在」「某张表有没有列」这一类要重取证；
引 CONTEXT-MAP / CONTEXT / ADR / UC 原文的部分不受影响。
