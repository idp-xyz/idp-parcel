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
// 虚构，而合同不覆盖是终局格（ADR-0051）——那一拒不得经补充翻案。
func TestAbsentClaimDeclarationIsNotDeclaredAtAll(t *testing.T) {
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
		rules, declared, err := fixture.viewFor(t, testCase.tenant).
			RulesForClaim(t.Context(), eligibilityQuery(t, testCase.contract, "LOSS"))
		if err != nil || declared {
			t.Fatalf("%s：err=%v declared=%v", testCase.name, err, declared)
		}
		// 声明不在场时连规则版本都交不出来——编排据此停在未决，而不是拿一份空规则
		// 去逐维核对，那会让每一维都「核不了」而看着像登记漏了很多样。
		if rules.RuleVersion != "" || rules.KindCovered {
			t.Fatalf("%s：空声明下仍交出了规则 %+v", testCase.name, rules)
		}
	}
}

// 声明在场而该类型不在覆盖集合内：这一维完整且永久成立——承担与否不随材料补充
// 而变，变了就是换了合同范围，而换范围按 CONTEXT 是另一个索赔项。目录只把这个事实
// 与它的版本交出去，「所以不予受理」由编排定（切块 (b)）。
func TestKindOutsideContractScopeIsReportedUncoveredWithItsRuleVersion(t *testing.T) {
	fixture := newEligibilityFixture(t)
	fixture.declare(t, "tenant-a", "contract/v1", "claim-rules/v1", "DAMAGE", "DELAY")

	rules, declared, err := fixture.viewFor(t, "tenant-a").
		RulesForClaim(t.Context(), eligibilityQuery(t, "contract/v1", "LOSS"))
	if err != nil || !declared {
		t.Fatalf("不在保：err=%v declared=%v", err, declared)
	}
	if rules.KindCovered {
		t.Fatal("不在覆盖集合内却报成了在保")
	}
	// 版本随规则交出：由它得出的不予受理是永久的，事后必须追得回依据的是哪一版声明。
	if !strings.Contains(rules.RuleVersion, "claim-rules/v1") {
		t.Fatalf("规则版本 = %q，追不回是哪一版声明", rules.RuleVersion)
	}
}

// 类型在保时目录如实报在保，但**另外三维一律未登记**：索赔时限要起算事件与业务日历、
// 最低材料要一份清单、授权要一份申请人目录，三样都属 `PAR-VIS-08` 待登记实例参数，
// 本上下文还没有那个登记面。凑一份就是发明实例参数，编排据此停在指名到维的未决。
func TestCoveredKindStillLeavesTheOtherThreeRulesUnregistered(t *testing.T) {
	fixture := newEligibilityFixture(t)
	fixture.declare(t, "tenant-a", "contract/v1", "claim-rules/v1", "DAMAGE")

	rules, declared, err := fixture.viewFor(t, "tenant-a").
		RulesForClaim(t.Context(), eligibilityQuery(t, "contract/v1", "DAMAGE"))
	if err != nil || !declared {
		t.Fatalf("在保：err=%v declared=%v", err, declared)
	}
	if !rules.KindCovered {
		t.Fatal("在覆盖集合内却报成了不在保")
	}
	if rules.FilingDeadline.Registered {
		t.Fatalf("凭空登记了首次索赔期限：%+v", rules.FilingDeadline)
	}
	if rules.Materials.Registered {
		t.Fatalf("凭空登记了最低材料清单：%+v", rules.Materials)
	}
	if rules.Authorization.Registered {
		t.Fatalf("凭空登记了授权目录：%+v", rules.Authorization)
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
