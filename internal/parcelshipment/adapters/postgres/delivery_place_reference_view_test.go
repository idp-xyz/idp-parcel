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

// 本文件对真实 PostgreSQL 16 证 ports.DeliveryPlaceReferenceView 的封闭四格（票 ps-port-remainder/06 完成判据 3；
// ADR-0130 决定二）：基线锚 / 已采用版本锚 / `待复核`不给引用 / 不属成员集合答没有。四格之外证两件读面纪律：
// 他租户按统一不可见结果答「没有」而不是错误；同一件包裹被两份已接受委托同时声明是读面坏了，上抛歧义而不挑一份。
//
// 夹具走真接受（Decide → Save）而不是只翻 state 列：读口要走的是接受基线的成员集合，只翻列的行没有基线，
// 重建门会把它当坏快照拒掉——那恰好证明读口没有绕过基线去读投影列。

var _ ports.DeliveryPlaceReferenceView = (*adapter.ShipmentRequests)(nil)

// acceptedRequestWithDeliveryPlaceChain 把一份两成员（parcel-1、parcel-2）的委托插入、真接受，再按给定链在
// 收件资料范围（委托级、DeliveryPlaceDataGroup）上逐份追加版本并保存：每项 [版本, 前版]，前版为空即以接受
// 基线为基准的补充。与 domain 测试同名夹具同一配方，只是每一步都落库。
func acceptedRequestWithDeliveryPlaceChain(
	t *testing.T,
	repository *adapter.ShipmentRequests,
	transactor bentoapp.Transactor,
	key, requestID string,
	chain ...[2]string,
) {
	t.Helper()
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, key, requestID))
	accepted, err := mustFind(t, repository, ctx, key).Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, key), accepted)

	scope, err := domain.NewShipmentScopedSourceData(
		mustBuild(t, domain.NewShipmentRequestID, requestID), domain.DeliveryPlaceDataGroup(),
	)
	if err != nil {
		t.Fatalf("收件资料范围：%v", err)
	}
	for index, link := range chain {
		current := mustFind(t, repository, ctx, key)
		basis := domain.NewSupplementOnAcceptanceBaseline()
		intent := domain.SupplementIntent
		if link[1] != "" {
			if basis, err = domain.NewAmendmentOfVersion(mustBuild(t, domain.NewSourceDataVersionID, link[1])); err != nil {
				t.Fatalf("前版基准：%v", err)
			}
			intent = domain.CorrectionIntent
		}
		version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
			VersionID: mustBuild(t, domain.NewSourceDataVersionID, link[0]),
			Scope:     scope,
			Basis:     basis,
			Intent:    intent,
			Request:   requestFingerprint(t, key+"-amend-"+link[0], "digest-"+link[0]),
			Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "RECIPIENT_MOVED"),
			Requester: mustBuild(t, domain.NewRequesterReference, "CUSTOMER-1"),
			Decider:   mustBuild(t, domain.NewDeciderReference, "OPERATOR-1"),
			Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "AUTH-SNAP-1"),
			FormedAt:  submittedAtFixture.Add(time.Duration(index+2) * time.Hour),
		})
		if err != nil {
			t.Fatalf("形成资料版本 %q：%v", link[0], err)
		}
		amended, err := current.AmendCustomerSourceData(version)
		if err != nil {
			t.Fatalf("追加资料版本 %q：%v", link[0], err)
		}
		mustSave(t, transactor, ctx, repository, requestIdentity(t, key), amended)
	}
}

func loadDeliveryPlace(
	t *testing.T,
	view ports.DeliveryPlaceReferenceView,
	tenant, parcel string,
) domain.DeliveryPlaceResolution {
	t.Helper()
	resolution, err := view.LoadDeliveryPlaceReference(t.Context(),
		mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("取收件地点引用（%s / %s）：%v", tenant, parcel, err)
	}
	return resolution
}

func TestAnUnamendedMemberIsAnchoredOnTheAcceptanceBaseline(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1")

	first := loadDeliveryPlace(t, repository, "tenant-1", "parcel-1")
	if first.Outcome() != domain.DeliveryPlaceAnchoredOnBaseline {
		t.Fatalf("outcome = %q, want ANCHORED_ON_BASELINE", first.Outcome())
	}
	reference, present := first.Reference()
	if !present {
		t.Fatal("基线锚那一格没带引用")
	}
	if got, want := reference.String(), "DPR-1:tenant-1/request-1/DELIVERY_PLACE/baseline"; got != want {
		t.Fatalf("reference = %q, want %q", got, want)
	}

	// 委托级：同一委托的第二个成员拿到逐字相同的串（ADR-0130 决定一）。
	second := loadDeliveryPlace(t, repository, "tenant-1", "parcel-2")
	other, _ := second.Reference()
	if other.String() != reference.String() {
		t.Fatalf("同一委托两个成员的引用不同：%q vs %q", reference.String(), other.String())
	}
}

