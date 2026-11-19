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

// 本包库区的用例同样不接真库：登记走真的 application.RegisterZoneHandler 加内存库区册与内存仓册，所以各种结果都是用例自己算出来
// 的，不是测试替身编出来的；读口与认证用假的（认证的假件与仓那一册共用，见 helpers_test.go）。

// memoryZoneRegistry 是内存里的库区登记册：键是（租户，仓编码，库区编码，修订号），撞键同内容答已在册、异内容答冲突。reads 与
// writes 数登记用例碰过册几次，信封一层与坏载荷的用例据此证明「不读册、不写册」。
type memoryZoneRegistry struct {
	mutex     sync.Mutex
	revisions map[string]domain.ZoneRevision
	reads     int
	writes    int
	loadErr   error
}

func newMemoryZoneRegistry() *memoryZoneRegistry {
	return &memoryZoneRegistry{revisions: map[string]domain.ZoneRevision{}}
}

func zoneRegistryKey(tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ZoneCode, revision int) string {
	return tenant.String() + "\x00" + warehouse.String() + "\x00" + code.String() + "\x00" + strconv.Itoa(revision)
}

func (registry *memoryZoneRegistry) LoadLatestZone(
	_ context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ZoneCode,
) (domain.ZoneRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.ZoneRevision{}, false, registry.loadErr
	}
	for revision := 9; revision >= 1; revision-- {
		if found, ok := registry.revisions[zoneRegistryKey(tenant, warehouse, code, revision)]; ok {
			return found, true, nil
		}
	}
	return domain.ZoneRevision{}, false, nil
}

func (registry *memoryZoneRegistry) LoadZoneRevision(
	_ context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ZoneCode, revision int,
) (domain.ZoneRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.ZoneRevision{}, false, registry.loadErr
	}
	found, ok := registry.revisions[zoneRegistryKey(tenant, warehouse, code, revision)]
	return found, ok, nil
}

func (registry *memoryZoneRegistry) SaveZoneRevision(
	_ context.Context, revision domain.ZoneRevision,
) (ports.RegistrySaveOutcome, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.writes++
	key := zoneRegistryKey(revision.Tenant(), revision.Warehouse(), revision.Code(), revision.Revision())
	if existing, ok := registry.revisions[key]; ok {
		if existing.SameContentAs(revision) {
			return ports.RegistryRevisionAlreadyRegistered, nil
		}
		return ports.RegistryRevisionContentConflict, nil
	}
	registry.revisions[key] = revision
	return ports.RegistryRevisionSaved, nil
}

func (registry *memoryZoneRegistry) touched() int {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	return registry.reads + registry.writes
}

// memoryWarehouseLookup 是「仓已登记」的内存替身：按（租户，仓编码）记在册的仓。reads 数问过几次，坏载荷与信封一层的用例据此证明
// 登记用例没有被走到。
type memoryWarehouseLookup struct {
	mutex      sync.Mutex
	registered map[string]bool
	reads      int
	err        error
}

func newMemoryWarehouseLookup(tenant string, warehouses ...string) *memoryWarehouseLookup {
	lookup := &memoryWarehouseLookup{registered: map[string]bool{}}
	for _, code := range warehouses {
		lookup.registered[tenant+"\x00"+code] = true
	}
	return lookup
}

func (lookup *memoryWarehouseLookup) LoadLatestWarehouse(
	_ context.Context, tenant domain.TenantID, code domain.WarehouseCode,
) (domain.WarehouseRevision, bool, error) {
	lookup.mutex.Lock()
	defer lookup.mutex.Unlock()
	lookup.reads++
	if lookup.err != nil {
		return domain.WarehouseRevision{}, false, lookup.err
	}
	return domain.WarehouseRevision{}, lookup.registered[tenant.String()+"\x00"+code.String()], nil
}

// zoneRegisterFixture 把库区登记口、它的认证、库区册、仓册与时钟装在一起：租户 SYN-TENANT-01 的仓 SYN-WH-01 已登记。
type zoneRegisterFixture struct {
	handler       http.Handler
	authenticator *fakeRegistryAuthenticator
	zones         *memoryZoneRegistry
	warehouses    *memoryWarehouseLookup
	now           *time.Time
}

