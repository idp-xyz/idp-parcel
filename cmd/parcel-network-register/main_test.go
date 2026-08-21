package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件证受控登记口的译装与退出码翻译：族路由、未知字段与未知族在入库前拒、受理门
// 拒绝译成退出码 1、未决译成退出码 3。替身立在 ports 写入口上，译装与受理门走的都是
// 真路径；写入口自身的版本纪律由适配器真库用例证。验证值一律 SYN- 合成（S 级只记 S）。
//
// 事务替身只透传闭包：事务纪律本身由适配器真库用例证（无事务登记被拒），登记口的
// 职责只是把用例包进一个事务。

type passthroughTransactor struct{}

func (passthroughTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	return fn(ctx)
}

// registryDouble 是 ports 写入口的替身，记谁被调了、带什么行到达。
type registryDouble struct {
	err error

	lastTenant domain.TenantID
	node       *ports.NodeDefinitionVersion
	connection *ports.ConnectionDefinitionVersion
	line       *ports.LineDefinitionVersion
	area       *ports.ServiceAreaDefinitionVersion
	calendar   *ports.ServiceCalendarDefinitionVersion
	strategy   *ports.RouteStrategyDefinitionVersion
	adjustment *ports.AvailabilityAdjustmentStatement
}

func (double *registryDouble) calls() int {
	count := 0
	for _, present := range []bool{
		double.node != nil, double.connection != nil, double.line != nil,
		double.area != nil, double.calendar != nil, double.strategy != nil,
		double.adjustment != nil,
	} {
		if present {
			count++
		}
	}
	return count
}

