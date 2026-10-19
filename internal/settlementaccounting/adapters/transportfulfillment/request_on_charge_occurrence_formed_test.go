package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/transportfulfillment"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证 ADR-0208 的消费侧翻译：按信封三维回读发生项，原因、版本、业务时间与协议原样译成发起命令；读不回是可见性
// 未决，读回的键对不上是提供方不变量已破；编排的各格落成消费两格——发起、未配置、修订版入账，未决重投，未受理与集外
// 结果上抛。夹具全是合成串，证据只记 S。

type occurrenceSourceDouble struct {
	records map[tfports.ChargeOccurrenceKey]tfports.ChargeOccurrenceRecord
	err     error
}

func (double occurrenceSourceDouble) FindByKey(
	_ context.Context, key tfports.ChargeOccurrenceKey,
) (tfports.ChargeOccurrenceRecord, bool, error) {
	if double.err != nil {
		return tfports.ChargeOccurrenceRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

type triggerDouble struct {
	commands []saapplication.TriggerBuyEvaluationOnOccurrenceFormedCommand
	result   saapplication.BuyEvaluationTriggerResult
	err      error
}

func (double *triggerDouble) Trigger(
	_ context.Context, command saapplication.TriggerBuyEvaluationOnOccurrenceFormedCommand,
) (saapplication.BuyEvaluationTriggerResult, error) {
	double.commands = append(double.commands, command)
	return double.result, double.err
}

func must[T any](t *testing.T) func(T, error) T {
	return func(value T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatalf("fixture: %v", err)
		}
		return value
	}
}

var occurredAt = time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)

func trunkOccurrence(t *testing.T) tfdomain.TransportChargeOccurrence {
	t.Helper()
	return must[tfdomain.TransportChargeOccurrence](t)(tfdomain.FormTransportChargeOccurrence(tfdomain.TransportChargeOccurrenceSpec{
		TenantID:    must[tfdomain.TenantID](t)(tfdomain.NewTenantID("SYN-TENANT-01")),
		Occurrence:  must[tfdomain.ChargeOccurrenceReference](t)(tfdomain.NewChargeOccurrenceReference("SYN-OCC-TRUNK-01")),
		Journey:     must[tfdomain.JourneyReference](t)(tfdomain.NewJourneyReference("SYN-JRN-SYN-PARCEL-01")),
		LegalEntity: must[tfdomain.ProcurementLegalEntityReference](t)(tfdomain.NewProcurementLegalEntityReference("SYN-LE-01")),
		Provider:    must[tfdomain.ServiceProviderReference](t)(tfdomain.NewServiceProviderReference("SYN-PARTY-CARRIER-TRUNK-01")),
		Agreement:   must[tfdomain.AgreementSnapshotReference](t)(tfdomain.NewAgreementSnapshotReference("SYN-SUPPLIER-TRUNK-CN-SG-01/v1")),
		Reason:      tfdomain.ActualFulfillmentOccurrence,
		FactBasis:   must[tfdomain.OccurrenceBasisReference](t)(tfdomain.NewOccurrenceBasisReference("SYN-PARTICIPATION-TRUNK-01")),
		Scope:       must[tfdomain.OccurrenceScopeReference](t)(tfdomain.NewOccurrenceScopeReference("SYN-SEGMENT-SZX-SIN-01")),
		Members:     []tfdomain.CarriedObjectReference{must[tfdomain.CarriedObjectReference](t)(tfdomain.NewCarriedObjectReference("SYN-PARCEL-01"))},
		Quantity:    1,
		Unit:        must[tfdomain.QuantityUnitReference](t)(tfdomain.NewQuantityUnitReference("SEGMENT")),
		OccurredAt:  occurredAt,
		Validity:    must[tfdomain.OccurrenceValidityVersion](t)(tfdomain.NewOccurrenceValidityVersion("V1")),
		Condition:   must[tfdomain.OccurrenceCondition](t)(tfdomain.NewAssertedOccurrenceCondition("SYN 断言：干线协议 v1 的实际履约条件已满足")),
	}))
}

