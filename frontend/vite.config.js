import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// https://vitejs.dev/config/
export default defineConfig({
  // wails3 dev 以 `npm run dev -- --port <VITE_PORT> --strictPort` 注入端口，CLI 参数优先级更高；
  // 这里的 port 仅在直接执行 vite 时生效。
  //
  // ⚠️ 改端口必须三处同步：Taskfile.yml 的 VITE_PORT、build/config.yml 的 readiness.tcp
  //    （那个是硬编码字面量，无法用环境变量替代）、以及这里的兜底值。
  //    Taskfile.yml 的 dev 任务会在端口不一致时直接拦下并提示。
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [
    vue(),
    AutoImport({
      resolvers: [ElementPlusResolver()],
    }),
    Components({
      resolvers: [ElementPlusResolver()],
    }),
  ],
})


