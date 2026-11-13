package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证异常披露规则目录（0023）：登记之前答未配置、登记之后
// 三格逐条读回、目录已配但这一键没有条目仍答未配置、成对约束由库守住；以及披露决定
// 登记册的往返、当前那一份的选取与三维幂等。

func disclosureRuleView(t *testing.T, fixture *registrarFixture, tenant domain.TenantID) *adapter.ExceptionDisclosureRules {
	t.Helper()
	view, err := adapter.NewExceptionDisclosureRules(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造异常披露规则视图：%v", err)
	}
	return view
}

func disclosureRuleEntry(
	t *testing.T,
	customer, kind, confidence string,
	disclosable, autoRelease bool,
	content string,
) ports.ExceptionDisclosureRuleEntry {
	t.Helper()
	entry := ports.ExceptionDisclosureRuleEntry{
		Customer:    build(t, domain.NewCustomerAccountReference, customer),
		Kind:        build(t, domain.NewExceptionSignalKindReference, kind),
		Confidence:  build(t, domain.NewConfidenceReference, confidence),
		Disclosable: disclosable,
		AutoRelease: autoRelease,
	}
	if content != "" {
		entry.Content = build(t, domain.NewDisclosureContentReference, content)
	}
	return entry
}

// Covers: 异常披露规则目录（`PAR-VIS-07`）的写入方与读口：三格（不披露 / 披露待授权 /
// 披露且自动发布）各自原样读回，规则引用取版本号；目录已配但这一键没有条目仍答未配置
// ——没有默认披露值可发明。
func TestRegisteredExceptionDisclosureRulesBecomeReadable(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view := disclosureRuleView(t, fixture, tenant)
	customer := build(t, domain.NewCustomerAccountReference, "acct-1")
	kind := build(t, domain.NewExceptionSignalKindReference, "ETA_BREACH_RISK")
	high := build(t, domain.NewConfidenceReference, "HIGH")

	if _, configured, err := view.RuleForSignal(t.Context(), customer, kind, high); err != nil || configured {
		t.Fatalf("登记之前必须答未配置：err=%v configured=%v", err, configured)
	}

	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterExceptionDisclosureRules(txCtx, tenant, ports.ExceptionDisclosureRuleRegistration{
			Header: mappingHeader("exception-disclosure/v1", time.Now().UTC().Add(-time.Hour)),
			Entries: []ports.ExceptionDisclosureRuleEntry{
				disclosureRuleEntry(t, "acct-1", "ETA_BREACH_RISK", "HIGH", true, true, "content/eta-breach/v1"),
				disclosureRuleEntry(t, "acct-1", "ETA_BREACH_RISK", "LOW", true, false, "content/eta-breach/v1"),
				disclosureRuleEntry(t, "acct-1", "CUSTOMS_HOLD", "HIGH", false, false, ""),
			},
		})
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("登记异常披露规则应成功，实得 %s", outcome)
	}

	auto, configured, err := view.RuleForSignal(t.Context(), customer, kind, high)
	if err != nil || !configured {
		t.Fatalf("登记之后应答已配置：err=%v configured=%v", err, configured)
	}
	if !auto.Disclosable || !auto.AutoRelease ||
		auto.Content.String() != "content/eta-breach/v1" ||
		auto.Policy.String() != "exception-disclosure/v1" {
		t.Fatalf("自动发布条目读回走样：%+v", auto)
	}

	awaiting, _, err := view.RuleForSignal(t.Context(), customer, kind, build(t, domain.NewConfidenceReference, "LOW"))
	if err != nil {
		t.Fatalf("读低可信条目：%v", err)
	}
	if !awaiting.Disclosable || awaiting.AutoRelease || awaiting.Content.String() != "content/eta-breach/v1" {
		t.Fatalf("待授权条目读回走样：%+v", awaiting)
	}

	withheld, _, err := view.RuleForSignal(t.Context(), customer,
		build(t, domain.NewExceptionSignalKindReference, "CUSTOMS_HOLD"), high)
	if err != nil {
		t.Fatalf("读不披露条目：%v", err)
	}
	if withheld.Disclosable || withheld.AutoRelease || withheld.Content.String() != "" {
		t.Fatalf("不披露条目读回走样：%+v", withheld)
	}

	// 目录已配但这个客户没有条目：仍是未配置，不是「不披露」。
	if _, configured, err := view.RuleForSignal(t.Context(),
		build(t, domain.NewCustomerAccountReference, "acct-9"), kind, high); err != nil || configured {
		t.Fatalf("无条目的客户应答未配置：err=%v configured=%v", err, configured)
	}
	// 另一租户看不见。
	if _, configured, err := disclosureRuleView(t, fixture, build(t, domain.NewTenantID, "tenant-b")).
		RuleForSignal(t.Context(), customer, kind, high); err != nil || configured {
		t.Fatalf("跨租户可见：err=%v configured=%v", err, configured)
	}
}