func keyOf(occurrence tfdomain.TransportChargeOccurrence) tfports.ChargeOccurrenceKey {
	return tfports.ChargeOccurrenceKey{TenantID: occurrence.TenantID(), Occurrence: occurrence.Occurrence(), Validity: occurrence.Validity()}
}

func sourceWith(occurrences ...tfdomain.TransportChargeOccurrence) occurrenceSourceDouble {
	records := map[tfports.ChargeOccurrenceKey]tfports.ChargeOccurrenceRecord{}
	for _, occurrence := range occurrences {
		records[keyOf(occurrence)] = tfports.ChargeOccurrenceRecord{Key: keyOf(occurrence), Occurrence: occurrence, RecordedAt: occurredAt}
	}
	return occurrenceSourceDouble{records: records}
}

func formedOf(occurrence tfdomain.TransportChargeOccurrence) sainbox.FormedChargeOccurrence {
	return sainbox.FormedChargeOccurrence{
		TenantID:   occurrence.TenantID().String(),
		Occurrence: occurrence.Occurrence().String(),
		Version:    occurrence.Validity().String(),
	}
}

func newAdapter(t *testing.T, source adapter.ChargeOccurrenceSource, trigger adapter.BuyEvaluationTrigger) *adapter.RequestOnChargeOccurrenceFormedAdapter {
	t.Helper()
	return must[*adapter.RequestOnChargeOccurrenceFormedAdapter](t)(adapter.NewRequestOnChargeOccurrenceFormedAdapter(source, trigger))
}

// Covers: 发生项四件引用与协议原样取自回读的那一份——原因用运输履约的原词、版本是有效性版本、业务时间不换成处理时刻；
// 首版不标修订。发起这一格入账。
func TestTheFormedOccurrenceIsTranslatedFromItsOwnRecord(t *testing.T) {
	occurrence := trunkOccurrence(t)
	trigger := &triggerDouble{}
	// 替身交回零值结果，那是封闭集合以外的一格：上抛，不静默入账。
	err := newAdapter(t, sourceWith(occurrence), trigger).HandleFormedChargeOccurrence(t.Context(), formedOf(occurrence))
	if !errors.Is(err, adapter.ErrUnexpectedBuyEvaluationTriggerOutcome) {
		t.Fatalf("err = %v, want ErrUnexpectedBuyEvaluationTriggerOutcome", err)
	}
	if len(trigger.commands) != 1 {
		t.Fatalf("编排被调 %d 次", len(trigger.commands))
	}
	command := trigger.commands[0]
	if command.TenantID.String() != "SYN-TENANT-01" || command.Occurrence.ID().String() != "SYN-OCC-TRUNK-01" ||
		command.Occurrence.Reason().String() != "ACTUAL_FULFILLMENT" || command.Occurrence.Version().String() != "V1" ||
		!command.Occurrence.OccurredAt().Equal(occurredAt) || command.Agreement.String() != "SYN-SUPPLIER-TRUNK-CN-SG-01/v1" ||
		command.Revised {
		t.Fatalf("命令 = %+v", command)
	}
}

// Covers: 有效性修订产生的那一版只把「是修订」一格交给编排，判不判归编排。
func TestARevisedOccurrenceIsMarkedForTheTrigger(t *testing.T) {
	original := trunkOccurrence(t)
	revised := must[tfdomain.TransportChargeOccurrence](t)(original.ReviseValidity(
		tfdomain.OccurrenceInvalidated,
		must[tfdomain.OccurrenceValidityVersion](t)(tfdomain.NewOccurrenceValidityVersion("V2")),
		must[tfdomain.OccurrenceBasisReference](t)(tfdomain.NewOccurrenceBasisReference("SYN-CORRECTION-01")),
		occurredAt.Add(time.Hour)))
	trigger := &triggerDouble{}
	_ = newAdapter(t, sourceWith(revised), trigger).HandleFormedChargeOccurrence(t.Context(), formedOf(revised))
	if len(trigger.commands) != 1 || !trigger.commands[0].Revised {
		t.Fatalf("修订版没被标出：%+v", trigger.commands)
	}
}

