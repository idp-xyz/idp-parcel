package domain_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// prePlatformDigest 是本切片动手之前、用当时未改动的代码对 payloadSpec 算出的摘要（基线 f55370512）。它是独立的事实来源：
// 货物明细与平台号以可缺键加进 PSC-1（ADR-0373 决定八，前提照 ADR-0267 决定三），没报这两样的载荷必须逐字节同前。
const prePlatformDigest = "PSC-1:8f169235481bf62cbe908b1e28a1443368b700d9d9130b83f487f8099ca77407"

// Covers: ADR-0373 决定八——同号改形不动旧载荷：没报货物明细与平台号的摘要与改动前相同，已存摘要与重放照旧成立。
func TestPayloadsWithoutGoodsOrPlatformKeepTheirDigest(t *testing.T) {
	if digest := canonicalize(t, payloadSpec(t)); digest.String() != prePlatformDigest {
		t.Fatalf("digest = %q, want %q; 旧载荷的编码变了", digest, prePlatformDigest)
	}
}

func goodsFor(t *testing.T, spec domain.SubmissionPayloadSpec, goods domain.GoodsDetailsSpec) domain.SubmissionPayloadSpec {
	t.Helper()
	spec.Goods = newGoodsDetails(t, goods, spec.DeclaredParcelIDs)
	return spec
}

func twoLineGoods() domain.GoodsDetailsSpec {
	watch := domain.GoodsLineSpec{Descriptions: []domain.GoodsDescriptionSpec{{Language: "en", Text: declared("Wrist watch")}}, Quantity: declared("1")}
	return domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{tShirtLine(), watch}}
}

// Covers: ADR-0373 决定六、八——货物明细进摘要；品名语言、编码口径与装箱分配是集合，给入顺序不是内容，行的顺序是内容（行号随它）。
func TestGoodsDetailsEnterTheDigest(t *testing.T) {
	base := canonicalize(t, payloadSpec(t))
	withGoods := canonicalize(t, goodsFor(t, payloadSpec(t), twoLineGoods()))
	if withGoods == base {
		t.Fatal("goods details did not change the digest")
	}
	if again := canonicalize(t, goodsFor(t, payloadSpec(t), twoLineGoods())); again != withGoods {
		t.Fatalf("digest = %q vs %q; 同一份货物明细两次算出不同摘要", again, withGoods)
	}

	reordered := twoLineGoods()
	reordered.Lines[0].Descriptions = []domain.GoodsDescriptionSpec{reordered.Lines[0].Descriptions[1], reordered.Lines[0].Descriptions[0]}
	reordered.Lines[0].CommodityCodes = append(reordered.Lines[0].CommodityCodes,
		domain.CommodityCodeSpec{Side: domain.CommodityCodeImportSide, Code: declared("6109100012")})
	sameAgain := twoLineGoods()
	sameAgain.Lines[0].CommodityCodes = append([]domain.CommodityCodeSpec{{Side: domain.CommodityCodeImportSide, Code: declared("6109100012")}},
		sameAgain.Lines[0].CommodityCodes...)
	if canonicalize(t, goodsFor(t, payloadSpec(t), reordered)) != canonicalize(t, goodsFor(t, payloadSpec(t), sameAgain)) {
		t.Fatal("description or code order changed the digest")
	}

	swapped := twoLineGoods()
	swapped.Lines[0], swapped.Lines[1] = swapped.Lines[1], swapped.Lines[0]
	if canonicalize(t, goodsFor(t, payloadSpec(t), swapped)) == withGoods {
		t.Fatal("line order did not change the digest; 行号是内容")
	}

	remarked := twoLineGoods()
	remarked.Lines[1].Quantity.Mark = domain.SourceMarkCommodityDeclaration
	if canonicalize(t, goodsFor(t, payloadSpec(t), remarked)) == withGoods {
		t.Fatal("source mark did not change the digest; 谁说的这一格也是内容")
	}

	packed := twoLineGoods()
	packed.Packing = []domain.PackingAllocationSpec{pack(t, "parcel-1", 1, "5"), pack(t, "parcel-2", 2, "1")}
	repacked := twoLineGoods()
	repacked.Packing = []domain.PackingAllocationSpec{pack(t, "parcel-2", 2, "1"), pack(t, "parcel-1", 1, "5")}
	packedDigest := canonicalize(t, goodsFor(t, payloadSpec(t), packed))
	if packedDigest == withGoods || packedDigest != canonicalize(t, goodsFor(t, payloadSpec(t), repacked)) {
		t.Fatal("packing must enter the digest and its order must not")
	}
}

