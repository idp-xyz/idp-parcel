package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func mappingSpecFixture(t *testing.T, binding domain.ProductChannelBinding) domain.ProductChannelMappingSpec {
	t.Helper()
	effective, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{},
	)
	if err != nil {
		t.Fatalf("NewEffectiveInterval：%v", err)
	}
	return domain.ProductChannelMappingSpec{
		Product:        commercialValue(t, domain.NewCommercialObjectID, "PROD-01"),
		ProductVersion: commercialValue(t, domain.NewCommercialVersionLabel, "v1"),
		Binding:        binding,
		Effective:      effective,
		Basis:          commercialValue(t, domain.NewMappingBasisReference, "BASIS-01"),
	}
}

func configuredBinding(t *testing.T, refs ...string) domain.ProductChannelBinding {
	t.Helper()
	channels := make([]domain.ChannelProductReference, 0, len(refs))
	for _, ref := range refs {
		channels = append(channels, commercialValue(t, domain.NewChannelProductReference, ref))
	}
	binding, err := domain.NewConfiguredChannelBinding(channels)
	if err != nil {
		t.Fatalf("NewConfiguredChannelBinding：%v", err)
	}
	return binding
}

// Covers: 已配置绑定的门——空集合、无效引用与重复引用都进不来（CONTEXT“一个或多个
// 渠道产品”，重复引用不构成更多候选）。
func TestAConfiguredBindingRequiresDistinctValidChannels(t *testing.T) {
	if _, err := domain.NewConfiguredChannelBinding(nil); !errors.Is(err, domain.ErrInvalidChannelBinding) {
		t.Fatalf("空集合 err = %v, want ErrInvalidChannelBinding", err)
	}
	if _, err := domain.NewConfiguredChannelBinding(
		[]domain.ChannelProductReference{{}},
	); !errors.Is(err, domain.ErrInvalidChannelBinding) {
		t.Fatalf("零值引用 err = %v, want ErrInvalidChannelBinding", err)
	}
	channel := commercialValue(t, domain.NewChannelProductReference, "CH-01")
	if _, err := domain.NewConfiguredChannelBinding(
		[]domain.ChannelProductReference{channel, channel},
	); !errors.Is(err, domain.ErrInvalidChannelBinding) {
		t.Fatalf("重复引用 err = %v, want ErrInvalidChannelBinding", err)
	}

	binding := configuredBinding(t, "CH-01", "CH-02")
	if !binding.Configured() {
		t.Fatal("两个引用的绑定应答已配置")
	}
	if channels := binding.Channels(); len(channels) != 2 {
		t.Fatalf("Channels = %d 个, want 2", len(channels))
	}
}

// Covers: 显式“未配置”是合法绑定格且不交出任何候选；零值绑定（没经两个构造门之一）
// 在登记信封上立不住——“未配置”必须是说出的话，不是忘了填的空。
func TestAnUnconfiguredBindingIsExplicitWhileZeroValueIsNot(t *testing.T) {
	unconfigured := domain.UnconfiguredChannelBinding()
	if unconfigured.Configured() {
		t.Fatal("未配置绑定不该答已配置")
	}
	if channels := unconfigured.Channels(); len(channels) != 0 {
		t.Fatalf("未配置绑定交出了 %d 个候选", len(channels))
	}

	tenant := commercialValue(t, domain.NewTenantID, "T-01")
	id := commercialValue(t, domain.NewProductChannelMappingID, "MAP-01")

	if _, err := domain.NewProductChannelMappingRegistration(
		tenant, id, 1, mappingSpecFixture(t, unconfigured),
	); err != nil {
		t.Fatalf("显式未配置的登记被拒：%v", err)
	}
	if _, err := domain.NewProductChannelMappingRegistration(
		tenant, id, 1, mappingSpecFixture(t, domain.ProductChannelBinding{}),
	); !errors.Is(err, domain.ErrInvalidMappingRegistration) {
		t.Fatalf("零值绑定 err = %v, want ErrInvalidMappingRegistration", err)
	}
}

// Covers: 登记信封的完备性门——租户、映射标识、修订号、产品引用、区间与依据缺一
// 不可；修订从 1 起。
func TestAMappingRegistrationRequiresACompleteEnvelope(t *testing.T) {
	tenant := commercialValue(t, domain.NewTenantID, "T-01")
	id := commercialValue(t, domain.NewProductChannelMappingID, "MAP-01")
	spec := mappingSpecFixture(t, configuredBinding(t, "CH-01"))

	registration, err := domain.NewProductChannelMappingRegistration(tenant, id, 1, spec)
	if err != nil {
		t.Fatalf("完整信封被拒：%v", err)
	}
	if registration.Tenant() != tenant || registration.ID() != id || registration.Revision() != 1 {
		t.Fatal("信封三键没有原样交回")
	}
	if registration.Product() != spec.Product || registration.ProductVersion() != spec.ProductVersion {
		t.Fatal("产品引用没有原样交回")
	}
	if registration.Basis() != spec.Basis || !registration.Binding().Configured() {
		t.Fatal("依据或绑定没有原样交回")
	}

	if _, err := domain.NewProductChannelMappingRegistration(
		tenant, id, 0, spec,
	); !errors.Is(err, domain.ErrInvalidMappingRegistration) {
		t.Fatalf("修订 0 err = %v, want ErrInvalidMappingRegistration", err)
	}
	broken := spec
	broken.Product = domain.CommercialObjectID{}
	if _, err := domain.NewProductChannelMappingRegistration(
		tenant, id, 1, broken,
	); !errors.Is(err, domain.ErrInvalidMappingRegistration) {
		t.Fatalf("缺产品引用 err = %v, want ErrInvalidMappingRegistration", err)
	}
	broken = spec
	broken.Effective = domain.EffectiveInterval{}
	if _, err := domain.NewProductChannelMappingRegistration(
		tenant, id, 1, broken,
	); !errors.Is(err, domain.ErrInvalidMappingRegistration) {
		t.Fatalf("缺区间 err = %v, want ErrInvalidMappingRegistration", err)
	}
	broken = spec
	broken.Basis = domain.MappingBasisReference{}
	if _, err := domain.NewProductChannelMappingRegistration(
		tenant, id, 1, broken,
	); !errors.Is(err, domain.ErrInvalidMappingRegistration) {
		t.Fatalf("缺依据 err = %v, want ErrInvalidMappingRegistration", err)
	}
	if _, err := domain.NewProductChannelMappingRegistration(
		domain.TenantID{}, id, 1, spec,
	); !errors.Is(err, domain.ErrInvalidMappingRegistration) {
		t.Fatalf("缺租户 err = %v, want ErrInvalidMappingRegistration", err)
	}
	if _, err := domain.NewProductChannelMappingRegistration(
		tenant, domain.ProductChannelMappingID{}, 1, spec,
	); !errors.Is(err, domain.ErrInvalidMappingRegistration) {
		t.Fatalf("缺映射标识 err = %v, want ErrInvalidMappingRegistration", err)
	}
}

// Covers: 绑定格的引用副本纪律——改动 Channels() 交回的切片不得触及绑定本体。
func TestBindingChannelsAreCopies(t *testing.T) {
	binding := configuredBinding(t, "CH-01", "CH-02")
	leaked := binding.Channels()
	leaked[0] = commercialValue(t, domain.NewChannelProductReference, "CH-EVIL")
	if binding.Channels()[0].String() != "CH-01" {
		t.Fatal("外部改动穿透进了绑定本体")
	}
}
