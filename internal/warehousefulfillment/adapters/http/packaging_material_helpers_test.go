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

// 本包包材的用例同样不接真库：登记走真的 application.RegisterPackagingMaterialHandler 加内存包材册与内存「本租户有没有仓」（后者与库位类型那一册共用，
// 见 location_type_helpers_test.go），所以各种结果都是用例自己算出来的，不是测试替身编出来的；读口与认证用假的（认证的假件与仓那一册共用，见
// helpers_test.go）。

// memoryPackagingMaterialRegistry 是内存里的包材登记册：键是（租户，包材编码，修订号），撞键同内容答已在册、异内容答冲突。reads 与 writes 数登记用例碰过册
// 几次，信封一层与坏载荷的用例据此证明「不读册、不写册」。
type memoryPackagingMaterialRegistry struct {
	mutex     sync.Mutex
	revisions map[string]domain.PackagingMaterialRevision
	reads     int
	writes    int
	loadErr   error
}

func newMemoryPackagingMaterialRegistry() *memoryPackagingMaterialRegistry {
	return &memoryPackagingMaterialRegistry{revisions: map[string]domain.PackagingMaterialRevision{}}
}

func packagingMaterialRegistryKey(tenant domain.TenantID, code domain.PackagingMaterialCode, revision int) string {
	return tenant.String() + "\x00" + code.String() + "\x00" + strconv.Itoa(revision)
}

func (registry *memoryPackagingMaterialRegistry) LoadLatestPackagingMaterial(
	_ context.Context, tenant domain.TenantID, code domain.PackagingMaterialCode,
) (domain.PackagingMaterialRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.PackagingMaterialRevision{}, false, registry.loadErr
	}
	for revision := 9; revision >= 1; revision-- {
		if found, ok := registry.revisions[packagingMaterialRegistryKey(tenant, code, revision)]; ok {
			return found, true, nil
		}
	}
	return domain.PackagingMaterialRevision{}, false, nil
}

func (registry *memoryPackagingMaterialRegistry) LoadPackagingMaterialRevision(
	_ context.Context, tenant domain.TenantID, code domain.PackagingMaterialCode, revision int,
) (domain.PackagingMaterialRevision, bool, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.reads++
	if registry.loadErr != nil {
		return domain.PackagingMaterialRevision{}, false, registry.loadErr
	}
	found, ok := registry.revisions[packagingMaterialRegistryKey(tenant, code, revision)]
	return found, ok, nil
}

func (registry *memoryPackagingMaterialRegistry) SavePackagingMaterialRevision(
	_ context.Context, revision domain.PackagingMaterialRevision,
) (ports.RegistrySaveOutcome, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.writes++
	key := packagingMaterialRegistryKey(revision.Tenant(), revision.Code(), revision.Revision())
	if existing, ok := registry.revisions[key]; ok {
		if existing.SameContentAs(revision) {
			return ports.RegistryRevisionAlreadyRegistered, nil
		}
		return ports.RegistryRevisionContentConflict, nil
	}
	registry.revisions[key] = revision
	return ports.RegistryRevisionSaved, nil
}

func (registry *memoryPackagingMaterialRegistry) touched() int {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	return registry.reads + registry.writes
}

// packagingMaterialRegisterFixture 把包材登记口、它的认证、包材册、「有没有仓」与时钟装在一起：租户 SYN-TENANT-01 已登记过仓。
type packagingMaterialRegisterFixture struct {
	handler            http.Handler
	authenticator      *fakeRegistryAuthenticator
	packagingMaterials *memoryPackagingMaterialRegistry
	warehouses         *memoryTenantWarehouses
	now                *time.Time
}

func newPackagingMaterialRegisterFixture(t *testing.T) *packagingMaterialRegisterFixture {
	t.Helper()
	now := firstInstant
	fixture := &packagingMaterialRegisterFixture{
		authenticator:      &fakeRegistryAuthenticator{identity: operatorIdentity(t, tenantA, operatorOne)},
		packagingMaterials: newMemoryPackagingMaterialRegistry(),
		warehouses:         newMemoryTenantWarehouses(tenantA),
		now:                &now,
	}
	intake, err := warehousehttp.NewOperatorRegistryIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.handler = warehousehttp.NewRegisterPackagingMaterialEndpoint(
		intake, application.NewRegisterPackagingMaterialHandler(fixture.packagingMaterials, fixture.warehouses, fixedClock{at: fixture.now}))
	return fixture
}

func (fixture *packagingMaterialRegisterFixture) post(body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, packagingMaterialRegistrationsPath, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	return response
}

