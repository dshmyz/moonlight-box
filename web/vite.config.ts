import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import { webcrypto } from 'node:crypto'

if (!globalThis.crypto) {
  globalThis.crypto = webcrypto as Crypto
}

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 5000,
    proxy: {
      '/api': {
        target: 'http://localhost:9081',
        changeOrigin: true,
      },
      '/npm': {
        target: 'http://localhost:9081',
        changeOrigin: true,
      },
      '/maven2': {
        target: 'http://localhost:9081',
        changeOrigin: true,
      },
      '/repo': {
        target: 'http://localhost:9081',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: '../cmd/registry/dist',
    emptyOutDir: true,
    assetsDir: 'assets',
    minify: 'esbuild',
    rollupOptions: {
      output: {
        manualChunks: {
          elementPlus: ['element-plus', '@element-plus/icons-vue'],
          mermaid: ['mermaid'],
          vendor: ['vue', 'vue-router', 'pinia', 'axios'],
        },
      },
    },
  },
})
