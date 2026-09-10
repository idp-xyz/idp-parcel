package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉住 DeliveryConditionSource 两段各自封闭的全函数翻译（票 tf-segment-lifecycle-closure/14 完成判据 1）：
// PS 三格（回指 / 没有 / error）× PC 四格（引用 / 没有 / 闭包不在场 error / 未采用合同 error）里可达的组合各一例，
// 两侧集外取值各一例。conditions 串断言与 PS 回指的 String() 逐字相等——TF 侧不重拼（ADR-0080 决定七：两段式只许 PC 拼）。
//
// PC 那一侧的引用经 PC 自己的领域门造（一份唯一解析、采用了客户合同版本的闭包），不在这里拼字面：
// pcdomain.DeliveryConditionReference 只能由 DeliveryConditionReferenceFor 从闭包产出，回指也由闭包派生。

// resolutionLookupStub 扮 PS 的回指读口：记下被问的键，按预设答一格或一个错。
type resolutionLookupStub struct {
	resolution psdomain.CommercialResolutionID
	present    bool
	err        error
	tenant     string
	parcel     string
	calls      int
}

func (stub *resolutionLookupStub) LoadCommercialResolutionReference(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CommercialResolutionID, bool, error) {
	stub.calls++
	stub.tenant, stub.parcel = tenant.String(), parcel.String()
	if stub.err != nil {
		return psdomain.CommercialResolutionID{}, false, stub.err
	}
	return stub.resolution, stub.present, nil
}

// conditionLookupStub 扮 PC 的交付条件读口：记下被问的回指，按预设答一格或一个错。
type conditionLookupStub struct {
	reference  pcdomain.DeliveryConditionReference
	declared   bool
	err        error
	tenant     string
	resolution string
	calls      int
}

func (stub *conditionLookupStub) LoadDeliveryConditionReference(
	_ context.Context, tenant pcdomain.TenantID, resolution pcdomain.ResolutionID,
) (pcdomain.DeliveryConditionReference, bool, error) {
	stub.calls++
	stub.tenant, stub.resolution = tenant.String(), resolution.String()
	if stub.err != nil {
		return pcdomain.DeliveryConditionReference{}, false, stub.err
	}
	return stub.reference, stub.declared, nil
}

