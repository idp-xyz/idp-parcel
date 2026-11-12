package cataloguepage

import (
	"fmt"
	"sort"
	"strings"
)

// Columns 把一册声明的各维落到读面的列上（ADR-0144 决定七「各册只声明自己的排序维、筛选维、q 覆盖的列」）。列名由
// 读面给，常是它自己 CTE 里导出的列；共用件只拿它们拼片段，不认任何表。
//
// 可排维与行标识的列必须非空：键集比较遇到 NULL 得 NULL，那一行会从后面各页静默消失。可为空的列不放出作可排维，或在
// 读面里先导出成非空列。q 覆盖的列须是文本列。
type Columns struct {
	// Sorts 按可排维名给列，须恰好覆盖本册声明的全部可排维。
	Sorts map[string]string
	// Identity 按 Spec.Identity 的段序给列。
	Identity []string
	// Filters 按筛选维名给列，须恰好覆盖本册声明的全部筛选维。
	Filters map[string]string
	// Codes 按筛选维给「词 → 列里存的整数代码」的换算，可缺；缺席的维按原词比较。状态这类维在登记表里常存成代码，
	// 有了它不必为了筛选在读面里另导一列文本。
	Codes map[string]func(string) (int64, error)
	// MultiValued 点名多值的筛选维（ADR-0378 决定四）：这一维落到的列是文本数组，一行可以同时属于几个值，所给的值里有
	// 一个在数组里即匹配；分面时一行记进它属于的每一格。多值维不带代码换算。
	MultiValued []string
	// Keyword 是 q 覆盖的文本列，至少一列：q 是保留键、Decode 总会收下，没落列的话检索词就被静默忽略了——
	// 正是决定四拒收未知键要防的那种「屏上的条件与答出的列表对不上」。
	Keyword []string
}

// Binding 是校验过的一册「声明 + 落列」，由 Catalogue.Bind 构造。
type Binding struct {
	catalogue *Catalogue
	columns   Columns
}

// Bind 校验一册的落列：声明的每一维都落了列、没有多出来的列、标识段数与声明一致、q 至少一列。落列是各册写死的常量，
// 这里报错即编程错误，应在装配时就暴露。
func (catalogue *Catalogue) Bind(columns Columns) (*Binding, error) {
	fail := func(format string, arguments ...any) (*Binding, error) {
		return nil, fmt.Errorf("cataloguepage: 册 %s 的落列：%s", catalogue.name, fmt.Sprintf(format, arguments...))
	}
	for _, name := range sortedKeys(catalogue.sorts) {
		if strings.TrimSpace(columns.Sorts[name]) == "" {
			return fail("可排维 %s 没有落到列上", name)
		}
	}
	for _, name := range sortedKeys(columns.Sorts) {
		if _, declared := catalogue.sorts[name]; !declared {
			return fail("可排维 %s 不在声明里", name)
		}
	}
	if len(columns.Identity) != len(catalogue.identity) {
		return fail("行标识应有 %d 段列，实有 %d 段", len(catalogue.identity), len(columns.Identity))
	}
	for index, column := range columns.Identity {
		if strings.TrimSpace(column) == "" {
			return fail("行标识第 %d 段的列为空", index+1)
		}
	}
	for _, name := range sortedKeys(catalogue.filters) {
		if strings.TrimSpace(columns.Filters[name]) == "" {
			return fail("筛选维 %s 没有落到列上", name)
		}
	}
	for _, name := range sortedKeys(columns.Filters) {
		if _, declared := catalogue.filters[name]; !declared {
			return fail("筛选维 %s 不在声明里", name)
		}
	}
	for _, name := range sortedKeys(columns.Codes) {
		if _, declared := catalogue.filters[name]; !declared {
			return fail("代码换算的筛选维 %s 不在声明里", name)
		}
		if columns.Codes[name] == nil {
			return fail("筛选维 %s 的代码换算为空", name)
		}
	}
	for _, name := range columns.MultiValued {
		if _, declared := catalogue.filters[name]; !declared {
			return fail("多值的筛选维 %s 不在声明里", name)
		}
		if columns.Codes[name] != nil {
			return fail("多值的筛选维 %s 不能带代码换算", name)
		}
	}
	// 分面按列里存的词分组：存成代码的列分出来的是代码，对不上词表。要给这样的维分面，先在读面里导出一列词。
	for _, name := range catalogue.facets {
		if columns.Codes[name] != nil {
			return fail("给分面的筛选维 %s 不能带代码换算", name)
		}
	}
	if len(columns.Keyword) == 0 {
		return fail("q 没有覆盖任何列")
	}
	for index, column := range columns.Keyword {
		if strings.TrimSpace(column) == "" {
			return fail("q 覆盖的第 %d 列为空", index+1)
		}
	}
	return &Binding{catalogue: catalogue, columns: copyColumns(columns)}, nil
}

