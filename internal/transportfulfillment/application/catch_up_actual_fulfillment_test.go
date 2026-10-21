package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证实际履约补形成与实际履约未形成册（ADR-0231 决定一、二、五、六）：采用与换版的门铃把段上已结束的参与逐件交回自动形成的同一个
// 后半；登记答「这一次不形成」时立或刷新册上一条，形成或认出已被盖住时了结。登记用例接的是真用例（替身只在它的读口与册上）。证据只记 S。

// unformedRegisterDouble 照真册的写法：RecordUnformed 立或刷新，两种了结只改册上已有的那一条。
type unformedRegisterDouble struct {
	records  map[ports.ActualFulfillmentUnformedKey]ports.ActualFulfillmentUnformedRecord
	writeErr error
}

func newUnformedRegister() *unformedRegisterDouble {
	return &unformedRegisterDouble{records: map[ports.ActualFulfillmentUnformedKey]ports.ActualFulfillmentUnformedRecord{}}
}

func (double *unformedRegisterDouble) RecordUnformed(_ context.Context, entry ports.ActualFulfillmentUnformedEntry, at time.Time) error {
	if double.writeErr != nil {
		return double.writeErr
	}
	record, existed := double.records[entry.Key]
	if !existed {
		record.FirstAnsweredAt = at
	}
	record.Entry, record.LastAnsweredAt, record.State = entry, at, ports.ActualFulfillmentUnformed
	record.Occurrence, record.NotChargedBy, record.SettledAt = ports.ChargeOccurrenceKey{}, "", time.Time{}
	double.records[entry.Key] = record
	return nil
}

func (double *unformedRegisterDouble) SettleFormed(_ context.Context, key ports.ActualFulfillmentUnformedKey, occurrence ports.ChargeOccurrenceKey, at time.Time) error {
	if double.writeErr != nil {
		return double.writeErr
	}
	if record, existed := double.records[key]; existed {
		record.State, record.Occurrence, record.NotChargedBy, record.SettledAt = ports.ActualFulfillmentUnformedFormed, occurrence, "", at
		double.records[key] = record
	}
	return nil
}

func (double *unformedRegisterDouble) SettleNotCharged(_ context.Context, key ports.ActualFulfillmentUnformedKey, agreement domain.AgreementSnapshotReference, at time.Time) error {
	if double.writeErr != nil {
		return double.writeErr
	}
	if record, existed := double.records[key]; existed {
		record.State, record.NotChargedBy, record.Occurrence, record.SettledAt = ports.ActualFulfillmentUnformedNotCharged, agreement.String(), ports.ChargeOccurrenceKey{}, at
		double.records[key] = record
	}
	return nil
}

func (double *unformedRegisterDouble) SettleInvalidatedByDecision(_ context.Context, key ports.ActualFulfillmentUnformedKey, invalidated ports.ChargeOccurrenceKey, at time.Time) error {
	if double.writeErr != nil {
		return double.writeErr
	}
	if record, existed := double.records[key]; existed {
		record.State, record.Occurrence, record.NotChargedBy, record.SettledAt = ports.ActualFulfillmentUnformedInvalidatedByDecision, invalidated, "", at
		double.records[key] = record
	}
	return nil
}

func (double *unformedRegisterDouble) entry(t *testing.T, object string) (ports.ActualFulfillmentUnformedRecord, bool) {
	t.Helper()
	record, found := double.records[ports.ActualFulfillmentUnformedKey{
		TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Segment:  occurrenceValue(t, domain.NewFulfillmentSegmentReference, trunkSegment),
		Object:   occurrenceValue(t, domain.NewCarriedObjectReference, object),
	}]
	return record, found
}

type adoptionsByCommissionDouble struct {
	records []ports.SegmentCommissionAdoptionRecord
	err     error
}

func (double adoptionsByCommissionDouble) ListByCommission(_ context.Context, _ ports.TransportCommissionKey) ([]ports.SegmentCommissionAdoptionRecord, error) {
	return double.records, double.err
}

