package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// manifestAuthenticator 同时实现两个认证口：只核租户的那一个（别的口用），与交回来源身份的这一个。
type manifestAuthenticator struct {
	clientAuthenticator
	carrier string
	err     error
}

func (fake manifestAuthenticator) AuthenticateManifestSource(
	_ context.Context,
	credential customshttp.PresentedClientCredential,
) (customshttp.ManifestSource, error) {
	if fake.err != nil {
		return customshttp.ManifestSource{}, fake.err
	}
	if credential.Token == "" {
		return customshttp.ManifestSource{}, customshttp.ErrIntegrationClientCredentialRejected
	}
	tenant, err := domain.NewTenantID(clientTenant)
	if err != nil {
		return customshttp.ManifestSource{}, err
	}
	return customshttp.ManifestSource{Tenant: tenant, Carrier: fake.carrier}, nil
}

// 端点测试驱动真实编排，替身只在端口层：传输层不该能捏造业务结果。
type manifestStoreStub struct {
	byManifest map[string]domain.ExternalManifestReference
	err        error
}

func manifestRow(carrier domain.CarrierResponsibilityReference, manifest domain.ExternalManifestID) string {
	return carrier.String() + "|" + manifest.String()
}

func (stub *manifestStoreStub) FindByManifest(_ context.Context, _ domain.TenantID, carrier domain.CarrierResponsibilityReference, manifest domain.ExternalManifestID) (domain.ExternalManifestReference, bool, error) {
	if stub.err != nil {
		return domain.ExternalManifestReference{}, false, stub.err
	}
	reference, found := stub.byManifest[manifestRow(carrier, manifest)]
	return reference, found, nil
}

func (stub *manifestStoreStub) Save(_ context.Context, _ domain.TenantID, reference domain.ExternalManifestReference) (ports.ManifestSaveOutcome, error) {
	row := manifestRow(reference.Carrier(), reference.Manifest())
	if _, exists := stub.byManifest[row]; exists {
		return ports.ManifestAlreadyRecorded, nil
	}
	stub.byManifest[row] = reference
	return ports.ManifestSaved, nil
}

func (stub *manifestStoreStub) Update(_ context.Context, _ domain.TenantID, reference domain.ExternalManifestReference) (ports.ManifestUpdateOutcome, error) {
	row := manifestRow(reference.Carrier(), reference.Manifest())
	prior, _ := reference.PriorVersion()
	if current, exists := stub.byManifest[row]; !exists || current.Version() != prior {
		return ports.ManifestVersionMoved, nil
	}
	stub.byManifest[row] = reference
	return ports.ManifestAdvanced, nil
}

type manifestCandidatesStub struct{ candidates []domain.AssociationCandidate }

func (stub manifestCandidatesStub) LoadAssociationCandidates(context.Context, domain.TenantID, domain.CustomsProcedureReference, domain.ManifestDirection) ([]domain.AssociationCandidate, error) {
	return stub.candidates, nil
}

type manifestHandoffStub struct{}

func (manifestHandoffStub) HandOffManifest(context.Context, ports.ManifestHandoffIntent) error {
	return nil
}

type manifestEndpoints struct {
	receive http.Handler
	revise  http.Handler
	store   *manifestStoreStub
}

func manifestEndpointsForTest(t *testing.T, authenticator customshttp.IntegrationClientAuthenticator) manifestEndpoints {
	t.Helper()
	store := &manifestStoreStub{byManifest: map[string]domain.ExternalManifestReference{}}
	handler := application.NewReceiveManifestHandler(application.ReceiveManifestDeps{
		Candidates: manifestCandidatesStub{},
		Store:      store,
		Downstream: manifestHandoffStub{},
		Clock:      fixedClock{at: clientReceivedAt},
	})
	intake := clientIntakeForTest(t, authenticator)
	return manifestEndpoints{
		receive: customshttp.NewReceiveManifestReferenceEndpoint(intake, handler),
		revise:  customshttp.NewReviseManifestReferenceEndpoint(intake, handler),
		store:   store,
	}
}

const manifestReferenceBody = `{"manifestId":"SYN-MANIFEST-0001","version":"V1","procedure":"SYN-EXPORT/GENERAL",` +
	`"direction":"EXPORT","scope":"SYN-MOVEMENT-0001","sourceFact":"SYN-CARRIER-FEED/manifest-filed"}`

type manifestAnswer struct {
	Outcome          string `json:"outcome"`
	Version          string `json:"version"`
	PriorVersion     string `json:"priorVersion"`
	DeclarationUnit  string `json:"declarationUnit"`
	HandoffReference string `json:"handoffReference"`
	Error            struct {
		Code string `json:"code"`
	} `json:"error"`
}

