import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

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
        target: 'http://localhost:9082',
        changeOrigin: true,
      },
      '/npm': {
        target: 'http://localhost:9082',
        changeOrigin: true,
      },
      '/maven2': {
        target: 'http://localhost:9082',
        changeOrigin: true,
      },
      '/repo': {
        target: 'http://localhost:9082',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
  },
})
