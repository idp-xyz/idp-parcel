package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var pickedUpAt = time.Date(2026, 8, 9, 8, 15, 0, 0, time.UTC)

func pickupValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type pickupRegistryDouble struct {
	record tfports.OffsitePickupRecord
	found  bool
	err    error
	last   tfports.OffsitePickupKey
}

func (double *pickupRegistryDouble) FindByKey(
	_ context.Context, key tfports.OffsitePickupKey,
) (tfports.OffsitePickupRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return tfports.OffsitePickupRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type pickupCountingHandler struct {
	calls int
}

func (double *pickupCountingHandler) Handle(
	context.Context, veapplication.DeriveProjectionCommand,
) (veapplication.DeriveProjectionResult, error) {
	double.calls++
	return veapplication.DeriveProjectionResult{}, errors.New("derive should not be called")
}

type pickupFactStoreDouble struct {
	byKey map[veports.FactKey]veports.FactRecord
}

func (double *pickupFactStoreDouble) FindByKey(_ context.Context, key veports.FactKey) (veports.FactRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *pickupFactStoreDouble) FindByParcel(
	_ context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) ([]veports.FactRecord, error) {
	records := make([]veports.FactRecord, 0)
	for _, record := range double.byKey {
		if record.Key.Tenant == tenant && record.Fact.Parcel() == parcel {
			records = append(records, record)
		}
	}
	return records, nil
}

func (double *pickupFactStoreDouble) Save(_ context.Context, record veports.FactRecord) (veports.FactSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return veports.FactAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	return veports.FactSaved, nil
}

type pickupMappingViewDouble struct {
	configured bool
	err        error
}

func (double *pickupMappingViewDouble) ClassifyFact(
	_ context.Context,
	_ vedomain.AcceptedSourceFact,
) (veports.MilestoneAnswer, bool, error) {
	if double.err != nil {
		return veports.MilestoneAnswer{}, false, double.err
	}
	return veports.MilestoneAnswer{}, double.configured, nil
}

type pickupProjectionStoreDouble struct {
	byKey map[string]vedomain.TrackingProjection
}

func (double *pickupProjectionStoreDouble) FindCurrent(
	_ context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) (vedomain.TrackingProjection, bool, error) {
	projection, found := double.byKey[tenant.String()+"/"+parcel.String()]
	return projection, found, nil
}

func (double *pickupProjectionStoreDouble) Save(
	_ context.Context, tenant vedomain.TenantID, projection vedomain.TrackingProjection,
) error {
	double.byKey[tenant.String()+"/"+projection.Parcel().String()] = projection
	return nil
}

// 派生编排不消费按版本读回的口（那是审计与读侧的入口）；替身只存当前版，如实答未找到。
func (double *pickupProjectionStoreDouble) FindByVersion(
	_ context.Context,
	_ vedomain.TenantID,
	_ vedomain.ProjectionVersionID,
) (vedomain.TrackingProjection, bool, error) {
	return vedomain.TrackingProjection{}, false, nil
}

type pickupProjectionIdentityDouble struct{}

func (pickupProjectionIdentityDouble) NextProjectionVersionID(_ context.Context) (vedomain.ProjectionVersionID, error) {
	return vedomain.NewProjectionVersionID("projection-1")
}

type pickupProjectionDownstreamDouble struct {
	err error
}

func (double *pickupProjectionDownstreamDouble) HandOffProjection(
	context.Context, veports.ProjectionHandoffIntent,
) error {
	return double.err
}

type pickupClock struct{ at time.Time }

func (clock pickupClock) Now() time.Time { return clock.at }

func registeredPickup(t *testing.T, object string) tfports.OffsitePickupRecord {
	t.Helper()
	pickup, err := tfdomain.FormOffsitePickup(tfdomain.OffsitePickupSpec{
		TenantID:   pickupValue(t, tfdomain.NewTenantID, "tenant-1"),
		Object:     pickupValue(t, tfdomain.NewCarriedObjectReference, object),
		Task:       pickupValue(t, tfdomain.NewPickupTaskReference, "pickup-task-1"),
		Attempt:    pickupValue(t, tfdomain.NewAttemptReference, "attempt-1"),
		Place:      pickupValue(t, tfdomain.NewPickupPlaceReference, "customer-warehouse-1"),
		Control:    pickupValue(t, tfdomain.NewTransportControlReference, "TF-3"),
		ExecutedBy: pickupValue(t, tfdomain.NewExecutingPartyReference, "courier-1"),
		Version:    pickupValue(t, tfdomain.NewPickupResultVersion, "pickup-result/v1"),
		OccurredAt: pickedUpAt,
	})
	if err != nil {
		t.Fatalf("构造对象级揽收：%v", err)
	}
	return tfports.OffsitePickupRecord{
		Key: tfports.OffsitePickupKey{
			TenantID: pickupValue(t, tfdomain.NewTenantID, "tenant-1"),
			Object:   pickupValue(t, tfdomain.NewCarriedObjectReference, object),
			Attempt:  pickupValue(t, tfdomain.NewAttemptReference, "attempt-1"),
		},
		ContentDigest: "digest-1",
		Pickup:        pickup,
		RecordedAt:    pickedUpAt.Add(time.Second),
	}
}

func registeredPickupRef() veinbox.RegisteredOffsitePickup {
	return veinbox.RegisteredOffsitePickup{
		TenantID: "tenant-1",
		Object:   "parcel-1",
		Attempt:  "attempt-1",
	}
}

func pickupDeriveHandler(t *testing.T, mapping pickupMappingViewDouble, downstream pickupProjectionDownstreamDouble) (
	*veapplication.DeriveProjectionHandler,
	*pickupFactStoreDouble,
	*pickupProjectionStoreDouble,
) {
	t.Helper()
	facts := &pickupFactStoreDouble{byKey: map[veports.FactKey]veports.FactRecord{}}
	projections := &pickupProjectionStoreDouble{byKey: map[string]vedomain.TrackingProjection{}}
	handler := veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:       facts,
		Mapping:     &mapping,
		Projections: projections,
		Identities:  pickupProjectionIdentityDouble{},
		Downstream:  &downstream,
		Clock:       pickupClock{at: pickedUpAt.Add(2 * time.Hour)},
	})
	return handler, facts, projections
}

