package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 两个凭证登记口的第二参是真编排（票 label-channel/18）——凭证登记册在真实 PostgreSQL 上装得
// 起来，事务边界成立（重放走已有版本，证首笔真的提交了）；改变适用关系经真装配换出新版本并回指前版。
// 测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredCredentialRegistrarAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	registrar, err := buildExternalCarrierCredentialRegistration(db)
	if err != nil {
		t.Fatalf("装配凭证登记编排：%v", err)
	}

	register := tfapp.RegisterExternalCarrierCredentialCommand{
		TenantID:       mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Credential:     "SYN-CARRIER-X/1Z-000001",
		Version:        "SYN-ECV-000000000001",
		Assigner:       "SYN-CARRIER-X",
		IdentifiedKind: "CARRIED_OBJECT",
		IdentifiedRef:  "SYN-PARCEL-1",
		EffectiveFrom:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}
	registered, err := registrar.Register(t.Context(), register)
	if err != nil {
		t.Fatalf("首登：%v", err)
	}
	if got := registered.Outcome(); got != tfapp.CredentialRegistered {
		t.Fatalf("outcome = %v, want CREDENTIAL_REGISTERED", got)
	}

	replay, err := registrar.Register(t.Context(), register)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.CredentialExistingVersion {
		t.Fatalf("outcome = %v, want EXISTING_VERSION——重放没走已有版本，首笔事务没有提交", got)
	}

	changed, err := registrar.ChangeApplicability(t.Context(), tfapp.ChangeCredentialApplicabilityCommand{
		TenantID:   register.TenantID,
		Credential: register.Credential,
		Change:     tfdomain.CredentialRevoked,
		At:         time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC),
		NewVersion: "SYN-ECV-000000000002",
	})
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	if got := changed.Outcome(); got != tfapp.CredentialApplicabilityChanged {
		t.Fatalf("outcome = %v, want APPLICABILITY_CHANGED", got)
	}
	record, has := changed.Record()
	if !has {
		t.Fatal("改变成功却没带回记录")
	}
	if prior, present := record.Credential.Supersedes(); !present || prior.String() != register.Version {
		t.Fatalf("supersedes = (%q, %v)，新版本没有回指首版", prior.String(), present)
	}
}
