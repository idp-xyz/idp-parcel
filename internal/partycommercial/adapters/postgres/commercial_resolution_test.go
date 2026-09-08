package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func TestAFixedResolutionRoundTripsByID(t *testing.T) {
	repository, transactor, _ := newResolutions(t)
	ctx := t.Context()
	closure := uniqueClosure(t)

	mustSaveResolution(t, transactor, ctx, repository, closure)

	found, ok, err := repository.LoadResolution(ctx, closure.ResolutionKey().TenantID, closure.ResolutionID())
	if err != nil {
		t.Fatalf("按标识取回：%v", err)
	}
	if !ok {
		t.Fatal("第一阶段写入后第二阶段 found=false")
	}
	if found.ResolutionID() != closure.ResolutionID() {
		t.Fatalf("resolution ID = %q, want %q", found.ResolutionID(), closure.ResolutionID())
	}
	if found.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q", found.Outcome())
	}
	if _, ok := found.AdoptedFor(domain.AcceptanceRulePackageObject); !ok {
		t.Fatal("读回的闭包丢了接单规则包——第二阶段要的正是它")
	}
}

// Covers: ADR-0050 — 解析闭包库存产品快照。形态随已采用项落在既有 snapshot JSON
// 上，不另开查询键；缺席的既有行读回仍不可观察。
func TestResolutionRoundTripsTheAdoptedServiceProduct(t *testing.T) {
	repository, transactor, _ := newResolutions(t)
	ctx := t.Context()
	closure := uniqueClosureWithServiceProduct(t)

	mustSaveResolution(t, transactor, ctx, repository, closure)

	found, ok, err := repository.LoadResolution(ctx, closure.ResolutionKey().TenantID, closure.ResolutionID())
	if err != nil {
		t.Fatalf("按标识取回：%v", err)
	}
	if !ok {
		t.Fatal("写入后 found=false")
	}
	adopted, present := found.AdoptedFor(domain.ServiceProductObject)
	if !present {
		t.Fatal("读回的闭包丢了服务产品依据")
	}
	product, observable := adopted.ServiceProduct()
	if !observable {
		t.Fatal("产品快照没有随闭包读回——形态又不可观察了")
	}
	if product.Form() != domain.NetworkServiceForm {
		t.Fatalf("form = %q, want NETWORK_SERVICE", product.Form())
	}
}

// Covers: ADR-0044 —— 采用了结算政策的闭包必须读得回来，且方式与六维适用范围随它读回。
//
// 这一票之前是「写得进、读不回」：解析键上的结算选择器没落库，读回时最小身份立不起来，
// 整份闭包被重建门拒掉；就算过了那道门，方式也没落库，`SettlementPolicy()` 一律缺席。
// 两处都不报错——Save 答 SAVED，Load 答一句像是「快照坏了」的话。生产今天走不到这里
// （PS 的解析键登记面明拒结算政策依据），所以它一直没被撞见；但下游 SA 正是凭这份方式
// 决定冻不冻款，缺席会被读成「商业侧没登记过控制」。
func TestResolutionRoundTripsTheAdoptedSettlementPolicy(t *testing.T) {
	repository, transactor, _ := newResolutions(t)
	ctx := t.Context()
	closure := uniqueClosureWithSettlementPolicy(t, domain.TermsMethod)

	mustSaveResolution(t, transactor, ctx, repository, closure)

	found, ok, err := repository.LoadResolution(ctx, closure.ResolutionKey().TenantID, closure.ResolutionID())
	if err != nil {
		t.Fatalf("按标识取回：%v", err)
	}
	if !ok {
		t.Fatal("写入后 found=false")
	}

	selector := found.ResolutionKey().Settlement
	if selector.Empty() {
		t.Fatalf("读回的解析键丢了结算选择器：%#v", selector)
	}
	if selector.Counterparty.String() != "customer-1" ||
		selector.ChargeScope.String() != "charge-express" ||
		selector.Currency.String() != "SYN" {
		t.Fatalf("选择器读回后变了形：%#v", selector)
	}
	// 合同维在闭包键上必须缺席（ADR-0080）。落库时顺手把解出的合同写回选择器，读回的键
	// 就不再是当初提问的那个键，而重校验正是照这个键重解——它会去问一个没人问过的问题。
	if selector.Contract.String() != "" {
		t.Fatalf("闭包键上长出了合同维：%q", selector.Contract)
	}

	adopted, present := found.AdoptedFor(domain.SettlementPolicyObject)
	if !present {
		t.Fatal("读回的闭包丢了结算政策依据")
	}
	policy, observable := adopted.SettlementPolicy()
	if !observable {
		t.Fatal("政策正文没有随闭包读回——方式又不可观察了")
	}
	if policy.Method() != domain.TermsMethod {
		t.Fatalf("method = %q, want TERMS；从预付默认里长出来的账期正是 ADR-0044 要堵的",
			policy.Method())
	}
	if policy.Version().ObjectID() != adopted.Version().ObjectID() ||
		policy.Version().Version() != adopted.Version().Version() {
		t.Fatalf("政策所指版本 %s/%s 与采用版本 %s/%s 对不上",
			policy.Version().ObjectID(), policy.Version().Version(),
			adopted.Version().ObjectID(), adopted.Version().Version())
	}
	applicability := policy.Applicability()
	if applicability.LegalEntity().String() != "legal-1" ||
		applicability.Counterparty().String() != "customer-1" ||
		applicability.Contract().String() != "contract-1/v1" ||
		applicability.ChargeScope().String() != "charge-express" ||
		applicability.Currency().String() != "SYN" {
		t.Fatalf("六维适用范围读回后变了形：%#v", applicability)
	}
	if end, bounded := applicability.Effective().EndsAt(); !bounded ||
		!end.Equal(effectiveAtRow.Add(90*24*time.Hour)) {
		t.Fatalf("有效区间终止时刻读回后变了形：end=%v bounded=%v", end, bounded)
	}
}

