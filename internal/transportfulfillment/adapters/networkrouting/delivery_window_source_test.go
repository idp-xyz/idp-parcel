package networkrouting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/networkrouting"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉票 tf-segment-lifecycle-closure/13 完成判据：五种输入各有唯一答格（缺席 / 解析失败 / 未找到 / 找到 / 读失败），
// 无 default；引用指已被替代版本的段仍答 RESOLVED（裁决②）。NR 那一侧用替身：本包只翻译 NR 的答法，不复证 NR 的读口。
// 夹具全部合成，不写任何真实时间窗取值。

var windowFixtureAt = time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

// plannedLegWindowStub 记下 NR 被问了什么、按脚本答。它没有「适用性」一格可供适配器读——适配器要是想按四态分支，
// 在这个接口上根本拿不到那一格，这正是裁决②在类型上的样子。
type plannedLegWindowStub struct {
	calls      int
	tenant     nrdomain.TenantID
	reference  nrdomain.PlannedLegReference
	window     nrdomain.PlannedTimeWindow
	found      bool
	err        error
	windowByID map[string]nrdomain.PlannedTimeWindow
}

func (stub *plannedLegWindowStub) LoadPlannedLegWindow(
	_ context.Context, tenant nrdomain.TenantID, reference nrdomain.PlannedLegReference,
) (nrdomain.PlannedTimeWindow, bool, error) {
	stub.calls++
	stub.tenant, stub.reference = tenant, reference
	if stub.err != nil {
		return nrdomain.PlannedTimeWindow{}, false, stub.err
	}
	if stub.windowByID != nil {
		window, found := stub.windowByID[reference.String()]
		return window, found, nil
	}
	return stub.window, stub.found, nil
}

func plannedWindow(t *testing.T, offset time.Duration) nrdomain.PlannedTimeWindow {
	t.Helper()
	basis, err := nrdomain.NewWindowBasisReference("calendar/v1")
	if err != nil {
		t.Fatalf("窗口依据：%v", err)
	}
	window, err := nrdomain.NewPlannedTimeWindow(windowFixtureAt.Add(offset), windowFixtureAt.Add(offset+4*time.Hour), basis)
	if err != nil {
		t.Fatalf("计划窗口：%v", err)
	}
	return window
}

func tfValue[T any](t *testing.T, construct func(string) (T, error), value string) T {
	t.Helper()
	built, err := construct(value)
	if err != nil {
		t.Fatalf("构造 %q：%v", value, err)
	}
	return built
}

func newDeliveryWindows(t *testing.T, stub *plannedLegWindowStub) *adapter.DeliveryWindows {
	t.Helper()
	windows, err := adapter.NewDeliveryWindows(stub)
	if err != nil {
		t.Fatalf("构造适配器：%v", err)
	}
	return windows
}

// Covers: 找到 → RESOLVED 带 Earliest / Latest；引用与租户原样到达 NR（拼写 `<版本>#<序位>` 由 NR 的解析门解回值，
// 序位自 1 起——本适配器不拆串、不自己拼）。
func TestAPlannedLegWindowIsTranslatedToAResolvedDeliveryWindow(t *testing.T) {
	stub := &plannedLegWindowStub{window: plannedWindow(t, 2*time.Hour), found: true}
	windows := newDeliveryWindows(t, stub)

	from, to, resolution, err := windows.LoadDeliveryWindow(context.Background(),
		tfValue(t, tfdomain.NewTenantID, "tenant-1"),
		tfValue(t, tfdomain.NewPlannedSegmentReference, "RPV-000000000007#2"), true)
	if err != nil || resolution != tfports.RequirementResolved {
		t.Fatalf("resolution=%s err=%v，want RESOLVED", resolution, err)
	}
	if !from.Equal(windowFixtureAt.Add(2*time.Hour)) || !to.Equal(windowFixtureAt.Add(6*time.Hour)) {
		t.Fatalf("窗口 = [%v, %v]，与 NR 交回的不等", from, to)
	}
	if stub.calls != 1 || stub.tenant.String() != "tenant-1" ||
		stub.reference.Version().String() != "RPV-000000000007" || stub.reference.Ordinal() != 2 {
		t.Fatalf("NR 收到的问法变形：calls=%d tenant=%s reference=%s", stub.calls, stub.tenant, stub.reference)
	}
}

// Covers: 引用缺席 → MISSING 且不调 NR（裁决③：没有计划段就没有窗口，NR 不给兜底窗口）。
func TestAnAbsentPlannedSegmentAnswersMissingWithoutAskingNetworkRouting(t *testing.T) {
	stub := &plannedLegWindowStub{window: plannedWindow(t, 0), found: true}
	windows := newDeliveryWindows(t, stub)

	_, _, resolution, err := windows.LoadDeliveryWindow(context.Background(),
		tfValue(t, tfdomain.NewTenantID, "tenant-1"), tfdomain.PlannedSegmentReference{}, false)
	if err != nil || resolution != tfports.RequirementMissing {
		t.Fatalf("resolution=%s err=%v，want MISSING", resolution, err)
	}
	if stub.calls != 0 {
		t.Fatalf("缺席仍问了 NR %d 次", stub.calls)
	}
}

