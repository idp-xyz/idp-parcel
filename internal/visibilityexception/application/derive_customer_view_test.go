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
	views      map[viewStoreKey]domain.CustomerTrackingView
	versions   map[domain.CustomerViewVersionID]domain.CustomerTrackingView
	findErr    error
	saveErr    error
	saved      int
	reaffirmed int
}

func newViewStore() *viewStoreDouble {
	return &viewStoreDouble{
		views:    map[viewStoreKey]domain.CustomerTrackingView{},
		versions: map[domain.CustomerViewVersionID]domain.CustomerTrackingView{},
	}
}

func (double *viewStoreDouble) FindVersion(
	_ context.Context,
	_ domain.TenantID,
	_ domain.CustomerAccountReference,
	_ domain.TrackedParcelReference,
	version domain.CustomerViewVersionID,
) (domain.CustomerTrackingView, bool, error) {
	view, found := double.versions[version]
	return view, found, nil
}

func (double *viewStoreDouble) Reaffirm(
	_ context.Context,
	tenant domain.TenantID,
	view domain.CustomerTrackingView,
	replacing domain.ProjectionVersionID,
) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	key := viewStoreKey{tenant: tenant, customer: view.Customer(), parcel: view.Parcel()}
	current, found := double.views[key]
	if !found || current.Version() != view.Version() || current.BasedOn() != replacing {
		return errors.New("view store double: current view changed")
	}
	double.views[key] = view
	double.reaffirmed++
	return nil
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
	double.versions[view.Version()] = view
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

// viewEntry 造投影里的一条：给了里程碑即归类，不给即留作未归类——终局不是里程碑。
func viewEntry(
	t *testing.T,
	source domain.SourceContext,
	fact string,
	kind string,
	milestone string,
) domain.MilestoneClassification {
	t.Helper()
	accepted, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      source,
		Parcel:      mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Fact:        mustValue(t, domain.NewSourceFactReference, fact),
		Kind:        mustValue(t, domain.NewSourceFactKind, kind),
		Version:     mustValue(t, domain.NewSourceFactVersion, fact+"/v1"),
		OccurredAt:  factOccurredAt,
		EffectiveAt: factOccurredAt,
		ReceivedAt:  factOccurredAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("new accepted source fact: %v", err)
	}
	mapping := mustValue(t, domain.NewMappingVersionReference, "milestone-map/v1")
	if milestone == "" {
		entry, err := domain.LeaveUnclassified(accepted, mapping)
		if err != nil {
			t.Fatalf("leave unclassified: %v", err)
		}
		return entry
	}
	entry, err := domain.ClassifyMilestone(accepted, mustValue(t, domain.NewMilestoneReference, milestone), mapping)
	if err != nil {
		t.Fatalf("classify milestone: %v", err)
	}
	return entry
}

func intakeEntry(t *testing.T) domain.MilestoneClassification {
	t.Helper()
	return viewEntry(t, domain.SourceNodeOperations, "fact-1", "node-intake", "PICKED_UP")
}

// deliveryEntry 是 TF 的有效交付，归入交付里程碑：终局的上游来源，不是终局本身。
func deliveryEntry(t *testing.T) domain.MilestoneClassification {
	t.Helper()
	return viewEntry(t, domain.SourceTransportFulfillment,
		"effective-delivery/parcel-1/attempt-1", "effective-delivery", "DELIVERED")
}

func finalEntry(t *testing.T, kind string) domain.MilestoneClassification {
	t.Helper()
	return viewEntry(t, domain.SourceParcelShipment, "final-outcome/parcel-1/EFFECTIVE_DELIVERY", kind, "")
}

func projectionOf(t *testing.T, version string, entries ...domain.MilestoneClassification) domain.TrackingProjection {
	t.Helper()
	projection, err := domain.DeriveTrackingProjection(
		mustValue(t, domain.NewProjectionVersionID, version),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		entries,
		factOccurredAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("derive tracking projection: %v", err)
	}
	return projection
}

