package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// fakeRegistrationNumberTypeRegistry 是注册号类型目录的内存替身：同键同内容重放、异内容冲突，
// LoadLatest 交回最高修订（判据同真适配器）。内容按摘要比而不是 reflect.DeepEqual——修订里的
// 格式带着编译好的正则，深比较比的是编译产物而不是登记原文。
type fakeRegistrationNumberTypeRegistry struct {
	rows    map[string][]domain.RegistrationNumberTypeRegistration
	loadErr error
	saves   int
}

func newFakeRegistrationNumberTypeRegistry() *fakeRegistrationNumberTypeRegistry {
	return &fakeRegistrationNumberTypeRegistry{rows: make(map[string][]domain.RegistrationNumberTypeRegistration)}
}

func registrationNumberTypeKey(
	tenant domain.TenantID,
	country domain.RegistrationCountryCode,
	code domain.RegistrationNumberTypeCode,
) string {
	return tenant.String() + "/" + country.String() + "/" + code.String()
}

func registrationNumberTypeContent(registration domain.RegistrationNumberTypeRegistration) string {
	basis, at, has := registration.Lifecycle().Deactivation()
	return fmt.Sprintf("%s|%s|%s|%s|%v|%s|%v|%v", registration.Name(), registration.Layer(),
		registration.Format().Pattern(), registration.Basis(), registration.Lifecycle().EffectiveFrom(),
		basis, at, has)
}

func (fake *fakeRegistrationNumberTypeRegistry) SaveRegistrationNumberType(
	_ context.Context,
	registration domain.RegistrationNumberTypeRegistration,
) (ports.RegistrationNumberTypeSaveOutcome, error) {
	fake.saves++
	key := registrationNumberTypeKey(registration.Tenant(), registration.Country(), registration.Code())
	for _, existing := range fake.rows[key] {
		if existing.Revision() != registration.Revision() {
			continue
		}
		if registrationNumberTypeContent(existing) == registrationNumberTypeContent(registration) {
			return ports.RegistrationNumberTypeRegistryAlreadyRegistered, nil
		}
		return ports.RegistrationNumberTypeRegistryContentConflict, nil
	}
	fake.rows[key] = append(fake.rows[key], registration)
	return ports.RegistrationNumberTypeRegistrySaved, nil
}

func (fake *fakeRegistrationNumberTypeRegistry) LoadLatestRegistrationNumberType(
	_ context.Context,
	tenant domain.TenantID,
	country domain.RegistrationCountryCode,
	code domain.RegistrationNumberTypeCode,
) (domain.RegistrationNumberTypeRegistration, bool, error) {
	if fake.loadErr != nil {
		return domain.RegistrationNumberTypeRegistration{}, false, fake.loadErr
	}
	var latest domain.RegistrationNumberTypeRegistration
	found := false
	for _, existing := range fake.rows[registrationNumberTypeKey(tenant, country, code)] {
		if !found || existing.Revision() > latest.Revision() {
			latest = existing
			found = true
		}
	}
	return latest, found, nil
}

var registrationTypeEffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func registerRegistrationTypeCommand(
	t *testing.T,
	revision int,
	layer domain.RegistrationNumberLayer,
	pattern string,
) application.RegisterRegistrationNumberTypeCommand {
	t.Helper()
	format, err := domain.NewRegistrationNumberFormat(pattern)
	if err != nil {
		t.Fatalf("new format %q: %v", pattern, err)
	}
	return application.RegisterRegistrationNumberTypeCommand{
		Tenant:   value(t, domain.NewTenantID, "tenant-1"),
		Country:  value(t, domain.NewRegistrationCountryCode, "XA"),
		Code:     value(t, domain.NewRegistrationNumberTypeCode, "SYN-LIFETIME"),
		Revision: revision,
		Spec: domain.RegistrationNumberTypeSpec{
			Name:   value(t, domain.NewRegistrationNumberTypeName, "合成终身注册号"),
			Layer:  layer,
			Format: format,
			Basis:  value(t, domain.NewRegistrationNumberTypeBasisReference, fmt.Sprintf("SYN-BASIS-r%d", revision)),
		},
		EffectiveFrom: registrationTypeEffectiveFrom,
	}
}

func deactivateRegistrationTypeCommand(
	t *testing.T,
	revision int,
	basis string,
	at time.Time,
) application.DeactivateRegistrationNumberTypeCommand {
	t.Helper()
	return application.DeactivateRegistrationNumberTypeCommand{
		Tenant:   value(t, domain.NewTenantID, "tenant-1"),
		Country:  value(t, domain.NewRegistrationCountryCode, "XA"),
		Code:     value(t, domain.NewRegistrationNumberTypeCode, "SYN-LIFETIME"),
		Revision: revision,
		Basis:    value(t, domain.NewRegistrationNumberTypeBasisReference, basis),
		At:       at,
	}
}

