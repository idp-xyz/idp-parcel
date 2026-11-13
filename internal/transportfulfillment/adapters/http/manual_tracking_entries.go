package tfhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件是承运商轨迹的手工录入口（dox/decisions.md「产品形态」）：没有对接承运商或轨迹聚合平台的租户，由操作员照承运商
// 给的材料录入，一次一条或一批。每一条进的是轨迹源适配器那一路后面的同一只认领编排（AdoptTrackingMaterialHandler）：
// 凭证解析、更正、有效时间规则一条不少；原始状态词原样引用、不解释。录入人与所凭原件另记一本留痕。

// ManualTrackingEntryLimit 是一次请求最多带几条。再多就分几次导：一批各条各自成一笔事务，一次请求占着连接逐条做。
const ManualTrackingEntryLimit = 200

// ManualTrackingSourcePrefix 是人工录入的素材在源事件标识上的前缀，理由同海关结果的手工录入：操作员多半拿承运商的
// 事件号当标识，而它也是连接器最自然的源事件标识；共用一个空间，人先录的那一条会让源随后送来的同号素材被答成重复
// 投递而丢掉。
const ManualTrackingSourcePrefix = "TRKMANUAL/"

// manualEventIDLimit 是操作员给的事件号的长度上限。
const manualEventIDLimit = 200

// manualEventReference 把操作员给的事件号放进人工录入自己的名字空间：前缀之后再按凭证分开。操作员多半拿每张运单内的
// 序号或表格行号当事件号，只在一张运单里唯一；不按凭证分，第二张运单的「1」会被认成第一张运单那一条的重复而丢掉。
// 已经带着前缀的（从响应里抄回来的）原样用。空的交回空。更正指向也经这里：它因此只指得到同一凭证上人工录入的轨迹，
// 指不到轨迹源送来的那些。
func manualEventReference(credential, event string) string {
	event = strings.TrimSpace(event)
	if event == "" || strings.HasPrefix(event, ManualTrackingSourcePrefix) {
		return event
	}
	return ManualTrackingSourcePrefix + credential + "/" + event
}

// ManualTrackingEntryPayload 是一条轨迹的线格式。source 是轨迹源（与有效时间规则、里程碑映射里登记的同一个词）；
// credential 是外部承运凭证（承运商运单号等，须先登记到载运对象上）；status 是承运商的原始状态词，原样填；occurredAt
// 是承运商给的发生时间，承运商没给就不填——不拿录入时间顶替。eventId 可不填：不填时按（源、凭证、状态、发生时间）
// 派生一个，同一条重复导入因此认得出来。correctionOf 填被更正那一条的 eventId——录入时填的那个或响应里交回的那个都行，
// 指的只能是同一个凭证上人工录入的轨迹。original 必填。
type ManualTrackingEntryPayload struct {
	Source       string `json:"source"`
	Credential   string `json:"credential"`
	EventID      string `json:"eventId"`
	OccurredAt   string `json:"occurredAt"`
	Status       string `json:"status"`
	CorrectionOf string `json:"correctionOf"`
	Original     string `json:"original"`
}

// ManualTrackingEntriesPayload 是手工录入口的线格式：一条或一批。
type ManualTrackingEntriesPayload struct {
	Entries []ManualTrackingEntryPayload `json:"entries"`
}

// ManualTrackingEntryIntake 是手工录入口的 Intake：交回一批待认领的素材，租户与录入人取认证结果，接收时刻由服务端给。
type ManualTrackingEntryIntake interface {
	IntakeManualTrackingEntries(ctx context.Context, request *http.Request, now time.Time) ([]ports.TrackingMaterial, error)
}

var _ ManualTrackingEntryIntake = (*OperatorDecisionIntake)(nil)

// TrackingMaterialAdopter 是本口转交的编排：一条素材一笔事务（装配点负责）。
type TrackingMaterialAdopter interface {
	Adopt(ctx context.Context, material ports.TrackingMaterial) (application.AdoptTrackingMaterialResult, error)
}

func (intake *OperatorDecisionIntake) IntakeManualTrackingEntries(ctx context.Context, request *http.Request, now time.Time) ([]ports.TrackingMaterial, error) {
	operator, err := intake.identityFor(ctx, request, OperatorDecisionExternalTrackingEntry)
	if err != nil {
		return nil, err
	}
	var payload ManualTrackingEntriesPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return nil, err
	}
	if len(payload.Entries) == 0 || len(payload.Entries) > ManualTrackingEntryLimit {
		return nil, fmt.Errorf("%w: entries must carry 1 to %d items", ErrMalformedRequest, ManualTrackingEntryLimit)
	}
	materials := make([]ports.TrackingMaterial, 0, len(payload.Entries))
	for index, entry := range payload.Entries {
		material := ports.TrackingMaterial{
			Source: ports.TrackingSourceReference(strings.TrimSpace(entry.Source)),
			Subject: ports.TrackingSubject{
				Tenant: operator.Tenant, CredentialReference: strings.TrimSpace(entry.Credential),
			},
			ReceivedAt:      now.UTC(),
			StatusReference: strings.TrimSpace(entry.Status),
			EnteredBy:       "OPERATOR/" + operator.Operator,
			Original:        strings.TrimSpace(entry.Original),
		}
		if material.Original == "" {
			return nil, fmt.Errorf("%w: entries[%d].original is required for a manually entered tracking event", ErrMalformedRequest, index)
		}
		occurred := strings.TrimSpace(entry.OccurredAt)
		if occurred != "" {
			at, err := time.Parse(time.RFC3339, occurred)
			if err != nil {
				return nil, fmt.Errorf("%w: entries[%d].occurredAt is not an RFC 3339 instant", ErrMalformedRequest, index)
			}
			material.OccurredAt = ports.SourceTime{Given: true, At: at.UTC()}
		}
		// 指纹与派生的事件号用解析后的 UTC 时刻，不用原样的字符串：同一时刻换一种写法（时区、小数位）再导一遍，该被
		// 认成同一条。
		instant := ""
		if material.OccurredAt.Given {
			instant = material.OccurredAt.At.Format(time.RFC3339Nano)
		}
		if len(entry.EventID) > manualEventIDLimit || len(entry.CorrectionOf) > manualEventIDLimit+len(ManualTrackingSourcePrefix)+len(entry.Credential)+1 {
			return nil, fmt.Errorf("%w: entries[%d].eventId or correctionOf is too long", ErrMalformedRequest, index)
		}
		material.CorrectionOf = manualEventReference(material.Subject.CredentialReference, entry.CorrectionOf)
		canonical := strings.Join([]string{string(material.Source), material.Subject.CredentialReference, material.StatusReference, instant, material.CorrectionOf}, "\x00")
		digest := sha256.Sum256([]byte(canonical))
		material.PayloadDigest = hex.EncodeToString(digest[:])
		material.Payload = []byte(canonical)
		event := strings.TrimSpace(entry.EventID)
		if event == "" {
			// 操作员没给事件号：按内容派生一个。源适配器那一路不许这样做（替源发明身份）；这里的「源」就是录入的人，
			// 同一条再导一遍该被认成重复，而不是落第二条事实。
			event = hex.EncodeToString(digest[:16])
		}
		material.SourceEventID = manualEventReference(material.Subject.CredentialReference, event)
		materials = append(materials, material)
	}
	return materials, nil
}

