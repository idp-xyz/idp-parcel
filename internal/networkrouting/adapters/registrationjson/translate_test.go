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

// demoNetwork 是本票随产品发布的演示网络那一版（ADR-0147 决定二的标识写法）。
const demoNetwork = "network-routing/network-catalog/SYN-CN-SG@1"

// Covers: ADR-0147 决定四——采用行只给身份、修订号与生效时点，内容取自点名的那一版参考配置，依据格由采用路径写成
// 引用串；修订号与生效时点照采用行原样，不代填。
func TestAnAdoptRowTakesItsContentFromTheReferenceAndCitesIt(t *testing.T) {
	reference, err := referenceconfig.ParseReference(demoNetwork)
	if err != nil {
		t.Fatalf("解析：%v", err)
	}
	citation := reference.Citation()

	node, err := registrationjson.NodeVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"code": "SYN-NODE-SIN-HUB", "version": 3, "effective_from": "2026-05-01T00:00:00Z",
		"effective_to": "2026-12-01T00:00:00Z", "adopt": "` + demoNetwork + `"}`))
	if err != nil {
		t.Fatalf("采用节点：%v", err)
	}
	if node.TenantID.String() != "SYN-TENANT-01" || node.Node.Code != "SYN-NODE-SIN-HUB" || node.Node.Version != 3 ||
		node.Node.BusinessTimezone != "Asia/Singapore" || node.Node.Basis.String() != citation ||
		!node.Node.HasEffectiveTo || node.Node.EffectiveTo.Format("2006-01-02") != "2026-12-01" {
		t.Fatalf("采用节点 = %+v", node)
	}

	line, err := registrationjson.LineVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"code": "SYN-LINE-CN-SG-01", "version": 1, "effective_from": "2026-01-01T00:00:00Z", "adopt": "` + demoNetwork + `"}`))
	if err != nil {
		t.Fatalf("采用线路：%v", err)
	}
	if line.Line.ApplicableScope != "NETWORK_SERVICE" || len(line.Line.Segments) != len(line.CostBases) ||
		line.Line.Basis.String() != citation || line.Line.HasEffectiveTo {
		t.Fatalf("采用线路 = %+v", line)
	}
	for index, basis := range line.CostBases {
		if basis.SegmentIndex != index || basis.Kind != ports.SupplierBuyPlanBasis || basis.Reference != "SYN-PLAN-CN-SG-COST-01/v1" {
			t.Fatalf("线路成本依据 %d = %+v", index, basis)
		}
	}

	calendar, err := registrationjson.ServiceCalendarVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"target_kind": "NODE", "target_code": "SYN-NODE-SHA-HUB", "version": 1, "effective_from": "2026-01-01T00:00:00Z",
		"adopt": "` + demoNetwork + `"}`))
	if err != nil {
		t.Fatalf("采用日历：%v", err)
	}
	if calendar.Calendar.ProcessingMinutes == nil || calendar.Calendar.CutoffLocalMinute == nil ||
		calendar.Calendar.Basis.String() != citation {
		t.Fatalf("采用日历 = %+v", calendar.Calendar)
	}

	strategy, err := registrationjson.RouteStrategyVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"code": "SYN-RS-CN-SG-01", "version": 1, "effective_from": "2026-01-01T00:00:00Z", "adopt": "` + demoNetwork + `"}`))
	if err != nil {
		t.Fatalf("采用路由策略：%v", err)
	}
	if strategy.Strategy.RankingForm != domain.CostSingleDimensionRanking || strategy.Strategy.ApplicableScope != "NETWORK_SERVICE" ||
		strategy.Strategy.Basis.String() != citation {
		t.Fatalf("采用路由策略 = %+v", strategy.Strategy)
	}

	area, err := registrationjson.ServiceAreaVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"code": "SYN-AREA-SG", "version": 1, "effective_from": "2026-01-01T00:00:00Z", "adopt": "` + demoNetwork + `"}`))
	if err != nil {
		t.Fatalf("采用服务区域：%v", err)
	}
	if !area.Area.HasCoverage || area.Area.CoverageCountry != "SG" || len(area.Area.DestinationNodes) != 1 ||
		area.Area.Basis.String() != citation {
		t.Fatalf("采用服务区域 = %+v", area.Area)
	}

	connection, err := registrationjson.ConnectionVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"code": "SYN-CONN-SZX-SIN", "version": 1, "effective_from": "2026-01-01T00:00:00Z", "adopt": "` + demoNetwork + `"}`))
	if err != nil {
		t.Fatalf("采用连接：%v", err)
	}
	if connection.Connection.FromNode != "SYN-NODE-SZX-GATE" || connection.Connection.ToNode != "SYN-NODE-SIN-HUB" ||
		connection.Connection.Basis.String() != citation {
		t.Fatalf("采用连接 = %+v", connection.Connection)
	}
}

