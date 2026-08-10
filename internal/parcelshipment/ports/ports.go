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
type ShipmentRequestRepository interface {
	FindBySourceIdentity(ctx context.Context, identity domain.SourceIdentity) (domain.ShipmentRequest, bool, error)
	Insert(ctx context.Context, identity domain.SourceIdentity, request domain.ShipmentRequest) error
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

// CommercialBasisResolver 是 parcel-shipment 视角下的 party-commercial 判断。快照
// 无效即表示未取得唯一依据；具体原因属那个上下文的语言，本上下文不重新解释。
type CommercialBasisResolver interface {
	ResolveCommercialBasis(ctx context.Context, query CommercialBasisQuery) (domain.CommercialBasisSnapshot, error)
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

// AcceptanceJudgmentRecorder 把一个已采用的判断记到它所推进的那份委托的接受判断任务上。
type AcceptanceJudgmentRecorder interface {
	RecordReachabilityJudgment(ctx context.Context, requestID domain.ShipmentRequestID, judgment domain.ReachabilityJudgment) error
}
