package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件是真库用例的送站预报夹具（ADR-0212 决定二；票 ps-external-mark-relations/11）：经 NO 的预报册与时窗册直接落一份客户账户的
// 预报与一行时窗，交回收寄要带的依据引用。客户包裹件号的送站此后都要有可验证交付范围才唯一识别（决定四、决定十），凡用例的本意是
// 「这件能被识别」，就带上它。值全为合成（S 级）：租户、节点、业务时刻与 receptionCommand 同一组。

var preAdviceSequence atomic.Int64

// preAdviseOnRealDatabase 给 customer 在 SYN-NODE-ORIGIN（receptionCommand 的节点）登一份预报，见 preAdviseAtNodeOnRealDatabase。
func preAdviseOnRealDatabase(t *testing.T, db *bentopg.DB, customer string, numbers ...string) nodomain.DeliveryScopeBasisReference {
	t.Helper()
	return preAdviseAtNodeOnRealDatabase(t, db, "SYN-NODE-ORIGIN", customer, numbers...)
}

// preAdviseAtNodeOnRealDatabase 给 customer 在 node 登一份预计送达时刻就是 receptionDeliveredAt、列着 numbers 的预报，并确保本租户有
// 一行生效的时窗（提前一天、宽限半天，与演示种子同量级）。时窗已在册时原样再登答已在册，不算失败。
func preAdviseAtNodeOnRealDatabase(t *testing.T, db *bentopg.DB, node, customer string, numbers ...string) nodomain.DeliveryScopeBasisReference {
	t.Helper()
	tenant := mustValue(t, nodomain.NewTenantID, "SYN-TENANT-1")
	preAdvices, err := nopostgres.NewDeliveryPreAdvices(db)
	if err != nil {
		t.Fatalf("构造预报册：%v", err)
	}
	windows, err := nopostgres.NewPreAdviceWindows(db)
	if err != nil {
		t.Fatalf("构造时窗册：%v", err)
	}
	window, err := nodomain.FormPreAdviceValidityWindow(nodomain.PreAdviceValidityWindowSpec{
		TenantID:      tenant,
		Lead:          24 * time.Hour,
		Grace:         12 * time.Hour,
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Basis:         "SYN-WINDOW-BASIS-PSXM11",
		RegisteredAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("时窗：%v", err)
	}
	sequence := preAdviceSequence.Add(1)
	id := fmt.Sprintf("SYN-PREADV-PSXM11-%04d", sequence)
	listed := make([]nodomain.PreAdvisedParcelNumber, 0, len(numbers))
	for _, number := range numbers {
		listed = append(listed, mustValue(t, nodomain.NewPreAdvisedParcelNumber, number))
	}
	preAdvice, err := nodomain.FormDeliveryPreAdvice(nodomain.DeliveryPreAdviceSpec{
		TenantID:          tenant,
		ID:                mustValue(t, nodomain.NewDeliveryPreAdviceID, id),
		CustomerAccount:   mustValue(t, nodomain.NewCustomerAccountReference, customer),
		Node:              mustValue(t, nodomain.NewNodeReference, node),
		ExpectedArrivalAt: receptionDeliveredAt,
		Numbers:           listed,
		Source: nodomain.CustomerRequestSource{
			Channel:       "SYN-CUSTOMER-CLIENT-PSXM11",
			RequestKey:    fmt.Sprintf("SYN-ACCESS-REQUEST-PSXM11-%04d", sequence),
			ContentDigest: "NOC-1:syn-" + id,
		},
		FormedAt: receptionDeliveredAt.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("预报：%v", err)
	}
	var windowOutcome, preAdviceOutcome noports.DeliveryScopeAppendOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		if _, windowOutcome, err = windows.RegisterPreAdviceWindow(txCtx, window); err != nil {
			return err
		}
		_, preAdviceOutcome, err = preAdvices.RecordDeliveryPreAdvice(txCtx, preAdvice)
		return err
	}); err != nil {
		t.Fatalf("登预报与时窗：%v", err)
	}
	if preAdviceOutcome != noports.DeliveryScopeAppended || windowOutcome == noports.DeliveryScopeAppendInvalid {
		t.Fatalf("预报 %s、时窗 %s, want 预报新落", preAdviceOutcome, windowOutcome)
	}
	basis, err := nodomain.NewDeliveryScopeBasisReference(nodomain.BasisDeliveryPreAdvice, id)
	if err != nil {
		t.Fatalf("依据引用：%v", err)
	}
	return basis
}

