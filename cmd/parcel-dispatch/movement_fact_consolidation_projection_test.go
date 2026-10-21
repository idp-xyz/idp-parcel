package main

import (
	"context"
	"testing"
	"time"

	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证全程追踪把段里在控的一袋按事实那一刻的成员展开投影，走的是生产 wireDispatcher：节点作业的单元、成员
// 事实与收寄身份，运输履约的段与移动事实，全程追踪的投影都在同一个库里各自的表上。测试输入是隔离合成，只记 `S`。

const (
	consolidationBag          = "SYN-BAG-123"
	consolidationParcelA      = "SYN-PARCEL-BAG-123-A"
	consolidationParcelB      = "SYN-PARCEL-BAG-123-B"
	consolidationParcelOut    = "SYN-PARCEL-BAG-123-OUT"
	consolidationParcelLate   = "SYN-PARCEL-BAG-123-LATE"
	consolidationMoveDispatch = "SYN-MOVE-BAG-123"
	consolidationMoveBackfill = "SYN-MOVE-BAG-123-BACKFILL"
)

// seedBagAroundTheDeparture 在节点作业的册上立一袋：出发两小时前装进 A、B、一件已出袋的与一件没有收寄记录的实物，出袋那一件在
// 出发一小时前移出，另一件在出发之后才装进。A、B、出袋那一件与迟到那一件都有识别成功的收寄。
func seedBagAroundTheDeparture(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := fixture.identity.TenantID().String()
	seed := func(sql string, args ...any) {
		t.Helper()
		if _, err := fixture.pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatalf("播种：%v", err)
		}
	}
	seed(`INSERT INTO node_operations.consolidation_unit (tenant_id, unit_id, asset_ref, phase, members, snapshots, opened_source)
		 VALUES ($1, $2, 'SYN-ASSET-' || $2, 'OPEN', '[]'::jsonb, '[]'::jsonb,
		         jsonb_build_object('sourceId', 'SYN-OPEN-' || $2, 'performedBy', 'SYN-PACKER', 'evidence', 'SYN-WORK',
		                            'occurredAt', $3::timestamptz))`,
		tenant, consolidationBag, movementDepartedAt.Add(-3*time.Hour))
	fact := func(action, member string, at time.Time) {
		t.Helper()
		seed(`INSERT INTO node_operations.consolidation_fact
				(tenant_id, source_id, content_digest, action, unit_id, member_id, performed_by, evidence, occurred_at, recorded_at)
			 VALUES ($1, 'SYN-' || $2 || '-' || $4, 'syn-digest', $2, $3, $4, 'SYN-PACKER', 'SYN-WORK', $5, $5)`,
			tenant, action, consolidationBag, member, at)
	}
	receive := func(handling, parcel string) {
		t.Helper()
		receivedAt := movementDepartedAt.Add(-4 * time.Hour)
		seed(`INSERT INTO node_operations.reception
				(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers,
				 service_result_check, recorded_at)
			 VALUES ($1, 'SYN-SCAN-' || $2, 'syn-digest', 'INTAKE_FORMED',
			         jsonb_build_object('unit', $2::text, 'node', 'SYN-NODE-SHA-HUB', 'deliveredBy', 'SYN-COURIER',
			                            'evidence', 'SYN-EVIDENCE', 'version', 'SYN-INTAKE-' || $2,
			                            'association', $3::text, 'receivedAt', $4::timestamptz),
			         jsonb_build_object('unit', $2::text, 'node', 'SYN-NODE-SHA-HUB', 'kind', 'NODE_INTAKE',
			                            'basis', 'SYN-INTAKE-' || $2, 'establishedAt', $4::timestamptz),
			         '[]'::jsonb, false, '[]'::jsonb, 'CHECKED', $4)`,
			tenant, handling, parcel, receivedAt)
	}
	receive("SYN-HU-123-A", consolidationParcelA)
	receive("SYN-HU-123-B", consolidationParcelB)
	receive("SYN-HU-123-OUT", consolidationParcelOut)
	receive("SYN-HU-123-LATE", consolidationParcelLate)
	for _, member := range []string{"SYN-HU-123-A", "SYN-HU-123-B", "SYN-HU-123-OUT", "SYN-HU-123-UNRECEIVED"} {
		fact("ADD_MEMBER", member, movementDepartedAt.Add(-2*time.Hour))
	}
	fact("REMOVE_MEMBER", "SYN-HU-123-OUT", movementDepartedAt.Add(-time.Hour))
	fact("ADD_MEMBER", "SYN-HU-123-LATE", movementDepartedAt.Add(time.Minute))
}

