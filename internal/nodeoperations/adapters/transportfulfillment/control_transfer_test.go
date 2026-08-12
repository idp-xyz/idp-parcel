package transportfulfillment_test

import (
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/transportfulfillment"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	controlledAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)
	handedOverAt = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
)

func value[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// heldControl 真经 NO 领域成立一份在身控制。
func heldControl(t *testing.T) nodomain.PhysicalControl {
	t.Helper()
	control, err := nodomain.EstablishPhysicalControl(nodomain.PhysicalControlSpec{
		TenantID:      value(t, nodomain.NewTenantID, "tenant-1"),
		Unit:          value(t, nodomain.NewHandlingUnitID, "unit-1"),
		Node:          value(t, nodomain.NewNodeReference, "node-origin"),
		Kind:          nodomain.EstablishedByNodeIntake,
		Basis:         value(t, nodomain.NewControlBasisReference, "NODE-INTAKE/intake-result/v1"),
		EstablishedAt: controlledAt,
	})
	if err != nil {
		t.Fatalf("establish physical control: %v", err)
	}
	return control
}

// handover 真经 TF 领域形成一份权威交接结果。
func handover(t *testing.T, verdict tfdomain.HandoverVerdict, object string) tfdomain.TransportHandover {
	t.Helper()
	spec := tfdomain.TransportHandoverSpec{
		TenantID:   value(t, tfdomain.NewTenantID, "tenant-1"),
		Object:     value(t, tfdomain.NewCarriedObjectReference, object),
		Scope:      value(t, tfdomain.NewHandoverScopeReference, "handover-scope-1"),
		ReleasedBy: value(t, tfdomain.NewHandoverPartyReference, "node-origin"),
		ReceivedBy: value(t, tfdomain.NewHandoverPartyReference, "carrier-1"),
		Verdict:    verdict,
		Version:    value(t, tfdomain.NewHandoverResultVersion, "handover-result/v1"),
		JudgedAt:   handedOverAt,
	}
	if verdict == tfdomain.ObjectHandedOver {
		spec.ReleasingEvidence = value(t, tfdomain.NewHandoverEvidenceReference, "LOAD-SCAN/7")
		spec.ReceivingEvidence = value(t, tfdomain.NewHandoverEvidenceReference, "CARRIER-ACK/9")
		spec.Rule = value(t, tfdomain.NewHandoverRuleReference, "handover-rules/v1")
	} else {
		spec.Basis = value(t, tfdomain.NewHandoverBasisReference, "SEAL_MISMATCH")
	}
	built, err := tfdomain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("form transport handover (%s): %v", verdict, err)
	}
	return built
}

// Covers: NO CONTEXT「节点控制 → 控制转出：transport-fulfillment 针对明确对象形成
// 权威运输交接的『已交接』结果」经适配器端到端——真 TF 已交接结果转出真 NO 控制，
// 转出依据带交接版本、时刻取交接判断的业务时间。
func TestAHandedOverResultTransfersTheNodeControl(t *testing.T) {
	subject := adapter.NewControlTransferAdapter()

	transferred, err := subject.TransferOut(heldControl(t), handover(t, tfdomain.ObjectHandedOver, "unit-1"))
	if err != nil {
		t.Fatalf("transfer out: %v", err)
	}

	if transferred.Active() {
		t.Fatal("已交接后控制还在身")
	}
	basis, at, released := transferred.Release()
	if !released || basis.String() != "TRANSPORT-HANDOVER/handover-result/v1" {
		t.Fatalf("basis = %q released = %v; 转出依据必须带交接版本", basis, released)
	}
	if !at.Equal(handedOverAt) {
		t.Fatalf("released at = %s, want the handover judgment time", at)
	}
}

// Covers: TF CONTEXT「已拒收或待确认不转出控制」与 NO 侧「转出唯一依据是已交接结果」
// 的两边对上——拒收与待确认独立哨兵拒绝（不是适配器故障）；交接对象与控制对象不是
// 同一件实物时拒绝（拿别的对象的交接转出这件的控制，部分交接的逐对象纪律就破了）。
func TestRefusalsPendingAndForeignObjectsDoNotTransfer(t *testing.T) {
	subject := adapter.NewControlTransferAdapter()

	for name, verdict := range map[string]tfdomain.HandoverVerdict{
		"refused":              tfdomain.HandoverRefused,
		"pending confirmation": tfdomain.HandoverPendingConfirmation,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := subject.TransferOut(heldControl(t), handover(t, verdict, "unit-1")); !errors.Is(err, adapter.ErrHandoverDoesNotTransfer) {
				t.Fatalf("err = %v, want ErrHandoverDoesNotTransfer", err)
			}
		})
	}

	if _, err := subject.TransferOut(heldControl(t), handover(t, tfdomain.ObjectHandedOver, "unit-9")); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
	}
}
