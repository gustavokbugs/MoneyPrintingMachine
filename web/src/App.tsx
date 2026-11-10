import { useQuery } from "@tanstack/react-query";
import { Factory, LayoutDashboard, Monitor, Moon, Rocket, Sun } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { NavLink, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { api } from "./api/client";
import { Notice, ICON_MD } from "./components/ui";
import { t, type MessageKey } from "./i18n";
import { ChannelWizard } from "./pages/ChannelWizard";
import { Dashboard } from "./pages/Dashboard";
import { Launchpad } from "./pages/Launchpad";

type ThemeChoice = "system" | "light" | "dark";
const THEME_KEY = "theme";

function readTheme(): ThemeChoice {
  try {
    const v = localStorage.getItem(THEME_KEY);
    return v === "light" || v === "dark" ? v : "system";
  } catch {
    return "system";
  }
}

function applyTheme(choice: ThemeChoice) {
  const root = document.documentElement;
  if (choice === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", choice);
  try {
    if (choice === "system") localStorage.removeItem(THEME_KEY);
    else localStorage.setItem(THEME_KEY, choice);
  } catch {
    // Storage can be unavailable; the choice then lasts for this visit only.
  }
}

const themeIcons = { system: Monitor, light: Sun, dark: Moon };
const themeOrder: ThemeChoice[] = ["system", "light", "dark"];

function ThemeToggle() {
  const [theme, setTheme] = useState<ThemeChoice>(readTheme);
  useEffect(() => applyTheme(theme), [theme]);
  const Icon = themeIcons[theme];
  const next = themeOrder[(themeOrder.indexOf(theme) + 1) % themeOrder.length] ?? "system";
  return (
    <button
      type="button"
      className="btn btn-ghost btn-sm"
      onClick={() => setTheme(next)}
      aria-label={t("theme.label", { current: t(`theme.${theme}` as MessageKey), next: t(`theme.${next}` as MessageKey) })}
    >
      <Icon size={16} aria-hidden="true" />
      {t(`theme.${theme}` as MessageKey)}
    </button>
  );
}

function HealthBanner() {
  const { data } = useQuery({ queryKey: ["health"], queryFn: api.health, refetchInterval: 30_000 });
  if (!data || data.status === "ok") return null;
  const names = data.dependencies
    .filter((d) => d.status !== "ok")
    .map((d) => d.name.replace("_", " "))
    .join(", ");
  return (
    <Notice tone="warning" live>
      {t("health.degraded", { names })}
    </Notice>
  );
}

/** useFocusOnRouteChange moves focus to main content after navigation. */
function useFocusOnRouteChange() {
  const { pathname } = useLocation();
  const mainRef = useRef<HTMLElement>(null);
  // Comparing paths (not a "first run" flag) keeps this correct when React
  // runs effects twice in development.
  const previous = useRef(pathname);
  useEffect(() => {
    if (previous.current === pathname) return;
    previous.current = pathname;
    mainRef.current?.focus({ preventScroll: true });
    window.scrollTo({ top: 0 });
  }, [pathname]);
  return mainRef;
}

const navItems = [
  { to: "/", label: "nav.dashboard" as const, icon: LayoutDashboard, end: true },
  { to: "/launchpad", label: "nav.launchpad" as const, icon: Rocket, end: false },
];

export function App() {
  const mainRef = useFocusOnRouteChange();
  return (
    <div className="shell">
      <a className="skip-link" href="#main">
        {t("app.skip")}
      </a>
      <aside className="sidebar">
        <NavLink to="/" className="brand" end>
          <span className="brand-mark" aria-hidden="true">
            <Factory size={18} />
          </span>
          {t("app.name")}
        </NavLink>
        <nav aria-label={t("nav.main")} className="nav">
          {navItems.map(({ to, label, icon: Icon, end }) => (
            <NavLink key={to} to={to} end={end} className="nav-link">
              <Icon size={ICON_MD} aria-hidden="true" />
              {t(label)}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar-footer">
          <ThemeToggle />
        </div>
      </aside>
      <main id="main" className="main" ref={mainRef} tabIndex={-1}>
        <div className="content">
          <HealthBanner />
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/launchpad" element={<Launchpad />} />
            <Route path="/channels/:id" element={<ChannelWizard />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </div>
      </main>
    </div>
  );
}
