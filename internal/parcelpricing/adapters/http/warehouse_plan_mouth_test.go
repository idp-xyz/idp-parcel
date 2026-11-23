package pricinghttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// warehousePlanCard 是登记口接受的一张卡：入库按件，仓租按库龄分成免收与按件两档。
const warehousePlanCard = `{
  "id": "SYN-PLAN-WH-01",
  "version": "1",
  "scope": "SYN-SCOPE-WH",
  "currency": "CNY",
  "declaredValue": "REFUSE",
  "effectiveFrom": "2026-11-01T00:00:00Z",
  "groups": [
    {
      "name": "INBOUND",
      "kind": "INBOUND_HANDLING",
      "level": "OCCURRENCE",
      "onMiss": "PENDING"
    },
    {
      "name": "STORAGE",
      "kind": "STORAGE",
      "level": "LINE",
      "line": "INBOUND_BATCH",
      "onMiss": "PENDING"
    }
  ],
  "rules": [
    {
      "code": "INBOUND_PIECE",
      "kind": "INBOUND_HANDLING",
      "level": "OCCURRENCE",
      "group": "INBOUND",
      "priority": 1,
      "method": "CHARGE",
      "unit": "PIECE",
      "unitQuantity": "1",
      "unitPriceMinor": 100,
      "rounding": "EXACT"
    },
    {
      "code": "STORAGE_FREE",
      "kind": "STORAGE",
      "level": "LINE",
      "line": "INBOUND_BATCH",
      "group": "STORAGE",
      "priority": 1,
      "method": "WAIVE",
      "when": [
        {"measureUnit": "DAY", "operator": "LT", "threshold": "30"}
      ]
    },
    {
      "code": "STORAGE_PIECE",
      "kind": "STORAGE",
      "level": "LINE",
      "line": "INBOUND_BATCH",
      "group": "STORAGE",
      "priority": 1,
      "method": "CHARGE",
      "unit": "PIECE",
      "unitQuantity": "1",
      "unitPriceMinor": 50,
      "rounding": "EXACT",
      "when": [
        {"measureUnit": "DAY", "operator": "GE", "threshold": "30"}
      ]
    }
  ]
}`

func TestWarehousePlanMouthKeepsAnAgeTier(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	registrar := &warehousePlanRegistrarDouble{}
	endpoint := pricinghttp.NewRegisterWarehousePlanEndpoint(intake, registrar)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, warehousePlanRequest(warehousePlanCard))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("登记 %d %s", recorder.Code, recorder.Body.String())
	}
	if body := decodeBody(t, recorder); body["outcome"] != "RECORDED" {
		t.Fatalf("outcome %v", body["outcome"])
	}
	if !registrar.called || registrar.tenant != "SYN-TENANT-01" {
		t.Fatalf("租户 %q called %v", registrar.tenant, registrar.called)
	}
	if _, issue := domain.NewWarehousePlan(registrar.plan); issue != "" {
		t.Fatalf("解码后的卡 %s", issue)
	}
	free := warehouseRule(t, registrar.plan, "STORAGE_FREE")
	if free.Method != "WAIVE" || len(free.When) != 1 || free.When[0].MeasureUnit != "DAY" || free.When[0].Operator != "LT" || free.When[0].Threshold != "30" {
		t.Fatalf("免收档 %+v", free)
	}
	charged := warehouseRule(t, registrar.plan, "STORAGE_PIECE")
	if charged.Unit != "PIECE" || charged.UnitPrice != 50 || len(charged.When) != 1 || charged.When[0].Operator != "GE" || charged.When[0].Threshold != "30" {
		t.Fatalf("计收档 %+v", charged)
	}
	inbound := warehouseRule(t, registrar.plan, "INBOUND_PIECE")
	if inbound.Kind != "INBOUND_HANDLING" || len(inbound.When) != 0 {
		t.Fatalf("入库 %+v", inbound)
	}

	smuggled := &warehousePlanRegistrarDouble{}
	refused := httptest.NewRecorder()
	pricinghttp.NewRegisterWarehousePlanEndpoint(intake, smuggled).ServeHTTP(refused, warehousePlanRequest(
		strings.Replace(warehousePlanCard, "{", `{"tenantId": "SYN-TENANT-02",`, 1),
	))
	if refused.Code != http.StatusBadRequest || errorCode(t, refused) != "MALFORMED_REQUEST" || smuggled.called {
		t.Fatalf("自报租户 %d %s called %v", refused.Code, refused.Body.String(), smuggled.called)
	}
}

