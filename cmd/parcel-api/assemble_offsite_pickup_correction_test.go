package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: 揽收更正口的第二参是真编排（票 tf-segment-lifecycle-closure/08）——首登口与更正口各自装配、
// 共用同一册：首登经控制事实那一格落 v1，更正经本格在真实 PostgreSQL 上落 v2 回指 v1、原行不动、
// 按键读回当前版是 v2；意图走真 Outbox 入了第二份（更正版本带版本段的 ID）；重放走已有版本，证首笔
// 事务真的提交了。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredPickupCorrectionLandsANewVersionAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	correction, err := buildOffsitePickupCorrectionOrchestration(db)
	if err != nil {
		t.Fatalf("装配揽收更正编排：%v", err)
	}
	registrations, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		t.Fatalf("揽收登记册读面：%v", err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-8")
	occurredAt := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)

	registered, err := controlFacts.pickupRegistration.Register(t.Context(), tfapp.RegisterOffsitePickupCommand{
		TenantID:   tenant,
		Object:     "SYN-PARCEL-8",
		Task:       "SYN-PICKUP-TASK-8",
		Attempt:    "SYN-ATTEMPT-8",
		Place:      "SYN-DOOR-8",
		Control:    "SYN-TRANSPORT-CONTROL-8",
		ExecutedBy: "SYN-COURIER-8",
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("首登：%v", err)
	}
	if got := registered.Outcome(); got != tfapp.PickupRegistered {
		t.Fatalf("outcome = %v, want PICKUP_REGISTERED", got)
	}
	first, _ := registered.Record()

	command := tfapp.CorrectOffsitePickupCommand{
		TenantID:           tenant,
		Object:             "SYN-PARCEL-8",
		Attempt:            "SYN-ATTEMPT-8",
		PredecessorVersion: first.Pickup.Version().String(),
		Place:              "SYN-DOOR-8-RECHECK",
		Control:            "SYN-TRANSPORT-CONTROL-8-RECHECK",
		ExecutedBy:         "SYN-COURIER-9",
		OccurredAt:         occurredAt.Add(-time.Hour),
		CorrectedAt:        time.Now().UTC().Add(time.Minute),
	}
	corrected, err := correction.Correct(t.Context(), command)
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	if got := corrected.Outcome(); got != tfapp.PickupCorrected {
		t.Fatalf("outcome = %v, want PICKUP_CORRECTED", got)
	}
	if ref := corrected.PickupHandoffReference(); ref != "" {
		t.Fatalf("意图交付走真 Outbox 应当成功，却留了续办引用 %q", ref)
	}
	record, has := corrected.Record()
	if !has {
		t.Fatal("更正成功却没带回登记")
	}
	if predecessor, ok := record.Pickup.Corrects(); !ok || predecessor != first.Pickup.Version() {
		t.Fatalf("corrects = (%q, %v)，版本链没回指前版 %q", predecessor.String(), ok, first.Pickup.Version())
	}
	if record.Pickup.Version() == first.Pickup.Version() {
		t.Fatal("更正沿用了原版本号")
	}

	current, found, err := registrations.FindByKey(t.Context(), tfports.OffsitePickupKey{
		TenantID: tenant,
		Object:   mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-8"),
		Attempt:  mustValue(t, tfdomain.NewAttemptReference, "SYN-ATTEMPT-8"),
	})
	if err != nil || !found {
		t.Fatalf("按键读回当前版：found=%v err=%v", found, err)
	}
	if current.Pickup.Version() != record.Pickup.Version() || current.Pickup.Control().String() != "SYN-TRANSPORT-CONTROL-8-RECHECK" {
		t.Fatalf("当前版 = %+v，want 更正版本", current.Pickup)
	}

	var rows int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.offsite_pickup
		  WHERE tenant_id = $1 AND object_ref = 'SYN-PARCEL-8' AND attempt_ref = 'SYN-ATTEMPT-8'`,
		tenant.String()).Scan(&rows); err != nil {
		t.Fatalf("数版本行：%v", err)
	}
	if rows != 2 {
		t.Fatalf("同键行数 = %d, want 2（原行不动，更正是新行）", rows)
	}

	correctionEventID := tenant.String() + "/SYN-PARCEL-8/SYN-ATTEMPT-8/" + record.Pickup.Version().String() + "/offsite-pickup-registration"
	var enqueued int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		correctionEventID).Scan(&enqueued); err != nil {
		t.Fatalf("数 Outbox：%v", err)
	}
	if enqueued != 1 {
		t.Fatalf("更正版本意图入队 %d 份, want 1——PS 收不到新版本", enqueued)
	}

	replay, err := correction.Correct(t.Context(), command)
	if err != nil {
		t.Fatalf("重放更正：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.PickupExistingVersion {
		t.Fatalf("outcome = %v, want EXISTING_VERSION——重放没走已有版本，首笔事务没有提交", got)
	}

	// 裁决点名的拒绝格（无前版、早于被更正版本的登记时刻、沿用已有版本号）在真库装配上同样成立，且都不落行、不入队。
	t.Run("a correction without a registered predecessor is not accepted", func(t *testing.T) {
		orphan := command
		orphan.Object = "SYN-PARCEL-NEVER-REGISTERED"
		result, err := correction.Correct(t.Context(), orphan)
		if err != nil {
			t.Fatalf("更正无中生有：%v", err)
		}
		if got := result.Outcome(); got != tfapp.PickupRegistrationNotAccepted {
			t.Fatalf("outcome = %v, want SOURCE_NOT_ACCEPTED（更正不出无中生有的揽收）", got)
		}
		if rows := pickupRowsFor(t, pool, tenant.String(), "SYN-PARCEL-NEVER-REGISTERED"); rows != 0 {
			t.Fatalf("无中生有的更正落了 %d 行", rows)
		}
	})

	t.Run("a correction dated before the registration it corrects is not accepted", func(t *testing.T) {
		early := command
		early.PredecessorVersion = record.Pickup.Version().String()
		early.CorrectedAt = current.RecordedAt.Add(-time.Second)
		result, err := correction.Correct(t.Context(), early)
		if err != nil {
			t.Fatalf("早于登记的更正：%v", err)
		}
		if got := result.Outcome(); got != tfapp.PickupRegistrationNotAccepted {
			t.Fatalf("outcome = %v, want SOURCE_NOT_ACCEPTED（更正时刻不得早于被更正版本的登记时刻）", got)
		}
		if rows := pickupRowsFor(t, pool, tenant.String(), "SYN-PARCEL-8"); rows != 2 {
			t.Fatalf("被拒的更正落了行：同键 %d 行, want 2", rows)
		}
	})

	t.Run("reusing an existing version number under the same key is refused by the primary key", func(t *testing.T) {
		duplicate := current
		duplicate.ContentDigest = "SYN-DIGEST-DUPLICATE"
		var outcome tfports.OffsitePickupSaveOutcome
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var saveErr error
			outcome, saveErr = registrations.Save(txCtx, duplicate)
			return saveErr
		}); err != nil {
			t.Fatalf("重用版本号：%v", err)
		}
		if outcome != tfports.OffsitePickupAlreadyRegistered {
			t.Fatalf("outcome = %v, want ALREADY_REGISTERED（沿用已有版本号即覆盖，库面拒）", outcome)
		}
	})
}

func pickupRowsFor(t *testing.T, pool *pgxpool.Pool, tenant, object string) int {
	t.Helper()
	var rows int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.offsite_pickup WHERE tenant_id = $1 AND object_ref = $2`,
		tenant, object).Scan(&rows); err != nil {
		t.Fatalf("数版本行：%v", err)
	}
	return rows
}

