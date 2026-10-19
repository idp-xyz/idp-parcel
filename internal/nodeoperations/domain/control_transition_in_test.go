package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证节点控制转入与交出节点声明不符的领域规则（ADR-0214 决定四末条、决定七与文末票 62 补记）：转入只在没有在身控制、也没有
// 晚于这一版的转出时立，依据由交接键单射铸出；撤回控制的更正在转入一侧带立起的那一份入账；声明的交出节点不符照转、记信息格；读回门
// 与构造是同一套形状。

func judgeIn(t *testing.T, known []domain.PhysicalControl, at time.Time) domain.ControlTransition {
	t.Helper()
	transition, err := domain.JudgeTransferIn(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewHandlingUnitID, "unit-1"),
		known,
		handoverRef(t, "v1"),
		mustValue(t, domain.NewNodeReference, "SYN-NODE-SZX-GATE"),
		at,
	)
	if err != nil {
		t.Fatalf("judge transfer in: %v", err)
	}
	return transition
}

func releasedControl(t *testing.T, basis string, establishedAt, releasedAt time.Time) domain.PhysicalControl {
	t.Helper()
	released, err := activeControl(t, basis, establishedAt).TransferOut(
		mustValue(t, domain.NewTransferOutReference, "TRANSPORT-HANDOVER/earlier"), releasedAt)
	if err != nil {
		t.Fatalf("transfer out: %v", err)
	}
	return released
}

// Covers: `AT-NO-113` 的领域面（ADR-0214 决定七）——没有在身控制、也没有晚于这一版的转出：在接收节点立 HANDOVER_IN 控制，依据由交接键
// 铸出、成立时刻取交接的业务时间，不带转出；读回门认同一份。节点之间直接交接时同一版刚落的转出时刻与这一版相同，不挡转入（`AT-NO-114`
// 的领域面）。
func TestAHandedOverHandoverEstablishesControlAtTheReceivingNode(t *testing.T) {
	for name, known := range map[string][]domain.PhysicalControl{
		"从没有过控制":        nil,
		"同一版刚转出交出节点那一份": {releasedControl(t, "NODE-INTAKE/v-1", controlAt, handedOverAt)},
	} {
		t.Run(name, func(t *testing.T) {
			transition := judgeIn(t, known, handedOverAt)

			if transition.Direction() != domain.ControlTransitionIn || transition.Outcome() != domain.ControlAdvanced {
				t.Fatalf("transition = %s %s %s", transition.Direction(), transition.Outcome(), transition.Reason())
			}
			control, carries := transition.Control()
			if !carries || !control.Active() || control.EstablishmentKind() != domain.EstablishedByHandoverIn ||
				control.Node().String() != "SYN-NODE-SZX-GATE" || !control.EstablishedAt().Equal(handedOverAt) ||
				control.Basis() != domain.HandoverInControlBasis(handoverRef(t, "v1")) {
				t.Fatalf("立起的控制 = %+v", control)
			}
			if _, mismatched := transition.ReleasingNodeMismatch(); mismatched {
				t.Fatal("转入不带交出节点的信息格")
			}
			again, err := domain.RehydrateControlTransition(domain.ControlTransitionSpec{
				TenantID: transition.TenantID(), Handover: transition.Handover(), Direction: transition.Direction(),
				Outcome: transition.Outcome(), Unit: mustValue(t, domain.NewHandlingUnitID, "unit-1"), Control: control,
				OccurredAt: transition.OccurredAt(),
			})
			if err != nil || again != transition {
				t.Fatalf("读回 = %+v err = %v, want 与构造的同一份", again, err)
			}
		})
	}
}

