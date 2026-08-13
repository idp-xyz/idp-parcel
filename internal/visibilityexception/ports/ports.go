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

// ProjectionStore 保存当前投影版本。原版本由重派生的指回关系承担历史，库只管当前。
type ProjectionStore interface {
	FindCurrent(ctx context.Context, parcel domain.TrackedParcelReference) (domain.TrackingProjection, bool, error)
	Save(ctx context.Context, projection domain.TrackingProjection) error
}

// ProjectionIdentityFactory 签发投影版本标识。
type ProjectionIdentityFactory interface {
	NextProjectionVersionID(ctx context.Context) (domain.ProjectionVersionID, error)
}

// ProjectionHandoffIntent 把新派生的投影交给适用下游（客户视图派生正是消费者）。
// 意图由投影版本认领，重放重发同一份（ADR-0043）。
type ProjectionHandoffIntent struct {
	Projection domain.TrackingProjection
}

// ProjectionHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017 的
// Bento/Outbox 闸门。
type ProjectionHandoff interface {
	HandOffProjection(ctx context.Context, intent ProjectionHandoffIntent) error
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
// 输入）。意图由视图版本认领，重放重发同一份（ADR-0043）。
type CustomerViewHandoffIntent struct {
	View domain.CustomerTrackingView
}

// CustomerViewHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
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
type TriageAnswer struct {
	Outcome domain.TriageOutcome
	Rule    domain.SignalRuleVersionReference
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
type RaisedSignalRecord struct {
	Tenant     domain.TenantID
	Parcel     domain.TrackedParcelReference
	Kind       domain.ExceptionSignalKindReference
	Episode    *domain.SignalEpisode
	Conclusion domain.TriageConclusion
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

// TriageHandoffIntent 把分诊结论交给适用下游（建案、复核队列的输入）。意图由发作期
// 标识认领（结论内含），重放重发同一份（ADR-0043）。
type TriageHandoffIntent struct {
	Parcel     domain.TrackedParcelReference
	Kind       domain.ExceptionSignalKindReference
	Conclusion domain.TriageConclusion
}

// TriageHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017 的
// Bento/Outbox 闸门。
type TriageHandoff interface {
	HandOffTriage(ctx context.Context, intent TriageHandoffIntent) error
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

// CustomerNotificationStore 按披露决定找回并保存通知。披露决定没有自有标识，按其身份
// 三维（客户、发作期、决定时间）定位——同一披露不重发通知的幂等界线就立在这里。
type CustomerNotificationStore interface {
	FindByDisclosure(
		ctx context.Context,
		disclosure domain.DisclosureDecision,
	) (*domain.CustomerNotification, bool, error)
	Save(ctx context.Context, notification *domain.CustomerNotification) error
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
// 由通知标识认领，重放重发同一份（ADR-0043）。
type NotificationHandoffIntent struct {
	Notification *domain.CustomerNotification
}

// NotificationHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017 的
// Bento/Outbox 闸门。
type NotificationHandoff interface {
	HandOffNotification(ctx context.Context, intent NotificationHandoffIntent) error
}

// ActiveCaseView 回答案件是否在场且未关闭。案件本体的读写归案件编排，这里只要一个
// 在场判据：处置请求只能挂在活案件下——「案件范围缩小、改派、归并或关闭前必须盘点
// 全部未完成处置请求」（CONTEXT），关闭后再挂新请求就是绕过那次盘点。
type ActiveCaseView interface {
	CaseActive(ctx context.Context, caseID domain.CaseID) (active bool, found bool, err error)
}

// DispositionRequestStore 按稳定身份与幂等键找回并保存处置请求。
//
// FindCurrent 按（案件+动作+范围）交回当前那份——未被替代的请求；同键重复到达据它
// 短路，不重发。SaveSupersession 把被替代者与后继同一提交：只落一半，替代关系与新
// 意图会各说各话。
type DispositionRequestStore interface {
	FindByID(ctx context.Context, id domain.DispositionRequestID) (*domain.DispositionRequest, bool, error)
	FindCurrent(
		ctx context.Context,
		caseID domain.CaseID,
		action domain.RequestedActionReference,
		scope domain.RequestScopeReference,
	) (*domain.DispositionRequest, bool, error)
	Save(ctx context.Context, request *domain.DispositionRequest) error
	SaveSupersession(ctx context.Context, prior, successor *domain.DispositionRequest) error
}

// DispositionRequestIdentityFactory 签发处置请求标识。与其余身份工厂分开，理由相同。
type DispositionRequestIdentityFactory interface {
	NextDispositionRequestID(ctx context.Context) (domain.DispositionRequestID, error)
}

// DispositionHandoffIntent 把已成立的处置请求交给发送侧下游。真实目标上下文的受理在
// 对方——这里只是发送意图，由请求标识认领，重放重发同一份（ADR-0043）。
type DispositionHandoffIntent struct {
	Request *domain.DispositionRequest
}

// DispositionHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017 的
// Bento/Outbox 闸门。
type DispositionHandoff interface {
	HandOffDispositionRequest(ctx context.Context, intent DispositionHandoffIntent) error
}

// ETAStore 按（包裹+里程碑）保存当前预测版本。历史版本由 Refresh 的指回关系承担，
// 库只管当前。
type ETAStore interface {
	FindCurrent(
		ctx context.Context,
		parcel domain.TrackedParcelReference,
		milestone domain.MilestoneReference,
	) (domain.ETAPrediction, bool, error)
	Save(ctx context.Context, eta domain.ETAPrediction) error
}

// ETAIdentityFactory 签发预测版本标识。与其余身份工厂分开，理由相同。
type ETAIdentityFactory interface {
	NextETAVersionID(ctx context.Context) (domain.ETAVersionID, error)
}

// ETAHandoffIntent 把新预测版本交给适用下游（客户视图链的重派生输入——「ETA 新版本
// ……必须重新派生客户视图」）。意图由预测版本认领，重放重发同一份（ADR-0043）。
type ETAHandoffIntent struct {
	Prediction domain.ETAPrediction
}

// ETAHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017 的
// Bento/Outbox 闸门。
type ETAHandoff interface {
	HandOffETA(ctx context.Context, intent ETAHandoffIntent) error
}

// VisibilityGapStore 按（包裹+预期观察+窗口规则版本）保存缺口。键含窗口规则版本：
// 「新窗口版本生效→后续采用新版本，原判断保留」——同一预期在新旧规则下是两次独立
// 判断，压成一个键会让新版本覆盖原判断。
type VisibilityGapStore interface {
	FindCurrent(
		ctx context.Context,
		parcel domain.TrackedParcelReference,
		expectation domain.ExpectedObservationReference,
		windowRule domain.ObservationWindowReference,
	) (domain.VisibilityGap, bool, error)
	Save(ctx context.Context, gap domain.VisibilityGap) error
}

// VisibilityGapHandoffIntent 把已成立的缺口交给信号链（缺口是否命中异常规则由分诊
// 判断，本编排不替它判）。意图由缺口身份三维认领，重放重发同一份（ADR-0043）。
type VisibilityGapHandoffIntent struct {
	Gap domain.VisibilityGap
}

// VisibilityGapHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
type VisibilityGapHandoff interface {
	HandOffVisibilityGap(ctx context.Context, intent VisibilityGapHandoffIntent) error
}

// ClaimStore 按（批次+项）找回并保存索赔项。键含批次：项标识由客户提交侧建立，
// 批次内唯一是它的口径，跨批次撞号不该互相干扰。
type ClaimStore interface {
	FindByBatchItem(
		ctx context.Context,
		batch domain.ClaimBatchReference,
		item domain.ClaimItemID,
	) (*domain.ClaimItem, bool, error)
	Save(ctx context.Context, claim *domain.ClaimItem) error
}

// EligibilityQuery 是资格审核规则的输入：申请人授权、客户账户、合同版本、索赔时限、
// 目标范围、重复关系和最低材料要求都由规则侧核对，本上下文只带引用。
type EligibilityQuery struct {
	Batch    domain.ClaimBatchReference
	Item     domain.ClaimItemID
	Customer domain.CustomerAccountReference
	Contract domain.ContractScopeReference
	Target   domain.RequestScopeReference
	Kind     domain.ClaimKindReference
}

// EligibilityAnswer 是资格目录的答复：通过或不通过，带判断依据。
type EligibilityAnswer struct {
	Screen domain.EligibilityScreen
	Basis  string
}

// EligibilityRuleView 回答「这项索赔按版本化资格规则过不过审」。第二个返回值为 false
// 即「资格目录未配置」——真实索赔时限、材料要求与授权目录属待登记实例参数。没有目录
// 的资格审核无从作出：默认受理与默认拒赔都是虚构，由编排形成未决等租户登记。依赖调
// 不通作为错误返回。
type EligibilityRuleView interface {
	ScreenClaim(ctx context.Context, query EligibilityQuery) (EligibilityAnswer, bool, error)
}

// LiabilityHandoffIntent 把责任结论交给结算侧（`UC-SA-007` 赔付金额链的上游源——
// 金额由结算形成，这里只交结论）。意图由索赔项认领，复核换出的新结论版本随重发到达；
// 重放重发同一份（ADR-0043）。
type LiabilityHandoffIntent struct {
	Claim *domain.ClaimItem
}

// LiabilityHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017 的
// Bento/Outbox 闸门。
type LiabilityHandoff interface {
	HandOffLiability(ctx context.Context, intent LiabilityHandoffIntent) error
}

// RecoveryStore 保存追偿事项与动作记录。FindCurrent 按（案件+相对方+范围）承担事项
// 幂等；动作是只增记录，CountActions 按（事项+动作种类）计数供 attempt 递增——预先
// 通知与正式主张各有各的尝试序列，合并计数会让一类动作吃掉另一类的次序。
type RecoveryStore interface {
	FindByID(ctx context.Context, id domain.RecoveryMatterID) (domain.RecoveryMatter, bool, error)
	FindCurrent(
		ctx context.Context,
		caseID domain.CaseID,
		counterparty domain.CounterpartyReference,
		scope domain.RequestScopeReference,
	) (domain.RecoveryMatter, bool, error)
	Save(ctx context.Context, matter domain.RecoveryMatter) error
	CountActions(
		ctx context.Context,
		matter domain.RecoveryMatterID,
		kind domain.RecoveryActionKind,
	) (int, error)
	AppendAction(ctx context.Context, action domain.RecoveryAction) error
}

// RecoveryIdentityFactory 签发追偿事项标识。与其余身份工厂分开，理由相同。
type RecoveryIdentityFactory interface {
	NextRecoveryMatterID(ctx context.Context) (domain.RecoveryMatterID, error)
}
