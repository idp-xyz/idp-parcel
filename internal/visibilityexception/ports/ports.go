// Package ports 定义 visibility-exception 应用层与外界的边界。领域包不依赖 HTTP/pgx
// 的纪律与其余上下文一致。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

type Clock interface {
	Now() time.Time
}

// FactKey 是已接受源事实的幂等键：租户+来源上下文+事实引用+来源版本。租户是最高
// 数据隔离边界（ADR-0003）——VE 消费多个源上下文的事实，事实引用只在各自租户的源
// 上下文内唯一，缺租户维两个租户的同名引用就会共用一份事实。
type FactKey struct {
	Tenant  domain.TenantID
	Source  domain.SourceContext
	Fact    domain.SourceFactReference
	Version domain.SourceFactVersion
}

// FactRecord 是一份已保存的事实及其内容指纹（同键异内容是来源冲突不是重放）。
type FactRecord struct {
	Key           FactKey
	ContentDigest string
	Fact          domain.AcceptedSourceFact
}

type FactSaveOutcome uint8

const (
	FactSaveOutcomeInvalid FactSaveOutcome = iota
	FactSaved
	FactAlreadyRecorded
)

// AcceptedFactStore 按幂等键找回并保存事实；FindByParcel 交回该租户下该包裹全部已
// 接受事实——投影派生的输入，跨租户的同名包裹引用互不可见。事实只增不删（来源更正
// 是新版本新键）。
type AcceptedFactStore interface {
	FindByKey(ctx context.Context, key FactKey) (FactRecord, bool, error)
	FindByParcel(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
	) ([]FactRecord, error)
	Save(ctx context.Context, record FactRecord) (FactSaveOutcome, error)
}

// MilestoneAnswer 是映射目录对一份事实的归类答复。
type MilestoneAnswer struct {
	Milestone  domain.MilestoneReference
	Classified bool
	Mapping    domain.MappingVersionReference
}

// MilestoneMappingView 按版本化映射归类事实。第二个返回值为 false 即「映射目录未
// 配置」——那不是未决而是整体无法可靠映射，事实按未归类进投影（CONTEXT：不强行
// 映射）；依赖调不通作为错误返回。
type MilestoneMappingView interface {
	ClassifyFact(ctx context.Context, fact domain.AcceptedSourceFact) (MilestoneAnswer, bool, error)
}

// ProjectionStore 保存追踪投影版本。版本只增不改写（ADR-0065）：Save 追加新版本行
// 并把当前标记指向它，原版本连同条目、所用映射版本与派生时间一并留存；FindCurrent
// 读当前标记指名的那一版，不退化为对历史的扫描；FindByVersion 按版本读回留存的任
// 一版——审计问「当时形成过什么」由它作答，重放只能答「今天会派生出什么」。
// 租户是最高数据隔离边界（ADR-0003）：TrackedParcelReference 只是字符串引用，缺
// 租户维两个租户的同名包裹就会共用一份投影。
type ProjectionStore interface {
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
	) (domain.TrackingProjection, bool, error)
	FindByVersion(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.ProjectionVersionID,
	) (domain.TrackingProjection, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, projection domain.TrackingProjection) error
}

// OperationsProjectionRead 是运营追踪查阅的读面(ADR-0076、CONTEXT「运营追踪查阅」):
// 当前投影列表、单件当前版与按版本读回留存版本(ADR-0065 的审计口)。查阅只读——
// 不形成新投影版本,也不产生披露决定、通知或任何业务事实(CONTEXT 生命周期句),
// 所以它是存储读面,不是编排的门。
//
// 键只含租户维,无客户维:投影是租户内部对象,运营查阅的授权边界只有租户;租户在
// 签名上看得见,与 ProjectionStore 现有方法同派。FindCurrent 与 FindByVersion 和
// ProjectionStore 同签名,由同一存储适配器一并作答;ListCurrent 是查阅面独有的列表
// 读法,派生编排用不到它,故不并进写侧接口——扩写侧接口会让每个写侧替身都被迫
// 长出一个列表方法。
//
// Limit 必须为正;每页多大由接入面按渠道契约裁决,读口只拒绝无意义的取值。
type OperationsProjectionRead interface {
	ListCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]domain.TrackingProjection, error)
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
	) (domain.TrackingProjection, bool, error)
	FindByVersion(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.ProjectionVersionID,
	) (domain.TrackingProjection, bool, error)
}

// ProjectionIdentityFactory 签发投影版本标识。
type ProjectionIdentityFactory interface {
	NextProjectionVersionID(ctx context.Context) (domain.ProjectionVersionID, error)
}

// ProjectionHandoffIntent 把新派生的投影交给适用下游（客户视图派生正是消费者）。
// 意图由投影版本认领，重放重发同一份（ADR-0043）。租户随意图到达（ADR-0003）：投影
// 对象没有租户维，下游按（租户+包裹）查库。
type ProjectionHandoffIntent struct {
	TenantID   domain.TenantID
	Projection domain.TrackingProjection
}

// ProjectionHandoff 把投影版本写入 Outbox（`OutboxProjectionHandoff`）。信封 ID 由
// 投影版本认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type ProjectionHandoff interface {
	HandOffProjection(ctx context.Context, intent ProjectionHandoffIntent) error
}

// ParcelCustomerAccountView 回答「这件追踪对象当前属于哪个货主客户账户」——客户
// 视图派生缺的账户维（DeriveCustomerViewCommand.Customer）唯一的自动来处。关系本体
// 不归本上下文：parcel-shipment 拥有包裹与委托的成员关系，party-commercial 拥有账户
// 身份（CONTEXT-MAP），这里只消费答复，绝不自行推导或缓存第二份映射。
//
// 答案三格与 ADR-0060 的零/一/多对齐。第二个返回值为 false 即「当前没有已接受委托
// 声明这件对象」——不派生视图、不发明账户，投影照旧存在。false 不分成因：追踪包裹
// 引用可能装着集运单元号（TF 侧不替对象猜身份种类），反查零行不是缺陷；成因区分留给
// 接线票。多于一个候选是未决/待确认，不是「无视图」（UC-VE-008 AT-VE-152）——适配器
// 以具名错误交回，绝不按时间或行序任选。依赖调不通作为错误返回。
type ParcelCustomerAccountView interface {
	FindCustomerAccount(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
	) (domain.CustomerAccountReference, bool, error)
}

// DimensionDisclosure 是披露策略对客户视图一维的答复：获准展示时带内容来处，
// 内容尚未到位时待确认，授权或披露规则不允许时不展示。State 取领域的封闭三态，
// 翻译成维度由编排经领域构造器完成——展示无内容在那里立不起来。
type DimensionDisclosure struct {
	State   domain.DimensionState
	Content domain.ViewContentReference
}

