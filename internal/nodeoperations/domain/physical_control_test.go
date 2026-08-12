package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

var controlAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func controlSpec(t *testing.T, kind domain.ControlEstablishmentKind, unit string) domain.PhysicalControlSpec {
	t.Helper()
	return domain.PhysicalControlSpec{
		TenantID:      mustValue(t, domain.NewTenantID, "tenant-1"),
		Unit:          mustValue(t, domain.NewHandlingUnitID, unit),
		Node:          mustValue(t, domain.NewNodeReference, "node-origin"),
		Kind:          kind,
		Basis:         mustValue(t, domain.NewControlBasisReference, "intake-result/v1"),
		EstablishedAt: controlAt,
	}
}

// Covers: CONTEXT「节点控制」节「尚未控制 → 节点控制成立：客户直接送站形成有效节点
// 收寄，或 transport-fulfillment 的权威交接结果确认控制已经转入」——两来源同语义成立，
// 封闭二值外（扫描、卸载、发现实物）没有格可落；依据与时刻必备。
func TestControlEstablishesFromExactlyTwoSources(t *testing.T) {
	for _, kind := range []domain.ControlEstablishmentKind{
		domain.EstablishedByNodeIntake,
		domain.EstablishedByHandoverIn,
	} {
		control, err := domain.EstablishPhysicalControl(controlSpec(t, kind, "unit-1"))
		if err != nil {
			t.Fatalf("establish (%s): %v", kind, err)
		}
		if !control.Active() {
			t.Fatalf("kind %s: 新成立的控制不在身", kind)
		}
		if !control.EstablishedAt().Equal(controlAt) {
			t.Fatalf("established at = %s", control.EstablishedAt())
		}
	}

	if _, err := domain.EstablishPhysicalControl(
		controlSpec(t, domain.ControlEstablishmentKindInvalid, "unit-1")); !errors.Is(err, domain.ErrInvalidPhysicalControl) {
		t.Fatalf("err = %v; 封闭二值之外立起了控制", err)
	}
	missingBasis := controlSpec(t, domain.EstablishedByNodeIntake, "unit-1")
	missingBasis.Basis = domain.ControlBasisReference{}
	if _, err := domain.EstablishPhysicalControl(missingBasis); !errors.Is(err, domain.ErrInvalidPhysicalControl) {
		t.Fatalf("err = %v; 没有依据的控制立起来了", err)
	}
}

// Covers: CONTEXT「节点控制 → 控制转出：transport-fulfillment 针对明确对象形成权威
// 运输交接的『已交接』结果」——转出仅凭已交接依据、不可逆、时刻不得早于成立；已拒收/
// 待确认/备货装载扫描在 TF 的权威交接对象上就不是`已交接`，从源头构造不出转出引用
// （类型面防线，此处钉转出自身的三条规则）。
func TestTransferOutIsHandoverOnlyAndIrreversible(t *testing.T) {
	control, err := domain.EstablishPhysicalControl(controlSpec(t, domain.EstablishedByNodeIntake, "unit-1"))
	if err != nil {
		t.Fatalf("establish: %v", err)
	}

	transferred, err := control.TransferOut(
		mustValue(t, domain.NewTransferOutReference, "HANDOVER/TF-9"),
		controlAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("transfer out: %v", err)
	}
	if transferred.Active() {
		t.Fatal("已转出的控制还在身")
	}
	handover, at, released := transferred.Release()
	if !released || handover.String() != "HANDOVER/TF-9" || !at.Equal(controlAt.Add(2*time.Hour)) {
		t.Fatalf("release = %v %s %v", handover, at, released)
	}
	if !control.Active() {
		t.Fatal("原控制记录被改写了")
	}

	if _, err := transferred.TransferOut(
		mustValue(t, domain.NewTransferOutReference, "HANDOVER/TF-10"),
		controlAt.Add(3*time.Hour),
	); !errors.Is(err, domain.ErrInvalidPhysicalControl) {
		t.Fatalf("err = %v; 转出转了两次", err)
	}
	if _, err := control.TransferOut(
		mustValue(t, domain.NewTransferOutReference, "HANDOVER/TF-9"),
		controlAt.Add(-time.Minute),
	); !errors.Is(err, domain.ErrInvalidPhysicalControl) {
		t.Fatalf("err = %v; 转出时刻早于成立", err)
	}
	if _, err := control.TransferOut(domain.TransferOutReference{}, controlAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidPhysicalControl) {
		t.Fatalf("err = %v; 没有依据的转出被收下了", err)
	}
}

// Covers: CONTEXT「部分交接时，各对象分别结束或保留节点控制；批次、集运单元或车辆
// 范围不形成一刀切的控制结果」——控制逐对象持有（类型上没有批次维度），同车两件各自
// 转出其一，另一件的控制纹丝不动。
func TestPartialHandoverEndsControlPerObject(t *testing.T) {
	first, err := domain.EstablishPhysicalControl(controlSpec(t, domain.EstablishedByNodeIntake, "unit-1"))
	if err != nil {
		t.Fatalf("establish unit-1: %v", err)
	}
	second, err := domain.EstablishPhysicalControl(controlSpec(t, domain.EstablishedByNodeIntake, "unit-2"))
	if err != nil {
		t.Fatalf("establish unit-2: %v", err)
	}

	transferred, err := first.TransferOut(
		mustValue(t, domain.NewTransferOutReference, "HANDOVER/TF-9"),
		controlAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("transfer out unit-1: %v", err)
	}

	if transferred.Active() {
		t.Fatal("已交接对象的控制还在身")
	}
	if !second.Active() {
		t.Fatal("同车另一件的控制被一刀切转出了")
	}
}
