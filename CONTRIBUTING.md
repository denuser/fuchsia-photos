# Contributing

Thanks for looking at Fuchsia Photos. Bug reports, ideas and pull requests are welcome.

## Before you start

- For a bug, open an issue with the version, how you installed it and what you did. Logs from `docker compose logs app` help a lot.
- For a new feature, open an issue or a [Discussion](https://github.com/denuser/fuchsia-photos/discussions) first, so we can agree on it before you spend time on code. The [roadmap](docs/roadmap.md) and the [improvements](docs/improvements.md) list what is planned, and [architecture](docs/architecture.md) records the decisions already made.
- The project is aimed at a modest home NAS. A change that needs another service or much more memory is unlikely to fit.

## Setting up

[Development](docs/development.md) explains how to run the backend and the frontend locally, how migrations and the API types work, and which commands run the tests.

## Making a change

- Backend changes come with tests. `make test` runs them against a real PostgreSQL in Docker. The frontend has one browser test of the main flow, `make e2e`; please do not add other frontend test frameworks.
- Text in the repository is in English: the README, docs, code comments and commit messages.
- Every string in the interface goes into `frontend/src/i18n/en.ts` and into the nine other language files under the same key. The type check fails when one is missing. If you do not speak a language, write your best translation and say so in the pull request.
- Add a line under "Unreleased" in [CHANGELOG.md](CHANGELOG.md) for anything users will notice.
- Keep a pull request to one change, with a message that says what changed and why.

## License

By contributing you agree that your work is released under the [MIT license](LICENSE).
