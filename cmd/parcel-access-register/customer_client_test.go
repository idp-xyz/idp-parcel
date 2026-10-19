package main

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipostgres "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件证客户渠道册受控登记口的两半（票 first-party-api/00 做什么「parcel-access-register 加登记与授予两个子命令，形状照
// 集成客户端那两个」，另加撤销一个）：翻译纪律——批文逐字段过领域构造门，未知字段、缺件、未知能力、倒置区间在触库前拒收，
// 「要不要求证书绑定令牌」缺席同样拒、不代填；批推进——对真库端到端跑 customer-client-register → customer-client-grant →
// customer-client-revoke。

func customerStandingOf(t *testing.T, dsn, sub string) accessidentity.CustomerClientStanding {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := aipostgres.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatalf("构造客户渠道册：%v", err)
	}
	subject, err := accessidentity.NewCustomerClientSubject(syntheticIssuer, sub)
	if err != nil {
		t.Fatalf("客户端主体：%v", err)
	}
	standing, found, err := registry.FindCustomerClient(t.Context(), subject)
	if err != nil || !found {
		t.Fatalf("%s 在册：found %v, err %v", sub, found, err)
	}
	return standing
}

const customerClientBatch = `{
  "tenantId": "SYN-TENANT-01",
  "clients": [
    {"issuer": "` + syntheticIssuer + `", "subject": "SYN-CUSTOMER-CLIENT-01", "customerAccountId": "SYN-ACCOUNT-01",
     "credentialRef": "SYN-CREDENTIAL-REF/customer-01", "certificateBoundTokenRequired": false, "basis": "SYN-CUSTOMER-BASIS-01"}
  ]
}`

const customerClientGrantBatch = `{
  "tenantId": "SYN-TENANT-01",
  "grants": [
    {"grantId": "SYN-CUSTOMER-GRANT-01", "issuer": "` + syntheticIssuer + `", "subject": "SYN-CUSTOMER-CLIENT-01",
     "capability": "SHIPMENT_REQUEST_SUBMISSION", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-CUSTOMER-GRANT-BASIS-01"}
  ]
}`

const customerClientRevocationBatch = `{
  "tenantId": "SYN-TENANT-01",
  "revocations": [
    {"grantId": "SYN-CUSTOMER-GRANT-01", "revokedAt": "2026-03-01T00:00:00Z", "basis": "SYN-CUSTOMER-REVOKE-BASIS-01"}
  ]
}`

// Covers: 票 first-party-api/00 做什么 1——三个子命令在真库上登记首方客户端、按客户面能力授予、撤销，整批重放以原结果回答；
// 授予只在撤销时刻前生效；乙租户登甲的客户端、同租户改指另一个客户账户、授未登记的客户端、撤不存在的授予，各报请人看（退出码
// 2、答复名如实回显），册上不因它们变。集成客户端册的撤销批撤不动客户渠道册的授予：两本册各认各的授予标识。
func TestCustomerClientRegisterGrantRevokeEndToEnd(t *testing.T) {
	dsn := freshMigratedDSN(t)
	for _, step := range []struct {
		name, subcommand, body string
	}{
		{"登客户端", "customer-client-register", customerClientBatch},
		{"授予", "customer-client-grant", customerClientGrantBatch},
		{"撤销", "customer-client-revoke", customerClientRevocationBatch},
	} {
		path := batchFile(t, step.body)
		if code, out := runCLI(t, dsn, step.subcommand, "-input", path); code != exitLanded || !strings.Contains(out, "RECORDED") {
			t.Fatalf("%s：exit = %d, want %d 且回显 RECORDED", step.name, code, exitLanded)
		}
		if code, out := runCLI(t, dsn, step.subcommand, "-input", path); code != exitLanded || !strings.Contains(out, "ALREADY_REGISTERED") {
			t.Fatalf("%s重放：exit = %d, want %d 且回显 ALREADY_REGISTERED", step.name, code, exitLanded)
		}
	}

	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	submission := accessidentity.CustomerCapabilityShipmentRequestSubmission
	customer := customerStandingOf(t, dsn, "SYN-CUSTOMER-CLIENT-01")
	if customer.Binding().CustomerAccountID() != "SYN-ACCOUNT-01" || customer.Binding().CertificateBoundTokenRequired() ||
		!customer.HoldsAt(submission, march.Add(-time.Second)) || customer.HoldsAt(submission, march) {
		t.Fatalf("SYN-CUSTOMER-CLIENT-01 现状 = %+v, want 绑 SYN-ACCOUNT-01、不要求证书绑定、提交授予到撤销时刻止", customer)
	}

	for _, refused := range []struct {
		name, subcommand, body, outcome string
	}{
		{"乙租户登甲的客户端", "customer-client-register",
			strings.Replace(customerClientBatch, `"tenantId": "SYN-TENANT-01"`, `"tenantId": "SYN-TENANT-02"`, 1),
			"CLIENT_BOUND_TO_ANOTHER_TENANT"},
		{"改指另一个客户账户", "customer-client-register",
			strings.Replace(customerClientBatch, `"SYN-ACCOUNT-01"`, `"SYN-ACCOUNT-02"`, 1),
			"CONTENT_CONFLICT"},
		{"授未登记的客户端", "customer-client-grant",
			strings.Replace(customerClientGrantBatch, `"subject": "SYN-CUSTOMER-CLIENT-01"`, `"subject": "SYN-CUSTOMER-GHOST"`, 1),
			"CLIENT_NOT_REGISTERED"},
		{"撤不存在的授予", "customer-client-revoke",
			strings.Replace(customerClientRevocationBatch, `"SYN-CUSTOMER-GRANT-01"`, `"SYN-CUSTOMER-GRANT-GHOST"`, 1),
			"GRANT_NOT_REGISTERED"},
		{"集成客户端册的撤销批撤客户渠道册的授予", "integration-client-revoke",
			strings.Replace(customerClientRevocationBatch, `"2026-03-01T00:00:00Z"`, `"2026-04-01T00:00:00Z"`, 1),
			"GRANT_NOT_REGISTERED"},
	} {
		code, out := runCLI(t, dsn, refused.subcommand, "-input", batchFile(t, refused.body))
		if code != exitAttention || !strings.Contains(out, refused.outcome) {
			t.Fatalf("%s：exit = %d, want %d 且回显 %s", refused.name, code, exitAttention, refused.outcome)
		}
	}
	if after := customerStandingOf(t, dsn, "SYN-CUSTOMER-CLIENT-01"); after.Binding().TenantID() != "SYN-TENANT-01" ||
		after.Binding().CustomerAccountID() != "SYN-ACCOUNT-01" || len(after.Grants()) != 1 {
		t.Fatalf("被拒之后现状 = %+v, want 仍绑甲租户的 SYN-ACCOUNT-01、仍是原一笔授予", after)
	}
}

