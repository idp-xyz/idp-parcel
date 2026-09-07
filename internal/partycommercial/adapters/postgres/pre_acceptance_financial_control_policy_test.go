package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证接受前财务控制策略册（票 party-commercial-context-gaps/07，ADR-0115，
// 0024 迁移）：组合控制往返、缺正文是合法缺席、同内容重放、异内容冲突且原行不动、租户是身份不是过滤器、
// 有父无子是坏数据、库上 CHECK 守住三个封闭集与两个唯一性。
//
// 夹具里的范围、责任取值只是取值，不作断言依据——本册的值属实例半边，仓库不持有任何一份。

type controlPolicyFixture struct {
	repository *adapter.CommercialPublications
	contents   *adapter.PreAcceptanceFinancialControlPolicyContents
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
	db         *bentopg.DB
}

func newControlPolicyFixture(t *testing.T) controlPolicyFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	contents, err := adapter.NewPreAcceptanceFinancialControlPolicyContents(db)
	if err != nil {
		t.Fatalf("构造策略正文读口：%v", err)
	}
	return controlPolicyFixture{
		repository: repository,
		contents:   contents,
		transactor: db.Transactor(),
		pool:       pool,
		db:         db,
	}
}

func controlItemRow(
	t *testing.T,
	kind domain.PreAcceptanceControlKind,
	scope string,
	order int,
	disposition domain.ControlFailureDisposition,
	responsibility string,
) domain.PreAcceptanceControlItem {
	t.Helper()
	item, err := domain.NewPreAcceptanceControlItem(
		kind,
		pcValue(t, domain.NewChargeScopeReference, scope),
		order,
		disposition,
		pcValue(t, domain.NewControlResponsibilityReference, responsibility),
	)
	if err != nil {
		t.Fatalf("控制项：%v", err)
	}
	return item
}

func controlPolicyOn(
	t *testing.T,
	version domain.CommercialVersion,
	items ...domain.PreAcceptanceControlItem,
) domain.PreAcceptanceFinancialControlPolicy {
	t.Helper()
	policy, err := domain.NewPreAcceptanceFinancialControlPolicy(version, domain.AllControlsPass, items)
	if err != nil {
		t.Fatalf("new control policy: %v", err)
	}
	return policy
}

func saveControlPolicy(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	policy domain.PreAcceptanceFinancialControlPolicy,
) ports.PreAcceptanceFinancialControlPolicySaveOutcome {
	t.Helper()
	var outcome ports.PreAcceptanceFinancialControlPolicySaveOutcome
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.SavePreAcceptanceFinancialControlPolicy(txCtx, policy)
		return err
	})
	return outcome
}

func mustSaveControlPolicy(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	policy domain.PreAcceptanceFinancialControlPolicy,
) {
	t.Helper()
	if outcome := saveControlPolicy(t, transactor, ctx, repository, policy); outcome != ports.PreAcceptanceFinancialControlPolicySaved {
		t.Fatalf("save outcome = %q, want SAVED", outcome)
	}
}

