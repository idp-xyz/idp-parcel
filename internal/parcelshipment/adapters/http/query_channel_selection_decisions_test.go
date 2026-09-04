package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var selectionDecidedAtFixture = time.Date(2026, 9, 4, 18, 30, 0, 0, time.UTC)

// channelSelectionIntakeDouble 只交出租户与页大小——本端点的 Intake 接口就只有这些。它不能用委托查阅那个
// Intake 顶替：择优对象是商业范围与产品—渠道映射，不是客户账户，收一个必带账户维的作用域等于在类型上
// 声称会按账户过滤而它不会（与面单交易查阅同一条理由）。
type channelSelectionIntakeDouble struct {
	tenant domain.TenantID
	limit  int
	err    error
}

func (double *channelSelectionIntakeDouble) IntakeChannelSelectionDecisionQuery(
	_ context.Context,
	_ *http.Request,
) (shipmenthttp.ChannelSelectionDecisionQuery, error) {
	if double.err != nil {
		return shipmenthttp.ChannelSelectionDecisionQuery{}, double.err
	}
	return shipmenthttp.ChannelSelectionDecisionQuery{Tenant: double.tenant, Limit: double.limit}, nil
}

func channelSelectionIntake(t *testing.T) *channelSelectionIntakeDouble {
	t.Helper()
	return &channelSelectionIntakeDouble{
		tenant: mustValue(t, domain.NewTenantID, "TENANT-1"),
		limit:  50,
	}
}

type channelSelectionReaderDouble struct {
	listed []domain.ChannelSelectionDecision
	found  domain.ChannelSelectionDecision
	has    bool
	err    error

	tenant   domain.TenantID
	filter   ports.TiedChannelSelectionFilter
	limit    int
	askedFor domain.ChannelSelectionDecisionID
}

func (double *channelSelectionReaderDouble) ListTiedChannelSelectionDecisions(
	_ context.Context,
	tenant domain.TenantID,
	filter ports.TiedChannelSelectionFilter,
	limit int,
) ([]domain.ChannelSelectionDecision, error) {
	double.tenant, double.filter, double.limit = tenant, filter, limit
	if double.err != nil {
		return nil, double.err
	}
	return double.listed, nil
}

func (double *channelSelectionReaderDouble) FindChannelSelectionDecision(
	_ context.Context,
	tenant domain.TenantID,
	id domain.ChannelSelectionDecisionID,
) (domain.ChannelSelectionDecision, bool, error) {
	double.tenant, double.askedFor = tenant, id
	if double.err != nil {
		return domain.ChannelSelectionDecision{}, false, double.err
	}
	return double.found, double.has, nil
}

func selectionSubject(t *testing.T, scope, mapping string) domain.ChannelSelectionSubject {
	t.Helper()
	subject, err := domain.NewChannelSelectionSubject(
		mustValue(t, domain.NewCommercialScopeReference, scope),
		mustValue(t, domain.NewProductChannelMappingReference, mapping),
	)
	if err != nil {
		t.Fatalf("造对象引用：%v", err)
	}
	return subject
}

func pricedCandidate(t *testing.T, id, amount, evaluation string) domain.ChannelCandidateCost {
	t.Helper()
	cost, err := domain.PricedChannelCandidate(
		mustValue(t, domain.NewChannelCandidateID, id),
		mustValue(t, domain.NewChannelCostAmount, amount),
		mustValue(t, domain.NewChannelCostCurrency, "SYN"),
	)
	if err != nil {
		t.Fatalf("造已定价候选 %s：%v", id, err)
	}
	if evaluation == "" {
		return cost
	}
	cost, err = cost.WithEvaluation(mustValue(t, domain.NewChannelCostEvaluationReference, evaluation))
	if err != nil {
		t.Fatalf("带评价引用：%v", err)
	}
	return cost
}

func excludedCandidate(t *testing.T, id string, grade domain.ChannelCostUnavailability) domain.ChannelCandidateCost {
	t.Helper()
	cost, err := domain.UnpriceableChannelCandidate(mustValue(t, domain.NewChannelCandidateID, id), grade)
	if err != nil {
		t.Fatalf("造不可计价候选 %s：%v", id, err)
	}
	return cost
}

func decisionOf(t *testing.T, id string, decidedAt time.Time, costs ...domain.ChannelCandidateCost) domain.ChannelSelectionDecision {
	t.Helper()
	decision, err := domain.FormChannelSelectionDecision(domain.ChannelSelectionDecisionSpec{
		ID:            mustValue(t, domain.NewChannelSelectionDecisionID, id),
		Tenant:        mustValue(t, domain.NewTenantID, "TENANT-1"),
		Subject:       selectionSubject(t, "scope-a", "mapping-1"),
		AssembledAsOf: decidedAt.Add(-time.Minute),
		DecidedAt:     decidedAt,
		Costs:         costs,
	})
	if err != nil {
		t.Fatalf("形成决定 %s：%v", id, err)
	}
	return decision
}

