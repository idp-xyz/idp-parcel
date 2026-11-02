package nodeopshttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证送站预报与送站交接单在线上的单据号（ADR-0355 决定二、八、十一；ADR-0140 新增字段；票 document-numbering/08）：答复与读口
// 有号带 documentNumber、没号只带 documentNumberAbsence 说为什么，都不答空串、不拿标识顶；编号规则撞号答 200 只带 outcome；列表
// 按 documentNumber 逐字检索。

func numberedAs(t *testing.T, value string) domain.DocumentNumbering {
	t.Helper()
	number, err := domain.NewDocumentNumber(value)
	if err != nil {
		t.Fatal(err)
	}
	return domain.NumberedDocument(number)
}

func rawFields(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("%s：%v", raw, err)
	}
	return fields
}

// Covers: AT-NO-135（传输半边）——首方答复的预报带单据号；单据编号时区未配置时不带单据号、带无号原因；撞号答 200 只带 outcome，不回显
// 预报。
func TestThePreAdviceAnswerCarriesItsDocumentNumber(t *testing.T) {
	for name, testCase := range map[string]struct {
		result  application.DeliveryPreAdviceSubmissionResult
		status  int
		number  string
		absence string
	}{
		"有号": {
			result: application.NewDeliveryPreAdviceSubmissionResult(application.DeliveryPreAdviceRegistered,
				registeredPreAdvice(t).WithDocumentNumbering(numberedAs(t, "PA-20261102-00045"))),
			status: http.StatusCreated, number: `"PA-20261102-00045"`,
		},
		"单据编号时区未配置": {
			result: application.NewDeliveryPreAdviceSubmissionResult(application.DeliveryPreAdviceRegistered,
				registeredPreAdvice(t).WithDocumentNumbering(domain.UnnumberedForMissingTimeZone())),
			status: http.StatusCreated, absence: `"NUMBERING_TIME_ZONE_NOT_CONFIGURED"`,
		},
	} {
		rig := newPreAdviceRig(t)
		rig.handler.submission = testCase.result
		response := post(rig.submit, preAdviceSubmissionBody)
		if response.Code != testCase.status {
			t.Fatalf("%s：status = %d %s", name, response.Code, response.Body.String())
		}
		preAdvice := rawFields(t, rawFields(t, response.Body.Bytes())["preAdvice"])
		if string(preAdvice["documentNumber"]) != testCase.number || string(preAdvice["documentNumberAbsence"]) != testCase.absence ||
			string(preAdvice["preAdviceId"]) != `"PREADV-SYN-0001"` {
			t.Fatalf("%s：答复 = %s", name, response.Body.String())
		}
	}

	rig := newPreAdviceRig(t)
	rig.handler.submission = application.NewDeliveryPreAdviceSubmissionResult(application.DeliveryPreAdviceNumberingRuleCollision, domain.DeliveryPreAdvice{})
	response := post(rig.submit, preAdviceSubmissionBody)
	body := rawFields(t, response.Body.Bytes())
	if response.Code != http.StatusOK || string(body["outcome"]) != `"NUMBERING_RULE_COLLISION"` || len(body) != 1 {
		t.Fatalf("撞号：%d %s", response.Code, response.Body.String())
	}
}

// Covers: AT-NO-136（传输半边）——开立口的答复带交接单单据号与设备签发的标识；撞号答 200 只带 outcome，不回显交接单。
func TestTheHandoverOpeningAnswerCarriesItsDocumentNumber(t *testing.T) {
	note, _ := handoverHTTPNote(t, false)
	line := handoverLines["open"]
	handler := &handoverHandlerDouble{result: application.NewDeliveryHandoverResult(application.DeliveryHandoverNoteOpened,
		application.DeliveryHandoverAnswer{Note: note.WithDocumentNumbering(numberedAs(t, "HO-20261102-00018"))})}
	response := httptest.NewRecorder()
	line.endpoint(t, handler, nil).ServeHTTP(response, handoverRequest(line, line.body))
	opened := rawFields(t, rawFields(t, response.Body.Bytes())["handoverNote"])
	if response.Code != http.StatusCreated || string(opened["documentNumber"]) != `"HO-20261102-00018"` ||
		string(opened["noteId"]) != `"SYN-HANDOVER-HTTP-01"` || opened["documentNumberAbsence"] != nil {
		t.Fatalf("开立答复 = %d %s", response.Code, response.Body.String())
	}

	collided := &handoverHandlerDouble{result: application.NewDeliveryHandoverResult(application.DeliveryHandoverNumberingRuleCollision, application.DeliveryHandoverAnswer{})}
	response = httptest.NewRecorder()
	line.endpoint(t, collided, nil).ServeHTTP(response, handoverRequest(line, line.body))
	body := rawFields(t, response.Body.Bytes())
	if response.Code != http.StatusOK || string(body["outcome"]) != `"NUMBERING_RULE_COLLISION"` || body["handoverNote"] != nil {
		t.Fatalf("撞号：%d %s", response.Code, response.Body.String())
	}
}