func mustRegisterRegistrationType(
	t *testing.T,
	handler *application.RegisterRegistrationNumberTypeHandler,
	command application.RegisterRegistrationNumberTypeCommand,
) application.RegistrationNumberTypeResult {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("Register r%d：%v", command.Revision, err)
	}
	return result
}

func mustDeactivateRegistrationType(
	t *testing.T,
	handler *application.RegisterRegistrationNumberTypeHandler,
	command application.DeactivateRegistrationNumberTypeCommand,
) application.RegistrationNumberTypeResult {
	t.Helper()
	result, err := handler.Deactivate(t.Context(), command)
	if err != nil {
		t.Fatalf("Deactivate r%d：%v", command.Revision, err)
	}
	return result
}

// Covers: 登记修订连续性——首笔只收修订 1、重放答重复、同修订异内容答冲突、跳号不受理；内容更正
// 占下一修订号；新修订改层不受理（层钉在类型上，改层是另一个类型）。
func TestRegisterRegistrationNumberTypeKeepsRevisionsContinuousAndLayerPinned(t *testing.T) {
	registry := newFakeRegistrationNumberTypeRegistry()
	handler := application.NewRegisterRegistrationNumberTypeHandler(registry)
	identity := domain.RegistrationNumberIdentityLayer

	if result := mustRegisterRegistrationType(t, handler, registerRegistrationTypeCommand(t, 2, identity, `SYN-[0-9]{6}`)); result.Outcome() != application.RegistrationNumberTypeNotAccepted ||
		!strings.Contains(result.Cause().Error(), "首笔") {
		t.Fatalf("从未登记即收修订 2 = %s（%v），want NOT_ACCEPTED 首笔", result.Outcome(), result.Cause())
	}
	first := registerRegistrationTypeCommand(t, 1, identity, `SYN-[0-9]{6}`)
	if result := mustRegisterRegistrationType(t, handler, first); result.Outcome() != application.RegistrationNumberTypeRegistered {
		t.Fatalf("修订 1 = %s（%v），want REGISTERED", result.Outcome(), result.Cause())
	}
	if result := mustRegisterRegistrationType(t, handler, first); result.Outcome() != application.RegistrationNumberTypeAlreadyRegistered {
		t.Fatalf("重放修订 1 = %s，want ALREADY_REGISTERED", result.Outcome())
	}
	if result := mustRegisterRegistrationType(t, handler, registerRegistrationTypeCommand(t, 1, identity, `SYN-[0-9]{7}`)); result.Outcome() != application.RegistrationNumberTypeContentConflict {
		t.Fatalf("同修订异内容 = %s，want CONTENT_CONFLICT", result.Outcome())
	}
	if result := mustRegisterRegistrationType(t, handler, registerRegistrationTypeCommand(t, 3, identity, `SYN-[0-9]{8}`)); result.Outcome() != application.RegistrationNumberTypeNotAccepted ||
		!strings.Contains(result.Cause().Error(), "连续") {
		t.Fatalf("跳号 = %s（%v），want NOT_ACCEPTED 连续", result.Outcome(), result.Cause())
	}
	relayered := registerRegistrationTypeCommand(t, 2, domain.RegistrationNumberProfileLayer, `SYN-[0-9]{8}`)
	if result := mustRegisterRegistrationType(t, handler, relayered); result.Outcome() != application.RegistrationNumberTypeNotAccepted ||
		!strings.Contains(result.Cause().Error(), "不改层") {
		t.Fatalf("新修订改层 = %s（%v），want NOT_ACCEPTED 不改层", result.Outcome(), result.Cause())
	}
	if result := mustRegisterRegistrationType(t, handler, registerRegistrationTypeCommand(t, 2, identity, `SYN-[0-9]{8}`)); result.Outcome() != application.RegistrationNumberTypeRegistered {
		t.Fatalf("内容更正修订 2 = %s（%v），want REGISTERED", result.Outcome(), result.Cause())
	}
}

// Covers: 领域门在触库之前拒——缺格式、缺生效时点的命令答未受理，一次写口都没碰。
func TestRegisterRegistrationNumberTypeRefusesAtTheDomainGateWithoutWriting(t *testing.T) {
	registry := newFakeRegistrationNumberTypeRegistry()
	handler := application.NewRegisterRegistrationNumberTypeHandler(registry)

	withoutFormat := registerRegistrationTypeCommand(t, 1, domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`)
	withoutFormat.Spec.Format = domain.RegistrationNumberFormat{}
	withoutEffective := registerRegistrationTypeCommand(t, 1, domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`)
	withoutEffective.EffectiveFrom = time.Time{}

	for name, command := range map[string]application.RegisterRegistrationNumberTypeCommand{
		"缺格式":   withoutFormat,
		"缺生效时点": withoutEffective,
	} {
		if result := mustRegisterRegistrationType(t, handler, command); result.Outcome() != application.RegistrationNumberTypeNotAccepted || result.Cause() == nil {
			t.Fatalf("%s = %s（%v），want NOT_ACCEPTED 带原因", name, result.Outcome(), result.Cause())
		}
	}
	if registry.saves != 0 {
		t.Fatalf("领域门拒绝后写口被调用 %d 次，want 0", registry.saves)
	}
}

