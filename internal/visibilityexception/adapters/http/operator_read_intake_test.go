package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
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

func operatorReadIntake(t *testing.T, reader operationsReader) *visibilityhttp.OperatorOperationsReadIntake {
	t.Helper()
	intake, err := visibilityhttp.NewOperatorOperationsReadIntake(reader)
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
	request := httptest.NewRequest(http.MethodGet, "/tracking-projections?tenant=TENANT-9&limit=9999", body)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	request.Header.Set("X-Reported-Tenant", "TENANT-9")

	query, err := intake.IntakeOperationsQuery(context.Background(), request)
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
		{visibilityhttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{visibilityhttp.ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{visibilityhttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{visibilityhttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.code, func(t *testing.T) {
			intake := operatorReadIntake(t, operationsReader{err: fmt.Errorf("%w: refused", testCase.grade)})
			// 读口一被触到就答 500：拒在 Intake 那一格的答复码因此不可能是读口给的。
			response := opsServe(t, intake, &projectionReaderDouble{err: errors.New("读口不该被触到")}, http.MethodGet, "/tracking-projections")
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("答 %d %s", response.Code, response.Body.String())
			}
			assertNoOutcome(t, response)
		})
	}
}

// Covers: 同上——装进端点后，读口按信封的租户作答：同一份投影，信封租户对得上才列出来。
func TestOperatorReadIntakeFeedsTheEndpointTheEnvelopeTenant(t *testing.T) {
	projection := operationsProjection(t)
	for _, testCase := range []struct {
		tenant string
		listed int
	}{{"tenant-1", 1}, {"tenant-2", 0}} {
		t.Run(testCase.tenant, func(t *testing.T) {
			response := opsServe(t, operatorReadIntake(t, operationsReader{tenant: testCase.tenant}), opsReader(t, projection),
				http.MethodGet, "/tracking-projections?tenant=tenant-1")
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			var body struct {
				Projections []json.RawMessage `json:"projections"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Projections) != testCase.listed {
				t.Fatalf("列出 %d 份，want %d（%v）：%s", len(body.Projections), testCase.listed, err, response.Body.String())
			}
		})
	}
}

// Covers: 查阅读 Intake 只进运营查阅面：客户查阅面的客户维是调用方自己的身份主张，归客户渠道；索赔是客户渠道的命令口。
// 登记命令口那一侧由 TestOperationsReadIntakeCannotServeCatalogRegistration 钉。
func TestOperatorReadIntakeServesOnlyTheOperationsReadFaces(t *testing.T) {
	var intake any = operatorReadIntake(t, operationsReader{tenant: "SYN-TENANT-01"})
	if _, ok := intake.(visibilityhttp.QueryIntake); ok {
		t.Fatal("查阅读 Intake 不该装得进客户查阅面")
	}
	if _, ok := intake.(visibilityhttp.ClaimIntake); ok {
		t.Fatal("查阅读 Intake 不该装得进索赔口")
	}
	if _, err := visibilityhttp.NewOperatorOperationsReadIntake(nil); err == nil {
		t.Fatal("缺认证方也构造出来了")
	}
}
