import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useDensity, useTheme } from '@idpxyz/ui-theme-runtime';
import {
  readDensityPreference,
  seedUpstreamTheme,
  writeDensityPreference,
  writeThemePreference,
} from './preferences';

// 把上游 ThemeProvider / DensityProvider 与本产品的持久化键接起来（票 admin-web-ux-alignment/01 第 3、4 条）。
// 上游两个 Provider 都没有初始值入口（见 preferences.ts 文件头），所以两边接法不同：
// - 主题：ThemeProvider 的 useState 初始化只读 idpxyz-theme，只能在它首次渲染之前把本产品偏好播进那个键；之后每次
//   变化镜像写回本产品键，本产品键始终是权威。
// - 密度：DensityProvider 起手 compact、不读存储、只给 toggle，只能挂载后按存储值对齐一次。对齐放 layout effect：
//   在浏览器绘制之前完成，操作者看不到 compact 那一帧；之后每次变化写回。
// 三件都不渲染任何 DOM，摆在 App.tsx 各自的 Provider 之内。

/**
 * 在 ThemeProvider 首次渲染之前播种上游主题键。要在渲染 ThemeProvider 的那个组件里调用——它的函数体先于子树执行；
 * 用 useState 的懒初始化是为了每次挂载只跑一次，StrictMode 的双调用落到同一个值上，幂等。
 */
export function useSeedUpstreamTheme(): void {
  useState(() => seedUpstreamTheme(window.localStorage));
}

/** 主题每变一次就写回本产品键；播种保证了首个值与本产品键一致，首次写回是幂等的。 */
export function ThemePreferenceMirror() {
  const { theme } = useTheme();
  useEffect(() => {
    writeThemePreference(window.localStorage, theme);
  }, [theme]);
  return null;
}

/**
 * 挂载后按存储值对齐密度一次，之后每次变化写回。
 *
 * 三段相位而不是一个布尔：DensityProvider 只给 toggle，「对齐」只能靠切一下，而 StrictMode 在开发模式会把挂载 effect
 * 跑两遍、两遍看到的都是切之前的值——不记「已经切过」就会切两次又回到起点。切过之后要等 density 真的变成存储值才算
 * 对齐，从那一刻起 density 才是权威、才开始写回；对齐前不写，免得把上游起手的 compact 写进本产品键。
 */
export function DensityPreferenceAlignment() {
  const { density, toggleDensity } = useDensity();
  const phase = useRef<'pending' | 'toggled' | 'aligned'>('pending');

  useLayoutEffect(() => {
    if (phase.current === 'aligned') {
      writeDensityPreference(window.localStorage, density);
      return;
    }
    if (density === readDensityPreference(window.localStorage)) {
      phase.current = 'aligned';
      return;
    }
    if (phase.current === 'pending') {
      phase.current = 'toggled';
      toggleDensity();
    }
  }, [density, toggleDensity]);

  return null;
}
