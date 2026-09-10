// Package networkrouting 是 transport-fulfillment 对 network-routing 计划履约段窗口读口的消费侧适配器
// （ADR-0025：只有本包可以同时导入两个上下文；只翻译不判断，翻译必须是全函数）。
//
// 它只答一件事：参与关系上登记方关联的那一段计划履约段，计划时间窗口是什么（ADR-0114 决定三的「时间窗」一件；
// ADR-0131 决定一 / 二 / 三）。窗口是计划，任务照抄它作工作范围，不据它推任何实际事实（ADR-0004）——所以这里
// 没有写口，不读适用性，不读对象。
package networkrouting

import (
	"context"
	"errors"
	"fmt"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUntranslatableAnswer 与其余跨上下文适配器的同名哨兵同义：某一侧交出了词汇表之外的内容，是编程错误不是业务答案。
var ErrUntranslatableAnswer = errors.New("transport fulfillment networkrouting adapter: untranslatable answer")

// PlannedLegWindowSource 是 NR 按计划履约段引用取那一段计划时间窗口的窄读口，按 NR 的 ports.PlannedLegWindowRead 原形取。
//
// 只声明这一个方法而不依赖 NR 的 ports 包：那里还有判断口与写口，消费侧拿到它们就拿到了改计划的能力，而「TF 引用
// 有效计划形成执行准备，不修改计划」（TF CONTEXT）正是要在类型上就表达出来的那句话。
type PlannedLegWindowSource interface {
	LoadPlannedLegWindow(
		ctx context.Context,
		tenant nrdomain.TenantID,
		reference nrdomain.PlannedLegReference,
	) (nrdomain.PlannedTimeWindow, bool, error)
}

// DeliveryWindows 把 NR 的窄读口翻成 TF 的 DeliveryWindowSource：按参与关系上的计划履约段引用问，不按对象问
// （ADR-0131 决定一——NR 计划里没有服务动作，按对象问要 NR 推「哪一段是派送段」，ADR-0114 决定一禁止那种推法）。
//
// **四态不分支。** 引用所钉的那一版计划此刻当前有效、已被替代、已失效还是已结束，本适配器一律交回那一段的内容答
// RESOLVED（ADR-0131 决定二）：段序位跨版本不对应，对象凭`已交接`进了这一段它就在执行中，改路说的是尚未执行的
// 剩余旅程；「这一版还算不算数」由 NR 的无当前有效路由与路由偏离两个口专门回答，不借本缝的 MISSING 说话。所以这里
// 不读 PlanApplicability，也没有任何一条按适用性走的分支——不是漏了 default，是四态对窗口答案无差别。
type DeliveryWindows struct {
	windows PlannedLegWindowSource
}

// NewDeliveryWindows 读口必备：缺了它这一缝永远答不出，而「答不出」在端口上会长成 error，日后有人会把它读成 NR 坏了。
func NewDeliveryWindows(windows PlannedLegWindowSource) (*DeliveryWindows, error) {
	if windows == nil {
		return nil, fmt.Errorf("transport fulfillment networkrouting adapter: planned leg window source is nil")
	}
	return &DeliveryWindows{windows: windows}, nil
}

// LoadDeliveryWindow 是全函数，五种输入各落唯一一格、无 default：
//
//   - 引用缺席（present=false，待路由的对象此刻没有计划段）→ MISSING，不出本上下文、不调 NR：没有计划段就没有窗口，
//     NR 不给兜底窗口（ADR-0131 决定三），运营给显式窗口走 admin 写面不是本缝；
//   - 引用解析失败（拼写不合 NR 一处定义的 `<版本>#<序位>`）→ MISSING：登记方关联了一个 NR 认不出的段，重跑不会让它
//     长出来，该显示为 REQUIREMENT_MISSING / DELIVERY_WINDOW 由人核声明，不是读不到（ADR-0131 越权风险点「悬空引用
//     同答缺失」）；
//   - NR 答 found=false（版本不在本租户下、或序位越出那一版的段链）→ MISSING，判据同上；
//   - NR 答 found=true → RESOLVED 带 Earliest / Latest，不看那一版的适用性；
//   - NR 返 error → 原样上抛，执行器读成 DELIVERY_WINDOW_SOURCE_UNAVAILABLE 重跑本拍——把「读不到」折成「没有」会让
//     一次故障长成一个业务答案，两者恢复动作不同（ADR-0029）。
//
// 租户按原值传过去——跨越租户必须在签名上看得见（ADR-0003），适配器不替换也不省略它。依据（Basis）不交：任务口今天
// 只收首尾两点，依据留在 NR 那一侧由需要它的人按引用取。
func (adapter *DeliveryWindows) LoadDeliveryWindow(
	ctx context.Context,
	tenant tfdomain.TenantID,
	planned tfdomain.PlannedSegmentReference,
	present bool,
) (from, to time.Time, resolution tfports.RequirementResolution, err error) {
	if !present {
		return time.Time{}, time.Time{}, tfports.RequirementMissing, nil
	}
	reference, err := nrdomain.ParsePlannedLegReference(planned.String())
	if err != nil {
		return time.Time{}, time.Time{}, tfports.RequirementMissing, nil
	}
	nrTenant, err := nrdomain.NewTenantID(tenant.String())
	if err != nil {
		return time.Time{}, time.Time{}, tfports.RequirementResolutionInvalid, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	window, found, err := adapter.windows.LoadPlannedLegWindow(ctx, nrTenant, reference)
	if err != nil {
		return time.Time{}, time.Time{}, tfports.RequirementResolutionInvalid, fmt.Errorf("load planned leg window: %w", err)
	}
	if !found {
		return time.Time{}, time.Time{}, tfports.RequirementMissing, nil
	}
	return window.Earliest(), window.Latest(), tfports.RequirementResolved, nil
}