// Covers: 0023 的两条成对约束——披露必带内容、不披露必不带；不披露不得自动发布。写入口
// 把矛盾条目交给库拒：不成对就整版不落。
func TestExceptionDisclosureRuleEntriesMustPairContentAndAutoReleaseWithDisclosable(t *testing.T) {
	fixture := newRegistrarFixture(t)

	for name, entry := range map[string]ports.ExceptionDisclosureRuleEntry{
		"披露却无内容":   disclosureRuleEntry(t, "acct-1", "ETA_BREACH_RISK", "HIGH", true, false, ""),
		"不披露却带内容":  disclosureRuleEntry(t, "acct-1", "ETA_BREACH_RISK", "HIGH", false, false, "content/x"),
		"不披露却自动发布": disclosureRuleEntry(t, "acct-1", "ETA_BREACH_RISK", "HIGH", false, true, ""),
	} {
		// 每种矛盾各用一个租户：前一次被拒的事务已回滚，但版本号仍分开，免得撞上重叠判定。
		tenant := build(t, domain.NewTenantID, "tenant-"+name)
		err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			_, err := fixture.registrar.RegisterExceptionDisclosureRules(txCtx, tenant, ports.ExceptionDisclosureRuleRegistration{
				Header:  mappingHeader("exception-disclosure/v1", time.Now().UTC().Add(-time.Hour)),
				Entries: []ports.ExceptionDisclosureRuleEntry{entry},
			})
			return err
		})
		if err == nil {
			t.Fatalf("%s 的条目穿过了写入口", name)
		}
		if _, configured, err := disclosureRuleView(t, fixture, tenant).RuleForSignal(t.Context(),
			entry.Customer, entry.Kind, entry.Confidence); err != nil || configured {
			t.Fatalf("%s 被拒后版本仍在：err=%v configured=%v", name, err, configured)
		}
	}
}

func decisionStore(t *testing.T, fixture *registrarFixture) *adapter.DisclosureDecisions {
	t.Helper()
	store, err := adapter.NewDisclosureDecisions(fixture.db)
	if err != nil {
		t.Fatalf("构造披露决定登记册：%v", err)
	}
	return store
}

func recordedDecision(
	t *testing.T,
	episode, customer string,
	conclusion domain.DisclosureConclusion,
	content string,
	decidedAt time.Time,
) domain.DisclosureDecision {
	t.Helper()
	var reference domain.DisclosureContentReference
	if content != "" {
		reference = build(t, domain.NewDisclosureContentReference, content)
	}
	decision, err := domain.DecideDisclosure(
		build(t, domain.NewEpisodeID, episode),
		build(t, domain.NewCustomerAccountReference, customer),
		build(t, domain.NewDisclosurePolicyReference, "exception-disclosure/v1"),
		conclusion,
		reference,
		decidedAt,
	)
	if err != nil {
		t.Fatalf("构造披露决定：%v", err)
	}
	return decision
}

