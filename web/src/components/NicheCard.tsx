import { Bookmark, BookmarkCheck, Lightbulb, Search, Sparkles } from "lucide-react";
import type { Niche } from "../api/types";
import { t, type MessageKey } from "../i18n";
import { MeasuredBlock } from "./Measured";
import { Button, RatingChip, TagList } from "./ui";

export const heuristicKeys = [
  "audiencePotential",
  "monetizationPotential",
  "competitionEstimate",
  "productionDifficulty",
  "sourceAvailability",
  "originalityPotential",
] as const;

interface Props {
  niche: Niche;
  rank: number;
  saved: boolean;
  compared: boolean;
  busy: boolean;
  creating: boolean;
  onSelect: () => void;
  onExplore: () => void;
  onSave: () => void;
  onToggleCompare: () => void;
}

/**
 * NicheCard shows one recommendation. Model estimates and measured data are
 * visually and textually separated so estimates are never read as facts.
 */
export function NicheCard({ niche: n, rank, saved, compared, busy, creating, onSelect, onExplore, onSave, onToggleCompare }: Props) {
  const headingId = `niche-${rank}`;
  return (
    <li className="niche" aria-labelledby={headingId}>
      <div className="niche-head">
        <div className="stack-sm">
          <h3 id={headingId}>
            <span className="muted tabular">{rank}. </span>
            {n.name}
          </h3>
          <p className="small">{n.description}</p>
        </div>
        <div className="score">
          <span className="score-value">{n.heuristicScore}</span>
          <span className="xs muted">{t("launchpad.heuristicScore")}</span>
        </div>
      </div>

      <div className="subtle-panel">
        <span className="data-origin">
          <Sparkles size={14} aria-hidden="true" />
          {t("launchpad.estimatesTitle")}
        </span>
        <dl className="metric-grid">
          {heuristicKeys.map((k) => (
            <div key={k}>
              <dt>{t(`heuristic.${k}` as MessageKey)}</dt>
              <dd>
                <RatingChip rating={n.heuristics[k]} />
              </dd>
            </div>
          ))}
        </dl>
        {n.rationale && <p className="small muted">{n.rationale}</p>}
      </div>

      <MeasuredBlock niche={n} />

      <details>
        <summary>
          <Lightbulb size={16} aria-hidden="true" />
          {t("launchpad.details")}
        </summary>
        <div className="stack-sm">
          <h4>{t("launchpad.subNiches")}</h4>
          <TagList items={n.subNiches} />
          <h4>{t("launchpad.opportunities")}</h4>
          <TagList items={n.contentOpportunities} />
          <h4>{t("launchpad.formats")}</h4>
          <TagList items={n.suggestedFormats} />
        </div>
      </details>

      <div className="row wrap">
        <Button variant="primary" loading={creating} disabled={creating} onClick={onSelect}>
          {t("launchpad.select")}
        </Button>
        <Button icon={Search} disabled={busy} onClick={onExplore}>
          {t("launchpad.exploreSub")}
        </Button>
        <Button variant="ghost" icon={saved ? BookmarkCheck : Bookmark} disabled={saved} onClick={onSave}>
          {saved ? t("launchpad.saved") : t("launchpad.save")}
        </Button>
        <label className="check">
          <input type="checkbox" checked={compared} onChange={onToggleCompare} />
          {t("launchpad.compare")}
        </label>
      </div>
    </li>
  );
}
