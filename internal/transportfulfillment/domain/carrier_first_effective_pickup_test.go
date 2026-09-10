package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证「实际承运商首次有效收寄」聚合的三道门（ADR-0135 决定二、四、六）：已形成必须带在册承运主体、
// 业务时间与至少一条依据；待确认必须带原因且不带承运主体；替代 / 失效版本只回指当前版、沿用原版本号即拒；
// 链尾失效后再次形成从失效版本长出新版本。夹具全为合成引用（S 级）。

var (
	carrierPickupOccurredAt = time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	carrierPickupJudgedAt   = time.Date(2026, 9, 10, 9, 5, 0, 0, time.UTC)
)

func carrierPickupBasis(t *testing.T, reference, version string) domain.CarrierPickupBasis {
	t.Helper()
	basis, err := domain.NewCarrierPickupBasis(domain.TrustedChannelCallback, reference, version)
	if err != nil {
		t.Fatalf("形成依据：%v", err)
	}
	return basis
}

func externalCarrierSubject(t *testing.T, reference string) domain.CarrierSubject {
	t.Helper()
	subject, err := domain.NewCarrierSubject(domain.ExternalCarrierParty, reference)
	if err != nil {
		t.Fatalf("形成承运主体：%v", err)
	}
	return subject
}

func formedCarrierPickupSpec(t *testing.T) domain.CarrierFirstEffectivePickupSpec {
	t.Helper()
	return domain.CarrierFirstEffectivePickupSpec{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Object:     mustValue(t, domain.NewCarriedObjectReference, "PCL-1"),
		Fact:       mustValue(t, domain.NewCarrierFirstEffectivePickupReference, "CFEP-1"),
		Version:    mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-1"),
		Carrier:    externalCarrierSubject(t, "party-x"),
		OccurredAt: carrierPickupOccurredAt,
		JudgedAt:   carrierPickupJudgedAt,
		Bases:      []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-1")},
	}
}

func formedCarrierPickup(t *testing.T) domain.CarrierFirstEffectivePickup {
	t.Helper()
	pickup, err := domain.FormCarrierFirstEffectivePickup(formedCarrierPickupSpec(t))
	if err != nil {
		t.Fatalf("形成收寄：%v", err)
	}
	return pickup
}

// Covers: ADR-0135 决定二——事实固定五件；已形成的首登版本没有前版、不失效、带在册承运主体与业务时间。
func TestAFormedFirstPickupCarriesItsFiveThingsAndNoPredecessor(t *testing.T) {
	pickup := formedCarrierPickup(t)
	if pickup.Result() != domain.CarrierPickupFormed || !pickup.Formed() {
		t.Fatalf("结果 = %s，期望已形成", pickup.Result())
	}
	carrier, identified := pickup.Carrier()
	if !identified || carrier.Reference() != "party-x" {
		t.Fatalf("承运主体没有原样带回：%v %v", carrier, identified)
	}
	occurredAt, present := pickup.OccurredAt()
	if !present || !occurredAt.Equal(carrierPickupOccurredAt) {
		t.Fatalf("业务发生时间没有原样带回：%s %v", occurredAt, present)
	}
	if !pickup.JudgedAt().Equal(carrierPickupJudgedAt) {
		t.Fatalf("判断形成时间没有原样带回：%s", pickup.JudgedAt())
	}
	if _, has := pickup.Supersedes(); has {
		t.Fatalf("首登不应回指前版")
	}
	if pickup.Voided() {
		t.Fatalf("首登不应失效")
	}
	if len(pickup.Bases()) != 1 || pickup.Bases()[0].Reference().String() != "EXTF-1" || pickup.Bases()[0].SourceVersion() != "EXTV-1" {
		t.Fatalf("依据没有原样带回：%+v", pickup.Bases())
	}
	if !pickup.BasedOn(pickup.Bases()[0].Reference(), "EXTV-1") || pickup.BasedOn(pickup.Bases()[0].Reference(), "EXTV-2") {
		t.Fatalf("BasedOn 应只认（引用，版本）恰相等的那一条依据")
	}
}

