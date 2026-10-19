package customscompliance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfcustoms "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/customscompliance"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证消费侧适配器把关务的动作准入判断译成出发读数的那一格：真库那一半接关务的生产编排与真表（隔离合成 S），替身那一半
// 钉住「问不出答复」只交 error、不冒充关务的读不动。

var departureGateAt = time.Date(2026, 9, 24, 14, 0, 0, 0, time.UTC)

const (
	departureTenant = "SYN-TENANT-01"
	departureParcel = "SYN-PCL-DEPARTURE-01"
)

func ref[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

// ownTransactionJudge 给关务判断一笔自己的事务，形照生产装配：判断册写口无事务即拒，判断不并进调用方那一笔。
type ownTransactionJudge struct {
	db    *bentopg.DB
	inner *ccapp.JudgeActionAdmissionHandler
}

func (judge ownTransactionJudge) Handle(ctx context.Context, command ccapp.JudgeActionAdmissionCommand) (ccapp.JudgeActionAdmissionResult, error) {
	var result ccapp.JudgeActionAdmissionResult
	err := judge.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = judge.inner.Handle(txCtx, command)
		return handleErr
	})
	return result, err
}

type departureGateDB struct {
	t    *testing.T
	pool *pgxpool.Pool
	view *tfcustoms.DepartureGateView
}

func newDepartureGateDB(t *testing.T) *departureGateDB {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("构造关务适配器：%v", err)
		}
	}
	zones, err := ccpostgres.NewNodeCustomsZonePointView(db)
	must(err)
	scopes, err := ccpostgres.NewObjectCustomsScopeView(db)
	must(err)
	restrictions, err := ccpostgres.NewRestrictions(db)
	must(err)
	gates, err := ccpostgres.NewGateConditionView(db)
	must(err)
	duty, err := ccpostgres.NewDutyPaymentReconciliation(db)
	must(err)
	releases, err := ccpostgres.NewReleaseOutcomeView(db)
	must(err)
	store, err := ccpostgres.NewActionAdmissionJudgements(db)
	must(err)
	handler, err := ccapp.NewJudgeActionAdmissionHandler(ccapp.JudgeActionAdmissionDeps{
		Zones: zones, Scopes: scopes, Restrictions: restrictions, Conditions: gates, DutyRules: gates,
		DutyVerifications: duty, Releases: releases, Store: store, Clock: fixedClock{at: departureGateAt.Add(time.Minute)},
	})
	must(err)
	view, err := tfcustoms.NewDepartureGateView(ownTransactionJudge{db: db, inner: handler})
	must(err)

	fixture := &departureGateDB{t: t, pool: pool, view: view}
	registry, err := ccpostgres.NewNodeCustomsZoneRegistrations(db)
	must(err)
	tenant := ref(t, ccdomain.NewTenantID, departureTenant)
	for node, zone := range map[string]string{
		"SYN-NODE-SHA-HUB": "SYN-ZONE-CN", "SYN-NODE-SZX-GATE": "SYN-ZONE-CN", "SYN-NODE-SIN-HUB": "SYN-ZONE-SG",
	} {
		nodeRef, zoneRef := ref(t, ccdomain.NewNetworkNodeReference, node), ref(t, ccdomain.NewCustomsZoneReference, zone)
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			_, err := registry.RegisterNodeCustomsZone(txCtx, tenant, nodeRef, zoneRef, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			return err
		}); err != nil {
			t.Fatalf("登区域 %s：%v", node, err)
		}
	}
	return fixture
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

func (fixture *departureGateDB) seed(sql string, args ...any) {
	fixture.t.Helper()
	if _, err := fixture.pool.Exec(fixture.t.Context(), sql, args...); err != nil {
		fixture.t.Fatalf("播种：%v", err)
	}
}

// inUnit 立一个出口案件与一个含本包裹的申报单元，形照关务真库用例的夹具。
func (fixture *departureGateDB) inUnit() {
	fixture.seed(`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at)
		 VALUES ($1, 'SYN-JUR-CN', 'EXPORT', 'SYN-PROC-CN-EXPORT', 'SYN-OBLIGATION-01', 'SYN-CASE-DEP-01',
		         jsonb_build_array(jsonb_build_object('parcel', $2::text, 'customer', 'SYN-ACCOUNT-01', 'sourceRef', 'SYN-SRC-01')),
		         '[]'::jsonb, $3)`,
		departureTenant, departureParcel, departureGateAt.Add(-48*time.Hour))
	fixture.seed(`INSERT INTO customs_compliance.declaration_unit
			(tenant_id, unit_id, case_id, procedure_ref, members, formed_at)
		 VALUES ($1, 'SYN-UNIT-DEP-01', 'SYN-CASE-DEP-01', 'SYN-PROC-CN-EXPORT', jsonb_build_array($2::text), $3)`,
		departureTenant, departureParcel, departureGateAt.Add(-47*time.Hour))
}

func (fixture *departureGateDB) ask(from, to string) tfdomain.DepartureGateFinding {
	fixture.t.Helper()
	finding, err := fixture.view.ReadDepartureGate(fixture.t.Context(), tfports.DepartureGateQuery{
		TenantID: ref(fixture.t, tfdomain.NewTenantID, departureTenant),
		Object:   ref(fixture.t, tfdomain.NewCarriedObjectReference, departureParcel),
		From:     ref(fixture.t, tfdomain.NewNetworkNodeReference, from),
		To:       ref(fixture.t, tfdomain.NewNetworkNodeReference, to),
		At:       departureGateAt,
	})
	if err != nil {
		fixture.t.Fatalf("问关务：%v", err)
	}
	return finding
}

