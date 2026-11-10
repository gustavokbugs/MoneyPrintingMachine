import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, CircleCheck, Clock, Link2, Pause, Play, Plus, Radio, Tv } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { Channel } from "../api/types";
import { useToast } from "../components/feedback";
import { Button, Empty, ErrorState, Skeleton, StatusBadge, errorMessage, type Tone } from "../components/ui";
import { formatCalendarDate, t, type MessageKey } from "../i18n";

/** nextPublication returns the first calendar entry on or after today. */
export function nextPublication(c: Channel, today: string): string | null {
  const entry = c.blueprint?.calendar.find((e) => e.date >= today);
  return entry ? `${formatCalendarDate(entry.date)} ${entry.localTime}` : null;
}

function todayISO(): string {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export interface PortfolioSummary {
  total: number;
  active: number;
  needsSetup: number;
  connected: number;
}

/** summarize counts channels by lifecycle group; archived ones are excluded. */
export function summarize(channels: Channel[]): PortfolioSummary {
  const live = channels.filter((c) => c.status !== "archived");
  return {
    total: live.length,
    active: live.filter((c) => c.status === "active").length,
    needsSetup: live.filter((c) => ["draft", "branding", "awaiting_platform_setup"].includes(c.status)).length,
    connected: live.filter((c) => (c.connectedPlatforms?.length ?? 0) > 0).length,
  };
}

function Kpi({ icon: Icon, label, value, tone }: { icon: typeof Tv; label: string; value: number; tone?: Tone }) {
  return (
    <div className="kpi">
      <span className="kpi-label">
        <Icon size={16} aria-hidden="true" className={tone ? `tone-text-${tone}` : undefined} />
        {label}
      </span>
      <span className="kpi-value">{value}</span>
    </div>
  );
}

function TableSkeleton() {
  return (
    <div className="card" role="status" aria-label={t("common.loading")}>
      {Array.from({ length: 4 }, (_, i) => (
        <div key={i} className="row">
          <Skeleton height={32} width={32} />
          <Skeleton height={14} width="30%" />
          <Skeleton height={14} width="15%" />
          <Skeleton height={14} width="20%" />
        </div>
      ))}
    </div>
  );
}

export function Dashboard() {
  const qc = useQueryClient();
  const toast = useToast();
  const channels = useQuery({ queryKey: ["channels"], queryFn: () => api.listChannels() });
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const bulk = useMutation({
    mutationFn: (status: "paused" | "active") => api.bulkStatus([...selected], status),
    onSuccess: (res) => {
      const ok = res.results.filter((r) => r.ok).length;
      const skipped = res.results.length - ok;
      if (ok > 0) toast.success(t("dashboard.bulkResult", { ok, failed: skipped }));
      else toast.error(t("dashboard.bulkNone"));
      setSelected(new Set());
      void qc.invalidateQueries({ queryKey: ["channels"] });
    },
    onError: (err) => toast.error(errorMessage(err)),
  });

  const items = channels.data?.items ?? [];
  const summary = summarize(items);
  const today = todayISO();
  const allSelected = items.length > 0 && selected.size === items.length;

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  return (
    <>
      <div className="page-header">
        <div>
          <h1>{t("dashboard.title")}</h1>
          <p className="muted">{t("dashboard.subtitle")}</p>
        </div>
        <Link className="btn btn-primary" to="/launchpad">
          <Plus size={16} aria-hidden="true" />
          {t("dashboard.create")}
        </Link>
      </div>

      {channels.isSuccess && items.length > 0 && (
        <section aria-label={t("dashboard.summary")} className="kpis">
          <Kpi icon={Tv} label={t("dashboard.kpi.total")} value={summary.total} />
          <Kpi icon={Radio} label={t("dashboard.kpi.active")} value={summary.active} />
          <Kpi icon={Clock} label={t("dashboard.kpi.setup")} value={summary.needsSetup} />
          <Kpi icon={Link2} label={t("dashboard.kpi.connected")} value={summary.connected} />
        </section>
      )}

      {channels.isPending && <TableSkeleton />}
      {channels.isError && <ErrorState error={channels.error} onRetry={() => void channels.refetch()} />}
      {channels.isSuccess && items.length === 0 && (
        <Empty
          icon={Tv}
          action={
            <Link className="btn btn-primary" to="/launchpad">
              <Plus size={16} aria-hidden="true" />
              {t("dashboard.create")}
            </Link>
          }
        >
          <p>{t("dashboard.empty")}</p>
        </Empty>
      )}

      {items.length > 0 && (
        <section className="card" aria-labelledby="channels-heading">
          <header className="card-header">
            <div>
              <h2 id="channels-heading">{t("dashboard.tableTitle")}</h2>
              <p className="small muted">{t("dashboard.bulkHint")}</p>
            </div>
            <div className="toolbar" role="toolbar" aria-label={t("dashboard.bulkActions")}>
              <span className="small muted" aria-live="polite">
                {t("dashboard.selected", { count: selected.size })}
              </span>
              <Button icon={Pause} size="sm" disabled={selected.size === 0} loading={bulk.isPending && bulk.variables === "paused"} onClick={() => bulk.mutate("paused")}>
                {t("dashboard.bulkPause")}
              </Button>
              <Button icon={Play} size="sm" disabled={selected.size === 0} loading={bulk.isPending && bulk.variables === "active"} onClick={() => bulk.mutate("active")}>
                {t("dashboard.bulkResume")}
              </Button>
            </div>
          </header>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th scope="col" className="cell-check">
                    <input
                      type="checkbox"
                      aria-label={t("dashboard.selectAll")}
                      checked={allSelected}
                      onChange={() => setSelected(allSelected ? new Set() : new Set(items.map((c) => c.id)))}
                    />
                  </th>
                  <th scope="col">{t("dashboard.col.channel")}</th>
                  <th scope="col">{t("dashboard.col.status")}</th>
                  <th scope="col">{t("dashboard.col.niche")}</th>
                  <th scope="col">{t("dashboard.col.platforms")}</th>
                  <th scope="col">{t("dashboard.col.next")}</th>
                  <th scope="col">{t("dashboard.col.performance")}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((c) => {
                  const name = c.name || t("dashboard.untitled");
                  const platforms = c.connectedPlatforms ?? [];
                  const next = nextPublication(c, today);
                  return (
                    <tr key={c.id}>
                      <td className="cell-check">
                        <input type="checkbox" aria-label={t("dashboard.select", { name })} checked={selected.has(c.id)} onChange={() => toggle(c.id)} />
                      </td>
                      <th scope="row">
                        <Link to={`/channels/${c.id}`} className="channel-link">
                          {c.identity ? (
                            <img src={api.assetUrl(c.id, "avatar", 0, c.version)} alt="" width={32} height={32} className="avatar-sm" />
                          ) : (
                            <span className="avatar-placeholder" aria-hidden="true">
                              <Tv size={16} />
                            </span>
                          )}
                          {name}
                        </Link>
                      </th>
                      <td>
                        <StatusBadge status={c.status} />
                      </td>
                      <td>{c.niche || <span className="muted">—</span>}</td>
                      <td>
                        {platforms.length > 0 ? (
                          <span className="row small">
                            <CircleCheck size={16} aria-hidden="true" className="tone-text-success" />
                            {platforms.map((p) => t(`platform.${p}` as MessageKey)).join(", ")}
                          </span>
                        ) : (
                          <span className="muted small">{t("dashboard.noPlatforms")}</span>
                        )}
                      </td>
                      <td className="tabular">
                        {next ? (
                          <span className="row small">
                            <CalendarClock size={16} aria-hidden="true" />
                            {next}
                          </span>
                        ) : (
                          <span className="muted">—</span>
                        )}
                      </td>
                      <td className="muted small">{t("dashboard.noAnalytics")}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </>
  );
}
