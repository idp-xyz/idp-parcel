package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// InitialRouteNotFormedRecord 是未形成登记读回的一行，只带决定面要用的件：判断键、最近一次停住的原因、那次交接的关联，以及
// 并列时并列的那几家（ADR-0309 决定一）。全部候选、打平的金额与缺口是给人看的，走读面的行模型，不在这里重建成领域对象。
type InitialRouteNotFormedRecord struct {
	Key         domain.InitialRouteJudgmentKey
	Correlation domain.RequestCorrelationID
	Reason      InitialRouteNotFormedReason
	// Tied 只在 Reason 是并列时给出。
	Tied       []domain.CandidateID
	AcceptedAt time.Time
	SettledAt  time.Time
}

// InitialRouteNotFormedRead 读回未形成登记。登记行答的是一件包裹「最近一次停住的原因」，所以读回的就是最新那一次，旧的已被
// 改写。
type InitialRouteNotFormedRead interface {
	// FindNotFormed 按判断键读回。没有登记行答 false；行重建不出来交 ErrStoredRouteRecordUnreadable。
	FindNotFormed(ctx context.Context, key domain.InitialRouteJudgmentKey) (InitialRouteNotFormedRecord, bool, error)
	// FindLatestNotFormedOfParcel 按（租户，包裹）读回最近入账的那一行：决定面只指名包裹，判断键各维从这一行取。
	FindLatestNotFormedOfParcel(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (InitialRouteNotFormedRecord, bool, error)
}

// InitialRouteTieDecision 是授权角色在最低成本并列的候选之间择一后留下的决定证据（ADR-0309 决定四）：所择候选、决定依据、
// 决定方与决定时刻。计划本体住 initial_route，这一行与它同键，答「谁凭什么把并列裁给了谁」。
type InitialRouteTieDecision struct {
	Key         domain.InitialRouteJudgmentKey
	Candidate   domain.CandidateID
	PlanVersion domain.RoutePlanVersionID
	Basis       domain.RerouteDecisionBasisReference
	DecidedBy   domain.AuthorizedRoleReference
	DecidedAt   time.Time
}

// InitialRouteTieDecisions 落并列决定的证据。写口要事务：这一行与同键的 initial_route 行、发布意图同笔提交，脱开事务写会留下
// 一条没有计划的决定或一份没有决定的计划。
type InitialRouteTieDecisions interface {
	// Record 写下一行。同一判断键已有一行是编排缺陷（initial_route 先成才走到这里），响亮报错，不翻成业务答案。
	Record(ctx context.Context, decision InitialRouteTieDecision) error
}

// InitialRouteTieState 是并列在读面上的三格。读面只答此刻的大概，决定成不成立仍由决定口在判断键锁里判。
type InitialRouteTieState string

const (
	// InitialRouteTiePending：未形成登记最近一次停在并列，判断库还没有这件包裹的初始路由。
	InitialRouteTiePending InitialRouteTieState = "PENDING"
	// InitialRouteTieDecided：授权角色已据它形成了首个计划。
	InitialRouteTieDecided InitialRouteTieState = "DECIDED"
	// InitialRouteTieFormedOtherwise：判断库已有这件包裹的初始路由，却不是经决定面形成的（并列之后重判、成本变了选出了唯一一条）。
	InitialRouteTieFormedOtherwise InitialRouteTieState = "FORMED_OTHERWISE"
)

// InitialRouteTieCatalogueRow 是并列读面的一行：判断键、全部候选与各自的依据、并列的那几家与打平的金额，以及决定状态。
type InitialRouteTieCatalogueRow struct {
	CustomerAccountID  string
	ShipmentRequestID  string
	AcceptanceBaseline string
	DeclaredParcelID   string
	ServicePurpose     string
	Candidates         []RerouteSuggestionCandidateRow
	Tied               []InitialRouteTieCostRow
	AcceptedAt         time.Time
	SettledAt          time.Time
	State              InitialRouteTieState
	// Decision 只在已决定时在场。
	Decision    InitialRouteTieDecisionRow
	HasDecision bool
}

// InitialRouteTieCostRow 是并列的一家与它打平的金额。
type InitialRouteTieCostRow struct {
	Candidate   string
	AmountMinor int64
	Currency    string
}

// InitialRouteTieDecisionRow 是并列上已形成的决定。
type InitialRouteTieDecisionRow struct {
	Candidate   string
	PlanVersion string
	DecidedBy   string
	Basis       string
	DecidedAt   time.Time
}

// InitialRouteTieCatalogueRead 是并列的列表读端口。租户在签名上；limit 非正拒；空册如实答空列表。
type InitialRouteTieCatalogueRead interface {
	ListInitialRouteTies(ctx context.Context, tenant domain.TenantID, limit int) ([]InitialRouteTieCatalogueRow, error)
}

// ParseInitialRouteNotFormedReason 按字面认未形成原因，库里存的就是 String() 给的这些词。认不出的交 false，读回方当坏数据。
func ParseInitialRouteNotFormedReason(text string) (InitialRouteNotFormedReason, bool) {
	for reason := NotFormedEvidenceNotConfigured; reason <= NotFormedCostSourceNotConfigured; reason++ {
		if reason.String() == text {
			return reason, true
		}
	}
	return InitialRouteNotFormedReasonInvalid, false
}