// Covers: 揽收更正口的生产装配在段上重派生参与（ADR-0112 决定一、二；票 operator-channel/40）——首登经控制
// 事实那一格带段落 v1，经本格更正落 v2 之后，段上该对象的当前参与凭 `OFFSITE-PICKUP/<v2>` 入场、回指
// `OFFSITE-PICKUP/<v1>`、起点是更正后的发生时刻、计划段沿用；首登那一行一字不动且不再是当前；段那一半两格都空。
//
// 这一条钉的是装配点不是重派生门：门在 tfpostgres 的真库用例里早已走通，但 rederiveFulfillmentParticipation
// 对缺席的段登记册既不重派生也不报失败，装配点漏接时答复与「重派生成功」逐格同形，只有读回段才看得出来。
// 承运主体判断照旧只有首版——本格不交判断登记册（票 tf-segment-lifecycle-closure/10 红线），更正碰不到它。
// 测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredPickupCorrectionSupersedesTheParticipationOnTheSegmentAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	correction, err := buildOffsitePickupCorrectionOrchestration(db)
	if err != nil {
		t.Fatalf("装配揽收更正编排：%v", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatalf("段登记册读面：%v", err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-40")
	object := mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-40")
	segmentKey := tfports.FulfillmentSegmentKey{
		TenantID: tenant,
		Segment:  mustValue(t, tfdomain.NewFulfillmentSegmentReference, "SYN-SEGMENT-40"),
	}
	occurredAt := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	correctedOccurredAt := occurredAt.Add(45 * time.Minute)

	registered, err := controlFacts.pickupRegistration.Register(t.Context(), tfapp.RegisterOffsitePickupCommand{
		TenantID:       tenant,
		Object:         "SYN-PARCEL-40",
		Task:           "SYN-PICKUP-TASK-40",
		Attempt:        "SYN-ATTEMPT-40",
		Place:          "SYN-DOOR-40",
		Control:        "SYN-TRANSPORT-CONTROL-40",
		ExecutedBy:     "SYN-COURIER-40",
		OccurredAt:     occurredAt,
		Segment:        "SYN-SEGMENT-40",
		PlannedSegment: "SYN-PLANNED-40",
	})
	if err != nil {
		t.Fatalf("首登：%v", err)
	}
	if got := registered.Outcome(); got != tfapp.PickupRegistered {
		t.Fatalf("outcome = %v, want PICKUP_REGISTERED", got)
	}
	if ref, refusal := registered.SegmentContinuationReference(), registered.SegmentEntryRefusal(); ref != "" || refusal != tfapp.SegmentEntryRefusalNone {
		t.Fatalf("首登进段应当成功：debt=%q refusal=%s", ref, refusal)
	}
	first, _ := registered.Record()
	rootBasis := "OFFSITE-PICKUP/" + first.Pickup.Version().String()

	corrected, err := correction.Correct(t.Context(), tfapp.CorrectOffsitePickupCommand{
		TenantID:           tenant,
		Object:             "SYN-PARCEL-40",
		Attempt:            "SYN-ATTEMPT-40",
		PredecessorVersion: first.Pickup.Version().String(),
		Place:              "SYN-DOOR-40-RECHECK",
		Control:            "SYN-TRANSPORT-CONTROL-40-RECHECK",
		ExecutedBy:         "SYN-COURIER-40",
		OccurredAt:         correctedOccurredAt,
		CorrectedAt:        time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	if got := corrected.Outcome(); got != tfapp.PickupCorrected {
		t.Fatalf("outcome = %v, want PICKUP_CORRECTED", got)
	}
	if ref, refusal := corrected.SegmentContinuationReference(), corrected.SegmentEntryRefusal(); ref != "" || refusal != tfapp.SegmentEntryRefusalNone {
		t.Fatalf("进过段的揽收被更正，重派生应当成功：debt=%q refusal=%s", ref, refusal)
	}
	second, _ := corrected.Record()
	successorBasis := "OFFSITE-PICKUP/" + second.Pickup.Version().String()

	record, found, err := segments.FindByKey(t.Context(), segmentKey)
	if err != nil || !found {
		t.Fatalf("读回段：found=%v err=%v", found, err)
	}
	current, present := record.Segment.ParticipationFor(object)
	if !present || current.EntryBasis().String() != successorBasis {
		t.Fatalf("段上当前参与 = (present=%v, basis=%s)，want %s——生产装配的揽收更正没有在段上重派生",
			present, current.EntryBasis(), successorBasis)
	}
	if supersedes, chained := current.Supersedes(); !chained || supersedes.String() != rootBasis {
		t.Fatalf("替代版本没有回指首登：chained=%v supersedes=%s, want %s", chained, supersedes, rootBasis)
	}
	if !current.EnteredAt().Equal(correctedOccurredAt) || !current.Active() || current.EntryKind() != tfdomain.EnteredByOffsitePickup {
		t.Fatalf("替代版本的起点、在场或种类不对：enteredAt=%v want %v, active=%v, kind=%v",
			current.EnteredAt(), correctedOccurredAt, current.Active(), current.EntryKind())
	}
	if planned, has := current.PlannedSegment(); !has || planned.String() != "SYN-PLANNED-40" {
		t.Fatalf("替代版本的计划段 = (%q, %v)，want 沿用 SYN-PLANNED-40", planned.String(), has)
	}
	history := record.Segment.ParticipationHistory(object)
	if len(history) != 2 || history[0].EntryBasis().String() != rootBasis || !history[0].Superseded() || history[0].Active() {
		t.Fatalf("链形不对：len=%d，首登须在链首且已被替代", len(history))
	}

	var rows int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = $1 AND segment_ref = 'SYN-SEGMENT-40' AND object_ref = 'SYN-PARCEL-40'`,
		tenant.String()).Scan(&rows); err != nil {
		t.Fatalf("数参与行：%v", err)
	}
	if rows != 2 {
		t.Fatalf("库里参与行 = %d，want 2（原参与不动，替代是新行）", rows)
	}
	var rootSupersedes *string
	var rootEnteredAt time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT supersedes_entry_basis, entered_at FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = $1 AND segment_ref = 'SYN-SEGMENT-40' AND object_ref = 'SYN-PARCEL-40' AND entry_basis = $2`,
		tenant.String(), rootBasis).Scan(&rootSupersedes, &rootEnteredAt); err != nil {
		t.Fatalf("读首登那一行：%v", err)
	}
	if rootSupersedes != nil || !rootEnteredAt.Equal(occurredAt) {
		t.Fatalf("首登那一行被动过：supersedes=%v enteredAt=%v want %v", rootSupersedes, rootEnteredAt, occurredAt)
	}

	assertPendingFirstJudgment(t, db, segmentKey)
}

// Covers: 从没进过段的揽收被生产装配更正时，段那一半如实答 `NO_PARTICIPATION_TO_REDERIVE`（票
// tf-segment-lifecycle-closure/10 裁决 2 的领域正当拒绝格），不是欠账，也不是两格皆空——两格皆空在调用方
// 眼里就是「重派生成功」。库里也不凭空长出参与。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredPickupCorrectionOfAnObjectOutsideAnySegmentAnswersNoParticipationToRederive(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	correction, err := buildOffsitePickupCorrectionOrchestration(db)
	if err != nil {
		t.Fatalf("装配揽收更正编排：%v", err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-41")
	occurredAt := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)

	registered, err := controlFacts.pickupRegistration.Register(t.Context(), tfapp.RegisterOffsitePickupCommand{
		TenantID:   tenant,
		Object:     "SYN-PARCEL-41",
		Task:       "SYN-PICKUP-TASK-41",
		Attempt:    "SYN-ATTEMPT-41",
		Place:      "SYN-DOOR-41",
		Control:    "SYN-TRANSPORT-CONTROL-41",
		ExecutedBy: "SYN-COURIER-41",
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("首登：%v", err)
	}
	first, has := registered.Record()
	if !has {
		t.Fatalf("首登没带回登记：outcome=%v", registered.Outcome())
	}

	corrected, err := correction.Correct(t.Context(), tfapp.CorrectOffsitePickupCommand{
		TenantID:           tenant,
		Object:             "SYN-PARCEL-41",
		Attempt:            "SYN-ATTEMPT-41",
		PredecessorVersion: first.Pickup.Version().String(),
		Place:              "SYN-DOOR-41-RECHECK",
		Control:            "SYN-TRANSPORT-CONTROL-41-RECHECK",
		ExecutedBy:         "SYN-COURIER-41",
		OccurredAt:         occurredAt.Add(30 * time.Minute),
		CorrectedAt:        time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	if got := corrected.Outcome(); got != tfapp.PickupCorrected {
		t.Fatalf("outcome = %v, want PICKUP_CORRECTED", got)
	}
	if got := corrected.SegmentEntryRefusal(); got != tfapp.SegmentEntryRefusedNoParticipationToRederive {
		t.Fatalf("refusal = %q, want NO_PARTICIPATION_TO_REDERIVE——段那一半两格皆空，与重派生成功同形", got)
	}
	if ref := corrected.SegmentContinuationReference(); ref != "" {
		t.Fatalf("无可替代是正当拒绝不是欠账：debt=%q", ref)
	}
	var rows int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.fulfillment_participation WHERE tenant_id = $1 AND object_ref = 'SYN-PARCEL-41'`,
		tenant.String()).Scan(&rows); err != nil {
		t.Fatalf("数参与行：%v", err)
	}
	if rows != 0 {
		t.Fatalf("从没进过段的对象长出了 %d 条参与", rows)
	}
}
