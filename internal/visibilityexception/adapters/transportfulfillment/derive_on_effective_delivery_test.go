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

var deliveredAt = time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)

func deliveryValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type deliveryFinderDouble struct {
	record tfports.EffectiveDeliveryRecord
	found  bool
	err    error
	last   tfports.EffectiveDeliveryKey
}

func (double *deliveryFinderDouble) FindByKey(
	_ context.Context, key tfports.EffectiveDeliveryKey,
) (tfports.EffectiveDeliveryRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return tfports.EffectiveDeliveryRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type countingProjectionHandler struct {
	calls int
}

func (double *countingProjectionHandler) Handle(
	context.Context, veapplication.DeriveProjectionCommand,
) (veapplication.DeriveProjectionResult, error) {
	double.calls++
	return veapplication.DeriveProjectionResult{}, errors.New("derive should not be called")
}

type deliveryFactStoreDouble struct {
	byKey map[veports.FactKey]veports.FactRecord
}

func (double *deliveryFactStoreDouble) FindByKey(_ context.Context, key veports.FactKey) (veports.FactRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *deliveryFactStoreDouble) FindByParcel(
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

func (double *deliveryFactStoreDouble) Save(_ context.Context, record veports.FactRecord) (veports.FactSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return veports.FactAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	return veports.FactSaved, nil
}

type deliveryMappingViewDouble struct {
	configured bool
	err        error
}

func (double *deliveryMappingViewDouble) ClassifyFact(
	_ context.Context,
	_ vedomain.AcceptedSourceFact,
) (veports.MilestoneAnswer, bool, error) {
	if double.err != nil {
		return veports.MilestoneAnswer{}, false, double.err
	}
	return veports.MilestoneAnswer{}, double.configured, nil
}

type deliveryProjectionStoreDouble struct {
	byKey map[string]vedomain.TrackingProjection
}

func (double *deliveryProjectionStoreDouble) FindCurrent(
	_ context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) (vedomain.TrackingProjection, bool, error) {
	projection, found := double.byKey[tenant.String()+"/"+parcel.String()]
	return projection, found, nil
}

func (double *deliveryProjectionStoreDouble) Save(
	_ context.Context, tenant vedomain.TenantID, projection vedomain.TrackingProjection,
) error {
	double.byKey[tenant.String()+"/"+projection.Parcel().String()] = projection
	return nil
}

type deliveryProjectionIdentityDouble struct{}

func (deliveryProjectionIdentityDouble) NextProjectionVersionID(_ context.Context) (vedomain.ProjectionVersionID, error) {
	return vedomain.NewProjectionVersionID("projection-1")
}

type deliveryProjectionDownstreamDouble struct {
	err error
}

func (double *deliveryProjectionDownstreamDouble) HandOffProjection(
	context.Context, veports.ProjectionHandoffIntent,
) error {
	return double.err
}

type deliveryFixedClock struct{ at time.Time }

func (clock deliveryFixedClock) Now() time.Time { return clock.at }

func registeredDelivery(t *testing.T, object string) tfports.EffectiveDeliveryRecord {
	t.Helper()

	attempt, err := tfdomain.FormFulfillmentAttempt(tfdomain.FulfillmentAttemptSpec{
		TenantID:    deliveryValue(t, tfdomain.NewTenantID, "tenant-1"),
		Attempt:     deliveryValue(t, tfdomain.NewAttemptReference, "attempt-1"),
		Task:        deliveryValue(t, tfdomain.NewDispatchTaskReference, "delivery-task-1"),
		ExecutedBy:  deliveryValue(t, tfdomain.NewExecutingPartyReference, "courier-1"),
		Place:       deliveryValue(t, tfdomain.NewAttemptPlaceReference, "recipient-door"),
		PlannedFrom: deliveredAt.Add(-2 * time.Hour),
		PlannedTo:   deliveredAt.Add(2 * time.Hour),
		ArrivedAt:   deliveredAt.Add(-10 * time.Minute),
		Objects:     []tfdomain.CarriedObjectReference{deliveryValue(t, tfdomain.NewCarriedObjectReference, object)},
		Evidence:    deliveryValue(t, tfdomain.NewAttemptEvidenceReference, "GPS-TRACE/1"),
	})
	if err != nil {
		t.Fatalf("构造履约尝试：%v", err)
	}
	result, err := tfdomain.FormDeliveryAttemptResult(
		attempt,
		deliveryValue(t, tfdomain.NewCarriedObjectReference, object),
		tfdomain.ObjectDelivered,
		tfdomain.AttemptResultBasisReference{},
		deliveredAt,
	)
	if err != nil {
		t.Fatalf("构造妥投结果：%v", err)
	}
	delivery, err := tfdomain.FormEffectiveDelivery(attempt, result, tfdomain.EffectiveDeliverySpec{
		Method:    deliveryValue(t, tfdomain.NewDeliveryMethodReference, "HAND_TO_RECIPIENT"),
		Recipient: deliveryValue(t, tfdomain.NewReceivingPartyReference, "recipient-1"),
		Proof:     deliveryValue(t, tfdomain.NewDeliveryProofReference, "POD-3"),
		Version:   deliveryValue(t, tfdomain.NewDeliveryResultVersion, "delivery-result/v1"),
	})
	if err != nil {
		t.Fatalf("构造有效交付：%v", err)
	}
	return tfports.EffectiveDeliveryRecord{
		Key: tfports.EffectiveDeliveryKey{
			TenantID: deliveryValue(t, tfdomain.NewTenantID, "tenant-1"),
			Object:   deliveryValue(t, tfdomain.NewCarriedObjectReference, object),
			Attempt:  deliveryValue(t, tfdomain.NewAttemptReference, "attempt-1"),
		},
		ContentDigest: "digest-1",
		Delivery:      delivery,
		RecordedAt:    deliveredAt.Add(time.Second),
	}
}

func registeredDeliveryRef() veinbox.RegisteredEffectiveDelivery {
	return veinbox.RegisteredEffectiveDelivery{
		TenantID: "tenant-1",
		Object:   "parcel-1",
		Attempt:  "attempt-1",
	}
}

func deliveryDeriveHandler(t *testing.T, mapping deliveryMappingViewDouble, downstream deliveryProjectionDownstreamDouble) (
	*veapplication.DeriveProjectionHandler,
	*deliveryFactStoreDouble,
	*deliveryProjectionStoreDouble,
) {
	t.Helper()
	facts := &deliveryFactStoreDouble{byKey: map[veports.FactKey]veports.FactRecord{}}
	projections := &deliveryProjectionStoreDouble{byKey: map[string]vedomain.TrackingProjection{}}
	handler := veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:       facts,
		Mapping:     &mapping,
		Projections: projections,
		Identities:  deliveryProjectionIdentityDouble{},
		Downstream:  &downstream,
		Clock:       deliveryFixedClock{at: deliveredAt.Add(2 * time.Hour)},
	})
	return handler, facts, projections
}

