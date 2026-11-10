import { messages, type MessageKey } from "./en-US";

export type { MessageKey };

export const locale = "en-US";

/** t looks up a message and substitutes {placeholders}. */
export function t(key: MessageKey, vars?: Record<string, string | number>): string {
  const template: string = messages[key];
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (match, name: string) =>
    name in vars ? String(vars[name]) : match,
  );
}

const dateFmt = new Intl.DateTimeFormat(locale, { dateStyle: "medium" });
const dateTimeFmt = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" });

export function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : dateFmt.format(d);
}

export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : dateTimeFmt.format(d);
}

/** formatCalendarDate formats a YYYY-MM-DD date without time zone shifts. */
export function formatCalendarDate(ymd: string): string {
  const [y, m, d] = ymd.split("-").map(Number);
  if (!y || !m || !d) return ymd;
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeZone: "UTC" }).format(Date.UTC(y, m - 1, d));
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds} sec`;
  const min = Math.round(seconds / 60);
  return `${min} min`;
}