// saveBagSegment 站在 TF 侧立出发声明的那一段：那一袋与一件散件在出发前一小时交接入段。单元交接不为成员形成参与（ADR-0263
// 决定四），段里在控的就是袋本身。
func saveBagSegment(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	segments, err := tfpostgres.NewFulfillmentSegments(fixture.db)
	if err != nil {
		t.Fatalf("构造段册：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	reference := mustTF(t, tfdomain.NewFulfillmentSegmentReference, movementSegment)
	var participations []tfdomain.RehydrateParticipationSpec
	for _, object := range []string{consolidationBag, movementParcelOnBoard} {
		participations = append(participations, tfdomain.RehydrateParticipationSpec{
			Object:     mustTF(t, tfdomain.NewCarriedObjectReference, object),
			EntryKind:  tfdomain.EnteredByTransportHandover,
			EntryBasis: mustTF(t, tfdomain.NewParticipationBasisReference, "SYN-HANDOVER/"+object),
			EnteredAt:  movementHandedOverAt,
		})
	}
	segment, err := tfdomain.RehydrateActualFulfillmentSegment(tfdomain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant, Segment: reference, Participations: participations,
	})
	if err != nil {
		t.Fatalf("段：%v", err)
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := segments.Save(txCtx, tfports.FulfillmentSegmentRecord{
			Key:        tfports.FulfillmentSegmentKey{TenantID: tenant, Segment: reference},
			Segment:    segment,
			RecordedAt: movementHandedOverAt,
		})
		if err == nil && outcome != tfports.SegmentSaved {
			t.Fatalf("段 outcome = %v", outcome)
		}
		return err
	})
}

// assertDepartedOntoBagMembers 核这条出发投到了袋里出发那一刻的两件成员身上，各一个条目、归 DEPARTED；袋本身、那一刻已出袋的、
// 出发后才装进的都没有它；没有收寄记录的那一件没有可投的追踪包裹。
func assertDepartedOntoBagMembers(t *testing.T, fixture *synVerticalFixture, fact string) {
	t.Helper()
	for _, parcel := range []string{consolidationParcelA, consolidationParcelB} {
		var departed int
		for _, entry := range projectionEntries(t, fixture, parcel) {
			if entry.Fact().Fact().String() != "movement-fact/"+parcel+"/"+fact {
				continue
			}
			departed++
			if milestone, classified := entry.Milestone(); entry.Fact().Kind().String() != "movement-departure" ||
				!classified || milestone.String() != "DEPARTED" {
				t.Fatalf("%s 的出发 = %s 归类 %q（%v），want movement-departure 归 DEPARTED", parcel, entry.Fact().Kind(), milestone, classified)
			}
			if !entry.Fact().OccurredAt().Equal(movementDepartedAt) {
				t.Fatalf("%s 的出发发生时间 = %s, want 事实的业务发生时间", parcel, entry.Fact().OccurredAt())
			}
		}
		if departed != 1 {
			t.Fatalf("%s 的出发条目 %d 个, want 1", parcel, departed)
		}
		if n := acceptedFactVersions(t, fixture, "movement-fact/"+parcel+"/"+fact); n != 1 {
			t.Fatalf("%s 这一版事实 %d 份, want 1", parcel, n)
		}
	}
	// 只数这条出发：袋凭交接进段时交接那一路也不给袋立投影，由补派生那条用例另核。
	for _, absent := range []string{consolidationBag, consolidationParcelOut, consolidationParcelLate, "SYN-HU-123-UNRECEIVED"} {
		for _, entry := range projectionEntries(t, fixture, absent) {
			if entry.Fact().Fact().String() == "movement-fact/"+absent+"/"+fact {
				t.Fatalf("%s 不该得到这条出发，却有它的条目", absent)
			}
		}
		if n := acceptedFactVersions(t, fixture, "movement-fact/"+absent+"/"+fact); n != 0 {
			t.Fatalf("%s 被派生了 %d 份", absent, n)
		}
	}
}

