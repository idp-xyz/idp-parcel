package main

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

// NoteMarker 之后到 Status 行末的原文，就是这份记录在索引里那一行的注记。
//
// 用一个显式标记而不从 Status 行里认「部分停用」之类的字样：Status 行同时写着本记录停用了谁
// （去向）与本记录被谁停用（来向），两种都叫「部分停用」，措辞各会话各写各的，靠字样认
// 必然把去向也搬进索引。标记是 HTML 注释，渲染时不显示，读 ADR 的人看到的仍是一句连贯的话。
const NoteMarker = "<!-- 索引注记 -->"

// Record 是一份 ADR 在索引里用得到的全部。
type Record struct {
	Number string // 四位号，与文件名前缀一致
	File   string
	Title  string // H1 里号之后的原文
	// SupersededBy 与 SupersederFile 取自 Status 行 `Superseded by` 之后那个链接；空即未被取代。
	SupersededBy   string
	SupersederFile string
	Note           string
	// ProxyTag 由 Status 行标记前段派生（见 proxyTag），不手写：代决与待谁复核只在前段写一次，
	// 索引要露出它就从那一处取，不另在注记里抄第二份。
	ProxyTag string
}

// AcceptedInNote 出现在注记里，就是把接受出处又说了一遍：接受出处只写在标记前段。
const AcceptedInNote = "**已接受**"

var (
	// 来向（后来哪份记录停用、收窄或回答了本记录）写进标记前段的写法：「（已）由 [ADR-…](…) 停用 /
	// 收窄 / 回答……」。只认这几个动词：前段的历史背景里「此前草案已由 ADR-… 否决」之类不是来向。去向（本记录停用了谁）写作「部分停用 [ADR-…]」
	// 「收窄 [ADR-…]」，主语是本记录、不带「由」，认不成来向。动词后面紧跟「的」是在转述背景
	// （「由 ADR-… 停用的那一句」），不是本记录被谁停用，放过；整份被取代的记录前段本来就是来向
	// （`Superseded by`），不查。这是查重的网，不是取注记的办法：注记仍只认标记，理由见 NoteMarker。
	incomingInFront = regexp.MustCompile(`由\s*\[ADR-\d{4}\]\([^)]*\)[^。；）\]]{0,24}?` +
		`(?:停用|收窄|回答|作答|改指|限定|取代|按口解|消解|补上|接上|扩成|改挂)(.?)`)
	// 头部停用行不进索引：读索引的人看不到它，它与注记写的又是同一件事。冒号全角半角都认。
	suspensionHeader = regexp.MustCompile(`^(Partially suspended by|Amended by)\s*[:：]`)

	// 代决标签的两个来源：前段写明「代决」「代裁」「代 X owner」即会话代 owner 裁定；
	// 「待 X owner 复核」取第一处，那是这份代决等谁复核。
	proxyDecided  = regexp.MustCompile(`代决|代裁|代\s*[A-Za-z][A-Za-z0-9 、与-]*?\s*owner`)
	pendingReview = regexp.MustCompile(`待\s*([^，。；）「」]*?)\s*owner\s*复核`)

	recordFile = regexp.MustCompile(`^(\d{4})-[^/]+\.md$`)
	// 冒号全角半角都收：早期记录的 H1 用半角加空格，后来统一成全角，正文不改，所以两种都在。
	recordHeading = regexp.MustCompile(`^# ADR-(\d{4})\s*[：:]\s*(.+?)\s*$`)
	supersededBy  = regexp.MustCompile(`^Superseded by \[ADR-(\d{4})\]\(\./(\d{4}-[^)]+\.md)\)`)
)