// trackedProjection 造一份终局已采用的投影版本：揽收一条加 PS 已采用终局一条。四维全准
// 时四维全展示，不看终局的各用例都从这里起。
func trackedProjection(t *testing.T, version string) domain.TrackingProjection {
	t.Helper()
	return projectionOf(t, version, intakeEntry(t), finalEntry(t, domain.FinalOutcomeAdoptedFactKind))
}

// inTransitProjection 造一份还没有终局的投影版本：只有揽收。
func inTransitProjection(t *testing.T, version string) domain.TrackingProjection {
	t.Helper()
	return projectionOf(t, version, intakeEntry(t))
}

func viewCommand(t *testing.T, projectionVersion string) application.DeriveCustomerViewCommand {
	t.Helper()
	return commandOver(t, trackedProjection(t, projectionVersion))
}

func commandOver(t *testing.T, projection domain.TrackingProjection) application.DeriveCustomerViewCommand {
	t.Helper()
	return application.DeriveCustomerViewCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Customer:   viewCustomer(t),
		Projection: projection,
	}
}

// Covers: CONTEXT 客户全程追踪视图生命周期「当前投影……首次满足展示条件 → 形成货主
// 客户账户隔离的客户视图版本」与「客户全程追踪视图只基于当前有效的全程追踪投影……
// 形成」——首版视图锚定投影版本、账户是字段、意图由视图版本认领恰发一份。点名
// `AT-VE-156`「当前投影包含已批准公开里程碑→展示标准语义……不复制内部来源状态」
// ——展示维只携带策略批准的内容引用。ETA 一维没有对客可见判断可依，策略准许也答
// 待确认（另见 ETA 一维的专门用例），其余三维照策略展示。
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
		"final":      dimensions.Final,
		"note":       dimensions.Note,
	} {
		if dimension.State() != domain.DimensionShown {
			t.Fatalf("%s dimension = %q, want SHOWN", name, dimension.State())
		}
	}
	if dimensions.ETA.State() != domain.DimensionPendingConfirmation {
		t.Fatalf("eta dimension = %q, want PENDING_CONFIRMATION without a customer-visible ETA judgement",
			dimensions.ETA.State())
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

// Covers: CONTEXT 生命周期「当前投影、客户可见 ETA、客户服务终局或已批准披露内容首次
// 满足展示条件 → 形成……客户视图版本」与「某一维信息待确认……只对该维返回待确认」——
// 策略准许展示终局，包裹在途、有效交付已进投影而终局未采用时，终局一维都答待确认、
// 不带内容，其余三维照策略展示；终局采用进投影后新版本视图指回前版、终局一维转为展示。
// 点名 `AT-VE-160` 的终局半边「只有有效交付扫描但终局尚未形成→可展示获准交付里程碑，
// 不显示包裹服务终局已完成」与 `AT-VE-159` 的展示半边「`parcel-shipment` 已形成当前
// 有效客户服务终局→展示终局及其依据版本，不由摘要反推」。视图每一维只带策略给的内容
// 来处，不列交付里程碑本身，也不带终局的依据版本，这两半没有钉。在途与有效交付两拍的
// 四维相同，第二拍不换代（视图版本不与投影版本一一对应）。
func TestAPermittedFinalPendsUntilTheProjectionCarriesTheAdoptedFinal(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := context.Background()

	var latest domain.CustomerTrackingView
	for _, stage := range []struct {
		name       string
		projection domain.TrackingProjection
		outcome    application.DeriveCustomerViewOutcome
	}{
		{"在途", inTransitProjection(t, "projection-1"), application.CustomerViewPublished},
		{"有效交付、终局未采用", projectionOf(t, "projection-2", intakeEntry(t), deliveryEntry(t)),
			application.CustomerViewExistingResult},
	} {
		result, err := fixture.handler.Handle(ctx, commandOver(t, stage.projection))
		if err != nil {
			t.Fatalf("%s: handle: %v", stage.name, err)
		}
		if result.Outcome() != stage.outcome {
			t.Fatalf("%s: outcome = %q, want %q", stage.name, result.Outcome(), stage.outcome)
		}
		latest, _ = result.View()
		dimensions := latest.Dimensions()
		if dimensions.Final.State() != domain.DimensionPendingConfirmation {
			t.Fatalf("%s: final = %q, want PENDING_CONFIRMATION", stage.name, dimensions.Final.State())
		}
		if _, has := dimensions.Final.Content(); has {
			t.Fatalf("%s: the final dimension carried content before any final was adopted", stage.name)
		}
		if dimensions.Milestones.State() != domain.DimensionShown ||
			dimensions.Note.State() != domain.DimensionShown {
			t.Fatalf("%s: pending the final pulled the other dimensions down with it", stage.name)
		}
	}

	adopted, err := fixture.handler.Handle(ctx, commandOver(t, projectionOf(t, "projection-3",
		intakeEntry(t), deliveryEntry(t), finalEntry(t, domain.FinalOutcomeAdoptedFactKind))))
	if err != nil {
		t.Fatalf("adopted handle: %v", err)
	}
	if adopted.Outcome() != application.CustomerViewPublished {
		t.Fatalf("outcome = %q, want PUBLISHED", adopted.Outcome())
	}
	after, _ := adopted.View()
	content, shown := after.Dimensions().Final.Content()
	if !shown || content.String() != "final/v1" {
		t.Fatalf("final = %q content %q, want SHOWN with the content the policy approved",
			after.Dimensions().Final.State(), content)
	}
	if prior, superseding := after.PriorVersion(); !superseding || prior != latest.Version() {
		t.Fatalf("prior version = %q, want the delivered-but-unadopted view %q kept as history",
			prior, latest.Version())
	}
}

// Covers: CONTEXT「客户全程追踪视图只基于当前有效的全程追踪投影、获准对客展示的 ETA……
// 形成」与 UC-VE-008 主流程「读取 `UC-VE-003` 的客户可见性判断；只有达到适用质量与合同
// 门槛的 ETA 才进入视图，不能使用计划时间或内部预测填充」——策略把 ETA 一维登成展示，
// 本编排没有任何获准对客的 ETA 判断可依，这一维答待确认、不带内容，不答「ETA：展示」；
// 策略不展示的照旧不展示。点名 `AT-VE-158`「只有内部 ETA 或质量未达对客门槛→不展示
// ETA，也不使用计划或承诺时间填充」。
func TestAPermittedETAPendsWithoutACustomerVisibleETAJudgement(t *testing.T) {
	for _, tc := range []struct {
		name string
		eta  ports.DimensionDisclosure
		want domain.DimensionState
	}{
		{"策略准许展示", shownAnswer(t).ETA, domain.DimensionPendingConfirmation},
		{"策略不展示", ports.DimensionDisclosure{State: domain.DimensionNotDisclosed}, domain.DimensionNotDisclosed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newViewFixture(t)
			fixture.policy.answer.ETA = tc.eta

			result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			view, _ := result.View()
			eta := view.Dimensions().ETA
			if eta.State() != tc.want {
				t.Fatalf("eta = %q, want %q", eta.State(), tc.want)
			}
			if _, has := eta.Content(); has {
				t.Fatal("the eta dimension carried content with no customer-visible ETA behind it")
			}
		})
	}
}

