package nodeopshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证身份冲突处置命令口（UC-NO-004；ADR-0210 决定六）：按 ADR-0022 把处置结果映射成响应——首次形成答 201，其余业务结果
// 答 200 带 outcome；指名的实物或版本查不到答 404，与越权探针同答；租户与提交操作者只从信封来，载荷封闭，坏载荷各种答 400。编排
// 是真的处置编排，依赖是内存替身。夹具全为合成（S 级）。

const dispositionTenant = "SYN-TENANT-NRI06"

var dispositionReceivedAt = time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)

// FindUnitIntakes 照真库在内存里找：这件实物带收寄的各行，按来源标识定序。
func (store *recheckStore) FindUnitIntakes(_ context.Context, _ domain.TenantID, unit domain.HandlingUnitID) ([]ports.ReceptionRecord, error) {
	found := []ports.ReceptionRecord{}
	for _, record := range store.byKey {
		if !record.Intake.ReceivedAt().IsZero() && record.Intake.Unit() == unit {
			found = append(found, record)
		}
	}
	slices.SortFunc(found, func(left, right ports.ReceptionRecord) int {
		return strings.Compare(left.Key.SourceID, right.Key.SourceID)
	})
	return found, nil
}

type dispositionEndpointFixture struct {
	store    *recheckStore
	intents  *downstreamDouble
	handler  *application.DisposeIdentityConflictHandler
	conflict ports.ReceptionRecord
	late     ports.ReceptionRecord
}

