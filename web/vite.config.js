import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';

// The dev server keeps Vite's default port, 5173, because the API's CORS allowlist
// names it. strictPort fails loudly instead of moving to a port the API would refuse.
export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: { port: 5173, strictPort: true },
  test: {
    include: ['src/**/*.test.js'],
    environment: 'node'
  }
});