// Covers: ADR-0135 决定二构造门——已形成缺承运主体 / 缺业务时间 / 缺依据 / 缺租户或对象或身份各拒一格。
func TestAFormedPickupRefusesEachMissingThing(t *testing.T) {
	cases := map[string]func(*domain.CarrierFirstEffectivePickupSpec){
		"缺承运主体": func(s *domain.CarrierFirstEffectivePickupSpec) { s.Carrier = domain.CarrierSubject{} },
		"缺业务时间": func(s *domain.CarrierFirstEffectivePickupSpec) { s.OccurredAt = time.Time{} },
		"缺判断时间": func(s *domain.CarrierFirstEffectivePickupSpec) { s.JudgedAt = time.Time{} },
		"缺依据":   func(s *domain.CarrierFirstEffectivePickupSpec) { s.Bases = nil },
		"缺租户":   func(s *domain.CarrierFirstEffectivePickupSpec) { s.TenantID = domain.TenantID{} },
		"缺载运对象": func(s *domain.CarrierFirstEffectivePickupSpec) { s.Object = domain.CarriedObjectReference{} },
		"缺事实身份": func(s *domain.CarrierFirstEffectivePickupSpec) {
			s.Fact = domain.CarrierFirstEffectivePickupReference{}
		},
		"缺版本": func(s *domain.CarrierFirstEffectivePickupSpec) {
			s.Version = domain.CarrierFirstEffectivePickupVersion{}
		},
		"依据重复同一代": func(s *domain.CarrierFirstEffectivePickupSpec) { s.Bases = append(s.Bases, s.Bases[0]) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := formedCarrierPickupSpec(t)
			mutate(&spec)
			if _, err := domain.FormCarrierFirstEffectivePickup(spec); !errors.Is(err, domain.ErrInvalidCarrierFirstEffectivePickup) {
				t.Fatalf("err = %v，期望 ErrInvalidCarrierFirstEffectivePickup", err)
			}
		})
	}
}

// Covers: ADR-0135 决定四——待确认是带原因的版本：原因封闭两支、依据至少一条、不带承运主体与业务时间。
func TestAPendingPickupIsAVersionWithAReasonAndNoCarrier(t *testing.T) {
	spec := domain.PendingCarrierFirstEffectivePickupSpec{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Object:   mustValue(t, domain.NewCarriedObjectReference, "PCL-1"),
		Fact:     mustValue(t, domain.NewCarrierFirstEffectivePickupReference, "CFEP-1"),
		Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-1"),
		Reason:   domain.PickupCarrierIdentityNotRegistered,
		JudgedAt: carrierPickupJudgedAt,
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-1")},
	}
	pending, err := domain.HoldCarrierFirstEffectivePickupPending(spec)
	if err != nil {
		t.Fatalf("形成待确认：%v", err)
	}
	if pending.Result() != domain.CarrierPickupPending || pending.Formed() {
		t.Fatalf("结果 = %s，期望待确认", pending.Result())
	}
	reason, isPending := pending.PendingReason()
	if !isPending || reason != domain.PickupCarrierIdentityNotRegistered {
		t.Fatalf("原因没有带回：%s %v", reason, isPending)
	}
	if _, identified := pending.Carrier(); identified {
		t.Fatalf("待确认不带承运主体")
	}
	if _, present := pending.OccurredAt(); present {
		t.Fatalf("待确认不带业务发生时间")
	}
	for name, mutate := range map[string]func(*domain.PendingCarrierFirstEffectivePickupSpec){
		"缺原因":   func(s *domain.PendingCarrierFirstEffectivePickupSpec) { s.Reason = domain.PendingPickupReasonNone },
		"缺依据":   func(s *domain.PendingCarrierFirstEffectivePickupSpec) { s.Bases = nil },
		"缺判断时间": func(s *domain.PendingCarrierFirstEffectivePickupSpec) { s.JudgedAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			bad := spec
			mutate(&bad)
			if _, err := domain.HoldCarrierFirstEffectivePickupPending(bad); !errors.Is(err, domain.ErrInvalidCarrierFirstEffectivePickup) {
				t.Fatalf("err = %v，期望 ErrInvalidCarrierFirstEffectivePickup", err)
			}
		})
	}
}

