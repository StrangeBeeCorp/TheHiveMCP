// Command analyzers runs TheHiveMCP's analyzers (internal/analyzers) over packages, test files
// included: the analyzers gate runs `make go CMD="run ./cmd/analyzers ./..."`.
package main

import (
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/analyzers"
)

func main() {
	multichecker.Main(analyzers.Untrusted, analyzers.CutFlag, analyzers.Parallel)
}