// newDispositionEndpointFixture 册上落一码两物：A 原判断关联 X、A 冲突版接续它，B 记身份冲突的待识别。
func newDispositionEndpointFixture(t *testing.T) *dispositionEndpointFixture {
	t.Helper()
	tenant := adapterValue(t, domain.NewTenantID, dispositionTenant)
	versionOf := func(sourceID, unit, version, predecessor, association string, candidates []string, conflict bool) ports.ReceptionRecord {
		spec := domain.NodeIntakeSpec{
			TenantID:    tenant,
			Unit:        adapterValue(t, domain.NewHandlingUnitID, unit),
			Node:        adapterValue(t, domain.NewNodeReference, "SYN-NODE-NRI06"),
			DeliveredBy: adapterValue(t, domain.NewDeliveringPartyReference, "SYN-COURIER-NRI06"),
			Evidence:    adapterValue(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-NRI06"),
			Version:     adapterValue(t, domain.NewIntakeResultVersion, version),
			ReceivedAt:  dispositionReceivedAt,
		}
		kind := ports.RecordPendingIdentification
		if association != "" {
			spec.Association = adapterValue(t, domain.NewParcelAssociationReference, association)
			kind = ports.RecordIntakeFormed
		}
		intake, err := domain.FormNodeIntake(spec)
		if err != nil {
			t.Fatalf("收寄：%v", err)
		}
		control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
			TenantID: tenant, Unit: intake.Unit(), Node: intake.Node(), Kind: domain.EstablishedByNodeIntake,
			Basis: adapterValue(t, domain.NewControlBasisReference, "NODE-INTAKE/"+unit), EstablishedAt: dispositionReceivedAt,
		})
		if err != nil {
			t.Fatalf("控制：%v", err)
		}
		record := ports.ReceptionRecord{
			Key: ports.ReceptionKey{TenantID: tenant, SourceID: sourceID}, ContentDigest: "NOC-1:syn", Kind: kind,
			JudgmentVersion: intake.Version(), Intake: intake, Control: control, IdentityConflict: conflict, RecordedAt: dispositionReceivedAt,
			Source: domain.DeliverySource{Unit: intake.Unit(), Mark: "SYN-MARK-NRI06", Claim: "RECEIVED", Evidence: intake.Evidence(),
				DeliveredBy: intake.DeliveredBy(), Node: intake.Node(), OccurredAt: dispositionReceivedAt},
		}
		if predecessor != "" {
			record.Predecessor = adapterValue(t, domain.NewIntakeResultVersion, predecessor)
		}
		for _, candidate := range candidates {
			record.Candidates = append(record.Candidates, adapterValue(t, domain.NewParcelAssociationReference, candidate))
		}
		if kind == ports.RecordIntakeFormed {
			record.ServiceResultCheck = ports.ServiceResultsChecked
		}
		return record
	}
	early := versionOf("SYN-DELIVERY-NRI06-A", "SYN-UNIT-NRI06-A", "SYN-V-A1", "", "SYN-PARCEL-NRI06-X", nil, false)
	conflict := versionOf("IDENTITY-CONFLICT/SYN-DELIVERY-NRI06-B/SYN-UNIT-NRI06-A", "SYN-UNIT-NRI06-A", "SYN-V-A2", "SYN-V-A1", "", []string{"SYN-PARCEL-NRI06-X"}, true)
	late := versionOf("SYN-DELIVERY-NRI06-B", "SYN-UNIT-NRI06-B", "SYN-V-B1", "", "", []string{"SYN-PARCEL-NRI06-X"}, true)
	solo := versionOf("SYN-DELIVERY-NRI06-SOLO", "SYN-UNIT-NRI06-SOLO", "SYN-V-S1", "", "SYN-PARCEL-NRI06-S", nil, false)
	store := &recheckStore{byKey: map[ports.ReceptionKey]ports.ReceptionRecord{}}
	for _, record := range []ports.ReceptionRecord{early, conflict, late, solo} {
		store.byKey[record.Key] = record
	}
	intents := &downstreamDouble{}
	handler, err := application.NewDisposeIdentityConflictHandler(application.DisposeIdentityConflictDeps{
		ServiceResults: &serviceResultsDouble{},
		Receptions:     store,
		Register:       store,
		Versions:       &recheckVersions{},
		Downstream:     intents,
		Clock:          adapterClock{at: dispositionReceivedAt.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("处置编排：%v", err)
	}
	return &dispositionEndpointFixture{store: store, intents: intents, handler: handler, conflict: conflict, late: late}
}

// dispositionBodyIntake 是只从请求体取载荷、租户与操作者写死的 Intake 替身：命令口的映射在这里测，认证与载荷在
// OperatorDecisionIntake 那几条测。
type dispositionBodyIntake struct{ err error }

func (intake dispositionBodyIntake) IntakeIdentityDisposition(_ context.Context, request *http.Request) (application.DisposeIdentityConflictCommand, error) {
	if intake.err != nil {
		return application.DisposeIdentityConflictCommand{}, intake.err
	}
	var body struct {
		Unit, DisposedVersion, Result, Parcel, Basis string
		CoDisposedUnits                              []struct{ Unit, Version string }
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.DisposeIdentityConflictCommand{}, err
	}
	result, err := domain.ParseIdentityDispositionResult(body.Result)
	if err != nil {
		return application.DisposeIdentityConflictCommand{}, err
	}
	command := application.DisposeIdentityConflictCommand{
		Operator: "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-NRI06",
		Result:   result,
		Basis:    body.Basis,
	}
	command.TenantID, _ = domain.NewTenantID(dispositionTenant)
	command.Unit, _ = domain.NewHandlingUnitID(body.Unit)
	command.Disposed, _ = domain.NewIntakeResultVersion(body.DisposedVersion)
	if body.Parcel != "" {
		command.Parcel, _ = domain.NewParcelAssociationReference(body.Parcel)
	}
	for _, co := range body.CoDisposedUnits {
		unit, _ := domain.NewHandlingUnitID(co.Unit)
		version, _ := domain.NewIntakeResultVersion(co.Version)
		command.CoDisposed = append(command.CoDisposed, domain.CoDisposedUnit{Unit: unit, Version: version})
	}
	return command, nil
}

func postDisposition(endpoint http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/node-operations/identity-dispositions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

type dispositionView struct {
	Outcome         string `json:"outcome"`
	Unit            string `json:"unit"`
	DisposedVersion string `json:"disposedVersion"`
	Disposition     *struct {
		DisposedVersion string   `json:"disposedVersion"`
		Unit            string   `json:"unit"`
		Result          string   `json:"result"`
		Parcel          string   `json:"parcel"`
		Excluded        []string `json:"excluded"`
		CoDisposedUnits []struct {
			Unit    string `json:"unit"`
			Version string `json:"version"`
		} `json:"coDisposedUnits"`
		Basis      string `json:"basis"`
		Operator   string `json:"operator"`
		DisposedAt string `json:"disposedAt"`
	} `json:"disposition"`
	Versions []struct {
		Unit            string   `json:"unit"`
		JudgmentVersion string   `json:"judgmentVersion"`
		Predecessor     string   `json:"predecessor"`
		Kind            string   `json:"kind"`
		Association     string   `json:"association"`
		Excluded        []string `json:"excluded"`
	} `json:"versions"`
	CurrentVersions []struct {
		JudgmentVersion string `json:"judgmentVersion"`
	} `json:"currentVersions"`
	Candidates      []string `json:"candidates"`
	AssociatedUnits []struct {
		Unit            string `json:"unit"`
		CurrentVersions []struct {
			JudgmentVersion  string   `json:"judgmentVersion"`
			Candidates       []string `json:"candidates"`
			IdentityConflict bool     `json:"identityConflict"`
		} `json:"currentVersions"`
	} `json:"associatedUnits"`
	IntentVersion string `json:"intentVersion"`
}

func decodeDisposition(t *testing.T, response *httptest.ResponseRecorder) dispositionView {
	t.Helper()
	var view dispositionView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, response.Body.String())
	}
	return view
}

