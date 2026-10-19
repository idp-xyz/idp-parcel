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

// 本文件证送站交接单五口的传输层（票 ps-external-mark-relations/12；UC-NO-006；ADR-0149 决定二、ADR-0151 决定一、ADR-0139 决定四；
// ADR-0022）：开立、归入、关闭挂作业事实登记，确认挂运营决定，客户结论挂首方客户渠道；租户、节点、操作者与客户账户只从认证结果来，
// 载荷封闭；信封各格逐格映射；业务格首次形成答 201，其余答 200 带 outcome，看不见的交接单答 404。夹具全为合成（S 级）。

const handoverHTTPNode = "SYN-NODE-SHA-HUB"

// FindRecheckableInHandoverNote 让重核那只收寄库替身也答按交接单那一问：本包的用例都不经交接单门铃重核，交回空即可。
func (store *recheckStore) FindRecheckableInHandoverNote(context.Context, domain.TenantID, domain.DeliveryHandoverNoteID) ([]ports.ReceptionRecord, error) {
	return []ports.ReceptionRecord{}, nil
}

// handoverWorkFactAuthenticatorFake 记下被问的作业事实；没带令牌答令牌不过，err 非空时答它，否则答合成租户与节点。
type handoverWorkFactAuthenticatorFake struct {
	err   error
	asked *[]string
}

func (fake handoverWorkFactAuthenticatorFake) AuthenticateDeliveryHandoverFact(
	_ context.Context, token, device, kind, reference string, fact nodeopshttp.DeliveryHandoverFact,
) (domain.TenantID, domain.NodeReference, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, string(fact)+"|"+device+"|"+kind+"|"+reference)
	}
	if token == "" {
		return domain.TenantID{}, domain.NodeReference{}, nodeopshttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return domain.TenantID{}, domain.NodeReference{}, fake.err
	}
	tenant, err := domain.NewTenantID(workFactTenant)
	if err != nil {
		return domain.TenantID{}, domain.NodeReference{}, err
	}
	node, err := domain.NewNodeReference(handoverHTTPNode)
	return tenant, node, err
}

// customerHandoverAuthenticatorDouble 照首方铸造器的次序办，同预报两口那一只。
type customerHandoverAuthenticatorDouble struct {
	err error
}