func TestAnAdoptedAmendmentMovesTheAnchorToThatVersion(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1",
		[2]string{"dp-ver-1", ""},
		[2]string{"dp-ver-2", "dp-ver-1"},
	)

	resolution := loadDeliveryPlace(t, repository, "tenant-1", "parcel-2")
	if resolution.Outcome() != domain.DeliveryPlaceAnchoredOnAdoptedVersion {
		t.Fatalf("outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", resolution.Outcome())
	}
	reference, present := resolution.Reference()
	if !present {
		t.Fatal("已采用版本锚那一格没带引用")
	}
	if got, want := reference.String(), "DPR-1:tenant-1/request-1/DELIVERY_PLACE/version;dp-ver-2"; got != want {
		t.Fatalf("reference = %q, want %q（锚是链尾那一版，不是首版也不是基线）", got, want)
	}
}

func TestAForkedAmendmentChainAnswersUndeterminedWithoutAReference(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1",
		[2]string{"dp-ver-1", ""},
		[2]string{"dp-ver-2", "dp-ver-1"},
		[2]string{"dp-ver-3", "dp-ver-1"},
	)

	resolution := loadDeliveryPlace(t, repository, "tenant-1", "parcel-1")
	if resolution.Outcome() != domain.DeliveryPlaceUndetermined {
		t.Fatalf("outcome = %q, want UNDETERMINED", resolution.Outcome())
	}
	if reference, present := resolution.Reference(); present || reference.String() != "" {
		t.Fatalf("`待复核`交出了引用 %q——任选一版就是在读口上偷做了那次合并", reference.String())
	}
}

func TestAnObjectOutsideEveryAcceptedBaselineHasNoDeliveryPlace(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1", [2]string{"dp-ver-1", ""})

	cases := map[string][2]string{
		"从未声明过的包裹":        {"tenant-1", "parcel-9"},
		"他租户问本租户已接受委托的成员": {"tenant-b", "parcel-1"},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			resolution := loadDeliveryPlace(t, repository, item[0], item[1])
			if resolution.Outcome() != domain.NoDeliveryPlace {
				t.Fatalf("outcome = %q, want NO_DELIVERY_PLACE", resolution.Outcome())
			}
			if _, present := resolution.Reference(); present {
				t.Fatal("「没有」那一格带了引用")
			}
		})
	}
}

// 责任起点在接受之后：仅`已提交`的委托没有基线，它的成员今天没有可派送的收件地点。
func TestAMemberOfAMerelySubmittedRequestHasNoDeliveryPlace(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-1", "request-1"))

	resolution := loadDeliveryPlace(t, repository, "tenant-1", "parcel-1")
	if resolution.Outcome() != domain.NoDeliveryPlace {
		t.Fatalf("outcome = %q, want NO_DELIVERY_PLACE", resolution.Outcome())
	}
}

// 两份已接受委托同时声明同一件包裹是读面坏了（ADR-0060 的歧义），不是四格里的任何一格：读口不挑一份作答。
func TestTwoAcceptedRequestsClaimingOneParcelAreAReadFaceFailure(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1")
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-2", "request-2")

	resolution, err := repository.LoadDeliveryPlaceReference(t.Context(),
		mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !errors.Is(err, domain.ErrAmbiguousParcelTarget) {
		t.Fatalf("err = %v, want ErrAmbiguousParcelTarget", err)
	}
	if resolution != (domain.DeliveryPlaceResolution{}) {
		t.Fatalf("上抛的同时还交回了答复 %#v", resolution)
	}
}

func TestDeliveryPlaceReferenceLookupRequiresTenantAndParcel(t *testing.T) {
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
		resolution, err := repository.LoadDeliveryPlaceReference(context.Background(), item.tenant, item.parcel)
		if err == nil {
			t.Fatalf("%s 没有上抛，而是拿残缺的键去查了：%#v", item.name, resolution)
		}
		if resolution != (domain.DeliveryPlaceResolution{}) {
			t.Fatalf("%s 上抛的同时还交回了答复 %#v", item.name, resolution)
		}
	}
}
