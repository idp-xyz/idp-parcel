package nodeopshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件钉送站预报与送站交接单运营查阅读的传输面（票 ps-external-mark-relations/13「做什么」的「查阅读口」；UC-NO-006「权限、隔离与审计」）：
// 方法门、信封各格、筛选参数的封闭形状、列表与详情的转写，与「查无与跨租户同答」在传输层的那一半（读口答 found=false，端点只有一种答法）。

var scopeReadAt = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

// stubDeliveryScopeCatalogue 是两本册查阅读口的替身：记下读口收到的租户、筛选、页大小与标识；calls 计读口被触到的次数。
type stubDeliveryScopeCatalogue struct {
	preAdvices []domain.DeliveryPreAdviceState
	notes      []ports.DeliveryHandoverNoteCatalogueRow
	preAdvice  map[string]domain.DeliveryPreAdviceState
	note       map[string]ports.DeliveryHandoverNoteDetail
	err        error

	calls     int
	gotTenant string
	gotFilter ports.DeliveryScopeCatalogueFilter
	gotLimit  int
	gotID     string
}

func (stub *stubDeliveryScopeCatalogue) ListDeliveryPreAdvices(
	_ context.Context, tenant domain.TenantID, filter ports.DeliveryScopeCatalogueFilter, limit int,
) ([]domain.DeliveryPreAdviceState, error) {
	stub.calls++
	stub.gotTenant, stub.gotFilter, stub.gotLimit = tenant.String(), filter, limit
	return stub.preAdvices, stub.err
}

func (stub *stubDeliveryScopeCatalogue) FindDeliveryPreAdvice(
	_ context.Context, tenant domain.TenantID, id domain.DeliveryPreAdviceID,
) (domain.DeliveryPreAdviceState, bool, error) {
	stub.calls++
	stub.gotTenant, stub.gotID = tenant.String(), id.String()
	state, found := stub.preAdvice[id.String()]
	return state, found, stub.err
}

func (stub *stubDeliveryScopeCatalogue) ListDeliveryHandoverNotes(
	_ context.Context, tenant domain.TenantID, filter ports.DeliveryScopeCatalogueFilter, limit int,
) ([]ports.DeliveryHandoverNoteCatalogueRow, error) {
	stub.calls++
	stub.gotTenant, stub.gotFilter, stub.gotLimit = tenant.String(), filter, limit
	return stub.notes, stub.err
}

func (stub *stubDeliveryScopeCatalogue) FindDeliveryHandoverNoteDetail(
	_ context.Context, tenant domain.TenantID, id domain.DeliveryHandoverNoteID,
) (ports.DeliveryHandoverNoteDetail, bool, error) {
	stub.calls++
	stub.gotTenant, stub.gotID = tenant.String(), id.String()
	detail, found := stub.note[id.String()]
	return detail, found, stub.err
}

// scopeRead 是四口之一：怎样构造、目标路径、详情口的路径参数名与值。
type scopeRead struct {
	name      string
	construct func(nodeopshttp.CatalogueQueryIntake, nodeopshttp.DeliveryScopeCatalogueReader) http.Handler
	target    string
	pathKey   string
	pathValue string
}

var scopeReads = []scopeRead{
	{name: "pre-advice list", construct: nodeopshttp.NewQueryDeliveryPreAdvicesEndpoint, target: "/node-operations-delivery-pre-advices"},
	{name: "pre-advice detail", construct: nodeopshttp.NewQueryDeliveryPreAdviceEndpoint, target: "/node-operations-delivery-pre-advices/SYN-PREADV-1", pathKey: "preAdviceId", pathValue: "SYN-PREADV-1"},
	{name: "handover note list", construct: nodeopshttp.NewQueryDeliveryHandoverNotesEndpoint, target: "/node-operations-delivery-handover-notes"},
	{name: "handover note detail", construct: nodeopshttp.NewQueryDeliveryHandoverNoteEndpoint, target: "/node-operations-delivery-handover-notes/SYN-HN-1", pathKey: "noteId", pathValue: "SYN-HN-1"},
}