// MustBind 是 Bind 的包级声明写法：落列不合规即 panic。
func (catalogue *Catalogue) MustBind(columns Columns) *Binding {
	binding, err := catalogue.Bind(columns)
	if err != nil {
		panic(err)
	}
	return binding
}

// Plan 是一次取页要的 SQL 片段与参数，由 Binding.Plan 拼出。总数与取页共用 Where 与 Args 的前段，total 才与本页出自同一组
// 条件（ADR-0144 决定五）。
type Plan struct {
	// Where 是读面给的固定条件与上筛选维与 q，不含键集。
	Where string
	// Args 与 Where 配套：读面已占用的参数在前，筛选维与 q 的参数接在后面。
	Args []any
	// PageWhere 是 Where 再与上键集比较；第一页时与 Where 相同。
	PageWhere string
	// OrderBy 是排序维加各段标识，全部同向（ADR-0144 决定三）。
	OrderBy string
	// Limit 是页大小那个参数的占位符。它的值是 limit+1：多取的一行只用来判有没有下一页（见 Finish）。
	Limit string
	// PageArgs 与 PageWhere、Limit 配套。
	PageArgs []any
}

// CountSQL 是总数那一句。source 是 FROM 之后的取数来源（表名或 CTE 导出名）；要 WITH 前半句的，读面接在前面。
func (plan Plan) CountSQL(source string) string {
	return "SELECT count(*) FROM " + source + " WHERE " + plan.Where
}

// PageSQL 是取页那一句，selection 是要取的列。
func (plan Plan) PageSQL(selection, source string) string {
	return "SELECT " + selection + " FROM " + source + " WHERE " + plan.PageWhere +
		" ORDER BY " + plan.OrderBy + " LIMIT " + plan.Limit
}

// Plan 按一次查询拼出总数与取页要的片段。base 是读面自己的固定条件（如 tenant_id = $1），空即没有；args 是 base 与读面
// 前半句已占用的参数，新参数从其后编号。查询对象须由本册的 Decode 解出；limit 非正是读面的编程错误。
func (binding *Binding) Plan(query Query, base string, args []any, limit int) (Plan, error) {
	name := binding.catalogue.name
	if query.catalogue != binding.catalogue {
		return Plan{}, fmt.Errorf("cataloguepage: 查询对象不是册 %s 的 Decode 解出来的", name)
	}
	if limit < 1 {
		return Plan{}, fmt.Errorf("cataloguepage: 册 %s 的页大小须为正，得 %d", name, limit)
	}
	sortColumn, mapped := binding.columns.Sorts[query.Sort.Field]
	if !mapped {
		return Plan{}, fmt.Errorf("cataloguepage: 册 %s 的排序维 %q 没有落到列上", name, query.Sort.Field)
	}

	where, bound, err := binding.conditions(query, base, args, "")
	if err != nil {
		return Plan{}, err
	}
	placeholder := func(value any) string {
		bound = append(bound, value)
		return fmt.Sprintf("$%d", len(bound))
	}

	plan := Plan{Where: where}
	plan.Args = append([]any(nil), bound...)

	keys := append([]string{sortColumn}, binding.columns.Identity...)
	direction, comparison := "ASC", ">"
	if query.Sort.Descending {
		direction, comparison = "DESC", "<"
	}

	plan.PageWhere = plan.Where
	if query.After != nil {
		kinds := append([]ValueKind{binding.catalogue.sorts[query.Sort.Field]}, binding.catalogue.identity...)
		values := append([]string{query.After.Value}, query.After.Identity...)
		if len(values) != len(keys) {
			return Plan{}, fmt.Errorf("cataloguepage: 册 %s 的游标位置有 %d 格，排序键有 %d 列", name, len(values), len(keys))
		}
		placeholders := make([]string, len(values))
		for index, value := range values {
			typed, err := typedValue(kinds[index], value)
			if err != nil {
				return Plan{}, fmt.Errorf("cataloguepage: 册 %s 的游标位置第 %d 格：%w", name, index+1, err)
			}
			placeholders[index] = placeholder(typed)
		}
		plan.PageWhere += fmt.Sprintf(" AND (%s) %s (%s)",
			strings.Join(keys, ", "), comparison, strings.Join(placeholders, ", "))
	}

	order := make([]string, len(keys))
	for index, column := range keys {
		order[index] = column + " " + direction
	}
	plan.OrderBy = strings.Join(order, ", ")
	plan.Limit = placeholder(limit + 1)
	plan.PageArgs = bound
	return plan, nil
}

