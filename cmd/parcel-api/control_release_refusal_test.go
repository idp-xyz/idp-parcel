package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件经生产的授权处置装配（操作者 Intake、PS 真库口、PS→SA 释放适配器、SA 释放编排与真账本、释放未受理册）证释放按
// 控制结果上存下的作用域认领、拒收落进释放未受理册并在答复里带出（ADR-0282 决定二、四；票 docs-consistency/code/71）。
// 本装配不接作用域源：存下作用域之前，这里的释放一律放不掉、只留补偿。处置授权只有授权器是合成替身，理由见
// synRDispositionAuthorizer。测试输入是合成数据，只记 `S`。

var refusalControlAt = time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)

// recordControlOnRealAssembly 给指名的提交版本记一份预付冻结成立的控制结果，作用域取 scope（零值即存量行没存下）。
func recordControlOnRealAssembly(
	t *testing.T,
	db *bentopg.DB,
	version domain.SubmissionVersionID,
	scope domain.OccupationScope,
) domain.FinancialControlResultID {
	t.Helper()
	command := submissionCommand(t)
	policy, err := domain.NewEchoedAsOfPolicy(domain.FinancialControlJudgmentKind,
		mustValue(t, domain.NewAsOfSemanticsReference, "SUBMISSION_RECEIPT"),
		mustValue(t, domain.NewAsOfPolicyVersion, "SYN-AS-OF-1"))
	if err != nil {
		t.Fatalf("时点策略：%v", err)
	}
	asOf, err := domain.NewJudgmentAsOf(refusalControlAt, policy)
	if err != nil {
		t.Fatalf("判断时点：%v", err)
	}
	item, err := domain.NewControlItemResult(domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied, domain.ControlBasisReference{})
	if err != nil {
		t.Fatalf("控制项：%v", err)
	}
	resultID := mustValue(t, domain.NewFinancialControlResultID, command.ShipmentRequestID.String()+"/"+version.String())
	result, err := domain.NewExecutedFinancialControlResult(domain.ExecutedFinancialControlSpec{
		ResultID: resultID, Items: []domain.ControlItemResult{item}, JointPass: domain.AllControlsPass, AsOf: asOf, Scope: scope,
	})
	if err != nil {
		t.Fatalf("控制结果：%v", err)
	}
	judgments, err := pspostgres.NewAcceptanceJudgments(db)
	if err != nil {
		t.Fatalf("构造判断账：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return judgments.RecordFinancialControlResult(txCtx, command.Identity.TenantID(), command.ShipmentRequestID, version, result)
	}); err != nil {
		t.Fatalf("记控制结果：%v", err)
	}
	return resultID
}

// freezeOnRealAssembly 在 SA 冻结账本上按控制结果标识冻一笔，落在 scope 那一份作用域里。
func freezeOnRealAssembly(t *testing.T, db *bentopg.DB, resultID domain.FinancialControlResultID, scope sadomain.SettlementScope) {
	t.Helper()
	tenant := mustValue(t, sadomain.NewTenantID, submissionCommand(t).Identity.TenantID().String())
	balance, err := sadomain.NewOperationalBalance(scope, 10_000, 0, 0, 0)
	if err != nil {
		t.Fatalf("运营余额：%v", err)
	}
	request, err := sadomain.NewFreezeRequest(
		mustValue(t, sadomain.NewControlRequestID, resultID.String()), scope, 4_000,
		mustValue(t, sadomain.NewBusinessAssociationReference, resultID.String()), refusalControlAt)
	if err != nil {
		t.Fatalf("冻结请求：%v", err)
	}
	ledger := sadomain.NewFreezeLedger()
	if _, err := ledger.Freeze(request, balance); err != nil {
		t.Fatalf("冻结：%v", err)
	}
	freezes, err := sapostgres.NewFreezeLedgers(db)
	if err != nil {
		t.Fatalf("构造冻结账本：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return freezes.Save(txCtx, tenant, scope, ledger)
	}); err != nil {
		t.Fatalf("落冻结：%v", err)
	}
}

func freezeStatusOnRealAssembly(t *testing.T, db *bentopg.DB, resultID domain.FinancialControlResultID) int16 {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("读口：%v", err)
	}
	var status int16
	if err := querier.QueryRow(t.Context(),
		`SELECT status FROM settlement_accounting.funds_freeze WHERE control_request_id = $1`, resultID.String(),
	).Scan(&status); err != nil {
		t.Fatalf("读冻结：%v", err)
	}
	return status
}

