package parcelshipment

import (
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// PreAdvisedNumberGate 实现 noports.PreAdvisedNumberGate：问的就是 PS 的入册门 NewExternalIdentifierNumber。预报列的号要与关联册里
// 的号逐字比，两边的门必须是同一道——这里另写一份界，PS 那一界一改，就会出现预报收得进、关联册永远存不下（或反过来）的号。
//
// 走入册门而不走核对用号那一道（NewExternalIdentifierLookupNumber）：预报是写，号要落进本上下文的册；核对用号不设长度界，是为了让
// 门立起来之前已在册的长号还比得上，那个理由对新交来的预报不成立。
type PreAdvisedNumberGate struct{}

var _ noports.PreAdvisedNumberGate = PreAdvisedNumberGate{}

// AdmitPreAdvisedNumber 过门，过了才交回预报号码；号码原样，一个字符不折算。拒的格一律包成 ErrPreAdvisedNumberNotAdmitted，原因
// 附在后面供人看。
func (PreAdvisedNumberGate) AdmitPreAdvisedNumber(value string) (nodomain.PreAdvisedParcelNumber, error) {
	if _, err := psdomain.NewExternalIdentifierNumber(value); err != nil {
		return nodomain.PreAdvisedParcelNumber{}, fmt.Errorf("%w: %v", noports.ErrPreAdvisedNumberNotAdmitted, err)
	}
	number, err := nodomain.NewPreAdvisedParcelNumber(value)
	if err != nil {
		return nodomain.PreAdvisedParcelNumber{}, fmt.Errorf("%w: %v", noports.ErrPreAdvisedNumberNotAdmitted, err)
	}
	return number, nil
}
