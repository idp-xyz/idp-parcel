// Package ports 声明 parcel-shipment 自己拥有的语义边界。这里只有接口：它们的
// PostgreSQL 适配器仍阻断在 Bento 持久化闸门之后（ADR-0017），所以当前唯一的实现是
// 测试用的确定性替身。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// SourceSubmissionRepository 在明确的来源作用域下保全不可变来源事实。查询以完整的
// SourceIdentity 为键，因此否定结果不区分「不存在」与「属于另一个租户或客户账户」。
type SourceSubmissionRepository interface {
	FindPreserved(ctx context.Context, identity domain.SourceIdentity) (domain.SourceSubmissionFingerprint, bool, error)
	Preserve(ctx context.Context, submission domain.SourceSubmissionFingerprint) error
	// AppendObservation 记录同一逻辑请求再次被观察到。它追加在已保全事实旁边，绝不
	// 替换它——这正是不同的 occurredAt/receivedAt 无法改写历史的原因。
	AppendObservation(ctx context.Context, observed domain.SourceSubmissionFingerprint) error
}

// ShipmentRequestRepository 以产生委托的来源身份为键存储委托聚合，这样重放才能返回
// 原委托而不是再建一份。
//
// Insert 与 Save 分开：建单只能发生一次，而决定是在既有委托上推进。合成一个方法会让
// 「这是第一份还是第二份」失去表达。
type ShipmentRequestRepository interface {
	FindBySourceIdentity(ctx context.Context, identity domain.SourceIdentity) (domain.ShipmentRequest, bool, error)
	Insert(ctx context.Context, identity domain.SourceIdentity, request domain.ShipmentRequest) error
	Save(ctx context.Context, identity domain.SourceIdentity, request domain.ShipmentRequest) error
}

// ProductionOwnershipAuthority 是试点准入控制，回答完整拟受理范围当前由谁承接。
// parcel-shipment 只消费该决定，绝不自行推导一个。
type ProductionOwnershipAuthority interface {
	DecideProductionOwnership(ctx context.Context, scope domain.AdmissionScope) (domain.ProductionOwnershipDecision, error)
}

// SubmissionIdentityFactory 签发 parcel-shipment 自己拥有的内部身份。刻意不从调用方
// 接收：客户参考号不得变成内部标识。
type SubmissionIdentityFactory interface {
	NextSubmissionVersionID(ctx context.Context) (domain.SubmissionVersionID, error)
	NextAcceptanceDecisionTaskID(ctx context.Context) (domain.AcceptanceDecisionTaskID, error)
}

// AcceptanceDecisionIdentity 与 SubmissionIdentityFactory 分开：建单期签发的身份与决定期
// 签发的身份由不同用例触发，合并会让形成决定的编排依赖它根本不签发的那两个身份。
type AcceptanceDecisionIdentity interface {
	NextAcceptanceDecisionID(ctx context.Context) (domain.AcceptanceDecisionID, error)
}

// RecordedJudgments 是接受判断任务上已经采用的全部权威判断。财务控制用零值表示尚未形成，
// 而不是配一个布尔：翻译函数据零值形成`无法判定`，因此「没有控制」无法被悄悄读成通过。
type RecordedJudgments struct {
	Reachability     []domain.ReachabilityJudgment
	FinancialControl domain.FinancialControlResult
}

// RecordedJudgmentReader 取回接受判断任务上已记录的判断，供形成决定那一步装配校验结果。
type RecordedJudgmentReader interface {
	LoadRecordedJudgments(ctx context.Context, requestID domain.ShipmentRequestID) (RecordedJudgments, error)
}

type Clock interface {
	Now() time.Time
}

// CommercialBasisQuery 是 parcel-shipment 请 party-commercial 据以解析的范围。它只
// 携带引用：本上下文说明需要哪种依据，绝不指定应当选中哪个商业版本。
type CommercialBasisQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
}

// CommercialBasisResolution 是 parcel-shipment 视角下的一次商业解析结果。
//
// 它不只交回快照。`UC-PC-002` 的消费者端口契约明写「必须返回结构化的唯一成功、无适用依据、
// 适用冲突、解析未决……不能只返回对象或通用错误」，原因就在这里：`无适用依据`要由本上下文
// 形成拒绝，`解析未决`只能保持未决，而两者都表现为「没有快照」，光看快照分不出来。
//
// Reason 是 party-commercial 给的稳定原因引用，本上下文原样记到校验结果上，不重新解释。
type CommercialBasisResolution struct {
	Snapshot      domain.CommercialBasisSnapshot
	Applicability domain.CommercialApplicability
	Reason        domain.CheckReason
}

