// `pnpm desktop:build`: tauri build with a Content-Security-Policy that lets the window
// talk to one API, the one named by EXPO_PUBLIC_API_URL when the app is exported (the
// same variable the app reads), so a script that got into the page cannot send the
// refresh token anywhere else. Extra arguments go to `tauri build`.

import { spawn } from 'node:child_process';

const api = process.env.EXPO_PUBLIC_API_URL || 'http://localhost:8085';
let origin;
try {
  origin = new URL(api).origin;
} catch {
  console.error(`EXPO_PUBLIC_API_URL is not a URL: ${api}`);
  process.exit(1);
}

const csp = [
  "default-src 'self'",
  `connect-src 'self' ${origin} ipc: http://ipc.localhost`,
  "img-src 'self' data: blob:",
  "font-src 'self' data:",
  "style-src 'self' 'unsafe-inline'",
  "script-src 'self'"
].join('; ');

const config = JSON.stringify({ app: { security: { csp } } });
const child = spawn('pnpm', ['tauri', 'build', '--config', config, ...process.argv.slice(2)], { stdio: 'inherit' });
child.on('exit', (code) => process.exit(code ?? 0));
