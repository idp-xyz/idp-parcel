package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证节点控制按权威运输交接转入的编排（ADR-0214 决定七与文末票 62 补记，UC-NO-005 转入一半），以及同一版交接先转出、后转入：
// 没声明接收节点不立；对不上作业实物入账不造实物；仍有在身控制、最近一次转出晚于这一版都入账；更正链与已在册的各格同转出一半；转出
// 一侧声明的交出节点不符照转、记信息格。替身照真库的折叠读控制，真库的锁与 CHECK 由 adapters/postgres 与派发真库用例证。

const (
	shanghaiHub = "SYN-NODE-SHA-HUB"
	shenzhenGW  = "SYN-NODE-SZX-GATE"
	singaporeHB = "SYN-NODE-SIN-HUB"
)

// declaredHandover 登一版声明了节点的交接。scope 不同即不同的更正链；替身的交接来源按版本串认，所以各版版本串要各不相同。
func (fixture *transferFixture) declaredHandover(t *testing.T, scope, version, releasing, receiving string, handedOver bool) {
	t.Helper()
	reference, err := domain.NewAuthoritativeHandoverReference("parcel-1", scope, version)
	if err != nil {
		t.Fatalf("交接键：%v", err)
	}
	answer := ports.AuthoritativeHandover{Reference: reference, HandedOver: handedOver, JudgedAt: transferJudgedAt}
	if handedOver {
		answer.TransferOut = mustValue(t, domain.NewTransferOutReference, "TRANSPORT-HANDOVER/"+version)
	}
	if releasing != "" {
		answer.ReleasingNode = mustValue(t, domain.NewNodeReference, releasing)
	}
	if receiving != "" {
		answer.ReceivingNode = mustValue(t, domain.NewNodeReference, receiving)
	}
	fixture.handovers.answers[version] = answer
}

func (fixture *transferFixture) advanceIn(t *testing.T, scope, version string) application.AdvanceControlOnHandoverResult {
	t.Helper()
	reference, err := domain.NewAuthoritativeHandoverReference("parcel-1", scope, version)
	if err != nil {
		t.Fatalf("交接键：%v", err)
	}
	result, err := fixture.handler.Advance(t.Context(), application.TransferControlOnHandoverCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"), Handover: reference,
	})
	if err != nil {
		t.Fatalf("推进 %s/%s：%v", scope, version, err)
	}
	return result
}

func establishedIn(t *testing.T, result application.TransferControlOnHandoverResult) domain.PhysicalControl {
	t.Helper()
	if result.Outcome() != application.ControlEstablishedInOnHandover {
		t.Fatalf("转入 = %s, want CONTROL_ESTABLISHED_IN", result.Outcome())
	}
	record, _ := result.Transition()
	control, carries := record.Transition.Control()
	if !carries || !control.Active() || control.EstablishmentKind() != domain.EstablishedByHandoverIn {
		t.Fatalf("转入那一行 = %+v", record)
	}
	return control
}

// Covers: `AT-NO-113`（ADR-0214 决定七）——`已交接`、声明了接收节点、对上一件作业实物、它没有在身控制：在接收节点立 HANDOVER_IN 控制，
// 成立时刻取交接的业务时间、记录时刻取时钟；同一版的转出一半照 61 入账「无在身控制」。整封入账。
func TestAHandoverIntoANodeEstablishesControlThere(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.controls.active = nil
	fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", "", shenzhenGW, true)

	result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

	control := establishedIn(t, result.In())
	if control.Node().String() != shenzhenGW || !control.EstablishedAt().Equal(transferJudgedAt) {
		t.Fatalf("立在 %s @ %s", control.Node(), control.EstablishedAt())
	}
	record, _ := result.In().Transition()
	if !record.RecordedAt.Equal(transferRecordedAt) || record.Transition.Direction() != domain.ControlTransitionIn {
		t.Fatalf("转入那一行 = %+v", record)
	}
	if out, _ := result.Out().Transition(); result.Out().Outcome() != application.ControlNotAdvancedOnHandover ||
		out.Transition.Reason() != domain.NoActiveControl {
		t.Fatalf("转出一半 = %s %s", result.Out().Outcome(), out.Transition.Reason())
	}
	if result.Disposition() != application.ControlOnHandoverSettled {
		t.Fatalf("处置 = %s", result.Disposition())
	}
}

// Covers: `AT-NO-114`（ADR-0214 决定四、七）——节点之间直接交接：同一版声明了交出与接收节点，先转出交出节点那一份，再在接收节点立；
// 两半各取一次同一把实物锁，转入读得到刚落的转出。
func TestANodeToNodeHandoverTransfersOutThenIn(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", shanghaiHub, shenzhenGW, true)

	result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

	if result.Out().Outcome() != application.ControlTransferredOutOnHandover {
		t.Fatalf("转出一半 = %s", result.Out().Outcome())
	}
	if control := establishedIn(t, result.In()); control.Node().String() != shenzhenGW {
		t.Fatalf("立在 %s", control.Node())
	}
	if len(fixture.controls.locked) != 2 || fixture.controls.locked[0] != "unit-1" || fixture.controls.locked[1] != "unit-1" {
		t.Fatalf("取锁 %v, want 两半各取 unit-1 那一把", fixture.controls.locked)
	}
}