func TestAMissingPickupRegistrationIsContinuableUndecided(t *testing.T) {
	derive := &pickupCountingHandler{}
	subject, err := adapter.NewDeriveOnOffsitePickupAdapter(&pickupRegistryDouble{}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); !errors.Is(err, adapter.ErrPickupNotVisible) {
		t.Fatalf("err = %v, want ErrPickupNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("缺揽收登记不该走到派生")
	}
}

func TestAnUnreadablePickupRegistryIsContinuableUndecided(t *testing.T) {
	derive := &pickupCountingHandler{}
	subject, err := adapter.NewDeriveOnOffsitePickupAdapter(
		&pickupRegistryDouble{err: errors.New("store unavailable")}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); !errors.Is(err, adapter.ErrPickupNotVisible) {
		t.Fatalf("err = %v, want ErrPickupNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("读失败不该走到派生")
	}
}

func TestAPickupRecordThatDisagreesWithItsKeyIsInconsistentNotUndecided(t *testing.T) {
	record := registeredPickup(t, "parcel-1")
	record.Key.Object = pickupValue(t, tfdomain.NewCarriedObjectReference, "other-parcel")
	derive := &pickupCountingHandler{}
	subject, err := adapter.NewDeriveOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); !errors.Is(err, adapter.ErrPickupRecordInconsistent) {
		t.Fatalf("err = %v, want ErrPickupRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("键本体不符不该走到派生")
	}
}

func TestAPickupWithZeroRecordedAtIsInconsistentNotUndecided(t *testing.T) {
	record := registeredPickup(t, "parcel-1")
	record.RecordedAt = time.Time{}
	derive := &pickupCountingHandler{}
	subject, err := adapter.NewDeriveOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); !errors.Is(err, adapter.ErrPickupRecordInconsistent) {
		t.Fatalf("err = %v, want ErrPickupRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("时间为零不该走到派生")
	}
}

func TestUnconfiguredMappingDerivesAnUnclassifiedPickupProjection(t *testing.T) {
	handler, facts, projections := pickupDeriveHandler(t, pickupMappingViewDouble{configured: false}, pickupProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); err != nil {
		t.Fatalf("处理揽收登记：%v", err)
	}

	if len(facts.byKey) != 1 {
		t.Fatalf("事实条数 = %d", len(facts.byKey))
	}
	var fact vedomain.AcceptedSourceFact
	for _, record := range facts.byKey {
		fact = record.Fact
	}
	if fact.Source() != vedomain.SourceTransportFulfillment ||
		fact.Parcel().String() != "parcel-1" ||
		fact.Fact().String() != "offsite-pickup/parcel-1/attempt-1" ||
		fact.Kind().String() != "offsite-pickup" ||
		fact.Version().String() != "pickup-result/v1" ||
		!fact.OccurredAt().Equal(pickedUpAt) ||
		!fact.EffectiveAt().Equal(pickedUpAt) ||
		!fact.ReceivedAt().Equal(pickedUpAt.Add(time.Second)) {
		t.Fatalf("已接受事实维 = %+v", fact)
	}

	projection, found := projections.byKey["tenant-1/parcel-1"]
	if !found {
		t.Fatal("未配置映射必须照常派生投影")
	}
	if _, classified := projection.Entries()[0].Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明已揽收里程碑")
	}
	if projection.Entries()[0].MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q", projection.Entries()[0].MappingVersion())
	}
}

