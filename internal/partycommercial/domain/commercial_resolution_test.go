package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var anchorAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func effectiveIn(t *testing.T, registry *domain.CommercialRegistry, kind domain.CommercialObjectKind, objectID, version, digest, scope string) domain.CommercialVersion {
	t.Helper()
	spec := commercialSpec(t, kind, objectID, version, digest)
	spec.Scope = commercialValue(t, domain.NewCommercialScopeReference, scope)

	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(approval(t, "approval-"+objectID+"-"+version), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	return live
}

// NewResolutionID 是消费侧适配器按标识回指的入口（ADR-0027）：把 PS 记录的字符串重建成
// 解析标识。空引用拒绝——空串回指没有可指的对象，静默通过只会把「没记录」伪装成一次查询。
func TestNewResolutionIDRebuildsARecordedReference(t *testing.T) {
	rebuilt, err := domain.NewResolutionID("RES-1234abcd")
	if err != nil {
		t.Fatalf("new resolution ID: %v", err)
	}
	if rebuilt.String() != "RES-1234abcd" {
		t.Fatalf("resolution ID = %q, want the recorded reference back", rebuilt.String())
	}

	for name, blank := range map[string]string{"empty": "", "spaces": "   "} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewResolutionID(blank); !errors.Is(err, domain.ErrBlankValue) {
				t.Fatalf("error = %v, want ErrBlankValue", err)
			}
		})
	}
}

func resolutionKey(t *testing.T, scope string, basis domain.CommercialObjectKind) domain.ResolutionKey {
	t.Helper()
	anchor, err := domain.NewSelectionAnchor(anchorAt, commercialValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	return domain.ResolutionKey{
		TenantID:             commercialValue(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID:    commercialValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                commercialValue(t, domain.NewCommercialScopeReference, scope),
		RequiredBasis:        basis,
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
	}
}

// Covers: AT-PC-017「锚点、产品、合同和规则包唯一适用 → 返回完整解析、版本、**区间**和
// **修订标识**」。
//
// 引的是验收项原文。原 Covers 写的是「……有效区间与选择锚点」：它一边声称覆盖有效区间，
// 一边把验收项点名的`修订标识`换成了`选择锚点`。
//
// 两项的实情不同，别把它们当成一回事（均实测于 `cab9a45`）：
//   - **有效区间当时全仓一条断言都没有。** 把解析交回的区间清成零值，改前没有任何用例会红。
//   - **修订标识在别处有专门用例**（`TestResolutionCarriesTheAuthorityViewRevision`），
//     缺的只是本用例自己不断却在 Covers 里点了它的名。补在这里是为了让这条 Covers 自洽，
//     不是因为它是个洞。
func TestUniqueCandidateResolvesWithItsAdoptedVersion(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	adopted := effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)

	if result.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", result.Outcome())
	}
	version, present := result.AdoptedVersion()
	if !present || version.ObjectID() != adopted.ObjectID() || version.Version() != adopted.Version() {
		t.Fatal("result does not carry the adopted version")
	}
	if result.ResolutionID().String() == "" {
		t.Fatal("a successful resolution carries no resolution identity")
	}
	if !result.Anchor().At().Equal(anchorAt) || result.Anchor().PolicyVersion().String() != "anchor-policy-v1" {
		t.Fatal("result did not echo the selection anchor it resolved under")
	}

	// 有效区间：消费方据它判断这份依据管到什么时候，缺了它「唯一已解析」说不出有效期。
	// 起止两端都比——只比起点时，终点被换成零值不会有任何东西变红。
	resolved, registered := version.Effective(), adopted.Effective()
	wantEnd, wantBounded := registered.EndsAt()
	gotEnd, gotBounded := resolved.EndsAt()
	if !resolved.StartsAt().Equal(registered.StartsAt()) {
		t.Fatalf("有效区间起点 = %v, want %v", resolved.StartsAt(), registered.StartsAt())
	}
	if gotBounded != wantBounded || (wantBounded && !gotEnd.Equal(wantEnd)) {
		t.Fatalf("有效区间终点 = %v/%t, want %v/%t", gotEnd, gotBounded, wantEnd, wantBounded)
	}

	// 修订标识：验收项点名要它。它证明这次解析依据的是哪一份权威视图，而步骤 8 的提交前
	// 重解全靠比对它。
	if view, hasView := result.ViewRevision(); !hasView || view.String() == "" {
		t.Fatal("a successful resolution carries no authority view revision")
	}
}

