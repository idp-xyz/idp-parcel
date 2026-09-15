package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var dutyGateVerifiedAt = time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)

func synDutyVerification(
	t *testing.T,
	coverage domain.DutyCoverage,
	delta domain.DutyDelta,
	validity domain.DutyFactValidity,
) domain.DutyPaymentVerification {
	t.Helper()
	verification, err := domain.VerifyDutyPayment(
		mustValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1"),
		mustValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01"),
		mustValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v1"),
		mustValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		coverage, delta, validity, dutyGateVerifiedAt)
	if err != nil {
		t.Fatalf("构造合成核对：%v", err)
	}
	return verification
}

func acceptRule(t *testing.T) domain.DutyPaymentGateRule {
	t.Helper()
	rule, err := domain.AcceptDutyPaymentWhen(
		[]domain.DutyCoverage{domain.CoverageFull},
		[]domain.DutyDelta{domain.DeltaNone, domain.DeltaExcess},
		[]domain.DutyFactValidity{domain.FundsFactValid})
	if err != nil {
		t.Fatalf("构造接受集合规则：%v", err)
	}
	return rule
}

// Covers: ADR-0137 决定三——规则正文两形之一：三个接受集合各非空，或「税费付款不构成本动作在本边界的前置条件」；
// 两形在类型上分得开，读口各答各的。
func TestADutyPaymentGateRuleTakesExactlyTwoShapes(t *testing.T) {
	rule := acceptRule(t)
	if rule.NotAPrecondition() {
		t.Fatal("接受集合规则被读成「不构成前置条件」")
	}
	coverage, delta, validity := rule.Accepts()
	if len(coverage) != 1 || coverage[0] != domain.CoverageFull ||
		len(delta) != 2 || len(validity) != 1 || validity[0] != domain.FundsFactValid {
		t.Fatalf("接受集合走样：%v %v %v", coverage, delta, validity)
	}

	waived := domain.DutyPaymentNotAPrecondition()
	if !waived.NotAPrecondition() {
		t.Fatal("「不构成前置条件」没有读出来")
	}
	if c, d, v := waived.Accepts(); len(c)+len(d)+len(v) != 0 {
		t.Fatalf("不构成前置条件的规则凭空带了接受集合：%v %v %v", c, d, v)
	}
}

// Covers: 「`待确认` / `冲突` 不可登记为接受」（ADR-0137 决定三；UC-CC-003「未知不能当作……有效」）
// ——差额待确认、有效性冲突 / 待确认进接受集合构造期拒；任一集合为空也拒（空集合等于「永不满足」，
// 那不是规则是常量）；集外取值拒。
func TestADutyPaymentGateRuleRefusesPendingConflictingAndEmptyAcceptance(t *testing.T) {
	full := []domain.DutyCoverage{domain.CoverageFull}
	none := []domain.DutyDelta{domain.DeltaNone}
	valid := []domain.DutyFactValidity{domain.FundsFactValid}

	cases := map[string]func() error{
		"差额待确认": func() error {
			_, err := domain.AcceptDutyPaymentWhen(full, []domain.DutyDelta{domain.DeltaPending}, valid)
			return err
		},
		"有效性冲突": func() error {
			_, err := domain.AcceptDutyPaymentWhen(full, none, []domain.DutyFactValidity{domain.FundsFactConflicting})
			return err
		},
		"有效性待确认": func() error {
			_, err := domain.AcceptDutyPaymentWhen(full, none, []domain.DutyFactValidity{domain.FundsFactPending})
			return err
		},
		"覆盖集合为空": func() error {
			_, err := domain.AcceptDutyPaymentWhen(nil, none, valid)
			return err
		},
		"覆盖集外": func() error {
			_, err := domain.AcceptDutyPaymentWhen([]domain.DutyCoverage{domain.DutyCoverage(9)}, none, valid)
			return err
		},
	}
	for name, construct := range cases {
		if err := construct(); !errors.Is(err, domain.ErrInvalidDutyPaymentGateRule) {
			t.Fatalf("%s该拒 ErrInvalidDutyPaymentGateRule，实得 %v", name, err)
		}
	}
	if _, err := domain.AcceptDutyPaymentWhen(full, none, []domain.DutyFactValidity{domain.FundsFactInvalidated}); err != nil {
		t.Fatalf("「失效」可以登记为接受（真实程序可能先放后税）：%v", err)
	}
}

