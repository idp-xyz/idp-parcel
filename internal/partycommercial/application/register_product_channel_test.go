package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// pcmPublicationsDouble 实现 ports.ServiceProductFormRegistry：整册装载答预置册面，
// 形态写入记录收到的产品并答预置落点。记录被问到的租户与范围——「不跨租户读取」
// 只有在端口实际收到的参数上才验得出来（判据同 authorityDouble）。
type pcmPublicationsDouble struct {
	registry    *domain.CommercialRegistry
	loadErr     error
	askedTenant domain.TenantID
	askedScope  domain.CommercialScopeReference
	saved       []domain.ServiceProduct
	saveOutcome ports.ServiceProductSaveOutcome
	saveErr     error
}

func (double *pcmPublicationsDouble) LoadForScope(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	double.askedTenant = tenant
	double.askedScope = scope
	if double.loadErr != nil {
		return nil, double.loadErr
	}
	return double.registry, nil
}

func (double *pcmPublicationsDouble) SaveServiceProduct(
	_ context.Context,
	product domain.ServiceProduct,
) (ports.ServiceProductSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.ServiceProductSaveOutcomeInvalid, double.saveErr
	}
	double.saved = append(double.saved, product)
	if double.saveOutcome == ports.ServiceProductSaveOutcomeInvalid {
		return ports.ServiceProductSaved, nil
	}
	return double.saveOutcome, nil
}

// fakeMappingRegistry 是映射登记册的内存替身：同键同内容重放、异内容冲突，
// LoadLatestMapping 交回最高修订（判据同真适配器）。
type fakeMappingRegistry struct {
	rows    map[string][]domain.ProductChannelMappingRegistration
	loadErr error
	saveErr error
}

func newFakeMappingRegistry() *fakeMappingRegistry {
	return &fakeMappingRegistry{rows: make(map[string][]domain.ProductChannelMappingRegistration)}
}

func mappingKey(tenant domain.TenantID, id domain.ProductChannelMappingID) string {
	return tenant.String() + "/" + id.String()
}

func (fake *fakeMappingRegistry) SaveMapping(
	_ context.Context,
	registration domain.ProductChannelMappingRegistration,
) (ports.MappingSaveOutcome, error) {
	if fake.saveErr != nil {
		return ports.MappingSaveOutcomeInvalid, fake.saveErr
	}
	key := mappingKey(registration.Tenant(), registration.ID())
	for _, existing := range fake.rows[key] {
		if existing.Revision() != registration.Revision() {
			continue
		}
		if reflect.DeepEqual(existing, registration) {
			return ports.MappingAlreadyRegistered, nil
		}
		return ports.MappingContentConflict, nil
	}
	fake.rows[key] = append(fake.rows[key], registration)
	return ports.MappingSaved, nil
}

func (fake *fakeMappingRegistry) LoadLatestMapping(
	_ context.Context,
	tenant domain.TenantID,
	id domain.ProductChannelMappingID,
) (domain.ProductChannelMappingRegistration, bool, error) {
	if fake.loadErr != nil {
		return domain.ProductChannelMappingRegistration{}, false, fake.loadErr
	}
	var latest domain.ProductChannelMappingRegistration
	found := false
	for _, existing := range fake.rows[mappingKey(tenant, id)] {
		if !found || existing.Revision() > latest.Revision() {
			latest = existing
			found = true
		}
	}
	return latest, found, nil
}

