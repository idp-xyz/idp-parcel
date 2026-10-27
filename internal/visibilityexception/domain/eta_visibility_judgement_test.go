package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var judgedAt = predictedAt.Add(time.Hour)

func visibilityRule(t *testing.T, milestone string, qualifying ...string) domain.ETAVisibilityRule {
	t.Helper()
	confidences := make([]domain.ConfidenceReference, 0, len(qualifying))
	for _, raw := range qualifying {
		confidences = append(confidences, mustValue(t, domain.NewConfidenceReference, raw))
	}
	rule, err := domain.NewETAVisibilityRule(domain.ETAVisibilityRuleSpec{
		GateVersion:     mustValue(t, domain.NewETAQualityGateVersion, "gate/v1"),
		ServiceRule:     mustValue(t, domain.NewCustomerServiceRuleVersion, "contract-c1/v3"),
		TargetMilestone: mustValue(t, domain.NewMilestoneReference, milestone),
		Qualifying:      confidences,
		Expression:      domain.ETAShownAsTimeRange,
		StopBoundary:    mustValue(t, domain.NewETAStopShowingBoundary, "stop/after-delivered"),
	})
	if err != nil {
		t.Fatalf("new ETA visibility rule: %v", err)
	}
	return rule
}

func candidateETA(t *testing.T, version string) domain.ETAPrediction {
	t.Helper()
	eta, err := domain.FormETAPrediction(etaSpec(t, version))
	if err != nil {
		t.Fatalf("form candidate ETA: %v", err)
	}
	return eta
}

func judgementCustomer(t *testing.T) domain.CustomerAccountReference {
	t.Helper()
	return mustValue(t, domain.NewCustomerAccountReference, "customer-1")
}

// Covers: AT-VE-187——内部 ETA 已形成，适用质量门槛与客户服务规则条件明确满足：形成客户可见
// ETA 判断，引用候选 ETA 版本，保存门槛版本、规则版本、获准对客表达与停止展示边界。
func TestAMetQualityGateFormsACustomerVisibleJudgement(t *testing.T) {
	eta := candidateETA(t, "eta-1/v1")
	rule := visibilityRule(t, "DELIVERED", "MEDIUM/history-based", "HIGH/carrier-confirmed")

	finding, err := domain.AssessETAVisibility(eta, domain.ConfiguredETARule(rule))
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	judgement, err := domain.FormETAVisibilityJudgement(
		mustValue(t, domain.NewETAVisibilityJudgementVersionID, "evj-1"), judgementCustomer(t), finding, judgedAt)
	if err != nil {
		t.Fatalf("form judgement: %v", err)
	}

	if judgement.Conclusion() != domain.CustomerVisibleETA || judgement.Reason() != domain.QualityMetTheGate {
		t.Fatalf("conclusion = %q reason = %q", judgement.Conclusion(), judgement.Reason())
	}
	if judgement.CandidateVersion() != eta.Version() || judgement.CandidateSource() != eta.Source() ||
		judgement.Milestone() != eta.Milestone() || judgement.Parcel() != eta.Parcel() {
		t.Fatalf("judgement does not reference its candidate ETA: %#v", judgement)
	}
	gate, service, bound := judgement.RuleBasis()
	if !bound || gate.String() != "gate/v1" || service.String() != "contract-c1/v3" {
		t.Fatalf("rule basis = %q %q bound=%v", gate, service, bound)
	}
	expression, boundary, permitted := judgement.CustomerExpression()
	if !permitted || expression != domain.ETAShownAsTimeRange || boundary.String() != "stop/after-delivered" {
		t.Fatalf("customer expression = %q %q permitted=%v", expression, boundary, permitted)
	}
	if _, superseding := judgement.PriorVersion(); superseding {
		t.Fatal("首个判断不该指回前版")
	}
}

