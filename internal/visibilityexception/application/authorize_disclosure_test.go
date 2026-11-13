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

// disclosureLedgerDouble 把人的那一半记在测试看得见的地方，授权确认写回同一只决定册替身。alreadyAuthorized 模拟
// 另一个人抢先确认：落库答 AlreadyRecorded，并把赢家放进决定册。
type disclosureLedgerDouble struct {
	decisions         *decisionStoreDouble
	deciders          []string
	authorizations    []ports.DisclosureAuthorization
	saveErr           error
	alreadyAuthorized *domain.DisclosureDecision
}

func (double *disclosureLedgerDouble) RecordDecider(_ context.Context, _ domain.TenantID, _ domain.DisclosureDecision, operator string) error {
	double.deciders = append(double.deciders, operator)
	return nil
}

func (double *disclosureLedgerDouble) SaveAuthorization(_ context.Context, authorization ports.DisclosureAuthorization) (ports.DisclosureDecisionSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.DisclosureDecisionSaveOutcomeInvalid, double.saveErr
	}
	key := decisionKey{tenant: authorization.Tenant, episode: authorization.Decision.Episode(), customer: authorization.Decision.Customer()}
	if double.alreadyAuthorized != nil {
		double.decisions.current[key] = *double.alreadyAuthorized
		return ports.DisclosureDecisionAlreadyRecorded, nil
	}
	double.authorizations = append(double.authorizations, authorization)
	double.decisions.current[key] = authorization.Decision
	return ports.DisclosureDecisionSaved, nil
}

// episodeByID 按标识在同一只发作期替身里找：标识对不上这件对象这一类信号的任何一期就答没有。
type episodeByID struct{ episodes *episodeStoreDouble }

func (lookup episodeByID) FindEpisode(_ context.Context, tenant domain.TenantID, parcel domain.TrackedParcelReference, kind domain.ExceptionSignalKindReference, id domain.EpisodeID) (*domain.SignalEpisode, bool, error) {
	for key, episode := range lookup.episodes.latestKind {
		if key.tenant == tenant && key.parcel == parcel && key.kind == kind && episode.ID() == id {
			return episode, true, nil
		}
	}
	for _, episode := range lookup.episodes.byID {
		snapshot := episode.Snapshot()
		if snapshot.Parcel == parcel && snapshot.Kind == kind && episode.ID() == id {
			return episode, true, nil
		}
	}
	return nil, false, nil
}

type authorizeFixture struct {
	*decideFixture
	ledger     *disclosureLedgerDouble
	authorizer *application.DecideDisclosureHandler
}

// newAuthorizeFixture 先按「披露但未批准自动发布」的规则经运营口判出一份`待授权`，再给出一只时钟晚一小时的编排用来
// 确认。
func newAuthorizeFixture(t *testing.T) *authorizeFixture {
	t.Helper()
	base := newDecideFixture(t)
	base.rules.rule.AutoRelease = false
	ledger := &disclosureLedgerDouble{decisions: base.decisions}
	deps := application.DecideDisclosureDeps{
		Episodes: base.episodes, EpisodeByID: episodeByID{base.episodes}, Customers: base.customers, Rules: base.rules, Decisions: base.decisions, Ledger: ledger,
		Clock: fixedClock{at: disclosureEpisodeAt.Add(time.Hour)},
	}
	command := decideCommand(t)
	command.Operator = "https://issuer#alice"
	command.Episode = mustValue(t, domain.NewEpisodeID, "episode-1")
	result, err := application.NewDecideDisclosureHandler(deps).Handle(context.Background(), command)
	if err != nil || result.Outcome() != application.DisclosureDecided {
		t.Fatalf("预置待授权决定：err=%v outcome=%q", err, result.Outcome())
	}
	deps.Clock = fixedClock{at: disclosureEpisodeAt.Add(2 * time.Hour)}
	return &authorizeFixture{decideFixture: base, ledger: ledger, authorizer: application.NewDecideDisclosureHandler(deps)}
}

func authorizeCommand(t *testing.T) application.AuthorizeDisclosureCommand {
	t.Helper()
	base := decideCommand(t)
	return application.AuthorizeDisclosureCommand{
		TenantID: base.TenantID, Parcel: base.Parcel, Kind: base.Kind,
		Episode: mustValue(t, domain.NewEpisodeID, "episode-1"), Operator: "https://issuer#bob",
	}
}

