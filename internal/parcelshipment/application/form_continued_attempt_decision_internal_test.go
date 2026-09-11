package application

import (
	"testing"
)

// TestClosureResponsibilitySourceComposesAndParsesBackToTheSameKind 钉合成 / 解析这一对（票 label-channel/30 裁决 ②，
// 票 label-channel/36 条 8）：每一个有名字的种类合成落册引用之后，解析得回同一个种类；主体两端的空白在合成时去掉，
// 解析不受它影响。放在包内是因为两个函数都不导出——它们是`关闭责任来源`编码的实现细节，导出只为测试会让调用方拿到
// 一个不该由它拼的串。
func TestClosureResponsibilitySourceComposesAndParsesBackToTheSameKind(t *testing.T) {
	t.Parallel()

	for _, kind := range []ClosureResponsibilitySourceKind{
		ShipperInstructionResponsibilitySource,
		OperatorActionResponsibilitySource,
		ExternalRestrictionResponsibilitySource,
	} {
		source, err := closureResponsibilitySourceOf(kind, "  SYN-SUBJECT-1 ")
		if err != nil {
			t.Fatalf("%s：合成失败：%v", kind, err)
		}
		if got := closureResponsibilitySourceKindOf(source); got != kind {
			t.Fatalf("%s：合成 %q 后解析回 %s", kind, source, got)
		}
		if source.String() != kind.String()+closureResponsibilitySourceSeparator+"SYN-SUBJECT-1" {
			t.Fatalf("%s：落册串 %q 没按 <种类>/<主体> 拼、或主体空白没去", kind, source)
		}
	}
}

// TestClosureResponsibilitySourceRefusesWhatWouldNotParseBack 钉对偶的另一半：没有名字的种类或空白主体合成不出引用
// ——那样一份引用落册后解析不回种类，重开那道门就核不了原关闭是不是货主指令形成的。与 FormControlledClosure 把这两格
// 折成`输入未受理 / RESPONSIBILITY_SOURCE_UNKNOWN`同一条判据。
func TestClosureResponsibilitySourceRefusesWhatWouldNotParseBack(t *testing.T) {
	t.Parallel()

	if _, err := closureResponsibilitySourceOf(ClosureResponsibilitySourceKindInvalid, "SYN-SUBJECT-1"); err == nil {
		t.Fatal("没有名字的种类合成出了引用")
	}
	if _, err := closureResponsibilitySourceOf(ShipperInstructionResponsibilitySource, "   "); err == nil {
		t.Fatal("空白主体合成出了引用")
	}
}
