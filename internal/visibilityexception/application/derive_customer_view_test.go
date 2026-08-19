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

type disclosureDouble struct {
	answer     ports.DisclosureAnswer
	configured bool
	err        error
	calls      int
}

func (double *disclosureDouble) AssessDisclosure(
	_ context.Context,
	_ domain.CustomerAccountReference,
	_ domain.TrackingProjection,
) (ports.DisclosureAnswer, bool, error) {
	double.calls++
	if double.err != nil {
		return ports.DisclosureAnswer{}, false, double.err
	}
	return double.answer, double.configured, nil
}

type viewStoreKey struct {
	tenant   domain.TenantID
	customer domain.CustomerAccountReference
	parcel   domain.TrackedParcelReference
}

type viewStoreDouble struct {
	views   map[viewStoreKey]domain.CustomerTrackingView
	findErr error
	saveErr error
	saved   int
}

func newViewStore() *viewStoreDouble {
	return &viewStoreDouble{views: map[viewStoreKey]domain.CustomerTrackingView{}}
}

func (double *viewStoreDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
) (domain.CustomerTrackingView, bool, error) {
	if double.findErr != nil {
		return domain.CustomerTrackingView{}, false, double.findErr
	}
	view, found := double.views[viewStoreKey{tenant: tenant, customer: customer, parcel: parcel}]
	return view, found, nil
}

func (double *viewStoreDouble) Save(_ context.Context, tenant domain.TenantID, view domain.CustomerTrackingView) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.views[viewStoreKey{tenant: tenant, customer: view.Customer(), parcel: view.Parcel()}] = view
	double.saved++
	return nil
}

type viewIdentityDouble struct {
	next int
	err  error
}

func (double *viewIdentityDouble) NextCustomerViewVersionID(_ context.Context) (domain.CustomerViewVersionID, error) {
	if double.err != nil {
		return domain.CustomerViewVersionID{}, double.err
	}
	double.next++
	return domain.NewCustomerViewVersionID("view-" + string(rune('0'+double.next)))
}

type viewDownstreamDouble struct {
	intents []ports.CustomerViewHandoffIntent
	err     error
}

func (double *viewDownstreamDouble) HandOffCustomerView(
	_ context.Context,
	intent ports.CustomerViewHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type viewFixture struct {
	handler    *application.DeriveCustomerViewHandler
	policy     *disclosureDouble
	views      *viewStoreDouble
	identities *viewIdentityDouble
	downstream *viewDownstreamDouble
}

// shownAnswer 是四维全部获准展示的策略答复。
func shownAnswer(t *testing.T) ports.DisclosureAnswer {
	t.Helper()
	shown := func(raw string) ports.DimensionDisclosure {
		return ports.DimensionDisclosure{
			State:   domain.DimensionShown,
			Content: mustValue(t, domain.NewViewContentReference, raw),
		}
	}
	return ports.DisclosureAnswer{
		Milestones: shown("milestones/v1"),
		ETA:        shown("eta/v1"),
		Final:      shown("final/v1"),
		Note:       shown("note/v1"),
	}
}

func newViewFixture(t *testing.T) *viewFixture {
	t.Helper()
	fixture := &viewFixture{
		policy:     &disclosureDouble{answer: shownAnswer(t), configured: true},
		views:      newViewStore(),
		identities: &viewIdentityDouble{},
		downstream: &viewDownstreamDouble{},
	}
	fixture.handler = application.NewDeriveCustomerViewHandler(application.DeriveCustomerViewDeps{
		Policy:     fixture.policy,
		Views:      fixture.views,
		Identities: fixture.identities,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: factOccurredAt.Add(3 * time.Hour)},
	})
	return fixture
}

func viewCustomer(t *testing.T) domain.CustomerAccountReference {
	t.Helper()
	return mustValue(t, domain.NewCustomerAccountReference, "customer-1")
}