// Covers: 不要结算依据的闭包读回后选择器必须仍然缺席（ADR-0044「含则必填、不含则必缺」）。
// 它与上一例配对：只证「写得进读得回」会让一个无条件写下空选择器的实现也通过，而那样的
// 快照读回时会因为「不该带却带了」被重建门整份拒掉。
func TestAClosureWithoutSettlementCarriesNoSelectorBack(t *testing.T) {
	repository, transactor, _ := newResolutions(t)
	ctx := t.Context()
	closure := uniqueClosure(t)

	mustSaveResolution(t, transactor, ctx, repository, closure)

	found, ok, err := repository.LoadResolution(ctx, closure.ResolutionKey().TenantID, closure.ResolutionID())
	if err != nil || !ok {
		t.Fatalf("按标识取回：found=%v err=%v", ok, err)
	}
	if !found.ResolutionKey().Settlement.Empty() {
		t.Fatal("不要结算依据的闭包读回后带上了选择器")
	}
	adopted, present := found.AdoptedFor(domain.CustomerContractObject)
	if !present {
		t.Fatal("读回的闭包丢了客户合同")
	}
	if _, observable := adopted.SettlementPolicy(); observable {
		t.Fatal("客户合同这一格读回后长出了结算政策")
	}
}

func TestResolutionReplayAndConflictSplitByContent(t *testing.T) {
	repository, transactor, pool := newResolutions(t)
	ctx := t.Context()
	original := uniqueClosure(t)
	mustSaveResolution(t, transactor, ctx, repository, original)

	var replayOutcome ports.ResolutionSaveOutcome
	mustWithinResolutionTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		replayOutcome, err = repository.Save(txCtx, original)
		return err
	})
	if replayOutcome != ports.ResolutionAlreadyRecorded {
		t.Fatalf("replay outcome = %s, want ALREADY_RECORDED", replayOutcome)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE party_commercial.commercial_resolution
		    SET content_digest = 'forged-digest'
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		original.ResolutionKey().TenantID.String(),
		original.ResolutionID().String(),
	); err != nil {
		t.Fatalf("伪造摘要：%v", err)
	}

	var conflictOutcome ports.ResolutionSaveOutcome
	mustWithinResolutionTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		conflictOutcome, err = repository.Save(txCtx, original)
		return err
	})
	if conflictOutcome != ports.ResolutionContentConflict {
		t.Fatalf("conflict outcome = %s, want CONTENT_CONFLICT", conflictOutcome)
	}
}

