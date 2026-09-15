package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// senderPlaceScope 是读口起点那一段要查的范围：委托级、寄件资料组原词。
func senderPlaceScope(t *testing.T, requestID string) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewShipmentScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, requestID), domain.SenderPlaceDataGroup(),
	)
	if err != nil {
		t.Fatalf("new sender place scope: %v", err)
	}
	return scope
}

// acceptedWithAddressVersions 接受夹具委托（tenant-1 / request-1，成员 parcel-1、parcel-2），再按给定链在指定范围上
// 追加版本：每项 [范围, 版本, 前版]，范围是 "sender" 或 "delivery"，前版为空即以接受基线为基准的补充。
func acceptedWithAddressVersions(t *testing.T, chain ...[3]string) domain.ShipmentRequest {
	t.Helper()
	request, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	for _, link := range chain {
		spec := amendmentSpec(t)
		switch link[0] {
		case "sender":
			spec.Scope = senderPlaceScope(t, "request-1")
		case "delivery":
			spec.Scope = deliveryPlaceScope(t, "request-1")
		default:
			t.Fatalf("unknown range %q", link[0])
		}
		spec.VersionID = mustValue(t, domain.NewSourceDataVersionID, link[1])
		if link[2] == "" {
			spec.Basis = domain.NewSupplementOnAcceptanceBaseline()
			spec.Intent = domain.SupplementIntent
		} else {
			if spec.Basis, err = domain.NewAmendmentOfVersion(
				mustValue(t, domain.NewSourceDataVersionID, link[2]),
			); err != nil {
				t.Fatalf("new amendment basis: %v", err)
			}
			spec.Intent = domain.CorrectionIntent
		}
		version, err := domain.FormCustomerSourceDataVersion(spec)
		if err != nil {
			t.Fatalf("form version %q: %v", link[1], err)
		}
		if request, err = request.AmendCustomerSourceData(version); err != nil {
			t.Fatalf("amend with version %q: %v", link[1], err)
		}
	}
	return request
}

func resolveAddressElements(t *testing.T, request domain.ShipmentRequest, parcel string) domain.ShipmentAddressElements {
	t.Helper()
	answer, err := request.AddressElementsFor(mustValue(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("address elements for %q: %v", parcel, err)
	}
	return answer
}

// Covers: 裁决 7（追裁取 A）——提交版本今天不携带地址要素，口对全部快照如实答「要素缺席」：两段都是 NOT_PROVIDED，
// 不带值也不带锚；同一委托的两个成员答一样（寄收件资料按委托级范围形成）。
func TestAMemberOfAnAcceptedRequestAnswersNotProvidedOnBothEndsToday(t *testing.T) {
	request := acceptedWithAddressVersions(t)

	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		answer := resolveAddressElements(t, request, parcel)
		for name, segment := range map[string]domain.AddressElementsResolution{
			"origin": answer.Origin(), "destination": answer.Destination(),
		} {
			if segment.Outcome() != domain.AddressElementsNotProvided {
				t.Fatalf("%s / %s outcome = %q, want NOT_PROVIDED", parcel, name, segment.Outcome())
			}
			if _, present := segment.Elements(); present {
				t.Fatalf("%s / %s 凭空长出了要素", parcel, name)
			}
			if _, anchored := segment.Anchor(); anchored {
				t.Fatalf("%s / %s 「要素缺席」那一格带了锚", parcel, name)
			}
		}
	}
}

// Covers: 裁决 2「一口两段，各段独立成格（一段有一段无是常态）」与「已采用版本 → 已采用版本锚」——收件范围上的修订被
// 采用后目的段换锚（只带锚不带值，理由同 02：客户原始资料版本今天只留痕不留内容），起点段不动。
func TestAnAdoptedDeliveryPlaceAmendmentMovesOnlyTheDestinationAnchor(t *testing.T) {
	request := acceptedWithAddressVersions(t,
		[3]string{"delivery", "dp-v1", ""},
		[3]string{"delivery", "dp-v2", "dp-v1"},
	)

	answer := resolveAddressElements(t, request, "parcel-1")
	destination := answer.Destination()
	if destination.Outcome() != domain.AddressElementsAnchoredOnAdoptedVersion {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", destination.Outcome())
	}
	anchor, anchored := destination.Anchor()
	adopted, onVersion := anchor.AdoptedVersion()
	if !anchored || !onVersion || adopted.String() != "dp-v2" {
		t.Fatalf("destination anchor = %q, want dp-v2（链尾那一版）", adopted)
	}
	if _, present := destination.Elements(); present {
		t.Fatal("已采用版本锚那一格交出了要素——那一版的内容本上下文没有")
	}
	if origin := answer.Origin(); origin.Outcome() != domain.AddressElementsNotProvided {
		t.Fatalf("origin outcome = %q, want NOT_PROVIDED（收件范围的修订动不到寄件段）", origin.Outcome())
	}
}

