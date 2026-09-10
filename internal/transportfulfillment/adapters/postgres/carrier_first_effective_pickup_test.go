package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证实际承运商首次有效收寄登记册与出向意图（lc/31 做法 4；ADR-0135 决定七）：首登 / 替代 /
// 失效三代同链往返、FindByKey 交指名那一代而不是链尾、链尾按回指派生、一对象一链与一版只被回指一次由唯一约束拦、
// CHECK 挡「待确认却带承运主体」；outbox 对待确认拒绝入队、三代各入一份且 ID 不同分区键相同。夹具全为合成登记（S 级）。

var (
	carrierPickupOccurredAtDB = time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	carrierPickupJudgedAtDB   = time.Date(2026, 9, 10, 9, 5, 0, 0, time.UTC)
)

func newCarrierPickups(t *testing.T) (*adapter.CarrierFirstEffectivePickups, bentoapp.Transactor, *bentopg.DB, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCarrierFirstEffectivePickups(db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	return repository, db.Transactor(), db, pool
}

func carrierPickupBasisDB(t *testing.T, reference, version string) domain.CarrierPickupBasis {
	t.Helper()
	basis, err := domain.NewCarrierPickupBasis(domain.TrustedChannelCallback, reference, version)
	if err != nil {
		t.Fatalf("形成依据：%v", err)
	}
	return basis
}

func formedCarrierPickupDB(t *testing.T, fact, version, object string) domain.CarrierFirstEffectivePickup {
	t.Helper()
	carrier, err := domain.NewCarrierSubject(domain.ExternalCarrierParty, "party/carrier-x")
	if err != nil {
		t.Fatalf("承运主体：%v", err)
	}
	pickup, err := domain.FormCarrierFirstEffectivePickup(domain.CarrierFirstEffectivePickupSpec{
		TenantID:   segmentRef(t, domain.NewTenantID, "tenant-1"),
		Object:     segmentRef(t, domain.NewCarriedObjectReference, object),
		Fact:       segmentRef(t, domain.NewCarrierFirstEffectivePickupReference, fact),
		Version:    segmentRef(t, domain.NewCarrierFirstEffectivePickupVersion, version),
		Carrier:    carrier,
		OccurredAt: carrierPickupOccurredAtDB,
		JudgedAt:   carrierPickupJudgedAtDB,
		Bases:      []domain.CarrierPickupBasis{carrierPickupBasisDB(t, "EXTF-1", "EXTV-1")},
	})
	if err != nil {
		t.Fatalf("形成收寄夹具：%v", err)
	}
	return pickup
}

func carrierPickupRecord(pickup domain.CarrierFirstEffectivePickup, recordedAt time.Time) ports.CarrierFirstEffectivePickupRecord {
	return ports.CarrierFirstEffectivePickupRecord{
		Key:        ports.CarrierFirstEffectivePickupKey{TenantID: pickup.TenantID(), Fact: pickup.Fact(), Version: pickup.Version()},
		Pickup:     pickup,
		RecordedAt: recordedAt,
	}
}

func mustSaveCarrierPickup(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, repository *adapter.CarrierFirstEffectivePickups, record ports.CarrierFirstEffectivePickupRecord) ports.CarrierPickupSaveOutcome {
	t.Helper()
	var outcome ports.CarrierPickupSaveOutcome
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, record)
		return err
	})
	return outcome
}

