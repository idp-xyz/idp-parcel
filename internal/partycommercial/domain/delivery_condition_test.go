package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉票 party-commercial-context-gaps/11 完成判据 1（ADR-0133 决定四）：交付条件是服务产品版本声明、
// 客户合同版本只能在其内收紧的商业条件——产品层零方式拒、合同层出现产品层没有的方式拒（放宽）、合同层子集
// 通过、拥有对象非对应种类或未生效拒。取值全部合成，不写任何租户的交付方式或规则。

func deliveryMethods(t *testing.T, names ...string) []domain.DeliveryMethodReference {
	t.Helper()
	methods := make([]domain.DeliveryMethodReference, 0, len(names))
	for _, name := range names {
		methods = append(methods, commercialValue(t, domain.NewDeliveryMethodReference, name))
	}
	return methods
}

func deliveryTerms(t *testing.T, methods ...string) domain.DeliveryConditionTerms {
	t.Helper()
	return domain.DeliveryConditionTerms{
		Methods:             deliveryMethods(t, methods...),
		RecipientScopeRule:  commercialValue(t, domain.NewDeliveryRuleReference, "RULE/recipient-scope"),
		ProofOfDeliveryRule: commercialValue(t, domain.NewDeliveryRuleReference, "RULE/proof-of-delivery"),
	}
}

func tightens(t *testing.T, objectID, version string) domain.TightenedProductVersion {
	t.Helper()
	target, err := domain.NewTightenedProductVersion(
		commercialValue(t, domain.NewCommercialObjectID, objectID),
		commercialValue(t, domain.NewCommercialVersionLabel, version),
	)
	if err != nil {
		t.Fatalf("构造所收紧的产品版本 %s/%s：%v", objectID, version, err)
	}
	return target
}

// Covers: 产品层声明立住——拥有对象是已生效服务产品版本；方式集合按字面排序、去重后交回；两条规则引用照登；
// 产品层不收紧任何东西（Tightens 答无）。
func TestProductDeliveryConditionsAreDeclaredOnAnEffectiveServiceProductVersion(t *testing.T) {
	product := effectiveVersionOfKind(t, domain.ServiceProductObject, "product-1")

	content, err := domain.DeclareProductDeliveryConditions(product, deliveryTerms(t, "method-b", "method-a"))
	if err != nil {
		t.Fatalf("产品层声明：%v", err)
	}
	if content.Owner().Kind() != domain.ServiceProductObject ||
		content.Owner().ObjectID() != product.ObjectID() || content.Owner().Version() != product.Version() {
		t.Fatalf("拥有对象 = %s/%s（%s）", content.Owner().ObjectID(), content.Owner().Version(), content.Owner().Kind())
	}
	methods := content.Methods()
	if len(methods) != 2 || methods[0].String() != "method-a" || methods[1].String() != "method-b" {
		t.Fatalf("方式集合 = %v，want 按字面排序的 [method-a method-b]", methods)
	}
	if !content.Allows(commercialValue(t, domain.NewDeliveryMethodReference, "method-a")) ||
		content.Allows(commercialValue(t, domain.NewDeliveryMethodReference, "method-z")) {
		t.Fatal("Allows 与方式集合对不上")
	}
	if content.RecipientScopeRule().String() != "RULE/recipient-scope" ||
		content.ProofOfDeliveryRule().String() != "RULE/proof-of-delivery" {
		t.Fatalf("规则引用 = %s / %s", content.RecipientScopeRule(), content.ProofOfDeliveryRule())
	}
	if _, tightening := content.Tightens(); tightening {
		t.Fatal("产品层不该报出所收紧的产品版本")
	}
}

