package customshttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 端点测试驱动真实编排，替身只在端口层：传输层不该能捏造业务结果。
type regulatoryRestrictionStoreStub struct {
	byID map[string]domain.RegulatoryRestriction
	err  error
}

func (stub *regulatoryRestrictionStoreStub) FindByID(_ context.Context, _ domain.TenantID, id domain.RestrictionID) (domain.RegulatoryRestriction, bool, error) {
	if stub.err != nil {
		return domain.RegulatoryRestriction{}, false, stub.err
	}
	restriction, found := stub.byID[id.String()]
	return restriction, found, nil
}

func (stub *regulatoryRestrictionStoreStub) ListByScope(context.Context, domain.TenantID, domain.DecisionScopeReference) ([]domain.RegulatoryRestriction, error) {
	return nil, nil
}

func (stub *regulatoryRestrictionStoreStub) Save(_ context.Context, _ domain.TenantID, restriction domain.RegulatoryRestriction) (ports.RestrictionSaveOutcome, error) {
	if stub.err != nil {
		return ports.RestrictionSaveOutcomeInvalid, stub.err
	}
	if _, exists := stub.byID[restriction.ID().String()]; exists {
		return ports.RestrictionAlreadyRecorded, nil
	}
	stub.byID[restriction.ID().String()] = restriction
	return ports.RestrictionSaved, nil
}

func (stub *regulatoryRestrictionStoreStub) Update(_ context.Context, _ domain.TenantID, restriction domain.RegulatoryRestriction) (ports.RestrictionUpdateOutcome, error) {
	if current, exists := stub.byID[restriction.ID().String()]; !exists || !current.Current() {
		return ports.RestrictionAlreadySettled, nil
	}
	stub.byID[restriction.ID().String()] = restriction
	return ports.RestrictionReleaseRecorded, nil
}

type acceptedResultsStub struct {
	bySource map[string]ports.ExternalResultRecord
}

func (stub acceptedResultsStub) FindByKey(_ context.Context, key ports.ExternalResultKey) (ports.ExternalResultRecord, bool, error) {
	record, found := stub.bySource[key.SourceID]
	return record, found, nil
}

type restrictionHandoffStub struct{}

func (restrictionHandoffStub) HandOffRestriction(context.Context, ports.RestrictionHandoffIntent) error {
	return nil
}

var regulatoryHeldAt = time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)

func acceptedResult(t *testing.T, source string, layer domain.ResultLayer, scope string, occurredAt time.Time) ports.ExternalResultRecord {
	t.Helper()
	role, err := domain.NewSourceAuthorityRole("SYN-CUSTOMS-AUTHORITY")
	if err != nil {
		t.Fatal(err)
	}
	rule, err := domain.NewInterpretationRuleReference("SYN-RULE/v1")
	if err != nil {
		t.Fatal(err)
	}
	version, err := domain.NewSubmissionVersionID("SYN-DECL-V-0001")
	if err != nil {
		t.Fatal(err)
	}
	scopeRef, err := domain.NewDecisionScopeReference(scope)
	if err != nil {
		t.Fatal(err)
	}
	result, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer: layer, SourceID: source, Role: role, RawSemantics: "SYN 监管原文", Rule: rule, Version: version, Attempt: 1,
		Scope: scopeRef, OccurredAt: occurredAt, ReceivedAt: occurredAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("interpret %s: %v", source, err)
	}
	return ports.ExternalResultRecord{Result: result}
}

type regulatoryRestrictionEndpoints struct {
	establish http.Handler
	release   http.Handler
	store     *regulatoryRestrictionStoreStub
	asked     *[]customshttp.OperatorDecision
}

