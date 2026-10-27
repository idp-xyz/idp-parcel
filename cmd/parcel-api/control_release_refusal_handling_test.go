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
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 证释放拒收处理口的生产装配（ADR-0293 决定二、三、四）：端点行按特性登记处那一份 mount 挂上，Intake 是生产那只操作者
// 铸造器经真操作者册（access_identity 0025 放开的「释放拒收处理」一种）铸的信封；编排是 buildControlReleaseRefusalHandling 那一套真库缝（委托仓储、
// 册、处理留痕、判断账、PS→SA 释放适配器与真两本账、续办信封交接与事务壳），只有令牌核验方与准入范围是替身。
//   - 在结算侧释放：存下作用域的那一笔提供方现在受理，答 201，冻结放掉，处理留痕与续办信封同笔落库；另一位再处理答已被处理、不多一封信封。
//   - 没存作用域的存量那一笔：释放再被拒收，答 200 且什么也不写；标记已处理才成立，查阅里随之消失。
//   - 并发：多位同时处理同一笔，恰一笔成立、恰一封信封。
//   - 授权：只持授权处置授予、别的租户、没有令牌都进不来，且什么也没写。
//   - 事务外拒跑（PBC-08）：编排的写在无事务上下文被拒，留痕与信封零行。
// 夹具全为合成（S 级）。

const (
	handlingTenant        = "SYN-TENANT-1"
	handlingOperatorA     = "SYN-HANDLER-CODE95-A"
	handlingOperatorB     = "SYN-HANDLER-CODE95-B"
	handlingDispositioner = "SYN-DISPOSITIONER-CODE95"
	handlingForeigner     = "SYN-FOREIGNER-CODE95"
	handlingForeignTenant = "SYN-TENANT-FOREIGN"
	handlingEventType     = "parcel-shipment.shipment-request.control-release-refusal-handled"
	handlingPath          = "/control-release-refusal-handlings"
	refusalsPath          = "/control-release-refusals"
)

type refusalHandlingE2E struct {
	db        *bentopg.DB
	pool      *pgxpool.Pool
	endpoints map[string]http.Handler
	version   domain.SubmissionVersionID
	inner     transactionalControlReleaseRefusalHandling
}

func newRefusalHandlingE2E(t *testing.T) *refusalHandlingE2E {
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
	version := submittedRequestOnRealAssembly(t, db)

	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	registerOperatorIn(t, db, operators, handlingTenant, handlingOperatorA, interval, accessidentity.DecisionControlReleaseRefusalHandling)
	registerOperatorIn(t, db, operators, handlingTenant, handlingOperatorB, interval, accessidentity.DecisionControlReleaseRefusalHandling)
	registerOperatorIn(t, db, operators, handlingTenant, handlingDispositioner, interval, accessidentity.DecisionAuthorizedDisposition)
	registerOperatorIn(t, db, operators, handlingForeignTenant, handlingForeigner, interval, accessidentity.DecisionControlReleaseRefusalHandling)
	registerOperationsReader(t, db, operators, handlingTenant, handlingOperatorA)
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
	handling, err := buildControlReleaseRefusalHandling(db)
	if err != nil {
		t.Fatalf("装配处理编排：%v", err)
	}
	catalogue, err := pspostgres.NewControlReleaseRefusalCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &refusalHandlingE2E{db: db, pool: pool, endpoints: map[string]http.Handler{}, version: version}
	fixture.inner = handling.(transactionalControlReleaseRefusalHandling)
	channels := endpointChannels{operatorDecisions: decisions, operatorRegistries: registries}
	for _, feature := range endpointRegistry {
		switch feature.wiringKey {
		case any((*controlReleaseRefusalHandlingWiring)(nil)):
			for _, endpoint := range feature.mountWith(channels, controlReleaseRefusalHandlingWiring{handling: handling}) {
				fixture.endpoints[endpoint.Pattern] = endpoint.Handler
			}
		case any((*controlReleaseRefusalsWiring)(nil)):
			for _, endpoint := range feature.mountWith(channels, controlReleaseRefusalsWiring{refusals: catalogue}) {
				fixture.endpoints[endpoint.Pattern] = endpoint.Handler
			}
		}
	}
	if fixture.endpoints[handlingPath] == nil || fixture.endpoints[refusalsPath] == nil {
		t.Fatal("处理口与查阅口没按登记处挂上")
	}
	return fixture
}

