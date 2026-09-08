package postgres_test

import (
	"context"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证信用政策册（票 party-commercial-context-gaps/03）：金额与比例
// 两格各自往返、缺正文是合法缺席、同内容重放、异内容冲突、租户是身份不是过滤器、额度两列
// 恰一非空由库上 CHECK 守住、目录上列带额度格。

func newCreditPolicyContents(t *testing.T) (*adapter.CommercialPublications, *adapter.CreditPolicyContents, *adapter.OperationsCatalogue, bentoapp.Transactor) {
	t.Helper()
	repository, transactor, db := newDeclarationFixture(t)
	contents, err := adapter.NewCreditPolicyContents(db)
	if err != nil {
		t.Fatalf("构造信用政策读口：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读面：%v", err)
	}
	return repository, contents, catalogue, transactor
}

func creditPolicyOn(
	t *testing.T,
	version domain.CommercialVersion,
	chargeType string,
	limit domain.CreditLimit,
) domain.CreditPolicy {
	t.Helper()
	policy, err := domain.NewCreditPolicy(
		version,
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewAuthorityLevel, "level-commercial"),
		pcValue(t, domain.NewChargeTypeReference, chargeType),
		limit,
		version.Effective(),
	)
	if err != nil {
		t.Fatalf("new credit policy: %v", err)
	}
	return policy
}

func creditAmountLimit(t *testing.T, minor int64) domain.CreditLimit {
	t.Helper()
	limit, err := domain.NewCreditAmountLimit(minor)
	if err != nil {
		t.Fatalf("金额额度：%v", err)
	}
	return limit
}

func creditRatioLimit(t *testing.T, bps int64) domain.CreditLimit {
	t.Helper()
	limit, err := domain.NewCreditRatioLimit(bps)
	if err != nil {
		t.Fatalf("比例额度：%v", err)
	}
	return limit
}

func mustSaveCreditPolicy(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	policy domain.CreditPolicy,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveCreditPolicy(txCtx, policy)
		if err != nil {
			return err
		}
		if outcome != ports.CreditPolicySaved {
			t.Fatalf("save outcome = %q, want SAVED", outcome)
		}
		return nil
	})
}

// Covers: CONTEXT「按责任法人、业务角色、费用类型、金额或比例形成版本」——两格各自往返，
// 读回的那一格就是写下的那一格，另一格答「不在场」。
func TestCreditPolicyRoundTripsInEitherLimitForm(t *testing.T) {
	repository, contents, _, transactor := newCreditPolicyContents(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	t.Run("amount", func(t *testing.T) {
		version := effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-amount", "v1", "digest-ca")
		mustSaveVersion(t, transactor, ctx, repository, version)
		mustSaveCreditPolicy(t, transactor, ctx, repository,
			creditPolicyOn(t, version, "charge-freight", creditAmountLimit(t, 500000)))

		policy, found, err := contents.LoadCreditPolicy(ctx, tenant, version)
		if err != nil || !found {
			t.Fatalf("读回：found=%v err=%v", found, err)
		}
		if minor, ok := policy.AuthorizedLimit().AmountMinor(); !ok || minor != 500000 {
			t.Fatalf("额度 = (%d, %v), want 500000", minor, ok)
		}
		if _, ok := policy.AuthorizedLimit().RatioBasisPoints(); ok {
			t.Fatal("金额额度读回来多了一格比例")
		}
		if policy.ChargeType().String() != "charge-freight" || policy.LegalEntity().String() != "legal-1" {
			t.Fatalf("正文被改动了：%#v", policy)
		}
		if !policy.Version().SameVersionAs(version) {
			t.Fatal("正文挂回了另一个版本")
		}
	})

	t.Run("ratio", func(t *testing.T) {
		version := effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-ratio", "v1", "digest-cr")
		mustSaveVersion(t, transactor, ctx, repository, version)
		mustSaveCreditPolicy(t, transactor, ctx, repository,
			creditPolicyOn(t, version, "charge-freight", creditRatioLimit(t, 1500)))

		policy, found, err := contents.LoadCreditPolicy(ctx, tenant, version)
		if err != nil || !found {
			t.Fatalf("读回：found=%v err=%v", found, err)
		}
		if bps, ok := policy.AuthorizedLimit().RatioBasisPoints(); !ok || bps != 1500 {
			t.Fatalf("额度 = (%d, %v), want 1500 bps", bps, ok)
		}
		if _, ok := policy.AuthorizedLimit().AmountMinor(); ok {
			t.Fatal("比例额度读回来多了一格金额")
		}
	})
}

// Covers: 缺正文是合法缺席（found=false），不是 error，也不是零额度——缺政策既不是无限信用
// 也不是零额度，该是哪一种只有拥有商业依据的一方能说。
func TestACreditPolicyVersionWithoutContentIsNotFound(t *testing.T) {
	repository, contents, _, transactor := newCreditPolicyContents(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-bare", "v1", "digest-cb")
	mustSaveVersion(t, transactor, ctx, repository, version)

	policy, found, err := contents.LoadCreditPolicy(ctx, pcTenant(t, "tenant-1"), version)
	if err != nil {
		t.Fatalf("没正文被当成了错误：%v", err)
	}
	if found {
		t.Fatalf("没登记正文却读回了一份：%#v", policy)
	}
	if _, ok := policy.AuthorizedLimit().AmountMinor(); ok {
		t.Fatal("未登记的正文交回了一个金额")
	}
}

// Covers: ADR-0031——同内容重放答`已登记`，异内容（换一格额度）答`内容冲突`，两者都不是 error，
// 且原行一字不动。
func TestSavingACreditPolicyTwiceIsAReplayAndAChangedLimitConflicts(t *testing.T) {
	repository, contents, _, transactor := newCreditPolicyContents(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-1", "v1", "digest-c1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	original := creditPolicyOn(t, version, "charge-freight", creditAmountLimit(t, 500000))
	mustSaveCreditPolicy(t, transactor, ctx, repository, original)

	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveCreditPolicy(txCtx, original)
		if err != nil {
			return err
		}
		if outcome != ports.CreditPolicyAlreadyRegistered {
			t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
		}
		return nil
	})

	// 同一个数换一格：金额 500000 与比例 500000 bps 在「一个数加一列标记」的表形上会撞成同一行，
	// 并存两列则是一次可见的内容冲突。
	changed := creditPolicyOn(t, version, "charge-freight", creditRatioLimit(t, 500000))
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SaveCreditPolicy(txCtx, changed)
		if err != nil {
			return err
		}
		if outcome != ports.CreditPolicyContentConflict {
			t.Fatalf("conflict outcome = %q, want CONTENT_CONFLICT", outcome)
		}
		return nil
	})

	policy, found, err := contents.LoadCreditPolicy(ctx, pcTenant(t, "tenant-1"), version)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	if minor, ok := policy.AuthorizedLimit().AmountMinor(); !ok || minor != 500000 {
		t.Fatalf("冲突写入改动了原行：%#v", policy.AuthorizedLimit())
	}
}

