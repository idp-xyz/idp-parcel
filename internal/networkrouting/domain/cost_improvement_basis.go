package domain

import "errors"

// ErrImprovementNotOverThreshold 说改善没有严格大于阈值：这样的依据撑不起一次自动切换。
var ErrImprovementNotOverThreshold = errors.New("network routing: cost improvement does not exceed the threshold")

// CostImprovementBasis 是仍适用线上改善改路的依据：被替代计划所选候选的成本、新计划所选候选的成本、两者共同的币种与策略
// 版本登记的阈值。失效线改路的依据是失效依据，不带它；一份`已改路`两者恰有其一，读的人才分得出是原计划走不通而改，还是
// 有更便宜的路而改，审计也答得出「改善了多少、比的是哪道线」。
//
// 被替代那一侧另带这次判断取到的逐段成本出处：新计划自带它的出处，被替代计划里留的是它当初入选时的出处，不是这次比的价。
// 两侧都有这次的出处，「为什么便宜了这么多」才回得到原币与汇率上去复算。取数侧没出处时这一格为空，同计划的缺席约定。
//
// 「严格大于阈值才切换」（ADR-0173 决定一）只在构造门定义：编排判改善够不够格、适配器读回在册依据，都过这一道。
type CostImprovementBasis struct {
	supersededCostMinor int64
	successorCostMinor  int64
	currency            requiredValue
	thresholdMinor      int64
	supersededCitations []PlannedLegCostCitation
}

type CostImprovementBasisSpec struct {
	SupersededCostMinor     int64
	SuccessorCostMinor      int64
	Currency                string
	ThresholdMinor          int64
	SupersededCostCitations []PlannedLegCostCitation
}

func NewCostImprovementBasis(spec CostImprovementBasisSpec) (CostImprovementBasis, error) {
	currency, err := newRequiredValue("cost improvement currency", spec.Currency)
	if err != nil || spec.SupersededCostMinor < 0 || spec.SuccessorCostMinor < 0 || spec.ThresholdMinor < 0 {
		return CostImprovementBasis{}, ErrInvalidReroute
	}
	// 序位不落在段链上这一条这里查不了：依据不带被替代计划的段链，那份计划归 reviewed_plan 引用。
	seenOrdinals := make(map[int]struct{}, len(spec.SupersededCostCitations))
	for _, citation := range spec.SupersededCostCitations {
		if !citation.valid() {
			return CostImprovementBasis{}, ErrInvalidReroute
		}
		if _, duplicated := seenOrdinals[citation.ordinal]; duplicated {
			return CostImprovementBasis{}, ErrInvalidReroute
		}
		seenOrdinals[citation.ordinal] = struct{}{}
	}
	if spec.SupersededCostMinor-spec.SuccessorCostMinor <= spec.ThresholdMinor {
		return CostImprovementBasis{}, ErrImprovementNotOverThreshold
	}
	return CostImprovementBasis{
		supersededCostMinor: spec.SupersededCostMinor,
		successorCostMinor:  spec.SuccessorCostMinor,
		currency:            currency,
		thresholdMinor:      spec.ThresholdMinor,
		supersededCitations: append([]PlannedLegCostCitation(nil), spec.SupersededCostCitations...),
	}, nil
}

// JudgeCostImprovement 判新候选相对当前候选的成本改善够不够格自动切换，够格时交回改善依据，currentCitations 是当前候选
// 这次取到的逐段出处。阈值没登记、成本比不出（缺价、币种不同）都不够格：比不出不是改善。构造门拒的一律按不够格处置——
// 没有依据就不切换，方向是保守的。
func JudgeCostImprovement(
	current, next CandidateCostFact,
	thresholdMinor *int,
	currentCitations []PlannedLegCostCitation,
) (CostImprovementBasis, bool) {
	if thresholdMinor == nil {
		return CostImprovementBasis{}, false
	}
	if _, comparable := CostImprovementMinor(current, next); !comparable {
		return CostImprovementBasis{}, false
	}
	basis, err := NewCostImprovementBasis(CostImprovementBasisSpec{
		SupersededCostMinor:     current.amountMinor,
		SuccessorCostMinor:      next.amountMinor,
		Currency:                current.currency.String(),
		ThresholdMinor:          int64(*thresholdMinor),
		SupersededCostCitations: currentCitations,
	})
	return basis, err == nil
}

func (basis CostImprovementBasis) SupersededCostMinor() int64 { return basis.supersededCostMinor }

func (basis CostImprovementBasis) SuccessorCostMinor() int64 { return basis.successorCostMinor }

func (basis CostImprovementBasis) Currency() string { return basis.currency.String() }

func (basis CostImprovementBasis) ThresholdMinor() int64 { return basis.thresholdMinor }

// SupersededCostCitations 交回被替代计划所选候选这次取到的逐段出处的拷贝。
func (basis CostImprovementBasis) SupersededCostCitations() []PlannedLegCostCitation {
	return append([]PlannedLegCostCitation(nil), basis.supersededCitations...)
}

// ImprovementMinor 是被替代计划成本减去新计划成本，恒严格大于阈值。
func (basis CostImprovementBasis) ImprovementMinor() int64 {
	return basis.supersededCostMinor - basis.successorCostMinor
}
