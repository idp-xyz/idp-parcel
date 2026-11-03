package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func numberedBill(t *testing.T, claimID, version, supplier, number string) application.ReceiveSupplierBillCommand {
	t.Helper()
	command := billCommand(t, claimID)
	command.Claim.Version = billValue(t, domain.NewBillClaimVersion, version)
	command.Claim.Supplier = billValue(t, domain.NewSupplierPartyReference, supplier)
	if number != "" {
		command.Claim.BillNumber = billValue(t, domain.NewSupplierBillNumber, number)
	}
	return command
}

func receiveOnce(t *testing.T, fixture *billFixture, command application.ReceiveSupplierBillCommand) application.ReceiveSupplierBillResult {
	t.Helper()
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("receive %s: %v", command.Claim.Claim, err)
	}
	return result
}

// Covers: `AT-SA-233`（编排半边）——供应商账单号记成主张的外部标识，查重在服务端：同一供应商同一号换个主张标识再录答账单号冲突并
// 带出已登记的主张；这份主张已以另一个号登过也答账单号冲突；另一家供应商同号照常接收；同一主张的新版本可带同号、可不带号；
// 重放时带了号而这份主张还没有号答版本冲突；新版本换了供应商，不论带不带号，答主张供应商冲突；不带号的旧载荷照收；先排队再查号。
func TestASupplierBillNumberIdentifiesOneClaimPerSupplier(t *testing.T) {
	fixture := newBillFixture(t)
	first := receiveOnce(t, fixture, numberedBill(t, "bill-a", "v1", "partner-1", "INV-001"))
	if first.Outcome() != application.BillReceived {
		t.Fatalf("outcome = %q", first.Outcome())
	}
	record, _ := first.Record()
	if number, has := record.Claim.BillNumber(); !has || number.String() != "INV-001" {
		t.Fatalf("账单号没随主张落下：%v %v", number, has)
	}
	if len(fixture.store.locks) == 0 || fixture.store.locks[0] != "bill-a#INV-001" {
		t.Fatalf("locks = %v，要先按主张与账单号排队", fixture.store.locks)
	}

	t.Run("the same number under another claim identity is a conflict", func(t *testing.T) {
		second := receiveOnce(t, fixture, numberedBill(t, "bill-b", "v1", "partner-1", "INV-001"))
		conflict, has := second.NumberConflict()
		if second.Outcome() != application.BillNumberConflict || !has || conflict.RegisteredClaim.String() != "bill-a" {
			t.Fatalf("outcome = %q conflict = %+v", second.Outcome(), conflict)
		}
		if len(fixture.store.records) != 1 {
			t.Fatal("同一张账单落成了第二份主张")
		}
	})

	t.Run("a claim registered under another number is a conflict", func(t *testing.T) {
		renumbered := receiveOnce(t, fixture, numberedBill(t, "bill-a", "v2", "partner-1", "INV-002"))
		conflict, _ := renumbered.NumberConflict()
		if renumbered.Outcome() != application.BillNumberConflict || conflict.RegisteredNumber.String() != "INV-001" {
			t.Fatalf("outcome = %q conflict = %+v", renumbered.Outcome(), conflict)
		}
	})

	t.Run("a new version under another supplier is refused with or without a number", func(t *testing.T) {
		for _, number := range []string{"INV-001", ""} {
			switched := receiveOnce(t, fixture, numberedBill(t, "bill-a", "v9", "partner-2", number))
			conflict, _ := switched.NumberConflict()
			if switched.Outcome() != application.BillClaimSupplierConflict || conflict.RegisteredSupplier.String() != "partner-1" {
				t.Fatalf("号 %q：outcome = %q conflict = %+v（同一份主张的各版本供应商不变）", number, switched.Outcome(), conflict)
			}
		}
	})

	t.Run("another supplier may use the same number", func(t *testing.T) {
		if got := receiveOnce(t, fixture, numberedBill(t, "bill-c", "v1", "partner-2", "INV-001")).Outcome(); got != application.BillReceived {
			t.Fatalf("outcome = %q", got)
		}
	})

	t.Run("a new version keeps or omits the claim's number", func(t *testing.T) {
		if got := receiveOnce(t, fixture, numberedBill(t, "bill-a", "v2", "partner-1", "INV-001")).Outcome(); got != application.BillReceived {
			t.Fatalf("同号新版本 outcome = %q", got)
		}
		if got := receiveOnce(t, fixture, numberedBill(t, "bill-a", "v3", "partner-1", "")).Outcome(); got != application.BillReceived {
			t.Fatalf("不带号的新版本 outcome = %q", got)
		}
	})

	t.Run("a replay with the number answers the original", func(t *testing.T) {
		if got := receiveOnce(t, fixture, numberedBill(t, "bill-a", "v1", "partner-1", "INV-001")).Outcome(); got != application.BillExistingResult {
			t.Fatalf("outcome = %q", got)
		}
	})

	t.Run("a number cannot be attached through a replay", func(t *testing.T) {
		if got := receiveOnce(t, fixture, numberedBill(t, "bill-d", "v1", "partner-1", "")).Outcome(); got != application.BillReceived {
			t.Fatalf("不带号的旧载荷 outcome = %q", got)
		}
		if got := receiveOnce(t, fixture, numberedBill(t, "bill-d", "v1", "partner-1", "INV-009")).Outcome(); got != application.BillVersionConflict {
			t.Fatalf("重放补号 outcome = %q，want VERSION_CONFLICT", got)
		}
	})
}