func (double *registryDouble) RegisterNodeVersion(
	_ context.Context, tenant domain.TenantID, row ports.NodeDefinitionVersion,
) error {
	double.lastTenant, double.node = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterConnectionVersion(
	_ context.Context, tenant domain.TenantID, row ports.ConnectionDefinitionVersion,
) error {
	double.lastTenant, double.connection = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterLineVersion(
	_ context.Context, tenant domain.TenantID, row ports.LineDefinitionVersion,
) error {
	double.lastTenant, double.line = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterServiceAreaVersion(
	_ context.Context, tenant domain.TenantID, row ports.ServiceAreaDefinitionVersion,
) error {
	double.lastTenant, double.area = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterServiceCalendarVersion(
	_ context.Context, tenant domain.TenantID, row ports.ServiceCalendarDefinitionVersion,
) error {
	double.lastTenant, double.calendar = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterRouteStrategyVersion(
	_ context.Context, tenant domain.TenantID, row ports.RouteStrategyDefinitionVersion,
) error {
	double.lastTenant, double.strategy = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterAvailabilityAdjustment(
	_ context.Context, tenant domain.TenantID, row ports.AvailabilityAdjustmentStatement,
) error {
	double.lastTenant, double.adjustment = tenant, &row
	return double.err
}

func registrarOver(t *testing.T, double *registryDouble) *application.NetworkCatalogRegistration {
	t.Helper()
	registrar, err := application.NewNetworkCatalogRegistration(double)
	if err != nil {
		t.Fatalf("构造登记用例：%v", err)
	}
	return registrar
}

// Covers: 绿路径与可选终点的译装——未闭区间不带 effective_to（Has 位为假），补历史
// 带 effective_to（Has 位为真且值原样）；行与租户逐字段到达写入口，`已登记`译成 0。
func TestExecuteRegistersANodeAndTranslatesTheOptionalEnd(t *testing.T) {
	open := []byte(`{
		"tenant_id": "SYN-TENANT-1", "code": "SYN-NODE-HUB", "version": 1,
		"business_timezone": "Asia/Shanghai", "effective_from": "2026-08-20T12:00:00Z"
	}`)
	double := &registryDouble{}
	message, code := execute(t.Context(), kindNode, open, registrarOver(t, double), passthroughTransactor{})
	if code != exitRegistered || !strings.Contains(message, "REGISTERED") {
		t.Fatalf("message=%q code=%d, 想要 REGISTERED/0", message, code)
	}
	if double.node == nil || double.calls() != 1 {
		t.Fatalf("节点行没有到达写入口：%+v", double)
	}
	if double.node.Code != "SYN-NODE-HUB" || double.node.Version != 1 ||
		double.node.BusinessTimezone != "Asia/Shanghai" ||
		!double.node.EffectiveFrom.Equal(time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("行没有原样译装：%+v", double.node)
	}
	if double.node.HasEffectiveTo {
		t.Fatal("没给 effective_to 却译出了终点")
	}
	if double.lastTenant.String() != "SYN-TENANT-1" {
		t.Fatalf("租户没有原样到达：%q", double.lastTenant.String())
	}

	closed := []byte(`{
		"tenant_id": "SYN-TENANT-1", "code": "SYN-NODE-HUB", "version": 1,
		"business_timezone": "Asia/Shanghai",
		"effective_from": "2026-08-20T12:00:00Z", "effective_to": "2026-08-21T12:00:00Z"
	}`)
	double = &registryDouble{}
	if _, code := execute(t.Context(), kindNode, closed, registrarOver(t, double), passthroughTransactor{}); code != exitRegistered {
		t.Fatalf("补历史闭区间应过门，code = %d", code)
	}
	if double.node == nil || !double.node.HasEffectiveTo ||
		!double.node.EffectiveTo.Equal(time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("闭区间终点没有原样译装：%+v", double.node)
	}
}

// Covers: 族路由封闭七格——每族落到自己的写方法，封闭枚举逐格译（调整族的
// target_kind 与 kind 都是词到格）。
func TestExecuteRoutesEachKindToItsRegisterMethod(t *testing.T) {
	payloads := map[string]string{
		kindNode: `{"tenant_id":"SYN-TENANT-1","code":"SYN-NODE-A","version":1,
			"business_timezone":"Asia/Shanghai","effective_from":"2026-08-20T12:00:00Z"}`,
		kindConnection: `{"tenant_id":"SYN-TENANT-1","code":"SYN-CONN-A-B","version":1,
			"from_node":"SYN-NODE-A","to_node":"SYN-NODE-B",
			"business_timezone":"Asia/Shanghai","effective_from":"2026-08-20T12:00:00Z"}`,
		kindLine: `{"tenant_id":"SYN-TENANT-1","code":"SYN-LINE-EU","version":1,
			"segments":["SYN-CONN-A-B"],"business_timezone":"Asia/Shanghai",
			"applicable_scope":"SYN-SCOPE-DECLARED","effective_from":"2026-08-20T12:00:00Z"}`,
		kindServiceArea: `{"tenant_id":"SYN-TENANT-1","code":"SYN-AREA-DE","version":1,
			"effective_from":"2026-08-20T12:00:00Z"}`,
		kindServiceCalendar: `{"tenant_id":"SYN-TENANT-1","target_kind":"NODE",
			"target_code":"SYN-NODE-A","version":1,"effective_from":"2026-08-20T12:00:00Z"}`,
		kindAvailabilityAdjustment: `{"tenant_id":"SYN-TENANT-1","code":"SYN-ADJ-1","version":1,
			"target_kind":"LINE","target_code":"SYN-LINE-EU","kind":"SUSPENSION",
			"source":"SYN-NET-OPS/EVT-1","effective_at":"2026-08-20T12:00:00Z"}`,
		kindRouteStrategy: `{"tenant_id":"SYN-TENANT-1","code":"SYN-STRATEGY-1","version":1,
			"applicable_scope":"SYN-SCOPE-DECLARED","effective_from":"2026-08-20T12:00:00Z"}`,
	}
	arrived := map[string]func(double *registryDouble) bool{
		kindNode:            func(d *registryDouble) bool { return d.node != nil },
		kindConnection:      func(d *registryDouble) bool { return d.connection != nil },
		kindLine:            func(d *registryDouble) bool { return d.line != nil },
		kindServiceArea:     func(d *registryDouble) bool { return d.area != nil },
		kindServiceCalendar: func(d *registryDouble) bool { return d.calendar != nil },
		kindAvailabilityAdjustment: func(d *registryDouble) bool {
			return d.adjustment != nil &&
				d.adjustment.TargetKind == ports.TargetLine &&
				d.adjustment.Kind == ports.AdjustmentSuspension
		},
		kindRouteStrategy: func(d *registryDouble) bool { return d.strategy != nil },
	}

	for kind, payload := range payloads {
		double := &registryDouble{}
		message, code := execute(t.Context(), kind, []byte(payload), registrarOver(t, double), passthroughTransactor{})
		if code != exitRegistered {
			t.Fatalf("%s: message=%q code=%d, 想要 0", kind, message, code)
		}
		if double.calls() != 1 || !arrived[kind](double) {
			t.Fatalf("%s: 路由错位：%+v", kind, double)
		}
	}
}

// Covers: 译装门在入库前拒——未知族、未知字段、封闭枚举外的词、空白租户都不到达
// 写入口，全部译成退出码 1。
func TestExecuteRefusesAtTheTranslationGate(t *testing.T) {
	cases := map[string]struct {
		kind    string
		payload string
	}{
		"未知族": {"moon-route", `{}`},
		"未知字段": {kindNode, `{"tenant_id":"SYN-TENANT-1","code":"SYN-NODE-A","version":1,
			"business_timezone":"Asia/Shanghai","effective_from":"2026-08-20T12:00:00Z",
			"timezone":"Asia/Shanghai"}`},
		"适用对象类别不在封闭三类": {kindServiceCalendar, `{"tenant_id":"SYN-TENANT-1",
			"target_kind":"WAREHOUSE","target_code":"SYN-NODE-A","version":1,
			"effective_from":"2026-08-20T12:00:00Z"}`},
		"调整种类不在封闭四格": {kindAvailabilityAdjustment, `{"tenant_id":"SYN-TENANT-1",
			"code":"SYN-ADJ-1","version":1,"target_kind":"LINE","target_code":"SYN-LINE-EU",
			"kind":"PAUSE","source":"SYN-NET-OPS/EVT-1","effective_at":"2026-08-20T12:00:00Z"}`,
		},
		"空白租户": {kindNode, `{"tenant_id":"  ","code":"SYN-NODE-A","version":1,
			"business_timezone":"Asia/Shanghai","effective_from":"2026-08-20T12:00:00Z"}`},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			double := &registryDouble{}
			message, code := execute(t.Context(), test.kind, []byte(test.payload), registrarOver(t, double), passthroughTransactor{})
			if code != exitUsage {
				t.Fatalf("message=%q code=%d, 想要译装拒绝/1", message, code)
			}
			if double.calls() != 0 {
				t.Fatal("译装不成的字节到达了写入口")
			}
		})
	}
}

// Covers: 受理门拒绝是登记方要改输入的答案——译成退出码 1，拒绝理由在消息里指名，
// 写入口零调用。
func TestExecuteTranslatesGateRefusals(t *testing.T) {
	missingTimezone := []byte(`{
		"tenant_id": "SYN-TENANT-1", "code": "SYN-NODE-HUB", "version": 1,
		"effective_from": "2026-08-20T12:00:00Z"
	}`)
	double := &registryDouble{}
	message, code := execute(t.Context(), kindNode, missingTimezone, registrarOver(t, double), passthroughTransactor{})
	if code != exitUsage || !strings.Contains(message, "TIMEZONE_MISSING") {
		t.Fatalf("message=%q code=%d, 想要 REFUSED(TIMEZONE_MISSING)/1", message, code)
	}
	if double.calls() != 0 {
		t.Fatal("被拒的登记到达了写入口")
	}
}

// Covers: 依赖故障译成退出码 3——登记与否未知，原因在消息里。
func TestExecuteSurfacesUndecided(t *testing.T) {
	boom := errors.New("catalog store is down")
	double := &registryDouble{err: boom}
	payload := []byte(`{
		"tenant_id": "SYN-TENANT-1", "code": "SYN-AREA-DE", "version": 1,
		"effective_from": "2026-08-20T12:00:00Z"
	}`)
	message, code := execute(t.Context(), kindServiceArea, payload, registrarOver(t, double), passthroughTransactor{})
	if code != exitUndecided || !strings.Contains(message, "catalog store is down") {
		t.Fatalf("message=%q code=%d, 想要未决/3 且带成因", message, code)
	}
}
