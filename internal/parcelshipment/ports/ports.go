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
//
// AdoptedCommercialResolution 是这些判断形成时所采用的那次商业解析。它必须留住，否则提交
// 决定前无从做 `UC-PC-002` 步骤 8 的重解——拿决定时刻的新解析自比永远相容，`AT-PC-026` 的
// 提交前失效就永远抓不到，而判断是在旧依据的时点策略下形成的。零值表示还没有任何一轮采用
// 过依据。
type RecordedJudgments struct {
	Reachability                []domain.ReachabilityJudgment
	FinancialControl            domain.FinancialControlResult
	AdoptedCommercialResolution domain.CommercialResolutionID
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

// JudgmentAsOfQuery 请 party-commercial 为某一类判断校验并回显一个时点值。
//
// 它只回指第一阶段的解析标识，不回传第一阶段的结果对象：跨上下文多步协议的中间状态由提供方
// 按标识保留（ADR-0027）。
//
// 它刻意不带时点值。`UC-PC-002` 步骤 6 要消费方形成值，而 ADR-0025 把「消费方」定为适配器：
// 值按声明的语义在适配器里形成，编排给不出它——编排手上只有本地时钟，拿它顶就是用例明禁的
// 「用一个全局时间代替」。没有租户时适配器形不出值，交回`未配置`，那正是首发要停下的地方。
type JudgmentAsOfQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Resolution        domain.CommercialResolutionID
	Declared          domain.DeclaredAsOf
}

// JudgmentAsOfOutcome 是 party-commercial 第二阶段答复在本上下文的落点。
//
// 五种未成形分开而不压成一个「不可用」：`未配置`等租户把 `PAR-COM-14` 登记上，`未决`等依赖
// 恢复，`值被拒`要本方改这次请求，`依据未解析`要回第一阶段重解，`输入未受理`说的是本方指名了
// 一份不属于自己的解析——五者里只有一种靠重试能解决。压平之后调用方只能靠猜，而 ADR-0025
// 要求提供方封闭集合里每个取值都有明确落点。
type JudgmentAsOfOutcome uint8

const (
	JudgmentAsOfOutcomeInvalid JudgmentAsOfOutcome = iota
	JudgmentAsOfFormed
	JudgmentAsOfBasisNotResolved
	JudgmentAsOfNotConfigured
	JudgmentAsOfPending
	JudgmentAsOfValueRejected
	JudgmentAsOfInputNotAccepted
)

