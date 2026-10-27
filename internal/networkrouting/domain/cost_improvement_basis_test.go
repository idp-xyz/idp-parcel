package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func pricedFact(t *testing.T, candidate string, amount int64, currency string) domain.CandidateCostFact {
	t.Helper()
	fact, err := domain.NewPricedCandidateCost(mustValue(t, domain.NewCandidateID, candidate), amount, currency)
	if err != nil {
		t.Fatalf("priced: %v", err)
	}
	return fact
}

func legCitation(t *testing.T, ordinal int) domain.PlannedLegCostCitation {
	t.Helper()
	citation, err := domain.NewPlannedLegCostCitation(domain.PlannedLegCostCitationSpec{
		Ordinal: ordinal, EvaluationID: "EST-current", PlanReference: "PLAN-COST/v1",
		ComparisonAmount: "21", ComparisonCurrency: "CNY", FactsBasis: domain.ParcelFactsDeclaration,
	})
	if err != nil {
		t.Fatalf("leg citation: %v", err)
	}
	return citation
}

// Covers: AT-NR-085、ADR-0173 决定一——成本改善严格大于已登记阈值才够格；够格时交回的改善依据带被替代计划成本、新计划成本、
// 币种、阈值，与被替代那一侧这次取到的逐段出处。
func TestACostImprovementQualifiesOnlyWhenStrictlyOverTheThreshold(t *testing.T) {
	current := pricedFact(t, "candidate-current", 4200, "CNY")
	next := pricedFact(t, "candidate-next", 3100, "CNY")

	threshold := 1000
	citations := []domain.PlannedLegCostCitation{legCitation(t, 1), legCitation(t, 2)}
	basis, over := domain.JudgeCostImprovement(current, next, &threshold, citations)
	if !over {
		t.Fatal("改善 1100 分、阈值 1000 分，应够格")
	}
	if basis.SupersededCostMinor() != 4200 || basis.SuccessorCostMinor() != 3100 ||
		basis.Currency() != "CNY" || basis.ThresholdMinor() != 1000 || basis.ImprovementMinor() != 1100 {
		t.Fatalf("basis = %+v", basis)
	}
	if got := basis.SupersededCostCitations(); len(got) != 2 || got[0].EvaluationID() != "EST-current" {
		t.Fatalf("被替代那一侧的出处 = %+v, want 两段", got)
	}

	equal := 1100
	if _, over := domain.JudgeCostImprovement(current, next, &equal, nil); over {
		t.Fatal("改善恰等于阈值不得够格：严格大于才切换")
	}
	if _, over := domain.JudgeCostImprovement(current, next, nil, nil); over {
		t.Fatal("阈值没登记不得够格")
	}
	zero := 0
	if _, over := domain.JudgeCostImprovement(current, pricedFact(t, "candidate-next", 3100, "SGD"), &zero, nil); over {
		t.Fatal("币种不同比不出，不得够格")
	}
	if _, over := domain.JudgeCostImprovement(next, current, &zero, nil); over {
		t.Fatal("新候选更贵不是改善")
	}
}

// Covers: AT-NR-085 改善依据的构造门——读回在册依据也过这一道，改善不超过阈值，或金额、阈值、币种、出处不成形都拒。
func TestACostImprovementBasisRefusesWhatCouldNotHaveSwitched(t *testing.T) {
	valid := domain.CostImprovementBasisSpec{
		SupersededCostMinor: 4200, SuccessorCostMinor: 3100, Currency: "CNY", ThresholdMinor: 1000,
	}
	if _, err := domain.NewCostImprovementBasis(valid); err != nil {
		t.Fatalf("valid basis: %v", err)
	}

	notOver := valid
	notOver.ThresholdMinor = 1100
	if _, err := domain.NewCostImprovementBasis(notOver); !errors.Is(err, domain.ErrImprovementNotOverThreshold) {
		t.Fatalf("改善不超过阈值 err = %v, want ErrImprovementNotOverThreshold", err)
	}

	for name, mutate := range map[string]func(*domain.CostImprovementBasisSpec){
		"币种空白":    func(spec *domain.CostImprovementBasisSpec) { spec.Currency = " " },
		"被替代成本为负": func(spec *domain.CostImprovementBasisSpec) { spec.SupersededCostMinor = -1 },
		"新计划成本为负": func(spec *domain.CostImprovementBasisSpec) { spec.SuccessorCostMinor = -1 },
		"阈值为负":    func(spec *domain.CostImprovementBasisSpec) { spec.ThresholdMinor = -1 },
		"出处段号重复": func(spec *domain.CostImprovementBasisSpec) {
			spec.SupersededCostCitations = []domain.PlannedLegCostCitation{legCitation(t, 1), legCitation(t, 1)}
		},
		"出处不成形": func(spec *domain.CostImprovementBasisSpec) {
			spec.SupersededCostCitations = []domain.PlannedLegCostCitation{{}}
		},
	} {
		spec := valid
		mutate(&spec)
		if _, err := domain.NewCostImprovementBasis(spec); !errors.Is(err, domain.ErrInvalidReroute) {
			t.Errorf("%s err = %v, want ErrInvalidReroute", name, err)
		}
	}
}
