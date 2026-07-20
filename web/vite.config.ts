import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const backendTarget = process.env.BUILDWORLD_API_TARGET || 'http://127.0.0.1:8700'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // Keep the development server separate from the production control-plane
    // port; API and WebSocket requests still proxy to 8700 by default.
    port: 8701,
    proxy: {
      '^/api(?:/|$)': {
        target: backendTarget,
        changeOrigin: true,
      },
      '/ws': {
        target: backendTarget,
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
