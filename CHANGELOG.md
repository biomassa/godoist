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
- Sub-projects in the sidebar show tree lines (`├`, `└`, `│`). Top-level projects keep a column for the `▾` / `▸` marker, so that each level is clearly indented.
- Sub-project moves in the sidebar: `>` and `<` indent and outdent a project, `m` opens a parent picker, and a drag onto a project or onto "My Projects" changes the parent. The project menu has Indent, Outdent, and Move under. Sub-projects move with their parent.
- Find and replace in the inline editor (`ctrl+f`): a box at the top of the editor with find, replace, and match case. `enter` finds the next match, `ctrl+r` replaces the current match, and `ctrl+a` replaces all. An empty replace field removes the matches. All matches are highlighted, and the box shows "3 of 7". Replace all is one undo step.
- Sub-tasks in Today, Upcoming, All tasks, labels, and filters: nested under their parents, collapsed by default, expanded with `z` or a click. A due sub-task brings in its parent (gray, expanded). Each view keeps its expanded tasks locally.
- The inline editor opens with `enter` in the reader, with `E`, or with a double-click on the note text. The reader shows the hint "enter or E · edit this note".
- Headings in the reader and the editor are full-width bars, colored by level.
- Markdown reader in notebook view: headings without `#` marks, `☐` / `☑` checkboxes, terminal hyperlinks, and syntax colors for fenced code. `tab` moves between the checkboxes of a note, and `space` or a click changes one.
- Inline markdown editor in notebook view (`E`): the source with live styles, autosave one second after the last change and on `esc`, the save state in the pane title, list continuation, `ctrl+b` / `ctrl+i` / `ctrl+k` / `ctrl+t`, word moves, and undo and redo.
- Sub-tasks: `A` on a task adds a sub-task (Todoist parses the text). `>` indents a task under the task above it, and `<` outdents it.
- Manual task order: `[` and `]` move a task among its siblings. At the edge of a section, a top-level task goes into the next section. In All tasks, this works for tasks without a due date.
- Collapse with `z` for sections, tasks with sub-tasks, and sub-projects. The state is saved in Todoist. `▾` and `▸` markers show it, and a click on a marker toggles it.
- Multi-select: `s`, `S`, `ctrl`+click, and `shift`+click. Bulk complete, due date, priority, move, labels (with `[~]` for partial labels), and delete. One `ctrl+z` opens again the one-time tasks of a bulk completion.
- Bulk date dialog: if the selected tasks have different dates, the text field shows `(mixed)`. One `backspace` or `delete` removes it and shows the hint "Clear dates for selected tasks". Then `enter` removes all the dates. Typed text replaces `(mixed)`. `enter` with `(mixed)` makes no change.
- Labels group in the sidebar with a label view grouped by project. Label management: add, rename, color, favorite, reorder, and delete.
- Completed view: the tasks completed in the last 30 days, grouped by project. `x` opens a task again.
- Projects in the sidebar: `A` adds a project (name, then a color picker with a sub-project option), `e` renames, `C` changes the color, `*` adds to or removes from Favorites, `[` and `]` move it among its siblings, and `Delete` or `Backspace` deletes or archives it. Right-click opens a project menu. The Inbox is protected.
- All tasks view at the end of the sidebar: all tasks grouped by project, dated tasks first by due date.
- Confirmations open a dialog with buttons. The action button is selected first. The arrows, `tab`, and `h`/`l` select a button, `enter` pushes it, the button keys and a click also work, and `esc` closes the dialog.
- The sidebar starts with the Inbox. The sidebar and the details pane start with an empty line.
- Task deletion: `Delete` or `Backspace` on a task, or "Delete" in the right-click menu. It asks y/n first.
- Sections: `A` adds a section after the section under the cursor. With the cursor on a section header, `e` renames it, `[` and `]` move it, and `Delete` or `Backspace` deletes it and its tasks after a y/n prompt with the task count. Right-click on a header opens a section menu.
- Two bottom lines: the key legend, and the last operation with the sync state. Legend items that do not fit on the first line start the second line, and the status follows them.
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