// DisclosureAnswer 是披露策略对四个展示维各自的答复（公开里程碑、获准展示的 ETA、
// 客户服务终局、追踪说明）。
type DisclosureAnswer struct {
	Milestones DimensionDisclosure
	ETA        DimensionDisclosure
	Final      DimensionDisclosure
	Note       DimensionDisclosure
}

// DisclosurePolicyView 回答「对这个货主客户账户与这份投影，四维各自获准展示什么」。
// 客户可见性按服务产品、客户合同和信息披露规则形成（CONTEXT），那些都不属本上下文——
// 这里只消费答复，绝不自行推导一份。
//
// 第二个返回值为 false 即「披露规则未配置」——真实披露范围、地点粒度与通知策略属
// 待登记实例参数，没有租户就没有规则。那不是未决而是如实的空白：四维全部待确认，
// 如实说等，不虚构可见性，也不把没人作过的披露决定说成「不展示」。依赖调不通作为
// 错误返回，由应用层形成未决。
type DisclosurePolicyView interface {
	AssessDisclosure(
		ctx context.Context,
		customer domain.CustomerAccountReference,
		projection domain.TrackingProjection,
	) (DisclosureAnswer, bool, error)
}

// CustomerViewStore 保存客户当前视图版本。键含租户与客户账户——租户是最高数据隔离
// 边界（ADR-0003），跨越它必须在签名上看得见；账户隔离是字段不是约定，按包裹一个键
// 会让两个客户的授权范围共用一份视图。原版本由替代关系承担历史，库只管当前。
type CustomerViewStore interface {
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.CustomerAccountReference,
		parcel domain.TrackedParcelReference,
	) (domain.CustomerTrackingView, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, view domain.CustomerTrackingView) error
}

// CustomerViewIdentityFactory 签发客户视图版本标识。与投影身份工厂分开：投影派生与
// 视图派生由不同用例触发，合并会让一个编排依赖它根本不签发的身份。
type CustomerViewIdentityFactory interface {
	NextCustomerViewVersionID(ctx context.Context) (domain.CustomerViewVersionID, error)
}

// CustomerViewHandoffIntent 把新发布的客户视图交给适用下游（门户展示、通知判断的
// 输入）。意图由视图版本认领，重放重发同一份（ADR-0043）。租户随意图到达（ADR-0003）：
// 视图对象没有租户维，下游按（租户+客户+包裹）查库。
type CustomerViewHandoffIntent struct {
	TenantID domain.TenantID
	View     domain.CustomerTrackingView
}

// CustomerViewHandoff 把客户视图写入 Outbox（`OutboxCustomerViewHandoff`）。信封 ID
// 由视图版本认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type CustomerViewHandoff interface {
	HandOffCustomerView(ctx context.Context, intent CustomerViewHandoffIntent) error
}

// TriageQuery 是分诊规则的输入：命中事实自带的对象、类型、信号规则版本与可信度。
// 全部取自命中事实，编排不自造——「每个信号必须保存对象、类型、规则版本、判断时间、
// 事实依据、可信度」（CONTEXT）里的哪一样都不该由本上下文补默认值。
type TriageQuery struct {
	Kind       domain.ExceptionSignalKindReference
	Parcel     domain.TrackedParcelReference
	Rule       domain.SignalRuleVersionReference
	Confidence domain.ConfidenceReference
}

// TriageAnswer 是版本化分诊规则的答复：四走向之一与所命中的分诊规则版本。
//
// Team 只随`自动建案`走向在场：「建立案件 → 固定根对象、初始影响范围、责任团队」
// （CONTEXT 生命周期），而「每个开放案件始终必须有一个内部案件责任团队」——规则说
// 自动建案却不说归谁，案件就建不起来。团队因此是自动建案条目登记时必带的一维
// （`PAR-VIS-05`），其余三走向不带；编排把「自动建案无团队」当端口坏答复上抛，不补。
type TriageAnswer struct {
	Outcome domain.TriageOutcome
	Rule    domain.SignalRuleVersionReference
	Team    domain.ResponsibleTeamReference
}

// TriageRuleView 回答「这个信号按版本化分诊规则该走哪一格」。「高可信、高影响且命中
// 版本化分诊规则的信号可以自动建立或关联案件」（CONTEXT）——自动建案只能来自这里的
// 规则命中。
//
// 第二个返回值为 false 即「分诊规则未配置」——真实分诊规则属待登记实例参数。那不是
// 未决而是如实的空白：信号进人工复核格，不自动建案也不装作没有信号。依赖调不通作为
// 错误返回，由应用层形成未决。
type TriageRuleView interface {
	TriageSignal(ctx context.Context, query TriageQuery) (TriageAnswer, bool, error)
}

// RaisedSignalRecord 是开启或重开发作期越过提交边界的最小单元：发作期与它的分诊结论
// 同一提交。只落发作期不落结论，重试会走进「已有活跃发作期」那一支去记命中，结论就
// 永远补不上了。租户、对象与类型随记录携带——发作期聚合不导出它们，没有这几维
// 适配器连存储键都立不起来（与 TriageHandoffIntent 同理）。
//
// Case 与结论走向成对：走向为`自动建案`时必带案件，其余走向必不带。案件与结论同一
// 提交是同一条理由——只落结论不落案件，重放同样走进「已有活跃发作期」那一支，一份
// 写着 AUTO_ESTABLISH 的结论后面就永远没有案件。实现在写入前核这一对，不核就落库
// 的是一份自相矛盾的记录。
type RaisedSignalRecord struct {
	Tenant     domain.TenantID
	Parcel     domain.TrackedParcelReference
	Kind       domain.ExceptionSignalKindReference
	Episode    *domain.SignalEpisode
	Conclusion domain.TriageConclusion
	Case       *domain.ExceptionCase
}

// SignalEpisodeStore 按租户+对象+类型找回最近一次发作期并保存。租户是最高数据隔离
// 边界（ADR-0003），跨越它必须在签名上看得见。最近一次含已结束的——「已结束+再命中」
// 要据它建立关联的新发作期，只查活跃会把重开误判成首启。
type SignalEpisodeStore interface {
	FindLatest(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
		kind domain.ExceptionSignalKindReference,
	) (*domain.SignalEpisode, bool, error)
	SaveRaised(ctx context.Context, record RaisedSignalRecord) error
	SaveHit(ctx context.Context, tenant domain.TenantID, episode *domain.SignalEpisode) error
}

// SignalEpisodeIdentityFactory 签发发作期标识。与视图、投影身份工厂分开，理由相同：
// 不同用例触发，合并会让一个编排依赖它根本不签发的身份。
type SignalEpisodeIdentityFactory interface {
	NextEpisodeID(ctx context.Context) (domain.EpisodeID, error)
}

