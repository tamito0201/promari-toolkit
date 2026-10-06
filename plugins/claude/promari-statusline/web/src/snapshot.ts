import { TONES } from "./model.ts";
import type {
  Band,
  Chip,
  CodexPoint,
  CodexWindow,
  ContextPoint,
  Group,
  Headline,
  Measurement,
  RatePoint,
  RateWindow,
  Session,
  Snapshot,
  Span,
  Tone,
} from "./model.ts";

// /api/snapshot の JSON を境界で検証する。契約に合わない応答は推測で補完せず拒否する。
type Fields = Readonly<Record<string, unknown>>;
type Parse<T> = (value: unknown, path: string) => T;

function invalid(path: string, expected: string): TypeError {
  return new TypeError(`/api/snapshot: ${path} は ${expected} ではありません`);
}
function isFields(value: unknown): value is Fields {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function isTone(value: unknown): value is Tone {
  return TONES.some((tone) => tone === value);
}
const fields: Parse<Fields> = (value, path) => {
  if (!isFields(value)) throw invalid(path, "オブジェクト");
  return value;
};
const text: Parse<string> = (value, path) => {
  if (typeof value !== "string") throw invalid(path, "文字列");
  return value;
};
const finite: Parse<number> = (value, path) => {
  if (typeof value !== "number" || !Number.isFinite(value))
    throw invalid(path, "有限の数値");
  return value;
};
const flag: Parse<boolean> = (value, path) => {
  if (typeof value !== "boolean") throw invalid(path, "真偽値");
  return value;
};
const tone: Parse<Tone> = (value, path) => {
  if (!isTone(value)) throw invalid(path, "既知の色名");
  return value;
};
function list<T>(parse: Parse<T>): Parse<readonly T[]> {
  return (value, path) => {
    if (!Array.isArray(value)) throw invalid(path, "配列");
    return value.map((item, index) => parse(item, `${path}[${index}]`));
  };
}
// Go 側は未知の値を省略する（omitzero）。null は契約にないので拒否する。
function optional<T>(parse: Parse<T>): Parse<T | undefined> {
  return (value, path) => (value === undefined ? undefined : parse(value, path));
}

const span: Parse<Span> = (value, path) => {
  const o = fields(value, path);
  return {
    text: text(o["text"], `${path}.text`),
    tone: optional(tone)(o["tone"], `${path}.tone`),
    bold: optional(flag)(o["bold"], `${path}.bold`),
    alarm: optional(flag)(o["alarm"], `${path}.alarm`),
  };
};
const chip: Parse<Chip> = (value, path) => ({
  spans: list(span)(fields(value, path)["spans"], `${path}.spans`),
});
const group: Parse<Group> = (value, path) => {
  const o = fields(value, path);
  return {
    band: finite(o["band"], `${path}.band`),
    title: text(o["title"], `${path}.title`),
    tone: tone(o["tone"], `${path}.tone`),
    chips: list(chip)(o["chips"], `${path}.chips`),
  };
};
const band: Parse<Band> = (value, path) => {
  const o = fields(value, path);
  return {
    band: finite(o["band"], `${path}.band`),
    name: text(o["name"], `${path}.name`),
    question: text(o["question"], `${path}.question`),
  };
};
const rateWindow: Parse<RateWindow> = (value, path) => {
  const o = fields(value, path);
  return {
    usedPct: finite(o["usedPct"], `${path}.usedPct`),
    resetsAt: optional(text)(o["resetsAt"], `${path}.resetsAt`),
  };
};
const headline: Parse<Headline> = (value, path) => {
  const o = fields(value, path);
  return {
    contextPct: optional(finite)(o["contextPct"], `${path}.contextPct`),
    fiveHour: optional(rateWindow)(o["fiveHour"], `${path}.fiveHour`),
    sevenDay: optional(rateWindow)(o["sevenDay"], `${path}.sevenDay`),
    sessionUsd: optional(finite)(o["sessionUsd"], `${path}.sessionUsd`),
    todayUsd: optional(finite)(o["todayUsd"], `${path}.todayUsd`),
    running: finite(o["running"], `${path}.running`),
  };
};
const session: Parse<Session> = (value, path) => {
  const o = fields(value, path);
  return {
    name: optional(text)(o["name"], `${path}.name`),
    model: optional(text)(o["model"], `${path}.model`),
    project: optional(text)(o["project"], `${path}.project`),
    version: optional(text)(o["version"], `${path}.version`),
  };
};
const ratePoint: Parse<RatePoint> = (value, path) => {
  const o = fields(value, path);
  return {
    at: text(o["at"], `${path}.at`),
    fiveHour: optional(finite)(o["fiveHour"], `${path}.fiveHour`),
    sevenDay: optional(finite)(o["sevenDay"], `${path}.sevenDay`),
  };
};
const codexWindow: Parse<CodexWindow> = (value, path) => {
  const o = fields(value, path);
  return {
    usedPct: finite(o["usedPct"], `${path}.usedPct`),
    windowMinutes: finite(o["windowMinutes"], `${path}.windowMinutes`),
  };
};
const codexPoint: Parse<CodexPoint> = (value, path) => {
  const o = fields(value, path);
  return {
    at: text(o["at"], `${path}.at`),
    primary: optional(codexWindow)(o["primary"], `${path}.primary`),
    secondary: optional(codexWindow)(o["secondary"], `${path}.secondary`),
  };
};
const contextPoint: Parse<ContextPoint> = (value, path) => {
  const o = fields(value, path);
  return {
    at: text(o["at"], `${path}.at`),
    tokens: finite(o["tokens"], `${path}.tokens`),
  };
};
const measurement: Parse<Measurement> = (value, path) => {
  const o = fields(value, path);
  return {
    value: optional(finite)(o["value"], `${path}.value`),
    note: optional(text)(o["note"], `${path}.note`),
  };
};
const measurements: Parse<Readonly<Record<string, Measurement>>> = (
  value,
  path,
) =>
  Object.fromEntries(
    Object.entries(fields(value, path)).map(([key, item]) => [
      key,
      measurement(item, `${path}.${key}`),
    ]),
  );

export function parseSnapshot(value: unknown): Snapshot {
  const o = fields(value, "snapshot");
  return {
    version: text(o["version"], "version"),
    at: text(o["at"], "at"),
    inputAt: optional(text)(o["inputAt"], "inputAt"),
    live: flag(o["live"], "live"),
    limitsAt: optional(text)(o["limitsAt"], "limitsAt"),
    session: session(o["session"], "session"),
    headline: headline(o["headline"], "headline"),
    bands: list(band)(o["bands"], "bands"),
    groups: list(group)(o["groups"], "groups"),
    rateHistory: list(ratePoint)(o["rateHistory"], "rateHistory"),
    codexHistory: list(codexPoint)(o["codexHistory"], "codexHistory"),
    contextHistory: list(contextPoint)(o["contextHistory"], "contextHistory"),
    measurements: measurements(o["measurements"], "measurements"),
  };
}
