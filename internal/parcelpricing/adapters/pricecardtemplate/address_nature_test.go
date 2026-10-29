package pricecardtemplate

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 导入模板认地址性质与两个派生重量的判定来源（ADR-0347 决定二、四）：ADDRESS_NATURE 是类别类来源、只收封闭词，
// CHARGEABLE_WEIGHT 与 VOLUMETRIC_WEIGHT 是重量类来源，阈值单位须是价表单位。

// 地址性质条件：把 fullCard 的偏远条件换成 ADDRESS_NATURE = RESIDENTIAL，方案立得住并读到这个特征。
func TestTheTemplateAcceptsAnAddressNatureCondition(t *testing.T) {
	card := fullCard().clone()
	remote := conditionNode(t, card, "C-REMOTE")
	remote["source"] = "ADDRESS_NATURE"
	remote["category"] = "RESIDENTIAL"

	plan := validated(t, read(t, card.bytes(t)))
	if !plan.Structures().ReadsFeature(domain.FeatureAddressNature) {
		t.Fatal("the imported card does not read the address nature")
	}
}

// 期望取值不在封闭词表内：落在 category 格上，构造门拒（与偏远档位那类开放取值不同）。
func TestTheTemplateRefusesAnAddressNatureOutsideTheVocabulary(t *testing.T) {
	card := fullCard().clone()
	remote := conditionNode(t, card, "C-REMOTE")
	remote["source"] = "ADDRESS_NATURE"
	remote["category"] = "RESIDENTAL"

	reading := read(t, card.bytes(t))
	want := at{sheetConditions, conditionRow(t, card, "C-REMOTE"), "category", ports.TemplateConstructionRejected}
	if !reading.Accepted || reading.Content != nil || !hasProblem(reading.Problems, want) {
		t.Fatalf("want %+v, got problems=%s", want, describe(reading.Problems))
	}
}

// 读自身计价重量的拒收条款：阈值按价表单位（KG）写，立得住。
func TestTheTemplateAcceptsAChargeableWeightCondition(t *testing.T) {
	card := fullCard().clone()
	exclusion := conditionNode(t, card, "C-EXCL")
	exclusion["source"] = "CHARGEABLE_WEIGHT"
	exclusion["threshold"] = "70"

	plan := validated(t, read(t, card.bytes(t)))
	if !plan.Structures().ReadsFeature(domain.FeatureChargeableWeight) {
		t.Fatal("the imported card does not read the chargeable weight")
	}
}

func conditionNode(t *testing.T, card filled, id string) map[string]string {
	t.Helper()
	for _, line := range card.data[sheetConditions] {
		if line["nodeId"] == id {
			return line
		}
	}
	t.Fatalf("夹具里没有条件节点 %s", id)
	return nil
}

func conditionRow(t *testing.T, card filled, id string) int {
	t.Helper()
	for index, line := range card.data[sheetConditions] {
		if line["nodeId"] == id {
			return index + 2
		}
	}
	t.Fatalf("夹具里没有条件节点 %s", id)
	return 0
}
