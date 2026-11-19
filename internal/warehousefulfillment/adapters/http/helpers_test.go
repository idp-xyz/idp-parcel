package warehousehttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本包的用例不接真库：登记走真的 application.RegisterWarehouseHandler 加内存登记册，所以各种结果都是用例自己算出来的，不是
// 测试替身编出来的；读口与认证用假的。租户、操作者与仓都是隔离合成值，只记 `S`。

const (
	tenantA       = "SYN-TENANT-01"
	tenantB       = "SYN-TENANT-02"
	operatorOne   = "https://id.syn.example/dex#SYN-OPERATOR-01"
	operatorTwo   = "https://id.syn.example/dex#SYN-OPERATOR-02"
	secretAddress = "SYN-SECRET-ADDRESS-TEXT"
)

var (
	firstInstant = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	laterInstant = firstInstant.Add(3 * time.Hour)
)

func valueOf[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

type fixedClock struct{ at *time.Time }

func (clock fixedClock) Now() time.Time { return *clock.at }

// memoryRegistry 是内存里的仓登记册：键是（租户，仓编码，修订号），撞键同内容答已在册、异内容答冲突。reads 与 writes 数
// 登记用例碰过册几次，信封一层与坏载荷的用例据此证明「不读册、不写册」。
type memoryRegistry struct {
	mutex     sync.Mutex
	revisions map[string]domain.WarehouseRevision
	reads     int
	writes    int
	loadErr   error
}

func newMemoryRegistry() *memoryRegistry {
	return &memoryRegistry{revisions: map[string]domain.WarehouseRevision{}}
}

func registryKey(tenant domain.TenantID, code domain.WarehouseCode, revision int) string {
	return tenant.String() + "\x00" + code.String() + "\x00" + strconv.Itoa(revision)
}

func (registry *memoryRegistry) LoadLatestWarehouse(
	_ context.Context, tenant domain.TenantID, code domain.WarehouseCode,
) (domain.WarehouseRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.WarehouseRevision{}, false, registry.loadErr
	}
	for revision := 9; revision >= 1; revision-- {
		if found, ok := registry.revisions[registryKey(tenant, code, revision)]; ok {
			return found, true, nil
		}
	}
	return domain.WarehouseRevision{}, false, nil
}

func (registry *memoryRegistry) LoadWarehouseRevision(
	_ context.Context, tenant domain.TenantID, code domain.WarehouseCode, revision int,
) (domain.WarehouseRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.WarehouseRevision{}, false, registry.loadErr
	}
	found, ok := registry.revisions[registryKey(tenant, code, revision)]
	return found, ok, nil
}

func (registry *memoryRegistry) SaveWarehouseRevision(
	_ context.Context, revision domain.WarehouseRevision,
) (ports.WarehouseSaveOutcome, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.writes++
	key := registryKey(revision.Tenant(), revision.Code(), revision.Revision())
	if existing, ok := registry.revisions[key]; ok {
		if existing.SameContentAs(revision) {
			return ports.WarehouseRevisionAlreadyRegistered, nil
		}
		return ports.WarehouseRevisionContentConflict, nil
	}
	registry.revisions[key] = revision
	return ports.WarehouseRevisionSaved, nil
}

func (registry *memoryRegistry) touched() int {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	return registry.reads + registry.writes
}

// fakeRegistryAuthenticator 记下被问了几次、收到的令牌；err 非空即交回它。
type fakeRegistryAuthenticator struct {
	identity warehousehttp.OperatorIdentity
	err      error
	asked    int
	token    string
}

func (authenticator *fakeRegistryAuthenticator) AuthenticateRegistryWrite(
	_ context.Context, bearerToken string,
) (warehousehttp.OperatorIdentity, error) {
	authenticator.asked++
	authenticator.token = bearerToken
	if authenticator.err != nil {
		return warehousehttp.OperatorIdentity{}, authenticator.err
	}
	return authenticator.identity, nil
}

type fakeReadAuthenticator struct {
	tenant domain.TenantID
	err    error
	asked  int
}

func (authenticator *fakeReadAuthenticator) AuthenticateOperationsRead(_ context.Context, _ string) (domain.TenantID, error) {
	authenticator.asked++
	if authenticator.err != nil {
		return domain.TenantID{}, authenticator.err
	}
	return authenticator.tenant, nil
}

func operatorIdentity(t *testing.T, tenant, registrant string) warehousehttp.OperatorIdentity {
	t.Helper()
	return warehousehttp.OperatorIdentity{
		Tenant:     valueOf(t, domain.NewTenantID, tenant),
		Registrant: valueOf(t, domain.NewRegistrant, registrant),
	}
}

