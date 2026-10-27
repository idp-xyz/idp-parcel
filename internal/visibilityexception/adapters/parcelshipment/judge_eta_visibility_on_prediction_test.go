package parcelshipment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件钉 ETA 预测新版本 → 客户可见性判断 → 客户视图重派生这一条消费链的适配器行为：账户三格、判断编排各结果格的
// 去向、有无投影的分叉、判断换版时视图随之换代（投影版本不变）。判断与视图库用内存替身，库的行为以
// adapters/postgres 的真库用例为准。

// emptyJudgementRead 是「还没有任何判断」的读口——派生编排不关心 ETA 一维的各用例用它。
type emptyJudgementRead struct{}

func (emptyJudgementRead) FindCurrent(
	context.Context, vedomain.TenantID, vedomain.CustomerAccountReference, vedomain.TrackedParcelReference,
) (vedomain.ETAVisibilityJudgement, bool, error) {
	return vedomain.ETAVisibilityJudgement{}, false, nil
}

// sharedJudgements 同时是判断编排的写口与派生编排的读口，让链上两环看见同一份判断。
type sharedJudgements struct {
	current  vedomain.ETAVisibilityJudgement
	has      bool
	versions int
}

func (double *sharedJudgements) FindCurrent(
	context.Context, vedomain.TenantID, vedomain.CustomerAccountReference, vedomain.TrackedParcelReference,
) (vedomain.ETAVisibilityJudgement, bool, error) {
	return double.current, double.has, nil
}

func (double *sharedJudgements) FindVersion(
	context.Context, vedomain.TenantID, vedomain.CustomerAccountReference, vedomain.TrackedParcelReference,
	vedomain.ETAVisibilityJudgementVersionID,
) (vedomain.ETAVisibilityJudgement, bool, error) {
	return vedomain.ETAVisibilityJudgement{}, false, nil
}

func (double *sharedJudgements) Save(_ context.Context, _ vedomain.TenantID, judgement vedomain.ETAVisibilityJudgement) error {
	double.current, double.has = judgement, true
	double.versions++
	return nil
}

type etaReadDouble struct {
	eta   vedomain.ETAPrediction
	found bool
	err   error
}

func (double *etaReadDouble) FindCurrent(
	_ context.Context, _ vedomain.TenantID, _ vedomain.TrackedParcelReference, _ vedomain.MilestoneReference,
	source vedomain.ETASourceKind,
) (vedomain.ETAPrediction, bool, error) {
	if double.err != nil || !double.found || double.eta.Source() != source {
		return vedomain.ETAPrediction{}, false, double.err
	}
	return double.eta, true, nil
}

func (double *etaReadDouble) ListCurrent(
	context.Context, vedomain.TenantID, vedomain.TrackedParcelReference, vedomain.MilestoneReference,
) ([]vedomain.ETAPrediction, error) {
	if double.err != nil || !double.found {
		return nil, double.err
	}
	return []vedomain.ETAPrediction{double.eta}, nil
}

type etaRuleDouble struct {
	basis vedomain.ETAVisibilityRuleBasis
	err   error
}

func (double *etaRuleDouble) AssessETAVisibilityRule(
	context.Context, vedomain.TenantID, vedomain.CustomerAccountReference, time.Time,
) (vedomain.ETAVisibilityRuleBasis, error) {
	return double.basis, double.err
}

type judgementVersionDouble struct{ next int }

func (double *judgementVersionDouble) NextETAVisibilityJudgementVersionID(
	context.Context,
) (vedomain.ETAVisibilityJudgementVersionID, error) {
	double.next++
	return vedomain.NewETAVisibilityJudgementVersionID(fmt.Sprintf("evj-%d", double.next))
}

// showEverythingPolicy 把四维都准许展示，让 ETA 一维的答复只取决于判断。
type showEverythingPolicy struct{}

func (showEverythingPolicy) AssessDisclosure(
	context.Context, vedomain.CustomerAccountReference, vedomain.TrackingProjection,
) (veports.DisclosureAnswer, bool, error) {
	shown := func(raw string) veports.DimensionDisclosure {
		content, _ := vedomain.NewViewContentReference(raw)
		return veports.DimensionDisclosure{State: vedomain.DimensionShown, Content: content}
	}
	return veports.DisclosureAnswer{
		Milestones: shown("milestones/v1"), ETA: shown("eta/v1"), Final: shown("final/v1"), Note: shown("note/v1"),
	}, true, nil
}

