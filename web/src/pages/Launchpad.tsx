import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, newIdempotencyKey } from "../api/client";
import { asNicheReport } from "../api/guards";
import type { Niche, NicheReport } from "../api/types";
import { Empty, ErrorState, JobProgress, Loading, RatingChip, Section, StatusBadge, TagList, errorMessage } from "../components/ui";
import { useJob } from "../hooks/useJob";
import { formatDateTime, t, type MessageKey } from "../i18n";

const LAST_JOB_KEY = "launchpad.lastDiscoveryJob";

function readLastJob(): string | null {
  try {
    return localStorage.getItem(LAST_JOB_KEY);
  } catch {
    return null;
  }
}

function writeLastJob(id: string) {
  try {
    localStorage.setItem(LAST_JOB_KEY, id);
  } catch {
    // Storage may be unavailable; the report simply will not persist.
  }
}

const heuristicKeys = [
  "audiencePotential",
  "monetizationPotential",
  "competitionEstimate",
  "productionDifficulty",
  "sourceAvailability",
  "originalityPotential",
] as const;

function parseReport(result: unknown): NicheReport | null {
  try {
    return asNicheReport(result);
  } catch {
    return null;
  }
}

export function Launchpad() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [jobId, setJobId] = useState<string | null>(readLastJob);
  const [query, setQuery] = useState("");
  const [manualNiche, setManualNiche] = useState("");
  const [compare, setCompare] = useState<Set<string>>(new Set());

  const job = useJob(jobId);
  const report = job.data?.status === "succeeded" ? parseReport(job.data.result) : null;

  const saved = useQuery({ queryKey: ["savedNiches"], queryFn: api.listSavedNiches });
  const channels = useQuery({ queryKey: ["channels"], queryFn: () => api.listChannels() });

  const discover = useMutation({
    mutationFn: (q: string) => api.discoverNiches(q, newIdempotencyKey()),
    onSuccess: (j) => {
      setJobId(j.id);
      writeLastJob(j.id);
      setCompare(new Set());
    },
  });

  const create = useMutation({
    mutationFn: (niche: string) => api.createChannel({ niche }),
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: ["channels"] });
      navigate(`/channels/${c.id}?step=strategy`);
    },
  });

  const save = useMutation({
    mutationFn: (n: Niche) => api.saveNiche(n.name, n.description, n),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["savedNiches"] }),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteSavedNiche(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["savedNiches"] }),
  });

  const savedNames = new Set((saved.data ?? []).map((s) => s.name.toLowerCase()));
  const inProgress = (channels.data?.items ?? []).filter((c) =>
    ["draft", "branding", "awaiting_platform_setup", "ready"].includes(c.status),
  );
  const busy = discover.isPending || job.data?.status === "queued" || job.data?.status === "running";

  const onSearch = (e: FormEvent) => {
    e.preventDefault();
    if (query.trim()) discover.mutate(query.trim());
  };

  const toggleCompare = (name: string) =>
    setCompare((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });

  const compared = report?.niches.filter((n) => compare.has(n.name)) ?? [];

  return (
    <>
      <div className="page-header">
        <div>
          <h1>{t("launchpad.title")}</h1>
          <p className="muted">{t("launchpad.subtitle")}</p>
        </div>
        <button type="button" className="btn btn-primary" disabled={busy} onClick={() => discover.mutate("")}>
          {t("launchpad.discover")}
        </button>
      </div>

      <div className="grid-2">
        <div className="stack">
          <Section title={t("launchpad.recommended")}>
            <form className="inline-form" onSubmit={onSearch}>
              <label htmlFor="niche-search">{t("launchpad.searchLabel")}</label>
              <div className="row">
                <input
                  id="niche-search"
                  value={query}
                  maxLength={200}
                  placeholder={t("launchpad.searchPlaceholder")}
                  onChange={(e) => setQuery(e.target.value)}
                />
                <button type="submit" className="btn btn-secondary" disabled={busy || !query.trim()}>
                  {t("launchpad.search")}
                </button>
              </div>
            </form>

            {discover.isError && <ErrorState error={discover.error} />}
            {jobId === null && !discover.isPending && <Empty>{t("launchpad.noReport")}</Empty>}
            {jobId !== null && job.data?.status !== "succeeded" && <JobProgress job={job.data} error={job.error} />}
            {job.data?.status === "succeeded" && !report && <ErrorState error={new Error(t("app.invalidReport"))} />}

            {report && (
              <>
                <div className="alert alert-info">
                  <strong>{t("launchpad.basisTitle")}</strong>
                  <p className="small">{report.basis}</p>
                  <p className="small muted">
                    {report.model} · {formatDateTime(report.generatedAt)}
                  </p>
                </div>
                <ol className="niche-list">
                  {report.niches.map((n) => (
                    <li key={n.name} className="niche">
                      <div className="niche-head">
                        <h3>{n.name}</h3>
                        <div className="score" title={t("common.estimate")}>
                          <span className="score-value">{n.heuristicScore}</span>
                          <span className="small muted">{t("launchpad.heuristicScore")}</span>
                        </div>
                      </div>
                      <p>{n.description}</p>
                      <p className="small muted">{n.rationale}</p>
                      <dl className="ratings">
                        {heuristicKeys.map((k) => (
                          <div key={k}>
                            <dt>{t(`heuristic.${k}` as MessageKey)}</dt>
                            <dd>
                              <RatingChip rating={n.heuristics[k]} />
                            </dd>
                          </div>
                        ))}
                        <div>
                          <dt>{t("launchpad.measuredTrend")}</dt>
                          <dd className="muted">{n.measured.trendScore ?? t("common.notMeasured")}</dd>
                        </div>
                        <div>
                          <dt>{t("launchpad.measuredCompetition")}</dt>
                          <dd className="muted">{n.measured.competitionScore ?? t("common.notMeasured")}</dd>
                        </div>
                      </dl>
                      <details>
                        <summary>{t("launchpad.subNiches")}</summary>
                        <TagList items={n.subNiches} />
                        <h4>{t("launchpad.opportunities")}</h4>
                        <TagList items={n.contentOpportunities} />
                        <h4>{t("launchpad.formats")}</h4>
                        <TagList items={n.suggestedFormats} />
                      </details>
                      <div className="row wrap">
                        <button
                          type="button"
                          className="btn btn-primary"
                          disabled={create.isPending}
                          onClick={() => create.mutate(n.name)}
                        >
                          {t("launchpad.select")}
                        </button>
                        <button
                          type="button"
                          className="btn btn-secondary"
                          disabled={busy}
                          onClick={() => {
                            setQuery(n.name);
                            discover.mutate(n.name);
                          }}
                        >
                          {t("launchpad.exploreSub")}
                        </button>
                        <button
                          type="button"
                          className="btn btn-ghost"
                          disabled={savedNames.has(n.name.toLowerCase()) || save.isPending}
                          onClick={() => save.mutate(n)}
                        >
                          {savedNames.has(n.name.toLowerCase()) ? t("launchpad.saved") : t("launchpad.save")}
                        </button>
                        <label className="check">
                          <input type="checkbox" checked={compare.has(n.name)} onChange={() => toggleCompare(n.name)} />
                          {t("launchpad.compare")}
                        </label>
                      </div>
                    </li>
                  ))}
                </ol>
                {create.isError && <ErrorState error={create.error} />}
                {save.isError && <p className="error-text">{errorMessage(save.error)}</p>}

                <h3>{t("launchpad.sourcesTitle")}</h3>
                <ul className="source-list">
                  {report.dataSources.map((ds) => (
                    <li key={ds.name}>
                      <strong>{ds.name}</strong> <span className={`chip chip-source-${ds.status}`}>{ds.status.replace("_", " ")}</span>
                      <span className="small muted"> {ds.note}</span>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </Section>

          {report && (
            <Section title={t("launchpad.compareTitle")}>
              {compared.length < 2 ? (
                <p className="muted">{t("launchpad.compareHint")}</p>
              ) : (
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th scope="col" />
                        {compared.map((n) => (
                          <th scope="col" key={n.name}>
                            {n.name}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      <tr>
                        <th scope="row">{t("launchpad.heuristicScore")}</th>
                        {compared.map((n) => (
                          <td key={n.name}>{n.heuristicScore}</td>
                        ))}
                      </tr>
                      {heuristicKeys.map((k) => (
                        <tr key={k}>
                          <th scope="row">{t(`heuristic.${k}` as MessageKey)}</th>
                          {compared.map((n) => (
                            <td key={n.name}>
                              <RatingChip rating={n.heuristics[k]} />
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </Section>
          )}
        </div>

        <aside className="stack">
          <Section title={t("launchpad.manualTitle")}>
            <form
              className="inline-form"
              onSubmit={(e) => {
                e.preventDefault();
                if (manualNiche.trim()) create.mutate(manualNiche.trim());
              }}
            >
              <label htmlFor="manual-niche">{t("launchpad.manualLabel")}</label>
              <div className="row">
                <input id="manual-niche" value={manualNiche} maxLength={120} onChange={(e) => setManualNiche(e.target.value)} />
                <button type="submit" className="btn btn-primary" disabled={!manualNiche.trim() || create.isPending}>
                  {t("launchpad.manualCreate")}
                </button>
              </div>
            </form>
          </Section>

          <Section title={t("launchpad.savedTitle")}>
            {saved.isPending && <Loading />}
            {saved.isError && <ErrorState error={saved.error} onRetry={() => void saved.refetch()} />}
            {saved.isSuccess && saved.data.length === 0 && <p className="muted">{t("launchpad.savedEmpty")}</p>}
            <ul className="plain-list">
              {saved.data?.map((s) => (
                <li key={s.id} className="row between">
                  <span>{s.name}</span>
                  <span className="row">
                    <button type="button" className="btn btn-ghost" onClick={() => create.mutate(s.name)}>
                      {t("launchpad.manualCreate")}
                    </button>
                    <button type="button" className="btn btn-ghost" onClick={() => remove.mutate(s.id)}>
                      {t("common.delete")}
                    </button>
                  </span>
                </li>
              ))}
            </ul>
          </Section>

          <Section title={t("launchpad.channelsInProgress")}>
            {channels.isPending && <Loading />}
            {inProgress.length === 0 && channels.isSuccess && <p className="muted">{t("common.none")}</p>}
            <ul className="plain-list">
              {inProgress.map((c) => (
                <li key={c.id} className="row between">
                  <Link to={`/channels/${c.id}`}>{c.name || c.niche || t("dashboard.untitled")}</Link>
                  <StatusBadge status={c.status} />
                </li>
              ))}
            </ul>
          </Section>
        </aside>
      </div>
    </>
  );
}
