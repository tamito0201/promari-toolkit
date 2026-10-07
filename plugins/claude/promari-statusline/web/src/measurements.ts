import type { Snapshot } from "./model.ts";
import {
  SOURCES,
  type MeasurementDefinition,
  type Source,
  type Unit,
} from "./definition.ts";
import { RESEARCH_MEASUREMENTS } from "./research.ts";
import { compact, decimal, duration, usd } from "./format.ts";

// 指標名・分類・単位・算出根拠。数値は表示文を解析せず API の集計値を使う。
export const MEASUREMENTS = {
  ...RESEARCH_MEASUREMENTS,
  sessionCost: {
    label: "Session cost",
    group: "Cost",
    unit: "usd",
    description:
      "Claude Code が報告したセッション費用",
  },
  todayCost: {
    label: "Today cost",
    group: "Cost",
    unit: "usd",
    description:
      "ccusage が集計した本日の全セッション費用",
  },
  blockCost: {
    label: "Block cost",
    group: "Cost",
    unit: "usd",
    description:
      "ccusage の稼働中の課金枠。稼働枠がない間は対象外",
  },
  blockEstimate: {
    label: "Block estimate",
    group: "Cost",
    unit: "usd",
    description:
      "枠の費用 + 消費ペース × 残り時間。稼働枠・ペース・残り時間が必要",
  },
  burnRate: {
    label: "Burn rate",
    group: "Burn",
    unit: "usd",
    description:
      "ccusage の稼働中の課金枠の1時間あたりの費用",
  },
  sessionBurn: {
    label: "Session burn",
    group: "Burn",
    unit: "usd",
    description:
      "セッション費用 ÷ 報告された経過時間（時間）。記録期間全体の平均",
  },
  costTurn: {
    label: "Cost / turn",
    group: "KPI",
    unit: "usd",
    description:
      "セッション費用 ÷ 記録済みターン数。ターンの記録が必要",
  },
  costLine: {
    label: "Cost / line",
    group: "KPI",
    unit: "usd",
    description:
      "セッション費用 ÷ 追加行数。追加行が0行の間は定義できない",
  },
  wall: {
    label: "Elapsed",
    group: "Burn",
    unit: "seconds",
    description:
      "Claude Code が報告した経過時間",
  },
  active: {
    label: "Active",
    group: "Burn",
    unit: "seconds",
    description:
      "描画間隔が5分以内だった区間の合計。保存済み活動履歴を集計",
  },
  idle: {
    label: "Idle",
    group: "Burn",
    unit: "seconds",
    description:
      "描画間隔が5分を超えた区間の合計。閲覧時間は加算しない",
  },
  deep: {
    label: "Deep work",
    group: "KPI",
    unit: "seconds",
    description:
      "23分以上続いた作業区間の合計。最終観測時刻で止める",
  },
  longest: {
    label: "Longest",
    group: "KPI",
    unit: "seconds",
    description:
      "最長の連続作業時間。最終観測時刻で止める",
  },
  focus: {
    label: "Focus",
    group: "KPI",
    unit: "percent",
    description:
      "Active ÷ (Active + Idle) × 100。記録済み観測時間が1分を超えると算出",
  },
  linesHour: {
    label: "Lines / h",
    group: "KPI",
    unit: "number",
    description:
      "追加行数 ÷ Active（時間）。記録済み稼働時間が15分を超えると算出",
  },
  parallel: {
    label: "Parallel",
    group: "Perf",
    unit: "ratio",
    description:
      "API 処理時間 ÷ セッション経過時間",
  },
  toolErrors: {
    label: "Tool errors",
    group: "Perf",
    unit: "percent",
    description:
      "エラー数 ÷ ツール実行数 × 100。実行0件では定義できない",
  },
  turnP50: {
    label: "Turn p50",
    group: "Perf",
    unit: "seconds",
    description:
      "完了した turn_duration の中央値。完了時間の記録が必要",
  },
  turnP90: {
    label: "Turn p90",
    group: "Perf",
    unit: "seconds",
    description:
      "完了した turn_duration の90パーセンタイル。完了時間の記録が必要",
  },
  cacheHit: {
    label: "Cache hit",
    group: "Cache",
    unit: "percent",
    description:
      "Claude Code の prompt_cache.hit_ratio × 100",
  },
  cacheSaved: {
    label: "Input saving",
    group: "Cache",
    unit: "percent",
    description:
      "キャッシュヒット率 × 90%。入力料金の削減目安で、セッション全体の削減率ではない",
  },
  cacheWrite: {
    label: "Cache write",
    group: "Cache",
    unit: "number",
    description:
      "Claude Code のキャッシュ書き込みトークン数",
  },
  inputTokens: {
    label: "Input tokens",
    group: "Tokens",
    unit: "number",
    description:
      "会話ログの入力 + キャッシュ書き込み + 読み込み。メッセージIDで重複除外",
  },
  outputTokens: {
    label: "Output tokens",
    group: "Tokens",
    unit: "number",
    description:
      "会話ログの出力トークン合計",
  },
  requests: {
    label: "Requests",
    group: "Tokens",
    unit: "number",
    description:
      "会話ログのモデル応答数。メッセージIDで重複除外",
  },
  thinking: {
    label: "Thinking share",
    group: "Tokens",
    unit: "percent",
    description:
      "思考トークン数 ÷ 出力トークン数 × 100",
  },
  turns: {
    label: "Turns",
    group: "Work",
    unit: "number",
    description:
      "描画時に記録した異なるプロンプトIDの数",
  },
  tools: {
    label: "Tool calls",
    group: "Work",
    unit: "number",
    description:
      "会話ログのツール呼び出し数",
  },
  edited: {
    label: "Edited files",
    group: "Work",
    unit: "number",
    description:
      "編集ツールが対象とした重複のないファイル数",
  },
  hooks: {
    label: "Hooks",
    group: "Work",
    unit: "number",
    description:
      "会話ログに記録されたフック実行数",
  },
  todoDone: {
    label: "Todo done",
    group: "Work",
    unit: "number",
    description:
      "セッションの Todo ファイルにある完了項目数。ファイルの取得が必要",
  },
  todoTotal: {
    label: "Todo total",
    group: "Work",
    unit: "number",
    description:
      "セッションの Todo ファイルにある全項目数。ファイルの取得が必要",
  },
  prompts: {
    label: "Prompts",
    group: "Agent",
    unit: "number",
    description:
      "会話ログの人間による入力数。ツール結果やシステム通知を除外",
  },
  autonomy: {
    label: "Autonomy",
    group: "Agent",
    unit: "ratio",
    description:
      "ツール呼び出し数 ÷ 人間の入力数。人間の入力が必要",
  },
  intervention: {
    label: "Intervention",
    group: "Agent",
    unit: "percent",
    description:
      "(中断 + 拒否) ÷ 人間の入力数 × 100",
  },
  exploreEdit: {
    label: "Explore / edit",
    group: "Trace",
    unit: "ratio",
    description:
      "探索回数 ÷ 編集回数。編集0回では定義できない",
  },
  explores: {
    label: "Explorations",
    group: "Trace",
    unit: "number",
    description:
      "Read・Grep・Glob・LS の実行数",
  },
  edits: {
    label: "Edits",
    group: "Trace",
    unit: "number",
    description:
      "編集ツールの実行回数",
  },
  changed: {
    label: "Changed files",
    group: "Git",
    unit: "number",
    description:
      "git status の変更ファイル数。クリーンなら0件",
  },
  staged: {
    label: "Staged",
    group: "Git",
    unit: "number",
    description:
      "ステージ済みファイル数",
  },
  inserted: {
    label: "Added lines",
    group: "Git",
    unit: "number",
    description:
      "HEAD に対する追加行数",
  },
  deleted: {
    label: "Deleted lines",
    group: "Git",
    unit: "number",
    description:
      "HEAD に対する削除行数",
  },
  untested: {
    label: "Untested files",
    group: "Quality",
    unit: "number",
    description:
      "編集後にテスト成功を確認できていないファイル数",
  },
  tests: {
    label: "Test runs",
    group: "Quality",
    unit: "number",
    description:
      "会話ログに記録されたテスト実行数",
  },
  builds: {
    label: "Build runs",
    group: "Quality",
    unit: "number",
    description:
      "会話ログに記録されたビルド実行数",
  },
  rules: {
    label: "Rule violations",
    group: "Rules",
    unit: "number",
    description:
      "作業ツリーの追加行から検出した規約違反数。未検査行は別集計",
  },
  rulesUnchecked: {
    label: "Unchecked lines",
    group: "Rules",
    unit: "number",
    description:
      "検査上限を超えて規約チェックできなかった行数",
  },
  commitStreak: {
    label: "Commit streak",
    group: "Habits",
    unit: "days",
    description:
      "本日または昨日まで連続してコミットした日数",
  },
  conventional: {
    label: "Conventional",
    group: "Habits",
    unit: "number",
    description:
      "本日の Conventional Commits 形式のコミット数",
  },
  commitsToday: {
    label: "Commits today",
    group: "Habits",
    unit: "number",
    description:
      "本日のコミット数",
  },
  cpuLoad: {
    label: "CPU load",
    group: "System",
    unit: "decimal",
    description:
      "OS の1分間の load average。CPU 使用率とは異なる",
  },
  cores: {
    label: "CPU cores",
    group: "System",
    unit: "number",
    description:
      "OS が報告したCPU数",
  },
  freeMemory: {
    label: "Free memory",
    group: "System",
    unit: "bytes",
    description:
      "OS が報告した利用可能メモリ",
  },
  freeDisk: {
    label: "Free disk",
    group: "System",
    unit: "bytes",
    description:
      "作業ディレクトリのディスク空き容量",
  },
  battery: {
    label: "Battery",
    group: "System",
    unit: "percent",
    description:
      "OS が報告したバッテリー残量",
  },
} as const satisfies Record<string, MeasurementDefinition>;