// registerRefusal 在册上摆被拒收的这一笔（版本为夹具的当前版本、控制关联为 recordControlOnRealAssembly 的约定）。
func (fixture *refusalHandlingE2E) registerRefusal(t *testing.T, scope domain.OccupationScope) {
	t.Helper()
	command := submissionCommand(t)
	register, err := pspostgres.NewControlReleaseRefusals(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	refusal := psports.ControlReleaseRefusal{
		TenantID:          command.Identity.TenantID(),
		CustomerAccountID: command.Identity.CustomerAccountID(),
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: fixture.version,
		ControlResultID:   mustValue(t, domain.NewFinancialControlResultID, command.ShipmentRequestID.String()+"/"+fixture.version.String()),
		Scope:             scope,
		Reason:            mustValue(t, domain.NewCheckReason, "SA-RELEASE_REQUEST_NOT_ACCEPTED"),
		RecordedAt:        refusalControlAt,
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := register.Record(txCtx, refusal)
		return err
	}); err != nil {
		t.Fatalf("记册：%v", err)
	}
}

func (fixture *refusalHandlingE2E) body(t *testing.T, action string) string {
	t.Helper()
	command := submissionCommand(t)
	return `{"shipmentRequestId":"` + command.ShipmentRequestID.String() + `","submissionVersionId":"` + fixture.version.String() +
		`","controlResultId":"` + command.ShipmentRequestID.String() + "/" + fixture.version.String() +
		`","action":"` + action + `","basis":"SYN-CHECKED-AGAINST-LEDGER"}`
}

func (fixture *refusalHandlingE2E) post(token, body string) (int, string) {
	request := httptest.NewRequest(http.MethodPost, handlingPath, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[handlingPath].ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

func (fixture *refusalHandlingE2E) list(t *testing.T) (int, int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, refusalsPath, nil)
	request.Header.Set("Authorization", "Bearer "+handlingOperatorA)
	response := httptest.NewRecorder()
	fixture.endpoints[refusalsPath].ServeHTTP(response, request)
	var body struct {
		UnhandledCount int   `json:"unhandledCount"`
		Refusals       []any `json:"refusals"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusOK {
		t.Fatalf("查阅 = %d %s", response.Code, response.Body.String())
	}
	return body.UnhandledCount, len(body.Refusals)
}

func (fixture *refusalHandlingE2E) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := fixture.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("计数 %q：%v", query, err)
	}
	return n
}

func (fixture *refusalHandlingE2E) handlingRows(t *testing.T) int {
	return fixture.count(t, `SELECT count(*) FROM parcel_shipment.control_release_refusal_handling`)
}

func (fixture *refusalHandlingE2E) resumeEnvelopes(t *testing.T) int {
	return fixture.count(t, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`, handlingEventType)
}

func handlingOutcomeOf(t *testing.T, raw string) string {
	t.Helper()
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("答复不是 JSON：%s", raw)
	}
	return body.Outcome
}