// tiedDecision 是一条并列冲突：两家同价并列，一家出局带因由。
func tiedDecision(t *testing.T, id string, decidedAt time.Time) domain.ChannelSelectionDecision {
	t.Helper()
	return decisionOf(t, id, decidedAt,
		pricedCandidate(t, "cand-a", "10.00", "eval-a"),
		pricedCandidate(t, "cand-b", "10.00", ""),
		excludedCandidate(t, "cand-c", domain.ChannelCostRatecardExclusion),
	)
}

func getChannelSelectionDecisions(
	t *testing.T,
	intake shipmenthttp.ChannelSelectionDecisionQueryIntake,
	reader shipmenthttp.ChannelSelectionDecisionsReader,
	method, target string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := shipmenthttp.NewQueryChannelSelectionDecisionsEndpoint(intake, reader)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(method, target, nil))
	return response
}

type channelSelectionDecisionBody struct {
	DecisionID        string `json:"decisionId"`
	Scope             string `json:"scope"`
	Mapping           string `json:"mapping"`
	AssembledAsOf     string `json:"assembledAsOf"`
	Rule              string `json:"rule"`
	DecidedAt         string `json:"decidedAt"`
	Conclusion        string `json:"conclusion"`
	SelectedCandidate string `json:"selectedCandidate"`
	Candidates        []struct {
		Candidate  string `json:"candidate"`
		Evaluation string `json:"evaluation"`
		Outcome    string `json:"outcome"`
		Exclusion  string `json:"exclusion"`
	} `json:"candidates"`
}

type channelSelectionListBody struct {
	Outcome   string                         `json:"outcome"`
	Decisions []channelSelectionDecisionBody `json:"decisions"`
}

type channelSelectionDetailBody struct {
	Outcome  string                        `json:"outcome"`
	Decision *channelSelectionDecisionBody `json:"decision"`
}

func decodeInto(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
}

// Covers: 并列冲突列表——视图词 tied 必备；每条决定整份透出（头部 + 逐候选四格、出局因由、评价引用有则带
// 无则缺席）；读口收到的是（租户，不收窄，limit）。没有金额、没有评价内容：读面只透评价引用（票 23 红线）。
func TestTiedDecisionsListTransmitsEachDecisionWithItsCandidates(t *testing.T) {
	reader := &channelSelectionReaderDouble{listed: []domain.ChannelSelectionDecision{
		tiedDecision(t, "CSDN-2", selectionDecidedAtFixture.Add(time.Hour)),
		tiedDecision(t, "CSDN-1", selectionDecidedAtFixture),
	}}
	response := getChannelSelectionDecisions(t, channelSelectionIntake(t), reader,
		http.MethodGet, "/channel-selection-decisions?view=tied")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200；body = %s", response.Code, response.Body)
	}
	var body channelSelectionListBody
	decodeInto(t, response, &body)
	if body.Outcome != "TIED_CHANNEL_SELECTION_DECISIONS_LISTED" {
		t.Fatalf("outcome = %q", body.Outcome)
	}
	if len(body.Decisions) != 2 || body.Decisions[0].DecisionID != "CSDN-2" || body.Decisions[1].DecisionID != "CSDN-1" {
		t.Fatalf("应按读口交回的顺序原样透出两条：%+v", body.Decisions)
	}

	first := body.Decisions[0]
	if first.Scope != "scope-a" || first.Mapping != "mapping-1" || first.Rule != "COST_ONLY" || first.Conclusion != "TIED" {
		t.Fatalf("头部透出不对：%+v", first)
	}
	if first.DecidedAt != selectionDecidedAtFixture.Add(time.Hour).Format(time.RFC3339Nano) ||
		first.AssembledAsOf != selectionDecidedAtFixture.Add(59*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("两个时刻透出不对：%+v", first)
	}
	if first.SelectedCandidate != "" {
		t.Fatalf("并列冲突没有选中者，却透出了 %q", first.SelectedCandidate)
	}
	if len(first.Candidates) != 3 {
		t.Fatalf("逐候选 %d 条，want 3", len(first.Candidates))
	}
	if first.Candidates[0].Candidate != "cand-a" || first.Candidates[0].Outcome != "TIED" || first.Candidates[0].Evaluation != "eval-a" {
		t.Fatalf("并列且带评价引用的候选透出不对：%+v", first.Candidates[0])
	}
	if first.Candidates[1].Candidate != "cand-b" || first.Candidates[1].Outcome != "TIED" || first.Candidates[1].Evaluation != "" {
		t.Fatalf("并列且无评价引用的候选透出不对：%+v", first.Candidates[1])
	}
	if first.Candidates[2].Candidate != "cand-c" || first.Candidates[2].Outcome != "EXCLUDED" || first.Candidates[2].Exclusion != "RATECARD_EXCLUSION" {
		t.Fatalf("出局候选应带因由：%+v", first.Candidates[2])
	}

	if reader.tenant.String() != "TENANT-1" || reader.limit != 50 {
		t.Fatalf("读口收到的作用域不对：tenant = %s, limit = %d", reader.tenant, reader.limit)
	}
	if _, narrowed := reader.filter.Subject(); narrowed {
		t.Fatal("没给对象参数却收窄了")
	}

	lowered := strings.ToLower(response.Body.String())
	for _, forbidden := range []string{"amount", "currency", "price"} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("读面透出了金额相关字段 %q：%s", forbidden, response.Body)
		}
	}
}

