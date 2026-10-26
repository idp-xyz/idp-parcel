package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证并列裁决口的生产装配（ADR-0309；票 docs-consistency/code/97）：端点行按特性登记处那一份 mount 挂上，
// Intake 是生产那只操作者铸造器经真操作者册铸的信封，授予与授权改路是同一种（ADR-0309 决定五）；编排是 initialRouteTieDeciderOver
// 那一套真库缝（判断键锁、判断库、未形成登记、裁决证据、取号、真 outbox 与事务壳），只换证据视图、成本取数侧与受理基线投影来源。
// 证据只记 `S`。

const (
	tieTenant    = "SYN-TENANT-01"
	tieOperator  = "SYN-NETOPS-CODE97"
	tieOutsider  = "SYN-SUBMITTER-CODE97"
	tieDecisions = "/route-initial-route-decisions"
)

type tieFixture struct {
	db        *bentopg.DB
	pool      *pgxpool.Pool
	decider   transactionalInitialRouteTieDecider
	endpoints map[string]http.Handler
	key       nrdomain.InitialRouteJudgmentKey
}

func newTieFixture(t *testing.T, grant bool) *tieFixture {
	t.Helper()
	// pgtest 的池只开两条连接；并发一格要一笔持锁、两笔裁决与一条探锁查询同时在库里，另开一只同库的池。
	config := pgtest.Pool(t).Config().Copy()
	config.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("开池：%v", err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	fixture := &tieFixture{db: db, pool: pool, key: nrdomain.InitialRouteJudgmentKey{
		TenantID:           mustNRValue(t, nrdomain.NewTenantID, tieTenant),
		CustomerAccountID:  mustNRValue(t, nrdomain.NewCustomerAccountID, "SYN-CUST-CODE97"),
		ShipmentRequestID:  mustNRValue(t, nrdomain.NewShipmentRequestID, "SYN-REQ-CODE97"),
		AcceptanceBaseline: mustNRValue(t, nrdomain.NewAcceptanceBaselineReference, "SYN-REQ-CODE97/v1"),
		DeclaredParcelID:   mustNRValue(t, nrdomain.NewDeclaredParcelID, "SYN-PARCEL-CODE97"),
		ServicePurpose:     mustNRValue(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	}}
	fixture.seedTie(t)

	baselines := &rerouteBaselines{answer: nrports.AcceptanceBaselineGeo{
		CustomerAccountID:  fixture.key.CustomerAccountID,
		ShipmentRequestID:  fixture.key.ShipmentRequestID,
		AcceptanceBaseline: fixture.key.AcceptanceBaseline,
		Geo: nrdomain.NewGeoResolutionProjection(
			nrdomain.NewGeoResolutionSide("CN", true, "200000", true),
			nrdomain.NewGeoResolutionSide("SG", true, "018956", true),
		),
	}}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	downstream, err := nrpostgres.NewOutboxInitialRouteHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	decider, err := initialRouteTieDeciderOver(db, systemClock{}, &rerouteEvidence{evidence: rerouteEvidenceFor(t)},
		rerouteNoCosts{}, baselines, downstream)
	if err != nil {
		t.Fatalf("装配裁决编排：%v", err)
	}
	fixture.decider = decider

	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// 授予未登记即未配置：只登别种授予的操作者与没登任何授予的操作者都被拒，不默认给任何角色。
	registerOperatorIn(t, db, operators, tieTenant, tieOutsider, interval, accessidentity.DecisionTransportCommissionSubmission)
	if grant {
		registerOperatorIn(t, db, operators, tieTenant, tieOperator, interval, accessidentity.DecisionAuthorizedReroute)
	}
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	fixture.endpoints = map[string]http.Handler{}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*initialRouteTieWiring)(nil)) {
			continue
		}
		mounted := feature.mountWith(endpointChannels{operatorDecisions: decisions},
			initialRouteTieWiring{decider: decider, ties: unwiredInitialRouteTies{}})
		for _, endpoint := range mounted {
			fixture.endpoints[endpoint.Pattern] = endpoint.Handler
		}
	}
	if fixture.endpoints[tieDecisions] == nil {
		t.Fatalf("%s 没按登记处挂上", tieDecisions)
	}
	return fixture
}

