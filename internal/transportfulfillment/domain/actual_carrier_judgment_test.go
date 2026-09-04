package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	judgmentSegmentEstablishedAt = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	judgmentFormedAt             = time.Date(2026, 9, 1, 8, 0, 5, 0, time.UTC)
	judgmentEvidenceAt           = time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)
)

func openJudgmentSpec(t *testing.T) domain.OpenActualCarrierJudgmentSpec {
	t.Helper()
	return domain.OpenActualCarrierJudgmentSpec{
		TenantID:      mustRef(t, domain.NewTenantID, "tenant-1"),
		Segment:       mustRef(t, domain.NewFulfillmentSegmentReference, "segment-1"),
		EstablishedAt: judgmentSegmentEstablishedAt,
		FormedAt:      judgmentFormedAt,
	}
}

func mustOpenJudgment(t *testing.T) domain.ActualCarrierJudgment {
	t.Helper()
	judgment, err := domain.OpenActualCarrierJudgment(openJudgmentSpec(t))
	if err != nil {
		t.Fatalf("open judgment: %v", err)
	}
	return judgment
}

func mustCarrierSubject(t *testing.T, kind domain.CarrierSubjectKind, reference string) domain.CarrierSubject {
	t.Helper()
	subject, err := domain.NewCarrierSubject(kind, reference)
	if err != nil {
		t.Fatalf("carrier subject: %v", err)
	}
	return subject
}

// registeredEvidence 是一条指名了在册承运主体的合格依据。
func registeredEvidence(
	t *testing.T,
	source domain.CarrierEvidenceSource,
	reference string,
	occurredAt time.Time,
	subject domain.CarrierSubject,
) domain.CarrierEvidence {
	t.Helper()
	evidence, err := domain.NewCarrierEvidence(domain.CarrierEvidenceSpec{
		Source:     source,
		Reference:  mustRef(t, domain.NewCarrierEvidenceReference, reference),
		OccurredAt: occurredAt,
		Subject:    subject,
	})
	if err != nil {
		t.Fatalf("carrier evidence: %v", err)
	}
	return evidence
}

// unregisteredEvidence 是一条只带名称素材、承运主体身份未在册的合格依据。
func unregisteredEvidence(
	t *testing.T,
	source domain.CarrierEvidenceSource,
	reference string,
	occurredAt time.Time,
	material string,
) domain.CarrierEvidence {
	t.Helper()
	evidence, err := domain.NewCarrierEvidence(domain.CarrierEvidenceSpec{
		Source:     source,
		Reference:  mustRef(t, domain.NewCarrierEvidenceReference, reference),
		OccurredAt: occurredAt,
		Material:   material,
	})
	if err != nil {
		t.Fatalf("carrier evidence: %v", err)
	}
	return evidence
}

// Covers: CONTEXT「实际承运商判断」规则节首条「实际履约段成立时即形成首个判断版本……否则为待确认
// （无合格证据）。段没有『尚无判断』的状态——未知也是一个版本，未知期间从这一版起算」；ADR-0103
// 决定二「待确认是判断值，段成立即有第一版」。
func TestOpeningAJudgmentWithoutEvidenceFormsAPendingFirstVersion(t *testing.T) {
	judgment := mustOpenJudgment(t)

	versions := judgment.Versions()
	if len(versions) != 1 {
		t.Fatalf("段成立即有第一版，实得 %d 版", len(versions))
	}
	current := judgment.Current()
	if current.Sequence() != 1 {
		t.Fatalf("首版序号 = %d, want 1", current.Sequence())
	}
	reason, pending := current.Verdict().Pending()
	if !pending || reason != domain.NoQualifiedCarrierEvidence {
		t.Fatalf("首版应为待确认（无合格证据），实得 pending=%v reason=%s", pending, reason)
	}
	if _, identified := current.Verdict().Identified(); identified {
		t.Fatal("没有证据却识别出了承运主体")
	}
	// 未知期间从段成立起算：首版业务时间是段成立时刻，形成时间是本上下文铸的时钟。
	if !current.BusinessTime().Equal(judgmentSegmentEstablishedAt) {
		t.Fatalf("首版业务时间 = %s, want 段成立时刻 %s", current.BusinessTime(), judgmentSegmentEstablishedAt)
	}
	if !current.FormedAt().Equal(judgmentFormedAt) {
		t.Fatalf("首版形成时间 = %s, want %s", current.FormedAt(), judgmentFormedAt)
	}
	if len(current.Bases()) != 0 {
		t.Fatalf("无合格证据的版本不该带依据，实得 %d 条", len(current.Bases()))
	}
	if judgment.Segment().String() != "segment-1" || judgment.TenantID().String() != "tenant-1" {
		t.Fatal("判断的键（租户，段）没有原样保存")
	}
	if !judgment.SegmentEstablishedAt().Equal(judgmentSegmentEstablishedAt) {
		t.Fatalf("段成立时刻 = %s", judgment.SegmentEstablishedAt())
	}
}

func TestOpeningAJudgmentRequiresItsKeyAndBothTimes(t *testing.T) {
	cases := map[string]func(*domain.OpenActualCarrierJudgmentSpec){
		"缺租户":    func(spec *domain.OpenActualCarrierJudgmentSpec) { spec.TenantID = domain.TenantID{} },
		"缺段":     func(spec *domain.OpenActualCarrierJudgmentSpec) { spec.Segment = domain.FulfillmentSegmentReference{} },
		"缺段成立时刻": func(spec *domain.OpenActualCarrierJudgmentSpec) { spec.EstablishedAt = time.Time{} },
		"缺形成时间":  func(spec *domain.OpenActualCarrierJudgmentSpec) { spec.FormedAt = time.Time{} },
	}
	for name, breakOne := range cases {
		t.Run(name, func(t *testing.T) {
			spec := openJudgmentSpec(t)
			breakOne(&spec)
			if _, err := domain.OpenActualCarrierJudgment(spec); !errors.Is(err, domain.ErrInvalidActualCarrierJudgment) {
				t.Fatalf("error = %v, want ErrInvalidActualCarrierJudgment", err)
			}
		})
	}
}
