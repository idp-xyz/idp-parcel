package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

var deliveredAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type identityViewDouble struct {
	candidates []domain.ParcelAssociationReference
	err        error
}

func (double *identityViewDouble) ResolveParcelIdentity(
	_ context.Context,
	_ domain.TenantID,
	_ ports.ExternalMarkObservation,
) ([]domain.ParcelAssociationReference, error) {
	return double.candidates, double.err
}

type receptionStoreDouble struct {
	byKey map[ports.ReceptionKey]ports.ReceptionRecord
	saved int
}

func newReceptionStore() *receptionStoreDouble {
	return &receptionStoreDouble{byKey: map[ports.ReceptionKey]ports.ReceptionRecord{}}
}

func (double *receptionStoreDouble) FindByKey(
	_ context.Context,
	key ports.ReceptionKey,
) (ports.ReceptionRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *receptionStoreDouble) Save(
	_ context.Context,
	record ports.ReceptionRecord,
) (ports.ReceptionSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return ports.ReceptionAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	double.saved++
	return ports.ReceptionSaved, nil
}

type versionFactoryDouble struct{ next int }

func (double *versionFactoryDouble) NextIntakeResultVersion(_ context.Context) (domain.IntakeResultVersion, error) {
	double.next++
	return domain.NewIntakeResultVersion("intake-result/v" + string(rune('0'+double.next)))
}

type downstreamDouble struct {
	intents []ports.NodeIntakeHandoffIntent
	err     error
}

func (double *downstreamDouble) HandOffNodeIntake(
	_ context.Context,
	intent ports.NodeIntakeHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type receptionFixture struct {
	handler    *application.ReceiveDeliveredUnitHandler
	identity   *identityViewDouble
	receptions *receptionStoreDouble
	downstream *downstreamDouble
}

func newReceptionFixture(t *testing.T) *receptionFixture {
	t.Helper()
	fixture := &receptionFixture{
		identity: &identityViewDouble{candidates: []domain.ParcelAssociationReference{
			mustValue(t, domain.NewParcelAssociationReference, "parcel-1"),
		}},
		receptions: newReceptionStore(),
		downstream: &downstreamDouble{},
	}
	fixture.handler = application.NewReceiveDeliveredUnitHandler(application.ReceiveDeliveredUnitDeps{
		Identity:   fixture.identity,
		Receptions: fixture.receptions,
		Versions:   &versionFactoryDouble{},
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: deliveredAt.Add(time.Minute)},
	})
	return fixture
}

func receiveCommand(t *testing.T) application.ReceiveDeliveredUnitCommand {
	t.Helper()
	return application.ReceiveDeliveredUnitCommand{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		SourceID:    "delivery-1",
		Node:        mustValue(t, domain.NewNodeReference, "node-origin"),
		DeliveredBy: mustValue(t, domain.NewDeliveringPartyReference, "customer-1"),
		Unit:        mustValue(t, domain.NewHandlingUnitID, "unit-1"),
		Mark:        ports.ExternalMarkObservation{Mark: "BARCODE-1"},
		Claim:       application.ExplicitReception,
		Evidence:    mustValue(t, domain.NewReceptionEvidenceReference, "SIGN-7"),
		OccurredAt:  deliveredAt,
	}
}

// Covers: `AT-NO-015`「已接受网络服务包裹由客户直接送达适用节点——形成节点收寄、节点
// 控制和来源证据；不直接形成正式承诺」——唯一身份关联收寄，控制以收寄为成立依据；
// 交给 PS 的只是发布意图，本编排不造承诺。`AT-NO-020/021/022` 的标记面：服务结果引用
// 原样保全，不阻止接收。
func TestAnExplicitReceptionFormsIntakeAndControl(t *testing.T) {
	fixture := newReceptionFixture(t)
	command := receiveCommand(t)
	command.ServiceMarkers = []string{"CANCELLED/decision-9"}

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("outcome = %q, want INTAKE_FORMED", result.Outcome())
	}
	record, _ := result.Record()
	association, identified := record.Intake.Association()
	if !identified || association.String() != "parcel-1" {
		t.Fatalf("association = %v identified = %v", association, identified)
	}
	if !record.Control.Active() || record.Control.EstablishmentKind() != domain.EstablishedByNodeIntake {
		t.Fatalf("control = %#v; 控制必须以节点收寄为成立依据", record.Control)
	}
	if !record.Intake.ReceivedAt().Equal(deliveredAt) {
		t.Fatalf("received at = %s", record.Intake.ReceivedAt())
	}
	if len(record.ServiceMarkers) != 1 || record.ServiceMarkers[0] != "CANCELLED/decision-9" {
		t.Fatal("服务结果标记没有原样保全")
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.downstream.intents))
	}
}

// Covers: `AT-NO-018`「实物无法可靠识别——建立待识别实物，保存控制、位置、状况和候选，
// 不创建正式包裹」与 `AT-NO-019`「同一外部条码出现在两件实物——保持独立，形成身份冲突
// 并暂停方向性作业」——两格都真实成立收寄与控制，都不交 PS 意图（没有可采用的关联）。
func TestUnknownAndConflictedIdentitiesStayPending(t *testing.T) {
	t.Run("unknown identity", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		fixture.identity.candidates = nil

		result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.UnitPendingIdentification {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		record, _ := result.Record()
		if _, identified := record.Intake.Association(); identified {
			t.Fatal("未知身份凭空长出了关联")
		}
		if !record.Control.Active() {
			t.Fatal("待识别实物的控制没有成立")
		}
		if record.IdentityConflict {
			t.Fatal("未知不是冲突")
		}
		if len(fixture.downstream.intents) != 0 {
			t.Fatal("没有关联的收寄交了意图")
		}
	})

	t.Run("conflicting candidates", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		fixture.identity.candidates = []domain.ParcelAssociationReference{
			mustValue(t, domain.NewParcelAssociationReference, "parcel-1"),
			mustValue(t, domain.NewParcelAssociationReference, "parcel-2"),
		}

		result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.UnitPendingIdentification {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		record, _ := result.Record()
		if !record.IdentityConflict || len(record.Candidates) != 2 {
			t.Fatalf("conflict = %v candidates = %d; 冲突必须带全部候选", record.IdentityConflict, len(record.Candidates))
		}
	})
}

