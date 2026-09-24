package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func pricedCost(t *testing.T, candidate string, amountMinor int64, currency string) domain.CandidateCostFact {
	t.Helper()
	fact, err := domain.NewPricedCandidateCost(mustValue(t, domain.NewCandidateID, candidate), amountMinor, currency)
	if err != nil {
		t.Fatalf("new priced candidate cost: %v", err)
	}
	return fact
}

// Covers: `PAR-NET-16`「首发只按成本单维择优」与 `AT-NR-001` 层次 4——只在通过硬约束与时间
// 可行性的合格候选里比，成本唯一最低者选中；被淘汰候选即使金额更低也不参选。
func TestCostSingleDimensionSelectsTheUniqueLowestAmongQualified(t *testing.T) {
	candidates := []domain.RouteCandidate{
		qualifiedRouteCandidate(t, "cand-dear"),
		qualifiedRouteCandidate(t, "cand-cheap"),
		eliminatedCandidate(t, "cand-banned", "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-1"),
	}
	costs := []domain.CandidateCostFact{
		pricedCost(t, "cand-dear", 5500, "CNY"),
		pricedCost(t, "cand-cheap", 3700, "CNY"),
		pricedCost(t, "cand-banned", 100, "CNY"),
	}

	ranking, err := domain.RankRouteCandidates(domain.CostSingleDimensionRanking, candidates, costs)
	if err != nil {
		t.Fatalf("rank: %v", err)
	}
	selected, ok := ranking.Selected()
	if ranking.Outcome() != domain.RankingSelected || !ok || selected.String() != "cand-cheap" {
		t.Fatalf("outcome = %s, selected = %s (%v); want SELECTED cand-cheap", ranking.Outcome(), selected, ok)
	}
}

// Covers: `PAR-NET-16`「候选评价为待判断或不可计价时该候选出局，不得以零金额或其他候选金额
// 顶替」——缺成本的两格都出局，剩下的唯一最低者照选；若按零顶替，出局的那两家会被选中。
// 合格候选全部缺成本时交「无已计价候选」，不硬选一个。
func TestCandidatesWithoutCostFactsAreExcludedNotZeroed(t *testing.T) {
	candidates := []domain.RouteCandidate{
		qualifiedRouteCandidate(t, "cand-pending"),
		qualifiedRouteCandidate(t, "cand-unpriceable"),
		qualifiedRouteCandidate(t, "cand-priced"),
	}
	pending, err := domain.NewPendingCandidateCost(mustValue(t, domain.NewCandidateID, "cand-pending"))
	if err != nil {
		t.Fatalf("new pending candidate cost: %v", err)
	}
	unpriceable, err := domain.NewUnpriceableCandidateCost(mustValue(t, domain.NewCandidateID, "cand-unpriceable"))
	if err != nil {
		t.Fatalf("new unpriceable candidate cost: %v", err)
	}

	ranking, err := domain.RankRouteCandidates(domain.CostSingleDimensionRanking, candidates,
		[]domain.CandidateCostFact{pending, unpriceable, pricedCost(t, "cand-priced", 5000, "CNY")})
	if err != nil {
		t.Fatalf("rank: %v", err)
	}
	selected, ok := ranking.Selected()
	if ranking.Outcome() != domain.RankingSelected || !ok || selected.String() != "cand-priced" {
		t.Fatalf("outcome = %s, selected = %s (%v); want SELECTED cand-priced", ranking.Outcome(), selected, ok)
	}
	excluded := ranking.Excluded()
	if len(excluded) != 2 || excluded[0].String() != "cand-pending" || excluded[1].String() != "cand-unpriceable" {
		t.Fatalf("excluded = %v, want [cand-pending cand-unpriceable]", excluded)
	}

	allWithout, err := domain.RankRouteCandidates(domain.CostSingleDimensionRanking, candidates[:2],
		[]domain.CandidateCostFact{pending, unpriceable})
	if err != nil {
		t.Fatalf("rank without priced: %v", err)
	}
	if _, ok := allWithout.Selected(); allWithout.Outcome() != domain.RankingNoPricedCandidate || ok {
		t.Fatalf("outcome = %s (selected %v), want NO_PRICED_CANDIDATE and nothing selected", allWithout.Outcome(), ok)
	}
}

// Covers: 币种不齐停下不比——两种币种的最小币单位金额之间没有可比关系，领域不折算、不挑一种
// 币种先比；数字更小的那一家也不选。
func TestCostsInDifferentCurrenciesStopTheComparison(t *testing.T) {
	candidates := []domain.RouteCandidate{
		qualifiedRouteCandidate(t, "cand-cny"),
		qualifiedRouteCandidate(t, "cand-sgd"),
	}
	ranking, err := domain.RankRouteCandidates(domain.CostSingleDimensionRanking, candidates,
		[]domain.CandidateCostFact{
			pricedCost(t, "cand-cny", 3000, "CNY"),
			pricedCost(t, "cand-sgd", 400, "SGD"),
		})
	if err != nil {
		t.Fatalf("rank: %v", err)
	}
	if _, ok := ranking.Selected(); ranking.Outcome() != domain.RankingCurrenciesDiffer || ok {
		t.Fatalf("outcome = %s (selected %v), want CURRENCIES_DIFFER and nothing selected", ranking.Outcome(), ok)
	}
}

