package transportfulfillment_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
)

// 本文件证消费侧处理适配器：按信封的（租户+对象+尝试）重读 TF 对象级揽收登记，反查
// 当前已接受委托，再转交采用编排；以及四类停格各自可识别。

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

type pickupTargetViewDouble struct {
	target psdomain.CurrentAcceptedParcelTarget
	found  bool
	err    error
	tenant string
	parcel string
}

func (double *pickupTargetViewDouble) FindCurrentAcceptedByParcel(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.tenant = tenant.String()
	double.parcel = parcel.String()
	if double.err != nil {
		return psdomain.CurrentAcceptedParcelTarget{}, false, double.err
	}
	return double.target, double.found, nil
}

type pickupAdopterDouble struct {
	calls int
}

func (double *pickupAdopterDouble) AdoptFromOffsitePickup(
	context.Context, tfdomain.OffsitePickup, adapter.TargetShipment,
) (psapplication.AdoptNetworkIntakeResult, error) {
	double.calls++
	return psapplication.AdoptNetworkIntakeResult{}, errors.New("adopter should not be called")
}

type recordingCommandHandler struct {
	inner   *psapplication.AdoptNetworkIntakeHandler
	command psapplication.AdoptNetworkIntakeCommand
}

func (recorder *recordingCommandHandler) Handle(
	ctx context.Context, command psapplication.AdoptNetworkIntakeCommand,
) (psapplication.AdoptNetworkIntakeResult, error) {
	recorder.command = command
	return recorder.inner.Handle(ctx, command)
}

type controllableIntakeDownstream struct {
	err   error
	calls int
}

func (double *controllableIntakeDownstream) HandOffNetworkIntake(
	context.Context, psports.NetworkIntakeHandoffIntent,
) error {
	double.calls++
	return double.err
}

type unconfiguredEligibility struct{}

func (unconfiguredEligibility) JudgeIntakeEligibility(
	context.Context, psdomain.SourceIdentity, psdomain.ShipmentRequestID, psdomain.IntakeSource,
) (psports.IntakeEligibility, bool, error) {
	return psports.IntakeEligibility{}, false, errors.New("stage rule catalogue unavailable")
}

type pickupHandlerConfig struct {
	eligibility psports.IntakeEligibilityView
	downstream  psports.NetworkIntakeHandoff
}

func pickupAdoptHandler(t *testing.T, config pickupHandlerConfig) *psapplication.AdoptNetworkIntakeHandler {
	t.Helper()

	var eligibility psports.IntakeEligibilityView = eligibilityDouble{}
	if config.eligibility != nil {
		eligibility = config.eligibility
	}
	var downstream psports.NetworkIntakeHandoff = downstreamDouble{}
	if config.downstream != nil {
		downstream = config.downstream
	}
	return psapplication.NewAdoptNetworkIntakeHandler(psapplication.AdoptNetworkIntakeDeps{
		Requests: &requestStoreDouble{records: map[psdomain.SourceIdentity]psdomain.ShipmentRequest{
			identity(t): acceptedRequest(t),
		}},
		Eligibility: eligibility,
		Adoptions:   &adoptionStoreDouble{byKey: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{}},
		Identities:  &commitmentIdentityDouble{},
		Downstream:  downstream,
		Clock:       fixedClock{at: pickedUpAt.Add(time.Minute)},
	})
}