// conditions 把读面的固定条件、各筛选维与 q 拼成一句 WHERE（没有任何条件时是 TRUE），连同参数交回；不含键集。
// except 非空时跳过那一维——分面数某一维各值的行数时，条件不含该维自己。
func (binding *Binding) conditions(query Query, base string, args []any, except string) (string, []any, error) {
	name := binding.catalogue.name
	bound := append([]any(nil), args...)
	placeholder := func(value any) string {
		bound = append(bound, value)
		return fmt.Sprintf("$%d", len(bound))
	}

	var conditions []string
	if strings.TrimSpace(base) != "" {
		// 括起来：base 里若有 OR，与后面各条 AND 在一起时优先级会把它拆开。
		conditions = append(conditions, "("+base+")")
	}
	// 按维名顺序拼，同一组条件每次拼出同一句，参数编号也稳定。
	for _, dimension := range sortedKeys(query.Filters) {
		if dimension == except {
			continue
		}
		column := binding.columns.Filters[dimension]
		values := query.Filters[dimension]
		if binding.multiValued(dimension) {
			// 数组有交集即匹配：维内为或（ADR-0144 决定四），一行的数组里有所给的任一个值就算。
			conditions = append(conditions, fmt.Sprintf("%s && %s", column, placeholder(append([]string(nil), values...))))
			continue
		}
		code := binding.columns.Codes[dimension]
		if code == nil {
			conditions = append(conditions, fmt.Sprintf("%s = ANY(%s)", column, placeholder(append([]string(nil), values...))))
			continue
		}
		codes := make([]int64, 0, len(values))
		for _, value := range values {
			converted, err := code(value)
			if err != nil {
				return "", nil, fmt.Errorf("cataloguepage: 册 %s 筛选维 %s 的取值 %q 换不成代码：%w", name, dimension, value, err)
			}
			codes = append(codes, converted)
		}
		conditions = append(conditions, fmt.Sprintf("%s = ANY(%s)", column, placeholder(codes)))
	}
	if query.Keyword != "" {
		keyword := placeholder("%" + likeLiteral.Replace(query.Keyword) + "%")
		matches := make([]string, len(binding.columns.Keyword))
		for index, column := range binding.columns.Keyword {
			matches[index] = column + " ILIKE " + keyword
		}
		conditions = append(conditions, "("+strings.Join(matches, " OR ")+")")
	}
	if len(conditions) == 0 {
		return "TRUE", bound, nil
	}
	return strings.Join(conditions, " AND "), bound, nil
}

func (binding *Binding) multiValued(dimension string) bool {
	for _, name := range binding.columns.MultiValued {
		if name == dimension {
			return true
		}
	}
	return false
}

