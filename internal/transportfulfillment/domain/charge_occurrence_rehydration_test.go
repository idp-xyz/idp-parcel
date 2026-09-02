package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证运输收费发生项的重建门（票 tf-unwired-seven/04，ADR-0028）：验形状与成对关系，
// 不重走转换门——不重放 ReviseValidity，也不重算「此刻该不该修订」。

var (
	occurrenceAtFixture = time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	revisedAtFixture    = time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
)

func occurrenceValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func rehydrateOccurrenceSpec(t *testing.T, objects ...string) domain.RehydrateChargeOccurrenceSpec {
	t.Helper()
	members := make([]domain.CarriedObjectReference, 0, len(objects))
	for _, object := range objects {
		members = append(members, occurrenceValue(t, domain.NewCarriedObjectReference, object))
	}
	return domain.RehydrateChargeOccurrenceSpec{
		TenantID:    occurrenceValue(t, domain.NewTenantID, "tenant-1"),
		Occurrence:  occurrenceValue(t, domain.NewChargeOccurrenceReference, "OCC-0001"),
		Journey:     occurrenceValue(t, domain.NewJourneyReference, "journey-1"),
		LegalEntity: occurrenceValue(t, domain.NewProcurementLegalEntityReference, "legal-1"),
		Provider:    occurrenceValue(t, domain.NewServiceProviderReference, "partner-1"),
		Agreement:   occurrenceValue(t, domain.NewAgreementSnapshotReference, "agreement-1/v1"),
		Reason:      domain.FailedAttemptOccurrence,
		FactBasis:   occurrenceValue(t, domain.NewOccurrenceBasisReference, "ATTEMPT-RESULT/a-1/parcel-1"),
		Scope:       occurrenceValue(t, domain.NewOccurrenceScopeReference, "scope-1"),
		Members:     members,
		Quantity:    1,
		Unit:        occurrenceValue(t, domain.NewQuantityUnitReference, "attempt"),
		OccurredAt:  occurrenceAtFixture,
		Validity:    occurrenceValue(t, domain.NewOccurrenceValidityVersion, "v1"),
	}
}

func revisedOccurrenceSpec(t *testing.T) domain.RehydrateChargeOccurrenceSpec {
	t.Helper()
	spec := rehydrateOccurrenceSpec(t, "parcel-1")
	spec.Validity = occurrenceValue(t, domain.NewOccurrenceValidityVersion, "v2")
	spec.Corrects = occurrenceValue(t, domain.NewOccurrenceValidityVersion, "v1")
	spec.RevisionKind = domain.OccurrenceSuperseded
	spec.RevisionBasis = occurrenceValue(t, domain.NewOccurrenceBasisReference, "CORRECTION/src-1")
	spec.RevisedAt = revisedAtFixture
	return spec
}

