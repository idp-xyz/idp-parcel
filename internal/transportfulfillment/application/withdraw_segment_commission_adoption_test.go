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

// 本文件证解除履约段采用的编排（票 demo-chain-gaps/57，ADR-0232 决定一至三、六）与登记用例在锁下重读采用。夹具复用登记用例那一套
// 合成串，证据只记 S；真库面与并发见 adapters/postgres 与 cmd/parcel-api 的同名用例。

const withdrawingOperator = "https://dex.syn.example#SYN-OPERATOR-CORRECTION-01"

// withdrawalStoresDouble 同时是当前采用、解除册与按委托列当前采用三口：一段的当前采用在 current 里，解除落进 withdrawals。
type withdrawalStoresDouble struct {
	current     map[domain.FulfillmentSegmentReference]ports.SegmentCommissionAdoptionRecord
	withdrawals map[domain.FulfillmentSegmentReference]ports.SegmentCommissionAdoptionWithdrawalRecord
	err         error
}

func (double *withdrawalStoresDouble) FindBySegment(_ context.Context, key ports.SegmentCommissionAdoptionKey) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	record, found := double.current[key.Segment]
	return record, found, double.err
}

func (*withdrawalStoresDouble) Save(context.Context, ports.SegmentCommissionAdoptionRecord) (ports.AdoptionSaveOutcome, error) {
	return ports.AdoptionSaveOutcomeInvalid, errors.New("解除不该落采用")
}

func (double *withdrawalStoresDouble) FindLatestWithdrawal(_ context.Context, key ports.SegmentCommissionAdoptionKey) (ports.SegmentCommissionAdoptionWithdrawalRecord, bool, error) {
	record, found := double.withdrawals[key.Segment]
	return record, found, nil
}

// FindWithdrawal 替身里一段至多一次解除：代次对得上才算那一代的解除。
func (double *withdrawalStoresDouble) FindWithdrawal(_ context.Context, key ports.SegmentCommissionAdoptionKey, generation int) (ports.SegmentCommissionAdoptionWithdrawalRecord, bool, error) {
	record, found := double.withdrawals[key.Segment]
	if !found || record.Withdrawal.Generation() != generation {
		return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, nil
	}
	return record, true, nil
}

func (double *withdrawalStoresDouble) SaveWithdrawal(_ context.Context, record ports.SegmentCommissionAdoptionWithdrawalRecord) (ports.AdoptionSaveOutcome, error) {
	if _, exists := double.withdrawals[record.Key.Segment]; exists {
		return ports.AdoptionAlreadyRegistered, nil
	}
	double.withdrawals[record.Key.Segment] = record
	delete(double.current, record.Key.Segment)
	return ports.AdoptionSaved, nil
}

func (double *withdrawalStoresDouble) FindCurrentByCommission(_ context.Context, key ports.TransportCommissionKey) ([]ports.SegmentCommissionAdoptionRecord, error) {
	var records []ports.SegmentCommissionAdoptionRecord
	for _, record := range double.current {
		if record.Adoption.Commission() == key.Commission {
			records = append(records, record)
		}
	}
	return records, nil
}

type scopeHeadsDouble struct {
	heads []ports.ChargeOccurrenceRecord
}

func (double scopeHeadsDouble) ListCurrentOnScope(context.Context, domain.TenantID, domain.ChargeOccurrenceReason, domain.OccurrenceScopeReference) ([]ports.ChargeOccurrenceRecord, error) {
	return double.heads, nil
}

type startCorrectionDouble struct {
	written []ports.TransportCommissionRecord
}

func (double *startCorrectionDouble) CorrectStart(_ context.Context, record ports.TransportCommissionRecord, _ domain.ParticipationBasisReference, _ time.Time) (bool, error) {
	double.written = append(double.written, record)
	return true, nil
}

type withdrawalFixture struct {
	stores     *withdrawalStoresDouble
	heads      scopeHeadsDouble
	scopes     *scopeHoldDouble
	correction *startCorrectionDouble
	commission ports.TransportCommissionRecord
	segment    domain.FulfillmentSegmentReference
}

