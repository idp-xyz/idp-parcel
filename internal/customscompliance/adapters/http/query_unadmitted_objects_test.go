package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

type stubUnadmittedObjects struct {
	listed    ports.UnadmittedObjects
	err       error
	gotTenant string
	gotLimit  int
}

func (stub *stubUnadmittedObjects) ListUnadmittedObjects(
	_ context.Context, tenant domain.TenantID, limit int, _ cataloguepage.Query,
) (ports.UnadmittedObjects, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.listed, stub.err
}

func serveUnadmitted(t *testing.T, intake customshttp.CatalogueQueryIntake, stub *stubUnadmittedObjects, method string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	customshttp.NewQueryUnadmittedObjectsEndpoint(intake, stub).
		ServeHTTP(response, httptest.NewRequest(method, "/customs-unadmitted-objects", nil))
	return response
}

// Covers: AT-CC-456 — 件数是读口答的全部件数，不是这一页的行数；还有下一页即标截断（该票 spec「已带计数与 truncated 的待办队列保留这两格」）；行形与判断册同一份；
// 租户与页大小只取自接入面。
func TestUnadmittedObjectQueryCarriesTheWholeCountAndMarksTruncation(t *testing.T) {
	stub := &stubUnadmittedObjects{listed: ports.UnadmittedObjects{
		Count: 4,
		Result: cataloguepage.Result[ports.ActionAdmissionJudgementRecord]{Next: "SYN-NEXT-PAGE", Total: 4, Rows: []ports.ActionAdmissionJudgementRecord{{
			Judgement: "J-1", Object: "PCL-1", Intent: "DEPARTURE", Node: "NODE-A", OnwardNode: "NODE-B",
			ActionAt: catalogueBaseAt, CrossZone: "CROSSING", Answer: "BLOCKED", JudgedAt: catalogueBaseAt,
			Findings: []ports.AdmissionFindingRecord{{Action: "CROSS_CUSTOMS_MOVEMENT", Answer: "BLOCKED",
				Bases: []ports.AdmissionBasisRecord{{Cause: "RELEASE_NOT_RECEIVED", Scope: "UNIT-1"}}}},
		}}},
	}}
	response := serveUnadmitted(t, grantedCatalogueIntake{tenant: "T1", limit: 5}, stub, http.MethodGet)
	if response.Code != http.StatusOK || stub.gotTenant != "T1" || stub.gotLimit != 5 {
		t.Fatalf("status = %d tenant = %q limit = %d", response.Code, stub.gotTenant, stub.gotLimit)
	}
	var body struct {
		Outcome         string `json:"outcome"`
		UnadmittedCount int    `json:"unadmittedCount"`
		Truncated       bool   `json:"truncated"`
		Judgements      []struct {
			Object   string `json:"object"`
			Answer   string `json:"answer"`
			Findings []struct {
				Bases []struct {
					Sources json.RawMessage `json:"sources"`
				} `json:"bases"`
			} `json:"findings"`
		} `json:"judgements"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "UNADMITTED_OBJECTS_LISTED" || body.UnadmittedCount != 4 || !body.Truncated {
		t.Fatalf("件数或截断走样（件数要的是全部，不是这一页）：%s", response.Body.String())
	}
	if len(body.Judgements) != 1 || body.Judgements[0].Object != "PCL-1" || body.Judgements[0].Answer != "BLOCKED" ||
		string(body.Judgements[0].Findings[0].Bases[0].Sources) != "[]" {
		t.Fatalf("行形与判断册不同：%s", response.Body.String())
	}
}

// Covers: AT-CC-456 — 没有卡关对象答 0 件与空数组、不截断；读不回答 NO_ANSWER_FORMED 不伪装成 0 件；
// 未授予答 403、读口不被触到；非 GET 答 405。
func TestUnadmittedObjectQueryAnswersZeroOnlyWhenTheReadAnswersZero(t *testing.T) {
	granted := grantedCatalogueIntake{tenant: "T1", limit: 5}
	response := serveUnadmitted(t, granted, &stubUnadmittedObjects{}, http.MethodGet)
	var body struct {
		UnadmittedCount int             `json:"unadmittedCount"`
		Truncated       bool            `json:"truncated"`
		Judgements      json.RawMessage `json:"judgements"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body.UnadmittedCount != 0 || body.Truncated || string(body.Judgements) != "[]" {
		t.Fatalf("空答案走样：%d %s", response.Code, response.Body.String())
	}

	failing := serveUnadmitted(t, granted, &stubUnadmittedObjects{err: errors.New("down")}, http.MethodGet)
	if failing.Code != http.StatusInternalServerError || problemCode(t, failing) != "NO_ANSWER_FORMED" {
		t.Fatalf("读不回必须答未形成：%d %s", failing.Code, failing.Body.String())
	}

	untouched := &stubUnadmittedObjects{}
	refused := serveUnadmitted(t, customshttp.UnconfiguredIntake{}, untouched, http.MethodGet)
	if refused.Code != http.StatusForbidden || untouched.gotTenant != "" {
		t.Fatalf("未授予必须 403 且不触读口：%d tenant=%q", refused.Code, untouched.gotTenant)
	}

	posted := serveUnadmitted(t, granted, &stubUnadmittedObjects{}, http.MethodPost)
	if posted.Code != http.StatusMethodNotAllowed || posted.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("非 GET 必须 405：%d allow=%q", posted.Code, posted.Header().Get("Allow"))
	}
}