// Covers: ADR-0135 决定六——替代版本回指当前版、沿用同一事实身份、原版本不动；沿用原版本号即覆盖，构造期拒绝。
func TestASupersedingVersionChainsToTheCurrentOneAndKeepsTheFactIdentity(t *testing.T) {
	first := formedCarrierPickup(t)
	later := carrierPickupOccurredAt.Add(30 * time.Minute)
	superseding, err := first.Supersede(domain.CarrierPickupSupersession{
		Version:    mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
		Carrier:    externalCarrierSubject(t, "party-x"),
		OccurredAt: later,
		JudgedAt:   carrierPickupJudgedAt.Add(time.Hour),
		Bases:      []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-2")},
	})
	if err != nil {
		t.Fatalf("替代：%v", err)
	}
	if superseding.Fact() != first.Fact() || superseding.Object() != first.Object() {
		t.Fatalf("替代版本换了事实身份或对象")
	}
	prior, has := superseding.Supersedes()
	if !has || prior != first.Version() {
		t.Fatalf("替代版本没有回指前版：%v %v", prior, has)
	}
	if occurredAt, _ := superseding.OccurredAt(); !occurredAt.Equal(later) {
		t.Fatalf("替代版本的业务时间没有随更正：%s", occurredAt)
	}
	if occurredAt, _ := first.OccurredAt(); !occurredAt.Equal(carrierPickupOccurredAt) {
		t.Fatalf("原版本被改写了：%s", occurredAt)
	}
	if _, err := first.Supersede(domain.CarrierPickupSupersession{
		Version:    first.Version(),
		Carrier:    externalCarrierSubject(t, "party-x"),
		OccurredAt: later,
		JudgedAt:   carrierPickupJudgedAt.Add(time.Hour),
		Bases:      []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-2")},
	}); !errors.Is(err, domain.ErrInvalidCarrierFirstEffectivePickup) {
		t.Fatalf("沿用原版本号应拒：%v", err)
	}
}

// Covers: ADR-0135 决定六——已形成 → 失效版本：回指前版、标失效、不带承运主体；链尾失效后再次形成从失效版本
// 长出新版本；待确认与失效版本上没有东西可失效。
func TestAVoidedVersionMarksTheChainTailAndANewFormationGrowsFromIt(t *testing.T) {
	first := formedCarrierPickup(t)
	voided, err := first.Void(domain.CarrierPickupVoiding{
		Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
		JudgedAt: carrierPickupJudgedAt.Add(time.Hour),
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-2")},
	})
	if err != nil {
		t.Fatalf("失效：%v", err)
	}
	if voided.Result() != domain.CarrierPickupVoided || !voided.Voided() || voided.Formed() {
		t.Fatalf("结果 = %s，期望失效", voided.Result())
	}
	if prior, has := voided.Supersedes(); !has || prior != first.Version() {
		t.Fatalf("失效版本没有回指前版")
	}
	if _, identified := voided.Carrier(); identified {
		t.Fatalf("失效版本不带承运主体")
	}
	if _, err := voided.Void(domain.CarrierPickupVoiding{
		Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-3"),
		JudgedAt: carrierPickupJudgedAt.Add(2 * time.Hour),
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-3")},
	}); !errors.Is(err, domain.ErrCarrierPickupNotFormed) {
		t.Fatalf("失效版本上再失效应拒 ErrCarrierPickupNotFormed：%v", err)
	}
	reformed, err := voided.Supersede(domain.CarrierPickupSupersession{
		Version:    mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-3"),
		Carrier:    externalCarrierSubject(t, "party-y"),
		OccurredAt: carrierPickupOccurredAt.Add(time.Hour),
		JudgedAt:   carrierPickupJudgedAt.Add(2 * time.Hour),
		Bases:      []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-9", "EXTV-9")},
	})
	if err != nil {
		t.Fatalf("从失效版本再次形成：%v", err)
	}
	if prior, _ := reformed.Supersedes(); prior != voided.Version() || !reformed.Formed() {
		t.Fatalf("再次形成应回指失效版本并为已形成")
	}
}

