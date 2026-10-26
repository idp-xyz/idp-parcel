package main

import (
	"context"
	"testing"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: 票 docs-consistency/code/36、UC-PS-005 `AT-PS-068`、ADR-0286——撤回成立而随附释放没能确定完成，客户随后重复撤回：
// 重发同一撤回（同来源身份，走已保全路径）与换一个来源身份再撤一次（走授权之后的短路），两条路都要按来源身份把已撤回的
// 委托从真库读回来，答`决定已形成`并带原撤回、不再形成第二次，补偿引用与首次撤回那一笔同一个。已撤回行读不回时两条路都答
// `委托读不回`，客户拿到的是未决而不是原撤回。夹具同 customer_supplement_pause_release_test.go：来源与委托仓储、判断账、
// 控制与释放都是生产件，只有撤回授权恒授权。
func TestARepeatedWithdrawalReadsBackTheOriginalWithdrawalAndItsCompensation(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter

	firstVersion := fixture.requestID.String() + "/SYN-VER-01"

	// 暂停那一次释放放不掉，首版那一笔留着；之后每一次释放都答可重试未形成，撤回的补偿因此一直挂着。
	release.failuresLeft = 1
	fixture.pauseFirstVersion(t, ctx)
	release.failuresLeft = 100

	withdrawal := fixture.withdrawal(t, release)
	handle := func(command psapplication.WithdrawShipmentRequestCommand) psapplication.WithdrawShipmentRequestResult {
		t.Helper()
		var result psapplication.WithdrawShipmentRequestResult
		mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
			var err error
			result, err = withdrawal.Handle(txCtx, command)
			return err
		})
		return result
	}

	command := fixture.withdrawCommand(t, firstVersion)
	original := handle(command)
	if original.Outcome() != psapplication.WithdrawalFormed {
		t.Fatalf("撤回 outcome = %q, want FORMED（未决原因 %q）", original.Outcome(), original.PendingReason())
	}
	if original.CompensationReference().String() == "" {
		t.Fatal("释放没放掉却没有留补偿引用，本用例证不了重复撤回读回的是原补偿关联")
	}
	originalRecord, present := original.Withdrawal()
	if !present {
		t.Fatal("撤回成立却没有撤回记录")
	}

	// 库里那一份读得回，撤回决定一格不少。
	stored := fixture.mustLoadRequest(t, ctx)
	storedRecord, formed := stored.Withdrawal()
	if stored.State() != psdomain.ShipmentRequestWithdrawn || !formed || !sameWithdrawal(storedRecord, originalRecord) {
		t.Fatalf("读回 state = %q 撤回 = %+v（formed=%v）, want 已撤回与原撤回 %+v", stored.State(), storedRecord, formed, originalRecord)
	}

	otherIdentity, err := psdomain.NewSourceIdentity(
		fixture.identity.TenantID(), fixture.identity.CustomerAccountID(), fixture.identity.Source(),
		mustPS(t, psdomain.NewSourceRequestKey, "SYN-KEY-01-WITHDRAWAL-AGAIN"))
	if err != nil {
		t.Fatalf("第二个撤回来源身份：%v", err)
	}
	again := command
	again.WithdrawalIdentity = otherIdentity
	again.PayloadDigest = mustPS(t, psdomain.NewPayloadDigest, "SYN-DIGEST-WITHDRAWAL-AGAIN")

	for name, repeated := range map[string]psapplication.WithdrawShipmentRequestCommand{
		"重发同一撤回":    command,
		"换一个来源身份再撤": again,
	} {
		t.Run(name, func(t *testing.T) {
			result := handle(repeated)
			if result.Outcome() != psapplication.WithdrawalDecisionAlreadyFormed {
				t.Fatalf("outcome = %q, want DECISION_ALREADY_FORMED（未决原因 %q）", result.Outcome(), result.PendingReason())
			}
			record, present := result.Withdrawal()
			if !present || !sameWithdrawal(record, originalRecord) {
				t.Fatalf("撤回 = %+v（present=%v）, want 原撤回 %+v——重复撤回不得形成第二次", record, present, originalRecord)
			}
			if result.CompensationReference() != original.CompensationReference() {
				t.Fatalf("补偿引用 = %q, want 原补偿 %q", result.CompensationReference(), original.CompensationReference())
			}
			if result.State() != psdomain.ShipmentRequestWithdrawn {
				t.Fatalf("state = %q, want 已撤回", result.State())
			}
		})
	}
	if after := fixture.mustLoadRequest(t, ctx); after.Revision() != stored.Revision() {
		t.Fatalf("重复撤回之后版本 %d, want 仍是 %d——它写了第二次", after.Revision(), stored.Revision())
	}
}

func sameWithdrawal(left, right psdomain.Withdrawal) bool {
	return left.DecisionID() == right.DecisionID() && left.Authority() == right.Authority() &&
		left.Requester() == right.Requester() && left.Reason() == right.Reason() &&
		left.DecidedAt().Equal(right.DecidedAt())
}

// Covers: 票 docs-consistency/code/36、ADR-0286、UC-PS-005 步骤 6——客户在「委托已提交」信封被消费之前撤回了：接受链读回的是
// 已撤回的委托，一步判断都不走，不为它占一分钱（撤回已放掉各版占用，之后才长出来的占用没有任何路径会再放它），
// 本份投递按处理完毕入账、委托仍是已撤回。已撤回行读不回时链停在读错误上，这一格看不见；重建门开到`已撤回`之后链必须自己拦。
func TestASubmittedEnvelopeConsumedAfterTheWithdrawalPlacesNoControl(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter

	fixture.submit(t, ctx)
	submitted := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: submitted})
	})
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))

	withdrawal := fixture.withdrawal(t, release)
	var withdrawn psapplication.WithdrawShipmentRequestResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		withdrawn, err = withdrawal.Handle(txCtx, fixture.withdrawCommand(t, submitted.CurrentSubmissionVersion().VersionID().String()))
		return err
	})
	if withdrawn.Outcome() != psapplication.WithdrawalFormed {
		t.Fatalf("撤回 outcome = %q, want FORMED（未决原因 %q）", withdrawn.Outcome(), withdrawn.PendingReason())
	}

	if err := fixture.submittedGate.Consume(ctx, delivery.Envelope); err != nil {
		t.Fatalf("撤回之后消费提交信封必须按处理完毕入账：%v", err)
	}
	fixture.markPublished(t, delivery)

	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 0 {
		t.Fatalf("冻结行数 = %d, want 0——为一份已撤回的委托占了钱，撤回之后没有路径会再放它", n)
	}
	if after := fixture.mustLoadRequest(t, ctx); after.State() != psdomain.ShipmentRequestWithdrawn {
		t.Fatalf("消费之后状态 = %q, want 仍是已撤回", after.State())
	}
}