// Covers: `UC-VE-006` 步 5「默认等待授权角色确认」——经运营口判出的决定记下发起人；授权角色确认后形成一份新的`披露`
// 决定：内容快照与规则引用取确认这一刻的规则，指回被确认的那份`待授权`，记下确认人；再确认一次交回原决定、不落第二份。
func TestAuthorizingAnAwaitingDecisionFormsADiscloseDecisionOnce(t *testing.T) {
	fixture := newAuthorizeFixture(t)
	if len(fixture.ledger.deciders) != 1 || fixture.ledger.deciders[0] != "https://issuer#alice" {
		t.Fatalf("决定人 = %v——该记下经运营口发起判断的操作者", fixture.ledger.deciders)
	}

	result, err := fixture.authorizer.Authorize(context.Background(), authorizeCommand(t))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	decision, present := result.Decision()
	if result.Outcome() != application.DisclosureAuthorized || !present || decision.Conclusion() != domain.DiscloseToCustomer {
		t.Fatalf("outcome = %q decision = %+v", result.Outcome(), decision)
	}
	content, disclosed := decision.Content()
	if !disclosed || content != fixture.rules.rule.Content || decision.Policy() != fixture.rules.rule.Policy {
		t.Fatalf("确认形成的决定没有取规则的内容快照与引用：%+v", decision)
	}
	if len(fixture.ledger.authorizations) != 1 {
		t.Fatalf("授权确认落了 %d 次", len(fixture.ledger.authorizations))
	}
	recorded := fixture.ledger.authorizations[0]
	if recorded.Operator != "https://issuer#bob" || !recorded.Authorizes.Equal(disclosureEpisodeAt.Add(time.Hour)) ||
		!recorded.Decision.DecidedAt().Equal(disclosureEpisodeAt.Add(2*time.Hour)) {
		t.Fatalf("授权确认 = %+v——该指回被确认的那一份并记下确认人", recorded)
	}

	again, err := fixture.authorizer.Authorize(context.Background(), authorizeCommand(t))
	if err != nil || again.Outcome() != application.DisclosureExistingResult || len(fixture.ledger.authorizations) != 1 {
		t.Fatalf("再确认：err=%v outcome=%q 落库 %d 次", err, again.Outcome(), len(fixture.ledger.authorizations))
	}
}

// Covers: 只有`待授权`能被确认——还没判过的信号、判成`暂不披露`的信号都没有可确认的；确认不替规则说披露条件成立。
func TestOnlyAnAwaitingDecisionCanBeAuthorized(t *testing.T) {
	undecided := newDecideFixture(t)
	ledger := &disclosureLedgerDouble{decisions: undecided.decisions}
	handler := application.NewDecideDisclosureHandler(application.DecideDisclosureDeps{
		Episodes: undecided.episodes, EpisodeByID: episodeByID{undecided.episodes}, Customers: undecided.customers, Rules: undecided.rules, Decisions: undecided.decisions,
		Ledger: ledger, Clock: fixedClock{at: disclosureEpisodeAt.Add(2 * time.Hour)},
	})
	result, err := handler.Authorize(context.Background(), authorizeCommand(t))
	if err != nil || result.Outcome() != application.DisclosureNotAwaitingAuthorization {
		t.Fatalf("没判过的信号：err=%v outcome=%q", err, result.Outcome())
	}

	undecided.rules.rule = ports.ExceptionDisclosureRule{Policy: undecided.rules.rule.Policy}
	if decided, err := handler.Handle(context.Background(), decideCommand(t)); err != nil || decided.Outcome() != application.DisclosureDecided {
		t.Fatalf("预置暂不披露：err=%v outcome=%q", err, decided.Outcome())
	}
	result, err = handler.Authorize(context.Background(), authorizeCommand(t))
	if err != nil || result.Outcome() != application.DisclosureNotAwaitingAuthorization || len(ledger.authorizations) != 0 {
		t.Fatalf("暂不披露的信号：err=%v outcome=%q 落库 %d 次", err, result.Outcome(), len(ledger.authorizations))
	}

	anonymous := authorizeCommand(t)
	anonymous.Operator = ""
	if result, err = handler.Authorize(context.Background(), anonymous); err != nil || result.Outcome() != application.DecideDisclosureNotAccepted {
		t.Fatalf("没有确认人：err=%v outcome=%q", err, result.Outcome())
	}
}

// Covers: 决定停在`待授权`期间规则变了——规则此刻说不披露，确认如实答条件已不成立；规则被撤下，答未决（规则未配置）。
// 两种都不落`披露`决定：确认的人只放行发布。
func TestAuthorizationFollowsTheRuleAsItStandsNow(t *testing.T) {
	fixture := newAuthorizeFixture(t)
	fixture.rules.rule.Disclosable = false
	result, err := fixture.authorizer.Authorize(context.Background(), authorizeCommand(t))
	if err != nil || result.Outcome() != application.DisclosureConditionNoLongerMet {
		t.Fatalf("规则改成不披露：err=%v outcome=%q", err, result.Outcome())
	}

	fixture.rules.configured = false
	result, err = fixture.authorizer.Authorize(context.Background(), authorizeCommand(t))
	if err != nil || result.Outcome() != application.DecideDisclosureUndecided || result.UndecidedReason() != application.DisclosureRuleNotConfigured {
		t.Fatalf("规则被撤下：err=%v outcome=%q reason=%q", err, result.Outcome(), result.UndecidedReason())
	}
	if len(fixture.ledger.authorizations) != 0 {
		t.Fatalf("规则不允许时仍落了 %d 份披露决定", len(fixture.ledger.authorizations))
	}
}

