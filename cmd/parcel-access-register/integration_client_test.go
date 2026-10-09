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

// 本文件证集成客户端册受控登记口的两半（票 operator-channel/11 做什么第 1 条）：翻译纪律——批文逐字段过领域构造门，
// 未知字段、缺件、未知事实类型、倒置区间在触库前拒收，「要不要求证书绑定令牌」缺席同样拒、不代填；批推进——对真库
// 端到端跑 integration-client-register → integration-client-grant → integration-client-revoke。

func clientStandingOf(t *testing.T, dsn, sub string) accessidentity.IntegrationClientStanding {
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
	registry, err := aipostgres.NewIntegrationClientRegistry(db)
	if err != nil {
		t.Fatalf("构造集成客户端册：%v", err)
	}
	subject, err := accessidentity.NewIntegrationClientSubject(syntheticIssuer, sub)
	if err != nil {
		t.Fatalf("客户端主体：%v", err)
	}
	standing, found, err := registry.FindIntegrationClient(t.Context(), subject)
	if err != nil || !found {
		t.Fatalf("%s 在册：found %v, err %v", sub, found, err)
	}
	return standing
}

const integrationClientBatch = `{
  "tenantId": "SYN-TENANT-01",
  "clients": [
    {"issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-01", "sourceIdentity": "SYN-SOURCE/bank-01",
     "credentialRef": "SYN-CREDENTIAL-REF/bank-01", "certificateBoundTokenRequired": true, "basis": "SYN-CLIENT-BASIS-01"},
    {"issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-02", "sourceIdentity": "SYN-SOURCE/broker-01",
     "credentialRef": "SYN-CREDENTIAL-REF/broker-01", "certificateBoundTokenRequired": false, "basis": "SYN-CLIENT-BASIS-02"}
  ]
}`

const integrationClientGrantBatch = `{
  "tenantId": "SYN-TENANT-01",
  "grants": [
    {"grantId": "SYN-CLIENT-GRANT-01", "issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-01",
     "factType": "EXTERNAL_FUNDS_FACT", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-CLIENT-GRANT-BASIS-01"},
    {"grantId": "SYN-CLIENT-GRANT-02", "issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-02",
     "factType": "CUSTOMS_EXTERNAL_RESULT", "effectiveStartsAt": "2026-01-01T00:00:00Z",
     "effectiveEndsAt": "2026-07-01T00:00:00Z", "basis": "SYN-CLIENT-GRANT-BASIS-02"},
    {"grantId": "SYN-CLIENT-GRANT-03", "issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-02",
     "factType": "REGULATORY_CREDENTIAL", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-CLIENT-GRANT-BASIS-03"}
  ]
}`

const integrationClientRevocationBatch = `{
  "tenantId": "SYN-TENANT-01",
  "revocations": [
    {"grantId": "SYN-CLIENT-GRANT-03", "revokedAt": "2026-03-01T00:00:00Z", "basis": "SYN-CLIENT-REVOKE-BASIS-01"}
  ]
}`

