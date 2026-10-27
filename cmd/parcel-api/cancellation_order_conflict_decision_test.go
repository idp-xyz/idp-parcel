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
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证取消先后冲突解开口的生产装配（ADR-0295 决定一至四）：端点行按特性登记处那一份 mount 挂上，Intake 是生产那只操作者铸造器经真操作者册
// （access_identity 0026 放开的「取消先后冲突解开」一种）铸的信封；编排是 buildCancellationOrderConflictDecision 那一套真库缝（冲突册、决定留痕、终局库、续办信封交接、
// 包裹取消边界锁与事务壳），只有令牌核验方与准入范围是替身。
//   - 认定取消依据不成立：答 201，有效取消读不再交出那份取消（取消记录仍在），依据它的终局不采用行作废，续办信封恰一封；查阅里随之消失；另一位再决定答 200
//     并读回先到的去向、不多一封信封。
//   - 交来源方更正：答 201，取消照旧有效，不采用行不动，没有续办信封。
//   - 并发：多位同时决定同一份冲突，恰一笔成立、恰一封信封。
//   - 授权：只持授权处置授予、没有令牌、载荷自报租户都进不来；别的租户的操作者读不到本租户的冲突（统一不可见 404）；以上都什么也没写。
//   - 事务外拒跑（PBC-08）：编排在无事务上下文拿不到取消边界锁，决定、作废与信封零行。
// 夹具全为合成（S 级）。

const (
	conflictTenant        = "SYN-TENANT-1"
	conflictDeciderA      = "SYN-DECIDER-CODE76-A"
	conflictDeciderB      = "SYN-DECIDER-CODE76-B"
	conflictDispositioner = "SYN-DISPOSITIONER-CODE76"
	conflictForeigner     = "SYN-FOREIGNER-CODE76"
	conflictForeignTenant = "SYN-TENANT-FOREIGN"
	conflictEventType     = "parcel-shipment.parcel-cancellation.not-upheld"
	conflictDecisionPath  = "/cancellation-order-conflict-decisions"
	conflictListPath      = "/cancellation-order-conflicts"
)

var (
	conflictCancelledAt = time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	conflictBoundaryAt  = conflictCancelledAt.Add(-time.Hour)
)

type conflictDecisionE2E struct {
	db        *bentopg.DB
	pool      *pgxpool.Pool
	endpoints map[string]http.Handler
	inner     transactionalCancellationOrderConflictDecision
}

func newConflictDecisionE2E(t *testing.T) *conflictDecisionE2E {
	t.Helper()
	// pgtest 的池只开两条连接；并发用例要多笔事务同时在库里，另开一只同库的池。
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

	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	registerOperatorIn(t, db, operators, conflictTenant, conflictDeciderA, interval, accessidentity.DecisionCancellationOrderConflictResolution)
	registerOperatorIn(t, db, operators, conflictTenant, conflictDeciderB, interval, accessidentity.DecisionCancellationOrderConflictResolution)
	registerOperatorIn(t, db, operators, conflictTenant, conflictDispositioner, interval, accessidentity.DecisionAuthorizedDisposition, accessidentity.DecisionControlReleaseRefusalHandling)
	registerOperatorIn(t, db, operators, conflictForeignTenant, conflictForeigner, interval, accessidentity.DecisionCancellationOrderConflictResolution)
	registerOperationsReader(t, db, operators, conflictTenant, conflictDeciderA)
	registerOperationsReader(t, db, operators, conflictForeignTenant, conflictForeigner)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	views, err := pspostgres.NewShipmentRequestViews(db)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, views, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := buildCancellationOrderConflictDecision(db)
	if err != nil {
		t.Fatalf("装配决定编排：%v", err)
	}
	catalogue, err := pspostgres.NewCancellationOrderConflictCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &conflictDecisionE2E{db: db, pool: pool, endpoints: map[string]http.Handler{}}
	fixture.inner = decision.(transactionalCancellationOrderConflictDecision)
	channels := endpointChannels{operatorDecisions: decisions, operatorRegistries: registries}
	for _, feature := range endpointRegistry {
		switch feature.wiringKey {
		case any((*cancellationOrderConflictDecisionWiring)(nil)):
			for _, endpoint := range feature.mountWith(channels, cancellationOrderConflictDecisionWiring{decision: decision}) {
				fixture.endpoints[endpoint.Pattern] = endpoint.Handler
			}
		case any((*cancellationOrderConflictsWiring)(nil)):
			for _, endpoint := range feature.mountWith(channels, cancellationOrderConflictsWiring{conflicts: catalogue}) {
				fixture.endpoints[endpoint.Pattern] = endpoint.Handler
			}
		}
	}
	if fixture.endpoints[conflictDecisionPath] == nil || fixture.endpoints[conflictListPath] == nil {
		t.Fatal("决定口与查阅口没按登记处挂上")
	}
	return fixture
}

