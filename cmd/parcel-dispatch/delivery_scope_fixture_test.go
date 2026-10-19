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
)

// 本文件是派发真库用例的可验证交付范围夹具（ADR-0212 决定二、三；票 ps-external-mark-relations/11）：经 NO 的预报册、交接单册与时窗册
// 直接落证据，交回收寄要带的依据引用。客户包裹件号的送站此后要有可验证交付范围才唯一识别（决定四、十），用例的本意是「这件能被识别」
// 或「这一格冲突在信号消失后解开」时就带上它。值全为合成（S 级）。

var scopeFixtureSequence atomic.Int64

// scopeWindowForDispatch 给租户登一行早就生效的时窗（提前一天、宽限半天）；已在册时原样再登答已在册，不算失败。
func scopeWindowForDispatch(t *testing.T, db *bentopg.DB, tenant nodomain.TenantID) {
	t.Helper()
	windows, err := nopostgres.NewPreAdviceWindows(db)
	if err != nil {
		t.Fatalf("构造时窗册：%v", err)
	}
	window, err := nodomain.FormPreAdviceValidityWindow(nodomain.PreAdviceValidityWindowSpec{
		TenantID:      tenant,
		Lead:          24 * time.Hour,
		Grace:         12 * time.Hour,
		EffectiveFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Basis:         "SYN-WINDOW-BASIS-PSXM11",
		RegisteredAt:  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("时窗：%v", err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, _, err := windows.RegisterPreAdviceWindow(txCtx, window)
		return err
	})
}

// preAdviseForDispatch 给 customer 在 SYN-NODE-01 登一份预计送达时刻为 expected、列着 number 的预报，并确保有时窗。
func preAdviseForDispatch(
	t *testing.T,
	db *bentopg.DB,
	tenant nodomain.TenantID,
	customer, number string,
	expected time.Time,
) nodomain.DeliveryScopeBasisReference {
	t.Helper()
	scopeWindowForDispatch(t, db, tenant)
	preAdvices, err := nopostgres.NewDeliveryPreAdvices(db)
	if err != nil {
		t.Fatalf("构造预报册：%v", err)
	}
	sequence := scopeFixtureSequence.Add(1)
	id := fmt.Sprintf("SYN-PREADV-PSXM11-D%04d", sequence)
	preAdvice, err := nodomain.FormDeliveryPreAdvice(nodomain.DeliveryPreAdviceSpec{
		TenantID:          tenant,
		ID:                mustNO(t, nodomain.NewDeliveryPreAdviceID, id),
		CustomerAccount:   mustNO(t, nodomain.NewCustomerAccountReference, customer),
		Node:              mustNO(t, nodomain.NewNodeReference, "SYN-NODE-01"),
		ExpectedArrivalAt: expected,
		Numbers:           []nodomain.PreAdvisedParcelNumber{mustNO(t, nodomain.NewPreAdvisedParcelNumber, number)},
		Source: nodomain.CustomerRequestSource{
			Channel:       "SYN-CUSTOMER-CLIENT-PSXM11",
			RequestKey:    fmt.Sprintf("SYN-ACCESS-REQUEST-PSXM11-D%04d", sequence),
			ContentDigest: "NOC-1:syn-" + id,
		},
		FormedAt: expected.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("预报：%v", err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, _, err := preAdvices.RecordDeliveryPreAdvice(txCtx, preAdvice)
		return err
	})
	return mustNO(t, func(raw string) (nodomain.DeliveryScopeBasisReference, error) {
		return nodomain.NewDeliveryScopeBasisReference(nodomain.BasisDeliveryPreAdvice, raw)
	}, id)
}

// confirmedHandoverNoteForDispatch 在 SYN-NODE-01 开立一张声称 customers 的送站交接单，各客户账户都经客户渠道确认了绑定，交回依据引用。
// 交接单成立的范围是声称的整组客户账户（ADR-0212 决定三）：一次送站同时替几个客户账户送，范围就是那几个，各账户的`已提交`声明都在
// 范围内。开立与确认的命令口归票 12，这里经交接单册直接落。
func confirmedHandoverNoteForDispatch(
	t *testing.T,
	db *bentopg.DB,
	tenant nodomain.TenantID,
	openedAt time.Time,
	customers ...string,
) nodomain.DeliveryScopeBasisReference {
	t.Helper()
	notes, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatalf("构造交接单册：%v", err)
	}
	id := fmt.Sprintf("SYN-HANDOVER-PSXM11-D%04d", scopeFixtureSequence.Add(1))
	accounts := make([]nodomain.CustomerAccountReference, 0, len(customers))
	for _, customer := range customers {
		accounts = append(accounts, mustNO(t, nodomain.NewCustomerAccountReference, customer))
	}
	note, err := nodomain.OpenDeliveryHandoverNote(nodomain.DeliveryHandoverNoteSpec{
		TenantID:        tenant,
		ID:              mustNO(t, nodomain.NewDeliveryHandoverNoteID, id),
		Node:            mustNO(t, nodomain.NewNodeReference, "SYN-NODE-01"),
		DeliveringParty: mustNO(t, nodomain.NewDeliveringPartyReference, "SYN-DRIVER-PSXM11"),
		ClaimedAccounts: accounts,
		OpenedAt:        openedAt,
	})
	if err != nil {
		t.Fatalf("交接单：%v", err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if _, _, err := notes.OpenDeliveryHandoverNote(txCtx, note); err != nil {
			return err
		}
		for _, account := range accounts {
			binding, err := nodomain.FormDeliveryHandoverBinding(nodomain.DeliveryHandoverBindingSpec{
				TenantID:         tenant,
				Note:             note.ID(),
				Account:          account,
				Conclusion:       nodomain.BindingConfirmed,
				Route:            nodomain.ConcludedViaCustomerChannel,
				Concluder:        "SYN-CUSTOMER-CLIENT-PSXM11",
				SourceRequestKey: "SYN-ACCESS-REQUEST-PSXM11-" + id + "-" + account.String(),
				ConcludedAt:      openedAt.Add(time.Minute),
			})
			if err != nil {
				return err
			}
			if _, _, err := notes.RecordDeliveryHandoverBinding(txCtx, binding); err != nil {
				return err
			}
		}
		return nil
	})
	return mustNO(t, func(raw string) (nodomain.DeliveryScopeBasisReference, error) {
		return nodomain.NewDeliveryScopeBasisReference(nodomain.BasisDeliveryHandoverNote, raw)
	}, id)
}

// deliveryScopeEvidenceFor 装收寄编排要的交付范围证据读口（生产那一只）。
func deliveryScopeEvidenceFor(t *testing.T, db *bentopg.DB) nopostgres.DeliveryScopeEvidence {
	t.Helper()
	scope, err := nopostgres.NewDeliveryScopeEvidence(db)
	if err != nil {
		t.Fatalf("构造交付范围证据：%v", err)
	}
	return scope
}

// handoverMembershipFor 装收寄编排落交接单成员那一口（生产那一只：交接单册）。
func handoverMembershipFor(t *testing.T, db *bentopg.DB) *nopostgres.DeliveryHandoverNotes {
	t.Helper()
	notes, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatalf("构造交接单册：%v", err)
	}
	return notes
}