// Covers: 按对象收窄要范围与映射同时给——只给一半是说不清对象的请求，拒在传输形状上；两个都给时读口收到
// 收窄后的过滤器。视图词是封闭集：缺席或集外一律 400，「没传就当全部」是隐含默认。
func TestTiedDecisionsListNarrowsBySubjectOnlyWhenBothHalvesAreGiven(t *testing.T) {
	reader := &channelSelectionReaderDouble{}
	response := getChannelSelectionDecisions(t, channelSelectionIntake(t), reader,
		http.MethodGet, "/channel-selection-decisions?view=tied&scope=scope-a&mapping=mapping-1")
	if response.Code != http.StatusOK {
		t.Fatalf("收窄 status = %d, want 200；body = %s", response.Code, response.Body)
	}
	subject, narrowed := reader.filter.Subject()
	if !narrowed || subject.Scope().String() != "scope-a" || subject.Mapping().String() != "mapping-1" {
		t.Fatalf("读口应收到收窄对象：%v/%v", subject, narrowed)
	}

	for _, target := range []string{
		"/channel-selection-decisions?view=tied&scope=scope-a",
		"/channel-selection-decisions?view=tied&mapping=mapping-1",
		"/channel-selection-decisions",
		"/channel-selection-decisions?view=all",
		"/channel-selection-decisions?view=TIED",
	} {
		malformed := getChannelSelectionDecisions(t, channelSelectionIntake(t), &channelSelectionReaderDouble{}, http.MethodGet, target)
		if malformed.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", target, malformed.Code)
		}
		if got := problemCode(t, malformed); got != "MALFORMED_REQUEST" {
			t.Fatalf("%s: code = %q", target, got)
		}
		assertNoOutcomeField(t, malformed)
	}
}

// Covers: 没有冲突是答案不是错误——200 + 空数组（ADR-0077 Decision 四），与「接入渠道未配置」（403）在答复上
// 分得开。
func TestNoTiedDecisionsIsAnAnswerNotAnError(t *testing.T) {
	response := getChannelSelectionDecisions(t, channelSelectionIntake(t), &channelSelectionReaderDouble{},
		http.MethodGet, "/channel-selection-decisions?view=tied")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(fields["decisions"]) != "[]" {
		t.Fatalf("decisions = %s, want []（空数组不是 null）", fields["decisions"])
	}
}

// Covers: 按标识取一条——`decisionId` 在场即走单份分支；整份透出含选中者；找不到答 404 + 单一 code、不带
// outcome（`统一不可见结果`：不存在与属别的租户同答，ADR-0029）；空标识拒在传输形状上。
func TestASingleDecisionIsServedByIdentity(t *testing.T) {
	found := decisionOf(t, "CSDN-9", selectionDecidedAtFixture,
		pricedCandidate(t, "cand-a", "8.00", "eval-a"),
		pricedCandidate(t, "cand-b", "9.00", "eval-b"),
	)
	reader := &channelSelectionReaderDouble{found: found, has: true}
	response := getChannelSelectionDecisions(t, channelSelectionIntake(t), reader,
		http.MethodGet, "/channel-selection-decisions?decisionId=CSDN-9")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200；body = %s", response.Code, response.Body)
	}
	var body channelSelectionDetailBody
	decodeInto(t, response, &body)
	if body.Outcome != "CHANNEL_SELECTION_DECISION" || body.Decision == nil {
		t.Fatalf("单份答复不对：%+v", body)
	}
	if body.Decision.DecisionID != "CSDN-9" || body.Decision.Conclusion != "SELECTED" || body.Decision.SelectedCandidate != "cand-a" {
		t.Fatalf("选出唯一者的记录透出不对：%+v", body.Decision)
	}
	if len(body.Decision.Candidates) != 2 || body.Decision.Candidates[0].Outcome != "SELECTED" || body.Decision.Candidates[1].Outcome != "NOT_SELECTED" {
		t.Fatalf("逐候选四格透出不对：%+v", body.Decision.Candidates)
	}
	if reader.askedFor.String() != "CSDN-9" || reader.tenant.String() != "TENANT-1" {
		t.Fatalf("读口收到的键不对：%s / %s", reader.tenant, reader.askedFor)
	}

	missing := getChannelSelectionDecisions(t, channelSelectionIntake(t), &channelSelectionReaderDouble{},
		http.MethodGet, "/channel-selection-decisions?decisionId=CSDN-404")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("找不到 status = %d, want 404", missing.Code)
	}
	if got := problemCode(t, missing); got != "CHANNEL_SELECTION_DECISION_NOT_VISIBLE" {
		t.Fatalf("找不到 code = %q", got)
	}
	assertNoOutcomeField(t, missing)

	blank := getChannelSelectionDecisions(t, channelSelectionIntake(t), &channelSelectionReaderDouble{},
		http.MethodGet, "/channel-selection-decisions?decisionId=%20")
	if blank.Code != http.StatusBadRequest {
		t.Fatalf("空标识 status = %d, want 400", blank.Code)
	}
}

