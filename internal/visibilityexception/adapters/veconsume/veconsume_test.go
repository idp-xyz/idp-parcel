package veconsume_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var factOccurredAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type factStoreDouble struct {
	byKey map[ports.FactKey]ports.FactRecord
}

func (double *factStoreDouble) FindByKey(_ context.Context, key ports.FactKey) (ports.FactRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *factStoreDouble) FindByParcel(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
) ([]ports.FactRecord, error) {
	records := make([]ports.FactRecord, 0)
	for _, record := range double.byKey {
		if record.Key.Tenant == tenant && record.Fact.Parcel() == parcel {
			records = append(records, record)
		}
	}
	return records, nil
}

func (double *factStoreDouble) Save(_ context.Context, record ports.FactRecord) (ports.FactSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return ports.FactAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	return ports.FactSaved, nil
}

type mappingViewDouble struct {
	configured bool
	err        error
}

func (double *mappingViewDouble) ClassifyFact(
	_ context.Context,
	_ domain.AcceptedSourceFact,
) (ports.MilestoneAnswer, bool, error) {
	if double.err != nil {
		return ports.MilestoneAnswer{}, false, double.err
	}
	return ports.MilestoneAnswer{}, double.configured, nil
}

type projectionStoreDouble struct {
	byKey map[string]domain.TrackingProjection
}

func (double *projectionStoreDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.TrackedParcelReference,
) (domain.TrackingProjection, bool, error) {
	projection, found := double.byKey[tenant.String()+"/"+parcel.String()]
	return projection, found, nil
}

func (double *projectionStoreDouble) Save(
	_ context.Context, tenant domain.TenantID, projection domain.TrackingProjection,
) error {
	double.byKey[tenant.String()+"/"+projection.Parcel().String()] = projection
	return nil
}

type projectionIdentityDouble struct{ next int }

func (double *projectionIdentityDouble) NextProjectionVersionID(_ context.Context) (domain.ProjectionVersionID, error) {
	double.next++
	return domain.NewProjectionVersionID("projection-1")
}

type projectionDownstreamDouble struct {
	err error
}

func (double *projectionDownstreamDouble) HandOffProjection(
	context.Context, ports.ProjectionHandoffIntent,
) error {
	return double.err
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

func deriveCommand(t *testing.T) application.DeriveProjectionCommand {
	t.Helper()
	return application.DeriveProjectionCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Fact: domain.AcceptedSourceFactSpec{
			Source:      domain.SourceNodeOperations,
			Parcel:      mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
			Fact:        mustValue(t, domain.NewSourceFactReference, "source-1"),
			Kind:        mustValue(t, domain.NewSourceFactKind, "node-intake"),
			Version:     mustValue(t, domain.NewSourceFactVersion, "intake-result/v1"),
			OccurredAt:  factOccurredAt,
			EffectiveAt: factOccurredAt,
			ReceivedAt:  factOccurredAt.Add(time.Minute),
		},
	}
}

func newHandler(t *testing.T, mapping mappingViewDouble, downstream projectionDownstreamDouble) *application.DeriveProjectionHandler {
	t.Helper()
	return application.NewDeriveProjectionHandler(application.DeriveProjectionDeps{
		Facts:       &factStoreDouble{byKey: map[ports.FactKey]ports.FactRecord{}},
		Mapping:     &mapping,
		Projections: &projectionStoreDouble{byKey: map[string]domain.TrackingProjection{}},
		Identities:  &projectionIdentityDouble{},
		Downstream:  &downstream,
		Clock:       fixedClock{at: factOccurredAt.Add(2 * time.Hour)},
	})
}

func TestAnUnmappedProjectionOutcomeIsNotSilentlyConsumed(t *testing.T) {
	err := veconsume.Consumption(application.DeriveProjectionResult{})
	if !errors.Is(err, veconsume.ErrUnexpectedProjectionOutcome) {
		t.Fatalf("err = %v, want ErrUnexpectedProjectionOutcome", err)
	}
}

func TestUnconfiguredMappingIsAccountedAsDerivedNotUnavailable(t *testing.T) {
	handler := newHandler(t, mappingViewDouble{configured: false}, projectionDownstreamDouble{})
	result, err := handler.Handle(context.Background(), deriveCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ProjectionDerived {
		t.Fatalf("outcome = %q; 映射未配置必须派生，不得折成未决", result.Outcome())
	}
	if err := veconsume.Consumption(result); err != nil {
		t.Fatalf("consumption: %v; 未归类投影必须入账", err)
	}
}

func TestMappingViewFailureIsUndecidedAndRollsBack(t *testing.T) {
	handler := newHandler(t, mappingViewDouble{err: errors.New("mapping unreachable")}, projectionDownstreamDouble{})
	result, err := handler.Handle(context.Background(), deriveCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DeriveUndecided ||
		result.UndecidedReason() != application.MappingViewUnavailable {
		t.Fatalf("outcome = %q/%q", result.Outcome(), result.UndecidedReason())
	}
	if err := veconsume.Consumption(result); !errors.Is(err, veconsume.ErrProjectionUndecided) {
		t.Fatalf("err = %v, want ErrProjectionUndecided", err)
	}
}

func TestAFailedProjectionHandoffRollsBackTheInbox(t *testing.T) {
	handler := newHandler(t, mappingViewDouble{configured: false}, projectionDownstreamDouble{
		err: errors.New("outbox down"),
	})
	result, err := handler.Handle(context.Background(), deriveCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.HandoffReference() == "" {
		t.Fatal("handoff 失败必须留下续办引用")
	}
	if err := veconsume.Consumption(result); !errors.Is(err, veconsume.ErrProjectionHandoffPending) {
		t.Fatalf("err = %v, want ErrProjectionHandoffPending", err)
	}
}

func TestReplayConflictAndNotAcceptedAreAccounted(t *testing.T) {
	handler := newHandler(t, mappingViewDouble{configured: false}, projectionDownstreamDouble{})
	first, err := handler.Handle(context.Background(), deriveCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if err := veconsume.Consumption(first); err != nil {
		t.Fatalf("first consumption: %v", err)
	}

	replay, err := handler.Handle(context.Background(), deriveCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.FactExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if err := veconsume.Consumption(replay); err != nil {
		t.Fatalf("replay must be accounted: %v", err)
	}

	conflicting := deriveCommand(t)
	conflicting.Fact.OccurredAt = factOccurredAt.Add(time.Minute)
	conflict, err := handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if conflict.Outcome() != application.FactSourceConflict {
		t.Fatalf("conflict = %q", conflict.Outcome())
	}
	if err := veconsume.Consumption(conflict); err != nil {
		t.Fatalf("conflict must be accounted: %v", err)
	}

	rejected, err := handler.Handle(context.Background(), application.DeriveProjectionCommand{})
	if err != nil {
		t.Fatalf("rejected handle: %v", err)
	}
	if rejected.Outcome() != application.FactNotAccepted {
		t.Fatalf("rejected = %q", rejected.Outcome())
	}
	if err := veconsume.Consumption(rejected); err != nil {
		t.Fatalf("not accepted must be accounted: %v", err)
	}
}