// Covers: ADR-0115 Decision 二——组合控制逐行往返：种类 × 范围 × 顺序 × 处置 × 责任，读回按判断顺序，
// 且经领域构造门重建（壳与正文同一版本）；共同通过条件随父行往返。
func TestAControlPolicyRoundTripsItsCombination(t *testing.T) {
	fixture := newControlPolicyFixture(t)
	repository, contents, transactor := fixture.repository, fixture.contents, fixture.transactor
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	version := effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1", "v1", "digest-fcp1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveControlPolicy(t, transactor, ctx, repository, controlPolicyOn(t, version,
		controlItemRow(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.AuthorizedDispositionOnControlFailure, "operator-legal-1"),
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "customer-1"),
		controlItemRow(t, domain.CreditCheckControl, "charge-scope-b", 3, domain.RejectOnControlFailure, "customer-1"),
	))

	policy, found, err := contents.LoadPreAcceptanceFinancialControlPolicy(ctx, tenant, version)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	if !policy.Version().SameVersionAs(version) {
		t.Fatal("正文挂回了另一个版本")
	}
	if policy.JointPassCondition() != domain.AllControlsPass {
		t.Fatalf("共同通过条件变形：%v", policy.JointPassCondition())
	}
	items := policy.Items()
	if len(items) != 3 {
		t.Fatalf("控制项 %d 行, want 3", len(items))
	}
	if items[0].Kind() != domain.PrepaidFreezeControl || items[0].EvaluationOrder() != 1 ||
		items[0].Scope().String() != "charge-scope-a" || items[0].FailureDisposition() != domain.RejectOnControlFailure ||
		items[0].Responsibility().String() != "customer-1" {
		t.Fatalf("第一项变形：%#v", items[0])
	}
	if items[1].Kind() != domain.CreditCheckControl || items[1].EvaluationOrder() != 2 ||
		items[1].FailureDisposition() != domain.AuthorizedDispositionOnControlFailure ||
		items[1].Responsibility().String() != "operator-legal-1" {
		t.Fatalf("第二项变形：%#v", items[1])
	}
	if items[2].Scope().String() != "charge-scope-b" || items[2].EvaluationOrder() != 3 {
		t.Fatalf("第三项变形：%#v", items[2])
	}
	if scoped := policy.ItemsFor(pcValue(t, domain.NewChargeScopeReference, "charge-scope-b")); len(scoped) != 1 {
		t.Fatalf("按范围取控制项 = %#v", scoped)
	}
}

// Covers: 缺正文是合法缺席（found=false），不是 error，也不是任何默认控制——settlement-accounting 据以停在
// `未配置`格；凑一份就是发明实例参数。
func TestAControlPolicyVersionWithoutContentIsNotFound(t *testing.T) {
	fixture := newControlPolicyFixture(t)
	repository, contents, transactor := fixture.repository, fixture.contents, fixture.transactor
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-bare", "v1", "digest-bare")
	mustSaveVersion(t, transactor, ctx, repository, version)

	policy, found, err := contents.LoadPreAcceptanceFinancialControlPolicy(ctx, pcTenant(t, "tenant-1"), version)
	if err != nil {
		t.Fatalf("没正文被当成了错误：%v", err)
	}
	if found {
		t.Fatalf("没登记正文却读回了一份：%#v", policy)
	}
	if len(policy.Items()) != 0 {
		t.Fatal("未登记的正文交回了控制项")
	}
}

// Covers: ADR-0031——同内容重放答`已登记`；改一项的顺序、处置或责任、多一行少一行，都是`内容冲突`；两者
// 都不是 error，且原正文一行不动。冲突路径一行不写，否则「绝不覆盖」只对父行成立。
func TestSavingAControlPolicyTwiceIsAReplayAndAChangedItemConflicts(t *testing.T) {
	fixture := newControlPolicyFixture(t)
	repository, contents, transactor := fixture.repository, fixture.contents, fixture.transactor
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	version := effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1", "v1", "digest-fcp1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveControlPolicy(t, transactor, ctx, repository, controlPolicyOn(t, version,
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "customer-1"),
		controlItemRow(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.RejectOnControlFailure, "customer-1"),
	))

	// 重放：同一份正文，声明顺序不同也算同一份——构造门已按判断顺序归档。
	replay := controlPolicyOn(t, version,
		controlItemRow(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.RejectOnControlFailure, "customer-1"),
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "customer-1"),
	)
	if outcome := saveControlPolicy(t, transactor, ctx, repository, replay); outcome != ports.PreAcceptanceFinancialControlPolicyAlreadyRegistered {
		t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
	}

	swappedOrder := controlPolicyOn(t, version,
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 2, domain.RejectOnControlFailure, "customer-1"),
		controlItemRow(t, domain.CreditCheckControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "customer-1"),
	)
	if outcome := saveControlPolicy(t, transactor, ctx, repository, swappedOrder); outcome != ports.PreAcceptanceFinancialControlPolicyContentConflict {
		t.Fatalf("swapped order outcome = %q, want CONTENT_CONFLICT", outcome)
	}

	changedDisposition := controlPolicyOn(t, version,
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "customer-1"),
		controlItemRow(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.AuthorizedDispositionOnControlFailure, "customer-1"),
	)
	if outcome := saveControlPolicy(t, transactor, ctx, repository, changedDisposition); outcome != ports.PreAcceptanceFinancialControlPolicyContentConflict {
		t.Fatalf("changed disposition outcome = %q, want CONTENT_CONFLICT", outcome)
	}

	changedResponsibility := controlPolicyOn(t, version,
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "operator-legal-1"),
		controlItemRow(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.RejectOnControlFailure, "customer-1"),
	)
	if outcome := saveControlPolicy(t, transactor, ctx, repository, changedResponsibility); outcome != ports.PreAcceptanceFinancialControlPolicyContentConflict {
		t.Fatalf("changed responsibility outcome = %q, want CONTENT_CONFLICT", outcome)
	}

	fewerItems := controlPolicyOn(t, version,
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "customer-1"),
	)
	if outcome := saveControlPolicy(t, transactor, ctx, repository, fewerItems); outcome != ports.PreAcceptanceFinancialControlPolicyContentConflict {
		t.Fatalf("fewer items outcome = %q, want CONTENT_CONFLICT", outcome)
	}

	policy, found, err := contents.LoadPreAcceptanceFinancialControlPolicy(ctx, tenant, version)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	items := policy.Items()
	if len(items) != 2 {
		t.Fatalf("冲突写入改动了行数：%d", len(items))
	}
	if items[0].Kind() != domain.PrepaidFreezeControl || items[0].Responsibility().String() != "customer-1" ||
		items[1].FailureDisposition() != domain.RejectOnControlFailure {
		t.Fatalf("冲突写入改动了原正文：%#v", items)
	}
}

