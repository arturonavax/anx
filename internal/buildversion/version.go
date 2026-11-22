// Package buildversion derives source-build metadata from local Git history.
package buildversion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// The last pre-policy commit used 1.0.0. Do not bump this checkpoint for routine commits.
const checkpoint = "cccb2a40926f9d3f7018121305142619db3c00e7"

type revision struct{ major, minor, patch int }

func (v revision) String() string { return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch) }

var (
	conventional = regexp.MustCompile(`(?i)^([a-z][a-z0-9-]*)(?:\([^)]+\))?(!)?: +\S`)
	footerToken  = regexp.MustCompile(`^(BREAKING(?: |-)CHANGE|[A-Za-z0-9-]+)(?:: +| +#)\S`)
)

func hasBreakingFooter(body string) bool {
	blank, inFooters := true, false
	for _, line := range strings.Split(body, "\n") {
		if (blank || inFooters) && footerToken.MatchString(line) {
			inFooters = true
			if strings.HasPrefix(line, "BREAKING CHANGE:") || strings.HasPrefix(line, "BREAKING-CHANGE:") {
				return true
			}
		}
		blank = strings.TrimSpace(line) == ""
	}
	return false
}

func (v *revision) advance(subject, body string) {
	m := conventional.FindStringSubmatch(subject)
	if m == nil {
		return
	}
	kind := strings.ToLower(m[1])
	switch {
	case m[2] == "!" || hasBreakingFooter(body):
		*v = revision{major: v.major + 1}
	case kind == "feat":
		v.minor++
		v.patch = 0
	case kind == "fix" || kind == "perf":
		v.patch++
	}
}

// Info describes an unstamped source build and its proposed stable version.
type Info struct {
	Version, Commit, Date string
	Next                  string
}

// Resolve returns development provenance and the planned stable version. On
// incomplete Git history it returns an honest fallback plus a diagnostic.
func Resolve(root string) (Info, error) { return resolve(root, checkpoint) }

func resolve(root, anchor string) (Info, error) {
	v := revision{major: 1}
	info := Info{Version: v.String() + "-dev+unknown", Commit: "dev"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	git := func(args ...string) (string, error) {
		opts := []string{
			"--no-optional-locks", "--no-replace-objects",
			"-c", "core.fsmonitor=false", "-c", "protocol.allow=never",
		}
		cmd := exec.CommandContext(ctx, "git", append(opts, args...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_ALLOW_PROTOCOL=", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git %s: %w", args[0], err)
		}
		return string(out), nil
	}
	prefix, err := git("rev-parse", "--show-prefix")
	if err != nil || strings.TrimSpace(prefix) != "" {
		return info, errors.New("local Git checkout root is unavailable")
	}
	head, err := git("log", "-1", "--no-show-signature", "--format=%H%x00%cI", "HEAD")
	if err != nil {
		return info, err
	}
	hash, stampedAt, ok := strings.Cut(strings.TrimSpace(head), "\x00")
	at, dateErr := time.Parse(time.RFC3339, stampedAt)
	if !ok || len(hash) < 7 || dateErr != nil {
		return info, errors.New("invalid Git commit metadata")
	}
	info.Commit, info.Date = hash[:7], at.UTC().Format(time.DateOnly)

	var distance int
	if hash != anchor {
		log, err := git("log", "-z", "--first-parent", "--reverse", "--boundary",
			"--no-show-signature", "--format=%m%x00%H%x00%s%x00%b", anchor+".."+hash)
		if err != nil {
			return info, fmt.Errorf("version checkpoint unavailable: %w", err)
		}
		fields := strings.Split(strings.TrimSuffix(log, "\x00"), "\x00")
		if len(fields)%4 != 0 || fields[0] != "-" || fields[1] != anchor {
			return info, errors.New("version checkpoint is not on the first-parent history")
		}
		for i := 4; i < len(fields); i += 4 {
			if fields[i] != ">" {
				return info, errors.New("incomplete first-parent version history")
			}
			v.advance(fields[i+2], fields[i+3])
			distance++
		}
	}
	status, err := git("status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return info, err
	}
	info.Next = v.String()
	info.Version = fmt.Sprintf("%s-dev.%d+g%s", info.Next, distance, info.Commit)
	if status != "" {
		info.Version += ".dirty"
	}
	return info, nil
}
