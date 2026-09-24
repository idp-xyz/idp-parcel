package accessidentity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrOperatorNotGranted 表示令牌有效，但操作者不在册、不绑所请求的租户，或此刻没有所请求能力面的
// 生效授予。三种情形同答这一格、不说哪一半不对（ADR-0100 决定四照 ADR-0055 决定四的探针同答）：
// 细分会把「这个主体在册」「它绑在哪个租户」告诉一个只证明了自己持有令牌的人；而三种情形的恢复
// 动作是同一个——管授予的人去登记。
var ErrOperatorNotGranted = errors.New("access identity: operator holds no effective grant for this tenant and capability face")

// ErrOperatorRegistryUnavailable 表示操作者册读不动。它不折进 ErrOperatorNotGranted：读不动是依赖
// 故障，要运维去救；折成「未授予」会让管授予的人去登一笔其实早已登好的授予（分格判据同 ADR-0029）。
var ErrOperatorRegistryUnavailable = errors.New("access identity: operator registry is unavailable")

// OperatorSourceAdminConsole 是操作者信封的来源：固定为管理台（ADR-0100 决定三）。
const OperatorSourceAdminConsole = "ADMIN_CONSOLE"

// OperatorRequest 是一次操作者出示所要的：在哪个租户上、以哪个能力面。
type OperatorRequest struct {
	TenantID string
	Face     CapabilityFace
	// DecisionKind 只在 Face 为 CapabilityOperationDecision 时用：核的是这一种决定的授予。
	DecisionKind DecisionKind
	// Admission 非空表示这一口形成生产事实，铸造前要判准入范围（ADR-0149 决定四、ADR-0151 决定三）；
	// 登记册配置写面不形成生产事实，传空（ADR-0100）。
	Admission *AdmissionRequirement
}

// OperatorEnvelope 是经铸造的操作者身份：租户、操作者主体、铸造那一刻生效的授予集，来源固定为
// 管理台（ADR-0100 决定三）。
//
// 它不复用 SourceEnvelope，也不给客户账户与来源请求键填占位：两个类型让「拿操作者信封去铸客户
// 委托」在编译期走不通。与 SourceEnvelope 同款：字段不导出、包外没有构造函数，拿到一个铸出来的
// 信封就等于它经过了铸造。包外仍能写出零值，所以零值做成不可用——Minted 为假、租户为空、不持有
// 任何一格授予；消费方先问 Minted。
type OperatorEnvelope struct {
	tenantID  string
	subject   OperatorSubject
	grants    []CapabilityFace
	decisions []DecisionKind
}

func (envelope OperatorEnvelope) TenantID() string         { return envelope.tenantID }
func (envelope OperatorEnvelope) Subject() OperatorSubject { return envelope.subject }
func (envelope OperatorEnvelope) Source() string           { return OperatorSourceAdminConsole }

// Holds 答信封里有没有 face 这一格生效授予。
func (envelope OperatorEnvelope) Holds(face CapabilityFace) bool {
	return slices.Contains(envelope.grants, face)
}

// HoldsDecision 答信封里有没有 kind 这一种运营决定的生效授予。「运营决定」一格不进 Holds，同
// OperatorStanding.HoldsAt 的理由。
func (envelope OperatorEnvelope) HoldsDecision(kind DecisionKind) bool {
	return kind != "" && slices.Contains(envelope.decisions, kind)
}

// Minted 答这个信封是不是铸出来的。
func (envelope OperatorEnvelope) Minted() bool { return envelope.subject.issuer != "" }

// OperatorMinter 铸造操作者信封。
type OperatorMinter struct {
	verifier  OperatorCredentialVerifier
	registry  OperatorRegistry
	admission AdmissionScope
	now       func() time.Time
}

// NewOperatorMinter 的 admission 不许缺：没登对照时传 UnconfiguredAdmissionScope，答不在准入范围。
// 允许缺席就得替缺席挑一个答案，而放行与拦截都不是该由构造器挑的。
func NewOperatorMinter(
	verifier OperatorCredentialVerifier,
	registry OperatorRegistry,
	admission AdmissionScope,
	now func() time.Time,
) (*OperatorMinter, error) {
	if verifier == nil || registry == nil || admission == nil || now == nil {
		return nil, ErrNilDependency
	}
	return &OperatorMinter{verifier: verifier, registry: registry, admission: admission, now: now}, nil
}

// MintOperator 铸造一次操作者出示的信封：校验令牌 → 查操作者册 → 按请求的租户与能力面核授予。
//
// 核验方的三格（未配置、令牌不过、取不回公钥集）原样交回；查册之后：册读不动答依赖故障，其余一律
// 答 ErrOperatorNotGranted；请求带准入要求时再判准入范围，不覆盖答 ErrOutsideAdmissionScope、
// 读不动答 ErrAdmissionScopeUnavailable。租户只取自册上的绑定，请求里的租户只用来比对——身份
// 不来自调用方能自由指定的东西（ADR-0003），同 Minter 那一侧的纪律。
func (minter *OperatorMinter) MintOperator(
	ctx context.Context,
	credential OperatorCredential,
	request OperatorRequest,
) (OperatorEnvelope, error) {
	subject, err := minter.verifier.VerifyOperatorCredential(ctx, credential)
	if err != nil {
		return OperatorEnvelope{}, err
	}
	standing, found, err := minter.registry.FindOperator(ctx, subject)
	if err != nil {
		return OperatorEnvelope{}, fmt.Errorf("%w: %w", ErrOperatorRegistryUnavailable, err)
	}
	at := minter.now()
	granted := standing.HoldsAt(request.Face, at)
	if request.Face == CapabilityOperationDecision {
		granted = standing.HoldsDecisionAt(request.DecisionKind, at)
	}
	// 请求没指名租户时取册上的绑定：一个操作者只绑一个租户，取绑定仍是「身份来自册、不来自请求」。
	// 指名了就比对，指名的不是绑定的那个就答未授予。
	named := strings.TrimSpace(request.TenantID)
	if !found || (named != "" && standing.binding.tenantID != named) || !granted {
		return OperatorEnvelope{}, ErrOperatorNotGranted
	}
	// 准入范围判在授予之后：没有授予的人不该从答复里看出这个租户登没登区间。
	if request.Admission != nil {
		admitted, err := minter.admission.Admits(ctx, standing.binding.tenantID, *request.Admission, at)
		if err != nil {
			return OperatorEnvelope{}, fmt.Errorf("%w: %w", ErrAdmissionScopeUnavailable, err)
		}
		if !admitted {
			return OperatorEnvelope{}, ErrOutsideAdmissionScope
		}
	}
	var grants []CapabilityFace
	var decisions []DecisionKind
	for _, recorded := range standing.grants {
		if !recorded.EffectiveAt(at) {
			continue
		}
		if recorded.grant.face == CapabilityOperationDecision {
			if !slices.Contains(decisions, recorded.grant.decisionKind) {
				decisions = append(decisions, recorded.grant.decisionKind)
			}
			continue
		}
		if !slices.Contains(grants, recorded.grant.face) {
			grants = append(grants, recorded.grant.face)
		}
	}
	return OperatorEnvelope{tenantID: standing.binding.tenantID, subject: subject, grants: grants, decisions: decisions}, nil
}
