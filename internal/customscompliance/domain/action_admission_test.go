package domain_test

import (
	"errors"
	"slices"
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

// unzonedBothEnds 起讫两端在动作时点都没登区域；租户登没登过别的节点由用例另给（零值即一条都没登）。
func (c *admissionCase) unzonedBothEnds() *admissionCase {
	c.input.Intent = departure(c.t, "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	c.input.Zones = []domain.NodeZoneReading{unzoned(c.t, "SYN-NODE-SZX-GATE"), unzoned(c.t, "SYN-NODE-SIN-HUB")}
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
// Covers: AT-CC-413、AT-CC-414（领域半边）。
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
// Covers: AT-CC-415、AT-CC-416 的折法半边——钉在装载出发那一道上，跨区那一道走同一份折法；跨区一道的整条在真库用例。
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
// Covers: AT-CC-418（折法半边，钉在装载出发那一道上）、AT-CC-419。
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

// Covers: AT-CC-420（领域半边）。
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

// 节点未登记区域时，对象带着跨区的门才答未配置，单元没登跨区目录的不列——这两格租户登没登过区域都一样。租户在
// 动作时点一个节点都没登（起讫两端都没登、另读的那一位为零值）时，不在任何单元里的对象这一道也不列。
// Covers: AT-CC-421（领域半边）、AT-CC-422（领域半边）、AT-CC-427（领域半边）——纯境内租户的不在册对象整次答复为不受门禁。
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

	outside := newAdmissionCase(t).unzonedBothEnds().judge()
	if outside.CrossZone() != domain.CrossZoneUndetermined || len(outside.Findings()) != 1 || outside.Answer() != domain.AdmissionNotGuarded {
		t.Fatalf("纯境内租户的不在册对象：%s / %s %+v", outside.CrossZone(), outside.Answer(), outside.Findings())
	}

	restricted := newAdmissionCase(t).unzonedDestination().
		restrictedOnObject("SYN-RESTRICTION-03", time.Time{}, domain.CrossCustomsMovement).judge()
	assertFinding(t, findingOf(t, restricted, domain.CrossCustomsMovement), domain.AdmissionNotConfigured, domain.CauseNodeZoneNotRegistered)
}

// 租户在动作时点已有节点登了区域（ADR-0300）：起讫有一端未登、对象不在任何案件与单元里时，跨区那一道答未配置并指名
// 未登的节点，整次答复未配置；装载出发照旧不受门禁。起讫另一端已登就说明租户登过，另读的那一位是零值也照此答；两端
// 都没登时只看那一位，答未配置就两个节点都指名。声明了去向的交出同一判法。
// Covers: AT-CC-426（领域半边）。
func TestAZonedTenantsUnbookedObjectAnswersNotConfiguredNamingTheUnregisteredNode(t *testing.T) {
	zonedTenant := func(c *admissionCase) *admissionCase {
		c.input.TenantZonesInForce = true
		return c
	}
	expectNamed := func(t *testing.T, judgement domain.ActionAdmissionJudgement, nodes ...string) {
		t.Helper()
		if judgement.CrossZone() != domain.CrossZoneUndetermined || judgement.Answer() != domain.AdmissionNotConfigured {
			t.Fatalf("跨区待定 = %s / %s，要待定 / 未配置", judgement.CrossZone(), judgement.Answer())
		}
		cross := findingOf(t, judgement, domain.CrossCustomsMovement)
		assertFinding(t, cross, domain.AdmissionNotConfigured, domain.CauseNodeZoneNotRegistered)
		if !slices.Equal(cross.Bases[0].Sources, nodes) {
			t.Fatalf("未配置要指名未登的节点：%v，要 %v", cross.Bases[0].Sources, nodes)
		}
	}

	departing := zonedTenant(newAdmissionCase(t).unzonedDestination()).judge()
	expectNamed(t, departing, "SYN-NODE-SIN-HUB")
	assertFinding(t, findingOf(t, departing, domain.LoadingDeparture), domain.AdmissionNotGuarded, domain.CauseOutsideCustomsBooks)
	expectNamed(t, newAdmissionCase(t).unzonedDestination().judge(), "SYN-NODE-SIN-HUB")

	expectNamed(t, zonedTenant(newAdmissionCase(t).unzonedBothEnds()).judge(), "SYN-NODE-SIN-HUB", "SYN-NODE-SZX-GATE")

	handingOver := zonedTenant(newAdmissionCase(t))
	intent, err := domain.NewHandoverIntent(node(t, "SYN-NODE-SZX-GATE"), node(t, "SYN-NODE-SIN-HUB"))
	if err != nil {
		t.Fatalf("构造交出意图：%v", err)
	}
	handingOver.input.Intent = intent
	handingOver.input.Zones = []domain.NodeZoneReading{zoned(t, "SYN-NODE-SZX-GATE", "SYN-ZONE-CN"), unzoned(t, "SYN-NODE-SIN-HUB")}
	handedOver := handingOver.judge()
	expectNamed(t, handedOver, "SYN-NODE-SIN-HUB")
	assertFinding(t, findingOf(t, handedOver, domain.OutboundRelease), domain.AdmissionNotGuarded, domain.CauseOutsideCustomsBooks)
}

// 已登区域的租户，ADR-0300 那一支只落在不在册的对象上：对象在册而单元没登跨区目录，这一道照旧不列；在案而未成单元，
// 这一道照旧不列，整次答复已是未决；两端都登了区域时这一格与它无关——跨区答未决、同区只适用装载出发。
// Covers: AT-CC-422（领域半边）。
func TestAZonedTenantStillLeavesTheCrossZoneLineOutForBookedObjects(t *testing.T) {
	noCrossCatalog := newAdmissionCase(t).unzonedDestination().inUnit().
		gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateNotApplicable).
		gate(domain.CrossCustomsMovement, domain.GateReadingCatalogNotRegistered, domain.GateConclusionInvalid)
	noCrossCatalog.input.TenantZonesInForce = true
	if judgement := noCrossCatalog.judge(); len(judgement.Findings()) != 1 || judgement.Answer() != domain.AdmissionNotGuarded {
		t.Fatalf("在册、没登跨区目录：%s %+v", judgement.Answer(), judgement.Findings())
	}

	caseWithoutUnit := newAdmissionCase(t).unzonedDestination()
	caseWithoutUnit.input.TenantZonesInForce = true
	caseWithoutUnit.input.Scope.CasesWithoutUnit = []domain.CustomsProcedureReference{mustValue(t, domain.NewCustomsProcedureReference, admissionProcedure)}
	judgement := caseWithoutUnit.judge()
	if len(judgement.Findings()) != 1 || judgement.Answer() != domain.AdmissionUndecided {
		t.Fatalf("在案未成单元：%s %+v", judgement.Answer(), judgement.Findings())
	}
	assertFinding(t, findingOf(t, judgement, domain.LoadingDeparture), domain.AdmissionUndecided, domain.CauseDeclarationUnitNotFormed)

	crossing := newAdmissionCase(t).crossBorder()
	crossing.input.TenantZonesInForce = true
	assertFinding(t, findingOf(t, crossing.judge(), domain.CrossCustomsMovement), domain.AdmissionUndecided, domain.CauseCustomsScopeUnresolved)
	domestic := newAdmissionCase(t)
	domestic.input.TenantZonesInForce = true
	if judgement := domestic.judge(); len(judgement.Findings()) != 1 || judgement.Answer() != domain.AdmissionNotGuarded {
		t.Fatalf("同区出发：%s %+v", judgement.Answer(), judgement.Findings())
	}
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

// internalOnObject 在对象自身上加一条内部合规限制（责任来源 SYN-SOURCE-TRADE-COMPLIANCE），覆盖项都按给定的开始时适用性；
// decisions 在限制形成后依次追加。
func (c *admissionCase) internalOnObject(id string, standing domain.CoverageStanding, constrains []domain.GuardedAction, decisions ...func(domain.InternalRestriction) domain.InternalRestriction) *admissionCase {
	c.t.Helper()
	spec := domain.InternalRestrictionSpec{
		ID:        mustValue(c.t, domain.NewInternalRestrictionID, id),
		Source:    mustValue(c.t, domain.NewRestrictionSource, "SYN-SOURCE-TRADE-COMPLIANCE"),
		Basis:     mustValue(c.t, domain.NewRestrictionBasis, "SYN-RULE-DUAL-USE@v3"),
		Mode:      domain.ManualDecision,
		DecidedBy: mustValue(c.t, domain.NewRestrictionDecider, "SYN-ROLE-COMPLIANCE-REVIEWER"),
		Condition: mustValue(c.t, domain.NewRestrictionReleaseCondition, "SYN-COND-LICENCE-ON-FILE"),
		StartedAt: admissionAt.Add(-2 * time.Hour),
	}
	for _, action := range constrains {
		spec.Coverage = append(spec.Coverage, domain.CoverageSpec{
			Scope:    mustValue(c.t, domain.NewDecisionScopeReference, admissionObject),
			Action:   action,
			Standing: standing,
		})
	}
	built, err := domain.EstablishInternalRestriction(spec)
	if err != nil {
		c.t.Fatalf("构造内部合规限制：%v", err)
	}
	for _, apply := range decisions {
		built = apply(built)
	}
	c.input.InternalRestrictions = append(c.input.InternalRestrictions, built)
	return c
}

// sourceDecides 是限制责任来源在给定时刻对对象上给定动作形成的确认或解除决定。
func (c *admissionCase) sourceDecides(id string, kind domain.RestrictionDecisionKind, at time.Time, action domain.GuardedAction) func(domain.InternalRestriction) domain.InternalRestriction {
	return func(restriction domain.InternalRestriction) domain.InternalRestriction {
		c.t.Helper()
		made, err := domain.NewRestrictionDecision(domain.RestrictionDecisionSpec{
			ID:        mustValue(c.t, domain.NewRestrictionDecisionID, id),
			Kind:      kind,
			Source:    mustValue(c.t, domain.NewRestrictionSource, "SYN-SOURCE-TRADE-COMPLIANCE"),
			Covers:    []domain.CoverageKey{{Scope: mustValue(c.t, domain.NewDecisionScopeReference, admissionObject), Action: action}},
			Evidence:  mustValue(c.t, domain.NewRestrictionEvidence, "SYN-EVIDENCE-LICENCE-4711"),
			DecidedBy: mustValue(c.t, domain.NewRestrictionDecider, "SYN-ROLE-COMPLIANCE-REVIEWER"),
			Authority: mustValue(c.t, domain.NewRestrictionAuthority, "SYN-DELEGATION-2026-07"),
			At:        at,
		})
		if err != nil {
			c.t.Fatalf("构造决定：%v", err)
		}
		decided, err := restriction.Decide(made)
		if err != nil {
			c.t.Fatalf("形成决定：%v", err)
		}
		return decided
	}
}

// 门禁满足、已有全部放行，内部合规限制的覆盖项在动作时点待确认：该动作答未决并指名那条限制——它不是一条有效限制（阻断要指得回
// 一条有效限制），也不能读成无内部限制；确认决定晚于动作时点的动作仍读到待确认，确认生效后才答阻断。空册不产生依据。
// Covers: AT-CC-449（领域半边）。
func TestAnInternalRestrictionUnconfirmedAtTheActionInstantMakesTheActionUndecided(t *testing.T) {
	met := func() *admissionCase {
		return newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateMet).
			release("SYN-SRC-FULL", domain.FullRelease, admissionAt.Add(-time.Hour))
	}

	pending := met().internalOnObject("SYN-IR-1", domain.CoverageUnconfirmed, []domain.GuardedAction{domain.LoadingDeparture}).judge()
	finding := findingOf(t, pending, domain.LoadingDeparture)
	assertFinding(t, finding, domain.AdmissionUndecided, domain.CauseInternalRestrictionUnconfirmed)
	if finding.Bases[0].Scope.String() != admissionObject || len(finding.Bases[0].Sources) != 1 || finding.Bases[0].Sources[0] != "SYN-IR-1" {
		t.Fatalf("待确认依据要指名范围与那条限制：%+v", finding.Bases[0])
	}
	if pending.Answer() != domain.AdmissionUndecided {
		t.Fatalf("整次答复 = %s", pending.Answer())
	}

	c := met()
	confirmedAfter := c.internalOnObject("SYN-IR-1", domain.CoverageUnconfirmed, []domain.GuardedAction{domain.LoadingDeparture},
		c.sourceDecides("D-1", domain.ConfirmCoverage, admissionAt.Add(time.Minute), domain.LoadingDeparture)).judge()
	assertFinding(t, findingOf(t, confirmedAfter, domain.LoadingDeparture), domain.AdmissionUndecided, domain.CauseInternalRestrictionUnconfirmed)

	c = met()
	confirmedBefore := c.internalOnObject("SYN-IR-1", domain.CoverageUnconfirmed, []domain.GuardedAction{domain.LoadingDeparture},
		c.sourceDecides("D-1", domain.ConfirmCoverage, admissionAt.Add(-time.Minute), domain.LoadingDeparture)).judge()
	assertFinding(t, findingOf(t, confirmedBefore, domain.LoadingDeparture), domain.AdmissionBlocked, domain.CauseInternalRestrictionInForce)

	elsewhere := met().internalOnObject("SYN-IR-1", domain.CoverageUnconfirmed, []domain.GuardedAction{domain.FinalDelivery}).judge()
	assertFinding(t, findingOf(t, elsewhere, domain.LoadingDeparture), domain.AdmissionAdmitted, domain.CauseReleasedInFull)

	emptyRegister := met().judge()
	assertFinding(t, findingOf(t, emptyRegister, domain.LoadingDeparture), domain.AdmissionAdmitted, domain.CauseReleasedInFull)
}

// 冲突与待确认同样答未决，但依据另成一种（续办不同：裁冲突，还是去确认来源）；同一动作上另有一条有效的内部合规限制时答阻断，
// 阻断压过未决，列着的只有挡住它的那一条。
// Covers: AT-CC-450（领域半边）。
func TestAConflictingInternalRestrictionIsUndecidedUnderItsOwnCause(t *testing.T) {
	conflict := newAdmissionCase(t).internalOnObject("SYN-IR-1", domain.CoverageConflicting, []domain.GuardedAction{domain.LoadingDeparture}).judge()
	assertFinding(t, findingOf(t, conflict, domain.LoadingDeparture), domain.AdmissionUndecided, domain.CauseInternalRestrictionConflicting)
	if conflict.Answer() != domain.AdmissionUndecided {
		t.Fatalf("整次答复 = %s", conflict.Answer())
	}

	both := newAdmissionCase(t).
		internalOnObject("SYN-IR-1", domain.CoverageConflicting, []domain.GuardedAction{domain.LoadingDeparture}).
		internalOnObject("SYN-IR-2", domain.CoverageUnconfirmed, []domain.GuardedAction{domain.LoadingDeparture}).judge()
	finding := findingOf(t, both, domain.LoadingDeparture)
	assertFinding(t, finding, domain.AdmissionUndecided, domain.CauseInternalRestrictionConflicting, domain.CauseInternalRestrictionUnconfirmed)
	if finding.Bases[0].Sources[0] != "SYN-IR-1" || finding.Bases[1].Sources[0] != "SYN-IR-2" {
		t.Fatalf("两种依据各指名各的限制：%+v", finding.Bases)
	}

	blocked := newAdmissionCase(t).
		internalOnObject("SYN-IR-1", domain.CoverageConflicting, []domain.GuardedAction{domain.LoadingDeparture}).
		internalOnObject("SYN-IR-3", domain.CoverageInForce, []domain.GuardedAction{domain.LoadingDeparture}).judge()
	finding = findingOf(t, blocked, domain.LoadingDeparture)
	assertFinding(t, finding, domain.AdmissionBlocked, domain.CauseInternalRestrictionInForce)
	if finding.Bases[0].Sources[0] != "SYN-IR-3" {
		t.Fatalf("阻断只列挡住它的那条：%+v", finding.Bases)
	}
}

// 两种限制各按自己的依据解除，互不相干：同一动作上监管限制与内部合规限制都有效时两条依据都列；只解除其中一种仍阻断，两种都
// 解除才不再有限制依据；外部放行不解除任何一种。
// Covers: AT-CC-451（领域半边）。
func TestEachKindOfRestrictionBlocksUntilItsOwnBasisReleasesIt(t *testing.T) {
	build := func(regulatoryReleasedAt time.Time, internalReleased bool) *admissionCase {
		c := newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateMet).
			release("SYN-SRC-FULL", domain.FullRelease, admissionAt.Add(-time.Hour)).
			restrictedOnObject("SYN-RESTRICTION-01", regulatoryReleasedAt, domain.LoadingDeparture)
		var decisions []func(domain.InternalRestriction) domain.InternalRestriction
		if internalReleased {
			decisions = append(decisions, c.sourceDecides("D-1", domain.ReleaseCoverage, admissionAt.Add(-30*time.Minute), domain.LoadingDeparture))
		}
		return c.internalOnObject("SYN-IR-1", domain.CoverageInForce, []domain.GuardedAction{domain.LoadingDeparture}, decisions...)
	}
	released := admissionAt.Add(-45 * time.Minute)

	both := findingOf(t, build(time.Time{}, false).judge(), domain.LoadingDeparture)
	assertFinding(t, both, domain.AdmissionBlocked, domain.CauseRestrictionInForce, domain.CauseInternalRestrictionInForce)
	if both.Bases[0].Sources[0] != "SYN-RESTRICTION-01" || both.Bases[1].Sources[0] != "SYN-IR-1" {
		t.Fatalf("两种限制的依据各指名各的限制：%+v", both.Bases)
	}

	onlyRegulatoryReleased := findingOf(t, build(released, false).judge(), domain.LoadingDeparture)
	assertFinding(t, onlyRegulatoryReleased, domain.AdmissionBlocked, domain.CauseInternalRestrictionInForce)

	onlyInternalReleased := findingOf(t, build(time.Time{}, true).judge(), domain.LoadingDeparture)
	assertFinding(t, onlyInternalReleased, domain.AdmissionBlocked, domain.CauseRestrictionInForce)

	neither := findingOf(t, build(released, true).judge(), domain.LoadingDeparture)
	assertFinding(t, neither, domain.AdmissionAdmitted, domain.CauseReleasedInFull)
}