const manualEntryAnswerNotFormed = "ANSWER_NOT_FORMED"

var errManualBatchNotProcessed = errors.New("tf http: no entry of the manual tracking batch formed an answer")

type manualTrackingEntryAnswer struct {
	Index           int    `json:"index"`
	Outcome         string `json:"outcome"`
	Reason          string `json:"reason,omitempty"`
	EventID         string `json:"eventId,omitempty"`
	Fact            string `json:"fact,omitempty"`
	Version         string `json:"version,omitempty"`
	Object          string `json:"object,omitempty"`
	EffectiveBasis  string `json:"effectiveBasis,omitempty"`
	HandoffNotSent  bool   `json:"handoffNotSent,omitempty"`
	Continuation    string `json:"continuationReference,omitempty"`
	AnswerNotFormed bool   `json:"answerNotFormed,omitempty"`
}

// manualTrackingEntriesResponse 是本口的响应：各条各自的答复，加按答复名数出来的小计。outcome 恒为 ENTRIES_PROCESSED
// ——一批里各条各答各的，没有一个能代表整批的业务结果。
type manualTrackingEntriesResponse struct {
	Outcome string                      `json:"outcome"`
	Counts  map[string]int              `json:"counts"`
	Entries []manualTrackingEntryAnswer `json:"entries"`
}

// NewManualTrackingEntriesEndpoint 交回手工录入口（POST /transport-fulfillment-external-tracking-manual-entries）。各条
// 逐条认领、各自成一笔事务：一条不成形或未决不拖累别的条。状态码只答这一批有没有被处理：处理了即 200，本批至少落了
// 一版新事实即 201；某一条没形成答案记在那一条上（answerNotFormed），重投整批是安全的——已落的认成重复。
func NewManualTrackingEntriesEndpoint(intake ManualTrackingEntryIntake, adopter TrackingMaterialAdopter, now func() time.Time) http.Handler {
	return commandEndpoint(func(request *http.Request) (manualTrackingEntriesResponse, error, bool) {
		materials, err := intake.IntakeManualTrackingEntries(request.Context(), request, now())
		if err != nil {
			return manualTrackingEntriesResponse{}, err, false
		}
		response := manualTrackingEntriesResponse{Outcome: "ENTRIES_PROCESSED", Counts: map[string]int{}}
		for index, material := range materials {
			answer := manualTrackingEntryAnswer{Index: index, EventID: material.SourceEventID}
			result, err := adopter.Adopt(request.Context(), material)
			switch {
			case err != nil || result.Outcome().String() == "":
				answer.Outcome, answer.AnswerNotFormed = manualEntryAnswerNotFormed, true
			default:
				answer.Outcome = result.Outcome().String()
				if result.Outcome() == application.TrackingAdoptionUndecided {
					answer.Reason = result.UndecidedReason().String()
				}
				if result.Outcome() == application.TrackingMaterialLeftUnadopted {
					answer.Reason = result.UnadoptedReason().String()
				}
				answer.Continuation = result.ContinuationReference()
				answer.HandoffNotSent = result.HandoffReference() != ""
				if record, has := result.Record(); has {
					answer.Fact, answer.Version = record.Key.Fact.String(), record.Key.Version.String()
					answer.Object = record.Fact.Object().String()
					answer.EffectiveBasis = record.Fact.Effective().Basis().String()
				}
			}
			response.Counts[answer.Outcome]++
			response.Entries = append(response.Entries, answer)
		}
		if response.Counts[manualEntryAnswerNotFormed] == len(materials) {
			// 一条答案都没形成（库不通、请求被取消）：这一批没有被处理，如实答没形成答案。重投整批是安全的。
			return manualTrackingEntriesResponse{}, errManualBatchNotProcessed, true
		}
		return response, nil, true
	}, func(writer http.ResponseWriter, response manualTrackingEntriesResponse) {
		status := http.StatusOK
		if response.Counts[application.TrackingFactAdopted.String()]+response.Counts[application.TrackingFactAdoptedPendingJudgment.String()] > 0 {
			status = http.StatusCreated
		}
		writeJSON(writer, status, response)
	})
}
