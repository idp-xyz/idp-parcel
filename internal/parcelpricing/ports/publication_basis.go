package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 价卡发布的依据门。依据是调用方交来的不透明引用，本上下文不解释它是什么。
// 没装这只读口时，发布照旧，不设这道门。

type PublicationBasisStanding uint8

const (
	PublicationBasisStandingInvalid PublicationBasisStanding = iota
	// PublicationBasisAllow 不拦这次发布。没有发出记录、规则未登记、规则说不要求、或依据已经对上，都是这一格。
	PublicationBasisAllow
	// PublicationBasisAcceptanceRequired 规则要求一条有效接受，这次没带依据。
	PublicationBasisAcceptanceRequired
	// PublicationBasisCitationRejected 带了依据，但它不是这一版的一条有效接受。
	PublicationBasisCitationRejected
)

func (standing PublicationBasisStanding) String() string {
	switch standing {
	case PublicationBasisAllow:
		return "ALLOW"
	case PublicationBasisAcceptanceRequired:
		return "ACCEPTANCE_REQUIRED"
	case PublicationBasisCitationRejected:
		return "CITATION_REJECTED"
	default:
		return ""
	}
}

type PublicationBasisQuery struct {
	Tenant        domain.TenantID
	PlanID        string
	PlanVersion   string
	ContentDigest string
	Basis         string
}

type PublicationBasisReader interface {
	JudgePublicationBasis(ctx context.Context, query PublicationBasisQuery) (PublicationBasisStanding, error)
}

type PublicationBasisRecord interface {
	SavePublicationBasis(ctx context.Context, tenant domain.TenantID, planID, planVersion, basis string) error
}