// Covers: 读不回与读不动都是可见性未决，交 ErrChargeOccurrenceNotVisible；读回的键对不上是提供方不变量已破，另一枚哨兵；
// 两格都不调编排。
func TestAnOccurrenceThatCannotBeReadBackStopsBeforeTheTrigger(t *testing.T) {
	occurrence := trunkOccurrence(t)
	elsewhere := trunkOccurrence(t)
	mismatched := sourceWith(elsewhere)
	key := keyOf(occurrence)
	other := key
	other.Validity = must[tfdomain.OccurrenceValidityVersion](t)(tfdomain.NewOccurrenceValidityVersion("V9"))
	mismatched.records = map[tfports.ChargeOccurrenceKey]tfports.ChargeOccurrenceRecord{
		key: {Key: other, Occurrence: elsewhere, RecordedAt: occurredAt},
	}
	for name, testCase := range map[string]struct {
		source occurrenceSourceDouble
		want   error
	}{
		"读不回": {sourceWith(), adapter.ErrChargeOccurrenceNotVisible},
		"读不动": {occurrenceSourceDouble{err: errors.New("registry down")}, adapter.ErrChargeOccurrenceNotVisible},
		"键不符": {mismatched, adapter.ErrChargeOccurrenceInconsistent},
	} {
		trigger := &triggerDouble{}
		err := newAdapter(t, testCase.source, trigger).HandleFormedChargeOccurrence(t.Context(), formedOf(occurrence))
		if !errors.Is(err, testCase.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, testCase.want)
		}
		if len(trigger.commands) != 0 {
			t.Fatalf("%s: 编排被调了", name)
		}
	}
}

// 下面几只替身只为在本包里装起真的触发编排：编排结果的字段不导出，各格只能由编排自己走出来。

type triggerViewDouble struct {
	feeItem string
	err     error
}

func (double triggerViewDouble) LoadBuyEvaluationTrigger(
	_ context.Context, _ sadomain.TenantID, reason sadomain.OccurrenceReasonReference,
) (sadomain.BuyEvaluationTriggerRegistration, bool, error) {
	if double.err != nil {
		return sadomain.BuyEvaluationTriggerRegistration{}, false, double.err
	}
	registration, err := sadomain.NewBuyEvaluationTriggerRegistration(reason, sadomain.BuyEvaluationTriggerOnOccurrenceFormed)
	if err != nil || double.feeItem == "" {
		return registration, err == nil, err
	}
	feeItem, err := sadomain.NewFeeItemReference(double.feeItem)
	if err != nil {
		return sadomain.BuyEvaluationTriggerRegistration{}, false, err
	}
	registration, err = registration.WithFeeItem(feeItem)
	return registration, err == nil, err
}

type scopeViewDouble struct{ found bool }

func (double scopeViewDouble) LoadSupplierAgreementScope(
	context.Context, sadomain.TenantID, sadomain.SupplierAgreementReference,
) (sadomain.PrimaryScopeReference, bool, error) {
	if !double.found {
		return sadomain.PrimaryScopeReference{}, false, nil
	}
	scope, err := sadomain.NewPrimaryScopeReference("SYN-SCOPE-01")
	return scope, err == nil, err
}

type registryDouble struct {
	saved []saports.EvaluationRequestRecord
}

func (double *registryDouble) Save(_ context.Context, record saports.EvaluationRequestRecord) (saports.EvaluationRequestSaveOutcome, error) {
	double.saved = append(double.saved, record)
	return saports.EvaluationRequestSaved, nil
}

func (double *registryDouble) FindByNaturalKey(
	context.Context, sadomain.TenantID, sadomain.EvaluationRequestNaturalKey,
) (saports.EvaluationRequestRecord, bool, error) {
	return saports.EvaluationRequestRecord{}, false, nil
}

type identityDouble struct{}

func (identityDouble) MintEvaluationRequestID(context.Context) (sadomain.EvaluationRequestID, error) {
	return sadomain.NewEvaluationRequestID("EVREQ-SYN-1")
}

type handoffDouble struct{}

