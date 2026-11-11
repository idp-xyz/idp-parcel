package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	ccparcelshipment "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/parcelshipment"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉关务读货物明细的消费侧翻译（ADR-0373 决定九，票 declared-goods-lines/07）：按（租户，资料版本锚，委托）取，内件
// 性质、行（逐格带来源标记）与装箱分配原样译成关务的记录，并交来接受基线的包裹；单件、多件不报分配、多件报全分配都取得到；
// 锚不存在与他租户同答不可见；分叉待复核时只说待复核、不交行。

// goodsViewDouble 按（租户，委托）登记一份货物明细，基线锚答 baseline，版本锚答 versions 里那一版；members 是接受基线的成员。
type goodsViewDouble struct {
	tenant   string
	request  string
	members  []psdomain.DeclaredParcelID
	baseline psdomain.GoodsDetails
	versions map[string]psdomain.GoodsDetails
	current  psdomain.GoodsDetailsResolution
	err      error
}

func (double *goodsViewDouble) LoadCurrentGoodsDetails(
	_ context.Context, tenant psdomain.TenantID, request psdomain.ShipmentRequestID,
) (psdomain.GoodsDetailsResolution, bool, error) {
	if double.err != nil {
		return psdomain.GoodsDetailsResolution{}, false, double.err
	}
	if tenant.String() != double.tenant || request.String() != double.request {
		return psdomain.GoodsDetailsResolution{}, false, nil
	}
	return double.current, true, nil
}

func (double *goodsViewDouble) LoadGoodsDetailsAt(
	_ context.Context, tenant psdomain.TenantID, request psdomain.ShipmentRequestID, anchor psdomain.SourceDataVersionAnchor,
) (psdomain.GoodsDetailsResolution, bool, error) {
	if tenant.String() != double.tenant || request.String() != double.request {
		return psdomain.GoodsDetailsResolution{}, false, nil
	}
	goods := double.baseline
	if !anchor.OnAcceptanceBaseline() {
		version, _ := anchor.AdoptedVersion()
		var found bool
		if goods, found = double.versions[version.String()]; !found {
			return psdomain.GoodsDetailsResolution{}, false, nil
		}
	}
	resolution, err := psdomain.GoodsDetailsAnchoredAt(anchor, goods, double.members)
	return resolution, err == nil, err
}

func membersOf(t *testing.T, parcels ...string) []psdomain.DeclaredParcelID {
	t.Helper()
	members := make([]psdomain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		members = append(members, must(t, psdomain.NewDeclaredParcelID, parcel))
	}
	return members
}

func psCell(value string, mark psdomain.DeclarationSourceMark) psdomain.GoodsCellSpec {
	return psdomain.GoodsCellSpec{Value: value, Mark: mark}
}

// twoLineGoods 是两行货：第一行英文品名取自箱单、单价取自商品申报资料、带出口编码与销售订单；packing 每项 [包裹, 行, 数量]。
func twoLineGoods(t *testing.T, packing ...[3]string) psdomain.GoodsDetails {
	t.Helper()
	this := psdomain.SourceMarkThisDeclaration
	spec := psdomain.GoodsDetailsSpec{
		Category: psdomain.ContentCategorySaleOfGoods, CategoryMark: this,
		Lines: []psdomain.GoodsLineSpec{
			{
				Descriptions:   []psdomain.GoodsDescriptionSpec{{Language: "en", Text: psCell("T-shirt", psdomain.SourceMarkBoxList)}},
				Quantity:       psCell("48", this),
				Unit:           psCell("PCS", this),
				UnitValue:      psCell("3.20", psdomain.SourceMarkCommodityDeclaration),
				Currency:       psCell("USD", this),
				CommodityCodes: []psdomain.CommodityCodeSpec{{Side: psdomain.CommodityCodeExportSide, Code: psCell("6109100000", this)}},
				SalesOrder:     psdomain.SalesOrderReferenceSpec{OrderNumber: psCell("SYN-ORDER-1", this)},
			},
			{Descriptions: []psdomain.GoodsDescriptionSpec{{Language: "en", Text: psCell("Cap", this)}}, Quantity: psCell("2", this)},
		},
	}
	var members []psdomain.DeclaredParcelID
	seen := map[string]bool{}
	for _, allocation := range packing {
		parcel := must(t, psdomain.NewDeclaredParcelID, allocation[0])
		line := 1
		if allocation[1] == "2" {
			line = 2
		}
		spec.Packing = append(spec.Packing, psdomain.PackingAllocationSpec{Parcel: parcel, Line: line, Quantity: allocation[2]})
		if !seen[allocation[0]] {
			seen[allocation[0]] = true
			members = append(members, parcel)
		}
	}
	goods, err := psdomain.NewGoodsDetails(spec, members)
	if err != nil {
		t.Fatalf("货物明细：%v", err)
	}
	return goods
}

