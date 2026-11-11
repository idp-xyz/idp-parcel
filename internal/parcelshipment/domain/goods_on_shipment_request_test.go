package domain_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// packedTwoLineGoods 是成员 parcel-1、parcel-2 上报全了装箱分配的两行货物。
func packedTwoLineGoods(t *testing.T) domain.GoodsDetailsSpec {
	t.Helper()
	goods := twoLineGoods()
	goods.Packing = []domain.PackingAllocationSpec{pack(t, "parcel-1", 1, "5"), pack(t, "parcel-2", 2, "1")}
	return goods
}

func withGoodsAndPlatform(t *testing.T, spec domain.SubmitShipmentRequestSpec) domain.SubmitShipmentRequestSpec {
	t.Helper()
	// 货物明细按 parcel-1、parcel-2 造：候选成员多出一件时，聚合要自己认出分配漏了它。
	spec.Goods = newGoodsDetails(t, packedTwoLineGoods(t), members(t, "parcel-1", "parcel-2"))
	spec.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "SYN-REF-1")
	spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{
		boxLabel(t, "parcel-1", "FBA15SYNTESTU000001"), boxLabel(t, "parcel-2", "FBA15SYNTESTU000002"),
	}
	return spec
}

// Covers: ADR-0373 决定五、ADR-0374 决定四——货物明细随提交版本保留；平台货件与箱标号挂在委托上（`AT-PS-188` 的聚合半边）。
func TestASubmittedRequestKeepsItsGoodsDetailsAndPlatformDeclarations(t *testing.T) {
	request, err := domain.SubmitShipmentRequest(withGoodsAndPlatform(t, submitSpec(t, "parcel-1", "parcel-2")))
	if err != nil {
		t.Fatalf("submit shipment request: %v", err)
	}
	goods := request.CurrentSubmissionVersion().Goods()
	if len(goods.Lines()) != 2 || len(goods.Packing()) != 2 {
		t.Fatalf("goods = %d lines, %d packing; 货物明细要随提交版本保留", len(goods.Lines()), len(goods.Packing()))
	}
	shipment, declared := request.PlatformShipment()
	if !declared || shipment.ShipmentNumber().String() != "FBA15SYNTEST" {
		t.Fatalf("platform shipment = %+v declared=%v", shipment, declared)
	}
	if labels := request.PlatformBoxLabels(); len(labels) != 2 || labels[1].Number().String() != "FBA15SYNTESTU000002" {
		t.Fatalf("box labels = %+v", labels)
	}
}

// Covers: ADR-0373 决定三、ADR-0374 决定四——聚合对着候选的成员集合再核：装箱分配漏了成员、箱标号指成员外、有箱标号没平台货件，都不建单。
func TestSubmitRefusesGoodsOrBoxLabelsThatDoNotFitTheMembers(t *testing.T) {
	short := withGoodsAndPlatform(t, submitSpec(t, "parcel-1", "parcel-2", "parcel-3"))
	if _, err := domain.SubmitShipmentRequest(short); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Errorf("packing leaves parcel-3 out: err = %v, want ErrInvalidGoodsDetails", err)
	}

	outside := withGoodsAndPlatform(t, submitSpec(t, "parcel-1", "parcel-2"))
	outside.PlatformBoxLabels = append(outside.PlatformBoxLabels, boxLabel(t, "parcel-9", "FBA15SYNTESTU000009"))
	if _, err := domain.SubmitShipmentRequest(outside); !errors.Is(err, domain.ErrInvalidPlatformIdentifier) {
		t.Errorf("box label outside the request: err = %v, want ErrInvalidPlatformIdentifier", err)
	}

	unshipped := withGoodsAndPlatform(t, submitSpec(t, "parcel-1", "parcel-2"))
	unshipped.PlatformShipment = domain.PlatformShipmentDeclaration{}
	if _, err := domain.SubmitShipmentRequest(unshipped); !errors.Is(err, domain.ErrInvalidPlatformIdentifier) {
		t.Errorf("box labels without a platform shipment: err = %v, want ErrInvalidPlatformIdentifier", err)
	}
}

