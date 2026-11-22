// Formatting helpers for the values the generated types expose.
//
// This file is where the type mapping becomes visible to the UI: int64 arrives as a
// string, bytes as base64, Timestamp as an RFC3339 string, and a money amount as
// units plus nanos. Rendering those is application code, not generator code.

import type { Book, Money } from "@/api/books";

// formatMoney renders a common.v1.Money as a minor-unit-aware decimal string.
//
// `units` is an int64, which the generated type correctly keeps as a string: a value
// above Number.MAX_SAFE_INTEGER would lose precision as a number. It is converted
// only at the point of display, and via BigInt so the conversion itself is exact.
export function formatMoney(money: Money | undefined): string {
  if (!money) return "—";

  const currency = money.currencyCode ?? "USD";
  const units = BigInt(money.units ?? "0");
  const nanos = money.nanos ?? 0;

  // nanos is a billionth, so six digits cover the common cent case while keeping
  // sub-cent precision out of the display.
  const fraction = Math.abs(nanos).toString().padStart(9, "0").slice(0, 2);
  const sign = units < 0n || nanos < 0 ? "-" : "";

  return `${sign}${currency} ${absBigInt(units)}.${fraction}`;
}

function absBigInt(value: bigint): bigint {
  return value < 0n ? -value : value;
}

// formatTimestamp renders an RFC3339 string from the proto as a local date.
//
// The generated type is `string`, because that is what protojson emits; parsing is
// the application's job and happens here once.
export function formatTimestamp(value: string | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

// formatBytes renders a base64 payload's size, which is the only thing this example
// needs from a bytes field.
export function formatBytesSize(base64: string | undefined): string {
  if (!base64) return "0 B";
  // Base64 encodes three bytes per four characters; padding is not part of the
  // payload length.
  const padding = base64.endsWith("==") ? 2 : base64.endsWith("=") ? 1 : 0;
  const bytes = Math.max(0, Math.floor((base64.length * 3) / 4) - padding);
  return `${bytes} B`;
}

// describeAvailability narrows the generated oneof union.
//
// The type is a discriminated union, so `"inPrint" in book` narrows to the arm that
// sets it. That is the payoff of generating exclusive unions rather than two
// independent optional fields.
export function describeAvailability(book: Book): string {
  if ("inPrint" in book && book.inPrint) {
    const stock = book.inPrint.stock ?? 0;
    return stock > 0 ? `${stock} in stock` : "out of stock, on order";
  }
  if ("outOfPrint" in book && book.outOfPrint) {
    return book.outOfPrint.reason ?? "out of print";
  }
  return "availability unknown";
}
