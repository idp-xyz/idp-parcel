package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证委托协议换版的库面（迁移 TF 0047，ADR-0231）：换版随委托按键读回，号与依据在同一份委托内各自唯一、撞上答已登记不覆盖，
// 换版锁不在事务里取被拒。证据只记 S。

func revisionOf(t *testing.T, commission domain.TransportCommission, version, basis string, appliesFrom time.Time) (domain.TransportCommission, domain.CommissionAgreementRevision) {
	t.Helper()
	successor, err := domain.NewSupplierAgreementVersion("agreement-1", version)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := commission.DraftAgreementRevision(domain.CommissionAgreementRevisionSpec{
		Successor:   successor,
		AppliesFrom: appliesFrom,
		Basis:       deliveryValue(t, domain.NewCommissionAgreementRevisionBasisReference, basis),
		DecidedBy:   deliveryValue(t, domain.NewRegistrantReference, "https://id.syn.example#SYN-OPERATOR-DCG88"),
	})
	if err != nil {
		t.Fatalf("换版草稿：%v", err)
	}
	revised, revision, err := draft.Revise(
		deliveryValue(t, domain.NewAgreementSnapshotReference, "agreement-1/"+version),
		deliveryValue(t, domain.NewProcurementLegalEntityReference, "legal-entity-"+version),
		commissionAt.Add(72*time.Hour),
	)
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	return revised, revision
}

// Covers: AT-TF-298 的库面一半（S，ADR-0231 决定三）——换版落行后随委托读回，事实按业务时间取那一版；同一份委托再落同号或同依据的
// 一行答已登记、原行不动；他租户同名委托上看不到这一行。
func TestACommissionReadsBackItsAgreementRevisions(t *testing.T) {
	commissions, _, _, transactor, _ := newCommissionStores(t)
	ctx := t.Context()
	record := submittedCommission(t, "tenant-a", "commission-revised")
	other := submittedCommission(t, "tenant-b", "commission-revised")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := commissions.Save(txCtx, record); err != nil {
			return err
		}
		_, err := commissions.Save(txCtx, other)
		return err
	})

	cut := commissionAt.Add(24 * time.Hour)
	revised, revision := revisionOf(t, record.Commission, "v2", "SYN-DECISION-REV-DCG88-1", cut)
	var saved ports.CommissionAgreementRevisionSaveOutcome
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := commissions.HoldRevisions(txCtx, record.Key); err != nil {
			return err
		}
		var err error
		saved, err = commissions.SaveRevision(txCtx, ports.CommissionAgreementRevisionRecord{Key: record.Key, Revision: revision})
		return err
	})
	if saved != ports.CommissionAgreementRevisionSaved {
		t.Fatalf("落换版 = %d", saved)
	}

	found, exists, err := commissions.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回：%v %v", err, exists)
	}
	revisions := found.Commission.AgreementRevisions()
	if len(revisions) != 1 || revisions[0].Number() != 2 || revisions[0].Successor().Version() != "v2" ||
		!revisions[0].AppliesFrom().Equal(cut) || revisions[0].Snapshot().String() != "agreement-1/v2" ||
		revisions[0].DecidedBy().String() != "https://id.syn.example#SYN-OPERATOR-DCG88" {
		t.Fatalf("读回的换版 = %+v", revisions)
	}
	if terms := found.Commission.AgreementTermsAt(cut.Add(-time.Second)); terms.Revision() != 1 {
		t.Fatalf("起用之前取第 %d 版", terms.Revision())
	}
	if terms := found.Commission.AgreementTermsAt(cut); terms.Revision() != 2 || terms.LegalEntity().String() != "legal-entity-v2" {
		t.Fatalf("起用那一刻 = %+v", terms)
	}
	if revised.AgreementTermsAt(cut).Revision() != found.Commission.AgreementTermsAt(cut).Revision() {
		t.Fatal("读回与落下之前的委托答的不是同一版")
	}
	if elsewhere, _, err := commissions.FindByKey(ctx, other.Key); err != nil || len(elsewhere.Commission.AgreementRevisions()) != 0 {
		t.Fatalf("他租户同名委托读到了换版：%v", err)
	}

	_, sameNumber := revisionOf(t, record.Commission, "v3", "SYN-DECISION-REV-DCG88-2", cut)
	_, sameBasis := revisionOf(t, revised, "v3", "SYN-DECISION-REV-DCG88-1", cut)
	for name, duplicate := range map[string]domain.CommissionAgreementRevision{"同号": sameNumber, "同依据": sameBasis} {
		mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			saved, err = commissions.SaveRevision(txCtx, ports.CommissionAgreementRevisionRecord{Key: record.Key, Revision: duplicate})
			return err
		})
		if saved != ports.CommissionAgreementRevisionAlreadyRegistered {
			t.Fatalf("%s再落 = %d, want 已登记", name, saved)
		}
	}
	again, _, err := commissions.FindByKey(ctx, record.Key)
	if err != nil || len(again.Commission.AgreementRevisions()) != 1 || again.Commission.AgreementRevisions()[0].Successor().Version() != "v2" {
		t.Fatalf("撞上之后原行被改：%v %+v", err, again.Commission.AgreementRevisions())
	}
}

// TestTheRevisionWritesRefuseToRunOutsideATransaction 证换版的两个写口无环境事务即拒：换版锁要与读委托、落行同一笔才守得住铸号，
// 落行要与门铃同一笔，事务外跑就是一次没有门铃的换版。
func TestTheRevisionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	commissions, _, _, _, _ := newCommissionStores(t)
	record := submittedCommission(t, "tenant-a", "commission-outside")
	_, revision := revisionOf(t, record.Commission, "v2", "SYN-DECISION-REV-DCG88-OUT", commissionAt)
	for name, write := range map[string]func() error{
		"HoldRevisions": func() error { return commissions.HoldRevisions(t.Context(), record.Key) },
		"SaveRevision": func() error {
			_, err := commissions.SaveRevision(t.Context(), ports.CommissionAgreementRevisionRecord{Key: record.Key, Revision: revision})
			return err
		},
	} {
		if err := write(); !errors.Is(err, bentopg.ErrTransactionRequired) {
			t.Errorf("%s 无事务应返回 ErrTransactionRequired，实得：%v", name, err)
		}
	}
}
