import type { ReactNode } from "react";
import { ApiError } from "../api/client";
import type { ChannelStatus, Job, Provenance, Rating } from "../api/types";
import { formatDateTime, t, type MessageKey } from "../i18n";

export function StatusBadge({ status }: { status: ChannelStatus }) {
  return <span className={`badge badge-${status}`}>{t(`status.${status}` as MessageKey)}</span>;
}

export function RatingChip({ rating }: { rating: Rating }) {
  return <span className={`chip chip-${rating}`}>{t(`rating.${rating}` as MessageKey)}</span>;
}

export function Loading({ label = t("common.loading") }: { label?: string }) {
  return (
    <p className="muted" role="status" aria-live="polite">
      <span className="spinner" aria-hidden="true" /> {label}
    </p>
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
      <strong>{t("common.errorTitle")}</strong>
      <p>{errorMessage(error)}</p>
      {onRetry && (
        <button type="button" className="btn btn-secondary" onClick={onRetry}>
          {t("common.retry")}
        </button>
      )}
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}

export function Section({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="card">
      <header className="card-header">
        <h2>{title}</h2>
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
        {t("job.failed", { error: job.error ?? "" })}
      </div>
    );
  }
  if (job.status === "succeeded") return null;
  return (
    <div className="alert alert-info" role="status" aria-live="polite">
      <span className="spinner" aria-hidden="true" />{" "}
      {job.status === "queued" && job.attempts === 0
        ? t("job.queued")
        : t("job.running", { attempt: Math.max(job.attempts, 1), max: job.maxAttempts })}
      {job.error && job.status === "queued" && <p className="small">{t("job.retrying", { error: job.error })}</p>}
    </div>
  );
}

export function ProvenanceNote({ p }: { p: Provenance }) {
  return (
    <p className="small muted">
      {t("wizard.provenance", { model: p.model, date: formatDateTime(p.generatedAt) })}{" "}
      {p.editedByUser && t("wizard.provenanceEdited")}
    </p>
  );
}

export function TagList({ items }: { items: string[] }) {
  if (items.length === 0) return <p className="muted">{t("common.none")}</p>;
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
