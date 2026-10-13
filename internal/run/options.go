package run

import (
	"errors"
	"fmt"
)

// Options are one run's flag values. A nil pointer is a flag not given, so an
// unset --since differs from an empty one and an unset offset from 0.
type Options struct {
	Paths         []string
	Check         bool
	Diff          bool
	Staged        bool
	Since         *string
	Config        string
	NoConfig      bool
	StdinFilepath string
	ShowConfig    string
	RangeStart    *int
	RangeEnd      *int
}

func (o Options) validate() error {
	if o.Config != "" && o.NoConfig {
		return errors.New("--config and --no-config are mutually exclusive")
	}

	if err := o.validateRange(); err != nil {
		return err
	}

	if o.ShowConfig != "" {
		if len(o.Paths) > 0 || o.StdinFilepath != "" || o.Staged || o.Since != nil || o.Check || o.Diff {
			return errors.New("--show-config takes one file and does not combine with paths, --stdin-filepath, --staged, --since, --check or --diff")
		}

		return nil
	}

	if o.Staged {
		return o.validateStaged()
	}

	if o.Since != nil {
		return o.validateSince()
	}

	if o.StdinFilepath == "" {
		return nil
	}

	if len(o.Paths) > 0 {
		return errors.New("--stdin-filepath does not accept path arguments")
	}

	if o.Check {
		return errors.New("--stdin-filepath does not combine with --check")
	}

	if o.Diff {
		return errors.New("--stdin-filepath does not combine with --diff")
	}

	return nil
}

func (o Options) validateStaged() error {
	if o.StdinFilepath != "" {
		return errors.New("--staged does not combine with --stdin-filepath")
	}

	if o.Since != nil {
		return errors.New("--staged does not combine with --since")
	}

	if len(o.Paths) > 0 {
		return errors.New("--staged does not accept path arguments")
	}

	if o.Check {
		return errors.New("--staged does not combine with --check")
	}

	if o.Diff {
		return errors.New("--staged does not combine with --diff")
	}

	return nil
}

// validateSince refuses an empty ref: CI passing an unset variable would
// otherwise run over the whole repository instead of the changed files.
func (o Options) validateSince() error {
	if *o.Since == "" {
		return errors.New("--since needs a git ref, such as origin/main")
	}

	if o.StdinFilepath != "" {
		return errors.New("--since does not combine with --stdin-filepath")
	}

	if len(o.Paths) > 0 {
		return errors.New("--since does not accept path arguments")
	}

	return nil
}

func (o Options) validateRange() error {
	if !o.ranged() {
		return nil
	}

	if o.StdinFilepath == "" {
		return errors.New("--range-start and --range-end require --stdin-filepath")
	}

	start, end := o.rangeStart(), 0
	if o.RangeEnd != nil {
		end = *o.RangeEnd
	}

	if start < 0 || end < 0 {
		return errors.New("--range-start and --range-end must not be negative")
	}

	if o.RangeEnd != nil && end < start {
		return fmt.Errorf("--range-end (%d) must not come before --range-start (%d)", end, start)
	}

	return nil
}

func (o Options) ranged() bool {
	return o.RangeStart != nil || o.RangeEnd != nil
}

func (o Options) rangeStart() int {
	if o.RangeStart == nil {
		return 0
	}

	return *o.RangeStart
}

// An editor's offsets may come from a buffer that has moved on, so one past
// the end narrows the range instead of failing the run.
func (o Options) resolveRange(size int) (int, int) {
	end := size

	if o.RangeEnd != nil {
		end = min(*o.RangeEnd, size)
	}

	return min(o.rangeStart(), size), end
}

// --staged and --since name no paths, so the anchor falls back to the
// working directory, the repository git resolves against anyway.
func (o Options) configAnchorPaths() []string {
	if o.ShowConfig != "" {
		return []string{o.ShowConfig}
	}

	if o.StdinFilepath != "" {
		return []string{o.StdinFilepath}
	}

	return o.Paths
}
