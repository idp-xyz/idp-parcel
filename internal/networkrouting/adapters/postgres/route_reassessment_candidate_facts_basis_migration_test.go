package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// Covers: 复核记录依据的封闭集去掉 PHYSICAL_WITH_DECLARATION（迁移 0035；CONTEXT「物理事实整份取用」）。库里要是真有这个值
// 的行，加约束时失败、行原样不动、旧约束仍在——这是该停下来看的信号，迁移不用 NOT VALID 绕过；没有这样的行（生产装配从未装过
// 能取得物理事实的来源）时照常施加，之后这个值被库拒，其余各值照收。
// 从空库按序施加本上下文的迁移，停在 0035 之前落一行带该值的复核记录，再施加 0035。
func TestMigration0035RetiresPhysicalWithDeclarationOnlyWhenNoRowCarriesIt(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA network_routing`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.NetworkRouting()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0035_route_reassessment_candidate_facts_basis_physical_with_declaration_retired"
	var retirement *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			retirement = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if retirement == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}

	insert := func(correlation string, basis *string) error {
		_, err := conn.Exec(ctx,
			`INSERT INTO network_routing.route_reassessment
				(tenant_id, correlation_id, customer_account_id, shipment_request_id,
				 acceptance_baseline, declared_parcel_id, service_purpose,
				 conclusion, reviewed_plan, lapse_basis, candidate_state, candidate_facts_basis,
				 trigger_occurred_at, reassessed_at)
			 VALUES ('SYN-TENANT-NR35', $1, 'SYN-CUSTOMER-NR35', 'SYN-REQUEST-NR35',
			         'baseline-v1', 'SYN-PARCEL-NR35', 'LAST_MILE_DELIVERY',
			         'PLAN_LAPSED', 'RPV-0001', 'closure-7', 'NO_QUALIFIED_CANDIDATES', $2,
			         now(), now())`, correlation, basis)
		return err
	}
	text := func(value string) *string { return &value }
	constraintDefinition := func() string {
		var definition string
		if err := conn.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
			WHERE conrelid = 'network_routing.route_reassessment'::regclass
			  AND conname = 'route_reassessment_candidate_facts_basis_closed'`).Scan(&definition); err != nil {
			t.Fatalf("读依据封闭集的约束：%v", err)
		}
		return definition
	}

	if err := insert("legacy-physical-with-declaration", text("PHYSICAL_WITH_DECLARATION")); err != nil {
		t.Fatalf("落 0035 之前带 PHYSICAL_WITH_DECLARATION 的行：%v", err)
	}
	if _, err := conn.Exec(ctx, retirement.SQL); err == nil {
		t.Fatal("库里有行带 PHYSICAL_WITH_DECLARATION，0035 照样施加了")
	}
	var stored string
	if err := conn.QueryRow(ctx, `SELECT candidate_facts_basis FROM network_routing.route_reassessment
		WHERE correlation_id = 'legacy-physical-with-declaration'`).Scan(&stored); err != nil || stored != "PHYSICAL_WITH_DECLARATION" {
		t.Fatalf("拒绝施加之后行变了：%q %v", stored, err)
	}
	if definition := constraintDefinition(); !strings.Contains(definition, "PHYSICAL_WITH_DECLARATION") {
		t.Fatalf("拒绝施加之后旧约束没有留在原处：%s", definition)
	}

	if _, err := conn.Exec(ctx, `UPDATE network_routing.route_reassessment SET candidate_facts_basis = NULL`); err != nil {
		t.Fatalf("置空：%v", err)
	}
	if _, err := conn.Exec(ctx, retirement.SQL); err != nil {
		t.Fatalf("置空之后施加 %s：%v", target, err)
	}
	if definition := constraintDefinition(); strings.Contains(definition, "PHYSICAL_WITH_DECLARATION") {
		t.Fatalf("0035 之后封闭集里仍有 PHYSICAL_WITH_DECLARATION：%s", definition)
	}
	if err := insert("after-retired", text("PHYSICAL_WITH_DECLARATION")); err == nil {
		t.Fatal("0035 之后库仍收 PHYSICAL_WITH_DECLARATION")
	}
	for name, basis := range map[string]*string{
		"physical": text("PHYSICAL"), "fallback": text("DECLARATION_FALLBACK"),
		"unresolved": text("PHYSICAL_UNRESOLVED"), "none": text("NONE"), "never-priced": nil,
	} {
		if err := insert("after-retired-"+name, basis); err != nil {
			t.Fatalf("0035 之后 %s 应照收：%v", name, err)
		}
	}
}
