package fs

import "testing"

func TestGlobToRegexp(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.go", "main.go", true}, // "**/" combacia con la stringa vuota
		{"**/*.go", "cmd/mani/main.go", true},
		{"**/*.go", "cmd/mani/main.md", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false}, // "*" non attraversa "/"
		{"cmd/**", "cmd/mani/main.go", true},
		{"cmd/**", "app/x.go", false},
		{"*", "README.md", true},
		{"*", "docs/x.md", false},
		{"?.go", "a.go", true},
		{"?.go", "ab.go", false},
		{"app/*.go", "app/runtime.go", true},
		{"app/*.go", "app/sub/x.go", false},
		{"**/test_*.py", "a/b/test_x.py", true},
	}
	for _, c := range cases {
		re, err := globToRegexp(c.pattern)
		if err != nil {
			t.Fatalf("globToRegexp(%q): %v", c.pattern, err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("glob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}