// Covers: 三代同链往返；FindByKey 交指名那一代；FindCurrentByObject 交链尾；ListByObject 按序交整链。
func TestThreeGenerationsOfOnePickupChainRoundTripAndTheNamedGenerationIsReturned(t *testing.T) {
	repository, transactor, _, _ := newCarrierPickups(t)
	ctx := t.Context()
	first := formedCarrierPickupDB(t, "CFEP-RT", "CFEV-RT1", "PCL-RT")
	if outcome := mustSaveCarrierPickup(t, transactor, ctx, repository, carrierPickupRecord(first, carrierPickupJudgedAtDB)); outcome != ports.CarrierPickupSaved {
		t.Fatalf("首登 outcome = %s", outcome)
	}
	carrier, _ := first.Carrier()
	superseding, err := first.Supersede(domain.CarrierPickupSupersession{
		Version:    segmentRef(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-RT2"),
		Carrier:    carrier,
		OccurredAt: carrierPickupOccurredAtDB.Add(10 * time.Minute),
		JudgedAt:   carrierPickupJudgedAtDB.Add(time.Hour),
		Bases:      []domain.CarrierPickupBasis{carrierPickupBasisDB(t, "EXTF-1", "EXTV-2"), carrierPickupBasisDB(t, "EXTF-9", "EXTV-1")},
	})
	if err != nil {
		t.Fatalf("替代：%v", err)
	}
	if outcome := mustSaveCarrierPickup(t, transactor, ctx, repository, carrierPickupRecord(superseding, carrierPickupJudgedAtDB.Add(time.Hour))); outcome != ports.CarrierPickupSaved {
		t.Fatalf("替代 outcome = %s", outcome)
	}
	voided, err := superseding.Void(domain.CarrierPickupVoiding{
		Version:  segmentRef(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-RT3"),
		JudgedAt: carrierPickupJudgedAtDB.Add(2 * time.Hour),
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasisDB(t, "EXTF-1", "EXTV-3")},
	})
	if err != nil {
		t.Fatalf("失效：%v", err)
	}
	if outcome := mustSaveCarrierPickup(t, transactor, ctx, repository, carrierPickupRecord(voided, carrierPickupJudgedAtDB.Add(2*time.Hour))); outcome != ports.CarrierPickupSaved {
		t.Fatalf("失效 outcome = %s", outcome)
	}

	named, found, err := repository.FindByKey(ctx, carrierPickupRecord(first, time.Time{}).Key)
	if err != nil || !found {
		t.Fatalf("按键取首代：%v %v", err, found)
	}
	if !named.Pickup.Formed() || named.Pickup.Version().String() != "CFEV-RT1" {
		t.Fatalf("FindByKey 应交指名那一代而不是链尾：%s %v", named.Pickup.Version(), named.Pickup.Result())
	}
	if at, _ := named.Pickup.OccurredAt(); !at.Equal(carrierPickupOccurredAtDB) {
		t.Fatalf("首代业务时间没有原样带回：%s", at)
	}
	if got, _ := named.Pickup.Carrier(); got != carrier {
		t.Fatalf("承运主体没有原样带回：%+v", got)
	}
	middle, _, _ := repository.FindByKey(ctx, carrierPickupRecord(superseding, time.Time{}).Key)
	if len(middle.Pickup.Bases()) != 2 || middle.Pickup.Bases()[1].Reference().String() != "EXTF-9" {
		t.Fatalf("多条依据按序带回：%+v", middle.Pickup.Bases())
	}
	if prior, _ := middle.Pickup.Supersedes(); prior.String() != "CFEV-RT1" {
		t.Fatalf("替代版本回指没有带回：%s", prior)
	}

	current, found, err := repository.FindCurrentByObject(ctx, first.TenantID(), first.Object())
	if err != nil || !found {
		t.Fatalf("取链尾：%v %v", err, found)
	}
	if !current.Pickup.Voided() || current.Pickup.Version().String() != "CFEV-RT3" {
		t.Fatalf("链尾应是失效版本：%s %v", current.Pickup.Version(), current.Pickup.Result())
	}
	chain, err := repository.ListByObject(ctx, first.TenantID(), first.Object())
	if err != nil || len(chain) != 3 || chain[0].Pickup.Version().String() != "CFEV-RT1" || chain[2].Pickup.Version().String() != "CFEV-RT3" {
		t.Fatalf("整链：%v %d", err, len(chain))
	}
	if _, found, _ := repository.FindCurrentByObject(ctx, segmentRef(t, domain.NewTenantID, "tenant-2"), first.Object()); found {
		t.Fatalf("另一租户不该看见这条链")
	}
}

// Covers: 同版本重放、同对象第二条首登、同一前版回指两次都答已登记且事务仍可用。
func TestDuplicateFirstRegistrationsAndForksAreAnsweredAsAlreadyRegistered(t *testing.T) {
	repository, transactor, _, _ := newCarrierPickups(t)
	ctx := t.Context()
	first := formedCarrierPickupDB(t, "CFEP-DUP", "CFEV-DUP1", "PCL-DUP")
	mustSaveCarrierPickup(t, transactor, ctx, repository, carrierPickupRecord(first, carrierPickupJudgedAtDB))

	if outcome := mustSaveCarrierPickup(t, transactor, ctx, repository, carrierPickupRecord(first, carrierPickupJudgedAtDB)); outcome != ports.CarrierPickupAlreadyRegistered {
		t.Fatalf("同版本重放 = %s", outcome)
	}
	secondChain := formedCarrierPickupDB(t, "CFEP-DUP-B", "CFEV-DUP-B1", "PCL-DUP")
	if outcome := mustSaveCarrierPickup(t, transactor, ctx, repository, carrierPickupRecord(secondChain, carrierPickupJudgedAtDB)); outcome != ports.CarrierPickupAlreadyRegistered {
		t.Fatalf("同对象第二条首登 = %s，一对象至多一条链", outcome)
	}
	carrier, _ := first.Carrier()
	forkA, _ := first.Supersede(domain.CarrierPickupSupersession{
		Version: segmentRef(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-DUP2A"), Carrier: carrier,
		OccurredAt: carrierPickupOccurredAtDB, JudgedAt: carrierPickupJudgedAtDB.Add(time.Hour),
		Bases: []domain.CarrierPickupBasis{carrierPickupBasisDB(t, "EXTF-1", "EXTV-2")},
	})
	forkB, _ := first.Supersede(domain.CarrierPickupSupersession{
		Version: segmentRef(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-DUP2B"), Carrier: carrier,
		OccurredAt: carrierPickupOccurredAtDB, JudgedAt: carrierPickupJudgedAtDB.Add(time.Hour),
		Bases: []domain.CarrierPickupBasis{carrierPickupBasisDB(t, "EXTF-1", "EXTV-2")},
	})
	if outcome := mustSaveCarrierPickup(t, transactor, ctx, repository, carrierPickupRecord(forkA, carrierPickupJudgedAtDB)); outcome != ports.CarrierPickupSaved {
		t.Fatalf("第一条替代 = %s", outcome)
	}
	// 撞唯一约束后同一事务里还要能读回链尾作答：ON CONFLICT DO NOTHING 保事务可用。
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, carrierPickupRecord(forkB, carrierPickupJudgedAtDB))
		if err != nil || outcome != ports.CarrierPickupAlreadyRegistered {
			t.Fatalf("同一前版回指两次 = %s %v", outcome, err)
		}
		current, found, err := repository.FindCurrentByObject(txCtx, first.TenantID(), first.Object())
		if err != nil || !found || current.Pickup.Version().String() != "CFEV-DUP2A" {
			t.Fatalf("撞键后同事务读回链尾：%v %v %s", err, found, current.Pickup.Version())
		}
		return nil
	})
}

