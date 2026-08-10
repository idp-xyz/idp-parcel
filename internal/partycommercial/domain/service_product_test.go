package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func productVersion(t *testing.T, objectID string) domain.CommercialVersion {
	t.Helper()
	live, err := registerable(t, domain.ServiceProductObject, objectID, "v1", "sha256:"+objectID).
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

func serviceProduct(t *testing.T, objectID string) domain.ServiceProduct {
	t.Helper()
	product, err := domain.NewServiceProduct(productVersion(t, objectID), domain.NetworkServiceForm)
	if err != nil {
		t.Fatalf("new service product: %v", err)
	}
	return product
}

func channelMapping(t *testing.T, product domain.ServiceProduct, channels ...string) domain.ProductChannelMapping {
	t.Helper()
	refs := make([]domain.ChannelProductReference, len(channels))
	for index, channel := range channels {
		refs[index] = commercialValue(t, domain.NewChannelProductReference, channel)
	}
	mapping, err := domain.NewProductChannelMapping(product, refs, mustInterval(t))
	if err != nil {
		t.Fatalf("new product channel mapping: %v", err)
	}
	return mapping
}

// Covers: CONTEXT「映射定义候选范围，不代表某次交易已经选择或使用了其中一个渠道产品」，
// 以及「不把候选关系伪装成实际选择」。
func TestAMappingYieldsCandidatesNotASelection(t *testing.T) {
	mapping := channelMapping(t, serviceProduct(t, "product-1"), "channel-a", "channel-b")

	candidates := mapping.CandidatesAt(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if len(candidates) != 2 {
		t.Fatalf("candidates = %v, want two", candidates)
	}

	mappingType := reflect.TypeOf(domain.ProductChannelMapping{})
	forbidden := []string{"selected", "selection", "chosen", "locked", "lock", "preferred", "default"}
	for index := 0; index < mappingType.NumField(); index++ {
		name := strings.ToLower(mappingType.Field(index).Name)
		for _, word := range forbidden {
			if strings.Contains(name, word) {
				t.Fatalf("ProductChannelMapping carries %s, which turns a candidate range into a selection",
					mappingType.Field(index).Name)
			}
		}
	}
}

// Covers: CONTEXT「对网络服务产品，客户可以通过渠道约束限定允许范围，运营企业只能在该
// 范围内选择」— 约束只收窄候选，绝不引入映射之外的渠道。
func TestCustomerConstraintOnlyNarrowsTheCandidateRange(t *testing.T) {
	mapping := channelMapping(t, serviceProduct(t, "product-1"), "channel-a", "channel-b")
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	narrowed := mapping.CandidatesAllowedBy([]domain.ChannelProductReference{
		commercialValue(t, domain.NewChannelProductReference, "channel-b"),
	}, at)
	if len(narrowed) != 1 || narrowed[0].String() != "channel-b" {
		t.Fatalf("narrowed = %v, want only channel-b", narrowed)
	}

	outside := mapping.CandidatesAllowedBy([]domain.ChannelProductReference{
		commercialValue(t, domain.NewChannelProductReference, "channel-z"),
	}, at)
	if len(outside) != 0 {
		t.Fatalf("a constraint naming a channel outside the mapping produced %v", outside)
	}
}

// Covers: CONTEXT「渠道产品停止商业可用或相关映射到期，只将其排除在新的渠道选择之外，
// 不删除历史依据」。
func TestAnExpiredMappingStopsOfferingCandidatesButKeepsItsRecord(t *testing.T) {
	mapping := channelMapping(t, serviceProduct(t, "product-1"), "channel-a")
	after := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)

	if len(mapping.CandidatesAt(after)) != 0 {
		t.Fatal("an expired mapping still offered candidates for a new selection")
	}
	if len(mapping.ChannelProducts()) != 1 {
		t.Fatal("expiry deleted the mapping's historical channel references")
	}
	if _, bounded := mapping.Effective().EndsAt(); !bounded {
		t.Fatal("the mapping lost its effective interval")
	}
}

// Covers: CONTEXT「网络产品是服务产品的一种形态，不建立独立于服务产品的第三套产品目录」，
// 以及首发由 `PAR-COM-12` 明确不适用独立面单渠道服务。
func TestServiceProductFormIsAFacetNotASeparateCatalog(t *testing.T) {
	product := serviceProduct(t, "product-1")

	if product.Form() != domain.NetworkServiceForm {
		t.Fatalf("form = %q, want NETWORK_SERVICE", product.Form())
	}
	if product.Version().Kind() != domain.ServiceProductObject {
		t.Fatal("a network product is not carried by a service product version")
	}
	if domain.ServiceProductForm(2).String() != "" {
		t.Fatal("a second product form carries a label; the label-channel form is not in the first release")
	}
}

// Covers: CONTEXT 商业版本共同不变量 — 产品与映射都挂在当前可用的版本上，且映射不得
// 引用空的渠道集合。
func TestProductAndMappingBindToUsableVersions(t *testing.T) {
	t.Run("product refuses a draft", func(t *testing.T) {
		draft := commercialDraft(t, domain.ServiceProductObject, "product-x", "v1", "sha256:x")
		if _, err := domain.NewServiceProduct(draft, domain.NetworkServiceForm); !errors.Is(err, domain.ErrInvalidServiceProduct) {
			t.Fatalf("error = %v, want ErrInvalidServiceProduct", err)
		}
	})

	t.Run("product refuses another object kind", func(t *testing.T) {
		if _, err := domain.NewServiceProduct(contractVersion(t, "contract-6"), domain.NetworkServiceForm); !errors.Is(err, domain.ErrInvalidServiceProduct) {
			t.Fatalf("error = %v, want ErrInvalidServiceProduct", err)
		}
	})

	t.Run("mapping refuses an empty channel set", func(t *testing.T) {
		if _, err := domain.NewProductChannelMapping(serviceProduct(t, "product-1"), nil, mustInterval(t)); !errors.Is(err, domain.ErrInvalidProductChannelMapping) {
			t.Fatalf("error = %v, want ErrInvalidProductChannelMapping", err)
		}
	})

	t.Run("mapping refuses a duplicate channel", func(t *testing.T) {
		duplicate := []domain.ChannelProductReference{
			commercialValue(t, domain.NewChannelProductReference, "channel-a"),
			commercialValue(t, domain.NewChannelProductReference, "channel-a"),
		}
		if _, err := domain.NewProductChannelMapping(serviceProduct(t, "product-1"), duplicate, mustInterval(t)); !errors.Is(err, domain.ErrInvalidProductChannelMapping) {
			t.Fatalf("error = %v, want ErrInvalidProductChannelMapping", err)
		}
	})
}
