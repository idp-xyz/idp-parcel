package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证「渠道择优决定」记录（票 `label-channel/14` 裁决）：一次择优在这些候选里、按这条
// 规则、选了谁、谁出局及为何，逐候选各落自己那一格；记录只校形状不重算判断，重建门同理。
//
// 四格的判据取自裁决原文：选中 / 出局（因由取 `ChannelCostUnavailability` 四格）/ 落选（可计价
// 但不最优）/ 并列冲突（`PAR-NET-16`：最低价并列选不出唯一一条，冲突也是一次决定的结果）。

var selectionDecidedAt = time.Date(2026, 9, 4, 18, 30, 0, 0, time.UTC)

func selectionSubject(t *testing.T) domain.ChannelSelectionSubject {
	t.Helper()

	subject, err := domain.NewChannelSelectionSubject(
		mustValue(t, domain.NewCommercialScopeReference, "scope-a"),
		mustValue(t, domain.NewProductChannelMappingReference, "mapping-1"),
	)
	if err != nil {
		t.Fatalf("造被择优对象引用：%v", err)
	}
	return subject
}

func selectionDecisionSpec(t *testing.T, costs ...domain.ChannelCandidateCost) domain.ChannelSelectionDecisionSpec {
	t.Helper()

	return domain.ChannelSelectionDecisionSpec{
		ID:            mustValue(t, domain.NewChannelSelectionDecisionID, "decision-1"),
		Tenant:        mustValue(t, domain.NewTenantID, "tenant-1"),
		Subject:       selectionSubject(t),
		AssembledAsOf: selectionDecidedAt.Add(-time.Minute),
		DecidedAt:     selectionDecidedAt,
		Costs:         costs,
	}
}

func selectionResultOf(t *testing.T, decision domain.ChannelSelectionDecision, candidate string) domain.ChannelCandidateResult {
	t.Helper()

	for _, result := range decision.Results() {
		if result.Candidate().String() == candidate {
			return result
		}
	}
	t.Fatalf("决定记录里没有候选 %s", candidate)
	return domain.ChannelCandidateResult{}
}

// Covers: 裁决「逐候选结果：选中 / 出局 / 落选」三格同时在场的一次择优，各自落各自那一格，
// 且出局带因由、落选不带——落选与出局在「没赢」上同形，分开记正是本票要答的事。
func TestAChannelSelectionDecisionRecordsEachCandidateInItsOwnGrade(t *testing.T) {
	t.Parallel()

	decision, err := domain.FormChannelSelectionDecision(selectionDecisionSpec(t,
		pricedCost(t, "cand-cheap", "10.00"),
		pricedCost(t, "cand-dear", "12.50"),
		unpriceableCost(t, "cand-excluded", domain.ChannelCostRatecardExclusion),
	))
	if err != nil {
		t.Fatalf("形成决定记录：%v", err)
	}

	if decision.Conclusion() != domain.ChannelSelectionConcludedSelected {
		t.Fatalf("结论 = %s，want SELECTED", decision.Conclusion())
	}
	selected, chosen := decision.Selected()
	if !chosen || selected.String() != "cand-cheap" {
		t.Fatalf("选中 = %v/%v，want cand-cheap", selected, chosen)
	}
	if decision.Rule() != domain.ChannelSelectionByCostOnly {
		t.Fatalf("规则引用 = %s，want COST_ONLY", decision.Rule())
	}
	if got := selectionResultOf(t, decision, "cand-cheap").Outcome(); got != domain.ChannelCandidateSelected {
		t.Fatalf("最便宜者 = %s，want SELECTED", got)
	}
	if got := selectionResultOf(t, decision, "cand-dear").Outcome(); got != domain.ChannelCandidateNotSelected {
		t.Fatalf("可计价但更贵者 = %s，want NOT_SELECTED", got)
	}
	excluded := selectionResultOf(t, decision, "cand-excluded")
	if excluded.Outcome() != domain.ChannelCandidateExcluded {
		t.Fatalf("算不出者 = %s，want EXCLUDED", excluded.Outcome())
	}
	grade, hasGrade := excluded.Exclusion()
	if !hasGrade || grade != domain.ChannelCostRatecardExclusion {
		t.Fatalf("出局因由 = %s/%v，want RATECARD_EXCLUSION", grade, hasGrade)
	}
	if _, hasGrade := selectionResultOf(t, decision, "cand-dear").Exclusion(); hasGrade {
		t.Fatal("落选者带上了出局因由——落选是比输了，不是算不出")
	}
	if len(decision.Results()) != 3 {
		t.Fatalf("逐候选结果 %d 条，want 3", len(decision.Results()))
	}
}