// Covers: AT-PC-019「客户回填早期业务时间试图使用已失效合同 → 不恢复旧资格，仍按独立选择
// 锚点解析」。此前全仓无任何 `Covers` 提及它（实测于 `cab9a45`）。
//
// 客户回填的时间根本没有入口：解析只读 `key.Anchor`，而锚点造不出来除非带上产生它的策略
// 版本。所以真正要守的是另一件——同一份已过期的合同在策略锚点下必须落选。
//
// 两个方向都断。只断「过期后落选」时，把 `AppliesAt` 整条删掉也不会红：那份合同会照样
// 被选中，而用例根本没问过它在自己有效期内本来选不选得中。
func TestALapsedContractIsNotRevivedByAnEarlierBusinessTime(t *testing.T) {
	lapsedFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lapsedUntil := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	registry := domain.NewCommercialRegistry()
	interval, err := domain.NewEffectiveInterval(lapsedFrom, lapsedUntil)
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	spec := commercialSpec(t, domain.CustomerContractObject, "contract-lapsed", "v1", "sha256:lapsed")
	spec.Scope = commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	spec.Effective = interval

	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(approval(t, "approval-contract-lapsed"), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}

	// 策略锚点在合同失效之后：不恢复旧资格。
	atPolicyAnchor := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
	if atPolicyAnchor.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS；一份已失效的合同被回填时间救活了", atPolicyAnchor.Outcome())
	}

	// 同一份合同、同一个登记册，锚点落在它自己的有效期内时本来是选得中的。
	withinKey := resolutionKey(t, "scope-a", domain.CustomerContractObject)
	within, err := domain.NewSelectionAnchor(
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		commercialValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	withinKey.Anchor = within
	if got := domain.ResolveCommercialBasis(registry, withinKey, nil).Outcome(); got != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED；上一断言因此是空的", got)
	}
}

// Covers: AT-PC-029「解析成功但接受前可达性失败 → PC 结果保持商业解析，接受决定由 PS
// 形成」。此前全仓无任何 `Covers` 提及它（实测于 `cab9a45`）。
//
// 本上下文要守的那一半是结构性的：商业解析结果里根本没有地方放接受判断。字段名与结果
// 取值两道一起断——只断字段名时，往 ResolutionOutcome 里加一格`已接受`照样溜过去。
func TestAResolutionResultHasNowhereToPutAnAcceptanceVerdict(t *testing.T) {
	forbidden := []string{"accept", "reject", "reachab", "verdict", "approved", "denied"}

	for _, resultType := range []reflect.Type{
		reflect.TypeOf(domain.Resolution{}),
		reflect.TypeOf(domain.CommercialClosure{}),
	} {
		for index := 0; index < resultType.NumField(); index++ {
			name := strings.ToLower(resultType.Field(index).Name)
			for _, word := range forbidden {
				if strings.Contains(name, word) {
					t.Fatalf("%s 带着字段 %s，接受判断因此在商业解析结果里有了落脚点",
						resultType.Name(), resultType.Field(index).Name)
				}
			}
		}
	}

	// 取值集合按**整份名单**断，不按关键词断。关键词判据在这里会当场误伤 `INPUT_NOT_ACCEPTED`
	// ——那说的是这次输入没被受理，不是委托被接受或拒绝，两件事只是碰巧共用一个词。
	//
	// 按名单断反而更硬：日后往这个封闭集合里加任何一格都会红，加`已接受`的人因此必须先来
	// 解释它为什么属于商业解析而不属于 PS 的接受判断。
	wantOutcomes := map[string]struct{}{
		"UNIQUELY_RESOLVED":      {},
		"NO_APPLICABLE_BASIS":    {},
		"APPLICABILITY_CONFLICT": {},
		"RESOLUTION_PENDING":     {},
		"INPUT_NOT_ACCEPTED":     {},
		"STALE":                  {},
		"BASIS_NOT_RESOLVED":     {},
	}
	for value := 0; value <= 255; value++ {
		name := domain.ResolutionOutcome(value).String()
		if name == "" {
			continue
		}
		if _, expected := wantOutcomes[name]; !expected {
			t.Fatalf("ResolutionOutcome 多出一格 %q；若它表达接受判断，那属于 PS 而不是本上下文", name)
		}
		delete(wantOutcomes, name)
	}
	for missing := range wantOutcomes {
		t.Fatalf("ResolutionOutcome 少了一格 %q，本用例的名单已经与实现脱节", missing)
	}
}

