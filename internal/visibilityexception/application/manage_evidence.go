package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ManageEvidenceOutcome 是证据到达与证据对外披露版本两个入口共用的应用处理结果。
// 提交与披露各占其格：材料收到不等于内容成立（`AT-VE-113`），准备好披露版本不等于
// 已对外提交（`AT-VE-132`）——每一格只说自己那一步。
type ManageEvidenceOutcome uint8

const (
	ManageEvidenceOutcomeInvalid ManageEvidenceOutcome = iota
	EvidenceSubmitted
	EvidenceExistingResult
	EvidenceDisclosurePrepared
	EvidenceDisclosureExistingResult
	ManageEvidenceUndecided
	ManageEvidenceNotAccepted
)

func (outcome ManageEvidenceOutcome) String() string {
	switch outcome {
	case EvidenceSubmitted:
		return "EVIDENCE_SUBMITTED"
	case EvidenceExistingResult:
		return "EVIDENCE_EXISTING_RESULT"
	case EvidenceDisclosurePrepared:
		return "DISCLOSURE_PREPARED"
	case EvidenceDisclosureExistingResult:
		return "DISCLOSURE_EXISTING_RESULT"
	case ManageEvidenceUndecided:
		return "UNDECIDED"
	case ManageEvidenceNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// ManageEvidenceUndecidedReason 指名本轮停在哪一步。
type ManageEvidenceUndecidedReason uint8

const (
	ManageEvidenceUndecidedReasonNone ManageEvidenceUndecidedReason = iota
	EvidenceStoreUnavailable
	EvidenceIdentityUnavailable
)

func (reason ManageEvidenceUndecidedReason) String() string {
	switch reason {
	case EvidenceStoreUnavailable:
		return "EVIDENCE_STORE_UNAVAILABLE"
	case EvidenceIdentityUnavailable:
		return "EVIDENCE_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// SubmitEvidenceCommand 携带一次证据到达：谁提交、内容指纹是什么、何时收到。命令里没有
// 评价——评价起点恒为`已收到`，由领域构造器钉死，调用方给不了（「材料被提交不表示其内容
// 已经被认定为事实」，CONTEXT）。也没有案件或索赔键：证据项是被引的本体，引用在引用方
// 身上。租户显式随命令到达（ADR-0003）。
type SubmitEvidenceCommand struct {
	TenantID    domain.TenantID
	Provider    domain.EvidenceProviderReference
	Digest      domain.EvidenceContentDigest
	SubmittedAt time.Time
}

// PrepareEvidenceDisclosureCommand 请求为一项已收到的证据形成对外披露版本：脱敏后的
// 内容指纹与披露范围由准备方给出，原件指纹从证据项自己身上取——调用方指不了原件，
// 也就造不出「来源不明、内容不一致的附件」。
type PrepareEvidenceDisclosureCommand struct {
	TenantID domain.TenantID
	Evidence domain.EvidenceItemID
	Redacted domain.EvidenceContentDigest
	Scope    string
}

type ManageEvidenceResult struct {
	outcome       ManageEvidenceOutcome
	item          domain.EvidenceItem
	hasItem       bool
	disclosure    domain.EvidenceDisclosureVersion
	hasDisclosure bool
	reason        ManageEvidenceUndecidedReason
}

func (result ManageEvidenceResult) Outcome() ManageEvidenceOutcome {
	return result.outcome
}

// Evidence 只在证据项在场（本轮受理或此前已有）时给出。
func (result ManageEvidenceResult) Evidence() (domain.EvidenceItem, bool) {
	return result.item, result.hasItem
}

// Disclosure 只在披露版本成立（本轮或此前）时给出。
func (result ManageEvidenceResult) Disclosure() (domain.EvidenceDisclosureVersion, bool) {
	return result.disclosure, result.hasDisclosure
}

func (result ManageEvidenceResult) UndecidedReason() ManageEvidenceUndecidedReason {
	return result.reason
}

type ManageEvidenceDeps struct {
	Evidence   ports.EvidenceStore
	Identities ports.EvidenceIdentityFactory
	Clock      ports.Clock
}

// ManageEvidenceHandler 是 `UC-VE-007` 步 2 的编排：证据到达形成证据项、证据对外形成
// 披露版本。它与索赔编排（HandleClaimHandler）分立而不并进去：两边的依赖没有一个重合，
// 并进去只会让索赔编排装一套它自己从不用的证据依赖，而装配时漏配一格在编译期看不见。
type ManageEvidenceHandler struct {
	deps ManageEvidenceDeps
}

func NewManageEvidenceHandler(deps ManageEvidenceDeps) *ManageEvidenceHandler {
	return &ManageEvidenceHandler{deps: deps}
}

// SubmitEvidence 受理一次证据到达：要件缺一即未受理；幂等按（提供方+内容指纹），同一提供
// 方再次提交同一份材料返回原项；否则签标识、按`已收到`形成证据项并落库（`AT-VE-113`：
// 形成证据项，不形成事实或责任）。
func (handler *ManageEvidenceHandler) SubmitEvidence(
	ctx context.Context,
	command SubmitEvidenceCommand,
) (ManageEvidenceResult, error) {
	if command.TenantID.String() == "" ||
		command.Provider.String() == "" ||
		command.Digest.String() == "" ||
		command.SubmittedAt.IsZero() {
		return ManageEvidenceResult{outcome: ManageEvidenceNotAccepted}, nil
	}

	existing, found, err := handler.deps.Evidence.FindByProviderDigest(ctx, command.TenantID, command.Provider, command.Digest)
	if err != nil {
		return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
	}
	if found {
		return ManageEvidenceResult{outcome: EvidenceExistingResult, item: existing, hasItem: true}, nil
	}

	id, err := handler.deps.Identities.NextEvidenceItemID(ctx)
	if err != nil {
		return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceIdentityUnavailable}, nil
	}
	item, err := domain.SubmitEvidence(id, command.Provider, command.Digest, command.SubmittedAt)
	if err != nil {
		return ManageEvidenceResult{}, fmt.Errorf("submit evidence: %w", err)
	}

	saved, err := handler.deps.Evidence.Save(ctx, command.TenantID, item)
	if err != nil {
		return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
	}
	switch saved {
	case ports.EvidenceSaved:
		return ManageEvidenceResult{outcome: EvidenceSubmitted, item: item, hasItem: true}, nil
	case ports.EvidenceAlreadyRecorded:
		// 取回之后、写入之前另一方把同（提供方+指纹）建了出来：读回赢家如实交出。
		existing, found, err := handler.deps.Evidence.FindByProviderDigest(ctx, command.TenantID, command.Provider, command.Digest)
		if err != nil || !found {
			return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
		}
		return ManageEvidenceResult{outcome: EvidenceExistingResult, item: existing, hasItem: true}, nil
	default:
		return ManageEvidenceResult{}, fmt.Errorf("submit evidence: unexpected save outcome %d", saved)
	}
}

// PrepareDisclosure 为一项已收到的证据形成对外披露版本：证据项不在场即未受理；幂等按
// （证据项+脱敏指纹+披露范围），同一范围的同一脱敏版本返回原版本，同一脱敏内容对另一
// 范围是另一个版本；披露范围缺席或脱敏指纹与原件相同由领域门拒下（相同即原件外流），
// 编排答未受理而不是撞错——那是准备方给错了东西，不是故障。
// 准备完成只是准备完成：对外提交、送达与对方确认是追偿动作或通知那一侧分别记录的节点
// （`AT-VE-132`）。
func (handler *ManageEvidenceHandler) PrepareDisclosure(
	ctx context.Context,
	command PrepareEvidenceDisclosureCommand,
) (ManageEvidenceResult, error) {
	if command.TenantID.String() == "" ||
		command.Evidence.String() == "" ||
		command.Redacted.String() == "" {
		return ManageEvidenceResult{outcome: ManageEvidenceNotAccepted}, nil
	}

	item, found, err := handler.deps.Evidence.FindByID(ctx, command.TenantID, command.Evidence)
	if err != nil {
		return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
	}
	if !found {
		return ManageEvidenceResult{outcome: ManageEvidenceNotAccepted}, nil
	}

	existing, found, err := handler.deps.Evidence.FindDisclosure(ctx, command.TenantID, command.Evidence, command.Redacted, command.Scope)
	if err != nil {
		return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
	}
	if found {
		return ManageEvidenceResult{
			outcome:       EvidenceDisclosureExistingResult,
			item:          item,
			hasItem:       true,
			disclosure:    existing,
			hasDisclosure: true,
		}, nil
	}

	version, err := domain.PrepareDisclosure(item, command.Redacted, command.Scope, handler.deps.Clock.Now())
	if err != nil {
		if errors.Is(err, domain.ErrInvalidDisclosureVersion) {
			return ManageEvidenceResult{outcome: ManageEvidenceNotAccepted, item: item, hasItem: true}, nil
		}
		return ManageEvidenceResult{}, fmt.Errorf("prepare evidence disclosure: %w", err)
	}

	saved, err := handler.deps.Evidence.SaveDisclosure(ctx, command.TenantID, version)
	if err != nil {
		return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
	}
	switch saved {
	case ports.EvidenceSaved:
		return ManageEvidenceResult{
			outcome:       EvidenceDisclosurePrepared,
			item:          item,
			hasItem:       true,
			disclosure:    version,
			hasDisclosure: true,
		}, nil
	case ports.EvidenceAlreadyRecorded:
		existing, found, err := handler.deps.Evidence.FindDisclosure(ctx, command.TenantID, command.Evidence, command.Redacted, command.Scope)
		if err != nil || !found {
			return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
		}
		return ManageEvidenceResult{
			outcome:       EvidenceDisclosureExistingResult,
			item:          item,
			hasItem:       true,
			disclosure:    existing,
			hasDisclosure: true,
		}, nil
	default:
		return ManageEvidenceResult{}, fmt.Errorf("prepare evidence disclosure: unexpected save outcome %d", saved)
	}
}
