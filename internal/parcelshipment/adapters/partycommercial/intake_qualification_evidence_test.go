package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

type intakeQualificationEvidenceDouble struct {
	proof psports.IntakeQualificationProof
	err   error
	asOf  time.Time
	rule  string
	calls int
}

func (double *intakeQualificationEvidenceDouble) ProveIntakeQualification(
	_ context.Context,
	_ psdomain.SourceIdentity,
	_ psdomain.IntakeSource,
	rule psdomain.QualificationRuleReference,
	asOf time.Time,
) (psports.IntakeQualificationProof, error) {
	double.calls++
	double.asOf = asOf
	double.rule = rule.String()
	if double.err != nil {
		return psports.IntakeQualificationProofInvalid, double.err
	}
	return double.proof, nil
}

func nodeIntakeWithQualifications(t *testing.T, refs ...string) pcdomain.IntakeQualificationContent {
	t.Helper()
	rules := make([]pcdomain.RuleReference, 0, len(refs))
	for _, ref := range refs {
		rules = append(rules, commercialRule(t, ref))
	}
	content, err := pcdomain.NewIntakeQualificationContent(
		stageRulePackage(t),
		[]pcdomain.DeclaredIntakeSource{pcdomain.DeclaredNodeIntake},
		rules,
	)
	if err != nil {
		t.Fatalf("new intake content: %v", err)
	}
	return content
}

func judgeWithEvidence(
	t *testing.T,
	content pcdomain.IntakeQualificationContent,
	evidence psports.IntakeQualificationEvidenceView,
) (psports.IntakeEligibility, bool, error) {
	t.Helper()
	subject := adapter.NewServiceStageRulesAdapter(
		intakeContentDouble{content: content, configured: true},
		finalContentDouble{},
		nil,
		nil,
		evidence,
	)
	return subject.JudgeIntakeEligibility(
		context.Background(), stageIdentity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		nodeIntakeSource(t),
	)
}

// Covers: ADR-0063——未配置证据口 + 非空清单保持未成立并点名引用；空清单仍成立且
// 不问证据；已证明则成立；读失败上抛不折未配置；未知前缀不得通过；nil 不得成立。
func TestIntakeEligibilityAsksEvidenceWithoutDefaultingEstablished(t *testing.T) {
	customs := "INTAKE-QUAL/customs-precheck"
	source := nodeIntakeSource(t)

	t.Run("unconfigured evidence leaves a declared qualification unproven", func(t *testing.T) {
		judged, configured, err := judgeWithEvidence(
			t, nodeIntakeWithQualifications(t, customs),
			adapter.UnconfiguredIntakeQualificationEvidence{},
		)
		if err != nil || !configured {
			t.Fatalf("configured = %v err = %v", configured, err)
		}
		if judged.Outcome != psports.IntakeEligibilityNotEstablished ||
			judged.Basis.String() != "INTAKE_QUALIFICATION_UNPROVEN/"+customs {
			t.Fatalf("outcome = %q basis = %q", judged.Outcome, judged.Basis)
		}
	})

	t.Run("empty qualification list stays established without asking evidence", func(t *testing.T) {
		spy := &intakeQualificationEvidenceDouble{proof: psports.IntakeQualificationUnproven}
		judged, configured, err := judgeWithEvidence(
			t, nodeIntakeWithQualifications(t), spy)
		if err != nil || !configured {
			t.Fatalf("configured = %v err = %v", configured, err)
		}
		if judged.Outcome != psports.IntakeEligibilityEstablished {
			t.Fatalf("outcome = %q; 空清单声明即成立", judged.Outcome)
		}
		if spy.calls != 0 {
			t.Fatalf("evidence calls = %d; 空清单不得问证据", spy.calls)
		}
	})

	t.Run("proven evidence establishes eligibility at the intake business time", func(t *testing.T) {
		spy := &intakeQualificationEvidenceDouble{proof: psports.IntakeQualificationProven}
		judged, configured, err := judgeWithEvidence(
			t, nodeIntakeWithQualifications(t, customs), spy)
		if err != nil || !configured {
			t.Fatalf("configured = %v err = %v", configured, err)
		}
		if judged.Outcome != psports.IntakeEligibilityEstablished {
			t.Fatalf("outcome = %q; 已证明应成立", judged.Outcome)
		}
		if spy.rule != customs || !spy.asOf.Equal(source.OccurredAt()) {
			t.Fatalf("rule = %q asOf = %s; 必须按引用与收寄业务时点取证", spy.rule, spy.asOf)
		}
	})

	t.Run("evidence read failure surfaces and is not unconfigured", func(t *testing.T) {
		unavailable := errors.New("证明方不可读")
		_, configured, err := judgeWithEvidence(
			t, nodeIntakeWithQualifications(t, customs),
			&intakeQualificationEvidenceDouble{err: unavailable},
		)
		if !errors.Is(err, unavailable) {
			t.Fatalf("error = %v, want wrapped evidence failure", err)
		}
		if configured {
			t.Fatal("configured = true; 依赖失败不得折成目录未配置")
		}
	})

	t.Run("unknown prefix cannot pass even if a registered prefix would prove", func(t *testing.T) {
		inner := &intakeQualificationEvidenceDouble{proof: psports.IntakeQualificationProven}
		evidence := adapter.NewKnownPrefixIntakeQualificationEvidence(
			map[string]psports.IntakeQualificationEvidenceView{
				"KNOWN": inner,
			},
		)
		judged, configured, err := judgeWithEvidence(
			t, nodeIntakeWithQualifications(t, customs), evidence)
		if err != nil || !configured {
			t.Fatalf("configured = %v err = %v", configured, err)
		}
		if judged.Outcome != psports.IntakeEligibilityNotEstablished ||
			judged.Basis.String() != "INTAKE_QUALIFICATION_UNPROVEN/"+customs {
			t.Fatalf("outcome = %q basis = %q; 未知前缀不得通过", judged.Outcome, judged.Basis)
		}
		if inner.calls != 0 {
			t.Fatalf("inner calls = %d; 未知前缀不得交给已登记口", inner.calls)
		}
	})

	t.Run("nil evidence does not establish a declared qualification", func(t *testing.T) {
		_, configured, err := judgeWithEvidence(
			t, nodeIntakeWithQualifications(t, customs), nil)
		if err == nil {
			t.Fatal("nil 证据口给出了答案——不得变成 ESTABLISHED")
		}
		if configured {
			t.Fatal("configured = true; nil 不得折成目录已配置且成立")
		}
	})
}

func TestQualificationRulePrefixSplitsOnlyTheAuthoritySegment(t *testing.T) {
	ref, err := psdomain.NewQualificationRuleReference("INTAKE-QUAL/customs-precheck")
	if err != nil {
		t.Fatalf("new reference: %v", err)
	}
	if got := psdomain.QualificationRulePrefix(ref); got != "INTAKE-QUAL" {
		t.Fatalf("prefix = %q, want INTAKE-QUAL（不得把后缀当关务身份）", got)
	}
}
