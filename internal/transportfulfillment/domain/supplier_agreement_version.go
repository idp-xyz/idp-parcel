package domain

// SupplierAgreementVersion 是登记方指名的一版供应商商业协议：协议对象与版本标签两格（票 demo-chain-gaps/22）。
//
// 两格都是 party-commercial 的身份，版本生命周期归它（CONTEXT Boundaries：本上下文保存采用的协议快照，不修改
// 商业版本），本上下文只搬运不解读。分两格收而不收一个拼好的串：「对象/版本」两段式指称只许 PC 一处拼
// （ADR-0080 决定七），本上下文在入口拼一遍，PC 那一处改了分隔符这里就静静对不上。委托与发生项上落的
// AgreementSnapshotReference 由核对口从 PC 那一处取回（ports.SupplierAgreementCheck），不在这里合成。
type SupplierAgreementVersion struct {
	object  requiredValue
	version requiredValue
}

func NewSupplierAgreementVersion(object, version string) (SupplierAgreementVersion, error) {
	objectValue, err := newRequiredValue("supplier agreement object", object)
	if err != nil {
		return SupplierAgreementVersion{}, err
	}
	versionValue, err := newRequiredValue("supplier agreement version label", version)
	if err != nil {
		return SupplierAgreementVersion{}, err
	}
	return SupplierAgreementVersion{object: objectValue, version: versionValue}, nil
}

// Object 是协议对象在 party-commercial 上的身份。
func (agreement SupplierAgreementVersion) Object() string {
	return agreement.object.String()
}

// Version 是该协议的版本标签。
func (agreement SupplierAgreementVersion) Version() string {
	return agreement.version.String()
}