// Covers: AT-VE-052——质量明确未达门槛：形成 ETA 不可见判断，保留候选 ETA、门槛版本与理由；
// 不可见的判断不带对客表达，也没有任何可被当成可见的字段。
func TestAGateMissFormsAnETANotVisibleJudgementWithoutAnExpression(t *testing.T) {
	eta := candidateETA(t, "eta-1/v1")
	rule := visibilityRule(t, "DELIVERED", "HIGH/carrier-confirmed")

	finding, err := domain.AssessETAVisibility(eta, domain.ConfiguredETARule(rule))
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	judgement, err := domain.FormETAVisibilityJudgement(
		mustValue(t, domain.NewETAVisibilityJudgementVersionID, "evj-1"), judgementCustomer(t), finding, judgedAt)
	if err != nil {
		t.Fatalf("form judgement: %v", err)
	}
	if judgement.Conclusion() != domain.ETANotVisible || judgement.Reason() != domain.QualityBelowTheGate {
		t.Fatalf("conclusion = %q reason = %q", judgement.Conclusion(), judgement.Reason())
	}
	if _, _, bound := judgement.RuleBasis(); !bound {
		t.Fatal("未达门槛的判断要留下它对照的门槛版本")
	}
	if _, _, permitted := judgement.CustomerExpression(); permitted {
		t.Fatal("ETA 不可见的判断带出了对客表达")
	}
	if judgement.CandidateVersion() != eta.Version() {
		t.Fatalf("candidate = %q; 不可见判断也要引用候选 ETA 版本", judgement.CandidateVersion())
	}
}

// Covers: AT-VE-170——适用合同、门槛或客户服务规则缺失或冲突：形成 ETA 可见性待判断并留下缺口或
// 冲突作理由，不按默认值判为可见，也不借用任何规则版本。
func TestAMissingOrConflictingRuleFormsPendingJudgementNeverVisible(t *testing.T) {
	eta := candidateETA(t, "eta-1/v1")
	for _, tc := range []struct {
		name   string
		basis  domain.ETAVisibilityRuleBasis
		reason domain.ETAVisibilityReason
	}{
		{"规则缺失", domain.UnconfiguredETARule(), domain.CustomerRuleNotConfigured},
		{"规则冲突", domain.ConflictingETARules(), domain.CustomerRuleInConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			finding, err := domain.AssessETAVisibility(eta, tc.basis)
			if err != nil {
				t.Fatalf("assess: %v", err)
			}
			judgement, err := domain.FormETAVisibilityJudgement(
				mustValue(t, domain.NewETAVisibilityJudgementVersionID, "evj-1"), judgementCustomer(t), finding, judgedAt)
			if err != nil {
				t.Fatalf("form judgement: %v", err)
			}
			if judgement.Conclusion() != domain.ETAVisibilityPendingJudgement || judgement.Reason() != tc.reason {
				t.Fatalf("conclusion = %q reason = %q", judgement.Conclusion(), judgement.Reason())
			}
			if _, _, bound := judgement.RuleBasis(); bound {
				t.Fatal("没有适用规则，却记下了规则版本")
			}
			if _, _, permitted := judgement.CustomerExpression(); permitted {
				t.Fatal("待判断带出了对客表达")
			}
		})
	}
}

// Covers: AT-VE-214——判断针对规则指名的目标里程碑；候选 ETA 的里程碑不是它时不判——不拿别的里程碑的
// ETA 去顶客户视图要的那一个。
func TestAnETAForAnotherMilestoneThanTheRuleTargetIsNotJudged(t *testing.T) {
	eta := candidateETA(t, "eta-1/v1")
	rule := visibilityRule(t, "OUT_FOR_DELIVERY", "MEDIUM/history-based")

	if _, err := domain.AssessETAVisibility(eta, domain.ConfiguredETARule(rule)); !errors.Is(err, domain.ErrETAOutsideRuleTarget) {
		t.Fatalf("err = %v, want ErrETAOutsideRuleTarget", err)
	}
}

