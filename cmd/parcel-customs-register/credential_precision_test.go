package main

// 票 tf-face-algebra/11 的真库回归：受控登记 CLI 的凭证子命令走同一只 RegisterCredentialHandler，buildRegistrar 接真库（生产
// 装配同一份）。登记册的时刻列只存到微秒：有效期带纳秒的同一份输入跑第二遍，是重放不是冲突。

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func TestCredentialSubcommandReplaysANanosecondValidityAsExistingOnRealDB(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"tenantId":"SYN-TENANT-01","credentialId":"SYN-CRED-TFA11-CLI","issuerRef":"SYN-AUTHORITY-01",` +
		`"holderRef":"SYN-HOLDER-01","procedureRef":"SYN-PROC-01","validFrom":"2026-09-01T00:00:00.123456789Z","validTo":"2027-09-01T00:00:00Z"}`)
	if message, code := execute(t.Context(), commandRegulatoryCredential, raw, registrar); code != exitRegistered || !strings.Contains(message, "REGISTERED") {
		t.Fatalf("第一遍 = %d（%s），want %d 且含 REGISTERED", code, message, exitRegistered)
	}
	if message, code := execute(t.Context(), commandRegulatoryCredential, raw, registrar); code != exitRegistered || !strings.Contains(message, "EXISTING") {
		t.Fatalf("第二遍 = %d（%s），want %d 且含 EXISTING", code, message, exitRegistered)
	}
}