func newWithdrawalFixture(t *testing.T) *withdrawalFixture {
	t.Helper()
	commission := trunkCommission(t, commissionShape{startedAt: occurrenceEnteredAt})
	segment := occurrenceValue(t, domain.NewFulfillmentSegmentReference, trunkSegment)
	basis, at, _ := commission.Commission.TransportStarted()
	adoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID: commission.Key.TenantID, Segment: segment, Commission: commission.Key.Commission,
		Provider: commission.Commission.Provider(), StartBasis: basis, StartedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &withdrawalFixture{
		stores: &withdrawalStoresDouble{
			current: map[domain.FulfillmentSegmentReference]ports.SegmentCommissionAdoptionRecord{
				segment: {Key: ports.SegmentCommissionAdoptionKey{TenantID: commission.Key.TenantID, Segment: segment}, Adoption: adoption, Generation: 1},
			},
			withdrawals: map[domain.FulfillmentSegmentReference]ports.SegmentCommissionAdoptionWithdrawalRecord{},
		},
		scopes:     &scopeHoldDouble{},
		correction: &startCorrectionDouble{},
		commission: commission,
		segment:    segment,
	}
}

func (fixture *withdrawalFixture) withdraw(t *testing.T, commission string) application.AdoptionWithdrawalResult {
	t.Helper()
	handler := application.NewWithdrawSegmentCommissionAdoptionHandler(application.WithdrawSegmentCommissionAdoptionDeps{
		Adoptions: fixture.stores, Withdrawals: fixture.stores, CommissionLinks: fixture.stores,
		Occurrences: fixture.heads, OccurrenceScope: fixture.scopes,
		Commissions:     commissionViewDouble{records: map[domain.TransportCommissionReference]ports.TransportCommissionRecord{fixture.commission.Key.Commission: fixture.commission}},
		CommissionHold:  freeCommissionHold{},
		StartCorrection: fixture.correction,
		Clock:           occurrenceClock{},
	})
	result, err := handler.Withdraw(t.Context(), application.WithdrawSegmentCommissionAdoptionCommand{
		TenantID: fixture.commission.Key.TenantID, Operator: withdrawingOperator, Segment: trunkSegment, Commission: commission,
		Generation: 1,
	})
	if err != nil {
		t.Fatalf("解除不该上抛：%v", err)
	}
	return result
}

// occurrenceOnTheSegment 是这一段上一项仍有效的实际履约发生项，服务方与协议快照可换。
func occurrenceOnTheSegment(t *testing.T, provider, snapshot string) ports.ChargeOccurrenceRecord {
	t.Helper()
	rig := newOccurrenceRig(t)
	record, _ := rig.register(t, actualFulfillmentCommand(t)).Record()
	spec := domain.RehydrateChargeOccurrenceSpec{
		TenantID: record.Occurrence.TenantID(), Occurrence: record.Occurrence.Occurrence(), Journey: record.Occurrence.Journey(),
		LegalEntity: record.Occurrence.LegalEntity(), Provider: occurrenceValue(t, domain.NewServiceProviderReference, provider),
		Agreement: occurrenceValue(t, domain.NewAgreementSnapshotReference, snapshot), Reason: record.Occurrence.Reason(),
		FactBasis: record.Occurrence.FactBasis(), Scope: record.Occurrence.Scope(), Members: record.Occurrence.Members(),
		OccurredAt: record.Occurrence.OccurredAt(), Validity: record.Occurrence.Validity(), Condition: record.Occurrence.Condition(),
	}
	spec.Quantity, spec.Unit = record.Occurrence.Quantity()
	occurrence, err := domain.RehydrateChargeOccurrence(spec)
	if err != nil {
		t.Fatal(err)
	}
	record.Occurrence = occurrence
	return record
}

