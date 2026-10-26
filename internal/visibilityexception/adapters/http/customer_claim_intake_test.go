package visibilityhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

type claimAuthenticatorDouble struct {
	viewer visibilityhttp.CustomerViewer
	err    error
}

// AuthenticateClaim 照真认证器的次序：令牌过了才读载荷。
func (double claimAuthenticatorDouble) AuthenticateClaim(
	_ context.Context,
	_ visibilityhttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (visibilityhttp.CustomerViewer, error) {
	if double.err != nil {
		return visibilityhttp.CustomerViewer{}, double.err
	}
	if _, err := accessRequestID(); err != nil {
		return visibilityhttp.CustomerViewer{}, err
	}
	return double.viewer, nil
}

type targetOwnerDouble struct {
	owner     domain.CustomerAccountReference
	attribute bool
	err       error
}

func (double targetOwnerDouble) FindCustomerAccount(
	context.Context, domain.TenantID, domain.TrackedParcelReference,
) (domain.CustomerAccountReference, bool, error) {
	return double.owner, double.attribute, double.err
}

type contractOwnerDouble struct {
	owned bool
	err   error
}

func (double contractOwnerDouble) ContractBelongsToCustomer(
	context.Context, domain.TenantID, domain.CustomerAccountReference, domain.ContractScopeReference,
) (bool, error) {
	return double.owned, double.err
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

const claimPayload = `{"accessRequestId":"REQ-1","batch":"B-1","item":"I-1","applicant":"A-1","contract":"C-1","target":"P-1","kind":"LOSS"}`

func claimIntakeFixture(t *testing.T, authenticator claimAuthenticatorDouble, targets targetOwnerDouble, contracts contractOwnerDouble) *visibilityhttp.CustomerClaimIntake {
	t.Helper()
	intake, err := visibilityhttp.NewCustomerClaimIntake(authenticator, targets, contracts, fixedClock{at: claimHTTPSubmittedAt})
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func claimViewer(t *testing.T) visibilityhttp.CustomerViewer {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	customer, err := domain.NewCustomerAccountReference("customer-1")
	if err != nil {
		t.Fatal(err)
	}
	return visibilityhttp.CustomerViewer{Tenant: tenant, Customer: customer}
}

func intakeClaim(intake *visibilityhttp.CustomerClaimIntake, body string) (application.ReceiveClaimCommand, error) {
	return intake.IntakeClaim(context.Background(), httptest.NewRequest(http.MethodPost, "/v1/claims", strings.NewReader(body)))
}

// Covers: AT-VE-208——首方索赔 Intake 的租户与账户取自认证结果，提交时间取到达信封的时钟；载荷自报的账户、租户、操作者与提交时间
// 都是未知字段，答形状错。
func TestCustomerClaimIntakeTakesIdentityFromTheEnvelopeAndClosesThePayload(t *testing.T) {
	viewer := claimViewer(t)
	intake := claimIntakeFixture(t, claimAuthenticatorDouble{viewer: viewer}, targetOwnerDouble{owner: viewer.Customer, attribute: true}, contractOwnerDouble{owned: true})

	command, err := intakeClaim(intake, claimPayload)
	if err != nil {
		t.Fatalf("intake: %v", err)
	}
	if command.TenantID != viewer.Tenant || command.Customer != viewer.Customer || !command.SubmittedAt.Equal(claimHTTPSubmittedAt) ||
		command.Batch.String() != "B-1" || command.Item.String() != "I-1" || command.Applicant.String() != "A-1" ||
		command.Contract.String() != "C-1" || command.Target.String() != "P-1" || command.Kind.String() != "LOSS" {
		t.Fatalf("command = %+v", command)
	}

	for _, extra := range []string{"customer", "customerAccount", "tenantId", "operator", "submittedAt"} {
		body := strings.Replace(claimPayload, `"kind":"LOSS"`, `"kind":"LOSS","`+extra+`":"X"`, 1)
		if _, err := intakeClaim(intake, body); !errors.Is(err, visibilityhttp.ErrMalformedClaim) {
			t.Fatalf("自报 %s：err = %v, want ErrMalformedClaim", extra, err)
		}
	}
	for name, body := range map[string]string{
		"缺项":   strings.Replace(claimPayload, `"item":"I-1",`, "", 1),
		"尾随内容": claimPayload + `{}`,
	} {
		if _, err := intakeClaim(intake, body); !errors.Is(err, visibilityhttp.ErrMalformedClaim) {
			t.Fatalf("%s：err = %v, want ErrMalformedClaim", name, err)
		}
	}
}

// Covers: AT-VE-209——目标包裹不属于信封账户（别的账户、没有委托声明、多份委托声明）与合同不在信封账户名下，同答找不到，不分格；
// 依赖读不动不折成找不到。
func TestCustomerClaimIntakeAnswersNotFoundForForeignReferencesWithoutDistinguishingThem(t *testing.T) {
	viewer := claimViewer(t)
	other, err := domain.NewCustomerAccountReference("customer-2")
	if err != nil {
		t.Fatal(err)
	}
	for name, testCase := range map[string]struct {
		targets   targetOwnerDouble
		contracts contractOwnerDouble
		want      error
	}{
		"目标属别的账户": {targetOwnerDouble{owner: other, attribute: true}, contractOwnerDouble{owned: true}, visibilityhttp.ErrClaimReferenceNotFound},
		"目标无人声明":  {targetOwnerDouble{}, contractOwnerDouble{owned: true}, visibilityhttp.ErrClaimReferenceNotFound},
		"目标多份委托":  {targetOwnerDouble{err: ports.ErrAmbiguousCustomerAccount}, contractOwnerDouble{owned: true}, visibilityhttp.ErrClaimReferenceNotFound},
		"合同不在名下":  {targetOwnerDouble{owner: viewer.Customer, attribute: true}, contractOwnerDouble{}, visibilityhttp.ErrClaimReferenceNotFound},
	} {
		intake := claimIntakeFixture(t, claimAuthenticatorDouble{viewer: viewer}, testCase.targets, testCase.contracts)
		if _, err := intakeClaim(intake, claimPayload); !errors.Is(err, testCase.want) {
			t.Fatalf("%s：err = %v, want %v", name, err, testCase.want)
		}
	}
	boom := errors.New("boom")
	for name, intake := range map[string]*visibilityhttp.CustomerClaimIntake{
		"目标读不动": claimIntakeFixture(t, claimAuthenticatorDouble{viewer: viewer}, targetOwnerDouble{err: boom}, contractOwnerDouble{owned: true}),
		"合同读不动": claimIntakeFixture(t, claimAuthenticatorDouble{viewer: viewer}, targetOwnerDouble{owner: viewer.Customer, attribute: true}, contractOwnerDouble{err: boom}),
	} {
		if _, err := intakeClaim(intake, claimPayload); !errors.Is(err, boom) || errors.Is(err, visibilityhttp.ErrClaimReferenceNotFound) {
			t.Fatalf("%s：err = %v, want 原错、不是找不到", name, err)
		}
	}
}

// Covers: AT-VE-211——Intake 的失败按恢复动作分格映射到状态与码，认证没过时不读载荷；各格都是问题详情、不带 outcome。
func TestClaimEndpointMapsEachIntakeFailureToItsOwnGrade(t *testing.T) {
	for name, testCase := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"令牌不过":   {visibilityhttp.ErrCustomerClientCredentialRejected, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"未授予":    {visibilityhttp.ErrCustomerClientNotGranted, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"不在准入范围": {visibilityhttp.ErrOutsideAdmissionScope, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		"未配置":    {visibilityhttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"依赖不可用":  {visibilityhttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"引用找不到":  {visibilityhttp.ErrClaimReferenceNotFound, http.StatusNotFound, "CLAIM_REFERENCE_NOT_FOUND"},
		"形状错":    {visibilityhttp.ErrMalformedClaim, http.StatusBadRequest, "MALFORMED_REQUEST"},
		"别的错":    {errors.New("boom"), http.StatusInternalServerError, "INTAKE_FAILED"},
	} {
		response := serveClaim(t, claimIntakeDouble{err: testCase.err}, erroringReceiver{}, http.MethodPost)
		if response.Code != testCase.status || !strings.Contains(response.Body.String(), `"code":"`+testCase.code+`"`) ||
			strings.Contains(response.Body.String(), "outcome") {
			t.Fatalf("%s：%d %s, want %d %s", name, response.Code, response.Body.String(), testCase.status, testCase.code)
		}
	}
}
