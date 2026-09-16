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
	if _, ok := intake.(commercialhttp.BusinessPartyRegistrationIntake); ok {
		t.Fatal("业务参与方登记口尚未成笔，不该装得进")
	}
	if _, ok := intake.(commercialhttp.CustomerAccountRegistrationIntake); ok {
		t.Fatal("货主客户账户登记口尚未成笔，不该装得进")
	}
	if _, ok := intake.(commercialhttp.PartyRelationshipRegistrationIntake); ok {
		t.Fatal("参与方关系登记口尚未成笔，不该装得进")
	}
	if _, ok := intake.(commercialhttp.PartyIdentityDeactivationIntake); ok {
		t.Fatal("身份停用口尚未成笔，不该装得进")
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
