package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 关务那一问没有形成本上下文认得的答复时，那一件记读不动，依据种类按救法分三种：
//   - departureGateCustomsUnavailable：调用失败，要查接线与关务进程，过一会儿可能就好；
//   - departureGateCustomsUntranslatable：答复译不回来，重问也是同一个结果，要查两边的口径；
//   - 关务自己答读不动时取关务指名的那一口（不是这里的常量），要查关务的某本册。
const (
	departureGateCustomsUnavailable    = "CUSTOMS_ADMISSION_UNAVAILABLE"
	departureGateCustomsUntranslatable = "CUSTOMS_ADMISSION_UNTRANSLATABLE"
)

// 集运成员关系那一问没有形成答复时，那一件同样记读不动：说不出它是不是一袋，就不能当成一件包裹去问关务（ADR-0225）。依据种类
// 照关务那两种的分法：调用失败过一会儿可能就好，译不回来要查两边的口径。
const (
	departureGateMembershipUnavailable    = "CONSOLIDATION_MEMBERSHIP_UNAVAILABLE"
	departureGateMembershipUntranslatable = "CONSOLIDATION_MEMBERSHIP_UNTRANSLATABLE"
)

var (
	errDepartureGateViewMisanswered = errors.New("transport fulfillment: the departure gate view answered for another object")
	errDepartureEntryBindingUnknown = errors.New("transport fulfillment: the departure entry binding answered outside its closed set")
	errConsolidationMemberMisshaped = errors.New("transport fulfillment: the consolidation membership view answered a member of no known kind")
)

// DepartureGateReadDeps 是出发门禁读数要问的几口：本上下文自己的段册、班次册与交接册（核班次是不是载着这一段），node-operations
// 的集运成员关系，以及关务的动作准入判断。
type DepartureGateReadDeps struct {
	Segments  ports.DepartureSegmentView
	Schedules ports.TransportScheduleReader
	Handovers ports.EntryHandoverView
	Members   ports.ConsolidationMembershipView
	Customs   ports.DepartureGateView
}

// DepartureGateReader 为一条出发形成门禁读数，供 RecordMovementFactHandler.Record 与事实同一笔落册（ADR-0205）；
// 设备出发前问询（PreviewDepartureGateHandler，ADR-0219）走同一条路，读数交回设备、不落册。
//
// 它由组合根在写事务之前、用外层 ctx 调：关务的判断在它自己的事务里记下（ADR-0206 决定六），嵌进出发登记那一笔会
// 随它回滚；读在池上，不在写事务里，任何一口读不动都只是读数里的一格，挡不住落册。
type DepartureGateReader struct {
	deps DepartureGateReadDeps
}

// NewDepartureGateReader 构造期拒缺件：漏装的一口到第一条出发才暴露，会被记成一格读不动，把装配缺陷演成依赖故障。
func NewDepartureGateReader(deps DepartureGateReadDeps) (*DepartureGateReader, error) {
	if deps.Segments == nil || deps.Schedules == nil || deps.Handovers == nil || deps.Members == nil || deps.Customs == nil {
		return nil, errors.New("transport fulfillment: the departure gate reader needs the segment view, the schedule reader, the entry handover view, the consolidation membership view and the customs view")
	}
	return &DepartureGateReader{deps: deps}, nil
}

// Read 只为出发问；移动、到达与受理不了的命令交回零值读数，由登记编排照常答（移动与到达不带读数，受理不了的答
// 未受理）。问不问关务不看设备自报：设门与否由关务按对象与起讫判（ADR-0205 决定三）。
func (reader *DepartureGateReader) Read(ctx context.Context, command RecordMovementFactCommand) (domain.DepartureGateReading, error) {
	spec, _, accepted := movementFactSpecFrom(command)
	if !accepted || spec.Kind != domain.DepartureFact {
		return domain.DepartureGateReading{}, nil
	}
	intent, accepted := departureGateIntentOf(command.TenantID, command.Schedule, command.Segment, command.OccurredAt)
	if !accepted {
		return domain.DepartureGateReading{}, nil
	}
	return reader.readIntent(ctx, intent, nil)
}