// trackedProjection 造一份最小的有效投影版本：单事实、已归类。
func trackedProjection(t *testing.T, version string) domain.TrackingProjection {
	t.Helper()
	parcel := mustValue(t, domain.NewTrackedParcelReference, "parcel-1")
	fact, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      domain.SourceNodeOperations,
		Parcel:      parcel,
		Fact:        mustValue(t, domain.NewSourceFactReference, "fact-1"),
		Kind:        mustValue(t, domain.NewSourceFactKind, "node-intake"),
		Version:     mustValue(t, domain.NewSourceFactVersion, "fact-1/v1"),
		OccurredAt:  factOccurredAt,
		EffectiveAt: factOccurredAt,
		ReceivedAt:  factOccurredAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("new accepted source fact: %v", err)
	}
	classification, err := domain.ClassifyMilestone(
		fact,
		mustValue(t, domain.NewMilestoneReference, "PICKED_UP"),
		mustValue(t, domain.NewMappingVersionReference, "milestone-map/v1"),
	)
	if err != nil {
		t.Fatalf("classify milestone: %v", err)
	}
	projection, err := domain.DeriveTrackingProjection(
		mustValue(t, domain.NewProjectionVersionID, version),
		parcel,
		[]domain.MilestoneClassification{classification},
		factOccurredAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("derive tracking projection: %v", err)
	}
	return projection
}

func viewCommand(t *testing.T, projectionVersion string) application.DeriveCustomerViewCommand {
	t.Helper()
	return application.DeriveCustomerViewCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Customer:   viewCustomer(t),
		Projection: trackedProjection(t, projectionVersion),
	}
}

// Covers: CONTEXT 客户全程追踪视图生命周期「当前投影……首次满足展示条件 → 形成货主
// 客户账户隔离的客户视图版本」与「客户全程追踪视图只基于当前有效的全程追踪投影……
// 形成」——首版视图锚定投影版本、账户是字段、意图由视图版本认领恰发一份。点名
// `AT-VE-156`「当前投影包含已批准公开里程碑→展示标准语义……不复制内部来源状态」
// ——展示维只携带策略批准的内容引用。
func TestFirstQualifyingProjectionPublishesAnAccountIsolatedView(t *testing.T) {
	fixture := newViewFixture(t)

	result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.CustomerViewPublished {
		t.Fatalf("outcome = %q, want PUBLISHED", result.Outcome())
	}
	view, present := result.View()
	if !present {
		t.Fatal("a published result carries no view")
	}
	if view.Customer() != viewCustomer(t) {
		t.Fatalf("view customer = %q; account isolation must be a field on the view", view.Customer())
	}
	if view.BasedOn().String() != "projection-1" {
		t.Fatalf("view based on %q, want the projection version it derived from", view.BasedOn())
	}
	if _, superseding := view.PriorVersion(); superseding {
		t.Fatal("the first view version claims to supersede a prior one")
	}
	dimensions := view.Dimensions()
	for name, dimension := range map[string]domain.ViewDimension{
		"milestones": dimensions.Milestones,
		"eta":        dimensions.ETA,
		"final":      dimensions.Final,
		"note":       dimensions.Note,
	} {
		if dimension.State() != domain.DimensionShown {
			t.Fatalf("%s dimension = %q, want SHOWN", name, dimension.State())
		}
	}
	if fixture.views.saved != 1 {
		t.Fatalf("saved %d views, want 1", fixture.views.saved)
	}
	if len(fixture.downstream.intents) != 1 ||
		fixture.downstream.intents[0].View.Version() != view.Version() {
		t.Fatal("exactly one handoff intent claimed by the published view version was expected")
	}
	if result.HandoffReference() != "" {
		t.Fatal("a delivered intent still left a resumable handoff reference")
	}
}

// Covers: 派工约束「未配置时四维全部 PendDimension，如实说等，不虚构可见性」与 CONTEXT
// 「某一维信息待确认或不可披露 → 只对该维返回待确认、暂不可用或不展示，不虚构里程碑、
// ETA、终局或异常说明」——披露规则属待登记实例参数，空白不是「不展示」的披露决定，
// 视图照常发布。
func TestAnUnconfiguredDisclosurePolicyPendsEveryDimensionInsteadOfInventingVisibility(t *testing.T) {
	fixture := newViewFixture(t)
	fixture.policy.configured = false

	result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.CustomerViewPublished {
		t.Fatalf("outcome = %q, want PUBLISHED; an unconfigured policy must not block the view", result.Outcome())
	}
	view, _ := result.View()
	dimensions := view.Dimensions()
	for name, dimension := range map[string]domain.ViewDimension{
		"milestones": dimensions.Milestones,
		"eta":        dimensions.ETA,
		"final":      dimensions.Final,
		"note":       dimensions.Note,
	} {
		if dimension.State() != domain.DimensionPendingConfirmation {
			t.Fatalf("%s dimension = %q, want PENDING_CONFIRMATION", name, dimension.State())
		}
		if _, has := dimension.Content(); has {
			t.Fatalf("%s dimension invented content while pending", name)
		}
	}
}