// Covers: 披露决定登记册的往返与「当前那一份」——三态各自原样读回；同（发作期+客户）
// 下决定时刻最晚的是当前决定（待授权转披露是新的一行，旧行不改）；同三维重登幂等；
// 另一租户、另一客户各不可见。
func TestDisclosureDecisionsRoundTripAndTheLatestOneIsCurrent(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	store := decisionStore(t, fixture)
	ctx := t.Context()
	episode := build(t, domain.NewEpisodeID, "ep-1")
	customer := build(t, domain.NewCustomerAccountReference, "acct-1")

	if _, found, err := store.FindCurrent(ctx, tenant, episode, customer); err != nil || found {
		t.Fatalf("空册应答无决定：err=%v found=%v", err, found)
	}

	awaiting := recordedDecision(t, "ep-1", "acct-1", domain.AwaitingAuthorization, "", registrarBaseAt)
	var outcome ports.DisclosureDecisionSaveOutcome
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = store.Save(txCtx, tenant, awaiting)
		return err
	}); err != nil || outcome != ports.DisclosureDecisionSaved {
		t.Fatalf("首登待授权决定：err=%v outcome=%d", err, outcome)
	}

	current, found, err := store.FindCurrent(ctx, tenant, episode, customer)
	if err != nil || !found {
		t.Fatalf("读回决定：%v found=%v", err, found)
	}
	if current.Conclusion() != domain.AwaitingAuthorization || !current.DecidedAt().Equal(registrarBaseAt) ||
		current.Policy().String() != "exception-disclosure/v1" {
		t.Fatalf("待授权决定读回走样：%+v", current)
	}
	if _, disclosed := current.Content(); disclosed {
		t.Fatal("待授权决定读回带了内容")
	}

	// 同三维重登：幂等，不是错误。
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = store.Save(txCtx, tenant, awaiting)
		return err
	}); err != nil || outcome != ports.DisclosureDecisionAlreadyRecorded {
		t.Fatalf("重登同一决定：err=%v outcome=%d，想要 AlreadyRecorded", err, outcome)
	}

	// 授权之后的新决定是新的一行，成为当前；旧行原样留着。
	disclosed := recordedDecision(t, "ep-1", "acct-1", domain.DiscloseToCustomer, "content/eta-breach/v1", registrarBaseAt.Add(time.Hour))
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = store.SaveAuthorization(txCtx, ports.DisclosureAuthorization{
			Tenant: tenant, Decision: disclosed, Authorizes: registrarBaseAt, Operator: "https://issuer#bob",
		})
		return err
	}); err != nil || outcome != ports.DisclosureDecisionSaved {
		t.Fatalf("登披露决定：err=%v outcome=%d", err, outcome)
	}
	current, _, err = store.FindCurrent(ctx, tenant, episode, customer)
	if err != nil {
		t.Fatalf("再读决定：%v", err)
	}
	content, isDisclosed := current.Content()
	if current.Conclusion() != domain.DiscloseToCustomer || !isDisclosed || content.String() != "content/eta-breach/v1" {
		t.Fatalf("当前决定应为新登的披露那一份，实得 %+v", current)
	}
	var rows int
	if err := fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM visibility_exception.disclosure_decision WHERE tenant_id = $1 AND episode_id = $2`,
		tenant.String(), "ep-1").Scan(&rows); err != nil {
		t.Fatalf("数决定行：%v", err)
	}
	if rows != 2 {
		t.Fatalf("决定行 = %d，想要 2：新决定不得改写旧行", rows)
	}

	// 另一客户、另一租户各自看不见。
	if _, found, err := store.FindCurrent(ctx, tenant, episode,
		build(t, domain.NewCustomerAccountReference, "acct-2")); err != nil || found {
		t.Fatalf("跨客户可见：err=%v found=%v", err, found)
	}
	if _, found, err := store.FindCurrent(ctx, build(t, domain.NewTenantID, "tenant-b"), episode, customer); err != nil || found {
		t.Fatalf("跨租户可见：err=%v found=%v", err, found)
	}
}

// Covers: 0023 决定表的两条 CHECK——结论封闭三值；披露必带内容、其余必不带。领域构造门
// 拦在前面，库是第二道网，这里直接写行证网在。
func TestDisclosureDecisionRowsAreCheckedByTheDatabase(t *testing.T) {
	fixture := newRegistrarFixture(t)

	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_decision
			(tenant_id, episode_id, customer_ref, decided_at, disclosure_policy_ref, conclusion, content_ref)
		 VALUES ('tenant-a', 'ep-1', 'acct-1', now(), 'p/v1', 'DISCLOSE', NULL)`); err == nil {
		t.Fatal("库接受了没有内容的披露决定")
	}
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_decision
			(tenant_id, episode_id, customer_ref, decided_at, disclosure_policy_ref, conclusion, content_ref)
		 VALUES ('tenant-a', 'ep-1', 'acct-1', now(), 'p/v1', 'NOT_YET_DISCLOSABLE', 'content/x')`); err == nil {
		t.Fatal("库接受了带内容的暂不披露决定")
	}
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_decision
			(tenant_id, episode_id, customer_ref, decided_at, disclosure_policy_ref, conclusion, content_ref)
		 VALUES ('tenant-a', 'ep-1', 'acct-1', now(), 'p/v1', 'MAYBE', NULL)`); err == nil {
		t.Fatal("库接受了三值之外的披露结论")
	}
}

