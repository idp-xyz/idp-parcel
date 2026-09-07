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

// Covers: 两个总单登记口的第二参是真编排（ADR-0113 决定五）——总单登记册在真实 PostgreSQL 上装得起来，事务
// 边界成立（重放走已有版本，证首笔连同关联子表真的提交了）；关联重述经真装配换出新版本并回指前版、关联集换
// 成新的一组；撤销之后再改答已不适用。测试输入是隔离合成，只记 `S`，不含任何真实总单号，不进生产装配。
func TestTheWiredMasterDocumentRegistrarAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	registrar, err := buildMasterDocumentRegistration(db)
	if err != nil {
		t.Fatalf("装配总单登记编排：%v", err)
	}

	register := tfapp.RegisterMasterDocumentCommand{
		TenantID: mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Document: "SYN-MAWB-000-00000001",
		Version:  "SYN-MDV-000000000001",
		Issuer:   "SYN-CARRIER-X",
		Scope:    "SYN-LANE-1",
		Associations: []tfapp.MasterDocumentAssociationInput{
			{Kind: "CONSOLIDATION_UNIT", Reference: "SYN-CU-1"},
		},
	}
	registered, err := registrar.Register(t.Context(), register)
	if err != nil {
		t.Fatalf("首登：%v", err)
	}
	if got := registered.Outcome(); got != tfapp.MasterDocumentRegistered {
		t.Fatalf("outcome = %v, want MASTER_DOCUMENT_REGISTERED", got)
	}

	replay, err := registrar.Register(t.Context(), register)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.MasterDocumentExistingVersion {
		t.Fatalf("outcome = %v, want EXISTING_VERSION——重放没走已有版本，首笔事务没有提交", got)
	}
	if record, has := replay.Record(); !has || len(record.Document.Associations()) != 1 {
		t.Fatal("重放带回的版本没有关联——子表那一半没有随首笔提交")
	}

	restated, err := registrar.Revise(t.Context(), tfapp.ReviseMasterDocumentCommand{
		TenantID:   register.TenantID,
		Document:   register.Document,
		Revision:   tfdomain.MasterDocumentAssociationRestatement,
		At:         time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC),
		NewVersion: "SYN-MDV-000000000002",
		Associations: []tfapp.MasterDocumentAssociationInput{
			{Kind: "CONSOLIDATION_UNIT", Reference: "SYN-CU-1"},
			{Kind: "CONSOLIDATION_UNIT", Reference: "SYN-CU-2"},
		},
	})
	if err != nil {
		t.Fatalf("关联重述：%v", err)
	}
	if got := restated.Outcome(); got != tfapp.MasterDocumentRevised {
		t.Fatalf("outcome = %v, want MASTER_DOCUMENT_REVISED", got)
	}
	record, has := restated.Record()
	if !has {
		t.Fatal("重述成功却没带回记录")
	}
	if prior, present := record.Document.Supersedes(); !present || prior.String() != register.Version {
		t.Fatalf("supersedes = (%q, %v)，新版本没有回指首版", prior.String(), present)
	}
	if len(record.Document.Associations()) != 2 || !record.Document.InForce() {
		t.Fatalf("重述版应仍有效且带两条关联：%s %d", record.Document.Standing(), len(record.Document.Associations()))
	}

	revoked, err := registrar.Revise(t.Context(), tfapp.ReviseMasterDocumentCommand{
		TenantID:   register.TenantID,
		Document:   register.Document,
		Revision:   tfdomain.MasterDocumentRevocation,
		At:         time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC),
		NewVersion: "SYN-MDV-000000000003",
	})
	if err != nil || revoked.Outcome() != tfapp.MasterDocumentRevised {
		t.Fatalf("撤销：%v %v", err, revoked.Outcome())
	}
	afterRevocation, err := registrar.Revise(t.Context(), tfapp.ReviseMasterDocumentCommand{
		TenantID:    register.TenantID,
		Document:    register.Document,
		Revision:    tfdomain.MasterDocumentSupersession,
		At:          time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC),
		NewVersion:  "SYN-MDV-000000000004",
		Replacement: "SYN-MAWB-000-00000002",
	})
	if err != nil || afterRevocation.Outcome() != tfapp.MasterDocumentNoLongerInForce {
		t.Fatalf("已撤销再替代应答已不适用：%v %v", err, afterRevocation.Outcome())
	}
}
