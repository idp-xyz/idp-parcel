package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var segmentEnteredAt = commissionSentAt.Add(time.Hour)

// adoptionStoreDouble 照迁移 TF 0035 的主键办：一段至多一份采用，同段第二份答已登记。
type adoptionStoreDouble struct {
	records map[string]ports.SegmentCommissionAdoptionRecord
	findErr error
	saveErr error
	saves   int
}

func newAdoptionStore() *adoptionStoreDouble {
	return &adoptionStoreDouble{records: map[string]ports.SegmentCommissionAdoptionRecord{}}
}

func adoptionStoreKey(key ports.SegmentCommissionAdoptionKey) string {
	return key.TenantID.String() + "|" + key.Segment.String()
}

func (double *adoptionStoreDouble) FindBySegment(
	_ context.Context,
	key ports.SegmentCommissionAdoptionKey,
) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	if double.findErr != nil {
		return ports.SegmentCommissionAdoptionRecord{}, false, double.findErr
	}
	record, found := double.records[adoptionStoreKey(key)]
	return record, found, nil
}

func (double *adoptionStoreDouble) Save(
	_ context.Context,
	record ports.SegmentCommissionAdoptionRecord,
) (ports.AdoptionSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.AdoptionSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.records[adoptionStoreKey(record.Key)]; exists {
		return ports.AdoptionAlreadyRegistered, nil
	}
	double.records[adoptionStoreKey(record.Key)] = record
	return ports.AdoptionSaved, nil
}

// adoptedSegmentViewDouble 只认登记过的（租户，段）。
type adoptedSegmentViewDouble struct {
	segments map[string]domain.ActualFulfillmentSegment
	findErr  error
}

func (double *adoptedSegmentViewDouble) FindByKey(
	_ context.Context,
	key ports.FulfillmentSegmentKey,
) (ports.FulfillmentSegmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.FulfillmentSegmentRecord{}, false, double.findErr
	}
	segment, found := double.segments[key.TenantID.String()+"|"+key.Segment.String()]
	if !found {
		return ports.FulfillmentSegmentRecord{}, false, nil
	}
	return ports.FulfillmentSegmentRecord{Key: key, Segment: segment}, true, nil
}

// adoptionRacingCommissionStore 在 Replace 那一刻先让另一笔落定（race 改写在册那一份），再照常按「既未开始也未取消」判这一笔落不落；
// AdvanceStart 同样先跑 advanceRace，再照迁移里那条 WHERE 判：只写已开始、未取消、在册开始晚于这一次的行。
type adoptionRacingCommissionStore struct {
	*commissionStoreDouble
	race        func(ports.TransportCommissionRecord) ports.TransportCommissionRecord
	advanceRace func(ports.TransportCommissionRecord) ports.TransportCommissionRecord
	replaceErr  error
	advances    int
}

func (double *adoptionRacingCommissionStore) AdvanceStart(_ context.Context, record ports.TransportCommissionRecord) (bool, error) {
	double.advances++
	key := commissionStoreKey(record.Key)
	if double.advanceRace != nil {
		double.records[key] = double.advanceRace(double.records[key])
		double.advanceRace = nil
	}
	stored, exists := double.records[key]
	if !exists {
		return false, nil
	}
	_, storedAt, started := stored.Commission.TransportStarted()
	_, at, _ := record.Commission.TransportStarted()
	if _, cancelled := stored.Commission.Cancelled(); cancelled || !started || !storedAt.After(at) {
		return false, nil
	}
	double.records[key] = record
	return true, nil
}

func (double *adoptionRacingCommissionStore) Replace(ctx context.Context, record ports.TransportCommissionRecord) (bool, error) {
	if double.replaceErr != nil {
		return false, double.replaceErr
	}
	key := commissionStoreKey(record.Key)
	if double.race != nil {
		double.records[key] = double.race(double.records[key])
		double.race = nil
	}
	stored := double.records[key]
	if _, _, started := stored.Commission.TransportStarted(); started {
		return false, nil
	}
	if _, cancelled := stored.Commission.Cancelled(); cancelled {
		return false, nil
	}
	return double.commissionStoreDouble.Replace(ctx, record)
}

