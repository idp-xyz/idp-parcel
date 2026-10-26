package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// AuthorizedRerouteDecision 是授权角色据一份改路建议形成的改路决定落册的那一份（ADR-0284 决定五）。Suggestion 是建议所在那次
// 复核的触发关联：一份建议至多一份决定，由它与租户承担。决定方、原计划、新计划与决定时刻在 Decision 里；Candidate 是所择候选，
// 与新计划的选中候选同一个；Basis 是决定方给的依据引用。
type AuthorizedRerouteDecision struct {
	Suggestion domain.RequestCorrelationID
	Key        domain.InitialRouteJudgmentKey
	Candidate  domain.CandidateID
	Basis      domain.RerouteDecisionBasisReference
	Decision   domain.RerouteDecision
}

// AuthorizedRerouteDecisionSaveOutcome 与其余判断库同一套写入代数（ADR-0031）：同一份建议已有决定是业务答案，不是错误。
type AuthorizedRerouteDecisionSaveOutcome uint8

const (
	AuthorizedRerouteDecisionSaveOutcomeInvalid AuthorizedRerouteDecisionSaveOutcome = iota
	AuthorizedRerouteDecisionSaved
	AuthorizedRerouteDecisionAlreadyRecorded
)

// AuthorizedRerouteDecisionStore 是授权改路决定册。读写都在决定编排的那一笔事务里、判断键锁之后（ReassessmentStore.LatestTriggerAt
// 取的那一把）：锁外读到的「没有决定」「没被取代」，落册之前可能已被另一笔换掉。
type AuthorizedRerouteDecisionStore interface {
	// FindBySuggestion 按（租户，建议所在复核的触发关联）取回已形成的决定；否定结果不区分不存在与他租户。决定行重建不出来
	// 交 ErrStoredRouteRecordUnreadable。
	FindBySuggestion(
		ctx context.Context,
		tenant domain.TenantID,
		suggestion domain.RequestCorrelationID,
	) (AuthorizedRerouteDecision, bool, error)
	Save(ctx context.Context, decision AuthorizedRerouteDecision) (AuthorizedRerouteDecisionSaveOutcome, error)
	// SuggestionOvertaken 答同一判断键上有没有建议所在那次以外的复核，其触发业务时间不早于 at（ADR-0284 决定二）。同一时刻
	// 也算：两次控制落在同一时刻时没有先后可比，不替它们排一个没有业务依据的次序。
	SuggestionOvertaken(
		ctx context.Context,
		key domain.InitialRouteJudgmentKey,
		suggestion domain.RequestCorrelationID,
		at time.Time,
	) (bool, error)
}

// AcceptanceBaselineGeo 是一件当前已接受包裹在判断键上的引用与受理基线那一版提交的地理投影：只有地理维，地址本体与身份内容
// 都不在这里（ADR-0075 决定二）。
type AcceptanceBaselineGeo struct {
	CustomerAccountID  domain.CustomerAccountID
	ShipmentRequestID  domain.ShipmentRequestID
	AcceptanceBaseline domain.AcceptanceBaselineReference
	Geo                domain.GeoResolutionProjection
}

// ErrAcceptanceBaselineGeoUnreadable 说此刻读不动 parcel-shipment：依赖故障，重发推得动。其余错误（反查歧义、接受基线自相
// 矛盾、译不出）重发不会变，原样上抛。
var ErrAcceptanceBaselineGeoUnreadable = errors.New("network routing: the acceptance baseline geo projection is not readable right now")

// AcceptanceBaselineGeoSource 是授权改路决定取所携地理投影的地方（ADR-0284 决定三）：决定由操作者发起，没有小包托运的请求可
// 随带投影，经消费侧适配器取受理基线那一版。found=false 即这件包裹不是当前已接受包裹。
type AcceptanceBaselineGeoSource interface {
	LoadAcceptanceBaselineGeo(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (AcceptanceBaselineGeo, bool, error)
}

// RerouteSuggestionState 是改路建议在读面上的三格（ADR-0284 决定六）。读面只答此刻的大概，决定成不成立仍由决定口在锁里判。
type RerouteSuggestionState string

const (
	// RerouteSuggestionPending：建议所在复核仍是判断键上业务时间最晚的那一次，也还没有决定。
	RerouteSuggestionPending RerouteSuggestionState = "PENDING"
	// RerouteSuggestionDecided：已有授权角色据它形成了决定。
	RerouteSuggestionDecided RerouteSuggestionState = "DECIDED"
	// RerouteSuggestionSuperseded：判断键上另有一次触发业务时间不早于它的复核。
	RerouteSuggestionSuperseded RerouteSuggestionState = "SUPERSEDED"
)

// RerouteSuggestionCatalogueRow 是改路建议读面的一行：建议所在那次复核的检索列、建议本体（阻塞与候选）与决定状态。
type RerouteSuggestionCatalogueRow struct {
	CorrelationID      string
	CustomerAccountID  string
	ShipmentRequestID  string
	AcceptanceBaseline string
	DeclaredParcelID   string
	ServicePurpose     string
	ReviewedPlan       string
	LapseBasis         string
	Blockers           []string
	Candidates         []RerouteSuggestionCandidateRow
	SuggestedAt        time.Time
	TriggerOccurredAt  time.Time
	State              RerouteSuggestionState
	// Decision 只在已决定时在场。
	Decision    RerouteSuggestionDecisionRow
	HasDecision bool
}

// RerouteSuggestionCandidateRow 是建议里的一条候选：结局与依据是建议形成那一刻的评估，不是此刻的。
type RerouteSuggestionCandidateRow struct {
	ID      string
	Outcome string
	Reason  string
}

// RerouteSuggestionDecisionRow 是建议上已形成的决定。
type RerouteSuggestionDecisionRow struct {
	Candidate      string
	NewPlanVersion string
	DecidedBy      string
	Basis          string
	DecidedAt      time.Time
}

// RerouteSuggestionCatalogueRead 是改路建议的列表读端口。租户在签名上；limit 非正拒；空册如实答空列表。
type RerouteSuggestionCatalogueRead interface {
	ListRerouteSuggestions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]RerouteSuggestionCatalogueRow, error)
}
