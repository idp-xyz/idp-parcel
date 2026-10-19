package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var admissionAt = time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)

const (
	admissionObject    = "SYN-PCL-01"
	admissionUnit      = "SYN-UNIT-CN-EXPORT-01"
	admissionProcedure = "SYN-PROC-CN-EXPORT"
)

type admissionCase struct {
	t     *testing.T
	input domain.ActionAdmissionInput
}

// newAdmissionCase 起一份同区出发（SHA→SZX）的准入输入，对象不在任何案件与单元里；各用例在其上加料。
func newAdmissionCase(t *testing.T) *admissionCase {
	t.Helper()
	return &admissionCase{t: t, input: domain.ActionAdmissionInput{
		Object:   mustValue(t, domain.NewDeclaredParcelReference, admissionObject),
		Intent:   departure(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"),
		At:       admissionAt,
		Zones:    []domain.NodeZoneReading{zoned(t, "SYN-NODE-SHA-HUB", "SYN-ZONE-CN"), zoned(t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN")},
		JudgedAt: admissionAt.Add(time.Minute),
	}}
}

func (c *admissionCase) crossBorder() *admissionCase {
	c.input.Intent = departure(c.t, "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	c.input.Zones = []domain.NodeZoneReading{zoned(c.t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN"), zoned(c.t, "SYN-NODE-SIN-HUB", "SYN-ZONE-SG")}
	return c
}

func (c *admissionCase) unzonedDestination() *admissionCase {
	c.input.Intent = departure(c.t, "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	c.input.Zones = []domain.NodeZoneReading{zoned(c.t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN"), unzoned(c.t, "SYN-NODE-SIN-HUB")}
	return c
}

func (c *admissionCase) inUnit() *admissionCase {
	c.input.Scope.Units = append(c.input.Scope.Units, domain.UnitScope{
		Scope:     mustValue(c.t, domain.NewDecisionScopeReference, admissionUnit),
		Procedure: mustValue(c.t, domain.NewCustomsProcedureReference, admissionProcedure),
	})
	return c
}

func (c *admissionCase) gate(action domain.GuardedAction, state domain.GateReadingState, conclusion domain.GateConclusion) *admissionCase {
	reading := domain.GateReading{
		Action:     action,
		Scope:      mustValue(c.t, domain.NewDecisionScopeReference, admissionUnit),
		Procedure:  mustValue(c.t, domain.NewCustomsProcedureReference, admissionProcedure),
		State:      state,
		Conclusion: conclusion,
	}
	if state == domain.GateReadingFormed && conclusion != domain.GateNotApplicable {
		reading.Preconditions = []domain.PreconditionReference{mustValue(c.t, domain.NewPreconditionReference, "SYN-PRE-EXPORT-DECL-RELEASED")}
	}
	c.input.Gates = append(c.input.Gates, reading)
	return c
}

func (c *admissionCase) release(source string, kind domain.ReleaseKind, occurredAt time.Time) *admissionCase {
	condition := ""
	if kind == domain.ConditionalRelease {
		condition = "SYN-COND-BONDED"
	}
	release, err := domain.ReceiveReleaseOutcome(kind,
		mustValue(c.t, domain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-CN"),
		mustValue(c.t, domain.NewDecisionScopeReference, admissionUnit), condition, occurredAt)
	if err != nil {
		c.t.Fatalf("构造放行事实：%v", err)
	}
	c.input.Releases = append(c.input.Releases, domain.ReleaseReading{Source: source, Release: release, OccurredAt: occurredAt})
	return c
}

func (c *admissionCase) restrictedOnObject(id string, releasedAt time.Time, constrains ...domain.GuardedAction) *admissionCase {
	built, err := domain.EstablishRestriction(domain.RegulatoryRestrictionSpec{
		ID:          mustValue(c.t, domain.NewRestrictionID, id),
		Decision:    mustValue(c.t, domain.NewRegulatoryDecisionID, "SYN-DECISION-HOLD"),
		Scope:       mustValue(c.t, domain.NewDecisionScopeReference, admissionObject),
		Constrains:  constrains,
		EffectiveAt: admissionAt.Add(-2 * time.Hour),
	})
	if err != nil {
		c.t.Fatalf("构造限制：%v", err)
	}
	if !releasedAt.IsZero() {
		built, err = built.ReleaseByRegulatoryOutcome(mustValue(c.t, domain.NewRegulatoryReleaseReference, "SYN-RELEASE-REF"), releasedAt)
		if err != nil {
			c.t.Fatalf("解除限制：%v", err)
		}
	}
	c.input.Restrictions = append(c.input.Restrictions, built)
	return c
}

func (c *admissionCase) judge() domain.ActionAdmissionJudgement {
	c.t.Helper()
	judgement, err := domain.JudgeActionAdmission(c.input)
	if err != nil {
		c.t.Fatalf("准入判断：%v", err)
	}
	return judgement
}

func findingOf(t *testing.T, judgement domain.ActionAdmissionJudgement, action domain.GuardedAction) domain.ActionAdmissionFinding {
	t.Helper()
	for _, finding := range judgement.Findings() {
		if finding.Action == action {
			return finding
		}
	}
	t.Fatalf("判断里没有 %s：%+v", action, judgement.Findings())
	return domain.ActionAdmissionFinding{}
}

func assertFinding(t *testing.T, finding domain.ActionAdmissionFinding, answer domain.AdmissionAnswer, causes ...domain.AdmissionCause) {
	t.Helper()
	if finding.Answer != answer {
		t.Fatalf("%s 答复 = %s，要 %s（依据 %+v）", finding.Action, finding.Answer, answer, finding.Bases)
	}
	if len(finding.Bases) != len(causes) {
		t.Fatalf("%s 依据 = %+v，要 %v", finding.Action, finding.Bases, causes)
	}
	for index, cause := range causes {
		if finding.Bases[index].Cause != cause {
			t.Fatalf("%s 第 %d 条依据 = %s，要 %s", finding.Action, index, finding.Bases[index].Cause, cause)
		}
	}
}

// 不在本上下文任何案件与单元里的对象：同区出发没有任何东西约束它；跨境那一步跨关务边界必须有案可依，答未决。
func TestObjectOutsideCustomsBooksIsUnguardedWithinAZoneButUndecidedAcrossIt(t *testing.T) {
	domestic := newAdmissionCase(t).judge()
	if domestic.Answer() != domain.AdmissionNotGuarded || len(domestic.Findings()) != 1 {
		t.Fatalf("同区出发 = %s %+v", domestic.Answer(), domestic.Findings())
	}
	assertFinding(t, findingOf(t, domestic, domain.LoadingDeparture), domain.AdmissionNotGuarded, domain.CauseOutsideCustomsBooks)

	crossing := newAdmissionCase(t).crossBorder().judge()
	if crossing.Answer() != domain.AdmissionUndecided {
		t.Fatalf("跨境出发 = %s", crossing.Answer())
	}
	assertFinding(t, findingOf(t, crossing, domain.LoadingDeparture), domain.AdmissionNotGuarded, domain.CauseOutsideCustomsBooks)
	assertFinding(t, findingOf(t, crossing, domain.CrossCustomsMovement), domain.AdmissionUndecided, domain.CauseCustomsScopeUnresolved)
}

// 门禁满足不答放行：要有动作时点前生效的全部放行；部分或附条件放行说不出本件放没放、条件成没成，答未决。
func TestGateMetAdmitsOnlyWithAFullReleaseInForceAtTheActionInstant(t *testing.T) {
	admitted := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateMet).
		release("SYN-SRC-FULL", domain.FullRelease, admissionAt.Add(-time.Hour)).judge()
	assertFinding(t, findingOf(t, admitted, domain.LoadingDeparture), domain.AdmissionAdmitted, domain.CauseReleasedInFull)
	if admitted.Answer() != domain.AdmissionAdmitted {
		t.Fatalf("整次答复 = %s", admitted.Answer())
	}

	late := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateMet).
		release("SYN-SRC-FULL", domain.FullRelease, admissionAt.Add(time.Hour)).judge()
	assertFinding(t, findingOf(t, late, domain.LoadingDeparture), domain.AdmissionBlocked, domain.CauseReleaseNotReceived)

	partial := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateMet).
		release("SYN-SRC-PART", domain.PartialRelease, admissionAt.Add(-time.Hour)).
		release("SYN-SRC-COND", domain.ConditionalRelease, admissionAt.Add(-time.Hour)).judge()
	assertFinding(t, findingOf(t, partial, domain.LoadingDeparture), domain.AdmissionUndecided,
		domain.CauseReleasePartialOnly, domain.CauseReleaseConditionalOnly)
}

