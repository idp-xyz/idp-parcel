package registrationjson_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 在线口那一路（票 operator-channel/52）：租户取认证结果注入命令，载荷带任何大小写的租户键即拒；形状照受控批量口
// 那一份译装。

// tenantKeyCaseVariants 是 encoding/json 会解进 `json:"tenantId"` 格的那些键：它匹配字段名不分大小写，拒自报只认
// 精确键时，变体就顶着认证租户落进别的租户。
var tenantKeyCaseVariants = []string{"tenantId", "TenantId", "tenantID", "tenantid", "tenantiD", "TENANTID"}

const onlineAdoptBody = `{"factRef":"SYN-FACT-1","sourceRef":"SYN-SOURCE-BANK-1","kind":"RECEIPT_CONFIRMED",` +
	`"currency":"XTS","amountMinor":8000,"version":"SYN-FACT-1/v1","occurredAt":"2026-09-01T08:00:00Z"}`

const onlineCorrectionBody = `{"factRef":"SYN-FACT-1","corrects":"SYN-FACT-1/v1","version":"SYN-FACT-1/v2",` +
	`"amountMinor":7000,"correctedAt":"2026-09-02T08:00:00Z"}`

func onlineTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

func TestOnlineFundsTranslationTakesTheInjectedTenant(t *testing.T) {
	tenant := onlineTenant(t)
	adopt, err := registrationjson.ExternalFundsFactFromJSONForTenant([]byte(onlineAdoptBody), tenant)
	if err != nil {
		t.Fatalf("online adopt: %v", err)
	}
	if adopt.TenantID != tenant || adopt.Fact != "SYN-FACT-1" || adopt.AmountMinor != 8000 {
		t.Fatalf("adopt = tenant %q, fact %q, amount %d", adopt.TenantID.String(), adopt.Fact, adopt.AmountMinor)
	}
	correction, err := registrationjson.ExternalFundsFactCorrectionFromJSONForTenant([]byte(onlineCorrectionBody), tenant)
	if err != nil {
		t.Fatalf("online correction: %v", err)
	}
	if correction.TenantID != tenant || correction.Corrects != "SYN-FACT-1/v1" || correction.AmountMinor != 7000 {
		t.Fatalf("correction = tenant %q, corrects %q, amount %d", correction.TenantID.String(), correction.Corrects, correction.AmountMinor)
	}
}

func TestOnlineFundsTranslationRefusesEveryLetterCaseOfTheTenantKey(t *testing.T) {
	tenant := onlineTenant(t)
	adoptRest := strings.TrimPrefix(onlineAdoptBody, "{")
	correctionRest := strings.TrimPrefix(onlineCorrectionBody, "{")
	for _, key := range tenantKeyCaseVariants {
		adopt, err := registrationjson.ExternalFundsFactFromJSONForTenant([]byte(`{"`+key+`":"SYN-TENANT-OTHER",`+adoptRest), tenant)
		if !errors.Is(err, registrationjson.ErrSelfReportedTenant) {
			t.Fatalf("采用键 %q：err = %v，命令租户 %q；want ErrSelfReportedTenant", key, err, adopt.TenantID.String())
		}
		correction, err := registrationjson.ExternalFundsFactCorrectionFromJSONForTenant(
			[]byte(`{"`+key+`":"SYN-TENANT-OTHER",`+correctionRest), tenant)
		if !errors.Is(err, registrationjson.ErrSelfReportedTenant) {
			t.Fatalf("更正键 %q：err = %v，命令租户 %q；want ErrSelfReportedTenant", key, err, correction.TenantID.String())
		}
	}
	// 与 tenantId 不同折叠的键 encoding/json 也不认，在线口照未知字段拒，不是放行。
	if _, err := registrationjson.ExternalFundsFactFromJSONForTenant([]byte(`{"tenantİd":"SYN-TENANT-OTHER",`+adoptRest), tenant); err == nil {
		t.Fatal("tenantİd 被放行")
	}
}