// Covers: AT-TF-309——这一段上已按这份委托形成了一项仍有效的发生项：解除被挡，答 OCCURRENCES_FORMED、回显那一项，不落解除也不动委托；
// 段上另有一项采购上下文不是这份委托的（采用之前登记方自报的），不挡解除。挡之前先取了这一段实际履约范围的发生项锁。
func TestAWithdrawalIsRefusedWhileTheSegmentHasOccurrencesFormedUnderTheCommission(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	formed := occurrenceOnTheSegment(t, trunkProvider, trunkSnapshot)
	fixture.heads = scopeHeadsDouble{heads: []ports.ChargeOccurrenceRecord{formed}}
	result := fixture.withdraw(t, bkCommission)
	if result.Outcome() != application.AdoptionWithdrawalOccurrencesFormed || len(result.FormedOccurrences()) != 1 ||
		result.FormedOccurrences()[0].Key != formed.Key {
		t.Fatalf("已形成发生项 = %q %d 项", result.Outcome(), len(result.FormedOccurrences()))
	}
	if len(fixture.stores.withdrawals) != 0 || len(fixture.correction.written) != 0 {
		t.Fatal("被挡的解除写了东西")
	}
	if len(fixture.scopes.held) != 1 || fixture.scopes.held[0].String() != trunkSegment {
		t.Fatalf("没先取段的发生项锁：%v", fixture.scopes.held)
	}

	fixture.heads = scopeHeadsDouble{heads: []ports.ChargeOccurrenceRecord{occurrenceOnTheSegment(t, "SYN-PARTY-CARRIER-LM-01", "SYN-SUPPLIER-LASTMILE-SG-01/v1")}}
	if result := fixture.withdraw(t, bkCommission); result.Outcome() != application.AdoptionWithdrawn {
		t.Fatalf("另一份采购上下文的发生项挡了解除：%q", result.Outcome())
	}
}

// Covers: AT-TF-306、AT-TF-307 的编排面——唯一一段解除：委托撤回到已提交，写口收到开始清空的那一份；原样再交答重放；点名另一份委托答
// 委托不符、回显在册那一份；从没采用过的段答不在册。
func TestWithdrawingTheOnlyAdoptionWithdrawsTheStartAndIsIdempotent(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	if mismatch := fixture.withdraw(t, "SYN-COMMISSION-OTHER"); mismatch.Outcome() != application.AdoptionWithdrawalCommissionMismatch {
		t.Fatalf("点名另一份委托 = %q", mismatch.Outcome())
	} else if echoed, _ := mismatch.Adoption(); echoed.Adoption.Commission().String() != bkCommission {
		t.Fatal("委托不符没回显在册那一份")
	}
	result := fixture.withdraw(t, bkCommission)
	if result.Outcome() != application.AdoptionWithdrawn || len(fixture.correction.written) != 1 {
		t.Fatalf("解除 = %q，写委托 %d 次", result.Outcome(), len(fixture.correction.written))
	}
	if _, _, started := fixture.correction.written[0].Commission.TransportStarted(); started {
		t.Fatal("唯一一段解除之后委托仍是已开始")
	}
	record, _ := result.Withdrawal()
	if record.Withdrawal.StartChange() != domain.CommissionStartWithdrawn || record.Withdrawal.WithdrawnBy().String() != withdrawingOperator {
		t.Fatalf("解除 = %+v", record.Withdrawal)
	}
	if replay := fixture.withdraw(t, bkCommission); replay.Outcome() != application.AdoptionWithdrawalExisting || len(fixture.correction.written) != 1 {
		t.Fatalf("重放 = %q", replay.Outcome())
	}
	if other := fixture.withdraw(t, "SYN-COMMISSION-OTHER"); other.Outcome() != application.AdoptionWithdrawalCommissionMismatch {
		t.Fatalf("已解除的那一代点名另一份委托 = %q, want ADOPTION_MISMATCH", other.Outcome())
	} else if echoed, present := other.Withdrawal(); !present || echoed.Withdrawal.Commission().String() != bkCommission {
		t.Fatal("委托不符没回显那一次解除")
	}
}

// Covers: AT-TF-310 的编排面（ADR-0232 决定一）——解除按（段、委托、代次）认：点名的代次不是段上当前那一代、也没有它的解除，答代次不是
// 当前并回显当前那一代；段上从没采用过的答不在册。
func TestAWithdrawalNamingAnotherGenerationIsNotApplied(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	handler := application.NewWithdrawSegmentCommissionAdoptionHandler(application.WithdrawSegmentCommissionAdoptionDeps{
		Adoptions: fixture.stores, Withdrawals: fixture.stores, CommissionLinks: fixture.stores,
		Occurrences: fixture.heads, OccurrenceScope: fixture.scopes,
		Commissions:     commissionViewDouble{records: map[domain.TransportCommissionReference]ports.TransportCommissionRecord{fixture.commission.Key.Commission: fixture.commission}},
		CommissionHold:  freeCommissionHold{},
		StartCorrection: fixture.correction,
		Clock:           occurrenceClock{},
	})
	result, err := handler.Withdraw(t.Context(), application.WithdrawSegmentCommissionAdoptionCommand{
		TenantID: fixture.commission.Key.TenantID, Operator: withdrawingOperator, Segment: trunkSegment, Commission: bkCommission, Generation: 2,
	})
	if err != nil || result.Outcome() != application.AdoptionWithdrawalGenerationNotCurrent || len(fixture.correction.written) != 0 {
		t.Fatalf("点名第二代 = %q %v，写委托 %d 次", result.Outcome(), err, len(fixture.correction.written))
	}
	if echoed, present := result.Adoption(); !present || echoed.Generation != 1 {
		t.Fatalf("没回显当前那一代：%+v", echoed)
	}
	delete(fixture.stores.current, fixture.segment)
	if missing := fixture.withdraw(t, bkCommission); missing.Outcome() != application.AdoptionWithdrawalNotFound {
		t.Fatalf("没有采用 = %q", missing.Outcome())
	}
}

