package parcelshipment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var viewDerivedAt = time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC)

// builtProjection 经领域真路径造一份投影：事实 → 未归类条目 → 派生。夹具不手搓投影
// ——构造门（条目非空、包裹一致）就是领域不变量。
func builtProjection(t *testing.T, version, parcel string) vedomain.TrackingProjection {
	t.Helper()
	fact, err := vedomain.NewAcceptedSourceFact(vedomain.AcceptedSourceFactSpec{
		Source:      vedomain.SourceTransportFulfillment,
		Parcel:      finalValue(t, vedomain.NewTrackedParcelReference, parcel),
		Fact:        finalValue(t, vedomain.NewSourceFactReference, "transport-handover/"+parcel+"/scope-1"),
		Kind:        finalValue(t, vedomain.NewSourceFactKind, "handover-handed-over"),
		Version:     finalValue(t, vedomain.NewSourceFactVersion, "v1"),
		OccurredAt:  viewDerivedAt.Add(-2 * time.Hour),
		EffectiveAt: viewDerivedAt.Add(-2 * time.Hour),
		ReceivedAt:  viewDerivedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("构造事实：%v", err)
	}
	entry, err := vedomain.LeaveUnclassified(fact,
		finalValue(t, vedomain.NewMappingVersionReference, "MAPPING_NOT_CONFIGURED"))
	if err != nil {
		t.Fatalf("未归类条目：%v", err)
	}
	projection, err := vedomain.DeriveTrackingProjection(
		finalValue(t, vedomain.NewProjectionVersionID, version),
		finalValue(t, vedomain.NewTrackedParcelReference, parcel),
		[]vedomain.MilestoneClassification{entry},
		viewDerivedAt,
	)
	if err != nil {
		t.Fatalf("派生投影：%v", err)
	}
	return projection
}

type currentProjectionDouble struct {
	projection vedomain.TrackingProjection
	found      bool
	err        error
}

func (double *currentProjectionDouble) FindCurrent(
	_ context.Context, _ vedomain.TenantID, _ vedomain.TrackedParcelReference,
) (vedomain.TrackingProjection, bool, error) {
	if double.err != nil {
		return vedomain.TrackingProjection{}, false, double.err
	}
	return double.projection, double.found, nil
}

type accountViewDouble struct {
	account vedomain.CustomerAccountReference
	found   bool
	err     error
	calls   int
}

func (double *accountViewDouble) FindCustomerAccount(
	_ context.Context, _ vedomain.TenantID, _ vedomain.TrackedParcelReference,
) (vedomain.CustomerAccountReference, bool, error) {
	double.calls++
	if double.err != nil {
		return vedomain.CustomerAccountReference{}, false, double.err
	}
	return double.account, double.found, nil
}

// viewDeriveCounting 只数调用并记住命令——钉「哪些格根本不该走到编排」。零值结果落
// 在封闭集合外，顺带钉 ViewConsumption 的不留兜底。
type viewDeriveCounting struct {
	calls int
	last  veapplication.DeriveCustomerViewCommand
}

func (double *viewDeriveCounting) Handle(
	_ context.Context, command veapplication.DeriveCustomerViewCommand,
) (veapplication.DeriveCustomerViewResult, error) {
	double.calls++
	double.last = command
	return veapplication.DeriveCustomerViewResult{}, nil
}

type viewPolicyDouble struct {
	configured bool
	err        error
}

func (double *viewPolicyDouble) AssessDisclosure(
	_ context.Context, _ vedomain.CustomerAccountReference, _ vedomain.TrackingProjection,
) (veports.DisclosureAnswer, bool, error) {
	if double.err != nil {
		return veports.DisclosureAnswer{}, false, double.err
	}
	return veports.DisclosureAnswer{}, double.configured, nil
}

type customerViewStoreDouble struct {
	byKey   map[string]vedomain.CustomerTrackingView
	findErr error
}

func (double *customerViewStoreDouble) FindCurrent(
	_ context.Context,
	tenant vedomain.TenantID,
	customer vedomain.CustomerAccountReference,
	parcel vedomain.TrackedParcelReference,
) (vedomain.CustomerTrackingView, bool, error) {
	if double.findErr != nil {
		return vedomain.CustomerTrackingView{}, false, double.findErr
	}
	view, found := double.byKey[tenant.String()+"/"+customer.String()+"/"+parcel.String()]
	return view, found, nil
}

func (double *customerViewStoreDouble) Save(
	_ context.Context, tenant vedomain.TenantID, view vedomain.CustomerTrackingView,
) error {
	double.byKey[tenant.String()+"/"+view.Customer().String()+"/"+view.Parcel().String()] = view
	return nil
}

type viewIdentityDouble struct{ next int }

func (double *viewIdentityDouble) NextCustomerViewVersionID(_ context.Context) (vedomain.CustomerViewVersionID, error) {
	double.next++
	return vedomain.NewCustomerViewVersionID(fmt.Sprintf("view-%d", double.next))
}

type viewDownstreamDouble struct{ err error }

func (double *viewDownstreamDouble) HandOffCustomerView(
	context.Context, veports.CustomerViewHandoffIntent,
) error {
	return double.err
}

type viewClock struct{ at time.Time }

func (clock viewClock) Now() time.Time { return clock.at }

// realViewDeriveHandler 用真实编排配替身依赖——结果类型的字段不导出，凡要特定结果
// 格的用例都走真路径拿。
func realViewDeriveHandler(
	views *customerViewStoreDouble, policy viewPolicyDouble, downstream viewDownstreamDouble,
) *veapplication.DeriveCustomerViewHandler {
	return veapplication.NewDeriveCustomerViewHandler(veapplication.DeriveCustomerViewDeps{
		Policy:     &policy,
		Views:      views,
		Identities: &viewIdentityDouble{},
		Downstream: &downstream,
		Clock:      viewClock{at: viewDerivedAt.Add(time.Minute)},
	})
}

func derivedRef(version string) veinbox.DerivedTrackingProjection {
	return veinbox.DerivedTrackingProjection{
		TenantID:  "tenant-1",
		Parcel:    "parcel-1",
		VersionID: version,
	}
}

func viewSubject(
	t *testing.T,
	projections *currentProjectionDouble,
	accounts *accountViewDouble,
	derive adapter.CustomerViewDeriveHandler,
) *adapter.DeriveCustomerViewOnProjectionAdapter {
	t.Helper()
	subject, err := adapter.NewDeriveCustomerViewOnProjectionAdapter(projections, accounts, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return subject
}

func resolvedAccount(t *testing.T) *accountViewDouble {
	t.Helper()
	return &accountViewDouble{
		account: finalValue(t, vedomain.NewCustomerAccountReference, "customer-1"),
		found:   true,
	}
}

func TestAnUntranslatableProjectionReferenceKeepsItsSentinel(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := viewSubject(t,
		&currentProjectionDouble{projection: builtProjection(t, "projection-1", "parcel-1"), found: true},
		resolvedAccount(t), derive)
	for name, reference := range map[string]veinbox.DerivedTrackingProjection{
		"空租户": {Parcel: "parcel-1", VersionID: "projection-1"},
		"空包裹": {TenantID: "tenant-1", VersionID: "projection-1"},
		"空版本": {TenantID: "tenant-1", Parcel: "parcel-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleDerivedTrackingProjection(
				t.Context(), reference); !errors.Is(err, adapter.ErrDerivedProjectionUntranslatable) {
				t.Fatalf("err = %v, want ErrDerivedProjectionUntranslatable", err)
			}
		})
	}
	if derive.calls != 0 {
		t.Fatal("引用译不出来不该走到派生")
	}
}

