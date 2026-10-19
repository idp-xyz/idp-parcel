package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证节点控制转移的领域规则（ADR-0214 决定二至五）：已交接只转出此刻唯一在身的那一份、交接早于控制成立不转出、对不上实物与撤回
// 控制的更正都入账不推进，以及读回门与构造函数是同一套形状。

var handedOverAt = controlAt.Add(4 * time.Hour)

func handoverRef(t *testing.T, version string) domain.AuthoritativeHandoverReference {
	t.Helper()
	reference, err := domain.NewAuthoritativeHandoverReference("SYN-PARCEL-61", "SYN-SCOPE-61", version)
	if err != nil {
		t.Fatalf("handover reference: %v", err)
	}
	return reference
}

func activeControl(t *testing.T, basis string, at time.Time) domain.PhysicalControl {
	t.Helper()
	spec := controlSpec(t, domain.EstablishedByNodeIntake, "unit-1")
	spec.Basis = mustValue(t, domain.NewControlBasisReference, basis)
	spec.EstablishedAt = at
	control, err := domain.EstablishPhysicalControl(spec)
	if err != nil {
		t.Fatalf("establish control: %v", err)
	}
	return control
}

func judgeOut(t *testing.T, active []domain.PhysicalControl, at time.Time) domain.ControlTransition {
	t.Helper()
	transition, err := domain.JudgeTransferOut(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewHandlingUnitID, "unit-1"),
		active,
		handoverRef(t, "v1"),
		mustValue(t, domain.NewTransferOutReference, "TRANSPORT-HANDOVER/v1"),
		domain.NodeReference{},
		at,
	)
	if err != nil {
		t.Fatalf("judge transfer out: %v", err)
	}
	return transition
}

// Covers: `AT-NO-101` 的领域面（ADR-0214 决定四）——恰一份在身控制、交接不早于它成立：转出，转出依据是交接给的引用、时刻取交接的业务
// 时间；转出那一份控制随行，读回门认同一份。
func TestAHandedOverHandoverTransfersTheOnlyActiveControl(t *testing.T) {
	transition := judgeOut(t, []domain.PhysicalControl{activeControl(t, "NODE-INTAKE/v-1", controlAt)}, handedOverAt)

	if transition.Direction() != domain.ControlTransitionOut || transition.Outcome() != domain.ControlAdvanced ||
		transition.Reason() != domain.ControlNotAdvancedReasonAbsent {
		t.Fatalf("transition = %s %s %s", transition.Direction(), transition.Outcome(), transition.Reason())
	}
	control, carries := transition.Control()
	if !carries || control.Active() {
		t.Fatal("转出那一行该带已转出的控制")
	}
	reference, at, released := control.Release()
	if !released || reference.String() != "TRANSPORT-HANDOVER/v1" || !at.Equal(handedOverAt) {
		t.Fatalf("release = %q %s %v", reference, at, released)
	}

	again, err := domain.RehydrateControlTransition(domain.ControlTransitionSpec{
		TenantID: transition.TenantID(), Handover: transition.Handover(), Direction: transition.Direction(),
		Outcome: transition.Outcome(), Unit: mustValue(t, domain.NewHandlingUnitID, "unit-1"), Control: control,
		OccurredAt: transition.OccurredAt(),
	})
	if err != nil || again != transition {
		t.Fatalf("读回 = %+v err = %v, want 与构造的同一份", again, err)
	}
}

// Covers: `AT-NO-107`（ADR-0214 决定四）——没有在身控制、不止一份在身控制、交接早于那一份成立：都入账不推进；前两格只带实物，第三格
// 带那一份在身控制。
func TestTransfersThatCannotNameOneControlAreRecordedNotAdvanced(t *testing.T) {
	cases := map[string]struct {
		active  []domain.PhysicalControl
		at      time.Time
		reason  domain.ControlNotAdvancedReason
		control bool
	}{
		"没有在身控制": {at: handedOverAt, reason: domain.NoActiveControl},
		"不止一份": {active: []domain.PhysicalControl{
			activeControl(t, "NODE-INTAKE/v-1", controlAt), activeControl(t, "NODE-INTAKE/v-2", controlAt.Add(time.Hour)),
		}, at: handedOverAt, reason: domain.SeveralActiveControls},
		"交接早于控制成立": {active: []domain.PhysicalControl{activeControl(t, "NODE-INTAKE/v-1", handedOverAt)},
			at: handedOverAt.Add(-time.Minute), reason: domain.HandoverPrecedesControl, control: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			transition := judgeOut(t, tc.active, tc.at)
			if transition.Outcome() != domain.ControlNotAdvanced || transition.Reason() != tc.reason {
				t.Fatalf("transition = %s %s, want NOT_ADVANCED %s", transition.Outcome(), transition.Reason(), tc.reason)
			}
			if unit, present := transition.Unit(); !present || unit.String() != "unit-1" {
				t.Fatal("这几格都对上了实物，该带实物")
			}
			if control, carries := transition.Control(); carries != tc.control || (carries && !control.Active()) {
				t.Fatalf("carries control = %v, want %v 且那一份不被转出", carries, tc.control)
			}
		})
	}
}

