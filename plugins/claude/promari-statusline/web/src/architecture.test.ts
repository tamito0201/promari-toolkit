import assert from "node:assert/strict";
import { readdir } from "node:fs/promises";
import { test } from "node:test";
import * as ts from "typescript/unstable/ast";
import { API } from "typescript/unstable/sync";
import { fileURLToPath } from "node:url";

// 型参照も含む依存方向を固定する。構成ルートだけが View と外部 I/O を組み立てる。
const DEPENDENCIES: Record<string, readonly string[]> = {
  "model.ts": [],
  "definition.ts": [],
  "research.ts": ["definition.ts"],
  "research-panels.ts": ["research.ts", "measurements.ts"],
  "detail-catalog.ts": ["research-panels.ts"],
  "categories.ts": ["model.ts"],
  "routes.ts": [],
  "series.ts": ["model.ts"],
  "snapshot.ts": ["model.ts"],
  "measurements.ts": ["model.ts", "definition.ts", "research.ts", "format.ts"],
  "ports.ts": ["model.ts"],
  "format.ts": ["model.ts"],
  "presentation.ts": [
    "model.ts",
    "format.ts",
    "visualization.ts",
    "categories.ts",
    "series.ts",
    "routes.ts",
    "measurements.ts",
    "detail-catalog.ts",
    "research-panels.ts",
  ],
  "visualization.ts": ["model.ts", "measurements.ts"],
  "visualization-view.ts": ["visualization.ts", "dom.ts", "format.ts"],
  "chart-math.ts": [],
  "view-model.ts": ["ports.ts", "presentation.ts"],
  "api.ts": ["ports.ts", "model.ts", "snapshot.ts"],
  "browser.ts": ["ports.ts"],
  "dom.ts": [],
  "focus-view.ts": [
    "presentation.ts",
    "view-model.ts",
    "components.ts",
    "dom.ts",
  ],
  "chart-view.ts": [
    "presentation.ts",
    "format.ts",
    "dom.ts",
    "chart-math.ts",
    "visualization-view.ts",
  ],
  "components.ts": [
    "presentation.ts",
    "dom.ts",
    "chart-view.ts",
    "visualization-view.ts",
  ],
  "view.ts": [
    "focus-view.ts",
    "presentation.ts",
    "view-model.ts",
    "dom.ts",
    "components.ts",
    "visualization-view.ts",
  ],
  "main.ts": ["style.css", "api.ts", "browser.ts", "view-model.ts", "view.ts"],
};
const PURE_MODULES = new Set([
  "definition.ts",
  "research-panels.ts",
  "detail-catalog.ts",
  "categories.ts",
  "routes.ts",
  "series.ts",
  "snapshot.ts",
  "measurements.ts",
  "research.ts",
  "model.ts",
  "visualization.ts",
  "chart-math.ts",
  "ports.ts",
  "format.ts",
  "presentation.ts",
  "view-model.ts",
]);
const BROWSER_GLOBALS = new Set([
  "document",
  "window",
  "fetch",
  "setTimeout",
  "setInterval",
  "location",
  "localStorage",
  "sessionStorage",
  "XMLHttpRequest",
]);

test("MVVM の依存方向を守り、Model・ViewModel にブラウザ処理を持ち込まない", async () => {
  const root = new URL("./", import.meta.url);
  const files = (await readdir(root)).filter(
    (file) =>
      file.endsWith(".ts") &&
      !file.endsWith(".test.ts") &&
      !file.endsWith(".d.ts"),
  );
  const api = new API();
  try {
    const config = fileURLToPath(new URL("../tsconfig.json", import.meta.url));
    const snapshot = api.updateSnapshot({ openProjects: [config] });
    const project = snapshot.getProject(config);
    assert.ok(project);
    for (const file of files) {
      const allowed = DEPENDENCIES[file];
      assert.ok(
        allowed,
        `${file}: 追加したモジュールの依存方向を定義してください`,
      );
      const source = project.program.getSourceFile(
        fileURLToPath(new URL(file, root)),
      );
      assert.ok(source);
      function visit(node: ts.Node): void {
        if (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) {
          const module = node.moduleSpecifier;
          if (module && ts.isStringLiteral(module)) {
            const dependency = module.text.replace(/^\.\//u, "");
            assert.ok(
              allowed!.includes(dependency),
              `${file} → ${module.text} は依存方向に反します`,
            );
            if (
              [
                "view.ts",
                "focus-view.ts",
                "components.ts",
                "chart-view.ts",
                "visualization-view.ts",
              ].includes(file) &&
              ["presentation.ts", "view-model.ts", "visualization.ts"].includes(
                dependency,
              )
            ) {
              assert.ok(
                ts.isImportDeclaration(node) &&
                  node.importClause?.phaseModifier ===
                    ts.SyntaxKind.TypeKeyword,
                `${file}: ViewModel から参照できるのは表示用の型だけです`,
              );
            }
          }
        }
        if (ts.isCallExpression(node))
          assert.notEqual(
            node.expression.kind,
            ts.SyntaxKind.ImportKeyword,
            `${file}: 動的 import は依存検査を通りません`,
          );
        if (
          PURE_MODULES.has(file) &&
          ts.isIdentifier(node) &&
          BROWSER_GLOBALS.has(node.text)
        ) {
          assert.fail(
            `${file}: ${node.text} はブラウザ側のアダプターに移してください`,
          );
        }
        if (ts.isPropertyAccessExpression(node))
          assert.notEqual(
            node.name.text,
            "innerHTML",
            `${file}: 外部由来の文字列は textContent で表示してください`,
          );
        node.forEachChild(visit);
      }
      visit(source);
    }
  } finally {
    api.close();
  }
});
