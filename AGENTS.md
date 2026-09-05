# Working agreement

- Make changes on a feature branch, using the `codex/` prefix by default.
- Never merge a branch without the user's explicit approval of that merge.
- Leave completed changes available for review and report the branch and checks run.

# Frontend checks

- Run `make test-web` after changing forecast calculations or their browser integration.
- Keep forecast processing in `caddy/html/forecast.mjs` independent of the DOM.