// pcmProductVersion 造一份服务产品版本并走到指定生命周期格。走真转换，不重建。
func pcmProductVersion(t *testing.T, objectID, versionLabel, scope, stage string) domain.CommercialVersion {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	draft, err := domain.NewCommercialDraft(domain.CommercialVersionSpec{
		TenantID:      value(t, domain.NewTenantID, "tenant-1"),
		Kind:          domain.ServiceProductObject,
		ObjectID:      value(t, domain.NewCommercialObjectID, objectID),
		Version:       value(t, domain.NewCommercialVersionLabel, versionLabel),
		Scope:         value(t, domain.NewCommercialScopeReference, scope),
		ContentDigest: value(t, domain.NewCommercialContentDigest, "sha256:"+objectID+"-"+versionLabel),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := domain.NewApprovalBasis(
		value(t, domain.NewApprovalReference, "approval-"+objectID),
		value(t, domain.NewCommercialSourceReference, "source-"+objectID),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("new approval basis: %v", err)
	}
	published, err := draft.Publish(basis, domain.ApprovalRoleConfirmed, approvedAt, nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if stage == "published" {
		return published
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if stage == "effective" {
		return live
	}
	retired, err := live.Retire(
		value(t, domain.NewRetirementReference, "retire-"+objectID),
		approvedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	return retired
}

func pcmRegistryWith(t *testing.T, versions ...domain.CommercialVersion) *domain.CommercialRegistry {
	t.Helper()
	registry := domain.NewCommercialRegistry()
	for _, version := range versions {
		if _, err := registry.Register(version); err != nil {
			t.Fatalf("register version: %v", err)
		}
	}
	return registry
}

func pcmMappingCommand(t *testing.T, revision int, binding domain.ProductChannelBinding) application.RegisterProductChannelMappingCommand {
	t.Helper()
	effective, err := domain.NewEffectiveInterval(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	return application.RegisterProductChannelMappingCommand{
		Tenant:   value(t, domain.NewTenantID, "tenant-1"),
		Scope:    value(t, domain.NewCommercialScopeReference, "scope-a"),
		ID:       value(t, domain.NewProductChannelMappingID, "MAP-1"),
		Revision: revision,
		Spec: domain.ProductChannelMappingSpec{
			Product:        value(t, domain.NewCommercialObjectID, "product-1"),
			ProductVersion: value(t, domain.NewCommercialVersionLabel, "v1"),
			Binding:        binding,
			Effective:      effective,
			Basis:          value(t, domain.NewMappingBasisReference, "basis-map-1"),
		},
	}
}

func pcmConfiguredBinding(t *testing.T, refs ...string) domain.ProductChannelBinding {
	t.Helper()
	channels := make([]domain.ChannelProductReference, 0, len(refs))
	for _, ref := range refs {
		channels = append(channels, value(t, domain.NewChannelProductReference, ref))
	}
	binding, err := domain.NewConfiguredChannelBinding(channels)
	if err != nil {
		t.Fatalf("new configured binding: %v", err)
	}
	return binding
}

// Covers: 形态登记的写路——版本从册上取回、经 NewServiceProduct 构造、落进形态册；
// 装载收到的是命令声明的租户与范围（ADR-0003）。
func TestRegisteringAServiceProductFormLandsTheConstructedProduct(t *testing.T) {
	double := &pcmPublicationsDouble{
		registry: pcmRegistryWith(t, pcmProductVersion(t, "product-1", "v1", "scope-a", "effective")),
	}
	handler := application.NewRegisterProductChannelHandler(double, newFakeMappingRegistry())

	result, err := handler.RegisterServiceProductForm(context.Background(), application.RegisterServiceProductFormCommand{
		Tenant:   value(t, domain.NewTenantID, "tenant-1"),
		Scope:    value(t, domain.NewCommercialScopeReference, "scope-a"),
		ObjectID: value(t, domain.NewCommercialObjectID, "product-1"),
		Version:  value(t, domain.NewCommercialVersionLabel, "v1"),
		Form:     domain.NetworkServiceForm,
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.ProductChannelRegistered {
		t.Fatalf("outcome = %s, want REGISTERED（原因：%v）", result.Outcome(), result.Cause())
	}
	if double.askedTenant.String() != "tenant-1" || double.askedScope.String() != "scope-a" {
		t.Fatalf("装载参数 = %s/%s, want tenant-1/scope-a", double.askedTenant, double.askedScope)
	}
	if len(double.saved) != 1 {
		t.Fatalf("落库 %d 份形态, want 1", len(double.saved))
	}
	saved := double.saved[0]
	if saved.Form() != domain.NetworkServiceForm || saved.Version().ObjectID().String() != "product-1" {
		t.Fatalf("落库的产品不对：form=%s object=%s", saved.Form(), saved.Version().ObjectID())
	}
}

// Covers: 形态不钉悬空引用；未生效版本过不了形态构造门（ADR-0050 装载侧同一条裁决），
// 原因里点名版本状态。重放与冲突照登记册的落点转写，不在用例层再判一次。
func TestServiceProductFormRegistrationRefusesDanglingOrIneffectiveVersions(t *testing.T) {
	command := application.RegisterServiceProductFormCommand{
		Tenant:   value(t, domain.NewTenantID, "tenant-1"),
		Scope:    value(t, domain.NewCommercialScopeReference, "scope-a"),
		ObjectID: value(t, domain.NewCommercialObjectID, "product-1"),
		Version:  value(t, domain.NewCommercialVersionLabel, "v1"),
		Form:     domain.NetworkServiceForm,
	}

	t.Run("dangling", func(t *testing.T) {
		double := &pcmPublicationsDouble{registry: pcmRegistryWith(t)}
		handler := application.NewRegisterProductChannelHandler(double, newFakeMappingRegistry())
		result, err := handler.RegisterServiceProductForm(context.Background(), command)
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.ProductChannelNotAccepted || result.Cause() == nil {
			t.Fatalf("outcome = %s（原因 %v）, want NOT_ACCEPTED 携原因", result.Outcome(), result.Cause())
		}
		if len(double.saved) != 0 {
			t.Fatal("被拒的登记不该落库")
		}
	})

	t.Run("published but not yet effective", func(t *testing.T) {
		double := &pcmPublicationsDouble{
			registry: pcmRegistryWith(t, pcmProductVersion(t, "product-1", "v1", "scope-a", "published")),
		}
		handler := application.NewRegisterProductChannelHandler(double, newFakeMappingRegistry())
		result, err := handler.RegisterServiceProductForm(context.Background(), command)
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.ProductChannelNotAccepted ||
			result.Cause() == nil || !strings.Contains(result.Cause().Error(), "PUBLISHED") {
			t.Fatalf("outcome = %s（原因 %v）, want NOT_ACCEPTED 且原因点名状态", result.Outcome(), result.Cause())
		}
	})

	t.Run("replay and conflict pass through", func(t *testing.T) {
		for outcome, want := range map[ports.ServiceProductSaveOutcome]application.ProductChannelOutcome{
			ports.ServiceProductAlreadyRegistered: application.ProductChannelAlreadyRegistered,
			ports.ServiceProductContentConflict:   application.ProductChannelContentConflict,
		} {
			double := &pcmPublicationsDouble{
				registry:    pcmRegistryWith(t, pcmProductVersion(t, "product-1", "v1", "scope-a", "effective")),
				saveOutcome: outcome,
			}
			handler := application.NewRegisterProductChannelHandler(double, newFakeMappingRegistry())
			result, err := handler.RegisterServiceProductForm(context.Background(), command)
			if err != nil {
				t.Fatalf("Handle：%v", err)
			}
			if result.Outcome() != want {
				t.Fatalf("outcome = %s, want %s", result.Outcome(), want)
			}
		}
	})
}

// Covers: 映射登记的写路——已配置与显式未配置两种绑定都能落库；已计划生效
// （PUBLISHED）的版本可以先配映射（渠道候选在产品开卖前备好是常态）。
func TestRegisteringMappingsAcceptsConfiguredAndUnconfiguredBindings(t *testing.T) {
	mappings := newFakeMappingRegistry()
	double := &pcmPublicationsDouble{
		registry: pcmRegistryWith(t, pcmProductVersion(t, "product-1", "v1", "scope-a", "published")),
	}
	handler := application.NewRegisterProductChannelHandler(double, mappings)

	configured := pcmMappingCommand(t, 1, pcmConfiguredBinding(t, "CH-01", "CH-02"))
	result, err := handler.RegisterMapping(context.Background(), configured)
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.ProductChannelRegistered {
		t.Fatalf("outcome = %s（原因 %v）, want REGISTERED", result.Outcome(), result.Cause())
	}

	unconfigured := pcmMappingCommand(t, 2, domain.UnconfiguredChannelBinding())
	result, err = handler.RegisterMapping(context.Background(), unconfigured)
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.ProductChannelRegistered {
		t.Fatalf("outcome = %s（原因 %v）, want REGISTERED", result.Outcome(), result.Cause())
	}

	latest, found, err := mappings.LoadLatestMapping(
		context.Background(),
		value(t, domain.NewTenantID, "tenant-1"),
		value(t, domain.NewProductChannelMappingID, "MAP-1"),
	)
	if err != nil || !found {
		t.Fatalf("册上取不回：found=%v err=%v", found, err)
	}
	if latest.Revision() != 2 || latest.Binding().Configured() {
		t.Fatalf("最新修订 = %d（configured=%v）, want 2 且未配置", latest.Revision(), latest.Binding().Configured())
	}
}

// Covers: 映射不钉悬空引用；已收尾版本不再支持新的映射登记（CONTEXT「退役只停止参与
// 新的商业选择」——新映射正是一次新的商业选择）；零值绑定在领域门被拒。
func TestMappingRegistrationRefusesDanglingClosedOrUndeclaredInput(t *testing.T) {
	t.Run("dangling product", func(t *testing.T) {
		handler := application.NewRegisterProductChannelHandler(
			&pcmPublicationsDouble{registry: pcmRegistryWith(t)}, newFakeMappingRegistry(),
		)
		result, err := handler.RegisterMapping(context.Background(),
			pcmMappingCommand(t, 1, pcmConfiguredBinding(t, "CH-01")))
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.ProductChannelNotAccepted || result.Cause() == nil {
			t.Fatalf("outcome = %s（原因 %v）, want NOT_ACCEPTED 携原因", result.Outcome(), result.Cause())
		}
	})

	t.Run("retired product", func(t *testing.T) {
		handler := application.NewRegisterProductChannelHandler(
			&pcmPublicationsDouble{
				registry: pcmRegistryWith(t, pcmProductVersion(t, "product-1", "v1", "scope-a", "retired")),
			}, newFakeMappingRegistry(),
		)
		result, err := handler.RegisterMapping(context.Background(),
			pcmMappingCommand(t, 1, pcmConfiguredBinding(t, "CH-01")))
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.ProductChannelNotAccepted ||
			result.Cause() == nil || !strings.Contains(result.Cause().Error(), "RETIRED") {
			t.Fatalf("outcome = %s（原因 %v）, want NOT_ACCEPTED 且原因点名状态", result.Outcome(), result.Cause())
		}
	})

	t.Run("undeclared binding", func(t *testing.T) {
		handler := application.NewRegisterProductChannelHandler(
			&pcmPublicationsDouble{
				registry: pcmRegistryWith(t, pcmProductVersion(t, "product-1", "v1", "scope-a", "effective")),
			}, newFakeMappingRegistry(),
		)
		result, err := handler.RegisterMapping(context.Background(),
			pcmMappingCommand(t, 1, domain.ProductChannelBinding{}))
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.ProductChannelNotAccepted ||
			!errors.Is(result.Cause(), domain.ErrInvalidMappingRegistration) {
			t.Fatalf("outcome = %s（原因 %v）, want NOT_ACCEPTED（领域门拒绝）", result.Outcome(), result.Cause())
		}
	})
}

// Covers: 修订连续性与登记册的重放/冲突答法——首笔必须是 1、跳号拒绝、同修订同内容
// 重放、同修订异内容冲突；映射标识钉着产品版本，改指产品是另一笔映射。
func TestMappingRevisionsStayContinuousAndPinned(t *testing.T) {
	mappings := newFakeMappingRegistry()
	registry := pcmRegistryWith(t,
		pcmProductVersion(t, "product-1", "v1", "scope-a", "effective"),
		pcmProductVersion(t, "product-2", "v1", "scope-a", "effective"),
	)
	handler := application.NewRegisterProductChannelHandler(
		&pcmPublicationsDouble{registry: registry}, mappings,
	)

	first := pcmMappingCommand(t, 3, pcmConfiguredBinding(t, "CH-01"))
	result, err := handler.RegisterMapping(context.Background(), first)
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.ProductChannelNotAccepted {
		t.Fatalf("首笔修订 3 outcome = %s, want NOT_ACCEPTED", result.Outcome())
	}

	first.Revision = 1
	if result, err = handler.RegisterMapping(context.Background(), first); err != nil ||
		result.Outcome() != application.ProductChannelRegistered {
		t.Fatalf("首笔修订 1：outcome = %s err = %v", result.Outcome(), err)
	}

	replay := first
	if result, err = handler.RegisterMapping(context.Background(), replay); err != nil ||
		result.Outcome() != application.ProductChannelAlreadyRegistered {
		t.Fatalf("重放：outcome = %s err = %v, want ALREADY_REGISTERED", result.Outcome(), err)
	}

	conflicting := pcmMappingCommand(t, 1, pcmConfiguredBinding(t, "CH-99"))
	if result, err = handler.RegisterMapping(context.Background(), conflicting); err != nil ||
		result.Outcome() != application.ProductChannelContentConflict {
		t.Fatalf("同修订异内容：outcome = %s err = %v, want CONTENT_CONFLICT", result.Outcome(), err)
	}

	skipped := pcmMappingCommand(t, 3, pcmConfiguredBinding(t, "CH-02"))
	if result, err = handler.RegisterMapping(context.Background(), skipped); err != nil ||
		result.Outcome() != application.ProductChannelNotAccepted {
		t.Fatalf("跳号：outcome = %s err = %v, want NOT_ACCEPTED", result.Outcome(), err)
	}

	repointed := pcmMappingCommand(t, 2, pcmConfiguredBinding(t, "CH-01"))
	repointed.Spec.Product = value(t, domain.NewCommercialObjectID, "product-2")
	result, err = handler.RegisterMapping(context.Background(), repointed)
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.ProductChannelNotAccepted ||
		result.Cause() == nil || !strings.Contains(result.Cause().Error(), "另一笔映射") {
		t.Fatalf("改指产品：outcome = %s（原因 %v）, want NOT_ACCEPTED", result.Outcome(), result.Cause())
	}

	next := pcmMappingCommand(t, 2, pcmConfiguredBinding(t, "CH-01", "CH-03"))
	if result, err = handler.RegisterMapping(context.Background(), next); err != nil ||
		result.Outcome() != application.ProductChannelRegistered {
		t.Fatalf("紧邻下一号：outcome = %s err = %v, want REGISTERED", result.Outcome(), err)
	}
}

// Covers: 登记是写权威的动作——整册或映射册读不回时不得闭眼登记，照原样上抛等重试。
func TestProductChannelRegistrationSurfacesReadFailures(t *testing.T) {
	boom := errors.New("库不可用")

	handler := application.NewRegisterProductChannelHandler(
		&pcmPublicationsDouble{loadErr: boom}, newFakeMappingRegistry(),
	)
	if _, err := handler.RegisterServiceProductForm(context.Background(), application.RegisterServiceProductFormCommand{
		Tenant:   value(t, domain.NewTenantID, "tenant-1"),
		Scope:    value(t, domain.NewCommercialScopeReference, "scope-a"),
		ObjectID: value(t, domain.NewCommercialObjectID, "product-1"),
		Version:  value(t, domain.NewCommercialVersionLabel, "v1"),
		Form:     domain.NetworkServiceForm,
	}); !errors.Is(err, boom) {
		t.Fatalf("形态登记 err = %v, want 上抛装载失败", err)
	}
	if _, err := handler.RegisterMapping(context.Background(),
		pcmMappingCommand(t, 1, pcmConfiguredBinding(t, "CH-01"))); !errors.Is(err, boom) {
		t.Fatalf("映射登记 err = %v, want 上抛装载失败", err)
	}

	brokenMappings := newFakeMappingRegistry()
	brokenMappings.loadErr = boom
	handler = application.NewRegisterProductChannelHandler(
		&pcmPublicationsDouble{
			registry: pcmRegistryWith(t, pcmProductVersion(t, "product-1", "v1", "scope-a", "effective")),
		}, brokenMappings,
	)
	if _, err := handler.RegisterMapping(context.Background(),
		pcmMappingCommand(t, 1, pcmConfiguredBinding(t, "CH-01"))); !errors.Is(err, boom) {
		t.Fatalf("映射册读失败 err = %v, want 上抛", err)
	}
}
