package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 16 证实际移动事实登记：版本链上多版共存、门禁放行凭据只挂得上出发、出发的门禁读数与事实
// 同一行往返、撞键译`已登记`且不顶替、无事务拒，以及库内 CHECK 挡住领域造不出的行。夹具全为合成登记（S 级）。

var movedAtDBFixture = time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)

// TestAMovementFactRoundTrips 证一条事实原样往返，且到达不带门禁依据。
func TestAMovementFactRoundTrips(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()

	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecord(t, "MF-0001", "v1", domain.ArrivalFact, ""))

	found, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-0001", "v1"))
	if err != nil || !exists {
		t.Fatalf("取回事实：%v exists=%v", err, exists)
	}
	if found.Fact.Kind() != domain.ArrivalFact {
		t.Fatalf("种类没有原样带回：%q", found.Fact.Kind())
	}
	if found.Fact.Location().String() != "hub-1" || found.Fact.Source().String() != "carrier-scan/v1" {
		t.Fatalf("地点或来源没有原样带回：%q / %q", found.Fact.Location(), found.Fact.Source())
	}
	if !found.Fact.OccurredAt().Equal(movedAtDBFixture) {
		t.Fatalf("发生时刻没有原样带回：%s", found.Fact.OccurredAt())
	}
	if _, gated := found.Fact.GateClearance(); gated {
		t.Fatal("到达读回来带了门禁放行依据")
	}
	if _, has := found.Fact.Corrects(); has {
		t.Fatal("首版读回来带了前身")
	}
}

// Covers: AT-TF-106 真库半边——出发连同它的门禁读数同一行往返：整格、问关务时的起讫、逐对象那一格（关务判断标识、依据的动作、
// 种类与来源）原样读回；声明的段与资源、设备出示的凭据也原样读回。阻断照样落得进去。
func TestADepartureRoundTripsWithItsGateReading(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()

	record := movementFactRecord(t, "MF-0002", "v1", domain.DepartureFact, "cc-clearance/v1")
	mustSaveMovementFact(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-0002", "v1"))
	if err != nil || !exists {
		t.Fatalf("取回事实：%v exists=%v", err, exists)
	}
	clearance, presented := found.Fact.GateClearance()
	if !presented || clearance.String() != "cc-clearance/v1" {
		t.Fatalf("放行凭据没有原样带回：%q presented=%v", clearance, presented)
	}
	if segment, declared := found.Fact.Segment(); !declared || segment.String() != "segment-1" {
		t.Fatalf("段没有原样带回：%q declared=%v", segment, declared)
	}
	if resource, declared := found.Fact.Resource(); !declared || resource.String() != "truck-1" {
		t.Fatalf("资源没有原样带回：%q declared=%v", resource, declared)
	}
	reading, read := found.Fact.GateReading()
	if !read || reading.Answer() != domain.DepartureGateBlocked {
		t.Fatalf("读数没有原样带回：%s read=%v", reading.Answer(), read)
	}
	if from, to, directed := reading.Direction(); !directed || from.String() != "NODE-SHA" || to.String() != "NODE-SZX" {
		t.Fatalf("起讫没有原样带回：%s → %s", from, to)
	}
	findings := reading.Findings()
	if len(findings) != 2 {
		t.Fatalf("逐对象 %d 格, want 2", len(findings))
	}
	if judgement, judged := findings[0].Judgement(); findings[0].Object().String() != "parcel-1" ||
		findings[0].Answer() != domain.DepartureGateBlocked || !judged || judgement != "AADJ-parcel-1" {
		t.Fatalf("第一件 = %s %s %q", findings[0].Object(), findings[0].Answer(), judgement)
	}
	bases := findings[0].Bases()
	if len(bases) != 1 || bases[0].Action != "LOADING_DEPARTURE" || bases[0].Cause != "RESTRICTION_IN_FORCE" ||
		len(bases[0].Sources) != 2 || bases[0].Sources[1] != "RST-2" {
		t.Fatalf("依据没有原样带回：%+v", bases)
	}
	if _, judged := findings[1].Judgement(); judged || findings[1].Answer() != domain.DepartureGateUnreadable {
		t.Fatalf("读不动那一件读回成了 %s judged=%v", findings[1].Answer(), judged)
	}
}

// Covers: AT-TF-109 真库半边——停在本上下文这一侧的读数（没有逐对象那一格）同样往返；没声明段的出发落得进去、读得回来。
func TestAStoppedDepartureReadingRoundTrips(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()

	spec := movementFactSpec(t, "MF-0007", "v1", domain.DepartureFact)
	reading, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
		Stops: []domain.DepartureGateStop{domain.DepartureSegmentNotDeclared, domain.DepartureScheduleNotRegistered},
	})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	spec.Segment, spec.GateReading = domain.FulfillmentSegmentReference{}, reading
	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecordOf(t, spec))

	found, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-0007", "v1"))
	if err != nil || !exists {
		t.Fatalf("取回事实：%v exists=%v", err, exists)
	}
	back, _ := found.Fact.GateReading()
	stops := back.Stops()
	if back.Answer() != domain.DepartureGateUndecided || len(stops) != 2 ||
		stops[0] != domain.DepartureSegmentNotDeclared || stops[1] != domain.DepartureScheduleNotRegistered {
		t.Fatalf("停点读数读回成了 %s %v", back.Answer(), stops)
	}
	if _, _, directed := back.Direction(); directed {
		t.Fatal("没读到班次的读数读回来带了起讫")
	}
}

