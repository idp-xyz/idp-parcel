package customshttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

type capturingRestrictionManager struct {
	established application.EstablishInternalRestrictionCommand
	decided     application.DecideInternalRestrictionCommand
	calls       int
}

func (fake *capturingRestrictionManager) Establish(_ context.Context, command application.EstablishInternalRestrictionCommand) (application.InternalRestrictionResult, error) {
	fake.established = command
	fake.calls++
	return application.InternalRestrictionResult{}, nil
}

func (fake *capturingRestrictionManager) Decide(_ context.Context, command application.DecideInternalRestrictionCommand) (application.InternalRestrictionResult, error) {
	fake.decided = command
	fake.calls++
	return application.InternalRestrictionResult{}, nil
}

const (
	establishRestrictionBody = `{"restrictionId":"IR-1","source":"SRC-1","basis":"RULE-1","condition":"COND-1","startedAt":"2026-10-20T00:00:00Z",` +
		`"coverage":[{"scope":"PARCEL-1","action":"OUTBOUND_RELEASE","standing":"IN_FORCE"}]}`
	decideRestrictionBody = `{"restrictionId":"IR-1","decisionId":"IRD-1","kind":"RELEASE","source":"SRC-1",` +
		`"covers":[{"scope":"PARCEL-1","action":"OUTBOUND_RELEASE"}],"evidence":"EV-1","authority":"DELEG-1","decidedAt":"2026-10-25T00:00:00Z"}`
)

// Covers: UC-CC-011（Intake 半边，票 launch-wiring/01）——两口各按自己那一种运营决定认证；租户与决定方取认证结果，决定方式恒为人工决定，
// 载荷里没有这几格；动作与适用性译成封闭集合的值。
func TestInternalRestrictionFacesAuthenticateTheirOwnKindAndTakeIdentityFromTheEnvelope(t *testing.T) {
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	manager := &capturingRestrictionManager{}
	customshttp.NewEstablishInternalRestrictionEndpoint(intake, manager).ServeHTTP(httptest.NewRecorder(), post("/customs-internal-restrictions", establishRestrictionBody))
	customshttp.NewDecideInternalRestrictionEndpoint(intake, manager).ServeHTTP(httptest.NewRecorder(), post("/customs-internal-restriction-decisions", decideRestrictionBody))

	if len(asked) != 2 || asked[0] != "INTERNAL_RESTRICTION_ESTABLISHMENT" || asked[1] != "INTERNAL_RESTRICTION_DECISION" {
		t.Fatalf("asked = %v", asked)
	}
	spec := manager.established.Spec
	if manager.established.TenantID.String() != "SYN-TENANT-01" || spec.DecidedBy.String() != "https://issuer#alice" ||
		spec.Mode != domain.ManualDecision || len(spec.Coverage) != 1 ||
		spec.Coverage[0].Action != domain.OutboundRelease || spec.Coverage[0].Standing != domain.CoverageInForce {
		t.Fatalf("establish command = %+v", manager.established)
	}
	decision := manager.decided.Spec
	if manager.decided.TenantID.String() != "SYN-TENANT-01" || manager.decided.RestrictionID.String() != "IR-1" ||
		decision.Kind != domain.ReleaseCoverage || decision.DecidedBy.String() != "https://issuer#alice" ||
		decision.Authority.String() != "DELEG-1" || len(decision.Covers) != 1 {
		t.Fatalf("decide command = %+v", manager.decided)
	}
}

// Covers: UC-CC-011（Intake 半边）——载荷是封闭的：不收租户、决定方与决定方式；动作、适用性与决定种类给了却不在封闭集合里是坏报文，
// 决定的覆盖项不带适用性；编排一次也不被调到。
func TestInternalRestrictionPayloadsAreClosed(t *testing.T) {
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	manager := &capturingRestrictionManager{}
	establish := customshttp.NewEstablishInternalRestrictionEndpoint(intake, manager)
	decide := customshttp.NewDecideInternalRestrictionEndpoint(intake, manager)
	for name, test := range map[string]struct {
		handler http.Handler
		body    string
	}{
		"建立带租户":         {establish, `{"tenantId":"T","restrictionId":"IR-1"}`},
		"建立带决定方":        {establish, `{"restrictionId":"IR-1","decidedBy":"bob"}`},
		"建立带决定方式":       {establish, `{"restrictionId":"IR-1","mode":"RULE"}`},
		"动作不在封闭集合":      {establish, `{"restrictionId":"IR-1","coverage":[{"scope":"P","action":"RECEIVE","standing":"IN_FORCE"}]}`},
		"适用性不在封闭集合":     {establish, `{"restrictionId":"IR-1","coverage":[{"scope":"P","action":"OUTBOUND_RELEASE","standing":"RELEASED"}]}`},
		"时刻不是 RFC 3339": {establish, `{"restrictionId":"IR-1","startedAt":"yesterday"}`},
		"决定种类不认识":       {decide, `{"restrictionId":"IR-1","kind":"REVOKE"}`},
		"决定的覆盖项带适用性":    {decide, `{"restrictionId":"IR-1","covers":[{"scope":"P","action":"OUTBOUND_RELEASE","standing":"IN_FORCE"}]}`},
		"决定带决定方":        {decide, `{"restrictionId":"IR-1","decidedBy":"bob"}`},
	} {
		response := httptest.NewRecorder()
		test.handler.ServeHTTP(response, post("/x", test.body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d，want 400", name, response.Code)
		}
	}
	if manager.calls != 0 {
		t.Fatalf("坏报文把编排调了 %d 次", manager.calls)
	}
}
