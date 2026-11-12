package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ports.GoodsDetailsView（ADR-0373 决定九，票 declared-goods-lines/07）：基线锚读回接受时那一版；
// 资料修订被采用后，当前锚指那一版资料修订，而基线锚照旧读回原来那一份——锚记下之后按它读回的永远是同一份行；锚指的版本不在
// 货物明细范围上、他租户、还没接受，都答不可见。夹具真接受（Decide → Save），不只翻 state 列。

var _ ports.GoodsDetailsView = (*adapter.ShipmentRequests)(nil)

func TestTheGoodsDetailsViewReadsByAnchorAndKeepsAnOldAnchorStable(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()
	tenant := mustBuild(t, domain.NewTenantID, "tenant-1")
	identity := requestScopedIdentity(t, "tenant-1", "customer-1", "goods-view-1")
	requestID := mustBuild(t, domain.NewShipmentRequestID, "REQ-GV1")

	mustInsert(t, transactor, ctx, repository, numberedRequestWith(t, "tenant-1", "customer-1", "goods-view-1", "REQ-GV1",
		[]string{"parcel-1", "parcel-2"}, nil, func(spec *domain.SubmitShipmentRequestSpec) { spec.Goods = roundTripGoods(t) }))
	if _, found, err := repository.LoadCurrentGoodsDetails(ctx, tenant, requestID); err != nil || found {
		t.Fatalf("还没接受：found=%v err=%v, want 不可见", found, err)
	}

	loaded, _, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := loaded.Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, identity, accepted)

	current, found, err := repository.LoadCurrentGoodsDetails(ctx, tenant, requestID)
	if err != nil || !found {
		t.Fatalf("当前：found=%v err=%v", found, err)
	}
	if anchor, anchored := current.Anchor(); !anchored || !anchor.OnAcceptanceBaseline() || len(current.Goods().Lines()) != 2 {
		t.Fatalf("没修订时当前锚要在接受基线上、带提交时的两行：%+v", current)
	}

	// 资料修订：整组换成一行一箱。
	scope, err := domain.NewShipmentScopedSourceData(requestID, domain.GoodsDetailsDataGroup())
	if err != nil {
		t.Fatal(err)
	}
	parcel1, parcel2 := mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-2")
	replacement, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{
		Lines: []domain.GoodsLineSpec{{Quantity: cell("2", domain.SourceMarkThisDeclaration)}},
		Packing: []domain.PackingAllocationSpec{
			{Parcel: parcel1, Line: 1, Quantity: "1"}, {Parcel: parcel2, Line: 1, Quantity: "1"},
		},
	}, []domain.DeclaredParcelID{parcel1, parcel2})
	if err != nil {
		t.Fatal(err)
	}
	content, err := domain.NewGoodsDetailsContent(replacement)
	if err != nil {
		t.Fatal(err)
	}
	version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
		VersionID: mustBuild(t, domain.NewSourceDataVersionID, "SDV-GV1"),
		Scope:     scope,
		Basis:     domain.NewSupplementOnAcceptanceBaseline(),
		Intent:    domain.SupplementIntent,
		Request:   requestFingerprint(t, "goods-view-1-amend", "digest-goods-view-1"),
		Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "CUSTOMS_RETURN"),
		Requester: mustBuild(t, domain.NewRequesterReference, "CUSTOMER-1"),
		Decider:   mustBuild(t, domain.NewDeciderReference, "OPERATOR-1"),
		Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "AUTH-SNAP-1"),
		Content:   content,
		FormedAt:  submittedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成资料版本：%v", err)
	}
	stored, _, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	amended, err := stored.AmendCustomerSourceData(version)
	if err != nil {
		t.Fatalf("追加资料版本：%v", err)
	}
	mustSave(t, transactor, ctx, repository, identity, amended)

	current, found, err = repository.LoadCurrentGoodsDetails(ctx, tenant, requestID)
	anchor, _ := current.Anchor()
	adopted, _ := anchor.AdoptedVersion()
	if err != nil || !found || adopted.String() != "SDV-GV1" || len(current.Goods().Lines()) != 1 {
		t.Fatalf("修订后当前：found=%v err=%v anchor=%s 行数=%d, want SDV-GV1 一行", found, err, adopted, len(current.Goods().Lines()))
	}
	onBaseline, found, err := repository.LoadGoodsDetailsAt(ctx, tenant, requestID, domain.NewAcceptanceBaselineAnchor())
	if err != nil || !found || len(onBaseline.Goods().Lines()) != 2 || len(onBaseline.Members()) != 2 {
		t.Fatalf("基线锚读回：found=%v err=%v 行数=%d, want 照旧两行、两个成员", found, err, len(onBaseline.Goods().Lines()))
	}

	// 别的资料组的一版：收件资料范围上一份不带内容的补充。拿它的版本号当货物明细的锚，答不可见，不答「没报」。
	deliveryScope, err := domain.NewShipmentScopedSourceData(requestID, domain.DeliveryPlaceDataGroup())
	if err != nil {
		t.Fatal(err)
	}
	other, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
		VersionID: mustBuild(t, domain.NewSourceDataVersionID, "SDV-GV-DELIVERY"),
		Scope:     deliveryScope,
		Basis:     domain.NewSupplementOnAcceptanceBaseline(),
		Intent:    domain.SupplementIntent,
		Request:   requestFingerprint(t, "goods-view-1-amend-2", "digest-goods-view-2"),
		Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "ADDRESS_RESTATED"),
		Requester: mustBuild(t, domain.NewRequesterReference, "CUSTOMER-1"),
		Decider:   mustBuild(t, domain.NewDeciderReference, "OPERATOR-1"),
		Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "AUTH-SNAP-1"),
		FormedAt:  submittedAtFixture.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, _, err = repository.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	withOther, err := stored.AmendCustomerSourceData(other)
	if err != nil {
		t.Fatal(err)
	}
	mustSave(t, transactor, ctx, repository, identity, withOther)

	for name, read := range map[string]func() (bool, error){
		"锚指别的资料组的一版": func() (bool, error) {
			anchor, err := domain.NewSourceDataVersionAnchor(mustBuild(t, domain.NewSourceDataVersionID, "SDV-GV-DELIVERY"))
			if err != nil {
				t.Fatal(err)
			}
			_, found, err := repository.LoadGoodsDetailsAt(ctx, tenant, requestID, anchor)
			return found, err
		},
		"锚指的版本不存在": func() (bool, error) {
			anchor, err := domain.NewSourceDataVersionAnchor(mustBuild(t, domain.NewSourceDataVersionID, "SDV-NOPE"))
			if err != nil {
				t.Fatal(err)
			}
			_, found, err := repository.LoadGoodsDetailsAt(ctx, tenant, requestID, anchor)
			return found, err
		},
		"他租户": func() (bool, error) {
			_, found, err := repository.LoadGoodsDetailsAt(ctx, mustBuild(t, domain.NewTenantID, "tenant-2"), requestID, domain.NewAcceptanceBaselineAnchor())
			return found, err
		},
		"他租户·当前": func() (bool, error) {
			_, found, err := repository.LoadCurrentGoodsDetails(ctx, mustBuild(t, domain.NewTenantID, "tenant-2"), requestID)
			return found, err
		},
	} {
		if found, err := read(); found || err != nil {
			t.Errorf("%s：found=%v err=%v, want 不可见", name, found, err)
		}
	}
}
