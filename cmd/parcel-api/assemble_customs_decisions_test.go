package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证关务三口的生产装配（票 product-strategy-boundary/18，ADR-0227）：端点行按特性登记处的那一份 mount
// 挂上，Intake 是生产那只操作者铸造器经真操作者册（access_identity 0018 放开的三种决定）铸的信封，编排是 buildCustomsDecisionFaces
// 那一套真库缝。已接受包裹借标签渠道那组用例种的那一份委托（saveAcceptedLabelChannelRequest），租户 SYN-TENANT-LC-RES。准入
// 范围取放行一切的替身——准入对照与区间各有自己的用例，这里证的是决定种类与编排。证据只记 `S`。

const (
	customsDecisionIssuer    = "https://id.syn.example/dex"
	customsDecisionTenant    = "SYN-TENANT-LC-RES"
	customsDecisionParcel    = "SYN-PARCEL-LC-RES"
	customsDecisionUnit      = "SYN-UNIT-PSB18-01"
	customsCaseOfficer       = "SYN-CASE-OFFICER-PSB18"
	customsApprover          = "SYN-APPROVER-PSB18"
	customsSubmitter         = "SYN-SUBMITTER-PSB18"
	customsDecisionProcedure = "SYN-PROC-CN-EXPORT"
)

// tokenSubjectVerifier 是核验方替身：令牌就是 sub，发行方固定；空令牌答令牌缺失。
type tokenSubjectVerifier struct{}

func (tokenSubjectVerifier) VerifyOperatorCredential(_ context.Context, credential accessidentity.OperatorCredential) (accessidentity.OperatorSubject, error) {
	if credential.Token() == "" {
		return accessidentity.OperatorSubject{}, accessidentity.ErrCredentialRejected
	}
	return accessidentity.NewOperatorSubject(customsDecisionIssuer, credential.Token())
}

type customsDecisionFixture struct {
	db        *bentopg.DB
	operators *aipg.OperatorRegistry
	endpoints map[string]http.Handler
	// numbers 是装进建案编排的那一套编号组件，用例经它的登记口为租户登单据编号时区或规则。
	numbers docnumbering.Components
}

// registerOperator 把一个合成操作者绑到演示委托所在的租户，并按给定区间逐种登运营决定授予，交回各笔授予号。断言在事务闭包外。
func registerOperator(t *testing.T, db *bentopg.DB, operators *aipg.OperatorRegistry, subject string,
	interval accessidentity.EffectiveInterval, kinds ...accessidentity.DecisionKind) []string {
	t.Helper()
	return registerOperatorIn(t, db, operators, customsDecisionTenant, subject, interval, kinds...)
}

// registerOperatorIn 同 registerOperator，租户由调用方给。
func registerOperatorIn(t *testing.T, db *bentopg.DB, operators *aipg.OperatorRegistry, tenant, subject string,
	interval accessidentity.EffectiveInterval, kinds ...accessidentity.DecisionKind) []string {
	t.Helper()
	operator, err := accessidentity.NewOperatorSubject(customsDecisionIssuer, subject)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewOperatorBinding(operator, tenant, "SYN-BIND-BASIS-PSB18")
	if err != nil {
		t.Fatal(err)
	}
	var grants []accessidentity.OperatorGrant
	var ids []string
	for _, kind := range kinds {
		id := "SYN-GRANT-" + subject + "-" + kind.String()
		grant, err := accessidentity.NewOperationDecisionGrant(tenant, id, operator, kind, interval, "SYN-GRANT-BASIS-PSB18")
		if err != nil {
			t.Fatal(err)
		}
		grants = append(grants, grant)
		ids = append(ids, id)
	}
	var outcomes []accessidentity.OperatorRegistrationOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		bound, err := operators.RegisterOperator(txCtx, binding)
		if err != nil {
			return err
		}
		outcomes = append(outcomes, bound)
		for _, grant := range grants {
			granted, err := operators.RegisterGrant(txCtx, grant)
			if err != nil {
				return err
			}
			outcomes = append(outcomes, granted)
		}
		return nil
	}); err != nil {
		t.Fatalf("登记操作者 %s：%v", subject, err)
	}
	for _, outcome := range outcomes {
		if outcome != accessidentity.OperatorRegistrationRecorded {
			t.Fatalf("登记操作者 %s：%v", subject, outcomes)
		}
	}
	return ids
}

