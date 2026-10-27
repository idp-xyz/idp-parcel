package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

// grantSpec 描述夹具操作者册上的一笔授予：能力面（运营决定另给种类），以及它此刻是否已过期。
type grantSpec struct {
	face    accessidentity.CapabilityFace
	kind    accessidentity.DecisionKind
	expired bool
}

type grantsRegister struct{ specs []grantSpec }

func (register grantsRegister) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	recorded := make([]accessidentity.RecordedGrant, 0, len(register.specs))
	for index, spec := range register.specs {
		start, end := time.Now().Add(-2*time.Hour), time.Time{}
		if spec.expired {
			end = time.Now().Add(-time.Hour)
		}
		interval, err := accessidentity.NewEffectiveInterval(start, end)
		if err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
		id := "SYN-GRANT-" + string(rune('A'+index))
		var grant accessidentity.OperatorGrant
		if spec.face == accessidentity.CapabilityOperationDecision {
			grant, err = accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", id, subject, spec.kind, interval, "SYN-BASIS-GRANT")
		} else {
			grant, err = accessidentity.NewOperatorGrant("SYN-TENANT-01", id, subject, spec.face, interval, "SYN-BASIS-GRANT")
		}
		if err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
		entry, err := accessidentity.NewRecordedGrant(grant, nil)
		if err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
		recorded = append(recorded, entry)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, recorded)
	return standing, err == nil, err
}

// Covers: ADR-0315——授予自查接在操作者查阅读上，答的是铸造那一刻生效的授予：查阅员只持读，配置员读写都持、另带运营决定种类，
// 已过期的写授予不算；没有查阅读授予的人与不在册的人同答未授予，不借自查口说出自己持有什么。
func TestOperatorGrantsAnswerFromTheOperatorChannel(t *testing.T) {
	if identityhttp.OperatorGrantsPath != "/operator-grants" {
		t.Fatalf("路径常量 = %s，端点表字面量对不上", identityhttp.OperatorGrantsPath)
	}
	read := grantSpec{face: accessidentity.CapabilityMasterDataAndOperationsRead}
	write := grantSpec{face: accessidentity.CapabilityRegistryConfigurationWrite}
	decision := grantSpec{face: accessidentity.CapabilityOperationDecision, kind: accessidentity.DecisionManualReviewCompletion}
	expiredWrite := grantSpec{face: accessidentity.CapabilityRegistryConfigurationWrite, expired: true}

	cases := []struct {
		name          string
		specs         []grantSpec
		token         string
		status        int
		code          string
		wantGrants    []string
		wantDecisions []string
	}{
		{"viewer holds read only", []grantSpec{read}, "presented.operator.token", http.StatusOK, "",
			[]string{"MASTER_DATA_AND_OPERATIONS_READ"}, []string{}},
		{"configurator holds read and write and a decision kind", []grantSpec{read, write, decision}, "presented.operator.token", http.StatusOK, "",
			[]string{"MASTER_DATA_AND_OPERATIONS_READ", "REGISTRY_CONFIGURATION_WRITE"}, []string{"MANUAL_REVIEW_COMPLETION"}},
		{"an expired write grant is not held", []grantSpec{read, expiredWrite}, "presented.operator.token", http.StatusOK, "",
			[]string{"MASTER_DATA_AND_OPERATIONS_READ"}, []string{}},
		{"write without read is not a read grant", []grantSpec{write}, "presented.operator.token", http.StatusForbidden, "OPERATOR_NOT_GRANTED", nil, nil},
		{"no bearer token", []grantSpec{read}, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED", nil, nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			minter, err := buildOperatorMinter(acceptingVerifier{}, grantsRegister{specs: testCase.specs}, admissionScopeUnset())
			if err != nil {
				t.Fatal(err)
			}
			handler, err := buildOperatorGrants(minter)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/operator-grants?subject=SYN-OPERATOR-02", nil)
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testCase.status {
				t.Fatalf("status %d body %s", response.Code, response.Body.String())
			}
			if testCase.code != "" {
				if problemCode(t, response) != testCase.code {
					t.Fatalf("code %s body %s", problemCode(t, response), response.Body.String())
				}
				return
			}
			var body struct {
				Outcome   string   `json:"outcome"`
				Grants    []string `json:"grants"`
				Decisions []string `json:"decisions"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Outcome != "OPERATOR_GRANTS_RESOLVED" ||
				!slices.Equal(body.Grants, testCase.wantGrants) || !slices.Equal(body.Decisions, testCase.wantDecisions) {
				t.Fatalf("body %s", response.Body.String())
			}
		})
	}

	unconfigured, err := buildOperatorMinter(accessidentity.UnconfiguredOperatorCredentialVerifier{}, grantsRegister{specs: []grantSpec{read}}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	handler, err := buildOperatorGrants(unconfigured)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/operator-grants", nil)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("unconfigured issuer: status %d body %s", response.Code, response.Body.String())
	}

	if _, err := buildOperatorGrants(nil); err == nil {
		t.Fatal("nil minter was accepted")
	}
}
