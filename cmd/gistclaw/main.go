package main

import (
	"fmt"
	"os"
)

const usage = `Usage: gistclaw <command> [options]

Commands:
  serve      Start the GistClaw daemon
  run        Submit a task directly
  inspect    Inspect daemon state

Inspect subcommands:
  inspect status           Show active runs, interrupted runs, pending approvals
  inspect runs             List all runs with status
  inspect replay <run_id>  Print replay for a run
  inspect token            Print admin token from settings table

Flags:
  -h    Show this help message
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "-h", "--help", "help":
		fmt.Print(usage)
		os.Exit(0)
	case "serve":
		runServe()
	case "run":
		runTask()
	case "inspect":
		runInspect()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", os.Args[1], usage)
		os.Exit(1)
	}
}

func runServe() {
	fmt.Println("gistclaw serve: not yet implemented")
	os.Exit(0)
}

func runTask() {
	fmt.Println("gistclaw run: not yet implemented")
	os.Exit(0)
}

func runInspect() {
	if len(os.Args) < 3 {
		fmt.Fprint(os.Stderr, "Usage: gistclaw inspect <subcommand>\n\nSubcommands:\n  status    Show active runs, interrupted runs, pending approvals\n  runs      List all runs with status\n  replay    Print replay for a run\n  token     Print admin token\n")
		os.Exit(1)
	}

	switch os.Args[2] {
	case "status":
		fmt.Println("gistclaw inspect status: not yet implemented")
	case "runs":
		fmt.Println("gistclaw inspect runs: not yet implemented")
	case "replay":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: gistclaw inspect replay <run_id>")
			os.Exit(1)
		}
		fmt.Printf("gistclaw inspect replay %s: not yet implemented\n", os.Args[3])
	case "token":
		fmt.Println("gistclaw inspect token: not yet implemented")
	default:
		fmt.Fprintf(os.Stderr, "unknown inspect subcommand: %s\n", os.Args[2])
		os.Exit(1)
	}
}