func mustBuild[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// resolvedClosure 经 PC 公开领域门造一份唯一解析、采用了客户合同版本的闭包：登一版生效的客户合同，按闭包解析键
// 解出来。scope 不同回指就不同，测试拿它造「PC 答的不是被问的那份闭包」。
func resolvedClosure(t *testing.T, scope string) pcdomain.CommercialClosure {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("effective interval: %v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      mustBuild(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          pcdomain.CustomerContractObject,
		ObjectID:      mustBuild(t, pcdomain.NewCommercialObjectID, "contract-1"),
		Version:       mustBuild(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         mustBuild(t, pcdomain.NewCommercialScopeReference, scope),
		ContentDigest: mustBuild(t, pcdomain.NewCommercialContentDigest, "sha256:contract-1"),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("commercial draft: %v", err)
	}
	basis, err := pcdomain.NewApprovalBasis(
		mustBuild(t, pcdomain.NewApprovalReference, "approval-contract-1"),
		mustBuild(t, pcdomain.NewCommercialSourceReference, "source-contract-1"),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("approval basis: %v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	registry := pcdomain.NewCommercialRegistry()
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	anchor, err := pcdomain.NewSelectionAnchor(
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), mustBuild(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"),
	)
	if err != nil {
		t.Fatalf("selection anchor: %v", err)
	}
	closure := pcdomain.ResolveCommercialClosure(registry, pcdomain.ClosureResolutionKey{
		TenantID:             mustBuild(t, pcdomain.NewTenantID, "tenant-1"),
		CustomerAccountID:    mustBuild(t, pcdomain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: mustBuild(t, pcdomain.NewLegalEntityReference, "legal-1"),
		Scope:                mustBuild(t, pcdomain.NewCommercialScopeReference, scope),
		Purpose:              pcdomain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases:        []pcdomain.CommercialObjectKind{pcdomain.CustomerContractObject},
	}, nil)
	if closure.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("fixture closure did not resolve uniquely: %s", closure.Outcome())
	}
	return closure
}

// conditionReferenceOf 让 PC 的领域门对该闭包答出交付条件引用（合同层有声明）。
func conditionReferenceOf(t *testing.T, closure pcdomain.CommercialClosure) pcdomain.DeliveryConditionReference {
	t.Helper()
	reference, declared, err := pcdomain.DeliveryConditionReferenceFor(closure, false, true)
	if err != nil || !declared {
		t.Fatalf("delivery condition reference: declared=%v err=%v", declared, err)
	}
	return reference
}

func psResolution(t *testing.T, closure pcdomain.CommercialClosure) psdomain.CommercialResolutionID {
	t.Helper()
	return mustBuild(t, psdomain.NewCommercialResolutionID, closure.ResolutionID().String())
}

func loadThrough(t *testing.T, resolutions *resolutionLookupStub, conditions *conditionLookupStub, object string) (string, tfports.RequirementResolution, error) {
	t.Helper()
	source, err := adapter.NewDeliveryConditionSource(resolutions, conditions)
	if err != nil {
		t.Fatalf("new delivery condition source: %v", err)
	}
	return source.LoadDeliveryConditions(t.Context(),
		mustBuild(t, tfdomain.NewTenantID, "tenant-1"), mustBuild(t, tfdomain.NewCarriedObjectReference, object))
}

var _ tfports.DeliveryConditionSource = (*adapter.DeliveryConditionSource)(nil)

// Covers: PS 回指 × PC 引用 → RequirementResolved，conditions 与 PS 回指 String() 逐字相等；两处键翻译都是同一串字面
// （租户、包裹身份进 PS；租户、回指进 PC）。
func TestAResolutionWithDeclaredConditionsIsHandedOverVerbatimAsResolved(t *testing.T) {
	closure := resolvedClosure(t, "scope-a")
	resolutions := &resolutionLookupStub{resolution: psResolution(t, closure), present: true}
	conditions := &conditionLookupStub{reference: conditionReferenceOf(t, closure), declared: true}

	value, resolution, err := loadThrough(t, resolutions, conditions, "parcel-1")
	if err != nil {
		t.Fatalf("load delivery conditions: %v", err)
	}
	if resolution != tfports.RequirementResolved || value != closure.ResolutionID().String() || value == "" {
		t.Fatalf("resolution = %q conditions = %q, want RESOLVED with the PS back-reference %q verbatim", resolution, value, closure.ResolutionID())
	}
	if resolutions.calls != 1 || resolutions.tenant != "tenant-1" || resolutions.parcel != "parcel-1" {
		t.Fatalf("PS asked %d times with tenant %q parcel %q", resolutions.calls, resolutions.tenant, resolutions.parcel)
	}
	if conditions.calls != 1 || conditions.tenant != "tenant-1" || conditions.resolution != closure.ResolutionID().String() {
		t.Fatalf("PC asked %d times with tenant %q resolution %q", conditions.calls, conditions.tenant, conditions.resolution)
	}
}

// Covers: PS 回指 × PC 没有交付条件 → RequirementMissing（第二行的「没有」：合同无条件，商业责任方去登）；照传不补默认签收。
func TestAContractWithoutDeliveryConditionsIsHandedOverAsMissing(t *testing.T) {
	closure := resolvedClosure(t, "scope-a")
	resolutions := &resolutionLookupStub{resolution: psResolution(t, closure), present: true}
	conditions := &conditionLookupStub{declared: false}

	value, resolution, err := loadThrough(t, resolutions, conditions, "parcel-1")
	if err != nil {
		t.Fatalf("load delivery conditions: %v", err)
	}
	if resolution != tfports.RequirementMissing || value != "" {
		t.Fatalf("resolution = %q conditions = %q, want MISSING with an empty reference", resolution, value)
	}
	if conditions.calls != 1 {
		t.Fatalf("PC asked %d times, want 1", conditions.calls)
	}
}

// Covers: PS 没有（对象不属任何已接受委托——集运单元、不可见对象）→ RequirementMissing（第一行的「没有」：对象无采用的
// 合同），**不进第二段**：PC 没有被问。
func TestAnObjectWithoutAnAdoptedContractIsMissingWithoutAskingPartyCommercial(t *testing.T) {
	resolutions := &resolutionLookupStub{present: false}
	conditions := &conditionLookupStub{reference: conditionReferenceOf(t, resolvedClosure(t, "scope-a")), declared: true}

	value, resolution, err := loadThrough(t, resolutions, conditions, "consolidation-unit-7")
	if err != nil {
		t.Fatalf("load delivery conditions: %v", err)
	}
	if resolution != tfports.RequirementMissing || value != "" {
		t.Fatalf("resolution = %q conditions = %q, want MISSING with an empty reference", resolution, value)
	}
	if conditions.calls != 0 {
		t.Fatalf("PC was asked %d times although PS said the object has no adopted contract", conditions.calls)
	}
}

// Covers: PS error（已接受缺回指 / 歧义 / 读面坏了）→ 原样上抛，不进第二段，不折成「没有」。
func TestAParcelShipmentFailureIsPropagatedBeforeAskingPartyCommercial(t *testing.T) {
	cases := map[string]error{
		"accepted without a resolution": psdomain.ErrAcceptedWithoutCommercialResolution,
		"ambiguous parcel":              psdomain.ErrAmbiguousParcelTarget,
		"read face down":                errors.New("parcel shipment: read face down"),
	}
	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			resolutions := &resolutionLookupStub{err: failure}
			conditions := &conditionLookupStub{}
			value, resolution, err := loadThrough(t, resolutions, conditions, "parcel-1")
			if !errors.Is(err, failure) {
				t.Fatalf("err = %v, want the PS failure wrapped", err)
			}
			if resolution != tfports.RequirementResolutionInvalid || value != "" {
				t.Fatalf("a failure still handed back resolution %q conditions %q", resolution, value)
			}
			if conditions.calls != 0 {
				t.Fatalf("PC was asked %d times after PS failed", conditions.calls)
			}
		})
	}
}

// Covers: PC 两格 error（闭包不在场 / 闭包未采用客户合同版本）与读面坏了 → 原样上抛，**不折成 Missing**：对一份已接受委托
// 的回指闭包不在场是提供方缺数据，不是「没登条件」（ADR-0133 决定二；判据同 ADR-0080 决定四 / ADR-0062 决定三）。
func TestAPartyCommercialFailureIsPropagatedNotReadAsNoConditions(t *testing.T) {
	cases := map[string]error{
		"closure absent":       pcdomain.ErrDeliveryConditionClosureAbsent,
		"contract not adopted": pcdomain.ErrDeliveryConditionContractNotAdopted,
		"read face down":       errors.New("party commercial: read face down"),
	}
	closure := resolvedClosure(t, "scope-a")
	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			resolutions := &resolutionLookupStub{resolution: psResolution(t, closure), present: true}
			conditions := &conditionLookupStub{err: failure}
			value, resolution, err := loadThrough(t, resolutions, conditions, "parcel-1")
			if !errors.Is(err, failure) {
				t.Fatalf("err = %v, want the PC failure wrapped", err)
			}
			if resolution != tfports.RequirementResolutionInvalid || value != "" {
				t.Fatalf("a failure still handed back resolution %q conditions %q", resolution, value)
			}
		})
	}
}

