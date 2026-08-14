package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var disclosureDecidedAt = time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)

type disclosureKey struct {
	tenant   domain.TenantID
	customer string
	episode  string
	at       time.Time
}

type notificationStoreDouble struct {
	byDisclosure map[disclosureKey]*domain.CustomerNotification
	findErr      error
	saveErr      error
	saved        int
}

func newNotificationStore() *notificationStoreDouble {
	return &notificationStoreDouble{byDisclosure: map[disclosureKey]*domain.CustomerNotification{}}
}

func keyOf(tenant domain.TenantID, disclosure domain.DisclosureDecision) disclosureKey {
	return disclosureKey{
		tenant:   tenant,
		customer: disclosure.Customer().String(),
		episode:  disclosure.Episode().String(),
		at:       disclosure.DecidedAt(),
	}
}

func (double *notificationStoreDouble) FindByDisclosure(
	_ context.Context,
	tenant domain.TenantID,
	disclosure domain.DisclosureDecision,
) (*domain.CustomerNotification, bool, error) {
	if double.findErr != nil {
		return nil, false, double.findErr
	}
	notification, found := double.byDisclosure[keyOf(tenant, disclosure)]
	return notification, found, nil
}

func (double *notificationStoreDouble) Save(
	_ context.Context,
	tenant domain.TenantID,
	notification *domain.CustomerNotification,
) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.byDisclosure[keyOf(tenant, notification.Disclosure())] = notification
	double.saved++
	return nil
}

type notificationPolicyDouble struct {
	directive  ports.NotificationDirective
	configured bool
	err        error
	calls      int
}

func (double *notificationPolicyDouble) DirectNotification(
	_ context.Context,
	_ domain.DisclosureDecision,
) (ports.NotificationDirective, bool, error) {
	double.calls++
	if double.err != nil {
		return ports.NotificationDirective{}, false, double.err
	}
	return double.directive, double.configured, nil
}

type notificationIdentityDouble struct {
	next int
	err  error
}

func (double *notificationIdentityDouble) NextNotificationID(_ context.Context) (domain.NotificationID, error) {
	if double.err != nil {
		return domain.NotificationID{}, double.err
	}
	double.next++
	return domain.NewNotificationID("notification-" + string(rune('0'+double.next)))
}

type channelGatewayDouble struct {
	err   error
	calls int
}

func (double *channelGatewayDouble) SubmitToChannel(
	_ context.Context,
	_ *domain.CustomerNotification,
) error {
	double.calls++
	return double.err
}

type notifyDownstreamDouble struct {
	intents []ports.NotificationHandoffIntent
	err     error
}

func (double *notifyDownstreamDouble) HandOffNotification(
	_ context.Context,
	intent ports.NotificationHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type notifyFixture struct {
	handler       *application.NotifyCustomerHandler
	notifications *notificationStoreDouble
	policy        *notificationPolicyDouble
	identities    *notificationIdentityDouble
	channel       *channelGatewayDouble
	downstream    *notifyDownstreamDouble
}

func newNotifyFixture(t *testing.T) *notifyFixture {
	t.Helper()
	fixture := &notifyFixture{
		notifications: newNotificationStore(),
		policy: &notificationPolicyDouble{
			directive: ports.NotificationDirective{
				Channel:    mustValue(t, domain.NewNotificationChannelReference, "portal-message/v1"),
				Deadline:   disclosureDecidedAt.Add(24 * time.Hour),
				Obligation: mustValue(t, domain.NewDisclosurePolicyReference, "notify-policy/v1"),
			},
			configured: true,
		},
		identities: &notificationIdentityDouble{},
		channel:    &channelGatewayDouble{},
		downstream: &notifyDownstreamDouble{},
	}
	fixture.handler = application.NewNotifyCustomerHandler(application.NotifyCustomerDeps{
		Notifications: fixture.notifications,
		Policy:        fixture.policy,
		Identities:    fixture.identities,
		Channel:       fixture.channel,
		Downstream:    fixture.downstream,
		Clock:         fixedClock{at: disclosureDecidedAt.Add(time.Minute)},
	})
	return fixture
}

// discloseDecision 造一份披露决定；conclusion 为披露格时带内容快照，其余不带。
func discloseDecision(t *testing.T, conclusion domain.DisclosureConclusion) domain.DisclosureDecision {
	t.Helper()
	content := domain.DisclosureContentReference{}
	if conclusion == domain.DiscloseToCustomer {
		content = mustValue(t, domain.NewDisclosureContentReference, "disclosure-content/v1")
	}
	decision, err := domain.DecideDisclosure(
		mustValue(t, domain.NewEpisodeID, "episode-1"),
		mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		mustValue(t, domain.NewDisclosurePolicyReference, "disclosure-policy/v1"),
		conclusion,
		content,
		disclosureDecidedAt,
	)
	if err != nil {
		t.Fatalf("decide disclosure: %v", err)
	}
	return decision
}

func notifyCommand(t *testing.T) application.NotifyCustomerCommand {
	t.Helper()
	return application.NotifyCustomerCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Disclosure: discloseDecision(t, domain.DiscloseToCustomer),
	}
}

