// 壳层偏好（主题 / 密度）的持久化纯逻辑（票 admin-web-ux-alignment/01 第 3 条）。
//
// 为什么单独一个 .ts：要钉的规则是键名、默认值、坏值处置与「播种上游键」这几件，全是与 React 无关的事实；
// 组件层在 node:test 里钉不到（理由见 templates/loading-shape.ts 文件头），把规则抬到这里钉，Provider 接线那半
// （App.tsx 与 shell/ 下的同步件）只剩摆。
//
// 上游 @idpxyz/ui-theme-runtime 的两个 Provider 都没有「初始值」入口：ThemeProvider 只读自己的 localStorage 键
// （UPSTREAM_THEME_STORAGE_KEY）且无存储时取 dark；DensityProvider 不读任何存储、起手 compact。手册「主题策略」要
// 企业工作台默认 Light、票面要键名带产品名，于是本产品自己的键才是权威，上游键只是给 ThemeProvider 初始化用的
// 传声筒——播种时以本产品键为准覆盖它，而不是反过来被上游残值带走。

/** 与 @idpxyz/ui-tokens 的 ThemeMode 同形；不从那里 import 是为了让本文件零依赖、测试编成 CommonJS 后能直接 require。 */
export type ThemePreference = 'light' | 'dark';

/** 与 @idpxyz/ui-theme-runtime 的 ListDensity 同形，理由同上。 */
export type DensityPreference = 'comfortable' | 'compact';

export const THEME_STORAGE_KEY = 'parcel-admin-web:theme';
export const DENSITY_STORAGE_KEY = 'parcel-admin-web:density';

/** ThemeProvider 自己读写的键；本产品不拥有它，只在挂载前播种一次。 */
export const UPSTREAM_THEME_STORAGE_KEY = 'idpxyz-theme';

/** 手册「主题策略」：企业级工作台默认 Light。 */
export const DEFAULT_THEME: ThemePreference = 'light';

/** 手册「栅格与密度」：表单 / 管理类页面默认 Comfortable；列表页的紧凑档由操作者自己切。 */
export const DEFAULT_DENSITY: DensityPreference = 'comfortable';

/** localStorage 的最小子集；测试用 Map 顶替，生产传 window.localStorage。 */
export interface PreferenceStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

const THEME_VALUES: readonly ThemePreference[] = ['light', 'dark'];
const DENSITY_VALUES: readonly DensityPreference[] = ['comfortable', 'compact'];

function readEnum<T extends string>(storage: PreferenceStorage, key: string, allowed: readonly T[], fallback: T): T {
  const raw = storage.getItem(key);
  // 坏值当没存：Provider 只认这几个词，把脏字符串交过去等于把一个不存在的主题写进 DOM class。
  return (allowed as readonly string[]).includes(raw ?? '') ? (raw as T) : fallback;
}

export function readThemePreference(storage: PreferenceStorage): ThemePreference {
  return readEnum(storage, THEME_STORAGE_KEY, THEME_VALUES, DEFAULT_THEME);
}

export function readDensityPreference(storage: PreferenceStorage): DensityPreference {
  return readEnum(storage, DENSITY_STORAGE_KEY, DENSITY_VALUES, DEFAULT_DENSITY);
}

export function writeThemePreference(storage: PreferenceStorage, theme: ThemePreference): void {
  storage.setItem(THEME_STORAGE_KEY, theme);
}

export function writeDensityPreference(storage: PreferenceStorage, density: DensityPreference): void {
  storage.setItem(DENSITY_STORAGE_KEY, density);
}

/**
 * 把本产品的主题偏好写进上游 ThemeProvider 读的那个键，让它初始化时读到的就是我们要的模式；返回播下去的值。
 * 必须在 ThemeProvider 首次渲染之前调用——它的 useState 初始化只跑一次。
 */
export function seedUpstreamTheme(storage: PreferenceStorage): ThemePreference {
  const theme = readThemePreference(storage);
  storage.setItem(UPSTREAM_THEME_STORAGE_KEY, theme);
  return theme;
}
