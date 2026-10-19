package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证节点控制按权威运输交接转出的编排（ADR-0214 决定二至六，UC-NO-005 转出）：读回那一版、认更正链、认交接对象、取实物锁读
// 在身控制，再按领域判转出或入账；以及各格对信封的处置。替身只在内存里扮转移册、交接来源与对象认法；真库的锁、折叠与唯一索引由
// adapters/postgres 的真库用例证。

var (
	transferJudgedAt   = deliveredAt.Add(5 * time.Hour)
	transferRecordedAt = deliveredAt.Add(6 * time.Hour)
)

type handoverSourceDouble struct {
	answers map[string]ports.AuthoritativeHandover
	err     error
}

func (double *handoverSourceDouble) FindAuthoritativeHandover(
	_ context.Context, _ domain.TenantID, reference domain.AuthoritativeHandoverReference,
) (ports.AuthoritativeHandover, bool, error) {
	if double.err != nil {
		return ports.AuthoritativeHandover{}, false, double.err
	}
	answer, found := double.answers[reference.Version()]
	return answer, found, nil
}

type objectResolverDouble struct {
	resolution ports.HandoverObjectResolution
	asked      int
}

func (double *objectResolverDouble) ResolveHandoverObject(
	context.Context, domain.TenantID, domain.AuthoritativeHandoverReference,
) (ports.HandoverObjectResolution, error) {
	double.asked++
	return double.resolution, nil
}

// controlRegisterDouble 在内存里扮转移册：按（对象，范围，版本，方向）存行，按（对象，范围，方向）认更正链。advanceUnderLock 扮
// 「同链前版在锁外刚提交」：取实物锁那一刻才出现在册上。lostRace 让 Save 答已在册而不落行，winner 是另一方先落的那一行。
type controlRegisterDouble struct {
	rows             map[string]ports.ControlTransitionRecord
	active           []domain.PhysicalControl
	locked           []string
	saves            int
	advanceUnderLock *ports.ControlTransitionRecord
	lostRace         bool
	winner           ports.ControlTransitionRecord
}

func newControlRegister() *controlRegisterDouble {
	return &controlRegisterDouble{rows: map[string]ports.ControlTransitionRecord{}}
}

func transitionKey(handover domain.AuthoritativeHandoverReference, direction domain.ControlTransitionDirection) string {
	return handover.Object() + "|" + handover.Scope() + "|" + handover.Version() + "|" + direction.String()
}

func (double *controlRegisterDouble) put(record ports.ControlTransitionRecord) {
	double.rows[transitionKey(record.Transition.Handover(), record.Transition.Direction())] = record
}

func (double *controlRegisterDouble) FindTransition(
	_ context.Context, _ domain.TenantID, handover domain.AuthoritativeHandoverReference, direction domain.ControlTransitionDirection,
) (ports.ControlTransitionRecord, bool, error) {
	record, found := double.rows[transitionKey(handover, direction)]
	return record, found, nil
}

func (double *controlRegisterDouble) FindAdvancedInHandoverChain(
	_ context.Context, _ domain.TenantID, handover domain.AuthoritativeHandoverReference, direction domain.ControlTransitionDirection,
) (ports.ControlTransitionRecord, bool, error) {
	for _, record := range double.rows {
		transition := record.Transition
		if transition.Handover().SameChain(handover) && transition.Direction() == direction &&
			transition.Outcome() == domain.ControlAdvanced {
			return record, true, nil
		}
	}
	return ports.ControlTransitionRecord{}, false, nil
}

func (double *controlRegisterDouble) FindActiveControls(
	_ context.Context, _ domain.TenantID, unit domain.HandlingUnitID,
) ([]domain.PhysicalControl, error) {
	double.locked = append(double.locked, unit.String())
	if double.advanceUnderLock != nil {
		double.put(*double.advanceUnderLock)
	}
	return double.active, nil
}

func (double *controlRegisterDouble) Save(_ context.Context, record ports.ControlTransitionRecord) (ports.ControlTransitionSaveOutcome, error) {
	double.saves++
	if double.lostRace {
		double.put(double.winner)
		return ports.ControlTransitionAlreadyRecorded, nil
	}
	if _, taken := double.rows[transitionKey(record.Transition.Handover(), record.Transition.Direction())]; taken {
		return ports.ControlTransitionAlreadyRecorded, nil
	}
	double.put(record)
	return ports.ControlTransitionSaved, nil
}