// Covers: CONTEXT「客户异常通知必须保存通知对象、内容快照、披露依据、目标客户、要求
// 时限和适用渠道」与「通知已生成、已提交消息渠道……分别记录」——披露决定生成通知、
// 提交渠道、两个节点分别在案、意图由通知标识认领。
func TestADiscloseDecisionGeneratesSubmitsAndRecordsTheNotification(t *testing.T) {
	fixture := newNotifyFixture(t)

	result, err := fixture.handler.Handle(context.Background(), notifyCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.NotificationSubmitted {
		t.Fatalf("outcome = %q, want SUBMITTED", result.Outcome())
	}
	notification, present := result.Notification()
	if !present {
		t.Fatal("a submitted result carries no notification")
	}
	if notification.Customer().String() != "customer-1" ||
		notification.Content().String() != "disclosure-content/v1" {
		t.Fatal("the notification must carry the disclosure's customer and content snapshot")
	}
	if notification.Deadline().IsZero() || notification.Obligation().String() != "notify-policy/v1" {
		t.Fatal("the notification must carry the policy's deadline and obligation basis")
	}
	milestones := notification.Milestones()
	if len(milestones) != 2 ||
		milestones[0] != domain.NotificationGenerated ||
		milestones[1] != domain.NotificationSubmittedToChannel {
		t.Fatalf("milestones = %v, want [GENERATED SUBMITTED_TO_CHANNEL] recorded separately", milestones)
	}
	if fixture.channel.calls != 1 {
		t.Fatalf("channel submissions = %d, want exactly 1", fixture.channel.calls)
	}
	if len(fixture.downstream.intents) != 1 ||
		fixture.downstream.intents[0].Notification.ID() != notification.ID() {
		t.Fatal("exactly one handoff intent claimed by the notification was expected")
	}
}

// Covers: 派工受理约束「只有 DiscloseToCustomer 结论能生成通知——领域已钉，编排不绕」
// 与 CONTEXT「披露条件不成立或授权不足时分别形成暂不披露或待授权结果」——两个非披露格
// 都构不成通知请求，门口即答未受理，不读任何依赖。点名 `AT-VE-099` 的不发消息半边
// 「暂不披露，不自动发消息」与 `AT-VE-100` 的不提交渠道半边「待授权，不提交渠道」。
func TestANonDiscloseConclusionCannotGenerateANotification(t *testing.T) {
	fixture := newNotifyFixture(t)

	for name, conclusion := range map[string]domain.DisclosureConclusion{
		"not yet disclosable":    domain.NotYetDisclosable,
		"awaiting authorization": domain.AwaitingAuthorization,
	} {
		result, err := fixture.handler.Handle(context.Background(), application.NotifyCustomerCommand{
			Disclosure: discloseDecision(t, conclusion),
		})
		if err != nil {
			t.Fatalf("handle %s: %v", name, err)
		}
		if result.Outcome() != application.NotifyNotAccepted {
			t.Fatalf("%s outcome = %q, want NOT_ACCEPTED", name, result.Outcome())
		}
	}
	if fixture.policy.calls != 0 || fixture.channel.calls != 0 || fixture.notifications.saved != 0 {
		t.Fatal("a non-disclose conclusion still reached a dependency")
	}
}

// Covers: 派工约束「渠道与时限目录是实例参数：未配置 → 未决不造渠道」——没有渠道的
// 通知不存在如实的空白格，造占位渠道是虚构；未决且不消耗标识、不写库、不碰渠道。
func TestUnconfiguredNotificationPolicyIsUndecidedWithoutInventingAChannel(t *testing.T) {
	fixture := newNotifyFixture(t)
	fixture.policy.configured = false

	result, err := fixture.handler.Handle(context.Background(), notifyCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.NotifyUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.UndecidedReason() != application.NotificationPolicyNotConfigured {
		t.Fatalf("reason = %q, want NOTIFICATION_POLICY_NOT_CONFIGURED", result.UndecidedReason())
	}
	if fixture.identities.next != 0 || fixture.notifications.saved != 0 || fixture.channel.calls != 0 {
		t.Fatal("an unconfigured policy still consumed an identity, wrote the store, or touched the channel")
	}
}

// Covers: 未决语义——策略调不通是依赖故障，与「未配置」分开：一个重试依赖，一个等
// 租户登记，混起来会对着一个没配置的租户参数无休止重试。
func TestAnUnavailableNotificationPolicyIsUndecidedUnderItsOwnReason(t *testing.T) {
	fixture := newNotifyFixture(t)
	fixture.policy.err = errors.New("notification policy unavailable")

	result, err := fixture.handler.Handle(context.Background(), notifyCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.UndecidedReason() != application.NotificationPolicyUnavailable {
		t.Fatalf("reason = %q, want NOTIFICATION_POLICY_UNAVAILABLE", result.UndecidedReason())
	}
	if application.NotificationPolicyUnavailable == application.NotificationPolicyNotConfigured {
		t.Fatal("unavailable and not-configured share one reason; their recovery actions diverge")
	}
}

// Covers: 派工幂等约束「同一披露不重发通知」与 ADR-0043「重放重发同一份意图」——已
// 提交过渠道的披露重放不再生成、不再提交，只把同一份意图再交一次。
func TestTheSameDisclosureDoesNotRenotifyButResendsTheSameIntent(t *testing.T) {
	fixture := newNotifyFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, notifyCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstNotification, _ := first.Notification()
	issuedBefore, submittedBefore := fixture.identities.next, fixture.channel.calls

	replay, err := fixture.handler.Handle(ctx, notifyCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}

	if replay.Outcome() != application.NotificationExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	replayed, present := replay.Notification()
	if !present || replayed.ID() != firstNotification.ID() {
		t.Fatal("the replay did not return the notification that already exists")
	}
	if fixture.identities.next != issuedBefore || fixture.channel.calls != submittedBefore {
		t.Fatal("a replay consumed a new identity or submitted the channel again")
	}
	if len(fixture.downstream.intents) != 2 ||
		fixture.downstream.intents[1].Notification.ID() != firstNotification.ID() {
		t.Fatal("the replay must resend the same intent claimed by the existing notification")
	}
}

// Covers: CONTEXT「已提交消息渠道、渠道已接受、已送达、失败和客户确认必须分别记录」
// 的失败半边——提交失败记`失败`节点，通知保持已成立可重试，不是未决也不改写已生成。
// 点名 `AT-VE-106`「投递失败→保留失败尝试，按策略重试或升级」的记录半边。
func TestAFailedChannelSubmissionIsRecordedAndKeptForRetry(t *testing.T) {
	fixture := newNotifyFixture(t)
	fixture.channel.err = errors.New("channel unavailable")

	result, err := fixture.handler.Handle(context.Background(), notifyCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.NotificationSubmissionFailed {
		t.Fatalf("outcome = %q, want SUBMISSION_FAILED", result.Outcome())
	}
	notification, _ := result.Notification()
	milestones := notification.Milestones()
	if len(milestones) != 2 ||
		milestones[0] != domain.NotificationGenerated ||
		milestones[1] != domain.NotificationFailed {
		t.Fatalf("milestones = %v, want [GENERATED FAILED]", milestones)
	}
	if fixture.notifications.saved == 0 {
		t.Fatal("a failed submission discarded the notification instead of keeping it for retry")
	}
}

// Covers: 派工约束「失败后重试产新节点前节点保留——领域已钉」与 CONTEXT「失败按策略
// 重试或升级」——重试沿用同一通知（不签新身份），新节点接在失败后面，历史一个不丢。
// 点名 `AT-VE-106`「投递失败→保留失败尝试，按策略重试或升级」的重试半边。
func TestARetryAfterFailureAppendsANewMilestoneKeepingTheFailedOne(t *testing.T) {
	fixture := newNotifyFixture(t)
	ctx := context.Background()
	fixture.channel.err = errors.New("channel unavailable")

	if _, err := fixture.handler.Handle(ctx, notifyCommand(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	fixture.channel.err = nil

	retry, err := fixture.handler.Handle(ctx, notifyCommand(t))
	if err != nil {
		t.Fatalf("retry handle: %v", err)
	}

	if retry.Outcome() != application.NotificationSubmitted {
		t.Fatalf("outcome = %q, want SUBMITTED", retry.Outcome())
	}
	notification, _ := retry.Notification()
	milestones := notification.Milestones()
	if len(milestones) != 3 ||
		milestones[0] != domain.NotificationGenerated ||
		milestones[1] != domain.NotificationFailed ||
		milestones[2] != domain.NotificationSubmittedToChannel {
		t.Fatalf("milestones = %v, want [GENERATED FAILED SUBMITTED_TO_CHANNEL]", milestones)
	}
	if fixture.identities.next != 1 {
		t.Fatalf("issued %d identities; the retry must reuse the same notification", fixture.identities.next)
	}
	if !notification.ObligationMetBy(domain.NotificationSubmittedToChannel) {
		t.Fatal("the retried notification does not report its successful submission")
	}
}

// Covers: 同一条未决语义的存储半边——通知库读不回时停在未决，不问策略不生成。
func TestAnUnreadableNotificationStoreIsUndecidedBeforeAnythingElse(t *testing.T) {
	fixture := newNotifyFixture(t)
	fixture.notifications.findErr = errors.New("notification store unavailable")

	result, err := fixture.handler.Handle(context.Background(), notifyCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.NotifyUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.UndecidedReason() != application.NotificationStoreUnavailable {
		t.Fatalf("reason = %q, want NOTIFICATION_STORE_UNAVAILABLE", result.UndecidedReason())
	}
	if fixture.policy.calls != 0 {
		t.Fatal("the round asked the policy before it could answer idempotently")
	}
}

// Covers: ADR-0043「首次交付失败不改写业务结果……另留一条发布续办引用」——意图交不
// 出去时通知保持已提交，只留可续办引用。
func TestAFailedNotificationHandoffLeavesAResumableReference(t *testing.T) {
	fixture := newNotifyFixture(t)
	fixture.downstream.err = errors.New("downstream unavailable")

	result, err := fixture.handler.Handle(context.Background(), notifyCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.NotificationSubmitted {
		t.Fatalf("outcome = %q, want SUBMITTED; a failed handoff must not rewrite the notification", result.Outcome())
	}
	if result.HandoffReference() == "" {
		t.Fatal("a failed handoff left no resumable reference")
	}
}

// Covers: 受理半边——零值披露决定构不成通知请求，门口即答未受理。
func TestACommandWithoutADisclosureIsNotAccepted(t *testing.T) {
	fixture := newNotifyFixture(t)

	result, err := fixture.handler.Handle(context.Background(), application.NotifyCustomerCommand{})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.NotifyNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
	}
}

// Covers: 端口封闭答复纪律——策略答了`已配置`却缺渠道、时限或义务判据，是端口坏答复，
// 上抛而不吞成业务结果（领域构造器在场拦住）。
func TestADirectiveWithoutAChannelIsRaisedAsAPortDefect(t *testing.T) {
	fixture := newNotifyFixture(t)
	fixture.policy.directive.Channel = domain.NotificationChannelReference{}

	_, err := fixture.handler.Handle(context.Background(), notifyCommand(t))
	if err == nil {
		t.Fatal("a directive without a channel was swallowed instead of raised")
	}
	if fixture.channel.calls != 0 {
		t.Fatal("a defective directive still reached the channel")
	}
}
