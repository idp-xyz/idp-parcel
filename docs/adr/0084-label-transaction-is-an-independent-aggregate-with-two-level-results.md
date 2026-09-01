# ADR-0084: 面单交易是独立聚合——建立即固定覆盖与依据，双层结果一次记录不许互推，定案是派生谓词；继续尝试决定单列登记册不进聚合

Status: Accepted（2026-08-31，批 admin-skeleton-closure 票 08 重启的第一件交付；用户同日频道指示两页骨架完整实现，授权来源句记于票 08 Comments）
Date: 2026-08-31

## Context

管理台 `label-transactions` 页要接真，而全仓没有承载面单交易的表（票 08 取证于 `65b6cf2`：`migrations/parcel_shipment` 十二张表无一承载）。词在 [CONTEXT.md](../domain/parcel-shipment/CONTEXT.md) 里是全的——面单交易、面单交易包裹结果、面单交易定案、面单继续尝试决定、权威业务截断边界——但没有任何代码实现它们。这一票是从建模开始的完整一条线，而聚合边界是难逆转取舍，按 AGENTS.md 先出 ADR 再动手。

边界要答的四问：交易挂不挂在委托聚合下；双层结果（交易级与包裹级）怎么存才不违反「不得压缩成一个无法解释的通用状态」；定案是状态还是派生；受控关闭/重开那一族进不进同一个聚合。另有一件此刻的事实约束：产品基线明写「独立面单渠道服务不进入首发生产」（[首发开发基线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)），[渠道适配缝设计记](../design/channel-adapter-seams-design-note.md)也把面单获取列为渠道墙后的事——**写入方缺席是设计**，本裁决不因为暂时没人写就把不变式的守护者省掉。

## Decision

**一、面单交易是独立聚合，键（租户 + 面单交易标识），不并入 ShipmentRequest。** CONTEXT 原句给了两个方向的多重性：「一笔面单交易可以覆盖一个或多个明确包裹；一个包裹也可以因失败、作废或换单先后关联多笔交易」，且覆盖包裹不限定同一委托。塞进委托聚合，M:N 关系会穿透聚合边界——改一笔跨委托交易要同时锁多份委托；交易的节奏（渠道请求→结果→定案）与委托的节奏（提交→接受）也不同拍。标识由本上下文签发（与 SubmissionVersionID 同族），渠道订单号等外部标识按 CONTEXT「外部标识」规则关联交易、不充当内部身份。

**二、建立即固定：覆盖包裹（至少一件，重复拒绝）、渠道账号、三个参与方角色（渠道账号持有人、渠道服务方、合同与结算相对方）、合同、费率、责任依据快照引用、业务时间，全部是出生属性，此后不变。** 出处是生命周期第一句「建立交易 → 已提交渠道：交易固定所覆盖包裹、渠道账号、参与方角色、合同、费率和责任依据」。重试/替代关系照 `PriorRequestLink` 先例做**新**交易的出生属性（指回被替代交易 + 关系种类 RETRY/REPLACEMENT），原交易一概不动——「原交易 → 被替代」改的是新交易的出处，不是原交易的状态。

**三、双层结果一次记录、只做结构校验、不做跨层推导。** 交易级状态取封闭集合：已建立 → 已提交渠道 →（结果不确定 ⇄）成功 / 部分成功 / 失败。记录渠道结果时**同一次**写入交易级结果与逐包裹结果（每件覆盖包裹恰一条：受理与否、取得的包裹级标识、原因引用），校验只到结构为止——覆盖完备、无越界包裹、无重复；**不**据交易级推包裹级、也不反推（「交易级失败不能推导包裹失败」「不能由一个层次覆盖另一个层次」）。「结果不确定不得直接按失败处理」由状态集合直接交付：它是一格，不是失败的别名。

**四、定案是派生谓词，不是存储状态。** `Finalized() = 交易级结果 ∈ {成功, 部分成功, 失败}`——逐包裹结果的完备性已由 Decision 三在记录时结构保证，所以这一个谓词就是 CONTEXT 定案定义的全部；渠道退款、对账、运营结算明文不属定案条件。存一列「定案状态」会造第二个来源，改结果不改列的一次写入让两处各说各话。投影列 `finalized` 允许存在（查询用），但它是快照的投影而不是权威，写入时与快照同事务同值。