func goodsAdapter(t *testing.T, double *goodsViewDouble) *ccparcelshipment.CustomerGoodsDetails {
	t.Helper()
	adapter, err := ccparcelshipment.NewCustomerGoodsDetails(double)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

var baselineAnchor = ccports.CustomerSourceDataAnchor{OnAcceptanceBaseline: true}

// Covers: 完成判据「单件、多件不报分配、多件报全分配三种都能取到」——行号、各格的值与出处、编码口径、销售订单与装箱分配原样
// 译过来，没报的格为 nil，不补也不并；接受基线的包裹一并交来，没报分配时关务靠它分得清单件（那一件装了全部行）与整票清单。
func TestCustomsReadsTheGoodsLinesOfAShipmentWithOrWithoutPacking(t *testing.T) {
	tenant := must(t, ccdomain.NewTenantID, "SYN-TENANT-01")
	for name, item := range map[string]struct {
		goods   psdomain.GoodsDetails
		members []string
		packing int
	}{
		"单件报分配":  {twoLineGoods(t, [3]string{"DPCL-1", "1", "48"}, [3]string{"DPCL-1", "2", "2"}), []string{"DPCL-1"}, 2},
		"单件不报分配": {twoLineGoods(t), []string{"DPCL-1"}, 0},
		"多件不报分配": {twoLineGoods(t), []string{"DPCL-1", "DPCL-2"}, 0},
		"多件报全分配": {twoLineGoods(t, [3]string{"DPCL-1", "1", "24"}, [3]string{"DPCL-2", "1", "24"}, [3]string{"DPCL-2", "2", "2"}),
			[]string{"DPCL-1", "DPCL-2"}, 3},
	} {
		t.Run(name, func(t *testing.T) {
			adapter := goodsAdapter(t, &goodsViewDouble{tenant: "SYN-TENANT-01", request: "SHR-SYN-1", baseline: item.goods,
				members: membersOf(t, item.members...)})
			details, found, err := adapter.FindCustomerGoodsDetails(context.Background(), tenant, "SHR-SYN-1", baselineAnchor)
			if err != nil || !found {
				t.Fatalf("found=%v err=%v", found, err)
			}
			if !details.Declared || details.ContentCategory != ccports.CustomerContentSaleOfGoods || details.Anchor != baselineAnchor ||
				len(details.Packing) != item.packing || len(details.Parcels) != len(item.members) {
				t.Fatalf("货物明细 = %+v", details)
			}
			first := details.Lines[0]
			if first.Line != 1 || first.Descriptions[0].Text.Source != ccports.CustomerGoodsCellFromBoxList ||
				first.UnitValue == nil || first.UnitValue.Value != "3.20" || first.UnitValue.Source != ccports.CustomerGoodsCellFromCommodityDeclaration ||
				first.Quantity.Value != "48" || first.Quantity.Source != ccports.CustomerGoodsCellFromThisDeclaration ||
				first.CommodityCodes[0].Side != ccports.CustomerCommodityCodeExport || first.SalesOrder.OrderNumber.Value != "SYN-ORDER-1" ||
				first.SalesOrder.Platform != nil {
				t.Fatalf("第 1 行 = %+v", first)
			}
			if second := details.Lines[1]; second.Line != 2 || second.LineValue != nil || second.Unit != nil {
				t.Fatalf("第 2 行没报的格要为 nil：%+v", second)
			}
		})
	}
}

// Covers: 完成判据「锚不存在与跨租户答不可见」——指名一版不在这份委托货物明细范围上的锚、他租户、委托号立不起来、锚两格同在或
// 同缺，一律 found=false，不区分哪一种；读不动走 error。
func TestCustomsSeesNoGoodsForAMissingAnchorOrAnotherTenant(t *testing.T) {
	double := &goodsViewDouble{tenant: "SYN-TENANT-01", request: "SHR-SYN-1", baseline: twoLineGoods(t),
		versions: map[string]psdomain.GoodsDetails{"SDV-1": twoLineGoods(t)}}
	adapter := goodsAdapter(t, double)
	ctx := context.Background()
	tenant := must(t, ccdomain.NewTenantID, "SYN-TENANT-01")
	other := must(t, ccdomain.NewTenantID, "SYN-TENANT-02")
	at := func(tenant ccdomain.TenantID, request string, anchor ccports.CustomerSourceDataAnchor) func() (bool, error) {
		return func() (bool, error) {
			_, found, err := adapter.FindCustomerGoodsDetails(ctx, tenant, request, anchor)
			return found, err
		}
	}
	for name, call := range map[string]func() (bool, error){
		"锚指的版本不在": at(tenant, "SHR-SYN-1", ccports.CustomerSourceDataAnchor{SourceDataVersion: "SDV-9"}),
		"他租户":     at(other, "SHR-SYN-1", baselineAnchor),
		"委托号空白":   at(tenant, " ", baselineAnchor),
		"锚两格同在":   at(tenant, "SHR-SYN-1", ccports.CustomerSourceDataAnchor{OnAcceptanceBaseline: true, SourceDataVersion: "SDV-1"}),
		"锚两格同缺":   at(tenant, "SHR-SYN-1", ccports.CustomerSourceDataAnchor{}),
		"当前·他租户": func() (bool, error) {
			_, _, found, err := adapter.FindCurrentCustomerGoodsDetails(ctx, other, "SHR-SYN-1")
			return found, err
		},
	} {
		if found, err := call(); found || err != nil {
			t.Errorf("%s：found=%v err=%v, want 不可见", name, found, err)
		}
	}
	details, found, err := adapter.FindCustomerGoodsDetails(ctx, tenant, "SHR-SYN-1", ccports.CustomerSourceDataAnchor{SourceDataVersion: "SDV-1"})
	if err != nil || !found || details.Anchor.SourceDataVersion != "SDV-1" {
		t.Fatalf("版本锚读回：found=%v err=%v anchor=%+v", found, err, details.Anchor)
	}

	broken := goodsAdapter(t, &goodsViewDouble{err: errors.New("读不动")})
	if _, _, _, err := broken.FindCurrentCustomerGoodsDetails(ctx, tenant, "SHR-SYN-1"); err == nil {
		t.Fatal("读不动没交 error")
	}
}

// Covers: 「当前」一问答此刻的锚与那一版；分叉待复核时只说待复核，不交行——关务不该挑一版形成；既没锚又不是待复核是提供方
// 的坏答复，答读不动。
func TestCustomsReadsTheCurrentAnchorAndStopsOnAnUndeterminedFork(t *testing.T) {
	tenant := must(t, ccdomain.NewTenantID, "SYN-TENANT-01")
	versionAnchor, err := psdomain.NewSourceDataVersionAnchor(must(t, psdomain.NewSourceDataVersionID, "SDV-2"))
	if err != nil {
		t.Fatal(err)
	}
	anchored, err := psdomain.GoodsDetailsAnchoredAt(versionAnchor, twoLineGoods(t), membersOf(t, "DPCL-1"))
	if err != nil {
		t.Fatal(err)
	}
	current := goodsAdapter(t, &goodsViewDouble{tenant: "SYN-TENANT-01", request: "SHR-SYN-1", current: anchored})
	details, held, found, err := current.FindCurrentCustomerGoodsDetails(context.Background(), tenant, "SHR-SYN-1")
	if err != nil || !found || held || details.Anchor.SourceDataVersion != "SDV-2" || len(details.Lines) != 2 {
		t.Fatalf("当前：found=%v held=%v err=%v anchor=%+v", found, held, err, details.Anchor)
	}

	forked := goodsAdapter(t, &goodsViewDouble{tenant: "SYN-TENANT-01", request: "SHR-SYN-1",
		current: psdomain.GoodsDetailsUndeterminedResolution(membersOf(t, "DPCL-1"))})
	details, held, found, err = forked.FindCurrentCustomerGoodsDetails(context.Background(), tenant, "SHR-SYN-1")
	if err != nil || !found || !held || details.Declared || len(details.Lines) != 0 || len(details.Parcels) != 1 {
		t.Fatalf("分叉：found=%v held=%v err=%v details=%+v, want 待复核、不交行、交包裹", found, held, err, details)
	}

	broken := goodsAdapter(t, &goodsViewDouble{tenant: "SYN-TENANT-01", request: "SHR-SYN-1"})
	if _, _, found, err := broken.FindCurrentCustomerGoodsDetails(context.Background(), tenant, "SHR-SYN-1"); err == nil || found {
		t.Fatalf("没锚的答复：found=%v err=%v, want error", found, err)
	}
}
