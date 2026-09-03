package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var movementFactOccurredAt = time.Date(2026, 8, 22, 6, 0, 0, 0, time.UTC)

// Covers: `/transport-fulfillment/movement-facts` 的第二参是真编排（票 tf-segment-lifecycle-closure/05）
// ——移动事实登记册在真实 PostgreSQL 上装得起来，事务边界成立（重放走已有版本，证首笔真的提交了）；
// 出发的门禁那道领域门经真装配照样成立：要门禁而没带放行是 DEPARTURE_GATE_BLOCKED 且不落库，带了放行
// 才登上。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredMovementFactAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	movement, err := buildMovementFactOrchestration(db)
	if err != nil {
		t.Fatalf("装配移动事实编排：%v", err)
	}

	arrival := recordMovementFactCommand(t, "SYN-MOVEMENT-ARR-1", tfdomain.ArrivalFact)
	recorded, err := movement.Record(t.Context(), arrival)
	if err != nil {
		t.Fatalf("登记到达：%v", err)
	}
	if got := recorded.Outcome(); got != tfapp.MovementFactRecorded {
		t.Fatalf("outcome = %v, want MOVEMENT_FACT_RECORDED", got)
	}
	if _, has := recorded.Record(); !has {
		t.Fatal("登记成功却没带回记录")
	}

	replay, err := movement.Record(t.Context(), arrival)
	if err != nil {
		t.Fatalf("重放到达：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.MovementFactVersionExists {
		t.Fatalf("outcome = %v, want MOVEMENT_FACT_VERSION_EXISTS——重放没走已有版本，首笔事务没有提交", got)
	}

	departure := recordMovementFactCommand(t, "SYN-MOVEMENT-DEP-1", tfdomain.DepartureFact)
	departure.GateRequired = true
	blocked, err := movement.Record(t.Context(), departure)
	if err != nil {
		t.Fatalf("无放行出发：%v", err)
	}
	if got := blocked.Outcome(); got != tfapp.MovementFactGateBlocked {
		t.Fatalf("outcome = %v, want DEPARTURE_GATE_BLOCKED", got)
	}

	departure.GateClearance = "SYN-GATE-CLEARANCE-1"
	cleared, err := movement.Record(t.Context(), departure)
	if err != nil {
		t.Fatalf("带放行出发：%v", err)
	}
	if got := cleared.Outcome(); got != tfapp.MovementFactRecorded {
		t.Fatalf("outcome = %v, want MOVEMENT_FACT_RECORDED——被门禁挡过的那次不该占掉这个版本", got)
	}
	record, has := cleared.Record()
	if !has {
		t.Fatal("登记成功却没带回记录")
	}
	if clearance, present := record.Fact.GateClearance(); !present || clearance.String() != "SYN-GATE-CLEARANCE-1" {
		t.Fatalf("gateClearance = (%q, %v)", clearance.String(), present)
	}
}

func recordMovementFactCommand(t *testing.T, fact string, kind tfdomain.MovementFactKind) tfapp.RecordMovementFactCommand {
	t.Helper()
	return tfapp.RecordMovementFactCommand{
		TenantID:   mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Fact:       fact,
		Schedule:   "SYN-SCHEDULE-1",
		Kind:       kind,
		Location:   "SYN-HUB-1",
		Source:     "SYN-OWN-FLEET-SCAN/v1",
		Version:    "SYN-MFV-000000000001",
		OccurredAt: movementFactOccurredAt,
	}
}
