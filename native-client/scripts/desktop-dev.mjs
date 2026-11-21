// The dev server for `pnpm desktop`: Expo's, on the port the window loads, without a
// browser tab, and failing rather than moving to another port.

import { spawn } from 'node:child_process';
import net from 'node:net';

const PORT = 8081;

const busy = await new Promise((resolve) => {
  const socket = net.connect(PORT, '127.0.0.1');
  socket.once('connect', () => (socket.destroy(), resolve(true)));
  socket.once('error', () => resolve(false));
});
if (busy) {
  console.error(`Port ${PORT} is in use, and the desktop window loads that port. Stop the other Metro or Expo server first.`);
  process.exit(1);
}

const child = spawn('pnpm', ['expo', 'start', '--port', String(PORT)], { stdio: 'inherit', env: { ...process.env, CI: '1' } });
child.on('exit', (code) => process.exit(code ?? 0));
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
