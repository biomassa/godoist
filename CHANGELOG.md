# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Terminal UI with three panes: sidebar, task list, and details.
- Sidebar with Today, Upcoming, Inbox, Favorites, and the project tree, with task counts.
- Project colors from the Todoist palette on the sidebar, the selected row, and the focused border.
- Priority colors on task circles. Due date colors for overdue, today, tomorrow, and this week.
- Task list grouped by section in project order, with empty sections.
- Today view with Overdue and Today groups. Upcoming view grouped by day.
- Quick add (`a`) with Todoist natural-language parsing for dates, `#project`, `@label`, and `p1`–`p4`.
- Quick add in a project puts the task in the section under the cursor. A `#` that does not name a project stays in the task text.
- Quick add in Today or Upcoming sets the date of that day if the text has no date.
- Task completion (`x`) and undo (`ctrl+z`). Undo is not available for recurring tasks.
- Find in the current view (`/`) and server-side Todoist filter queries (`f`). An empty query removes the filter view. Plain words search the task names. A failed query opens the dialog again. `esc` clears the filter and goes back to the previous view. The legend shows `esc clear filter` and `esc clear find`.
- Comments in the details pane, with attachments. Add (`c`), edit (`e`), and delete (`d`) comments.
- Task name editing (`e`) in a dialog. Todoist parses the text as in quick add: a date, `#project`, `/section`, `@label`, or `p1`–`p4` changes only that field. In notebook view, the name is saved as typed.
- A centered dialog for all text inputs except find. Long text wraps, `enter` saves, `esc` cancels.
- Task description editing (`E`).
- Date dialog (`t`): a text field that Todoist parses, a month calendar, and a time field. `tab` moves between them and parses a changed text once, so the calendar shows the result. `enter` saves the input changed last. `no date` removes the date.
- Calendar: arrows, PgUp/PgDn, Home, quick picks (Today, Tomorrow, Next week, Weekend, No date), a dot on days with tasks, and the week start from the Todoist settings. The mouse selects days and quick picks, a double-click saves, and the wheel changes the month.
- A calendar date on a recurring task asks: `o` moves only this occurrence and keeps the repeat, `r` replaces the repeat with the date.
- Priority keys `1` to `4`.
- Label checklist picker (`@`) with a filter. A new name creates the label.
- Move picker (`m`) for projects and sections, with a filter.
- Mouse support: click to select, click `○` to complete (recurring tasks ask first), double-click to rename a task or edit a comment, wheel to scroll, and click a legend item to run its key.
- Right-click menu on a task: rename, due date, priority, labels, move, description, comment, complete.
- Drag a task to a sidebar project or a section header to move it.
- An empty line before sidebar groups, before section headers, and under the title of each task list. Section headers use the project color.
- Unit tests for the mouse handling.
- Task deletion: `Delete` or `Backspace` on a task, or "Delete" in the right-click menu. It asks y/n first.
- Sections: `A` adds a section after the section under the cursor. With the cursor on a section header, `e` renames it, `[` and `]` move it, and `Delete` or `Backspace` deletes it and its tasks after a y/n prompt with the task count. Right-click on a header opens a section menu.
- Two bottom lines: the key legend, and the last operation with the sync state. Legend items that do not fit on the first line go to the free space on the second line.
- Long hints in the date dialog, the pickers, and the details pane wrap to a second line.
- Key hints in the bottom bar for the focused pane, in each input, and in each picker.
- Built-in multi-line editor with `ctrl+s` to save. `ctrl+e` opens the text in `$VISUAL` or `$EDITOR`.
- Notebook view per project (`v`): note titles with previews and a markdown reader. The view choice is saved in `~/.local/state/godoist/state.toml`.
- Local cache of the account in `~/.cache/godoist/sync.json`, kept current with the Todoist Sync API.
- Sync at start, after each write, every 60 seconds, on terminal focus, and on `r`.
- Quit waits for pending writes. The editor stays open until a save succeeds. A failed add opens the input again with the typed text.
- Light and dark palettes, selected from the terminal background color.
- Layout for narrow terminals: the details and the editor replace the task list.
- CLI commands for scripts: `ls`, `add`, `done`, `reopen`, `rm`, `projects`, and `login`.
- CLI output as aligned columns on a terminal, as TSV in a pipe, and as JSON with `--json`.
- API token from `TODOIST_TOKEN` or `~/.config/godoist/config.toml`.