func newCustomsDecisionFixture(t *testing.T) *customsDecisionFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for subject, kind := range map[string]accessidentity.DecisionKind{
		customsCaseOfficer: accessidentity.DecisionCustomsCaseEstablishment,
		customsApprover:    accessidentity.DecisionCustomsSubmissionAuthorization,
		customsSubmitter:   accessidentity.DecisionCustomsDeclarationSubmission,
	} {
		registerOperator(t, db, operators, subject, interval, kind)
	}
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := docnumbering.Build(db, docnumber.SecretTable{})
	if err != nil {
		t.Fatalf("装配编号组件：%v", err)
	}
	faces, err := buildCustomsDecisionFaces(db, numbers)
	if err != nil {
		t.Fatalf("装配：%v", err)
	}
	wiring := customsDecisionsWiring{
		caseEstablishment:       faces.caseEstablishment,
		submissionAuthorization: faces.submissionAuthorization,
		declarationSubmission:   faces.declarationSubmission,
	}
	endpoints := map[string]http.Handler{}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*customsDecisionsWiring)(nil)) {
			continue
		}
		for _, endpoint := range feature.mountWith(endpointChannels{operatorDecisions: decisions}, wiring) {
			endpoints[endpoint.Pattern] = endpoint.Handler
		}
	}
	if len(endpoints) != 3 {
		t.Fatalf("关务三口没按登记处挂上：%v", endpoints)
	}
	return &customsDecisionFixture{db: db, operators: operators, endpoints: endpoints, numbers: numbers}
}

func (fixture *customsDecisionFixture) post(t *testing.T, pattern, operator, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+operator)
	response := httptest.NewRecorder()
	fixture.endpoints[pattern].ServeHTTP(response, request)
	var answer map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("%s 答复不是 JSON：%s", pattern, response.Body.String())
	}
	return response.Code, answer
}

func (fixture *customsDecisionFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := querier.QueryRow(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("%s：%v", query, err)
	}
	return count
}

func (fixture *customsDecisionFixture) registerCaseRequirement(t *testing.T) {
	t.Helper()
	registrations, err := ccpostgres.NewCaseRequirementRegistrations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := registrations.RegisterCaseRequirementRule(txCtx,
			mustValue(t, ccdomain.NewTenantID, customsDecisionTenant),
			mustValue(t, ccdomain.NewRegulatoryJurisdictionReference, "SYN-JUR-CN"),
			ccdomain.ExportManifest,
			mustValue(t, ccdomain.NewCustomsProcedureReference, customsDecisionProcedure),
			ccports.CaseRequirementJudgment{Required: true, Basis: "SYN-BASIS-CASE-REQ-PSB18"})
		return err
	}); err != nil {
		t.Fatalf("登记建案要求：%v", err)
	}
}

func (fixture *customsDecisionFixture) registerReadiness(t *testing.T, unit string) {
	t.Helper()
	registrations, err := ccpostgres.NewReadinessRegistrations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	judgment, err := ccdomain.JudgeReady(mustValue(t, ccdomain.NewDeclarationUnitID, unit),
		mustValue(t, ccdomain.NewReadinessBasisReference, "SYN-BASIS-READINESS-PSB18"), time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := registrations.RegisterReadiness(txCtx, mustValue(t, ccdomain.NewTenantID, customsDecisionTenant), judgment)
		return err
	}); err != nil {
		t.Fatalf("登记就绪：%v", err)
	}
}

const customsCaseBody = `{"jurisdiction":"SYN-JUR-CN","direction":"EXPORT","procedure":"SYN-PROC-CN-EXPORT","obligation":"SYN-OBL-CN-EXPORT",` +
	`"parcels":["SYN-PARCEL-LC-RES"],"roles":[{"role":"DECLARANT","party":"SYN-PARTY-BROKER-01","authority":"SYN-MANDATE-01"}]}`

