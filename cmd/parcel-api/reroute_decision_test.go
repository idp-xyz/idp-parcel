package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证授权改路决定口的生产装配（ADR-0284；票 docs-consistency/code/29）：端点行按特性登记处那一份 mount
// 挂上，Intake 是生产那只操作者铸造器经真操作者册（access_identity 0023 放开的授权改路一种）铸的信封；编排是 rerouteDeciderOver
// 那一套真库缝（复核册、决定册、适用性册、取号与事务壳），只换证据视图、成本取数侧与受理基线投影来源。并发两格用真锁取证：
// 另开一笔事务按判断键取锁拦住决定，看它等在锁上、再放行。准入范围取放行一切的替身。证据只记 `S`。

const (
	rerouteTenant     = "SYN-TENANT-01"
	rerouteOperatorA  = "SYN-NETOPS-CODE29-A"
	rerouteOperatorB  = "SYN-NETOPS-CODE29-B"
	rerouteOutsider   = "SYN-SUBMITTER-CODE29"
	rerouteDecisions  = "/route-reroute-decisions"
	rerouteSuggestion = "reassess/code29-1"
)

type rerouteFixture struct {
	db            *bentopg.DB
	pool          *pgxpool.Pool
	reassessments *nrpostgres.RouteReassessments
	decider       transactionalRerouteDecider
	endpoints     map[string]http.Handler
	key           nrdomain.InitialRouteJudgmentKey
	suggestedAt   time.Time
}

func mustNRValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("%q: %v", raw, err)
	}
	return built
}

func newRerouteFixture(t *testing.T) *rerouteFixture {
	t.Helper()
	// pgtest 的池只开两条连接；并发两格要一笔持锁、两笔决定与一条探锁查询同时在库里，另开一只同库的池。
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
	fixture := &rerouteFixture{
		db:          db,
		pool:        pool,
		suggestedAt: time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Microsecond),
		key: nrdomain.InitialRouteJudgmentKey{
			TenantID:           mustNRValue(t, nrdomain.NewTenantID, rerouteTenant),
			CustomerAccountID:  mustNRValue(t, nrdomain.NewCustomerAccountID, "SYN-CUST-CODE29"),
			ShipmentRequestID:  mustNRValue(t, nrdomain.NewShipmentRequestID, "SYN-REQ-CODE29"),
			AcceptanceBaseline: mustNRValue(t, nrdomain.NewAcceptanceBaselineReference, "SYN-REQ-CODE29/v1"),
			DeclaredParcelID:   mustNRValue(t, nrdomain.NewDeclaredParcelID, "SYN-PARCEL-CODE29"),
			ServicePurpose:     mustNRValue(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
		},
	}
	if fixture.reassessments, err = nrpostgres.NewRouteReassessments(db); err != nil {
		t.Fatal(err)
	}
	fixture.seedLapsedSuggestion(t)

	evidence := &rerouteEvidence{evidence: rerouteEvidenceFor(t)}
	baselines := &rerouteBaselines{answer: nrports.AcceptanceBaselineGeo{
		CustomerAccountID:  fixture.key.CustomerAccountID,
		ShipmentRequestID:  fixture.key.ShipmentRequestID,
		AcceptanceBaseline: fixture.key.AcceptanceBaseline,
		Geo: nrdomain.NewGeoResolutionProjection(
			nrdomain.NewGeoResolutionSide("CN", true, "200000", true),
			nrdomain.NewGeoResolutionSide("SG", true, "018956", true),
		),
	}}
	if fixture.decider, err = rerouteDeciderOver(db, systemClock{}, evidence, rerouteNoCosts{}, baselines); err != nil {
		t.Fatalf("装配决定编排：%v", err)
	}

	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for subject, kind := range map[string]accessidentity.DecisionKind{
		rerouteOperatorA: accessidentity.DecisionAuthorizedReroute,
		rerouteOperatorB: accessidentity.DecisionAuthorizedReroute,
		rerouteOutsider:  accessidentity.DecisionTransportCommissionSubmission,
	} {
		registerOperatorIn(t, db, operators, rerouteTenant, subject, interval, kind)
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
		if feature.wiringKey != any((*rerouteDecisionWiring)(nil)) {
			continue
		}
		mounted := feature.mountWith(endpointChannels{operatorDecisions: decisions},
			rerouteDecisionWiring{decider: fixture.decider, suggestions: unwiredRerouteSuggestions{}})
		for _, endpoint := range mounted {
			fixture.endpoints[endpoint.Pattern] = endpoint.Handler
		}
	}
	if fixture.endpoints[rerouteDecisions] == nil {
		t.Fatalf("%s 没按登记处挂上", rerouteDecisions)
	}
	return fixture
}

