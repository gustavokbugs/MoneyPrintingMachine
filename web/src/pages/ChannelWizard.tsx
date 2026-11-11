import { useQuery } from "@tanstack/react-query";
import { Check, ChevronRight } from "lucide-react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { Channel } from "../api/types";
import { AutopilotPanel } from "../components/AutopilotPanel";
import { ErrorState, SkeletonCard, StatusBadge } from "../components/ui";
import { t, type MessageKey } from "../i18n";
import { BlueprintStep } from "./steps/BlueprintStep";
import { BrandingStep } from "./steps/BrandingStep";
import { NicheStep } from "./steps/NicheStep";
import { PlatformStep } from "./steps/PlatformStep";
import { ReviewStep } from "./steps/ReviewStep";
import { StrategyStep } from "./steps/StrategyStep";

export const steps = ["niche", "strategy", "branding", "platforms", "blueprint", "review"] as const;
export type StepId = (typeof steps)[number];

function isStep(v: string | null): v is StepId {
  return v !== null && (steps as readonly string[]).includes(v);
}

/** completed reports whether a step's data exists. */
export function completed(step: StepId, c: Channel): boolean {
  switch (step) {
    case "niche":
      return c.niche !== "";
    case "strategy":
      return c.strategy !== null;
    case "branding":
      return c.identity !== null && c.name !== "";
    case "platforms":
      // Ready and later states require a verified connection.
      return c.status === "ready" || c.status === "active" || c.status === "paused";
    case "blueprint":
      return c.blueprint !== null;
    case "review":
      return c.status === "ready" || c.status === "active";
  }
}

export interface StepProps {
  channel: Channel;
  goTo: (step: StepId) => void;
}

export function ChannelWizard() {
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const channel = useQuery({ queryKey: ["channel", id], queryFn: () => api.getChannel(id) });

  const requested = params.get("step");
  const step: StepId = isStep(requested) ? requested : "niche";
  const autopilotJob = params.get("autopilot");
  const goTo = (s: StepId) => {
    setParams(autopilotJob ? { step: s, autopilot: autopilotJob } : { step: s });
    window.scrollTo({ top: 0 });
    document.getElementById("main")?.focus({ preventScroll: true });
  };

  if (channel.isPending) return <SkeletonCard lines={4} />;
  if (channel.isError) return <ErrorState error={channel.error} onRetry={() => void channel.refetch()} />;
  const c = channel.data;
  const title = c.name || c.niche || t("dashboard.untitled");
  const current = steps.indexOf(step);
  const doneCount = steps.filter((s) => completed(s, c)).length;

  const props: StepProps = { channel: c, goTo };
  return (
    <>
      <nav aria-label={t("wizard.breadcrumb")} className="breadcrumb">
        <ol>
          <li>
            <Link to="/launchpad">{t("nav.launchpad")}</Link>
            <ChevronRight size={14} aria-hidden="true" />
          </li>
          <li aria-current="page">{title}</li>
        </ol>
      </nav>
      <div className="page-header">
        <div>
          <h1>{title}</h1>
          <p className="small muted">{t("wizard.progress", { current: current + 1, total: steps.length, done: doneCount })}</p>
        </div>
        <div className="row">
          <Link className="btn btn-secondary btn-sm" to={`/channels/${c.id}/videos`}>
            {t("videos.manage")}
          </Link>
          <StatusBadge status={c.status} />
        </div>
      </div>

      <AutopilotPanel channel={c} jobId={autopilotJob} onStarted={(j) => setParams({ step, autopilot: j })} />

      <nav aria-label={t("wizard.steps")} className="stepper">
        <ol>
          {steps.map((s, i) => {
            const done = completed(s, c);
            return (
              <li key={s}>
                <button
                  type="button"
                  className={["step", s === step ? "step-current" : "", done ? "step-done" : ""].filter(Boolean).join(" ")}
                  aria-current={s === step ? "step" : undefined}
                  onClick={() => goTo(s)}
                >
                  <span className="step-num" aria-hidden="true">
                    {done ? <Check size={14} strokeWidth={3} /> : i + 1}
                  </span>
                  <span>
                    {t(`wizard.step.${s}` as MessageKey)}
                    {done && <span className="sr-only"> ({t("wizard.stepDone")})</span>}
                  </span>
                </button>
              </li>
            );
          })}
        </ol>
      </nav>

      {step === "niche" && <NicheStep {...props} />}
      {step === "strategy" && <StrategyStep {...props} />}
      {step === "branding" && <BrandingStep {...props} />}
      {step === "platforms" && <PlatformStep {...props} />}
      {step === "blueprint" && <BlueprintStep {...props} />}
      {step === "review" && <ReviewStep {...props} />}
    </>
  );
}