func customsSubmissionBody(caseID string) string {
	return `{"unitId":"` + customsDecisionUnit + `","caseId":"` + caseID + `","procedure":"` + customsDecisionProcedure + `",` +
		`"members":["` + customsDecisionParcel + `"],"dossier":"SYN-DOSSIER-PSB18","roles":"SYN-ROLES-PSB18","target":"SYN-CHANNEL-CN-EXPORT"}`
}

// Covers: AT-CC-437、AT-CC-439、AT-CC-440、AT-CC-442、AT-CC-441、AT-CC-122（生产装配半边）；AT-CC-107（版本冻结半边，发送归票 psb/10）——经生产端点：建案口按已接受
// 包裹建出案件（客户归属与来源引用取自 PS 那份已接受委托），重放答已有案件；授权确认之前提交执行答未决并指名授权未配置，不留
// 任何单元、版本或尝试；业务角色确认之后形成授权，重放答已有授权；提交执行固定提交版本，版本的授权依据带确认人，首次尝试是
// `已登记待发送`、库里 sent_at 为空；提交重放答已有版本，尝试与意图都不翻倍。
func TestCustomsCaseAuthorizationAndSubmissionRunThroughTheProductionFaces(t *testing.T) {
	fixture := newCustomsDecisionFixture(t)
	saveAcceptedLabelChannelRequest(t, fixture.db)
	fixture.registerCaseRequirement(t)
	fixture.registerReadiness(t, customsDecisionUnit)

	status, established := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBody)
	caseID, _ := established["caseId"].(string)
	if status != http.StatusCreated || established["outcome"] != "ESTABLISHED" || caseID == "" {
		t.Fatalf("建案：%d %v", status, established)
	}
	if n := fixture.count(t, `SELECT count(*) FROM customs_compliance.customs_case
		 WHERE tenant_id = $1 AND case_id = $2
		   AND parcels @> jsonb_build_array(jsonb_build_object('parcel', $3::text, 'customer', 'SYN-CUSTOMER-LC-RES',
		                                                       'sourceRef', 'SYN-REQUEST-LC-RES/SYN-VERSION-LC-RES'))`,
		customsDecisionTenant, caseID, customsDecisionParcel); n != 1 {
		t.Fatalf("案件关联的客户归属与来源引用不是 PS 那份已接受委托的：%d", n)
	}
	if n := fixture.count(t, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'customs-compliance.customs-case.established'`); n != 1 {
		t.Fatalf("案件建立意图 = %d, want 1", n)
	}
	status, replayed := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBody)
	if status != http.StatusOK || replayed["outcome"] != "EXISTING" || replayed["caseId"] != caseID {
		t.Fatalf("建案重放：%d %v", status, replayed)
	}

	status, unauthorized := fixture.post(t, "/customs-declaration-submissions", customsSubmitter, customsSubmissionBody(caseID))
	if status != http.StatusOK || unauthorized["outcome"] != "DECLARATION_UNDECIDED" || unauthorized["undecidedReason"] != "AUTHORITY_UNCONFIGURED" {
		t.Fatalf("授权前提交：%d %v", status, unauthorized)
	}
	if n := fixture.count(t, `SELECT (SELECT count(*) FROM customs_compliance.declaration_unit) + (SELECT count(*) FROM customs_compliance.declaration_submission)`); n != 0 {
		t.Fatalf("授权未配置却留下了 %d 行单元或版本", n)
	}

	status, confirmed := fixture.post(t, "/customs-submission-authorizations", customsApprover, `{"unitId":"`+customsDecisionUnit+`"}`)
	approverAuthority := "CUSTOMS_SUBMISSION_AUTHORIZATION@" + customsDecisionIssuer + "#" + customsApprover
	if status != http.StatusCreated || confirmed["outcome"] != "AUTHORIZATION_CONFIRMED" || confirmed["authority"] != approverAuthority {
		t.Fatalf("授权确认：%d %v", status, confirmed)
	}
	status, reconfirmed := fixture.post(t, "/customs-submission-authorizations", customsApprover, `{"unitId":"`+customsDecisionUnit+`"}`)
	if status != http.StatusOK || reconfirmed["outcome"] != "AUTHORIZATION_EXISTING" || reconfirmed["authority"] != approverAuthority {
		t.Fatalf("授权确认重放：%d %v", status, reconfirmed)
	}

	status, submitted := fixture.post(t, "/customs-declaration-submissions", customsSubmitter, customsSubmissionBody(caseID))
	versionID, _ := submitted["versionId"].(string)
	if status != http.StatusCreated || submitted["outcome"] != "DECLARATION_SUBMITTED" || versionID == "" ||
		submitted["authority"] != approverAuthority || submitted["attemptResult"] != "REGISTERED_PENDING_SEND" {
		t.Fatalf("提交执行：%d %v", status, submitted)
	}
	if _, stamped := submitted["attemptSentAt"]; stamped {
		t.Fatalf("没发出去的尝试报了发送时间：%v", submitted)
	}
	if n := fixture.count(t, `SELECT count(*) FROM customs_compliance.submission_attempt
		 WHERE tenant_id = $1 AND version_id = $2 AND result = 'REGISTERED_PENDING_SEND' AND sent_at IS NULL`,
		customsDecisionTenant, versionID); n != 1 {
		t.Fatalf("库里的首次尝试不是无发送时间的待发送：%d", n)
	}

	status, resubmitted := fixture.post(t, "/customs-declaration-submissions", customsSubmitter, customsSubmissionBody(caseID))
	if status != http.StatusOK || resubmitted["outcome"] != "EXISTING_VERSION" || resubmitted["versionId"] != versionID {
		t.Fatalf("提交重放：%d %v", status, resubmitted)
	}
	if n := fixture.count(t, `SELECT count(*) FROM customs_compliance.submission_attempt WHERE tenant_id = $1`, customsDecisionTenant); n != 1 {
		t.Fatalf("重放后尝试 = %d, want 1", n)
	}
	if n := fixture.count(t, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'customs-compliance.declaration-submission.formed'`); n != 1 {
		t.Fatalf("提交版本形成意图 = %d, want 1", n)
	}

	// —— 线下递交标记（dox/decisions.md「产品形态」）：没有对接报关行时，这一版在线下递交之后回来记一笔 ——
	const lodger = "SYN-LODGER-PSB18"
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	registerOperator(t, fixture.db, fixture.operators, lodger, interval, accessidentity.DecisionCustomsManualLodgement)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, fixture.operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	const lodgementFace = "/customs-declaration-manual-lodgements"
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*manualExternalFactsWiring)(nil)) {
			continue
		}
		mounted, err := feature.production(endpointSources{db: fixture.db}, endpointChannels{operatorDecisions: decisions})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		for _, candidate := range mounted {
			fixture.endpoints[candidate.Pattern] = candidate.Handler
		}
	}
	// 实际递交的时刻：晚于这一版成版、不晚于现在。早于成版的不受理。
	lodgedAt := time.Now().UTC().Truncate(time.Microsecond)
	lodgement := `{"version":"` + versionID + `","lodgedAt":"` + lodgedAt.Format(time.RFC3339Nano) + `","reference":"SYN-CUSTOMS-ACK-001","original":"SYN-LODGEMENT-SLIP-1"}`
	if status, answer := fixture.post(t, lodgementFace, lodger, `{"version":"`+versionID+`","lodgedAt":"`+lodgedAt.Add(-time.Hour).Format(time.RFC3339Nano)+`","original":"SYN-LODGEMENT-SLIP-1"}`); status != http.StatusOK ||
		answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
		t.Fatalf("递交时刻早于成版 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, lodgementFace, customsSubmitter, lodgement); status != http.StatusForbidden {
		t.Fatalf("持提交授予的人标记递交 = %d %v，想要 403", status, answer)
	}
	if status, answer := fixture.post(t, lodgementFace, lodger, `{"version":"`+versionID+`","lodgedAt":"`+time.Now().UTC().Add(time.Hour).Format(time.RFC3339)+`","original":"SYN-LODGEMENT-SLIP-1"}`); status != http.StatusOK ||
		answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
		t.Fatalf("递交时刻在未来 = %d %v", status, answer)
	}
	// 两个人同时标记同一版：恰有一人记下，另一人答已发过，交回同一笔。
	type lodgementReply struct {
		status int
		answer map[string]any
	}
	replies := make(chan lodgementReply, 2)
	for range 2 {
		go func() {
			status, answer := fixture.post(t, lodgementFace, lodger, lodgement)
			replies <- lodgementReply{status, answer}
		}()
	}
	outcomes := map[string]int{}
	for range 2 {
		reply := <-replies
		outcomes[reply.answer["outcome"].(string)]++
		if reply.answer["result"] != "PENDING_CONFIRMATION" || reply.answer["sentAt"] != lodgedAt.Format(time.RFC3339Nano) {
			t.Fatalf("标记答复 = %d %v", reply.status, reply.answer)
		}
	}
	if outcomes["LODGEMENT_RECORDED"] != 1 || outcomes["DECLARATION_ALREADY_SENT"] != 1 {
		t.Fatalf("两人同时标记 = %v", outcomes)
	}
	if n := fixture.count(t, `SELECT count(*) FROM customs_compliance.submission_manual_lodgement
		 WHERE tenant_id = $1 AND version_id = $2 AND original_ref = 'SYN-LODGEMENT-SLIP-1' AND reference = 'SYN-CUSTOMS-ACK-001' AND lodged_by = $3`,
		customsDecisionTenant, versionID, "OPERATOR/"+customsDecisionIssuer+"#"+lodger); n != 1 {
		t.Fatalf("线下递交留痕 = %d 条，想要 1", n)
	}
	if n := fixture.count(t, `SELECT count(*) FROM customs_compliance.submission_send_fact WHERE tenant_id = $1 AND version_id = $2 AND channel_message IS NULL`, customsDecisionTenant, versionID); n != 1 {
		t.Fatalf("发送事实 = %d 笔，想要 1", n)
	}
}

