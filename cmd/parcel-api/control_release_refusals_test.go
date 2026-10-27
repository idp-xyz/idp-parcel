package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证释放未受理册查阅口的生产装配（ADR-0293 决定一）：端点行按特性登记处那一份 mount 挂上，Intake 是生产那只
// 操作者铸造器经真操作者册铸的信封，读口是真库适配器。租户只取自信封（请求里自报的租户不采信）；只持运营决定授予而没有查阅读授予的
// 操作者进不来；没有令牌答 401。

const (
	refusalsTenantA = "SYN-TENANT-01"
	refusalsTenantB = "SYN-TENANT-02"
	refusalsReaderA = "SYN-READER-CODE95-A"
	refusalsReaderB = "SYN-READER-CODE95-B"
	refusalsNoRead  = "SYN-DECIDER-CODE95"
)

type refusalsFixture struct {
	db        *bentopg.DB
	endpoints map[string]http.Handler
}

func registerOperationsReader(t *testing.T, db *bentopg.DB, operators *aipg.OperatorRegistry, tenant, subject string) {
	t.Helper()
	operator, err := accessidentity.NewOperatorSubject(customsDecisionIssuer, subject)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewOperatorBinding(operator, tenant, "SYN-BIND-BASIS-CODE95")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewOperatorGrant(tenant, "SYN-GRANT-READ-"+subject, operator,
		accessidentity.CapabilityMasterDataAndOperationsRead, interval, "SYN-GRANT-BASIS-CODE95")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := operators.RegisterOperator(txCtx, binding); err != nil {
			return err
		}
		_, err := operators.RegisterGrant(txCtx, grant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func newRefusalsFixture(t *testing.T) *refusalsFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
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
	registerOperationsReader(t, db, operators, refusalsTenantA, refusalsReaderA)
	registerOperationsReader(t, db, operators, refusalsTenantB, refusalsReaderB)
	registerOperatorIn(t, db, operators, refusalsTenantA, refusalsNoRead, interval, accessidentity.DecisionControlReleaseRefusalHandling)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := pspostgres.NewControlReleaseRefusalCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &refusalsFixture{db: db, endpoints: map[string]http.Handler{}}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*controlReleaseRefusalsWiring)(nil)) {
			continue
		}
		for _, endpoint := range feature.mountWith(endpointChannels{operatorRegistries: registries},
			controlReleaseRefusalsWiring{refusals: catalogue}) {
			fixture.endpoints[endpoint.Pattern] = endpoint.Handler
		}
	}
	if fixture.endpoints["/control-release-refusals"] == nil {
		t.Fatal("/control-release-refusals 没按登记处挂上")
	}
	return fixture
}

func (fixture *refusalsFixture) record(t *testing.T, tenant, request, version string, scoped bool) psports.ControlReleaseRefusal {
	t.Helper()
	register, err := pspostgres.NewControlReleaseRefusals(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	refusal := psports.ControlReleaseRefusal{
		TenantID:          mustPSValue(t, psdomain.NewTenantID, tenant),
		CustomerAccountID: mustPSValue(t, psdomain.NewCustomerAccountID, "SYN-CUST-CODE95"),
		ShipmentRequestID: mustPSValue(t, psdomain.NewShipmentRequestID, request),
		SubmissionVersion: mustPSValue(t, psdomain.NewSubmissionVersionID, version),
		ControlResultID:   mustPSValue(t, psdomain.NewFinancialControlResultID, request+"/"+version),
		Reason:            mustPSValue(t, psdomain.NewCheckReason, "SA-RELEASE_REQUEST_NOT_ACCEPTED"),
		RecordedAt:        time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC),
	}
	if scoped {
		scope, err := psdomain.NewOccupationScope("SYN-LEGAL-ENTITY-1", "SYN-ACCOUNT-1", "CNY")
		if err != nil {
			t.Fatal(err)
		}
		refusal.Scope = scope
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := register.Record(txCtx, refusal)
		return err
	}); err != nil {
		t.Fatalf("记册：%v", err)
	}
	return refusal
}

func mustPSValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("%q: %v", raw, err)
	}
	return built
}

func (fixture *refusalsFixture) get(t *testing.T, target, token string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints["/control-release-refusals"].ServeHTTP(response, request)
	var answer map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复不是 JSON：%s", response.Body.String())
	}
	return response.Code, answer
}

// Covers: ADR-0293 决定一——查阅按租户隔离：租户 A 的操作者只看到 A 的未处理行与计数，B 的行看不见；请求里自报的租户参数不采信；
// 没有查阅读授予（只持运营决定授予）答 403 OPERATOR_NOT_GRANTED；没有令牌答 401。
func TestTheWiredRefusalQueryIsScopedToTheOperatorsTenant(t *testing.T) {
	fixture := newRefusalsFixture(t)
	fixture.record(t, refusalsTenantA, "SYN-REQ-A1", "SYN-VER-01", false)
	fixture.record(t, refusalsTenantA, "SYN-REQ-A2", "SYN-VER-02", true)
	fixture.record(t, refusalsTenantB, "SYN-REQ-B1", "SYN-VER-01", true)

	status, answer := fixture.get(t, "/control-release-refusals", refusalsReaderA)
	rows, _ := answer["refusals"].([]any)
	if status != http.StatusOK || answer["outcome"] != "CONTROL_RELEASE_REFUSALS_LISTED" || answer["unhandledCount"] != float64(2) || len(rows) != 2 {
		t.Fatalf("租户 A = %d %v, want 200 与两行（计数 2）", status, answer)
	}
	for _, raw := range rows {
		if request := raw.(map[string]any)["shipmentRequestId"]; request == "SYN-REQ-B1" {
			t.Fatalf("租户 A 看到了租户 B 的行：%v", answer)
		}
	}

	status, answer = fixture.get(t, "/control-release-refusals?tenant="+refusalsTenantA+"&limit=1", refusalsReaderB)
	rows, _ = answer["refusals"].([]any)
	if status != http.StatusOK || len(rows) != 1 || rows[0].(map[string]any)["shipmentRequestId"] != "SYN-REQ-B1" {
		t.Fatalf("租户 B 自报了租户 A = %d %v, want 只有 B 自己那一行", status, answer)
	}

	if status, answer = fixture.get(t, "/control-release-refusals", refusalsNoRead); status != http.StatusForbidden || problemCodeOf(answer) != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("只持运营决定授予 = %d %v, want 403 OPERATOR_NOT_GRANTED", status, answer)
	}
	if status, answer = fixture.get(t, "/control-release-refusals", ""); status != http.StatusUnauthorized {
		t.Fatalf("没有令牌 = %d %v, want 401", status, answer)
	}
}

// problemCodeOf 取两种问题体里的 code。
func problemCodeOf(answer map[string]any) string {
	if code, ok := answer["code"].(string); ok {
		return code
	}
	if inner, ok := answer["error"].(map[string]any); ok {
		code, _ := inner["code"].(string)
		return code
	}
	return ""
}
