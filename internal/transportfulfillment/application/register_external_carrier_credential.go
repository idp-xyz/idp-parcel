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

// ErrUnexpectedCredentialSave 说明凭证登记册交回了封闭集合以外的写入结果。
var ErrUnexpectedCredentialSave = errors.New("transport fulfillment: unexpected external carrier credential save outcome")

// CredentialRegistrationOutcome 是外部承运凭证登记的应用结果代数（label-channel/18）。
//
// 首登与改变适用关系是两个入口，答案却共用一套：两者落库的都是一个版本，重放、冲突、未受理、
// 未决四格的续办也一样。多出来的三格各说一件只有这本册子才有的事：`已有版本链`（首登撞上一份
// 已经登记过的凭证——改它走改变入口，不许再立一个首版）、`未登记`（改一份从没登记过的凭证——
// 改变不出无中生有的凭证）、`已不适用`（对已作废/失效/替代的凭证再改——去看它最后一版是谁收的）。
type CredentialRegistrationOutcome uint8

const (
	CredentialRegistrationOutcomeInvalid CredentialRegistrationOutcome = iota
	CredentialRegistered
	CredentialApplicabilityChanged
	CredentialExistingVersion
	CredentialContentConflict
	CredentialAlreadyChained
	CredentialNotRegistered
	CredentialNoLongerApplicable
	CredentialRegistrationNotAccepted
	CredentialRegistrationUndecided
)

func (outcome CredentialRegistrationOutcome) String() string {
	switch outcome {
	case CredentialRegistered:
		return "CREDENTIAL_REGISTERED"
	case CredentialApplicabilityChanged:
		return "APPLICABILITY_CHANGED"
	case CredentialExistingVersion:
		return "EXISTING_VERSION"
	case CredentialContentConflict:
		return "CONTENT_CONFLICT"
	case CredentialAlreadyChained:
		return "CREDENTIAL_ALREADY_REGISTERED"
	case CredentialNotRegistered:
		return "CREDENTIAL_NOT_REGISTERED"
	case CredentialNoLongerApplicable:
		return "NO_LONGER_APPLICABLE"
	case CredentialRegistrationNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case CredentialRegistrationUndecided:
		return "REGISTRATION_UNDECIDED"
	default:
		return ""
	}
}

// CredentialRegistrationUndecidedReason 指名登记停在哪一步等谁。本编排只有一条缝，也就只有一格。
type CredentialRegistrationUndecidedReason uint8

const (
	CredentialRegistrationUndecidedReasonNone CredentialRegistrationUndecidedReason = iota
	CredentialRegistryUnavailable
)