func predictionETA(t *testing.T, version, confidence string) vedomain.ETAPrediction {
	t.Helper()
	eta, err := vedomain.FormETAPrediction(vedomain.ETAPredictionSpec{
		Version:     finalValue(t, vedomain.NewETAVersionID, version),
		Parcel:      finalValue(t, vedomain.NewTrackedParcelReference, "parcel-1"),
		Milestone:   finalValue(t, vedomain.NewMilestoneReference, "DELIVERED"),
		Source:      vedomain.OperatorDerivedETA,
		Inputs:      finalValue(t, vedomain.NewPredictionInputsReference, "inputs/"+version),
		Model:       finalValue(t, vedomain.NewPredictionModelReference, "eta-model/v4"),
		RangeFrom:   viewDerivedAt.Add(30 * time.Hour),
		RangeTo:     viewDerivedAt.Add(42 * time.Hour),
		Confidence:  finalValue(t, vedomain.NewConfidenceReference, confidence),
		PredictedAt: viewDerivedAt,
	})
	if err != nil {
		t.Fatalf("形成 ETA：%v", err)
	}
	return eta
}

func predictionRule(t *testing.T, qualifying, milestone string) vedomain.ETAVisibilityRuleBasis {
	t.Helper()
	rule, err := vedomain.NewETAVisibilityRule(vedomain.ETAVisibilityRuleSpec{
		GateVersion:     finalValue(t, vedomain.NewETAQualityGateVersion, "gate/v1"),
		ServiceRule:     finalValue(t, vedomain.NewCustomerServiceRuleVersion, "contract-c1/v3"),
		TargetMilestone: finalValue(t, vedomain.NewMilestoneReference, milestone),
		Qualifying:      []vedomain.ConfidenceReference{finalValue(t, vedomain.NewConfidenceReference, qualifying)},
		Expression:      vedomain.ETAShownAsTimeRange,
		StopBoundary:    finalValue(t, vedomain.NewETAStopShowingBoundary, "stop/after-delivered"),
	})
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	return vedomain.ConfiguredETARule(rule)
}

type predictionChain struct {
	subject     *adapter.JudgeETAVisibilityOnPredictionAdapter
	projections *currentProjectionDouble
	accounts    *accountViewDouble
	etas        *etaReadDouble
	rules       *etaRuleDouble
	judgements  *sharedJudgements
	views       *customerViewStoreDouble
}