// catchUpRig 立一台补形成夹具：干线段已采用委托，段上的参与由 rows 给。
func catchUpRig(t *testing.T, commission ports.TransportCommissionRecord, rows ...domain.RehydrateParticipationSpec) (*formationRig, *occurrenceRig) {
	t.Helper()
	occurrences := adoptedTrunkRig(t, commission)
	occurrences.segments = trunkSegmentWith(t, rows...)
	rig := newFormationRig(t, occurrences)
	for _, adoption := range occurrences.adoptions.records {
		rig.byCommission.records = append(rig.byCommission.records, adoption)
	}
	return rig, occurrences
}

func (rig *formationRig) onAdoption(t *testing.T) application.CatchUpActualFulfillmentResult {
	t.Helper()
	result, err := rig.handler().CatchUpOnAdoption(t.Context(), application.CatchUpActualFulfillmentOnAdoptionCommand{
		TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Segment:  trunkSegment,
	})
	if err != nil {
		t.Fatalf("采用门铃不该上抛：%v", err)
	}
	return result
}

func (rig *formationRig) onRevision(t *testing.T, revision int) application.CatchUpActualFulfillmentResult {
	t.Helper()
	result, err := rig.handler().CatchUpOnRevision(t.Context(), application.CatchUpActualFulfillmentOnRevisionCommand{
		TenantID:   occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Commission: bkCommission,
		Revision:   revision,
	})
	if err != nil {
		t.Fatalf("换版门铃不该上抛：%v", err)
	}
	return result
}

func terminatedRow(t *testing.T, object string) domain.RehydrateParticipationSpec {
	t.Helper()
	row := participationRow(t, object, false)
	row.EndKind = domain.EndedByControlTermination
	row.EndBasis = occurrenceValue(t, domain.NewParticipationBasisReference, "CONTROL-TERMINATION/SYN-TERM-"+object)
	row.EndedAt = occurrenceEndedAt
	return row
}

// Covers: AT-TF-300——采用晚到：采用门铃对段上以下一次交接或有效交付结束的参与逐件交回自动形成的同一个后半，各成一项、成员各一件、登记者
// 是系统身份；仍在场的、以明确终止结束的不补。同一封门铃重投（或采用之后某件的交接信封才到）落回同一身份，答已存在、不多一项。
func TestAnAdoptionDoorbellFormsTheParticipationsThatEndedBeforeIt(t *testing.T) {
	rig, occurrences := catchUpRig(t, adoptedCommission(t),
		participationRow(t, trunkParcel, true), participationRow(t, trunkSecondParcel, true),
		participationRow(t, "SYN-PARCEL-STILL-ON-BOARD", false), terminatedRow(t, "SYN-PARCEL-TERMINATED"))

	result := rig.onAdoption(t)
	if result.Outcome() != application.CatchUpAnswered || result.Disposition() != application.FormationSettled {
		t.Fatalf("门铃 = %s/%s", result.Outcome(), result.Disposition())
	}
	formations := result.Formations()
	if len(formations) != 2 || len(rig.spy.commands) != 2 {
		t.Fatalf("补了 %d 件、交登记 %d 次, want 只补两件以交接结束的", len(formations), len(rig.spy.commands))
	}
	for index, object := range []string{trunkParcel, trunkSecondParcel} {
		occurrence := registeredOccurrence(t, formations[index].Registration())
		if members := occurrence.Members(); len(members) != 1 || members[0].String() != object ||
			occurrence.Condition().RegisteredBy().String() != application.ActualFulfillmentFormationRegistrant ||
			occurrence.Journey().String() != "JRN/"+object {
			t.Fatalf("第 %d 件 = %+v", index, occurrence)
		}
	}

	again := rig.onAdoption(t)
	if again.Disposition() != application.FormationSettled || occurrences.registry.saves != 2 {
		t.Fatalf("重投 = %s，册上 %d 项, want 仍 2 项", again.Disposition(), occurrences.registry.saves)
	}
	for _, formed := range again.Formations() {
		if formed.Registration().Outcome() != application.ChargeOccurrenceExisting {
			t.Fatalf("重投那一件 = %s, want EXISTING", formed.Registration().Outcome())
		}
	}
	if len(rig.unformed.records) != 0 {
		t.Fatalf("顺利形成的进了未形成册：%+v", rig.unformed.records)
	}
}