// Covers: CONTEXT「某一维信息待确认或不可披露 → 只对该维返回待确认、暂不可用或不展示」
// 与「只展示适用服务与授权允许的公开里程碑、地点粒度、ETA、终局和说明」——一维不可
// 披露只压那一维，其余维照常展示。点名 `AT-VE-157`「投影含内部控制位置、非公开节点
// 和供应商商业信息→按规则过滤，不进入客户视图」的逐维不展示格。
func TestANonDisclosableDimensionIsWithheldAloneWithoutHidingTheOthers(t *testing.T) {
	fixture := newViewFixture(t)
	fixture.policy.answer.ETA = ports.DimensionDisclosure{State: domain.DimensionNotDisclosed}

	result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	view, _ := result.View()
	dimensions := view.Dimensions()
	if dimensions.ETA.State() != domain.DimensionNotDisclosed {
		t.Fatalf("eta dimension = %q, want NOT_DISCLOSED", dimensions.ETA.State())
	}
	if dimensions.Milestones.State() != domain.DimensionShown ||
		dimensions.Final.State() != domain.DimensionShown ||
		dimensions.Note.State() != domain.DimensionShown {
		t.Fatal("withholding one dimension suppressed the others")
	}
}

// Covers: 派工幂等约束「同一投影版本不重发视图」与 ADR-0043「重放重发同一份意图」——
// 重放不签新身份、不重问策略、不写库，但把已有视图的同一份意图再交一次：只答已有结果
// 就收工，一份首次发布失败的视图会永远停在「已发布、下游不知道」。
func TestTheSameProjectionVersionDoesNotRepublishButResendsTheSameIntent(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstView, _ := first.View()
	issuedBefore, savedBefore := fixture.identities.next, fixture.views.saved
	policyCallsBefore := fixture.policy.calls

	replay, err := fixture.handler.Handle(ctx, viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}

	if replay.Outcome() != application.CustomerViewExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	replayed, present := replay.View()
	if !present || replayed.Version() != firstView.Version() {
		t.Fatal("the replay did not return the view that was already published")
	}
	if fixture.identities.next != issuedBefore {
		t.Fatal("a replay consumed a new view version identity")
	}
	if fixture.views.saved != savedBefore {
		t.Fatal("a replay wrote the view store again")
	}
	if fixture.policy.calls != policyCallsBefore {
		t.Fatal("a replay asked the disclosure policy again")
	}
	if len(fixture.downstream.intents) != 2 ||
		fixture.downstream.intents[1].View.Version() != firstView.Version() {
		t.Fatal("the replay must resend the same intent claimed by the existing view version")
	}
}

// Covers: CONTEXT「来源更正、事实有效性变化、身份谱系变化、ETA 新版本或终局更正必须
// 重新派生客户视图并形成追加更正或明确替代关系。原已发布版本保留」——投影换版本时
// 新视图指回前版并锚定新投影。点名 `AT-VE-161`「来源更正使原里程碑或终局失效→形成
// 追加更正/替代视图，原版本保留且不再作为当前结果」。
func TestANewerProjectionSupersedesTheCurrentViewKeepingHistory(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstView, _ := first.View()

	second, err := fixture.handler.Handle(ctx, viewCommand(t, "projection-2"))
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}

	if second.Outcome() != application.CustomerViewPublished {
		t.Fatalf("outcome = %q, want PUBLISHED", second.Outcome())
	}
	view, _ := second.View()
	prior, superseding := view.PriorVersion()
	if !superseding || prior != firstView.Version() {
		t.Fatalf("prior version = %q, want the superseded view %q", prior, firstView.Version())
	}
	if view.BasedOn().String() != "projection-2" {
		t.Fatalf("view based on %q, want the newer projection version", view.BasedOn())
	}
	if view.Version() == firstView.Version() {
		t.Fatal("the superseding view reused the prior version identity")
	}
}

