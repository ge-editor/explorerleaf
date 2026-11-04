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
- Activating a file asks gecore to open its path (`tree.OpenFile`). Any leaf
  that implements `tree.FileOpener` (editorleaf does) can handle it; the
  application does not have to wire anything.

## Registering the explorer pane

The application registers a `LeafType` with gecore and supplies its keymap:

```go
tree.LeafTypes.Register(func() tree.LeafType {
    return explorerleaf.NewLeafType(bindExplorerKeys)
}, 10, "explorerleaf")
```

`bindExplorerKeys` receives the keymap root and the ExplorerLeaf. Bind the
navigation, activation, filtering, and mode-switching methods there. The
package does not impose a default keymap. If no leaf can open the file,
activating it displays an error in ge's echo area (the application can set
`tree.OnNoFileOpener` to create a pane instead).

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
- Search results
- Workspace marks and RootTrees
- Git status

## License

This package is licensed under the MIT License. See [LICENSE](LICENSE).
