import { RefreshCw, Sparkles, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Provenance } from "../api/types";
import type { useGeneration } from "../hooks/useGeneration";
import { t } from "../i18n";
import { Button, Empty, ErrorState, JobProgress, ProvenanceNote, Section } from "./ui";

interface Props {
  title: string;
  description?: string;
  gen: ReturnType<typeof useGeneration>;
  provenance: Provenance | undefined;
  disabled: boolean;
  emptyIcon: LucideIcon;
  emptyText: string;
  hint?: string;
  children?: ReactNode;
}

/**
 * GenerationPanel is the shared header of every AI-generated step: one
 * generate/regenerate action, job progress, an empty state, and the
 * AI-generated disclosure.
 */
export function GenerationPanel({ title, description, gen, provenance, disabled, emptyIcon, emptyText, hint, children }: Props) {
  const has = provenance !== undefined;
  const action = (
    <Button variant={has ? "secondary" : "primary"} icon={has ? RefreshCw : Sparkles} loading={gen.running} disabled={disabled} onClick={gen.start}>
      {has ? t("common.regenerate") : t("common.generate")}
    </Button>
  );
  return (
    <Section title={title} description={description} actions={has ? action : undefined}>
      {hint && <p className="small muted">{hint}</p>}
      {gen.startError && <ErrorState error={gen.startError} />}
      {gen.jobId && <JobProgress job={gen.job.data} error={gen.job.error} />}
      {!has && !gen.jobId && (
        <Empty icon={emptyIcon} action={action}>
          <p>{emptyText}</p>
        </Empty>
      )}
      {provenance && <ProvenanceNote p={provenance} />}
      {children}
    </Section>
  );
}