export type MeasurementKey = keyof typeof MEASUREMENTS;

// 定義ごとに異なる省略可能な項目（出典・評価）を、同じ型として読むための視点。
const DEFINITIONS: Readonly<Record<MeasurementKey, MeasurementDefinition>> =
  MEASUREMENTS;
const GIB = 2 ** 30;
const SECONDS_PER_MINUTE = 60;
const TINY = 0.01;
// 算出条件を満たさない間の理由。API が理由を返した場合はそちらを優先する。
const PENDING_NOTES: Partial<Readonly<Record<MeasurementKey, string>>> = {
  costLine: "追加行待ち",
  focus: "1分稼働待ち",
  linesHour: "15分稼働待ち",
  turnP50: "完了記録待ち",
  turnP90: "完了記録待ち",
  exploreEdit: "編集待ち",
  todoDone: "リスト待ち",
  todoTotal: "リスト待ち",
};

/** isMeasurementKey は URL などの外部文字列が既知の指標名かを、継承プロパティを除いて判定する。 */
export function isMeasurementKey(key: string): key is MeasurementKey {
  return Object.hasOwn(MEASUREMENTS, key);
}

/** 表示順（定義順）の全指標名。 */
export const MEASUREMENT_KEYS: readonly MeasurementKey[] = Object.keys(
  MEASUREMENTS,
).filter(isMeasurementKey);