// Covers: AT-CC-445、AT-CC-112（生产装配半边）——三口各按自己那一种决定认证：建案官、批准人、提交人互相用不了别人的口，答
// OPERATOR_NOT_GRANTED，什么都不写；没带令牌答 401。
func TestEachCustomsFaceOpensOnlyForItsOwnDecisionKind(t *testing.T) {
	fixture := newCustomsDecisionFixture(t)
	saveAcceptedLabelChannelRequest(t, fixture.db)
	fixture.registerCaseRequirement(t)
	fixture.registerReadiness(t, customsDecisionUnit)

	for _, attempt := range []struct{ pattern, operator, body string }{
		{"/customs-cases", customsApprover, customsCaseBody},
		{"/customs-cases", customsSubmitter, customsCaseBody},
		{"/customs-submission-authorizations", customsSubmitter, `{"unitId":"` + customsDecisionUnit + `"}`},
		{"/customs-submission-authorizations", customsCaseOfficer, `{"unitId":"` + customsDecisionUnit + `"}`},
		{"/customs-declaration-submissions", customsApprover, customsSubmissionBody("CASE-ANY")},
		{"/customs-declaration-submissions", customsCaseOfficer, customsSubmissionBody("CASE-ANY")},
	} {
		status, answer := fixture.post(t, attempt.pattern, attempt.operator, attempt.body)
		code, _ := answer["error"].(map[string]any)
		if status != http.StatusForbidden || code["code"] != "OPERATOR_NOT_GRANTED" {
			t.Fatalf("%s 持 %s：%d %v", attempt.pattern, attempt.operator, status, answer)
		}
	}
	status, _ := fixture.post(t, "/customs-cases", "", customsCaseBody)
	if status != http.StatusUnauthorized {
		t.Fatalf("没令牌：%d", status)
	}
	if n := fixture.count(t, `SELECT (SELECT count(*) FROM customs_compliance.customs_case) + (SELECT count(*) FROM customs_compliance.submission_authority)`); n != 0 {
		t.Fatalf("越权请求写下了 %d 行", n)
	}
}

