package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证实际履约补形成的库面（ADR-0231 决定一、五、六；迁移 0048）：采用与换版的门铃与各自那一笔同落、重发不出
// 第二封；按委托列段只列这份委托的、本租户的；实际履约未形成册立、刷新、了结、只列未形成的、按租户隔离，写口无事务即拒、库面约束拒的
// 一律带确定性拒绝。夹具全是合成登记，证据只记 S。

type doorbellRow struct {
	id, partitionKey string
	payload          map[string]any
}

func doorbellsOf(t *testing.T, pool *pgxpool.Pool, eventType string) []doorbellRow {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT event_id, partition_key, payload FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 ORDER BY event_id`, eventType)
	if err != nil {
		t.Fatalf("读门铃：%v", err)
	}
	defer rows.Close()
	var found []doorbellRow
	for rows.Next() {
		var row doorbellRow
		var raw []byte
		if err := rows.Scan(&row.id, &row.partitionKey, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &row.payload); err != nil {
			t.Fatal(err)
		}
		found = append(found, row)
	}
	return found
}

// Covers: AT-TF-300 的库面一半（S，ADR-0231 决定一）——采用成立的那一笔交出恰一封门铃：载荷只带租户、段与委托，分区取「租户/履约段」；同段
// 原样再交答已有采用、不再交；按委托列出采用了它的各段，别的委托、别的租户的不在列。
func TestAnEstablishedAdoptionLeavesOneDoorbellAndIsListedByItsCommission(t *testing.T) {
	stores := newAdoptionStores(t)
	ctx := t.Context()
	stores.seed(t, ctx)
	mustSaveSegment(t, stores.transactor, ctx, stores.segments, segmentRecord(t, "SEG-ADOPT-2", activeMember(t, "parcel-2")))
	adopter := application.NewAdoptSegmentCommissionHandler(application.AdoptSegmentCommissionDeps{
		Adoptions: stores.adoptions, Segments: stores.segments, Commissions: stores.commissions, StartAdvancement: stores.commissions,
		Doorbell: stores.doorbell, CommissionHold: stores.commissions, Withdrawals: stores.adoptions,
		Clock: fixedAdoptionClock{at: segmentEnteredAtFixture.Add(24 * time.Hour)},
	})
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")
	adopt := func(segment string) application.CommissionAdoptionOutcome {
		t.Helper()
		var result application.CommissionAdoptionResult
		mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
			var err error
			result, err = adopter.Adopt(txCtx, application.AdoptSegmentCommissionCommand{TenantID: tenant, Segment: segment, Commission: "commission-1", Provider: "partner-1"})
			return err
		})
		return result.Outcome()
	}
	if outcome := adopt("SEG-ADOPT"); outcome != application.CommissionAdopted {
		t.Fatalf("采用 = %s", outcome)
	}
	if outcome := adopt("SEG-ADOPT"); outcome != application.CommissionAdoptionExisting {
		t.Fatalf("原样再交 = %s", outcome)
	}
	bells := doorbellsOf(t, stores.pool, adapter.SegmentCommissionAdoptionRegisteredEventType)
	if len(bells) != 1 || bells[0].partitionKey != "tenant-1/fulfillment-segment/SEG-ADOPT/segment-commission-adoption" ||
		bells[0].payload["tenantId"] != "tenant-1" || bells[0].payload["segment"] != "SEG-ADOPT" ||
		bells[0].payload["commission"] != "commission-1" || len(bells[0].payload) != 3 {
		t.Fatalf("门铃 = %+v", bells)
	}
	if outcome := adopt("SEG-ADOPT-2"); outcome != application.CommissionAdopted {
		t.Fatalf("第二段采用 = %s", outcome)
	}

	listed, err := stores.adoptions.ListByCommission(ctx, ports.TransportCommissionKey{
		TenantID: tenant, Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-1"),
	})
	if err != nil || len(listed) != 2 || listed[0].Key.Segment.String() != "SEG-ADOPT" || listed[1].Key.Segment.String() != "SEG-ADOPT-2" ||
		listed[0].Adoption.Provider().String() != "partner-1" {
		t.Fatalf("按委托列段 = %+v %v", listed, err)
	}
	for name, key := range map[string]ports.TransportCommissionKey{
		"别的委托": {TenantID: tenant, Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-2")},
		"别的租户": {TenantID: deliveryValue(t, domain.NewTenantID, "tenant-2"), Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-1")},
	} {
		if elsewhere, err := stores.adoptions.ListByCommission(ctx, key); err != nil || len(elsewhere) != 0 {
			t.Fatalf("%s 列出了 %+v %v", name, elsewhere, err)
		}
	}
}

// Covers: AT-TF-301 的库面一半（S，ADR-0231 决定五）——换版门铃：同一次换版重交不出第二封，下一次换版另一封，两封同在「租户/运输委托」那一区；
// 载荷只带租户、委托与号。
func TestEachCommissionRevisionLeavesOneDoorbellInTheCommissionsPartition(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := adapter.NewOutboxCommissionAgreementRevisionHandoff(db, store, tfHandoffClock{at: commissionAt.Add(72 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	record := submittedCommission(t, "tenant-a", "commission-rung")
	revised, second := revisionOf(t, record.Commission, "v2", "SYN-DECISION-REV-RING-1", commissionAt)
	_, third := revisionOf(t, revised, "v3", "SYN-DECISION-REV-RING-2", commissionAt)
	for _, revision := range []domain.CommissionAgreementRevision{second, second, third} {
		mustWithinDeliveryTransaction(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
			return handoff.HandOffCommissionAgreementRevision(txCtx, ports.CommissionAgreementRevisedIntent{
				Record: ports.CommissionAgreementRevisionRecord{Key: record.Key, Revision: revision},
			})
		})
	}
	bells := doorbellsOf(t, pool, adapter.CommissionAgreementRevisedEventType)
	if len(bells) != 2 {
		t.Fatalf("门铃 %d 封, want 两次换版各一封", len(bells))
	}
	revisions := map[float64]bool{}
	for _, bell := range bells {
		if bell.partitionKey != "tenant-a/transport-commission/commission-rung/agreement-revision" ||
			bell.payload["tenantId"] != "tenant-a" || bell.payload["commission"] != "commission-rung" || len(bell.payload) != 3 {
			t.Fatalf("门铃 = %+v", bell)
		}
		revisions[bell.payload["revision"].(float64)] = true
	}
	if !revisions[2] || !revisions[3] {
		t.Fatalf("门铃的号 = %v", revisions)
	}
}

func unformedKey(t *testing.T, tenant, segment, object string) ports.ActualFulfillmentUnformedKey {
	t.Helper()
	return ports.ActualFulfillmentUnformedKey{
		TenantID: deliveryValue(t, domain.NewTenantID, tenant),
		Segment:  deliveryValue(t, domain.NewFulfillmentSegmentReference, segment),
		Object:   deliveryValue(t, domain.NewCarriedObjectReference, object),
	}
}

func unformedEntry(t *testing.T, key ports.ActualFulfillmentUnformedKey, endedAt time.Time, reason ports.ActualFulfillmentUnformedReason) ports.ActualFulfillmentUnformedEntry {
	t.Helper()
	entry := ports.ActualFulfillmentUnformedEntry{
		Key:        key,
		Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-1"),
		Revision:   1,
		EndedAt:    endedAt,
		Reason:     reason,
	}
	if reason != ports.UnformedProcurementContextMissing {
		agreement, err := domain.NewSupplierAgreementVersion("agreement-1", "v1")
		if err != nil {
			t.Fatal(err)
		}
		entry.Agreement = agreement
	}
	return entry
}

// Covers: AT-TF-302 的库面一半、AT-TF-304 的按租户隔离（S，ADR-0231 决定六）——未形成册：立一条、再答刷新（首次答复时刻不动）；了结为已形成或不计费之后不上列；
// 册上没有的了结什么也不写；已形成的再答未形成又回到列上。列表只列本租户未形成的、参与终点最早的在前。
func TestTheUnformedRegisterTracksAndListsOnlyWhatIsStillUnformed(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	register, err := adapter.NewActualFulfillmentUnformeds(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	ended := at.Add(-48 * time.Hour)
	within := func(write func(context.Context) error) {
		t.Helper()
		mustWithinDeliveryTransaction(t, db.Transactor(), ctx, write)
	}
	early := unformedKey(t, "tenant-1", "SEG-A", "parcel-early")
	late := unformedKey(t, "tenant-1", "SEG-A", "parcel-late")
	settled := unformedKey(t, "tenant-1", "SEG-B", "parcel-settled")
	foreign := unformedKey(t, "tenant-2", "SEG-A", "parcel-early")
	within(func(txCtx context.Context) error {
		for _, entry := range []ports.ActualFulfillmentUnformedEntry{
			unformedEntry(t, late, ended.Add(time.Hour), ports.UnformedAgreementTerminated),
			unformedEntry(t, early, ended, ports.UnformedNotConfigured),
			unformedEntry(t, settled, ended, ports.UnformedProcurementContextMissing),
			unformedEntry(t, foreign, ended, ports.UnformedNotConfigured),
		} {
			if err := register.RecordUnformed(txCtx, entry, at); err != nil {
				return err
			}
		}
		refreshed := unformedEntry(t, early, ended, ports.UnformedAgreementNotEffective)
		refreshed.Revision = 2
		return register.RecordUnformed(txCtx, refreshed, at.Add(time.Hour))
	})
	got, found, err := register.FindByKey(ctx, early)
	if err != nil || !found || got.Entry.Reason != ports.UnformedAgreementNotEffective || got.Entry.Revision != 2 ||
		!got.FirstAnsweredAt.Equal(at) || !got.LastAnsweredAt.Equal(at.Add(time.Hour)) || got.Entry.Agreement.Version() != "v1" {
		t.Fatalf("刷新之后 = %+v %v %v", got, found, err)
	}

	occurrence := ports.ChargeOccurrenceKey{
		TenantID:   settled.TenantID,
		Occurrence: deliveryValue(t, domain.NewChargeOccurrenceReference, "TCO-0123456789abcdef0123456789abcdef"),
		Validity:   domain.FirstOccurrenceValidity(),
	}
	within(func(txCtx context.Context) error {
		if err := register.SettleFormed(txCtx, settled, occurrence, at.Add(2*time.Hour)); err != nil {
			return err
		}
		return register.SettleNotCharged(txCtx, unformedKey(t, "tenant-1", "SEG-C", "parcel-never"),
			deliveryValue(t, domain.NewAgreementSnapshotReference, "agreement-1/v1"), at)
	})
	if row, _, _ := register.FindByKey(ctx, settled); row.State != ports.ActualFulfillmentUnformedFormed || row.Occurrence != occurrence {
		t.Fatalf("了结为已形成 = %+v", row)
	}
	if _, found, _ := register.FindByKey(ctx, unformedKey(t, "tenant-1", "SEG-C", "parcel-never")); found {
		t.Fatal("册上没有的那一件被了结出了一行")
	}

	listed, err := register.ListActualFulfillmentUnformed(ctx, early.TenantID, 50)
	if err != nil || len(listed) != 2 || listed[0].Entry.Key != early || listed[1].Entry.Key != late {
		t.Fatalf("列表 = %+v %v", listed, err)
	}

	within(func(txCtx context.Context) error {
		return register.RecordUnformed(txCtx, unformedEntry(t, settled, ended, ports.UnformedAgreementTerminated), at.Add(3*time.Hour))
	})
	if row, _, _ := register.FindByKey(ctx, settled); row.State != ports.ActualFulfillmentUnformed || row.Occurrence != (ports.ChargeOccurrenceKey{}) {
		t.Fatalf("已形成之后再答未形成 = %+v", row)
	}
}

// Covers: AT-TF-303 的存储半边——写口的确定性拒绝：无环境事务、库面约束不让写（问过的那一版与那一格对不上）都带
// ErrActualFulfillmentUnformedWriteRefused，补形成据此按缺陷处置、不当成此刻写不动去重投。
func TestTheUnformedRegisterRefusesWhatItCannotHold(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	register, err := adapter.NewActualFulfillmentUnformeds(db)
	if err != nil {
		t.Fatal(err)
	}
	key := unformedKey(t, "tenant-1", "SEG-A", "parcel-1")
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	for name, write := range map[string]func() error{
		"RecordUnformed": func() error {
			return register.RecordUnformed(t.Context(), unformedEntry(t, key, at, ports.UnformedNotConfigured), at)
		},
		"SettleFormed": func() error { return register.SettleFormed(t.Context(), key, ports.ChargeOccurrenceKey{}, at) },
		"SettleNotCharged": func() error {
			return register.SettleNotCharged(t.Context(), key, deliveryValue(t, domain.NewAgreementSnapshotReference, "agreement-1/v1"), at)
		},
	} {
		if err := write(); !errors.Is(err, bentopg.ErrTransactionRequired) || !errors.Is(err, ports.ErrActualFulfillmentUnformedWriteRefused) {
			t.Errorf("%s 无事务应返回 ErrTransactionRequired 并带确定性拒绝，实得：%v", name, err)
		}
	}

	unasked := unformedEntry(t, key, at, ports.UnformedNotConfigured)
	unasked.Agreement = domain.SupplierAgreementVersion{}
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return register.RecordUnformed(txCtx, unasked, at)
	})
	if !errors.Is(err, ports.ErrActualFulfillmentUnformedWriteRefused) {
		t.Fatalf("未配置却没记问的那一版 = %v, want 确定性拒绝", err)
	}
}

// Covers: AT-TF-303 的存储半边——门铃的交出口无环境事务即拒：门铃要与采用连线、换版同一笔落，事务外入队
// 就是一封与采用或换版脱了钩的门铃。
func TestTheCatchUpDoorbellsRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	clock := tfHandoffClock{at: commissionAt}
	adoptionHandoff, err := adapter.NewOutboxSegmentCommissionAdoptionHandoff(db, store, clock)
	if err != nil {
		t.Fatal(err)
	}
	revisionHandoff, err := adapter.NewOutboxCommissionAgreementRevisionHandoff(db, store, clock)
	if err != nil {
		t.Fatal(err)
	}
	record := submittedCommission(t, "tenant-a", "commission-outside-ring")
	_, revision := revisionOf(t, record.Commission, "v2", "SYN-DECISION-REV-OUTSIDE", commissionAt)
	adoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID:   record.Key.TenantID,
		Segment:    deliveryValue(t, domain.NewFulfillmentSegmentReference, "SEG-OUTSIDE"),
		Commission: record.Key.Commission,
		Provider:   deliveryValue(t, domain.NewServiceProviderReference, "partner-1"),
		StartBasis: deliveryValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/outside"),
		StartedAt:  commissionAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func() error{
		"HandOffSegmentCommissionAdoption": func() error {
			return adoptionHandoff.HandOffSegmentCommissionAdoption(t.Context(), ports.SegmentCommissionAdoptedIntent{Record: ports.SegmentCommissionAdoptionRecord{
				Key:      ports.SegmentCommissionAdoptionKey{TenantID: record.Key.TenantID, Segment: adoption.Segment()},
				Adoption: adoption, RecordedAt: commissionAt,
			}})
		},
		"HandOffCommissionAgreementRevision": func() error {
			return revisionHandoff.HandOffCommissionAgreementRevision(t.Context(), ports.CommissionAgreementRevisedIntent{
				Record: ports.CommissionAgreementRevisionRecord{Key: record.Key, Revision: revision},
			})
		},
	} {
		if err := write(); !errors.Is(err, bentopg.ErrTransactionRequired) {
			t.Errorf("%s 无事务应返回 ErrTransactionRequired，实得：%v", name, err)
		}
	}
}

// Covers: AT-TF-300 的存储半边（ADR-0231 决定一）——同一段两次采用同一份委托（解除采用之后再采用）各交一封门铃：信封 ID 带这一次采用的
// 登记时刻，第二封不被当成第一封的重发吞掉；同一次采用重交（含从册上读回、时刻只到微秒的那一份）落回同一个 ID、不多一封。
func TestEachAdoptionOfTheSameSegmentAndCommissionRingsItsOwnDoorbell(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := adapter.NewOutboxSegmentCommissionAdoptionHandoff(db, store, tfHandoffClock{at: commissionAt})
	if err != nil {
		t.Fatal(err)
	}
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-a")
	segment := deliveryValue(t, domain.NewFulfillmentSegmentReference, "SEG-READOPTED")
	adoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID:   tenant,
		Segment:    segment,
		Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-readopted"),
		Provider:   deliveryValue(t, domain.NewServiceProviderReference, "partner-1"),
		StartBasis: deliveryValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/readopted"),
		StartedAt:  commissionAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := commissionAt.Add(time.Hour).Add(123 * time.Nanosecond)
	for _, recordedAt := range []time.Time{first, first.Truncate(time.Microsecond), first.Add(24 * time.Hour)} {
		mustWithinDeliveryTransaction(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
			return handoff.HandOffSegmentCommissionAdoption(txCtx, ports.SegmentCommissionAdoptedIntent{Record: ports.SegmentCommissionAdoptionRecord{
				Key:      ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: segment},
				Adoption: adoption, RecordedAt: recordedAt,
			}})
		})
	}
	bells := doorbellsOf(t, pool, adapter.SegmentCommissionAdoptionRegisteredEventType)
	if len(bells) != 2 || bells[0].id == bells[1].id {
		t.Fatalf("门铃 = %+v, want 两次采用各一封、同一次重交不多一封", bells)
	}
}

// Covers: AT-TF-303 的存储半边（ADR-0231 决定二、六）——缺陷那一格往返：不带号与问的那一版、带停在哪里；库面只认缺陷一格带它、别的格不带，
// 号只在缺陷一格可缺。
func TestTheUnformedRegisterKeepsAFormationDefect(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	register, err := adapter.NewActualFulfillmentUnformeds(db)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	key := unformedKey(t, "tenant-1", "SEG-DEFECT", "parcel-defect")
	defect := ports.ActualFulfillmentUnformedEntry{
		Key: key, Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-1"),
		EndedAt: at.Add(-time.Hour), Reason: ports.UnformedFormationDefect, Defect: "AMBIGUOUS_JOURNEY",
	}
	mustWithinDeliveryTransaction(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		return register.RecordUnformed(txCtx, defect, at)
	})
	got, found, err := register.FindByKey(t.Context(), key)
	if err != nil || !found || got.Entry.Reason != ports.UnformedFormationDefect || got.Entry.Defect != "AMBIGUOUS_JOURNEY" ||
		got.Entry.Revision != 0 || got.Entry.Agreement != (domain.SupplierAgreementVersion{}) {
		t.Fatalf("缺陷那一条读回 = %+v %v %v", got, found, err)
	}
	if listed, err := register.ListActualFulfillmentUnformed(t.Context(), key.TenantID, 10); err != nil || len(listed) != 1 {
		t.Fatalf("缺陷那一条不上列：%+v %v", listed, err)
	}

	blank := defect
	blank.Defect = ""
	defectElsewhere := unformedEntry(t, key, at, ports.UnformedNotConfigured)
	defectElsewhere.Defect = "AMBIGUOUS_JOURNEY"
	revisionless := unformedEntry(t, key, at, ports.UnformedNotConfigured)
	revisionless.Revision = 0
	for name, entry := range map[string]ports.ActualFulfillmentUnformedEntry{
		"缺陷不带停在哪里": blank, "别的格带缺陷": defectElsewhere, "别的格不带号": revisionless,
	} {
		err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			return register.RecordUnformed(txCtx, entry, at)
		})
		if !errors.Is(err, ports.ErrActualFulfillmentUnformedWriteRefused) {
			t.Fatalf("%s = %v, want 库面确定性拒绝", name, err)
		}
	}
}
