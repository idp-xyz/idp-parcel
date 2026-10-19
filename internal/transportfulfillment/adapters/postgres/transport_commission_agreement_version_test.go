package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证委托册记下登记方指名的那一版协议（迁移 TF 0038，票 demo-chain-gaps/28）：两格随提交落库、随读回重建，开始与取消
// 的写口不碰它们；迁移之前落的行两格为空，读回如实说没记下；库面只认两格同在或同缺。证据只记 S。

// Covers: AT-TF-213 的库面一半——提交记下的协议对象与版本往返；开始之后仍在；两格为空的旧行读回不带版本。
func TestACommissionKeepsTheAgreementVersionItWasSubmittedUnder(t *testing.T) {
	commissions, _, _, transactor, pool := newCommissionStores(t)
	ctx := t.Context()

	record := submittedCommission(t, "tenant-a", "commission-versioned")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := commissions.Save(txCtx, record)
		return err
	})
	found, exists, err := commissions.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回：err=%v exists=%v", err, exists)
	}
	version, recorded := found.Commission.AgreementVersion()
	if !recorded || version.Object() != "agreement-1" || version.Version() != "v1" {
		t.Fatalf("读回的协议版本 = %v %v，want agreement-1 v1", version, recorded)
	}

	started, err := found.Commission.MarkTransportStarted(
		deliveryValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/ho-1"), commissionAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	replaced := found
	replaced.Commission = started
	var replacedOK bool
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var replaceErr error
		replacedOK, replaceErr = commissions.Replace(txCtx, replaced)
		return replaceErr
	})
	if !replacedOK {
		t.Fatal("开始 Replace 答 false")
	}
	after, _, err := commissions.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatal(err)
	}
	if version, recorded := after.Commission.AgreementVersion(); !recorded || version.Version() != "v1" {
		t.Fatalf("开始之后协议版本 = %v %v", version, recorded)
	}

	// 迁移 0038 之前落的行：两格为空。读回不是坏行，只是没记下。
	legacy := submittedCommission(t, "tenant-a", "commission-legacy")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := commissions.Save(txCtx, legacy)
		return err
	})
	if _, err := pool.Exec(ctx, `UPDATE transport_fulfillment.transport_commission
	                                SET agreement_object = NULL, agreement_version = NULL
	                              WHERE tenant_id = 'tenant-a' AND commission_id = 'commission-legacy'`); err != nil {
		t.Fatalf("模拟旧行：%v", err)
	}
	old, exists, err := commissions.FindByKey(ctx, ports.TransportCommissionKey{TenantID: legacy.Key.TenantID, Commission: legacy.Key.Commission})
	if err != nil || !exists {
		t.Fatalf("旧行读回：err=%v exists=%v", err, exists)
	}
	if _, recorded := old.Commission.AgreementVersion(); recorded {
		t.Fatal("两格为空的旧行读出了协议版本")
	}
	if old.Commission.Agreement().String() != "agreement-snapshot-1" {
		t.Fatalf("旧行快照 = %q", old.Commission.Agreement())
	}
}

// Covers: 库面两格同在或同缺：只留一格、或写成空白，CHECK 拒。
func TestTheCommissionAgreementVersionColumnsArePaired(t *testing.T) {
	commissions, _, _, transactor, pool := newCommissionStores(t)
	ctx := t.Context()
	record := submittedCommission(t, "tenant-a", "commission-paired")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := commissions.Save(txCtx, record)
		return err
	})
	for name, statement := range map[string]string{
		"只留对象": `UPDATE transport_fulfillment.transport_commission SET agreement_version = NULL
		              WHERE tenant_id = 'tenant-a' AND commission_id = 'commission-paired'`,
		"版本空白": `UPDATE transport_fulfillment.transport_commission SET agreement_version = ' '
		              WHERE tenant_id = 'tenant-a' AND commission_id = 'commission-paired'`,
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("%s：CHECK 应拒", name)
		}
	}
}