// FacetPlan 是数一个分面维要的那一句与参数（ADR-0378 决定五），由 Binding.FacetPlans 拼出。
type FacetPlan struct {
	// Dimension 是筛选维的名字，也是答复里 facets 下的键。
	Dimension string
	// Args 与 SQL 配套：读面已占用的参数在前，其余各维与 q 的参数接在后面。
	Args []any

	column      string
	where       string
	multiValued bool
	words       []string
}

// SQL 是数各值行数的那一句，答两列：值与行数。source 是 FROM 之后的取数来源（同 Plan.CountSQL）；要 WITH 前半句的，
// 读面接在前面。列里没出现的值不在结果里，由 Counts 补零。
func (plan FacetPlan) SQL(source string) string {
	if plan.multiValued {
		// 多值维按数组里的每个值各记一次：一行属于几个值，就数进几格。
		return "SELECT facet_value, count(*) FROM " + source + ", unnest(" + plan.column + ") AS facet_value WHERE " +
			plan.where + " GROUP BY facet_value"
	}
	return "SELECT " + plan.column + " AS facet_value, count(*) FROM " + source + " WHERE " + plan.where +
		" GROUP BY facet_value"
}

// Counts 把取回的「值 → 行数」排成答复里的那一格：照词表次序列出词表上的每个值，没取到的记零。词表之外的值不列——
// 分面答的是筛选维的词表，列里多出来的东西不是这一维能筛的。
func (plan FacetPlan) Counts(found map[string]int64) []FacetCount {
	counts := make([]FacetCount, 0, len(plan.words))
	for _, word := range plan.words {
		counts = append(counts, FacetCount{Value: word, Count: found[word]})
	}
	return counts
}

// FacetPlans 按一次查询给本册点名的各分面维各拼一句：条件取本次的 q 与其余各筛选维，不含该维自己——点下某一格会得到
// 几行，那一格上就是几（ADR-0378 决定五）。没点名分面的册交回空切片。base 与 args 同 Plan。
func (binding *Binding) FacetPlans(query Query, base string, args []any) ([]FacetPlan, error) {
	if query.catalogue != binding.catalogue {
		return nil, fmt.Errorf("cataloguepage: 查询对象不是册 %s 的 Decode 解出来的", binding.catalogue.name)
	}
	plans := make([]FacetPlan, 0, len(binding.catalogue.facets))
	for _, dimension := range binding.catalogue.facets {
		where, bound, err := binding.conditions(query, base, args, dimension)
		if err != nil {
			return nil, err
		}
		plans = append(plans, FacetPlan{
			Dimension:   dimension,
			Args:        bound,
			column:      binding.columns.Filters[dimension],
			where:       where,
			multiValued: binding.multiValued(dimension),
			words:       binding.catalogue.words[dimension],
		})
	}
	return plans, nil
}

// likeLiteral 让 q 在 ILIKE 里只做字面包含：% 与 _ 是通配符，反斜杠是 LIKE 的缺省转义符。
var likeLiteral = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// typedValue 把游标里规整过的一格串换回列的 Go 值，参数才按列的类型比较，而不是拿文本去比时刻或整数。
func typedValue(kind ValueKind, value string) (any, error) {
	switch kind {
	case Instant:
		return ParseInstant(value)
	case Integer:
		return ParseInteger(value)
	case Text:
		return value, nil
	default:
		return nil, fmt.Errorf("值形状 %q 未知", kind)
	}
}

func sortedKeys[Value any](entries map[string]Value) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func copyColumns(columns Columns) Columns {
	copied := Columns{
		Sorts:    make(map[string]string, len(columns.Sorts)),
		Identity: append([]string(nil), columns.Identity...),
		Filters:  make(map[string]string, len(columns.Filters)),
		Codes:    make(map[string]func(string) (int64, error), len(columns.Codes)),
		Keyword:  append([]string(nil), columns.Keyword...),
	}
	copied.MultiValued = append([]string(nil), columns.MultiValued...)
	for key, value := range columns.Sorts {
		copied.Sorts[key] = value
	}
	for key, value := range columns.Filters {
		copied.Filters[key] = value
	}
	for key, value := range columns.Codes {
		copied.Codes[key] = value
	}
	return copied
}
