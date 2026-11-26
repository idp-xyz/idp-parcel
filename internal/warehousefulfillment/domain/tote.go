package domain

// ToteCode 是周转箱上贴的码；拣货车上的格口也按一个箱登记，用它自己的码。逐字保存，在一个仓里唯一。
type ToteCode struct{ requiredValue }

func NewToteCode(value string) (ToteCode, error) {
	required, err := newRequiredValue("tote code", value)
	return ToteCode{required}, err
}
