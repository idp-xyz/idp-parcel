package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 申报发送（ADR-0342 决定一、二）：提交版本已形成之后，把首次尝试经它的发送目标对应的通道发出去，越过边界记一笔发送事实。
// 尝试行不改；没登通道答未配置、什么都不发；登记之后就绪或授权失效了也不发（CC CONTEXT「提交授权与就绪判断分别形成和失效」
// 把这一格归申报发送通道）；只有确知没发出去（连不上通道）才重投，发出去之后拿不到答复记待确认，不当失败，也不自动重发——
// 再发走受控重发，要有安全再次发送判断。

// ErrDeclarationNotSent 由发送口交回：确知请求没有离开本进程（连不上通道），重投会改变结果。
var ErrDeclarationNotSent = errors.New("customs compliance: declaration was not sent")

// ErrDeclarationCredentialMissing 由发送口交回：通道登的凭据变量在运行环境里没有值。补上环境之前重投只会再答一次同一格。
var ErrDeclarationCredentialMissing = errors.New("customs compliance: declaration channel credential is missing from the environment")

// DeclarationPackage 是交给通道的申报包：哪一版、哪一次尝试、哪个单元与程序、成员与两份快照引用。幂等键由尝试身份派生，通道
// 据它认出同一次尝试的重到，见 ADR-0342 越权风险点 1。
type DeclarationPackage struct {
	Tenant         string
	Version        string
	Sequence       int
	Unit           string
	Procedure      string
	Members        []string
	Dossier        string
	Roles          string
	Target         string
	IdempotencyKey string
}

// ChannelAnswer 是通道对一次发送的同步答复。Answered 为假即发出去了却没拿到可读的答复。
type ChannelAnswer struct {
	Answered bool
	Accepted bool
	Message  string
	Response string
}

// DeclarationSender 是发送口：把申报包发到通道的端点。确知没发出去交回 ErrDeclarationNotSent；凭据变量缺值交回
// ErrDeclarationCredentialMissing；发出去之后的任何失败都折成未答复，不交错误。
type DeclarationSender interface {
	SendDeclaration(ctx context.Context, channel domain.DeclarationChannel, pkg DeclarationPackage) (ChannelAnswer, error)
}

// SendDeclarationOutcome 是一次发送的答案。
type SendDeclarationOutcome uint8

const (
	SendDeclarationOutcomeInvalid SendDeclarationOutcome = iota
	// DeclarationSent：越过了发送边界，发送事实记下了。
	DeclarationSent
	// DeclarationAlreadySent：这一份尝试已有发送事实，或尝试行本身不是待发送（受控重发形成时已带结果），不再发。
	DeclarationAlreadySent
	// DeclarationChannelUnconfigured：发送目标没登通道，或登的凭据在运行环境里没有值；什么都不发，尝试照旧待发送。
	DeclarationChannelUnconfigured
	// DeclarationSendHeld：登记之后单元不再就绪或授权已失效（或已被撤掉），什么都不发，尝试照旧待发送。
	DeclarationSendHeld
	// DeclarationSendUndecided：提交版本读不回、通道册或发送事实册读写不动、或确知没发出去，要重投。
	DeclarationSendUndecided
)

func (outcome SendDeclarationOutcome) String() string {
	switch outcome {
	case DeclarationSent:
		return "DECLARATION_SENT"
	case DeclarationAlreadySent:
		return "DECLARATION_ALREADY_SENT"
	case DeclarationChannelUnconfigured:
		return "DECLARATION_CHANNEL_UNCONFIGURED"
	case DeclarationSendHeld:
		return "DECLARATION_SEND_HELD"
	case DeclarationSendUndecided:
		return "DECLARATION_SEND_UNDECIDED"
	default:
		return ""
	}
}

type SendDeclarationResult struct {
	outcome SendDeclarationOutcome
	fact    domain.SubmissionSendFact
	cause   error
}

func (result SendDeclarationResult) Outcome() SendDeclarationOutcome { return result.outcome }

func (result SendDeclarationResult) Fact() domain.SubmissionSendFact { return result.fact }

func (result SendDeclarationResult) UndecidedCause() error { return result.cause }

type SendDeclarationDeps struct {
	Submissions ports.DeclarationSubmissionStore
	Channels    ports.DeclarationChannelRegistry
	SendFacts   ports.SubmissionSendFactStore
	Readiness   ports.ReadinessView
	Authority   ports.SubmissionAuthorityView
	Sender      DeclarationSender
	Clock       ports.Clock
}

type SendDeclarationHandler struct {
	deps SendDeclarationDeps
}

func NewSendDeclarationHandler(deps SendDeclarationDeps) (*SendDeclarationHandler, error) {
	if deps.Submissions == nil || deps.Channels == nil || deps.SendFacts == nil || deps.Readiness == nil ||
		deps.Authority == nil || deps.Sender == nil || deps.Clock == nil {
		return nil, fmt.Errorf("customs compliance: send declaration dependency is nil")
	}
	return &SendDeclarationHandler{deps: deps}, nil
}