// Covers: 首版往返——修订四件全缺时不得凭空长出一段修订史。
func TestRehydratedFirstVersionCarriesNoRevision(t *testing.T) {
	rebuilt, err := domain.RehydrateChargeOccurrence(rehydrateOccurrenceSpec(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	if _, has := rebuilt.Corrects(); has {
		t.Fatal("首版长出了前身引用")
	}
	if _, _, _, revised := rebuilt.Revision(); revised {
		t.Fatal("首版长出了修订三件")
	}
	if len(rebuilt.Members()) != 2 {
		t.Fatalf("成员数 = %d, want 2", len(rebuilt.Members()))
	}
	if quantity, unit := rebuilt.Quantity(); quantity != 1 || unit.String() != "attempt" {
		t.Fatalf("数量与单位没有成对带回：%d %q", quantity, unit)
	}
	if rebuilt.Reason() != domain.FailedAttemptOccurrence {
		t.Fatalf("发生原因 = %q", rebuilt.Reason())
	}
}

// Covers: 修订版往返——前身引用与修订三件原样带回，且不重放 ReviseValidity。
func TestRehydratedRevisionCarriesItsPredecessorAndRevisionTriple(t *testing.T) {
	rebuilt, err := domain.RehydrateChargeOccurrence(revisedOccurrenceSpec(t))
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	corrects, has := rebuilt.Corrects()
	if !has || corrects.String() != "v1" {
		t.Fatalf("前身引用没带回：has=%v", has)
	}
	kind, basis, at, revised := rebuilt.Revision()
	if !revised || kind != domain.OccurrenceSuperseded ||
		basis.String() != "CORRECTION/src-1" || !at.Equal(revisedAtFixture) {
		t.Fatalf("修订三件没带回：%v %q %v", revised, kind, at)
	}
}

// Covers: 修订四件（前身、走向、依据、时刻）同在或同缺——库面半截会重建出一个
// 领域任何路径都产不出的版本。
func TestRehydrationRefusesAHalfRevision(t *testing.T) {
	for name, mutate := range map[string]func(*domain.RehydrateChargeOccurrenceSpec){
		"缺前身": func(s *domain.RehydrateChargeOccurrenceSpec) {
			s.Corrects = domain.OccurrenceValidityVersion{}
		},
		"缺走向": func(s *domain.RehydrateChargeOccurrenceSpec) {
			s.RevisionKind = domain.OccurrenceRevisionKindInvalid
		},
		"缺依据": func(s *domain.RehydrateChargeOccurrenceSpec) {
			s.RevisionBasis = domain.OccurrenceBasisReference{}
		},
		"缺时刻": func(s *domain.RehydrateChargeOccurrenceSpec) {
			s.RevisedAt = time.Time{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			broken := revisedOccurrenceSpec(t)
			mutate(&broken)
			if _, err := domain.RehydrateChargeOccurrence(broken); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
				t.Fatalf("%s 被收下了：err = %v", name, err)
			}
		})
	}
}

// Covers: 领域 ReviseValidity 的三道门在库面同样立得住——沿用原版本号是覆盖，
// 修订时刻不得早于发生时间。这两条是行内就比得出来的，不属重放转换门。
func TestRehydrationRefusesASelfCorrectingOrBackdatedRevision(t *testing.T) {
	selfCorrecting := revisedOccurrenceSpec(t)
	selfCorrecting.Corrects = selfCorrecting.Validity
	if _, err := domain.RehydrateChargeOccurrence(selfCorrecting); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
		t.Fatalf("自指的修订被收下了：err = %v", err)
	}

	backdated := revisedOccurrenceSpec(t)
	backdated.RevisedAt = occurrenceAtFixture.Add(-time.Hour)
	if _, err := domain.RehydrateChargeOccurrence(backdated); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
		t.Fatalf("早于发生时间的修订被收下了：err = %v", err)
	}
}

// Covers: 本体必备件缺一即拒，且成员不得为空或重复——与构造门同一套判据。
func TestRehydrationRefusesAnIncompleteOccurrence(t *testing.T) {
	for name, mutate := range map[string]func(*domain.RehydrateChargeOccurrenceSpec){
		"缺协议快照": func(s *domain.RehydrateChargeOccurrenceSpec) {
			s.Agreement = domain.AgreementSnapshotReference{}
		},
		"缺服务提供方": func(s *domain.RehydrateChargeOccurrenceSpec) {
			s.Provider = domain.ServiceProviderReference{}
		},
		"缺旅程": func(s *domain.RehydrateChargeOccurrenceSpec) { s.Journey = domain.JourneyReference{} },
		"集外发生原因": func(s *domain.RehydrateChargeOccurrenceSpec) {
			s.Reason = domain.ChargeOccurrenceReason(99)
		},
		"空成员": func(s *domain.RehydrateChargeOccurrenceSpec) {
			s.Members = nil
		},
		"数量非正": func(s *domain.RehydrateChargeOccurrenceSpec) { s.Quantity = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			broken := rehydrateOccurrenceSpec(t, "parcel-1")
			mutate(&broken)
			if _, err := domain.RehydrateChargeOccurrence(broken); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
				t.Fatalf("%s 被收下了：err = %v", name, err)
			}
		})
	}
}

// Covers: 重建出的发生项仍是可继续演进的聚合——后续修订走原有转换门（ADR-0028）。
func TestARehydratedOccurrenceStillAcceptsReviseValidity(t *testing.T) {
	rebuilt, err := domain.RehydrateChargeOccurrence(rehydrateOccurrenceSpec(t, "parcel-1"))
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	revised, err := rebuilt.ReviseValidity(
		domain.OccurrenceInvalidated,
		occurrenceValue(t, domain.NewOccurrenceValidityVersion, "v2"),
		occurrenceValue(t, domain.NewOccurrenceBasisReference, "CORRECTION/src-2"),
		revisedAtFixture,
	)
	if err != nil {
		t.Fatalf("重建出的发生项应当还能走转换门：%v", err)
	}
	if corrects, has := revised.Corrects(); !has || corrects.String() != "v1" {
		t.Fatalf("修订没有回指首版：has=%v", has)
	}
}