func adoptableSegment(t *testing.T, segment string, enteredAt time.Time) domain.ActualFulfillmentSegment {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	segmentRef, _ := domain.NewFulfillmentSegmentReference(segment)
	object, _ := domain.NewCarriedObjectReference("parcel-1")
	basis, _ := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/" + segment + "/parcel-1/v1")
	rebuilt, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant,
		Segment:  segmentRef,
		Participations: []domain.RehydrateParticipationSpec{{
			Object:     object,
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: basis,
			EnteredAt:  enteredAt,
		}},
	})
	if err != nil {
		t.Fatalf("重建段：%v", err)
	}
	return rebuilt
}

type commissionAdoptionFixture struct {
	*commissionFixture
	racing    *adoptionRacingCommissionStore
	adoptions *adoptionStoreDouble
	segments  *adoptedSegmentViewDouble
	adopter   *application.AdoptSegmentCommissionHandler
}

// newCommissionAdoptionFixture 交回一份已提交的委托（服务方 partner-1）与两个在册的段：SEG-1、SEG-2 都在委托提交之后开始。
func newCommissionAdoptionFixture(t *testing.T) *commissionAdoptionFixture {
	t.Helper()
	base := newCommissionFixture(t)
	if result, err := base.handler.SubmitCommission(context.Background(), submitCommissionCommand(t)); err != nil || result.Outcome() != application.CommissionSubmitted {
		t.Fatalf("submit commission: %q %v", result.Outcome(), err)
	}
	fixture := &commissionAdoptionFixture{
		commissionFixture: base,
		racing:            &adoptionRacingCommissionStore{commissionStoreDouble: base.commissions},
		adoptions:         newAdoptionStore(),
		segments: &adoptedSegmentViewDouble{segments: map[string]domain.ActualFulfillmentSegment{
			"tenant-1|SEG-1": adoptableSegment(t, "SEG-1", segmentEnteredAt),
			"tenant-1|SEG-2": adoptableSegment(t, "SEG-2", segmentEnteredAt.Add(time.Hour)),
		}},
	}
	fixture.adopter = application.NewAdoptSegmentCommissionHandler(application.AdoptSegmentCommissionDeps{
		Adoptions:        fixture.adoptions,
		Segments:         fixture.segments,
		Commissions:      fixture.racing,
		StartAdvancement: fixture.racing,
		Clock:            commissionClock{at: commissionNowAt},
	})
	return fixture
}

func adoptCommand(segment, commission, provider string) application.AdoptSegmentCommissionCommand {
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.AdoptSegmentCommissionCommand{TenantID: tenant, Segment: segment, Commission: commission, Provider: provider}
}

func (fixture *commissionAdoptionFixture) storedCommission(t *testing.T) ports.TransportCommissionRecord {
	t.Helper()
	key := ports.TransportCommissionKey{}
	key.TenantID, _ = domain.NewTenantID("tenant-1")
	key.Commission, _ = domain.NewTransportCommissionReference("commission-1")
	return fixture.commissions.records[commissionStoreKey(key)]
}

