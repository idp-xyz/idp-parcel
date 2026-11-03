// Package referenceconfig 拥有随产品版本发布的参考配置（ADR-0147）：判断方法与公开标准的实例数据，
// 租户显式采用才生效（ADR-0146 决定三）。
//
// 放在仓库根而不是某个上下文包里，理由同 migrations：它是随产品版本发布的数据，`//go:embed` 取不到
// 包目录之外的文件；不可改写的发布清单与引用串的解析也因此只有一处。本包只管「有哪几版、原文是什么、
// 依据格里怎么引」，原文的形状与领域构造门归采用它的那本登记册的入口。
package referenceconfig

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 嵌入清单列上下文目录；新上下文的首份参考配置与本行、发布清单同一笔提交落——引用一个尚无文件的
// 目录会让干净检出编译不过，与 migrations 的嵌入行同一条纪律。
//
//go:embed all:party-commercial
//go:embed all:parcel-shipment
//go:embed all:network-routing
var assets embed.FS

var (
	ErrInvalidReference    = errors.New("reference configuration: invalid reference")
	ErrNotReleased         = errors.New("reference configuration: not released")
	ErrAlteredAfterRelease = errors.New("reference configuration: content differs from its released digest")
)

// citationShapeVersion 是引用串自身的形状版本（ADR-0147 决定三），与引用之间以冒号相接——写法同摘要串的
// `PSC-1`、`PCC-1`。串里要多带一格时换 REFCFG-2，旧串照旧可读。
const citationShapeVersion = "REFCFG-1"

