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

// 本文件对真实 PostgreSQL 证收寄后服务处置决定口的生产装配（ADR-0332）：端点行按特性登记处那一份 mount 挂上，Intake 是生产那只操作者铸造器经真操作者册
// （access_identity 0028 放开的「收寄后服务处置」一种）铸的信封；编排是 buildPostIntakeDisposition 那一套真库缝（决定链、请求册、采用链、终局库、取消册、信封交接、
// 取消边界锁与事务壳），只有令牌核验方与准入范围是替身。夹具全为合成（S 级）。

const (
	pidTenant      = "SYN-TENANT-1"
	pidDecider     = "SYN-DECIDER-CODE37-A"
	pidDeciderB    = "SYN-DECIDER-CODE37-B"
	pidOtherGrant  = "SYN-OTHERGRANT-CODE37"
	pidForeigner   = "SYN-FOREIGNER-CODE37"
	pidForeignTen  = "SYN-TENANT-FOREIGN"
	pidDecisionURL = "/post-intake-disposition-decisions"
	pidListURL     = "/post-intake-disposition-requests"
	pidEventType   = "parcel-shipment.post-intake-disposition.decided"
	pidIntakeAt    = "2026-10-04T08:00:00Z"
)

type postIntakeE2E struct {
	db        *bentopg.DB
	pool      *pgxpool.Pool
	endpoints map[string]http.Handler
	inner     transactionalPostIntakeDisposition
}

func newPostIntakeE2E(t *testing.T) *postIntakeE2E {
	t.Helper()
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
	registerOperatorIn(t, db, operators, pidTenant, pidDecider, interval, accessidentity.DecisionPostIntakeServiceDisposition)
	registerOperatorIn(t, db, operators, pidTenant, pidDeciderB, interval, accessidentity.DecisionPostIntakeServiceDisposition)
	registerOperatorIn(t, db, operators, pidTenant, pidOtherGrant, interval, accessidentity.DecisionAuthorizedDisposition, accessidentity.DecisionCancellationOrderConflictResolution)
	registerOperatorIn(t, db, operators, pidForeignTen, pidForeigner, interval, accessidentity.DecisionPostIntakeServiceDisposition)
	registerOperationsReader(t, db, operators, pidTenant, pidDecider)
	registerOperationsReader(t, db, operators, pidForeignTen, pidForeigner)
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
	decision, err := buildPostIntakeDisposition(db)
	if err != nil {
		t.Fatalf("装配决定编排：%v", err)
	}
	requests, err := pspostgres.NewPostIntakeDispositionRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &postIntakeE2E{db: db, pool: pool, endpoints: map[string]http.Handler{}}
	fixture.inner = decision.(transactionalPostIntakeDisposition)
	channels := endpointChannels{operatorDecisions: decisions, operatorRegistries: registries}
	for _, feature := range endpointRegistry {
		if feature.wiringKey == any((*postIntakeDispositionWiring)(nil)) {
			for _, endpoint := range feature.mountWith(channels, postIntakeDispositionWiring{decision: decision, requests: requests}) {
				fixture.endpoints[endpoint.Pattern] = endpoint.Handler
			}
		}
	}
	if fixture.endpoints[pidDecisionURL] == nil || fixture.endpoints[pidListURL] == nil {
		t.Fatal("决定口与查阅口没按登记处挂上")
	}
	return fixture
}

