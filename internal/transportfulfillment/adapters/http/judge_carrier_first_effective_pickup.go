package tfhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 实际承运商首次有效收寄显式判断的在线写面（label-channel/31；ADR-0135 决定八；写面形状按 ADR-0085 两阶段接线）。
//
// 这是 ADR-0135 决定四「读法的两种来源」里第一种——判断方就一条证据显式给出读法——的管理台入口；规则那一路随
// 第一家真源另立。一次判断说的是一条证据的几件事，逐条判不批量导入，因此是**逐字段表单**（同有效时间判断）。
// 载荷只有内容没有身份：租户从操作者信封来，由 Intake 作为入参交进翻译。
//
// 面上不解释状态词、不建议「这条算不算收寄」——那正是判断方要给的读法；本文件只转交与映射。

// CarrierPickupJudgmentIntake 把已认证的接入请求翻译成判断命令。接口而非本包内解析代码的理由同
// EffectiveTimeJudgmentIntake：租户身份只能来自认证结果（ADR-0003），操作者渠道就位前装配点挂 UnconfiguredIntake。
type CarrierPickupJudgmentIntake interface {
	IntakeCarrierPickupJudgment(ctx context.Context, request *http.Request) (application.JudgeCarrierFirstEffectivePickupCommand, error)
}

// CarrierPickupJudge 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type CarrierPickupJudge interface {
	Judge(ctx context.Context, command application.JudgeCarrierFirstEffectivePickupCommand) (application.JudgeCarrierFirstEffectivePickupResult, error)
}

// CarrierPickupJudgmentPayload 是载荷的线格式。
//
// source 取 CarrierEvidenceSource 的封闭词；evidenceVersion 必备——依据是「F 的第 v1 代」；expressesControl 是读法本体；
// occurredAt（RFC 3339）只在来源不是外部承运轨迹事实时有意义（轨迹事实的业务时间由编排读回它已判断的有效时间）；
// carrierKind / carrierReference 与 carrierName 恰给其一；correctsSourceVersion 指名这条证据更正了同一来源的哪一代；
// segment / plannedSegment / segmentServiceAction 与收寄、交接登记同形。
type CarrierPickupJudgmentPayload struct {
	Object                string `json:"object"`
	Source                string `json:"source"`
	EvidenceReference     string `json:"evidenceReference"`
	EvidenceVersion       string `json:"evidenceVersion"`
	ExpressesControl      bool   `json:"expressesControl"`
	OccurredAt            string `json:"occurredAt,omitempty"`
	CarrierKind           string `json:"carrierKind,omitempty"`
	CarrierReference      string `json:"carrierReference,omitempty"`
	CarrierName           string `json:"carrierName,omitempty"`
	CorrectsSourceVersion string `json:"correctsSourceVersion,omitempty"`
	Segment               string `json:"segment,omitempty"`
	PlannedSegment        string `json:"plannedSegment,omitempty"`
	SegmentServiceAction  string `json:"segmentServiceAction,omitempty"`
}

// DecodeCarrierPickupJudgmentPayload 只做结构解码：JSON 合法、键都认识。严格解码不让自报身份有地方落——
// 载荷里出现 tenant 之类的键一律按未知键拒。
func DecodeCarrierPickupJudgmentPayload(body io.Reader) (CarrierPickupJudgmentPayload, error) {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var payload CarrierPickupJudgmentPayload
	if err := decoder.Decode(&payload); err != nil {
		return CarrierPickupJudgmentPayload{}, fmt.Errorf("%w: carrier pickup judgment payload: %v", ErrMalformedRequest, err)
	}
	return payload, nil
}

// Command 把载荷连同信封给的租户翻成判断命令。来源词与承运主体分支词不在封闭集内、时刻解不出，是坏报文（400）；
// 其余成形与否留给编排答`未受理`（200，形成了的业务答案）——两者的续办不同，不折成一格。
func (payload CarrierPickupJudgmentPayload) Command(tenant domain.TenantID) (application.JudgeCarrierFirstEffectivePickupCommand, error) {
	if tenant.String() == "" {
		return application.JudgeCarrierFirstEffectivePickupCommand{}, ErrOperatorIdentityMissing
	}
	source, err := domain.ParseCarrierEvidenceSource(strings.TrimSpace(payload.Source))
	if err != nil {
		return application.JudgeCarrierFirstEffectivePickupCommand{}, fmt.Errorf("%w: source is not a carrier evidence source word: %v", ErrMalformedRequest, err)
	}
	command := application.JudgeCarrierFirstEffectivePickupCommand{
		TenantID:              tenant,
		Object:                payload.Object,
		Source:                source,
		EvidenceReference:     payload.EvidenceReference,
		EvidenceVersion:       payload.EvidenceVersion,
		CorrectsSourceVersion: payload.CorrectsSourceVersion,
		ExpressesControl:      payload.ExpressesControl,
		SubjectReference:      payload.CarrierReference,
		NameMaterial:          payload.CarrierName,
		Segment:               payload.Segment,
		PlannedSegment:        payload.PlannedSegment,
		SegmentServiceAction:  payload.SegmentServiceAction,
	}
	if raw := strings.TrimSpace(payload.CarrierKind); raw != "" {
		kind, err := domain.ParseCarrierSubjectKind(raw)
		if err != nil {
			return application.JudgeCarrierFirstEffectivePickupCommand{}, fmt.Errorf("%w: carrierKind is not a carrier subject kind word: %v", ErrMalformedRequest, err)
		}
		command.SubjectKind = kind
	}
	if raw := strings.TrimSpace(payload.OccurredAt); raw != "" {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return application.JudgeCarrierFirstEffectivePickupCommand{}, fmt.Errorf("%w: occurredAt is not an RFC 3339 instant: %v", ErrMalformedRequest, err)
		}
		command.OccurredAt = at.UTC()
	}
	return command, nil
}

