package main

import (
	"context"
	"errors"
	"testing"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 16 证释放答复三格与作用域随结果存下（ADR-0282；票 docs-consistency/code/71），夹具同
// superseded_version_control_release_test.go：控制与释放走生产 PS→SA 适配器、作用域源、SA 施加与释放编排与两本账。
//   - 施加时的那一份作用域随控制结果落库，与 SA 冻结所在的作用域逐段相同。
//   - 结果上没存作用域的那一笔（存量行，这里把三列抹空来摆），释放交零值给 SA、SA 拒收：撤回照样成立，那一笔记进释放
//     未受理册、不留补偿引用，冻结原样占着等人处理。
//   - 同样的一笔若是被取代那一版的，接受链判新版本之前放它被拒收，消费门响亮失败（不进未决），新版本一步不判。

// forgetOccupationScopes 把控制结果上存下的作用域抹掉，摆出施加时还不存作用域的存量行。
func (fixture *customerSupplementLoopFixture) forgetOccupationScopes(t *testing.T, ctx context.Context) {
	t.Helper()
	if _, err := fixture.pool.Exec(ctx,
		`UPDATE parcel_shipment.acceptance_financial_control
		    SET occupation_legal_entity = NULL, occupation_settlement_account = NULL, occupation_currency = NULL`); err != nil {
		t.Fatalf("抹掉存下的作用域：%v", err)
	}
}

// Covers: ADR-0282 决定四——施加时用的那一份作用域随控制结果落库，释放按它认领；与 SA 冻结所在的作用域逐段相同。
func TestTheControlResultRecordsTheScopeItsFreezeWasFormedIn(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{failuresLeft: 1}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter

	fixture.pauseFirstVersion(t, ctx)

	var recorded, frozen [3]string
	if err := fixture.pool.QueryRow(ctx,
		`SELECT occupation_legal_entity, occupation_settlement_account, occupation_currency
		   FROM parcel_shipment.acceptance_financial_control WHERE shipment_request_id = $1`,
		fixture.requestID.String()).Scan(&recorded[0], &recorded[1], &recorded[2]); err != nil {
		t.Fatalf("读控制结果上的作用域：%v", err)
	}
	if err := fixture.pool.QueryRow(ctx,
		`SELECT legal_entity, account_id, currency FROM settlement_accounting.funds_freeze WHERE control_request_id = $1`,
		fixture.requestID.String()+"/SYN-VER-01").Scan(&frozen[0], &frozen[1], &frozen[2]); err != nil {
		t.Fatalf("读冻结所在的作用域：%v", err)
	}
	if recorded != frozen {
		t.Fatalf("控制结果上存下的作用域 = %v, 冻结所在 = %v——释放会去另一个作用域认领", recorded, frozen)
	}
}

// Covers: AT-PS-115；ADR-0282 决定二、四；UC-PS-005「也不得把释放失败改写为撤回失败」——没存作用域的那一笔，撤回照样成立，释放被
// SA 拒收、记进释放未受理册待人处理，不留补偿引用；冻结原样占着。
func TestAWithdrawalOverAnUnscopedFreezeRecordsTheRefusalAndStands(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{failuresLeft: 1}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter
	firstVersion := fixture.requestID.String() + "/SYN-VER-01"

	// 暂停那一次释放放不掉，首版那一笔留着；再把它摆成没存作用域的存量行。
	fixture.pauseFirstVersion(t, ctx)
	fixture.forgetOccupationScopes(t, ctx)

	withdrawal := fixture.withdrawal(t, pspostgres.NewSavepointedControlRelease(fixture.db, release))
	var result psapplication.WithdrawShipmentRequestResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = withdrawal.Handle(txCtx, fixture.withdrawCommand(t, "SYN-VER-01"))
		return err
	})
	if result.Outcome() != psapplication.WithdrawalFormed {
		t.Fatalf("撤回 outcome = %q, want FORMED（未决原因 %q）——释放被拒收改写了撤回", result.Outcome(), result.PendingReason())
	}
	if !result.ControlReleaseNotAccepted() {
		t.Fatal("结果没带出「释放确定未受理、待人处理」这一格")
	}
	if result.CompensationReference().String() != "" {
		t.Fatalf("拒收已成定局却留了补偿引用 %q", result.CompensationReference())
	}
	// 已撤回的委托今天读不回聚合（票 code/36），状态取投影列。
	var state int16
	if err := fixture.pool.QueryRow(ctx,
		`SELECT state FROM parcel_shipment.shipment_request WHERE shipment_request_id = $1`,
		fixture.requestID.String()).Scan(&state); err != nil {
		t.Fatalf("读状态投影：%v", err)
	}
	if state != int16(psdomain.ShipmentRequestWithdrawn) {
		t.Fatalf("库里 state = %d, want WITHDRAWN", state)
	}

	refusals, err := pspostgres.NewControlReleaseRefusals(fixture.db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	listed, err := refusals.ListByShipmentRequest(ctx, fixture.identity.TenantID(), fixture.requestID)
	if err != nil {
		t.Fatalf("读释放未受理册：%v", err)
	}
	if len(listed) != 1 || listed[0].SubmissionVersion.String() != "SYN-VER-01" ||
		listed[0].ControlResultID.String() != firstVersion ||
		listed[0].Reason.String() != "SA-RELEASE_REQUEST_NOT_ACCEPTED" || listed[0].Scope.Recorded() {
		t.Fatalf("册上 = %+v, want 首版那一笔、SA 拒收、作用域没存下", listed)
	}
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 1 {
		t.Fatalf("首版冻结 = %+v, want 原样占着、等人处理", got)
	}
}

// Covers: AT-PS-116；ADR-0282 决定三；ADR-0081 决定三——被取代那一版的释放被 SA 拒收时，接受链判新版本之前响亮上抛
// ErrControlReleaseNotAccepted，消费门不入账、不当未决；新版本一步不判，冻结原样，册上不记（这一轮整笔回滚）。
func TestASupersededVersionWhoseReleaseIsRefusedFailsTheChainLoudly(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{failuresLeft: 1}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter
	firstVersion := fixture.requestID.String() + "/SYN-VER-01"

	paused := fixture.pauseFirstVersion(t, ctx)
	fixture.forgetOccupationScopes(t, ctx)
	supplemented := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
	if supplemented.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q（未决原因 %q）", supplemented.Outcome(), supplemented.PendingReason())
	}

	resume := fixture.claimDeliveryOfType(t, string(psinbox.SubmissionVersionFormedEventType)).Envelope
	err := fixture.supplementGate.Consume(ctx, resume)
	if !errors.Is(err, psapplication.ErrControlReleaseNotAccepted) {
		t.Fatalf("消费门 = %v, want ErrControlReleaseNotAccepted——拒收折成内部重试会一直挡着新版本", err)
	}
	if errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("消费门把拒收当成了未决：%v", err)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 1 {
		t.Fatalf("冻结行数 = %d, want 1——新版本一步不判，不该占下任何东西", n)
	}
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 1 {
		t.Fatalf("首版冻结 = %+v, want 原样占着", got)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.control_release_refusal`); n != 0 {
		t.Fatalf("册上 %d 笔, want 0——这一轮整笔回滚，记下的会随之撤回", n)
	}
	if after := fixture.mustLoadRequest(t, ctx); after.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("状态 = %q, want SUBMITTED", after.State())
	}
}
