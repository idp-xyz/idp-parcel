package domain

import "math/big"

// RecordedTruncatedWeightConversionForTest 造一条「按 places 位向零截断」的重量换算记录，给外部测试包的冻结用例用：
// 它代表当初某个构建按另一版位数公式记下的换算——生产路径只会按当前公式记，要造出位数与当前公式不同的记录，
// 只能走这里。仅测试编译，不进生产构建。
func RecordedTruncatedWeightConversionForTest(quantity ConvertedQuantity, from, to, factor, original string, places uint32) UnitConversionStep {
	rate, ok := parseCanonicalFactor(factor)
	if !ok {
		panic("系数不是规范写法：" + factor)
	}
	value, err := ParseDecimal(original)
	if err != nil {
		panic(err)
	}
	product := new(big.Rat).Mul(decimalRat(value), rate)
	truncated, ok := ratTruncate(product, places)
	if !ok {
		panic("截断失败")
	}
	return UnitConversionStep{
		quantity: quantity, from: from, to: to, factor: factor,
		original: []Decimal{value}, converted: []Decimal{truncated}, places: places,
	}
}
