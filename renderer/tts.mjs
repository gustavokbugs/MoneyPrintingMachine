// tts.mjs synthesizes one utterance with Kokoro through sherpa-onnx-node.
//
// The request arrives as a JSON file (or on stdin), never as command-line
// text:
// Windows passes arguments to native executables in the ANSI code page,
// which breaks accented text such as Portuguese ("você", "não").
//
// English uses the Kokoro lexicon. Other languages (for example "pt-br")
// are turned into IPA phonemes by espeak-ng (phonemes.mjs, run as its own
// process) and passed to Kokoro as phonemes, which is how Kokoro itself
// handles non-English text.
//
// Request: {model, voices, tokens, lexicon, lang, sid, lengthScale, threads, text, out}
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const sherpa = require("sherpa-onnx-node");

async function readStdin() {
  const chunks = [];
  for await (const c of process.stdin) chunks.push(c);
  return Buffer.concat(chunks).toString("utf8");
}

// toCodepoints keeps the phoneme characters Kokoro knows (tokens.txt) and
// splits sentences so long text stays within the model's context.
function toCodepoints(phonemes, known) {
  const out = [];
  for (const sentence of phonemes) {
    const cps = [];
    for (const ch of sentence) {
      if (known.has(ch)) cps.push(ch.codePointAt(0));
    }
    if (cps.length > 0) out.push(cps);
  }
  return out;
}

async function main() {
  // The request comes from a UTF-8 JSON file (argument) or stdin.
  const req = JSON.parse(process.argv[2] ? readFileSync(process.argv[2], "utf8") : await readStdin());
  for (const k of ["model", "voices", "tokens", "text", "out"]) {
    if (typeof req[k] !== "string" || req[k] === "") throw new Error(`missing ${k}`);
  }
  const lang = (req.lang || "").toLowerCase();
  const english = lang === "" || lang.startsWith("en");
  const tts = new sherpa.OfflineTts({
    model: {
      kokoro: {
        model: req.model,
        voices: req.voices,
        tokens: req.tokens,
        lexicon: req.lexicon || "",
        lengthScale: typeof req.lengthScale === "number" && req.lengthScale > 0 ? req.lengthScale : 1,
      },
      numThreads: req.threads || 4,
      provider: "cpu",
      debug: 0,
    },
    maxNumSentences: 1,
  });
  const request = { text: req.text, sid: req.sid || 0, speed: 1 };
  if (!english) {
    const known = new Set(
      readFileSync(req.tokens, "utf8").split("\n").map((l) => l.split(" ")[0]).filter((t) => t !== ""),
    );
    known.add(" ");
    const script = fileURLToPath(new URL("./phonemes.mjs", import.meta.url));
    const phonemes = execFileSync(process.execPath, [script, lang], { input: req.text, encoding: "utf8", timeout: 60000 })
      .split("\n")
      .filter((l) => l.trim() !== "");
    const codepoints = toCodepoints(phonemes, known);
    if (codepoints.length === 0) throw new Error(`no phonemes for language ${lang}`);
    request.generationConfig = { sid: req.sid || 0, speed: 1, phonemeCodepoints: codepoints };
  }
  const audio = tts.generate(request);
  if (!audio || !audio.samples || audio.samples.length === 0) throw new Error("no audio generated");
  sherpa.writeWave(req.out, { samples: audio.samples, sampleRate: audio.sampleRate });
}

main().catch((err) => {
  process.stderr.write(String(err && err.message ? err.message : err) + "\n");
  process.exit(1);
});