// Covers: ADR-0003——租户是身份不是过滤器。拿另一个租户去读本租户的版本是 error 且不交内容；
// 他租户登记的同名版本不进本租户的读口。
func TestCreditPolicyContentIsBoundToItsTenant(t *testing.T) {
	repository, contents, _, transactor := newCreditPolicyContents(t)
	ctx := t.Context()

	mine := policyVersionInTenant(t, "tenant-1", domain.CreditPolicyObject, "credit-1", "v1", "digest-mine")
	theirs := policyVersionInTenant(t, "tenant-2", domain.CreditPolicyObject, "credit-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveCreditPolicy(t, transactor, ctx, repository,
		creditPolicyOn(t, theirs, "charge-freight", creditAmountLimit(t, 900000)))

	if _, found, err := contents.LoadCreditPolicy(ctx, pcTenant(t, "tenant-2"), mine); err == nil || found {
		t.Fatalf("拿他租户身份读本租户版本：found=%v err=%v，应是 error 且不交内容", found, err)
	}
	if _, found, err := contents.LoadCreditPolicy(ctx, pcTenant(t, "tenant-1"), mine); err != nil || found {
		t.Fatalf("他租户的正文进了本租户的读口：found=%v err=%v", found, err)
	}
}

// Covers: 额度两列恰一非空由库上 CHECK 守住——两空与两满都进不来，负值也进不来。领域构造门
// 拦得住经它进来的，CHECK 拦的是绕开它的那条路。
func TestCreditLimitColumnsRejectBothEmptyBothSetAndNegative(t *testing.T) {
	repository, transactor, pool := newPublications(t)
	ctx := t.Context()
	version := effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-1", "v1", "digest-c1")
	mustSaveVersion(t, transactor, ctx, repository, version)

	insert := func(limitMinor, limitBps string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.credit_policy
				(tenant_id, object_kind, object_id, version_label,
				 legal_entity_ref, authority_level_ref, charge_type_ref,
				 limit_minor, limit_ratio_bps, effective_starts_at)
			 VALUES ('tenant-1', 8, 'credit-1', 'v1',
			         'legal-1', 'level-commercial', 'charge-freight',
			         `+limitMinor+`, `+limitBps+`, now())`)
		return err
	}
	if err := insert("NULL", "NULL"); err == nil {
		t.Fatal("两格都空的额度进了信用政策册")
	}
	if err := insert("100", "100"); err == nil {
		t.Fatal("两格都有的额度进了信用政策册")
	}
	if err := insert("-1", "NULL"); err == nil {
		t.Fatal("负金额进了信用政策册")
	}
	if err := insert("NULL", "-1"); err == nil {
		t.Fatal("负比例进了信用政策册")
	}
}

// Covers: 目录上列信用政策册（ports.CommercialPolicyCatalogueRead 注释「正文表落库时按封闭集
// 扩方法」），额度格由 HasAmount 说明哪一列在场；跨租户不可见。
func TestCreditPolicyCatalogueListsBothLimitForms(t *testing.T) {
	repository, _, catalogue, transactor := newCreditPolicyContents(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	amount := effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-amount", "v1", "digest-ca")
	ratio := effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-ratio", "v1", "digest-cr")
	theirs := policyVersionInTenant(t, "tenant-2", domain.CreditPolicyObject, "credit-theirs", "v1", "digest-ct")
	for _, version := range []domain.CommercialVersion{amount, ratio, theirs} {
		mustSaveVersion(t, transactor, ctx, repository, version)
	}
	mustSaveCreditPolicy(t, transactor, ctx, repository,
		creditPolicyOn(t, amount, "charge-freight", creditAmountLimit(t, 500000)))
	mustSaveCreditPolicy(t, transactor, ctx, repository,
		creditPolicyOn(t, ratio, "charge-surcharge", creditRatioLimit(t, 1500)))
	mustSaveCreditPolicy(t, transactor, ctx, repository,
		creditPolicyOn(t, theirs, "charge-freight", creditAmountLimit(t, 1)))

	rows, err := catalogue.ListCreditPolicies(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列信用政策：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("上列 %d 行，want 2（他租户的不得进本租户目录）", len(rows))
	}
	byObject := map[string]ports.CreditPolicyRow{}
	for _, row := range rows {
		byObject[row.ObjectID] = row
	}
	if row := byObject["credit-amount"]; !row.HasAmount || row.LimitMinor != 500000 || row.ChargeType != "charge-freight" {
		t.Fatalf("金额行 = %#v", row)
	}
	if row := byObject["credit-ratio"]; row.HasAmount || row.LimitRatioBasisPoints != 1500 || row.ChargeType != "charge-surcharge" {
		t.Fatalf("比例行 = %#v", row)
	}
	if _, err := catalogue.ListCreditPolicies(ctx, tenant, 0); err == nil {
		t.Fatal("limit 非正应被拒")
	}
}

// Covers: ADR-0127 决定三——信用政策正文进整册装载：LoadForScope 把 0020 的行过 NewCreditPolicy 进
// 登记册，闭包解析才选得出额度；登记推动范围的 ViewRevision；光有版本没有正文是合法缺席；他租户
// 的正文不进本租户的册。
func TestCreditPolicyEntersTheScopeRegistryAndMovesItsViewRevision(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()
	tenant, scope := pcTenant(t, "tenant-1"), pcScope(t)

	version := effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-1", "v1", "digest-c1")
	theirs := policyVersionInTenant(t, "tenant-2", domain.CreditPolicyObject, "credit-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveCreditPolicy(t, transactor, ctx, repository,
		creditPolicyOn(t, theirs, "charge-freight", creditAmountLimit(t, 1)))

	bare, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("登记正文前读回：%v", err)
	}
	if bare.Count() != 1 {
		t.Fatalf("版本数 = %d, want 1", bare.Count())
	}
	if policies := bare.CreditPolicies(); len(policies) != 0 {
		t.Fatalf("没登记正文（或只有他租户登记）却读回 %d 份信用政策", len(policies))
	}
	before := bare.ViewRevision(tenant, scope)

	mustSaveCreditPolicy(t, transactor, ctx, repository,
		creditPolicyOn(t, version, "charge-freight", creditRatioLimit(t, 2500)))

	loaded, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("登记正文后读回：%v", err)
	}
	policies := loaded.CreditPolicies()
	if len(policies) != 1 {
		t.Fatalf("读回 %d 份信用政策，want 1", len(policies))
	}
	if bps, ok := policies[0].AuthorizedLimit().RatioBasisPoints(); !ok || bps != 2500 {
		t.Fatalf("limit = %#v, want 2500 bps", policies[0].AuthorizedLimit())
	}
	if policies[0].ChargeType().String() != "charge-freight" || policies[0].Level().String() != "level-commercial" {
		t.Fatalf("正文读回后变了形：%#v", policies[0])
	}
	if !policies[0].Version().SameVersionAs(version) {
		t.Fatal("正文挂回了另一个版本")
	}
	if loaded.ViewRevision(tenant, scope) == before {
		t.Fatal("信用政策从缺席变为在场，范围修订却没动——先前解析的失效检测看不见它")
	}
}
