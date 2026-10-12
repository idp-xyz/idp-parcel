// form_plan_given_estimate_evaluations.go 是对调用方给的价卡形成试算评价的入口（票 product-strategy-boundary/24；ADR-0152
// 决定二、三、五）：调用方交一张已解析的价卡版本、一组试算声明与证据层级，本入口逐件造输入、与正式评价共用补齐读数、以纯函数
// 形成评价，逐件交回评价或缺项。计算一步都不在这里。
//
// 选卡不在这里：卡按调用方自己的口径解析好再交进来（受理前估价的口径见 ADR-0179 决定三），所以依赖里没有价卡装载口，答复里
// 也没有「价卡未配置」「适用冲突」——那是选卡那一步的答案。与 UC-PP-001 那条编排（FormEstimateEvaluationsHandler）的不同还有：
// 试算对象引用由调用方给，不由声明派生；证据层级由调用方声明，不按定义记 S——受理前估价的输入是客户申报，不是假设包裹。
//
// 不入册、不交付都在结构上（ADR-0152 决定二）：依赖里没有评价库写口，也没有 EvaluationHandoff。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// FormPlanGivenEstimateEvaluationsOutcome 是本入口的封闭结果，只说编排、不替评价说话；按恢复动作分格（ADR-0029）。
type FormPlanGivenEstimateEvaluationsOutcome uint8

const (
	FormPlanGivenEstimateEvaluationsOutcomeInvalid FormPlanGivenEstimateEvaluationsOutcome = iota
	// PlanGivenEstimatesFormed：逐件交回结果，与声明同序。
	PlanGivenEstimatesFormed
	// PlanGivenEstimatesNotAccepted：命令不成形，一件都没算。
	PlanGivenEstimatesNotAccepted
	// PlanGivenEstimatesUndecided：补齐读数的依赖读不回，形成与否未知。
	PlanGivenEstimatesUndecided
)

