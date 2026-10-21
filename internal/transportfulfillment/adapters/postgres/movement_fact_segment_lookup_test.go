package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证按段列移动事实（ADR-0224 决定二）：声明了这一段的每个版本都在列、同一条事实按登记先后排，
// 别的段、别的租户与没声明段的不在列；没有事实声明的段答空；一行装不回整次报错。夹具全为合成登记（S 级）。

// Covers: AT-TF-273——段 segment-1 上一条出发连同它的更正版、一条声明了段的到达都在列，次序按事实、再按登记先后（更正版的版本串
// 字面排在前版之前，照样排在后面）；声明别段的、没声明段的、别租户声明同一段的都不在列。读回的是整条事实：读数与替代关系都在。
func TestMovementFactsDeclaringASegmentAreListedInRecordingOrder(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()

	departure := movementFactRecord(t, "MF-A", "v1", domain.DepartureFact, "")
	correctedFact, err := departure.Fact.Correct(departure.Fact.Location(), movedAtDBFixture.Add(time.Minute),
		departure.Fact.Source(), segmentRef(t, domain.NewMovementFactVersion, "a-fix"), movedAtDBFixture.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	correction := ports.MovementFactRecord{
		Key:        ports.MovementFactKey{TenantID: departure.Key.TenantID, Fact: departure.Key.Fact, Version: correctedFact.Version()},
		Fact:       correctedFact,
		RecordedAt: movedAtDBFixture.Add(2 * time.Hour),
	}
	arrivalSpec := movementFactSpec(t, "MF-B", "v1", domain.ArrivalFact)
	arrivalSpec.Segment = segmentRef(t, domain.NewFulfillmentSegmentReference, "segment-1")
	elsewhereSpec := movementFactSpec(t, "MF-C", "v1", domain.DepartureFact)
	elsewhereSpec.Segment = segmentRef(t, domain.NewFulfillmentSegmentReference, "segment-2")
	foreignSpec := movementFactSpec(t, "MF-D", "v1", domain.DepartureFact)
	foreignSpec.TenantID = segmentRef(t, domain.NewTenantID, "tenant-2")
	for _, record := range []ports.MovementFactRecord{
		correction, departure,
		movementFactRecordOf(t, arrivalSpec),
		movementFactRecordOf(t, elsewhereSpec),
		movementFactRecord(t, "MF-E", "v1", domain.ArrivalFact, ""),
		movementFactRecordOf(t, foreignSpec),
	} {
		mustSaveMovementFact(t, transactor, ctx, repository, record)
	}

	listed, err := repository.FindBySegment(ctx, segmentKeyFixture(t, "tenant-1", "segment-1"))
	if err != nil {
		t.Fatalf("按段列：%v", err)
	}
	var got []string
	for _, record := range listed {
		got = append(got, record.Key.Fact.String()+"/"+record.Key.Version.String())
	}
	want := []string{"MF-A/v1", "MF-A/a-fix", "MF-B/v1"}
	if len(got) != len(want) {
		t.Fatalf("列出 %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("列出 %v, want %v", got, want)
		}
	}
	first, fix := listed[0], listed[1]
	if reading, read := first.Fact.GateReading(); !read || reading.Answer() != domain.DepartureGateBlocked {
		t.Fatalf("读数没随事实读回：%s read=%v", reading.Answer(), read)
	}
	if corrects, has := fix.Fact.Corrects(); !has || corrects.String() != "v1" {
		t.Fatalf("更正版的回指没读回：%q has=%v", corrects, has)
	}
	if !first.RecordedAt.Equal(departure.RecordedAt) || first.Key.TenantID != departure.Key.TenantID {
		t.Fatalf("登记时刻或租户没原样读回：%s %s", first.RecordedAt, first.Key.TenantID)
	}
}

// Covers: AT-TF-274——没有事实声明的段答空切片、不报错；一行装不回（库面被绕过构造门写过）时整次报错，不跳过那一行。
func TestASegmentNoFactDeclaresListsEmptyAndAnUnreadableRowFailsTheListing(t *testing.T) {
	repository, transactor, pool := newMovementFacts(t)
	ctx := t.Context()
	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecord(t, "MF-A", "v1", domain.DepartureFact, ""))

	listed, err := repository.FindBySegment(ctx, segmentKeyFixture(t, "tenant-1", "segment-9"))
	if err != nil || listed == nil || len(listed) != 0 {
		t.Fatalf("没人声明的段 = %v（nil=%v）err = %v, want 空切片", listed, listed == nil, err)
	}

	// 整格与逐对象那一格的词都在形状检查之外：库面只守形状，认词是领域重建门的事。
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_movement_fact
		     (tenant_id, fact_ref, version, schedule_ref, kind, location_ref, source_ref, occurred_at, segment_ref,
		      gate_reading, gate_reading_detail, recorded_at)
		 VALUES ('tenant-1', 'MF-BROKEN', 'v1', 'schedule-1', 'DEPARTURE', 'hub-1', 'carrier-scan/v1', now(), 'segment-1',
		         'BLOCKED', '{"from": null, "to": null, "stops": [], "findings": [{"object": "parcel-1", "answer": "NONSENSE", "bases": []}]}'::jsonb,
		         now())`,
	); err != nil {
		t.Fatalf("绕过构造门写一行：%v", err)
	}
	if listed, err := repository.FindBySegment(ctx, segmentKeyFixture(t, "tenant-1", "segment-1")); err == nil {
		t.Fatalf("装不回的那一行被跳过了：列出 %d 条", len(listed))
	}
}