func TestResolutionTenantsAreInvisibleToEachOther(t *testing.T) {
	repository, transactor, _ := newResolutions(t)
	ctx := t.Context()
	closure := uniqueClosure(t)
	mustSaveResolution(t, transactor, ctx, repository, closure)

	found, ok, err := repository.LoadResolution(ctx, pcTenant(t, "tenant-b"), closure.ResolutionID())
	if err != nil {
		t.Fatalf("他租户取回：%v", err)
	}
	if ok {
		t.Fatalf("他租户读到了本租户的解析：%q", found.ResolutionID())
	}
}

func TestResolutionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newResolutions(t)
	if _, err := repository.Save(t.Context(), uniqueClosure(t)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务固定应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestResolutionRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newResolutions(t)
	ctx := t.Context()
	closure := uniqueClosure(t)
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, closure); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, ok, err := repository.LoadResolution(ctx, closure.ResolutionKey().TenantID, closure.ResolutionID())
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if ok {
		t.Fatal("回滚后解析仍在")
	}
}

func TestResolutionSnapshotNullDoesNotPassTheThreeValuedCheck(t *testing.T) {
	_, _, pool := newResolutions(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_resolution
			(tenant_id, resolution_id, customer_account_id, outcome, content_digest, snapshot)
		 VALUES ('tenant-1', 'CLO-null-1', 'customer-1', 1, 'digest-1', NULL)`); err == nil {
		t.Fatal("一行「快照列为 NULL」按 jsonb 三值缝溜进了解析库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_resolution
			(tenant_id, resolution_id, customer_account_id, outcome, content_digest, snapshot)
		 VALUES ('tenant-1', 'CLO-null-2', 'customer-1', 1, 'digest-1', 'null'::jsonb)`); err == nil {
		t.Fatal("一行「快照为 json null」按 jsonb 三值缝溜进了解析库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.commercial_resolution
			(tenant_id, resolution_id, customer_account_id, outcome, content_digest, snapshot)
		 VALUES ('tenant-1', 'CLO-not-unique', 'customer-1', 4, 'digest-1', '{}'::jsonb)`); err == nil {
		t.Fatal("一行「非唯一解析」进了只收唯一已解析的表")
	}
}

func newResolutions(t *testing.T) (*adapter.CommercialResolutions, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinResolutionTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func mustSaveResolution(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialResolutions,
	closure domain.CommercialClosure,
) {
	t.Helper()
	var savedOutcome ports.ResolutionSaveOutcome
	mustWithinResolutionTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedOutcome, err = repository.Save(txCtx, closure)
		return err
	})
	if savedOutcome != ports.ResolutionSaved {
		t.Fatalf("save outcome = %s", savedOutcome)
	}
}

func uniqueClosure(t *testing.T) domain.CommercialClosure {
	t.Helper()

	registry := domain.NewCommercialRegistry()
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")
	rules := effectiveRules(t, "rules-1", "v1", "digest-r1")
	if _, err := registry.Register(contract); err != nil {
		t.Fatalf("登记合同：%v", err)
	}
	if _, err := registry.Register(rules); err != nil {
		t.Fatalf("登记规则包：%v", err)
	}

	anchor, err := domain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour),
		pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	closure := domain.ResolveCommercialClosure(registry, domain.ClosureResolutionKey{
		TenantID:             pcTenant(t, "tenant-1"),
		CustomerAccountID:    pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                pcScope(t),
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases: []domain.CommercialObjectKind{
			domain.CustomerContractObject,
			domain.AcceptanceRulePackageObject,
		},
	}, nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	return closure
}

func uniqueClosureWithServiceProduct(t *testing.T) domain.CommercialClosure {
	t.Helper()

	registry := domain.NewCommercialRegistry()
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")
	rules := effectiveRules(t, "rules-1", "v1", "digest-r1")
	productVersion := effectiveServiceProductVersion(t, "product-1", "v1", "digest-p1")
	if _, err := registry.Register(contract); err != nil {
		t.Fatalf("登记合同：%v", err)
	}
	if _, err := registry.Register(rules); err != nil {
		t.Fatalf("登记规则包：%v", err)
	}
	if _, err := registry.Register(productVersion); err != nil {
		t.Fatalf("登记产品版本：%v", err)
	}
	product, err := domain.NewServiceProduct(productVersion, domain.NetworkServiceForm)
	if err != nil {
		t.Fatalf("构造服务产品：%v", err)
	}
	registry.RegisterServiceProduct(product)

	anchor, err := domain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour),
		pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	closure := domain.ResolveCommercialClosure(registry, domain.ClosureResolutionKey{
		TenantID:             pcTenant(t, "tenant-1"),
		CustomerAccountID:    pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                pcScope(t),
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases: []domain.CommercialObjectKind{
			domain.CustomerContractObject,
			domain.AcceptanceRulePackageObject,
			domain.ServiceProductObject,
		},
	}, nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	if adopted, ok := closure.AdoptedFor(domain.ServiceProductObject); !ok {
		t.Fatal("闭包没采用服务产品")
	} else if _, present := adopted.ServiceProduct(); !present {
		t.Fatal("登记了产品的闭包 ServiceProduct() 仍缺席")
	}
	return closure
}

// uniqueClosureWithSettlementPolicy 造一份含合同、规则包与结算政策的唯一已解析闭包——
// 正是接受控制目的下下游 SA 要凭以问「要不要控制、按哪种方式」的那个形状。政策的适用
// 范围与键上的选择器逐维对齐，否则解析选不中它。
func uniqueClosureWithSettlementPolicy(
	t *testing.T,
	method domain.SettlementMethod,
) domain.CommercialClosure {
	t.Helper()

	registry := domain.NewCommercialRegistry()
	for _, version := range []domain.CommercialVersion{
		effectiveContract(t, "contract-1", "v1", "digest-1"),
		effectiveRules(t, "rules-1", "v1", "digest-r1"),
	} {
		if _, err := registry.Register(version); err != nil {
			t.Fatalf("登记 %s：%v", version.ObjectID(), err)
		}
	}
	policyVersion := effectiveVersionOfKind(t, domain.SettlementPolicyObject, "policy-1", "v1", "digest-s1")
	if _, err := registry.Register(policyVersion); err != nil {
		t.Fatalf("登记结算政策版本：%v", err)
	}
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	applicability, err := domain.NewSettlementApplicability(
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewCounterpartyReference, "customer-1"),
		pcValue(t, domain.NewCommercialVersionLabel, "contract-1/v1"),
		pcValue(t, domain.NewChargeScopeReference, "charge-express"),
		pcValue(t, domain.NewCurrencyCode, "SYN"),
		interval,
	)
	if err != nil {
		t.Fatalf("结算适用范围：%v", err)
	}
	policy, err := domain.NewSettlementPolicy(policyVersion, method, applicability)
	if err != nil {
		t.Fatalf("构造结算政策：%v", err)
	}
	registry.RegisterSettlementPolicy(policy)

	anchor, err := domain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour),
		pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	closure := domain.ResolveCommercialClosure(registry, domain.ClosureResolutionKey{
		TenantID:             pcTenant(t, "tenant-1"),
		CustomerAccountID:    pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                pcScope(t),
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
		// 合同维不在键上：它由本闭包解出的 contract-1/v1 填（ADR-0080）。
		Settlement: domain.SettlementSelector{
			Counterparty: pcValue(t, domain.NewCounterpartyReference, "customer-1"),
			ChargeScope:  pcValue(t, domain.NewChargeScopeReference, "charge-express"),
			Currency:     pcValue(t, domain.NewCurrencyCode, "SYN"),
		},
		RequiredBases: []domain.CommercialObjectKind{
			domain.CustomerContractObject,
			domain.AcceptanceRulePackageObject,
			domain.SettlementPolicyObject,
		},
	}, nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	return closure
}

// Covers: ADR-0127 决定三——采用了信用政策的闭包必须读得回来：键上的信用选择器与采用项上的额度
// 都随快照往返，额度整份留存不回册重读；金额与比例两格各自往返，读回的那一格就是写下的那一格。
// 少了选择器，读回的键最小身份立不起来、整份闭包被重建门拒；少了额度，SA 拿到的又只是一份裸版本。
func TestResolutionRoundTripsTheAdoptedCreditBasis(t *testing.T) {
	limits := map[string]domain.CreditLimit{
		"amount": creditAmountLimit(t, 500000),
		"ratio":  creditRatioLimit(t, 2500),
	}
	for name, limit := range limits {
		t.Run(name, func(t *testing.T) {
			repository, transactor, _ := newResolutions(t)
			ctx := t.Context()
			closure := uniqueClosureWithCreditBasis(t, limit)

			mustSaveResolution(t, transactor, ctx, repository, closure)

			found, ok, err := repository.LoadResolution(ctx, closure.ResolutionKey().TenantID, closure.ResolutionID())
			if err != nil {
				t.Fatalf("按标识取回：%v", err)
			}
			if !ok {
				t.Fatal("写入后 found=false")
			}

			selector := found.ResolutionKey().Credit
			if selector.Level.String() != "level-commercial" || selector.ChargeType.String() != "charge-freight" {
				t.Fatalf("读回的解析键丢了或改了信用选择器：%#v", selector)
			}
			adopted, present := found.AdoptedFor(domain.CreditPolicyObject)
			if !present {
				t.Fatal("读回的闭包丢了信用政策依据")
			}
			basis, observable := adopted.CreditBasis()
			if !observable || !basis.Applicable() {
				t.Fatal("额度没有随闭包读回——SA 拿到的又只是一份裸版本")
			}
			if basis.AuthorizedLimit() != limit {
				t.Fatalf("limit = %#v, want %#v——读回的那一格必须就是写下的那一格", basis.AuthorizedLimit(), limit)
			}
			if !basis.PolicyVersion().SameVersionAs(adopted.Version()) {
				t.Fatal("读回的额度出处与采用版本不是同一版")
			}
			if found.ResolutionID() != closure.ResolutionID() {
				t.Fatalf("resolution ID = %q, want %q", found.ResolutionID(), closure.ResolutionID())
			}
		})
	}
}

// uniqueClosureWithCreditBasis 造一份含客户合同、接单规则包与信用政策的唯一已解析闭包，信用政策
// 正文按给定额度登记；键上的信用选择器与正文逐维对齐（ADR-0127）。
func uniqueClosureWithCreditBasis(t *testing.T, limit domain.CreditLimit) domain.CommercialClosure {
	t.Helper()

	registry := domain.NewCommercialRegistry()
	for _, version := range []domain.CommercialVersion{
		effectiveContract(t, "contract-1", "v1", "digest-1"),
		effectiveRules(t, "rules-1", "v1", "digest-r1"),
	} {
		if _, err := registry.Register(version); err != nil {
			t.Fatalf("登记 %s：%v", version.ObjectID(), err)
		}
	}
	creditVersion := effectiveVersionOfKind(t, domain.CreditPolicyObject, "credit-1", "v1", "digest-cr1")
	if _, err := registry.Register(creditVersion); err != nil {
		t.Fatalf("登记信用政策版本：%v", err)
	}
	registry.RegisterCreditPolicy(creditPolicyOn(t, creditVersion, "charge-freight", limit))

	anchor, err := domain.NewSelectionAnchor(effectiveAtRow.Add(24*time.Hour),
		pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	closure := domain.ResolveCommercialClosure(registry, domain.ClosureResolutionKey{
		TenantID:             pcTenant(t, "tenant-1"),
		CustomerAccountID:    pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                pcScope(t),
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
		Credit: domain.CreditSelector{
			Level:      pcValue(t, domain.NewAuthorityLevel, "level-commercial"),
			ChargeType: pcValue(t, domain.NewChargeTypeReference, "charge-freight"),
		},
		RequiredBases: []domain.CommercialObjectKind{
			domain.CustomerContractObject,
			domain.AcceptanceRulePackageObject,
			domain.CreditPolicyObject,
		},
	}, nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（reason=%q, unresolved=%v）",
			closure.Outcome(), closure.Reason(), closure.UnresolvedBases())
	}
	return closure
}

func effectiveServiceProductVersion(t *testing.T, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()

	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-product"),
		pcValue(t, domain.NewCommercialSourceReference, "source-product"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          domain.ServiceProductObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建服务产品版本：%v", err)
	}
	return version
}

func effectiveRules(t *testing.T, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()

	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-rules"),
		pcValue(t, domain.NewCommercialSourceReference, "source-rules"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          domain.AcceptanceRulePackageObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建规则包：%v", err)
	}
	return version
}