// CaseIdentityFactory 签发异常案件标识。与发作期身份工厂分开：信号与案件始终是两个
// 对象（CONTEXT「异常信号与异常案件始终保持独立」），共用一个签发器会让案件借发作期
// 的号，两份历史从此按同一串编号互相指错。
type CaseIdentityFactory interface {
	NextCaseID(ctx context.Context) (domain.CaseID, error)
}

// TriageHandoffIntent 把分诊结论交给适用下游（建案、复核队列的输入）。意图由租户加
// 对象加类型认领（与 FindLatest 键一致），重放重发同一份（ADR-0043）。租户随意图到达
// （ADR-0003）：下游按（租户+包裹+类型）查库；信封 ID 缺租户维时两租户同包裹+类型
// 会合成一份。
type TriageHandoffIntent struct {
	TenantID   domain.TenantID
	Parcel     domain.TrackedParcelReference
	Kind       domain.ExceptionSignalKindReference
	Conclusion domain.TriageConclusion
}

// TriageHandoff 把分诊结论写入 Outbox（`OutboxTriageHandoff`）。信封 ID 由租户加对象
// 加类型认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type TriageHandoff interface {
	HandOffTriage(ctx context.Context, intent TriageHandoffIntent) error
}

// ExceptionDisclosureRule 是异常披露规则目录对（货主客户账户 + 信号类型 + 可信度）的
// 答复——`UC-VE-006` 前半「判断是否满足披露条件」与「默认等待授权角色确认；只有批准
// 范围才形成自动发布决定」两步各取一格：
//   - Disclosable 为假：合同不要求对该客户披露这类异常（`AT-VE-099`），决定落`暂不披露`；
//   - Disclosable 为真、AutoRelease 为假：影响明确但需授权（`AT-VE-100`），决定落`待授权`；
//   - 两者皆真：批准范围允许自动发布（`AT-VE-102`），决定落`披露`并带内容快照引用。
//
// Content 只随 Disclosable 在场：披露必带内容快照、不披露必不带，与 DecideDisclosure
// 的构造门同一条线。Policy 是这一版规则的引用，决定带着它走，通知策略按它取渠道与时限。
//
// 「客户可见性必须根据服务产品、客户合同、信号可信度、影响范围、预计客户影响和信息
// 披露规则形成版本化决定」（CONTEXT）——这里按客户账户（合同的落点）、信号类型与可信度
// 查规则；影响范围与预计客户影响今天没有可查的登记维，等它们有形状再扩键，不先替租户
// 拟一格。
type ExceptionDisclosureRule struct {
	Policy      domain.DisclosurePolicyReference
	Disclosable bool
	AutoRelease bool
	Content     domain.DisclosureContentReference
}

// ExceptionDisclosureRuleView 回答「对这个客户与这类信号，按当前适用的披露规则该作什么
// 决定」。规则内容属实例半边（`PAR-VIS-07` 待提供）：第二个返回值为 false 即「未配置」
// ——目录整个没发布，或发布了但这个（客户+类型+可信度）没有条目，两者都交回未配置：没有
// 默认披露值可发明，「还没写到这个账户」不能读成「这个账户什么都不披露」。依赖调不通
// 作为错误返回，由应用层形成未决。
type ExceptionDisclosureRuleView interface {
	RuleForSignal(
		ctx context.Context,
		customer domain.CustomerAccountReference,
		kind domain.ExceptionSignalKindReference,
		confidence domain.ConfidenceReference,
	) (ExceptionDisclosureRule, bool, error)
}

// ExceptionDisclosureRuleEntry 是一条异常披露条目：某客户账户的某类信号在某可信度下
// 披露与否、能否自动发布、内容快照从哪来。Content 与 Disclosable 成对（披露必带、不披露
// 必不带），AutoRelease 只在 Disclosable 为真时可为真——不披露就谈不上自动发布。
type ExceptionDisclosureRuleEntry struct {
	Customer    domain.CustomerAccountReference
	Kind        domain.ExceptionSignalKindReference
	Confidence  domain.ConfidenceReference
	Disclosable bool
	AutoRelease bool
	Content     domain.DisclosureContentReference
}

// ExceptionDisclosureRuleRegistration 登记一版异常披露规则：抬头加整版条目，条目纪律同
// 映射登记。版本引用（Header.Version）就是决定与通知策略共用的那个披露策略引用：决定
// 带着它走，`notification_policy` 按它取渠道与时限——两张目录靠这一个值接上。
type ExceptionDisclosureRuleRegistration struct {
	Header  CatalogVersionHeader
	Entries []ExceptionDisclosureRuleEntry
}

// ExceptionDisclosureRuleRegistry 是异常披露规则目录的写入口（`PAR-VIS-07` 的披露与自动
// 发布范围那一半；渠道那一半在 CatalogRegistry.RegisterNotificationPolicy）。
//
// 单立端口而不并进 CatalogRegistry：那个接口每加一口就拆一遍它全部的测试
// 替身与受控 CLI 的桩；本目录的登记入口（CLI / 在线登记口）另立票接，先把写入方与读口
// 立起来。事务纪律同 CatalogRegistry：在调用方的事务内执行，一版抬头与整版条目同一提交。
type ExceptionDisclosureRuleRegistry interface {
	RegisterExceptionDisclosureRules(
		ctx context.Context,
		tenant domain.TenantID,
		registration ExceptionDisclosureRuleRegistration,
	) (CatalogRegistrationOutcome, error)
}

type DisclosureDecisionSaveOutcome uint8

const (
	DisclosureDecisionSaveOutcomeInvalid DisclosureDecisionSaveOutcome = iota
	DisclosureDecisionSaved
	DisclosureDecisionAlreadyRecorded
)

// DisclosureDecisionStore 保存披露决定并按（发作期+客户账户）取回当前那一份。三态结论
// 全部入册——「披露条件不成立或授权不足时分别形成暂不披露或待授权结果」（CONTEXT），
// 暂不披露与待授权是已作出的决定，不是没有决定；不记它们，同一发作期就会被反复重判。
// 租户是最高数据隔离边界（ADR-0003），跨越它必须在签名上看得见。
//
// 决定按身份三维（客户、发作期、决定时间）成行，与 CustomerNotificationStore 定位通知
// 的三维同一口径；FindCurrent 交回同（发作期+客户）下决定时间最晚的那一份。Save 撞
// 三维主键交回 AlreadyRecorded（ADR-0031 写入代数），事务保持可用。
type DisclosureDecisionStore interface {
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		episode domain.EpisodeID,
		customer domain.CustomerAccountReference,
	) (domain.DisclosureDecision, bool, error)
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		decision domain.DisclosureDecision,
	) (DisclosureDecisionSaveOutcome, error)
}

