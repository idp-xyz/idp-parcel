import { DensityProvider, ThemeProvider, ToastProvider } from '@idpxyz/ui-theme-runtime';
import { Layout } from './Layout';
import { DensityPreferenceAlignment, ThemePreferenceMirror, useSeedUpstreamTheme } from './shell/preference-sync';

// 租户管理台外壳：沿用 idp-ui loms-web 当前默认的传统控制台形态
// （品牌头 + 左侧导航 + 单页区），不用 IDE 工作区的标签页/面板形态——
// 管理面以查阅与复核为主，单页区足够，形态取舍随首个真实页面再议。
//
// 三个 Provider 同级：ThemeProvider（主题）、DensityProvider（密度两档，手册「栅格与密度」；
// 票 03 的列表模板从 useDensity 读它）、ToastProvider。主题与密度的默认值与持久化不归上游
// Provider 管，由 shell/preference-sync 的两个同步件接到本产品自己的 localStorage 键上；
// 播种上游主题键必须先于 ThemeProvider 首次渲染，所以那一步是本组件函数体里的 hook。
//
// ui-tokens 的 productAccent 尚未登记 parcel 的产品色，这里仍不传 product（取 ThemeProvider
// 默认色）；登记在 idp-ui 上游仓、归用户，登记后在此显式传入，Top Bar 的归属信息处一并跟上。
export default function App() {
  useSeedUpstreamTheme();
  return (
    <ThemeProvider>
      <ThemePreferenceMirror />
      <DensityProvider>
        <DensityPreferenceAlignment />
        <ToastProvider>
          <Layout />
        </ToastProvider>
      </DensityProvider>
    </ThemeProvider>
  );
}