// seedTie 落一行最低成本并列的未形成登记：两条候选都合格，打平在 1000 CNY。
func (fixture *tieFixture) seedTie(t *testing.T) {
	t.Helper()
	routes, err := nrpostgres.NewInitialRoutes(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	var tied []nrdomain.CandidateID
	var costs []nrdomain.CandidateCostFact
	for _, id := range []string{"SYN-LINE-A@1", "SYN-LINE-B@1"} {
		candidate := mustNRValue(t, nrdomain.NewCandidateID, id)
		fact, err := nrdomain.NewPricedCandidateCost(candidate, 1000, "CNY")
		if err != nil {
			t.Fatal(err)
		}
		tied = append(tied, candidate)
		costs = append(costs, fact)
	}
	mustWithin(t, fixture.db, t.Context(), func(txCtx context.Context) error {
		return routes.RecordNotFormed(txCtx, nrports.InitialRouteNotFormed{
			Key:         fixture.key,
			Correlation: mustNRValue(t, nrdomain.NewRequestCorrelationID, "SYN-HANDOFF-CODE97"),
			Reason:      nrports.NotFormedCandidatesTied,
			Candidates:  []nrdomain.RouteCandidate{qualifiedRerouteCandidate(t, "SYN-LINE-A@1"), qualifiedRerouteCandidate(t, "SYN-LINE-B@1")},
			Tied:        tied,
			TiedCosts:   costs,
			AcceptedAt:  time.Now().Add(-3 * time.Hour).UTC().Truncate(time.Microsecond),
			SettledAt:   time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Microsecond),
		})
	})
}

func (fixture *tieFixture) post(t *testing.T, token, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, tieDecisions, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[tieDecisions].ServeHTTP(response, request)
	var answer map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复不是 JSON：%s", response.Body.String())
	}
	return response.Code, answer
}

func tieBody(candidate string) string {
	return `{"declaredParcelId":"SYN-PARCEL-CODE97","candidate":"` + candidate + `","basis":"SYN-APPROVAL-CODE97"}`
}