// registeredPickup 造一份对象级揽收登记，键与本体逐维一致——真库里两者由同一次提交写下。
func registeredPickup(t *testing.T, object string) tfports.OffsitePickupRecord {
	t.Helper()

	pickup, err := tfdomain.FormOffsitePickup(tfdomain.OffsitePickupSpec{
		TenantID:   value(t, tfdomain.NewTenantID, "tenant-1"),
		Object:     value(t, tfdomain.NewCarriedObjectReference, object),
		Task:       value(t, tfdomain.NewPickupTaskReference, "pickup-task-1"),
		Attempt:    value(t, tfdomain.NewAttemptReference, "attempt-1"),
		Place:      value(t, tfdomain.NewPickupPlaceReference, "customer-warehouse-1"),
		Control:    value(t, tfdomain.NewTransportControlReference, "TF-3"),
		ExecutedBy: value(t, tfdomain.NewExecutingPartyReference, "courier-1"),
		Version:    value(t, tfdomain.NewPickupResultVersion, "pickup-result/v1"),
		OccurredAt: pickedUpAt,
	})
	if err != nil {
		t.Fatalf("构造对象级揽收：%v", err)
	}
	return tfports.OffsitePickupRecord{
		Key: tfports.OffsitePickupKey{
			TenantID: value(t, tfdomain.NewTenantID, "tenant-1"),
			Object:   value(t, tfdomain.NewCarriedObjectReference, object),
			Attempt:  value(t, tfdomain.NewAttemptReference, "attempt-1"),
		},
		ContentDigest: "digest-1",
		Pickup:        pickup,
		RecordedAt:    pickedUpAt.Add(time.Second),
	}
}

func pickupTarget(t *testing.T) psdomain.CurrentAcceptedParcelTarget {
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

func registeredRef() psinbox.RegisteredOffsitePickup {
	return psinbox.RegisteredOffsitePickup{
		TenantID: "tenant-1",
		Object:   "parcel-1",
		Attempt:  "attempt-1",
	}
}

func TestARegisteredPickupLooksUpTheUniqueAcceptedTargetAndAdopts(t *testing.T) {
	registry := &pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true}
	targets := &pickupTargetViewDouble{target: pickupTarget(t), found: true}
	recorder := &recordingCommandHandler{inner: pickupAdoptHandler(t, pickupHandlerConfig{})}
	subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
		registry, targets, adapter.NewOffsitePickupAdapter(recorder))
	if err != nil {
		t.Fatalf("构造处理适配器：%v", err)
	}

	if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredRef()); err != nil {
		t.Fatalf("处理揽收登记：%v", err)
	}

	if registry.last.TenantID.String() != "tenant-1" ||
		registry.last.Object.String() != "parcel-1" ||
		registry.last.Attempt.String() != "attempt-1" {
		t.Fatalf("揽收查询键 = %+v", registry.last)
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
	if source.Kind != psdomain.OffsitePickupSource ||
		source.Parcel.String() != "parcel-1" ||
		source.Object.String() != "parcel-1" ||
		source.Place.String() != "customer-warehouse-1" ||
		source.Control.String() != "OFFSITE-PICKUP/TF-3" ||
		source.Version.String() != "pickup-result/v1" ||
		!source.OccurredAt.Equal(pickedUpAt) {
		t.Fatalf("采用命令来源 = %+v", source)
	}
}

func TestAMissingPickupRegistrationIsContinuableUndecided(t *testing.T) {
	adopter := &pickupAdopterDouble{}
	subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
		&pickupRegistryDouble{},
		&pickupTargetViewDouble{target: pickupTarget(t), found: true},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(
		t.Context(), registeredRef()); !errors.Is(err, adapter.ErrPickupNotVisible) {
		t.Fatalf("err = %v, want ErrPickupNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("缺登记不该走到采用")
	}
}

func TestAnUnreadablePickupRegistryIsContinuableUndecided(t *testing.T) {
	adopter := &pickupAdopterDouble{}
	subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
		&pickupRegistryDouble{err: errors.New("registry unavailable")},
		&pickupTargetViewDouble{target: pickupTarget(t), found: true},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(
		t.Context(), registeredRef()); !errors.Is(err, adapter.ErrPickupNotVisible) {
		t.Fatalf("err = %v, want ErrPickupNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("读失败不该走到采用")
	}
}

// Covers: 登记本体与幂等键各说各话时不采认，且与「还看不见」分开报。按键去查却拿回
// 另一个键或另一个对象是仓储/数据不变量破坏（ADR-0029），重投同一内容不会自愈——混进
// 可续办那一格会让永久损坏被当成等依赖，一直重试到投递上限。
func TestAPickupRecordThatDisagreesWithItsKeyIsInconsistentNotInvisible(t *testing.T) {
	for name, damage := range map[string]func(*testing.T, *tfports.OffsitePickupRecord){
		"键的对象与信封不符": func(t *testing.T, record *tfports.OffsitePickupRecord) {
			record.Key.Object = value(t, tfdomain.NewCarriedObjectReference, "parcel-other")
		},
		"本体与键不符": func(t *testing.T, record *tfports.OffsitePickupRecord) {
			record.Pickup = registeredPickup(t, "parcel-other").Pickup
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := registeredPickup(t, "parcel-1")
			damage(t, &record)
			adopter := &pickupAdopterDouble{}
			subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
				&pickupRegistryDouble{record: record, found: true},
				&pickupTargetViewDouble{target: pickupTarget(t), found: true},
				adopter,
			)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}

			first := subject.HandleRegisteredOffsitePickup(t.Context(), registeredRef())
			if !errors.Is(first, adapter.ErrPickupRecordInconsistent) {
				t.Fatalf("err = %v, want ErrPickupRecordInconsistent", first)
			}
			if errors.Is(first, adapter.ErrPickupNotVisible) {
				t.Fatal("不变量破坏不得混进可续办的「还看不见」")
			}
			if second := subject.HandleRegisteredOffsitePickup(
				t.Context(), registeredRef()); !errors.Is(second, adapter.ErrPickupRecordInconsistent) {
				t.Fatalf("重投 err = %v——同一内容重投不自愈", second)
			}
			if adopter.calls != 0 {
				t.Fatal("键与本体不符不该走到采用")
			}
		})
	}
}

