package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

// Covers: ADR-0363 决定二——操作者接入管理面的四口接在操作者铸造器上、守口是操作者接入管理这一格：发行方未配置答未配置；没令牌答
// 令牌不过；只持登记册配置写或查阅读答未授予；授予齐备而册读失败答答案未形成；空册答空列表并回本进程信任的发行方。
func TestOperatorAccessAdministrationAnswersFromTheOperatorChannel(t *testing.T) {
	for path, constant := range map[string]string{
		"/operators": identityhttp.OperatorsPath, "/operator-registrations": identityhttp.OperatorRegistrationsPath,
		"/operator-grant-registrations": identityhttp.OperatorGrantRegistrationsPath, "/operator-grant-revocations": identityhttp.OperatorGrantRevocationsPath,
	} {
		if constant != path {
			t.Fatalf("路径常量 = %s，端点表字面量 %s 对不上", constant, path)
		}
	}
	granted, err := buildOperatorMinter(acceptingVerifier{}, accessAdministratorRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	writer, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	unconfigured, err := buildOperatorMinter(accessidentity.UnconfiguredOperatorCredentialVerifier{}, accessAdministratorRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		minter    *accessidentity.OperatorMinter
		catalogue accessidentity.OperatorCatalogue
		token     string
		status    int
		code      string
		outcome   string
	}{
		{"issuer unconfigured", unconfigured, emptyOperatorCatalogue{}, "presented.operator.token", http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED", ""},
		{"no bearer token", granted, emptyOperatorCatalogue{}, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED", ""},
		{"write grant is not an administration grant", writer, emptyOperatorCatalogue{}, "presented.operator.token", http.StatusForbidden, "OPERATOR_NOT_GRANTED", ""},
		{"read grant is not an administration grant", reader, emptyOperatorCatalogue{}, "presented.operator.token", http.StatusForbidden, "OPERATOR_NOT_GRANTED", ""},
		{"catalogue down", granted, failingOperatorCatalogue{}, "presented.operator.token", http.StatusInternalServerError, "NO_ANSWER_FORMED", ""},
		{"empty catalogue", granted, emptyOperatorCatalogue{}, "presented.operator.token", http.StatusOK, "", "OPERATORS_LISTED"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler, err := buildOperatorCatalogue(testCase.minter, testCase.catalogue, "https://id.syn.example/dex")
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/operators?tenant=SYN-TENANT-02", nil)
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testCase.status {
				t.Fatalf("status %d body %s", response.Code, response.Body.String())
			}
			if testCase.outcome != "" {
				var listed struct {
					Outcome   string            `json:"outcome"`
					Issuer    string            `json:"issuer"`
					Operators []json.RawMessage `json:"operators"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
					t.Fatal(err)
				}
				if listed.Outcome != testCase.outcome || listed.Issuer != "https://id.syn.example/dex" || listed.Operators == nil || len(listed.Operators) != 0 {
					t.Fatalf("body %s", response.Body.String())
				}
				return
			}
			if problemCode(t, response) != testCase.code {
				t.Fatalf("code %s body %s", problemCode(t, response), response.Body.String())
			}
		})
	}

	// 三个写口同一只认证件：只持登记册配置写的人在写口上同样答未授予，不开事务。
	transactor := &countingTransactor{}
	for _, build := range []func() (http.Handler, error){
		func() (http.Handler, error) { return buildOperatorRegistration(writer, transactor, unusedRegistrar{}) },
		func() (http.Handler, error) {
			return buildOperatorGrantRegistration(writer, transactor, unusedRegistrar{})
		},
		func() (http.Handler, error) {
			return buildOperatorGrantRevocation(writer, transactor, unusedRegistrar{}, emptyOperatorCatalogue{})
		},
	} {
		handler, err := build()
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || problemCode(t, response) != "OPERATOR_NOT_GRANTED" || transactor.opened != 0 {
			t.Fatalf("写口对登记册配置写的人：%d %s（事务 %d 笔）", response.Code, response.Body.String(), transactor.opened)
		}
	}
}

// accessAdministratorRegister 是操作者册的替身：主体绑在 SYN-TENANT-01 上，只持操作者接入管理一格。
type accessAdministratorRegister struct{}

func (accessAdministratorRegister) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	grant, err := accessidentity.NewOperatorGrant("SYN-TENANT-01", "SYN-GRANT-ADMIN", subject, accessidentity.CapabilityOperatorAccessAdministration, interval, "SYN-BASIS-GRANT")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, nil)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	standing, err := accessidentity.NewOperatorStanding(binding, []accessidentity.RecordedGrant{recorded})
	return standing, err == nil, err
}

type emptyOperatorCatalogue struct{}

func (emptyOperatorCatalogue) ListOperators(context.Context, string) ([]accessidentity.OperatorCatalogueRow, error) {
	return []accessidentity.OperatorCatalogueRow{}, nil
}

func (emptyOperatorCatalogue) FindGrant(context.Context, string, string) (accessidentity.RecordedGrant, bool, error) {
	return accessidentity.RecordedGrant{}, false, nil
}

type failingOperatorCatalogue struct{}

func (failingOperatorCatalogue) ListOperators(context.Context, string) ([]accessidentity.OperatorCatalogueRow, error) {
	return nil, errors.New("operator catalogue down")
}

func (failingOperatorCatalogue) FindGrant(context.Context, string, string) (accessidentity.RecordedGrant, bool, error) {
	return accessidentity.RecordedGrant{}, false, errors.New("operator catalogue down")
}

type countingTransactor struct{ opened int }

func (transactor *countingTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	transactor.opened++
	return fn(ctx)
}

type unusedRegistrar struct{}

func (unusedRegistrar) RegisterOperator(context.Context, accessidentity.OperatorBinding) (accessidentity.OperatorRegistrationOutcome, error) {
	return "", errors.New("unused")
}

func (unusedRegistrar) RegisterGrant(context.Context, accessidentity.OperatorGrant) (accessidentity.OperatorRegistrationOutcome, error) {
	return "", errors.New("unused")
}

func (unusedRegistrar) RegisterRevocation(context.Context, accessidentity.GrantRevocation) (accessidentity.OperatorRegistrationOutcome, error) {
	return "", errors.New("unused")
}