// NotificationDirective 是通知策略对一份披露决定的答复：适用渠道、要求时限与义务判据。
// 「客户异常通知必须保存……要求时限和适用渠道」（CONTEXT）——三样都来自版本化通知
// 策略，编排不补默认值。
type NotificationDirective struct {
	Channel    domain.NotificationChannelReference
	Deadline   time.Time
	Obligation domain.DisclosurePolicyReference
}

// NotificationPolicyView 回答「这份披露按客户合同与通知策略该走什么渠道、限时多少」。
// 门户展示能否满足通知义务同样由它背后的合同判断，本上下文不自行推导。
//
// 第二个返回值为 false 即「通知策略未配置」——渠道与时限目录属待登记实例参数。没有
// 渠道的通知不存在一个如实的空白格：造一个占位渠道是虚构，所以未配置由编排形成未决，
// 与依赖调不通分开（一个等租户登记，一个重试依赖）。
type NotificationPolicyView interface {
	DirectNotification(
		ctx context.Context,
		disclosure domain.DisclosureDecision,
	) (NotificationDirective, bool, error)
}

type NotificationSaveOutcome uint8

const (
	NotificationSaveOutcomeInvalid NotificationSaveOutcome = iota
	NotificationSaved
	NotificationAlreadyRecorded
)

// CustomerNotificationStore 按披露决定找回并保存通知。披露决定没有自有标识，按其身份
// 三维（客户、发作期、决定时间）定位——同一披露不重发通知的幂等界线就立在这里。
// 租户是最高数据隔离边界（ADR-0003），跨越它必须在签名上看得见：客户账户引用只在
// 租户内唯一，缺租户维两个租户的同名客户就会共用一份通知。
//
// Save 的写入代数同 ADR-0031：过程节点回填走主键 UPSERT；首发撞披露身份三维唯一
// 约束交回 AlreadyRecorded（事务保持可用，编排读回赢家）。两条冲突路径不压成一个
// ON CONFLICT。
type CustomerNotificationStore interface {
	FindByDisclosure(
		ctx context.Context,
		tenant domain.TenantID,
		disclosure domain.DisclosureDecision,
	) (*domain.CustomerNotification, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, notification *domain.CustomerNotification) (NotificationSaveOutcome, error)
}

// NotificationIdentityFactory 签发通知标识。与其余身份工厂分开，理由相同。
type NotificationIdentityFactory interface {
	NextNotificationID(ctx context.Context) (domain.NotificationID, error)
}

// NotificationChannelGateway 把通知提交给适用消息渠道。真实渠道与其凭证属实例参数，
// 今天没有实现，唯一实现是测试替身。提交失败作为错误返回，由编排记为`失败`节点——
// 那是要分别记录的过程结果（CONTEXT），不是未决。
type NotificationChannelGateway interface {
	SubmitToChannel(ctx context.Context, notification *domain.CustomerNotification) error
}

// NotificationHandoffIntent 把通知决定交给适用下游（义务台账、升级判断的输入）。意图
// 由通知标识认领，重放重发同一份（ADR-0043）。租户随意图到达（ADR-0003）：通知对象
// 没有租户维，下游按（租户+披露身份）查库。
type NotificationHandoffIntent struct {
	TenantID     domain.TenantID
	Notification *domain.CustomerNotification
}

// NotificationHandoff 把通知决定写入 Outbox（`OutboxNotificationHandoff`）。信封 ID
// 由通知标识认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type NotificationHandoff interface {
	HandOffNotification(ctx context.Context, intent NotificationHandoffIntent) error
}

// ActiveCaseView 回答案件是否在场且未关闭。案件本体的读写归案件编排，这里只要一个
// 在场判据：处置请求只能挂在活案件下——「案件范围缩小、改派、归并或关闭前必须盘点
// 全部未完成处置请求」（CONTEXT），关闭后再挂新请求就是绕过那次盘点。
type ActiveCaseView interface {
	CaseActive(ctx context.Context, caseID domain.CaseID) (active bool, found bool, err error)
}

type DispositionSaveOutcome uint8

const (
	DispositionSaveOutcomeInvalid DispositionSaveOutcome = iota
	DispositionSaved
	DispositionAlreadyRecorded
)

// DispositionRequestStore 按稳定身份与幂等键找回并保存处置请求。租户是最高数据隔离
// 边界（ADR-0003），跨越它必须在签名上看得见。
//
// FindCurrent 按（案件+动作+范围）交回当前那份——未被替代的请求；同键重复到达据它
// 短路，不重发。Save 的写入代数同 ADR-0031：首发撞部分唯一索引交回 AlreadyRecorded
// （事务保持可用，编排读回赢家）；判断/取消回填走主键 UPSERT，两条冲突路径不压成
// 一个 ON CONFLICT。SaveSupersession 把被替代者与后继同一提交：只落一半，替代关系
// 与新意图会各说各话。
type DispositionRequestStore interface {
	FindByID(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.DispositionRequestID,
	) (*domain.DispositionRequest, bool, error)
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		caseID domain.CaseID,
		action domain.RequestedActionReference,
		scope domain.RequestScopeReference,
	) (*domain.DispositionRequest, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, request *domain.DispositionRequest) (DispositionSaveOutcome, error)
	SaveSupersession(
		ctx context.Context,
		tenant domain.TenantID,
		prior, successor *domain.DispositionRequest,
	) error
}

// DispositionRequestIdentityFactory 签发处置请求标识。与其余身份工厂分开，理由相同。
type DispositionRequestIdentityFactory interface {
	NextDispositionRequestID(ctx context.Context) (domain.DispositionRequestID, error)
}

// DispositionHandoffIntent 把已成立的处置请求交给发送侧下游。真实目标上下文的受理在
// 对方——这里只是发送意图，由请求标识认领，重放重发同一份（ADR-0043）。租户随意图到达
// （ADR-0003）：请求对象本身没有租户维，下游按（租户+请求标识）查库。
type DispositionHandoffIntent struct {
	TenantID domain.TenantID
	Request  *domain.DispositionRequest
}

// DispositionHandoff 把发送意图写入 Outbox（`OutboxDispositionHandoff`）。信封 ID
// 由请求标识认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type DispositionHandoff interface {
	HandOffDispositionRequest(ctx context.Context, intent DispositionHandoffIntent) error
}

// ETAStore 按（租户+包裹+里程碑）保存当前预测版本。历史版本由 Refresh 的指回关系
// 承担，库只管当前。租户是最高数据隔离边界（ADR-0003）：TrackedParcelReference 只
// 是字符串引用，缺租户维两个租户的同名包裹就会共用一份预测。
type ETAStore interface {
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
		milestone domain.MilestoneReference,
	) (domain.ETAPrediction, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, eta domain.ETAPrediction) error
}

