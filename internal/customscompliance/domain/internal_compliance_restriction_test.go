package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var internalStartedAt = time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

type coverage struct {
	scope    string
	action   domain.GuardedAction
	standing domain.CoverageStanding
}

func binding(scope string, action domain.GuardedAction) coverage {
	return coverage{scope, action, domain.CoverageInForce}
}

func internalSpec(t *testing.T, id, source string, items ...coverage) domain.InternalRestrictionSpec {
	t.Helper()
	spec := domain.InternalRestrictionSpec{
		ID:        mustValue(t, domain.NewInternalRestrictionID, id),
		Source:    mustValue(t, domain.NewRestrictionSource, source),
		Basis:     mustValue(t, domain.NewRestrictionBasis, "SYN-RULE-CN-DUAL-USE@v3"),
		Mode:      domain.RuleBasedDecision,
		DecidedBy: mustValue(t, domain.NewRestrictionDecider, "SYN-ROLE-COMPLIANCE-REVIEWER"),
		Condition: mustValue(t, domain.NewRestrictionReleaseCondition, "SYN-COND-LICENCE-ON-FILE"),
		StartedAt: internalStartedAt,
	}
	for _, item := range items {
		spec.Coverage = append(spec.Coverage, domain.CoverageSpec{
			Scope:    mustValue(t, domain.NewDecisionScopeReference, item.scope),
			Action:   item.action,
			Standing: item.standing,
		})
	}
	return spec
}

func internalRestriction(t *testing.T, id, source string, items ...coverage) domain.InternalRestriction {
	t.Helper()
	built, err := domain.EstablishInternalRestriction(internalSpec(t, id, source, items...))
	if err != nil {
		t.Fatalf("establish internal restriction %q: %v", id, err)
	}
	return built
}

func key(t *testing.T, scope string, action domain.GuardedAction) domain.CoverageKey {
	t.Helper()
	return domain.CoverageKey{Scope: mustValue(t, domain.NewDecisionScopeReference, scope), Action: action}
}

func decisionSpec(t *testing.T, id string, kind domain.RestrictionDecisionKind, source string, at time.Time, covers ...domain.CoverageKey) domain.RestrictionDecisionSpec {
	t.Helper()
	return domain.RestrictionDecisionSpec{
		ID:        mustValue(t, domain.NewRestrictionDecisionID, id),
		Kind:      kind,
		Source:    mustValue(t, domain.NewRestrictionSource, source),
		Covers:    covers,
		Evidence:  mustValue(t, domain.NewRestrictionEvidence, "SYN-EVIDENCE-LICENCE-4711"),
		DecidedBy: mustValue(t, domain.NewRestrictionDecider, "SYN-ROLE-COMPLIANCE-REVIEWER"),
		Authority: mustValue(t, domain.NewRestrictionAuthority, "SYN-DELEGATION-2026-07"),
		At:        at,
	}
}

func decide(t *testing.T, restriction domain.InternalRestriction, id string, kind domain.RestrictionDecisionKind, source string, at time.Time, covers ...domain.CoverageKey) domain.InternalRestriction {
	t.Helper()
	made, err := domain.NewRestrictionDecision(decisionSpec(t, id, kind, source, at, covers...))
	if err != nil {
		t.Fatalf("decision %q: %v", id, err)
	}
	decided, err := restriction.Decide(made)
	if err != nil {
		t.Fatalf("decide %q: %v", id, err)
	}
	return decided
}

func applicabilityOf(t *testing.T, action domain.GuardedAction, scope string, at time.Time, restrictions ...domain.InternalRestriction) domain.InternalApplicability {
	t.Helper()
	judged, err := domain.JudgeInternalApplicability(action, mustValue(t, domain.NewDecisionScopeReference, scope), restrictions, at)
	if err != nil {
		t.Fatalf("judge applicability: %v", err)
	}
	return judged
}

func ids(list []domain.InternalRestrictionID) []string {
	out := make([]string, 0, len(list))
	for _, id := range list {
		out = append(out, id.String())
	}
	return out
}

