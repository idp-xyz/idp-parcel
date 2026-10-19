package nodeopshttp_test

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

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证送站预报两口的传输层（票 ps-external-mark-relations/09；UC-NO-006 应用流程步骤 1、2；ADR-0139 决定四；ADR-0022）：信封
// 各格逐格映射、各对一种恢复动作；认证过了才读载荷；租户、客户账户与来源只从认证结果来，载荷封闭，坏载荷各种答 400；业务格首次
// 答 201、已有结果与来源冲突答 200 带 outcome、指名的预报看不见答 404。夹具全为合成（S 级）。

const (
	preAdviceTenant  = "SYN-TENANT-01"
	preAdviceAccount = "SYN-ACCOUNT-01"
	preAdviceChannel = "https://auth.syn.example/customers#SYN-CUSTOMER-CLIENT-01"
)

// customerAuthenticatorDouble 照首方铸造器的次序办：先答认证那几格（err 非空即停，不读请求），过了才调读请求的口；接入请求标识
// 空白时答推导不出——真适配器把那一格译成坏载荷。
type customerAuthenticatorDouble struct {
	err        error
	credential *nodeopshttp.PresentedCustomerCredential
	asked      []string
}

func (double *customerAuthenticatorDouble) authenticate(line string, credential nodeopshttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (nodeopshttp.CustomerIdentity, error) {
	double.asked = append(double.asked, line)
	double.credential = &credential
	if double.err != nil {
		return nodeopshttp.CustomerIdentity{}, double.err
	}
	id, err := accessRequestID()
	if err != nil {
		return nodeopshttp.CustomerIdentity{}, err
	}
	if strings.TrimSpace(id) == "" {
		return nodeopshttp.CustomerIdentity{}, fmt.Errorf("%w: accessRequestId is absent", nodeopshttp.ErrMalformedRequest)
	}
	tenant, _ := domain.NewTenantID(preAdviceTenant)
	account, _ := domain.NewCustomerAccountReference(preAdviceAccount)
	return nodeopshttp.CustomerIdentity{Tenant: tenant, CustomerAccount: account, Channel: preAdviceChannel, RequestKey: strings.TrimSpace(id)}, nil
}

func (double *customerAuthenticatorDouble) AuthenticateDeliveryPreAdviceSubmission(_ context.Context, credential nodeopshttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (nodeopshttp.CustomerIdentity, error) {
	return double.authenticate("submission", credential, accessRequestID)
}

func (double *customerAuthenticatorDouble) AuthenticateDeliveryPreAdviceWithdrawal(_ context.Context, credential nodeopshttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (nodeopshttp.CustomerIdentity, error) {
	return double.authenticate("withdrawal", credential, accessRequestID)
}

// numberGateDouble 拒带 REJECT 字样的号，其余原样放行——界归 PS，这里只证拒了的号答坏载荷。
type numberGateDouble struct{}

func (numberGateDouble) AdmitPreAdvisedNumber(value string) (domain.PreAdvisedParcelNumber, error) {
	if strings.Contains(value, "REJECT") {
		return domain.PreAdvisedParcelNumber{}, ports.ErrPreAdvisedNumberNotAdmitted
	}
	return domain.NewPreAdvisedParcelNumber(value)
}

// preAdviceHandlerDouble 记下收到的命令，按给定的结果作答。
type preAdviceHandlerDouble struct {
	submitted  []application.SubmitDeliveryPreAdviceCommand
	withdrawn  []application.WithdrawDeliveryPreAdviceCommand
	submission application.DeliveryPreAdviceSubmissionResult
	withdrawal application.DeliveryPreAdviceWithdrawalResult
	err        error
}

func (double *preAdviceHandlerDouble) Submit(_ context.Context, command application.SubmitDeliveryPreAdviceCommand) (application.DeliveryPreAdviceSubmissionResult, error) {
	double.submitted = append(double.submitted, command)
	return double.submission, double.err
}

func (double *preAdviceHandlerDouble) Withdraw(_ context.Context, command application.WithdrawDeliveryPreAdviceCommand) (application.DeliveryPreAdviceWithdrawalResult, error) {
	double.withdrawn = append(double.withdrawn, command)
	return double.withdrawal, double.err
}

func registeredPreAdvice(t *testing.T) domain.DeliveryPreAdvice {
	t.Helper()
	spec := domain.DeliveryPreAdviceSpec{
		ExpectedArrivalAt: time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC),
		Source:            domain.CustomerRequestSource{Channel: preAdviceChannel, RequestKey: "SYN-ACCESS-REQUEST-PA-0001", ContentDigest: "NOC-1:00"},
		FormedAt:          time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC),
	}
	var err error
	if spec.TenantID, err = domain.NewTenantID(preAdviceTenant); err != nil {
		t.Fatal(err)
	}
	if spec.ID, err = domain.NewDeliveryPreAdviceID("PREADV-SYN-0001"); err != nil {
		t.Fatal(err)
	}
	if spec.CustomerAccount, err = domain.NewCustomerAccountReference(preAdviceAccount); err != nil {
		t.Fatal(err)
	}
	if spec.Node, err = domain.NewNodeReference("SYN-NODE-SHA-HUB"); err != nil {
		t.Fatal(err)
	}
	if spec.DeliveringParty, err = domain.NewDeliveringPartyReference("SYN-DRIVER-01"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"SYN-PCL-PA-0001", "SYN-PCL-PA-0002"} {
		number, err := domain.NewPreAdvisedParcelNumber(value)
		if err != nil {
			t.Fatal(err)
		}
		spec.Numbers = append(spec.Numbers, number)
	}
	preAdvice, err := domain.FormDeliveryPreAdvice(spec)
	if err != nil {
		t.Fatal(err)
	}
	return preAdvice
}

func recordedWithdrawal(t *testing.T) domain.DeliveryPreAdviceWithdrawal {
	t.Helper()
	spec := domain.DeliveryPreAdviceWithdrawalSpec{
		Source:      domain.CustomerRequestSource{Channel: preAdviceChannel, RequestKey: "SYN-ACCESS-REQUEST-PA-0003", ContentDigest: "NOC-1:01"},
		WithdrawnAt: time.Date(2026, 10, 19, 7, 0, 0, 0, time.UTC),
	}
	var err error
	if spec.TenantID, err = domain.NewTenantID(preAdviceTenant); err != nil {
		t.Fatal(err)
	}
	if spec.PreAdvice, err = domain.NewDeliveryPreAdviceID("PREADV-SYN-0001"); err != nil {
		t.Fatal(err)
	}
	if spec.CustomerAccount, err = domain.NewCustomerAccountReference(preAdviceAccount); err != nil {
		t.Fatal(err)
	}
	withdrawal, err := domain.FormDeliveryPreAdviceWithdrawal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return withdrawal
}

const preAdviceSubmissionBody = `{
  "accessRequestId": " SYN-ACCESS-REQUEST-PA-0001 ",
  "node": "SYN-NODE-SHA-HUB",
  "expectedArrivalAt": "2026-10-20T10:00:00+08:00",
  "customerParcelNumbers": ["SYN-PCL-PA-0001", "SYN-PCL-PA-0002"],
  "deliveringParty": "SYN-DRIVER-01"
}`

const preAdviceWithdrawalBody = `{"accessRequestId": "SYN-ACCESS-REQUEST-PA-0003", "preAdviceId": "PREADV-SYN-0001"}`

type preAdviceRig struct {
	authenticator *customerAuthenticatorDouble
	handler       *preAdviceHandlerDouble
	submit        http.Handler
	withdraw      http.Handler
}

func newPreAdviceRig(t *testing.T) *preAdviceRig {
	t.Helper()
	authenticator := &customerAuthenticatorDouble{}
	intake, err := nodeopshttp.NewCustomerPreAdviceIntake(authenticator, numberGateDouble{})
	if err != nil {
		t.Fatal(err)
	}
	handler := &preAdviceHandlerDouble{}
	return &preAdviceRig{
		authenticator: authenticator,
		handler:       handler,
		submit:        nodeopshttp.NewSubmitDeliveryPreAdviceEndpoint(intake, handler),
		withdraw:      nodeopshttp.NewWithdrawDeliveryPreAdviceEndpoint(intake, handler),
	}
}

func post(endpoint http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/node-operations/delivery-pre-advices", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.customer.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

func decodeJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
}

type preAdviceResponse struct {
	Outcome   string `json:"outcome"`
	PreAdvice struct {
		PreAdviceID           string   `json:"preAdviceId"`
		CustomerAccount       string   `json:"customerAccount"`
		Node                  string   `json:"node"`
		ExpectedArrivalAt     string   `json:"expectedArrivalAt"`
		CustomerParcelNumbers []string `json:"customerParcelNumbers"`
		DeliveringParty       string   `json:"deliveringParty"`
		FormedAt              string   `json:"formedAt"`
	} `json:"preAdvice"`
	Withdrawal struct {
		PreAdviceID string `json:"preAdviceId"`
		WithdrawnAt string `json:"withdrawnAt"`
	} `json:"withdrawal"`
}

// Covers: AT-NO-069（传输半边）——认证过了才读载荷；命令的租户、客户账户、客户端与来源请求键整组取自认证结果；节点、预计送达时刻、
// 号码（逐字、照交来的次序）与交付方线索取自载荷；Bearer 令牌原样交给认证方。首次答 201 送站预报已登记，回显签发的预报标识。
func TestSubmittingAPreAdviceTranslatesThePayloadAndAnswersRegistered(t *testing.T) {
	rig := newPreAdviceRig(t)
	rig.handler.submission = application.NewDeliveryPreAdviceSubmissionResult(application.DeliveryPreAdviceRegistered, registeredPreAdvice(t))

	response := post(rig.submit, preAdviceSubmissionBody)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d %s, want 201", response.Code, response.Body.String())
	}
	var body preAdviceResponse
	decodeJSON(t, response, &body)
	if body.Outcome != "DELIVERY_PRE_ADVICE_REGISTERED" || body.PreAdvice.PreAdviceID != "PREADV-SYN-0001" ||
		body.PreAdvice.CustomerAccount != preAdviceAccount || body.PreAdvice.Node != "SYN-NODE-SHA-HUB" ||
		body.PreAdvice.ExpectedArrivalAt != "2026-10-20T02:00:00Z" || body.PreAdvice.FormedAt != "2026-10-19T06:00:00Z" ||
		strings.Join(body.PreAdvice.CustomerParcelNumbers, ",") != "SYN-PCL-PA-0001,SYN-PCL-PA-0002" || body.PreAdvice.DeliveringParty != "SYN-DRIVER-01" {
		t.Fatalf("答复 = %s", response.Body.String())
	}
	if rig.authenticator.credential == nil || rig.authenticator.credential.Token != "presented.customer.token" {
		t.Fatalf("交给认证方的出示 = %+v", rig.authenticator.credential)
	}
	if len(rig.handler.submitted) != 1 {
		t.Fatalf("编排收到 %d 条命令", len(rig.handler.submitted))
	}
	command := rig.handler.submitted[0]
	if command.TenantID.String() != preAdviceTenant || command.CustomerAccount.String() != preAdviceAccount ||
		command.Channel != preAdviceChannel || command.RequestKey != "SYN-ACCESS-REQUEST-PA-0001" ||
		command.Node.String() != "SYN-NODE-SHA-HUB" || !command.ExpectedArrivalAt.Equal(time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC)) ||
		len(command.Numbers) != 2 || command.Numbers[0].String() != "SYN-PCL-PA-0001" || command.Numbers[1].String() != "SYN-PCL-PA-0002" ||
		command.DeliveringParty.String() != "SYN-DRIVER-01" {
		t.Fatalf("命令 = %+v", command)
	}

	clueless := strings.Replace(preAdviceSubmissionBody, `,
  "deliveringParty": "SYN-DRIVER-01"`, "", 1)
	if response := post(rig.submit, clueless); response.Code != http.StatusCreated {
		t.Fatalf("不给交付方线索：%d %s", response.Code, response.Body.String())
	}
	if clue := rig.handler.submitted[1].DeliveringParty; clue != (domain.DeliveringPartyReference{}) {
		t.Fatalf("没给线索却带了 %q", clue)
	}
}