const (
	designateLate     = `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"DESIGNATED","parcel":"SYN-PARCEL-NRI06-X","coDisposedUnits":[{"unit":"SYN-UNIT-NRI06-A","version":"SYN-V-A2"}],"basis":"SYN-DISPOSITION-BASIS-NRI06"}`
	designateLateOnly = `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"DESIGNATED","parcel":"SYN-PARCEL-NRI06-X","basis":"SYN-DISPOSITION-BASIS-NRI06"}`
)

// Covers: `AT-NO-053` 的答复面：首次形成答 201 `IDENTITY_CONFLICT_DISPOSED`，回显这次处置（结果、指定的包裹、连带实物、依据、操作者、
// 时刻）、追加的各版（主实物关联 X、连带实物排除 X）与交了意图的那一版；原样重投答 200 `EXISTING_RESULT`、回显同一次处置，换结果
// 答 200 `ALREADY_DISPOSED`。
func TestADispositionAnswersCreatedThenTheRecordedOne(t *testing.T) {
	fixture := newDispositionEndpointFixture(t)
	endpoint := nodeopshttp.NewDisposeIdentityConflictEndpoint(dispositionBodyIntake{}, fixture.handler)

	first := postDisposition(endpoint, designateLate)
	if first.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（%s）", first.Code, first.Body.String())
	}
	formed := decodeDisposition(t, first)
	if formed.Outcome != "IDENTITY_CONFLICT_DISPOSED" || formed.Unit != "SYN-UNIT-NRI06-B" || formed.DisposedVersion != "SYN-V-B1" ||
		formed.Disposition == nil || formed.Disposition.Result != "DESIGNATED" || formed.Disposition.Parcel != "SYN-PARCEL-NRI06-X" ||
		len(formed.Disposition.CoDisposedUnits) != 1 || formed.Disposition.CoDisposedUnits[0].Version != "SYN-V-A2" ||
		formed.Disposition.Basis != "SYN-DISPOSITION-BASIS-NRI06" || formed.Disposition.Operator == "" || formed.Disposition.DisposedAt == "" {
		t.Fatalf("首次形成的答复 = %s", first.Body.String())
	}
	if len(formed.Versions) != 2 || formed.IntentVersion == "" {
		t.Fatalf("追加的版本 %d、意图那一版 %q", len(formed.Versions), formed.IntentVersion)
	}
	for _, version := range formed.Versions {
		switch version.Unit {
		case "SYN-UNIT-NRI06-B":
			if version.Kind != "INTAKE_FORMED" || version.Association != "SYN-PARCEL-NRI06-X" || version.Predecessor != "SYN-V-B1" || version.JudgmentVersion != formed.IntentVersion {
				t.Fatalf("B 那一版 = %+v", version)
			}
		case "SYN-UNIT-NRI06-A":
			if version.Kind != "PENDING_IDENTIFICATION" || version.Association != "" || !slices.Equal(version.Excluded, []string{"SYN-PARCEL-NRI06-X"}) || version.Predecessor != "SYN-V-A2" {
				t.Fatalf("A 那一版 = %+v", version)
			}
		default:
			t.Fatalf("多出一版 %+v", version)
		}
	}

	again := postDisposition(endpoint, designateLate)
	if replay := decodeDisposition(t, again); again.Code != http.StatusOK || replay.Outcome != "EXISTING_RESULT" ||
		replay.Disposition == nil || replay.Disposition.DisposedAt != formed.Disposition.DisposedAt || len(replay.Versions) != 1 {
		t.Fatalf("原样重投 = %d %s", again.Code, again.Body.String())
	}
	other := postDisposition(endpoint, `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-DISPOSITION-BASIS-NRI06"}`)
	if already := decodeDisposition(t, other); other.Code != http.StatusOK || already.Outcome != "ALREADY_DISPOSED" || already.Disposition == nil || already.Disposition.Result != "DESIGNATED" {
		t.Fatalf("换结果重投 = %d %s", other.Code, other.Body.String())
	}
	if len(fixture.intents.intents) != 1 {
		t.Fatalf("意图 %d 封, want 1", len(fixture.intents.intents))
	}
}