// ETAIdentityFactory 签发预测版本标识。与其余身份工厂分开，理由相同。
type ETAIdentityFactory interface {
	NextETAVersionID(ctx context.Context) (domain.ETAVersionID, error)
}

// ETAHandoffIntent 把新预测版本交给适用下游（客户视图链的重派生输入——「ETA 新版本
// ……必须重新派生客户视图」）。意图由预测版本认领，重放重发同一份（ADR-0043）。租户
// 随意图到达（ADR-0003）：预测对象没有租户维，下游按（租户+包裹+里程碑）查库。
type ETAHandoffIntent struct {
	TenantID   domain.TenantID
	Prediction domain.ETAPrediction
}

// ETAHandoff 把预测版本写入 Outbox（`OutboxETAHandoff`）。信封 ID 由预测版本认领，
// 入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type ETAHandoff interface {
	HandOffETA(ctx context.Context, intent ETAHandoffIntent) error
}

// VisibilityGapStore 按（租户+包裹+预期观察+窗口规则版本）保存缺口。键含窗口规则
// 版本：「新窗口版本生效→后续采用新版本，原判断保留」——同一预期在新旧规则下是两
// 次独立判断，压成一个键会让新版本覆盖原判断。租户是最高数据隔离边界（ADR-0003），
// 跨越它必须在签名上看得见。
type VisibilityGapStore interface {
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
		expectation domain.ExpectedObservationReference,
		windowRule domain.ObservationWindowReference,
	) (domain.VisibilityGap, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, gap domain.VisibilityGap) error
}

// VisibilityGapHandoffIntent 把已成立的缺口交给信号链（缺口是否命中异常规则由分诊
// 判断，本编排不替它判）。意图由缺口身份三维认领，重放重发同一份（ADR-0043）。租户
// 随意图到达（ADR-0003）：缺口对象没有租户维，下游按（租户+包裹+预期+窗口规则）查库。
type VisibilityGapHandoffIntent struct {
	TenantID domain.TenantID
	Gap      domain.VisibilityGap
}

// VisibilityGapHandoff 把已成立的缺口写入 Outbox（`OutboxVisibilityGapHandoff`）。
// 信封 ID 由缺口身份三维认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份
// （ADR-0043）。
type VisibilityGapHandoff interface {
	HandOffVisibilityGap(ctx context.Context, intent VisibilityGapHandoffIntent) error
}

type ClaimSaveOutcome uint8

const (
	ClaimSaveOutcomeInvalid ClaimSaveOutcome = iota
	ClaimSaved
	// ClaimRevisionConflict 说明另一方已经把这项索赔推进过了：本次写入这条路走通了，
	// 只是手里的快照不再是当前那一版。它是业务答案而不是 error（ADR-0031）——调用方
	// 要重读再重放，而 error 那一格的恢复动作是重试同一份，两者不同。
	ClaimRevisionConflict
)

// ClaimStore 按（租户+批次+项）找回并保存索赔项。键含租户：租户是最高数据隔离边界
// （ADR-0003），批次引用只在租户内唯一。键含批次：项标识由客户提交侧建立，批次内
// 唯一是它的口径，跨批次撞号不该互相干扰。
//
// Save 的写入代数同 ADR-0031：预期修订由索赔项自己携带（`ClaimItem.Revision()` 是它
// 被读出时的那一版，三判转移一律不动它），不符即交回 ClaimRevisionConflict，事务保持
// 可用，编排读回赢家再作答。三判分步意味着同一项索赔的每一步都经这一个入口落库，
// 于是资格审核与撤回、复核与延期这些并发对能各自从旧快照出发——没有这一格，后写者的
// 整行重写会把前一个转换悄悄抹掉，两边都以为自己成功。
// CountLiveScopeClaims 数同租户下与本项同（客户账户+目标范围+索赔类型）的其他索赔
// 项，供资格审核的重复关系那一维取事实。它只数事实，不判后果——重复成立意味着什么
// 由规则说了算。
//
// 「其他」按项标识排除本项自己：重判一项已在办的索赔不该把它数成自己的重复。已撤回
// 的不数：`AT-VE-123`「客户撤回后重新提交同一范围」明写要建立新索赔项并重新检查重复
// 关系，把撤回那项算进来，同一范围就再也提不了第二次。其余状态一律数进来——已不予
// 受理的算不算重复是一次尚未作出的领域裁断，在这里先替它拍板会把裁断藏进一个计数。
type ClaimStore interface {
	FindByBatchItem(
		ctx context.Context,
		tenant domain.TenantID,
		batch domain.ClaimBatchReference,
		item domain.ClaimItemID,
	) (*domain.ClaimItem, bool, error)
	CountLiveScopeClaims(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.CustomerAccountReference,
		target domain.RequestScopeReference,
		kind domain.ClaimKindReference,
		excluding domain.ClaimItemID,
	) (int, error)
	Save(ctx context.Context, tenant domain.TenantID, claim *domain.ClaimItem) (ClaimSaveOutcome, error)
}

// EligibilityQuery 是资格规则的查找键：按客户账户、合同版本、目标范围与索赔类型
// 找出适用的那一版规则。Tenant 随查询到达（ADR-0003 显式入参一族）：目录行只在租户
// 内成立，多租户入口的读适配器凭它按册作答；把租户钉在构造期的形状留给受控登记口，
// 两个形状不合并（.scratch/ve-claims-read-seams/01——合并会让登记口拿到跨租户读）。
// Applicant 也是查找键——授权名单可按它收窄到相关行；存量索赔未带申请人时为零值，
// 实现照常答目录登记情况，缺席那一维由编排如实停下。除此之外查询不带事实——事实由
// 编排另取（见 ClaimStore 与 ClaimEvidenceView），目录只答规则是什么。
type EligibilityQuery struct {
	Tenant    domain.TenantID
	Batch     domain.ClaimBatchReference
	Item      domain.ClaimItemID
	Customer  domain.CustomerAccountReference
	Contract  domain.ContractScopeReference
	Target    domain.RequestScopeReference
	Kind      domain.ClaimKindReference
	Applicant domain.ApplicantReference
}

// FilingDeadlineRule 是首次索赔期限规则。CONTEXT 要求每个期限保存适用规则版本、
// 起算事件、业务时区或日历、截止时间和适用范围——五样是一体的，缺一这条期限就算
// 不出来。Registered 为假时其余字段一律不看：编排据此如实答未登记，不拿本方时钟
// 凑一个默认时限，那会把「租户还没登记」变成一次有依据的超期拒赔。
type FilingDeadlineRule struct {
	Registered  bool
	RuleVersion string
	StartEvent  string
	Calendar    string
	Scope       string
	Deadline    time.Time
}

