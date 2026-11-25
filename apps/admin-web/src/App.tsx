import { DensityProvider, ThemeProvider, ToastProvider } from '@idpxyz/ui-theme-runtime';
import { Layout } from './Layout';
import { DensityPreferenceAlignment, ThemePreferenceMirror, useSeedUpstreamTheme } from './shell/preference-sync';
import { StationEntry } from './shell/StationEntry';
import { useStationHash } from './shell/use-station-address';

// 租户管理台外壳：多标签工作区形态（品牌头 + 左侧导航 + EditorGroup 多标签主区 + 右侧检查器栏 + 底部状态栏），
// 参照 idp-ui apps/myshop-web（票 admin-web-workspace-form/01、02）。形态取舍与理由在 Layout.tsx 文件头，这里不复述。
//
// 三个 Provider 同级：ThemeProvider（主题）、DensityProvider（密度两档，手册「栅格与密度」；
// 票 03 的列表模板从 useDensity 读它）、ToastProvider。主题与密度的默认值与持久化不归上游
// Provider 管，由 shell/preference-sync 的两个同步件接到本产品自己的 localStorage 键上；
// 播种上游主题键必须先于 ThemeProvider 首次渲染，所以那一步是本组件函数体里的 hook。
//
// ui-tokens 的 productAccent 尚未登记 parcel 的产品色，这里仍不传 product（取 ThemeProvider
// 默认色）；登记在 idp-ui 上游仓、归用户，登记后在此显式传入，Top Bar 的归属信息处一并跟上。
//
// 作业台（`#/station…`）不带外壳：地址是作业台的时不渲染 Layout，渲染全屏的作业台，三个 Provider 两边共用。地址的判断在
// shell/station-address.ts；不是作业台的地址时这里的输出与没有作业台时一样——订阅的快照是「作业台地址或 null」，外壳里地址
// 来回变不会让本组件重渲。进出作业台会卸掉 Layout 再装上：标签集与活动标签在本机存储里，回来原样恢复，页面里的内存状态不保留。
export default function App() {
  useSeedUpstreamTheme();
  const stationHash = useStationHash();
  return (
    <ThemeProvider>
      <ThemePreferenceMirror />
      <DensityProvider>
        <DensityPreferenceAlignment />
        <ToastProvider>{stationHash === null ? <Layout /> : <StationEntry hash={stationHash} />}</ToastProvider>
      </DensityProvider>
    </ThemeProvider>
  );
}