func newPredictionChain(t *testing.T) *predictionChain {
	t.Helper()
	chain := &predictionChain{
		projections: &currentProjectionDouble{projection: builtProjection(t, "projection-1", "parcel-1"), found: true},
		accounts:    resolvedAccount(t),
		etas:        &etaReadDouble{eta: predictionETA(t, "eta-1", "MEDIUM/history-based"), found: true},
		rules:       &etaRuleDouble{basis: predictionRule(t, "MEDIUM/history-based", "DELIVERED")},
		judgements:  &sharedJudgements{},
		views:       &customerViewStoreDouble{byKey: map[string]vedomain.CustomerTrackingView{}},
	}
	judge := veapplication.NewJudgeETAVisibilityHandler(veapplication.JudgeETAVisibilityDeps{
		Predictions: chain.etas,
		Rules:       chain.rules,
		Judgements:  chain.judgements,
		Identities:  &judgementVersionDouble{},
		Clock:       viewClock{at: viewDerivedAt.Add(time.Minute)},
	})
	derive := veapplication.NewDeriveCustomerViewHandler(veapplication.DeriveCustomerViewDeps{
		Policy:        showEverythingPolicy{},
		ETAJudgements: chain.judgements,
		Views:         chain.views,
		Identities:    &viewIdentityDouble{},
		Downstream:    &viewDownstreamDouble{},
		Clock:         viewClock{at: viewDerivedAt.Add(2 * time.Minute)},
	})
	subject, err := adapter.NewJudgeETAVisibilityOnPredictionAdapter(chain.projections, chain.accounts, judge, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	chain.subject = subject
	return chain
}

func formedETA(version string) veinbox.FormedETAPrediction {
	return veinbox.FormedETAPrediction{
		TenantID: "tenant-1", Parcel: "parcel-1", Milestone: "DELIVERED", VersionID: version,
	}
}

func (chain *predictionChain) currentView(t *testing.T) vedomain.CustomerTrackingView {
	t.Helper()
	view, found := chain.views.byKey["tenant-1/customer-1/parcel-1"]
	if !found {
		t.Fatal("链走完之后没有客户视图")
	}
	return view
}

// Covers: AT-VE-187、AT-VE-215 + CONTEXT「ETA 新版本……必须重新派生客户视图并形成追加更正或明确替代关系」——新 ETA 版本达标：
// 形成客户可见 ETA 判断，同一事务里重派生客户视图，ETA 一维展示、内容来处是这一版判断；随后 ETA 新版本不再达标
// （AT-VE-158）：判断换版为 ETA 不可见，视图在投影版本不变的情况下换代、ETA 一维不展示，前一版指回。
func TestAFormedETAJudgesAndRederivesTheCustomerViewAcrossVersions(t *testing.T) {
	chain := newPredictionChain(t)

	if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); err != nil {
		t.Fatalf("首个 ETA：%v", err)
	}
	first := chain.currentView(t)
	content, shown := first.Dimensions().ETA.Content()
	if !shown || content.String() != "evj-1" {
		t.Fatalf("ETA 一维 = %q 内容 %q shown = %v，want 展示且来处为判断 evj-1", first.Dimensions().ETA.State(), content, shown)
	}

	chain.etas.eta = predictionETA(t, "eta-2", "LOW/sparse")
	if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-2")); err != nil {
		t.Fatalf("新 ETA 版本：%v", err)
	}
	second := chain.currentView(t)
	if second.Dimensions().ETA.State() != vedomain.DimensionNotDisclosed {
		t.Fatalf("ETA 一维 = %q, want NOT_DISCLOSED", second.Dimensions().ETA.State())
	}
	if prior, superseding := second.PriorVersion(); !superseding || prior != first.Version() {
		t.Fatalf("prior = %q, want %q", prior, first.Version())
	}
	if second.BasedOn() != first.BasedOn() {
		t.Fatal("判断变化触发的重派生不该换投影锚")
	}
}

// Covers: AT-VE-215、AT-VE-213——信封重投（同一预测版本再来一封）：判断按同一候选与同一依据重放，视图四维没变，不再换代。
func TestAReplayedETAEnvelopeDoesNotChurnTheJudgementOrTheView(t *testing.T) {
	chain := newPredictionChain(t)
	if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); err != nil {
		t.Fatalf("首投：%v", err)
	}
	firstView := chain.currentView(t)

	if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if chain.judgements.versions != 1 {
		t.Fatalf("判断版本 = %d, want 1", chain.judgements.versions)
	}
	if chain.currentView(t).Version() != firstView.Version() {
		t.Fatal("重投换了视图版本")
	}
}

// Covers: 投影还不存在——判断照形成（留库），不派生视图，也不欠任何动作：视图在第一份投影派生时读判断。
func TestWithoutACurrentProjectionTheJudgementIsFormedAndNoViewIsDerived(t *testing.T) {
	chain := newPredictionChain(t)
	chain.projections.found = false

	if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); err != nil {
		t.Fatalf("处理：%v", err)
	}
	if chain.judgements.versions != 1 || len(chain.views.byKey) != 0 {
		t.Fatalf("判断版本 = %d 视图数 = %d, want 1 与 0", chain.judgements.versions, len(chain.views.byKey))
	}
}

// Covers: 账户反查三格之「零行」——没有归属的包裹没有可见性判断的对象：不判断、不发明账户，入账收工。
func TestAParcelWithoutAnAccountIsNotJudged(t *testing.T) {
	chain := newPredictionChain(t)
	chain.accounts.found = false

	if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); err != nil {
		t.Fatalf("零行应入账跳过：%v", err)
	}
	if chain.judgements.versions != 0 {
		t.Fatal("零行不该形成判断——账户不得发明")
	}
}