// MinimumMaterialsRule 是最低材料要求：这一索赔类型在这一版规则下必须齐备的材料。
//
// Required 里每一项都是目录签发的材料引用，编排只拿它与已收到的作差集——差出来的
// 缺口因此全由目录的词构成。Notice 与 SupplementDeadline 是资料不足时四件落点里由
// 规则决定的那两件（通知依据、当前截止）；缺少材料由差集得出，补充范围取索赔自己
// 固定的目标范围。四件凑不齐就停在未决，不记一个残缺的第三态。
type MinimumMaterialsRule struct {
	Registered         bool
	RuleVersion        string
	Required           []domain.MaterialRequirementReference
	Notice             domain.SupplementNoticeReference
	SupplementDeadline time.Time
}

// AuthorizationCatalogue 是申请人授权目录：登记情况、版本与授权名单。
//
// 名单语义只有一条：在列即该申请人获此客户账户的索赔提交授权（`AT-VE-125` 把申请人
// 授权与客户账户并列——两者不是一回事，账户对不对是另一维）。实现可以按查询里的
// 申请人把名单收窄到相关那一行，收窄不改语义，编排仍按「在不在列」核对。Registered
// 为假时名单不看：目录未登记是 `PAR-VIS-08` 待提供的实例参数，空名单在那时不是
// 「无人获授权」而是「还没登记」，两者的恢复动作不同。
type AuthorizationCatalogue struct {
	Registered           bool
	RuleVersion          string
	AuthorizedApplicants []domain.ApplicantReference
}

// EligibilityRules 是资格目录交出的规则本体。它答「规则是什么」，不答「这项索赔过
// 不过审」——后者要拿规则去核对事实，而重复关系在 ClaimStore、已收材料在证据侧，
// 两样都不是目录行。让目录去读它们会造出一个既是目录又能读业务数据的东西。
//
// KindCovered 是唯一由目录独力判完的一维：合同责任范围承不承担这个索赔类型。它
// 不随材料补充而变（变了就是换了合同范围，而换范围按 CONTEXT 是另一个索赔项），
// 所以由它得出的`不予受理`是 ADR-0051 认可的两个永久格之一。
type EligibilityRules struct {
	RuleVersion    string
	KindCovered    bool
	FilingDeadline FilingDeadlineRule
	Materials      MinimumMaterialsRule
	Authorization  AuthorizationCatalogue
}

// EligibilityRuleView 交出适用于一项索赔的资格规则。第二个返回值为 false 即「合同
// 的索赔资格声明不在场」——那时连「这个类型在不在保」都无从谈起，缺一个类型是「没人
// 声明过」而不是「声明说不保」，凭一张空表拒赔就是虚构。声明在场但某一维规则尚未
// 登记，由各维自己的 Registered 如实交代，不折成整体未配置：两者的恢复动作不同，
// 一个要登记整份声明，一个只差那一维。依赖调不通作为错误返回。
type EligibilityRuleView interface {
	RulesForClaim(ctx context.Context, query EligibilityQuery) (EligibilityRules, bool, error)
}

// ClaimEvidenceView 交出一项索赔已经收到的材料。它答「事实是什么」，与目录答的
// 「规则是什么」分列两个端口——最低材料要求这一维正是靠两边相减才核得出来。
//
// 第二个返回值为 false 即「这项索赔的材料归集无从查起」，与「一件都还没收到」分开：
// 后者是有效事实（差集等于整份清单，索赔该进限期补充），前者核不了，只能停在未决。
// 材料已提交只表示收到，不表示达到最低材料要求（CONTEXT）——本端口只交到齐的材料
// 引用，采信与否是证据评价那一步的事。
type ClaimEvidenceView interface {
	ReceivedMaterials(
		ctx context.Context,
		tenant domain.TenantID,
		batch domain.ClaimBatchReference,
		item domain.ClaimItemID,
	) ([]domain.MaterialRequirementReference, bool, error)
}

// LiabilityHandoffIntent 把责任结论交给结算侧（`UC-SA-007` 赔付金额链的上游源——
// 金额由结算形成，这里只交结论）。意图由索赔项认领，复核换出的新结论版本随重发到达；
// 重放重发同一份（ADR-0043）。租户随意图到达（ADR-0003）：索赔对象没有租户维，下游
// 按（租户+批次+项）查库。
type LiabilityHandoffIntent struct {
	TenantID domain.TenantID
	Claim    *domain.ClaimItem
}

// LiabilityHandoff 把责任结论写入 Outbox（`OutboxLiabilityHandoff`）。信封 ID 由索赔
// 项标识认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type LiabilityHandoff interface {
	HandOffLiability(ctx context.Context, intent LiabilityHandoffIntent) error
}

type RecoverySaveOutcome uint8

const (
	RecoverySaveOutcomeInvalid RecoverySaveOutcome = iota
	RecoverySaved
	RecoveryAlreadyRecorded
)

// RecoveryStore 保存追偿事项与动作记录。租户是最高数据隔离边界（ADR-0003），跨越它
// 必须在签名上看得见。FindCurrent 按（案件+相对方+范围）承担事项幂等；动作是只增
// 记录，CountActions 按（事项+动作种类）计数供 attempt 递增——预先通知与正式主张
// 各有各的尝试序列，合并计数会让一类动作吃掉另一类的次序。
//
// Save 的写入代数同 ADR-0031：首发撞（案件+相对方+范围）唯一约束交回 AlreadyRecorded
// （事务保持可用，编排读回赢家）；零行命中即已有，不是错误。
type RecoveryStore interface {
	FindByID(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.RecoveryMatterID,
	) (domain.RecoveryMatter, bool, error)
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		caseID domain.CaseID,
		counterparty domain.CounterpartyReference,
		scope domain.RequestScopeReference,
	) (domain.RecoveryMatter, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, matter domain.RecoveryMatter) (RecoverySaveOutcome, error)
	CountActions(
		ctx context.Context,
		tenant domain.TenantID,
		matter domain.RecoveryMatterID,
		kind domain.RecoveryActionKind,
	) (int, error)
	AppendAction(ctx context.Context, tenant domain.TenantID, action domain.RecoveryAction) error
}

// RecoveryIdentityFactory 签发追偿事项标识。与其余身份工厂分开，理由相同。
type RecoveryIdentityFactory interface {
	NextRecoveryMatterID(ctx context.Context) (domain.RecoveryMatterID, error)
}