func newZoneRegisterFixture(t *testing.T) *zoneRegisterFixture {
	t.Helper()
	now := firstInstant
	fixture := &zoneRegisterFixture{
		authenticator: &fakeRegistryAuthenticator{identity: operatorIdentity(t, tenantA, operatorOne)},
		zones:         newMemoryZoneRegistry(),
		warehouses:    newMemoryWarehouseLookup(tenantA, "SYN-WH-01"),
		now:           &now,
	}
	intake, err := warehousehttp.NewOperatorRegistryIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.handler = warehousehttp.NewRegisterZoneEndpoint(
		intake, application.NewRegisterZoneHandler(fixture.zones, fixture.warehouses, fixedClock{at: fixture.now}))
	return fixture
}

func (fixture *zoneRegisterFixture) post(body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, zoneRegistrationsPath, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	return response
}

// touched 数库区册与仓册一共被碰过几次。
func (fixture *zoneRegisterFixture) touched() int {
	fixture.warehouses.mutex.Lock()
	defer fixture.warehouses.mutex.Unlock()
	return fixture.zones.touched() + fixture.warehouses.reads
}

// zoneJSON 拼一份库区登记报文：字段缺省值是一个合法的存储区，用例只改它关心的格。
type zoneJSON struct {
	warehouse, code, name, usage string
	revision                     int
	deactivated                  bool
}

func syntheticZone(code string, revision int) zoneJSON {
	return zoneJSON{warehouse: "SYN-WH-01", code: code, revision: revision, name: "合成存储区", usage: "STORAGE"}
}

func (zone zoneJSON) String() string {
	raw, err := json.Marshal(map[string]any{
		"warehouseCode": zone.warehouse,
		"zoneCode":      zone.code,
		"revision":      zone.revision,
		"name":          zone.name,
		"usage":         zone.usage,
		"deactivated":   zone.deactivated,
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// zoneRow 造读面的一行。
func syntheticZoneRow(warehouse, code string, revision int, deactivated bool) ports.ZoneRow {
	return ports.ZoneRow{
		WarehouseCode: warehouse, ZoneCode: code, Revision: revision, Name: "合成存储区", Usage: "STORAGE",
		Deactivated: deactivated, Registrant: operatorOne, RegisteredAt: firstInstant,
	}
}

// fakeZoneCatalogue 同时是库区列表与修订史的读口。current 是各租户当前修订，history 按（租户，仓编码，库区编码）给修订史；
// fail 非空即交回它。
type fakeZoneCatalogue struct {
	current map[string][]ports.ZoneRow
	history map[string][]ports.ZoneRow
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

func (catalogue *fakeZoneCatalogue) ListZones(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.ZoneRow], error) {
	catalogue.listed++
	catalogue.gotTenant, catalogue.gotLimit, catalogue.gotQuery = tenant.String(), limit, query
	if catalogue.fail != nil {
		return cataloguepage.Result[ports.ZoneRow]{}, catalogue.fail
	}
	rows := catalogue.current[tenant.String()]
	return cataloguepage.Result[ports.ZoneRow]{Rows: rows, Total: int64(len(rows)), Next: catalogue.next}, nil
}

func (catalogue *fakeZoneCatalogue) ZoneRevisions(
	_ context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ZoneCode,
) ([]ports.ZoneRow, error) {
	catalogue.revisions++
	catalogue.gotRevTenant, catalogue.gotRevWH, catalogue.gotRevCode = tenant.String(), warehouse.String(), code.String()
	if catalogue.fail != nil {
		return nil, catalogue.fail
	}
	return catalogue.history[tenant.String()+"\x00"+warehouse.String()+"\x00"+code.String()], nil
}

// zoneReadFixture 把库区各读口、它们的认证与假读面装在一起。
type zoneReadFixture struct {
	list          http.Handler
	revisionsLine http.Handler
	authenticator *fakeReadAuthenticator
	catalogue     *fakeZoneCatalogue
}

func newZoneReadFixture(t *testing.T, catalogue *fakeZoneCatalogue) *zoneReadFixture {
	t.Helper()
	fixture := &zoneReadFixture{
		authenticator: &fakeReadAuthenticator{tenant: valueOf(t, domain.NewTenantID, tenantA)},
		catalogue:     catalogue,
	}
	intake, err := warehousehttp.NewOperatorOperationsReadIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.list = warehousehttp.NewQueryZonesEndpoint(intake, catalogue)
	fixture.revisionsLine = warehousehttp.NewQueryZoneRevisionsEndpoint(intake, catalogue)
	return fixture
}
