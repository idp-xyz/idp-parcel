package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 外部监管舱单引用两口的第二参是真编排（UC-CC-012）——候选读口、引用册、Outbox 意图交付与事务壳在真实 PostgreSQL
// 上装得起来。没有任何可关联的申报单元时首登照样入册、答待关联并交一份意图；同一份重放答已有、不再入队，证首登的事务
// 提交了；来源版本推进形成新版本指回前版、按新版本再交一份意图；没交过这个号的另一家承运商推进它，读的是自己的编号空间，答不在册，在册一行不动、不入队。
// 测试输入是隔离合成，只记 `S`。
func TestTheWiredManifestReferencesRecordAdvanceAndHandOffAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	manifests, err := buildManifestReferencesOrchestration(db)
	if err != nil {
		t.Fatalf("装配外部监管舱单引用编排：%v", err)
	}
	const tenant, manifest = "SYN-TENANT-1", "SYN-MANIFEST-1"
	carrier := mustValue(t, customsdomain.NewCarrierResponsibilityReference, "SYN-PARTY-CARRIER-TRUNK-01")
	receive := customsapp.ReceiveManifestCommand{
		TenantID: mustValue(t, customsdomain.NewTenantID, tenant),
		Spec: customsdomain.ExternalManifestReferenceSpec{
			Manifest:   mustValue(t, customsdomain.NewExternalManifestID, manifest),
			Version:    mustValue(t, customsdomain.NewManifestSourceVersion, "V1"),
			Carrier:    carrier,
			Procedure:  mustValue(t, customsdomain.NewCustomsProcedureReference, "SYN-EXPORT/GENERAL"),
			Direction:  customsdomain.ExportManifest,
			Scope:      mustValue(t, customsdomain.NewDecisionScopeReference, "SYN-MOVEMENT-1"),
			SourceFact: "SYN-CARRIER-FEED/manifest-filed",
			AcceptedAt: time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC),
		},
	}

	recorded, err := manifests.Receive(t.Context(), receive)
	if err != nil {
		t.Fatalf("首登：%v", err)
	}
	if recorded.Outcome() != customsapp.ManifestPendingAssociation || recorded.HandoffReference() != "" {
		t.Fatalf("outcome = %v handoff = %q, want PENDING_ASSOCIATION 且意图已交", recorded.Outcome(), recorded.HandoffReference())
	}
	firstIntent := string(outboxintent.FingerprintEventID("carrier-manifest", tenant, carrier.String(), manifest, "V1"))
	if got := countOutboxRows(t, pool, firstIntent); got != 1 {
		t.Fatalf("首版 outbox 行数 = %d，want 1", got)
	}

	replayed, err := manifests.Receive(t.Context(), receive)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if replayed.Outcome() != customsapp.ManifestExisting {
		t.Fatalf("outcome = %v, want EXISTING——重放没走已有引用，首登的事务没有提交", replayed.Outcome())
	}
	if got := countOutboxRows(t, pool, firstIntent); got != 1 {
		t.Fatalf("重放后首版 outbox 行数 = %d，want 1——重放不该再入队", got)
	}

	revise := customsapp.ReviseManifestCommand{
		TenantID:   receive.TenantID,
		Carrier:    mustValue(t, customsdomain.NewCarrierResponsibilityReference, "SYN-PARTY-CARRIER-OTHER"),
		Manifest:   receive.Spec.Manifest,
		Version:    mustValue(t, customsdomain.NewManifestSourceVersion, "V2"),
		Scope:      mustValue(t, customsdomain.NewDecisionScopeReference, "SYN-MOVEMENT-2"),
		SourceFact: "SYN-CARRIER-FEED/manifest-amended",
		At:         time.Date(2026, 11, 13, 3, 0, 0, 0, time.UTC),
	}
	secondIntent := string(outboxintent.FingerprintEventID("carrier-manifest", tenant, carrier.String(), manifest, "V2"))
	foreign, err := manifests.Revise(t.Context(), revise)
	if err != nil {
		t.Fatalf("别家承运商推进：%v", err)
	}
	if foreign.Outcome() != customsapp.ManifestNotFound {
		t.Fatalf("outcome = %v, want NOT_FOUND——别家承运商读的是自己的编号空间", foreign.Outcome())
	}
	if got := countOutboxRows(t, pool, secondIntent); got != 0 {
		t.Fatalf("别家承运商推进后第二版 outbox 行数 = %d，want 0", got)
	}

	revise.Carrier = carrier
	advanced, err := manifests.Revise(t.Context(), revise)
	if err != nil {
		t.Fatalf("推进：%v", err)
	}
	reference, has := advanced.Reference()
	prior, revised := reference.PriorVersion()
	if advanced.Outcome() != customsapp.ManifestRevised || !has || reference.Version().String() != "V2" || !revised || prior.String() != "V1" {
		t.Fatalf("outcome = %v reference = %+v, want REVISED V2 指回 V1", advanced.Outcome(), reference)
	}
	if got := countOutboxRows(t, pool, secondIntent); got != 1 {
		t.Fatalf("第二版 outbox 行数 = %d，want 1——修订版要按自己的来源版本入队", got)
	}
	var version, scope string
	if err := pool.QueryRow(t.Context(),
		`SELECT version_id, scope_ref FROM customs_compliance.external_manifest_reference WHERE tenant_id = $1 AND carrier_ref = $2 AND manifest_id = $3`,
		tenant, carrier.String(), manifest,
	).Scan(&version, &scope); err != nil {
		t.Fatalf("读在册引用：%v", err)
	}
	if version != "V2" || scope != "SYN-MOVEMENT-2" {
		t.Fatalf("在册引用 version=%q scope=%q，want V2 / SYN-MOVEMENT-2", version, scope)
	}
}
