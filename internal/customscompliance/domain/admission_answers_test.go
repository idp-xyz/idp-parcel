package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// Covers: AdmissionAnswers 交出的全集与封闭五格同一份——读面按 Admits 从全集里挑去库里筛，漏一格或多一格都会让卡关的件数悄悄走样。
func TestAdmissionAnswersListsEveryAnswerOnceAndOnlyAdmittedAndNotGuardedAdmit(t *testing.T) {
	seen := map[string]bool{}
	admits := 0
	for _, answer := range domain.AdmissionAnswers() {
		if answer.String() == "" || seen[answer.String()] {
			t.Fatalf("全集里有无名或重复的一格：%v", answer)
		}
		seen[answer.String()] = true
		if answer.Admits() {
			admits++
		}
	}
	for value := 0; value < 256; value++ {
		if name := domain.AdmissionAnswer(value).String(); name != "" && !seen[name] {
			t.Fatalf("有一格不在全集里：%s", name)
		}
	}
	if !domain.AdmissionAdmitted.Admits() || !domain.AdmissionNotGuarded.Admits() || admits != 2 {
		t.Fatalf("只有放行与不受门禁过门，数到 %d 格", admits)
	}
}