// Covers: AT-CC-339（形成限制：责任来源、依据、决定方式、实际决定方、解除条件、开始时间与逐项覆盖项）、AT-CC-340（缺责任来源、依据、
// 范围或受限动作的请求不形成占位限制）。
func TestAnInternalRestrictionNeedsASourceABasisAConditionAndExplicitCoverage(t *testing.T) {
	spec := internalSpec(t, "SYN-IR-1", "SYN-SOURCE-TRADE-COMPLIANCE",
		binding("SYN-PCL-1", domain.OutboundRelease),
		coverage{"SYN-PCL-1", domain.LoadingDeparture, domain.CoverageUnconfirmed})
	built, err := domain.EstablishInternalRestriction(spec)
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	if built.ID().String() != "SYN-IR-1" || built.Source().String() != "SYN-SOURCE-TRADE-COMPLIANCE" ||
		built.Basis().String() != "SYN-RULE-CN-DUAL-USE@v3" || built.Mode() != domain.RuleBasedDecision ||
		built.DecidedBy().String() != "SYN-ROLE-COMPLIANCE-REVIEWER" ||
		built.Condition().String() != "SYN-COND-LICENCE-ON-FILE" || !built.StartedAt().Equal(internalStartedAt) {
		t.Fatalf("限制头没原样带出：%+v", built)
	}
	items := built.Coverage()
	if len(items) != 2 || items[0].Action != domain.OutboundRelease || items[0].Standing != domain.CoverageInForce ||
		items[1].Action != domain.LoadingDeparture || items[1].Standing != domain.CoverageUnconfirmed {
		t.Fatalf("覆盖项要逐项原样保存：%+v", items)
	}
	items[0].Action = domain.FinalDelivery
	if built.Coverage()[0].Action != domain.OutboundRelease {
		t.Fatal("读回的覆盖项切片改得动限制")
	}

	mutate := map[string]func(*domain.InternalRestrictionSpec){
		"缺限制标识":  func(s *domain.InternalRestrictionSpec) { s.ID = domain.InternalRestrictionID{} },
		"缺责任来源":  func(s *domain.InternalRestrictionSpec) { s.Source = domain.RestrictionSource{} },
		"缺依据":    func(s *domain.InternalRestrictionSpec) { s.Basis = domain.RestrictionBasis{} },
		"缺实际决定方": func(s *domain.InternalRestrictionSpec) { s.DecidedBy = domain.RestrictionDecider{} },
		"缺解除条件":  func(s *domain.InternalRestrictionSpec) { s.Condition = domain.RestrictionReleaseCondition{} },
		"缺决定方式":  func(s *domain.InternalRestrictionSpec) { s.Mode = domain.DecisionModeInvalid },
		"缺开始时间":  func(s *domain.InternalRestrictionSpec) { s.StartedAt = time.Time{} },
		"没有覆盖项（不造全局冻结占位）": func(s *domain.InternalRestrictionSpec) { s.Coverage = nil },
		"覆盖项缺范围":          func(s *domain.InternalRestrictionSpec) { s.Coverage[0].Scope = domain.DecisionScopeReference{} },
		"覆盖项缺受限动作": func(s *domain.InternalRestrictionSpec) {
			s.Coverage[0].Action = domain.GuardedActionInvalid
		},
		"覆盖项开始时适用性在词表外": func(s *domain.InternalRestrictionSpec) { s.Coverage[0].Standing = domain.CoverageStandingInvalid },
		"同一范围同一动作的覆盖项重复": func(s *domain.InternalRestrictionSpec) {
			s.Coverage = append(s.Coverage, s.Coverage[0])
		},
	}
	for name, change := range mutate {
		broken := internalSpec(t, "SYN-IR-1", "SYN-SOURCE-TRADE-COMPLIANCE", binding("SYN-PCL-1", domain.OutboundRelease))
		change(&broken)
		if _, err := domain.EstablishInternalRestriction(broken); !errors.Is(err, domain.ErrInvalidInternalRestriction) {
			t.Errorf("%s：err = %v，要 ErrInvalidInternalRestriction", name, err)
		}
	}
}

