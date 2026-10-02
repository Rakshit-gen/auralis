# Auralis design notes

The app is audio fiction you listen to at night, so the look is the sea after dark:
cold deep water, warm paper for text, one lamp. Tokens live in `tailwind.config.ts`
and `src/app/globals.css`. New pages use them; they never pick their own colors or
fonts.

## Color

| Token | Value | Job |
|---|---|---|
| `ink-950` to `ink-500` | `#04070c` to `#3c6072` | Ground and surfaces. Near-black with a cold blue cast. |
| `bone-100` to `bone-500` | `#f4efe6` to `#938669` | Text. Warm, so long passages read like paper. Body is `bone-200` or `bone-300`; `bone-500` is the lowest text color and still passes AA (4.9:1 on surfaces). |
| `amber` | `#e2a24d` | The main action on a screen (play, create account, save). One per view. |
| `signal` | `#54d0cc` | The story studio and live state only. If it is not AI or live, it is not teal. |

## Type

- Display: Iowan Old Style, falling back to Palatino and Georgia. Headings and show titles.
- Body: the system sans. Everything else.
- Mono: code and promo codes only.
- Sentence case everywhere, including buttons and table headers. No tracked all-caps labels.

## Surfaces

- `.surface` is a flat fill with a hairline border. No gradients, no colored glow.
- Blur only on the sticky nav and the player dock, where content scrolls underneath.
- The page background (radial water light plus the wave canvas) is the only atmosphere.
  Do not add blurred glow circles inside sections.

## Motion

Allowed: playback (waveforms, the voice bars), the pipeline stage timer, dialogs opening
(`animate-tide-in`), the slow sound-portal breathing on the landing page, color changes on
hover. Not allowed: fade-in on scroll or on mount, hover lifts or scaling, looping shine
effects on buttons or text. Everything respects `prefers-reduced-motion`.

## Words

- Say what the thing does. "Audio fiction that remembers exactly where you stopped" over
  "Every story opens a new world".
- Buttons name the action: "Create a free account", "Generate the series", "Redeem".
- No arrows glued to link text, no em dashes, no sparkle icon for AI.
- Labels above headings only when they add something the heading does not
  ("Try it, no account", "Tuned to your listening").
- Errors say what failed and what to do next.
- No numbers on the site unless they come from the API.