func refusalSettlementScope(t *testing.T) (domain.OccupationScope, sadomain.SettlementScope) {
	t.Helper()
	recorded, err := domain.NewOccupationScope("SYN-LEGAL-ENTITY-1", "SYN-ACCOUNT-1", "CNY")
	if err != nil {
		t.Fatalf("占用作用域：%v", err)
	}
	settlement, err := sadomain.NewSettlementScope(
		mustValue(t, sadomain.NewLegalEntityReference, "SYN-LEGAL-ENTITY-1"),
		mustValue(t, sadomain.NewSettlementAccountID, "SYN-ACCOUNT-1"),
		mustValue(t, sadomain.NewCurrencyCode, "CNY"))
	if err != nil {
		t.Fatalf("结算作用域：%v", err)
	}
	return recorded, settlement
}

func disposeRejectOnRealAssembly(t *testing.T, db *bentopg.DB, version domain.SubmissionVersionID) (int, decisionAnswer) {
	t.Helper()
	disposition, err := dispositionOrchestrationWith(db, synRDispositionAuthorizer{t: t})
	if err != nil {
		t.Fatalf("装配授权处置编排：%v", err)
	}
	views, err := pspostgres.NewShipmentRequestViews(db)
	if err != nil {
		t.Fatalf("构造委托查阅读口：%v", err)
	}
	command := submissionCommand(t)
	intake, err := shipmenthttp.NewOperatorDecisionIntake(
		channelOperatorAuthenticator{tenant: command.Identity.TenantID(), decision: shipmenthttp.OperatorDecisionAuthorizedDisposition},
		views, unwiredDecisionTargets{})
	if err != nil {
		t.Fatalf("构造操作者 Intake：%v", err)
	}
	endpoint := shipmenthttp.NewDisposeShipmentRequestEndpoint(intake, disposition)
	body := `{"shipmentRequestId": "` + command.ShipmentRequestID.String() + `", "submissionVersionId": "` + version.String() +
		`", "choice": "REJECT", "reason": "SYN-DISPOSITION-REASON-1"}`
	return postDecision(t, endpoint, "/shipment-requests/authorized-dispositions", "SYN-OPERATOR-01", body)
}

// Covers: ADR-0282 决定四——生产处置装配不接作用域源，释放按控制结果上存下的那一份作用域认领：冻结放掉，答复不带补偿、
// 不带拒收。
func TestTheWiredDispositionReleasesTheFreezeInItsRecordedScope(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	version := awaitingDispositionOnRealAssembly(t, db)
	recorded, settlement := refusalSettlementScope(t)
	resultID := recordControlOnRealAssembly(t, db, version, recorded)
	freezeOnRealAssembly(t, db, resultID, settlement)

	status, answer := disposeRejectOnRealAssembly(t, db, version)
	if status != http.StatusCreated || answer.Outcome != "RECORDED" {
		t.Fatalf("处置答 %d %+v，want 201 RECORDED", status, answer)
	}
	if answer.CompensationReference != "" || answer.ControlReleaseNotAccepted {
		t.Fatalf("答复 %+v，want 释放已收口、不带补偿也不带拒收", answer)
	}
	if got := freezeStatusOnRealAssembly(t, db, resultID); got != int16(sadomain.FreezeReleased) {
		t.Fatalf("冻结 status = %d, want 已释放", got)
	}
}

// Covers: AT-PS-035 释放被确定不受理那一格；ADR-0282 决定二、四——存量结果没存作用域：处置照样成立，SA 拒收释放，答复带 `controlReleaseNotAccepted`、
// 不带补偿引用，那一笔记进释放未受理册，冻结原样占着等人处理。
func TestTheWiredDispositionRecordsAndAnswersARefusedRelease(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	version := awaitingDispositionOnRealAssembly(t, db)
	_, settlement := refusalSettlementScope(t)
	resultID := recordControlOnRealAssembly(t, db, version, domain.OccupationScope{})
	freezeOnRealAssembly(t, db, resultID, settlement)

	status, answer := disposeRejectOnRealAssembly(t, db, version)
	if status != http.StatusCreated || answer.Outcome != "RECORDED" {
		t.Fatalf("处置答 %d %+v，want 201 RECORDED——释放被拒收改写了处置", status, answer)
	}
	if !answer.ControlReleaseNotAccepted || answer.CompensationReference != "" {
		t.Fatalf("答复 %+v，want 带出拒收、不带补偿引用", answer)
	}
	refusals, err := pspostgres.NewControlReleaseRefusals(db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	command := submissionCommand(t)
	listed, err := refusals.ListByShipmentRequest(t.Context(), command.Identity.TenantID(), command.ShipmentRequestID)
	if err != nil {
		t.Fatalf("读释放未受理册：%v", err)
	}
	if len(listed) != 1 || listed[0].ControlResultID != resultID || listed[0].SubmissionVersion != version ||
		listed[0].Reason.String() != "SA-RELEASE_REQUEST_NOT_ACCEPTED" {
		t.Fatalf("册上 = %+v, want 那一笔、SA 拒收", listed)
	}
	if got := freezeStatusOnRealAssembly(t, db, resultID); got != int16(sadomain.FreezeHeld) {
		t.Fatalf("冻结 status = %d, want 原样占着", got)
	}
}
