package main

import (
	"context"
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
}
