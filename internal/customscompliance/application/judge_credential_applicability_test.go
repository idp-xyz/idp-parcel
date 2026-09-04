package application_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 凭证适用性判断（UC-CC-003 步 7 的判断半边）的行为面：四格分得开——适用、不适用、
// 凭证未登记、未决。「未登记」与「不适用」是两个不同的续办（等登记 vs 等凭证责任流程
// 形成有效依据），压成一格会把实例半边没到读成判断结论。

func applicabilityCommand(t *testing.T) application.JudgeCredentialApplicabilityCommand {
	t.Helper()
	return application.JudgeCredentialApplicabilityCommand{
		TenantID:   configValue(t, domain.NewTenantID, "tenant-a"),
		Credential: configValue(t, domain.NewCredentialID, "SYN-CRED-01"),
		Procedure:  configValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		Holder:     configValue(t, domain.NewCredentialHolderReference, "SYN-HOLDER-01"),
		At:         credentialValidFrom.Add(30 * 24 * time.Hour),
	}
}

func registeredCredentialStore(t *testing.T) *credentialStoreDouble {
	t.Helper()
	store := &credentialStoreDouble{}
	if outcome, err := newCredentialHandler(store).Handle(t.Context(), credentialCommand(t)); err != nil ||
		outcome != application.ConfigurationRegistered {
		t.Fatalf("预登记凭证：err=%v outcome=%v", err, outcome)
	}
	return store
}

func TestARegisteredCredentialIsJudgedApplicableWithinItsTerms(t *testing.T) {
	handler := application.NewJudgeCredentialApplicabilityHandler(application.JudgeCredentialApplicabilityDeps{
		View: registeredCredentialStore(t),
	})

	outcome, err := handler.Handle(t.Context(), applicabilityCommand(t))
	if err != nil || outcome != application.CredentialApplicable {
		t.Fatalf("有效期内同程序同持有人该适用：err=%v outcome=%v", err, outcome)
	}
}

// 程序不符、持有人不符、时点出有效期三者任一即不适用——同名附件在别的程序上用不了，
// 过期凭证谁拿着都不适用（领域 JudgeApplicability 的三维，这里钉编排把三格都翻成同一个
// 不适用而不是吞掉）。
func TestACredentialOutsideItsTermsIsJudgedNotApplicable(t *testing.T) {
	handler := application.NewJudgeCredentialApplicabilityHandler(application.JudgeCredentialApplicabilityDeps{
		View: registeredCredentialStore(t),
	})

	otherProcedure := applicabilityCommand(t)
	otherProcedure.Procedure = configValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-EXPORT")
	otherHolder := applicabilityCommand(t)
	otherHolder.Holder = configValue(t, domain.NewCredentialHolderReference, "SYN-HOLDER-02")
	expired := applicabilityCommand(t)
	expired.At = credentialValidFrom.Add(2 * 365 * 24 * time.Hour)

	for name, command := range map[string]application.JudgeCredentialApplicabilityCommand{
		"程序不符":  otherProcedure,
		"持有人不符": otherHolder,
		"已过期":   expired,
	} {
		outcome, err := handler.Handle(t.Context(), command)
		if err != nil || outcome != application.CredentialNotApplicable {
			t.Fatalf("%s该不适用：err=%v outcome=%v", name, err, outcome)
		}
	}
}

func TestAnUnregisteredCredentialIsNotJudgedAtAll(t *testing.T) {
	handler := application.NewJudgeCredentialApplicabilityHandler(application.JudgeCredentialApplicabilityDeps{
		View: &credentialStoreDouble{},
	})

	outcome, err := handler.Handle(t.Context(), applicabilityCommand(t))
	if err != nil || outcome != application.CredentialNotRegistered {
		t.Fatalf("未登记该答`凭证未登记`而不是不适用：err=%v outcome=%v", err, outcome)
	}
}

// 受理门：租户、凭证身份、程序、持有人、时点任一缺席不受理——判断不出的输入不该被翻成
// 「不适用」。
func TestCredentialApplicabilityRefusesBlankInputs(t *testing.T) {
	handler := application.NewJudgeCredentialApplicabilityHandler(application.JudgeCredentialApplicabilityDeps{
		View: registeredCredentialStore(t),
	})

	blankTenant := applicabilityCommand(t)
	blankTenant.TenantID = domain.TenantID{}
	blankCredential := applicabilityCommand(t)
	blankCredential.Credential = domain.CredentialID{}
	zeroAt := applicabilityCommand(t)
	zeroAt.At = time.Time{}

	for name, command := range map[string]application.JudgeCredentialApplicabilityCommand{
		"租户": blankTenant,
		"凭证": blankCredential,
		"时点": zeroAt,
	} {
		outcome, err := handler.Handle(t.Context(), command)
		if err != nil || outcome != application.CredentialApplicabilityNotAccepted {
			t.Fatalf("缺%s该不受理：err=%v outcome=%v", name, err, outcome)
		}
	}
}

func TestCredentialApplicabilityDependencyFailureIsUndecided(t *testing.T) {
	handler := application.NewJudgeCredentialApplicabilityHandler(application.JudgeCredentialApplicabilityDeps{
		View: &credentialStoreDouble{loadErr: errors.New("view unavailable")},
	})

	outcome, err := handler.Handle(t.Context(), applicabilityCommand(t))
	if err != nil || outcome != application.CredentialApplicabilityUndecided {
		t.Fatalf("读口故障该未决：err=%v outcome=%v", err, outcome)
	}
}