// Covers: `AT-NO-115`（ADR-0214 决定四、七）——转入立起的控制，此后被以那个节点为交出节点的另一次交接转出：转出那一份的依据就是转入
// 铸出的依据，声明的交出节点与它相符，不记信息格；这次交接只交给运输方、不声明接收节点，转入一半零写入。
func TestAControlEstablishedInIsTransferredOutByTheNextHandover(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", shanghaiHub, shenzhenGW, true)
	inbound := establishedIn(t, fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1").In())
	fixture.declaredHandover(t, "SYN-SCOPE-TRUNK", "trunk-v1", shenzhenGW, "", true)

	result := fixture.advanceIn(t, "SYN-SCOPE-TRUNK", "trunk-v1")

	record, _ := result.Out().Transition()
	released, _ := record.Transition.Control()
	if result.Out().Outcome() != application.ControlTransferredOutOnHandover || released.Basis() != inbound.Basis() ||
		released.Node().String() != shenzhenGW || released.Active() {
		t.Fatalf("转出 = %s，转出的是 %q@%s", result.Out().Outcome(), released.Basis(), released.Node())
	}
	if _, noted := record.Transition.ReleasingNodeMismatch(); noted {
		t.Fatal("声明的交出节点就是在身控制的节点，不该记信息格")
	}
	if result.In().Outcome() != application.HandoverDeclaresNoReceivingNode {
		t.Fatalf("交给运输方的转入一半 = %s, want NO_RECEIVING_NODE_DECLARED", result.In().Outcome())
	}
}

// Covers: `AT-NO-116`（ADR-0214 决定七，ADR-0213 决定二）——没声明接收节点：转入零写入，不认对象、不取锁、不落行；转出一半照常。
func TestAHandoverWithoutAReceivingNodeEstablishesNothing(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", shanghaiHub, "", true)

	result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

	if result.In().Outcome() != application.HandoverDeclaresNoReceivingNode || result.Disposition() != application.ControlOnHandoverSettled {
		t.Fatalf("转入 = %s 处置 = %s", result.In().Outcome(), result.Disposition())
	}
	if fixture.controls.saves != 1 || fixture.objects.asked != 1 || len(fixture.controls.locked) != 1 {
		t.Fatalf("落册 %d、认对象 %d、取锁 %v, want 只有转出那一次", fixture.controls.saves, fixture.objects.asked, fixture.controls.locked)
	}
}

// Covers: `AT-NO-117`（ADR-0214 决定二、七）——对不上一件作业实物（零件、多件、集运单元）：转入一侧也入账一格，不取锁、不造实物。
func TestATransferInThatCannotNameAUnitIsRecordedNotAdvanced(t *testing.T) {
	two := []domain.HandlingUnitID{mustValue(t, domain.NewHandlingUnitID, "unit-1"), mustValue(t, domain.NewHandlingUnitID, "unit-2")}
	for name, tc := range map[string]struct {
		resolution ports.HandoverObjectResolution
		reason     domain.ControlNotAdvancedReason
	}{
		"零件":   {resolution: ports.HandoverObjectResolution{Kind: ports.HandoverObjectParcel}, reason: domain.NoAssociatedUnit},
		"多件":   {resolution: ports.HandoverObjectResolution{Kind: ports.HandoverObjectParcel, Units: two}, reason: domain.SeveralAssociatedUnits},
		"集运单元": {resolution: ports.HandoverObjectResolution{Kind: ports.HandoverObjectConsolidationUnit}, reason: domain.ConsolidationUnitPendingRule},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newTransferFixture(t)
			fixture.objects.resolution = tc.resolution
			fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", "", shenzhenGW, true)

			result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

			record, _ := result.In().Transition()
			if result.In().Outcome() != application.ControlNotAdvancedOnHandover || record.Transition.Reason() != tc.reason ||
				record.Transition.Direction() != domain.ControlTransitionIn {
				t.Fatalf("转入 = %s %s", result.In().Outcome(), record.Transition.Reason())
			}
			if len(fixture.controls.locked) != 0 {
				t.Fatalf("对不上实物却取了锁 %v", fixture.controls.locked)
			}
		})
	}
}