func (double customerHandoverAuthenticatorDouble) AuthenticateDeliveryHandoverConclusion(
	_ context.Context, _ nodeopshttp.PresentedCustomerCredential, accessRequestID func() (string, error),
) (nodeopshttp.CustomerIdentity, error) {
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

// handoverHandlerDouble 记下收到的命令，按给定的结果作答。
type handoverHandlerDouble struct {
	opened    []application.OpenDeliveryHandoverNoteCommand
	assigned  []application.AssignToDeliveryHandoverNoteCommand
	closed    []application.CloseDeliveryHandoverNoteCommand
	confirmed []application.ConfirmDeliveryHandoverCommand
	concluded []application.ConcludeDeliveryHandoverCommand
	result    application.DeliveryHandoverResult
	err       error
}

func (double *handoverHandlerDouble) calls() int {
	return len(double.opened) + len(double.assigned) + len(double.closed) + len(double.confirmed) + len(double.concluded)
}

func (double *handoverHandlerDouble) Open(_ context.Context, command application.OpenDeliveryHandoverNoteCommand) (application.DeliveryHandoverResult, error) {
	double.opened = append(double.opened, command)
	return double.result, double.err
}

func (double *handoverHandlerDouble) Assign(_ context.Context, command application.AssignToDeliveryHandoverNoteCommand) (application.DeliveryHandoverResult, error) {
	double.assigned = append(double.assigned, command)
	return double.result, double.err
}

func (double *handoverHandlerDouble) Close(_ context.Context, command application.CloseDeliveryHandoverNoteCommand) (application.DeliveryHandoverResult, error) {
	double.closed = append(double.closed, command)
	return double.result, double.err
}

func (double *handoverHandlerDouble) ConfirmByOperationDecision(_ context.Context, command application.ConfirmDeliveryHandoverCommand) (application.DeliveryHandoverResult, error) {
	double.confirmed = append(double.confirmed, command)
	return double.result, double.err
}

func (double *handoverHandlerDouble) ConcludeByCustomer(_ context.Context, command application.ConcludeDeliveryHandoverCommand) (application.DeliveryHandoverResult, error) {
	double.concluded = append(double.concluded, command)
	return double.result, double.err
}

// handoverLine 是五口之一：路径、合规请求体、出示的身份头，与装这一口的端点。
type handoverLine struct {
	path     string
	body     string
	customer bool
	endpoint func(*testing.T, *handoverHandlerDouble, error) http.Handler
}

func handoverWorkFactIntake(t *testing.T, err error, asked *[]string) *nodeopshttp.WorkFactHandoverNoteIntake {
	t.Helper()
	intake, buildErr := nodeopshttp.NewWorkFactHandoverNoteIntake(handoverWorkFactAuthenticatorFake{err: err, asked: asked})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return intake
}

func handoverDecisionIntake(t *testing.T, err error, asked *[]nodeopshttp.OperatorDecision) *nodeopshttp.OperatorDecisionIntake {
	t.Helper()
	intake, buildErr := nodeopshttp.NewOperatorDecisionIntake(dispositionAuthenticatorFake{err: err, asked: asked})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return intake
}

func handoverCustomerIntake(t *testing.T, err error) *nodeopshttp.CustomerHandoverConclusionIntake {
	t.Helper()
	intake, buildErr := nodeopshttp.NewCustomerHandoverConclusionIntake(customerHandoverAuthenticatorDouble{err: err})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return intake
}

var handoverLines = map[string]handoverLine{
	"open": {
		path: "/node-operations/delivery-handover-notes",
		body: `{"noteId":"SYN-HANDOVER-HTTP-01","deliveringParty":"SYN-DRIVER-HTTP","claimedAccounts":["SYN-ACCOUNT-02","SYN-ACCOUNT-01"],"openedAt":"2026-10-19T08:00:00+08:00"}`,
		endpoint: func(t *testing.T, handler *handoverHandlerDouble, err error) http.Handler {
			return nodeopshttp.NewOpenDeliveryHandoverNoteEndpoint(handoverWorkFactIntake(t, err, nil), handler)
		},
	},
	"assign": {
		path: "/node-operations/delivery-handover-assignments",
		body: `{"factId":"SYN-ASSIGN-HTTP-01","noteId":"SYN-HANDOVER-HTTP-01","unit":"SYN-UNIT-HTTP-01","assignedAt":"2026-10-19T08:30:00+08:00"}`,
		endpoint: func(t *testing.T, handler *handoverHandlerDouble, err error) http.Handler {
			return nodeopshttp.NewAssignToDeliveryHandoverNoteEndpoint(handoverWorkFactIntake(t, err, nil), handler)
		},
	},
	"close": {
		path: "/node-operations/delivery-handover-closures",
		body: `{"factId":"SYN-CLOSE-HTTP-01","noteId":"SYN-HANDOVER-HTTP-01","closedAt":"2026-10-19T09:00:00+08:00"}`,
		endpoint: func(t *testing.T, handler *handoverHandlerDouble, err error) http.Handler {
			return nodeopshttp.NewCloseDeliveryHandoverNoteEndpoint(handoverWorkFactIntake(t, err, nil), handler)
		},
	},
	"confirm": {
		path: "/node-operations/delivery-handover-confirmations",
		body: `{"noteId":"SYN-HANDOVER-HTTP-01","customerAccount":"SYN-ACCOUNT-01","deliveringParticipant":"SYN-PARTY-DELIVERER-01","basis":" SYN-BASIS-LETTER-01 "}`,
		endpoint: func(t *testing.T, handler *handoverHandlerDouble, err error) http.Handler {
			return nodeopshttp.NewConfirmDeliveryHandoverEndpoint(handoverDecisionIntake(t, err, nil), handler)
		},
	},
	"conclude": {
		path:     "/node-operations/delivery-handover-conclusions",
		body:     `{"accessRequestId":"SYN-ACCESS-REQUEST-HN-0001","noteId":"SYN-HANDOVER-HTTP-01","conclusion":"DENIED"}`,
		customer: true,
		endpoint: func(t *testing.T, handler *handoverHandlerDouble, err error) http.Handler {
			return nodeopshttp.NewConcludeDeliveryHandoverEndpoint(handoverCustomerIntake(t, err), handler)
		},
	},
}

func handoverRequest(line handoverLine, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, line.path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.token")
	if !line.customer {
		request.Header.Set("Parcel-Device-Id", "SYN-DEVICE-NODE")
		request.Header.Set("Parcel-Work-Scope-Kind", "NODE")
		request.Header.Set("Parcel-Work-Scope-Reference", presentedScopeReference)
	}
	return request
}

func handoverHTTPNote(t *testing.T, closed bool) (domain.DeliveryHandoverNote, domain.DeliveryHandoverClosure) {
	t.Helper()
	note, err := domain.OpenDeliveryHandoverNote(domain.DeliveryHandoverNoteSpec{
		TenantID:        adapterValue(t, domain.NewTenantID, workFactTenant),
		ID:              adapterValue(t, domain.NewDeliveryHandoverNoteID, "SYN-HANDOVER-HTTP-01"),
		Node:            adapterValue(t, domain.NewNodeReference, handoverHTTPNode),
		DeliveringParty: adapterValue(t, domain.NewDeliveringPartyReference, "SYN-DRIVER-HTTP"),
		ClaimedAccounts: []domain.CustomerAccountReference{
			adapterValue(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"), adapterValue(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-02"),
		},
		OpenedAt: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !closed {
		return note, domain.DeliveryHandoverClosure{}
	}
	closure, err := domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{
		TenantID: note.TenantID(), Note: note.ID(), FactID: "SYN-CLOSE-HTTP-01", ClosedAt: time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return note, closure
}

// Covers: `AT-NO-071`（传输面）、票 12 做什么第 1 项与 ADR-0149 决定二——作业事实登记三口：租户与节点只从认证结果来，设备与作业范围从三个头来，问的是这一口
// 的事实类型（开立、归入、关闭各一种，准入按它判）；交接单标识、事实身份与业务时间照 ADR-0023 从载荷如实收，不重签；声称原样交下。
func TestHandoverWorkFactsTranslateTheirPayloads(t *testing.T) {
	var asked []string
	intake := handoverWorkFactIntake(t, nil, &asked)
	handler := &handoverHandlerDouble{result: application.NewDeliveryHandoverResult(application.DeliveryHandoverNoteOpened, application.DeliveryHandoverAnswer{})}
	for _, endpoint := range []struct {
		line    handoverLine
		handler http.Handler
	}{
		{handoverLines["open"], nodeopshttp.NewOpenDeliveryHandoverNoteEndpoint(intake, handler)},
		{handoverLines["assign"], nodeopshttp.NewAssignToDeliveryHandoverNoteEndpoint(intake, handler)},
		{handoverLines["close"], nodeopshttp.NewCloseDeliveryHandoverNoteEndpoint(intake, handler)},
	} {
		response := httptest.NewRecorder()
		endpoint.handler.ServeHTTP(response, handoverRequest(endpoint.line, endpoint.line.body))
		if response.Code >= 400 {
			t.Fatalf("%s：%d %s", endpoint.line.path, response.Code, response.Body.String())
		}
	}
	wantAsked := []string{
		"DELIVERY_HANDOVER_NOTE_OPENING|SYN-DEVICE-NODE|NODE|" + presentedScopeReference,
		"DELIVERY_HANDOVER_NOTE_ASSIGNMENT|SYN-DEVICE-NODE|NODE|" + presentedScopeReference,
		"DELIVERY_HANDOVER_NOTE_CLOSURE|SYN-DEVICE-NODE|NODE|" + presentedScopeReference,
	}
	if strings.Join(asked, ";") != strings.Join(wantAsked, ";") {
		t.Fatalf("认证被问 %v, want %v", asked, wantAsked)
	}
	opened, assigned, closed := handler.opened[0], handler.assigned[0], handler.closed[0]
	switch {
	case opened.TenantID.String() != workFactTenant || opened.Node.String() != handoverHTTPNode || opened.Note.String() != "SYN-HANDOVER-HTTP-01" ||
		opened.DeliveringParty.String() != "SYN-DRIVER-HTTP" || len(opened.ClaimedAccounts) != 2 || opened.ClaimedAccounts[0].String() != "SYN-ACCOUNT-02" ||
		!opened.OpenedAt.Equal(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)):
		t.Fatalf("开立命令 = %+v", opened)
	case assigned.Node.String() != handoverHTTPNode || assigned.FactID != "SYN-ASSIGN-HTTP-01" || assigned.Note.String() != "SYN-HANDOVER-HTTP-01" ||
		assigned.Unit.String() != "SYN-UNIT-HTTP-01" || !assigned.AssignedAt.Equal(time.Date(2026, 10, 19, 0, 30, 0, 0, time.UTC)):
		t.Fatalf("归入命令 = %+v", assigned)
	case closed.Node.String() != handoverHTTPNode || closed.FactID != "SYN-CLOSE-HTTP-01" || !closed.ClosedAt.Equal(time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)):
		t.Fatalf("关闭命令 = %+v", closed)
	}
}

// Covers: 票 12 做什么第 2 项与 ADR-0151 决定二——运营决定确认口按「送站交接确认」这一种决定认证，租户与提交操作者只取认证结果，操作
// 者写作 `OPERATOR/<发行方>#<sub>`，依据引用去两端空白；客户面结论口的客户账户只取信封，来源请求键取推导口铸的那一个，结论按线上名
// 认。
func TestTheConfirmationAndConclusionIntakesTakeIdentityFromTheEnvelope(t *testing.T) {
	var asked []nodeopshttp.OperatorDecision
	handler := &handoverHandlerDouble{result: application.NewDeliveryHandoverResult(application.DeliveryHandoverBindingConfirmed, application.DeliveryHandoverAnswer{})}
	confirm := handoverLines["confirm"]
	response := httptest.NewRecorder()
	nodeopshttp.NewConfirmDeliveryHandoverEndpoint(handoverDecisionIntake(t, nil, &asked), handler).ServeHTTP(response, handoverRequest(confirm, confirm.body))
	if response.Code >= 400 || len(asked) != 1 || asked[0] != nodeopshttp.OperatorDecisionDeliveryHandoverConfirmation {
		t.Fatalf("确认口：%d，认证被问 %v", response.Code, asked)
	}
	confirmed := handler.confirmed[0]
	if confirmed.TenantID.String() != dispositionTenant || confirmed.Operator != "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-NRI06" ||
		confirmed.Note.String() != "SYN-HANDOVER-HTTP-01" || confirmed.Account.String() != "SYN-ACCOUNT-01" ||
		confirmed.DeliveringParticipant.String() != "SYN-PARTY-DELIVERER-01" || confirmed.Basis != "SYN-BASIS-LETTER-01" {
		t.Fatalf("确认命令 = %+v", confirmed)
	}

	conclude := handoverLines["conclude"]
	response = httptest.NewRecorder()
	nodeopshttp.NewConcludeDeliveryHandoverEndpoint(handoverCustomerIntake(t, nil), handler).ServeHTTP(response, handoverRequest(conclude, conclude.body))
	if response.Code >= 400 {
		t.Fatalf("结论口：%d %s", response.Code, response.Body.String())
	}
	concluded := handler.concluded[0]
	if concluded.TenantID.String() != preAdviceTenant || concluded.CustomerAccount.String() != preAdviceAccount || concluded.Channel != preAdviceChannel ||
		concluded.RequestKey != "SYN-ACCESS-REQUEST-HN-0001" || concluded.Note.String() != "SYN-HANDOVER-HTTP-01" || concluded.Conclusion != domain.BindingDenied {
		t.Fatalf("结论命令 = %+v", concluded)
	}
}

// Covers: 票 12 做什么第 6 项的坏载荷格——各口载荷封闭：自报租户、节点、客户账户（客户面）或操作者即未知字段；缺格、空白、带 NUL、
// 时刻解不出、声称为空或重复、结论不在词表、作业范围种类不是 NODE，都答 400 MALFORMED_REQUEST，不进编排。
func TestHandoverLinesRefuseMalformedPayloads(t *testing.T) {
	for name, testCase := range map[string]struct {
		line string
		body string
	}{
		"开立自报租户":    {"open", `{"tenantId":"T","noteId":"N","deliveringParty":"D","claimedAccounts":["A"],"openedAt":"2026-10-19T08:00:00Z"}`},
		"开立自报节点":    {"open", `{"node":"X","noteId":"N","deliveringParty":"D","claimedAccounts":["A"],"openedAt":"2026-10-19T08:00:00Z"}`},
		"开立声称为空":    {"open", `{"noteId":"N","deliveringParty":"D","claimedAccounts":[],"openedAt":"2026-10-19T08:00:00Z"}`},
		"开立声称重复":    {"open", `{"noteId":"N","deliveringParty":"D","claimedAccounts":["A","A"],"openedAt":"2026-10-19T08:00:00Z"}`},
		"开立缺交付方":    {"open", `{"noteId":"N","claimedAccounts":["A"],"openedAt":"2026-10-19T08:00:00Z"}`},
		"开立缺开立时刻":   {"open", `{"noteId":"N","deliveringParty":"D","claimedAccounts":["A"]}`},
		"开立标识带NUL":  {"open", `{"noteId":"N\u0000","deliveringParty":"D","claimedAccounts":["A"],"openedAt":"2026-10-19T08:00:00Z"}`},
		"归入缺事实身份":   {"assign", `{"noteId":"N","unit":"U","assignedAt":"2026-10-19T08:00:00Z"}`},
		"归入时刻解不出":   {"assign", `{"factId":"F","noteId":"N","unit":"U","assignedAt":"yesterday"}`},
		"归入缺实物":     {"assign", `{"factId":"F","noteId":"N","assignedAt":"2026-10-19T08:00:00Z"}`},
		"关闭空白标识":    {"close", `{"factId":"F","noteId":"  ","closedAt":"2026-10-19T08:00:00Z"}`},
		"关闭尾随内容":    {"close", `{"factId":"F","noteId":"N","closedAt":"2026-10-19T08:00:00Z"}{}`},
		"确认自报操作者":   {"confirm", `{"operator":"X","noteId":"N","customerAccount":"A","deliveringParticipant":"P","basis":"B"}`},
		"确认缺依据":     {"confirm", `{"noteId":"N","customerAccount":"A","deliveringParticipant":"P"}`},
		"确认缺交付方参与方": {"confirm", `{"noteId":"N","customerAccount":"A","basis":"B"}`},
		"结论自报客户账户":  {"conclude", `{"accessRequestId":"R","noteId":"N","conclusion":"CONFIRMED","customerAccount":"SYN-ACCOUNT-09"}`},
		"结论不在词表":    {"conclude", `{"accessRequestId":"R","noteId":"N","conclusion":"confirmed"}`},
		"结论缺接入请求标识": {"conclude", `{"noteId":"N","conclusion":"CONFIRMED"}`},
		"结论缺交接单":    {"conclude", `{"accessRequestId":"R","conclusion":"CONFIRMED"}`},
	} {
		t.Run(name, func(t *testing.T) {
			line := handoverLines[testCase.line]
			handler := &handoverHandlerDouble{}
			response := httptest.NewRecorder()
			line.endpoint(t, handler, nil).ServeHTTP(response, handoverRequest(line, testCase.body))
			if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
				t.Fatalf("answer = %d %s", response.Code, response.Body.String())
			}
			if handler.calls() != 0 {
				t.Fatal("坏载荷进了编排")
			}
		})
	}
	open := handoverLines["open"]
	request := handoverRequest(open, open.body)
	request.Header.Set("Parcel-Work-Scope-Kind", "OFFSITE")
	response := httptest.NewRecorder()
	open.endpoint(t, &handoverHandlerDouble{}, nil).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("场外作业范围开交接单：%d", response.Code)
	}
}

