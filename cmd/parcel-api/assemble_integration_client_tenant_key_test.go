package main

// 票 operator-channel/52 的真渠道真库回归：集成客户端绑在 SYN-TENANT-01，载荷里不论用哪种大小写写租户键，都只能答
// 400 坏报文，两个租户的册上都不许多出一行。链路照生产装配：替身核验方 → buildIntegrationClientMinter → 生产准入桥
// buildAdmissionScope（真库对照与区间）→ buildIntegrationClientIntakes → 两上下文的登记端点 → 生产编排 → 真库。
// 只替掉令牌核验与客户端册这两处外部依赖：变体键改写租户发生在 Intake 与译装之间，替身不碰那一段。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

// 这些键 encoding/json 都会解进 `json:"tenantId"` 格——它匹配字段名不分大小写。
var selfReportedTenantKeys = []string{"tenantId", "TenantId", "tenantID", "tenantid", "tenantiD", "TENANTID"}

const otherTenant = "SYN-TENANT-OTHER"

// integrationClientChannelOnRealDB 登好 SYN-TENANT-01 对这一类事实的准入对照与生产权威区间，交回生产 Intake 与真库。
func integrationClientChannelOnRealDB(t *testing.T, fact accessidentity.ExternalFactType) (integrationClientIntakes, *bentopg.DB, func(string, ...any) int) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	mappings, err := aipg.NewAdmissionScopeMappings(db)
	if err != nil {
		t.Fatal(err)
	}
	intervals, err := pgpostgres.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	validity, err := accessidentity.NewEffectiveInterval(from, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	requirement := accessidentity.AdmissionRequirement{Capability: accessidentity.ExternalFactAdmissionCapability, FactKind: fact.String()}
	mapping, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-OC52", requirement,
		"SYN-PILOT-SCOPE/external-facts@v1", validity, "SYN-MAP-BASIS-OC52")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := mappings.RegisterAdmissionScopeMapping(txCtx, mapping); err != nil {
			return err
		}
		return intervals.Append(txCtx, pgdomain.AuthorityInterval{
			ObjectScope: "SYN-PILOT-SCOPE/external-facts@v1",
			Capability:  requirement.Capability,
			FactKind:    requirement.FactKind,
			Authority:   isolatedSelfAuthority,
			From:        from,
		})
	}); err != nil {
		t.Fatalf("登对照与区间：%v", err)
	}
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	minter, err := buildIntegrationClientMinter(acceptingClientVerifier{},
		clientFactRegister{facts: []accessidentity.ExternalFactType{fact}, start: now.Add(-time.Hour), end: now.Add(time.Hour)}, admission)
	if err != nil {
		t.Fatal(err)
	}
	intakes, err := buildIntegrationClientIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var rows int
		if err := pool.QueryRow(t.Context(), query, args...).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	return intakes, db, count
}

func postAsIntegrationClient(t *testing.T, endpoint http.Handler, body string) (int, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.client.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response.Code, problemCode(t, response)
}

func credentialBody(id string, uses int) string {
	return `{"credentialId":"` + id + `","issuerRef":"SYN-AUTHORITY/customs-sg","holderRef":"SYN-HOLDER/broker-52",` +
		`"procedureRef":"SYN-PROCEDURE/import-general","validFrom":"2026-01-01T00:00:00Z","validTo":"2027-01-01T00:00:00Z",` +
		`"uses":` + strconv.Itoa(uses) + `}`
}

func withTenantKey(key, body string) string {
	return `{"` + key + `":"` + otherTenant + `",` + strings.TrimPrefix(body, "{")
}

