package domain_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var (
	amendmentEffectiveAt = time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	amendmentFormedAt    = time.Date(2026, 8, 9, 9, 0, 5, 0, time.UTC)
)

func consigneeScope(t *testing.T) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewParcelScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		mustValue(t, domain.NewSourceDataGroupReference, "CONSIGNEE_ADDRESS"),
	)
	if err != nil {
		t.Fatalf("new parcel scoped source data: %v", err)
	}
	return scope
}

func amendmentSpec(t *testing.T) domain.CustomerSourceDataVersionSpec {
	t.Helper()
	return domain.CustomerSourceDataVersionSpec{
		VersionID:   mustValue(t, domain.NewSourceDataVersionID, "data-version-1"),
		Scope:       consigneeScope(t),
		Basis:       domain.NewSupplementOnAcceptanceBaseline(),
		Request:     sourceFingerprint(t, "tenant-1", "customer-1", "portal", "amend-1", "digest-1"),
		Reason:      mustValue(t, domain.NewAmendmentReasonReference, "CONSIGNEE_ADDRESS_CORRECTION"),
		Requester:   mustValue(t, domain.NewRequesterReference, "CUSTOMER-CONTACT-1"),
		Decider:     mustValue(t, domain.NewDeciderReference, "OPERATOR-1"),
		Authority:   mustValue(t, domain.NewAmendmentAuthoritySnapshot, "PC-AMEND-GRANT-1"),
		EffectiveAt: amendmentEffectiveAt,
		FormedAt:    amendmentFormedAt,
	}
}

// Covers: CONTEXT「接受后资料版本必须明确关联委托、包裹、字段或资料范围、基础版本、原因、
// 请求方、实际决定方、授权快照和业务/适用时间」— 留痕读得回，且请求方与实际决定方分开记，
// 「登录操作人不能替代实际决定方」靠的就是这两格分立。
func TestACustomerSourceDataVersionKeepsItsFullAuditTrail(t *testing.T) {
	version, err := domain.FormCustomerSourceDataVersion(amendmentSpec(t))
	if err != nil {
		t.Fatalf("form customer source data version: %v", err)
	}

	if version.VersionID().String() != "data-version-1" {
		t.Fatalf("version ID = %q, want data-version-1", version.VersionID())
	}
	if version.Scope() != consigneeScope(t) {
		t.Fatalf("scope = %#v, want the consignee address scope", version.Scope())
	}
	if version.Reason().String() == "" || version.Authority().String() == "" {
		t.Fatalf("version lost its reason or authority snapshot: %#v", version)
	}
	if version.Requester().String() == version.Decider().String() {
		t.Fatal("requester and decider collapsed into one reference")
	}
	if !version.EffectiveAt().Equal(amendmentEffectiveAt) {
		t.Fatalf("effective at = %v, want %v", version.EffectiveAt(), amendmentEffectiveAt)
	}
	if !version.FormedAt().Equal(amendmentFormedAt) {
		t.Fatalf("formed at = %v, want %v", version.FormedAt(), amendmentFormedAt)
	}
}

