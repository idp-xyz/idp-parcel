package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var withdrawalSeedAt = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)

// Covers: `/shipment-requests/withdrawals` 的第二参是真编排——整条链（来源保全、委托
// 仓储、撤回授权经 PC 裁定、判断读写、SA 释放、决定标识）在真实 PostgreSQL 上装得起来；
// 授权请求映射未配置（实例半边）时编排如实停在`撤回授权不可用`，不代拟坐标、不冒充
// `授权规则未配置`、也不默认任何角色有撤回权（UC-PS-005 明禁两个方向的默认）；来源保全
// 确实落库——重放同一份输入走已保全路径，证明首笔事务真的提交了。测试输入是隔离合成，
// 只记 `S`，不进生产装配。
func TestTheWiredWithdrawalAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	identity := withdrawalSeedIdentity(t)
	seedSubmittedShipmentRequest(t, db, identity)

	withdrawal, err := buildWithdrawalOrchestration(db)
	if err != nil {
		t.Fatalf("装配撤回编排：%v", err)
	}

	command := withdrawalCommand(t, identity)
	first, err := withdrawal.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("首次撤回：%v", err)
	}
	if got := first.Outcome(); got != shipmentapp.WithdrawalUndecided {
		t.Fatalf("outcome = %v, want UNDECIDED——授权映射未配置时撤回只能停下", got)
	}
	if got := first.PendingReason(); got != shipmentapp.WithdrawalAuthorityUnavailable {
		t.Fatalf("reason = %v, want WITHDRAWAL_AUTHORITY_UNAVAILABLE", got)
	}
	if first.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %v；未决交回的是已查到的事实，委托此刻仍是已提交", first.State())
	}
	if first.ContinuationReference().String() == "" {
		t.Fatal("未决没带续办引用，调用方无从查询原次尝试")
	}

	// 重放走已保全路径而不再问授权，正证明首笔事务（含来源保全）已提交；答案的形状
	// （resolvePreserved → 读回既有状态）由应用层拥有并在其单测钉住，这里只认路径。
	replay, err := withdrawal.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放同一份输入：%v", err)
	}
	if got := replay.Outcome(); got == shipmentapp.WithdrawalUndecided {
		t.Fatalf("outcome = %v；重放仍走了整套判断——首笔事务的来源保全没有提交", got)
	}
}

func withdrawalSeedIdentity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-1"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, "SYN-SUBMIT-KEY-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity
}

// seedSubmittedShipmentRequest 直接经领域构造并落库一份`已提交`委托。不走提交编排，
// 是因为治理目录未配置时提交停在 OWNERSHIP_UNRESOLVED、根本建不出单——被测行为从
// 委托已经存在开始（与 form_acceptance_decision 单测的 submittedRequest 同一条理由）。
func seedSubmittedShipmentRequest(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity) {
	t.Helper()

	scope, err := domain.NewAdmissionScope(
		mustValue(t, domain.NewAdmissionScopeReference, "scope-ref-syn-1"),
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-syn-1"),
	)
	if err != nil {
		t.Fatalf("准入范围：%v", err)
	}
	validity, err := domain.NewOwnershipValidityInterval(
		withdrawalSeedAt.Add(-24*time.Hour),
		withdrawalSeedAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	decision, err := domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustValue(t, domain.NewProductionOwnershipDecisionID, "SYN-DECISION-1"),
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      mustValue(t, domain.NewProductionOwnershipRuleVersion, "SYN-RULE-1"),
		AsOf:             withdrawalSeedAt,
		Validity:         validity,
		Revision:         mustValue(t, domain.NewProductionOwnershipRevision, "SYN-REV-1"),
		DecisionAt:       withdrawalSeedAt,
	})
	if err != nil {
		t.Fatalf("归属决定：%v", err)
	}
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision, scope.Digest(),
		mustValue(t, domain.NewProductionOwnershipRevision, "SYN-REV-1"), withdrawalSeedAt)
	if err != nil {
		t.Fatalf("建单门禁：%v", err)
	}
	fingerprint, err := domain.NewSourceSubmissionFingerprint(
		identity,
		mustValue(t, domain.NewPayloadDigest, "syn-submit-digest-1"),
		withdrawalSeedAt.Add(-time.Hour),
		withdrawalSeedAt.Add(-time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("来源指纹：%v", err)
	}
	candidate, err := domain.NewSubmissionCandidate(
		fingerprint,
		mustValue(t, domain.NewSubmissionBatchID, "SYN-BATCH-1"),
		mustValue(t, domain.NewShipmentRequestID, "SYN-REQUEST-1"),
		[]domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-1")},
	)
	if err != nil {
		t.Fatalf("提交候选：%v", err)
	}
	request, err := domain.SubmitShipmentRequest(domain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   mustValue(t, domain.NewSubmissionVersionID, "SYN-VERSION-1"),
		TaskID:      mustValue(t, domain.NewAcceptanceDecisionTaskID, "SYN-TASK-1"),
		SubmittedAt: withdrawalSeedAt,
	})
	if err != nil {
		t.Fatalf("建单：%v", err)
	}

	repository, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	// 事务闭包里不 Fatalf：FailNow 走 Goexit，事务连接不归还，测试清理的 pool.Close
	// 会永远等它——outcome 留到事务提交后再断言。
	var outcome ports.ShipmentRequestInsertOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		inserted, insertErr := repository.Insert(txCtx, identity, request)
		if insertErr != nil {
			return insertErr
		}
		outcome = inserted
		return nil
	}); err != nil {
		t.Fatalf("种单落库：%v", err)
	}
	if outcome != ports.ShipmentRequestInserted {
		t.Fatalf("种单 outcome = %v, want INSERTED", outcome)
	}
}

func withdrawalCommand(t *testing.T, identity domain.SourceIdentity) shipmentapp.WithdrawShipmentRequestCommand {
	t.Helper()
	withdrawalIdentity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-1"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, "SYN-WITHDRAW-KEY-1"),
	)
	if err != nil {
		t.Fatalf("new withdrawal identity: %v", err)
	}
	return shipmentapp.WithdrawShipmentRequestCommand{
		Identity:           identity,
		WithdrawalIdentity: withdrawalIdentity,
		PayloadDigest:      mustValue(t, domain.NewPayloadDigest, "syn-withdraw-digest-1"),
		OccurredAt:         withdrawalSeedAt.Add(time.Hour),
		ReceivedAt:         withdrawalSeedAt.Add(time.Hour + time.Second),
		ShipmentRequestID:  mustValue(t, domain.NewShipmentRequestID, "SYN-REQUEST-1"),
		SubmissionVersion:  mustValue(t, domain.NewSubmissionVersionID, "SYN-VERSION-1"),
		Requester:          mustValue(t, domain.NewWithdrawalRequesterReference, "SYN-CUSTOMER-1"),
		Reason:             mustValue(t, domain.NewWithdrawalReasonReference, "SYN-ORDER-CANCELLED"),
	}
}
