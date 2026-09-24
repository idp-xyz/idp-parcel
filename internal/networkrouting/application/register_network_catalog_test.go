package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件证目录登记用例的三件职责：受理门把缺件译成指名拒绝且不放行到写入口、齐备的
// 登记原样透传（一个字段都不补不改）、写入口的错误上抛不折格。写入口本身的版本纪律
// （接续闭合、修订同事务 +1、主键与部分唯一索引）由适配器真库用例证，这里用替身。
// 验证值一律 SYN- 合成（隔离合成只记 S）。

var catalogEffective = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

type catalogRegistryDouble struct {
	err   error
	calls int

	lastTenant domain.TenantID
	node       ports.NodeDefinitionVersion
	connection ports.ConnectionDefinitionVersion
	line       ports.LineDefinitionVersion
	area       ports.ServiceAreaDefinitionVersion
	calendar   ports.ServiceCalendarDefinitionVersion
	strategy   ports.RouteStrategyDefinitionVersion
	adjustment ports.AvailabilityAdjustmentStatement
}

func (double *catalogRegistryDouble) RegisterNodeVersion(
	_ context.Context, tenant domain.TenantID, row ports.NodeDefinitionVersion,
) error {
	double.calls++
	double.lastTenant, double.node = tenant, row
	return double.err
}

func (double *catalogRegistryDouble) RegisterConnectionVersion(
	_ context.Context, tenant domain.TenantID, row ports.ConnectionDefinitionVersion,
) error {
	double.calls++
	double.lastTenant, double.connection = tenant, row
	return double.err
}

func (double *catalogRegistryDouble) RegisterLineVersion(
	_ context.Context, tenant domain.TenantID, row ports.LineDefinitionVersion,
) error {
	double.calls++
	double.lastTenant, double.line = tenant, row
	return double.err
}

func (double *catalogRegistryDouble) RegisterServiceAreaVersion(
	_ context.Context, tenant domain.TenantID, row ports.ServiceAreaDefinitionVersion,
) error {
	double.calls++
	double.lastTenant, double.area = tenant, row
	return double.err
}

func (double *catalogRegistryDouble) RegisterServiceCalendarVersion(
	_ context.Context, tenant domain.TenantID, row ports.ServiceCalendarDefinitionVersion,
) error {
	double.calls++
	double.lastTenant, double.calendar = tenant, row
	return double.err
}

func (double *catalogRegistryDouble) RegisterRouteStrategyVersion(
	_ context.Context, tenant domain.TenantID, row ports.RouteStrategyDefinitionVersion,
) error {
	double.calls++
	double.lastTenant, double.strategy = tenant, row
	return double.err
}

func (double *catalogRegistryDouble) RegisterAvailabilityAdjustment(
	_ context.Context, tenant domain.TenantID, row ports.AvailabilityAdjustmentStatement,
) error {
	double.calls++
	double.lastTenant, double.adjustment = tenant, row
	return double.err
}

func newCatalogRegistration(t *testing.T, registry ports.NetworkCatalogRegistry) *application.NetworkCatalogRegistration {
	t.Helper()
	service, err := application.NewNetworkCatalogRegistration(registry)
	if err != nil {
		t.Fatalf("构造目录登记用例：%v", err)
	}
	return service
}

func catalogTenant(t *testing.T) domain.TenantID {
	t.Helper()
	return value(t, domain.NewTenantID, "SYN-TENANT-1")
}

func validNode() ports.NodeDefinitionVersion {
	return ports.NodeDefinitionVersion{
		Code: "SYN-NODE-HUB", Version: 1, BusinessTimezone: "Asia/Shanghai",
		EffectiveFrom: catalogEffective,
	}
}

func validConnection() ports.ConnectionDefinitionVersion {
	return ports.ConnectionDefinitionVersion{
		Code: "SYN-CONN-A-B", Version: 1, FromNode: "SYN-NODE-A", ToNode: "SYN-NODE-B",
		BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogEffective,
	}
}

func validLine() ports.LineDefinitionVersion {
	return ports.LineDefinitionVersion{
		Code: "SYN-LINE-EU", Version: 1, Segments: []string{"SYN-CONN-A-B"},
		BusinessTimezone: "Asia/Shanghai", ApplicableScope: "SYN-SCOPE-DECLARED",
		EffectiveFrom: catalogEffective,
	}
}