// Covers: ADR-0293 决定二、三、四——核对后在结算侧释放：存下作用域的那一笔提供方现在受理，答 201 HANDLED，冻结放掉，处理留痕（做法、处理人取认证出的
// 操作者、依据）与续办信封同笔落库，信封带委托的当前版本；查阅里随之消失；另一位操作者再处理答 200 ALREADY_HANDLED、不多一封信封，回执不带操作者。
func TestTheWiredHandlingReleasesTheRefusedOccupationAndHandsOffOneResume(t *testing.T) {
	fixture := newRefusalHandlingE2E(t)
	recorded, settlement := refusalSettlementScope(t)
	resultID := recordControlOnRealAssembly(t, fixture.db, fixture.version, recorded)
	freezeOnRealAssembly(t, fixture.db, resultID, settlement)
	fixture.registerRefusal(t, recorded)
	if unhandled, listed := fixture.list(t); unhandled != 1 || listed != 1 {
		t.Fatalf("处理前查阅 = 计数 %d 行 %d, want 1 与 1", unhandled, listed)
	}

	status, raw := fixture.post(handlingOperatorA, fixture.body(t, "RELEASE_AT_SETTLEMENT"))
	if status != http.StatusCreated || handlingOutcomeOf(t, raw) != "HANDLED" || !strings.Contains(raw, `"handlingKind":"RELEASED_AT_SETTLEMENT"`) {
		t.Fatalf("处理 = %d %s, want 201 HANDLED RELEASED_AT_SETTLEMENT", status, raw)
	}
	if got := freezeStatusOnRealAssembly(t, fixture.db, resultID); got != int16(sadomain.FreezeReleased) {
		t.Fatalf("冻结 status = %d, want 已释放", got)
	}
	var kind, handler, basis string
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT handling_kind, handled_by, handling_basis FROM parcel_shipment.control_release_refusal_handling`).Scan(&kind, &handler, &basis); err != nil {
		t.Fatalf("读处理留痕：%v", err)
	}
	if kind != "RELEASED_AT_SETTLEMENT" || handler != customsDecisionIssuer+"#"+handlingOperatorA || basis != "SYN-CHECKED-AGAINST-LEDGER" {
		t.Fatalf("留痕 = %q %q %q, want 做法、认证出的操作者「发行方#sub」、依据", kind, handler, basis)
	}
	if fixture.resumeEnvelopes(t) != 1 {
		t.Fatalf("续办信封 %d 封, want 恰好 1", fixture.resumeEnvelopes(t))
	}
	var payload struct {
		SubmissionVersionID        string `json:"submissionVersionId"`
		HandledSubmissionVersionID string `json:"handledSubmissionVersionId"`
	}
	var raw2 []byte
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`, handlingEventType).Scan(&raw2); err != nil {
		t.Fatalf("读信封：%v", err)
	}
	if err := json.Unmarshal(raw2, &payload); err != nil || payload.SubmissionVersionID != fixture.version.String() ||
		payload.HandledSubmissionVersionID != fixture.version.String() {
		t.Fatalf("信封载荷 = %s, want 委托的当前版本", raw2)
	}
	if unhandled, listed := fixture.list(t); unhandled != 0 || listed != 0 {
		t.Fatalf("处理后查阅 = 计数 %d 行 %d, want 0", unhandled, listed)
	}

	status, raw = fixture.post(handlingOperatorB, fixture.body(t, "MARK_HANDLED"))
	if status != http.StatusOK || handlingOutcomeOf(t, raw) != "ALREADY_HANDLED" || !strings.Contains(raw, `"handlingKind":"RELEASED_AT_SETTLEMENT"`) {
		t.Fatalf("另一位再处理 = %d %s, want 200 ALREADY_HANDLED 且读回先到的做法", status, raw)
	}
	if strings.Contains(raw, "SYN-HANDLER") || strings.Contains(raw, "id.syn.example") {
		t.Fatalf("回执带了操作者：%s", raw)
	}
	if fixture.handlingRows(t) != 1 || fixture.resumeEnvelopes(t) != 1 {
		t.Fatalf("再处理之后留痕 %d 行、信封 %d 封, want 仍各 1", fixture.handlingRows(t), fixture.resumeEnvelopes(t))
	}
}