func TestAConsolidationUnitLikeObjectIsUsedAsParcelReference(t *testing.T) {
	handler, facts, projections := pickupDeriveHandler(t, pickupMappingViewDouble{configured: false}, pickupProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "unit-cons-1"), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	ref := veinbox.RegisteredOffsitePickup{
		TenantID: "tenant-1",
		Object:   "unit-cons-1",
		Attempt:  "attempt-1",
	}
	if err := subject.HandleRegisteredOffsitePickup(t.Context(), ref); err != nil {
		t.Fatalf("集运单元号原样当包裹引用必须入账，got %v", err)
	}
	var fact vedomain.AcceptedSourceFact
	for _, record := range facts.byKey {
		fact = record.Fact
	}
	if fact.Parcel().String() != "unit-cons-1" {
		t.Fatalf("parcel = %q, want unit-cons-1；不得猜、不得跳过", fact.Parcel())
	}
	if fact.Fact().String() != "offsite-pickup/unit-cons-1/attempt-1" {
		t.Fatalf("fact = %q", fact.Fact())
	}
	if _, found := projections.byKey["tenant-1/unit-cons-1"]; !found {
		t.Fatal("集运单元号必须能锚一份投影")
	}
}

func TestAFailedPickupProjectionHandoffKeepsThePendingSentinel(t *testing.T) {
	handler, _, _ := pickupDeriveHandler(t, pickupMappingViewDouble{configured: false}, pickupProjectionDownstreamDouble{
		err: errors.New("outbox down"),
	})
	subject, err := adapter.NewDeriveOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); !errors.Is(err, adapter.ErrPickupProjectionHandoffPending) {
		t.Fatalf("err = %v, want ErrPickupProjectionHandoffPending", err)
	}
}

func TestPickupMappingViewFailureDoesNotLookLikeUnconfigured(t *testing.T) {
	handler, _, _ := pickupDeriveHandler(t, pickupMappingViewDouble{err: errors.New("mapping unreachable")}, pickupProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); !errors.Is(err, adapter.ErrPickupProjectionUndecided) {
		t.Fatalf("err = %v, want ErrPickupProjectionUndecided", err)
	}
}

func TestAnUntranslatablePickupReferenceKeepsItsSentinel(t *testing.T) {
	derive := &pickupCountingHandler{}
	subject, err := adapter.NewDeriveOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(t.Context(), veinbox.RegisteredOffsitePickup{
		TenantID: "  ",
		Object:   "parcel-1",
		Attempt:  "attempt-1",
	}); !errors.Is(err, adapter.ErrPickupUntranslatableAnswer) {
		t.Fatalf("err = %v, want ErrPickupUntranslatableAnswer", err)
	}
	if derive.calls != 0 {
		t.Fatal("译不出的引用不该走到派生")
	}
}