func validArea() ports.ServiceAreaDefinitionVersion {
	return ports.ServiceAreaDefinitionVersion{
		Code: "SYN-AREA-DE", Version: 1, EffectiveFrom: catalogEffective,
	}
}

func validCalendar() ports.ServiceCalendarDefinitionVersion {
	return ports.ServiceCalendarDefinitionVersion{
		TargetKind: ports.TargetNode, TargetCode: "SYN-NODE-HUB", Version: 1,
		EffectiveFrom: catalogEffective,
	}
}

func validStrategy() ports.RouteStrategyDefinitionVersion {
	return ports.RouteStrategyDefinitionVersion{
		Code: "SYN-STRATEGY-1", Version: 1, ApplicableScope: "SYN-SCOPE-DECLARED",
		EffectiveFrom: catalogEffective,
	}
}

func validAdjustment() ports.AvailabilityAdjustmentStatement {
	return ports.AvailabilityAdjustmentStatement{
		Code: "SYN-ADJ-1", Version: 1,
		TargetKind: ports.TargetLine, TargetCode: "SYN-LINE-EU",
		Kind: ports.AdjustmentSuspension, Source: "SYN-NET-OPS/EVT-1",
		EffectiveAt: catalogEffective,
	}
}

// register 把七族登记收敛成一个签名，让门用例按族查表；它只透传结果，不吸收错误。
type registerCall func(t *testing.T, service *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error)