// arrangeCrossed 摆出一件已越过取消边界的包裹：收寄采用在册（责任起点），客户的取消请求停在待处置，请求四件随记录保全。
func (fixture *postIntakeE2E) arrangeCrossed(t *testing.T, parcel, requestKey string) {
	t.Helper()
	tenant := mustValue(t, domain.NewTenantID, pidTenant)
	parcelID := mustValue(t, domain.NewDeclaredParcelID, parcel)
	adoptions, err := pspostgres.NewIntakeAdoptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	cancellations, err := pspostgres.NewParcelCancellations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	intakeAt, _ := time.Parse(time.RFC3339, pidIntakeAt)
	version := mustValue(t, domain.NewSourceResultVersion, "intake-result/v1")
	source, err := domain.NewIntakeSource(domain.IntakeSourceSpec{
		Kind:       domain.NodeIntakeSource,
		Object:     mustValue(t, domain.NewSourceObjectReference, "SYN-UNIT-"+parcel),
		Parcel:     parcelID,
		Place:      mustValue(t, domain.NewIntakePlaceReference, "SYN-NODE-ORIGIN"),
		Control:    mustValue(t, domain.NewIntakeControlReference, "SYN-CONTROL-"+parcel),
		Version:    version,
		OccurredAt: intakeAt,
	})
	if err != nil {
		t.Fatalf("收寄来源：%v", err)
	}
	intake, err := domain.AdoptNetworkIntake(source, mustValue(t, domain.NewSubmissionVersionID, "SYN-VERSION-1"))
	if err != nil {
		t.Fatalf("采用收寄：%v", err)
	}
	commitment, err := domain.FormFormalCommitment(
		mustValue(t, domain.NewCommitmentVersionID, "SYN-CMT-"+parcel), intake,
		mustValue(t, domain.NewExpectedCommitmentReference, "SYN-RES-1@2026-10-01T00:00:00Z"))
	if err != nil {
		t.Fatalf("正式承诺：%v", err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := adoptions.Save(txCtx, psports.IntakeAdoptionRecord{
			Key:               psports.IntakeAdoptionKey{TenantID: tenant, Parcel: parcelID, Kind: domain.NodeIntakeSource, Version: version},
			CustomerAccountID: mustValue(t, domain.NewCustomerAccountID, "SYN-ACCOUNT-1"),
			ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "SYN-REQUEST-"+parcel),
			ContentDigest:     "digest-adoption-" + parcel,
			Adopted:           true,
			Intake:            intake,
			Commitment:        commitment,
			AdoptedAt:         intakeAt.Add(time.Minute),
		}); err != nil {
			return err
		}
		_, err := cancellations.Save(txCtx, psports.CancellationRecord{
			Key: psports.CancellationRequestKey{
				TenantID: tenant, RequestKey: mustValue(t, domain.NewSourceRequestKey, requestKey), Parcel: parcelID,
			},
			ContentDigest: "digest-pending-" + requestKey,
			Kind:          psports.RecordDispositionPending,
			IntakeVersion: version,
			StopRequest: psports.StopRequest{
				Requester:   mustValue(t, domain.NewCancellationRequesterReference, "customer-a"),
				Authority:   mustValue(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/1"),
				Reason:      mustValue(t, domain.NewCancellationReasonReference, "CHANGED_MIND"),
				RequestedAt: intakeAt.Add(2 * time.Hour),
			},
			DecidedAt: intakeAt.Add(2*time.Hour + time.Minute),
		})
		return err
	}); err != nil {
		t.Fatalf("摆夹具：%v", err)
	}
}

func (fixture *postIntakeE2E) post(token, body string) (int, string) {
	return fixture.postTo(pidDecisionURL, token, body)
}

func (fixture *postIntakeE2E) postTo(target, token, body string) (int, string) {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[pidDecisionURL].ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

func (fixture *postIntakeE2E) list(t *testing.T, token string) (int, int, int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, pidListURL, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[pidListURL].ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		return response.Code, 0, 0
	}
	var body struct {
		PendingCount int   `json:"pendingCount"`
		Requests     []any `json:"requests"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("查阅答复不是 JSON：%s", response.Body.String())
	}
	return response.Code, body.PendingCount, len(body.Requests)
}

func (fixture *postIntakeE2E) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := fixture.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("计数 %q：%v", query, err)
	}
	return n
}

func (fixture *postIntakeE2E) decisionRows(t *testing.T) int {
	return fixture.count(t, `SELECT count(*) FROM parcel_shipment.post_intake_disposition_decision`)
}

func (fixture *postIntakeE2E) envelopes(t *testing.T) int {
	return fixture.count(t, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`, pidEventType)
}

func pidBody(parcel string, supersedes int, direction, requestKey, destination string) string {
	body := `{"parcel":"` + parcel + `","supersedes":` + itoa(supersedes) + `,"direction":"` + direction + `","basis":"SYN-CHECKED-PRODUCT-AND-CONTRACT"`
	if requestKey != "" {
		body += `,"requestKey":"` + requestKey + `"`
	}
	if destination != "" {
		body += `,"destination":"` + destination + `"`
	}
	return body + `}`
}

func itoa(n int) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

