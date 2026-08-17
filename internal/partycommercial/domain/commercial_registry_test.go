package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func registerable(t *testing.T, kind domain.CommercialObjectKind, objectID, version, digest string) domain.CommercialVersion {
	t.Helper()
	published, err := commercialDraft(t, kind, objectID, version, digest).
		Publish(approval(t, "approval-"+objectID+"-"+version), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish %s/%s: %v", objectID, version, err)
	}
	return published
}

// Covers: `AT-PC-002`「同一来源版本与摘要重复导入 → 返回原结果，不创建第二版本」。
func TestRegisteringTheSameContentTwiceIsAReplay(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	version := registerable(t, domain.ServiceProductObject, "product-1", "v1", "sha256:content-1")

	first, err := registry.Register(version)
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	if first != domain.RegistrationCreated {
		t.Fatalf("first outcome = %q, want CREATED", first)
	}

	replay, err := registry.Register(version)
	if err != nil {
		t.Fatalf("replay register: %v", err)
	}
	if replay != domain.RegistrationReplay {
		t.Fatalf("replay outcome = %q, want REPLAY", replay)
	}
	if got := registry.Count(); got != 1 {
		t.Fatalf("registry holds %d versions, want 1", got)
	}
}

// Covers: `AT-PC-003`「同一来源身份携带不同正文 → 形成来源冲突，不覆盖」。
func TestSameVersionWithChangedContentConflicts(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	original := registerable(t, domain.ServiceProductObject, "product-1", "v1", "sha256:content-1")
	if _, err := registry.Register(original); err != nil {
		t.Fatalf("register original: %v", err)
	}

	changed := registerable(t, domain.ServiceProductObject, "product-1", "v1", "sha256:content-CHANGED")
	outcome, err := registry.Register(changed)
	if !errors.Is(err, domain.ErrCommercialVersionConflict) {
		t.Fatalf("error = %v, want ErrCommercialVersionConflict", err)
	}
	if outcome != domain.RegistrationConflict {
		t.Fatalf("outcome = %q, want CONFLICT", outcome)
	}

	stored, found := registry.Lookup(original.Tenant(), original.Kind(), original.ObjectID(), original.Version())
	if !found || stored.ContentDigest() != original.ContentDigest() {
		t.Fatalf("the conflicting attempt overwrote the registered content: %q", stored.ContentDigest())
	}
}

