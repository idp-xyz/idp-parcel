package application

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// projectCandidateTime 按节点业务时区把处理时长和截单折成绝对时刻（ADR-0175）。
// 节点没有可用日历（整行缺席，或没登记处理时长）时第三返回值点名那个节点：不补默认日历。
//
// 末个节点的日历登了派送时长、且候选的交付范围已解析出来（scoped），段链末尾接一段交付段，
// 候选的投影窗口也延到交付段的终点（ADR-0217）：预计承诺说的是交到收件人，止于交付节点的投影
// 会把派送那一截算成零。没登派送时长就不成段，投影止于离开交付节点。
func projectCandidateTime(
	candidate catalogCandidate,
	scope domain.ServiceAreaVersionReference,
	scoped bool,
	asOf time.Time,
	nodes map[string]ports.NodeDefinitionVersion,
	calendars []ports.ServiceCalendarDefinitionVersion,
) (domain.CandidateTimeProjection, ports.CandidatePath, string, error) {
	none := domain.CandidateTimeProjection{}
	noPath := ports.CandidatePath{}
	order := nodeOrder(candidate)
	cursor := asOf.UTC()
	var basis []string
	path := ports.CandidatePath{Candidate: candidate.id, Legs: make([]domain.PlannedLeg, 0, len(candidate.legs))}
	for index, node := range order {
		definition, found := nodes[node]
		if !found {
			return none, noPath, "", fmt.Errorf("%w: node %s has no applicable version", ErrCatalogUnresolvable, node)
		}
		calendar, registered := nodeCalendar(calendars, node)
		if !registered || calendar.ProcessingMinutes == nil {
			return none, noPath, node, nil
		}
		departed, err := departAt(cursor, definition.BusinessTimezone, calendar)
		if err != nil {
			return none, noPath, "", err
		}
		basis = append(basis, "NODE/"+versionReference(calendar.TargetCode, calendar.Version))
		if index == len(order)-1 {
			cursor = departed
			if calendar.DeliveryMinutes != nil && scoped {
				delivered := departed.Add(time.Duration(*calendar.DeliveryMinutes) * time.Minute)
				leg, err := deliveryLeg(node, calendar, scope, departed, delivered)
				if err != nil {
					return none, noPath, "", err
				}
				path.Delivery, path.HasDelivery = leg, true
				cursor = delivered
			}
			break
		}
		connection := candidate.legs[index]
		arrival := departed
		if buffer, declared := connectionBuffer(calendars, connection.Code); declared {
			arrival = departed.Add(time.Duration(*buffer.BufferMinutes) * time.Minute)
			basis = append(basis, "CONNECTION/"+versionReference(buffer.TargetCode, buffer.Version))
		}
		leg, err := plannedLeg(connection, departed, arrival)
		if err != nil {
			return none, noPath, "", err
		}
		path.Legs = append(path.Legs, leg)
		cursor = arrival
	}
	windowBasis, err := domain.NewWindowBasisReference(joinSortedUnique(basis))
	if err != nil {
		return none, noPath, "", err
	}
	window, err := domain.NewPlannedTimeWindow(asOf.UTC(), cursor.UTC(), windowBasis)
	if err != nil {
		return none, noPath, "", fmt.Errorf("%w: candidate %s: %v", ErrCatalogUnresolvable, candidate.id, err)
	}
	projection, err := domain.NewCandidateTimeProjection(candidate.id, window)
	if err != nil {
		return none, noPath, "", err
	}
	return projection, path, "", nil
}

func nodeOrder(candidate catalogCandidate) []string {
	if len(candidate.legs) == 0 {
		return nil
	}
	order := []string{candidate.legs[0].FromNode}
	for _, leg := range candidate.legs {
		order = append(order, leg.ToNode)
	}
	return order
}

func nodeCalendar(calendars []ports.ServiceCalendarDefinitionVersion, node string) (ports.ServiceCalendarDefinitionVersion, bool) {
	for _, calendar := range calendars {
		if calendar.TargetKind == ports.TargetNode && calendar.TargetCode == node {
			return calendar, true
		}
	}
	return ports.ServiceCalendarDefinitionVersion{}, false
}

func connectionBuffer(calendars []ports.ServiceCalendarDefinitionVersion, connection string) (ports.ServiceCalendarDefinitionVersion, bool) {
	for _, calendar := range calendars {
		if calendar.TargetKind == ports.TargetConnection && calendar.TargetCode == connection && calendar.BufferMinutes != nil {
			return calendar, true
		}
	}
	return ports.ServiceCalendarDefinitionVersion{}, false
}

// departAt 把到达绝对时刻换成离开绝对时刻。本地时刻不早于截单时，从下一个本地日历日的零点起算处理时长。
func departAt(arrival time.Time, timezone string, calendar ports.ServiceCalendarDefinitionVersion) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: business timezone %q: %v", ErrCatalogUnresolvable, timezone, err)
	}
	local := arrival.In(location)
	start := local
	if calendar.CutoffLocalMinute != nil {
		minute := local.Hour()*60 + local.Minute()
		if minute >= *calendar.CutoffLocalMinute {
			start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1)
		}
	}
	return start.Add(time.Duration(*calendar.ProcessingMinutes) * time.Minute).UTC(), nil
}

func plannedLeg(
	connection ports.ConnectionDefinitionVersion,
	earliest, latest time.Time,
) (domain.PlannedLeg, error) {
	from, err := domain.NewPlanNodeReference(connection.FromNode)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	to, err := domain.NewPlanNodeReference(connection.ToNode)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	// 目录没有责任方列。段的责任引用先指连接版本，不编造法人。
	responsible, err := domain.NewResponsiblePartyReference(versionReference(connection.Code, connection.Version))
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	basis, err := domain.NewWindowBasisReference("CONNECTION/" + versionReference(connection.Code, connection.Version))
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	window, err := domain.NewPlannedTimeWindow(earliest.UTC(), latest.UTC(), basis)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	return domain.NewPlannedLeg(domain.PlannedLegSpec{
		From: from, To: to, Responsible: responsible, Window: window,
	})
}

// deliveryLeg 立交付段（ADR-0217）：起于交付节点，止于交付范围。窗口依据指交付节点那一版日历——派送
// 时长与离开时刻都出自它；目录没有末端责任方列，责任引用先指交付范围，与节点间段先指连接版本同一个
// 「不编造法人」的退法。
func deliveryLeg(
	node string,
	calendar ports.ServiceCalendarDefinitionVersion,
	scope domain.ServiceAreaVersionReference,
	earliest, latest time.Time,
) (domain.PlannedLeg, error) {
	from, err := domain.NewPlanNodeReference(node)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	responsible, err := domain.NewResponsiblePartyReference(scope.String())
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	basis, err := domain.NewWindowBasisReference("NODE/" + versionReference(calendar.TargetCode, calendar.Version))
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	window, err := domain.NewPlannedTimeWindow(earliest.UTC(), latest.UTC(), basis)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	return domain.NewDeliveryLeg(domain.DeliveryLegSpec{
		From: from, Scope: scope, Responsible: responsible, Window: window,
	})
}