// Covers: AT-TF-301——协议后补条件：委托那一版没声明实际履约的条件，自动形成答未配置、册上立一条（委托、第 1 版、问的那一版、参与终点）；
// 委托换版到声明了条件的后继，换版门铃按后继补形成，册上那一条了结为已形成、记下那一项，不再上列。
func TestARevisionDoorbellFormsWhatTheEarlierVersionLeftUnconfigured(t *testing.T) {
	rig, occurrences := catchUpRig(t, adoptedCommission(t), participationRow(t, trunkParcel, true))
	occurrences.agreements.check.ChargeConditions = declaredConditions(t,
		declaredCondition{domain.FailedAttemptOccurrence, true, domain.PerAttemptQuantity})

	first := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
	if first.Registration().Outcome() != application.ChargeOccurrenceNotConfigured || first.Disposition() != application.FormationSettled {
		t.Fatalf("先前那一版 = %s/%s", first.Registration().Outcome(), first.Disposition())
	}
	entry, found := rig.unformed.entry(t, trunkParcel)
	if !found || entry.State != ports.ActualFulfillmentUnformed || entry.Entry.Reason != ports.UnformedNotConfigured ||
		entry.Entry.Commission.String() != bkCommission || entry.Entry.Revision != 1 ||
		entry.Entry.Agreement.Version() != trunkAgreementTag || !entry.Entry.EndedAt.Equal(occurrenceEndedAt) ||
		!entry.FirstAnsweredAt.Equal(occurrenceNow) {
		t.Fatalf("未形成那一条 = %+v %v", entry, found)
	}

	revised := revisedTrunkCommission(t, occurrenceEnteredAt)
	occurrences.commissions.records[revised.Key.Commission] = revised
	occurrences.agreements.check.Snapshot = occurrenceValue(t, domain.NewAgreementSnapshotReference, revisedSnapshot)
	occurrences.agreements.check.ChargeConditions = declaredConditions(t,
		declaredCondition{domain.ActualFulfillmentOccurrence, true, domain.PerPieceQuantity})
	rig.spy.inner = occurrences.handler()

	result := rig.onRevision(t, 2)
	if result.Disposition() != application.FormationSettled || len(result.Formations()) != 1 {
		t.Fatalf("换版门铃 = %s，%d 件", result.Disposition(), len(result.Formations()))
	}
	occurrence := registeredOccurrence(t, result.Formations()[0].Registration())
	if occurrence.Agreement().String() != revisedSnapshot {
		t.Fatalf("补形成按 %s, want 后继", occurrence.Agreement())
	}
	entry, _ = rig.unformed.entry(t, trunkParcel)
	if entry.State != ports.ActualFulfillmentUnformedFormed || entry.Occurrence.Occurrence != occurrence.Occurrence() ||
		entry.Entry.Reason != ports.UnformedNotConfigured {
		t.Fatalf("了结之后 = %+v", entry)
	}
}

