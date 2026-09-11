package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 凭证门禁判断编排（UC-CC-003 步 7「记录凭证门禁」，票 sa-cc/04）的行为面：四格各落一版、同键
// 同内容重放`已存在`、换内容追加新版不覆盖、受理门逐格拒、登记册故障折未决、构造门逐口拒 nil。
// 替身照真库代数——键含指纹，撞键只答`已登记`。

var credentialGateJudgedAt = time.Date(2026, 9, 11, 10, 30, 0, 0, time.UTC)

type credentialGateRegistryDouble struct {
	rows        []ports.CredentialGateRecord
	registerErr error
}

func (double *credentialGateRegistryDouble) RegisterCredentialGate(
	_ context.Context,
	record ports.CredentialGateRecord,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.registerErr
	}
	for _, row := range double.rows {
		if row.Key == record.Key {
			return ports.CaseConfigurationAlreadyRegistered, nil
		}
	}
	double.rows = append(double.rows, record)
	return ports.CaseConfigurationRegistered, nil
}

func (double *credentialGateRegistryDouble) LoadCredentialGate(
	_ context.Context,
	key ports.CredentialGateKey,
) (ports.CredentialGateRecord, bool, error) {
	for _, row := range double.rows {
		if row.Key == key {
			return row, true, nil
		}
	}
	return ports.CredentialGateRecord{}, false, nil
}

func credentialGateDeps(credentials *credentialStoreDouble, registry *credentialGateRegistryDouble) application.RecordCredentialGateDeps {
	return application.RecordCredentialGateDeps{
		Credentials: credentials,
		Registry:    registry,
		Clock:       fixedClock{at: credentialGateJudgedAt},
	}
}

func newCredentialGateHandler(t *testing.T, credentials *credentialStoreDouble, registry *credentialGateRegistryDouble) *application.RecordCredentialGateHandler {
	t.Helper()
	handler, err := application.NewRecordCredentialGateHandler(credentialGateDeps(credentials, registry))
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	return handler
}

func credentialGateCommand(t *testing.T) application.RecordCredentialGateCommand {
	t.Helper()
	judge := applicabilityCommand(t)
	return application.RecordCredentialGateCommand{
		TenantID:   judge.TenantID,
		Unit:       configValue(t, domain.NewDeclarationUnitID, "SYN-UNIT-01"),
		Credential: judge.Credential,
		Procedure:  judge.Procedure,
		Holder:     judge.Holder,
		AsOf:       judge.At,
		Basis:      configValue(t, domain.NewCredentialGateBasisReference, "SYN-CRED-EVIDENCE/2026-09"),
		Role:       configValue(t, domain.NewResponsibleRoleReference, "SYN-ROLE-CUSTOMS-ASSESSOR"),
	}
}

func recorded(t *testing.T, result application.CredentialGateResult, err error, want domain.CredentialGateConclusion) {
	t.Helper()
	if err != nil || result.Outcome() != application.CredentialGateRecorded {
		t.Fatalf("该落成新版：err=%v outcome=%v", err, result.Outcome())
	}
	if result.Conclusion() != want {
		t.Fatalf("conclusion = %v, want %v", result.Conclusion(), want)
	}
	if result.Key().Digest == "" {
		t.Fatal("落成的版本没有交回可绑定的键")
	}
}

// Covers: AT-CC-056——有效期内同程序同持有人的在册凭证判`适用`并落成一版：凭证身份、程序、
// 持有人、截至时点（拟使用时点，不是判断时刻）、依据引用、责任角色逐格如实，判断时刻取时钟；
// 交回的键指向刚落的那一版。本用例不占用或核销额度——凭证册上那一行纹丝不动。
func TestAnApplicableCredentialGateIsJudgedAndRecorded(t *testing.T) {
	credentials := registeredCredentialStore(t)
	registry := &credentialGateRegistryDouble{}
	command := credentialGateCommand(t)

	result, err := newCredentialGateHandler(t, credentials, registry).Handle(t.Context(), command)
	recorded(t, result, err, domain.CredentialGateApplicable)

	if len(registry.rows) != 1 {
		t.Fatalf("册上行数走样：%d", len(registry.rows))
	}
	row := registry.rows[0]
	if row.Key != result.Key() ||
		row.Key.TenantID != command.TenantID || row.Key.Unit != command.Unit || row.Key.Credential != command.Credential {
		t.Fatalf("键走样：%+v vs %+v", row.Key, result.Key())
	}
	judgment := row.Judgment
	if judgment.Unit() != command.Unit || judgment.Credential() != command.Credential ||
		judgment.Procedure() != command.Procedure || judgment.Holder() != command.Holder ||
		!judgment.AsOf().Equal(command.AsOf) || judgment.Basis() != command.Basis || judgment.Role() != command.Role ||
		judgment.Conclusion() != domain.CredentialGateApplicable {
		t.Fatalf("判断内容走样：%+v", judgment)
	}
	if !judgment.JudgedAt().Equal(credentialGateJudgedAt) {
		t.Fatalf("判断时刻该取时钟 %v，实得 %v", credentialGateJudgedAt, judgment.JudgedAt())
	}
	if len(credentials.rows) != 1 {
		t.Fatalf("凭证册被动过：%d 行", len(credentials.rows))
	}
	if uses, provided := credentials.rows[0].credential.Uses(); uses != 12 || !provided {
		t.Fatalf("额度被占用或核销了：uses=%d provided=%v", uses, provided)
	}
}

