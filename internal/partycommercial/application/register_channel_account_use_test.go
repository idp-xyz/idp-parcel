package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// channelAccountUseRegistryStub 是登记册的内存替身：只记最新修订，够本用例的连续性与承继
// 检查用。它不模仿撞键代数——那一格由真库适配器测试覆盖，在这里模仿等于测替身。
type channelAccountUseRegistryStub struct {
	latest map[string]domain.ChannelAccountUseAuthorizationRegistration
	saved  []domain.ChannelAccountUseAuthorizationRegistration
	err    error
}

func newChannelAccountUseRegistryStub() *channelAccountUseRegistryStub {
	return &channelAccountUseRegistryStub{
		latest: map[string]domain.ChannelAccountUseAuthorizationRegistration{},
	}
}

func (stub *channelAccountUseRegistryStub) SaveChannelAccountUse(
	_ context.Context,
	registration domain.ChannelAccountUseAuthorizationRegistration,
) (ports.ChannelAccountUseSaveOutcome, error) {
	if stub.err != nil {
		return ports.ChannelAccountUseSaveOutcomeInvalid, stub.err
	}
	stub.saved = append(stub.saved, registration)
	stub.latest[registration.ID().String()] = registration
	return ports.ChannelAccountUseSaved, nil
}

func (stub *channelAccountUseRegistryStub) LoadLatest(
	_ context.Context,
	_ domain.TenantID,
	authorization domain.ChannelAccountUseAuthorizationID,
) (domain.ChannelAccountUseAuthorizationRegistration, bool, error) {
	if stub.err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, false, stub.err
	}
	registration, found := stub.latest[authorization.String()]
	return registration, found, nil
}

func (stub *channelAccountUseRegistryStub) LoadAuthorizedAccountUse(
	_ context.Context,
	_ domain.TenantID,
	_ domain.ChannelAccountID,
) ([]domain.ChannelAccountUseAuthorizationRegistration, error) {
	return nil, nil
}

var _ ports.ChannelAccountUseAuthorizationRegistry = (*channelAccountUseRegistryStub)(nil)

func channelUseValue[T any](t *testing.T, make func(string) (T, error), value string) T {
	t.Helper()
	built, err := make(value)
	if err != nil {
		t.Fatalf("build %q: %v", value, err)
	}
	return built
}