func TestAnUnreadableProjectionStoreIsContinuableUndecided(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := viewSubject(t,
		&currentProjectionDouble{err: errors.New("store unavailable")},
		resolvedAccount(t), derive)
	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); !errors.Is(err, adapter.ErrDerivedProjectionUnreadable) {
		t.Fatalf("err = %v, want ErrDerivedProjectionUnreadable", err)
	}
	if derive.calls != 0 {
		t.Fatal("投影库调不通不该走到派生")
	}
}

// Covers: 派生信封与投影行同一事务入库——信封在而行不在是仓储不变量已破，响亮报错
// 而不是当可见性滞后等下去。
func TestAMissingCurrentProjectionIsInconsistentNotUndecided(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := viewSubject(t, &currentProjectionDouble{}, resolvedAccount(t), derive)
	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); !errors.Is(err, adapter.ErrDerivedProjectionInconsistent) {
		t.Fatalf("err = %v, want ErrDerivedProjectionInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("读不到当前投影不该走到派生")
	}
}

// Covers: 版本判新旧——旧版本信封业务终局跳过，不反查账户也不派生，视图由后继信封
// 派生；为旧版派生会让当前视图倒退到已被替代的投影内容。
func TestAStaleProjectionEnvelopeIsSkippedAsBusinessFinal(t *testing.T) {
	derive := &viewDeriveCounting{}
	accounts := resolvedAccount(t)
	subject := viewSubject(t,
		&currentProjectionDouble{projection: builtProjection(t, "projection-2", "parcel-1"), found: true},
		accounts, derive)
	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); err != nil {
		t.Fatalf("旧版本信封应入账跳过：%v", err)
	}
	if accounts.calls != 0 {
		t.Fatal("旧版本不该反查账户")
	}
	if derive.calls != 0 {
		t.Fatal("旧版本不该走到派生")
	}
}

