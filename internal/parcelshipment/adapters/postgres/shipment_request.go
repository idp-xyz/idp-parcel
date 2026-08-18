package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ShipmentRequests 实现 ports.ShipmentRequestRepository 与
// ports.CurrentAcceptedParcelTargetView：以来源身份为键的全聚合快照持久化，
// 并以同行投影列支持按包裹反查当前已接受委托（ADR-0060）。
//
// 快照文档的形状照 RehydrateShipmentRequestSpec 设计，读回时逐字段过领域构造函数再进
// RehydrateShipmentRequest——库里一行坏数据在这两道门上暴露，不会变成一个看起来合法的
// 聚合（ADR-0028）。重建那扇门今天只开到`已提交`（ADR-0030 的登记缺口）：写入不设状态
// 门（决定/撤回落库不能拦），读回一份别的状态会得到 ErrRehydrationStateNotSupported
// ——边界如实透出，而不是在适配器里悄悄拼一份缺决定的聚合。
type ShipmentRequests struct {
	db *bentopg.DB
}

func NewShipmentRequests(db *bentopg.DB) (*ShipmentRequests, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &ShipmentRequests{db: db}, nil
}

// FindBySourceIdentity 按完整来源身份取回委托聚合。
//
// 否定结果只回 false，不区分「不存在」与「属于另一个租户或客户账户」——区分它们等于
// 泄露其他作用域是否存在该对象（与来源保全仓储同一条纪律）。
func (repository *ShipmentRequests) FindBySourceIdentity(
	ctx context.Context,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ShipmentRequest{}, false, fmt.Errorf("find shipment request: %w", err)
	}

	var revision int64
	var state uint8
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT revision, state, snapshot
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND source = $3
		    AND source_request_key = $4`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
	).Scan(&revision, &state, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ShipmentRequest{}, false, nil
	}
	if err != nil {
		return domain.ShipmentRequest{}, false, fmt.Errorf("find shipment request: %w", err)
	}

	var document requestDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.ShipmentRequest{}, false, fmt.Errorf("find shipment request: 快照不是本适配器写下的形状：%w", err)
	}
	spec, err := document.rehydrationSpec(revision, domain.ShipmentRequestState(state))
	if err != nil {
		return domain.ShipmentRequest{}, false, fmt.Errorf("find shipment request: %w", err)
	}
	request, err := domain.RehydrateShipmentRequest(spec)
	if err != nil {
		return domain.ShipmentRequest{}, false, fmt.Errorf("find shipment request: %w", err)
	}
	return request, true, nil
}

// Insert 建单只发生一次：revision 从 1 起写入，主键冲突译成`已存在`——那是业务答案
// （另一方先建了单），不是技术故障（ADR-0031）。
func (repository *ShipmentRequests) Insert(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ShipmentRequestInsertOutcomeInvalid, fmt.Errorf("insert shipment request: %w", err)
	}

	raw, err := json.Marshal(documentOf(request))
	if err != nil {
		return ports.ShipmentRequestInsertOutcomeInvalid, fmt.Errorf("insert shipment request: %w", err)
	}
	versionID, parcels := currentParcelProjection(request)
	// `已存在`用 ON CONFLICT DO NOTHING 而不是捕 23505 译码：撞键的 INSERT 会把整个
	// 事务打进中止态，后续读写全部失败——而`已存在`是业务答案（ADR-0031），编排拿到它
	// 还要在同一个事务里继续读原委托作答。零行命中即冲突；委托身份唯一键（同租户同号
	// 不同来源键）撞上也归这一格——那说明身份签发在两个来源键下发了同一个号，同样是
	// 「另一份已经占住了」，答案一致。
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.shipment_request
			(tenant_id, customer_account_id, source, source_request_key,
			 shipment_request_id, revision, state, snapshot, submitted_at,
			 current_submission_version_id, declared_parcel_ids)
		 VALUES ($1, $2, $3, $4, $5, 1, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		request.ShipmentRequestID().String(),
		uint8(request.State()),
		raw,
		request.SubmittedAt().UTC(),
		versionID,
		parcels,
	)
	if err != nil {
		return ports.ShipmentRequestInsertOutcomeInvalid, fmt.Errorf("insert shipment request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ShipmentRequestAlreadyExists, nil
	}
	return ports.ShipmentRequestInserted, nil
}

// Save 在既有委托上推进。预期版本由聚合自己携带（request.Revision() 是它被读出时的
// 版本，转移一律不动它），UPDATE 的 WHERE 带上它并加一：零行命中即`版本冲突`——抢先
// 那一方已经落库，本方要重读再重放。
func (repository *ShipmentRequests) Save(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, fmt.Errorf("save shipment request: %w", err)
	}

	raw, err := json.Marshal(documentOf(request))
	if err != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, fmt.Errorf("save shipment request: %w", err)
	}
	versionID, parcels := currentParcelProjection(request)
	tag, err := executor.Exec(ctx,
		`UPDATE parcel_shipment.shipment_request
		    SET revision = $5 + 1,
		        state = $6,
		        snapshot = $7,
		        saved_at = now(),
		        current_submission_version_id = $8,
		        declared_parcel_ids = $9
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND source = $3
		    AND source_request_key = $4
		    AND revision = $5`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		request.Revision(),
		uint8(request.State()),
		raw,
		versionID,
		parcels,
	)
	if err != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, fmt.Errorf("save shipment request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ShipmentRequestRevisionConflict, nil
	}
	return ports.ShipmentRequestSaved, nil
}

func currentParcelProjection(request domain.ShipmentRequest) (string, []string) {
	current := request.CurrentSubmissionVersion()
	members := current.DeclaredParcelIDs()
	parcels := make([]string, len(members))
	for index, parcel := range members {
		parcels[index] = parcel.String()
	}
	return current.VersionID().String(), parcels
}

// requestDocument 是快照列里的文档形状——RehydrateShipmentRequestSpec 的 JSON 表达。
// Revision 与 State 刻意不进文档：两者都是列（版本给乐观锁用、状态给读回与巡检用），
// 文档里再存一份就是第二个来源，改列不改文档的一次写入会让两处从此各说各话。
type requestDocument struct {
	ShipmentRequestID string             `json:"shipmentRequestId"`
	BatchID           string             `json:"batchId"`
	SubmittedAt       time.Time          `json:"submittedAt"`
	CurrentVersion    versionDocument    `json:"currentVersion"`
	AcceptanceTask    taskDocument       `json:"acceptanceTask"`
	PriorVersions     []versionDocument  `json:"priorVersions,omitempty"`
	PriorTasks        []taskDocument     `json:"priorTasks,omitempty"`
	PriorLink         *priorLinkDocument `json:"priorLink,omitempty"`
}

type priorLinkDocument struct {
	PriorRequestID string `json:"priorRequestId"`
	Kind           uint8  `json:"kind"`
}

type versionDocument struct {
	VersionID         string            `json:"versionId"`
	Source            sourceDocument    `json:"source"`
	DeclaredParcelIDs []string          `json:"declaredParcelIds"`
	Profiles          []profileDocument `json:"profiles,omitempty"`
	EstablishedAt     time.Time         `json:"establishedAt"`
}

type sourceDocument struct {
	TenantID          string    `json:"tenantId"`
	CustomerAccountID string    `json:"customerAccountId"`
	Source            string    `json:"source"`
	RequestKey        string    `json:"requestKey"`
	PayloadDigest     string    `json:"payloadDigest"`
	OccurredAt        time.Time `json:"occurredAt"`
	ReceivedAt        time.Time `json:"receivedAt"`
}

type profileDocument struct {
	Parcel     string              `json:"parcel"`
	Weight     measurementDocument `json:"weight"`
	Dimensions *dimensionsDocument `json:"dimensions,omitempty"`
}

type measurementDocument struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

type dimensionsDocument struct {
	Length string `json:"length"`
	Width  string `json:"width"`
	Height string `json:"height"`
	Unit   string `json:"unit"`
}

type taskDocument struct {
	TaskID              string            `json:"taskId"`
	SubmissionVersionID string            `json:"submissionVersionId"`
	EstablishedAt       time.Time         `json:"establishedAt"`
	State               uint8             `json:"state"`
	WaitingOn           uint8             `json:"waitingOn,omitempty"`
	ProcessingAttempts  []attemptDocument `json:"processingAttempts,omitempty"`
	ReviewCompletion    *reviewDocument   `json:"reviewCompletion,omitempty"`
}

type attemptDocument struct {
	Reason       string    `json:"reason"`
	ResumePath   uint8     `json:"resumePath"`
	Continuation string    `json:"continuation"`
	AttemptedAt  time.Time `json:"attemptedAt"`
}

type reviewDocument struct {
	Authority   string    `json:"authority"`
	Reviewer    string    `json:"reviewer"`
	Evidence    string    `json:"evidence"`
	CompletedAt time.Time `json:"completedAt"`
}

// documentOf 从聚合的公开访问器摊出文档。只读不判断：状态门在读回那一侧的
// RehydrateShipmentRequest 上。
func documentOf(request domain.ShipmentRequest) requestDocument {
	document := requestDocument{
		ShipmentRequestID: request.ShipmentRequestID().String(),
		BatchID:           request.BatchID().String(),
		SubmittedAt:       request.SubmittedAt().UTC(),
		CurrentVersion:    versionDocumentOf(request.CurrentSubmissionVersion()),
		AcceptanceTask:    taskDocumentOf(request.AcceptanceDecisionTask()),
	}
	for _, prior := range request.PriorSubmissionVersions() {
		document.PriorVersions = append(document.PriorVersions, versionDocumentOf(prior))
	}
	for _, prior := range request.PriorAcceptanceTasks() {
		document.PriorTasks = append(document.PriorTasks, taskDocumentOf(prior))
	}
	if link, established := request.PriorRequestLink(); established {
		document.PriorLink = &priorLinkDocument{
			PriorRequestID: link.PriorRequestID().String(),
			Kind:           uint8(link.Kind()),
		}
	}
	return document
}

func versionDocumentOf(version domain.SubmissionVersion) versionDocument {
	source := version.SourceSubmission()
	identity := source.Identity()
	document := versionDocument{
		VersionID: version.VersionID().String(),
		Source: sourceDocument{
			TenantID:          identity.TenantID().String(),
			CustomerAccountID: identity.CustomerAccountID().String(),
			Source:            identity.Source().String(),
			RequestKey:        identity.RequestKey().String(),
			PayloadDigest:     source.Digest().String(),
			OccurredAt:        source.OccurredAt().UTC(),
			ReceivedAt:        source.ReceivedAt().UTC(),
		},
		EstablishedAt: version.EstablishedAt().UTC(),
	}
	for _, parcel := range version.DeclaredParcelIDs() {
		document.DeclaredParcelIDs = append(document.DeclaredParcelIDs, parcel.String())
	}
	for _, profile := range version.DeclaredProfiles() {
		measurement := profile.Measurement()
		profileDoc := profileDocument{
			Parcel: profile.Parcel().String(),
			Weight: measurementDocument{
				Value: measurement.Weight().Value().String(),
				Unit:  measurement.Weight().Unit().String(),
			},
		}
		if dimensions, declared := measurement.Dimensions(); declared {
			profileDoc.Dimensions = &dimensionsDocument{
				Length: dimensions.Length().String(),
				Width:  dimensions.Width().String(),
				Height: dimensions.Height().String(),
				Unit:   dimensions.Unit().String(),
			}
		}
		document.Profiles = append(document.Profiles, profileDoc)
	}
	return document
}

func taskDocumentOf(task domain.AcceptanceDecisionTask) taskDocument {
	document := taskDocument{
		TaskID:              task.TaskID().String(),
		SubmissionVersionID: task.SubmissionVersionID().String(),
		EstablishedAt:       task.EstablishedAt().UTC(),
		State:               taskStateOf(task),
	}
	if waiting, present := task.WaitingOn(); present {
		document.WaitingOn = uint8(waiting)
	}
	for _, attempt := range task.ProcessingAttempts() {
		document.ProcessingAttempts = append(document.ProcessingAttempts, attemptDocument{
			Reason:       attempt.Reason().String(),
			ResumePath:   uint8(attempt.ResumePath()),
			Continuation: attempt.ContinuationReference().String(),
			AttemptedAt:  attempt.AttemptedAt().UTC(),
		})
	}
	if completion, done := task.ManualReviewCompletion(); done {
		document.ReviewCompletion = &reviewDocument{
			Authority:   completion.Authority().String(),
			Reviewer:    completion.Reviewer().String(),
			Evidence:    completion.Evidence().String(),
			CompletedAt: completion.CompletedAt().UTC(),
		}
	}
	return document
}

// taskStateOf 从公开谓词读回任务状态的数字表达。三个谓词覆盖全部三态：既不完成也
// 不停止即运行中。
func taskStateOf(task domain.AcceptanceDecisionTask) uint8 {
	switch {
	case task.IsComplete():
		return uint8(domain.AcceptanceTaskComplete)
	case task.IsStopped():
		return uint8(domain.AcceptanceTaskStopped)
	default:
		return uint8(domain.AcceptanceTaskRunning)
	}
}

// rehydrationSpec 把文档逐字段过领域构造函数拼回重建规格。任何一个构造函数拒绝都说明
// 这一行不是本适配器写下的形状（或写它的版本有 bug），错误如实上抛。
func (document requestDocument) rehydrationSpec(
	revision int64,
	state domain.ShipmentRequestState,
) (domain.RehydrateShipmentRequestSpec, error) {
	requestID, err := domain.NewShipmentRequestID(document.ShipmentRequestID)
	if err != nil {
		return domain.RehydrateShipmentRequestSpec{}, err
	}
	batchID, err := domain.NewSubmissionBatchID(document.BatchID)
	if err != nil {
		return domain.RehydrateShipmentRequestSpec{}, err
	}
	currentVersion, err := document.CurrentVersion.spec()
	if err != nil {
		return domain.RehydrateShipmentRequestSpec{}, err
	}
	acceptanceTask, err := document.AcceptanceTask.spec()
	if err != nil {
		return domain.RehydrateShipmentRequestSpec{}, err
	}
	spec := domain.RehydrateShipmentRequestSpec{
		Revision:          revision,
		ShipmentRequestID: requestID,
		BatchID:           batchID,
		State:             state,
		SubmittedAt:       document.SubmittedAt,
		CurrentVersion:    currentVersion,
		AcceptanceTask:    acceptanceTask,
	}
	for _, prior := range document.PriorVersions {
		priorSpec, err := prior.spec()
		if err != nil {
			return domain.RehydrateShipmentRequestSpec{}, err
		}
		spec.PriorVersions = append(spec.PriorVersions, priorSpec)
	}
	for _, prior := range document.PriorTasks {
		priorSpec, err := prior.spec()
		if err != nil {
			return domain.RehydrateShipmentRequestSpec{}, err
		}
		spec.PriorTasks = append(spec.PriorTasks, priorSpec)
	}
	if document.PriorLink != nil {
		priorID, err := domain.NewShipmentRequestID(document.PriorLink.PriorRequestID)
		if err != nil {
			return domain.RehydrateShipmentRequestSpec{}, err
		}
		spec.PriorLink = domain.RehydratePriorRequestLinkSpec{
			PriorRequestID: priorID,
			Kind:           domain.RequestLinkKind(document.PriorLink.Kind),
		}
	}
	return spec, nil
}

func (document versionDocument) spec() (domain.RehydrateSubmissionVersionSpec, error) {
	versionID, err := domain.NewSubmissionVersionID(document.VersionID)
	if err != nil {
		return domain.RehydrateSubmissionVersionSpec{}, err
	}
	source, err := document.Source.fingerprint()
	if err != nil {
		return domain.RehydrateSubmissionVersionSpec{}, err
	}
	spec := domain.RehydrateSubmissionVersionSpec{
		VersionID:        versionID,
		SourceSubmission: source,
		EstablishedAt:    document.EstablishedAt,
	}
	for _, raw := range document.DeclaredParcelIDs {
		parcel, err := domain.NewDeclaredParcelID(raw)
		if err != nil {
			return domain.RehydrateSubmissionVersionSpec{}, err
		}
		spec.DeclaredParcelIDs = append(spec.DeclaredParcelIDs, parcel)
	}
	for _, profileDoc := range document.Profiles {
		profile, err := profileDoc.profile()
		if err != nil {
			return domain.RehydrateSubmissionVersionSpec{}, err
		}
		spec.Profiles = append(spec.Profiles, profile)
	}
	return spec, nil
}

func (document sourceDocument) fingerprint() (domain.SourceSubmissionFingerprint, error) {
	tenantID, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, err
	}
	customerID, err := domain.NewCustomerAccountID(document.CustomerAccountID)
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, err
	}
	source, err := domain.NewSource(document.Source)
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, err
	}
	requestKey, err := domain.NewSourceRequestKey(document.RequestKey)
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, err
	}
	identity, err := domain.NewSourceIdentity(tenantID, customerID, source, requestKey)
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, err
	}
	digest, err := domain.NewPayloadDigest(document.PayloadDigest)
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, err
	}
	return domain.NewSourceSubmissionFingerprint(identity, digest, document.OccurredAt, document.ReceivedAt)
}

func (document profileDocument) profile() (domain.DeclaredParcelProfile, error) {
	parcel, err := domain.NewDeclaredParcelID(document.Parcel)
	if err != nil {
		return domain.DeclaredParcelProfile{}, err
	}
	weightValue, err := domain.NewMeasurementValue(document.Weight.Value)
	if err != nil {
		return domain.DeclaredParcelProfile{}, err
	}
	weightUnit, err := domain.NewMeasurementUnitReference(document.Weight.Unit)
	if err != nil {
		return domain.DeclaredParcelProfile{}, err
	}
	weight, err := domain.NewDeclaredWeight(weightValue, weightUnit)
	if err != nil {
		return domain.DeclaredParcelProfile{}, err
	}
	dimensions := domain.DeclaredDimensions{}
	if document.Dimensions != nil {
		length, err := domain.NewMeasurementValue(document.Dimensions.Length)
		if err != nil {
			return domain.DeclaredParcelProfile{}, err
		}
		width, err := domain.NewMeasurementValue(document.Dimensions.Width)
		if err != nil {
			return domain.DeclaredParcelProfile{}, err
		}
		height, err := domain.NewMeasurementValue(document.Dimensions.Height)
		if err != nil {
			return domain.DeclaredParcelProfile{}, err
		}
		unit, err := domain.NewMeasurementUnitReference(document.Dimensions.Unit)
		if err != nil {
			return domain.DeclaredParcelProfile{}, err
		}
		if dimensions, err = domain.NewDeclaredDimensions(length, width, height, unit); err != nil {
			return domain.DeclaredParcelProfile{}, err
		}
	}
	measurement, err := domain.NewDeclaredMeasurement(weight, dimensions)
	if err != nil {
		return domain.DeclaredParcelProfile{}, err
	}
	return domain.NewDeclaredParcelProfile(parcel, measurement)
}

func (document taskDocument) spec() (domain.RehydrateAcceptanceTaskSpec, error) {
	taskID, err := domain.NewAcceptanceDecisionTaskID(document.TaskID)
	if err != nil {
		return domain.RehydrateAcceptanceTaskSpec{}, err
	}
	versionID, err := domain.NewSubmissionVersionID(document.SubmissionVersionID)
	if err != nil {
		return domain.RehydrateAcceptanceTaskSpec{}, err
	}
	spec := domain.RehydrateAcceptanceTaskSpec{
		TaskID:              taskID,
		SubmissionVersionID: versionID,
		EstablishedAt:       document.EstablishedAt,
		State:               domain.AcceptanceTaskState(document.State),
		WaitingOn:           domain.ResumePath(document.WaitingOn),
	}
	for _, attemptDoc := range document.ProcessingAttempts {
		reason, err := domain.NewProcessingAttemptReason(attemptDoc.Reason)
		if err != nil {
			return domain.RehydrateAcceptanceTaskSpec{}, err
		}
		continuation, err := domain.NewOwnershipContinuationReference(attemptDoc.Continuation)
		if err != nil {
			return domain.RehydrateAcceptanceTaskSpec{}, err
		}
		attempt, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
			Reason:       reason,
			ResumePath:   domain.ResumePath(attemptDoc.ResumePath),
			Continuation: continuation,
			AttemptedAt:  attemptDoc.AttemptedAt,
		})
		if err != nil {
			return domain.RehydrateAcceptanceTaskSpec{}, err
		}
		spec.ProcessingAttempts = append(spec.ProcessingAttempts, attempt)
	}
	if document.ReviewCompletion != nil {
		authority, err := domain.NewReviewAuthorityReference(document.ReviewCompletion.Authority)
		if err != nil {
			return domain.RehydrateAcceptanceTaskSpec{}, err
		}
		reviewer, err := domain.NewReviewerReference(document.ReviewCompletion.Reviewer)
		if err != nil {
			return domain.RehydrateAcceptanceTaskSpec{}, err
		}
		evidence, err := domain.NewReviewEvidenceReference(document.ReviewCompletion.Evidence)
		if err != nil {
			return domain.RehydrateAcceptanceTaskSpec{}, err
		}
		completion, err := domain.NewManualReviewCompletion(domain.ManualReviewCompletionSpec{
			Authority:   authority,
			Reviewer:    reviewer,
			Evidence:    evidence,
			CompletedAt: document.ReviewCompletion.CompletedAt,
		})
		if err != nil {
			return domain.RehydrateAcceptanceTaskSpec{}, err
		}
		spec.ReviewCompletion = completion
	}
	return spec, nil
}
