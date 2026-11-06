package cataloguepage

import (
	"fmt"
	"math"
)

// Enum 是领域里「零值无效、各格 String() 给出词」的那种枚举，登记表里存它的数值。
type Enum interface {
	~uint8
	String() string
}

// Words 按数值次序列出一个领域枚举各格的词，作筛选维的词表：词的唯一来源是领域枚举的 String()，声明里不另抄一份，
// 领域加一格时词表跟着加。整段数值都扫一遍，不假设各格连续。
func Words[State Enum]() []string {
	var words []string
	for value := 1; value <= math.MaxUint8; value++ {
		if word := State(value).String(); word != "" {
			words = append(words, word)
		}
	}
	return words
}

// Code 把词表上的一个词换成枚举的数值，作 Columns.Codes 的换算。空串先拒：没有词的数值格 String() 也答空串，
// 不拒就会换成其中一格。
func Code[State Enum](word string) (int64, error) {
	if word != "" {
		for value := 1; value <= math.MaxUint8; value++ {
			if State(value).String() == word {
				return int64(value), nil
			}
		}
	}
	return 0, fmt.Errorf("%q 不是这个枚举的词", word)
}