// Covers: AT-CC-352（非原限制责任来源请求解除有效限制不成立）、AT-CC-353（责任来源带证据、权限、生效时间形成解除决定）、
// AT-CC-351（管理员、商业批准、异常关闭、外部放行在这里没有对应的构造入口——决定只认限制责任来源这一个专名类型）。
func TestOnlyTheResponsibleSourceDecidesOnARestriction(t *testing.T) {
	original := internalRestriction(t, "SYN-IR-1", "SYN-SOURCE-TRADE-COMPLIANCE", binding("SYN-PCL-1", domain.OutboundRelease))
	at := internalStartedAt.Add(time.Hour)

	foreign, err := domain.NewRestrictionDecision(decisionSpec(t, "D-1", domain.ReleaseCoverage, "SYN-SOURCE-SALES-MANAGER", at,
		key(t, "SYN-PCL-1", domain.OutboundRelease)))
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	if _, err := original.Decide(foreign); !errors.Is(err, domain.ErrNotTheResponsibleSource) {
		t.Fatalf("非责任来源的解除 err = %v，要 ErrNotTheResponsibleSource", err)
	}
	foreignConfirm, err := domain.NewRestrictionDecision(decisionSpec(t, "D-2", domain.ConfirmCoverage, "SYN-SOURCE-SALES-MANAGER", at,
		key(t, "SYN-PCL-1", domain.OutboundRelease)))
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	if _, err := original.Decide(foreignConfirm); !errors.Is(err, domain.ErrNotTheResponsibleSource) {
		t.Fatalf("非责任来源的确认 err = %v，要 ErrNotTheResponsibleSource", err)
	}
	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", at, original); got.Applicability() != domain.RestrictionInForce {
		t.Fatalf("被拒的解除改动了限制：%v", got.Applicability())
	}

	released := decide(t, original, "D-3", domain.ReleaseCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", at, key(t, "SYN-PCL-1", domain.OutboundRelease))
	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", at, released); got.Applicability() != domain.RestrictionNoneRecorded {
		t.Fatalf("责任来源解除后：%v", got.Applicability())
	}
	if len(original.Decisions()) != 0 || len(released.Decisions()) != 1 {
		t.Fatal("Decide 要返新值、不改旧值")
	}
	recorded := released.Decisions()[0]
	if recorded.Evidence().String() != "SYN-EVIDENCE-LICENCE-4711" || recorded.Authority().String() != "SYN-DELEGATION-2026-07" ||
		recorded.DecidedBy().String() != "SYN-ROLE-COMPLIANCE-REVIEWER" || !recorded.At().Equal(at) ||
		recorded.Source().String() != "SYN-SOURCE-TRADE-COMPLIANCE" || recorded.Kind() != domain.ReleaseCoverage {
		t.Fatalf("解除决定没带齐证据、权限、决定方、来源与生效时间：%+v", recorded)
	}

	blankAuthority := decisionSpec(t, "D-4", domain.ReleaseCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", at, key(t, "SYN-PCL-1", domain.OutboundRelease))
	blankAuthority.Authority = domain.RestrictionAuthority{}
	if _, err := domain.NewRestrictionDecision(blankAuthority); !errors.Is(err, domain.ErrInvalidRestrictionDecision) {
		t.Errorf("缺授权依据 err = %v", err)
	}
	blankEvidence := decisionSpec(t, "D-5", domain.ReleaseCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", at, key(t, "SYN-PCL-1", domain.OutboundRelease))
	blankEvidence.Evidence = domain.RestrictionEvidence{}
	if _, err := domain.NewRestrictionDecision(blankEvidence); !errors.Is(err, domain.ErrConditionNotEvidenced) {
		t.Errorf("缺证据 err = %v，要 ErrConditionNotEvidenced", err)
	}
}

// Covers: AT-CC-355（覆盖 R1 与 R2，只解除 R1：R2 继续有效）、AT-CC-356（对全部当前覆盖项解除即全部解除）、AT-CC-357（只解除 A1，
// 同一对象 A2 仍被覆盖）、AT-CC-343（P2 与 A2 不在覆盖里就不受约束）、AT-CC-344（只阻断明确受限动作）。
func TestReleaseWorksCoverageItemByItem(t *testing.T) {
	covering := internalRestriction(t, "SYN-IR-1", "SYN-SOURCE-TRADE-COMPLIANCE",
		binding("SYN-R1", domain.OutboundRelease),
		binding("SYN-R2", domain.OutboundRelease),
		binding("SYN-R1", domain.LoadingDeparture))
	at := internalStartedAt.Add(2 * time.Hour)

	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-P2", at, covering); got.Applicability() != domain.RestrictionNoneRecorded {
		t.Fatalf("没覆盖的对象被约束：%v", got.Applicability())
	}
	if got := applicabilityOf(t, domain.FinalDelivery, "SYN-R1", at, covering); got.Applicability() != domain.RestrictionNoneRecorded {
		t.Fatalf("没覆盖的动作被约束：%v", got.Applicability())
	}

	partial := decide(t, covering, "D-1", domain.ReleaseCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", at, key(t, "SYN-R1", domain.OutboundRelease))
	for scope, action := range map[string]domain.GuardedAction{"SYN-R2": domain.OutboundRelease, "SYN-R1": domain.LoadingDeparture} {
		if got := applicabilityOf(t, action, scope, at, partial); got.Applicability() != domain.RestrictionInForce {
			t.Errorf("部分解除之后 %s/%s 应仍有效，实得 %v", scope, action, got.Applicability())
		}
	}
	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-R1", at, partial); got.Applicability() != domain.RestrictionNoneRecorded {
		t.Fatalf("被解除的覆盖项仍约束：%v", got.Applicability())
	}

	whole := decide(t, partial, "D-2", domain.ReleaseCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", at.Add(time.Hour),
		key(t, "SYN-R2", domain.OutboundRelease), key(t, "SYN-R1", domain.LoadingDeparture))
	for scope, action := range map[string]domain.GuardedAction{"SYN-R2": domain.OutboundRelease, "SYN-R1": domain.LoadingDeparture} {
		if got := applicabilityOf(t, action, scope, at.Add(time.Hour), whole); got.Applicability() != domain.RestrictionNoneRecorded {
			t.Errorf("全部解除之后 %s/%s 仍约束：%v", scope, action, got.Applicability())
		}
	}
	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-R2", at, whole); got.Applicability() != domain.RestrictionInForce {
		t.Fatalf("解除之前的动作时点要仍读到有效（按动作时点判）：%v", got.Applicability())
	}
}

