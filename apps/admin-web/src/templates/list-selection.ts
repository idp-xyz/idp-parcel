// 列表页多选与「导出所选」的纯逻辑（票 admin-web-workspace-form/04；蓝图 10.5「多选行 → 露出 Bulk Action Bar」、10.7 批量动作，
// idp-ui@6751fb2 `docs/oms_ui_ux_blueprint_v_1.md`）。抽成纯函数与 list-page-structure.ts 同一个理由：模板本体是 .tsx、依赖 ESM-only
// 的 @idpxyz 原语，run-tests 的 CommonJS 发射加载不了它，要钉的规则抬到这里用 node:test 钉。
//
// 选中集的生命周期：按 rowKey 记、翻页保留、筛选变更不自动清——用户可能在跨筛选攒一批再一起导出；换模块（组件卸载）即丢。
// 它是 UI 瞬态，不进 saved-views：保存视图存的是「筛选态」，选中集是对某一批具体对象的临时圈定，两者保质期差几个量级。
//
// 「复选列点击不算行交互」为什么不并进 list-page-structure.ts 的 rowInteraction：那个函数回答的是「调用方接了哪些行回调 → 行的
// tabIndex 与指针样式」，是每张表算一次的静态判读；「点复选不触发 onRowClick / onRowOpen」是逐个事件的边界，落点是复选格上
// 的 stopPropagation，没有可供纯函数计算的输入，只能在组件层（一次性 esbuild 束）验，这里没有它对应的函数。

/** 表头复选的三态：本页无一选中 / 部分选中 / 全部选中。 */
export type PageSelectionState = 'none' | 'some' | 'all';

/** 切换一行：不在则加、在则减。返回新集合，入参不动——调用方把它当 React state，原地改会绕过相等判断。 */
export function toggleSelected(selected: ReadonlySet<string>, key: string): Set<string> {
  const next = new Set(selected);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  return next;
}

/**
 * 表头复选的动作，语义是「本页」：本页尚未全选就补齐本页，已全选就只退掉本页；别页攒下的选中两种情形都保留。
 * 不做「全选所有页」——隔离读口 `isolatedReadLimit` 有上限且分页在服务端，全量选中是假的（票面「不做」）。
 */
export function toggleAllOnPage(selected: ReadonlySet<string>, pageKeys: readonly string[]): Set<string> {
  const next = new Set(selected);
  if (selectedOnPage(selected, pageKeys) === 'all') {
    for (const key of pageKeys) next.delete(key);
  } else {
    for (const key of pageKeys) next.add(key);
  }
  return next;
}

/** 表头复选该显哪一态。只看本页：别页的选中不算，否则翻到一页空白页也会显「部分」；本页为空是「无」。 */
export function selectedOnPage(selected: ReadonlySet<string>, pageKeys: readonly string[]): PageSelectionState {
  if (pageKeys.length === 0) return 'none';
  let hit = 0;
  for (const key of pageKeys) if (selected.has(key)) hit += 1;
  if (hit === 0) return 'none';
  return hit === pageKeys.length ? 'all' : 'some';
}

/** 「取消选择」。每次给新的空集合，不复用常量——调用方可能把它当可变 state 继续 add。 */
export function clearSelection(): Set<string> {
  return new Set();
}

/**
 * 选中行的记忆：键 → 行。选中集只记键，而「导出所选」要拿到行；翻页之后被翻走的行模板手里已经没有，所以在它可见时记下来。
 * 每次渲染按当前选中集修剪：可见的行以最新数据刷新、翻走的按记忆保留、取消选中的删掉、从未见过的键不凭空造行
 * （调用方预置了模板没见过的键时，导出会少那几行，「已选 N 项」的 N 仍按选中集报——两个数不同时以选中集为准，不补假行）。
 */
export function retainSelectedRows<Row>(
  memory: ReadonlyMap<string, Row>,
  selected: ReadonlySet<string>,
  visibleRows: readonly Row[],
  rowKey: (row: Row) => string,
): Map<string, Row> {
  const visible = new Map<string, Row>();
  for (const row of visibleRows) visible.set(rowKey(row), row);
  const next = new Map<string, Row>();
  for (const key of selected) {
    const row = visible.get(key) ?? memory.get(key);
    if (row !== undefined) next.set(key, row);
  }
  return next;
}

/** 按选中的先后（Set 的插入序）取出行——用户攒的顺序就是他要的顺序；记忆里没有的键跳过而不是塞空行。 */
export function selectedRows<Row>(memory: ReadonlyMap<string, Row>, selected: ReadonlySet<string>): Row[] {
  const rows: Row[] = [];
  for (const key of selected) {
    const row = memory.get(key);
    if (row !== undefined) rows.push(row);
  }
  return rows;
}

/** CSV 只用得到列的这两格；不引 ListColumn 本体，免得纯逻辑文件反向依赖 .tsx。 */
export interface CsvColumn {
  id: string;
  /** 模板里是 ReactNode；只有文本（string / number）能进 CSV 表头，其它退回列 id。 */
  header: unknown;
}

/**
 * 取字函数由调用方给：列的 `render` 出的是 ReactNode，CSV 不能从节点里抠字。对某列回 undefined 表示「这一格没有文本」；
 * 一列在全部待导出行上都没有文本就整列跳过（如动作列），只有部分行没有则那几格写空。
 */
export type CsvCellText<Row, Column extends CsvColumn> = (row: Row, column: Column) => string | undefined;

// UTF-8 BOM。Excel 打开 .csv 时只有见到它才按 UTF-8 解，否则中文按本地代码页读成乱码；其它读者按 Unicode 规范忽略它。
const utf8Bom = '\ufeff';

/**
 * 选中行 → CSV 文本（RFC 4180）：记录以 CRLF 分隔（2.1），含逗号、双引号或换行的字段用双引号包住（2.6），字段内的双引号写成两个（2.7）；
 * 不含这些字符的字段不加引号。首字符是 UTF-8 BOM。零行时只出表头，此时「整列有没有文本」无从判，列全部保留。
 */
export function rowsToCsv<Row, Column extends CsvColumn>(
  rows: readonly Row[],
  columns: readonly Column[],
  cellText: CsvCellText<Row, Column>,
): string {
  const cells = rows.map((row) => columns.map((column) => cellText(row, column)));
  const kept = columns
    .map((column, index) => ({ column, index }))
    .filter(({ index }) => rows.length === 0 || cells.some((line) => line[index] !== undefined));
  const header = kept.map(({ column }) => csvField(headerText(column)));
  const lines = [header, ...cells.map((line) => kept.map(({ index }) => csvField(line[index] ?? '')))];
  return utf8Bom + lines.map((line) => line.join(',') + '\r\n').join('');
}

function headerText(column: CsvColumn): string {
  const { header } = column;
  return typeof header === 'string' || typeof header === 'number' ? String(header) : column.id;
}

function csvField(text: string): string {
  return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
}
