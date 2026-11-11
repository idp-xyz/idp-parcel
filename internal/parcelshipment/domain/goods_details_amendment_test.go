package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉委托级资料组「货物明细」的修订（ADR-0373 决定四，票 declared-goods-lines/06，AT-PS-185）：整组替换、行号按新版本
// 重编、旧版本与接受基线不动；装箱分配以接受基线为界；在用的货物明细跟着采用判断走。

func goodsScope(t *testing.T) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewShipmentScopedSourceData(mustValue(t, domain.NewShipmentRequestID, "request-1"), domain.GoodsDetailsDataGroup())
	if err != nil {
		t.Fatalf("货物明细范围：%v", err)
	}
	return scope
}

func cellOf(value string) domain.GoodsCellSpec {
	return domain.GoodsCellSpec{Value: value, Mark: domain.SourceMarkThisDeclaration}
}

// goodsOf 造一份货物明细：每项 [英文品名, 数量]；packing 每项 [包裹, 行号, 数量]，空即整票清单。
func goodsOf(t *testing.T, lines [][2]string, packing ...[3]string) domain.GoodsDetails {
	t.Helper()
	spec := domain.GoodsDetailsSpec{Category: domain.ContentCategorySaleOfGoods, CategoryMark: domain.SourceMarkThisDeclaration}
	for _, line := range lines {
		spec.Lines = append(spec.Lines, domain.GoodsLineSpec{
			Descriptions: []domain.GoodsDescriptionSpec{{Language: "en", Text: cellOf(line[0])}},
			Quantity:     cellOf(line[1]),
		})
	}
	members := []domain.DeclaredParcelID{}
	seen := map[string]bool{}
	for _, allocation := range packing {
		line := 0
		for _, digit := range allocation[1] {
			line = line*10 + int(digit-'0')
		}
		spec.Packing = append(spec.Packing, domain.PackingAllocationSpec{
			Parcel: mustValue(t, domain.NewDeclaredParcelID, allocation[0]), Line: line, Quantity: allocation[2],
		})
		if !seen[allocation[0]] {
			seen[allocation[0]] = true
			members = append(members, mustValue(t, domain.NewDeclaredParcelID, allocation[0]))
		}
	}
	goods, err := domain.NewGoodsDetails(spec, members)
	if err != nil {
		t.Fatalf("货物明细：%v", err)
	}
	return goods
}

func acceptedWithGoods(t *testing.T, goods domain.GoodsDetails) domain.ShipmentRequest {
	t.Helper()
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.Goods = goods
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	if request, err = request.Decide(decisionSpec(t, allGroupsPassing(t))); err != nil {
		t.Fatalf("接受：%v", err)
	}
	return request
}

func goodsVersion(t *testing.T, id, prior string, intent domain.AmendmentIntent, goods domain.GoodsDetails) domain.CustomerSourceDataVersion {
	t.Helper()
	spec := amendmentSpec(t)
	spec.VersionID = mustValue(t, domain.NewSourceDataVersionID, id)
	spec.Scope = goodsScope(t)
	spec.Intent = intent
	spec.Basis = domain.NewSupplementOnAcceptanceBaseline()
	if prior != "" {
		basis, err := domain.NewAmendmentOfVersion(mustValue(t, domain.NewSourceDataVersionID, prior))
		if err != nil {
			t.Fatalf("基准：%v", err)
		}
		spec.Basis = basis
	}
	if goods.Declared() {
		content, err := domain.NewGoodsDetailsContent(goods)
		if err != nil {
			t.Fatalf("货物明细内容：%v", err)
		}
		spec.Content = content
	}
	version, err := domain.FormCustomerSourceDataVersion(spec)
	if err != nil {
		t.Fatalf("形成版本 %s：%v", id, err)
	}
	return version
}

func descriptionsOf(goods domain.GoodsDetails) []string {
	var texts []string
	for _, line := range goods.Lines() {
		texts = append(texts, line.Descriptions()[0].Text().Value())
	}
	return texts
}