type transferFixture struct {
	handovers *handoverSourceDouble
	objects   *objectResolverDouble
	controls  *controlRegisterDouble
	handler   *application.TransferControlOnHandoverHandler
}

func newTransferFixture(t *testing.T) *transferFixture {
	t.Helper()
	fixture := &transferFixture{
		handovers: &handoverSourceDouble{answers: map[string]ports.AuthoritativeHandover{}},
		objects: &objectResolverDouble{resolution: ports.HandoverObjectResolution{
			Kind: ports.HandoverObjectParcel, Units: []domain.HandlingUnitID{mustValue(t, domain.NewHandlingUnitID, "unit-1")},
		}},
		controls: newControlRegister(),
	}
	fixture.controls.active = []domain.PhysicalControl{heldIntakeControl(t)}
	handler, err := application.NewTransferControlOnHandoverHandler(application.TransferControlOnHandoverDeps{
		Handovers: fixture.handovers, Objects: fixture.objects, Controls: fixture.controls,
		Clock: fixedClock{at: transferRecordedAt},
	})
	if err != nil {
		t.Fatalf("构造转出编排：%v", err)
	}
	fixture.handler = handler
	return fixture
}

func heldIntakeControl(t *testing.T) domain.PhysicalControl {
	t.Helper()
	control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
		TenantID:      mustValue(t, domain.NewTenantID, "tenant-1"),
		Unit:          mustValue(t, domain.NewHandlingUnitID, "unit-1"),
		Node:          mustValue(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB"),
		Kind:          domain.EstablishedByNodeIntake,
		Basis:         mustValue(t, domain.NewControlBasisReference, "NODE-INTAKE/intake-1"),
		EstablishedAt: deliveredAt,
	})
	if err != nil {
		t.Fatalf("成立控制：%v", err)
	}
	return control
}

func handoverVersion(t *testing.T, version string) domain.AuthoritativeHandoverReference {
	t.Helper()
	reference, err := domain.NewAuthoritativeHandoverReference("parcel-1", "SYN-SCOPE-61", version)
	if err != nil {
		t.Fatalf("交接键：%v", err)
	}
	return reference
}

// handedOver 登一版`已交接`；refused 登一版拒收或待确认（本上下文只看得见「不是`已交接`」）。
func (fixture *transferFixture) handedOver(t *testing.T, version string) {
	t.Helper()
	fixture.handovers.answers[version] = ports.AuthoritativeHandover{
		Reference: handoverVersion(t, version), HandedOver: true, JudgedAt: transferJudgedAt,
		TransferOut: mustValue(t, domain.NewTransferOutReference, "TRANSPORT-HANDOVER/"+version),
	}
}

func (fixture *transferFixture) refused(t *testing.T, version string) {
	t.Helper()
	fixture.handovers.answers[version] = ports.AuthoritativeHandover{Reference: handoverVersion(t, version), JudgedAt: transferJudgedAt}
}

func (fixture *transferFixture) transfer(t *testing.T, version string) application.TransferControlOnHandoverResult {
	t.Helper()
	result, err := fixture.handler.Transfer(context.Background(), application.TransferControlOnHandoverCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Handover: handoverVersion(t, version),
	})
	if err != nil {
		t.Fatalf("转出 %s：%v", version, err)
	}
	return result
}