// Covers: ADR-0003——租户是身份不是过滤器。拿另一个租户去读本租户的版本是 error 且不交内容；他租户登记
// 的同名版本不进本租户的读口。
func TestControlPolicyContentIsBoundToItsTenant(t *testing.T) {
	fixture := newControlPolicyFixture(t)
	repository, contents, transactor := fixture.repository, fixture.contents, fixture.transactor
	ctx := t.Context()

	mine := policyVersionInTenant(t, "tenant-1", domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1", "v1", "digest-mine")
	theirs := policyVersionInTenant(t, "tenant-2", domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveControlPolicy(t, transactor, ctx, repository, controlPolicyOn(t, theirs,
		controlItemRow(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure, "customer-1"),
	))

	if _, found, err := contents.LoadPreAcceptanceFinancialControlPolicy(ctx, pcTenant(t, "tenant-2"), mine); err == nil || found {
		t.Fatalf("拿他租户身份读本租户版本：found=%v err=%v，应是 error 且不交内容", found, err)
	}
	if _, found, err := contents.LoadPreAcceptanceFinancialControlPolicy(ctx, pcTenant(t, "tenant-1"), mine); err != nil || found {
		t.Fatalf("他租户的正文进了本租户的读口：found=%v err=%v", found, err)
	}
}

// Covers: 有父行而零子行是坏数据（领域要求至少一项）：error，不是 found=false——把损坏的正文伪装成从未
// 登记会让消费方去催一份其实已经写坏的配置。
func TestAControlPolicyParentWithoutItemsIsBadData(t *testing.T) {
	fixture := newControlPolicyFixture(t)
	repository, contents, transactor, pool := fixture.repository, fixture.contents, fixture.transactor, fixture.pool
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-empty", "v1", "digest-empty")
	mustSaveVersion(t, transactor, ctx, repository, version)
	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.pre_acceptance_financial_control_policy
			(tenant_id, object_kind, object_id, version_label, joint_pass_condition)
		 VALUES ('tenant-1', 5, 'fcp-empty', 'v1', 'ALL_CONTROLS_PASS')`); err != nil {
		t.Fatalf("直接写父行：%v", err)
	}

	_, found, err := contents.LoadPreAcceptanceFinancialControlPolicy(ctx, pcTenant(t, "tenant-1"), version)
	if err == nil || found {
		t.Fatalf("有父无子被读成了：found=%v err=%v", found, err)
	}
	if !errors.Is(err, domain.ErrInvalidPreAcceptanceFinancialControlPolicy) {
		t.Fatalf("err = %v, want ErrInvalidPreAcceptanceFinancialControlPolicy", err)
	}
}

// Covers: 库上 CHECK 守住绕开构造门的那条路——类别钉在第 5 类、共同通过条件与控制种类与失败处置三个
// 封闭集、顺序为正、（种类 × 范围）与顺序两个唯一性。领域构造门拦得住经它进来的，CHECK 拦的是直接写表的。
// 尤其要证的是**没有「无控制」那一格**：写一行 NO_CONTROL 进不去（ADR-0115 Decision 一）。
func TestControlPolicyColumnsRejectShapesTheDomainRefuses(t *testing.T) {
	fixture := newControlPolicyFixture(t)
	repository, transactor, pool := fixture.repository, fixture.transactor, fixture.pool
	ctx := t.Context()
	version := effectiveVersionOfKind(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1", "v1", "digest-fcp1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	contract := effectiveVersionOfKind(t, domain.CustomerContractObject, "contract-1", "v1", "digest-contract1")
	mustSaveVersion(t, transactor, ctx, repository, contract)

	insertParent := func(kind int, objectID, jointPass string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.pre_acceptance_financial_control_policy
				(tenant_id, object_kind, object_id, version_label, joint_pass_condition)
			 VALUES ('tenant-1', $1, $2, 'v1', $3)`, kind, objectID, jointPass)
		return err
	}
	if err := insertParent(2, "contract-1", "ALL_CONTROLS_PASS"); err == nil {
		t.Fatal("挂在客户合同版本上的策略正文进了策略册")
	}
	if err := insertParent(5, "fcp-1", "ANY_CONTROL_PASSES"); err == nil {
		t.Fatal("集外的共同通过条件进了策略册")
	}
	if err := insertParent(5, "fcp-1", "ALL_CONTROLS_PASS"); err != nil {
		t.Fatalf("合法父行进不去：%v", err)
	}

	insertItem := func(kind, scope string, order int, disposition, responsibility string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.pre_acceptance_financial_control_item
				(tenant_id, object_kind, object_id, version_label,
				 control_kind, charge_scope_ref, evaluation_order, failure_disposition, responsibility_ref)
			 VALUES ('tenant-1', 5, 'fcp-1', 'v1', $1, $2, $3, $4, $5)`,
			kind, scope, order, disposition, responsibility)
		return err
	}
	if err := insertItem("NO_CONTROL", "charge-scope-a", 1, "REJECT", "customer-1"); err == nil {
		t.Fatal("「无控制」进了控制项表——那一格该只在合同声明里")
	}
	if err := insertItem("SOMETHING_ELSE", "charge-scope-a", 1, "REJECT", "customer-1"); err == nil {
		t.Fatal("集外的控制种类进了控制项表")
	}
	if err := insertItem("PREPAID_FREEZE", "charge-scope-a", 1, "ALLOW", "customer-1"); err == nil {
		t.Fatal("集外的失败处置进了控制项表")
	}
	if err := insertItem("PREPAID_FREEZE", "charge-scope-a", 0, "REJECT", "customer-1"); err == nil {
		t.Fatal("零顺序进了控制项表")
	}
	if err := insertItem("PREPAID_FREEZE", "  ", 1, "REJECT", "customer-1"); err == nil {
		t.Fatal("空白范围进了控制项表")
	}
	if err := insertItem("PREPAID_FREEZE", "charge-scope-a", 1, "REJECT", " "); err == nil {
		t.Fatal("空白责任引用进了控制项表")
	}
	if err := insertItem("PREPAID_FREEZE", "charge-scope-a", 1, "REJECT", "customer-1"); err != nil {
		t.Fatalf("合法控制项进不去：%v", err)
	}
	if err := insertItem("PREPAID_FREEZE", "charge-scope-a", 2, "AUTHORIZED_DISPOSITION", "customer-1"); err == nil {
		t.Fatal("同一范围上同一种控制第二行进了控制项表")
	}
	if err := insertItem("CREDIT_CHECK", "charge-scope-a", 1, "REJECT", "customer-1"); err == nil {
		t.Fatal("两行抢同一个判断顺序进了控制项表")
	}
	if err := insertItem("CREDIT_CHECK", "charge-scope-a", 2, "AUTHORIZED_DISPOSITION", "operator-legal-1"); err != nil {
		t.Fatalf("合法的第二项进不去：%v", err)
	}
}