// Covers: AT-PS-185 前半——客户以更正交来完整的新货物明细，删掉一行、改了另一行的品名、相应改了装箱分配：形成新版本，内件
// 性质、行与装箱分配整组替换，行号按新版本从 1 重编；接受基线那一版上的货物明细一格不动。
func TestACorrectionReplacesTheGoodsDetailsAsAWholeAndRenumbersTheLines(t *testing.T) {
	original := goodsOf(t, [][2]string{{"T-shirt", "48"}, {"Cap", "2"}, {"Socks", "10"}},
		[3]string{"parcel-1", "1", "24"}, [3]string{"parcel-1", "2", "2"}, [3]string{"parcel-2", "1", "24"}, [3]string{"parcel-2", "3", "10"})
	request := acceptedWithGoods(t, original)

	corrected := goodsOf(t, [][2]string{{"Cotton T-shirt", "48"}, {"Socks", "10"}},
		[3]string{"parcel-1", "1", "24"}, [3]string{"parcel-2", "1", "24"}, [3]string{"parcel-2", "2", "10"})
	request, err := request.AmendCustomerSourceData(goodsVersion(t, "goods-v1", "", domain.SupplementIntent, corrected))
	if err != nil {
		t.Fatalf("修订：%v", err)
	}

	inEffect, standing := request.GoodsDetailsInEffect()
	if standing != domain.GoodsDetailsOnAdoptedVersion {
		t.Fatalf("standing = %v, want ON_ADOPTED_VERSION", standing)
	}
	if got := descriptionsOf(inEffect); len(got) != 2 || got[0] != "Cotton T-shirt" || got[1] != "Socks" {
		t.Fatalf("在用的行 = %v, want [Cotton T-shirt Socks]", got)
	}
	for index, line := range inEffect.Lines() {
		if line.Number() != index+1 {
			t.Fatalf("第 %d 行的行号 = %d：行号按新版本从 1 重编", index+1, line.Number())
		}
	}
	baseline, _ := request.AcceptanceBaseline()
	current := request.CurrentSubmissionVersion()
	if current.VersionID() != baseline.SubmissionVersionID() {
		t.Fatalf("接受后当前提交版本 %s 不是接受基线那一版 %s", current.VersionID(), baseline.SubmissionVersionID())
	}
	if got := descriptionsOf(current.Goods()); len(got) != 3 || got[1] != "Cap" {
		t.Fatalf("接受基线那一版的货物明细被动了：%v", got)
	}
}

// Covers: AT-PS-185 后半——另一请求只交改动的那一行：按整组读，新货物明细只剩那一行，不按行与旧版本合并。
func TestAnAmendmentCarryingOneLineIsReadAsTheWholeGroup(t *testing.T) {
	request := acceptedWithGoods(t, goodsOf(t, [][2]string{{"T-shirt", "48"}, {"Cap", "2"}}))
	request, err := request.AmendCustomerSourceData(goodsVersion(t, "goods-v1", "", domain.SupplementIntent,
		goodsOf(t, [][2]string{{"Cap", "3"}})))
	if err != nil {
		t.Fatalf("修订：%v", err)
	}
	inEffect, _ := request.GoodsDetailsInEffect()
	if got := descriptionsOf(inEffect); len(got) != 1 || got[0] != "Cap" {
		t.Fatalf("在用的行 = %v, want 只剩 [Cap]", got)
	}
}

// Covers: ADR-0373 决定四「增删行或改分配不是增删委托成员」——装箱分配只能指接受基线上的成员：指到基线外答同一个业务拒绝
// （与范围越过基线同格），漏了成员是内容自己没写全。
func TestAGoodsAmendmentsPackingIsBoundByTheAcceptanceBaseline(t *testing.T) {
	request := acceptedWithGoods(t, domain.GoodsDetails{})
	outside := goodsOf(t, [][2]string{{"T-shirt", "3"}},
		[3]string{"parcel-1", "1", "1"}, [3]string{"parcel-2", "1", "1"}, [3]string{"parcel-9", "1", "1"})
	if _, err := request.AmendCustomerSourceData(goodsVersion(t, "goods-v1", "", domain.SupplementIntent, outside)); !errors.Is(err, domain.ErrParcelOutsideAcceptanceBaseline) {
		t.Fatalf("指到基线外：err = %v, want ErrParcelOutsideAcceptanceBaseline", err)
	}
	partial := goodsOf(t, [][2]string{{"T-shirt", "3"}}, [3]string{"parcel-1", "1", "3"})
	if _, err := request.AmendCustomerSourceData(goodsVersion(t, "goods-v1", "", domain.SupplementIntent, partial)); !errors.Is(err, domain.ErrInvalidPacking) {
		t.Fatalf("漏了成员：err = %v, want ErrInvalidPacking", err)
	}
}

