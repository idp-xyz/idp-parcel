package nodeoperations_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"

	"go.idp.xyz/idp-bento-go/eventing"
)

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

type targetViewDouble struct {
	target psdomain.CurrentAcceptedParcelTarget
	found  bool
	err    error
	tenant string
	parcel string
}

func (double *targetViewDouble) FindCurrentAcceptedByParcel(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.tenant = tenant.String()
	double.parcel = parcel.String()
	if double.err != nil {
		return psdomain.CurrentAcceptedParcelTarget{}, false, double.err
	}
	return double.target, double.found, nil
}

type adopterDouble struct {
	calls int
}

func (double *adopterDouble) AdoptFromNodeIntake(
	context.Context, nodomain.NodeIntake, adapter.TargetShipment,
) (psapplication.AdoptNetworkIntakeResult, error) {
	double.calls++
	return psapplication.AdoptNetworkIntakeResult{}, errors.New("adopter should not be called")
}

type recordingHandler struct {
	inner   adapter.NetworkIntakeCommandHandler
	command psapplication.AdoptNetworkIntakeCommand
}

func (recorder *recordingHandler) Handle(
	ctx context.Context, command psapplication.AdoptNetworkIntakeCommand,
) (psapplication.AdoptNetworkIntakeResult, error) {
	recorder.command = command
	return recorder.inner.Handle(ctx, command)
}

func uniqueTarget(t *testing.T) psdomain.CurrentAcceptedParcelTarget {
	t.Helper()
	target, err := psdomain.NewCurrentAcceptedParcelTarget(
		identity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		value(t, psdomain.NewSubmissionVersionID, "version-1"),
	)
	if err != nil {
		t.Fatalf("当前已接受目标：%v", err)
	}
	return target
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
		RecordedAt: receivedAt,
	}
}

func formedRef() psinbox.FormedNodeIntake {
	return psinbox.FormedNodeIntake{TenantID: "tenant-1", SourceID: "source-1"}
}

func TestFormedIntakeLooksUpTheUniqueAcceptedTargetAndAdopts(t *testing.T) {
	receptions := &receptionDouble{record: formedReception(t), found: true}
	targets := &targetViewDouble{target: uniqueTarget(t), found: true}
	recorder := &recordingHandler{inner: adoptHandler(t)}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		receptions, targets, adapter.NewNodeIntakeAdapter(recorder))
	if err != nil {
		t.Fatalf("构造处理适配器：%v", err)
	}

	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); err != nil {
		t.Fatalf("处理形成收寄：%v", err)
	}

	if receptions.last.TenantID.String() != "tenant-1" || receptions.last.SourceID != "source-1" {
		t.Fatalf("收寄查询键 = %+v", receptions.last)
	}
	if targets.tenant != "tenant-1" || targets.parcel != "parcel-1" {
		t.Fatalf("反查租户/包裹 = %q / %q", targets.tenant, targets.parcel)
	}

	command := recorder.command
	if command.Identity != identity(t) ||
		command.ShipmentRequestID.String() != "request-1" ||
		command.SubmissionVersion.String() != "version-1" {
		t.Fatalf("目标指名 = %+v", command)
	}
	source := command.Source
	if source.Kind != psdomain.NodeIntakeSource ||
		source.Parcel.String() != "parcel-1" ||
		source.Object.String() != "unit-1" ||
		source.Place.String() != "node-origin" ||
		source.Control.String() != "NODE-INTAKE/SIGN-7" ||
		source.Version.String() != "intake-result/v1" ||
		!source.OccurredAt.Equal(receivedAt) {
		t.Fatalf("采用命令来源 = %+v", source)
	}
}

func TestAMissingReceptionIsContinuableUndecided(t *testing.T) {
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{}, &targetViewDouble{found: true, target: uniqueTarget(t)}, adopter)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionNotVisible) {
		t.Fatalf("err = %v, want ErrReceptionNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("缺收寄记录不该走到采用")
	}
}

func TestAnUnreadableReceptionIsContinuableUndecided(t *testing.T) {
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{err: errors.New("store unavailable")},
		&targetViewDouble{found: true, target: uniqueTarget(t)},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionNotVisible) {
		t.Fatalf("err = %v, want ErrReceptionNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("读失败不该走到采用")
	}
}

func TestAPendingIdentificationRecordIsNotAnAdoptableIntake(t *testing.T) {
	record := formedReception(t)
	record.Kind = noports.RecordPendingIdentification
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{record: record, found: true},
		&targetViewDouble{found: true, target: uniqueTarget(t)},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrReceptionNotVisible) {
		t.Fatalf("err = %v, want ErrReceptionNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("待识别格不该走到采用")
	}
}

func TestAnUnidentifiedFormedIntakeKeepsItsSentinel(t *testing.T) {
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
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{record: record, found: true},
		&targetViewDouble{found: true, target: uniqueTarget(t)},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrUnidentifiedHandlingUnit) {
		t.Fatalf("err = %v, want ErrUnidentifiedHandlingUnit", err)
	}
	if adopter.calls != 0 {
		t.Fatal("未识别实物不该走到采用")
	}
}