// registerFixture 把登记口、它的认证、登记册与时钟装在一起。
type registerFixture struct {
	handler       http.Handler
	authenticator *fakeRegistryAuthenticator
	registry      *memoryRegistry
	now           *time.Time
}

func newRegisterFixture(t *testing.T) *registerFixture {
	t.Helper()
	now := firstInstant
	fixture := &registerFixture{
		authenticator: &fakeRegistryAuthenticator{identity: operatorIdentity(t, tenantA, operatorOne)},
		registry:      newMemoryRegistry(),
		now:           &now,
	}
	intake, err := warehousehttp.NewOperatorRegistryIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.handler = warehousehttp.NewRegisterWarehouseEndpoint(
		intake, application.NewRegisterWarehouseHandler(fixture.registry, fixedClock{at: fixture.now}))
	return fixture
}

func (fixture *registerFixture) post(body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/warehouse-fulfillment-warehouse-registrations", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	return response
}

// warehouseJSON 拼一份登记报文：字段缺省值是一个合法的自营仓，用例只改它关心的格。
type warehouseJSON struct {
	code, name, kind, country, text, zone, node string
	revision                                    int
	deactivated                                 bool
}

func syntheticWarehouse(code string, revision int) warehouseJSON {
	return warehouseJSON{
		code: code, revision: revision, name: "合成一号仓", kind: "SELF_OPERATED", country: "CN",
		text: "合成路 1 号", zone: "Asia/Shanghai", node: "SYN-NODE-01",
	}
}

func (warehouse warehouseJSON) String() string {
	document := map[string]any{
		"warehouseCode": warehouse.code,
		"revision":      warehouse.revision,
		"name":          warehouse.name,
		"kind":          warehouse.kind,
		"address":       map[string]string{"country": warehouse.country, "text": warehouse.text},
		"deactivated":   warehouse.deactivated,
	}
	if warehouse.zone != "" {
		document["businessTimeZone"] = warehouse.zone
	}
	if warehouse.node != "" {
		document["colocatedNode"] = warehouse.node
	}
	raw, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func decodeObject(t *testing.T, response *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &object); err != nil {
		t.Fatalf("答复体不是 JSON 对象：%q（%v）", response.Body.String(), err)
	}
	return object
}

func stringMember(t *testing.T, object map[string]json.RawMessage, key string) string {
	t.Helper()
	raw, present := object[key]
	if !present {
		t.Fatalf("答复体没有 %s：%v", key, keysOf(object))
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("%s 不是字符串：%s", key, raw)
	}
	return value
}

func keysOf(object map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	return keys
}

// assertKeys 断言答复体的顶层键恰是 want：多出来的键与缺的键一样是契约漂移。
func assertKeys(t *testing.T, name string, object map[string]json.RawMessage, want ...string) {
	t.Helper()
	if len(object) != len(want) {
		t.Fatalf("%s：答复体键 %v，want %v", name, keysOf(object), want)
	}
	for _, key := range want {
		if _, present := object[key]; !present {
			t.Fatalf("%s：答复体键 %v，want %v", name, keysOf(object), want)
		}
	}
}