// Covers: AT-TF-302——收尾造成的两格（ADR-0233）：委托那一版在参与终点已终止或未生效，册上立一条并记下那一格；同一件此前已由别的版本形成
// （换版之后参与终点所凭的那一版答否），先认出它已被盖住、照已形成了结，不记成未形成。不计费只了结册上已有的那一条，没进过册的不进册。
func TestClosedVersionsAreTracedAndAlreadyCoveredParticipationsAreNot(t *testing.T) {
	for standing, want := range map[ports.SupplierAgreementStanding]ports.ActualFulfillmentUnformedReason{
		ports.SupplierAgreementTerminated:   ports.UnformedAgreementTerminated,
		ports.SupplierAgreementNotEffective: ports.UnformedAgreementNotEffective,
	} {
		t.Run(standing.String(), func(t *testing.T) {
			rig, occurrences := catchUpRig(t, adoptedCommission(t), participationRow(t, trunkParcel, true))
			occurrences.agreements.check = ports.SupplierAgreementCheck{Standing: standing}
			if result := rig.onAdoption(t); result.Disposition() != application.FormationSettled {
				t.Fatalf("disposition = %s", result.Disposition())
			}
			entry, found := rig.unformed.entry(t, trunkParcel)
			if !found || entry.State != ports.ActualFulfillmentUnformed || entry.Entry.Reason != want ||
				entry.Entry.Agreement.Version() != trunkAgreementTag {
				t.Fatalf("未形成那一条 = %+v %v", entry, found)
			}
		})
	}

	t.Run("已被盖住", func(t *testing.T) {
		rig, occurrences := catchUpRig(t, adoptedCommission(t), participationRow(t, trunkParcel, true))
		registeredOccurrence(t, rig.onAdoption(t).Formations()[0].Registration())
		rig.unformed.records[ports.ActualFulfillmentUnformedKey{
			TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
			Segment:  occurrenceValue(t, domain.NewFulfillmentSegmentReference, trunkSegment),
			Object:   occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel),
		}] = ports.ActualFulfillmentUnformedRecord{State: ports.ActualFulfillmentUnformed}
		occurrences.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementTerminated}
		rig.onAdoption(t)
		if entry, _ := rig.unformed.entry(t, trunkParcel); entry.State != ports.ActualFulfillmentUnformedFormed {
			t.Fatalf("已被盖住的那一件记成了 %s", entry.State)
		}
	})

	t.Run("不计费", func(t *testing.T) {
		rig, occurrences := catchUpRig(t, adoptedCommission(t), participationRow(t, trunkParcel, true))
		occurrences.agreements.check.ChargeConditions = declaredConditions(t, declaredCondition{domain.ActualFulfillmentOccurrence, false, 0})
		rig.onAdoption(t)
		if _, found := rig.unformed.entry(t, trunkParcel); found {
			t.Fatal("从没进过册的不计费进了册")
		}
		occurrences.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementTerminated}
		rig.onAdoption(t)
		occurrences.agreements.check = chargeRegisteredAgreement(t).check
		occurrences.agreements.check.ChargeConditions = declaredConditions(t, declaredCondition{domain.ActualFulfillmentOccurrence, false, 0})
		rig.onAdoption(t)
		if entry, _ := rig.unformed.entry(t, trunkParcel); entry.State != ports.ActualFulfillmentUnformedNotCharged || entry.NotChargedBy != trunkSnapshot {
			t.Fatalf("先未形成、后不计费 = %+v", entry)
		}
	})
}

// Covers: AT-TF-303——补形成读不动或写不动：采用连线、按委托列段、段登记册、「盖住」那一问、未形成册此刻读写不动都答未决（整封重投）；
// 门铃指名的段已没有采用连线入账；未形成册确定性拒写是缺陷，上抛。
func TestCatchUpRetriesWhatItCannotReadAndRefusesWhatItCannotWrite(t *testing.T) {
	ended := participationRow(t, trunkParcel, true)
	unconfigured := func(rig *formationRig, occurrences *occurrenceRig) {
		occurrences.agreements.check.ChargeConditions = declaredConditions(t, declaredCondition{domain.FailedAttemptOccurrence, true, domain.PerAttemptQuantity})
	}
	for name, mutate := range map[string]func(*formationRig, *occurrenceRig){
		"段登记册": func(rig *formationRig, _ *occurrenceRig) { rig.segments.findErr = errors.New("segments down") },
		"盖住那一问": func(rig *formationRig, occurrences *occurrenceRig) {
			unconfigured(rig, occurrences)
			occurrences.registry.coverageErr = errors.New("occurrences down")
		},
		"未形成册": func(rig *formationRig, occurrences *occurrenceRig) {
			unconfigured(rig, occurrences)
			rig.unformed.writeErr = errors.New("connection reset")
		},
	} {
		t.Run(name, func(t *testing.T) {
			rig, occurrences := catchUpRig(t, adoptedCommission(t), ended)
			mutate(rig, occurrences)
			if result := rig.onAdoption(t); result.Disposition() != application.FormationRetry || result.UndecidedReason() == application.FormationUndecidedReasonNone {
				t.Fatalf("disposition = %s reason = %s, want 重投并指名停在哪一口", result.Disposition(), result.UndecidedReason())
			}
		})
	}

	rig, occurrences := catchUpRig(t, adoptedCommission(t), ended)
	occurrences.adoptions.err = errors.New("adoptions down")
	if result := rig.onAdoption(t); result.Disposition() != application.FormationRetry {
		t.Fatalf("采用连线读不动 = %s", result.Disposition())
	}
	rig.byCommission.err = errors.New("adoptions down")
	if result := rig.onRevision(t, 2); result.Disposition() != application.FormationRetry {
		t.Fatalf("按委托列段读不动 = %s", result.Disposition())
	}

	unadopted := newFormationRig(t, newOccurrenceRig(t))
	if result := unadopted.onAdoption(t); result.Outcome() != application.CatchUpSegmentNotAdopted || result.Disposition() != application.FormationSettled {
		t.Fatalf("段已没有采用连线 = %s/%s", result.Outcome(), result.Disposition())
	}

	refused, refusedOccurrences := catchUpRig(t, adoptedCommission(t), ended)
	unconfigured(refused, refusedOccurrences)
	refused.unformed.writeErr = fmt.Errorf("check violated: %w", ports.ErrActualFulfillmentUnformedWriteRefused)
	if _, err := refused.handler().CatchUpOnAdoption(t.Context(), application.CatchUpActualFulfillmentOnAdoptionCommand{
		TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Segment: trunkSegment,
	}); !errors.Is(err, ports.ErrActualFulfillmentUnformedWriteRefused) {
		t.Fatalf("确定性拒写 err = %v, want 上抛", err)
	}

	malformed, err := rig.handler().CatchUpOnRevision(t.Context(), application.CatchUpActualFulfillmentOnRevisionCommand{
		TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Commission: bkCommission, Revision: 1,
	})
	if err != nil || malformed.Disposition() != application.FormationDefect {
		t.Fatalf("第 1 版当换版门铃 = %v %s, want 缺陷", err, malformed.Disposition())
	}
}

