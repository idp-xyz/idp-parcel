package partycommercial_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证解析键登记面（票 03 件 2）：登记后 FormResolutionKey
// 折出完整闭包解析键；无行是显式未配置（formed=false，不是 error）；重放与冲突分格且
// 冲突一行不动；租户与客户各自圈定；锚点与依据种类不设任何默认。

var resolutionKeyAnchorAt = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func newResolutionKeys(t *testing.T) (*adapter.CommercialResolutionKeys, bentoapp.Transactor) {
	t.Helper()
	keys, transactor, _ := newResolutionKeysOnPool(t)
	return keys, transactor
}

// newResolutionKeysOnPool 额外交出池，供直插库证「库内 CHECK 是第二道镜像」：登记面拒过的
// 组合，绕开登记面照样进不去。
func newResolutionKeysOnPool(t *testing.T) (
	*adapter.CommercialResolutionKeys,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		t.Fatalf("构造解析键持久化面：%v", err)
	}
	keys, err := adapter.NewCommercialResolutionKeys(store)
	if err != nil {
		t.Fatalf("构造解析键登记面：%v", err)
	}
	return keys, db.Transactor(), pool
}

// mustWithinKeyTransaction 只把闭包的 error 交给事务判提交还是回滚。断言一律留在闭包外：
// t.Fatal 走 runtime.Goexit，闭包永不返回，提交与回滚两条分支都会被跳过（架构门禁
// TestNoTransactionClosureCarriesAGoexitAssertion）。
func mustWithinKeyTransaction(
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

func resolutionKeyIdentity(t *testing.T, tenant, customer string) psdomain.SourceIdentity {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, tenant),
		value(t, psdomain.NewCustomerAccountID, customer),
		value(t, psdomain.NewSource, "portal"),
		value(t, psdomain.NewSourceRequestKey, "req-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity
}

func keyRegistration(t *testing.T, tenant, customer, scope string) adapter.ResolutionKeyRegistration {
	t.Helper()
	return adapter.ResolutionKeyRegistration{
		TenantID:          value(t, psdomain.NewTenantID, tenant),
		CustomerAccountID: value(t, psdomain.NewCustomerAccountID, customer),
		Scope:             value(t, pcdomain.NewCommercialScopeReference, scope),
		LegalEntity:       value(t, pcdomain.NewLegalEntityReference, "legal-1"),
		AnchorPolicy:      value(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy/v1"),
		AnchorAt:          resolutionKeyAnchorAt,
		RequiredBases: []pcdomain.CommercialObjectKind{
			pcdomain.CustomerContractObject,
			pcdomain.AcceptanceRulePackageObject,
			pcdomain.ServiceProductObject,
		},
	}
}

// settlementKeyRegistration 是同一行登记再加上结算依据与它的三维（ADR-0080）。合同维不在
// 其中：登记面结构上就没有那个字段，它由闭包解出的合同来填。
func settlementKeyRegistration(t *testing.T, tenant, customer, scope string) adapter.ResolutionKeyRegistration {
	t.Helper()
	registration := keyRegistration(t, tenant, customer, scope)
	registration.RequiredBases = append(registration.RequiredBases, pcdomain.SettlementPolicyObject)
	registration.SettlementCounterparty = value(t, pcdomain.NewCounterpartyReference, customer)
	registration.SettlementChargeScope = value(t, pcdomain.NewChargeScopeReference, "charge-express")
	registration.SettlementCurrency = value(t, pcdomain.NewCurrencyCode, "SYN")
	return registration
}

// creditKeyRegistration 是同一行登记再加上信用依据与它的两维（ADR-0127 决定二）。法人与时点
// 不在其中：键上已有法人候选与锚点，重复携带就允许两者不一致。两维的取值与
// registerCreditPolicy 登记的政策正文逐维对齐，闭包往返那条用例靠这一点命中。
func creditKeyRegistration(t *testing.T, tenant, customer, scope string) adapter.ResolutionKeyRegistration {
	t.Helper()
	registration := keyRegistration(t, tenant, customer, scope)
	registration.RequiredBases = append(registration.RequiredBases, pcdomain.CreditPolicyObject)
	registration.CreditLevel = value(t, pcdomain.NewAuthorityLevel, "level-commercial")
	registration.CreditChargeType = value(t, pcdomain.NewChargeTypeReference, "charge-freight")
	return registration
}

func basisQuery(t *testing.T, tenant, customer string) psports.CommercialBasisQuery {
	t.Helper()
	return psports.CommercialBasisQuery{
		Identity:          resolutionKeyIdentity(t, tenant, customer),
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "SHIP-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "SUB-1"),
	}
}

func mustRegisterKey(
	t *testing.T,
	transactor bentoapp.Transactor,
	keys *adapter.CommercialResolutionKeys,
	registration adapter.ResolutionKeyRegistration,
	want adapter.ResolutionKeySaveOutcome,
) {
	t.Helper()
	var outcome adapter.ResolutionKeySaveOutcome
	mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = keys.Register(txCtx, registration)
		return err
	})
	if outcome != want {
		t.Fatalf("register outcome = %s, want %s", outcome, want)
	}
}

