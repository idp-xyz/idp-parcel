package application

import (
	"context"
	"errors"
	"fmt"
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

var errDepartureGateViewMisanswered = errors.New("transport fulfillment: the departure gate view answered for another object")

// DepartureGateReadDeps 是出发门禁读数要问的三口：本上下文自己的段册与班次册，以及关务的动作准入判断。
type DepartureGateReadDeps struct {
	Segments  ports.DepartureSegmentView
	Schedules ports.TransportScheduleReader
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
	if deps.Segments == nil || deps.Schedules == nil || deps.Customs == nil {
		return nil, errors.New("transport fulfillment: the departure gate reader needs the segment view, the schedule reader and the customs view")
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
	return reader.readIntent(ctx, intent)
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

// readIntent 先在本上下文这一侧找齐问关务要的东西：段里出发那一刻在控的对象、班次的方向。缺哪样就停在哪样、不问关务——
// 没有对象就没有可问的，没有方向就说不出「自哪到哪」。找齐了才逐对象问；某一件读不动不停下，其余照问，每件各记一格。
// error 只留给读数自己拼不出来（实现坏了）。
func (reader *DepartureGateReader) readIntent(ctx context.Context, intent departureGateIntent) (domain.DepartureGateReading, error) {
	var stops []domain.DepartureGateStop
	var objects []domain.CarriedObjectReference
	if intent.segment == (domain.FulfillmentSegmentReference{}) {
		stops = append(stops, domain.DepartureSegmentNotDeclared)
	} else {
		record, found, err := reader.deps.Segments.FindByKey(ctx, ports.FulfillmentSegmentKey{TenantID: intent.tenant, Segment: intent.segment})
		switch {
		case err != nil:
			stops = append(stops, domain.DepartureSegmentRegistryUnavailable)
		case !found:
			stops = append(stops, domain.DepartureSegmentNotRegistered)
		default:
			if objects = record.Segment.ObjectsUnderControlAt(intent.at); len(objects) == 0 {
				stops = append(stops, domain.DepartureNoObjectUnderControl)
			}
		}
	}

	var direction domain.ScheduleDirection
	directed := false
	schedule, found, err := reader.deps.Schedules.FindByKey(ctx, ports.ScheduleKey{TenantID: intent.tenant, Schedule: intent.schedule})
	switch {
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

	for _, object := range objects {
		finding, err := reader.deps.Customs.ReadDepartureGate(ctx, ports.DepartureGateQuery{
			TenantID: intent.tenant,
			Object:   object,
			From:     reading.From,
			To:       reading.To,
			At:       intent.at,
		})
		if err != nil {
			cause := departureGateCustomsUnavailable
			if errors.Is(err, ports.ErrDepartureGateAnswerUntranslatable) {
				cause = departureGateCustomsUntranslatable
			}
			if finding, err = domain.NewDepartureGateFinding(object, domain.DepartureGateUnreadable, "",
				[]domain.DepartureGateBasis{{Cause: cause}}); err != nil {
				return domain.DepartureGateReading{}, err
			}
		}
		if finding.Object() != object {
			// 端口答的是另一件、或交回零值却不报错：那是实现坏了，不是关务读不动，记成一格只会把它藏起来。
			return domain.DepartureGateReading{}, fmt.Errorf("%w: the customs view answered for %q when asked about %q",
				errDepartureGateViewMisanswered, finding.Object(), object)
		}
		reading.Findings = append(reading.Findings, finding)
	}
	return domain.FormDepartureGateReading(reading)
}
