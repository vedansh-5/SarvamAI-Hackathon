package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/vedansh/vans/internal/server"
)

const usage = `vans-testers manages invite codes in the VANS server's table.

  vans-testers add NAME [-daily 200]   make a code for a new tester
  vans-testers list                    every tester, and what each used today
  vans-testers off NAME|CODE           switch a code off at once
  vans-testers on NAME|CODE            switch it back on
  vans-testers feedback                what testers sent, newest first

Uses the AWS profile in AWS_PROFILE (vans-deploy if unset) and the table in
VANS_TABLE (vans-server if unset).`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if os.Getenv("AWS_PROFILE") == "" {
		_ = os.Setenv("AWS_PROFILE", "vans-deploy")
	}
	table := strings.TrimSpace(os.Getenv(server.EnvTable))
	if table == "" {
		table = "vans-server"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ledger, _, _, err := server.OnAWS(ctx, table)
	if err == nil {
		err = run(ctx, ledger, os.Args[1], os.Args[2:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vans-testers:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, d *server.Dynamo, command string, args []string) error {
	switch command {
	case "add":
		fs := flag.NewFlagSet("add", flag.ExitOnError)
		daily := fs.Int("daily", 200, "requests a day; 0 means no cap of their own")
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return fmt.Errorf("add needs the tester's name first")
		}
		name := args[0]
		_ = fs.Parse(args[1:])
		t := server.Tester{Name: name, Code: server.NewCode(), Daily: *daily}
		if err := d.AddTester(ctx, t); err != nil {
			return err
		}
		fmt.Printf("%s can use VANS with the code\n\n  %s\n\nup to %d requests a day.\n", t.Name, t.Code, t.Daily)
		return nil
	case "list":
		testers, err := d.Testers(ctx)
		if err != nil {
			return err
		}
		used, err := d.Used(ctx, server.Day(time.Now()))
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tCODE\tTODAY\tDAILY\t")
		for _, t := range testers {
			state := ""
			if t.Off {
				state = "off"
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", t.Name, t.Code, used[t.Code], t.Daily, state)
		}
		fmt.Fprintf(w, "everyone\t\t%d\t\t\n", used["all"])
		return w.Flush()
	case "off", "on":
		if len(args) != 1 {
			return fmt.Errorf("%s needs one name or code", command)
		}
		t, err := find(ctx, d, args[0])
		if err != nil {
			return err
		}
		if err := d.SetOff(ctx, t.Code, command == "off"); err != nil {
			return err
		}
		fmt.Printf("%s's code is %s.\n", t.Name, command)
		return nil
	case "feedback":
		all, err := d.Feedback(ctx)
		if err != nil {
			return err
		}
		if len(all) == 0 {
			fmt.Println("No feedback yet.")
		}
		for _, f := range all {
			fmt.Printf("%s  %s  %s\n%s\n\n", f.At.Local().Format("2 Jan 15:04"), f.From, strings.ReplaceAll(f.Kind, "_", " "), f.Text)
		}
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", command, usage)
}

func find(ctx context.Context, d *server.Dynamo, nameOrCode string) (server.Tester, error) {
	testers, err := d.Testers(ctx)
	if err != nil {
		return server.Tester{}, err
	}
	var found []server.Tester
	for _, t := range testers {
		if t.Code == nameOrCode || strings.EqualFold(t.Name, nameOrCode) {
			found = append(found, t)
		}
	}
	switch len(found) {
	case 0:
		return server.Tester{}, fmt.Errorf("no tester is called or has the code %q", nameOrCode)
	case 1:
		return found[0], nil
	}
	return server.Tester{}, fmt.Errorf("%d testers are called %q; use the code", len(found), nameOrCode)
}
