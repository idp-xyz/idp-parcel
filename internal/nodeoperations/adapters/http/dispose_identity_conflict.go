package nodeopshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是身份冲突处置命令口（UC-NO-004；ADR-0210 决定六）：复核页上持授予的人对一件判身份冲突的作业实物指定一件候选，或确认
// 都不是。它是管理台上作判断的口，归操作者渠道的「运营决定」能力面（ADR-0151 决定一），决定种类`身份冲突处置`。

// OperatorDecisionIdentityConflictDisposition 是复核页处置一件作业实物的身份冲突。
const OperatorDecisionIdentityConflictDisposition OperatorDecision = "IDENTITY_CONFLICT_DISPOSITION"

// IdentityDispositionIntake 把一次已认证的处置请求译成命令。租户与提交操作者只取认证结果（ADR-0151 决定二），请求体只带处置的内容。
type IdentityDispositionIntake interface {
	IntakeIdentityDisposition(ctx context.Context, request *http.Request) (application.DisposeIdentityConflictCommand, error)
}

// IdentityDispositionHandler 是本口转交的处置编排，装配负责把它包进一笔事务。
type IdentityDispositionHandler interface {
	Dispose(ctx context.Context, command application.DisposeIdentityConflictCommand) (application.DisposeIdentityConflictResult, error)
}

// NewDisposeIdentityConflictEndpoint 交回身份冲突处置的 HTTP 入口。按 ADR-0022 映射：首次形成答 201；已有结果、已处置、版本已换代与
// 三格业务拒绝都是形成了的答案，答 200 带 outcome——服务端知道为什么没落版，这不是调用方的错（4xx），也不是没形成答案（5xx）。指名的
// 实物或版本在本租户查不到答 404，与越权探针同答。落册或意图入队失败答 5xx：处置没形成答案，册上什么也没追加。
func NewDisposeIdentityConflictEndpoint(intake IdentityDispositionIntake, handler IdentityDispositionHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeIdentityDisposition(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Dispose(request.Context(), command)
		switch {
		case errors.Is(err, application.ErrDispositionSubjectNotFound):
			writeProblem(response, http.StatusNotFound, codeJudgmentNotVisible)
			return
		case errors.Is(err, application.ErrDispositionCommandRejected):
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		case err != nil:
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeDispositionOutcome(response, command, result)
	})
}

// identityDispositionPayload 是身份冲突处置的线格式（ADR-0210 决定六）：主实物、所处置的版本、结果、指定的包裹（只在指定一件）、
// 连带实物（只在指定一件）、处置依据引用。没有租户、没有操作者，也没有时间——形成时刻取服务端时刻。
type identityDispositionPayload struct {
	Unit            string                  `json:"unit"`
	DisposedVersion string                  `json:"disposedVersion"`
	Result          string                  `json:"result"`
	Parcel          string                  `json:"parcel"`
	CoDisposedUnits []coDisposedUnitPayload `json:"coDisposedUnits"`
	Basis           string                  `json:"basis"`
}

type coDisposedUnitPayload struct {
	Unit    string `json:"unit"`
	Version string `json:"version"`
}

