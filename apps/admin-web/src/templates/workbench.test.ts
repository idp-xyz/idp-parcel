import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { listRatioCap, scrollDeltaToReveal, stepSelection, toggleWorkbenchSort, workbenchDetailMinWidth, workbenchKeyAction } from './workbench';

// Covers: 同列翻方向，换列从降序起。
test('toggleWorkbenchSort：同列翻方向、换列从降序起', () => {
  deepEqual(toggleWorkbenchSort({ key: 'registered', dir: -1 }, 'registered'), { key: 'registered', dir: 1 });
  deepEqual(toggleWorkbenchSort({ key: 'registered', dir: 1 }, 'registered'), { key: 'registered', dir: -1 });
  deepEqual(toggleWorkbenchSort({ key: 'registered', dir: 1 }, 'id'), { key: 'id', dir: -1 });
});

// Covers: 输入框与弹层里一个键都不占；页签条里让出方向键与 `/`、Esc 照旧收详情；Esc 只在详情开着时算数；其余键不认。
test('workbenchKeyAction：按焦点位置让键，Esc 看详情开没开', () => {
  equal(workbenchKeyAction('/', 'typing', false), null);
  equal(workbenchKeyAction('Escape', 'typing', true), null);
  equal(workbenchKeyAction('Escape', 'overlay', true), null);
  equal(workbenchKeyAction('ArrowDown', 'overlay', true), null);
  equal(workbenchKeyAction('ArrowDown', 'tablist', true), null);
  equal(workbenchKeyAction('/', 'tablist', true), null);
  equal(workbenchKeyAction('Escape', 'tablist', true), 'close-detail');
  equal(workbenchKeyAction('/', 'free', false), 'focus-search');
  equal(workbenchKeyAction('Escape', 'free', true), 'close-detail');
  equal(workbenchKeyAction('Escape', 'free', false), null);
  equal(workbenchKeyAction('ArrowDown', 'free', false), 'select-next');
  equal(workbenchKeyAction('ArrowUp', 'free', false), 'select-previous');
  equal(workbenchKeyAction('Enter', 'free', true), null);
});

// Covers: 无选中时下落首行、上落末行；到头不绕回；选中行被筛掉同无选中；空表 null。
test('stepSelection：起点、到头与被筛掉的选中', () => {
  const keys = ['a', 'b', 'c'];
  equal(stepSelection(keys, null, 'select-next'), 'a');
  equal(stepSelection(keys, null, 'select-previous'), 'c');
  equal(stepSelection(keys, 'a', 'select-next'), 'b');
  equal(stepSelection(keys, 'c', 'select-next'), 'c');
  equal(stepSelection(keys, 'a', 'select-previous'), 'a');
  equal(stepSelection(keys, 'gone', 'select-next'), 'a');
  equal(stepSelection([], 'a', 'select-next'), null);
});

// Covers: 详情栏至少留出最小宽度；容器还没量到时不封顶；容器比最小宽度还窄时列表让到零。
test('listRatioCap：右边详情栏留出最小宽度', () => {
  equal(listRatioCap(0, workbenchDetailMinWidth), 80);
  equal(listRatioCap(1000, 480), 52);
  equal(listRatioCap(400, 480), 0);
  equal(listRatioCap(2000, 480), 76);
});

// Covers: 表头粘在顶上，滚回被它挡住的行要多滚出一个表头；行在表头之下且没出底边时不动；出了底边只补那一截。
test('scrollDeltaToReveal：顶上让出表头，看得见的行不动', () => {
  equal(scrollDeltaToReveal({ listTop: 100, listBottom: 500, rowTop: 40, rowBottom: 80, header: 36 }), 40 - (100 + 36));
  equal(scrollDeltaToReveal({ listTop: 100, listBottom: 500, rowTop: 140, rowBottom: 180, header: 36 }), 0);
  equal(scrollDeltaToReveal({ listTop: 100, listBottom: 500, rowTop: 480, rowBottom: 520, header: 36 }), 20);
  // 底边让出横向滚动条：行的底已经进了外框，但仍压在滚动条上，要再往上让出那一截。
  equal(scrollDeltaToReveal({ listTop: 100, listBottom: 500, rowTop: 440, rowBottom: 490, header: 36, bottom: 16 }), 6);
});