// Covers: 票 03 件 2——登记面四项（范围/法人候选/锚点策略/必需依据种类）落库后，
// FormResolutionKey 折出的键最小身份成立、目的钉在接受控制、锚点取登记值而不是任何时钟。
func TestARegisteredKeyFormsACompleteResolutionKey(t *testing.T) {
	keys, transactor := newResolutionKeys(t)
	mustRegisterKey(t, transactor, keys, keyRegistration(t, "tenant-1", "customer-1", "scope-1"), adapter.ResolutionKeySaved)

	key, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1"))
	if err != nil {
		t.Fatalf("FormResolutionKey：%v", err)
	}
	if !formed {
		t.Fatal("登记过的键没形成")
	}
	if !key.MinimumIdentityEstablished() {
		t.Fatal("形成的键最小身份不成立")
	}
	if key.Scope.String() != "scope-1" || key.LegalEntityCandidate.String() != "legal-1" {
		t.Fatalf("范围/法人候选变形：%q %q", key.Scope, key.LegalEntityCandidate)
	}
	if key.Purpose != pcdomain.AcceptanceControlPurpose {
		t.Fatalf("purpose = %v, want 接受控制", key.Purpose)
	}
	if !key.Anchor.At().Equal(resolutionKeyAnchorAt) ||
		key.Anchor.PolicyVersion().String() != "anchor-policy/v1" {
		t.Fatalf("锚点没有取登记值：%v %q", key.Anchor.At(), key.Anchor.PolicyVersion())
	}
	if len(key.RequiredBases) != 3 {
		t.Fatalf("必需依据 = %v, want 3 项", key.RequiredBases)
	}
}

// Covers: ADR-0080 —— 登记面放行 SETTLEMENT_POLICY，携三维不携合同维；折出的键最小身份
// 成立，合同维留空等闭包解出的合同来填。
//
// 这是接受前控制链上游那一段：没有它，闭包里就没有已采用结算政策，PS 的商业依据快照缺
// `SettlementTerms`，编排一律停在 `CONTROL_SCOPE_NOT_CONFIGURED`——与「账户映射未配置」同码
// 不同因。
func TestARegisteredSettlementBasisFormsAThreeDimensionSelector(t *testing.T) {
	keys, transactor := newResolutionKeys(t)
	mustRegisterKey(t, transactor, keys,
		settlementKeyRegistration(t, "tenant-1", "customer-1", "scope-1"), adapter.ResolutionKeySaved)

	key, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1"))
	if err != nil || !formed {
		t.Fatalf("formed=%v err=%v", formed, err)
	}
	if !key.MinimumIdentityEstablished() {
		t.Fatalf("带结算依据的键最小身份不成立：%#v", key.Settlement)
	}
	if key.Settlement.Counterparty.String() != "customer-1" ||
		key.Settlement.ChargeScope.String() != "charge-express" ||
		key.Settlement.Currency.String() != "SYN" {
		t.Fatalf("结算三维读回后变了形：%#v", key.Settlement)
	}
	// 合同维必须缺席：登记进来就是消费方在指定该选中哪个商业版本，而它由本闭包解出。
	if key.Settlement.Contract.String() != "" {
		t.Fatalf("登记面填出了合同维：%q", key.Settlement.Contract)
	}
	var found bool
	for _, kind := range key.RequiredBases {
		if kind == pcdomain.SettlementPolicyObject {
			found = true
		}
	}
	if !found {
		t.Fatalf("必需依据里没有结算政策：%v", key.RequiredBases)
	}
}

