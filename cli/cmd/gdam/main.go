package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aviorstudio/gdam/cli/internal/commands"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run(os.Args))
}

func run(args []string) int {
	if len(args) < 2 {
		printUsage()
		return 2
	}

	cmd := args[1]
	switch cmd {
	case "-h", "--help", "help":
		printUsage()
		return 0
	case "-v", "--version", "version":
		printVersion()
		return 0
	case "init":
		return runInit(args[2:])
	case "add":
		return runAdd(args[2:])
	case "remove", "rm":
		return runRemove(args[2:])
	case "link":
		return runLink(args[2:])
	case "unlink":
		return runUnlink(args[2:])
	case "install":
		return runInstall(args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		return 2
	}
}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: gdam init")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := commands.Init(ctx, commands.InitOptions{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runAdd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: gdam add @username/addon[@version]")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := commands.Add(ctx, commands.AddOptions{
		Spec: fs.Arg(0),
	}); err != nil {
		if errors.Is(err, commands.ErrUserInput) {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runRemove(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: gdam remove @username/addon")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := commands.Remove(ctx, commands.RemoveOptions{
		Spec: fs.Arg(0),
	}); err != nil {
		if errors.Is(err, commands.ErrUserInput) {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runLink(args []string) int {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() != 1 && fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: gdam link @username/addon [local_path]")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var localPath string
	if fs.NArg() == 2 {
		localPath = fs.Arg(1)
	}
	opts := commands.LinkOptions{
		Spec: fs.Arg(0),
		Path: localPath,
	}

	if err := commands.Link(ctx, opts); err != nil {
		if errors.Is(err, commands.ErrUserInput) {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runUnlink(args []string) int {
	fs := flag.NewFlagSet("unlink", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	all := fs.Bool("all", false, "unlink all linked addons")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *all {
		if fs.NArg() != 0 {
			fmt.Fprintln(os.Stderr, "usage: gdam unlink --all")
			return 2
		}
	} else if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: gdam unlink @username/addon")
		fmt.Fprintln(os.Stderr, "       gdam unlink --all")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var err error
	if *all {
		err = commands.UnlinkAll(ctx, commands.UnlinkAllOptions{})
	} else {
		err = commands.Unlink(ctx, commands.UnlinkOptions{
			Spec: fs.Arg(0),
		})
	}
	if err != nil {
		if errors.Is(err, commands.ErrUserInput) {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: gdam install")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := commands.Install(ctx, commands.InstallOptions{}); err != nil {
		if errors.Is(err, commands.ErrUserInput) {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `gdam - Godot addon manager (GitHub addons installer)

Usage:
  gdam --version
  gdam init
  gdam add @username/addon[@version]
  gdam install
  gdam remove @username/addon
  gdam link @username/addon [local_path]
  gdam unlink @username/addon
  gdam unlink --all

Environment:
  GITHUB_TOKEN   Optional GitHub token to avoid rate limits.`)
}

func printVersion() {
	fmt.Printf("gdam %s\ncommit: %s\nbuilt: %s\n", version, commit, date)
}
