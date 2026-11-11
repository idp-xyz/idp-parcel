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
	{"party-commercial/registration-number-types/AT", 1, "04edd2c49a01c8164d29a4f9e2c2c2273ef59edea857d2ae3480cc7cb7be11ca"},
	{"party-commercial/registration-number-types/BE", 1, "573883e3f3172c326c5bfe9b0bc4cf056f0c3241895f43914fa1513acb8f4b9c"},
	{"party-commercial/registration-number-types/BG", 1, "7431934e0b3b87bbaecf89f201fab28fbb3bb7b0a626527272815869b77add06"},
	{"party-commercial/registration-number-types/CN", 2, "92501c67bd5c1530a47f2f56456169661c2c3d8e4fae490cebc4550a5820f52e"},
	{"party-commercial/registration-number-types/CY", 1, "717735ad181b1d1edc258a699b96d45445e693d811a192507d62aab69d38c990"},
	{"party-commercial/registration-number-types/CZ", 1, "5e84c137dfef774f8df115fcca0d71ea7b39a9bfca55652c465145278d05e12d"},
	{"party-commercial/registration-number-types/DE", 1, "91b7b714cdffd8bbcc881d66ffe1c04a8a2c81dba43852227d8aea8e9a8c0475"},
	{"party-commercial/registration-number-types/DK", 1, "b91c7370f98cb291e363629227b5502e66629f17138fd195cf9e2344b5e815d5"},
	{"party-commercial/registration-number-types/EE", 1, "7c4395e4e1c9641a4b82b2956ab1a99bd557bd491f147e5374a2e39243805856"},
	{"party-commercial/registration-number-types/ES", 1, "e859fd5660e4a02bfbb51c2c6b59d675ef9b44519f7057fc377c7e0eb1984abd"},
	{"party-commercial/registration-number-types/FI", 1, "d68942b072f528185d4192dfda59f212255e098446edb33fffbde649c2cfe06e"},
	{"party-commercial/registration-number-types/FR", 1, "954cffdfd334cb81f7d02ef60ce51f12e825993ef0faef48130f4e635b7eeac1"},
	{"party-commercial/registration-number-types/GB", 1, "48eb42b90c3593e336212239c1a93b6f0d94f0ae35bec5f1d4d764f8f179cf9a"},
	{"party-commercial/registration-number-types/GR", 1, "2bb4e2284a4953a48626fb29d9a7f77fa2b1086f4cf50615aa3c3c205a462a0a"},
	{"party-commercial/registration-number-types/HK", 1, "b7042da531096674ab9d16b3c3246ae60b4b048a7f2a403e021a072310e3f15a"},
	{"party-commercial/registration-number-types/HR", 1, "16a51ef26045903a96a782f6512d8a9cf6c8f862b9143cc14d05c4157878495d"},
	{"party-commercial/registration-number-types/HU", 1, "d3fd20863261a41f9131f96b5be70facfc2a6ec3d758fbb319cf6ce0befdde96"},
	{"party-commercial/registration-number-types/IE", 1, "7dc4f202ecac82d464b05e36c4a7a765c95b987cd39a331e6bcbaf3ea9863156"},
	{"party-commercial/registration-number-types/IT", 1, "17811a72ebc0c01193d506b52ab0a6e449919f15a7fc43e82e7b909a2d394f37"},
	{"party-commercial/registration-number-types/JP", 1, "2c9a4291cdc742527d5b7e16b9923974b25b68cd5d3d594383a6af19159a3502"},
	{"party-commercial/registration-number-types/LT", 1, "82c82a01bdc13fac4b1e4fe8f6fb9bbc7e12c537b070cfe313cb1d2d2d49bd0d"},
	{"party-commercial/registration-number-types/LU", 1, "aef754a0dd6912fbd723e5110077551c7a462327ec5ca3d040c7b020227c7c9d"},
	{"party-commercial/registration-number-types/LV", 1, "91b0d2a9c9c89f4195632a7e1ec725471e7ef1ad6ccbff2019eb662bcfedb082"},
	{"party-commercial/registration-number-types/MT", 1, "6334b86659df6bc8893e77d8e3fd50ed78c320801bf3e9a5c5e3461a605573af"},
	{"party-commercial/registration-number-types/NL", 1, "48adba399146b10e38d9c5f93874e646e25e99304526bd361d105f0a9e32f119"},
	{"party-commercial/registration-number-types/PL", 1, "b59007e8a74b3ccaff3383742d04e45dbe867896671426905a86d2d400f1eb2d"},
	{"party-commercial/registration-number-types/PT", 1, "4c0b2b574bfd97dfe78687ff7d9f4556e2254a698ca00226bafee8a462d26837"},
	{"party-commercial/registration-number-types/RO", 1, "c33ba851a26a35c6e1c35f44a7e6db99c79aa6d13a0bf3cf6eceffca70898788"},
	{"party-commercial/registration-number-types/SE", 1, "cead42802ad9ca707ecc6a51efa2f46257a6e92caeacdbe3345c20c82de8363b"},
	{"party-commercial/registration-number-types/SG", 2, "235b56ddb10764278f4a15bba62490b6eca80c5aeb68326e8aa9ca983954ee28"},
	{"party-commercial/registration-number-types/SI", 1, "8cad3f39e10b99059f001045ca303b5bcb8eae5ba1c919f0b4a24d6e08cf9e30"},
	{"party-commercial/registration-number-types/SK", 1, "b891afd0b84686f937c7022cfbfe5a328cc6c79bed9e01c54f0e394b2b5d9f10"},
	{"party-commercial/registration-number-types/US", 1, "42ca69905eb38616291c5f0fbebefd7a4093ee0ae89bff02d3b3187aee81cae8"},
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
