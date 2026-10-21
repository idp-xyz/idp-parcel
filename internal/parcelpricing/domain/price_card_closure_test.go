package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 本文件证价卡收尾的判据（ADR-0234）：截止把旧版此刻的选用截在后继起点、退役让旧版整版不再参选，两种都要后继接得住
// 旧版此后的那一段；重放认出册上那一条；不成立的收尾逐格拒。夹具全是合成价卡，证据只记 S。

var (
	closureStart  = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	closureMiddle = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	closureLater  = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	closureEnd    = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	closureAt     = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
)

type closureCard struct {
	version      string
	scope        string
	direction    domain.PricingDirection
	starts, ends time.Time
}

func closurePlan(t *testing.T, card closureCard) domain.PricingPlanVersion {
	t.Helper()
	scope, direction := card.scope, card.direction
	if scope == "" {
		scope = "SYN-SCOPE-CLOSURE"
	}
	if direction == "" {
		direction = domain.PricingDirectionBuy
	}
	purpose := domain.PricingPurposeSupplierCost
	if direction == domain.PricingDirectionSell {
		purpose = domain.PricingPurposeCustomerCharge
	}
	ends := card.ends
	if ends.IsZero() {
		ends = closureEnd
	}
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-CLOSURE", card.version),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, "SYN-TABLE-CLOSURE", card.version),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, "SYN-WEIGHT-CLOSURE", card.version),
		Scope:                 scope,
		Direction:             direction,
		Purpose:               purpose,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: card.starts, EndsAt: ends},
		RateEntryID:           "SYN-CLOSURE-Z1-" + card.version,
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "10",
		WeightRounding:        domain.RoundingNone,
		WeightStepKilograms:   "1",
	})
}

func closureTenant(t *testing.T) domain.TenantID {
	t.Helper()
	return pptest.Value(t, domain.NewTenantID, "SYN-TENANT-CLOSURE")
}

func closureReference(t *testing.T, raw string) domain.PriceCardClosureReference {
	t.Helper()
	return pptest.Value(t, domain.NewPriceCardClosureReference, raw)
}

func standingOf(t *testing.T, card closureCard, cuts []domain.PriceCardCut, retirement *domain.PriceCardRetirement) domain.PriceCardStanding {
	t.Helper()
	standing, err := domain.NewPriceCardStanding(closureTenant(t), closurePlan(t, card), cuts, retirement)
	if err != nil {
		t.Fatalf("拼 %s 此刻的样子：%v", card.version, err)
	}
	return standing
}

func mustSelect(t *testing.T, standing domain.PriceCardStanding) domain.EffectivePeriod {
	t.Helper()
	period, selects := standing.Selection()
	if !selects {
		t.Fatalf("%s 此刻一刻也选不中", standing.Plan().Reference().Version())
	}
	return period
}

// Covers: AT-PP-007——后继晚于旧版起点：截止点取后继的起点，旧版此刻的选用到那一刻为止、前一段照旧，后继接着往后。
func TestACutHandsThePredecessorsRestToTheSuccessorAtItsStart(t *testing.T) {
	v1 := closureCard{version: "v1", starts: closureStart}
	v2 := closureCard{version: "v2", starts: closureMiddle}
	old := standingOf(t, v1, nil, nil)

	cut, replay, err := old.CutForSuccessor(standingOf(t, v2, nil, nil), closureReference(t, "SYN-CUT-V2"), closureAt)
	if err != nil || replay {
		t.Fatalf("截止 replay=%v err=%v", replay, err)
	}
	if !cut.SelectionEndsAt().Equal(closureMiddle) || cut.Successor().Version() != "v2" || !cut.RegisteredAt().Equal(closureAt) {
		t.Fatalf("截止 = 到 %s、后继 %s、登记于 %s", cut.SelectionEndsAt(), cut.Successor().Version(), cut.RegisteredAt())
	}

	after := standingOf(t, v1, []domain.PriceCardCut{cut}, nil)
	selection := mustSelect(t, after)
	if !selection.StartsAt().Equal(closureStart) || !selection.EndsAt().Equal(closureMiddle) {
		t.Fatalf("截止之后旧版选 %s–%s，want %s–%s", selection.StartsAt(), selection.EndsAt(), closureStart, closureMiddle)
	}
	if !selection.Contains(closureMiddle.Add(-time.Second)) || selection.Contains(closureMiddle) {
		t.Fatal("截止点之前应仍选旧版、截止点起不再选")
	}
	if published := after.Plan().EffectivePeriod(); !published.EndsAt().Equal(closureEnd) {
		t.Fatalf("发布时的适用期被改写成 %s", published.EndsAt())
	}
}