func TestAMissingParcelTargetForAPickupIsContinuableUndecided(t *testing.T) {
	adopter := &pickupAdopterDouble{}
	subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true},
		&pickupTargetViewDouble{},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(
		t.Context(), registeredRef()); !errors.Is(err, adapter.ErrParcelTargetNotFound) {
		t.Fatalf("err = %v, want ErrParcelTargetNotFound", err)
	}
	if adopter.calls != 0 {
		t.Fatal("没有可采认目标不该走到采用")
	}
}

// Covers: TF 的载运对象引用可能指集运单元而非包裹，而 TF 不带判别位（见适配器注释）。
// 今天不猜成员映射：集运单元号反查不到当前已接受委托，落与「委托还没到已接受」同一格。
func TestAConsolidationUnitLikeObjectFallsIntoTargetNotFound(t *testing.T) {
	adopter := &pickupAdopterDouble{}
	targets := &pickupTargetViewDouble{}
	subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "bag-1"), found: true},
		targets,
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	reference := registeredRef()
	reference.Object = "bag-1"
	if err := subject.HandleRegisteredOffsitePickup(
		t.Context(), reference); !errors.Is(err, adapter.ErrParcelTargetNotFound) {
		t.Fatalf("err = %v, want ErrParcelTargetNotFound", err)
	}
	if targets.parcel != "bag-1" {
		t.Fatalf("反查包裹 = %q——不得替 TF 猜集运单元的成员", targets.parcel)
	}
	if adopter.calls != 0 {
		t.Fatal("反查不中不该走到采用")
	}
}

func TestAnAmbiguousParcelTargetForAPickupStaysIdentifiable(t *testing.T) {
	adopter := &pickupAdopterDouble{}
	subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true},
		&pickupTargetViewDouble{err: psdomain.ErrAmbiguousParcelTarget},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredOffsitePickup(
		t.Context(), registeredRef()); !errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
		t.Fatalf("err = %v, want ErrAmbiguousParcelTarget", err)
	}
	if adopter.calls != 0 {
		t.Fatal("歧义不得按 latest 采认")
	}
}

func TestAnUntranslatableReferenceKeepsItsSentinel(t *testing.T) {
	adopter := &pickupAdopterDouble{}
	subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true},
		&pickupTargetViewDouble{target: pickupTarget(t), found: true},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	for name, reference := range map[string]psinbox.RegisteredOffsitePickup{
		"空租户": {Object: "parcel-1", Attempt: "attempt-1"},
		"空对象": {TenantID: "tenant-1", Attempt: "attempt-1"},
		"空尝试": {TenantID: "tenant-1", Object: "parcel-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleRegisteredOffsitePickup(
				t.Context(), reference); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
			}
		})
	}
	if adopter.calls != 0 {
		t.Fatal("引用译不出来不该走到采用")
	}
}