// Covers: 票 04 件①「受理门齐备：拒零值、逐格指名」。每格改坏一件，期望指名拒绝且
// 写入口零调用——被拒的登记到达写入口，环境事务就会带着半份检查进库。
func TestCatalogRegistrationRefusesIncompleteRowsBySlot(t *testing.T) {
	tenant := func(t *testing.T) domain.TenantID { return catalogTenant(t) }

	cases := []struct {
		name string
		call registerCall
		want application.CatalogRefusalReason
	}{
		{"节点缺租户", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			return s.RegisterNodeVersion(t.Context(), application.RegisterNodeVersionCommand{Node: validNode()})
		}, application.CatalogTenantMissing},
		{"节点缺身份码", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validNode()
			row.Code = "  "
			return s.RegisterNodeVersion(t.Context(), application.RegisterNodeVersionCommand{TenantID: tenant(t), Node: row})
		}, application.CatalogIdentityMissing},
		{"节点缺版本号", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validNode()
			row.Version = 0
			return s.RegisterNodeVersion(t.Context(), application.RegisterNodeVersionCommand{TenantID: tenant(t), Node: row})
		}, application.CatalogVersionMissing},
		{"节点缺业务时区", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validNode()
			row.BusinessTimezone = ""
			return s.RegisterNodeVersion(t.Context(), application.RegisterNodeVersionCommand{TenantID: tenant(t), Node: row})
		}, application.CatalogTimezoneMissing},
		{"节点缺生效时间", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validNode()
			row.EffectiveFrom = time.Time{}
			return s.RegisterNodeVersion(t.Context(), application.RegisterNodeVersionCommand{TenantID: tenant(t), Node: row})
		}, application.CatalogEffectiveTimeMissing},
		{"节点区间倒序", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validNode()
			row.EffectiveTo, row.HasEffectiveTo = row.EffectiveFrom, true
			return s.RegisterNodeVersion(t.Context(), application.RegisterNodeVersionCommand{TenantID: tenant(t), Node: row})
		}, application.CatalogEffectiveRangeReversed},

		{"连接缺端点", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validConnection()
			row.ToNode = ""
			return s.RegisterConnectionVersion(t.Context(), application.RegisterConnectionVersionCommand{TenantID: tenant(t), Connection: row})
		}, application.CatalogEndpointMissing},
		{"连接两端相同", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validConnection()
			row.ToNode = row.FromNode
			return s.RegisterConnectionVersion(t.Context(), application.RegisterConnectionVersionCommand{TenantID: tenant(t), Connection: row})
		}, application.CatalogEndpointsNotDistinct},
		{"连接缺业务时区", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validConnection()
			row.BusinessTimezone = " "
			return s.RegisterConnectionVersion(t.Context(), application.RegisterConnectionVersionCommand{TenantID: tenant(t), Connection: row})
		}, application.CatalogTimezoneMissing},

		{"线路零段", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validLine()
			row.Segments = nil
			return s.RegisterLineVersion(t.Context(), application.RegisterLineVersionCommand{TenantID: tenant(t), Line: row})
		}, application.CatalogSegmentsMissing},
		{"线路段链带空身份", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validLine()
			row.Segments = []string{"SYN-CONN-A-B", "  "}
			return s.RegisterLineVersion(t.Context(), application.RegisterLineVersionCommand{TenantID: tenant(t), Line: row})
		}, application.CatalogSegmentBlank},
		{"线路缺适用范围", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validLine()
			row.ApplicableScope = ""
			return s.RegisterLineVersion(t.Context(), application.RegisterLineVersionCommand{TenantID: tenant(t), Line: row})
		}, application.CatalogApplicableScopeMissing},

		{"区域缺身份码", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validArea()
			row.Code = ""
			return s.RegisterServiceAreaVersion(t.Context(), application.RegisterServiceAreaVersionCommand{TenantID: tenant(t), Area: row})
		}, application.CatalogIdentityMissing},

		{"日历适用对象类别不在封闭三类", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validCalendar()
			row.TargetKind = ports.CatalogTargetKindInvalid
			return s.RegisterServiceCalendarVersion(t.Context(), application.RegisterServiceCalendarVersionCommand{TenantID: tenant(t), Calendar: row})
		}, application.CatalogTargetKindUnknown},
		{"日历缺对象身份", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validCalendar()
			row.TargetCode = ""
			return s.RegisterServiceCalendarVersion(t.Context(), application.RegisterServiceCalendarVersionCommand{TenantID: tenant(t), Calendar: row})
		}, application.CatalogIdentityMissing},

		{"策略缺适用范围", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validStrategy()
			row.ApplicableScope = "  "
			return s.RegisterRouteStrategyVersion(t.Context(), application.RegisterRouteStrategyVersionCommand{TenantID: tenant(t), Strategy: row})
		}, application.CatalogApplicableScopeMissing},
		{"策略排序形态不在族内", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validStrategy()
			row.RankingForm = domain.RankingForm(99)
			return s.RegisterRouteStrategyVersion(t.Context(), application.RegisterRouteStrategyVersionCommand{TenantID: tenant(t), Strategy: row})
		}, application.CatalogRankingFormUnknown},

		{"调整种类不在封闭四格", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validAdjustment()
			row.Kind = ports.AvailabilityAdjustmentKindInvalid
			return s.RegisterAvailabilityAdjustment(t.Context(), application.RegisterAvailabilityAdjustmentCommand{TenantID: tenant(t), Adjustment: row})
		}, application.CatalogAdjustmentKindUnknown},
		{"调整缺来源", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validAdjustment()
			row.Source = ""
			return s.RegisterAvailabilityAdjustment(t.Context(), application.RegisterAvailabilityAdjustmentCommand{TenantID: tenant(t), Adjustment: row})
		}, application.CatalogSourceMissing},
		{"调整缺生效时刻", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validAdjustment()
			row.EffectiveAt = time.Time{}
			return s.RegisterAvailabilityAdjustment(t.Context(), application.RegisterAvailabilityAdjustmentCommand{TenantID: tenant(t), Adjustment: row})
		}, application.CatalogEffectiveTimeMissing},
		{"调整解除不晚于生效", func(t *testing.T, s *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			row := validAdjustment()
			row.LiftedAt, row.HasLiftedAt = row.EffectiveAt, true
			return s.RegisterAvailabilityAdjustment(t.Context(), application.RegisterAvailabilityAdjustmentCommand{TenantID: tenant(t), Adjustment: row})
		}, application.CatalogEffectiveRangeReversed},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			double := &catalogRegistryDouble{}
			result, err := test.call(t, newCatalogRegistration(t, double))
			if err != nil {
				t.Fatalf("受理门拒绝是业务答案不是错误，实得：%v", err)
			}
			if result.Outcome() != application.CatalogRegistrationRefused {
				t.Fatalf("outcome = %s，想要 REFUSED", result.Outcome())
			}
			if result.RefusalReason() != test.want {
				t.Fatalf("refusal = %s，想要 %s", result.RefusalReason(), test.want)
			}
			if double.calls != 0 {
				t.Fatal("被拒的登记到达了写入口")
			}
		})
	}
}

