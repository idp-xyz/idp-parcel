package architecture

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParcelPricingHasNoPartyCommercialAdapter 守 ADR-0260「认价卡」在依赖图上的那一半：评价按范围从 parcel-pricing
// 自己的价卡登记册选卡，体积系数、金额取整与换算序列只从卡上读，不读商业价格政策的正文，也不读已解析商业依据（决定一
// 至三）。读法落在依赖图上就是 parcel-pricing 没有一只伸向 party-commercial 的适配器：跨上下文翻译只能放在
// internal/parcelpricing/adapters/partycommercial（TestBusinessModulesDoNotReachIntoEachOther），这个位置一旦出现，政策
// 口径就有了进评价的路，而 AT-PP-013 的「政策另有口径不改变结果」只靠两条评价路的输入面上没有政策的位置成立——用例不会
// 因为有人加了这只适配器而变红。
//
// 汇率口径的声明仍在商业价格政策上，由序列取值登记的口径引用指名（决定三）；那是一份版本引用，不读政策正文，不需要这只
// 适配器。决定四的重启条件成立、改走绑定选卡时，随那份新记录删掉本门禁。
func TestParcelPricingHasNoPartyCommercialAdapter(t *testing.T) {
	t.Parallel()

	position := filepath.Join(repositoryRoot(t), "internal", "parcelpricing", "adapters", "partycommercial")
	if _, err := os.Stat(position); err == nil {
		t.Errorf("internal/parcelpricing/adapters/partycommercial 存在：评价只从价卡读计价条款（ADR-0260 决定一至三），不读商业价格政策")
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("查 %s：%v", position, err)
	}

	pricing := modulePath + "/internal/parcelpricing/"
	commercial := modulePath + "/internal/partycommercial"
	for _, file := range selectSources(t, "parcel-pricing 不读 party-commercial", func(pkg string) bool {
		return strings.HasPrefix(pkg+"/", pricing)
	}) {
		for _, imported := range file.imports {
			if imported == commercial || strings.HasPrefix(imported, commercial+"/") {
				t.Errorf("%s：parcel-pricing 导入 %q；评价不读商业价格政策与已解析商业依据（ADR-0260 决定一至三）", file.path, imported)
			}
		}
	}
}