// touched 数包材册与「有没有仓」一共被碰过几次。
func (fixture *packagingMaterialRegisterFixture) touched() int {
	fixture.warehouses.mutex.Lock()
	defer fixture.warehouses.mutex.Unlock()
	return fixture.packagingMaterials.touched() + fixture.warehouses.reads
}

// packagingMaterialJSON 拼一份包材登记报文：字段缺省值是一个合法的纸箱，用例只改它关心的格；数值与单位都以 JSON 字符串给出。
type packagingMaterialJSON struct {
	code, name                    string
	revision                      int
	length, width, height, weight string
	dimensionsUnit, weightUnit    string
	deactivated                   bool
}

func syntheticPackagingMaterial(code string, revision int) packagingMaterialJSON {
	return packagingMaterialJSON{
		code: code, revision: revision, name: "合成一号纸箱",
		length: "30", width: "20", height: "15.5", dimensionsUnit: "CM", weight: "0.25", weightUnit: "KG",
	}
}

func (material packagingMaterialJSON) String() string {
	raw, err := json.Marshal(map[string]any{
		"packagingMaterialCode": material.code,
		"revision":              material.revision,
		"name":                  material.name,
		"dimensions": map[string]string{
			"length": material.length, "width": material.width, "height": material.height, "unit": material.dimensionsUnit,
		},
		"weight":      map[string]string{"value": material.weight, "unit": material.weightUnit},
		"deactivated": material.deactivated,
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// syntheticPackagingMaterialRow 造读面的一行。
func syntheticPackagingMaterialRow(code string, revision int, deactivated bool) ports.PackagingMaterialRow {
	return ports.PackagingMaterialRow{
		PackagingMaterialCode: code, Revision: revision, Name: "合成一号纸箱",
		Length: "30", Width: "20", Height: "15.5", DimensionsUnit: "CM", Weight: "0.25", WeightUnit: "KG",
		Deactivated: deactivated, Registrant: operatorOne, RegisteredAt: firstInstant,
	}
}

// fakePackagingMaterialCatalogue 同时是包材列表与修订史的读口。current 是各租户当前修订，history 按（租户，包材编码）给修订史；fail 非空即交回它。
type fakePackagingMaterialCatalogue struct {
	current map[string][]ports.PackagingMaterialRow
	history map[string][]ports.PackagingMaterialRow
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

func (catalogue *fakePackagingMaterialCatalogue) ListPackagingMaterials(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.PackagingMaterialRow], error) {
	catalogue.listed++
	catalogue.gotTenant, catalogue.gotLimit, catalogue.gotQuery = tenant.String(), limit, query
	if catalogue.fail != nil {
		return cataloguepage.Result[ports.PackagingMaterialRow]{}, catalogue.fail
	}
	rows := catalogue.current[tenant.String()]
	return cataloguepage.Result[ports.PackagingMaterialRow]{Rows: rows, Total: int64(len(rows)), Next: catalogue.next}, nil
}

func (catalogue *fakePackagingMaterialCatalogue) PackagingMaterialRevisions(
	_ context.Context, tenant domain.TenantID, code domain.PackagingMaterialCode,
) ([]ports.PackagingMaterialRow, error) {
	catalogue.revisions++
	catalogue.gotRevTenant, catalogue.gotRevCode = tenant.String(), code.String()
	if catalogue.fail != nil {
		return nil, catalogue.fail
	}
	return catalogue.history[tenant.String()+"\x00"+code.String()], nil
}

// packagingMaterialReadFixture 把包材各读口、它们的认证与假读面装在一起。
type packagingMaterialReadFixture struct {
	list          http.Handler
	revisionsLine http.Handler
	authenticator *fakeReadAuthenticator
	catalogue     *fakePackagingMaterialCatalogue
}

func newPackagingMaterialReadFixture(t *testing.T, catalogue *fakePackagingMaterialCatalogue) *packagingMaterialReadFixture {
	t.Helper()
	fixture := &packagingMaterialReadFixture{
		authenticator: &fakeReadAuthenticator{tenant: valueOf(t, domain.NewTenantID, tenantA)},
		catalogue:     catalogue,
	}
	intake, err := warehousehttp.NewOperatorOperationsReadIntake(fixture.authenticator)
	if err != nil {
		t.Fatal(err)
	}
	fixture.list = warehousehttp.NewQueryPackagingMaterialsEndpoint(intake, catalogue)
	fixture.revisionsLine = warehousehttp.NewQueryPackagingMaterialRevisionsEndpoint(intake, catalogue)
	return fixture
}