// Covers: AT-NO-136 的客户确认一格（ADR-0355 决定十一，协调方裁定）——客户确认口的答复在交接单标识之外带交接单号；没有号的带无号
// 原因；原有各格不变，不因此回显声称了谁。
func TestTheCustomerConclusionAnswerCarriesTheHandoverNoteNumber(t *testing.T) {
	note, closure := handoverHTTPNote(t, true)
	binding, err := domain.FormDeliveryHandoverBinding(domain.DeliveryHandoverBindingSpec{
		TenantID: note.TenantID(), Note: note.ID(), Account: adapterValue(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Conclusion: domain.BindingConfirmed, Route: domain.ConcludedViaCustomerChannel, Concluder: "SYN-CLIENT",
		SourceRequestKey: "SYN-KEY-CONFIRM-01", ConcludedAt: note.OpenedAt().Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	line := handoverLines["conclude"]
	for name, testCase := range map[string]struct {
		numbering domain.DocumentNumbering
		field     string
		want      string
	}{
		"有号":        {numberedAs(t, "HO-20261102-00018"), "documentNumber", `"HO-20261102-00018"`},
		"单据编号时区未配置": {domain.UnnumberedForMissingTimeZone(), "documentNumberAbsence", `"NUMBERING_TIME_ZONE_NOT_CONFIGURED"`},
	} {
		handler := &handoverHandlerDouble{result: application.NewDeliveryHandoverResult(application.DeliveryHandoverBindingConfirmed,
			application.DeliveryHandoverAnswer{Note: note.WithDocumentNumbering(testCase.numbering), Closure: closure, Binding: binding})}
		response := httptest.NewRecorder()
		line.endpoint(t, handler, nil).ServeHTTP(response, handoverRequest(line, line.body))
		body := rawFields(t, response.Body.Bytes())
		if response.Code != http.StatusCreated || string(body[testCase.field]) != testCase.want || string(body["noteId"]) != `"SYN-HANDOVER-HTTP-01"` ||
			body["binding"] == nil || body["handoverNote"] != nil || body["claims"] != nil {
			t.Fatalf("%s：%d %s", name, response.Code, response.Body.String())
		}
	}
}

// Covers: AT-NO-137（传输半边）——送站预报与送站交接单的列表收 documentNumber 逐字检索，空白答 400、读口不被触到；列表与详情行有号带号，上线前开立
// 与单据编号时区未配置各带各的无号原因。
func TestDeliveryScopeReadsCarryAndSearchByDocumentNumber(t *testing.T) {
	for _, read := range []scopeRead{scopeReads[0], scopeReads[2]} {
		reader := &stubDeliveryScopeCatalogue{}
		if response := read.serve(grantedIntake(), reader, http.MethodGet, read.target+"?documentNumber=PA-20261102-00045"); response.Code != http.StatusOK {
			t.Fatalf("%s：status = %d", read.name, response.Code)
		}
		if reader.gotFilter.DocumentNumber.String() != "PA-20261102-00045" {
			t.Fatalf("%s：读口收到的单据号 = %q", read.name, reader.gotFilter.DocumentNumber)
		}
		for _, query := range []string{"?documentNumber=", "?documentNumber=%20", "?documentNumber=a&documentNumber=b"} {
			blank := &stubDeliveryScopeCatalogue{}
			if response := read.serve(grantedIntake(), blank, http.MethodGet, read.target+query); response.Code != http.StatusBadRequest || blank.calls != 0 {
				t.Fatalf("%s%s：status = %d calls = %d", read.name, query, response.Code, blank.calls)
			}
		}
	}

	numberedPreAdvice := registeredPreAdvice(t).WithDocumentNumbering(numberedAs(t, "PA-20261102-00045"))
	legacy, err := domain.RehydrateDocumentNumbering("", "")
	if err != nil {
		t.Fatal(err)
	}
	reader := &stubDeliveryScopeCatalogue{preAdvices: []domain.DeliveryPreAdviceState{
		{PreAdvice: numberedPreAdvice},
		{PreAdvice: registeredPreAdvice(t).WithDocumentNumbering(legacy)},
	}}
	response := scopeReads[0].serve(grantedIntake(), reader, http.MethodGet, scopeReads[0].target)
	var listed struct {
		PreAdvices []map[string]json.RawMessage `json:"preAdvices"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil || len(listed.PreAdvices) != 2 {
		t.Fatalf("预报列表 = %s", response.Body.String())
	}
	if string(listed.PreAdvices[0]["documentNumber"]) != `"PA-20261102-00045"` || listed.PreAdvices[0]["documentNumberAbsence"] != nil ||
		listed.PreAdvices[1]["documentNumber"] != nil || string(listed.PreAdvices[1]["documentNumberAbsence"]) != `"ISSUED_BEFORE_NUMBERING"` {
		t.Fatalf("预报列表的单据号与无号原因 = %s", response.Body.String())
	}

	note, _ := handoverHTTPNote(t, false)
	notes := &stubDeliveryScopeCatalogue{
		notes: []ports.DeliveryHandoverNoteCatalogueRow{{State: domain.DeliveryHandoverNoteState{Note: note.WithDocumentNumbering(domain.UnnumberedForMissingTimeZone())}}},
		note: map[string]ports.DeliveryHandoverNoteDetail{
			"SYN-HN-1": {State: domain.DeliveryHandoverNoteState{Note: note.WithDocumentNumbering(numberedAs(t, "HO-20261102-00018"))}},
		},
	}
	response = scopeReads[2].serve(grantedIntake(), notes, http.MethodGet, scopeReads[2].target)
	var noteList struct {
		HandoverNotes []map[string]json.RawMessage `json:"handoverNotes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &noteList); err != nil || len(noteList.HandoverNotes) != 1 ||
		noteList.HandoverNotes[0]["documentNumber"] != nil || string(noteList.HandoverNotes[0]["documentNumberAbsence"]) != `"NUMBERING_TIME_ZONE_NOT_CONFIGURED"` {
		t.Fatalf("交接单列表 = %s", response.Body.String())
	}
	response = scopeReads[3].serve(grantedIntake(), notes, http.MethodGet, scopeReads[3].target)
	detail := rawFields(t, rawFields(t, response.Body.Bytes())["handoverNote"])
	if string(detail["documentNumber"]) != `"HO-20261102-00018"` {
		t.Fatalf("交接单详情 = %s", response.Body.String())
	}
}
