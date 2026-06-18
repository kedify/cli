# Repository Instructions

## CLI output

- Keep `stdout` clean for command results and machine-readable output.
- Send interactive prompts, progress messages, and other human-oriented terminal UX to `stderr`.
- Preserve this split for Bubble Tea or other TUI flows so commands remain shell-friendly when `stdout` is redirected.

## API pagination

- When an API endpoint returns a paginated response, the CLI should read all pages automatically.
- Hide pagination mechanics from the user and print only the combined result.
