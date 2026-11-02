package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var billAuditedAt = billMatchedAt.Add(time.Hour)

// decide 在 match 那一行上给一份决定；basis 与 resolves 为空即不带。
func decide(
	t *testing.T,
	match domain.BillLineMatch,
	prior []domain.BillLineAuditDecision,
	id string,
	verdict domain.AuditVerdict,
	from, to int64,
	basis, resolves string,
) domain.BillLineAuditDecision {
	t.Helper()
	decision, err := domain.DecideBillLineAudit(match, prior, decisionSpec(t, id, verdict, from, to, basis, resolves))
	if err != nil {
		t.Fatalf("decide %s %s [%d, %d): %v", id, verdict, from, to, err)
	}
	return decision
}

func decisionSpec(t *testing.T, id string, verdict domain.AuditVerdict, from, to int64, basis, resolves string) domain.BillLineAuditDecisionSpec {
	t.Helper()
	span, err := domain.NewAmountRange(from, to)
	if err != nil {
		t.Fatalf("range [%d, %d): %v", from, to, err)
	}
	spec := domain.BillLineAuditDecisionSpec{
		Decision:  settlementValue(t, domain.NewAuditDecisionID, id),
		Version:   settlementValue(t, domain.NewBillClaimVersion, "bill-claim/v1"),
		Verdict:   verdict,
		Span:      span,
		Auditor:   settlementValue(t, domain.NewAuditorReference, "auditor-1"),
		AuditedAt: billAuditedAt,
	}
	if basis != "" {
		spec.Basis = settlementValue(t, domain.NewAuditBasisReference, basis)
	}
	if resolves != "" {
		spec.Resolves = settlementValue(t, domain.NewAuditDecisionID, resolves)
	}
	return spec
}

func lineMatch(t *testing.T, classification domain.MatchClassification, expectedMinor int64, basis string) domain.BillLineMatch {
	t.Helper()
	claim := receivedClaim(t)
	expected := domain.SupplierExpectedCost{}
	if expectedMinor > 0 {
		expected = expectedCost(t, "cost/v1", expectedMinor)
	}
	match, err := domain.MatchBillLine(claim, settlementValue(t, domain.NewBillLineReference, "line-1"),
		classification, expected, optionalBasis(t, basis), billMatchedAt)
	if err != nil {
		t.Fatalf("match line-1 %s: %v", classification, err)
	}
	return match
}

func optionalBasis(t *testing.T, value string) domain.MatchBasisReference {
	if value == "" {
		return domain.MatchBasisReference{}
	}
	return settlementValue(t, domain.NewMatchBasisReference, value)
}

// Covers: `AT-SA-234`「通过的各段之和不超过这一行的已匹配金额（价差行只到预期成本，没有对应发生项与重复计费是零）」——
// 已匹配金额只在 MatchedAmountFor 一处定义。line-1 的主张金额是 12000。
func TestTheMatchedAmountBoundsWhatCanPass(t *testing.T) {
	cases := []struct {
		name           string
		classification domain.MatchClassification
		expected       int64
		basis          string
		matched        int64
	}{
		{"matched line passes in full", domain.LineMatched, 12000, "", 12000},
		{"overcharged price variance passes only up to the expected cost", domain.PriceVariance, 11000, "price-1", 11000},
		{"undercharged quantity variance passes in full", domain.QuantityVariance, 13000, "weight-1", 12000},
		{"no matching occurrence passes nothing", domain.NoMatchingOccurrence, 0, "search-1", 0},
		{"duplicate billing passes nothing", domain.DuplicateBilling, 12000, "claim-0", 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			match := lineMatch(t, testCase.classification, testCase.expected, testCase.basis)
			if got := match.MatchedMinor(); got != testCase.matched {
				t.Fatalf("matched amount = %d, want %d", got, testCase.matched)
			}
			if testCase.matched > 0 {
				decide(t, match, nil, "pass-all", domain.AuditPassed, 0, testCase.matched, "", "")
			}
			if testCase.matched < 12000 {
				if _, err := domain.DecideBillLineAudit(match, nil,
					decisionSpec(t, "pass-over", domain.AuditPassed, 0, testCase.matched+1, "", "")); !errors.Is(err, domain.ErrNotAuditable) {
					t.Fatalf("passing past the matched amount: error = %v, want ErrNotAuditable", err)
				}
				// 差异那一段、没有成本撑着的整行，争议与拒绝照样给得出——它们的处理入口。
				decide(t, match, nil, "dispute-rest", domain.AuditDisputed, testCase.matched, 12000, "evidence-1", "")
			}
		})
	}
}

