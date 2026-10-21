package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 16 证出发读数里集运单元那一格与它的成员同一行往返（ADR-0225 决定五）。夹具全为合成登记（S 级）。

// consolidationDepartureSpec 是一条出发：段里在控的是笼 cage-1，笼里装着袋 bag-1 与包裹 parcel-loose，袋里装着被扣的 parcel-held；
// 笼里还有一件身份不成立的作业实物 hu-9。
func consolidationDepartureSpec(t *testing.T, fact string) domain.MovementFactSpec {
	t.Helper()
	spec := movementFactSpec(t, fact, "v1", domain.DepartureFact)
	held, err := domain.NewGateFinding(segmentRef(t, domain.NewCarriedObjectReference, "parcel-held"), domain.GateBlocked,
		"AADJ-parcel-held", []domain.GateBasis{{Action: "LOADING_DEPARTURE", Cause: "RESTRICTION_IN_FORCE", Sources: []string{"RST-1"}}})
	if err != nil {
		t.Fatalf("被扣那一件：%v", err)
	}
	loose, err := domain.NewGateFinding(segmentRef(t, domain.NewCarriedObjectReference, "parcel-loose"), domain.GateNotGuarded,
		"AADJ-parcel-loose", []domain.GateBasis{{Action: "LOADING_DEPARTURE", Cause: "NOT_GUARDED", Sources: []string{}}})
	if err != nil {
		t.Fatalf("不受门禁那一件：%v", err)
	}
	bag, err := domain.FormConsolidationUnitGateFinding(segmentRef(t, domain.NewCarriedObjectReference, "bag-1"),
		[]domain.GateFinding{held}, nil)
	if err != nil {
		t.Fatalf("袋那一格：%v", err)
	}
	cage, err := domain.FormConsolidationUnitGateFinding(segmentRef(t, domain.NewCarriedObjectReference, "cage-1"),
		[]domain.GateFinding{bag, loose},
		[]domain.ConsolidationMemberGap{{Stop: domain.ConsolidationMemberIdentityUnresolved, Sources: []string{"hu-9"}}})
	if err != nil {
		t.Fatalf("笼那一格：%v", err)
	}
	spec.GateReading, err = domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
		From:     segmentRef(t, domain.NewNetworkNodeReference, "NODE-SHA"),
		To:       segmentRef(t, domain.NewNetworkNodeReference, "NODE-SZX"),
		Findings: []domain.GateFinding{cage, bag, held, loose},
	})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	return spec
}

// Covers: AT-TF-279 真库半边（ADR-0225 决定五）——单元那一格连同它指名的成员、它的缘由，与成员各自那一格同一行往返；单元那一格读回
// 仍是单元那一格、不带关务判断标识，问关务问来的那几格读回仍不是单元。
func TestAUnitCellRoundTripsWithItsMembers(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()
	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecordOf(t, consolidationDepartureSpec(t, "MF-CU-1")))

	found, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-CU-1", "v1"))
	if err != nil || !exists {
		t.Fatalf("取回事实：%v exists=%v", err, exists)
	}
	reading, _ := found.Fact.GateReading()
	if reading.Answer() != domain.GateBlocked {
		t.Fatalf("整格 = %s", reading.Answer())
	}
	cells := map[string]domain.GateFinding{}
	for _, finding := range reading.Findings() {
		cells[finding.Object().String()] = finding
	}
	cage := cells["cage-1"]
	members, unit := cage.ConsolidationUnit()
	if !unit || cage.Answer() != domain.GateBlocked || len(members) != 2 || members[0].String() != "bag-1" {
		t.Fatalf("笼那一格读回成了 %s %v unit=%v", cage.Answer(), members, unit)
	}
	if bases := cage.Bases(); len(bases) != 1 || bases[0].Cause != domain.ConsolidationMemberIdentityUnresolved.String() ||
		bases[0].Sources[0] != "hu-9" {
		t.Fatalf("笼的缘由没有原样带回：%+v", bases)
	}
	if _, judged := cage.Judgement(); judged {
		t.Fatal("单元那一格读回来带了关务判断标识")
	}
	if _, unit := cells["parcel-held"].ConsolidationUnit(); unit {
		t.Fatal("问关务问来的那一格读回成了单元")
	}
	if judgement, _ := cells["parcel-held"].Judgement(); judgement != "AADJ-parcel-held" {
		t.Fatalf("成员的关务判断标识没有原样带回：%q", judgement)
	}
}

// Covers: AT-TF-279 真库半边（ADR-0225 决定五）——单元那一格被改得与成员对不上（成员阻断、存成不受门禁）、或带上关务判断标识，那一行
// 不装回；整格那一列另行一致也救不了它。
func TestATamperedUnitCellDoesNotLoad(t *testing.T) {
	repository, transactor, pool := newMovementFacts(t)
	ctx := t.Context()
	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecordOf(t, consolidationDepartureSpec(t, "MF-CU-2")))
	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecordOf(t, consolidationDepartureSpec(t, "MF-CU-3")))

	for fact, rewrite := range map[string]string{
		"MF-CU-2": `UPDATE transport_fulfillment.transport_movement_fact
		               SET gate_reading_detail = jsonb_set(gate_reading_detail, '{findings}',
		                   (SELECT jsonb_agg(CASE WHEN f ->> 'object' = 'bag-1' THEN jsonb_set(f, '{answer}', '"NOT_GUARDED"') ELSE f END)
		                      FROM jsonb_array_elements(gate_reading_detail -> 'findings') AS f))
		             WHERE fact_ref = 'MF-CU-2'`,
		"MF-CU-3": `UPDATE transport_fulfillment.transport_movement_fact
		               SET gate_reading_detail = jsonb_set(gate_reading_detail, '{findings}',
		                   (SELECT jsonb_agg(CASE WHEN f ->> 'object' = 'bag-1' THEN jsonb_set(f, '{judgement}', '"AADJ-bag-1"') ELSE f END)
		                      FROM jsonb_array_elements(gate_reading_detail -> 'findings') AS f))
		             WHERE fact_ref = 'MF-CU-3'`,
	} {
		if tag, err := pool.Exec(ctx, rewrite); err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("改写 %s：%v rows=%d", fact, err, tag.RowsAffected())
		}
		if _, _, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", fact, "v1")); err == nil {
			t.Fatalf("%s：被改过的单元那一格装回来了", fact)
		}
	}
}
