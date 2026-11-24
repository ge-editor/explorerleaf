package explorerleaf

import "testing"

func TestParseGitStatus(t *testing.T) {
	// M  staged modify, " M" worktree modify, ?? untracked, A  added,
	// R  renamed (the old name follows), UU conflict, " D" deleted.
	out := []byte(" M a.go\x00" +
		"M  pkg/b.go\x00" +
		"?? pkg/sub/new.txt\x00" +
		"A  added.go\x00" +
		"R  moved.go\x00old.go\x00" +
		"UU conflict.go\x00" +
		" D gone.go\x00" +
		"!! ignored.log\x00")

	st := parseGitStatus(out, "/repo", "/repo", "/repo")
	want := map[string]byte{
		"a.go":            'M',
		"pkg/b.go":        'M',
		"pkg/sub/new.txt": 'U',
		"added.go":        'A',
		"moved.go":        'R',
		"conflict.go":     '!',
		"gone.go":         'D',
	}
	if len(st.files) != len(want) {
		t.Fatalf("files = %v", st.files)
	}
	for p, c := range want {
		if st.files[p] != c {
			t.Errorf("%s = %q, want %q", p, st.files[p], c)
		}
	}

	// A directory takes the strongest status below it.
	if got := st.dirs["pkg"]; got != 'M' {
		t.Errorf("pkg = %q, want M", got)
	}
	if got := st.dirs["pkg/sub"]; got != 'U' {
		t.Errorf("pkg/sub = %q, want U", got)
	}
	if got := st.dirs[""]; got != '!' {
		t.Errorf("root = %q, want !", got)
	}
}

func TestParseGitStatusSubdirectoryRoot(t *testing.T) {
	// The listed directory is a subdirectory of the repository.
	out := []byte(" M cmd/main.go\x00 M README.md\x00")
	st := parseGitStatus(out, "/repo", "/repo/cmd", "/repo/cmd")
	if len(st.files) != 1 || st.files["main.go"] != 'M' {
		t.Fatalf("files = %v", st.files)
	}

	g := gitStatuses{st}
	if c := g.of("/repo/cmd/main.go", false); c != 'M' {
		t.Errorf("of = %q", c)
	}
	if c := g.of("/repo/other/x.go", false); c != 0 {
		t.Errorf("outside = %q", c)
	}
}

func TestGitMarkItem(t *testing.T) {
	st := parseGitStatus([]byte(" M a.go\x00"), "/r", "/r", "/r")
	g := gitStatuses{st}

	var f, d Item
	g.mark(&f, "/r/a.go", false)
	g.mark(&d, "/r", true)
	if f.Badge != "M" || d.Badge != "*" {
		t.Fatalf("badges = %q, %q", f.Badge, d.Badge)
	}

	var none Item
	g.mark(&none, "/r/clean.go", false)
	if none.Badge != "" {
		t.Fatalf("clean file has badge %q", none.Badge)
	}
}
