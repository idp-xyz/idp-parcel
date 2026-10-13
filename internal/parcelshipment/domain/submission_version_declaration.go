package domain

// SubmissionVersionDeclaration 是一份提交版本上客户申报的计价要素：逐件的申报测量，与寄、收两段邮编（ADR-0179 决定四）。
//
// 它按提交版本取，不经接受基线：受理前估价发生在接受之前，没有基线可走；提交版本是不可覆盖的记录（ADR-0048 决定二随它保全
// 画像），答的就是那一版上客户报了什么。客户原始资料的修订只在接受之后形成，这一版上没有可采用的修订，所以资料版本锚不参与。
// 接受之后的正式评价仍走 DeclaredMeasurementFor / AddressElementsFor 那两条经基线的路，本值不替它们作答。
//
// 两段邮编是委托级的：CONTEXT 硬句「同一委托中的包裹必须共享……寄收件关系」，寄收件资料按委托级范围形成，各件共用这一对；
// 测量按件申报，缺席如实。
type SubmissionVersionDeclaration struct {
	version     SubmissionVersionID
	parcels     []DeclaredParcelDeclaration
	origin      AddressElements
	destination AddressElements
}

// DeclaredParcelDeclaration 是一件声明包裹在这一版上的申报测量。客户没报测量即缺席，不拿同委托别件的测量顶，也不填默认。
type DeclaredParcelDeclaration struct {
	parcel      DeclaredParcelID
	measurement DeclaredMeasurement
}

func (declaration DeclaredParcelDeclaration) Parcel() DeclaredParcelID {
	return declaration.parcel
}

// Measurement 的第二个返回值为假即这一版上没有这件的画像。
func (declaration DeclaredParcelDeclaration) Measurement() (DeclaredMeasurement, bool) {
	return declaration.measurement, declaration.measurement.declared()
}

func (declaration SubmissionVersionDeclaration) Version() SubmissionVersionID {
	return declaration.version
}

// Parcels 交回逐件申报的拷贝，顺序随这一版的声明成员。
func (declaration SubmissionVersionDeclaration) Parcels() []DeclaredParcelDeclaration {
	return append([]DeclaredParcelDeclaration(nil), declaration.parcels...)
}

// OriginPostalCode 是寄件资料范围上申报的邮编；第二个返回值为假即没报。值原样交，格式属实例半边（ADR-0109），本上下文不校验。
func (declaration SubmissionVersionDeclaration) OriginPostalCode() (string, bool) {
	return declaration.origin.PostalCode()
}

// DestinationPostalCode 是收件资料范围上申报的邮编，交法同 OriginPostalCode。
func (declaration SubmissionVersionDeclaration) DestinationPostalCode() (string, bool) {
	return declaration.destination.PostalCode()
}

// SubmissionVersionDeclarationFor 按提交版本标识答那一版上的申报。版本不在本委托上答 false——当前版本与此前各版都认，取哪一版
// 由调用方指名：一次控制请求钉的是它当时判断的那一版，换代之后那一版的申报仍是真话。
func (request ShipmentRequest) SubmissionVersionDeclarationFor(versionID SubmissionVersionID) (SubmissionVersionDeclaration, bool) {
	version, found := request.submissionVersionByID(versionID)
	if !found {
		return SubmissionVersionDeclaration{}, false
	}
	members := version.DeclaredParcelIDs()
	parcels := make([]DeclaredParcelDeclaration, 0, len(members))
	for _, member := range members {
		declaration := DeclaredParcelDeclaration{parcel: member}
		if profile, declared := version.ProfileFor(member); declared {
			declaration.measurement = profile.Measurement()
		}
		parcels = append(parcels, declaration)
	}
	return SubmissionVersionDeclaration{
		version:     version.VersionID(),
		parcels:     parcels,
		origin:      version.addressElements(SenderPlaceDataGroup()),
		destination: version.addressElements(DeliveryPlaceDataGroup()),
	}, true
}
