package domain

import (
	"reflect"
	"testing"
	"time"
)

// 本文件钉业务参与方类别的领域判定（ADR-0378 决定一）。读面 SQL 的对照测试在 postgres 适配器里，用的是同一批情形。

func lifecycleFrom(t *testing.T, effectiveFrom time.Time) IdentityLifecycle {
	t.Helper()
	lifecycle, err := NewIdentityLifecycle(effectiveFrom)
	if err != nil {
		t.Fatalf("NewIdentityLifecycle: %v", err)
	}
	return lifecycle
}

func deactivatedAt(t *testing.T, lifecycle IdentityLifecycle, at time.Time) IdentityLifecycle {
	t.Helper()
	basis, err := NewIdentityBasisReference("SYN-BASIS-DEACTIVATE")
	if err != nil {
		t.Fatalf("NewIdentityBasisReference: %v", err)
	}
	deactivated, err := lifecycle.Deactivate(basis, at)
	if err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	return deactivated
}

// 用例里的关系只摆角色与状态两格：判定只读这两格，其余各格（双方、范围、依据、区间）与类别无关。
func TestPartyCategoriesAt(t *testing.T) {
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	past := at.AddDate(0, -1, 0)
	future := at.AddDate(0, 1, 0)

	effective := lifecycleFrom(t, past)
	notYetEffective := lifecycleFrom(t, future)
	deactivated := deactivatedAt(t, lifecycleFrom(t, past.AddDate(0, -1, 0)), past)
	deactivatesLater := deactivatedAt(t, lifecycleFrom(t, past), future)

	cases := []struct {
		name     string
		held     []PartyRelationship
		accounts []IdentityLifecycle
		entities []IdentityLifecycle
		want     []PartyCategory
	}{
		{name: "一类都不属于", want: []PartyCategory{NoRoleCategory}},
		{
			name: "只凭在用客户关系是客户",
			held: []PartyRelationship{{role: CustomerRole, status: RelationshipEffective}},
			want: []PartyCategory{"CUSTOMER"},
		},
		{
			name:     "只凭未停用客户账户是客户",
			accounts: []IdentityLifecycle{effective},
			want:     []PartyCategory{"CUSTOMER"},
		},
		{
			name:     "关系与账户都有，客户只记一次",
			held:     []PartyRelationship{{role: CustomerRole, status: RelationshipEffective}},
			accounts: []IdentityLifecycle{effective},
			want:     []PartyCategory{"CUSTOMER"},
		},
		{
			name: "候选关系算在用",
			held: []PartyRelationship{{role: SupplierRole, status: RelationshipCandidate}},
			want: []PartyCategory{"SUPPLIER"},
		},
		{
			name: "只剩到期、撤销、替代的关系不算",
			held: []PartyRelationship{
				{role: CustomerRole, status: RelationshipExpired},
				{role: SupplierRole, status: RelationshipRevoked},
				{role: CarrierAgentRole, status: RelationshipSuperseded},
			},
			want: []PartyCategory{NoRoleCategory},
		},
		{
			name:     "已登记待生效的账户算未停用",
			accounts: []IdentityLifecycle{notYetEffective},
			want:     []PartyCategory{"CUSTOMER"},
		},
		{
			name:     "已停用的账户不算",
			accounts: []IdentityLifecycle{deactivated},
			want:     []PartyCategory{NoRoleCategory},
		},
		{
			name:     "停用时点未到的账户仍算",
			accounts: []IdentityLifecycle{deactivatesLater},
			want:     []PartyCategory{"CUSTOMER"},
		},
		{
			name:     "被未停用的责任法人钉着是我方公司",
			entities: []IdentityLifecycle{effective},
			want:     []PartyCategory{OurCompanyCategory},
		},
		{
			name:     "被已停用的责任法人钉着不是",
			entities: []IdentityLifecycle{deactivated},
			want:     []PartyCategory{NoRoleCategory},
		},
		{
			name:     "几个法人钉着，有一个未停用就是",
			entities: []IdentityLifecycle{deactivated, notYetEffective},
			want:     []PartyCategory{OurCompanyCategory},
		},
		{
			name: "我方公司同时持有角色，次序照词表",
			held: []PartyRelationship{
				{role: AccountHolderRole, status: RelationshipEffective},
				{role: CustomerRole, status: RelationshipCandidate},
				{role: AccountHolderRole, status: RelationshipCandidate},
			},
			entities: []IdentityLifecycle{effective},
			want:     []PartyCategory{"CUSTOMER", "ACCOUNT_HOLDER", OurCompanyCategory},
		},
		{
			name: "无效的角色不进类别",
			held: []PartyRelationship{{role: PartyRoleInvalid, status: RelationshipEffective}},
			want: []PartyCategory{NoRoleCategory},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PartyCategoriesAt(tc.held, tc.accounts, tc.entities, at)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("PartyCategoriesAt = %v, want %v", got, tc.want)
			}
		})
	}
}

// 时点无效时生命周期判不出状态：账户与法人两条都不成立；关系的状态是登记的事实，不随时点走，照算。
func TestPartyCategoriesAtWithoutAnInstant(t *testing.T) {
	effective := lifecycleFrom(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	got := PartyCategoriesAt(
		[]PartyRelationship{{role: ResellerRole, status: RelationshipEffective}},
		[]IdentityLifecycle{effective},
		[]IdentityLifecycle{effective},
		time.Time{},
	)
	if want := []PartyCategory{"RESELLER"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("PartyCategoriesAt = %v, want %v", got, want)
	}
}

// 词表：各角色照枚举次序，然后是我方公司、无角色；加一个角色时词表跟着长，两个新词仍在最后。
func TestPartyCategoryWords(t *testing.T) {
	want := []string{
		"CUSTOMER", "SUPPLIER", "CARRIER_AGENT", "RESELLER", "ACCOUNT_HOLDER", "AUTHORIZED_DELIVERING_PARTY",
		"OUR_COMPANY", "NO_ROLE",
	}
	if got := PartyCategoryWords(); !reflect.DeepEqual(got, want) {
		t.Fatalf("PartyCategoryWords = %v, want %v", got, want)
	}
	if RoleCategory(PartyRoleInvalid) != "" {
		t.Fatalf("无效角色的类别应为空串")
	}
}

func TestRelationshipStatusLive(t *testing.T) {
	live := map[RelationshipStatus]bool{
		RelationshipCandidate: true, RelationshipEffective: true,
		RelationshipExpired: false, RelationshipRevoked: false, RelationshipSuperseded: false,
		RelationshipStatusInvalid: false,
	}
	for status, want := range live {
		if status.Live() != want {
			t.Errorf("%q.Live() = %v, want %v", status.String(), status.Live(), want)
		}
	}
}