func regulatoryRestrictionEndpointsForTest(t *testing.T) regulatoryRestrictionEndpoints {
	t.Helper()
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	store := &regulatoryRestrictionStoreStub{byID: map[string]domain.RegulatoryRestriction{}}
	handler := application.NewManageRestrictionHandler(application.ManageRestrictionDeps{
		Results: acceptedResultsStub{bySource: map[string]ports.ExternalResultRecord{
			"SYN-HOLD-0001":    acceptedResult(t, "SYN-HOLD-0001", domain.ProcessDecisionLayer, "SYN-DECL-UNIT-0001", regulatoryHeldAt),
			"SYN-RELEASE-0001": acceptedResult(t, "SYN-RELEASE-0001", domain.ReleaseResultLayer, "SYN-DECL-UNIT-0001", regulatoryHeldAt.Add(time.Hour)),
			"SYN-RECEIPT-0001": acceptedResult(t, "SYN-RECEIPT-0001", domain.RegulatoryReceiptLayer, "SYN-DECL-UNIT-0001", regulatoryHeldAt),
		}},
		Store:      store,
		Downstream: restrictionHandoffStub{},
		Clock:      fixedClock{at: clientReceivedAt},
	})
	return regulatoryRestrictionEndpoints{
		establish: customshttp.NewEstablishRegulatoryRestrictionEndpoint(intake, handler),
		release:   customshttp.NewReleaseRegulatoryRestrictionEndpoint(intake, handler),
		store:     store,
		asked:     &asked,
	}
}

type regulatoryRestrictionAnswer struct {
	Outcome       string   `json:"outcome"`
	RestrictionID string   `json:"restrictionId"`
	Decision      string   `json:"decision"`
	Scope         string   `json:"scope"`
	Constrains    []string `json:"constrains"`
	EffectiveAt   string   `json:"effectiveAt"`
	ReleasedBy    string   `json:"releasedBy"`
	ReleasedAt    string   `json:"releasedAt"`
	Error         struct {
		Code string `json:"code"`
	} `json:"error"`
}

func serveRestriction(t *testing.T, handler http.Handler, body string) (int, regulatoryRestrictionAnswer) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, post("/x", body))
	var answer regulatoryRestrictionAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, recorder.Body.String())
	}
	return recorder.Code, answer
}

const (
	establishRegulatoryRestrictionBody = `{"restrictionId":"SYN-RR-0001","decision":"SYN-HOLD-0001","constrains":["OUTBOUND_RELEASE","LOADING_DEPARTURE"]}`
	releaseRegulatoryRestrictionBody   = `{"restrictionId":"SYN-RR-0001","result":"SYN-RELEASE-0001"}`
)

// Covers: 监管限制的两个运营决定口——各按自己那一种决定认证；建立时限制的决定身份、范围与生效时刻取自所引的那条已接收
// 监管决定（201），重放答已有（200）；解除凭同范围、生效之后的一条放行结果，解除时刻取它的业务发生时间（201），再解除
// 一次答已解除（200）。
func TestRegulatoryRestrictionFacesEstablishAndReleaseOnAcceptedResults(t *testing.T) {
	endpoints := regulatoryRestrictionEndpointsForTest(t)

	status, answer := serveRestriction(t, endpoints.establish, establishRegulatoryRestrictionBody)
	if status != http.StatusCreated || answer.Outcome != "ESTABLISHED" || answer.Decision != "SYN-HOLD-0001" ||
		answer.Scope != "SYN-DECL-UNIT-0001" || answer.EffectiveAt != "2026-11-12T08:00:00Z" || len(answer.Constrains) != 2 {
		t.Fatalf("建立：status %d answer %+v", status, answer)
	}
	if status, answer = serveRestriction(t, endpoints.establish, establishRegulatoryRestrictionBody); status != http.StatusOK || answer.Outcome != "EXISTING" {
		t.Fatalf("重放建立：status %d answer %+v, want 200 EXISTING", status, answer)
	}

	status, answer = serveRestriction(t, endpoints.release, releaseRegulatoryRestrictionBody)
	if status != http.StatusCreated || answer.Outcome != "RELEASED" || answer.ReleasedBy != "SYN-RELEASE-0001" || answer.ReleasedAt != "2026-11-12T09:00:00Z" {
		t.Fatalf("解除：status %d answer %+v", status, answer)
	}
	if status, answer = serveRestriction(t, endpoints.release, releaseRegulatoryRestrictionBody); status != http.StatusOK || answer.Outcome != "ALREADY_RELEASED" {
		t.Fatalf("再解除：status %d answer %+v, want 200 ALREADY_RELEASED", status, answer)
	}

	asked := *endpoints.asked
	if len(asked) != 4 || asked[0] != "REGULATORY_RESTRICTION_ESTABLISHMENT" || asked[2] != "REGULATORY_RESTRICTION_RELEASE" {
		t.Fatalf("asked = %v——两口要各按自己那一种决定认证", asked)
	}
}