// CatalogRegistrationOutcome 是一次目录登记的写入结果。三格照 ADR-0031 的写入代数：
// 撞既有行是业务答案不是错误，交回`已登记`让编排如实答复，不捕 23505——那会把整个
// 事务打进中止态，而登记口常与同一批别的目录写入共事务。
//
// `同一时点已有另一适用版本`独立成格而不并进`已登记`：前者是登记方给错了有效区间
// （改区间重登），后者是这个版本号已经登记过（不可覆盖，要换版本号），两条恢复动作
// 不同。它也不是读侧那条 ErrAmbiguousCatalog——那一条兜的是库里已经坏了的数据，本格
// 是在坏数据形成之前就把它挡在门外。
type CatalogRegistrationOutcome uint8

const (
	CatalogRegistrationOutcomeInvalid CatalogRegistrationOutcome = iota
	CatalogVersionRegistered
	CatalogVersionAlreadyRegistered
	CatalogVersionOverlapsExisting
)

func (outcome CatalogRegistrationOutcome) String() string {
	switch outcome {
	case CatalogVersionRegistered:
		return "REGISTERED"
	case CatalogVersionAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case CatalogVersionOverlapsExisting:
		return "OVERLAPS_EXISTING"
	default:
		return ""
	}
}

// CatalogVersionHeader 是一份带有效区间的目录版本的登记抬头：版本号、发布批准责任与
// `[EffectiveFrom, EffectiveTo)`。三份区间型目录（里程碑映射 `PAR-VIS-01`、分诊规则
// `PAR-VIS-05`、披露策略 `PAR-VIS-09`）共用它——三张版本表的列本来就同形，各写一份
// 抬头只会让「版本/适用范围/发布批准责任」这条要求在三处各表达一次。
//
// 适用范围不设独立列：区间型目录的适用范围由**条目维**表达（映射按源上下文与事实
// 类型、分诊按信号类型与可信度、披露按货主客户账户），更细的伙伴/产品/线路范围是
// `PAR-VIS-01`/`05` 尚未定形的开放集。替租户拟一个范围列，与 ADR-0068 拒绝预拟内容列
// 是同一件错事——形态定了以新迁移扩列。
//
// HasEffectiveTo 为假即未闭区间（当前版本）。用显式布尔而不是零值判断，理由同 NR
// 目录：零时刻是一个合法的绝对时刻，拿它兼作「没有终点」会让补历史的区间登不进来。
type CatalogVersionHeader struct {
	Version        string
	ApprovedBy     string
	EffectiveFrom  time.Time
	EffectiveTo    time.Time
	HasEffectiveTo bool
}

// CatalogApprovalHeader 是不带区间那两份目录（索赔资格声明与申请人授权目录，同属
// `PAR-VIS-08`）的登记抬头。两张表以（租户+合同范围）与（租户+客户账户）为键、版本
// 存在列上，结构上一个身份只容一行——所以它们没有区间可登，换版本要换身份或另立
// 迁移，登记口不替它们发明一个区间。
type CatalogApprovalHeader struct {
	Version    string
	ApprovedBy string
}

// MilestoneMappingEntry 是一条映射条目：某源上下文的某类事实归到哪个标准里程碑。
// 键取（源上下文+事实类型）——「标准里程碑映射按源上下文与事实类型版本化登记，一行
// 覆盖此后同类型事实，不得按单条事实引用建目录」（CONTEXT 硬句）。
type MilestoneMappingEntry struct {
	Source    domain.SourceContext
	Kind      domain.SourceFactKind
	Milestone domain.MilestoneReference
}

// MilestoneMappingRegistration 登记一版里程碑映射：抬头加整版条目。条目随版本一次
// 写全，不支持事后追加——「不可覆盖版本」意味着一版的内容在发布那一刻就定了，事后
// 往已发布版本里塞条目会让「按 vN 判的未归类」这个已作出的判断在事后变成已归类。
type MilestoneMappingRegistration struct {
	Header  CatalogVersionHeader
	Entries []MilestoneMappingEntry
}

// TriageRuleEntry 是一条分诊条目：某信号类型在某可信度依据下走哪一格。键含可信度，
// 因为四走向的分界正立在它上面（「高可信、高影响且命中版本化分诊规则的信号可以自动
// 建立或关联案件」）。
//
// Team 与走向成对：`自动建案`条目必带责任团队，其余走向必不带（理由见 TriageAnswer）。
// 团队是谁属实例半边——本端口只要求登记方在说「自动建案」的同一行说清归谁，不替它挑。
type TriageRuleEntry struct {
	Kind       domain.ExceptionSignalKindReference
	Confidence domain.ConfidenceReference
	Outcome    domain.TriageOutcome
	Team       domain.ResponsibleTeamReference
}

// TriageRuleRegistration 登记一版分诊规则。条目纪律同映射登记。
type TriageRuleRegistration struct {
	Header  CatalogVersionHeader
	Entries []TriageRuleEntry
}

// NotificationPolicyRegistration 登记一条通知策略：某份披露策略走什么渠道、限时多久、
// 按哪条判据算满足通知义务，加发布批准责任。
//
// 它没有 CatalogVersionHeader：这份目录以（租户+披露策略引用）为键，版本化由引用值
// 本身承担（0010），换版即换引用、新旧两行并存，因此既无版本列也无区间可登。
//
// DeadlineAfter 是**相对量**：合同写的是「披露后 N 小时内」，而披露决定时间逐份不同。
// 存绝对时间等于给整个目录钉死一个截止点。
type NotificationPolicyRegistration struct {
	Policy        domain.DisclosurePolicyReference
	Channel       domain.NotificationChannelReference
	DeadlineAfter time.Duration
	Obligation    domain.DisclosurePolicyReference
	ApprovedBy    string
}

// ClaimEligibilityRegistration 登记一份合同责任范围的索赔资格声明与它承担的索赔类型。
//
// 声明与覆盖类型一次写全：只有声明在场，「不在集合内」才说得通（0011）。分两步登记会
// 出现一段「声明已在、覆盖类型还没写」的窗口，那期间任一索赔都会被判成`不予受理`——
// 而那是 ADR-0051 的永久格，审过不再审，没有第二次机会。
type ClaimEligibilityRegistration struct {
	Header       CatalogApprovalHeader
	Contract     domain.ContractScopeReference
	CoveredKinds []domain.ClaimKindReference
}

// ClaimAuthorizationRegistration 登记一个货主客户账户的申请人授权名单。名单语义只有
// 一条：在列即该申请人获此账户的索赔提交授权（`AT-VE-125`）。
//
// 允许空名单：目录在场而名单为空是「这个账户目前不授权任何人代提」，与「还没登记」
// 不是一回事——后者由整张目录行不在场表达（视图答未登记，编排停在未决）。两者的恢复
// 动作不同，压成一格会让人去补错东西。
type ClaimAuthorizationRegistration struct {
	Header     CatalogApprovalHeader
	Customer   domain.CustomerAccountReference
	Applicants []domain.ApplicantReference
}

