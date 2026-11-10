import type { Niche } from "../api/types";
import { formatDateTime, t } from "../i18n";

export type NicheSort = "heuristic" | "demand" | "competition";

/**
 * sortNiches orders niches without mutating the input. Niches without a
 * measurement sort after measured ones for measured orderings.
 */
export function sortNiches(niches: Niche[], by: NicheSort): Niche[] {
  const out = [...niches];
  // The server already ranks by heuristic score (keeping an explored niche first).
  if (by === "heuristic") return out;
  const value = (n: Niche) => (by === "demand" ? n.measured.demandScore : n.measured.competitionScore);
  return out.sort((a, b) => {
    const va = value(a);
    const vb = value(b);
    if (va === null && vb === null) return b.heuristicScore - a.heuristicScore;
    if (va === null) return 1;
    if (vb === null) return -1;
    return by === "demand" ? vb - va : va - vb;
  });
}

const num = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
const compact = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 });

function Score({ label, value, suffix = "" }: { label: string; value: number | null; suffix?: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={value === null ? "muted" : "measured-value"}>
        {value === null ? t("launchpad.notMeasuredYet") : `${num.format(value)}${suffix}`}
      </dd>
    </div>
  );
}

/** MeasuredBlock shows YouTube measurements, clearly separated from estimates. */
export function MeasuredBlock({ niche }: { niche: Niche }) {
  const m = niche.measured;
  if (!m.metrics || !m.collectedAt) {
    return <p className="small muted">{t("common.notMeasured")}</p>;
  }
  const momentum = m.momentumPercent === null ? null : m.momentumPercent;
  return (
    <div className="measured">
      <p className="small">
        <strong>{t("launchpad.measuredTitle")}</strong>{" "}
        <span className="muted">{t("launchpad.collected", { date: formatDateTime(m.collectedAt) })}</span>
      </p>
      <dl className="ratings">
        <Score label={t("launchpad.demand")} value={m.demandScore} />
        <Score label={t("launchpad.competition")} value={m.competitionScore} />
        <Score label={t("launchpad.trend")} value={m.trendScore} />
        <Score label={t("launchpad.momentum")} value={momentum} suffix="%" />
        <div>
          <dt>{t("launchpad.medianViewsPerDay")}</dt>
          <dd>{compact.format(m.metrics.medianViewsPerDay)}</dd>
        </div>
        <div>
          <dt>{t("launchpad.channelsInSample", { count: m.metrics.sampleSize })}</dt>
          <dd>{m.metrics.distinctChannels}</dd>
        </div>
        <div>
          <dt>{t("launchpad.medianSubs")}</dt>
          <dd>{m.metrics.medianSubscribers === null ? "—" : compact.format(m.metrics.medianSubscribers)}</dd>
        </div>
      </dl>
      <p className="small muted">{t("launchpad.measuredHelp")}</p>
    </div>
  );
}
