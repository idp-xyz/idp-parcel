package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 端点测试驱动真实编排，替身只在端口层。
type evidenceStub struct {
	items       map[string]domain.EvidenceItem
	disclosures map[string]domain.EvidenceDisclosureVersion
	registrars  map[string]string
	appraisers  map[string]string
	err         error
}

func (stub *evidenceStub) FindByID(_ context.Context, tenant domain.TenantID, id domain.EvidenceItemID) (domain.EvidenceItem, bool, error) {
	if stub.err != nil {
		return domain.EvidenceItem{}, false, stub.err
	}
	item, found := stub.items[tenant.String()+"|"+id.String()]
	return item, found, nil
}

func (stub *evidenceStub) FindByProviderDigest(_ context.Context, tenant domain.TenantID, provider domain.EvidenceProviderReference, digest domain.EvidenceContentDigest) (domain.EvidenceItem, bool, error) {
	if stub.err != nil {
		return domain.EvidenceItem{}, false, stub.err
	}
	for key, item := range stub.items {
		if item.Provider() == provider && item.Digest() == digest && key == tenant.String()+"|"+item.ID().String() {
			return item, true, nil
		}
	}
	return domain.EvidenceItem{}, false, nil
}

func (stub *evidenceStub) Save(_ context.Context, tenant domain.TenantID, item domain.EvidenceItem) (ports.EvidenceSaveOutcome, error) {
	stub.items[tenant.String()+"|"+item.ID().String()] = item
	return ports.EvidenceSaved, nil
}

func (stub *evidenceStub) FindDisclosure(_ context.Context, tenant domain.TenantID, item domain.EvidenceItemID, redacted domain.EvidenceContentDigest, scope string) (domain.EvidenceDisclosureVersion, bool, error) {
	version, found := stub.disclosures[tenant.String()+"|"+item.String()+"|"+redacted.String()+"|"+scope]
	return version, found, nil
}

func (stub *evidenceStub) SaveDisclosure(_ context.Context, tenant domain.TenantID, version domain.EvidenceDisclosureVersion) (ports.EvidenceSaveOutcome, error) {
	stub.disclosures[tenant.String()+"|"+version.Item().String()+"|"+version.Redacted().String()+"|"+version.Scope()] = version
	return ports.EvidenceSaved, nil
}

func (stub *evidenceStub) RecordRegistrar(_ context.Context, tenant domain.TenantID, item domain.EvidenceItemID, operator string) error {
	key := tenant.String() + "|" + item.String()
	if _, recorded := stub.registrars[key]; !recorded {
		stub.registrars[key] = operator
	}
	return nil
}

func (stub *evidenceStub) AppraiseEvidence(_ context.Context, record ports.EvidenceAppraisalRecord) (ports.EvidenceAppraisalOutcome, error) {
	key := record.Tenant.String() + "|" + record.Item.ID().String()
	if current, found := stub.items[key]; !found || current.Appraisal() != domain.EvidenceReceived {
		return ports.EvidenceAlreadyAppraised, nil
	}
	stub.items[key] = record.Item
	stub.appraisers[key] = record.Operator
	return ports.EvidenceAppraisalRecorded, nil
}

type evidenceIdentities struct{ next int }

func (identities *evidenceIdentities) NextEvidenceItemID(context.Context) (domain.EvidenceItemID, error) {
	identities.next++
	return domain.NewEvidenceItemID("SYN-EVIDENCE-000" + string(rune('0'+identities.next)))
}

type evidenceEndpoints struct {
	submit, appraise, disclose http.Handler
	store                      *evidenceStub
	asked                      *[]visibilityhttp.OperatorDecision
}

func evidenceEndpointsForTest(t *testing.T, authErr error) evidenceEndpoints {
	t.Helper()
	var asked []visibilityhttp.OperatorDecision
	intake, err := visibilityhttp.NewOperatorDecisionIntake(caseDecisionAuthenticator{asked: &asked, err: authErr})
	if err != nil {
		t.Fatal(err)
	}
	store := &evidenceStub{items: map[string]domain.EvidenceItem{}, disclosures: map[string]domain.EvidenceDisclosureVersion{},
		registrars: map[string]string{}, appraisers: map[string]string{}}
	manager := application.NewManageEvidenceHandler(application.ManageEvidenceDeps{
		Decisions: store, Evidence: store, Identities: &evidenceIdentities{}, Clock: caseClock{},
	})
	return evidenceEndpoints{
		submit:   visibilityhttp.NewSubmitEvidenceEndpoint(intake, manager),
		appraise: visibilityhttp.NewAppraiseEvidenceEndpoint(intake, manager),
		disclose: visibilityhttp.NewPrepareEvidenceDisclosureEndpoint(intake, manager),
		store:    store,
		asked:    &asked,
	}
}

