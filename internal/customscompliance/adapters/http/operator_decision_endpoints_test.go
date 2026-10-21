package customshttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// decisionAuthenticator 是运营决定认证方的替身：记下每次被问的决定种类，按需交回一格渠道错误。
type decisionAuthenticator struct {
	err   error
	asked *[]customshttp.OperatorDecision
}

func (fake decisionAuthenticator) AuthenticateOperatorDecision(
	_ context.Context,
	token string,
	decision customshttp.OperatorDecision,
) (customshttp.OperatorIdentity, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, decision)
	}
	if token == "" {
		return customshttp.OperatorIdentity{}, customshttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return customshttp.OperatorIdentity{}, fake.err
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	return customshttp.OperatorIdentity{Tenant: tenant, Operator: "https://issuer#alice"}, err
}

type capturingEstablisher struct {
	command application.EstablishCaseForParcelsCommand
	calls   int
}

func (fake *capturingEstablisher) Handle(_ context.Context, command application.EstablishCaseForParcelsCommand) (application.EstablishCaseResult, error) {
	fake.command = command
	fake.calls++
	return application.EstablishCaseResult{}, nil
}

type capturingConfirmer struct {
	command application.ConfirmSubmissionAuthorizationCommand
}

func (fake *capturingConfirmer) Handle(_ context.Context, command application.ConfirmSubmissionAuthorizationCommand) (application.AuthorizationConfirmationResult, error) {
	fake.command = command
	return application.AuthorizationConfirmationResult{}, nil
}

type capturingSubmitter struct {
	command application.SubmitDeclarationCommand
}

func (fake *capturingSubmitter) Handle(_ context.Context, command application.SubmitDeclarationCommand) (application.SubmitDeclarationResult, error) {
	fake.command = command
	return application.SubmitDeclarationResult{}, nil
}

const (
	caseEstablishmentBody = `{"jurisdiction":"SYN-JUR-CN","direction":"EXPORT","procedure":"SYN-PROC-CN-EXPORT","obligation":"SYN-OBL-01",` +
		`"parcels":["DPCL-1"],"roles":[{"role":"DECLARANT","party":"SYN-PARTY-BROKER-01","authority":"SYN-MANDATE-01"}]}`
	authorizationBody = `{"unitId":"SYN-UNIT-01"}`
	submissionBody    = `{"unitId":"SYN-UNIT-01","caseId":"CASE-1","procedure":"SYN-PROC-CN-EXPORT","members":["DPCL-1"],` +
		`"dossier":"SYN-DOSSIER-01","roles":"SYN-ROLES-01","target":"SYN-CHANNEL-01"}`
)

func post(target, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer t")
	return request
}

// Covers: AT-CC-445（Intake 半边）——三口各按自己那一种运营决定认证；租户取认证结果，确认人取认证出的操作者，所凭授予取本口
// 的决定种类；提交执行口的首次尝试一律是`已登记待发送`、没有发送时间。
func TestEachCustomsDecisionFaceAuthenticatesItsOwnKindAndTakesIdentityFromTheEnvelope(t *testing.T) {
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}

	establisher := &capturingEstablisher{}
	customshttp.NewEstablishCaseEndpoint(intake, establisher).ServeHTTP(httptest.NewRecorder(), post("/customs-cases", caseEstablishmentBody))
	confirmer := &capturingConfirmer{}
	customshttp.NewConfirmSubmissionAuthorizationEndpoint(intake, confirmer).ServeHTTP(httptest.NewRecorder(), post("/customs-submission-authorizations", authorizationBody))
	submitter := &capturingSubmitter{}
	customshttp.NewSubmitDeclarationEndpoint(intake, submitter).ServeHTTP(httptest.NewRecorder(), post("/customs-declaration-submissions", submissionBody))

	if len(asked) != 3 || asked[0] != "CUSTOMS_CASE_ESTABLISHMENT" || asked[1] != "CUSTOMS_SUBMISSION_AUTHORIZATION" ||
		asked[2] != "CUSTOMS_DECLARATION_SUBMISSION" {
		t.Fatalf("asked = %v", asked)
	}
	established := establisher.command
	if established.TenantID.String() != "SYN-TENANT-01" || established.Direction != domain.ExportManifest ||
		established.Jurisdiction.String() != "SYN-JUR-CN" || len(established.Parcels) != 1 || established.Parcels[0] != "DPCL-1" ||
		len(established.Roles) != 1 || established.Roles[0].Party != "SYN-PARTY-BROKER-01" {
		t.Fatalf("case command = %+v", established)
	}
	if confirmer.command.TenantID.String() != "SYN-TENANT-01" || confirmer.command.UnitID != "SYN-UNIT-01" ||
		confirmer.command.Confirmer != "https://issuer#alice" || confirmer.command.Grant != "CUSTOMS_SUBMISSION_AUTHORIZATION" {
		t.Fatalf("confirmation command = %+v", confirmer.command)
	}
	submitted := submitter.command
	if submitted.TenantID.String() != "SYN-TENANT-01" || submitted.CaseID != "CASE-1" || submitted.Target != "SYN-CHANNEL-01" ||
		submitted.InitialResult != domain.AttemptRegisteredPendingSend || !submitted.SentAt.IsZero() {
		t.Fatalf("submission command = %+v", submitted)
	}
}

