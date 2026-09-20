import { test } from 'node:test';
import { equal, deepEqual } from 'node:assert/strict';
import {
  DEFAULT_DENSITY,
  DEFAULT_THEME,
  DENSITY_STORAGE_KEY,
  THEME_STORAGE_KEY,
  UPSTREAM_THEME_STORAGE_KEY,
  readDensityPreference,
  readThemePreference,
  seedUpstreamTheme,
  writeDensityPreference,
  writeThemePreference,
  type PreferenceStorage,
} from './preferences';

// 本文件钉的是壳层偏好的纯逻辑（票 admin-web-ux-alignment/01 第 3 条）：键名带产品名、无存储时 Light / comfortable、
// 坏值当没存、写了能读回、以及把本产品的主题偏好播进上游 ThemeProvider 自己读的那个键。组件那半（TopBar 四个位、
// Provider 接线）在 node:test 里钉不到，理由见 templates/loading-shape.ts 文件头；本票用一次性 esbuild 束实测，结论写票面。

function storageOf(initial: Record<string, string> = {}): PreferenceStorage & { dump(): Record<string, string> } {
  const map = new Map(Object.entries(initial));
  return {
    getItem: (key) => map.get(key) ?? null,
    setItem: (key, value) => {
      map.set(key, value);
    },
    dump: () => Object.fromEntries(map),
  };
}

// Covers: 键名带产品名——同一浏览器上多个 IDP 产品各存各的，不互相覆盖。
test('存储键带产品名', () => {
  equal(THEME_STORAGE_KEY, 'parcel-admin-web:theme');
  equal(DENSITY_STORAGE_KEY, 'parcel-admin-web:density');
});

// Covers: 无存储时的默认值是手册「主题策略」的 Light 与「栅格与密度」里表单 / 管理类的 comfortable。
test('无存储时默认 Light 与 comfortable', () => {
  const storage = storageOf();
  equal(DEFAULT_THEME, 'light');
  equal(DEFAULT_DENSITY, 'comfortable');
  equal(readThemePreference(storage), 'light');
  equal(readDensityPreference(storage), 'comfortable');
});

// Covers: 存了合法值原样读回；两个偏好互不串键。
test('合法存储值原样读回', () => {
  const storage = storageOf({ [THEME_STORAGE_KEY]: 'dark', [DENSITY_STORAGE_KEY]: 'compact' });
  equal(readThemePreference(storage), 'dark');
  equal(readDensityPreference(storage), 'compact');
});

// Covers: 坏值（拼错、大小写、空串、别的产品的词）一律当没存——读回默认值，不把脏字符串交给 Provider。
test('坏值当没存', () => {
  for (const bad of ['Light', 'DARK', '', 'blue', 'comfortable']) {
    equal(readThemePreference(storageOf({ [THEME_STORAGE_KEY]: bad })), 'light', `theme=${JSON.stringify(bad)}`);
  }
  for (const bad of ['Compact', '', 'dense', 'dark']) {
    equal(readDensityPreference(storageOf({ [DENSITY_STORAGE_KEY]: bad })), 'comfortable', `density=${JSON.stringify(bad)}`);
  }
});

// Covers: 写了能读回，且只写自己那个键。
test('写入后可读回', () => {
  const storage = storageOf();
  writeThemePreference(storage, 'dark');
  writeDensityPreference(storage, 'compact');
  equal(readThemePreference(storage), 'dark');
  equal(readDensityPreference(storage), 'compact');
  deepEqual(storage.dump(), { [THEME_STORAGE_KEY]: 'dark', [DENSITY_STORAGE_KEY]: 'compact' });
});

// Covers: 上游 ThemeProvider 只认自己的键（idpxyz-theme）且默认 dark，本产品的 Light 默认要先播进那个键才生效；
// 播种以本产品键为准——本产品键没存时写 Light，上游键里残留的 dark 被压掉，而不是反过来被它带成 dark。
test('播种上游主题键以本产品偏好为准', () => {
  equal(UPSTREAM_THEME_STORAGE_KEY, 'idpxyz-theme');

  const fresh = storageOf({ [UPSTREAM_THEME_STORAGE_KEY]: 'dark' });
  equal(seedUpstreamTheme(fresh), 'light');
  equal(fresh.getItem(UPSTREAM_THEME_STORAGE_KEY), 'light');

  const prefersDark = storageOf({ [THEME_STORAGE_KEY]: 'dark' });
  equal(seedUpstreamTheme(prefersDark), 'dark');
  equal(prefersDark.getItem(UPSTREAM_THEME_STORAGE_KEY), 'dark');
});
