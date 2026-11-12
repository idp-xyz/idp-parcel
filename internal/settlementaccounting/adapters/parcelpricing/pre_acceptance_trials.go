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
	return saports.PreAcceptanceTrialAnswer{
		Outcome: saports.PreAcceptanceTrialsFormed, PriceCard: priceCard, ServiceProduct: serviceProductOf(result.Estimates), Trials: trials,
	}, nil
}

// serviceProductOf 取选卡用的服务产品（ADR-0379 决定一）：每件评价的输入快照都记着它（ADR-0378 决定三），一份估价只选一张卡、按一个
// 产品选，各件本该相同。各件不同时不记——不挑一个顶，也不报错：产品只作留痕，补记不得挡住一份已经形成的估价。
func serviceProductOf(estimates []ppapplication.PreAcceptanceParcelEstimate) string {
	product := ""
	for index, estimate := range estimates {
		declared, present := estimate.Evaluation.Input().ServiceProduct()
		text := ""
		if present {
			text = declared.String()
		}
		if index > 0 && text != product {
			return ""
		}
		product = text
	}
	return product
}

// trialOf 把一份试算评价译成本上下文的试算：评价标识、冻结的版本清单原样带上；合计按币种的 ISO 4217 最小币单位位数
// （Currency.MinorUnitScale，产品内置）经同包 minorUnitsOf 换写成整数。合计的小数位多于币种位数时不截断——截断是在改一个
// 要占用的金额——这一份当没有合计交回，估价随之答未形成。
//
// 有合计的那一份另补记计价重量与费用行（ADR-0379 决定一，见 withDetail）。没有合计的那一份不补：估价随它停下、不留痕，补了也没处放。
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
	trial, err := sadomain.NewPreAcceptanceTrial(estimate.Parcel, evaluation.ID().String(), versions, minor, currency)
	if err != nil {
		return sadomain.PreAcceptanceTrial{}, err
	}
	return withDetail(trial, evaluation), nil
}

// withDetail 补记计价重量（进位之后的那一个，评价据它查表）与费用行。补记只作留痕，任何一格译不出都只是那一格不记、不报错：留痕
// 让一份本已形成的估价停下，就是拿一份运营看的明细去挡一次控制。费用行的金额照评价的十进制原文、连同卡本币带上，不换写成最小币
// 单位（理由在 sadomain.PreAcceptanceTrialChargeLine）。
func withDetail(trial sadomain.PreAcceptanceTrial, evaluation ppdomain.PricingEvaluation) sadomain.PreAcceptanceTrial {
	if result, present := evaluation.PricingWeight(); present {
		rounded := result.RoundedWeight()
		if weight, err := sadomain.NewPreAcceptanceTrialPricingWeight(rounded.Value().String(), rounded.Unit().String()); err == nil {
			trial = trial.WithPricingWeight(weight)
		}
	}
	evaluated := evaluation.ChargeLines()
	lines := make([]sadomain.PreAcceptanceTrialChargeLine, 0, len(evaluated))
	for _, line := range evaluated {
		translated, translatable := chargeLineOf(line)
		if !translatable {
			// 缺一行的列表会被读成整份明细，一行译不出就整份不记。
			return trial
		}
		lines = append(lines, translated)
	}
	if detailed, err := trial.WithChargeLines(lines); err == nil {
		trial = detailed
	}
	return trial
}

func chargeLineOf(line ppdomain.ChargeLine) (sadomain.PreAcceptanceTrialChargeLine, bool) {
	var effect sadomain.ChargeLineEffect
	switch line.Effect() {
	case ppdomain.ChargeEffectAdd:
		effect = sadomain.ChargeLineAdd
	case ppdomain.ChargeEffectDeduct:
		effect = sadomain.ChargeLineDeduct
	default:
		return sadomain.PreAcceptanceTrialChargeLine{}, false
	}
	code, err := sadomain.NewPricingChargeCode(line.Code().String())
	if err != nil {
		return sadomain.PreAcceptanceTrialChargeLine{}, false
	}
	currency, err := sadomain.NewCurrencyCode(line.Amount().Currency().String())
	if err != nil {
		return sadomain.PreAcceptanceTrialChargeLine{}, false
	}
	translated, err := sadomain.NewPreAcceptanceTrialChargeLine(code, effect, line.Amount().Amount().String(), currency)
	return translated, err == nil
}

// versionReferenceLiteral 是一条版本引用在留痕里的字面：种类、标识、版本，有指纹时带上。复算要的正是这几段；文法在
// sadomain.VersionReferenceLiteral，读口按同一处拆回。
func versionReferenceLiteral(reference ppdomain.VersionReference) string {
	return sadomain.VersionReferenceLiteral(sadomain.VersionReferenceParts{
		Kind: string(reference.Kind()), ID: reference.ID(), Version: reference.Version(), Fingerprint: reference.Fingerprint(),
	})
}
