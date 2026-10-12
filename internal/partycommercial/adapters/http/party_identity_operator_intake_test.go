package commercialhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
)

// 本文件对操作者渠道在参与方身份族各口的 Intake（ADR-0100 决定四；票 operator-channel/04）证传输面：租户格只来自认证
// 结果、行内容只从载荷取、自报租户即拒、形状错即拒、令牌不过时载荷一个字节都不读，渠道各格经端点答到线上。
//
// 载荷形状照票 02 的表单（`legalEntities[0]` 五格，镜像受控 CLI `register-parties` 的一项、去掉整批的 tenantId）。
// 期望值取自载荷字面量而不是重算：这里证的是「译装没换字」，不是译装怎么算。

const presentedToken = "presented.operator.token"

// identityRequest 发一次持令牌的登记请求。请求头与查询串里另带自报租户：它们一律被无视，租户只取认证结果。
func identityRequest(target, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, target+"?tenant=TENANT-9", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+presentedToken)
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	return request
}

func legalEntityRequest(body string) *http.Request {
	return identityRequest("/commercial-legal-entity-registrations", body)
}

// identityLine 是身份族的一口：Intake 方法与一份本口合规的外壳正文（不带外层花括号、不带 tenantId）。
type identityLine struct {
	target string
	intake func(*commercialhttp.OperatorRegistryIntake, *http.Request) error
	fields string
}

var identityLines = map[string]identityLine{
	"业务参与方": {
		target: "/commercial-business-party-registrations",
		intake: func(intake *commercialhttp.OperatorRegistryIntake, request *http.Request) error {
			_, err := intake.IntakeBusinessPartyRegistration(context.Background(), request)
			return err
		},
		fields: `"businessParties":[{"partyId":"SYN-PARTY-02","name":"n","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}]`,
	},
	"责任法人": {
		target: "/commercial-legal-entity-registrations",
		intake: func(intake *commercialhttp.OperatorRegistryIntake, request *http.Request) error {
			_, err := intake.IntakeLegalEntityRegistration(context.Background(), request)
			return err
		},
		fields: `"legalEntities":[{"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}]`,
	},
	"货主客户账户": {
		target: "/commercial-customer-account-registrations",
		intake: func(intake *commercialhttp.OperatorRegistryIntake, request *http.Request) error {
			_, err := intake.IntakeCustomerAccountRegistration(context.Background(), request)
			return err
		},
		fields: `"customerAccounts":[{"accountId":"SYN-ACCT-02","customerPartyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}]`,
	},
	"参与方关系": {
		target: "/commercial-party-relationship-registrations",
		intake: func(intake *commercialhttp.OperatorRegistryIntake, request *http.Request) error {
			_, err := intake.IntakePartyRelationshipRegistration(context.Background(), request)
			return err
		},
		fields: `"relationships":[{"relationshipId":"SYN-REL-02","revision":1,"holder":"SYN-PARTY-02","counterparty":"SYN-PARTY-03",` +
			`"role":"CUSTOMER","scope":"s","basis":"b","effectiveStartsAt":"2026-09-01T00:00:00Z"}]`,
	},
	"身份停用": {
		target: "/commercial-party-identity-deactivations",
		intake: func(intake *commercialhttp.OperatorRegistryIntake, request *http.Request) error {
			_, err := intake.IntakePartyIdentityDeactivation(context.Background(), request)
			return err
		},
		fields: `"deactivations":[{"kind":"LEGAL_ENTITY","id":"SYN-LE-02","revision":2,"basis":"b","at":"2026-09-17T00:00:00Z"}]`,
	},
	"法人资料": {
		target: "/commercial-legal-entity-profile-registrations",
		intake: func(intake *commercialhttp.OperatorRegistryIntake, request *http.Request) error {
			_, err := intake.IntakeLegalEntityProfileRegistration(context.Background(), request)
			return err
		},
		fields: `"profiles":[{"legalEntityId":"SYN-LE-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}]`,
	},
}