type evidenceAnswer struct {
	Outcome        string `json:"outcome"`
	EvidenceID     string `json:"evidenceId"`
	Appraisal      string `json:"appraisal"`
	AppraisalBasis string `json:"appraisalBasis"`
	SubmittedAt    string `json:"submittedAt"`
	RedactedDigest string `json:"redactedDigest"`
	Scope          string `json:"scope"`
	Error          struct {
		Code string `json:"code"`
	} `json:"error"`
}

func serveEvidence(t *testing.T, handler http.Handler, token, body string) (int, evidenceAnswer) {
	t.Helper()
	status, raw := serveRaw(t, handler, token, body)
	var answer evidenceAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, raw)
	}
	return status, answer
}

const (
	submitEvidenceBody   = `{"provider":"SYN-CUSTOMER-01","contentDigest":"sha256:syn-damage-photo"}`
	appraiseEvidenceBody = `{"evidenceId":"SYN-EVIDENCE-0001","appraisal":"CREDITED","basis":"照片元数据与收寄时间一致"}`
	discloseEvidenceBody = `{"evidenceId":"SYN-EVIDENCE-0001","redactedDigest":"sha256:syn-damage-photo-redacted","scope":"SYN-CARRIER-01"}`
)

// Covers: 证据的三个运营决定口（UC-VE-007 步 2）——各按自己那一种决定认证；登记形成一项已收到的证据，收到时刻是服务端
// 的这一刻、登记人是认证出来的操作者（201），再登同一份答已有（200）；评价把它推成采信并带出依据（201），再评价答
// 已评价（200）；为它准备一份披露版本（201），同一范围的同一脱敏版本再准备答已有（200）。
func TestEvidenceFacesRegisterAppraiseAndPrepareDisclosure(t *testing.T) {
	endpoints := evidenceEndpointsForTest(t, nil)

	status, answer := serveEvidence(t, endpoints.submit, "t", submitEvidenceBody)
	if status != http.StatusCreated || answer.Outcome != "EVIDENCE_SUBMITTED" || answer.EvidenceID != "SYN-EVIDENCE-0001" ||
		answer.Appraisal != "RECEIVED" || answer.SubmittedAt != "2026-11-13T02:00:00Z" {
		t.Fatalf("登记：status %d answer %+v", status, answer)
	}
	if registrar := endpoints.store.registrars["SYN-TENANT-01|SYN-EVIDENCE-0001"]; registrar != "https://issuer#alice" {
		t.Fatalf("登记人 = %q", registrar)
	}
	if status, answer = serveEvidence(t, endpoints.submit, "t", submitEvidenceBody); status != http.StatusOK || answer.Outcome != "EVIDENCE_EXISTING_RESULT" {
		t.Fatalf("再登：status %d answer %+v", status, answer)
	}

	status, answer = serveEvidence(t, endpoints.appraise, "t", appraiseEvidenceBody)
	if status != http.StatusCreated || answer.Outcome != "EVIDENCE_APPRAISED" || answer.Appraisal != "CREDITED" || answer.AppraisalBasis != "照片元数据与收寄时间一致" {
		t.Fatalf("评价：status %d answer %+v", status, answer)
	}
	if status, answer = serveEvidence(t, endpoints.appraise, "t", `{"evidenceId":"SYN-EVIDENCE-0001","appraisal":"DISCREDITED","basis":"相反结论"}`); status != http.StatusOK || answer.Outcome != "EVIDENCE_ALREADY_APPRAISED" || answer.Appraisal != "CREDITED" {
		t.Fatalf("再评价：status %d answer %+v——该交回在册的采信，不改写", status, answer)
	}

	status, answer = serveEvidence(t, endpoints.disclose, "t", discloseEvidenceBody)
	if status != http.StatusCreated || answer.Outcome != "DISCLOSURE_PREPARED" || answer.RedactedDigest != "sha256:syn-damage-photo-redacted" || answer.Scope != "SYN-CARRIER-01" {
		t.Fatalf("披露版本：status %d answer %+v", status, answer)
	}
	if status, answer = serveEvidence(t, endpoints.disclose, "t", discloseEvidenceBody); status != http.StatusOK || answer.Outcome != "DISCLOSURE_EXISTING_RESULT" {
		t.Fatalf("再准备：status %d answer %+v", status, answer)
	}

	asked := *endpoints.asked
	if len(asked) != 6 || asked[0] != "EVIDENCE_SUBMISSION" || asked[2] != "EVIDENCE_APPRAISAL" || asked[4] != "EVIDENCE_DISCLOSURE_PREPARATION" {
		t.Fatalf("asked = %v——三口要各按自己那一种决定认证", asked)
	}
}