// IntakeIdentityDisposition 译身份冲突处置（`/node-operations/identity-dispositions`）。坏载荷的各种都答坏报文：缺格、空白、带 NUL、
// 未知字段（含任何身份格）、甲不带 X、乙带 X 或连带实物、连带实物重复或就是主实物或缺版本、结果不在词表里。依据引用去两端空白后
// 逐字交下去，别的格原样——标识逐字比对，替调用方改一个字符就指名了另一件。
func (intake *OperatorDecisionIntake) IntakeIdentityDisposition(ctx context.Context, request *http.Request) (application.DisposeIdentityConflictCommand, error) {
	none := application.DisposeIdentityConflictCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionIdentityConflictDisposition)
	if err != nil {
		return none, err
	}
	var payload identityDispositionPayload
	if err := decodeClosedDocument(request, &payload); err != nil {
		return none, err
	}
	malformed := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrMalformedRequest, fmt.Sprintf(format, args...))
	}
	present := func(name, value string) error {
		if strings.TrimSpace(value) == "" {
			return malformed("%s is required", name)
		}
		if strings.ContainsRune(value, 0) {
			return malformed("%s carries NUL", name)
		}
		return nil
	}
	for name, value := range map[string]string{"unit": payload.Unit, "disposedVersion": payload.DisposedVersion, "result": payload.Result, "basis": payload.Basis} {
		if err := present(name, value); err != nil {
			return none, err
		}
	}
	result, err := domain.ParseIdentityDispositionResult(payload.Result)
	if err != nil {
		return none, malformed("result: %v", err)
	}
	command := application.DisposeIdentityConflictCommand{
		TenantID: operator.Tenant,
		Operator: "OPERATOR/" + operator.Operator,
		Result:   result,
		Basis:    strings.TrimSpace(payload.Basis),
	}
	if command.Unit, err = domain.NewHandlingUnitID(payload.Unit); err != nil {
		return none, malformed("unit: %v", err)
	}
	if command.Disposed, err = domain.NewIntakeResultVersion(payload.DisposedVersion); err != nil {
		return none, malformed("disposedVersion: %v", err)
	}
	switch result {
	case domain.DispositionDesignatesParcel:
		if err := present("parcel", payload.Parcel); err != nil {
			return none, err
		}
		if command.Parcel, err = domain.NewParcelAssociationReference(payload.Parcel); err != nil {
			return none, malformed("parcel: %v", err)
		}
	case domain.DispositionNoneOfCandidates:
		if payload.Parcel != "" || len(payload.CoDisposedUnits) > 0 {
			return none, malformed("none-of-candidates names no parcel and no co-disposed unit")
		}
	}
	seen := map[string]bool{payload.Unit: true}
	for index, co := range payload.CoDisposedUnits {
		if err := present(fmt.Sprintf("coDisposedUnits[%d].unit", index), co.Unit); err != nil {
			return none, err
		}
		if err := present(fmt.Sprintf("coDisposedUnits[%d].version", index), co.Version); err != nil {
			return none, err
		}
		if seen[co.Unit] {
			return none, malformed("coDisposedUnits[%d] repeats a unit or names the disposed unit", index)
		}
		seen[co.Unit] = true
		unit, err := domain.NewHandlingUnitID(co.Unit)
		if err != nil {
			return none, malformed("coDisposedUnits[%d].unit: %v", index, err)
		}
		version, err := domain.NewIntakeResultVersion(co.Version)
		if err != nil {
			return none, malformed("coDisposedUnits[%d].version: %v", index, err)
		}
		command.CoDisposed = append(command.CoDisposed, domain.CoDisposedUnit{Unit: unit, Version: version})
	}
	return command, nil
}

var _ IdentityDispositionIntake = (*OperatorDecisionIntake)(nil)

var _ IdentityDispositionIntake = UnconfiguredIntake{}

// IntakeIdentityDisposition 同 IntakeReception：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeIdentityDisposition(context.Context, *http.Request) (application.DisposeIdentityConflictCommand, error) {
	return application.DisposeIdentityConflictCommand{}, ErrAccessChannelNotConfigured
}

// identityDispositionResponse 是本口的封闭响应形状。outcome 取处置结果枚举的原名；unit 与 disposedVersion 恒在场，回请求所指名的那
// 两样。其余各格随 outcome 在场，与 application.DisposeIdentityConflictResult 的各格一一对应：
//   - disposition：首次形成是这一次，已有结果与已处置是在册的那一次；
//   - versions：首次形成是追加的各版，已有结果与已处置是接续所处置版本的那一版；
//   - currentVersions：版本已换代与无可处置的冲突时，主实物此刻各链的当前有效版本；
//   - candidates：不在候选之内时的处置候选；
//   - associatedUnits：关联对象另有归属时，此刻关联到指定包裹的别的实物与它们的当前有效版本——复核页据此带出须连带的实物；
//   - intentVersion：首次形成且交了收寄发布意图时，交了意图的那一版。
type identityDispositionResponse struct {
	Outcome         string                   `json:"outcome"`
	Unit            string                   `json:"unit"`
	DisposedVersion string                   `json:"disposedVersion"`
	Disposition     *identityDispositionBody `json:"disposition,omitempty"`
	Versions        []judgmentVersionBody    `json:"versions,omitempty"`
	CurrentVersions []judgmentVersionBody    `json:"currentVersions,omitempty"`
	Candidates      []string                 `json:"candidates,omitempty"`
	AssociatedUnits []associatedUnitBody     `json:"associatedUnits,omitempty"`
	IntentVersion   string                   `json:"intentVersion,omitempty"`
}

