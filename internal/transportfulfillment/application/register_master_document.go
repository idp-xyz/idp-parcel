package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedMasterDocumentSave 说明总单登记册交回了封闭集合以外的写入结果。
var ErrUnexpectedMasterDocumentSave = errors.New("transport fulfillment: unexpected master document save outcome")

// MasterDocumentRegistrationOutcome 是总单登记的应用结果代数（ADR-0113；票 tf-carrier-master-document-register/01）。
//
// 首登与形成新版本（撤销 / 替代 / 关联重述）是两个入口，答案却共用一套：两者落库的都是一个版本，重放、冲突、
// 未受理、未决四格的续办也一样。多出来的三格各说一件只有这本册子才有的事：`已有版本链`（首登撞上一份已经
// 登记过的总单——改它走新版本入口，不许再立一个首版）、`未登记`（给一份从没登记过的总单形成新版本——版本不出
// 无中生有的总单）、`已不适用`（对已撤销 / 已替代的总单再形成版本——去看它最后一版是谁收的）。
type MasterDocumentRegistrationOutcome uint8

const (
	MasterDocumentRegistrationOutcomeInvalid MasterDocumentRegistrationOutcome = iota
	MasterDocumentRegistered
	MasterDocumentRevised
	MasterDocumentExistingVersion
	MasterDocumentContentConflict
	MasterDocumentAlreadyChained
	MasterDocumentNotRegistered
	MasterDocumentNoLongerInForce
	MasterDocumentRegistrationNotAccepted
	MasterDocumentRegistrationUndecided
)

func (outcome MasterDocumentRegistrationOutcome) String() string {
	switch outcome {
	case MasterDocumentRegistered:
		return "MASTER_DOCUMENT_REGISTERED"
	case MasterDocumentRevised:
		return "MASTER_DOCUMENT_REVISED"
	case MasterDocumentExistingVersion:
		return "EXISTING_VERSION"
	case MasterDocumentContentConflict:
		return "CONTENT_CONFLICT"
	case MasterDocumentAlreadyChained:
		return "MASTER_DOCUMENT_ALREADY_REGISTERED"
	case MasterDocumentNotRegistered:
		return "MASTER_DOCUMENT_NOT_REGISTERED"
	case MasterDocumentNoLongerInForce:
		return "NO_LONGER_IN_FORCE"
	case MasterDocumentRegistrationNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case MasterDocumentRegistrationUndecided:
		return "REGISTRATION_UNDECIDED"
	default:
		return ""
	}
}

// MasterDocumentRegistrationUndecidedReason 指名登记停在哪一步等谁。本编排只有一条缝，也就只有一格。
type MasterDocumentRegistrationUndecidedReason uint8

const (
	MasterDocumentRegistrationUndecidedReasonNone MasterDocumentRegistrationUndecidedReason = iota
	MasterDocumentRegistryUnavailable
)