// Covers: 迁移 0026 的存量回填——本迁移之前落册的出发（读数列为空）经迁移里那一句 UPDATE 回填为未决、停点「登记于门禁读数之前」，
// 并由重建门原样装回。用例在本库拷贝上临时拆掉「出发必带读数」那道 CHECK，造一行迁移前的形状，再执行迁移文件里原文的那一句。
func TestTheMigrationBackfillsEarlierDeparturesAsRecordedBeforeTheGateReading(t *testing.T) {
	repository, _, pool := newMovementFacts(t)
	ctx := t.Context()

	assets, err := migrations.TransportFulfillment()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	var backfill string
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "0026_") {
			start := strings.Index(asset.SQL, "UPDATE transport_fulfillment.transport_movement_fact")
			end := strings.Index(asset.SQL[start:], ";")
			if start < 0 || end < 0 {
				t.Fatal("迁移 0026 里找不到回填那一句")
			}
			backfill = asset.SQL[start : start+end]
		}
	}
	if backfill == "" {
		t.Fatal("没有迁移 0026")
	}

	for _, statement := range []string{
		`ALTER TABLE transport_fulfillment.transport_movement_fact DROP CONSTRAINT transport_movement_fact_gate_reading_on_departure`,
		`INSERT INTO transport_fulfillment.transport_movement_fact
		     (tenant_id, fact_ref, version, schedule_ref, kind, location_ref, source_ref, occurred_at, gate_clearance, recorded_at)
		 VALUES ('tenant-1', 'MF-LEGACY', 'v1', 'schedule-1', 'DEPARTURE', 'hub-1', 'carrier-scan/v1', now(), 'cc-clearance/v0', now()),
		        ('tenant-1', 'MF-LEGACY-ARR', 'v1', 'schedule-1', 'ARRIVAL', 'hub-2', 'carrier-scan/v1', now(), NULL, now())`,
		backfill,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("执行 %q：%v", statement, err)
		}
	}

	found, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-LEGACY", "v1"))
	if err != nil || !exists {
		t.Fatalf("回填后的出发装不回：%v exists=%v", err, exists)
	}
	reading, read := found.Fact.GateReading()
	if !read || reading.Answer() != domain.DepartureGateUndecided || len(reading.Stops()) != 1 ||
		reading.Stops()[0] != domain.DepartureRecordedBeforeGateReading {
		t.Fatalf("回填读数 = %s %v", reading.Answer(), reading.Stops())
	}
	if clearance, presented := found.Fact.GateClearance(); !presented || clearance.String() != "cc-clearance/v0" {
		t.Fatal("回填改了事实本身的列")
	}
	arrival, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-LEGACY-ARR", "v1"))
	if err != nil || !exists {
		t.Fatalf("回填后的到达装不回：%v exists=%v", err, exists)
	}
	if _, read := arrival.Fact.GateReading(); read {
		t.Fatal("到达被回填了读数")
	}
}

// TestASecondSaveOfTheSameFactVersionKeepsTheOriginal 证撞键译`已登记`且不顶替：更正走新
// 版本，原记录与其派生历史不被改写。
func TestASecondSaveOfTheSameFactVersionKeepsTheOriginal(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()

	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecord(t, "MF-0003", "v1", domain.InTransitFact, ""))

	other := movementFactRecord(t, "MF-0003", "v1", domain.InTransitFact, "")
	var outcome ports.MovementFactSaveOutcome
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, other)
		return err
	})
	if outcome != ports.MovementFactVersionAlreadyRegistered {
		t.Fatalf("撞键 outcome = %d, want MovementFactVersionAlreadyRegistered", outcome)
	}
}

