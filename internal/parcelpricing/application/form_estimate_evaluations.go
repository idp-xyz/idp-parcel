// form_estimate_evaluations.go 编排 UC-PP-001「按假设包裹形成试算评价」（ADR-0152）：发起方声明一件假设包裹，本上下文
// 取全部适用价卡、逐卡造试算输入、与正式评价共用补齐读数、以纯函数形成评价。计算一步都不在这里。
//
// 不入册、不交付都在结构上（ADR-0152 决定二）：本编排的依赖里没有评价库写口，也没有 EvaluationHandoff——试算不形成
// 承诺、不进结算，而正式评价的交付是无条件的、结算侧不按对象种类过滤，装进来一次就是一笔可被采用的评价。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ApplicablePriceCards 是试算要的那一口：只装载，不登记。收窄成这一个方法，是让依赖里结构上没有价卡登记册的写口
// （ports.PriceCardCatalog 同时带着 Register）。
type ApplicablePriceCards interface {
	LoadApplicable(ctx context.Context, tenant domain.TenantID, direction domain.PricingDirection, scope domain.PricingScopeID, asOf time.Time) ([]domain.PricingPlanVersion, error)
}

// FormEstimateEvaluationsOutcome 是试算的封闭结果，只说编排、不替评价说话（ADR-0152 决定七）；按恢复动作分格（ADR-0029）。
type FormEstimateEvaluationsOutcome uint8

const (
	FormEstimateEvaluationsOutcomeInvalid FormEstimateEvaluationsOutcome = iota
	// EstimateEvaluationsFormed：逐卡交回结果，卡与卡之间不排序、不标首选。
	EstimateEvaluationsFormed
	// EstimatePriceCardNotConfigured：此范围、方向、时点下没有适用价卡。
	EstimatePriceCardNotConfigured
	// EstimatePriceCardApplicabilityConflict：同一方案身份两版同时适用，候选交人裁。
	EstimatePriceCardApplicabilityConflict
	// EstimateNotAccepted：声明不成形。
	EstimateNotAccepted
	// EstimateUndecided：依赖读不回，形成与否未知，停在哪一口见 Reason。
	EstimateUndecided
)