func expectFinding(t *testing.T, finding tfdomain.DepartureGateFinding, answer tfdomain.DepartureGateAnswer, action, cause string) {
	t.Helper()
	bases := finding.Bases()
	judgement, judged := finding.Judgement()
	if finding.Answer() != answer || len(bases) != 1 || bases[0].Action != action || bases[0].Cause != cause || !judged || judgement == "" {
		t.Fatalf("那一格 = %s %+v judgement=%q，要 %s / %s / %s", finding.Answer(), bases, judgement, answer, action, cause)
	}
}

// Covers: AT-TF-105 适配器半边（演示判据，证据 S）——自营 SHA→SZX 同区、包裹不在案：关务只适用装载出发、答不受门禁，译回不受门禁
// 并带回关务判断标识。SZX→SIN 跨区：跨区那一道答未决，整次未决，带回的只有跨区那一条依据。
func TestADomesticDepartureReadsNotGuardedAndACrossZoneDepartureReadsUndecided(t *testing.T) {
	fixture := newDepartureGateDB(t)
	expectFinding(t, fixture.ask("SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"),
		tfdomain.DepartureGateNotGuarded, "LOADING_DEPARTURE", "OUTSIDE_CUSTOMS_BOOKS")
	expectFinding(t, fixture.ask("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB"),
		tfdomain.DepartureGateUndecided, "CROSS_CUSTOMS_MOVEMENT", "CUSTOMS_SCOPE_UNRESOLVED")
}

// Covers: AT-TF-106 适配器半边——直接落在包裹上的装载出发限制：关务答阻断，译回阻断，依据带限制标识。关务的未配置（单元没登
// 门禁目录）译成未决，依据种类仍说缺的是配置。
func TestABlockedAndANotConfiguredAnswerAreTranslatedWithTheirBases(t *testing.T) {
	fixture := newDepartureGateDB(t)
	fixture.inUnit()
	expectFinding(t, fixture.ask("SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"),
		tfdomain.DepartureGateUndecided, "LOADING_DEPARTURE", "GATE_CATALOG_NOT_REGISTERED")

	fixture.seed(`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
		 VALUES ($1, 'SYN-RESTRICTION-DEP-01', 'SYN-DECISION-HOLD', $2, jsonb_build_array('LOADING_DEPARTURE'), $3)`,
		departureTenant, departureParcel, departureGateAt.Add(-3*time.Hour))
	blocked := fixture.ask("SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
	expectFinding(t, blocked, tfdomain.DepartureGateBlocked, "LOADING_DEPARTURE", "RESTRICTION_IN_FORCE")
	if sources := blocked.Bases()[0].Sources; len(sources) != 1 || sources[0] != "SYN-RESTRICTION-DEP-01" {
		t.Fatalf("阻断依据要指回那条限制：%v", sources)
	}
}

type stubJudge struct {
	result ccapp.JudgeActionAdmissionResult
	err    error
	calls  int
}

func (judge *stubJudge) Handle(context.Context, ccapp.JudgeActionAdmissionCommand) (ccapp.JudgeActionAdmissionResult, error) {
	judge.calls++
	return judge.result, judge.err
}

// 问不出答复只交 error：调用失败原样带出；答复越出封闭集（未受理、零值）是译不回来；起讫立不成意图的那一问不发出去。出发登记把
// 这些都记成读不动，但它们不冒充关务自己指名的读不动。
func TestAnAnswerThatDoesNotFormIsAnErrorNotAFinding(t *testing.T) {
	query := tfports.DepartureGateQuery{
		TenantID: ref(t, tfdomain.NewTenantID, departureTenant),
		Object:   ref(t, tfdomain.NewCarriedObjectReference, departureParcel),
		From:     ref(t, tfdomain.NewNetworkNodeReference, "SYN-NODE-SHA-HUB"),
		To:       ref(t, tfdomain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE"),
		At:       departureGateAt,
	}
	down := errors.New("关务进程不可达")
	failing := &stubJudge{err: down}
	view, err := tfcustoms.NewDepartureGateView(failing)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if _, err := view.ReadDepartureGate(t.Context(), query); !errors.Is(err, down) {
		t.Fatalf("调用失败：err = %v", err)
	}

	zero := &stubJudge{}
	view, _ = tfcustoms.NewDepartureGateView(zero)
	if _, err := view.ReadDepartureGate(t.Context(), query); !errors.Is(err, tfcustoms.ErrUntranslatableAnswer) {
		t.Fatalf("零值答复：err = %v, want ErrUntranslatableAnswer", err)
	}

	sameNode := query
	sameNode.To = sameNode.From
	if _, err := view.ReadDepartureGate(t.Context(), sameNode); !errors.Is(err, tfcustoms.ErrUntranslatableAnswer) || zero.calls != 1 {
		t.Fatalf("起讫同一节点：err = %v calls=%d，立不成意图就不该问", err, zero.calls)
	}

	if _, err := tfcustoms.NewDepartureGateView(nil); err == nil {
		t.Fatal("没有判断口也装得起来")
	}
}
