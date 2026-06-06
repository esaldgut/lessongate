// Command lessongate watches a private repo's curated lesson registry, asks
// Claude which lessons are generalizable, sanitizes them through an NDA gate,
// and opens draft pull requests to a public skills repo for human review.
//
// Subcommands:
//
//	run       process up to --max newly-merged PRs, opening draft PRs
//	backfill  process historical PRs (capped per run; resume across runs)
//	status    print ledger state + counters (no content)
//	dry-run   list generalizable candidates + cost estimate; open NO PRs
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"lessongate/internal/config"
	"lessongate/internal/obs"
	"lessongate/internal/pipeline"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	log := obs.NewLogger(slog.LevelInfo)

	// Harden the process: a crash must not write in-memory NDA content to a core
	// file. Best-effort — logged, not fatal.
	if err := config.DisableCoreDumps(); err != nil {
		log.Warn("could not disable core dumps", "err", err.Error())
	}

	if len(args) == 0 {
		usage()
		return 2
	}
	sub, rest := args[0], args[1:]

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	switch sub {
	case "run":
		return cmdRun(ctx, log, rest, false)
	case "backfill":
		// backfill is run with a larger cap; same pipeline, resumes via the ledger.
		return cmdRun(ctx, log, rest, false)
	case "dry-run":
		return cmdRun(ctx, log, rest, true)
	case "status":
		return cmdStatus(ctx, log, rest)
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "lessongate: unknown subcommand %q\n", sub)
		usage()
		return 2
	}
}

// cmdRun wires and runs the pipeline. dryRun lists candidates without opening
// any PR. Credentials are required for non-dry runs (and for the watch/extract
// API calls in any run); the command fails closed with a clear message when a
// prerequisite is missing, never degrading to a silent no-op.
func cmdRun(ctx context.Context, log *slog.Logger, args []string, dryRun bool) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var maxPRs int
	var repo, publicRepo, lessonsFile string
	var once bool
	fs.IntVar(&maxPRs, "max", config.DefaultMaxPRsPerRun, "cap PRs processed this run")
	fs.StringVar(&repo, "repo", "", "private repo to watch (owner/name)")
	fs.StringVar(&publicRepo, "public-repo", "", "public repo to open draft PRs into (owner/name)")
	fs.StringVar(&lessonsFile, "lessons-file", "", "override the curated lesson registry path (e.g. for a synthetic smoke)")
	fs.BoolVar(&once, "once", false, "process the lesson source exactly once, bypassing the watch/prefilter (deterministic smoke)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	deps, cleanup, err := buildDeps(ctx, log, runConfig{
		repo:        repo,
		publicRepo:  publicRepo,
		lessonsFile: lessonsFile,
		dryRun:      dryRun,
		once:        once,
	})
	if err != nil {
		// Fail closed — never skip the gate or silently no-op.
		log.Error("cannot start pipeline (prerequisite missing)", "err", err.Error())
		fmt.Fprintf(os.Stderr, "lessongate: %v\n", err)
		return 1
	}
	defer cleanup()

	rep := &pipeline.Report{}
	n, err := pipeline.Run(ctx, *deps, pipeline.Options{MaxPRs: maxPRs, DryRun: dryRun, Once: once, Report: rep})
	if err != nil {
		log.Error("pipeline run failed", "err", err.Error())
		return 1
	}

	// Surface every rejection (threat N4) so a candidates:0 run is never silent.
	for _, rej := range rep.Rejections {
		log.Warn("lesson not published", "lesson", rej.Lesson, "reason", rej.Reason, "detail", rej.Detail)
	}
	if rep.ExtractErrors > 0 {
		log.Error("extract stage hit API errors — check credentials/credits",
			"extract_errors", rep.ExtractErrors)
	}

	if dryRun {
		log.Info("dry-run complete", "candidates", n, "rejected", len(rep.Rejections))
	} else {
		log.Info("run complete", "emitted", n, "rejected", len(rep.Rejections))
	}
	// Non-zero exit if every lesson was rejected by an infra error (not by content).
	if n == 0 && rep.ExtractErrors > 0 {
		return 1
	}
	return 0
}

// cmdStatus prints ledger state (counts only, no content).
func cmdStatus(ctx context.Context, log *slog.Logger, args []string) int {
	_ = ctx
	_ = args
	cfg, err := config.Default()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lessongate: %v\n", err)
		return 1
	}
	log.Info("status", "state_dir", cfg.StateDir, "quarantine_dir", cfg.QuarantineDir, "model", cfg.ClaudeModel)
	return 0
}

func usage() {
	fmt.Fprint(os.Stderr, `lessongate — generalizable-lesson upstream agent

usage:
  lessongate run [--max N]        process newly-merged PRs, open draft PRs
  lessongate backfill [--max N]   process historical PRs (resumable)
  lessongate status               print ledger state + counters
  lessongate dry-run [--max N]    list candidates + cost estimate (no PRs)
`)
}