// Covers: `AT-NO-118`、`AT-NO-119` 的领域面（ADR-0214 决定七落地补记）——仍有在身控制：不另立第二份；最近一次转出晚于这一版：晚到的
// 旧交接不立。两格都入账、只带实物。
func TestATransferInThatWouldLeaveTwoOrAStaleControlIsRecordedNotAdvanced(t *testing.T) {
	cases := map[string]struct {
		known  []domain.PhysicalControl
		reason domain.ControlNotAdvancedReason
	}{
		"仍有在身控制": {known: []domain.PhysicalControl{activeControl(t, "NODE-INTAKE/v-1", controlAt)},
			reason: domain.ControlStillActive},
		"在身的与转出过的都有": {known: []domain.PhysicalControl{
			releasedControl(t, "NODE-INTAKE/v-1", controlAt, controlAt.Add(time.Hour)), activeControl(t, "NODE-INTAKE/v-2", controlAt),
		}, reason: domain.ControlStillActive},
		"最近一次转出晚于这一版": {known: []domain.PhysicalControl{
			releasedControl(t, "NODE-INTAKE/v-1", controlAt, controlAt.Add(time.Hour)),
			releasedControl(t, "NODE-INTAKE/v-2", controlAt, handedOverAt.Add(time.Minute)),
		}, reason: domain.HandoverPrecedesRelease},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			transition := judgeIn(t, tc.known, handedOverAt)
			if transition.Outcome() != domain.ControlNotAdvanced || transition.Reason() != tc.reason ||
				transition.Direction() != domain.ControlTransitionIn {
				t.Fatalf("transition = %s %s %s", transition.Direction(), transition.Outcome(), transition.Reason())
			}
			if unit, present := transition.Unit(); !present || unit.String() != "unit-1" {
				t.Fatal("这两格都对上了实物，该带实物")
			}
			if _, carries := transition.Control(); carries {
				t.Fatal("这两格不带控制")
			}
		})
	}
	if _, err := domain.JudgeTransferIn(mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewHandlingUnitID, "unit-1"),
		nil, handoverRef(t, "v1"), domain.NodeReference{}, handedOverAt); !errors.Is(err, domain.ErrInvalidControlTransition) {
		t.Fatalf("没声明接收节点 err = %v, want ErrInvalidControlTransition（编排不该走到这里）", err)
	}
	minted := activeControl(t, domain.HandoverInControlBasis(handoverRef(t, "v1")).String(), controlAt)
	if _, err := domain.JudgeTransferIn(mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewHandlingUnitID, "unit-1"),
		[]domain.PhysicalControl{minted}, handoverRef(t, "v1"), mustValue(t, domain.NewNodeReference, "SYN-NODE-SZX-GATE"),
		handedOverAt); !errors.Is(err, domain.ErrInvalidControlTransition) {
		t.Fatalf("这一版已立过 err = %v, want ErrInvalidControlTransition", err)
	}
}

// Covers: ADR-0214 决定七落地补记——转入依据由交接键单射铸出：三件以斜杠直拼会撞的两组键铸出不同的依据；同一版本串在两个范围下也
// 不同。
func TestTheHandoverInBasisIsInjectiveOverTheHandoverKey(t *testing.T) {
	pairs := [][2][3]string{
		{{"SYN-PARCEL/A", "SCOPE", "v1"}, {"SYN-PARCEL", "A/SCOPE", "v1"}},
		{{"SYN-PARCEL", "SCOPE-1", "v1"}, {"SYN-PARCEL", "SCOPE-2", "v1"}},
	}
	for _, pair := range pairs {
		left, err := domain.NewAuthoritativeHandoverReference(pair[0][0], pair[0][1], pair[0][2])
		if err != nil {
			t.Fatal(err)
		}
		right, err := domain.NewAuthoritativeHandoverReference(pair[1][0], pair[1][1], pair[1][2])
		if err != nil {
			t.Fatal(err)
		}
		if domain.HandoverInControlBasis(left) == domain.HandoverInControlBasis(right) {
			t.Fatalf("%v 与 %v 铸出同一个依据 %q", pair[0], pair[1], domain.HandoverInControlBasis(left))
		}
	}
	if got := domain.HandoverInControlBasis(handoverRef(t, "v1")).String(); got != "HANDOVER-IN/13:SYN-PARCEL-61/12:SYN-SCOPE-61/2:v1" {
		t.Fatalf("依据 = %q", got)
	}
}

// Covers: `AT-NO-120` 的领域面（ADR-0214 决定四末条）——声明的交出节点不是被转出那一份控制的节点：照转，信息格记声明的那一个；声明的
// 就是那个节点、或没声明：不记。读回门拒信息格出现在转入、没推进的行上，或与控制节点相同。
func TestAMismatchedReleasingNodeIsNotedOnTheTransfer(t *testing.T) {
	tenant, unit := mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewHandlingUnitID, "unit-1")
	held := activeControl(t, "NODE-INTAKE/v-1", controlAt)
	for name, tc := range map[string]struct {
		declared string
		noted    bool
	}{
		"声明不符":     {declared: "SYN-NODE-SZX-GATE", noted: true},
		"声明就是那个节点": {declared: "node-origin"},
		"没声明":      {},
	} {
		t.Run(name, func(t *testing.T) {
			declared := domain.NodeReference{}
			if tc.declared != "" {
				declared = mustValue(t, domain.NewNodeReference, tc.declared)
			}
			transition, err := domain.JudgeTransferOut(tenant, unit, []domain.PhysicalControl{held}, handoverRef(t, "v1"),
				mustValue(t, domain.NewTransferOutReference, "TRANSPORT-HANDOVER/v1"), declared, handedOverAt)
			if err != nil || transition.Outcome() != domain.ControlAdvanced {
				t.Fatalf("照转：%v %s", err, transition.Outcome())
			}
			node, noted := transition.ReleasingNodeMismatch()
			if noted != tc.noted || (noted && node.String() != tc.declared) {
				t.Fatalf("信息格 = %q noted=%v, want noted=%v", node, noted, tc.noted)
			}
			control, _ := transition.Control()
			again, err := domain.RehydrateControlTransition(domain.ControlTransitionSpec{
				TenantID: tenant, Handover: transition.Handover(), Direction: transition.Direction(), Outcome: transition.Outcome(),
				Unit: unit, Control: control, OccurredAt: transition.OccurredAt(), MismatchedReleasingNode: node,
			})
			if err != nil || again != transition {
				t.Fatalf("读回 = %+v err = %v", again, err)
			}
		})
	}

	transferred := judgeOut(t, []domain.PhysicalControl{held}, handedOverAt)
	control, _ := transferred.Control()
	established := judgeIn(t, nil, handedOverAt)
	inControl, _ := established.Control()
	for name, spec := range map[string]domain.ControlTransitionSpec{
		"与控制节点相同": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionOut,
			Outcome: domain.ControlAdvanced, Unit: unit, Control: control, OccurredAt: handedOverAt,
			MismatchedReleasingNode: mustValue(t, domain.NewNodeReference, "node-origin")},
		"在转入上": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionIn,
			Outcome: domain.ControlAdvanced, Unit: unit, Control: inControl, OccurredAt: handedOverAt,
			MismatchedReleasingNode: mustValue(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB")},
		"在没推进的行上": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionOut,
			Outcome: domain.ControlNotAdvanced, Reason: domain.NoActiveControl, Unit: unit, OccurredAt: handedOverAt,
			MismatchedReleasingNode: mustValue(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB")},
	} {
		if _, err := domain.RehydrateControlTransition(spec); !errors.Is(err, domain.ErrInvalidControlTransition) {
			t.Fatalf("%s: err = %v, want ErrInvalidControlTransition", name, err)
		}
	}
}