// TestMovementFactWritesRefuseToRunOutsideATransaction 证写口无环境事务即拒。断言指名
// ErrTransactionRequired 而不是 err != nil。
func TestMovementFactWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newMovementFacts(t)

	_, err := repository.Save(t.Context(), movementFactRecord(t, "MF-0004", "v1", domain.ArrivalFact, ""))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestMovementFactRollbackLeavesNothingBehind 证首登与它所在的事务同生共死。
func TestMovementFactRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, movementFactRecord(t, "MF-0005", "v1", domain.ArrivalFact, "")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-0005", "v1")); err != nil || exists {
		t.Errorf("回滚后事实仍在：exists=%v err=%v", exists, err)
	}
}

// TestMovementFactCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门。
//
// 「到达挂了放行依据」那一格尤其要有：领域构造门已拒，但**绕过构造门的写入路径同样不该落进
// 一个不存在的门**。
func TestMovementFactCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	repository, transactor, pool := newMovementFacts(t)
	ctx := t.Context()
	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecord(t, "MF-0006", "v1", domain.ArrivalFact, ""))

	base := `INSERT INTO transport_fulfillment.transport_movement_fact
	    (tenant_id, fact_ref, version, schedule_ref, kind, location_ref, source_ref,
	     occurred_at, gate_clearance, corrects_version, corrected_at, recorded_at) VALUES `
	for name, values := range map[string]string{
		"集外事实种类":  `('tenant-1','BAD-1','v1','s','DIVERTED','l','src',now(),NULL,NULL,NULL,now())`,
		"到达挂放行依据": `('tenant-1','BAD-2','v1','s','ARRIVAL','l','src',now(),'cc/v1',NULL,NULL,now())`,
		"移动挂放行依据": `('tenant-1','BAD-3','v1','s','IN_TRANSIT','l','src',now(),'cc/v1',NULL,NULL,now())`,
		"更正只有前身":  `('tenant-1','BAD-4','v2','s','ARRIVAL','l','src',now(),NULL,'v1',NULL,now())`,
		"更正只有时刻":  `('tenant-1','BAD-5','v2','s','ARRIVAL','l','src',now(),NULL,NULL,now(),now())`,
		"前身指向自己":  `('tenant-1','BAD-6','v2','s','ARRIVAL','l','src',now(),NULL,'v2',now(),now())`,
		"更正早于发生":  `('tenant-1','BAD-7','v2','s','ARRIVAL','l','src',now(),NULL,'v1',now()-interval '1 day',now())`,
		"缺来源":     `('tenant-1','BAD-8','v1','s','ARRIVAL','l','',now(),NULL,NULL,NULL,now())`,
		"缺地点":     `('tenant-1','BAD-9','v1','s','ARRIVAL','','src',now(),NULL,NULL,NULL,now())`,
		"出发没有读数":  `('tenant-1','BAD-10','v1','s','DEPARTURE','l','src',now(),NULL,NULL,NULL,now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}

	// 0026 加的几格：读数只在出发上、整格封闭、明细与整格同在同缺、明细里停点与逐对象恰有其一，段与资源不许是空串。
	withReading := `INSERT INTO transport_fulfillment.transport_movement_fact
	    (tenant_id, fact_ref, version, schedule_ref, kind, location_ref, source_ref,
	     occurred_at, segment_ref, resource_ref, gate_reading, gate_reading_detail, recorded_at) VALUES `
	stopped := `'{"from":null,"to":null,"stops":["SEGMENT_NOT_DECLARED"],"findings":[]}'`
	for name, values := range map[string]string{
		"到达带读数":    `('tenant-1','BAD-11','v1','s','ARRIVAL','l','src',now(),NULL,NULL,'UNDECIDED',` + stopped + `,now())`,
		"集外整格":     `('tenant-1','BAD-12','v1','s','DEPARTURE','l','src',now(),NULL,NULL,'DEPARTURE_GATE_BLOCKED',` + stopped + `,now())`,
		"有整格无明细":   `('tenant-1','BAD-13','v1','s','DEPARTURE','l','src',now(),NULL,NULL,'UNDECIDED',NULL,now())`,
		"停点与逐对象皆空": `('tenant-1','BAD-14','v1','s','DEPARTURE','l','src',now(),NULL,NULL,'UNDECIDED','{"stops":[],"findings":[]}',now())`,
		"明细缺停点那一键": `('tenant-1','BAD-15','v1','s','DEPARTURE','l','src',now(),NULL,NULL,'UNDECIDED','{"findings":[{}]}',now())`,
		"明细不是对象":   `('tenant-1','BAD-16','v1','s','DEPARTURE','l','src',now(),NULL,NULL,'UNDECIDED','[]',now())`,
		"段是空串":     `('tenant-1','BAD-17','v1','s','DEPARTURE','l','src',now(),' ',NULL,'UNDECIDED',` + stopped + `,now())`,
		"资源是空串":    `('tenant-1','BAD-18','v1','s','ARRIVAL','l','src',now(),NULL,'',NULL,NULL,now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, withReading+values); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}
}

