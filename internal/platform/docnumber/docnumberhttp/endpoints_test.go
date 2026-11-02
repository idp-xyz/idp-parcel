package docnumberhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

type authenticator struct {
	err error
}

func (fake authenticator) AuthenticateOperationsRead(context.Context, string) (string, error) {
	return "SYN-TENANT-01", fake.err
}

func (fake authenticator) AuthenticateRegistryWrite(context.Context, string) (Registrant, error) {
	return Registrant{Tenant: "SYN-TENANT-01", Operator: "OPERATOR/syn#01"}, fake.err
}

// services 是编排的替身：记下交进来的命令，按 err 与 outcome 答。
type services struct {
	ruleCommand    docnumber.RegisterRuleCommand
	zoneCommand    docnumber.RegisterTimeZoneCommand
	previewRequest docnumber.PreviewRequest
	readTenant     string
	outcome        docnumber.RegistrationOutcome
	err            error
	preview        docnumber.Preview
	view           docnumber.RegisterView
	registeredAt   time.Time
}

func (fake *services) Read(_ context.Context, tenant string) (docnumber.RegisterView, error) {
	fake.readTenant = tenant
	return fake.view, fake.err
}

func (fake *services) Preview(_ context.Context, request docnumber.PreviewRequest) (docnumber.Preview, error) {
	fake.previewRequest = request
	return fake.preview, fake.err
}

func (fake *services) RegisterRule(_ context.Context, command docnumber.RegisterRuleCommand) (docnumber.RuleRegistration, error) {
	fake.ruleCommand = command
	return docnumber.RuleRegistration{Outcome: fake.outcome, Version: docnumber.RuleVersion{
		Tenant: command.Tenant, Type: command.Type, Template: command.Template, EffectiveFrom: command.EffectiveFrom,
		RegisteredBy: command.RegisteredBy, RegisteredAt: fake.registeredAt,
	}}, fake.err
}

func (fake *services) RegisterTimeZone(_ context.Context, command docnumber.RegisterTimeZoneCommand) (docnumber.TimeZoneRegistration, error) {
	fake.zoneCommand = command
	return docnumber.TimeZoneRegistration{Outcome: fake.outcome, Version: docnumber.TimeZoneVersion{
		Tenant: command.Tenant, TimeZone: command.TimeZone, EffectiveFrom: command.EffectiveFrom,
		RegisteredBy: command.RegisteredBy, RegisteredAt: fake.registeredAt,
	}}, fake.err
}

func serve(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decode(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %v %s", err, response.Body.String())
	}
	return body
}

func errorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	problem, _ := decode(t, response)["error"].(map[string]any)
	code, _ := problem["code"].(string)
	return code
}

type endpointCase struct {
	name    string
	handler func(authenticator, *services) (http.Handler, error)
	method  string
	path    string
	body    string
}

var endpointCases = []endpointCase{
	{"read", func(a authenticator, s *services) (http.Handler, error) { return NewReadEndpoint(a, s) }, http.MethodGet, RulesPath, ""},
	{"preview", func(a authenticator, s *services) (http.Handler, error) { return NewPreviewEndpoint(a, s) }, http.MethodPost, PreviewsPath,
		`{"documentType":"SHIPMENT","template":"P{yyyy}{seq:6}"}`},
	{"rule", func(a authenticator, s *services) (http.Handler, error) { return NewRuleRegistrationEndpoint(a, s) }, http.MethodPost, RuleRegistrationsPath,
		`{"documentType":"SHIPMENT","template":"P{yyyy}{seq:6}","effectiveFrom":"2026-12-01T00:00:00+08:00"}`},
	{"time zone", func(a authenticator, s *services) (http.Handler, error) { return NewTimeZoneRegistrationEndpoint(a, s) }, http.MethodPost, TimeZoneRegistrationsPath,
		`{"timeZone":"Asia/Shanghai","effectiveFrom":"2026-01-01T00:00:00+08:00"}`},
}