// errDepartureGateReaskMisplaced：交来补问的不是一条声明了段的出发。补问只由 ReaskDepartureGatesHandler 按段列出的出发调，
// 走到这里是接线错了，上抛而不形成一份读数。
var errDepartureGateReaskMisplaced = errors.New("transport fulfillment: only a departure declaring its segment can be re-asked")

// Reask 按一条在册出发的原出发时点、只替 object 再问一次（ADR-0305）：意图取那一版事实自己的租户、班次、声明的段与业务发生时间，
// 与登记那一刻同一条读数路径（readIntent），只是在控的对象收到 object 这一件——集运单元照旧按出发那一刻的成员展开，班次与段照旧
// 核这一件进段的那次交接。段册、班次册、交接册、成员关系与关务都按此刻的册子读：段登记了、对象凭控制事实进了段、某一口读得动了，
// 补问才问得出登记那一刻问不出的东西。object 此刻在那一刻不在控，停在「出发那一刻段里没有在控对象」。
func (reader *DepartureGateReader) Reask(
	ctx context.Context,
	fact domain.TransportMovementFact,
	object domain.CarriedObjectReference,
) (domain.DepartureGateReading, error) {
	segment, declared := fact.Segment()
	if fact.Kind() != domain.DepartureFact || !declared {
		return domain.DepartureGateReading{}, errDepartureGateReaskMisplaced
	}
	intent := departureGateIntent{tenant: fact.TenantID(), schedule: fact.Schedule(), segment: segment, at: fact.OccurredAt()}
	return reader.readIntent(ctx, intent, &gateFocus{inControl: object})
}

// ReaskMember 按一条在册出发的原出发时点，替一件经集运单元随袋走过这条出发的成员再问一次关务（ADR-0331）：段与班次的核对落在出发
// 那一刻在控的那件单元身上（它进段的那次交接），关务只问 member 这一件包裹，单元与同袋别的成员一概不问，读数里只有这一件的一格。
// 其余同 Reask：与登记那一刻同一条读数路径，册子按此刻读。member 在那一刻是不是随这件单元走，由 CarriesMember 先答，本方法不重判。
func (reader *DepartureGateReader) ReaskMember(
	ctx context.Context,
	fact domain.TransportMovementFact,
	unit, member domain.CarriedObjectReference,
) (domain.DepartureGateReading, error) {
	segment, declared := fact.Segment()
	if fact.Kind() != domain.DepartureFact || !declared {
		return domain.DepartureGateReading{}, errDepartureGateReaskMisplaced
	}
	intent := departureGateIntent{tenant: fact.TenantID(), schedule: fact.Schedule(), segment: segment, at: fact.OccurredAt()}
	return reader.readIntent(ctx, intent, &gateFocus{inControl: unit, member: &member})
}

// CarriesMember 说集运单元 unit 在 at 那一刻，逐层往下是不是装着 member 这件包裹。成员口径就是出发读数展开用的那一口
// （ConsolidationMembershipView.MembersAt）：按成员关系事实的业务发生时间取，成员的包裹身份取此刻的当前有效版本——身份不成立或读不动
// 的成员不是任何一件包裹，不算。嵌套单元逐层往下、每件只问一次，成环的册也走得完。
//
// error 是成员关系那一问没有形成答复（包住 ports.ErrConsolidationMembershipUntranslatable 的是译不回来，其余是调用失败）：说不出就
// 不能答「不在」，那等于放过一件本该问关务的包裹。
func (reader *DepartureGateReader) CarriesMember(
	ctx context.Context,
	tenant domain.TenantID,
	unit, member domain.CarriedObjectReference,
	at time.Time,
) (bool, error) {
	seen := map[domain.CarriedObjectReference]bool{unit: true}
	queue := []domain.CarriedObjectReference{unit}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		membership, err := reader.deps.Members.MembersAt(ctx, ports.ConsolidationMembershipQuery{TenantID: tenant, Object: current, At: at})
		if err != nil {
			return false, err
		}
		for _, held := range membership.Members {
			switch held.Kind {
			case ports.ConsolidationMemberParcel:
				if held.Object == member {
					return true, nil
				}
			case ports.ConsolidationMemberUnit:
				if !seen[held.Object] {
					seen[held.Object] = true
					queue = append(queue, held.Object)
				}
			case ports.ConsolidationMemberIdentityUnresolved, ports.ConsolidationMemberIdentityUnreadable:
			default:
				return false, fmt.Errorf("%w: member %q of %q", ports.ErrConsolidationMembershipUntranslatable, held.Handling, current)
			}
		}
	}
	return false, nil
}

