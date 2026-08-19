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

var handoverJudgedAt = time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC)

func handoverValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type handoverFinderDouble struct {
	record tfports.TransportHandoverRecord
	found  bool
	err    error
	last   tfports.TransportHandoverKey
}

func (double *handoverFinderDouble) FindByKey(
	_ context.Context, key tfports.TransportHandoverKey,
) (tfports.TransportHandoverRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return tfports.TransportHandoverRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type handoverCountingHandler struct {
	calls int
}

func (double *handoverCountingHandler) Handle(
	context.Context, veapplication.DeriveProjectionCommand,
) (veapplication.DeriveProjectionResult, error) {
	double.calls++
	return veapplication.DeriveProjectionResult{}, errors.New("derive should not be called")
}

type handoverFactStoreDouble struct {
	byKey map[veports.FactKey]veports.FactRecord
}

func (double *handoverFactStoreDouble) FindByKey(_ context.Context, key veports.FactKey) (veports.FactRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *handoverFactStoreDouble) FindByParcel(
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

func (double *handoverFactStoreDouble) Save(_ context.Context, record veports.FactRecord) (veports.FactSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return veports.FactAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	return veports.FactSaved, nil
}

type handoverMappingViewDouble struct {
	configured bool
	err        error
}

func (double *handoverMappingViewDouble) ClassifyFact(
	_ context.Context,
	_ vedomain.AcceptedSourceFact,
) (veports.MilestoneAnswer, bool, error) {
	if double.err != nil {
		return veports.MilestoneAnswer{}, false, double.err
	}
	return veports.MilestoneAnswer{}, double.configured, nil
}

type handoverProjectionStoreDouble struct {
	byKey map[string]vedomain.TrackingProjection
}

func (double *handoverProjectionStoreDouble) FindCurrent(
	_ context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) (vedomain.TrackingProjection, bool, error) {
	projection, found := double.byKey[tenant.String()+"/"+parcel.String()]
	return projection, found, nil
}

func (double *handoverProjectionStoreDouble) Save(
	_ context.Context, tenant vedomain.TenantID, projection vedomain.TrackingProjection,
) error {
	double.byKey[tenant.String()+"/"+projection.Parcel().String()] = projection
	return nil
}

type handoverProjectionIdentityDouble struct{}

func (handoverProjectionIdentityDouble) NextProjectionVersionID(_ context.Context) (vedomain.ProjectionVersionID, error) {
	return vedomain.NewProjectionVersionID("projection-handover-1")
}

type handoverProjectionDownstreamDouble struct {
	err error
}

func (double *handoverProjectionDownstreamDouble) HandOffProjection(
	context.Context, veports.ProjectionHandoffIntent,
) error {
	return double.err
}

type handoverClock struct{ at time.Time }

func (clock handoverClock) Now() time.Time { return clock.at }

func registeredHandover(t *testing.T, verdict tfdomain.HandoverVerdict) tfports.TransportHandoverRecord {
	t.Helper()
	spec := tfdomain.TransportHandoverSpec{
		TenantID:   handoverValue(t, tfdomain.NewTenantID, "tenant-1"),
		Object:     handoverValue(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
		Scope:      handoverValue(t, tfdomain.NewHandoverScopeReference, "scope-1"),
		ReleasedBy: handoverValue(t, tfdomain.NewHandoverPartyReference, "node-1"),
		ReceivedBy: handoverValue(t, tfdomain.NewHandoverPartyReference, "carrier-1"),
		Verdict:    verdict,
		Version:    handoverValue(t, tfdomain.NewHandoverResultVersion, "handover-result/v1"),
		JudgedAt:   handoverJudgedAt,
	}
	if verdict == tfdomain.ObjectHandedOver {
		spec.ReleasingEvidence = handoverValue(t, tfdomain.NewHandoverEvidenceReference, "seal-out")
		spec.ReceivingEvidence = handoverValue(t, tfdomain.NewHandoverEvidenceReference, "seal-in")
		spec.Rule = handoverValue(t, tfdomain.NewHandoverRuleReference, "rule/v1")
	} else {
		spec.Basis = handoverValue(t, tfdomain.NewHandoverBasisReference, "gap-1")
	}
	handover, err := tfdomain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("构造交接：%v", err)
	}
	return tfports.TransportHandoverRecord{
		Key: tfports.TransportHandoverKey{
			TenantID: handoverValue(t, tfdomain.NewTenantID, "tenant-1"),
			Object:   handoverValue(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
			Scope:    handoverValue(t, tfdomain.NewHandoverScopeReference, "scope-1"),
			Version:  handoverValue(t, tfdomain.NewHandoverResultVersion, "handover-result/v1"),
		},
		ContentDigest: "digest-handover",
		Handover:      handover,
		RecordedAt:    handoverJudgedAt.Add(time.Second),
	}
}

func registeredHandoverRef() veinbox.RegisteredTransportHandover {
	return veinbox.RegisteredTransportHandover{
		TenantID: "tenant-1",
		Object:   "parcel-1",
		Scope:    "scope-1",
		Version:  "handover-result/v1",
	}
}

func handoverDeriveHandler(t *testing.T, mapping handoverMappingViewDouble, downstream handoverProjectionDownstreamDouble) (
	*veapplication.DeriveProjectionHandler,
	*handoverFactStoreDouble,
	*handoverProjectionStoreDouble,
) {
	t.Helper()
	facts := &handoverFactStoreDouble{byKey: map[veports.FactKey]veports.FactRecord{}}
	projections := &handoverProjectionStoreDouble{byKey: map[string]vedomain.TrackingProjection{}}
	handler := veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:       facts,
		Mapping:     &mapping,
		Projections: projections,
		Identities:  handoverProjectionIdentityDouble{},
		Downstream:  &downstream,
		Clock:       handoverClock{at: handoverJudgedAt.Add(2 * time.Hour)},
	})
	return handler, facts, projections
}

func TestAMissingHandoverIsContinuableUndecided(t *testing.T) {
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnTransportHandoverAdapter(&handoverFinderDouble{}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); !errors.Is(err, adapter.ErrHandoverNotVisible) {
		t.Fatalf("err = %v, want ErrHandoverNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("缺交接记录不该走到派生")
	}
}

func TestAnUnreadableHandoverIsContinuableUndecided(t *testing.T) {
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnTransportHandoverAdapter(
		&handoverFinderDouble{err: errors.New("store unavailable")}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); !errors.Is(err, adapter.ErrHandoverNotVisible) {
		t.Fatalf("err = %v, want ErrHandoverNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("读失败不该走到派生")
	}
}

func TestAMismatchedHandoverKeyIsInconsistentNotUndecided(t *testing.T) {
	record := registeredHandover(t, tfdomain.ObjectHandedOver)
	record.Key.Scope = handoverValue(t, tfdomain.NewHandoverScopeReference, "other-scope")
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnTransportHandoverAdapter(
		&handoverFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); !errors.Is(err, adapter.ErrHandoverRecordInconsistent) {
		t.Fatalf("err = %v, want ErrHandoverRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("键本体不符不该走到派生")
	}
}

func TestAHandoverWithZeroRecordedAtIsInconsistentNotUndecided(t *testing.T) {
	record := registeredHandover(t, tfdomain.ObjectHandedOver)
	record.RecordedAt = time.Time{}
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnTransportHandoverAdapter(
		&handoverFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); !errors.Is(err, adapter.ErrHandoverRecordInconsistent) {
		t.Fatalf("err = %v, want ErrHandoverRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("时间为零不该走到派生")
	}
}

func TestAnUntranslatableHandoverReferenceKeepsItsSentinel(t *testing.T) {
	derive := &handoverCountingHandler{}
	subject, err := adapter.NewDeriveOnTransportHandoverAdapter(
		&handoverFinderDouble{record: registeredHandover(t, tfdomain.ObjectHandedOver), found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	for name, reference := range map[string]veinbox.RegisteredTransportHandover{
		"空租户": {Object: "parcel-1", Scope: "scope-1", Version: "handover-result/v1"},
		"空对象": {TenantID: "tenant-1", Scope: "scope-1", Version: "handover-result/v1"},
		"空范围": {TenantID: "tenant-1", Object: "parcel-1", Version: "handover-result/v1"},
		"空版本": {TenantID: "tenant-1", Object: "parcel-1", Scope: "scope-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleRegisteredTransportHandover(
				t.Context(), reference); !errors.Is(err, adapter.ErrHandoverUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrHandoverUntranslatableAnswer", err)
			}
		})
	}
	if derive.calls != 0 {
		t.Fatal("引用译不出来不该走到派生")
	}
}

func TestFindByKeyLooksUpAllFourHandoverDimensionsIncludingVersion(t *testing.T) {
	finder := &handoverFinderDouble{record: registeredHandover(t, tfdomain.ObjectHandedOver), found: true}
	handler, _, _ := handoverDeriveHandler(t, handoverMappingViewDouble{configured: false}, handoverProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnTransportHandoverAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); err != nil {
		t.Fatalf("处理交接：%v", err)
	}
	if finder.last.TenantID.String() != "tenant-1" ||
		finder.last.Object.String() != "parcel-1" ||
		finder.last.Scope.String() != "scope-1" ||
		finder.last.Version.String() != "handover-result/v1" {
		t.Fatalf("FindByKey 键 = %+v；必须含 version", finder.last)
	}
}

func TestUnconfiguredMappingDerivesAnUnclassifiedHandoverProjection(t *testing.T) {
	handler, facts, projections := handoverDeriveHandler(t, handoverMappingViewDouble{configured: false}, handoverProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnTransportHandoverAdapter(
		&handoverFinderDouble{record: registeredHandover(t, tfdomain.ObjectHandedOver), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); err != nil {
		t.Fatalf("处理交接：%v", err)
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
		fact.Fact().String() != "transport-handover/parcel-1/scope-1" ||
		fact.Kind().String() != "handover-handed-over" ||
		fact.Version().String() != "handover-result/v1" ||
		!fact.OccurredAt().Equal(handoverJudgedAt) ||
		!fact.EffectiveAt().Equal(handoverJudgedAt) ||
		!fact.ReceivedAt().Equal(handoverJudgedAt.Add(time.Second)) {
		t.Fatalf("已接受事实维 = %+v kind=%q", fact, fact.Kind())
	}

	projection, found := projections.byKey["tenant-1/parcel-1"]
	if !found {
		t.Fatal("未配置映射必须照常派生投影——交接投影不是控制转出")
	}
	if _, classified := projection.Entries()[0].Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明已交接里程碑")
	}
	if projection.Entries()[0].MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q", projection.Entries()[0].MappingVersion())
	}
}

func TestThreeHandoverVerdictsWriteDistinctFactKinds(t *testing.T) {
	want := map[tfdomain.HandoverVerdict]string{
		tfdomain.ObjectHandedOver:            "handover-handed-over",
		tfdomain.HandoverRefused:             "handover-refused",
		tfdomain.HandoverPendingConfirmation: "handover-pending-confirmation",
	}
	for verdict, kind := range want {
		t.Run(kind, func(t *testing.T) {
			handler, facts, _ := handoverDeriveHandler(t, handoverMappingViewDouble{configured: false}, handoverProjectionDownstreamDouble{})
			subject, err := adapter.NewDeriveOnTransportHandoverAdapter(
				&handoverFinderDouble{record: registeredHandover(t, verdict), found: true}, handler)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			if err := subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); err != nil {
				t.Fatalf("处理交接：%v", err)
			}
			if len(facts.byKey) != 1 {
				t.Fatalf("事实条数 = %d", len(facts.byKey))
			}
			var got string
			for _, record := range facts.byKey {
				got = record.Fact.Kind().String()
			}
			if got != kind {
				t.Fatalf("kind = %q, want %q", got, kind)
			}
		})
	}
}

func TestAFailedHandoverProjectionHandoffKeepsThePendingSentinel(t *testing.T) {
	handler, _, _ := handoverDeriveHandler(t, handoverMappingViewDouble{configured: false}, handoverProjectionDownstreamDouble{
		err: errors.New("outbox down"),
	})
	subject, err := adapter.NewDeriveOnTransportHandoverAdapter(
		&handoverFinderDouble{record: registeredHandover(t, tfdomain.ObjectHandedOver), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); !errors.Is(err, adapter.ErrHandoverProjectionHandoffPending) {
		t.Fatalf("err = %v, want ErrHandoverProjectionHandoffPending", err)
	}
}

func TestHandoverMappingViewFailureDoesNotLookLikeUnconfigured(t *testing.T) {
	handler, _, _ := handoverDeriveHandler(t, handoverMappingViewDouble{err: errors.New("mapping unreachable")}, handoverProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnTransportHandoverAdapter(
		&handoverFinderDouble{record: registeredHandover(t, tfdomain.ObjectHandedOver), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); !errors.Is(err, adapter.ErrHandoverProjectionUndecided) {
		t.Fatalf("err = %v, want ErrHandoverProjectionUndecided", err)
	}
}
