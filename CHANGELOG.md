# Changelog

This file records all notable changes to this project.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-09-27

The first release.

### Added

Screen and navigation:

- Terminal UI with three panes: sidebar, task list, and details. On a narrow terminal, the details and the editor replace the task list.
- Sidebar: Inbox, Today, Upcoming, Favorites, the project tree with tree lines, Labels, All tasks, and Completed, with task counts. Top-level projects line up with Inbox, and the `▾` / `▸` marker of a project with sub-projects is after its name.
- Todoist project colors on the sidebar, the selected row, the section headers, and the focused border. Priority colors on task circles. Due date colors for overdue, today, tomorrow, and this week.
- Light and dark palettes, selected from the terminal background color.
- Two bottom lines: the key legend for the focused pane, and the last operation with the sync state. Legend items that do not fit on the first line start the second line.
- Key hints in each input, dialog, and picker, and a help screen (`?`) that shows the version.

Views:

- Project view: tasks grouped by section in project order, with empty sections.
- Today with Overdue and Today groups. Upcoming grouped by day.
- All tasks: all tasks grouped by project and section. In each group, dated tasks come first, by date.
- Label views, grouped by project and section as in All tasks.
- Completed: the tasks completed in the last 30 days, grouped by project. `x` opens a task again.
- Sub-tasks in Today, Upcoming, All tasks, labels, and filters: nested under their parents, collapsed at first, expanded with `z` or a click. A due sub-task brings in its parent (gray, expanded). Each view keeps its expanded tasks in `~/.local/state/godoist/state.toml`.
- Find in the current view (`/`), and Todoist filter queries (`f`). Plain words search the task names. `esc` clears the find or the filter.

Tasks:

- Task dialog for add (`a`), edit (`e`), and sub-task (`A`), with a name and a description. Todoist parses the name as in quick add: a date, `#project`, `/section`, `@label`, or `p1`–`p4`. In edit, only the fields that the name gives change. godoist saves the description as typed. `enter` in the name saves, `tab` goes to the description, and `ctrl+enter` or `ctrl+s` saves from the two fields.
- Quick add in a project puts the task in the section under the cursor. Quick add in Today or Upcoming gives the date of that day if the text has no date.
- Complete (`x`) and undo (`ctrl+z`) for one-time tasks. Delete (`Delete` or `Backspace`) after a confirmation.
- Date dialog (`t`): a text field that Todoist parses, a month calendar with quick picks, and a time field. On a recurring task, a calendar date asks: `o` moves only this occurrence, `r` replaces the repeat.
- Priority keys `1` to `4`, a label picker (`@`) that can make a new label, and a move picker (`m`) for projects and sections.
- Sub-tasks: `A` adds one, `>` indents, `<` outdents. The new parent expands, so the task stays in view.
- Manual order: `[` and `]` move a task among its siblings, and into the next section at the edge. In All tasks and the label views, they skip the dated tasks.
- Collapse with `z` for sections, tasks with sub-tasks, and projects. Todoist keeps the state.
- Multi-select: `s`, `S`, `ctrl`+click, and `shift`+click, with bulk complete, date, priority, move, labels, and delete. In the bulk date dialog, `(mixed)` shows for different dates, and one `backspace` clears all the dates.
- Links in task names, as `[text](url)` or as a bare URL, are terminal hyperlinks. `o` opens the first link of the task in the browser.

Notes, descriptions, and comments:

- Notebook view per project (`v`): note titles with previews and a markdown reader. The setting is the line `godoist:notes` at the end of the project description, so it is the same on all computers.
- The details pane shows descriptions and comments as formatted markdown. `tab` and `space` or a click change checkboxes in descriptions and comments.
- One markdown editor for notes, descriptions (`E`), and comments (`c`, `e`), with a live preview: each line shows formatted markdown, except the line with the cursor. A code block or a table shows as source while the cursor is in it.
- Editor keys: `ctrl+b` bold, `ctrl+i` italic, `ctrl+k` link, `ctrl+t` checkbox, list continuation, word moves, `ctrl+z` / `ctrl+y` undo and redo, `ctrl+f` find and replace, and `ctrl+e` to open the text in `$VISUAL` or `$EDITOR`.
- The editor saves a note, a description, or a changed comment one second after the last change. It adds a new comment when it closes. `esc`, `ctrl+enter`, or `ctrl+s` closes it.
- Text selection with `shift` and the arrows, `home`, or `end`, word selection, `ctrl+a`, and a mouse drag. `ctrl+c`, `ctrl+x`, and `ctrl+v` copy, cut, and paste through the system clipboard (OSC 52). The task dialog fields have the same keys.
- Paste works in all text fields. One-line fields get the text without line breaks.

Projects, sections, and labels:

- Projects: add (`A`, with a color picker and a sub-project option), rename, color, favorite, reorder, indent and outdent, move under a parent (`m` or a drag), and delete or archive. godoist protects the Inbox.
- Sections: add after the section under the cursor, rename, reorder, and delete with its tasks after a confirmation.
- Labels: add, rename, color, favorite, reorder, and delete.
- Confirmations open a dialog with buttons. The action button is selected first.

Mouse:

- Click to select, click `○` to complete, double-click to edit a task or a comment, wheel to scroll, and click a legend item to run its key. Right-click menus for tasks, sections, projects, and labels. Drag a task to a project or a section header to move it.

Sync and offline:

- A local copy of the account in `~/.cache/godoist/sync.json`, kept current with the Todoist Sync API: at start, after each change, every 60 seconds, on terminal focus, and on `r`.
- Offline queue: offline, task changes wait in `~/.cache/godoist/queue-*.json` and show in the list at once. godoist sends them in order at the next sync. A name that Todoist must parse shows with `⏳` until then. The bottom line shows `offline · N changes waiting`, and quit asks first while changes wait.
- Quit waits for the running changes. A failed add opens the dialog again with the typed text.

CLI:

- Commands for scripts: `ls`, `add`, `done`, `reopen`, `rm`, `projects`, `login`, and `version` (also `--version`).
- Output as aligned columns on a terminal, as TSV in a pipe, and as JSON with `--json`. `add` prints a summary line on a terminal and the task ID in a pipe.
- API token from `TODOIST_TOKEN` or `~/.config/godoist/config.toml`.

License:

- MIT license.

[Unreleased]: https://github.com/biomassa/godoist/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/biomassa/godoist/releases/tag/v0.1.0