// Covers: AT-CC-438、AT-CC-444（生产装配半边）——前提不足各格如实作答、不写：包裹不在已接受委托里答 PARCEL_NOT_ACCEPTED 并列出；
// 建案要求规则没登答 UNDECIDED、指名 CASE_REQUIREMENT_UNCONFIGURED；单元就绪没登时确认答 AUTHORIZATION_UNDECIDED、指名
// READINESS_UNCONFIGURED。
func TestUnconfiguredCustomsRegistersAnswerNotConfiguredThroughTheProductionFaces(t *testing.T) {
	fixture := newCustomsDecisionFixture(t)

	status, unaccepted := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBody)
	parcels, _ := unaccepted["unacceptedParcels"].([]any)
	if status != http.StatusOK || unaccepted["outcome"] != "PARCEL_NOT_ACCEPTED" || len(parcels) != 1 || parcels[0] != customsDecisionParcel {
		t.Fatalf("未接受包裹：%d %v", status, unaccepted)
	}

	saveAcceptedLabelChannelRequest(t, fixture.db)
	status, unconfigured := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBody)
	if status != http.StatusOK || unconfigured["outcome"] != "UNDECIDED" || unconfigured["undecidedReason"] != "CASE_REQUIREMENT_UNCONFIGURED" {
		t.Fatalf("建案要求未登：%d %v", status, unconfigured)
	}

	status, unready := fixture.post(t, "/customs-submission-authorizations", customsApprover, `{"unitId":"`+customsDecisionUnit+`"}`)
	if status != http.StatusOK || unready["outcome"] != "AUTHORIZATION_UNDECIDED" || unready["undecidedReason"] != "READINESS_UNCONFIGURED" {
		t.Fatalf("就绪未登：%d %v", status, unready)
	}
	if n := fixture.count(t, `SELECT (SELECT count(*) FROM customs_compliance.customs_case) + (SELECT count(*) FROM customs_compliance.submission_authority)`); n != 0 {
		t.Fatalf("未配置时写下了 %d 行", n)
	}
}

