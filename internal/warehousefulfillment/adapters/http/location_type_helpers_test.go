package warehousehttp_test

import (
	"context"
	"encoding/json"
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

// 本包库位类型的用例同样不接真库：登记走真的 application.RegisterLocationTypeHandler 加内存库位类型册与内存「本租户有没有仓」，所以各种结果
// 都是用例自己算出来的，不是测试替身编出来的；读口与认证用假的（认证的假件与仓那一册共用，见 helpers_test.go）。

// memoryLocationTypeRegistry 是内存里的库位类型登记册：键是（租户，库位类型编码，修订号），撞键同内容答已在册、异内容答冲突。reads 与
// writes 数登记用例碰过册几次，信封一层与坏载荷的用例据此证明「不读册、不写册」。
type memoryLocationTypeRegistry struct {
	mutex     sync.Mutex
	revisions map[string]domain.LocationTypeRevision
	reads     int
	writes    int
	loadErr   error
}

func newMemoryLocationTypeRegistry() *memoryLocationTypeRegistry {
	return &memoryLocationTypeRegistry{revisions: map[string]domain.LocationTypeRevision{}}
}

func locationTypeRegistryKey(tenant domain.TenantID, code domain.LocationTypeCode, revision int) string {
	return tenant.String() + "\x00" + code.String() + "\x00" + strconv.Itoa(revision)
}

func (registry *memoryLocationTypeRegistry) LoadLatestLocationType(
	_ context.Context, tenant domain.TenantID, code domain.LocationTypeCode,
) (domain.LocationTypeRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.LocationTypeRevision{}, false, registry.loadErr
	}
	for revision := 9; revision >= 1; revision-- {
		if found, ok := registry.revisions[locationTypeRegistryKey(tenant, code, revision)]; ok {
			return found, true, nil
		}
	}
	return domain.LocationTypeRevision{}, false, nil
}

func (registry *memoryLocationTypeRegistry) LoadLocationTypeRevision(
	_ context.Context, tenant domain.TenantID, code domain.LocationTypeCode, revision int,
) (domain.LocationTypeRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.LocationTypeRevision{}, false, registry.loadErr
	}
	found, ok := registry.revisions[locationTypeRegistryKey(tenant, code, revision)]
	return found, ok, nil
}

func (registry *memoryLocationTypeRegistry) SaveLocationTypeRevision(
	_ context.Context, revision domain.LocationTypeRevision,
) (ports.RegistrySaveOutcome, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.writes++
	key := locationTypeRegistryKey(revision.Tenant(), revision.Code(), revision.Revision())
	if existing, ok := registry.revisions[key]; ok {
		if existing.SameContentAs(revision) {
			return ports.RegistryRevisionAlreadyRegistered, nil
		}
		return ports.RegistryRevisionContentConflict, nil
	}
	registry.revisions[key] = revision
	return ports.RegistryRevisionSaved, nil
}

func (registry *memoryLocationTypeRegistry) touched() int {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	return registry.reads + registry.writes
}

// memoryTenantWarehouses 是「本租户有没有登记过仓」的内存替身：按租户记。reads 数问过几次，坏载荷与信封一层的用例据此证明登记用例没有被走到。
type memoryTenantWarehouses struct {
	mutex      sync.Mutex
	registered map[string]bool
	reads      int
	err        error
}

func newMemoryTenantWarehouses(tenants ...string) *memoryTenantWarehouses {
	lookup := &memoryTenantWarehouses{registered: map[string]bool{}}
	for _, tenant := range tenants {
		lookup.registered[tenant] = true
	}
	return lookup
}

func (lookup *memoryTenantWarehouses) HasAnyWarehouse(_ context.Context, tenant domain.TenantID) (bool, error) {
	lookup.mutex.Lock()
	defer lookup.mutex.Unlock()
	lookup.reads++
	if lookup.err != nil {
		return false, lookup.err
	}
	return lookup.registered[tenant.String()], nil
}

// locationTypeRegisterFixture 把库位类型登记口、它的认证、库位类型册、「有没有仓」与时钟装在一起：租户 SYN-TENANT-01 已登记过仓。
type locationTypeRegisterFixture struct {
	handler       http.Handler
	authenticator *fakeRegistryAuthenticator
	locationTypes *memoryLocationTypeRegistry
	warehouses    *memoryTenantWarehouses
	now           *time.Time
}