// Covers: 缺件与形状错在触库之前拒收，绝不代填——「要不要求证书绑定令牌」没有缺省值，缺席即拒；客户账户缺席、来源身份一格
// （集成客户端的列）、未知能力、事实类型一格、倒置区间各自拒收。
func TestCustomerClientBatchTranslationRefusesBeforeTouchingTheDatabase(t *testing.T) {
	clientItem := func(rest string) string {
		return `{"tenantId": "SYN-TENANT-01", "clients": [{"issuer": "` + syntheticIssuer + `", "subject": "SYN-CUSTOMER-CLIENT-01", ` + rest + `}]}`
	}
	const validClientRest = `"customerAccountId": "SYN-ACCOUNT-01", "credentialRef": "r", "certificateBoundTokenRequired": false, "basis": "b"`
	grantItem := func(rest string) string {
		return `{"tenantId": "SYN-TENANT-01", "grants": [{"grantId": "SYN-CUSTOMER-GRANT-01", "issuer": "` + syntheticIssuer +
			`", "subject": "SYN-CUSTOMER-CLIENT-01", ` + rest + `}]}`
	}
	const validGrantRest = `"capability": "SHIPMENT_REQUEST_SUBMISSION", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "b"`
	for name, probe := range map[string]struct {
		translate func([]byte) ([]batchItem, error)
		body      string
	}{
		"客户端基准": {customerClientBatchFromJSON, clientItem(validClientRest)},
		"授予基准":  {customerClientGrantBatchFromJSON, grantItem(validGrantRest)},
		"撤销基准":  {customerClientRevocationBatchFromJSON, customerClientRevocationBatch},
	} {
		if _, err := probe.translate([]byte(probe.body)); err != nil {
			t.Fatalf("%s应译得出，否则下面的拒收证不了各自那一处：%v", name, err)
		}
	}

	for name, probe := range map[string]struct {
		translate func([]byte) ([]batchItem, error)
		body      string
	}{
		"客户端批未知字段": {customerClientBatchFromJSON, clientItem(validClientRest + `, "clientSecret": "s3cr3t"`)},
		"客户端带来源身份": {customerClientBatchFromJSON, clientItem(validClientRest + `, "sourceIdentity": "SYN-SOURCE/x"`)},
		"客户端缺客户账户": {customerClientBatchFromJSON, clientItem(`"credentialRef": "r", "certificateBoundTokenRequired": false, "basis": "b"`)},
		"证书绑定要求缺席": {customerClientBatchFromJSON, clientItem(`"customerAccountId": "SYN-ACCOUNT-01", "credentialRef": "r", "basis": "b"`)},
		"客户端批缺租户":  {customerClientBatchFromJSON, `{"clients": [{"issuer": "i", "subject": "s", ` + validClientRest + `}]}`},
		"授予未知能力":   {customerClientGrantBatchFromJSON, grantItem(`"capability": "shipments:submit", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "b"`)},
		"授予带事实类型":  {customerClientGrantBatchFromJSON, grantItem(validGrantRest + `, "factType": "EXTERNAL_FUNDS_FACT"`)},
		"授予倒置区间":   {customerClientGrantBatchFromJSON, grantItem(validGrantRest + `, "effectiveEndsAt": "2025-12-31T00:00:00Z"`)},
		"授予缺依据":    {customerClientGrantBatchFromJSON, grantItem(`"capability": "SHIPMENT_REQUEST_SUBMISSION", "effectiveStartsAt": "2026-01-01T00:00:00Z"`)},
	} {
		if _, err := probe.translate([]byte(probe.body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", name)
		}
	}
}
