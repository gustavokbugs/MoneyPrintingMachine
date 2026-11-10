import {
  Archive,
  CircleAlert,
  CircleCheck,
  Clock,
  FilePen,
  Info,
  LoaderCircle,
  Palette,
  Pause,
  Play,
  RotateCw,
  Sparkles,
  TriangleAlert,
  type LucideIcon,
} from "lucide-react";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { ApiError } from "../api/client";
import type { ChannelStatus, Job, Provenance, Rating } from "../api/types";
import { formatDateTime, t, type MessageKey } from "../i18n";

export type Tone = "success" | "warning" | "danger" | "info" | "neutral";

/** Icon sizes are tokens: inline text 16, navigation and headers 20. */
export const ICON_SM = 16;
export const ICON_MD = 20;

const statusStyle: Record<ChannelStatus, { tone: Tone; icon: LucideIcon }> = {
  draft: { tone: "neutral", icon: FilePen },
  branding: { tone: "info", icon: Palette },
  awaiting_platform_setup: { tone: "warning", icon: Clock },
  ready: { tone: "success", icon: CircleCheck },
  active: { tone: "success", icon: Play },
  paused: { tone: "warning", icon: Pause },
  error: { tone: "danger", icon: TriangleAlert },
  archived: { tone: "neutral", icon: Archive },
};

/** Badge conveys status with icon, text, and color (never color alone). */
export function Badge({ tone, icon: Icon, children }: { tone: Tone; icon?: LucideIcon; children: ReactNode }) {
  return (
    <span className={`badge tone-${tone}`}>
      {Icon && <Icon size={12} strokeWidth={2.5} aria-hidden="true" />}
      {children}
    </span>
  );
}

export function StatusBadge({ status }: { status: ChannelStatus }) {
  const s = statusStyle[status];
  return (
    <Badge tone={s.tone} icon={s.icon}>
      {t(`status.${status}` as MessageKey)}
    </Badge>
  );
}

const ratingTone: Record<Rating, Tone> = { low: "danger", medium: "warning", high: "success" };

export function RatingChip({ rating }: { rating: Rating }) {
  return <Badge tone={ratingTone[rating]}>{t(`rating.${rating}` as MessageKey)}</Badge>;
}

/** AiBadge discloses model-generated content. */
export function AiBadge() {
  return (
    <Badge tone="info" icon={Sparkles}>
      {t("ai.generated")}
    </Badge>
  );
}

type ButtonVariant = "primary" | "secondary" | "ghost" | "danger" | "danger-ghost";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: "md" | "sm";
  icon?: LucideIcon;
  loading?: boolean;
}

/**
 * Button with a built-in loading state: it stays the same size, shows a
 * spinner, and is disabled while an async action runs.
 */
export function Button({ variant = "secondary", size = "md", icon: Icon, loading = false, disabled, children, className, type, ...rest }: ButtonProps) {
  const classes = ["btn", `btn-${variant}`, size === "sm" ? "btn-sm" : "", className ?? ""].filter(Boolean).join(" ");
  return (
    <button type={type ?? "button"} className={classes} disabled={disabled || loading} aria-busy={loading || undefined} {...rest}>
      {loading ? (
        <LoaderCircle size={ICON_SM} className="spin" aria-hidden="true" />
      ) : (
        Icon && <Icon size={ICON_SM} aria-hidden="true" />
      )}
      {children}
    </button>
  );
}

export function Loading({ label = t("common.loading") }: { label?: string }) {
  return (
    <p className="row muted small" role="status" aria-live="polite">
      <LoaderCircle size={ICON_SM} className="spin" aria-hidden="true" /> {label}
    </p>
  );
}

/** Skeleton reserves the space of loading content to avoid layout shift. */
export function Skeleton({ height = 16, width = "100%" }: { height?: number; width?: number | string }) {
  return <span className="skeleton" style={{ height, width }} aria-hidden="true" />;
}

export function SkeletonCard({ lines = 3 }: { lines?: number }) {
  return (
    <div className="card" role="status" aria-label={t("common.loading")}>
      <Skeleton height={20} width="40%" />
      {Array.from({ length: lines }, (_, i) => (
        <Skeleton key={i} width={i === lines - 1 ? "70%" : "100%"} />
      ))}
    </div>
  );
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return t("common.errorTitle");
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  return (
    <div className="alert alert-error" role="alert">
      <CircleAlert size={ICON_MD} aria-hidden="true" />
      <div className="alert-body">
        <strong>{t("common.errorTitle")}</strong>
        <p>{errorMessage(error)}</p>
        {onRetry && (
          <div>
            <Button size="sm" icon={RotateCw} onClick={onRetry}>
              {t("common.retry")}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

const alertIcons: Record<"info" | "success" | "warning", LucideIcon> = {
  info: Info,
  success: CircleCheck,
  warning: TriangleAlert,
};

export function Notice({ tone, title, children, live = false }: { tone: "info" | "success" | "warning"; title?: string; children: ReactNode; live?: boolean }) {
  const Icon = alertIcons[tone];
  return (
    <div className={`alert alert-${tone}`} role={live ? "status" : undefined}>
      <Icon size={ICON_MD} aria-hidden="true" />
      <div className="alert-body">
        {title && <strong>{title}</strong>}
        <div>{children}</div>
      </div>
    </div>
  );
}

export function Empty({ icon: Icon = Info, children, action }: { icon?: LucideIcon; children: ReactNode; action?: ReactNode }) {
  return (
    <div className="empty">
      <span className="empty-icon">
        <Icon size={ICON_MD} aria-hidden="true" />
      </span>
      <div>{children}</div>
      {action}
    </div>
  );
}

export function Section({ title, description, actions, children }: { title: string; description?: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="card">
      <header className="card-header">
        <div>
          <h2>{title}</h2>
          {description && <p className="small muted">{description}</p>}
        </div>
        {actions && <div className="card-actions">{actions}</div>}
      </header>
      {children}
    </section>
  );
}

export function JobProgress({ job, error }: { job: Job | undefined; error: unknown }) {
  if (error) return <ErrorState error={error} />;
  if (!job) return <Loading />;
  if (job.status === "failed") {
    return (
      <div className="alert alert-error" role="alert">
        <CircleAlert size={ICON_MD} aria-hidden="true" />
        <div className="alert-body">{t("job.failed", { error: job.error ?? "" })}</div>
      </div>
    );
  }
  if (job.status === "succeeded") return null;
  return (
    <div className="alert alert-info" role="status" aria-live="polite">
      <LoaderCircle size={ICON_MD} className="spin" aria-hidden="true" />
      <div className="alert-body">
        <span>
          {job.status === "queued" && job.attempts === 0
            ? t("job.queued")
            : t("job.running", { attempt: Math.max(job.attempts, 1), max: job.maxAttempts })}
        </span>
        {job.error && job.status === "queued" && <span className="small">{t("job.retrying", { error: job.error })}</span>}
      </div>
    </div>
  );
}

export function ProvenanceNote({ p }: { p: Provenance }) {
  return (
    <div className="row wrap small muted">
      <AiBadge />
      <span>{t("wizard.provenance", { model: p.model, date: formatDateTime(p.generatedAt) })}</span>
      {p.editedByUser && <span>{t("wizard.provenanceEdited")}</span>}
    </div>
  );
}

export function TagList({ items }: { items: string[] }) {
  if (items.length === 0) return <p className="muted small">{t("common.none")}</p>;
  return (
    <ul className="tags">
      {items.map((it) => (
        <li key={it}>{it}</li>
      ))}
    </ul>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="field">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}
