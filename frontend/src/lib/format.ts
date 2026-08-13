// Shared display formatters. Previously duplicated across Budgets, Overview,
// KeyPortal, and Usage — consolidated so spend/token formatting stays consistent.

export function microsToUSD(micros: number, decimals = 2): string {
  return formatUSD(micros / 1_000_000, decimals);
}

export function formatUSD(usd: number, decimals = 2): string {
  return `$${usd.toFixed(decimals)}`;
}

// Picks a decimal count that keeps small spend amounts legible: sub-cent values
// show more digits so a $0.0042 charge doesn't collapse to "$0.00".
export function formatSpendUSD(usd: number): string {
  if (usd === 0) return "$0.00";
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  if (usd < 1) return `$${usd.toFixed(3)}`;
  return `$${usd.toFixed(2)}`;
}

export function formatTokens(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`;
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

export function compactNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}
