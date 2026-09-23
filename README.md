# godoist

godoist is a terminal program for [Todoist](https://todoist.com). It has a text user interface (TUI) with three panes and a command-line interface (CLI) for scripts. It is written in Go.

> This software was developed with the assistance of a LLM.

## Features

- Three panes: sidebar, task list, and details.
- Project colors from the Todoist palette. Light and dark terminal themes.
- Today, Upcoming, Inbox, Favorites, and the project tree, with task counts.
- Quick add with Todoist natural-language parsing: dates, `#project`, `/section`, `@label`, `p1`–`p4`.
- A date dialog with a text field, a month calendar, and a time field.
- Priorities, labels, moves, comments, descriptions, and sections.
- Notebook view per project, with a markdown reader.
- Todoist filter queries and a find in the current view.
- Mouse support: click, double-click, right-click menus, drag to move, and the wheel.
- A local cache. The program starts from the cache and syncs in the background.
- CLI commands with aligned columns, TSV, or JSON output.

## Requirements

- Go 1.27 or later, to build the program.
- A Todoist account and its API token.
- A terminal with true color and mouse support. Most modern terminals have both.

## Installation

Install the latest version with Go:

```sh
go install github.com/biomassa/godoist/cmd/godoist@latest
```

Go puts the binary in `$(go env GOPATH)/bin`. Make sure that this directory is in your `PATH`.

To build from the source:

```sh
git clone https://github.com/biomassa/godoist.git
cd godoist
go build -o godoist ./cmd/godoist
```

## Configuration

godoist needs your Todoist API token. Get it in Todoist: **Settings → Integrations → Developer**.

Give the token to godoist in one of these two ways:

1. Set the environment variable:

   ```sh
   export TODOIST_TOKEN=your-token
   ```

2. Save the token in the config file:

   ```sh
   godoist login
   ```

   This command asks for the token, makes sure that it works, and writes it to `~/.config/godoist/config.toml` with owner-only permissions.

The environment variable has priority over the config file.

godoist writes these files:

| File | Content |
|---|---|
| `~/.config/godoist/config.toml` | The API token (after `godoist login`). |
| `~/.cache/godoist/sync.json` | The local copy of your account (owner-only permissions). |
| `~/.local/state/godoist/state.toml` | The view mode of each project (tasks or notebook). |

## TUI

Start the TUI:

```sh
godoist
```

Push `?` to see all keys. The first bottom line shows the keys for the pane that has the focus. You can click a key there to run it. The second bottom line shows the last operation and the sync state.

### Main keys

| Key | Action |
|---|---|
| `tab`, `h` / `l` | Go to the next or previous pane. |
| `j` / `k`, `g` / `G` | Move down or up, go to the top or the bottom. |
| `enter` | Open the project, or open the details. |
| `a` | Add a task. Todoist parses the text. |
| `x` or `space` | Complete the task. `ctrl+z` undoes the last completion of a one-time task. |
| `e` | Rename the task. Todoist parses dates, `#project`, `@label`, and `p1`–`p4` in the text. |
| `E` | Edit the description. `ctrl+s` saves, `ctrl+e` opens `$EDITOR`. |
| `t` | Open the date dialog. |
| `1`–`4` | Set the priority. |
| `@` | Select labels. |
| `m` | Move the task to a project or a section. |
| `c` | Add a comment. |
| `Delete` / `Backspace` | Delete the task. godoist asks first, because a deletion cannot be undone. |
| `v` | Change the project between task view and notebook view. |
| `/` | Find in the current view. |
| `f` | Run a Todoist filter query. Plain words search the task names. |
| `esc` | Clear the find or the filter. |
| `r` | Sync now. |
| `q` | Quit. godoist waits until all changes are saved. |

### Date dialog

The `t` key opens a dialog with three parts: a text field, a month calendar, and a time field.

1. Type a date in the text field, for example `fri 9am` or `every mon`. Type `no date` to remove the date.
2. Push `tab` to go to the calendar. Todoist parses the changed text one time, and the calendar shows the result.
3. In the calendar, use the arrow keys, `PgUp` / `PgDn`, `Home`, or the quick picks `1`–`5`.
4. Push `enter`. godoist saves the input that you changed last.

For a recurring task, a date from the calendar asks a question. Push `o` to move only this occurrence and keep the repeat. Push `r` to replace the repeat with the date.

### Sections

These keys work in a project:

| Key | Action |
|---|---|
| `A` | Add a section after the section under the cursor. |
| `e` | Rename the section under the cursor. |
| `[` / `]` | Move the section up or down. |
| `Delete` / `Backspace` | Delete the section and its tasks. godoist asks first. |

### Mouse

- Click to select. Click the `○` circle to complete a task.
- Double-click a task to rename it.
- Right-click a task or a section header to open a menu. The task menu can also delete the task.
- Drag a task to a sidebar project or a section header to move it.
- Use the wheel to scroll.

To select text in the terminal, hold `Shift` while you drag.

## CLI

Use the CLI in scripts. On a terminal, the output has aligned columns. In a pipe, the output is TSV. Add `--json` to get JSON.

| Command | Action |
|---|---|
| `godoist ls` | List the active tasks. |
| `godoist ls -p <project>` | List the tasks of one project. |
| `godoist ls -f "<filter>"` | List the tasks that match a Todoist filter. |
| `godoist add "<text>"` | Add a task with natural-language parsing. It prints the task ID. |
| `godoist add -p <project> "<text>"` | Add a task to a project. |
| `godoist done <id>...` | Complete tasks. |
| `godoist reopen <id>...` | Open completed tasks again. |
| `godoist rm <id>...` | Delete tasks. |
| `godoist projects` | List the projects. |
| `godoist login` | Save the API token. |

Examples:

```sh
godoist ls -f "today | overdue"
godoist add "Buy cables tomorrow 10am #work p2 @errand"
godoist ls -f "today" --json | jq -r '.[].content'
godoist ls -p work | cut -f1 | xargs godoist done
```

## Sync

godoist keeps a local copy of your account and uses the Todoist Sync API. It syncs:

- when it starts,
- after each change,
- every 60 seconds,
- when the terminal gets the focus,
- when you push `r`.

Each change goes to Todoist at once. godoist does not keep changes that only exist on your computer.

## Changes

See [CHANGELOG.md](CHANGELOG.md).
