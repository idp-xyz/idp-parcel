package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 口岸目录与申报路径目录登记用例的行为面：受理门逐格拒、重放/换版/冲突三格分得开、
// 依赖故障折未决。替身照真库代数——换版先给开放前版落终点，撞键与撞重叠都只答
// `已登记`，绝不顶替（ports_paths_registry.go 同一段规则的内存版）。

var portsPathsBaseAt = time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)

type portsPathsPortRow struct {
	tenant string
	entry  ports.CandidatePortEntry
}

type portsPathsPathRow struct {
	tenant string
	entry  ports.DeclarationPathEntry
}

type portsPathsStoreDouble struct {
	portRows    []portsPathsPortRow
	pathRows    []portsPathsPathRow
	registerErr error
	loadErr     error
}

// intervalsOverlap 判半开区间 [fromA, untilA) 与 [fromB, untilB) 相交；零值终点即开放。
func intervalsOverlap(fromA, untilA, fromB, untilB time.Time) bool {
	aBeforeBEnd := untilB.IsZero() || fromA.Before(untilB)
	bBeforeAEnd := untilA.IsZero() || fromB.Before(untilA)
	return aBeforeBEnd && bBeforeAEnd
}

func (double *portsPathsStoreDouble) RegisterCandidatePort(
	_ context.Context,
	tenant domain.TenantID,
	port domain.CustomsPortReference,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.registerErr
	}
	for index := range double.portRows {
		row := &double.portRows[index]
		if row.tenant == tenant.String() && row.entry.Port == port &&
			row.entry.AppliesUntil.IsZero() && row.entry.AppliesFrom.Before(appliesFrom) {
			row.entry.AppliesUntil = appliesFrom
		}
	}
	for _, row := range double.portRows {
		if row.tenant == tenant.String() && row.entry.Port == port &&
			(row.entry.AppliesFrom.Equal(appliesFrom) ||
				intervalsOverlap(appliesFrom, time.Time{}, row.entry.AppliesFrom, row.entry.AppliesUntil)) {
			return ports.CaseConfigurationAlreadyRegistered, nil
		}
	}
	double.portRows = append(double.portRows, portsPathsPortRow{
		tenant: tenant.String(),
		entry:  ports.CandidatePortEntry{Port: port, AppliesFrom: appliesFrom},
	})
	return ports.CaseConfigurationRegistered, nil
}

func (double *portsPathsStoreDouble) RegisterDeclarationPath(
	_ context.Context,
	tenant domain.TenantID,
	path domain.DeclarationPathReference,
	route domain.DeclarationPathRoute,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.registerErr
	}
	for index := range double.pathRows {
		row := &double.pathRows[index]
		if row.tenant == tenant.String() && row.entry.Path == path &&
			row.entry.AppliesUntil.IsZero() && row.entry.AppliesFrom.Before(appliesFrom) {
			row.entry.AppliesUntil = appliesFrom
		}
	}
	for _, row := range double.pathRows {
		if row.tenant == tenant.String() && row.entry.Path == path &&
			(row.entry.AppliesFrom.Equal(appliesFrom) ||
				intervalsOverlap(appliesFrom, time.Time{}, row.entry.AppliesFrom, row.entry.AppliesUntil)) {
			return ports.CaseConfigurationAlreadyRegistered, nil
		}
	}
	double.pathRows = append(double.pathRows, portsPathsPathRow{
		tenant: tenant.String(),
		entry:  ports.DeclarationPathEntry{Path: path, Route: route, AppliesFrom: appliesFrom},
	})
	return ports.CaseConfigurationRegistered, nil
}

func (double *portsPathsStoreDouble) LoadCandidatePort(
	_ context.Context,
	tenant domain.TenantID,
	port domain.CustomsPortReference,
	evaluatedAt time.Time,
) (ports.CandidatePortEntry, bool, error) {
	if double.loadErr != nil {
		return ports.CandidatePortEntry{}, false, double.loadErr
	}
	for _, row := range double.portRows {
		if row.tenant == tenant.String() && row.entry.Port == port &&
			!row.entry.AppliesFrom.After(evaluatedAt) &&
			(row.entry.AppliesUntil.IsZero() || row.entry.AppliesUntil.After(evaluatedAt)) {
			return row.entry, true, nil
		}
	}
	return ports.CandidatePortEntry{}, false, nil
}

func (double *portsPathsStoreDouble) LoadDeclarationPath(
	_ context.Context,
	tenant domain.TenantID,
	path domain.DeclarationPathReference,
	evaluatedAt time.Time,
) (ports.DeclarationPathEntry, bool, error) {
	if double.loadErr != nil {
		return ports.DeclarationPathEntry{}, false, double.loadErr
	}
	for _, row := range double.pathRows {
		if row.tenant == tenant.String() && row.entry.Path == path &&
			!row.entry.AppliesFrom.After(evaluatedAt) &&
			(row.entry.AppliesUntil.IsZero() || row.entry.AppliesUntil.After(evaluatedAt)) {
			return row.entry, true, nil
		}
	}
	return ports.DeclarationPathEntry{}, false, nil
}

