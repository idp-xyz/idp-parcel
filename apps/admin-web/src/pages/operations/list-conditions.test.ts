import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  allStatuses,
  handoverNotePushedStatus,
  pageStatusChips,
  preAdvicePushedStatus,
  pushedStatusChips,
  pushedStatusFilters,
  reviewListConditions,
  rowsOnPage,
  transportPushedStatus,
  type PushedStatus,
} from './list-conditions';

const verdict: PushedStatus = { dimension: 'verdict', labels: { HANDED_OVER: '已交接', REFUSED: '拒收' } };

// Covers: 选了「全部」或这一册没有可下推的维时不带筛选维；选了一个词即那一维只带它；本册别的筛选维照带。
test('检索词、下推的胶囊与别的筛选维拼成一问的条件', () => {
  deepEqual(reviewListConditions('SYN', undefined, 'REFUSED'), { q: 'SYN', filters: {} });
  deepEqual(reviewListConditions('', verdict, allStatuses), { q: '', filters: {} });
  deepEqual(reviewListConditions('SYN', verdict, 'REFUSED'), { q: 'SYN', filters: { verdict: ['REFUSED'] } });
  deepEqual(reviewListConditions('', verdict, 'REFUSED', { booking: ['SYN-BK-1'] }), {
    q: '',
    filters: { booking: ['SYN-BK-1'], verdict: ['REFUSED'] },
  });
});

// Covers: 交接结果与总单两册的胶囊只出读口词表里的词——集外的词整问答 400。
test('下推的胶囊词取自读口词表', () => {
  deepEqual(Object.keys(transportPushedStatus['transport-handover']?.labels ?? {}).sort(), [
    'HANDED_OVER',
    'PENDING_CONFIRMATION',
    'REFUSED',
  ]);
  deepEqual(Object.keys(transportPushedStatus['carrier-master-document']?.labels ?? {}).sort(), ['IN_FORCE', 'REVOKED', 'SUPERSEDED']);
});

// Covers: 送站两册的胶囊只出读口布尔维的两个词，先「否」后「是」；「全部」不带这一维，选了一格即这一维只带它。
test('送站两册的胶囊下推成布尔维', () => {
  deepEqual(
    pushedStatusChips(preAdvicePushedStatus, allStatuses, () => undefined).map((chip) => [chip.id, chip.label]),
    [
      [allStatuses, '全部'],
      ['false', '未撤回'],
      ['true', '已撤回'],
    ],
  );
  deepEqual(
    pushedStatusChips(handoverNotePushedStatus, 'true', () => undefined).map((chip) => [chip.id, chip.label, chip.active]),
    [
      [allStatuses, '全部', false],
      ['false', '开放', false],
      ['true', '已关闭', true],
    ],
  );
  deepEqual(pushedStatusFilters(preAdvicePushedStatus, allStatuses), {});
  deepEqual(pushedStatusFilters(undefined, 'true'), {});
  deepEqual(pushedStatusFilters(preAdvicePushedStatus, 'true'), { withdrawn: ['true'] });
  deepEqual(pushedStatusFilters(handoverNotePushedStatus, 'false'), { closed: ['false'] });
});

// Covers: 下推的胶囊是「全部」加词表各词、不带计数；点选中的那格回到「全部」。
test('下推的胶囊不带计数，点选中的那格回到全部', () => {
  const picked: string[] = [];
  const chips = pushedStatusChips(verdict, 'REFUSED', (status) => picked.push(status));
  deepEqual(
    chips.map((chip) => [chip.id, chip.label, chip.active, chip.count]),
    [
      [allStatuses, '全部', false, undefined],
      ['HANDED_OVER', '已交接', false, undefined],
      ['REFUSED', '拒收', true, undefined],
    ],
  );
  chips[2].onClick();
  chips[1].onClick();
  chips[0].onClick();
  deepEqual(picked, [allStatuses, 'HANDED_OVER', allStatuses]);
});

// Covers: 只筛本页的胶囊词取这一页上出现过的（去重、去空），选中的词翻到没有它的页也留着、好点掉。
test('只筛本页的胶囊取页上的词，选中的词不在页上也留着', () => {
  const chips = pageStatusChips(['已撤回', '未撤回', '', '未撤回'], allStatuses, () => undefined);
  // 词序按中文排序规则排，依运行时的 ICU；这里只钉去重、去空与「全部」打头。
  equal(chips[0].id, allStatuses);
  deepEqual(new Set(chips.map((chip) => chip.id)), new Set([allStatuses, '已撤回', '未撤回']));
  equal(chips.length, 3);
  const kept = pageStatusChips(['未撤回'], '已撤回', () => undefined);
  deepEqual(
    kept.map((chip) => [chip.id, chip.active]),
    [
      [allStatuses, false],
      ['未撤回', false],
      ['已撤回', true],
    ],
  );
});

// Covers: 页内再筛——状态只在给了 statusOf 时才筛；筛选框搜行上转写好的全部格（名称、中文词都在里面）。
test('到手的这一页上按状态与筛选框再筛', () => {
  const rows = [
    { key: 'a', values: { node: '合成一号站', state: '开放' } },
    { key: 'b', values: { node: '合成二号站', state: '已关闭' } },
  ];
  const stateOf = (row: (typeof rows)[number]) => row.values.state;
  deepEqual(rowsOnPage(rows, '').map((row) => row.key), ['a', 'b']);
  deepEqual(rowsOnPage(rows, '二号').map((row) => row.key), ['b']);
  deepEqual(rowsOnPage(rows, '', '开放', stateOf).map((row) => row.key), ['a']);
  deepEqual(rowsOnPage(rows, '', '开放').map((row) => row.key), ['a', 'b']);
  deepEqual(rowsOnPage(rows, '一号', '已关闭', stateOf), []);
});
