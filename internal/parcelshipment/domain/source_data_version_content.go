package domain

// 本文件是 PS CONTEXT「客户原始资料版本」词条「版本另保留该资料范围上由本上下文定名的封闭要素内容」那一句的机制半边
// （pp-seams/05 裁决 1 / 2 / 3）：一份客户原始资料版本在它的资料范围上留下的内容，按范围只有一种形——申报测量范围留
// 一份 DeclaredMeasurement，寄件 / 收件两个地址范围各留一份 AddressElements。其余 name/value 条目照旧只进 PayloadDigest，
// 版本上不留原文。
//
// 只留封闭要素而不留整份条目：机制半边没有任何消费方读原文，实例半边（租户字段映到要素名）却要靠它长；「有消费方才留」
// 让每多留一格都要先有词条，与 AddressElementName 的封闭集同一把尺。封闭集要扩只走 CONTEXT 词条，不走「顺手多留」。

// SourceDataVersionContent 是一份客户原始资料版本携带的封闭要素内容。零值即「那一版上要素缺席」——补充与更正可以不带
// 内容（该范围上被改的也许是本上下文没有词条的条目），显式清空必须不带（清空就是那一版上要素缺席，两口的已采用格随之
// 如实答缺）。两格互斥由构造器保证：一份内容要么是测量、要么是要素、要么缺席。
type SourceDataVersionContent struct {
	measurement DeclaredMeasurement
	elements    AddressElements
}

// NewMeasurementContent 把一份有重量的申报测量包成测量范围的内容；没有重量的测量不是内容，拒。
func NewMeasurementContent(measurement DeclaredMeasurement) (SourceDataVersionContent, error) {
	if !measurement.declared() {
		return SourceDataVersionContent{}, ErrInvalidDeclaredMeasurement
	}
	return SourceDataVersionContent{measurement: measurement}, nil
}

// NewAddressElementsContent 把一段地址要素包成寄件 / 收件范围的内容。两格都缺的一段就是缺席，交回零值——「报了但两格
// 都空」与「没报」在读口上本来就是同一格。
func NewAddressElementsContent(elements AddressElements) SourceDataVersionContent {
	if elements.Empty() {
		return SourceDataVersionContent{}
	}
	return SourceDataVersionContent{elements: elements}
}

// Measurement 的第二个返回值为假即这份内容不是测量（要素或缺席）。
func (content SourceDataVersionContent) Measurement() (DeclaredMeasurement, bool) {
	return content.measurement, content.measurement.declared()
}

// AddressElements 的第二个返回值为假即这份内容不是地址要素（测量或缺席）。
func (content SourceDataVersionContent) AddressElements() (AddressElements, bool) {
	return content.elements, !content.elements.Empty()
}

// Empty 报告这份内容缺席。
func (content SourceDataVersionContent) Empty() bool {
	return !content.measurement.declared() && content.elements.Empty()
}

// FitsAmendment 回答这份内容能不能落在某个资料范围、某种意图的版本上（裁决 3 的构造门）：测量内容只落申报测量范围，
// 要素内容只落寄件 / 收件两个地址范围，缺席落任何范围；显式清空的版本只许缺席。
//
// 它单独暴露而不只藏在 FormCustomerSourceDataVersion 里，判据同 SourceDataScopeOutsideAcceptanceBaseline：编排要在
// 授权、规则与签发版本号之前就能问「这条命令本身成不成立」，而不是把一份注定不成立的命令跑到形成版本那一步才拒。
// 两处共用这一段判断，权威仍然只有一处。
func (content SourceDataVersionContent) FitsAmendment(scope SourceDataScope, intent AmendmentIntent) bool {
	if content.Empty() {
		return true
	}
	if intent == ExplicitClearIntent {
		return false
	}
	if _, measured := content.Measurement(); measured {
		return scope.DataGroup() == DeclaredMeasurementDataGroup()
	}
	return scope.DataGroup() == SenderPlaceDataGroup() || scope.DataGroup() == DeliveryPlaceDataGroup()
}