// Covers: 两个人同时确认——后落的那一笔读回赢家、答已有结果；决定册写不动答未决。
func TestALostAuthorizationRaceReturnsTheWinner(t *testing.T) {
	fixture := newAuthorizeFixture(t)
	awaiting := fixture.decisions.current[decisionKey{
		tenant: decideCommand(t).TenantID, episode: mustValue(t, domain.NewEpisodeID, "episode-1"),
		customer: mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
	}]
	winner, err := domain.AuthorizeDisclosure(awaiting, fixture.rules.rule.Policy, fixture.rules.rule.Content, disclosureEpisodeAt.Add(90*time.Minute))
	if err != nil {
		t.Fatalf("构造赢家：%v", err)
	}
	fixture.ledger.alreadyAuthorized = &winner
	result, err := fixture.authorizer.Authorize(context.Background(), authorizeCommand(t))
	decision, _ := result.Decision()
	if err != nil || result.Outcome() != application.DisclosureExistingResult || !decision.DecidedAt().Equal(winner.DecidedAt()) {
		t.Fatalf("输掉的确认：err=%v outcome=%q decision=%+v", err, result.Outcome(), decision)
	}

	failing := newAuthorizeFixture(t)
	failing.ledger.saveErr = errors.New("store down")
	result, err = failing.authorizer.Authorize(context.Background(), authorizeCommand(t))
	if err != nil || result.Outcome() != application.DecideDisclosureUndecided || result.UndecidedReason() != application.DisclosureDecisionStoreUnavailable {
		t.Fatalf("决定册写不动：err=%v outcome=%q", err, result.Outcome())
	}
}

// Covers: 人作的决定钉在他看过的那一期上。同一件包裹同一类信号因另一个原因又开了一期、成了「最近的那一期」之后，
// 对原来那一期的确认仍然落在原来那一期；不指名哪一期的确认、经运营口不指名哪一期的判断都不受理；指名一期不属于这件
// 包裹这一类信号的也不受理。
func TestAuthorizationStaysOnTheNamedEpisodeWhenANewerOneOpens(t *testing.T) {
	fixture := newAuthorizeFixture(t)
	first := fixture.episodes.latestKind[episodeKey{
		tenant: decideCommand(t).TenantID, parcel: decideCommand(t).Parcel, kind: decideCommand(t).Kind,
	}]
	fixture.episodes.byID = map[string]*domain.SignalEpisode{"episode-1": first}
	later := raiseCommandFor(t, "movement-fact/parcel-1/MF-2", disclosureEpisodeAt.Add(30*time.Minute))
	second, err := domain.OpenEpisode(mustValue(t, domain.NewEpisodeID, "episode-2"), domain.SignalHit{
		Kind: later.Kind, Parcel: later.Parcel, Cause: later.Cause, Rule: later.Rule,
		Confidence: later.Confidence, Basis: later.Basis, At: later.HitAt,
	})
	if err != nil {
		t.Fatalf("open second episode: %v", err)
	}
	fixture.episodes.latestKind[episodeKey{tenant: decideCommand(t).TenantID, parcel: later.Parcel, kind: later.Kind}] = second

	result, err := fixture.authorizer.Authorize(context.Background(), authorizeCommand(t))
	decision, _ := result.Decision()
	if err != nil || result.Outcome() != application.DisclosureAuthorized || decision.Episode().String() != "episode-1" {
		t.Fatalf("确认原来那一期：err=%v outcome=%q episode=%q", err, result.Outcome(), decision.Episode())
	}

	unnamed := authorizeCommand(t)
	unnamed.Episode = domain.EpisodeID{}
	if result, err = fixture.authorizer.Authorize(context.Background(), unnamed); err != nil || result.Outcome() != application.DecideDisclosureNotAccepted {
		t.Fatalf("不指名哪一期的确认：err=%v outcome=%q", err, result.Outcome())
	}
	stray := authorizeCommand(t)
	stray.Episode = mustValue(t, domain.NewEpisodeID, "episode-of-another-parcel")
	if result, err = fixture.authorizer.Authorize(context.Background(), stray); err != nil || result.Outcome() != application.DecideDisclosureNotAccepted {
		t.Fatalf("指名别处的一期：err=%v outcome=%q", err, result.Outcome())
	}
	operatorWithoutEpisode := decideCommand(t)
	operatorWithoutEpisode.Operator = "https://issuer#alice"
	if result, err = fixture.authorizer.Handle(context.Background(), operatorWithoutEpisode); err != nil || result.Outcome() != application.DecideDisclosureNotAccepted {
		t.Fatalf("经运营口不指名哪一期的判断：err=%v outcome=%q", err, result.Outcome())
	}
}