// Covers: ADR-0135 决定四生命周期——待确认 → 已形成凭同一依据；待确认 → 待确认（来源冲突）追加依据。
func TestAPendingChainMovesToFormedOrToConflict(t *testing.T) {
	pending, err := domain.HoldCarrierFirstEffectivePickupPending(domain.PendingCarrierFirstEffectivePickupSpec{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Object:   mustValue(t, domain.NewCarriedObjectReference, "PCL-1"),
		Fact:     mustValue(t, domain.NewCarrierFirstEffectivePickupReference, "CFEP-1"),
		Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-1"),
		Reason:   domain.PickupCarrierIdentityNotRegistered,
		JudgedAt: carrierPickupJudgedAt,
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-1")},
	})
	if err != nil {
		t.Fatalf("形成待确认：%v", err)
	}
	conflict, err := pending.HoldPending(domain.CarrierPickupPendingSupersession{
		Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
		Reason:   domain.PickupSourceConflict,
		JudgedAt: carrierPickupJudgedAt.Add(time.Minute),
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-1"), carrierPickupBasis(t, "EXTF-2", "EXTV-1")},
	})
	if err != nil {
		t.Fatalf("待确认 → 来源冲突：%v", err)
	}
	if reason, _ := conflict.PendingReason(); reason != domain.PickupSourceConflict || len(conflict.Bases()) != 2 {
		t.Fatalf("冲突版本应带两条依据与冲突原因：%s %d", reason, len(conflict.Bases()))
	}
	formed, err := conflict.Supersede(domain.CarrierPickupSupersession{
		Version:    mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-3"),
		Carrier:    externalCarrierSubject(t, "party-x"),
		OccurredAt: carrierPickupOccurredAt,
		JudgedAt:   carrierPickupJudgedAt.Add(time.Hour),
		Bases:      []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-1")},
	})
	if err != nil || !formed.Formed() {
		t.Fatalf("待确认 → 已形成：%v", err)
	}
	// 已形成之后不回待确认（ADR-0135 决定六）：另一来源的相反证据进段级判断，不是收寄的更正。
	if _, err := formed.HoldPending(domain.CarrierPickupPendingSupersession{
		Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-4"),
		Reason:   domain.PickupSourceConflict,
		JudgedAt: carrierPickupJudgedAt.Add(2 * time.Hour),
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-3", "EXTV-1")},
	}); !errors.Is(err, domain.ErrCarrierPickupAlreadyFormed) {
		t.Fatalf("已形成回待确认应拒 ErrCarrierPickupAlreadyFormed：%v", err)
	}
}

// Covers: 封闭词表往返——结果三格与待确认原因两支的 String / Parse 互逆，集外词拒。
func TestCarrierPickupClosedSetsRoundTripThroughTheirWords(t *testing.T) {
	for _, result := range []domain.CarrierPickupResult{domain.CarrierPickupFormed, domain.CarrierPickupPending, domain.CarrierPickupVoided} {
		parsed, err := domain.ParseCarrierPickupResult(result.String())
		if err != nil || parsed != result {
			t.Fatalf("结果词往返失败：%s → %v %v", result, parsed, err)
		}
	}
	if _, err := domain.ParseCarrierPickupResult("SOMETHING"); err == nil {
		t.Fatalf("集外结果词应拒")
	}
	for _, reason := range []domain.PendingPickupReason{domain.PickupSourceConflict, domain.PickupCarrierIdentityNotRegistered} {
		parsed, err := domain.ParsePendingPickupReason(reason.String())
		if err != nil || parsed != reason {
			t.Fatalf("原因词往返失败：%s → %v %v", reason, parsed, err)
		}
	}
	if _, err := domain.ParsePendingPickupReason("NO_QUALIFIED_EVIDENCE"); err == nil {
		t.Fatalf("「无合格证据」不是收寄的待确认原因（ADR-0135 决定四），应拒")
	}
	if domain.CarrierPickupResult(9).String() != "" || domain.PendingPickupReason(9).String() != "" {
		t.Fatalf("集外值不应有字串")
	}
}