// 标识恰三段：上下文 / 目录 / 键。上下文与目录是小写短横线名；键取该目录的自然键（如国家 / 地区码），
// 允许大写。
var identifierShape = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*/[a-z0-9]+(?:-[a-z0-9]+)*/[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$`)

// 版本从 1 起、不带前导零：同一版只有一种写法，依据格里才不会出现两串指同一份原文。
var versionShape = regexp.MustCompile(`^[1-9][0-9]*$`)

type release struct {
	identifier string
	version    int
	digest     string
}

// releases 是发布清单：每一版原文的 sha256。进 main 即发布，此后那份文件一字不改；更正或扩充另立
// 新版本，并在此加一行。
var releases = []release{
	{"party-commercial/registration-number-types/CN", 1, "46087f2adfc18dc277d4dc534c2254663de5e13f8415cc8b180a553cb0fa300c"},
	{"party-commercial/registration-number-types/SG", 1, "928d06b9ee6b96ded103aad5bc636c5e40e408a2620e67b026811cd0bb96767c"},
	{"parcel-shipment/as-of-semantics/submission-receipt", 1, "299a3e122322e3b44bef185ee42fd3a97991942efd866cf731f9d2493d6764b2"},
	{"network-routing/network-catalog/SYN-CN-SG", 1, "ad25370891505b9f5a11615f7206ed73539ca65f96e764da31daca7b5f0f05e5"},
	// ADR-0359 决定二：注册号类型参考表——CN、SG 各出第 2 版增列资料层类型，另加 HK、US、GB、JP 与欧盟各成员国；
	// 只收公开规范有明确格式的类型，其余不随附、不猜。
	{"party-commercial/registration-number-types/AT", 1, "cbe8fdd2247641356b1a0dbdcf991a27bf6e92b1d1bdcd560a4b98cc405b5051"},
	{"party-commercial/registration-number-types/BE", 1, "6da8a8bcac4df514b98fef3fa4803c4b4d6668916bc48245d16fb7eb5dbfec4c"},
	{"party-commercial/registration-number-types/BG", 1, "4f50ba35941ec35394c1e6711d4c03bf952e56c13d35f9ad70935f14bd9a8aeb"},
	{"party-commercial/registration-number-types/CN", 2, "9f11e82247b88f0b1ae2bf716e2f981fbefe16b92896092ba6287f261d151f02"},
	{"party-commercial/registration-number-types/CY", 1, "a90c6b914b26166f6a700ae0d3e30fcf4569068a2e33b391a20db1773be8e449"},
	{"party-commercial/registration-number-types/CZ", 1, "df19dd57caba454d391c5449ea73172dfd7112e90451025ccabf9255d54cade9"},
	{"party-commercial/registration-number-types/DE", 1, "51b6640a1448a5ad4619ea708a85d3cc8bbea27853272d70b13358492dc6ec06"},
	{"party-commercial/registration-number-types/DK", 1, "f3d60f8703acbcabfe0d10956b31f22a62eef2634a6d9ea332f0594e8da04b3f"},
	{"party-commercial/registration-number-types/EE", 1, "d32f8c3409a8babcecae57fe175729d66fbfc79210ef11929e2e662752c7fe6b"},
	{"party-commercial/registration-number-types/ES", 1, "17a61d1bf853412aeaae0c481ff57fd92416ce422e7d4cbb7e0bd3f8747894ee"},
	{"party-commercial/registration-number-types/FI", 1, "2f46d46318130e7ae76af308714dd9c74aaad9aeaf6229a3d0207ee2b5cc891f"},
	{"party-commercial/registration-number-types/FR", 1, "590dad0ef7faed21e4e8a6d5b68850f56e6f792c62009d92c1accc46cfc89765"},
	{"party-commercial/registration-number-types/GB", 1, "1deafb67786412962204188e85ebf82e6f1d1d8a704efaeed64a5178cc807450"},
	{"party-commercial/registration-number-types/GR", 1, "9123106eecac7399dc9bb92c0bf637eb1eb3bb888aebb14126018ce4b5754f9a"},
	{"party-commercial/registration-number-types/HK", 1, "bbbe3632a667a08c69e7747f17cf9e92baa229943ae1f1f2baa3ff75b5c7c161"},
	{"party-commercial/registration-number-types/HR", 1, "24e2c69c294bf34120d9b1f57a23f851a9608117c6ef5c0afa331e5038eb8e35"},
	{"party-commercial/registration-number-types/HU", 1, "30a4ebe80ba57c517c95a071278e8a72e03e7b1d2f96fdf08d0165c88b5024e3"},
	{"party-commercial/registration-number-types/IE", 1, "c08fd20050fe9b2473542d142a3a8627c91d0ace12f002160b67e755bec26ccb"},
	{"party-commercial/registration-number-types/IT", 1, "2a76d7aa71aedd5c917e168afc64aa438e9f779703b3be6a9490f825657d179e"},
	{"party-commercial/registration-number-types/JP", 1, "4f94d338e9364ba73695b917efca6dae4bf3a1691a4268550fba3a35d543261c"},
	{"party-commercial/registration-number-types/LT", 1, "ec4b58c3dbb6218999e1fa467a0c848ec3974ce637ff8d70a44ad04c80a24850"},
	{"party-commercial/registration-number-types/LU", 1, "5c86415118ef3e8a163d879376afd5cf4b92280eafe8b749a839230afe62e7a2"},
	{"party-commercial/registration-number-types/LV", 1, "8b2be53e47b2e56be7ae0172f35b710de0adc9bb3d5b3be583e29b6f197b110f"},
	{"party-commercial/registration-number-types/MT", 1, "0afbe80de1f523a9a9218ce7c0e7662a5dae628fc4be692b8860c0e99908b0e6"},
	{"party-commercial/registration-number-types/NL", 1, "be7ecfc4545f9a29f23d168f23bd258ea04a4430432ac02e1a2401e568765b42"},
	{"party-commercial/registration-number-types/PL", 1, "04925d77530a3c09dcdce5b9421c614b67df9d0ab75ded1963c4526983bef42a"},
	{"party-commercial/registration-number-types/PT", 1, "fdd35e39a88266fbd5e585339a2c074bf491d63ca66f26adb6c54258c32e7b48"},
	{"party-commercial/registration-number-types/RO", 1, "2be97b8a41ba09f401c79f0bf919e858544ee48edf8482b4c497d4aa1c18e683"},
	{"party-commercial/registration-number-types/SE", 1, "251eb4080cc1391ec385afc2d0ca2776854a18fe2622d352f745fe83e51fa7fe"},
	{"party-commercial/registration-number-types/SG", 2, "5216d287a0983351f079c3529d2055905bca2d26a229e262f0a83d93410a9e99"},
	{"party-commercial/registration-number-types/SI", 1, "4c6a9e78713cd34c875f5b7013f4565d5241df713a8d17acd567272d16d10078"},
	{"party-commercial/registration-number-types/SK", 1, "51d069005f781b61ae394929c933124ee269bb76e8c3b1f947c814d3e6eb789d"},
	{"party-commercial/registration-number-types/US", 1, "e8fe9ad61c70f792b72c9b9389fb734242cfb5c4e5370aa43fdb00298d469363"},
}

// Reference 指名一份参考配置的一个版本。
type Reference struct {
	identifier string
	version    int
}

func NewReference(identifier string, version int) (Reference, error) {
	if !identifierShape.MatchString(identifier) || version < 1 {
		return Reference{}, fmt.Errorf("%w: %q@%d", ErrInvalidReference, identifier, version)
	}
	return Reference{identifier: identifier, version: version}, nil
}

// ParseReference 读「标识@版本」。
func ParseReference(text string) (Reference, error) {
	identifier, version, found := strings.Cut(text, "@")
	if !found || !versionShape.MatchString(version) {
		return Reference{}, fmt.Errorf("%w: %q", ErrInvalidReference, text)
	}
	number, err := strconv.Atoi(version)
	if err != nil {
		return Reference{}, fmt.Errorf("%w: %q", ErrInvalidReference, text)
	}
	return NewReference(identifier, number)
}

// ParseCitation 从依据格反解引用串。不带前缀或形状不对的串答 false：依据格里别的依据照旧是别的依据，
// 不当成一次坏掉的采用。
func ParseCitation(text string) (Reference, bool) {
	rest, found := strings.CutPrefix(text, citationShapeVersion+":")
	if !found {
		return Reference{}, false
	}
	reference, err := ParseReference(rest)
	if err != nil {
		return Reference{}, false
	}
	return reference, true
}

// OpenCitation 处理声称是引用的串。不带前缀答 claimed=false，调用方当不透明值。
// 带前缀但形状不对，或该版未发布，claimed=true 且 err 非空：登记必须拒，不能当成另一个不透明串。
func OpenCitation(text string) (Reference, bool, error) {
	if !strings.HasPrefix(text, citationShapeVersion+":") {
		return Reference{}, false, nil
	}
	reference, ok := ParseCitation(text)
	if !ok {
		return Reference{}, true, fmt.Errorf("%w: %q", ErrInvalidReference, text)
	}
	if _, err := Open(reference); err != nil {
		return Reference{}, true, err
	}
	return reference, true, nil
}

func (reference Reference) Identifier() string {
	return reference.identifier
}

func (reference Reference) Version() int {
	return reference.version
}

func (reference Reference) String() string {
	return reference.identifier + "@" + strconv.Itoa(reference.version)
}

// Citation 交回写进登记依据格的引用串。
func (reference Reference) Citation() string {
	return citationShapeVersion + ":" + reference.String()
}

// Open 交回一份已发布参考配置的原文。未发布答 ErrNotReleased；原文与发布时的摘要不符答
// ErrAlteredAfterRelease——已采用的租户引的是发布时那一版，改过的原文不能冒它的名。
func Open(reference Reference) ([]byte, error) {
	for _, entry := range releases {
		if entry.identifier != reference.identifier || entry.version != reference.version {
			continue
		}
		raw, err := assets.ReadFile(reference.String() + ".json")
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrNotReleased, reference, err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != entry.digest {
			return nil, fmt.Errorf("%w: %s", ErrAlteredAfterRelease, reference)
		}
		return raw, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotReleased, reference)
}

// Released 按标识、版本列出全部已发布版本。
func Released() []Reference {
	references := make([]Reference, 0, len(releases))
	for _, entry := range releases {
		references = append(references, Reference{identifier: entry.identifier, version: entry.version})
	}
	sort.Slice(references, func(i, j int) bool {
		if references[i].identifier != references[j].identifier {
			return references[i].identifier < references[j].identifier
		}
		return references[i].version < references[j].version
	})
	return references
}