func (fixture *tieFixture) count(t *testing.T, query string) int {
	t.Helper()
	var rows int
	if err := fixture.pool.QueryRow(t.Context(), query, tieTenant).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// Covers: AT-NR-103（ADR-0309 决定五）——授予未登记答 403 OPERATOR_NOT_GRANTED，不落任何东西：没登任何授予的操作者与只持别种授予的
// 操作者同样被拒，不默认给任何角色。
func TestAnUnregisteredGrantAnswersNotConfiguredOnTheTieDecisionFace(t *testing.T) {
	notGranted := newTieFixture(t, false)
	for _, token := range []string{tieOutsider, tieOperator} {
		status, answer := notGranted.post(t, token, tieBody("SYN-LINE-B@1"))
		if status != http.StatusForbidden || answer["error"].(map[string]any)["code"] != "OPERATOR_NOT_GRANTED" {
			t.Fatalf("授予未登记（%s）：%d %v", token, status, answer)
		}
	}
	if rows := notGranted.count(t, `SELECT count(*) FROM network_routing.initial_route WHERE tenant_id = $1`); rows != 0 {
		t.Fatalf("没授予也落了 %d 行初始路由", rows)
	}
}

// Covers: AT-NR-100、AT-NR-101（ADR-0309 决定二、三、五）——登记授权改路授予之后同一口成立：201 INITIAL_ROUTE_DECIDED，决定方是会话里
// 认证出的「发行方#sub」，判断库有这件包裹的首个计划、裁决证据一行、发布意图入 outbox；载荷里自报操作者答 400；同一件包裹再裁答 200
// INITIAL_ROUTE_ALREADY_FORMED，不再落第二份。
func TestAnAuthorizedOperatorDecidesAnInitialRouteTieOnTheDatabase(t *testing.T) {
	routeRows := `SELECT count(*) FROM network_routing.initial_route WHERE tenant_id = $1`
	fixture := newTieFixture(t, true)
	status, answer := fixture.post(t, tieOperator, `{"declaredParcelId":"SYN-PARCEL-CODE97","candidate":"SYN-LINE-B@1","basis":"X","decidedBy":"someone"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("载荷里自报操作者：%d %v, want 400", status, answer)
	}
	status, answer = fixture.post(t, tieOperator, tieBody("SYN-LINE-B@1"))
	if status != http.StatusCreated || answer["outcome"] != "INITIAL_ROUTE_DECIDED" {
		t.Fatalf("裁决：%d %v", status, answer)
	}
	plan := answer["plan"].(map[string]any)
	if plan["candidate"] != "SYN-LINE-B@1" || plan["effectiveFrom"] != plan["judgedAt"] {
		t.Fatalf("plan = %v", plan)
	}
	if rows := fixture.count(t, routeRows+` AND conclusion = 'ROUTE_FORMED'`); rows != 1 {
		t.Fatalf("判断库首个计划 %d 行, want 1", rows)
	}
	var decidedBy, candidate string
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT decided_by, chosen_candidate FROM network_routing.initial_route_tie_decision WHERE tenant_id = $1`, tieTenant,
	).Scan(&decidedBy, &candidate); err != nil {
		t.Fatalf("裁决证据：%v", err)
	}
	if decidedBy != "OPERATOR/"+customsDecisionIssuer+"#"+tieOperator || candidate != "SYN-LINE-B@1" {
		t.Fatalf("裁决证据 = %q %q", decidedBy, candidate)
	}
	if rows := fixture.count(t,
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'network-routing.initial-route.formed' AND scope = $1`); rows != 1 {
		t.Fatalf("发布意图 %d 条, want 1", rows)
	}

	status, answer = fixture.post(t, tieOperator, tieBody("SYN-LINE-A@1"))
	if status != http.StatusOK || answer["outcome"] != "INITIAL_ROUTE_ALREADY_FORMED" ||
		answer["existing"].(map[string]any)["conclusion"] != "ROUTE_FORMED" {
		t.Fatalf("第二次裁决：%d %v", status, answer)
	}
	if rows := fixture.count(t, `SELECT count(*) FROM network_routing.initial_route_tie_decision WHERE tenant_id = $1`); rows != 1 {
		t.Fatalf("裁决证据 %d 行, want 1", rows)
	}
}

func (fixture *tieFixture) command(t *testing.T, operator, candidate string) nrapplication.DecideInitialRouteCommand {
	t.Helper()
	return nrapplication.DecideInitialRouteCommand{
		Tenant:    fixture.key.TenantID,
		Parcel:    fixture.key.DeclaredParcelID,
		Candidate: mustNRValue(t, nrdomain.NewCandidateID, candidate),
		Basis:     mustNRValue(t, nrdomain.NewRerouteDecisionBasisReference, "SYN-APPROVAL-CODE97"),
		DecidedBy: mustNRValue(t, nrdomain.NewAuthorizedRoleReference, "OPERATOR/"+customsDecisionIssuer+"#"+operator),
	}
}

// waitForAdvisoryWaiters 等本库里至少 n 个等在咨询锁上的请求：裁决真等在判断键锁上，才放行持锁那一笔。
func (fixture *tieFixture) waitForAdvisoryWaiters(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := fixture.pool.QueryRow(t.Context(),
			`SELECT count(*)
			   FROM pg_locks l
			   JOIN pg_database d ON d.oid = l.database
			  WHERE l.locktype = 'advisory' AND NOT l.granted AND d.datname = current_database()`,
		).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("等判断键锁的请求 %d 个, want %d——裁决没有等判断键锁", waiting, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Covers: AT-NR-101（ADR-0309 决定三「判断键锁里串行」）——另开一笔事务按判断键取锁（与复核、授权改路决定同一把）拦住，两个授权角色同时裁
// 同一件包裹的并列都等在锁上，放行后只有一笔成立，另一笔读到先成的那一份、答 INITIAL_ROUTE_ALREADY_FORMED；判断库与裁决证据各一行。
func TestTwoConcurrentTieDecisionsFormOnlyOneFirstPlan(t *testing.T) {
	fixture := newTieFixture(t, true)
	reassessments, err := nrpostgres.NewRouteReassessments(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	held := make(chan struct{})
	release := make(chan struct{})
	holder := make(chan error, 1)
	go func() {
		holder <- fixture.db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			if _, _, err := reassessments.LatestTriggerAt(txCtx, fixture.key); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	select {
	case <-held:
	case err := <-holder:
		t.Fatalf("持锁那一笔没取到锁：%v", err)
	}

	var wait sync.WaitGroup
	results := make([]nrapplication.DecideInitialRouteResult, 2)
	errs := make([]error, 2)
	for index, choice := range []struct{ operator, candidate string }{
		{tieOperator, "SYN-LINE-A@1"}, {tieOperator + "-2", "SYN-LINE-B@1"},
	} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index], errs[index] = fixture.decider.Decide(context.Background(), fixture.command(t, choice.operator, choice.candidate))
		}()
	}
	fixture.waitForAdvisoryWaiters(t, 2)
	close(release)
	if err := <-holder; err != nil {
		t.Fatalf("持锁那一笔：%v", err)
	}
	wait.Wait()

	outcomes := map[nrapplication.DecideInitialRouteOutcome]int{}
	for index := range results {
		if errs[index] != nil {
			t.Fatalf("裁决 %d：%v", index, errs[index])
		}
		outcomes[results[index].Outcome()]++
	}
	if outcomes[nrapplication.InitialRouteTieDecided] != 1 || outcomes[nrapplication.InitialRouteAlreadyFormed] != 1 {
		t.Fatalf("outcomes = %v, want 一笔成立、一笔已有结果", outcomes)
	}
	if rows := fixture.count(t, `SELECT count(*) FROM network_routing.initial_route WHERE tenant_id = $1`); rows != 1 {
		t.Fatalf("判断库 %d 行, want 1", rows)
	}
	if rows := fixture.count(t, `SELECT count(*) FROM network_routing.initial_route_tie_decision WHERE tenant_id = $1`); rows != 1 {
		t.Fatalf("裁决证据 %d 行, want 1", rows)
	}
}