// Covers: AT-CC-445（生产装配半边：授予的时效）——三种授予过期或被撤销之后，三口都答 OPERATOR_NOT_GRANTED、不写：授予只在它
// 生效的区间里开门，撤销之后的出示与从未授予同答。
func TestExpiredOrRevokedCustomsGrantsNoLongerOpenTheFaces(t *testing.T) {
	fixture := newCustomsDecisionFixture(t)
	saveAcceptedLabelChannelRequest(t, fixture.db)
	fixture.registerCaseRequirement(t)
	fixture.registerReadiness(t, customsDecisionUnit)

	now := time.Now()
	kinds := []accessidentity.DecisionKind{
		accessidentity.DecisionCustomsCaseEstablishment,
		accessidentity.DecisionCustomsSubmissionAuthorization,
		accessidentity.DecisionCustomsDeclarationSubmission,
	}
	expired, err := accessidentity.NewEffectiveInterval(now.Add(-2*time.Hour), now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	registerOperator(t, fixture.db, fixture.operators, "SYN-EXPIRED-PSB18", expired, kinds...)

	open, err := accessidentity.NewEffectiveInterval(now.Add(-2*time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grantIDs := registerOperator(t, fixture.db, fixture.operators, "SYN-REVOKED-PSB18", open, kinds...)
	var revoked []accessidentity.OperatorRegistrationOutcome
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		for _, id := range grantIDs {
			revocation, err := accessidentity.NewGrantRevocation(customsDecisionTenant, id, now.Add(-time.Minute), "SYN-REVOKE-BASIS-PSB18")
			if err != nil {
				return err
			}
			outcome, err := fixture.operators.RegisterRevocation(txCtx, revocation)
			if err != nil {
				return err
			}
			revoked = append(revoked, outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("撤销授予：%v", err)
	}
	for _, outcome := range revoked {
		if outcome != accessidentity.OperatorRegistrationRecorded {
			t.Fatalf("撤销授予：%v", revoked)
		}
	}

	for _, operator := range []string{"SYN-EXPIRED-PSB18", "SYN-REVOKED-PSB18"} {
		for pattern, body := range map[string]string{
			"/customs-cases":                     customsCaseBody,
			"/customs-submission-authorizations": `{"unitId":"` + customsDecisionUnit + `"}`,
			"/customs-declaration-submissions":   customsSubmissionBody("CASE-ANY"),
		} {
			status, answer := fixture.post(t, pattern, operator, body)
			code, _ := answer["error"].(map[string]any)
			if status != http.StatusForbidden || code["code"] != "OPERATOR_NOT_GRANTED" {
				t.Fatalf("%s 由 %s 出示：%d %v", pattern, operator, status, answer)
			}
		}
	}
	if n := fixture.count(t, `SELECT (SELECT count(*) FROM customs_compliance.customs_case) + (SELECT count(*) FROM customs_compliance.submission_authority)
		+ (SELECT count(*) FROM customs_compliance.declaration_submission)`); n != 0 {
		t.Fatalf("过期或撤销的授予写下了 %d 行", n)
	}
}

// exec 在一笔事务里执行一句写，要求恰好动了一行——用例直接在库里摆出「换格式之前已发出的号」那种现场。
func (fixture *customsDecisionFixture) exec(t *testing.T, statement string, args ...any) {
	t.Helper()
	var affected int64
	err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		executor, err := fixture.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		tag, err := executor.Exec(txCtx, statement, args...)
		affected = tag.RowsAffected()
		return err
	})
	if err != nil || affected != 1 {
		t.Fatalf("%s：%v（动了 %d 行）", statement, err, affected)
	}
}

// customsCaseBodyFor 是同一件已接受包裹、换一个法定义务范围的建案载荷——范围键换了就是另一个案件。
func customsCaseBodyFor(obligation string) string {
	return strings.Replace(customsCaseBody, `"obligation":"SYN-OBL-CN-EXPORT"`, `"obligation":"`+obligation+`"`, 1)
}

// registerCaseNumbering 经装进建案编排的那一套编号组件为本用例的租户登单据编号时区与一版不带日期的关务案件规则：
// 号不随测试跑的日子变，撞号用例才能事先占住要撞的那一段。
func (fixture *customsDecisionFixture) registerCaseNumbering(t *testing.T) {
	t.Helper()
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	zone, err := fixture.numbers.Registrations.RegisterTimeZone(t.Context(), docnumber.RegisterTimeZoneCommand{
		Tenant: customsDecisionTenant, TimeZone: "Asia/Shanghai", EffectiveFrom: since, RegisteredBy: "SYN-TEST-CC07",
	})
	if err != nil || zone.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登单据编号时区：%+v %v", zone, err)
	}
	rule, err := fixture.numbers.Registrations.RegisterRule(t.Context(), docnumber.RegisterRuleCommand{
		Tenant: customsDecisionTenant, Type: ccdomain.CustomsCaseDocumentNumberType.Code, Template: "SYN-CC-{seq:4}",
		EffectiveFrom: since, RegisteredBy: "SYN-TEST-CC07",
	})
	if err != nil || rule.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登关务案件规则：%+v %v", rule, err)
	}
}

