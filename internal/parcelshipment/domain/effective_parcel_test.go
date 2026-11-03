package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func parcelIDs(t *testing.T, raw ...string) []domain.DeclaredParcelID {
	t.Helper()
	ids := make([]domain.DeclaredParcelID, len(raw))
	for index, value := range raw {
		ids[index] = mustValue(t, domain.NewDeclaredParcelID, value)
	}
	return ids
}

func declaredNumber(t *testing.T, parcel, number string) domain.CustomerParcelNumberDeclaration {
	t.Helper()
	declaration, err := domain.DeclareCustomerParcelNumber(
		mustValue(t, domain.NewDeclaredParcelID, parcel), mustValue(t, domain.NewExternalIdentifierNumber, number))
	if err != nil {
		t.Fatalf("配对 %s：%v", parcel, err)
	}
	return declaration
}

// Covers: ADR-0356 决定三「次序」与 PS CONTEXT「委托」客户按委托的单据号追踪那一条——谱系落地之前，当前有效包裹就是接受基线
// 成员，次序与位次取首个提交版本（基线所在版本的次序不同也不跟它），每件带回自己的客户包裹件号，没声明的件没有件号；来源关系
// 恒空。
func TestBeforeLineageTheEffectiveParcelsAreTheBaselineInFirstVersionOrder(t *testing.T) {
	parcels, err := domain.CurrentEffectiveParcelsOfBaseline(
		parcelIDs(t, "parcel-b", "parcel-a", "parcel-c"),
		parcelIDs(t, "parcel-a", "parcel-c", "parcel-b"),
		[]domain.CustomerParcelNumberDeclaration{declaredNumber(t, "parcel-c", "CUST-C"), declaredNumber(t, "parcel-b", "CUST-B")},
	)
	if err != nil {
		t.Fatalf("立当前有效包裹：%v", err)
	}
	want := []struct {
		parcel string
		number string
	}{{"parcel-b", "CUST-B"}, {"parcel-a", ""}, {"parcel-c", "CUST-C"}}
	if len(parcels) != len(want) {
		t.Fatalf("立出 %d 件，want %d", len(parcels), len(want))
	}
	for index, parcel := range parcels {
		if parcel.Parcel().String() != want[index].parcel || parcel.Position() != index+1 {
			t.Errorf("第 %d 件 = %s 位次 %d，want %s 位次 %d", index, parcel.Parcel(), parcel.Position(), want[index].parcel, index+1)
		}
		number, declared := parcel.CustomerParcelNumber()
		if declared != (want[index].number != "") || number.String() != want[index].number {
			t.Errorf("%s 的件号 = %q（%v），want %q", parcel.Parcel(), number, declared, want[index].number)
		}
		if replaces := parcel.Replaces(); len(replaces) != 0 {
			t.Errorf("%s 谱系未建模却带了来源包裹 %v", parcel.Parcel(), replaces)
		}
	}
}

// Covers: 当前有效包裹立不起来时具名报错，不择一、不补齐——空基线、基线成员重复、首个提交版本与基线成员集合不等、件号配到基线
// 之外、同一件两个件号。
func TestInconsistentInputsDoNotFormEffectiveParcels(t *testing.T) {
	for name, inputs := range map[string]struct {
		first, baseline []domain.DeclaredParcelID
		numbers         []domain.CustomerParcelNumberDeclaration
	}{
		"empty baseline":          {parcelIDs(t), parcelIDs(t), nil},
		"repeated baseline":       {parcelIDs(t, "parcel-a", "parcel-b"), parcelIDs(t, "parcel-a", "parcel-a"), nil},
		"first version differs":   {parcelIDs(t, "parcel-a", "parcel-x"), parcelIDs(t, "parcel-a", "parcel-b"), nil},
		"first version shorter":   {parcelIDs(t, "parcel-a"), parcelIDs(t, "parcel-a", "parcel-b"), nil},
		"first version repeated":  {parcelIDs(t, "parcel-a", "parcel-a"), parcelIDs(t, "parcel-a", "parcel-b"), nil},
		"number outside baseline": {parcelIDs(t, "parcel-a"), parcelIDs(t, "parcel-a"), []domain.CustomerParcelNumberDeclaration{declaredNumber(t, "parcel-x", "CUST-X")}},
		"two numbers on one parcel": {parcelIDs(t, "parcel-a"), parcelIDs(t, "parcel-a"),
			[]domain.CustomerParcelNumberDeclaration{declaredNumber(t, "parcel-a", "CUST-1"), declaredNumber(t, "parcel-a", "CUST-2")}},
	} {
		if parcels, err := domain.CurrentEffectiveParcelsOfBaseline(inputs.first, inputs.baseline, inputs.numbers); !errors.Is(err, domain.ErrInconsistentEffectiveParcels) {
			t.Errorf("%s：%v %v, want ErrInconsistentEffectiveParcels", name, parcels, err)
		}
	}
}
