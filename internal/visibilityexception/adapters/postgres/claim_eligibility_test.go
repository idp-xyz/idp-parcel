package postgres_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

type eligibilityFixture struct {
	pool *pgxpool.Pool
	db   *bentopg.DB
}

func newEligibilityFixture(t *testing.T) *eligibilityFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return &eligibilityFixture{pool: pool, db: db}
}

func (fixture *eligibilityFixture) viewFor(t *testing.T, tenant string) *adapter.ClaimEligibilityRules {
	t.Helper()
	var tenantID domain.TenantID
	if tenant != "" {
		tenantID = projectionValue(t, domain.NewTenantID, tenant)
	}
	view, err := adapter.NewClaimEligibilityRules(fixture.db, tenantID)
	if err != nil {
		t.Fatalf("构造资格视图：%v", err)
	}
	return view
}

func (fixture *eligibilityFixture) declare(t *testing.T, tenant, contract, ruleVersion string, kinds ...string) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.claim_contract_scope
			(tenant_id, contract_scope_ref, rule_version, approved_by)
		 VALUES ($1, $2, $3, 'customer-service')`,
		tenant, contract, ruleVersion); err != nil {
		t.Fatalf("登记索赔声明 %s：%v", contract, err)
	}
	for _, kind := range kinds {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.claim_covered_kind
				(tenant_id, contract_scope_ref, claim_kind_ref)
			 VALUES ($1, $2, $3)`,
			tenant, contract, kind); err != nil {
			t.Fatalf("登记覆盖类型 %s：%v", kind, err)
		}
	}
}

func eligibilityQuery(t *testing.T, contract, kind string) ports.EligibilityQuery {
	t.Helper()
	return ports.EligibilityQuery{
		Batch:    projectionValue(t, domain.NewClaimBatchReference, "batch-1"),
		Item:     projectionValue(t, domain.NewClaimItemID, "item-1"),
		Customer: projectionValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Contract: projectionValue(t, domain.NewContractScopeReference, contract),
		Target:   projectionValue(t, domain.NewRequestScopeReference, "parcel-1"),
		Kind:     projectionValue(t, domain.NewClaimKindReference, kind),
	}
}

// 声明不在场时缺一个类型是「没人声明过」，不是「声明说不保」。凭一张空表拒赔就是
// 虚构，而 ScreenEligibility 一次性——那一拒永远翻不了案。
func TestAbsentClaimDeclarationIsNotConfiguredNotIneligible(t *testing.T) {
	fixture := newEligibilityFixture(t)
	fixture.declare(t, "tenant-a", "contract/v1", "claim-rules/v1", "DAMAGE")

	for _, testCase := range []struct {
		name     string
		tenant   string
		contract string
	}{
		{"合同没有声明行", "tenant-a", "contract/v9"},
		{"租户未登记", "", "contract/v1"},
		{"跨租户", "tenant-b", "contract/v1"},
	} {
		answer, configured, err := fixture.viewFor(t, testCase.tenant).
			ScreenClaim(t.Context(), eligibilityQuery(t, testCase.contract, "LOSS"))
		if err != nil || configured {
			t.Fatalf("%s：err=%v configured=%v", testCase.name, err, configured)
		}
		if answer.Screen == domain.ClaimIneligible {
			t.Fatalf("%s：空声明下拒了赔", testCase.name)
		}
	}
}

// 声明在场而该类型不在覆盖集合内：这一维完整且永久成立——承担与否不随材料补充
// 而变，变了就是换了合同范围，而换范围按 CONTEXT 是另一个索赔项。
func TestKindOutsideContractScopeIsIneligibleWithTraceableBasis(t *testing.T) {
	fixture := newEligibilityFixture(t)
	fixture.declare(t, "tenant-a", "contract/v1", "claim-rules/v1", "DAMAGE", "DELAY")

	answer, configured, err := fixture.viewFor(t, "tenant-a").
		ScreenClaim(t.Context(), eligibilityQuery(t, "contract/v1", "LOSS"))
	if err != nil || !configured {
		t.Fatalf("不在保：err=%v configured=%v", err, configured)
	}
	if answer.Screen != domain.ClaimIneligible {
		t.Fatalf("不在保却没判不通过：screen=%d", answer.Screen)
	}
	// 依据是唯一会被永久记进索赔项的东西，必须点得出是哪条判据、哪个类型、哪一版声明。
	for _, fragment := range []string{"CLAIM_KIND_NOT_IN_CONTRACT_SCOPE", "LOSS", "claim-rules/v1"} {
		if !strings.Contains(answer.Basis, fragment) {
			t.Fatalf("依据 %q 里没有 %q", answer.Basis, fragment)
		}
	}
}

// 类型在保并不等于资格通过：索赔时限、申请人授权、最低材料与重复关系四维都还证不了，
// 而 Screen 是封闭二值、一次性。此时必须停在未配置，绝不能凑一个`通过`出来——那会
// 把四维未核的索赔永久标成已过审。
func TestCoveredKindStillStopsAtNotConfigured(t *testing.T) {
	fixture := newEligibilityFixture(t)
	fixture.declare(t, "tenant-a", "contract/v1", "claim-rules/v1", "DAMAGE")

	answer, configured, err := fixture.viewFor(t, "tenant-a").
		ScreenClaim(t.Context(), eligibilityQuery(t, "contract/v1", "DAMAGE"))
	if err != nil || configured {
		t.Fatalf("在保：err=%v configured=%v", err, configured)
	}
	if answer.Screen == domain.ClaimEligible {
		t.Fatal("四维未核却凑出了一个通过")
	}
}

func TestClaimEligibilityChecksRejectUnusableRows(t *testing.T) {
	fixture := newEligibilityFixture(t)
	fixture.declare(t, "tenant-a", "contract/v1", "claim-rules/v1")

	// 覆盖行挂在未登记的声明下：孤立行会让「声明在场」这个前提失真，而那条永久
	// 判定完全建立在它之上。
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.claim_covered_kind
			(tenant_id, contract_scope_ref, claim_kind_ref)
		 VALUES ('tenant-a', 'contract/v9', 'DAMAGE')`); err == nil {
		t.Fatal("库接受了挂在未登记声明下的覆盖类型")
	}
	// 声明缺规则版本：依据就追不回它依据的是哪一版。
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.claim_contract_scope
			(tenant_id, contract_scope_ref, rule_version, approved_by)
		 VALUES ('tenant-a', 'contract/v2', '   ', 'customer-service')`); err == nil {
		t.Fatal("库接受了没有规则版本的索赔声明")
	}
}