// Covers: 0037——披露决定上由人作出的那一半。发起人只在那一行还没有决定人时写；授权确认记下确认人并指回被确认的
// 那一份；同一份待授权只收一次确认，换个时刻再确认也不落第二份；同一（发作期，客户）换个时刻再落一份按规则形成的
// 决定同样不落。
func TestDisclosureDecisionsKeepWhoDecidedAndAcceptOneAuthorizationPerAwaitingDecision(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	store := decisionStore(t, fixture)
	ctx := t.Context()
	within := func(work func(context.Context) error) {
		t.Helper()
		if err := fixture.transactor.WithinTransaction(ctx, work); err != nil {
			t.Fatalf("事务：%v", err)
		}
	}

	awaiting := recordedDecision(t, "ep-1", "acct-1", domain.AwaitingAuthorization, "", registrarBaseAt)
	within(func(txCtx context.Context) error {
		if _, err := store.Save(txCtx, tenant, awaiting); err != nil {
			return err
		}
		if err := store.RecordDecider(txCtx, tenant, awaiting, "https://issuer#alice"); err != nil {
			return err
		}
		// 第二次记不改写第一次的。
		return store.RecordDecider(txCtx, tenant, awaiting, "https://issuer#mallory")
	})

	// 同一（发作期，客户）换个时刻再落一份按规则形成的决定：主键拦不住（决定时间不同），部分唯一索引拦。
	var outcome ports.DisclosureDecisionSaveOutcome
	within(func(txCtx context.Context) error {
		var err error
		outcome, err = store.Save(txCtx, tenant, recordedDecision(t, "ep-1", "acct-1", domain.NotYetDisclosable, "", registrarBaseAt.Add(time.Minute)))
		return err
	})
	if outcome != ports.DisclosureDecisionAlreadyRecorded {
		t.Fatalf("第二份按规则形成的决定 outcome=%d，想要 AlreadyRecorded", outcome)
	}

	authorize := func(at time.Time, operator string) ports.DisclosureDecisionSaveOutcome {
		t.Helper()
		var outcome ports.DisclosureDecisionSaveOutcome
		within(func(txCtx context.Context) error {
			var err error
			outcome, err = store.SaveAuthorization(txCtx, ports.DisclosureAuthorization{
				Tenant:     tenant,
				Decision:   recordedDecision(t, "ep-1", "acct-1", domain.DiscloseToCustomer, "content/eta-breach/v1", at),
				Authorizes: registrarBaseAt, Operator: operator,
			})
			return err
		})
		return outcome
	}
	if got := authorize(registrarBaseAt.Add(time.Hour), "https://issuer#bob"); got != ports.DisclosureDecisionSaved {
		t.Fatalf("首次确认 outcome=%d", got)
	}
	if got := authorize(registrarBaseAt.Add(2*time.Hour), "https://issuer#carol"); got != ports.DisclosureDecisionAlreadyRecorded {
		t.Fatalf("换个时刻再确认 outcome=%d，想要 AlreadyRecorded", got)
	}

	rows, err := fixture.pool.Query(ctx,
		`SELECT conclusion, decided_by, authorizes_decided_at IS NOT NULL
		   FROM visibility_exception.disclosure_decision
		  WHERE tenant_id = $1 AND episode_id = 'ep-1' ORDER BY decided_at`, tenant.String())
	if err != nil {
		t.Fatalf("读决定行：%v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var conclusion, by string
		var authorization bool
		if err := rows.Scan(&conclusion, &by, &authorization); err != nil {
			t.Fatalf("扫决定行：%v", err)
		}
		got = append(got, fmt.Sprintf("%s|%s|%t", conclusion, by, authorization))
	}
	want := []string{"AWAITING_AUTHORIZATION|https://issuer#alice|false", "DISCLOSE|https://issuer#bob|true"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("决定行 = %v，想要 %v", got, want)
	}

	// 没有确认人、或拿一份不是披露的决定来当确认：写入口拒。
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := store.SaveAuthorization(txCtx, ports.DisclosureAuthorization{
			Tenant: tenant, Decision: awaiting, Authorizes: registrarBaseAt, Operator: "https://issuer#bob",
		})
		return err
	}); err == nil {
		t.Fatal("一份待授权的决定被当成授权确认落了库")
	}
}

