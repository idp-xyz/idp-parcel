package visibilityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrOutsideAdmissionScope：索赔提交在铸造侧判准入范围（ADR-0139 决定八），这个账户不在准入范围内（403）——租户运营去登记准入范围。
	ErrOutsideAdmissionScope = errors.New("visibility http: customer account is outside the admission scope")
	// ErrClaimReferenceNotFound：载荷指名的目标包裹或合同责任范围不属于信封账户，或根本不在册（404）。两种情形同答这一格，
	// 别的账户的引用因此不泄露存在；两个引用也不分格，不说是哪一个不对。
	ErrClaimReferenceNotFound = errors.New("visibility http: claim reference not found for this customer account")
)

const (
	codeOutsideAdmissionScope  = "OUTSIDE_ADMISSION_SCOPE"
	codeClaimReferenceNotFound = "CLAIM_REFERENCE_NOT_FOUND"
)

// CustomerClaimAuthenticator 认证一次首方索赔出示，交回（租户，客户账户），都取自客户渠道册上的绑定。accessRequestID 在令牌、册、授予与
// 准入范围都过了之后才被调用，认证没过的调用方看不到载荷校验的结果；失败只交回本包的格。
type CustomerClaimAuthenticator interface {
	AuthenticateClaim(
		ctx context.Context,
		credential PresentedCustomerCredential,
		accessRequestID func() (string, error),
	) (CustomerViewer, error)
}

// CustomerClaimTargets 回答索赔目标包裹当前归属哪个账户，即 ports.ParcelCustomerAccountView。
type CustomerClaimTargets = ports.ParcelCustomerAccountView

// CustomerClaimIntake 是首方客户渠道在 `/v1/claims` 的 Intake（ADR-0139 决定四；票 first-party-api/09、demo-chain-gaps/102）。
//
// 租户与客户账户只取自认证结果；载荷封闭（CustomerClaimShapeFields），自报账户、租户或操作者都落在「未知字段」一类答坏请求。
// 提交时间取到达信封的时钟，不取客户自报（首次索赔期限从可证明的提交事实起算）。
//
// 引用核对在翻译命令之前：目标包裹须属信封账户（PS 的当前已接受委托归属，经 ports.ParcelCustomerAccountView；多份委托声明同一
// 包裹按「不属于」，不择一），合同责任范围须登记在信封账户名下（ports.ClaimContractOwnership）。不属于、不存在同答
// ErrClaimReferenceNotFound。申请人不在这里核：授权名单本就按（租户、账户）分，由资格审核的授权维核，不重复拦。
type CustomerClaimIntake struct {
	authenticator CustomerClaimAuthenticator
	targets       CustomerClaimTargets
	contracts     ports.ClaimContractOwnership
	clock         ports.Clock
}

var _ ClaimIntake = (*CustomerClaimIntake)(nil)

func NewCustomerClaimIntake(
	authenticator CustomerClaimAuthenticator,
	targets CustomerClaimTargets,
	contracts ports.ClaimContractOwnership,
	clock ports.Clock,
) (*CustomerClaimIntake, error) {
	if authenticator == nil || targets == nil || contracts == nil || clock == nil {
		return nil, errors.New("visibility http: customer claim intake needs an authenticator, a target view, a contract ownership view and a clock")
	}
	return &CustomerClaimIntake{authenticator: authenticator, targets: targets, contracts: contracts, clock: clock}, nil
}

// claimDraft 是 VCL-1 的载荷。
type claimDraft struct {
	AccessRequestID string `json:"accessRequestId"`
	Batch           string `json:"batch"`
	Item            string `json:"item"`
	Applicant       string `json:"applicant"`
	Contract        string `json:"contract"`
	Target          string `json:"target"`
	Kind            string `json:"kind"`
}

func (intake *CustomerClaimIntake) IntakeClaim(ctx context.Context, request *http.Request) (application.ReceiveClaimCommand, error) {
	var draft claimDraft
	identity, err := intake.authenticator.AuthenticateClaim(ctx, presentedCustomerCredential(request), func() (string, error) {
		if err := decodeClaimDraft(request.Body, &draft); err != nil {
			return "", err
		}
		return draft.AccessRequestID, nil
	})
	if err != nil {
		return application.ReceiveClaimCommand{}, err
	}

	malformed := func(err error) (application.ReceiveClaimCommand, error) {
		return application.ReceiveClaimCommand{}, fmt.Errorf("%w: %v", ErrMalformedClaim, err)
	}
	batch, err := domain.NewClaimBatchReference(draft.Batch)
	if err != nil {
		return malformed(err)
	}
	item, err := domain.NewClaimItemID(draft.Item)
	if err != nil {
		return malformed(err)
	}
	applicant, err := domain.NewApplicantReference(draft.Applicant)
	if err != nil {
		return malformed(err)
	}
	contract, err := domain.NewContractScopeReference(draft.Contract)
	if err != nil {
		return malformed(err)
	}
	target, err := domain.NewRequestScopeReference(draft.Target)
	if err != nil {
		return malformed(err)
	}
	kind, err := domain.NewClaimKindReference(draft.Kind)
	if err != nil {
		return malformed(err)
	}

	if err := intake.checkReferences(ctx, identity, contract, target); err != nil {
		return application.ReceiveClaimCommand{}, err
	}
	return application.ReceiveClaimCommand{
		TenantID:  identity.Tenant,
		Batch:     batch,
		Item:      item,
		Customer:  identity.Customer,
		Applicant: applicant,
		Contract:  contract,
		Target:    target,
		Kind:      kind,
		// 截到微秒：库存 timestamptz 只到微秒，首答与重放读回的提交时间才逐字节同。
		SubmittedAt: intake.clock.Now().UTC().Truncate(time.Microsecond),
	}, nil
}

// checkReferences 核目标与合同属于信封账户。依赖读不动原样上抛（5xx，运维救依赖），不折成「找不到」：那会把一次故障说成
// 「这件不归你」。
func (intake *CustomerClaimIntake) checkReferences(
	ctx context.Context,
	identity CustomerViewer,
	contract domain.ContractScopeReference,
	target domain.RequestScopeReference,
) error {
	parcel, err := domain.NewTrackedParcelReference(target.String())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedClaim, err)
	}
	owner, attributed, err := intake.targets.FindCustomerAccount(ctx, identity.Tenant, parcel)
	switch {
	case errors.Is(err, ports.ErrAmbiguousCustomerAccount):
		return ErrClaimReferenceNotFound
	case err != nil:
		return fmt.Errorf("customer claim intake: target ownership: %w", err)
	case !attributed || owner != identity.Customer:
		return ErrClaimReferenceNotFound
	}
	owned, err := intake.contracts.ContractBelongsToCustomer(ctx, identity.Tenant, identity.Customer, contract)
	if err != nil {
		return fmt.Errorf("customer claim intake: contract ownership: %w", err)
	}
	if !owned {
		return ErrClaimReferenceNotFound
	}
	return nil
}

// decodeClaimDraft：封闭解码，只许一个 JSON 对象、其后无尾随内容。
func decodeClaimDraft(body io.Reader, draft *claimDraft) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(draft); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedClaim, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", ErrMalformedClaim)
	}
	return nil
}
