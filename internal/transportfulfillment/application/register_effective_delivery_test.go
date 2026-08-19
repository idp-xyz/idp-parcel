package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	deliveryArrivedAt  = time.Date(2026, 8, 13, 9, 10, 0, 0, time.UTC)
	deliveryRecordedAt = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
)

type deliveryViewDouble struct {
	outcome domain.DeliveryObjectOutcome
	found   bool
	err     error
}

func (double *deliveryViewDouble) LoadDeliveryResult(
	_ context.Context,
	_ domain.TenantID,
	attempt domain.AttemptReference,
	object domain.CarriedObjectReference,
) (domain.FulfillmentAttempt, domain.DeliveryAttemptResult, bool, error) {
	if double.err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, double.err
	}
	if !double.found {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, nil
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, err
	}
	spec := domain.FulfillmentAttemptSpec{
		TenantID:    tenant,
		Attempt:     attempt,
		PlannedFrom: deliveryArrivedAt.Add(-time.Hour),
		PlannedTo:   deliveryArrivedAt.Add(3 * time.Hour),
		ArrivedAt:   deliveryArrivedAt,
		Objects:     []domain.CarriedObjectReference{object},
	}
	var buildErr error
	if spec.Task, buildErr = domain.NewDispatchTaskReference("delivery-task-1"); buildErr != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, buildErr
	}
	if spec.ExecutedBy, buildErr = domain.NewExecutingPartyReference("courier-1"); buildErr != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, buildErr
	}
	if spec.Place, buildErr = domain.NewAttemptPlaceReference("recipient-door-1"); buildErr != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, buildErr
	}
	if spec.Evidence, buildErr = domain.NewAttemptEvidenceReference("attempt-evidence-1"); buildErr != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, buildErr
	}
	attemptValue, err := domain.FormFulfillmentAttempt(spec)
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, err
	}
	basis := domain.AttemptResultBasisReference{}
	if double.outcome != domain.ObjectDelivered {
		if basis, err = domain.NewAttemptResultBasisReference("reason-1"); err != nil {
			return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, err
		}
	}
	result, err := domain.FormDeliveryAttemptResult(attemptValue, object, double.outcome, basis, deliveryArrivedAt.Add(10*time.Minute))
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, err
	}
	return attemptValue, result, true, nil
}

type deliveryStoreDouble struct {
	records       map[string]ports.EffectiveDeliveryRecord
	findErr       error
	saveErr       error
	saveResult    ports.DeliverySaveOutcome
	forceResult   bool
	missSupersede bool
	saves         int
}

func newDeliveryStore() *deliveryStoreDouble {
	return &deliveryStoreDouble{records: map[string]ports.EffectiveDeliveryRecord{}}
}

func deliveryKey(key ports.EffectiveDeliveryKey) string {
	return key.TenantID.String() + "|" + key.Object.String() + "|" + key.Attempt.String()
}

func (double *deliveryStoreDouble) FindByKey(
	_ context.Context,
	key ports.EffectiveDeliveryKey,
) (ports.EffectiveDeliveryRecord, bool, error) {
	if double.findErr != nil {
		return ports.EffectiveDeliveryRecord{}, false, double.findErr
	}
	record, found := double.records[deliveryKey(key)]
	return record, found, nil
}

func (double *deliveryStoreDouble) FindByKeyAndVersion(
	ctx context.Context,
	key ports.EffectiveDeliveryKey,
	_ domain.DeliveryResultVersion,
) (ports.EffectiveDeliveryRecord, bool, error) {
	// 登记编排只用 FindByKey 判幂等，不按版本读；这个方法只为满足
	// ports.EffectiveDeliveryStore。按版本回读的语义由生产适配器与
	// visibility-exception 的对应测试覆盖，此处不引入第二套。
	return double.FindByKey(ctx, key)
}