func TestAMissingParcelTargetIsContinuableUndecided(t *testing.T) {
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{record: formedReception(t), found: true},
		&targetViewDouble{},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrParcelTargetNotFound) {
		t.Fatalf("err = %v, want ErrParcelTargetNotFound", err)
	}
	if adopter.calls != 0 {
		t.Fatal("没有可采认目标不该走到采用")
	}
}

func TestAnAmbiguousParcelTargetStaysIdentifiable(t *testing.T) {
	adopter := &adopterDouble{}
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{record: formedReception(t), found: true},
		&targetViewDouble{err: psdomain.ErrAmbiguousParcelTarget},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
		t.Fatalf("err = %v, want ErrAmbiguousParcelTarget", err)
	}
	if adopter.calls != 0 {
		t.Fatal("歧义不得按 latest 采认")
	}
}

type controllableDownstream struct {
	err   error
	calls int
}

func (double *controllableDownstream) HandOffNetworkIntake(
	context.Context, psports.NetworkIntakeHandoffIntent,
) error {
	double.calls++
	return double.err
}

type eligibilityStub struct {
	eligibility psports.IntakeEligibility
	configured  bool
	err         error
}

func (stub eligibilityStub) JudgeIntakeEligibility(
	context.Context, psdomain.SourceIdentity, psdomain.ShipmentRequestID, psdomain.IntakeSource,
) (psports.IntakeEligibility, bool, error) {
	if stub.err != nil {
		return psports.IntakeEligibility{}, false, stub.err
	}
	return stub.eligibility, stub.configured, nil
}

func processingThrough(t *testing.T, handler *psapplication.AdoptNetworkIntakeHandler) *adapter.AdoptOnNodeIntakeAdapter {
	t.Helper()
	subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
		&receptionDouble{record: formedReception(t), found: true},
		&targetViewDouble{target: uniqueTarget(t), found: true},
		adapter.NewNodeIntakeAdapter(handler),
	)
	if err != nil {
		t.Fatalf("构造处理适配器：%v", err)
	}
	return subject
}

func TestAPendingHandoffRollsBackUntilDownstreamSucceeds(t *testing.T) {
	downstream := &controllableDownstream{err: errors.New("outbox unavailable")}
	handler := newAdoptHandler(t, adoptHandlerConfig{downstream: downstream})
	subject := processingThrough(t, handler)

	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); !errors.Is(err, adapter.ErrAdoptionHandoffPending) {
		t.Fatalf("err = %v, want ErrAdoptionHandoffPending", err)
	}
	if downstream.calls != 1 {
		t.Fatalf("handoff 调用 = %d, want 1", downstream.calls)
	}

	downstream.err = nil
	if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); err != nil {
		t.Fatalf("handoff 恢复后重投：%v", err)
	}
	if downstream.calls != 2 {
		t.Fatalf("已有结果路径应再交一次意图，调用 = %d", downstream.calls)
	}
}

