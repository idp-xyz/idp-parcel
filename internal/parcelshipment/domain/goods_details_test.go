package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// declared 造一格客户渠道交来的在场申报格：客户渠道交来的一律是本单申报（PS CONTEXT「申报格来源标记」）。
func declared(value string) domain.GoodsCellSpec {
	return domain.GoodsCellSpec{Value: value, Mark: domain.SourceMarkThisDeclaration}
}

func members(t *testing.T, references ...string) []domain.DeclaredParcelID {
	t.Helper()
	parcels := make([]domain.DeclaredParcelID, 0, len(references))
	for _, reference := range references {
		parcels = append(parcels, mustValue(t, domain.NewDeclaredParcelID, reference))
	}
	return parcels
}

func tShirtLine() domain.GoodsLineSpec {
	return domain.GoodsLineSpec{
		Descriptions: []domain.GoodsDescriptionSpec{
			{Language: "en", Text: declared("Men's cotton T-shirt")},
			{Language: "zh", Text: declared("男式棉质T恤")},
		},
		Quantity:  declared("5"),
		Unit:      declared("PCS"),
		UnitValue: declared("3.20"),
		Currency:  declared("USD"),
		CommodityCodes: []domain.CommodityCodeSpec{
			{Side: domain.CommodityCodeExportSide, Code: declared("6109100000")},
		},
		OriginCountry: declared("CN"),
		OwnerSKU:      declared("SKU-TEE-1"),
		SalesOrder: domain.SalesOrderReferenceSpec{
			Platform:    declared("SHOPIFY"),
			OrderNumber: declared("1001"),
			OrderLine:   declared("1"),
		},
	}
}

func newGoodsDetails(t *testing.T, spec domain.GoodsDetailsSpec, parcels []domain.DeclaredParcelID) domain.GoodsDetails {
	t.Helper()
	details, err := domain.NewGoodsDetails(spec, parcels)
	if err != nil {
		t.Fatalf("new goods details: %v", err)
	}
	return details
}

// Covers: ADR-0373 决定二、六——货物明细行在场的格原样保全，连同各自的申报格来源标记；`AT-PS-181` 的领域半边。
func TestGoodsDetailsKeepEveryDeclaredCellVerbatim(t *testing.T) {
	details := newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{tShirtLine()}}, members(t, "box-1"))

	lines := details.Lines()
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(lines))
	}
	line := lines[0]
	if line.Number() != 1 {
		t.Fatalf("line number = %d, want 1; 行号按声明顺序从 1 编起", line.Number())
	}
	descriptions := line.Descriptions()
	if len(descriptions) != 2 || descriptions[1].Language() != "zh" || descriptions[1].Text().Value() != "男式棉质T恤" {
		t.Fatalf("descriptions = %+v; 品名按语言多条、原样保全", descriptions)
	}
	if quantity, present := line.Quantity(); !present || quantity.Value() != "5" || quantity.Mark() != domain.SourceMarkThisDeclaration {
		t.Fatalf("quantity = %+v present=%v", quantity, present)
	}
	if _, present := line.LineValue(); present {
		t.Fatal("line value present; 客户没报的格不推导、照实缺席")
	}
	codes := line.CommodityCodes()
	if len(codes) != 1 || codes[0].Side() != domain.CommodityCodeExportSide || codes[0].Code().Value() != "6109100000" {
		t.Fatalf("commodity codes = %+v; 编码位数原样", codes)
	}
	order, present := line.SalesOrder()
	if !present {
		t.Fatal("sales order reference missing")
	}
	if number, _ := order.OrderNumber(); number.Value() != "1001" {
		t.Fatalf("sales order number = %q", number.Value())
	}
}

