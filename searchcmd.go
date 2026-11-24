package explorerleaf

import (
	"context"
	"errors"
	"strings"

	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/excommand"
	"github.com/ge-editor/gecore/pathscope"
	"github.com/ge-editor/gecore/tree"
)

// Ex commands of the search mode (ESC x). The search itself is typed in the
// search form of the host (C-s, see ge/explorer_search_form.go).
//
//	explorer-search-scope KIND      current | workspaces | all: which directories (this pane)
//	explorer-replace TEXT           replace every match in the result
//	explorer-replace-file TEXT      replace the matches in the file of the selected row
//	explorer-replace-line TEXT      replace the matches of the selected line
//
// "explorer-replace """ replaces with nothing. The editor shows what will
// change and asks before it changes the buffers; nothing is saved.

// argText returns the text after the command name. ESC x passes the whole
// input ("explorer-replace foo") as one argument, other callers may pass the
// name alone; both are accepted.
func argText(args []string, name string) string {
	text := strings.TrimSpace(strings.Join(args, " "))
	first, rest, _ := strings.Cut(text, " ")
	if strings.EqualFold(first, name) {
		return strings.TrimSpace(rest)
	}
	return text
}

// ReplaceInFiles replaces the matches of the result of the Search mode in
// scope ("all", "node", "file" or "line", see Replacer) with text. The editor
// leaf shows what changes and asks before it edits.
func (e *ExplorerLeaf) ReplaceInFiles(with, scope string) error {
	r, ok := e.Mode().(Replacer)
	if !ok {
		return errors.New("explorer: this mode cannot replace")
	}
	req, err := r.BuildReplace(with, scope)
	if err != nil {
		return err
	}
	// The editor echoes its own errors; the dispatcher does not hand them
	// back.
	tree.DispatchLeafRequest("editorleaf", "replace", tree.ActiveOrFirstLeaf, req,
		func(handled bool, err error) {
			if err != nil {
				gecore.Echo.AddText(err.Error())
			}
		})
	return nil
}

func (e *ExplorerLeaf) searchCommands() []excommand.ExCommand {
	replace := func(name, scope, desc string) *command {
		return &command{
			name: name,
			desc: desc,
			run: func(ctx context.Context, args []string) (bool, error) {
				with := argText(args, name)
				switch with {
				case "":
					return true, errors.New("usage: " + name + ` TEXT   (use "" for nothing)`)
				case `""`:
					with = ""
				}
				return true, e.ReplaceInFiles(with, scope)
			},
		}
	}

	return []excommand.ExCommand{
		&command{
			name: "explorer-search-scope",
			desc: "Set which directories are searched: current, workspaces or all (this pane only).",
			run: func(ctx context.Context, args []string) (bool, error) {
				sm := e.SearchMode()
				if sm == nil {
					return true, errors.New("explorer: no Search mode")
				}
				k, err := pathscope.ParseKind(argText(args, "explorer-search-scope"))
				if err != nil {
					return true, err
				}
				sm.SetScope(k)
				gecore.Echo.AddText("search scope: " + k.String() + " (search again)")
				return true, nil
			},
			complete: func(input string) []string {
				var out []string
				for _, k := range pathscope.Kinds() {
					if strings.HasPrefix(string(k), strings.ToLower(input)) {
						out = append(out, string(k))
					}
				}
				return out
			},
		},
		replace("explorer-replace", "all", "Replace every match of the search result with TEXT."),
		replace("explorer-replace-file", "file", "Replace the matches in the file of the selected row with TEXT."),
		replace("explorer-replace-line", "line", "Replace the matches of the selected line with TEXT."),
	}
}