// Covers: 账户反查三格之「零行」——不派生视图、不发明账户，入账收工；零行的两种成因
// 今天在数据上分不开，本适配器不发明区分（勘察报告裁定，对外亦不区分）。
func TestAParcelWithoutAnAccountIsSkippedWithoutInventingOne(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := viewSubject(t,
		&currentProjectionDouble{projection: builtProjection(t, "projection-1", "parcel-1"), found: true},
		&accountViewDouble{}, derive)
	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); err != nil {
		t.Fatalf("零行应入账跳过：%v", err)
	}
	if derive.calls != 0 {
		t.Fatal("零行不该走到派生——账户不得发明")
	}
}

// Covers: 账户反查三格之「多行」——机制拒绝自动采认（AT-VE-152、ADR-0060），具名
// 哨兵原样上抛落未决，不任选。
func TestAnAmbiguousAccountStaysUndecidedWithItsSentinel(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := viewSubject(t,
		&currentProjectionDouble{projection: builtProjection(t, "projection-1", "parcel-1"), found: true},
		&accountViewDouble{err: fmt.Errorf("%w: parcel %q",
			adapter.ErrAmbiguousCustomerAccount, "parcel-1")}, derive)
	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); !errors.Is(err, adapter.ErrAmbiguousCustomerAccount) {
		t.Fatalf("err = %v, want ErrAmbiguousCustomerAccount", err)
	}
	if derive.calls != 0 {
		t.Fatal("歧义不该走到派生——不得任选账户")
	}
}

func TestAnUnavailableAccountLookupIsContinuableUndecided(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := viewSubject(t,
		&currentProjectionDouble{projection: builtProjection(t, "projection-1", "parcel-1"), found: true},
		&accountViewDouble{err: errors.New("ps store unavailable")}, derive)
	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); !errors.Is(err, adapter.ErrCustomerAccountUnavailable) {
		t.Fatalf("err = %v, want ErrCustomerAccountUnavailable", err)
	}
	if derive.calls != 0 {
		t.Fatal("反查读口调不通不该走到派生")
	}
}