// Covers: ADR-0373 决定二——一份委托 1 至 999 行，上限是产品机制。
func TestGoodsDetailsHoldBetweenOneAndNineHundredNinetyNineLines(t *testing.T) {
	parcels := members(t, "box-1")
	if _, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{}, parcels); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Fatalf("zero lines err = %v, want ErrInvalidGoodsDetails", err)
	}

	lines := make([]domain.GoodsLineSpec, 999)
	for index := range lines {
		lines[index] = tShirtLine()
	}
	newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: lines}, parcels)

	tooMany := append(lines, tShirtLine())
	if _, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{Lines: tooMany}, parcels); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Fatalf("1000 lines err = %v, want ErrInvalidGoodsDetails", err)
	}
}

// Covers: ADR-0373 决定六——一行一格都没有是形状错（`AT-PS-183`）。
func TestAGoodsLineWithoutAnyCellIsRefused(t *testing.T) {
	spec := domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{tShirtLine(), {}}}
	if _, err := domain.NewGoodsDetails(spec, members(t, "box-1")); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Fatalf("empty line err = %v, want ErrInvalidGoodsDetails", err)
	}
}

// Covers: ADR-0373 决定六——只认形状：数量与金额不是数、同一语言两条品名、同一口径两条编码、在场的格没有来源标记，
// 答形状不对（`AT-PS-183`）。
func TestGoodsLineShapeErrorsAreRefused(t *testing.T) {
	cases := map[string]func(line *domain.GoodsLineSpec){
		"quantity is not a number":   func(line *domain.GoodsLineSpec) { line.Quantity = declared("five") },
		"unit value is not a number": func(line *domain.GoodsLineSpec) { line.UnitValue = declared("3,20") },
		"line value is not a number": func(line *domain.GoodsLineSpec) { line.LineValue = declared("-16") },
		"net weight is not a number": func(line *domain.GoodsLineSpec) { line.NetWeight = declared("0.2kg") },
		"two descriptions in one lang": func(line *domain.GoodsLineSpec) {
			line.Descriptions = append(line.Descriptions, domain.GoodsDescriptionSpec{Language: "en", Text: declared("Tee")})
		},
		"description without language": func(line *domain.GoodsLineSpec) { line.Descriptions[0].Language = "" },
		"description without text":     func(line *domain.GoodsLineSpec) { line.Descriptions[0].Text = domain.GoodsCellSpec{} },
		"two export codes": func(line *domain.GoodsLineSpec) {
			line.CommodityCodes = append(line.CommodityCodes, domain.CommodityCodeSpec{Side: domain.CommodityCodeExportSide, Code: declared("610910")})
		},
		"code without side":  func(line *domain.GoodsLineSpec) { line.CommodityCodes[0].Side = domain.CommodityCodeSideInvalid },
		"code without value": func(line *domain.GoodsLineSpec) { line.CommodityCodes[0].Code = domain.GoodsCellSpec{} },
		"cell without mark":  func(line *domain.GoodsLineSpec) { line.OriginCountry = domain.GoodsCellSpec{Value: "CN"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			line := tShirtLine()
			mutate(&line)
			_, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{line}}, members(t, "box-1"))
			if !errors.Is(err, domain.ErrInvalidGoodsDetails) {
				t.Fatalf("err = %v, want ErrInvalidGoodsDetails", err)
			}
		})
	}
}

// Covers: ADR-0373 决定六——形状之外一格都不核：单价乘数量与行总价对不上、全是空白的值、编码位数与国家码花样都照收（`AT-PS-182`）。
func TestGoodsLinesAreNotCrossChecked(t *testing.T) {
	line := tShirtLine()
	line.LineValue = declared("12.00") // 5 × 3.20 = 16.00，平台折扣后对不上是常态
	line.OriginCountry = declared("  ")
	line.CommodityCodes = []domain.CommodityCodeSpec{
		{Side: domain.CommodityCodeExportSide, Code: declared("61")},
		{Side: domain.CommodityCodeImportSide, Code: declared("6109.10.0012")},
	}
	details := newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{line}}, members(t, "box-1"))

	kept := details.Lines()[0]
	if value, _ := kept.LineValue(); value.Value() != "12.00" {
		t.Fatalf("line value = %q; 不补算、不改写", value.Value())
	}
	if origin, present := kept.OriginCountry(); !present || origin.Value() != "  " {
		t.Fatalf("origin = %q present=%v; 全是空白的值原样在场", origin.Value(), present)
	}
}