// identityEndpoints 以同一个 Intake 装身份族各口的生产端点构造函数；编排是「被调即失败」的替身，资料口的替身由调用方给，
// 事后查它被调过没有。
func identityEndpoints(t *testing.T, intake *commercialhttp.OperatorRegistryIntake, profiles commercialhttp.LegalEntityProfileRegistrar) map[string]http.Handler {
	t.Helper()
	parties := unreachablePartyRegistrar{t: t}
	return map[string]http.Handler{
		"/commercial-business-party-registrations":       commercialhttp.NewRegisterBusinessPartyEndpoint(intake, parties),
		"/commercial-legal-entity-registrations":         commercialhttp.NewRegisterLegalEntityEndpoint(intake, parties),
		"/commercial-customer-account-registrations":     commercialhttp.NewRegisterCustomerAccountEndpoint(intake, parties),
		"/commercial-party-relationship-registrations":   commercialhttp.NewRegisterPartyRelationshipEndpoint(intake, parties),
		"/commercial-party-identity-deactivations":       commercialhttp.NewDeactivatePartyIdentityEndpoint(intake, parties),
		"/commercial-legal-entity-profile-registrations": commercialhttp.NewRegisterLegalEntityProfileEndpoint(intake, profiles),
	}
}

// Covers: register_party_identity.go 包注释「真渠道 Intake 要把认证结果填进租户格、只从载荷取行内容」——租户格是认证结果，
// 五格行内容逐字来自载荷，请求头与查询串里的自报一律无视。
func TestOperatorRegistryIntakeTranslatesLegalEntityRegistrationUnderTheAuthenticatedTenant(t *testing.T) {
	command, err := operatorIntake(t).IntakeLegalEntityRegistration(context.Background(), legalEntityRequest(`{"legalEntities":[{
		"legalEntityId":"SYN-LE-02",
		"partyId":"SYN-PARTY-02",
		"revision":3,
		"basis":"SYN-BASIS/le-02-r3",
		"effectiveFrom":"2026-09-16T08:00:00+08:00"
	}]}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.Tenant.String(); got != operatorAuthenticatedTenant {
		t.Fatalf("Tenant = %q, want %q（认证结果，不是请求里的自报）", got, operatorAuthenticatedTenant)
	}
	if got := command.Entity.String(); got != "SYN-LE-02" {
		t.Fatalf("Entity = %q, want SYN-LE-02", got)
	}
	if got := command.Party.String(); got != "SYN-PARTY-02" {
		t.Fatalf("Party = %q, want SYN-PARTY-02", got)
	}
	if command.Revision != 3 {
		t.Fatalf("Revision = %d, want 3", command.Revision)
	}
	if got := command.Basis.String(); got != "SYN-BASIS/le-02-r3" {
		t.Fatalf("Basis = %q, want SYN-BASIS/le-02-r3", got)
	}
	wantEffective := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	if !command.EffectiveFrom.Equal(wantEffective) {
		t.Fatalf("EffectiveFrom = %s, want %s（RFC 3339 带时区偏移，按绝对时刻比）", command.EffectiveFrom, wantEffective)
	}
}

// Covers: 身份族各口载荷里带 tenantId 即拒（MALFORMED_REQUEST，理由点名 tenantId）。值与认证结果相同也拒，否则「碰巧相同」的
// 载荷会在别的环境里变成穿透 ADR-0003 隔离边界的第一步；null 也是自报。同一份载荷去掉那一格照常译出——拒的是那一格，不是这口。
func TestPartyIdentityLinesRefuseASelfReportedTenant(t *testing.T) {
	intake := operatorIntake(t)
	for name, line := range identityLines {
		t.Run(name, func(t *testing.T) {
			if err := line.intake(intake, identityRequest(line.target, `{`+line.fields+`}`)); err != nil {
				t.Fatalf("不带 tenantId 的合规载荷：%v", err)
			}
			for variant, tenant := range map[string]string{
				"另一租户":    `"SYN-TEN-OTHER"`,
				"与认证结果同值": `"` + operatorAuthenticatedTenant + `"`,
				"null":    `null`,
			} {
				err := line.intake(intake, identityRequest(line.target, `{"tenantId":`+tenant+`,`+line.fields+`}`))
				if !errors.Is(err, commercialhttp.ErrMalformedRequest) || !strings.Contains(err.Error(), "tenantId") {
					t.Fatalf("%s：err = %v, want ErrMalformedRequest 且点名 tenantId", variant, err)
				}
			}
		})
	}
}

// Covers: OperatorRegistryIntake 类型注释「先认证、后解载荷」——令牌不过时身份族各口一个字节都不读，答凭证被拒。
func TestPartyIdentityLinesAuthenticateBeforeReadingThePayload(t *testing.T) {
	intake := operatorIntake(t)
	for name, line := range identityLines {
		probe := &bodyReadProbe{}
		err := line.intake(intake, httptest.NewRequest(http.MethodPost, line.target, probe))
		if !errors.Is(err, commercialhttp.ErrOperatorCredentialRejected) || probe.read {
			t.Fatalf("%s：err = %v（载荷读了：%v），want ErrOperatorCredentialRejected 且一字未读", name, err, probe.read)
		}
	}
}

// Covers: ADR-0100 决定四的三格与未配置格经身份族各口的端点答到线上（writeRegistrationIntakeProblem）——不带令牌 401、
// 没有登记册配置写授予 403、身份依赖读不动 503、发行方未配置 403 未配置。四格都在读载荷之前定：编排一次也不被调到，
// 答复不带 outcome。
func TestPartyIdentityEndpointsAnswerTheOperatorChannelGrades(t *testing.T) {
	grades := map[string]struct {
		operators envelopeOperators
		token     string
		status    int
		code      string
	}{
		"不带令牌":    {envelopeOperators{}, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"没有登记写授予": {envelopeOperators{err: commercialhttp.ErrOperatorNotGranted}, presentedToken, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"身份依赖读不动": {envelopeOperators{err: commercialhttp.ErrIdentityDependencyUnavailable}, presentedToken, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"发行方未配置":  {envelopeOperators{err: commercialhttp.ErrAccessChannelNotConfigured}, presentedToken, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
	}
	for grade, testCase := range grades {
		t.Run(grade, func(t *testing.T) {
			profiles := &legalEntityProfileRegistrarDouble{}
			for target, endpoint := range identityEndpoints(t, publicationOperatorIntake(t, testCase.operators), profiles) {
				probe := &bodyReadProbe{}
				request := httptest.NewRequest(http.MethodPost, target, probe)
				if testCase.token != "" {
					request.Header.Set("Authorization", "Bearer "+testCase.token)
				}
				recorder := httptest.NewRecorder()
				endpoint.ServeHTTP(recorder, request)
				if recorder.Code != testCase.status || errorCode(t, recorder) != testCase.code {
					t.Fatalf("%s：答 %d %s，want %d %s", target, recorder.Code, recorder.Body, testCase.status, testCase.code)
				}
				assertNoRegistrationOutcome(t, recorder)
				if probe.read {
					t.Fatalf("%s：认证没过却读了载荷", target)
				}
			}
			if profiles.called {
				t.Fatal("资料登记编排被调到了")
			}
		})
	}
}

// Covers: 形状级失败包 ErrMalformedRequest（4xx，重发同样内容不会好），判据同 ADR-0029。一次一笔：本端点一次只收一项，
// 批走受控 CLI；零项与多项都不是这一口的形状。空标识由领域值对象拒，Intake 不替它放行一个立不住的命令。
func TestOperatorRegistryIntakeRefusesMalformedLegalEntityPayloads(t *testing.T) {
	intake := operatorIntake(t)
	item := func(id string) string {
		return `{"legalEntityId":"` + id + `","partyId":"SYN-PARTY-02","revision":1,"basis":"SYN-BASIS/le-02","effectiveFrom":"2026-09-16T00:00:00Z"}`
	}
	for name, body := range map[string]string{
		"不是 JSON": "!!not-json!!",
		"空载荷":     "",
		"零项":      `{"legalEntities":[]}`,
		"缺键":      `{}`,
		"两项":      `{"legalEntities":[` + item("SYN-LE-02") + `,` + item("SYN-LE-03") + `]}`,
		"未知字段":    `{"legalEntities":[{"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z","name":"x"}]}`,
		"空法人标识":   `{"legalEntities":[` + item("") + `]}`,
		"时刻不是时刻":  `{"legalEntities":[{"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"昨天"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeLegalEntityRegistration(context.Background(), legalEntityRequest(body))
			if !errors.Is(err, commercialhttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

// Covers: business-party 一口——`businessParties[0]` 四格 + 生效时刻逐字来自载荷，租户格来自认证结果；参与方本体
// （BusinessParty）由领域构造函数在 Intake 里立起来，用例侧不再重构。
func TestOperatorRegistryIntakeTranslatesBusinessPartyRegistrationUnderTheAuthenticatedTenant(t *testing.T) {
	command, err := operatorIntake(t).IntakeBusinessPartyRegistration(context.Background(),
		identityRequest("/commercial-business-party-registrations", `{"businessParties":[{
		"partyId":"SYN-PARTY-02",
		"name":"SYN 合成参与方二号",
		"revision":2,
		"basis":"SYN-BASIS/party-02-r2",
		"effectiveFrom":"2026-09-16T00:00:00Z"
	}]}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.Party.Tenant().String(); got != operatorAuthenticatedTenant {
		t.Fatalf("Party.Tenant = %q, want %q（认证结果）", got, operatorAuthenticatedTenant)
	}
	if got := command.Party.ID().String(); got != "SYN-PARTY-02" {
		t.Fatalf("Party.ID = %q, want SYN-PARTY-02", got)
	}
	if got := command.Party.Name().String(); got != "SYN 合成参与方二号" {
		t.Fatalf("Party.Name = %q, want SYN 合成参与方二号", got)
	}
	if command.Revision != 2 {
		t.Fatalf("Revision = %d, want 2", command.Revision)
	}
	if got := command.Basis.String(); got != "SYN-BASIS/party-02-r2" {
		t.Fatalf("Basis = %q, want SYN-BASIS/party-02-r2", got)
	}
	if want := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC); !command.EffectiveFrom.Equal(want) {
		t.Fatalf("EffectiveFrom = %s, want %s", command.EffectiveFrom, want)
	}
}

// Covers: customer-account 一口——`customerAccounts[0]` 五格逐字来自载荷，租户格来自认证结果；跨租户绑定由领域构造门
// 在用例侧拒（ADR-0041），Intake 不预判。
func TestOperatorRegistryIntakeTranslatesCustomerAccountRegistrationUnderTheAuthenticatedTenant(t *testing.T) {
	command, err := operatorIntake(t).IntakeCustomerAccountRegistration(context.Background(),
		identityRequest("/commercial-customer-account-registrations", `{"customerAccounts":[{
		"accountId":"SYN-ACCT-02",
		"customerPartyId":"SYN-PARTY-02",
		"revision":1,
		"basis":"SYN-BASIS/acct-02",
		"effectiveFrom":"2026-09-16T00:00:00Z"
	}]}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.Tenant.String(); got != operatorAuthenticatedTenant {
		t.Fatalf("Tenant = %q, want %q（认证结果）", got, operatorAuthenticatedTenant)
	}
	if got := command.Account.String(); got != "SYN-ACCT-02" {
		t.Fatalf("Account = %q, want SYN-ACCT-02", got)
	}
	if got := command.CustomerParty.String(); got != "SYN-PARTY-02" {
		t.Fatalf("CustomerParty = %q, want SYN-PARTY-02", got)
	}
	if command.Revision != 1 {
		t.Fatalf("Revision = %d, want 1", command.Revision)
	}
	if got := command.Basis.String(); got != "SYN-BASIS/acct-02" {
		t.Fatalf("Basis = %q, want SYN-BASIS/acct-02", got)
	}
	if want := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC); !command.EffectiveFrom.Equal(want) {
		t.Fatalf("EffectiveFrom = %s, want %s", command.EffectiveFrom, want)
	}
}

// Covers: party-relationship 一口——`relationships[0]` 正文沿 domain.PartyRelationshipSpec 逐格来自载荷，租户格来自认证
// 结果；批准事实与区间终点两格可缺席：缺席即候选关系 / 开区间，不造假值顶上（CLI 同形）。角色是封闭集的名称镜像，
// 集合外取值拒收不吸收。
func TestOperatorRegistryIntakeTranslatesPartyRelationshipRegistrationUnderTheAuthenticatedTenant(t *testing.T) {
	intake := operatorIntake(t)
	relationshipRequest := func(body string) *http.Request {
		return identityRequest("/commercial-party-relationship-registrations", body)
	}

	approved, err := intake.IntakePartyRelationshipRegistration(context.Background(), relationshipRequest(`{"relationships":[{
		"relationshipId":"SYN-REL-02",
		"revision":1,
		"holder":"SYN-PARTY-02",
		"counterparty":"SYN-PARTY-03",
		"role":"CUSTOMER",
		"scope":"SYN-SCOPE/rel-02",
		"basis":"SYN-BASIS/rel-02",
		"effectiveStartsAt":"2026-09-01T00:00:00Z",
		"effectiveEndsAt":"2027-09-01T00:00:00Z",
		"approval":{"reference":"SYN-APPROVAL/rel-02","approvedAt":"2026-08-30T00:00:00Z"}
	}]}`))
	if err != nil {
		t.Fatalf("intake（已批准）：%v", err)
	}
	if got := approved.Tenant.String(); got != operatorAuthenticatedTenant {
		t.Fatalf("Tenant = %q, want %q（认证结果）", got, operatorAuthenticatedTenant)
	}
	if got := approved.ID.String(); got != "SYN-REL-02" {
		t.Fatalf("ID = %q, want SYN-REL-02", got)
	}
	if approved.Revision != 1 {
		t.Fatalf("Revision = %d, want 1", approved.Revision)
	}
	if got := approved.Spec.Holder.String(); got != "SYN-PARTY-02" {
		t.Fatalf("Holder = %q, want SYN-PARTY-02", got)
	}
	if got := approved.Spec.Counterparty.String(); got != "SYN-PARTY-03" {
		t.Fatalf("Counterparty = %q, want SYN-PARTY-03", got)
	}
	if got := approved.Spec.Role.String(); got != "CUSTOMER" {
		t.Fatalf("Role = %q, want CUSTOMER", got)
	}
	if got := approved.Spec.Scope.String(); got != "SYN-SCOPE/rel-02" {
		t.Fatalf("Scope = %q, want SYN-SCOPE/rel-02", got)
	}
	if got := approved.Spec.Basis.String(); got != "SYN-BASIS/rel-02" {
		t.Fatalf("Basis = %q, want SYN-BASIS/rel-02", got)
	}
	if want := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC); !approved.Spec.Effective.StartsAt().Equal(want) {
		t.Fatalf("Effective.StartsAt = %s, want %s", approved.Spec.Effective.StartsAt(), want)
	}
	endsAt, bounded := approved.Spec.Effective.EndsAt()
	if want := time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC); !bounded || !endsAt.Equal(want) {
		t.Fatalf("Effective.EndsAt = %s/%v, want %s", endsAt, bounded, want)
	}
	if approved.Approval == nil {
		t.Fatal("Approval = nil，载荷带了批准事实")
	}
	if got := approved.Approval.Reference.String(); got != "SYN-APPROVAL/rel-02" {
		t.Fatalf("Approval.Reference = %q, want SYN-APPROVAL/rel-02", got)
	}
	if want := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC); !approved.Approval.ApprovedAt.Equal(want) {
		t.Fatalf("Approval.ApprovedAt = %s, want %s", approved.Approval.ApprovedAt, want)
	}

	candidate, err := intake.IntakePartyRelationshipRegistration(context.Background(), relationshipRequest(`{"relationships":[{
		"relationshipId":"SYN-REL-03",
		"revision":1,
		"holder":"SYN-PARTY-02",
		"counterparty":"SYN-PARTY-03",
		"role":"SUPPLIER",
		"scope":"SYN-SCOPE/rel-03",
		"basis":"SYN-BASIS/rel-03",
		"effectiveStartsAt":"2026-09-01T00:00:00Z"
	}]}`))
	if err != nil {
		t.Fatalf("intake（候选）：%v", err)
	}
	if candidate.Approval != nil {
		t.Fatalf("Approval = %+v，载荷没带批准事实，该登为候选", candidate.Approval)
	}
	if _, bounded := candidate.Spec.Effective.EndsAt(); bounded {
		t.Fatal("Effective.EndsAt 有界，载荷没给终点，该是开区间")
	}

	_, err = intake.IntakePartyRelationshipRegistration(context.Background(), relationshipRequest(`{"relationships":[{
		"relationshipId":"SYN-REL-04","revision":1,"holder":"SYN-PARTY-02","counterparty":"SYN-PARTY-03",
		"role":"OWNER","scope":"s","basis":"b","effectiveStartsAt":"2026-09-01T00:00:00Z"
	}]}`))
	if !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("集合外角色：err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: identity-deactivation 一口——`deactivations[0]` 五格逐字来自载荷，租户格来自认证结果。停用是往修订链上插一笔
// 新修订，Revision 是操作者声明自己看到的册面（= 最新修订 + 1），错位由用例拒；身份种类是封闭集的名称镜像，关系不在
// 内（关系的终止走撤销/到期/替代，不叫停用）。外壳镜像 CLI `deactivate-party-identity` 的文档、去掉整批的 tenantId。
func TestOperatorRegistryIntakeTranslatesDeactivationUnderTheAuthenticatedTenant(t *testing.T) {
	intake := operatorIntake(t)
	deactivationRequest := func(body string) *http.Request {
		return identityRequest("/commercial-party-identity-deactivations", body)
	}

	command, err := intake.IntakePartyIdentityDeactivation(context.Background(), deactivationRequest(`{"deactivations":[{
		"kind":"LEGAL_ENTITY",
		"id":"SYN-LE-02",
		"revision":2,
		"basis":"SYN-BASIS/le-02-deactivate",
		"at":"2026-09-17T00:00:00Z"
	}]}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.Tenant.String(); got != operatorAuthenticatedTenant {
		t.Fatalf("Tenant = %q, want %q（认证结果）", got, operatorAuthenticatedTenant)
	}
	if got := command.Kind.String(); got != "LEGAL_ENTITY" {
		t.Fatalf("Kind = %q, want LEGAL_ENTITY", got)
	}
	if command.ID != "SYN-LE-02" {
		t.Fatalf("ID = %q, want SYN-LE-02", command.ID)
	}
	if command.Revision != 2 {
		t.Fatalf("Revision = %d, want 2", command.Revision)
	}
	if got := command.Basis.String(); got != "SYN-BASIS/le-02-deactivate" {
		t.Fatalf("Basis = %q, want SYN-BASIS/le-02-deactivate", got)
	}
	if want := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC); !command.At.Equal(want) {
		t.Fatalf("At = %s, want %s", command.At, want)
	}

	for name, body := range map[string]string{
		"关系不叫停用": `{"deactivations":[{"kind":"PARTY_RELATIONSHIP","id":"SYN-REL-02","revision":2,"basis":"b","at":"2026-09-17T00:00:00Z"}]}`,
		"两项":     `{"deactivations":[{"kind":"LEGAL_ENTITY","id":"SYN-LE-02","revision":2,"basis":"b","at":"2026-09-17T00:00:00Z"},{"kind":"LEGAL_ENTITY","id":"SYN-LE-03","revision":2,"basis":"b","at":"2026-09-17T00:00:00Z"}]}`,
		"投了登记项":  `{"legalEntities":[{"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakePartyIdentityDeactivation(context.Background(), deactivationRequest(body))
			if !errors.Is(err, commercialhttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

// Covers: 票 admin-web-group-legal-entities/07 要做的第 1 条——一份载荷只许一个文档。尾随的第二个 JSON 值不是本载荷
// 的一部分，放过它就是无声丢掉一段输入：与下面 exactlyOne 对「无声丢掉」的立场相反，也与同包发布口对同一形状的答复
// 相反。两份外壳（登记 / 停用）各投一次：它们各走一遍解码，一处改了另一处未必跟着改。在 HTTP 缝上证，钉的是登记方
// 看得见的三样：400、MALFORMED_REQUEST、无 outcome；编排一次也不该被调到。
func TestPartyIdentityIntakeRefusesTrailingContentAfterThePayload(t *testing.T) {
	intake := operatorIntake(t)
	legalEntity := `{"legalEntities":[{"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}]}`
	deactivation := `{"deactivations":[{"kind":"LEGAL_ENTITY","id":"SYN-LE-02","revision":2,"basis":"b","at":"2026-09-17T00:00:00Z"}]}`
	for name, testCase := range map[string]struct {
		endpoint func(commercialhttp.PartyIdentityRegistrar) http.Handler
		body     string
	}{
		"登记口": {
			endpoint: func(registrar commercialhttp.PartyIdentityRegistrar) http.Handler {
				return commercialhttp.NewRegisterLegalEntityEndpoint(intake, registrar)
			},
			body: legalEntity + ` {"x":1}`,
		},
		"停用口": {
			endpoint: func(registrar commercialhttp.PartyIdentityRegistrar) http.Handler {
				return commercialhttp.NewDeactivatePartyIdentityEndpoint(intake, registrar)
			},
			body: deactivation + ` {"x":1}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			registrar := &scriptedPartyRegistrar{}
			recorder := httptest.NewRecorder()
			testCase.endpoint(registrar).ServeHTTP(recorder, identityRequest("/probe", testCase.body))

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("带尾随内容的载荷答 %d %s，want 400", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
				t.Fatalf("错误码 = %q，want MALFORMED_REQUEST", code)
			}
			assertNoRegistrationOutcome(t, recorder)
			if registrar.called {
				t.Fatal("尾随内容没拦在 Intake，编排被调到了")
			}
		})
	}
}

// Covers: 票 admin-web-group-legal-entities/11 要做的第 1 条——形状拒绝的 400 把 Intake 已经写在错误里的理由交到线上：
// `error.detail` 在场、非空、点得出是哪一种错。多种形状错只交 code 时在线上长一张脸，页面只能显一句通用说明，操作者要
// 靶着每一种可能挨个试。判据同 `cause`：detail 是散文，这里只钉「非空且含关键字」，不钉全文——全文属 encoding/json
// 与本包的措辞自由，改一个字不该让这条变红。哨兵自己的英文原句不该重复进 detail：登记方要读的是哨兵之后那半。
func TestMalformedRegistrationRefusalCarriesTheReasonAsDetail(t *testing.T) {
	intake := operatorIntake(t)
	item := `{"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}`
	for name, testCase := range map[string]struct {
		body    string
		keyword string
	}{
		"带 tenantId": {
			body:    `{"tenantId":"` + operatorAuthenticatedTenant + `","legalEntities":[` + item + `]}`,
			keyword: "tenantId",
		},
		"未知键": {
			body:    `{"legalEntities":[{"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z","name":"x"}]}`,
			keyword: "unknown field",
		},
		"两项": {
			body:    `{"legalEntities":[` + item + `,` + item + `]}`,
			keyword: "exactly one",
		},
		"尾随内容": {
			body:    `{"legalEntities":[` + item + `]} {"x":1}`,
			keyword: "trailing",
		},
		"空法人标识": {
			body:    `{"legalEntities":[{"legalEntityId":"","partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}]}`,
			keyword: "legalEntities[0].legalEntityId",
		},
	} {
		t.Run(name, func(t *testing.T) {
			registrar := &scriptedPartyRegistrar{}
			recorder := httptest.NewRecorder()
			commercialhttp.NewRegisterLegalEntityEndpoint(intake, registrar).ServeHTTP(recorder, legalEntityRequest(testCase.body))

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("答 %d %s，want 400", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
				t.Fatalf("错误码 = %q，want MALFORMED_REQUEST", code)
			}
			detail, present := problemDetailOf(t, recorder)
			if !present || detail == "" {
				t.Fatalf("400 没带 detail，多种形状错在线上长一张脸：%s", recorder.Body.String())
			}
			if !strings.Contains(detail, testCase.keyword) {
				t.Fatalf("detail = %q，没点出 %q 这件事", detail, testCase.keyword)
			}
			if strings.Contains(detail, commercialhttp.ErrMalformedRequest.Error()) {
				t.Fatalf("detail 把哨兵自己的英文原句重复了一遍：%q", detail)
			}
			assertNoRegistrationOutcome(t, recorder)
			if registrar.called {
				t.Fatal("形状错没拦在 Intake，编排被调到了")
			}
		})
	}
}

// problemDetailOf 取问题体里的 `detail` 一格并报它在不在场。分开报「在场」与「值」：5xx 那几条要证的正是这一格
// 不在场，而 map 取值把「缺席」与「空串」折成同一个空串。
func problemDetailOf(t *testing.T, recorder *httptest.ResponseRecorder) (string, bool) {
	t.Helper()
	body := decodeBody(t, recorder)
	problem, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("响应没有 error 体：%s", recorder.Body.String())
	}
	raw, present := problem["detail"]
	if !present {
		return "", false
	}
	detail, _ := raw.(string)
	return detail, true
}

// Covers: 一口只收本口的项。载荷外壳镜像 CLI 的整份文档，因此别的口的数组在这里**解得开**；解得开不等于
// 可以忽略——一份同时带着法人项与参与方项的载荷投到法人口，参与方那一项会被无声丢掉，登记方以为两样都登了。
// 四类登记共用同一份外壳，这里各投一次别人的项。
func TestPartyIdentityIntakeRefusesItemsMeantForAnotherLine(t *testing.T) {
	intake := operatorIntake(t)
	legalEntity := `{"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}`
	businessParty := `{"partyId":"SYN-PARTY-02","name":"n","revision":1,"basis":"b","effectiveFrom":"2026-09-16T00:00:00Z"}`

	_, err := intake.IntakeLegalEntityRegistration(context.Background(),
		legalEntityRequest(`{"legalEntities":[`+legalEntity+`],"businessParties":[`+businessParty+`]}`))
	if !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("法人口收了参与方项：err = %v, want ErrMalformedRequest", err)
	}
	_, err = intake.IntakeBusinessPartyRegistration(context.Background(),
		legalEntityRequest(`{"legalEntities":[`+legalEntity+`]}`))
	if !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("参与方口收了法人项：err = %v, want ErrMalformedRequest", err)
	}
}
