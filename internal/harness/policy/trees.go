package policy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/shell"
)

const (
	cmdRsync = "rsync"
	cmdDitto = "ditto"
)

// maxTreeFiles bounds the files the policy judges under one directory a command writes whole.
const maxTreeFiles = 20000

var (
	// targeted are the copiers that take -t DIR.
	targeted = []string{"cp", "mv", "install", "ln"}
	// copiers write a copy of their sources at their last argument, or in the directory -t names.
	copiers = append(slices.Clone(targeted), cmdRsync, cmdDitto)
)

var errTooMany = errors.New("too many files")

// trees judges, for an agent, the files a command writes besides the paths it names: what a copy
// or a move puts under a directory, what a move takes away, and what rm -r, chmod -R or chown -R
// reach under a directory. A pattern is judged by the files it matches now.
func (j *judge) trees(name string, args []string) Decision {
	if !j.role.Agent() {
		return Allow
	}

	switch {
	case slices.Contains(copiers, name):
		return j.copies(name, args)
	case name == "rm" && recursiveFlag(args, "rR", "--recursive"),
		slices.Contains([]string{"chmod", "chown", "chgrp"}, name) && recursiveFlag(args, "R", "--recursive"):
		for _, p := range j.expand(positionals(args)) {
			if d := j.writeTree(p); d.Deny {
				return d
			}
		}
	}

	return Allow
}

// copies judges where a copy or a move writes: each source's copy, at the destination, or under it
// when it is a directory (-t, several sources, a trailing /, or one that exists), with the files
// of a directory it copies whole; and each source too when the command removes it (mv, rsync
// --remove-source-files). A directory's copy is judged both as itself and as its contents, which
// cp and rsync write with a trailing / on the source.
func (j *judge) copies(name string, args []string) Decision {
	sources, dir, into := j.copyArgs(name, args)
	removes := name == "mv" || (name == cmdRsync && slices.Contains(args, "--remove-source-files"))
	whole := name == "mv" || name == cmdDitto || recursiveFlag(args, "rRa", "--recursive", "--archive")

	for _, src := range j.expand(sources) {
		if removes {
			if d := j.writeTree(src); d.Deny {
				return d
			}
		}

		for _, dest := range destinations(name, args, src, dir, into, j.isDir(src)) {
			if d := j.copyTo(src, dest, whole); d.Deny {
				return d
			}
		}
	}

	return Allow
}

// copyArgs are a copy's sources, its destination, and whether that is a directory the copies go
// under. No destination means no copy.
func (j *judge) copyArgs(name string, args []string) ([]string, string, bool) {
	sources := positionals(args)

	if dir, ok := targetDirectory(name, args); ok {
		return slices.DeleteFunc(sources, func(s string) bool { return s == dir }), dir, true
	}

	if len(sources) < 2 {
		return nil, "", false
	}

	dir, sources := sources[len(sources)-1], sources[:len(sources)-1]

	return sources, dir, len(sources) > 1 || strings.HasSuffix(dir, "/") || j.isDir(dir)
}

// copyTo judges the copy of src at dest, and with whole the copy of every file under it.
func (j *judge) copyTo(src, dest string, whole bool) Decision {
	if d := j.write(dest); d.Deny || !whole {
		return d
	}

	return j.copyTree(src, dest)
}

// destinations are where a copy of src may land.
func destinations(name string, args []string, src, dir string, into, srcIsDir bool) []string {
	if !into {
		return []string{dir}
	}

	out := []string{filepath.Join(dir, filepath.Base(src))}
	if srcIsDir {
		out = append(out, dir)
	}

	if name == cmdRsync && recursiveFlag(args, "R", "--relative") && !filepath.IsAbs(src) {
		out = append(out, filepath.Join(dir, src))
	}

	return out
}

