package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对启用范围登记的两个端点证传输面：登记口的租户与登记责任方只取认证信封、自报的一概拒、令牌不过时不读载荷、
// 答案逐名转写（目录外的码答 200 未受理并指名理由）；读回口的租户取自信封、他租户不可见、种类必给、状态由读的这一刻导出。

// enablementRegistryDouble 是启用范围登记册的内存替身，按租户隔离。
type enablementRegistryDouble struct {
	rows []domain.ReferenceEnablementRegistration
}

func (double *enablementRegistryDouble) SaveReferenceEnablement(
	_ context.Context,
	registration domain.ReferenceEnablementRegistration,
) (ports.ReferenceEnablementSaveOutcome, error) {
	for _, existing := range double.rows {
		if existing.Tenant() == registration.Tenant() && existing.Kind() == registration.Kind() &&
			existing.Code() == registration.Code() && existing.Revision() == registration.Revision() {
			if existing.SameContent(registration) {
				return ports.ReferenceEnablementAlreadyRegistered, nil
			}
			return ports.ReferenceEnablementContentConflict, nil
		}
	}
	double.rows = append(double.rows, registration)
	return ports.ReferenceEnablementSaved, nil
}

func (double *enablementRegistryDouble) LoadLatestReferenceEnablement(
	_ context.Context,
	tenant domain.TenantID,
	kind domain.ReferenceKind,
	code domain.ReferenceCode,
) (domain.ReferenceEnablementRegistration, bool, error) {
	var latest domain.ReferenceEnablementRegistration
	found := false
	for _, row := range double.rows {
		if row.Tenant() == tenant && row.Kind() == kind && row.Code() == code && (!found || row.Revision() > latest.Revision()) {
			latest, found = row, true
		}
	}
	return latest, found, nil
}

func (double *enablementRegistryDouble) ListReferenceEnablements(
	_ context.Context,
	tenant domain.TenantID,
	kind domain.ReferenceKind,
) ([]domain.ReferenceEnablementRegistration, error) {
	var listed []domain.ReferenceEnablementRegistration
	for _, row := range double.rows {
		if row.Tenant() == tenant && row.Kind() == kind {
			listed = append(listed, row)
		}
	}
	sort.Slice(listed, func(i, j int) bool {
		if listed[i].Code().String() != listed[j].Code().String() {
			return listed[i].Code().String() < listed[j].Code().String()
		}
		return listed[i].Revision() < listed[j].Revision()
	})
	return listed, nil
}

type enablementClock struct{ now time.Time }

func (clock enablementClock) Now() time.Time { return clock.now }

var enablementReadAt = time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

// enablementOperators 是按令牌认证的替身：每个令牌对应一个租户与一个操作者主体。
type enablementOperators map[string]commercialhttp.OperatorIdentity

func (operators enablementOperators) AuthenticateRegistryWrite(_ context.Context, token string) (commercialhttp.OperatorIdentity, error) {
	identity, ok := operators[token]
	if !ok {
		return commercialhttp.OperatorIdentity{}, commercialhttp.ErrOperatorCredentialRejected
	}
	return identity, nil
}

func enablementOperator(t *testing.T, tenant, issuer, subject string) commercialhttp.OperatorIdentity {
	t.Helper()
	reference := catValue(t, domain.NewOperatorSubjectReference, issuer+"#"+subject)
	operator, err := domain.NewOperatorSubject(reference, nil)
	if err != nil {
		t.Fatal(err)
	}
	return commercialhttp.OperatorIdentity{Tenant: catValue(t, domain.NewTenantID, tenant), Operator: operator}
}

func enablementPost(t *testing.T, endpoint http.Handler, token, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/reference-data/enablement-registrations", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复不是 JSON：%v（%s）", err, recorder.Body)
	}
	return recorder.Code, answer
}

type enablementCatalogueIntake struct {
	tenants map[string]string
}

func (intake enablementCatalogueIntake) IntakeCatalogueQuery(_ context.Context, request *http.Request) (commercialhttp.CatalogueQuery, error) {
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	tenant, ok := intake.tenants[token]
	if !ok {
		return commercialhttp.CatalogueQuery{}, commercialhttp.ErrOperatorNotGranted
	}
	scope, err := domain.NewOperationsQueryScope(
		mustValue(domain.NewOperationsScopeReference, "MASTER_DATA_AND_OPERATIONS_READ"), mustValue(domain.NewTenantID, tenant))
	return commercialhttp.CatalogueQuery{Scope: scope, Limit: 200}, err
}