// ParseRecord 从一份 ADR 的全文取出索引要的东西。H1 必须是第一行，Status 行在第一个二级标题之前。
func ParseRecord(file, content string) (Record, error) {
	m := recordFile.FindStringSubmatch(file)
	if m == nil {
		return Record{}, fmt.Errorf("%s：文件名不是「四位号-名字.md」", file)
	}
	record := Record{Number: m[1], File: file}

	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	h := recordHeading.FindStringSubmatch(lines[0])
	if h == nil {
		return Record{}, fmt.Errorf("%s：第一行不是「# ADR-%s：标题」", file, record.Number)
	}
	if h[1] != record.Number {
		return Record{}, fmt.Errorf("%s：H1 写的是 ADR-%s，与文件名的号不符", file, h[1])
	}
	record.Title = h[2]
	// 标题原样进链接文字，方括号会把链接截断，而截断后的那一行渲染出来不报任何错。
	if strings.ContainsAny(record.Title, "[]") {
		return Record{}, fmt.Errorf("%s：H1 标题含方括号，进不了索引链接", file)
	}

	status, found := "", false
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "## ") {
			break
		}
		if h := suspensionHeader.FindStringSubmatch(line); h != nil {
			return Record{}, fmt.Errorf("%s：头部有「%s」行；部分停用写进 Status 行的索引注记，不另设头部行", file, h[1])
		}
		if strings.HasPrefix(line, "Status:") && !found {
			status, found = line, true
		}
	}
	if !found {
		return Record{}, fmt.Errorf("%s：第一个二级标题之前没有 Status 行", file)
	}

	body := status
	if at := strings.Index(status, NoteMarker); at >= 0 {
		body = status[:at]
		record.Note = strings.TrimRight(status[at+len(NoteMarker):], " \t")
		if strings.Contains(record.Note, NoteMarker) {
			return Record{}, fmt.Errorf("%s：Status 行里索引注记标记出现了不止一次", file)
		}
	}
	// 别的 HTML 注释多半是把标记写走了样（少个空格、换了字）；放过去，那句注记就静默地不进索引。
	if strings.Contains(body, "<!--") || strings.Contains(record.Note, "<!--") {
		return Record{}, fmt.Errorf("%s：Status 行里有不是「%s」的 HTML 注释", file, NoteMarker)
	}
	// 同一件事在一行里写两遍，两份措辞迟早分岔，读的人不知道哪份算数（票 infra-speed/07 的来由）。
	if !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(body, "Status:")), "Superseded") {
		for _, m := range incomingInFront.FindAllStringSubmatch(body, -1) {
			if m[1] == "的" {
				continue
			}
			return Record{}, fmt.Errorf("%s：Status 行标记前段写了来向「%s」；后来者的停用与回答只写在「%s」之后", file, m[0], NoteMarker)
		}
	}
	if strings.Contains(record.Note, AcceptedInNote) {
		return Record{}, fmt.Errorf("%s：索引注记里又写了一段「%s」；接受出处只写在标记前段", file, AcceptedInNote)
	}

	rest := strings.TrimSpace(strings.TrimPrefix(body, "Status:"))
	if strings.HasPrefix(rest, "Superseded") {
		s := supersededBy.FindStringSubmatch(rest)
		if s == nil {
			return Record{}, fmt.Errorf("%s：Status 写了 Superseded，后面却不是「Superseded by [ADR-NNNN](./NNNN-….md)」", file)
		}
		if s[1] != s[2][:4] {
			return Record{}, fmt.Errorf("%s：取代者链接文字 ADR-%s 与目标文件 %s 的号不符", file, s[1], s[2])
		}
		record.SupersededBy, record.SupersederFile = s[1], s[2]
	}
	record.ProxyTag = proxyTag(body)
	return record, nil
}

// proxyTag 从标记前段认出代决与待谁复核，交回索引行首的短标签；两样都认不出交回空串。
func proxyTag(front string) string {
	proxy := proxyDecided.MatchString(front)
	who := ""
	reviewed := false
	if m := pendingReview.FindStringSubmatch(front); m != nil {
		reviewed = true
		who = strings.TrimSpace(m[1])
	}
	pending := "待 owner 复核"
	if who != "" {
		// 照前段原样留空格：英文缩写前后空一格，中文直接相连（「待各册 owner 复核」）。
		sep := ""
		if who[0] < 0x80 {
			sep = " "
		}
		pending = "待" + sep + who + " owner 复核"
	}
	switch {
	case proxy && reviewed:
		return "〔代决·" + pending + "〕"
	case proxy:
		return "〔代决〕"
	case reviewed:
		return "〔" + pending + "〕"
	}
	return ""
}

// ReadRecords 读 fsys 根下全部 ADR，按号排。号重复、取代者不存在或指向自己都算错，不跳过。
func ReadRecords(fsys fs.FS) ([]Record, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var records []Record
	byNumber := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !recordFile.MatchString(entry.Name()) {
			continue
		}
		data, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		record, err := ParseRecord(entry.Name(), string(data))
		if err != nil {
			return nil, err
		}
		if other, dup := byNumber[record.Number]; dup {
			return nil, fmt.Errorf("ADR-%s 有两份文件：%s 与 %s", record.Number, other, record.File)
		}
		byNumber[record.Number] = record.File
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Number < records[j].Number })

	for _, record := range records {
		if record.SupersededBy == "" {
			continue
		}
		if record.SupersededBy == record.Number {
			return nil, fmt.Errorf("%s：取代者指向自己", record.File)
		}
		if byNumber[record.SupersededBy] != record.SupersederFile {
			return nil, fmt.Errorf("%s：取代者 %s 不是本目录里的文件", record.File, record.SupersederFile)
		}
	}
	return records, nil
}

// Render 交回两个列表的正文：未被取代的（含草案）一份，已被取代的一份，各按号排、一份一行。
func Render(records []Record) (accepted, superseded string) {
	var a, s strings.Builder
	for _, record := range records {
		if record.SupersededBy == "" {
			fmt.Fprintf(&a, "- %s[ADR-%s：%s](./%s)%s\n", record.ProxyTag, record.Number, record.Title, record.File, record.Note)
			continue
		}
		fmt.Fprintf(&s, "- %s[ADR-%s：%s](./%s) → 由 [ADR-%s](./%s) 取代%s\n",
			record.ProxyTag, record.Number, record.Title, record.File, record.SupersededBy, record.SupersederFile, record.Note)
	}
	return a.String(), s.String()
}
