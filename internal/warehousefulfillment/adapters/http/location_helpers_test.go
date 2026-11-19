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

// 本包库位的用例同样不接真库：登记走真的 application.RegisterLocationHandler 加内存库位册、内存仓册、内存库区册与内存库位类型册，所以
// 各种结果都是用例自己算出来的，不是测试替身编出来的；读口与认证用假的（认证的假件与仓那一册共用，见 helpers_test.go）。

// memoryLocationRegistry 是内存里的库位登记册：键是（租户，仓编码，库位编码，修订号），撞键同内容答已在册、异内容答冲突。reads 与
// writes 数登记用例碰过册几次，信封一层与坏载荷的用例据此证明「不读册、不写册」。
type memoryLocationRegistry struct {
	mutex     sync.Mutex
	revisions map[string]domain.LocationRevision
	reads     int
	writes    int
	loadErr   error
}

func newMemoryLocationRegistry() *memoryLocationRegistry {
	return &memoryLocationRegistry{revisions: map[string]domain.LocationRevision{}}
}

func locationRegistryKey(tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.LocationCode, revision int) string {
	return tenant.String() + "\x00" + warehouse.String() + "\x00" + code.String() + "\x00" + strconv.Itoa(revision)
}

func (registry *memoryLocationRegistry) LoadLatestLocation(
	_ context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.LocationCode,
) (domain.LocationRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.LocationRevision{}, false, registry.loadErr
	}
	for revision := 9; revision >= 1; revision-- {
		if found, ok := registry.revisions[locationRegistryKey(tenant, warehouse, code, revision)]; ok {
			return found, true, nil
		}
	}
	return domain.LocationRevision{}, false, nil
}

func (registry *memoryLocationRegistry) LoadLocationRevision(
	_ context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.LocationCode, revision int,
) (domain.LocationRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.LocationRevision{}, false, registry.loadErr
	}
	found, ok := registry.revisions[locationRegistryKey(tenant, warehouse, code, revision)]
	return found, ok, nil
}

func (registry *memoryLocationRegistry) SaveLocationRevision(
	_ context.Context, revision domain.LocationRevision,
) (ports.RegistrySaveOutcome, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.writes++
	key := locationRegistryKey(revision.Tenant(), revision.Warehouse(), revision.Code(), revision.Revision())
	if existing, ok := registry.revisions[key]; ok {
		if existing.SameContentAs(revision) {
			return ports.RegistryRevisionAlreadyRegistered, nil
		}
		return ports.RegistryRevisionContentConflict, nil
	}
	registry.revisions[key] = revision
	return ports.RegistryRevisionSaved, nil
}

func (registry *memoryLocationRegistry) touched() int {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	return registry.reads + registry.writes
}

// memoryZoneLookup 是「所属库区已登记且属于这个仓」的内存替身：按（租户，仓编码，库区编码）记在册的库区。reads 数问过几次。
type memoryZoneLookup struct {
	mutex      sync.Mutex
	registered map[string]bool
	reads      int
	err        error
}

func newMemoryZoneLookup() *memoryZoneLookup {
	return &memoryZoneLookup{registered: map[string]bool{}}
}

func zoneLookupKey(tenant, warehouse, zone string) string {
	return tenant + "\x00" + warehouse + "\x00" + zone
}

func (lookup *memoryZoneLookup) LoadLatestZone(
	_ context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ZoneCode,
) (domain.ZoneRevision, bool, error) {
	lookup.mutex.Lock()
	defer lookup.mutex.Unlock()
	lookup.reads++
	if lookup.err != nil {
		return domain.ZoneRevision{}, false, lookup.err
	}
	return domain.ZoneRevision{}, lookup.registered[zoneLookupKey(tenant.String(), warehouse.String(), code.String())], nil
}

// memoryLocationTypeLookup 是库位类型的内存替身：按（租户，库位类型编码）记在册的类型，值是它此刻是否停用。登记用例只看读到的那一笔
// 的 Content().Deactivated()，所以这里造一笔带着停用标记的修订。
type memoryLocationTypeLookup struct {
	mutex       sync.Mutex
	deactivated map[string]bool
	reads       int
	err         error
}