// departureGateIntent 是读数从输入里只取的那四样：租户、班次、声明的段（零值即没声明）与出发时点；其余取决于此刻的段册、
// 班次册与关务。出发登记与出发前问询（ADR-0219）都经 departureGateIntentOf 把输入收成它，再走同一个 readIntent——「同参
// 同读数」靠的是只有这一条路，不靠两边各写一遍再比对。
type departureGateIntent struct {
	tenant   domain.TenantID
	schedule domain.ScheduleReference
	segment  domain.FulfillmentSegmentReference
	at       time.Time
}

// departureGateIntentOf 是两口把输入收成 departureGateIntent 的唯一一处：出发登记的 Read 与出发前问询的 Preview 都调它，
// 同名三格加时点在两口受理与否一致由结构保证，不靠两份判据各自跟着改。时点的判据同出发构造门（domain.RecordMovementFact）：
// 按登记精度为零即不受理——不拿服务端时钟补（ADR-0023）。
func departureGateIntentOf(tenant domain.TenantID, schedule, segment string, at time.Time) (departureGateIntent, bool) {
	if domain.AtRegistryPrecision(at).IsZero() {
		return departureGateIntent{}, false
	}
	scheduleRef, segmentRef, accepted := gateReferencesFrom(tenant, schedule, segment)
	if !accepted {
		return departureGateIntent{}, false
	}
	return departureGateIntent{tenant: tenant, schedule: scheduleRef, segment: segmentRef, at: at}, true
}

// gateReferencesFrom 过读数取决于的租户、班次与段：段可缺，空串读作没有声明，给了就得立得起来。movementFactSpecFrom 也过
// 这一处——日后给班次或段加判据，出发登记、出发读数与问询三处一起变。
func gateReferencesFrom(
	tenant domain.TenantID,
	schedule, segment string,
) (domain.ScheduleReference, domain.FulfillmentSegmentReference, bool) {
	none := domain.FulfillmentSegmentReference{}
	if strings.TrimSpace(tenant.String()) == "" {
		return domain.ScheduleReference{}, none, false
	}
	scheduleRef, err := domain.NewScheduleReference(schedule)
	if err != nil {
		return domain.ScheduleReference{}, none, false
	}
	if strings.TrimSpace(segment) == "" {
		return scheduleRef, none, true
	}
	segmentRef, err := domain.NewFulfillmentSegmentReference(segment)
	if err != nil {
		return domain.ScheduleReference{}, none, false
	}
	return scheduleRef, segmentRef, true
}

// gateFocus 是补问把一次读数收窄到的范围。
type gateFocus struct {
	inControl domain.CarriedObjectReference
	member    *domain.CarriedObjectReference
}