// Covers: `AT-NO-101`（ADR-0214 决定二、四）——`已交接`、交接对象按正式包裹身份对上恰一件作业实物、它恰有一份在身控制：取实物锁，
// 转出那一份，转出依据是交接给的引用，时刻取交接的业务时间，记录时刻取时钟；入账不重投。
func TestAHandedOverHandoverTransfersOutTheUnitsControl(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.handedOver(t, "v1")

	result := fixture.transfer(t, "v1")

	if result.Outcome() != application.ControlTransferredOutOnHandover || result.Disposition() != application.ControlOnHandoverSettled {
		t.Fatalf("outcome = %s disposition = %s", result.Outcome(), result.Disposition())
	}
	record, present := result.Transition()
	control, carries := record.Transition.Control()
	if !present || !carries || control.Active() || !record.RecordedAt.Equal(transferRecordedAt) {
		t.Fatalf("落下的那一行 = %+v", record)
	}
	reference, at, _ := control.Release()
	if reference.String() != "TRANSPORT-HANDOVER/v1" || !at.Equal(transferJudgedAt) || control.Basis().String() != "NODE-INTAKE/intake-1" {
		t.Fatalf("转出 = %q @ %s，转出的控制依据 %q", reference, at, control.Basis())
	}
	if len(fixture.controls.locked) != 1 || fixture.controls.locked[0] != "unit-1" {
		t.Fatalf("实物锁 = %v, want 取了 unit-1 那一把", fixture.controls.locked)
	}
}

// Covers: `AT-NO-102`（ADR-0214 决定六）——同一版重投：这一版在转出方向上已有一行，交回那一行，不再认对象、不取锁、不落第二行。
func TestARedeliveredHandoverAnswersTheRecordedTransition(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.handedOver(t, "v1")
	first := fixture.transfer(t, "v1")

	again := fixture.transfer(t, "v1")

	if again.Outcome() != application.ControlTransitionAlreadyRecorded || again.Disposition() != application.ControlOnHandoverSettled {
		t.Fatalf("重投 = %s", again.Outcome())
	}
	recorded, _ := again.Transition()
	original, _ := first.Transition()
	if recorded.Transition != original.Transition || fixture.controls.saves != 1 || fixture.objects.asked != 1 {
		t.Fatalf("重投交回 %+v、落册 %d 次、认对象 %d 次, want 原那一行、1、1", recorded, fixture.controls.saves, fixture.objects.asked)
	}
}

// Covers: `AT-NO-103`（ADR-0214 决定四）——拒收与待确认：同链上没转出过，零写入，不认对象、不取锁。
func TestARefusedOrPendingHandoverWritesNothing(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.refused(t, "v1")

	result := fixture.transfer(t, "v1")

	if result.Outcome() != application.HandoverDoesNotTransferControl || result.Disposition() != application.ControlOnHandoverSettled {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if fixture.controls.saves != 0 || fixture.objects.asked != 0 || len(fixture.controls.locked) != 0 {
		t.Fatalf("落册 %d、认对象 %d、取锁 %v, want 都没有", fixture.controls.saves, fixture.objects.asked, fixture.controls.locked)
	}
}

// Covers: `AT-NO-104`、`AT-NO-105`、`AT-NO-106`、`AT-NO-107`（ADR-0214 决定二、四）——对不上一件作业实物（零件、多件、对象是集运单元）
// 与对上了却没有在身控制：各入账一格不推进，对不上的那几格不取实物锁。
func TestHandoversThatCannotTransferAreRecordedNotAdvanced(t *testing.T) {
	two := []domain.HandlingUnitID{}
	for _, raw := range []string{"unit-1", "unit-2"} {
		two = append(two, mustValue(t, domain.NewHandlingUnitID, raw))
	}
	cases := map[string]struct {
		resolution ports.HandoverObjectResolution
		noControl  bool
		reason     domain.ControlNotAdvancedReason
		locks      int
	}{
		"零件":     {resolution: ports.HandoverObjectResolution{Kind: ports.HandoverObjectParcel}, reason: domain.NoAssociatedUnit},
		"多件":     {resolution: ports.HandoverObjectResolution{Kind: ports.HandoverObjectParcel, Units: two}, reason: domain.SeveralAssociatedUnits},
		"集运单元":   {resolution: ports.HandoverObjectResolution{Kind: ports.HandoverObjectConsolidationUnit}, reason: domain.ConsolidationUnitPendingRule},
		"没有在身控制": {noControl: true, reason: domain.NoActiveControl, locks: 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newTransferFixture(t)
			if tc.resolution.Kind != ports.HandoverObjectKindInvalid {
				fixture.objects.resolution = tc.resolution
			}
			if tc.noControl {
				fixture.controls.active = nil
			}
			fixture.handedOver(t, "v1")

			result := fixture.transfer(t, "v1")

			record, _ := result.Transition()
			if result.Outcome() != application.ControlNotAdvancedOnHandover || record.Transition.Reason() != tc.reason ||
				result.Disposition() != application.ControlOnHandoverSettled {
				t.Fatalf("outcome = %s reason = %s", result.Outcome(), record.Transition.Reason())
			}
			if len(fixture.controls.locked) != tc.locks {
				t.Fatalf("取锁 %v, want %d 次", fixture.controls.locked, tc.locks)
			}
		})
	}
}

