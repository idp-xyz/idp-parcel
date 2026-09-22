import { DensityProvider, ThemeProvider, ToastProvider } from '@idpxyz/ui-theme-runtime';
import { Layout } from './Layout';
import { DensityPreferenceAlignment, ThemePreferenceMirror, useSeedUpstreamTheme } from './shell/preference-sync';

// 租户管理台外壳：多标签工作区形态（品牌头 + 左侧导航 + EditorGroup 多标签主区 + 右侧检查器栏 + 底部状态栏），
// 参照 idp-ui apps/myshop-web（票 admin-web-workspace-form/01、02）。此前沿 loms-web 的单页区形态、把「标签页 / 面板」
// 留待首个真实页面出现后再议——真实页面早已有了，形态取舍与理由在 Layout.tsx 文件头，这里不复述。
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
