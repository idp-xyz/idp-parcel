package domain

import (
	"errors"
	"testing"
	"time"
)

func rehydratedTrackingSpec(t *testing.T) RehydrateExternalTrackingFactSpec {
	t.Helper()
	adopted := trackingFactSpec(t)
	return RehydrateExternalTrackingFactSpec{
		TenantID:       adopted.TenantID,
		Fact:           adopted.Fact,
		Version:        adopted.Version,
		Source:         adopted.Source,
		Credential:     adopted.Credential,
		Object:         adopted.Object,
		SourceEvent:    adopted.SourceEvent,
		Status:         adopted.Status,
		OccurredAt:     adopted.OccurredAt,
		ReceivedAt:     adopted.ReceivedAt,
		EffectiveBasis: EffectiveTimePending,
		Origin:         VersionFromMaterial,
	}
}

func TestARehydratedPendingFactReadsBackAsPending(t *testing.T) {
	fact, err := RehydrateExternalTrackingFact(rehydratedTrackingSpec(t))
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	if _, judged := fact.EffectiveAt(); judged {
		t.Fatal("待判断的行装回来却成了判断过")
	}
}

func TestARehydratedRuleJudgmentCarriesItsRuleVersion(t *testing.T) {
	spec := rehydratedTrackingSpec(t)
	spec.EffectiveBasis = EffectiveTimeJudgedByRule
	spec.EffectiveAt = trackingReceivedAt
	spec.EffectiveRule = "aggregator-a/effective-time"
	spec.EffectiveRuleVersion = "v3"
	fact, err := RehydrateExternalTrackingFact(spec)
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	rule, has := fact.Effective().Rule()
	if !has || rule.Version() != "v3" {
		t.Fatalf("规则版本没有装回：%q has=%v", rule.Version(), has)
	}
}

func TestRehydrationRefusesRowsNoDoorCouldHaveProduced(t *testing.T) {
	cases := map[string]func(*RehydrateExternalTrackingFactSpec){
		"源未给发生时间的行": func(spec *RehydrateExternalTrackingFactSpec) { spec.OccurredAt = time.Time{} },
		"待判断却带有效时间": func(spec *RehydrateExternalTrackingFactSpec) { spec.EffectiveAt = trackingReceivedAt },
		"显式判断却没有时间": func(spec *RehydrateExternalTrackingFactSpec) { spec.EffectiveBasis = EffectiveTimeJudgedExplicitly },
		"按规则判断却没有规则版本": func(spec *RehydrateExternalTrackingFactSpec) {
			spec.EffectiveBasis = EffectiveTimeJudgedByRule
			spec.EffectiveAt = trackingReceivedAt
			spec.EffectiveRule = "aggregator-a/effective-time"
		},
		"依据不在封闭集合内": func(spec *RehydrateExternalTrackingFactSpec) { spec.EffectiveBasis = EffectiveTimeBasis(9) },
		"素材版本回指前版却无源声明": func(spec *RehydrateExternalTrackingFactSpec) {
			spec.Supersedes, _ = NewExternalTrackingFactVersion("EXTV-000000000000")
		},
		"判断版本却不回指前版": func(spec *RehydrateExternalTrackingFactSpec) {
			spec.Origin = VersionFromJudgment
			spec.EffectiveBasis = EffectiveTimeJudgedExplicitly
			spec.EffectiveAt = trackingReceivedAt
		},
		"判断版本却仍待判断": func(spec *RehydrateExternalTrackingFactSpec) {
			spec.Origin = VersionFromJudgment
			spec.Supersedes, _ = NewExternalTrackingFactVersion("EXTV-000000000000")
		},
		"版本来路不在封闭集合内": func(spec *RehydrateExternalTrackingFactSpec) { spec.Origin = VersionOrigin(7) },
		"前版引用指向自己": func(spec *RehydrateExternalTrackingFactSpec) {
			spec.CorrectionOf, _ = NewSourceEventReference("evt-0")
			spec.Supersedes = spec.Version
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := rehydratedTrackingSpec(t)
			mutate(&spec)
			if _, err := RehydrateExternalTrackingFact(spec); !errors.Is(err, ErrInvalidRehydratedExternalTrackingFact) {
				t.Fatalf("应以重建哨兵拒绝，得到：%v", err)
			}
		})
	}
}

func TestAJudgedVersionMayPointBackWithoutASourceCorrection(t *testing.T) {
	spec := rehydratedTrackingSpec(t)
	spec.Version, _ = NewExternalTrackingFactVersion("EXTV-000000000002")
	spec.Supersedes, _ = NewExternalTrackingFactVersion("EXTV-000000000001")
	spec.EffectiveBasis = EffectiveTimeJudgedExplicitly
	spec.EffectiveAt = trackingReceivedAt
	spec.Origin = VersionFromJudgment
	fact, err := RehydrateExternalTrackingFact(spec)
	if err != nil {
		t.Fatalf("判断形成的版本回指待判断的那一版是正当来路：%v", err)
	}
	if fact.Origin() != VersionFromJudgment {
		t.Fatalf("版本来路没有装回：%s", fact.Origin())
	}
}