// Covers: `AT-NO-122` 的领域面（ADR-0214 决定五、七）——转入之后撤回控制的更正：同方向入账「控制来源链待重算」，带先前立起的那一份
// （在身、不带转出），不撤那一份。
func TestAWithdrawingCorrectionOfATransferInRecordsThePendingRecompute(t *testing.T) {
	earlier := judgeIn(t, nil, handedOverAt)

	pending, err := domain.PendingControlSourceRecompute(earlier, handoverRef(t, "v2"), handedOverAt)
	if err != nil {
		t.Fatalf("pending recompute: %v", err)
	}
	control, carries := pending.Control()
	established, _ := earlier.Control()
	if pending.Direction() != domain.ControlTransitionIn || pending.Reason() != domain.ControlSourceChainPendingRecompute ||
		!carries || control != established || !control.Active() {
		t.Fatalf("pending = %s %s carries=%v", pending.Direction(), pending.Reason(), carries)
	}
	again, err := domain.RehydrateControlTransition(domain.ControlTransitionSpec{
		TenantID: pending.TenantID(), Handover: pending.Handover(), Direction: pending.Direction(), Outcome: pending.Outcome(),
		Reason: pending.Reason(), Unit: mustValue(t, domain.NewHandlingUnitID, "unit-1"), Control: control,
		OccurredAt: pending.OccurredAt(),
	})
	if err != nil || again != pending {
		t.Fatalf("读回 = %+v err = %v", again, err)
	}
}

// Covers: 读回门与构造函数同一套形状（ADR-0214 决定七落地补记；迁移 0018 的 CHECK 同守）——只属转出的格出现在转入上、只属转入的格出现在
// 转出上、转入立起的控制成立时刻不是业务时间，都读不回来；交接早于控制成立那一格在微秒截断后与控制成立时刻相等时照样读得回。
func TestRehydrationKeepsEachReasonToItsDirection(t *testing.T) {
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	unit := mustValue(t, domain.NewHandlingUnitID, "unit-1")
	established := judgeIn(t, nil, handedOverAt)
	inControl, _ := established.Control()
	for name, spec := range map[string]domain.ControlTransitionSpec{
		"无在身控制在转入上": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionIn,
			Outcome: domain.ControlNotAdvanced, Reason: domain.NoActiveControl, Unit: unit, OccurredAt: handedOverAt},
		"仍有在身控制在转出上": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionOut,
			Outcome: domain.ControlNotAdvanced, Reason: domain.ControlStillActive, Unit: unit, OccurredAt: handedOverAt},
		"晚于转出在转出上": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionOut,
			Outcome: domain.ControlNotAdvanced, Reason: domain.HandoverPrecedesRelease, Unit: unit, OccurredAt: handedOverAt},
		"转入成立时刻不是业务时间": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionIn,
			Outcome: domain.ControlAdvanced, Unit: unit, Control: inControl, OccurredAt: handedOverAt.Add(time.Minute)},
	} {
		if _, err := domain.RehydrateControlTransition(spec); !errors.Is(err, domain.ErrInvalidControlTransition) {
			t.Fatalf("%s: err = %v, want ErrInvalidControlTransition", name, err)
		}
	}

	truncated := activeControl(t, "NODE-INTAKE/v-1", handedOverAt)
	if _, err := domain.RehydrateControlTransition(domain.ControlTransitionSpec{
		TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionOut, Outcome: domain.ControlNotAdvanced,
		Reason: domain.HandoverPrecedesControl, Unit: unit, Control: truncated, OccurredAt: handedOverAt,
	}); err != nil {
		t.Fatalf("截断后相等的交接早于控制 err = %v, want 读得回", err)
	}
}