// Covers: ADR-0043「首次交付失败不改写业务结果……另留一条发布续办引用，与『结果未
// 形成』的续办分开」——意图交不出去时视图保持已发布，只留可续办引用。
func TestAFailedHandoffKeepsThePublishedViewWithAResumableIntentReference(t *testing.T) {
	fixture := newViewFixture(t)
	fixture.downstream.err = errors.New("downstream unavailable")

	result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.CustomerViewPublished {
		t.Fatalf("outcome = %q, want PUBLISHED; a failed handoff must not rewrite the published view", result.Outcome())
	}
	if result.HandoffReference() == "" {
		t.Fatal("a failed handoff left no resumable reference")
	}
	if fixture.views.saved != 1 {
		t.Fatalf("saved %d views, want the published view kept", fixture.views.saved)
	}
}

// Covers: 结果语义的未决半边——策略调不通是依赖故障不是「规则未配置」，形成未决且
// 不消耗视图版本标识、不写库；混起来会让一次故障被读成租户还没登记披露规则。
func TestAnUnavailableDisclosurePolicyIsUndecidedWithoutConsumingAnIdentity(t *testing.T) {
	fixture := newViewFixture(t)
	fixture.policy.err = errors.New("disclosure policy unavailable")

	result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.CustomerViewUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.UndecidedReason() != application.DisclosurePolicyUnavailable {
		t.Fatalf("reason = %q, want DISCLOSURE_POLICY_UNAVAILABLE", result.UndecidedReason())
	}
	if fixture.identities.next != 0 {
		t.Fatal("an undecided round consumed a scarce view version identity")
	}
	if fixture.views.saved != 0 {
		t.Fatal("an undecided round wrote the view store")
	}
}

// Covers: 同一条未决语义的存储半边——视图库读不回时不重新发布也不虚构已有结果。
func TestAnUnreadableViewStoreIsUndecidedRatherThanRepublishing(t *testing.T) {
	fixture := newViewFixture(t)
	fixture.views.findErr = errors.New("view store unavailable")

	result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.CustomerViewUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.UndecidedReason() != application.CustomerViewStoreUnavailable {
		t.Fatalf("reason = %q, want CUSTOMER_VIEW_STORE_UNAVAILABLE", result.UndecidedReason())
	}
	if fixture.policy.calls != 0 {
		t.Fatal("the round asked the disclosure policy before it could answer idempotently")
	}
}

// Covers: CONTEXT「每次普通追踪查询必须同时核对请求方身份、货主客户账户和目标对象
// 授权」的入口半边——账户或投影身份立不起来时未受理，不读任何依赖。
func TestACommandWithoutItsMinimumIdentityIsNotAccepted(t *testing.T) {
	fixture := newViewFixture(t)

	command := viewCommand(t, "projection-1")
	command.Customer = domain.CustomerAccountReference{}
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle without customer: %v", err)
	}
	if result.Outcome() != application.CustomerViewNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
	}

	zeroProjection, err := fixture.handler.Handle(context.Background(), application.DeriveCustomerViewCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Customer: viewCustomer(t),
	})
	if err != nil {
		t.Fatalf("handle without projection: %v", err)
	}
	if zeroProjection.Outcome() != application.CustomerViewNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", zeroProjection.Outcome())
	}
	if fixture.policy.calls != 0 {
		t.Fatal("an unaccepted command still reached the disclosure policy")
	}
	if fixture.views.saved != 0 {
		t.Fatal("an unaccepted command wrote the view store")
	}
}

// Covers: customer_view.go「展示时必带内容来处……两个方向的虚构（无中生有与有中说无）
// 都在构造期拦下」——策略答「展示」却不带内容是端口坏答复，上抛而不吞成业务结果。
func TestAShownAnswerWithoutContentIsRaisedAsAPortDefect(t *testing.T) {
	fixture := newViewFixture(t)
	fixture.policy.answer.Milestones = ports.DimensionDisclosure{State: domain.DimensionShown}

	_, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
	if err == nil {
		t.Fatal("a shown answer without content was swallowed instead of raised")
	}
	if fixture.identities.next != 0 {
		t.Fatal("a defective policy answer consumed a scarce view version identity")
	}
	if fixture.views.saved != 0 {
		t.Fatal("a defective policy answer wrote the view store")
	}
}