func (outcome FormEstimateEvaluationsOutcome) String() string {
	switch outcome {
	case EstimateEvaluationsFormed:
		return "FORMED"
	case EstimatePriceCardNotConfigured:
		return "PRICE_CARD_NOT_CONFIGURED"
	case EstimatePriceCardApplicabilityConflict:
		return "PRICE_CARD_APPLICABILITY_CONFLICT"
	case EstimateNotAccepted:
		return "NOT_ACCEPTED"
	case EstimateUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// FormEstimateEvaluationsUndecidedReason 指名未决停在哪一口。
type FormEstimateEvaluationsUndecidedReason uint8

const (
	FormEstimateEvaluationsUndecidedReasonNone FormEstimateEvaluationsUndecidedReason = iota
	EstimatePriceCardLoadUnavailable
	EstimateConflictCandidatesUnavailable
	EstimateReadingCompletionUnavailable
)

func (reason FormEstimateEvaluationsUndecidedReason) String() string {
	switch reason {
	case EstimatePriceCardLoadUnavailable:
		return "PRICE_CARD_LOAD_UNAVAILABLE"
	case EstimateConflictCandidatesUnavailable:
		return "CONFLICT_CANDIDATES_UNAVAILABLE"
	case EstimateReadingCompletionUnavailable:
		return "READING_COMPLETION_UNAVAILABLE"
	default:
		return ""
	}
}

// EstimateCandidateAnswer 是「已形成」下逐卡一格。
type EstimateCandidateAnswer uint8

const (
	EstimateCandidateAnswerInvalid EstimateCandidateAnswer = iota
	// EstimateCandidateEvaluated：交回那份试算评价，其状态五格原样透出。
	EstimateCandidateEvaluated
	// EstimateCandidateInputIncomplete：这张卡要的某一格发起方没给，Missing 点名。
	EstimateCandidateInputIncomplete
)

func (answer EstimateCandidateAnswer) String() string {
	switch answer {
	case EstimateCandidateEvaluated:
		return "EVALUATED"
	case EstimateCandidateInputIncomplete:
		return "INPUT_INCOMPLETE"
	default:
		return ""
	}
}

// 输入缺项的封闭码：稳定、机器可读，页面自译成中文。
const (
	EstimateMissingZone        = "ZONE"
	EstimateMissingPostalRoute = "POSTAL_ROUTE"
)

// EstimateCandidate 是一张适用价卡的结果。
type EstimateCandidate struct {
	Plan       domain.VersionReference
	Answer     EstimateCandidateAnswer
	Evaluation domain.PricingEvaluation
	Missing    []string
}

// FormEstimateEvaluationsCommand 是一份试算声明在本上下文词汇里的转述（UC-PP-001「试算声明」）。租户由接入面从信封交进来；
// 其余各项由发起方声明、不给默认：分区为空串、邮编路线与结算币种为 nil 即未声明。
type FormEstimateEvaluationsCommand struct {
	Tenant     domain.TenantID
	Scope      domain.PricingScopeID
	Direction  domain.PricingDirection
	BasisAt    time.Time
	Weight     domain.Weight
	Dimensions *domain.Dimensions
	Zone       string
	Route      *domain.PostalRoute
	Settlement *domain.Currency
}

// FormEstimateEvaluationsResult 是入口的答复。字段导出、不设访问器，形状随 FormEvaluationFromRequestResult。
type FormEstimateEvaluationsResult struct {
	Outcome FormEstimateEvaluationsOutcome
	// Reason 只在 EstimateUndecided 时非零。
	Reason FormEstimateEvaluationsUndecidedReason
	// Candidates 只在已形成时有值，按价卡装载的顺序，顺序没有优劣含义。
	Candidates []EstimateCandidate
	// Conflict 只在适用冲突时有值：同时适用的各版价卡引用。
	Conflict []domain.VersionReference
}

// FormEstimateEvaluationsDeps 是试算的依赖。价卡装载、在用解析与时钟必备；补齐读数的两对与正式评价同形、各自成对可选。
// 这里刻意没有评价库与交付口（文件头）。
type FormEstimateEvaluationsDeps struct {
	PriceCards ApplicablePriceCards
	// InForce 只在装载报「同一方案两版同时适用」时问一次，取交人裁的候选——那正是它答冲突时交候选的本义。
	InForce          ports.PriceCardInForceResolver
	Clock            ports.Clock
	SeriesInForce    ports.ReferenceSeriesInForceResolver
	SeriesVersions   ports.ReferenceSeriesRegister
	CatalogueInForce ports.ReferenceCatalogueInForceResolver
	Catalogues       ports.ReferenceCatalogueRegister
}

// FormEstimateEvaluationsHandler 是试算编排。
type FormEstimateEvaluationsHandler struct {
	deps       FormEstimateEvaluationsDeps
	completion readingCompletion
}

// NewFormEstimateEvaluationsHandler 构造期拒 nil 与半套装配：漏装一口在这里就报出来。
func NewFormEstimateEvaluationsHandler(deps FormEstimateEvaluationsDeps) (*FormEstimateEvaluationsHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"applicable price cards", deps.PriceCards == nil},
		{"price card in-force resolver", deps.InForce == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	completion := readingCompletion{
		clock:            deps.Clock,
		inForce:          deps.SeriesInForce,
		seriesVersions:   deps.SeriesVersions,
		catalogueInForce: deps.CatalogueInForce,
		catalogues:       deps.Catalogues,
	}
	if err := completion.halfWired(); err != nil {
		return nil, err
	}
	return &FormEstimateEvaluationsHandler{deps: deps, completion: completion}, nil
}

// Handle 把一份试算声明推进到答案：受理 → 取全部适用价卡 → 逐卡造输入、补齐读数、纯函数评价 → 汇总。
func (handler *FormEstimateEvaluationsHandler) Handle(
	ctx context.Context,
	command FormEstimateEvaluationsCommand,
) (FormEstimateEvaluationsResult, error) {
	if !command.accepted() {
		return FormEstimateEvaluationsResult{Outcome: EstimateNotAccepted}, nil
	}
	plans, err := handler.deps.PriceCards.LoadApplicable(ctx, command.Tenant, command.Direction, command.Scope, command.BasisAt)
	if errors.Is(err, ports.ErrAmbiguousPriceCard) {
		return handler.applicabilityConflict(ctx, command), nil
	}
	if err != nil {
		return undecidedEstimate(EstimatePriceCardLoadUnavailable), nil
	}
	if len(plans) == 0 {
		return FormEstimateEvaluationsResult{Outcome: EstimatePriceCardNotConfigured}, nil
	}

	candidates := make([]EstimateCandidate, 0, len(plans))
	for _, plan := range plans {
		candidate, stopped, err := handler.estimate(ctx, command, plan)
		if err != nil {
			return FormEstimateEvaluationsResult{}, err
		}
		if stopped != nil {
			return *stopped, nil
		}
		candidates = append(candidates, candidate)
	}
	return FormEstimateEvaluationsResult{Outcome: EstimateEvaluationsFormed, Candidates: candidates}, nil
}

// applicabilityConflict 是装载报「同一方案两版同时适用」之后的那一步。装载把它作为登记册数据错误交回，恢复动作是交价卡
// 治理责任方修登记册——与适用冲突同一个，所以按恢复动作译成适用冲突（ADR-0029）。候选问在用解析口：它答冲突时交回候选，
// 正是为了交人裁。两口答得不一致（它说不冲突）不猜哪边对，照未决处置。
func (handler *FormEstimateEvaluationsHandler) applicabilityConflict(
	ctx context.Context,
	command FormEstimateEvaluationsCommand,
) FormEstimateEvaluationsResult {
	resolution, err := handler.deps.InForce.ResolveInForce(ctx, command.Tenant, command.Scope, command.Direction,
		pairedPurpose(command.Direction), command.BasisAt)
	if err != nil || resolution.Outcome != ports.PriceCardApplicabilityConflict {
		return undecidedEstimate(EstimateConflictCandidatesUnavailable)
	}
	return FormEstimateEvaluationsResult{
		Outcome:  EstimatePriceCardApplicabilityConflict,
		Conflict: append([]domain.VersionReference(nil), resolution.Candidates...),
	}
}

// pairedPurpose 按领域配对表取与方向成对的计算目的（首发一一对应，CONTEXT）；配对表由领域一处持有。
func pairedPurpose(direction domain.PricingDirection) domain.PricingPurpose {
	for _, purpose := range []domain.PricingPurpose{
		domain.PricingPurposeCustomerCharge,
		domain.PricingPurposeSupplierCost,
		domain.PricingPurposeInternalPrice,
	} {
		if purpose.PairsWithDirection(direction) {
			return purpose
		}
	}
	return ""
}

// estimate 为一张卡造输入、补齐读数并形成评价。stopped 非 nil 时它就是整份入口的答案（依赖故障）。
func (handler *FormEstimateEvaluationsHandler) estimate(
	ctx context.Context,
	command FormEstimateEvaluationsCommand,
	plan domain.PricingPlanVersion,
) (EstimateCandidate, *FormEstimateEvaluationsResult, error) {
	candidate := EstimateCandidate{Plan: plan.Reference()}
	identity := estimateIdentity(command, plan)
	subject, err := domain.NewEstimateSubject(identity)
	if err != nil {
		return EstimateCandidate{}, nil, fmt.Errorf("estimate subject: %w", err)
	}
	input, missing, err := estimateInput(command, plan, subject)
	if err != nil {
		return EstimateCandidate{}, nil, fmt.Errorf("estimate input: %w", err)
	}
	if len(missing) > 0 {
		candidate.Answer = EstimateCandidateInputIncomplete
		candidate.Missing = missing
		return candidate, nil, nil
	}
	id, err := domain.NewEvaluationID(identity)
	if err != nil {
		return EstimateCandidate{}, nil, fmt.Errorf("estimate evaluation ID: %w", err)
	}
	request, err := domain.NewEvaluationRequest(id, plan, input, domain.EvidenceSynthetic)
	if err != nil {
		return EstimateCandidate{}, nil, fmt.Errorf("estimate evaluation request: %w", err)
	}
	request, _, err = handler.completion.complete(ctx, request)
	if err != nil {
		stopped := undecidedEstimate(EstimateReadingCompletionUnavailable)
		return EstimateCandidate{}, &stopped, nil
	}
	candidate.Answer = EstimateCandidateEvaluated
	candidate.Evaluation = domain.EvaluatePricing(request)
	return candidate, nil, nil
}

// estimateInput 按卡的目录绑定选分区给法（ADR-0152 决定五）：绑了分区目录的卡用邮编路线、分区由目录解析；没绑的卡用
// 发起方给的分区，另绑了偏远档位目录时还要邮编路线。该给的没给交回缺项、不造输入——不拿另一种给法顶替，也不给默认分区。
func estimateInput(
	command FormEstimateEvaluationsCommand,
	plan domain.PricingPlanVersion,
	subject domain.EvaluationSubject,
) (domain.PricingInputSnapshot, []string, error) {
	zoneBound, tierBound := false, false
	for _, link := range plan.Structures().ReferenceCatalogues() {
		switch link.Kind() {
		case domain.CatalogueKindZone:
			zoneBound = true
		case domain.CatalogueKindRemoteTier:
			tierBound = true
		}
	}

	var input domain.PricingInputSnapshot
	var err error
	if zoneBound {
		if command.Route == nil {
			return domain.PricingInputSnapshot{}, []string{EstimateMissingPostalRoute}, nil
		}
		input, err = domain.NewPostalPricingInputSnapshot(command.Tenant, command.Scope, subject, *command.Route,
			command.Weight, command.Dimensions, command.BasisAt)
	} else {
		missing := make([]string, 0, 2)
		if command.Zone == "" {
			missing = append(missing, EstimateMissingZone)
		}
		if tierBound && command.Route == nil {
			missing = append(missing, EstimateMissingPostalRoute)
		}
		if len(missing) > 0 {
			return domain.PricingInputSnapshot{}, missing, nil
		}
		input, err = domain.NewPricingInputSnapshot(command.Tenant, command.Scope, subject, command.Zone,
			command.Weight, command.Dimensions, command.BasisAt)
		if err == nil && tierBound {
			input, err = input.WithPostalRoute(*command.Route)
		}
	}
	if err != nil {
		return domain.PricingInputSnapshot{}, nil, err
	}
	if command.Settlement != nil {
		input, err = input.WithSettlementCurrency(*command.Settlement)
		if err != nil {
			return domain.PricingInputSnapshot{}, nil, err
		}
	}
	return input, nil, nil
}

// estimateIdentity 由租户、声明各项与价卡版本引用确定性派生试算对象引用与评价标识（ADR-0152 决定五）：同声明同卡得同一
// 标识，纯评价可比对可复算。尺寸按排序后的三边取：同一件包裹换个量法不是另一件。
func estimateIdentity(command FormEstimateEvaluationsCommand, plan domain.PricingPlanVersion) string {
	parts := []string{
		command.Tenant.String(),
		command.Scope.String(),
		command.Direction.String(),
		command.BasisAt.UTC().Format(time.RFC3339Nano),
		command.Weight.Value().String(),
		command.Weight.Unit().String(),
		command.Zone,
		plan.Reference().ID(),
		plan.Reference().Version(),
	}
	if command.Dimensions != nil {
		parts = append(parts,
			command.Dimensions.LongestSide().Value().String(),
			command.Dimensions.SecondLongestSide().Value().String(),
			command.Dimensions.ShortestSide().Value().String(),
			command.Dimensions.Unit().String())
	} else {
		parts = append(parts, "-")
	}
	if command.Route != nil {
		parts = append(parts, command.Route.Origin(), command.Route.Destination())
	} else {
		parts = append(parts, "-")
	}
	if command.Settlement != nil {
		parts = append(parts, command.Settlement.String())
	} else {
		parts = append(parts, "-")
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "EST-" + hex.EncodeToString(digest[:8])
}

// accepted 是受理门：必需项都在、方向在配对表里、分区不带两侧空白。可缺的几项（尺寸、邮编路线、结算币种、分区本身）不在这里判——
// 哪张卡要哪一格逐卡定（estimateInput），在这里拦会把「这张卡不要分区」的声明也拒掉。
func (command FormEstimateEvaluationsCommand) accepted() bool {
	if command.Tenant.String() == "" || command.Scope.String() == "" || command.BasisAt.IsZero() {
		return false
	}
	if pairedPurpose(command.Direction) == "" || command.Weight.Unit().String() == "" {
		return false
	}
	return strings.TrimSpace(command.Zone) == command.Zone
}

func undecidedEstimate(reason FormEstimateEvaluationsUndecidedReason) FormEstimateEvaluationsResult {
	return FormEstimateEvaluationsResult{Outcome: EstimateUndecided, Reason: reason}
}