// Covers: AT-PP-008——后继不晚于旧版起点：旧版退役，此刻一刻也选不中；后继连起点一起改正（更早起）也成立。
func TestARetirementLeavesOnlyTheCoveringSuccessor(t *testing.T) {
	for name, successorStart := range map[string]time.Time{
		"同起点重发": closureStart,
		"起点改早":  closureStart.Add(-30 * 24 * time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			v1 := closureCard{version: "v1", starts: closureStart}
			v2 := closureCard{version: "v2", starts: successorStart}
			retirement, replay, err := standingOf(t, v1, nil, nil).RetireForCoveringSuccessor(
				standingOf(t, v2, nil, nil), closureReference(t, "SYN-RETIRE-V1"), closureAt)
			if err != nil || replay {
				t.Fatalf("退役 replay=%v err=%v", replay, err)
			}
			if !retirement.RetiredAt().Equal(closureAt) || retirement.Successor().Version() != "v2" {
				t.Fatalf("退役 = 让位给 %s、于 %s", retirement.Successor().Version(), retirement.RetiredAt())
			}
			if _, selects := standingOf(t, v1, nil, &retirement).Selection(); selects {
				t.Fatal("退役之后旧版仍选得中")
			}
		})
	}
}

// Covers: AT-PP-009——重放：同一后继、同一依据再截一次认出册上那一条（判在区间检查之前），换一份依据截同一处答越界；
// 退役重放交回原来的时刻，换依据或换后继答已凭别的退役；退役之后再截答已退役，截止过的那一版仍可整段让位退役。
func TestReplayingAClosureRecognisesTheRegisteredOne(t *testing.T) {
	v1 := closureCard{version: "v1", starts: closureStart}
	v2 := closureCard{version: "v2", starts: closureMiddle}
	v3 := closureCard{version: "v3", starts: closureStart, ends: closureEnd}
	successor := standingOf(t, v2, nil, nil)
	cut, _, err := standingOf(t, v1, nil, nil).CutForSuccessor(successor, closureReference(t, "SYN-CUT-V2"), closureAt)
	if err != nil {
		t.Fatal(err)
	}
	cutOnce := standingOf(t, v1, []domain.PriceCardCut{cut}, nil)

	again, replay, err := cutOnce.CutForSuccessor(successor, closureReference(t, "SYN-CUT-V2"), closureAt.Add(time.Hour))
	if err != nil || !replay || !again.RegisteredAt().Equal(closureAt) {
		t.Fatalf("截止重放 replay=%v err=%v 登记于 %s，want 认出原来那一条", replay, err, again.RegisteredAt())
	}
	if _, _, err := cutOnce.CutForSuccessor(successor, closureReference(t, "SYN-CUT-V2-OTHER"), closureAt); !errors.Is(err, domain.ErrPriceCardSuccessorCutOutsideSelection) {
		t.Fatalf("换依据截同一处 err = %v，want ErrPriceCardSuccessorCutOutsideSelection", err)
	}

	covering := standingOf(t, v3, nil, nil)
	retirement, replay, err := cutOnce.RetireForCoveringSuccessor(covering, closureReference(t, "SYN-RETIRE-V1"), closureAt)
	if err != nil || replay {
		t.Fatalf("截止过的旧版整段让位给 v3：replay=%v err=%v", replay, err)
	}
	retired := standingOf(t, v1, []domain.PriceCardCut{cut}, &retirement)
	echoed, replay, err := retired.RetireForCoveringSuccessor(covering, closureReference(t, "SYN-RETIRE-V1"), closureAt.Add(time.Hour))
	if err != nil || !replay || !echoed.RetiredAt().Equal(closureAt) {
		t.Fatalf("退役重放 replay=%v err=%v 时刻 %s，want 交回原来的时刻", replay, err, echoed.RetiredAt())
	}
	if _, _, err := retired.RetireForCoveringSuccessor(covering, closureReference(t, "SYN-RETIRE-V1-OTHER"), closureAt); !errors.Is(err, domain.ErrPriceCardRetiredOtherwise) {
		t.Fatalf("换依据再退 err = %v，want ErrPriceCardRetiredOtherwise", err)
	}
	if _, replay, err := retired.CutForSuccessor(successor, closureReference(t, "SYN-CUT-V2"), closureAt); err != nil || !replay {
		t.Fatalf("退役之后重放那次截止 replay=%v err=%v，want 仍认出", replay, err)
	}
	if _, _, err := retired.CutForSuccessor(standingOf(t, closureCard{version: "v4", starts: closureLater}, nil, nil),
		closureReference(t, "SYN-CUT-V4"), closureAt); !errors.Is(err, domain.ErrPriceCardClosureRetired) {
		t.Fatalf("退役之后再截 err = %v，want ErrPriceCardClosureRetired", err)
	}
}