// Covers: ADR-0373 决定二——内件性质封闭六格、委托一格、可缺；只有「其他」可带说明。
func TestContentCategoryIsClosedAndOnlyOtherCarriesANote(t *testing.T) {
	parcels := members(t, "box-1")
	lines := []domain.GoodsLineSpec{tShirtLine()}

	gift := newGoodsDetails(t, domain.GoodsDetailsSpec{Category: domain.ContentCategoryGift, CategoryMark: domain.SourceMarkThisDeclaration, Lines: lines}, parcels)
	if category, mark, present := gift.Category(); !present || category != domain.ContentCategoryGift || mark != domain.SourceMarkThisDeclaration {
		t.Fatalf("category = %v %v present=%v", category, mark, present)
	}
	other := newGoodsDetails(t, domain.GoodsDetailsSpec{Category: domain.ContentCategoryOther, CategoryMark: domain.SourceMarkThisDeclaration, CategoryNote: declared("spare parts for repair"), Lines: lines}, parcels)
	if note, present := other.CategoryNote(); !present || note.Value() != "spare parts for repair" {
		t.Fatalf("note = %q present=%v", note.Value(), present)
	}
	if _, _, present := newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: lines}, parcels).Category(); present {
		t.Fatal("category present; 没报就是缺席")
	}

	refused := map[string]domain.GoodsDetailsSpec{
		"unknown category":      {Category: domain.ContentCategory(99), CategoryMark: domain.SourceMarkThisDeclaration, Lines: lines},
		"category without mark": {Category: domain.ContentCategoryGift, Lines: lines},
		"note on a gift":        {Category: domain.ContentCategoryGift, CategoryMark: domain.SourceMarkThisDeclaration, CategoryNote: declared("birthday"), Lines: lines},
		"note without category": {CategoryNote: declared("birthday"), Lines: lines},
	}
	for name, spec := range refused {
		if _, err := domain.NewGoodsDetails(spec, parcels); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
			t.Errorf("%s: err = %v, want ErrInvalidGoodsDetails", name, err)
		}
	}
}

func pack(t *testing.T, parcel string, line int, quantity string) domain.PackingAllocationSpec {
	t.Helper()
	return domain.PackingAllocationSpec{Parcel: mustValue(t, domain.NewDeclaredParcelID, parcel), Line: line, Quantity: quantity}
}

// Covers: ADR-0373 决定三——单件委托不报分配；多件委托可以不报（整票清单，`AT-PS-186`），报了就收下逐箱的分配。
func TestPackingAllocationMayBeOmittedOrDeclaredInFull(t *testing.T) {
	watch := domain.GoodsLineSpec{Descriptions: []domain.GoodsDescriptionSpec{{Language: "en", Text: declared("Wrist watch")}}, Quantity: declared("1")}
	lines := []domain.GoodsLineSpec{tShirtLine(), watch}

	single := newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: lines}, members(t, "box-1"))
	if len(single.Packing()) != 0 {
		t.Fatalf("packing = %+v; 单件委托不报分配", single.Packing())
	}
	newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: lines}, members(t, "box-1", "box-2", "box-3"))

	full := newGoodsDetails(t, domain.GoodsDetailsSpec{
		Lines:   lines,
		Packing: []domain.PackingAllocationSpec{pack(t, "box-1", 1, "3"), pack(t, "box-2", 1, "2"), pack(t, "box-2", 2, "1")},
	}, members(t, "box-1", "box-2"))
	packing := full.Packing()
	if len(packing) != 3 || packing[2].Parcel().String() != "box-2" || packing[2].Line() != 2 || packing[2].Quantity() != "1" {
		t.Fatalf("packing = %+v", packing)
	}
}