// Covers: `AT-NO-118`、`AT-NO-119`（ADR-0214 决定七落地补记）——转出一半没推进、实物仍有在身控制：转入入账「仍有在身控制」，不另立；
// 实物最近一次转出晚于这一版（晚到的旧交接）：入账「交接早于最近一次转出」。
func TestATransferInThatWouldLeaveTwoOrAStaleControlIsNotAdvanced(t *testing.T) {
	t.Run("仍有在身控制", func(t *testing.T) {
		fixture := newTransferFixture(t)
		second := heldIntakeControl(t)
		spec := domain.PhysicalControlSpec{
			TenantID: second.TenantID(), Unit: second.Unit(), Node: mustValue(t, domain.NewNodeReference, shenzhenGW),
			Kind: domain.EstablishedByNodeIntake, Basis: mustValue(t, domain.NewControlBasisReference, "NODE-INTAKE/intake-2"),
			EstablishedAt: deliveredAt,
		}
		other, err := domain.EstablishPhysicalControl(spec)
		if err != nil {
			t.Fatal(err)
		}
		fixture.controls.active = append(fixture.controls.active, other)
		fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", shanghaiHub, singaporeHB, true)

		result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

		out, _ := result.Out().Transition()
		in, _ := result.In().Transition()
		if out.Transition.Reason() != domain.SeveralActiveControls || in.Transition.Reason() != domain.ControlStillActive {
			t.Fatalf("转出 %s、转入 %s", out.Transition.Reason(), in.Transition.Reason())
		}
	})
	t.Run("最近一次转出晚于这一版", func(t *testing.T) {
		fixture := newTransferFixture(t)
		released, err := heldIntakeControl(t).TransferOut(
			mustValue(t, domain.NewTransferOutReference, "TRANSPORT-HANDOVER/later"), transferJudgedAt.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		fixture.controls.active, fixture.controls.released = nil, []domain.PhysicalControl{released}
		fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", "", shenzhenGW, true)

		result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

		in, _ := result.In().Transition()
		if result.In().Outcome() != application.ControlNotAdvancedOnHandover || in.Transition.Reason() != domain.HandoverPrecedesRelease {
			t.Fatalf("转入 = %s %s", result.In().Outcome(), in.Transition.Reason())
		}
	})
}

// Covers: `AT-NO-120`（ADR-0214 决定四末条）——声明的交出节点不是在身控制的节点：照转，信息格记声明的那一个。
func TestAMismatchedReleasingNodeStillTransfersOut(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", shenzhenGW, "", true)

	result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

	record, _ := result.Out().Transition()
	node, noted := record.Transition.ReleasingNodeMismatch()
	if result.Out().Outcome() != application.ControlTransferredOutOnHandover || !noted || node.String() != shenzhenGW {
		t.Fatalf("转出 = %s，信息格 %q noted=%v", result.Out().Outcome(), node, noted)
	}
}

// Covers: `AT-NO-121`（ADR-0214 决定五、六、七）——转入一侧的重投与更正同转出一半：同一版重投交回已在册那一行、不再认对象；后继版仍
// `已交接`零写入，它的转出一半也不把同链立起的那一份转出；被接续了的前版晚到零写入；并发里同一版、同链前版在锁外刚转入，锁下认出来。
func TestTransferInRedeliveriesAndCorrectionsWriteNothingNew(t *testing.T) {
	t.Run("重投", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.controls.active = nil
		fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", "", shenzhenGW, true)
		first, _ := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1").In().Transition()
		saves, asked := fixture.controls.saves, fixture.objects.asked

		again := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

		recorded, _ := again.In().Transition()
		if again.In().Outcome() != application.ControlTransitionAlreadyRecorded || recorded.Transition != first.Transition ||
			fixture.controls.saves != saves || fixture.objects.asked != asked {
			t.Fatalf("重投 = %s，落册 +%d、认对象 +%d", again.In().Outcome(), fixture.controls.saves-saves, fixture.objects.asked-asked)
		}
	})
	t.Run("后继版仍已交接", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.controls.active = nil
		fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", "", shenzhenGW, true)
		fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v2", "", shenzhenGW, true)
		inbound := establishedIn(t, fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1").In())

		result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v2")

		if result.In().Outcome() != application.ControlAlreadyEstablishedInChain {
			t.Fatalf("转入 = %s", result.In().Outcome())
		}
		// 前版的转出没推进（当时没有在身控制），这一版照 ADR-0214 决定五按首版重判转出；同链前版刚立起的那一份不是它要交出的东西。
		out, _ := result.Out().Transition()
		if result.Out().Outcome() != application.ControlNotAdvancedOnHandover || out.Transition.Reason() != domain.NoActiveControl {
			t.Fatalf("转出 = %s %s, want 入账「无在身控制」而不是把同链立起的控制转出", result.Out().Outcome(), out.Transition.Reason())
		}
		known, err := fixture.controls.FindActiveControls(t.Context(), mustValue(t, domain.NewTenantID, "tenant-1"),
			mustValue(t, domain.NewHandlingUnitID, "unit-1"))
		if err != nil || len(known) != 1 || known[0] != inbound {
			t.Fatalf("在身控制 = %v, want 前版立起的那一份", known)
		}
	})
	t.Run("被接续的前版晚到", func(t *testing.T) {
		fixture := newTransferFixture(t)
		fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", "", shenzhenGW, true)
		answer := fixture.handovers.answers["in-v1"]
		answer.Superseded = true
		fixture.handovers.answers["in-v1"] = answer

		result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

		if result.In().Outcome() != application.HandoverVersionSuperseded || fixture.controls.saves != 0 {
			t.Fatalf("转入 = %s，落册 %d", result.In().Outcome(), fixture.controls.saves)
		}
	})
	for name, version := range map[string]string{"同一版并发": "in-v1", "同链前版在锁外刚转入": "in-v0"} {
		t.Run(name, func(t *testing.T) {
			fixture := newTransferFixture(t)
			fixture.controls.active = nil
			reference, err := domain.NewAuthoritativeHandoverReference("parcel-1", "SYN-SCOPE-IN", version)
			if err != nil {
				t.Fatal(err)
			}
			winner, err := domain.JudgeTransferIn(mustValue(t, domain.NewTenantID, "tenant-1"),
				mustValue(t, domain.NewHandlingUnitID, "unit-1"), nil, reference, mustValue(t, domain.NewNodeReference, shenzhenGW),
				transferJudgedAt)
			if err != nil {
				t.Fatal(err)
			}
			fixture.controls.advanceInUnderLock = &ports.ControlTransitionRecord{Transition: winner, RecordedAt: transferRecordedAt}
			fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", "", shenzhenGW, true)
			saves := fixture.controls.saves

			result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1")

			want := application.ControlAlreadyEstablishedInChain
			if version == "in-v1" {
				want = application.ControlTransitionAlreadyRecorded
			}
			if result.In().Outcome() != want || fixture.controls.saves != saves+1 {
				t.Fatalf("转入 = %s want %s，落册 +%d（只有转出一半那一行）", result.In().Outcome(), want, fixture.controls.saves-saves)
			}
		})
	}
}