type pidAnswer struct {
	Outcome  string `json:"outcome"`
	Decision *struct {
		Version       int    `json:"version"`
		Direction     string `json:"direction"`
		Source        string `json:"source"`
		DecidedAt     string `json:"decidedAt"`
		OwnerRequests []struct {
			Owner  string `json:"owner"`
			Action string `json:"action"`
		} `json:"ownerRequests"`
	} `json:"decision"`
}

func pidAnswerOf(t *testing.T, raw string) pidAnswer {
	t.Helper()
	var answer pidAnswer
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		t.Fatalf("答复不是 JSON：%s", raw)
	}
	return answer
}

// Covers: AT-PS-174；ADR-0332 决定一至四——越界待处置的请求被一份普通退运决定回应：答 201 DECIDED，决定链首版留痕（来源客户请求、决定人取认证出的
// 操作者、依据），向路由、节点、运输三个所有者各提出一项范围化请求，信封恰一封且依据种类是服务处置决定；等回应清单里随之消失；原样重发答 200 ALREADY_DECIDED 读回
// 同一份、不多一封；别的决定再回应同一请求答 REQUEST_ALREADY_ANSWERED；运营发起的后一份取代前一份，信封另一封，前一版不动；链尾对不上答 PREDECESSOR_MOVED。
func TestTheWiredDecisionAnswersAPendingRequestAndHandsTheOwnersTheirScopedRequests(t *testing.T) {
	fixture := newPostIntakeE2E(t)
	fixture.arrangeCrossed(t, "SYN-PARCEL-1", "cancel-key-1")
	if status, pending, listed := fixture.list(t, pidDecider); status != http.StatusOK || pending != 1 || listed != 1 {
		t.Fatalf("决定前查阅 = %d 计数 %d 行 %d, want 200 与 1 与 1", status, pending, listed)
	}

	status, raw := fixture.post(pidDecider, pidBody("SYN-PARCEL-1", 0, "RETURN", "cancel-key-1", ""))
	answer := pidAnswerOf(t, raw)
	if status != http.StatusCreated || answer.Outcome != "DECIDED" || answer.Decision == nil ||
		answer.Decision.Version != 1 || answer.Decision.Direction != "RETURN" || answer.Decision.Source != "CUSTOMER_REQUEST" ||
		len(answer.Decision.OwnerRequests) != 3 {
		t.Fatalf("决定 = %d %s, want 201 DECIDED 首版 RETURN 来源客户请求 三项所有者请求", status, raw)
	}
	var decider, basis string
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT decided_by, decision_basis FROM parcel_shipment.post_intake_disposition_decision`).Scan(&decider, &basis); err != nil {
		t.Fatalf("读决定留痕：%v", err)
	}
	if decider != customsDecisionIssuer+"#"+pidDecider || basis != "SYN-CHECKED-PRODUCT-AND-CONTRACT" {
		t.Fatalf("留痕 = %q %q, want 认证出的操作者「发行方#sub」与依据", decider, basis)
	}
	if strings.Contains(raw, "SYN-DECIDER") {
		t.Fatalf("回执带了操作者：%s", raw)
	}
	var envelope []byte
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`, pidEventType).Scan(&envelope); err != nil {
		t.Fatalf("读信封：%v", err)
	}
	var payload struct {
		Parcel          string `json:"parcel"`
		Direction       string `json:"direction"`
		BasisKind       string `json:"basisKind"`
		BasisReference  string `json:"basisReference"`
		OriginalJourney string `json:"originalJourney"`
		Requests        []any  `json:"requests"`
	}
	if err := json.Unmarshal(envelope, &payload); err != nil || payload.Parcel != "SYN-PARCEL-1" || payload.Direction != "RETURN" ||
		payload.BasisKind != "SERVICE_DISPOSITION" || payload.BasisReference != "POST_INTAKE_DISPOSITION/SYN-PARCEL-1/1" ||
		payload.OriginalJourney != "ASSOCIATED_INDEPENDENT" || len(payload.Requests) != 3 {
		t.Fatalf("信封载荷 = %s", envelope)
	}
	if status, pending, _ := fixture.list(t, pidDecider); status != http.StatusOK || pending != 0 {
		t.Fatalf("决定后查阅 = %d 计数 %d, want 200 与 0", status, pending)
	}

	status, raw = fixture.post(pidDecider, pidBody("SYN-PARCEL-1", 0, "RETURN", "cancel-key-1", ""))
	replay := pidAnswerOf(t, raw)
	if status != http.StatusOK || replay.Outcome != "ALREADY_DECIDED" || replay.Decision == nil || replay.Decision.DecidedAt != answer.Decision.DecidedAt {
		t.Fatalf("原样重发 = %d %s, want 200 ALREADY_DECIDED 且决定时刻与首次答复一致", status, raw)
	}
	status, raw = fixture.post(pidDeciderB, pidBody("SYN-PARCEL-1", 0, "INTERCEPT", "cancel-key-1", ""))
	if status != http.StatusOK || pidAnswerOf(t, raw).Outcome != "REQUEST_ALREADY_ANSWERED" {
		t.Fatalf("再回应同一请求 = %d %s, want 200 REQUEST_ALREADY_ANSWERED", status, raw)
	}
	if fixture.decisionRows(t) != 1 || fixture.envelopes(t) != 1 {
		t.Fatalf("重发与再回应之后留痕 %d 行、信封 %d 封, want 各 1", fixture.decisionRows(t), fixture.envelopes(t))
	}

	status, raw = fixture.post(pidDeciderB, pidBody("SYN-PARCEL-1", 1, "CONTINUE", "", ""))
	correction := pidAnswerOf(t, raw)
	if status != http.StatusCreated || correction.Outcome != "DECIDED" || correction.Decision == nil ||
		correction.Decision.Version != 2 || correction.Decision.Source != "OPERATOR_INITIATED" || len(correction.Decision.OwnerRequests) != 0 {
		t.Fatalf("运营发起的更正 = %d %s, want 201 第 2 版 OPERATOR_INITIATED 不请求任何所有者", status, raw)
	}
	if fixture.decisionRows(t) != 2 || fixture.envelopes(t) != 2 {
		t.Fatalf("更正之后留痕 %d 行、信封 %d 封, want 各 2", fixture.decisionRows(t), fixture.envelopes(t))
	}
	status, raw = fixture.post(pidDecider, pidBody("SYN-PARCEL-1", 1, "INTERCEPT", "", ""))
	if status != http.StatusOK || pidAnswerOf(t, raw).Outcome != "PREDECESSOR_MOVED" {
		t.Fatalf("链尾已动 = %d %s, want 200 PREDECESSOR_MOVED", status, raw)
	}
	if fixture.decisionRows(t) != 2 {
		t.Fatalf("PREDECESSOR_MOVED 之后留痕 %d 行, want 仍 2", fixture.decisionRows(t))
	}
}

