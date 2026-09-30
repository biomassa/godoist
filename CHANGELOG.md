# Changelog

This file records all notable changes to this project.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-09-30

### Added

- Delete on a recurring task asks what to do. `s` skips only the current date: the task moves to its next date and keeps its repeat. Todoist records no completion. `d` deletes the task with all its dates.
- Bulk delete asks one time when the selection has recurring tasks. `s` skips the dates of the recurring tasks and deletes the other tasks. `d` deletes all the tasks.
- The hint of the details pane shows `y copy`.

### Changed

- The changelog follows the STE writing rules: short sentences with one topic each, in the active voice.

## [0.3.0] - 2026-09-30

### Added

- `y` copies text to the system clipboard. In the task list, it copies the name of the task or the note. In the details pane, it copies the selected comment. If you select no comment, it copies the description.
- Notebook view shows the labels on each note row and in the reader. The label picker (`@`) works in notebook view as in task view.
- `pgup` and `pgdown` in the sidebar go to the top and to the bottom.

### Fixed

- In the label picker, `enter` on "+ create" makes the label and puts it on the task. Before, `enter` saved nothing: only `space` checked the new label.
- A label change shows at once in the task list and in the details pane. Before, it showed only after the next sync.

## [0.2.1] - 2026-09-28

### Changed

- The install and build commands in the README use `-trimpath -ldflags="-s -w"`. These flags remove the debug data, as in the release binaries. Thus the program is about 7 MB smaller, and it works the same.

## [0.2.0] - 2026-09-28

### Added

- Color themes. `T` opens the theme picker. When you move through the list, the whole screen shows the highlighted theme. `enter` keeps the theme and saves it as `theme` in `~/.config/godoist/config.toml`. `esc` goes back to the theme that you had.
- 20 themes. The default theme, `todoist`, uses the Todoist colors on the terminal background. The other 19 themes come from tideui, for example Catppuccin, Nord, Dracula, Gruvbox, Tokyo Night, Rose Pine, and One Dark. These themes set the terminal background while godoist runs.
- `--theme NAME` selects a theme for one run. The legends show `T theme`.
- The markdown colors follow the theme. The heading bars use the accent of the theme: level 1 is the strongest, and each lower level is fainter. Links, inline code, quotes, and rules also use the theme colors.
- Code blocks use the Chroma style of the theme, for example `nord` or `catppuccin-mocha`. If Chroma has no style for the theme, the code gets colors from the theme.
- `{` and `}` move the border between the task list and the details pane by two columns. A mouse drag on the border also moves it. The legends show `{ } resize`.
- The width of the task list persists as `list_share` in `~/.config/godoist/config.toml`. The value is a share of the space, so it fits all terminal widths.

### Fixed

- `godoist login` keeps the other settings of the config file.

## [0.1.0] - 2026-09-27

The first release.

### Added

Screen and navigation:

- Terminal UI with three panes: the sidebar, the task list, and the details. On a narrow terminal, the details and the editor replace the task list.
- Sidebar with Inbox, Today, Upcoming, Favorites, the project tree, Labels, All tasks, and Completed. Each item shows its task count.
- Tree lines for sub-projects in the sidebar. Top-level projects line up with Inbox. The `▾` / `▸` marker of a project with sub-projects comes after its name.
- Todoist project colors on the sidebar, the selected row, the section headers, and the focused border.
- Priority colors on the task circles. Due date colors for overdue, today, tomorrow, and this week.
- Light and dark palettes. The terminal background selects the palette.
- Two bottom lines. The first line shows the keys of the focused pane. The second line shows the last operation and the sync state. Keys that do not fit on the first line go to the start of the second line.
- Key hints in each input, dialog, and picker. A help screen (`?`) shows all keys and the version.

Views:

- Project view: the tasks by section in the project order, also the empty sections.
- Today with an Overdue group and a Today group. Upcoming by day.
- All tasks: all tasks by project and by section. In each group, the dated tasks come first, by date.
- Label views, by project and by section as in All tasks.
- Completed: the tasks that you completed in the last 30 days, by project. `x` opens a task again.
- Sub-tasks in Today, Upcoming, All tasks, labels, and filters. The sub-tasks show under their parent, collapsed at first. `z` or a click expands them.
- A due sub-task brings its parent into the view, gray and expanded. Each view keeps its expanded tasks in `~/.local/state/godoist/state.toml`.
- Find in the current view (`/`), and Todoist filter queries (`f`). Plain words search the task names. `esc` clears the find or the filter.

Tasks:

- Task dialog with a name and a description, for add (`a`), edit (`e`), and sub-task (`A`).
- Todoist parses the name as in quick add: a date, `#project`, `/section`, `@label`, or `p1`–`p4`. In edit, the name changes only the fields that it names. godoist saves the description as typed.
- Keys of the task dialog: `enter` in the name saves, and `tab` goes to the description. `ctrl+enter` or `ctrl+s` saves from the two fields.
- Quick add in a project puts the task in the section under the cursor. Quick add in Today or Upcoming gives the task the date of that day, if the text has no date.
- Complete (`x`), and undo (`ctrl+z`) for one-time tasks. Delete (`Delete` or `Backspace`) after a confirmation.
- Date dialog (`t`) with a text field that Todoist parses, a month calendar with quick picks, and a time field.
- A calendar date on a recurring task asks what to do. `o` moves only this occurrence. `r` replaces the repeat with the date.
- Priority keys `1` to `4`. A label picker (`@`) that can also make a new label. A move picker (`m`) for projects and sections.
- Sub-tasks: `A` adds a sub-task, `>` indents a task, and `<` outdents it. After `>`, the new parent expands, so the task stays in view.
- Manual order: `[` and `]` move a task among its siblings. At the edge of a section, the task goes into the next section. In All tasks and the label views, the keys skip the dated tasks.
- Collapse with `z` for sections, tasks with sub-tasks, and projects. Todoist keeps the state.
- Multi-select with `s`, `S`, `ctrl`+click, and `shift`+click. The bulk actions are complete, date, priority, move, labels, and delete.
- In the bulk date dialog, `(mixed)` shows when the tasks have different dates. One `backspace` clears all the dates.
- Links in task names, as `[text](url)` or as a bare URL, are terminal hyperlinks. `o` opens the first link of the task in the browser.

Notes, descriptions, and comments:

- Notebook view per project (`v`), with the note titles, previews, and a markdown reader.
- The notebook view setting is the line `godoist:notes` at the end of the project description. Thus the setting is the same on all computers.
- The details pane shows descriptions and comments as formatted markdown. `tab` and `space`, or a click, change the checkboxes in descriptions and comments.
- One markdown editor for notes, descriptions (`E`), and comments (`c`, `e`).
- A live preview in the editor. Each line shows formatted markdown, but the line with the cursor shows its source. A code block or a table shows as source while the cursor is in it.
- Editor keys: `ctrl+b` bold, `ctrl+i` italic, `ctrl+k` link, `ctrl+t` checkbox, list continuation, word moves, and `ctrl+z` / `ctrl+y` undo and redo.
- More editor keys: `ctrl+f` find and replace, and `ctrl+e` to open the text in `$VISUAL` or `$EDITOR`.
- The editor saves a note, a description, or a changed comment one second after the last change. It adds a new comment when it closes. `esc`, `ctrl+enter`, or `ctrl+s` closes the editor.
- Text selection with `shift` and the arrows, `home`, or `end`. Also word selection, `ctrl+a`, and a mouse drag.
- `ctrl+c`, `ctrl+x`, and `ctrl+v` copy, cut, and paste through the system clipboard (OSC 52). The task dialog fields have the same keys.
- Paste in all text fields. One-line fields get the text without line breaks.

Projects, sections, and labels:

- Projects: add (`A`), rename, color, favorite, reorder, indent, outdent, delete, and archive. godoist protects the Inbox.
- A new project gets a color picker with a sub-project option. `m` or a drag moves a project under a new parent.
- Sections: add after the section under the cursor, rename, reorder, and delete. A deletion also deletes the tasks of the section, after a confirmation.
- Labels: add, rename, color, favorite, reorder, and delete.
- Confirmations open a dialog with buttons. The dialog selects the action button first.

Mouse:

- A click selects, and a click on `○` completes a task. A double-click edits a task or a comment. The wheel scrolls.
- A click on a legend item runs its key. A right-click opens a menu for a task, a section, a project, or a label.
- A drag of a task to a project or a section header moves the task.

Sync and offline:

- A local copy of the account in `~/.cache/godoist/sync.json`. The Todoist Sync API keeps it current.
- godoist syncs at the start, after each change, every 60 seconds, when the terminal gets the focus, and on `r`.
- Offline queue: when the computer is offline, task changes wait in `~/.cache/godoist/queue-*.json`. The list shows them at once. godoist sends them in order at the next sync.
- A name that Todoist must parse shows `⏳` until the sync. The bottom line shows `offline · N changes waiting`. While changes wait, quit asks first.
- Quit waits for the changes that run. A failed add opens the dialog again with the typed text.

CLI:

- Commands for scripts: `ls`, `add`, `done`, `reopen`, `rm`, `projects`, `login`, and `version` (also `--version`).
- Output as aligned columns on a terminal, as TSV in a pipe, and as JSON with `--json`.
- `add` prints a summary line on a terminal, and the task ID in a pipe.
- The API token comes from `TODOIST_TOKEN` or from `~/.config/godoist/config.toml`.

License:

- MIT license.

[Unreleased]: https://github.com/biomassa/godoist/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/biomassa/godoist/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/biomassa/godoist/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/biomassa/godoist/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/biomassa/godoist/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/biomassa/godoist/releases/tag/v0.1.0