// ambiguousJourneysFor 是同一刻开始、都覆盖 object 的两趟退运旅程：那一件的旅程定不下来，是旅程册自己的不一致。
func ambiguousJourneysFor(t *testing.T, object string) []ports.AlternateJourneyRecord {
	t.Helper()
	var records []ports.AlternateJourneyRecord
	for _, name := range []string{"A", "B"} {
		journey, err := domain.FormAlternateJourney(domain.AlternateJourneySpec{
			TenantID:        occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
			Journey:         occurrenceValue(t, domain.NewJourneyReference, "SYN-JRN-RETURN-"+name+"-"+object),
			Purpose:         domain.ReturnJourneyPurpose,
			OriginalJourney: occurrenceValue(t, domain.NewJourneyReference, "JRN/"+object),
			BasisKind:       domain.ServiceDispositionDecision,
			Basis:           occurrenceValue(t, domain.NewDispositionBasisReference, "SYN-DISPOSITION-RETURN-"+name),
			Members:         []domain.CarriedObjectReference{occurrenceValue(t, domain.NewCarriedObjectReference, object)},
			StartedAt:       occurrenceEnteredAt.Add(-time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, ports.AlternateJourneyRecord{Journey: journey})
	}
	return records
}

// Covers: AT-TF-303（ADR-0231 决定二、六）——补形成里一件撞上数据缺陷（旅程平局；采用连线指名的委托读不回；PC 对委托记着的那一版答不在册、
// 供应商不符或拼回另一串快照），那一件记进未形成册：格 FORMATION_DEFECT，类别码连同完整错误原文，号与问的那一版不记；交回的答复带着那一件
// 与完整错误，同一封门铃里的其余各件照常形成，整封入账。未形成册连缺陷都记不下（确定性拒写）时整封响亮失败。
func TestADataDefectIsRecordedWhileTheRestOfTheDoorbellForms(t *testing.T) {
	rig, occurrences := catchUpRig(t, adoptedCommission(t),
		participationRow(t, trunkParcel, true), participationRow(t, trunkSecondParcel, true))
	rig.journeys.records = ambiguousJourneysFor(t, trunkSecondParcel)

	result := rig.onAdoption(t)
	if result.Disposition() != application.FormationSettled || len(result.Formations()) != 2 {
		t.Fatalf("门铃 = %s，%d 件, want 整封入账、两件都有答复", result.Disposition(), len(result.Formations()))
	}
	registeredOccurrence(t, result.Formations()[0].Registration())
	defective := result.Formations()[1]
	if defective.Outcome() != application.FormationDefectRecorded || defective.Object() != trunkSecondParcel ||
		defective.Segment() != trunkSegment || !errors.Is(defective.Defect(), domain.ErrAmbiguousJourney) || occurrences.registry.saves != 1 {
		t.Fatalf("平局那一件 = %s %q %v，册上 %d 项", defective.Outcome(), defective.Object(), defective.Defect(), occurrences.registry.saves)
	}
	entry, found := rig.unformed.entry(t, trunkSecondParcel)
	if !found || entry.State != ports.ActualFulfillmentUnformed || entry.Entry.Reason != ports.UnformedFormationDefect ||
		entry.Entry.Defect != "AMBIGUOUS_JOURNEY: "+defective.Defect().Error() || entry.Entry.Revision != 0 ||
		entry.Entry.Commission.String() != bkCommission || !entry.Entry.EndedAt.Equal(occurrenceEndedAt) {
		t.Fatalf("缺陷那一条 = %+v %v", entry, found)
	}
	if _, found := rig.unformed.entry(t, trunkParcel); found {
		t.Fatal("顺利形成的那一件进了册")
	}

	for name, data := range map[string]struct {
		mutate   func(*occurrenceRig)
		code     string
		sentinel error
	}{
		"委托读不回": {func(occurrences *occurrenceRig) { occurrences.commissions = commissionViewDouble{} },
			"ADOPTION_WITHOUT_COMMISSION: ", application.ErrAdoptionWithoutCommission},
		"协议核对答不在册": {func(occurrences *occurrenceRig) {
			occurrences.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementNotRegistered}
		}, "COMMISSION_AGREEMENT_DRIFT: ", application.ErrCommissionAgreementDrift},
		"快照走岔": {func(occurrences *occurrenceRig) {
			occurrences.agreements.check.Snapshot = occurrenceValue(t, domain.NewAgreementSnapshotReference, revisedSnapshot)
		}, "COMMISSION_AGREEMENT_DRIFT: ", application.ErrCommissionAgreementDrift},
	} {
		t.Run(name, func(t *testing.T) {
			rig, occurrences := catchUpRig(t, adoptedCommission(t), participationRow(t, trunkParcel, true))
			data.mutate(occurrences)
			rig.spy.inner = occurrences.handler()
			result := rig.onAdoption(t)
			if result.Disposition() != application.FormationSettled || len(result.Formations()) != 1 ||
				!errors.Is(result.Formations()[0].Defect(), data.sentinel) {
				t.Fatalf("那一封 = %s %+v", result.Disposition(), result.Formations())
			}
			entry, _ := rig.unformed.entry(t, trunkParcel)
			if entry.Entry.Reason != ports.UnformedFormationDefect ||
				entry.Entry.Defect != data.code+result.Formations()[0].Defect().Error() {
				t.Fatalf("那一条 = %+v", entry)
			}
		})
	}

	refused, _ := catchUpRig(t, adoptedCommission(t), participationRow(t, trunkParcel, true))
	refused.journeys.records = ambiguousJourneysFor(t, trunkParcel)
	refused.unformed.writeErr = fmt.Errorf("check violated: %w", ports.ErrActualFulfillmentUnformedWriteRefused)
	if _, err := refused.handler().CatchUpOnAdoption(t.Context(), application.CatchUpActualFulfillmentOnAdoptionCommand{
		TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Segment: trunkSegment,
	}); !errors.Is(err, ports.ErrActualFulfillmentUnformedWriteRefused) {
		t.Fatalf("缺陷也记不下 err = %v, want 上抛", err)
	}
}

// Covers: AT-TF-303（ADR-0231 决定二）——数据缺陷那一路上读写不动照样重投、什么都不记；缺陷若出在那一件已有发生项盖住之后，照已形成了结。
func TestTheDefectPathRetriesWhatItCannotReadAndSettlesWhatIsCovered(t *testing.T) {
	for name, mutate := range map[string]func(*formationRig, *occurrenceRig){
		"盖住那一问": func(_ *formationRig, occurrences *occurrenceRig) {
			occurrences.registry.coverageErr = errors.New("occurrences down")
		},
		"未形成册": func(rig *formationRig, _ *occurrenceRig) { rig.unformed.writeErr = errors.New("connection reset") },
	} {
		t.Run(name, func(t *testing.T) {
			rig, occurrences := catchUpRig(t, adoptedCommission(t), participationRow(t, trunkParcel, true))
			rig.journeys.records = ambiguousJourneysFor(t, trunkParcel)
			mutate(rig, occurrences)
			result := rig.onAdoption(t)
			if _, found := rig.unformed.entry(t, trunkParcel); result.Disposition() != application.FormationRetry ||
				result.UndecidedReason() == application.FormationUndecidedReasonNone || found {
				t.Fatalf("disposition = %s reason = %s 进册 = %v, want 重投、指名停在哪一口、不进册", result.Disposition(), result.UndecidedReason(), found)
			}
		})
	}

	rig, _ := catchUpRig(t, adoptedCommission(t), participationRow(t, trunkParcel, true))
	formed := registeredOccurrence(t, rig.onAdoption(t).Formations()[0].Registration())
	key := ports.ActualFulfillmentUnformedKey{
		TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Segment:  occurrenceValue(t, domain.NewFulfillmentSegmentReference, trunkSegment),
		Object:   occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel),
	}
	rig.unformed.records[key] = ports.ActualFulfillmentUnformedRecord{State: ports.ActualFulfillmentUnformed}
	rig.journeys.records = ambiguousJourneysFor(t, trunkParcel)
	result := rig.onAdoption(t)
	if result.Disposition() != application.FormationSettled || result.Formations()[0].Outcome() != application.FormationDefectRecorded {
		t.Fatalf("已被盖住的缺陷那一封 = %s %+v", result.Disposition(), result.Formations())
	}
	if entry := rig.unformed.records[key]; entry.State != ports.ActualFulfillmentUnformedFormed || entry.Occurrence.Occurrence != formed.Occurrence() {
		t.Fatalf("已被盖住的那一件 = %+v, want 照已形成了结、记下那一项", entry)
	}
}

