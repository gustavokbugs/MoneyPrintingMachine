import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { Identity, NameAvailability, Palette } from "../../api/types";
import { downloadSvgAsPng, downloadUrl, slug } from "../../components/download";
import { EditableText } from "../../components/EditableText";
import { Empty, ErrorState, Field, JobProgress, ProvenanceNote, Section, TagList, errorMessage } from "../../components/ui";
import { useGeneration } from "../../hooks/useGeneration";
import { t, type MessageKey } from "../../i18n";
import type { StepProps } from "../ChannelWizard";
import { useChannelUpdate } from "./shared";

const VARIANTS = 4;
const paletteKeys: (keyof Palette)[] = ["primary", "secondary", "accent", "background", "text"];

const availabilityClass: Record<NameAvailability, string> = {
  not_checked: "chip",
  handle_taken: "chip chip-low",
  no_channel_with_handle: "chip chip-high",
  invalid_handle: "chip chip-medium",
};

function AvailabilityChip({ availability, handle }: { availability: NameAvailability | ""; handle?: string }) {
  const a: NameAvailability = availability === "" ? "not_checked" : availability;
  return (
    <span className={availabilityClass[a]}>{t(`branding.availability.${a}` as MessageKey, { handle: handle ?? "" })}</span>
  );
}