// readIntent 先在本上下文这一侧找齐问关务要的东西：段里出发那一刻在控的对象、班次的方向，再核这一班与这一段（entryBinding）。
// 缺哪样就停在哪样、不问关务——没有对象就没有可问的，没有方向就说不出「自哪到哪」，班次对不上这一段就是在替别的一车问。核不上
// 但也对不出错的（未绑定）照问，读数标上那一位，回给执行方时一件不列。找齐了才逐对象问（集运单元按成员展开，见
// departureGateExpansion）；某一件读不动不停下，其余照问，每件各记一格。error 只留给读数自己拼不出来（实现坏了）。
//
// focus 非空时是补问（ADR-0305、ADR-0331）：段里在控的对象只留 focus.inControl 这一件，核班次与段也只核它进段的那次交接；
// focus.member 非空时关务只问这一件成员，在控的 focus.inControl 是随袋带着它的那件单元，不展开、不问关务。段册或班次册那一行装不回
// 与连不上分记两个停点（ports 的两个 RecordUnreadable 哨兵）：同属读不动，救法不同，补问只为后者再问。
func (reader *DepartureGateReader) readIntent(
	ctx context.Context,
	intent departureGateIntent,
	focus *gateFocus,
) (domain.DepartureGateReading, error) {
	var stops []domain.DepartureGateStop
	var inControl []domain.FulfillmentParticipation
	if intent.segment == (domain.FulfillmentSegmentReference{}) {
		stops = append(stops, domain.DepartureSegmentNotDeclared)
	} else {
		record, found, err := reader.deps.Segments.FindByKey(ctx, ports.FulfillmentSegmentKey{TenantID: intent.tenant, Segment: intent.segment})
		switch {
		case errors.Is(err, ports.ErrFulfillmentSegmentRecordUnreadable):
			stops = append(stops, domain.DepartureSegmentRecordUnreadable)
		case err != nil:
			stops = append(stops, domain.DepartureSegmentRegistryUnavailable)
		case !found:
			stops = append(stops, domain.DepartureSegmentNotRegistered)
		default:
			inControl = record.Segment.ParticipationsUnderControlAt(intent.at)
			if focus != nil {
				inControl = slices.DeleteFunc(inControl, func(participation domain.FulfillmentParticipation) bool {
					return participation.Object() != focus.inControl
				})
			}
			if len(inControl) == 0 {
				stops = append(stops, domain.DepartureNoObjectUnderControl)
			}
		}
	}

	var direction domain.ScheduleDirection
	directed := false
	schedule, found, err := reader.deps.Schedules.FindByKey(ctx, ports.ScheduleKey{TenantID: intent.tenant, Schedule: intent.schedule})
	switch {
	case errors.Is(err, ports.ErrTransportScheduleRecordUnreadable):
		stops = append(stops, domain.DepartureScheduleRecordUnreadable)
	case err != nil:
		stops = append(stops, domain.DepartureScheduleRegistryUnavailable)
	case !found:
		stops = append(stops, domain.DepartureScheduleNotRegistered)
	default:
		direction, directed = schedule.Schedule.Direction(), true
	}

	reading := domain.DepartureGateReadingSpec{Stops: stops}
	if directed {
		reading.From, reading.To = direction.From(), direction.To()
	}
	if len(stops) > 0 {
		return domain.FormDepartureGateReading(reading)
	}
	binding, readable, err := reader.entryBinding(ctx, intent.tenant, direction, inControl)
	if err != nil {
		return domain.DepartureGateReading{}, err
	}
	if !readable {
		reading.Stops = []domain.DepartureGateStop{domain.DepartureHandoverRegistryUnavailable}
		return domain.FormDepartureGateReading(reading)
	}
	switch binding {
	case domain.DepartureEntryMismatched:
		reading.Stops = []domain.DepartureGateStop{domain.DepartureScheduleNotServingSegment}
		return domain.FormDepartureGateReading(reading)
	case domain.DepartureEntryUnbound:
		reading.EntryUnbound = true
	case domain.DepartureEntryBound:
	default:
		return domain.DepartureGateReading{}, fmt.Errorf("%w: %d", errDepartureEntryBindingUnknown, binding)
	}

	expansion := departureGateExpansion{
		reader: reader,
		intent: intent,
		from:   reading.From,
		to:     reading.To,
		cells:  map[domain.CarriedObjectReference]domain.GateFinding{},
		onPath: map[domain.CarriedObjectReference]bool{},
	}
	if focus != nil && focus.member != nil {
		if _, err := expansion.object(ctx, *focus.member); err != nil {
			return domain.DepartureGateReading{}, err
		}
	} else {
		for _, participation := range inControl {
			if _, err := expansion.object(ctx, participation.Object()); err != nil {
				return domain.DepartureGateReading{}, err
			}
		}
	}
	for _, cell := range expansion.cells {
		reading.Findings = append(reading.Findings, cell)
	}
	return domain.FormDepartureGateReading(reading)
}

