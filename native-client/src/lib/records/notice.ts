// A message for the next screen to show once, such as why a deal that was just added
// could not be closed. Reading it clears it, so a reload does not bring it back.

let pending = '';

export function setPendingNotice(message: string) {
  pending = message;
}

export function takePendingNotice(): string {
  const message = pending;
  pending = '';
  return message;
}