// Covers: 四格各一条落库路径——不适用（程序不符）、凭证未登记、未决（读凭证册故障）各落成
// 自己那一格；「凭证未登记」与「不适用」两格分立，不压成一格。
func TestEveryCredentialGateConclusionHasItsOwnRecordedWord(t *testing.T) {
	otherProcedure := credentialGateCommand(t)
	otherProcedure.Procedure = configValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-EXPORT")

	cases := []struct {
		name        string
		credentials *credentialStoreDouble
		command     application.RecordCredentialGateCommand
		want        domain.CredentialGateConclusion
	}{
		{"不适用", registeredCredentialStore(t), otherProcedure, domain.CredentialGateNotApplicable},
		{"凭证未登记", &credentialStoreDouble{}, credentialGateCommand(t), domain.CredentialGateCredentialNotRegistered},
		{"未决", &credentialStoreDouble{loadErr: errors.New("view unavailable")}, credentialGateCommand(t), domain.CredentialGateUndecided},
	}
	for _, testCase := range cases {
		registry := &credentialGateRegistryDouble{}
		result, err := newCredentialGateHandler(t, testCase.credentials, registry).Handle(t.Context(), testCase.command)
		recorded(t, result, err, testCase.want)
		if len(registry.rows) != 1 || registry.rows[0].Judgment.Conclusion() != testCase.want {
			t.Fatalf("%s：册上没有落成 %v：%+v", testCase.name, testCase.want, registry.rows)
		}
	}
	if domain.CredentialGateCredentialNotRegistered == domain.CredentialGateNotApplicable {
		t.Fatal("「凭证未登记」与「不适用」被压成了一格")
	}
}

// Covers: 同键同内容重放`已存在`且交回同一把键（时钟走了不算换内容）；换内容（这里换依据引用）
// 追加新版、不覆盖——两版并存，首版内容原样。
func TestReplayingACredentialGateSplitsExistingFromANewVersion(t *testing.T) {
	credentials := registeredCredentialStore(t)
	registry := &credentialGateRegistryDouble{}
	deps := credentialGateDeps(credentials, registry)
	handler, err := application.NewRecordCredentialGateHandler(deps)
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}

	first, err := handler.Handle(t.Context(), credentialGateCommand(t))
	recorded(t, first, err, domain.CredentialGateApplicable)

	deps.Clock = fixedClock{at: credentialGateJudgedAt.Add(time.Hour)}
	later, err := application.NewRecordCredentialGateHandler(deps)
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	replay, err := later.Handle(t.Context(), credentialGateCommand(t))
	if err != nil || replay.Outcome() != application.CredentialGateExisting {
		t.Fatalf("重放该是`已存在`：err=%v outcome=%v", err, replay.Outcome())
	}
	if replay.Key() != first.Key() || replay.Conclusion() != domain.CredentialGateApplicable {
		t.Fatalf("重放交回的键或结论走样：%+v vs %+v", replay.Key(), first.Key())
	}
	if len(registry.rows) != 1 {
		t.Fatalf("重放长出了第二版：%d 行", len(registry.rows))
	}

	changedBasis := credentialGateCommand(t)
	changedBasis.Basis = configValue(t, domain.NewCredentialGateBasisReference, "SYN-CRED-EVIDENCE/2026-10")
	second, err := handler.Handle(t.Context(), changedBasis)
	recorded(t, second, err, domain.CredentialGateApplicable)
	if second.Key() == first.Key() {
		t.Fatal("换依据没有换版")
	}
	if len(registry.rows) != 2 || registry.rows[0].Judgment.Basis().String() != "SYN-CRED-EVIDENCE/2026-09" {
		t.Fatalf("新版覆盖了首版：%+v", registry.rows)
	}
}

