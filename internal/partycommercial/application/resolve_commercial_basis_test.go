package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var (
	anchorAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	judgedAt = time.Date(2026, 6, 2, 9, 30, 0, 0, time.UTC)
)

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// authorityDouble 记录它被问到的租户与范围，因为「不跨租户读取」这条只有在端口实际收到
// 的参数上才验得出来。
type authorityDouble struct {
	registry    *domain.CommercialRegistry
	err         error
	askedTenant domain.TenantID
	askedScope  domain.CommercialScopeReference
	loadCalled  int
}

func (double *authorityDouble) LoadScope(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	double.loadCalled++
	double.askedTenant = tenant
	double.askedScope = scope
	if double.err != nil {
		return nil, double.err
	}
	return double.registry, nil
}

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// effectiveIn 把一个版本走完发布与生效，登记进权威视图。解析只看得见已生效版本，草稿与
// 已收尾的版本都不是候选。
func effectiveIn(
	t *testing.T,
	registry *domain.CommercialRegistry,
	kind domain.CommercialObjectKind,
	objectID, version, digest, scope string,
) {
	t.Helper()

	interval, err := domain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	draft, err := domain.NewCommercialDraft(domain.CommercialVersionSpec{
		TenantID:      value(t, domain.NewTenantID, "tenant-1"),
		Kind:          kind,
		ObjectID:      value(t, domain.NewCommercialObjectID, objectID),
		Version:       value(t, domain.NewCommercialVersionLabel, version),
		Scope:         value(t, domain.NewCommercialScopeReference, scope),
		ContentDigest: value(t, domain.NewCommercialContentDigest, digest),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}

	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := domain.NewApprovalBasis(
		value(t, domain.NewApprovalReference, "approval-"+objectID),
		value(t, domain.NewCommercialSourceReference, "source-"+objectID),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("new approval basis: %v", err)
	}
	published, err := draft.Publish(basis, domain.ApprovalRoleConfirmed, approvedAt, nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
}

func closureKey(t *testing.T, scope string, bases ...domain.CommercialObjectKind) domain.ClosureResolutionKey {
	t.Helper()
	anchor, err := domain.NewSelectionAnchor(anchorAt, value(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	return domain.ClosureResolutionKey{
		TenantID:             value(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID:    value(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: value(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                value(t, domain.NewCommercialScopeReference, scope),
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases:        bases,
	}
}

// Covers: UC-PC-002 步骤 5「固定解析标识、判断时间、锚点、版本、有效区间和当前修订」——
// 六项里只有判断时间不属于领域，它必须由应用层从时钟取得。
func TestUniqueResolutionCarriesAJudgmentTimeTakenFromTheClock(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	effectiveIn(t, registry, domain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:r1", "scope-a")

	authority := &authorityDouble{registry: registry}
	handler := application.NewResolveCommercialBasisHandler(authority, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), application.ResolveCommercialBasisCommand{
		Key: closureKey(t, "scope-a", domain.CustomerContractObject, domain.AcceptanceRulePackageObject),
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Closure().Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", result.Closure().Outcome())
	}
	if !result.JudgedAt().Equal(judgedAt) {
		t.Fatalf("judged at = %s, want the clock reading %s", result.JudgedAt(), judgedAt)
	}
	if result.JudgedAt().Equal(anchorAt) {
		t.Fatal("判断时间取成了选择锚点：锚点决定选哪个版本，判断时间只说明这次判断何时作出")
	}
	if len(result.Closure().Adopted()) != 2 {
		t.Fatalf("adopted %d bases, want both required ones", len(result.Closure().Adopted()))
	}
}

// Covers: UC-PC-002「依赖调用超时与权威确认『无适用依据』是不同结果。缓存过期、读取失败
// 或修订无法确认只能形成解析未决」。合并二者会让一次读取失败被下游读成客户没有合同。
func TestUnreadableAuthorityIsPendingRatherThanNoApplicableBasis(t *testing.T) {
	authority := &authorityDouble{err: errors.New("authority view unavailable")}
	handler := application.NewResolveCommercialBasisHandler(authority, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), application.ResolveCommercialBasisCommand{
		Key: closureKey(t, "scope-a", domain.CustomerContractObject),
	})
	if err != nil {
		t.Fatalf("读取失败被当成技术错误抛出，而用例要求它形成解析未决: %v", err)
	}

	if result.Closure().Outcome() != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING", result.Closure().Outcome())
	}
	// 只断言 outcome 说不出这次未决为何停下。锚点策略未配置等的是实例参数落地，读取失败
	// 等的是重试；原因缺席，下游就只能靠猜要不要重试。
	if result.Closure().Reason() != domain.AuthorityUnreadable {
		t.Fatalf("reason = %q, want AUTHORITY_UNREADABLE", result.Closure().Reason())
	}
	if result.Closure().ContinuationReference().String() == "" {
		t.Fatal("未决无法续办，而用例要求保存缺口并安全续办")
	}
	if len(result.Closure().Adopted()) != 0 {
		t.Fatal("未决结果携带了已采用依据")
	}
	if !result.JudgedAt().Equal(judgedAt) {
		t.Fatal("未决同样要能说明这次判断是什么时候作出的")
	}
}

// Covers: ADR-0003 租户是最高数据隔离边界——权威视图必须按租户与范围双双限定，否则一个
// 客户的解析可能被另一个客户的候选回答。
func TestAuthorityViewIsAskedForTheTenantAndScopeOnTheKey(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	authority := &authorityDouble{registry: registry}
	handler := application.NewResolveCommercialBasisHandler(authority, fixedClock{at: judgedAt})
	key := closureKey(t, "scope-a", domain.CustomerContractObject)

	if _, err := handler.Handle(context.Background(), application.ResolveCommercialBasisCommand{Key: key}); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if authority.loadCalled != 1 {
		t.Fatalf("authority loaded %d times, want exactly one view per resolution", authority.loadCalled)
	}
	if authority.askedTenant != key.TenantID {
		t.Fatalf("asked tenant = %q, want %q", authority.askedTenant, key.TenantID)
	}
	if authority.askedScope != key.Scope {
		t.Fatalf("asked scope = %q, want %q", authority.askedScope, key.Scope)
	}
}

// Covers: UC-PC-002 步骤 2「不泄露其他客户/租户候选」——最小身份不成立时不得去问权威。
func TestIncompleteKeyIsRefusedWithoutReadingTheAuthority(t *testing.T) {
	authority := &authorityDouble{registry: domain.NewCommercialRegistry()}
	handler := application.NewResolveCommercialBasisHandler(authority, fixedClock{at: judgedAt})

	key := closureKey(t, "scope-a", domain.CustomerContractObject)
	key.CustomerAccountID = domain.CustomerAccountID{}

	result, err := handler.Handle(context.Background(), application.ResolveCommercialBasisCommand{Key: key})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Closure().Outcome() != domain.InputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Closure().Outcome())
	}
	if authority.loadCalled != 0 {
		t.Fatal("身份不成立却已经去读权威视图，这本身就泄露了该范围有没有对象")
	}
}
