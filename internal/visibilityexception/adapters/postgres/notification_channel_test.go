package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

func channelEndpoint(t *testing.T, channel, endpoint, credential string) domain.NotificationChannelEndpoint {
	t.Helper()
	registered, err := domain.NewNotificationChannelEndpoint(build(t, domain.NewNotificationChannelReference, channel), endpoint, credential)
	if err != nil {
		t.Fatalf("构造渠道端点：%v", err)
	}
	return registered
}

// Covers: 0038——通知渠道登记册的往返：按（租户，渠道引用）找回端点与凭据变量名；没登记答没有；重登即换端点；另一个
// 租户看不见。无事务登记即拒。
func TestNotificationChannelsRoundTripPerTenantAndReRegistrationReplacesTheEndpoint(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	ctx := t.Context()
	channels, err := adapter.NewNotificationChannels(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	reference := build(t, domain.NewNotificationChannelReference, "SYN-customer-webhook")

	if _, found, err := channels.FindChannel(ctx, tenant, reference); err != nil || found {
		t.Fatalf("空册：err=%v found=%v", err, found)
	}
	if err := channels.RegisterChannel(ctx, tenant, channelEndpoint(t, "SYN-customer-webhook", "https://hooks.syn.example/a", ""), time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务登记通知渠道应返回 ErrTransactionRequired，实得：%v", err)
	}
	register := func(endpoint, credential string) {
		t.Helper()
		if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			return channels.RegisterChannel(txCtx, tenant, channelEndpoint(t, "SYN-customer-webhook", endpoint, credential), time.Now())
		}); err != nil {
			t.Fatalf("登记渠道：%v", err)
		}
	}
	register("https://hooks.syn.example/a", "SYN_HOOK_TOKEN")
	found, present, err := channels.FindChannel(ctx, tenant, reference)
	if err != nil || !present || found.Endpoint() != "https://hooks.syn.example/a" || found.Credential() != "SYN_HOOK_TOKEN" {
		t.Fatalf("读回：err=%v present=%v %+v", err, present, found)
	}
	register("https://hooks.syn.example/b", "")
	if found, _, err = channels.FindChannel(ctx, tenant, reference); err != nil || found.Endpoint() != "https://hooks.syn.example/b" || found.Credential() != "" {
		t.Fatalf("重登后：err=%v %+v", err, found)
	}
	if _, present, err := channels.FindChannel(ctx, build(t, domain.NewTenantID, "tenant-b"), reference); err != nil || present {
		t.Fatalf("跨租户可见：err=%v present=%v", err, present)
	}

	notifications, err := adapter.NewCustomerNotifications(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := notifications.LockDisclosure(ctx, tenant, build(t, domain.NewEpisodeID, "ep-1"), build(t, domain.NewCustomerAccountReference, "acct-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务取通知发送锁应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 端点只收 https，或回环地址上的 http。
func TestNotificationChannelEndpointsMustBeHTTPSOrLoopback(t *testing.T) {
	reference := build(t, domain.NewNotificationChannelReference, "SYN-customer-webhook")
	for _, refused := range []string{"http://hooks.syn.example/a", "ftp://hooks.syn.example/a", "/relative", "", "https://"} {
		if _, err := domain.NewNotificationChannelEndpoint(reference, refused, ""); !errors.Is(err, domain.ErrInvalidNotificationChannel) {
			t.Errorf("端点 %q 被收下了：%v", refused, err)
		}
	}
	for _, accepted := range []string{"https://hooks.syn.example/a", "http://127.0.0.1:8080/hook", "http://localhost/hook"} {
		if _, err := domain.NewNotificationChannelEndpoint(reference, accepted, ""); err != nil {
			t.Errorf("端点 %q 被拒：%v", accepted, err)
		}
	}
}