// CommercialBasisResolver 是 parcel-shipment 视角下的 party-commercial 判断。
type CommercialBasisResolver interface {
	ResolveCommercialBasis(ctx context.Context, query CommercialBasisQuery) (CommercialBasisResolution, error)
}

// ReachabilityRequest 携带按所采用规则包声明的策略形成的判断时点。权威提供方必须校验
// 并回显它——所以它显式随请求传递，而不是留给提供方自己的时钟。
type ReachabilityRequest struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	DeclaredParcelID  domain.DeclaredParcelID
	AsOf              domain.JudgmentAsOf
}

// ReachabilityAssessor 是 parcel-shipment 视角下的 network-routing 三值判断。三个
// 取值没有一个是接受决定，本上下文也不得在这里据其推导出一个。
type ReachabilityAssessor interface {
	AssessParcelReachability(ctx context.Context, request ReachabilityRequest) (domain.ReachabilityJudgment, error)
}

// FinancialControlRequest 是 parcel-shipment 请求一次接受前财务控制的范围。它按当前提交
// 版本取，不像可达性那样按声明包裹取：控制作用在整份委托上，逐成员发起会把一份委托的资金
// 占用重复成成员份数。
//
// 它同样不携带金额、账户或阈值。价格、余额与冻结属 settlement-accounting，策略属
// party-commercial；本上下文说明要为哪份提交版本、按哪个时点控制，仅此而已。
type FinancialControlRequest struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	AsOf              domain.JudgmentAsOf
}

// PreAcceptanceFinancialController 是 parcel-shipment 视角下的接受前财务控制。三个取值
// 没有一个是接受决定，本上下文也不得在这里据其推导出一个。
//
// 依赖调不通要作为错误返回。把它读成`明确无控制`正是用例禁止的默认放行：一次故障会因此
// 变成一个看起来通过了的接受前控制。
type PreAcceptanceFinancialController interface {
	ApplyPreAcceptanceFinancialControl(ctx context.Context, request FinancialControlRequest) (domain.FinancialControlResult, error)
}

// ControlReleaseRequest 指名要释放哪一次接受前资金控制。它只携带原控制的业务关联，不带
// 金额、账户或币种：释放哪一笔由 settlement-accounting 按原关联认领，冻结不属本上下文。
type ControlReleaseRequest struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	ControlResultID   domain.FinancialControlResultID
}

// PreAcceptanceControlRelease 在接受确定未成立后按原关联请求解除资金控制。
//
// 它与 PreAcceptanceFinancialController 分开：施加控制由推进判断那一步发起，解除由形成决定
// 那一步发起，合并成一个端口会让形成决定的编排依赖一个它根本不会调用的方法。
type PreAcceptanceControlRelease interface {
	ReleasePreAcceptanceControl(ctx context.Context, request ControlReleaseRequest) error
}

// ActiveRejectionAuthorizationQuery 说明谁要以什么原因主动拒绝哪一份提交版本。它刻意不带
// 授权引用：调用方自带一个，就等于自己给自己签字。
type ActiveRejectionAuthorizationQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Decider           domain.DeciderReference
	Reason            domain.RejectionReasonReference
}

// ActiveRejectionAuthorizer 回答 party-commercial 是否授权这次主动拒绝。授权引用由那边
// 签发，parcel-shipment 只保存所采用的引用——角色等级与原因目录都不属本上下文。
//
// 未授权时交回零值引用而不是错误：那是一个业务答案（这个人不能拒这单），与「授权服务答不出」
// 分属两回事，后者才是错误。
type ActiveRejectionAuthorizer interface {
	AuthorizeActiveRejection(ctx context.Context, query ActiveRejectionAuthorizationQuery) (domain.RejectionAuthorityReference, error)
}

// WithdrawalAuthorizationQuery 说明谁要以什么原因撤回哪一份待决委托。与主动拒绝那一支同样
// 不带授权引用：调用方自带一个，就等于自己给自己签字。
type WithdrawalAuthorizationQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Requester         domain.WithdrawalRequesterReference
	Reason            domain.WithdrawalReasonReference
}

// WithdrawalAuthorizer 回答请求方当前是否有权撤回这份委托。它与 ActiveRejectionAuthorizer
// 分开：一个问的是货主客户或其授权代表，另一个问的是运营侧授权角色，两者的授权来源、有效
// 期间与原因目录都不同，合并会让「客户能不能取消」和「我们能不能不接」共用一套规则。
//
// 未授权时交回零值引用而不是错误：那是一个业务答案（这个人不能撤这单），与「授权服务答不出」
// 分属两回事，后者才是错误。真实撤回授权角色与原因语义仍是 `PAR-COM-14` 待提供的实例参数，
// 本上下文不内置任何默认——`UC-PS-005` 明禁默认任何角色有撤回权。
type WithdrawalAuthorizer interface {
	AuthorizeWithdrawal(ctx context.Context, query WithdrawalAuthorizationQuery) (domain.WithdrawalAuthorityReference, error)
}

