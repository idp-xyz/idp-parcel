package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var receptionDeliveredAt = time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)

// Covers: `/node-operations/receptions` 的第二参是真编排——收寄库、结果版本签发与
// Outbox 意图交付在真实 PostgreSQL 上装得起来；身份核对缝显式未配置时，明确接收支
// 如实停在`收寄待确认`（未决带续办，不留记录），绝不冒充`待识别`那格业务答案——
// 后者是「查过了，查无此标识」，会真实建立收寄与控制；不经身份核对的明确拒收支
// 真实落库——重放同一份输入走已有结果，证明首笔事务真的提交了。测试输入是隔离
// 合成，只记 `S`，不进生产装配。
func TestTheWiredReceptionAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}

	explicit := receptionCommand(t, "SYN-DELIVERY-EXPLICIT-1", nodeopsapp.ExplicitReception)
	undecided, err := reception.Handle(t.Context(), explicit)
	if err != nil {
		t.Fatalf("明确接收：%v", err)
	}
	if got := undecided.Outcome(); got != nodeopsapp.ReceptionUndecided {
		t.Fatalf("outcome = %v, want RECEPTION_UNDECIDED——身份视图未配置时明确接收只能停下", got)
	}
	if _, has := undecided.Record(); has {
		t.Fatal("身份视图未配置的未决不该留下收寄记录——那会把「没核对」记成已判断")
	}
	if undecided.ContinuationReference() == "" {
		t.Fatal("未决没带续办引用，调用方无从查询原次尝试")
	}

	refusal := receptionCommand(t, "SYN-DELIVERY-REFUSED-1", nodeopsapp.ExplicitRefusal)
	first, err := reception.Handle(t.Context(), refusal)
	if err != nil {
		t.Fatalf("明确拒收：%v", err)
	}
	if got := first.Outcome(); got != nodeopsapp.NodeIntakeNotFormed {
		t.Fatalf("outcome = %v, want INTAKE_NOT_FORMED——拒收不经身份核对，未配置缝拦不到它", got)
	}

	replay, err := reception.Handle(t.Context(), refusal)
	if err != nil {
		t.Fatalf("重放同一份拒收：%v", err)
	}
	if got := replay.Outcome(); got != nodeopsapp.ReceptionExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT——重放没走已有结果，首笔事务的落库没有提交", got)
	}
}

func receptionCommand(
	t *testing.T,
	sourceID string,
	claim nodeopsapp.ReceptionClaimKind,
) nodeopsapp.ReceiveDeliveredUnitCommand {
	t.Helper()
	command := nodeopsapp.ReceiveDeliveredUnitCommand{
		TenantID:    mustValue(t, nodomain.NewTenantID, "SYN-TENANT-1"),
		SourceID:    sourceID,
		Node:        mustValue(t, nodomain.NewNodeReference, "SYN-NODE-ORIGIN"),
		DeliveredBy: mustValue(t, nodomain.NewDeliveringPartyReference, "SYN-CUSTOMER-1"),
		Unit:        mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-1"),
		Mark:        noports.ExternalMarkObservation{Mark: "SYN-BARCODE-1"},
		Claim:       claim,
		OccurredAt:  receptionDeliveredAt,
	}
	switch claim {
	case nodeopsapp.ExplicitReception:
		command.Evidence = mustValue(t, nodomain.NewReceptionEvidenceReference, "SYN-SIGN-1")
	case nodeopsapp.ExplicitRefusal:
		command.Refusal = "SYN-REFUSED-DAMAGED"
	}
	return command
}