// Covers: 两口的拒绝各格——载荷是封闭的，不收租户、范围、生效或解除时刻（那些取自所引结果），动作词不在封闭集合里是坏
// 报文（400）；所引结果不是决定层、或不在册，答依据不成立（200，限制册不动）；限制册读不动是没形成答案（500）。
func TestRegulatoryRestrictionFacesGradeTheirRefusals(t *testing.T) {
	for name, test := range map[string]struct {
		release  bool
		body     string
		storeErr bool
		status   int
		outcome  string
		code     string
	}{
		"建立带租户":     {body: `{"tenantId":"T","restrictionId":"SYN-RR-0001","decision":"SYN-HOLD-0001","constrains":["OUTBOUND_RELEASE"]}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"建立带范围":     {body: `{"scope":"SYN-OTHER","restrictionId":"SYN-RR-0001","decision":"SYN-HOLD-0001","constrains":["OUTBOUND_RELEASE"]}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"建立带生效时刻":   {body: `{"effectiveAt":"2026-01-01T00:00:00Z","restrictionId":"SYN-RR-0001","decision":"SYN-HOLD-0001","constrains":["OUTBOUND_RELEASE"]}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"动作不在封闭集合":  {body: `{"restrictionId":"SYN-RR-0001","decision":"SYN-HOLD-0001","constrains":["RECEIVE"]}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"缺限制标识":     {body: `{"decision":"SYN-HOLD-0001","constrains":["OUTBOUND_RELEASE"]}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"解除带解除时刻":   {release: true, body: `{"restrictionId":"SYN-RR-0001","result":"SYN-RELEASE-0001","releasedAt":"2026-01-01T00:00:00Z"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"依据不是决定层":   {body: `{"restrictionId":"SYN-RR-0001","decision":"SYN-RECEIPT-0001","constrains":["OUTBOUND_RELEASE"]}`, status: http.StatusOK, outcome: "BASIS_NOT_ACCEPTED"},
		"依据不在册":     {body: `{"restrictionId":"SYN-RR-0001","decision":"SYN-HOLD-9999","constrains":["OUTBOUND_RELEASE"]}`, status: http.StatusOK, outcome: "BASIS_NOT_ACCEPTED"},
		"没声明约束哪些动作": {body: `{"restrictionId":"SYN-RR-0001","decision":"SYN-HOLD-0001","constrains":[]}`, status: http.StatusOK, outcome: "NOT_ACCEPTED"},
		"解除不在册的限制":  {release: true, body: releaseRegulatoryRestrictionBody, status: http.StatusOK, outcome: "NOT_FOUND"},
		"限制册读不动":    {body: establishRegulatoryRestrictionBody, storeErr: true, status: http.StatusInternalServerError, code: "NO_ANSWER_FORMED"},
	} {
		t.Run(name, func(t *testing.T) {
			endpoints := regulatoryRestrictionEndpointsForTest(t)
			if test.storeErr {
				endpoints.store.err = context.DeadlineExceeded
			}
			handler := endpoints.establish
			if test.release {
				handler = endpoints.release
			}
			status, answer := serveRestriction(t, handler, test.body)
			if status != test.status || answer.Outcome != test.outcome || answer.Error.Code != test.code {
				t.Fatalf("status %d outcome %q code %q, want %d %q %q", status, answer.Outcome, answer.Error.Code, test.status, test.outcome, test.code)
			}
			if len(endpoints.store.byID) != 0 {
				t.Fatal("被拒的请求留下了限制")
			}
		})
	}
}