func TestIntegrationClientCredentialRegistrationRefusesEveryLetterCaseOfASelfReportedTenantOnRealDB(t *testing.T) {
	intakes, db, count := integrationClientChannelOnRealDB(t, accessidentity.FactRegulatoryCredential)
	registration, err := buildCustomsRegistrationOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := customshttp.NewRegisterRegulatoryCredentialEndpoint(intakes.customs, registration.regulatoryCredential)
	rowsOf := func(id, tenant string) int {
		return count(`SELECT count(*) FROM customs_compliance.regulatory_credential WHERE credential_id = $1 AND tenant_id = $2`, id, tenant)
	}

	// 对照：不带租户键时这条链确实登得进认证租户——下面的 400 才说明是被拒，不是链路没通。
	if status, _ := postAsIntegrationClient(t, endpoint, credentialBody("SYN-CRED-OC52-CONTROL", 3)); status != http.StatusCreated {
		t.Fatalf("对照登记 = %d，want 201", status)
	}
	if rowsOf("SYN-CRED-OC52-CONTROL", "SYN-TENANT-01") != 1 {
		t.Fatal("对照登记没落在认证租户")
	}

	for _, key := range selfReportedTenantKeys {
		id := "SYN-CRED-OC52-" + key
		status, code := postAsIntegrationClient(t, endpoint, withTenantKey(key, credentialBody(id, 3)))
		if status != http.StatusBadRequest || code != "MALFORMED_REQUEST" {
			t.Errorf("键 %q：答 %d %q，want 400 MALFORMED_REQUEST", key, status, code)
		}
		if own, other := rowsOf(id, "SYN-TENANT-01"), rowsOf(id, otherTenant); own != 0 || other != 0 {
			t.Errorf("键 %q：册上 SYN-TENANT-01 %d 行、%s %d 行，want 都是零", key, own, otherTenant, other)
		}
	}

	// 跨租户探询：他租户在册的凭证，猜中与猜错都只能答坏报文，不许借 EXISTING / CONTENT_CONFLICT 漏出存在与内容。
	tenant, err := customsdomain.NewTenantID(otherTenant)
	if err != nil {
		t.Fatal(err)
	}
	credentialID, _ := customsdomain.NewCredentialID("SYN-CRED-OC52-OTHER")
	issuer, _ := customsdomain.NewRegulatoryAuthorityReference("SYN-AUTHORITY/customs-sg")
	holder, _ := customsdomain.NewCredentialHolderReference("SYN-HOLDER/broker-52")
	procedure, _ := customsdomain.NewCustomsProcedureReference("SYN-PROCEDURE/import-general")
	if _, err := registration.regulatoryCredential.Handle(t.Context(), customsapp.RegisterCredentialCommand{
		TenantID: tenant, ID: credentialID, Issuer: issuer, Holder: holder, Procedure: procedure,
		ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Uses: 3,
	}); err != nil {
		t.Fatalf("登他租户的凭证：%v", err)
	}
	for _, key := range selfReportedTenantKeys {
		for guess, uses := range map[string]int{"猜中": 3, "猜错额度": 4} {
			status, code := postAsIntegrationClient(t, endpoint, withTenantKey(key, credentialBody("SYN-CRED-OC52-OTHER", uses)))
			if status != http.StatusBadRequest || code != "MALFORMED_REQUEST" {
				t.Errorf("探询（键 %q，%s）：答 %d %q，want 400 MALFORMED_REQUEST", key, guess, status, code)
			}
		}
	}
	if own, other := rowsOf("SYN-CRED-OC52-OTHER", "SYN-TENANT-01"), rowsOf("SYN-CRED-OC52-OTHER", otherTenant); own != 0 || other != 1 {
		t.Fatalf("探询后册上 SYN-TENANT-01 %d 行、%s %d 行，want 0 与 1", own, otherTenant, other)
	}
}

func fundsFactBody(fact string) string {
	return `{"factRef":"` + fact + `","sourceRef":"SYN-FUNDS-SOURCE/bank-52","kind":"RECEIPT_CONFIRMED","currency":"XTS",` +
		`"amountMinor":5200,"version":"` + fact + `/v1","occurredAt":"2026-09-24T07:00:00Z"}`
}

func fundsCorrectionBody(fact string) string {
	return `{"factRef":"` + fact + `","corrects":"` + fact + `/v1","version":"` + fact + `/v2","amountMinor":5300,` +
		`"correctedAt":"2026-09-25T07:00:00Z"}`
}

func TestIntegrationClientFundsFactRegistrationRefusesEveryLetterCaseOfASelfReportedTenantOnRealDB(t *testing.T) {
	intakes, db, count := integrationClientChannelOnRealDB(t, accessidentity.FactExternalFunds)
	registration, err := buildSettlementRegistrationOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	adopt := settlementhttp.NewRegisterExternalFundsFactEndpoint(intakes.settlement, registration.externalFundsFact)
	correct := settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(intakes.settlement, registration.externalFundsFactCorrection)
	factsOf := func(fact, tenant string) int {
		return count(`SELECT count(*) FROM settlement_accounting.external_funds_fact WHERE fact_id = $1 AND tenant_id = $2`, fact, tenant)
	}
	versionsOf := func(fact, tenant string) int {
		return count(`SELECT count(*) FROM settlement_accounting.external_funds_fact_version WHERE fact_id = $1 AND tenant_id = $2`, fact, tenant)
	}

	// 对照：不带租户键时采用落在认证租户；更正各变体都回指这一条，旧路上 TenantId 一类会把更正登上去。
	const control = "SYN-FUNDS-FACT-OC52-CONTROL"
	if status, _ := postAsIntegrationClient(t, adopt, fundsFactBody(control)); status != http.StatusCreated {
		t.Fatalf("对照采用 = %d，want 201", status)
	}
	if factsOf(control, "SYN-TENANT-01") != 1 {
		t.Fatal("对照采用没落在认证租户")
	}

	for _, key := range selfReportedTenantKeys {
		fact := "SYN-FUNDS-FACT-OC52-" + key
		status, code := postAsIntegrationClient(t, adopt, withTenantKey(key, fundsFactBody(fact)))
		if status != http.StatusBadRequest || code != "MALFORMED_REQUEST" {
			t.Errorf("采用（键 %q）：答 %d %q，want 400 MALFORMED_REQUEST", key, status, code)
		}
		if own, other := factsOf(fact, "SYN-TENANT-01"), factsOf(fact, otherTenant); own != 0 || other != 0 {
			t.Errorf("采用（键 %q）：册上 SYN-TENANT-01 %d 条、%s %d 条，want 都是零", key, own, otherTenant, other)
		}
		status, code = postAsIntegrationClient(t, correct, withTenantKey(key, fundsCorrectionBody(control)))
		if status != http.StatusBadRequest || code != "MALFORMED_REQUEST" {
			t.Errorf("更正（键 %q）：答 %d %q，want 400 MALFORMED_REQUEST", key, status, code)
		}
	}
	if own, other := versionsOf(control, "SYN-TENANT-01"), versionsOf(control, otherTenant); own != 1 || other != 0 {
		t.Fatalf("更正各变体之后对照事实的版本：SYN-TENANT-01 %d 版、%s %d 版，want 1 与 0", own, otherTenant, other)
	}
}
