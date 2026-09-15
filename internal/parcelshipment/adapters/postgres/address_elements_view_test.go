package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ports.AddressElementsView 一口两段的封闭答格（pp-seams/03 完成判据 3 与裁决 7 追裁）：
// 锚 / 未定 / 无三格真库能真答，而「要素缺席」是今天全部快照的答案——提交版本不携带地址要素（条目只进 PayloadDigest），
// 内容落库归 pp-seams/05。夹具与收件地点引用的同款：真接受（Decide → Save）而不是只翻 state 列。

var _ ports.AddressElementsView = (*adapter.ShipmentRequests)(nil)

// acceptedRequestWithAddressChain 插入并真接受夹具委托，再按给定链在指定范围上逐份追加版本并保存：每项
// [范围, 版本, 前版]，范围是 "sender"（SenderPlaceDataGroup）或 "delivery"（DeliveryPlaceDataGroup），皆委托级；前版为空
// 即以接受基线为基准的补充。
func acceptedRequestWithAddressChain(
	t *testing.T,
	repository *adapter.ShipmentRequests,
	transactor bentoapp.Transactor,
	key, requestID string,
	chain ...[3]string,
) {
	t.Helper()
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, key, requestID))
	accepted, err := mustFind(t, repository, ctx, key).Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, key), accepted)

	for index, link := range chain {
		group := domain.DeliveryPlaceDataGroup()
		if link[0] == "sender" {
			group = domain.SenderPlaceDataGroup()
		}
		scope, err := domain.NewShipmentScopedSourceData(mustBuild(t, domain.NewShipmentRequestID, requestID), group)
		if err != nil {
			t.Fatalf("资料范围：%v", err)
		}
		current := mustFind(t, repository, ctx, key)
		basis := domain.NewSupplementOnAcceptanceBaseline()
		intent := domain.SupplementIntent
		if link[2] != "" {
			if basis, err = domain.NewAmendmentOfVersion(mustBuild(t, domain.NewSourceDataVersionID, link[2])); err != nil {
				t.Fatalf("前版基准：%v", err)
			}
			intent = domain.CorrectionIntent
		}
		version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
			VersionID: mustBuild(t, domain.NewSourceDataVersionID, link[1]),
			Scope:     scope,
			Basis:     basis,
			Intent:    intent,
			Request:   requestFingerprint(t, key+"-amend-"+link[1], "digest-"+link[1]),
			Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "ADDRESS_RESTATED"),
			Requester: mustBuild(t, domain.NewRequesterReference, "CUSTOMER-1"),
			Decider:   mustBuild(t, domain.NewDeciderReference, "OPERATOR-1"),
			Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "AUTH-SNAP-1"),
			FormedAt:  submittedAtFixture.Add(time.Duration(index+2) * time.Hour),
		})
		if err != nil {
			t.Fatalf("形成资料版本 %q：%v", link[1], err)
		}
		amended, err := current.AmendCustomerSourceData(version)
		if err != nil {
			t.Fatalf("追加资料版本 %q：%v", link[1], err)
		}
		mustSave(t, transactor, ctx, repository, requestIdentity(t, key), amended)
	}
}

func loadAddressElements(
	t *testing.T,
	view ports.AddressElementsView,
	tenant, parcel string,
) domain.ShipmentAddressElements {
	t.Helper()
	answer, err := view.LoadAddressElements(t.Context(),
		mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("取地址要素（%s / %s）：%v", tenant, parcel, err)
	}
	return answer
}

// 今天全部快照的答案：提交版本不携带地址要素，两段都是「要素缺席」，不带值不带锚——不是「无」（它是委托成员）。
func TestAMemberOfAnAcceptedRequestAnswersNotProvidedOnBothEndsFromTheSnapshot(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithAddressChain(t, repository, transactor, "req-key-1", "request-1")

	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		answer := loadAddressElements(t, repository, "tenant-1", parcel)
		for name, segment := range map[string]domain.AddressElementsResolution{
			"origin": answer.Origin(), "destination": answer.Destination(),
		} {
			if segment.Outcome() != domain.AddressElementsNotProvided {
				t.Fatalf("%s / %s outcome = %q, want NOT_PROVIDED", parcel, name, segment.Outcome())
			}
			if _, present := segment.Elements(); present {
				t.Fatalf("%s / %s 凭空长出了要素", parcel, name)
			}
		}
	}
}