// Covers: ADR-0293 决定二——没存作用域的存量那一笔（走生产授权处置装配摆出：处置照样成立，SA 拒收，记进册）：选「在结算侧释放」提供方再次拒收，答 200
// RELEASE_REFUSED_AGAIN 带原因，冻结原样占着、什么也不写（不留痕、不交信封）；改选「标记已处理」才成立，查阅里消失，信封恰一封。
func TestTheWiredHandlingKeepsAnUnscopedRefusalUntilTheOperatorMarksIt(t *testing.T) {
	fixture := newRefusalHandlingE2E(t)
	_, settlement := refusalSettlementScope(t)
	// 摆出被拒收的这一笔：控制结果没存作用域（存量行），册上有它被 SA 拒收的记录。
	resultID := recordControlOnRealAssembly(t, fixture.db, fixture.version, domain.OccupationScope{})
	freezeOnRealAssembly(t, fixture.db, resultID, settlement)
	fixture.registerRefusal(t, domain.OccupationScope{})

	status, raw := fixture.post(handlingOperatorA, fixture.body(t, "RELEASE_AT_SETTLEMENT"))
	if status != http.StatusOK || handlingOutcomeOf(t, raw) != "RELEASE_REFUSED_AGAIN" || !strings.Contains(raw, `"reason":"SA-RELEASE_REQUEST_NOT_ACCEPTED"`) {
		t.Fatalf("释放 = %d %s, want 200 RELEASE_REFUSED_AGAIN 带提供方的原因", status, raw)
	}
	if got := freezeStatusOnRealAssembly(t, fixture.db, resultID); got != int16(sadomain.FreezeHeld) {
		t.Fatalf("冻结 status = %d, want 原样占着", got)
	}
	if fixture.handlingRows(t) != 0 || fixture.resumeEnvelopes(t) != 0 {
		t.Fatalf("释放被再拒收却留痕 %d 行、信封 %d 封, want 0", fixture.handlingRows(t), fixture.resumeEnvelopes(t))
	}
	if unhandled, _ := fixture.list(t); unhandled != 1 {
		t.Fatalf("查阅计数 = %d, want 仍为 1", unhandled)
	}

	status, raw = fixture.post(handlingOperatorA, fixture.body(t, "MARK_HANDLED"))
	if status != http.StatusCreated || handlingOutcomeOf(t, raw) != "HANDLED" || !strings.Contains(raw, `"handlingKind":"MARKED_HANDLED"`) {
		t.Fatalf("标记 = %d %s, want 201 HANDLED MARKED_HANDLED", status, raw)
	}
	if got := freezeStatusOnRealAssembly(t, fixture.db, resultID); got != int16(sadomain.FreezeHeld) {
		t.Fatalf("标记已处理动了账本：冻结 status = %d, want 仍占着（标记只留痕）", got)
	}
	if unhandled, _ := fixture.list(t); unhandled != 0 {
		t.Fatalf("标记之后查阅计数 = %d, want 0", unhandled)
	}
	if fixture.handlingRows(t) != 1 || fixture.resumeEnvelopes(t) != 1 {
		t.Fatalf("标记之后留痕 %d 行、信封 %d 封, want 各 1", fixture.handlingRows(t), fixture.resumeEnvelopes(t))
	}
}

// Covers: ADR-0293 决定二「并发：两人同时处理同一行，只一笔成立」——多路同时标记同一笔，恰一路答 201，其余答 200 ALREADY_HANDLED；留痕一行、续办信封一封。
func TestConcurrentHandlersOfOneWiredRefusalLeaveOneHandlingAndOneEnvelope(t *testing.T) {
	fixture := newRefusalHandlingE2E(t)
	recorded, _ := refusalSettlementScope(t)
	fixture.registerRefusal(t, recorded)

	const handlers = 6
	statuses := make([]int, handlers)
	outcomes := make([]string, handlers)
	var group sync.WaitGroup
	start := make(chan struct{})
	for index := range handlers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			token := handlingOperatorA
			if index%2 == 1 {
				token = handlingOperatorB
			}
			code, raw := fixture.post(token, fixture.body(t, "MARK_HANDLED"))
			statuses[index] = code
			var body struct {
				Outcome string `json:"outcome"`
			}
			_ = json.Unmarshal([]byte(raw), &body)
			outcomes[index] = body.Outcome
		}()
	}
	close(start)
	group.Wait()

	created := 0
	for index := range handlers {
		switch {
		case statuses[index] == http.StatusCreated && outcomes[index] == "HANDLED":
			created++
		case statuses[index] == http.StatusOK && outcomes[index] == "ALREADY_HANDLED":
		default:
			t.Fatalf("第 %d 路 = %d %q, want 201 HANDLED 或 200 ALREADY_HANDLED", index, statuses[index], outcomes[index])
		}
	}
	if created != 1 {
		t.Fatalf("成立了 %d 笔, want 恰好 1", created)
	}
	if fixture.handlingRows(t) != 1 || fixture.resumeEnvelopes(t) != 1 {
		t.Fatalf("留痕 %d 行、信封 %d 封, want 各 1", fixture.handlingRows(t), fixture.resumeEnvelopes(t))
	}
}