// Covers: 票 12 完成判据「信封各格」的传输面——三族各自的未配置、令牌无效、未授予、不在准入范围、依赖读不动逐格映射，各对一种恢复
// 动作；作业事实一族另有设备不在用一格；客户面两格用首方族自己的错误码；认证没过不进编排、不带 outcome。
func TestHandoverLinesAnswerTheEnvelopeGrades(t *testing.T) {
	type grade struct {
		err    error
		status int
		code   string
	}
	common := map[string]grade{
		"未配置":    {nodeopshttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"不在准入范围": {nodeopshttp.ErrOutsideAdmissionScope, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		"依赖读不动":  {nodeopshttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	operator := map[string]grade{
		"令牌无效": {nodeopshttp.ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"未授予":  {nodeopshttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
	}
	customer := map[string]grade{
		"令牌无效": {nodeopshttp.ErrCustomerClientCredentialRejected, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"未授予":  {nodeopshttp.ErrCustomerClientNotGranted, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
	}
	for lineName, line := range handoverLines {
		grades := map[string]grade{}
		for name, value := range common {
			grades[name] = value
		}
		family := operator
		if line.customer {
			family = customer
		}
		for name, value := range family {
			grades[name] = value
		}
		if lineName == "open" || lineName == "assign" || lineName == "close" {
			grades["设备不在用"] = grade{nodeopshttp.ErrDeviceNotInUse, http.StatusForbidden, "DEVICE_NOT_IN_USE"}
		}
		for gradeName, want := range grades {
			t.Run(lineName+"/"+gradeName, func(t *testing.T) {
				handler := &handoverHandlerDouble{}
				response := httptest.NewRecorder()
				line.endpoint(t, handler, want.err).ServeHTTP(response, handoverRequest(line, line.body))
				if response.Code != want.status || problemCode(t, response) != want.code {
					t.Fatalf("answer = %d %s, want %d %s", response.Code, response.Body.String(), want.status, want.code)
				}
				assertNoOutcome(t, response)
				if handler.calls() != 0 {
					t.Fatal("认证没过进了编排")
				}
			})
		}
	}
}

// Covers: ADR-0055 第一条——Intake 未配置时五口答 403 ACCESS_CHANNEL_NOT_CONFIGURED、不读请求；方法不对仍由处理器答 405。
func TestHandoverLinesAnswerUnconfiguredAndKeepTheMethodGate(t *testing.T) {
	unconfigured := nodeopshttp.UnconfiguredIntake{}
	handler := &handoverHandlerDouble{}
	for path, endpoint := range map[string]http.Handler{
		"/node-operations/delivery-handover-notes":         nodeopshttp.NewOpenDeliveryHandoverNoteEndpoint(unconfigured, handler),
		"/node-operations/delivery-handover-assignments":   nodeopshttp.NewAssignToDeliveryHandoverNoteEndpoint(unconfigured, handler),
		"/node-operations/delivery-handover-closures":      nodeopshttp.NewCloseDeliveryHandoverNoteEndpoint(unconfigured, handler),
		"/node-operations/delivery-handover-confirmations": nodeopshttp.NewConfirmDeliveryHandoverEndpoint(unconfigured, handler),
		"/node-operations/delivery-handover-conclusions":   nodeopshttp.NewConcludeDeliveryHandoverEndpoint(unconfigured, handler),
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s 未配置：%d %s", path, response.Code, response.Body.String())
		}
		response = httptest.NewRecorder()
		endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("%s 方法门：%d", path, response.Code)
		}
	}
	if handler.calls() != 0 {
		t.Fatal("未配置进了编排")
	}
}

// Covers: `AT-NO-073`（HTTP 面）、票 12 做什么第 6 项的业务格与 ADR-0022——首次形成（开立、归入、关闭、确认、否认）答 201，已有结果、已有结论、来源冲突、
// 交接单状态不符、不在授权交付关系内、未配置都是形成了的答案、答 200 带 outcome；看不见的交接单答 404，拒形答 400，编排读写不动答
// 500。操作者面回显交接单全貌；客户面只回显交接单标识与自己那一个账户的结论，不回显声称了谁、范围成立没有。
func TestHandoverLinesMapTheirOutcomes(t *testing.T) {
	note, closure := handoverHTTPNote(t, true)
	binding, err := domain.FormDeliveryHandoverBinding(domain.DeliveryHandoverBindingSpec{
		TenantID: note.TenantID(), Note: note.ID(), Account: adapterValue(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Conclusion: domain.BindingConfirmed, Route: domain.ConcludedViaOperationDecision, Concluder: "OPERATOR/SYN",
		DeliveringParticipant: "SYN-PARTY-DELIVERER-01", Basis: "SYN-BASIS-LETTER-01", ConcludedAt: time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	answer := application.DeliveryHandoverAnswer{Note: note, Closure: closure, Binding: binding, ScopeEstablishedAt: time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)}
	for lineName, line := range handoverLines {
		created := map[string]application.DeliveryHandoverOutcome{
			"open": application.DeliveryHandoverNoteOpened, "assign": application.DeliveryHandoverUnitAssigned,
			"close": application.DeliveryHandoverNoteClosed, "confirm": application.DeliveryHandoverBindingConfirmed,
			"conclude": application.DeliveryHandoverBindingDenied,
		}[lineName]
		for _, testCase := range []struct {
			outcome application.DeliveryHandoverOutcome
			err     error
			status  int
			code    string
		}{
			{outcome: created, status: http.StatusCreated},
			{outcome: application.DeliveryHandoverExistingResult, status: http.StatusOK},
			{outcome: application.DeliveryHandoverExistingConclusion, status: http.StatusOK},
			{outcome: application.DeliveryHandoverSourceConflict, status: http.StatusOK},
			{outcome: application.DeliveryHandoverNoteStateMismatch, status: http.StatusOK},
			{outcome: application.DeliveryHandoverNotAuthorized, status: http.StatusOK},
			{outcome: application.DeliveryHandoverAuthorizationNotConfigured, status: http.StatusOK},
			{err: application.ErrDeliveryHandoverNoteNotFound, status: http.StatusNotFound, code: "DELIVERY_HANDOVER_NOTE_NOT_VISIBLE"},
			{err: application.ErrDeliveryHandoverCommandRejected, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
			{err: errors.New("register unreachable"), status: http.StatusInternalServerError, code: "NO_ANSWER_FORMED"},
		} {
			handler := &handoverHandlerDouble{result: application.NewDeliveryHandoverResult(testCase.outcome, answer), err: testCase.err}
			response := httptest.NewRecorder()
			line.endpoint(t, handler, nil).ServeHTTP(response, handoverRequest(line, line.body))
			if response.Code != testCase.status {
				t.Fatalf("%s %s %v：status = %d, want %d；%s", lineName, testCase.outcome, testCase.err, response.Code, testCase.status, response.Body.String())
			}
			if testCase.err != nil {
				if problemCode(t, response) != testCase.code {
					t.Fatalf("%s %v：code = %q", lineName, testCase.err, problemCode(t, response))
				}
				assertNoOutcome(t, response)
				continue
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if string(body["outcome"]) != `"`+testCase.outcome.String()+`"` {
				t.Fatalf("%s：outcome = %s, want %s", lineName, body["outcome"], testCase.outcome)
			}
			_, echoesNote := body["handoverNote"]
			_, echoesScope := body["scopeEstablishedAt"]
			if line.customer {
				if echoesNote || echoesScope || string(body["noteId"]) != `"SYN-HANDOVER-HTTP-01"` || strings.Contains(response.Body.String(), "SYN-ACCOUNT-02") {
					t.Fatalf("客户面回显了不该给客户看的格：%s", response.Body.String())
				}
				continue
			}
			if !echoesNote || !echoesScope || !strings.Contains(string(body["handoverNote"]), "SYN-ACCOUNT-02") || !strings.Contains(string(body["handoverNote"]), "closedAt") {
				t.Fatalf("操作者面没回显交接单全貌：%s", response.Body.String())
			}
		}
	}

	unnamed := &handoverHandlerDouble{result: application.NewDeliveryHandoverResult(application.DeliveryHandoverOutcomeInvalid, answer)}
	line := handoverLines["open"]
	response := httptest.NewRecorder()
	line.endpoint(t, unnamed, nil).ServeHTTP(response, handoverRequest(line, line.body))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("答不出名字的 outcome：%d", response.Code)
	}
}