// Covers: AT-CC-452（待确认或冲突的覆盖项只经限制责任来源的确认决定成为有效，不回溯）、AT-CC-342（待确认不读成无内部限制、不读成已解除）。
func TestUnconfirmedAndConflictingCoverageTurnInForceOnlyByTheSourcesConfirmation(t *testing.T) {
	pending := internalRestriction(t, "SYN-IR-1", "SYN-SOURCE-TRADE-COMPLIANCE",
		coverage{"SYN-PCL-1", domain.OutboundRelease, domain.CoverageUnconfirmed},
		coverage{"SYN-PCL-1", domain.LoadingDeparture, domain.CoverageConflicting},
		binding("SYN-PCL-1", domain.FinalDelivery))
	confirmedAt := internalStartedAt.Add(3 * time.Hour)

	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", internalStartedAt.Add(time.Hour), pending); got.Applicability() != domain.RestrictionUnconfirmed ||
		!equalStrings(ids(got.Unconfirmed()), []string{"SYN-IR-1"}) {
		t.Fatalf("待确认：%v %v", got.Applicability(), ids(got.Unconfirmed()))
	}
	if got := applicabilityOf(t, domain.LoadingDeparture, "SYN-PCL-1", internalStartedAt.Add(time.Hour), pending); got.Applicability() != domain.RestrictionConflicting ||
		!equalStrings(ids(got.Conflicting()), []string{"SYN-IR-1"}) {
		t.Fatalf("冲突：%v %v", got.Applicability(), ids(got.Conflicting()))
	}

	confirm, err := domain.NewRestrictionDecision(decisionSpec(t, "D-1", domain.ConfirmCoverage, "SYN-SOURCE-SALES-MANAGER", confirmedAt,
		key(t, "SYN-PCL-1", domain.OutboundRelease), key(t, "SYN-PCL-1", domain.LoadingDeparture)))
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	if _, err := pending.Decide(confirm); !errors.Is(err, domain.ErrNotTheResponsibleSource) {
		t.Fatalf("非责任来源确认 err = %v", err)
	}

	confirmed := decide(t, pending, "D-2", domain.ConfirmCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", confirmedAt,
		key(t, "SYN-PCL-1", domain.OutboundRelease), key(t, "SYN-PCL-1", domain.LoadingDeparture))
	for _, action := range []domain.GuardedAction{domain.OutboundRelease, domain.LoadingDeparture} {
		if got := applicabilityOf(t, action, "SYN-PCL-1", confirmedAt, confirmed); got.Applicability() != domain.RestrictionInForce {
			t.Errorf("确认生效时刻起 %s 应有效：%v", action, got.Applicability())
		}
	}
	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", confirmedAt.Add(-time.Nanosecond), confirmed); got.Applicability() != domain.RestrictionUnconfirmed {
		t.Fatalf("确认不回溯：动作时点早于确认仍读到待确认，实得 %v", got.Applicability())
	}

	// 确认不解除任何限制，也不动别的覆盖项；已有效的覆盖项不再确认，已确认的也不再确认。
	again, err := domain.NewRestrictionDecision(decisionSpec(t, "D-3", domain.ConfirmCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", confirmedAt.Add(time.Hour),
		key(t, "SYN-PCL-1", domain.OutboundRelease)))
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	if _, err := confirmed.Decide(again); !errors.Is(err, domain.ErrCoverageNotDecidable) {
		t.Fatalf("重复确认 err = %v", err)
	}
	onForce, err := domain.NewRestrictionDecision(decisionSpec(t, "D-4", domain.ConfirmCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", confirmedAt,
		key(t, "SYN-PCL-1", domain.FinalDelivery)))
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	if _, err := pending.Decide(onForce); !errors.Is(err, domain.ErrCoverageNotDecidable) {
		t.Fatalf("确认本就有效的覆盖项 err = %v", err)
	}

	// 来源判定它不约束，走解除：待确认与冲突的覆盖项同样可以解除，解除之后不再确认。
	dismissed := decide(t, pending, "D-5", domain.ReleaseCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", confirmedAt,
		key(t, "SYN-PCL-1", domain.OutboundRelease))
	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", confirmedAt, dismissed); got.Applicability() != domain.RestrictionNoneRecorded {
		t.Fatalf("解除待确认的覆盖项：%v", got.Applicability())
	}
	late, err := domain.NewRestrictionDecision(decisionSpec(t, "D-6", domain.ConfirmCoverage, "SYN-SOURCE-TRADE-COMPLIANCE", confirmedAt.Add(time.Hour),
		key(t, "SYN-PCL-1", domain.OutboundRelease)))
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	if _, err := dismissed.Decide(late); !errors.Is(err, domain.ErrCoverageNotDecidable) {
		t.Fatalf("已解除的覆盖项不再确认 err = %v", err)
	}
}

