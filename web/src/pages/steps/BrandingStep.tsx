import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, AtSign, Check, Download, Paintbrush, Shuffle } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { Identity, NameAvailability, Palette } from "../../api/types";
import { downloadSvgAsPng, downloadUrl, slug } from "../../components/download";
import { EditableText } from "../../components/EditableText";
import { useToast } from "../../components/feedback";
import { GenerationPanel } from "../../components/GenerationPanel";
import { Badge, Button, ErrorState, Field, Notice, Section, TagList, errorMessage, type Tone } from "../../components/ui";
import { useGeneration } from "../../hooks/useGeneration";
import { t, type MessageKey } from "../../i18n";
import type { StepProps } from "../ChannelWizard";
import { useChannelUpdate } from "./shared";

const VARIANTS = 4;
const paletteKeys: (keyof Palette)[] = ["primary", "secondary", "accent", "background", "text"];

const availabilityTone: Record<NameAvailability, Tone> = {
  not_checked: "neutral",
  handle_taken: "danger",
  no_channel_with_handle: "success",
  invalid_handle: "warning",
};

function AvailabilityBadge({ availability, handle }: { availability: NameAvailability | ""; handle?: string }) {
  const a: NameAvailability = availability === "" ? "not_checked" : availability;
  return (
    <Badge tone={availabilityTone[a]} icon={a === "not_checked" ? undefined : AtSign}>
      {t(`branding.availability.${a}` as MessageKey, { handle: handle ?? "" })}
    </Badge>
  );
}

function AssetActions({ url, width, height, stem, onError }: { url: string; width: number; height: number; stem: string; onError: (msg: string) => void }) {
  const run = (fn: () => Promise<void>) => fn().catch((err: unknown) => onError(errorMessage(err)));
  return (
    <div className="row wrap">
      <Button size="sm" variant="ghost" icon={Download} onClick={() => run(() => downloadSvgAsPng(url, width, height, `${stem}.png`))}>
        {t("branding.downloadPng")}
      </Button>
      <Button size="sm" variant="ghost" icon={Download} onClick={() => run(() => downloadUrl(url, `${stem}.svg`))}>
        {t("branding.downloadSvg")}
      </Button>
    </div>
  );
}

