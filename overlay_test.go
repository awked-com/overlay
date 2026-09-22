package overlay_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/awked-com/overlay"
	"github.com/awked-com/overlay/internal/process"
)

func TestMetadataRejectsSymlinksAndMismatchedStacks(t *testing.T) {
	root := t.TempDir()
	worktree, patches := filepath.Join(root, "source"), filepath.Join(root, "patches")
	if e := os.MkdirAll(filepath.Join(worktree, ".pc"), 0755); e != nil {
		t.Fatal(e)
	}

	if e := os.Mkdir(patches, 0755); e != nil {
		t.Fatal(e)
	}

	for _, p := range []string{"0001-first.patch", "0002-second.patch"} {
		if e := os.WriteFile(filepath.Join(patches, p), nil, 0644); e != nil {
			t.Fatal(e)
		}
	}

	applied := filepath.Join(worktree, ".pc/applied-patches")
	if e := os.WriteFile(applied, []byte("0002-second.patch\n"), 0600); e != nil {
		t.Fatal(e)
	}

	if _, _, e := overlay.Stack(worktree, patches); e == nil {
		t.Fatal("out-of-order stack accepted")
	}

	if e := os.WriteFile(applied, []byte("0001-first.patch\n"), 0600); e != nil {
		t.Fatal(e)
	}

	if _, _, e := overlay.Stack(worktree, patches); e != nil {
		t.Fatal(e)
	}

	if e := os.Symlink(applied, filepath.Join(worktree, ".quilt-series")); e != nil {
		t.Fatal(e)
	}

	if e := overlay.ValidateMetadata(worktree); e == nil {
		t.Fatal("symlink metadata accepted")
	}
}

func TestAtomicMetadataWriteDoesNotFollowHardlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	target := filepath.Join(root, "metadata")
	if e := os.WriteFile(outside, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}

	if e := os.Link(outside, target); e != nil {
		t.Fatal(e)
	}

	if e := process.AtomicWrite(target, []byte("new"), 0600); e != nil {
		t.Fatal(e)
	}

	b, e := os.ReadFile(outside)
	if e != nil || string(b) != "original" {
		t.Fatal("hardlink target overwritten")
	}
}