func TestAWithdrawalThatCannotHoldTheSegmentScopeIsUndecided(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.scopes.err = errors.New("lock down")
	result := fixture.withdraw(t, bkCommission)
	if result.Outcome() != application.AdoptionWithdrawalUndecided ||
		result.UndecidedReason() != application.AdoptionWithdrawalOccurrenceRegistryUnavailable || result.ContinuationReference() == "" {
		t.Fatalf("取锁失败 = %q/%q", result.Outcome(), result.UndecidedReason())
	}
	for _, malformed := range []application.WithdrawSegmentCommissionAdoptionCommand{
		{TenantID: fixture.commission.Key.TenantID, Operator: withdrawingOperator, Segment: "", Commission: bkCommission, Generation: 1},
		{TenantID: fixture.commission.Key.TenantID, Operator: withdrawingOperator, Segment: trunkSegment, Commission: "", Generation: 1},
		{TenantID: fixture.commission.Key.TenantID, Operator: "", Segment: trunkSegment, Commission: bkCommission, Generation: 1},
		{TenantID: fixture.commission.Key.TenantID, Operator: withdrawingOperator, Segment: trunkSegment, Commission: bkCommission},
	} {
		handler := application.NewWithdrawSegmentCommissionAdoptionHandler(application.WithdrawSegmentCommissionAdoptionDeps{})
		if result, err := handler.Withdraw(t.Context(), malformed); err != nil || result.Outcome() != application.AdoptionWithdrawalNotAccepted {
			t.Fatalf("%+v = %q, %v", malformed, result.Outcome(), err)
		}
	}
}

// flippingAdoptions 第一次按段读交 first，之后都交 later：登记在锁外读到的采用与锁下重读到的不同。
type flippingAdoptions struct {
	first, later ports.SegmentCommissionAdoptionRecord
	firstFound   bool
	laterFound   bool
	reads        *int
}

func (double flippingAdoptions) FindBySegment(context.Context, ports.SegmentCommissionAdoptionKey) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	*double.reads++
	if *double.reads == 1 {
		return double.first, double.firstFound, nil
	}
	return double.later, double.laterFound, nil
}

// Covers: AT-TF-313——登记取事实时读到这一段采用了委托，取到范围锁之后重读，采用已被解除：不凭旧连线落册，答未决
// PROCUREMENT_SOURCE_CHANGED，什么都不写；连线没变照常登记。
func TestARegistrationWhoseAdoptionChangedUnderTheScopeHoldIsUndecided(t *testing.T) {
	commission := trunkCommission(t, commissionShape{startedAt: occurrenceEnteredAt})
	rig := adoptedTrunkRig(t, commission)
	adopted := rig.adoptions.records[occurrenceValue(t, domain.NewFulfillmentSegmentReference, trunkSegment)]
	omitted := actualFulfillmentCommand(t)
	omitted.Provider, omitted.AgreementObject, omitted.AgreementVersion = "", "", ""

	reads := 0
	handler := application.NewRegisterChargeOccurrenceHandler(application.RegisterChargeOccurrenceDeps{
		Segments: rig.segments, CarrierJudgments: rig.judgments,
		Adoptions:   flippingAdoptions{first: adopted, firstFound: true, laterFound: false, reads: &reads},
		Commissions: rig.commissions, Agreements: rig.agreements, Occurrences: rig.registry, Handoff: rig.handoff, Clock: occurrenceClock{},
	})
	result, err := handler.Register(t.Context(), omitted)
	if err != nil || result.Outcome() != application.ChargeOccurrenceUndecided || result.Detail() != application.ProcurementSourceChanged {
		t.Fatalf("锁下采用变了 = %q/%q %v", result.Outcome(), result.Detail(), err)
	}
	if reads != 2 {
		t.Fatalf("按段读采用 %d 次，want 锁外一次、锁下一次", reads)
	}
	assertNothingWritten(t, rig)

	steady := adoptedTrunkRig(t, commission)
	registeredOccurrence(t, steady.register(t, omitted))
}