func (outcome JudgmentAsOfOutcome) String() string {
	switch outcome {
	case JudgmentAsOfFormed:
		return "FORMED"
	case JudgmentAsOfBasisNotResolved:
		return "BASIS_NOT_RESOLVED"
	case JudgmentAsOfNotConfigured:
		return "NOT_CONFIGURED"
	case JudgmentAsOfPending:
		return "PENDING"
	case JudgmentAsOfValueRejected:
		return "VALUE_REJECTED"
	case JudgmentAsOfInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// JudgmentAsOfFormation 只在`已形成`时携带时点。其余四种一律不带：交回一个零值时点，调用方
// 会拿一个没人授权过的时刻去推进权威判断。
type JudgmentAsOfFormation struct {
	Outcome JudgmentAsOfOutcome
	AsOf    domain.JudgmentAsOf
}

// CommercialRevalidationQuery 请 party-commercial 在本方提交决定前按原查询重解一次。
//
// 与第二阶段同样只回指解析标识：让调用方另给一份解析键，一次「校验」就能拿另一个范围的视图
// 去证明这份解析仍然成立（ADR-0027）。
type CommercialRevalidationQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Resolution        domain.CommercialResolutionID
}

// CommercialRevalidationOutcome 是第三阶段独有的结果代数。
//
// `已失效`必须与`解析未决`分开，不能一起落进 CommercialApplicability 的`无法判定`。两者要
// 采取的动作相反：未决是等权威恢复，本方重试同一次重校验就行；`已失效`重试一万次也还是失效，
// `UC-PC-002` 第 58 行要的是**重新解析**——回第一阶段拿当前有效的那一份。混成一格，一份被
// 新修订推翻的依据会拿着同一个失效标识把第三阶段重试到底。
type CommercialRevalidationOutcome uint8

const (
	CommercialRevalidationOutcomeInvalid CommercialRevalidationOutcome = iota
	CommercialBasisStillValid
	CommercialBasisSuperseded
	CommercialRevalidationUndetermined
)

func (outcome CommercialRevalidationOutcome) String() string {
	switch outcome {
	case CommercialBasisStillValid:
		return "STILL_VALID"
	case CommercialBasisSuperseded:
		return "SUPERSEDED"
	case CommercialRevalidationUndetermined:
		return "UNDETERMINED"
	default:
		return ""
	}
}

// CommercialRevalidation 只在`仍然成立`时携带解析。`已失效`不带：交回一份，调用方会以为
// 可以继续用它，而用例明写「不能继续使用或覆盖原历史」。
type CommercialRevalidation struct {
	Outcome    CommercialRevalidationOutcome
	Resolution CommercialBasisResolution
	Reason     domain.CheckReason
}

// CommercialBasisResolver 是 parcel-shipment 视角下的 party-commercial 三步协议。
//
// 三个方法对应 `UC-PC-002` 的三个阶段，用例第 114 行要求「第一阶段解析与第二阶段逐项 `asOf`
// 必须在类型和测试中可见」，第 87 行要求实现的是「两阶段端口」。它们不合成一个带开关的方法：
// 三步由三个不同时刻的事件触发——开始判断、逐项形成时点、即将提交决定——合成会让编排依赖
// 它当轮根本不会走的分支，也会把提交前重解提前到判断开始时做（ADR-0027）。
type CommercialBasisResolver interface {
	ResolveCommercialBasis(ctx context.Context, query CommercialBasisQuery) (CommercialBasisResolution, error)
	FormJudgmentAsOf(ctx context.Context, query JudgmentAsOfQuery) (JudgmentAsOfFormation, error)
	RevalidateCommercialBasis(ctx context.Context, query CommercialRevalidationQuery) (CommercialRevalidation, error)
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

// ReachabilityOutcome 是 network-routing 一次可达性答复在本上下文的落点。
//
// `请求冲突`与`未受理`不译成 error。它们是业务答案而非技术故障——调用方必须能据以纠正自己
// 这次请求，而不是当作故障重试。译成 error 之后消费侧只剩「依赖答不出」一格，续办路径是
// 内部重试，正是那句话禁止的事：对着一个拼错的请求重试到底，冲突也不会消失。
type ReachabilityOutcome uint8

const (
	ReachabilityOutcomeInvalid ReachabilityOutcome = iota
	ReachabilityAssessed
	ReachabilityNotFormed
	ReachabilityRequestConflict
	ReachabilityRequestNotAccepted
)

func (outcome ReachabilityOutcome) String() string {
	switch outcome {
	case ReachabilityAssessed:
		return "ASSESSED"
	case ReachabilityNotFormed:
		return "NOT_FORMED"
	case ReachabilityRequestConflict:
		return "REQUEST_CONFLICT"
	case ReachabilityRequestNotAccepted:
		return "REQUEST_NOT_ACCEPTED"
	default:
		return ""
	}
}

// ReachabilityAssessment 只在`已判断`时携带判断，其余一律不带：交回一份零值判断，编排会把
// 一次没作出的判断记到接受判断任务上。
//
// 提供方的`首次形成`、`重放既有`与`不适用`都落在`已判断`。前两者交回的是同一份判断，重放与
// 首次对本上下文没有分别；`不适用`的分别由判断自身的取值与所携依据带过来，不必在这一层再
// 声明一次——`ReachabilityValue` 已经有那一格。
//
// Reason 是提供方给的稳定原因引用，非`已判断`时给出，本上下文原样记录，不重新解释。
type ReachabilityAssessment struct {
	Outcome  ReachabilityOutcome
	Judgment domain.ReachabilityJudgment
	Reason   domain.CheckReason
}

// ReachabilityAssessor 是 parcel-shipment 视角下的 network-routing 可达性答复。没有一个
// 取值是接受决定，本上下文也不得在这里据其推导出一个。
type ReachabilityAssessor interface {
	AssessParcelReachability(ctx context.Context, request ReachabilityRequest) (ReachabilityAssessment, error)
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

// PreAcceptanceControlOutcome 是 settlement-accounting 一次接受前控制答复在本上下文的落点。
//
// 与可达性同一条分界：`请求冲突`与`未受理`是业务答案，提供方明写「调用方必须能据以纠正，而
// 不是当作故障重试」。译成 error 会让它们并进「依赖答不出」，续办路径变成内部重试。
type PreAcceptanceControlOutcome uint8

const (
	PreAcceptanceControlOutcomeInvalid PreAcceptanceControlOutcome = iota
	PreAcceptanceControlFormed
	PreAcceptanceControlNotFormed
	PreAcceptanceControlRequestConflict
	PreAcceptanceControlRequestNotAccepted
)

func (outcome PreAcceptanceControlOutcome) String() string {
	switch outcome {
	case PreAcceptanceControlFormed:
		return "FORMED"
	case PreAcceptanceControlNotFormed:
		return "NOT_FORMED"
	case PreAcceptanceControlRequestConflict:
		return "REQUEST_CONFLICT"
	case PreAcceptanceControlRequestNotAccepted:
		return "REQUEST_NOT_ACCEPTED"
	default:
		return ""
	}
}

// PreAcceptanceControlAssessment 只在`已形成`时携带结果。提供方的`已执行`与`明确无控制`
// 都落在`已形成`：两者的分别由 FinancialControlOutcome 与所携依据带过来。
//
// 其余三种一律不带结果。交回一个零值结果正是用例禁止的默认放行——一次没能执行的控制会因此
// 看起来像通过了。
type PreAcceptanceControlAssessment struct {
	Outcome PreAcceptanceControlOutcome
	Result  domain.FinancialControlResult
	Reason  domain.CheckReason
}

// PreAcceptanceFinancialController 是 parcel-shipment 视角下的接受前财务控制。没有一个
// 取值是接受决定，本上下文也不得在这里据其推导出一个。
//
// 依赖调不通仍作为错误返回。把它读成`明确无控制`正是用例禁止的默认放行：一次故障会因此
// 变成一个看起来通过了的接受前控制。
type PreAcceptanceFinancialController interface {
	ApplyPreAcceptanceFinancialControl(
		ctx context.Context,
		request FinancialControlRequest,
	) (PreAcceptanceControlAssessment, error)
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

// SourceDataVersionHandoffIntent 是步骤 9 交给下游的那份引用。
//
// 它只携带引用与范围，不携带资料内容：跨上下文只传自己拥有的事实与引用，`UC-CC-002` 等下游按
// 各自的业务时间与门禁重新判断，本上下文既不替它们判断，也不把客户声明复制过去。
//
// Adoption 与 Version 一起交出，两者在分叉时并不是同一个。下游要消费的是「此刻该用哪一份」，
// 只发刚形成的那个版本号，会让它把一份尚未合并的支线当成当前资料。
type SourceDataVersionHandoffIntent struct {
	Identity domain.SourceIdentity
	Version  domain.SourceDataVersionID
	Scope    domain.SourceDataScope
	Adoption domain.SourceDataAdoptionJudgment
}

// SourceDataVersionHandoff 把一份已形成的客户原始资料版本引用交给适用下游。
//
// 一份版本发一份意图，而不是逐下游各设一个端口：哪些下游该重新判断，取决于范围、阶段与各自的
// 门禁，那是下游自己的判断。按下游拆端口会把那份判断搬进本上下文，而 `UC-PS-002` 明写本用例
// 「不形成关务、节点、运输、路由或财务决定」。
//
// 意图由版本标识认领，因此重发的是同一份而不是第二份——`AT-PS-031` 要的「版本只形成一次；仅
// 重试同一发布意图」正落在这里。首次交接失败时编排停在`技术未形成`，版本不因此重形成一遍。
//
// 它今天没有实现：outbox 与事务发布侧仍阻断于 ADR-0017 的 Bento 持久化闸门，端口接口属机制
// 半边因而可以先定，唯一的实现是测试用的确定性替身。
type SourceDataVersionHandoff interface {
	HandOffSourceDataVersion(ctx context.Context, intent SourceDataVersionHandoffIntent) error
}

// AcceptanceJudgmentRecorder 把一个已采用的判断记到它所推进的那份委托的接受判断任务上。
//
// RecordProcessingAttempt 记的是没能推进的那一轮。用例要求任务「追加判断与处理尝试」两样
// 都留：只留成功的判断，一份卡了十轮的委托看起来会和刚建单的一模一样。
type AcceptanceJudgmentRecorder interface {
	RecordReachabilityJudgment(ctx context.Context, requestID domain.ShipmentRequestID, judgment domain.ReachabilityJudgment) error
	RecordFinancialControlResult(ctx context.Context, requestID domain.ShipmentRequestID, result domain.FinancialControlResult) error
	RecordProcessingAttempt(ctx context.Context, requestID domain.ShipmentRequestID, attempt domain.ProcessingAttempt) error
	// RecordAdoptedCommercialResolution 记下本轮采用的那次商业解析，供提交决定前按它重解。
	//
	// 它与三个判断记录方法分开：解析在任何一项判断之前就被采用，两类判断也共用同一次解析，
	// 挂到某一个判断的记录上会让另一类判断的轮次看起来没有采用过依据。重复记录同一标识是
	// 幂等的——一份委托的多轮判断本就该采用同一次解析。
	RecordAdoptedCommercialResolution(ctx context.Context, requestID domain.ShipmentRequestID, resolution domain.CommercialResolutionID) error
}