// Covers: `AT-TF-181`、`AT-TF-182`（ADR-0204 决定一、二、三）——段采用委托答采用成立，委托随之以段的首个参与标为已开始（不取
// 登记时刻）；此后取消委托答运输已开始（AT-TF-092），原样再交答已有采用，同段换一份委托答采用冲突并回显在册那一份；另一段
// 采用同一份已开始的委托照常成立；它开始得更晚就不动委托的开始，开始得更早就把委托的开始前移到它（不回退为未开始）。
func TestAdoptingACommissionStartsItAndCancellationThenAnswersAlreadyStarted(t *testing.T) {
	fixture := newCommissionAdoptionFixture(t)
	result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if result.Outcome() != application.CommissionAdopted {
		t.Fatalf("outcome = %q, want COMMISSION_ADOPTED", result.Outcome())
	}
	commission, present := result.Commission()
	if !present {
		t.Fatal("采用成立没有交回委托")
	}
	basis, at, started := commission.Commission.TransportStarted()
	if !started || basis.String() != "TRANSPORT-HANDOVER/SEG-1/parcel-1/v1" || !at.Equal(segmentEnteredAt) {
		t.Fatalf("委托的开始 = (%s, %s, %v)，want 段的首个参与", basis, at, started)
	}
	if _, _, started := fixture.storedCommission(t).Commission.TransportStarted(); !started {
		t.Fatal("委托册里的委托没有标为开始")
	}

	cancel, err := fixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
		TenantID: commission.Key.TenantID, Commission: "commission-1", CancelledAt: commissionNowAt,
	})
	if err != nil || cancel.Outcome().String() != "TRANSPORT_ALREADY_STARTED" {
		t.Fatalf("采用之后取消委托 = %q %v，want TRANSPORT_ALREADY_STARTED", cancel.Outcome(), err)
	}

	replay, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
	if err != nil || replay.Outcome() != application.CommissionAdoptionExisting {
		t.Fatalf("原样再交 = %q %v，want EXISTING_ADOPTION", replay.Outcome(), err)
	}

	if result, err := fixture.handler.SubmitCommission(context.Background(), func() application.SubmitCommissionCommand {
		command := submitCommissionCommand(t)
		command.Commission = "commission-2"
		return command
	}()); err != nil || result.Outcome() != application.CommissionSubmitted {
		t.Fatalf("submit second commission: %q %v", result.Outcome(), err)
	}
	conflict, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-2", "partner-1"))
	if err != nil || conflict.Outcome() != application.CommissionAdoptionConflict {
		t.Fatalf("同段换委托 = %q %v，want ADOPTION_CONFLICT", conflict.Outcome(), err)
	}
	if stored, present := conflict.Adoption(); !present || stored.Adoption.Commission().String() != "commission-1" {
		t.Fatal("采用冲突没有回显段上在册的那一份")
	}

	second, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-2", "commission-1", "partner-1"))
	if err != nil || second.Outcome() != application.CommissionAdopted {
		t.Fatalf("另一段采用已开始的委托 = %q %v", second.Outcome(), err)
	}
	if basis, at, _ := fixture.storedCommission(t).Commission.TransportStarted(); basis.String() != "TRANSPORT-HANDOVER/SEG-1/parcel-1/v1" || !at.Equal(segmentEnteredAt) {
		t.Fatalf("后到的段改写了委托的首次开始：(%s, %s)", basis, at)
	}
	adoption, _ := second.Adoption()
	if basis, _ := adoption.Adoption.Started(); basis.String() != "TRANSPORT-HANDOVER/SEG-2/parcel-1/v1" {
		t.Fatal("后到那一段的连线没带自己的开始依据")
	}

	fixture.segments.segments["tenant-1|SEG-0"] = adoptableSegment(t, "SEG-0", segmentEnteredAt.Add(-30*time.Minute))
	earlier, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-0", "commission-1", "partner-1"))
	if err != nil || earlier.Outcome() != application.CommissionAdopted {
		t.Fatalf("开始得更早的段后采用 = %q %v", earlier.Outcome(), err)
	}
	stored := fixture.storedCommission(t)
	if basis, at, _ := stored.Commission.TransportStarted(); basis.String() != "TRANSPORT-HANDOVER/SEG-0/parcel-1/v1" || !at.Equal(segmentEnteredAt.Add(-30*time.Minute)) {
		t.Fatalf("委托的开始没有前移到更早的那一段：(%s, %s)", basis, at)
	}
	if echoed, _ := earlier.Commission(); !echoed.RecordedAt.Equal(stored.RecordedAt) {
		t.Fatal("采用成立回显的委托不是前移之后在册的那一份")
	}
	if fixture.racing.advances != 1 {
		t.Fatalf("前移写口调了 %d 次，want 1", fixture.racing.advances)
	}
}