// Covers: 三态各落在自己的接受集合内才满足，任一不在即未满足（ADR-0137 决定三）；判出的三态原值
// 随读数原样带回，不折成合成布尔（CONTEXT「覆盖状态（无覆盖、部分覆盖、已覆盖）、差额状态（无差额、不足、超额或待确认）和有效性状态（有效、失效、冲突或待确认）分别表达」）。
func TestAcceptanceSetsJudgeEachAxisSeparately(t *testing.T) {
	rule := acceptRule(t)

	met, err := rule.Judge(synDutyVerification(t, domain.CoverageFull, domain.DeltaExcess, domain.FundsFactValid))
	if err != nil || met.State != domain.PreconditionMet {
		t.Fatalf("三态都在接受集合内该满足：err=%v state=%v", err, met.State)
	}
	if met.Coverage != domain.CoverageFull || met.Delta != domain.DeltaExcess || met.Validity != domain.FundsFactValid {
		t.Fatalf("读数没带回三态原值：%+v", met)
	}

	for name, verification := range map[string]domain.DutyPaymentVerification{
		"覆盖不在集内":  synDutyVerification(t, domain.CoveragePartial, domain.DeltaNone, domain.FundsFactValid),
		"差额不在集内":  synDutyVerification(t, domain.CoverageFull, domain.DeltaShort, domain.FundsFactValid),
		"有效性不在集内": synDutyVerification(t, domain.CoverageFull, domain.DeltaNone, domain.FundsFactInvalidated),
	} {
		reading, err := rule.Judge(verification)
		if err != nil || reading.State != domain.PreconditionUnmet {
			t.Fatalf("%s该未满足：err=%v state=%v", name, err, reading.State)
		}
	}
}

// Covers: 三态任一为`待确认` / `冲突`时这道门禁未决并指名——不是未满足也不是满足（UC-CC-003「未知不能当作可选、不适用、有效或已解除」）；
// 「不构成前置条件」的规则不判核对。
func TestPendingOrConflictingAxesLeaveTheDutyGateUndecided(t *testing.T) {
	rule := acceptRule(t)
	for name, verification := range map[string]domain.DutyPaymentVerification{
		"差额待确认":  synDutyVerification(t, domain.CoverageFull, domain.DeltaPending, domain.FundsFactValid),
		"有效性冲突":  synDutyVerification(t, domain.CoverageFull, domain.DeltaNone, domain.FundsFactConflicting),
		"有效性待确认": synDutyVerification(t, domain.CoverageFull, domain.DeltaNone, domain.FundsFactPending),
	} {
		if _, err := rule.Judge(verification); !errors.Is(err, domain.ErrDutyPaymentGateUndecided) {
			t.Fatalf("%s该未决 ErrDutyPaymentGateUndecided，实得 %v", name, err)
		}
	}

	if _, err := domain.DutyPaymentNotAPrecondition().Judge(
		synDutyVerification(t, domain.CoverageNone, domain.DeltaShort, domain.FundsFactValid)); !errors.Is(err, domain.ErrInvalidDutyPaymentGateRule) {
		t.Fatalf("不构成前置条件的规则不该拿去判核对：%v", err)
	}
}

// Covers: 门禁记录带税费付款那一道的读数——三态原值 + 核对版本引用（ADR-0137 决定三；票 sa-cc/06
// 裁决 2 引用不快照）；读数只能挂在前置条件清单含「税费付款」的判断上，没有那一项却带读数是矛盾；
// 未挂读数的判断读口答「无」。
func TestAReleaseGateCarriesTheDutyPaymentReadingByReference(t *testing.T) {
	reading := domain.DutyPaymentGateReading{
		State:    domain.PreconditionMet,
		Coverage: domain.CoverageFull,
		Delta:    domain.DeltaNone,
		Validity: domain.FundsFactValid,
		Verification: domain.DutyVerificationReference{
			Duty:    mustValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1"),
			Funds:   mustValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01"),
			Version: "SYN-DIGEST-01",
		},
	}
	withDuty, err := domain.VerifyReleaseGate(
		mustValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		domain.OutboundRelease,
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		[]domain.PreconditionReference{domain.DutyPaymentPrecondition},
		domain.GateMet, dutyGateVerifiedAt)
	if err != nil {
		t.Fatalf("construct gate: %v", err)
	}
	if _, has := withDuty.DutyPayment(); has {
		t.Fatal("没挂读数的判断读出了读数")
	}

	attached, err := withDuty.WithDutyPayment(reading)
	if err != nil {
		t.Fatalf("attach reading: %v", err)
	}
	got, has := attached.DutyPayment()
	if !has || got != reading {
		t.Fatalf("读数走样：%+v has=%v", got, has)
	}
	if _, has := withDuty.DutyPayment(); has {
		t.Fatal("WithDutyPayment 改了原值——判断是不可变版本")
	}

	withoutDuty, err := domain.VerifyReleaseGate(
		mustValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		domain.OutboundRelease,
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		[]domain.PreconditionReference{mustValue(t, domain.NewPreconditionReference, "RESTRICTION/released")},
		domain.GateMet, dutyGateVerifiedAt)
	if err != nil {
		t.Fatalf("construct gate: %v", err)
	}
	if _, err := withoutDuty.WithDutyPayment(reading); !errors.Is(err, domain.ErrInvalidGateVerification) {
		t.Fatalf("清单里没有税费付款那一项却挂读数该拒：%v", err)
	}

	blank := reading
	blank.Verification.Version = ""
	if _, err := withDuty.WithDutyPayment(blank); !errors.Is(err, domain.ErrInvalidGateVerification) {
		t.Fatalf("没有核对版本引用的读数该拒：%v", err)
	}
	conflicting := reading
	conflicting.Validity = domain.FundsFactConflicting
	if _, err := withDuty.WithDutyPayment(conflicting); !errors.Is(err, domain.ErrInvalidGateVerification) {
		t.Fatalf("带`冲突`轴的读数不该落进门禁记录（那一格是未决，不入册）：%v", err)
	}
}