// 决定只在它适用的地方成立：覆盖项不在这条限制里、已解除、时间早于限制开始或该覆盖项上一次决定。
// Covers: AT-CC-354（解除请求范围不足、动作不明、条件未证明——解除不成立，限制继续覆盖未解除范围）。
func TestADecisionIsRefusedWhereItDoesNotApply(t *testing.T) {
	covering := internalRestriction(t, "SYN-IR-1", "SYN-SOURCE-TRADE-COMPLIANCE",
		binding("SYN-PCL-1", domain.OutboundRelease), coverage{"SYN-PCL-1", domain.LoadingDeparture, domain.CoverageUnconfirmed})
	source := "SYN-SOURCE-TRADE-COMPLIANCE"
	at := internalStartedAt.Add(time.Hour)
	try := func(kind domain.RestrictionDecisionKind, when time.Time, restriction domain.InternalRestriction, covers ...domain.CoverageKey) error {
		t.Helper()
		made, err := domain.NewRestrictionDecision(decisionSpec(t, "D-X", kind, source, when, covers...))
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		_, err = restriction.Decide(made)
		return err
	}

	if err := try(domain.ReleaseCoverage, at, covering, key(t, "SYN-PCL-9", domain.OutboundRelease)); !errors.Is(err, domain.ErrCoverageNotInRestriction) {
		t.Errorf("别的对象的覆盖项 err = %v", err)
	}
	if err := try(domain.ReleaseCoverage, at, covering, key(t, "SYN-PCL-1", domain.FinalDelivery)); !errors.Is(err, domain.ErrCoverageNotInRestriction) {
		t.Errorf("没覆盖的动作 err = %v", err)
	}
	if err := try(domain.ReleaseCoverage, internalStartedAt.Add(-time.Second), covering, key(t, "SYN-PCL-1", domain.OutboundRelease)); !errors.Is(err, domain.ErrInvalidRestrictionDecision) {
		t.Errorf("早于限制开始的解除 err = %v", err)
	}
	// 一条决定里有一项不成立，整条不成立：不留下一半。
	if err := try(domain.ReleaseCoverage, at, covering,
		key(t, "SYN-PCL-1", domain.OutboundRelease), key(t, "SYN-PCL-9", domain.OutboundRelease)); !errors.Is(err, domain.ErrCoverageNotInRestriction) {
		t.Errorf("夹带不在册覆盖项的解除 err = %v", err)
	}

	released := decide(t, covering, "D-1", domain.ReleaseCoverage, source, at, key(t, "SYN-PCL-1", domain.OutboundRelease))
	if err := try(domain.ReleaseCoverage, at.Add(time.Hour), released, key(t, "SYN-PCL-1", domain.OutboundRelease)); !errors.Is(err, domain.ErrCoverageNotDecidable) {
		t.Errorf("重复解除 err = %v", err)
	}
	confirmed := decide(t, covering, "D-2", domain.ConfirmCoverage, source, at.Add(2*time.Hour), key(t, "SYN-PCL-1", domain.LoadingDeparture))
	if err := try(domain.ReleaseCoverage, at.Add(time.Hour), confirmed, key(t, "SYN-PCL-1", domain.LoadingDeparture)); !errors.Is(err, domain.ErrCoverageNotDecidable) {
		t.Errorf("早于该覆盖项上一次决定的解除 err = %v", err)
	}
	if err := try(domain.ReleaseCoverage, at.Add(2*time.Hour), confirmed, key(t, "SYN-PCL-1", domain.LoadingDeparture)); err != nil {
		t.Errorf("与上一次决定同一时刻的解除应成立：%v", err)
	}

	for name, spec := range map[string]domain.RestrictionDecisionSpec{
		"没有作用的覆盖项": decisionSpec(t, "D-Y", domain.ReleaseCoverage, source, at),
		"同一覆盖项列两次": decisionSpec(t, "D-Y", domain.ReleaseCoverage, source, at,
			key(t, "SYN-PCL-1", domain.OutboundRelease), key(t, "SYN-PCL-1", domain.OutboundRelease)),
		"种类在词表外": decisionSpec(t, "D-Y", domain.RestrictionDecisionKindInvalid, source, at, key(t, "SYN-PCL-1", domain.OutboundRelease)),
		"缺生效时间":  decisionSpec(t, "D-Y", domain.ReleaseCoverage, source, time.Time{}, key(t, "SYN-PCL-1", domain.OutboundRelease)),
	} {
		if _, err := domain.NewRestrictionDecision(spec); !errors.Is(err, domain.ErrInvalidRestrictionDecision) {
			t.Errorf("%s：err = %v", name, err)
		}
	}
}

