package settlementhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

// 本文件对隔离写路径准入在外部资金事实采用口的注入式放行（ADR-0091；票 operator-channel/08）证传输面：租户格只来自注入、
// 资金事实的身份与发生时刻照 ADR-0023 从载荷如实收、自报租户即拒、形状错即拒、未放行的口在类型上装不进去。

const isolatedCommandTenant = "SYN-TENANT-01"

func isolatedCommandIntakeForTest(t *testing.T) *settlementhttp.IsolatedCommandIntake {
	t.Helper()
	intake, err := settlementhttp.NewIsolatedCommandIntake(isolatedCommandTenant)
	if err != nil {
		t.Fatalf("构造隔离命令 Intake：%v", err)
	}
	return intake
}

func commandRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	request.URL.RawQuery = "tenant=TENANT-9"
	return request
}

const isolatedExternalFundsFactBody = `{"factRef":"SYN-FUNDS-FACT-0801","sourceRef":"SYN-FUNDS-SOURCE/bank-08",` +
	`"payerRef":"SYN-PAYER/consignee-08","kind":"RECEIPT_CONFIRMED","currency":"SGD","amountMinor":12345,"version":"v1",` +
	`"occurredAt":"2026-09-24T15:00:00+08:00"}`

// Covers: 采用口的线格式就是受控 CLI `parcel-settlement-register -input` 的载荷去掉 tenantId（registrationjson 包注释：tenantId 是
// 不是调用方有权写的那个租户由在线口的 Intake 回答）——隔离 Intake 把注入的租户拼回那一格，交给同一份译装。
func TestIsolatedCommandIntakeTranslatesExternalFundsFactWithInjectedTenant(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakeExternalFundsFactRegistration(context.Background(),
		commandRequest(isolatedExternalFundsFactBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q（注入值，不是请求里的自报）", got, isolatedCommandTenant)
	}
	if command.Fact != "SYN-FUNDS-FACT-0801" || command.Source != "SYN-FUNDS-SOURCE/bank-08" || command.Payer != "SYN-PAYER/consignee-08" ||
		command.Kind.String() != "RECEIPT_CONFIRMED" || command.Currency != "SGD" || command.AmountMinor != 12345 || command.Version != "v1" {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	if want := time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s（资金事实的发生时刻，不是服务端时钟）", command.OccurredAt, want)
	}
}

// Covers: 载荷里带 tenantId 即拒——值与开关相同也拒；拼接只在一个 JSON 对象上做，别的形状、尾随内容与译装不认的键都拒。
func TestIsolatedCommandIntakeRefusesMalformedExternalFundsFacts(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	rest := strings.TrimPrefix(isolatedExternalFundsFactBody, "{")
	for name, testCase := range map[string]struct{ body, keyword string }{
		"与开关同值的租户": {`{"tenantId":"` + isolatedCommandTenant + `",` + rest, "tenantId"},
		"空租户":      {`{"tenantId":null,` + rest, "tenantId"},
		"null":     {`null`, ""},
		"数组":       {`[` + isolatedExternalFundsFactBody + `]`, ""},
		"尾随内容":     {isolatedExternalFundsFactBody + ` {"x":1}`, "trailing"},
		"未知键":      {`{"bankRef":"x",` + rest, ""},
		"词表外的种类":   {strings.Replace(isolatedExternalFundsFactBody, "RECEIPT_CONFIRMED", "MAYBE_PAID", 1), ""},
		"空载荷":      {``, ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeExternalFundsFactRegistration(context.Background(), commandRequest(testCase.body))
			if !errors.Is(err, settlementhttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
			if testCase.keyword != "" && !strings.Contains(err.Error(), testCase.keyword) {
				t.Fatalf("拒绝理由没点名 %s：%v", testCase.keyword, err)
			}
		})
	}
}

// Covers: 立不起来的注入在构造时拒，不等第一个请求。
func TestNewIsolatedCommandIntakeRejectsBlankTenant(t *testing.T) {
	if _, err := settlementhttp.NewIsolatedCommandIntake("   "); err == nil {
		t.Fatal("空租户被接受")
	}
}

// Covers: ADR-0091 Consequences「命令面按端点逐口放行」——本类型只放行采用口；更正口（同族未列）与查阅行在类型上就装不进去。
func TestIsolatedCommandIntakeServesOnlyAdmittedLines(t *testing.T) {
	var intake any = isolatedCommandIntakeForTest(t)
	if _, ok := intake.(settlementhttp.ExternalFundsFactRegistrationIntake); !ok {
		t.Fatal("外部资金事实采用口该已放行")
	}
	if _, ok := intake.(settlementhttp.ExternalFundsFactCorrectionRegistrationIntake); ok {
		t.Fatal("更正口同族未列，隔离命令 Intake 不该装得进")
	}
	if _, ok := intake.(settlementhttp.CatalogueQueryIntake); ok {
		t.Fatal("查阅行归隔离读 Intake，写 Intake 不该装得进")
	}
}