// Covers: AT-PP-010——不成立的收尾逐格拒。
func TestAPriceCardClosureThatDoesNotHoldIsRefused(t *testing.T) {
	v1 := closureCard{version: "v1", starts: closureStart, ends: closureLater}
	retiredSuccessor := func(t *testing.T) domain.PriceCardStanding {
		retirement, err := domain.RehydratePriceCardRetirement(closureTenant(t),
			pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-CLOSURE", "v2"),
			pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-CLOSURE", "v9"),
			closureReference(t, "SYN-RETIRE-V2"), closureAt)
		if err != nil {
			t.Fatal(err)
		}
		return standingOf(t, closureCard{version: "v2", starts: closureMiddle}, nil, &retirement)
	}
	for _, refused := range []struct {
		name      string
		successor func(*testing.T) domain.PriceCardStanding
		cut       error
		retire    error
	}{
		{"后继就是旧版", func(t *testing.T) domain.PriceCardStanding { return standingOf(t, v1, nil, nil) },
			domain.ErrPriceCardClosureSameVersion, domain.ErrPriceCardClosureSameVersion},
		{"换了计价范围", func(t *testing.T) domain.PriceCardStanding {
			return standingOf(t, closureCard{version: "v2", scope: "SYN-SCOPE-OTHER", starts: closureMiddle}, nil, nil)
		}, domain.ErrPriceCardClosureAcrossScopes, domain.ErrPriceCardClosureAcrossScopes},
		{"换了方向", func(t *testing.T) domain.PriceCardStanding {
			return standingOf(t, closureCard{version: "v2", direction: domain.PricingDirectionSell, starts: closureMiddle}, nil, nil)
		}, domain.ErrPriceCardClosureAcrossScopes, domain.ErrPriceCardClosureAcrossScopes},
		{"后继同起点", func(t *testing.T) domain.PriceCardStanding {
			return standingOf(t, closureCard{version: "v2", starts: closureStart}, nil, nil)
		}, domain.ErrPriceCardSuccessorCutOutsideSelection, nil},
		{"后继从旧版终点起", func(t *testing.T) domain.PriceCardStanding {
			return standingOf(t, closureCard{version: "v2", starts: closureLater}, nil, nil)
		}, domain.ErrPriceCardSuccessorCutOutsideSelection, domain.ErrPriceCardRetirementSuccessorStartsLater},
		{"后继中途起", func(t *testing.T) domain.PriceCardStanding {
			return standingOf(t, closureCard{version: "v2", starts: closureMiddle}, nil, nil)
		}, nil, domain.ErrPriceCardRetirementSuccessorStartsLater},
		{"后继到不了旧版终点", func(t *testing.T) domain.PriceCardStanding {
			return standingOf(t, closureCard{version: "v2", starts: closureStart.Add(time.Hour), ends: closureMiddle}, nil, nil)
		}, domain.ErrPriceCardClosureLeavesGap, domain.ErrPriceCardRetirementSuccessorStartsLater},
		{"同起点后继到不了旧版终点", func(t *testing.T) domain.PriceCardStanding {
			return standingOf(t, closureCard{version: "v2", starts: closureStart, ends: closureMiddle}, nil, nil)
		}, domain.ErrPriceCardSuccessorCutOutsideSelection, domain.ErrPriceCardClosureLeavesGap},
		{"后继已退役", retiredSuccessor, domain.ErrPriceCardClosureRetired, domain.ErrPriceCardClosureRetired},
	} {
		t.Run(refused.name, func(t *testing.T) {
			old := standingOf(t, v1, nil, nil)
			successor := refused.successor(t)
			if _, _, err := old.CutForSuccessor(successor, closureReference(t, "SYN-CUT"), closureAt); !errors.Is(err, refused.cut) {
				t.Errorf("截止 err = %v，want %v", err, refused.cut)
			}
			if _, _, err := old.RetireForCoveringSuccessor(successor, closureReference(t, "SYN-RETIRE"), closureAt); !errors.Is(err, refused.retire) {
				t.Errorf("退役 err = %v，want %v", err, refused.retire)
			}
		})
	}

	old := standingOf(t, v1, nil, nil)
	successor := standingOf(t, closureCard{version: "v2", starts: closureMiddle}, nil, nil)
	if _, _, err := old.CutForSuccessor(successor, closureReference(t, "SYN-CUT"), time.Time{}); !errors.Is(err, domain.ErrInvalidPriceCardClosure) {
		t.Errorf("没有登记时刻 err = %v，want ErrInvalidPriceCardClosure", err)
	}
	if _, _, err := old.CutForSuccessor(successor, domain.PriceCardClosureReference{}, closureAt); !errors.Is(err, domain.ErrInvalidPriceCardClosure) {
		t.Errorf("没有依据 err = %v，want ErrInvalidPriceCardClosure", err)
	}
}