// Covers: 齐备登记的透传——七族各自过门后原样到达写入口（租户与整行逐字段同），
// 结果为`已登记`且不带拒绝理由。补历史（已闭区间）是正当用法，节点样例带闭区间证它
// 过得了门。
func TestCatalogRegistrationPassesCompleteRowsThroughUntouched(t *testing.T) {
	double := &catalogRegistryDouble{}
	service := newCatalogRegistration(t, double)
	tenant := catalogTenant(t)
	ctx := t.Context()

	closedNode := validNode()
	closedNode.EffectiveTo, closedNode.HasEffectiveTo = catalogEffective.Add(24*time.Hour), true

	checks := []struct {
		name string
		run  func() (application.RegisterCatalogResult, error)
		got  func() any
		want any
	}{
		{"节点（补历史闭区间）", func() (application.RegisterCatalogResult, error) {
			return service.RegisterNodeVersion(ctx, application.RegisterNodeVersionCommand{TenantID: tenant, Node: closedNode})
		}, func() any { return double.node }, closedNode},
		{"连接", func() (application.RegisterCatalogResult, error) {
			return service.RegisterConnectionVersion(ctx, application.RegisterConnectionVersionCommand{TenantID: tenant, Connection: validConnection()})
		}, func() any { return double.connection }, validConnection()},
		{"线路", func() (application.RegisterCatalogResult, error) {
			return service.RegisterLineVersion(ctx, application.RegisterLineVersionCommand{TenantID: tenant, Line: validLine()})
		}, func() any { return double.line }, validLine()},
		{"服务区域", func() (application.RegisterCatalogResult, error) {
			return service.RegisterServiceAreaVersion(ctx, application.RegisterServiceAreaVersionCommand{TenantID: tenant, Area: validArea()})
		}, func() any { return double.area }, validArea()},
		{"服务日历", func() (application.RegisterCatalogResult, error) {
			return service.RegisterServiceCalendarVersion(ctx, application.RegisterServiceCalendarVersionCommand{TenantID: tenant, Calendar: validCalendar()})
		}, func() any { return double.calendar }, validCalendar()},
		{"路由策略", func() (application.RegisterCatalogResult, error) {
			return service.RegisterRouteStrategyVersion(ctx, application.RegisterRouteStrategyVersionCommand{TenantID: tenant, Strategy: validStrategy()})
		}, func() any { return double.strategy }, validStrategy()},
		{"可用性调整", func() (application.RegisterCatalogResult, error) {
			return service.RegisterAvailabilityAdjustment(ctx, application.RegisterAvailabilityAdjustmentCommand{TenantID: tenant, Adjustment: validAdjustment()})
		}, func() any { return double.adjustment }, validAdjustment()},
	}

	for _, check := range checks {
		result, err := check.run()
		if err != nil {
			t.Fatalf("%s：%v", check.name, err)
		}
		if result.Outcome() != application.CatalogRegistered ||
			result.RefusalReason() != application.CatalogRefusalReasonNone {
			t.Fatalf("%s：outcome=%s refusal=%s，想要 REGISTERED 且无拒绝理由",
				check.name, result.Outcome(), result.RefusalReason())
		}
		if !reflect.DeepEqual(check.got(), check.want) {
			t.Fatalf("%s：写入口收到 %+v，想要原样 %+v", check.name, check.got(), check.want)
		}
		if double.lastTenant != tenant {
			t.Fatalf("%s：租户没有原样到达写入口", check.name)
		}
	}
	if double.calls != len(checks) {
		t.Fatalf("写入口应被调 %d 次，实得 %d", len(checks), double.calls)
	}
}

// Covers: 写入口错误上抛不折格——依赖故障时登记与否未知，没有一个如实的业务格可记，
// 交回错误由进程级入口译成`未决`。
func TestCatalogRegistrationSurfacesRegistryErrors(t *testing.T) {
	boom := errors.New("catalog store is down")
	double := &catalogRegistryDouble{err: boom}
	service := newCatalogRegistration(t, double)

	result, err := service.RegisterNodeVersion(t.Context(),
		application.RegisterNodeVersionCommand{TenantID: catalogTenant(t), Node: validNode()})
	if !errors.Is(err, boom) {
		t.Fatalf("写入口的错误没有原样上抛：%v", err)
	}
	if result.Outcome() != application.RegisterCatalogOutcomeInvalid {
		t.Fatalf("错误路上不得携带业务结果，实得 %s", result.Outcome())
	}
}

// Covers: 构造门——没有写入口的登记用例什么也登不了，nil 依赖在构造期拒绝。
func TestCatalogRegistrationRequiresARegistry(t *testing.T) {
	if _, err := application.NewNetworkCatalogRegistration(nil); err == nil {
		t.Fatal("nil 写入口应在构造期被拒")
	}
}