export function BrandingStep({ channel, goTo }: StepProps) {
  const gen = useGeneration(channel.id, "identity");
  const saveIdentity = useChannelUpdate(channel.id, (i: Identity) => api.putIdentity(channel.id, i));
  const setName = useChannelUpdate(channel.id, (name: string) => api.updateChannel(channel.id, { name }));
  const advance = useChannelUpdate(channel.id, () => api.changeStatus(channel.id, "awaiting_platform_setup"));
  const [variant, setVariant] = useState(0);
  const [customName, setCustomName] = useState("");
  const [paletteDraft, setPaletteDraft] = useState<Palette | null>(null);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const idn = channel.identity;
  const locked = channel.status === "archived";

  const qc = useQueryClient();
  const integrations = useQuery({ queryKey: ["integrations"], queryFn: api.integrations });
  const handleChecks = integrations.data?.youtubeDataApi.configured === true;
  const [checkWarning, setCheckWarning] = useState<string | null>(null);
  const checkNames = useMutation({
    mutationFn: () => api.checkNames(channel.id),
    onSuccess: (res) => {
      qc.setQueryData(["channel", channel.id], res.channel);
      void qc.invalidateQueries({ queryKey: ["integrations"] });
      setCheckWarning(res.complete ? null : res.warning);
    },
  });

  const edit = (patch: Partial<Identity>) => (idn ? saveIdentity.mutateAsync({ ...idn, ...patch }) : Promise.resolve());
  const fileStem = slug(channel.name || channel.niche);

  const download = (fn: () => Promise<void>) => {
    setDownloadError(null);
    fn().catch((err: unknown) => setDownloadError(errorMessage(err)));
  };

  const next = () => {
    if (channel.status === "branding") {
      advance.mutate(undefined, { onSuccess: () => goTo("platforms") });
    } else {
      goTo("platforms");
    }
  };

  return (
    <div className="stack">
      <Section
        title={t("branding.title")}
        actions={
          <button type="button" className="btn btn-secondary" disabled={gen.running || locked || !channel.strategy} onClick={gen.start}>
            {idn ? t("common.regenerate") : t("common.generate")}
          </button>
        }
      >
        {gen.startError && <ErrorState error={gen.startError} />}
        {gen.jobId && <JobProgress job={gen.job.data} error={gen.job.error} />}
        {!idn && !gen.jobId && <Empty>{t("branding.empty")}</Empty>}
        {idn && <ProvenanceNote p={idn.provenance} />}
      </Section>

      {idn && (
        <>
          <Section
            title={t("branding.names")}
            actions={
              <button
                type="button"
                className="btn btn-secondary"
                disabled={!handleChecks || checkNames.isPending || locked}
                title={handleChecks ? undefined : t("branding.checkUnavailable")}
                onClick={() => checkNames.mutate()}
              >
                {checkNames.isPending ? t("branding.checking") : t("branding.checkNames")}
              </button>
            }
          >
            <p className="small muted">{t("branding.namesHint")}</p>
            {!handleChecks && integrations.isSuccess && <p className="small muted">{t("branding.checkUnavailable")}</p>}
            {checkNames.isError && <ErrorState error={checkNames.error} />}
            {checkWarning && (
              <div className="alert alert-warning" role="status">
                {t("branding.checkPartial", { reason: checkWarning })}
              </div>
            )}
            <ul className="name-grid">
              {idn.nameSuggestions.map((n) => {
                const selected = n.name === channel.name;
                return (
                  <li key={n.name} className={selected ? "name selected" : "name"}>
                    <strong>{n.name}</strong>
                    <AvailabilityChip availability={n.availability} handle={n.handle} />
                    <p className="small muted">{n.rationale}</p>
                    <button
                      type="button"
                      className={selected ? "btn btn-primary btn-sm" : "btn btn-secondary btn-sm"}
                      disabled={selected || setName.isPending || locked}
                      aria-pressed={selected}
                      onClick={() => setName.mutate(n.name)}
                    >
                      {selected ? t("branding.selected") : t("branding.selectName")}
                    </button>
                  </li>
                );
              })}
            </ul>
            <form
              className="inline-form"
              onSubmit={(e) => {
                e.preventDefault();
                if (customName.trim()) setName.mutate(customName.trim(), { onSuccess: () => setCustomName("") });
              }}
            >
              <label htmlFor="custom-name">{t("branding.customName")}</label>
              <div className="row">
                <input id="custom-name" maxLength={100} value={customName} onChange={(e) => setCustomName(e.target.value)} />
                <button type="submit" className="btn btn-secondary" disabled={!customName.trim() || setName.isPending || locked}>
                  {t("branding.selectName")}
                </button>
              </div>
            </form>
            {setName.isError && <ErrorState error={setName.error} />}
          </Section>

          <Section title={t("branding.description")}>
            <dl className="fields">
              <EditableText label={t("branding.bio")} value={idn.shortBio} maxLength={300} onSave={(v) => edit({ shortBio: v })} />
              <EditableText label={t("branding.description")} value={idn.fullDescription} multiline onSave={(v) => edit({ fullDescription: v })} />
              <EditableText label={t("branding.positioning")} value={idn.positioningStatement} multiline onSave={(v) => edit({ positioningStatement: v })} />
              <EditableText label={t("branding.tagline")} value={idn.tagline} onSave={(v) => edit({ tagline: v })} />
              <Field label={t("branding.keywords")}>
                <TagList items={idn.keywords} />
              </Field>
            </dl>
          </Section>

          <Section title={t("branding.palette")}>
            <div className="palette">
              {paletteKeys.map((k) => {
                const value = paletteDraft?.[k] ?? idn.palette[k];
                return (
                  <label key={k} className="swatch">
                    <input
                      type="color"
                      value={value}
                      disabled={locked}
                      onChange={(e) => setPaletteDraft({ ...(paletteDraft ?? idn.palette), [k]: e.target.value.toUpperCase() })}
                    />
                    <span className="small">{k}</span>
                    <code className="small">{value}</code>
                  </label>
                );
              })}
            </div>
            {paletteDraft && (
              <div className="row">
                <button
                  type="button"
                  className="btn btn-primary btn-sm"
                  disabled={saveIdentity.isPending}
                  onClick={() =>
                    edit({ palette: paletteDraft })
                      .then(() => setPaletteDraft(null))
                      // The error is shown through saveIdentity.isError below.
                      .catch(() => undefined)
                  }
                >
                  {saveIdentity.isPending ? t("common.saving") : t("common.save")}
                </button>
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => setPaletteDraft(null)}>
                  {t("common.cancel")}
                </button>
              </div>
            )}
            {saveIdentity.isError && <ErrorState error={saveIdentity.error} />}
            <h3>{t("branding.typography")}</h3>
            <p>
              {idn.typography.headingFont} / {idn.typography.bodyFont}
            </p>
            <p className="small muted">{idn.typography.notes}</p>
          </Section>

          <Section
            title={`${t("branding.avatar")} · ${t("branding.banner")}`}
            actions={
              <button type="button" className="btn btn-secondary" onClick={() => setVariant((v) => (v + 1) % VARIANTS)}>
                {t("branding.variant")}
              </button>
            }
          >
            <div className="assets">
              <figure>
                <img
                  className="avatar-lg"
                  src={api.assetUrl(channel.id, "avatar", variant, channel.version)}
                  alt={t("branding.avatar")}
                  width={160}
                  height={160}
                />
                <figcaption className="small muted">{idn.avatarConcept}</figcaption>
                <div className="row">
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() =>
                      download(() => downloadSvgAsPng(api.assetUrl(channel.id, "avatar", variant, channel.version), 800, 800, `${fileStem}-avatar.png`))
                    }
                  >
                    {t("branding.downloadPng")}
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => download(() => downloadUrl(api.assetUrl(channel.id, "avatar", variant, channel.version), `${fileStem}-avatar.svg`))}
                  >
                    {t("branding.downloadSvg")}
                  </button>
                </div>
              </figure>
              <figure className="banner-fig">
                <img className="banner" src={api.assetUrl(channel.id, "banner", variant, channel.version)} alt={t("branding.banner")} />
                <figcaption className="small muted">{t("branding.bannerHint")}</figcaption>
                <div className="row">
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() =>
                      download(() => downloadSvgAsPng(api.assetUrl(channel.id, "banner", variant, channel.version), 2560, 1440, `${fileStem}-banner.png`))
                    }
                  >
                    {t("branding.downloadPng")}
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => download(() => downloadUrl(api.assetUrl(channel.id, "banner", variant, channel.version), `${fileStem}-banner.svg`))}
                  >
                    {t("branding.downloadSvg")}
                  </button>
                </div>
              </figure>
            </div>
            {downloadError && (
              <p className="error-text" role="alert">
                {downloadError}
              </p>
            )}
          </Section>

          <Section title={t("branding.guidelines")}>
            <dl className="fields">
              <EditableText label={t("branding.thumbnail")} value={idn.thumbnailStyle} multiline onSave={(v) => edit({ thumbnailStyle: v })} />
              <EditableText label={t("branding.captions")} value={idn.captionStyle} multiline onSave={(v) => edit({ captionStyle: v })} />
              <EditableText label={t("branding.motion")} value={idn.motionGuidelines} multiline onSave={(v) => edit({ motionGuidelines: v })} />
              <EditableText label={t("branding.introOutro")} value={idn.introOutroGuidelines} multiline onSave={(v) => edit({ introOutroGuidelines: v })} />
            </dl>
            {advance.isError && <ErrorState error={advance.error} />}
            <div className="row">
              <button type="button" className="btn btn-primary" disabled={!channel.name || advance.isPending || locked} onClick={next}>
                {t("branding.toPlatforms")}
              </button>
            </div>
          </Section>
        </>
      )}
    </div>
  );
}