// Covers: CONTEXT 生命周期「某一维信息待确认……只对该维返回待确认……不虚构里程碑」——
// 策略准许展示里程碑，投影里却只有未归类条目（初始路由、终局这类不是标准追踪里程碑
// 的事实）时，里程碑一维答待确认；第一条已归类的里程碑进投影，四维变了，新版本指回
// 前版、里程碑一维转为展示。
func TestPermittedMilestonesPendUntilTheProjectionCarriesAClassifiedMilestone(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := context.Background()
	route := viewEntry(t, domain.SourceNetworkRouting, "initial-route/parcel-1", "initial-route-formed", "")

	first, err := fixture.handler.Handle(ctx, commandOver(t, projectionOf(t, "projection-1", route)))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	before, _ := first.View()
	if before.Dimensions().Milestones.State() != domain.DimensionPendingConfirmation {
		t.Fatalf("milestones = %q, want PENDING_CONFIRMATION with only unclassified entries",
			before.Dimensions().Milestones.State())
	}
	if _, has := before.Dimensions().Milestones.Content(); has {
		t.Fatal("the milestones dimension carried content before any milestone was classified")
	}

	second, err := fixture.handler.Handle(ctx, commandOver(t, projectionOf(t, "projection-2", route, intakeEntry(t))))
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}
	if second.Outcome() != application.CustomerViewPublished {
		t.Fatalf("outcome = %q, want PUBLISHED once a classified milestone changes the dimensions", second.Outcome())
	}
	after, _ := second.View()
	if content, shown := after.Dimensions().Milestones.Content(); !shown || content.String() != "milestones/v1" {
		t.Fatalf("milestones = %q content %q, want SHOWN with the content the policy approved",
			after.Dimensions().Milestones.State(), content)
	}
	if prior, superseding := after.PriorVersion(); !superseding || prior != before.Version() {
		t.Fatalf("prior version = %q, want %q", prior, before.Version())
	}
}