// Covers: 业务拒绝各格答 200 带 outcome（ADR-0022）：漏列连带实物答 `ASSOCIATED_ELSEWHERE` 并回显此刻关联到 X 的实物与它的当前有效
// 版本；候选之外答 `NOT_A_CANDIDATE` 回显处置候选；没有冲突的实物答 `NO_IDENTITY_CONFLICT`；所处置的版本已不是链头答
// `VERSION_SUPERSEDED` 回显当前有效版本。指名的实物或版本查不到答 404 `JUDGMENT_NOT_VISIBLE`，不带 outcome。都不落版。
func TestDispositionRefusalsAreFormedAnswers(t *testing.T) {
	fixture := newDispositionEndpointFixture(t)
	endpoint := nodeopshttp.NewDisposeIdentityConflictEndpoint(dispositionBodyIntake{}, fixture.handler)
	rows := len(fixture.store.byKey)

	elsewhere := postDisposition(endpoint, designateLateOnly)
	view := decodeDisposition(t, elsewhere)
	if elsewhere.Code != http.StatusOK || view.Outcome != "ASSOCIATED_ELSEWHERE" || len(view.AssociatedUnits) != 1 ||
		view.AssociatedUnits[0].Unit != "SYN-UNIT-NRI06-A" || len(view.AssociatedUnits[0].CurrentVersions) != 1 ||
		view.AssociatedUnits[0].CurrentVersions[0].JudgmentVersion != "SYN-V-A2" || !view.AssociatedUnits[0].CurrentVersions[0].IdentityConflict {
		t.Fatalf("漏列连带实物 = %d %s", elsewhere.Code, elsewhere.Body.String())
	}
	outside := postDisposition(endpoint, `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"DESIGNATED","parcel":"SYN-PARCEL-NRI06-OUT","basis":"SYN-BASIS"}`)
	if view := decodeDisposition(t, outside); outside.Code != http.StatusOK || view.Outcome != "NOT_A_CANDIDATE" || !slices.Equal(view.Candidates, []string{"SYN-PARCEL-NRI06-X"}) {
		t.Fatalf("候选之外 = %d %s", outside.Code, outside.Body.String())
	}
	solo := postDisposition(endpoint, `{"unit":"SYN-UNIT-NRI06-SOLO","disposedVersion":"SYN-V-S1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`)
	if view := decodeDisposition(t, solo); solo.Code != http.StatusOK || view.Outcome != "NO_IDENTITY_CONFLICT" || len(view.CurrentVersions) != 1 {
		t.Fatalf("没有冲突 = %d %s", solo.Code, solo.Body.String())
	}
	stale := postDisposition(endpoint, `{"unit":"SYN-UNIT-NRI06-A","disposedVersion":"SYN-V-A1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`)
	if view := decodeDisposition(t, stale); stale.Code != http.StatusOK || view.Outcome != "VERSION_SUPERSEDED" ||
		len(view.CurrentVersions) != 1 || view.CurrentVersions[0].JudgmentVersion != "SYN-V-A2" {
		t.Fatalf("版本已换代 = %d %s", stale.Code, stale.Body.String())
	}
	for name, body := range map[string]string{
		"查无此版":     `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-ABSENT","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`,
		"实物与版本对不上": `{"unit":"SYN-UNIT-NRI06-A","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`,
	} {
		response := postDisposition(endpoint, body)
		if response.Code != http.StatusNotFound || problemCode(t, response) != "JUDGMENT_NOT_VISIBLE" {
			t.Fatalf("%s = %d %q, want 404 JUDGMENT_NOT_VISIBLE", name, response.Code, problemCode(t, response))
		}
		assertNoOutcome(t, response)
	}
	if len(fixture.store.byKey) != rows || len(fixture.intents.intents) != 0 {
		t.Fatal("被拒的处置落了版或交了意图")
	}
}

