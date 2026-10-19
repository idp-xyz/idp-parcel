package nodeopshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

// 本文件证人工重核命令口（ADR-0199 决定三第二个触发；归「运营决定」能力面，ADR-0151 决定一）：按 ADR-0022 把重核结果映射成
// 响应——追加了一版答 201，其余业务结果答 200 带 outcome；指名册上没有的版本答 404；没形成答案答 5xx；租户只从信封来，载荷
// 封闭。编排是真的重核编排，依赖是内存替身。夹具全为合成（S 级）。

const recheckTenant = "SYN-TENANT-NRS03"

var recheckReceivedAt = time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)

// recheckStore 是收寄库与待识别查找的内存替身：按键、按判断版本、按号码三问，前版链照库面走。
type recheckStore struct {
	byKey map[ports.ReceptionKey]ports.ReceptionRecord
}

func (store *recheckStore) FindByKey(_ context.Context, key ports.ReceptionKey) (ports.ReceptionRecord, bool, error) {
	record, found := store.byKey[key]
	return record, found, nil
}

func (store *recheckStore) Save(_ context.Context, record ports.ReceptionRecord) (ports.ReceptionSaveOutcome, error) {
	if _, exists := store.byKey[record.Key]; exists {
		return ports.ReceptionAlreadyRecorded, nil
	}
	store.byKey[record.Key] = record
	return ports.ReceptionSaved, nil
}

func (store *recheckStore) FindOtherUnitIntakes(
	_ context.Context, _ domain.TenantID, parcel domain.ParcelAssociationReference, unit domain.HandlingUnitID,
) ([]ports.ReceptionRecord, error) {
	var found []ports.ReceptionRecord
	for _, record := range store.byKey {
		if association, present := record.Intake.Association(); present && association == parcel && record.Intake.Unit() != unit {
			found = append(found, record)
		}
	}
	return found, nil
}

func (store *recheckStore) CurrentVersion(_ context.Context, _ domain.TenantID, version domain.IntakeResultVersion) (domain.IntakeResultVersion, error) {
	current := version
	for advanced := true; advanced; {
		advanced = false
		for _, record := range store.byKey {
			if record.Predecessor == current {
				current, advanced = record.JudgmentVersion, true
				break
			}
		}
	}
	return current, nil
}

func (store *recheckStore) FindByJudgmentVersion(_ context.Context, _ domain.TenantID, version domain.IntakeResultVersion) (ports.ReceptionRecord, bool, error) {
	for _, record := range store.byKey {
		if record.JudgmentVersion == version {
			return record, true, nil
		}
	}
	return ports.ReceptionRecord{}, false, nil
}

func (store *recheckStore) FindRecheckableByMark(context.Context, domain.TenantID, string) ([]ports.ReceptionRecord, error) {
	return nil, errors.New("命令口只按判断版本指名")
}

type recheckIdentity struct {
	candidates []string
	// ambiguous 是 PS 随候选答出的歧义信号（ADR-0195）。
	ambiguous bool
	// customerNumbered 为真时候选只经客户包裹件号命中，没带范围就不范围相容；否则照渠道包裹件号答（ADR-0212 决定四）。
	customerNumbered bool
	err              error
	asked            int
}

func (double *recheckIdentity) ResolveParcelIdentity(
	_ context.Context, _ domain.TenantID, _ ports.ExternalMarkObservation, scope []domain.CustomerAccountReference,
) (ports.ParcelIdentityAnswer, error) {
	double.asked++
	answer := ports.ParcelIdentityAnswer{Ambiguous: double.ambiguous, Scoped: len(scope) > 0}
	for _, raw := range double.candidates {
		candidate, err := domain.NewParcelAssociationReference(raw)
		if err != nil {
			return ports.ParcelIdentityAnswer{}, err
		}
		answer.Candidates = append(answer.Candidates, ports.ParcelIdentityCandidate{Parcel: candidate, ScopeCompatible: !double.customerNumbered})
	}
	return answer, double.err
}

type recheckVersions struct{ next int }

func (double *recheckVersions) NextIntakeResultVersion(context.Context) (domain.IntakeResultVersion, error) {
	double.next++
	return domain.NewIntakeResultVersion("SYN-INTAKEV-NRS03-RECHECK-" + string(rune('0'+double.next)))
}