// seedLapsedSuggestion 落失效线上的一份建议：被复核计划 SYN-RPV-CODE29-1 在建议那次触发的业务时间离场，两条候选都合格、改善不过
// 阈值只成建议。
func (fixture *rerouteFixture) seedLapsedSuggestion(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	applicabilities, err := nrpostgres.NewPlanApplicabilities(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	reviewed := mustNRValue(t, nrdomain.NewRoutePlanVersionID, "SYN-RPV-CODE29-1")
	basis := mustNRValue(t, nrdomain.NewApplicabilityBasisReference, "PLAN_ORIGIN_MISMATCH/SYN-NODE-ELSEWHERE")
	established, err := nrdomain.EstablishPlanApplicability(reviewed, fixture.suggestedAt.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	lapsed, err := established.Lapse(basis, fixture.suggestedAt)
	if err != nil {
		t.Fatal(err)
	}
	blockers := []string{"IMPROVEMENT_BELOW_THRESHOLD"}
	suggestion, err := nrdomain.NewRerouteSuggestion(nrdomain.RerouteSuggestionSpec{
		Key:         fixture.key,
		Trigger:     mustNRValue(t, nrdomain.NewRerouteTriggerReference, rerouteSuggestion),
		Candidates:  []nrdomain.RouteCandidate{qualifiedRerouteCandidate(t, "SYN-LINE-A@1"), qualifiedRerouteCandidate(t, "SYN-LINE-B@1")},
		Blockers:    blockers,
		SuggestedAt: fixture.suggestedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	correlation := mustNRValue(t, nrdomain.NewRequestCorrelationID, rerouteSuggestion)
	mustWithin(t, fixture.db, ctx, func(txCtx context.Context) error {
		if err := applicabilities.Save(txCtx, lapsed); err != nil {
			return err
		}
		_, err := fixture.reassessments.Save(txCtx, correlation, nrports.ReassessmentRecord{
			Correlation:       correlation,
			Key:               fixture.key,
			Conclusion:        nrports.ReassessmentPlanLapsed,
			ReviewedPlan:      reviewed,
			LapseBasis:        basis,
			CandidateState:    nrports.CandidatesAvailable,
			RerouteState:      nrdomain.SuggestionOnly,
			RerouteBlockers:   blockers,
			Suggestion:        suggestion,
			HasSuggestion:     true,
			TriggerOccurredAt: fixture.suggestedAt,
			ReassessedAt:      fixture.suggestedAt,
		})
		return err
	})
}

func mustWithin(t *testing.T, db *bentopg.DB, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := db.Transactor().WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务：%v", err)
	}
}

func qualifiedRerouteCandidate(t *testing.T, id string) nrdomain.RouteCandidate {
	t.Helper()
	candidate, err := nrdomain.NewRouteCandidate(mustNRValue(t, nrdomain.NewCandidateID, id), nrdomain.CandidateQualified, nrdomain.CandidateReason{})
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

// rerouteEvidenceFor 是两条候选都覆盖目的地、各有一段节点间段的证据；排序形态不声明，决定口不向成本取数侧要价。
func rerouteEvidenceFor(t *testing.T) nrports.InitialRouteEvidence {
	t.Helper()
	window, err := nrdomain.NewPlannedTimeWindow(time.Now(), time.Now().Add(48*time.Hour),
		mustNRValue(t, nrdomain.NewWindowBasisReference, "SYN-CAL/v1"))
	if err != nil {
		t.Fatal(err)
	}
	evidence := nrports.InitialRouteEvidence{
		Strategy:     mustNRValue(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY/v1"),
		ViewRevision: mustNRValue(t, nrdomain.NewNetworkViewRevision, "SYN-VIEW-REV-CODE29"),
	}
	for _, id := range []string{"SYN-LINE-A@1", "SYN-LINE-B@1"} {
		area, err := nrdomain.NewServiceAreaResolution(nrdomain.ServiceAreaResolutionSpec{
			Candidate:   mustNRValue(t, nrdomain.NewCandidateID, id),
			Outcome:     nrdomain.AreaCoversDestination,
			AreaVersion: mustNRValue(t, nrdomain.NewServiceAreaVersionReference, "SYN-AREA-SG/v1"),
		})
		if err != nil {
			t.Fatal(err)
		}
		leg, err := nrdomain.NewPlannedLeg(nrdomain.PlannedLegSpec{
			From:        mustNRValue(t, nrdomain.NewPlanNodeReference, "SYN-NODE-SZX-HUB"),
			To:          mustNRValue(t, nrdomain.NewPlanNodeReference, "SYN-NODE-SIN-HUB"),
			Responsible: mustNRValue(t, nrdomain.NewResponsiblePartyReference, "SYN-CARRIER-"+id),
			Window:      window,
		})
		if err != nil {
			t.Fatal(err)
		}
		evidence.ServiceAreas = append(evidence.ServiceAreas, area)
		evidence.Paths = append(evidence.Paths, nrports.CandidatePath{Candidate: mustNRValue(t, nrdomain.NewCandidateID, id), Legs: []nrdomain.PlannedLeg{leg}})
	}
	return evidence
}

type rerouteEvidence struct{ evidence nrports.InitialRouteEvidence }

func (double *rerouteEvidence) LoadInitialRouteEvidence(
	context.Context, nrdomain.InitialRouteJudgmentKey, nrports.RequestCarriedContent,
) (nrports.InitialRouteEvidence, bool, error) {
	return double.evidence, true, nil
}

type rerouteBaselines struct{ answer nrports.AcceptanceBaselineGeo }

func (double *rerouteBaselines) LoadAcceptanceBaselineGeo(
	context.Context, nrdomain.TenantID, nrdomain.DeclaredParcelID,
) (nrports.AcceptanceBaselineGeo, bool, error) {
	return double.answer, true, nil
}

type rerouteNoCosts struct{}

func (rerouteNoCosts) LoadCandidateCosts(
	context.Context, nrdomain.InitialRouteJudgmentKey, nrdomain.ParcelFactsPoint, nrports.InitialRouteEvidence, nrports.RequestCarriedContent,
) (nrports.RouteCandidateCosts, error) {
	return nrports.RouteCandidateCosts{}, errors.New("排序形态未声明，决定口不该来要价")
}

func (fixture *rerouteFixture) post(t *testing.T, token, candidate string) (int, map[string]any) {
	t.Helper()
	body := `{"declaredParcelId":"SYN-PARCEL-CODE29","suggestion":"` + rerouteSuggestion + `","candidate":"` + candidate +
		`","basis":"SYN-APPROVAL-CODE29"}`
	request := httptest.NewRequest(http.MethodPost, rerouteDecisions, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[rerouteDecisions].ServeHTTP(response, request)
	var answer map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复不是 JSON：%s", response.Body.String())
	}
	return response.Code, answer
}

func (fixture *rerouteFixture) decisionRows(t *testing.T) int {
	t.Helper()
	var rows int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM network_routing.authorized_reroute_decision WHERE tenant_id = $1`, rerouteTenant,
	).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func (fixture *rerouteFixture) command(t *testing.T, operator, candidate string) nrapplication.DecideRerouteCommand {
	t.Helper()
	return nrapplication.DecideRerouteCommand{
		Tenant:     fixture.key.TenantID,
		Parcel:     fixture.key.DeclaredParcelID,
		Suggestion: mustNRValue(t, nrdomain.NewRequestCorrelationID, rerouteSuggestion),
		Candidate:  mustNRValue(t, nrdomain.NewCandidateID, candidate),
		Basis:      mustNRValue(t, nrdomain.NewRerouteDecisionBasisReference, "SYN-APPROVAL-CODE29"),
		DecidedBy:  mustNRValue(t, nrdomain.NewAuthorizedRoleReference, "OPERATOR/"+customsDecisionIssuer+"#"+operator),
	}
}

// judgmentKeyLock 是另开的一笔持着判断键锁的事务：release 放行（可重复调），done 交出那一笔的结局。
type judgmentKeyLock struct {
	release func()
	done    chan error
}

// holdJudgmentKeyLock 另开一笔事务按判断键取锁（与复核、决定同一把），在锁里跑 inside，等放行再关门；取到锁、inside 跑完才交回。
// 用例中途失败时由 Cleanup 放行，持锁那一笔不会把连接与库一直占着。
func (fixture *rerouteFixture) holdJudgmentKeyLock(t *testing.T, inside func(context.Context) error) *judgmentKeyLock {
	t.Helper()
	releaseCh := make(chan struct{})
	var once sync.Once
	lock := &judgmentKeyLock{release: func() { once.Do(func() { close(releaseCh) }) }, done: make(chan error, 1)}
	t.Cleanup(lock.release)
	held := make(chan struct{})
	go func() {
		lock.done <- fixture.db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			if _, _, err := fixture.reassessments.LatestTriggerAt(txCtx, fixture.key); err != nil {
				return err
			}
			if inside != nil {
				if err := inside(txCtx); err != nil {
					return err
				}
			}
			close(held)
			<-releaseCh
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-lock.done:
		t.Fatalf("持锁那一笔没取到锁：%v", err)
	}
	return lock
}

// waitForAdvisoryWaiters 等本库里至少 n 个等在咨询锁上的请求：决定真等在判断键锁上，才放行持锁那一笔。
func (fixture *rerouteFixture) waitForAdvisoryWaiters(t *testing.T, n int) {
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
			t.Fatalf("等判断键锁的请求 %d 个, want %d——决定没有等判断键锁", waiting, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Covers: AT-NR-095、AT-NR-096、AT-NR-098（ADR-0284 决定一、二、五）——持授权改路授予的操作者经生产装配的口据当前建议择一候选：201 REROUTE_DECIDED，决定方是会话里
// 认证出的「发行方#sub」，新计划落进决定册并进路由历史；另一位持同种授予的操作者再决定答 200 SUGGESTION_ALREADY_DECIDED、交回
// 先成的那一份；只持别种授予的答 403 OPERATOR_NOT_GRANTED，不默认放行。
func TestAnAuthorizedOperatorDecidesARerouteSuggestionOnTheDatabase(t *testing.T) {
	fixture := newRerouteFixture(t)

	status, answer := fixture.post(t, rerouteOutsider, "SYN-LINE-B@1")
	if status != http.StatusForbidden || answer["error"].(map[string]any)["code"] != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("只持别种授予：%d %v", status, answer)
	}
	if rows := fixture.decisionRows(t); rows != 0 {
		t.Fatalf("没授予也落了 %d 份决定", rows)
	}

	status, answer = fixture.post(t, rerouteOperatorA, "SYN-LINE-B@1")
	if status != http.StatusCreated || answer["outcome"] != "REROUTE_DECIDED" {
		t.Fatalf("决定：%d %v", status, answer)
	}
	decision := answer["decision"].(map[string]any)
	if decision["decidedBy"] != "OPERATOR/"+customsDecisionIssuer+"#"+rerouteOperatorA || decision["candidate"] != "SYN-LINE-B@1" ||
		decision["originalPlan"] != "SYN-RPV-CODE29-1" || decision["mode"] != "AUTHORIZED_ROLE" ||
		decision["effectiveFromNode"] != "SYN-NODE-SZX-HUB" || decision["effectiveFrom"] != decision["decidedAt"] {
		t.Fatalf("decision = %v", decision)
	}

	status, answer = fixture.post(t, rerouteOperatorB, "SYN-LINE-A@1")
	if status != http.StatusOK || answer["outcome"] != "SUGGESTION_ALREADY_DECIDED" ||
		answer["decision"].(map[string]any)["newPlanVersion"] != decision["newPlanVersion"] {
		t.Fatalf("第二次决定：%d %v", status, answer)
	}
	if rows := fixture.decisionRows(t); rows != 1 {
		t.Fatalf("决定册 %d 行, want 1", rows)
	}
	formed, err := fixture.reassessments.FormedPlans(t.Context(), fixture.key)
	if err != nil || len(formed) != 1 || formed[0].Version().String() != decision["newPlanVersion"] {
		t.Fatalf("路由历史里的新计划 = %v err = %v", formed, err)
	}
}

// Covers: AT-NR-096（ADR-0284 决定五「决定与新一次复核并发」）——一笔复核先取到判断键锁、在锁里落下一次业务时间更晚的复核：决定等在锁上，
// 复核提交之后它读到的建议已过时，答 SUGGESTION_SUPERSEDED，不落决定。决定不等锁，就会在那一笔提交之前按一份已过时的建议改路。
func TestARerouteDecisionWaitsForTheJudgmentKeyAndLosesToAConcurrentReassessment(t *testing.T) {
	fixture := newRerouteFixture(t)
	later := mustNRValue(t, nrdomain.NewRequestCorrelationID, "reassess/code29-later")
	lock := fixture.holdJudgmentKeyLock(t, func(txCtx context.Context) error {
		_, err := fixture.reassessments.Save(txCtx, later, nrports.ReassessmentRecord{
			Correlation:       later,
			Key:               fixture.key,
			Conclusion:        nrports.ReassessmentPlanLapsed,
			CandidateState:    nrports.NoQualifiedCandidates,
			TriggerOccurredAt: fixture.suggestedAt.Add(time.Minute),
			ReassessedAt:      time.Now(),
		})
		return err
	})

	type outcome struct {
		result nrapplication.DecideRerouteResult
		err    error
	}
	decided := make(chan outcome, 1)
	go func() {
		result, err := fixture.decider.Decide(context.Background(), fixture.command(t, rerouteOperatorA, "SYN-LINE-B@1"))
		decided <- outcome{result, err}
	}()
	fixture.waitForAdvisoryWaiters(t, 1)
	lock.release()
	if err := <-lock.done; err != nil {
		t.Fatalf("持锁那一笔：%v", err)
	}
	got := <-decided
	if got.err != nil || got.result.Outcome() != nrapplication.RerouteSuggestionSuperseded {
		t.Fatalf("outcome = %q err = %v, want SUGGESTION_SUPERSEDED", got.result.Outcome(), got.err)
	}
	if rows := fixture.decisionRows(t); rows != 0 {
		t.Fatalf("过时的建议落了 %d 份决定", rows)
	}
}

// Covers: AT-NR-096（ADR-0284 决定五「两个授权角色同时对同一份建议作决定」）——两笔决定都等在判断键锁上，放行后只有一笔成立，另一笔读到先成的
// 那一份、答 SUGGESTION_ALREADY_DECIDED；决定册只有一行。
func TestTwoConcurrentRerouteDecisionsFormOnlyOne(t *testing.T) {
	fixture := newRerouteFixture(t)
	lock := fixture.holdJudgmentKeyLock(t, nil)

	var wait sync.WaitGroup
	results := make([]nrapplication.DecideRerouteResult, 2)
	errs := make([]error, 2)
	for index, choice := range []struct{ operator, candidate string }{
		{rerouteOperatorA, "SYN-LINE-A@1"}, {rerouteOperatorB, "SYN-LINE-B@1"},
	} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index], errs[index] = fixture.decider.Decide(context.Background(), fixture.command(t, choice.operator, choice.candidate))
		}()
	}
	fixture.waitForAdvisoryWaiters(t, 2)
	lock.release()
	if err := <-lock.done; err != nil {
		t.Fatalf("持锁那一笔：%v", err)
	}
	wait.Wait()

	outcomes := map[nrapplication.DecideRerouteOutcome]int{}
	for index := range results {
		if errs[index] != nil {
			t.Fatalf("决定 %d：%v", index, errs[index])
		}
		outcomes[results[index].Outcome()]++
	}
	if outcomes[nrapplication.RerouteDecided] != 1 || outcomes[nrapplication.RerouteSuggestionAlreadyDecided] != 1 {
		t.Fatalf("outcomes = %v, want 一笔成立、一笔已决定", outcomes)
	}
	if rows := fixture.decisionRows(t); rows != 1 {
		t.Fatalf("决定册 %d 行, want 1", rows)
	}
}
