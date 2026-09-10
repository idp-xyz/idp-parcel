package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉住「商业解析回指」按包裹身份答出的三格里进程内能构造的两格（票 ps-port-remainder/07；
// ADR-0133 决定二）：`回指`与`没有`。第三格（已接受而接受决定缺回指 → error）经公开构造函数造不出来
// ——Decide 的依据快照必带解析标识、重建门拒已接受缺产物——它的用例在同包内部测试里。

func resolveCommercialReference(t *testing.T, request domain.ShipmentRequest, parcel string) (domain.CommercialResolutionID, bool) {
	t.Helper()
	reference, present, err := request.CommercialResolutionReferenceFor(mustValue(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("commercial resolution reference for %q: %v", parcel, err)
	}
	return reference, present
}

// Covers: ADR-0133 决定一「交付条件引用是委托接受时固定的商业解析回指（接受决定上的 CommercialResolutionID）」
// 与决定二「按（租户，包裹身份）答」——回指就是那个标识，不拆不拼；同一委托的两个成员拿到同一个回指。
func TestAMemberOfAnAcceptedRequestGetsTheAcceptanceTimeCommercialResolution(t *testing.T) {
	request := acceptedWithVersions(t)

	first, present := resolveCommercialReference(t, request, "parcel-1")
	if !present {
		t.Fatal("an accepted member got no commercial resolution reference")
	}
	if first.String() != "RES-1" {
		t.Fatalf("reference = %q, want RES-1 (the acceptance decision's resolution)", first)
	}
	decision, _ := request.AcceptanceDecision()
	if first != decision.Basis().ResolutionID() {
		t.Fatalf("reference %q is not the acceptance decision's resolution %q", first, decision.Basis().ResolutionID())
	}

	second, _ := resolveCommercialReference(t, request, "parcel-2")
	if second != first {
		t.Fatalf("two members of one request got different references: %q vs %q", first, second)
	}
}

// Covers: ADR-0133 决定二「对象不属于任何已接受委托的成员集合……按统一不可见结果答没有」；未接受的委托
// 没有基线也没有接受时固定的回指，其成员同样答没有——决定前的依据是候选，不是固定下来的引用。
func TestAnObjectOutsideTheAcceptanceBaselineHasNoCommercialResolution(t *testing.T) {
	accepted := acceptedWithVersions(t)
	if reference, present := resolveCommercialReference(t, accepted, "parcel-9"); present || reference.String() != "" {
		t.Fatalf("a non-member got a reference %q", reference)
	}

	if reference, present := resolveCommercialReference(t, submitted(t), "parcel-1"); present || reference.String() != "" {
		t.Fatalf("a member of a merely submitted request got a reference %q", reference)
	}
}

// Covers: 资料修订不动回指——回指是接受时固定的，客户原始资料版本再多也不换合同（ADR-0133 决定一「接受时固定」）。
func TestSourceDataAmendmentsDoNotMoveTheCommercialResolution(t *testing.T) {
	amended := acceptedWithVersions(t, [2]string{"data-version-1", ""}, [2]string{"data-version-2", "data-version-1"})
	reference, present := resolveCommercialReference(t, amended, "parcel-1")
	if !present || reference.String() != "RES-1" {
		t.Fatalf("reference after amendments = %q present = %v, want RES-1", reference, present)
	}
}
