package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func pricingScopes(t *testing.T, names ...string) []domain.PricingScopeReference {
	t.Helper()
	scopes := make([]domain.PricingScopeReference, 0, len(names))
	for _, name := range names {
		scopes = append(scopes, commercialValue(t, domain.NewPricingScopeReference, name))
	}
	return scopes
}

// Covers: CONTEXT「客户合同版本」销售价格约定（ADR-0380 决定一）——合同声明一份有序的销售价格来源，每项指名一个
// 计价范围；读回的顺序就是声明的顺序，回退只在这份顺序里发生，所以顺序本身是正文。
func TestAContractDeclaresItsSellPriceSourcesInOrder(t *testing.T) {
	declared, err := domain.DeclareContractSellPriceArrangement(
		contractVersion(t, "contract-1"),
		pricingScopes(t, "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD"),
		domain.NoSellPriceBasis{},
	)
	if err != nil {
		t.Fatalf("declare contract sell price: %v", err)
	}

	sources, priced := declared.Sources()
	if !priced || len(sources) != 2 ||
		sources[0].String() != "SYN-SCOPE-EXCLUSIVE" || sources[1].String() != "SYN-SCOPE-STANDARD" {
		t.Fatalf("sources = %v priced = %v, want the declared order", sources, priced)
	}
	if _, notPriced := declared.NoSellPriceBasis(); notPriced {
		t.Fatal("a contract that names sell price sources also reads as not priced")
	}
	if declared.Contract().ObjectID().String() != "contract-1" {
		t.Fatalf("owner = %s, want contract-1", declared.Contract().ObjectID())
	}
}

// Covers: ADR-0380 决定一「或者显式声明本合同不计销售价并留依据」——不计销售价是一句带依据的声明，读回时没有来源
// 列表；它与「合同没说」是两件事，后者根本立不成一份约定。
func TestAContractMayExplicitlyDeclareNoSellPriceWithItsBasis(t *testing.T) {
	declared, err := domain.DeclareContractSellPriceArrangement(
		contractVersion(t, "contract-1"),
		nil,
		commercialValue(t, domain.NewNoSellPriceBasis, "SYN-NO-SELL-PRICE-01"),
	)
	if err != nil {
		t.Fatalf("declare no sell price: %v", err)
	}

	basis, notPriced := declared.NoSellPriceBasis()
	if !notPriced || basis.String() != "SYN-NO-SELL-PRICE-01" {
		t.Fatalf("basis = %q notPriced = %v", basis, notPriced)
	}
	if sources, priced := declared.Sources(); priced || len(sources) != 0 {
		t.Fatalf("a contract that is not priced reported sources %v", sources)
	}
}

// Covers: ADR-0380 决定一——来源列表与不计销售价二选一：两种都没声明、两种都声明、列表为空、同一范围出现两次、
// 列表里夹着零值，都立不成约定。两种都没声明不折成「不计销售价」：那会把缺声明读成一句没人说过的话。
func TestASellPriceArrangementDeclaresExactlyOneOfSourcesOrNoSellPrice(t *testing.T) {
	basis := commercialValue(t, domain.NewNoSellPriceBasis, "SYN-NO-SELL-PRICE-01")
	refusals := map[string]struct {
		sources []domain.PricingScopeReference
		basis   domain.NoSellPriceBasis
		want    error
	}{
		"两种都没声明": {want: domain.ErrInvalidSellPriceArrangement},
		"列表为空而没有依据": {
			sources: []domain.PricingScopeReference{},
			want:    domain.ErrInvalidSellPriceArrangement,
		},
		"两种都声明": {
			sources: pricingScopes(t, "SYN-SCOPE-STANDARD"),
			basis:   basis,
			want:    domain.ErrInvalidSellPriceArrangement,
		},
		"同一范围出现两次": {
			sources: pricingScopes(t, "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD", "SYN-SCOPE-EXCLUSIVE"),
			want:    domain.ErrDuplicateSellPriceSource,
		},
		"列表里夹着零值": {
			sources: append(pricingScopes(t, "SYN-SCOPE-STANDARD"), domain.PricingScopeReference{}),
			want:    domain.ErrInvalidSellPriceArrangement,
		},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			_, err := domain.DeclareContractSellPriceArrangement(contractVersion(t, "contract-1"), refusal.sources, refusal.basis)
			if !errors.Is(err, refusal.want) {
				t.Fatalf("err = %v, want %v", err, refusal.want)
			}
		})
	}
}

// Covers: ADR-0380 决定一「每项指名一个计价范围，同一范围不出现两次」——计价范围引用指名的是计价一侧选卡键上的
// 范围，那一侧的标识不带首尾空白：带着空白的名字指不到任何范围，还会让「不出现两次」被一个空格绕过，所以在构造门
// 就拒。只有空白的串照旧答必填；名字中间的空白原样保留。
func TestAPricingScopeReferenceCarriesNoSurroundingWhitespace(t *testing.T) {
	for _, raw := range []string{" SYN-SCOPE-STANDARD", "SYN-SCOPE-STANDARD ", "SYN-SCOPE-STANDARD\n"} {
		if _, err := domain.NewPricingScopeReference(raw); !errors.Is(err, domain.ErrPricingScopeReferenceNotExact) {
			t.Fatalf("%q: err = %v, want ErrPricingScopeReferenceNotExact", raw, err)
		}
	}
	if _, err := domain.NewPricingScopeReference("   "); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("blank: err = %v, want ErrBlankValue", err)
	}
	scope, err := domain.NewPricingScopeReference("SYN SCOPE 01")
	if err != nil || scope.String() != "SYN SCOPE 01" {
		t.Fatalf("a name with inner whitespace must be kept as given: %q, %v", scope, err)
	}
}

// Covers: CONTEXT 商业版本共同不变量——销售价格约定挂在一个当前可用的客户合同版本上；草稿与别类对象承载不了它，
// 判据与合同级接受前控制声明同一条。
func TestASellPriceArrangementNeedsAUsableCustomerContractVersion(t *testing.T) {
	sources := pricingScopes(t, "SYN-SCOPE-STANDARD")

	draft := commercialDraft(t, domain.CustomerContractObject, "contract-x", "v1", "sha256:x")
	if _, err := domain.DeclareContractSellPriceArrangement(draft, sources, domain.NoSellPriceBasis{}); !errors.Is(err, domain.ErrUnusableContract) {
		t.Fatalf("draft: err = %v, want ErrUnusableContract", err)
	}

	policy, err := registerable(t, domain.SettlementPolicyObject, "policy-9", "v1", "sha256:p9").
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := domain.DeclareContractSellPriceArrangement(policy, sources, domain.NoSellPriceBasis{}); !errors.Is(err, domain.ErrUnusableContract) {
		t.Fatalf("another kind: err = %v, want ErrUnusableContract", err)
	}
}