// Covers: ADR-0055 未配置即拒与 ADR-0029 的分格——两条分支在渠道未配置时同答 403；读口答不出答 5xx；非 GET
// 拒在方法门上。
func TestChannelSelectionDecisionsAnswersSplitByRecoveryAction(t *testing.T) {
	for _, target := range []string{
		"/channel-selection-decisions?view=tied",
		"/channel-selection-decisions?decisionId=CSDN-1",
	} {
		unconfigured := getChannelSelectionDecisions(t, shipmenthttp.UnconfiguredIntake{},
			&channelSelectionReaderDouble{}, http.MethodGet, target)
		if unconfigured.Code != http.StatusForbidden {
			t.Fatalf("%s 未配置渠道 status = %d, want 403", target, unconfigured.Code)
		}
		if got := problemCode(t, unconfigured); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s 未配置渠道 code = %q", target, got)
		}
		assertNoOutcomeField(t, unconfigured)
	}

	failing := getChannelSelectionDecisions(t, channelSelectionIntake(t),
		&channelSelectionReaderDouble{err: errors.New("库连不上")}, http.MethodGet, "/channel-selection-decisions?view=tied")
	if failing.Code != http.StatusInternalServerError {
		t.Fatalf("读口失败 status = %d, want 500", failing.Code)
	}
	if got := problemCode(t, failing); got != "NO_ANSWER_FORMED" {
		t.Fatalf("读口失败 code = %q", got)
	}
	assertNoOutcomeField(t, failing)

	failingDetail := getChannelSelectionDecisions(t, channelSelectionIntake(t),
		&channelSelectionReaderDouble{err: errors.New("库连不上")}, http.MethodGet, "/channel-selection-decisions?decisionId=CSDN-1")
	if failingDetail.Code != http.StatusInternalServerError || problemCode(t, failingDetail) != "NO_ANSWER_FORMED" {
		t.Fatalf("单份读口失败应答 5xx NO_ANSWER_FORMED 而不是伪装成不可见：%d %s", failingDetail.Code, failingDetail.Body)
	}

	posted := getChannelSelectionDecisions(t, channelSelectionIntake(t),
		&channelSelectionReaderDouble{}, http.MethodPost, "/channel-selection-decisions?view=tied")
	if posted.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", posted.Code)
	}
	if got := posted.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want GET；本端点只有读", got)
	}
}

// Covers: ADR-0078 Decision 二——隔离读放行只交出注入作用域的租户维与页大小，不读请求里的任何授权输入；
// 冒充头与查询参数都不改变答案。
func TestIsolatedReadIntakeHandsOutOnlyTheInjectedTenantForChannelSelectionDecisions(t *testing.T) {
	intake, err := shipmenthttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-1", "SYN-TENANT-01", []string{"SYN-ACCOUNT-01"}, 25)
	if err != nil {
		t.Fatalf("构造隔离读 Intake：%v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/channel-selection-decisions?view=tied&tenant=TENANT-9", nil)
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	request.Header.Set("Authorization", "Bearer whatever")

	query, err := intake.IntakeChannelSelectionDecisionQuery(t.Context(), request)
	if err != nil {
		t.Fatalf("隔离读 Intake：%v", err)
	}
	if query.Tenant.String() != "SYN-TENANT-01" || query.Limit != 25 {
		t.Fatalf("应只交出注入的租户与页大小：%+v", query)
	}
}