// Covers: 采用行写了内容或依据即拒（那几格由参考配置与采用路径给）；点名的版本没发布、不是网络目录的参考配置、或
// 里面没有这个身份，都在触库前拒——没有「最接近的一版」可顶替。
func TestAnAdoptRowRefusesContentOfItsOwnAndReferencesThatDoNotResolve(t *testing.T) {
	adopt := func(code, adopt, extra string) error {
		_, err := registrationjson.NodeVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01", "code": "` + code +
			`", "version": 1, "effective_from": "2026-01-01T00:00:00Z", "adopt": "` + adopt + `"` + extra + `}`))
		return err
	}
	for _, extra := range []string{`, "business_timezone": "Asia/Tokyo"`, `, "basis": "SYN-NET-OPS/CHANGE-0001"`} {
		if err := adopt("SYN-NODE-SHA-HUB", demoNetwork, extra); !errors.Is(err, registrationjson.ErrAdoptedContentGiven) {
			t.Fatalf("采用行另写 %s：err=%v，想要 ErrAdoptedContentGiven", extra, err)
		}
	}
	if err := adopt("SYN-NODE-SHA-HUB", "network-routing/network-catalog/SYN-CN-SG@99", ""); !errors.Is(err, referenceconfig.ErrNotReleased) {
		t.Fatalf("未发布版本：err=%v，想要 ErrNotReleased", err)
	}
	if err := adopt("SYN-NODE-SHA-HUB", "network-catalog/SYN-CN-SG@1", ""); !errors.Is(err, referenceconfig.ErrInvalidReference) {
		t.Fatalf("坏形状：err=%v，想要 ErrInvalidReference", err)
	}
	if err := adopt("SYN-NODE-SHA-HUB", "party-commercial/registration-number-types/CN@1", ""); !errors.Is(err, registrationjson.ErrNotNetworkCatalogReference) {
		t.Fatalf("别的目录的参考配置：err=%v，想要 ErrNotNetworkCatalogReference", err)
	}
	if err := adopt("SYN-NODE-NOT-THERE", demoNetwork, ""); !errors.Is(err, registrationjson.ErrNotInReference) {
		t.Fatalf("参考配置里没有的身份：err=%v，想要 ErrNotInReference", err)
	}
	if _, err := registrationjson.ServiceCalendarVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"target_kind": "LINE", "target_code": "SYN-LINE-CN-SG-01", "version": 1, "effective_from": "2026-01-01T00:00:00Z",
		"adopt": "` + demoNetwork + `"}`)); !errors.Is(err, registrationjson.ErrNotInReference) {
		t.Fatalf("参考配置里没有的日历：err=%v，想要 ErrNotInReference", err)
	}
}

// Covers: 在线口那一路同样能采用，租户取操作者信封；采用行带 tenant_id 照旧拒。
func TestOnlineAdoptionTakesTheTenantFromTheEnvelope(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	command, err := registrationjson.NodeVersionFromJSONForTenant([]byte(`{"code": "SYN-NODE-SHA-HUB", "version": 1,
		"effective_from": "2026-01-01T00:00:00Z", "adopt": "`+demoNetwork+`"}`), tenant)
	if err != nil || command.TenantID != tenant || command.Node.BusinessTimezone != "Asia/Shanghai" {
		t.Fatalf("在线口采用：%+v err=%v", command, err)
	}
	if _, err := registrationjson.NodeVersionFromJSONForTenant([]byte(`{"tenant_id": "SYN-TENANT-02", "code": "SYN-NODE-SHA-HUB",
		"version": 1, "effective_from": "2026-01-01T00:00:00Z", "adopt": "`+demoNetwork+`"}`), tenant); !errors.Is(err, registrationjson.ErrSelfReportedTenant) {
		t.Fatalf("在线口采用行自报租户：err=%v", err)
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
