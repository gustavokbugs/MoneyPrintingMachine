import { useQuery } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { Channel } from "../api/types";
import { ErrorState, Loading, StatusBadge } from "../components/ui";
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
  const goTo = (s: StepId) => {
    setParams({ step: s });
    window.scrollTo({ top: 0 });
  };

  if (channel.isPending) return <Loading />;
  if (channel.isError) return <ErrorState error={channel.error} onRetry={() => void channel.refetch()} />;
  const c = channel.data;

  const props: StepProps = { channel: c, goTo };
  return (
    <>
      <div className="page-header">
        <div>
          <p className="small muted">
            <Link to="/launchpad">{t("nav.launchpad")}</Link>
          </p>
          <h1>{c.name || c.niche || t("dashboard.untitled")}</h1>
        </div>
        <StatusBadge status={c.status} />
      </div>

      <nav aria-label={t("wizard.steps")} className="stepper">
        <ol>
          {steps.map((s, i) => (
            <li key={s}>
              <button
                type="button"
                className={`step ${s === step ? "step-current" : ""} ${completed(s, c) ? "step-done" : ""}`}
                aria-current={s === step ? "step" : undefined}
                onClick={() => goTo(s)}
              >
                <span className="step-num" aria-hidden="true">
                  {completed(s, c) ? "✓" : i + 1}
                </span>
                {t(`wizard.step.${s}` as MessageKey)}
              </button>
            </li>
          ))}
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
