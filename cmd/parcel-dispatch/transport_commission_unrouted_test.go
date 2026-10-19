package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

type commissionHandoffClock struct{}

func (commissionHandoffClock) Now() time.Time { return beatInstant() }

// Covers: 票 demo-chain-gaps/23 完成判据「路由不登，`no_subscriber` 如实可见」与 ADR-0202 决定六——生产的委托意图适配器
// 入队的「委托已提交」信封，派发一拍不发布、记 `dispatch.no_subscriber`：本进程照 ADR-0066 没给这一类型登消费者，失败
// 如实入账，不被别的路由误吃。哪天登了消费者，这条用例要随之改写，那正是它要提醒的事。
func TestTheTransportCommissionSubmittedEnvelopeHasNoSubscriber(t *testing.T) {
	beat, db, store := wiredBeat(t)
	handoff, err := tfpostgres.NewOutboxTransportCommissionHandoff(db, store, commissionHandoffClock{})
	if err != nil {
		t.Fatalf("构造委托意图适配器：%v", err)
	}

	tenant, _ := tfdomain.NewTenantID("tenant-a")
	commissionRef, _ := tfdomain.NewTransportCommissionReference("SYN-COMMISSION-DCG23")
	provider, _ := tfdomain.NewServiceProviderReference("SYN-PARTY-CARRIER-TRUNK-01")
	agreement, _ := tfdomain.NewSupplierAgreementVersion("SYN-SUPPLIER-TRUNK-CN-SG-01", "v1")
	conditions, _ := tfdomain.NewConditionsSnapshotReference("SYN-CONDITIONS-01")
	role, _ := tfdomain.NewRoleSnapshotReference("SYN-ROLE-01")
	responsibility, _ := tfdomain.NewResponsibilitySnapshotReference("SYN-RESPONSIBILITY-01")
	member, _ := tfdomain.NewCarriedObjectReference("SYN-PARCEL-DCG23")
	draft, err := tfdomain.DraftTransportCommission(tfdomain.TransportCommissionDraftSpec{
		TenantID: tenant, Commission: commissionRef, Provider: provider, Agreement: agreement,
		Conditions: conditions, Role: role, Responsibility: responsibility,
		Members: []tfdomain.CarriedObjectReference{member}, SubmittedAt: beatInstant(),
	})
	if err != nil {
		t.Fatalf("委托草稿：%v", err)
	}
	snapshot, _ := tfdomain.NewAgreementSnapshotReference("SYN-SUPPLIER-TRUNK-CN-SG-01/v1")
	legalEntity, _ := tfdomain.NewProcurementLegalEntityReference("SYN-LE-01")
	commission, err := draft.Submit(snapshot, legalEntity)
	if err != nil {
		t.Fatalf("委托：%v", err)
	}
	record := tfports.TransportCommissionRecord{
		Key:           tfports.TransportCommissionKey{TenantID: tenant, Commission: commissionRef},
		ContentDigest: "TFC-1:synthetic",
		Commission:    commission,
		RecordedAt:    beatInstant(),
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffTransportCommission(txCtx, tfports.TransportCommissionIntent{Record: record})
	}); err != nil {
		t.Fatalf("入队委托意图：%v", err)
	}

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("委托信封被当成发布成功定稿了：published = %d", published)
	}
	eventID := string(outboxintent.FingerprintEventID("transport-commission", "tenant-a", "SYN-COMMISSION-DCG23"))
	if got := recordedFailureCode(t, db, eventID); got != "dispatch.no_subscriber" {
		t.Fatalf("failure_code = %q, want dispatch.no_subscriber", got)
	}
}
