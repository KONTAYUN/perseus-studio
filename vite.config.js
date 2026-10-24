import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  // remark's entity decoder offers a DOM-free worker export. Use the same
  // parser in both editor and Worker rather than injecting a fake document.
  resolve: { conditions: ['worker', 'module', 'browser', 'development|production'] },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
});