// Send 发出一版提交的首次尝试。调用方给环境事务：发送事实与入 inbox 那一笔同生同灭。
func (handler *SendDeclarationHandler) Send(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
) (SendDeclarationResult, error) {
	record, found, err := handler.deps.Submissions.FindByVersion(ctx, tenant, version)
	if err != nil {
		return undecidedSend(fmt.Errorf("declaration submission store: %w", err)), nil
	}
	if !found {
		return undecidedSend(fmt.Errorf("declaration submission %s is not visible", version)), nil
	}
	attempt := record.Attempt
	if attempt.Result() != domain.AttemptRegisteredPendingSend {
		return SendDeclarationResult{outcome: DeclarationAlreadySent}, nil
	}
	if fact, sent, err := handler.deps.SendFacts.FindSendFact(ctx, tenant, version, attempt.Sequence()); err != nil {
		return undecidedSend(fmt.Errorf("submission send fact store: %w", err)), nil
	} else if sent {
		return SendDeclarationResult{outcome: DeclarationAlreadySent, fact: fact}, nil
	}
	channel, configured, err := handler.deps.Channels.LoadDeclarationChannel(ctx, tenant, attempt.Target())
	if err != nil {
		return undecidedSend(fmt.Errorf("declaration channel register: %w", err)), nil
	}
	if !configured {
		return SendDeclarationResult{outcome: DeclarationChannelUnconfigured}, nil
	}
	if held, err := handler.lapsed(ctx, tenant, record.Version.Unit()); err != nil {
		return undecidedSend(err), nil
	} else if held {
		return SendDeclarationResult{outcome: DeclarationSendHeld}, nil
	}
	answer, err := handler.deps.Sender.SendDeclaration(ctx, channel, packageOf(tenant, record))
	if errors.Is(err, ErrDeclarationCredentialMissing) {
		return SendDeclarationResult{outcome: DeclarationChannelUnconfigured, cause: err}, nil
	}
	if err != nil {
		return undecidedSend(err), nil
	}
	sentAt := handler.deps.Clock.Now()
	outcome := domain.AttemptPendingConfirmation
	if answer.Answered {
		outcome = domain.AttemptFailed
		if answer.Accepted {
			outcome = domain.AttemptAcknowledged
		}
	}
	fact, err := domain.RecordSubmissionSend(version, attempt.Sequence(), sentAt, outcome, answer.Message, answer.Response)
	if err != nil {
		return SendDeclarationResult{}, err
	}
	saved, err := handler.deps.SendFacts.SaveSendFact(ctx, tenant, fact, sentAt)
	if err != nil {
		return undecidedSend(fmt.Errorf("submission send fact store: %w", err)), nil
	}
	if saved == ports.SubmissionSendConflict {
		// 同一份尝试已有另一笔发送事实，或通道把同一个消息号给过另一份尝试：两侧说法矛盾，不覆盖。
		return SendDeclarationResult{}, fmt.Errorf("customs compliance: send fact conflict for %s/%d", version, attempt.Sequence())
	}
	return SendDeclarationResult{outcome: DeclarationSent, fact: fact}, nil
}

// lapsed 读单元此刻的就绪与授权。两者成版时都有效；此刻任一不在了或已失效，就不再支持这一次实际发送。读不动是未决。
// 不比成版时的那一份依据：重新取得的判断照样支持发送，这与提交编排「双有效才成版」同一口径。
func (handler *SendDeclarationHandler) lapsed(ctx context.Context, tenant domain.TenantID, unit domain.DeclarationUnitID) (bool, error) {
	readiness, found, err := handler.deps.Readiness.LoadReadiness(ctx, tenant, unit)
	if err != nil {
		return false, fmt.Errorf("readiness view: %w", err)
	}
	if !found || !readiness.Effective() {
		return true, nil
	}
	authorization, granted, err := handler.deps.Authority.LoadSubmissionAuthority(ctx, tenant, unit)
	if err != nil {
		return false, fmt.Errorf("submission authority view: %w", err)
	}
	return !granted || !authorization.Effective(), nil
}

func packageOf(tenant domain.TenantID, record ports.DeclarationSubmissionRecord) DeclarationPackage {
	version := record.Version
	members := make([]string, 0, len(version.Members()))
	for _, member := range version.Members() {
		members = append(members, member.String())
	}
	return DeclarationPackage{
		Tenant:         tenant.String(),
		Version:        version.ID().String(),
		Sequence:       record.Attempt.Sequence(),
		Unit:           version.Unit().String(),
		Procedure:      record.Key.Procedure.String(),
		Members:        members,
		Dossier:        version.Dossier().String(),
		Roles:          version.Roles().String(),
		Target:         record.Attempt.Target(),
		IdempotencyKey: fmt.Sprintf("%s/%s/%d", tenant, version.ID(), record.Attempt.Sequence()),
	}
}

func undecidedSend(cause error) SendDeclarationResult {
	return SendDeclarationResult{outcome: DeclarationSendUndecided, cause: cause}
}
