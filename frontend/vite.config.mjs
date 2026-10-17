import { defineConfig } from 'vite';

export default defineConfig({
  // Keep the same output directory create-react-app used so the Dockerfile
  // and backend (BUILD_DIR) keep working unchanged.
  build: {
    outDir: 'build',
  },
  // React 16.13 has no jsx-runtime; the source already imports React.
  oxc: {
    jsx: { runtime: 'classic' },
  },
  server: {
    port: 3000,
  },
  define: {
    'process.env.PUBLIC_URL': JSON.stringify(''),
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/setupTests.js',
  },
});