// Covers: 资料组「货物明细」作用于整份委托、补充与更正交来完整的新货物明细、显式清空清掉整组——内容与范围、意图不配的组合
// 在形成版本之前就答不配。
func TestGoodsDetailsContentFitsOnlyTheShipmentScopedGoodsGroup(t *testing.T) {
	goods, err := domain.NewGoodsDetailsContent(goodsOf(t, [][2]string{{"T-shirt", "1"}}))
	if err != nil {
		t.Fatalf("货物明细内容：%v", err)
	}
	parcelScoped, err := domain.NewParcelScopedSourceData(mustValue(t, domain.NewShipmentRequestID, "request-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"), domain.GoodsDetailsDataGroup())
	if err != nil {
		t.Fatalf("按包裹的货物明细范围：%v", err)
	}
	cases := map[string]struct {
		scope   domain.SourceDataScope
		intent  domain.AmendmentIntent
		content domain.SourceDataVersionContent
		fits    bool
	}{
		"更正带整组":    {goodsScope(t), domain.CorrectionIntent, goods, true},
		"补充带整组":    {goodsScope(t), domain.SupplementIntent, goods, true},
		"更正不带内容":   {goodsScope(t), domain.CorrectionIntent, domain.SourceDataVersionContent{}, false},
		"显式清空不带内容": {goodsScope(t), domain.ExplicitClearIntent, domain.SourceDataVersionContent{}, true},
		"显式清空带内容":  {goodsScope(t), domain.ExplicitClearIntent, goods, false},
		"按包裹指":     {parcelScoped, domain.CorrectionIntent, goods, false},
		"落在测量范围上":  {measurementScope(t, "request-1", "parcel-1"), domain.CorrectionIntent, goods, false},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			if got := item.content.FitsAmendment(item.scope, item.intent); got != item.fits {
				t.Fatalf("FitsAmendment = %v, want %v", got, item.fits)
			}
		})
	}
	if _, err := domain.NewGoodsDetailsContent(domain.GoodsDetails{}); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Fatalf("没报的货物明细立成了内容：%v", err)
	}
}

// Covers: 在用的货物明细跟着采用判断走——没修订是提交时那一份；显式清空被采用后如实缺席，不回退；两条修订分叉时说不出该按
// 哪一版，不交货物明细。
func TestTheGoodsDetailsInEffectFollowTheAdoptionJudgment(t *testing.T) {
	submittedGoods := goodsOf(t, [][2]string{{"T-shirt", "48"}})
	request := acceptedWithGoods(t, submittedGoods)
	if goods, standing := request.GoodsDetailsInEffect(); standing != domain.GoodsDetailsOnSubmission || len(goods.Lines()) != 1 {
		t.Fatalf("没修订：standing = %v, %d 行", standing, len(goods.Lines()))
	}

	cleared, err := request.AmendCustomerSourceData(goodsVersion(t, "goods-v1", "", domain.ExplicitClearIntent, domain.GoodsDetails{}))
	if err != nil {
		t.Fatalf("清空：%v", err)
	}
	if goods, standing := cleared.GoodsDetailsInEffect(); standing != domain.GoodsDetailsOnAdoptedVersion || goods.Declared() {
		t.Fatalf("清空后：standing = %v, declared = %v；清空要如实缺席", standing, goods.Declared())
	}

	// 两位客户代表各自基于 goods-v1 改：后到的那一份基准已过期，分叉停在待复核。
	forked := request
	for _, version := range []domain.CustomerSourceDataVersion{
		goodsVersion(t, "goods-v1", "", domain.SupplementIntent, goodsOf(t, [][2]string{{"A", "1"}})),
		goodsVersion(t, "goods-v2", "goods-v1", domain.CorrectionIntent, goodsOf(t, [][2]string{{"B", "1"}})),
		goodsVersion(t, "goods-v3", "goods-v1", domain.CorrectionIntent, goodsOf(t, [][2]string{{"C", "1"}})),
	} {
		if forked, err = forked.AmendCustomerSourceData(version); err != nil {
			t.Fatalf("修订：%v", err)
		}
	}
	if goods, standing := forked.GoodsDetailsInEffect(); standing != domain.GoodsDetailsUndetermined || goods.Declared() {
		t.Fatalf("分叉：standing = %v, declared = %v", standing, goods.Declared())
	}
}
