import { useState } from "react";
import { t } from "../i18n";
import { errorMessage } from "./ui";

interface Props {
  label: string;
  value: string;
  multiline?: boolean;
  maxLength?: number;
  onSave: (next: string) => Promise<unknown>;
}

/** EditableText shows a value with an inline editor. */
export function EditableText({ label, value, multiline = false, maxLength = 5000, onSave }: Props) {
  const [draft, setDraft] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const id = `edit-${label.replace(/\W+/g, "-").toLowerCase()}`;

  if (draft === null) {
    return (
      <div className="field">
        <dt>{label}</dt>
        <dd className="editable">
          <span className={multiline ? "prewrap" : ""}>{value || "—"}</span>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setDraft(value)} aria-label={`${t("common.edit")} ${label}`}>
            {t("common.edit")}
          </button>
        </dd>
      </div>
    );
  }

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      await onSave(draft);
      setDraft(null);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="field">
      <dt>
        <label htmlFor={id}>{label}</label>
      </dt>
      <dd>
        {multiline ? (
          <textarea id={id} rows={5} value={draft} maxLength={maxLength} onChange={(e) => setDraft(e.target.value)} />
        ) : (
          <input id={id} value={draft} maxLength={maxLength} onChange={(e) => setDraft(e.target.value)} />
        )}
        {error && (
          <p className="error-text" role="alert">
            {error}
          </p>
        )}
        <div className="row">
          <button type="button" className="btn btn-primary btn-sm" disabled={saving} onClick={() => void save()}>
            {saving ? t("common.saving") : t("common.save")}
          </button>
          <button type="button" className="btn btn-ghost btn-sm" disabled={saving} onClick={() => setDraft(null)}>
            {t("common.cancel")}
          </button>
        </div>
      </dd>
    </div>
  );
}