// Covers: CC CONTEXT「当前限制适用性：有效限制、无内部限制、待确认或冲突」与 AT-CC-347、AT-CC-348（两个独立来源的限制各自保留，一个来源解除
// 自己的限制，另一个来源的限制继续约束）——四格与次序在这里钉：有效压冲突压待确认，册上无记录不是无内部限制的结论。
func TestApplicabilityFoldsFourCellsAtTheActionInstant(t *testing.T) {
	at := internalStartedAt.Add(time.Hour)
	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", at); got.Applicability() != domain.RestrictionNoneRecorded ||
		len(got.InForce())+len(got.Unconfirmed())+len(got.Conflicting()) != 0 {
		t.Fatalf("空册 = %v", got.Applicability())
	}

	first := internalRestriction(t, "SYN-IR-1", "SYN-SOURCE-A", binding("SYN-PCL-1", domain.OutboundRelease))
	second := internalRestriction(t, "SYN-IR-2", "SYN-SOURCE-B", binding("SYN-PCL-1", domain.OutboundRelease))
	pending := internalRestriction(t, "SYN-IR-3", "SYN-SOURCE-C", coverage{"SYN-PCL-1", domain.OutboundRelease, domain.CoverageUnconfirmed})
	conflict := internalRestriction(t, "SYN-IR-4", "SYN-SOURCE-D", coverage{"SYN-PCL-1", domain.OutboundRelease, domain.CoverageConflicting})

	all := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", at, first, second, pending, conflict)
	if all.Applicability() != domain.RestrictionInForce ||
		!equalStrings(ids(all.InForce()), []string{"SYN-IR-1", "SYN-IR-2"}) ||
		!equalStrings(ids(all.Unconfirmed()), []string{"SYN-IR-3"}) ||
		!equalStrings(ids(all.Conflicting()), []string{"SYN-IR-4"}) {
		t.Fatalf("四条限制并存：%v 有效%v 待确认%v 冲突%v", all.Applicability(), ids(all.InForce()), ids(all.Unconfirmed()), ids(all.Conflicting()))
	}

	firstReleased := decide(t, first, "D-1", domain.ReleaseCoverage, "SYN-SOURCE-A", at, key(t, "SYN-PCL-1", domain.OutboundRelease))
	stillBlocked := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", at, firstReleased, second)
	if stillBlocked.Applicability() != domain.RestrictionInForce || !equalStrings(ids(stillBlocked.InForce()), []string{"SYN-IR-2"}) {
		t.Fatalf("来源 A 解除自己的限制后来源 B 的限制要继续约束：%v %v", stillBlocked.Applicability(), ids(stillBlocked.InForce()))
	}

	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", at, pending, conflict); got.Applicability() != domain.RestrictionConflicting {
		t.Fatalf("冲突压过待确认：%v", got.Applicability())
	}
	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", at, pending); got.Applicability() != domain.RestrictionUnconfirmed {
		t.Fatalf("只有待确认：%v", got.Applicability())
	}
	if got := applicabilityOf(t, domain.OutboundRelease, "SYN-PCL-1", internalStartedAt.Add(-time.Second), first, pending, conflict); got.Applicability() != domain.RestrictionNoneRecorded {
		t.Fatalf("限制开始之前的动作时点不受约束：%v", got.Applicability())
	}
	if _, err := domain.JudgeInternalApplicability(domain.OutboundRelease, domain.DecisionScopeReference{}, nil, at); !errors.Is(err, domain.ErrInvalidAdmissibility) {
		t.Errorf("缺范围 err = %v", err)
	}
	if _, err := domain.JudgeInternalApplicability(domain.GuardedActionInvalid, mustValue(t, domain.NewDecisionScopeReference, "SYN-PCL-1"), nil, at); !errors.Is(err, domain.ErrInvalidAdmissibility) {
		t.Errorf("缺动作 err = %v", err)
	}
	if _, err := domain.JudgeInternalApplicability(domain.OutboundRelease, mustValue(t, domain.NewDecisionScopeReference, "SYN-PCL-1"), nil, time.Time{}); !errors.Is(err, domain.ErrInvalidAdmissibility) {
		t.Errorf("缺动作时点 err = %v", err)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
