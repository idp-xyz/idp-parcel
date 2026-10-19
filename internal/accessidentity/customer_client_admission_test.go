package accessidentity

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 本文件证首方铸造器准入范围那一步（ADR-0139 决定八）。提交口不在铸造侧判准入——它的生产归属由 UC-PS-001 编排自己
// 判——所以公开的 MintSubmission 走不到这一步；撤回、取消、补充、修订与索赔各口换上首方 Intake 时各带自己那一格
// 准入要求，这里先钉住那一步答哪几格。

type admissionAnswer struct {
	admitted bool
	err      error
}

func (answer admissionAnswer) Admits(context.Context, string, AdmissionRequirement, time.Time) (bool, error) {
	return answer.admitted, answer.err
}

type grantedCustomerVerifier struct{ subject CustomerClientSubject }

func (verifier grantedCustomerVerifier) VerifyCustomerClientCredential(context.Context, IntegrationClientCredential) (VerifiedCustomerClientToken, error) {
	return NewVerifiedCustomerClientToken(verifier.subject, false), nil
}

type grantedCustomerRegistry struct{ standing CustomerClientStanding }

func (registry grantedCustomerRegistry) FindCustomerClient(context.Context, CustomerClientSubject) (CustomerClientStanding, bool, error) {
	return registry.standing, true, nil
}

// Covers: 票 first-party-api/00 完成判据「生产写不在准入范围 403」——要判准入的口：不在范围答 ErrOutsideAdmissionScope，
// 范围读不动答 ErrAdmissionScopeUnavailable、不折成不在范围，在范围内照常铸出。授予要先过：没有授予时不问准入范围。
func TestCustomerClientAdmissionStepAnswersItsOwnGrades(t *testing.T) {
	at := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	subject, err := NewCustomerClientSubject("https://auth.syn.example/customers", "SYN-CUSTOMER-CLIENT-01")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := NewCredentialReference("SYN-CREDENTIAL-REF/customer-01")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := NewCustomerClientBinding(subject, "SYN-TENANT-01", "SYN-ACCOUNT-01", reference, false, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := NewEffectiveInterval(at.Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-01", subject, CustomerCapabilityShipmentRequestSubmission, interval, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := NewRecordedCustomerClientGrant(grant, nil)
	if err != nil {
		t.Fatal(err)
	}
	standing, err := NewCustomerClientStanding(binding, []RecordedCustomerClientGrant{recorded})
	if err != nil {
		t.Fatal(err)
	}
	requirement := &AdmissionRequirement{Capability: "SYN-CAP/customer-write", FactKind: "SYN-FACT/customer-write"}
	request := customerClientRequest{capability: CustomerCapabilityShipmentRequestSubmission, admission: requirement}

	for name, testCase := range map[string]struct {
		admission AdmissionScope
		want      error
	}{
		"outside":    {UnconfiguredAdmissionScope{}, ErrOutsideAdmissionScope},
		"unreadable": {admissionAnswer{err: errors.New("governance register unreachable")}, ErrAdmissionScopeUnavailable},
		"inside":     {admissionAnswer{admitted: true}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			minter, err := NewCustomerClientMinter(grantedCustomerVerifier{subject: subject}, grantedCustomerRegistry{standing: standing},
				testCase.admission, func() time.Time { return at })
			if err != nil {
				t.Fatal(err)
			}
			registration, err := minter.admit(context.Background(), NewIntegrationClientCredential("t", nil), request)
			if testCase.want == nil {
				if err != nil || registration.TenantID() != "SYN-TENANT-01" || registration.CustomerAccountID() != "SYN-ACCOUNT-01" {
					t.Fatalf("在范围内：registration = %+v, err = %v", registration, err)
				}
				return
			}
			if !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want %v", err, testCase.want)
			}
			if errors.Is(err, ErrCustomerClientNotGranted) {
				t.Fatalf("准入那一格折进了未授予：%v", err)
			}
		})
	}

	ungranted, err := NewCustomerClientStanding(binding, nil)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := NewCustomerClientMinter(grantedCustomerVerifier{subject: subject}, grantedCustomerRegistry{standing: ungranted},
		admissionAnswer{err: errors.New("must not be asked")}, func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := minter.admit(context.Background(), NewIntegrationClientCredential("t", nil), request); !errors.Is(err, ErrCustomerClientNotGranted) {
		t.Fatalf("没有授予：err = %v, want ErrCustomerClientNotGranted（不问准入范围）", err)
	}
}

// Covers: ADR-0139 越权风险点「撤回、取消、补充、资料修订、索赔提交各命令的来源请求键推导」待撤回口那一票裁——首方族的
// 撤回推导此刻答推导不出，不拿提交那一格顶上；提交那一格照旧取接入请求标识。
func TestCustomerWithdrawalRequestKeyIsNotDerivedYet(t *testing.T) {
	request := NewChannelRequest(map[string]string{AccessRequestIdentifier: " SYN-ACCESS-REQUEST-0001 "})
	if key, err := (customerAPIRequestKeys{}).DeriveSubmissionRequestKey(context.Background(), request); err != nil || key != "SYN-ACCESS-REQUEST-0001" {
		t.Fatalf("提交：key = %q, err = %v", key, err)
	}
	if key, err := (customerAPIRequestKeys{}).DeriveWithdrawalRequestKey(context.Background(), request); !errors.Is(err, ErrRequestKeyNotDerived) || key != "" {
		t.Fatalf("撤回：key = %q, err = %v, want ErrRequestKeyNotDerived", key, err)
	}
}