// Covers: 换一维结算维度是换一套解析口径——与换范围、换锚点同级，判`内容冲突`且原登记
// 一行不动，不静默覆盖。漏比它，一次改维会被答成`已登记`而库里留着旧维。
func TestChangingASettlementDimensionIsAContentConflict(t *testing.T) {
	keys, transactor := newResolutionKeys(t)
	original := settlementKeyRegistration(t, "tenant-1", "customer-1", "scope-1")
	mustRegisterKey(t, transactor, keys, original, adapter.ResolutionKeySaved)
	mustRegisterKey(t, transactor, keys, original, adapter.ResolutionKeyAlreadyRegistered)

	changed := settlementKeyRegistration(t, "tenant-1", "customer-1", "scope-1")
	changed.SettlementChargeScope = value(t, pcdomain.NewChargeScopeReference, "charge-economy")
	mustRegisterKey(t, transactor, keys, changed, adapter.ResolutionKeyContentConflict)

	key, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1"))
	if err != nil || !formed {
		t.Fatalf("formed=%v err=%v", formed, err)
	}
	if key.Settlement.ChargeScope.String() != "charge-express" {
		t.Fatalf("冲突写入改动了原登记：%q", key.Settlement.ChargeScope)
	}
}

// Covers: 库内 CHECK 是同一判据的第二道镜像，不是唯一一道，也不是摆设。绕开登记面直插，
// 登记面拒过的每一种组合仍然进不去。
//
// 两道都要有：只靠 Go 那道，任何别的写入路径（迁移脚本、手工修数、日后另一个适配器）都能
// 造出一行永远立不起来的键；只靠库那道，判读理由到了调用方那里只剩一句约束名。
func TestTheDatabaseMirrorsTheSettlementPairingRule(t *testing.T) {
	_, _, pool := newResolutionKeysOnPool(t)
	ctx := t.Context()

	insert := func(t *testing.T, customer string, bases []string, counterparty, chargeScope, currency *string) error {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO parcel_shipment.commercial_resolution_key_registration
				(tenant_id, customer_account_id, scope_ref, legal_entity_ref,
				 anchor_policy_version, anchor_at, required_bases,
				 settlement_counterparty_ref, settlement_charge_scope_ref, settlement_currency_code)
			 VALUES ('tenant-1', $1, 'scope-1', 'legal-1', 'anchor-policy/v1', $2, $3, $4, $5, $6)`,
			customer, resolutionKeyAnchorAt, bases, counterparty, chargeScope, currency)
		return err
	}
	text := func(value string) *string { return &value }
	withSettlement := []string{"CUSTOMER_CONTRACT", "ACCEPTANCE_RULE_PACKAGE", "SETTLEMENT_POLICY"}

	t.Run("要结算却三维缺一", func(t *testing.T) {
		if err := insert(t, "c-1", withSettlement, text("customer-1"), text("charge-express"), nil); err == nil {
			t.Fatal("部分给出的结算维度直插进去了")
		}
	})
	t.Run("不要结算却带维度", func(t *testing.T) {
		bases := []string{"CUSTOMER_CONTRACT", "ACCEPTANCE_RULE_PACKAGE"}
		if err := insert(t, "c-2", bases, text("customer-1"), text("charge-express"), text("SYN")); err == nil {
			t.Fatal("不要结算依据的行带上了结算维度")
		}
	})
	t.Run("要结算却不要合同", func(t *testing.T) {
		bases := []string{"ACCEPTANCE_RULE_PACKAGE", "SETTLEMENT_POLICY"}
		if err := insert(t, "c-3", bases, text("customer-1"), text("charge-express"), text("SYN")); err == nil {
			t.Fatal("要结算却不要合同的行直插进去了——合同维永远没人填得上")
		}
	})
	t.Run("维度写成空串", func(t *testing.T) {
		if err := insert(t, "c-4", withSettlement, text("customer-1"), text("charge-express"), text("")); err == nil {
			t.Fatal("空串冒充了在场的维度")
		}
	})
	t.Run("价格规则仍在名集之外", func(t *testing.T) {
		bases := []string{"CUSTOMER_CONTRACT", "PRICE_RULE"}
		if err := insert(t, "c-5", bases, nil, nil, nil); err == nil {
			t.Fatal("PRICE_RULE 进了封闭名集——价格方向那一维本表仍不承载")
		}
	})
	t.Run("齐备则放行", func(t *testing.T) {
		if err := insert(t, "c-6", withSettlement, text("customer-1"), text("charge-express"), text("SYN")); err != nil {
			t.Fatalf("齐备的一行被挡了：%v", err)
		}
	})
}

// Covers: ADR-0127 决定二在登记面的正向——登记面放行 CREDIT_POLICY 并携两维，折出的键最小身份
// 成立、两维读回不变形、必需依据里有信用政策。这是 ADR-0127 决定四那条路的入口：租户登记的解析键
// 不要求信用政策，SA 账期分支就永远停在 CREDIT_BASIS_NOT_CONFIGURED；此前登记面放行它却不承载两维，
// 形成的键在闭包解析处答`输入未受理`——一个登记得进去、永远立不起来的键。
func TestARegisteredCreditBasisFormsATwoDimensionSelector(t *testing.T) {
	keys, transactor := newResolutionKeys(t)
	mustRegisterKey(t, transactor, keys,
		creditKeyRegistration(t, "tenant-1", "customer-1", "scope-1"), adapter.ResolutionKeySaved)

	key, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1"))
	if err != nil || !formed {
		t.Fatalf("formed=%v err=%v", formed, err)
	}
	if !key.MinimumIdentityEstablished() {
		t.Fatalf("带信用依据的键最小身份不成立：%#v", key.Credit)
	}
	if key.Credit.Level.String() != "level-commercial" || key.Credit.ChargeType.String() != "charge-freight" {
		t.Fatalf("信用两维读回后变了形：%#v", key.Credit)
	}
	if !slices.Contains(key.RequiredBases, pcdomain.CreditPolicyObject) {
		t.Fatalf("必需依据里没有信用政策：%v", key.RequiredBases)
	}
	// 结算维度不得被顺手带上：本行没要结算依据（validateSettlement 的「不含则必缺」在读回一侧同样成立）。
	if !key.Settlement.Empty() {
		t.Fatalf("不要结算依据的键读回时带上了结算维度：%#v", key.Settlement)
	}
}

// Covers: 换一维信用维度是换一套解析口径——与换结算维度、换范围、换锚点同级，判`内容冲突`且原登记
// 一行不动。漏比它，一次改维会被答成`已登记`而库里留着旧维。
func TestChangingACreditDimensionIsAContentConflict(t *testing.T) {
	keys, transactor := newResolutionKeys(t)
	original := creditKeyRegistration(t, "tenant-1", "customer-1", "scope-1")
	mustRegisterKey(t, transactor, keys, original, adapter.ResolutionKeySaved)
	mustRegisterKey(t, transactor, keys, original, adapter.ResolutionKeyAlreadyRegistered)

	changed := creditKeyRegistration(t, "tenant-1", "customer-1", "scope-1")
	changed.CreditChargeType = value(t, pcdomain.NewChargeTypeReference, "charge-surcharge")
	mustRegisterKey(t, transactor, keys, changed, adapter.ResolutionKeyContentConflict)

	key, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1"))
	if err != nil || !formed {
		t.Fatalf("formed=%v err=%v", formed, err)
	}
	if key.Credit.ChargeType.String() != "charge-freight" {
		t.Fatalf("冲突写入改动了原登记：%q", key.Credit.ChargeType)
	}
}

// Covers: 库内 `..._credit_paired` / `..._credit_not_blank` 是登记面 validateCredit 同一判据的第二道
// 镜像。绕开登记面直插，登记面拒过的每一种组合仍然进不去，且拒它的必须是这两条约束之一——
// 换成任何别的错（比如列不存在）都算没守住：那种「拒」在迁移落地前也成立，证不了镜像在。
func TestTheDatabaseMirrorsTheCreditPairingRule(t *testing.T) {
	_, _, pool := newResolutionKeysOnPool(t)
	ctx := t.Context()

	insert := func(t *testing.T, customer string, bases []string, level, chargeType *string) error {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO parcel_shipment.commercial_resolution_key_registration
				(tenant_id, customer_account_id, scope_ref, legal_entity_ref,
				 anchor_policy_version, anchor_at, required_bases,
				 credit_level, credit_charge_type)
			 VALUES ('tenant-1', $1, 'scope-1', 'legal-1', 'anchor-policy/v1', $2, $3, $4, $5)`,
			customer, resolutionKeyAnchorAt, bases, level, chargeType)
		return err
	}
	refusedBy := func(t *testing.T, err error, complaint, constraint string) {
		t.Helper()
		if err == nil {
			t.Fatal(complaint)
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != constraint {
			t.Fatalf("被拒了，但不是 %s 拒的：%v", constraint, err)
		}
	}
	text := func(value string) *string { return &value }
	withCredit := []string{"CUSTOMER_CONTRACT", "ACCEPTANCE_RULE_PACKAGE", "CREDIT_POLICY"}
	withoutCredit := []string{"CUSTOMER_CONTRACT", "ACCEPTANCE_RULE_PACKAGE"}
	const paired = "commercial_resolution_key_registration_credit_paired"
	const notBlank = "commercial_resolution_key_registration_credit_not_blank"

	t.Run("要信用却两维缺一", func(t *testing.T) {
		refusedBy(t, insert(t, "c-1", withCredit, text("level-commercial"), nil),
			"部分给出的信用维度直插进去了", paired)
	})
	t.Run("要信用却两维全缺", func(t *testing.T) {
		refusedBy(t, insert(t, "c-2", withCredit, nil, nil),
			"要信用依据却不带两维的行直插进去了——那个键永远立不起来", paired)
	})
	t.Run("不要信用却带维度", func(t *testing.T) {
		refusedBy(t, insert(t, "c-3", withoutCredit, text("level-commercial"), text("charge-freight")),
			"不要信用依据的行带上了信用维度", paired)
	})
	t.Run("维度写成空串", func(t *testing.T) {
		refusedBy(t, insert(t, "c-4", withCredit, text("level-commercial"), text("")),
			"空串冒充了在场的维度", notBlank)
	})
	t.Run("齐备则放行", func(t *testing.T) {
		if err := insert(t, "c-5", withCredit, text("level-commercial"), text("charge-freight")); err != nil {
			t.Fatalf("齐备的一行被挡了：%v", err)
		}
	})
}