// Covers: 账户反查三格之「多行」与反查口调不通——歧义哨兵原样上抛（AT-VE-152 不任选），调不通是续办；都不判断。
func TestAmbiguousOrUnavailableAccountLookupStaysUndecidedWithoutJudging(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"多账户歧义": {fmt.Errorf("%w: parcel", adapter.ErrAmbiguousCustomerAccount), adapter.ErrAmbiguousCustomerAccount},
		"反查调不通": {errors.New("ps store unavailable"), adapter.ErrCustomerAccountUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			chain := newPredictionChain(t)
			chain.accounts.err = tc.err
			if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if chain.judgements.versions != 0 {
				t.Fatal("账户没定下来就形成了判断")
			}
		})
	}
}

// Covers: AT-VE-214 与判断编排各结果格的去向（ADR-0025，不落进 default）——未决是续办哨兵；候选 ETA 消失是仓储不变量已破；
// 规则指名别的里程碑是业务终局，不判不派生。
func TestEachJudgementOutcomeHasItsOwnLanding(t *testing.T) {
	t.Run("依赖读不动是续办", func(t *testing.T) {
		chain := newPredictionChain(t)
		chain.rules.err = errors.New("rule register down")
		if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); !errors.Is(err, adapter.ErrETAVisibilityUndecided) {
			t.Fatalf("err = %v, want ErrETAVisibilityUndecided", err)
		}
	})
	t.Run("候选 ETA 消失", func(t *testing.T) {
		chain := newPredictionChain(t)
		chain.etas.found = false
		if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); !errors.Is(err, adapter.ErrETAPredictionInconsistent) {
			t.Fatalf("err = %v, want ErrETAPredictionInconsistent", err)
		}
	})
	t.Run("规则指名别的里程碑", func(t *testing.T) {
		chain := newPredictionChain(t)
		chain.rules.basis = predictionRule(t, "MEDIUM/history-based", "OUT_FOR_DELIVERY")
		if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); err != nil {
			t.Fatalf("规则目标之外应入账收工：%v", err)
		}
		if chain.judgements.versions != 0 || len(chain.views.byKey) != 0 {
			t.Fatal("规则目标之外的 ETA 形成了判断或视图")
		}
	})
}

func TestAnUntranslatableETAReferenceKeepsItsSentinelAndReadsNothing(t *testing.T) {
	chain := newPredictionChain(t)
	chain.accounts.err = errors.New("must not be read")
	for name, formed := range map[string]veinbox.FormedETAPrediction{
		"空租户":  {Parcel: "parcel-1", Milestone: "DELIVERED", VersionID: "eta-1"},
		"空包裹":  {TenantID: "tenant-1", Milestone: "DELIVERED", VersionID: "eta-1"},
		"空里程碑": {TenantID: "tenant-1", Parcel: "parcel-1", VersionID: "eta-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := chain.subject.HandleFormedETAPrediction(t.Context(), formed); !errors.Is(err, adapter.ErrETAPredictionUntranslatable) {
				t.Fatalf("err = %v, want ErrETAPredictionUntranslatable", err)
			}
		})
	}
}

func TestAnUnreadableProjectionStoreIsContinuableUndecidedBeforeAnyJudging(t *testing.T) {
	chain := newPredictionChain(t)
	chain.projections.err = errors.New("store unavailable")
	if err := chain.subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); !errors.Is(err, adapter.ErrDerivedProjectionUnreadable) {
		t.Fatalf("err = %v, want ErrDerivedProjectionUnreadable", err)
	}
	if chain.judgements.versions != 0 {
		t.Fatal("投影库调不通就形成了判断")
	}
}

func TestTheJudgementAdapterRefusesMissingDependencies(t *testing.T) {
	chain := newPredictionChain(t)
	judge := veapplication.NewJudgeETAVisibilityHandler(veapplication.JudgeETAVisibilityDeps{})
	derive := &viewDeriveCounting{}
	for name, build := range map[string]func() error{
		"投影读口": func() error {
			_, err := adapter.NewJudgeETAVisibilityOnPredictionAdapter(nil, chain.accounts, judge, derive)
			return err
		},
		"账户读口": func() error {
			_, err := adapter.NewJudgeETAVisibilityOnPredictionAdapter(chain.projections, nil, judge, derive)
			return err
		},
		"判断编排": func() error {
			_, err := adapter.NewJudgeETAVisibilityOnPredictionAdapter(chain.projections, chain.accounts, nil, derive)
			return err
		},
		"视图派生": func() error {
			_, err := adapter.NewJudgeETAVisibilityOnPredictionAdapter(chain.projections, chain.accounts, judge, nil)
			return err
		},
	} {
		if build() == nil {
			t.Errorf("缺 %s 却构造成功", name)
		}
	}
}

