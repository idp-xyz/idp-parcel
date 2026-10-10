package networkhttp_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// operatorAuthenticator 替操作者认证：不带令牌答凭证不过；带了令牌答 err，err 为空即认证到 SYN-TENANT-01。
type operatorAuthenticator struct{ err error }

func (fake operatorAuthenticator) AuthenticateRegistryWrite(_ context.Context, token string) (domain.TenantID, error) {
	if token == "" {
		return domain.TenantID{}, networkhttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return domain.TenantID{}, fake.err
	}
	return domain.NewTenantID("SYN-TENANT-01")
}

func operatorIntake(t *testing.T, authenticator networkhttp.OperatorRegistryAuthenticator) *networkhttp.OperatorRegistryIntake {
	t.Helper()
	intake, err := networkhttp.NewOperatorRegistryIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func operatorRegistration(token string, body io.Reader) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/probe", body)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

// onlineRegistrationRows 是七族各一行在线登记批文，键同 registrationEndpoints：不带 tenant_id，每族只给受理所需的几格。
var onlineRegistrationRows = map[string]string{
	"node":                    `{"code": "SYN-NODE-A", "version": 1, "business_timezone": "Asia/Shanghai", "effective_from": "2026-09-01T00:00:00Z"}`,
	"connection":              `{"code": "SYN-CONN-A-B", "version": 1, "from_node": "SYN-NODE-A", "to_node": "SYN-NODE-B", "business_timezone": "Asia/Shanghai", "effective_from": "2026-09-01T00:00:00Z"}`,
	"line":                    `{"code": "SYN-LINE-1", "version": 1, "segments": ["SYN-CONN-A-B"], "business_timezone": "Asia/Shanghai", "applicable_scope": "NETWORK_SERVICE", "effective_from": "2026-09-01T00:00:00Z"}`,
	"service-area":            `{"code": "SYN-AREA-1", "version": 1, "effective_from": "2026-09-01T00:00:00Z"}`,
	"service-calendar":        `{"target_kind": "NODE", "target_code": "SYN-NODE-A", "version": 1, "effective_from": "2026-09-01T00:00:00Z"}`,
	"availability-adjustment": `{"code": "SYN-ADJ-1", "version": 1, "target_kind": "LINE", "target_code": "SYN-LINE-1", "kind": "SUSPENSION", "source": "SYN-NET-OPS/EVT-1", "effective_at": "2026-09-01T00:00:00Z"}`,
	"route-strategy":          `{"code": "SYN-RS-1", "version": 1, "applicable_scope": "NETWORK_SERVICE", "effective_from": "2026-09-01T00:00:00Z"}`,
}

// Covers: 票 operator-channel/04 第三批——七族在线登记口都把批文交 registrationjson 在线那一路，命令的租户取认证结果。
func TestOperatorRegistryIntakeTranslatesEveryNetworkFamilyUnderTheAuthenticatedTenant(t *testing.T) {
	intake := operatorIntake(t, operatorAuthenticator{})
	ctx := context.Background()
	row := func(family string) *http.Request {
		return operatorRegistration("t", strings.NewReader(onlineRegistrationRows[family]))
	}
	type translated struct {
		tenant domain.TenantID
		code   string
		err    error
	}
	got := map[string]translated{}
	node, err := intake.IntakeNodeVersionRegistration(ctx, row("node"))
	got["node"] = translated{node.TenantID, node.Node.Code, err}
	connection, err := intake.IntakeConnectionVersionRegistration(ctx, row("connection"))
	got["connection"] = translated{connection.TenantID, connection.Connection.Code, err}
	line, err := intake.IntakeLineVersionRegistration(ctx, row("line"))
	got["line"] = translated{line.TenantID, line.Line.Code, err}
	area, err := intake.IntakeServiceAreaVersionRegistration(ctx, row("service-area"))
	got["service-area"] = translated{area.TenantID, area.Area.Code, err}
	calendar, err := intake.IntakeServiceCalendarVersionRegistration(ctx, row("service-calendar"))
	got["service-calendar"] = translated{calendar.TenantID, calendar.Calendar.TargetCode, err}
	adjustment, err := intake.IntakeAvailabilityAdjustmentRegistration(ctx, row("availability-adjustment"))
	got["availability-adjustment"] = translated{adjustment.TenantID, adjustment.Adjustment.Code, err}
	strategy, err := intake.IntakeRouteStrategyVersionRegistration(ctx, row("route-strategy"))
	got["route-strategy"] = translated{strategy.TenantID, strategy.Strategy.Code, err}

	want := map[string]string{
		"node":                    "SYN-NODE-A",
		"connection":              "SYN-CONN-A-B",
		"line":                    "SYN-LINE-1",
		"service-area":            "SYN-AREA-1",
		"service-calendar":        "SYN-NODE-A",
		"availability-adjustment": "SYN-ADJ-1",
		"route-strategy":          "SYN-RS-1",
	}
	for family, code := range want {
		if result := got[family]; result.err != nil || result.tenant.String() != "SYN-TENANT-01" || result.code != code {
			t.Fatalf("%s：租户 %q、编码 %q、err %v，想要 SYN-TENANT-01 与 %s", family, result.tenant.String(), result.code, result.err, code)
		}
	}
}

// Covers: ADR-0100 决定四的三格与未配置一格逐族经端点答各自的码，批文自报租户答坏报文；被拒的登记一个也走不到登记编排。
// 自报租户用本族的合法行加 tenant_id 键：只带那一个键的批文在部分族会因缺格答坏报文，分不出拒的是不是自报租户。
func TestNetworkOperatorRegistryAnswerGrades(t *testing.T) {
	cases := map[string]struct {
		err    error
		token  string
		status int
		code   string
	}{
		"no bearer token":          {token: "", status: http.StatusUnauthorized, code: "OPERATOR_CREDENTIAL_REJECTED"},
		"issuer parameters unset":  {err: networkhttp.ErrAccessChannelNotConfigured, token: "t", status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"no registry grant":        {err: networkhttp.ErrOperatorNotGranted, token: "t", status: http.StatusForbidden, code: "OPERATOR_NOT_GRANTED"},
		"identity dependency down": {err: networkhttp.ErrIdentityDependencyUnavailable, token: "t", status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	for name, testCase := range cases {
		for family, row := range onlineRegistrationRows {
			t.Run(name+"/"+family, func(t *testing.T) {
				endpoint := registrationEndpoints(operatorIntake(t, operatorAuthenticator{err: testCase.err}), unreachableRegistrar(t))[family]
				recorder := httptest.NewRecorder()
				endpoint.ServeHTTP(recorder, operatorRegistration(testCase.token, strings.NewReader(row)))
				if code := problemCode(t, recorder); recorder.Code != testCase.status || code != testCase.code {
					t.Fatalf("答 %d %q，想要 %d %q", recorder.Code, code, testCase.status, testCase.code)
				}
			})
		}
	}
	for family, row := range onlineRegistrationRows {
		t.Run("self-reported tenant/"+family, func(t *testing.T) {
			endpoint := registrationEndpoints(operatorIntake(t, operatorAuthenticator{}), unreachableRegistrar(t))[family]
			body := strings.Replace(row, "{", `{"tenant_id": "SYN-TENANT-02", `, 1)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, operatorRegistration("t", strings.NewReader(body)))
			if code := problemCode(t, recorder); recorder.Code != http.StatusBadRequest || code != "MALFORMED_REQUEST" {
				t.Fatalf("答 %d %q，想要 400 MALFORMED_REQUEST", recorder.Code, code)
			}
		})
	}
}

// readCounter 数批文被读了几次。
type readCounter struct {
	reader io.Reader
	reads  int
}

func (counter *readCounter) Read(p []byte) (int, error) {
	counter.reads++
	return counter.reader.Read(p)
}

// Covers: 先认证、后读批文——令牌不过的调用方看不到批文校验的结果，批文一个字节也不读。
func TestNetworkOperatorRegistryIntakeAuthenticatesBeforeReadingTheBody(t *testing.T) {
	intake := operatorIntake(t, operatorAuthenticator{})
	body := &readCounter{reader: strings.NewReader(`not json`)}
	_, err := intake.IntakeNodeVersionRegistration(context.Background(), operatorRegistration("", body))
	if !errors.Is(err, networkhttp.ErrOperatorCredentialRejected) || body.reads != 0 {
		t.Fatalf("不带令牌：err %v、批文读了 %d 次，想要凭证不过且一次未读", err, body.reads)
	}
}

// Covers: 批文读取上限一兆字节（本包 maxRegistrationBytes）：恰在上限的合法批文照常译，多一个字节即坏报文。
func TestNetworkOperatorRegistryIntakeCapsTheBodyAtOneMebibyte(t *testing.T) {
	intake := operatorIntake(t, operatorAuthenticator{})
	row := onlineRegistrationRows["node"]
	padded := func(size int) io.Reader {
		return strings.NewReader(strings.Repeat(" ", size-len(row)) + row)
	}
	if _, err := intake.IntakeNodeVersionRegistration(context.Background(), operatorRegistration("t", padded(1<<20))); err != nil {
		t.Fatalf("恰一兆字节：err = %v，想要照常译", err)
	}
	_, err := intake.IntakeNodeVersionRegistration(context.Background(), operatorRegistration("t", padded(1<<20+1)))
	if !errors.Is(err, networkhttp.ErrMalformedRequest) {
		t.Fatalf("超一个字节：err = %v，想要 ErrMalformedRequest", err)
	}
}