func newPortsPathsHandler(store *portsPathsStoreDouble) *application.RegisterPortsPathsHandler {
	return application.NewRegisterPortsPathsHandler(application.RegisterPortsPathsDeps{
		Registry: store,
		View:     store,
	})
}

func portCommand(t *testing.T, appliesFrom time.Time) application.RegisterCandidatePortCommand {
	t.Helper()
	return application.RegisterCandidatePortCommand{
		TenantID:    configValue(t, domain.NewTenantID, "tenant-a"),
		Port:        configValue(t, domain.NewCustomsPortReference, "SYN-PORT-01"),
		AppliesFrom: appliesFrom,
	}
}

func pathCommand(t *testing.T, portRef string, appliesFrom time.Time) application.RegisterDeclarationPathCommand {
	t.Helper()
	route, err := domain.NewDeclarationPathRoute(
		configValue(t, domain.NewCustomsPortReference, portRef),
		domain.ImportManifest,
		configValue(t, domain.NewDeclarationModeReference, "SYN-MODE-GENERAL"))
	if err != nil {
		t.Fatalf("构造路径三维：%v", err)
	}
	return application.RegisterDeclarationPathCommand{
		TenantID:    configValue(t, domain.NewTenantID, "tenant-a"),
		Path:        configValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"),
		Route:       route,
		AppliesFrom: appliesFrom,
	}
}

func TestRegisteringPortsAndPathsLands(t *testing.T) {
	store := &portsPathsStoreDouble{}
	handler := newPortsPathsHandler(store)

	outcome, err := handler.RegisterCandidatePort(t.Context(), portCommand(t, portsPathsBaseAt))
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("口岸登记：err=%v outcome=%v", err, outcome)
	}
	outcome, err = handler.RegisterDeclarationPath(t.Context(), pathCommand(t, "SYN-PORT-01", portsPathsBaseAt))
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("路径登记：err=%v outcome=%v", err, outcome)
	}
	if len(store.portRows) != 1 || len(store.pathRows) != 1 {
		t.Fatalf("册上行数走样：ports=%d paths=%d", len(store.portRows), len(store.pathRows))
	}
}

// 受理门逐格拒：租户/键/起点/路径三维任一缺席都不受理，被拒的登记不落册。
func TestPortsPathsRegistrationRefusesBlankFields(t *testing.T) {
	store := &portsPathsStoreDouble{}
	handler := newPortsPathsHandler(store)

	blankTenantPort := portCommand(t, portsPathsBaseAt)
	blankTenantPort.TenantID = domain.TenantID{}
	blankPort := portCommand(t, portsPathsBaseAt)
	blankPort.Port = domain.CustomsPortReference{}
	zeroStartPort := portCommand(t, time.Time{})

	for name, command := range map[string]application.RegisterCandidatePortCommand{
		"租户": blankTenantPort,
		"口岸": blankPort,
		"起点": zeroStartPort,
	} {
		outcome, err := handler.RegisterCandidatePort(t.Context(), command)
		if err != nil || outcome != application.ConfigurationNotAccepted {
			t.Fatalf("口岸登记缺%s该拒：err=%v outcome=%v", name, err, outcome)
		}
	}

	blankPath := pathCommand(t, "SYN-PORT-01", portsPathsBaseAt)
	blankPath.Path = domain.DeclarationPathReference{}
	zeroRoute := pathCommand(t, "SYN-PORT-01", portsPathsBaseAt)
	zeroRoute.Route = domain.DeclarationPathRoute{}
	zeroStartPath := pathCommand(t, "SYN-PORT-01", time.Time{})

	for name, command := range map[string]application.RegisterDeclarationPathCommand{
		"路径": blankPath,
		"三维": zeroRoute,
		"起点": zeroStartPath,
	} {
		outcome, err := handler.RegisterDeclarationPath(t.Context(), command)
		if err != nil || outcome != application.ConfigurationNotAccepted {
			t.Fatalf("路径登记缺%s该拒：err=%v outcome=%v", name, err, outcome)
		}
	}
	if len(store.portRows) != 0 || len(store.pathRows) != 0 {
		t.Fatal("被拒的登记落了册")
	}
}

