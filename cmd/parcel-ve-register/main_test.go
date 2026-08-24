package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	vepg "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证登记口的编排：六命令各自路由到对的登记方法、退出码翻译、身份双轨各行
// 其道（②登记内容不得冒充①通道身份）、只有已登记的执行留痕、留痕失败随事务翻成
// 未决。用例的缺件判据在应用层已证，这里接真用例配写入口替身，证的是登记口把它们
// 接对。

var executeAt = time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)

var testIdentity = channelIdentity{osUser: "OPSHOST\\operator-a", hostname: "ops-host-01"}

type passthroughTransactor struct{}

func (passthroughTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	return fn(ctx)
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type traceRecorder struct {
	executions []vepg.ChannelExecution
	err        error
}

func (recorder *traceRecorder) Append(_ context.Context, execution vepg.ChannelExecution) error {
	if recorder.err != nil {
		return recorder.err
	}
	recorder.executions = append(recorder.executions, execution)
	return nil
}

// cliCatalogRegistry 是写入口替身：记方法名与租户，按配置交回结果。
type cliCatalogRegistry struct {
	outcome ports.CatalogRegistrationOutcome
	err     error
	calls   []string
}

func (registry *cliCatalogRegistry) record(method string, tenant domain.TenantID) (ports.CatalogRegistrationOutcome, error) {
	registry.calls = append(registry.calls, method+"/"+tenant.String())
	return registry.outcome, registry.err
}

func (registry *cliCatalogRegistry) RegisterMilestoneMapping(
	_ context.Context, tenant domain.TenantID, _ ports.MilestoneMappingRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.record(commandMilestoneMapping, tenant)
}

func (registry *cliCatalogRegistry) RegisterTriageRules(
	_ context.Context, tenant domain.TenantID, _ ports.TriageRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.record(commandTriageRules, tenant)
}

func (registry *cliCatalogRegistry) RegisterNotificationPolicy(
	_ context.Context, tenant domain.TenantID, _ ports.NotificationPolicyRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.record(commandNotificationPolicy, tenant)
}

func (registry *cliCatalogRegistry) RegisterClaimEligibility(
	_ context.Context, tenant domain.TenantID, _ ports.ClaimEligibilityRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.record(commandClaimEligibility, tenant)
}

func (registry *cliCatalogRegistry) RegisterClaimAuthorization(
	_ context.Context, tenant domain.TenantID, _ ports.ClaimAuthorizationRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.record(commandClaimAuthorization, tenant)
}

func (registry *cliCatalogRegistry) RegisterDisclosurePolicy(
	_ context.Context, tenant domain.TenantID, _ ports.DisclosurePolicyRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.record(commandDisclosurePolicy, tenant)
}

var _ ports.CatalogRegistry = (*cliCatalogRegistry)(nil)

func newTestRegistrars(t *testing.T, registry ports.CatalogRegistry, tracer executionTracer) registrars {
	t.Helper()
	catalogs, err := application.NewCatalogRegistration(registry)
	if err != nil {
		t.Fatalf("构造登记用例：%v", err)
	}
	return registrars{
		catalogs:   catalogs,
		tracer:     tracer,
		transactor: passthroughTransactor{},
		clock:      fixedClock{at: executeAt},
	}
}

// validInputByCommand 给六命令各一份过得了翻译与用例门的最小输入（隔离合成 S）。
func validInputByCommand() map[string]string {
	versionHeader := `"tenantId":"SYN-TEN-VE15","version":"SYN-V1","approvedBy":"SYN-approver-1",` +
		`"effectiveFrom":"2026-09-01T00:00:00Z"`
	return map[string]string{
		commandMilestoneMapping: `{` + versionHeader + `,"entries":[
			{"source":"PARCEL_SHIPMENT","factKind":"SYN_KIND","milestone":"SYN-MILESTONE"}]}`,
		commandTriageRules: `{` + versionHeader + `,"entries":[
			{"kind":"SYN-SIGNAL","confidence":"SYN-CONF","outcome":"MANUAL_REVIEW"}]}`,
		commandNotificationPolicy: `{"tenantId":"SYN-TEN-VE15","policy":"SYN-POLICY-1",
			"channel":"SYN-CHANNEL","deadlineAfter":"72h","obligation":"SYN-OBLIGATION",
			"approvedBy":"SYN-approver-1"}`,
		commandClaimEligibility: `{"tenantId":"SYN-TEN-VE15","version":"SYN-V1",
			"approvedBy":"SYN-approver-1","contract":"SYN-CONTRACT","coveredKinds":["SYN-KIND-LOSS"]}`,
		commandClaimAuthorization: `{"tenantId":"SYN-TEN-VE15","version":"SYN-V1",
			"approvedBy":"SYN-approver-1","customer":"SYN-CUSTOMER","applicants":["SYN-APPLICANT"]}`,
		commandDisclosurePolicy: `{` + versionHeader + `,"entries":[
			{"customer":"SYN-CUSTOMER","milestones":{"state":"SHOWN","content":"SYN-CONTENT"},
			"eta":{"state":"PENDING_CONFIRMATION"},"final":{"state":"NOT_DISCLOSED"},
			"note":{"state":"NOT_DISCLOSED"}}]}`,
	}
}

// expectedReferenceByCommand 是留痕引用的口径：区间型目录用租户+版本，键型目录用
// 租户+键身份。
var expectedReferenceByCommand = map[string]string{
	commandMilestoneMapping:   "SYN-TEN-VE15/SYN-V1",
	commandTriageRules:        "SYN-TEN-VE15/SYN-V1",
	commandNotificationPolicy: "SYN-TEN-VE15/SYN-POLICY-1",
	commandClaimEligibility:   "SYN-TEN-VE15/SYN-CONTRACT",
	commandClaimAuthorization: "SYN-TEN-VE15/SYN-CUSTOMER",
	commandDisclosurePolicy:   "SYN-TEN-VE15/SYN-V1",
}

// TestExecuteRoutesEachCommandAndTracesTheRegistration 证六命令各自到达对的登记
// 方法，且已登记的执行带着两样通道技术身份与固定时钟落痕。
func TestExecuteRoutesEachCommandAndTracesTheRegistration(t *testing.T) {
	for command, input := range validInputByCommand() {
		t.Run(command, func(t *testing.T) {
			registry := &cliCatalogRegistry{outcome: ports.CatalogVersionRegistered}
			tracer := &traceRecorder{}
			regs := newTestRegistrars(t, registry, tracer)

			message, code := execute(t.Context(), command, []byte(input), testIdentity, regs)
			if code != exitRegistered || !strings.Contains(message, "REGISTERED") {
				t.Fatalf("退出码 = %d（%s）", code, message)
			}
			if len(registry.calls) != 1 || registry.calls[0] != command+"/SYN-TEN-VE15" {
				t.Fatalf("写入口调用 = %v", registry.calls)
			}
			if len(tracer.executions) != 1 {
				t.Fatalf("留痕数 = %d，要 1", len(tracer.executions))
			}
			trace := tracer.executions[0]
			if trace.Command != command ||
				trace.RecordReference != expectedReferenceByCommand[command] ||
				trace.OSUser != testIdentity.osUser ||
				trace.Hostname != testIdentity.hostname ||
				trace.Outcome != "REGISTERED" ||
				!trace.ExecutedAt.Equal(executeAt) {
				t.Fatalf("留痕 = %+v", trace)
			}
		})
	}
}

// TestExecuteUseCaseRefusalExitsUsageWithoutTouchingRegistryOrTrace 证用例缺件拒绝
// 走 1：写入口没被碰、没有留痕——拒绝的登记压根没到册面。
func TestExecuteUseCaseRefusalExitsUsageWithoutTouchingRegistryOrTrace(t *testing.T) {
	registry := &cliCatalogRegistry{outcome: ports.CatalogVersionRegistered}
	tracer := &traceRecorder{}
	regs := newTestRegistrars(t, registry, tracer)

	missingVersion := []byte(`{"tenantId":"SYN-TEN-VE15","version":"","approvedBy":"SYN-approver-1",
		"effectiveFrom":"2026-09-01T00:00:00Z",
		"entries":[{"source":"PARCEL_SHIPMENT","factKind":"SYN_KIND","milestone":"SYN-MILESTONE"}]}`)
	message, code := execute(t.Context(), commandMilestoneMapping, missingVersion, testIdentity, regs)
	if code != exitUsage || !strings.Contains(message, "VERSION_MISSING") {
		t.Fatalf("退出码 = %d（%s），要 1 且指名 VERSION_MISSING", code, message)
	}
	if len(registry.calls) != 0 || len(tracer.executions) != 0 {
		t.Fatalf("缺件拒绝碰了写入口或留痕：calls=%v traces=%d", registry.calls, len(tracer.executions))
	}
}

// TestExecuteRegisterGovernanceAnswersExitGovernanceWithoutTrace 证登记册的两格治理
// 答案（版本不可覆盖、区间重叠）走 2 且不留痕——原行未被顶替，没有行到达登记册。
func TestExecuteRegisterGovernanceAnswersExitGovernanceWithoutTrace(t *testing.T) {
	cases := []struct {
		name    string
		outcome ports.CatalogRegistrationOutcome
		hint    string
	}{
		{"版本不可覆盖", ports.CatalogVersionAlreadyRegistered, "VERSION_NOT_OVERWRITABLE"},
		{"同一时点已有另一适用版本", ports.CatalogVersionOverlapsExisting, "VERSION_OVERLAPS_EXISTING"},
	}
	input := validInputByCommand()[commandMilestoneMapping]
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			registry := &cliCatalogRegistry{outcome: testCase.outcome}
			tracer := &traceRecorder{}
			regs := newTestRegistrars(t, registry, tracer)

			message, code := execute(t.Context(), commandMilestoneMapping, []byte(input), testIdentity, regs)
			if code != exitGovernance || !strings.Contains(message, testCase.hint) {
				t.Fatalf("退出码 = %d（%s），要 2 且指名 %s", code, message, testCase.hint)
			}
			if len(tracer.executions) != 0 {
				t.Fatalf("治理答案留了痕：%+v", tracer.executions)
			}
		})
	}
}

