# Repository Instructions

## CLI output

- Keep `stdout` clean for command results and machine-readable output.
- Send interactive prompts, progress messages, and other human-oriented terminal UX to `stderr`.
- Preserve this split for Bubble Tea or other TUI flows so commands remain shell-friendly when `stdout` is redirected.