func mustValue[T any](construct func(string) (T, error), raw string) T {
	value, err := construct(raw)
	if err != nil {
		panic(err)
	}
	return value
}

func enablementEndpoints(t *testing.T, registry *enablementRegistryDouble) (write, read http.Handler) {
	t.Helper()
	intake, err := commercialhttp.NewOperatorRegistryIntake(enablementOperators{
		"t1": enablementOperator(t, "SYN-TENANT-01", "https://idp.example", "operator-a"),
		"t2": enablementOperator(t, "SYN-TENANT-02", "https://idp.example", "operator-b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := enablementClock{enablementReadAt}
	write = commercialhttp.NewRegisterReferenceEnablementEndpoint(intake, application.NewRegisterReferenceEnablementHandler(registry, clock))
	read = commercialhttp.NewQueryReferenceEnablementsEndpoint(
		enablementCatalogueIntake{tenants: map[string]string{"t1": "SYN-TENANT-01", "t2": "SYN-TENANT-02"}},
		application.NewQueryReferenceEnablementsHandler(registry, clock))
	return write, read
}

func enablementRead(t *testing.T, read http.Handler, token, query string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/reference-data/enablement"+query, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	read.ServeHTTP(recorder, request)
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("答复不是 JSON：%v（%s）", err, recorder.Body)
	}
	return recorder.Code, body
}

const enableChina = `{"kind":"COUNTRY","code":"CN","revision":1,"status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z","source":"MEMO-1"}`

// Covers: ADR-0345 决定四——登记口的租户与登记责任方都只取认证信封，落库的登记责任方是“发行方#sub”；
// 他租户读不到；未授予（凭证被拒）一律先于载荷。
func TestEnablementRegistrationTakesTenantAndRegistrantFromTheEnvelope(t *testing.T) {
	registry := &enablementRegistryDouble{}
	write, read := enablementEndpoints(t, registry)

	if status, answer := enablementPost(t, write, "t1", enableChina); status != http.StatusCreated || answer["outcome"] != "REGISTERED" {
		t.Fatalf("登记 = %d %v, want 201 REGISTERED", status, answer)
	}
	if len(registry.rows) != 1 {
		t.Fatalf("应落一行，实际 %d", len(registry.rows))
	}
	row := registry.rows[0]
	if row.Tenant().String() != "SYN-TENANT-01" || row.Registrant().String() != "https://idp.example#operator-a" {
		t.Fatalf("租户与登记责任方应取自信封：%s %s", row.Tenant(), row.Registrant())
	}

	if status, body := enablementRead(t, read, "t2", "?kind=COUNTRY"); status != http.StatusOK || len(body["enablements"].([]any)) != 0 {
		t.Fatalf("他租户读不到别人登的行：%d %v", status, body)
	}
	_, body := enablementRead(t, read, "t1", "?kind=COUNTRY")
	entry := body["enablements"].([]any)[0].(map[string]any)
	history := entry["history"].([]any)[0].(map[string]any)
	if history["registeredBy"] != "https://idp.example#operator-a" || entry["standing"] != "ENABLED" || entry["latestRevision"] != float64(1) {
		t.Fatalf("读回应带登记责任方、此刻状态与最新修订：%v", entry)
	}
}

// Covers: ADR-0345 决定四——载荷里自报租户（null 也算）、自报登记责任方（未知键）一概拒；缺修订、缺生效时点、集外种类与
// 状态都是请求畸形；令牌缺失在读载荷之前就答凭证被拒。
func TestEnablementRegistrationRefusesWhatTheEnvelopeAlreadyAnswers(t *testing.T) {
	registry := &enablementRegistryDouble{}
	write, _ := enablementEndpoints(t, registry)
	for name, body := range map[string]string{
		"自报租户":       `{"tenantId":"SYN-TENANT-02","kind":"COUNTRY","code":"CN","revision":1,"status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z"}`,
		"null 租户":    `{"tenantId":null,"kind":"COUNTRY","code":"CN","revision":1,"status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z"}`,
		"自报登记责任方":    `{"registeredBy":"someone","kind":"COUNTRY","code":"CN","revision":1,"status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z"}`,
		"缺修订":        `{"kind":"COUNTRY","code":"CN","status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z"}`,
		"缺生效时点":      `{"kind":"COUNTRY","code":"CN","revision":1,"status":"ENABLED"}`,
		"集外种类":       `{"kind":"REGION","code":"CN","revision":1,"status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z"}`,
		"集外状态":       `{"kind":"COUNTRY","code":"CN","revision":1,"status":"PENDING","effectiveFrom":"2026-06-01T00:00:00Z"}`,
		"空码":         `{"kind":"COUNTRY","code":" ","revision":1,"status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z"}`,
		"来源单据超长":     `{"kind":"COUNTRY","code":"CN","revision":1,"status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z","source":"` + strings.Repeat("单", 201) + `"}`,
		"尾随第二个 JSON": enableChina + `{}`,
	} {
		status, answer := enablementPost(t, write, "t1", body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s：status = %d %v, want 400", name, status, answer)
		}
	}
	if len(registry.rows) != 0 {
		t.Fatalf("畸形请求一个字节没写：%d 行", len(registry.rows))
	}

	probe := &bodyReadProbe{}
	recorder := httptest.NewRecorder()
	write.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/reference-data/enablement-registrations", probe))
	if recorder.Code != http.StatusUnauthorized || probe.read {
		t.Fatalf("无令牌：status = %d，读了载荷 = %v，want 401 且先于载荷", recorder.Code, probe.read)
	}
}

// Covers: ADR-0345 决定三——目录外的码答 200 未受理并指名 CODE_NOT_IN_CATALOGUE（业务答复，不是 5xx 也不是 400）；
// 重放答 200 已登记，异内容答 200 内容冲突；修订错位指名理由。
func TestEnablementRegistrationAnswersBusinessGrades(t *testing.T) {
	registry := &enablementRegistryDouble{}
	write, _ := enablementEndpoints(t, registry)

	status, answer := enablementPost(t, write, "t1", `{"kind":"COUNTRY","code":"ZZ","revision":1,"status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z"}`)
	if status != http.StatusOK || answer["outcome"] != "NOT_ACCEPTED" || answer["refusalReason"] != "CODE_NOT_IN_CATALOGUE" || answer["cause"] == "" {
		t.Fatalf("目录外的国家码 = %d %v, want 200 NOT_ACCEPTED / CODE_NOT_IN_CATALOGUE", status, answer)
	}
	status, answer = enablementPost(t, write, "t1", `{"kind":"CURRENCY","code":"XXX","revision":1,"status":"ENABLED","effectiveFrom":"2026-06-01T00:00:00Z"}`)
	if status != http.StatusOK || answer["refusalReason"] != "CODE_NOT_IN_CATALOGUE" {
		t.Fatalf("目录外的币种码 = %d %v", status, answer)
	}
	if len(registry.rows) != 0 {
		t.Fatalf("目录外的码一个字节没写：%d 行", len(registry.rows))
	}

	if status, _ := enablementPost(t, write, "t1", enableChina); status != http.StatusCreated {
		t.Fatalf("目录内的码照常登记，实际 %d", status)
	}
	if status, answer := enablementPost(t, write, "t1", enableChina); status != http.StatusOK || answer["outcome"] != "ALREADY_REGISTERED" {
		t.Fatalf("重放 = %d %v", status, answer)
	}
	conflicting := strings.Replace(enableChina, `"ENABLED"`, `"DISABLED"`, 1)
	if status, answer := enablementPost(t, write, "t1", conflicting); status != http.StatusOK || answer["outcome"] != "CONTENT_CONFLICT" {
		t.Fatalf("同键异内容 = %d %v", status, answer)
	}
	skipped := strings.Replace(enableChina, `"revision":1`, `"revision":5`, 1)
	if status, answer := enablementPost(t, write, "t1", skipped); status != http.StatusOK || answer["refusalReason"] != "REVISION_OUT_OF_SEQUENCE" {
		t.Fatalf("跳号 = %d %v", status, answer)
	}
}

// Covers: ADR-0345 决定四——读回口：种类必给且在封闭集内；未授予不读册；方法只放 GET；读册失败答 5xx 没形成答案；
// 状态由读的这一刻导出（未来生效的行答 NOT_CONFIGURED，停用后再启用以后到者为准）。
func TestEnablementReadBackDerivesTheStandingAtTheMomentOfReading(t *testing.T) {
	registry := &enablementRegistryDouble{}
	write, read := enablementEndpoints(t, registry)
	for _, body := range []string{
		enableChina,
		`{"kind":"COUNTRY","code":"CN","revision":2,"status":"DISABLED","effectiveFrom":"2026-06-10T00:00:00Z"}`,
		`{"kind":"COUNTRY","code":"CN","revision":3,"status":"ENABLED","effectiveFrom":"2026-06-20T00:00:00Z","source":"MEMO-3"}`,
		`{"kind":"COUNTRY","code":"SG","revision":1,"status":"ENABLED","effectiveFrom":"2026-08-01T00:00:00Z"}`,
	} {
		if status, answer := enablementPost(t, write, "t1", body); status != http.StatusCreated {
			t.Fatalf("登记失败 %d %v", status, answer)
		}
	}

	status, body := enablementRead(t, read, "t1", "?kind=COUNTRY")
	if status != http.StatusOK || body["outcome"] != "REFERENCE_ENABLEMENTS_LISTED" || body["kind"] != "COUNTRY" {
		t.Fatalf("读回 = %d %v", status, body)
	}
	entries := body["enablements"].([]any)
	if len(entries) != 2 {
		t.Fatalf("册上登过两个码，实际 %d", len(entries))
	}
	china, singapore := entries[0].(map[string]any), entries[1].(map[string]any)
	if china["code"] != "CN" || china["standing"] != "ENABLED" || china["latestRevision"] != float64(3) || len(china["history"].([]any)) != 3 {
		t.Fatalf("停用后再启用以后到者为准：%v", china)
	}
	if singapore["code"] != "SG" || singapore["standing"] != "NOT_CONFIGURED" || singapore["effectiveFrom"] != nil {
		t.Fatalf("只有未来生效的行是未配置，不是启用：%v", singapore)
	}

	for query, want := range map[string]int{"": http.StatusBadRequest, "?kind=REGION": http.StatusBadRequest, "?kind=country": http.StatusBadRequest} {
		if status, _ := enablementRead(t, read, "t1", query); status != want {
			t.Fatalf("%q = %d, want %d", query, status, want)
		}
	}
	if status, _ := enablementRead(t, read, "no-such-token", "?kind=COUNTRY"); status != http.StatusForbidden {
		t.Fatalf("未授予 = %d, want 403", status)
	}
	if status, _ := enablementRead(t, read, "t1", "?kind=CURRENCY"); status != http.StatusOK {
		t.Fatalf("另一种类读空册也是 200，实际 %d", status)
	}
	recorder := httptest.NewRecorder()
	read.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/reference-data/enablement?kind=COUNTRY", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("读口只放 GET，实际 %d", recorder.Code)
	}
}

type failingEnablementReader struct{}

func (failingEnablementReader) ListReferenceEnablements(context.Context, domain.TenantID, domain.ReferenceKind) (time.Time, []application.ReferenceEnablementEntry, error) {
	return time.Time{}, nil, errors.New("db down")
}

func TestEnablementReadFailureIsNoAnswerNotAnEmptyRegister(t *testing.T) {
	read := commercialhttp.NewQueryReferenceEnablementsEndpoint(
		enablementCatalogueIntake{tenants: map[string]string{"t1": "SYN-TENANT-01"}}, failingEnablementReader{})
	if status, _ := enablementRead(t, read, "t1", "?kind=COUNTRY"); status != http.StatusInternalServerError {
		t.Fatalf("读册失败 = %d, want 500", status)
	}
}

// Covers: 接入渠道未配置时两口一律 403 ACCESS_CHANNEL_NOT_CONFIGURED，不读载荷、不调编排。
func TestEnablementEndpointsRefuseWithoutAChannel(t *testing.T) {
	write := commercialhttp.NewRegisterReferenceEnablementEndpoint(commercialhttp.UnconfiguredIntake{}, panicRegistrar{})
	probe := &bodyReadProbe{}
	recorder := httptest.NewRecorder()
	write.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/reference-data/enablement-registrations", probe))
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") || probe.read {
		t.Fatalf("登记口：%d %s 读了载荷 = %v", recorder.Code, recorder.Body, probe.read)
	}

	read := commercialhttp.NewQueryReferenceEnablementsEndpoint(commercialhttp.UnconfiguredIntake{}, failingEnablementReader{})
	recorder = httptest.NewRecorder()
	read.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/reference-data/enablement?kind=COUNTRY", nil))
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") {
		t.Fatalf("读回口：%d %s", recorder.Code, recorder.Body)
	}
}

type panicRegistrar struct{}

func (panicRegistrar) Register(context.Context, application.RegisterReferenceEnablementCommand) (application.ReferenceEnablementResult, error) {
	panic("未配置态不该调到登记编排")
}
