// phonemes.mjs prints IPA phonemes for text read from stdin, one line per
// sentence, using espeak-ng (WebAssembly, GPL-3.0-or-later). It runs as its
// own process so the GPL tool stays separate from the rest of the project.
// Usage: node phonemes.mjs <espeak voice, e.g. pt-br> < text.txt
import ESpeakNg from "espeak-ng";

const voice = process.argv[2];
if (!voice || !/^[a-z]{2,3}(-[a-z0-9]{2,8})*$/i.test(voice)) {
  process.stderr.write("usage: node phonemes.mjs <voice>\n");
  process.exit(2);
}
const chunks = [];
for await (const c of process.stdin) chunks.push(c);
const text = Buffer.concat(chunks).toString("utf8").replace(/\s+/g, " ").trim();
if (text === "") process.exit(0);
const espeak = await ESpeakNg({
  arguments: ["--phonout", "out.txt", "-q", "-b", "1", "--ipa", "-v", voice, "-f", "in.txt"],
  preRun: [(m) => m.FS.writeFile("in.txt", text, { encoding: "utf8" })],
});
process.stdout.write(espeak.FS.readFile("out.txt", { encoding: "utf8" }));