// Covers: ADR-0373 决定四、ADR-0376 决定二——补充形成的新版本整份重报货物明细，旧版本留着原来的；平台声明同样整份重报，
// 这一份补充没带平台段，委托上的平台声明随之撤下。
func TestANewSubmissionVersionCarriesItsOwnGoodsAndPlatformDeclarations(t *testing.T) {
	request, err := domain.SubmitShipmentRequest(withGoodsAndPlatform(t, submitSpec(t, "parcel-1", "parcel-2")))
	if err != nil {
		t.Fatalf("submit shipment request: %v", err)
	}
	next := supersessionSpecFor(t, "parcel-1", "parcel-2")
	oneLine := domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{tShirtLine()}}
	next.Goods = newGoodsDetails(t, oneLine, next.DeclaredParcelIDs)

	superseded, err := request.FormNewSubmissionVersion(next)
	if err != nil {
		t.Fatalf("form new submission version: %v", err)
	}
	if lines := superseded.CurrentSubmissionVersion().Goods().Lines(); len(lines) != 1 {
		t.Fatalf("current goods = %d lines, want 1; 新版本带它自己的货物明细", len(lines))
	}
	if prior := superseded.PriorSubmissionVersions(); len(prior) != 1 || len(prior[0].Goods().Lines()) != 2 {
		t.Fatal("prior version lost its goods; 旧版本不被覆盖")
	}
	if _, declared := superseded.PlatformShipment(); declared || len(superseded.PlatformBoxLabels()) != 0 {
		t.Fatal("补充没带平台段，委托上的平台声明却留着：补充整份重报，没带即这一版不报")
	}

	misfit := supersessionSpecFor(t, "parcel-1", "parcel-2")
	misfit.Goods = newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: twoLineGoods().Lines, Packing: []domain.PackingAllocationSpec{
		pack(t, "parcel-1", 1, "5"), pack(t, "parcel-1", 2, "1")}}, members(t, "parcel-1"))
	if _, err := request.FormNewSubmissionVersion(misfit); !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Errorf("new version packing leaves parcel-2 out: err = %v, want ErrInvalidGoodsDetails", err)
	}
}

// Covers: ADR-0028 重建——库里的货物明细与平台声明原样带回；漏带回，一份报过货的委托重建后看起来像没报过。
func TestRehydrationBringsBackGoodsAndPlatformDeclarations(t *testing.T) {
	snapshot := submittedSnapshot(t) // 成员只有 parcel-1
	snapshot.CurrentVersion.Goods = newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{tShirtLine()}},
		snapshot.CurrentVersion.DeclaredParcelIDs)
	snapshot.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "")
	snapshot.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "parcel-1", "FBA15SYNTESTU000001")}

	request, err := domain.RehydrateShipmentRequest(snapshot)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if len(request.CurrentSubmissionVersion().Goods().Lines()) != 1 {
		t.Fatal("goods lost on rehydration")
	}
	if _, declared := request.PlatformShipment(); !declared || len(request.PlatformBoxLabels()) != 1 {
		t.Fatal("platform declarations lost on rehydration")
	}
}

// Covers: 代码评审——快照立不起来是库或适配器的问题，答重建拒绝（ADR-0029 按恢复动作分格），不答成客户输入错。
func TestAMisfittingStoredSnapshotIsARehydrationRefusal(t *testing.T) {
	snapshot := submittedSnapshot(t) // 成员只有 parcel-1
	snapshot.CurrentVersion.Goods = newGoodsDetails(t, domain.GoodsDetailsSpec{Lines: twoLineGoods().Lines,
		Packing: []domain.PackingAllocationSpec{pack(t, "parcel-1", 1, "5"), pack(t, "parcel-2", 2, "1")}}, members(t, "parcel-1", "parcel-2"))
	if _, err := domain.RehydrateShipmentRequest(snapshot); !errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
		t.Errorf("goods packing a non-member: err = %v, want ErrInvalidRehydratedShipmentRequest", err)
	}
	snapshot = submittedSnapshot(t)
	snapshot.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "parcel-1", "FBA15SYNTESTU000001")}
	if _, err := domain.RehydrateShipmentRequest(snapshot); !errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
		t.Errorf("box label without a platform shipment: err = %v, want ErrInvalidRehydratedShipmentRequest", err)
	}
}

// Covers: 代码评审——平台号读回不再过入册门：门日后收紧，已存的委托照样读得回来；新声明照旧过门。
func TestStoredPlatformDeclarationsRehydrateWithoutTheRegisterGate(t *testing.T) {
	long, err := domain.RehydrateExternalIdentifierNumber(strings.Repeat("A", 600))
	if err != nil {
		t.Fatalf("rehydrate number: %v", err)
	}
	if _, err := domain.RehydratePlatformShipment("AMAZON", long, strings.Repeat("R", 600)); err != nil {
		t.Errorf("rehydrate a stored long shipment: %v", err)
	}
	if _, err := domain.RehydratePlatformBoxLabel(mustValue(t, domain.NewDeclaredParcelID, "parcel-1"), long); err != nil {
		t.Errorf("rehydrate a stored long box label: %v", err)
	}
	if _, err := domain.RehydratePlatformShipment(" ", long, ""); !errors.Is(err, domain.ErrInvalidPlatformIdentifier) {
		t.Errorf("blank platform on rehydrate: err = %v", err)
	}
}