// entryBinding 认段里在控各件进段的那次`已交接`，交领域核班次的起讫是不是它们声明的交出、去向节点（domain.BindDepartureEntries）。
// 交接册读不动时 readable 为假：核不了就说不出这一班载的是不是这一段，调用方停在读不动、不问关务。
func (reader *DepartureGateReader) entryBinding(
	ctx context.Context,
	tenant domain.TenantID,
	direction domain.ScheduleDirection,
	inControl []domain.FulfillmentParticipation,
) (binding domain.DepartureEntryBinding, readable bool, err error) {
	entries := make([]domain.TransportHandover, 0, len(inControl))
	unresolved := false
	for _, participation := range inControl {
		entry, recognised := entryHandoverOf(ctx, reader.deps.Handovers, tenant, participation)
		switch recognised {
		case entryHandoverFound:
			entries = append(entries, entry)
		case entryHandoverUnreadable:
			return domain.DepartureEntryBindingInvalid, false, nil
		case entryByPickup, entryHandoverNotOnRegister, entryHandoverAmbiguous:
			unresolved = true
		default:
			return domain.DepartureEntryBindingInvalid, false, fmt.Errorf("%w: %d", errEntryHandoverFindingUnknown, recognised)
		}
	}
	return domain.BindDepartureEntries(direction, entries, unresolved), true, nil
}

// departureGateExpansion 是一次读数里逐对象问的那一遍（ADR-0225）。段里在控的每一件先问 node-operations 是不是集运单元：不是就按
// 包裹问关务；是就按出发那一刻的直接成员逐件问，嵌套单元一层层展开到包裹，单元自己那一格由领域取成员里最重的一格
// （domain.FormConsolidationUnitGateFinding）。不问关务单元本身：关务按包裹认对象，问它只会答出一格假的干净读数。
//
// 每件对象只问一次、只有一格：同一件包裹既直接在段里、又在某一袋里，或几袋都指名同一件，问来的是同一格，读数里也只记一格。一格按
// 对象引用记，所以每个引用都先经 object 认种类，成员认出的包裹也不例外：成员的包裹号若恰与段里另一件单元同串，先按包裹问关务记下
// 那一格，那件单元随后取到它就永不展开——同串按单元判（ADR-0214 决定二），先问的一侧不能替后到的一侧定种类。展开
// 只沿这一条路径防环：嵌套成员已在路径上，就记成那一袋的缘由、不再回头；对象与成员都按定序走，同样的三源展开出同样的读数——出发
// 前问询与随后同参出发靠的就是这一点（ADR-0219 决定二）。
type departureGateExpansion struct {
	reader *DepartureGateReader
	intent departureGateIntent
	from   domain.NetworkNodeReference
	to     domain.NetworkNodeReference
	cells  map[domain.CarriedObjectReference]domain.GateFinding
	onPath map[domain.CarriedObjectReference]bool
}

// object 为一件不知种类的对象形成它那一格：段里在控的对象与各直接成员（认出的包裹、嵌套单元）都从这里进。
func (expansion *departureGateExpansion) object(ctx context.Context, object domain.CarriedObjectReference) (domain.GateFinding, error) {
	if cell, formed := expansion.cells[object]; formed {
		return cell, nil
	}
	membership, err := expansion.reader.deps.Members.MembersAt(ctx, ports.ConsolidationMembershipQuery{
		TenantID: expansion.intent.tenant,
		Object:   object,
		At:       expansion.intent.at,
	})
	if err != nil {
		cause := departureGateMembershipUnavailable
		if errors.Is(err, ports.ErrConsolidationMembershipUntranslatable) {
			cause = departureGateMembershipUntranslatable
		}
		return expansion.keep(domain.NewGateFinding(object, domain.GateUnreadable, "", []domain.GateBasis{{Cause: cause}}))
	}
	if !membership.Unit {
		return expansion.parcel(ctx, object)
	}
	return expansion.unit(ctx, object, membership.Members)
}