// Covers: AT-TF-303（ADR-0231 决定二）——编程缺陷不进册：协议核对口交回集外的一格、登记用例答出本路不该见的一格或集外的答复，补形成照
// 交接、交付两路一样响亮失败（上抛或整封答缺陷），未形成册上什么都不记，同一封里排在后面的那一件也不再问。
func TestAProgrammingDefectFailsTheWholeDoorbellAndRecordsNothing(t *testing.T) {
	rows := []domain.RehydrateParticipationSpec{participationRow(t, trunkParcel, true), participationRow(t, trunkSecondParcel, true)}

	t.Run("核对口集外一格", func(t *testing.T) {
		rig, occurrences := catchUpRig(t, adoptedCommission(t), rows...)
		occurrences.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementStanding(250)}
		rig.spy.inner = occurrences.handler()
		_, err := rig.handler().CatchUpOnAdoption(t.Context(), application.CatchUpActualFulfillmentOnAdoptionCommand{
			TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Segment: trunkSegment,
		})
		if err == nil || !strings.Contains(err.Error(), "outside its closed set") {
			t.Fatalf("err = %v, want 上抛", err)
		}
		if len(rig.unformed.records) != 0 || len(rig.spy.commands) != 1 {
			t.Fatalf("册上 %d 条、交登记 %d 次, want 不进册、停在第一件", len(rig.unformed.records), len(rig.spy.commands))
		}
	})

	for name, canned := range map[string]application.RegisterChargeOccurrenceResult{
		"不适用":     cannedRegistration(t, application.ChargeOccurrenceNotApplicable),
		"采购上下文冲突": cannedRegistration(t, application.ChargeOccurrenceConflict),
		"集外答复":    {},
	} {
		t.Run(name, func(t *testing.T) {
			rig, _ := catchUpRig(t, adoptedCommission(t), rows...)
			rig.spy = &registrationSpy{canned: canned}
			for doorbell, answer := range map[string]func() application.CatchUpActualFulfillmentResult{
				"采用": func() application.CatchUpActualFulfillmentResult { return rig.onAdoption(t) },
				"换版": func() application.CatchUpActualFulfillmentResult { return rig.onRevision(t, 2) },
			} {
				rig.spy.commands = nil
				if result := answer(); result.Disposition() != application.FormationDefect || len(rig.spy.commands) != 1 {
					t.Fatalf("%s门铃 = %s，交登记 %d 次, want 缺陷、停在第一件", doorbell, result.Disposition(), len(rig.spy.commands))
				}
				if len(rig.unformed.records) != 0 {
					t.Fatalf("%s门铃把编程缺陷记进了册：%+v", doorbell, rig.unformed.records)
				}
			}
		})
	}
}