// Covers: ADR-0100、ADR-0029——四口先认方法、再认证、后解载荷：认证件交回的四格各有其码，令牌不过的调用方看不到载荷
// 校验的结果；方法不对答 405 并带 Allow。
func TestEndpointsAuthenticateBeforeReadingThePayload(t *testing.T) {
	answers := []struct {
		err    error
		status int
		code   string
	}{
		{ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		{errors.New("unnamed"), http.StatusInternalServerError, "INTAKE_FAILED"},
	}
	for _, endpoint := range endpointCases {
		for _, answer := range answers {
			t.Run(endpoint.name+" "+answer.code, func(t *testing.T) {
				fake := &services{}
				handler, err := endpoint.handler(authenticator{err: answer.err}, fake)
				if err != nil {
					t.Fatal(err)
				}
				response := serve(t, handler, endpoint.method, endpoint.path, "{not json")
				if response.Code != answer.status || errorCode(t, response) != answer.code {
					t.Fatalf("got %d %s", response.Code, response.Body.String())
				}
				if fake.readTenant != "" || fake.ruleCommand.Tenant != "" || fake.zoneCommand.Tenant != "" || fake.previewRequest.Tenant != "" {
					t.Fatal("the orchestration ran without authentication")
				}
			})
		}
		t.Run(endpoint.name+" method", func(t *testing.T) {
			handler, err := endpoint.handler(authenticator{}, &services{})
			if err != nil {
				t.Fatal(err)
			}
			response := serve(t, handler, http.MethodDelete, endpoint.path, "")
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != endpoint.method {
				t.Fatalf("got %d allow %q", response.Code, response.Header().Get("Allow"))
			}
		})
		t.Run(endpoint.name+" unconfigured", func(t *testing.T) {
			response := serve(t, HandlerOrUnconfigured(nil, endpoint.method), endpoint.method, endpoint.path, endpoint.body)
			if response.Code != http.StatusForbidden || errorCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("got %d %s", response.Code, response.Body.String())
			}
		})
	}
}