func TestGateNotMetBlocksUnlessTheAuthorityAlreadyReleasedInFull(t *testing.T) {
	blocked := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateUnmet).judge()
	finding := findingOf(t, blocked, domain.LoadingDeparture)
	assertFinding(t, finding, domain.AdmissionBlocked, domain.CauseGateUnmet)
	if got := finding.Bases[0].Sources; len(got) != 1 || got[0] != "SYN-PRE-EXPORT-DECL-RELEASED" {
		t.Fatalf("阻断依据要指回前置条件：%v", got)
	}

	conflicting := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateConflicting).judge()
	assertFinding(t, findingOf(t, conflicting, domain.LoadingDeparture), domain.AdmissionBlocked, domain.CauseGateConflicting)

	// 内部门禁说未满足、监管机构却已全部放行：不能用内部结果覆盖外部事实，也不能当没看见。
	discrepant := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateUnmet).
		release("SYN-SRC-FULL", domain.FullRelease, admissionAt.Add(-time.Hour)).judge()
	assertFinding(t, findingOf(t, discrepant, domain.LoadingDeparture), domain.AdmissionUndecided, domain.CauseGateReleaseDiscrepancy)
}

// 外部放行不解内部限制；限制按动作时点判——动作之后才解除的限制，当时仍挡着。
func TestRestrictionInForceAtTheActionInstantBlocksDespiteAFullRelease(t *testing.T) {
	blocked := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateMet).
		release("SYN-SRC-FULL", domain.FullRelease, admissionAt.Add(-time.Hour)).
		restrictedOnObject("SYN-RESTRICTION-01", admissionAt.Add(time.Hour), domain.LoadingDeparture).judge()
	finding := findingOf(t, blocked, domain.LoadingDeparture)
	assertFinding(t, finding, domain.AdmissionBlocked, domain.CauseRestrictionInForce)
	if finding.Bases[0].Scope.String() != admissionObject || finding.Bases[0].Sources[0] != "SYN-RESTRICTION-01" {
		t.Fatalf("限制依据要指名范围与限制：%+v", finding.Bases[0])
	}

	releasedBefore := newAdmissionCase(t).
		restrictedOnObject("SYN-RESTRICTION-01", admissionAt.Add(-time.Hour), domain.LoadingDeparture).judge()
	assertFinding(t, findingOf(t, releasedBefore, domain.LoadingDeparture), domain.AdmissionNotGuarded, domain.CauseOutsideCustomsBooks)

	otherAction := newAdmissionCase(t).
		restrictedOnObject("SYN-RESTRICTION-02", time.Time{}, domain.FinalDelivery).judge()
	assertFinding(t, findingOf(t, otherAction, domain.LoadingDeparture), domain.AdmissionNotGuarded, domain.CauseOutsideCustomsBooks)
}