func (read scopeRead) serve(intake nodeopshttp.CatalogueQueryIntake, reader nodeopshttp.DeliveryScopeCatalogueReader, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	if read.pathKey != "" {
		request.SetPathValue(read.pathKey, read.pathValue)
	}
	response := httptest.NewRecorder()
	read.construct(intake, reader).ServeHTTP(response, request)
	return response
}

// Covers: ADR-0022——方法不对不是业务答案：四口都答 405 + Allow GET，读口不被触到。
func TestDeliveryScopeReadsRefuseNonGetMethods(t *testing.T) {
	for _, read := range scopeReads {
		t.Run(read.name, func(t *testing.T) {
			reader := &stubDeliveryScopeCatalogue{}
			response := read.serve(grantedIntake(), reader, http.MethodPost, read.target)
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("status = %d Allow = %q", response.Code, response.Header().Get("Allow"))
			}
			if reader.calls != 0 {
				t.Fatal("方法不对时读口被触到")
			}
		})
	}
}

// Covers: 票 13 完成判据「未授予运营查阅读答其格」的传输面（ADR-0055、ADR-0100 决定四）——四口的信封各格逐格映射：未配置 403、令牌
// 不过 401、没有查阅读授予 403、身份依赖读不动 503；都先于读请求里的任何内容（坏筛选也答信封那一格），读口不被触到、不带 outcome。
func TestDeliveryScopeReadsAnswerTheEnvelopeGradesFirst(t *testing.T) {
	grades := []struct {
		name   string
		intake nodeopshttp.CatalogueQueryIntake
		status int
		code   string
	}{
		{"unconfigured", nodeopshttp.UnconfiguredIntake{}, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"credential rejected", failingCatalogueIntake{err: nodeopshttp.ErrOperatorCredentialRejected}, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"not granted", failingCatalogueIntake{err: nodeopshttp.ErrOperatorNotGranted}, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"identity dependency down", failingCatalogueIntake{err: nodeopshttp.ErrIdentityDependencyUnavailable}, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	for _, read := range scopeReads {
		for _, grade := range grades {
			t.Run(read.name+"/"+grade.name, func(t *testing.T) {
				reader := &stubDeliveryScopeCatalogue{}
				response := read.serve(grade.intake, reader, http.MethodGet, read.target+"?node=&tenant=SYN-TENANT-9")
				if response.Code != grade.status || problemCode(t, response) != grade.code {
					t.Fatalf("status = %d body = %s, want %d %s", response.Code, response.Body.String(), grade.status, grade.code)
				}
				assertNoOutcome(t, response)
				if reader.calls != 0 {
					t.Fatal("信封没过时读口被触到")
				}
			})
		}
	}
}

// Covers: 筛选参数的读法——节点逐字、两个时刻按 RFC 3339 读成同一瞬间；租户与页大小只取自信封；不给参数即不筛。
func TestDeliveryScopeListsTakeTheirFiltersFromTheQuery(t *testing.T) {
	from := time.Date(2026, 10, 19, 16, 0, 0, 0, time.UTC)
	until := from.Add(24 * time.Hour)
	cases := []struct {
		read  scopeRead
		query string
	}{
		{scopeReads[0], "?node=SYN-NODE-1&expectedArrivalFrom=2026-10-20T00:00:00%2B08:00&expectedArrivalUntil=2026-10-21T00:00:00%2B08:00"},
		{scopeReads[2], "?node=SYN-NODE-1&openedFrom=2026-10-20T00:00:00%2B08:00&openedUntil=2026-10-21T00:00:00%2B08:00"},
	}
	for _, testCase := range cases {
		t.Run(testCase.read.name, func(t *testing.T) {
			reader := &stubDeliveryScopeCatalogue{}
			response := testCase.read.serve(grantedIntake(), reader, http.MethodGet, testCase.read.target+testCase.query)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if reader.gotTenant != "TENANT-1" || reader.gotLimit != 25 || reader.gotFilter.Node.String() != "SYN-NODE-1" ||
				!reader.gotFilter.From.Equal(from) || !reader.gotFilter.Until.Equal(until) {
				t.Fatalf("读口收到 tenant=%s limit=%d filter=%+v", reader.gotTenant, reader.gotLimit, reader.gotFilter)
			}

			bare := &stubDeliveryScopeCatalogue{}
			if response := testCase.read.serve(grantedIntake(), bare, http.MethodGet, testCase.read.target); response.Code != http.StatusOK {
				t.Fatalf("不带参数 status = %d", response.Code)
			}
			if bare.gotFilter != (ports.DeliveryScopeCatalogueFilter{}) {
				t.Fatalf("不带参数的筛选 = %+v, want 零值", bare.gotFilter)
			}
		})
	}
}

// Covers: 筛选参数封闭——空节点、认不出的时刻、起点不早于终点、未知键（含自报租户）与重复键都答 400 MALFORMED_REQUEST，读口不被触到：
// 静默丢掉一个拼错的键，人会把全租户的列表当成按条件筛过的。
func TestDeliveryScopeListsRefuseMalformedFilters(t *testing.T) {
	cases := []struct {
		read    scopeRead
		queries []string
	}{
		{scopeReads[0], []string{
			"?node=", "?node=%20", "?node=a&node=b", "?expectedArrivalFrom=yesterday", "?expectedArrivalUntil=2026-10-21",
			"?expectedArrivalFrom=2026-10-21T00:00:00Z&expectedArrivalUntil=2026-10-21T00:00:00Z",
			"?expectedArrivalFrom=2026-10-22T00:00:00Z&expectedArrivalUntil=2026-10-21T00:00:00Z",
			"?tenant=SYN-TENANT-9", "?openedFrom=2026-10-21T00:00:00Z",
		}},
		{scopeReads[2], []string{
			"?node=", "?openedFrom=bad", "?openedFrom=2026-10-22T00:00:00Z&openedUntil=2026-10-21T00:00:00Z",
			"?limit=9999", "?expectedArrivalFrom=2026-10-21T00:00:00Z",
		}},
	}
	for _, testCase := range cases {
		for _, query := range testCase.queries {
			t.Run(testCase.read.name+query, func(t *testing.T) {
				reader := &stubDeliveryScopeCatalogue{}
				response := testCase.read.serve(grantedIntake(), reader, http.MethodGet, testCase.read.target+query)
				if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
					t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
				}
				assertNoOutcome(t, response)
				if reader.calls != 0 {
					t.Fatal("坏筛选时读口被触到")
				}
			})
		}
	}
}

// Covers: 详情口的路径参数——空白标识是请求构造不出查询（400），不是查无此单。
func TestDeliveryScopeDetailsRefuseABlankIdentifier(t *testing.T) {
	for _, read := range []scopeRead{scopeReads[1], scopeReads[3]} {
		t.Run(read.name, func(t *testing.T) {
			read.pathValue = " "
			reader := &stubDeliveryScopeCatalogue{}
			response := read.serve(grantedIntake(), reader, http.MethodGet, read.target)
			if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" || reader.calls != 0 {
				t.Fatalf("status = %d body = %s calls = %d", response.Code, response.Body.String(), reader.calls)
			}
		})
	}
}