func serveManifest(t *testing.T, handler http.Handler, request *http.Request) (int, manifestAnswer) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var answer manifestAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, recorder.Body.String())
	}
	return recorder.Code, answer
}

// Covers: UC-CC-012 结果「外部监管舱单待关联」与「来源只取认证结果」——承运商的客户端交来一份引用，没有可唯一关联的申报
// 单元时照样入册、答待关联（201）；入册引用上的承运商责任是客户端在册上绑定的来源身份，租户是册上绑定的租户，都不取
// 请求头或查询串里自报的那一个；同一份重放答已有（200）。
func TestACarrierClientRegistersAManifestReferenceUnderItsBoundIdentity(t *testing.T) {
	endpoints := manifestEndpointsForTest(t, manifestAuthenticator{carrier: "SYN-PARTY-CARRIER-TRUNK-01"})

	status, answer := serveManifest(t, endpoints.receive, commandRequest(manifestReferenceBody))
	if status != http.StatusCreated || answer.Outcome != "PENDING_ASSOCIATION" || answer.Version != "V1" || answer.DeclarationUnit != "" {
		t.Fatalf("status %d answer %+v, want 201 PENDING_ASSOCIATION V1 且不带申报单元", status, answer)
	}
	recorded, found := endpoints.store.byManifest["SYN-PARTY-CARRIER-TRUNK-01|SYN-MANIFEST-0001"]
	if !found || recorded.Carrier().String() != "SYN-PARTY-CARRIER-TRUNK-01" || recorded.Direction() != domain.ExportManifest ||
		!recorded.AcceptedAt().Equal(clientReceivedAt) {
		t.Fatalf("入册引用 = %+v（found=%v）——承运商责任该是册上绑定的来源身份，接受时刻该是服务端时钟", recorded, found)
	}

	status, answer = serveManifest(t, endpoints.receive, commandRequest(manifestReferenceBody))
	if status != http.StatusOK || answer.Outcome != "EXISTING" {
		t.Fatalf("重放：status %d answer %+v, want 200 EXISTING", status, answer)
	}
}

// Covers: UC-CC-012「更正、替代……形成新版本和关系，原引用不覆盖」的入口面——同舱单异版本经首登口来答版本冲突；经推进口来
// 形成新版本并指回前版（201）；别家承运商的客户端推进同一个号读的是它自己的编号空间，答不在册，在册引用不动；同版本
// 而范围不同答内容冲突。
func TestARevisionAdvancesTheVersionOnlyThroughItsOwnFaceAndOnlyForTheSameCarrier(t *testing.T) {
	endpoints := manifestEndpointsForTest(t, manifestAuthenticator{carrier: "SYN-PARTY-CARRIER-TRUNK-01"})
	if status, answer := serveManifest(t, endpoints.receive, commandRequest(manifestReferenceBody)); status != http.StatusCreated {
		t.Fatalf("首登：status %d answer %+v", status, answer)
	}

	secondVersion := strings.Replace(manifestReferenceBody, `"version":"V1"`, `"version":"V2"`, 1)
	if status, answer := serveManifest(t, endpoints.receive, commandRequest(secondVersion)); status != http.StatusOK || answer.Outcome != "VERSION_CONFLICT" {
		t.Fatalf("异版本走首登口：status %d answer %+v, want 200 VERSION_CONFLICT", status, answer)
	}

	const revisionBody = `{"manifestId":"SYN-MANIFEST-0001","version":"V2","scope":"SYN-MOVEMENT-0002","sourceFact":"SYN-CARRIER-FEED/manifest-amended"}`
	foreign := manifestEndpoints{
		revise: customshttp.NewReviseManifestReferenceEndpoint(
			clientIntakeForTest(t, manifestAuthenticator{carrier: "SYN-PARTY-CARRIER-OTHER"}),
			application.NewReceiveManifestHandler(application.ReceiveManifestDeps{
				Candidates: manifestCandidatesStub{}, Store: endpoints.store, Downstream: manifestHandoffStub{}, Clock: fixedClock{at: clientReceivedAt},
			})),
	}
	if status, answer := serveManifest(t, foreign.revise, commandRequest(revisionBody)); status != http.StatusOK || answer.Outcome != "NOT_FOUND" || answer.Version != "" {
		t.Fatalf("别家承运商推进：status %d answer %+v, want 200 NOT_FOUND 且不带在册版本", status, answer)
	}
	if kept := endpoints.store.byManifest["SYN-PARTY-CARRIER-TRUNK-01|SYN-MANIFEST-0001"]; kept.Version().String() != "V1" {
		t.Fatalf("在册版本 = %q，被别家承运商推进了", kept.Version())
	}

	changedScope := strings.Replace(manifestReferenceBody, `"SYN-MOVEMENT-0001"`, `"SYN-MOVEMENT-0009"`, 1)
	if status, answer := serveManifest(t, endpoints.receive, commandRequest(changedScope)); status != http.StatusOK || answer.Outcome != "CONTENT_CONFLICT" || answer.Version != "" {
		t.Fatalf("同版本异范围：status %d answer %+v, want 200 CONTENT_CONFLICT 且不带在册版本", status, answer)
	}

	status, answer := serveManifest(t, endpoints.revise, commandRequest(revisionBody))
	if status != http.StatusCreated || answer.Outcome != "REVISED" || answer.Version != "V2" || answer.PriorVersion != "V1" {
		t.Fatalf("推进：status %d answer %+v, want 201 REVISED V2 指回 V1", status, answer)
	}
}