// Covers: AT-PS-174；ADR-0332 决定一——多路同时回应同一份请求（去向还不一致），恰一路答 201，其余读回先到的那一份；留痕一行、信封一封。
func TestConcurrentDecidersOfOnePendingRequestLeaveOneDecision(t *testing.T) {
	fixture := newPostIntakeE2E(t)
	fixture.arrangeCrossed(t, "SYN-PARCEL-1", "cancel-key-1")

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
			token, direction := pidDecider, "INTERCEPT"
			if index%2 == 1 {
				token, direction = pidDeciderB, "TERMINATE_SERVICE"
			}
			statuses[index], bodies[index] = fixture.post(token, pidBody("SYN-PARCEL-1", 0, direction, "cancel-key-1", ""))
		}()
	}
	close(start)
	group.Wait()

	created := 0
	for index := range deciders {
		outcome := pidAnswerOf(t, bodies[index]).Outcome
		switch {
		case statuses[index] == http.StatusCreated && outcome == "DECIDED":
			created++
		case statuses[index] == http.StatusOK && (outcome == "ALREADY_DECIDED" || outcome == "REQUEST_ALREADY_ANSWERED"):
		default:
			t.Fatalf("第 %d 路 = %d %s", index, statuses[index], bodies[index])
		}
	}
	if created != 1 || fixture.decisionRows(t) != 1 || fixture.envelopes(t) != 1 {
		t.Fatalf("成立 %d 笔、留痕 %d 行、信封 %d 封, want 各恰 1", created, fixture.decisionRows(t), fixture.envelopes(t))
	}
}

