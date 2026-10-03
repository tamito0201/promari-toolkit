# Contributing to Promari Toolkit

Place website plugins under `plugins/<component>/` and standalone tools under
`tools/<component>/`. Keep implementation, tests, configuration examples,
documentation, and the changelog with the component. Add each new component to
the root README catalog.

Each component is self-contained: its directory holds its own `package.json`,
`pnpm-lock.yaml`, and tool versions, and there is no root workspace. The
component directories here are published copies that change only on a release,
so changes are made upstream; run the component's `verify` command from its
directory to check a copy. Python and PHP tests belong to the component they exercise.

Use English for documentation, explanatory comments, commit titles, and commit
bodies. Localized UI strings and multilingual test data may remain in their
original language.

Version components independently. A release advances only the affected
component's manifest, configuration version, and changelog, together with its
rebuilt distribution files. Tags use `<component>-v<version>` and are never
moved after publication.

For Promari SNS Share, see its [contributor guide](plugins/wordpress/promari-sns-share/CONTRIBUTING.md).