func newLocationTypeRegisterFixture(t *testing.T) *locationTypeRegisterFixture {
	t.Helper()
	now := firstInstant
	fixture := &locationTypeRegisterFixture{
		authenticator: &fakeRegistryAuthenticator{identity: operatorIdentity(t, tenantA, operatorOne)},
		locationTypes: newMemoryLocationTypeRegistry(),
		warehouses:    newMemoryTenantWarehouses(tenantA),
		now:           &now,
	}
	intake, err := warehousehttp.NewOperatorRegistryIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.handler = warehousehttp.NewRegisterLocationTypeEndpoint(
		intake, application.NewRegisterLocationTypeHandler(fixture.locationTypes, fixture.warehouses, fixedClock{at: fixture.now}))
	return fixture
}

func (fixture *locationTypeRegisterFixture) post(body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, locationTypeRegistrationsPath, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	return response
}

// touched 数库位类型册与「有没有仓」一共被碰过几次。
func (fixture *locationTypeRegisterFixture) touched() int {
	fixture.warehouses.mutex.Lock()
	defer fixture.warehouses.mutex.Unlock()
	return fixture.locationTypes.touched() + fixture.warehouses.reads
}

// locationTypeJSON 拼一份库位类型登记报文：字段缺省值是一个合法的库位类型，用例只改它关心的格。
type locationTypeJSON struct {
	code, name  string
	revision    int
	deactivated bool
}

func syntheticLocationType(code string, revision int) locationTypeJSON {
	return locationTypeJSON{code: code, revision: revision, name: "合成货架位"}
}

func (locationType locationTypeJSON) String() string {
	raw, err := json.Marshal(map[string]any{
		"locationTypeCode": locationType.code,
		"revision":         locationType.revision,
		"name":             locationType.name,
		"deactivated":      locationType.deactivated,
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// syntheticLocationTypeRow 造读面的一行。
func syntheticLocationTypeRow(code string, revision int, deactivated bool) ports.LocationTypeRow {
	return ports.LocationTypeRow{
		LocationTypeCode: code, Revision: revision, Name: "合成货架位", Deactivated: deactivated,
		Registrant: operatorOne, RegisteredAt: firstInstant,
	}
}

// fakeLocationTypeCatalogue 同时是库位类型列表与修订史的读口。current 是各租户当前修订，history 按（租户，库位类型编码）给修订史；
// fail 非空即交回它。
type fakeLocationTypeCatalogue struct {
	current map[string][]ports.LocationTypeRow
	history map[string][]ports.LocationTypeRow
	fail    error
	next    string

	listed       int
	revisions    int
	gotTenant    string
	gotLimit     int
	gotQuery     cataloguepage.Query
	gotRevTenant string
	gotRevCode   string
}

func (catalogue *fakeLocationTypeCatalogue) ListLocationTypes(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.LocationTypeRow], error) {
	catalogue.listed++
	catalogue.gotTenant, catalogue.gotLimit, catalogue.gotQuery = tenant.String(), limit, query
	if catalogue.fail != nil {
		return cataloguepage.Result[ports.LocationTypeRow]{}, catalogue.fail
	}
	rows := catalogue.current[tenant.String()]
	return cataloguepage.Result[ports.LocationTypeRow]{Rows: rows, Total: int64(len(rows)), Next: catalogue.next}, nil
}

func (catalogue *fakeLocationTypeCatalogue) LocationTypeRevisions(
	_ context.Context, tenant domain.TenantID, code domain.LocationTypeCode,
) ([]ports.LocationTypeRow, error) {
	catalogue.revisions++
	catalogue.gotRevTenant, catalogue.gotRevCode = tenant.String(), code.String()
	if catalogue.fail != nil {
		return nil, catalogue.fail
	}
	return catalogue.history[tenant.String()+"\x00"+code.String()], nil
}

// locationTypeReadFixture 把库位类型各读口、它们的认证与假读面装在一起。
type locationTypeReadFixture struct {
	list          http.Handler
	revisionsLine http.Handler
	authenticator *fakeReadAuthenticator
	catalogue     *fakeLocationTypeCatalogue
}

func newLocationTypeReadFixture(t *testing.T, catalogue *fakeLocationTypeCatalogue) *locationTypeReadFixture {
	t.Helper()
	fixture := &locationTypeReadFixture{
		authenticator: &fakeReadAuthenticator{tenant: valueOf(t, domain.NewTenantID, tenantA)},
		catalogue:     catalogue,
	}
	intake, err := warehousehttp.NewOperatorOperationsReadIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.list = warehousehttp.NewQueryLocationTypesEndpoint(intake, catalogue)
	fixture.revisionsLine = warehousehttp.NewQueryLocationTypeRevisionsEndpoint(intake, catalogue)
	return fixture
}
