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

// Covers: `AT-PC-015`「发布独立面单渠道服务的服务产品版本 → 按 `PAR-COM-12` 与网络服务产品
// **同等**进入首发发布范围」与 `AT-PC-030`「请求独立面单渠道服务 → 与网络服务同一两阶段机制，
// 且**不把网络服务产品当作面单渠道服务的候选**」。两条验收行均已随 ADR-0088 修订。
//
// **本用例的断言方向随 ADR-0088 反转过一次，原样记在这里。** 它原名
// `TestNoServiceProductCanTakeAnIndependentWaybillChannelForm`，断言的是「除网络服务外任何
// 取值都构造不出服务产品」，依据是 `PAR-COM-12` 当时登记为**本期不适用**。ADR-0088 把该参数
// 由范围裁剪改为纳入，那条依据不再成立，因此原断言不再成立——不是它写错了，是它守的那条
// 范围决策被改了。守卫本身的价值没变，换的只是期望集合。
//
// 没变的那部分是它当初为什么这么写：扫完整个 uint8 值域，并且问「构造得出服务产品吗」而不只是
// 「有没有名字」。`TestServiceProductFormIsAFacetNotASeparateCatalog` 有一个真实的假阴性——
// 它只探一个写死的取值。实测（于 `cab9a45`）把新形态加在取值 3 上并让它 `valid()`，那条用例
// 照绿而本用例变红：**新加的那一格取什么数，不由守卫这边说了算**。这一条今天照旧成立，
// 只是它现在挡的是第三格而不是第二格。
func TestOnlyTheTwoDocumentedServiceProductFormsConstruct(t *testing.T) {
	live := productVersion(t, "product-form-guard")

	documented := map[domain.ServiceProductForm]string{
		domain.NetworkServiceForm:      "NETWORK_SERVICE",
		domain.LabelChannelServiceForm: "LABEL_CHANNEL_SERVICE",
	}
	for form, name := range documented {
		if _, err := domain.NewServiceProduct(live, form); err != nil {
			t.Fatalf("%s 已由 ADR-0088 进入首发形态，它却构造不出来: %v", name, err)
		}
		if form.String() != name {
			t.Fatalf("形态名 = %q, want %q——取值名取的是领域文档原词，不得就地改", form.String(), name)
		}
	}

	for value := 0; value <= 255; value++ {
		form := domain.ServiceProductForm(value)
		if _, documented := documented[form]; documented {
			continue
		}
		if _, err := domain.NewServiceProduct(live, form); !errors.Is(err, domain.ErrInvalidServiceProduct) {
			t.Fatalf("服务形态取值 %d 构造出了服务产品（error = %v），而领域今天只有两格", value, err)
		}
		if name := form.String(); name != "" {
			t.Fatalf("服务形态取值 %d 已经有名字 %q，形态枚举被扩过而这道守卫没有跟上", value, name)
		}
	}
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

// Covers: CONTEXT「网络产品是服务产品的一种形态，不建立独立于服务产品的第三套产品目录」。
//
// 原用例的后半段断言 `ServiceProductForm(2).String() == ""`，依据是首发不适用面单渠道服务；
// ADR-0088 之后取值 2 正是它，那半段随之失效。**换掉的不是「第三套目录」这条不变量，是它
// 当初借来表达该不变量的手段**——两种形态都由服务产品版本承载，才是「不另立目录」的直接说法，
// 而「第二格没有名字」只是当时恰好也成立的一个推论。取值域边界由
// `TestOnlyTheTwoDocumentedServiceProductFormsConstruct` 扫全域守，这里不再重复探单个取值。
//
// `AT-PC-030`「不把网络服务产品当作面单渠道服务的候选」要求两格可分辨，因此这里也钉住它们
// 不同名——同名会让解析在选候选时分不出两种形态。
func TestServiceProductFormIsAFacetNotASeparateCatalog(t *testing.T) {
	live := productVersion(t, "product-facet")

	for _, form := range []domain.ServiceProductForm{domain.NetworkServiceForm, domain.LabelChannelServiceForm} {
		product, err := domain.NewServiceProduct(live, form)
		if err != nil {
			t.Fatalf("形态 %q 构造服务产品失败: %v", form, err)
		}
		if product.Form() != form {
			t.Fatalf("form = %q, want %q", product.Form(), form)
		}
		if product.Version().Kind() != domain.ServiceProductObject {
			t.Fatalf("形态 %q 的产品不由服务产品版本承载，等于它自带了第二套目录", form)
		}
	}

	if domain.NetworkServiceForm.String() == domain.LabelChannelServiceForm.String() {
		t.Fatal("两种形态同名，解析选候选时将分不出它们（AT-PC-030）")
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