// Covers: AT-PS-174, AT-PS-175；ADR-0332 决定二、ADR-0151 决定二——授权与可见性：只持别种授予 403，没有令牌 401，载荷自报租户 400，改送缺目的地与继续带目的地 400，
// 册上没有的请求答 404 统一不可见（别的租户对本租户的包裹没有责任起点，答 BOUNDARY_NOT_CROSSED，与不存在的包裹同答）；包裹没越过边界答 BOUNDARY_NOT_CROSSED（确定的拒绝，不是未决）；包裹已有有效取消答 FINAL_OUTCOME_STANDS；
// 查阅同理：没有查阅读授予 403、没有令牌 401、别的租户只看见自己的（空）清单；以上都什么也没写。
func TestTheWiredDecisionIsOpenOnlyToItsOwnGrantAndWritesNothingWhenRefused(t *testing.T) {
	fixture := newPostIntakeE2E(t)
	fixture.arrangeCrossed(t, "SYN-PARCEL-1", "cancel-key-1")
	good := pidBody("SYN-PARCEL-1", 0, "INTERCEPT", "cancel-key-1", "")
	for name, test := range map[string]struct {
		token  string
		body   string
		status int
		code   string
	}{
		"只持别种授予":   {pidOtherGrant, good, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"没有令牌":     {"", good, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"载荷带租户":    {pidDecider, strings.Replace(good, `"direction"`, `"tenantId":"SYN-TENANT-FOREIGN","direction"`, 1), http.StatusBadRequest, "MALFORMED_REQUEST"},
		"改送缺目的地":   {pidDecider, pidBody("SYN-PARCEL-1", 0, "REDIRECT", "cancel-key-1", ""), http.StatusBadRequest, "MALFORMED_REQUEST"},
		"继续带目的地":   {pidDecider, pidBody("SYN-PARCEL-1", 0, "CONTINUE", "cancel-key-1", "SYN-PLACE"), http.StatusBadRequest, "MALFORMED_REQUEST"},
		"去向不在封闭集":  {pidDecider, pidBody("SYN-PARCEL-1", 0, "DESTROY", "cancel-key-1", ""), http.StatusBadRequest, "MALFORMED_REQUEST"},
		"别的租户":     {pidForeigner, good, http.StatusOK, "BOUNDARY_NOT_CROSSED"},
		"册上没有这份请求": {pidDecider, strings.Replace(good, "cancel-key-1", "cancel-key-9", 1), http.StatusNotFound, "POST_INTAKE_DISPOSITION_REQUEST_NOT_VISIBLE"},
	} {
		status, raw := fixture.post(test.token, test.body)
		if status != test.status || !strings.Contains(raw, test.code) {
			t.Errorf("%s = %d %s, want %d %s", name, status, raw, test.status, test.code)
		}
	}
	if fixture.decisionRows(t) != 0 || fixture.envelopes(t) != 0 {
		t.Fatalf("被拒的请求写了东西：留痕 %d 行、信封 %d 封", fixture.decisionRows(t), fixture.envelopes(t))
	}

	// 有效取消在册的包裹：先摆一份收寄，再摆一份取消，决定答终局成立。
	fixture.arrangeCrossed(t, "SYN-PARCEL-2", "cancel-key-2")
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustValue(t, domain.NewParcelCancellationID, "PCXL-2"),
		Parcel:      mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-2"),
		Requester:   mustValue(t, domain.NewCancellationRequesterReference, "customer-a"),
		Authority:   mustValue(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/1"),
		Reason:      mustValue(t, domain.NewCancellationReasonReference, "CHANGED_MIND"),
		RequestedAt: time.Date(2026, 10, 4, 7, 0, 0, 0, time.UTC),
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("形成取消决定：%v", err)
	}
	cancellations, err := pspostgres.NewParcelCancellations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := cancellations.Save(txCtx, psports.CancellationRecord{
			Key: psports.CancellationRequestKey{
				TenantID:   mustValue(t, domain.NewTenantID, pidTenant),
				RequestKey: mustValue(t, domain.NewSourceRequestKey, "cancel-key-3"),
				Parcel:     mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-2"),
			},
			ContentDigest: "digest-cancel-3", Kind: psports.RecordParcelCancelled, Cancellation: cancellation,
			DecidedAt: time.Date(2026, 10, 4, 7, 1, 0, 0, time.UTC),
		})
		return err
	}); err != nil {
		t.Fatalf("摆取消：%v", err)
	}
	status, raw := fixture.post(pidDecider, pidBody("SYN-PARCEL-2", 0, "RETURN", "", ""))
	if status != http.StatusOK || pidAnswerOf(t, raw).Outcome != "FINAL_OUTCOME_STANDS" || !strings.Contains(raw, "PARCEL_CANCELLATION/PCXL-2") {
		t.Errorf("有效取消在册 = %d %s, want 200 FINAL_OUTCOME_STANDS 指名那份取消", status, raw)
	}
	status, raw = fixture.post(pidDecider, pidBody("SYN-PARCEL-NEVER", 0, "RETURN", "", ""))
	if status != http.StatusOK || pidAnswerOf(t, raw).Outcome != "BOUNDARY_NOT_CROSSED" {
		t.Errorf("没有责任起点 = %d %s, want 200 BOUNDARY_NOT_CROSSED", status, raw)
	}
	if fixture.decisionRows(t) != 0 || fixture.envelopes(t) != 0 {
		t.Fatalf("确定拒绝写了东西：留痕 %d 行、信封 %d 封", fixture.decisionRows(t), fixture.envelopes(t))
	}

	if status, _, _ := fixture.list(t, pidDeciderB); status != http.StatusForbidden {
		t.Errorf("没有查阅读授予 = %d, want 403", status)
	}
	if status, _, _ := fixture.list(t, ""); status != http.StatusUnauthorized {
		t.Errorf("没有令牌的查阅 = %d, want 401", status)
	}
	if status, pending, listed := fixture.list(t, pidForeigner); status != http.StatusOK || pending != 0 || listed != 0 {
		t.Errorf("别的租户的查阅 = %d 计数 %d 行 %d, want 200 与 0 与 0", status, pending, listed)
	}
}

