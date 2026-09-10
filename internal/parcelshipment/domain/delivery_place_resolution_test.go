package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// acceptedWithDeliveryPlaceVersions 接受夹具委托（tenant-1 / request-1，成员 parcel-1、parcel-2），
// 再按给定链在**收件资料范围**（委托级、DeliveryPlaceDataGroup）上追加版本：每项 [版本, 前版]，
// 前版为空即以接受基线为基准的补充。与 acceptedWithVersions 同一配方，只是范围换成读口要查的那一个。
func acceptedWithDeliveryPlaceVersions(t *testing.T, chain ...[2]string) domain.ShipmentRequest {
	t.Helper()
	request, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	for _, link := range chain {
		spec := amendmentSpec(t)
		spec.Scope = deliveryPlaceScope(t, "request-1")
		spec.VersionID = mustValue(t, domain.NewSourceDataVersionID, link[0])
		if link[1] == "" {
			spec.Basis = domain.NewSupplementOnAcceptanceBaseline()
			spec.Intent = domain.SupplementIntent
		} else {
			if spec.Basis, err = domain.NewAmendmentOfVersion(
				mustValue(t, domain.NewSourceDataVersionID, link[1]),
			); err != nil {
				t.Fatalf("new amendment basis: %v", err)
			}
			spec.Intent = domain.CorrectionIntent
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

func resolveDeliveryPlace(t *testing.T, request domain.ShipmentRequest, parcel string) domain.DeliveryPlaceResolution {
	t.Helper()
	resolution, err := request.DeliveryPlaceReferenceFor(mustValue(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("delivery place reference for %q: %v", parcel, err)
	}
	return resolution
}

// Covers: ADR-0130 决定二「范围上无修订版本 → 引用带接受基线锚」，以及决定一「同一委托的两个包裹
// 答出同一个引用」——读口按包裹问，答的却是委托级引用，两个成员拿到逐字相同的串。
func TestAMemberOfAnAcceptedRequestWithoutAmendmentsGetsABaselineAnchoredReference(t *testing.T) {
	request := acceptedWithDeliveryPlaceVersions(t)

	first := resolveDeliveryPlace(t, request, "parcel-1")
	if first.Outcome() != domain.DeliveryPlaceAnchoredOnBaseline {
		t.Fatalf("outcome = %q, want ANCHORED_ON_BASELINE", first.Outcome())
	}
	reference, present := first.Reference()
	if !present {
		t.Fatal("a baseline anchored answer carries no reference")
	}
	if !reference.Anchor().OnAcceptanceBaseline() {
		t.Fatal("the reference is not anchored on the acceptance baseline")
	}
	if reference.TenantID().String() != "tenant-1" || reference.ShipmentRequestID().String() != "request-1" {
		t.Fatalf("reference names tenant %q request %q", reference.TenantID(), reference.ShipmentRequestID())
	}
	if reference.Scope() != deliveryPlaceScope(t, "request-1") {
		t.Fatalf("reference scope = %#v, want the shipment scoped delivery place group", reference.Scope())
	}

	second := resolveDeliveryPlace(t, request, "parcel-2")
	other, _ := second.Reference()
	if other.String() != reference.String() || other.String() == "" {
		t.Fatalf("two members of one request got different references: %q vs %q", reference.String(), other.String())
	}
}

// Covers: ADR-0130 决定二「`已采用` → 引用带该版本锚」与「取当前采用而不取接受基线……按基线派送等于
// 对一份已被客户更正的地址照旧送」。链尾那一版是锚，不是首版也不是基线。
func TestAnAdoptedAmendmentMovesTheAnchorToTheAdoptedVersion(t *testing.T) {
	request := acceptedWithDeliveryPlaceVersions(t,
		[2]string{"data-version-1", ""},
		[2]string{"data-version-2", "data-version-1"},
	)

	resolution := resolveDeliveryPlace(t, request, "parcel-2")
	if resolution.Outcome() != domain.DeliveryPlaceAnchoredOnAdoptedVersion {
		t.Fatalf("outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", resolution.Outcome())
	}
	reference, present := resolution.Reference()
	if !present {
		t.Fatal("an adopted version answer carries no reference")
	}
	adopted, anchored := reference.Anchor().AdoptedVersion()
	if !anchored || adopted.String() != "data-version-2" {
		t.Fatalf("anchor = %q anchored = %v, want data-version-2", adopted, anchored)
	}

	// 决定三：旧引用含义不变，修订只让新查询拿到换了锚的引用——两个引用委托与范围两段相同、锚不同。
	before, _ := resolveDeliveryPlace(t, acceptedWithDeliveryPlaceVersions(t), "parcel-2").Reference()
	if before.Scope() != reference.Scope() || before.ShipmentRequestID() != reference.ShipmentRequestID() {
		t.Fatal("the amendment changed more than the anchor")
	}
	if before.String() == reference.String() {
		t.Fatal("the amendment left the rendered reference unchanged, so TF could not tell the address moved")
	}
}

// Covers: ADR-0130 决定二「`待复核`（两条修订分叉未并）→ 不给引用，答『收件地点未定』」；Alternatives
// 「任选一版交出去就是在读口上偷做了那次合并」。分叉照 CurrentSourceDataAdoption 既有的那条路构造：两份
// 修订各自以 v1 为基准。
func TestAForkedAmendmentChainLeavesTheDeliveryPlaceUndetermined(t *testing.T) {
	request := acceptedWithDeliveryPlaceVersions(t,
		[2]string{"data-version-1", ""},
		[2]string{"data-version-2", "data-version-1"},
		[2]string{"data-version-3", "data-version-1"},
	)
	if judgment, _ := request.CurrentSourceDataAdoption(deliveryPlaceScope(t, "request-1")); judgment.Outcome() != domain.SourceDataAwaitingReview {
		t.Fatalf("fixture is not forked: adoption = %q", judgment.Outcome())
	}

	resolution := resolveDeliveryPlace(t, request, "parcel-1")
	if resolution.Outcome() != domain.DeliveryPlaceUndetermined {
		t.Fatalf("outcome = %q, want UNDETERMINED", resolution.Outcome())
	}
	if reference, present := resolution.Reference(); present || reference.String() != "" {
		t.Fatalf("an undetermined answer handed out a reference %q", reference.String())
	}
}

// Covers: ADR-0130 决定二「对象不属于任何已接受委托的成员集合……按统一不可见结果答『没有收件地点』」。
// 成员集合是接受基线（票面要落的第 3 条：经 AcceptanceBaseline 走声明成员）；未接受的委托没有基线，
// 其成员同样答没有——责任起点在接受之后，接受前没有可派送的收件地点。
func TestAnObjectOutsideTheAcceptanceBaselineHasNoDeliveryPlace(t *testing.T) {
	accepted := acceptedWithDeliveryPlaceVersions(t, [2]string{"data-version-1", ""})
	stranger := resolveDeliveryPlace(t, accepted, "parcel-9")
	if stranger.Outcome() != domain.NoDeliveryPlace {
		t.Fatalf("outcome for a non-member = %q, want NO_DELIVERY_PLACE", stranger.Outcome())
	}
	if _, present := stranger.Reference(); present {
		t.Fatal("a non-member got a reference")
	}

	pending := resolveDeliveryPlace(t, submitted(t), "parcel-1")
	if pending.Outcome() != domain.NoDeliveryPlace {
		t.Fatalf("outcome for a member of a merely submitted request = %q, want NO_DELIVERY_PLACE", pending.Outcome())
	}
}

// Covers: 票面要落的第 4 条——读口按本上下文的原词查收件范围，别的资料组上的修订不动收件地点的锚。
// 一份写在 CONSIGNEE_ADDRESS（矩阵侧另一个词）上的版本，读口不认；收件范围上没有版本就答基线锚，
// 这是如实答案不是默认。
func TestAmendmentsOnAnotherDataGroupDoNotMoveTheDeliveryPlaceAnchor(t *testing.T) {
	request := acceptedWithVersions(t, [2]string{"data-version-1", ""})

	resolution := resolveDeliveryPlace(t, request, "parcel-1")
	if resolution.Outcome() != domain.DeliveryPlaceAnchoredOnBaseline {
		t.Fatalf("outcome = %q, want ANCHORED_ON_BASELINE", resolution.Outcome())
	}
	reference, _ := resolution.Reference()
	if reference.Scope().DataGroup() != domain.DeliveryPlaceDataGroup() {
		t.Fatalf("reference scope group = %q, want the delivery place word", reference.Scope().DataGroup())
	}
}

// Covers: 四格的构造器各自只造自己那一格，且带引用的两格由锚决定格——一份基线锚引用不可能被包装成
// 「已采用版本锚」那一格。
func TestADeliveryPlaceResolutionDerivesItsCellFromTheAnchor(t *testing.T) {
	baseline := deliveryPlaceReference(t, "tenant-1", "request-1", domain.NewAcceptanceBaselineAnchor())
	resolved, err := domain.DeliveryPlaceReferenced(baseline)
	if err != nil {
		t.Fatalf("referenced: %v", err)
	}
	if resolved.Outcome() != domain.DeliveryPlaceAnchoredOnBaseline {
		t.Fatalf("outcome = %q, want ANCHORED_ON_BASELINE", resolved.Outcome())
	}

	adopted := deliveryPlaceReference(t, "tenant-1", "request-1", versionAnchor(t, "data-version-1"))
	resolved, err = domain.DeliveryPlaceReferenced(adopted)
	if err != nil {
		t.Fatalf("referenced: %v", err)
	}
	if resolved.Outcome() != domain.DeliveryPlaceAnchoredOnAdoptedVersion {
		t.Fatalf("outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", resolved.Outcome())
	}

	if _, err := domain.DeliveryPlaceReferenced(domain.DeliveryPlaceReference{}); err == nil {
		t.Fatal("a zero reference was wrapped into a resolution")
	}
	if undetermined := domain.DeliveryPlaceUndeterminedResolution(); undetermined.Outcome() != domain.DeliveryPlaceUndetermined {
		t.Fatalf("undetermined constructor produced %q", undetermined.Outcome())
	}
	if none := domain.NoDeliveryPlaceResolution(); none.Outcome() != domain.NoDeliveryPlace {
		t.Fatalf("no delivery place constructor produced %q", none.Outcome())
	}
	if (domain.DeliveryPlaceResolution{}).Outcome().String() != "" {
		t.Fatal("the zero outcome has a name")
	}
}