type recheckFixture struct {
	store    *recheckStore
	identity *recheckIdentity
	intents  *downstreamDouble
	handler  *application.RecheckPendingIdentityHandler
	pending  ports.ReceptionRecord
}

// newRecheckEndpointFixture 册上落一件零候选的待识别（带来源文档）、一行来源未保全的旧待识别、一行记身份冲突的待识别。
func newRecheckEndpointFixture(t *testing.T) *recheckFixture {
	t.Helper()
	tenant := adapterValue(t, domain.NewTenantID, recheckTenant)
	pendingOf := func(sourceID, version, unit string) ports.ReceptionRecord {
		intake, err := domain.FormNodeIntake(domain.NodeIntakeSpec{
			TenantID:    tenant,
			Unit:        adapterValue(t, domain.NewHandlingUnitID, unit),
			Node:        adapterValue(t, domain.NewNodeReference, "SYN-NODE-NRS03"),
			DeliveredBy: adapterValue(t, domain.NewDeliveringPartyReference, "SYN-COURIER-NRS03"),
			Evidence:    adapterValue(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-NRS03"),
			Version:     adapterValue(t, domain.NewIntakeResultVersion, version),
			ReceivedAt:  recheckReceivedAt,
		})
		if err != nil {
			t.Fatalf("待识别收寄：%v", err)
		}
		control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
			TenantID: tenant, Unit: intake.Unit(), Node: intake.Node(), Kind: domain.EstablishedByNodeIntake,
			Basis:         adapterValue(t, domain.NewControlBasisReference, "NODE-INTAKE/"+version),
			EstablishedAt: recheckReceivedAt,
		})
		if err != nil {
			t.Fatalf("控制：%v", err)
		}
		return ports.ReceptionRecord{
			Key:             ports.ReceptionKey{TenantID: tenant, SourceID: sourceID},
			ContentDigest:   "NOC-1:syn",
			Kind:            ports.RecordPendingIdentification,
			JudgmentVersion: intake.Version(),
			Source: domain.DeliverySource{
				Unit: intake.Unit(), Mark: "SYN-MARK-NRS03", Claim: "RECEIVED", Evidence: intake.Evidence(),
				DeliveredBy: intake.DeliveredBy(), Node: intake.Node(), OccurredAt: recheckReceivedAt,
			},
			Intake:     intake,
			Control:    control,
			RecordedAt: recheckReceivedAt,
		}
	}
	pending := pendingOf("SYN-DELIVERY-NRS03", "SYN-INTAKEV-NRS03-PENDING", "SYN-UNIT-NRS03")
	sourceless := pendingOf("SYN-DELIVERY-NRS03-OLD", "SYN-INTAKEV-NRS03-OLD", "SYN-UNIT-NRS03-OLD")
	sourceless.Source = domain.DeliverySource{}
	conflicted := pendingOf("SYN-DELIVERY-NRS03-CONFLICT", "SYN-INTAKEV-NRS03-CONFLICT", "SYN-UNIT-NRS03-CONFLICT")
	conflicted.IdentityConflict = true

	store := &recheckStore{byKey: map[ports.ReceptionKey]ports.ReceptionRecord{}}
	for _, record := range []ports.ReceptionRecord{pending, sourceless, conflicted} {
		store.byKey[record.Key] = record
	}
	identity := &recheckIdentity{}
	intents := &downstreamDouble{}
	handler, err := application.NewRecheckPendingIdentityHandler(application.RecheckPendingIdentityDeps{
		Identity:       identity,
		ServiceResults: &serviceResultsDouble{},
		Receptions:     store,
		Pending:        store,
		Versions:       &recheckVersions{},
		Downstream:     intents,
		Scope:          noScopeEvidence{},
		Clock:          adapterClock{at: recheckReceivedAt.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("重核编排：%v", err)
	}
	return &recheckFixture{store: store, identity: identity, intents: intents, handler: handler, pending: pending}
}

// namedIntake 是只从请求体取判断版本、租户写死的 Intake 替身：命令口的映射在这里测，认证与载荷在 OperatorDecisionIntake 那几条测。
type namedIntake struct{ err error }

func (intake namedIntake) IntakeIdentityRecheck(_ context.Context, request *http.Request) (application.RecheckPendingIdentityCommand, error) {
	if intake.err != nil {
		return application.RecheckPendingIdentityCommand{}, intake.err
	}
	var body struct {
		JudgmentVersion string `json:"judgmentVersion"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.RecheckPendingIdentityCommand{}, err
	}
	version, err := domain.NewIntakeResultVersion(body.JudgmentVersion)
	if err != nil {
		return application.RecheckPendingIdentityCommand{}, err
	}
	tenant, err := domain.NewTenantID(recheckTenant)
	return application.RecheckPendingIdentityCommand{TenantID: tenant, Version: version}, err
}

func postRecheck(endpoint http.Handler, version string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/node-operations/identity-rechecks",
		strings.NewReader(`{"judgmentVersion":"`+version+`"}`))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

type recheckView struct {
	Outcome            string   `json:"outcome"`
	RecheckedVersion   string   `json:"recheckedVersion"`
	JudgmentVersion    string   `json:"judgmentVersion"`
	CurrentVersion     string   `json:"currentVersion"`
	RecordedOutcome    string   `json:"recordedOutcome"`
	Unit               string   `json:"unit"`
	Association        string   `json:"association"`
	Candidates         []string `json:"candidates"`
	ServiceResultCheck string   `json:"serviceResultCheck"`
}

func decodeRecheck(t *testing.T, response *httptest.ResponseRecorder) recheckView {
	t.Helper()
	var view recheckView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, response.Body.String())
	}
	return view
}

// Covers: 人工重核追加了`节点收寄已形成`答 201：outcome 取编排结果原名，带追加那一版的判断版本、关联与核对态，以及被重核的
// 那一版；同一件再重核答 200 `EXISTING_RESULT`，带当前有效版本与它的判断种类，不再追加。
func TestAManualRecheckThatFormsAnIntakeAnswersCreated(t *testing.T) {
	fixture := newRecheckEndpointFixture(t)
	fixture.identity.candidates = []string{"SYN-PARCEL-NRS03"}
	endpoint := nodeopshttp.NewRecheckPendingIdentityEndpoint(namedIntake{}, fixture.handler)

	first := postRecheck(endpoint, "SYN-INTAKEV-NRS03-PENDING")
	if first.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（%s）", first.Code, first.Body.String())
	}
	formed := decodeRecheck(t, first)
	if formed.Outcome != "INTAKE_FORMED" || formed.RecheckedVersion != "SYN-INTAKEV-NRS03-PENDING" ||
		formed.JudgmentVersion == "" || formed.Association != "SYN-PARCEL-NRS03" || formed.ServiceResultCheck != "CHECKED" ||
		formed.Unit != "SYN-UNIT-NRS03" {
		t.Fatalf("响应 = %+v", formed)
	}

	again := postRecheck(endpoint, "SYN-INTAKEV-NRS03-PENDING")
	if again.Code != http.StatusOK {
		t.Fatalf("再重核 status = %d, want 200", again.Code)
	}
	existing := decodeRecheck(t, again)
	if existing.Outcome != "EXISTING_RESULT" || existing.CurrentVersion != formed.JudgmentVersion || existing.RecordedOutcome != "INTAKE_FORMED" {
		t.Fatalf("再重核响应 = %+v, want EXISTING_RESULT 指向 %s", existing, formed.JudgmentVersion)
	}
	if len(fixture.intents.intents) != 1 {
		t.Fatalf("意图 %d 封, want 1", len(fixture.intents.intents))
	}
}

// Covers: 不追加的几格答 200 带 outcome（ADR-0022：形成了答案就是 2xx）：零候选判断不变；记身份冲突的待识别与来源未保全的旧行
// 不重核，各答各的业务结果——服务端知道为什么不重核，不是调用方的错，也不是没形成答案。指名册上没有的版本答 404，不带 outcome。
func TestAManualRecheckAnswersEveryUnappendedCellAsAFormedAnswer(t *testing.T) {
	fixture := newRecheckEndpointFixture(t)
	endpoint := nodeopshttp.NewRecheckPendingIdentityEndpoint(namedIntake{}, fixture.handler)
	for version, want := range map[string]string{
		"SYN-INTAKEV-NRS03-PENDING":  "STILL_UNIDENTIFIED",
		"SYN-INTAKEV-NRS03-CONFLICT": "IDENTITY_CONFLICT_RECORDED",
		"SYN-INTAKEV-NRS03-OLD":      "SOURCE_NOT_PRESERVED",
	} {
		response := postRecheck(endpoint, version)
		if response.Code != http.StatusOK {
			t.Fatalf("%s：status = %d, want 200", version, response.Code)
		}
		if got := decodeRecheck(t, response); got.Outcome != want || got.RecheckedVersion != version {
			t.Fatalf("%s：响应 = %+v, want outcome %s", version, got, want)
		}
	}
	missing := postRecheck(endpoint, "SYN-INTAKEV-NRS03-ABSENT")
	if missing.Code != http.StatusNotFound || problemCode(t, missing) != "JUDGMENT_NOT_VISIBLE" {
		t.Fatalf("指名册上没有的版本 = %d %q, want 404 JUDGMENT_NOT_VISIBLE", missing.Code, problemCode(t, missing))
	}
	assertNoOutcome(t, missing)
}

// Covers: 多候选追加冲突版答 201 `IDENTITY_CONFLICT`，候选照列；身份核对读不动答 500 NO_ANSWER_FORMED、不带 outcome、不追加。
func TestAManualRecheckMapsConflictAndUnreadableIdentity(t *testing.T) {
	fixture := newRecheckEndpointFixture(t)
	fixture.identity.candidates = []string{"SYN-PARCEL-NRS03-A", "SYN-PARCEL-NRS03-B"}
	endpoint := nodeopshttp.NewRecheckPendingIdentityEndpoint(namedIntake{}, fixture.handler)
	conflict := postRecheck(endpoint, "SYN-INTAKEV-NRS03-PENDING")
	if conflict.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", conflict.Code)
	}
	if got := decodeRecheck(t, conflict); got.Outcome != "IDENTITY_CONFLICT" || len(got.Candidates) != 2 || got.Association != "" {
		t.Fatalf("响应 = %+v", got)
	}

	unreadable := newRecheckEndpointFixture(t)
	unreadable.identity.err = errors.New("parcel shipment unreachable")
	rows := len(unreadable.store.byKey)
	response := postRecheck(nodeopshttp.NewRecheckPendingIdentityEndpoint(namedIntake{}, unreadable.handler), "SYN-INTAKEV-NRS03-PENDING")
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("读不动 = %d %q, want 500 NO_ANSWER_FORMED", response.Code, problemCode(t, response))
	}
	assertNoOutcome(t, response)
	if len(unreadable.store.byKey) != rows {
		t.Fatal("读不动的重核落了册")
	}
}

// Covers: 命令口的传输层各格——只收 POST；Intake 的接入格逐格映射（未配置 403、令牌不过 401、无授予 403、不在准入范围 403、
// 身份依赖读不动 503、载荷坏 400），都不带 outcome、不进编排。
func TestTheRecheckEndpointMapsIntakeRefusals(t *testing.T) {
	fixture := newRecheckEndpointFixture(t)
	get := httptest.NewRecorder()
	nodeopshttp.NewRecheckPendingIdentityEndpoint(namedIntake{}, fixture.handler).
		ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/node-operations/identity-rechecks", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", get.Code)
	}
	cases := map[string]struct {
		err    error
		status int
		code   string
	}{
		"未配置":    {nodeopshttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"令牌不过":   {nodeopshttp.ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"无授予":    {nodeopshttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"不在准入范围": {nodeopshttp.ErrOutsideAdmissionScope, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		"身份依赖不动": {nodeopshttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"载荷坏":    {nodeopshttp.ErrMalformedRequest, http.StatusBadRequest, "MALFORMED_REQUEST"},
	}
	for name, testCase := range cases {
		response := postRecheck(nodeopshttp.NewRecheckPendingIdentityEndpoint(namedIntake{err: testCase.err}, fixture.handler), "SYN-INTAKEV-NRS03-PENDING")
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s：答 %d %q, want %d %q", name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
		assertNoOutcome(t, response)
	}
	if fixture.identity.asked != 0 || len(fixture.intents.intents) != 0 {
		t.Fatal("被 Intake 拒的请求进了编排")
	}
}

// Covers: 未配置 Intake 在命令口同样不读请求、答 403 未配置。
func TestUnconfiguredRecheckRefusesWithoutReadingTheBody(t *testing.T) {
	fixture := newRecheckEndpointFixture(t)
	probe := &readProbe{}
	response := httptest.NewRecorder()
	nodeopshttp.NewRecheckPendingIdentityEndpoint(nodeopshttp.UnconfiguredIntake{}, fixture.handler).
		ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/node-operations/identity-rechecks", probe))
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" || probe.read {
		t.Fatalf("未配置答 %d %q（读了请求 %v）", response.Code, problemCode(t, response), probe.read)
	}
}

// recheckAuthenticatorFake 记下被问的决定种类；没带令牌答令牌不过，否则答 err 或租户。
type recheckAuthenticatorFake struct {
	err   error
	asked *[]nodeopshttp.OperatorDecision
}

func (fake recheckAuthenticatorFake) AuthenticateOperatorDecision(_ context.Context, token string, decision nodeopshttp.OperatorDecision) (nodeopshttp.OperatorIdentity, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, decision)
	}
	if token == "" {
		return nodeopshttp.OperatorIdentity{}, nodeopshttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return nodeopshttp.OperatorIdentity{}, fake.err
	}
	tenant, err := domain.NewTenantID(recheckTenant)
	return nodeopshttp.OperatorIdentity{Tenant: tenant, Operator: "https://id.syn.example/dex#SYN-OPERATOR-NRS03"}, err
}

type bodyProbe struct {
	reader io.Reader
	read   bool
}

func (probe *bodyProbe) Read(buffer []byte) (int, error) {
	probe.read = true
	return probe.reader.Read(buffer)
}

// Covers: 操作者渠道「运营决定」能力面在本口的 Intake（ADR-0151 决定一、二）：按`待识别重核`这一种决定认证，租户只取认证结果；
// 先认证、后解载荷——授予不过的调用方看不到载荷校验；载荷封闭，自报租户、缺判断版本都答坏报文。
func TestTheOperatorDecisionIntakeNamesOnePendingUnitForTheEnvelopeTenant(t *testing.T) {
	var asked []nodeopshttp.OperatorDecision
	intake, err := nodeopshttp.NewOperatorDecisionIntake(recheckAuthenticatorFake{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, body string) *http.Request {
		built := httptest.NewRequest(http.MethodPost, "/node-operations/identity-rechecks", strings.NewReader(body))
		if token != "" {
			built.Header.Set("Authorization", "Bearer "+token)
		}
		return built
	}
	command, err := intake.IntakeIdentityRecheck(context.Background(), request("presented.operator.token", `{"judgmentVersion":"SYN-INTAKEV-NRS03-PENDING"}`))
	if err != nil {
		t.Fatalf("已认证的操作者：%v", err)
	}
	if command.TenantID.String() != recheckTenant || command.Version.String() != "SYN-INTAKEV-NRS03-PENDING" {
		t.Fatalf("命令 = %+v", command)
	}
	if len(asked) != 1 || asked[0] != nodeopshttp.OperatorDecisionPendingIdentityRecheck || string(asked[0]) != "PENDING_IDENTITY_RECHECK" {
		t.Fatalf("问认证方的决定种类 = %v", asked)
	}
	for name, body := range map[string]string{
		"自报租户":    `{"judgmentVersion":"SYN-INTAKEV-NRS03-PENDING","tenantId":"SYN-TENANT-02"}`,
		"缺判断版本":   `{}`,
		"不是 JSON": `!!`,
	} {
		if _, err := intake.IntakeIdentityRecheck(context.Background(), request("presented.operator.token", body)); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
	refusing, err := nodeopshttp.NewOperatorDecisionIntake(recheckAuthenticatorFake{err: nodeopshttp.ErrOperatorNotGranted})
	if err != nil {
		t.Fatal(err)
	}
	probe := &bodyProbe{reader: strings.NewReader(`!!`)}
	refused := httptest.NewRequest(http.MethodPost, "/node-operations/identity-rechecks", probe)
	refused.Header.Set("Authorization", "Bearer presented.operator.token")
	if _, err := refusing.IntakeIdentityRecheck(context.Background(), refused); !errors.Is(err, nodeopshttp.ErrOperatorNotGranted) || probe.read {
		t.Fatalf("无授予：err = %v、读了载荷 %v, want 先答无授予、不读载荷", err, probe.read)
	}
	if _, err := nodeopshttp.NewOperatorDecisionIntake(nil); err == nil {
		t.Fatal("缺认证方却构造成功")
	}
}