// Covers: AT-VE-213 与 UC-VE-003 规则「新 ETA、质量门槛、合同或客户服务规则版本生效时必须形成新的 ETA 客户可见性
// 判断和替代关系，原判断不覆盖」——替代换版本、指回前版，前版一个字段都不动。
func TestASupersedingJudgementKeepsTheOriginalIntact(t *testing.T) {
	rule := visibilityRule(t, "DELIVERED", "MEDIUM/history-based")
	first := judgementOver(t, "evj-1", candidateETA(t, "eta-1/v1"), domain.ConfiguredETARule(rule))

	secondFinding, err := domain.AssessETAVisibility(candidateETA(t, "eta-1/v2"), domain.UnconfiguredETARule())
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	next, err := first.Supersede(mustValue(t, domain.NewETAVisibilityJudgementVersionID, "evj-2"), secondFinding, judgedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	prior, superseding := next.PriorVersion()
	if !superseding || prior != first.Version() {
		t.Fatalf("prior = %q superseding = %v; 替代要指回前版", prior, superseding)
	}
	if first.Conclusion() != domain.CustomerVisibleETA || first.CandidateVersion().String() != "eta-1/v1" {
		t.Fatalf("前一版被改写：%#v", first)
	}
	if _, err := first.Supersede(first.Version(), secondFinding, judgedAt); !errors.Is(err, domain.ErrInvalidETAVisibilityJudgement) {
		t.Fatalf("err = %v; 以自身版本号替代自己不该成立", err)
	}
}

func judgementOver(
	t *testing.T,
	version string,
	eta domain.ETAPrediction,
	basis domain.ETAVisibilityRuleBasis,
) domain.ETAVisibilityJudgement {
	t.Helper()
	finding, err := domain.AssessETAVisibility(eta, basis)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	judgement, err := domain.FormETAVisibilityJudgement(
		mustValue(t, domain.NewETAVisibilityJudgementVersionID, version), judgementCustomer(t), finding, judgedAt)
	if err != nil {
		t.Fatalf("form judgement: %v", err)
	}
	return judgement
}

// Covers: 同一候选 ETA 版本、同一规则依据得同一发现——应用层据此判重放，不为没变的依据再换一代判断。
func TestTheSameBasisAssessesToTheSameFinding(t *testing.T) {
	eta := candidateETA(t, "eta-1/v1")
	rule := visibilityRule(t, "DELIVERED", "MEDIUM/history-based")
	first, err := domain.AssessETAVisibility(eta, domain.ConfiguredETARule(rule))
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	again, err := domain.AssessETAVisibility(eta, domain.ConfiguredETARule(rule))
	if err != nil {
		t.Fatalf("assess again: %v", err)
	}
	if first != again {
		t.Fatalf("同一依据得到两个发现：%#v vs %#v", first, again)
	}
	otherVersion, err := domain.AssessETAVisibility(candidateETA(t, "eta-1/v2"), domain.ConfiguredETARule(rule))
	if err != nil {
		t.Fatalf("assess v2: %v", err)
	}
	if first == otherVersion {
		t.Fatal("候选 ETA 换了版本，发现却相等——新 ETA 会被当成重放")
	}
}

// Covers: 判断重建口同样要过结论与字段成对的不变量：可见必带表达与停止边界与规则版本；不可见带规则版本、不带表达；
// 待判断什么都不带；结论与理由必须配对。坏行不能在读回时变成一个看起来合法的判断。
func TestRehydrationRefusesShapesTheConclusionForbids(t *testing.T) {
	rule := visibilityRule(t, "DELIVERED", "MEDIUM/history-based")
	eta := candidateETA(t, "eta-1/v1")
	good, err := domain.AssessETAVisibility(eta, domain.ConfiguredETARule(rule))
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	pending, err := domain.AssessETAVisibility(eta, domain.UnconfiguredETARule())
	if err != nil {
		t.Fatalf("assess pending: %v", err)
	}
	notVisible, err := domain.AssessETAVisibility(eta, domain.ConfiguredETARule(visibilityRule(t, "DELIVERED", "HIGH/x")))
	if err != nil {
		t.Fatalf("assess not visible: %v", err)
	}

	mutate := func(base domain.ETAVisibilityFinding, change func(*domain.ETAVisibilityFinding)) domain.ETAVisibilityFinding {
		change(&base)
		return base
	}
	for _, tc := range []struct {
		name    string
		finding domain.ETAVisibilityFinding
	}{
		{"可见缺停止边界", mutate(good, func(f *domain.ETAVisibilityFinding) { f.StopBoundary = domain.ETAStopShowingBoundary{} })},
		{"可见缺对客表达", mutate(good, func(f *domain.ETAVisibilityFinding) { f.Expression = domain.ETAExpressionInvalid })},
		{"可见缺规则版本", mutate(good, func(f *domain.ETAVisibilityFinding) { f.ServiceRule = domain.CustomerServiceRuleVersion{} })},
		{"可见配了未达门槛的理由", mutate(good, func(f *domain.ETAVisibilityFinding) { f.Reason = domain.QualityBelowTheGate })},
		{"不可见带对客表达", mutate(notVisible, func(f *domain.ETAVisibilityFinding) { f.Expression = domain.ETAShownAsPointValue })},
		{"不可见缺门槛版本", mutate(notVisible, func(f *domain.ETAVisibilityFinding) { f.GateVersion = domain.ETAQualityGateVersion{} })},
		{"待判断带规则版本", mutate(pending, func(f *domain.ETAVisibilityFinding) { f.GateVersion = good.GateVersion })},
		{"待判断配了达标理由", mutate(pending, func(f *domain.ETAVisibilityFinding) { f.Reason = domain.QualityMetTheGate })},
		{"结论未定", mutate(good, func(f *domain.ETAVisibilityFinding) { f.Conclusion = domain.ETAVisibilityConclusionInvalid })},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.FormETAVisibilityJudgement(
				mustValue(t, domain.NewETAVisibilityJudgementVersionID, "evj-1"), judgementCustomer(t), tc.finding, judgedAt)
			if !errors.Is(err, domain.ErrInvalidETAVisibilityJudgement) {
				t.Fatalf("err = %v; 形状不成立的判断被收下了", err)
			}
		})
	}
}