// Covers: 命令口的传输层各格——只收 POST；Intake 的接入格逐格映射（未配置 403、令牌不过 401、无授予 403、不在准入范围 403、身份
// 依赖读不动 503、载荷坏 400），都不带 outcome、不进编排；未配置 Intake 不读请求体。
func TestTheDispositionEndpointMapsIntakeRefusals(t *testing.T) {
	fixture := newDispositionEndpointFixture(t)
	get := httptest.NewRecorder()
	nodeopshttp.NewDisposeIdentityConflictEndpoint(dispositionBodyIntake{}, fixture.handler).
		ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/node-operations/identity-dispositions", nil))
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
		response := postDisposition(nodeopshttp.NewDisposeIdentityConflictEndpoint(dispositionBodyIntake{err: testCase.err}, fixture.handler), designateLate)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s：答 %d %q, want %d %q", name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
		assertNoOutcome(t, response)
	}
	probe := &readProbe{}
	unconfigured := httptest.NewRecorder()
	nodeopshttp.NewDisposeIdentityConflictEndpoint(nodeopshttp.UnconfiguredIntake{}, fixture.handler).
		ServeHTTP(unconfigured, httptest.NewRequest(http.MethodPost, "/node-operations/identity-dispositions", probe))
	if unconfigured.Code != http.StatusForbidden || problemCode(t, unconfigured) != "ACCESS_CHANNEL_NOT_CONFIGURED" || probe.read {
		t.Fatalf("未配置答 %d %q（读了请求 %v）", unconfigured.Code, problemCode(t, unconfigured), probe.read)
	}
	if len(fixture.intents.intents) != 0 {
		t.Fatal("被 Intake 拒的请求进了编排")
	}
}

// dispositionAuthenticatorFake 记下被问的决定种类；没带令牌答令牌不过，否则答 err 或信封上的租户与操作者。
type dispositionAuthenticatorFake struct {
	err   error
	asked *[]nodeopshttp.OperatorDecision
}

