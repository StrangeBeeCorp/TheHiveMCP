// Command coverage holds the unit suite's coverage to the per-package floors of
// gates/coverage.yaml (the coverage gate), or proposes floors from a measurement.
//
//	go run ./cmd/coverage --profile coverage.out --floors gates/coverage.yaml [--propose]
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/coverage"
)

const module = "github.com/StrangeBeeCorp/TheHiveMCP"

var errUnderFloor = errors.New("coverage under its floor")

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "coverage:", err)
		os.Exit(1)
	}
}

func run() error {
	profile := flag.String("profile", "coverage.out", "the profile of go test -coverprofile")
	floorsPath := flag.String("floors", "gates/coverage.yaml", "the per-package floors")
	propose := flag.Bool("propose", false, "print floors from this measurement, one point under it, instead of checking")

	flag.Parse()

	f, err := os.Open(*profile)
	if err != nil {
		return fmt.Errorf("open the profile (make test COVERAGE=1 writes it): %w", err)
	}
	defer func() { _ = f.Close() }()

	measured, err := coverage.Parse(f, module)
	if err != nil {
		return fmt.Errorf("parse %s: %w", *profile, err)
	}

	if *propose {
		data, err := yaml.Marshal(coverage.Propose(measured))
		if err != nil {
			return fmt.Errorf("encode the floors: %w", err)
		}

		_, err = os.Stdout.Write(data)

		return err //nolint:wrapcheck // stdout
	}

	data, err := os.ReadFile(*floorsPath)
	if err != nil {
		return fmt.Errorf("read the floors: %w", err)
	}

	var floors coverage.Floors

	err = yaml.Unmarshal(data, &floors)
	if err != nil {
		return fmt.Errorf("parse %s: %w", *floorsPath, err)
	}

	problems := coverage.Check(measured, floors)
	for _, p := range problems {
		fmt.Println(p)
	}

	fmt.Println(coverage.Summary(measured, floors))

	if len(problems) > 0 {
		return fmt.Errorf("%w: %d package(s)", errUnderFloor, len(problems))
	}

	return nil
}