func TestGateStopsFoldIntoNotConfiguredOrUndecided(t *testing.T) {
	tests := map[string]struct {
		state  domain.GateReadingState
		answer domain.AdmissionAnswer
		cause  domain.AdmissionCause
	}{
		"目录未登记":     {domain.GateReadingCatalogNotRegistered, domain.AdmissionNotConfigured, domain.CauseGateCatalogNotRegistered},
		"税费付款规则未登记": {domain.GateReadingDutyRuleNotRegistered, domain.AdmissionNotConfigured, domain.CauseDutyPaymentRuleNotRegistered},
		"规则旁另登认定":   {domain.GateReadingDutyFindingBesideRule, domain.AdmissionNotConfigured, domain.CauseDutyPaymentFindingBesideRule},
		"没有付款核对":    {domain.GateReadingDutyVerificationAbsent, domain.AdmissionUndecided, domain.CauseDutyVerificationAbsent},
		"付款核对待确认":   {domain.GateReadingDutyVerificationPending, domain.AdmissionUndecided, domain.CauseDutyVerificationPending},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			judgement := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, test.state, domain.GateConclusionInvalid).judge()
			assertFinding(t, findingOf(t, judgement, domain.LoadingDeparture), test.answer, test.cause)
		})
	}

	unguarded := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateNotApplicable).judge()
	assertFinding(t, findingOf(t, unguarded, domain.LoadingDeparture), domain.AdmissionNotGuarded, domain.CauseGateNotApplicable)
}