// Covers: `AT-NO-122`（ADR-0214 决定五、七）——节点之间直接交接推进了两半之后被更正成拒收：两个方向各入账一行「控制来源链待重算」，转出
// 一侧带先前那次转出，转入一侧带先前立起的那一份；立起的那一份照旧在身，不撤。
func TestAWithdrawingCorrectionOfANodeToNodeHandoverIsRecordedOnBothSides(t *testing.T) {
	fixture := newTransferFixture(t)
	fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v1", shanghaiHub, shenzhenGW, true)
	inbound := establishedIn(t, fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v1").In())
	fixture.declaredHandover(t, "SYN-SCOPE-IN", "in-v2", shanghaiHub, shenzhenGW, false)

	result := fixture.advanceIn(t, "SYN-SCOPE-IN", "in-v2")

	out, _ := result.Out().Transition()
	in, _ := result.In().Transition()
	if out.Transition.Reason() != domain.ControlSourceChainPendingRecompute || in.Transition.Reason() != domain.ControlSourceChainPendingRecompute ||
		in.Transition.Direction() != domain.ControlTransitionIn {
		t.Fatalf("转出 %s、转入 %s %s", out.Transition.Reason(), in.Transition.Direction(), in.Transition.Reason())
	}
	if carried, _ := in.Transition.Control(); carried != inbound {
		t.Fatalf("转入一侧带的 = %+v, want 先前立起的那一份", carried)
	}
	known, err := fixture.controls.FindActiveControls(t.Context(), mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewHandlingUnitID, "unit-1"))
	if err != nil || len(known) != 1 || known[0] != inbound {
		t.Fatalf("在身控制 = %v, want 立起的那一份照旧在身", known)
	}
}

// Covers: ADR-0214 决定六——两半折成一个处置：任一半封闭集合以外即缺陷；读不动两半同答未决、整封重投。
func TestBothHalvesFoldIntoOneDisposition(t *testing.T) {
	if got := (application.AdvanceControlOnHandoverResult{}).Disposition(); got != application.ControlOnHandoverDefect {
		t.Fatalf("零值 → %s, want DEFECT", got)
	}
	fixture := newTransferFixture(t)
	result := fixture.advanceIn(t, "SYN-SCOPE-IN", "unseen")
	if result.Disposition() != application.ControlOnHandoverRetry || result.In().Outcome() != application.ControlOnHandoverUndecided ||
		result.In().UndecidedReason() != application.ControlOnHandoverHandoverNotVisible {
		t.Fatalf("读不回 → %s，转入 %s %s", result.Disposition(), result.In().Outcome(), result.In().UndecidedReason())
	}
}