func (handoffDouble) HandOffEvaluationRequest(context.Context, saports.EvaluationRequestIntent) error {
	return nil
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return occurredAt.Add(time.Minute) }

func realTrigger(t *testing.T, view triggerViewDouble, scopes scopeViewDouble) (*saapplication.TriggerBuyEvaluationOnOccurrenceFormedHandler, *registryDouble) {
	t.Helper()
	registry := &registryDouble{}
	requests := must[*saapplication.RequestBuyEvaluationHandler](t)(saapplication.NewRequestBuyEvaluationHandler(saapplication.RequestBuyEvaluationDeps{
		Registry: registry, Identity: identityDouble{}, Downstream: handoffDouble{}, Clock: fixedClock{},
	}))
	trigger := must[*saapplication.TriggerBuyEvaluationOnOccurrenceFormedHandler](t)(saapplication.NewTriggerBuyEvaluationOnOccurrenceFormedHandler(
		saapplication.TriggerBuyEvaluationOnOccurrenceFormedDeps{Triggers: view, Agreements: scopes, Requests: requests}))
	return trigger, registry
}

// Covers: 编排各格的去向——发起、未配置、修订版入账；未决交 ErrBuyEvaluationTriggerUndecided；协议在 PC 不认的矛盾
// 原样上抛，不当未决。
func TestTriggerOutcomesLandAsConsumption(t *testing.T) {
	original := trunkOccurrence(t)
	revised := must[tfdomain.TransportChargeOccurrence](t)(original.ReviseValidity(
		tfdomain.OccurrenceSuperseded,
		must[tfdomain.OccurrenceValidityVersion](t)(tfdomain.NewOccurrenceValidityVersion("V2")),
		must[tfdomain.OccurrenceBasisReference](t)(tfdomain.NewOccurrenceBasisReference("SYN-CORRECTION-01")),
		occurredAt.Add(time.Hour)))
	for name, testCase := range map[string]struct {
		occurrence tfdomain.TransportChargeOccurrence
		view       triggerViewDouble
		scopes     scopeViewDouble
		want       error
		requests   int
	}{
		"发起":   {original, triggerViewDouble{feeItem: "SYN-FEE-ITEM-TRUNK-COST-01"}, scopeViewDouble{found: true}, nil, 1},
		"未配置":  {original, triggerViewDouble{}, scopeViewDouble{found: true}, nil, 0},
		"修订版":  {revised, triggerViewDouble{feeItem: "SYN-FEE-ITEM-TRUNK-COST-01"}, scopeViewDouble{found: true}, nil, 0},
		"未决":   {original, triggerViewDouble{err: errors.New("register down")}, scopeViewDouble{found: true}, adapter.ErrBuyEvaluationTriggerUndecided, 0},
		"协议矛盾": {original, triggerViewDouble{feeItem: "SYN-FEE-ITEM-TRUNK-COST-01"}, scopeViewDouble{}, saapplication.ErrOccurrenceAgreementNotRegistered, 0},
	} {
		trigger, registry := realTrigger(t, testCase.view, testCase.scopes)
		err := newAdapter(t, sourceWith(testCase.occurrence), trigger).HandleFormedChargeOccurrence(t.Context(), formedOf(testCase.occurrence))
		if testCase.want == nil && err != nil {
			t.Fatalf("%s: err = %v, want 入账", name, err)
		}
		if testCase.want != nil && !errors.Is(err, testCase.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, testCase.want)
		}
		if len(registry.saved) != testCase.requests {
			t.Fatalf("%s: 登了 %d 份请求, want %d", name, len(registry.saved), testCase.requests)
		}
	}
}

func TestTheAdapterRefusesMissingCollaborators(t *testing.T) {
	if _, err := adapter.NewRequestOnChargeOccurrenceFormedAdapter(nil, &triggerDouble{}); err == nil {
		t.Fatal("发生项读口缺席却构造成功")
	}
	if _, err := adapter.NewRequestOnChargeOccurrenceFormedAdapter(sourceWith(), nil); err == nil {
		t.Fatal("编排缺席却构造成功")
	}
}