// TestExecuteTraceFailureTurnsUndecided 证留痕失败随事务翻成未决——登记不许在无痕
// 状态下落地。
func TestExecuteTraceFailureTurnsUndecided(t *testing.T) {
	registry := &cliCatalogRegistry{outcome: ports.CatalogVersionRegistered}
	tracer := &traceRecorder{err: errors.New("trace store down")}
	regs := newTestRegistrars(t, registry, tracer)

	input := validInputByCommand()[commandMilestoneMapping]
	message, code := execute(t.Context(), commandMilestoneMapping, []byte(input), testIdentity, regs)
	if code != exitUndecided || !strings.Contains(message, "未决") {
		t.Fatalf("退出码 = %d（%s），要 3", code, message)
	}
}

// TestExecuteRegistryErrorTurnsUndecided 证依赖故障走 3：登记与否未知，重跑续办。
func TestExecuteRegistryErrorTurnsUndecided(t *testing.T) {
	registry := &cliCatalogRegistry{err: errors.New("registry down")}
	tracer := &traceRecorder{}
	regs := newTestRegistrars(t, registry, tracer)

	input := validInputByCommand()[commandMilestoneMapping]
	message, code := execute(t.Context(), commandMilestoneMapping, []byte(input), testIdentity, regs)
	if code != exitUndecided {
		t.Fatalf("退出码 = %d（%s），要 3", code, message)
	}
	if len(tracer.executions) != 0 {
		t.Fatalf("依赖故障留了痕：%+v", tracer.executions)
	}
}

// TestExecuteRejectsCommandsOutsideTheSet 证集合外命令（含别的登记口的命令）点名拒绝。
func TestExecuteRejectsCommandsOutsideTheSet(t *testing.T) {
	registry := &cliCatalogRegistry{outcome: ports.CatalogVersionRegistered}
	regs := newTestRegistrars(t, registry, &traceRecorder{})

	message, code := execute(t.Context(), "suspend", []byte(`{}`), testIdentity, regs)
	if code != exitUsage || !strings.Contains(message, "suspend") {
		t.Fatalf("退出码 = %d（%s），要 1 且点名命令", code, message)
	}
}