// Covers: 受理门逐格拒——租户、单元、依据引用、责任角色本编排自己守；凭证身份、程序、持有人、
// 截至时点由判断口的受理门守、这里翻成同一格`未受理`。被拒的请求不落册：判不出的输入不该被翻成
// 任何一格结论。
func TestRecordingACredentialGateRefusesBlankInputs(t *testing.T) {
	credentials := registeredCredentialStore(t)
	registry := &credentialGateRegistryDouble{}
	handler := newCredentialGateHandler(t, credentials, registry)

	mutations := map[string]func(*application.RecordCredentialGateCommand){
		"租户": func(command *application.RecordCredentialGateCommand) { command.TenantID = domain.TenantID{} },
		"单元": func(command *application.RecordCredentialGateCommand) { command.Unit = domain.DeclarationUnitID{} },
		"依据引用": func(command *application.RecordCredentialGateCommand) {
			command.Basis = domain.CredentialGateBasisReference{}
		},
		"责任角色": func(command *application.RecordCredentialGateCommand) {
			command.Role = domain.ResponsibleRoleReference{}
		},
		"凭证身份": func(command *application.RecordCredentialGateCommand) { command.Credential = domain.CredentialID{} },
		"程序": func(command *application.RecordCredentialGateCommand) {
			command.Procedure = domain.CustomsProcedureReference{}
		},
		"持有人": func(command *application.RecordCredentialGateCommand) {
			command.Holder = domain.CredentialHolderReference{}
		},
		"截至时点": func(command *application.RecordCredentialGateCommand) { command.AsOf = time.Time{} },
	}
	for name, mutate := range mutations {
		command := credentialGateCommand(t)
		mutate(&command)
		result, err := handler.Handle(t.Context(), command)
		if err != nil || result.Outcome() != application.CredentialGateNotAccepted {
			t.Fatalf("缺%s该不受理：err=%v outcome=%v", name, err, result.Outcome())
		}
	}
	if len(registry.rows) != 0 {
		t.Fatal("被拒的请求落了册")
	}
}

// Covers: 登记册故障折`未决`，不交回键——没落成的判断不能被当成可绑定的依据。
func TestCredentialGateRegistryFailureIsUndecided(t *testing.T) {
	registry := &credentialGateRegistryDouble{registerErr: errors.New("registry unavailable")}

	result, err := newCredentialGateHandler(t, registeredCredentialStore(t), registry).Handle(t.Context(), credentialGateCommand(t))
	if err != nil || result.Outcome() != application.CredentialGateRecordUndecided {
		t.Fatalf("登记册故障该未决：err=%v outcome=%v", err, result.Outcome())
	}
	if result.Key() != (ports.CredentialGateKey{}) {
		t.Fatalf("未落成却交回了键：%+v", result.Key())
	}
}

// 构造门：RecordCredentialGateDeps 每一口各缺一次，构造期就以具名错误停下（形照
// NewDutyPaymentReconciliationHandler）。表里的口要与 Deps 的字段一一对上——加口不加表，这里不会红。
func TestTheCredentialGateHandlerNamesWhichDependencyIsMissing(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*application.RecordCredentialGateDeps)
	}{
		{"credential view", func(deps *application.RecordCredentialGateDeps) { deps.Credentials = nil }},
		{"credential gate registry", func(deps *application.RecordCredentialGateDeps) { deps.Registry = nil }},
		{"clock", func(deps *application.RecordCredentialGateDeps) { deps.Clock = nil }},
	}
	for _, testCase := range cases {
		deps := credentialGateDeps(&credentialStoreDouble{}, &credentialGateRegistryDouble{})
		testCase.mutate(&deps)
		handler, err := application.NewRecordCredentialGateHandler(deps)
		if !errors.Is(err, application.ErrNilDependency) {
			t.Fatalf("缺 %s：err = %v, want ErrNilDependency", testCase.name, err)
		}
		if !strings.Contains(err.Error(), testCase.name) {
			t.Fatalf("缺 %s：错误没点名那一口：%v", testCase.name, err)
		}
		if handler != nil {
			t.Fatalf("缺 %s：拒了还交出编排", testCase.name)
		}
	}
}