// Covers: 停用——从未登记答未找到；修订错位不受理；正确落点成下一修订；撞上已停用册面时同修订同
// 内容答重复、同修订异内容答冲突；已停用的类型不再收登记修订。
func TestDeactivateRegistrationNumberTypeFormsTheNextRevisionOnce(t *testing.T) {
	registry := newFakeRegistrationNumberTypeRegistry()
	handler := application.NewRegisterRegistrationNumberTypeHandler(registry)
	retiredAt := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	if result := mustDeactivateRegistrationType(t, handler, deactivateRegistrationTypeCommand(t, 2, "SYN-BASIS-RETIRE", retiredAt)); result.Outcome() != application.RegistrationNumberTypeNotFound {
		t.Fatalf("停用从未登记的类型 = %s，want NOT_FOUND", result.Outcome())
	}
	if result := mustRegisterRegistrationType(t, handler, registerRegistrationTypeCommand(t, 1, domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`)); result.Outcome() != application.RegistrationNumberTypeRegistered {
		t.Fatalf("修订 1 = %s", result.Outcome())
	}
	if result := mustDeactivateRegistrationType(t, handler, deactivateRegistrationTypeCommand(t, 3, "SYN-BASIS-RETIRE", retiredAt)); result.Outcome() != application.RegistrationNumberTypeNotAccepted ||
		!strings.Contains(result.Cause().Error(), "错位") {
		t.Fatalf("停用落点错位 = %s（%v），want NOT_ACCEPTED 错位", result.Outcome(), result.Cause())
	}
	deactivation := deactivateRegistrationTypeCommand(t, 2, "SYN-BASIS-RETIRE", retiredAt)
	if result := mustDeactivateRegistrationType(t, handler, deactivation); result.Outcome() != application.RegistrationNumberTypeDeactivated {
		t.Fatalf("停用 = %s（%v），want DEACTIVATED", result.Outcome(), result.Cause())
	}
	if result := mustDeactivateRegistrationType(t, handler, deactivation); result.Outcome() != application.RegistrationNumberTypeAlreadyRegistered {
		t.Fatalf("重放停用 = %s，want ALREADY_REGISTERED", result.Outcome())
	}
	if result := mustDeactivateRegistrationType(t, handler, deactivateRegistrationTypeCommand(t, 2, "SYN-BASIS-OTHER", retiredAt)); result.Outcome() != application.RegistrationNumberTypeContentConflict {
		t.Fatalf("同修订异依据停用 = %s，want CONTENT_CONFLICT", result.Outcome())
	}
	if result := mustDeactivateRegistrationType(t, handler, deactivateRegistrationTypeCommand(t, 3, "SYN-BASIS-RETIRE", retiredAt)); result.Outcome() != application.RegistrationNumberTypeNotAccepted {
		t.Fatalf("对已停用类型再落停用 = %s，want NOT_ACCEPTED", result.Outcome())
	}
	if result := mustRegisterRegistrationType(t, handler, registerRegistrationTypeCommand(t, 3, domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`)); result.Outcome() != application.RegistrationNumberTypeNotAccepted ||
		!strings.Contains(result.Cause().Error(), "已停用") {
		t.Fatalf("已停用类型收登记修订 = %s（%v），want NOT_ACCEPTED 已停用", result.Outcome(), result.Cause())
	}
}

// Covers: 册面读不回是技术失败照原样上抛，不折成「从未登记」——那会让操作者从修订 1 重登一个本来
// 就在的类型；停用命令缺键在触库前答未受理。
func TestRegisterRegistrationNumberTypePropagatesLoadFailures(t *testing.T) {
	registry := newFakeRegistrationNumberTypeRegistry()
	registry.loadErr = errors.New("database unavailable")
	handler := application.NewRegisterRegistrationNumberTypeHandler(registry)

	if _, err := handler.Register(t.Context(), registerRegistrationTypeCommand(t, 1, domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`)); err == nil {
		t.Fatalf("登记遇读失败应上抛 error")
	}
	if _, err := handler.Deactivate(t.Context(), deactivateRegistrationTypeCommand(t, 2, "SYN-BASIS-RETIRE", registrationTypeEffectiveFrom)); err == nil {
		t.Fatalf("停用遇读失败应上抛 error")
	}
	result, err := handler.Deactivate(t.Context(), application.DeactivateRegistrationNumberTypeCommand{Revision: 2})
	if err != nil || result.Outcome() != application.RegistrationNumberTypeNotAccepted {
		t.Fatalf("缺键停用 = (%s, %v)，want (NOT_ACCEPTED, nil)", result.Outcome(), err)
	}
}
