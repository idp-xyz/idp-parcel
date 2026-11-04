package identityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

// 本文件证操作者接入管理面的四口（ADR-0363 决定二、三）：查阅答复的形状与词表、三个写口的准入三格、坏载荷、答复代数到状态码的
// 映射、每次写各开一笔事务，以及撤销口写入前的防锁死。

var administrationAt = time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)

type adminAuthenticatorFake struct {
	administrator identityhttp.OperatorAccessAdministrator
	err           error
}

func (fake adminAuthenticatorFake) AuthenticateAccessAdministration(context.Context, string) (identityhttp.OperatorAccessAdministrator, error) {
	return fake.administrator, fake.err
}

type operatorCatalogueFake struct {
	tenant   string
	rows     []accessidentity.OperatorCatalogueRow
	grant    accessidentity.RecordedGrant
	found    bool
	err      error
	listed   int
	grantIDs []string
}

func (fake *operatorCatalogueFake) ListOperators(_ context.Context, tenantID string) ([]accessidentity.OperatorCatalogueRow, error) {
	fake.listed++
	fake.tenant = tenantID
	if fake.err != nil {
		return nil, fake.err
	}
	return fake.rows, nil
}

func (fake *operatorCatalogueFake) FindGrant(_ context.Context, tenantID, grantID string) (accessidentity.RecordedGrant, bool, error) {
	fake.tenant = tenantID
	fake.grantIDs = append(fake.grantIDs, grantID)
	return fake.grant, fake.found, fake.err
}

// transactorFake 数开了几笔事务；写口的每一次登记都要落在自己那一笔里。
type transactorFake struct {
	opened int
	err    error
}

func (fake *transactorFake) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	fake.opened++
	if fake.err != nil {
		return fake.err
	}
	return fn(ctx)
}

type registrarFake struct {
	outcome     accessidentity.OperatorRegistrationOutcome
	err         error
	bindings    []accessidentity.OperatorBinding
	grants      []accessidentity.OperatorGrant
	revocations []accessidentity.GrantRevocation
}

func (fake *registrarFake) RegisterOperator(_ context.Context, binding accessidentity.OperatorBinding) (accessidentity.OperatorRegistrationOutcome, error) {
	fake.bindings = append(fake.bindings, binding)
	return fake.outcome, fake.err
}

func (fake *registrarFake) RegisterGrant(_ context.Context, grant accessidentity.OperatorGrant) (accessidentity.OperatorRegistrationOutcome, error) {
	fake.grants = append(fake.grants, grant)
	return fake.outcome, fake.err
}

func (fake *registrarFake) RegisterRevocation(_ context.Context, revocation accessidentity.GrantRevocation) (accessidentity.OperatorRegistrationOutcome, error) {
	fake.revocations = append(fake.revocations, revocation)
	return fake.outcome, fake.err
}

func administrator(t *testing.T) identityhttp.OperatorAccessAdministrator {
	t.Helper()
	return identityhttp.OperatorAccessAdministrator{TenantID: catalogueTenant, Subject: catalogueSubject(t)}
}

