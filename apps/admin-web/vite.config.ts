import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// 开发代理到 parcel-api（cmd/parcel-api 默认监听 :8080，后端用
// IDP_PARCEL_HTTP_ADDR 改监听、前端用 PARCEL_API_TARGET 改目标）。
// 路径约定：前端一律带 /api 前缀发起（装配侧 configureShipmentRequestApi
// 传 basePrefix='/api'），代理剥掉前缀转发——parcel-api 的路由本身不带
// /api（如 POST /shipment-requests），前缀只属前端侧的转发约定，
// 这样单条代理规则即可覆盖全部端点，不必逐路径枚举。
const apiTarget = process.env.PARCEL_API_TARGET || 'http://localhost:8080';

export default defineConfig({
  plugins: [react()],
  build: {
    rollupOptions: {
      output: {
        // 按变更频率拆 vendor 块：react 与组件库几乎不随业务页变，拆开后业务改动不失效它们的缓存。
        // 页面另按域懒加载（page-registry.tsx）：应用代码本身大到能把入口块推过告警线，加载闪烁由首屏后
        // 空闲预取盖住。vendor 块仍超线，病根在上游组件库，见 README「已知跟进」。
        manualChunks(id: string) {
          if (!id.includes('node_modules')) return undefined;
          if (/[\\/]node_modules[\\/](react|react-dom|scheduler)[\\/]/.test(id)) {
            return 'react-vendor';
          }
          if (id.includes('@idpxyz')) return 'ui-kit';
          // 兜底规则也收懒加载页独用的第三方依赖，会把它提前拉进首屏。今天各页只经 ui-kit 用第三方依赖；
          // 哪天某页独用一个重依赖，先改这条。
          return 'vendor';
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
      // OIDC 令牌交换同源转发：gk.idp.xyz 的 token 端点不对本地开发源放 CORS
      // （实测预检无 Access-Control-Allow-Origin），授权跳转是顶层导航不受约束，
      // 只有换令牌的 XHR 需要借道。目标与 src/auth/oidc.ts 的 ISSUER 同源。
      '/oidc': {
        target: 'https://gk.idp.xyz',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/oidc/, ''),
      },
    },
  },
});
