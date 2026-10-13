package domain

import "fmt"

// PreAcceptanceEstimateForm 是受理前估价的形态（ADR-0179 决定二）。首版只内置一种：委托当前提交版本逐件 SELL 试算、按结算币种
// 合计。词表外的形态登记时拒——加一种形态是产品发版的事（同一扇门的后继形态），不是登一行的事。
type PreAcceptanceEstimateForm uint8

const (
	PreAcceptanceEstimateFormInvalid PreAcceptanceEstimateForm = iota
	PerParcelSellTrialSum
)

func (form PreAcceptanceEstimateForm) String() string {
	switch form {
	case PerParcelSellTrialSum:
		return "PER_PARCEL_SELL_TRIAL_SUM"
	default:
		return ""
	}
}

func (form PreAcceptanceEstimateForm) valid() bool {
	return form == PerParcelSellTrialSum
}

// PreAcceptanceEstimateFormFromName 把册上与登记输入里的形态词译回领域常量。词与 String 以及库上的 CHECK 同字。
func PreAcceptanceEstimateFormFromName(name string) (PreAcceptanceEstimateForm, error) {
	switch name {
	case "PER_PARCEL_SELL_TRIAL_SUM":
		return PerParcelSellTrialSum, nil
	default:
		return PreAcceptanceEstimateFormInvalid, fmt.Errorf("%w: pre-acceptance estimate form %q", ErrBlankValue, name)
	}
}

// EstimatePricingScope 回指 parcel-pricing 的一个计价范围：受理前试算按它选卡（ADR-0179 决定三、五）。由登记方指名——本上下文的
// 主要计费范围与计价范围是不是同一标识空间没有裁，这里不从任何一格推。
type EstimatePricingScope struct{ requiredValue }

func NewEstimatePricingScope(value string) (EstimatePricingScope, error) {
	required, err := newRequiredValue("estimate pricing scope", value)
	return EstimatePricingScope{required}, err
}

// PreAcceptanceEstimateAdoption 是一个结算账户采用的受理前估价：键是（租户、结算账户），值是（形态、计价范围）。不设修订，照
// ADR-0158 的不可变元组：同一户再登同一份是重放，任一格不同是冲突、不覆盖。租户在册的键上，不在这个值里。
type PreAcceptanceEstimateAdoption struct {
	account SettlementAccountID
	form    PreAcceptanceEstimateForm
	scope   EstimatePricingScope
}

// NewPreAcceptanceEstimateAdoption 拒空白与词表外的形态。
func NewPreAcceptanceEstimateAdoption(
	account SettlementAccountID,
	form PreAcceptanceEstimateForm,
	scope EstimatePricingScope,
) (PreAcceptanceEstimateAdoption, error) {
	if !account.valid() || !scope.valid() {
		return PreAcceptanceEstimateAdoption{}, fmt.Errorf("%w: pre-acceptance estimate adoption", ErrBlankValue)
	}
	if !form.valid() {
		return PreAcceptanceEstimateAdoption{}, fmt.Errorf("%w: pre-acceptance estimate form %d", ErrBlankValue, uint8(form))
	}
	return PreAcceptanceEstimateAdoption{account: account, form: form, scope: scope}, nil
}

func (adoption PreAcceptanceEstimateAdoption) Account() SettlementAccountID    { return adoption.account }
func (adoption PreAcceptanceEstimateAdoption) Form() PreAcceptanceEstimateForm { return adoption.form }
func (adoption PreAcceptanceEstimateAdoption) PricingScope() EstimatePricingScope {
	return adoption.scope
}

// SameRegistration 比的是登记内容，不含落库时刻。
func (adoption PreAcceptanceEstimateAdoption) SameRegistration(other PreAcceptanceEstimateAdoption) bool {
	return adoption.account.String() == other.account.String() &&
		adoption.form == other.form &&
		adoption.scope.String() == other.scope.String()
}
