# Experimental UI

## Goal

Explore a different forecast interaction model while keeping the current Buitekant App look and feel.

The main page should remain the primary experience: same visual language, weather cards, charts, tables, dark/light theme, and compact weather-dashboard feel. The experiment changes how users inspect a specific day.

## Current Behavior

The main page shows a multi-day forecast summary. Selecting a day opens a detail popup with that day’s hourly rows and chart.

This works, but it creates a separate inspection surface. On small screens the popup can feel cramped and it makes the selected-day details feel disconnected from the main charts.

## Experiment

Replace the day-detail popup with an in-page selected-day state.

When a user selects a day pill/card:

- The selected day becomes active in the forecast card row.
- The main charts update to show the selected day’s available detailed entries.
- The table area updates to show all available entries for that selected day.
- The rest of the page keeps the current styling and structure as much as possible.

The data density may vary by day. Some days may have hourly entries, while later forecast days may only have a few entries, such as four time periods. The UI should adapt to the entries we receive rather than assuming a fixed hourly cadence.

## Design Constraints

- Preserve the existing visual identity: typography, colors, chart style, cards, spacing, and overall atmosphere.
- Avoid adding a modal, drawer, or popup for day details.
- Keep mobile interaction simple: tap a day, see the page update.
- Keep the forecast cards useful as navigation and summary.
- Make the selected-day state visually clear without overwhelming the card row.
- Do not hide important daily context; the page should still make sense as a forecast overview.

## Expected UI Shape

- Forecast card row remains near the top.
- Selecting a card updates a selected-day detail section below.
- Temperature, rain, wind, pressure, and hourly/period rows should reflect the selected day.
- If the selected day has sparse forecast entries, charts and tables should still render gracefully.
- If a metric is missing from an entry, show a quiet placeholder rather than breaking layout.

## Success Criteria

- No day-detail popup is needed.
- Users can move between days quickly by tapping the existing day cards.
- The selected day is obvious.
- Charts and tables do not assume exactly 24 hourly points.
- The page remains polished on iPhone-sized screens.
- The implementation remains simple enough to revert if the experiment does not work.

## Implementation Decision

For the first pass, the main charts become selected-day charts rather than remaining 10-day overview charts. The forecast card row keeps the multi-day context, and the selected day drives:

- Temperature chart
- Rain chart
- Wind chart
- Detail table
- Summary stat cards

## Open Questions

- Should there be a compact “10-day overview” plus a separate selected-day analysis area?
- Should today be selected by default on first load?
- Should selected day persist across reloads, or should only location persist?
