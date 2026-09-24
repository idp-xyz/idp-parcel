package commercialhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

type operatorTenant struct{}

func (operatorTenant) AuthenticateRegistryWrite(_ context.Context, token string) (domain.TenantID, error) {
	if token == "" {
		return domain.TenantID{}, commercialhttp.ErrOperatorCredentialRejected
	}
	return domain.NewTenantID("SYN-TENANT-01")
}

func operatorRequest(token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func operatorIntake(t *testing.T) *commercialhttp.OperatorRegistryIntake {
	t.Helper()
	intake, err := commercialhttp.NewOperatorRegistryIntake(operatorTenant{})
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

// Covers: 票 operator-channel/04——服务形态口吃受控 CLI 同一份批文外壳，租户取认证结果；自报租户（null 也算）、零项、
// 别的口的项都答请求畸形，未出示令牌在解载荷之前就答凭证被拒。
func TestServiceProductFormTakesTheCLIShellWithTheAuthenticatedTenant(t *testing.T) {
	intake := operatorIntake(t)
	form := domain.NetworkServiceForm.String()
	command, err := intake.IntakeServiceProductFormRegistration(context.Background(), operatorRequest("t",
		`{"scope": "SYN-SCOPE", "forms": [{"productId": "SYN-PRODUCT-01", "version": "v1", "form": "`+form+`"}]}`))
	if err != nil {
		t.Fatalf("service product form: %v", err)
	}
	if command.Tenant.String() != "SYN-TENANT-01" || command.ObjectID.String() != "SYN-PRODUCT-01" || command.Form != domain.NetworkServiceForm {
		t.Fatalf("command = %+v", command)
	}
	for name, body := range map[string]string{
		"self-reported tenant": `{"tenantId": "SYN-TENANT-01", "scope": "SYN-SCOPE", "forms": [{"productId": "P", "version": "v1", "form": "` + form + `"}]}`,
		"null tenant":          `{"tenantId": null, "scope": "SYN-SCOPE", "forms": [{"productId": "P", "version": "v1", "form": "` + form + `"}]}`,
		"no item":              `{"scope": "SYN-SCOPE"}`,
		"other line's item": `{"scope": "SYN-SCOPE", "forms": [{"productId": "P", "version": "v1", "form": "` + form + `"}],
			"mappings": [{"mappingId": "M", "revision": 1, "productId": "P", "productVersion": "v1", "channels": [], "basis": "B", "effectiveStartsAt": "2026-01-01T00:00:00Z"}]}`,
		"unknown form": `{"scope": "SYN-SCOPE", "forms": [{"productId": "P", "version": "v1", "form": "NOT_A_FORM"}]}`,
	} {
		if _, err := intake.IntakeServiceProductFormRegistration(context.Background(), operatorRequest("t", body)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
	if _, err := intake.IntakeServiceProductFormRegistration(context.Background(), operatorRequest("", `not json`)); !errors.Is(err, commercialhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v, want ErrOperatorCredentialRejected", err)
	}
}

func TestRegistrationNumberTypeDeactivationTakesOneItem(t *testing.T) {
	intake := operatorIntake(t)
	command, err := intake.IntakeRegistrationNumberTypeDeactivation(context.Background(), operatorRequest("t",
		`{"deactivations": [{"countryCode": "XA", "typeCode": "SYN-LIFETIME", "revision": 2, "basis": "SYN-BASIS-02", "at": "2026-09-25T00:00:00Z"}]}`))
	if err != nil {
		t.Fatalf("deactivation: %v", err)
	}
	if command.Tenant.String() != "SYN-TENANT-01" || command.Revision != 2 || command.At.IsZero() {
		t.Fatalf("command = %+v", command)
	}
	if _, err := intake.IntakeRegistrationNumberTypeRegistration(context.Background(), operatorRequest("t",
		`{"deactivations": [{"countryCode": "XA", "typeCode": "SYN-LIFETIME", "revision": 2, "basis": "SYN-BASIS-02", "at": "2026-09-25T00:00:00Z"}]}`)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("deactivation posted to the registration endpoint: err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: ADR-0093 决定六与票 operator-channel/04——使用授权登记的两格存续按封闭集译、缺席即未答交给发布门；撤销载荷
// 装不下授权正文，带正文的格按未知键拒。
func TestChannelAccountUseRegistrationAndRevocation(t *testing.T) {
	intake := operatorIntake(t)
	registration := `{"authorizationId": "SYN-CAU-01", "revision": 1, "account": "SYN-ACCOUNT-01", "grantor": "SYN-PARTY-01",
		"grantee": "SYN-PARTY-02", "channel": "SYN-CHANNEL-01", "scope": "SYN-SCOPE", "effectiveStartsAt": "2026-01-01T00:00:00Z",
		"technicalStanding": "TECHNICALLY_AVAILABLE", "businessStanding": "BUSINESS_AUTHORIZED", "publishedAt": "2026-09-25T00:00:00Z"}`
	command, err := intake.IntakeChannelAccountUseRegistration(context.Background(), operatorRequest("t", registration))
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	if command.Tenant.String() != "SYN-TENANT-01" || command.Technical != domain.ChannelAccountTechnicallyAvailable ||
		command.Business != domain.ChannelAccountBusinessAuthorized || command.PublishedAt.IsZero() {
		t.Fatalf("command = %+v", command)
	}
	unanswered, err := intake.IntakeChannelAccountUseRegistration(context.Background(), operatorRequest("t",
		strings.Replace(registration, `"technicalStanding": "TECHNICALLY_AVAILABLE", `, "", 1)))
	if err != nil || unanswered.Technical != domain.ChannelAccountTechnicalStandingInvalid {
		t.Fatalf("unanswered technical standing: %+v, %v", unanswered.Technical, err)
	}
	if _, err := intake.IntakeChannelAccountUseRegistration(context.Background(), operatorRequest("t",
		strings.Replace(registration, "BUSINESS_AUTHORIZED", "MAYBE", 1))); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("unknown standing: err = %v, want ErrMalformedRequest", err)
	}
	revocation, err := intake.IntakeChannelAccountUseRevocation(context.Background(), operatorRequest("t",
		`{"authorizationId": "SYN-CAU-01", "basis": "SYN-NOTICE-01", "revokedAt": "2026-09-26T00:00:00Z"}`))
	if err != nil || revocation.Tenant.String() != "SYN-TENANT-01" || revocation.RevokedAt.IsZero() {
		t.Fatalf("revocation = %+v, %v", revocation, err)
	}
	if _, err := intake.IntakeChannelAccountUseRevocation(context.Background(), operatorRequest("t",
		`{"authorizationId": "SYN-CAU-01", "basis": "SYN-NOTICE-01", "revokedAt": "2026-09-26T00:00:00Z", "account": "SYN-ACCOUNT-09"}`)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("revocation restating the body: err = %v, want ErrMalformedRequest", err)
	}
}