// Covers: AT-VE-196 真库半边、票面判据「一袋出发时，袋内每个成员在全程追踪投影上得到出发里程碑」（证据 S）——经生产登记编排登一条
// 出发，段里在控的是一袋与一件散件；生产路由一拍：袋里出发那一刻的两件成员与散件各得一份 movement-departure，归 DEPARTED；袋本身
// 不立追踪投影；出发前已出袋的、出发后才装进的不投；没有收寄记录的那一件入账不派生、不挡其余各件。同一条事实以新 ID 再投一次，
// 只答已有结果：条目与事实都不多出第二份。
func TestABagDepartureReachesEveryMemberThroughTheProductionRoute(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)
	seedBagAroundTheDeparture(t, fixture)
	saveBagSegment(t, fixture)

	// 读数由夹具按领域形成，只为让出发能登记；投影不读读数，对象取段册，成员取节点作业。
	eventID := recordDepartureThroughProduction(t, fixture, consolidationMoveDispatch)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("第一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, eventID))
	}
	if n := fixture.countInbox(t, deriveMovementFactConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
	assertDepartedOntoBagMembers(t, fixture, consolidationMoveDispatch)
	if entries := projectionEntries(t, fixture, movementParcelOnBoard); len(entries) != 1 {
		t.Fatalf("散件条目 %d 个, want 1", len(entries))
	}

	derive, inboxStore := redeliveryParts(t, fixture)
	consumer, err := deriveMovementFactConsumer(fixture.db, inboxStore, derive)
	if err != nil {
		t.Fatalf("装配移动事实那一路：%v", err)
	}
	tenant := fixture.identity.TenantID().String()
	if err := consumer.Consume(t.Context(), redeliveryEnvelope(tenant, "redeliver-bag-departure",
		veinbox.MovementFactRecordedEventType, consolidationMoveDispatch,
		`{"tenantId":"`+tenant+`","fact":"`+consolidationMoveDispatch+`","version":"v1"}`)); err != nil {
		t.Fatalf("移动事实重投：%v", err)
	}
	assertDepartedOntoBagMembers(t, fixture, consolidationMoveDispatch)
}

// Covers: AT-VE-196 真库半边（补派生）——出发先于段登记，一袋随后经生产登记编排`已交接`进段、立起段；交接那一封上的补派生把出发
// 补到袋里出发那一刻的成员身上，袋本身不补。
// Covers: AT-VE-199 真库半边（交接经生产路由一拍）——同一封交接信上的交接投影那一路入账，袋上一个条目也没有。
func TestABagHandedIntoTheSegmentAfterItsDepartureBackfillsEveryMember(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)
	seedBagAroundTheDeparture(t, fixture)

	factEvent := recordDepartureThroughProduction(t, fixture, consolidationMoveBackfill)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("出发那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, factEvent))
	}
	if entries := projectionEntries(t, fixture, consolidationParcelA); len(entries) != 0 {
		t.Fatalf("段不在册时成员已有 %d 个条目——前提不成立", len(entries))
	}

	handoverEvent := registerHandoverOfObjectIntoMovementSegment(t, fixture, consolidationBag)
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("交接那一拍：%v", err)
	}
	if n := fixture.countInbox(t, backfillConsumerName, handoverEvent); n != 1 {
		t.Fatalf("补派生 inbox 行数 = %d, want 1；失败码 = %q", n, recordedFailureCode(t, fixture.db, handoverEvent))
	}
	assertDepartedOntoBagMembers(t, fixture, consolidationMoveBackfill)
	if n := fixture.countInbox(t, deriveHandoverConsumerName, handoverEvent); n != 1 {
		t.Fatalf("交接投影 inbox 行数 = %d, want 1；失败码 = %q", n, recordedFailureCode(t, fixture.db, handoverEvent))
	}
	if entries := projectionEntries(t, fixture, consolidationBag); len(entries) != 0 {
		t.Fatalf("袋被当成一件追踪包裹立了投影，条目 %d 个", len(entries))
	}
}
