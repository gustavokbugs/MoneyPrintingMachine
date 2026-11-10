import { useQuery } from "@tanstack/react-query";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api } from "./api/client";
import { t } from "./i18n";
import { ChannelWizard } from "./pages/ChannelWizard";
import { Dashboard } from "./pages/Dashboard";
import { Launchpad } from "./pages/Launchpad";

function HealthBanner() {
  const { data } = useQuery({ queryKey: ["health"], queryFn: api.health, refetchInterval: 30_000 });
  if (!data || data.status === "ok") return null;
  const names = data.dependencies
    .filter((d) => d.status !== "ok")
    .map((d) => d.name.replace("_", " "))
    .join(", ");
  return (
    <div className="alert alert-warning health" role="status">
      {t("health.degraded", { names })}
    </div>
  );
}

export function App() {
  return (
    <div className="shell">
      <a className="skip-link" href="#main">
        {t("app.skip")}
      </a>
      <header className="topbar">
        <span className="brand">{t("app.name")}</span>
        <nav aria-label={t("nav.main")}>
          <NavLink to="/" end>
            {t("nav.dashboard")}
          </NavLink>
          <NavLink to="/launchpad">{t("nav.launchpad")}</NavLink>
        </nav>
      </header>
      <HealthBanner />
      <main id="main" className="content">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/launchpad" element={<Launchpad />} />
          <Route path="/channels/:id" element={<ChannelWizard />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  );
}