// Covers: AT-CC-437、AT-CC-441（Intake 半边）——载荷是封闭的：建案口不收客户归属与来源引用，授权确认口不收确认人，提交执行口
// 不收尝试结果与发送时间，任何口都不收租户；带了即坏报文，编排一次也不被调到。方向给了却不是两值之一同样是坏报文。
func TestCustomsDecisionPayloadsRefuseIdentityAndSendFacts(t *testing.T) {
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	establisher := &capturingEstablisher{}
	for name, test := range map[string]struct {
		handler http.Handler
		body    string
	}{
		"建案带租户":    {customshttp.NewEstablishCaseEndpoint(intake, establisher), `{"tenantId":"T","parcels":["DPCL-1"]}`},
		"建案带客户归属":  {customshttp.NewEstablishCaseEndpoint(intake, establisher), `{"parcels":[{"parcel":"DPCL-1","customer":"C"}]}`},
		"建案方向不认识":  {customshttp.NewEstablishCaseEndpoint(intake, establisher), `{"direction":"OUTBOUND","parcels":["DPCL-1"]}`},
		"授权确认带确认人": {customshttp.NewConfirmSubmissionAuthorizationEndpoint(intake, &capturingConfirmer{}), `{"unitId":"U","confirmer":"bob"}`},
		"提交带尝试结果":  {customshttp.NewSubmitDeclarationEndpoint(intake, &capturingSubmitter{}), `{"unitId":"U","initialResult":"ACKNOWLEDGED"}`},
		"提交带发送时间":  {customshttp.NewSubmitDeclarationEndpoint(intake, &capturingSubmitter{}), `{"unitId":"U","sentAt":"2026-10-20T00:00:00Z"}`},
		"提交多余内容":   {customshttp.NewSubmitDeclarationEndpoint(intake, &capturingSubmitter{}), `{"unitId":"U"} {}`},
	} {
		response := httptest.NewRecorder()
		test.handler.ServeHTTP(response, post("/x", test.body))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "MALFORMED_REQUEST") {
			t.Fatalf("%s: %d %s", name, response.Code, response.Body.String())
		}
	}
	if establisher.calls != 0 {
		t.Fatal("坏报文到了建案编排")
	}
}