// registerCreditPolicy 把一份信用政策正文放进登记册：与 creditKeyRegistration 的两维逐维对齐，
// 法人对齐键上的法人候选，有效区间盖住登记的锚点。夹具形状照抄 party-commercial 自己的领域测试
// （那份在 _test.go 里，此处不可 import，只能重建）。
func registerCreditPolicy(t *testing.T, registry *pcdomain.CommercialRegistry, scope string, minor int64) {
	t.Helper()
	version := effectiveIn(t, registry, pcdomain.CreditPolicyObject, "credit-1", "v1", "sha256:credit-1", scope)
	limit, err := pcdomain.NewCreditAmountLimit(minor)
	if err != nil {
		t.Fatalf("new credit amount limit: %v", err)
	}
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	policy, err := pcdomain.NewCreditPolicy(
		version,
		value(t, pcdomain.NewLegalEntityReference, "legal-1"),
		value(t, pcdomain.NewAuthorityLevel, "level-commercial"),
		value(t, pcdomain.NewChargeTypeReference, "charge-freight"),
		limit,
		interval,
	)
	if err != nil {
		t.Fatalf("new credit policy: %v", err)
	}
	registry.RegisterCreditPolicy(policy)
}

// Covers: 闭包键往返——登记面形成的键送 party-commercial 的真实闭包解析，请求信用依据时不再答
// `输入未受理`，而是唯一解出并交回出自哪一版政策、授权多少额度（ADR-0127 决定一与三）。
//
// 这一条证的是两个上下文对「信用二维」的读法一致：登记面写进去的等级与费用类型，正是闭包解析拿去
// 命中政策正文的那两格。只在本包断言键的形状，证不了这一点——形状对了而 PC 换了判据，键照样立不起来。
func TestACreditKeyResolvesAgainstTheClosure(t *testing.T) {
	keys, transactor := newResolutionKeys(t)
	mustRegisterKey(t, transactor, keys,
		creditKeyRegistration(t, "tenant-1", "customer-1", "scope-1"), adapter.ResolutionKeySaved)
	key, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1"))
	if err != nil || !formed {
		t.Fatalf("formed=%v err=%v", formed, err)
	}

	registry := pcdomain.NewCommercialRegistry()
	effectiveIn(t, registry, pcdomain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-1")
	effectiveIn(t, registry, pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:r1", "scope-1")
	effectiveIn(t, registry, pcdomain.ServiceProductObject, "product-1", "v1", "sha256:p1", "scope-1")
	registerCreditPolicy(t, registry, "scope-1", 500000)

	closure := pcdomain.ResolveCommercialClosure(registry, key, nil)
	if closure.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("outcome = %q（reason=%q unresolved=%v）, want UNIQUELY_RESOLVED——登记面形成的键在闭包解析处立不起来",
			closure.Outcome(), closure.Reason(), closure.UnresolvedBases())
	}
	adopted, present := closure.AdoptedFor(pcdomain.CreditPolicyObject)
	if !present {
		t.Fatal("闭包没有采用信用依据")
	}
	credit, ok := adopted.CreditBasis()
	if !ok || !credit.Applicable() {
		t.Fatal("闭包采用了信用依据却没有额度——键上两维没有命中政策正文")
	}
	if minor, ok := credit.AuthorizedLimit().AmountMinor(); !ok || minor != 500000 {
		t.Fatalf("limit = (%d, %v), want 500000", minor, ok)
	}
}

// Covers: 「今天 nil 即显式未配置」的登记面版本——无行交回 formed=false 且无错误，
// 与读取失败分格；他租户与他客户的登记互不可见。
func TestAnUnregisteredCustomerFormsNothing(t *testing.T) {
	keys, transactor := newResolutionKeys(t)

	if _, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1")); err != nil || formed {
		t.Fatalf("formed=%v err=%v；未登记应是显式未配置，不是错误", formed, err)
	}

	mustRegisterKey(t, transactor, keys, keyRegistration(t, "tenant-1", "customer-1", "scope-1"), adapter.ResolutionKeySaved)

	t.Run("他租户", func(t *testing.T) {
		if _, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-b", "customer-1")); err != nil || formed {
			t.Fatalf("formed=%v err=%v；他租户看见了本租户的登记", formed, err)
		}
	})
	t.Run("他客户", func(t *testing.T) {
		if _, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-b")); err != nil || formed {
			t.Fatalf("formed=%v err=%v；他客户看见了本客户的登记", formed, err)
		}
	})
}

