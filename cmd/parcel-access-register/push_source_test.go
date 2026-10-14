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

// 本文件证推送源册受控登记口（票 operator-channel/12）：翻译纪律——窗口与时钟偏差缺席不代填，密钥本体进批文即拒；批推进
// ——对真库端到端跑 push-source-register → push-signing-key-register → push-signing-key-retire。

func TestPushSourceBatchRejectsAMissingWindowAndAKeyBody(t *testing.T) {
	missingSkew := `{
	  "tenantId": "SYN-TENANT-01",
	  "sources": [{"sourceId": "SYN-PUSH-SOURCE/bank-01", "issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-01",
	    "windowSeconds": 300, "basis": "SYN-BASIS"}]
	}`
	if _, err := pushSourceBatchFromJSON([]byte(missingSkew)); err == nil {
		t.Fatal("缺 clockSkewSeconds 应拒收，不代填成零")
	}
	withSecret := `{
	  "tenantId": "SYN-TENANT-01",
	  "keys": [{"sourceId": "SYN-PUSH-SOURCE/bank-01", "keyId": "SYN-K1", "form": "HMAC_SHA256",
	    "keyRef": "SYN-PUSH-KEY-REF/SYN-K1", "secret": "not-a-production-secret",
	    "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-BASIS"}]
	}`
	if _, err := pushSigningKeyBatchFromJSON([]byte(withSecret)); err == nil {
		t.Fatal("批文里的密钥本体应被未知字段拒收")
	}
	zeroSkew := `{
	  "tenantId": "SYN-TENANT-01",
	  "sources": [{"sourceId": "SYN-PUSH-SOURCE/bank-01", "issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-01",
	    "windowSeconds": 300, "clockSkewSeconds": 0, "basis": "SYN-BASIS"}]
	}`
	if _, err := pushSourceBatchFromJSON([]byte(zeroSkew)); err != nil {
		t.Fatalf("时钟偏差为零是租户可以作的选择：%v", err)
	}
}

const pushSourceBatch = `{
  "tenantId": "SYN-TENANT-01",
  "sources": [
    {"sourceId": "SYN-PUSH-SOURCE/bank-01", "issuer": "` + syntheticIssuer + `", "subject": "SYN-CLIENT-01",
     "windowSeconds": 300, "clockSkewSeconds": 30, "basis": "SYN-PUSH-SOURCE-BASIS-01"}
  ]
}`

const pushSigningKeyBatch = `{
  "tenantId": "SYN-TENANT-01",
  "keys": [
    {"sourceId": "SYN-PUSH-SOURCE/bank-01", "keyId": "SYN-K-OLD", "form": "HMAC_SHA256",
     "keyRef": "SYN-PUSH-KEY-REF/SYN-K-OLD", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-PUSH-KEY-BASIS-OLD"},
    {"sourceId": "SYN-PUSH-SOURCE/bank-01", "keyId": "SYN-K-NEW", "form": "ED25519",
     "keyRef": "SYN-PUSH-KEY-REF/SYN-K-NEW", "effectiveStartsAt": "2026-06-01T00:00:00Z", "basis": "SYN-PUSH-KEY-BASIS-NEW"}
  ]
}`

const pushSigningKeyRetirementBatch = `{
  "tenantId": "SYN-TENANT-01",
  "retirements": [
    {"sourceId": "SYN-PUSH-SOURCE/bank-01", "keyId": "SYN-K-OLD", "retiresAt": "2026-07-01T00:00:00Z",
     "basis": "SYN-PUSH-KEY-RETIREMENT-BASIS"}
  ]
}`

// Covers: 三个子命令在真库上登记推送源、两把密钥与停用，整批重放以原结果回答；读回的窗口、引用与生效时刻与批文一致，
// 册上没有密钥本体。
func TestPushSourceRegisterKeyAndRetireEndToEnd(t *testing.T) {
	dsn := freshMigratedDSN(t)
	clientPath := batchFile(t, integrationClientBatch)
	if code, out := runCLI(t, dsn, "integration-client-register", "-input", clientPath); code != exitLanded || !strings.Contains(out, "RECORDED") {
		t.Fatalf("登客户端：exit = %d", code)
	}
	for _, step := range []struct {
		name, subcommand, body string
	}{
		{"登推送源", "push-source-register", pushSourceBatch},
		{"登密钥", "push-signing-key-register", pushSigningKeyBatch},
		{"停用", "push-signing-key-retire", pushSigningKeyRetirementBatch},
	} {
		path := batchFile(t, step.body)
		if code, out := runCLI(t, dsn, step.subcommand, "-input", path); code != exitLanded || !strings.Contains(out, "RECORDED") {
			t.Fatalf("%s：exit = %d, want %d 且回显 RECORDED", step.name, code, exitLanded)
		}
		if code, out := runCLI(t, dsn, step.subcommand, "-input", path); code != exitLanded || !strings.Contains(out, "ALREADY_REGISTERED") {
			t.Fatalf("%s重放：exit = %d, want %d 且回显 ALREADY_REGISTERED", step.name, code, exitLanded)
		}
	}

	standing := pushStandingOf(t, dsn)
	if standing.Binding().Window() != 5*time.Minute || standing.Binding().ClockSkew() != 30*time.Second ||
		standing.Binding().Client().Subject() != "SYN-CLIENT-01" || len(standing.Keys()) != 2 {
		t.Fatalf("读回窗口 %s 偏差 %s 客户端 %s 钥 %d", standing.Binding().Window(), standing.Binding().ClockSkew(),
			standing.Binding().Client().Subject(), len(standing.Keys()))
	}
	overlap := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	retired := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	byID := map[string]accessidentity.RecordedPushSigningKey{}
	for _, recorded := range standing.Keys() {
		byID[recorded.Key().KeyID()] = recorded
		if strings.Contains(recorded.Key().Reference().String(), "secret") {
			t.Fatalf("引用里不该有本体：%s", recorded.Key().Reference())
		}
	}
	if !byID["SYN-K-OLD"].EffectiveAt(overlap) || !byID["SYN-K-NEW"].EffectiveAt(overlap) {
		t.Fatal("重叠期两把都应有效")
	}
	if byID["SYN-K-OLD"].EffectiveAt(retired) || !byID["SYN-K-NEW"].EffectiveAt(retired) {
		t.Fatal("停用时刻起只剩新钥")
	}
}

func pushStandingOf(t *testing.T, dsn string) accessidentity.PushSourceStanding {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := aipostgres.NewPushSourceRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	source, err := accessidentity.NewPushSourceID("SYN-PUSH-SOURCE/bank-01")
	if err != nil {
		t.Fatal(err)
	}
	standing, found, err := registry.FindPushSource(t.Context(), source)
	if err != nil || !found {
		t.Fatalf("在册：found %v, err %v", found, err)
	}
	return standing
}
