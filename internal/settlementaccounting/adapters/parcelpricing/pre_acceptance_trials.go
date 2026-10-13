package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUntranslatablePreAcceptanceTrial 表示计价那一侧交出了本适配器词汇表之外的答复——编程错误，不是业务答案（ADR-0025）。
var ErrUntranslatablePreAcceptanceTrial = errors.New("settlement accounting parcelpricing adapter: untranslatable pre-acceptance trial")

// PreAcceptanceTrials 把结算的受理前试算口接到 parcel-pricing「按委托提交版本形成受理前试算」的入口上（ADR-0179 决定一）。只翻译：
// 选卡、逐件造输入、补齐读数都在那一侧；合计与停法在本上下文的领域里。
type PreAcceptanceTrials struct {
	estimates *ppapplication.FormPreAcceptanceEstimatesHandler
}

// NewPreAcceptanceTrials 构造期拒 nil：漏装要在启动时炸。
func NewPreAcceptanceTrials(estimates *ppapplication.FormPreAcceptanceEstimatesHandler) (*PreAcceptanceTrials, error) {
	if estimates == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: pre-acceptance estimates handler is nil")
	}
	return &PreAcceptanceTrials{estimates: estimates}, nil
}

var _ saports.PreAcceptanceTrialSource = (*PreAcceptanceTrials)(nil)

// FormPreAcceptanceTrials 译请求、问计价、按格译回。计价未决译成 ErrPreAcceptanceTrialsUnavailable；命令立不起来与计价答未受理都是
// 本适配器译坏了，按编程错误上抛。
func (adapter *PreAcceptanceTrials) FormPreAcceptanceTrials(
	ctx context.Context,
	query saports.PreAcceptanceTrialQuery,
) (saports.PreAcceptanceTrialAnswer, error) {
	none := saports.PreAcceptanceTrialAnswer{}
	tenant, err := ppdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatablePreAcceptanceTrial, err)
	}
	scope, err := ppdomain.NewPricingScopeID(query.PricingScope.String())
	if err != nil {
		return none, fmt.Errorf("%w: pricing scope: %v", ErrUntranslatablePreAcceptanceTrial, err)
	}
	currency, err := ppdomain.NewCurrency(query.Currency.String())
	if err != nil {
		return none, fmt.Errorf("%w: currency: %v", ErrUntranslatablePreAcceptanceTrial, err)
	}
	result, err := adapter.estimates.Handle(ctx, ppapplication.FormPreAcceptanceEstimatesCommand{
		Tenant:            tenant,
		Scope:             scope,
		ShipmentRequest:   query.ShipmentRequest,
		SubmissionVersion: query.SubmissionVersion,
		At:                query.At,
		Settlement:        currency,
	})
	if err != nil {
		return none, fmt.Errorf("form pre-acceptance trials: %w", err)
	}
	priceCard := versionReferenceLiteral(result.Plan)
	switch result.Outcome {
	case ppapplication.PreAcceptanceEstimatesFormed:
	case ppapplication.PreAcceptanceDeclarationIncomplete:
		return saports.PreAcceptanceTrialAnswer{Outcome: saports.PreAcceptanceTrialsIncomplete, PriceCard: priceCard}, nil
	case ppapplication.PreAcceptancePriceCardNotConfigured:
		return saports.PreAcceptanceTrialAnswer{Outcome: saports.PreAcceptanceTrialPriceCardNotConfigured}, nil
	case ppapplication.PreAcceptancePriceCardApplicabilityConflict:
		return saports.PreAcceptanceTrialAnswer{Outcome: saports.PreAcceptanceTrialPriceCardConflict}, nil
	case ppapplication.PreAcceptanceSubmissionNotVisible:
		return saports.PreAcceptanceTrialAnswer{Outcome: saports.PreAcceptanceTrialSubmissionNotVisible, PriceCard: priceCard}, nil
	case ppapplication.PreAcceptanceEstimatesUndecided:
		return none, fmt.Errorf("form pre-acceptance trials: %w: %s", saports.ErrPreAcceptanceTrialsUnavailable, result.Reason)
	case ppapplication.PreAcceptanceEstimatesNotAccepted:
		return none, fmt.Errorf("%w: pricing refused the trial command", ErrUntranslatablePreAcceptanceTrial)
	default:
		return none, fmt.Errorf("%w: outcome %d", ErrUntranslatablePreAcceptanceTrial, result.Outcome)
	}

	trials := make([]sadomain.PreAcceptanceTrial, 0, len(result.Estimates))
	for _, estimate := range result.Estimates {
		trial, err := trialOf(estimate)
		if err != nil {
			return none, err
		}
		trials = append(trials, trial)
	}
	return saports.PreAcceptanceTrialAnswer{Outcome: saports.PreAcceptanceTrialsFormed, PriceCard: priceCard, Trials: trials}, nil
}

// trialOf 把一份试算评价译成本上下文的试算：评价标识、冻结的版本清单原样带上；合计按币种的 ISO 4217 最小币单位位数
// （Currency.MinorUnitScale，产品内置）经同包 minorUnitsOf 换写成整数。合计的小数位多于币种位数时不截断——截断是在改一个
// 要占用的金额——这一份当没有合计交回，估价随之答未形成。
func trialOf(estimate ppapplication.PreAcceptanceParcelEstimate) (sadomain.PreAcceptanceTrial, error) {
	evaluation := estimate.Evaluation
	if estimate.Answer != ppapplication.EstimateCandidateEvaluated || evaluation.ID().String() == "" {
		return sadomain.PreAcceptanceTrial{}, fmt.Errorf("%w: a formed trial carries no evaluation for %s",
			ErrUntranslatablePreAcceptanceTrial, estimate.Parcel)
	}
	references := evaluation.Manifest().References()
	versions := make([]string, 0, len(references))
	for _, reference := range references {
		versions = append(versions, versionReferenceLiteral(reference))
	}
	money, present := evaluation.Total()
	if !present {
		return sadomain.NewUntotaledPreAcceptanceTrial(estimate.Parcel, evaluation.ID().String(), versions)
	}
	minor, err := minorUnitsOf(money.Amount().String(), int(money.Currency().MinorUnitScale()))
	if err != nil {
		return sadomain.NewUntotaledPreAcceptanceTrial(estimate.Parcel, evaluation.ID().String(), versions)
	}
	currency, err := sadomain.NewCurrencyCode(money.Currency().String())
	if err != nil {
		return sadomain.PreAcceptanceTrial{}, fmt.Errorf("%w: currency: %v", ErrUntranslatablePreAcceptanceTrial, err)
	}
	return sadomain.NewPreAcceptanceTrial(estimate.Parcel, evaluation.ID().String(), versions, minor, currency)
}

// versionReferenceLiteral 是一条版本引用在留痕里的字面：种类、标识、版本，有指纹时带上。复算要的正是这几段。
func versionReferenceLiteral(reference ppdomain.VersionReference) string {
	if reference.ID() == "" {
		return ""
	}
	literal := fmt.Sprintf("%s:%s@%s", reference.Kind(), reference.ID(), reference.Version())
	if reference.HasFingerprint() {
		literal += "#" + reference.Fingerprint()
	}
	return literal
}