func platformShipment(t *testing.T, shipment, reference string) domain.PlatformShipmentDeclaration {
	t.Helper()
	declaration, err := domain.DeclarePlatformShipment("AMAZON", mustValue(t, domain.NewExternalIdentifierNumber, shipment), reference)
	if err != nil {
		t.Fatalf("declare platform shipment: %v", err)
	}
	return declaration
}

func boxLabel(t *testing.T, parcel, number string) domain.PlatformBoxLabelDeclaration {
	t.Helper()
	declaration, err := domain.DeclarePlatformBoxLabel(mustValue(t, domain.NewDeclaredParcelID, parcel), mustValue(t, domain.NewExternalIdentifierNumber, number))
	if err != nil {
		t.Fatalf("declare platform box label: %v", err)
	}
	return declaration
}

// Covers: ADR-0374 决定二、四——平台货件号、平台参考号与每箱的平台箱标号进摘要；箱标号要指成员、一箱至多一个、要有平台货件。
func TestPlatformIdentifiersEnterTheDigest(t *testing.T) {
	base := canonicalize(t, payloadSpec(t))
	spec := payloadSpec(t)
	spec.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "SYN-REF-1")
	shipmentOnly := canonicalize(t, spec)
	if shipmentOnly == base {
		t.Fatal("platform shipment did not change the digest")
	}
	spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "parcel-2", "FBA15SYNTESTU000002"), boxLabel(t, "parcel-1", "FBA15SYNTESTU000001")}
	labelled := canonicalize(t, spec)
	if labelled == shipmentOnly {
		t.Fatal("box labels did not change the digest")
	}
	spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{spec.PlatformBoxLabels[1], spec.PlatformBoxLabels[0]}
	if canonicalize(t, spec) != labelled {
		t.Fatal("box label order changed the digest; 箱标号随成员，成员是集合")
	}

	refused := map[string]func(spec *domain.SubmissionPayloadSpec){
		"label for a parcel outside the request": func(spec *domain.SubmissionPayloadSpec) {
			spec.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "")
			spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "parcel-9", "FBA15SYNTESTU000009")}
		},
		"two labels on one parcel": func(spec *domain.SubmissionPayloadSpec) {
			spec.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "")
			spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "parcel-1", "A"), boxLabel(t, "parcel-1", "B")}
		},
		"labels without a platform shipment": func(spec *domain.SubmissionPayloadSpec) {
			spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "parcel-1", "FBA15SYNTESTU000001")}
		},
		"packing that names a parcel outside the request": func(spec *domain.SubmissionPayloadSpec) {
			goods := twoLineGoods()
			goods.Packing = []domain.PackingAllocationSpec{pack(t, "parcel-1", 1, "5"), pack(t, "parcel-9", 2, "1")}
			spec.Goods = newGoodsDetails(t, goods, members(t, "parcel-1", "parcel-9"))
		},
	}
	for name, mutate := range refused {
		spec := payloadSpec(t)
		mutate(&spec)
		if _, err := domain.CanonicalizeSubmissionPayload(spec); !errors.Is(err, domain.ErrInvalidSubmissionPayload) {
			t.Errorf("%s: err = %v, want ErrInvalidSubmissionPayload", name, err)
		}
	}
	if _, err := domain.DeclarePlatformShipment("", mustValue(t, domain.NewExternalIdentifierNumber, "FBA15SYNTEST"), ""); err == nil {
		t.Error("platform shipment without a platform was accepted; 分配方是销售平台，要指名")
	}
}