// Covers: AT-PS-174；PBC-08 行为面负向证据——决定编排在无事务上下文拿不到包裹取消边界锁，整笔拒跑，决定与信封零行；事务壳之内才成立。
func TestThePostIntakeDecisionRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newPostIntakeE2E(t)
	fixture.arrangeCrossed(t, "SYN-PARCEL-1", "cancel-key-1")
	command := shipmentapp.DecidePostIntakeDispositionCommand{
		Tenant:     mustValue(t, domain.NewTenantID, pidTenant),
		Parcel:     mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-1"),
		Direction:  domain.PostIntakeIntercept,
		RequestKey: mustValue(t, domain.NewSourceRequestKey, "cancel-key-1"),
		Decider:    mustValue(t, domain.NewPostIntakeDispositionDeciderReference, "https://id.syn.example#alice"),
		Basis:      mustValue(t, domain.NewPostIntakeDispositionBasis, "SYN-BASIS"),
	}
	if _, err := fixture.inner.inner.Handle(t.Context(), command); !errors.Is(err, shipmentapp.ErrCancellationBoundaryNotLocked) {
		t.Fatalf("无事务决定应拒跑（取不到边界锁），实得：%v", err)
	}
	if fixture.decisionRows(t) != 0 || fixture.envelopes(t) != 0 {
		t.Fatalf("被拒的编排写了东西：留痕 %d 行、信封 %d 封", fixture.decisionRows(t), fixture.envelopes(t))
	}
	if _, err := fixture.inner.Handle(t.Context(), command); err != nil {
		t.Fatalf("事务壳之内应成立：%v", err)
	}
	if fixture.decisionRows(t) != 1 || fixture.envelopes(t) != 1 {
		t.Fatalf("事务壳之内：留痕 %d 行、信封 %d 封, want 各 1", fixture.decisionRows(t), fixture.envelopes(t))
	}
}

