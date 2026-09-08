package commercialhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉词表读口的传输形状（票 admin-write-faces/20「要做的」http 三格：有集 / 空集 / 坏 kind），外加每个
// 端点都要过的两道门（方法门、未配置 Intake）。集合的内容归领域测试钉，这里只证「领域答什么、线上就是什么」。

// vocabularyIntakeDouble 是准入替身：err 为 nil 即放行；它不读请求，与真 Intake 的分界一致。
type vocabularyIntakeDouble struct{ err error }

func (double vocabularyIntakeDouble) IntakePublicationVocabularyQuery(context.Context, *http.Request) error {
	return double.err
}

func serveVocabulary(intake commercialhttp.PublicationVocabularyIntake, method, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	commercialhttp.NewQueryPublicationVocabularyEndpoint(intake).ServeHTTP(response, httptest.NewRequest(method, target, nil))
	return response
}

type vocabularyAnswer struct {
	Outcome string `json:"outcome"`
	Kind    string `json:"kind"`
	Sets    []struct {
		Name  string   `json:"name"`
		Codes []string `json:"codes"`
	} `json:"sets"`
}

func decodeVocabulary(t *testing.T, response *httptest.ResponseRecorder) vocabularyAnswer {
	t.Helper()
	var body vocabularyAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body
}

// Covers: 票 20 完成判据「kind=ACCEPTANCE_RULE_PACKAGE 答票 12 点名的那几个集合、码序同领域枚举」的线上半边——
// 200 + PUBLICATION_VOCABULARY_LISTED，kind 回显原词，sets 与领域口逐集合同名、同码、同序。
func TestVocabularyEndpointAnswersTheRegisterSets(t *testing.T) {
	response := serveVocabulary(vocabularyIntakeDouble{}, http.MethodGet, "/commercial-publication-vocabularies?kind=ACCEPTANCE_RULE_PACKAGE")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d %s", response.Code, response.Body.Bytes())
	}
	body := decodeVocabulary(t, response)
	if body.Outcome != "PUBLICATION_VOCABULARY_LISTED" || body.Kind != "ACCEPTANCE_RULE_PACKAGE" {
		t.Fatalf("answer = %s", response.Body.Bytes())
	}

	sets, err := domain.PublicationVocabulary(domain.AcceptanceRulePackageObject)
	if err != nil {
		t.Fatalf("domain vocabulary: %v", err)
	}
	if len(body.Sets) != len(sets) || len(sets) == 0 {
		t.Fatalf("sets on the wire = %d, domain = %d (%s)", len(body.Sets), len(sets), response.Body.Bytes())
	}
	for index, set := range sets {
		if body.Sets[index].Name != set.Name || strings.Join(body.Sets[index].Codes, ",") != strings.Join(set.Codes, ",") {
			t.Fatalf("set[%d] on the wire = %+v, domain = %+v", index, body.Sets[index], set)
		}
	}
	// 票 12 点名的两格要在线上看得见；表单据它们供下拉。
	found := map[string]bool{}
	for _, set := range body.Sets {
		found[set.Name] = true
	}
	if !found["stage"] || !found["intent"] {
		t.Fatalf("stage / intent missing from %s", response.Body.Bytes())
	}
}

// Covers: 票 20 裁决三「某册没有封闭集就答空集合列表，不答 404」——kind=SERVICE_PRODUCT 答 200，sets 是空数组
// （不是缺键、不是 null）：调用方按 sets 迭代，什么也不用特判。
func TestVocabularyEndpointAnswersAnEmptyListForRegistersWithoutClosedSets(t *testing.T) {
	response := serveVocabulary(vocabularyIntakeDouble{}, http.MethodGet, "/commercial-publication-vocabularies?kind=SERVICE_PRODUCT")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d %s", response.Code, response.Body.Bytes())
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if string(fields["kind"]) != `"SERVICE_PRODUCT"` || string(fields["sets"]) != `[]` || string(fields["outcome"]) != `"PUBLICATION_VOCABULARY_LISTED"` {
		t.Fatalf("answer = %s; want kind echoed, sets = [] and the listed outcome", response.Body.Bytes())
	}
}

// Covers: 票 20 裁决四「未知 kind 答问题不答空」——集合外与缺席的 kind 都答 400 + MALFORMED_REQUEST，响应体带
// kind 那一格的问题（与命令口的逐格问题同一形状）；kind 属传输形状、先于 Intake，未配置 Intake 也不把它盖成 403。
func TestVocabularyEndpointRefusesAnUnknownKindWithAFieldProblem(t *testing.T) {
	for name, target := range map[string]string{
		"unknown": "/commercial-publication-vocabularies?kind=PRICE_CARD",
		"lower":   "/commercial-publication-vocabularies?kind=settlement_policy",
		"missing": "/commercial-publication-vocabularies",
	} {
		response := serveVocabulary(vocabularyIntakeDouble{err: commercialhttp.ErrAccessChannelNotConfigured}, http.MethodGet, target)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d %s, want 400", name, response.Code, response.Body.Bytes())
		}
		var body struct {
			Error struct {
				Code     string `json:"code"`
				Problems []struct {
					Field   string `json:"field"`
					Problem string `json:"problem"`
				} `json:"problems"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decode %s: %v", name, response.Body.Bytes(), err)
		}
		if body.Error.Code != "MALFORMED_REQUEST" || len(body.Error.Problems) != 1 || body.Error.Problems[0].Field != "kind" || body.Error.Problems[0].Problem == "" {
			t.Fatalf("%s: body = %s", name, response.Body.Bytes())
		}
		if strings.Contains(response.Body.String(), `"sets"`) || strings.Contains(response.Body.String(), `"outcome"`) {
			t.Fatalf("%s: a refused kind must not look like an answer: %s", name, response.Body.Bytes())
		}
	}
}

// Covers: ADR-0055 — 合法 kind 遇未配置 Intake 答 403 + ACCESS_CHANNEL_NOT_CONFIGURED 且不带 outcome；方法门在
// 最前，非 GET 答 405 + Allow: GET。UnconfiguredIntake 本身就是装配点挂的那个值，这里拿真值不拿替身。
func TestVocabularyEndpointKeepsTheUnconfiguredAndMethodGates(t *testing.T) {
	unconfigured := serveVocabulary(commercialhttp.UnconfiguredIntake{}, http.MethodGet, "/commercial-publication-vocabularies?kind=SETTLEMENT_POLICY")
	if unconfigured.Code != http.StatusForbidden {
		t.Fatalf("unconfigured: status = %d %s", unconfigured.Code, unconfigured.Body.Bytes())
	}
	var problem struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		Outcome *string `json:"outcome"`
	}
	if err := json.Unmarshal(unconfigured.Body.Bytes(), &problem); err != nil || problem.Error.Code != "ACCESS_CHANNEL_NOT_CONFIGURED" || problem.Outcome != nil {
		t.Fatalf("unconfigured: body = %s (%v)", unconfigured.Body.Bytes(), err)
	}

	wrongMethod := serveVocabulary(vocabularyIntakeDouble{}, http.MethodPost, "/commercial-publication-vocabularies?kind=SETTLEMENT_POLICY")
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST: status = %d, Allow = %q", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}
}
