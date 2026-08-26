package postgres_test

import (
	"strings"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

func multiTenantQuery(t *testing.T, tenant, contract, kind string) ports.EligibilityQuery {
	t.Helper()
	query := eligibilityQuery(t, contract, kind)
	if tenant != "" {
		query.Tenant = projectionValue(t, domain.NewTenantID, tenant)
	}
	return query
}

// Covers: 票 ve-claims-read-seams/01 的主张——同一个视图实例按查询里的租户各答各的
// 册。两租户各登一版声明后互不串答；没登记的第三户答「声明不在场」的业务格（不是
// 报错）——那正是多租户入口该有的答案，接线未决从此与「待登记」分开。
func TestMultiTenantViewAnswersEachTenantFromItsOwnCatalogue(t *testing.T) {
	fixture := newEligibilityFixture(t)
	fixture.declare(t, "tenant-a", "contract/v1", "claim-rules/a1", "DAMAGE")
	fixture.declare(t, "tenant-b", "contract/v1", "claim-rules/b1", "LOSS")

	view, err := adapter.NewMultiTenantClaimEligibilityRules(fixture.db)
	if err != nil {
		t.Fatalf("构造多租户资格视图：%v", err)
	}

	forA, declared, err := view.RulesForClaim(t.Context(), multiTenantQuery(t, "tenant-a", "contract/v1", "DAMAGE"))
	if err != nil || !declared {
		t.Fatalf("tenant-a：err=%v declared=%v", err, declared)
	}
	if !strings.Contains(forA.RuleVersion, "claim-rules/a1") || !forA.KindCovered {
		t.Fatalf("tenant-a 没按自己的册作答：%+v", forA)
	}

	// 同一份查询字段、只换租户：b 的册里 DAMAGE 没登记，覆盖判定必须换答案——
	// 这一格若与 a 相同，租户维就没真的进查询。
	forB, declared, err := view.RulesForClaim(t.Context(), multiTenantQuery(t, "tenant-b", "contract/v1", "DAMAGE"))
	if err != nil || !declared {
		t.Fatalf("tenant-b：err=%v declared=%v", err, declared)
	}
	if !strings.Contains(forB.RuleVersion, "claim-rules/b1") || forB.KindCovered {
		t.Fatalf("tenant-b 没按自己的册作答：%+v", forB)
	}

	absent, declared, err := view.RulesForClaim(t.Context(), multiTenantQuery(t, "tenant-c", "contract/v1", "DAMAGE"))
	if err != nil || declared {
		t.Fatalf("未登记租户应答声明不在场：err=%v declared=%v", err, declared)
	}
	if absent.RuleVersion != "" {
		t.Fatalf("未登记租户仍交出了规则：%+v", absent)
	}
}

// Covers: 两个形状各自的守卫。多租户视图收到不带租户的查询按「依赖调不通」报错——
// 命令的租户由领域校验非空，走到这里还缺是接线错误，折成「声明不在场」会把恢复动作
// 指去登记声明；现绑视图收到带租户且与钉住不一致的查询同样报错——沉默地用钉住租户
// 作答会把接错装成接对，而带一致租户或不带租户（登记口既有调用面）照常作答。
func TestTenantGuardsRefuseMiswiredQueries(t *testing.T) {
	fixture := newEligibilityFixture(t)
	fixture.declare(t, "tenant-a", "contract/v1", "claim-rules/a1", "DAMAGE")

	multi, err := adapter.NewMultiTenantClaimEligibilityRules(fixture.db)
	if err != nil {
		t.Fatalf("构造多租户资格视图：%v", err)
	}
	if _, _, err := multi.RulesForClaim(t.Context(), multiTenantQuery(t, "", "contract/v1", "DAMAGE")); err == nil {
		t.Fatal("不带租户的查询没有按接线错误报错")
	}

	pinned := fixture.viewFor(t, "tenant-a")
	if _, _, err := pinned.RulesForClaim(t.Context(), multiTenantQuery(t, "tenant-b", "contract/v1", "DAMAGE")); err == nil {
		t.Fatal("现绑视图对错配租户没有报错——它会用钉住租户作答，把接错装成接对")
	}
	matched, declared, err := pinned.RulesForClaim(t.Context(), multiTenantQuery(t, "tenant-a", "contract/v1", "DAMAGE"))
	if err != nil || !declared || !matched.KindCovered {
		t.Fatalf("一致租户应照常作答：err=%v declared=%v rules=%+v", err, declared, matched)
	}
	bare, declared, err := pinned.RulesForClaim(t.Context(), eligibilityQuery(t, "contract/v1", "DAMAGE"))
	if err != nil || !declared || !bare.KindCovered {
		t.Fatalf("不带租户（登记口既有调用面）应照常作答：err=%v declared=%v rules=%+v", err, declared, bare)
	}
}
