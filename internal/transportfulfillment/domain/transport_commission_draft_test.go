package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: CONTEXT 生命周期「运输委托草稿 → 已提交：固定请求方、服务提供方、运输范围、条件和供应商协议依据」与 ADR-0202
// 决定三「采购责任法人从协议快照，不由调用方给」——草稿只收登记方声明的一半、指名待核的协议版本，还没有快照与法人；
// 提交时两格从外面交进来，其余各格原样取自草稿；同一份草稿交两次各得一份，草稿本身不变。缺快照、缺法人与声明各格的
// 拒收见 TestACommissionSnapshotsItsCommercialBasis，提交时刻判零见 TestCommissionAndBookingGatesJudgeZeroOnTheRegistry。
func TestADraftOnlyBecomesACommissionWithACheckedAgreementAndLegalEntity(t *testing.T) {
	draft, err := domain.DraftTransportCommission(commissionSpec(t))
	if err != nil {
		t.Fatalf("草稿：%v", err)
	}
	if draft.Agreement().Object() != "agreement-1" || draft.Agreement().Version() != "v1" ||
		draft.Provider().String() != "partner-1" || draft.Commission().String() != "commission-1" ||
		draft.TenantID().String() != "tenant-1" || !draft.SubmittedAt().Equal(commissionSubmittedAt) {
		t.Fatalf("草稿没有原样留下登记方的声明：%+v", draft)
	}

	snapshot := mustValue(t, domain.NewAgreementSnapshotReference, "agreement-1/v1")
	legalEntity := mustValue(t, domain.NewProcurementLegalEntityReference, "SYN-LE-01")
	commission, err := draft.Submit(snapshot, legalEntity)
	if err != nil {
		t.Fatalf("提交：%v", err)
	}
	if commission.Agreement() != snapshot || commission.LegalEntity() != legalEntity ||
		commission.Provider().String() != "partner-1" || commission.Conditions().String() != "conditions-snapshot-1" ||
		len(commission.Members()) != 1 || !commission.SubmittedAt().Equal(commissionSubmittedAt) {
		t.Fatalf("提交后的委托没有带上核过的快照与法人和草稿的声明：%+v", commission)
	}

	other := mustValue(t, domain.NewProcurementLegalEntityReference, "SYN-LE-02")
	again, err := draft.Submit(snapshot, other)
	if err != nil {
		t.Fatalf("再交：%v", err)
	}
	if again.LegalEntity() != other || commission.LegalEntity() != legalEntity {
		t.Fatal("同一份草稿再交改动了上一份委托")
	}
}
