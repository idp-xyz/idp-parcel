package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// MovementFacts 实现 ports.MovementFactRegistry：一个版本一行，出发的门禁读数在同一行（ADR-0205）。
//
// **只插不改。** 移动是已经发生的事实——它可以被更正（新版本回指前身），但不可回写；本类型上
// 没有任何 UPDATE。读数随事实那一笔 INSERT 落下，此后同样不改：它记的是登记那一刻关务怎么答。
type MovementFacts struct {
	db *bentopg.DB
}

func NewMovementFacts(db *bentopg.DB) (*MovementFacts, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &MovementFacts{db: db}, nil
}

var _ ports.MovementFactRegistry = (*MovementFacts)(nil)

// FindByKey 按（租户+事实+版本）取回一个版本。否定结果只回 false。
func (repository *MovementFacts) FindByKey(
	ctx context.Context,
	key ports.MovementFactKey,
) (ports.MovementFactRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}

	var scheduleRef, kindText, locationRef, sourceRef string
	var occurredAt, recordedAt time.Time
	var segmentRef, resourceRef, gateClearance, gateReading, correctsVersion *string
	var gateReadingDetail []byte
	var correctedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT schedule_ref, kind, location_ref, source_ref, occurred_at,
		        segment_ref, resource_ref, gate_clearance, gate_reading, gate_reading_detail,
		        corrects_version, corrected_at, recorded_at
		   FROM transport_fulfillment.transport_movement_fact
		  WHERE tenant_id = $1 AND fact_ref = $2 AND version = $3`,
		key.TenantID.String(), key.Fact.String(), key.Version.String(),
	).Scan(&scheduleRef, &kindText, &locationRef, &sourceRef, &occurredAt,
		&segmentRef, &resourceRef, &gateClearance, &gateReading, &gateReadingDetail,
		&correctsVersion, &correctedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.MovementFactRecord{}, false, nil
	}
	if err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}

	spec := domain.RehydrateMovementFactSpec{
		TenantID:   key.TenantID,
		Fact:       key.Fact,
		Version:    key.Version,
		OccurredAt: occurredAt.UTC(),
	}
	// 逐列走各自的构造门装回，不按列直接拼结构体。
	if spec.Kind, err = movementFactKindFrom(kindText); err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	if spec.Schedule, err = domain.NewScheduleReference(scheduleRef); err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	if spec.Location, err = domain.NewMovementLocationReference(locationRef); err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	if spec.Source, err = domain.NewMovementSourceReference(sourceRef); err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	if segmentRef != nil {
		if spec.Segment, err = domain.NewFulfillmentSegmentReference(*segmentRef); err != nil {
			return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
		}
	}
	if resourceRef != nil {
		if spec.Resource, err = domain.NewTransportResourceReference(*resourceRef); err != nil {
			return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
		}
	}
	if gateClearance != nil {
		if spec.GateClearance, err = domain.NewGateClearanceReference(*gateClearance); err != nil {
			return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
		}
	}
	if gateReading != nil {
		if spec.GateReading, err = departureGateReadingFrom(*gateReading, gateReadingDetail); err != nil {
			return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
		}
	}
	if correctsVersion != nil {
		if spec.Corrects, err = domain.NewMovementFactVersion(*correctsVersion); err != nil {
			return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
		}
	}
	if correctedAt != nil {
		spec.CorrectedAt = correctedAt.UTC()
	}

	fact, err := domain.RehydrateMovementFact(spec)
	if err != nil {
		return ports.MovementFactRecord{}, false, fmt.Errorf("find movement fact: %w", err)
	}
	return ports.MovementFactRecord{Key: key, Fact: fact, RecordedAt: recordedAt.UTC()}, true, nil
}

// Save 登记一个版本。撞键答`已登记`（ADR-0031），编排据此读回原版本。
func (repository *MovementFacts) Save(
	ctx context.Context,
	record ports.MovementFactRecord,
) (ports.MovementFactSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.MovementFactSaveOutcomeInvalid, fmt.Errorf("save movement fact: %w", err)
	}
	if err := assertMovementFactKeyAgrees(record); err != nil {
		return ports.MovementFactSaveOutcomeInvalid, fmt.Errorf("save movement fact: %w", err)
	}

	var segmentText, resourceText, clearanceText, readingText, correctsText *string
	var readingDetail []byte
	if segment, declared := record.Fact.Segment(); declared {
		text := segment.String()
		segmentText = &text
	}
	if resource, declared := record.Fact.Resource(); declared {
		text := resource.String()
		resourceText = &text
	}
	if clearance, presented := record.Fact.GateClearance(); presented {
		text := clearance.String()
		clearanceText = &text
	}
	if reading, read := record.Fact.GateReading(); read {
		text := reading.Answer().String()
		readingText = &text
		if readingDetail, err = json.Marshal(departureGateReadingDetailOf(reading)); err != nil {
			return ports.MovementFactSaveOutcomeInvalid, fmt.Errorf("save movement fact: gate reading detail: %w", err)
		}
	}
	if corrects, has := record.Fact.Corrects(); has {
		text := corrects.String()
		correctsText = &text
	}
	correctedAt, corrected := record.Fact.CorrectedAt()
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_movement_fact
		     (tenant_id, fact_ref, version, schedule_ref, kind, location_ref, source_ref,
		      occurred_at, segment_ref, resource_ref, gate_clearance, gate_reading, gate_reading_detail,
		      corrects_version, corrected_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Fact.String(),
		record.Key.Version.String(),
		record.Fact.Schedule().String(),
		record.Fact.Kind().String(),
		record.Fact.Location().String(),
		record.Fact.Source().String(),
		record.Fact.OccurredAt().UTC(),
		segmentText,
		resourceText,
		clearanceText,
		readingText,
		readingDetail,
		correctsText,
		nullableTime(correctedAt, corrected),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.MovementFactSaveOutcomeInvalid, fmt.Errorf("save movement fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.MovementFactVersionAlreadyRegistered, nil
	}
	return ports.MovementFactSaved, nil
}

// assertMovementFactKeyAgrees 挡住「键说的是一个版本、聚合说的是另一个」那种写入。
func assertMovementFactKeyAgrees(record ports.MovementFactRecord) error {
	if record.Key.TenantID != record.Fact.TenantID() ||
		record.Key.Fact != record.Fact.Fact() ||
		record.Key.Version != record.Fact.Version() {
		return fmt.Errorf("movement fact key disagrees with the aggregate")
	}
	return nil
}

func movementFactKindFrom(raw string) (domain.MovementFactKind, error) {
	switch raw {
	case domain.DepartureFact.String():
		return domain.DepartureFact, nil
	case domain.InTransitFact.String():
		return domain.InTransitFact, nil
	case domain.ArrivalFact.String():
		return domain.ArrivalFact, nil
	default:
		return domain.MovementFactKindInvalid, fmt.Errorf("unknown movement fact kind %q", raw)
	}
}

// departureGateReadingDetail 是 gate_reading_detail 列的形状。词都取领域枚举自己的 String()，读回时按封闭集认，认不出的
// 行不装回——那是库面被绕过构造门写过，不是一格可读的答复。
type departureGateReadingDetail struct {
	From     *string                      `json:"from"`
	To       *string                      `json:"to"`
	Stops    []string                     `json:"stops"`
	Findings []departureGateFindingDetail `json:"findings"`
}

type departureGateFindingDetail struct {
	Object    string                     `json:"object"`
	Answer    string                     `json:"answer"`
	Judgement *string                    `json:"judgement"`
	Bases     []departureGateBasisDetail `json:"bases"`
}

type departureGateBasisDetail struct {
	Action  string   `json:"action,omitempty"`
	Cause   string   `json:"cause"`
	Sources []string `json:"sources"`
}

func departureGateReadingDetailOf(reading domain.DepartureGateReading) departureGateReadingDetail {
	detail := departureGateReadingDetail{Stops: []string{}, Findings: []departureGateFindingDetail{}}
	if from, to, directed := reading.Direction(); directed {
		fromText, toText := from.String(), to.String()
		detail.From, detail.To = &fromText, &toText
	}
	for _, stop := range reading.Stops() {
		detail.Stops = append(detail.Stops, stop.String())
	}
	for _, finding := range reading.Findings() {
		detail.Findings = append(detail.Findings, gateFindingDetailOf(finding))
	}
	return detail
}

// gateFindingDetailOf 是逐对象那一格在 jsonb 里的形状，出发与交出两种读数共用（交出见 transport_handover_gate_reading.go）。
func gateFindingDetailOf(finding domain.GateFinding) departureGateFindingDetail {
	row := departureGateFindingDetail{Object: finding.Object().String(), Answer: finding.Answer().String(), Bases: []departureGateBasisDetail{}}
	if judgement, judged := finding.Judgement(); judged {
		row.Judgement = &judgement
	}
	for _, basis := range finding.Bases() {
		sources := basis.Sources
		if sources == nil {
			sources = []string{}
		}
		row.Bases = append(row.Bases, departureGateBasisDetail{Action: basis.Action, Cause: basis.Cause, Sources: sources})
	}
	return row
}

// gateFindingFrom 是 gateFindingDetailOf 的反方向，过同一道构造门。
func gateFindingFrom(row departureGateFindingDetail) (domain.GateFinding, error) {
	object, err := domain.NewCarriedObjectReference(row.Object)
	if err != nil {
		return domain.GateFinding{}, fmt.Errorf("gate reading detail: %w", err)
	}
	answer, err := gateAnswerFrom(row.Answer)
	if err != nil {
		return domain.GateFinding{}, err
	}
	judgement := ""
	if row.Judgement != nil {
		judgement = *row.Judgement
	}
	bases := make([]domain.GateBasis, 0, len(row.Bases))
	for _, basis := range row.Bases {
		bases = append(bases, domain.GateBasis{Action: basis.Action, Cause: basis.Cause, Sources: basis.Sources})
	}
	finding, err := domain.NewGateFinding(object, answer, judgement, bases)
	if err != nil {
		return domain.GateFinding{}, fmt.Errorf("gate reading detail: %w", err)
	}
	return finding, nil
}

func departureGateReadingFrom(answerText string, raw []byte) (domain.DepartureGateReading, error) {
	none := domain.DepartureGateReading{}
	answer, err := gateAnswerFrom(answerText)
	if err != nil {
		return none, err
	}
	var detail departureGateReadingDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return none, fmt.Errorf("gate reading detail: %w", err)
	}
	var spec domain.DepartureGateReadingSpec
	if (detail.From == nil) != (detail.To == nil) {
		return none, fmt.Errorf("gate reading detail: half a direction")
	}
	if detail.From != nil {
		if spec.From, err = domain.NewNetworkNodeReference(*detail.From); err != nil {
			return none, fmt.Errorf("gate reading detail: %w", err)
		}
		if spec.To, err = domain.NewNetworkNodeReference(*detail.To); err != nil {
			return none, fmt.Errorf("gate reading detail: %w", err)
		}
	}
	for _, word := range detail.Stops {
		stop, err := departureGateStopFrom(word)
		if err != nil {
			return none, err
		}
		spec.Stops = append(spec.Stops, stop)
	}
	for _, row := range detail.Findings {
		finding, err := gateFindingFrom(row)
		if err != nil {
			return none, err
		}
		spec.Findings = append(spec.Findings, finding)
	}
	return domain.RehydrateDepartureGateReading(spec, answer)
}

// gateAnswerFrom 是门禁读数五格的名称镜像，出发与交出共用，词取各常量自己的 String()。
func gateAnswerFrom(raw string) (domain.GateAnswer, error) {
	for _, answer := range []domain.GateAnswer{
		domain.GateNotGuarded, domain.GateAdmitted, domain.GateUndecided, domain.GateUnreadable, domain.GateBlocked,
	} {
		if answer.String() == raw {
			return answer, nil
		}
	}
	return domain.GateAnswerInvalid, fmt.Errorf("unknown gate answer %q", raw)
}

func departureGateStopFrom(raw string) (domain.DepartureGateStop, error) {
	for _, stop := range []domain.DepartureGateStop{
		domain.DepartureSegmentNotDeclared, domain.DepartureSegmentNotRegistered, domain.DepartureNoObjectUnderControl,
		domain.DepartureScheduleNotRegistered, domain.DepartureRecordedBeforeGateReading,
		domain.DepartureSegmentRegistryUnavailable, domain.DepartureScheduleRegistryUnavailable,
	} {
		if stop.String() == raw {
			return stop, nil
		}
	}
	return domain.DepartureGateStopInvalid, fmt.Errorf("unknown departure gate stop %q", raw)
}