func newMemoryLocationTypeLookup() *memoryLocationTypeLookup {
	return &memoryLocationTypeLookup{deactivated: map[string]bool{}}
}

func (lookup *memoryLocationTypeLookup) LoadLatestLocationType(
	_ context.Context, tenant domain.TenantID, code domain.LocationTypeCode,
) (domain.LocationTypeRevision, bool, error) {
	lookup.mutex.Lock()
	defer lookup.mutex.Unlock()
	lookup.reads++
	if lookup.err != nil {
		return domain.LocationTypeRevision{}, false, lookup.err
	}
	deactivated, registered := lookup.deactivated[tenant.String()+"\x00"+code.String()]
	if !registered {
		return domain.LocationTypeRevision{}, false, nil
	}
	name, err := domain.NewLocationTypeName("合成库位类型")
	if err != nil {
		return domain.LocationTypeRevision{}, false, err
	}
	content, err := domain.NewLocationTypeContent(domain.LocationTypeContentSpec{Name: name, Deactivated: deactivated})
	if err != nil {
		return domain.LocationTypeRevision{}, false, err
	}
	registrant, err := domain.NewRegistrant(operatorOne)
	if err != nil {
		return domain.LocationTypeRevision{}, false, err
	}
	revision, err := domain.NewLocationTypeRevision(tenant, code, 1, content, registrant, firstInstant)
	return revision, err == nil, err
}

// locationRegisterFixture 把库位登记口、它的认证、库位册、仓册、库区册、库位类型册与时钟装在一起：租户 SYN-TENANT-01 的仓 SYN-WH-01 已登记，
// 仓下有库区 SYN-ZONE-01，租户有一个在用的库位类型 SYN-LT-01。
type locationRegisterFixture struct {
	handler       http.Handler
	authenticator *fakeRegistryAuthenticator
	locations     *memoryLocationRegistry
	warehouses    *memoryWarehouseLookup
	zones         *memoryZoneLookup
	types         *memoryLocationTypeLookup
	now           *time.Time
}

func newLocationRegisterFixture(t *testing.T) *locationRegisterFixture {
	t.Helper()
	now := firstInstant
	fixture := &locationRegisterFixture{
		authenticator: &fakeRegistryAuthenticator{identity: operatorIdentity(t, tenantA, operatorOne)},
		locations:     newMemoryLocationRegistry(),
		warehouses:    newMemoryWarehouseLookup(tenantA, "SYN-WH-01"),
		zones:         newMemoryZoneLookup(),
		types:         newMemoryLocationTypeLookup(),
		now:           &now,
	}
	fixture.zones.registered[zoneLookupKey(tenantA, "SYN-WH-01", "SYN-ZONE-01")] = true
	fixture.types.deactivated[tenantA+"\x00SYN-LT-01"] = false
	intake, err := warehousehttp.NewOperatorRegistryIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.handler = warehousehttp.NewRegisterLocationEndpoint(intake, application.NewRegisterLocationHandler(
		fixture.locations, fixture.warehouses, fixture.zones, fixture.types, fixedClock{at: fixture.now}))
	return fixture
}

func (fixture *locationRegisterFixture) post(body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, locationRegistrationsPath, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	return response
}

// touched 数库位册、仓册、库区册与库位类型册一共被碰过几次。
func (fixture *locationRegisterFixture) touched() int {
	fixture.warehouses.mutex.Lock()
	warehouseReads := fixture.warehouses.reads
	fixture.warehouses.mutex.Unlock()
	fixture.zones.mutex.Lock()
	zoneReads := fixture.zones.reads
	fixture.zones.mutex.Unlock()
	fixture.types.mutex.Lock()
	typeReads := fixture.types.reads
	fixture.types.mutex.Unlock()
	return fixture.locations.touched() + warehouseReads + zoneReads + typeReads
}