// Covers: `AT-TF-183`、`AT-TF-184`、`AT-TF-187`（ADR-0204 决定一、三）——段不在册、委托不在册、服务方不符（回显委托的服务提供
// 方）、委托在段开始之前已取消、委托在段开始之后才登记取消（回显在册的取消与段的首个参与）、段早于委托提交就已开始、声明
// 不成形各答一格，都零写入、委托不标开始。
func TestAnAdoptionAnswersEachUnmetPreconditionWithoutWriting(t *testing.T) {
	for name, tc := range map[string]struct {
		command application.AdoptSegmentCommissionCommand
		prepare func(t *testing.T, fixture *commissionAdoptionFixture)
		want    application.CommissionAdoptionOutcome
	}{
		"segment not registered":    {command: adoptCommand("SEG-9", "commission-1", "partner-1"), want: application.CommissionAdoptionSegmentNotFound},
		"commission not registered": {command: adoptCommand("SEG-1", "commission-9", "partner-1"), want: application.CommissionAdoptionCommissionNotFound},
		"provider mismatch":         {command: adoptCommand("SEG-1", "commission-1", "partner-9"), want: application.CommissionAdoptionProviderMismatch},
		"blank segment":             {command: adoptCommand(" ", "commission-1", "partner-1"), want: application.CommissionAdoptionNotAccepted},
		"commission cancelled": {
			command: adoptCommand("SEG-1", "commission-1", "partner-1"),
			prepare: func(t *testing.T, fixture *commissionAdoptionFixture) {
				tenant, _ := domain.NewTenantID("tenant-1")
				if result, err := fixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
					TenantID: tenant, Commission: "commission-1", CancelledAt: commissionSentAt.Add(10 * time.Minute),
				}); err != nil || result.Outcome() != application.CommissionCancelled {
					t.Fatalf("cancel: %q %v", result.Outcome(), err)
				}
			},
			want: application.CommissionNotAdoptable,
		},
		"commission cancelled after the segment started": {
			command: adoptCommand("SEG-1", "commission-1", "partner-1"),
			prepare: func(t *testing.T, fixture *commissionAdoptionFixture) {
				tenant, _ := domain.NewTenantID("tenant-1")
				if result, err := fixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
					TenantID: tenant, Commission: "commission-1", CancelledAt: segmentEnteredAt.Add(10 * time.Minute),
				}); err != nil || result.Outcome() != application.CommissionCancelled {
					t.Fatalf("采用登记之前的取消：%q %v（这一格今天照常成立，ADR-0204 越权风险点）", result.Outcome(), err)
				}
			},
			want: application.CommissionAdoptionCancelledAfterStart,
		},
		"segment started before the commission": {
			command: adoptCommand("SEG-EARLY", "commission-1", "partner-1"),
			prepare: func(t *testing.T, fixture *commissionAdoptionFixture) {
				fixture.segments.segments["tenant-1|SEG-EARLY"] = adoptableSegment(t, "SEG-EARLY", commissionSentAt.Add(-time.Minute))
			},
			want: application.CommissionAdoptionSegmentStartedBeforeCommission,
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCommissionAdoptionFixture(t)
			if tc.prepare != nil {
				tc.prepare(t, fixture)
			}
			result, err := fixture.adopter.Adopt(context.Background(), tc.command)
			if err != nil || result.Outcome() != tc.want {
				t.Fatalf("outcome = %q %v, want %q", result.Outcome(), err, tc.want)
			}
			if fixture.adoptions.saves != 0 {
				t.Fatal("没成立的采用落了连线")
			}
			if _, _, started := fixture.storedCommission(t).Commission.TransportStarted(); started {
				t.Fatal("没成立的采用把委托标成了开始")
			}
			provider, named := result.CommissionProvider()
			if named != (tc.want == application.CommissionAdoptionProviderMismatch) || (named && provider.String() != "partner-1") {
				t.Fatalf("服务方不符的回显 = (%s, %v)", provider, named)
			}
			basis, startedAt, echoed := result.SegmentStart()
			commission, withCommission := result.Commission()
			if tc.want == application.CommissionAdoptionCancelledAfterStart {
				cancelledAt, cancelled := commission.Commission.Cancelled()
				if !echoed || basis.String() != "TRANSPORT-HANDOVER/SEG-1/parcel-1/v1" || !startedAt.Equal(segmentEnteredAt) ||
					!withCommission || !cancelled || !cancelledAt.Equal(segmentEnteredAt.Add(10*time.Minute)) {
					t.Fatalf("取消晚于开始的回显 = 段 (%s, %s, %v)，委托 %v", basis, startedAt, echoed, withCommission)
				}
			} else if echoed || withCommission {
				t.Fatal("别的格回显了段的开始或委托")
			}
		})
	}
}