func (reason MasterDocumentRegistrationUndecidedReason) String() string {
	switch reason {
	case MasterDocumentRegistryUnavailable:
		return "MASTER_DOCUMENT_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// MasterDocumentAssociationInput 是登记输入里的一条关联：类别词取 domain.AssociatedObjectKind 的封闭词
// （CONSOLIDATION_UNIT / PARCEL / FULFILLMENT_SEGMENT），词不在集合内即未受理。
type MasterDocumentAssociationInput struct {
	Kind      string
	Reference string
}

// RegisterMasterDocumentCommand 携带一份总单首版的全部输入：CONTEXT「总单」点名的签发方、主运输凭证范围、
// 关联对象集，加可缺的运输委托 / 订舱引用、总单身份、版本与租户。
//
// 总单引用与版本都由登记方指名而不是这里铸（ADR-0113 决定二）：引用通常就是签发方给的总单号，版本是登记方
// 这一侧对这份总单的版本身份，同一版本重放要能被认出来（ADR-0031），铸出来的号做不到这一点。
type RegisterMasterDocumentCommand struct {
	TenantID     domain.TenantID
	Document     string
	Version      string
	Issuer       string
	Scope        string
	Commission   string
	Booking      string
	Associations []MasterDocumentAssociationInput
}

// ReviseMasterDocumentCommand 携带一次形成新版本的全部输入。Revision 取 domain.MasterDocumentRevision 三格
// （REVOKE / SUPERSEDE / RESTATE_ASSOCIATIONS）；Replacement 只在替代时有意义且必备；Associations 只在关联重述
// 时有意义。At 是改变的业务时间，NewVersion 是新版本的身份——沿用当前版本号就是覆盖，领域会拒。
type ReviseMasterDocumentCommand struct {
	TenantID     domain.TenantID
	Document     string
	Revision     domain.MasterDocumentRevision
	At           time.Time
	NewVersion   string
	Replacement  string
	Associations []MasterDocumentAssociationInput
}

type RegisterMasterDocumentResult struct {
	outcome      MasterDocumentRegistrationOutcome
	reason       MasterDocumentRegistrationUndecidedReason
	record       ports.MasterDocumentRecord
	hasRecord    bool
	continuation string
}

func (result RegisterMasterDocumentResult) Outcome() MasterDocumentRegistrationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result RegisterMasterDocumentResult) UndecidedReason() MasterDocumentRegistrationUndecidedReason {
	return result.reason
}

// Record 在`已登记`、`已形成新版本`、`已有版本`时给出那一版；`已有版本链`、`已不适用`与链被别人推进后的
// `内容冲突`时给出撞上的当前版，让调用方不必再读一次就知道自己撞的是谁。
func (result RegisterMasterDocumentResult) Record() (ports.MasterDocumentRecord, bool) {
	return result.record, result.hasRecord
}

func (result RegisterMasterDocumentResult) ContinuationReference() string {
	return result.continuation
}

type RegisterMasterDocumentDeps struct {
	Documents ports.MasterDocumentRegistry
	Clock     ports.Clock
}

// RegisterMasterDocumentHandler 是总单登记册的写侧编排：首登一份总单，或对它此刻的当前版落一次撤销、
// 替代、关联重述。没有意图交付：总单登记是本上下文自己的凭证事实，parcel-pricing 的主单级评价按需来引
// （ADR-0111），没有谁要在它变动时被通知。
type RegisterMasterDocumentHandler struct {
	deps RegisterMasterDocumentDeps
}

func NewRegisterMasterDocumentHandler(deps RegisterMasterDocumentDeps) *RegisterMasterDocumentHandler {
	return &RegisterMasterDocumentHandler{deps: deps}
}

// Register 首登一份总单：受理（完备性由 RegisterMasterDocument 把门）→ 幂等按（租户+总单+版本）分重放/冲突
// → 这份总单若已有别的版本则拒立第二个首版 → 原子提交。
func (handler *RegisterMasterDocumentHandler) Register(
	ctx context.Context,
	command RegisterMasterDocumentCommand,
) (RegisterMasterDocumentResult, error) {
	document, err := masterDocumentFrom(command)
	if err != nil {
		return RegisterMasterDocumentResult{outcome: MasterDocumentRegistrationNotAccepted}, nil
	}
	key := ports.MasterDocumentKey{TenantID: command.TenantID, Document: document.Document(), Version: document.Version()}

	existing, found, err := handler.deps.Documents.FindByKey(ctx, key)
	if err != nil {
		return masterDocumentRegistryUndecided(key), nil
	}
	if found {
		return masterDocumentReplayOrConflict(existing, document), nil
	}

	// 同一份总单只有一个首版。它已经有版本链时，登记方要做的是形成新版本而不是再立一份——两个都不回指
	// 前版的版本会让「当前版」成为两个答案（0017 的部分唯一索引是这一格的第二道门）。
	current, found, err := handler.deps.Documents.FindCurrent(ctx, command.TenantID, document.Document())
	if err != nil {
		return masterDocumentRegistryUndecided(key), nil
	}
	if found {
		return RegisterMasterDocumentResult{outcome: MasterDocumentAlreadyChained, record: current, hasRecord: true}, nil
	}

	return handler.commit(ctx, ports.MasterDocumentRecord{
		Key:        key,
		Document:   document,
		RecordedAt: handler.deps.Clock.Now(),
	}, MasterDocumentRegistered)
}

// Revise 对一份总单此刻的当前版形成新版本：读回当前版 → 领域三门之一（只对有效版本开放、新版本回指本版）
// → 以新版本键登记。同一新版本重放答`已有版本`。
func (handler *RegisterMasterDocumentHandler) Revise(
	ctx context.Context,
	command ReviseMasterDocumentCommand,
) (RegisterMasterDocumentResult, error) {
	reference, version, replacement, associations, err := revisionFrom(command)
	if err != nil {
		return RegisterMasterDocumentResult{outcome: MasterDocumentRegistrationNotAccepted}, nil
	}
	key := ports.MasterDocumentKey{TenantID: command.TenantID, Document: reference, Version: version}

	current, found, err := handler.deps.Documents.FindCurrent(ctx, command.TenantID, reference)
	if err != nil {
		return masterDocumentRegistryUndecided(key), nil
	}
	if !found {
		return RegisterMasterDocumentResult{outcome: MasterDocumentNotRegistered}, nil
	}
	if current.Key.Version == version {
		// 新版本号就是当前版：要么是同一次改变的重放（当前版已是那次改变的产物），要么是拿当前版本号再改
		// 一次。领域门会把后者当覆盖拒掉；前者按内容比对分重放/冲突。
		return handler.replayAgainstCurrent(ctx, current, command, replacement, associations), nil
	}

	revised, err := applyRevision(current.Document, command, version, replacement, associations)
	if errors.Is(err, domain.ErrMasterDocumentNoLongerInForce) {
		return RegisterMasterDocumentResult{outcome: MasterDocumentNoLongerInForce, record: current, hasRecord: true}, nil
	}
	if err != nil {
		return RegisterMasterDocumentResult{outcome: MasterDocumentRegistrationNotAccepted}, nil
	}

	return handler.commit(ctx, ports.MasterDocumentRecord{
		Key:        key,
		Document:   revised,
		RecordedAt: handler.deps.Clock.Now(),
	}, MasterDocumentRevised)
}

// replayAgainstCurrent 处理新版本号与当前版相同的那一格。当前版若是首版，拿它自己的版本号改它就是覆盖——
// 交给领域门去拒（它按 version == document.version 拒）；当前版若已是一次改变的产物，把这次命令套在它的
// 前版上重算一遍，比对是不是同一次改变。
func (handler *RegisterMasterDocumentHandler) replayAgainstCurrent(
	ctx context.Context,
	current ports.MasterDocumentRecord,
	command ReviseMasterDocumentCommand,
	replacement domain.MasterDocumentReference,
	associations []domain.MasterDocumentAssociation,
) RegisterMasterDocumentResult {
	prior, has := current.Document.Supersedes()
	if !has {
		return RegisterMasterDocumentResult{outcome: MasterDocumentRegistrationNotAccepted}
	}
	previous, found, err := handler.deps.Documents.FindByKey(ctx, ports.MasterDocumentKey{
		TenantID: current.Key.TenantID, Document: current.Key.Document, Version: prior,
	})
	if err != nil || !found {
		return masterDocumentRegistryUndecided(current.Key)
	}
	replayed, err := applyRevision(previous.Document, command, current.Key.Version, replacement, associations)
	if err != nil {
		return RegisterMasterDocumentResult{outcome: MasterDocumentContentConflict, record: current, hasRecord: true}
	}
	return masterDocumentReplayOrConflict(current, replayed)
}

// commit 提交记录；写口答`已登记`时读回同键——找到是并发下另一方先提交了同一版，按内容分重放/冲突；找不到
// 说明撞的是链的索引（第二个首版、同一前版被回指两次），读回当前版：首登答`已有版本链`，形成新版本答
// `内容冲突`并带回当前版，让调用方在链尾之上重做。
func (handler *RegisterMasterDocumentHandler) commit(
	ctx context.Context,
	record ports.MasterDocumentRecord,
	outcome MasterDocumentRegistrationOutcome,
) (RegisterMasterDocumentResult, error) {
	saved, err := handler.deps.Documents.Save(ctx, record)
	if err != nil {
		return masterDocumentRegistryUndecided(record.Key), nil
	}
	switch saved {
	case ports.MasterDocumentSaved:
		return RegisterMasterDocumentResult{outcome: outcome, record: record, hasRecord: true}, nil
	case ports.MasterDocumentAlreadyRegistered:
		winner, found, err := handler.deps.Documents.FindByKey(ctx, record.Key)
		if err != nil {
			return masterDocumentRegistryUndecided(record.Key), nil
		}
		if found {
			return masterDocumentReplayOrConflict(winner, record.Document), nil
		}
		current, found, err := handler.deps.Documents.FindCurrent(ctx, record.Key.TenantID, record.Key.Document)
		if err != nil || !found {
			return masterDocumentRegistryUndecided(record.Key), nil
		}
		if outcome == MasterDocumentRegistered {
			return RegisterMasterDocumentResult{outcome: MasterDocumentAlreadyChained, record: current, hasRecord: true}, nil
		}
		return RegisterMasterDocumentResult{outcome: MasterDocumentContentConflict, record: current, hasRecord: true}, nil
	default:
		return RegisterMasterDocumentResult{}, fmt.Errorf("%w: %d", ErrUnexpectedMasterDocumentSave, saved)
	}
}

// masterDocumentReplayOrConflict 按业务内容比对同键的两个版本（ADR-0031）：内容相同是重放，不同是冲突——
// 冲突保留原版本，改它走新版本入口，不按最后到达顶替。
func masterDocumentReplayOrConflict(
	existing ports.MasterDocumentRecord,
	incoming domain.MasterDocument,
) RegisterMasterDocumentResult {
	if existing.Document.Equal(incoming) {
		return RegisterMasterDocumentResult{outcome: MasterDocumentExistingVersion, record: existing, hasRecord: true}
	}
	return RegisterMasterDocumentResult{outcome: MasterDocumentContentConflict, record: existing, hasRecord: true}
}

func masterDocumentFrom(command RegisterMasterDocumentCommand) (domain.MasterDocument, error) {
	spec := domain.MasterDocumentSpec{TenantID: command.TenantID}
	var err error
	if spec.Document, err = domain.NewMasterDocumentReference(command.Document); err != nil {
		return domain.MasterDocument{}, err
	}
	if spec.Version, err = domain.NewMasterDocumentVersion(command.Version); err != nil {
		return domain.MasterDocument{}, err
	}
	if spec.Issuer, err = domain.NewMasterDocumentIssuerReference(command.Issuer); err != nil {
		return domain.MasterDocument{}, err
	}
	if spec.Scope, err = domain.NewTransportScopeReference(command.Scope); err != nil {
		return domain.MasterDocument{}, err
	}
	if strings.TrimSpace(command.Commission) != "" {
		if spec.Commission, err = domain.NewTransportCommissionReference(command.Commission); err != nil {
			return domain.MasterDocument{}, err
		}
	}
	if strings.TrimSpace(command.Booking) != "" {
		if spec.Booking, err = domain.NewBookingReference(command.Booking); err != nil {
			return domain.MasterDocument{}, err
		}
	}
	if spec.Associations, err = associationsFrom(command.Associations); err != nil {
		return domain.MasterDocument{}, err
	}
	return domain.RegisterMasterDocument(spec)
}

func associationsFrom(inputs []MasterDocumentAssociationInput) ([]domain.MasterDocumentAssociation, error) {
	associations := make([]domain.MasterDocumentAssociation, 0, len(inputs))
	for _, input := range inputs {
		kind, err := domain.ParseAssociatedObjectKind(strings.TrimSpace(input.Kind))
		if err != nil {
			return nil, err
		}
		association, err := domain.NewMasterDocumentAssociation(kind, input.Reference)
		if err != nil {
			return nil, err
		}
		associations = append(associations, association)
	}
	return associations, nil
}

func revisionFrom(command ReviseMasterDocumentCommand) (
	domain.MasterDocumentReference,
	domain.MasterDocumentVersion,
	domain.MasterDocumentReference,
	[]domain.MasterDocumentAssociation,
	error,
) {
	none := domain.MasterDocumentReference{}
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return none, domain.MasterDocumentVersion{}, none, nil, errors.New("blank tenant")
	}
	reference, err := domain.NewMasterDocumentReference(command.Document)
	if err != nil {
		return none, domain.MasterDocumentVersion{}, none, nil, err
	}
	version, err := domain.NewMasterDocumentVersion(command.NewVersion)
	if err != nil {
		return none, domain.MasterDocumentVersion{}, none, nil, err
	}
	switch command.Revision {
	case domain.MasterDocumentRevocation:
		if strings.TrimSpace(command.Replacement) != "" || len(command.Associations) != 0 {
			return none, domain.MasterDocumentVersion{}, none, nil, errors.New("revocation carries neither a replacement nor associations")
		}
		return reference, version, none, nil, nil
	case domain.MasterDocumentSupersession:
		if len(command.Associations) != 0 {
			return none, domain.MasterDocumentVersion{}, none, nil, errors.New("supersession does not restate associations")
		}
		replacement, err := domain.NewMasterDocumentReference(command.Replacement)
		if err != nil {
			return none, domain.MasterDocumentVersion{}, none, nil, err
		}
		return reference, version, replacement, nil, nil
	case domain.MasterDocumentAssociationRestatement:
		if strings.TrimSpace(command.Replacement) != "" {
			return none, domain.MasterDocumentVersion{}, none, nil, errors.New("restatement carries no replacement")
		}
		associations, err := associationsFrom(command.Associations)
		if err != nil {
			return none, domain.MasterDocumentVersion{}, none, nil, err
		}
		return reference, version, none, associations, nil
	default:
		return none, domain.MasterDocumentVersion{}, none, nil, fmt.Errorf("revision %q is not one of revoke, supersede, restate associations", command.Revision)
	}
}

func applyRevision(
	current domain.MasterDocument,
	command ReviseMasterDocumentCommand,
	version domain.MasterDocumentVersion,
	replacement domain.MasterDocumentReference,
	associations []domain.MasterDocumentAssociation,
) (domain.MasterDocument, error) {
	switch command.Revision {
	case domain.MasterDocumentRevocation:
		return current.Revoke(command.At, version)
	case domain.MasterDocumentSupersession:
		return current.Supersede(command.At, replacement, version)
	default:
		return current.RestateAssociations(command.At, associations, version)
	}
}

func masterDocumentRegistryUndecided(key ports.MasterDocumentKey) RegisterMasterDocumentResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		MasterDocumentRegistryUnavailable.String(), key.TenantID.String(), key.Document.String(), key.Version.String(),
	}, "\x00")))
	return RegisterMasterDocumentResult{
		outcome:      MasterDocumentRegistrationUndecided,
		reason:       MasterDocumentRegistryUnavailable,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