// Covers: 代码评审——装箱分配「每个包裹都要有」对着这份提交的成员集合核：货物明细按较少的成员造出来，摘要也不收。
// 形状错同时是货物明细的错，接入口据此答货物明细这一格。
func TestPackingIsCheckedAgainstTheRequestMembersWhenDigesting(t *testing.T) {
	goods := twoLineGoods()
	goods.Packing = []domain.PackingAllocationSpec{pack(t, "parcel-1", 1, "5"), pack(t, "parcel-1", 2, "1")}
	spec := payloadSpec(t) // 成员 parcel-1、parcel-2
	spec.Goods = newGoodsDetails(t, goods, members(t, "parcel-1"))
	_, err := domain.CanonicalizeSubmissionPayload(spec)
	if !errors.Is(err, domain.ErrInvalidSubmissionPayload) || !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Fatalf("err = %v, want both ErrInvalidSubmissionPayload and ErrInvalidGoodsDetails; parcel-2 没有分配", err)
	}
}

// Covers: 代码评审——销售平台名带 NUL 立不起来。
func TestAPlatformNameCarryingNULIsRefused(t *testing.T) {
	if _, err := domain.DeclarePlatformShipment("AMA\x00ZON", mustValue(t, domain.NewExternalIdentifierNumber, "FBA15SYNTEST"), ""); !errors.Is(err, domain.ErrInvalidPlatformIdentifier) {
		t.Fatalf("err = %v, want ErrInvalidPlatformIdentifier", err)
	}
}

// Covers: 代码评审复审——平台号的入册门不能被绕过：重建来的号超过字节界照样拒；销售平台名要是库存得下的文本、不超过同一字节界；
// 同一委托里两箱报同一个箱标号拒——平台给每箱唯一的箱标（ADR-0374 决定四）。
func TestPlatformIdentifierGatesCannotBeBypassed(t *testing.T) {
	long := strings.Repeat("A", 600)
	rehydrated, err := domain.RehydrateExternalIdentifierNumber(long)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if _, err := domain.DeclarePlatformShipment("AMAZON", rehydrated, ""); !errors.Is(err, domain.ErrInvalidPlatformIdentifier) {
		t.Errorf("rehydrated long shipment number: err = %v", err)
	}
	if _, err := domain.DeclarePlatformBoxLabel(mustValue(t, domain.NewDeclaredParcelID, "parcel-1"), rehydrated); !errors.Is(err, domain.ErrInvalidPlatformIdentifier) {
		t.Errorf("rehydrated long box label: err = %v", err)
	}
	if _, err := domain.DeclarePlatformShipment(long, mustValue(t, domain.NewExternalIdentifierNumber, "FBA15SYNTEST"), ""); !errors.Is(err, domain.ErrInvalidPlatformIdentifier) {
		t.Errorf("platform name over the byte limit: err = %v", err)
	}
	if _, err := domain.DeclarePlatformShipment("AMA\xffZON", mustValue(t, domain.NewExternalIdentifierNumber, "FBA15SYNTEST"), ""); !errors.Is(err, domain.ErrInvalidPlatformIdentifier) {
		t.Errorf("platform name with invalid UTF-8: err = %v", err)
	}

	spec := payloadSpec(t)
	spec.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "")
	spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "parcel-1", "FBA15SYNTESTU000001"), boxLabel(t, "parcel-2", "FBA15SYNTESTU000001")}
	if _, err := domain.CanonicalizeSubmissionPayload(spec); !errors.Is(err, domain.ErrInvalidSubmissionPayload) {
		t.Errorf("two boxes with one label: err = %v, want ErrInvalidSubmissionPayload", err)
	}
}