func TestReRegisteringTheSamePortsPathsVersionIsExisting(t *testing.T) {
	store := &portsPathsStoreDouble{}
	handler := newPortsPathsHandler(store)

	if outcome, err := handler.RegisterCandidatePort(t.Context(),
		portCommand(t, portsPathsBaseAt)); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("口岸首登：err=%v outcome=%v", err, outcome)
	}
	if outcome, err := handler.RegisterCandidatePort(t.Context(),
		portCommand(t, portsPathsBaseAt)); err != nil || outcome != application.ConfigurationExisting {
		t.Fatalf("口岸重放该是`已存在`：err=%v outcome=%v", err, outcome)
	}

	if outcome, err := handler.RegisterDeclarationPath(t.Context(),
		pathCommand(t, "SYN-PORT-01", portsPathsBaseAt)); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("路径首登：err=%v outcome=%v", err, outcome)
	}
	if outcome, err := handler.RegisterDeclarationPath(t.Context(),
		pathCommand(t, "SYN-PORT-01", portsPathsBaseAt)); err != nil || outcome != application.ConfigurationExisting {
		t.Fatalf("路径重放该是`已存在`：err=%v outcome=%v", err, outcome)
	}
}

// 同键换三维是冲突不是换版，册面纹丝不动；换版走登记更晚起点的新版本——那是新登记。
func TestChangingARouteAtTheSameStartConflictsButSupersessionRegisters(t *testing.T) {
	store := &portsPathsStoreDouble{}
	handler := newPortsPathsHandler(store)

	if outcome, err := handler.RegisterDeclarationPath(t.Context(),
		pathCommand(t, "SYN-PORT-01", portsPathsBaseAt)); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}
	outcome, err := handler.RegisterDeclarationPath(t.Context(),
		pathCommand(t, "SYN-PORT-02", portsPathsBaseAt))
	if err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("同键换三维该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
	if store.pathRows[0].entry.Route.Port().String() != "SYN-PORT-01" {
		t.Fatalf("冲突顶掉了在册三维：%+v", store.pathRows[0].entry)
	}

	successionAt := portsPathsBaseAt.Add(48 * time.Hour)
	outcome, err = handler.RegisterDeclarationPath(t.Context(), pathCommand(t, "SYN-PORT-02", successionAt))
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("换版该是新登记：err=%v outcome=%v", err, outcome)
	}
	if !store.pathRows[0].entry.AppliesUntil.Equal(successionAt) {
		t.Fatalf("前版终点没落定：%+v", store.pathRows[0].entry)
	}
}

// 错序（起点早于既有覆盖面）与落进历史区间中途的登记都是冲突：读回版本起点不等于
// 请求起点——区间也是登记内容的一部分，不折成重放。
func TestRegistrationsAgainstForeignIntervalsConflict(t *testing.T) {
	store := &portsPathsStoreDouble{}
	handler := newPortsPathsHandler(store)
	successionAt := portsPathsBaseAt.Add(48 * time.Hour)

	if outcome, err := handler.RegisterCandidatePort(t.Context(),
		portCommand(t, portsPathsBaseAt)); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("v1：err=%v outcome=%v", err, outcome)
	}
	if outcome, err := handler.RegisterCandidatePort(t.Context(),
		portCommand(t, successionAt)); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("v2：err=%v outcome=%v", err, outcome)
	}

	backdated, err := handler.RegisterCandidatePort(t.Context(),
		portCommand(t, portsPathsBaseAt.Add(-48*time.Hour)))
	if err != nil || backdated != application.ConfigurationContentConflict {
		t.Fatalf("错序登记该是`内容冲突`：err=%v outcome=%v", err, backdated)
	}
	midInterval, err := handler.RegisterCandidatePort(t.Context(),
		portCommand(t, portsPathsBaseAt.Add(time.Hour)))
	if err != nil || midInterval != application.ConfigurationContentConflict {
		t.Fatalf("落进历史区间中途该是`内容冲突`：err=%v outcome=%v", err, midInterval)
	}
}

func TestPortsPathsDependencyFailuresAreUndecided(t *testing.T) {
	writerDown := &portsPathsStoreDouble{registerErr: errors.New("writer unavailable")}
	outcome, err := newPortsPathsHandler(writerDown).RegisterCandidatePort(
		t.Context(), portCommand(t, portsPathsBaseAt))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("写口故障该未决：err=%v outcome=%v", err, outcome)
	}

	viewDown := &portsPathsStoreDouble{}
	handler := newPortsPathsHandler(viewDown)
	if outcome, err := handler.RegisterDeclarationPath(t.Context(),
		pathCommand(t, "SYN-PORT-01", portsPathsBaseAt)); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}
	viewDown.loadErr = errors.New("view unavailable")
	outcome, err = handler.RegisterDeclarationPath(t.Context(), pathCommand(t, "SYN-PORT-01", portsPathsBaseAt))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("已在册但读不回该未决：err=%v outcome=%v", err, outcome)
	}
}