// Covers: CONTEXT「客户服务终局只能来自 `parcel-shipment`」——不采用判断同样来自 PS，说的
// 却是那份来源没有成为终局；投影里只有它时，策略准许也不展示终局。
func TestANotAdoptedFinalJudgmentDoesNotShowTheFinal(t *testing.T) {
	fixture := newViewFixture(t)

	result, err := fixture.handler.Handle(context.Background(), commandOver(t, projectionOf(t, "projection-1",
		intakeEntry(t), finalEntry(t, domain.FinalOutcomeNotAdoptedFactKind))))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	view, _ := result.View()
	if view.Dimensions().Final.State() != domain.DimensionPendingConfirmation {
		t.Fatalf("final = %q, want PENDING_CONFIRMATION; a refusal to adopt is not a final",
			view.Dimensions().Final.State())
	}
}

// Covers: CONTEXT「只展示适用服务与授权允许的……终局」——终局已经形成，策略不准看的仍不
// 展示，策略待确认的仍待确认；投影只能把展示压成待确认，不能把别的答复抬成展示。
func TestAFinalThePolicyDoesNotShowStaysUnshownAfterItForms(t *testing.T) {
	for _, tc := range []struct {
		name  string
		final ports.DimensionDisclosure
		want  domain.DimensionState
	}{
		{"不展示", ports.DimensionDisclosure{State: domain.DimensionNotDisclosed}, domain.DimensionNotDisclosed},
		{"待确认", ports.DimensionDisclosure{State: domain.DimensionPendingConfirmation}, domain.DimensionPendingConfirmation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newViewFixture(t)
			fixture.policy.answer.Final = tc.final

			result, err := fixture.handler.Handle(context.Background(), viewCommand(t, "projection-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			view, _ := result.View()
			if view.Dimensions().Final.State() != tc.want {
				t.Fatalf("final = %q, want %q", view.Dimensions().Final.State(), tc.want)
			}
		})
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
// 重新派生客户视图并形成追加更正或明确替代关系。原已发布版本保留」——新投影改变了
// 四维（终局采用进投影，终局一维由待确认转为展示）时，新视图指回前版并锚定新投影，
// 前一版按版本仍读得回、原样未改。点名 `AT-VE-161`「来源更正使原里程碑或终局失效→
// 形成追加更正/替代视图，原版本保留且不再作为当前结果」。
func TestANewerProjectionThatChangesTheDimensionsSupersedesTheCurrentViewKeepingHistory(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, commandOver(t, inTransitProjection(t, "projection-1")))
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
	kept, found, err := fixture.views.FindVersion(ctx, mustValue(t, domain.NewTenantID, "tenant-1"),
		viewCustomer(t), view.Parcel(), prior)
	if err != nil || !found {
		t.Fatalf("the superseded version is not readable: err=%v found=%v", err, found)
	}
	if kept.Dimensions() != firstView.Dimensions() || kept.BasedOn() != firstView.BasedOn() {
		t.Fatal("the superseded version was rewritten instead of kept as published")
	}
}

// Covers: CONTEXT「客户全程追踪视图的版本由披露更正与替代关系驱动，不与内部投影版本一一
// 对应；投影重新派生本身不要求形成新的客户视图版本」——新投影派生出的四维与当前版相同
// 时不换代：不签新身份、不写新版本，当前版只改记所依据的投影，答已有结果；意图由同一
// 视图版本认领，再交一次也是同一份。
func TestANewerProjectionThatLeavesTheDimensionsUnchangedKeepsTheViewVersion(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, viewCommand(t, "projection-1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstView, _ := first.View()
	issuedBefore := fixture.identities.next

	second, err := fixture.handler.Handle(ctx, commandOver(t, projectionOf(t, "projection-2",
		intakeEntry(t), deliveryEntry(t), finalEntry(t, domain.FinalOutcomeAdoptedFactKind))))
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}

	if second.Outcome() != application.CustomerViewExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT; unchanged dimensions must not form a new view version",
			second.Outcome())
	}
	view, present := second.View()
	if !present || view.Version() != firstView.Version() {
		t.Fatalf("view version = %q, want the current version %q kept", view.Version(), firstView.Version())
	}
	if view.BasedOn().String() != "projection-2" {
		t.Fatalf("view based on %q, want it to follow the newer projection", view.BasedOn())
	}
	if _, superseding := view.PriorVersion(); superseding {
		t.Fatal("an unchanged view grew a prior version")
	}
	if fixture.identities.next != issuedBefore {
		t.Fatal("an unchanged view consumed a new view version identity")
	}
	if fixture.views.saved != 1 || fixture.views.reaffirmed != 1 {
		t.Fatalf("saved %d, reaffirmed %d; want the first version saved once and then reaffirmed once",
			fixture.views.saved, fixture.views.reaffirmed)
	}
	if len(fixture.downstream.intents) != 2 ||
		fixture.downstream.intents[1].View.Version() != firstView.Version() {
		t.Fatal("the reaffirmed view must hand off the same intent claimed by its unchanged version")
	}

	replay, err := fixture.handler.Handle(ctx, commandOver(t, projectionOf(t, "projection-2",
		intakeEntry(t), deliveryEntry(t), finalEntry(t, domain.FinalOutcomeAdoptedFactKind))))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.CustomerViewExistingResult || fixture.views.reaffirmed != 1 {
		t.Fatal("replaying the projection the view already follows reaffirmed it again")
	}
}

// Covers: 同一条未决语义的重新确认半边——改记所依据的投影没写成（并发输家或库故障）时
// 答未决，交回重投，不答已有结果：答了，消费门就入账收工，视图永远停在旧投影上。
func TestAFailedReaffirmIsUndecided(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, viewCommand(t, "projection-1")); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	fixture.views.saveErr = errors.New("view store unavailable")

	result, err := fixture.handler.Handle(ctx, viewCommand(t, "projection-2"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.CustomerViewUndecided ||
		result.UndecidedReason() != application.CustomerViewStoreUnavailable {
		t.Fatalf("outcome = %q reason = %q, want UNDECIDED / CUSTOMER_VIEW_STORE_UNAVAILABLE",
			result.Outcome(), result.UndecidedReason())
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
