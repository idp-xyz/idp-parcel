package pricinghttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

type registryAuthenticator struct{ err error }

func (fake registryAuthenticator) AuthenticateRegistryWrite(_ context.Context, token string) (pricinghttp.OperatorIdentity, error) {
	if token == "" {
		return pricinghttp.OperatorIdentity{}, pricinghttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return pricinghttp.OperatorIdentity{}, fake.err
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	return pricinghttp.OperatorIdentity{Tenant: tenant, Operator: "https://id.syn.example/dex#SYN-OPERATOR-02"}, err
}

func reviewRequest(token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/pricing-reference-series-reviews", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func TestSeriesReviewTakesTheReviewerFromTheAuthenticatedOperator(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	command, err := intake.IntakeReferenceSeriesReview(context.Background(), reviewRequest("t",
		`{"seriesId": "SYN-SERIES-01", "seriesVersion": "v2", "decision": "APPROVED", "basis": "SYN-BASIS", "reviewedAt": "2026-09-25T01:00:00Z"}`))
	if err != nil {
		t.Fatalf("series review: %v", err)
	}
	if command.Tenant.String() != "SYN-TENANT-01" || command.Reviewer != "https://id.syn.example/dex#SYN-OPERATOR-02" ||
		command.SeriesID != "SYN-SERIES-01" || command.ReviewedAt.IsZero() {
		t.Fatalf("command = %+v", command)
	}
	for name, body := range map[string]string{
		"reviewer smuggled": `{"seriesId": "SYN-SERIES-01", "reviewer": "someone-else"}`,
		"tenant smuggled":   `{"seriesId": "SYN-SERIES-01", "tenant": "SYN-TENANT-02"}`,
		"instant garbled":   `{"seriesId": "SYN-SERIES-01", "reviewedAt": "yesterday"}`,
	} {
		if _, err := intake.IntakeReferenceSeriesReview(context.Background(), reviewRequest("t", body)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

func TestOnlineSeriesRegistrationRefusesSelfReportedIdentity(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"tenant": "SYN-TENANT-02"}`, `{"registrant": "someone-else"}`} {
		if _, err := intake.IntakeReferenceSeriesRegistration(context.Background(), reviewRequest("t", body)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", body, err)
		}
	}
}