// Covers: `AT-NO-023`「只有到站扫描——判断待确认，不建立控制」与 `AT-NO-024`「节点明确
// 拒收且未取得实物控制——形成未收寄结果和原因，不形成委托拒绝或取消」——两格都不建
// 控制、不交意图；拒收原因随记录保全。
func TestScanOnlyAndRefusalDoNotEstablishControl(t *testing.T) {
	t.Run("scan only", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		command := receiveCommand(t)
		command.Claim = application.ScanOnlyObservation
		command.Evidence = domain.ReceptionEvidenceReference{}

		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ReceptionUndecided {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		record, _ := result.Record()
		if record.Control.Active() {
			t.Fatal("扫描建立了控制")
		}
		if fixture.receptions.saved != 1 {
			t.Fatal("扫描来源没有随记录保全")
		}
	})

	t.Run("explicit refusal", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		command := receiveCommand(t)
		command.Claim = application.ExplicitRefusal
		command.Refusal = "PACKAGING_UNSAFE"
		command.Evidence = domain.ReceptionEvidenceReference{}

		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.NodeIntakeNotFormed {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		record, _ := result.Record()
		if record.RefusalReason != "PACKAGING_UNSAFE" {
			t.Fatal("拒收原因没有随记录保全")
		}
		if record.Control.Active() {
			t.Fatal("拒收建立了控制")
		}
		if len(fixture.downstream.intents) != 0 {
			t.Fatal("未收寄交了意图")
		}
	})
}

// Covers: `AT-NO-016`「同一交付来源和内容重复到达——返回原结果，不重复建立作业实物、
// 控制起点或事件」、`AT-NO-017`「同一来源身份携带不同包裹范围——形成来源冲突，保留原
// 结果」与 `AT-NO-027`「收寄结果提交成功但事件投递失败——保留业务结果，只重试同一发布
// 意图」。
func TestReplayConflictAndFailedIntentStayDisciplined(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.downstream.err = errors.New("downstream unreachable")

	first, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("outcome = %q; 投递失败不得翻收寄结果", first.Outcome())
	}
	if first.IntakeHandoffReference() == "" {
		t.Fatal("首投失败没有留发布续办引用")
	}

	fixture.downstream.err = nil
	replay, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ReceptionExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if fixture.receptions.saved != 1 {
		t.Fatal("重放重复建立了记录")
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("intents = %d; 重发的必须是同一份", len(fixture.downstream.intents))
	}

	conflicting := receiveCommand(t)
	conflicting.Unit = mustValue(t, domain.NewHandlingUnitID, "unit-9")
	conflict, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if conflict.Outcome() != application.ReceptionSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", conflict.Outcome())
	}
	if fixture.receptions.saved != 1 {
		t.Fatal("冲突覆盖了原结果")
	}
}

// Covers: `AT-NO-025`「批量交付部分收寄、部分待识别、部分未收寄——逐件结果并存，成功
// 成员不因其他成员失败回滚」——批量由调用方逐件分发，三件三种走向各自成立，先成的
// 记录纹丝不动。
func TestABatchKeepsPerUnitResultsIndependent(t *testing.T) {
	fixture := newReceptionFixture(t)

	formed, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("formed handle: %v", err)
	}
	if formed.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("formed = %q", formed.Outcome())
	}

	fixture.identity.candidates = nil
	pending := receiveCommand(t)
	pending.SourceID = "delivery-2"
	pending.Unit = mustValue(t, domain.NewHandlingUnitID, "unit-2")
	pendingResult, err := fixture.handler.Handle(context.Background(), pending)
	if err != nil {
		t.Fatalf("pending handle: %v", err)
	}
	if pendingResult.Outcome() != application.UnitPendingIdentification {
		t.Fatalf("pending = %q", pendingResult.Outcome())
	}

	refused := receiveCommand(t)
	refused.SourceID = "delivery-3"
	refused.Unit = mustValue(t, domain.NewHandlingUnitID, "unit-3")
	refused.Claim = application.ExplicitRefusal
	refused.Refusal = "LEAKING"
	refused.Evidence = domain.ReceptionEvidenceReference{}
	refusedResult, err := fixture.handler.Handle(context.Background(), refused)
	if err != nil {
		t.Fatalf("refused handle: %v", err)
	}
	if refusedResult.Outcome() != application.NodeIntakeNotFormed {
		t.Fatalf("refused = %q", refusedResult.Outcome())
	}

	if fixture.receptions.saved != 3 {
		t.Fatalf("saved = %d; 三件该有三份互不相扰的记录", fixture.receptions.saved)
	}
	survivor, found, err := fixture.receptions.FindByKey(context.Background(),
		ports.ReceptionKey{TenantID: mustValue(t, domain.NewTenantID, "tenant-1"), SourceID: "delivery-1"})
	if err != nil || !found || survivor.Kind != ports.RecordIntakeFormed {
		t.Fatalf("survivor = %#v; 先成的收寄被回滚了", survivor)
	}
}
