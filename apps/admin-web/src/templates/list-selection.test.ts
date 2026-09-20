import { test } from 'node:test';
import { deepEqual, equal, notEqual } from 'node:assert/strict';
import {
  clearSelection,
  retainSelectedRows,
  rowsToCsv,
  selectedOnPage,
  selectedRows,
  toggleAllOnPage,
  toggleSelected,
} from './list-selection';

// 本文件钉的是列表页多选与「导出所选」的纯逻辑（票 admin-web-workspace-form/04）。模板本体依赖 ESM-only 的 @idpxyz 原语，
// run-tests 的 CommonJS 发射加载不了它（票 admin-web-ux-alignment/05 完成记录有实测），要钉的规则抬到 list-selection.ts 用这里钉。

// Covers: 单行切换是纯函数——不在则加、在则减，且返回新集合、入参原样不动：模板把 next 交给调用方的 onChange，
// 调用方拿 ReadonlySet 当 state，原地改会绕过 React 的相等判断。
test('toggleSelected：不在则加、在则减，不改入参', () => {
  const before = new Set(['a']);
  const added = toggleSelected(before, 'b');
  deepEqual([...added], ['a', 'b']);
  const removed = toggleSelected(added, 'a');
  deepEqual([...removed], ['b']);
  deepEqual([...before], ['a']);
  notEqual(added, before);
});

// Covers: 表头复选的语义是「本页」——本页尚未全选时补齐本页，本页已全选时只退掉本页；别页攒下的选中两种情形都原样保留
// （票面：翻页保留）。本页为空时什么也不做。
test('toggleAllOnPage：本页未全选则补齐、已全选则退掉本页，别页选中保留', () => {
  const page = ['p1', 'p2', 'p3'];
  const other = 'q9';
  const none = new Set([other]);
  deepEqual([...toggleAllOnPage(none, page)], [other, 'p1', 'p2', 'p3']);
  const some = new Set([other, 'p2']);
  deepEqual([...toggleAllOnPage(some, page)], [other, 'p2', 'p1', 'p3']);
  const all = new Set([other, 'p1', 'p2', 'p3']);
  deepEqual([...toggleAllOnPage(all, page)], [other]);
  deepEqual([...toggleAllOnPage(all, [])], [...all]);
  deepEqual([...all], [other, 'p1', 'p2', 'p3']);
});

// Covers: 表头复选三态只看本页：无 / 部分 / 全；别页的选中不参与判定（否则翻到一页空白页会显「部分」）；本页为空为「无」。
test('selectedOnPage：三态只按本页判', () => {
  const page = ['p1', 'p2'];
  equal(selectedOnPage(new Set(), page), 'none');
  equal(selectedOnPage(new Set(['q9']), page), 'none');
  equal(selectedOnPage(new Set(['p1', 'q9']), page), 'some');
  equal(selectedOnPage(new Set(['p1', 'p2']), page), 'all');
  equal(selectedOnPage(new Set(['p1', 'p2']), []), 'none');
});

// Covers: 「取消选择」给一个空的新集合，而不是复用某个常量——调用方可能把它当可变 state 继续 add。
test('clearSelection：空的新集合', () => {
  const a = clearSelection();
  const b = clearSelection();
  equal(a.size, 0);
  notEqual(a, b);
});

interface Row {
  id: string;
  name: string;
}

const rowKey = (row: Row) => row.id;

// Covers: 选中行的记忆随选中集修剪——当前可见的行以最新数据刷新、翻走的行按记忆保留（导出所选要拿到别页的行）、
// 取消选中的键从记忆里删掉、从未见过的键不会凭空造行。
test('retainSelectedRows：可见的刷新、翻走的保留、取消的删掉、没见过的不造', () => {
  const memory = new Map<string, Row>([
    ['a', { id: 'a', name: 'old-a' }],
    ['b', { id: 'b', name: 'b' }],
    ['gone', { id: 'gone', name: 'gone' }],
  ]);
  const next = retainSelectedRows(
    memory,
    new Set(['a', 'b', 'never']),
    [{ id: 'a', name: 'new-a' }, { id: 'c', name: 'c' }],
    rowKey,
  );
  deepEqual(
    [...next.entries()],
    [
      ['a', { id: 'a', name: 'new-a' }],
      ['b', { id: 'b', name: 'b' }],
    ],
  );
  equal(memory.size, 3);
});

