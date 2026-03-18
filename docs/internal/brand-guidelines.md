# archway — Brand Guidelines

## Name

Always lowercase: **archway** — never "Archway" or "ARCHWAY".

## Logo / Wordmark

The wordmark splits into two parts:

- **arch** — neutral color (white in dark mode, black in light mode)
- **way** — accent amber (`#E8913A` dark, `#C47A2E` light)

This applies everywhere the name appears as a visual element:
- Navbar site title (custom `SiteTitle.astro` component)
- Hero title on homepage (HTML in frontmatter: `arch<span style="color:var(--sl-color-accent)">way</span>`)
- Any future marketing material, social images, etc.

## Colors

"Structural Amber" palette — warm, architectural, AAA accessible. See `docs/internal/research/color-audit-rebranding-2026-03-14.md` for rationale.

| Token | Dark | Light | Usage |
|-------|------|-------|-------|
| Accent | `#E8913A` | `#8F5819` | "way" in logo, links, borders, h3 subcategories. Light mode darkened per [[contrast-of-saturation]]: amber deepened toward brown = "conscientious, dependable" per [[color-associations]], passes AA (5.0:1) on warm cream bg. |
| Accent low | `#1E1A14` | `#FFF5E8` | Backgrounds, subtle highlights |
| Accent high | `#F5D4A8` | `#5C3D1A` | Tagline, emphasis text |
| Background | `#111110` | `#F0EDE8` | Warm page background |
| Glow | `rgba(232,145,58,0.08)` | `rgba(196,122,46,0.06)` | Card hover shadow |

### Warm Gray Scale

All grays are warm-tinted toward the amber hue family. Per [[color-temperature]] (Itten): warm tones = near, dense, earthy — reinforces the "infrastructure" brand personality. Per [[functional-color]] (Birren): tinted backgrounds reduce monotony and fatigue on docs sites. No neutral/cool grays.

| Token | Dark | Light | Usage |
|-------|------|-------|-------|
| gray-1 | `#E8E4DE` | `#3D3A36` | Primary text |
| gray-2 | `#A8A29E` | `#57534E` | Secondary text, descriptions |
| gray-3 | `#78716C` | `#78716C` | Muted text, labels |
| gray-4 | `#57534E` | `#A8A29E` | Borders, dividers |
| gray-5 | `#3D3A36` | `#D6D0C8` | Subtle borders |
| gray-6 | `#1E1D1B` | `#EBE7E1` | Card backgrounds, surfaces |
| gray-7 | `#171614` | `#F5F2ED` | Elevated surfaces |
| Code bg | `#2a2520` | `#2a2520` | Code blocks (same in both modes) |

## Starlight Card & Aside Colors

Starlight's default card rotation (purple, orange, green, blue, red) is overridden with an amber-family rotation. Cards still have visual variety but stay within the brand palette.

| Starlight Token | Dark Mode | Light Mode |
|-----------------|-----------|------------|
| orange | `hsl(30, 80%, 63%)` | `hsl(30, 80%, 45%)` |
| purple | `hsl(25, 70%, 58%)` | `hsl(25, 70%, 42%)` |
| green | `hsl(45, 65%, 58%)` | `hsl(45, 65%, 42%)` |
| blue | `hsl(35, 60%, 55%)` | `hsl(35, 60%, 40%)` |
| red | `hsl(15, 75%, 58%)` | `hsl(15, 75%, 42%)` |

Aside semantic colors (tip, caution, danger) use Starlight defaults unless they clash visually with amber.

## Typography

- Site title: system font, weight 600
- Hero tagline: DM Serif Display (italic), loaded from Google Fonts
- Body: system font stack

## Rules

- Never use "Archway" with capital A in docs, website, or marketing
- The "way" color split is the primary brand element — maintain it in all logo contexts
- No emoji in the brand name or tagline

---

*Created 2026-03-11*
*Updated 2026-03-14 — Structural Amber rebrand*
