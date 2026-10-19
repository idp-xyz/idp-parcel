package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ScheduleCandidatesOutcome 是「这条连接在这段时间里有哪些班次可评」的答案代数（ADR-0201 决定五、六）。
//
// `未配置`与`已列出`（哪怕列出的是空集）是两格：前者是该连接一份周期安排、一个班次都没登过——缺的是租户取值，
// 评估方记`未配置`并确认消费，不重投（ADR-0094 判据）；后者是登过、只是这段时间里没有候选，评估方据以答`无当前
// 运输机会`。两格都不建占位班次。
type ScheduleCandidatesOutcome uint8

const (
	ScheduleCandidatesOutcomeInvalid ScheduleCandidatesOutcome = iota
	ScheduleCandidatesNotConfigured
	ScheduleCandidatesListed
	ScheduleCandidatesNotAccepted
	ScheduleCandidatesUndecided
)

func (outcome ScheduleCandidatesOutcome) String() string {
	switch outcome {
	case ScheduleCandidatesNotConfigured:
		return "NOT_CONFIGURED"
	case ScheduleCandidatesListed:
		return "CANDIDATES_LISTED"
	case ScheduleCandidatesNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case ScheduleCandidatesUndecided:
		return "CANDIDATES_UNDECIDED"
	default:
		return ""
	}
}

// ScheduleCandidatesQuery 问一条连接上出发时刻落在 [DepartFrom, DepartTo] 的班次。只按连接代码问：起讫不符是淘汰
// 原因，归评估方连同候选一起交代。
type ScheduleCandidatesQuery struct {
	TenantID   domain.TenantID
	Connection string
	DepartFrom time.Time
	DepartTo   time.Time
}

type ScheduleCandidatesResult struct {
	outcome      ScheduleCandidatesOutcome
	candidates   []domain.TransportSchedule
	continuation string
}

func (result ScheduleCandidatesResult) Outcome() ScheduleCandidatesOutcome {
	return result.outcome
}

// Candidates 只在`已列出`时有内容，按出发先后、同刻按班次标识排。
func (result ScheduleCandidatesResult) Candidates() []domain.TransportSchedule {
	return append([]domain.TransportSchedule(nil), result.candidates...)
}

func (result ScheduleCandidatesResult) ContinuationReference() string {
	return result.continuation
}

type ScheduleCandidatesDeps struct {
	Source ports.ScheduleCandidateSource
}

// ScheduleCandidatesHandler 是准备评估候选那一处物化（ADR-0201 决定五）：周期安排在评估时按日期物化，只算不存；
// 已登记的具体班次按身份顶替同一（代码、日期）的物化结果——登记册上那一班可能已改过时刻，它才是这一班现在的
// 样子。哪一班被选中，归调用它的准备编排。
type ScheduleCandidatesHandler struct {
	deps ScheduleCandidatesDeps
}

func NewScheduleCandidatesHandler(deps ScheduleCandidatesDeps) *ScheduleCandidatesHandler {
	return &ScheduleCandidatesHandler{deps: deps}
}

func (handler *ScheduleCandidatesHandler) Candidates(
	ctx context.Context,
	query ScheduleCandidatesQuery,
) (ScheduleCandidatesResult, error) {
	connection, err := domain.NewNetworkConnectionReference(query.Connection)
	if err != nil || strings.TrimSpace(query.TenantID.String()) == "" ||
		query.DepartFrom.IsZero() || query.DepartTo.IsZero() || query.DepartTo.Before(query.DepartFrom) {
		return ScheduleCandidatesResult{outcome: ScheduleCandidatesNotAccepted}, nil
	}
	undecided := scheduleCandidatesUndecided(query.TenantID, connection)

	configured, err := handler.deps.Source.ConnectionHasSchedules(ctx, query.TenantID, connection)
	if err != nil {
		return undecided, nil
	}
	if !configured {
		return ScheduleCandidatesResult{outcome: ScheduleCandidatesNotConfigured}, nil
	}
	templates, err := handler.deps.Source.ListCurrentTemplatesOnConnection(ctx, query.TenantID, connection)
	if err != nil {
		return undecided, nil
	}
	registered, err := handler.deps.Source.ListCurrentSchedulesDepartingWithin(ctx, query.TenantID, connection,
		query.DepartFrom, query.DepartTo)
	if err != nil {
		return undecided, nil
	}

	byIdentity := map[string]domain.TransportSchedule{}
	for _, record := range registered {
		byIdentity[record.Schedule.Schedule().String()] = record.Schedule
	}
	var unregistered []domain.ScheduleReference
	for _, template := range templates {
		for _, schedule := range template.Template.MaterializeDepartingWithin(query.DepartFrom, query.DepartTo) {
			if _, taken := byIdentity[schedule.Schedule().String()]; taken {
				continue
			}
			byIdentity[schedule.Schedule().String()] = schedule
			unregistered = append(unregistered, schedule.Schedule())
		}
	}
	overrides, err := handler.deps.Source.FindCurrentSchedules(ctx, query.TenantID, unregistered)
	if err != nil {
		return undecided, nil
	}
	for _, record := range overrides {
		byIdentity[record.Schedule.Schedule().String()] = record.Schedule
	}

	candidates := make([]domain.TransportSchedule, 0, len(byIdentity))
	for _, schedule := range byIdentity {
		// 登记册上那一班改过连接或时刻、改出了这次问的范围，它就不再是候选——同一身份的物化结果也不能替它留下。
		if schedule.Direction().Connection() != connection ||
			schedule.DepartsAt().Before(query.DepartFrom) || schedule.DepartsAt().After(query.DepartTo) {
			continue
		}
		candidates = append(candidates, schedule)
	}
	slices.SortFunc(candidates, func(left, right domain.TransportSchedule) int {
		if order := left.DepartsAt().Compare(right.DepartsAt()); order != 0 {
			return order
		}
		return strings.Compare(left.Schedule().String(), right.Schedule().String())
	})
	return ScheduleCandidatesResult{outcome: ScheduleCandidatesListed, candidates: candidates}, nil
}

func scheduleCandidatesUndecided(tenant domain.TenantID, connection domain.NetworkConnectionReference) ScheduleCandidatesResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"SCHEDULE_CANDIDATE_SOURCE_UNAVAILABLE", tenant.String(), connection.String(),
	}, "\x00")))
	return ScheduleCandidatesResult{
		outcome:      ScheduleCandidatesUndecided,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
