import { Pencil } from "lucide-react";
import { useId, useRef, useState } from "react";
import { t } from "../i18n";
import { useToast } from "./feedback";
import { Button, errorMessage } from "./ui";

interface Props {
  label: string;
  value: string;
  multiline?: boolean;
  maxLength?: number;
  onSave: (next: string) => Promise<unknown>;
}

/** EditableText shows a value with an inline editor and save feedback. */
export function EditableText({ label, value, multiline = false, maxLength = 5000, onSave }: Props) {
  const [draft, setDraft] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const toast = useToast();
  const id = useId();
  const editButton = useRef<HTMLButtonElement>(null);

  const close = () => {
    setDraft(null);
    setError(null);
    // Return focus to the control that opened the editor.
    requestAnimationFrame(() => editButton.current?.focus());
  };

  if (draft === null) {
    return (
      <div className="field">
        <dt>{label}</dt>
        <dd className="editable">
          <span className={multiline ? "prewrap" : ""}>{value || <span className="muted">—</span>}</span>
          <button ref={editButton} type="button" className="btn btn-ghost btn-sm" onClick={() => setDraft(value)} aria-label={t("common.editField", { field: label })}>
            <Pencil size={14} aria-hidden="true" />
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
      toast.success(t("common.savedField", { field: label }));
      close();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const errorId = `${id}-error`;
  return (
    <div className="field">
      <dt>
        <label htmlFor={id}>{label}</label>
      </dt>
      <dd className="stack-sm">
        {multiline ? (
          <textarea
            id={id}
            rows={5}
            value={draft}
            maxLength={maxLength}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && close()}
            autoFocus
          />
        ) : (
          <input
            id={id}
            value={draft}
            maxLength={maxLength}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") close();
              if (e.key === "Enter" && !e.nativeEvent.isComposing) {
                e.preventDefault();
                void save();
              }
            }}
            autoFocus
          />
        )}
        {error && (
          <span id={errorId} className="error-text" role="alert">
            {error}
          </span>
        )}
        <div className="row">
          <Button size="sm" variant="primary" loading={saving} onClick={() => void save()}>
            {t("common.save")}
          </Button>
          <Button size="sm" variant="ghost" disabled={saving} onClick={close}>
            {t("common.cancel")}
          </Button>
        </div>
      </dd>
    </div>
  );
}
