import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  clearScreen: false,
  server: { host: '127.0.0.1', port: 5173, strictPort: false },
  build: {
    target: 'es2022',
    outDir: '../desktop/ui',
    emptyOutDir: true,
    rollupOptions: { output: { manualChunks: { vue: ['vue'], ui: ['naive-ui'] } } },
  },
})
