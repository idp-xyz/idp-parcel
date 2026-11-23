package warehousehttp_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const customerWarehousesPath = "/v1/warehouse/reads/warehouses"

func getCustomerWarehouses(handler http.Handler, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Authorization", "Bearer presented")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// Covers: 货主查阅仓读本租户仓册的当前修订，不另做一套，不带登记责任方。查询串自带身份是坏查询，不进读口。
func TestCustomerWarehouseListReadsTheSameCatalogueWithoutTheRegistrant(t *testing.T) {
	bare := syntheticRow("SYN-WH-02", 3, true)
	bare.HasBusinessTimeZone, bare.BusinessTimeZone = false, ""
	bare.HasColocatedNode, bare.ColocatedNode = false, ""
	catalogue := &fakeCatalogue{current: map[string][]ports.WarehouseRow{
		tenantA: {syntheticRow("SYN-WH-01", 2, false), bare},
		tenantB: {syntheticRow("SYN-WH-OTHER", 1, false)},
	}, next: "cursor-after-SYN-WH-02"}
	intake, admission := customerIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityWarehouseRead)
	handler := warehousehttp.NewListCustomerWarehousesEndpoint(intake, catalogue)

	response := getCustomerWarehouses(handler, customerWarehousesPath)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	answer := decodeList(t, response)
	if answer.Outcome != "WAREHOUSES_LISTED" || len(answer.Warehouses) != 2 {
		t.Fatalf("答复 = %+v", answer)
	}
	if answer.Warehouses[0].WarehouseCode != "SYN-WH-01" || answer.Warehouses[0].Revision != 2 ||
		answer.Warehouses[0].BusinessTimeZone != "Asia/Shanghai" || answer.Warehouses[0].ColocatedNode != "SYN-NODE-01" {
		t.Fatalf("第一行 = %+v", answer.Warehouses[0])
	}
	if answer.Warehouses[1].WarehouseCode != "SYN-WH-02" || !answer.Warehouses[1].Deactivated {
		t.Fatalf("第二行 = %+v", answer.Warehouses[1])
	}
	var raw struct {
		Warehouses []map[string]json.RawMessage `json:"warehouses"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, present := raw.Warehouses[0]["registrant"]; present {
		t.Fatalf("货主读口带了登记责任方：%s", response.Body.String())
	}
	if _, present := raw.Warehouses[1]["businessTimeZone"]; present || raw.Warehouses[1]["colocatedNode"] != nil {
		t.Fatalf("没登记的格应整键缺席：%s", response.Body.String())
	}
	if catalogue.gotTenant != tenantA || catalogue.gotLimit != 200 || answer.Page.Size != 200 ||
		answer.Page.Total == nil || *answer.Page.Total != 2 || answer.Page.Next == nil || *answer.Page.Next != "cursor-after-SYN-WH-02" {
		t.Fatalf("读口租户 %s 页大小 %d page %+v", catalogue.gotTenant, catalogue.gotLimit, answer.Page)
	}
	if len(admission.asked) != 0 {
		t.Fatalf("读口问了准入：%+v", admission.asked)
	}

	listed := catalogue.listed
	for _, target := range []string{
		customerWarehousesPath + "?ownerAccount=SYN-OWNER-01",
		customerWarehousesPath + "?tenantId=" + tenantB,
		customerWarehousesPath + "?registrant=someone",
	} {
		refused := getCustomerWarehouses(handler, target)
		assertProblemDetails(t, target, refused, http.StatusBadRequest, "MALFORMED_REQUEST")
	}
	if catalogue.listed != listed {
		t.Fatalf("坏查询进了读口 %d 次", catalogue.listed-listed)
	}

	filtered := getCustomerWarehouses(handler, customerWarehousesPath+"?deactivated=true")
	if filtered.Code != http.StatusOK {
		t.Fatalf("筛选：%d %s", filtered.Code, filtered.Body.String())
	}
	if filter := catalogue.gotQuery.Filters["deactivated"]; len(filter) != 1 || filter[0] != "true" {
		t.Fatalf("筛选 = %v", catalogue.gotQuery.Filters)
	}
}

// Covers: 没授予、渠道未配置、读不动都不把仓册答成空。坏查询在授予与渠道这层先被挡住，不进读口。
func TestCustomerWarehouseListRefusesBeforeReadingTheCatalogue(t *testing.T) {
	catalogue := &fakeCatalogue{current: map[string][]ports.WarehouseRow{
		tenantA: {syntheticRow("SYN-WH-01", 1, false)},
	}}
	withoutGrant, _ := customerIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityGoodsMaintenance)
	refused := getCustomerWarehouses(
		warehousehttp.NewListCustomerWarehousesEndpoint(withoutGrant, catalogue),
		customerWarehousesPath+"?deactivated=maybe",
	)
	assertProblemDetails(t, "没有查阅授予", refused, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED")

	unconfigured := warehousehttp.NewListCustomerWarehousesEndpoint(warehousehttp.OpenCustomerInboundPreAdviceIntake(nil), catalogue)
	assertProblemDetails(t, "渠道未配置", getCustomerWarehouses(unconfigured, customerWarehousesPath+"?deactivated=maybe"), http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")

	down := &fakeCatalogue{fail: errors.New("read side down")}
	holding, _ := customerIntakeHolding(t, "SYN-OWNER-01", accessidentity.CustomerCapabilityWarehouseRead)
	assertProblemDetails(t, "读不动", getCustomerWarehouses(warehousehttp.NewListCustomerWarehousesEndpoint(holding, down), customerWarehousesPath), http.StatusInternalServerError, "NO_ANSWER_FORMED")

	empty := getCustomerWarehouses(warehousehttp.NewListCustomerWarehousesEndpoint(holding, &fakeCatalogue{}), customerWarehousesPath)
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"warehouses":[]`) {
		t.Fatalf("空册 = %d %s", empty.Code, empty.Body.String())
	}
	if catalogue.listed != 0 {
		t.Fatalf("拒绝仍问了读口 %d 次", catalogue.listed)
	}

	wrongMethod := httptest.NewRequest(http.MethodPost, customerWarehousesPath, nil)
	wrongMethod.Header.Set("Authorization", "Bearer presented")
	methodResponse := httptest.NewRecorder()
	warehousehttp.NewListCustomerWarehousesEndpoint(holding, catalogue).ServeHTTP(methodResponse, wrongMethod)
	assertProblemDetails(t, "方法不对", methodResponse, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	if methodResponse.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q", methodResponse.Header().Get("Allow"))
	}
}