func TestPickupAdoptionOutcomesMapToConsumptionSlots(t *testing.T) {
	pickupThrough := func(t *testing.T, handler *psapplication.AdoptNetworkIntakeHandler) *adapter.AdoptOnOffsitePickupAdapter {
		t.Helper()
		subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
			&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true},
			&pickupTargetViewDouble{target: pickupTarget(t), found: true},
			adapter.NewOffsitePickupAdapter(handler),
		)
		if err != nil {
			t.Fatalf("构造处理适配器：%v", err)
		}
		return subject
	}

	t.Run("COMMITMENT_FORMED 入账", func(t *testing.T) {
		subject := pickupThrough(t, pickupAdoptHandler(t, pickupHandlerConfig{}))
		if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredRef()); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("EXISTING_RESULT 入账", func(t *testing.T) {
		subject := pickupThrough(t, pickupAdoptHandler(t, pickupHandlerConfig{}))
		if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredRef()); err != nil {
			t.Fatalf("首次：%v", err)
		}
		if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredRef()); err != nil {
			t.Fatalf("重投 err = %v, want nil", err)
		}
	})

	t.Run("ELIGIBILITY_UNDECIDED 回滚", func(t *testing.T) {
		subject := pickupThrough(t, pickupAdoptHandler(t, pickupHandlerConfig{
			eligibility: unconfiguredEligibility{},
		}))
		if err := subject.HandleRegisteredOffsitePickup(
			t.Context(), registeredRef()); !errors.Is(err, adapter.ErrAdoptionUndecided) {
			t.Fatalf("err = %v, want ErrAdoptionUndecided", err)
		}
	})

	t.Run("handoff \u672a\u4ea4\u51fa\u5373\u56de\u6eda", func(t *testing.T) {
		downstream := &controllableIntakeDownstream{err: errors.New("outbox unavailable")}
		subject := pickupThrough(t, pickupAdoptHandler(t, pickupHandlerConfig{downstream: downstream}))
		if err := subject.HandleRegisteredOffsitePickup(
			t.Context(), registeredRef()); !errors.Is(err, adapter.ErrAdoptionHandoffPending) {
			t.Fatalf("err = %v, want ErrAdoptionHandoffPending", err)
		}
		if downstream.calls != 1 {
			t.Fatalf("handoff 调用 = %d, want 1", downstream.calls)
		}

		downstream.err = nil
		if err := subject.HandleRegisteredOffsitePickup(t.Context(), registeredRef()); err != nil {
			t.Fatalf("handoff 恢复后重投：%v", err)
		}
		if downstream.calls != 2 {
			t.Fatalf("已有结果路径应再交一次意图，调用 = %d", downstream.calls)
		}
	})
}

// Covers: 揽收这条链的未交出 handoff 同样不得入账。与节点收寄那条对偶：两条链共用
// adoptconsume 的同一份判断，这里对真实 Inbox 证它在本链上也生效。
func TestAPendingPickupHandoffDoesNotMarkTheInboxProcessed(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	downstream := &controllableIntakeDownstream{err: errors.New("outbox unavailable")}
	processing, err := adapter.NewAdoptOnOffsitePickupAdapter(
		&pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true},
		&pickupTargetViewDouble{target: pickupTarget(t), found: true},
		adapter.NewOffsitePickupAdapter(pickupAdoptHandler(t, pickupHandlerConfig{downstream: downstream})),
	)
	if err != nil {
		t.Fatalf("构造处理适配器：%v", err)
	}
	consumer, err := psinbox.NewOffsitePickupConsumer(db.Transactor(), store, processing)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-1",
		"object":   "parcel-1",
		"attempt":  "attempt-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := pickedUpAt.Add(time.Minute)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID("tenant-1/parcel-1/attempt-1/offsite-pickup-registration"),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         psinbox.OffsitePickupRegisteredEventType,
		Version:      1,
		Scope:        "tenant-1",
		Subject:      "parcel-1/attempt-1",
		PartitionKey: "tenant-1/parcel-1/attempt-1",
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
