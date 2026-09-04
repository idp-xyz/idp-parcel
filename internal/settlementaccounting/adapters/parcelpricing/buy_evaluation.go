// Package parcelpricing 是 settlement-accounting 消费 parcel-pricing BUY 评价的适配器（ADR-0025 消费方侧；
// mechanism-executor-triage/06 的 SA-c 缝；票 pricing-amount-precision/02 消费侧那一项）。
//
// 它只翻译不判断：读提供方的评价库，把一份 BUY·SUPPLIER_COST 的 PricingEvaluation 译成 SA 的采用快照。
// 金额从十进制到最小币单位是**单位换写不是算术**（ADR-0107 Decision 五）：位数取评价取整留痕里合计那一步
// 的进位单位，换写只在恰好能整除时成立；没声明取整策略的卡（评价带 AMOUNT_PRECISION_UNDECLARED）一律拒，
// 本包不编任何币种小数位表、不补一次取整。
package parcelpricing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrNotABuyEvaluation 表示指名的评价不是 BUY·SUPPLIER_COST 方向：SA 只从采购方向的评价形成预期成本，
	// 拿一份客户计费评价来是提交矛盾。
	ErrNotABuyEvaluation = errors.New("settlement accounting parcelpricing adapter: evaluation is not BUY / SUPPLIER_COST")
	// ErrAmountPrecisionUndeclared 表示评价带 AMOUNT_PRECISION_UNDECLARED：卡没声明金额取整策略，合计是精确十进制，
	// 本包没有任何依据把它换写成最小币单位——补一次取整就是在评价之外另算一个数。
	ErrAmountPrecisionUndeclared = errors.New("settlement accounting parcelpricing adapter: the price card declares no amount rounding policy")
	// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致：某一侧交出了词汇表之外的内容。
	ErrUntranslatableAnswer = errors.New("settlement accounting parcelpricing adapter: untranslatable answer")
)

// amountPrecisionUndeclared 是提供方问题项的原词（ADR-0107 Decision 四）。
const amountPrecisionUndeclared = "AMOUNT_PRECISION_UNDECLARED"

// EvaluationSource 是提供方评价库在本适配器眼里的读口——按评价标识读回一份评价。parcel-pricing 的评价库
// 适配器直接满足它。
type EvaluationSource interface {
	FindByID(ctx context.Context, id ppdomain.EvaluationID) (ppdomain.PricingEvaluation, bool, error)
}

// BuyEvaluationAdapter 实现 saports.BuyEvaluationView。
type BuyEvaluationAdapter struct {
	evaluations EvaluationSource
}

func NewBuyEvaluationAdapter(evaluations EvaluationSource) (*BuyEvaluationAdapter, error) {
	if evaluations == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: evaluation source is nil")
	}
	return &BuyEvaluationAdapter{evaluations: evaluations}, nil
}

