import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { stepSelection, toggleWorkbenchSort, workbenchKeyAction } from './workbench';

// Covers: 同列翻方向，换列从降序起。
test('toggleWorkbenchSort：同列翻方向、换列从降序起', () => {
  deepEqual(toggleWorkbenchSort({ key: 'registered', dir: -1 }, 'registered'), { key: 'registered', dir: 1 });
  deepEqual(toggleWorkbenchSort({ key: 'registered', dir: 1 }, 'registered'), { key: 'registered', dir: -1 });
  deepEqual(toggleWorkbenchSort({ key: 'registered', dir: 1 }, 'id'), { key: 'id', dir: -1 });
});

// Covers: 输入框里不占任何键；Esc 只在详情开着时算关详情；其余键不认。
test('workbenchKeyAction：输入中不占键，Esc 看详情开没开', () => {
  equal(workbenchKeyAction('/', true, false), null);
  equal(workbenchKeyAction('ArrowDown', true, true), null);
  equal(workbenchKeyAction('/', false, false), 'focus-search');
  equal(workbenchKeyAction('Escape', false, true), 'close-detail');
  equal(workbenchKeyAction('Escape', false, false), null);
  equal(workbenchKeyAction('ArrowDown', false, false), 'select-next');
  equal(workbenchKeyAction('ArrowUp', false, false), 'select-previous');
  equal(workbenchKeyAction('Enter', false, true), null);
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
