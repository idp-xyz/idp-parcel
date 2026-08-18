package postgres_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证接受前财务控制声明读口。重点在 found=false 是「未配置」
// 而不是`不适用`：CONTEXT 禁止用缺失结果或默认通过代替「明确无控制」，两格若被读成同一
// 件事，首发没有租户时唯一走得到的分支就会变成一次静默放行。

func TestAnUndeclaredContractIsNotConfiguredForPreAcceptanceControl(t *testing.T) {
	declarations, _ := newPreAcceptanceControlDeclarations(t)
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")

	declaration, found, err := declarations.LoadPreAcceptanceControl(
		t.Context(), pcTenant(t, "tenant-1"), contract)
	if err != nil {
		t.Fatalf("未配置被当成错误：%v", err)
	}
	if found {
		t.Fatal("查无声明却答已配置")
	}
	if declaration.Requirement().Declared() {
		t.Fatal("未配置的答复带了内容——那等于替租户拟了一份声明")
	}
	if _, notApplicable := declaration.NotApplicableBasis(); notApplicable {
		t.Fatal("缺声明被读成了`不适用`——正是 CONTEXT 禁止的默认通过")
	}
}

// TestDeclaredPreAcceptanceControlRoundTrips 证声明保真：`要求`不带依据回来，`不适用`
// 带着原依据回来。两格若在往返中串味，消费方会从结算方式倒推控制要不要。
func TestDeclaredPreAcceptanceControlRoundTrips(t *testing.T) {
	declarations, pool := newPreAcceptanceControlDeclarations(t)
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")

	declarePreAcceptanceControl(t, pool, "tenant-1", "contract-1", "v1", "REQUIRED", nil)

	declaration, found, err := declarations.LoadPreAcceptanceControl(
		t.Context(), pcTenant(t, "tenant-1"), contract)
	if err != nil || !found {
		t.Fatalf("读回要求控制：found = %v err = %v", found, err)
	}
	if declaration.Requirement() != domain.PreAcceptanceControlRequired {
		t.Fatalf("requirement = %q，声明说 REQUIRED", declaration.Requirement())
	}
	if _, notApplicable := declaration.NotApplicableBasis(); notApplicable {
		t.Fatal("`要求控制`带着不适用依据回来了")
	}

	other := effectiveContract(t, "contract-2", "v1", "digest-2")
	basis := "CONTRACT-CLAUSE/NO-PRE-ACCEPTANCE-CONTROL"
	declarePreAcceptanceControl(t, pool, "tenant-1", "contract-2", "v1", "NOT_APPLICABLE", &basis)

	declaration, found, err = declarations.LoadPreAcceptanceControl(
		t.Context(), pcTenant(t, "tenant-1"), other)
	if err != nil || !found {
		t.Fatalf("读回不适用：found = %v err = %v", found, err)
	}
	got, ok := declaration.NotApplicableBasis()
	if declaration.Requirement() != domain.PreAcceptanceControlNotApplicable ||
		!ok || got.String() != basis {
		t.Fatalf("declaration = %#v；`不适用`必须带原依据回来", declaration)
	}
}

// TestPreAcceptanceControlDeclarationsAreScopedByTenantAndVersion 证圈定：他租户与
// 同对象另一版本都读不到本版本的声明——换版本就是换一份正文。
func TestPreAcceptanceControlDeclarationsAreScopedByTenantAndVersion(t *testing.T) {
	declarations, pool := newPreAcceptanceControlDeclarations(t)
	declarePreAcceptanceControl(t, pool, "tenant-1", "contract-1", "v1", "REQUIRED", nil)

	if _, found, err := declarations.LoadPreAcceptanceControl(
		t.Context(), pcTenant(t, "tenant-b"), contractVersionInTenant(t, "tenant-b", "contract-1", "v1", "digest-1"),
	); err != nil || found {
		t.Fatalf("他租户读到了本租户的声明：found = %v err = %v", found, err)
	}
	if _, found, err := declarations.LoadPreAcceptanceControl(
		t.Context(), pcTenant(t, "tenant-1"), effectiveContract(t, "contract-1", "v2", "digest-2"),
	); err != nil || found {
		t.Fatalf("换版本读到了上一版的声明：found = %v err = %v", found, err)
	}
}

