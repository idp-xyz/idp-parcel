package tfhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

type registryTenant struct{}

func (registryTenant) AuthenticateRegistryWrite(_ context.Context, token string) (domain.TenantID, error) {
	if token == "" {
		return domain.TenantID{}, tfhttp.ErrOperatorCredentialRejected
	}
	return domain.NewTenantID("SYN-TENANT-01")
}

func registryRequest(token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func registryIntake(t *testing.T) *tfhttp.OperatorRegistryIntake {
	t.Helper()
	intake, err := tfhttp.NewOperatorRegistryIntake(registryTenant{})
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

// Covers: 票 operator-channel/04——凭证首登逐字段翻译、租户取认证结果；载荷里的租户键、解不出的时刻答请求畸形，
// 未出示令牌在解载荷之前就答凭证被拒。
func TestCredentialRegistrationTranslatesFieldByField(t *testing.T) {
	intake := registryIntake(t)
	command, err := intake.IntakeCredentialRegistration(context.Background(), registryRequest("t",
		`{"credential": "SYN-BOOKING-01", "version": "v1", "assigner": "SYN-CARRIER-01", "identifiedKind": "BOOKING",
		  "identifiedRef": "SYN-SEGMENT-01", "effectiveFrom": "2026-09-25T00:00:00Z"}`))
	if err != nil {
		t.Fatalf("credential registration: %v", err)
	}
	if command.TenantID.String() != "SYN-TENANT-01" || command.Credential != "SYN-BOOKING-01" || command.EffectiveFrom.IsZero() || !command.EffectiveUntil.IsZero() {
		t.Fatalf("command = %+v", command)
	}
	for name, body := range map[string]string{
		"self-reported tenant": `{"tenantId": "SYN-TENANT-01", "credential": "SYN-BOOKING-01"}`,
		"garbled instant":      `{"credential": "SYN-BOOKING-01", "effectiveFrom": "tomorrow"}`,
	} {
		if _, err := intake.IntakeCredentialRegistration(context.Background(), registryRequest("t", body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
	if _, err := intake.IntakeCredentialRegistration(context.Background(), registryRequest("", `not json`)); !errors.Is(err, tfhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v, want ErrOperatorCredentialRejected", err)
	}
}

// Covers: 票 operator-channel/04——改变适用关系与总单新版本的词认回封闭集；认不出的词译成零值交编排答`未受理`，
// 不在传输层折成坏报文。
func TestChangeAndRevisionWordsLeaveUnknownWordsToTheUseCase(t *testing.T) {
	intake := registryIntake(t)
	change, err := intake.IntakeCredentialApplicabilityChange(context.Background(), registryRequest("t",
		`{"credential": "SYN-BOOKING-01", "change": "SUPERSEDED", "at": "2026-09-26T00:00:00Z", "newVersion": "v2", "replacement": "SYN-BOOKING-02"}`))
	if err != nil || change.Change != domain.CredentialSuperseded || change.Replacement != "SYN-BOOKING-02" {
		t.Fatalf("change = %+v, %v", change, err)
	}
	unknown, err := intake.IntakeCredentialApplicabilityChange(context.Background(), registryRequest("t",
		`{"credential": "SYN-BOOKING-01", "change": "PAUSED", "at": "2026-09-26T00:00:00Z", "newVersion": "v2"}`))
	if err != nil || unknown.Change != domain.CredentialStandingInvalid {
		t.Fatalf("unknown change word = %+v, %v", unknown.Change, err)
	}
	revision, err := intake.IntakeMasterDocumentRevision(context.Background(), registryRequest("t",
		`{"document": "SYN-MAWB-01", "revision": "RESTATE_ASSOCIATIONS", "at": "2026-09-26T00:00:00Z", "newVersion": "v2",
		  "associations": [{"kind": "PARCEL", "reference": "SYN-PARCEL-01"}]}`))
	if err != nil || revision.Revision != domain.MasterDocumentAssociationRestatement || len(revision.Associations) != 1 {
		t.Fatalf("revision = %+v, %v", revision, err)
	}
	registration, err := intake.IntakeMasterDocumentRegistration(context.Background(), registryRequest("t",
		`{"document": "SYN-MAWB-01", "version": "v1", "issuer": "SYN-CARRIER-01", "scope": "SYN-SCOPE", "associations": []}`))
	if err != nil || registration.TenantID.String() != "SYN-TENANT-01" || registration.Associations == nil {
		t.Fatalf("registration = %+v, %v", registration, err)
	}
	rule, err := intake.IntakeEffectiveTimeRuleRegistration(context.Background(), registryRequest("t",
		`{"source": "SYN-SOURCE-01", "version": "v1", "sourceTimeMeaning": "LOCAL", "anchor": "EVENT", "offsetSeconds": 60}`))
	if err != nil || rule.TenantID.String() != "SYN-TENANT-01" || rule.Offset.Seconds() != 60 {
		t.Fatalf("rule = %+v, %v", rule, err)
	}
}
