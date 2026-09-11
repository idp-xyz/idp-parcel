package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var (
	credentialGateAsOf     = time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	credentialGateJudgedAt = time.Date(2026, 9, 11, 10, 30, 0, 0, time.UTC)
)

func credentialGateSpec(t *testing.T, conclusion domain.CredentialGateConclusion) domain.CredentialGateSpec {
	t.Helper()
	return domain.CredentialGateSpec{
		Unit:       mustValue(t, domain.NewDeclarationUnitID, "SYN-UNIT-01"),
		Credential: mustValue(t, domain.NewCredentialID, "SYN-CRED-01"),
		Procedure:  mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		Holder:     mustValue(t, domain.NewCredentialHolderReference, "SYN-HOLDER-01"),
		AsOf:       credentialGateAsOf,
		Conclusion: conclusion,
		Basis:      mustValue(t, domain.NewCredentialGateBasisReference, "SYN-CRED-EVIDENCE/2026-09"),
		Role:       mustValue(t, domain.NewResponsibleRoleReference, "SYN-ROLE-CUSTOMS-ASSESSOR"),
		JudgedAt:   credentialGateJudgedAt,
	}
}

// Covers: UC-CC-003 步 7「记录凭证门禁」、AT-CC-056「保存适用性和截至时点」——一条判断把
// 凭证身份、程序、持有人、截至时点、结论、依据引用、责任角色与判断时刻一次固定；读口逐格
// 原样交回，两个时间归一到 UTC。
func TestACredentialGateJudgmentFixesEveryDimensionItWasFormedWith(t *testing.T) {
	spec := credentialGateSpec(t, domain.CredentialGateApplicable)
	spec.AsOf = spec.AsOf.In(time.FixedZone("UTC+8", 8*3600))

	judgment, err := domain.RecordCredentialGate(spec)
	if err != nil {
		t.Fatalf("record credential gate: %v", err)
	}
	if judgment.Unit() != spec.Unit || judgment.Credential() != spec.Credential ||
		judgment.Procedure() != spec.Procedure || judgment.Holder() != spec.Holder ||
		judgment.Basis() != spec.Basis || judgment.Role() != spec.Role {
		t.Fatalf("判断的引用维走样：%+v", judgment)
	}
	if judgment.Conclusion() != domain.CredentialGateApplicable {
		t.Fatalf("conclusion = %v", judgment.Conclusion())
	}
	if !judgment.AsOf().Equal(credentialGateAsOf) || judgment.AsOf().Location() != time.UTC {
		t.Fatalf("截至时点没有归一到 UTC：%v", judgment.AsOf())
	}
	if !judgment.JudgedAt().Equal(credentialGateJudgedAt) || judgment.JudgedAt().Location() != time.UTC {
		t.Fatalf("判断时刻没有归一到 UTC：%v", judgment.JudgedAt())
	}
}

// Covers: 结论封闭四格，与 JudgeCredentialApplicability 的四格同词——「凭证未登记」与「不适用」
// 是两格（票 sa-cc/04 红线：压成一格，租户上线前每一次判断都会读成「凭证不适用」）；集外
// 取值（含零值）构造期拒。
func TestACredentialGateConclusionIsOneOfExactlyFourWords(t *testing.T) {
	words := map[domain.CredentialGateConclusion]string{
		domain.CredentialGateApplicable:              "APPLICABLE",
		domain.CredentialGateNotApplicable:           "NOT_APPLICABLE",
		domain.CredentialGateCredentialNotRegistered: "CREDENTIAL_NOT_REGISTERED",
		domain.CredentialGateUndecided:               "UNDECIDED",
	}
	for conclusion, word := range words {
		if conclusion.String() != word {
			t.Fatalf("%d 的词形 = %q, want %q", conclusion, conclusion.String(), word)
		}
		if _, err := domain.RecordCredentialGate(credentialGateSpec(t, conclusion)); err != nil {
			t.Fatalf("%s 该能落成判断：%v", word, err)
		}
	}
	if domain.CredentialGateCredentialNotRegistered == domain.CredentialGateNotApplicable {
		t.Fatal("「凭证未登记」与「不适用」被压成了一格")
	}

	for name, conclusion := range map[string]domain.CredentialGateConclusion{
		"零值": domain.CredentialGateConclusionInvalid,
		"集外": domain.CredentialGateConclusion(99),
	} {
		if conclusion.String() != "" {
			t.Fatalf("%s该无词形，实得 %q", name, conclusion.String())
		}
		if _, err := domain.RecordCredentialGate(credentialGateSpec(t, conclusion)); !errors.Is(err, domain.ErrInvalidCredentialGate) {
			t.Fatalf("%s结论该拒：%v", name, err)
		}
	}
}

// Covers: 每一维必备——单元、凭证身份、程序、持有人、截至时点、依据引用、责任角色、判断时刻
// 任一缺席即拒。依据引用不可空：UC-CC-003 步 11「逐门禁依据」要审计的正是它；责任角色不可空：
// 范围节「保存每项门禁的……责任角色」。
func TestACredentialGateJudgmentRefusesAnyMissingDimension(t *testing.T) {
	mutations := map[string]func(*domain.CredentialGateSpec){
		"单元":   func(spec *domain.CredentialGateSpec) { spec.Unit = domain.DeclarationUnitID{} },
		"凭证身份": func(spec *domain.CredentialGateSpec) { spec.Credential = domain.CredentialID{} },
		"程序":   func(spec *domain.CredentialGateSpec) { spec.Procedure = domain.CustomsProcedureReference{} },
		"持有人":  func(spec *domain.CredentialGateSpec) { spec.Holder = domain.CredentialHolderReference{} },
		"截至时点": func(spec *domain.CredentialGateSpec) { spec.AsOf = time.Time{} },
		"依据引用": func(spec *domain.CredentialGateSpec) { spec.Basis = domain.CredentialGateBasisReference{} },
		"责任角色": func(spec *domain.CredentialGateSpec) { spec.Role = domain.ResponsibleRoleReference{} },
		"判断时刻": func(spec *domain.CredentialGateSpec) { spec.JudgedAt = time.Time{} },
	}
	for name, mutate := range mutations {
		spec := credentialGateSpec(t, domain.CredentialGateApplicable)
		mutate(&spec)
		if _, err := domain.RecordCredentialGate(spec); !errors.Is(err, domain.ErrInvalidCredentialGate) {
			t.Fatalf("缺%s该拒 ErrInvalidCredentialGate，实得 %v", name, err)
		}
	}
}

// Covers: 依据引用与责任角色是必填引用值——空白串构造期即拒（判据同本包其余 requiredValue）。
func TestCredentialGateReferencesRejectBlankValues(t *testing.T) {
	if _, err := domain.NewCredentialGateBasisReference("   "); err == nil {
		t.Fatal("空白依据引用被接受")
	}
	if _, err := domain.NewResponsibleRoleReference(""); err == nil {
		t.Fatal("空责任角色被接受")
	}
}