// Covers: 「没有默认」——产品层零方式不是声明（缺席才是「没有交付条件」，一份登了却什么方式都不许的产品层
// 说不通）；规则引用缺一格同样是缺件；同一方式两行是冲突，即便字面完全相同也不替登记方去重成一行。
func TestAProductLayerWithNoMethodOrMissingRuleIsNotADeclaration(t *testing.T) {
	product := effectiveVersionOfKind(t, domain.ServiceProductObject, "product-1")

	if _, err := domain.DeclareProductDeliveryConditions(product, deliveryTerms(t)); !errors.Is(err, domain.ErrDeliveryConditionNotConfigured) {
		t.Fatalf("零方式：err = %v，want ErrDeliveryConditionNotConfigured", err)
	}

	missingProof := deliveryTerms(t, "method-a")
	missingProof.ProofOfDeliveryRule = domain.DeliveryRuleReference{}
	if _, err := domain.DeclareProductDeliveryConditions(product, missingProof); !errors.Is(err, domain.ErrDeliveryConditionNotConfigured) {
		t.Fatalf("缺交付证明规则引用：err = %v，want ErrDeliveryConditionNotConfigured", err)
	}

	missingScope := deliveryTerms(t, "method-a")
	missingScope.RecipientScopeRule = domain.DeliveryRuleReference{}
	if _, err := domain.DeclareProductDeliveryConditions(product, missingScope); !errors.Is(err, domain.ErrDeliveryConditionNotConfigured) {
		t.Fatalf("缺收件范围规则引用：err = %v，want ErrDeliveryConditionNotConfigured", err)
	}

	blankMethod := deliveryTerms(t, "method-a")
	blankMethod.Methods = append(blankMethod.Methods, domain.DeliveryMethodReference{})
	if _, err := domain.DeclareProductDeliveryConditions(product, blankMethod); !errors.Is(err, domain.ErrDeliveryConditionNotConfigured) {
		t.Fatalf("空方式引用：err = %v，want ErrDeliveryConditionNotConfigured", err)
	}

	if _, err := domain.DeclareProductDeliveryConditions(product, deliveryTerms(t, "method-a", "method-a")); !errors.Is(err, domain.ErrConflictingDeliveryCondition) {
		t.Fatalf("同一方式两行：err = %v，want ErrConflictingDeliveryCondition", err)
	}
}

// Covers: 拥有对象非对应种类或未生效拒——接单规则包不是交付条件的家（ADR-0133 决定四否决那一支）；已发布未生效
// 的产品版本挂不上声明；产品层的门不收合同版本、合同层的门不收产品版本，两层不互相冒名。
func TestDeliveryConditionsRefuseOwnersOfTheWrongKindOrNotYetEffective(t *testing.T) {
	rulePackage := effectiveVersionOfKind(t, domain.AcceptanceRulePackageObject, "rules-1")
	if _, err := domain.DeclareProductDeliveryConditions(rulePackage, deliveryTerms(t, "method-a")); !errors.Is(err, domain.ErrDeliveryConditionOwner) {
		t.Fatalf("接单规则包当产品层：err = %v，want ErrDeliveryConditionOwner", err)
	}

	published := registerable(t, domain.ServiceProductObject, "product-1", "v1", "sha256:product-1")
	if _, err := domain.DeclareProductDeliveryConditions(published, deliveryTerms(t, "method-a")); !errors.Is(err, domain.ErrDeliveryConditionOwner) {
		t.Fatalf("已发布未生效的产品：err = %v，want ErrDeliveryConditionOwner", err)
	}

	contract := effectiveVersionOfKind(t, domain.CustomerContractObject, "contract-1")
	if _, err := domain.DeclareProductDeliveryConditions(contract, deliveryTerms(t, "method-a")); !errors.Is(err, domain.ErrDeliveryConditionOwner) {
		t.Fatalf("合同版本走产品层的门：err = %v，want ErrDeliveryConditionOwner", err)
	}
	product := effectiveVersionOfKind(t, domain.ServiceProductObject, "product-1")
	if _, err := domain.DeclareContractDeliveryConditions(product, tightens(t, "product-1", "v1"), deliveryTerms(t, "method-a")); !errors.Is(err, domain.ErrDeliveryConditionOwner) {
		t.Fatalf("产品版本走合同层的门：err = %v，want ErrDeliveryConditionOwner", err)
	}
	if _, err := domain.DeclareContractDeliveryConditions(contract, domain.TightenedProductVersion{}, deliveryTerms(t, "method-a")); !errors.Is(err, domain.ErrDeliveryConditionNotConfigured) {
		t.Fatalf("合同层不指名所收紧的产品版本：err = %v，want ErrDeliveryConditionNotConfigured", err)
	}
}