// identityDispositionBody 是处置文档的传输形状：命令口回显与复核目录用同一个。excluded 只在确认都不是时非空；parcel 与
// coDisposedUnits 只在指定一件时出现。
type identityDispositionBody struct {
	DisposedVersion string               `json:"disposedVersion"`
	Unit            string               `json:"unit"`
	Result          string               `json:"result"`
	Parcel          string               `json:"parcel,omitempty"`
	Excluded        []string             `json:"excluded,omitempty"`
	CoDisposedUnits []coDisposedUnitBody `json:"coDisposedUnits,omitempty"`
	Basis           string               `json:"basis"`
	Operator        string               `json:"operator"`
	DisposedAt      string               `json:"disposedAt"`
}

type coDisposedUnitBody struct {
	Unit    string `json:"unit"`
	Version string `json:"version"`
}

// judgmentVersionBody 是一版判断：kind 取收寄判断的封闭词；excluded 只在处置版上非空，是这一版排除的那一组（ADR-0210 决定九据此
// 让重核不关联回去）。
type judgmentVersionBody struct {
	Unit               string   `json:"unit"`
	JudgmentVersion    string   `json:"judgmentVersion"`
	Predecessor        string   `json:"predecessor,omitempty"`
	Kind               string   `json:"kind"`
	Node               string   `json:"node,omitempty"`
	Association        string   `json:"association,omitempty"`
	Candidates         []string `json:"candidates,omitempty"`
	IdentityConflict   bool     `json:"identityConflict,omitempty"`
	Excluded           []string `json:"excluded,omitempty"`
	ServiceMarkers     []string `json:"serviceMarkers,omitempty"`
	ServiceResultCheck string   `json:"serviceResultCheck,omitempty"`
}

type associatedUnitBody struct {
	Unit            string                `json:"unit"`
	CurrentVersions []judgmentVersionBody `json:"currentVersions"`
}

func writeDispositionOutcome(response http.ResponseWriter, command application.DisposeIdentityConflictCommand, result application.DisposeIdentityConflictResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := identityDispositionResponse{Outcome: outcome, Unit: command.Unit.String(), DisposedVersion: command.Disposed.String()}
	if disposition, recorded := result.Disposition(); recorded {
		body.Disposition = identityDispositionBodyOf(disposition)
	}
	for _, version := range result.Versions() {
		body.Versions = append(body.Versions, judgmentVersionBodyOf(version))
	}
	for _, version := range result.CurrentVersions() {
		body.CurrentVersions = append(body.CurrentVersions, judgmentVersionBodyOf(version))
	}
	for _, candidate := range result.Candidates() {
		body.Candidates = append(body.Candidates, candidate.String())
	}
	for _, associated := range result.AssociatedUnits() {
		unit := associatedUnitBody{Unit: associated.Unit.String(), CurrentVersions: []judgmentVersionBody{}}
		for _, version := range associated.CurrentVersions {
			unit.CurrentVersions = append(unit.CurrentVersions, judgmentVersionBodyOf(version))
		}
		body.AssociatedUnits = append(body.AssociatedUnits, unit)
	}
	status := http.StatusOK
	if result.Outcome() == application.IdentityConflictDisposed {
		status = http.StatusCreated
		if key, handed := result.IntakeHandedOff(); handed {
			for _, version := range result.Versions() {
				if version.Key == key {
					body.IntentVersion = version.JudgmentVersion.String()
				}
			}
		}
	}
	writeJSON(response, status, body)
}