export function measurementDefinition(
  key: MeasurementKey,
): MeasurementDefinition {
  return DEFINITIONS[key];
}

export function measurementSource(key: string): Source | undefined {
  if (!isMeasurementKey(key)) return undefined;
  const id = DEFINITIONS[key].source;
  return id === undefined ? undefined : SOURCES[id];
}

export function measurementNumber(
  s: Pick<Snapshot, "measurements">,
  key: MeasurementKey,
): number | undefined {
  const value = s.measurements[key]?.value;
  return value !== undefined && Number.isFinite(value) ? value : undefined;
}

function unreachable(value: never): never {
  throw new Error(`未対応の単位: ${String(value)}`);
}
function tiny(value: number): boolean {
  return value > 0 && value < TINY;
}
function formatValue(unit: Unit, value: number): string {
  switch (unit) {
    case "usd":
      return tiny(value) ? `$${value.toPrecision(2)}` : usd(value);
    case "percent":
      return `${decimal(value, 1)}%`;
    case "ratio":
      return `×${tiny(value) ? value.toPrecision(2) : decimal(value, 2)}`;
    case "seconds":
      return value < SECONDS_PER_MINUTE
        ? `${decimal(value, 1)}s`
        : duration(value * 1000);
    case "bytes":
      return `${decimal(value / GIB, 1)} GiB`;
    case "days":
      return `${decimal(value, 2)}d`;
    case "decimal":
      return decimal(value, 1);
    case "number":
      return compact(value, 1);
    default:
      return unreachable(unit);
  }
}

export function measurementText(
  s: Pick<Snapshot, "measurements">,
  key: MeasurementKey,
): string {
  const value = measurementNumber(s, key);
  // 理由（API の note か既知の算出条件）があればそれを、なければ「—」。
  if (value === undefined)
    return s.measurements[key]?.note ?? PENDING_NOTES[key] ?? "—";
  return formatValue(DEFINITIONS[key].unit, value);
}