// Covers: 门槛规则本身的完整性——达标的可信度依据集合不能为空（空集合意味着永远不可见，那是
// 另一个决定），表达与停止边界必备。
func TestARuleDemandsItsFullShape(t *testing.T) {
	valid := domain.ETAVisibilityRuleSpec{
		GateVersion:     mustValue(t, domain.NewETAQualityGateVersion, "gate/v1"),
		ServiceRule:     mustValue(t, domain.NewCustomerServiceRuleVersion, "contract-c1/v3"),
		TargetMilestone: mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Qualifying:      []domain.ConfidenceReference{mustValue(t, domain.NewConfidenceReference, "HIGH")},
		Expression:      domain.ETAShownAsPointValue,
		StopBoundary:    mustValue(t, domain.NewETAStopShowingBoundary, "stop/x"),
	}
	if _, err := domain.NewETAVisibilityRule(valid); err != nil {
		t.Fatalf("valid rule refused: %v", err)
	}
	for name, change := range map[string]func(*domain.ETAVisibilityRuleSpec){
		"缺门槛版本":  func(s *domain.ETAVisibilityRuleSpec) { s.GateVersion = domain.ETAQualityGateVersion{} },
		"缺规则版本":  func(s *domain.ETAVisibilityRuleSpec) { s.ServiceRule = domain.CustomerServiceRuleVersion{} },
		"缺目标里程碑": func(s *domain.ETAVisibilityRuleSpec) { s.TargetMilestone = domain.MilestoneReference{} },
		"达标集合为空": func(s *domain.ETAVisibilityRuleSpec) { s.Qualifying = nil },
		"表达未定":   func(s *domain.ETAVisibilityRuleSpec) { s.Expression = domain.ETAExpressionInvalid },
		"缺停止边界":  func(s *domain.ETAVisibilityRuleSpec) { s.StopBoundary = domain.ETAStopShowingBoundary{} },
	} {
		t.Run(name, func(t *testing.T) {
			spec := valid
			change(&spec)
			if _, err := domain.NewETAVisibilityRule(spec); !errors.Is(err, domain.ErrInvalidETAVisibilityRule) {
				t.Fatalf("err = %v; 不完整的规则被收下了", err)
			}
		})
	}
}