func identityDispositionBodyOf(disposition domain.IdentityDisposition) *identityDispositionBody {
	body := &identityDispositionBody{
		DisposedVersion: disposition.Disposed.String(),
		Unit:            disposition.Unit.String(),
		Result:          disposition.Result.String(),
		Parcel:          disposition.Parcel.String(),
		Basis:           disposition.Basis,
		Operator:        disposition.Operator,
		DisposedAt:      rfc3339(disposition.DisposedAt),
	}
	for _, parcel := range disposition.Excluded {
		body.Excluded = append(body.Excluded, parcel.String())
	}
	for _, co := range disposition.CoDisposed {
		body.CoDisposedUnits = append(body.CoDisposedUnits, coDisposedUnitBody{Unit: co.Unit.String(), Version: co.Version.String()})
	}
	return body
}

func judgmentVersionBodyOf(record ports.ReceptionRecord) judgmentVersionBody {
	body := judgmentVersionBody{
		Unit:               record.Intake.Unit().String(),
		JudgmentVersion:    record.JudgmentVersion.String(),
		Predecessor:        record.Predecessor.String(),
		Kind:               record.Kind.String(),
		Node:               record.Intake.Node().String(),
		IdentityConflict:   record.IdentityConflict,
		ServiceMarkers:     append([]string{}, record.ServiceMarkers...),
		ServiceResultCheck: record.ServiceResultCheck.String(),
	}
	if association, associated := record.Intake.Association(); associated {
		body.Association = association.String()
	}
	for _, candidate := range record.Candidates {
		body.Candidates = append(body.Candidates, candidate.String())
	}
	for _, parcel := range record.Disposition.ExcludedFor(record.Intake.Unit()) {
		body.Excluded = append(body.Excluded, parcel.String())
	}
	return body
}

// identityReviewUnitBody 是待识别实物页按实物列的一件：各条判断链此刻的当前有效版本。
type identityReviewUnitBody struct {
	Unit  string                   `json:"unit"`
	Heads []identityReviewHeadBody `json:"heads"`
}

// identityReviewHeadBody 逐格透出一条链的当前有效版本。identityConflict 恒在场（不省成 omitempty）：复核页据它与各版关联判这件实物
// 此刻是否冲突，缺键与 false 分不开就判不准。disposition 只在这一版是处置版时出现。
type identityReviewHeadBody struct {
	SourceID         string                   `json:"sourceId"`
	Kind             string                   `json:"kind"`
	Node             string                   `json:"node"`
	JudgmentVersion  string                   `json:"judgmentVersion"`
	Association      string                   `json:"association,omitempty"`
	Candidates       []string                 `json:"candidates"`
	IdentityConflict bool                     `json:"identityConflict"`
	ReceivedAt       string                   `json:"receivedAt"`
	RecordedAt       string                   `json:"recordedAt"`
	Disposition      *identityDispositionBody `json:"disposition,omitempty"`
}

func identityReviewUnitBodies(request *http.Request, reader ReviewCatalogueReader, tenant domain.TenantID, limit int) ([]identityReviewUnitBody, error) {
	rows, err := reader.ListIdentityReviewUnits(request.Context(), tenant, limit)
	if err != nil {
		return nil, err
	}
	units := make([]identityReviewUnitBody, 0, len(rows))
	for _, row := range rows {
		unit := identityReviewUnitBody{Unit: row.Unit, Heads: make([]identityReviewHeadBody, 0, len(row.Heads))}
		for _, head := range row.Heads {
			body := identityReviewHeadBody{
				SourceID:         head.SourceID,
				Kind:             head.Kind,
				Node:             head.Node,
				JudgmentVersion:  head.JudgmentVersion,
				Association:      head.Association,
				Candidates:       append([]string{}, head.Candidates...),
				IdentityConflict: head.IdentityConflict,
				ReceivedAt:       rfc3339(head.ReceivedAt),
				RecordedAt:       rfc3339(head.RecordedAt),
			}
			if head.Disposition.Recorded() {
				body.Disposition = identityDispositionBodyOf(head.Disposition)
			}
			unit.Heads = append(unit.Heads, body)
		}
		units = append(units, unit)
	}
	return units, nil
}