// TestMismatchedTenantDoesNotReturnEitherTenantsControlDeclaration 证租户身份闭包：
// 两租户合法同号，用 A 的租户参数配 B 的合同对象不得读回任一方声明。按 A 查库再用 B
// 重建，会把 A 的「要不要」装进 B 的合同。
func TestMismatchedTenantDoesNotReturnEitherTenantsControlDeclaration(t *testing.T) {
	declarations, pool := newPreAcceptanceControlDeclarations(t)
	declarePreAcceptanceControl(t, pool, "tenant-a", "contract-1", "v1", "REQUIRED", nil)
	basis := "CONTRACT-CLAUSE/TENANT-B"
	declarePreAcceptanceControl(t, pool, "tenant-b", "contract-1", "v1", "NOT_APPLICABLE", &basis)

	theirs := contractVersionInTenant(t, "tenant-b", "contract-1", "v1", "digest-b")
	got, found, err := declarations.LoadPreAcceptanceControl(t.Context(), pcTenant(t, "tenant-a"), theirs)
	if err == nil || found {
		t.Fatalf("租户不一致被收下：found = %v err = %v", found, err)
	}
	if got.Requirement().Declared() {
		t.Fatalf("交回了声明 %q——不得返回任一方内容", got.Requirement())
	}
	if _, notApplicable := got.NotApplicableBasis(); notApplicable {
		t.Fatal("交回了 B 的不适用依据")
	}
}

// TestPreAcceptanceControlInvariantsAreMirroredInTheDatabase 证库内守住四条：要求封闭
// 集、合同对象钉死、`不适用`必须带依据、`要求`不得带依据。最后两条是本表的要害——一行
// `不适用`而依据为空，读回来就是一次默认放行。
func TestPreAcceptanceControlInvariantsAreMirroredInTheDatabase(t *testing.T) {
	_, pool := newPreAcceptanceControlDeclarations(t)

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.pre_acceptance_control_declaration
			(tenant_id, object_kind, object_id, version_label, requirement, not_applicable_basis)
		 VALUES ('tenant-1', 2, 'contract-9', 'v1', 'MAYBE', NULL)`,
	); err == nil {
		t.Fatal("集合外的要求取值进了声明表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.pre_acceptance_control_declaration
			(tenant_id, object_kind, object_id, version_label, requirement, not_applicable_basis)
		 VALUES ('tenant-1', 1, 'contract-9', 'v1', 'REQUIRED', NULL)`,
	); err == nil {
		t.Fatal("非客户合同对象进了声明表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.pre_acceptance_control_declaration
			(tenant_id, object_kind, object_id, version_label, requirement, not_applicable_basis)
		 VALUES ('tenant-1', 2, 'contract-9', 'v1', 'NOT_APPLICABLE', NULL)`,
	); err == nil {
		t.Fatal("一条没有依据的`不适用`进了表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.pre_acceptance_control_declaration
			(tenant_id, object_kind, object_id, version_label, requirement, not_applicable_basis)
		 VALUES ('tenant-1', 2, 'contract-9', 'v1', 'NOT_APPLICABLE', '  ')`,
	); err == nil {
		t.Fatal("一条空白依据的`不适用`进了表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.pre_acceptance_control_declaration
			(tenant_id, object_kind, object_id, version_label, requirement, not_applicable_basis)
		 VALUES ('tenant-1', 2, 'contract-9', 'v1', 'REQUIRED', 'should-not-be-here')`,
	); err == nil {
		t.Fatal("`要求控制`带着不适用依据进了表")
	}
}

func newPreAcceptanceControlDeclarations(t *testing.T) (*adapter.PreAcceptanceControlDeclarations, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	declarations, err := adapter.NewPreAcceptanceControlDeclarations(db)
	if err != nil {
		t.Fatalf("构造接受前财务控制声明读口：%v", err)
	}
	return declarations, pool
}

func declarePreAcceptanceControl(
	t *testing.T,
	pool *pgxpool.Pool,
	tenant, objectID, label, requirement string,
	basis *string,
) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.pre_acceptance_control_declaration
			(tenant_id, object_kind, object_id, version_label, requirement, not_applicable_basis)
		 VALUES ($1, 2, $2, $3, $4, $5)`,
		tenant, objectID, label, requirement, basis); err != nil {
		t.Fatalf("登记接受前财务控制声明：%v", err)
	}
}