// SourceDataAmendmentAuthorizationQuery 说明谁要以什么原因修订哪一处资料范围。它同样不带
// 授权引用：调用方自带一个，就等于自己给自己签字。
type SourceDataAmendmentAuthorizationQuery struct {
	Identity  domain.SourceIdentity
	Scope     domain.SourceDataScope
	Requester domain.RequesterReference
	Reason    domain.AmendmentReasonReference
}

// SourceDataAmendmentAuthorization 交回本次修订所采用的授权依据与实际决定方。
//
// 两项一起由 party-commercial 给出，不由调用方声明：`UC-PS-002` 要求「登录操作人不能替代实际
// 决定方」，而让请求方自报决定方正是那句话禁止的事。本上下文只保存所采用的那一份，不判断它
// 够不够格——授权规则属 party-commercial。
type SourceDataAmendmentAuthorization struct {
	Authority domain.AmendmentAuthoritySnapshot
	Decider   domain.DeciderReference
}

// SourceDataAmendmentAuthorizer 回答请求方当前是否有权修订这处资料范围。
//
// 未授权时交回零值而不是错误：那是一个业务答案（这个人不能改这处资料），与「授权服务答不出」
// 分属两回事，后者才是错误。真实请求方、实际决定方与授权入口仍是 `BD-PS-009` 待确认的实例
// 参数，本上下文不内置任何默认。
type SourceDataAmendmentAuthorizer interface {
	AuthorizeSourceDataAmendment(
		ctx context.Context,
		query SourceDataAmendmentAuthorizationQuery,
	) (SourceDataAmendmentAuthorization, error)
}

// SourceDataAmendmentAllowance 是某处资料范围在当前阶段上「能不能这样改」的登记结论。
//
// `NotDeclared` 是零值且刻意如此：矩阵没登记就是没登记，而零值必须落在最保守的那一格。让它
// 落在`允许`上，一个尚未登记规则的租户会因为系统便利而放行任意修订，那正是 `UC-PS-002`
// 「未登记时只能形成未决或业务拒绝，不能以系统便利推断允许」明禁的事。
type SourceDataAmendmentAllowance uint8

const (
	SourceDataAmendmentNotDeclared SourceDataAmendmentAllowance = iota
	SourceDataAmendmentAllowed
	SourceDataAmendmentDisallowed
)

// SourceDataAmendmentQuery 说明要判断哪一处资料范围在当前阶段的允许动作。
type SourceDataAmendmentQuery struct {
	Identity domain.SourceIdentity
	Scope    domain.SourceDataScope
	Reason   domain.AmendmentReasonReference
}

// SourceDataRuleDeclaration 回答已登记规则是否允许这次修订。
//
// 字段、字段组、阶段与允许动作由 `PAR-COM-13` 与真实合同、产品、线路和关务规则登记，属实例
// 半边；本上下文只消费登记结论，绝不自带一份矩阵。没有租户时它必然交回 `NotDeclared`，编排
// 据以停在`待复核`——那是「还没人说这能不能改」，不是「客户违规」。
type SourceDataRuleDeclaration interface {
	DeclareSourceDataAmendment(
		ctx context.Context,
		query SourceDataAmendmentQuery,
	) (SourceDataAmendmentAllowance, error)
}

// SourceDataVersionIdentity 签发客户原始资料版本的内部身份。与另外两个身份工厂分开，理由
// 相同：建单期、决定期与修订期由不同用例触发，合并会让一个编排依赖它根本不签发的身份。
type SourceDataVersionIdentity interface {
	NextSourceDataVersionID(ctx context.Context) (domain.SourceDataVersionID, error)
}

// AcceptanceJudgmentRecorder 把一个已采用的判断记到它所推进的那份委托的接受判断任务上。
//
// RecordProcessingAttempt 记的是没能推进的那一轮。用例要求任务「追加判断与处理尝试」两样
// 都留：只留成功的判断，一份卡了十轮的委托看起来会和刚建单的一模一样。
type AcceptanceJudgmentRecorder interface {
	RecordReachabilityJudgment(ctx context.Context, requestID domain.ShipmentRequestID, judgment domain.ReachabilityJudgment) error
	RecordFinancialControlResult(ctx context.Context, requestID domain.ShipmentRequestID, result domain.FinancialControlResult) error
	RecordProcessingAttempt(ctx context.Context, requestID domain.ShipmentRequestID, attempt domain.ProcessingAttempt) error
}
