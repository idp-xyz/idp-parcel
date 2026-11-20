package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ConcludeDiscrepancyOutcome 是对一份收货差异作结论的结果。库存账不在这里改。
type ConcludeDiscrepancyOutcome uint8

const (
	ConcludeDiscrepancyOutcomeInvalid ConcludeDiscrepancyOutcome = iota
	ConcludeDiscrepancyRecorded
	ConcludeDiscrepancyExistingResult
	ConcludeDiscrepancyHeld
	ConcludeDiscrepancyNotFound
	ConcludeDiscrepancyNotConfigured
	ConcludeDiscrepancySourceConflict
)

func (outcome ConcludeDiscrepancyOutcome) String() string {
	switch outcome {
	case ConcludeDiscrepancyRecorded:
		return "CONCLUSION_RECORDED"
	case ConcludeDiscrepancyExistingResult:
		return "EXISTING_RESULT"
	case ConcludeDiscrepancyHeld:
		return "CONCLUSION_HELD"
	case ConcludeDiscrepancyNotFound:
		return "DISCREPANCY_NOT_FOUND"
	case ConcludeDiscrepancyNotConfigured:
		return "NOT_CONFIGURED"
	case ConcludeDiscrepancySourceConflict:
		return "SOURCE_CONFLICT"
	case ConcludeDiscrepancyOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// ConcludeDiscrepancyResult 回显现行结论。查无、未配置与来源冲突不带结论：来源冲突时被指名的那一份不变。
type ConcludeDiscrepancyResult struct {
	outcome    ConcludeDiscrepancyOutcome
	current    domain.DiscrepancyConclusion
	hasCurrent bool
}

func (result ConcludeDiscrepancyResult) Outcome() ConcludeDiscrepancyOutcome { return result.outcome }

func (result ConcludeDiscrepancyResult) Current() (domain.DiscrepancyConclusion, bool) {
	return result.current, result.hasCurrent
}

func concludeBare(outcome ConcludeDiscrepancyOutcome) ConcludeDiscrepancyResult {
	return ConcludeDiscrepancyResult{outcome: outcome}
}

func concludeEcho(outcome ConcludeDiscrepancyOutcome, current domain.DiscrepancyConclusion) ConcludeDiscrepancyResult {
	return ConcludeDiscrepancyResult{outcome: outcome, current: current, hasCurrent: true}
}

// ConcludeDiscrepancyCommand 是一次结论。租户只从信封来。货主口的货主与来源请求键也只从信封来。
// 代录不带货主、不带决定标识：货主取自差异所属的预报，重放身份就是这份差异。依据引用必带。
type ConcludeDiscrepancyCommand struct {
	Tenant      domain.TenantID
	Discrepancy domain.ReceiptDiscrepancyID
	Verdict     domain.ConclusionVerdict
	Note        domain.ConclusionNote
	Path        domain.SubmissionPath
	Owner       domain.OwnerAccountID
	RequestKey  domain.RequestKey
	Operator    domain.Registrant
	Evidence    domain.EvidenceReference
}

func (command ConcludeDiscrepancyCommand) assemblyError() error {
	if command.Tenant.String() == "" || command.Discrepancy.String() == "" {
		return errors.New("缺租户或差异")
	}
	if command.Verdict != domain.ConclusionConfirm && command.Verdict != domain.ConclusionObject {
		return errors.New("缺结论")
	}
	switch command.Path {
	case domain.SubmissionByOwner:
		if command.Owner.String() == "" || command.RequestKey.String() == "" || command.Operator.String() != "" || command.Evidence.String() != "" {
			return errors.New("货主口的身份不齐")
		}
	case domain.SubmissionOnBehalf:
		if command.Owner.String() != "" || command.RequestKey.String() != "" || command.Operator.String() == "" || command.Evidence.String() == "" {
			return errors.New("代录口的身份不齐")
		}
	default:
		return errors.New("缺途径")
	}
	return nil
}

// ConcludeDiscrepancyHandler 把一条结论落在一份差异上。事务边界在调用方。
// 本租户一个仓都没登记时不查差异在不在。同一份差异上的两笔靠差异标识上的咨询锁串行。
type ConcludeDiscrepancyHandler struct {
	conclusions ports.DiscrepancyConclusionStore
	warehouses  ports.TenantWarehouseLookup
	clock       ports.Clock
}

func NewConcludeDiscrepancyHandler(
	conclusions ports.DiscrepancyConclusionStore,
	warehouses ports.TenantWarehouseLookup,
	clock ports.Clock,
) *ConcludeDiscrepancyHandler {
	return &ConcludeDiscrepancyHandler{conclusions: conclusions, warehouses: warehouses, clock: clock}
}

// Conclude 的判定次序：
//
//  1. 装配错：交 error。
//  2. 本租户一个仓都没登记：NOT_CONFIGURED，到此为止，不查差异在不在。
//  3. 锁这份差异。货主口再锁来源请求键，次序是先差异、后来源请求键。
//  4. 差异在这个货主名下查不到：DISCREPANCY_NOT_FOUND。代录的货主取自差异所属的预报；货主口对不上与查不到同答。
//  5. 已落册的来源请求键又来指名另一份差异：SOURCE_CONFLICT，这一份不变。
//  6. 还没有结论：货主本人或代录都记下，CONCLUSION_RECORDED。
//  7. 现行是代录的：货主本人的结论取代它，不论相同还是不同，被取代的照留。代录不取代：相同 EXISTING_RESULT，不同 CONCLUSION_HELD。
//  8. 现行是货主本人的：不再改。相同 EXISTING_RESULT，不同 CONCLUSION_HELD，并回显现行。
func (handler *ConcludeDiscrepancyHandler) Conclude(
	ctx context.Context,
	command ConcludeDiscrepancyCommand,
) (ConcludeDiscrepancyResult, error) {
	if err := command.assemblyError(); err != nil {
		return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
	}
	registered, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
	}
	if !registered {
		return concludeBare(ConcludeDiscrepancyNotConfigured), nil
	}
	if err := handler.conclusions.Hold(ctx, command.Tenant, command.Discrepancy); err != nil {
		return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
	}
	if command.Path == domain.SubmissionByOwner {
		if err := handler.conclusions.HoldRequestKey(ctx, command.Tenant, command.RequestKey); err != nil {
			return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
		}
	}
	subject, found, err := handler.conclusions.FindSubject(ctx, command.Tenant, command.Discrepancy)
	if err != nil {
		return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
	}
	if !found {
		return concludeBare(ConcludeDiscrepancyNotFound), nil
	}
	if command.Path == domain.SubmissionByOwner && subject.Owner != command.Owner {
		return concludeBare(ConcludeDiscrepancyNotFound), nil
	}
	if command.Path == domain.SubmissionByOwner {
		named, taken, err := handler.conclusions.FindRequestKey(ctx, command.Tenant, command.RequestKey)
		if err != nil {
			return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
		}
		if taken && named != command.Discrepancy {
			return concludeBare(ConcludeDiscrepancySourceConflict), nil
		}
	}
	rows, err := handler.conclusions.History(ctx, command.Tenant, command.Discrepancy)
	if err != nil {
		return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
	}
	current, hasCurrent, err := splitCurrent(rows)
	if err != nil {
		return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
	}
	record, supersede, outcome := judgeConclusion(command, current, hasCurrent)
	if !record {
		return concludeEcho(outcome, current), nil
	}
	built, err := domain.NewDiscrepancyConclusion(
		command.Tenant, command.Discrepancy, command.Verdict, command.Note, command.Path,
		command.RequestKey, command.Operator, command.Evidence, handler.clock.Now(), true,
	)
	if err != nil {
		return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
	}
	if err := handler.conclusions.Record(ctx, built, supersede); err != nil {
		return ConcludeDiscrepancyResult{}, fmt.Errorf("conclude discrepancy: %w", err)
	}
	return concludeEcho(outcome, built), nil
}

func judgeConclusion(
	command ConcludeDiscrepancyCommand,
	current domain.DiscrepancyConclusion,
	hasCurrent bool,
) (record bool, supersede bool, outcome ConcludeDiscrepancyOutcome) {
	if !hasCurrent {
		return true, false, ConcludeDiscrepancyRecorded
	}
	if current.Path() == domain.SubmissionOnBehalf && command.Path == domain.SubmissionByOwner {
		return true, true, ConcludeDiscrepancyRecorded
	}
	if current.Verdict() == command.Verdict {
		return false, false, ConcludeDiscrepancyExistingResult
	}
	return false, false, ConcludeDiscrepancyHeld
}

func splitCurrent(rows []domain.DiscrepancyConclusion) (domain.DiscrepancyConclusion, bool, error) {
	var current domain.DiscrepancyConclusion
	has := false
	for _, row := range rows {
		if !row.Current() {
			continue
		}
		if has {
			return domain.DiscrepancyConclusion{}, false, errors.New("一份差异的现行结论不唯一")
		}
		current = row
		has = true
	}
	if len(rows) > 0 && !has {
		return domain.DiscrepancyConclusion{}, false, errors.New("结论都在，却没有现行的那一条")
	}
	return current, has, nil
}

// DiscrepancyConclusionView 是一份差异在读口上的结论：现行的那一条，以及被取代仍留下的代录结论。
// 没有现行结论时 Current 为假，读口答待货主确认。
type DiscrepancyConclusionView struct {
	id         domain.ReceiptDiscrepancyID
	current    domain.DiscrepancyConclusion
	hasCurrent bool
	superseded []domain.DiscrepancyConclusion
}

func (view DiscrepancyConclusionView) ID() domain.ReceiptDiscrepancyID { return view.id }

func (view DiscrepancyConclusionView) Current() (domain.DiscrepancyConclusion, bool) {
	return view.current, view.hasCurrent
}

func (view DiscrepancyConclusionView) Superseded() []domain.DiscrepancyConclusion {
	out := make([]domain.DiscrepancyConclusion, len(view.superseded))
	copy(out, view.superseded)
	return out
}

// GroupDiscrepancyConclusions 按差异标识把落下的行分成现行与被取代。被问到的标识即使没有行也占一格，读口才答得出待货主确认。
func GroupDiscrepancyConclusions(
	ids []domain.ReceiptDiscrepancyID,
	rows []domain.DiscrepancyConclusion,
) ([]DiscrepancyConclusionView, error) {
	grouped := map[string][]domain.DiscrepancyConclusion{}
	for _, row := range rows {
		grouped[row.Discrepancy().String()] = append(grouped[row.Discrepancy().String()], row)
	}
	views := make([]DiscrepancyConclusionView, 0, len(ids))
	for _, id := range ids {
		current, has, err := splitCurrent(grouped[id.String()])
		if err != nil {
			return nil, err
		}
		var superseded []domain.DiscrepancyConclusion
		for _, row := range grouped[id.String()] {
			if !row.Current() {
				superseded = append(superseded, row)
			}
		}
		views = append(views, DiscrepancyConclusionView{
			id: id, current: current, hasCurrent: has, superseded: superseded,
		})
	}
	return views, nil
}