func newMovementFacts(t *testing.T) (*adapter.MovementFacts, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewMovementFacts(db)
	if err != nil {
		t.Fatalf("构造移动事实登记库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustSaveMovementFact(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.MovementFacts,
	record ports.MovementFactRecord,
) {
	t.Helper()
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, record)
		return err
	})
}

func movementFactKeyFixture(t *testing.T, tenant, fact, version string) ports.MovementFactKey {
	t.Helper()
	return ports.MovementFactKey{
		TenantID: segmentRef(t, domain.NewTenantID, tenant),
		Fact:     segmentRef(t, domain.NewMovementFactReference, fact),
		Version:  segmentRef(t, domain.NewMovementFactVersion, version),
	}
}

// movementFactSpec 是一份成形的事实输入。出发声明段 segment-1 与资源 truck-1，读数问到两件：parcel-1 被两条限制阻断，
// parcel-2 关务读不动——整格是阻断。
func movementFactSpec(t *testing.T, fact, version string, kind domain.MovementFactKind) domain.MovementFactSpec {
	t.Helper()
	spec := domain.MovementFactSpec{
		TenantID:   segmentRef(t, domain.NewTenantID, "tenant-1"),
		Fact:       segmentRef(t, domain.NewMovementFactReference, fact),
		Schedule:   segmentRef(t, domain.NewScheduleReference, "schedule-1"),
		Kind:       kind,
		Location:   segmentRef(t, domain.NewMovementLocationReference, "hub-1"),
		Source:     segmentRef(t, domain.NewMovementSourceReference, "carrier-scan/v1"),
		Version:    segmentRef(t, domain.NewMovementFactVersion, version),
		OccurredAt: movedAtDBFixture,
	}
	if kind != domain.DepartureFact {
		return spec
	}
	spec.Segment = segmentRef(t, domain.NewFulfillmentSegmentReference, "segment-1")
	spec.Resource = segmentRef(t, domain.NewTransportResourceReference, "truck-1")
	blocked, err := domain.NewDepartureGateFinding(segmentRef(t, domain.NewCarriedObjectReference, "parcel-1"),
		domain.DepartureGateBlocked, "AADJ-parcel-1",
		[]domain.DepartureGateBasis{{Action: "LOADING_DEPARTURE", Cause: "RESTRICTION_IN_FORCE", Sources: []string{"RST-1", "RST-2"}}})
	if err != nil {
		t.Fatalf("阻断那一件：%v", err)
	}
	unreadable, err := domain.NewDepartureGateFinding(segmentRef(t, domain.NewCarriedObjectReference, "parcel-2"),
		domain.DepartureGateUnreadable, "", []domain.DepartureGateBasis{{Cause: "RELEASE_OUTCOME_VIEW_UNAVAILABLE"}})
	if err != nil {
		t.Fatalf("读不动那一件：%v", err)
	}
	spec.GateReading, err = domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
		From:     segmentRef(t, domain.NewNetworkNodeReference, "NODE-SHA"),
		To:       segmentRef(t, domain.NewNetworkNodeReference, "NODE-SZX"),
		Findings: []domain.DepartureGateFinding{unreadable, blocked},
	})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	return spec
}

func movementFactRecordOf(t *testing.T, spec domain.MovementFactSpec) ports.MovementFactRecord {
	t.Helper()
	recorded, err := domain.RecordMovementFact(spec)
	if err != nil {
		t.Fatalf("形成事实夹具：%v", err)
	}
	return ports.MovementFactRecord{
		Key:        ports.MovementFactKey{TenantID: spec.TenantID, Fact: spec.Fact, Version: spec.Version},
		Fact:       recorded,
		RecordedAt: movedAtDBFixture.Add(time.Minute),
	}
}

func movementFactRecord(
	t *testing.T,
	fact, version string,
	kind domain.MovementFactKind,
	clearance string,
) ports.MovementFactRecord {
	t.Helper()
	spec := movementFactSpec(t, fact, version, kind)
	if clearance != "" {
		spec.GateClearance = segmentRef(t, domain.NewGateClearanceReference, clearance)
	}
	return movementFactRecordOf(t, spec)
}