// Covers: `AT-SA-234`「一行拆成几段分别给通过、争议与拒绝；各段在主张金额之内、互不重叠；通过与拒绝终局」。
func TestALineSplitsIntoNonOverlappingRanges(t *testing.T) {
	match := lineMatch(t, domain.LineMatched, 12000, "")
	passed := decide(t, match, nil, "d-pass", domain.AuditPassed, 0, 6000, "", "")
	disputed := decide(t, match, []domain.BillLineAuditDecision{passed}, "d-dispute", domain.AuditDisputed, 6000, 9000, "weight-proof", "")
	prior := []domain.BillLineAuditDecision{passed, disputed}
	rejected := decide(t, match, prior, "d-reject", domain.AuditRejected, 9000, 12000, "not-ordered", "")
	prior = append(prior, rejected)

	tally := domain.TallyLineDecisions(12000, prior)
	if tally != (domain.LineAuditTally{PassedMinor: 6000, RejectedMinor: 3000, OpenDisputeMinor: 3000}) {
		t.Fatalf("tally = %+v", tally)
	}

	refused := []struct {
		name    string
		spec    domain.BillLineAuditDecisionSpec
		wantErr error
	}{
		{"a passed range is final", decisionSpec(t, "x1", domain.AuditRejected, 1000, 2000, "late", ""), domain.ErrAuditRangeTaken},
		{"a rejected range is final", decisionSpec(t, "x2", domain.AuditPassed, 9000, 9500, "", ""), domain.ErrAuditRangeTaken},
		{"a disputed range is not re-decided directly", decisionSpec(t, "x3", domain.AuditPassed, 6000, 7000, "", ""), domain.ErrAuditRangeTaken},
		{"straddling two ranges", decisionSpec(t, "x4", domain.AuditDisputed, 5000, 6500, "b", ""), domain.ErrAuditRangeTaken},
		{"past the claimed amount", decisionSpec(t, "x5", domain.AuditDisputed, 11000, 12001, "b", ""), domain.ErrAuditRangeOutsideLine},
		{"a dispute without basis", decisionSpec(t, "x6", domain.AuditDisputed, 0, 1, "", ""), domain.ErrInvalidAuditDecision},
		{"a rejection without basis", decisionSpec(t, "x7", domain.AuditRejected, 0, 1, "", ""), domain.ErrInvalidAuditDecision},
	}
	for _, testCase := range refused {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := domain.DecideBillLineAudit(match, prior, testCase.spec); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("error = %v, want %v", err, testCase.wantErr)
			}
		})
	}

	t.Run("ranges that only touch do not overlap", func(t *testing.T) {
		first := decide(t, match, nil, "a", domain.AuditPassed, 0, 100, "", "")
		decide(t, match, []domain.BillLineAuditDecision{first}, "b", domain.AuditPassed, 100, 200, "", "")
	})

	t.Run("the same change under another identity repeats the earlier decision", func(t *testing.T) {
		span, _ := domain.NewAmountRange(0, 6000)
		if !passed.Repeats(domain.AuditPassed, span, domain.AuditBasisReference{}, domain.AuditDecisionID{}) {
			t.Fatal("同一段同一结果，Repeats 应认作同一个业务变化")
		}
		other, _ := domain.NewAmountRange(0, 5000)
		if passed.Repeats(domain.AuditPassed, other, domain.AuditBasisReference{}, domain.AuditDecisionID{}) ||
			passed.Repeats(domain.AuditRejected, span, settlementValue(t, domain.NewAuditBasisReference, "b"), domain.AuditDecisionID{}) {
			t.Fatal("范围或结果不同不是同一个业务变化")
		}
	})

	t.Run("an empty or inverted range is no range", func(t *testing.T) {
		for _, bounds := range [][2]int64{{5, 5}, {6, 5}, {-1, 3}} {
			if _, err := domain.NewAmountRange(bounds[0], bounds[1]); err == nil {
				t.Fatalf("[%d, %d) 应拒", bounds[0], bounds[1])
			}
		}
	})
}