// Covers: ADR-0022——读不动是没形成答案：四口都答 500 NO_ANSWER_FORMED、不带 outcome，不伪装成空册或查无此单。
func TestDeliveryScopeReadsAnswer500WhenTheRegisterFails(t *testing.T) {
	for _, read := range scopeReads {
		t.Run(read.name, func(t *testing.T) {
			reader := &stubDeliveryScopeCatalogue{err: errors.New("register unreachable")}
			response := read.serve(grantedIntake(), reader, http.MethodGet, read.target)
			if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			assertNoOutcome(t, response)
		})
	}
}

func scopeMust[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func preAdviceStateForRead(t *testing.T, id, clue string, withdrawnAt time.Time, numbers ...string) domain.DeliveryPreAdviceState {
	t.Helper()
	listed := make([]domain.PreAdvisedParcelNumber, 0, len(numbers))
	for _, number := range numbers {
		listed = append(listed, scopeMust(t, domain.NewPreAdvisedParcelNumber, number))
	}
	source := domain.CustomerRequestSource{Channel: "https://id.syn.example/dex#syn-client", RequestKey: "SYN-REQUEST-" + id, ContentDigest: "sha256:" + id}
	spec := domain.DeliveryPreAdviceSpec{
		TenantID:          scopeMust(t, domain.NewTenantID, "TENANT-1"),
		ID:                scopeMust(t, domain.NewDeliveryPreAdviceID, id),
		CustomerAccount:   scopeMust(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-1"),
		Node:              scopeMust(t, domain.NewNodeReference, "SYN-NODE-1"),
		ExpectedArrivalAt: scopeReadAt.Add(2 * time.Hour),
		Numbers:           listed,
		Source:            source,
		FormedAt:          scopeReadAt,
	}
	if clue != "" {
		spec.DeliveringParty = scopeMust(t, domain.NewDeliveringPartyReference, clue)
	}
	preAdvice, err := domain.FormDeliveryPreAdvice(spec)
	if err != nil {
		t.Fatal(err)
	}
	state := domain.DeliveryPreAdviceState{PreAdvice: preAdvice}
	if !withdrawnAt.IsZero() {
		if state.Withdrawal, err = domain.FormDeliveryPreAdviceWithdrawal(domain.DeliveryPreAdviceWithdrawalSpec{
			TenantID: preAdvice.TenantID(), PreAdvice: preAdvice.ID(), CustomerAccount: preAdvice.CustomerAccount(),
			Source: domain.CustomerRequestSource{Channel: source.Channel, RequestKey: "SYN-WITHDRAW-" + id, ContentDigest: "sha256:w"}, WithdrawnAt: withdrawnAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func decodeObject(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return fields
}

// Covers: 票 13「做什么」的「查阅读口」（预报）的转写——列表与详情同一个形：标识、客户账户、节点、预计送达、号码集合（照交来的顺序）、交付方线索
// （没给即整键缺席）、形成时刻与撤回与否（撤回才带撤回时刻）；空册答 []；详情查无答 404 DELIVERY_PRE_ADVICE_NOT_VISIBLE，与跨租户同答。
func TestPreAdviceReadsTranscribeTheRegister(t *testing.T) {
	withdrawn := preAdviceStateForRead(t, "SYN-PREADV-2", "", scopeReadAt.Add(time.Hour), "SYN-NUMBER-3")
	current := preAdviceStateForRead(t, "SYN-PREADV-1", "SYN-COURIER-1", time.Time{}, "SYN-NUMBER-2", "SYN-NUMBER-1")
	reader := &stubDeliveryScopeCatalogue{
		preAdvices: []domain.DeliveryPreAdviceState{withdrawn, current},
		preAdvice:  map[string]domain.DeliveryPreAdviceState{"SYN-PREADV-1": current},
	}
	list := scopeReads[0].serve(grantedIntake(), reader, http.MethodGet, scopeReads[0].target)
	if list.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", list.Code, list.Body.String())
	}
	var listBody struct {
		Outcome    string                       `json:"outcome"`
		PreAdvices []map[string]json.RawMessage `json:"preAdvices"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil || listBody.Outcome != "DELIVERY_PRE_ADVICES_LISTED" || len(listBody.PreAdvices) != 2 {
		t.Fatalf("列表走样：%s", list.Body.String())
	}
	first, second := listBody.PreAdvices[0], listBody.PreAdvices[1]
	if string(first["preAdviceId"]) != `"SYN-PREADV-2"` || string(first["withdrawn"]) != `true` ||
		string(first["withdrawnAt"]) != `"2026-10-20T10:00:00Z"` {
		t.Errorf("撤回那一行走样：%s", list.Body.String())
	}
	if _, present := first["deliveringParty"]; present {
		t.Errorf("没给交付方线索的行带了 deliveringParty：%s", list.Body.String())
	}
	if string(second["withdrawn"]) != `false` || string(second["customerParcelNumbers"]) != `["SYN-NUMBER-2","SYN-NUMBER-1"]` ||
		string(second["deliveringParty"]) != `"SYN-COURIER-1"` || string(second["customerAccount"]) != `"SYN-ACCOUNT-1"` ||
		string(second["node"]) != `"SYN-NODE-1"` || string(second["expectedArrivalAt"]) != `"2026-10-20T11:00:00Z"` ||
		string(second["formedAt"]) != `"2026-10-20T09:00:00Z"` {
		t.Errorf("未撤回那一行走样：%s", list.Body.String())
	}
	if _, present := second["withdrawnAt"]; present {
		t.Errorf("未撤回的行带了 withdrawnAt：%s", list.Body.String())
	}

	empty := scopeReads[0].serve(grantedIntake(), &stubDeliveryScopeCatalogue{}, http.MethodGet, scopeReads[0].target)
	if got := arrayAt(t, empty, "preAdvices"); got != "[]" {
		t.Errorf("空册 preAdvices = %s, want []", got)
	}

	detailRead := scopeReads[1]
	detailRead.pathValue = "SYN-PREADV-1"
	detail := detailRead.serve(grantedIntake(), reader, http.MethodGet, "/node-operations-delivery-pre-advices/SYN-PREADV-1")
	if detail.Code != http.StatusOK {
		t.Fatalf("详情 status = %d body = %s", detail.Code, detail.Body.String())
	}
	fields := decodeObject(t, detail.Body.Bytes())
	if string(fields["outcome"]) != `"DELIVERY_PRE_ADVICE_FOUND"` || reader.gotID != "SYN-PREADV-1" || reader.gotTenant != "TENANT-1" {
		t.Fatalf("详情走样：%s（读口收到 %s/%s）", detail.Body.String(), reader.gotTenant, reader.gotID)
	}
	if item := decodeObject(t, fields["preAdvice"]); string(item["customerParcelNumbers"]) != `["SYN-NUMBER-2","SYN-NUMBER-1"]` || string(item["withdrawn"]) != `false` {
		t.Errorf("详情那一份走样：%s", detail.Body.String())
	}

	detailRead.pathValue = "SYN-PREADV-ELSEWHERE"
	missing := detailRead.serve(grantedIntake(), reader, http.MethodGet, "/node-operations-delivery-pre-advices/SYN-PREADV-ELSEWHERE")
	if missing.Code != http.StatusNotFound || problemCode(t, missing) != "DELIVERY_PRE_ADVICE_NOT_VISIBLE" {
		t.Fatalf("查无 status = %d body = %s", missing.Code, missing.Body.String())
	}
	assertNoOutcome(t, missing)
}

func handoverStateForRead(t *testing.T, id string, closedAt time.Time, accounts []string, bindings ...domain.DeliveryHandoverBinding) domain.DeliveryHandoverNoteState {
	t.Helper()
	claimed := make([]domain.CustomerAccountReference, 0, len(accounts))
	for _, account := range accounts {
		claimed = append(claimed, scopeMust(t, domain.NewCustomerAccountReference, account))
	}
	note, err := domain.OpenDeliveryHandoverNote(domain.DeliveryHandoverNoteSpec{
		TenantID:        scopeMust(t, domain.NewTenantID, "TENANT-1"),
		ID:              scopeMust(t, domain.NewDeliveryHandoverNoteID, id),
		Node:            scopeMust(t, domain.NewNodeReference, "SYN-NODE-1"),
		DeliveringParty: scopeMust(t, domain.NewDeliveringPartyReference, "SYN-DRIVER-1"),
		ClaimedAccounts: claimed,
		OpenedAt:        scopeReadAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	state := domain.DeliveryHandoverNoteState{Note: note, Bindings: bindings}
	if !closedAt.IsZero() {
		if state.Closure, err = domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{
			TenantID: note.TenantID(), Note: note.ID(), FactID: "SYN-CLOSE-" + id, ClosedAt: closedAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func bindingForRead(t *testing.T, note, account string, conclusion domain.HandoverBindingConclusion, route domain.HandoverBindingRoute, at time.Time) domain.DeliveryHandoverBinding {
	t.Helper()
	spec := domain.DeliveryHandoverBindingSpec{
		TenantID: scopeMust(t, domain.NewTenantID, "TENANT-1"), Note: scopeMust(t, domain.NewDeliveryHandoverNoteID, note),
		Account: scopeMust(t, domain.NewCustomerAccountReference, account), Conclusion: conclusion, Route: route, ConcludedAt: at,
	}
	if route == domain.ConcludedViaOperationDecision {
		spec.Concluder, spec.DeliveringParticipant, spec.Basis = "OPERATOR/https://id.syn.example/dex#syn-operator", "SYN-PARTY-DELIVERER-1", "SYN-BASIS-1"
	} else {
		spec.Concluder, spec.SourceRequestKey = "https://id.syn.example/dex#syn-client", "SYN-ACCESS-REQUEST-1"
	}
	binding, err := domain.FormDeliveryHandoverBinding(spec)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

// Covers: 票 13「做什么」的「查阅读口」（交接单）的转写——列表每行带交付方线索、开立、关闭（事实身份与时刻）、成员数、声称的每个账户各一格
// （结论与途径、作结论方、运营决定另带交付方参与方与依据引用，没结论的只带账户）、范围成立与否（成立才带时刻）；详情另带成员与各自的
// 形成方式；查无答 404 DELIVERY_HANDOVER_NOTE_NOT_VISIBLE，与跨租户同答。
func TestHandoverNoteReadsCarryClaimsClosureMembersAndScope(t *testing.T) {
	established := handoverStateForRead(t, "SYN-HN-1", scopeReadAt.Add(time.Hour), []string{"SYN-ACCOUNT-1", "SYN-ACCOUNT-2"},
		bindingForRead(t, "SYN-HN-1", "SYN-ACCOUNT-1", domain.BindingConfirmed, domain.ConcludedViaOperationDecision, scopeReadAt.Add(2*time.Hour)),
		bindingForRead(t, "SYN-HN-1", "SYN-ACCOUNT-2", domain.BindingConfirmed, domain.ConcludedViaCustomerChannel, scopeReadAt.Add(3*time.Hour)))
	pending := handoverStateForRead(t, "SYN-HN-2", time.Time{}, []string{"SYN-ACCOUNT-1", "SYN-ACCOUNT-3"},
		bindingForRead(t, "SYN-HN-2", "SYN-ACCOUNT-3", domain.BindingDenied, domain.ConcludedViaCustomerChannel, scopeReadAt.Add(time.Hour)))
	member := func(unit string, kind domain.HandoverMembershipKind, fact string) domain.DeliveryHandoverMember {
		formed, err := domain.FormDeliveryHandoverMember(domain.DeliveryHandoverMemberSpec{
			TenantID: scopeMust(t, domain.NewTenantID, "TENANT-1"), Note: established.Note.ID(), Unit: scopeMust(t, domain.NewHandlingUnitID, unit),
			Kind: kind, FactID: fact, FormedAt: scopeReadAt.Add(10 * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		return formed
	}
	reader := &stubDeliveryScopeCatalogue{
		notes: []ports.DeliveryHandoverNoteCatalogueRow{{State: pending, MemberCount: 0}, {State: established, MemberCount: 2}},
		note: map[string]ports.DeliveryHandoverNoteDetail{"SYN-HN-1": {State: established, Members: []domain.DeliveryHandoverMember{
			member("SYN-UNIT-1", domain.MembershipByReception, "SYN-SCAN-1"), member("SYN-UNIT-2", domain.MembershipByAssignment, "SYN-ASSIGN-2"),
		}}},
	}

	list := scopeReads[2].serve(grantedIntake(), reader, http.MethodGet, scopeReads[2].target)
	var listBody struct {
		Outcome       string                       `json:"outcome"`
		HandoverNotes []map[string]json.RawMessage `json:"handoverNotes"`
	}
	if list.Code != http.StatusOK || json.Unmarshal(list.Body.Bytes(), &listBody) != nil ||
		listBody.Outcome != "DELIVERY_HANDOVER_NOTES_LISTED" || len(listBody.HandoverNotes) != 2 {
		t.Fatalf("列表走样：%d %s", list.Code, list.Body.String())
	}
	open, closed := listBody.HandoverNotes[0], listBody.HandoverNotes[1]
	if string(open["closed"]) != `false` || string(open["scopeEstablished"]) != `false` || string(open["memberCount"]) != `0` {
		t.Errorf("开放那一张走样：%s", list.Body.String())
	}
	for _, key := range []string{"closedAt", "closureFactId", "scopeEstablishedAt"} {
		if _, present := open[key]; present {
			t.Errorf("开放、未成立的交接单带了 %s：%s", key, list.Body.String())
		}
	}
	if string(open["claims"]) != `[{"customerAccount":"SYN-ACCOUNT-1"},{"customerAccount":"SYN-ACCOUNT-3","conclusion":"DENIED","route":"CUSTOMER_CHANNEL","concluder":"https://id.syn.example/dex#syn-client","concludedAt":"2026-10-20T10:00:00Z"}]` {
		t.Errorf("开放那一张的声称 = %s", open["claims"])
	}
	if string(closed["noteId"]) != `"SYN-HN-1"` || string(closed["node"]) != `"SYN-NODE-1"` || string(closed["deliveringParty"]) != `"SYN-DRIVER-1"` ||
		string(closed["openedAt"]) != `"2026-10-20T09:00:00Z"` || string(closed["closed"]) != `true` ||
		string(closed["closureFactId"]) != `"SYN-CLOSE-SYN-HN-1"` || string(closed["closedAt"]) != `"2026-10-20T10:00:00Z"` ||
		string(closed["memberCount"]) != `2` || string(closed["scopeEstablished"]) != `true` ||
		string(closed["scopeEstablishedAt"]) != `"2026-10-20T12:00:00Z"` {
		t.Errorf("已成立那一张走样：%s", list.Body.String())
	}
	if string(closed["claims"]) != `[{"customerAccount":"SYN-ACCOUNT-1","conclusion":"CONFIRMED","route":"OPERATION_DECISION","concluder":"OPERATOR/https://id.syn.example/dex#syn-operator","concludedAt":"2026-10-20T11:00:00Z","deliveringParticipant":"SYN-PARTY-DELIVERER-1","basis":"SYN-BASIS-1"},{"customerAccount":"SYN-ACCOUNT-2","conclusion":"CONFIRMED","route":"CUSTOMER_CHANNEL","concluder":"https://id.syn.example/dex#syn-client","concludedAt":"2026-10-20T12:00:00Z"}]` {
		t.Errorf("已成立那一张的声称 = %s", closed["claims"])
	}
	empty := scopeReads[2].serve(grantedIntake(), &stubDeliveryScopeCatalogue{}, http.MethodGet, scopeReads[2].target)
	if got := arrayAt(t, empty, "handoverNotes"); got != "[]" {
		t.Errorf("空册 handoverNotes = %s, want []", got)
	}

	detail := scopeReads[3].serve(grantedIntake(), reader, http.MethodGet, scopeReads[3].target)
	if detail.Code != http.StatusOK {
		t.Fatalf("详情 status = %d body = %s", detail.Code, detail.Body.String())
	}
	fields := decodeObject(t, detail.Body.Bytes())
	if string(fields["outcome"]) != `"DELIVERY_HANDOVER_NOTE_FOUND"` || reader.gotID != "SYN-HN-1" {
		t.Fatalf("详情走样：%s", detail.Body.String())
	}
	if item := decodeObject(t, fields["handoverNote"]); string(item["memberCount"]) != `2` || string(item["scopeEstablished"]) != `true` {
		t.Errorf("详情那一张走样：%s", detail.Body.String())
	}
	if string(fields["members"]) != `[{"factId":"SYN-SCAN-1","unit":"SYN-UNIT-1","membership":"RECEPTION","formedAt":"2026-10-20T09:10:00Z"},{"factId":"SYN-ASSIGN-2","unit":"SYN-UNIT-2","membership":"ASSIGNMENT","formedAt":"2026-10-20T09:10:00Z"}]` {
		t.Errorf("成员 = %s", fields["members"])
	}

	elsewhere := scopeReads[3]
	elsewhere.pathValue = "SYN-HN-ELSEWHERE"
	missing := elsewhere.serve(grantedIntake(), reader, http.MethodGet, "/node-operations-delivery-handover-notes/SYN-HN-ELSEWHERE")
	if missing.Code != http.StatusNotFound || problemCode(t, missing) != "DELIVERY_HANDOVER_NOTE_NOT_VISIBLE" {
		t.Fatalf("查无 status = %d body = %s", missing.Code, missing.Body.String())
	}
	assertNoOutcome(t, missing)
}

// Covers: 票 13「做什么」的「复核目录」的传输面（ADR-0212 决定五）——待识别册每行带交付范围核对态各键（与收寄答复同名同值）、来源里的依据引用
// （与收寄载荷同一个 {kind, id} 形）与此刻归入的交接单；这一版没记、来源没带、没归入过的各自整键缺席；收寄册的行也带核对态各键。
func TestUnidentifiedItemsCarryTheScopeCheckBasisAndAssignedNote(t *testing.T) {
	check, err := domain.NewDeliveryScopeCheck(domain.DeliveryScopeUnverified, domain.ScopeBasisNotApplicable, domain.ScopeNotAHandoverMember)
	if err != nil {
		t.Fatal(err)
	}
	basis, err := domain.NewDeliveryScopeBasisReference(domain.BasisDeliveryHandoverNote, "SYN-HN-SOURCE")
	if err != nil {
		t.Fatal(err)
	}
	register := &stubReviewCatalogue{
		items: []ports.UnidentifiedItemCatalogueRow{
			{
				SourceID: "scan-scoped", Unit: "unit-1", Node: "node-1", Candidates: []string{"parcel-1/link"},
				ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt, JudgmentVersion: "SYN-INTAKEV-1", SourcePreserved: true,
				IdentityCheck: ports.IdentityChecked, DeliveryScopeCheck: check,
				DeliveryScopeBasis: domain.UnitDeliveryScopeBasis{Source: basis, LastAssignedNote: scopeMust(t, domain.NewDeliveryHandoverNoteID, "SYN-HN-ASSIGNED")},
			},
			{
				SourceID: "scan-bare", Unit: "unit-2", Node: "node-1", Candidates: []string{},
				ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt, JudgmentVersion: "SYN-INTAKEV-2", SourcePreserved: true,
				IdentityCheck: ports.IdentityUnchecked,
			},
		},
		receptions: []ports.ReceptionCatalogueRow{{
			SourceID: "scan-scoped", Kind: "PENDING_IDENTIFICATION", Unit: "unit-1", Node: "node-1",
			ReceivedAt: catalogueBaseAt, ControlKind: "NODE_INTAKE", ControlEstablishedAt: catalogueBaseAt,
			ServiceMarkers: []string{}, RecordedAt: catalogueBaseAt, IdentityCheck: ports.IdentityChecked, DeliveryScopeCheck: check,
		}},
	}
	items := serveGet(recordsEndpoint(register), "/node-operations-records?registry=unidentified-item")
	var itemBody struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if items.Code != http.StatusOK || json.Unmarshal(items.Body.Bytes(), &itemBody) != nil || len(itemBody.Items) != 2 {
		t.Fatalf("待识别册走样：%d %s", items.Code, items.Body.String())
	}
	scoped, bare := itemBody.Items[0], itemBody.Items[1]
	for key, want := range map[string]string{
		"deliveryScopeCheck":            `"UNVERIFIED"`,
		"deliveryScopeUnverifiedReason": `"BASIS_NOT_APPLICABLE"`,
		"deliveryScopeInapplicability":  `"NOT_A_HANDOVER_MEMBER"`,
		"deliveryScopeBasis":            `{"kind":"DELIVERY_HANDOVER_NOTE","id":"SYN-HN-SOURCE"}`,
		"assignedHandoverNote":          `"SYN-HN-ASSIGNED"`,
	} {
		if got := string(scoped[key]); got != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}
	for _, key := range []string{"deliveryScopeCheck", "deliveryScopeUnverifiedReason", "deliveryScopeInapplicability", "deliveryScopeBasis", "assignedHandoverNote"} {
		if _, present := bare[key]; present {
			t.Errorf("没记、没带、没归入的行带了 %s：%s", key, items.Body.String())
		}
	}

	receptions := serveGet(recordsEndpoint(register), "/node-operations-records?registry=reception")
	var receptionBody struct {
		Receptions []map[string]json.RawMessage `json:"receptions"`
	}
	if receptions.Code != http.StatusOK || json.Unmarshal(receptions.Body.Bytes(), &receptionBody) != nil || len(receptionBody.Receptions) != 1 {
		t.Fatalf("收寄册走样：%d %s", receptions.Code, receptions.Body.String())
	}
	if got := fmt.Sprintf("%s/%s/%s", receptionBody.Receptions[0]["deliveryScopeCheck"], receptionBody.Receptions[0]["deliveryScopeUnverifiedReason"],
		receptionBody.Receptions[0]["deliveryScopeInapplicability"]); got != `"UNVERIFIED"/"BASIS_NOT_APPLICABLE"/"NOT_A_HANDOVER_MEMBER"` {
		t.Errorf("收寄册核对态 = %s", got)
	}
}