// Covers: AT-NO-069 与 AT-NO-070 的答复格——已有结果与来源冲突都是形成了的答案，答 200 带 outcome 并回显在册那一份；编排拒形答
// 400；指名的预报看不见答 404，与越权探针同答；编排读写不动答 500，不带 outcome。撤回首次答 201 送站预报已撤回。
func TestPreAdviceLinesMapTheirOutcomes(t *testing.T) {
	for name, testCase := range map[string]struct {
		submission application.DeliveryPreAdviceSubmissionResult
		withdrawal application.DeliveryPreAdviceWithdrawalResult
		err        error
		withdraw   bool
		status     int
		outcome    string
		code       string
	}{
		"existing pre-advice":     {submission: application.NewDeliveryPreAdviceSubmissionResult(application.DeliveryPreAdviceExistingResult, registeredPreAdvice(t)), status: http.StatusOK, outcome: "EXISTING_RESULT"},
		"pre-advice conflict":     {submission: application.NewDeliveryPreAdviceSubmissionResult(application.DeliveryPreAdviceSourceConflict, registeredPreAdvice(t)), status: http.StatusOK, outcome: "SOURCE_CONFLICT"},
		"pre-advice rejected":     {err: fmt.Errorf("%w: repeated number", application.ErrDeliveryPreAdviceRejected), status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"pre-advice unanswered":   {err: errors.New("connection refused"), status: http.StatusInternalServerError, code: "NO_ANSWER_FORMED"},
		"withdrawn":               {withdraw: true, withdrawal: application.NewDeliveryPreAdviceWithdrawalResult(application.DeliveryPreAdviceWithdrawn, recordedWithdrawal(t)), status: http.StatusCreated, outcome: "DELIVERY_PRE_ADVICE_WITHDRAWN"},
		"existing withdrawal":     {withdraw: true, withdrawal: application.NewDeliveryPreAdviceWithdrawalResult(application.DeliveryPreAdviceExistingResult, recordedWithdrawal(t)), status: http.StatusOK, outcome: "EXISTING_RESULT"},
		"withdrawal conflict":     {withdraw: true, withdrawal: application.NewDeliveryPreAdviceWithdrawalResult(application.DeliveryPreAdviceSourceConflict, recordedWithdrawal(t)), status: http.StatusOK, outcome: "SOURCE_CONFLICT"},
		"withdrawal not visible":  {withdraw: true, err: application.ErrDeliveryPreAdviceNotFound, status: http.StatusNotFound, code: "DELIVERY_PRE_ADVICE_NOT_VISIBLE"},
		"withdrawal rejected":     {withdraw: true, err: application.ErrDeliveryPreAdviceRejected, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"withdrawal unanswered":   {withdraw: true, err: errors.New("connection refused"), status: http.StatusInternalServerError, code: "NO_ANSWER_FORMED"},
		"withdrawal not answered": {withdraw: true, status: http.StatusInternalServerError, code: "UNNAMED_OUTCOME"},
	} {
		t.Run(name, func(t *testing.T) {
			rig := newPreAdviceRig(t)
			rig.handler.submission, rig.handler.withdrawal, rig.handler.err = testCase.submission, testCase.withdrawal, testCase.err
			endpoint, body := rig.submit, preAdviceSubmissionBody
			if testCase.withdraw {
				endpoint, body = rig.withdraw, preAdviceWithdrawalBody
			}
			response := post(endpoint, body)
			if response.Code != testCase.status {
				t.Fatalf("status = %d %s, want %d", response.Code, response.Body.String(), testCase.status)
			}
			if testCase.code != "" {
				if got := problemCode(t, response); got != testCase.code {
					t.Fatalf("code = %q, want %q", got, testCase.code)
				}
				assertNoOutcome(t, response)
				return
			}
			var decoded preAdviceResponse
			decodeJSON(t, response, &decoded)
			if decoded.Outcome != testCase.outcome {
				t.Fatalf("outcome = %q, want %q", decoded.Outcome, testCase.outcome)
			}
			if testCase.withdraw && (decoded.Withdrawal.PreAdviceID != "PREADV-SYN-0001" || decoded.Withdrawal.WithdrawnAt != "2026-10-19T07:00:00Z") {
				t.Fatalf("撤回回显 = %s", response.Body.String())
			}
			if !testCase.withdraw && decoded.PreAdvice.PreAdviceID != "PREADV-SYN-0001" {
				t.Fatalf("预报回显 = %s", response.Body.String())
			}
		})
	}
}

// Covers: AT-NO-070（传输半边）——撤回命令的租户、客户账户与来源只取认证结果，载荷只指名预报标识；撤回口走撤回那一格认证。
func TestWithdrawingAPreAdviceTakesTheAccountFromTheEnvelope(t *testing.T) {
	rig := newPreAdviceRig(t)
	rig.handler.withdrawal = application.NewDeliveryPreAdviceWithdrawalResult(application.DeliveryPreAdviceWithdrawn, recordedWithdrawal(t))
	if response := post(rig.withdraw, preAdviceWithdrawalBody); response.Code != http.StatusCreated {
		t.Fatalf("status = %d %s", response.Code, response.Body.String())
	}
	if len(rig.authenticator.asked) != 1 || rig.authenticator.asked[0] != "withdrawal" {
		t.Fatalf("认证走了 %v", rig.authenticator.asked)
	}
	command := rig.handler.withdrawn[0]
	if command.TenantID.String() != preAdviceTenant || command.CustomerAccount.String() != preAdviceAccount ||
		command.Channel != preAdviceChannel || command.RequestKey != "SYN-ACCESS-REQUEST-PA-0003" || command.PreAdvice.String() != "PREADV-SYN-0001" {
		t.Fatalf("命令 = %+v", command)
	}
}

// Covers: 票 ps-external-mark-relations/09 完成判据「令牌无效 401、不在册或没有这一格授予 403、不在准入范围 403、依赖读不动 503、发行方
// 未配置 403」与 ADR-0139 决定四——两口各格逐格映射、错误码命名状态；认证没过时载荷一个字节都不读，编排一次都不调。
func TestPreAdviceLinesAnswerTheEnvelopeGrades(t *testing.T) {
	for name, testCase := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"issuer unset":        {nodeopshttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"token rejected":      {nodeopshttp.ErrCustomerClientCredentialRejected, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"not granted":         {nodeopshttp.ErrCustomerClientNotGranted, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"outside admission":   {nodeopshttp.ErrOutsideAdmissionScope, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		"dependency unusable": {nodeopshttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"unclassified":        {errors.New("boom"), http.StatusInternalServerError, "INTAKE_FAILED"},
	} {
		for _, line := range []string{"submission", "withdrawal"} {
			t.Run(name+"/"+line, func(t *testing.T) {
				rig := newPreAdviceRig(t)
				rig.authenticator.err = fmt.Errorf("wrapped: %w", testCase.err)
				endpoint := rig.submit
				if line == "withdrawal" {
					endpoint = rig.withdraw
				}
				probe := &readProbe{}
				request := httptest.NewRequest(http.MethodPost, "/node-operations/delivery-pre-advices", probe)
				response := httptest.NewRecorder()
				endpoint.ServeHTTP(response, request)
				if response.Code != testCase.status || problemCode(t, response) != testCase.code {
					t.Fatalf("answer = %d %q, want %d %q", response.Code, problemCode(t, response), testCase.status, testCase.code)
				}
				assertNoOutcome(t, response)
				if probe.read {
					t.Fatal("认证没过却读了载荷")
				}
				if len(rig.handler.submitted)+len(rig.handler.withdrawn) != 0 {
					t.Fatal("认证没过却调了编排")
				}
			})
		}
	}
}

// Covers: AT-NO-069「号码为空或重复、请求自带租户或客户账户答坏载荷」与票 ps-external-mark-relations/09 做什么「封闭解码」——自报的
// 租户、客户账户与来源都落在未知字段；缺格、空白、带 NUL、时刻解不出、号码进不了形状门、尾随内容、不是 JSON、缺接入请求标识，
// 两口一律答 400，编排一次都不调。
func TestMalformedPreAdvicePayloadsAnswerBadRequest(t *testing.T) {
	submission := func(mutate func(map[string]any)) string {
		document := map[string]any{
			"accessRequestId":       "SYN-ACCESS-REQUEST-PA-0001",
			"node":                  "SYN-NODE-SHA-HUB",
			"expectedArrivalAt":     "2026-10-20T02:00:00Z",
			"customerParcelNumbers": []string{"SYN-PCL-PA-0001"},
		}
		mutate(document)
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	cases := map[string]string{
		"self-reported tenant":  submission(func(document map[string]any) { document["tenantId"] = "TENANT-PROD-9" }),
		"self-reported account": submission(func(document map[string]any) { document["customerAccountId"] = "ACCOUNT-PROD-9" }),
		"self-reported source":  submission(func(document map[string]any) { document["source"] = "SYN-OTHER-CLIENT" }),
		"no numbers":            submission(func(document map[string]any) { document["customerParcelNumbers"] = []string{} }),
		"numbers absent":        submission(func(document map[string]any) { delete(document, "customerParcelNumbers") }),
		"repeated number": submission(func(document map[string]any) {
			document["customerParcelNumbers"] = []string{"SYN-PCL-PA-0001", "SYN-PCL-PA-0001"}
		}),
		"blank number":             submission(func(document map[string]any) { document["customerParcelNumbers"] = []string{" "} }),
		"number refused by gate":   submission(func(document map[string]any) { document["customerParcelNumbers"] = []string{"SYN-REJECT-0001"} }),
		"node absent":              submission(func(document map[string]any) { delete(document, "node") }),
		"node blank":               submission(func(document map[string]any) { document["node"] = " " }),
		"node with NUL":            submission(func(document map[string]any) { document["node"] = "SYN-NODE\x00" }),
		"arrival absent":           submission(func(document map[string]any) { delete(document, "expectedArrivalAt") }),
		"arrival not a time":       submission(func(document map[string]any) { document["expectedArrivalAt"] = "tomorrow" }),
		"blank delivering party":   submission(func(document map[string]any) { document["deliveringParty"] = " " }),
		"access request id absent": submission(func(document map[string]any) { delete(document, "accessRequestId") }),
		"trailing content":         submission(func(map[string]any) {}) + `{}`,
		"not JSON":                 `accessRequestId=SYN`,
	}
	for name, body := range cases {
		t.Run("submission/"+name, func(t *testing.T) {
			rig := newPreAdviceRig(t)
			response := post(rig.submit, body)
			if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
				t.Fatalf("answer = %d %s, want 400 MALFORMED_REQUEST", response.Code, response.Body.String())
			}
			if len(rig.handler.submitted) != 0 {
				t.Fatal("坏载荷到了编排")
			}
		})
	}
	for name, body := range map[string]string{
		"self-reported account":    `{"accessRequestId": "SYN-ACCESS-REQUEST-PA-0003", "preAdviceId": "PREADV-SYN-0001", "customerAccountId": "SYN-ACCOUNT-02"}`,
		"pre-advice absent":        `{"accessRequestId": "SYN-ACCESS-REQUEST-PA-0003"}`,
		"pre-advice blank":         `{"accessRequestId": "SYN-ACCESS-REQUEST-PA-0003", "preAdviceId": " "}`,
		"pre-advice with NUL":      `{"accessRequestId": "SYN-ACCESS-REQUEST-PA-0003", "preAdviceId": "PREADV\u0000"}`,
		"access request id absent": `{"preAdviceId": "PREADV-SYN-0001"}`,
	} {
		t.Run("withdrawal/"+name, func(t *testing.T) {
			rig := newPreAdviceRig(t)
			response := post(rig.withdraw, body)
			if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
				t.Fatalf("answer = %d %s, want 400 MALFORMED_REQUEST", response.Code, response.Body.String())
			}
			if len(rig.handler.withdrawn) != 0 {
				t.Fatal("坏载荷到了编排")
			}
		})
	}
}

// Covers: ADR-0055——首方族没接上时两口答未配置，不读请求；方法不对仍由处理器自己答 405。
func TestPreAdviceLinesAnswerUnconfiguredAndKeepTheMethodGate(t *testing.T) {
	handler := &preAdviceHandlerDouble{}
	for name, endpoint := range map[string]http.Handler{
		"submission": nodeopshttp.NewSubmitDeliveryPreAdviceEndpoint(nodeopshttp.UnconfiguredIntake{}, handler),
		"withdrawal": nodeopshttp.NewWithdrawDeliveryPreAdviceEndpoint(nodeopshttp.UnconfiguredIntake{}, handler),
	} {
		probe := &readProbe{}
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/node-operations/delivery-pre-advices", probe))
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" || probe.read {
			t.Fatalf("%s：%d %s（读了请求：%v）", name, response.Code, response.Body.String(), probe.read)
		}
		response = httptest.NewRecorder()
		endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/node-operations/delivery-pre-advices", nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("%s GET：%d Allow=%q", name, response.Code, response.Header().Get("Allow"))
		}
	}
	if len(handler.submitted)+len(handler.withdrawn) != 0 {
		t.Fatal("未配置的口调了编排")
	}
	if _, err := nodeopshttp.NewCustomerPreAdviceIntake(nil, numberGateDouble{}); err == nil {
		t.Fatal("缺认证方也建起来了")
	}
	if _, err := nodeopshttp.NewCustomerPreAdviceIntake(&customerAuthenticatorDouble{}, nil); err == nil {
		t.Fatal("缺号码形状门也建起来了")
	}
}