// Covers: `AT-NO-108`（ADR-0214 决定五）——后继版仍`已交接`：控制已随前版转出，零写入；前版在锁外刚转出、锁下才看得见时同答。被后继版
// 接续了的那一版晚到：零写入，不认对象。
func TestCorrectionsThatKeepTheTransferWriteNothing(t *testing.T) {
	t.Run("后继版仍已交接", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.handedOver(t, "v1")
		fixture.handedOver(t, "v2")
		fixture.transfer(t, "v1")

		result := fixture.transfer(t, "v2")

		if result.Outcome() != application.ControlAlreadyTransferredInChain || fixture.controls.saves != 1 {
			t.Fatalf("outcome = %s，落册 %d 次", result.Outcome(), fixture.controls.saves)
		}
	})
	t.Run("前版在锁外刚转出", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.handedOver(t, "v1")
		fixture.handedOver(t, "v2")
		earlier := fixture.transfer(t, "v1")
		recorded, _ := earlier.Transition()
		fixture.controls.rows = map[string]ports.ControlTransitionRecord{}
		fixture.controls.advanceUnderLock = &recorded
		saves := fixture.controls.saves

		result := fixture.transfer(t, "v2")

		if result.Outcome() != application.ControlAlreadyTransferredInChain || fixture.controls.saves != saves {
			t.Fatalf("outcome = %s，又落册 %d 次", result.Outcome(), fixture.controls.saves-saves)
		}
	})
	t.Run("被接续的前版晚到", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.handedOver(t, "v1")
		answer := fixture.handovers.answers["v1"]
		answer.Superseded = true
		fixture.handovers.answers["v1"] = answer

		result := fixture.transfer(t, "v1")

		if result.Outcome() != application.HandoverVersionSuperseded || fixture.controls.saves != 0 || fixture.objects.asked != 0 {
			t.Fatalf("outcome = %s，落册 %d、认对象 %d", result.Outcome(), fixture.controls.saves, fixture.objects.asked)
		}
	})
}

// Covers: `AT-NO-109`（ADR-0214 决定五）——撤回控制的更正：同链前版转出过、这一版不是`已交接`，入账「控制来源链待重算」，带先前那次
// 转出；控制不回退，不取锁。
func TestAWithdrawingCorrectionIsRecordedForRecompute(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.handedOver(t, "v1")
	fixture.refused(t, "v2")
	fixture.transfer(t, "v1")

	result := fixture.transfer(t, "v2")

	record, _ := result.Transition()
	control, carries := record.Transition.Control()
	if result.Outcome() != application.ControlNotAdvancedOnHandover ||
		record.Transition.Reason() != domain.ControlSourceChainPendingRecompute || !carries || control.Active() {
		t.Fatalf("outcome = %s reason = %s carries = %v", result.Outcome(), record.Transition.Reason(), carries)
	}
	if reference, _, _ := control.Release(); reference.String() != "TRANSPORT-HANDOVER/v1" || len(fixture.controls.locked) != 1 {
		t.Fatalf("待重算那一行带的转出 = %q，取锁 %v（只有 v1 那一次）", reference, fixture.controls.locked)
	}
}