// Covers: 裁决 3「起点 = 寄件人邮编」——起点段查的是寄件资料范围（SENDER_PLACE），寄件范围上的分叉只让起点段答未定。
func TestAForkedSenderPlaceChainLeavesOnlyTheOriginUndetermined(t *testing.T) {
	request := acceptedWithAddressVersions(t,
		[3]string{"sender", "sp-v1", ""},
		[3]string{"sender", "sp-v2", "sp-v1"},
		[3]string{"sender", "sp-v3", "sp-v1"},
	)

	answer := resolveAddressElements(t, request, "parcel-2")
	if origin := answer.Origin(); origin.Outcome() != domain.AddressElementsUndetermined {
		t.Fatalf("origin outcome = %q, want UNDETERMINED", origin.Outcome())
	} else if _, anchored := origin.Anchor(); anchored {
		t.Fatal("`待复核`交出了锚")
	}
	if destination := answer.Destination(); destination.Outcome() != domain.AddressElementsNotProvided {
		t.Fatalf("destination outcome = %q, want NOT_PROVIDED", destination.Outcome())
	}
}

// Covers: 判据 3「非委托对象答无」——不在任何已接受委托成员集合里的对象两段都答无；仅`已提交`的委托没有基线，其成员同答。
func TestAnObjectOutsideEveryAcceptedBaselineHasNoAddressElements(t *testing.T) {
	stranger := resolveAddressElements(t, acceptedWithAddressVersions(t), "parcel-9")
	if stranger.Origin().Outcome() != domain.NoAddressElements || stranger.Destination().Outcome() != domain.NoAddressElements {
		t.Fatalf("outcomes = %q / %q, want NO_ADDRESS_ELEMENTS on both ends", stranger.Origin().Outcome(), stranger.Destination().Outcome())
	}

	pending := resolveAddressElements(t, submitted(t), "parcel-1")
	if pending.Origin().Outcome() != domain.NoAddressElements || pending.Destination().Outcome() != domain.NoAddressElements {
		t.Fatalf("outcomes for a merely submitted request = %q / %q, want NO_ADDRESS_ELEMENTS", pending.Origin().Outcome(), pending.Destination().Outcome())
	}
}

// Covers: 五格的构造器各造各格——基线格必须带至少一个要素（两个都缺是「要素缺席」那一格，不是空的基线格），已采用格
// 只收指着某一版的锚；零值没有名字。
func TestAddressElementsResolutionCellsAreBuiltByTheirOwnConstructors(t *testing.T) {
	elements := domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), []domain.CanonicalContentEntry{
		contentEntry(t, "DELIVERY_PLACE.POSTAL_CODE", "10115"),
	})
	onBaseline, err := domain.AddressElementsOnBaseline(elements)
	if err != nil {
		t.Fatalf("on baseline: %v", err)
	}
	if onBaseline.Outcome() != domain.AddressElementsAnchoredOnBaseline {
		t.Fatalf("outcome = %q, want ANCHORED_ON_BASELINE", onBaseline.Outcome())
	}
	got, present := onBaseline.Elements()
	if postal, declared := got.PostalCode(); !present || !declared || postal != "10115" {
		t.Fatalf("elements = %#v present = %v", got, present)
	}
	if anchor, anchored := onBaseline.Anchor(); !anchored || !anchor.OnAcceptanceBaseline() {
		t.Fatal("基线格没带基线锚")
	}
	if _, err := domain.AddressElementsOnBaseline(domain.AddressElements{}); err == nil {
		t.Fatal("两个要素都缺的基线格被造出来了")
	}

	if _, err := domain.AddressElementsOnAdoptedVersion(domain.NewAcceptanceBaselineAnchor()); err == nil {
		t.Fatal("基线锚被包成了已采用版本格")
	}
	adopted, err := domain.AddressElementsOnAdoptedVersion(versionAnchor(t, "dp-v1"))
	if err != nil || adopted.Outcome() != domain.AddressElementsAnchoredOnAdoptedVersion {
		t.Fatalf("adopted = %#v err = %v", adopted, err)
	}

	names := map[domain.AddressElementsOutcome]string{
		domain.AddressElementsAnchoredOnBaseline:       "ANCHORED_ON_BASELINE",
		domain.AddressElementsAnchoredOnAdoptedVersion: "ANCHORED_ON_ADOPTED_VERSION",
		domain.AddressElementsUndetermined:             "UNDETERMINED",
		domain.AddressElementsNotProvided:              "NOT_PROVIDED",
		domain.NoAddressElements:                       "NO_ADDRESS_ELEMENTS",
	}
	for outcome, want := range names {
		if outcome.String() != want {
			t.Fatalf("%d.String() = %q, want %q", outcome, outcome.String(), want)
		}
	}
	if (domain.AddressElementsResolution{}).Outcome().String() != "" {
		t.Fatal("the zero outcome has a name")
	}
}