// Covers: 引用解析失败（拼写不合 NR 定义）→ MISSING 且不调 NR：悬空引用重跑不会长出那一段来，由人核声明，不是读不到。
func TestAnUnparsableReferenceAnswersMissingNotUnavailable(t *testing.T) {
	for name, spelling := range map[string]string{
		"没有分隔符":  "RPV-000000000007",
		"序位为零":   "RPV-000000000007#0",
		"序位带前导零": "RPV-000000000007#02",
		"序位不是数字": "RPV-000000000007#two",
	} {
		t.Run(name, func(t *testing.T) {
			stub := &plannedLegWindowStub{window: plannedWindow(t, 0), found: true}
			windows := newDeliveryWindows(t, stub)
			_, _, resolution, err := windows.LoadDeliveryWindow(context.Background(),
				tfValue(t, tfdomain.NewTenantID, "tenant-1"),
				tfValue(t, tfdomain.NewPlannedSegmentReference, spelling), true)
			if err != nil || resolution != tfports.RequirementMissing {
				t.Fatalf("resolution=%s err=%v，want MISSING", resolution, err)
			}
			if stub.calls != 0 {
				t.Fatalf("解析失败仍问了 NR %d 次", stub.calls)
			}
		})
	}
}

// Covers: NR 答 found=false（版本不在本租户下 / 序位越界）→ MISSING；NR 返 error → 原样上抛且答法不在二格里，
// 执行器据此读成 DELIVERY_WINDOW_SOURCE_UNAVAILABLE 而不是「没有」。
func TestNotFoundIsMissingWhileAReadFailureIsRaised(t *testing.T) {
	notFound := newDeliveryWindows(t, &plannedLegWindowStub{found: false})
	_, _, resolution, err := notFound.LoadDeliveryWindow(context.Background(),
		tfValue(t, tfdomain.NewTenantID, "tenant-1"),
		tfValue(t, tfdomain.NewPlannedSegmentReference, "RPV-000000000007#9"), true)
	if err != nil || resolution != tfports.RequirementMissing {
		t.Fatalf("未找到：resolution=%s err=%v，want MISSING", resolution, err)
	}

	failure := errors.New("network routing is down")
	failing := newDeliveryWindows(t, &plannedLegWindowStub{err: failure})
	_, _, resolution, err = failing.LoadDeliveryWindow(context.Background(),
		tfValue(t, tfdomain.NewTenantID, "tenant-1"),
		tfValue(t, tfdomain.NewPlannedSegmentReference, "RPV-000000000007#1"), true)
	if !errors.Is(err, failure) || resolution == tfports.RequirementMissing || resolution == tfports.RequirementResolved {
		t.Fatalf("读失败：resolution=%s err=%v，want 原样上抛且答法不在二格里", resolution, err)
	}
}

// Covers: 裁决②——引用指已被替代版本（v1）的段，适配器照样交回那一段的内容答 RESOLVED，不因新版本（v2）在场而
// 改答、也不去读 v2 的段。NR 替身两版都有段：v1 第二段与 v2 第一段窗口不同，读回的必须是 v1 那一段。
func TestASegmentOfASupersededPlanVersionStillResolves(t *testing.T) {
	stub := &plannedLegWindowStub{windowByID: map[string]nrdomain.PlannedTimeWindow{
		"RPV-000000000001#2": plannedWindow(t, 8*time.Hour),
		"RPV-000000000002#1": plannedWindow(t, 30*time.Hour),
	}}
	windows := newDeliveryWindows(t, stub)

	from, to, resolution, err := windows.LoadDeliveryWindow(context.Background(),
		tfValue(t, tfdomain.NewTenantID, "tenant-1"),
		tfValue(t, tfdomain.NewPlannedSegmentReference, "RPV-000000000001#2"), true)
	if err != nil || resolution != tfports.RequirementResolved {
		t.Fatalf("被替代版本的段：resolution=%s err=%v，want RESOLVED", resolution, err)
	}
	if !from.Equal(windowFixtureAt.Add(8*time.Hour)) || !to.Equal(windowFixtureAt.Add(12*time.Hour)) {
		t.Fatalf("读到的不是 v1 第二段的窗口：[%v, %v]", from, to)
	}
	if stub.calls != 1 || stub.reference.String() != "RPV-000000000001#2" {
		t.Fatalf("适配器改问了别的段：calls=%d reference=%s", stub.calls, stub.reference)
	}
}

// Covers: 读口缺席即拒——缺了它这一缝永远答不出，装配期就该拒而不是运行期长成 error。
func TestTheAdapterRefusesANilSource(t *testing.T) {
	if _, err := adapter.NewDeliveryWindows(nil); err == nil {
		t.Fatal("nil 读口被收下了")
	}
}