// LoadBuyEvaluation 按评价引用查一份采用快照。别的租户的评价对本租户不存在（ADR-0003 的隔离）；方向不对拒；
// 四种非完成结果逐格译、不带金额；已完成的评价金额按取整留痕换写。
func (adapter *BuyEvaluationAdapter) LoadBuyEvaluation(
	ctx context.Context,
	tenant sadomain.TenantID,
	reference sadomain.BuyEvaluationReference,
) (saports.BuyEvaluationAdoption, bool, error) {
	id, err := ppdomain.NewEvaluationID(reference.String())
	if err != nil {
		return saports.BuyEvaluationAdoption{}, false, nil
	}
	evaluation, found, err := adapter.evaluations.FindByID(ctx, id)
	if err != nil {
		return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("load buy evaluation: %w", err)
	}
	if !found || evaluation.Input().TenantID().String() != tenant.String() {
		return saports.BuyEvaluationAdoption{}, false, nil
	}
	if evaluation.Direction() != ppdomain.PricingDirectionBuy || evaluation.Purpose() != ppdomain.PricingPurposeSupplierCost {
		return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: %s / %s", ErrNotABuyEvaluation, evaluation.Direction(), evaluation.Purpose())
	}

	ruleVersion, err := sadomain.NewPurchaseRuleVersionReference(
		evaluation.PlanReference().ID() + "@" + evaluation.PlanReference().Version())
	if err != nil {
		return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: plan reference: %v", ErrUntranslatableAnswer, err)
	}
	adoption := saports.BuyEvaluationAdoption{
		Evaluation:  reference,
		Outcome:     outcomeOf(evaluation.Status()),
		RuleVersion: ruleVersion,
	}
	if subject, ok := evaluation.Input().Subject(); ok {
		adoption.SubjectKind = subject.Kind().String()
	}
	if members, ok := evaluation.Input().Members(); ok {
		for _, member := range members.Members() {
			adoption.MemberPackages = append(adoption.MemberPackages, member.String())
		}
	}
	if adoption.Outcome == saports.BuyEvaluationOutcomeInvalid {
		return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: evaluation status %q", ErrUntranslatableAnswer, evaluation.Status())
	}
	if adoption.Outcome != saports.BuyEvaluationCompleted {
		return adoption, true, nil
	}

	total, ok := evaluation.Total()
	if !ok {
		return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: completed evaluation without a total", ErrUntranslatableAnswer)
	}
	for _, issue := range evaluation.Issues() {
		if issue.Code() == amountPrecisionUndeclared {
			return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: evaluation %s", ErrAmountPrecisionUndeclared, reference)
		}
	}
	steps := evaluation.AmountRounding()
	totalDigits, declared := incrementDigits(steps, ppdomain.AmountRoundingTotal)
	if !declared {
		// 没有问题项却也没有合计那一步的留痕：提供方的形状变了，不是本包该猜的事。
		return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: completed evaluation carries no TOTAL rounding step", ErrUntranslatableAnswer)
	}
	settlementCurrency, err := sadomain.NewCurrencyCode(total.Currency().String())
	if err != nil {
		return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: settlement currency: %v", ErrUntranslatableAnswer, err)
	}
	settlementMinor, err := minorUnitsOf(total.Amount().String(), totalDigits)
	if err != nil {
		return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: total %s: %v", ErrUntranslatableAnswer, total.Amount(), err)
	}
	adoption.SettlementCurrency, adoption.SettlementMinor = settlementCurrency, settlementMinor
	adoption.OriginalCurrency, adoption.OriginalMinor = settlementCurrency, settlementMinor

	if conversion, converted := evaluation.ConversionStep(); converted {
		original := conversion.Original()
		originalCurrency, err := sadomain.NewCurrencyCode(original.Currency().String())
		if err != nil {
			return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: original currency: %v", ErrUntranslatableAnswer, err)
		}
		// 原币金额没有合计那一步的进位单位可依：逐行那一点声明了就取它，否则金额自己的位数就是它的
		// 精度——换写只在不丢位时成立，丢位即拒。
		originalDigits, declared := incrementDigits(steps, ppdomain.AmountRoundingPerLine)
		if !declared {
			originalDigits = fractionDigits(original.Amount().String())
		}
		originalMinor, err := minorUnitsOf(original.Amount().String(), originalDigits)
		if err != nil {
			return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: original amount %s: %v", ErrUntranslatableAnswer, original.Amount(), err)
		}
		stepReference, err := sadomain.NewConversionStepReference(conversion.SeriesReference().ID() + "@" + conversion.SeriesReference().Version())
		if err != nil {
			return saports.BuyEvaluationAdoption{}, false, fmt.Errorf("%w: conversion step: %v", ErrUntranslatableAnswer, err)
		}
		adoption.OriginalCurrency, adoption.OriginalMinor, adoption.Conversion = originalCurrency, originalMinor, stepReference
	}
	return adoption, true, nil
}

// outcomeOf 把提供方的五种结果逐格译成 SA 的五格（UC-SA-002 各有自己的结束格，不合并）。
func outcomeOf(status ppdomain.EvaluationStatus) saports.BuyEvaluationOutcome {
	switch status {
	case ppdomain.EvaluationCompleted:
		return saports.BuyEvaluationCompleted
	case ppdomain.EvaluationPending:
		return saports.BuyEvaluationPending
	case ppdomain.EvaluationUnratable:
		return saports.BuyEvaluationUnratable
	case ppdomain.EvaluationConflict:
		return saports.BuyEvaluationConflict
	case ppdomain.EvaluationFailed:
		return saports.BuyEvaluationNotFormed
	default:
		return saports.BuyEvaluationOutcomeInvalid
	}
}

// incrementDigits 从取整留痕里取某一点的进位单位的小数位数——那就是该点金额的最小币单位依据。
func incrementDigits(steps []ppdomain.AmountRoundingStep, point ppdomain.AmountRoundingPoint) (int, bool) {
	for _, step := range steps {
		if step.Point() == point {
			return fractionDigits(step.Increment().Amount().String()), true
		}
	}
	return 0, false
}

func fractionDigits(canonical string) int {
	_, fraction, _ := strings.Cut(canonical, ".")
	return len(fraction)
}

// minorUnitsOf 把规范十进制文本换写成 digits 位最小单位的整数。超位即拒——那是要取整，不是换写。
func minorUnitsOf(canonical string, digits int) (int64, error) {
	if canonical == "" || strings.HasPrefix(canonical, "-") {
		return 0, errors.New("amount is empty or negative")
	}
	whole, fraction, _ := strings.Cut(canonical, ".")
	if len(fraction) > digits {
		return 0, fmt.Errorf("amount %s exceeds %d minor digits", canonical, digits)
	}
	fraction += strings.Repeat("0", digits-len(fraction))
	combined := strings.TrimLeft(whole+fraction, "0")
	if combined == "" {
		return 0, nil
	}
	var minor int64
	for _, character := range combined {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("amount %s is not canonical", canonical)
		}
		if minor > (1<<62)/10 {
			return 0, fmt.Errorf("amount %s overflows", canonical)
		}
		minor = minor*10 + int64(character-'0')
	}
	return minor, nil
}

var _ saports.BuyEvaluationView = (*BuyEvaluationAdapter)(nil)