func TestWarehousePlanMouthKeepsBothConditionShapesAndMeasureClass(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	registrar := &warehousePlanRegistrarDouble{}
	recorder := httptest.NewRecorder()
	pricinghttp.NewRegisterWarehousePlanEndpoint(intake, registrar).ServeHTTP(recorder, warehousePlanRequest(`{
	  "id": "SYN-PLAN-WH-02",
	  "version": "1",
	  "scope": "SYN-SCOPE-WH",
	  "currency": "CNY",
	  "declaredValue": "USE",
	  "effectiveFrom": "2026-11-01T00:00:00Z",
	  "groups": [{
	    "name": "STORAGE",
	    "kind": "STORAGE",
	    "level": "LINE",
	    "line": "INBOUND_BATCH",
	    "onMiss": "NOT_CHARGED"
	  }],
	  "rules": [{
	    "code": "STORAGE_M3",
	    "kind": "STORAGE",
	    "level": "LINE",
	    "line": "INBOUND_BATCH",
	    "group": "STORAGE",
	    "priority": 1,
	    "method": "CHARGE",
	    "measureClass": "VOLUME",
	    "unit": "M3",
	    "unitQuantity": "1",
	    "unitPriceMinor": 100,
	    "baseFeeMinor": 20,
	    "allowance": "0.5",
	    "rounding": "EXACT",
	    "when": [
	      {"name": "FROZEN", "equals": "无"},
	      {"measureClass": "WEIGHT", "measureUnit": "KG", "operator": "LE", "threshold": "1.5"}
	    ]
	  }]
	}`))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("登记 %d %s", recorder.Code, recorder.Body.String())
	}
	rule := warehouseRule(t, registrar.plan, "STORAGE_M3")
	if rule.MeasureClass != "VOLUME" || rule.Unit != "M3" || rule.BaseFee != 20 || rule.Allowance != "0.5" || len(rule.When) != 2 {
		t.Fatalf("规则 %+v", rule)
	}
	if rule.When[0].Name != "FROZEN" || rule.When[0].Equals != "无" || rule.When[0].MeasureUnit != "" {
		t.Fatalf("属性条件 %+v", rule.When[0])
	}
	if rule.When[1].MeasureClass != "WEIGHT" || rule.When[1].MeasureUnit != "KG" || rule.When[1].Operator != "LE" || rule.When[1].Threshold != "1.5" {
		t.Fatalf("重量条件 %+v", rule.When[1])
	}
	if _, issue := domain.NewWarehousePlan(registrar.plan); issue != "" {
		t.Fatalf("解码后的卡 %s", issue)
	}
}

func warehousePlanRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/pricing-warehouse-plans", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer t")
	return request
}

func warehouseRule(t *testing.T, plan domain.WarehousePlan, code string) domain.WarehouseChargeRule {
	t.Helper()
	for _, rule := range plan.Rules {
		if rule.Code == code {
			return rule
		}
	}
	t.Fatalf("没有规则 %s", code)
	return domain.WarehouseChargeRule{}
}

type warehousePlanRegistrarDouble struct {
	tenant string
	plan   domain.WarehousePlan
	called bool
}

func (double *warehousePlanRegistrarDouble) Register(_ context.Context, tenant string, plan domain.WarehousePlan) (application.WarehousePlanOutcome, error) {
	double.called = true
	double.tenant = tenant
	double.plan = plan
	return application.WarehousePlanRecorded, nil
}
