import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    watch: {
      ignored: ['**/.*.tmp*/**', '**/*.tmp', '**/dist/**', '**/node_modules/**'],
    },
    proxy: {
      '/dockium.panel.v1.': {
        target: 'http://127.0.0.1:3434',
        changeOrigin: false,
      },
      '/healthz': {
        target: 'http://127.0.0.1:3434',
        changeOrigin: false,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    target: 'es2022',
    assetsInlineLimit: 0,
  },
})