// Covers: ADR-0214 决定六——交接登记册读不动或还读不回这一版：答未决、重投，不写；并发里同一版另一方先落了行：读回赢家作答。
func TestUndecidedAndLostRacesAnswerWithoutAWrite(t *testing.T) {
	t.Run("读不动", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.handovers.err = fmt.Errorf("%w: connection reset", ports.ErrAuthoritativeHandoverUnreadable)
		result := fixture.transfer(t, "v1")
		if result.Outcome() != application.ControlOnHandoverUndecided ||
			result.UndecidedReason() != application.ControlOnHandoverHandoverUnreadable ||
			result.Disposition() != application.ControlOnHandoverRetry {
			t.Fatalf("outcome = %s reason = %s", result.Outcome(), result.UndecidedReason())
		}
	})
	t.Run("读不回", func(t *testing.T) {
		fixture := newTransferFixture(t)
		result := fixture.transfer(t, "v1")
		if result.UndecidedReason() != application.ControlOnHandoverHandoverNotVisible || fixture.controls.saves != 0 {
			t.Fatalf("reason = %s", result.UndecidedReason())
		}
	})
	t.Run("译不过来不是未决", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.handovers.err = errors.New("the provider record is broken")
		if _, err := fixture.handler.Transfer(context.Background(), application.TransferControlOnHandoverCommand{
			TenantID: mustValue(t, domain.NewTenantID, "tenant-1"), Handover: handoverVersion(t, "v1"),
		}); err == nil {
			t.Fatal("坏记录该原样交回错误，而不是答未决让它一直重投")
		}
	})
	t.Run("同一版并发输了", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.handedOver(t, "v1")
		winner, err := domain.ObjectNotResolved(mustValue(t, domain.NewTenantID, "tenant-1"), handoverVersion(t, "v1"),
			domain.ControlTransitionOut, domain.NoAssociatedUnit, transferJudgedAt)
		if err != nil {
			t.Fatal(err)
		}
		fixture.controls.lostRace = true
		fixture.controls.winner = ports.ControlTransitionRecord{Transition: winner, RecordedAt: transferRecordedAt}

		result := fixture.transfer(t, "v1")

		recorded, _ := result.Transition()
		if result.Outcome() != application.ControlTransitionAlreadyRecorded || recorded.Transition != winner {
			t.Fatalf("outcome = %s，交回 %+v, want 赢家那一行", result.Outcome(), recorded)
		}
	})
}

// Covers: ADR-0214 决定六的处置表逐格——落了行、已在册与零写入的几格入账，未决重投，封闭集合以外是缺陷。
func TestEveryOutcomeHasItsDisposition(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.handedOver(t, "v1")
	if got := fixture.transfer(t, "v1").Disposition(); got != application.ControlOnHandoverSettled {
		t.Fatalf("转出 → %s", got)
	}
	if got := (application.TransferControlOnHandoverResult{}).Disposition(); got != application.ControlOnHandoverDefect {
		t.Fatalf("零值 → %s, want DEFECT", got)
	}
}

// Covers: `AT-NO-111` 的编排面（ADR-0199 决定四、ADR-0214 决定三）——收寄之后控制已经转出（读口经折叠交回带转出的控制），同一件实物
// 再送到同一节点：不再并入旧收寄，形成新的收寄与控制起点。
func TestADeliveryAfterTheTransferOutFormsANewIntake(t *testing.T) {
	fixture := newReceptionFixture(t)
	first := receiveCommand(t)
	if result, err := fixture.handler.Handle(context.Background(), first); err != nil || result.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("首次收寄 = %v err = %v", result.Outcome(), err)
	}
	stored := fixture.receptions.byKey[keyOf(first)]
	transferred, err := stored.Control.TransferOut(mustValue(t, domain.NewTransferOutReference, "TRANSPORT-HANDOVER/v1"), transferJudgedAt)
	if err != nil {
		t.Fatalf("转出：%v", err)
	}
	stored.Control = transferred
	fixture.receptions.byKey[keyOf(first)] = stored

	again := deliveredAgain(t, first, "delivery-after-transfer", application.ExplicitReception)
	again.OccurredAt = transferJudgedAt.Add(time.Hour)
	result, err := fixture.handler.Handle(context.Background(), again)
	if err != nil {
		t.Fatalf("再次送站：%v", err)
	}
	record, _ := result.Record()
	if result.Outcome() != application.NodeIntakeFormed || record.Kind == ports.RecordMergedIntoExistingIntake ||
		!record.Control.Active() || record.Control.Basis() == stored.Control.Basis() {
		t.Fatalf("再次送站 = %s kind = %s，控制依据 %q, want 新收寄、新的控制起点", result.Outcome(), record.Kind, record.Control.Basis())
	}
}