// Covers: ADR-0022 的状态码分法与本口的 Intake 各格——载荷自报承运商或租户、方向不是封闭词、缺来源事实都是 400（重发同样
// 内容不会变）；令牌不过 401；没有这一类授予、客户端没绑来源身份 403；认证器不认这一类事实整口答未配置 403；引用册读不动
// 是没形成答案 500（通道留队重发），不是业务未决。
func TestManifestFacesGradeTheirRefusals(t *testing.T) {
	carrier := manifestAuthenticator{carrier: "SYN-PARTY-CARRIER-TRUNK-01"}
	cases := map[string]struct {
		authenticator customshttp.IntegrationClientAuthenticator
		body          string
		token         bool
		storeErr      error
		status        int
		code          string
	}{
		"payload reports a carrier": {authenticator: carrier, token: true, status: http.StatusBadRequest, code: "MALFORMED_REQUEST",
			body: strings.Replace(manifestReferenceBody, `{`, `{"carrier":"SYN-PARTY-CARRIER-OTHER",`, 1)},
		"payload reports a tenant": {authenticator: carrier, token: true, status: http.StatusBadRequest, code: "MALFORMED_REQUEST",
			body: strings.Replace(manifestReferenceBody, `{`, `{"tenantId":"TENANT-9",`, 1)},
		"direction is not a closed word": {authenticator: carrier, token: true, status: http.StatusBadRequest, code: "MALFORMED_REQUEST",
			body: strings.Replace(manifestReferenceBody, `"EXPORT"`, `"export"`, 1)},
		"source fact missing": {authenticator: carrier, token: true, status: http.StatusBadRequest, code: "MALFORMED_REQUEST",
			body: strings.Replace(manifestReferenceBody, `"SYN-CARRIER-FEED/manifest-filed"`, `" "`, 1)},
		"token rejected": {authenticator: carrier, token: false, status: http.StatusUnauthorized, code: "INTEGRATION_CLIENT_CREDENTIAL_REJECTED",
			body: manifestReferenceBody},
		"no grant for this fact": {authenticator: manifestAuthenticator{err: customshttp.ErrIntegrationClientNotGranted}, token: true,
			status: http.StatusForbidden, code: "INTEGRATION_CLIENT_NOT_GRANTED", body: manifestReferenceBody},
		"client bound to no source identity": {authenticator: manifestAuthenticator{carrier: ""}, token: true,
			status: http.StatusForbidden, code: "INTEGRATION_CLIENT_NOT_GRANTED", body: manifestReferenceBody},
		"authenticator does not know manifests": {authenticator: clientAuthenticator{}, token: true,
			status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED", body: manifestReferenceBody},
		"reference register unreadable": {authenticator: carrier, token: true, storeErr: errors.New("connection refused"),
			status: http.StatusInternalServerError, code: "NO_ANSWER_FORMED", body: manifestReferenceBody},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			endpoints := manifestEndpointsForTest(t, testCase.authenticator)
			endpoints.store.err = testCase.storeErr
			request := commandRequest(testCase.body)
			if !testCase.token {
				request.Header.Del("Authorization")
			}
			status, answer := serveManifest(t, endpoints.receive, request)
			if status != testCase.status || answer.Error.Code != testCase.code {
				t.Fatalf("status %d code %q, want %d %q", status, answer.Error.Code, testCase.status, testCase.code)
			}
			if len(endpoints.store.byManifest) != 0 {
				t.Fatal("被拒的请求留下了引用")
			}
		})
	}
}
