import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          const normalizedId = id.replace(/\\/g, '/');
          if (!normalizedId.includes('node_modules')) return undefined;
          if (isPackage(normalizedId, ['react', 'react-dom', 'scheduler', 'loose-envify', 'js-tokens'])) return 'vendor-react';
          if (id.includes('@tiptap') || id.includes('prosemirror')) return 'vendor-editor';
          if (id.includes('lucide-react')) return 'vendor-icons';
          return 'vendor';
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
});

function isPackage(id, names) {
  return names.some((name) => {
    const escapedName = name.replace('/', '\\/');
    return new RegExp(`/node_modules/(?:\\.pnpm/[^/]+/node_modules/)?${escapedName}(?:/|$)`).test(id);
  });
}
