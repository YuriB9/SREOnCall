import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import path from 'path'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api/ingest':      { target: 'http://localhost:8080', changeOrigin: true },
      '/api/incidents':   { target: 'http://localhost:8081', changeOrigin: true },
      '/api/schedules':   { target: 'http://localhost:8082', changeOrigin: true },
      '/api/escalations': { target: 'http://localhost:8083', changeOrigin: true },
      '/api/notifications': { target: 'http://localhost:8084', changeOrigin: true },
    },
  },
})