// locationJSON 拼一份库位登记报文：字段缺省值是 SYN-WH-01 下 SYN-ZONE-01 里的一个库位、不带库位类型与优先级，用例只改它关心的格。
// 库位类型与优先级可缺：没设就不写那个键。
type locationJSON struct {
	warehouse, code, zone string
	revision              int
	locationType          *string
	priority              *int32
	deactivated           bool
}

func syntheticLocation(code string, revision int) locationJSON {
	return locationJSON{warehouse: "SYN-WH-01", code: code, zone: "SYN-ZONE-01", revision: revision}
}

func (location locationJSON) typed(code string) locationJSON {
	location.locationType = &code
	return location
}

func (location locationJSON) prioritised(value int32) locationJSON {
	location.priority = &value
	return location
}

func (location locationJSON) String() string {
	document := map[string]any{
		"warehouseCode": location.warehouse,
		"locationCode":  location.code,
		"revision":      location.revision,
		"zoneCode":      location.zone,
		"deactivated":   location.deactivated,
	}
	if location.locationType != nil {
		document["locationTypeCode"] = *location.locationType
	}
	if location.priority != nil {
		document["priority"] = *location.priority
	}
	raw, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// syntheticLocationRow 造读面的一行：不带库位类型与优先级。
func syntheticLocationRow(warehouse, code string, revision int, deactivated bool) ports.LocationRow {
	return ports.LocationRow{
		WarehouseCode: warehouse, LocationCode: code, Revision: revision, ZoneCode: "SYN-ZONE-01",
		Deactivated: deactivated, Registrant: operatorOne, RegisteredAt: firstInstant,
	}
}

// fakeLocationCatalogue 同时是库位列表与修订史的读口。current 是各租户当前修订，history 按（租户，仓编码，库位编码）给修订史；
// fail 非空即交回它。
type fakeLocationCatalogue struct {
	current map[string][]ports.LocationRow
	history map[string][]ports.LocationRow
	fail    error
	next    string

	listed       int
	revisions    int
	gotTenant    string
	gotLimit     int
	gotQuery     cataloguepage.Query
	gotRevTenant string
	gotRevWH     string
	gotRevCode   string
}

func (catalogue *fakeLocationCatalogue) ListLocations(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.LocationRow], error) {
	catalogue.listed++
	catalogue.gotTenant, catalogue.gotLimit, catalogue.gotQuery = tenant.String(), limit, query
	if catalogue.fail != nil {
		return cataloguepage.Result[ports.LocationRow]{}, catalogue.fail
	}
	rows := catalogue.current[tenant.String()]
	return cataloguepage.Result[ports.LocationRow]{Rows: rows, Total: int64(len(rows)), Next: catalogue.next}, nil
}

func (catalogue *fakeLocationCatalogue) LocationRevisions(
	_ context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.LocationCode,
) ([]ports.LocationRow, error) {
	catalogue.revisions++
	catalogue.gotRevTenant, catalogue.gotRevWH, catalogue.gotRevCode = tenant.String(), warehouse.String(), code.String()
	if catalogue.fail != nil {
		return nil, catalogue.fail
	}
	return catalogue.history[tenant.String()+"\x00"+warehouse.String()+"\x00"+code.String()], nil
}

// locationReadFixture 把库位各读口、它们的认证与假读面装在一起。
type locationReadFixture struct {
	list          http.Handler
	revisionsLine http.Handler
	authenticator *fakeReadAuthenticator
	catalogue     *fakeLocationCatalogue
}

func newLocationReadFixture(t *testing.T, catalogue *fakeLocationCatalogue) *locationReadFixture {
	t.Helper()
	fixture := &locationReadFixture{
		authenticator: &fakeReadAuthenticator{tenant: valueOf(t, domain.NewTenantID, tenantA)},
		catalogue:     catalogue,
	}
	intake, err := warehousehttp.NewOperatorOperationsReadIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.list = warehousehttp.NewQueryLocationsEndpoint(intake, catalogue)
	fixture.revisionsLine = warehousehttp.NewQueryLocationRevisionsEndpoint(intake, catalogue)
	return fixture
}