// Covers: AT-PC-020, AT-PC-021, AT-PC-027 — 零、多与读取失败是三个不同结果，任何一个
// 都不得由系统任选一条或伪装成客户不合格。
//
// Covers: `AT-PC-006`「两个合同版本在同一解析键和期间重叠 → 形成适用冲突，不按版本号任选」。
// 与 AT-PC-021 同句。权威在解析适用冲突（CONTEXT / UC-PC-001 无引文要求 Register/Publish
// 因区间重叠拒绝）；发布侧故意不因重叠拦，否则 AT-PC-007「两历史版本保留」无处安放。
func TestZeroMultipleAndUnavailableAreDistinctOutcomes(t *testing.T) {
	t.Run("zero candidates is no applicable basis", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-other")

		result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
		if result.Outcome() != domain.NoApplicableBasis {
			t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", result.Outcome())
		}
		if _, present := result.AdoptedVersion(); present {
			t.Fatal("a no-basis result named an adopted version")
		}
	})

	t.Run("two candidates conflict rather than picking the higher version", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
		effectiveIn(t, registry, domain.CustomerContractObject, "contract-2", "v9", "sha256:c2", "scope-a")
		// AT-PC-006 路径 A：重叠合同可以先入册；冲突只在解析出口成形。
		if got := registry.Count(); got != 2 {
			t.Fatalf("registry holds %d versions after overlapping register, want 2", got)
		}

		result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
		if result.Outcome() != domain.ApplicabilityConflict {
			t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", result.Outcome())
		}
		if _, present := result.AdoptedVersion(); present {
			t.Fatal("a conflict picked a winner")
		}
		if result.CandidateCount() != 2 {
			t.Fatalf("candidate count = %d, want 2", result.CandidateCount())
		}
	})

	t.Run("an unreadable authority view is pending, not no-basis", func(t *testing.T) {
		result := domain.ResolveCommercialBasis(nil, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
		if result.Outcome() != domain.ResolutionPending {
			t.Fatalf("outcome = %q, want RESOLUTION_PENDING", result.Outcome())
		}
	})
}

// Covers: AT-PC-018 — 商业选择锚点策略未配置时解析未决，绝不退回系统当前时间。
func TestMissingAnchorPolicyIsPendingRatherThanDefaultingToNow(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	if _, err := domain.NewSelectionAnchor(anchorAt, domain.AnchorPolicyVersion{}); err == nil {
		t.Fatal("an anchor without its policy version constructed")
	}

	key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
	key.Anchor = domain.SelectionAnchor{}

	result := domain.ResolveCommercialBasis(registry, key, nil)
	if result.Outcome() != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING", result.Outcome())
	}
	if _, present := result.AdoptedVersion(); present {
		t.Fatal("resolution proceeded without a selection anchor")
	}
}