// Covers: 两个并发窗口各由一条部分唯一索引守住。两个人同时确认同一份待授权——各带各的时刻，主键不相撞——恰有一笔
// 落成；两个人同时对同一条信号发起判断，恰有一行按规则形成的决定。
func TestConcurrentDisclosureDecisionsAndAuthorizationsLandExactlyOnce(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	store := decisionStore(t, fixture)
	ctx := t.Context()

	race := func(first, second func(context.Context) (ports.DisclosureDecisionSaveOutcome, error)) int {
		t.Helper()
		start := make(chan struct{})
		outcomes := make(chan ports.DisclosureDecisionSaveOutcome, 2)
		errs := make(chan error, 2)
		for _, work := range []func(context.Context) (ports.DisclosureDecisionSaveOutcome, error){first, second} {
			go func() {
				<-start
				var outcome ports.DisclosureDecisionSaveOutcome
				err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
					var err error
					outcome, err = work(txCtx)
					return err
				})
				outcomes <- outcome
				errs <- err
			}()
		}
		close(start)
		saved := 0
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatalf("并发写：%v", err)
			}
			if <-outcomes == ports.DisclosureDecisionSaved {
				saved++
			}
		}
		return saved
	}

	for round := range 20 {
		episode := fmt.Sprintf("ep-race-%d", round)
		ruleFormed := func(offset time.Duration) func(context.Context) (ports.DisclosureDecisionSaveOutcome, error) {
			return func(txCtx context.Context) (ports.DisclosureDecisionSaveOutcome, error) {
				return store.Save(txCtx, tenant, recordedDecision(t, episode, "acct-1", domain.AwaitingAuthorization, "", registrarBaseAt.Add(offset)))
			}
		}
		if saved := race(ruleFormed(0), ruleFormed(time.Millisecond)); saved != 1 {
			t.Fatalf("第 %d 轮：同时判断落成 %d 份，想要 1", round, saved)
		}
		var awaitingAt time.Time
		if err := fixture.pool.QueryRow(ctx,
			`SELECT decided_at FROM visibility_exception.disclosure_decision WHERE tenant_id = $1 AND episode_id = $2`,
			tenant.String(), episode).Scan(&awaitingAt); err != nil {
			t.Fatalf("第 %d 轮：按规则形成的决定该恰有一行：%v", round, err)
		}
		authorization := func(offset time.Duration, operator string) func(context.Context) (ports.DisclosureDecisionSaveOutcome, error) {
			return func(txCtx context.Context) (ports.DisclosureDecisionSaveOutcome, error) {
				return store.SaveAuthorization(txCtx, ports.DisclosureAuthorization{
					Tenant:     tenant,
					Decision:   recordedDecision(t, episode, "acct-1", domain.DiscloseToCustomer, "content/x", awaitingAt.Add(offset)),
					Authorizes: awaitingAt, Operator: operator,
				})
			}
		}
		if saved := race(authorization(time.Hour, "https://issuer#bob"), authorization(time.Hour+time.Millisecond, "https://issuer#carol")); saved != 1 {
			t.Fatalf("第 %d 轮：同时确认落成 %d 份，想要 1", round, saved)
		}
		var disclosed int
		if err := fixture.pool.QueryRow(ctx,
			`SELECT count(*) FROM visibility_exception.disclosure_decision WHERE tenant_id = $1 AND episode_id = $2 AND conclusion = 'DISCLOSE'`,
			tenant.String(), episode).Scan(&disclosed); err != nil || disclosed != 1 {
			t.Fatalf("第 %d 轮：披露决定 %d 行（err=%v），想要 1", round, disclosed, err)
		}
	}
}