func (double *deliveryStoreDouble) Save(
	_ context.Context,
	record ports.EffectiveDeliveryRecord,
) (ports.DeliverySaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.DeliverySaveOutcomeInvalid, double.saveErr
	}
	if double.forceResult {
		return double.saveResult, nil
	}
	if _, exists := double.records[deliveryKey(record.Key)]; exists {
		return ports.DeliveryAlreadyRegistered, nil
	}
	double.records[deliveryKey(record.Key)] = record
	return ports.DeliverySaved, nil
}

func (double *deliveryStoreDouble) Supersede(
	_ context.Context,
	record ports.EffectiveDeliveryRecord,
) (bool, error) {
	if double.saveErr != nil {
		return false, double.saveErr
	}
	if double.missSupersede {
		// 模拟并发窗口：读到了登记，但顶替时它已不在。
		return false, nil
	}
	if _, exists := double.records[deliveryKey(record.Key)]; !exists {
		return false, nil
	}
	double.records[deliveryKey(record.Key)] = record
	return true, nil
}

type deliveryVersionFactory struct {
	minted int
	err    error
}

func (double *deliveryVersionFactory) NextDeliveryResultVersion(context.Context) (domain.DeliveryResultVersion, error) {
	if double.err != nil {
		return domain.DeliveryResultVersion{}, double.err
	}
	double.minted++
	return domain.NewDeliveryResultVersion(fmt.Sprintf("delivery-result/v%d", double.minted))
}

type deliveryHandoffDouble struct {
	intents []ports.EffectiveDeliveryHandoffIntent
	err     error
}

func (double *deliveryHandoffDouble) HandOffEffectiveDelivery(
	_ context.Context,
	intent ports.EffectiveDeliveryHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type deliveryClock struct{ at time.Time }

func (clock deliveryClock) Now() time.Time { return clock.at }

type deliveryFixture struct {
	view     *deliveryViewDouble
	store    *deliveryStoreDouble
	versions *deliveryVersionFactory
	handoff  *deliveryHandoffDouble
	handler  *application.RegisterEffectiveDeliveryHandler
}

func newDeliveryFixture(t *testing.T) *deliveryFixture {
	t.Helper()
	fixture := &deliveryFixture{
		view:     &deliveryViewDouble{outcome: domain.ObjectDelivered, found: true},
		store:    newDeliveryStore(),
		versions: &deliveryVersionFactory{},
		handoff:  &deliveryHandoffDouble{},
	}
	fixture.handler = application.NewRegisterEffectiveDeliveryHandler(application.RegisterEffectiveDeliveryDeps{
		Attempts:   fixture.view,
		Deliveries: fixture.store,
		Versions:   fixture.versions,
		Downstream: fixture.handoff,
		Clock:      deliveryClock{at: deliveryRecordedAt},
	})
	return fixture
}

func registerCommand(t *testing.T) application.RegisterEffectiveDeliveryCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.RegisterEffectiveDeliveryCommand{
		TenantID:  tenant,
		Attempt:   "attempt-1",
		Object:    "parcel-1",
		Method:    "method-signature",
		Recipient: "recipient-1",
		Proof:     "pod-1",
	}
}

// Covers: CONTEXT「有效交付结果」词条与 POD 要求——妥投结果+POD 首登生效交付（地点取
// 尝试、时间取结果由领域保证）；重放返原版不重签；异内容冲突走更正入口不顶替。
func TestARegistrationIsIdempotentPerObjectAttempt(t *testing.T) {
	fixture := newDeliveryFixture(t)
	command := registerCommand(t)

	first, err := fixture.handler.Register(context.Background(), command)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if first.Outcome() != application.DeliveryRegistered {
		t.Fatalf("outcome = %q, want DELIVERY_REGISTERED", first.Outcome())
	}
	record, _ := first.Record()
	if record.Delivery.Proof().String() != "pod-1" {
		t.Fatalf("proof = %q", record.Delivery.Proof())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}

	replay, err := fixture.handler.Register(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.DeliveryExistingVersion {
		t.Fatalf("outcome = %q, want EXISTING_VERSION", replay.Outcome())
	}
	if fixture.versions.minted != 1 {
		t.Fatalf("versions minted = %d, want 1（重放不重签）", fixture.versions.minted)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（重放重发同一份）", len(fixture.handoff.intents))
	}

	t.Run("a different POD under the same key is a conflict", func(t *testing.T) {
		flipped := registerCommand(t)
		flipped.Proof = "pod-2"
		result, err := fixture.handler.Register(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict register: %v", err)
		}
		if result.Outcome() != application.DeliveryRegistrationConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT（修 POD 走更正入口）", result.Outcome())
		}
	})
}

