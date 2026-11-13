package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// Covers: UC-VE-006 步 3–5 在真库上经生产装配走一遍——包裹的客户归属取 parcel-shipment 的已接受委托，披露规则按认证
// 出来的租户取；规则未登记时答未决（规则未配置）、不落决定；登了「披露但未批准自动发布」的规则后判出`待授权`；两个
// 人同时确认，恰有一人答已授权、另一人答已有结果，库里恰一份`披露`决定；另一个租户对同一件包裹什么也判不出。合成
// 租户、账户与包裹只记 `S`。
func TestTheProductionDisclosureFacesDecideAndAuthorizeOnceAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	const parcel = "SYN-PARCEL-C31-01"
	submission := trackingViewSubmissionCommand(t, parcel)
	submitOnRealAssembly(t, db, submission)
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), submission.Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	acceptAndSave(t, db, requests, submission, request)

	tenant := mustValue(t, vedomain.NewTenantID, "SYN-TENANT-01")
	tracked := mustValue(t, vedomain.NewTrackedParcelReference, parcel)
	kind := mustValue(t, vedomain.NewExceptionSignalKindReference, "ETA_BREACH_RISK")
	confidence := mustValue(t, vedomain.NewConfidenceReference, "HIGH")
	fact := mustValue(t, vedomain.NewSourceFactReference, "movement-fact/"+parcel+"/MF-1")
	cause, err := vedomain.SourceFactCausalCondition(vedomain.SourceTransportFulfillment, fact)
	if err != nil {
		t.Fatal(err)
	}
	signalRule := mustValue(t, vedomain.NewSignalRuleVersionReference, "signal-rules/v3")
	raisedAt := time.Now().UTC().Add(-time.Hour)
	episode, err := vedomain.OpenEpisode(mustValue(t, vedomain.NewEpisodeID, "SYN-EPISODE-DISC-01"), vedomain.SignalHit{
		Kind: kind, Parcel: tracked, Cause: cause, Rule: signalRule, Confidence: confidence,
		Basis: []vedomain.SignalFactBasis{{
			Source: vedomain.SourceTransportFulfillment, Fact: fact, Version: mustValue(t, vedomain.NewSourceFactVersion, "v1"),
		}},
		At: raisedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	conclusion, err := vedomain.ConcludeTriage(episode.ID(), vedomain.ManualReviewRequired, signalRule, raisedAt)
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := vepostgres.NewSignalEpisodes(db)
	if err != nil {
		t.Fatal(err)
	}
	transactor := db.Transactor()
	if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return episodes.SaveRaised(txCtx, veports.RaisedSignalRecord{Tenant: tenant, Parcel: tracked, Kind: kind, Episode: episode, Conclusion: conclusion})
	}); err != nil {
		t.Fatalf("落信号发作期：%v", err)
	}

	faces, err := buildCustomerDisclosureFaces(db)
	if err != nil {
		t.Fatal(err)
	}
	decide := visibilityapp.DecideDisclosureCommand{TenantID: tenant, Parcel: tracked, Kind: kind, Episode: episode.ID(), Operator: "https://id.syn.example/dex#SYN-OPERATOR-01"}
	decisionRows := func(where string) (count int) {
		t.Helper()
		if err := pool.QueryRow(t.Context(),
			`SELECT count(*) FROM visibility_exception.disclosure_decision WHERE tenant_id = 'SYN-TENANT-01' AND `+where).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	result, err := faces.Handle(t.Context(), decide)
	if err != nil || result.Outcome() != visibilityapp.DecideDisclosureUndecided || result.UndecidedReason() != visibilityapp.DisclosureRuleNotConfigured || decisionRows("true") != 0 {
		t.Fatalf("规则未登记：err=%v outcome=%q reason=%q 决定 %d 行", err, result.Outcome(), result.UndecidedReason(), decisionRows("true"))
	}

	registrar, err := vepostgres.NewCatalogRegistrar(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := registrar.RegisterExceptionDisclosureRules(txCtx, tenant, veports.ExceptionDisclosureRuleRegistration{
			Header: veports.CatalogVersionHeader{Version: "SYN-exception-disclosure/v1", ApprovedBy: "SYN-OPERATOR-01", EffectiveFrom: time.Now().UTC().Add(-time.Minute)},
			Entries: []veports.ExceptionDisclosureRuleEntry{{
				Customer: mustValue(t, vedomain.NewCustomerAccountReference, syntheticCustomerAccount), Kind: kind, Confidence: confidence,
				Disclosable: true, Content: mustValue(t, vedomain.NewDisclosureContentReference, "SYN-content/eta-breach/v1"),
			}},
		})
		return err
	}); err != nil {
		t.Fatalf("登披露规则：%v", err)
	}

	// 另一个租户对同一件包裹：它没有这一期信号，未受理。
	foreign := decide
	foreign.TenantID = mustValue(t, vedomain.NewTenantID, "SYN-TENANT-02")
	if result, err = faces.Handle(t.Context(), foreign); err != nil || result.Outcome() != visibilityapp.DecideDisclosureNotAccepted {
		t.Fatalf("另一个租户：err=%v outcome=%q", err, result.Outcome())
	}

	result, err = faces.Handle(t.Context(), decide)
	decision, _ := result.Decision()
	if err != nil || result.Outcome() != visibilityapp.DisclosureDecided || decision.Conclusion() != vedomain.AwaitingAuthorization ||
		decision.Customer().String() != syntheticCustomerAccount || decisionRows("decided_by = 'https://id.syn.example/dex#SYN-OPERATOR-01'") != 1 {
		t.Fatalf("判出待授权：err=%v outcome=%q decision=%+v", err, result.Outcome(), decision)
	}
	if result, err = faces.Handle(t.Context(), decide); err != nil || result.Outcome() != visibilityapp.DisclosureExistingResult {
		t.Fatalf("重判：err=%v outcome=%q", err, result.Outcome())
	}

	// 同一件包裹同一类信号因另一个原因又开了一期、成了最近的那一期：对原来那一期的确认仍落在原来那一期上。
	laterFact := mustValue(t, vedomain.NewSourceFactReference, "movement-fact/"+parcel+"/MF-2")
	laterCause, err := vedomain.SourceFactCausalCondition(vedomain.SourceTransportFulfillment, laterFact)
	if err != nil {
		t.Fatal(err)
	}
	later, err := vedomain.OpenEpisode(mustValue(t, vedomain.NewEpisodeID, "SYN-EPISODE-DISC-02"), vedomain.SignalHit{
		Kind: kind, Parcel: tracked, Cause: laterCause, Rule: signalRule, Confidence: confidence,
		Basis: []vedomain.SignalFactBasis{{
			Source: vedomain.SourceTransportFulfillment, Fact: laterFact, Version: mustValue(t, vedomain.NewSourceFactVersion, "v1"),
		}},
		At: raisedAt.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	laterConclusion, err := vedomain.ConcludeTriage(later.ID(), vedomain.ManualReviewRequired, signalRule, raisedAt.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return episodes.SaveRaised(txCtx, veports.RaisedSignalRecord{Tenant: tenant, Parcel: tracked, Kind: kind, Episode: later, Conclusion: laterConclusion})
	}); err != nil {
		t.Fatalf("落第二期：%v", err)
	}

	start := make(chan struct{})
	outcomes := make(chan visibilityapp.DecideDisclosureOutcome, 2)
	for _, operator := range []string{"https://id.syn.example/dex#SYN-OPERATOR-02", "https://id.syn.example/dex#SYN-OPERATOR-03"} {
		go func() {
			<-start
			result, err := faces.Authorize(context.Background(), visibilityapp.AuthorizeDisclosureCommand{TenantID: tenant, Parcel: tracked, Kind: kind, Episode: episode.ID(), Operator: operator})
			if err != nil {
				t.Errorf("确认：%v", err)
			}
			outcomes <- result.Outcome()
		}()
	}
	close(start)
	seen := map[visibilityapp.DecideDisclosureOutcome]int{}
	for range 2 {
		seen[<-outcomes]++
	}
	if seen[visibilityapp.DisclosureAuthorized] != 1 || seen[visibilityapp.DisclosureExistingResult] != 1 {
		t.Fatalf("两人同时确认 = %v，想要一人已授权、一人已有结果", seen)
	}
	if decisionRows("episode_id = 'SYN-EPISODE-DISC-01' AND conclusion = 'DISCLOSE' AND content_ref = 'SYN-content/eta-breach/v1' AND authorizes_decided_at IS NOT NULL") != 1 || decisionRows("true") != 2 {
		t.Fatalf("披露决定 %d 行、共 %d 行，想要 1 与 2", decisionRows("conclusion = 'DISCLOSE'"), decisionRows("true"))
	}

	// —— UC-VE-006 步 6：把这份披露通知客户，经生产装配的通知口走一遍 ——
	var pushes atomic.Int32
	var pushStatus atomic.Int32
	pushStatus.Store(http.StatusInternalServerError)
	var lastKey atomic.Value
	customerSystem := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		pushes.Add(1)
		lastKey.Store(request.Header.Get("Idempotency-Key") + "|" + request.Header.Get("Authorization"))
		response.WriteHeader(int(pushStatus.Load()))
	}))
	defer customerSystem.Close()
	notifier, err := buildCustomerExceptionNotificationFace(db, func(name string) string {
		if name == "IDP_PARCEL_NOTIFY_SYN_CUSTOMER" {
			return "SYN-token"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	notify := visibilityapp.NotifySignalCommand{TenantID: tenant, Parcel: tracked, Kind: kind, Episode: episode.ID(), Operator: "https://id.syn.example/dex#SYN-OPERATOR-04"}
	notificationRows := func() (count int) {
		t.Helper()
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM visibility_exception.customer_notification WHERE tenant_id = 'SYN-TENANT-01'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	undecidedFor := func(stage string, reason visibilityapp.NotifyCustomerUndecidedReason) {
		t.Helper()
		result, err := notifier.HandleSignal(t.Context(), notify)
		if err != nil || result.Outcome() != visibilityapp.NotifyUndecided || result.UndecidedReason() != reason || notificationRows() != 0 || pushes.Load() != 0 {
			t.Fatalf("%s：err=%v outcome=%q reason=%q 通知 %d 行 推送 %d 次", stage, err, result.Outcome(), result.UndecidedReason(), notificationRows(), pushes.Load())
		}
	}
	undecidedFor("通知策略未登记", visibilityapp.NotificationPolicyNotConfigured)

	channelRef := mustValue(t, vedomain.NewNotificationChannelReference, "SYN-customer-webhook")
	if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := registrar.RegisterNotificationPolicy(txCtx, tenant, veports.NotificationPolicyRegistration{
			Policy: mustValue(t, vedomain.NewDisclosurePolicyReference, "SYN-exception-disclosure/v1"), Channel: channelRef,
			DeadlineAfter: 24 * time.Hour, Obligation: mustValue(t, vedomain.NewDisclosurePolicyReference, "SYN-notify-policy/v1"), ApprovedBy: "SYN-OPERATOR-01",
		})
		return err
	}); err != nil {
		t.Fatalf("登通知策略：%v", err)
	}
	undecidedFor("渠道未登记", visibilityapp.NotificationChannelNotConfigured)

	channels, err := vepostgres.NewNotificationChannels(db)
	if err != nil {
		t.Fatal(err)
	}
	registerChannel := func(credential string) {
		t.Helper()
		endpoint, err := vedomain.NewNotificationChannelEndpoint(channelRef, customerSystem.URL+"/hooks/exceptions", credential)
		if err != nil {
			t.Fatal(err)
		}
		if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			return channels.RegisterChannel(txCtx, tenant, endpoint, time.Now())
		}); err != nil {
			t.Fatalf("登通知渠道：%v", err)
		}
	}
	registerChannel("IDP_PARCEL_NOTIFY_SYN_UNSET")
	undecidedFor("凭据变量没有值", visibilityapp.NotificationChannelCredentialMissing)
	registerChannel("IDP_PARCEL_NOTIFY_SYN_CUSTOMER")

	// 调用方在客户系统收到推送之后断开（操作员关页面、反向代理超时）：推送那一笔不随它取消，节点照样记下。通知记录
	// 先于推送提交，所以即使那一笔没落成，记录也在、标识不变。
	var cancelCaller atomic.Value
	cancelCaller.Store(context.CancelFunc(func() {}))
	hangUp := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		cancelCaller.Load().(context.CancelFunc)()
		response.WriteHeader(http.StatusInternalServerError)
	}))
	defer hangUp.Close()
	hangUpEndpoint, err := vedomain.NewNotificationChannelEndpoint(channelRef, hangUp.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return channels.RegisterChannel(txCtx, tenant, hangUpEndpoint, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	callerCtx, cancel := context.WithCancel(t.Context())
	cancelCaller.Store(cancel)
	sent, err := notifier.HandleSignal(callerCtx, notify)
	cancel()
	if err != nil || sent.Outcome() != visibilityapp.NotificationSubmissionFailed || notificationRows() != 1 {
		t.Fatalf("调用方中途断开：err=%v outcome=%q 通知 %d 行，想要记下失败节点", err, sent.Outcome(), notificationRows())
	}
	var firstID string
	if err := pool.QueryRow(t.Context(), `SELECT notification_id FROM visibility_exception.customer_notification WHERE tenant_id = 'SYN-TENANT-01'`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	registerChannel("IDP_PARCEL_NOTIFY_SYN_CUSTOMER")

	// 客户系统答 500：再记一个`失败`节点，通知留着等重发，标识不换。
	sent, err = notifier.HandleSignal(t.Context(), notify)
	if resent, _ := sent.Notification(); resent == nil || resent.ID().String() != firstID {
		t.Fatalf("重发换了通知标识：%v，想要 %s", resent, firstID)
	}
	if err != nil || sent.Outcome() != visibilityapp.NotificationSubmissionFailed || notificationRows() != 1 || pushes.Load() != 1 {
		t.Fatalf("客户系统答 500：err=%v outcome=%q 通知 %d 行 推送 %d 次", err, sent.Outcome(), notificationRows(), pushes.Load())
	}
	// 另一个租户、没有披露决定的第二期：都不受理，不推。
	foreignNotify := notify
	foreignNotify.TenantID = mustValue(t, vedomain.NewTenantID, "SYN-TENANT-02")
	undisclosed := notify
	undisclosed.Episode = later.ID()
	for name, command := range map[string]visibilityapp.NotifySignalCommand{"另一个租户": foreignNotify, "没有披露决定的那一期": undisclosed} {
		if sent, err = notifier.HandleSignal(t.Context(), command); err != nil || sent.Outcome() != visibilityapp.NotifyNotAccepted || pushes.Load() != 1 {
			t.Fatalf("%s：err=%v outcome=%q 推送 %d 次", name, err, sent.Outcome(), pushes.Load())
		}
	}

	// 客户系统恢复：两个人同时重发，恰推一次——一人答已提交、一人答已有结果，过程节点接在失败之后。
	pushStatus.Store(http.StatusNoContent)
	resend := make(chan struct{})
	resent := make(chan visibilityapp.NotifyCustomerOutcome, 2)
	for range 2 {
		go func() {
			<-resend
			result, err := notifier.HandleSignal(context.Background(), notify)
			if err != nil {
				t.Errorf("重发：%v", err)
			}
			resent <- result.Outcome()
		}()
	}
	close(resend)
	got := map[visibilityapp.NotifyCustomerOutcome]int{}
	for range 2 {
		got[<-resent]++
	}
	if got[visibilityapp.NotificationSubmitted] != 1 || got[visibilityapp.NotificationExistingResult] != 1 || pushes.Load() != 2 {
		t.Fatalf("两人同时重发 = %v 推送共 %d 次，想要一人已提交、一人已有结果、共 2 次", got, pushes.Load())
	}
	if key, _ := lastKey.Load().(string); !strings.HasPrefix(key, "SYN-TENANT-01/SYN-EPISODE-DISC-01/"+syntheticCustomerAccount+"/") || !strings.HasSuffix(key, "|Bearer SYN-token") {
		t.Fatalf("幂等键与凭据 = %q", key)
	}
	var milestones string
	if err := pool.QueryRow(t.Context(),
		`SELECT milestones::text FROM visibility_exception.customer_notification WHERE tenant_id = 'SYN-TENANT-01'`).Scan(&milestones); err != nil {
		t.Fatalf("读过程节点：%v", err)
	}
	if !strings.Contains(milestones, "FAILED") || !strings.Contains(milestones, "SUBMITTED_TO_CHANNEL") || strings.Count(milestones, "SUBMITTED_TO_CHANNEL") != 1 {
		t.Fatalf("过程节点 = %s，想要失败之后接一次已提交", milestones)
	}
	if sent, err = notifier.HandleSignal(t.Context(), notify); err != nil || sent.Outcome() != visibilityapp.NotificationExistingResult || pushes.Load() != 2 {
		t.Fatalf("已提交后再发：err=%v outcome=%q 推送 %d 次", err, sent.Outcome(), pushes.Load())
	}
}