// Covers: ADR-0293 决定三、ADR-0151 决定二——授权：只持授权处置授予的操作者进不来（两种授予不通用），别的租户的操作者拿着本租户的委托标识同样
// 403（越权探针同答），没有令牌 401；册上没有的那一笔 404；以上都什么也没写。
func TestTheWiredHandlingIsOpenOnlyToItsOwnGrantWithinTheOwnTenant(t *testing.T) {
	fixture := newRefusalHandlingE2E(t)
	recorded, _ := refusalSettlementScope(t)
	fixture.registerRefusal(t, recorded)

	for name, test := range map[string]struct {
		token  string
		body   string
		status int
		code   string
	}{
		"只持授权处置授予": {handlingDispositioner, fixture.body(t, "MARK_HANDLED"), http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"别的租户":     {handlingForeigner, fixture.body(t, "MARK_HANDLED"), http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"没有令牌":     {"", fixture.body(t, "MARK_HANDLED"), http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"载荷带租户":    {handlingOperatorA, strings.Replace(fixture.body(t, "MARK_HANDLED"), `"action"`, `"tenantId":"SYN-TENANT-FOREIGN","action"`, 1), http.StatusBadRequest, "MALFORMED_REQUEST"},
		"册上没有这一版":  {handlingOperatorA, strings.Replace(fixture.body(t, "MARK_HANDLED"), fixture.version.String(), "SYN-VER-NOT-ON-REGISTER", 2), http.StatusNotFound, "SHIPMENT_REQUEST_NOT_VISIBLE"},
	} {
		status, raw := fixture.post(test.token, test.body)
		if status != test.status || !strings.Contains(raw, test.code) {
			t.Errorf("%s = %d %s, want %d %s", name, status, raw, test.status, test.code)
		}
	}
	if fixture.handlingRows(t) != 0 || fixture.resumeEnvelopes(t) != 0 {
		t.Fatalf("被拒的请求写了留痕 %d 行、信封 %d 封", fixture.handlingRows(t), fixture.resumeEnvelopes(t))
	}
}

// Covers: PBC-08 行为面负向证据——处理编排的写在无事务上下文被拒（ErrTransactionRequired），留痕与信封零行；事务壳之内才成立。
func TestTheHandlingOrchestrationRefusesToWriteOutsideATransaction(t *testing.T) {
	fixture := newRefusalHandlingE2E(t)
	recorded, _ := refusalSettlementScope(t)
	fixture.registerRefusal(t, recorded)
	command := submissionCommand(t)
	_, err := fixture.inner.inner.Handle(t.Context(), shipmentapp.HandleControlReleaseRefusalCommand{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: fixture.version,
		ControlResultID:   mustValue(t, domain.NewFinancialControlResultID, command.ShipmentRequestID.String()+"/"+fixture.version.String()),
		Action:            shipmentapp.MarkControlReleaseRefusalHandled,
		Handler:           mustValue(t, domain.NewControlReleaseRefusalHandlerReference, "https://id.syn.example#alice"),
		Basis:             mustValue(t, domain.NewControlReleaseRefusalHandlingBasis, "SYN-BASIS"),
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务处理 err = %v, want ErrTransactionRequired", err)
	}
	if fixture.handlingRows(t) != 0 || fixture.resumeEnvelopes(t) != 0 {
		t.Fatalf("被拒的处理写了留痕 %d 行、信封 %d 封", fixture.handlingRows(t), fixture.resumeEnvelopes(t))
	}
}
