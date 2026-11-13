package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

type sendLockDouble struct {
	locked []string
	err    error
}

func (double *sendLockDouble) LockDisclosure(_ context.Context, tenant domain.TenantID, episode domain.EpisodeID, customer domain.CustomerAccountReference) error {
	double.locked = append(double.locked, tenant.String()+"|"+episode.String()+"|"+customer.String())
	return double.err
}

type notifySignalFixture struct {
	*notifyFixture
	handler   *application.NotifyCustomerHandler
	customers *customerAccountDouble
	lock      *sendLockDouble
}

// newNotifySignalFixture 在通知替身之上接运营入口要的三只：按标识找发作期（episode-1 属 parcel-1 / ETA_BREACH_RISK）、
// 可反查的客户（customer-1）与发送锁。
func newNotifySignalFixture(t *testing.T) *notifySignalFixture {
	t.Helper()
	base := newNotifyFixture(t)
	decide := newDecideFixture(t)
	fixture := &notifySignalFixture{notifyFixture: base, customers: decide.customers, lock: &sendLockDouble{}}
	fixture.handler = application.NewNotifyCustomerHandler(application.NotifyCustomerDeps{
		Decisions: base.decisions, Notifications: base.notifications, Policy: base.policy, Identities: base.identities,
		Channel: base.channel, Downstream: base.downstream, Clock: fixedClock{at: disclosureDecidedAt.Add(time.Minute)},
		EpisodeByID: episodeByID{decide.episodes}, Customers: decide.customers, SendLock: fixture.lock,
	})
	return fixture
}

func notifySignalCommand(t *testing.T) application.NotifySignalCommand {
	t.Helper()
	base := decideCommand(t)
	return application.NotifySignalCommand{
		TenantID: base.TenantID, Parcel: base.Parcel, Kind: base.Kind,
		Episode: mustValue(t, domain.NewEpisodeID, "episode-1"), Operator: "https://issuer#carol",
	}
}

// Covers: `UC-VE-006` 步 6 的运营入口——操作员指名包裹、信号类型与哪一期；客户由包裹反查；同一份披露上的发送先上锁再
// 走；披露决定成立即生成并提交通知，再打一次交回原通知、不再推。
func TestAnOperatorNotifiesTheCustomerOfANamedEpisodeOnce(t *testing.T) {
	fixture := newNotifySignalFixture(t)

	result, err := fixture.handler.HandleSignal(context.Background(), notifySignalCommand(t))
	notification, present := result.Notification()
	if err != nil || result.Outcome() != application.NotificationSubmitted || !present || notification.Customer().String() != "customer-1" {
		t.Fatalf("首发：err=%v outcome=%q", err, result.Outcome())
	}
	if len(fixture.lock.locked) != 1 || fixture.lock.locked[0] != "tenant-1|episode-1|customer-1" {
		t.Fatalf("发送锁 = %v", fixture.lock.locked)
	}
	again, err := fixture.handler.HandleSignal(context.Background(), notifySignalCommand(t))
	if err != nil || again.Outcome() != application.NotificationExistingResult || fixture.channel.calls != 1 {
		t.Fatalf("再发：err=%v outcome=%q 渠道被叫 %d 次", err, again.Outcome(), fixture.channel.calls)
	}
}

// Covers: 运营入口的受理与未决——不指名哪一期、没有操作者、指名一期不属于这件包裹这一类信号，都不受理且不碰渠道；
// 客户归属不可确定答未决并说原因；锁拿不到答未决。
func TestTheOperatorNotificationEntryRefusesWhatItCannotPinDown(t *testing.T) {
	fixture := newNotifySignalFixture(t)
	for name, mutate := range map[string]func(*application.NotifySignalCommand){
		"不指名哪一期": func(command *application.NotifySignalCommand) { command.Episode = domain.EpisodeID{} },
		"没有操作者":  func(command *application.NotifySignalCommand) { command.Operator = "" },
		"别处的一期": func(command *application.NotifySignalCommand) {
			command.Episode = mustValue(t, domain.NewEpisodeID, "episode-elsewhere")
		},
	} {
		command := notifySignalCommand(t)
		mutate(&command)
		result, err := fixture.handler.HandleSignal(context.Background(), command)
		if err != nil || result.Outcome() != application.NotifyNotAccepted {
			t.Fatalf("%s：err=%v outcome=%q", name, err, result.Outcome())
		}
	}
	fixture.customers.found = false
	result, err := fixture.handler.HandleSignal(context.Background(), notifySignalCommand(t))
	if err != nil || result.Outcome() != application.NotifyUndecided || result.UndecidedReason() != application.NotificationCustomerAccountUndeterminable {
		t.Fatalf("客户归属不可确定：err=%v outcome=%q reason=%q", err, result.Outcome(), result.UndecidedReason())
	}
	fixture.customers.found = true
	fixture.lock.err = errors.New("lock unavailable")
	result, err = fixture.handler.HandleSignal(context.Background(), notifySignalCommand(t))
	if err != nil || result.Outcome() != application.NotifyUndecided || result.UndecidedReason() != application.NotificationStoreUnavailable {
		t.Fatalf("锁拿不到：err=%v outcome=%q reason=%q", err, result.Outcome(), result.UndecidedReason())
	}
	if fixture.channel.calls != 0 {
		t.Fatalf("渠道被叫了 %d 次", fixture.channel.calls)
	}
}

// Covers: 渠道没登记、凭据变量没有值——没有东西发出去，答未决并说原因，不记`失败`节点（那一格只给真发过而没成的）。
func TestAnUnconfiguredChannelIsUndecidedNotAFailedSubmission(t *testing.T) {
	for reason, sentinel := range map[application.NotifyCustomerUndecidedReason]error{
		application.NotificationChannelNotConfigured:     ports.ErrNotificationChannelNotConfigured,
		application.NotificationChannelCredentialMissing: ports.ErrNotificationChannelCredentialMissing,
	} {
		fixture := newNotifySignalFixture(t)
		fixture.channel.err = fmt.Errorf("%w: portal-message/v1", sentinel)
		result, err := fixture.handler.HandleSignal(context.Background(), notifySignalCommand(t))
		if err != nil || result.Outcome() != application.NotifyUndecided || result.UndecidedReason() != reason {
			t.Fatalf("%s：err=%v outcome=%q reason=%q", reason, err, result.Outcome(), result.UndecidedReason())
		}
		if _, present := result.Notification(); present {
			t.Fatalf("%s：未决的答复带了通知", reason)
		}
	}
}