// Covers: 票 operator-channel/11 做什么第 1 条——三个子命令在真库上登记客户端、按事实类型授予、撤销，整批重放以原结果
// 回答；授予只在区间内、撤销时刻前生效；乙租户登甲的客户端、授未登记的客户端、撤不存在的授予、同标识异内容，各报请人看
// （退出码 2、答复名如实回显），册上不因它们变。操作者册的撤销批撤不动集成客户端册的授予：两本册各认各的授予标识。
func TestIntegrationClientRegisterGrantRevokeEndToEnd(t *testing.T) {
	dsn := freshMigratedDSN(t)
	for _, step := range []struct {
		name, subcommand, body string
	}{
		{"登客户端", "integration-client-register", integrationClientBatch},
		{"授予", "integration-client-grant", integrationClientGrantBatch},
		{"撤销", "integration-client-revoke", integrationClientRevocationBatch},
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
	bank := clientStandingOf(t, dsn, "SYN-CLIENT-01")
	if !bank.Binding().CertificateBoundTokenRequired() || bank.Binding().SourceIdentity() != "SYN-SOURCE/bank-01" ||
		bank.Binding().CredentialReference().String() != "SYN-CREDENTIAL-REF/bank-01" || !bank.HoldsAt(accessidentity.FactExternalFunds, march) {
		t.Fatalf("SYN-CLIENT-01 现状 = %+v, want 要求证书绑定、来源与凭据引用如批文、三月持有外部资金事实", bank)
	}
	broker := clientStandingOf(t, dsn, "SYN-CLIENT-02")
	if broker.Binding().CertificateBoundTokenRequired() ||
		!broker.HoldsAt(accessidentity.FactCustomsExternalResult, march) ||
		broker.HoldsAt(accessidentity.FactCustomsExternalResult, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) ||
		!broker.HoldsAt(accessidentity.FactRegulatoryCredential, march.Add(-time.Second)) ||
		broker.HoldsAt(accessidentity.FactRegulatoryCredential, march) {
		t.Fatalf("SYN-CLIENT-02 现状 = %+v, want 外部结果到七月止、监管凭证自撤销时刻起不生效", broker)
	}

	for _, refused := range []struct {
		name, subcommand, body, outcome string
	}{
		{"乙租户登甲的客户端", "integration-client-register",
			`{"tenantId": "SYN-TENANT-02", "clients": [{"issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-01", "sourceIdentity": "SYN-SOURCE/bank-01",
			  "credentialRef": "SYN-CREDENTIAL-REF/bank-01", "certificateBoundTokenRequired": true, "basis": "SYN-CLIENT-BASIS-09"}]}`,
			"CLIENT_BOUND_TO_ANOTHER_TENANT"},
		{"乙租户授甲的客户端", "integration-client-grant",
			`{"tenantId": "SYN-TENANT-02", "grants": [{"grantId": "SYN-CLIENT-GRANT-09", "issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-01",
			  "factType": "EXTERNAL_FUNDS_FACT", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-CLIENT-GRANT-BASIS-09"}]}`,
			"CLIENT_NOT_REGISTERED"},
		{"撤不存在的授予", "integration-client-revoke",
			`{"tenantId": "SYN-TENANT-01", "revocations": [{"grantId": "SYN-CLIENT-GRANT-GHOST", "revokedAt": "2026-03-01T00:00:00Z", "basis": "SYN-REVOKE-BASIS-09"}]}`,
			"GRANT_NOT_REGISTERED"},
		{"同标识异事实类型", "integration-client-grant",
			`{"tenantId": "SYN-TENANT-01", "grants": [{"grantId": "SYN-CLIENT-GRANT-01", "issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-01",
			  "factType": "CUSTOMS_EXTERNAL_RESULT", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-CLIENT-GRANT-BASIS-01"}]}`,
			"CONTENT_CONFLICT"},
		{"操作者册的撤销批撤客户端的授予", "operator-revoke",
			`{"tenantId": "SYN-TENANT-01", "revocations": [{"grantId": "SYN-CLIENT-GRANT-01", "revokedAt": "2026-03-01T00:00:00Z", "basis": "SYN-REVOKE-BASIS-09"}]}`,
			"GRANT_NOT_REGISTERED"},
	} {
		code, out := runCLI(t, dsn, refused.subcommand, "-input", batchFile(t, refused.body))
		if code != exitAttention || !strings.Contains(out, refused.outcome) {
			t.Fatalf("%s：exit = %d, want %d 且回显 %s", refused.name, code, exitAttention, refused.outcome)
		}
	}
	if after := clientStandingOf(t, dsn, "SYN-CLIENT-01"); after.Binding().TenantID() != "SYN-TENANT-01" ||
		len(after.Grants()) != 1 || !after.HoldsAt(accessidentity.FactExternalFunds, march) {
		t.Fatalf("被拒之后 SYN-CLIENT-01 现状 = %+v, want 仍绑甲租户、仍是原一笔授予且未撤", after)
	}
}

// Covers: 缺件与形状错在触库之前拒收，绝不代填——「要不要求证书绑定令牌」没有缺省值，缺席即拒；未知事实类型、操作者册
// 的能力面字段、倒置区间各自拒收。
func TestIntegrationClientBatchTranslationRefusesBeforeTouchingTheDatabase(t *testing.T) {
	clientItem := func(rest string) string {
		return `{"tenantId": "SYN-TENANT-01", "clients": [{"issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-01", ` + rest + `}]}`
	}
	const validClientRest = `"sourceIdentity": "SYN-SOURCE/bank-01", "credentialRef": "SYN-CREDENTIAL-REF/bank-01", "certificateBoundTokenRequired": false, "basis": "b"`
	grantItem := func(rest string) string {
		return `{"tenantId": "SYN-TENANT-01", "grants": [{"grantId": "SYN-CLIENT-GRANT-01", "issuer": "` + syntheticIssuer +
			`", "subject": "SYN-CLIENT-01", ` + rest + `}]}`
	}
	const validGrantRest = `"factType": "EXTERNAL_FUNDS_FACT", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "b"`
	for name, probe := range map[string]struct {
		translate func([]byte) ([]batchItem, error)
		body      string
	}{
		"客户端基准": {integrationClientBatchFromJSON, clientItem(validClientRest)},
		"授予基准":  {integrationClientGrantBatchFromJSON, grantItem(validGrantRest)},
		"撤销基准":  {integrationClientRevocationBatchFromJSON, integrationClientRevocationBatch},
	} {
		if _, err := probe.translate([]byte(probe.body)); err != nil {
			t.Fatalf("%s应译得出，否则下面的拒收证不了各自那一处：%v", name, err)
		}
	}

	cases := map[string]struct {
		translate func([]byte) ([]batchItem, error)
		body      string
	}{
		"客户端批未知字段": {integrationClientBatchFromJSON, clientItem(validClientRest + `, "clientSecret": "s3cr3t"`)},
		"客户端批缺租户":  {integrationClientBatchFromJSON, `{"clients": [{"issuer": "i", "subject": "s", ` + validClientRest + `}]}`},
		"客户端批空数组":  {integrationClientBatchFromJSON, `{"tenantId": "SYN-TENANT-01", "clients": []}`},
		"客户端缺来源身份": {integrationClientBatchFromJSON, clientItem(`"credentialRef": "r", "certificateBoundTokenRequired": false, "basis": "b"`)},
		"客户端缺凭据引用": {integrationClientBatchFromJSON, clientItem(`"sourceIdentity": "s", "certificateBoundTokenRequired": false, "basis": "b"`)},
		"证书绑定要求缺席": {integrationClientBatchFromJSON, clientItem(`"sourceIdentity": "s", "credentialRef": "r", "basis": "b"`)},
		"证书绑定要求为空": {integrationClientBatchFromJSON, clientItem(`"sourceIdentity": "s", "credentialRef": "r", "certificateBoundTokenRequired": null, "basis": "b"`)},
		"客户端缺依据":   {integrationClientBatchFromJSON, clientItem(`"sourceIdentity": "s", "credentialRef": "r", "certificateBoundTokenRequired": false`)},
		"授予未知事实类型": {integrationClientGrantBatchFromJSON, grantItem(`"factType": "ANYTHING", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "b"`)},
		"授予带能力面":   {integrationClientGrantBatchFromJSON, grantItem(validGrantRest + `, "capabilityFace": "REGISTRY_CONFIGURATION_WRITE"`)},
		"授予缺起点":    {integrationClientGrantBatchFromJSON, grantItem(`"factType": "EXTERNAL_FUNDS_FACT", "basis": "b"`)},
		"授予倒置区间":   {integrationClientGrantBatchFromJSON, grantItem(validGrantRest + `, "effectiveEndsAt": "2025-12-31T00:00:00Z"`)},
		"授予终点给零值":  {integrationClientGrantBatchFromJSON, grantItem(validGrantRest + `, "effectiveEndsAt": "0001-01-01T00:00:00Z"`)},
		"授予缺依据":    {integrationClientGrantBatchFromJSON, grantItem(`"factType": "EXTERNAL_FUNDS_FACT", "effectiveStartsAt": "2026-01-01T00:00:00Z"`)},
		"撤销缺时刻":    {integrationClientRevocationBatchFromJSON, `{"tenantId": "SYN-TENANT-01", "revocations": [{"grantId": "g", "basis": "b"}]}`},
		"撤销缺依据":    {integrationClientRevocationBatchFromJSON, `{"tenantId": "SYN-TENANT-01", "revocations": [{"grantId": "g", "revokedAt": "2026-03-01T00:00:00Z"}]}`},
	}
	for name, probe := range cases {
		if _, err := probe.translate([]byte(probe.body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", name)
		}
	}
}
