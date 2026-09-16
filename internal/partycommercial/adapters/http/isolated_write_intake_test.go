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

// 本文件对隔离写路径准入在身份族的注入式放行（ADR-0091；票 admin-web-group-legal-entities/06）证传输面：
// 租户格只来自注入、行内容只从载荷取、自报租户即拒、形状错即拒、未放行的口在类型上装不进去。
//
// 载荷形状照票 02 的表单（`legalEntities[0]` 五格，镜像受控 CLI `register-parties` 的一项、去掉整批的 tenantId）。
// 期望值取自载荷字面量而不是重算：这里证的是「译装没换字」，不是译装怎么算。

const isolatedIdentityTenant = "SYN-TENANT-01"

func isolatedIdentityIntakeForTest(t *testing.T) *commercialhttp.IsolatedPartyIdentityIntake {
	t.Helper()
	intake, err := commercialhttp.NewIsolatedPartyIdentityIntake(isolatedIdentityTenant)
	if err != nil {
		t.Fatalf("构造隔离身份 Intake：%v", err)
	}
	return intake
}

func legalEntityRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/commercial-legal-entity-registrations", strings.NewReader(body))
}

// Covers: ADR-0091 决定一第二判据 + register_party_identity.go 包注释「真渠道 Intake 要把认证结果填进租户格、
// 只从载荷取行内容」——隔离形态照办：租户格是开关值，五格行内容逐字来自载荷，请求头与查询串里的自报一律无视。
func TestIsolatedPartyIdentityIntakeTranslatesLegalEntityRegistrationWithInjectedTenant(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
	request := legalEntityRequest(`{"legalEntities":[{
		"legalEntityId":"SYN-LE-02",
		"partyId":"SYN-PARTY-02",
		"revision":3,
		"basis":"SYN-BASIS/le-02-r3",
		"effectiveFrom":"2026-09-16T08:00:00+08:00"
	}]}`)
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	request.URL.RawQuery = "tenant=TENANT-9"

	command, err := intake.IntakeLegalEntityRegistration(context.Background(), request)
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.Tenant.String(); got != isolatedIdentityTenant {
		t.Fatalf("Tenant = %q, want %q（注入值，不是请求里的自报）", got, isolatedIdentityTenant)
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

// Covers: 票 06 要做的第 1 条——载荷里带 tenantId 即拒（MALFORMED_REQUEST）。在线口不采信自报租户，隔离形态也
// 不例外：值与开关相同也拒，否则「碰巧相同」的载荷会在别的环境里变成穿透 ADR-0003 隔离边界的第一步。
func TestIsolatedPartyIdentityIntakeRefusesSelfReportedTenant(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
	item := `{"legalEntityId":"SYN-LE-02","partyId":"SYN-PARTY-02","revision":1,"basis":"SYN-BASIS/le-02","effectiveFrom":"2026-09-16T00:00:00Z"}`
	for name, body := range map[string]string{
		"另一租户":  `{"tenantId":"SYN-TEN-OTHER","legalEntities":[` + item + `]}`,
		"与开关同值": `{"tenantId":"` + isolatedIdentityTenant + `","legalEntities":[` + item + `]}`,
		"空租户":   `{"tenantId":null,"legalEntities":[` + item + `]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeLegalEntityRegistration(context.Background(), legalEntityRequest(body))
			if !errors.Is(err, commercialhttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
			if !strings.Contains(err.Error(), "tenantId") {
				t.Fatalf("拒绝理由没点名 tenantId，登记方不知道该去掉哪一格：%v", err)
			}
		})
	}
}

// Covers: 形状级失败包 ErrMalformedRequest（4xx，重发同样内容不会好），判据同 ADR-0029。一次一笔：本端点一次只收一项，
// 批走受控 CLI；零项与多项都不是这一口的形状。空标识由领域值对象拒，Intake 不替它放行一个立不住的命令。
func TestIsolatedPartyIdentityIntakeRefusesMalformedLegalEntityPayloads(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
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

// Covers: 第二口 business-party（票 06 要做的第 2 条）——`businessParties[0]` 四格 + 生效时刻逐字来自载荷，
// 租户格来自注入；参与方本体（BusinessParty）由领域构造函数在 Intake 里立起来，用例侧不再重构。
func TestIsolatedPartyIdentityIntakeTranslatesBusinessPartyRegistrationWithInjectedTenant(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
	request := httptest.NewRequest(http.MethodPost, "/commercial-business-party-registrations", strings.NewReader(`{"businessParties":[{
		"partyId":"SYN-PARTY-02",
		"name":"SYN 合成参与方二号",
		"revision":2,
		"basis":"SYN-BASIS/party-02-r2",
		"effectiveFrom":"2026-09-16T00:00:00Z"
	}]}`))
	request.Header.Set("X-Reported-Tenant", "TENANT-9")

	command, err := intake.IntakeBusinessPartyRegistration(context.Background(), request)
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.Party.Tenant().String(); got != isolatedIdentityTenant {
		t.Fatalf("Party.Tenant = %q, want %q（注入值）", got, isolatedIdentityTenant)
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

// Covers: 第三口 customer-account——`customerAccounts[0]` 五格逐字来自载荷，租户格来自注入；跨租户绑定由领域构造门
// 在用例侧拒（ADR-0041），Intake 不预判。
func TestIsolatedPartyIdentityIntakeTranslatesCustomerAccountRegistrationWithInjectedTenant(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
	request := httptest.NewRequest(http.MethodPost, "/commercial-customer-account-registrations", strings.NewReader(`{"customerAccounts":[{
		"accountId":"SYN-ACCT-02",
		"customerPartyId":"SYN-PARTY-02",
		"revision":1,
		"basis":"SYN-BASIS/acct-02",
		"effectiveFrom":"2026-09-16T00:00:00Z"
	}]}`))
	request.Header.Set("X-Reported-Tenant", "TENANT-9")

	command, err := intake.IntakeCustomerAccountRegistration(context.Background(), request)
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.Tenant.String(); got != isolatedIdentityTenant {
		t.Fatalf("Tenant = %q, want %q（注入值）", got, isolatedIdentityTenant)
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

// Covers: 第四口 party-relationship——`relationships[0]` 正文沿 domain.PartyRelationshipSpec 逐格来自载荷，租户格来自
// 注入；批准事实与区间终点两格可缺席：缺席即候选关系 / 开区间，不造假值顶上（CLI 同形）。角色是封闭集的名称镜像，
// 集合外取值拒收不吸收。
func TestIsolatedPartyIdentityIntakeTranslatesPartyRelationshipRegistrationWithInjectedTenant(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
	relationshipRequest := func(body string) *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/commercial-party-relationship-registrations", strings.NewReader(body))
		request.Header.Set("X-Reported-Tenant", "TENANT-9")
		return request
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
	if got := approved.Tenant.String(); got != isolatedIdentityTenant {
		t.Fatalf("Tenant = %q, want %q（注入值）", got, isolatedIdentityTenant)
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

// Covers: 第五口 identity-deactivation——`deactivations[0]` 五格逐字来自载荷，租户格来自注入。停用是往修订链上插一笔
// 新修订，Revision 是操作者声明自己看到的册面（= 最新修订 + 1），错位由用例拒；身份种类是封闭三值的名称镜像，关系不在
// 内（关系的终止走撤销/到期/替代，不叫停用）。外壳镜像 CLI `deactivate-party-identity` 的文档、去掉整批的 tenantId。
func TestIsolatedPartyIdentityIntakeTranslatesDeactivationWithInjectedTenant(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
	deactivationRequest := func(body string) *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/commercial-party-identity-deactivations", strings.NewReader(body))
		request.Header.Set("X-Reported-Tenant", "TENANT-9")
		return request
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
	if got := command.Tenant.String(); got != isolatedIdentityTenant {
		t.Fatalf("Tenant = %q, want %q（注入值）", got, isolatedIdentityTenant)
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
		"自报租户":   `{"tenantId":"` + isolatedIdentityTenant + `","deactivations":[{"kind":"LEGAL_ENTITY","id":"SYN-LE-02","revision":2,"basis":"b","at":"2026-09-17T00:00:00Z"}]}`,
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

// Covers: 一口只收本口的项。载荷外壳镜像 CLI 的整份文档，因此别的口的数组在这里**解得开**；解得开不等于
// 可以忽略——一份同时带着法人项与参与方项的载荷投到法人口，参与方那一项会被无声丢掉，登记方以为两样都登了。
// 五口共用同一份外壳，这条对每一口都成立，这里各口投一次别人的项。
func TestIsolatedPartyIdentityIntakeRefusesItemsMeantForAnotherLine(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
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

// Covers: 立不起来的注入在构造时拒，不等第一个请求（同隔离读 Intake 的纪律）。
func TestNewIsolatedPartyIdentityIntakeRejectsBlankTenant(t *testing.T) {
	if _, err := commercialhttp.NewIsolatedPartyIdentityIntake("   "); err == nil {
		t.Fatal("空租户被接受")
	}
}

// Covers: ADR-0091 Consequences「命令面按端点逐口放行，不是一次全开」——本类型此刻只实现放行了的那几口，
// 未放行的口在类型上就装不进去（编译期），不靠装配点的纪律。每放一口，该口的断言从这里移走、改成放行测试。
// 发布口与产品渠道口不在本票（票面「不做」），永远不该出现在这里的放行名单里。
func TestIsolatedPartyIdentityIntakeServesOnlyAdmittedLines(t *testing.T) {
	var intake any = isolatedIdentityIntakeForTest(t)
	if _, ok := intake.(commercialhttp.LegalEntityRegistrationIntake); !ok {
		t.Fatal("责任法人登记口该已放行")
	}
	if _, ok := intake.(commercialhttp.BusinessPartyRegistrationIntake); !ok {
		t.Fatal("业务参与方登记口该已放行")
	}
	if _, ok := intake.(commercialhttp.CustomerAccountRegistrationIntake); !ok {
		t.Fatal("货主客户账户登记口该已放行")
	}
	if _, ok := intake.(commercialhttp.PartyRelationshipRegistrationIntake); !ok {
		t.Fatal("参与方关系登记口该已放行")
	}
	if _, ok := intake.(commercialhttp.PartyIdentityDeactivationIntake); !ok {
		t.Fatal("身份停用口该已放行")
	}
	if _, ok := intake.(commercialhttp.CommercialPublicationIntake); ok {
		t.Fatal("发布口不在本票，隔离身份 Intake 不该装得进")
	}
	if _, ok := intake.(commercialhttp.PublicationDraftApprovalIntake); ok {
		t.Fatal("批准口不在本票，隔离身份 Intake 不该装得进")
	}
	if _, ok := intake.(commercialhttp.ServiceProductFormRegistrationIntake); ok {
		t.Fatal("服务形态登记口不在本票，隔离身份 Intake 不该装得进")
	}
	if _, ok := intake.(commercialhttp.ProductChannelMappingRegistrationIntake); ok {
		t.Fatal("产品渠道映射登记口不在本票，隔离身份 Intake 不该装得进")
	}
	if _, ok := intake.(commercialhttp.CommercialCatalogueIntake); ok {
		t.Fatal("查阅行归隔离读 Intake，写 Intake 不该装得进（两个开关分设，ADR-0091 决定四）")
	}
}
