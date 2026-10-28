package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrPostIntakeJourneyUndecided：旅程册或意图交付停在自己的未决上。重投会改变结果（册子恢复后落旅程），是等依赖，不是坏信封。
	ErrPostIntakeJourneyUndecided = errors.New("transport fulfillment: post-intake disposition journey start is undecided")
	// ErrPostIntakeUntranslatable：信封的依据种类不是服务处置决定，或译出的命令被编排拒收（未受理、来源冲突）。重投不会变，响亮报出，不入未决名单。
	ErrPostIntakeUntranslatable = errors.New("transport fulfillment: post-intake disposition cannot be translated into a journey start")
)

// PostIntakeJourneyStarter 是本适配器转交的应用编排（UC-TF-007 的旅程启动，与运营决定口同一只处理方）。
type PostIntakeJourneyStarter interface {
	Handle(ctx context.Context, command tfapp.StartAlternateJourneyCommand) (tfapp.StartAlternateJourneyResult, error)
}

// StartJourneyOnPostIntakeDisposition 实现 tfinbox.PostIntakeDispositionHandler：PS 的处置决定向运输履约请求另起备用旅程（改送）或退运旅程（退运）时，
// 以链上这一版作处置依据启动旅程（ADR-0332 请求表，UC-TF-007）。
//
// 只承接「另起旅程」这两个动作。信封里其余请求项（拦截与服务终止对运输履约的「不再安排向前移动」、节点运营与网络路由的各项）不是本适配器的事：
// 系统今天没有这几个所有者的执行器，PS 在决定形成时已把它们同笔落成待人工处理项（ADR-0340），由运营人员登记处理结果。所以这里忽略非「另起旅程」的项、
// 信封照常入账——不丢（待人工处理项在 PS 一侧已落）、不重复（旅程身份由包裹与决定版本确定性派生）。信封不带「谁是人工项」，本适配器也不判：
// 它只认自己能执行的两个动作。
type StartJourneyOnPostIntakeDisposition struct {
	starter  PostIntakeJourneyStarter
	existing ports.AlternateJourneyMemberView
}

func NewStartJourneyOnPostIntakeDisposition(
	starter PostIntakeJourneyStarter,
	existing ports.AlternateJourneyMemberView,
) (*StartJourneyOnPostIntakeDisposition, error) {
	if starter == nil {
		return nil, fmt.Errorf("post-intake disposition journey: starter is nil")
	}
	if existing == nil {
		return nil, fmt.Errorf("post-intake disposition journey: member view is nil")
	}
	return &StartJourneyOnPostIntakeDisposition{starter: starter, existing: existing}, nil
}

var _ tfinbox.PostIntakeDispositionHandler = (*StartJourneyOnPostIntakeDisposition)(nil)

const (
	// 两个字面是 PS 信封的拼写，由消费方自己写出（ADR-0025）。
	postIntakeOwnerTransportFulfillment = "TRANSPORT_FULFILLMENT"
	postIntakeActionStartAlternate      = "START_ALTERNATE_JOURNEY"
	postIntakeActionStartReturn         = "START_RETURN_JOURNEY"
	postIntakeServiceDispositionKind    = "SERVICE_DISPOSITION"
)

func (processing *StartJourneyOnPostIntakeDisposition) HandleDecidedPostIntakeDisposition(
	ctx context.Context,
	decided tfinbox.DecidedPostIntakeDisposition,
) error {
	purpose := domain.JourneyPurposeInvalid
	for _, request := range decided.Requests {
		if request.Owner != postIntakeOwnerTransportFulfillment {
			continue
		}
		switch request.Action {
		case postIntakeActionStartAlternate:
			purpose = domain.AlternateJourneyPurpose
		case postIntakeActionStartReturn:
			purpose = domain.ReturnJourneyPurpose
		}
	}
	if purpose == domain.JourneyPurposeInvalid {
		return nil
	}
	// 对运输履约，处置决定的依据种类恒为服务处置决定：监管退运经 UC-TF-001，不冒充（ADR-0332 决定三）。
	if decided.BasisKind != postIntakeServiceDispositionKind {
		return fmt.Errorf("%w: basis kind %q", ErrPostIntakeUntranslatable, decided.BasisKind)
	}

	tenant, err := domain.NewTenantID(decided.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrPostIntakeUntranslatable, err)
	}
	parcel, err := domain.NewCarriedObjectReference(decided.Parcel)
	if err != nil {
		return fmt.Errorf("%w: parcel: %v", ErrPostIntakeUntranslatable, err)
	}
	basis, err := domain.NewDispositionBasisReference(decided.BasisReference)
	if err != nil {
		return fmt.Errorf("%w: basis: %v", ErrPostIntakeUntranslatable, err)
	}

	// 原旅程：对象在决定那一刻走的那一趟，规则与实际履约发生项定旅程的同一条（ActualFulfillmentJourney，ADR-0218），没有旅程册记录时是派生项。
	// 以这份依据启动过的旅程不参与判断——重投时否则会把自己当原旅程，换了键而开出第二条旅程。
	records, err := processing.existing.FindByMember(ctx, tenant, parcel)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPostIntakeJourneyUndecided, err)
	}
	var earlier []domain.AlternateJourney
	for _, record := range records {
		if record.Key.Basis == basis {
			continue
		}
		earlier = append(earlier, record.Journey)
	}
	original, err := domain.ActualFulfillmentJourney(parcel, decided.DecidedAt, earlier)
	if err != nil {
		return fmt.Errorf("%w: original journey: %v", ErrPostIntakeUntranslatable, err)
	}

	// 新旅程身份由（包裹，决定版本）确定性派生：同一份决定重放落回同一条旅程，下一版决定是另一条。
	command := tfapp.StartAlternateJourneyCommand{
		TenantID:  tenant,
		Journey:   fmt.Sprintf("JRN/%s/PID-%d", decided.Parcel, decided.DecisionVersion),
		Purpose:   purpose,
		Original:  original.String(),
		BasisKind: domain.ServiceDispositionDecision,
		Basis:     decided.BasisReference,
		Members:   []string{decided.Parcel},
		StartedAt: decided.DecidedAt,
	}
	result, err := processing.starter.Handle(ctx, command)
	if err != nil {
		return err
	}
	switch result.Outcome() {
	case tfapp.JourneyStarted, tfapp.JourneyExistingResult:
		// 意图没交出去时整笔回滚重投：旅程与意图同笔落地，重投从头来过。
		if result.DispositionHandoffReference() != "" || result.ExceptionHandoffReference() != "" {
			return fmt.Errorf("%w: journey intents not handed off", ErrPostIntakeJourneyUndecided)
		}
		return nil
	case tfapp.JourneyUndecided:
		return fmt.Errorf("%w: %s", ErrPostIntakeJourneyUndecided, result.UndecidedReason())
	default:
		return fmt.Errorf("%w: outcome %s", ErrPostIntakeUntranslatable, result.Outcome())
	}
}
