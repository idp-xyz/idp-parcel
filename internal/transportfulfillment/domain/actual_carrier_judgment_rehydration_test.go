package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// rehydratedJudgmentSpec 是一份三版判断在库面的样子：首版待确认（无合格证据）、第二版凭 SCAN-A 已识别 X、
// 第三版因 CALLBACK-B 指向 Y 而来源冲突。
func rehydratedJudgmentSpec(t *testing.T) domain.RehydrateActualCarrierJudgmentSpec {
	t.Helper()
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	carrierY := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-y")
	scanA := registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-A", judgmentEvidenceAt, carrierX)
	callbackB := registeredEvidence(t, domain.TrustedChannelCallback, "CALLBACK-B", judgmentEvidenceAt.Add(time.Hour), carrierY)
	return domain.RehydrateActualCarrierJudgmentSpec{
		TenantID:      mustRef(t, domain.NewTenantID, "tenant-1"),
		Segment:       mustRef(t, domain.NewFulfillmentSegmentReference, "segment-1"),
		EstablishedAt: judgmentSegmentEstablishedAt,
		Versions: []domain.RehydrateActualCarrierJudgmentVersionSpec{
			{Sequence: 1, Pending: domain.NoQualifiedCarrierEvidence, BusinessTime: judgmentSegmentEstablishedAt, FormedAt: judgmentFormedAt},
			{Sequence: 2, Subject: carrierX, BusinessTime: judgmentEvidenceAt, FormedAt: judgmentEvidenceAt.Add(time.Minute), Bases: []domain.CarrierEvidence{scanA}},
			{Sequence: 3, Pending: domain.CarrierEvidenceSourceConflict, BusinessTime: judgmentEvidenceAt.Add(time.Hour), FormedAt: judgmentEvidenceAt.Add(time.Hour + time.Minute), Bases: []domain.CarrierEvidence{scanA, callbackB}},
		},
	}
}

func TestRehydratingAJudgmentRestoresEveryVersionAsRecorded(t *testing.T) {
	spec := rehydratedJudgmentSpec(t)
	judgment, err := domain.RehydrateActualCarrierJudgment(spec)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if judgment.TenantID() != spec.TenantID || judgment.Segment() != spec.Segment || !judgment.SegmentEstablishedAt().Equal(judgmentSegmentEstablishedAt) {
		t.Fatal("键或段成立时刻没有原样装回")
	}
	versions := judgment.Versions()
	if len(versions) != 3 {
		t.Fatalf("应装回三版，实得 %d", len(versions))
	}
	if subject, identified := versions[1].Verdict().Identified(); !identified || subject.Reference() != "party/carrier-x" {
		t.Fatal("第二版的已识别主体没有装回")
	}
	if reason, pending := judgment.Current().Verdict().Pending(); !pending || reason != domain.CarrierEvidenceSourceConflict {
		t.Fatal("第三版的冲突没有装回")
	}
	if len(judgment.Current().Bases()) != 2 {
		t.Fatalf("第三版依据应两条，实得 %d", len(judgment.Current().Bases()))
	}
	// 装回来的聚合照样能往前走：转换门只看当前版本与段成立时刻。
	if _, err := judgment.WithdrawEvidence(mustRef(t, domain.NewCarrierEvidenceReference, "CALLBACK-B"), judgmentEvidenceAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("装回后的判断应能重新派生：%v", err)
	}
}

// 重建门只核形状与成对关系（ADR-0028）：每一格都是库里一行自己就看得出的坏，不重算「该不该识别」。
func TestRehydratingRefusesRowsTheDomainCannotProduce(t *testing.T) {
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	cases := map[string]func(*domain.RehydrateActualCarrierJudgmentSpec){
		"缺租户": func(spec *domain.RehydrateActualCarrierJudgmentSpec) { spec.TenantID = domain.TenantID{} },
		"缺段": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Segment = domain.FulfillmentSegmentReference{}
		},
		"缺段成立时刻": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.EstablishedAt = time.Time{}
		},
		"一版都没有——段成立即有首版，空历史是坏行": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions = nil
		},
		"序号不从 1 起": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[0].Sequence = 2
		},
		"序号有洞——版本只追加，中间少一版说明有人删过": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[2].Sequence = 4
		},
		"判断值既识别又待确认": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[1].Pending = domain.CarrierIdentityNotRegistered
		},
		"判断值既不识别也无原因": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[1].Subject = domain.CarrierSubject{}
		},
		"缺业务时间": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[1].BusinessTime = time.Time{}
		},
		"缺形成时间": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[1].FormedAt = time.Time{}
		},
		"业务时间早于段成立": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[1].BusinessTime = judgmentSegmentEstablishedAt.Add(-time.Second)
		},
		"无合格证据却带着依据": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[0].Bases = spec.Versions[1].Bases
		},
		"已识别却没有依据": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[1].Bases = nil
		},
		"来源冲突却没有依据": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[2].Bases = nil
		},
		"同一版里同一引用出现两次": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[2].Bases = []domain.CarrierEvidence{spec.Versions[1].Bases[0], spec.Versions[1].Bases[0]}
		},
		"依据本身是零值": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[1].Bases = []domain.CarrierEvidence{{}}
		},
		"依据的业务时间早于段成立": func(spec *domain.RehydrateActualCarrierJudgmentSpec) {
			spec.Versions[1].Bases = []domain.CarrierEvidence{
				registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-A", judgmentSegmentEstablishedAt.Add(-time.Second), carrierX),
			}
		},
	}
	for name, breakOne := range cases {
		t.Run(name, func(t *testing.T) {
			spec := rehydratedJudgmentSpec(t)
			breakOne(&spec)
			if _, err := domain.RehydrateActualCarrierJudgment(spec); !errors.Is(err, domain.ErrInvalidRehydratedActualCarrierJudgment) {
				t.Fatalf("error = %v, want ErrInvalidRehydratedActualCarrierJudgment", err)
			}
		})
	}
}

// 「身份未登记」这一版可以只带素材依据、也可以在册与未在册并存——重建门不重走派生算法去核它。
func TestRehydratingAcceptsAPendingIdentityVersionWithMixedBases(t *testing.T) {
	spec := rehydratedJudgmentSpec(t)
	unregistered := unregisteredEvidence(t, domain.TrustedChannelCallback, "CALLBACK-C", judgmentEvidenceAt.Add(2*time.Hour), "Z Express")
	spec.Versions = append(spec.Versions, domain.RehydrateActualCarrierJudgmentVersionSpec{
		Sequence:     4,
		Pending:      domain.CarrierIdentityNotRegistered,
		BusinessTime: judgmentEvidenceAt.Add(2 * time.Hour),
		FormedAt:     judgmentEvidenceAt.Add(2*time.Hour + time.Minute),
		Bases:        append(spec.Versions[2].Bases, unregistered),
	})
	judgment, err := domain.RehydrateActualCarrierJudgment(spec)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if got := judgment.Current().Bases(); len(got) != 3 || got[2].Material() != "Z Express" {
		t.Fatalf("素材依据没有装回：%+v", got)
	}
}