// Covers: 渠道各格逐格映射（ADR-0100 决定四、ADR-0149 决定四）：未配置 403、令牌不过 401、无此种授予 403、不在准入范围 403、
// 依赖读不动 503；方法不对 405。
func TestCustomsDecisionFacesMapChannelAnswersGradeByGrade(t *testing.T) {
	for name, test := range map[string]struct {
		err    error
		token  bool
		status int
		code   string
	}{
		"未配置":    {err: customshttp.ErrAccessChannelNotConfigured, token: true, status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"没令牌":    {status: http.StatusUnauthorized, code: "OPERATOR_CREDENTIAL_REJECTED"},
		"没有这种授予": {err: customshttp.ErrOperatorNotGranted, token: true, status: http.StatusForbidden, code: "OPERATOR_NOT_GRANTED"},
		"不在准入范围": {err: customshttp.ErrOutsideAdmissionScope, token: true, status: http.StatusForbidden, code: "OUTSIDE_ADMISSION_SCOPE"},
		"依赖读不动":  {err: customshttp.ErrIdentityDependencyUnavailable, token: true, status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	} {
		intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{err: test.err})
		if err != nil {
			t.Fatal(err)
		}
		submitter := &capturingSubmitter{}
		request := post("/customs-declaration-submissions", submissionBody)
		if !test.token {
			request.Header.Del("Authorization")
		}
		response := httptest.NewRecorder()
		customshttp.NewSubmitDeclarationEndpoint(intake, submitter).ServeHTTP(response, request)
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
			t.Fatalf("%s: %d %s", name, response.Code, response.Body.String())
		}
		if submitter.command.UnitID != "" {
			t.Fatalf("%s: 渠道没放行却到了编排", name)
		}
	}

	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	customshttp.NewEstablishCaseEndpoint(intake, &capturingEstablisher{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-cases", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET: %d allow=%q", response.Code, response.Header().Get("Allow"))
	}
}

// Covers: 编排交回零值结果（没有名字的格）答 500 UNNAMED_OUTCOME，不以 200 冒充业务答案。各格的业务值由应用层测试与
// cmd/parcel-api 的真库用例钉；这里的替身只交零值。
func TestAnUnnamedOutcomeIsNotPassedOffAsAnAnswer(t *testing.T) {
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	for name, handler := range map[string]http.Handler{
		"建案":   customshttp.NewEstablishCaseEndpoint(intake, &capturingEstablisher{}),
		"授权确认": customshttp.NewConfirmSubmissionAuthorizationEndpoint(intake, &capturingConfirmer{}),
		"提交执行": customshttp.NewSubmitDeclarationEndpoint(intake, &capturingSubmitter{}),
	} {
		body := submissionBody
		switch name {
		case "建案":
			body = caseEstablishmentBody
		case "授权确认":
			body = authorizationBody
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, post("/x", body))
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "UNNAMED_OUTCOME") {
			t.Fatalf("%s：%d %s", name, response.Code, response.Body.String())
		}
	}
}

// noAcceptedParcels 是源上下文的替身：任何包裹都不在已接受委托里。
type noAcceptedParcels struct{}

func (noAcceptedParcels) FindAcceptedParcelSource(context.Context, domain.TenantID, string) (ports.AcceptedParcelSource, bool, error) {
	return ports.AcceptedParcelSource{}, false, nil
}

// Covers: AT-CC-438（答复半边）——包裹不在已接受委托里答 200 PARCEL_NOT_ACCEPTED 并逐件列出；建案编排没被调到，没有案件号。
func TestAnUnacceptedParcelAnswerListsTheParcels(t *testing.T) {
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	handler := application.NewEstablishCaseForParcelsHandler(application.EstablishCaseForParcelsDeps{
		Parcels: noAcceptedParcels{},
		Cases:   application.NewEstablishCaseHandler(application.EstablishCaseDeps{}),
	})
	response := httptest.NewRecorder()
	customshttp.NewEstablishCaseEndpoint(intake, handler).ServeHTTP(response, post("/customs-cases", caseEstablishmentBody))
	var body struct {
		Outcome           string   `json:"outcome"`
		CaseID            string   `json:"caseId"`
		UnacceptedParcels []string `json:"unacceptedParcels"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body.Outcome != "PARCEL_NOT_ACCEPTED" || body.CaseID != "" ||
		len(body.UnacceptedParcels) != 1 || body.UnacceptedParcels[0] != "DPCL-1" {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
}