// Covers: 恰一行——账户与当前投影原样进命令，视图发布成功入账（披露未配置四维全待
// 确认由编排负责，本层不兜底）。
func TestAResolvedAccountDerivesAndPublishesTheCustomerView(t *testing.T) {
	projection := builtProjection(t, "projection-1", "parcel-1")
	views := &customerViewStoreDouble{byKey: map[string]vedomain.CustomerTrackingView{}}
	handler := realViewDeriveHandler(views, viewPolicyDouble{configured: false}, viewDownstreamDouble{})
	subject := viewSubject(t,
		&currentProjectionDouble{projection: projection, found: true},
		resolvedAccount(t), handler)

	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); err != nil {
		t.Fatalf("发布成功应入账：%v", err)
	}
	view, found := views.byKey["tenant-1/customer-1/parcel-1"]
	if !found {
		t.Fatal("客户视图没落库")
	}
	if view.BasedOn().String() != "projection-1" {
		t.Fatalf("视图采用版本 = %s, want projection-1", view.BasedOn())
	}
}

// Covers: 同一投影版本重投——已有结果入账，不形成第二个视图版本。
func TestARedeliveredProjectionAnswersTheExistingView(t *testing.T) {
	projection := builtProjection(t, "projection-1", "parcel-1")
	views := &customerViewStoreDouble{byKey: map[string]vedomain.CustomerTrackingView{}}
	handler := realViewDeriveHandler(views, viewPolicyDouble{configured: false}, viewDownstreamDouble{})
	subject := viewSubject(t,
		&currentProjectionDouble{projection: projection, found: true},
		resolvedAccount(t), handler)

	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); err != nil {
		t.Fatalf("重投应答已有结果入账：%v", err)
	}
	if len(views.byKey) != 1 {
		t.Fatalf("视图行数 = %d, want 1", len(views.byKey))
	}
}

func TestAnUndecidedViewDerivationRollsBack(t *testing.T) {
	views := &customerViewStoreDouble{
		byKey:   map[string]vedomain.CustomerTrackingView{},
		findErr: errors.New("view store down"),
	}
	handler := realViewDeriveHandler(views, viewPolicyDouble{configured: false}, viewDownstreamDouble{})
	subject := viewSubject(t,
		&currentProjectionDouble{projection: builtProjection(t, "projection-1", "parcel-1"), found: true},
		resolvedAccount(t), handler)

	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); !errors.Is(err, veconsume.ErrCustomerViewUndecided) {
		t.Fatalf("err = %v, want ErrCustomerViewUndecided", err)
	}
}

// Covers: 视图已发布但意图没交出去——必须拦在入账前，否则视图在、下游 outbox 永久缺。
func TestAPendingViewHandoffBlocksConsumption(t *testing.T) {
	views := &customerViewStoreDouble{byKey: map[string]vedomain.CustomerTrackingView{}}
	handler := realViewDeriveHandler(views, viewPolicyDouble{configured: false},
		viewDownstreamDouble{err: errors.New("outbox down")})
	subject := viewSubject(t,
		&currentProjectionDouble{projection: builtProjection(t, "projection-1", "parcel-1"), found: true},
		resolvedAccount(t), handler)

	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); !errors.Is(err, veconsume.ErrCustomerViewHandoffPending) {
		t.Fatalf("err = %v, want ErrCustomerViewHandoffPending", err)
	}
}

// Covers: 封闭集合外的结果（含本路走不到的 NOT_ACCEPTED）不留兜底，响亮报错；命令
// 三件套（租户/账户/当前投影）原样到达编排。
func TestAnOutcomeOutsideTheClosedSetIsLoud(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := viewSubject(t,
		&currentProjectionDouble{projection: builtProjection(t, "projection-1", "parcel-1"), found: true},
		resolvedAccount(t), derive)

	if err := subject.HandleDerivedTrackingProjection(t.Context(), derivedRef("projection-1")); !errors.Is(err, veconsume.ErrUnexpectedCustomerViewOutcome) {
		t.Fatalf("err = %v, want ErrUnexpectedCustomerViewOutcome", err)
	}
	if derive.calls != 1 {
		t.Fatalf("派生调用次数 = %d, want 1", derive.calls)
	}
	if derive.last.Customer.String() != "customer-1" || derive.last.TenantID.String() != "tenant-1" ||
		derive.last.Projection.Version().String() != "projection-1" {
		t.Fatalf("命令走样：%+v", derive.last)
	}
}
