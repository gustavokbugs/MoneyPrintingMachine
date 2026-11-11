import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Compass, Plus, Search, Sparkles, Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { api, newIdempotencyKey } from "../api/client";
import { asNicheReport } from "../api/guards";
import type { Niche, NicheReport } from "../api/types";
import { LaunchCard, useStartAutopilot } from "../components/AutopilotPanel";
import { useConfirm, useToast } from "../components/feedback";
import { sortNiches, type NicheSort } from "../components/Measured";
import { NicheCard, heuristicKeys } from "../components/NicheCard";
import { Badge, Button, Empty, ErrorState, JobProgress, Notice, RatingChip, Section, Skeleton, StatusBadge, errorMessage } from "../components/ui";
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

function parseReport(result: unknown): NicheReport | null {
  try {
    return asNicheReport(result);
  } catch {
    return null;
  }
}

const sourceTone = { connected: "success", not_configured: "warning", unavailable: "neutral" } as const;

export function Launchpad() {
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const [jobId, setJobId] = useState<string | null>(readLastJob);
  const [query, setQuery] = useState("");
  const [manualNiche, setManualNiche] = useState("");
  const [compare, setCompare] = useState<Set<string>>(new Set());
  const [sortBy, setSortBy] = useState<NicheSort>("heuristic");

  const job = useJob(jobId);
  const report = job.data?.status === "succeeded" ? parseReport(job.data.result) : null;
  const integrations = useQuery({ queryKey: ["integrations"], queryFn: api.integrations, refetchInterval: 60_000 });
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

  // Starting a channel from any niche runs the full automatic setup.
  const create = useStartAutopilot();

  const save = useMutation({
    mutationFn: (n: Niche) => api.saveNiche(n.name, n.description, n),
    onSuccess: (n) => {
      void qc.invalidateQueries({ queryKey: ["savedNiches"] });
      toast.success(t("launchpad.savedToast", { name: n.name }));
    },
    onError: (err) => toast.error(errorMessage(err)),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteSavedNiche(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["savedNiches"] }),
    onError: (err) => toast.error(errorMessage(err)),
  });

  const savedNames = new Set((saved.data ?? []).map((s) => s.name.toLowerCase()));
  const inProgress = (channels.data?.items ?? []).filter((c) => ["draft", "branding", "awaiting_platform_setup", "ready"].includes(c.status));
  const busy = discover.isPending || job.data?.status === "queued" || job.data?.status === "running";
  const quota = integrations.data?.youtubeDataApi.quota;

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

  const deleteSaved = async (id: string, name: string) => {
    const ok = await confirm({ title: t("launchpad.deleteTitle"), body: t("launchpad.deleteBody", { name }), confirmLabel: t("common.delete"), tone: "danger" });
    if (ok) remove.mutate(id);
  };

  const compared = report?.niches.filter((n) => compare.has(n.name)) ?? [];
  const sorted = report ? sortNiches(report.niches, sortBy) : [];

  return (
    <>
      <div className="page-header">
        <div>
          <h1>{t("launchpad.title")}</h1>
          <p className="muted">{t("launchpad.subtitle")}</p>
        </div>
        <Button variant="primary" icon={Sparkles} loading={discover.isPending} disabled={busy} onClick={() => discover.mutate("")}>
          {t("launchpad.discover")}
        </Button>
      </div>

      <LaunchCard />

      <div className="grid-2">
        <div className="stack">
          <Section title={t("launchpad.recommended")} description={t("launchpad.recommendedHelp")}>
            <form onSubmit={onSearch} className="field-group" role="search">
              <label htmlFor="niche-search">{t("launchpad.searchLabel")}</label>
              <div className="input-row">
                <input id="niche-search" value={query} maxLength={200} placeholder={t("launchpad.searchPlaceholder")} onChange={(e) => setQuery(e.target.value)} />
                <Button type="submit" icon={Search} disabled={busy || !query.trim()}>
                  {t("launchpad.search")}
                </Button>
              </div>
            </form>

            {discover.isError && <ErrorState error={discover.error} />}
            {jobId === null && !discover.isPending && (
              <Empty icon={Compass}>
                <p>{t("launchpad.noReport")}</p>
              </Empty>
            )}
            {jobId !== null && job.data?.status !== "succeeded" && (
              <>
                <JobProgress job={job.data} error={job.error} />
                {job.data?.status !== "failed" && (
                  <div className="stack-sm" aria-hidden="true">
                    <Skeleton height={120} />
                    <Skeleton height={120} />
                  </div>
                )}
              </>
            )}
            {job.data?.status === "succeeded" && !report && <ErrorState error={new Error(t("app.invalidReport"))} />}

            {report && (
              <>
                <Notice tone="info" title={t("launchpad.basisTitle")}>
                  <p>{report.basis}</p>
                  <p className="xs">
                    {report.model} · {formatDateTime(report.generatedAt)}
                  </p>
                </Notice>
                <div className="row wrap between">
                  <div className="row">
                    <label htmlFor="niche-sort">{t("launchpad.sortBy")}</label>
                    <select id="niche-sort" value={sortBy} onChange={(e) => setSortBy(e.target.value as NicheSort)}>
                      <option value="heuristic">{t("launchpad.sort.heuristic")}</option>
                      <option value="demand">{t("launchpad.sort.demand")}</option>
                      <option value="competition">{t("launchpad.sort.competition")}</option>
                    </select>
                  </div>
                  {quota && <span className="small muted tabular">{t("launchpad.quota", { used: quota.usedToday, limit: quota.limit })}</span>}
                </div>
                <ol className="niche-list">
                  {sorted.map((n, i) => (
                    <NicheCard
                      key={n.name}
                      niche={n}
                      rank={i + 1}
                      saved={savedNames.has(n.name.toLowerCase())}
                      compared={compare.has(n.name)}
                      busy={busy}
                      creating={create.isPending && create.variables === n.name}
                      onSelect={() => create.mutate(n.name)}
                      onExplore={() => {
                        setQuery(n.name);
                        discover.mutate(n.name);
                      }}
                      onSave={() => save.mutate(n)}
                      onToggleCompare={() => toggleCompare(n.name)}
                    />
                  ))}
                </ol>

                <div className="stack-sm">
                  <h3>{t("launchpad.sourcesTitle")}</h3>
                  <ul className="source-list">
                    {report.dataSources.map((ds) => (
                      <li key={ds.name}>
                        <strong className="small">{ds.name}</strong>
                        <Badge tone={sourceTone[ds.status]}>{t(`source.${ds.status}` as MessageKey)}</Badge>
                        <span className="small muted">{ds.note}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              </>
            )}
          </Section>

          {report && (
            <Section title={t("launchpad.compareTitle")}>
              {compared.length < 2 ? (
                <p className="muted small">{t("launchpad.compareHint")}</p>
              ) : (
                <div className="table-wrap">
                  <table>
                    <caption className="sr-only">{t("launchpad.compareTitle")}</caption>
                    <thead>
                      <tr>
                        <th scope="col">{t("launchpad.metric")}</th>
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
                          <td key={n.name} className="tabular">
                            {n.heuristicScore}
                          </td>
                        ))}
                      </tr>
                      <tr>
                        <th scope="row">{t("launchpad.demand")}</th>
                        {compared.map((n) => (
                          <td key={n.name} className="tabular">
                            {n.measured.demandScore ?? "—"}
                          </td>
                        ))}
                      </tr>
                      <tr>
                        <th scope="row">{t("launchpad.competition")}</th>
                        {compared.map((n) => (
                          <td key={n.name} className="tabular">
                            {n.measured.competitionScore ?? "—"}
                          </td>
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

        <aside className="stack" aria-label={t("launchpad.sidebar")}>
          <Section title={t("launchpad.manualTitle")} description={t("launchpad.manualHelp")}>
            <form
              className="field-group"
              onSubmit={(e) => {
                e.preventDefault();
                if (manualNiche.trim()) create.mutate(manualNiche.trim());
              }}
            >
              <label htmlFor="manual-niche">{t("launchpad.manualLabel")}</label>
              <div className="input-row">
                <input id="manual-niche" value={manualNiche} maxLength={120} onChange={(e) => setManualNiche(e.target.value)} />
                <Button type="submit" variant="secondary" icon={Plus} loading={create.isPending && create.variables === manualNiche.trim()} disabled={!manualNiche.trim()}>
                  {t("launchpad.manualCreate")}
                </Button>
              </div>
            </form>
          </Section>

          <Section title={t("launchpad.savedTitle")}>
            {saved.isPending && <Skeleton height={40} />}
            {saved.isError && <ErrorState error={saved.error} onRetry={() => void saved.refetch()} />}
            {saved.isSuccess && saved.data.length === 0 && <p className="muted small">{t("launchpad.savedEmpty")}</p>}
            <ul className="plain-list">
              {saved.data?.map((s) => (
                <li key={s.id} className="row between wrap">
                  <span className="small">{s.name}</span>
                  <span className="row">
                    <Button size="sm" variant="ghost" icon={Plus} onClick={() => create.mutate(s.name)}>
                      {t("launchpad.manualCreate")}
                    </Button>
                    <button type="button" className="btn btn-danger-ghost btn-sm btn-icon" aria-label={t("launchpad.deleteSaved", { name: s.name })} onClick={() => void deleteSaved(s.id, s.name)}>
                      <Trash2 size={16} aria-hidden="true" />
                    </button>
                  </span>
                </li>
              ))}
            </ul>
          </Section>

          <Section title={t("launchpad.channelsInProgress")}>
            {channels.isPending && <Skeleton height={40} />}
            {inProgress.length === 0 && channels.isSuccess && <p className="muted small">{t("common.none")}</p>}
            <ul className="plain-list">
              {inProgress.map((c) => (
                <li key={c.id} className="row between wrap">
                  <Link to={`/channels/${c.id}`} className="small">
                    {c.name || c.niche || t("dashboard.untitled")}
                  </Link>
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
