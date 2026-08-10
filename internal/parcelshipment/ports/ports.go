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

// AcceptanceJudgmentRecorder 把一个已采用的判断记到它所推进的那份委托的接受判断任务上。
//
// RecordProcessingAttempt 记的是没能推进的那一轮。用例要求任务「追加判断与处理尝试」两样
// 都留：只留成功的判断，一份卡了十轮的委托看起来会和刚建单的一模一样。
type AcceptanceJudgmentRecorder interface {
	RecordReachabilityJudgment(ctx context.Context, requestID domain.ShipmentRequestID, judgment domain.ReachabilityJudgment) error
	RecordFinancialControlResult(ctx context.Context, requestID domain.ShipmentRequestID, result domain.FinancialControlResult) error
	RecordProcessingAttempt(ctx context.Context, requestID domain.ShipmentRequestID, attempt domain.ProcessingAttempt) error
}