// Covers: `PAR-NET-16`「并列且无法选出唯一一条时为冲突，交人工裁决，不得任选」——最低成本
// 并列交回并列的那几家，不按候选标识或任何无业务依据的次序收尾；更贵的那家不在并列名单里。
func TestLowestCostTieIsHandedOverNotBrokenByIdentifier(t *testing.T) {
	candidates := []domain.RouteCandidate{
		qualifiedRouteCandidate(t, "cand-b"),
		qualifiedRouteCandidate(t, "cand-a"),
		qualifiedRouteCandidate(t, "cand-dear"),
	}
	ranking, err := domain.RankRouteCandidates(domain.CostSingleDimensionRanking, candidates,
		[]domain.CandidateCostFact{
			pricedCost(t, "cand-b", 4200, "CNY"),
			pricedCost(t, "cand-a", 4200, "CNY"),
			pricedCost(t, "cand-dear", 9900, "CNY"),
		})
	if err != nil {
		t.Fatalf("rank: %v", err)
	}
	if _, ok := ranking.Selected(); ranking.Outcome() != domain.RankingTied || ok {
		t.Fatalf("outcome = %s (selected %v), want TIED and nothing selected", ranking.Outcome(), ok)
	}
	tied := map[string]bool{}
	for _, candidate := range ranking.Tied() {
		tied[candidate.String()] = true
	}
	if len(ranking.Tied()) != 2 || !tied["cand-a"] || !tied["cand-b"] {
		t.Fatalf("tied = %v, want exactly cand-a and cand-b", ranking.Tied())
	}
}

// Covers: 形态集合落在领域（ADR-0146 决定二）——首版族里只有成本单维一种，登记方给的词译得回
// 才算声明了形态；族外的词拒绝，不吸收成某一格，也不回落成「未声明」。
func TestRankingFormsAreAClosedFamily(t *testing.T) {
	form, err := domain.RankingFormFrom("COST_SINGLE_DIMENSION")
	if err != nil || form != domain.CostSingleDimensionRanking || form.String() != "COST_SINGLE_DIMENSION" {
		t.Fatalf("form = %v (%q), err = %v; want COST_SINGLE_DIMENSION", form, form.String(), err)
	}
	for _, raw := range []string{"TIMELINESS_FIRST", "cost_single_dimension", ""} {
		if _, err := domain.RankingFormFrom(raw); !errors.Is(err, domain.ErrUnknownRankingForm) {
			t.Fatalf("RankingFormFrom(%q) err = %v, want ErrUnknownRankingForm", raw, err)
		}
	}
}

// Covers: 排序不替策略作答——没有合格候选是`无路由`或未决的信号，与形态有没有声明无关；有合格
// 候选而版本没声明形态时交「形态未声明」，不替租户选一种；合格候选整条缺成本事实、或同一候选
// 两条事实，是证据装配坏了，响亮报错而不是当成待判断。
func TestRankingRefusesToInventAnAnswer(t *testing.T) {
	onlyEliminated := []domain.RouteCandidate{eliminatedCandidate(t, "cand-1", "PATH_NOT_EXECUTABLE/SCHED-V3")}
	for _, form := range []domain.RankingForm{domain.CostSingleDimensionRanking, domain.RankingFormUndeclared} {
		ranking, err := domain.RankRouteCandidates(form, onlyEliminated, nil)
		if err != nil || ranking.Outcome() != domain.RankingNoQualifiedCandidate {
			t.Fatalf("form %q: outcome = %s, err = %v; want NO_QUALIFIED_CANDIDATE", form, ranking.Outcome(), err)
		}
	}

	qualified := []domain.RouteCandidate{qualifiedRouteCandidate(t, "cand-1")}
	costs := []domain.CandidateCostFact{pricedCost(t, "cand-1", 100, "CNY")}
	undeclared, err := domain.RankRouteCandidates(domain.RankingFormUndeclared, qualified, costs)
	if err != nil || undeclared.Outcome() != domain.RankingFormNotDeclared {
		t.Fatalf("outcome = %s, err = %v; want RANKING_FORM_NOT_DECLARED", undeclared.Outcome(), err)
	}

	if _, err := domain.RankRouteCandidates(domain.CostSingleDimensionRanking, qualified, nil); !errors.Is(err, domain.ErrInvalidRanking) {
		t.Fatalf("err = %v; 合格候选缺整条成本事实是装配错误", err)
	}
	duplicated := append(costs, pricedCost(t, "cand-1", 90, "CNY"))
	if _, err := domain.RankRouteCandidates(domain.CostSingleDimensionRanking, qualified, duplicated); !errors.Is(err, domain.ErrInvalidRanking) {
		t.Fatalf("err = %v; 同一候选两条成本事实取哪条都是掷硬币", err)
	}
	if _, err := domain.RankRouteCandidates(domain.CostSingleDimensionRanking, qualified,
		[]domain.CandidateCostFact{{}}); !errors.Is(err, domain.ErrInvalidRanking) {
		t.Fatalf("err = %v; 零值成本事实没有候选也没有状态", err)
	}
}