// Covers: 两侧集外取值 → ErrUntranslatableAnswer。PS 侧：present 却交回零值回指；PC 侧：declared 却交回零值引用、或引用
// 指着另一个回指（PC 答的不是被问的那份闭包）。都是编程错误不是业务答案，不读成「没有」。
func TestAnAnswerOutsideEitherClosedSetIsUntranslatable(t *testing.T) {
	closure := resolvedClosure(t, "scope-a")
	t.Run("PS present but zero resolution", func(t *testing.T) {
		resolutions := &resolutionLookupStub{present: true}
		conditions := &conditionLookupStub{}
		_, resolution, err := loadThrough(t, resolutions, conditions, "parcel-1")
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) || resolution != tfports.RequirementResolutionInvalid {
			t.Fatalf("err = %v resolution = %q, want ErrUntranslatableAnswer", err, resolution)
		}
		if conditions.calls != 0 {
			t.Fatal("PC was asked with an untranslatable resolution")
		}
	})
	t.Run("PC declared but zero reference", func(t *testing.T) {
		resolutions := &resolutionLookupStub{resolution: psResolution(t, closure), present: true}
		conditions := &conditionLookupStub{declared: true}
		_, resolution, err := loadThrough(t, resolutions, conditions, "parcel-1")
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) || resolution != tfports.RequirementResolutionInvalid {
			t.Fatalf("err = %v resolution = %q, want ErrUntranslatableAnswer", err, resolution)
		}
	})
	t.Run("PC answered for another resolution", func(t *testing.T) {
		other := resolvedClosure(t, "scope-b")
		if other.ResolutionID() == closure.ResolutionID() {
			t.Fatal("fixture: two scopes resolved to the same reference")
		}
		resolutions := &resolutionLookupStub{resolution: psResolution(t, closure), present: true}
		conditions := &conditionLookupStub{reference: conditionReferenceOf(t, other), declared: true}
		_, resolution, err := loadThrough(t, resolutions, conditions, "parcel-1")
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) || resolution != tfports.RequirementResolutionInvalid {
			t.Fatalf("err = %v resolution = %q, want ErrUntranslatableAnswer", err, resolution)
		}
	})
}

// Covers: 两只读口都要给，缺一只那一段永远答不出（判据同 NewCarrierIdentityDirectory）。
func TestADeliveryConditionSourceNeedsBothLookups(t *testing.T) {
	if _, err := adapter.NewDeliveryConditionSource(nil, &conditionLookupStub{}); err == nil {
		t.Fatal("a nil resolution lookup was accepted")
	}
	if _, err := adapter.NewDeliveryConditionSource(&resolutionLookupStub{}, nil); err == nil {
		t.Fatal("a nil condition lookup was accepted")
	}
}
