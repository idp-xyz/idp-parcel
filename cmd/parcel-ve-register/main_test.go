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

// 本文件证登记口的编排：目录册六命令与归集面两命令各自路由到对的登记方法、退出码
// 翻译（两族答案代数不同——归集面有幂等重放格）、身份双轨各行其道（②登记内容不得
// 冒充①通道身份）、只有真正落库的执行留痕、留痕失败随事务翻成未决。用例的缺件判据
// 在应用层已证，这里接真用例配写入口替身，证的是登记口把它们接对。

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

func (registry *cliCatalogRegistry) RegisterExceptionDisclosureRules(
	_ context.Context, tenant domain.TenantID, _ ports.ExceptionDisclosureRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.record(commandExceptionDisclosureRules, tenant)
}

func (registry *cliCatalogRegistry) RegisterConflictSignalRule(
	_ context.Context, tenant domain.TenantID, _ ports.ConflictSignalRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	return registry.record(commandConflictSignalRule, tenant)
}

var _ application.CatalogRegistries = (*cliCatalogRegistry)(nil)

func newTestRegistrars(t *testing.T, registry application.CatalogRegistries, tracer executionTracer) registrars {
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

// validInputByCommand 给目录册各命令一份过得了翻译与用例门的最小输入（隔离合成 S）。
func validInputByCommand() map[string]string {
	versionHeader := `"tenantId":"SYN-TEN-VE15","version":"SYN-V1","approvedBy":"SYN-approver-1",` +
		`"effectiveFrom":"2026-09-01T00:00:00Z"`
	return map[string]string{
		commandExceptionDisclosureRules: `{` + versionHeader + `,"entries":[
			{"customer":"SYN-CUSTOMER","signalKind":"SYN-SIGNAL","confidence":"SYN-CONF",
			"disclosable":true,"autoRelease":false,"content":"SYN-CONTENT"}]}`,
		commandConflictSignalRule: `{"tenantId":"SYN-TEN-VE15","signalKind":"SYN-SIGNAL-CONFLICT",
			"version":"SYN-RULE-V1","confidence":"SYN-CONF","approvedBy":"SYN-approver-1"}`,
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
// 租户+键身份；冲突信号规则的键只有租户，痕上再带识别规则版本（理由见 translateCommand）。
var expectedReferenceByCommand = map[string]string{
	commandMilestoneMapping:         "SYN-TEN-VE15/SYN-V1",
	commandTriageRules:              "SYN-TEN-VE15/SYN-V1",
	commandNotificationPolicy:       "SYN-TEN-VE15/SYN-POLICY-1",
	commandClaimEligibility:         "SYN-TEN-VE15/SYN-CONTRACT",
	commandClaimAuthorization:       "SYN-TEN-VE15/SYN-CUSTOMER",
	commandDisclosurePolicy:         "SYN-TEN-VE15/SYN-V1",
	commandExceptionDisclosureRules: "SYN-TEN-VE15/SYN-V1",
	commandConflictSignalRule:       "SYN-TEN-VE15/SYN-RULE-V1",
}

// TestExecuteRoutesEachCommandAndTracesTheRegistration 证目录册各命令各自到达对的登记
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

// TestExecuteRuleRegistriesFollowTheCatalogueExitCodes 证两册规则命令（票
// ve-disclosure-policy-view/02 步一）走目录册同一套退出码：治理答案 2 不留痕、用例缺件 1
// 不碰写入口、翻译拒收（未知字段）1、依赖故障 3。绿路径与留痕在
// TestExecuteRoutesEachCommandAndTracesTheRegistration 的表里随其余目录命令一并证。
func TestExecuteRuleRegistriesFollowTheCatalogueExitCodes(t *testing.T) {
	inputs := validInputByCommand()

	t.Run("治理答案走 2 不留痕", func(t *testing.T) {
		for _, testCase := range []struct {
			command string
			outcome ports.CatalogRegistrationOutcome
			hint    string
		}{
			{commandExceptionDisclosureRules, ports.CatalogVersionAlreadyRegistered, "VERSION_NOT_OVERWRITABLE"},
			{commandExceptionDisclosureRules, ports.CatalogVersionOverlapsExisting, "VERSION_OVERLAPS_EXISTING"},
			// 冲突信号规则一租户一条：撞既有行是它唯一的治理答案（0025），没有区间可重叠。
			{commandConflictSignalRule, ports.CatalogVersionAlreadyRegistered, "VERSION_NOT_OVERWRITABLE"},
		} {
			registry := &cliCatalogRegistry{outcome: testCase.outcome}
			tracer := &traceRecorder{}
			regs := newTestRegistrars(t, registry, tracer)

			message, code := execute(t.Context(), testCase.command, []byte(inputs[testCase.command]), testIdentity, regs)
			if code != exitGovernance || !strings.Contains(message, testCase.hint) {
				t.Fatalf("%s 退出码 = %d（%s），要 2 且指名 %s", testCase.command, code, message, testCase.hint)
			}
			if len(tracer.executions) != 0 {
				t.Fatalf("%s 治理答案留了痕：%+v", testCase.command, tracer.executions)
			}
		}
	})

	t.Run("用例缺件走 1 不碰写入口", func(t *testing.T) {
		for _, testCase := range []struct {
			command string
			input   string
			hint    string
		}{
			{commandExceptionDisclosureRules,
				`{"tenantId":"SYN-TEN-VE15","version":"","approvedBy":"SYN-approver-1",
				"effectiveFrom":"2026-09-01T00:00:00Z","entries":[
				{"customer":"SYN-CUSTOMER","signalKind":"SYN-SIGNAL","confidence":"SYN-CONF",
				"disclosable":true,"autoRelease":false,"content":"SYN-CONTENT"}]}`,
				"VERSION_MISSING"},
			// 0023 的成对纪律在用例门拒：声明披露却没有内容来处。
			{commandExceptionDisclosureRules,
				`{"tenantId":"SYN-TEN-VE15","version":"SYN-V1","approvedBy":"SYN-approver-1",
				"effectiveFrom":"2026-09-01T00:00:00Z","entries":[
				{"customer":"SYN-CUSTOMER","signalKind":"SYN-SIGNAL","confidence":"SYN-CONF",
				"disclosable":true,"autoRelease":false}]}`,
				"ENTRY_INCOMPLETE"},
			{commandConflictSignalRule,
				`{"tenantId":"SYN-TEN-VE15","signalKind":"SYN-SIGNAL-CONFLICT","version":"SYN-RULE-V1",
				"confidence":"SYN-CONF","approvedBy":""}`,
				"APPROVAL_MISSING"},
		} {
			registry := &cliCatalogRegistry{outcome: ports.CatalogVersionRegistered}
			tracer := &traceRecorder{}
			regs := newTestRegistrars(t, registry, tracer)

			message, code := execute(t.Context(), testCase.command, []byte(testCase.input), testIdentity, regs)
			if code != exitUsage || !strings.Contains(message, testCase.hint) {
				t.Fatalf("%s 退出码 = %d（%s），要 1 且指名 %s", testCase.command, code, message, testCase.hint)
			}
			if len(registry.calls) != 0 || len(tracer.executions) != 0 {
				t.Fatalf("%s 缺件拒绝碰了写入口或留痕：calls=%v traces=%d",
					testCase.command, registry.calls, len(tracer.executions))
			}
		}
	})

	t.Run("未知字段在翻译处拒收走 1", func(t *testing.T) {
		for _, testCase := range []struct {
			command string
			input   string
		}{
			// 打错字段名（kind 而非 signalKind）不得静默变成「没给」再被用例当缺件拒。
			{commandExceptionDisclosureRules,
				`{"tenantId":"SYN-TEN-VE15","version":"SYN-V1","approvedBy":"SYN-approver-1",
				"effectiveFrom":"2026-09-01T00:00:00Z","entries":[
				{"customer":"SYN-CUSTOMER","kind":"SYN-SIGNAL","confidence":"SYN-CONF",
				"disclosable":false,"autoRelease":false}]}`},
			{commandConflictSignalRule,
				`{"tenantId":"SYN-TEN-VE15","signalKind":"SYN-SIGNAL-CONFLICT","ruleVersion":"SYN-RULE-V1",
				"confidence":"SYN-CONF","approvedBy":"SYN-approver-1"}`},
		} {
			registry := &cliCatalogRegistry{outcome: ports.CatalogVersionRegistered}
			regs := newTestRegistrars(t, registry, &traceRecorder{})

			message, code := execute(t.Context(), testCase.command, []byte(testCase.input), testIdentity, regs)
			if code != exitUsage || !strings.Contains(message, "输入被拒") {
				t.Fatalf("%s 退出码 = %d（%s），要 1 且报输入被拒", testCase.command, code, message)
			}
			if len(registry.calls) != 0 {
				t.Fatalf("%s 翻译拒收后仍碰了写入口：%v", testCase.command, registry.calls)
			}
		}
	})

	t.Run("依赖故障走 3", func(t *testing.T) {
		for _, command := range []string{commandExceptionDisclosureRules, commandConflictSignalRule} {
			registry := &cliCatalogRegistry{err: errors.New("registry down")}
			tracer := &traceRecorder{}
			regs := newTestRegistrars(t, registry, tracer)

			message, code := execute(t.Context(), command, []byte(inputs[command]), testIdentity, regs)
			if code != exitUndecided {
				t.Fatalf("%s 退出码 = %d（%s），要 3", command, code, message)
			}
			if len(tracer.executions) != 0 {
				t.Fatalf("%s 依赖故障留了痕：%+v", command, tracer.executions)
			}
		}
	})
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

// cliReceiptRegistry 是归集面写入口替身，形状随 cliCatalogRegistry。
type cliReceiptRegistry struct {
	outcome ports.MaterialReceiptWriteOutcome
	err     error
	calls   []string
}

func (registry *cliReceiptRegistry) RegisterReceipt(
	_ context.Context, tenant domain.TenantID, _ ports.MaterialReceipt,
) (ports.MaterialReceiptWriteOutcome, error) {
	registry.calls = append(registry.calls, commandMaterialReceipt+"/"+tenant.String())
	return registry.outcome, registry.err
}

func (registry *cliReceiptRegistry) RevokeReceipt(
	_ context.Context, tenant domain.TenantID, _ ports.MaterialReceiptRevocation,
) (ports.MaterialReceiptWriteOutcome, error) {
	registry.calls = append(registry.calls, commandMaterialReceiptRevocation+"/"+tenant.String())
	return registry.outcome, registry.err
}

var _ ports.MaterialReceiptRegistry = (*cliReceiptRegistry)(nil)

// newReceiptTestRegistrars 只装归集面那半（catalogs 留空）：路由在 execute 已分岔，
// 归集面命令碰不到目录用例。
func newReceiptTestRegistrars(t *testing.T, registry ports.MaterialReceiptRegistry, tracer executionTracer) registrars {
	t.Helper()
	receipts, err := application.NewMaterialReceiptRegistration(registry)
	if err != nil {
		t.Fatalf("构造收讫登记用例：%v", err)
	}
	return registrars{
		receipts:   receipts,
		tracer:     tracer,
		transactor: passthroughTransactor{},
		clock:      fixedClock{at: executeAt},
	}
}

const validReceiptInput = `{"tenantId":"SYN-TEN-VE15","batch":"SYN-BATCH-1","item":"SYN-ITEM-1",
	"material":"SYN-MAT-PHOTO","receivedAt":"2026-08-20T08:00:00Z","receivedBy":"SYN-operator-9"}`

const validRevocationInput = `{"tenantId":"SYN-TEN-VE15","batch":"SYN-BATCH-1","item":"SYN-ITEM-1",
	"material":"SYN-MAT-PHOTO","receivedAt":"2026-08-20T08:00:00Z",
	"revokedBy":"SYN-supervisor-1","revokedAt":"2026-08-22T10:00:00Z"}`

// 归集面留痕引用是五件行身份连串，收讫时刻带满精度。
const expectedReceiptReference = "SYN-TEN-VE15/SYN-BATCH-1/SYN-ITEM-1/SYN-MAT-PHOTO@2026-08-20T08:00:00Z"

// TestExecuteRoutesReceiptCommandsAndTracesLandedWrites 证归集面两命令各自到达对的
// 写方法，真正落库的两格（REGISTERED / REVOKED）带通道身份落痕。
func TestExecuteRoutesReceiptCommandsAndTracesLandedWrites(t *testing.T) {
	for _, testCase := range []struct {
		command string
		input   string
		outcome ports.MaterialReceiptWriteOutcome
		want    string
	}{
		{commandMaterialReceipt, validReceiptInput, ports.MaterialReceiptRecorded, "REGISTERED"},
		{commandMaterialReceiptRevocation, validRevocationInput, ports.MaterialReceiptRevocationRecorded, "REVOKED"},
	} {
		t.Run(testCase.command, func(t *testing.T) {
			registry := &cliReceiptRegistry{outcome: testCase.outcome}
			tracer := &traceRecorder{}
			regs := newReceiptTestRegistrars(t, registry, tracer)

			message, code := execute(t.Context(), testCase.command, []byte(testCase.input), testIdentity, regs)
			if code != exitRegistered || !strings.Contains(message, testCase.want) {
				t.Fatalf("退出码 = %d（%s），要 0 且指名 %s", code, message, testCase.want)
			}
			if len(registry.calls) != 1 || registry.calls[0] != testCase.command+"/SYN-TEN-VE15" {
				t.Fatalf("写入口调用 = %v", registry.calls)
			}
			if len(tracer.executions) != 1 {
				t.Fatalf("留痕数 = %d，要 1", len(tracer.executions))
			}
			trace := tracer.executions[0]
			if trace.Command != testCase.command ||
				trace.RecordReference != expectedReceiptReference ||
				trace.OSUser != testIdentity.osUser ||
				trace.Hostname != testIdentity.hostname ||
				trace.Outcome != testCase.want ||
				!trace.ExecutedAt.Equal(executeAt) {
				t.Fatalf("留痕 = %+v", trace)
			}
		})
	}
}

// TestExecuteReceiptReplayExitsZeroWithoutTrace 证幂等重放走 0 且不留痕：答案已指名
// 本次没有写入（ALREADY_*），续办不被读成失败，痕也不声称一笔没落库的登记。
func TestExecuteReceiptReplayExitsZeroWithoutTrace(t *testing.T) {
	for _, testCase := range []struct {
		command string
		input   string
		outcome ports.MaterialReceiptWriteOutcome
		want    string
	}{
		{commandMaterialReceipt, validReceiptInput, ports.MaterialReceiptAlreadyRecorded, "ALREADY_REGISTERED"},
		{commandMaterialReceiptRevocation, validRevocationInput, ports.MaterialReceiptRevocationAlreadyRecorded, "ALREADY_REVOKED"},
	} {
		t.Run(testCase.command, func(t *testing.T) {
			registry := &cliReceiptRegistry{outcome: testCase.outcome}
			tracer := &traceRecorder{}
			regs := newReceiptTestRegistrars(t, registry, tracer)

			message, code := execute(t.Context(), testCase.command, []byte(testCase.input), testIdentity, regs)
			if code != exitRegistered || !strings.Contains(message, testCase.want) {
				t.Fatalf("退出码 = %d（%s），要 0 且指名 %s", code, message, testCase.want)
			}
			if len(tracer.executions) != 0 {
				t.Fatalf("重放留了痕：%+v", tracer.executions)
			}
		})
	}
}

// TestExecuteReceiptRefusalsSplitByRecoveryAction 证归集面拒绝按恢复动作分路：无从
// 撤销是登记册治理答案（2，人工核对收讫引用）；缺经手声明要改输入（1）。两路都不碰
// 留痕，缺件那路连写入口都不碰。
func TestExecuteReceiptRefusalsSplitByRecoveryAction(t *testing.T) {
	t.Run("无从撤销走 2", func(t *testing.T) {
		registry := &cliReceiptRegistry{outcome: ports.MaterialReceiptUnknown}
		tracer := &traceRecorder{}
		regs := newReceiptTestRegistrars(t, registry, tracer)

		message, code := execute(t.Context(), commandMaterialReceiptRevocation,
			[]byte(validRevocationInput), testIdentity, regs)
		if code != exitGovernance || !strings.Contains(message, "RECEIPT_NOT_FOUND") {
			t.Fatalf("退出码 = %d（%s），要 2 且指名 RECEIPT_NOT_FOUND", code, message)
		}
		if len(tracer.executions) != 0 {
			t.Fatalf("治理答案留了痕：%+v", tracer.executions)
		}
	})

	t.Run("缺经手声明走 1", func(t *testing.T) {
		registry := &cliReceiptRegistry{outcome: ports.MaterialReceiptRecorded}
		tracer := &traceRecorder{}
		regs := newReceiptTestRegistrars(t, registry, tracer)

		missing := `{"tenantId":"SYN-TEN-VE15","batch":"SYN-BATCH-1","item":"SYN-ITEM-1",
			"material":"SYN-MAT-PHOTO","receivedAt":"2026-08-20T08:00:00Z","receivedBy":""}`
		message, code := execute(t.Context(), commandMaterialReceipt, []byte(missing), testIdentity, regs)
		if code != exitUsage || !strings.Contains(message, "RESPONSIBLE_MISSING") {
			t.Fatalf("退出码 = %d（%s），要 1 且指名 RESPONSIBLE_MISSING", code, message)
		}
		if len(registry.calls) != 0 || len(tracer.executions) != 0 {
			t.Fatalf("缺件拒绝碰了写入口或留痕：calls=%v traces=%d", registry.calls, len(tracer.executions))
		}
	})
}

// TestExecuteReceiptTraceFailureTurnsUndecided 证归集面路的留痕失败同样随事务翻成
// 未决——两族命令共守「登记不许在无痕状态下落地」。
func TestExecuteReceiptTraceFailureTurnsUndecided(t *testing.T) {
	registry := &cliReceiptRegistry{outcome: ports.MaterialReceiptRecorded}
	tracer := &traceRecorder{err: errors.New("trace store down")}
	regs := newReceiptTestRegistrars(t, registry, tracer)

	message, code := execute(t.Context(), commandMaterialReceipt, []byte(validReceiptInput), testIdentity, regs)
	if code != exitUndecided || !strings.Contains(message, "未决") {
		t.Fatalf("退出码 = %d（%s），要 3", code, message)
	}
}
