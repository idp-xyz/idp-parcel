package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 口岸目录与申报路径目录写口的往返用例。做法承 case_config_registry_test.go 头注那
// 两条：断言穿读口取回、写入一律进环境事务。覆盖的版本代数与解释规则写口同款——
// 不可覆盖、换版落终点、错序被排他约束接住——但按本册的键各自钉一遍：这两张表的
// 约束与 SQL 是新写的，先例的绿证不了它们。

func newPortsPathsRegistry(t *testing.T) (*adapter.PortsPathsRegistrations, *adapter.PortsPathsPointView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	registry, err := adapter.NewPortsPathsRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造口岸路径写口：%v", err)
	}
	view, err := adapter.NewPortsPathsPointView(fixture.db)
	if err != nil {
		t.Fatalf("构造口岸路径读口：%v", err)
	}
	return registry, view, fixture
}

// registerPort 是口岸登记短手：同支（tenant-a，SYN-PORT-01）在每个用例里重复太吵。
func registerPort(
	t *testing.T,
	fixture *viewFixture,
	registry *adapter.PortsPathsRegistrations,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	t.Helper()
	return register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCandidatePort(ctx,
			viewValue(t, domain.NewTenantID, "tenant-a"),
			viewValue(t, domain.NewCustomsPortReference, "SYN-PORT-01"), appliesFrom)
	})
}

func loadPortAt(
	t *testing.T,
	view *adapter.PortsPathsPointView,
	evaluatedAt time.Time,
) (ports.CandidatePortEntry, bool, error) {
	t.Helper()
	return view.LoadCandidatePort(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewCustomsPortReference, "SYN-PORT-01"), evaluatedAt)
}

// synRoute 构造一条三维齐全的合成路由。
func synRoute(t *testing.T, port, mode string, direction domain.ManifestDirection) domain.DeclarationPathRoute {
	t.Helper()
	route, err := domain.NewDeclarationPathRoute(
		viewValue(t, domain.NewCustomsPortReference, port),
		direction,
		viewValue(t, domain.NewDeclarationModeReference, mode))
	if err != nil {
		t.Fatalf("构造路径三维：%v", err)
	}
	return route
}

func registerPath(
	t *testing.T,
	fixture *viewFixture,
	registry *adapter.PortsPathsRegistrations,
	route domain.DeclarationPathRoute,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	t.Helper()
	return register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterDeclarationPath(ctx,
			viewValue(t, domain.NewTenantID, "tenant-a"),
			viewValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"), route, appliesFrom)
	})
}

func loadPathAt(
	t *testing.T,
	view *adapter.PortsPathsPointView,
	evaluatedAt time.Time,
) (ports.DeclarationPathEntry, bool, error) {
	t.Helper()
	return view.LoadDeclarationPath(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"), evaluatedAt)
}

// Covers: 口岸登记往返——登记即可按时点解析回，起点如实、终点开放（零值）。
func TestCandidatePortRoundTripsThroughTheView(t *testing.T) {
	registry, view, fixture := newPortsPathsRegistry(t)

	outcome, err := registerPort(t, fixture, registry, registryBaseAt)
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首次登记：err=%v outcome=%v", err, outcome)
	}
	entry, found, err := loadPortAt(t, view, registryBaseAt.Add(time.Hour))
	if err != nil || !found {
		t.Fatalf("登记后读不回：err=%v found=%v", err, found)
	}
	if entry.Port.String() != "SYN-PORT-01" || !entry.AppliesFrom.Equal(registryBaseAt) ||
		!entry.AppliesUntil.IsZero() {
		t.Fatalf("口岸目录行走样：%+v", entry)
	}
}

// Covers: 路径登记往返——三维（口岸、方向、模式）逐格如实读回，不缺不换。
func TestDeclarationPathRoundTripsThroughTheView(t *testing.T) {
	registry, view, fixture := newPortsPathsRegistry(t)
	route := synRoute(t, "SYN-PORT-01", "SYN-MODE-GENERAL", domain.ImportManifest)

	outcome, err := registerPath(t, fixture, registry, route, registryBaseAt)
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首次登记：err=%v outcome=%v", err, outcome)
	}
	entry, found, err := loadPathAt(t, view, registryBaseAt.Add(time.Hour))
	if err != nil || !found {
		t.Fatalf("登记后读不回：err=%v found=%v", err, found)
	}
	if entry.Route != route || entry.Path.String() != "SYN-PATH-01" ||
		!entry.AppliesFrom.Equal(registryBaseAt) || !entry.AppliesUntil.IsZero() {
		t.Fatalf("路径目录行走样：%+v", entry)
	}
}

// Covers: 同键（同支同起点）重登交回`已登记`且不顶替——路径三维换内容也一样，库里
// 仍是首版三维；内容是否同一份由编排读回自己比（写口不判）。
func TestSameStartRegistrationNeverReplacesTheFirstVersion(t *testing.T) {
	registry, view, fixture := newPortsPathsRegistry(t)
	first := synRoute(t, "SYN-PORT-01", "SYN-MODE-GENERAL", domain.ImportManifest)
	second := synRoute(t, "SYN-PORT-02", "SYN-MODE-SIMPLIFIED", domain.ExportManifest)

	if _, err := registerPath(t, fixture, registry, first, registryBaseAt); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	outcome, err := registerPath(t, fixture, registry, second, registryBaseAt)
	if err != nil || outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同键重登该交回`已登记`：err=%v outcome=%v", err, outcome)
	}
	entry, found, err := loadPathAt(t, view, registryBaseAt)
	if err != nil || !found || entry.Route != first {
		t.Fatalf("首版三维被顶替：err=%v found=%v route=%+v", err, found, entry.Route)
	}

	if _, err := registerPort(t, fixture, registry, registryBaseAt); err != nil {
		t.Fatalf("口岸首次登记：%v", err)
	}
	portOutcome, err := registerPort(t, fixture, registry, registryBaseAt)
	if err != nil || portOutcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("口岸同键重登该交回`已登记`：err=%v outcome=%v", err, portOutcome)
	}
}