// orderedProjection 与 orderedJudgements 把「锁投影行」与「写判断」记进同一份事件序，钉锁序：投影行在前，
// 判断随后，客户视图最后。
type orderedProjection struct {
	*currentProjectionDouble
	events *[]string
}

func (double orderedProjection) LockCurrent(
	ctx context.Context, tenant vedomain.TenantID, parcel vedomain.TrackedParcelReference,
) (vedomain.TrackingProjection, bool, error) {
	*double.events = append(*double.events, "lock-projection")
	return double.currentProjectionDouble.LockCurrent(ctx, tenant, parcel)
}

type orderedJudgements struct {
	*sharedJudgements
	events *[]string
}

func (double orderedJudgements) Save(
	ctx context.Context, tenant vedomain.TenantID, judgement vedomain.ETAVisibilityJudgement,
) error {
	*double.events = append(*double.events, "save-judgement")
	return double.sharedJudgements.Save(ctx, tenant, judgement)
}

type orderedViews struct {
	*customerViewStoreDouble
	events *[]string
}

func (double orderedViews) Save(
	ctx context.Context, tenant vedomain.TenantID, view vedomain.CustomerTrackingView,
) error {
	*double.events = append(*double.events, "save-view")
	return double.customerViewStoreDouble.Save(ctx, tenant, view)
}

// Covers: 端口头注的锁序——投影行（LockCurrent）→ 判断 → 客户视图，与投影派生、客户归属补派生两路取锁方向一致。
// 判断与视图在同一事务里先后写，锁若排在写之后，旧判断的信封就可能按已被后继投影替换的投影写回视图。
func TestTheProjectionRowIsLockedBeforeTheJudgementAndTheViewAreWritten(t *testing.T) {
	var events []string
	judgements := &sharedJudgements{}
	views := &customerViewStoreDouble{byKey: map[string]vedomain.CustomerTrackingView{}}
	judge := veapplication.NewJudgeETAVisibilityHandler(veapplication.JudgeETAVisibilityDeps{
		Predictions: &etaReadDouble{eta: predictionETA(t, "eta-1", "MEDIUM/history-based"), found: true},
		Rules:       &etaRuleDouble{basis: predictionRule(t, "MEDIUM/history-based", "DELIVERED")},
		Judgements:  orderedJudgements{sharedJudgements: judgements, events: &events},
		Identities:  &judgementVersionDouble{},
		Clock:       viewClock{at: viewDerivedAt.Add(time.Minute)},
	})
	derive := veapplication.NewDeriveCustomerViewHandler(veapplication.DeriveCustomerViewDeps{
		Policy:        showEverythingPolicy{},
		ETAJudgements: judgements,
		Views:         orderedViews{customerViewStoreDouble: views, events: &events},
		Identities:    &viewIdentityDouble{},
		Downstream:    &viewDownstreamDouble{},
		Clock:         viewClock{at: viewDerivedAt.Add(2 * time.Minute)},
	})
	subject, err := adapter.NewJudgeETAVisibilityOnPredictionAdapter(
		orderedProjection{
			currentProjectionDouble: &currentProjectionDouble{projection: builtProjection(t, "projection-1", "parcel-1"), found: true},
			events:                  &events,
		},
		resolvedAccount(t), judge, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleFormedETAPrediction(t.Context(), formedETA("eta-1")); err != nil {
		t.Fatalf("处理：%v", err)
	}
	want := []string{"lock-projection", "save-judgement", "save-view"}
	if len(events) != len(want) {
		t.Fatalf("事件序 = %v, want %v", events, want)
	}
	for index := range want {
		if events[index] != want[index] {
			t.Fatalf("事件序 = %v, want %v", events, want)
		}
	}
}
