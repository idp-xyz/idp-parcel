package domain

import (
	"errors"
	"testing"
)

// 本文件钉住「商业解析回指」三格里经公开构造函数造不出来的那一格（票 ps-port-remainder/07 完成判据 1
// 「已接受缺回指 → error」）：Decide 的依据快照必带解析标识、重建门拒「已接受缺产物」，所以只有在包内直接
// 摆出那个状态才能证这一格确实是 error 而不是被折成「没有」——折成「没有」会让 TF 把一次接受流的装配缺陷
// 当成「所有者说这个对象没有交付条件」，任务永远停在待形成而原因不可见（ADR-0062 决定三同一判据）。

func TestAnAcceptedRequestWithoutAResolutionIsAReadFaceFailureNotAnAbsence(t *testing.T) {
	parcel, err := NewDeclaredParcelID("parcel-1")
	if err != nil {
		t.Fatalf("parcel: %v", err)
	}
	request := ShipmentRequest{
		state:    ShipmentRequestAccepted,
		baseline: AcceptanceBaseline{declaredParcelIDs: []DeclaredParcelID{parcel}},
	}

	reference, present, err := request.CommercialResolutionReferenceFor(parcel)
	if !errors.Is(err, ErrAcceptedWithoutCommercialResolution) {
		t.Fatalf("err = %v, want ErrAcceptedWithoutCommercialResolution", err)
	}
	if present || reference.valid() {
		t.Fatalf("the failure still handed back present = %v reference = %q", present, reference)
	}
}