func (fake dispositionAuthenticatorFake) AuthenticateOperatorDecision(_ context.Context, token string, decision nodeopshttp.OperatorDecision) (nodeopshttp.OperatorIdentity, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, decision)
	}
	if token == "" {
		return nodeopshttp.OperatorIdentity{}, nodeopshttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return nodeopshttp.OperatorIdentity{}, fake.err
	}
	tenant, err := domain.NewTenantID(dispositionTenant)
	return nodeopshttp.OperatorIdentity{Tenant: tenant, Operator: "https://id.syn.example/dex#SYN-OPERATOR-NRI06"}, err
}

func dispositionRequest(token, body string) *http.Request {
	built := httptest.NewRequest(http.MethodPost, "/node-operations/identity-dispositions", strings.NewReader(body))
	if token != "" {
		built.Header.Set("Authorization", "Bearer "+token)
	}
	return built
}

// Covers: 「运营决定」能力面在本口的 Intake（ADR-0151 决定一、二；ADR-0210 决定六）：按`身份冲突处置`这一种决定认证，租户与提交
// 操作者只取认证结果，操作者写作 `OPERATOR/<发行方>#<sub>`；依据引用去两端空白；先认证、后解载荷。
func TestTheDispositionIntakeTakesTenantAndOperatorFromTheEnvelope(t *testing.T) {
	var asked []nodeopshttp.OperatorDecision
	intake, err := nodeopshttp.NewOperatorDecisionIntake(dispositionAuthenticatorFake{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	command, err := intake.IntakeIdentityDisposition(context.Background(), dispositionRequest("presented.operator.token",
		`{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"DESIGNATED","parcel":"SYN-PARCEL-NRI06-X","coDisposedUnits":[{"unit":"SYN-UNIT-NRI06-A","version":"SYN-V-A2"}],"basis":"  SYN-BASIS-NRI06 "}`))
	if err != nil {
		t.Fatalf("已认证的操作者：%v", err)
	}
	if command.TenantID.String() != dispositionTenant || command.Operator != "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-NRI06" ||
		command.Unit.String() != "SYN-UNIT-NRI06-B" || command.Disposed.String() != "SYN-V-B1" || command.Result != domain.DispositionDesignatesParcel ||
		command.Parcel.String() != "SYN-PARCEL-NRI06-X" || len(command.CoDisposed) != 1 || command.CoDisposed[0].Version.String() != "SYN-V-A2" ||
		command.Basis != "SYN-BASIS-NRI06" {
		t.Fatalf("命令 = %+v", command)
	}
	if len(asked) != 1 || asked[0] != nodeopshttp.OperatorDecisionIdentityConflictDisposition || string(asked[0]) != "IDENTITY_CONFLICT_DISPOSITION" {
		t.Fatalf("问认证方的决定种类 = %v", asked)
	}
	none, err := intake.IntakeIdentityDisposition(context.Background(), dispositionRequest("presented.operator.token",
		`{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS-NRI06"}`))
	if err != nil || none.Result != domain.DispositionNoneOfCandidates || none.Parcel.String() != "" || len(none.CoDisposed) != 0 {
		t.Fatalf("都不是：%+v err %v", none, err)
	}

	refusing, err := nodeopshttp.NewOperatorDecisionIntake(dispositionAuthenticatorFake{err: nodeopshttp.ErrOperatorNotGranted})
	if err != nil {
		t.Fatal(err)
	}
	probe := &bodyProbe{reader: strings.NewReader(`!!`)}
	refused := httptest.NewRequest(http.MethodPost, "/node-operations/identity-dispositions", probe)
	refused.Header.Set("Authorization", "Bearer presented.operator.token")
	if _, err := refusing.IntakeIdentityDisposition(context.Background(), refused); !errors.Is(err, nodeopshttp.ErrOperatorNotGranted) || probe.read {
		t.Fatalf("无授予：err = %v、读了载荷 %v, want 先答无授予、不读载荷", err, probe.read)
	}
}

// Covers: ADR-0210 决定六坏载荷各种都答 MALFORMED_REQUEST：缺格、空白、带 NUL、未知字段（含自报租户与操作者）、甲不带 X、乙带 X 或
// 连带实物、连带实物重复、连带实物就是主实物或缺版本、结果不在词表里、不是 JSON、尾随内容。
func TestMalformedDispositionPayloadsAreRefused(t *testing.T) {
	intake, err := nodeopshttp.NewOperatorDecisionIntake(dispositionAuthenticatorFake{})
	if err != nil {
		t.Fatal(err)
	}
	const co = `"coDisposedUnits":[{"unit":"SYN-UNIT-NRI06-A","version":"SYN-V-A2"}]`
	for name, body := range map[string]string{
		"缺实物":       `{"disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`,
		"缺所处置的版本":   `{"unit":"SYN-UNIT-NRI06-B","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`,
		"缺结果":       `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","basis":"SYN-BASIS"}`,
		"缺依据":       `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES"}`,
		"实物空白":      `{"unit":"  ","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`,
		"依据空白":      `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":" \t "}`,
		"依据带 NUL":   `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN\u0000BASIS"}`,
		"版本带 NUL":   `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V\u0000B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`,
		"自报租户":      `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS","tenantId":"SYN-TENANT-02"}`,
		"自报操作者":     `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS","operator":"SYN-OPERATOR-02"}`,
		"甲不带 X":     `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"DESIGNATED","basis":"SYN-BASIS"}`,
		"乙带 X":      `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","parcel":"SYN-PARCEL-NRI06-X","basis":"SYN-BASIS"}`,
		"乙带连带实物":    `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES",` + co + `,"basis":"SYN-BASIS"}`,
		"连带实物重复":    `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"DESIGNATED","parcel":"SYN-PARCEL-NRI06-X","coDisposedUnits":[{"unit":"SYN-UNIT-NRI06-A","version":"SYN-V-A2"},{"unit":"SYN-UNIT-NRI06-A","version":"SYN-V-A1"}],"basis":"SYN-BASIS"}`,
		"连带实物是主实物":  `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"DESIGNATED","parcel":"SYN-PARCEL-NRI06-X","coDisposedUnits":[{"unit":"SYN-UNIT-NRI06-B","version":"SYN-V-B1"}],"basis":"SYN-BASIS"}`,
		"连带实物缺版本":   `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"DESIGNATED","parcel":"SYN-PARCEL-NRI06-X","coDisposedUnits":[{"unit":"SYN-UNIT-NRI06-A"}],"basis":"SYN-BASIS"}`,
		"连带实物带未知字段": `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"DESIGNATED","parcel":"SYN-PARCEL-NRI06-X","coDisposedUnits":[{"unit":"SYN-UNIT-NRI06-A","version":"SYN-V-A2","tenantId":"SYN-TENANT-02"}],"basis":"SYN-BASIS"}`,
		"结果不在词表":    `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"designated","parcel":"SYN-PARCEL-NRI06-X","basis":"SYN-BASIS"}`,
		"不是 JSON":   `!!`,
		"尾随内容":      `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"} {}`,
	} {
		if _, err := intake.IntakeIdentityDisposition(context.Background(), dispositionRequest("presented.operator.token", body)); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// reviewUnitsCatalogue 在查阅目录替身外面补上按实物上列的那一问。
type reviewUnitsCatalogue struct {
	*stubReviewCatalogue
	reviewUnits []ports.IdentityReviewUnitRow
}

func (catalogue reviewUnitsCatalogue) ListIdentityReviewUnits(_ context.Context, tenant domain.TenantID, limit int) ([]ports.IdentityReviewUnitRow, error) {
	catalogue.gotTenant, catalogue.gotLimit = tenant.String(), limit
	return catalogue.reviewUnits, nil
}

// ListIdentityReviewUnits 让既有的查阅目录替身也满足读口：它们只测别的册子，这一问交回空。
func (stub *stubReviewCatalogue) ListIdentityReviewUnits(_ context.Context, tenant domain.TenantID, limit int) ([]ports.IdentityReviewUnitRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return nil, stub.err
}

func (stub unreachableReviewCatalogue) ListIdentityReviewUnits(context.Context, domain.TenantID, int) ([]ports.IdentityReviewUnitRow, error) {
	stub.refuse("identity review unit")
	return nil, nil
}

// Covers: 票 06「做什么」第 7 项的传输面：待识别实物册的答复另带 `units`——按实物把各链当前有效版本列在一起；处置版带处置文档
// （结果、指定的包裹或排除的那一组、操作者、依据、时刻），其余版本不带 `disposition` 键。
func TestUnidentifiedItemsCarryEachUnitsChainHeadsAndDisposition(t *testing.T) {
	disposition, err := domain.FormIdentityDisposition(domain.IdentityDisposition{
		Disposed: adapterValue(t, domain.NewIntakeResultVersion, "SYN-V-B1"), Unit: adapterValue(t, domain.NewHandlingUnitID, "SYN-UNIT-NRI06-B"),
		Result: domain.DispositionNoneOfCandidates, Excluded: []domain.ParcelAssociationReference{adapterValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-X")},
		Basis: "SYN-BASIS-NRI06", Operator: "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-NRI06", DisposedAt: dispositionReceivedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	catalogue := reviewUnitsCatalogue{stubReviewCatalogue: &stubReviewCatalogue{}, reviewUnits: []ports.IdentityReviewUnitRow{{
		Unit: "SYN-UNIT-NRI06-B",
		Heads: []ports.IdentityReviewHead{
			{SourceID: "IDENTITY-DISPOSITION/SYN-V-B1", Kind: "PENDING_IDENTIFICATION", Node: "SYN-NODE-NRI06", JudgmentVersion: "SYN-V-B2",
				Candidates: []string{}, ReceivedAt: dispositionReceivedAt, RecordedAt: dispositionReceivedAt.Add(time.Hour), Disposition: disposition},
			{SourceID: "SYN-DELIVERY-NRI06-B2", Kind: "PENDING_IDENTIFICATION", Node: "SYN-NODE-NRI06", JudgmentVersion: "SYN-V-B9",
				Candidates: []string{"SYN-PARCEL-NRI06-Y"}, IdentityConflict: true, ReceivedAt: dispositionReceivedAt, RecordedAt: dispositionReceivedAt},
		},
	}}}
	response := serveGet(nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(grantedIntake(), catalogue), "/node-operations-records?registry=unidentified-item")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Items []json.RawMessage `json:"items"`
		Units []struct {
			Unit  string                       `json:"unit"`
			Heads []map[string]json.RawMessage `json:"heads"`
		} `json:"units"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Units) != 1 || len(body.Units[0].Heads) != 2 {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	disposed, other := body.Units[0].Heads[0], body.Units[0].Heads[1]
	var document struct {
		Result, Basis, Operator, DisposedAt, DisposedVersion, Unit string
		Excluded                                                   []string
	}
	if err := json.Unmarshal(disposed["disposition"], &document); err != nil || document.Result != "NONE_OF_CANDIDATES" ||
		!slices.Equal(document.Excluded, []string{"SYN-PARCEL-NRI06-X"}) || document.Basis != "SYN-BASIS-NRI06" ||
		document.Operator == "" || document.DisposedAt == "" || document.DisposedVersion != "SYN-V-B1" || document.Unit != "SYN-UNIT-NRI06-B" {
		t.Fatalf("处置版的处置文档 = %s（%v）", disposed["disposition"], err)
	}
	if _, present := other["disposition"]; present || string(other["identityConflict"]) != "true" || string(other["judgmentVersion"]) != `"SYN-V-B9"` {
		t.Fatalf("另一条链 = %v", other)
	}
}
