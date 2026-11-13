package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	// EvidenceAppraised：这一轮形成了采信或不采信的评价。
	EvidenceAppraised
	// EvidenceAlreadyAppraisedResult：这项证据此前已经评价过。评价不改写——异议走调查与裁决——所以交回在册的那一份。
	EvidenceAlreadyAppraisedResult
	// EvidenceNotFound：指名的证据项不在本租户的册上。
	EvidenceNotFound
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
	case EvidenceAppraised:
		return "EVIDENCE_APPRAISED"
	case EvidenceAlreadyAppraisedResult:
		return "EVIDENCE_ALREADY_APPRAISED"
	case EvidenceNotFound:
		return "EVIDENCE_NOT_FOUND"
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
//
// SubmittedAt 为零时取编排时钟：运营决定口不收补报的收到时刻，材料是在登记的这一刻被本产品收下的。Operator 是把它登
// 成证据项的操作者，非空时记作登记人。
type SubmitEvidenceCommand struct {
	TenantID    domain.TenantID
	Provider    domain.EvidenceProviderReference
	Digest      domain.EvidenceContentDigest
	SubmittedAt time.Time
	Operator    string
}

// AppraiseEvidenceCommand 请求对一项已收到的证据作出经调查的评价：采信或不采信，依据必备。评价时刻取编排时钟。
type AppraiseEvidenceCommand struct {
	TenantID  domain.TenantID
	Evidence  domain.EvidenceItemID
	Appraisal domain.EvidenceAppraisal
	Basis     string
	Operator  string
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
	// Decisions 记登记人与评价。Appraise 必须有它；SubmitEvidence 在它在场且命令带了操作者时记登记人。
	Decisions  ports.EvidenceDecisionStore
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
	if command.SubmittedAt.IsZero() && handler.deps.Clock != nil {
		command.SubmittedAt = handler.deps.Clock.Now().UTC()
	}
	if command.TenantID.String() == "" ||
		command.Provider.String() == "" ||
		command.Digest.String() == "" ||
		command.SubmittedAt.IsZero() {
		return ManageEvidenceResult{outcome: ManageEvidenceNotAccepted}, nil
	}
	// 带了操作者却没有地方记登记人：不受理，不让这份证据不声不响地落成一项没有登记人的。
	if strings.TrimSpace(command.Operator) != "" && handler.deps.Decisions == nil {
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
		if operator := strings.TrimSpace(command.Operator); operator != "" && handler.deps.Decisions != nil {
			if err := handler.deps.Decisions.RecordRegistrar(ctx, command.TenantID, item.ID(), operator); err != nil {
				return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
			}
		}
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

// Appraise 对一项已收到的证据作出经调查的评价（`UC-VE-007` 步 2「有效性」）：采信或不采信，依据必备。评价只落一次——
// 已评价的证据再评价，交回在册的那一份而不改写它；两个人同时评价同一项，后落的那一笔同样答已评价。评价不形成事实或
// 责任结论，只说这份材料经调查之后本产品信不信它。
func (handler *ManageEvidenceHandler) Appraise(
	ctx context.Context,
	command AppraiseEvidenceCommand,
) (ManageEvidenceResult, error) {
	basis, operator := strings.TrimSpace(command.Basis), strings.TrimSpace(command.Operator)
	if handler.deps.Decisions == nil || handler.deps.Clock == nil || command.TenantID.String() == "" || command.Evidence.String() == "" ||
		basis == "" || operator == "" ||
		(command.Appraisal != domain.EvidenceCredited && command.Appraisal != domain.EvidenceDiscredited) {
		return ManageEvidenceResult{outcome: ManageEvidenceNotAccepted}, nil
	}
	item, found, err := handler.deps.Evidence.FindByID(ctx, command.TenantID, command.Evidence)
	if err != nil {
		return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
	}
	if !found {
		return ManageEvidenceResult{outcome: EvidenceNotFound}, nil
	}
	if item.Appraisal() != domain.EvidenceReceived {
		return ManageEvidenceResult{outcome: EvidenceAlreadyAppraisedResult, item: item, hasItem: true}, nil
	}
	at := handler.deps.Clock.Now().UTC()
	appraised, err := item.Appraise(command.Appraisal, basis, at)
	if err != nil {
		return ManageEvidenceResult{outcome: ManageEvidenceNotAccepted, item: item, hasItem: true}, nil
	}
	recorded, err := handler.deps.Decisions.AppraiseEvidence(ctx, ports.EvidenceAppraisalRecord{
		Tenant: command.TenantID, Item: appraised, Operator: operator, AppraisedAt: at,
	})
	if err != nil {
		return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
	}
	switch recorded {
	case ports.EvidenceAppraisalRecorded:
		return ManageEvidenceResult{outcome: EvidenceAppraised, item: appraised, hasItem: true}, nil
	case ports.EvidenceAlreadyAppraised:
		// 读到`已收到`之后、落评价之前另一笔评价先落了：读回赢家如实交出，不盖掉它。
		winner, found, err := handler.deps.Evidence.FindByID(ctx, command.TenantID, command.Evidence)
		if err != nil || !found {
			return ManageEvidenceResult{outcome: ManageEvidenceUndecided, reason: EvidenceStoreUnavailable}, nil
		}
		return ManageEvidenceResult{outcome: EvidenceAlreadyAppraisedResult, item: winner, hasItem: true}, nil
	default:
		return ManageEvidenceResult{}, fmt.Errorf("appraise evidence: unexpected outcome %d", recorded)
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