func anotherSubject(t *testing.T) accessidentity.OperatorSubject {
	t.Helper()
	subject, err := accessidentity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-02")
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

func recordedGrantOf(t *testing.T, subject accessidentity.OperatorSubject, grantID string, face accessidentity.CapabilityFace, startsAt, endsAt time.Time, revokedAt *time.Time) accessidentity.RecordedGrant {
	t.Helper()
	interval, err := accessidentity.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewOperatorGrant(catalogueTenant, grantID, subject, face, interval, "SYN-BASIS-"+grantID)
	if err != nil {
		t.Fatal(err)
	}
	var revocation *accessidentity.GrantRevocation
	if revokedAt != nil {
		built, err := accessidentity.NewGrantRevocation(catalogueTenant, grantID, *revokedAt, "SYN-REVOKE-"+grantID)
		if err != nil {
			t.Fatal(err)
		}
		revocation = &built
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, revocation)
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

func grantRowOf(t *testing.T, recorded accessidentity.RecordedGrant, recordedAt time.Time) accessidentity.RecordedGrantRow {
	t.Helper()
	row, err := accessidentity.NewRecordedGrantRow(recorded, recordedAt)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func serveJSON(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func operatorsHandler(t *testing.T, authenticator identityhttp.OperatorAccessAdministrationAuthenticator, catalogue accessidentity.OperatorCatalogue, issuer string) http.Handler {
	t.Helper()
	handler, err := identityhttp.NewQueryOperatorsEndpoint(authenticator, catalogue, issuer)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// Covers: ADR-0363 决定二——查阅答复带本租户的操作者、授予与撤销记录，外加产品词表与本进程信任的发行方；可选格按能力面在场而不是 null；
// 请求里自报的租户不进答复，册只按信封上的租户列。
func TestQueryOperatorsListsTheTenantWithVocabulary(t *testing.T) {
	subject := catalogueSubject(t)
	binding, err := accessidentity.NewOperatorBinding(subject, catalogueTenant, "SYN-BASIS-BINDING")
	if err != nil {
		t.Fatal(err)
	}
	startsAt := administrationAt.Add(-48 * time.Hour)
	endsAt := administrationAt.Add(48 * time.Hour)
	revokedAt := administrationAt.Add(-time.Hour)
	plain := recordedGrantOf(t, subject, "SYN-GRANT-WRITE", accessidentity.CapabilityRegistryConfigurationWrite, startsAt, time.Time{}, nil)
	bounded := recordedGrantOf(t, subject, "SYN-GRANT-ADMIN", accessidentity.CapabilityOperatorAccessAdministration, startsAt, endsAt, &revokedAt)
	interval, err := accessidentity.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	decisionGrant, err := accessidentity.NewOperationDecisionGrant(catalogueTenant, "SYN-GRANT-DECISION", subject, accessidentity.DecisionSegmentClosure, interval, "SYN-BASIS-DECISION")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := accessidentity.NewRecordedGrant(decisionGrant, nil)
	if err != nil {
		t.Fatal(err)
	}
	workGrant, err := accessidentity.NewWorkFactRegistrationGrant(catalogueTenant, "SYN-GRANT-WF", subject, catalogueScope(t, accessidentity.WorkScopeNode, "SYN-NODE-SZX-01"), interval, "SYN-BASIS-WF")
	if err != nil {
		t.Fatal(err)
	}
	work, err := accessidentity.NewRecordedGrant(workGrant, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err := accessidentity.NewOperatorCatalogueRow(binding, startsAt, []accessidentity.RecordedGrantRow{
		grantRowOf(t, plain, startsAt), grantRowOf(t, bounded, startsAt.Add(time.Minute)),
		grantRowOf(t, decision, startsAt.Add(2*time.Minute)), grantRowOf(t, work, startsAt.Add(3*time.Minute)),
	})
	if err != nil {
		t.Fatal(err)
	}
	emptyBinding, err := accessidentity.NewOperatorBinding(anotherSubject(t), catalogueTenant, "SYN-BASIS-BINDING-2")
	if err != nil {
		t.Fatal(err)
	}
	empty, err := accessidentity.NewOperatorCatalogueRow(emptyBinding, startsAt.Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	catalogue := &operatorCatalogueFake{rows: []accessidentity.OperatorCatalogueRow{row, empty}}
	handler := operatorsHandler(t, adminAuthenticatorFake{administrator: administrator(t)}, catalogue, "https://id.syn.example/dex")

	response := serveJSON(handler, http.MethodGet, identityhttp.OperatorsPath+"?tenant=SYN-TENANT-02", "")
	if response.Code != http.StatusOK || catalogue.tenant != catalogueTenant {
		t.Fatalf("status %d tenant %s body %s", response.Code, catalogue.tenant, response.Body.String())
	}
	var listed struct {
		Outcome        string           `json:"outcome"`
		Issuer         string           `json:"issuer"`
		Faces          []string         `json:"faces"`
		DecisionKinds  []string         `json:"decisionKinds"`
		WorkScopeKinds []string         `json:"workScopeKinds"`
		Operators      []map[string]any `json:"operators"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Outcome != "OPERATORS_LISTED" || listed.Issuer != "https://id.syn.example/dex" {
		t.Fatalf("body = %s", response.Body.String())
	}
	wantFaces := []string{"REGISTRY_CONFIGURATION_WRITE", "MASTER_DATA_AND_OPERATIONS_READ", "OPERATION_DECISION", "WORK_FACT_REGISTRATION", "OPERATOR_ACCESS_ADMINISTRATION"}
	if strings.Join(listed.Faces, ",") != strings.Join(wantFaces, ",") {
		t.Fatalf("faces = %v", listed.Faces)
	}
	if len(listed.DecisionKinds) != len(accessidentity.DecisionKinds()) || listed.DecisionKinds[0] != "MANUAL_REVIEW_COMPLETION" {
		t.Fatalf("decisionKinds = %v", listed.DecisionKinds)
	}
	if strings.Join(listed.WorkScopeKinds, ",") != "NODE,OFFSITE" {
		t.Fatalf("workScopeKinds = %v", listed.WorkScopeKinds)
	}
	if len(listed.Operators) != 2 {
		t.Fatalf("operators = %v", listed.Operators)
	}
	first := listed.Operators[0]
	if first["issuer"] != "https://id.syn.example/dex" || first["subject"] != "SYN-OPERATOR-01" || first["basis"] != "SYN-BASIS-BINDING" ||
		first["recordedAt"] != startsAt.Format(time.RFC3339Nano) {
		t.Fatalf("first operator = %v", first)
	}
	grants := first["grants"].([]any)
	if len(grants) != 4 {
		t.Fatalf("grants = %v", grants)
	}
	assertDeviceKeys(t, grants[0].(map[string]any), map[string]any{
		"grantId": "SYN-GRANT-WRITE", "capabilityFace": "REGISTRY_CONFIGURATION_WRITE",
		"effectiveStartsAt": startsAt.Format(time.RFC3339Nano), "basis": "SYN-BASIS-SYN-GRANT-WRITE",
		"recordedAt": startsAt.Format(time.RFC3339Nano),
	})
	second := grants[1].(map[string]any)
	if second["effectiveEndsAt"] != endsAt.Format(time.RFC3339Nano) || second["capabilityFace"] != "OPERATOR_ACCESS_ADMINISTRATION" {
		t.Fatalf("bounded grant = %v", second)
	}
	revocation := second["revocation"].(map[string]any)
	if revocation["revokedAt"] != revokedAt.Format(time.RFC3339Nano) || revocation["basis"] != "SYN-REVOKE-SYN-GRANT-ADMIN" {
		t.Fatalf("revocation = %v", revocation)
	}
	if grants[2].(map[string]any)["decisionKind"] != "SEGMENT_CLOSURE" {
		t.Fatalf("decision grant = %v", grants[2])
	}
	scope := grants[3].(map[string]any)["workScope"].(map[string]any)
	if scope["kind"] != "NODE" || scope["reference"] != "SYN-NODE-SZX-01" {
		t.Fatalf("work scope = %v", scope)
	}
	// 没授予的操作者 grants 是空数组而不是 null，管理台按数组判「没有」。
	var raw struct {
		Operators []map[string]json.RawMessage `json:"operators"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw.Operators[1]["grants"]) != "[]" {
		t.Fatalf("empty grants = %s", raw.Operators[1]["grants"])
	}
}

// Covers: ADR-0363 决定二——空册答空数组，词表照样带：没有任何操作者的租户也要摆得出选项。
func TestQueryOperatorsEmptyCatalogueStillCarriesTheVocabulary(t *testing.T) {
	handler := operatorsHandler(t, adminAuthenticatorFake{administrator: administrator(t)}, &operatorCatalogueFake{}, "https://id.syn.example/dex")
	response := serveJSON(handler, http.MethodGet, identityhttp.OperatorsPath, "")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || string(raw["operators"]) != "[]" || string(raw["faces"]) == "[]" || string(raw["decisionKinds"]) == "[]" {
		t.Fatalf("status %d body %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0363 决定二「各走 ADR-0100 决定四的三格准入」——查阅口的拒绝各态与 /operator-grants 同一套；读册失败答答案未形成；只收 GET。
func TestQueryOperatorsAnswerGrades(t *testing.T) {
	for _, testCase := range admissionGrades() {
		t.Run(testCase.name, func(t *testing.T) {
			catalogue := &operatorCatalogueFake{}
			handler := operatorsHandler(t, adminAuthenticatorFake{err: testCase.err}, catalogue, "issuer")
			response := serveJSON(handler, http.MethodGet, identityhttp.OperatorsPath, "")
			if response.Code != testCase.status || problemCode(t, response) != testCase.code || catalogue.listed != 0 {
				t.Fatalf("status %d code %s listed %d", response.Code, problemCode(t, response), catalogue.listed)
			}
		})
	}
	down := operatorsHandler(t, adminAuthenticatorFake{administrator: administrator(t)}, &operatorCatalogueFake{err: errors.New("db down")}, "issuer")
	response := serveJSON(down, http.MethodGet, identityhttp.OperatorsPath, "")
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("读册失败 = %d %s", response.Code, response.Body.String())
	}
	post := serveJSON(operatorsHandler(t, adminAuthenticatorFake{administrator: administrator(t)}, &operatorCatalogueFake{}, "issuer"), http.MethodPost, identityhttp.OperatorsPath, "{}")
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST = %d allow %s", post.Code, post.Header().Get("Allow"))
	}
}

type admissionGrade struct {
	name   string
	err    error
	status int
	code   string
}

func admissionGrades() []admissionGrade {
	return []admissionGrade{
		{"issuer unconfigured", accessidentity.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"credential rejected", accessidentity.ErrCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"not granted", accessidentity.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"verifier unavailable", accessidentity.ErrCredentialVerifierUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		{"registry unavailable", accessidentity.ErrOperatorRegistryUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
}

// writeFixture 是一个写口连同它的替身。
type writeFixture struct {
	path       string
	handler    http.Handler
	transactor *transactorFake
	registrar  *registrarFake
	catalogue  *operatorCatalogueFake
	valid      string
}

const (
	validOperatorPayload   = `{"issuer":"https://id.syn.example/dex","subject":"SYN-OPERATOR-02","basis":"SYN-BASIS-OP-02"}`
	validGrantPayload      = `{"grantId":"SYN-GRANT-NEW","issuer":"https://id.syn.example/dex","subject":"SYN-OPERATOR-02","capabilityFace":"REGISTRY_CONFIGURATION_WRITE","effectiveStartsAt":"2026-01-01T00:00:00Z","basis":"SYN-BASIS-GRANT"}`
	validRevocationPayload = `{"grantId":"SYN-GRANT-NEW","revokedAt":"2026-11-04T08:00:00Z","basis":"SYN-BASIS-REVOKE"}`
)

func writeFixtures(t *testing.T, authenticator identityhttp.OperatorAccessAdministrationAuthenticator, outcome accessidentity.OperatorRegistrationOutcome, registrarErr error) []writeFixture {
	t.Helper()
	build := func(path, valid string, construct func(*transactorFake, *registrarFake, *operatorCatalogueFake) (http.Handler, error)) writeFixture {
		transactor := &transactorFake{}
		registrar := &registrarFake{outcome: outcome, err: registrarErr}
		catalogue := &operatorCatalogueFake{}
		handler, err := construct(transactor, registrar, catalogue)
		if err != nil {
			t.Fatal(err)
		}
		return writeFixture{path: path, handler: handler, transactor: transactor, registrar: registrar, catalogue: catalogue, valid: valid}
	}
	return []writeFixture{
		build(identityhttp.OperatorRegistrationsPath, validOperatorPayload, func(tx *transactorFake, registrar *registrarFake, _ *operatorCatalogueFake) (http.Handler, error) {
			return identityhttp.NewRegisterOperatorEndpoint(authenticator, tx, registrar)
		}),
		build(identityhttp.OperatorGrantRegistrationsPath, validGrantPayload, func(tx *transactorFake, registrar *registrarFake, _ *operatorCatalogueFake) (http.Handler, error) {
			return identityhttp.NewRegisterOperatorGrantEndpoint(authenticator, tx, registrar)
		}),
		build(identityhttp.OperatorGrantRevocationsPath, validRevocationPayload, func(tx *transactorFake, registrar *registrarFake, catalogue *operatorCatalogueFake) (http.Handler, error) {
			return identityhttp.NewRegisterOperatorGrantRevocationEndpoint(authenticator, tx, registrar, catalogue, func() time.Time { return administrationAt })
		}),
	}
}

func (fixture writeFixture) registrations() int {
	return len(fixture.registrar.bindings) + len(fixture.registrar.grants) + len(fixture.registrar.revocations)
}

// Covers: ADR-0363 决定二——三个写口各走三格准入；认证不过不读载荷、不开事务；只收 POST。
func TestOperatorWriteEndpointsAnswerGrades(t *testing.T) {
	for _, testCase := range admissionGrades() {
		for _, fixture := range writeFixtures(t, adminAuthenticatorFake{err: testCase.err}, accessidentity.OperatorRegistrationRecorded, nil) {
			t.Run(testCase.name+" "+fixture.path, func(t *testing.T) {
				response := serveJSON(fixture.handler, http.MethodPost, fixture.path, fixture.valid)
				if response.Code != testCase.status || problemCode(t, response) != testCase.code {
					t.Fatalf("status %d code %s body %s", response.Code, problemCode(t, response), response.Body.String())
				}
				if fixture.transactor.opened != 0 || fixture.registrations() != 0 {
					t.Fatal("认证没过却开了事务或写了册")
				}
			})
		}
	}
	for _, fixture := range writeFixtures(t, adminAuthenticatorFake{administrator: administrator(t)}, accessidentity.OperatorRegistrationRecorded, nil) {
		get := serveJSON(fixture.handler, http.MethodGet, fixture.path, "")
		if get.Code != http.StatusMethodNotAllowed || get.Header().Get("Allow") != http.MethodPost || problemCode(t, get) != "METHOD_NOT_ALLOWED" {
			t.Fatalf("%s GET = %d allow %s", fixture.path, get.Code, get.Header().Get("Allow"))
		}
	}
}

// Covers: ADR-0363 决定二「租户只取自信封，带了即 400；未知键拒」——空体、坏 JSON、带 tenantId、未知键、尾随内容、形状过不了
// 构造门（缺依据、未知能力面、撤销时刻缺席）都答 400 MALFORMED_REQUEST，不开事务。
func TestOperatorWriteEndpointsRejectMalformedPayloads(t *testing.T) {
	bad := map[string][]string{
		identityhttp.OperatorRegistrationsPath: {
			"", "{", `{"issuer":"https://id.syn.example/dex","subject":"SYN-OPERATOR-02","basis":"B","tenantId":"SYN-TENANT-01"}`,
			`{"issuer":"https://id.syn.example/dex","subject":"SYN-OPERATOR-02","basis":"B","name":"someone"}`,
			validOperatorPayload + `{}`, `{"issuer":"https://id.syn.example/dex","subject":"SYN-OPERATOR-02"}`,
		},
		identityhttp.OperatorGrantRegistrationsPath: {
			"", "[]", `{"tenantId":"SYN-TENANT-01",` + validGrantPayload[1:],
			`{"grantId":"G","issuer":"i","subject":"s","capabilityFace":"GOVERNANCE_REGISTRATION","effectiveStartsAt":"2026-01-01T00:00:00Z","basis":"B"}`,
			`{"grantId":"G","issuer":"i","subject":"s","capabilityFace":"REGISTRY_CONFIGURATION_WRITE","decisionKind":"SEGMENT_CLOSURE","effectiveStartsAt":"2026-01-01T00:00:00Z","basis":"B"}`,
			`{"grantId":"G","issuer":"i","subject":"s","capabilityFace":"REGISTRY_CONFIGURATION_WRITE","basis":"B"}`,
		},
		identityhttp.OperatorGrantRevocationsPath: {
			"", "null", `{"grantId":"G","revokedAt":"2026-11-04T08:00:00Z","basis":"B","tenantId":"SYN-TENANT-01"}`,
			`{"grantId":"G","basis":"B"}`, `{"grantId":"G","revokedAt":"2026-11-04T08:00:00Z"}`,
		},
	}
	for _, fixture := range writeFixtures(t, adminAuthenticatorFake{administrator: administrator(t)}, accessidentity.OperatorRegistrationRecorded, nil) {
		for _, payload := range bad[fixture.path] {
			response := serveJSON(fixture.handler, http.MethodPost, fixture.path, payload)
			if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
				t.Fatalf("%s %q = %d %s", fixture.path, payload, response.Code, response.Body.String())
			}
		}
		if fixture.transactor.opened != 0 || fixture.registrations() != 0 {
			t.Fatalf("%s：坏载荷开了事务或写了册", fixture.path)
		}
	}
}

// Covers: ADR-0363 决定二「RECORDED 答 201、其余 200（ADR-0022）」——每次写各开一笔事务，租户取自信封；册写不动答答案未形成。
func TestOperatorWriteEndpointsMapOutcomesToStatus(t *testing.T) {
	cases := []struct {
		outcome accessidentity.OperatorRegistrationOutcome
		status  int
	}{
		{accessidentity.OperatorRegistrationRecorded, http.StatusCreated},
		{accessidentity.OperatorRegistrationAlreadyRegistered, http.StatusOK},
		{accessidentity.OperatorRegistrationContentConflict, http.StatusOK},
		{accessidentity.OperatorSubjectBoundToAnotherTenant, http.StatusOK},
		{accessidentity.OperatorNotRegistered, http.StatusOK},
		{accessidentity.OperatorGrantNotRegistered, http.StatusOK},
	}
	for _, testCase := range cases {
		for _, fixture := range writeFixtures(t, adminAuthenticatorFake{administrator: administrator(t)}, testCase.outcome, nil) {
			response := serveJSON(fixture.handler, http.MethodPost, fixture.path, fixture.valid)
			var body struct {
				Outcome string `json:"outcome"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != testCase.status || body.Outcome != testCase.outcome.String() {
				t.Fatalf("%s %s = %d %s", fixture.path, testCase.outcome, response.Code, response.Body.String())
			}
			if fixture.transactor.opened != 1 || fixture.registrations() != 1 {
				t.Fatalf("%s：事务 %d 笔、登记 %d 次，want 各一", fixture.path, fixture.transactor.opened, fixture.registrations())
			}
		}
	}
	fixtures := writeFixtures(t, adminAuthenticatorFake{administrator: administrator(t)}, accessidentity.OperatorRegistrationRecorded, nil)
	for _, fixture := range fixtures {
		serveJSON(fixture.handler, http.MethodPost, fixture.path, fixture.valid)
	}
	if fixtures[0].registrar.bindings[0].TenantID() != catalogueTenant || fixtures[1].registrar.grants[0].TenantID() != catalogueTenant ||
		fixtures[2].registrar.revocations[0].TenantID() != catalogueTenant {
		t.Fatal("写进册的租户不是信封上的租户")
	}
	for _, fixture := range writeFixtures(t, adminAuthenticatorFake{administrator: administrator(t)}, "", errors.New("db down")) {
		response := serveJSON(fixture.handler, http.MethodPost, fixture.path, fixture.valid)
		if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
			t.Fatalf("%s 册写不动 = %d %s", fixture.path, response.Code, response.Body.String())
		}
	}
}

// Covers: ADR-0363 决定三——撤销落在调用者自己此刻生效的操作者接入管理授予上答 REVOCATION_WOULD_LOCK_OUT_CALLER、不写；别人的管理授予、
// 自己的其它格、自己已到期或已撤销的管理授予照常交给登记口；授予不在册也交给登记口（它答 GRANT_NOT_REGISTERED）。
func TestRevocationRefusesToLockOutTheCaller(t *testing.T) {
	self, other := catalogueSubject(t), anotherSubject(t)
	past, future := administrationAt.Add(-48*time.Hour), administrationAt.Add(48*time.Hour)
	expired := administrationAt.Add(-time.Hour)
	cases := []struct {
		name    string
		grant   accessidentity.RecordedGrant
		found   bool
		outcome accessidentity.OperatorRegistrationOutcome
		written bool
	}{
		{"own effective administration grant", recordedGrantOf(t, self, "SYN-GRANT-NEW", accessidentity.CapabilityOperatorAccessAdministration, past, time.Time{}, nil), true, accessidentity.OperatorRevocationWouldLockOutCaller, false},
		{"own administration grant bounded and still effective", recordedGrantOf(t, self, "SYN-GRANT-NEW", accessidentity.CapabilityOperatorAccessAdministration, past, future, nil), true, accessidentity.OperatorRevocationWouldLockOutCaller, false},
		{"someone else's administration grant", recordedGrantOf(t, other, "SYN-GRANT-NEW", accessidentity.CapabilityOperatorAccessAdministration, past, time.Time{}, nil), true, accessidentity.OperatorRegistrationRecorded, true},
		{"own grant of another face", recordedGrantOf(t, self, "SYN-GRANT-NEW", accessidentity.CapabilityRegistryConfigurationWrite, past, time.Time{}, nil), true, accessidentity.OperatorRegistrationRecorded, true},
		{"own administration grant already expired", recordedGrantOf(t, self, "SYN-GRANT-NEW", accessidentity.CapabilityOperatorAccessAdministration, past, expired, nil), true, accessidentity.OperatorRegistrationRecorded, true},
		{"own administration grant already revoked", recordedGrantOf(t, self, "SYN-GRANT-NEW", accessidentity.CapabilityOperatorAccessAdministration, past, time.Time{}, &expired), true, accessidentity.OperatorRegistrationRecorded, true},
		{"own administration grant not yet effective", recordedGrantOf(t, self, "SYN-GRANT-NEW", accessidentity.CapabilityOperatorAccessAdministration, future, time.Time{}, nil), true, accessidentity.OperatorRegistrationRecorded, true},
		{"grant not registered", accessidentity.RecordedGrant{}, false, accessidentity.OperatorRegistrationRecorded, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transactor := &transactorFake{}
			registrar := &registrarFake{outcome: accessidentity.OperatorRegistrationRecorded}
			catalogue := &operatorCatalogueFake{grant: testCase.grant, found: testCase.found}
			handler, err := identityhttp.NewRegisterOperatorGrantRevocationEndpoint(adminAuthenticatorFake{administrator: administrator(t)}, transactor, registrar, catalogue, func() time.Time { return administrationAt })
			if err != nil {
				t.Fatal(err)
			}
			response := serveJSON(handler, http.MethodPost, identityhttp.OperatorGrantRevocationsPath, validRevocationPayload)
			var body struct {
				Outcome string `json:"outcome"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			wantStatus := http.StatusOK
			if testCase.written {
				wantStatus = http.StatusCreated
			}
			if response.Code != wantStatus || body.Outcome != testCase.outcome.String() {
				t.Fatalf("status %d body %s", response.Code, response.Body.String())
			}
			if (len(registrar.revocations) == 1) != testCase.written {
				t.Fatalf("写了 %d 次，want written=%t", len(registrar.revocations), testCase.written)
			}
			if transactor.opened != 1 || catalogue.tenant != catalogueTenant || len(catalogue.grantIDs) != 1 || catalogue.grantIDs[0] != "SYN-GRANT-NEW" {
				t.Fatalf("事务 %d 笔，取授予 %v（租户 %s）", transactor.opened, catalogue.grantIDs, catalogue.tenant)
			}
		})
	}
	down := &operatorCatalogueFake{err: errors.New("db down")}
	handler, err := identityhttp.NewRegisterOperatorGrantRevocationEndpoint(adminAuthenticatorFake{administrator: administrator(t)}, &transactorFake{}, &registrarFake{}, down, func() time.Time { return administrationAt })
	if err != nil {
		t.Fatal(err)
	}
	response := serveJSON(handler, http.MethodPost, identityhttp.OperatorGrantRevocationsPath, validRevocationPayload)
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("取授予失败 = %d %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0363 决定一——认证件以操作者接入管理这一格铸信封、不带准入要求，交回信封上的租户与调用者主体；只持登记册配置写的人
// 答未授予（分格：能登业务数据的人不因此能给人授权）；发行方未配置答未配置。
func TestAccessAdministrationAuthenticatorAsksForTheAdministrationFace(t *testing.T) {
	held := standingWithFace(t, accessidentity.CapabilityOperatorAccessAdministration)
	recorded := &recordingMinter{inner: testMinter(t, verifierFake{subject: catalogueSubject(t)}, registryFake{standing: held, found: true})}
	authenticator, err := identityhttp.NewOperatorAccessAdministrationAuthenticator(recorded)
	if err != nil {
		t.Fatal(err)
	}
	administrator, err := authenticator.AuthenticateAccessAdministration(context.Background(), "presented.operator.token")
	if err != nil || administrator.TenantID != catalogueTenant || administrator.Subject != catalogueSubject(t) {
		t.Fatalf("administrator %+v err %v", administrator, err)
	}
	if recorded.request.Face != accessidentity.CapabilityOperatorAccessAdministration ||
		recorded.request.TenantID != "" || recorded.request.Admission != nil || recorded.request.DecisionKind != "" {
		t.Fatalf("铸造请求 = %+v", recorded.request)
	}

	writer := standingWithFace(t, accessidentity.CapabilityRegistryConfigurationWrite)
	writerAuth, err := identityhttp.NewOperatorAccessAdministrationAuthenticator(testMinter(t, verifierFake{subject: catalogueSubject(t)}, registryFake{standing: writer, found: true}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writerAuth.AuthenticateAccessAdministration(context.Background(), "token"); !errors.Is(err, accessidentity.ErrOperatorNotGranted) {
		t.Fatalf("只有登记册配置写授予 err = %v", err)
	}

	unconfigured, err := identityhttp.NewOperatorAccessAdministrationAuthenticator(testMinter(t, accessidentity.UnconfiguredOperatorCredentialVerifier{}, registryFake{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unconfigured.AuthenticateAccessAdministration(context.Background(), "token"); !errors.Is(err, accessidentity.ErrAccessChannelNotConfigured) {
		t.Fatalf("发行方未配置 err = %v", err)
	}
}

// Covers: 端点表上没接铸造器时四口各答未配置：读口只收 GET、写口只收 POST。
func TestUnconfiguredAccessAdministrationHandlers(t *testing.T) {
	read := identityhttp.OperatorsHandler(nil)
	response := serveJSON(read, http.MethodGet, identityhttp.OperatorsPath, "")
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("GET /operators = %d %s", response.Code, response.Body.String())
	}
	for _, handler := range []http.Handler{
		identityhttp.OperatorRegistrationsHandler(nil), identityhttp.OperatorGrantRegistrationsHandler(nil), identityhttp.OperatorGrantRevocationsHandler(nil),
	} {
		response := serveJSON(handler, http.MethodPost, "/", "{}")
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("POST = %d %s", response.Code, response.Body.String())
		}
		get := serveJSON(handler, http.MethodGet, "/", "")
		if get.Code != http.StatusMethodNotAllowed || get.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("GET = %d allow %s", get.Code, get.Header().Get("Allow"))
		}
	}
	wired := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusTeapot) })
	for _, handler := range []http.Handler{
		identityhttp.OperatorsHandler(wired), identityhttp.OperatorRegistrationsHandler(wired),
		identityhttp.OperatorGrantRegistrationsHandler(wired), identityhttp.OperatorGrantRevocationsHandler(wired),
	} {
		if response := serveJSON(handler, http.MethodPost, "/", "{}"); response.Code != http.StatusTeapot {
			t.Fatalf("已装的处理器没被交回：%d", response.Code)
		}
	}
}

func TestAccessAdministrationConstructorsRejectNil(t *testing.T) {
	authenticator := adminAuthenticatorFake{}
	transactor := &transactorFake{}
	registrar := &registrarFake{}
	catalogue := &operatorCatalogueFake{}
	now := func() time.Time { return administrationAt }
	if _, err := identityhttp.NewOperatorAccessAdministrationAuthenticator(nil); err == nil {
		t.Fatal("nil minter")
	}
	if _, err := identityhttp.NewQueryOperatorsEndpoint(nil, catalogue, "i"); err == nil {
		t.Fatal("operators: nil authenticator")
	}
	if _, err := identityhttp.NewQueryOperatorsEndpoint(authenticator, nil, "i"); err == nil {
		t.Fatal("operators: nil catalogue")
	}
	if _, err := identityhttp.NewRegisterOperatorEndpoint(nil, transactor, registrar); err == nil {
		t.Fatal("registration: nil authenticator")
	}
	if _, err := identityhttp.NewRegisterOperatorEndpoint(authenticator, nil, registrar); err == nil {
		t.Fatal("registration: nil transactor")
	}
	if _, err := identityhttp.NewRegisterOperatorEndpoint(authenticator, transactor, nil); err == nil {
		t.Fatal("registration: nil registrar")
	}
	if _, err := identityhttp.NewRegisterOperatorGrantEndpoint(nil, transactor, registrar); err == nil {
		t.Fatal("grant: nil authenticator")
	}
	if _, err := identityhttp.NewRegisterOperatorGrantEndpoint(authenticator, transactor, nil); err == nil {
		t.Fatal("grant: nil registrar")
	}
	if _, err := identityhttp.NewRegisterOperatorGrantRevocationEndpoint(authenticator, transactor, registrar, nil, now); err == nil {
		t.Fatal("revocation: nil catalogue")
	}
	if _, err := identityhttp.NewRegisterOperatorGrantRevocationEndpoint(authenticator, transactor, registrar, catalogue, nil); err == nil {
		t.Fatal("revocation: nil clock")
	}
}