// publishedNaming 构造一个在正文里指名了对外引用的已发布版本，其余各项与 registerable 相同。
func publishedNaming(
	t *testing.T,
	objectID, version, digest string,
	references map[domain.CommercialObjectKind]string,
) domain.CommercialVersion {
	t.Helper()
	spec := commercialSpec(t, domain.CustomerContractObject, objectID, version, digest)
	spec.References = make(map[domain.CommercialObjectKind]domain.CommercialObjectID, len(references))
	for kind, referencedID := range references {
		spec.References[kind] = commercialValue(t, domain.NewCommercialObjectID, referencedID)
	}

	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(
		approval(t, "approval-"+objectID+"-"+version),
		domain.ApprovalRoleConfirmed,
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		namedReferencesPublished(references),
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return published
}

// namedReferencesPublished 在发布时声明「这些指名此刻已发布」。AT-PC-022 的夹具需要它：
// 发布闸门（005）通过后，解析时登记册里可以已经没有那份规则包。
func namedReferencesPublished(references map[domain.CommercialObjectKind]string) domain.NamedReferenceStandingLookup {
	return func(kind domain.CommercialObjectKind, objectID domain.CommercialObjectID) domain.NamedReferenceStanding {
		if want, ok := references[kind]; ok && want == objectID.String() {
			return domain.NamedReferencePublished
		}
		return domain.NamedReferenceUnpublished
	}
}

// Covers: party-commercial CONTEXT 发布后正文不可覆盖 — 指名引用也属于一次发布固定下来的
// 内容。
//
// 同一版本号改挂另一个规则包时内容摘要可以一字不变：摘要覆盖的是正文，而引用随规格另给。
// 引用不参与判定，这一次改挂就会被读成重放而静默通过——它其实是一次需要商业责任方修正的
// 冲突，且下游解析会因此换掉采用对象。
func TestSameVersionReboundToAnotherReferenceConflicts(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	original := publishedNaming(t, "contract-1", "v1", "sha256:content-1",
		map[domain.CommercialObjectKind]string{domain.AcceptanceRulePackageObject: "rules-a"})
	if _, err := registry.Register(original); err != nil {
		t.Fatalf("register original: %v", err)
	}

	rebound := publishedNaming(t, "contract-1", "v1", "sha256:content-1",
		map[domain.CommercialObjectKind]string{domain.AcceptanceRulePackageObject: "rules-b"})
	outcome, err := registry.Register(rebound)
	if !errors.Is(err, domain.ErrCommercialVersionConflict) {
		t.Fatalf("error = %v, want ErrCommercialVersionConflict", err)
	}
	if outcome != domain.RegistrationConflict {
		t.Fatalf("outcome = %q, want CONFLICT", outcome)
	}

	stored, found := registry.Lookup(original.Tenant(), original.Kind(), original.ObjectID(), original.Version())
	if !found {
		t.Fatal("原登记不见了")
	}
	if named, present := stored.ReferenceTo(domain.AcceptanceRulePackageObject); !present || named.String() != "rules-a" {
		t.Fatalf("改挂尝试覆盖了原登记的指名引用：%q", named)
	}
}

// Covers: `AT-PC-004`「发布服务产品新版本 → 原版本保持不变，新版本按生效区间参与新选择」
// 的并存半边：新版本与旧版本并存，登记新版本不抹掉旧的。
func TestNewVersionCoexistsWithTheOneItReplaces(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	v1 := registerable(t, domain.ServiceProductObject, "product-1", "v1", "sha256:content-1")
	v2 := registerable(t, domain.ServiceProductObject, "product-1", "v2", "sha256:content-2")

	for _, version := range []domain.CommercialVersion{v1, v2} {
		if outcome, err := registry.Register(version); err != nil || outcome != domain.RegistrationCreated {
			t.Fatalf("register %q = %q, %v", version.Version(), outcome, err)
		}
	}

	if got := registry.Count(); got != 2 {
		t.Fatalf("registry holds %d versions, want 2", got)
	}
	stored, found := registry.Lookup(v1.Tenant(), v1.Kind(), v1.ObjectID(), v1.Version())
	if !found || stored.ContentDigest() != v1.ContentDigest() {
		t.Fatal("publishing v2 removed or rewrote v1")
	}
}

// Covers: party-commercial CONTEXT — 各对象保持独立身份。同一标识在不同对象类型下
// 是不同对象，不能互相覆盖或互相冲突。
func TestSameIdentifierUnderDifferentKindsAreDifferentObjects(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	product := registerable(t, domain.ServiceProductObject, "shared-1", "v1", "sha256:product")
	contract := registerable(t, domain.CustomerContractObject, "shared-1", "v1", "sha256:contract")

	if _, err := registry.Register(product); err != nil {
		t.Fatalf("register product: %v", err)
	}
	outcome, err := registry.Register(contract)
	if err != nil || outcome != domain.RegistrationCreated {
		t.Fatalf("a contract collided with a service product sharing an ID: %q, %v", outcome, err)
	}
	if got := registry.Count(); got != 2 {
		t.Fatalf("registry holds %d versions, want 2", got)
	}
}

// Covers: party-commercial CONTEXT 草稿可修订 — 草稿还不是受控发布，登记册只收已发布
// 及其后续状态。
func TestRegistryRefusesADraft(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	draft := commercialDraft(t, domain.ServiceProductObject, "product-1", "v1", "sha256:content-1")

	if _, err := registry.Register(draft); !errors.Is(err, domain.ErrCommercialVersionNotPublished) {
		t.Fatalf("error = %v, want ErrCommercialVersionNotPublished", err)
	}
	if got := registry.Count(); got != 0 {
		t.Fatalf("a draft entered the registry: %d versions", got)
	}
}

func registerableInTenant(
	t *testing.T,
	tenant string,
	kind domain.CommercialObjectKind,
	objectID, version, digest string,
) domain.CommercialVersion {
	t.Helper()
	spec := commercialSpec(t, kind, objectID, version, digest)
	spec.TenantID = commercialValue(t, domain.NewTenantID, tenant)
	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(
		approval(t, "approval-"+tenant+"-"+objectID+"-"+version),
		domain.ApprovalRoleConfirmed,
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		nil,
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return published
}

// Covers: `AT-PC-014`「跨租户引用参与方或规则 → 拒绝越界且不泄露另一租户内容」的**对象半边**
// （ADR-0040）：版本身份键含 TenantID。应用半边见 Validate/FormJudgment 错租户同形用例。
func TestCrossTenantSameObjectVersionNeitherReplaysNorLeaks(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	tenantA := registerableInTenant(t, "tenant-a", domain.CustomerContractObject, "contract-1", "v1", "sha256:a")
	tenantB := registerableInTenant(t, "tenant-b", domain.CustomerContractObject, "contract-1", "v1", "sha256:b-DIFFERENT")

	if outcome, err := registry.Register(tenantA); err != nil || outcome != domain.RegistrationCreated {
		t.Fatalf("register tenant-a = %q, %v", outcome, err)
	}
	viewOnlyA := registry.ViewRevision(tenantA.Tenant(), tenantA.Scope())

	outcome, err := registry.Register(tenantB)
	if err != nil {
		t.Fatalf("跨租户同号被当成冲突/错误: %v", err)
	}
	if outcome != domain.RegistrationCreated {
		t.Fatalf("跨租户同号 outcome = %q, want CREATED（不得走 Replay/Conflict）", outcome)
	}
	if got := registry.Count(); got != 2 {
		t.Fatalf("registry holds %d, want 2 tenant-isolated versions", got)
	}

	storedA, foundA := registry.Lookup(tenantA.Tenant(), tenantA.Kind(), tenantA.ObjectID(), tenantA.Version())
	if !foundA || storedA.ContentDigest().String() != "sha256:a" {
		t.Fatal("本租户正文被他租写入改写或盖掉")
	}
	storedB, foundB := registry.Lookup(tenantB.Tenant(), tenantB.Kind(), tenantB.ObjectID(), tenantB.Version())
	if !foundB || storedB.ContentDigest().String() != "sha256:b-DIFFERENT" {
		t.Fatal("他租版本没有独立落下")
	}
	if storedB.ContentDigest().String() == storedA.ContentDigest().String() {
		t.Fatal("他租 Lookup 拿到了本租户正文")
	}

	otherTenant := commercialValue(t, domain.NewTenantID, "tenant-c")
	if _, found := registry.Lookup(otherTenant, tenantA.Kind(), tenantA.ObjectID(), tenantA.Version()); found {
		t.Fatal("第三租户 Lookup 看见了别人的版本（泄露）")
	}
	if registry.ViewRevision(tenantA.Tenant(), tenantA.Scope()) != viewOnlyA {
		t.Fatal("他租写入推动了本租户 ViewRevision")
	}
	if registry.ViewRevision(tenantB.Tenant(), tenantB.Scope()) == viewOnlyA {
		t.Fatal("两租户同 scope 却共用同一 ViewRevision")
	}
}

// Covers: 同上一条的租户轴（ADR-0040 / ADR-0003），但走**价格政策**那一支。
//
// 三支分别独立过滤：版本、结算政策与服务产品都判租户，价格政策一支曾只判范围，因此同范围
// 的他租政策会被算进本租户的修订。落在这里的后果不是泄露正文，而是本租户在自己一字未动时
// 收到`已失效`——提交前失效检测据修订判断，而修订被别人推动了。
func TestAnotherTenantsPricePolicyDoesNotMoveThisTenantsViewRevision(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	tenantA := commercialValue(t, domain.NewTenantID, "tenant-a")
	shared := commercialValue(t, domain.NewCommercialScopeReference, "scope-shared")

	registry.RegisterPricePolicy(tenantPricePolicy(t, "tenant-a", "policy-a", "scope-shared", "plan-a"))
	viewOnlyA := registry.ViewRevision(tenantA, shared)

	registry.RegisterPricePolicy(tenantPricePolicy(t, "tenant-b", "policy-b", "scope-shared", "plan-b"))

	if registry.ViewRevision(tenantA, shared) != viewOnlyA {
		t.Fatal("他租价格政策推动了本租户 ViewRevision")
	}
}

// tenantPricePolicy 造一份指定租户与范围下`已生效`的价格政策。范围要能指定，因为本用例
// 的整个问题就在「两租户共用同一个范围」。
func tenantPricePolicy(t *testing.T, tenant, objectID, scope, plan string) domain.CommercialPricePolicy {
	t.Helper()

	spec := commercialSpec(t, domain.PriceRuleObject, objectID, "v1", "sha256:"+objectID)
	spec.TenantID = commercialValue(t, domain.NewTenantID, tenant)
	spec.Scope = commercialValue(t, domain.NewCommercialScopeReference, scope)

	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(
		approval(t, "approval-"+tenant+"-"+objectID),
		domain.ApprovalRoleConfirmed,
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		nil,
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}

	policy, err := domain.NewCommercialPricePolicy(
		live,
		domain.SellDirection,
		commercialValue(t, domain.NewPricingPlanReference, plan),
		domain.SellDirection,
		domain.PlanBindingConversionNone,
		commercialValue(t, domain.NewCommercialScopeReference, scope),
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new price policy: %v", err)
	}
	return policy
}
