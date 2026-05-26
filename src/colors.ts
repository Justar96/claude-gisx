// ANSI escape codes used throughout the statusline. Kept as plain strings
// so they can be concatenated cheaply into the output.

export const blue = "\x1b[38;2;0;153;255m";
export const green = "\x1b[38;2;0;175;80m";
export const cyan = "\x1b[38;2;86;182;194m";
export const red = "\x1b[38;2;255;85;85m";
export const yellow = "\x1b[38;2;230;200;0m";
export const white = "\x1b[38;2;220;220;220m";
export const magenta = "\x1b[38;2;180;140;255m";
export const orange = "\x1b[38;2;255;176;85m";
export const dimgray = "\x1b[38;2;120;120;120m";
export const dim = "\x1b[2m";
export const bold = "\x1b[1m";
export const reset = "\x1b[0m";

export const sep = `${dim} · ${reset}`;

export function pct(p: number): string {
  if (p >= 90) return red;
  if (p >= 70) return yellow;
  if (p >= 50) return orange;
  return green;
}

export function remaining(p: number): string {
  if (p <= 10) return red;
  if (p <= 30) return yellow;
  if (p <= 50) return orange;
  return green;
}