// Covers: 换版——登记更晚起点的新版给开放前版落终点；旧时点仍解析回旧版（半开区间：
// 换版边界起归后继），绝不是当前指针。
func TestSupersedingClosesThePredecessorOnBothRegisters(t *testing.T) {
	registry, view, fixture := newPortsPathsRegistry(t)
	successionAt := registryBaseAt.Add(48 * time.Hour)

	if _, err := registerPort(t, fixture, registry, registryBaseAt); err != nil {
		t.Fatalf("登记口岸 v1：%v", err)
	}
	outcome, err := registerPort(t, fixture, registry, successionAt)
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("口岸换版该是新登记：err=%v outcome=%v", err, outcome)
	}
	old, foundOld, err := loadPortAt(t, view, registryBaseAt.Add(time.Hour))
	if err != nil || !foundOld || !old.AppliesUntil.Equal(successionAt) {
		t.Fatalf("口岸前版终点没落定为后继起点：err=%v found=%v until=%v", err, foundOld, old.AppliesUntil)
	}
	current, foundCurrent, err := loadPortAt(t, view, successionAt)
	if err != nil || !foundCurrent || !current.AppliesFrom.Equal(successionAt) {
		t.Fatalf("换版边界该归后继：err=%v found=%v from=%v", err, foundCurrent, current.AppliesFrom)
	}

	v1 := synRoute(t, "SYN-PORT-01", "SYN-MODE-GENERAL", domain.ImportManifest)
	v2 := synRoute(t, "SYN-PORT-02", "SYN-MODE-GENERAL", domain.ImportManifest)
	if _, err := registerPath(t, fixture, registry, v1, registryBaseAt); err != nil {
		t.Fatalf("登记路径 v1：%v", err)
	}
	if _, err := registerPath(t, fixture, registry, v2, successionAt); err != nil {
		t.Fatalf("登记路径 v2：%v", err)
	}
	oldPath, foundOldPath, err := loadPathAt(t, view, registryBaseAt.Add(time.Hour))
	if err != nil || !foundOldPath || oldPath.Route != v1 || !oldPath.AppliesUntil.Equal(successionAt) {
		t.Fatalf("路径旧时点没解析回旧版：err=%v found=%v entry=%+v", err, foundOldPath, oldPath)
	}
	currentPath, foundCurrentPath, err := loadPathAt(t, view, successionAt.Add(365*24*time.Hour))
	if err != nil || !foundCurrentPath || currentPath.Route != v2 {
		t.Fatalf("路径开放尾段没答当前版：err=%v found=%v entry=%+v", err, foundCurrentPath, currentPath)
	}
}

// Covers: 起点早于既有覆盖面的错序登记撞排他约束折成`已登记`，一行未写——历史区间
// 不接受追改；按请求起点读不回版本，编排据此判冲突。
func TestABackdatedRegistrationIsHeldByTheOverlapGuard(t *testing.T) {
	registry, view, fixture := newPortsPathsRegistry(t)

	if _, err := registerPort(t, fixture, registry, registryBaseAt); err != nil {
		t.Fatalf("登记开放版：%v", err)
	}
	backdatedAt := registryBaseAt.Add(-48 * time.Hour)
	outcome, err := registerPort(t, fixture, registry, backdatedAt)
	if err != nil || outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("错序登记该被折成`已登记`：err=%v outcome=%v", err, outcome)
	}
	if _, found, err := loadPortAt(t, view, backdatedAt); err != nil || found {
		t.Fatalf("错序登记竟然可解析：err=%v found=%v", err, found)
	}
}

// Covers: 零值生效起点与零值路由方向都不静默写成一行——调用方编程错误，与「实例还
// 没登记」两回事。
func TestRegisteringWithZeroStartOrZeroRouteIsLoud(t *testing.T) {
	registry, _, fixture := newPortsPathsRegistry(t)

	if _, err := registerPort(t, fixture, registry, time.Time{}); err == nil {
		t.Fatal("零值起点的口岸登记被静默接受")
	}
	route := synRoute(t, "SYN-PORT-01", "SYN-MODE-GENERAL", domain.ImportManifest)
	if _, err := registerPath(t, fixture, registry, route, time.Time{}); err == nil {
		t.Fatal("零值起点的路径登记被静默接受")
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterDeclarationPath(ctx,
			viewValue(t, domain.NewTenantID, "tenant-a"),
			viewValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"),
			domain.DeclarationPathRoute{}, registryBaseAt)
	}); err == nil {
		t.Fatal("零值路由的路径登记被静默接受")
	}
}

// Covers: 两个写方法在无事务上下文一律被 RequireExecutor 拒绝（ErrTransactionRequired），
// 不退回连接池旁路写入——判据同包 transaction_guard_test.go 那族，随适配器逐个成立。
func TestPortsPathsWritesRefuseToRunOutsideATransaction(t *testing.T) {
	registry, _, _ := newPortsPathsRegistry(t)
	ctx := t.Context()

	if _, err := registry.RegisterCandidatePort(ctx,
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewCustomsPortReference, "SYN-PORT-01"),
		registryBaseAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记口岸应返回 ErrTransactionRequired，实得：%v", err)
	}
	route := synRoute(t, "SYN-PORT-01", "SYN-MODE-GENERAL", domain.ImportManifest)
	if _, err := registry.RegisterDeclarationPath(ctx,
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"),
		route, registryBaseAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记路径应返回 ErrTransactionRequired，实得：%v", err)
	}
}