// Covers: 登记的落点代数——同（租户+客户）同参数是重放，异参数是冲突且原登记一行不动：
// 换范围或换锚点是换一套解析口径，要走显式新决定，不静默覆盖。
func TestKeyRegistrationReplayAndConflictSplitByContent(t *testing.T) {
	keys, transactor := newResolutionKeys(t)
	original := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
	mustRegisterKey(t, transactor, keys, original, adapter.ResolutionKeySaved)
	mustRegisterKey(t, transactor, keys, original, adapter.ResolutionKeyAlreadyRegistered)

	changed := keyRegistration(t, "tenant-1", "customer-1", "scope-ANOTHER")
	mustRegisterKey(t, transactor, keys, changed, adapter.ResolutionKeyContentConflict)

	key, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1"))
	if err != nil || !formed {
		t.Fatalf("formed=%v err=%v", formed, err)
	}
	if key.Scope.String() != "scope-1" {
		t.Fatalf("冲突写入改动了原登记：%q", key.Scope)
	}
}

// Covers: 登记不设默认——锚点零值、空依据集合、集合外与需要额外选择维度的种类都在
// 触库前被拒；写入拒绝在事务之外运行。
func TestKeyRegistrationRefusesDefaultsAndBareCalls(t *testing.T) {
	keys, transactor := newResolutionKeys(t)

	t.Run("锚点零值", func(t *testing.T) {
		broken := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.AnchorAt = time.Time{}
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("零值锚点被登记了——那就是等人来补默认")
		}
	})

	t.Run("空依据集合", func(t *testing.T) {
		broken := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.RequiredBases = nil
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("零必需依据的键被登记了")
		}
	})

	t.Run("结算政策要三维", func(t *testing.T) {
		broken := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.RequiredBases = append(broken.RequiredBases, pcdomain.SettlementPolicyObject)
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("结算政策进了不带三维的登记——那个键永远立不起来")
		}
	})

	t.Run("三维缺一", func(t *testing.T) {
		broken := settlementKeyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.SettlementCurrency = pcdomain.CurrencyCode{}
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("部分给出的结算维度被登记了")
		}
	})

	t.Run("不要结算却带维度", func(t *testing.T) {
		broken := settlementKeyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.RequiredBases = keyRegistration(t, "tenant-1", "customer-1", "scope-1").RequiredBases
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("不要结算依据的登记带上了结算维度——ADR-0044 的「不含则必缺」破了")
		}
	})

	t.Run("要结算却不要合同", func(t *testing.T) {
		broken := settlementKeyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.RequiredBases = []pcdomain.CommercialObjectKind{
			pcdomain.AcceptanceRulePackageObject,
			pcdomain.SettlementPolicyObject,
		}
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("要结算却不要合同的登记过了——合同维永远没人填得上（ADR-0080）")
		}
	})

	t.Run("信用政策要两维", func(t *testing.T) {
		broken := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.RequiredBases = append(broken.RequiredBases, pcdomain.CreditPolicyObject)
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("信用政策进了不带两维的登记——那个键在闭包解析处一律答`输入未受理`（ADR-0127 决定二）")
		}
	})

	t.Run("两维缺一", func(t *testing.T) {
		broken := creditKeyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.CreditChargeType = pcdomain.ChargeTypeReference{}
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("部分给出的信用维度被登记了")
		}
	})

	t.Run("不要信用却带维度", func(t *testing.T) {
		broken := creditKeyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.RequiredBases = keyRegistration(t, "tenant-1", "customer-1", "scope-1").RequiredBases
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("不要信用依据的登记带上了信用维度——「不含则必缺」破了")
		}
	})

	t.Run("价格规则仍不承载", func(t *testing.T) {
		broken := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.RequiredBases = append(broken.RequiredBases, pcdomain.PriceRuleObject)
		var registerErr error
		mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			_, registerErr = keys.Register(txCtx, broken)
			return nil
		})
		if registerErr == nil {
			t.Fatal("价格规则进了登记面——价格方向那一维本面仍不承载")
		}
	})

	t.Run("无事务拒", func(t *testing.T) {
		if _, err := keys.Register(t.Context(), keyRegistration(t, "tenant-1", "customer-1", "scope-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
			t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
		}
	})
}