func (reason CredentialRegistrationUndecidedReason) String() string {
	switch reason {
	case CredentialRegistryUnavailable:
		return "CREDENTIAL_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// RegisterExternalCarrierCredentialCommand 携带一份凭证首版的全部输入：CONTEXT「外部承运凭证」
// 点名的分配方、真实标识对象、适用范围、版本，加凭证身份与租户。
//
// 版本由登记方指名而不是这里铸：它是分配方那一侧这份凭证的版本身份，同一版本重放要能被认出来
// （ADR-0031），铸出来的号做不到这一点。IdentifiedKind 取 domain.IdentifiedObjectKind 的封闭词
// （TRANSPORT_COMMISSION / BOOKING / TRANSPORT_SCHEDULE / CARRIED_OBJECT / FULFILLMENT_SEGMENT），
// 词不在集合内即未受理——类别是登记出来的，不从凭证字符串的格式推断。EffectiveUntil 零值表示开放。
type RegisterExternalCarrierCredentialCommand struct {
	TenantID       domain.TenantID
	Credential     string
	Version        string
	Assigner       string
	IdentifiedKind string
	IdentifiedRef  string
	EffectiveFrom  time.Time
	EffectiveUntil time.Time
}

// ChangeCredentialApplicabilityCommand 携带一次改变适用关系的全部输入。Change 只认作废、失效、
// 替代三格（domain.CredentialStanding 里除`适用中`之外的三个）；Replacement 只在替代时有意义且
// 必备。At 是改变的业务时间，NewVersion 是新版本的身份——沿用当前版本号就是覆盖，领域会拒。
type ChangeCredentialApplicabilityCommand struct {
	TenantID    domain.TenantID
	Credential  string
	Change      domain.CredentialStanding
	At          time.Time
	NewVersion  string
	Replacement string
}

type RegisterExternalCarrierCredentialResult struct {
	outcome      CredentialRegistrationOutcome
	reason       CredentialRegistrationUndecidedReason
	record       ports.ExternalCarrierCredentialRecord
	hasRecord    bool
	continuation string
}

func (result RegisterExternalCarrierCredentialResult) Outcome() CredentialRegistrationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result RegisterExternalCarrierCredentialResult) UndecidedReason() CredentialRegistrationUndecidedReason {
	return result.reason
}

// Record 在`已登记`、`已改变`、`已有版本`时给出那一版；`已有版本链`与`已不适用`时给出撞上的那一版，
// 让调用方不必再读一次就知道自己撞的是谁。
func (result RegisterExternalCarrierCredentialResult) Record() (ports.ExternalCarrierCredentialRecord, bool) {
	return result.record, result.hasRecord
}

func (result RegisterExternalCarrierCredentialResult) ContinuationReference() string {
	return result.continuation
}

type RegisterExternalCarrierCredentialDeps struct {
	Credentials ports.ExternalCarrierCredentialRegistry
	Clock       ports.Clock
}

// RegisterExternalCarrierCredentialHandler 是外部承运凭证登记册的写侧编排：首登一份凭证，或对
// 它此刻的当前版落一次作废、失效、替代。没有意图交付：凭证登记是本上下文自己的实例参数，
// 收编执行器按需来问（ExternalCarrierCredentialResolver），没有谁要在它变动时被通知。
type RegisterExternalCarrierCredentialHandler struct {
	deps RegisterExternalCarrierCredentialDeps
}

func NewRegisterExternalCarrierCredentialHandler(deps RegisterExternalCarrierCredentialDeps) *RegisterExternalCarrierCredentialHandler {
	return &RegisterExternalCarrierCredentialHandler{deps: deps}
}

// Register 首登一份凭证：受理（五件事的完备性由 RegisterExternalCarrierCredential 把门）→ 幂等按
// （租户+凭证+版本）分重放/冲突 → 这份凭证若已有别的版本则拒立第二个首版 → 原子提交。
func (handler *RegisterExternalCarrierCredentialHandler) Register(
	ctx context.Context,
	command RegisterExternalCarrierCredentialCommand,
) (RegisterExternalCarrierCredentialResult, error) {
	credential, err := credentialFrom(command)
	if err != nil {
		return RegisterExternalCarrierCredentialResult{outcome: CredentialRegistrationNotAccepted}, nil
	}
	key := ports.ExternalCarrierCredentialKey{
		TenantID:   command.TenantID,
		Credential: credential.Credential(),
		Version:    credential.Version(),
	}

	existing, found, err := handler.deps.Credentials.FindByKey(ctx, key)
	if err != nil {
		return credentialRegistryUndecided(key), nil
	}
	if found {
		return replayOrConflict(existing, credential), nil
	}

	// 同一份凭证只有一个首版。它已经有版本链时，登记方要做的是改变适用关系而不是再立一份——
	// 两个都不回指前版的版本会让「当前版」成为两个答案。
	current, found, err := handler.deps.Credentials.FindCurrent(ctx, command.TenantID, credential.Credential())
	if err != nil {
		return credentialRegistryUndecided(key), nil
	}
	if found {
		return RegisterExternalCarrierCredentialResult{outcome: CredentialAlreadyChained, record: current, hasRecord: true}, nil
	}

	return handler.commit(ctx, ports.ExternalCarrierCredentialRecord{
		Key:        key,
		Credential: credential,
		RecordedAt: handler.deps.Clock.Now(),
	}, CredentialRegistered)
}

// ChangeApplicability 对一份凭证此刻的当前版落一次改变：读回当前版 → 领域三门之一（只对适用中
// 的版本开放、新版本回指本版、终点落定）→ 以新版本键登记。同一新版本重放答`已有版本`。
func (handler *RegisterExternalCarrierCredentialHandler) ChangeApplicability(
	ctx context.Context,
	command ChangeCredentialApplicabilityCommand,
) (RegisterExternalCarrierCredentialResult, error) {
	reference, version, replacement, err := applicabilityChangeFrom(command)
	if err != nil {
		return RegisterExternalCarrierCredentialResult{outcome: CredentialRegistrationNotAccepted}, nil
	}
	key := ports.ExternalCarrierCredentialKey{TenantID: command.TenantID, Credential: reference, Version: version}

	current, found, err := handler.deps.Credentials.FindCurrent(ctx, command.TenantID, reference)
	if err != nil {
		return credentialRegistryUndecided(key), nil
	}
	if !found {
		return RegisterExternalCarrierCredentialResult{outcome: CredentialNotRegistered}, nil
	}
	if current.Key.Version == version {
		// 新版本号就是当前版：要么是同一次改变的重放（当前版已是那次改变的产物），要么是拿
		// 当前版本号再改一次。领域门会把后者当覆盖拒掉；前者按内容比对分重放/冲突。
		return handler.replayAgainstCurrent(current, command, replacement), nil
	}

	changed, err := applyApplicabilityChange(current.Credential, command, version, replacement)
	if errors.Is(err, domain.ErrCredentialNoLongerApplicable) {
		return RegisterExternalCarrierCredentialResult{outcome: CredentialNoLongerApplicable, record: current, hasRecord: true}, nil
	}
	if err != nil {
		return RegisterExternalCarrierCredentialResult{outcome: CredentialRegistrationNotAccepted}, nil
	}

	return handler.commit(ctx, ports.ExternalCarrierCredentialRecord{
		Key:        key,
		Credential: changed,
		RecordedAt: handler.deps.Clock.Now(),
	}, CredentialApplicabilityChanged)
}

// replayAgainstCurrent 处理新版本号与当前版相同的那一格。当前版若是适用中的，拿它自己的版本号
// 改它就是覆盖——交给领域门去拒（它按 version == credential.version 拒）；当前版若已是一次改变
// 的产物，比对这次命令描述的是不是同一次改变。
func (handler *RegisterExternalCarrierCredentialHandler) replayAgainstCurrent(
	current ports.ExternalCarrierCredentialRecord,
	command ChangeCredentialApplicabilityCommand,
	replacement domain.ExternalCarrierCredentialReference,
) RegisterExternalCarrierCredentialResult {
	if current.Credential.Applicable() {
		return RegisterExternalCarrierCredentialResult{outcome: CredentialRegistrationNotAccepted}
	}
	changedAt, _ := current.Credential.ChangedAt()
	replacedBy, _ := current.Credential.ReplacedBy()
	if current.Credential.Standing() == command.Change && changedAt.Equal(command.At.UTC()) && replacedBy == replacement {
		return RegisterExternalCarrierCredentialResult{outcome: CredentialExistingVersion, record: current, hasRecord: true}
	}
	return RegisterExternalCarrierCredentialResult{outcome: CredentialContentConflict, record: current, hasRecord: true}
}

// commit 提交记录；并发下另一方先提交时读回赢家按内容分重放/冲突。
func (handler *RegisterExternalCarrierCredentialHandler) commit(
	ctx context.Context,
	record ports.ExternalCarrierCredentialRecord,
	outcome CredentialRegistrationOutcome,
) (RegisterExternalCarrierCredentialResult, error) {
	saved, err := handler.deps.Credentials.Save(ctx, record)
	if err != nil {
		return credentialRegistryUndecided(record.Key), nil
	}
	switch saved {
	case ports.ExternalCarrierCredentialSaved:
		return RegisterExternalCarrierCredentialResult{outcome: outcome, record: record, hasRecord: true}, nil
	case ports.ExternalCarrierCredentialAlreadyRegistered:
		winner, found, err := handler.deps.Credentials.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return credentialRegistryUndecided(record.Key), nil
		}
		return replayOrConflict(winner, record.Credential), nil
	default:
		return RegisterExternalCarrierCredentialResult{}, fmt.Errorf("%w: %d", ErrUnexpectedCredentialSave, saved)
	}
}