// Covers: UC-PS-002「不得创建占位版本」与 CONTEXT 的同一条留痕清单 — 缺任一项都不成版本。
// 少一项就形成一个半截版本，而版本不可覆盖，事后补不回来。
func TestACustomerSourceDataVersionWithoutItsAuditTrailCannotBeFormed(t *testing.T) {
	missing := map[string]func(domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec{
		"version ID": func(s domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec {
			s.VersionID = domain.SourceDataVersionID{}
			return s
		},
		"scope": func(s domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec {
			s.Scope = domain.SourceDataScope{}
			return s
		},
		"basis": func(s domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec {
			s.Basis = domain.SourceDataBasis{}
			return s
		},
		"request": func(s domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec {
			s.Request = domain.SourceSubmissionFingerprint{}
			return s
		},
		"reason": func(s domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec {
			s.Reason = domain.AmendmentReasonReference{}
			return s
		},
		"requester": func(s domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec {
			s.Requester = domain.RequesterReference{}
			return s
		},
		"decider": func(s domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec {
			s.Decider = domain.DeciderReference{}
			return s
		},
		"authority snapshot": func(s domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec {
			s.Authority = domain.AmendmentAuthoritySnapshot{}
			return s
		},
		"formed at": func(s domain.CustomerSourceDataVersionSpec) domain.CustomerSourceDataVersionSpec {
			s.FormedAt = time.Time{}
			return s
		},
	}

	for name, drop := range missing {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.FormCustomerSourceDataVersion(drop(amendmentSpec(t))); !errors.Is(
				err, domain.ErrInvalidCustomerSourceDataVersion,
			) {
				t.Fatalf("error = %v, want ErrInvalidCustomerSourceDataVersion", err)
			}
		})
	}
}

// Covers: CONTEXT「接受决定前，普通资料纠错或补充在同一委托下形成新的提交版本并重新判断」—
// 资料版本这条路只对`已接受`委托开放。决定前拿它改资料，就绕过了重新判断；已拒绝或已撤回后
// 的新需求按 CONTEXT 形成关联新委托，同样不是这条路。
func TestCustomerSourceDataCannotBeAmendedBeforeTheAcceptanceDecision(t *testing.T) {
	version, err := domain.FormCustomerSourceDataVersion(amendmentSpec(t))
	if err != nil {
		t.Fatalf("form customer source data version: %v", err)
	}

	if _, err := submitted(t).AmendCustomerSourceData(version); !errors.Is(
		err, domain.ErrShipmentRequestNotAccepted,
	) {
		t.Fatalf("error = %v, want ErrShipmentRequestNotAccepted", err)
	}
}

// Covers: CONTEXT「版本追加保存，不覆盖接受基线或旧版本」与 AT-PS-015「保留旧版本和更正关系」
// — 第二个版本不顶掉第一个，接受基线也不因资料更正而改变。基线一旦可被资料版本改写，接受时
// 固定的服务范围就成了活的，下游按它办的事全部失去依据。
func TestCustomerSourceDataVersionsAppendWithoutOverwritingBaselineOrPriorVersions(t *testing.T) {
	accepted, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	baselineBefore, present := accepted.AcceptanceBaseline()
	if !present {
		t.Fatal("acceptance fixed no baseline")
	}

	first, err := domain.FormCustomerSourceDataVersion(amendmentSpec(t))
	if err != nil {
		t.Fatalf("form first version: %v", err)
	}
	amended, err := accepted.AmendCustomerSourceData(first)
	if err != nil {
		t.Fatalf("amend with first version: %v", err)
	}

	secondSpec := amendmentSpec(t)
	secondSpec.VersionID = mustValue(t, domain.NewSourceDataVersionID, "data-version-2")
	if secondSpec.Basis, err = domain.NewAmendmentOfVersion(first.VersionID()); err != nil {
		t.Fatalf("new amendment basis: %v", err)
	}
	second, err := domain.FormCustomerSourceDataVersion(secondSpec)
	if err != nil {
		t.Fatalf("form second version: %v", err)
	}
	amended, err = amended.AmendCustomerSourceData(second)
	if err != nil {
		t.Fatalf("amend with second version: %v", err)
	}

	versions := amended.CustomerSourceDataVersions()
	if len(versions) != 2 {
		t.Fatalf("versions kept = %d, want 2", len(versions))
	}
	if versions[0].VersionID() != first.VersionID() || versions[1].VersionID() != second.VersionID() {
		t.Fatalf(
			"versions lost their order or identity: %q then %q",
			versions[0].VersionID(), versions[1].VersionID(),
		)
	}

	baselineAfter, _ := amended.AcceptanceBaseline()
	if baselineAfter.SubmissionVersionID() != baselineBefore.SubmissionVersionID() ||
		!baselineAfter.FixedAt().Equal(baselineBefore.FixedAt()) ||
		!reflect.DeepEqual(
			stringValues(baselineAfter.DeclaredParcelIDs()),
			stringValues(baselineBefore.DeclaredParcelIDs()),
		) {
		t.Fatal("a customer source data version overwrote the acceptance baseline")
	}
	if amended.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", amended.State())
	}
}

// Covers: CONTEXT「接受后不得以资料更正方式新增或删除委托成员」与 `AT-PS-023`「请求新增或删除
// 已接受委托成员 → 拒绝普通资料更正，转取消、重组或关联新委托路径」— 接受基线自己就是成员集合
// 的权威，指向基线外包裹的资料版本不需要任何已登记目录就能拒。放过去等于让客户拿一份「更正」
// 往已接受委托里塞成员，而基线固定的正是那份成员集合。
func TestACustomerSourceDataVersionCannotReachAParcelOutsideTheAcceptanceBaseline(t *testing.T) {
	accepted, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	spec := amendmentSpec(t)
	if spec.Scope, err = domain.NewParcelScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-never-declared"),
		mustValue(t, domain.NewSourceDataGroupReference, "GOODS_DESCRIPTION"),
	); err != nil {
		t.Fatalf("new parcel scoped source data: %v", err)
	}
	intruder, err := domain.FormCustomerSourceDataVersion(spec)
	if err != nil {
		t.Fatalf("form customer source data version: %v", err)
	}

	if _, err := accepted.AmendCustomerSourceData(intruder); !errors.Is(
		err, domain.ErrParcelOutsideAcceptanceBaseline,
	) {
		t.Fatalf("error = %v, want ErrParcelOutsideAcceptanceBaseline", err)
	}
	if len(accepted.CustomerSourceDataVersions()) != 0 {
		t.Fatal("a version naming an undeclared parcel was kept anyway")
	}
}

// Covers: 同一条规则的另一侧 — 委托级资料范围不指名成员，因此不受成员基线约束。寄件人一类
// 资料作用于整份委托，若也拿基线成员去卡，一份合法的委托级更正会因为它没指名包裹而被拒。
func TestAShipmentScopedSourceDataVersionNeedsNoBaselineMember(t *testing.T) {
	accepted, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	spec := amendmentSpec(t)
	if spec.Scope, err = domain.NewShipmentScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		mustValue(t, domain.NewSourceDataGroupReference, "CONSIGNOR_CONTACT"),
	); err != nil {
		t.Fatalf("new shipment scoped source data: %v", err)
	}
	version, err := domain.FormCustomerSourceDataVersion(spec)
	if err != nil {
		t.Fatalf("form customer source data version: %v", err)
	}

	amended, err := accepted.AmendCustomerSourceData(version)
	if err != nil {
		t.Fatalf("amend with a shipment scoped version: %v", err)
	}
	if len(amended.CustomerSourceDataVersions()) != 1 {
		t.Fatalf("versions kept = %d, want the shipment scoped version", len(amended.CustomerSourceDataVersions()))
	}
}

// acceptedWithVersions 按给定的 (版本号, 基准) 依次追加资料版本。基准用版本号表达，空串表示
// 以接受基线为准。
func acceptedWithVersions(t *testing.T, chain ...[2]string) domain.ShipmentRequest {
	t.Helper()
	request, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	for _, link := range chain {
		spec := amendmentSpec(t)
		spec.VersionID = mustValue(t, domain.NewSourceDataVersionID, link[0])
		if link[1] == "" {
			spec.Basis = domain.NewSupplementOnAcceptanceBaseline()
		} else if spec.Basis, err = domain.NewAmendmentOfVersion(
			mustValue(t, domain.NewSourceDataVersionID, link[1]),
		); err != nil {
			t.Fatalf("new amendment basis: %v", err)
		}

		version, err := domain.FormCustomerSourceDataVersion(spec)
		if err != nil {
			t.Fatalf("form version %q: %v", link[0], err)
		}
		if request, err = request.AmendCustomerSourceData(version); err != nil {
			t.Fatalf("amend with version %q: %v", link[0], err)
		}
	}
	return request
}

// Covers: CONTEXT「版本已形成 → 当前资料版本采用判断：依据版本关系、来源更正/撤销、业务有效
// 时间和并发顺序派生」— 一条干净的基准链上，当前采用判断指向链尾。
func TestTheCurrentSourceDataAdoptionFollowsTheBasisChain(t *testing.T) {
	t.Run("no version in scope yields no adoption", func(t *testing.T) {
		accepted, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		if _, present := accepted.CurrentSourceDataAdoption(consigneeScope(t)); present {
			t.Fatal("a scope with no version reported an adoption judgment")
		}
	})

	t.Run("a chain adopts its last link", func(t *testing.T) {
		request := acceptedWithVersions(t,
			[2]string{"data-version-1", ""},
			[2]string{"data-version-2", "data-version-1"},
		)

		judgment, present := request.CurrentSourceDataAdoption(consigneeScope(t))
		if !present {
			t.Fatal("two versions formed no adoption judgment")
		}
		if judgment.Outcome() != domain.SourceDataAdopted {
			t.Fatalf("outcome = %q, want ADOPTED", judgment.Outcome())
		}
		adopted, hasAdopted := judgment.AdoptedVersion()
		if !hasAdopted || adopted.String() != "data-version-2" {
			t.Fatalf("adopted version = %q, want data-version-2", adopted)
		}
	})
}

// Covers: UC-PS-002「请求引用过期基础版本且与当前版本存在重叠字段时，形成冲突或待复核；只有
// 登记了明确合并规则的非重叠范围才可合并，首发默认保持待复核」与 CONTEXT「过期基础版本与当前
// 版本重叠时不得按最后到达覆盖」— 两位客户代表各自基于同一版改同一块资料，后到的那份仍然
// 追加保存，但它不许顶掉当前采用的那一版。
func TestAStaleBasisHoldsTheAdoptionForReviewInsteadOfOverwriting(t *testing.T) {
	request := acceptedWithVersions(t,
		[2]string{"data-version-1", ""},
		[2]string{"data-version-2", "data-version-1"},
		[2]string{"data-version-3", "data-version-1"},
	)

	if len(request.CustomerSourceDataVersions()) != 3 {
		t.Fatalf("versions kept = %d, want all 3 preserved", len(request.CustomerSourceDataVersions()))
	}

	judgment, present := request.CurrentSourceDataAdoption(consigneeScope(t))
	if !present {
		t.Fatal("three versions formed no adoption judgment")
	}
	if judgment.Outcome() != domain.SourceDataAwaitingReview {
		t.Fatalf("outcome = %q, want AWAITING_REVIEW", judgment.Outcome())
	}
	adopted, hasAdopted := judgment.AdoptedVersion()
	if !hasAdopted || adopted.String() != "data-version-2" {
		t.Fatalf("adopted version = %q, want data-version-2 to survive the later arrival", adopted)
	}
}

// Covers: UC-PS-002「只有登记了明确合并规则的非重叠范围才可合并，首发默认保持待复核」— 待复核
// 一旦成立就不会被后续版本顺手抹掉。v3 那个分叉没有人并过，而 v4 只是接着 v2 往下改；让 v4
// 把判断推回`已采用`，等于用一次无关的修订宣布分叉已解决，v3 就此静默出局。
func TestAnUnmergedForkKeepsTheAdoptionUnderReview(t *testing.T) {
	request := acceptedWithVersions(t,
		[2]string{"data-version-1", ""},
		[2]string{"data-version-2", "data-version-1"},
		[2]string{"data-version-3", "data-version-1"},
		[2]string{"data-version-4", "data-version-2"},
	)

	judgment, present := request.CurrentSourceDataAdoption(consigneeScope(t))
	if !present {
		t.Fatal("four versions formed no adoption judgment")
	}
	if judgment.Outcome() != domain.SourceDataAwaitingReview {
		t.Fatalf("outcome = %q, want the unmerged fork to keep it AWAITING_REVIEW", judgment.Outcome())
	}
}

// Covers: CONTEXT「当前客户资料版本采用判断……依据……对象范围……派生」与 UC-PS-002「批量操作
// 不得形成『全部已采用』或『全部已拒绝』的总结果」— 一个范围上的待复核不拖累另一个范围。
func TestAnAdoptionHeldForReviewDoesNotSpreadToAnotherScope(t *testing.T) {
	request := acceptedWithVersions(t,
		[2]string{"data-version-1", ""},
		[2]string{"data-version-2", "data-version-1"},
		[2]string{"data-version-3", "data-version-1"},
	)

	goodsScope, err := domain.NewParcelScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		mustValue(t, domain.NewSourceDataGroupReference, "GOODS_DESCRIPTION"),
	)
	if err != nil {
		t.Fatalf("new parcel scoped source data: %v", err)
	}
	spec := amendmentSpec(t)
	spec.VersionID = mustValue(t, domain.NewSourceDataVersionID, "data-version-4")
	spec.Scope = goodsScope
	goodsVersion, err := domain.FormCustomerSourceDataVersion(spec)
	if err != nil {
		t.Fatalf("form goods version: %v", err)
	}
	if request, err = request.AmendCustomerSourceData(goodsVersion); err != nil {
		t.Fatalf("amend with goods version: %v", err)
	}

	judgment, present := request.CurrentSourceDataAdoption(goodsScope)
	if !present || judgment.Outcome() != domain.SourceDataAdopted {
		t.Fatalf("goods scope outcome = %q present = %v, want ADOPTED", judgment.Outcome(), present)
	}
	if held, _ := request.CurrentSourceDataAdoption(consigneeScope(t)); held.Outcome() != domain.SourceDataAwaitingReview {
		t.Fatalf("consignee scope outcome = %q, want it still AWAITING_REVIEW", held.Outcome())
	}
}

// Covers: UC-PS-002「每次版本必须关联集团租户、责任法人、客户账户、委托、包裹和明确字段/
// 资料范围，跨客户合报不得扩大可见范围」— 范围指向别的委托时不得追加。版本自带范围，若不
// 核对，一份指向他人委托的版本就能挂到手里这一份上，而下游是按范围去重新判断的。
func TestACustomerSourceDataVersionScopedToAnotherShipmentRequestIsRefused(t *testing.T) {
	accepted, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	spec := amendmentSpec(t)
	if spec.Scope, err = domain.NewShipmentScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "someone-elses-request"),
		mustValue(t, domain.NewSourceDataGroupReference, "CONSIGNEE_ADDRESS"),
	); err != nil {
		t.Fatalf("new shipment scoped source data: %v", err)
	}
	foreign, err := domain.FormCustomerSourceDataVersion(spec)
	if err != nil {
		t.Fatalf("form customer source data version: %v", err)
	}

	if _, err := accepted.AmendCustomerSourceData(foreign); !errors.Is(
		err, domain.ErrInvalidSourceDataScope,
	) {
		t.Fatalf("error = %v, want ErrInvalidSourceDataScope", err)
	}
	if len(accepted.CustomerSourceDataVersions()) != 0 {
		t.Fatal("a version scoped to another shipment request was kept anyway")
	}
}

// Covers: UC-PS-002「不得用 `occurredAt` 或 `receivedAt` 默认补齐缺失的 `requestEffectiveAt`」—
// 适用时间缺失是合法的，它与其余留痕不同：其余各项缺了就不成版本，它缺了只是「客户没说」。
// 因此它不在上一个测试的必填清单里，且缺失状态必须读得出来。
func TestAMissingRequestEffectiveAtIsRecordedAsAbsentRatherThanBackfilled(t *testing.T) {
	spec := amendmentSpec(t)
	spec.EffectiveAt = time.Time{}

	version, err := domain.FormCustomerSourceDataVersion(spec)
	if err != nil {
		t.Fatalf("form customer source data version: %v", err)
	}

	if version.HasEffectiveAt() {
		t.Fatal("an absent requestEffectiveAt was reported as present")
	}
	if !version.EffectiveAt().IsZero() {
		t.Fatalf("effective at = %v, want it backfilled by nothing", version.EffectiveAt())
	}
}