**五、渠道作废、渠道退款、替代是追加式后续动作记录，作用于明确范围（整笔或指名包裹），不改写原结果。** 「可以与未受影响包裹的原结果并存；它们不构成一条覆盖整笔交易的统一线性状态」——聚合上是 append-only 清单，读面把它们并列呈现在受影响包裹的原结果旁边。

**六、面单继续尝试决定不进本聚合，单列追加式决定登记册，本切片不建。** CONTEXT 把它定义为「针对明确包裹当前完整面单服务范围」的版本化决定——范围是包裹跨其全部相关交易，不是某一笔交易的内部状态；授权快照、权威业务截断边界、重开链那一族约束自成一体。写入方（授权业务角色的决定入口）双重在墙后：渠道墙未降、管理台写面也未裁（[.scratch/admin-write-faces/01](../../.scratch/admin-write-faces/issues/01-registry-configuration-has-no-admin-write-face.md) needs-triage）。在登记册落地之前，读面上的「继续尝试判断」按规则原文「只由有效的关闭、重开决定及当前有效终局结果派生」对**空决定集**派生——无生效关闭即开放；登记册另票落地后由联查取代。这不是默认值：是派生规则应用在真实（此刻为空）的决定历史上。

**七、读面按登记册实有维度成形：租户 + 交易，页面行粒度（交易 × 包裹）在读侧由快照摊开。** 本册有租户维（ADR-0003 隔离边界照常），查询签名收（租户, limit）；客户账户不是交易维度（覆盖包裹可跨委托），不上签名。隔离读放行沿用 ADR-0078 同一开关、同一装配点，查阅行注入合成租户；命令面维持不存在——本上下文的写入等渠道墙，照 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 未配置即拒的只有将来才有的写行，本切片根本不开。

**八、持久化照既有纹样：全聚合快照 JSONB + 同行投影列（ADR-0028/0060），写入代数照 ADR-0031（Insert 的「已存在」与 Save 的「版本冲突」是业务答案不是错误）。** 重建逐字段过领域构造函数，坏行在门上暴露。

## Consequences

- 票 08 范围落定：`internal/parcelshipment/domain/label_transaction.go`（建立/提交/结果/后续动作/定案/重建 + 测试）、`migrations/parcel_shipment/0010_label_transaction.sql`、`ports.go` 增仓储与查阅两口、`adapters/postgres` 双适配器 + 真库测试、`adapters/http` 查阅端点、`cmd/parcel-api` 装配、页面接真。
- 写编排（随委托提交产生交易、渠道适配器回填结果）等渠道墙降后另票；继续尝试决定登记册（含截断边界与重开链建模）另票，重启条件是写面裁决或渠道墙任一先到。
- 页面「继续尝试判断」列在登记册落地前恒为「开放」，页头注明派生依据，防止读成「已核对过关闭册」。

## Alternatives considered

- **并进 ShipmentRequest 聚合。** 否决，见 Decision 一：M:N 穿透边界，跨委托覆盖直接违反单聚合一致性边界。
- **拆成「包裹交易参与」聚合（逐包裹一份）。** 否决：交易级结果没有家——「同时保存整笔交易结果」会退化成逐包裹复制，第一层压缩正是硬句禁止的。
- **只建读投影表、不建聚合。** 否决：双层结果完备性与定案派生没有守护者，渠道墙一降写入方就得对着裸表重新发明不变式，届时已有历史行没人担保形状。
- **「定案」「继续尝试判断」入列存储。** 否决，见 Decision 四/六：与快照或决定册两处各说各话；后者 CONTEXT 明写「只由……派生」。

## Links

- [CONTEXT.md](../domain/parcel-shipment/CONTEXT.md)：面单交易一族术语、`Rules and invariants` 里「多包裹面单交易可以具有共同交易结果」与「交易级失败不能推导包裹失败」两族硬句、生命周期「面单交易」「面单继续尝试」——本裁决全部硬句的出处
- [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)、[ADR-0060](./0060-parcel-lookup-uses-current-snapshot-projection.md)、[ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：持久化与写入代数纹样
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)：未配置即拒与隔离读准入，Decision 七的两半
- [渠道适配缝设计记](../design/channel-adapter-seams-design-note.md)、[首发开发基线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：写入方缺席是设计的出处
- [票 08](../../.scratch/admin-skeleton-closure-batch/issues/08-label-transaction-domain-modeling.md)：本裁决的驱动票与重启范围
