# Contributing to Promari Toolkit

Place website plugins under `plugins/<component>/` and standalone tools under
`tools/<component>/`. Keep implementation, tests, configuration examples,
documentation, and the changelog with the component. Add each new component to
the root README catalog.

Use pnpm workspaces for JavaScript tooling. Run `pnpm install --frozen-lockfile`
from the root and the affected component's `verify` command before submitting
changes. Python and PHP tests belong to the component they exercise.

Use English for documentation, explanatory comments, commit titles, and commit
bodies. Localized UI strings and multilingual test data may remain in their
original language.

Version components independently. Update the affected component's manifest,
configuration version, and changelog, rebuild its distribution files, and commit
those files before publishing. Do not advance unrelated component versions.
Tags use `<component>-v<version>` and must never be moved after publication.

For Promari SNS Share, see its [contributor guide](plugins/promari-sns-share/CONTRIBUTING.md).