// Covers: AT-CC-459、AT-CC-460、AT-CC-461（生产装配半边）；AT-CC-462（生产装配的并发半边）——经生产建案口：租户没登单据编号时区时
// 照常建案、不带号并答无号情形；登了之后建案在同一事务里取号、答复带号，重放答原号；渲染出的号一路撞到上限答编号规则撞号、
// 说人话指到规则册、不落案件，事务壳回滚、流水释放（腾出一个号后下一次建案取到的正是它）；同一范围并发建案只落一个案件、
// 输家答原号，之后的新案件号段连续。
func TestACaseIsNumberedInItsEstablishingTransactionThroughTheProductionFace(t *testing.T) {
	fixture := newCustomsDecisionFixture(t)
	saveAcceptedLabelChannelRequest(t, fixture.db)
	fixture.registerCaseRequirement(t)

	status, unnumbered := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBodyFor("SYN-OBL-CC07-0"))
	if _, numbered := unnumbered["documentNumber"]; status != http.StatusCreated || unnumbered["outcome"] != "ESTABLISHED" || numbered ||
		unnumbered["documentNumberAbsence"] != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("没登时区的建案：%d %v", status, unnumbered)
	}

	fixture.registerCaseNumbering(t)
	status, established := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBodyFor("SYN-OBL-CC07-1"))
	if status != http.StatusCreated || established["documentNumber"] != "SYN-CC-0001" {
		t.Fatalf("建案取号：%d %v", status, established)
	}
	if _, absent := established["documentNumberAbsence"]; absent {
		t.Fatalf("带号的答复还带了无号情形：%v", established)
	}
	status, replayed := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBodyFor("SYN-OBL-CC07-1"))
	if status != http.StatusOK || replayed["outcome"] != "EXISTING" || replayed["documentNumber"] != "SYN-CC-0001" {
		t.Fatalf("重放：%d %v", status, replayed)
	}
	status, stillUnnumbered := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBodyFor("SYN-OBL-CC07-0"))
	if _, numbered := stillUnnumbered["documentNumber"]; status != http.StatusOK || numbered ||
		stillUnnumbered["documentNumberAbsence"] != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("补登时区之后旧案件被补了号：%d %v", status, stillUnnumbered)
	}

	// 换格式之前已发出的号恰好占住 SYN-CC-0002 起的整整一段。
	for i := 2; i <= docnumber.MaxSkippedNumbers+1; i++ {
		fixture.exec(t, `INSERT INTO customs_compliance.customs_case
			    (tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at, document_number)
			VALUES ($1, 'SYN-JUR-OLD', 'EXPORT', 'SYN-PROC-OLD', $2, $2,
			        '[{"parcel":"P-OLD","customer":"SYN-CUSTOMER-LC-RES","sourceRef":"PS/P-OLD"}]', '[]', now(), $3)`,
			customsDecisionTenant, fmt.Sprintf("SYN-OLD-%02d", i), fmt.Sprintf("SYN-CC-%04d", i))
	}
	status, collision := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBodyFor("SYN-OBL-CC07-2"))
	detail, _ := collision["detail"].(string)
	if status != http.StatusOK || collision["outcome"] != "NUMBERING_RULE_COLLISION" || !strings.Contains(detail, "设置 → 单据编号") {
		t.Fatalf("撞号：%d %v", status, collision)
	}
	if _, present := collision["caseId"]; present {
		t.Fatalf("撞号的答复带了案件：%v", collision)
	}
	if n := fixture.count(t, `SELECT count(*) FROM customs_compliance.customs_case WHERE tenant_id = $1 AND obligation_ref = 'SYN-OBL-CC07-2'`,
		customsDecisionTenant); n != 0 {
		t.Fatalf("撞号落了 %d 个案件", n)
	}
	fixture.exec(t, `DELETE FROM customs_compliance.customs_case WHERE tenant_id = $1 AND document_number = 'SYN-CC-0009'`,
		customsDecisionTenant)
	status, freed := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBodyFor("SYN-OBL-CC07-2"))
	if status != http.StatusCreated || freed["documentNumber"] != "SYN-CC-0009" {
		t.Fatalf("撞号回滚没释放流水（应从 2 起跳到腾出的 0009）：%d %v", status, freed)
	}

	const racers = 4
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		answers []map[string]any
	)
	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			request := httptest.NewRequest(http.MethodPost, "/customs-cases", strings.NewReader(customsCaseBodyFor("SYN-OBL-CC07-RACE")))
			request.Header.Set("Authorization", "Bearer "+customsCaseOfficer)
			response := httptest.NewRecorder()
			fixture.endpoints["/customs-cases"].ServeHTTP(response, request)
			var answer map[string]any
			_ = json.Unmarshal(response.Body.Bytes(), &answer)
			mu.Lock()
			answers = append(answers, answer)
			mu.Unlock()
		}()
	}
	wg.Wait()
	established = nil
	for _, answer := range answers {
		if answer["outcome"] == "ESTABLISHED" {
			if established != nil {
				t.Fatalf("同一范围并发建出了两个案件：%v", answers)
			}
			established = answer
		} else if answer["outcome"] != "EXISTING" {
			t.Fatalf("并发建案：%v", answers)
		}
	}
	for _, answer := range answers {
		if established == nil || answer["documentNumber"] != established["documentNumber"] {
			t.Fatalf("并发建案答了不同的号：%v", answers)
		}
	}
	status, next := fixture.post(t, "/customs-cases", customsCaseOfficer, customsCaseBodyFor("SYN-OBL-CC07-NEXT"))
	if raced, _ := established["documentNumber"].(string); status != http.StatusCreated || raced != "SYN-CC-0022" || next["documentNumber"] != "SYN-CC-0023" {
		t.Fatalf("并发输家没释放流水：并发那一个 %v，下一个 %d %v", established["documentNumber"], status, next)
	}
}