// Covers: 代码评审——对着客户包裹参考造好的货物明细挪到成员上，只换分配的包裹标识；漏了哪件就答装箱分配错。装箱分配的错同时是货物明细的错。
func TestGoodsDetailsMoveOntoMembersAndPackingErrorsAreTheirOwnKind(t *testing.T) {
	goods := newGoodsDetails(t, packedTwoLineGoods(t), members(t, "parcel-1", "parcel-2"))
	moved, err := goods.WithPackingParcels(map[domain.DeclaredParcelID]domain.DeclaredParcelID{
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"): mustValue(t, domain.NewDeclaredParcelID, "member-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-2"): mustValue(t, domain.NewDeclaredParcelID, "member-2"),
	})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if packing := moved.Packing(); packing[0].Parcel().String() != "member-1" || packing[1].Parcel().String() != "member-2" || len(moved.Lines()) != 2 {
		t.Fatalf("moved packing = %+v", packing)
	}
	if goods.Packing()[0].Parcel().String() != "parcel-1" {
		t.Fatal("the original was changed; 交回的是拷贝")
	}
	if _, err := goods.WithPackingParcels(map[domain.DeclaredParcelID]domain.DeclaredParcelID{}); !errors.Is(err, domain.ErrInvalidPacking) {
		t.Errorf("missing member: err = %v, want ErrInvalidPacking", err)
	}
	short := domain.GoodsDetailsSpec{Lines: twoLineGoods().Lines, Packing: []domain.PackingAllocationSpec{pack(t, "parcel-1", 1, "4"), pack(t, "parcel-2", 2, "1")}}
	_, err = domain.NewGoodsDetails(short, members(t, "parcel-1", "parcel-2"))
	if !errors.Is(err, domain.ErrInvalidPacking) || !errors.Is(err, domain.ErrInvalidGoodsDetails) {
		t.Errorf("sum short: err = %v, want ErrInvalidPacking and ErrInvalidGoodsDetails", err)
	}
}

// Covers: 代码评审——读回的货物明细不过接入的门：门日后收紧（行数、形状），已存的委托照样读得回来；贴不贴合成员由聚合重建时核。
func TestStoredGoodsRehydrateWithoutTheIntakeGates(t *testing.T) {
	lines := make([]domain.GoodsLineSpec, 1000)
	for index := range lines {
		lines[index] = domain.GoodsLineSpec{Quantity: declared("48 pcs")}
	}
	goods := domain.RehydrateGoodsDetails(domain.GoodsDetailsSpec{Lines: lines})
	if len(goods.Lines()) != 1000 {
		t.Fatalf("lines = %d, want 1000; 读回不按今天的上限截", len(goods.Lines()))
	}
	if quantity, _ := goods.Lines()[0].Quantity(); quantity.Value() != "48 pcs" || goods.Lines()[999].Number() != 1000 {
		t.Fatalf("读回改了内容或行号：%q / %d", quantity.Value(), goods.Lines()[999].Number())
	}
}

// Covers: 代码评审——读回的箱标号不再过入册门，存不进关联册的那一个在接受时不形成关联，不让整笔接受回滚。
func TestAnUnregistrableStoredBoxLabelFormsNoRelationAtAcceptance(t *testing.T) {
	long, err := domain.RehydrateExternalIdentifierNumber(strings.Repeat("B", 600))
	if err != nil {
		t.Fatalf("rehydrate number: %v", err)
	}
	snapshot := submittedSnapshot(t)
	snapshot.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "")
	label, err := domain.RehydratePlatformBoxLabel(mustValue(t, domain.NewDeclaredParcelID, "parcel-1"), long)
	if err != nil {
		t.Fatalf("rehydrate label: %v", err)
	}
	snapshot.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{label}
	request, err := domain.RehydrateShipmentRequest(snapshot)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	accepted, err := request.Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("接受：%v", err)
	}
	for _, relation := range accepted.ExternalIdentifierRelationsFormedByAcceptance(identityOfFixture(t)) {
		if relation.Kind() == domain.PlatformBoxLabel {
			t.Fatalf("存不进册的箱标号形成了关联：%+v", relation)
		}
	}
}

// Covers: 代码评审——换挂包裹标识要一一对应：两个来源包裹换成同一个成员，同一成员同一行会有两条分配，拒掉。
func TestMovingGoodsOntoMembersRefusesTwoParcelsBecomingOne(t *testing.T) {
	goods := newGoodsDetails(t, packedTwoLineGoods(t), members(t, "parcel-1", "parcel-2"))
	same := mustValue(t, domain.NewDeclaredParcelID, "member-1")
	_, err := goods.WithPackingParcels(map[domain.DeclaredParcelID]domain.DeclaredParcelID{
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"): same,
		mustValue(t, domain.NewDeclaredParcelID, "parcel-2"): same,
	})
	if !errors.Is(err, domain.ErrInvalidPacking) {
		t.Fatalf("err = %v, want ErrInvalidPacking", err)
	}
}
