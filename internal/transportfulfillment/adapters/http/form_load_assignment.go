package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 票 tf-segment-lifecycle-closure/07 的第三个写面：运输运营形成装载分配（UC-TF-003/004 运输准备）。
// 它是写面而不是内部触发：`PrepareTransportOpportunity.Consume` 收的是调用方给的分配引用、消耗后才把它交给
// 装载分配链——分配本体在消耗之前、由人定（票 03 简报第 8 行核过）。

// LoadAssignmentIntake 把已认证的运营写请求翻译成形成装载分配的命令。版本由调用方指名而不是这里铸。
type LoadAssignmentIntake interface {
	IntakeLoadAssignment(ctx context.Context, request *http.Request) (application.FormLoadAssignmentCommand, error)
}

// LoadAssigner 是本适配器转交的应用编排。
type LoadAssigner interface {
	Form(
		ctx context.Context,
		command application.FormLoadAssignmentCommand,
	) (application.FormLoadAssignmentResult, error)
}

// NewFormLoadAssignmentEndpoint 交回形成装载分配的 HTTP 入口。
func NewFormLoadAssignmentEndpoint(intake LoadAssignmentIntake, assigner LoadAssigner) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.FormLoadAssignmentResult, error, bool) {
		command, err := intake.IntakeLoadAssignment(request.Context(), request)
		if err != nil {
			return application.FormLoadAssignmentResult{}, err, false
		}
		result, err := assigner.Form(request.Context(), command)
		return result, err, true
	}, writeLoadAssignmentOutcome)
}

// loadAssignmentResponse 是装载分配口的封闭响应形状。**没有任何「已装载」字段**：分配是执行意图不是已经
// 发生的装载，实际装载、短装、多装或错装是与分配版本比较的独立事实（移动事实那一口）。
//
// withdrawn 与 withdrawnAt 两键成对：回显的版本是撤回版时一起在场，否则一起缺席。`分配已形成`回显的是链头，
// 链头可能是撤回版；缺了这两键，调用方会把已撤回的分配读成仍在生效、转去变化入口，而撤回之后只能另起分配。
// 非撤回版整键缺席而不答 false 或空串：本包的可空格一律以缺席表「未发生」（optionalInstant 的头注、
// transportHandoverBody 的 corrects 两键），这里若另用 false 占位，调用方就得为同一种「没有」认两种写法。
//
// 容量保障各格（ADR-0203 决定一、二）带的是判出这一格所对的东西，同样以缺席表「这一格不涉及」：`allocations` 是回显那一版
// 逐池的分配量；`poolsWithoutReservation` 只随`须引用预占`，是缺行的池；`allocationsNotOnSchedule` 只随`预占不在本班次`，
// 是指错了的行；`refusedAllocation`、`pool`、`reservation` 随逐行判出的那几格，是挡下的那一行、它所在的池与那一份预占；
// `availableQuantity` 只随`分配超出`，是那一份预占此刻还能分给这次分配的量，单位同 `pool.unit`。
type loadAssignmentResponse struct {
	Outcome                  string                   `json:"outcome"`
	Assignment               string                   `json:"assignment,omitempty"`
	Schedule                 string                   `json:"schedule,omitempty"`
	Members                  []string                 `json:"members,omitempty"`
	Version                  string                   `json:"version,omitempty"`
	AssignedAt               string                   `json:"assignedAt,omitempty"`
	Allocations              []loadAllocationBody     `json:"allocations,omitempty"`
	Withdrawn                bool                     `json:"withdrawn,omitempty"`
	WithdrawnAt              string                   `json:"withdrawnAt,omitempty"`
	ContinuationReference    string                   `json:"continuationReference,omitempty"`
	PoolsWithoutReservation  []registeredPoolBody     `json:"poolsWithoutReservation,omitempty"`
	AllocationsNotOnSchedule []loadAllocationBody     `json:"allocationsNotOnSchedule,omitempty"`
	RefusedAllocation        *loadAllocationBody      `json:"refusedAllocation,omitempty"`
	Pool                     *registeredPoolBody      `json:"pool,omitempty"`
	Reservation              *capacityReservationBody `json:"reservation,omitempty"`
	AvailableQuantity        string                   `json:"availableQuantity,omitempty"`
}

// loadAllocationBody 逐字段透出一行分配量。`quantityBasis` 说这个数是声明的（`DECLARED`）还是取成员数（`MEMBER_COUNT`）。
type loadAllocationBody struct {
	Pool          string             `json:"pool"`
	Reservation   string             `json:"reservation"`
	Quantity      string             `json:"quantity"`
	QuantityBasis string             `json:"quantityBasis"`
	Unit          string             `json:"unit"`
	Source        capacitySourceBody `json:"source"`
}

func loadAllocation(allocation domain.CapacityAllocation) loadAllocationBody {
	demand := allocation.Demand()
	return loadAllocationBody{
		Pool:          allocation.Pool().String(),
		Reservation:   allocation.Reservation().String(),
		Quantity:      quantity(demand.Quantity()),
		QuantityBasis: allocation.Basis().String(),
		Unit:          demand.Unit().String(),
		Source:        capacitySource(demand.Source()),
	}
}

