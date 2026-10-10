package pricinghttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
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

// onlinePriceCardSnapshot 是价卡登记口收的在线镜像：受控批量口那份快照去掉 tenant 一格。夹具登记记在 SYN-TENANT-02
// 下，认证替身答 SYN-TENANT-01——译出来的租户若是前者，就是从快照里漏过来的。
func onlinePriceCardSnapshot(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	source, err := domain.NewSourceFileIdentity("SYN-PRC-CARD-01.json", strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewPriceCardRegistration(
		pptest.Value(t, domain.NewTenantID, "SYN-TENANT-02"), estimatePlan(t), source,
		pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-GRANT-SELL-01", "v1"),
		"SYN-PRC-PRICING-GOVERNANCE",
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := domain.MarshalPriceCardRegistration(registration)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "tenant")
	return document
}

func TestOnlinePriceCardRegistrationTakesTheTenantFromTheEnvelope(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := onlinePriceCardSnapshot(t)
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	command, err := intake.IntakePriceCardRegistration(context.Background(), reviewRequest("t", string(body)))
	if err != nil {
		t.Fatalf("price card registration: %v", err)
	}
	// 发布批准责任方照受控批量口从快照取：它记的是批准人引用，不是提交者。
	registration := command.Registration
	if registration.Tenant().String() != "SYN-TENANT-01" || registration.PublicationApprover() != "SYN-PRC-PRICING-GOVERNANCE" ||
		registration.Plan().Reference().ID() != "SYN-PLAN-EST" {
		t.Fatalf("registration: tenant %s, approver %s, plan %s",
			registration.Tenant(), registration.PublicationApprover(), registration.Plan().Reference().ID())
	}

	// 受控批量口那份原样送来也拒：tenant 键在场即拒、不看值，与信封相同的租户也算自报。
	for name, value := range map[string]string{
		"another tenant":        `"SYN-TENANT-02"`,
		"the envelope's tenant": `"SYN-TENANT-01"`,
		"null":                  `null`,
	} {
		snapshot["tenant"] = json.RawMessage(value)
		carrying, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := intake.IntakePriceCardRegistration(context.Background(), reviewRequest("t", string(carrying))); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}

	// 先认证、后读快照：没带令牌答凭据格，不看载荷。
	if _, err := intake.IntakePriceCardRegistration(context.Background(), reviewRequest("", "not a snapshot")); !errors.Is(err, pricinghttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v, want ErrOperatorCredentialRejected", err)
	}

	// 认证交回空身份是 Intake 一侧的错：单独一格，不折成坏报文。
	blank, err := pricinghttp.NewOperatorRegistryIntake(blankIdentityAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blank.IntakePriceCardRegistration(context.Background(), reviewRequest("t", string(body))); !errors.Is(err, pricinghttp.ErrOperatorIdentityMissing) {
		t.Fatalf("blank identity: err = %v, want ErrOperatorIdentityMissing", err)
	}
}

type blankIdentityAuthenticator struct{}

func (blankIdentityAuthenticator) AuthenticateRegistryWrite(context.Context, string) (pricinghttp.OperatorIdentity, error) {
	return pricinghttp.OperatorIdentity{}, nil
}