// replayOrConflict 按业务内容比对同键的两个版本（ADR-0031）：内容相同是重放，不同是冲突——
// 冲突保留原版本，改它走改变入口换新版，不按最后到达顶替。
func replayOrConflict(
	existing ports.ExternalCarrierCredentialRecord,
	incoming domain.ExternalCarrierCredential,
) RegisterExternalCarrierCredentialResult {
	if existing.Credential.Equal(incoming) {
		return RegisterExternalCarrierCredentialResult{outcome: CredentialExistingVersion, record: existing, hasRecord: true}
	}
	return RegisterExternalCarrierCredentialResult{outcome: CredentialContentConflict, record: existing, hasRecord: true}
}

func credentialFrom(command RegisterExternalCarrierCredentialCommand) (domain.ExternalCarrierCredential, error) {
	spec := domain.ExternalCarrierCredentialSpec{TenantID: command.TenantID}
	var err error
	if spec.Credential, err = domain.NewExternalCarrierCredentialReference(command.Credential); err != nil {
		return domain.ExternalCarrierCredential{}, err
	}
	if spec.Version, err = domain.NewExternalCarrierCredentialVersion(command.Version); err != nil {
		return domain.ExternalCarrierCredential{}, err
	}
	if spec.Assigner, err = domain.NewCredentialAssignerReference(command.Assigner); err != nil {
		return domain.ExternalCarrierCredential{}, err
	}
	kind, err := domain.ParseIdentifiedObjectKind(strings.TrimSpace(command.IdentifiedKind))
	if err != nil {
		return domain.ExternalCarrierCredential{}, err
	}
	if spec.Identifies, err = domain.NewIdentifiedObject(kind, command.IdentifiedRef); err != nil {
		return domain.ExternalCarrierCredential{}, err
	}
	if spec.Applicability, err = domain.NewCredentialApplicability(command.EffectiveFrom, command.EffectiveUntil); err != nil {
		return domain.ExternalCarrierCredential{}, err
	}
	return domain.RegisterExternalCarrierCredential(spec)
}