// withdrawnOnce 是解除册的替身：这一段第 1 代已解除过。
type withdrawnOnce struct {
	noWithdrawals
	latest ports.SegmentCommissionAdoptionWithdrawalRecord
}

func (double withdrawnOnce) FindLatestWithdrawal(context.Context, ports.SegmentCommissionAdoptionKey) (ports.SegmentCommissionAdoptionWithdrawalRecord, bool, error) {
	return double.latest, true, nil
}

// Covers: AT-TF-310 的采用那一半（ADR-0232 决定一）——这一段解除过之后，采用必须点名接在哪一代之后：没点名（多半是解除之前那一次采用
// 被原样重交）或点错，答已被解除并回显那一次解除，不落新一代、不动委托；点对了落成第 2 代。首次采用点了代次是坏请求。
func TestAnAdoptionAfterAWithdrawalMustNameTheGenerationItFollows(t *testing.T) {
	fixture := newCommissionAdoptionFixture(t)
	tenant, _ := domain.NewTenantID("tenant-1")
	withdrawal, err := domain.RehydrateSegmentCommissionAdoptionWithdrawal(domain.RehydrateSegmentCommissionAdoptionWithdrawalSpec{
		TenantID: tenant, Segment: occurrenceValue(t, domain.NewFulfillmentSegmentReference, "SEG-1"), Generation: 1,
		Commission: occurrenceValue(t, domain.NewTransportCommissionReference, "commission-1"), WithdrawnBy: occurrenceValue(t, domain.NewRegistrantReference, withdrawingOperator),
		StartBeforeBasis: occurrenceValue(t, domain.NewParticipationBasisReference, "HO/before"), StartBeforeAt: segmentEnteredAt,
		StartChange: domain.CommissionStartWithdrawn,
	})
	if err != nil {
		t.Fatal(err)
	}
	adopter := application.NewAdoptSegmentCommissionHandler(application.AdoptSegmentCommissionDeps{
		Adoptions: fixture.adoptions, Segments: fixture.segments, Commissions: fixture.racing, StartAdvancement: fixture.racing,
		CommissionHold: freeCommissionHold{}, Doorbell: fixture.doorbell,
		Withdrawals: withdrawnOnce{latest: ports.SegmentCommissionAdoptionWithdrawalRecord{Withdrawal: withdrawal}},
		Clock:       commissionClock{at: commissionNowAt},
	})
	for _, after := range []int{0, 2} {
		command := adoptCommand("SEG-1", "commission-1", "partner-1")
		command.AfterGeneration = after
		result, err := adopter.Adopt(context.Background(), command)
		echoed, present := result.Withdrawal()
		if err != nil || result.Outcome() != application.CommissionAdoptionAlreadyWithdrawn || !present || echoed.Withdrawal.Generation() != 1 {
			t.Fatalf("接在第 %d 代之后 = %q %v", after, result.Outcome(), err)
		}
	}
	if fixture.adoptions.saves != 0 {
		t.Fatalf("没点对代次也落了 %d 次采用", fixture.adoptions.saves)
	}
	command := adoptCommand("SEG-1", "commission-1", "partner-1")
	command.AfterGeneration = 1
	result, err := adopter.Adopt(context.Background(), command)
	if record, _ := result.Adoption(); err != nil || result.Outcome() != application.CommissionAdopted || record.Generation != 2 {
		t.Fatalf("点对代次 = %q %v，代次 %d", result.Outcome(), err, record.Generation)
	}

	first := adoptCommand("SEG-2", "commission-1", "partner-1")
	first.AfterGeneration = 1
	if result, err := fixture.adopter.Adopt(context.Background(), first); err != nil || result.Outcome() != application.CommissionAdoptionNotAccepted {
		t.Fatalf("首次采用点了代次 = %q %v", result.Outcome(), err)
	}
}