func writeLoadAssignmentOutcome(response http.ResponseWriter, result application.FormLoadAssignmentResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := loadAssignmentResponse{Outcome: outcome, ContinuationReference: result.ContinuationReference()}
	if record, present := result.Record(); present {
		body.Assignment = record.Assignment.Assignment().String()
		body.Schedule = record.Assignment.Schedule().String()
		for _, member := range record.Assignment.Members() {
			body.Members = append(body.Members, member.String())
		}
		body.Version = record.Assignment.Version().String()
		body.AssignedAt = rfc3339(record.Assignment.AssignedAt())
		for _, allocation := range record.Assignment.Allocations() {
			body.Allocations = append(body.Allocations, loadAllocation(allocation))
		}
		if withdrawnAt, withdrawn := record.Assignment.Withdrawn(); withdrawn {
			body.Withdrawn = true
			body.WithdrawnAt = rfc3339(withdrawnAt)
		}
	}
	for _, pool := range result.PoolsWithoutReservation() {
		body.PoolsWithoutReservation = append(body.PoolsWithoutReservation, registeredPool(pool))
	}
	for _, allocation := range result.AllocationsNotOnSchedule() {
		body.AllocationsNotOnSchedule = append(body.AllocationsNotOnSchedule, loadAllocation(allocation))
	}
	if allocation, pool, refused := result.RefusedAllocation(); refused {
		writeRefusedAllocation(&body, allocation, pool)
		if result.Outcome() == application.LoadAssignmentAllocationExceeded {
			body.AvailableQuantity = quantity(result.AvailableQuantity())
		}
	}
	// ADR-0022：只有新形成了一版用 201；同版本已在册、分配已形成、班次不在册、未受理、未决都是形成了的答案（200）。
	// 分配已形成回显的是链头，不是这次送来的那一份。
	status := http.StatusOK
	if result.Outcome() == application.LoadAssignmentFormed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

// writeRefusedAllocation 带上挡下的那一行、它所在的池与那一份预占。预占三量连同已分配照实透出，不代算余量。
func writeRefusedAllocation(body *loadAssignmentResponse, allocation domain.CapacityAllocation, record ports.CapacityPoolRecord) {
	refused := loadAllocation(allocation)
	body.RefusedAllocation = &refused
	pool := registeredPool(record)
	body.Pool = &pool
	if reservation, found := record.Pool.ReservationFor(allocation.Reservation()); found {
		held := capacityReservation(reservation, record.Pool.Unit())
		body.Reservation = &held
	}
}

// LoadAssignmentPayload 是装载分配登记口的线格式（票 operator-channel/15 定；此前本口只挂未配置、没有线格式）。
// 载荷只有内容、没有身份：租户从信封来，载荷带租户即拒（封闭解码）。字段照命令逐格取，时刻照 ADR-0023 由提交方给。
//
// `allocations` 可缺：班次没挂容量池时不要求（ADR-0203 决定一）。没有任何越过预占余量或池上限的键，带这类键的载荷是坏报文。
type LoadAssignmentPayload struct {
	Assignment  string                         `json:"assignment"`
	Schedule    string                         `json:"schedule"`
	Members     []string                       `json:"members"`
	Version     string                         `json:"version"`
	AssignedAt  string                         `json:"assignedAt"`
	Allocations []LoadAssignmentAllocationLine `json:"allocations"`
}

// LoadAssignmentAllocationLine 是一行分配量的线格式：哪个池上的哪一份预占、分多少、什么单位、依据什么来源。
// `quantityFromMembers` 为真时数量取分配的成员数，只许落在件数维的池上，`quantity` 此时不给；两者成不成立由编排答
// `未受理`，不在这里折成坏报文。来源的线格式与预占口同一个。
type LoadAssignmentAllocationLine struct {
	Pool                string                      `json:"pool"`
	Reservation         string                      `json:"reservation"`
	Quantity            int64                       `json:"quantity"`
	QuantityFromMembers bool                        `json:"quantityFromMembers"`
	Unit                string                      `json:"unit"`
	Source              CapacityDemandSourcePayload `json:"source"`
}

// Command 把载荷连同信封给的租户翻成装载分配命令。时刻解不出是坏报文（400）；分配成不成立由编排答。
func (payload LoadAssignmentPayload) Command(tenant domain.TenantID) (application.FormLoadAssignmentCommand, error) {
	if tenant.String() == "" {
		return application.FormLoadAssignmentCommand{}, ErrOperatorIdentityMissing
	}
	assignedAt, err := parseOptionalInstant("assignedAt", payload.AssignedAt)
	if err != nil {
		return application.FormLoadAssignmentCommand{}, err
	}
	allocations := make([]application.LoadAssignmentAllocationLine, 0, len(payload.Allocations))
	for _, line := range payload.Allocations {
		allocations = append(allocations, application.LoadAssignmentAllocationLine{
			Pool:            line.Pool,
			Reservation:     line.Reservation,
			Quantity:        line.Quantity,
			FromMembers:     line.QuantityFromMembers,
			Unit:            line.Unit,
			SourceKind:      line.Source.Kind,
			SourceReference: line.Source.Reference,
		})
	}
	return application.FormLoadAssignmentCommand{
		TenantID:    tenant,
		Assignment:  payload.Assignment,
		Schedule:    payload.Schedule,
		Members:     append([]string(nil), payload.Members...),
		Version:     payload.Version,
		AssignedAt:  assignedAt,
		Allocations: allocations,
	}, nil
}