// Covers: 库内 CHECK 挡领域造不出的行——待确认却带承运主体、首登却失效。
func TestTheCheckConstraintsRefuseRowsTheDomainCannotProduce(t *testing.T) {
	_, _, _, pool := newCarrierPickups(t)
	ctx := t.Context()
	insert := `INSERT INTO transport_fulfillment.carrier_first_effective_pickup
	     (tenant_id, fact_ref, version, object_ref, result, carrier_kind, carrier_ref, occurred_at, pending_reason, claimed_material, judged_at, supersedes_version, recorded_at)
	 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
	if _, err := pool.Exec(ctx, insert, "tenant-1", "CFEP-CK", "CFEV-CK1", "PCL-CK", "PENDING", "EXTERNAL_PARTY", "party/x", nil, "IDENTITY_NOT_REGISTERED", "Carrier X", carrierPickupJudgedAtDB, nil, carrierPickupJudgedAtDB); err == nil {
		t.Fatalf("待确认却带承运主体的行应被 CHECK 拦下")
	}
	if _, err := pool.Exec(ctx, insert, "tenant-1", "CFEP-CK", "CFEV-CK2", "PCL-CK", "VOIDED", nil, nil, nil, nil, nil, carrierPickupJudgedAtDB, nil, carrierPickupJudgedAtDB); err == nil {
		t.Fatalf("不回指前版的失效行应被 CHECK 拦下")
	}
	if _, err := pool.Exec(ctx, insert, "tenant-1", "CFEP-CK", "CFEV-CK3", "PCL-CK", "PENDING", nil, nil, nil, "NO_QUALIFIED_EVIDENCE", "Carrier X", carrierPickupJudgedAtDB, nil, carrierPickupJudgedAtDB); err == nil {
		t.Fatalf("「无合格证据」不是收寄的待确认原因，应被 CHECK 拦下")
	}
}

// Covers: 参与关系表接受第三格与其失效版本（迁移 0020 改 CHECK），第四个词仍拒。
func TestTheParticipationTableAcceptsTheThirdEntryKind(t *testing.T) {
	_, _, _, pool := newCarrierPickups(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `INSERT INTO transport_fulfillment.actual_fulfillment_segment (tenant_id, segment_ref, closed, closed_at, recorded_at) VALUES ('tenant-1', 'SEG-CFEP', false, NULL, $1)`, carrierPickupJudgedAtDB); err != nil {
		t.Fatalf("立段行：%v", err)
	}
	insert := `INSERT INTO transport_fulfillment.fulfillment_participation
	     (tenant_id, segment_ref, object_ref, planned_ref, entry_kind, entry_basis, entered_at, end_kind, end_basis, ended_at, recorded_at, supersedes_entry_basis, voided)
	 VALUES ('tenant-1', 'SEG-CFEP', 'PCL-CFEP', NULL, $1, $2, $3, NULL, NULL, NULL, $3, $4, $5)`
	if _, err := pool.Exec(ctx, insert, "CARRIER_FIRST_EFFECTIVE_PICKUP", "CARRIER-FIRST-EFFECTIVE-PICKUP/CFEV-1", carrierPickupOccurredAtDB, nil, false); err != nil {
		t.Fatalf("第三格首登行应被接受：%v", err)
	}
	if _, err := pool.Exec(ctx, insert, "CARRIER_FIRST_EFFECTIVE_PICKUP", "CARRIER-FIRST-EFFECTIVE-PICKUP/CFEV-2", carrierPickupOccurredAtDB, "CARRIER-FIRST-EFFECTIVE-PICKUP/CFEV-1", true); err != nil {
		t.Fatalf("第三格失效版本应被接受：%v", err)
	}
	if _, err := pool.Exec(ctx, insert, "SOMETHING_ELSE", "X/1", carrierPickupOccurredAtDB, nil, false); err == nil {
		t.Fatalf("第四个入场词应被 CHECK 拦下")
	}
}

func newCarrierPickupHandoffFixture(t *testing.T) (*adapter.OutboxCarrierFirstEffectivePickupHandoff, *bentopg.DB, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxCarrierFirstEffectivePickupHandoff(db, store, tfHandoffClock{at: carrierPickupJudgedAtDB.Add(time.Minute)})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

// Covers: ADR-0135 决定七——三代各入一份、ID 带版本互不相同、分区键同为（租户/对象/口名）；载荷四维指名自己那一代。
func TestThreeGenerationsShareAPartitionButNotAnIDAndThePayloadNamesItsOwnGeneration(t *testing.T) {
	handoff, db, pool := newCarrierPickupHandoffFixture(t)
	ctx := t.Context()
	first := formedCarrierPickupDB(t, "CFEP-H1", "CFEV-H1a", "PCL-H1")
	carrier, _ := first.Carrier()
	second, _ := first.Supersede(domain.CarrierPickupSupersession{
		Version: segmentRef(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-H1b"), Carrier: carrier,
		OccurredAt: carrierPickupOccurredAtDB, JudgedAt: carrierPickupJudgedAtDB.Add(time.Hour),
		Bases: []domain.CarrierPickupBasis{carrierPickupBasisDB(t, "EXTF-1", "EXTV-2")},
	})
	third, _ := second.Void(domain.CarrierPickupVoiding{
		Version:  segmentRef(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-H1c"),
		JudgedAt: carrierPickupJudgedAtDB.Add(2 * time.Hour), Bases: []domain.CarrierPickupBasis{carrierPickupBasisDB(t, "EXTF-1", "EXTV-3")},
	})
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		for _, pickup := range []domain.CarrierFirstEffectivePickup{first, second, third} {
			if err := handoff.HandOffCarrierFirstEffectivePickup(txCtx, ports.CarrierFirstEffectivePickupHandoffIntent{Record: carrierPickupRecord(pickup, carrierPickupJudgedAtDB)}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("三代入队：%v", err)
	}
	ids := []string{
		"tenant-1/CFEP-H1/CFEV-H1a/carrier-first-effective-pickup",
		"tenant-1/CFEP-H1/CFEV-H1b/carrier-first-effective-pickup",
		"tenant-1/CFEP-H1/CFEV-H1c/carrier-first-effective-pickup",
	}
	for _, id := range ids {
		if count := countTFIntents(t, pool, id); count != 1 {
			t.Fatalf("%s 行数 = %d, want 1", id, count)
		}
		if got := partitionKeyOf(t, pool, id); got != "tenant-1/PCL-H1/carrier-first-effective-pickup" {
			t.Fatalf("分区键 = %q", got)
		}
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, ids[2]).Scan(&raw); err != nil {
		t.Fatalf("读取载荷：%v", err)
	}
	var payload struct {
		TenantID, Fact, Version, Object string
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("译载荷：%v", err)
	}
	if payload.TenantID != "tenant-1" || payload.Fact != "CFEP-H1" || payload.Version != "CFEV-H1c" || payload.Object != "PCL-H1" {
		t.Fatalf("载荷应四维指名自己那一代：%+v", payload)
	}
	var eventType string
	if err := pool.QueryRow(ctx, `SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, ids[0]).Scan(&eventType); err != nil {
		t.Fatalf("读取类型：%v", err)
	}
	if eventType != "transport-fulfillment.carrier-first-effective-pickup.registered" {
		t.Fatalf("事件类型 = %q", eventType)
	}
}

