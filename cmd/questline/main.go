// Команда questline — утилиты для авторов сценариев.
//
//	questline validate <scenario.yaml>
//	questline routes [-teams N] [-seed S] <scenario.yaml>
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"questline/internal/routes"
	"questline/internal/scenario"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "validate":
		err = cmdValidate(os.Args[2:])
	case "routes":
		err = cmdRoutes(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		var errs scenario.Errors
		if errors.As(err, &errs) {
			fmt.Fprintf(os.Stderr, "Найдено ошибок: %d\n", len(errs))
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Использование:
  questline validate <scenario.yaml>                  проверить сценарий
  questline routes [-teams N] [-seed S] <scenario.yaml>  показать маршруты команд
`)
}

func cmdValidate(args []string) error {
	if len(args) != 1 {
		return errors.New("укажите файл сценария")
	}
	s, err := scenario.ParseFile(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("OK: %s (%s)\n", s.Title, s.ID)
	fmt.Printf("  участники: %s, команда %d–%d\n", s.Participants.Mode, s.Participants.TeamSize.Min, s.Participants.TeamSize.Max)
	fmt.Printf("  старт: %s", s.Start.Mode)
	if s.Start.Mode == scenario.StartStaggered {
		fmt.Printf(" через %s, порядок %s", s.Start.Interval, s.Start.Order)
	}
	fmt.Println()
	fmt.Printf("  flow: %s, маршруты: %s, очки: %s\n", s.Flow.Mode, s.Flow.Routes, s.Scoring.Mode)
	if s.Timing.TimeLimit > 0 {
		fmt.Printf("  лимит: %s\n", s.Timing.TimeLimit)
	}
	fmt.Printf("  лидерборд: %s\n", s.Leaderboard.Visibility)

	points, bonuses := 0, 0
	for _, l := range s.Levels {
		points += l.Points
		for _, b := range l.Bonuses {
			bonuses += b.Points
		}
	}
	fmt.Printf("  уровней: %d, баллов: %d (+%d бонусами)\n", len(s.Levels), points, bonuses)
	return nil
}

func cmdRoutes(args []string) error {
	fs := flag.NewFlagSet("routes", flag.ContinueOnError)
	teams := fs.Int("teams", 4, "число команд")
	seed := fs.Uint64("seed", 1, "seed для routes: random")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("укажите файл сценария")
	}
	s, err := scenario.ParseFile(fs.Arg(0))
	if err != nil {
		return err
	}
	if s.Flow.Mode != scenario.FlowLinear {
		return fmt.Errorf("маршруты строятся только для flow: linear (в сценарии %s)", s.Flow.Mode)
	}
	rs, err := routes.Build(s.RouteLevels(), routes.Options{
		Mode:     s.Flow.Routes,
		Teams:    *teams,
		Seed:     *seed,
		Explicit: s.Flow.RouteList,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Маршруты (%s, команд: %d):\n", s.Flow.Routes, *teams)
	for i, r := range rs {
		fmt.Printf("  %2d: %s\n", i+1, strings.Join(r, " → "))
	}
	return nil
}
