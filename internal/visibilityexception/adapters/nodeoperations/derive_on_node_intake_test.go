package nodeoperations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/nodeoperations"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var receivedAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func value[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type receptionDouble struct {
	record noports.ReceptionRecord
	found  bool
	err    error
	last   noports.ReceptionKey
}

func (double *receptionDouble) FindByKey(
	_ context.Context, key noports.ReceptionKey,
) (noports.ReceptionRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return noports.ReceptionRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type countingHandler struct {
	calls int
}

func (double *countingHandler) Handle(
	context.Context, veapplication.DeriveProjectionCommand,
) (veapplication.DeriveProjectionResult, error) {
	double.calls++
	return veapplication.DeriveProjectionResult{}, errors.New("derive should not be called")
}

type factStoreDouble struct {
	byKey map[veports.FactKey]veports.FactRecord
}

func (double *factStoreDouble) FindByKey(_ context.Context, key veports.FactKey) (veports.FactRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *factStoreDouble) FindByParcel(
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

func (double *factStoreDouble) Save(_ context.Context, record veports.FactRecord) (veports.FactSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return veports.FactAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	return veports.FactSaved, nil
}

type mappingViewDouble struct {
	configured bool
	err        error
}

func (double *mappingViewDouble) ClassifyFact(
	_ context.Context,
	_ vedomain.AcceptedSourceFact,
) (veports.MilestoneAnswer, bool, error) {
	if double.err != nil {
		return veports.MilestoneAnswer{}, false, double.err
	}
	return veports.MilestoneAnswer{}, double.configured, nil
}

type projectionStoreDouble struct {
	byKey map[string]vedomain.TrackingProjection
}

func (double *projectionStoreDouble) FindCurrent(
	_ context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) (vedomain.TrackingProjection, bool, error) {
	projection, found := double.byKey[tenant.String()+"/"+parcel.String()]
	return projection, found, nil
}

func (double *projectionStoreDouble) Save(
	_ context.Context, tenant vedomain.TenantID, projection vedomain.TrackingProjection,
) error {
	double.byKey[tenant.String()+"/"+projection.Parcel().String()] = projection
	return nil
}

type projectionIdentityDouble struct{}

func (projectionIdentityDouble) NextProjectionVersionID(_ context.Context) (vedomain.ProjectionVersionID, error) {
	return vedomain.NewProjectionVersionID("projection-1")
}

type projectionDownstreamDouble struct {
	err error
}

func (double *projectionDownstreamDouble) HandOffProjection(
	context.Context, veports.ProjectionHandoffIntent,
) error {
	return double.err
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

func identifiedIntake(t *testing.T) nodomain.NodeIntake {
	t.Helper()
	intake, err := nodomain.FormNodeIntake(nodomain.NodeIntakeSpec{
		TenantID:    value(t, nodomain.NewTenantID, "tenant-1"),
		Unit:        value(t, nodomain.NewHandlingUnitID, "unit-1"),
		Node:        value(t, nodomain.NewNodeReference, "node-origin"),
		DeliveredBy: value(t, nodomain.NewDeliveringPartyReference, "customer-1"),
		Evidence:    value(t, nodomain.NewReceptionEvidenceReference, "SIGN-7"),
		Version:     value(t, nodomain.NewIntakeResultVersion, "intake-result/v1"),
		Association: value(t, nodomain.NewParcelAssociationReference, "parcel-1"),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("form node intake: %v", err)
	}
	return intake
}

func formedReception(t *testing.T) noports.ReceptionRecord {
	t.Helper()
	return noports.ReceptionRecord{
		Key: noports.ReceptionKey{
			TenantID: value(t, nodomain.NewTenantID, "tenant-1"),
			SourceID: "source-1",
		},
		Kind:       noports.RecordIntakeFormed,
		Intake:     identifiedIntake(t),
		RecordedAt: receivedAt.Add(time.Minute),
	}
}

func formedRef() veinbox.FormedNodeIntake {
	return veinbox.FormedNodeIntake{TenantID: "tenant-1", SourceID: "source-1"}
}

func deriveHandler(t *testing.T, mapping mappingViewDouble, downstream projectionDownstreamDouble) (
	*veapplication.DeriveProjectionHandler,
	*factStoreDouble,
	*projectionStoreDouble,
) {
	t.Helper()
	facts := &factStoreDouble{byKey: map[veports.FactKey]veports.FactRecord{}}
	projections := &projectionStoreDouble{byKey: map[string]vedomain.TrackingProjection{}}
	handler := veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:       facts,
		Mapping:     &mapping,
		Projections: projections,
		Identities:  projectionIdentityDouble{},
		Downstream:  &downstream,
		Clock:       fixedClock{at: receivedAt.Add(2 * time.Hour)},
	})
	return handler, facts, projections
}

func TestAMissingReceptionIsContinuableUndecided(t *testing.T) {
	derive := &countingHandler{}
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(&receptionDouble{}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionNotVisible) {
		t.Fatalf("err = %v, want ErrReceptionNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("缺收寄记录不该走到派生")
	}
}

func TestAnUnreadableReceptionIsContinuableUndecided(t *testing.T) {
	derive := &countingHandler{}
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(
		&receptionDouble{err: errors.New("store unavailable")}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionNotVisible) {
		t.Fatalf("err = %v, want ErrReceptionNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("读失败不该走到派生")
	}
}

func TestAPendingIdentificationRecordWaitsForIdentity(t *testing.T) {
	record := formedReception(t)
	record.Kind = noports.RecordPendingIdentification
	derive := &countingHandler{}
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(
		&receptionDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrUnidentifiedHandlingUnit) {
		t.Fatalf("err = %v, want ErrUnidentifiedHandlingUnit", err)
	}
	if derive.calls != 0 {
		t.Fatal("待识别格不该走到派生")
	}
}

func TestAnUnidentifiedFormedIntakeWaitsForIdentity(t *testing.T) {
	pending, err := nodomain.FormNodeIntake(nodomain.NodeIntakeSpec{
		TenantID:    value(t, nodomain.NewTenantID, "tenant-1"),
		Unit:        value(t, nodomain.NewHandlingUnitID, "unit-1"),
		Node:        value(t, nodomain.NewNodeReference, "node-origin"),
		DeliveredBy: value(t, nodomain.NewDeliveringPartyReference, "customer-1"),
		Evidence:    value(t, nodomain.NewReceptionEvidenceReference, "SIGN-7"),
		Version:     value(t, nodomain.NewIntakeResultVersion, "intake-result/v1"),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("构造待识别收寄：%v", err)
	}
	record := formedReception(t)
	record.Intake = pending
	derive := &countingHandler{}
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(
		&receptionDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrUnidentifiedHandlingUnit) {
		t.Fatalf("err = %v, want ErrUnidentifiedHandlingUnit", err)
	}
	if derive.calls != 0 {
		t.Fatal("无关联的形成格不该走到派生")
	}
}

func TestAnIntakeThatWasNotFormedIsAccountedWithoutDeriving(t *testing.T) {
	record := formedReception(t)
	record.Kind = noports.RecordIntakeNotFormed
	derive := &countingHandler{}
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(
		&receptionDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); err != nil {
		t.Fatalf("未形成应收寄跳过入账，got %v", err)
	}
	if derive.calls != 0 {
		t.Fatal("未形成收寄不该走到派生")
	}
}

func TestAMismatchedReceptionKeyIsInconsistentNotUndecided(t *testing.T) {
	record := formedReception(t)
	record.Key.SourceID = "other-source"
	derive := &countingHandler{}
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(
		&receptionDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionRecordInconsistent) {
		t.Fatalf("err = %v, want ErrReceptionRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("键本体不符不该走到派生")
	}
}

func TestAZeroRecordedAtIsInconsistentNotUndecided(t *testing.T) {
	record := formedReception(t)
	record.RecordedAt = time.Time{}
	derive := &countingHandler{}
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(
		&receptionDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionRecordInconsistent) {
		t.Fatalf("err = %v, want ErrReceptionRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("时间为零不该走到派生")
	}
}

func TestUnconfiguredMappingDerivesAnUnclassifiedProjection(t *testing.T) {
	handler, facts, projections := deriveHandler(t, mappingViewDouble{configured: false}, projectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(
		&receptionDouble{record: formedReception(t), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); err != nil {
		t.Fatalf("处理形成收寄：%v", err)
	}

	if len(facts.byKey) != 1 {
		t.Fatalf("事实条数 = %d", len(facts.byKey))
	}
	var fact vedomain.AcceptedSourceFact
	for _, record := range facts.byKey {
		fact = record.Fact
	}
	if fact.Source() != vedomain.SourceNodeOperations ||
		fact.Parcel().String() != "parcel-1" ||
		fact.Fact().String() != "source-1" ||
		fact.Version().String() != "intake-result/v1" ||
		!fact.OccurredAt().Equal(receivedAt) ||
		!fact.EffectiveAt().Equal(receivedAt) ||
		!fact.ReceivedAt().Equal(receivedAt.Add(time.Minute)) {
		t.Fatalf("已接受事实维 = %+v", fact)
	}

	projection, found := projections.byKey["tenant-1/parcel-1"]
	if !found {
		t.Fatal("未配置映射必须照常派生投影")
	}
	if _, classified := projection.Entries()[0].Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明已收寄里程碑")
	}
	if projection.Entries()[0].MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q", projection.Entries()[0].MappingVersion())
	}
}

func TestAFailedProjectionHandoffKeepsThePendingSentinel(t *testing.T) {
	handler, _, _ := deriveHandler(t, mappingViewDouble{configured: false}, projectionDownstreamDouble{
		err: errors.New("outbox down"),
	})
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(
		&receptionDouble{record: formedReception(t), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrProjectionHandoffPending) {
		t.Fatalf("err = %v, want ErrProjectionHandoffPending", err)
	}
}

func TestMappingViewFailureDoesNotLookLikeUnconfigured(t *testing.T) {
	handler, _, _ := deriveHandler(t, mappingViewDouble{err: errors.New("mapping unreachable")}, projectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnNodeIntakeAdapter(
		&receptionDouble{record: formedReception(t), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrProjectionUndecided) {
		t.Fatalf("err = %v, want ErrProjectionUndecided", err)
	}
}
