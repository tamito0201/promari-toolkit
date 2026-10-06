import "./style.css";
import { createSnapshotRepository } from "./api.ts";
import {
  browserScheduler,
  browserNavigation,
  browserVisibility,
  browserFullscreen,
} from "./browser.ts";
import { createDashboardViewModel } from "./view-model.ts";
import { createDashboardView } from "./view.ts";

// 構成ルートだけが具体的な実装を知り、各層を接続する。
const model = createDashboardViewModel({
  repository: createSnapshotRepository(window.fetch.bind(window)),
  scheduler: browserScheduler(window),
  navigation: browserNavigation(window),
  visibility: browserVisibility(document),
  fullscreen: browserFullscreen(document),
});
const view = createDashboardView(model.actions);
const unsubscribe = model.subscribe(view.render);
model.start();
window.addEventListener("pagehide", (event) => {
  if (event.persisted) return;
  unsubscribe();
  view.dispose();
  model.dispose();
});
