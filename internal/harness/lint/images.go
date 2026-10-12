package lint

import (
	"errors"
	"regexp"
)

// The pinned images of the linters and formatters. renovate.json tracks the tagged images and
// the prettier version; a digest-only pin stays as it is.
const (
	golangciImage   = "golangci/golangci-lint:v2.12.2"
	shellcheckImage = "koalaman/shellcheck:v0.11.0"
	shfmtImage      = "mvdan/shfmt:v3.13.1"
	markdownImage   = "davidanson/markdownlint-cli2:v0.23.0"
	lycheeImage     = "lycheeverse/lychee:0.24.2"
	yamllintImage   = "cytopia/yamllint@sha256:3e9eb827ab2b12a5ea5f49d4257bb3aca94bba9f1ba427c8bc7f2456385a5204"
	hadolintImage   = "hadolint/hadolint:v2.14.0"
	actionlintImage = "rhysd/actionlint:1.7.7"
	checkmakeImage  = "quay.io/checkmake/checkmake@sha256:6db49d6b62e2abed57c2fd334ca5f43067126bd9419a71b13967bd0eccf6cdcb" // v0.3.2
	// prettier is not in the node image: npx fetches this exact version, so that every host wraps
	// Markdown the same way.
	prettierImage   = "node:22-alpine"
	prettierVersion = "3.9.5"
)

// Images are the Go images, which the Makefile pins (GO_IMAGE, and GO_IMAGE_CGO, whose image
// has the git the harness's tests run): a lint run builds and tests with the toolchain of make
// test.
type Images struct {
	Go     string
	GoTest string
}

var makeVar = regexp.MustCompile(`(?m)^(GO_IMAGE|GO_IMAGE_CGO)\s*:=\s*(\S+)\s*$`)

// MakefileImages reads the Go images from the Makefile.
func MakefileImages(makefile []byte) (Images, error) {
	var im Images

	for _, m := range makeVar.FindAllSubmatch(makefile, -1) {
		switch string(m[1]) {
		case "GO_IMAGE":
			im.Go = string(m[2])
		case "GO_IMAGE_CGO":
			im.GoTest = string(m[2])
		}
	}

	if im.Go == "" || im.GoTest == "" {
		return Images{}, errors.New("the Makefile pins no GO_IMAGE or no GO_IMAGE_CGO")
	}

	return im, nil
}
