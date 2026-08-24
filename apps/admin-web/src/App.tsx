import { ThemeProvider, ToastProvider } from '@idpxyz/ui-theme-runtime';
import { Layout } from './Layout';

// 租户管理台外壳：沿用 idp-ui loms-web 当前默认的传统控制台形态
// （品牌头 + 左侧导航 + 单页区），不用 IDE 工作区的标签页/面板形态——
// 管理面以查阅与复核为主，单页区足够，形态取舍随首个真实页面再议。
// ui-tokens 的 productAccent 尚未登记 parcel 的产品色，这里先不传
// product（取 ThemeProvider 默认色）；上游登记后再显式传入。
export default function App() {
  return (
    <ThemeProvider>
      <ToastProvider>
        <Layout />
      </ToastProvider>
    </ThemeProvider>
  );
}