// NewJudgeCarrierFirstEffectivePickupEndpoint 交回显式判断的 HTTP 入口。分格由编排作答，这里只映射。
func NewJudgeCarrierFirstEffectivePickupEndpoint(intake CarrierPickupJudgmentIntake, judge CarrierPickupJudge) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.JudgeCarrierFirstEffectivePickupResult, error, bool) {
		command, err := intake.IntakeCarrierPickupJudgment(request.Context(), request)
		if err != nil {
			return application.JudgeCarrierFirstEffectivePickupResult{}, err, false
		}
		result, err := judge.Judge(request.Context(), command)
		return result, err, true
	}, writeCarrierPickupJudgmentOutcome)
}

// carrierPickupJudgmentResponse 是封闭响应形状。`outcome` 取应用结果枚举原名；带记录的答案把链尾那一版原样透出；
// 三个续办引用各归各键（意图 / 段 / 判断），段的正当拒绝另一键。
type carrierPickupJudgmentResponse struct {
	Outcome                              string             `json:"outcome"`
	UndecidedReason                      string             `json:"undecidedReason,omitempty"`
	ContinuationReference                string             `json:"continuationReference,omitempty"`
	HandoffReference                     string             `json:"handoffReference,omitempty"`
	SegmentContinuationReference         string             `json:"segmentContinuationReference,omitempty"`
	SegmentEntryRefusal                  string             `json:"segmentEntryRefusal,omitempty"`
	CarrierJudgmentContinuationReference string             `json:"carrierJudgmentContinuationReference,omitempty"`
	Pickup                               *carrierPickupBody `json:"pickup,omitempty"`
}

// carrierPickupBody 逐字段透出一版。已形成带承运主体两键与业务发生时间；待确认带原因与名称素材；失效三者都不带；
// supersedes 首登缺席。judgedAt 与 recordedAt 是另外两个时间，归属不同（ADR-0135 决定三）。时间一律 RFC 3339 UTC。
type carrierPickupBody struct {
	Fact             string                   `json:"fact"`
	Version          string                   `json:"version"`
	Object           string                   `json:"object"`
	Result           string                   `json:"result"`
	CarrierKind      string                   `json:"carrierKind,omitempty"`
	CarrierReference string                   `json:"carrierReference,omitempty"`
	OccurredAt       string                   `json:"occurredAt,omitempty"`
	PendingReason    string                   `json:"pendingReason,omitempty"`
	CarrierName      string                   `json:"carrierName,omitempty"`
	JudgedAt         string                   `json:"judgedAt"`
	Supersedes       string                   `json:"supersedes,omitempty"`
	Bases            []carrierPickupBasisBody `json:"bases"`
	RecordedAt       string                   `json:"recordedAt"`
}

type carrierPickupBasisBody struct {
	Source            string `json:"source"`
	EvidenceReference string `json:"evidenceReference"`
	SourceVersion     string `json:"sourceVersion"`
}

func carrierPickupBodyOf(record ports.CarrierFirstEffectivePickupRecord) carrierPickupBody {
	pickup := record.Pickup
	body := carrierPickupBody{
		Fact:       pickup.Fact().String(),
		Version:    pickup.Version().String(),
		Object:     pickup.Object().String(),
		Result:     pickup.Result().String(),
		JudgedAt:   pickup.JudgedAt().UTC().Format(time.RFC3339),
		RecordedAt: record.RecordedAt.UTC().Format(time.RFC3339),
		Bases:      make([]carrierPickupBasisBody, 0, len(pickup.Bases())),
	}
	if carrier, identified := pickup.Carrier(); identified {
		body.CarrierKind, body.CarrierReference = carrier.Kind().String(), carrier.Reference()
	}
	if at, present := pickup.OccurredAt(); present {
		body.OccurredAt = at.UTC().Format(time.RFC3339)
	}
	if reason, pending := pickup.PendingReason(); pending {
		body.PendingReason, body.CarrierName = reason.String(), pickup.Material()
	}
	if prior, has := pickup.Supersedes(); has {
		body.Supersedes = prior.String()
	}
	for _, basis := range pickup.Bases() {
		body.Bases = append(body.Bases, carrierPickupBasisBody{
			Source: basis.Source().String(), EvidenceReference: basis.Reference().String(), SourceVersion: basis.SourceVersion(),
		})
	}
	return body
}

func writeCarrierPickupJudgmentOutcome(response http.ResponseWriter, result application.JudgeCarrierFirstEffectivePickupResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := carrierPickupJudgmentResponse{
		Outcome:                              outcome,
		UndecidedReason:                      result.UndecidedReason().String(),
		ContinuationReference:                result.ContinuationReference(),
		HandoffReference:                     result.HandoffReference(),
		SegmentContinuationReference:         result.SegmentContinuationReference(),
		SegmentEntryRefusal:                  result.SegmentEntryRefusal().String(),
		CarrierJudgmentContinuationReference: result.CarrierJudgmentContinuationReference(),
	}
	if record, present := result.Record(); present {
		pickup := carrierPickupBodyOf(record)
		body.Pickup = &pickup
	}
	// ADR-0022：状态码只报有没有新落一版收寄。已形成 / 替代 / 失效是新落的收寄版本用 201；待确认、不构成、非首次、
	// 依据不可用、已在册、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	switch result.Outcome() {
	case application.CarrierPickupFormedOutcome, application.CarrierPickupSupersededOutcome, application.CarrierPickupVoidedOutcome:
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