// Covers: 裁决「并列冲突（PAR-NET-16：并列且无法选出唯一一条时为冲突，交人工裁决）——冲突也是
// 一次决定的结果，同样留痕」。最低价那一格上的两家各记 TIED，比它们贵的照旧记落选，整条记录无
// 选中者。
func TestATiedLowestCostIsRecordedAsAConflictOnEachTiedCandidate(t *testing.T) {
	t.Parallel()

	decision, err := domain.FormChannelSelectionDecision(selectionDecisionSpec(t,
		pricedCost(t, "cand-a", "10.00"),
		pricedCost(t, "cand-b", "10.0"),
		pricedCost(t, "cand-c", "11.00"),
	))
	if err != nil {
		t.Fatalf("形成决定记录：%v", err)
	}

	if decision.Conclusion() != domain.ChannelSelectionConcludedTied {
		t.Fatalf("结论 = %s，want TIED", decision.Conclusion())
	}
	if _, chosen := decision.Selected(); chosen {
		t.Fatal("并列冲突的决定交出了一个选中者")
	}
	for _, tied := range []string{"cand-a", "cand-b"} {
		if got := selectionResultOf(t, decision, tied).Outcome(); got != domain.ChannelCandidateTied {
			t.Fatalf("%s = %s，want TIED", tied, got)
		}
	}
	if got := selectionResultOf(t, decision, "cand-c").Outcome(); got != domain.ChannelCandidateNotSelected {
		t.Fatalf("比并列者贵的候选 = %s，want NOT_SELECTED", got)
	}
}

// Covers: 一个候选都没资格参选时决定仍成一条记录——四格因由各自留在自己的候选上，「全部出局」
// 与「没做过择优」在库里从此分得开。
func TestADecisionWithNoQualifiedCandidateStillRecordsEveryExclusion(t *testing.T) {
	t.Parallel()

	decision, err := domain.FormChannelSelectionDecision(selectionDecisionSpec(t,
		unpriceableCost(t, "cand-pending", domain.ChannelCostPendingEvidence),
		unpriceableCost(t, "cand-conflict", domain.ChannelCostConflict),
		unpriceableCost(t, "cand-failed", domain.ChannelCostNotFormed),
	))
	if err != nil {
		t.Fatalf("形成决定记录：%v", err)
	}

	if decision.Conclusion() != domain.ChannelSelectionConcludedNoneQualified {
		t.Fatalf("结论 = %s，want NONE_QUALIFIED", decision.Conclusion())
	}
	want := map[string]domain.ChannelCostUnavailability{
		"cand-pending":  domain.ChannelCostPendingEvidence,
		"cand-conflict": domain.ChannelCostConflict,
		"cand-failed":   domain.ChannelCostNotFormed,
	}
	for candidate, grade := range want {
		result := selectionResultOf(t, decision, candidate)
		got, hasGrade := result.Exclusion()
		if result.Outcome() != domain.ChannelCandidateExcluded || !hasGrade || got != grade {
			t.Fatalf("%s = %s/%s，want EXCLUDED/%s", candidate, result.Outcome(), got, grade)
		}
	}
}

