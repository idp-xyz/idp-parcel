package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
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
}

// registerOperator 把一个合成操作者绑到演示委托所在的租户，并按给定区间逐种登运营决定授予，交回各笔授予号。断言在事务闭包外。
func registerOperator(t *testing.T, db *bentopg.DB, operators *aipg.OperatorRegistry, subject string,
	interval accessidentity.EffectiveInterval, kinds ...accessidentity.DecisionKind) []string {
	t.Helper()
	operator, err := accessidentity.NewOperatorSubject(customsDecisionIssuer, subject)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewOperatorBinding(operator, customsDecisionTenant, "SYN-BIND-BASIS-PSB18")
	if err != nil {
		t.Fatal(err)
	}
	var grants []accessidentity.OperatorGrant
	var ids []string
	for _, kind := range kinds {
		id := "SYN-GRANT-" + subject + "-" + kind.String()
		grant, err := accessidentity.NewOperationDecisionGrant(customsDecisionTenant, id, operator, kind, interval, "SYN-GRANT-BASIS-PSB18")
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
	faces, err := buildCustomsDecisionFaces(db)
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
	return &customsDecisionFixture{db: db, operators: operators, endpoints: endpoints}
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

// Covers: AT-CC-437、AT-CC-439、AT-CC-440、AT-CC-442、AT-CC-441、AT-CC-107、AT-CC-122（生产装配半边）——经生产端点：建案口按已接受
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