// Covers: AT-PP-007 的连截那一格——截止之后再截一次（后继起点落在此刻的选用之内）照样成立，选用按最新一条收窄；截过的
// 一版也能整段让位退役。选用区间的读法只在 SelectablePriceCardPeriod 一处。
func TestTheSelectablePeriodFoldsTheLatestCutAndTheRetirement(t *testing.T) {
	v1 := closureCard{version: "v1", starts: closureStart}
	first, _, err := standingOf(t, v1, nil, nil).CutForSuccessor(
		standingOf(t, closureCard{version: "v2", starts: closureLater}, nil, nil), closureReference(t, "SYN-CUT-V2"), closureAt)
	if err != nil {
		t.Fatal(err)
	}
	second, replay, err := standingOf(t, v1, []domain.PriceCardCut{first}, nil).CutForSuccessor(
		standingOf(t, closureCard{version: "v3", starts: closureMiddle}, nil, nil), closureReference(t, "SYN-CUT-V3"), closureAt)
	if err != nil || replay {
		t.Fatalf("连截 replay=%v err=%v", replay, err)
	}
	if selection := mustSelect(t, standingOf(t, v1, []domain.PriceCardCut{first, second}, nil)); !selection.EndsAt().Equal(closureMiddle) {
		t.Fatalf("连截之后旧版选到 %s，want %s", selection.EndsAt(), closureMiddle)
	}

	published, err := domain.NewEffectivePeriod(closureStart, closureLater)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		name    string
		cutEnd  time.Time
		retired bool
		ends    time.Time
		selects bool
	}{
		{"没有收尾", time.Time{}, false, closureLater, true},
		{"截在期内", closureMiddle, false, closureMiddle, true},
		{"截在期后", closureEnd, false, closureLater, true},
		{"退役", time.Time{}, true, time.Time{}, false},
	} {
		period, selects := domain.SelectablePriceCardPeriod(published, check.cutEnd, check.retired)
		if selects != check.selects || (selects && !period.EndsAt().Equal(check.ends)) {
			t.Errorf("%s：selects=%v 终点 %s，want selects=%v 终点 %s", check.name, selects, period.EndsAt(), check.selects, check.ends)
		}
	}
}
