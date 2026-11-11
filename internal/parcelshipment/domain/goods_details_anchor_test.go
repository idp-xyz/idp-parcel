package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉货物明细按资料版本锚的读法（ADR-0373 决定九，票 declared-goods-lines/07）：锚记下之后读回的永远是那一版；版本锚只认
// 货物明细范围上的客户原始资料版本；分叉时当前一问只答待复核；没接受的委托一律不可见。

// Covers: 锚不随采用判断漂移、只认货物明细范围——被后来修订顶掉的版本锚照样答它那一版；显式清空那一版答「在、但没有货物明细」；
// 别的资料组的版本、提交版本的标识、不存在的版本都答不可见。
func TestAGoodsAnchorReadsItsOwnVersionAndNothingElse(t *testing.T) {
	request := acceptedWithGoods(t, goodsOf(t, [][2]string{{"T-shirt", "2"}}))
	baseline, _ := request.AcceptanceBaseline()
	var err error
	for _, version := range []domain.CustomerSourceDataVersion{
		goodsVersion(t, "goods-v1", "", domain.SupplementIntent, goodsOf(t, [][2]string{{"Cap", "1"}})),
		goodsVersion(t, "goods-v2", "goods-v1", domain.ExplicitClearIntent, domain.GoodsDetails{}),
	} {
		if request, err = request.AmendCustomerSourceData(version); err != nil {
			t.Fatalf("修订：%v", err)
		}
	}
	other := amendmentSpec(t)
	other.VersionID = mustValue(t, domain.NewSourceDataVersionID, "consignee-v1")
	otherVersion, err := domain.FormCustomerSourceDataVersion(other)
	if err != nil {
		t.Fatal(err)
	}
	if request, err = request.AmendCustomerSourceData(otherVersion); err != nil {
		t.Fatalf("别的资料组的修订：%v", err)
	}

	if goods, found := request.GoodsDetailsAt(mustAnchor(t, "goods-v1")); !found || descriptionsOf(goods)[0] != "Cap" {
		t.Fatalf("被顶掉的 goods-v1：found=%v", found)
	}
	if goods, found := request.GoodsDetailsAt(mustAnchor(t, "goods-v2")); !found || goods.Declared() {
		t.Fatalf("显式清空那一版：found=%v declared=%v, want 在且没有货物明细", found, goods.Declared())
	}
	if goods, found := request.GoodsDetailsAt(domain.NewAcceptanceBaselineAnchor()); !found || descriptionsOf(goods)[0] != "T-shirt" {
		t.Fatalf("基线锚：found=%v", found)
	}
	for name, anchor := range map[string]domain.SourceDataVersionAnchor{
		"别的资料组的版本": mustAnchor(t, "consignee-v1"),
		"提交版本的标识":  mustAnchor(t, baseline.SubmissionVersionID().String()),
		"不存在的版本":   mustAnchor(t, "goods-v9"),
	} {
		if _, found := request.GoodsDetailsAt(anchor); found {
			t.Errorf("%s：答了可见", name)
		}
	}
	resolution, found := request.GoodsDetailsResolutionAt(mustAnchor(t, "goods-v1"))
	if !found || len(resolution.Members()) != 2 {
		t.Fatalf("带成员的答复：found=%v 成员=%d, want 接受基线的两件", found, len(resolution.Members()))
	}
}

// Covers: 当前一问在货物明细范围分叉时只答待复核，不给锚也不给货物明细，成员照交；没接受的委托不可见。
func TestTheCurrentGoodsDetailsHoldOnAForkAndAreInvisibleBeforeAcceptance(t *testing.T) {
	request := acceptedWithGoods(t, goodsOf(t, [][2]string{{"T-shirt", "1"}}))
	var err error
	for _, version := range []domain.CustomerSourceDataVersion{
		goodsVersion(t, "goods-v1", "", domain.SupplementIntent, goodsOf(t, [][2]string{{"A", "1"}})),
		goodsVersion(t, "goods-v2", "goods-v1", domain.CorrectionIntent, goodsOf(t, [][2]string{{"B", "1"}})),
		goodsVersion(t, "goods-v3", "goods-v1", domain.CorrectionIntent, goodsOf(t, [][2]string{{"C", "1"}})),
	} {
		if request, err = request.AmendCustomerSourceData(version); err != nil {
			t.Fatalf("修订：%v", err)
		}
	}
	current, found, err := request.CurrentGoodsDetails()
	if err != nil || !found || !current.Undetermined() || current.Goods().Declared() || len(current.Members()) != 2 {
		t.Fatalf("分叉：found=%v err=%v undetermined=%v", found, err, current.Undetermined())
	}
	if _, anchored := current.Anchor(); anchored {
		t.Fatal("分叉时给了锚")
	}

	pending := submitted(t)
	if _, found, err := pending.CurrentGoodsDetails(); found || err != nil {
		t.Fatalf("还没接受：found=%v err=%v", found, err)
	}
	if _, found := pending.GoodsDetailsAt(domain.NewAcceptanceBaselineAnchor()); found {
		t.Fatal("还没接受就按基线锚读到了")
	}
}