func registerChannelUseCommand(t *testing.T, grantee string, revision int) application.RegisterChannelAccountUseCommand {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{},
	)
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	return application.RegisterChannelAccountUseCommand{
		Tenant:      channelUseValue(t, domain.NewTenantID, "tenant-1"),
		ID:          channelUseValue(t, domain.NewChannelAccountUseAuthorizationID, "cauth-1"),
		Revision:    revision,
		Account:     channelUseValue(t, domain.NewChannelAccountID, "ACCT-1"),
		Grantor:     channelUseValue(t, domain.NewPartyID, "holder-1"),
		Grantee:     channelUseValue(t, domain.NewPartyID, grantee),
		Channel:     channelUseValue(t, domain.NewChannelProductReference, "CH-SG-POST"),
		Scope:       channelUseValue(t, domain.NewCommercialScopeReference, "scope-a"),
		Effective:   interval,
		Technical:   domain.ChannelAccountTechnicallyAvailable,
		Business:    domain.ChannelAccountBusinessAuthorized,
		PublishedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// Covers: ADR-0039 「取得账号凭据不等于取得业务使用授权」在写侧编排上的落点。
//
// 业务未授权自成一格，不并进`未受理`：两者的恢复动作相反。`未受理`要商业责任方改输入；
// 业务未授权改多少遍输入都不会成功，要去取得持有人的授权。压成一格之后调用方会去查自己
// 填的字段，而那件事本来就没错。
func TestBusinessUnauthorizedIsItsOwnGradeNotARejectedInput(t *testing.T) {
	registry := newChannelAccountUseRegistryStub()
	handler := application.NewRegisterChannelAccountUseHandler(registry)

	command := registerChannelUseCommand(t, "operator-1", 1)
	command.Business = domain.ChannelAccountBusinessUnauthorized

	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.Outcome() != application.ChannelAccountUseBusinessUnauthorized {
		t.Fatalf("outcome = %s, want BUSINESS_UNAUTHORIZED", result.Outcome())
	}
	if !errors.Is(result.Cause(), domain.ErrChannelAccountBusinessUnauthorized) {
		t.Fatalf("cause = %v, want ErrChannelAccountBusinessUnauthorized", result.Cause())
	}
	if len(registry.saved) != 0 {
		t.Fatal("业务未授权仍往登记册写了行")
	}

	// 对照：字段不全走另一格，两格不可互相冒充。
	broken := registerChannelUseCommand(t, "operator-1", 1)
	broken.Account = domain.ChannelAccountID{}
	rejected, err := handler.Register(t.Context(), broken)
	if err != nil {
		t.Fatalf("register broken: %v", err)
	}
	if rejected.Outcome() != application.ChannelAccountUseNotAccepted {
		t.Fatalf("outcome = %s, want NOT_ACCEPTED", rejected.Outcome())
	}
}

// Covers: ADR-0093 决定六在写侧的落点——后继修订不得改换授权双方，判据只在领域信封里一处。
func TestASecondRevisionCannotBeUsedToSwapTheGrantee(t *testing.T) {
	registry := newChannelAccountUseRegistryStub()
	handler := application.NewRegisterChannelAccountUseHandler(registry)

	first, err := handler.Register(t.Context(), registerChannelUseCommand(t, "operator-1", 1))
	if err != nil || first.Outcome() != application.ChannelAccountUseRegistered {
		t.Fatalf("first register = (%s, %v)", first.Outcome(), err)
	}

	swapped, err := handler.Register(t.Context(), registerChannelUseCommand(t, "operator-2", 2))
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if swapped.Outcome() != application.ChannelAccountUseNotAccepted {
		t.Fatalf("outcome = %s, want NOT_ACCEPTED；换掉被授权人的修订入了册", swapped.Outcome())
	}
	if len(registry.saved) != 1 {
		t.Fatalf("登记册被写了 %d 次，want 1", len(registry.saved))
	}
}

// Covers: 撤销的两条边界——册上没有这笔就不造它，已撤销的不再撤第二次。
//
// 后一条要紧的是时刻：第二次撤销带的是另一个时刻，落库不是同内容重放而是把「何时起不能用」
// 改掉，而已经形成的交易正靠那一刻定依据。
func TestRevocationNeitherInventsNorRewritesTheMoment(t *testing.T) {
	registry := newChannelAccountUseRegistryStub()
	handler := application.NewRegisterChannelAccountUseHandler(registry)
	basis := channelUseValue(t, domain.NewChannelAccountRevocationBasisReference, "holder-notice-1")

	missing, err := handler.Revoke(t.Context(), application.RevokeChannelAccountUseCommand{
		Tenant:    channelUseValue(t, domain.NewTenantID, "tenant-1"),
		ID:        channelUseValue(t, domain.NewChannelAccountUseAuthorizationID, "cauth-absent"),
		Basis:     basis,
		RevokedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("revoke absent: %v", err)
	}
	if missing.Outcome() != application.ChannelAccountUseNotAccepted {
		t.Fatalf("outcome = %s, want NOT_ACCEPTED", missing.Outcome())
	}
	if len(registry.saved) != 0 {
		t.Fatal("撤销一笔不存在的授权时凭空造了行")
	}

	if _, err := handler.Register(t.Context(), registerChannelUseCommand(t, "operator-1", 1)); err != nil {
		t.Fatalf("register: %v", err)
	}
	revokeCommand := application.RevokeChannelAccountUseCommand{
		Tenant:    channelUseValue(t, domain.NewTenantID, "tenant-1"),
		ID:        channelUseValue(t, domain.NewChannelAccountUseAuthorizationID, "cauth-1"),
		Basis:     basis,
		RevokedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	}
	first, err := handler.Revoke(t.Context(), revokeCommand)
	if err != nil || first.Outcome() != application.ChannelAccountUseRegistered {
		t.Fatalf("first revoke = (%s, %v)", first.Outcome(), err)
	}

	revokeCommand.RevokedAt = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	second, err := handler.Revoke(t.Context(), revokeCommand)
	if err != nil {
		t.Fatalf("second revoke: %v", err)
	}
	if second.Outcome() != application.ChannelAccountUseNotAccepted {
		t.Fatalf("outcome = %s, want NOT_ACCEPTED；撤销时刻被改写了", second.Outcome())
	}
	if len(registry.saved) != 2 {
		t.Fatalf("登记册被写了 %d 次，want 2（登记一次、撤销一次）", len(registry.saved))
	}
}