// Covers: ADR-0373 决定三、ADR-0374 决定二——FBA 同款五十箱是一行加五十条分配，值只写一处（`AT-PS-188` 的领域半边）。
func TestFiftyIdenticalCartonsAreOneLineAndFiftyAllocations(t *testing.T) {
	line := tShirtLine()
	line.Quantity = declared("1200")
	references := make([]string, 0, 50)
	packing := make([]domain.PackingAllocationSpec, 0, 50)
	for carton := 1; carton <= 50; carton++ {
		reference := "FBA15TEST" + string(rune('A'+carton/26)) + string(rune('A'+carton%26))
		references = append(references, reference)
		packing = append(packing, pack(t, reference, 1, "24"))
	}
	details := newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{line}, Packing: packing}, members(t, references...))
	if len(details.Lines()) != 1 || len(details.Packing()) != 50 {
		t.Fatalf("lines = %d packing = %d", len(details.Lines()), len(details.Packing()))
	}
}

// Covers: ADR-0373 决定三——报了分配就要报全：只报一部分包裹、指成员外的包裹或不存在的行、某行之和对不上、该行数量缺席、
// 同一包裹同一行两条、分配数量不是正数，答形状不对（`AT-PS-183`）。
func TestPartialOrInconsistentPackingIsRefused(t *testing.T) {
	watch := domain.GoodsLineSpec{Descriptions: []domain.GoodsDescriptionSpec{{Language: "en", Text: declared("Wrist watch")}}, Quantity: declared("1")}
	noQuantity := domain.GoodsLineSpec{Descriptions: []domain.GoodsDescriptionSpec{{Language: "en", Text: declared("Socks")}}}
	parcels := members(t, "box-1", "box-2")
	lines := []domain.GoodsLineSpec{tShirtLine(), watch}

	refused := map[string]domain.GoodsDetailsSpec{
		"one parcel left out": {Lines: lines, Packing: []domain.PackingAllocationSpec{pack(t, "box-1", 1, "5"), pack(t, "box-1", 2, "1")}},
		"parcel outside the request": {Lines: lines, Packing: []domain.PackingAllocationSpec{
			pack(t, "box-1", 1, "5"), pack(t, "box-2", 2, "1"), pack(t, "box-9", 2, "1")}},
		"line that does not exist": {Lines: lines, Packing: []domain.PackingAllocationSpec{
			pack(t, "box-1", 1, "5"), pack(t, "box-2", 2, "1"), pack(t, "box-2", 3, "1")}},
		"sum short of the line": {Lines: lines, Packing: []domain.PackingAllocationSpec{pack(t, "box-1", 1, "4"), pack(t, "box-2", 2, "1")}},
		"line without quantity": {Lines: []domain.GoodsLineSpec{tShirtLine(), noQuantity}, Packing: []domain.PackingAllocationSpec{
			pack(t, "box-1", 1, "5"), pack(t, "box-2", 2, "1")}},
		"same parcel and line twice": {Lines: lines, Packing: []domain.PackingAllocationSpec{
			pack(t, "box-1", 1, "2"), pack(t, "box-1", 1, "3"), pack(t, "box-2", 2, "1")}},
		"zero allocation": {Lines: lines, Packing: []domain.PackingAllocationSpec{
			pack(t, "box-1", 1, "5"), pack(t, "box-2", 1, "0"), pack(t, "box-2", 2, "1")}},
		"allocation not a number": {Lines: lines, Packing: []domain.PackingAllocationSpec{pack(t, "box-1", 1, "5"), pack(t, "box-2", 2, "one")}},
		"allocation with trailing letters": {Lines: lines, Packing: []domain.PackingAllocationSpec{
			pack(t, "box-1", 1, "4"), pack(t, "box-2", 1, "1x"), pack(t, "box-2", 2, "1")}},
	}
	for name, spec := range refused {
		if _, err := domain.NewGoodsDetails(spec, parcels); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
			t.Errorf("%s: err = %v, want ErrInvalidGoodsDetails", name, err)
		}
	}
}

