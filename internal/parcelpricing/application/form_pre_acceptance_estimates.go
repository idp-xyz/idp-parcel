// form_pre_acceptance_estimates.go 是「按委托提交版本形成受理前试算」的入口（ADR-0179 决定一、三、四；票 product-strategy-boundary/25）：
// 结算按采用行要控制金额时来问。本入口按提交版本读申报、按申报的服务产品解析在用的 SELL·客户费用价卡、逐件造声明，交给定价卡的
// 入口（FormPlanGivenEstimateEvaluationsHandler）形成试算评价，逐件交回。合计不在这里：怎么合、哪几种结果停，是结算的估价形态。
//
// 选卡与正式 SELL·客户费用评价同口径：按（租户、计价范围、SELL、客户费用、时点）与这一版申报的服务产品经 PriceCardInForceResolver
// 解析唯一在用版本（ADR-0382 决定二、三），时点取调用方交来的控制判断时点。未配置与适用冲突各自停，不挑卡；缺件自成一格，逐件点名，
// 不拿零或别件顶。
// 不入册、不交付（ADR-0152 决定二）：依赖里没有评价库写口，也没有交付口。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// FormPreAcceptanceEstimatesOutcome 是本入口的封闭结果，按恢复动作分格（ADR-0029）。
type FormPreAcceptanceEstimatesOutcome uint8

const (
	FormPreAcceptanceEstimatesOutcomeInvalid FormPreAcceptanceEstimatesOutcome = iota
	// PreAcceptanceEstimatesFormed：每一件都形成了一份试算评价，与声明成员同序；评价自己的状态原样透出。
	PreAcceptanceEstimatesFormed
	// PreAcceptanceEstimatesNotAccepted：命令不成形，什么都没问。
	PreAcceptanceEstimatesNotAccepted
	// PreAcceptancePriceCardNotConfigured：这一时点没有在用的 SELL·客户费用价卡，要去登一张。
	PreAcceptancePriceCardNotConfigured
	// PreAcceptancePriceCardApplicabilityConflict：多于一版同时在用，候选交人裁。
	PreAcceptancePriceCardApplicabilityConflict
	// PreAcceptanceSubmissionNotVisible：这份委托的这一版对本租户不可见。
	PreAcceptanceSubmissionNotVisible
	// PreAcceptanceDeclarationIncomplete：至少一件缺测量，或缺这张卡要的那一格（邮编路线 / 分区）。逐件同序交回，缺的那几件点名。
	PreAcceptanceDeclarationIncomplete
	// PreAcceptanceEstimatesUndecided：依赖读不回，形成与否未知，停在哪一口见 Reason。
	PreAcceptanceEstimatesUndecided
)