// Covers: `AT-NO-104`、`AT-NO-105`、`AT-NO-106` 的领域面（ADR-0214 决定二）——对不上实物的三格只入账原因，不带实物与控制；需要实物的
// 原因不能经这一口入账。
func TestUnresolvedObjectsAreRecordedWithoutAUnit(t *testing.T) {
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	for _, reason := range []domain.ControlNotAdvancedReason{
		domain.NoAssociatedUnit, domain.SeveralAssociatedUnits, domain.ConsolidationUnitPendingRule,
	} {
		transition, err := domain.ObjectNotResolved(tenant, handoverRef(t, "v1"), domain.ControlTransitionOut, reason, handedOverAt)
		if err != nil {
			t.Fatalf("%s: %v", reason, err)
		}
		if _, present := transition.Unit(); present || transition.Reason() != reason {
			t.Fatalf("%s: unit present = %v reason = %s", reason, present, transition.Reason())
		}
	}
	if _, err := domain.ObjectNotResolved(tenant, handoverRef(t, "v1"), domain.ControlTransitionOut,
		domain.NoActiveControl, handedOverAt); !errors.Is(err, domain.ErrInvalidControlTransition) {
		t.Fatalf("err = %v, want ErrInvalidControlTransition", err)
	}
}

// Covers: `AT-NO-109` 的领域面（ADR-0214 决定五）——撤回控制的更正带着先前那次转出入账「控制来源链待重算」，控制不回退；只认同一条
// 更正链上推进了的转出。
func TestAWithdrawingCorrectionRecordsThePendingRecompute(t *testing.T) {
	earlier := judgeOut(t, []domain.PhysicalControl{activeControl(t, "NODE-INTAKE/v-1", controlAt)}, handedOverAt)

	pending, err := domain.PendingControlSourceRecompute(earlier, handoverRef(t, "v2"), handedOverAt)
	if err != nil {
		t.Fatalf("pending recompute: %v", err)
	}
	control, carries := pending.Control()
	if pending.Outcome() != domain.ControlNotAdvanced || pending.Reason() != domain.ControlSourceChainPendingRecompute ||
		!carries || control.Active() || pending.Handover().Version() != "v2" {
		t.Fatalf("pending = %s %s carries=%v active=%v", pending.Outcome(), pending.Reason(), carries, control.Active())
	}

	other, err := domain.NewAuthoritativeHandoverReference("SYN-PARCEL-61", "SYN-SCOPE-OTHER", "v2")
	if err != nil {
		t.Fatal(err)
	}
	notAdvanced := judgeOut(t, nil, handedOverAt)
	for name, attempt := range map[string]func() error{
		"另一条链": func() error { _, err := domain.PendingControlSourceRecompute(earlier, other, handedOverAt); return err },
		"同一版": func() error {
			_, err := domain.PendingControlSourceRecompute(earlier, handoverRef(t, "v1"), handedOverAt)
			return err
		},
		"前版没推进过": func() error {
			_, err := domain.PendingControlSourceRecompute(notAdvanced, handoverRef(t, "v2"), handedOverAt)
			return err
		},
	} {
		if err := attempt(); !errors.Is(err, domain.ErrInvalidControlTransition) {
			t.Fatalf("%s: err = %v, want ErrInvalidControlTransition", name, err)
		}
	}
}

// Covers: 读回门与构造函数同一套形状（ADR-0214 决定三；迁移 0017 的 CHECK 同守）——推进了的转出不带已转出的控制、对不上实物的
// 那几格带了实物、原因与结果搭配不对，都读不回来，不悄悄读成一次合法的转出。
func TestRehydrationRefusesMisshapenRows(t *testing.T) {
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	unit := mustValue(t, domain.NewHandlingUnitID, "unit-1")
	active := activeControl(t, "NODE-INTAKE/v-1", controlAt)
	for name, spec := range map[string]domain.ControlTransitionSpec{
		"转出却不带转出": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionOut,
			Outcome: domain.ControlAdvanced, Unit: unit, Control: active, OccurredAt: handedOverAt},
		"对不上实物却带实物": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionOut,
			Outcome: domain.ControlNotAdvanced, Reason: domain.NoAssociatedUnit, Unit: unit, OccurredAt: handedOverAt},
		"推进了却带原因": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionOut,
			Outcome: domain.ControlAdvanced, Reason: domain.NoActiveControl, Unit: unit, OccurredAt: handedOverAt},
		"没推进却不带原因": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionOut,
			Outcome: domain.ControlNotAdvanced, Unit: unit, OccurredAt: handedOverAt},
		"转入却不是交接转入成立": {TenantID: tenant, Handover: handoverRef(t, "v1"), Direction: domain.ControlTransitionIn,
			Outcome: domain.ControlAdvanced, Unit: unit, Control: active, OccurredAt: controlAt},
	} {
		if _, err := domain.RehydrateControlTransition(spec); !errors.Is(err, domain.ErrInvalidControlTransition) {
			t.Fatalf("%s: err = %v, want ErrInvalidControlTransition", name, err)
		}
	}
}