// unit 按直接成员形成一件集运单元那一格。问不到关务的成员按缘由记进单元那一格的依据，各缘由所属的格由领域定。
func (expansion *departureGateExpansion) unit(
	ctx context.Context,
	unit domain.CarriedObjectReference,
	members []ports.ConsolidationMember,
) (domain.GateFinding, error) {
	expansion.onPath[unit] = true
	defer delete(expansion.onPath, unit)

	var cells []domain.GateFinding
	named := map[domain.CarriedObjectReference]bool{}
	sources := map[domain.ConsolidationMemberStop][]string{}
	for _, member := range members {
		switch member.Kind {
		case ports.ConsolidationMemberParcel, ports.ConsolidationMemberUnit:
			if named[member.Object] {
				continue
			}
			if expansion.onPath[member.Object] {
				sources[domain.ConsolidationMembershipCycle] = append(sources[domain.ConsolidationMembershipCycle], member.Object.String())
				continue
			}
			cell, err := expansion.object(ctx, member.Object)
			if err != nil {
				return domain.GateFinding{}, err
			}
			named[member.Object] = true
			cells = append(cells, cell)
		case ports.ConsolidationMemberIdentityUnresolved:
			sources[domain.ConsolidationMemberIdentityUnresolved] = append(sources[domain.ConsolidationMemberIdentityUnresolved], member.Handling)
		case ports.ConsolidationMemberIdentityUnreadable:
			sources[domain.ConsolidationMemberIdentityUnreadable] = append(sources[domain.ConsolidationMemberIdentityUnreadable], member.Handling)
		default:
			// 成员落在封闭集外：那是适配器坏了，不是那一件读不动——记成一格只会把它藏起来。
			return domain.GateFinding{}, fmt.Errorf("%w: member %q of %q", errConsolidationMemberMisshaped, member.Handling, unit)
		}
	}
	var gaps []domain.ConsolidationMemberGap
	for _, stop := range []domain.ConsolidationMemberStop{
		domain.ConsolidationMemberIdentityUnresolved, domain.ConsolidationMemberIdentityUnreadable, domain.ConsolidationMembershipCycle,
	} {
		if len(sources[stop]) > 0 {
			gaps = append(gaps, domain.ConsolidationMemberGap{Stop: stop, Sources: sources[stop]})
		}
	}
	return expansion.keep(domain.FormConsolidationUnitGateFinding(unit, cells, gaps))
}

// parcel 按包裹问关务那一件，只由 object 在认出「不是单元」之后调。关务那一问没形成本上下文认得的答复时那一件记读不动，依据种类按
// 救法分开（本文件头上的常量）。
func (expansion *departureGateExpansion) parcel(ctx context.Context, object domain.CarriedObjectReference) (domain.GateFinding, error) {
	finding, err := expansion.reader.deps.Customs.ReadDepartureGate(ctx, ports.DepartureGateQuery{
		TenantID: expansion.intent.tenant,
		Object:   object,
		From:     expansion.from,
		To:       expansion.to,
		At:       expansion.intent.at,
	})
	if err != nil {
		cause := departureGateCustomsUnavailable
		if errors.Is(err, ports.ErrDepartureGateAnswerUntranslatable) {
			cause = departureGateCustomsUntranslatable
		}
		return expansion.keep(domain.NewDepartureGateFinding(object, domain.DepartureGateUnreadable, "",
			[]domain.DepartureGateBasis{{Cause: cause}}))
	}
	if finding.Object() != object {
		// 端口答的是另一件、或交回零值却不报错：那是实现坏了，不是关务读不动，记成一格只会把它藏起来。
		return domain.GateFinding{}, fmt.Errorf("%w: the customs view answered for %q when asked about %q",
			errDepartureGateViewMisanswered, finding.Object(), object)
	}
	return expansion.keep(finding, nil)
}

func (expansion *departureGateExpansion) keep(finding domain.GateFinding, err error) (domain.GateFinding, error) {
	if err != nil {
		return domain.GateFinding{}, err
	}
	expansion.cells[finding.Object()] = finding
	return finding, nil
}