// 锚那一格真库能真答：收件范围上的修订被采用后目的段换成已采用版本锚（只带锚），起点段仍答缺——两段各自成格。
func TestAnAdoptedDeliveryPlaceAmendmentMovesOnlyTheDestinationAnchorInTheStore(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithAddressChain(t, repository, transactor, "req-key-1", "request-1",
		[3]string{"delivery", "dp-v1", ""},
		[3]string{"delivery", "dp-v2", "dp-v1"},
	)

	answer := loadAddressElements(t, repository, "tenant-1", "parcel-2")
	destination := answer.Destination()
	if destination.Outcome() != domain.AddressElementsAnchoredOnAdoptedVersion {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", destination.Outcome())
	}
	anchor, anchored := destination.Anchor()
	adopted, onVersion := anchor.AdoptedVersion()
	if !anchored || !onVersion || adopted.String() != "dp-v2" {
		t.Fatalf("destination anchor = %q, want dp-v2", adopted)
	}
	if _, present := destination.Elements(); present {
		t.Fatal("已采用版本锚那一格交出了要素")
	}
	if origin := answer.Origin(); origin.Outcome() != domain.AddressElementsNotProvided {
		t.Fatalf("origin outcome = %q, want NOT_PROVIDED", origin.Outcome())
	}
}

// 未定那一格真库能真答：寄件范围上两条修订分叉，起点段答未定，目的段不受影响。
func TestAForkedSenderPlaceChainLeavesOnlyTheOriginUndeterminedInTheStore(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithAddressChain(t, repository, transactor, "req-key-1", "request-1",
		[3]string{"sender", "sp-v1", ""},
		[3]string{"sender", "sp-v2", "sp-v1"},
		[3]string{"sender", "sp-v3", "sp-v1"},
	)

	answer := loadAddressElements(t, repository, "tenant-1", "parcel-1")
	if origin := answer.Origin(); origin.Outcome() != domain.AddressElementsUndetermined {
		t.Fatalf("origin outcome = %q, want UNDETERMINED", origin.Outcome())
	} else if _, anchored := origin.Anchor(); anchored {
		t.Fatal("`待复核`交出了锚")
	}
	if destination := answer.Destination(); destination.Outcome() != domain.AddressElementsNotProvided {
		t.Fatalf("destination outcome = %q, want NOT_PROVIDED", destination.Outcome())
	}
}

// 无那一格真库能真答：非成员、他租户、仅`已提交`的委托成员，两段都按统一不可见结果答无。
func TestAnObjectOutsideEveryAcceptedBaselineHasNoAddressElementsInTheStore(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithAddressChain(t, repository, transactor, "req-key-1", "request-1")
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-2", "request-2"))

	cases := map[string][2]string{
		"从未声明过的包裹":        {"tenant-1", "parcel-9"},
		"他租户问本租户已接受委托的成员": {"tenant-b", "parcel-1"},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			answer := loadAddressElements(t, repository, item[0], item[1])
			if answer.Origin().Outcome() != domain.NoAddressElements || answer.Destination().Outcome() != domain.NoAddressElements {
				t.Fatalf("outcomes = %q / %q, want NO_ADDRESS_ELEMENTS", answer.Origin().Outcome(), answer.Destination().Outcome())
			}
		})
	}
}

// 两份已接受委托同时声明同一件包裹是读面坏了（ADR-0060 的歧义），不是任何一格。
func TestTwoAcceptedRequestsClaimingOneParcelAreAReadFaceFailureForAddressElements(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithAddressChain(t, repository, transactor, "req-key-1", "request-1")
	acceptedRequestWithAddressChain(t, repository, transactor, "req-key-2", "request-2")

	answer, err := repository.LoadAddressElements(t.Context(),
		mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !errors.Is(err, domain.ErrAmbiguousParcelTarget) {
		t.Fatalf("err = %v, want ErrAmbiguousParcelTarget", err)
	}
	if answer != (domain.ShipmentAddressElements{}) {
		t.Fatalf("上抛的同时还交回了答复 %#v", answer)
	}
}

func TestAddressElementsLookupRequiresTenantAndParcel(t *testing.T) {
	repository, _, _ := newShipmentRequests(t)
	cases := []struct {
		name   string
		tenant domain.TenantID
		parcel domain.DeclaredParcelID
	}{
		{"缺租户", domain.TenantID{}, mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")},
		{"缺包裹", mustBuild(t, domain.NewTenantID, "tenant-1"), domain.DeclaredParcelID{}},
	}
	for _, item := range cases {
		answer, err := repository.LoadAddressElements(context.Background(), item.tenant, item.parcel)
		if err == nil {
			t.Fatalf("%s 没有上抛，而是拿残缺的键去查了：%#v", item.name, answer)
		}
		if answer != (domain.ShipmentAddressElements{}) {
			t.Fatalf("%s 上抛的同时还交回了答复 %#v", item.name, answer)
		}
	}
}