// Covers: ADR-0029 在采用编排的恢复面——采用册、段册、委托册读不动与标开始写不动各答未决并指名等谁；并发里另一笔先取消了
// 委托答委托不可采用（连线已落，由装配整笔回滚撤掉），先开始了委托则照常采用、不改写那一次开始；同段并发首交的输家按摘要分
// 已有与冲突。
func TestAdoptionRecoveryAndConcurrency(t *testing.T) {
	t.Run("store failures are undecided with their reasons", func(t *testing.T) {
		for want, break_ := range map[application.CommissionAdoptionUndecidedReason]func(*commissionAdoptionFixture){
			application.CommissionAdoptionStoreUnavailable:           func(f *commissionAdoptionFixture) { f.adoptions.findErr = errors.New("down") },
			application.CommissionAdoptionSegmentStoreUnavailable:    func(f *commissionAdoptionFixture) { f.segments.findErr = errors.New("down") },
			application.CommissionAdoptionCommissionStoreUnavailable: func(f *commissionAdoptionFixture) { f.racing.replaceErr = errors.New("down") },
		} {
			fixture := newCommissionAdoptionFixture(t)
			break_(fixture)
			result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
			if err != nil || result.Outcome() != application.CommissionAdoptionUndecided || result.UndecidedReason() != want || result.ContinuationReference() == "" {
				t.Fatalf("%s: outcome = %q / %q %v", want, result.Outcome(), result.UndecidedReason(), err)
			}
		}
	})

	t.Run("a cancellation dated after the segment started that lands first is answered as cancelled after start", func(t *testing.T) {
		fixture := newCommissionAdoptionFixture(t)
		fixture.racing.race = func(record ports.TransportCommissionRecord) ports.TransportCommissionRecord {
			cancelled, err := record.Commission.Cancel(segmentEnteredAt.Add(5 * time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			record.Commission = cancelled
			return record
		}
		result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
		if err != nil || result.Outcome() != application.CommissionAdoptionCancelledAfterStart {
			t.Fatalf("outcome = %q %v, want COMMISSION_CANCELLED_AFTER_START", result.Outcome(), err)
		}
		if _, _, echoed := result.SegmentStart(); !echoed {
			t.Fatal("并发窗口里的取消晚于开始没有回显段的首个参与")
		}
	})

	t.Run("a segment adopted concurrently that started later is overtaken by this earlier start", func(t *testing.T) {
		fixture := newCommissionAdoptionFixture(t)
		fixture.racing.race = func(record ports.TransportCommissionRecord) ports.TransportCommissionRecord {
			basis, _ := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/SEG-LATE/parcel-1/v1")
			started, err := record.Commission.MarkTransportStarted(basis, segmentEnteredAt.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			record.Commission = started
			return record
		}
		result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
		if err != nil || result.Outcome() != application.CommissionAdopted {
			t.Fatalf("outcome = %q %v, want COMMISSION_ADOPTED", result.Outcome(), err)
		}
		if basis, at, _ := fixture.storedCommission(t).Commission.TransportStarted(); basis.String() != "TRANSPORT-HANDOVER/SEG-1/parcel-1/v1" || !at.Equal(segmentEnteredAt) {
			t.Fatalf("并发里先落的更晚开始没有被这一段前移：(%s, %s)", basis, at)
		}
	})

	t.Run("an advance overtaken by an even earlier one keeps the earlier start", func(t *testing.T) {
		fixture := newCommissionAdoptionFixture(t)
		basis, _ := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/SEG-LATE/parcel-1/v1")
		key := ports.TransportCommissionKey{}
		key.TenantID, _ = domain.NewTenantID("tenant-1")
		key.Commission, _ = domain.NewTransportCommissionReference("commission-1")
		unstarted := fixture.commissions.records[commissionStoreKey(key)]
		started, err := unstarted.Commission.MarkTransportStarted(basis, segmentEnteredAt.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		record := unstarted
		record.Commission = started
		fixture.commissions.records[commissionStoreKey(key)] = record
		fixture.racing.advanceRace = func(record ports.TransportCommissionRecord) ports.TransportCommissionRecord {
			earliest, _ := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/SEG-EARLIEST/parcel-1/v1")
			startedEarliest, err := unstarted.Commission.MarkTransportStarted(earliest, segmentEnteredAt.Add(-time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			record.Commission = startedEarliest
			return record
		}
		result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
		if err != nil || result.Outcome() != application.CommissionAdopted {
			t.Fatalf("outcome = %q %v, want COMMISSION_ADOPTED", result.Outcome(), err)
		}
		if basis, _, _ := fixture.storedCommission(t).Commission.TransportStarted(); basis.String() != "TRANSPORT-HANDOVER/SEG-EARLIEST/parcel-1/v1" {
			t.Fatalf("更早的那一次前移被这一段改写成 %s", basis)
		}
		if fixture.racing.advances != 1 {
			t.Fatalf("前移写口调了 %d 次，want 1", fixture.racing.advances)
		}
	})

	t.Run("a cancellation that lands first makes the commission not adoptable", func(t *testing.T) {
		fixture := newCommissionAdoptionFixture(t)
		fixture.racing.race = func(record ports.TransportCommissionRecord) ports.TransportCommissionRecord {
			cancelled, err := record.Commission.Cancel(commissionSentAt.Add(5 * time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			record.Commission = cancelled
			return record
		}
		result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
		if err != nil || result.Outcome() != application.CommissionNotAdoptable {
			t.Fatalf("outcome = %q %v, want COMMISSION_NOT_ADOPTABLE", result.Outcome(), err)
		}
	})

	t.Run("another segment that starts the commission first keeps its start", func(t *testing.T) {
		fixture := newCommissionAdoptionFixture(t)
		fixture.racing.race = func(record ports.TransportCommissionRecord) ports.TransportCommissionRecord {
			basis, _ := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/SEG-OTHER/parcel-1/v1")
			started, err := record.Commission.MarkTransportStarted(basis, segmentEnteredAt.Add(-30*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			record.Commission = started
			return record
		}
		result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
		if err != nil || result.Outcome() != application.CommissionAdopted {
			t.Fatalf("outcome = %q %v, want COMMISSION_ADOPTED", result.Outcome(), err)
		}
		if basis, _, _ := fixture.storedCommission(t).Commission.TransportStarted(); basis.String() != "TRANSPORT-HANDOVER/SEG-OTHER/parcel-1/v1" {
			t.Fatalf("先开始的那一次被改写成 %s", basis)
		}
	})

	t.Run("the loser of a concurrent first adoption answers by digest", func(t *testing.T) {
		fixture := newCommissionAdoptionFixture(t)
		winnerDigest, _ := domain.CanonicalizeSegmentCommissionAdoptionPayload("commission-2", "partner-1")
		tenant, _ := domain.NewTenantID("tenant-1")
		segment, _ := domain.NewFulfillmentSegmentReference("SEG-1")
		key := ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: segment}
		commission, _ := domain.NewTransportCommissionReference("commission-2")
		provider, _ := domain.NewServiceProviderReference("partner-1")
		basis, _ := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/SEG-1/parcel-1/v1")
		winner, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
			TenantID: tenant, Segment: segment, Commission: commission, Provider: provider, StartBasis: basis, StartedAt: segmentEnteredAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		lateWinner := &lateAdoptionStore{adoptionStoreDouble: fixture.adoptions, winner: ports.SegmentCommissionAdoptionRecord{Key: key, ContentDigest: winnerDigest, Adoption: winner}}
		adopter := application.NewAdoptSegmentCommissionHandler(application.AdoptSegmentCommissionDeps{
			Adoptions: lateWinner, Segments: fixture.segments, Commissions: fixture.racing, Clock: commissionClock{at: commissionNowAt},
		})
		result, err := adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
		if err != nil || result.Outcome() != application.CommissionAdoptionConflict {
			t.Fatalf("outcome = %q %v, want ADOPTION_CONFLICT", result.Outcome(), err)
		}
		if _, _, started := fixture.storedCommission(t).Commission.TransportStarted(); started {
			t.Fatal("输家没落连线，却把委托标成了开始")
		}
	})
}

// lateAdoptionStore 让第一次读答空、落库那一刻赢家已先落：模拟同段两份采用并发首交。
type lateAdoptionStore struct {
	*adoptionStoreDouble
	winner ports.SegmentCommissionAdoptionRecord
	landed bool
}

func (double *lateAdoptionStore) Save(_ context.Context, _ ports.SegmentCommissionAdoptionRecord) (ports.AdoptionSaveOutcome, error) {
	double.records[adoptionStoreKey(double.winner.Key)] = double.winner
	double.landed = true
	return ports.AdoptionAlreadyRegistered, nil
}

// Covers: `AT-TF-189`（ADR-0204 决定二、三）——一份已开始的委托再被一个首个参与更早的段采用，开始前移到它；一次申请时刻落在
// 新旧两个开始之间的订舱，前移之前读作开始之前提出、取消答运输已开始，前移之后读作开始之后为剩余范围提出、取消成立。
func TestAnEarlierSegmentAdvancesTheStartAndReleasesBookingsForTheRemainingScope(t *testing.T) {
	fixture := newCommissionAdoptionFixture(t)
	if result, err := fixture.handler.SubmitBooking(context.Background(), submitBookingCommand(t)); err != nil || result.Outcome() != application.BookingSubmitted {
		t.Fatalf("submit booking: %q %v", result.Outcome(), err)
	}
	if result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1")); err != nil || result.Outcome() != application.CommissionAdopted {
		t.Fatalf("采用 SEG-1 = %q %v", result.Outcome(), err)
	}
	if !bookingSentAt.Before(segmentEnteredAt) {
		t.Fatal("夹具前提：订舱要在 SEG-1 开始之前提出")
	}
	if result, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", commissionNowAt)); err != nil || result.Outcome() != application.CommissionTransportStarted {
		t.Fatalf("前移之前取消订舱 = %q %v，want TRANSPORT_ALREADY_STARTED", result.Outcome(), err)
	}

	earliestAt := bookingSentAt.Add(-10 * time.Minute)
	fixture.segments.segments["tenant-1|SEG-EARLIEST"] = adoptableSegment(t, "SEG-EARLIEST", earliestAt)
	if result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-EARLIEST", "commission-1", "partner-1")); err != nil || result.Outcome() != application.CommissionAdopted {
		t.Fatalf("采用 SEG-EARLIEST = %q %v", result.Outcome(), err)
	}
	if basis, at, started := fixture.storedCommission(t).Commission.TransportStarted(); !started || basis.String() != "TRANSPORT-HANDOVER/SEG-EARLIEST/parcel-1/v1" || !at.Equal(earliestAt) {
		t.Fatalf("委托的开始 = (%s, %s, %v)，want 前移到 SEG-EARLIEST", basis, at, started)
	}
	if result, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", commissionNowAt)); err != nil || result.Outcome() != application.BookingCancelled {
		t.Fatalf("前移之后取消订舱 = %q %v，want BOOKING_CANCELLED", result.Outcome(), err)
	}
}