export function BrandingStep({ channel, goTo }: StepProps) {
  const qc = useQueryClient();
  const toast = useToast();
  const gen = useGeneration(channel.id, "identity");
  const saveIdentity = useChannelUpdate(channel.id, (i: Identity) => api.putIdentity(channel.id, i));
  const setName = useChannelUpdate(channel.id, (name: string) => api.updateChannel(channel.id, { name }));
  const advance = useChannelUpdate(channel.id, () => api.changeStatus(channel.id, "awaiting_platform_setup"));
  const [variant, setVariant] = useState(0);
  const [customName, setCustomName] = useState("");
  const [paletteDraft, setPaletteDraft] = useState<Palette | null>(null);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const [checkWarning, setCheckWarning] = useState<string | null>(null);
  const idn = channel.identity;
  const locked = channel.status === "archived";

  const integrations = useQuery({ queryKey: ["integrations"], queryFn: api.integrations });
  const handleChecks = integrations.data?.youtubeDataApi.configured === true;
  const checkNames = useMutation({
    mutationFn: () => api.checkNames(channel.id),
    onSuccess: (res) => {
      qc.setQueryData(["channel", channel.id], res.channel);
      void qc.invalidateQueries({ queryKey: ["integrations"] });
      setCheckWarning(res.complete ? null : res.warning);
      if (res.complete) toast.success(t("branding.checkDone"));
    },
  });

  const edit = (patch: Partial<Identity>) => (idn ? saveIdentity.mutateAsync({ ...idn, ...patch }) : Promise.resolve());
  const stem = slug(channel.name || channel.niche);
  const avatarUrl = api.assetUrl(channel.id, "avatar", variant, channel.version);
  const bannerUrl = api.assetUrl(channel.id, "banner", variant, channel.version);

  const chooseName = (name: string, after?: () => void) =>
    setName.mutate(name, {
      onSuccess: () => {
        toast.success(t("branding.nameSelected", { name }));
        after?.();
      },
    });

  const next = () => {
    if (channel.status === "branding") advance.mutate(undefined, { onSuccess: () => goTo("platforms") });
    else goTo("platforms");
  };

  return (
    <div className="stack">
      <GenerationPanel
        title={t("branding.title")}
        description={t("branding.description.help")}
        gen={gen}
        provenance={idn?.provenance}
        disabled={gen.running || locked || !channel.strategy}
        emptyIcon={Paintbrush}
        emptyText={channel.strategy ? t("branding.empty") : t("branding.needsStrategy")}
      />

      {idn && (
        <>
          <Section
            title={t("branding.names")}
            description={t("branding.namesHint")}
            actions={
              <Button icon={AtSign} loading={checkNames.isPending} disabled={!handleChecks || locked} onClick={() => checkNames.mutate()}>
                {t("branding.checkNames")}
              </Button>
            }
          >
            {!handleChecks && integrations.isSuccess && <p className="small muted">{t("branding.checkUnavailable")}</p>}
            {checkNames.isError && <ErrorState error={checkNames.error} />}
            {checkWarning && (
              <Notice tone="warning" live>
                {t("branding.checkPartial", { reason: checkWarning })}
              </Notice>
            )}
            <ul className="name-grid">
              {idn.nameSuggestions.map((n) => {
                const selected = n.name === channel.name;
                return (
                  <li key={n.name} className={selected ? "name selected" : "name"}>
                    <strong>{n.name}</strong>
                    <AvailabilityBadge availability={n.availability} handle={n.handle} />
                    <p className="small muted">{n.rationale}</p>
                    <Button
                      size="sm"
                      variant={selected ? "primary" : "secondary"}
                      icon={selected ? Check : undefined}
                      loading={setName.isPending && setName.variables === n.name}
                      disabled={selected || locked}
                      aria-pressed={selected}
                      onClick={() => chooseName(n.name)}
                    >
                      {selected ? t("branding.selected") : t("branding.selectName")}
                    </Button>
                  </li>
                );
              })}
            </ul>
            <form
              className="field-group"
              onSubmit={(e) => {
                e.preventDefault();
                const name = customName.trim();
                if (name) chooseName(name, () => setCustomName(""));
              }}
            >
              <label htmlFor="custom-name">{t("branding.customName")}</label>
              <div className="input-row">
                <input id="custom-name" maxLength={100} value={customName} onChange={(e) => setCustomName(e.target.value)} />
                <Button type="submit" disabled={!customName.trim() || locked} loading={setName.isPending && setName.variables === customName.trim()}>
                  {t("branding.selectName")}
                </Button>
              </div>
            </form>
            {setName.isError && <ErrorState error={setName.error} />}
          </Section>

          <Section title={t("branding.copyTitle")}>
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

          <div className="grid-halves">
            <Section title={t("branding.palette")} description={t("branding.paletteHelp")}>
              <div className="palette">
                {paletteKeys.map((k) => {
                  const value = paletteDraft?.[k] ?? idn.palette[k];
                  const id = `swatch-${k}`;
                  return (
                    <div key={k} className="swatch">
                      <input
                        id={id}
                        type="color"
                        value={value}
                        disabled={locked}
                        onChange={(e) => setPaletteDraft({ ...(paletteDraft ?? idn.palette), [k]: e.target.value.toUpperCase() })}
                      />
                      <label htmlFor={id} className="xs">
                        {t(`branding.paletteKey.${k}` as MessageKey)}
                      </label>
                      <code className="xs">{value}</code>
                    </div>
                  );
                })}
              </div>
              {paletteDraft && (
                <div className="row">
                  <Button
                    size="sm"
                    variant="primary"
                    loading={saveIdentity.isPending}
                    onClick={() =>
                      edit({ palette: paletteDraft })
                        .then(() => {
                          setPaletteDraft(null);
                          toast.success(t("branding.paletteSaved"));
                        })
                        // The error is shown through saveIdentity.isError below.
                        .catch(() => undefined)
                    }
                  >
                    {t("common.save")}
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setPaletteDraft(null)}>
                    {t("common.cancel")}
                  </Button>
                </div>
              )}
              {saveIdentity.isError && <ErrorState error={saveIdentity.error} />}
            </Section>

            <Section title={t("branding.typography")}>
              <p>
                <strong>{idn.typography.headingFont}</strong> <span className="muted">/</span> {idn.typography.bodyFont}
              </p>
              <p className="small muted">{idn.typography.notes}</p>
            </Section>
          </div>

          <Section
            title={t("branding.assetsTitle")}
            description={t("branding.assetsHelp")}
            actions={
              <Button icon={Shuffle} onClick={() => setVariant((v) => (v + 1) % VARIANTS)}>
                {t("branding.variant")}
              </Button>
            }
          >
            <p className="xs muted" aria-live="polite">
              {t("branding.variantOf", { n: variant + 1, total: VARIANTS })}
            </p>
            <div className="assets">
              <figure>
                <img className="avatar-lg" src={avatarUrl} alt={t("branding.avatarAlt", { name: channel.name || channel.niche })} width={160} height={160} />
                <figcaption className="small muted">{idn.avatarConcept}</figcaption>
                <AssetActions url={avatarUrl} width={800} height={800} stem={`${stem}-avatar`} onError={setDownloadError} />
              </figure>
              <figure>
                <img className="banner" src={bannerUrl} alt={t("branding.bannerAlt", { name: channel.name || channel.niche })} width={2560} height={1440} />
                <figcaption className="small muted">{t("branding.bannerHint")}</figcaption>
                <AssetActions url={bannerUrl} width={2560} height={1440} stem={`${stem}-banner`} onError={setDownloadError} />
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
            <div className="row wrap">
              <Button variant="primary" icon={ArrowRight} loading={advance.isPending} disabled={!channel.name || locked} onClick={next}>
                {t("branding.toPlatforms")}
              </Button>
              {!channel.name && <span className="small muted">{t("branding.chooseNameFirst")}</span>}
            </div>
          </Section>
        </>
      )}
    </div>
  );
}