func (outcome FormPlanGivenEstimateEvaluationsOutcome) String() string {
	switch outcome {
	case PlanGivenEstimatesFormed:
		return "FORMED"
	case PlanGivenEstimatesNotAccepted:
		return "NOT_ACCEPTED"
	case PlanGivenEstimatesUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// PlanGivenEstimateDeclaration 是一件试算声明。Subject 是调用方给的试算对象引用，本上下文不为它铸造身份；其余各项不给默认：
// 分区为空串、尺寸、邮编路线与结算币种为 nil 即未声明。这张卡要哪一格，按卡的目录绑定逐件定（estimateInput）。
type PlanGivenEstimateDeclaration struct {
	Subject    string
	BasisAt    time.Time
	Weight     domain.Weight
	Dimensions *domain.Dimensions
	Zone       string
	Route      *domain.PostalRoute
	Settlement *domain.Currency
}

// FormPlanGivenEstimateEvaluationsCommand 是一次调用：一张卡、一个证据层级、一组声明。租户由调用方从自己的信封交进来。计价
// 范围与价格方向取卡自己的，声明里不另给：卡已经选定，另给的只可能与卡一致或与卡相抵。
type FormPlanGivenEstimateEvaluationsCommand struct {
	Tenant       domain.TenantID
	Plan         domain.PricingPlanVersion
	Evidence     domain.EvidenceKind
	Declarations []PlanGivenEstimateDeclaration
}

// PlanGivenEstimate 是一件声明的结果。答案与缺项码沿用 UC-PP-001 逐卡那一格（EstimateCandidateAnswer、EstimateMissing*）：
// 说的是同一件事——这一份试算算了，或缺哪一格——不另立第二套词。
type PlanGivenEstimate struct {
	Subject    string
	Answer     EstimateCandidateAnswer
	Evaluation domain.PricingEvaluation
	Missing    []string
}

// FormPlanGivenEstimateEvaluationsResult 是入口的答复。字段导出、不设访问器，形状随 FormEstimateEvaluationsResult。
type FormPlanGivenEstimateEvaluationsResult struct {
	Outcome FormPlanGivenEstimateEvaluationsOutcome
	// Estimates 只在已形成时有值，与声明一一对应、同序。
	Estimates []PlanGivenEstimate
}

// FormPlanGivenEstimateEvaluationsDeps 是本入口的依赖。时钟必备；补齐读数的两对与正式评价同形、各自成对可选。这里刻意没有
// 评价库、交付口与价卡装载口（文件头）。
type FormPlanGivenEstimateEvaluationsDeps struct {
	Clock            ports.Clock
	SeriesInForce    ports.ReferenceSeriesInForceResolver
	SeriesVersions   ports.ReferenceSeriesRegister
	CatalogueInForce ports.ReferenceCatalogueInForceResolver
	Catalogues       ports.ReferenceCatalogueRegister
}

// FormPlanGivenEstimateEvaluationsHandler 是本入口的编排。
type FormPlanGivenEstimateEvaluationsHandler struct {
	completion readingCompletion
}

// NewFormPlanGivenEstimateEvaluationsHandler 构造期拒缺时钟与半套补齐装配：漏装一口在这里就报出来。
func NewFormPlanGivenEstimateEvaluationsHandler(deps FormPlanGivenEstimateEvaluationsDeps) (*FormPlanGivenEstimateEvaluationsHandler, error) {
	if deps.Clock == nil {
		return nil, fmt.Errorf("%w: clock", ErrNilDependency)
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
	return &FormPlanGivenEstimateEvaluationsHandler{completion: completion}, nil
}

// Handle 把一次调用推进到答案：受理 → 逐件造输入、补齐读数、纯函数评价。任一件补齐读数读不回即整份未决，与 UC-PP-001 同一处置：
// 只交回其余几件，一份缺件的答复会被当成完整的来用。
func (handler *FormPlanGivenEstimateEvaluationsHandler) Handle(
	ctx context.Context,
	command FormPlanGivenEstimateEvaluationsCommand,
) (FormPlanGivenEstimateEvaluationsResult, error) {
	if !command.accepted() {
		return FormPlanGivenEstimateEvaluationsResult{Outcome: PlanGivenEstimatesNotAccepted}, nil
	}
	estimates := make([]PlanGivenEstimate, 0, len(command.Declarations))
	for _, declaration := range command.Declarations {
		estimate, undecided, err := handler.estimate(ctx, command, declaration)
		if err != nil {
			return FormPlanGivenEstimateEvaluationsResult{}, err
		}
		if undecided {
			return FormPlanGivenEstimateEvaluationsResult{Outcome: PlanGivenEstimatesUndecided}, nil
		}
		estimates = append(estimates, estimate)
	}
	return FormPlanGivenEstimateEvaluationsResult{Outcome: PlanGivenEstimatesFormed, Estimates: estimates}, nil
}

// estimate 为一件声明造输入、补齐读数并形成评价。undecided 为真即补齐读数读不回。
func (handler *FormPlanGivenEstimateEvaluationsHandler) estimate(
	ctx context.Context,
	command FormPlanGivenEstimateEvaluationsCommand,
	declaration PlanGivenEstimateDeclaration,
) (PlanGivenEstimate, bool, error) {
	estimate := PlanGivenEstimate{Subject: declaration.Subject}
	translated := declaration.estimateCommand(command.Tenant, command.Plan)
	subject, err := domain.NewEstimateSubject(declaration.Subject)
	if err != nil {
		return PlanGivenEstimate{}, false, fmt.Errorf("plan-given estimate subject: %w", err)
	}
	input, missing, err := estimateInput(translated, command.Plan, subject)
	if err != nil {
		return PlanGivenEstimate{}, false, fmt.Errorf("plan-given estimate input: %w", err)
	}
	if len(missing) > 0 {
		estimate.Answer = EstimateCandidateInputIncomplete
		estimate.Missing = missing
		return estimate, false, nil
	}
	id, err := domain.NewEvaluationID(planGivenEstimateIdentity(declaration.Subject, translated, command.Plan))
	if err != nil {
		return PlanGivenEstimate{}, false, fmt.Errorf("plan-given estimate evaluation ID: %w", err)
	}
	request, err := domain.NewEvaluationRequest(id, command.Plan, input, command.Evidence)
	if err != nil {
		return PlanGivenEstimate{}, false, fmt.Errorf("plan-given estimate evaluation request: %w", err)
	}
	request, _, err = handler.completion.complete(ctx, request)
	if err != nil {
		return PlanGivenEstimate{}, true, nil
	}
	estimate.Answer = EstimateCandidateEvaluated
	estimate.Evaluation = domain.EvaluatePricing(request)
	return estimate, false, nil
}

// accepted 是受理门：租户、卡与证据层级都在，至少一件声明；每件的试算对象引用成形、一次调用里不重名，转述后过 UC-PP-001 同一道
// 受理门。证据层级不给默认，没声明即拒。引用不重名，是因为试算对象只在该次试算内有身份（PP CONTEXT「评价对象」），两件同名就分
// 不清哪份评价是哪一件的。
func (command FormPlanGivenEstimateEvaluationsCommand) accepted() bool {
	if command.Tenant.String() == "" || command.Plan.Reference().ID() == "" || !command.Evidence.Declared() || len(command.Declarations) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(command.Declarations))
	for _, declaration := range command.Declarations {
		if _, err := domain.NewEstimateSubject(declaration.Subject); err != nil {
			return false
		}
		if _, duplicate := seen[declaration.Subject]; duplicate {
			return false
		}
		seen[declaration.Subject] = struct{}{}
		if !declaration.estimateCommand(command.Tenant, command.Plan).accepted() {
			return false
		}
	}
	return true
}

// estimateCommand 把一件声明转述成 UC-PP-001 的试算声明。分区给法（estimateInput）、受理门与「同一声明」含哪几格
// （estimateIdentity）都只在那一侧写，这里借用、不复写；计价范围与价格方向取卡自己的。
func (declaration PlanGivenEstimateDeclaration) estimateCommand(tenant domain.TenantID, plan domain.PricingPlanVersion) FormEstimateEvaluationsCommand {
	return FormEstimateEvaluationsCommand{
		Tenant:     tenant,
		Scope:      plan.Scope(),
		Direction:  plan.Direction(),
		BasisAt:    declaration.BasisAt,
		Weight:     declaration.Weight,
		Dimensions: declaration.Dimensions,
		Zone:       declaration.Zone,
		Route:      declaration.Route,
		Settlement: declaration.Settlement,
	}
}

// planGivenEstimateIdentity 由调用方给的试算对象引用、该件声明与价卡版本确定性派生评价标识（ADR-0152 决定五）：同一声明、同一
// 张卡得同一标识，评价可比对可复算。声明与卡取 estimateIdentity 那一份，引用在外层拼进来，各段带长度前缀：引用由调用方给、
// 可含任何字符，不带前缀就能拼出与另一件相同的串。打头的 plan-given 让本入口与 UC-PP-001 的标识各在各的空间里。
func planGivenEstimateIdentity(reference string, declaration FormEstimateEvaluationsCommand, plan domain.PricingPlanVersion) string {
	digest := sha256.New()
	for _, part := range []string{"plan-given", reference, estimateIdentity(declaration, plan)} {
		fmt.Fprintf(digest, "%d:%s", len(part), part)
	}
	return "EST-" + hex.EncodeToString(digest.Sum(nil)[:8])
}
