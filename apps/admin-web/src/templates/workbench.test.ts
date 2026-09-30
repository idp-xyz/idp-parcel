import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { stepSelection, toggleWorkbenchSort, workbenchKeyAction } from './workbench';

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
