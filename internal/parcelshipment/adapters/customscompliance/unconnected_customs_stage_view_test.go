package customscompliance_test

import (
	"testing"

	pscustoms "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/customscompliance"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: 读面未接时三格一律`不知道`且不随包裹变——它不是「没有案件」（那是`不在`，得由关务
// 读面答），答`不在`会让每个包裹都被判成尚未申报、把最早阶段偷偷设成默认值；答复随包裹变化
// 是采信了内容的第一个征兆。
func TestTheUnconnectedCustomsStageViewAnswersUnknownForEveryParcel(t *testing.T) {
	view := pscustoms.UnconnectedCustomsStageView{}
	tenant, err := psdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	for _, raw := range []string{"SYN-PARCEL-01", "SYN-PARCEL-02", "-"} {
		parcel, err := psdomain.NewDeclaredParcelID(raw)
		if err != nil {
			t.Fatalf("parcel %q: %v", raw, err)
		}
		facts, err := view.LoadCustomsStageFacts(t.Context(), tenant, parcel)
		if err != nil {
			t.Fatalf("%s：未接读面不该报错：%v", raw, err)
		}
		if facts.DataForming != psdomain.StageFactUnknown ||
			facts.Submitted != psdomain.StageFactUnknown ||
			facts.CaseClosed != psdomain.StageFactUnknown {
			t.Fatalf("%s：facts = %+v, want 三格都是不知道——读面未接不是没有案件", raw, facts)
		}
		if psdomain.JudgeAmendmentStage(psdomain.AmendmentStageEvidence{
			Accepted:                     psdomain.StageFactPresent,
			Received:                     psdomain.StageFactAbsent,
			LabelledOrBagged:             psdomain.StageFactAbsent,
			CustomsDataForming:           facts.DataForming,
			CustomsSubmitted:             facts.Submitted,
			CaseClosedOrServiceCompleted: facts.CaseClosed,
		}).Determined() {
			t.Fatalf("%s：拿未接读面的答复判出了阶段——三格不知道必须让阶段判不出", raw)
		}
	}
}