// Covers: 合同层只能在产品层之内收紧（PC CONTEXT「交付条件」词条）——子集通过；出现产品层没有的方式是放宽，拒；
// 对着的不是它指名的那一版产品层（另一版、或根本是一份合同层）也拒——收紧是对着一份具体的产品层说的话。
func TestContractDeliveryConditionsMayOnlyTightenWithinTheNamedProductLayer(t *testing.T) {
	product := effectiveVersionOfKind(t, domain.ServiceProductObject, "product-1")
	productLayer, err := domain.DeclareProductDeliveryConditions(product, deliveryTerms(t, "method-a", "method-b", "method-c"))
	if err != nil {
		t.Fatalf("产品层声明：%v", err)
	}
	contract := effectiveVersionOfKind(t, domain.CustomerContractObject, "contract-1")

	subset, err := domain.DeclareContractDeliveryConditions(contract, tightens(t, "product-1", "v1"), deliveryTerms(t, "method-a"))
	if err != nil {
		t.Fatalf("合同层声明：%v", err)
	}
	if target, tightening := subset.Tightens(); !tightening ||
		target.ObjectID().String() != "product-1" || target.Version().String() != "v1" {
		t.Fatalf("合同层所收紧的产品版本 = %+v %v", target, tightening)
	}
	if err := subset.TightensWithin(productLayer); err != nil {
		t.Fatalf("子集应通过：%v", err)
	}

	widened, err := domain.DeclareContractDeliveryConditions(contract, tightens(t, "product-1", "v1"), deliveryTerms(t, "method-a", "method-d"))
	if err != nil {
		t.Fatalf("合同层声明（待核）：%v", err)
	}
	if err := widened.TightensWithin(productLayer); !errors.Is(err, domain.ErrDeliveryConditionWidened) {
		t.Fatalf("放宽：err = %v，want ErrDeliveryConditionWidened", err)
	}

	otherVersion, err := domain.DeclareContractDeliveryConditions(contract, tightens(t, "product-1", "v2"), deliveryTerms(t, "method-a"))
	if err != nil {
		t.Fatalf("合同层声明（指名另一版）：%v", err)
	}
	if err := otherVersion.TightensWithin(productLayer); !errors.Is(err, domain.ErrDeliveryConditionTighteningTarget) {
		t.Fatalf("对着不是它指名的那一版：err = %v，want ErrDeliveryConditionTighteningTarget", err)
	}

	if err := subset.TightensWithin(subset); !errors.Is(err, domain.ErrDeliveryConditionTighteningTarget) {
		t.Fatalf("对着一份合同层：err = %v，want ErrDeliveryConditionTighteningTarget", err)
	}
	if err := productLayer.TightensWithin(productLayer); !errors.Is(err, domain.ErrDeliveryConditionOwner) {
		t.Fatalf("产品层无所谓收紧：err = %v，want ErrDeliveryConditionOwner", err)
	}
}

// Covers: 按回指答「有没有交付条件」的封闭四格（ADR-0133 决定二，票面「要建什么」2）：闭包唯一解析且采用了合同，
// 两层至少一层有声明 → 交付条件引用，且引用就是回指本身、不另造；两层都无 → 没有（found=false，err=nil）；闭包
// 不在场（未唯一解析的结果）→ ErrDeliveryConditionClosureAbsent；闭包在场却未采用客户合同版本 → ErrDeliveryConditionContractNotAdopted。
func TestDeliveryConditionReferenceIsAnsweredFromTheClosureAndTheTwoLayers(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", domain.ServiceProductObject, domain.CustomerContractObject)
	closure := domain.ResolveCommercialClosure(registry,
		closureKey(t, "scope-a", domain.ServiceProductObject, domain.CustomerContractObject), nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("夹具闭包未唯一解析：%s", closure.Outcome())
	}

	for _, layers := range []struct{ product, contract bool }{{true, false}, {false, true}, {true, true}} {
		reference, found, err := domain.DeliveryConditionReferenceFor(closure, layers.product, layers.contract)
		if err != nil || !found || reference.Resolution() != closure.ResolutionID() || reference.String() != closure.ResolutionID().String() {
			t.Fatalf("产品层 %v / 合同层 %v：reference=%s found=%v err=%v", layers.product, layers.contract, reference, found, err)
		}
	}
	if _, found, err := domain.DeliveryConditionReferenceFor(closure, false, false); err != nil || found {
		t.Fatalf("两层都无：found=%v err=%v，want false, nil", found, err)
	}

	if _, _, err := domain.DeliveryConditionReferenceFor(domain.CommercialClosure{}, true, true); !errors.Is(err, domain.ErrDeliveryConditionClosureAbsent) {
		t.Fatalf("闭包不在场：err = %v，want ErrDeliveryConditionClosureAbsent", err)
	}

	productOnly := domain.NewCommercialRegistry()
	seedClosure(t, productOnly, "scope-a", domain.ServiceProductObject)
	withoutContract := domain.ResolveCommercialClosure(productOnly, closureKey(t, "scope-a", domain.ServiceProductObject), nil)
	if withoutContract.Outcome() != domain.UniquelyResolved {
		t.Fatalf("只要产品的夹具闭包未唯一解析：%s", withoutContract.Outcome())
	}
	if _, _, err := domain.DeliveryConditionReferenceFor(withoutContract, true, false); !errors.Is(err, domain.ErrDeliveryConditionContractNotAdopted) {
		t.Fatalf("未采用客户合同版本：err = %v，want ErrDeliveryConditionContractNotAdopted", err)
	}
}
