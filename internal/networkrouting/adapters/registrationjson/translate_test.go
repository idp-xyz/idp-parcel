package registrationjson_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// 在线口那一路（票 operator-channel/04）：租户取操作者信封给的，批文带 tenant_id 即拒；受控批量口那一路租户取批文。

func TestNetworkTranslationTakesTheTenantFromItsSource(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	online, err := registrationjson.NodeVersionFromJSONForTenant([]byte(`{"code": "SYN-NODE-A", "version": 1, "business_timezone": "Asia/Shanghai", "effective_from": "2026-09-01T00:00:00Z"}`), tenant)
	if err != nil || online.TenantID != tenant || online.Node.Code != "SYN-NODE-A" || online.Node.HasEffectiveTo {
		t.Fatalf("online node = %+v, err = %v", online, err)
	}
	batch, err := registrationjson.NodeVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-09", "code": "SYN-NODE-A", "version": 1, "business_timezone": "Asia/Shanghai", "effective_from": "2026-09-01T00:00:00Z"}`))
	if err != nil || batch.TenantID.String() != "SYN-TENANT-09" {
		t.Fatalf("batch node tenant = %q, err = %v", batch.TenantID.String(), err)
	}
}

func TestOnlineTranslationRefusesASelfReportedTenantOnEveryNetworkFamily(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	families := map[string]func([]byte) error{
		"node": func(raw []byte) error {
			_, err := registrationjson.NodeVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"connection": func(raw []byte) error {
			_, err := registrationjson.ConnectionVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"line": func(raw []byte) error {
			_, err := registrationjson.LineVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"service area": func(raw []byte) error {
			_, err := registrationjson.ServiceAreaVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"service calendar": func(raw []byte) error {
			_, err := registrationjson.ServiceCalendarVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"availability adjustment": func(raw []byte) error {
			_, err := registrationjson.AvailabilityAdjustmentFromJSONForTenant(raw, tenant)
			return err
		},
		"route strategy": func(raw []byte) error {
			_, err := registrationjson.RouteStrategyVersionFromJSONForTenant(raw, tenant)
			return err
		},
	}
	for name, translate := range families {
		for _, raw := range []string{`{"tenant_id": "SYN-TENANT-02"}`, `{"tenant_id": null}`} {
			if err := translate([]byte(raw)); !errors.Is(err, registrationjson.ErrSelfReportedTenant) {
				t.Fatalf("%s with %s: err = %v, want ErrSelfReportedTenant", name, raw, err)
			}
		}
	}
}

// basisRows 是稳定定义各族一行齐全的直接登记行（受控批量口那一路），`%s` 处放依据那一格。
var basisRows = map[string]string{
	"node":             `{"tenant_id": "SYN-TENANT-01", "code": "SYN-NODE-A", "version": 1, "business_timezone": "Asia/Shanghai", "effective_from": "2026-09-01T00:00:00Z"%s}`,
	"connection":       `{"tenant_id": "SYN-TENANT-01", "code": "SYN-CONN-A-B", "version": 1, "from_node": "SYN-NODE-A", "to_node": "SYN-NODE-B", "business_timezone": "Asia/Shanghai", "effective_from": "2026-09-01T00:00:00Z"%s}`,
	"line":             `{"tenant_id": "SYN-TENANT-01", "code": "SYN-LINE-1", "version": 1, "segments": ["SYN-CONN-A-B"], "business_timezone": "Asia/Shanghai", "applicable_scope": "NETWORK_SERVICE", "effective_from": "2026-09-01T00:00:00Z"%s}`,
	"service area":     `{"tenant_id": "SYN-TENANT-01", "code": "SYN-AREA-1", "version": 1, "effective_from": "2026-09-01T00:00:00Z"%s}`,
	"service calendar": `{"tenant_id": "SYN-TENANT-01", "target_kind": "NODE", "target_code": "SYN-NODE-A", "version": 1, "effective_from": "2026-09-01T00:00:00Z"%s}`,
	"route strategy":   `{"tenant_id": "SYN-TENANT-01", "code": "SYN-RS-1", "version": 1, "applicable_scope": "NETWORK_SERVICE", "effective_from": "2026-09-01T00:00:00Z"%s}`,
}

// basisOf 按族译一行，交回译出的依据格。
func basisOf(family string, raw []byte) (domain.CatalogBasisReference, error) {
	switch family {
	case "node":
		command, err := registrationjson.NodeVersionFromJSON(raw)
		return command.Node.Basis, err
	case "connection":
		command, err := registrationjson.ConnectionVersionFromJSON(raw)
		return command.Connection.Basis, err
	case "line":
		command, err := registrationjson.LineVersionFromJSON(raw)
		return command.Line.Basis, err
	case "service area":
		command, err := registrationjson.ServiceAreaVersionFromJSON(raw)
		return command.Area.Basis, err
	case "service calendar":
		command, err := registrationjson.ServiceCalendarVersionFromJSON(raw)
		return command.Calendar.Basis, err
	case "route strategy":
		command, err := registrationjson.RouteStrategyVersionFromJSON(raw)
		return command.Strategy.Basis, err
	default:
		panic("unknown family " + family)
	}
}

func rowWith(template, basisField string) []byte {
	return []byte(strings.Replace(template, "%s", basisField, 1))
}

// Covers: 依据格在直接登记行上可选——缺席即没给依据，给了就原样到达；给了空串是写坏了，不是没给。
func TestDirectRowsCarryAnOptionalBasisOnEveryStableFamily(t *testing.T) {
	for family, template := range basisRows {
		absent, err := basisOf(family, rowWith(template, ""))
		if err != nil || absent.Present() {
			t.Fatalf("%s 缺依据：Present=%v err=%v", family, absent.Present(), err)
		}
		given, err := basisOf(family, rowWith(template, `, "basis": "SYN-NET-OPS/CHANGE-0001"`))
		if err != nil || given.String() != "SYN-NET-OPS/CHANGE-0001" {
			t.Fatalf("%s 带依据：%q err=%v", family, given.String(), err)
		}
		if _, err := basisOf(family, rowWith(template, `, "basis": ""`)); !errors.Is(err, domain.ErrBlankValue) {
			t.Fatalf("%s 依据空串：err=%v，想要 ErrBlankValue", family, err)
		}
	}
}

// Covers: ADR-0147 决定四「依据格由采用路径写成引用串，采用方不另填」——直接登记行自己写引用串即拒，哪怕那一版
// 已发布：内容不是从参考配置取的，却声称是采用。
func TestADirectRowMayNotWriteACitationIntoItsBasis(t *testing.T) {
	released := referenceconfig.Released()
	if len(released) == 0 {
		t.Fatal("发布清单为空")
	}
	citation := released[0].Citation()
	for family, template := range basisRows {
		_, err := basisOf(family, rowWith(template, `, "basis": "`+citation+`"`))
		if !errors.Is(err, registrationjson.ErrCitationOutsideAdoption) {
			t.Fatalf("%s 直接行自写引用串：err=%v，想要 ErrCitationOutsideAdoption", family, err)
		}
	}
}

// Covers: 日历三格（ADR-0175 决定一）逐格到达：没写是 nil，写了 0 是 0。
func TestACalendarRowCarriesItsContent(t *testing.T) {
	command, err := registrationjson.ServiceCalendarVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"target_kind": "NODE", "target_code": "SYN-NODE-A", "version": 1, "effective_from": "2026-09-01T00:00:00Z",
		"cutoff_local_minute": 1080, "processing_minutes": 0}`))
	if err != nil {
		t.Fatalf("译装：%v", err)
	}
	calendar := command.Calendar
	if calendar.CutoffLocalMinute == nil || *calendar.CutoffLocalMinute != 1080 {
		t.Fatalf("截单 = %v，想要 1080", calendar.CutoffLocalMinute)
	}
	if calendar.ProcessingMinutes == nil || *calendar.ProcessingMinutes != 0 {
		t.Fatalf("处理时长 = %v，登了 0 就是 0", calendar.ProcessingMinutes)
	}
	if calendar.BufferMinutes != nil {
		t.Fatalf("衔接缓冲没登却译出 %v", *calendar.BufferMinutes)
	}
}

// Covers: 线路登记行带逐段成本依据（票 routing-first-cut/10 的列，本票接进登记口）：种类按封闭两类逐格译，集外的
// 词在入库前拒。
func TestALineRowCarriesItsCostBases(t *testing.T) {
	command, err := registrationjson.LineVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"code": "SYN-LINE-1", "version": 1, "segments": ["SYN-CONN-A-B", "SYN-CONN-B-C"],
		"business_timezone": "Asia/Shanghai", "applicable_scope": "NETWORK_SERVICE", "effective_from": "2026-09-01T00:00:00Z",
		"cost_bases": [
			{"segment_index": 0, "kind": "SUPPLIER_BUY_PLAN", "reference": "SYN-PLAN-COST-1/v1"},
			{"segment_index": 1, "kind": "INTERNAL_POLICY", "reference": "SYN-POLICY-1/v1"}
		]}`))
	if err != nil {
		t.Fatalf("译装：%v", err)
	}
	want := []ports.LineSegmentCostBasis{
		{SegmentIndex: 0, Kind: ports.SupplierBuyPlanBasis, Reference: "SYN-PLAN-COST-1/v1"},
		{SegmentIndex: 1, Kind: ports.InternalPolicyBasis, Reference: "SYN-POLICY-1/v1"},
	}
	if !reflect.DeepEqual(command.CostBases, want) {
		t.Fatalf("成本依据 = %+v，想要 %+v", command.CostBases, want)
	}
	if _, err := registrationjson.LineVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"code": "SYN-LINE-1", "version": 1, "segments": ["SYN-CONN-A-B"],
		"business_timezone": "Asia/Shanghai", "applicable_scope": "NETWORK_SERVICE", "effective_from": "2026-09-01T00:00:00Z",
		"cost_bases": [{"segment_index": 0, "kind": "FREE_OF_CHARGE", "reference": "SYN-PLAN-COST-1/v1"}]}`)); err == nil {
		t.Fatal("集外的成本依据种类应在入库前拒")
	}
}
