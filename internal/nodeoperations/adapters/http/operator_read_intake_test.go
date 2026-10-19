package nodeopshttp_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// operationsReader 是查阅读认证方的替身：记下被问的令牌，交回设定的租户或格。
type operationsReader struct {
	tenant    string
	err       error
	presented *string
}

func (reader operationsReader) AuthenticateOperationsRead(_ context.Context, bearerToken string) (domain.TenantID, error) {
	if reader.presented != nil {
		*reader.presented = bearerToken
	}
	if reader.err != nil {
		return domain.TenantID{}, reader.err
	}
	return domain.NewTenantID(reader.tenant)
}

func operatorReadIntake(t *testing.T, reader operationsReader) *nodeopshttp.OperatorOperationsReadIntake {
	t.Helper()
	intake, err := nodeopshttp.NewOperatorOperationsReadIntake(reader)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

type bodyReadProbe struct {
	io.Reader
	read bool
}

func (probe *bodyReadProbe) Read(p []byte) (int, error) {
	probe.read = true
	return probe.Reader.Read(p)
}

// Covers: 票 operator-channel/35——租户只取自信封，认证方被问的是 Bearer 里那一枚；查询串、头部与请求体里的自报一概不读。
// 作用域引用与页大小是产品常量。
func TestOperatorReadIntakeTakesTheTenantFromTheEnvelope(t *testing.T) {
	var presented string
	intake := operatorReadIntake(t, operationsReader{tenant: "SYN-TENANT-01", presented: &presented})
	body := &bodyReadProbe{Reader: strings.NewReader(`{"tenantId":"TENANT-9"}`)}
	request := httptest.NewRequest(http.MethodGet, "/node-operations-records?registry=reception&tenant=TENANT-9&limit=9999", body)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	request.Header.Set("X-Reported-Tenant", "TENANT-9")

	query, err := intake.IntakeCatalogueQuery(context.Background(), request)
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if presented != "presented.operator.token" {
		t.Fatalf("认证方被问的令牌是 %q", presented)
	}
	if query.Scope.Tenant().String() != "SYN-TENANT-01" || query.Scope.Reference().String() != "MASTER_DATA_AND_OPERATIONS_READ" ||
		query.Limit != 200 || body.read {
		t.Fatalf("tenant %s reference %s limit %d bodyRead %v", query.Scope.Tenant(), query.Scope.Reference(), query.Limit, body.read)
	}
}

// Covers: 同上——认证方交回的格在端点上各映到自己的答复码，读口不被触到，答复不带业务结果格。
func TestOperatorReadRefusalsKeepTheirGradesAtTheEndpoint(t *testing.T) {
	cases := []struct {
		grade  error
		status int
		code   string
	}{
		{nodeopshttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{nodeopshttp.ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{nodeopshttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{nodeopshttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.code, func(t *testing.T) {
			intake := operatorReadIntake(t, operationsReader{err: fmt.Errorf("%w: refused", testCase.grade)})
			response := serveGet(nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(intake, unreachableReviewCatalogue{t: t}), recordsTarget)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("答 %d %s", response.Code, response.Body.String())
			}
			assertNoOutcome(t, response)
		})
	}
}

// Covers: 同上——装进端点后，每本册子的读口收到的都是信封的租户与产品常量页大小，请求里的自报不起作用。
func TestOperatorReadIntakeFeedsEveryRegistryTheEnvelopeTenant(t *testing.T) {
	intake := operatorReadIntake(t, operationsReader{tenant: "SYN-TENANT-01"})
	for _, registry := range []string{"reception", "unidentified-item", "consolidation-unit"} {
		t.Run(registry, func(t *testing.T) {
			register := &stubReviewCatalogue{}
			response := serveGet(nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(intake, register),
				"/node-operations-records?registry="+registry+"&tenant=TENANT-9&limit=9999")
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if register.gotTenant != "SYN-TENANT-01" || register.gotLimit != 200 {
				t.Fatalf("读口收到 tenant=%q limit=%d", register.gotTenant, register.gotLimit)
			}
		})
	}
}

// Covers: 查阅读 Intake 装不进收寄命令口：查阅不形成作业事实，收寄要的是作业事实那一面与设备（ADR-0149）。
func TestOperatorReadIntakeCannotServeReception(t *testing.T) {
	var intake any = operatorReadIntake(t, operationsReader{tenant: "SYN-TENANT-01"})
	if _, ok := intake.(nodeopshttp.ReceptionIntake); ok {
		t.Fatal("查阅读 Intake 不该装得进收寄口")
	}
	if _, err := nodeopshttp.NewOperatorOperationsReadIntake(nil); err == nil {
		t.Fatal("缺认证方也构造出来了")
	}
}