// Covers: 领域硬句「无人签收、地址错误、收件人拒收不构成有效交付」的编排面——失败
// 结果登生效交付落 NOT_EFFECTIVE 业务负向格，编排不把领域拒绝折成未受理；不落库不
// 签版不交意图。
func TestAFailedResultCannotRegisterAnEffectiveDelivery(t *testing.T) {
	fixture := newDeliveryFixture(t)
	fixture.view.outcome = domain.NoOneToReceive
	result, err := fixture.handler.Register(context.Background(), registerCommand(t))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.Outcome() != application.DeliveryNotEffective {
		t.Fatalf("outcome = %q, want NOT_EFFECTIVE（改约再派，不是改单）", result.Outcome())
	}
	if len(fixture.store.records) != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatal("未生效还落库或交意图")
	}
}

// Covers: `AT-TF-072` 的编排面——更正走 CorrectProof 新版本回指前版；没有可更正的登记
// 更正不出交付；新版本随意图重新交付下游。
func TestACorrectionSupersedesWithTheVersionChain(t *testing.T) {
	fixture := newDeliveryFixture(t)
	if _, err := fixture.handler.Register(context.Background(), registerCommand(t)); err != nil {
		t.Fatalf("register: %v", err)
	}

	tenant, _ := domain.NewTenantID("tenant-1")
	corrected, err := fixture.handler.Correct(context.Background(), application.CorrectDeliveryProofCommand{
		TenantID:    tenant,
		Attempt:     "attempt-1",
		Object:      "parcel-1",
		NewProof:    "pod-2",
		CorrectedAt: deliveryRecordedAt.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if corrected.Outcome() != application.DeliveryCorrected {
		t.Fatalf("outcome = %q, want DELIVERY_CORRECTED", corrected.Outcome())
	}
	record, _ := corrected.Record()
	predecessor, present := record.Delivery.Corrects()
	if !present || predecessor.String() != "delivery-result/v1" {
		t.Fatalf("corrects = %q present=%v, want v1（新版回指前版）", predecessor, present)
	}
	if record.Delivery.Proof().String() != "pod-2" {
		t.Fatalf("proof = %q", record.Delivery.Proof())
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（更正版本重新交付下游）", len(fixture.handoff.intents))
	}

	t.Run("correcting an absent registration is refused", func(t *testing.T) {
		missing, err := fixture.handler.Correct(context.Background(), application.CorrectDeliveryProofCommand{
			TenantID:    tenant,
			Attempt:     "attempt-9",
			Object:      "parcel-9",
			NewProof:    "pod-3",
			CorrectedAt: deliveryRecordedAt.Add(24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("correct absent: %v", err)
		}
		if missing.Outcome() != application.DeliveryNotAccepted {
			t.Fatalf("outcome = %q; 更正出了无中生有的交付", missing.Outcome())
		}
	})

	t.Run("a supersede miss does not claim a correction", func(t *testing.T) {
		fixture.store.missSupersede = true
		defer func() { fixture.store.missSupersede = false }()
		result, err := fixture.handler.Correct(context.Background(), application.CorrectDeliveryProofCommand{
			TenantID:    tenant,
			Attempt:     "attempt-1",
			Object:      "parcel-1",
			NewProof:    "pod-5",
			CorrectedAt: deliveryRecordedAt.Add(48 * time.Hour),
		})
		if err != nil {
			t.Fatalf("correct: %v", err)
		}
		if result.Outcome() != application.DeliveryNotAccepted {
			t.Fatalf("outcome = %q; 顶替没落地却宣称更正成功", result.Outcome())
		}
	})

	t.Run("a correction before the delivery time is refused by the domain", func(t *testing.T) {
		early, err := fixture.handler.Correct(context.Background(), application.CorrectDeliveryProofCommand{
			TenantID:    tenant,
			Attempt:     "attempt-1",
			Object:      "parcel-1",
			NewProof:    "pod-4",
			CorrectedAt: deliveryArrivedAt.Add(-time.Hour),
		})
		if err != nil {
			t.Fatalf("correct early: %v", err)
		}
		if early.Outcome() != application.DeliveryNotAccepted {
			t.Fatalf("outcome = %q（领域拒绝编排不绕）", early.Outcome())
		}
	})
}

// 未受理与恢复纪律：缺 POD/方式/接收方、结果查无 → 未受理；视图/库/版本厂故障各归
// 未决一格；投递失败不翻结果；写入代数外是编程错误。
// Covers: 恢复纪律——缺 POD 等必备件未受理（`AT-TF-069`「外部状态码为 DELIVERED、
// 无可验证 POD→不形成有效交付」的不形成半边）；意图投递失败登记不翻、重放重发同一份
// （`AT-TF-073`「交付结果保存成功但发布失败→不重复派送，只重试原发布意图」的编排面）。
func TestDeliveryRecoveryDiscipline(t *testing.T) {
	broken := map[string]func(*application.RegisterEffectiveDeliveryCommand){
		"no proof":     func(command *application.RegisterEffectiveDeliveryCommand) { command.Proof = " " },
		"no method":    func(command *application.RegisterEffectiveDeliveryCommand) { command.Method = " " },
		"no recipient": func(command *application.RegisterEffectiveDeliveryCommand) { command.Recipient = " " },
	}
	for name, breakCommand := range broken {
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryFixture(t)
			command := registerCommand(t)
			breakCommand(&command)
			result, err := fixture.handler.Register(context.Background(), command)
			if err != nil {
				t.Fatalf("register: %v", err)
			}
			if result.Outcome() != application.DeliveryNotAccepted {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED（POD/方式/接收方缺一不可）", result.Outcome())
			}
		})
	}

	t.Run("a missing delivery result is not accepted", func(t *testing.T) {
		fixture := newDeliveryFixture(t)
		fixture.view.found = false
		result, err := fixture.handler.Register(context.Background(), registerCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if result.Outcome() != application.DeliveryNotAccepted {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})

	t.Run("dependency failures are undecided with their reasons", func(t *testing.T) {
		view := newDeliveryFixture(t)
		view.view.err = errors.New("view down")
		result, err := view.handler.Register(context.Background(), registerCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if result.UndecidedReason() != application.DeliveryViewUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		store := newDeliveryFixture(t)
		store.store.findErr = errors.New("store down")
		result, err = store.handler.Register(context.Background(), registerCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if result.UndecidedReason() != application.DeliveryStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		factory := newDeliveryFixture(t)
		factory.versions.err = errors.New("factory down")
		result, err = factory.handler.Register(context.Background(), registerCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if result.UndecidedReason() != application.DeliveryIdentityUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newDeliveryFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.Register(context.Background(), registerCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if first.Outcome() != application.DeliveryRegistered || first.DeliveryHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q", first.Outcome(), first.DeliveryHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.Register(context.Background(), registerCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.DeliveryHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q", len(fixture.handoff.intents), replay.DeliveryHandoffReference())
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		fixture := newDeliveryFixture(t)
		fixture.store.forceResult = true
		fixture.store.saveResult = ports.DeliverySaveOutcome(99)
		if _, err := fixture.handler.Register(context.Background(), registerCommand(t)); !errors.Is(err, application.ErrUnexpectedDeliverySave) {
			t.Fatalf("error = %v, want ErrUnexpectedDeliverySave", err)
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.DeliveryUndecidedReason{
			application.DeliveryStoreUnavailable, application.DeliveryViewUnavailable, application.DeliveryIdentityUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.DeliveryUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第四个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
