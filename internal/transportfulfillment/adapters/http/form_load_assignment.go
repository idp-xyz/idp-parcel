package tfhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
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
type loadAssignmentResponse struct {
	Outcome               string   `json:"outcome"`
	Assignment            string   `json:"assignment,omitempty"`
	Schedule              string   `json:"schedule,omitempty"`
	Members               []string `json:"members,omitempty"`
	Version               string   `json:"version,omitempty"`
	AssignedAt            string   `json:"assignedAt,omitempty"`
	ContinuationReference string   `json:"continuationReference,omitempty"`
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
		body.AssignedAt = record.Assignment.AssignedAt().UTC().Format(time.RFC3339)
	}
	// ADR-0022：只有新形成了一版用 201；同版本已在册、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.LoadAssignmentFormed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

// LoadAssignmentPayload 是装载分配登记口的线格式（票 operator-channel/15 定；此前本口只挂未配置、没有线格式）。
// 载荷只有内容、没有身份：租户从信封来，载荷带租户即拒（封闭解码）。字段照命令逐格取，时刻照 ADR-0023 由提交方给。
type LoadAssignmentPayload struct {
	Assignment string   `json:"assignment"`
	Schedule   string   `json:"schedule"`
	Members    []string `json:"members"`
	Version    string   `json:"version"`
	AssignedAt string   `json:"assignedAt"`
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
	return application.FormLoadAssignmentCommand{
		TenantID:   tenant,
		Assignment: payload.Assignment,
		Schedule:   payload.Schedule,
		Members:    append([]string(nil), payload.Members...),
		Version:    payload.Version,
		AssignedAt: assignedAt,
	}, nil
}