// DisclosurePolicyEntry 是一条披露条目：对某货主客户账户，客户视图四维各自获准展示
// 什么。四维用 domain.ViewDimension 而不是（状态+内容）两个裸字段——展示必带内容来处、
// 待确认与不展示必不带，两个方向的虚构在构造期就被拦下，库上的四条 shape 约束是第二
// 道网而不是唯一一道。
type DisclosurePolicyEntry struct {
	Customer   domain.CustomerAccountReference
	Milestones domain.ViewDimension
	ETA        domain.ViewDimension
	Final      domain.ViewDimension
	Note       domain.ViewDimension
}

// DisclosurePolicyRegistration 登记一版披露策略。条目纪律同映射登记。
type DisclosurePolicyRegistration struct {
	Header  CatalogVersionHeader
	Entries []DisclosurePolicyEntry
}

// CatalogRegistry 是 VE 五类规则与策略目录的写入口（`PAR-VIS-01`/`05`/`07`/`08`/`09`；
// `PAR-VIS-08` 跨索赔资格与申请人授权两组表，故它一类占两法）。与五个只读装载口成对：
// 那五口至今只能答`未配置`，是因为除测试外没有任何东西写得进这些表。
//
// 目录**内容**属实例半边、待租户提供，本端口只建门：它不带任何默认条目，也不在缺件
// 时替登记方补值——那会把「还没人登记」变成一次有依据的判断。
//
// 写入侧防重叠是本端口的硬要求：同一时点两个适用版本在读侧是错误（ErrAmbiguousCatalog），
// 而登记口的职责是让那种数据根本进不来，不是让读口去兜。实现须在自己的写入事务内完成
// 判定，调用方不必先查后写。
//
// 所有方法都在调用方的事务内执行（RequireExecutor 语义）：一版抬头与它的整版条目必须
// 同一提交，半版目录比没有目录更坏——读口会把它当成一次已作出的判断。
type CatalogRegistry interface {
	RegisterMilestoneMapping(
		ctx context.Context,
		tenant domain.TenantID,
		registration MilestoneMappingRegistration,
	) (CatalogRegistrationOutcome, error)
	RegisterTriageRules(
		ctx context.Context,
		tenant domain.TenantID,
		registration TriageRuleRegistration,
	) (CatalogRegistrationOutcome, error)
	RegisterNotificationPolicy(
		ctx context.Context,
		tenant domain.TenantID,
		registration NotificationPolicyRegistration,
	) (CatalogRegistrationOutcome, error)
	RegisterClaimEligibility(
		ctx context.Context,
		tenant domain.TenantID,
		registration ClaimEligibilityRegistration,
	) (CatalogRegistrationOutcome, error)
	RegisterClaimAuthorization(
		ctx context.Context,
		tenant domain.TenantID,
		registration ClaimAuthorizationRegistration,
	) (CatalogRegistrationOutcome, error)
	RegisterDisclosurePolicy(
		ctx context.Context,
		tenant domain.TenantID,
		registration DisclosurePolicyRegistration,
	) (CatalogRegistrationOutcome, error)
}

// MaterialReceipt 是一笔材料收讫登记：某项索赔的某件材料在某时刻经受控通道登记为
// 已收讫（.scratch/ve-claims-read-seams/02）。行身份是（租户、批次、项、材料要求
// 引用、收讫时间）五件——租户照本端口家族的惯例走方法签名，其余四件在此。材料要求
// 引用必须是资格目录签发的词：ClaimEvidenceView 与最低材料要求相减才核得出缺口，
// 两边不同词，差集就永远不为空。
//
// ReceivedBy 是执行者身份双轨的第②轨（登记内容，「登记者声明了谁经手收讫」），
// 显式必填；第①轨通道技术身份由 CLI 入口自取、与登记同笔事务落
// visibility_exception.channel_execution，不进本结构——结构上不存在从输入伪造
// 通道身份的路径。行上只登收讫事实与经手声明，不登材料内容实体（敏感实例外置）。
type MaterialReceipt struct {
	Batch      domain.ClaimBatchReference
	Item       domain.ClaimItemID
	Material   domain.MaterialRequirementReference
	ReceivedAt time.Time
	ReceivedBy string
}

// MaterialReceiptRevocation 撤销一笔收讫：以收讫行的五件全键指名对象——同一
// （批次+项+材料）可能收讫多次，缺收讫时间就指不清撤的是哪一次。撤销不删收讫行，
// 另立撤销行（0021）；撤销之后同一材料要再次采信，走新的收讫行。
type MaterialReceiptRevocation struct {
	Batch      domain.ClaimBatchReference
	Item       domain.ClaimItemID
	Material   domain.MaterialRequirementReference
	ReceivedAt time.Time
	RevokedBy  string
	RevokedAt  time.Time
}

type MaterialReceiptWriteOutcome uint8

const (
	MaterialReceiptWriteOutcomeInvalid MaterialReceiptWriteOutcome = iota
	MaterialReceiptRecorded
	MaterialReceiptAlreadyRecorded
	MaterialReceiptRevocationRecorded
	MaterialReceiptRevocationAlreadyRecorded
	// MaterialReceiptUnknown 只由撤销给出：五件指名的收讫行不在场，无从撤销。这是
	// 登记册的治理答案，不是故障——恢复动作是人工核对引用，不是重试。
	MaterialReceiptUnknown
)

// MaterialReceiptRegistry 是材料归集面的写入口，与 ClaimEvidenceView 成对：读口答
// 「已经收到什么」，本口把收讫与撤销写进去。与目录册（CatalogRegistry）分列两个端口
// ——那边登的是规则版本，重登同版本号是要人换号的治理答案；这边登的是事实，行身份
// 就是事实本身，同五件重登幂等（AlreadyRecorded），没有内容可被顶替。重放答案已指名
// 本次没有写入，实现不比对经手声明——先登的那份声明留在行上。
//
// 两个方法都在调用方的事务内执行（RequireExecutor 语义）：登记与通道留痕必须同一
// 提交——痕不声称一笔没落库的登记，登记也不许在无痕状态下落地。
type MaterialReceiptRegistry interface {
	RegisterReceipt(
		ctx context.Context,
		tenant domain.TenantID,
		receipt MaterialReceipt,
	) (MaterialReceiptWriteOutcome, error)
	RevokeReceipt(
		ctx context.Context,
		tenant domain.TenantID,
		revocation MaterialReceiptRevocation,
	) (MaterialReceiptWriteOutcome, error)
}