// Covers: ADR-0355 决定五、六与 ADR-0022 的答复映射——登记口租户与登记人只取自信封；首登 201、重放与生效时刻被占 200 带
// outcome 与册上那一版；拒因 400 各有其码；载荷缺生效时刻、带未知字段或尾随内容是坏报文；册写不动答没形成答案。
func TestRegistrationEndpointsMapOutcomesAndRejections(t *testing.T) {
	fake := &services{outcome: docnumber.VersionRegistered, registeredAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}
	rule, err := NewRuleRegistrationEndpoint(authenticator{}, fake)
	if err != nil {
		t.Fatal(err)
	}
	zone, err := NewTimeZoneRegistrationEndpoint(authenticator{}, fake)
	if err != nil {
		t.Fatal(err)
	}

	response := serve(t, rule, http.MethodPost, RuleRegistrationsPath, endpointCases[2].body)
	if response.Code != http.StatusCreated {
		t.Fatalf("first registration: %d %s", response.Code, response.Body.String())
	}
	body := decode(t, response)
	version, _ := body["version"].(map[string]any)
	if body["outcome"] != "VERSION_REGISTERED" || version["documentType"] != "SHIPMENT" || version["registeredBy"] != "OPERATOR/syn#01" {
		t.Fatalf("body: %v", body)
	}
	if fake.ruleCommand.Tenant != "SYN-TENANT-01" || fake.ruleCommand.RegisteredBy != "OPERATOR/syn#01" || fake.ruleCommand.Type != "SHIPMENT" {
		t.Fatalf("command: %+v", fake.ruleCommand)
	}

	for _, outcome := range []docnumber.RegistrationOutcome{docnumber.VersionExisting, docnumber.EffectiveFromTaken} {
		fake.outcome = outcome
		for _, endpoint := range []struct {
			handler http.Handler
			path    string
			body    string
		}{{rule, RuleRegistrationsPath, endpointCases[2].body}, {zone, TimeZoneRegistrationsPath, endpointCases[3].body}} {
			response := serve(t, endpoint.handler, http.MethodPost, endpoint.path, endpoint.body)
			if response.Code != http.StatusOK || decode(t, response)["outcome"] != string(outcome) {
				t.Fatalf("%s %s: %d %s", endpoint.path, outcome, response.Code, response.Body.String())
			}
		}
	}
	if fake.zoneCommand.TimeZone != "Asia/Shanghai" || fake.zoneCommand.RegisteredBy != "OPERATOR/syn#01" {
		t.Fatalf("time zone command: %+v", fake.zoneCommand)
	}

	fake.outcome = docnumber.VersionRegistered
	for reason, code := range map[error]string{
		docnumber.ErrTypeNotDeclared:              "DOCUMENT_TYPE_NOT_DECLARED",
		docnumber.ErrTemplateUnparseable:          "TEMPLATE_UNPARSEABLE",
		docnumber.ErrTemplateWithoutSequence:      "TEMPLATE_WITHOUT_SEQUENCE",
		docnumber.ErrTemplateNeedsUnsuppliedValue: "TEMPLATE_NEEDS_UNSUPPLIED_VALUE",
		docnumber.ErrTemplateUnrenderable:         "TEMPLATE_UNRENDERABLE",
		docnumber.ErrScrambleSecretNotConfigured:  "SCRAMBLE_SECRET_NOT_CONFIGURED",
		docnumber.ErrTimeZoneNotRecognized:        "TIME_ZONE_NOT_RECOGNIZED",
		docnumber.ErrRegistrationIncomplete:       "MALFORMED_REQUEST",
	} {
		fake.err = fmt.Errorf("%w: %w", docnumber.ErrRegistrationRejected, reason)
		response := serve(t, rule, http.MethodPost, RuleRegistrationsPath, endpointCases[2].body)
		if response.Code != http.StatusBadRequest || errorCode(t, response) != code {
			t.Errorf("%v: %d %s", reason, response.Code, response.Body.String())
		}
	}
	fake.err = errors.New("register unavailable")
	if response := serve(t, zone, http.MethodPost, TimeZoneRegistrationsPath, endpointCases[3].body); response.Code != http.StatusInternalServerError || errorCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("dependency failure: %d %s", response.Code, response.Body.String())
	}

	fake.err = nil
	for _, malformed := range []string{
		`{"documentType":"SHIPMENT","template":"P{seq}"}`,
		`{"documentType":"SHIPMENT","template":"P{seq}","effectiveFrom":"2026-12-01T00:00:00Z","tenantId":"SYN-TENANT-02"}`,
		endpointCases[2].body + ` {}`,
		``,
	} {
		if response := serve(t, rule, http.MethodPost, RuleRegistrationsPath, malformed); response.Code != http.StatusBadRequest || errorCode(t, response) != "MALFORMED_REQUEST" {
			t.Errorf("%q: %d %s", malformed, response.Code, response.Body.String())
		}
	}
	if response := serve(t, zone, http.MethodPost, TimeZoneRegistrationsPath, `{"timeZone":"Asia/Shanghai"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("time zone without an effective moment: %d", response.Code)
	}
}

// Covers: ADR-0355 决定六——读口答每类单据此刻的规则、所用时区或「单据编号时区未配置」与历次版本；租户只取自信封。
func TestReadEndpointShape(t *testing.T) {
	registeredAt := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	version := docnumber.RuleVersion{Tenant: "SYN-TENANT-01", Type: "SHIPMENT", Template: "SYN{seq:6}", EffectiveFrom: registeredAt, RegisteredBy: "OPERATOR/syn#01", RegisteredAt: registeredAt}
	fake := &services{view: docnumber.RegisterView{
		Tenant: "SYN-TENANT-01", At: registeredAt,
		Types: []docnumber.TypeView{
			{Declaration: docnumber.TypeDeclaration{Code: "SHIPMENT", DefaultTemplate: "P{yyyy}{mm}{dd}{seq:6}", DateTimeZone: docnumber.DateTimeZoneFromTenant},
				Source: docnumber.SourceTenantRule, Template: "SYN{seq:6}", EffectiveVersion: &version, ResetPeriod: docnumber.ResetContinuous, Versions: []docnumber.RuleVersion{version}},
			{Declaration: docnumber.TypeDeclaration{Code: "DELIVERY_PRE_ADVICE", DefaultTemplate: "PA-{yyyy}{mm}{dd}-{seq:5}", DateTimeZone: docnumber.DateTimeZoneFromIssuer},
				Source: docnumber.SourceProductDefault, Template: "PA-{yyyy}{mm}{dd}-{seq:5}", ResetPeriod: docnumber.ResetDaily},
		},
	}}
	handler, err := NewReadEndpoint(authenticator{}, fake)
	if err != nil {
		t.Fatal(err)
	}
	response := serve(t, handler, http.MethodGet, RulesPath+"?tenant=SYN-TENANT-02", "")
	if response.Code != http.StatusOK || fake.readTenant != "SYN-TENANT-01" {
		t.Fatalf("%d tenant %q", response.Code, fake.readTenant)
	}
	body := decode(t, response)
	zone, _ := body["numberingTimeZone"].(map[string]any)
	types, _ := body["documentTypes"].([]any)
	if body["outcome"] != "DOCUMENT_NUMBERING_READ" || zone["status"] != "NOT_CONFIGURED" || len(types) != 2 {
		t.Fatalf("body: %v", body)
	}
	shipment, _ := types[0].(map[string]any)
	dateZone, _ := shipment["dateTimeZone"].(map[string]any)
	if shipment["source"] != "TENANT_RULE" || shipment["effectiveFrom"] == nil || shipment["resetPeriod"] != "CONTINUOUS" ||
		dateZone["source"] != "TENANT_NUMBERING_TIME_ZONE" || dateZone["numberingTimeZoneStatus"] != "NOT_CONFIGURED" || shipment["sample"] != nil {
		t.Fatalf("shipment: %v", shipment)
	}
	if preAdvice, _ := types[1].(map[string]any); preAdvice["source"] != "PRODUCT_DEFAULT" || preAdvice["effectiveFrom"] != nil {
		t.Fatalf("pre-advice: %v", preAdvice)
	}

	fake.view.NumberingTimeZone = &docnumber.TimeZoneVersion{Tenant: "SYN-TENANT-01", TimeZone: "Asia/Shanghai", EffectiveFrom: registeredAt, RegisteredBy: "SYN", RegisteredAt: registeredAt}
	fake.view.Types[0].Sample = "SYN000001"
	body = decode(t, serve(t, handler, http.MethodGet, RulesPath, ""))
	zone, _ = body["numberingTimeZone"].(map[string]any)
	types, _ = body["documentTypes"].([]any)
	shipment, _ = types[0].(map[string]any)
	dateZone, _ = shipment["dateTimeZone"].(map[string]any)
	if zone["status"] != "CONFIGURED" || zone["timeZone"] != "Asia/Shanghai" || shipment["sample"] != "SYN000001" || dateZone["numberingTimeZone"] != "Asia/Shanghai" {
		t.Fatalf("configured: %v", body)
	}

	fake.err = errors.New("register unavailable")
	if response := serve(t, handler, http.MethodGet, RulesPath, ""); response.Code != http.StatusInternalServerError {
		t.Fatalf("dependency failure: %d", response.Code)
	}
}

// Covers: ADR-0355 决定六、八——预览口把拒因放进 200 的 reason；没有时区答未配置；渲染出样例时带时区、时刻与可能重叠的
// 早先模板；时区与时刻原样交给编排。
func TestPreviewEndpointShape(t *testing.T) {
	fake := &services{}
	handler, err := NewPreviewEndpoint(authenticator{}, fake)
	if err != nil {
		t.Fatal(err)
	}

	fake.preview = docnumber.Preview{Outcome: docnumber.PreviewRejected, Rejection: docnumber.ErrTemplateWithoutSequence}
	body := decode(t, serve(t, handler, http.MethodPost, PreviewsPath, endpointCases[1].body))
	if body["outcome"] != "PREVIEW_REJECTED" || body["reason"] != "TEMPLATE_WITHOUT_SEQUENCE" {
		t.Fatalf("rejected: %v", body)
	}

	fake.preview = docnumber.Preview{Outcome: docnumber.PreviewTimeZoneNotConfigured, ResetPeriod: docnumber.ResetYearly}
	body = decode(t, serve(t, handler, http.MethodPost, PreviewsPath, endpointCases[1].body))
	if body["outcome"] != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" || body["resetPeriod"] != "YEARLY" || body["sample"] != nil {
		t.Fatalf("not configured: %v", body)
	}

	at := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	fake.preview = docnumber.Preview{
		Outcome: docnumber.PreviewRendered, Sample: "P2026000001", TimeZone: "Asia/Shanghai", At: at, ResetPeriod: docnumber.ResetYearly,
		Overlaps: []docnumber.Overlap{{Source: docnumber.SourceProductDefault, Template: "P{yyyy}{mm}{dd}{seq:6}"}},
	}
	response := serve(t, handler, http.MethodPost, PreviewsPath,
		`{"documentType":"SHIPMENT","template":"P{yyyy}{seq:6}","timeZone":"Asia/Shanghai","at":"2026-12-01T00:00:00Z"}`)
	body = decode(t, response)
	overlaps, _ := body["overlaps"].([]any)
	if response.Code != http.StatusOK || body["outcome"] != "PREVIEW_RENDERED" || body["sample"] != "P2026000001" || len(overlaps) != 1 {
		t.Fatalf("rendered: %d %v", response.Code, body)
	}
	if fake.previewRequest.TimeZone != "Asia/Shanghai" || !fake.previewRequest.At.Equal(at) || fake.previewRequest.Tenant != "SYN-TENANT-01" {
		t.Fatalf("request: %+v", fake.previewRequest)
	}

	if response := serve(t, handler, http.MethodPost, PreviewsPath, `{"documentType":"SHIPMENT","bogus":1}`); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", response.Code)
	}
}