// 内部合规限制约束跨区移动、而终点节点未登记区域：同监管限制一样说明对象带着跨区的门，这一道答未配置并指名未登记的节点。
// Covers: AT-CC-421（内部合规限制那一支的领域半边）。
func TestAnInternalRestrictionOnTheCrossZoneLineMakesAnUnzonedDestinationNotConfigured(t *testing.T) {
	restricted := newAdmissionCase(t).unzonedDestination().
		internalOnObject("SYN-IR-1", domain.CoverageUnconfirmed, []domain.GuardedAction{domain.CrossCustomsMovement}).judge()
	assertFinding(t, findingOf(t, restricted, domain.CrossCustomsMovement), domain.AdmissionNotConfigured, domain.CauseNodeZoneNotRegistered)
}

// 判断输入里的内部合规限制是零值（编排漏了构造）是编程错误，不折成任何业务格。
func TestAnUnconstructedInternalRestrictionIsAProgrammingError(t *testing.T) {
	c := newAdmissionCase(t)
	c.input.InternalRestrictions = []domain.InternalRestriction{{}}
	if _, err := domain.JudgeActionAdmission(c.input); !errors.Is(err, domain.ErrInvalidAdmissionInput) {
		t.Fatalf("零值内部合规限制：err = %v", err)
	}
}