// Covers: 待确认版本到 handoff 响亮拒绝、不入队；无环境事务即拒。
func TestAPendingPickupVersionIsRefusedAtTheHandoffAndTheHandoffNeedsATransaction(t *testing.T) {
	handoff, db, pool := newCarrierPickupHandoffFixture(t)
	pending, err := domain.HoldCarrierFirstEffectivePickupPending(domain.PendingCarrierFirstEffectivePickupSpec{
		TenantID: segmentRef(t, domain.NewTenantID, "tenant-1"),
		Object:   segmentRef(t, domain.NewCarriedObjectReference, "PCL-H2"),
		Fact:     segmentRef(t, domain.NewCarrierFirstEffectivePickupReference, "CFEP-H2"),
		Version:  segmentRef(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-H2"),
		Reason:   domain.PickupCarrierIdentityNotRegistered,
		Material: "Carrier X",
		JudgedAt: carrierPickupJudgedAtDB,
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasisDB(t, "EXTF-1", "EXTV-1")},
	})
	if err != nil {
		t.Fatalf("待确认夹具：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffCarrierFirstEffectivePickup(txCtx, ports.CarrierFirstEffectivePickupHandoffIntent{Record: carrierPickupRecord(pending, carrierPickupJudgedAtDB)})
	}); err == nil {
		t.Fatal("待确认版本入了队")
	}
	if count := countTFIntents(t, pool, "tenant-1/CFEP-H2/CFEV-H2/carrier-first-effective-pickup"); count != 0 {
		t.Fatalf("待确认版本行数 = %d, want 0", count)
	}
	formed := formedCarrierPickupDB(t, "CFEP-H3", "CFEV-H3", "PCL-H3")
	if err := handoff.HandOffCarrierFirstEffectivePickup(t.Context(), ports.CarrierFirstEffectivePickupHandoffIntent{Record: carrierPickupRecord(formed, carrierPickupJudgedAtDB)}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 写口在无事务上下文必须经 RequireExecutor 拒绝——版本登记与意图入队同笔落地的前提。
func TestCarrierPickupsRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _, _ := newCarrierPickups(t)
	pickup := formedCarrierPickupDB(t, "CFEP-TX", "CFEV-TX1", "PCL-TX")
	_, err := repository.Save(t.Context(), carrierPickupRecord(pickup, carrierPickupJudgedAtDB))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务写入应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 身份签发走事务、两条序列各自推进、前缀可读不承载语义。
func TestCarrierPickupIdentitiesAreIssuedInsideATransaction(t *testing.T) {
	_, transactor, db, _ := newCarrierPickups(t)
	factory, err := adapter.NewResultVersions(db)
	if err != nil {
		t.Fatalf("构造签发器：%v", err)
	}
	if _, err := factory.NextCarrierFirstEffectivePickupReference(t.Context()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务签发应拒：%v", err)
	}
	mustWithinDispatchTaskTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		fact, err := factory.NextCarrierFirstEffectivePickupReference(txCtx)
		if err != nil || len(fact.String()) < 6 || fact.String()[:5] != "CFEP-" {
			t.Fatalf("事实身份：%v %s", err, fact)
		}
		version, err := factory.NextCarrierFirstEffectivePickupVersion(txCtx)
		if err != nil || version.String()[:5] != "CFEV-" {
			t.Fatalf("版本：%v %s", err, version)
		}
		return nil
	})
}
