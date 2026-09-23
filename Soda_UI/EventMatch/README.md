# EventMatch

A responsive, eight-screen HTML/CSS/JavaScript frontend based on the supplied Figma exports. No framework, build step, API key, or package installation is needed.

## Open locally

Open `dist/index.html` in a modern browser. Keep `styles.css` and `app.js` beside it. For a local server: `python3 -m http.server 8000 --directory dist`.

## Files

- `dist/index.html`: semantic application shell and page metadata.
- `dist/styles.css`: olive brand palette, typography, components, and responsive layouts.
- `dist/app.js`: eight screen templates, navigation, form state, demo catalog, filtering, and optional WebMCP tool.

## Eight screens

1. `#event`: city, format, date.
2. `#category`: single category selection.
3. `#preferences`: budget, optional hours and language.
4. `#review`: editable requirement summary.
5. `#results`: up to three matching cards.
6. `#profile/demo-001`: details of a contractor in the current shortlist.
7. `#no-category`: no city/category combination in the catalog.
8. `#no-matches`: profiles exist, but none meet the conditions.

Results and profile screens require a completed search. The address hash enables back/forward navigation. Form state is in memory and resets on refresh. No personal data is collected or sent to a server.

## Demo data and limitations

The archive supplied for this task contains screenshots, not the hackathon dataset. The 12 records in `CATALOG` are all explicitly marked synthetic and labeled in the UI. They are illustrative, not real contractors. Availability is limited to September 23–December 31, 2026. Prices are starting rates rather than quotes. Optional language is treated as a strict requirement when selected; null maximum hours means services are not tied to on-site duration.

The filter runs in the browser. It excludes booked, over-budget, unsupported-format, language, and duration mismatches. Eligible records sort by starting price, then ID; the first three appear. Exclusion counts may overlap. Explanations use factual description text and requested constraints; this is not an LLM or an API integration. There are no bookings, outbound requests, ratings, or fabricated match scores.

To use the supplied hackathon JSONL, replace `CATALOG` and normalize category, event-format, and language labels to this UI's English values. Keep the source IDs and synthetic/imputed flags, and add visible disclosures if imported data includes imputed values. Verify matching policy with your backend team before production.

## Useful scenarios

- Two matches: Almaty, October 17, 2026, Corporate event, Host, ₸200,000, 5 hours, Kazakh.
- A different date: change October 17 to October 18; Arman's record is busy and Nurlan becomes eligible.
- Rare category: Almaty, October 17, 2026, Corporate event, Florist, ₸200,000, 5 hours, Kazakh. On-site hours are not applicable.
- Category absent: Astana + Florist.
- Conditions not met: Almaty, December 19, 2026, Corporate event, Host, ₸200,000, 5 hours, Kazakh.

Fonts use Google Fonts with local sans-serif fallbacks. Icons are inline SVG so the interface remains usable offline.