func TestAMissingDeliveryIsContinuableUndecided(t *testing.T) {
	derive := &countingProjectionHandler{}
	subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(&deliveryFinderDouble{}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrDeliveryNotVisible) {
		t.Fatalf("err = %v, want ErrDeliveryNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("缺交付记录不该走到派生")
	}
}

func TestAnUnreadableDeliveryIsContinuableUndecided(t *testing.T) {
	derive := &countingProjectionHandler{}
	subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(
		&deliveryFinderDouble{err: errors.New("store unavailable")}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrDeliveryNotVisible) {
		t.Fatalf("err = %v, want ErrDeliveryNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("读失败不该走到派生")
	}
}

func TestAMismatchedDeliveryKeyIsInconsistentNotUndecided(t *testing.T) {
	record := registeredDelivery(t, "parcel-1")
	record.Key.Object = deliveryValue(t, tfdomain.NewCarriedObjectReference, "other-parcel")
	derive := &countingProjectionHandler{}
	subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(
		&deliveryFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrDeliveryRecordInconsistent) {
		t.Fatalf("err = %v, want ErrDeliveryRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("键本体不符不该走到派生")
	}
}

func TestAZeroRecordedAtIsInconsistentNotUndecided(t *testing.T) {
	record := registeredDelivery(t, "parcel-1")
	record.RecordedAt = time.Time{}
	derive := &countingProjectionHandler{}
	subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(
		&deliveryFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrDeliveryRecordInconsistent) {
		t.Fatalf("err = %v, want ErrDeliveryRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("时间为零不该走到派生")
	}
}

func TestAnUntranslatableDeliveryReferenceKeepsItsSentinel(t *testing.T) {
	derive := &countingProjectionHandler{}
	subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(
		&deliveryFinderDouble{record: registeredDelivery(t, "parcel-1"), found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	for name, reference := range map[string]veinbox.RegisteredEffectiveDelivery{
		"空租户": {Object: "parcel-1", Attempt: "attempt-1"},
		"空对象": {TenantID: "tenant-1", Attempt: "attempt-1"},
		"空尝试": {TenantID: "tenant-1", Object: "parcel-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleRegisteredEffectiveDelivery(
				t.Context(), reference); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
			}
		})
	}
	if derive.calls != 0 {
		t.Fatal("引用译不出来不该走到派生")
	}
}

func TestFindByKeyRereadsTheCurrentVersionByKey(t *testing.T) {
	finder := &deliveryFinderDouble{record: registeredDelivery(t, "parcel-1"), found: true}
	handler, _, _ := deliveryDeriveHandler(t, deliveryMappingViewDouble{configured: false}, deliveryProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); err != nil {
		t.Fatalf("处理有效交付：%v", err)
	}
	if finder.last.TenantID.String() != "tenant-1" ||
		finder.last.Object.String() != "parcel-1" ||
		finder.last.Attempt.String() != "attempt-1" {
		t.Fatalf("FindByKey 键 = %+v；必须只按三维键读当前版", finder.last)
	}
}

func TestUnconfiguredMappingDerivesAnUnclassifiedProjection(t *testing.T) {
	handler, facts, projections := deliveryDeriveHandler(t, deliveryMappingViewDouble{configured: false}, deliveryProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(
		&deliveryFinderDouble{record: registeredDelivery(t, "parcel-1"), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); err != nil {
		t.Fatalf("处理有效交付：%v", err)
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
		fact.Fact().String() != "effective-delivery/parcel-1/attempt-1" ||
		fact.Version().String() != "delivery-result/v1" ||
		!fact.OccurredAt().Equal(deliveredAt) ||
		!fact.EffectiveAt().Equal(deliveredAt) ||
		!fact.ReceivedAt().Equal(deliveredAt.Add(time.Second)) {
		t.Fatalf("已接受事实维 = %+v", fact)
	}

	projection, found := projections.byKey["tenant-1/parcel-1"]
	if !found {
		t.Fatal("未配置映射必须照常派生投影——交付投影不是 PS 终局")
	}
	if _, classified := projection.Entries()[0].Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明已交付里程碑")
	}
	if projection.Entries()[0].MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q", projection.Entries()[0].MappingVersion())
	}
}

func TestAFailedProjectionHandoffKeepsThePendingSentinel(t *testing.T) {
	handler, _, _ := deliveryDeriveHandler(t, deliveryMappingViewDouble{configured: false}, deliveryProjectionDownstreamDouble{
		err: errors.New("outbox down"),
	})
	subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(
		&deliveryFinderDouble{record: registeredDelivery(t, "parcel-1"), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrProjectionHandoffPending) {
		t.Fatalf("err = %v, want ErrProjectionHandoffPending", err)
	}
}

func TestMappingViewFailureDoesNotLookLikeUnconfigured(t *testing.T) {
	handler, _, _ := deliveryDeriveHandler(t, deliveryMappingViewDouble{err: errors.New("mapping unreachable")}, deliveryProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnEffectiveDeliveryAdapter(
		&deliveryFinderDouble{record: registeredDelivery(t, "parcel-1"), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrProjectionUndecided) {
		t.Fatalf("err = %v, want ErrProjectionUndecided", err)
	}
}
