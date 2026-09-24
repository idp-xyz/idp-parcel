package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// unwiredDecisionTargets 是委托寻址读口的占位：换口那几行的答复格在寻址之前就定了，走不到它。
type unwiredDecisionTargets struct{}

func (unwiredDecisionTargets) FindOperatorDecisionTarget(context.Context, psdomain.TenantID, psdomain.ShipmentRequestID) (psports.OperatorDecisionTarget, bool, error) {
	return psports.OperatorDecisionTarget{}, false, errors.New("unwired: operator decision target lookup")
}

// decisionRegister 是操作者册的替身：主体绑在 SYN-TENANT-01 上，持有 kinds 里每一种运营决定的授予。
type decisionRegister struct{ kinds []accessidentity.DecisionKind }

func (register decisionRegister) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	var grants []accessidentity.RecordedGrant
	for index, kind := range register.kinds {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", fmt.Sprintf("SYN-GRANT-%d", index), subject, kind, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
		recorded, err := accessidentity.NewRecordedGrant(grant, nil)
		if err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, grants)
	return standing, err == nil, err
}

// acceptingVerifier 替身核验方：带了令牌就答同一个合成操作者，没带答令牌缺失。
type acceptingVerifier struct{}

func (acceptingVerifier) VerifyOperatorCredential(_ context.Context, credential accessidentity.OperatorCredential) (accessidentity.OperatorSubject, error) {
	if credential.Token() == "" {
		return accessidentity.OperatorSubject{}, accessidentity.ErrCredentialRejected
	}
	return accessidentity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-01")
}

// unconfiguredOperatorDecisions 是发行方参数未设时的运营决定 Intake：与生产装配在未配置环境里的那一套同形。
func unconfiguredOperatorDecisions() operatorDecisionIntakes {
	decisions, err := buildOperatorDecisionIntakes(unconfiguredOperatorMinter(), unwiredDecisionTargets{})
	if err != nil {
		panic(err)
	}
	return decisions
}

func unconfiguredOperatorRegistries() operatorRegistryIntakes {
	registries, err := buildOperatorRegistryIntakes(unconfiguredOperatorMinter())
	if err != nil {
		panic(err)
	}
	return registries
}

func unconfiguredOperatorMinter() *accessidentity.OperatorMinter {
	minter, err := buildOperatorMinter(accessidentity.UnconfiguredOperatorCredentialVerifier{}, decisionRegister{})
	if err != nil {
		panic(err)
	}
	return minter
}

// registryWriterRegister 是操作者册的替身：主体绑在 SYN-TENANT-01 上，持有登记册配置写的授予。
type registryWriterRegister struct{}

func (registryWriterRegister) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	grant, err := accessidentity.NewOperatorGrant("SYN-TENANT-01", "SYN-GRANT-WRITE", subject, accessidentity.CapabilityRegistryConfigurationWrite, interval, "SYN-BASIS-GRANT")
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

var swappedRegistryFaces = []string{
	"/visibility-catalogue-milestone-mapping-registrations",
	"/visibility-catalogue-triage-rule-registrations",
	"/visibility-catalogue-notification-policy-registrations",
	"/visibility-catalogue-claim-eligibility-registrations",
	"/visibility-catalogue-claim-authorization-registrations",
	"/visibility-catalogue-disclosure-policy-registrations",
	"/visibility-catalogue-exception-disclosure-rule-registrations",
	"/visibility-catalogue-conflict-signal-rule-registrations",
	"/customs-interpretation-rule-registrations",
	"/customs-gate-catalog-registrations",
	"/customs-candidate-port-registrations",
	"/customs-declaration-path-registrations",
	"/customs-case-requirement-registrations",
	"/customs-duty-collaboration-registrations",
	"/customs-duty-payment-verification-registrations",
}

func TestSwappedRegistryFacesAnswerFromTheOperatorChannel(t *testing.T) {
	minter, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{})
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(unconfiguredOperatorDecisions(), registries, nil, nil, nil, nil, nil, nil, nil))

	for _, pattern := range swappedRegistryFaces {
		cases := map[string]struct {
			token  string
			status int
			code   string
		}{
			"no bearer token": {"", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
			// 授予齐备：认证过了才走到译装，批文自报租户在那里被拒——租户只从信封来。
			"granted, tenant self-reported": {"presented.operator.token", http.StatusBadRequest, "MALFORMED_REQUEST"},
		}
		for name, testCase := range cases {
			request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(`{"tenantId": "SYN-TENANT-02"}`))
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != testCase.status {
				t.Fatalf("%s, %s: status = %d, want %d", pattern, name, response.Code, testCase.status)
			}
			if got := problemCode(t, response); got != testCase.code {
				t.Fatalf("%s, %s: code = %q, want %q", pattern, name, got, testCase.code)
			}
		}
	}
}

var swappedDecisionFaces = map[string]accessidentity.DecisionKind{
	"/shipment-requests/manual-review-completions":         accessidentity.DecisionManualReviewCompletion,
	"/shipment-requests/rejections":                        accessidentity.DecisionActiveRejection,
	"/shipment-requests/authorized-dispositions":           accessidentity.DecisionAuthorizedDisposition,
	"/transport-fulfillment-load-assignment-registrations": accessidentity.DecisionLoadAssignment,
	"/transport-fulfillment-participation-terminations":    accessidentity.DecisionParticipationTermination,
}

func TestSwappedDecisionFacesAnswerFromTheOperatorChannel(t *testing.T) {
	var kinds []accessidentity.DecisionKind
	for _, kind := range swappedDecisionFaces {
		kinds = append(kinds, kind)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: kinds})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(decisions, unconfiguredOperatorRegistries(), nil, nil, nil, nil, nil, nil, nil))

	for pattern := range swappedDecisionFaces {
		cases := map[string]struct {
			token  string
			status int
			code   string
		}{
			// 没带令牌：操作者渠道的「需要登录」格，未换口时这里是 403 未配置。
			"no bearer token": {"", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
			// 授予齐备但租户的准入对照没登：首个租户登记区间之前，生产上就停在这一格（ADR-0149 越权风险点 3）。
			"granted, admission not registered": {"presented.operator.token", http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		}
		for name, testCase := range cases {
			request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(`{}`))
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != testCase.status {
				t.Fatalf("%s, %s: status = %d, want %d", pattern, name, response.Code, testCase.status)
			}
			if got := problemCode(t, response); got != testCase.code {
				t.Fatalf("%s, %s: code = %q, want %q", pattern, name, got, testCase.code)
			}
			assertNoOutcome(t, response, pattern)
		}
	}
}