// 已在案却未成单元：对象受关务管，只是范围还没成形——不是「不受门禁」。
func TestCaseWithoutUnitIsUndecided(t *testing.T) {
	c := newAdmissionCase(t)
	c.input.Scope.CasesWithoutUnit = []domain.CustomsProcedureReference{mustValue(t, domain.NewCustomsProcedureReference, admissionProcedure)}
	assertFinding(t, findingOf(t, c.judge(), domain.LoadingDeparture), domain.AdmissionUndecided, domain.CauseDeclarationUnitNotFormed)
}

// 节点未登记区域时，只有对象带着跨区的门才答未配置；不带跨区门的对象（含不在任何单元里的）这一道不列。
func TestUnregisteredNodeZoneIsNotConfiguredOnlyWhenTheObjectHasACrossZoneGate(t *testing.T) {
	guarded := newAdmissionCase(t).unzonedDestination().inUnit().
		gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateNotApplicable).
		gate(domain.CrossCustomsMovement, domain.GateReadingFormed, domain.GateMet).judge()
	if guarded.CrossZone() != domain.CrossZoneUndetermined || guarded.Answer() != domain.AdmissionNotConfigured {
		t.Fatalf("带跨区门 = %s / %s", guarded.CrossZone(), guarded.Answer())
	}
	finding := findingOf(t, guarded, domain.CrossCustomsMovement)
	assertFinding(t, finding, domain.AdmissionNotConfigured, domain.CauseNodeZoneNotRegistered)
	if finding.Bases[0].Sources[0] != "SYN-NODE-SIN-HUB" {
		t.Fatalf("未配置要指名未登记的节点：%v", finding.Bases[0].Sources)
	}

	noCrossCatalog := newAdmissionCase(t).unzonedDestination().inUnit().
		gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateNotApplicable).
		gate(domain.CrossCustomsMovement, domain.GateReadingCatalogNotRegistered, domain.GateConclusionInvalid).judge()
	if len(noCrossCatalog.Findings()) != 1 || noCrossCatalog.Answer() != domain.AdmissionNotGuarded {
		t.Fatalf("没登跨区目录：%s %+v", noCrossCatalog.Answer(), noCrossCatalog.Findings())
	}

	outside := newAdmissionCase(t).unzonedDestination().judge()
	if len(outside.Findings()) != 1 || outside.Answer() != domain.AdmissionNotGuarded {
		t.Fatalf("不在册对象：%s %+v", outside.Answer(), outside.Findings())
	}

	restricted := newAdmissionCase(t).unzonedDestination().
		restrictedOnObject("SYN-RESTRICTION-03", time.Time{}, domain.CrossCustomsMovement).judge()
	assertFinding(t, findingOf(t, restricted, domain.CrossCustomsMovement), domain.AdmissionNotConfigured, domain.CauseNodeZoneNotRegistered)
}

// 整次答复按压制次序取各动作里最重的一格。
func TestOverallAnswerTakesTheHeaviestActionAnswer(t *testing.T) {
	judgement := newAdmissionCase(t).crossBorder().inUnit().
		gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateNotApplicable).
		gate(domain.CrossCustomsMovement, domain.GateReadingFormed, domain.GateUnmet).judge()
	assertFinding(t, findingOf(t, judgement, domain.LoadingDeparture), domain.AdmissionNotGuarded, domain.CauseGateNotApplicable)
	assertFinding(t, findingOf(t, judgement, domain.CrossCustomsMovement), domain.AdmissionBlocked, domain.CauseGateUnmet)
	if judgement.Answer() != domain.AdmissionBlocked {
		t.Fatalf("整次答复 = %s", judgement.Answer())
	}
}

// 编排漏读了某单元某动作的门禁是编程错误，不折成任何业务格。
func TestMissingGateReadingIsAProgrammingError(t *testing.T) {
	c := newAdmissionCase(t).inUnit()
	if _, err := domain.JudgeActionAdmission(c.input); !errors.Is(err, domain.ErrGateReadingMissing) {
		t.Fatalf("缺门禁读数：err = %v", err)
	}
	c = newAdmissionCase(t)
	c.input.At = time.Time{}
	if _, err := domain.JudgeActionAdmission(c.input); !errors.Is(err, domain.ErrInvalidAdmissionInput) {
		t.Fatalf("缺动作时点：err = %v", err)
	}
}