func TestAdoptionOutcomesMapToConsumptionSlots(t *testing.T) {
	type setup func(*testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error)
	cases := []struct {
		outcome string
		want    error
		new     setup
	}{
		{
			outcome: "COMMITMENT_FORMED",
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				return processingThrough(t, adoptHandler(t)), nil
			},
		},
		{
			outcome: "EXISTING_RESULT",
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				subject := processingThrough(t, adoptHandler(t))
				if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); err != nil {
					return nil, err
				}
				return subject, nil
			},
		},
		{
			outcome: "SOURCE_NOT_ADOPTED",
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				store := &adoptionStoreDouble{byKey: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{}}
				prior := psports.IntakeAdoptionRecord{
					Key: psports.IntakeAdoptionKey{
						TenantID: identity(t).TenantID(),
						Parcel:   value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
						Kind:     psdomain.NodeIntakeSource,
						Version:  value(t, psdomain.NewSourceResultVersion, "intake-result/prior"),
					},
					Adopted: true,
				}
				store.byKey[prior.Key] = prior
				return processingThrough(t, newAdoptHandler(t, adoptHandlerConfig{adoptions: store})), nil
			},
		},
		{
			outcome: "SOURCE_CONFLICT",
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				store := &adoptionStoreDouble{byKey: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{}}
				key := psports.IntakeAdoptionKey{
					TenantID: identity(t).TenantID(),
					Parcel:   value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
					Kind:     psdomain.NodeIntakeSource,
					Version:  value(t, psdomain.NewSourceResultVersion, "intake-result/v1"),
				}
				store.byKey[key] = psports.IntakeAdoptionRecord{Key: key, ContentDigest: "other-digest"}
				return processingThrough(t, newAdoptHandler(t, adoptHandlerConfig{adoptions: store})), nil
			},
		},
		{
			outcome: "NOT_APPLICABLE",
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				return processingThrough(t, newAdoptHandler(t, adoptHandlerConfig{
					eligibility: eligibilityStub{
						configured: true,
						eligibility: psports.IntakeEligibility{
							Outcome: psports.IntakeServiceNotApplicable,
							Basis:   value(t, psdomain.NewCheckReason, "PRODUCT-WAYBILL-ONLY"),
						},
					},
				})), nil
			},
		},
		{
			outcome: "REQUEST_NOT_ACCEPTED",
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				wrong, err := psdomain.NewCurrentAcceptedParcelTarget(
					identity(t),
					value(t, psdomain.NewShipmentRequestID, "request-other"),
					value(t, psdomain.NewSubmissionVersionID, "version-1"),
				)
				if err != nil {
					return nil, err
				}
				subject, err := adapter.NewAdoptOnNodeIntakeAdapter(
					&receptionDouble{record: formedReception(t), found: true},
					&targetViewDouble{target: wrong, found: true},
					adapter.NewNodeIntakeAdapter(adoptHandler(t)),
				)
				return subject, err
			},
		},
		{
			outcome: "ELIGIBILITY_UNDECIDED",
			want:    adapter.ErrAdoptionUndecided,
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				return processingThrough(t, newAdoptHandler(t, adoptHandlerConfig{
					eligibility: eligibilityStub{err: errors.New("catalogue down")},
				})), nil
			},
		},
		{
			outcome: "COMMITMENT_FORMED handoff pending",
			want:    adapter.ErrAdoptionHandoffPending,
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				return processingThrough(t, newAdoptHandler(t, adoptHandlerConfig{
					downstream: &controllableDownstream{err: errors.New("outbox unavailable")},
				})), nil
			},
		},
		{
			outcome: "SOURCE_NOT_ADOPTED handoff pending",
			want:    adapter.ErrAdoptionHandoffPending,
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				store := &adoptionStoreDouble{byKey: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{}}
				prior := psports.IntakeAdoptionRecord{
					Key: psports.IntakeAdoptionKey{
						TenantID: identity(t).TenantID(),
						Parcel:   value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
						Kind:     psdomain.NodeIntakeSource,
						Version:  value(t, psdomain.NewSourceResultVersion, "intake-result/prior"),
					},
					Adopted: true,
				}
				store.byKey[prior.Key] = prior
				return processingThrough(t, newAdoptHandler(t, adoptHandlerConfig{
					adoptions:  store,
					downstream: &controllableDownstream{err: errors.New("outbox unavailable")},
				})), nil
			},
		},
		{
			outcome: "EXISTING_RESULT handoff pending",
			want:    adapter.ErrAdoptionHandoffPending,
			new: func(t *testing.T) (*adapter.AdoptOnNodeIntakeAdapter, error) {
				downstream := &controllableDownstream{}
				subject := processingThrough(t, newAdoptHandler(t, adoptHandlerConfig{downstream: downstream}))
				if err := subject.HandleFormedNodeIntake(t.Context(), formedRef()); err != nil {
					return nil, err
				}
				downstream.err = errors.New("outbox unavailable")
				return subject, nil
			},
		},
	}

	for _, item := range cases {
		t.Run(item.outcome, func(t *testing.T) {
			subject, err := item.new(t)
			if err != nil {
				t.Fatalf("准备：%v", err)
			}
			got := subject.HandleFormedNodeIntake(t.Context(), formedRef())
			if item.want == nil {
				if got != nil {
					t.Fatalf("err = %v, want nil——消费完成不等于形成采用", got)
				}
				return
			}
			if !errors.Is(got, item.want) {
				t.Fatalf("err = %v, want %v", got, item.want)
			}
		})
	}
}

func TestAPendingHandoffDoesNotMarkTheInboxProcessed(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	downstream := &controllableDownstream{err: errors.New("outbox unavailable")}
	handler := newAdoptHandler(t, adoptHandlerConfig{downstream: downstream})
	processing := processingThrough(t, handler)
	consumer, err := psinbox.NewNodeIntakeConsumer(db.Transactor(), store, processing)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}

	payload, err := json.Marshal(map[string]string{"tenantId": "tenant-1", "sourceId": "source-1"})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 18, 14, 0, 0, 0, time.UTC)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID("tenant-1/source-1"),
		Source:       "idp-parcel/node-operations",
		Type:         psinbox.NodeIntakeFormedEventType,
		Version:      1,
		Scope:        "tenant-1",
		Subject:      "source-1",
		PartitionKey: "tenant-1/source-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := consumer.Consume(t.Context(), envelope); !errors.Is(err, adapter.ErrAdoptionHandoffPending) {
		t.Fatalf("err = %v, want ErrAdoptionHandoffPending", err)
	}
	if err := consumer.Consume(t.Context(), envelope); !errors.Is(err, adapter.ErrAdoptionHandoffPending) {
		t.Fatalf("未入账的重投应再处理：%v", err)
	}
	if downstream.calls != 2 {
		t.Fatalf("inbox 若已 processed，第二次不会再交意图；调用 = %d", downstream.calls)
	}

	downstream.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("handoff 恢复后重投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("已处理后的重复投递：%v", err)
	}
	if downstream.calls != 3 {
		t.Fatalf("成功入账后重复投递不应再交意图；调用 = %d", downstream.calls)
	}
}