// Covers: ADR-0373 决定二——内件性质为「文件」时可以没有货物明细行，这时也不报装箱分配；其余情形至少一行。
func TestDocumentsMayCarryNoGoodsLines(t *testing.T) {
	parcels := members(t, "box-1")
	documents := newGoodsDetails(t, domain.GoodsDetailsSpec{Category: domain.ContentCategoryDocuments, CategoryMark: domain.SourceMarkThisDeclaration}, parcels)
	if !documents.Declared() || len(documents.Lines()) != 0 {
		t.Fatalf("declared = %v lines = %d; 文件件只报内件性质也算报了货物明细", documents.Declared(), len(documents.Lines()))
	}
	refused := map[string]domain.GoodsDetailsSpec{
		"gift without lines":          {Category: domain.ContentCategoryGift, CategoryMark: domain.SourceMarkThisDeclaration},
		"documents with packing only": {Category: domain.ContentCategoryDocuments, CategoryMark: domain.SourceMarkThisDeclaration, Packing: []domain.PackingAllocationSpec{pack(t, "box-1", 1, "1")}},
	}
	for name, spec := range refused {
		if _, err := domain.NewGoodsDetails(spec, parcels); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
			t.Errorf("%s: err = %v, want ErrInvalidGoodsDetails", name, err)
		}
	}
}

// Covers: 代码评审——没报内件性质却带了来源标记，读口答没报、摘要却不同，同一请求重发会被判冲突；拒掉。
func TestACategoryMarkWithoutACategoryIsRefused(t *testing.T) {
	spec := domain.GoodsDetailsSpec{CategoryMark: domain.SourceMarkBoxList, Lines: []domain.GoodsLineSpec{tShirtLine()}}
	if _, err := domain.NewGoodsDetails(spec, members(t, "box-1")); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Fatalf("err = %v, want ErrInvalidGoodsDetails", err)
	}
}

// Covers: 代码评审——Postgres 的 text 存不下 NUL；带它的格在构造时就答形状不对，不留到写库时答依赖故障。
func TestCellsCarryingNULAreRefused(t *testing.T) {
	cases := map[string]func(line *domain.GoodsLineSpec){
		"description text":     func(line *domain.GoodsLineSpec) { line.Descriptions[0].Text = declared("T-\x00shirt") },
		"description language": func(line *domain.GoodsLineSpec) { line.Descriptions[0].Language = "e\x00n" },
		"owner sku":            func(line *domain.GoodsLineSpec) { line.OwnerSKU = declared("SKU\x00") },
		"sales order number":   func(line *domain.GoodsLineSpec) { line.SalesOrder.OrderNumber = declared("10\x0001") },
	}
	for name, mutate := range cases {
		line := tShirtLine()
		mutate(&line)
		if _, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{line}}, members(t, "box-1")); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
			t.Errorf("%s: err = %v, want ErrInvalidGoodsDetails", name, err)
		}
	}
}

// Covers: 代码评审复审——非法 UTF-8 与 NUL 同一理由：库存不下，且 JSON 编码会把不同的坏字节编成同一个替换字符，让两份
// 不同的内容在摘要里相等。内件性质说明与销售订单各格同样要核。
func TestCellsThatAreNotStorableTextAreRefused(t *testing.T) {
	parcels := members(t, "box-1")
	line := tShirtLine()
	line.OwnerSKU = declared("SKU\xff")
	if _, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{line}}, parcels); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Errorf("invalid UTF-8 sku: err = %v, want ErrInvalidGoodsDetails", err)
	}
	line = tShirtLine()
	line.SalesOrder.Platform = declared("SHOP\x00IFY")
	if _, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{line}}, parcels); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Errorf("NUL in sales order platform: err = %v, want ErrInvalidGoodsDetails", err)
	}
	note := domain.GoodsDetailsSpec{Category: domain.ContentCategoryOther, CategoryMark: domain.SourceMarkThisDeclaration,
		CategoryNote: declared("spare\x00parts"), Lines: []domain.GoodsLineSpec{tShirtLine()}}
	if _, err := domain.NewGoodsDetails(note, parcels); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Errorf("NUL in category note: err = %v, want ErrInvalidGoodsDetails", err)
	}
}
