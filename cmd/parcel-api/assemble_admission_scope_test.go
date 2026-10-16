package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 本文件钉准入范围桥接进生产装配之后的那一半（票 operator-channel/19 完成判据第二条）。本产品的权威串没设时，两只铸造器
// 拿到的是未配置那一只：TestSwappedDecisionFacesAnswerFromTheOperatorChannel 与 TestIntegrationClientFacesAnswerTheChannelGrades
// 经 admissionScopeUnset 取它，准入那一格照旧。设了且对照与区间齐备时，装载分配与终止参与越过准入范围、到达真编排。

// admissionScopeUnset 是权威串未设时生产装配交给两只铸造器的那一只：经 buildAdmissionScope 取，不在测试里另拼。
func admissionScopeUnset() accessidentity.AdmissionScope {
	scope, err := buildAdmissionScope(nil, "")
	if err != nil {
		panic(err)
	}
	return scope
}

// Covers: ADR-0180 决定三——权威串没设或只有空白时，准入范围是未配置那一只，库一行都不读（db 传 nil 也装得起来）；
// 参数照部署取值原样取，只去首尾空白。
func TestTheAdmissionScopeIsUnconfiguredWithoutASelfAuthority(t *testing.T) {
	for _, value := range []string{"", " \t"} {
		scope, err := buildAdmissionScope(nil, value)
		if err != nil {
			t.Fatalf("权威串 %q：%v", value, err)
		}
		if _, unconfigured := scope.(accessidentity.UnconfiguredAdmissionScope); !unconfigured {
			t.Fatalf("权威串 %q：scope = %T, want UnconfiguredAdmissionScope", value, scope)
		}
	}
	if got := governanceSelfAuthority(fakeGetenv(map[string]string{governanceSelfAuthorityEnv: "  SYN-AUTH/idp-parcel-pilot \n"})); got != "SYN-AUTH/idp-parcel-pilot" {
		t.Fatalf("解析出 %q", got)
	}
}

const (
	operationDecisionObjectScope = "SYN-PILOT-SCOPE/operation-decisions@v1"
	loadAssignmentPayload        = `{"assignment": "SYN-ASSIGNMENT-OC19", "schedule": "SYN-SCHEDULE-OC19", "members": ["SYN-PARCEL-OC19"], "version": "v1", "assignedAt": "2026-10-13T00:00:00Z"}`
	participationTerminationBody = `{"segment": "SYN-SEGMENT-OC19", "object": "SYN-PARCEL-OC19", "basis": "SYN-BASIS-OC19", "endedAt": "2026-10-13T00:00:00Z"}`
)

// registerOperationDecisionAdmission 给 SYN-TENANT-01 在两种决定上各登一行对照，并在对照指向的对象范围上各登一条
// authority 的生产权威区间——与演示种子同一种登法：对照住接入身份，区间住治理登记册。
func registerOperationDecisionAdmission(t *testing.T, db *bentopg.DB, authority string, kinds ...accessidentity.DecisionKind) {
	t.Helper()
	mappings, err := aipg.NewAdmissionScopeMappings(db)
	if err != nil {
		t.Fatal(err)
	}
	intervals, err := pgpostgres.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	validity, err := accessidentity.NewEffectiveInterval(from, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range kinds {
		requirement := accessidentity.AdmissionRequirement{Capability: accessidentity.CapabilityOperationDecision.String(), FactKind: kind.String()}
		mapping, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-"+kind.String(), requirement, operationDecisionObjectScope, validity, "SYN-BASIS-MAPPING")
		if err != nil {
			t.Fatal(err)
		}
		var outcome accessidentity.AdmissionScopeMappingOutcome
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var registerErr error
			outcome, registerErr = mappings.RegisterAdmissionScopeMapping(txCtx, mapping)
			if registerErr != nil {
				return registerErr
			}
			return intervals.Append(txCtx, pgdomain.AuthorityInterval{
				ObjectScope: operationDecisionObjectScope,
				Capability:  requirement.Capability,
				FactKind:    requirement.FactKind,
				Authority:   authority,
				From:        from,
			})
		}); err != nil {
			t.Fatalf("登对照与区间 %s：%v", kind, err)
		}
		if outcome != accessidentity.AdmissionScopeMappingRecorded {
			t.Fatalf("登对照 %s = %s", kind, outcome)
		}
	}
}

// Covers: 票 operator-channel/19 完成判据第二条「配了且对照与区间齐备时，装载分配与终止参与答业务结果」（真库）：权威串设成
// 区间上那个串，两口越过准入范围、到达真编排——装载分配形成一版，终止参与答空库上的段不在册；同一组对照与区间、权威串
// 换成别的产品，两口照旧答 403 OUTSIDE_ADMISSION_SCOPE——区间不是本产品的。测试输入是隔离合成，只记 `S`。
func TestTheWiredAdmissionScopeLetsLoadAssignmentAndTerminationThrough(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority,
		accessidentity.DecisionLoadAssignment, accessidentity.DecisionParticipationTermination)
	operations, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatalf("装配段作业编排：%v", err)
	}

	serve := func(selfAuthority string, endpoint func(operatorDecisionIntakes) http.Handler, body string) *httptest.ResponseRecorder {
		t.Helper()
		admission, err := buildAdmissionScope(db, selfAuthority)
		if err != nil {
			t.Fatal(err)
		}
		minter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: []accessidentity.DecisionKind{
			accessidentity.DecisionLoadAssignment, accessidentity.DecisionParticipationTermination,
		}}, admission)
		if err != nil {
			t.Fatal(err)
		}
		decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		endpoint(decisions).ServeHTTP(response, request)
		return response
	}
	faces := map[string]struct {
		endpoint func(operatorDecisionIntakes) http.Handler
		body     string
		status   int
		outcome  string
	}{
		"load assignment": {
			func(decisions operatorDecisionIntakes) http.Handler {
				return tfhttp.NewFormLoadAssignmentEndpoint(decisions.transport, operations.assign)
			}, loadAssignmentPayload, http.StatusCreated, "LOAD_ASSIGNMENT_FORMED",
		},
		"participation termination": {
			func(decisions operatorDecisionIntakes) http.Handler {
				return tfhttp.NewTerminateFulfillmentParticipationEndpoint(decisions.transport, operations.enderOf)
			}, participationTerminationBody, http.StatusOK, "SEGMENT_NOT_FOUND",
		},
	}
	for name, face := range faces {
		admitted := serve(isolatedSelfAuthority, face.endpoint, face.body)
		if admitted.Code != face.status || answeredOutcome(t, admitted) != face.outcome {
			t.Fatalf("%s，本产品的区间：%d %s, want %d %s", name, admitted.Code, admitted.Body.Bytes(), face.status, face.outcome)
		}
		refused := serve("SYN-AUTH/another-product", face.endpoint, face.body)
		if refused.Code != http.StatusForbidden || problemCode(t, refused) != "OUTSIDE_ADMISSION_SCOPE" {
			t.Fatalf("%s，区间不是本产品的：%d %s", name, refused.Code, refused.Body.Bytes())
		}
	}
}