// withdrawPreAdviceOnRealDatabase 由预报的客户账户撤回那一份。
func withdrawPreAdviceOnRealDatabase(t *testing.T, db *bentopg.DB, basis nodomain.DeliveryScopeBasisReference, customer string) {
	t.Helper()
	preAdvices, err := nopostgres.NewDeliveryPreAdvices(db)
	if err != nil {
		t.Fatalf("构造预报册：%v", err)
	}
	withdrawal, err := nodomain.FormDeliveryPreAdviceWithdrawal(nodomain.DeliveryPreAdviceWithdrawalSpec{
		TenantID:        mustValue(t, nodomain.NewTenantID, "SYN-TENANT-1"),
		PreAdvice:       mustValue(t, nodomain.NewDeliveryPreAdviceID, basis.Identifier()),
		CustomerAccount: mustValue(t, nodomain.NewCustomerAccountReference, customer),
		Source: nodomain.CustomerRequestSource{
			Channel:       "SYN-CUSTOMER-CLIENT-PSXM11",
			RequestKey:    "SYN-ACCESS-REQUEST-PSXM11-WITHDRAW-" + basis.Identifier(),
			ContentDigest: "NOC-1:syn-withdraw-" + basis.Identifier(),
		},
		WithdrawnAt: receptionDeliveredAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("撤回：%v", err)
	}
	var outcome noports.DeliveryScopeAppendOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		_, outcome, err = preAdvices.RecordDeliveryPreAdviceWithdrawal(txCtx, withdrawal)
		return err
	}); err != nil || outcome != noports.DeliveryScopeAppended {
		t.Fatalf("撤回预报：outcome %s err %v", outcome, err)
	}
}

// confirmedHandoverNoteOnRealDatabase 在 SYN-NODE-ORIGIN 开立一张声称 customer 的送站交接单，并由那个客户账户经客户渠道确认绑定，交回
// 收寄要带的依据引用。交接单成立的范围是账户级、不看号码（ADR-0212 决定三）；开立、确认的命令口归票 12，这里经交接单册直接落。
func confirmedHandoverNoteOnRealDatabase(t *testing.T, db *bentopg.DB, customer string) nodomain.DeliveryScopeBasisReference {
	t.Helper()
	tenant := mustValue(t, nodomain.NewTenantID, "SYN-TENANT-1")
	notes, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatalf("构造交接单册：%v", err)
	}
	id := fmt.Sprintf("SYN-HANDOVER-PSXM11-%04d", preAdviceSequence.Add(1))
	account := mustValue(t, nodomain.NewCustomerAccountReference, customer)
	note, err := nodomain.OpenDeliveryHandoverNote(nodomain.DeliveryHandoverNoteSpec{
		TenantID:        tenant,
		ID:              mustValue(t, nodomain.NewDeliveryHandoverNoteID, id),
		Node:            mustValue(t, nodomain.NewNodeReference, "SYN-NODE-ORIGIN"),
		DeliveringParty: mustValue(t, nodomain.NewDeliveringPartyReference, "SYN-DRIVER-PSXM11"),
		ClaimedAccounts: []nodomain.CustomerAccountReference{account},
		OpenedAt:        receptionDeliveredAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("交接单：%v", err)
	}
	binding, err := nodomain.FormDeliveryHandoverBinding(nodomain.DeliveryHandoverBindingSpec{
		TenantID:    tenant,
		Note:        note.ID(),
		Account:     account,
		Conclusion:  nodomain.BindingConfirmed,
		Route:       nodomain.ConcludedViaCustomerChannel,
		Concluder:   "SYN-CUSTOMER-CLIENT-PSXM11",
		ConcludedAt: receptionDeliveredAt.Add(-30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("绑定结论：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, _, err := notes.OpenDeliveryHandoverNote(txCtx, note); err != nil {
			return err
		}
		_, _, err := notes.RecordDeliveryHandoverBinding(txCtx, binding)
		return err
	}); err != nil {
		t.Fatalf("落交接单与确认：%v", err)
	}
	basis, err := nodomain.NewDeliveryScopeBasisReference(nodomain.BasisDeliveryHandoverNote, id)
	if err != nil {
		t.Fatalf("依据引用：%v", err)
	}
	return basis
}
