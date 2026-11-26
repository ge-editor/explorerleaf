# explorerleaf

`explorerleaf` is a pane for browsing files and other lists in the ge editor. A
single ExplorerLeaf can host multiple modes. Each pane has independent mode
state, including its list, selection, and filter.

## Built-in Files mode

`NewLeafType` registers the Files mode by default. It lists the current
workspace directories. If there is no current workspace or it has no
directories, it lists the process working directory instead.

- Directories are shown before files and can be expanded or collapsed.
- Typing filters the list. While a filter is active, files under the roots are
  searched even inside collapsed directories.
- `.git`, `.hg`, `.svn`, and `node_modules` directories are skipped.
- Activating a file sends a `tree.OpenRequest` with
  `tree.DispatchLeafRequest` to the registered `editorleaf` type. The editor
  leaf type opens the path in the first editor leaf it finds.

## Git status marks

Files mode marks files with the git status, as VS Code does: `M` modified,
`A` added, `D` deleted, `R` renamed, `U` untracked, `!` conflict. A directory
shows `*` in the color of the strongest status below it. The marks are read
with `git status` whenever the list is reloaded (`Ctrl+L`, or when the pane
gets the focus again). A directory outside a repository, or a machine without
git, shows no marks. `FileMode.SetGitStatus(false)` turns them off.

## Search mode (full text search and replace)

`C-s` in the explorer opens the search form in the minibuffer (the form of
the editor's search, `ge/explorer_search_form.go`) and shows the Search mode.
`Tab` onto the Search mode opens the form too. `C-g` closes the form; the
result stays and the keys go to the result tree.

```
Options: □Replace, ▣Match Case, □Whole word, □Use Regex, Scope: current
Search: ...
Replace: ...
```

- The search runs while typing (after 160 ms, in the background), and again
  when a condition is toggled. Only the first line of Search is searched.
- Keys of the form: `Tab` / `Shift+Tab` move the focus (Search, Replace while
  it is on, the row of options), `Enter` / `C-j` type a newline, `C-s` / `C-r`
  next / previous matching line of the tree, `Alt+p` / `Alt+n` history,
  `Alt+Enter` (Replace on) replace the selected row, `Alt+a` replace all,
  `C-g` close the form. On the row of options (Replace, Match Case, Whole
  word, Use Regex, Scope) `Left` / `Right` (`C-b` / `C-f`) choose an option
  and `Space` switches it.
- The result is a tree. `Enter` on a directory or file row folds or unfolds
  it, as in Files mode; `+` and `-` do it too (on a line, or a row that is
  folded already, `-` goes to the parent):

```
- dir (2)
  - file name (2)
      12: the line
```

  `Enter` on a line opens the file at the match. A click does the same as
  `Enter`.
- The directories searched follow a PathScope (feature `search`, default
  `current`): `current` is the current workspace, `workspaces` and `all` are
  every open workspace. The Scope option of the form toggles current /
  workspaces, and
  `explorer-search-scope KIND` sets it; the mode line shows
  `[Search:current 12/3]` (lines/files). The workspace setting is the
  `search` entry of the PathScope settings, next to `mark` and `recent`.
- Only valid UTF-8 text files are searched: binary files, files over 2 MB and
  the directories Files mode hides are skipped. The result is cut at 5000
  lines or 5 seconds.
- Replace: `Alt+Enter` replaces the matches of the selected row and the rows
  below it (a directory, a file or a line), `Alt+a` all of them. The same is
  `explorer-replace TEXT` (all), `explorer-replace-file TEXT` and
  `explorer-replace-line TEXT`. With Use Regex the text may use `$1`,
  `${name}`; `""` replaces with nothing. The editor leaf checks every place
  against the text it has now, shows one line per changed row, and asks
  before it edits, so the form is closed first. The files are edited as
  buffers (not saved); `workspace-edit-undo` takes it back.

## Mouse

A left click on a row selects it and does what `Enter` does: a file is opened
in the editor, a directory is expanded or collapsed. This needs the mouse
enabled in the screen (`screen.EnableMouse()`) and `tree.DispatchMouse` called
for mouse events.

## Registering the explorer pane

The application registers a `LeafType` with gecore and supplies its keymap:

```go
tree.LeafTypes.Register(func() tree.LeafType {
    return explorerleaf.NewLeafType(bindExplorerKeys)
}, 10, "explorerleaf")
```

`bindExplorerKeys` receives the keymap root and the ExplorerLeaf. Bind the
navigation, activation, filtering, and mode-switching methods there. The
package does not impose a default keymap.

## Adding a mode

Implement `Mode` and register a factory on the `LeafType`. A factory creates a
fresh mode for each pane:

```go
type Mode interface {
    Name() string
    List() *explorerleaf.List
    Reload() error
    SetFilter(string)
    Activate(explorerleaf.Item) explorerleaf.Activation
}

leafType := explorerleaf.NewLeafType(bindExplorerKeys).
    AddMode(func() explorerleaf.Mode { return newSymbolMode() })
```

The Files mode is the initial mode. Modes added with `AddMode` follow it and
can be reached with `NextMode` or `PrevMode`. Each mode owns its items and
activation behavior; ExplorerLeaf provides shared drawing, navigation, and
filter input.

## Planned modes

- Symbol and function lists
- Markdown headings
- Workspace marks and RootTrees
- Git status list (changed files only)

## License

This package is licensed under the MIT License. See [LICENSE](LICENSE).