// Covers: 币种不齐时比较器拒绝比较，决定记录同样不成立——没有比较就没有「谁落选」可记，硬记
// 会把一次没发生的择优写成一条决定。
func TestACurrencyMismatchCannotFormADecision(t *testing.T) {
	t.Parallel()

	_, err := domain.FormChannelSelectionDecision(selectionDecisionSpec(t,
		pricedCostIn(t, "cand-a", "10.00", "SYN"),
		pricedCostIn(t, "cand-b", "10.00", "SYX"),
	))
	if !errors.Is(err, domain.ErrChannelCostCurrencyMismatch) {
		t.Fatalf("err = %v，want %v", err, domain.ErrChannelCostCurrencyMismatch)
	}
}

// Covers: 构造门只校形状——候选集非空、同一候选不重复、时刻在场。这些在编排里造不出来，但
// 记录是要落库的，门要写在类型上而不是靠调用方记住。
func TestAChannelSelectionDecisionRefusesAMalformedShape(t *testing.T) {
	t.Parallel()

	cases := map[string]func(spec *domain.ChannelSelectionDecisionSpec){
		"候选集为空": func(spec *domain.ChannelSelectionDecisionSpec) { spec.Costs = nil },
		"同一候选出现两次": func(spec *domain.ChannelSelectionDecisionSpec) {
			spec.Costs = append(spec.Costs, pricedCost(t, "cand-cheap", "9.00"))
		},
		"没有决定时刻": func(spec *domain.ChannelSelectionDecisionSpec) { spec.DecidedAt = time.Time{} },
		"没有装配时点": func(spec *domain.ChannelSelectionDecisionSpec) { spec.AssembledAsOf = time.Time{} },
		"没有标识":   func(spec *domain.ChannelSelectionDecisionSpec) { spec.ID = domain.ChannelSelectionDecisionID{} },
		"没有对象引用": func(spec *domain.ChannelSelectionDecisionSpec) {
			spec.Subject = domain.ChannelSelectionSubject{}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			spec := selectionDecisionSpec(t, pricedCost(t, "cand-cheap", "10.00"))
			mutate(&spec)
			if _, err := domain.FormChannelSelectionDecision(spec); !errors.Is(err, domain.ErrInvalidChannelSelectionDecision) {
				t.Fatalf("err = %v，want %v", err, domain.ErrInvalidChannelSelectionDecision)
			}
		})
	}
}

// Covers: 裁决「每候选：渠道候选引用 + 所用 BUY 评价引用，不拷金额不拷评价内容」。评价引用随成本
// 取值进来就随结果出去；没经评价的候选（没登记价卡）这一格如实缺席。
func TestAChannelCandidateResultCarriesTheEvaluationReferenceItWasPricedFrom(t *testing.T) {
	t.Parallel()

	evaluated, err := pricedCost(t, "cand-evaluated", "10.00").
		WithEvaluation(mustValue(t, domain.NewChannelCostEvaluationReference, "eval-77"))
	if err != nil {
		t.Fatalf("带上评价引用：%v", err)
	}
	decision, err := domain.FormChannelSelectionDecision(selectionDecisionSpec(t,
		evaluated,
		unpriceableCost(t, "cand-uncarded", domain.ChannelCostPendingEvidence),
	))
	if err != nil {
		t.Fatalf("形成决定记录：%v", err)
	}

	reference, present := selectionResultOf(t, decision, "cand-evaluated").Evaluation()
	if !present || reference.String() != "eval-77" {
		t.Fatalf("评价引用 = %v/%v，want eval-77", reference, present)
	}
	if _, present := selectionResultOf(t, decision, "cand-uncarded").Evaluation(); present {
		t.Fatal("没经评价的候选凭空带上了评价引用")
	}
	if _, err := pricedCost(t, "cand-x", "1.00").WithEvaluation(domain.ChannelCostEvaluationReference{}); !errors.Is(err, domain.ErrInvalidChannelCandidateCost) {
		t.Fatalf("空评价引用 err = %v，want %v", err, domain.ErrInvalidChannelCandidateCost)
	}
}
