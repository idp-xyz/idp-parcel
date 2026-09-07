package nodeoperations_test

import (
	"testing"

	psnode "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: 读面未接时装袋事实一律`不知道`且不随包裹变；与本上下文已知`不在`的面单结果并成
// 「已制签或已装袋」后整格仍`不知道`——已知的那半是「不在」替不了另一半作答。
func TestTheUnconnectedConsolidationStageViewAnswersUnknownForEveryParcel(t *testing.T) {
	view := psnode.UnconnectedConsolidationStageView{}
	tenant, err := psdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	for _, raw := range []string{"SYN-PARCEL-01", "SYN-PARCEL-02", "-"} {
		parcel, err := psdomain.NewDeclaredParcelID(raw)
		if err != nil {
			t.Fatalf("parcel %q: %v", raw, err)
		}
		bagged, err := view.LoadBaggingFact(t.Context(), tenant, parcel)
		if err != nil {
			t.Fatalf("%s：未接读面不该报错：%v", raw, err)
		}
		if bagged != psdomain.StageFactUnknown {
			t.Fatalf("%s：bagged = %v, want 不知道——读面未接不是没装袋", raw, bagged)
		}
		if got := psdomain.EitherStageFact(psdomain.StageFactAbsent, bagged); got != psdomain.StageFactUnknown {
			t.Fatalf("%s：面单不在 + 装袋不知道 并成 %v, want 不知道", raw, got)
		}
	}
}