// arrange 摆出一件被取消挡住的包裹：取消 PCXL-1 成立，收寄来源（NODE_INTAKE/v1）发生在取消之前而后到，冲突入册；终局采用另有一条依据这份取消的不采用行。
func (fixture *conflictDecisionE2E) arrange(t *testing.T, parcel string) {
	t.Helper()
	parcelID := mustValue(t, domain.NewDeclaredParcelID, parcel)
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustValue(t, domain.NewParcelCancellationID, "PCXL-1"),
		Parcel:      parcelID,
		Requester:   mustValue(t, domain.NewCancellationRequesterReference, "customer-a"),
		Authority:   mustValue(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/1"),
		Reason:      mustValue(t, domain.NewCancellationReasonReference, "CHANGED_MIND"),
		RequestedAt: conflictCancelledAt,
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("形成取消决定：%v", err)
	}
	conflict, conflicted, err := domain.JudgeCancellationOrder(cancellation, domain.CancellationBoundaryFact{
		Kind:       domain.NetworkIntakeBoundary,
		Reference:  mustValue(t, domain.NewCancellationBoundaryReference, "NODE_INTAKE/v1"),
		OccurredAt: conflictBoundaryAt,
	})
	if err != nil || !conflicted {
		t.Fatalf("判冲突：%v conflicted=%v", err, conflicted)
	}
	tenant := mustValue(t, domain.NewTenantID, conflictTenant)
	cancellations, err := pspostgres.NewParcelCancellations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	conflicts, err := pspostgres.NewCancellationOrderConflicts(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	finals, err := pspostgres.NewFinalOutcomes(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	deliverySpec := domain.ResponsibilityOutcomeSpec{
		Kind: domain.EffectiveDeliveryOutcome, Parcel: parcelID,
		Decision:   mustValue(t, domain.NewResponsibilityDecisionReference, "DELIVERY-JUDGMENT/"+parcel+"/ATT-1"),
		Execution:  mustValue(t, domain.NewExecutionEvidenceReference, "POD-1"),
		Version:    mustValue(t, domain.NewResponsibilityOutcomeVersion, "DLV-v1"),
		OccurredAt: conflictCancelledAt.Add(2 * time.Hour),
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := cancellations.Save(txCtx, psports.CancellationRecord{
			Key: psports.CancellationRequestKey{
				TenantID: tenant, RequestKey: mustValue(t, domain.NewSourceRequestKey, "cancel-key-1"), Parcel: parcelID,
			},
			ContentDigest: "digest-cancel-1", Kind: psports.RecordParcelCancelled, Cancellation: cancellation,
			DecidedAt: conflictCancelledAt.Add(time.Minute),
		}); err != nil {
			return err
		}
		if _, err := conflicts.Record(txCtx, psports.CancellationOrderConflictRecord{
			TenantID: tenant, Conflict: conflict, DetectedAt: conflictCancelledAt.Add(time.Hour),
		}); err != nil {
			return err
		}
		_, err := finals.Save(txCtx, psports.FinalOutcomeRecord{
			Key: psports.FinalAdoptionKey{
				TenantID: tenant, Parcel: parcelID, Kind: domain.EffectiveDeliveryOutcome, Version: deliverySpec.Version,
			},
			ContentDigest: "digest-delivery-1",
			RefusalBasis:  mustValue(t, domain.NewCheckReason, "PARCEL_CANCELLATION/PCXL-1"),
			AdoptedAt:     conflictCancelledAt.Add(3 * time.Hour),
			CancellationReplay: &psports.CancellationBlockedReplay{
				Cancellation: cancellation.ID(), Final: &deliverySpec,
			},
		})
		return err
	}); err != nil {
		t.Fatalf("摆夹具：%v", err)
	}
}

func conflictBody(parcel, decision string) string {
	return `{"parcel":"` + parcel + `","boundaryKind":"NETWORK_INTAKE","boundaryRef":"NODE_INTAKE/v1","cancellation":"PCXL-1","decision":"` +
		decision + `","basis":"SYN-NODE-SCAN-AT-0700"}`
}

func (fixture *conflictDecisionE2E) post(token, body string) (int, string) {
	return fixture.postTo(conflictDecisionPath, token, body)
}

func (fixture *conflictDecisionE2E) postTo(target, token, body string) (int, string) {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[conflictDecisionPath].ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

func (fixture *conflictDecisionE2E) list(t *testing.T, token string) (int, int, int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, conflictListPath, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[conflictListPath].ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		return response.Code, 0, 0
	}
	var body struct {
		UnresolvedCount int   `json:"unresolvedCount"`
		Conflicts       []any `json:"conflicts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("查阅答复不是 JSON：%s", response.Body.String())
	}
	return response.Code, body.UnresolvedCount, len(body.Conflicts)
}

func (fixture *conflictDecisionE2E) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := fixture.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("计数 %q：%v", query, err)
	}
	return n
}

func (fixture *conflictDecisionE2E) decisionRows(t *testing.T) int {
	return fixture.count(t, `SELECT count(*) FROM parcel_shipment.cancellation_order_conflict_decision`)
}

func (fixture *conflictDecisionE2E) envelopes(t *testing.T) int {
	return fixture.count(t, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`, conflictEventType)
}

func (fixture *conflictDecisionE2E) refusalRows(t *testing.T) int {
	return fixture.count(t, `SELECT count(*) FROM parcel_shipment.final_outcome WHERE NOT finalized`)
}

func (fixture *conflictDecisionE2E) effectiveCancellation(t *testing.T, parcel string) bool {
	t.Helper()
	cancellations, err := pspostgres.NewParcelCancellations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := cancellations.FindCancellation(t.Context(), mustValue(t, domain.NewTenantID, conflictTenant), mustValue(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("读有效取消：%v", err)
	}
	return found
}

func conflictFieldsOf(t *testing.T, raw string) (string, string) {
	t.Helper()
	var body struct {
		Outcome      string `json:"outcome"`
		DecisionKind string `json:"decisionKind"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("答复不是 JSON：%s", raw)
	}
	return body.Outcome, body.DecisionKind
}

// Covers: AT-PS-166, AT-PS-168, AT-PS-169, AT-PS-170；ADR-0295 决定一至四——认定取消依据不成立：答 201 DECIDED 带作废行数，决定留痕（去向、决定人取认证出的操作者、依据）、
// 依据这份取消的终局不采用行作废、续办信封同笔落库；有效取消读不再交出这份取消而取消记录仍在；查阅里随之消失；另一位再决定（即便说的是另一种去向）答 200 ALREADY_DECIDED
// 读回先到的去向、不多一封信封，回执不带操作者。
func TestTheWiredDecisionNotUpholdsTheCancellationAndHandsOffOneResume(t *testing.T) {
	fixture := newConflictDecisionE2E(t)
	fixture.arrange(t, "SYN-PARCEL-1")
	if status, unresolved, listed := fixture.list(t, conflictDeciderA); status != http.StatusOK || unresolved != 1 || listed != 1 {
		t.Fatalf("决定前查阅 = %d 计数 %d 行 %d, want 200 与 1 与 1", status, unresolved, listed)
	}
	if !fixture.effectiveCancellation(t, "SYN-PARCEL-1") || fixture.refusalRows(t) != 1 {
		t.Fatal("夹具没摆出有效取消与依据它的不采用行")
	}

	status, raw := fixture.post(conflictDeciderA, conflictBody("SYN-PARCEL-1", "CANCELLATION_NOT_UPHELD"))
	outcome, kind := conflictFieldsOf(t, raw)
	if status != http.StatusCreated || outcome != "DECIDED" || kind != "CANCELLATION_NOT_UPHELD" || !strings.Contains(raw, `"voidedRefusals":1`) {
		t.Fatalf("决定 = %d %s, want 201 DECIDED CANCELLATION_NOT_UPHELD 作废 1 行", status, raw)
	}
	var storedKind, decider, basis string
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT decision_kind, decided_by, decision_basis FROM parcel_shipment.cancellation_order_conflict_decision`).Scan(&storedKind, &decider, &basis); err != nil {
		t.Fatalf("读决定留痕：%v", err)
	}
	if storedKind != "CANCELLATION_NOT_UPHELD" || decider != customsDecisionIssuer+"#"+conflictDeciderA || basis != "SYN-NODE-SCAN-AT-0700" {
		t.Fatalf("留痕 = %q %q %q, want 去向、认证出的操作者「发行方#sub」、依据", storedKind, decider, basis)
	}
	if fixture.effectiveCancellation(t, "SYN-PARCEL-1") {
		t.Fatal("认定不成立之后有效取消读仍交出那份取消")
	}
	if n := fixture.count(t, `SELECT count(*) FROM parcel_shipment.parcel_cancellation WHERE kind = 'PARCEL_CANCELLED'`); n != 1 {
		t.Fatalf("取消记录 %d 行, want 1——取消作历史仍在", n)
	}
	if fixture.refusalRows(t) != 0 {
		t.Fatalf("依据这份取消的不采用行仍有 %d 行", fixture.refusalRows(t))
	}
	if n := fixture.count(t, `SELECT count(*) FROM parcel_shipment.cancellation_blocked_replay`); n != 1 {
		t.Fatalf("回放原料 %d 行, want 1——作废的是不采用行，原料留给续办", n)
	}
	if fixture.envelopes(t) != 1 {
		t.Fatalf("续办信封 %d 封, want 恰好 1", fixture.envelopes(t))
	}
	var payload struct {
		TenantID     string `json:"tenantId"`
		Parcel       string `json:"parcel"`
		Cancellation string `json:"cancellation"`
		BoundaryKind string `json:"boundaryKind"`
		BoundaryRef  string `json:"boundaryRef"`
	}
	var envelope []byte
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`, conflictEventType).Scan(&envelope); err != nil {
		t.Fatalf("读信封：%v", err)
	}
	if err := json.Unmarshal(envelope, &payload); err != nil || payload.TenantID != conflictTenant || payload.Parcel != "SYN-PARCEL-1" ||
		payload.Cancellation != "PCXL-1" || payload.BoundaryKind != "NETWORK_INTAKE" || payload.BoundaryRef != "NODE_INTAKE/v1" {
		t.Fatalf("信封载荷 = %s", envelope)
	}
	if status, unresolved, listed := fixture.list(t, conflictDeciderA); status != http.StatusOK || unresolved != 0 || listed != 0 {
		t.Fatalf("决定后查阅 = %d 计数 %d 行 %d, want 200 与 0 与 0", status, unresolved, listed)
	}

	status, raw = fixture.post(conflictDeciderB, conflictBody("SYN-PARCEL-1", "BOUNDARY_FACT_REFERRED"))
	outcome, kind = conflictFieldsOf(t, raw)
	if status != http.StatusOK || outcome != "ALREADY_DECIDED" || kind != "CANCELLATION_NOT_UPHELD" {
		t.Fatalf("另一位再决定 = %d %s, want 200 ALREADY_DECIDED 且读回先到的去向", status, raw)
	}
	if strings.Contains(raw, "SYN-DECIDER") || strings.Contains(raw, "id.syn.example") {
		t.Fatalf("回执带了操作者：%s", raw)
	}
	if fixture.decisionRows(t) != 1 || fixture.envelopes(t) != 1 {
		t.Fatalf("再决定之后留痕 %d 行、信封 %d 封, want 仍各 1", fixture.decisionRows(t), fixture.envelopes(t))
	}
}

// Covers: AT-PS-166, AT-PS-171；ADR-0295 决定二——交来源方更正：答 201 DECIDED，取消照旧有效，终局不采用行不动，没有续办信封（没有被挡住的来源要重驱）；查阅里随之消失。
func TestTheWiredReferralLeavesTheCancellationStandingAndHandsOffNothing(t *testing.T) {
	fixture := newConflictDecisionE2E(t)
	fixture.arrange(t, "SYN-PARCEL-1")

	status, raw := fixture.post(conflictDeciderB, conflictBody("SYN-PARCEL-1", "BOUNDARY_FACT_REFERRED"))
	outcome, kind := conflictFieldsOf(t, raw)
	if status != http.StatusCreated || outcome != "DECIDED" || kind != "BOUNDARY_FACT_REFERRED" || strings.Contains(raw, "voidedRefusals") {
		t.Fatalf("决定 = %d %s, want 201 DECIDED BOUNDARY_FACT_REFERRED 不带作废行数", status, raw)
	}
	if !fixture.effectiveCancellation(t, "SYN-PARCEL-1") || fixture.refusalRows(t) != 1 {
		t.Fatal("交来源方更正动了取消或不采用行")
	}
	if fixture.envelopes(t) != 0 {
		t.Fatalf("续办信封 %d 封, want 0", fixture.envelopes(t))
	}
	if _, unresolved, _ := fixture.list(t, conflictDeciderA); unresolved != 0 {
		t.Fatalf("决定后查阅计数 = %d, want 0", unresolved)
	}
}

// Covers: AT-PS-166；ADR-0295 决定一「一份冲突至多被决定一次」——多路同时决定同一份冲突（去向还不一致），恰一路答 201，其余答 200 ALREADY_DECIDED；留痕一行；
// 认定取消不成立赢时恰一封信封，交来源方更正赢时零封。
func TestConcurrentDecidersOfOneWiredConflictLeaveOneDecision(t *testing.T) {
	fixture := newConflictDecisionE2E(t)
	fixture.arrange(t, "SYN-PARCEL-1")

	const deciders = 6
	statuses := make([]int, deciders)
	bodies := make([]string, deciders)
	var group sync.WaitGroup
	start := make(chan struct{})
	for index := range deciders {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			token, decision := conflictDeciderA, "CANCELLATION_NOT_UPHELD"
			if index%2 == 1 {
				token, decision = conflictDeciderB, "BOUNDARY_FACT_REFERRED"
			}
			statuses[index], bodies[index] = fixture.post(token, conflictBody("SYN-PARCEL-1", decision))
		}()
	}
	close(start)
	group.Wait()

	created, winnerKind := 0, ""
	for index := range deciders {
		outcome, kind := conflictFieldsOf(t, bodies[index])
		switch {
		case statuses[index] == http.StatusCreated && outcome == "DECIDED":
			created++
			winnerKind = kind
		case statuses[index] == http.StatusOK && outcome == "ALREADY_DECIDED":
		default:
			t.Fatalf("第 %d 路 = %d %s, want 201 DECIDED 或 200 ALREADY_DECIDED", index, statuses[index], bodies[index])
		}
	}
	if created != 1 {
		t.Fatalf("成立了 %d 笔, want 恰好 1", created)
	}
	wantEnvelopes := 0
	if winnerKind == "CANCELLATION_NOT_UPHELD" {
		wantEnvelopes = 1
	}
	if fixture.decisionRows(t) != 1 || fixture.envelopes(t) != wantEnvelopes {
		t.Fatalf("留痕 %d 行、信封 %d 封, want 1 行与 %d 封（赢家 %s）", fixture.decisionRows(t), fixture.envelopes(t), wantEnvelopes, winnerKind)
	}
	for index := range deciders {
		if _, kind := conflictFieldsOf(t, bodies[index]); kind != winnerKind {
			t.Fatalf("第 %d 路答的去向 %s, want 都是赢家的 %s", index, kind, winnerKind)
		}
	}
}

// Covers: AT-PS-166, AT-PS-167；ADR-0295 决定二、ADR-0151 决定二——授权：只持授权处置与释放拒收处理授予的操作者进不来（几种授予不通用），没有令牌 401，载荷自报租户 400，
// 册上没有的那份冲突 404，别的租户的操作者读不到本租户的冲突（统一不可见 404）；查阅同理：没有查阅读授予 403，别的租户只看见自己的（空）册；以上都什么也没写。
func TestTheWiredDecisionIsOpenOnlyToItsOwnGrantWithinTheOwnTenant(t *testing.T) {
	fixture := newConflictDecisionE2E(t)
	fixture.arrange(t, "SYN-PARCEL-1")
	good := conflictBody("SYN-PARCEL-1", "CANCELLATION_NOT_UPHELD")

	for name, test := range map[string]struct {
		token  string
		body   string
		status int
		code   string
	}{
		"只持别种授予":   {conflictDispositioner, good, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"没有令牌":     {"", good, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"载荷带租户":    {conflictDeciderA, strings.Replace(good, `"decision"`, `"tenantId":"SYN-TENANT-FOREIGN","decision"`, 1), http.StatusBadRequest, "MALFORMED_REQUEST"},
		"别的租户":     {conflictForeigner, good, http.StatusNotFound, "CANCELLATION_ORDER_CONFLICT_NOT_VISIBLE"},
		"册上没有这一代":  {conflictDeciderA, strings.Replace(good, "NODE_INTAKE/v1", "NODE_INTAKE/v9", 1), http.StatusNotFound, "CANCELLATION_ORDER_CONFLICT_NOT_VISIBLE"},
		"册上没有这份取消": {conflictDeciderA, strings.Replace(good, "PCXL-1", "PCXL-9", 1), http.StatusNotFound, "CANCELLATION_ORDER_CONFLICT_NOT_VISIBLE"},
	} {
		status, raw := fixture.post(test.token, test.body)
		if status != test.status || !strings.Contains(raw, test.code) {
			t.Errorf("%s = %d %s, want %d %s", name, status, raw, test.status, test.code)
		}
	}
	// 别的租户的操作者在请求上自报本租户：租户只取信封，冲突仍落在它自己的租户里查不到。
	status, raw := fixture.postTo(conflictDecisionPath+"?tenant="+conflictTenant, conflictForeigner, good)
	if status != http.StatusNotFound || !strings.Contains(raw, "CANCELLATION_ORDER_CONFLICT_NOT_VISIBLE") {
		t.Errorf("别的租户自报本租户 = %d %s, want 404 统一不可见——请求里自报的租户不采信", status, raw)
	}
	if fixture.decisionRows(t) != 0 || fixture.envelopes(t) != 0 || !fixture.effectiveCancellation(t, "SYN-PARCEL-1") || fixture.refusalRows(t) != 1 {
		t.Fatalf("被拒的请求写了东西：留痕 %d 行、信封 %d 封", fixture.decisionRows(t), fixture.envelopes(t))
	}

	if status, _, _ := fixture.list(t, conflictDeciderB); status != http.StatusForbidden {
		t.Errorf("没有查阅读授予 = %d, want 403", status)
	}
	if status, _, _ := fixture.list(t, ""); status != http.StatusUnauthorized {
		t.Errorf("没有令牌的查阅 = %d, want 401", status)
	}
	if status, unresolved, listed := fixture.list(t, conflictForeigner); status != http.StatusOK || unresolved != 0 || listed != 0 {
		t.Errorf("别的租户的查阅 = %d 计数 %d 行 %d, want 200 与 0 与 0——只看得见自己租户的册", status, unresolved, listed)
	}
}

// Covers: AT-PS-166；PBC-08 行为面负向证据——决定编排在无事务上下文拿不到包裹取消边界锁（锁是事务级咨询锁），整笔拒跑，决定、作废与信封零行；事务壳之内才成立。
func TestTheDecisionOrchestrationRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newConflictDecisionE2E(t)
	fixture.arrange(t, "SYN-PARCEL-1")
	command := shipmentapp.DecideCancellationOrderConflictCommand{
		Tenant:       mustValue(t, domain.NewTenantID, conflictTenant),
		Parcel:       mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-1"),
		BoundaryKind: domain.NetworkIntakeBoundary,
		BoundaryRef:  mustValue(t, domain.NewCancellationBoundaryReference, "NODE_INTAKE/v1"),
		Cancellation: mustValue(t, domain.NewParcelCancellationID, "PCXL-1"),
		Kind:         domain.CancellationNotUpheld,
		Decider:      mustValue(t, domain.NewCancellationOrderConflictDeciderReference, "https://id.syn.example#alice"),
		Basis:        mustValue(t, domain.NewCancellationOrderConflictDecisionBasis, "SYN-BASIS"),
	}
	if _, err := fixture.inner.inner.Handle(t.Context(), command); !errors.Is(err, shipmentapp.ErrCancellationBoundaryNotLocked) {
		t.Fatalf("无事务决定应拒跑（取不到边界锁），实得：%v", err)
	}
	if fixture.decisionRows(t) != 0 || fixture.envelopes(t) != 0 || fixture.refusalRows(t) != 1 {
		t.Fatalf("被拒的编排写了东西：留痕 %d 行、信封 %d 封、不采用行 %d", fixture.decisionRows(t), fixture.envelopes(t), fixture.refusalRows(t))
	}
	if _, err := fixture.inner.Handle(t.Context(), command); err != nil {
		t.Fatalf("事务壳之内应成立：%v", err)
	}
	if fixture.decisionRows(t) != 1 || fixture.envelopes(t) != 1 {
		t.Fatalf("事务壳之内：留痕 %d 行、信封 %d 封, want 各 1", fixture.decisionRows(t), fixture.envelopes(t))
	}
}
