import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { Channel } from "../api/types";
import { Empty, ErrorState, Loading, StatusBadge, errorMessage } from "../components/ui";
import { formatCalendarDate, t } from "../i18n";

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

export function Dashboard() {
  const qc = useQueryClient();
  const channels = useQuery({ queryKey: ["channels"], queryFn: () => api.listChannels() });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkMessage, setBulkMessage] = useState<string | null>(null);

  const bulk = useMutation({
    mutationFn: (status: "paused" | "active") => api.bulkStatus([...selected], status),
    onSuccess: (res) => {
      const ok = res.results.filter((r) => r.ok).length;
      setBulkMessage(t("dashboard.bulkResult", { ok, failed: res.results.length - ok }));
      setSelected(new Set());
      void qc.invalidateQueries({ queryKey: ["channels"] });
    },
    onError: (err) => setBulkMessage(errorMessage(err)),
  });

  const items = channels.data?.items ?? [];
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
          {t("dashboard.create")}
        </Link>
      </div>

      {channels.isPending && <Loading />}
      {channels.isError && <ErrorState error={channels.error} onRetry={() => void channels.refetch()} />}
      {channels.isSuccess && items.length === 0 && (
        <Empty>
          <p>{t("dashboard.empty")}</p>
          <Link className="btn btn-primary" to="/launchpad">
            {t("dashboard.create")}
          </Link>
        </Empty>
      )}

      {items.length > 0 && (
        <section className="card">
          <div className="toolbar">
            <button
              type="button"
              className="btn btn-secondary"
              disabled={selected.size === 0 || bulk.isPending}
              onClick={() => bulk.mutate("paused")}
            >
              {t("dashboard.bulkPause")}
            </button>
            <button
              type="button"
              className="btn btn-secondary"
              disabled={selected.size === 0 || bulk.isPending}
              onClick={() => bulk.mutate("active")}
            >
              {t("dashboard.bulkResume")}
            </button>
            <span className="small muted">{bulkMessage ?? t("dashboard.bulkHint")}</span>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th scope="col">
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
                  return (
                    <tr key={c.id}>
                      <td>
                        <input
                          type="checkbox"
                          aria-label={t("dashboard.select", { name })}
                          checked={selected.has(c.id)}
                          onChange={() => toggle(c.id)}
                        />
                      </td>
                      <td>
                        <Link to={`/channels/${c.id}`} className="channel-link">
                          {c.identity && (
                            <img src={api.assetUrl(c.id, "avatar", 0, c.version)} alt="" className="avatar-sm" />
                          )}
                          {name}
                        </Link>
                      </td>
                      <td>
                        <StatusBadge status={c.status} />
                      </td>
                      <td>{c.niche || "—"}</td>
                      <td className="muted">{t("dashboard.noPlatforms")}</td>
                      <td>{nextPublication(c, today) ?? "—"}</td>
                      <td className="muted">{t("dashboard.noAnalytics")}</td>
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
