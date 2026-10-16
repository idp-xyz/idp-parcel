package application

import (
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// carrierNaming 是一份证据怎样指名承运主体：给了引用（组得成主体），或只给了名称素材。首次有效收寄判断、实际
// 承运商判断与同依据重放都从这一处读，各处对「指名」的理解因此不会各说各话——只给素材时主体轴不参与比对（票
// operator-channel/20 判断项），正是靠这一格分出来的。
type carrierNaming struct {
	claimed     domain.CarrierSubject
	material    string
	byReference bool
}

// readCarrierNaming 只把命令上的三格读成一种指名方式，不问身份目录。第二个返回值为 false 即立不起来：引用与名称素材
// 都没给，或给了引用却与种类组不成主体——调用方一律答未受理。
func readCarrierNaming(kind domain.CarrierSubjectKind, reference, material string) (carrierNaming, bool) {
	material = strings.TrimSpace(material)
	if strings.TrimSpace(reference) == "" {
		if material == "" {
			return carrierNaming{}, false
		}
		return carrierNaming{material: material}, true
	}
	claimed, err := domain.NewCarrierSubject(kind, reference)
	if err != nil {
		return carrierNaming{}, false
	}
	return carrierNaming{claimed: claimed, material: material, byReference: true}, true
}