func (outcome FormPreAcceptanceEstimatesOutcome) String() string {
	switch outcome {
	case PreAcceptanceEstimatesFormed:
		return "FORMED"
	case PreAcceptanceEstimatesNotAccepted:
		return "NOT_ACCEPTED"
	case PreAcceptancePriceCardNotConfigured:
		return "PRICE_CARD_NOT_CONFIGURED"
	case PreAcceptancePriceCardApplicabilityConflict:
		return "PRICE_CARD_APPLICABILITY_CONFLICT"
	case PreAcceptanceSubmissionNotVisible:
		return "SUBMISSION_NOT_VISIBLE"
	case PreAcceptanceDeclarationIncomplete:
		return "DECLARATION_INCOMPLETE"
	case PreAcceptanceEstimatesUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// FormPreAcceptanceEstimatesUndecidedReason 指名未决停在哪一口。
type FormPreAcceptanceEstimatesUndecidedReason uint8

const (
	FormPreAcceptanceEstimatesUndecidedReasonNone FormPreAcceptanceEstimatesUndecidedReason = iota
	PreAcceptancePriceCardResolutionUnavailable
	PreAcceptanceSubmissionDeclarationUnavailable
	PreAcceptanceReadingCompletionUnavailable
)

func (reason FormPreAcceptanceEstimatesUndecidedReason) String() string {
	switch reason {
	case PreAcceptancePriceCardResolutionUnavailable:
		return "PRICE_CARD_RESOLUTION_UNAVAILABLE"
	case PreAcceptanceSubmissionDeclarationUnavailable:
		return "SUBMISSION_DECLARATION_UNAVAILABLE"
	case PreAcceptanceReadingCompletionUnavailable:
		return "READING_COMPLETION_UNAVAILABLE"
	default:
		return ""
	}
}

// FormPreAcceptanceEstimatesCommand 是一次受理前试算。委托与提交版本是 parcel-shipment 的标识原样；At 是控制回显的判断时点
// （ADR-0179 决定三）；Settlement 是控制作用域的币种，由调用方交入，本入口不另找来源。计价范围由调用方按它的采用行交来。
type FormPreAcceptanceEstimatesCommand struct {
	Tenant            domain.TenantID
	Scope             domain.PricingScopeID
	ShipmentRequest   string
	SubmissionVersion string
	At                time.Time
	Settlement        domain.Currency
	// Scopes 是有序的销售价格来源（ADR-0380 决定三）。空或一项时不看它，仍按 Scope 给全部件用同一张卡。
	// 多于一项时每件各自按这个顺序问；Scope 必须是第一项。
	Scopes []domain.PricingScopeID
	// Zone 是调用方已经定下的分区。只在按有序列表问、且价卡不绑分区目录时用：逐件与逐委托整票读的是同一份。
	// 空则仍只按申报的邮编路线。只带一个范围的路不读它。
	Zone string
}

// PreAcceptanceParcelEstimate 是一件声明包裹的结果。Answer 与缺项码沿用 UC-PP-001 逐卡那一格的词（EstimateCandidateAnswer、
// EstimateMissing*）。Parcel 是申报里的声明包裹标识；评价对象另由委托、版本与件派生（preAcceptanceSubject）。
type PreAcceptanceParcelEstimate struct {
	Parcel     string
	Answer     EstimateCandidateAnswer
	Evaluation domain.PricingEvaluation
	Missing    []string
	// Scope 是这一件最终用的销售价格来源。只带一个范围时为零：那一格就是命令上的 Scope，卡在结果的 Plan 上。
	Scope domain.PricingScopeID
	// SourceTrail 是这一件问到过的每一个来源各自的那一格。只带一个范围时为空。
	SourceTrail []SellPriceSourceStep
}

// FormPreAcceptanceEstimatesResult 是入口的答复。Plan 在选到卡之后的各格都有值；Estimates 在已形成与缺件两格有值；Conflict 只在
// 适用冲突时有值；Reason 只在未决时非零。
type FormPreAcceptanceEstimatesResult struct {
	Outcome   FormPreAcceptanceEstimatesOutcome
	Reason    FormPreAcceptanceEstimatesUndecidedReason
	Plan      domain.VersionReference
	Estimates []PreAcceptanceParcelEstimate
	Conflict  []domain.VersionReference
}

// FormPreAcceptanceEstimatesDeps 是本入口的依赖，四件必备。证据层级由装配声明（ADR-0179 决定四）：受理前的输入是客户申报，层级
// 随部署形态定，不按定义记 S，也不由调用方逐次给。
type FormPreAcceptanceEstimatesDeps struct {
	PriceCards   ports.PriceCardInForceResolver
	Declarations ports.SubmissionDeclarationView
	Estimates    *FormPlanGivenEstimateEvaluationsHandler
	Evidence     domain.EvidenceKind
}

// FormPreAcceptanceEstimatesHandler 是本入口的编排。
type FormPreAcceptanceEstimatesHandler struct {
	deps FormPreAcceptanceEstimatesDeps
}

// NewFormPreAcceptanceEstimatesHandler 构造期拒缺件与未声明的证据层级：漏装一口要在启动时炸，而不是等第一份委托来问。
func NewFormPreAcceptanceEstimatesHandler(deps FormPreAcceptanceEstimatesDeps) (*FormPreAcceptanceEstimatesHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"price card in-force resolver", deps.PriceCards == nil},
		{"submission declaration view", deps.Declarations == nil},
		{"plan-given estimate evaluations", deps.Estimates == nil},
		{"evidence kind", !deps.Evidence.Declared()},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &FormPreAcceptanceEstimatesHandler{deps: deps}, nil
}

// Handle 把一次受理前试算推进到答案：受理 → 读申报 → 按申报的服务产品选卡 → 逐件交给定价卡的入口。先读申报后选卡：选哪张卡取决于
// 这一版申报的服务产品（ADR-0382 决定三），不读申报就选不出卡。不可见的委托因此在选卡之前就停，在用解析口一次都不问。
func (handler *FormPreAcceptanceEstimatesHandler) Handle(
	ctx context.Context,
	command FormPreAcceptanceEstimatesCommand,
) (FormPreAcceptanceEstimatesResult, error) {
	if !command.accepted() {
		return FormPreAcceptanceEstimatesResult{Outcome: PreAcceptanceEstimatesNotAccepted}, nil
	}
	declaration, found, err := handler.deps.Declarations.LoadSubmissionDeclaration(
		ctx, command.Tenant, command.ShipmentRequest, command.SubmissionVersion)
	if err != nil {
		return undecidedPreAcceptance(PreAcceptanceSubmissionDeclarationUnavailable, domain.VersionReference{}), nil
	}
	if !found || len(declaration.Parcels) == 0 {
		return FormPreAcceptanceEstimatesResult{Outcome: PreAcceptanceSubmissionNotVisible}, nil
	}
	if len(command.Scopes) > 1 {
		return handler.estimateAcrossSellPriceSources(ctx, command, declaration)
	}

	resolution, err := handler.deps.PriceCards.ResolveInForce(ctx, command.Tenant, command.Scope,
		domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, command.At, declaration.ServiceProduct)
	if err != nil {
		return undecidedPreAcceptance(PreAcceptancePriceCardResolutionUnavailable, domain.VersionReference{}), nil
	}
	switch resolution.Outcome {
	case ports.PriceCardVersionInForce:
	case ports.PriceCardNotConfigured:
		return FormPreAcceptanceEstimatesResult{Outcome: PreAcceptancePriceCardNotConfigured}, nil
	case ports.PriceCardApplicabilityConflict:
		return FormPreAcceptanceEstimatesResult{
			Outcome:  PreAcceptancePriceCardApplicabilityConflict,
			Conflict: append([]domain.VersionReference(nil), resolution.Candidates...),
		}, nil
	default:
		return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance estimates: unhandled price card resolution %d", resolution.Outcome)
	}
	plan := resolution.Plan
	if plan.Aggregation() == domain.AggregationPerShipment {
		return handler.estimateWholeShipment(ctx, command, plan, declaration)
	}

	estimates, undecided, err := handler.estimate(ctx, command, plan, declaration)
	if err != nil {
		return FormPreAcceptanceEstimatesResult{}, err
	}
	if undecided {
		return undecidedPreAcceptance(PreAcceptanceReadingCompletionUnavailable, plan.Reference()), nil
	}
	result := FormPreAcceptanceEstimatesResult{Outcome: PreAcceptanceEstimatesFormed, Plan: plan.Reference(), Estimates: estimates}
	for _, estimate := range estimates {
		if estimate.Answer != EstimateCandidateEvaluated {
			result.Outcome = PreAcceptanceDeclarationIncomplete
		}
	}
	return result, nil
}

// estimate 逐件造声明交给定价卡的入口，按申报顺序合回。缺测量的件不交过去、就地点名：那道受理门要每件都有重量，带一件缺的进去
// 整份只会答「命令不成形」，说不出缺的是哪件。undecided 为真即补齐读数读不回。
func (handler *FormPreAcceptanceEstimatesHandler) estimate(
	ctx context.Context,
	command FormPreAcceptanceEstimatesCommand,
	plan domain.PricingPlanVersion,
	declaration ports.SubmissionDeclaration,
) ([]PreAcceptanceParcelEstimate, bool, error) {
	estimates := make([]PreAcceptanceParcelEstimate, len(declaration.Parcels))
	settlement := command.Settlement
	measured := make([]PlanGivenEstimateDeclaration, 0, len(declaration.Parcels))
	positions := make([]int, 0, len(declaration.Parcels))
	for index, parcel := range declaration.Parcels {
		estimates[index] = PreAcceptanceParcelEstimate{Parcel: parcel.Parcel}
		if parcel.Weight == nil {
			estimates[index].Answer = EstimateCandidateInputIncomplete
			estimates[index].Missing = []string{EstimateMissingMeasurement}
			continue
		}
		measured = append(measured, PlanGivenEstimateDeclaration{
			Subject:        preAcceptanceSubject(command, parcel.Parcel),
			BasisAt:        command.At,
			Weight:         *parcel.Weight,
			Dimensions:     parcel.Dimensions,
			Route:          declaration.Route,
			Settlement:     &settlement,
			ServiceProduct: declaration.ServiceProduct,
		})
		positions = append(positions, index)
	}
	if len(measured) == 0 {
		return estimates, false, nil
	}

	formed, err := handler.deps.Estimates.Handle(ctx, FormPlanGivenEstimateEvaluationsCommand{
		Tenant:       command.Tenant,
		Plan:         plan,
		Evidence:     handler.deps.Evidence,
		Declarations: measured,
	})
	if err != nil {
		return nil, false, fmt.Errorf("pre-acceptance estimates: %w", err)
	}
	switch formed.Outcome {
	case PlanGivenEstimatesFormed:
	case PlanGivenEstimatesUndecided:
		return nil, true, nil
	case PlanGivenEstimatesNotAccepted:
		// 声明是本入口按那道受理门的要求造的，被拒说明造法与门不一致——编程错误，不是一种业务答案。
		return nil, false, fmt.Errorf("pre-acceptance estimates: plan-given estimates refused the declarations")
	default:
		return nil, false, fmt.Errorf("pre-acceptance estimates: unhandled plan-given outcome %d", formed.Outcome)
	}
	if len(formed.Estimates) != len(positions) {
		return nil, false, fmt.Errorf("pre-acceptance estimates: %d declarations answered by %d estimates", len(positions), len(formed.Estimates))
	}
	for offset, estimate := range formed.Estimates {
		target := &estimates[positions[offset]]
		target.Answer = estimate.Answer
		target.Evaluation = estimate.Evaluation
		target.Missing = append([]string(nil), estimate.Missing...)
	}
	return estimates, false, nil
}

// preAcceptanceSubject 由委托、提交版本与件派生试算对象引用：同一版同一件得同一引用，评价标识随之确定，留痕可复算（ADR-0179
// 决定六）。各段带长度前缀，理由同 planGivenEstimateIdentity：引用来自另一个上下文，可含任何字符。
func preAcceptanceSubject(command FormPreAcceptanceEstimatesCommand, parcel string) string {
	var subject strings.Builder
	subject.WriteString("pre-acceptance")
	for _, part := range []string{command.ShipmentRequest, command.SubmissionVersion, parcel} {
		fmt.Fprintf(&subject, "|%d:%s", len(part), part)
	}
	return subject.String()
}

// accepted 是受理门：租户、计价范围、两个引用、时点与结算币种都在，引用不带两侧空白。币种不给默认——估价按哪个币种出，是控制
// 作用域说的话。
func (command FormPreAcceptanceEstimatesCommand) accepted() bool {
	if command.Tenant.String() == "" || command.Scope.String() == "" ||
		!trimmedReference(command.ShipmentRequest) || !trimmedReference(command.SubmissionVersion) ||
		command.At.IsZero() || command.Settlement.String() == "" {
		return false
	}
	if len(command.Scopes) == 0 {
		return true
	}
	if command.Scopes[0] != command.Scope {
		return false
	}
	seen := make(map[domain.PricingScopeID]struct{}, len(command.Scopes))
	for _, scope := range command.Scopes {
		if scope.String() == "" {
			return false
		}
		if _, repeated := seen[scope]; repeated {
			return false
		}
		seen[scope] = struct{}{}
	}
	return true
}

func trimmedReference(reference string) bool {
	return reference != "" && strings.TrimSpace(reference) == reference
}

func undecidedPreAcceptance(reason FormPreAcceptanceEstimatesUndecidedReason, plan domain.VersionReference) FormPreAcceptanceEstimatesResult {
	return FormPreAcceptanceEstimatesResult{Outcome: PreAcceptanceEstimatesUndecided, Reason: reason, Plan: plan}
}

// estimateWholeShipment 在用卡是逐委托时按整票试算一次。读不到件清单不编一份；缺测量或没有邮编路线就停在缺件，不逐件相加。
func (handler *FormPreAcceptanceEstimatesHandler) estimateWholeShipment(
	ctx context.Context,
	command FormPreAcceptanceEstimatesCommand,
	plan domain.PricingPlanVersion,
	declaration ports.SubmissionDeclaration,
) (FormPreAcceptanceEstimatesResult, error) {
	if len(declaration.Parcels) == 0 {
		return FormPreAcceptanceEstimatesResult{Outcome: PreAcceptanceSubmissionNotVisible, Plan: plan.Reference()}, nil
	}
	incomplete := make([]PreAcceptanceParcelEstimate, 0)
	pieces := make([]domain.MemberPiece, 0, len(declaration.Parcels))
	ids := make([]domain.PackageID, 0, len(declaration.Parcels))
	for _, parcel := range declaration.Parcels {
		if parcel.Weight == nil {
			incomplete = append(incomplete, PreAcceptanceParcelEstimate{
				Parcel: parcel.Parcel, Answer: EstimateCandidateInputIncomplete, Missing: []string{EstimateMissingMeasurement},
			})
			continue
		}
		packageID, err := domain.NewPackageID(parcel.Parcel)
		if err != nil {
			return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
		}
		piece, err := domain.NewMemberPiece(packageID, *parcel.Weight, parcel.Dimensions)
		if err != nil {
			return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
		}
		piece, err = piece.WithMeasurementOrigin(domain.PieceMeasurementDeclared)
		if err != nil {
			return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
		}
		pieces = append(pieces, piece)
		ids = append(ids, packageID)
	}
	if len(incomplete) > 0 {
		return FormPreAcceptanceEstimatesResult{
			Outcome: PreAcceptanceDeclarationIncomplete, Plan: plan.Reference(), Estimates: incomplete,
		}, nil
	}
	if declaration.Route == nil || declaration.Route.Destination() == "" {
		return FormPreAcceptanceEstimatesResult{
			Outcome: PreAcceptanceDeclarationIncomplete, Plan: plan.Reference(),
			Estimates: []PreAcceptanceParcelEstimate{{
				Parcel: command.ShipmentRequest, Answer: EstimateCandidateInputIncomplete, Missing: []string{EstimateMissingPostalRoute},
			}},
		}, nil
	}
	total := pieces[0].ActualWeight().Value()
	unit := pieces[0].ActualWeight().Unit()
	for _, piece := range pieces[1:] {
		weight := piece.ActualWeight()
		if weight.Unit() != unit {
			return FormPreAcceptanceEstimatesResult{
				Outcome: PreAcceptanceDeclarationIncomplete, Plan: plan.Reference(),
				Estimates: []PreAcceptanceParcelEstimate{{
					Parcel: piece.ID().String(), Answer: EstimateCandidateInputIncomplete, Missing: []string{EstimateMissingMeasurement},
				}},
			}, nil
		}
		summed, err := total.Add(weight.Value())
		if err != nil {
			return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
		}
		total = summed
	}
	weight, err := domain.NewWeight(total, unit)
	if err != nil {
		return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
	}
	manifest, err := domain.NewMemberManifest(ids, weight, nil)
	if err != nil {
		return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
	}
	manifest, err = manifest.WithPieces(pieces)
	if err != nil {
		return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
	}
	subject, err := domain.NewShipmentSubject(command.ShipmentRequest)
	if err != nil {
		return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
	}
	input, err := domain.NewPostalAggregatePricingInputSnapshot(
		command.Tenant, plan.Scope(), subject, *declaration.Route, manifest, command.At)
	if err != nil {
		return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
	}
	// 有序列表上的逐委托试算与逐件那一路读同一份调用方分区。只带一个范围的路不读它。
	// 绑了分区目录的卡只认目录，把调用方分区写进快照会改摘要。
	if len(command.Scopes) > 1 && command.Zone != "" && !planBindsZoneCatalogue(plan) {
		input, err = input.WithCallerZone(command.Zone)
		if err != nil {
			return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
		}
	}
	input, err = input.WithSettlementCurrency(command.Settlement)
	if err != nil {
		return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
	}
	if declaration.ServiceProduct.Declared() {
		input, err = input.WithServiceProduct(declaration.ServiceProduct)
		if err != nil {
			return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
		}
	}
	digest := sha256.New()
	planID := plan.Reference().ID() + "@" + plan.Reference().Version()
	fmt.Fprintf(digest, "%d:%s%d:%s%d:%s", len(command.ShipmentRequest), command.ShipmentRequest,
		len(command.SubmissionVersion), command.SubmissionVersion, len(planID), planID)
	id, err := domain.NewEvaluationID("EST-SHIP-" + hex.EncodeToString(digest.Sum(nil)[:8]))
	if err != nil {
		return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance shipment estimate: %w", err)
	}
	evaluation, undecided, err := handler.deps.Estimates.EvaluateShipment(ctx, id, plan, input, handler.deps.Evidence)
	if err != nil {
		return FormPreAcceptanceEstimatesResult{}, err
	}
	if undecided {
		return undecidedPreAcceptance(PreAcceptanceReadingCompletionUnavailable, plan.Reference()), nil
	}
	return FormPreAcceptanceEstimatesResult{
		Outcome: PreAcceptanceEstimatesFormed, Plan: plan.Reference(),
		Estimates: []PreAcceptanceParcelEstimate{{
			Parcel: command.ShipmentRequest, Answer: EstimateCandidateEvaluated, Evaluation: evaluation,
		}},
	}, nil
}

func planBindsZoneCatalogue(plan domain.PricingPlanVersion) bool {
	for _, link := range plan.Structures().ReferenceCatalogues() {
		if link.Kind() == domain.CatalogueKindZone {
			return true
		}
	}
	return false
}
