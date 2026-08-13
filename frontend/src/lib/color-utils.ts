/**
 * Color utility functions for generating shade scales from hex colors.
 * Used by the palette system to dynamically generate accent/secondary
 * color scales (50-900) that get applied as CSS custom properties.
 */

/** Parse a hex color string to [r, g, b] (0-255). */
function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace("#", "");
  return [
    parseInt(h.substring(0, 2), 16),
    parseInt(h.substring(2, 4), 16),
    parseInt(h.substring(4, 6), 16),
  ];
}

/** Convert [r, g, b] (0-255) to hex string. */
function rgbToHex(r: number, g: number, b: number): string {
  const clamp = (v: number) => Math.max(0, Math.min(255, Math.round(v)));
  return `#${[clamp(r), clamp(g), clamp(b)]
    .map((c) => c.toString(16).padStart(2, "0"))
    .join("")}`;
}

/** Convert RGB to HSL. Returns [h (0-360), s (0-1), l (0-1)]. */
function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
  const rn = r / 255;
  const gn = g / 255;
  const bn = b / 255;
  const max = Math.max(rn, gn, bn);
  const min = Math.min(rn, gn, bn);
  const d = max - min;
  let h = 0;
  const l = (max + min) / 2;

  if (d !== 0) {
    if (max === rn) h = ((gn - bn) / d + (gn < bn ? 6 : 0)) / 6;
    else if (max === gn) h = ((bn - rn) / d + 2) / 6;
    else h = ((rn - gn) / d + 4) / 6;
  }

  const s = l === 0 || l === 1 ? 0 : d / (1 - Math.abs(2 * l - 1));
  return [h * 360, s, l];
}

/** Convert HSL to RGB. h: 0-360, s: 0-1, l: 0-1. Returns [r, g, b] (0-255). */
function hslToRgb(h: number, s: number, l: number): [number, number, number] {
  const c = (1 - Math.abs(2 * l - 1)) * s;
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
  const m = l - c / 2;
  let r1 = 0, g1 = 0, b1 = 0;

  if (h < 60) { r1 = c; g1 = x; b1 = 0; }
  else if (h < 120) { r1 = x; g1 = c; b1 = 0; }
  else if (h < 180) { r1 = 0; g1 = c; b1 = x; }
  else if (h < 240) { r1 = 0; g1 = x; b1 = c; }
  else if (h < 300) { r1 = x; g1 = 0; b1 = c; }
  else { r1 = c; g1 = 0; b1 = x; }

  return [
    Math.round((r1 + m) * 255),
    Math.round((g1 + m) * 255),
    Math.round((b1 + m) * 255),
  ];
}

/** Mix two hex colors by a ratio (0 = colorA, 1 = colorB). */
function mixColors(colorA: string, colorB: string, ratio: number): string {
  const [r1, g1, b1] = hexToRgb(colorA);
  const [r2, g2, b2] = hexToRgb(colorB);
  return rgbToHex(
    r1 + (r2 - r1) * ratio,
    g1 + (g2 - g1) * ratio,
    b1 + (b2 - b1) * ratio,
  );
}

export interface ShadeScale {
  50: string;
  100: string;
  200: string;
  300: string;
  400: string;
  500: string;
  600: string;
  700: string;
  800: string;
  900: string;
}

/**
 * Generate a 10-step shade scale (50-900) from a single hex color.
 *
 * Strategy:
 *   - 50  = very light tint (mix 92% white)
 *   - 100 = light tint (mix 82% white)
 *   - 200 = soft tint (mix 64% white)
 *   - 300 = medium tint (mix 44% white)
 *   - 400 = light shade (mix 22% white)
 *   - 500 = the base color (unchanged)
 *   - 600 = slightly darker (shift L down)
 *   - 700 = darker (shift L down more)
 *   - 800 = dark (shift L down significantly)
 *   - 900 = very dark (shift L down to near-black)
 *
 * This produces perceptually balanced scales that work for both accent and
 * secondary colors across warm, cool, and neutral hue families.
 */
export function generateShades(hex: string): ShadeScale {
  const [r, g, b] = hexToRgb(hex);
  const [h, s] = rgbToHsl(r, g, b);

  // Lighter shades: mix with white
  const white = "#ffffff";
  const shade50 = mixColors(hex, white, 0.92);
  const shade100 = mixColors(hex, white, 0.82);
  const shade200 = mixColors(hex, white, 0.64);
  const shade300 = mixColors(hex, white, 0.44);
  const shade400 = mixColors(hex, white, 0.22);

  // Base color
  const shade500 = hex;

  // Darker shades: reduce lightness while slightly desaturating for depth
  const [r6, g6, b6] = hslToRgb(h, Math.max(0, s - 0.02), 0.38);
  const [r7, g7, b7] = hslToRgb(h, Math.max(0, s - 0.04), 0.28);
  const [r8, g8, b8] = hslToRgb(h, Math.max(0, s - 0.06), 0.2);
  const [r9, g9, b9] = hslToRgb(h, Math.max(0, s - 0.08), 0.14);

  return {
    50: shade50,
    100: shade100,
    200: shade200,
    300: shade300,
    400: shade400,
    500: shade500,
    600: rgbToHex(r6, g6, b6),
    700: rgbToHex(r7, g7, b7),
    800: rgbToHex(r8, g8, b8),
    900: rgbToHex(r9, g9, b9),
  };
}

/**
 * Apply a shade scale as CSS custom properties on the given element.
 * Keys are formatted as `--color-{prefix}-{shade}`.
 */
export function applyShadeScale(
  el: HTMLElement,
  prefix: string,
  scale: ShadeScale,
): void {
  for (const [shade, value] of Object.entries(scale)) {
    el.style.setProperty(`--color-${prefix}-${shade}`, value);
  }
}

/**
 * Remove all CSS custom properties for a given prefix from an element.
 */
export function removeShadeScale(el: HTMLElement, prefix: string): void {
  for (const shade of [50, 100, 200, 300, 400, 500, 600, 700, 800, 900]) {
    el.style.removeProperty(`--color-${prefix}-${shade}`);
  }
}