// Covers: 三口的拒绝各格——载荷是封闭的，不收租户、收到时刻、评价起点与原件指纹（400）；评价词只收采信与不采信（400）；
// 令牌缺失 401；没有这一种授予 403；评价不在册的证据、脱敏指纹与原件相同是业务答复（200）；证据册读不动是没形成
// 答案（500）。
func TestEvidenceFacesGradeTheirRefusals(t *testing.T) {
	for name, test := range map[string]struct {
		face     string
		token    string
		body     string
		authErr  error
		seed     bool
		storeErr bool
		status   int
		outcome  string
		code     string
	}{
		"登记带租户":     {face: "submit", token: "t", body: `{"tenantId":"T","provider":"P","contentDigest":"d"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"登记带收到时刻":   {face: "submit", token: "t", body: `{"provider":"P","contentDigest":"d","submittedAt":"2026-01-01T00:00:00Z"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"登记带评价":     {face: "submit", token: "t", body: `{"provider":"P","contentDigest":"d","appraisal":"CREDITED"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"登记缺指纹":     {face: "submit", token: "t", body: `{"provider":"P"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"评价词是已收到":   {face: "appraise", token: "t", body: `{"evidenceId":"SYN-EVIDENCE-0001","appraisal":"RECEIVED","basis":"x"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"披露带原件指纹":   {face: "disclose", token: "t", body: `{"evidenceId":"E","redactedDigest":"r","scope":"s","originalDigest":"o"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"令牌缺失":      {face: "submit", body: submitEvidenceBody, status: http.StatusUnauthorized, code: "OPERATOR_CREDENTIAL_REJECTED"},
		"没有这一种授予":   {face: "appraise", token: "t", body: appraiseEvidenceBody, authErr: visibilityhttp.ErrOperatorNotGranted, status: http.StatusForbidden, code: "OPERATOR_NOT_GRANTED"},
		"评价不在册的证据":  {face: "appraise", token: "t", body: appraiseEvidenceBody, status: http.StatusOK, outcome: "EVIDENCE_NOT_FOUND"},
		"评价没有依据":    {face: "appraise", token: "t", seed: true, body: `{"evidenceId":"SYN-EVIDENCE-0001","appraisal":"CREDITED","basis":" "}`, status: http.StatusOK, outcome: "NOT_ACCEPTED"},
		"脱敏指纹与原件相同": {face: "disclose", token: "t", seed: true, body: `{"evidenceId":"SYN-EVIDENCE-0001","redactedDigest":"sha256:syn-damage-photo","scope":"SYN-CARRIER-01"}`, status: http.StatusOK, outcome: "NOT_ACCEPTED"},
		"披露没有范围":    {face: "disclose", token: "t", seed: true, body: `{"evidenceId":"SYN-EVIDENCE-0001","redactedDigest":"sha256:redacted","scope":""}`, status: http.StatusOK, outcome: "NOT_ACCEPTED"},
		"证据册读不动":    {face: "submit", token: "t", body: submitEvidenceBody, storeErr: true, status: http.StatusInternalServerError, code: "NO_ANSWER_FORMED"},
	} {
		t.Run(name, func(t *testing.T) {
			endpoints := evidenceEndpointsForTest(t, test.authErr)
			if test.seed {
				if status, _ := serveEvidence(t, endpoints.submit, "t", submitEvidenceBody); status != http.StatusCreated {
					t.Fatalf("预置证据没登上：%d", status)
				}
			}
			if test.storeErr {
				endpoints.store.err = errors.New("connection refused")
			}
			handler := map[string]http.Handler{"submit": endpoints.submit, "appraise": endpoints.appraise, "disclose": endpoints.disclose}[test.face]
			status, answer := serveEvidence(t, handler, test.token, test.body)
			if status != test.status || answer.Outcome != test.outcome || answer.Error.Code != test.code {
				t.Fatalf("status %d outcome %q code %q, want %d %q %q", status, answer.Outcome, answer.Error.Code, test.status, test.outcome, test.code)
			}
			if len(endpoints.store.disclosures) != 0 || len(endpoints.store.appraisers) != 0 {
				t.Fatal("被拒的请求留下了评价或披露版本")
			}
		})
	}
}