// Covers: `AT-SA-235`「再审点名所解的争议，范围落在原争议之内、只能通过或拒绝、必须带依据；同一争议的几份再审互不重叠，
// 没覆盖到的部分仍在争议中；已通过与已拒绝的范围碰不到」。
func TestADisputeIsResolvedByReauditsWithinIt(t *testing.T) {
	match := lineMatch(t, domain.PriceVariance, 11000, "price-1")
	passed := decide(t, match, nil, "pass", domain.AuditPassed, 0, 8000, "", "")
	dispute := decide(t, match, []domain.BillLineAuditDecision{passed}, "dispute", domain.AuditDisputed, 8000, 12000, "price-dispute", "")
	prior := []domain.BillLineAuditDecision{passed, dispute}

	resolvedPass := decide(t, match, prior, "re-pass", domain.AuditPassed, 8000, 11000, "contract-confirmed", "dispute")
	prior = append(prior, resolvedPass)
	tally := domain.TallyLineDecisions(12000, prior)
	if tally.OpenDisputeMinor != 1000 || tally.PassedMinor != 11000 || tally.UndecidedMinor != 0 {
		t.Fatalf("部分解除之后 tally = %+v，应仍有 1000 在争议中", tally)
	}

	refused := []struct {
		name    string
		spec    domain.BillLineAuditDecisionSpec
		wantErr error
	}{
		{"past the matched amount even on re-audit", decisionSpec(t, "r1", domain.AuditPassed, 11000, 12000, "b", "dispute"), domain.ErrNotAuditable},
		{"overlapping another re-audit of the same dispute", decisionSpec(t, "r2", domain.AuditRejected, 10500, 12000, "b", "dispute"), domain.ErrAuditRangeTaken},
		{"reaching outside the dispute into the passed range", decisionSpec(t, "r3", domain.AuditRejected, 7000, 9000, "b", "dispute"), domain.ErrAuditRangeOutsideLine},
		{"a re-audit cannot dispute again", decisionSpec(t, "r4", domain.AuditDisputed, 11000, 12000, "b", "dispute"), domain.ErrInvalidAuditDecision},
		{"a re-audit needs its basis", decisionSpec(t, "r5", domain.AuditRejected, 11000, 12000, "", "dispute"), domain.ErrInvalidAuditDecision},
		{"resolving a decision that is not a dispute", decisionSpec(t, "r6", domain.AuditRejected, 0, 100, "b", "pass"), domain.ErrDisputeNotFound},
		{"resolving an unknown decision", decisionSpec(t, "r7", domain.AuditRejected, 11000, 12000, "b", "nowhere"), domain.ErrDisputeNotFound},
	}
	for _, testCase := range refused {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := domain.DecideBillLineAudit(match, prior, testCase.spec); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("error = %v, want %v", err, testCase.wantErr)
			}
		})
	}

	rejectedRest := decide(t, match, prior, "re-reject", domain.AuditRejected, 11000, 12000, "excess-refused", "dispute")
	prior = append(prior, rejectedRest)
	if tally := domain.TallyLineDecisions(12000, prior); tally != (domain.LineAuditTally{PassedMinor: 11000, RejectedMinor: 1000}) {
		t.Fatalf("争议覆盖完之后 tally = %+v", tally)
	}
}

// Covers: `AT-SA-234`「审核结果只认三种字面」。
func TestAuditVerdictsAreAClosedSet(t *testing.T) {
	for _, verdict := range []domain.AuditVerdict{domain.AuditPassed, domain.AuditDisputed, domain.AuditRejected} {
		parsed, err := domain.AuditVerdictFromName(verdict.String())
		if err != nil || parsed != verdict {
			t.Fatalf("%s 往返失败：%v %v", verdict, parsed, err)
		}
	}
	for _, name := range []string{"", "APPROVED", "passed", "ESCALATED"} {
		if _, err := domain.AuditVerdictFromName(name); !errors.Is(err, domain.ErrUnknownAuditVerdict) {
			t.Fatalf("%q 应拒，实得 %v", name, err)
		}
	}
	if domain.AuditVerdict(4).String() != "" {
		t.Fatal("第四个结果带了字面——封闭集合被悄悄放开")
	}
}

// Covers: 重建门复验每份决定自证的那几件（ADR-0030）：回填的旧应付没有版本照样重建，争议没有依据、再审是争议则拒。
func TestAnAuditDecisionRehydratesOnlyWhenItsShapeHolds(t *testing.T) {
	span, _ := domain.NewAmountRange(0, 500)
	spec := domain.RehydrateBillLineAuditDecisionSpec{
		Decision:  settlementValue(t, domain.NewAuditDecisionID, "payable-legacy"),
		Claim:     settlementValue(t, domain.NewBillClaimID, "bill-claim-1"),
		Line:      settlementValue(t, domain.NewBillLineReference, "line-1"),
		Verdict:   domain.AuditPassed,
		Span:      span,
		Auditor:   settlementValue(t, domain.NewAuditorReference, "auditor-1"),
		AuditedAt: billAuditedAt,
	}
	decision, err := domain.RehydrateBillLineAuditDecision(spec)
	if err != nil {
		t.Fatalf("rehydrate legacy pass: %v", err)
	}
	if _, has := decision.Version(); has {
		t.Fatal("回填的旧应付不该有版本")
	}
	broken := spec
	broken.Verdict = domain.AuditDisputed
	if _, err := domain.RehydrateBillLineAuditDecision(broken); !errors.Is(err, domain.ErrInvalidAuditDecision) {
		t.Fatalf("没有依据的争议 error = %v", err)
	}
	broken = spec
	broken.Verdict, broken.Basis = domain.AuditDisputed, settlementValue(t, domain.NewAuditBasisReference, "b")
	broken.Resolves = settlementValue(t, domain.NewAuditDecisionID, "another")
	if _, err := domain.RehydrateBillLineAuditDecision(broken); !errors.Is(err, domain.ErrInvalidAuditDecision) {
		t.Fatalf("解争议的争议 error = %v", err)
	}
}