// Covers: AT-PS-174；ADR-0332 决定五、ADR-0295 决定三之一——被取消先后冲突解开决定认定依据不成立的取消，同样进等回应清单（来路 CANCELLATION_NOT_UPHELD），并能被一份决定回应；
// 回应之后它从清单消失，不再被回应第二次。
func TestACancellationNotUpheldWaitsForADispositionAndCanBeAnswered(t *testing.T) {
	fixture := newPostIntakeE2E(t)
	fixture.arrangeCrossed(t, "SYN-PARCEL-3", "cancel-key-pending")
	tenant := mustValue(t, domain.NewTenantID, pidTenant)
	parcel := mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-3")
	cancelledAt := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustValue(t, domain.NewParcelCancellationID, "PCXL-3"),
		Parcel:      parcel,
		Requester:   mustValue(t, domain.NewCancellationRequesterReference, "customer-a"),
		Authority:   mustValue(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/1"),
		Reason:      mustValue(t, domain.NewCancellationReasonReference, "CHANGED_MIND"),
		RequestedAt: cancelledAt,
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatal(err)
	}
	boundaryAt := cancelledAt.Add(-time.Hour)
	conflict, conflicted, err := domain.JudgeCancellationOrder(cancellation, domain.CancellationBoundaryFact{
		Kind:       domain.NetworkIntakeBoundary,
		Reference:  mustValue(t, domain.NewCancellationBoundaryReference, "NODE_INTAKE/v1"),
		OccurredAt: boundaryAt,
	})
	if err != nil || !conflicted {
		t.Fatalf("判冲突：%v %v", err, conflicted)
	}
	cancellations, err := pspostgres.NewParcelCancellations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	conflicts, err := pspostgres.NewCancellationOrderConflicts(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := cancellations.Save(txCtx, psports.CancellationRecord{
			Key: psports.CancellationRequestKey{
				TenantID: tenant, RequestKey: mustValue(t, domain.NewSourceRequestKey, "cancel-key-3"), Parcel: parcel,
			},
			ContentDigest: "digest-cancel-3", Kind: psports.RecordParcelCancelled, Cancellation: cancellation,
			DecidedAt: cancelledAt.Add(time.Minute),
		}); err != nil {
			return err
		}
		_, err := conflicts.Record(txCtx, psports.CancellationOrderConflictRecord{
			TenantID: tenant, Conflict: conflict, DetectedAt: cancelledAt.Add(time.Hour),
		})
		return err
	}); err != nil {
		t.Fatalf("摆夹具：%v", err)
	}
	conflictDecision, err := buildCancellationOrderConflictDecision(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := conflictDecision.Handle(t.Context(), shipmentapp.DecideCancellationOrderConflictCommand{
		Tenant:       tenant,
		Parcel:       parcel,
		BoundaryKind: domain.NetworkIntakeBoundary,
		BoundaryRef:  mustValue(t, domain.NewCancellationBoundaryReference, "NODE_INTAKE/v1"),
		Cancellation: mustValue(t, domain.NewParcelCancellationID, "PCXL-3"),
		Kind:         domain.CancellationNotUpheld,
		Decider:      mustValue(t, domain.NewCancellationOrderConflictDeciderReference, "https://id.syn.example#alice"),
		Basis:        mustValue(t, domain.NewCancellationOrderConflictDecisionBasis, "SYN-BASIS"),
	}); err != nil || result.Outcome() != shipmentapp.CancellationOrderConflictDecided {
		t.Fatalf("认定取消依据不成立：%v %v", result.Outcome(), err)
	}

	request := httptest.NewRequest(http.MethodGet, pidListURL, nil)
	request.Header.Set("Authorization", "Bearer "+pidDecider)
	response := httptest.NewRecorder()
	fixture.endpoints[pidListURL].ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"requestKey":"cancel-key-3"`) ||
		!strings.Contains(response.Body.String(), "CANCELLATION_NOT_UPHELD") || !strings.Contains(response.Body.String(), `"pendingCount":2`) {
		t.Fatalf("清单 = %d %s, want 含来路 CANCELLATION_NOT_UPHELD 的 cancel-key-3 与待处置那条，共 2", response.Code, response.Body.String())
	}
	status, raw := fixture.post(pidDecider, pidBody("SYN-PARCEL-3", 0, "RETURN", "cancel-key-3", ""))
	if status != http.StatusCreated || pidAnswerOf(t, raw).Outcome != "DECIDED" {
		t.Fatalf("回应被认定不成立的取消 = %d %s, want 201 DECIDED", status, raw)
	}
	if _, pending, _ := fixture.list(t, pidDecider); pending != 1 {
		t.Fatalf("回应之后清单计数 = %d, want 1（待处置那条仍在）", pending)
	}
}