func applicabilityChangeFrom(command ChangeCredentialApplicabilityCommand) (
	domain.ExternalCarrierCredentialReference,
	domain.ExternalCarrierCredentialVersion,
	domain.ExternalCarrierCredentialReference,
	error,
) {
	none := domain.ExternalCarrierCredentialReference{}
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return none, domain.ExternalCarrierCredentialVersion{}, none, errors.New("blank tenant")
	}
	reference, err := domain.NewExternalCarrierCredentialReference(command.Credential)
	if err != nil {
		return none, domain.ExternalCarrierCredentialVersion{}, none, err
	}
	version, err := domain.NewExternalCarrierCredentialVersion(command.NewVersion)
	if err != nil {
		return none, domain.ExternalCarrierCredentialVersion{}, none, err
	}
	switch command.Change {
	case domain.CredentialRevoked, domain.CredentialExpired:
		if strings.TrimSpace(command.Replacement) != "" {
			return none, domain.ExternalCarrierCredentialVersion{}, none, errors.New("replacement only accompanies supersession")
		}
		return reference, version, none, nil
	case domain.CredentialSuperseded:
		replacement, err := domain.NewExternalCarrierCredentialReference(command.Replacement)
		if err != nil {
			return none, domain.ExternalCarrierCredentialVersion{}, none, err
		}
		return reference, version, replacement, nil
	default:
		return none, domain.ExternalCarrierCredentialVersion{}, none, fmt.Errorf("applicability change %q is not one of revoke, expire, supersede", command.Change)
	}
}

func applyApplicabilityChange(
	current domain.ExternalCarrierCredential,
	command ChangeCredentialApplicabilityCommand,
	version domain.ExternalCarrierCredentialVersion,
	replacement domain.ExternalCarrierCredentialReference,
) (domain.ExternalCarrierCredential, error) {
	switch command.Change {
	case domain.CredentialRevoked:
		return current.Revoke(command.At, version)
	case domain.CredentialExpired:
		return current.Expire(command.At, version)
	default:
		return current.Supersede(command.At, replacement, version)
	}
}

func credentialRegistryUndecided(key ports.ExternalCarrierCredentialKey) RegisterExternalCarrierCredentialResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		CredentialRegistryUnavailable.String(), key.TenantID.String(), key.Credential.String(), key.Version.String(),
	}, "\x00")))
	return RegisterExternalCarrierCredentialResult{
		outcome:      CredentialRegistrationUndecided,
		reason:       CredentialRegistryUnavailable,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
