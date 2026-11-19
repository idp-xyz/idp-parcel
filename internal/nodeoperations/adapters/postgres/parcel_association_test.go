package postgres_test

import (
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func TestTheCurrentAssociationDropsASucceededJudgment(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	associations, err := adapter.NewParcelAssociations(db)
	if err != nil {
		t.Fatal(err)
	}
	const insert = `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
		 service_markers, recorded_at, judgment_version, predecessor_version)
		VALUES ('tenant-a', $1, 'digest', 'INTAKE_FORMED', $2::jsonb, '{}'::jsonb, '[]'::jsonb, false,
		        '[]'::jsonb, now(), $3, $4)`
	if _, err := pool.Exec(t.Context(), insert, "src-old",
		`{"unit":"unit-old","association":"parcel-1","version":"v1"}`, "v1", nil); err != nil {
		t.Fatalf("旧判断：%v", err)
	}
	if _, err := pool.Exec(t.Context(), insert, "src-new",
		`{"unit":"unit-new","association":"parcel-1","version":"v2"}`, "v2", "v1"); err != nil {
		t.Fatalf("当前判断：%v", err)
	}
	units, err := associations.ListHandlingUnits(t.Context(), ref(t, domain.NewTenantID, "tenant-a"), ref(t, domain.NewParcelAssociationReference, "parcel-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].String() != "unit-new" {
		t.Fatalf("关联 = %v", units)
	}
}

// Covers: 反方向的列表与 ListHandlingUnits 共用「只留判断链当前版本」这一句：同一件作业实物被后来的判断改关联到别的包裹之后，
// 只列得出当前关联着它的那个包裹。
func TestTheCurrentAssociationListsOnlyTheParcelOfTheCurrentJudgmentForAUnit(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	associations, err := adapter.NewParcelAssociations(db)
	if err != nil {
		t.Fatal(err)
	}
	const insert = `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
		 service_markers, recorded_at, judgment_version, predecessor_version)
		VALUES ('tenant-a', $1, 'digest', 'INTAKE_FORMED', $2::jsonb, '{}'::jsonb, '[]'::jsonb, false,
		        '[]'::jsonb, now(), $3, $4)`
	if _, err := pool.Exec(t.Context(), insert, "src-old",
		`{"unit":"unit-1","association":"parcel-old","version":"v1"}`, "v1", nil); err != nil {
		t.Fatalf("旧判断：%v", err)
	}
	if _, err := pool.Exec(t.Context(), insert, "src-new",
		`{"unit":"unit-1","association":"parcel-new","version":"v2"}`, "v2", "v1"); err != nil {
		t.Fatalf("当前判断：%v", err)
	}
	parcels, err := associations.ListParcels(t.Context(), ref(t, domain.NewTenantID, "tenant-a"), ref(t, domain.NewHandlingUnitID, "unit-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(parcels) != 1 || parcels[0].String() != "parcel-new" {
		t.Fatalf("关联 = %v", parcels)
	}
}
