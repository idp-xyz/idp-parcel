package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// PickupAttemptIntake 把已认证的接入请求翻译成一次到访多对象的揽收执行命令。
//
// 段引用在这里是**两层**：整次到访一个 `Segment`（同一次到访取得控制的对象进同一个共同控制
// 范围），逐对象各自的 `PlannedSegment`（CONTEXT 要求每个对象分别关联自己的计划履约段）。
// Intake 两层都要收——收了整次那一格而漏掉逐对象那一格，成员差异就在端点层被抹平了。
type PickupAttemptIntake interface {
	IntakePickupAttempt(ctx context.Context, request *http.Request) (application.PerformOffsitePickupCommand, error)
}

// PickupAttemptHandler 是本适配器转交的应用编排。
type PickupAttemptHandler interface {
	Handle(
		ctx context.Context,
		command application.PerformOffsitePickupCommand,
	) (application.PerformOffsitePickupResult, error)
}

// NewPerformOffsitePickupEndpoint 交回一次到访多对象揽收执行的 HTTP 入口。与单对象登记口同组
// 不同口：命令类型互不相同，合成一口就得在 Intake 里先认形状再分路。
func NewPerformOffsitePickupEndpoint(intake PickupAttemptIntake, handler PickupAttemptHandler) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.PerformOffsitePickupResult, error, bool) {
		command, err := intake.IntakePickupAttempt(request.Context(), request)
		if err != nil {
			return application.PerformOffsitePickupResult{}, err, false
		}
		result, err := handler.Handle(request.Context(), command)
		return result, err, true
	}, writePickupAttemptOutcome)
}

// pickupAttemptResponse 是多对象口的封闭响应形状。
//
// `objects` 逐对象透出成败与版本：UC-TF-002 要求任务汇总只能由对象结果派生，传输层不替它
// 汇总成一个「本次到访成功/失败」。`segmentEntries` 逐对象列进段那一半的欠账，只列真有欠账
// 的对象——进去了的、没要求进的、领域正当拒绝的都不在列，与编排交回的形状一致。
type pickupAttemptResponse struct {
	Outcome               string                      `json:"outcome"`
	UndecidedReason       string                      `json:"undecidedReason,omitempty"`
	Attempt               string                      `json:"attempt,omitempty"`
	Task                  string                      `json:"task,omitempty"`
	Objects               []pickupAttemptObjectResult `json:"objects,omitempty"`
	ContinuationReference string                      `json:"continuationReference,omitempty"`
	HandoffReference      string                      `json:"handoffReference,omitempty"`
	SegmentEntries        []segmentEntryResponse      `json:"segmentEntries,omitempty"`
}

type pickupAttemptObjectResult struct {
	Object        string `json:"object"`
	Outcome       string `json:"outcome"`
	Basis         string `json:"basis,omitempty"`
	PickupVersion string `json:"pickupVersion,omitempty"`
}

type segmentEntryResponse struct {
	Object                string `json:"object"`
	ContinuationReference string `json:"continuationReference"`
}

func writePickupAttemptOutcome(response http.ResponseWriter, result application.PerformOffsitePickupResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := pickupAttemptResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
		HandoffReference:      result.PickupHandoffReference(),
	}
	if record, present := result.Record(); present {
		body.Attempt = record.Attempt.Attempt().String()
		body.Task = record.Attempt.Task().String()
		versions := map[string]string{}
		for _, pickup := range record.Pickups {
			versions[pickup.Object().String()] = pickup.Version().String()
		}
		for _, objectResult := range record.Results {
			entry := pickupAttemptObjectResult{
				Object:        objectResult.Object().String(),
				Outcome:       objectResult.Outcome().String(),
				PickupVersion: versions[objectResult.Object().String()],
			}
			if basis, present := objectResult.Basis(); present {
				entry.Basis = basis.String()
			}
			body.Objects = append(body.Objects, entry)
		}
	}
	for _, entry := range result.SegmentEntries() {
		body.SegmentEntries = append(body.SegmentEntries, segmentEntryResponse{
			Object:                entry.Object.String(),
			ContinuationReference: entry.ContinuationReference,
		})
	}

	// ADR-0022：只有这次到访新落了结果用 201；重放、冲突、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.PickupAttemptRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