// Covers: UC-PC-002 输入未受理 — 最小身份不成立时不查询、不泄露候选。
func TestIncompleteKeyIsNotAcceptedWithoutTouchingCandidates(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	incomplete := map[string]func(*domain.ResolutionKey){
		"no tenant":        func(key *domain.ResolutionKey) { key.TenantID = domain.TenantID{} },
		"no customer":      func(key *domain.ResolutionKey) { key.CustomerAccountID = domain.CustomerAccountID{} },
		"no legal entity":  func(key *domain.ResolutionKey) { key.LegalEntityCandidate = domain.LegalEntityReference{} },
		"no scope":         func(key *domain.ResolutionKey) { key.Scope = domain.CommercialScopeReference{} },
		"no required kind": func(key *domain.ResolutionKey) { key.RequiredBasis = domain.CommercialObjectKindInvalid },
	}

	for name, breakKey := range incomplete {
		t.Run(name, func(t *testing.T) {
			key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
			breakKey(&key)

			result := domain.ResolveCommercialBasis(registry, key, nil)
			if result.Outcome() != domain.InputNotAccepted {
				t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
			}
			if result.CandidateCount() != 0 {
				t.Fatal("an unaccepted input still counted candidates, which leaks their existence")
			}
		})
	}
}

// Covers: party-commercial CONTEXT 已生效 → 已到期/已退役/已替代 停止用于新的解析 —
// 收尾过的版本不再是候选，且这不同于「从来没有」。另覆盖 `AT-PC-026`「解析后合同被当前
// 修订替代」的候选集一侧。
//
// Covers: `AT-PC-008`「产品退役时已有已接受委托 → 停止新选择」在 **PC 侧**的那一半：退役后
// 不再被新解析选中。不取消/不重算既有委托属 PS（AT-PC-025），本用例不声称覆盖。
//
// `已替代`那一支此前一条断言都没有：相关用例走的全是 `Retire`（实测于 `cab9a45`）。两支
// 由不同的转移产生，共用同一句 `AppliesAt` 判据只是今天如此，不是它们必然同生共死。
func TestEndedVersionsLeaveTheCandidateSet(t *testing.T) {
	ended := map[string]func(t *testing.T, live domain.CommercialVersion) domain.CommercialVersion{
		"retired": func(t *testing.T, live domain.CommercialVersion) domain.CommercialVersion {
			t.Helper()
			retired, err := live.Retire(commercialValue(t, domain.NewRetirementReference, "retire-1"), anchorAt.AddDate(0, -1, 0))
			if err != nil {
				t.Fatalf("retire: %v", err)
			}
			return retired
		},
		"superseded": func(t *testing.T, live domain.CommercialVersion) domain.CommercialVersion {
			t.Helper()
			successor := registerable(t, domain.CustomerContractObject, "contract-1", "v2", "sha256:c1-v2")
			superseded, err := live.SupersededBy(successor, anchorAt.AddDate(0, -1, 0))
			if err != nil {
				t.Fatalf("supersede: %v", err)
			}
			return superseded
		},
	}

	for name, end := range ended {
		t.Run(name, func(t *testing.T) {
			seed := domain.NewCommercialRegistry()
			live := effectiveIn(t, seed, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

			endedOnly := domain.NewCommercialRegistry()
			if _, err := endedOnly.Register(end(t, live)); err != nil {
				t.Fatalf("register %s: %v", name, err)
			}

			result := domain.ResolveCommercialBasis(endedOnly, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
			if result.Outcome() != domain.NoApplicableBasis {
				t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", result.Outcome())
			}
			if _, present := result.AdoptedVersion(); present {
				t.Fatalf("一个%s的版本仍被选作新解析的依据", name)
			}
		})
	}

	t.Run("retired service product leaves new product selection", func(t *testing.T) {
		seed := domain.NewCommercialRegistry()
		live := effectiveIn(t, seed, domain.ServiceProductObject, "product-1", "v1", "sha256:p1", "scope-a")
		retired, err := live.Retire(commercialValue(t, domain.NewRetirementReference, "retire-product"), anchorAt.AddDate(0, -1, 0))
		if err != nil {
			t.Fatalf("retire product: %v", err)
		}
		registry := domain.NewCommercialRegistry()
		if _, err := registry.Register(retired); err != nil {
			t.Fatalf("register retired product: %v", err)
		}
		result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.ServiceProductObject), nil)
		if result.Outcome() != domain.NoApplicableBasis {
			t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS after product retirement", result.Outcome())
		}
		if retired.AppliesAt(anchorAt) {
			t.Fatal("退役产品仍 AppliesAt，新选择没有停")
		}
	})
}