// targetDirectory is the directory cp, mv, install or ln copy into with -t DIR, -tDIR or
// --target-directory=DIR, if any.
func targetDirectory(name string, args []string) (string, bool) {
	if !slices.Contains(targeted, name) {
		return "", false
	}

	for i, a := range args {
		switch {
		case a == "--target-directory" && i+1 < len(args):
			return args[i+1], true
		case strings.HasPrefix(a, "--target-directory="):
			return strings.TrimPrefix(a, "--target-directory="), true
		case strings.HasPrefix(a, "--") || !strings.HasPrefix(a, "-"):
			continue
		}

		for k := 1; k < len(a); k++ {
			switch {
			case a[k] == 't' && k+1 < len(a):
				return a[k+1:], true
			case a[k] == 't' && i+1 < len(args):
				return args[i+1], true
			case strings.IndexByte("Smog", a[k]) >= 0:
				// An option that takes the rest of the word, or the next one, as its value.
				k = len(a)
			}
		}
	}

	return "", false
}

// positionals are the arguments that are not options.
func positionals(args []string) []string {
	var out []string

	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
		}
	}

	return out
}

// recursiveFlag reports whether the arguments hold one of the short options (alone or in a cluster
// such as -rf) or one of the long ones.
func recursiveFlag(args []string, short string, long ...string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		if strings.HasPrefix(a, "--") {
			return slices.Contains(long, a)
		}

		return strings.HasPrefix(a, "-") && strings.ContainsAny(a[1:], short)
	})
}

// expand is the words with each pattern replaced by the files it matches now, the pattern kept
// too: the shell passes it on as it is when nothing matches.
func (j *judge) expand(words []string) []string {
	var out []string

	for _, w := range words {
		w, _ = j.resolveTemp(w, false)
		out = append(out, w)

		if strings.Contains(w, shell.Computed) || !strings.ContainsAny(w, "*?[") || j.cwd == "" {
			continue
		}

		matches, _ := filepath.Glob(strings.ReplaceAll(j.abs(w), "[!", "[^"))
		out = append(out, matches...)
	}

	return out
}

func (j *judge) isDir(word string) bool {
	if strings.Contains(word, shell.Computed) || (j.cwd == "" && !filepath.IsAbs(word)) {
		return false
	}

	info, err := os.Stat(j.abs(word))

	return err == nil && info.IsDir()
}

// writeTree judges a path a command writes whole: itself, and when it is a directory (not a link
// to one) every file under it.
func (j *judge) writeTree(word string) Decision {
	if d := j.write(word); d.Deny {
		return d
	}

	return j.eachFile(word, func(_, abs string) Decision { return Write(j.role, j.root, abs) })
}

// copyTree judges the copy at dest of every file under the directory src.
func (j *judge) copyTree(src, dest string) Decision {
	return j.eachFile(src, func(rel, _ string) Decision { return j.write(filepath.Join(dest, rel)) })
}

// eachFile judges every file under the directory word names, by its path relative to it and its
// absolute path: nothing when word is not a directory. A nested repository's .git is skipped, and
// a tree past maxTreeFiles is refused.
func (j *judge) eachFile(word string, judge func(rel, abs string) Decision) Decision {
	if strings.Contains(word, shell.Computed) || (j.cwd == "" && !filepath.IsAbs(word)) {
		return Allow
	}

	top := j.abs(word)

	info, err := os.Lstat(top)
	if err != nil || !info.IsDir() {
		return Allow
	}

	var (
		d     Decision
		count int
	)

	err = filepath.WalkDir(top, func(p string, e fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case e.IsDir() && e.Name() == ".git" && p != top:
			return fs.SkipDir
		case e.IsDir():
			return nil
		}

		if count++; count > maxTreeFiles {
			return errTooMany
		}

		rel, _ := filepath.Rel(top, p)
		if d = judge(rel, p); d.Deny {
			return fs.SkipAll
		}

		return nil
	})
	switch {
	case errors.Is(err, errTooMany):
		return deny("%s holds more than %d files, too many to judge: name the files", word, maxTreeFiles)
	case err != nil:
		return deny("the policy could not read all of %s (%v): name the files", word, err)
	}

	return d
}