// assertProblem 断言一份操作者各族的旧形状问题体 {"error":{"code","detail"?}}：状态码、码，且顶层不带 `outcome`——问题体只说明为什么
// 没有 `outcome`（ADR-0022）。交回 error 对象的各成员，供用例核 detail。
func assertProblem(t *testing.T, name string, response *httptest.ResponseRecorder, status int, code string) map[string]json.RawMessage {
	t.Helper()
	if response.Code != status {
		t.Fatalf("%s：状态 %d %s，want %d %s", name, response.Code, response.Body.String(), status, code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("%s：Content-Type = %q，want application/json", name, contentType)
	}
	outer := decodeObject(t, response)
	assertKeys(t, name, outer, "error")
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(outer["error"], &inner); err != nil {
		t.Fatalf("%s：error 不是对象：%s", name, response.Body.String())
	}
	if got := stringMember(t, inner, "code"); got != code {
		t.Fatalf("%s：code = %q，want %q（%s）", name, got, code, response.Body.String())
	}
	return inner
}

// assertProblemDetails 断言一份 RFC 9457 问题详情：状态码、码、媒体类型，且不带 `outcome`。只有开关关着那一格写这个形状。
func assertProblemDetails(t *testing.T, name string, response *httptest.ResponseRecorder, status int, code string) map[string]json.RawMessage {
	t.Helper()
	if response.Code != status {
		t.Fatalf("%s：状态 %d %s，want %d %s", name, response.Code, response.Body.String(), status, code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("%s：Content-Type = %q，want application/problem+json", name, contentType)
	}
	object := decodeObject(t, response)
	if got := stringMember(t, object, "code"); got != code {
		t.Fatalf("%s：code = %q，want %q（%s）", name, got, code, response.Body.String())
	}
	if _, present := object["outcome"]; present {
		t.Fatalf("%s：一个没形成答案的响应带了 outcome：%s", name, response.Body.String())
	}
	return object
}

// countingBody 是一份记下被读了几次的请求体。
type countingBody struct {
	content string
	reads   int
	reader  io.Reader
}

func newCountingBody(content string) *countingBody {
	return &countingBody{content: content, reader: strings.NewReader(content)}
}

func (body *countingBody) Read(p []byte) (int, error) {
	body.reads++
	return body.reader.Read(p)
}

// fakeCatalogue 同时是列表与修订史的读口。rows 是各租户当前修订，history 按（租户，仓编码）给修订史；fail 非空即交回它。
type fakeCatalogue struct {
	current map[string][]ports.WarehouseRow
	history map[string][]ports.WarehouseRow
	fail    error
	next    string

	listed       int
	revisions    int
	gotTenant    string
	gotLimit     int
	gotQuery     cataloguepage.Query
	gotRevCode   string
	gotRevTenant string
}

func (catalogue *fakeCatalogue) ListWarehouses(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.WarehouseRow], error) {
	catalogue.listed++
	catalogue.gotTenant, catalogue.gotLimit, catalogue.gotQuery = tenant.String(), limit, query
	if catalogue.fail != nil {
		return cataloguepage.Result[ports.WarehouseRow]{}, catalogue.fail
	}
	rows := catalogue.current[tenant.String()]
	return cataloguepage.Result[ports.WarehouseRow]{Rows: rows, Total: int64(len(rows)), Next: catalogue.next}, nil
}

func (catalogue *fakeCatalogue) WarehouseRevisions(
	_ context.Context, tenant domain.TenantID, code domain.WarehouseCode,
) ([]ports.WarehouseRow, error) {
	catalogue.revisions++
	catalogue.gotRevTenant, catalogue.gotRevCode = tenant.String(), code.String()
	if catalogue.fail != nil {
		return nil, catalogue.fail
	}
	return catalogue.history[tenant.String()+"\x00"+code.String()], nil
}

func syntheticRow(code string, revision int, deactivated bool) ports.WarehouseRow {
	return ports.WarehouseRow{
		WarehouseCode: code, Revision: revision, Name: "合成一号仓", Kind: "SELF_OPERATED",
		CountryCode: "CN", AddressText: "合成路 1 号",
		BusinessTimeZone: "Asia/Shanghai", HasBusinessTimeZone: true,
		ColocatedNode: "SYN-NODE-01", HasColocatedNode: true,
		Deactivated: deactivated, Registrant: operatorOne, RegisteredAt: firstInstant,
	}
}

// readFixture 把各读口、它们的认证与假读面装在一起。
type readFixture struct {
	list          http.Handler
	revisionsLine http.Handler
	authenticator *fakeReadAuthenticator
	catalogue     *fakeCatalogue
}

func newReadFixture(t *testing.T, catalogue *fakeCatalogue) *readFixture {
	t.Helper()
	fixture := &readFixture{
		authenticator: &fakeReadAuthenticator{tenant: valueOf(t, domain.NewTenantID, tenantA)},
		catalogue:     catalogue,
	}
	intake, err := warehousehttp.NewOperatorOperationsReadIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.list = warehousehttp.NewQueryWarehousesEndpoint(intake, catalogue)
	fixture.revisionsLine = warehousehttp.NewQueryWarehouseRevisionsEndpoint(intake, catalogue)
	return fixture
}

func get(handler http.Handler, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// envelopeCases 是信封一层的各格：认证交回的错与它对应的状态码、码。
var envelopeCases = []struct {
	name   string
	err    error
	status int
	code   string
}{
	{"issuer parameters unset", warehousehttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
	{"credential rejected", warehousehttp.ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
	{"not granted", warehousehttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
	{"identity dependency unavailable", warehousehttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	{"an unclassified failure", errors.New("minting blew up"), http.StatusInternalServerError, "INTAKE_FAILED"},
}
