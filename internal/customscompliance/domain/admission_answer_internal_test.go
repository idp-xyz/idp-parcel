package domain

import (
	"slices"
	"testing"
)

// Covers: 全集、valid、String 三者同一来源——全集里的每一格都有名、都 valid；不在全集里的取值没有名、也不 valid。String 仍是
// switch，这一条钉住它与全集不会各说各的。
func TestAdmissionAnswersValidAndStringAgree(t *testing.T) {
	inList := map[AdmissionAnswer]bool{}
	for _, answer := range AdmissionAnswers() {
		inList[answer] = true
	}
	for value := 0; value < 256; value++ {
		answer := AdmissionAnswer(value)
		if inList[answer] != answer.valid() || inList[answer] != (answer.String() != "") {
			t.Fatalf("取值 %d：在全集 %v、valid %v、名 %q", value, inList[answer], answer.valid(), answer.String())
		}
	}
	// 压制次序有两处在用：领域折叠按取值比大小，卡关读面按全集的列表次序挑最重的一份。两处一致只靠全集按取值升序排，
	// 这一句把它钉住——谁在全集里插错位置，两处就会对「哪一格更重」答出不同的话。
	if !slices.IsSorted(AdmissionAnswers()) {
		t.Fatalf("全集须按取值升序排，与领域折叠的压制次序一致：%v", AdmissionAnswers())
	}
}