// Covers: `AT-PC-007`「新合同明确替代旧合同且边界无重叠 → 两个历史版本保留，新解析按
// 边界唯一选择」。
//
// 替代把旧版收出候选集，后继在边界之后唯一适用；两边都仍在登记册里，旧正文不被抹掉。
func TestSupersedingContractKeepsHistoryAndSelectsSuccessorUniquely(t *testing.T) {
	live := effectiveVersionInScope(t, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	successor := effectiveVersionInScope(t, domain.CustomerContractObject, "contract-1", "v2", "sha256:c2", "scope-a")
	boundary := anchorAt.AddDate(0, -1, 0)
	superseded, err := live.SupersededBy(successor, boundary)
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}

	registry := domain.NewCommercialRegistry()
	if _, err := registry.Register(superseded); err != nil {
		t.Fatalf("register superseded: %v", err)
	}
	if _, err := registry.Register(successor); err != nil {
		t.Fatalf("register successor: %v", err)
	}
	if got := registry.Count(); got != 2 {
		t.Fatalf("registry holds %d versions, want 2 historical versions", got)
	}
	storedOld, found := registry.Lookup(superseded.Tenant(), superseded.Kind(), superseded.ObjectID(), superseded.Version())
	if !found || storedOld.ContentDigest() != live.ContentDigest() {
		t.Fatal("supersession rewrote or dropped the prior contract body")
	}
	named, present := storedOld.Successor()
	if !present || named != successor.Version() {
		t.Fatalf("successor = %q present=%v, want v2", named, present)
	}

	after := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
	if after.Outcome() != domain.UniquelyResolved {
		t.Fatalf("after boundary outcome = %q, want UNIQUELY_RESOLVED", after.Outcome())
	}
	adopted, ok := after.AdoptedVersion()
	if !ok || adopted.Version() != successor.Version() {
		t.Fatalf("adopted = %q, want the successor", adopted.Version())
	}
}

func effectiveVersionInScope(
	t *testing.T,
	kind domain.CommercialObjectKind,
	objectID, version, digest, scope string,
) domain.CommercialVersion {
	t.Helper()
	spec := commercialSpec(t, kind, objectID, version, digest)
	spec.Scope = commercialValue(t, domain.NewCommercialScopeReference, scope)
	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(approval(t, "approval-"+objectID+"-"+version), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

// Covers: AT-PC-024 — 相同输入与相同权威视图重复解析返回相同语义，不产生新商业版本；
// 视图修订变化后则必须重新检查，不得复用原编号。
func TestRepeatedResolutionIsStableUntilTheViewRevisionChanges(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	key := resolutionKey(t, "scope-a", domain.CustomerContractObject)

	first := domain.ResolveCommercialBasis(registry, key, nil)
	second := domain.ResolveCommercialBasis(registry, key, nil)

	if first.Outcome() != second.Outcome() || first.ResolutionID() != second.ResolutionID() {
		t.Fatalf("repeated resolution diverged under one view: %q/%q vs %q/%q",
			first.Outcome(), first.ResolutionID(), second.Outcome(), second.ResolutionID())
	}
	firstView, _ := first.ViewRevision()
	secondView, _ := second.ViewRevision()
	if firstView != secondView {
		t.Fatal("an unchanged registry reported two different view revisions")
	}
	if registry.Count() != 1 {
		t.Fatalf("resolving created commercial versions: registry holds %d", registry.Count())
	}

	// 同一对象的第二个版本改变了范围视图，所以即便查询没变，先前那个身份也不得存活下来。
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v2", "sha256:c1-v2", "scope-a")
	afterChange := domain.ResolveCommercialBasis(registry, key, nil)

	changedView, _ := afterChange.ViewRevision()
	if changedView == firstView {
		t.Fatal("the view revision ignored a new candidate in the scope")
	}
	if afterChange.ResolutionID() == first.ResolutionID() {
		t.Fatal("the resolution identity survived a changed authority view")
	}
}