// Covers: 导出的行序照选中的先后（Set 的插入序），不照页面行序——用户攒的顺序就是他要的顺序；记忆里没有的键跳过而不是塞空行。
test('selectedRows：按选中先后取行，记忆里没有的跳过', () => {
  const memory = new Map<string, Row>([
    ['a', { id: 'a', name: 'a' }],
    ['b', { id: 'b', name: 'b' }],
  ]);
  deepEqual(selectedRows(memory, new Set(['b', 'never', 'a'])), [
    { id: 'b', name: 'b' },
    { id: 'a', name: 'a' },
  ]);
});

interface Col {
  id: string;
  header: unknown;
}

const columns: Col[] = [
  { id: 'id', header: '标识' },
  { id: 'name', header: '名称' },
  { id: 'actions', header: '操作' },
];

// Covers: 首字符是 UTF-8 BOM（Excel 只认它才按 UTF-8 解中文）；表头取 header 文本；调用方对某列一律给不出文本的列整列跳过
// （render 出的是 ReactNode，抠不出字，票面「没给的列跳过」）；记录以 CRLF 分隔（RFC 4180 2.1）。
test('rowsToCsv：BOM 开头、表头取文本、整列无文本的列跳过、CRLF 分行', () => {
  const rows: Row[] = [
    { id: '1', name: '甲' },
    { id: '2', name: '乙' },
  ];
  const csv = rowsToCsv(rows, columns, (row, column) =>
    column.id === 'actions' ? undefined : row[column.id as keyof Row],
  );
  equal(csv.charCodeAt(0), 0xfeff);
  equal(csv.slice(1), '标识,名称\r\n1,甲\r\n2,乙\r\n');
});

// Covers: RFC 4180 2.6 / 2.7——含逗号、双引号、换行（LF 与 CRLF 都算）的字段要用双引号包住，字段里的双引号写成两个；
// 不含这些字符的字段不加引号（Excel 与 RFC 都接受裸字段，加了反而让别的工具读成带引号的字面量）。
test('rowsToCsv：逗号、双引号、换行三种字段加引号，双引号成对', () => {
  const rows: Row[] = [
    { id: 'a,b', name: '说"引"话' },
    { id: 'line1\nline2', name: 'crlf\r\nhere' },
    { id: 'plain', name: '' },
  ];
  const csv = rowsToCsv(rows, columns.slice(0, 2), (row, column) => row[column.id as keyof Row]);
  equal(
    csv.slice(1),
    '标识,名称\r\n"a,b","说""引""话"\r\n"line1\nline2","crlf\r\nhere"\r\nplain,\r\n',
  );
});

// Covers: 某列只在部分行给不出文本时列仍保留、那几格写空——整列跳过与格内为空是两件事，前者是列的性质，后者是那一行的数据；
// 零行时只出表头（此时列的性质无从判，全部保留）；非文本的 header（ReactNode）退回列 id，不猜节点里的字。
test('rowsToCsv：部分行无文本写空格、零行只出表头、非文本表头退回列 id', () => {
  const rows: Row[] = [
    { id: '1', name: 'x' },
    { id: '2', name: 'y' },
  ];
  const partial = rowsToCsv(rows, columns.slice(0, 2), (row, column) =>
    column.id === 'name' && row.id === '2' ? undefined : row[column.id as keyof Row],
  );
  equal(partial.slice(1), '标识,名称\r\n1,x\r\n2,\r\n');
  const empty = rowsToCsv([] as Row[], columns, () => undefined);
  equal(empty.slice(1), '标识,名称,操作\r\n');
  const nodeHeader = rowsToCsv(rows, [{ id: 'name', header: { type: 'span' } }], (row) => row.name);
  equal(nodeHeader.slice(1), 'name\r\nx\r\ny\r\n');
});
