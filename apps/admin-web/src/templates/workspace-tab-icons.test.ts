import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { workspaceTabIcons } from './workspace-tab-icons';
import { workspaceTabOrder } from './workspace-tabs';

// 本文件钉的是图标表的一条纯规则（票 company-subject-ux/10）：一签一图、互不重复。
// 「每签有图」由 Record<WorkspaceTabId, LucideIcon> 交给 tsc；「两签不同图」类型说不出，只有这里钉。
// 表本身只引 lucide-react 与 workspace-tabs，CommonJS 发射加载得了；渲染件 WorkspaceTabLabel 不在这里验。

// Covers: 图标是给人扫一眼认签用的，两签同图比没图更误导。
test('图标表：固定表里的每一签各有一个不与别签重复的图标', () => {
  const icons = workspaceTabOrder.map((id) => workspaceTabIcons[id]);
  equal(icons.every((icon) => typeof icon === 'object' || typeof icon === 'function'), true);
  equal(new Set(icons).size, workspaceTabOrder.length);
});
