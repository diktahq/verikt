# verikt Documentation Site

Built with [Astro](https://astro.build) + [Starlight](https://starlight.astro.build).

## Development

```bash
cd website
bun install
bun run dev       # localhost:4321
bun run build     # production build to ./dist/
```

Docs live in `src/content/docs/`. Each `.md` file becomes a route.

## Deployment

Auto-deploys to GitHub Pages on push to `main` when files in `website/` change.
